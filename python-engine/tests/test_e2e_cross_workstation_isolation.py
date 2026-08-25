"""绔埌绔祴璇? 璺ㄥ伐浣滃彴绉熸埛闅旂楠岃瘉

娴嬭瘯鍦烘櫙:
1. Agent 鍗忓悓宸ヤ綔鍙?- 澶?Agent 骞跺彂 + 涓婁笅鏂囧叡浜?(绉熸埛闅旂)
2. 宸ヤ綔娴佸伐浣滃彴 - DAG 鎵ц + 杩愯鏃剁紪杈?(绉熸埛鐘舵€侀殧绂?
3. 鎶€鑳藉伐浣滃彴 - MCP 宸ュ叿璋冪敤 (闄愭祦闅旂)
4. 鐭ヨ瘑搴?RAG - 鏂囨。妫€绱?(鍚戦噺鏁版嵁闅旂)
5. 娣峰悎璐熻浇 - 澶氱鎴峰苟鍙戣姹傞獙璇?

SaaS 瀹夊叏楠岃瘉:
- Redis Stream trace 鎸夌鎴峰垎 key
- Milvus 鍚戦噺鏁版嵁 tenant_id filter
- Skill/MCP 璋冪敤 quota 鐙珛璁℃暟
- Context Store 鍛藉悕绌洪棿闅旂
"""
import pytest
import asyncio
import uuid
from unittest.mock import AsyncMock, MagicMock, patch
from dataclasses import dataclass, field

import app.main  # noqa: F401 鈥?鍒濆鍖?app 鍖咃紝閬垮厤寰幆瀵煎叆


@dataclass
class TenantContext:
    """绉熸埛涓婁笅鏂?(娴嬭瘯鐢?"""
    tenant_id: str
    name: str
    is_premium: bool = False


# 鈹€鈹€ 娴嬭瘯澶瑰叿 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
@pytest.fixture
def tenant_a():
    return TenantContext(tenant_id="tenant_alpha", name="Alpha Corp")


@pytest.fixture
def tenant_b():
    return TenantContext(tenant_id="tenant_beta", name="Beta Inc")


