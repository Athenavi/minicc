# 记忆系统四层架构实施任务清单

# PR-1：基础层

- [✓] Task 1: 创建数据模型和类型定义
    - 1.1: 创建 `python-engine/app/memory/layers.py`，定义 SessionMeta、Scope、RecallResult 等核心 dataclass
    - 1.2: 定义 MemoryRef 类型（profile 或 summary）、MemoryConflict 事件结构
    - 1.3: 定义 ProfileUpdateResult、ConflictRef 等返回类型
    - 1.4: 添加完整的类型注解和文档字符串

- [✓] Task 2: 数据库迁移 - L2 用户档案表
    - 2.1: 创建 `migrations/20260823000006_user_memory_profile.up.sql`，定义 user_memory_profile 表
    - 2.2: 添加必要索引（PRIMARY KEY、idx_ump_reference）
    - 2.3: 创建 `migrations/20260823000006_user_memory_profile.down.sql`，定义回滚逻辑
    - 2.4: 在 `python-engine/app/db.py` 的 ensure_tables 中同步补建表结构（双轨约定）

- [✓] Task 3: 实现 L2 ProfileCard provider
    - 3.1: 创建 `python-engine/app/memory/profile_card.py`，实现 ProfileCard 类
    - 3.2: 实现 get_profile 方法（整卡读取 + Redis 缓存，TTL 60s）
    - 3.3: 实现 upsert_item 方法（槽位级 upsert，处理冲突规则）
    - 3.4: 实现 delete_item 方法（硬删 + 抑制名单处理）
    - 3.5: 实现 archive_low_confidence 方法（180天未引用且低置信度归档）
    - 3.6: 实现 evict_over_limit 方法（200条目软限淘汰）

- [✓] Task 4: 实现 L1 SessionMeta store
    - 4.1: 在 `layers.py` 中实现 SessionMetaStore 类
    - 4.2: 实现 create 方法（进程内存 dict 存储）
    - 4.3: 实现 get/update 方法
    - 4.4: 实现 15 分钟空闲 TTL 清理机制
    - 4.5: 实现 delete 方法（会话结束时调用）

- [✓] Task 5: 实现 MemoryService 门面骨架
    - 5.1: 创建 `python-engine/app/memory/service.py`，定义 MemoryService 类
    - 5.2: 实现 __init__ 方法（组合 session_meta_store、profile_card、summaries 等依赖）
    - 5.3: 实现 on_session_start 方法（L1 建立 + L2 预取）
    - 5.4: 实现 on_turn_complete 方法（L1 记账，暂时不处理 compaction）
    - 5.5: 实现 on_session_end 方法（L1 丢弃）
    - 5.6: 实现占位方法：recall、save_summary、update_profile、forget、list_conflicts、resolve_conflict

- [✓] Task 6: 改造 memory 工具
    - 6.1: 修改 `python-engine/app/tools/memory.py`，将 remember 工具改接 MemoryService.update_profile
    - 6.2: 修改 recall 工具，实现 L2 精确匹配（暂不接入 L3）
    - 6.3: 修改 forget 工具，改接 MemoryService.forget
    - 6.4: 新增 memory_search 工具占位（暂返回空列表）

- [✓] Task 7: 单元测试 - L2 ProfileCard
    - 7.1: 创建 `python-engine/tests/memory/test_profile_card.py`
    - 7.2: 测试槽位 upsert 幂等性
    - 7.3: 测试 version 递增逻辑
    - 7.4: 测试冲突规则（user_confirmed 不覆盖、derived 覆盖）
    - 7.5: 测试 Redis 缓存穿透失效
    - 7.6: 测试 200 条目淘汰逻辑（confidence × recency 排序）

- [✓] Task 8: 单元测试 - MemoryService 基础
    - 8.1: 创建 `python-engine/tests/memory/test_service.py`
    - 8.2: 测试 on_session_start（L1 建立 + L2 预取）
    - 8.3: 测试 on_turn_complete（turn_count 递增、token 累计）
    - 8.4: 测试 on_session_end（L1 丢弃）
    - 8.5: 测试工具集成（remember/recall/forget）

- [✓] Task 9: 移除废弃的 MemoryStore
    - 9.1: 删除 `python-engine/app/memory/store.py`（全局单例）
    - 9.2: 从 `python-engine/app/skill/store.py` 中移除对 store.py 的引用
    - 9.3: 检查代码库中其他对 store.py 的引用并清理

- [✓] Task 10: 验证与文档
    - 10.1: 运行 pytest，确保所有测试通过
    - 10.2: 检查测试覆盖率（新增模块 ≥90%）
    - 10.3: 更新 API 文档（如有相关端点）
    - 10.4: 提交 PR-1（单个 commit）

# PR-2：摘要层

