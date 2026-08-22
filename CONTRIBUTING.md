# Contributing to MiniCC

欢迎贡献!MiniCC 是一个多租户 SaaS AI Agent 平台(Go 网关 + Python AI 引擎 + Vue 3 前端)。在提交 PR 前,请先阅读本文件与 [SECURITY.md](SECURITY.md)。

## 开发环境

### 前置依赖

| 依赖 | 版本要求 | 用途 |
|---|---|---|
| Go | 1.26+([go.mod](go.mod) 声明 `go 1.26.6`) | 网关 |
| Python | 3.11+ | AI 引擎 |
| Node.js | 22+ | 前端(Vite 8 要求) |
| pnpm / npm | 任意 | 前端依赖管理(仓库同时提交 `pnpm-lock.yaml` 与 `package-lock.json`,CI 使用 npm) |
| PostgreSQL | 16(pgvector) | 主库 / 向量库 |
| Redis | 7 | 必需依赖:队列 / 语义缓存 / 分布式限流 |
| MinIO / S3 | 可选 | 媒体对象存储(`STORAGE_BACKEND=s3`) |
| Milvus | 2.5 | 可选,向量检索(也可用 pgvector) |
| Temporal | 最新 | 可选,工作流引擎 |

### 搭建步骤

```bash
# 1. 克隆
git clone https://github.com/athenavi/minicc.git && cd minicc

# 2. 启动基础设施(PostgreSQL + Redis;需要媒体/向量时再加 minio milvus-standalone temporal)
docker compose up -d postgres redis

# 3. 配置环境变量
cp .env.example .env
#    编辑 .env:JWT_SECRET(必填,openssl rand -base64 48)、POSTGRES_DSN、LLM API Key

# 4. 安装依赖并启动
python run.py setup      # 首次:安装 Python 依赖、前端依赖
python run.py start      # 启动网关(:8080)+ 引擎(:8000)+ 前端(:5173)
```

常用命令:

```bash
python run.py status | logs | stop | restart | build
make build test lint fmt            # Go 侧(可选,CI 已覆盖)
```

## 代码规范

### Go(网关,`internal/`、`cmd/`、`config/`)

- 使用 `gofmt` 格式化,`go vet ./...` 零告警;
- 提交前运行 `go test ./... -count=1`(相关包);
- 错误处理:不吞错、不裸 `panic`;日志使用 `log/slog` 结构化输出。

### Python(引擎,`python-engine/`)

- `ruff check .` 零告警(配置见 `python-engine/pyproject.toml`:E/F/W/I/N/UP/B,line-length 120);
- 类型标注:新代码尽量通过 `mypy app/ --ignore-missing-imports`;
- 测试:`cd python-engine && python -m pytest tests`(异步测试 `asyncio_mode=auto`);集成类测试打 `@pytest.mark.integration` 标记。

### Vue 3 / TypeScript(前端,`frontend-vue/`)

- `npm run lint`(eslint + eslint-plugin-vue)零告警;
- `npx vue-tsc --noEmit -p tsconfig.app.json` 类型检查通过;
- 组件使用 `<script setup lang="ts">`;路由、状态(Pinia)与 API 封装分层清晰;
- 单测:`npm test`(vitest)。

## 测试

| 层 | 命令 |
|---|---|
| 网关 | `go test ./... -race -count=1 -timeout=120s` |
| 引擎 | `cd python-engine && python -m pytest tests` |
| 前端 | `cd frontend-vue && npm test` |

CI([.github/workflows/ci.yml](.github/workflows/ci.yml))会在 push / PR 到 `main` 时运行以上三端检查,PR 必须全部通过。

## 提交信息规范

使用 Conventional Commits 风格:`<type>(<scope>): <subject>`,例如:

```text
feat(media): 增加媒体签名 URL 全链路
fix(security): 修复存储型 XSS(分片上传 + /media/ 服务)
refactor(gateway): 抽取统一中间件链
test(api): /ready 新契约适配
docs(architecture): 补充多租户隔离矩阵
ci: 三端 CI 流水线(go/pytest/vue-tsc)
```

- `type`:`feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `ci` / `perf` / `style`;
- `scope`(可选):`gateway` / `engine` / `frontend` / `media` / `market` / `security` / `redis` 等;
- subject 用祈使句、小写开头;中文或英文均可,但同一 PR 内保持一致。

## PR 流程

1. 从最新 `main` 切出分支:`git checkout -b feat/my-feature`;
2. 小步提交,每个提交保持可构建、可测试;
3. 推送后创建 PR,目标分支 `main`,描述改动动机与影响范围(涉及 UI 请附截图);
4. CI 三端检查(gateway / engine / frontend)全部通过;
5. 至少 1 名维护者 review 通过后 squash 合并。

## 行为准则(Code of Conduct)

暂用 [Contributor Covenant](https://www.contributor-covenant.org/) 2.1 版作为默认行为准则;在仓库正式发布前,请保持尊重、包容、建设性的协作氛围。维护者有权拒绝违反协作精神的内容。

## 问题反馈

- Bug / 功能建议:GitHub Issues;
- **安全漏洞:请走 [SECURITY.md](SECURITY.md) 的私有报告渠道,勿公开提交。**