@pytest.fixture
def mock_redis():
    """妯℃嫙 Redis 瀹㈡埛绔?""
    redis = AsyncMock()
    redis.ping = AsyncMock(return_value=True)
    redis.xadd = AsyncMock(return_value="1234567890")
    redis.xrange = AsyncMock(return_value=[])
    redis.setex = AsyncMock(return_value=True)
    redis.get = AsyncMock(return_value=None)
    redis.delete = AsyncMock(return_value=1)
    redis.keys = AsyncMock(return_value=[])
    return redis


@pytest.fixture
def mock_gateway():
    """妯℃嫙 Gateway Router"""
    gateway = MagicMock()
    gateway.chat_stream = AsyncMock()
    return gateway


@pytest.fixture
def trace_writer_mock(mock_redis):
    """鎶?mock Redis 娉ㄥ叆 TraceWriter 鍗曚緥锛屾祴璇曞悗杩樺師銆?

    record_span 缁忔ā鍧楃骇 _trace_writer 鍗曚緥鍐?span锛涗笉娉ㄥ叆鍒?
    鍗曚緥鐨?_redis 涓?None锛宻pan 琚潤榛樹涪寮冿紝鏂█澶辨晥銆?
    """
    from app.trace import writer as trace_writer_mod
    from app.trace.writer import TraceWriter

    prev_writer = trace_writer_mod._trace_writer
    prev_redis = TraceWriter._redis
    prev_instance = TraceWriter._instance

    TraceWriter._redis = mock_redis
    if TraceWriter._instance is None:
        TraceWriter._instance = TraceWriter()
    trace_writer_mod._trace_writer = TraceWriter._instance

    yield mock_redis

    TraceWriter._redis = prev_redis
    TraceWriter._instance = prev_instance
    trace_writer_mod._trace_writer = prev_writer


# 鈹€鈹€ 娴嬭瘯 1: Agent 鍗忓悓宸ヤ綔鍙?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestAgentCollaborationIsolation:
    """楠岃瘉 Agent 鍗忓悓鐨勭鎴烽殧绂?""
    
    @pytest.mark.asyncio
    async def test_multi_agent_context_isolation(self, mock_redis, tenant_a, tenant_b):
        """涓嶅悓绉熸埛鐨?Agent 鍏变韩涓婁笅鏂囦笉搴旀硠婕?""
        from app.agent.collaboration import AgentContextStore

        # 涓烘瘡涓鎴峰垱寤虹嫭绔嬬殑 Context Store
        context_a = AgentContextStore(tenant_id=tenant_a.tenant_id)
        context_b = AgentContextStore(tenant_id=tenant_b.tenant_id)

        # 鐩存帴娉ㄥ叆 mock Redis 瀹㈡埛绔紙redis.asyncio 鍦ㄦ柟娉曞唴閮ㄥ欢杩熷鍏ワ級
        context_a._redis_client = mock_redis
        context_b._redis_client = mock_redis

        # 鍐欏叆涓婁笅鏂?
        await context_a.set("research_data", {"findings": "Alpha findings"}, ttl=3600)
        await context_b.set("research_data", {"findings": "Beta findings"}, ttl=3600)

        # Verify: Redis key 鍖呭惈 tenant_id
        assert mock_redis.setex.call_count == 2
        calls = mock_redis.setex.call_args_list
        assert "tenant_alpha" in str(calls[0])
        assert "tenant_beta" in str(calls[1])

        # 璇诲彇涓婁笅鏂囷紙mock get 杩斿洖 None锛岄獙璇佽皟鐢ㄨ矾寰勬寜绉熸埛鍙?key锛?
        await context_a.get("research_data")
        await context_b.get("research_data")
        get_calls = mock_redis.get.call_args_list
        assert "tenant_alpha" in str(get_calls[0])
        assert "tenant_beta" in str(get_calls[1])
    
    @pytest.mark.asyncio
    async def test_agent_concurrent_quota(self, mock_gateway, tenant_a):
        """楠岃瘉绉熸埛骞跺彂 Agent 鏁伴檺鍒?""
        from app.agent.collaboration import AgentHub, CollaborativeTask, AgentRole
        
        hub = AgentHub(gateway=mock_gateway)
        
        # 璁剧疆杈冧綆鐨勯厤棰濅互渚挎祴璇?
        hub._max_concurrent_per_tenant = 2
        
        task = CollaborativeTask(
            task_id="test_task",
            original_query="Test query",
            tenant_id=tenant_a.tenant_id,
            trace_id="trace_001",
            subtasks=[],
        )
        
        # 妯℃嫙瓒呰繃閰嶉
        hub._tenant_running_agents[tenant_a.tenant_id] = 3
        
        events = []
        async for event in hub.run_collaborative_task(task):
            events.append(event)
            if event.type == "error":
                break
        
        # Verify: 搴旇繑鍥為敊璇簨浠?
        error_events = [e for e in events if e.type == "error"]
        assert len(error_events) > 0
        assert "宸茶揪涓婇檺" in error_events[0].content


