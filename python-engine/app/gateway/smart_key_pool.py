"""
智能 API Key 池 — 多 Key 轮询 + 动态权重 + 熔断
"""
from __future__ import annotations

import asyncio
import logging
import random
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional

logger = logging.getLogger(__name__)


class KeyStatus(str, Enum):
    """Key 状态"""
    ACTIVE = "active"           # 正常
    RATE_LIMITED = "rate_limited"  # 限流中
    CIRCUIT_OPEN = "circuit_open"  # 熔断


@dataclass
class CircuitBreaker:
    """熔断器"""
    failure_threshold: int = 5
    recovery_timeout: float = 60.0
    _failure_count: int = field(default=0, init=False)
    _last_failure_time: float = field(default=0.0, init=False)
    _state: str = field(default="closed", init=False)
    
    def record_success(self):
        """记录成功"""
        self._failure_count = 0
        self._state = "closed"
    
    def record_failure(self):
        """记录失败"""
        self._failure_count += 1
        self._last_failure_time = time.time()
        
        if self._failure_count >= self.failure_threshold:
            self._state = "open"
            logger.warning("Circuit breaker opened after %d failures", self._failure_count)
    
    def is_open(self) -> bool:
        """检查是否熔断"""
        if self._state == "closed":
            return False
        
        # 检查是否可以恢复
        if time.time() - self._last_failure_time > self.recovery_timeout:
            self._state = "half_open"
            return False
        
        return True
    
    def is_half_open(self) -> bool:
        """检查是否半开"""
        return self._state == "half_open"


@dataclass
class APIKeyInfo:
    """API Key 信息"""
    key: str
    provider: str
    weight: float = 1.0
    failures: int = 0
    last_used: float = 0.0
    status: KeyStatus = KeyStatus.ACTIVE
    circuit_breaker: Optional[CircuitBreaker] = None
    remark: str = ""


