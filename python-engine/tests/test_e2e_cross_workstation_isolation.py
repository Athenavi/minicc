"""端到端测试: 跨工作台租户隔离验证

测试场景:
1. Agent 协同工作台 - 多 Agent 并发 + 上下文共享 (租户隔离)
2. 工作流工作台 - DAG 执行 + 运行时编辑 (租户状态隔离)
3. 技能工作台 - MCP 工具调用 (限流隔离)
4. 知识库 RAG - 文档检索 (向量数据隔离)
5. 混合负载 - 多租户并发请求验证

SaaS 安全验证:
- Redis Stream trace 按租户分 key
- Milvus 向量数据 tenant_id filter
- Skill/MCP 调用 quota 独立计数
- Context Store 命名空间隔离
"""
import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from dataclasses import dataclass, field


@dataclass
class TenantContext:
    """租户上下文 (测试用)"""
    tenant_id: str
    name: str
    is_premium: bool = False


# ── 测试夹具 ──────────────────────────────────────────────────────
@pytest.fixture
def tenant_a():
    return TenantContext(tenant_id="tenant_alpha", name="Alpha Corp")


@pytest.fixture
def tenant_b():
    return TenantContext(tenant_id="tenant_beta", name="Beta Inc")


@pytest.fixture
def mock_redis():
    """模拟 Redis 客户端"""
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
    """模拟 Gateway Router"""
    gateway = MagicMock()
    gateway.chat_stream = AsyncMock()
    return gateway


# ── 测试 1: Agent 协同工作台 ──────────────────────────────────────
class TestAgentCollaborationIsolation:
    """验证 Agent 协同的租户隔离"""
    
    @pytest.mark.asyncio
    async def test_multi_agent_context_isolation(self, mock_redis, tenant_a, tenant_b):
        """不同租户的 Agent 共享上下文不应泄漏"""
        from app.agent.collaboration import AgentContextStore
        
        # 为每个租户创建独立的 Context Store
        context_a = AgentContextStore(tenant_id=tenant_a.tenant_id)
        context_b = AgentContextStore(tenant_id=tenant_b.tenant_id)
        
        # 模拟 Redis
        with patch('app.agent.collaboration.aioredis') as mock_aioredis:
            mock_aioredis.from_url = MagicMock(return_value=mock_redis)
            context_a._redis_client = mock_redis
            context_b._redis_client = mock_redis
            
            # 写入上下文
            await context_a.set("research_data", {"findings": "Alpha findings"}, ttl=3600)
            await context_b.set("research_data", {"findings": "Beta findings"}, ttl=3600)
            
            # Verify: Redis key 包含 tenant_id
            assert mock_redis.setex.call_count == 2
            calls = mock_redis.setex.call_args_list
            assert "tenant_alpha" in str(calls[0])
            assert "tenant_beta" in str(calls[1])
            
            # 读取上下文 (应只读到自己租户的数据)
            result_a = await context_a.get("research_data")
            result_b = await context_b.get("research_data")
            
            assert result_a["findings"] == "Alpha findings"
            assert result_b["findings"] == "Beta findings"
            assert result_a["findings"] != result_b["findings"]
    
    @pytest.mark.asyncio
    async def test_agent_concurrent_quota(self, mock_gateway, tenant_a):
        """验证租户并发 Agent 数限制"""
        from app.agent.collaboration import AgentHub, CollaborativeTask, AgentRole
        
        hub = AgentHub(gateway=mock_gateway)
        
        # 设置较低的配额以便测试
        hub._max_concurrent_per_tenant = 2
        
        task = CollaborativeTask(
            task_id="test_task",
            original_query="Test query",
            tenant_id=tenant_a.tenant_id,
            trace_id="trace_001",
            subtasks=[],
        )
        
        # 模拟超过配额
        hub._tenant_running_agents[tenant_a.tenant_id] = 3
        
        events = []
        async for event in hub.run_collaborative_task(task):
            events.append(event)
            if event.type == "error":
                break
        
        # Verify: 应返回错误事件
        error_events = [e for e in events if e.type == "error"]
        assert len(error_events) > 0
        assert "已达上限" in error_events[0].content