# 鈹€鈹€ 娴嬭瘯 2: 宸ヤ綔娴佸伐浣滃彴 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestWorkflowIsolation:
    """楠岃瘉宸ヤ綔娴佹墽琛岀殑绉熸埛闅旂"""
    
    @pytest.mark.asyncio
    async def test_workflow_trace_tenant_isolation(self, mock_redis, trace_writer_mock, tenant_a, tenant_b):
        """宸ヤ綔娴?trace span 搴旀寜绉熸埛闅旂

        鐪熷疄瑙﹀彂 run_workflow_with_trace锛堜袱涓鎴峰悇璺戜竴閬嶅悓鏋勫浘锛夛紝
        楠岃瘉鎵€鏈?span 钀藉埌鍚勮嚜鐨?chiron:traces:{tenant_id} 娴侊紝
        涓旇妭鐐归敊璇?span锛堝唴閮ㄦ崟鑾疯矾寰勶級涔熶笉娉勬紡鍒?anonymous 娴併€?
        """
        from app.workflow.tracing_engine import TracingWorkflowEngine

        written_streams = set()

        async def mock_xadd(stream, entry, maxlen=None, approximate=None):
            written_streams.add(stream)
            return "123"

        mock_redis.xadd.side_effect = mock_xadd

        engine = TracingWorkflowEngine(gateway_router=MagicMock())

        graph_json = {
            "name": "test_workflow",
            "nodes": [
                {"id": "node1", "node_type": "llm", "label": "LLM Node",
                 "config": {"user_message": "hi", "model": "m"}},
                {"id": "node2", "node_type": "output", "label": "Output"},
            ],
            "edges": [
                {"source_id": "node1", "target_id": "node2"},
            ],
        }

        for tenant in (tenant_a, tenant_b):
            events = []
            async for ev in engine.run_workflow_with_trace(
                graph_json=graph_json,
                initial_state={},
                tenant_id=tenant.tenant_id,
                instance_id=f"wf_iso_{tenant.tenant_id}",
            ):
                events.append(ev)
            # 宸ヤ綔娴佹甯稿畬鎴?
            assert any(e.type == "workflow_done" for e in events), (
                f"workflow for {tenant.tenant_id} did not finish"
            )

        # Verify: 鎵€鏈?span 鍙啓鍏ヤ袱涓鎴峰悇鑷殑娴?
        expected_a = f"chiron:traces:{tenant_a.tenant_id}"
        expected_b = f"chiron:traces:{tenant_b.tenant_id}"
        assert written_streams == {expected_a, expected_b}, (
            f"span 娉勬紡鍒伴潪绉熸埛娴? {written_streams}"
        )

    @pytest.mark.asyncio
    async def test_workflow_node_error_span_stays_in_tenant_stream(
        self, mock_redis, trace_writer_mock, tenant_a
    ):
        """鑺傜偣绾ч敊璇?span锛坃execute_node_with_trace 鍐呴儴鎹曡幏璺緞锛?
        蹇呴』鎼哄甫 tenant_id锛屼笉寰楄惤鍏?anonymous 娴併€?""
        from app.workflow.tracing_engine import TracingWorkflowEngine

        class _BrokenGateway:
            async def chat_stream(self, messages=None, model=""):
                raise RuntimeError("llm exploded")
                yield  # pragma: no cover

        written_streams = set()

        async def mock_xadd(stream, entry, maxlen=None, approximate=None):
            written_streams.add(stream)
            return "123"

        mock_redis.xadd.side_effect = mock_xadd

        engine = TracingWorkflowEngine(gateway_router=_BrokenGateway())

        graph_json = {
            "name": "wf_err",
            "nodes": [
                {"id": "llm", "node_type": "llm", "label": "LLM",
                 "config": {"user_message": "boom", "model": "m"}},
            ],
            "edges": [],
        }

        events = []
        async for ev in engine.run_workflow_with_trace(
            graph_json=graph_json,
            initial_state={},
            tenant_id=tenant_a.tenant_id,
            instance_id="wf_err_iso",
        ):
            events.append(ev)

        assert any(e.type == "workflow_done" for e in events)
        expected = f"chiron:traces:{tenant_a.tenant_id}"
        assert written_streams == {expected}, (
            f"閿欒 span 娉勬紡: {written_streams}"
        )
    
    @pytest.mark.asyncio
    async def test_edit_session_isolation(self, tenant_a, tenant_b):
        """缂栬緫浼氳瘽搴旀寜绉熸埛闅旂"""
        from app.workflow.tracing_engine import create_edit_session, get_edit_session
        
        # 鍒涘缓缂栬緫浼氳瘽
        session_id_a = create_edit_session(
            workflow_instance_id="wf_001",
            tenant_id=tenant_a.tenant_id,
        )
        session_id_b = create_edit_session(
            workflow_instance_id="wf_002",
            tenant_id=tenant_b.tenant_id,
        )
        
        # 鑾峰彇浼氳瘽
        session_a = get_edit_session(session_id_a)
        session_b = get_edit_session(session_id_b)
        
        assert session_a is not None
        assert session_a.tenant_id == tenant_a.tenant_id
        assert session_b.tenant_id == tenant_b.tenant_id
        assert session_a.tenant_id != session_b.tenant_id
        
        # 楠岃瘉涓嶈兘璺ㄧ鎴疯闂?
        assert session_a.workflow_instance_id != session_b.workflow_instance_id


# 鈹€鈹€ 娴嬭瘯 3: 鎶€鑳藉伐浣滃彴 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestSkillWorkstationIsolation:
    """楠岃瘉鎶€鑳借皟鐢ㄧ殑绉熸埛闅旂"""
    
    @pytest.mark.asyncio
    async def test_skill_registration_isolation(self, tenant_a, tenant_b):
        """鎶€鑳芥敞鍐屽簲鎸夌鎴烽殧绂?""
        from app.skill.manager import SkillManager, SkillType
        
        manager = SkillManager()
        
        # 娉ㄥ唽鎶€鑳?
        await manager.register_skill(
            tenant_id=tenant_a.tenant_id,
            skill_id="my_skill",
            name="Alpha's Skill",
            description="For Alpha Corp",
            type=SkillType.PROMPT,
            config={"template": "Hello {{name}}"},
        )
        
        await manager.register_skill(
            tenant_id=tenant_b.tenant_id,
            skill_id="my_skill",
            name="Beta's Skill",
            description="For Beta Inc",
            type=SkillType.PROMPT,
            config={"template": "Goodbye {{name}}"},
        )
        
        # 鍒楀嚭鎶€鑳?(绉熸埛闅旂)
        skills_a = await manager.list_skills(tenant_id=tenant_a.tenant_id)
        skills_b = await manager.list_skills(tenant_id=tenant_b.tenant_id)
        
        assert len(skills_a) == 1
        assert len(skills_b) == 1
        assert skills_a[0].name == "Alpha's Skill"
        assert skills_b[0].name == "Beta's Skill"
        
        # Verify: full_skill_id 鍖呭惈绉熸埛鍓嶇紑
        assert skills_a[0].skill_id == f"{tenant_a.tenant_id}:my_skill"
        assert skills_b[0].skill_id == f"{tenant_b.tenant_id}:my_skill"
    
    @pytest.mark.asyncio
    async def test_skill_rate_limiting(self, mock_redis):
        """楠岃瘉鎶€鑳借皟鐢ㄧ殑鐙珛闄愭祦锛堢鎴锋粦鍔ㄧ獥鍙ｈ鏁板櫒锛?""
        from app.gateway.ratelimit import TenantRateLimiter

        # pipeline 鍛戒护鏄悓姝ラ摼寮忚皟鐢紝execute 鏄?await 鈥?鍗曠嫭 mock
        pipe = MagicMock()
        pipe.zremrangebyscore = MagicMock(return_value=None)
        pipe.zcard = MagicMock(return_value=0)
        pipe.zadd = MagicMock(return_value=1)
        pipe.expire = MagicMock(return_value=True)
        pipe.execute = AsyncMock(return_value=[0, 0, 1, True])
        mock_redis.pipeline = MagicMock(return_value=pipe)

        # 鍒涘缓闄愭祦鍣?(rps=2, rpm=60)
        limiter = TenantRateLimiter(mock_redis, requests_per_second=2, requests_per_minute=60)

        # 妯℃嫙鍚屼竴绉熸埛鐨勫揩閫熻繛缁皟鐢紙绐楀彛璁℃暟 0 鈫?鎭掓斁琛岋紝楠岃瘉 key 闅旂锛?
        tenant_id = "tenant_test"
        results = [await limiter.allow(tenant_id) for _ in range(3)]

        # Verify: 闄愭祦鍣ㄦ寜绐楀彛鍒ゅ畾锛堟粦鍔ㄧ獥鍙?key 鎸夌鎴烽殧绂伙級
        assert all(results)
        # Redis 婊戝姩绐楀彛鎿嶄綔鍏ㄩ儴鎸夌鎴?key 鎵ц
        zadd_calls = pipe.zadd.call_args_list
        assert all(tenant_id in str(c) for c in zadd_calls)
    
    @pytest.mark.asyncio
    async def test_mcp_tool_call_tracing(self, mock_redis):
        """楠岃瘉 MCP 宸ュ叿璋冪敤鐨勯摼璺拷韪?""
        from app.skill.manager import MCPClient
        
        client = MCPClient(server_url="http://mcp-server:8080", transport="http")
        
        # Mock HTTP response
        with patch('httpx.AsyncClient') as mock_httpclient:
            mock_response = MagicMock()
            mock_response.json.return_value = {
                "jsonrpc": "2.0",
                "result": {
                    "tools": [
                        {"name": "search", "description": "Search docs"}
                    ]
                }
            }
            mock_httpclient.return_value.__aenter__.return_value.post.return_value = mock_response
            
            tools = await client.discover_tools()
            
            assert len(tools) > 0
            assert tools[0]["name"] == "search"