class SmartAPIKeyPool:
    """
    智能 API Key 池
    
    功能：
    1. 多 Key 轮询，绕过单账户 Rate Limit
    2. 动态权重，根据延迟和成功率调整
    3. 熔断机制，Key 失效时自动切换
    """
    
    def __init__(
        self,
        keys: dict[str, list[str]] | None = None,
        failure_threshold: int = 5,
        recovery_timeout: float = 60.0,
        weight_decay: float = 0.5,
        weight_recovery: float = 1.1,
        max_weight: float = 2.0,
    ):
        self._failure_threshold = failure_threshold
        self._recovery_timeout = recovery_timeout
        self._weight_decay = weight_decay
        self._weight_recovery = weight_recovery
        self._max_weight = max_weight
        self._lock = asyncio.Lock()
        
        # 初始化 Key 池
        self._pools: dict[str, list[APIKeyInfo]] = {}
        if keys:
            for provider, key_list in keys.items():
                self._pools[provider] = [
                    APIKeyInfo(
                        key=key,
                        provider=provider,
                        circuit_breaker=CircuitBreaker(
                            failure_threshold=failure_threshold,
                            recovery_timeout=recovery_timeout,
                        ),
                    )
                    for key in key_list
                ]
    
    async def get_key(self, provider: str) -> Optional[str]:
        """
        获取可用的 API Key
        
        Args:
            provider: 提供商名称
        
        Returns:
            API Key，无可用则返回 None
        """
        async with self._lock:
            pool = self._pools.get(provider, [])
            if not pool:
                return None
            
            # 过滤可用的 Key
            available = [
                k for k in pool
                if not k.circuit_breaker or not k.circuit_breaker.is_open()
            ]
            
            if not available:
                # 全部熔断，计算最短等待时间
                logger.warning("All keys for %s are circuit open, waiting for recovery", provider)
                wait_time = self._calc_recovery_wait(pool)
                if wait_time < float('inf'):
                    # 释放锁后等待，避免死锁
                    pass
                else:
                    return None
            else:
                # 按权重选择
                total_weight = sum(k.weight for k in available)
                r = random.uniform(0, total_weight)
                
                cumulative = 0
                for key_info in available:
                    cumulative += key_info.weight
                    if r <= cumulative:
                        key_info.last_used = time.time()
                        return key_info.key
                
                # Fallback: 返回第一个
                available[0].last_used = time.time()
                return available[0].key
        
        # 在锁外等待恢复，然后重试
        logger.info("Waiting %.1f seconds for key recovery", wait_time)
        await asyncio.sleep(wait_time)
        return await self.get_key(provider)
    
    def _calc_recovery_wait(self, pool: list) -> float:
        """计算最近的 Key 恢复等待时间（需在锁内调用）"""
        min_wait = float('inf')
        for k in pool:
            if k.circuit_breaker and k.circuit_breaker.is_open():
                wait_time = k.circuit_breaker.recovery_timeout - (time.time() - k.circuit_breaker._last_failure_time)
                min_wait = min(min_wait, max(0, wait_time))
        return min_wait
    
    async def report_success(self, key: str, latency: float = 0.0) -> None:
        """
        报告成功，调整权重
        
        Args:
            key: API Key
            latency: 延迟（毫秒）
        """
        async with self._lock:
            for pool in self._pools.values():
                for k in pool:
                    if k.key == key:
                        k.failures = max(0, k.failures - 1)
                        k.weight = min(self._max_weight, k.weight * self._weight_recovery)
                        k.status = KeyStatus.ACTIVE
                        
                        if k.circuit_breaker:
                            k.circuit_breaker.record_success()
                        
                        return
    
    async def report_failure(self, key: str, error: str = "") -> None:
        """
        报告失败，降低权重或熔断
        
        Args:
            key: API Key
            error: 错误信息
        """
        async with self._lock:
            for pool in self._pools.values():
                for k in pool:
                    if k.key == key:
                        k.failures += 1
                        k.weight *= self._weight_decay
                        
                        if k.circuit_breaker:
                            k.circuit_breaker.record_failure()
                            
                            if k.circuit_breaker.is_open():
                                k.status = KeyStatus.CIRCUIT_OPEN
                                logger.warning("Key %s circuit opened: %s", key[:20], error)
                        
                        return
    

    async def add_key(self, provider: str, key: str, remark: str = "") -> None:
        """添加 Key"""
        async with self._lock:
            if provider not in self._pools:
                self._pools[provider] = []
            
            self._pools[provider].append(APIKeyInfo(
            key=key,
            provider=provider,
            remark=remark,
            circuit_breaker=CircuitBreaker(
                failure_threshold=self._failure_threshold,
                recovery_timeout=self._recovery_timeout,
            ),
        ))
    
    async def remove_key(self, provider: str, key: str) -> bool:
        """删除 Key"""
        async with self._lock:
            pool = self._pools.get(provider, [])
            for i, k in enumerate(pool):
                if k.key == key:
                    pool.pop(i)
                    return True
        return False
    
    def get_stats(self) -> dict:
        """获取统计信息"""
        stats = {
            "total": 0,
            "active": 0,
            "rate_limited": 0,
            "circuit_open": 0,
            "providers": {},
        }
        
        for provider, pool in self._pools.items():
            provider_stats = {
                "total": len(pool),
                "active": 0,
                "rate_limited": 0,
                "circuit_open": 0,
            }
            
            for k in pool:
                stats["total"] += 1
                
                if k.status == KeyStatus.ACTIVE:
                    stats["active"] += 1
                    provider_stats["active"] += 1
                elif k.status == KeyStatus.RATE_LIMITED:
                    stats["rate_limited"] += 1
                    provider_stats["rate_limited"] += 1
                elif k.status == KeyStatus.CIRCUIT_OPEN:
                    stats["circuit_open"] += 1
                    provider_stats["circuit_open"] += 1
            
            stats["providers"][provider] = provider_stats
        
        return stats
    
    def get_all_keys(self) -> list[dict]:
        """获取所有 Key 信息"""
        keys = []
        for provider, pool in self._pools.items():
            for k in pool:
                keys.append({
                    "provider": k.provider,
                    "key": k.key[:20] + "...",
                    "weight": k.weight,
                    "failures": k.failures,
                    "status": k.status.value,
                    "last_used": k.last_used,
                    "remark": k.remark,
                })
        return keys
