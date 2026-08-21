# MiniCC 项目长期笔记

## 项目概况
- 多租户 SaaS AI Agent 平台：Go 网关(:8080) + Python 引擎(:8000) + Vue3 前端(:5173) + PG + Redis + Milvus
- 上游：https://github.com/Athenavi/minicc（克隆于 2026-08-21，基线 commit ce25332）
- AGENTS.md 定义 12 条开发规则：fail loud、surgical changes、简朴优先、测试验证意图

## 关键架构约定
- **安全三栅栏**：输入注入检测 / 工具调用三态裁决 / 输出路径脱敏
- **沙箱复用模式**：Python 脚本执行复用 `run_code.py`（静态 AST 检查 + `_safe_builtins` 运行时守卫）；Shell 复用 `sandbox.run_in_sandbox`；出站 HTTP 复用 `ssrf.assert_safe_url`
- **fail-loud 原则**：未实现功能显式抛 NotImplementedError/错误，绝不返回空列表/硬编码 0 伪装成功。实现功能后必须同步更新对应 fail-loud 测试断言
- **迁移双轨**：SQL 迁移放 migrations/（Atlas 格式 up/down 对），Python 侧 db.py `ensure_tables` 需同步补列（幂等 ALTER）
- `workflow/engine.py` 的节点函数是 `_build_node_fns` 内的**闭包**，不能从模块 import——tracing_engine 曾因此踩坑
- **测试文件命名**：必须 `test_*.py` 才被 pytest 收集——`e2e_test_*.py` 曾导致整个文件 9 个失败从未暴露
- **AgentRuntime 签名**：`__init__(gateway, tool_executor=None, sse_producer=None, session_store=None)`；mode/compaction 按任务从 `task.llm_config` 解析（`_resolve_compaction`：llm_config["compaction"] 优先于 mode 默认）
- **TraceWriter 单例 mock**：测试需同时替换 `TraceWriter._redis`、`TraceWriter._instance`、`trace_writer_mod._trace_writer` 三处
- **ToolRegistry API**：`list_names()` / `get()` / `register()`，无 `list_tools()`

## 环境备忘
- Git 克隆需 `GIT_SSL_NO_VERIFY=1`（Windows schannel 吊销检查问题）
- python-engine venv 在 `python-engine/.venv`（pytest 9.x + fastapi + redis + asyncpg + httpx 等）
- **Go 1.26.6 已可用**（scoop 安装，2026-08-21 确认；此前"无 Go 工具链"结论过时，minicc.exe 预编译产物仍是线上跑的）
- 仓库 git 身份：yang / yang@local.dev（repo-local）
- **本机栈为裸进程部署**（非 Docker）：scoop PostgreSQL 17.4(:5432，数据 `D:/scoop/persist/postgresql/data`，日志 pg.log，**未注册 Windows 服务，挂了不会自动拉起**) + scoop redis(:6379) + minicc.exe(:8080) + python 引擎(:8000) + vite(:5173)；PG 启动命令 `D:/scoop/apps/postgresql/current/bin/pg_ctl.exe -D D:/scoop/persist/postgresql/data -l D:/scoop/persist/postgresql/pg.log start`；DSN `postgres://minicc:minicc@localhost:5432/minicc`