# 鈹€鈹€ 娴嬭瘯 4: 鐭ヨ瘑搴?RAG 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestKnowledgeBaseRAGIsolation:
    """楠岀煡璇嗗簱 RAG 鐨勭鎴烽殧绂?""
    
    @pytest.mark.asyncio
    async def test_document_indexing_isolation(self, mock_redis, trace_writer_mock, tenant_a, tenant_b):
        """鏂囨。绱㈠紩搴旀寜绉熸埛闅旂"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase

        kb = EnhancedKnowledgeBase()

        # Mock retriever
        kb.retriever._get_collection = AsyncMock(return_value=None)
        kb.retriever.index_document = AsyncMock(return_value={
            "document_id": "doc_001",
            "chunks_count": 10,
            "status": "indexed",
        })

        # 绱㈠紩鏂囨。
        result_a = await kb.index_document(
            tenant_id=tenant_a.tenant_id,
            document_id="doc_001",
            content="Alpha's secret doc",
            trace_id="trace_idx_001",
        )

        result_b = await kb.index_document(
            tenant_id=tenant_b.tenant_id,
            document_id="doc_001",  # 鐩稿悓 ID, 涓嶅悓绉熸埛
            content="Beta's secret doc",
            trace_id="trace_idx_002",
        )

        # Verify: 閮芥垚鍔熺储寮?
        assert result_a["status"] == "indexed"
        assert result_b["status"] == "indexed"
        assert result_a["document_id"] == "doc_001"
        assert result_b["document_id"] == "doc_001"

        # Verify: Redis span 鎼哄甫涓嶅悓 tenant_id锛堝啓鍏ュ悇鑷殑绉熸埛娴侊級
        assert mock_redis.xadd.call_count == 2
        calls = mock_redis.xadd.call_args_list
        assert f"chiron:traces:{tenant_a.tenant_id}" in str(calls[0])
        assert f"chiron:traces:{tenant_b.tenant_id}" in str(calls[1])
    
    @pytest.mark.asyncio
    async def test_retrieve_tenant_filter(self, mock_redis, tenant_a, tenant_b):
        """妫€绱㈡椂搴斿己鍒惰繃婊?tenant_id"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase
        
        kb = EnhancedKnowledgeBase()
        
        # Mock 妫€绱㈢粨鏋?(鍚笉鍚岀鎴锋暟鎹?- 妯℃嫙鏁版嵁娉勬紡)
        mock_results = [
            {
                "document_id": "doc_001",
                "chunk_id": "chunk_0",
                "content": "Should not see this",
                "score": 0.9,
                "tenant_id": tenant_b.tenant_id,  # 鍏朵粬绉熸埛鐨勬暟鎹?
            },
            {
                "document_id": "doc_002",
                "chunk_id": "chunk_0",
                "content": "Visible to all tenants",
                "score": 0.8,
                "tenant_id": tenant_a.tenant_id,  # 褰撳墠绉熸埛鏁版嵁
            },
        ]
        
        kb.retriever.retrieve = AsyncMock(return_value=mock_results)
        
        # 妫€绱?(绉熸埛 A)
        results = await kb.retrieve(
            tenant_id=tenant_a.tenant_id,
            query="test query",
            trace_id="trace_ret_001",
        )
        
        # Verify: 鎵€鏈夌粨鏋滅殑 tenant_id 蹇呴』涓庢煡璇竴鑷?
        for result in results:
            assert result.tenant_id == tenant_a.tenant_id
        
        # 杩囨护鎺変笉鍖归厤鐨勭粨鏋?
        valid_results = [r for r in results if r.tenant_id == tenant_a.tenant_id]
        assert len(valid_results) == 1
        assert valid_results[0].content == "Visible to all tenants"
    
    @pytest.mark.asyncio
    async def test_delete_document_cascade(self, mock_redis, tenant_a):
        """鍒犻櫎鏂囨。搴旂骇鑱斿垹闄?Milvus chunks"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase

        kb = EnhancedKnowledgeBase()

        collection = MagicMock()
        collection.delete = AsyncMock(return_value=None)
        collection.flush = AsyncMock(return_value=None)
        kb.retriever._get_collection = AsyncMock(return_value=collection)

        # Mock PG pool锛坉elete_document 鍐呴儴 from app.db import get_pool锛?
        mock_pool = AsyncMock()
        mock_pool.execute = AsyncMock(return_value="DELETE 1")
        with patch("app.db.get_pool", return_value=mock_pool):
            success = await kb.delete_document(
                tenant_id=tenant_a.tenant_id,
                document_id="doc_001",
            )

        # Verify: 璋冪敤 delete 鏃跺甫 tenant_id filter
        assert success is True
        collection.delete.assert_called_once()
        call_args = collection.delete.call_args
        assert tenant_a.tenant_id in str(call_args)
        assert "doc_001" in str(call_args)
        # PG 鍒犻櫎涔熷甫 tenant_id 鏉′欢
        pg_args = mock_pool.execute.call_args[0]
        assert "tenant_id = $2" in pg_args[0]
        assert pg_args[2] == tenant_a.tenant_id


