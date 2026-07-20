# MiniCC 企业级部署指南

> 版本: 3.0.0 | 最后更新: 2026-07-20

---

## 目录

1. [架构概览](#1-架构概览)
2. [环境要求](#2-环境要求)
3. [基础设施部署](#3-基础设施部署)
4. [应用部署 (Helm/K8s)](#4-应用部署-helmk8s)
5. [水平扩展](#5-水平扩展)
6. [高可用配置](#6-高可用配置)
7. [监控告警](#7-监控告警)
8. [日志聚合](#8-日志聚合)
9. [CI/CD](#9-cicd)
10. [数据库迁移](#10-数据库迁移)
11. [备份恢复](#11-备份恢复)
12. [安全加固](#12-安全加固)
13. [故障排查](#13-故障排查)
14. [附录](#14-附录)

---

## 1. 架构概览

```
                                    ┌──────────────────────────────────────┐
                                    │          客户端 (Web/App/API)         │
                                    └──────────────┬───────────────────────┘
                                                   │ HTTPS
                                                   ▼
                                    ┌──────────────────────────────────────┐
                                    │       负载均衡 (LB / Ingress)         │
                                    │     TLS 终止 / 7层路由 / WAF          │
                                    └──────────────┬───────────────────────┘
                                                   │
                          ┌────────────────────────┼────────────────────────┐
                          ▼                        ▼                        ▼
               ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
               │  Go Gateway      │    │  Go Gateway      │    │  Go Gateway      │
               │  (无状态)         │    │  (无状态)         │    │  (无状态)         │
               │  :8080           │    │  :8080           │    │  :8080           │
               └────────┬─────────┘    └────────┬─────────┘    └────────┬─────────┘
                        │                       │                       │
                        └───────────┬───────────┴───────────┬───────────┘
                                    │                       │
                                    ▼                       ▼
                         ┌──────────────────┐   ┌──────────────────┐
                         │ Python Engine    │   │ Python Engine    │
                         │ (无状态)         │   │ (无状态)         │
                         │ SessionStore    │   │ SessionStore    │
                         │ Redis-backed    │   │ Redis-backed    │
                         │ :8000           │   │ :8000           │
                         └────────┬─────────┘   └────────┬─────────┘
                                  │                      │
                                  └──────────┬───────────┘
                                             │
                    ┌────────────────────────┼────────────────────────┐
                    ▼                        ▼                        ▼
         ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
         │   PostgreSQL     │    │     Redis        │    │     Milvus      │
         │   (主从/共享)     │    │ (缓存/限流/会话)   │    │  (向量数据库)    │
         │   :5432          │    │   :6379          │    │  :19530         │
         └──────────────────┘    └──────────────────┘    └──────────────────┘
                                              │
                                              ▼
                                   ┌──────────────────┐
                                   │    MinIO/S3     │
                                   │  (文件存储)      │
                                   │   :9000         │
                                   └──────────────────┘

    监控:
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │  Prometheus  │  │   Grafana    │  │   Loki       │  │   Jaeger     │
    │  (指标)      │  │  (可视化)    │  │  (日志)       │  │  (链路追踪)  │
    └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘
```

### 核心设计原则

| 原则 | 实现方式 |
|------|---------|
| **无状态水平扩展** | Go Gateway 纯代理；Python Engine SessionStore 使用 Redis 后端，无粘性会话依赖 |
| **共享存储** | PostgreSQL（主从）、Redis（缓存/限流/会话）、MinIO/S3（文件）、Milvus（向量） |
| **降级韧性** | Redis 不可用时自动降级到内存；PG 不可用时功能受限但服务不崩溃 |
| **安全分层** | WAF → TLS → JWT/API Key → RBAC → 输入清洗 → 限流 |

---

## 2. 环境要求

### 2.1 资源规划

| 组件 | 最低配置 | 推荐配置 | 存储 |
|------|---------|---------|------|
| Go Gateway (×2) | 1C / 512Mi | 2C / 1Gi | — |
| Python Engine (×2) | 2C / 2Gi | 4C / 8Gi | — |
| PostgreSQL | 2C / 4Gi | 8C / 32Gi | 100GB+ SSD |
| Redis | 1C / 1Gi | 4C / 8Gi | 20GB+ |
| Milvus | 4C / 8Gi | 8C / 32Gi | 200GB+ SSD |
| MinIO | 2C / 4Gi | 4C / 16Gi | 500GB+ |
| Frontend (×2) | 0.5C / 256Mi | 1C / 512Mi | — |

### 2.2 软件版本

| 软件 | 版本 | 说明 |
|------|------|------|
| Kubernetes | ≥ 1.26 | 建议 1.28+ |
| Helm | ≥ 3.12 | Charts v2 格式 |
| PostgreSQL | ≥ 16 | 建议 16.4+ |
| Redis | ≥ 7 | 建议 7.2+ |
| Milvus | ≥ 2.4 | Standalone 或 Cluster |
| MinIO | ≥ 2024 | 或兼容 S3 存储 |
| Go | ≥ 1.22 (构建用) | 仅构建时需要 |
| Python | ≥ 3.11 (构建用) | 仅构建时需要 |
| Node.js | ≥ 20 (构建用) | 仅构建前端时需要 |

### 2.3 网络端口

| 端口 | 组件 | 说明 |
|------|------|------|
| 443 | Ingress/LB | HTTPS 入口 |
| 8080 | Go Gateway | HTTP 服务 |
| 8000 | Python Engine | HTTP 服务 |
| 5432 | PostgreSQL | 数据库 |
| 6379 | Redis | 缓存 |
| 9000 | MinIO | S3 API |
| 19530 | Milvus | gRPC 向量 |
| 9090 | Prometheus | 指标 |
| 3100 | Loki | 日志写入 |
| 4318 | OpenTelemetry | 链路追踪 |

---

## 3. 基础设施部署

### 3.1 PostgreSQL (主从)

```yaml
# postgres-values.yaml
architecture: replication
auth:
  database: minicc
  username: minicc
  password: ${POSTGRES_PASSWORD}
primary:
  resources:
    requests: { cpu: "2", memory: "4Gi" }
    limits:   { cpu: "8", memory: "32Gi" }
  persistence:
    size: 100Gi
    storageClass: ssd
readReplicas:
  count: 2
  resources:
    requests: { cpu: "2", memory: "4Gi" }
```

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm upgrade --install postgres bitnami/postgresql \
  --namespace minicc-infra --create-namespace \
  -f postgres-values.yaml
```

### 3.2 Redis (Cluster)

```yaml
# redis-values.yaml
architecture: replication
auth:
  enabled: true
  password: ${REDIS_PASSWORD}
master:
  resources:
    requests: { cpu: "1", memory: "2Gi" }
replica:
  replicaCount: 3
  resources:
    requests: { cpu: "1", memory: "2Gi" }
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

```bash
helm upgrade --install redis bitnami/redis \
  --namespace minicc-infra \
  -f redis-values.yaml
```

### 3.3 Milvus

```bash
helm repo add milvus https://zilliztech.github.io/milvus-helm/
helm upgrade --install milvus milvus/milvus \
  --namespace minicc-infra \
  --set cluster.enabled=false \
  --set persistence.size=200Gi
```

### 3.4 MinIO

```bash
helm upgrade --install minio bitnami/minio \
  --namespace minicc-infra \
  --set auth.rootUser=minicc \
  --set auth.rootPassword=${MINIO_PASSWORD} \
  --set persistence.size=500Gi \
  --set defaultBuckets=minicc-media
```

### 3.5 一键部署基础设施

```bash
# deploy/scripts/deploy-infra.sh
#!/bin/bash
set -euo pipefail

NAMESPACE="minicc-infra"
POSTGRES_PASSWORD="${1:?Usage: $0 <postgres-password> <redis-password> <minio-password>}"
REDIS_PASSWORD="${2:?}"
MINIO_PASSWORD="${3:?}"

echo "=== Deploying MiniCC Infrastructure ==="

# PostgreSQL
helm upgrade --install postgres bitnami/postgresql \
  --namespace "$NAMESPACE" --create-namespace \
  --set auth.database=minicc,auth.username=minicc \
  --set auth.password="$POSTGRES_PASSWORD" \
  --set architecture=replication,readReplicas.count=2

# Redis
helm upgrade --install redis bitnami/redis \
  --namespace "$NAMESPACE" \
  --set auth.password="$REDIS_PASSWORD" \
  --set architecture=replication,replica.replicaCount=3

# Milvus
helm upgrade --install milvus milvus/milvus \
  --namespace "$NAMESPACE" \
  --set cluster.enabled=false

# MinIO
helm upgrade --install minio bitnami/minio \
  --namespace "$NAMESPACE" \
  --set auth.rootUser=minicc,auth.rootPassword="$MINIO_PASSWORD" \
  --set defaultBuckets=minicc-media

echo "=== Infrastructure Deployed ==="
```

---

## 4. 应用部署 (Helm/K8s)

### 4.1 配置管理

创建 Secret：

```bash
# 从 .env 文件生成 K8s Secret
kubectl create secret generic minicc-config \
  --namespace minicc \
  --from-env-file=.env.production
```

推荐使用 [External Secrets Operator](https://external-secrets.io/) 集成 Vault/AWS Secrets Manager：

```yaml
# externalsecret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: minicc-config
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: vault
  target:
    name: minicc-config
  data:
    - secretKey: LLM_API_KEY
      remoteRef:
        key: /minicc/production/llm
        property: api_key
    - secretKey: JWT_SECRET
      remoteRef:
        key: /minicc/production/jwt
        property: secret
    - secretKey: POSTGRES_DSN
      remoteRef:
        key: /minicc/production/postgres
        property: dsn
```

### 4.2 部署 Go Gateway

```bash
cd deploy/helm/gateway

helm upgrade --install minicc-gateway . \
  --namespace minicc --create-namespace \
  --set replicaCount=2 \
  --set image.repository=registry.example.com/minicc-gateway \
  --set image.tag=v3.0.0 \
  --set config.postgresDsn="postgres://minicc:${PG_PASS}@postgres:5432/minicc" \
  --set config.redisAddr="redis://:${REDIS_PASS}@redis-master:6379" \
  --set config.jwtSecret="${JWT_SECRET}" \
  --set config.llmApiKey="${LLM_API_KEY}" \
  --set ingress.enabled=true \
  --set ingress.host=minicc.example.com \
  --set resources.requests.cpu=500m \
  --set resources.requests.memory=512Mi \
  --set hpa.enabled=true \
  --set hpa.minReplicas=2 \
  --set hpa.maxReplicas=10
```

### 4.3 部署 Python Engine

```bash
cd deploy/helm/python-engine

helm upgrade --install minicc-python . \
  --namespace minicc \
  --set replicaCount=2 \
  --set image.repository=registry.example.com/minicc-python \
  --set image.tag=v3.0.0 \
  --set config.openaiApiKey="${LLM_API_KEY}" \
  --set config.openaiBaseUrl="https://api.deepseek.com" \
  --set config.defaultModel="deepseek-v4-flash" \
  --set config.redisUrl="redis://:${REDIS_PASS}@redis-master:6379" \
  --set config.milvusAddress="milvus:19530" \
  --set resources.requests.cpu=1 \
  --set resources.requests.memory=2Gi \
  --set hpa.enabled=true \
  --set hpa.minReplicas=2 \
  --set hpa.maxReplicas=50
```

### 4.4 部署 Frontend

```bash
cd deploy/helm/frontend

helm upgrade --install minicc-frontend . \
  --namespace minicc \
  --set replicaCount=2 \
  --set image.repository=registry.example.com/minicc-frontend \
  --set image.tag=v3.0.0 \
  --set config.apiBaseUrl="https://minicc.example.com" \
  --set ingress.enabled=true \
  --set ingress.host=minicc.example.com
```

### 4.5 一键部署应用

```bash
# deploy/scripts/deploy-app.sh
#!/bin/bash
set -euo pipefail

NAMESPACE="minicc"
REGISTRY="${REGISTRY:-registry.example.com/minicc}"
TAG="${TAG:-v3.0.0}"

# 创建命名空间
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# 部署 Go Gateway
helm upgrade --install minicc-gateway deploy/helm/gateway \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-gateway" \
  --set image.tag="$TAG" \
  -f deploy/helm/gateway/production-values.yaml

# 部署 Python Engine
helm upgrade --install minicc-python deploy/helm/python-engine \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-python" \
  --set image.tag="$TAG" \
  -f deploy/helm/python-engine/production-values.yaml

# 部署 Frontend
helm upgrade --install minicc-frontend deploy/helm/frontend \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-frontend" \
  --set image.tag="$TAG" \
  -f deploy/helm/frontend/production-values.yaml

echo "=== Application Deployed ==="
```

---

## 5. 水平扩展

### 5.1 自动扩缩容 (HPA)

Go Gateway（基于 CPU/请求数）：

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: minicc-gateway
spec:
  minReplicas: 2
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Pods
      pods:
        metric:
          name: http_requests_per_second
        target:
          type: AverageValue
          averageValue: 1000
```

Python Engine（基于 CPU/内存）：

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: minicc-python
spec:
  minReplicas: 2
  maxReplicas: 50
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

### 5.2 Pod 调度策略

```yaml
# 反亲和：同一组件的 Pod 分布在不同节点
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app: minicc-python
          topologyKey: kubernetes.io/hostname

# 节点选择（GPU 节点可选）
nodeSelector:
  node-type: compute
```

### 5.3 PodDisruptionBudget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: minicc-gateway-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: minicc-gateway
```

---

## 6. 高可用配置

### 6.1 健康检查

Go Gateway:

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 15
  periodSeconds: 30
```

Python Engine:

```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8000
  initialDelaySeconds: 10
  periodSeconds: 15
livenessProbe:
  httpGet:
    path: /healthz
    port: 8000
  initialDelaySeconds: 30
  periodSeconds: 30
```

### 6.2 优雅关闭

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "10"]
# 配合 terminationGracePeriodSeconds: 60
```

### 6.3 Ingress + TLS

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minicc-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-read-timeout: "180"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "180"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"  # SSE 支持
spec:
  ingressClassName: nginx
  tls:
    - hosts: [minicc.example.com]
      secretName: minicc-tls
  rules:
    - host: minicc.example.com
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: minicc-gateway
                port: 8080
          - path: /
            pathType: Prefix
            backend:
              service:
                name: minicc-frontend
                port: 80
```

### 6.4 限流配置

```yaml
# 分布式限流（Redis-backed）
config:
  rateLimit:
    globalRPM: 10000   # 全局：每分钟最多 10000 请求
    tenantRPM: 5000     # 每租户：每分钟最多 5000
    userRPM: 1000       # 每用户：每分钟最多 1000
```

---

## 7. 监控告警

### 7.1 Prometheus 指标

所有组件暴露标准 `/metrics` 端点：

| 指标 | 类型 | 说明 |
|------|------|------|
| `minicc_http_requests_total` | Counter | 总请求量 (status/method/path) |
| `minicc_http_request_duration_ms` | Histogram | 请求延迟分布 |
| `minicc_llm_requests_total` | Counter | LLM 调用次数 (provider/model) |
| `minicc_llm_tokens_total` | Counter | Token 用量 (input/output) |
| `minicc_cache_hit_ratio` | Gauge | 语义缓存命中率 (L1/L2/L3) |
| `minicc_circuit_breaker_state` | Gauge | 熔断器状态 (0=closed, 1=open) |
| `minicc_queue_depth` | Gauge | 任务队列深度 |

### 7.2 告警规则

```yaml
# prometheus-rules.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: minicc-alerts
spec:
  groups:
    - name: minicc
      rules:
        - alert: HighErrorRate
          expr: rate(minicc_http_requests_total{status=~"5.."}[5m]) / rate(minicc_http_requests_total[5m]) > 0.05
          for: 5m
          labels: { severity: critical }
          annotations:
            summary: "Error rate exceeds 5%"

        - alert: HighLLMLatency
          expr: histogram_quantile(0.95, rate(minicc_llm_request_duration_ms_bucket[5m])) > 10000
          for: 5m
          labels: { severity: warning }
          annotations:
            summary: "P95 LLM latency > 10s"

        - alert: CircuitBreakerOpen
          expr: minicc_circuit_breaker_state > 0
          for: 1m
          labels: { severity: critical }
          annotations:
            summary: "LLM provider circuit breaker open"

        - alert: PodCrashLoop
          expr: kube_pod_status_phase{phase="CrashLoopBackOff"} > 0
          for: 5m
          labels: { severity: critical }
```

### 7.3 Grafana 仪表盘

预置仪表盘 JSON 文件位置：`deploy/monitoring/grafana-dashboard.json`

关键面板：
- **QPS & 延迟** — 网关请求速率和响应时间
- **LLM 用量** — Provider 分布、Token 消耗、缓存命中率
- **系统资源** — CPU/内存/网络/磁盘
- **业务指标** — 活跃会话数、消息量、工具调用数
- **限流/熔断** — 被限流请求数、熔断器状态

### 7.4 部署监控栈

```bash
# deploy/scripts/deploy-monitoring.sh
#!/bin/bash
set -euo pipefail

NAMESPACE="minicc-monitoring"

# Prometheus Stack (包含 Grafana + AlertManager)
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace "$NAMESPACE" --create-namespace \
  --set grafana.enabled=true \
  --set grafana.adminPassword="${GRAFANA_PASSWORD}" \
  --set alertmanager.enabled=true

# ServiceMonitor 已在 Helm Chart 中内置
echo "=== Monitoring Deployed ==="
```

---

## 8. 日志聚合

### 8.1 Loki + Promtail

```bash
helm upgrade --install loki grafana/loki-stack \
  --namespace minicc-monitoring \
  --set promtail.enabled=true \
  --set loki.persistence.enabled=true \
  --set loki.persistence.size=50Gi
```

### 8.2 日志格式

所有组件输出 JSON 格式日志：

```json
{"time":"2026-07-20T10:00:00Z","level":"INFO","msg":"request processed",
 "method":"POST","path":"/submit","status":202,"duration_ms":4520,
 "user_id":"u_abc123","session_id":"s_xxx","request_id":"r_yyy"}
```

### 8.3 日志查询

```logql
# 查询特定用户的请求
{app="minicc-gateway"} |= `"user_id":"u_abc123"`

# 查询错误率
sum(rate({app="minicc-gateway"} |= `"status":5` [5m])) / sum(rate({app="minicc-gateway"}[5m]))

# 查询 Python 引擎的 LLM 调用
{app="minicc-python"} |= "chat_stream"
```

---

## 9. CI/CD

### 9.1 完整 CI/CD 流水线

```yaml
# .github/workflows/cd.yml
name: Deploy

on:
  push:
    tags: [ 'v*' ]

jobs:
  # 并行构建各组件镜像
  build-gateway:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with: { registry: ${{ vars.REGISTRY }}, username: ${{ vars.DOCKER_USER }}, password: ${{ secrets.DOCKER_PASS }} }
      - run: docker buildx build --push -t $REGISTRY/minicc-gateway:$TAG -f Dockerfile .

  build-python:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker buildx build --push -t $REGISTRY/minicc-python:$TAG -f python-engine/Dockerfile python-engine/

  build-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker buildx build --push -t $REGISTRY/minicc-frontend:$TAG -f frontend-vue/Dockerfile frontend-vue/

  # 部署到 K8s
  deploy-staging:
    needs: [build-gateway, build-python, build-frontend]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          helm upgrade --install minicc-gateway deploy/helm/gateway \
            --namespace minicc-staging \
            --set image.tag=$TAG \
            --set replicaCount=1

  deploy-production:
    needs: [deploy-staging]
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      - run: |
          # 金丝雀：先更新 20% 的副本
          helm upgrade --install minicc-gateway deploy/helm/gateway \
            --namespace minicc \
            --set image.tag=$TAG \
            --set canary.enabled=true \
            --set canary.weight=20
```

### 9.2 金丝雀部署

```yaml
# canary.yaml (使用 Flagger)
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: minicc-gateway
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: minicc-gateway
  service:
    port: 8080
  analysis:
    interval: 1m
    threshold: 5
    maxWeight: 50
    stepWeight: 10
    metrics:
      - name: error-rate
        templateRef:
          kind: AlertMetric
          name: error-rate
        thresholdRange:
          max: 5
      - name: latency
        templateRef:
          kind: AlertMetric
          name: latency
        thresholdRange:
          max: 2000
```

### 9.3 数据库迁移自动化

```yaml
# .github/workflows/migrate.yml
name: Database Migration

on:
  push:
    paths: [ 'migrations/*.sql' ]

jobs:
  migrate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run migrations
        run: |
          go run ./cmd/migrate -dsn "${{ secrets.STAGING_DSN }}" up
      - name: Verify
        run: |
          go run ./cmd/migrate -dsn "${{ secrets.STAGING_DSN }}" status
```

---

## 10. 数据库迁移

### 10.1 迁移工具

使用项目内置的 `migrate` 命令：

```bash
# 查看迁移状态
go run ./cmd/migrate -dsn "$POSTGRES_DSN" status

# 执行迁移
go run ./cmd/migrate -dsn "$POSTGRES_DSN" up

# 回滚
go run ./cmd/migrate -dsn "$POSTGRES_DSN" down
```

### 10.2 迁移策略

| 阶段 | 操作 | 说明 |
|------|------|------|
| **准备** | 创建新列 (nullable) | 加列不锁表 |
| **写入** | 双写新旧列 | 应用代码同时写入 |
| **迁移** | 回填旧数据 | 定时任务逐步回填 |
| **切换** | 改为读取新列 | 代码部署切换 |
| **清理** | 删除旧列 | 确认稳定后删除 |

### 10.3 Atlas 集成

项目使用 [Atlas](https://atlasgo.io/) 管理迁移：

```bash
# 生成迁移文件
atlas migrate diff --env minicc

# 应用迁移
atlas migrate apply --env minicc --url "$POSTGRES_DSN"
```

---

## 11. 备份恢复

### 11.1 PostgreSQL 备份

```bash
#!/bin/bash
# deploy/scripts/backup-postgres.sh
BACKUP_DIR="/backups/postgres"
RETENTION_DAYS=30
DATE=$(date +%Y%m%d_%H%M%S)

pg_dump -Fc -f "$BACKUP_DIR/minicc_$DATE.dump" \
  -h postgres-primary -U minicc -d minicc

# 保留 30 天
find "$BACKUP_DIR" -name "minicc_*.dump" -mtime +$RETENTION_DAYS -delete

# 上传到 S3
aws s3 cp "$BACKUP_DIR/minicc_$DATE.dump" "s3://minicc-backups/postgres/"
```

### 11.2 MinIO 备份

```bash
# 使用 rclone 同步到异地
rclone sync minio:minicc-media backblaze:minicc-backup/media
```

### 11.3 恢复流程

```bash
# PostgreSQL 恢复
pg_restore -Fc -d minicc -h postgres-primary \
  -U minicc -c minicc_20260720_120000.dump
```

---

## 12. 安全加固

### 12.1 网络安全

```yaml
# 网络策略
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: minicc-network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/part-of: minicc
  policyTypes: [Ingress, Egress]
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
      ports: [8080, 8000]
  egress:
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/instance: postgres
        - podSelector:
            matchLabels:
              app.kubernetes.io/instance: redis
```

### 12.2 Secret 管理

| 密文 | 存储位置 | 轮转策略 |
|------|---------|---------|
| LLM API Key | Vault / External Secrets | 每月轮转 |
| JWT Secret | Vault / External Secrets | 每季度轮转 |
| DB 密码 | Vault / External Secrets | 每年轮转 |
| Redis 密码 | Vault / External Secrets | 每年轮转 |

### 12.3 安全配置清单

- [ ] TLS 1.3 仅用于 Ingress
- [ ] JWT 黑名单已启用（Redis-backed）
- [ ] API Key 哈希存储
- [ ] RBAC 最小权限原则
- [ ] 请求限流已启用（分布式/本地降级）
- [ ] 输入清洗（prompt injection 防护）
- [ ] CORS 白名单配置
- [ ] 安全头已启用（CSP/HSTS/XSS 等）
- [ ] NetworkPolicy 已配置
- [ ] Pod Security Standard: restricted
- [ ] 容器不以 root 运行
- [ ] 只读根文件系统

---

## 13. 故障排查

### 13.1 常见问题

| 症状 | 可能原因 | 排查命令 |
|------|---------|---------|
| Gateway 返回 502 | Python Engine 未就绪 | `kubectl get pods -l app=minicc-python` |
| 登录返回 401 | JWT Secret 不匹配 | `kubectl logs -l app=minicc-gateway` |
| AI 对话无响应 | LLM API Key 无效 | `kubectl logs -l app=minicc-python \| grep "chat_stream failed"` |
| 工作流保存失败 | PostgreSQL 不可用 | `kubectl logs -l app=minicc-python \| grep "graph insert failed"` |
| SSE 连接断开 | 反向代理缓冲 | 检查 Ingress `proxy-buffering: off` |
| 限流过快 | 限流配置过低 | `kubectl describe configmap minicc-gateway` |

### 13.2 诊断命令

```bash
# 查看所有组件状态
kubectl get pods -n minicc -o wide
kubectl get hpa -n minicc
kubectl get pdb -n minicc

# 查看实时日志
kubectl logs -l app=minicc-gateway -n minicc --tail=100 -f
kubectl logs -l app=minicc-python -n minicc --tail=100 -f

# 查看 Python Engine 启动日志
kubectl logs -l app=minicc-python -n minicc --tail=50 | grep -E "ERROR|WARN"

# 端口转发（本地调试）
kubectl port-forward -n minicc svc/minicc-gateway 8080:8080
kubectl port-forward -n minicc svc/minicc-python 8000:8000

# 查看资源使用
kubectl top pods -n minicc
kubectl top nodes
```

### 13.3 性能调优

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| Gateway `terminationGracePeriodSeconds` | 60 | 优雅关闭等待时间 |
| Python `--workers` | CPU 核心数 × 2 | Uvicorn Worker 数 |
| Redis `maxmemory` | 实例内存的 80% | 防止 OOM |
| PG `max_connections` | 200 | 连接池上限 |
| `PYTHON_ENGINE_TIMEOUT` | 300s | LLM 流式超时 |
| `queue_worker_concurrency` | 10 | 异步任务并发数 |

---

## 14. 附录

### 14.1 目录结构

```
deploy/
├── helm/
│   ├── gateway/          # Go Gateway Helm Chart
│   ├── python-engine/    # Python Engine Helm Chart
│   └── frontend/         # Frontend Helm Chart
├── k8s/                  # K8s 原生清单
├── scripts/
│   ├── deploy-infra.sh   # 基础设施一键部署
│   ├── deploy-app.sh     # 应用一键部署
│   ├── deploy-monitoring.sh  # 监控栈部署
│   └── backup-postgres.sh    # PG 备份脚本
└── monitoring/
    └── grafana-dashboard.json  # 预置仪表盘
```

### 14.2 环境变量清单

| 变量 | 组件 | 必填 | 说明 |
|------|------|------|------|
| `LLM_API_KEY` | 全部 | ✅ | LLM Provider API Key |
| `LLM_BASE_URL` | 全部 | ✅ | LLM API 地址 |
| `LLM_MODEL` | 全部 | ✅ | 默认模型名 |
| `JWT_SECRET` | Gateway | ✅ | JWT 签名密钥 (≥16字符) |
| `POSTGRES_DSN` | 全部 | ✅ | PostgreSQL 连接串 |
| `REDIS_ADDR` | Gateway | ✅ | Redis 地址 |
| `REDIS_URL` | Python | ✅ | Redis 连接 URL |
| `MILVUS_ADDRESS` | Python | ✅ | Milvus 地址 |
| `STORAGE_BACKEND` | Gateway | | 存储后端 (local/s3) |
| `S3_ENDPOINT` | Gateway | S3模式 | S3 端点 |
| `CORS_ORIGINS` | Gateway | | 允许的 Origin |
| `RATE_LIMIT_RPM` | Gateway | | 单实例限流 |

### 14.3 引用文档

- [Helm Chart 配置参考](deploy/helm/gateway/README.md)
- [Atlas 迁移指南](migrations/README.md)
- [API 文档](docs/api.md)
- [架构文档](docs/architecture.md)