## 已完成开发（2026-08-21 会话）
skill 三种执行类型沙箱化、kb_documents tenant_id 元数据管理、ContextBus Redis Pub/Sub、协同 DAG 调度器、unified executor workflow 模式 + tracing_engine 闭包 bug 修复（commit aca9e73）。第二轮：llm/client.py 零向量桩 → 真实 LLMClient（本地优先+gateway 回退+fail-loud），retriever 嵌入前先查 Milvus（commit 23182b0）。第三轮：**六大工作台互联互通**（commit 68c8bf1）——能力注册中心修复（租户可见性/success_rate 冲突/preload 重写 9 能力）、TaskRouter 编排修复（get_gateway import/推荐站台流水线/`${dep.field}` 依赖模板注入/general_chat 兜底/工具上下文+ContextBus）、/v1/chat/submit 统一入口 + /v1/capabilities API、Go 网关代理 + /v1/quick-execute、前端 quickExecute 真实调用与调用链渲染。第四轮：**P0 上线阻断项修复**（commit fbcd582）——插件沙箱 subprocess 隔离（plugin_runner.py + code_guard.py 守卫抽取）、网关消息规范化 normalize_messages（修缓存层 dict.to_dict() 潜伏 bug）、路由降级死代码修复（降级意图 complexity 默认值短路）、3 个迁移幂等修复（全新部署阻断）。第五轮：**P1 任务全部完成**（commit 26a52b1）——MCP STDIO 子进程传输（_stdio_rpc: spawn+initialize握手+JSON-RPC stdin/stdout+超时kill+fail-loud，Windows shlex posix=False 兼容）、TaskRouter 意图定制聚合（search→来源去重/analyze→结论串联/generate_code→代码拼接）、NER 规则式实体抽取（8种：URL/邮箱/手机号/文件路径/日期/金额/IP/代码引用）、web 工具测试增强（DuckDuckGoProvider mock httpx SSRF/解析/截断/HTTP错误）、沙箱测试 Windows 大小写修复。全套 504 tests passed，P0+P1 全部清零。第六轮：**P2 收尾完成**（commit 2ff630e，543 tests passed）——collaboration.py TODO 掩盖的 3 个连环 bug 修复（无效构造参数/不存在的 run_single_turn/伪造 task）+ `_make_agent_task` 正确注入链路 + runtime per-task compaction 覆盖；tracing_engine span 租户泄漏修复（anonymous 流）；enhanced_kb.retrieve 按结果实际 tenant_id 过滤（原实现盲目覆盖掩盖泄漏）；e2e 测试重命名纳入回归（9 个预存失败修复）；browser StubHub 15 测试。**P0+P1+P2 全部清零，项目收尾完成。** 第七轮：**三方登录 + 人机验证防接口滥用**（commit 8e2d308，545 tests passed）——OIDC/OAuth2 全 Provider（google/github/wechat/dingtalk/feishu/qq/custom）+ State 双模式(bind/login HMAC) + 自助绑定/解绑守卫/设密码 + 人机验证 5 Provider（turnstile/recaptcha/hcaptcha/tencent/custom）+ 防滥用双保险（IP 失败计数升级≥5 强制验证码/≥30 硬 429）+ SSO cookie→Bearer 引导（`GET /v1/auth/session`）+ 前端 CaptchaWidget/SsoLoginButtons/ProfileView/admin OAuthProvidersView 全链路。SMS 手机验证码 P4 延后。第八轮：**P0+P1 安全缺口与存根清零**（commit fa4d256，563 pytest + 35 vitest passed）——路由守卫 requiresAdmin 真实校验（guard.ts 独立抽取 + auth store user 持久化 localStorage + AppLayout fetchProfile 三层保障）、前端 Vitest 测试框架从零搭建（35 用例覆盖 auth store/守卫/SSO 组件）、admin api key 管理前后端契约修复（SmartAPIKeyPool 稳定 key_id + update/remove，delete 路径 ID 定位）、AgentCollabPanel/WorkstationNav 两处空实现补全、CI type check 改 `vue-tsc -b` 强制。**代码库已知 TODO/存根/安全缺口全部清零。**第九轮：**SMS 手机验证码登录**（commit c1e458b，三方登录 P4 收尾，563 pytest + 40 vitest + Go 32 新测试）——ent_sms_config 单行配置表、阿里云/腾讯云/custom 三 Provider 真实签名发送、验证码 Redis 存储 + 防滥用四保险、自动建号、绑定/解绑/管理配置全链路、前端短信登录 Tab + Profile 绑定卡 + admin 配置卡。认证体系至此完备：密码/三方 OIDC+OAuth2/短信三通道 + 人机验证 + 防滥用。第十轮：**记忆系统 L2 档案卡 + 前端记忆页面**（commit 2179d25 后端 + 7e6ca19 前端，595 pytest + 44 vitest）——`user_memory_entries` 表四槽位（identity/preference/decision/fact）+ 条目级 embedding + ProfileStore asyncpg CRUD + MemoryService 门面（upsert 容量护栏/语义搜索 cosine+rerank/异步整理三步 backfill→merge→archive）+ 8 条 API 路由 + Go 网关代理 + tools/memory.py 改造（service 优先回退全局）+ 前端 MemoryView.vue（Tabs/语义检索/增删改 Modal/异步整理轮询/去重提示）+ api/memory.ts + 路由导航。L2 档案卡全链路就绪，L1/L3/L4 尚未接入 agent 主循环（设计稿已就位）。