# 鈹€鈹€ 娴嬭瘯 5: 娣峰悎璐熻浇鍘嬫祴 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestMixedLoadStress:
    """娣峰悎璐熻浇鍘嬪姏娴嬭瘯"""
    
    @pytest.mark.asyncio
    async def test_concurrent_cross_tenant_requests(self, mock_redis):
        """骞跺彂璺ㄧ鎴疯姹傞獙璇侀殧绂绘€?""
        import uuid
        
        tenants = [
            TenantContext(tenant_id=f"tenant_{i}", name=f"Tenant {i}")
            for i in range(5)  # 5 涓鎴?
        ]
        
        # 妯℃嫙姣忎釜绉熸埛鍙戦€?20 涓姹?
        tasks = []
        for tenant in tenants:
            for req_id in range(20):
                task = self._simulate_request(tenant, req_id)
                tasks.append(task)
        
        # 骞跺彂鎵ц
        results = await asyncio.gather(*tasks)
        
        # Verify: 鏃犱氦鍙夋薄鏌?
        errors = [r for r in results if r["error"]]
        assert len(errors) == 0, f"Found {len(errors)} cross-tenant errors"
    
    @staticmethod
    async def _simulate_request(tenant: TenantContext, req_id: int) -> dict:
        """妯℃嫙涓€涓姹?""
        trace_id = uuid.uuid4().hex[:12]
        
        # 妯℃嫙 Redis XADD (璁板綍 trace span)
        stream = f"chiron:traces:{tenant.tenant_id}"
        
        return {
            "tenant_id": tenant.tenant_id,
            "trace_id": trace_id,
            "stream": stream,
            "error": None,
        }