- [✓] Task 11: 数据库迁移 - L3 摘要表
    - 11.1: 创建 `migrations/20260821000004_memory_summaries.up.sql`，定义 memory_summaries 表
    - 11.2: 添加唯一索引 uq_ms_hash（content_hash 去重）
    - 11.3: 创建 `migrations/20260821000004_memory_summaries.down.sql`，定义回滚逻辑
    - 11.4: 在 `python-engine/app/db.py` 的 ensure_tables 中同步补建表结构

- [✓] Task 12: 实现 L3 SummaryStore provider
    - 12.1: 创建 `python-engine/app/memory/summary_store.py`，实现 SummaryStore 类
    - 12.2: 实现 save_summary 方法（Milvus insert + PG 镜像双写）
    - 12.3: 实现 recall 方法（向量检索 + final_score 排序，token 预算硬上限 6KB）
    - 12.4: 实现查询缓存（Redis 5 分钟，key = sha256(query + user_id)）
    - 12.5: 实现嵌入向量缓存（Redis 30 分钟）

- [✓] Task 13: 实现 Consolidator 巩固 pipeline
    - 13.1: 创建 `python-engine/app/memory/consolidator.py`，实现 Consolidator 类
    - 13.2: 实现步骤①：调用 ContextManager._summarise 生成摘要（复用现有）
    - 13.3: 实现步骤②：调用 TaskRouter NER 抽取实体（复用现有）
    - 13.4: 实现步骤③：去重检查（content_hash 精确 + cosine>0.95 近重复）
    - 13.5: 实现步骤④：调用 SummaryStore.save_summary 双写
    - 13.6: 实现步骤⑤：稳定事实探测（entities/topics 命中 L2 或跨会话≥2次）
    - 13.7: 实现 rollup 合并（同主题>20条触发）

- [✓] Task 14: 实现异步队列集成
    - 14.1: 在 MemoryService.on_turn_complete 中添加巩固任务入队逻辑
    - 14.2: 在 MemoryService.on_session_end 中添加会话级 rollup 入队逻辑
    - 14.3: 实现定期 rollup 后台任务（每凌晨触发）
    - 14.4: 实现队列积压超限告警

- [✓] Task 15: 扩展 Milvus memory_type
    - 15.1: 修改 MemoryManager，扩展 memory_type 取值（summary/topic/long_term）
    - 15.2: 确保所有查询都带 tenant_id 过滤
    - 15.3: 验证 metadata_json 结构兼容性

- [✓] Task 16: 实现 MemoryService 语义查询
    - 16.1: 完善 MemoryService.recall 方法（L2 + L3 合并）
    - 16.2: 实现最终排序（L2 整卡 + L3 top_k=5）
    - 16.3: 实现去重（摘要覆盖的 turn_range 与窗口重叠则丢弃）
    - 16.4: 实现 fail-soft 降级（Milvus 不可用时返回空 L3）

- [ ] Task 17: 单元测试 - SummaryStore
    - 17.1: 创建 `python-engine/tests/memory/test_summary_store.py`
    - 17.2: 测试 save_summary 双写（Milvus + PG）
    - 17.3: 测试 recall 向量检索（mock gateway embed）
    - 17.4: 测试 final_score 排序（构造 recency/access 差异）
    - 17.5: 测试查询缓存（同 query 重放零检索）
    - 17.6: 测试 token 预算截断（超出按 final_score 截断）

- [ ] Task 18: 单元测试 - Consolidator
    - 18.1: 创建 `python-engine/tests/memory/test_consolidator.py`
    - 18.2: 测试完整 pipeline（fake queue + mock LLM/NER）
    - 18.3: 测试去重逻辑（content_hash + cosine>0.95）
    - 18.4: 测试先写后清顺序（确保不丢数据）
    - 18.5: 测试崩溃中断重放幂等（重复调用不重复写入）
    - 18.6: 测试 rollup 合并

- [ ] Task 19: 单元测试 - L3 语义查询
    - 19.1: 测试 recall 方法（L2 + L3 合并）
    - 19.2: 测试去重逻辑（摘要与窗口重叠）
    - 19.3: 测试 fail-soft 降级（Milvus 异常）

- [ ] Task 20: 验证与文档
    - 20.1: 运行 pytest，确保所有测试通过（包括 PR-1）
    - 20.2: 检查测试覆盖率（新增模块 ≥90%）
    - 20.3: 更新架构文档（L3 层说明）
    - 20.4: 提交 PR-2（单个 commit）

# PR-3：主循环集成

- [ ] Task 21: 改造 AgentRuntime - 生命周期集成
    - 21.1: 修改 `python-engine/app/agent/runtime.py`，在 __init__ 中添加 memory: MemoryService 可选参数
    - 21.2: 在 execute 方法开头调用 memory.on_session_start
    - 21.3: 在 execute 方法中调用 memory.recall 获取 recalled
    - 21.4: 在 execute 方法结尾调用 memory.on_turn_complete
    - 21.5: 实现 memory=None 时的行为回归一致（兼容性）