## 关键架构约定（第三轮补充）
- **能力注册中心**：capability_id 格式 `{workstation}:{name}`；tenant_id="" 为全局能力，所有租户可见；执行器工厂模式（_tool_executor 委托 tools registry / _llm_executor 懒取 gateway fail-loud）
- **bind_gateway 注入模式**：模块级单例 + 启动时注入（tools/graph、tools/pm、workflow/tools、llm/client 均用此模式），新增模块注入 gateway 时放 main.py lifespan 的 gateway 注入区
- **Go 网关 Python 代理**：`newProxy` 工厂 + `pathFn`/`pathParamSuffix` 路径构造器，代理时自动追加 `?user_id=<claims.UserID>`

## 关键架构约定（第七轮补充）
- **OAuth2 Exchanger 抽象**：SSOHandler 按 `provider.protocol` 分派 OIDC（go-oidc 发现）或 OAuth2（显式端点）；OIDCExchanger 同接口
- **Provider 模板注册表**：oauth_profiles.go 内置 google/github/wechat/dingtalk/feishu/qq/custom 端点模板；DB 列非空即覆盖模板
- **State 双模式**：StatePayload `Mode`("login"|"bind") + `UID`，HMAC 全覆盖；旧格式（无 m/u 字段）向后兼容按 login 处理
- **人机验证契约**：428+Error="captcha_required" 作为前端加载验证码组件信号；fail-loud（服务商不可达 502/启用但 secret 缺失 503，绝不静默放行）
- **防滥用双保险**：管理员启用验证码强制校验 + IP 失败计数升级（≥5 强制验证码/≥30 硬 429，Redis 15 分钟窗口）
- **failCounterStore 窄接口**：`incr/get/clear` 三方法抽象，生产 redisFailCounter / 测试 memCounter
- **SSO cookie→Bearer 引导**：SSO 回调只设 httpOnly cookie（前端不可 JS 读）→ `GET /v1/auth/session` 端点引导为 `{token, user}` → 前端 bootstrapSession
- **`FRONTEND_URL` 环境变量**：组装 SSO 回调绝对地址（dev `http://localhost:5173/?sso=ok`）

## 关键架构约定（第八轮补充）
- **vue-tsc 假通过陷阱**：前端根 tsconfig 是 `{"files":[],"references":[...]}`，`vue-tsc --noEmit` 不带 `-b` 检查空文件集 = 永远零错误；真实类型检查必须 `vue-tsc -b`
- **前端测试栈**：vitest@4 + @vue/test-utils@2 + jsdom@30（vitest.config.ts jsdom 环境）；`vi.mock('api模块', 工厂)` mock 网络；断言语法 `expect(vi.mocked(fn)).toHaveBeenCalledWith(...)`
- **SmartAPIKeyPool 稳定 key_id**：`{provider}-{sha256(provider:key)[:12]}`——不泄露明文、同 key 稳定、管理端 PUT/DELETE 路径定位用
- **admin api key 前后端契约**：delete 前端不带 body → 后端路径 ID 优先 + body provider+key 兼容；get_all_keys 必须返回 id
- **git push 凭据链路**：WorkBuddy PortableGit 的 GCM 无缓存凭据（非交互环境不可用且段错误）；推送必须用系统 Git `GIT_SSL_NO_VERIFY=1 D:/scoop/apps/git/2.48.1/mingw64/bin/git.exe push origin main`（系统 GCM 有缓存 Athenavi 凭据）
- **npm install 需 `env -u NODE_OPTIONS`** 规避 WorkBuddy safe-delete 钩子

