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
    """TraceWriter 鍗曞厓娴嬭瘯."""
    
    @pytest.mark.asyncio
    async def test_write_span_without_redis(self):
        """Redis 涓嶅彲鐢ㄦ椂,write_span 搴旈潤榛樿烦杩?(涓嶅穿婧?."""
        from app.trace.writer import TraceWriter
        
        writer = await TraceWriter.get_instance()
        
        # Redis 鏈厤缃?鈫?xadd 涓嶄細琚皟鐢?        with patch.object(writer, '_redis', None):
            await writer.write_span(
                trace_id="test123",
                span_name="llm_call",
                duration_ms=1500,
                metadata={"model": "gpt-4"},
            )
            # No exception 鈫?success
    
    @pytest.mark.asyncio
    async def test_write_span_to_redis(self):
        """Redis 鍙敤鏃?write_span 搴斿啓鍏?Stream."""
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
            # 澶氱鎴疯璁★細鏃?tenant_id 鏃跺啓鍏?anonymous stream
            assert call_args[0][0] == "chiron:traces:anonymous"  # stream name
            entry = call_args[0][1]
            assert entry["trace_id"] == "abc123"
            assert entry["span_name"] == "tool:read_file"
            assert entry["duration_ms"] == "250"
            assert 'metadata' in entry
    
    @pytest.mark.asyncio
    async def test_write_batch(self):
        """鎵归噺鍐欏叆搴斾娇鐢?Redis pipeline."""
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
            mock_pipeline.execute.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_get_instance_singleton(self):
        """TraceWriter 搴斾负鍗曚緥."""
        from app.trace.writer import TraceWriter
        
        w1 = await TraceWriter.get_instance()
        w2 = await TraceWriter.get_instance()
        assert w1 is w2
    
    @pytest.mark.asyncio
    async def test_record_span_convenience(self):
        """record_span 渚挎嵎鍑芥暟搴旇嚜鍔ㄥ垵濮嬪寲 TraceWriter."""
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
    """AgentEvent 鏂板 trace 瀛楁楠岃瘉."""
    
    def test_agent_event_has_trace_fields(self):
        """AgentEvent dataclass 搴斿寘鍚?trace_id/span_name/duration_ms."""
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
        """鏃т唬鐮佸垱寤?AgentEvent 涓嶅簲鍥犵己灏?trace 瀛楁鑰屽け璐?"""
        from app.agent.runtime import AgentEvent
        
        # 鏃у啓娉?(涓嶅惈 trace 瀛楁)
        event = AgentEvent(type="text", content="hello")
        
        assert event.type == "text"
        assert event.content == "hello"
        assert event.trace_id == ""  # default
        assert event.span_name == ""  # default
        assert event.duration_ms == 0  # default