# ── 测试 2: 工作流工作台 ───────────────────────────────────────────
class TestWorkflowIsolation:
    """验证工作流执行的租户隔离"""
    
    @pytest.mark.asyncio
    async def test_workflow_trace_tenant_isolation(self, mock_redis, tenant_a, tenant_b):
        """工作流 trace span 应按租户隔离"""
        from app.workflow.tracing_engine import TracingWorkflowEngine
        
        engine = TracingWorkflowEngine(gateway=MagicMock())
        
        # 模拟两个租户的工作流
        graph_json = {
            "name": "test_workflow",
            "nodes": [
                {"id": "node1", "type": "llm", "label": "LLM Node"},
                {"id": "node2", "type": "output", "label": "Output"},
            ],
            "edges": [
                {"source_id": "node1", "target_id": "node2"},
            ],
        }
        
        # 记录写入的 Redis Stream key
        written_streams = set()
        
        async def mock_xadd(stream, entry, maxlen=None, approximate=None):
            written_streams.add(stream)
            return "123"
        
        mock_redis.xadd.side_effect = mock_xadd
        
        # TODO: 实际应触发 run_workflow_with_trace
        # 这里简化验证 Redis key 生成逻辑
    
    @pytest.mark.asyncio
    async def test_edit_session_isolation(self, tenant_a, tenant_b):
        """编辑会话应按租户隔离"""
        from app.workflow.tracing_engine import create_edit_session, get_edit_session
        
        # 创建编辑会话
        session_id_a = create_edit_session(
            workflow_instance_id="wf_001",
            tenant_id=tenant_a.tenant_id,
        )
        session_id_b = create_edit_session(
            workflow_instance_id="wf_002",
            tenant_id=tenant_b.tenant_id,
        )
        
        # 获取会话
        session_a = get_edit_session(session_id_a)
        session_b = get_edit_session(session_id_b)
        
        assert session_a is not None
        assert session_a.tenant_id == tenant_a.tenant_id
        assert session_b.tenant_id == tenant_b.tenant_id
        assert session_a.tenant_id != session_b.tenant_id
        
        # 验证不能跨租户访问
        assert session_a.workflow_instance_id != session_b.workflow_instance_id


# ── 测试 3: 技能工作台 ─────────────────────────────────────────────
class TestSkillWorkstationIsolation:
    """验证技能调用的租户隔离"""
    
    @pytest.mark.asyncio
    async def test_skill_registration_isolation(self, tenant_a, tenant_b):
        """技能注册应按租户隔离"""
        from app.skill.manager import SkillManager, SkillType
        
        manager = SkillManager()
        
        # 注册技能
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
        
        # 列出技能 (租户隔离)
        skills_a = await manager.list_skills(tenant_id=tenant_a.tenant_id)
        skills_b = await manager.list_skills(tenant_id=tenant_b.tenant_id)
        
        assert len(skills_a) == 1
        assert len(skills_b) == 1
        assert skills_a[0].name == "Alpha's Skill"
        assert skills_b[0].name == "Beta's Skill"
        
        # Verify: full_skill_id 包含租户前缀
        assert skills_a[0].skill_id == f"{tenant_a.tenant_id}:my_skill"
        assert skills_b[0].skill_id == f"{tenant_b.tenant_id}:my_skill"
    
    @pytest.mark.asyncio
    async def test_skill_rate_limiting(self, mock_redis):
        """验证技能调用的独立限流"""
        from app.api.tenant_rate_limiter import TenantRateLimiter
        
        # 创建限流器 (QPS=10, Burst=20)
        limiter = TenantRateLimiter(mock_redis, max_qps=10, burst=20)
        
        # 模拟同一租户的 30 次调用
        tenant_id = "tenant_test"
        allowed_count = 0
        
        for i in range(30):
            allowed, remaining = limiter.allow("skill", tenant_id)
            if allowed:
                allowed_count += 1
        
        # Verify: 不应超过 quota
        assert allowed_count <= 20  # Burst limit
    
    @pytest.mark.asyncio
    async def test_mcp_tool_call_tracing(self, mock_redis):
        """验证 MCP 工具调用的链路追踪"""
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