## 关键架构约定（第九轮补充，SMS 登录）
- **SMS Provider 注册表**：仿 captcha.go/oauth_profiles.go 模式——常量 → SmsKnownProviders() → 默认端点 map → SmsSender 接口 → HTTPSmsSender 按协议分派（aliyun/tencent/custom）
- **阿里云短信 POP RPC V1 签名**：RFC3986 percentEncode（`+`→`%20`、`*`→`%2A`、`%7E`→`~`）+ key 升序查询串 + `stringToSign = "POST&"+enc("/")+"&"+enc(query)` + base64(HMAC-SHA1(secret+"&", sts))
- **腾讯云 TC3-HMAC-SHA256**：canonical request（content-type;host;x-tc-action 签名头）→ HMAC 链（Date→Service→Signing）；手机号 E.164 裸号剥 `+`；SmsSdkAppId 复用 AccessKeyID 字段
- **ent_sms_config 单行配置表**：UNIQUE(tenant_id) + INSERT ... ON CONFLICT DO UPDATE；secret EncryptAESGCM 加密入库 + maskedSecret 脱敏响应
- **验证码防滥用四保险**：CaptchaHandler.Enforce + 发送冷却(Redis TTL) + 每日上限 + 错 5 次作废（计入 IP 失败计数）；验证通过立即 DelCode 防重放
- **自动建号模式**（同 provisionAndBind）：email=`{phone}@sms.local` + 随机 bcrypt 密码 + password_set=FALSE + ON CONFLICT DO NOTHING RETURNING（ErrNoRows 回查处理并发）
- **签名正确性测试策略**：mock 服务端按官方文档**独立复算**签名比对（不回放实现自身逻辑），阿里云/腾讯云均如此

## 关键架构约定（第十轮补充，记忆系统 L2）
- **嵌入文本格式**：`_embed_text(key, value) = f"{key}: {value}"`——测试桩 FakeEmbedder 映射键必须用此格式，否则 embedding 为 None
- **fail-soft 嵌入**：embedder 不可用/抛错时条目照常入库（embedding=NULL），整理任务稍后补齐——全记忆系统唯一 fail-soft 点，其余路径 fail-loud
- **异步整理状态二义性**：`running=False` 在"任务尚未启动"与"任务已完成"两种状态都成立 → 轮询必须判 `finished_at > 0` 或 `result/error` 非空
- **vitest 不要 `env -u NODE_OPTIONS`**：该选项导致 vitest 输出为空（退出码 0 但 0 字节），仅 npm install 需要该选项
- **jsdom matchMedia polyfill**：ant-design-vue Tabs/Grid 经 useBreakpoint 调用 window.matchMedia，jsdom 未实现 → 组件 setup 抛错 → onMounted 不执行 → 需测试顶部手动 polyfill
- **Tabs/TabPane 必须 import**：`<script setup>` 遗漏导入会导致组件不渲染（真实 bug，非仅测试）

## 剩余 TODO（上游遗留，均为非阻塞）
- tools/browser.py 真实浏览器自动化（StubHub 已有 15 测试覆盖，真实实现依赖 Chrome Extension WebSocket 设施）
- 记忆系统 L1/L3/L4 尚未接入 agent 主循环（设计稿 `.workbuddy/design/记忆系统四层架构实施方案.md` 已就位）；L2 档案卡已全链路就绪
