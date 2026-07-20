"""
任务消费者 — 消费 Redis Stream 任务
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
from typing import Optional

import redis.asyncio as aioredis

from app.agent.runtime import AgentRuntime, AgentTask
from app.sse.producer import SSEProducer

logger = logging.getLogger(__name__)


class AgentTaskConsumer:
    """
    Agent 任务消费者 — 消费 Redis Stream 任务
    
    从 tasks:agent Stream 消费任务，交给 AgentRuntime 执行
    """
    
    def __init__(
        self,
        redis: aioredis.Redis,
        runtime: AgentRuntime,
        sse_producer: SSEProducer,
        stream_key: str = "tasks:agent",
        group_name: str = "python-workers",
        concurrency: int = 5,
    ):
        self._redis = redis
        self._runtime = runtime
        self._sse = sse_producer
        self._stream_key = stream_key
        self._group_name = group_name
        self._consumer_name = f"worker-{os.getpid()}"
        self._concurrency = concurrency
        self._running = False
        self._tasks: set[asyncio.Task] = set()
    
    async def start(self) -> None:
        """启动消费者"""
        # 创建消费者组
        try:
            await self._redis.xgroup_create(
                self._stream_key,
                self._group_name,
                mkstream=True,
            )
            logger.info("Created consumer group: %s", self._group_name)
        except Exception as e:
            if "BUSYGROUP" in str(e):
                logger.info("Consumer group already exists: %s", self._group_name)
            else:
                raise
        
        self._running = True
        logger.info("Agent task consumer started (consumer=%s, concurrency=%d)", self._consumer_name, self._concurrency)
        
        # 启动消费循环
        while self._running:
            try:
                await self._consume_batch()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error("Consumer error: %s", e)
                await asyncio.sleep(1)
    
    async def stop(self) -> None:
        """停止消费者"""
        self._running = False

        # 取消并等待所有进行中的任务
        if self._tasks:
            logger.info("Cancelling %d active tasks...", len(self._tasks))
            for t in self._tasks:
                t.cancel()
            await asyncio.gather(*self._tasks, return_exceptions=True)

        logger.info("Agent task consumer stopped")
    
    async def _consume_batch(self) -> None:
        """消费一批任务"""
        try:
            messages = await self._redis.xreadgroup(
                groupname=self._group_name,
                consumername=self._consumer_name,
                streams={self._stream_key: ">"},
                count=self._concurrency,
                block=2000,  # 2 秒超时
            )
            
            if not messages:
                return
            
            for stream, msgs in messages:
                for msg_id, data in msgs:
                    # 解析任务
                    task_data = data.get("data", data)
                    if isinstance(task_data, str):
                        task_dict = json.loads(task_data)
                    else:
                        task_dict = task_data
                    
                    task = AgentTask.parse(task_dict)
                    
                    # 异步执行任务
                    asyncio_task = asyncio.create_task(
                        self._process_task(task, msg_id)
                    )
                    self._tasks.add(asyncio_task)
                    asyncio_task.add_done_callback(self._tasks.discard)
                    
        except Exception as e:
            logger.error("Error consuming batch: %s", e)
    
    async def _process_task(self, task: AgentTask, msg_id: str) -> None:
        """处理单个任务"""
        logger.info("Processing task: %s (session=%s)", task.id, task.session_id)
        
        try:
            # 执行 Agent 推理
            async for event in self._runtime.run(task):
                # 发布 SSE 事件
                await self._sse.publish(task.id, {
                    "type": event.type,
                    "content": event.content,
                    "id": event.tool_call_id,
                    "name": event.tool_name,
                    "arguments": event.tool_arguments,
                    "input_tokens": event.input_tokens,
                    "output_tokens": event.output_tokens,
                    "message": event.error,
                })
            
            # ACK 消息
            await self._redis.xack(self._stream_key, self._group_name, msg_id)
            logger.info("Task completed: %s", task.id)
            
        except Exception as e:
            logger.error("Task failed: %s - %s", task.id, e)
            
            # 发送错误事件
            await self._sse.publish_error(task.id, str(e))
            
            # ACK 消息（避免重复消费）
            await self._redis.xack(self._stream_key, self._group_name, msg_id)