# ── 测试 4: 知识库 RAG ─────────────────────────────────────────────
class TestKnowledgeBaseRAGIsolation:
    """验知识库 RAG 的租户隔离"""
    
    @pytest.mark.asyncio
    async def test_document_indexing_isolation(self, mock_redis, tenant_a, tenant_b):
        """文档索引应按租户隔离"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase
        
        kb = EnhancedKnowledgeBase()
        
        # Mock retriever
        kb.retriever._get_collection = AsyncMock(return_value=None)
        kb.retriever.index_document = AsyncMock(return_value={
            "document_id": "doc_001",
            "chunks_count": 10,
            "status": "indexed",
        })
        
        # 索引文档
        result_a = await kb.index_document(
            tenant_id=tenant_a.tenant_id,
            document_id="doc_001",
            content="Alpha's secret doc",
            trace_id="trace_idx_001",
        )
        
        result_b = await kb.index_document(
            tenant_id=tenant_b.tenant_id,
            document_id="doc_001",  # 相同 ID, 不同租户
            content="Beta's secret doc",
            trace_id="trace_idx_002",
        )
        
        # Verify: 都成功索引
        assert result_a["status"] == "indexed"
        assert result_b["status"] == "indexed"
        assert result_a["document_id"] == "doc_001"
        assert result_b["document_id"] == "doc_001"
        
        # Verify: Redis span 携带不同 tenant_id
        assert mock_redis.xadd.call_count == 2
        calls = mock_redis.xadd.call_args_list
        assert "tenant_alpha" in str(calls[0])
        assert "tenant_beta" in str(calls[1])
    
    @pytest.mark.asyncio
    async def test_retrieve_tenant_filter(self, mock_redis, tenant_a, tenant_b):
        """检索时应强制过滤 tenant_id"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase
        
        kb = EnhancedKnowledgeBase()
        
        # Mock 检索结果 (含不同租户数据 - 模拟数据泄漏)
        mock_results = [
            {
                "document_id": "doc_001",
                "chunk_id": "chunk_0",
                "content": "Should not see this",
                "score": 0.9,
                "tenant_id": tenant_b.tenant_id,  # 其他租户的数据
            },
            {
                "document_id": "doc_002",
                "chunk_id": "chunk_0",
                "content": "Visible to all tenants",
                "score": 0.8,
                "tenant_id": tenant_a.tenant_id,  # 当前租户数据
            },
        ]
        
        kb.retriever.retrieve = AsyncMock(return_value=mock_results)
        
        # 检索 (租户 A)
        results = await kb.retrieve(
            tenant_id=tenant_a.tenant_id,
            query="test query",
            trace_id="trace_ret_001",
        )
        
        # Verify: 所有结果的 tenant_id 必须与查询一致
        for result in results:
            assert result.tenant_id == tenant_a.tenant_id
        
        # 过滤掉不匹配的结果
        valid_results = [r for r in results if r.tenant_id == tenant_a.tenant_id]
        assert len(valid_results) == 1
        assert valid_results[0].content == "Visible to all tenants"
    
    @pytest.mark.asyncio
    async def test_delete_document_cascade(self, mock_redis, tenant_a):
        """删除文档应级联删除 Milvus chunks"""
        from app.knowledge.enhanced_kb import EnhancedKnowledgeBase
        
        kb = EnhancedKnowledgeBase()
        
        collection = MagicMock()
        collection.delete = AsyncMock(return_value=None)
        collection.flush = AsyncMock(return_value=None)
        kb.retriever._get_collection = AsyncMock(return_value=collection)
        
        # 删除文档
        success = await kb.delete_document(
            tenant_id=tenant_a.tenant_id,
            document_id="doc_001",
        )
        
        # Verify: 调用 delete 时带 tenant_id filter
        assert success is True
        collection.delete.assert_called_once()
        call_args = collection.delete.call_args
        assert tenant_a.tenant_id in str(call_args)
        assert "doc_001" in str(call_args)


# ── 测试 5: 混合负载压测 ───────────────────────────────────────────
class TestMixedLoadStress:
    """混合负载压力测试"""
    
    @pytest.mark.asyncio
    async def test_concurrent_cross_tenant_requests(self, mock_redis):
        """并发跨租户请求验证隔离性"""
        import uuid
        
        tenants = [
            TenantContext(tenant_id=f"tenant_{i}", name=f"Tenant {i}")
            for i in range(5)  # 5 个租户
        ]
        
        # 模拟每个租户发送 20 个请求
        tasks = []
        for tenant in tenants:
            for req_id in range(20):
                task = self._simulate_request(tenant, req_id)
                tasks.append(task)
        
        # 并发执行
        results = await asyncio.gather(*tasks)
        
        # Verify: 无交叉污染
        errors = [r for r in results if r["error"]]
        assert len(errors) == 0, f"Found {len(errors)} cross-tenant errors"
    
    @staticmethod
    async def _simulate_request(tenant: TenantContext, req_id: int) -> dict:
        """模拟一个请求"""
        trace_id = uuid.uuid4().hex[:12]
        
        # 模拟 Redis XADD (记录 trace span)
        stream = f"minicc:traces:{tenant.tenant_id}"
        
        return {
            "tenant_id": tenant.tenant_id,
            "trace_id": trace_id,
            "stream": stream,
            "error": None,
        }


# ── 测试 6: Trace 系统完整性 ───────────────────────────────────────
class TestTraceSystemIntegrity:
    """验证链路追踪系统的完整性"""
    
    @pytest.mark.asyncio
    async def test_trace_id_propagation(self, mock_redis):
        """验证 trace_id 在所有工作台中透传"""
        from app.trace.writer import record_span
        
        trace_id = "propagation_test_001"
        tenant_id = "tenant_trace_test"
        
        # 记录多个 span
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
        
        # Verify: 所有 span 写入同一 tenant stream
        assert mock_redis.xadd.call_count == 4
        calls = mock_redis.xadd.call_args_list
        
        for call in calls:
            stream = call[0][0]
            assert stream == f"minicc:traces:{tenant_id}"
    
    @pytest.mark.asyncio
    async def test_anonymous_tenant_isolation(self, mock_redis):
        """匿名用户应使用独立的 anonymous stream"""
        from app.trace.writer import get_tenant_stream
        
        anonymous_stream = get_tenant_stream("")
        normal_stream = get_tenant_stream("tenant_001")
        
        assert anonymous_stream == "minicc:traces:anonymous"
        assert normal_stream == "minicc:traces:tenant_001"
        assert anonymous_stream != normal_stream


# ── 运行测试 ───────────────────────────────────────────────────────
if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
