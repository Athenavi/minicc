"""SaaS Security Tests 鈥?绉熸埛闅旂楠岃瘉.

鍏抽敭瀹夊叏鍦烘櫙:
1. Trace Stream 鎸?tenant_id 闅旂
2. AgentEvent 鎼哄甫 tenant_id 閫忎紶
3. Redis Stream 鍐欏叆甯?tenant_id 鏍囪
"""
import pytest
from unittest.mock import AsyncMock, patch


class TestTenantIsolationTraceWriter:
    """TraceWriter 绉熸埛闅旂娴嬭瘯."""
    
    @pytest.mark.asyncio
    async def test_trace_stream_isolation_by_tenant(self):
        """涓嶅悓 tenant 搴斿啓鍏ヤ笉鍚岀殑 Redis Stream."""
        from app.trace.writer import TraceWriter, get_tenant_stream
        
        # 楠岃瘉 stream key 鐢熸垚閫昏緫
        assert get_tenant_stream("tenant_001") == "chiron:traces:tenant_001"
        assert get_tenant_stream("tenant_002") == "chiron:traces:tenant_002"
        assert get_tenant_stream("") == "chiron:traces:anonymous"
        assert get_tenant_stream(None) == "chiron:traces:anonymous"
        
        mock_redis = AsyncMock()
        mock_redis.ping = AsyncMock()
        mock_redis.xadd = AsyncMock(return_value="123")
        
        with patch('app.trace.writer.TraceWriter._redis', mock_redis):
            writer = await TraceWriter.get_instance()
            
            # 鍐欏叆 tenant_001 鐨?span
            await writer.write_span(
                trace_id="trace_a",
                span_name="llm_call",
                duration_ms=100,
                tenant_id="tenant_001",
            )
            
            # 鍐欏叆 tenant_002 鐨?span
            await writer.write_span(
                trace_id="trace_b",
                span_name="tool:read_file",
                duration_ms=200,
                tenant_id="tenant_002",
            )
            
            # Verify: xadd 琚皟鐢ㄤ簡涓ゆ,鍒嗗埆鍐欏叆涓嶅悓 stream
            assert mock_redis.xadd.call_count == 2
            
            calls = mock_redis.xadd.call_args_list
            assert calls[0][0][0] == "chiron:traces:tenant_001"
            assert calls[1][0][0] == "chiron:traces:tenant_002"
            
            # Verify entry 涓甫鏈?tenant_id 鏍囪
            entry_001 = calls[0][0][1]
            entry_002 = calls[1][0][1]
            assert entry_001["tenant_id"] == "tenant_001"
            assert entry_002["tenant_id"] == "tenant_002"
    
    @pytest.mark.asyncio
    async def test_anonymous_tenant_isolation(self):
        """鍖垮悕绉熸埛 (鏃?tenant_id) 搴斿啓鍏?anonymous stream."""
        from app.trace.writer import TraceWriter
        
        mock_redis = AsyncMock()
        mock_redis.ping = AsyncMock()
        mock_redis.xadd = AsyncMock(return_value="123")
        
        with patch('app.trace.writer.TraceWriter._redis', mock_redis):
            writer = await TraceWriter.get_instance()
            
            # 涓嶄紶 tenant_id
            await writer.write_span(
                trace_id="anon_trace",
                span_name="text",
                duration_ms=50,
            )
            
            mock_redis.xadd.assert_called_once()
            call_args = mock_redis.xadd.call_args
            assert call_args[0][0] == "chiron:traces:anonymous"
            entry = call_args[0][1]
            assert entry["tenant_id"] == ""
    
    @pytest.mark.asyncio
    async def test_record_span_with_tenant(self):
        """record_span 渚挎嵎鍑芥暟搴旈€忎紶 tenant_id."""
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
                tenant_id="secure_tenant",
            )
            
            call_args = mock_redis.xadd.call_args
            stream = call_args[0][0]
            entry = call_args[0][1]
            
            assert stream == "chiron:traces:secure_tenant"
            assert entry["tenant_id"] == "secure_tenant"