- [ ] Task 22: 改造 PromptEngine - 记忆分段注入
    - 22.1: 修改 `python-engine/app/agent/prompt_engine.py`，build 方法增加 recalled 参数
    - 22.2: 实现 L2 档案卡注入（"── 记忆：用户档案 ──" 区块，≤1.5KB）
    - 22.3: 实现 L3 相关历史注入（"── 记忆：相关历史 ──" 区块，≤6KB）
    - 22.4: 确保记忆区块在系统提示之后、L4 窗口之前

- [ ] Task 23: 编排 compaction 与巩固
    - 23.1: 在 MemoryService.on_turn_complete 中检测 token 预算是否达到 80%
    - 23.2: 触发 compaction 时，先调用 consolidator.save_summary 确认
    - 23.3: 等待 memory_id 返回后，才用摘要块替换窗口内容
    - 23.4: 实现降级链：LLM 摘要失败 → 重试1次 → trim_to_fit + degraded 标记

- [ ] Task 24: 实现降级链
    - 24.1: 在 Consolidator 中捕获 LLM 摘要异常
    - 24.2: 实现重试逻辑（最多1次）
    - 24.3: 重试仍失败则标记 L1.degraded=True
    - 24.4: 回合结束后将待摘内容补交后台巩固队列

- [ ] Task 25: 集成测试 - 前缀稳定
    - 25.1: 创建 `python-engine/tests/integration/test_memory_prefix_stable.py`
    - 25.2: 测试 compaction 后 messages[:retain] 与之前逐字节一致
    - 25.3: 测试缓存命中率不受影响

- [ ] Task 26: 集成测试 - 降级链
    - 26.1: 创建 `python-engine/tests/integration/test_memory_degradation.py`
    - 26.2: 测试 LLM 摘要失败 → 重试 → trim 流程
    - 26.3: 测试 degraded 标记正确设置
    - 26.4: 测试后台补交逻辑

- [ ] Task 27: 集成测试 - 兼容性
    - 27.1: 创建 `python-engine/tests/integration/test_memory_compat.py`
    - 27.2: 测试 memory=None 时行为回归一致
    - 27.3: 测试现有对话不受影响

- [ ] Task 28: 集成测试 - 多租户隔离
    - 28.1: 创建 `python-engine/tests/integration/test_memory_isolation.py`
    - 28.2: 测试 PG 行级 tenant_id 过滤
    - 28.3: 测试 Milvus expr 过滤
    - 28.4: 测试跨租户零泄漏（双租户场景）

- [ ] Task 29: 完善工具 memory_search
    - 29.1: 修改 memory_search 工具，直查 L3 语义层
    - 29.2: 实现 token 预算控制
    - 29.3: 添加单元测试

- [ ] Task 30: 验证与文档
    - 30.1: 运行 pytest，确保所有测试通过（包括 PR-1/2）
    - 30.2: 检查测试覆盖率（新增模块 ≥90%）
    - 30.3: 运行集成测试（前缀稳定、降级链、兼容性、多租户）
    - 30.4: 更新架构文档（L4 层、集成点说明）
    - 30.5: 提交 PR-3（单个 commit）

# PR-4：冲突与前端

- [ ] Task 31: 实现 Python 侧冲突管理
    - 31.1: 在 ProfileCard.upsert_item 中产出 MemoryConflict 事件
    - 31.2: 实现 pending_confirmation Redis 存储（TTL 7天）
    - 31.3: 实现 derived 二次出现自动写入逻辑
    - 31.4: 实现 user_confirmed 冲突挂起逻辑

- [ ] Task 32: 实现冲突 API（Python 侧）
    - 32.1: 创建 `python-engine/app/api/memory.py`
    - 32.2: 实现 GET /v1/memory/profile（读整卡）
    - 32.3: 实现 PUT /v1/memory/profile/{slot}/{key}（手工维护）
    - 32.4: 实现 DELETE /v1/memory/profile/{slot}/{key}
    - 32.5: 实现 GET /v1/memory/summaries?limit=50（查看摘要记忆）
    - 32.6: 实现 GET /v1/memory/conflicts（待裁决冲突列表）
    - 32.7: 实现 POST /v1/memory/conflicts/{id}/resolve（裁决）

- [ ] Task 33: Go 网关 API 代理
    - 33.1: 修改 `internal/api/gateway_router.go`，注册 6 个记忆 API 代理
    - 33.2: 使用 newProxy 模式（复用现有模式）
    - 33.3: 添加必要的 JWT 鉴权中间件

