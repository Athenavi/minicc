"""通用上下文总线 (ContextBus)

设计目标:
- 类似 Apache Kafka + Redis Pub/Sub 的共享内存机制
- 各工作台发布/订阅状态变更
- 支持消息持久化、回放和 TTL 自动清理
- SaaS 租户隔离

架构思想:
类比微服务架构的事件总线 (Event Bus)
- Agent 执行完成后发布结果到总线
- 下游工作流节点订阅该主题获取输入
- 支持跨进程、跨实例的状态共享
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Callable, Awaitable
from dataclasses import dataclass, field
from collections import defaultdict
from enum import Enum

logger = logging.getLogger(__name__)


class MessageType(str, Enum):
    """消息类型"""
    STATE_CHANGE = "state_change"      # 状态变更
    RESULT_PUBLISH = "result_publish"   # 结果发布
    COMMAND = "command"                 # 命令下发
    QUERY = "query"                     # 查询请求
    EVENT = "event"                     # 事件通知


@dataclass
class ContextMessage:
    """上下文消息"""
    topic: str                          # 主题: "agent.results", "workflow.state"
    message_type: MessageType
    data: dict[str, Any]
    tenant_id: str                      # SaaS 安全: 租户隔离
    message_id: str = ""                # UUID
    timestamp: float = field(default_factory=time.time)
    ttl: int = 3600                     # TTL (秒), 默认 1 小时
    reply_to: str = ""                  # 回复主题 (请求-响应模式)
    


class InMemoryContextBus:
    """内存版 ContextBus (单机测试用)"""
    
    def __init__(self):
        self._subscriptions: dict[str, list[Callable]] = defaultdict(list)  # topic -> [callbacks]
        self._message_log: dict[str, list[ContextMessage]] = defaultdict(list)  # topic -> [messages]
        self._max_log_size = 1000  # 每个主题最多保留 1000 条消息
    
    async def publish(self, topic: str, data: dict, tenant_id: str, 
                      message_type: MessageType = MessageType.RESULT_PUBLISH,
                      ttl: int = 3600) -> ContextMessage:
        """发布消息到主题"""
        import uuid
        
        message = ContextMessage(
            topic=topic,
            message_type=message_type,
            data=data,
            tenant_id=tenant_id,
            message_id=uuid.uuid4().hex[:12],
            ttl=ttl,
        )
        
        # 存储到日志
        self._message_log[topic].append(message)
        if len(self._message_log[topic]) > self._max_log_size:
            self._message_log[topic] = self._message_log[topic][-self._max_log_size:]
        
        # 触发订阅者回调
        for callback in self._subscriptions[topic]:
            try:
                if asyncio.iscoroutinefunction(callback):
                    await callback(message)
                else:
                    callback(message)
            except Exception as e:
                logger.error(f"Subscription callback failed: {e}", exc_info=True)
        
        logger.debug(f"Published message to topic '{topic}' (tenant={tenant_id}, id={message.message_id})")
        return message
    
    async def subscribe(self, topic: str, callback: Callable[[ContextMessage], Awaitable[None]]) -> None:
        """订阅主题"""
        self._subscriptions[topic].append(callback)
        logger.info(f"Subscribed to topic '{topic}'")
    
    async def unsubscribe(self, topic: str, callback: Callable) -> bool:
        """取消订阅"""
        if topic in self._subscriptions:
            try:
                self._subscriptions[topic].remove(callback)
                return True
            except ValueError:
                return False
        return False
    
    async def query(self, topic: str, tenant_id: str, limit: int = 10) -> list[ContextMessage]:
        """查询主题的历史消息"""
        messages = self._message_log.get(topic, [])
        # 过滤租户
        filtered = [m for m in messages if m.tenant_id == tenant_id]
        return filtered[-limit:]
    
    async def get_latest(self, topic: str, tenant_id: str) -> ContextMessage | None:
        """获取主题的最近一条消息"""
        messages = await self.query(topic, tenant_id, limit=1)
        return messages[0] if messages else None


class RedisContextBus:
    """Redis 版 ContextBus (生产环境使用)
    
    实现方式:
    - 使用 Redis Stream 作为消息日志
    - 使用 Redis Pub/Sub 实现实时推送
    - 使用 Redis Hash 存储最新状态 (Snapshot)
    """
    
    def __init__(self, redis_client):
        self.redis = redis_client
        self._local_subs: dict[str, list[Callable]] = defaultdict(list)
    
    async def publish(self, topic: str, data: dict, tenant_id: str,
                      message_type: MessageType = MessageType.RESULT_PUBLISH,
                      ttl: int = 3600) -> ContextMessage:
        """发布消息到 Redis Stream
        
        Stream key: contextbus:{tenant}:{topic}
        """
        import uuid
        
        message = ContextMessage(
            topic=topic,
            message_type=message_type,
            data=data,
            tenant_id=tenant_id,
            message_id=uuid.uuid4().hex[:12],
            ttl=ttl,
        )
        
        # 写入 Redis Stream
        stream_key = f"contextbus:{tenant_id}:{topic}"
        entry = {
            "message_id": message.message_id,
            "message_type": message_type.value,
            "timestamp": str(message.timestamp),
            "data": json.dumps(data, ensure_ascii=False),
            "ttl": str(ttl),
        }
        
        await self.redis.xadd(stream_key, entry, maxlen=1000, approximate=True)
        
        # 更新 Snapshot (最新状态)
        snapshot_key = f"contextbus:snapshot:{tenant_id}:{topic}"
        await self.redis.setex(
            snapshot_key,
            ttl,
            json.dumps(data, ensure_ascii=False)
        )
        
        # 本地订阅者推送 (同一进程)
        for callback in self._local_subs[topic]:
            try:
                if asyncio.iscoroutinefunction(callback):
                    await callback(message)
                else:
                    callback(message)
            except Exception as e:
                logger.error(f"Local subscription callback failed: {e}", exc_info=True)
        
        logger.debug(f"Published message to Redis Stream '{stream_key}' (id={message.message_id})")
        return message
    
    async def subscribe(self, topic: str, callback: Callable, tenant_id: str = "") -> None:
        """订阅主题 (Redis Pub/Sub)"""
        self._local_subs[topic].append(callback)
        
        if tenant_id:
            channel = f"contextbus:{tenant_id}:{topic}"
        else:
            channel = f"contextbus:*:{topic}"  # 通配符
        
        logger.info(f"Subscribed to Redis channel '{channel}'")
        # TODO: 实际应创建独立的 Redis Pub/Sub connection
    
    async def unsubscribe(self, topic: str, callback: Callable, tenant_id: str = "") -> bool:
        """取消订阅"""
        if topic in self._local_subs:
            try:
                self._local_subs[topic].remove(callback)
                return True
            except ValueError:
                return False
        return False
    
    async def query(self, topic: str, tenant_id: str, limit: int = 10) -> list[ContextMessage]:
        """查询历史消息 (从 Redis Stream)"""
        stream_key = f"contextbus:{tenant_id}:{topic}"
        
        entries = await self.redis.xrange(stream_key, count=limit)
        messages = []
        
        for entry in entries:
            data = json.loads(entry["data"])
            messages.append(ContextMessage(
                topic=topic,
                message_type=MessageType(entry["message_type"]),
                data=data,
                tenant_id=tenant_id,
                message_id=entry["message_id"],
                timestamp=float(entry["timestamp"]),
            ))
        
        return messages
    
    async def get_latest(self, topic: str, tenant_id: str) -> ContextMessage | None:
        """获取最新状态 (从 Snapshot)"""
        snapshot_key = f"contextbus:snapshot:{tenant_id}:{topic}"
        
        data = await self.redis.get(snapshot_key)
        if data:
            return ContextMessage(
                topic=topic,
                message_type=MessageType.RESULT_PUBLISH,
                data=json.loads(data),
                tenant_id=tenant_id,
                timestamp=time.time(),
            )
        return None


# ── 工厂函数 ───────────────────────────────────────────────────────
_context_bus_instance = None
_is_redis = False


async def get_context_bus(redis_client=None) -> Any:
    """获取 ContextBus 实例 (单例模式)
    
    Args:
        redis_client: Redis 客户端 (生产环境传入,否则使用内存版)
    """
    global _context_bus_instance, _is_redis
    
    if _context_bus_instance is None:
        if redis_client:
            _context_bus_instance = RedisContextBus(redis_client)
            _is_redis = True
            logger.info("ContextBus initialized with Redis")
        else:
            _context_bus_instance = InMemoryContextBus()
            _is_redis = False
            logger.info("ContextBus initialized with in-memory store")
    
    return _context_bus_instance


# ── 便捷函数 ────────────────────────────────────────────────────────
async def publish_result(topic: str, data: dict, tenant_id: str, trace_id: str = ""):
    """便捷函数: 发布结果到总线"""
    bus = await get_context_bus()
    return await bus.publish(topic, data, tenant_id, MessageType.RESULT_PUBLISH)


async def publish_state_change(topic: str, data: dict, tenant_id: str):
    """便捷函数: 发布状态变更"""
    bus = await get_context_bus()
    return await bus.publish(topic, data, tenant_id, MessageType.STATE_CHANGE)


async def subscribe_topic(topic: str, callback: Callable, tenant_id: str = ""):
    """便捷函数: 订阅主题"""
    bus = await get_context_bus()
    return await bus.subscribe(topic, callback, tenant_id)


# ── 典型使用场景 ────────────────────────────────────────────────────
"""
典型用例:

1. Agent 执行完成后发布结果:
   await publish_result(
       topic="agent.tasks.analysis",
       data={"output": "...", "status": "completed"},
       tenant_id="tenant_001",
   )

2. 工作流节点订阅结果:
   async def on_agent_result(message):
       print(f"收到结果: {message.data}")
   
   await subscribe_topic("agent.tasks.analysis", on_agent_result)

3. 查询历史状态:
   latest = await get_context_bus().get_latest("workflow.status", "tenant_001")
"""