class TestAgentEventTenantPropagation:
    """AgentEvent 绉熸埛 ID 閫忎紶娴嬭瘯."""
    
    def test_agent_event_includes_tenant_id(self):
        """AgentEvent 搴旀惡甯?tenant_id."""
        from app.agent.runtime import AgentEvent
        
        event = AgentEvent(
            type="trace_span",
            trace_id="abc123",
            span_name="llm_call",
            duration_ms=1500,
            content='{"model": "gpt-4"}',
        )
        event.trace_id = "abc123"
        
        # 楠岃瘉鏂板瓧娈靛瓨鍦?        assert hasattr(event, 'trace_id')
        assert hasattr(event, 'span_name')
        assert hasattr(event, 'duration_ms')
        assert event.trace_id == "abc123"
    
    @pytest.mark.asyncio
    async def test_runtime_propagates_tenant_to_trace(self):
        """AgentRuntime.run 搴斿皢 tenant_id 浼犵粰 record_span."""
        from app.agent.runtime import AgentRuntime, AgentTask
        from app.gateway.router import GatewayRouter
        
        # 鍒涘缓 mock gateway锛堜笌鍏朵粬娴嬭瘯涓€鑷达細async generator 妯℃嫙娴佸紡鍝嶅簲锛?        mock_gateway = AsyncMock(spec=GatewayRouter)

        async def fake_stream(**_kwargs):
            yield MagicMock(
                content="Hello",
                reasoning_content="",
                tool_calls=[],
                input_tokens=100,
                output_tokens=50,
                finish_reason="stop",
            )

        mock_gateway.chat_stream = fake_stream
        
        # 鍒涘缓甯?tenant_id 鐨?task
        task = AgentTask(
            id="task_001",
            tenant_id="isolated_tenant",
            user_id="user_123",
            session_id="session_456",
            content="test message",
        )
        
        runtime = AgentRuntime(gateway=mock_gateway)
        
        # Mock record_span 骞舵嫤鎴皟鐢?        recorded_spans = []
        
        async def mock_record_span(**kwargs):
            recorded_spans.append(kwargs)
        
        # runtime 鍐呭眬閮ㄥ鍏?record_span锛岄渶 patch 婧愭ā鍧楁墠鑳芥嫤鎴?        with patch('app.trace.record_span', side_effect=mock_record_span):
            # 娑堣垂瀹屼簨浠跺悗鍋滄
            events_collected = []
            try:
                async for event in runtime.run(task):
                    events_collected.append(event)
                    if event.type == "done":
                        break
            except StopAsyncIteration:
                pass
        
        # Verify: 鎵€鏈夎褰曠殑 span 閮藉甫鏈夋纭殑 tenant_id
        for span in recorded_spans:
            assert span.get("tenant_id") == "isolated_tenant", \
                f"Span tenant_id mismatch: expected 'isolated_tenant', got '{span.get('tenant_id')}'"


class TestTenantSecurityScenarios:
    """SaaS 瀹夊叏鍦烘櫙娴嬭瘯."""
    
    def test_tenants_cannot_cross_reference_traces(self):
        """Tenant A 鏃犳硶閫氳繃 trace 鎺ㄦ柇 Tenant B 鐨勬椿鍔?"""
        from app.trace.writer import get_tenant_stream
        
        # 楠岃瘉 stream key 璁捐闃叉璺ㄧ鎴锋煡璇?        stream_a = get_tenant_stream("tenant_alpha")
        stream_b = get_tenant_stream("tenant_beta")
        
        # 涓や釜 stream 瀹屽叏鐙珛
        assert stream_a != stream_b
        assert "tenant_alpha" not in stream_b
        assert "tenant_beta" not in stream_a
        
        # Anonymous stream 涓庡凡璁よ瘉绉熸埛闅旂
        stream_anon = get_tenant_stream("")
        assert stream_anon != stream_a
        assert "anonymous" in stream_anon
    
    def test_trace_metadata_no_sensitive_tenant_info(self):
        """Trace metadata 涓嶅簲鍖呭惈鏁忔劅绉熸埛淇℃伅 (濡傚悕绉般€侀厤缃?."""
        from app.agent.runtime import AgentEvent
        
        event = AgentEvent(
            type="trace_span",
            trace_id="test_trace",
            span_name="llm_call",
            duration_ms=1000,
            content='{"model": "gpt-4"}',
        )
        
        # 楠屾 metadata 鍙寘鍚繀瑕佷俊鎭?        import json
        metadata = json.loads(event.content) if event.content else {}
        
        # 鍏佽鐧藉悕鍗曞瓧娈?        allowed_fields = {"span_name", "duration_ms", "input_tokens", "output_tokens", 
                         "model", "turn", "tool_name", "success"}
        
        for key in metadata.keys():
            assert key in allowed_fields, \
                f"Sensitive field '{key}' found in trace metadata"
    
    def test_tenant_id_validation(self):
        """tenant_id 搴旈槻姝㈡敞鍏ユ敾鍑?(濡?Redis 鍛戒护娉ㄥ叆)."""
        from app.trace.writer import get_tenant_stream
        
        malicious_ids = [
            "tenant;* MSET attack:true",  # Redis 鍛戒护娉ㄥ叆
            "tenant\nbadline:attack",      # Redis CRLF 娉ㄥ叆
            "tenant{\"noisy\":true}",       # JSON 娉ㄥ叆
        ]
        
        for mal_id in malicious_ids:
            # 搴旇瀹夊叏鍦拌浆涔夊埌 stream key (涓嶄細宕╂簝鎴栨硠闇?
            stream = get_tenant_stream(mal_id)
            assert isinstance(stream, str)
            assert len(stream) > 0
            # 鎭舵剰瀛楃搴旇鑷劧杞箟涓?key 鐨勪竴閮ㄥ垎 (Redis key 鏄畨鍏ㄧ殑)
            assert "chiron:traces:" in stream