- [ ] Task 34: 实现内部调用路径
    - 34.1: 修改 `internal/engine/python_client.go`，添加内部调用支持
    - 34.2: 实现 X-Internal-Token 鉴权
    - 34.3: 测试 Go → Python 调用链路

- [ ] Task 35: 前端 API 封装
    - 35.1: 修改 `frontend-vue/src/api/index.ts`，添加记忆 API 封装
    - 35.2: 实现 getMemoryProfile、updateMemoryProfile、deleteMemoryProfile
    - 35.3: 实现 getMemorySummaries、getMemoryConflicts、resolveMemoryConflict

- [ ] Task 36: 实现 ChatView 冲突流
    - 36.1: 修改 `frontend-vue/src/views/ChatView.vue`，监听 SSE "memory_conflict" 事件
    - 36.2: 创建 `frontend-vue/src/components/MemoryConflictCard.vue`
    - 36.3: 实现内联确认卡片（显示旧值/新值）
    - 36.4: 实现三按钮：保留旧值、采用新值、手动修改
    - 36.5: 实现表单验证和错误处理

- [ ] Task 37: 实现 ProfileView 记忆管理卡片
    - 37.1: 修改 `frontend-vue/src/views/ProfileView.vue`，新增"记忆管理"卡片
    - 37.2: 显示用户档案卡（按 slot 分组）
    - 37.3: 提供手工编辑入口（槽位/键值对表单）
    - 37.4: 显示最近摘要记忆（分页列表）
    - 37.5: 提供清空记忆按钮（级联 L2/L3/归档）

- [ ] Task 38: 单元测试 - 冲突管理
    - 38.1: 创建 `python-engine/tests/memory/test_conflict.py`
    - 38.2: 测试 derived 二次出现自动写入
    - 38.3: 测试 user_confirmed 冲突挂起
    - 38.4: 测试三选项裁决生效
    - 38.5: 测试用户否认 → 抑制名单
    - 38.6: 测试 pending_confirmation TTL

- [ ] Task 39: 单元测试 - API
    - 39.1: 创建 `python-engine/tests/api/test_memory_api.py`
    - 39.2: 测试 6 个 REST API 端点
    - 39.3: 测试权限控制（租户/用户隔离）

- [ ] Task 40: Go 侧单元测试
    - 40.1: 创建 `internal/api/memory_proxy_test.go`
    - 40.2: 测试 6 个代理端点
    - 40.3: 测试鉴权逻辑

- [ ] Task 41: 前端组件测试
    - 41.1: 创建 `frontend-vue/src/components/__tests__/MemoryConflictCard.spec.ts`
    - 41.2: 测试冲突卡片渲染
    - 41.3: 测试三按钮交互
    - 41.4: 测试表单验证

- [ ] Task 42: e2e 测试 - 冲突流
    - 42.1: 创建 `frontend-vue/tests/e2e/memory_conflict.spec.ts`
    - 42.2: 测试完整冲突流（冲突产生 → SSE 推送 → 用户裁决）
    - 42.3: 测试三选项全部路径

- [ ] Task 43: 性能测试
    - 43.1: 测试 L2 缓存命中率（目标 >95%）
    - 43.2: 测试 L3 召回延迟（目标 <500ms）
    - 43.3: 测试并发场景

- [ ] Task 44: 验收与文档
    - 44.1: 运行 pytest，确保所有测试通过（包括 PR-1/2/3）
    - 44.2: 运行 vitest，确保前端组件测试通过
    - 44.3: 运行 e2e 测试
    - 44.4: 检查整体测试覆盖率（≥90%）
    - 44.5: 验证所有验收标准（见 doc.md §10）
    - 44.6: 更新 API 文档（6 个新端点）
    - 44.7: 更新用户文档（记忆管理功能说明）
    - 44.8: 提交 PR-4（单个 commit）

- [ ] Task 45: 全量回归测试
    - 45.1: 运行 `go test ./...`（Go 侧）
    - 45.2: 运行 `pytest`（Python 侧）
    - 45.3: 运行 `ruff check`（代码检查）
    - 45.4: 运行 `vue-tsc`（类型检查）
    - 45.5: 运行 `vite build`（前端构建）
    - 45.6: 验证在全新数据库上迁移可运行

- [ ] Task 46: 最终提交与推送
    - 46.1: 合并所有 4 个 PR（squash 成 4 个逻辑提交）
    - 46.2: 推送到远程仓库
    - 46.3: 触发 CI 流水线
    - 46.4: 确认 CI 全绿

- [ ] Task 47: 清理 .workbuddy 目录
    - 47.1: 删除 `.workbuddy` 目录及其所有内容
    - 47.2: 重写 git 历史移除 .workbuddy（git rebase -i 或 filter-branch）
    - 47.3: 强制推送（谨慎操作，确认备份）
