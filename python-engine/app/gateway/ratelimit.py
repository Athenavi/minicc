# 租户级限流 — Redis 滑动窗口计数器
from __future__ import annotations

import time
import logging

import redis.asyncio as aioredis

logger = logging.getLogger(__name__)


class TenantRateLimiter:
    """
    算法: Redis 有序集合滑动窗口

    key: "ratelimit:{tenant_id}:{window_type}"
    每次请求 ZADD score=timestamp, member=unique_id
    ZREMRANGEBYSCORE 清理窗口外的记录
    ZCARD 计算当前窗口内请求数
    """

    def __init__(
        self,
        redis: aioredis.Redis,
        requests_per_minute: int = 60,
        requests_per_second: int = 10,
    ):
        self._redis = redis
        self.rpm = requests_per_minute
        self.rps = requests_per_second

    async def allow(self, tenant_id: str) -> bool:
        """检查是否允许请求通过"""
        if not tenant_id:
            return True

        now = time.time()

        # 检查每秒限流
        if not await self._check_window(
            f"ratelimit:{tenant_id}:s", now, 1.0, self.rps
        ):
            logger.warning("Rate limit exceeded: tenant=%s (rps=%d)", tenant_id, self.rps)
            return False

        # 检查每分钟限流
        if not await self._check_window(
            f"ratelimit:{tenant_id}:m", now, 60.0, self.rpm
        ):
            logger.warning("Rate limit exceeded: tenant=%s (rpm=%d)", tenant_id, self.rpm)
            return False

        return True

    async def _check_window(
        self, key: str, now: float, window_seconds: float, max_requests: int
    ) -> bool:
        """滑动窗口检查 + 计数（原子操作）"""
        pipe = self._redis.pipeline(transaction=True)
        window_start = now - window_seconds

        # 清理过期记录
        pipe.zremrangebyscore(key, "-inf", window_start)
        # 计数
        pipe.zcard(key)
        # 添加当前请求
        member = f"{now}:{id(self)}"
        pipe.zadd(key, {member: now})
        # 设置 key 过期（兜底清理）
        pipe.expire(key, int(window_seconds) + 1)

        results = await pipe.execute()
        count = results[1]  # ZCARD 结果

        if count >= max_requests:
            # 超限，移除刚添加的记录
            await self._redis.zrem(key, member)
            return False

        return True

    async def get_remaining(self, tenant_id: str) -> dict:
        """返回剩余额度"""
        now = time.time()
        s_key = f"ratelimit:{tenant_id}:s"
        m_key = f"ratelimit:{tenant_id}:m"

        s_count = await self._redis.zcount(s_key, now - 1.0, now)
        m_count = await self._redis.zcount(m_key, now - 60.0, now)

        return {
            "rps_remaining": max(0, self.rps - s_count),
            "rpm_remaining": max(0, self.rpm - m_count),
        }