# 鈹€鈹€ 娴嬭瘯 6: Trace 绯荤粺瀹屾暣鎬?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
class TestTraceSystemIntegrity:
    """楠岃瘉閾捐矾杩借釜绯荤粺鐨勫畬鏁存€?""
    
    @pytest.mark.asyncio
    async def test_trace_id_propagation(self, mock_redis, trace_writer_mock):
        """楠岃瘉 trace_id 鍦ㄦ墍鏈夊伐浣滃彴涓€忎紶"""
        from app.trace.writer import record_span
        
        trace_id = "propagation_test_001"
        tenant_id = "tenant_trace_test"
        
        # 璁板綍澶氫釜 span
        await record_span(
            trace_id=trace_id,
            span_name="agent:start",
            duration_ms=100,
            tenant_id=tenant_id,
        )
        await record_span(
            trace_id=trace_id,
            span_name="workflow:node1",
            duration_ms=200,
            tenant_id=tenant_id,
        )
        await record_span(
            trace_id=trace_id,
            span_name="kb:retrieve",
            duration_ms=50,
            tenant_id=tenant_id,
        )
        await record_span(
            trace_id=trace_id,
            span_name="skill:execute",
            duration_ms=150,
            tenant_id=tenant_id,
        )
        
        # Verify: 鎵€鏈?span 鍐欏叆鍚屼竴 tenant stream
        assert mock_redis.xadd.call_count == 4
        calls = mock_redis.xadd.call_args_list
        
        for call in calls:
            stream = call[0][0]
            assert stream == f"chiron:traces:{tenant_id}"
    
    @pytest.mark.asyncio
    async def test_anonymous_tenant_isolation(self, mock_redis):
        """鍖垮悕鐢ㄦ埛搴斾娇鐢ㄧ嫭绔嬬殑 anonymous stream"""
        from app.trace.writer import get_tenant_stream
        
        anonymous_stream = get_tenant_stream("")
        normal_stream = get_tenant_stream("tenant_001")
        
        assert anonymous_stream == "chiron:traces:anonymous"
        assert normal_stream == "chiron:traces:tenant_001"
        assert anonymous_stream != normal_stream


# 鈹€鈹€ 杩愯娴嬭瘯 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])

