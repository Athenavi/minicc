"""Tests for app.trace.writer module.

Verifies:
1. TraceWriter writes to Redis Stream (mocked)
2. record_span convenience function works
3. Graceful degradation when Redis is unavailable
"""
import asyncio
import pytest
from unittest.mock import AsyncMock, MagicMock, patch


class TestTraceWriter:
    """TraceWriter 单元测试."""
    
    @pytest.mark.asyncio
    async def test_write_span_without_redis(self):
        """Redis 不可用时,write_span 应静默跳过 (不崩溃)."""
        from app.trace.writer import TraceWriter
        
        writer = await TraceWriter.get_instance()
        
        # Redis 未配置 → xadd 不会被调用
        with patch.object(writer, '_redis', None):
            await writer.write_span(
                trace_id="test123",
                span_name="llm_call",
                duration_ms=1500,
                metadata={"model": "gpt-4"},
            )
            # No exception → success
    
    @pytest.mark.asyncio
    async def test_write_span_to_redis(self):
        """Redis 可用时,write_span 应写入 Stream."""
        from app.trace.writer import TraceWriter
        
        mock_redis = AsyncMock()
        mock_redis.ping = AsyncMock()
        mock_redis.xadd = AsyncMock(return_value="1234567890-0")
        
        with patch('app.trace.writer.TraceWriter._redis', mock_redis):
            writer = await TraceWriter.get_instance()
            await writer.write_span(
                trace_id="abc123",
                span_name="tool:read_file",
                duration_ms=250,
                metadata={"file": "test.py"},
            )
            
            # Verify xadd was called with correct args
            mock_redis.xadd.assert_called_once()
            call_args = mock_redis.xadd.call_args
            assert call_args[0][0] == "minicc:traces"  # stream name
            entry = call_args[0][1]
            assert entry["trace_id"] == "abc123"
            assert entry["span_name"] == "tool:read_file"
            assert entry["duration_ms"] == "250"
            assert 'metadata' in entry
    
    @pytest.mark.asyncio
    async def test_write_batch(self):
        """批量写入应使用 Redis pipeline."""
        from app.trace.writer import TraceWriter
        
        mock_redis = AsyncMock()
        mock_redis.pipeline = MagicMock()
        mock_pipeline = AsyncMock()
        mock_redis.pipeline.return_value = mock_pipeline
        mock_pipeline.execute = AsyncMock()
        
        spans = [
            {"trace_id": "t1", "span_name": "s1", "duration_ms": 100},
            {"trace_id": "t2", "span_name": "s2", "duration_ms": 200},
        ]
        
        with patch('app.trace.writer.TraceWriter._redis', mock_redis):
            writer = await TraceWriter.get_instance()
            await writer.write_batch(spans)
            
            # Verify pipeline usage
            mock_redis.pipeline.assert_called_once_with(transaction=False)
            assert mock_pipeline.xadd.call_count == 2
            await mock_pipeline.execute()
            mock_pipeline.execute.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_get_instance_singleton(self):
        """TraceWriter 应为单例."""
        from app.trace.writer import TraceWriter
        
        w1 = await TraceWriter.get_instance()
        w2 = await TraceWriter.get_instance()
        assert w1 is w2
    
    @pytest.mark.asyncio
    async def test_record_span_convenience(self):
        """record_span 便捷函数应自动初始化 TraceWriter."""
        from app.trace import record_span
        
        mock_redis = AsyncMock()
        mock_redis.ping = AsyncMock()
        mock_redis.xadd = AsyncMock(return_value="123")
        
        with patch('app.trace.writer.TraceWriter._redis', mock_redis):
            await record_span(
                trace_id="conv_test",
                span_name="workflow_node",
                duration_ms=500,
                metadata={"node_id": "llm_1"},
            )
            
            mock_redis.xadd.assert_called_once()


class TestTraceEventFields:
    """AgentEvent 新增 trace 字段验证."""
    
    def test_agent_event_has_trace_fields(self):
        """AgentEvent dataclass 应包含 trace_id/span_name/duration_ms."""
        from app.agent.runtime import AgentEvent
        
        event = AgentEvent(
            type="trace_span",
            trace_id="test_trace",
            span_name="llm_call",
            duration_ms=1234,
        )
        
        assert event.trace_id == "test_trace"
        assert event.span_name == "llm_call"
        assert event.duration_ms == 1234
    
    def test_agent_event_backwards_compatible(self):
        """旧代码创建 AgentEvent 不应因缺少 trace 字段而失败."""
        from app.agent.runtime import AgentEvent
        
        # 旧写法 (不含 trace 字段)
        event = AgentEvent(type="text", content="hello")
        
        assert event.type == "text"
        assert event.content == "hello"
        assert event.trace_id == ""  # default
        assert event.span_name == ""  # default
        assert event.duration_ms == 0  # default
