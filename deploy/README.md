# MiniCC 部署配置

## 目录结构

```
deploy/
├── helm/                    # Helm Charts
│   ├── gateway/            # Go 网关服务
│   ├── python-engine/      # Python AI 引擎（内置 LLM Gateway）
│   └── frontend/           # Vue 前端
├── k8s/                    # 原生 K8s 清单
│   ├── python-engine-deployment.yaml
│   ├── python-engine-service.yaml
│   ├── python-engine-hpa.yaml
│   ├── python-engine-pdb.yaml
│   └── python-engine-servicemonitor.yaml
└── docker/                 # Docker 镜像配置
```

## 快速部署

### 开发环境 (docker-compose)

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑 .env 填入 LLM Provider API Keys
vim .env

# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps
```

### Kubernetes (Helm)

```bash
# 安装 Go 网关
helm install minicc-gateway deploy/helm/gateway \
  --namespace minicc --create-namespace

# 安装 Python 引擎
helm install minicc-engine deploy/helm/python-engine \
  --namespace minicc
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | Go 服务端口 | `8080` |
| `POSTGRES_DSN` | PostgreSQL 连接串 | - |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `JWT_SECRET` | JWT 密钥 (≥16字符) | - |
| `PYTHON_ENGINE_ADDRESS` | Python 引擎地址 | `localhost:8000` |
| `ANTHROPIC_API_KEY` | Anthropic API Key | - |
| `OPENAI_API_KEY` | OpenAI API Key | - |
| `DEEPSEEK_API_KEY` | DeepSeek API Key | - |
| `TEMPORAL_ADDRESS` | Temporal Server 地址 | `localhost:7233` |
