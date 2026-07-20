# MiniCC 云平台部署指南

> 版本: 3.0.0 | 最后更新: 2026-07-20

---

## 现有脚本的云平台差距分析

### 现有脚本架构

```
deploy-infra.sh          deploy-app.sh            deploy-monitoring.sh
  ├── PostgreSQL (Helm)    ├── Gateway (Helm)       ├── Prometheus (Helm)
  ├── Redis (Helm)         ├── Python (Helm)        ├── Loki (Helm)
  ├── Milvus (Helm)        └── Frontend (Helm)      └── Alert Rules
  └── MinIO (Helm)
```

**问题：所有基础设施都在 K8s 集群内自建，云平台应该使用托管服务。**

### 云原生架构

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: 应用层 (Helm - 几乎云无关)                          │
│  Go Gateway × N  │  Python Engine × N  │  Frontend × N     │
├─────────────────────────────────────────────────────────────┤
│ Layer 2: 基础设施服务 (Terraform - 每个云平台一个 module)     │
│  AWS: RDS│ElastiCache│S3│Zilliz  │  阿里云: RDS│Redis│OSS│  │
│  GCP: CloudSQL│Memorystore│GCS    │  Azure: DB│Cache│Blob   │
├─────────────────────────────────────────────────────────────┤
│ Layer 1: 集群 + 网络 + IAM (Terraform)                      │
│  EKS/AKS/GKE/ACK + VPC + 私网 + IAM Role + Secrets          │
└─────────────────────────────────────────────────────────────┘
```

---

## 各组件云平台适配表

### PostgreSQL

| 云平台 | 托管服务 | 连接方式 | 代码改动 |
|--------|---------|---------|---------|
| AWS | RDS for PostgreSQL | `postgres://user:pass@rds-host:5432/minicc` | **零改动** — 只需改 DSN |
| GCP | Cloud SQL for PostgreSQL | `postgres://user:pass@cloudsql-ip:5432/minicc` | 同上 |
| Azure | Azure Database for PostgreSQL | `postgres://user:pass@azure-host:5432/minicc` | 同上 |
| 阿里云 | RDS PostgreSQL | `postgres://user:pass@rds-host:5432/minicc` | 同上 |

**配置示例 (AWS RDS):**
```hcl
# terraform/aws/rds.tf
resource "aws_db_instance" "minicc" {
  engine         = "postgres"
  engine_version = "16.4"
  instance_class = "db.r6g.large"
  db_name        = "minicc"
  username       = "minicc"
  password       = var.postgres_password
  storage_type   = "gp3"
  allocated_storage = 100
  multi_az       = true
  skip_final_snapshot = false
  backup_retention_period = 30
  deletion_protection = true
}
```

### Redis

| 云平台 | 托管服务 | 模式 | 代码改动 |
|--------|---------|------|---------|
| AWS | ElastiCache for Redis | Cluster/Single | **零改动** — `RedisMode=cluster` |
| GCP | Memorystore for Redis | Single | 同上 |
| Azure | Azure Cache for Redis | Cluster/Single | 同上 |
| 阿里云 | 云数据库 Redis | Cluster/Single | 同上 |

**配置示例 (AWS ElastiCache):**
```hcl
# terraform/aws/redis.tf
resource "aws_elasticache_replication_group" "minicc" {
  replication_group_id = "minicc"
  engine               = "redis"
  engine_version       = "7.2"
  node_type            = "cache.r6g.large"
  num_cache_clusters   = 3
  automatic_failover_enabled = true
  multi_az_enabled     = true
  parameter_group_name = "default.redis7.cluster.on"
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
}
```

### 对象存储 (S3 兼容)

| 云平台 | 服务 | Go (`internal/storage/s3.go`) | Python (`s3_store.py`) |
|--------|------|------------------------------|------------------------|
| AWS | S3 | `endpoint=""` 自动使用 AWS 路由 | 移除 `endpoint_url` 使用默认凭证链 |
| GCP | GCS (XML API) | `endpoint="https://storage.googleapis.com"` | 同上 |
| Azure | Blob Storage | `endpoint="https://<account>.blob.core.windows.net"` | 同上 |
| 阿里云 | OSS | `endpoint="https://oss-<region>.aliyuncs.com"` | 同上 |

**代码层面的关键改动：**

```go
// internal/storage/s3.go — 云平台 IAM 支持
func NewS3Store(cfg S3Config) (*S3Store, error) {
    var creds *credentials.Credentials
    if cfg.AccessKey != "" {
        // 兼容现有静态凭证（自建 MinIO 场景）
        creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
    } else {
        // 云平台 IAM Role（AWS IRSA / GCP Workload Identity）
        creds = credentials.NewIAM("")  // 或直接使用默认凭证链
    }
    client, err := minio.New(cfg.Endpoint, &minio.Options{
        Creds:  creds,
        Secure: cfg.UseSSL,
    })
    // ...
}
```

```python
# python-engine/app/media/s3_store.py — 云平台 IAM 支持
import boto3

class S3Store:
    def __init__(self, bucket: str, endpoint: str = "", region: str = ""):
        self._bucket = bucket
        kwargs = {}
        if endpoint:
            kwargs["endpoint_url"] = endpoint  # 自建 MinIO
        if region:
            kwargs["region_name"] = region
        # 无静态凭证时自动使用 IAM Role
        self._client = boto3.client("s3", **kwargs)
```

### Milvus / 向量数据库

| 平台 | 服务 | 说明 |
|------|------|------|
| 自建 | Milvus Cluster | 需要大量资源（建议 8C/32G+） |
| Zilliz Cloud | 托管 Milvus | 兼容 pymilvus 协议，零代码改动 |
| AWS | Sagemaker + PGVector | 需适配 |
| Pinecone | 托管向量库 | 需替换 pymilvus → pinecone SDK |

---

## 跨云/混合云策略

### 1. 对象存储跨云同步

```
阿里云 OSS (主)  ──→  AWS S3 (灾备)
       │                    │
   同步策略:                同步策略:
   OSS 跨区域复制            S3 跨区域复制
   或 rclone 定时同步        或 S3 Batch Replication
```

**应用层双写（一致性要求高时）：**
```go
// 在 S3Store.Put() 中同时写入主备存储
func (s *S3Store) Put(ctx context.Context, path string, reader io.Reader) error {
    // 写入主存储
    if err := s.primary.Put(ctx, path, reader); err != nil {
        return err
    }
    // 异步写入灾备存储
    go func() {
        if err := s.secondary.Put(context.Background(), path, reader); err != nil {
            slog.Warn("secondary storage write failed", "path", path, "error", err)
        }
    }()
    return nil
}
```

### 2. DNS/流量管理

```
用户 ──→ DNS (AWS Route53 / GCP Cloud DNS / 阿里云 DNS)
           ↓
      全局 LB (AWS Global Accelerator / GCP Traffic Director)
           ↓
      区域 Ingress (AWS ALB / GCP GLB / 阿里云 ALB)
           ↓
      Go Gateway Pods
```

**ExternalDNS 配置：**
```yaml
# 自动管理 DNS 记录
apiVersion: v1
kind: Service
metadata:
  annotations:
    external-dns.alpha.kubernetes.io/hostname: minicc.example.com
    external-dns.alpha.kubernetes.io/ttl: "60"
```

### 3. 配置管理/Secret

```mermaid
flowchart LR
    Vault/AWS_Secrets_Manager -->|External Secrets Operator| K8s_Secret
    K8s_Secret -->|volumemount/env| Gateway_Pod
    K8s_Secret -->|volumemount/env| Python_Pod
```

**External Secrets 集成：**
```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secrets-manager
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: minicc-secrets
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: minicc-config
  data:
    - secretKey: LLM_API_KEY
      remoteRef:
        key: /minicc/prod/llm
        property: api_key
    - secretKey: JWT_SECRET
      remoteRef:
        key: /minicc/prod/jwt
        property: secret
```

---

## 各云平台 Terraform Module

### AWS

```hcl
# terraform/aws/main.tf
module "minicc" {
  source = "./modules/minicc"

  environment = "production"
  region      = "us-east-1"

  # RDS
  postgres_instance_class = "db.r6g.large"
  postgres_storage        = 100

  # ElastiCache
  redis_node_type = "cache.r6g.large"
  redis_num_nodes = 3

  # S3
  s3_bucket_name = "minicc-media-prod"

  # EKS
  eks_node_groups = {
    gateway = { min = 2, max = 20, instance_types = ["m6i.large"] }
    python  = { min = 2, max = 50, instance_types = ["c6i.2xlarge"] }
  }

  # DNS
  domain_name = "minicc.example.com"
}
```

### GCP

```hcl
# terraform/gcp/main.tf
module "minicc" {
  source = "./modules/minicc-gcp"

  project_id = "minicc-prod"
  region     = "us-central1"

  # Cloud SQL
  postgres_tier = "db-custom-4-16384"
  postgres_version = "POSTGRES_16"

  # Memorystore
  redis_tier  = "STANDARD_HA"
  redis_memory_gb = 8

  # GCS
  gcs_bucket_name = "minicc-media-prod"

  # GKE
  gke_node_pools = {
    gateway = { machine_type = "e2-standard-2", min_nodes = 2, max_nodes = 20 }
    python  = { machine_type = "e2-highcpu-8",  min_nodes = 2, max_nodes = 50 }
  }
}
```

### Azure

```hcl
# terraform/azure/main.tf
module "minicc" {
  source = "./modules/minicc-azure"

  location = "eastus"

  # Azure Database for PostgreSQL
  postgres_sku = "GP_Standard_D4s_v3"
  postgres_storage_mb = 102400

  # Azure Cache for Redis
  redis_sku = "Premium"
  redis_capacity = 2

  # Blob Storage
  storage_account_name = "miniccmediaprod"
  container_name       = "media"

  # AKS
  aks_node_pools = {
    gateway = { vm_size = "Standard_D2s_v3", min_nodes = 2, max_nodes = 20 }
    python  = { vm_size = "Standard_F8s_v2",  min_nodes = 2, max_nodes = 50 }
  }
}
```

### 阿里云

```hcl
# terraform/alicloud/main.tf
module "minicc" {
  source = "./modules/minicc-alicloud"

  region = "cn-hangzhou"

  # RDS PostgreSQL
  postgres_instance_class = "pg.sql.x4r2.large.2"
  postgres_storage_size   = 100

  # 云数据库 Redis
  redis_instance_type = "redis.master.large.default"
  redis_instance_class = "standard"

  # OSS
  oss_bucket_name = "minicc-media-prod"

  # ACK (阿里云 K8s)
  ack_node_pools = {
    gateway = { instance_type = "ecs.g6.large",     min_nodes = 2, max_nodes = 20 }
    python  = { instance_type = "ecs.c6.2xlarge",   min_nodes = 2, max_nodes = 50 }
  }
}
```

---

## 部署流程对比

### 当前脚本（非云原生）

```bash
# 1. 基础设施自建在 K8s 内
./deploy-infra.sh

# 2. 应用部署
./deploy-app.sh

# 3. 监控
./deploy-monitoring.sh
```

### 云原生流程

```bash
# 1. Terraform 管理云资源 + K8s 集群
cd terraform/aws
terraform init
terraform plan -out=tfplan
terraform apply tfplan
# 输出: RDS 地址、ElastiCache 地址、S3 桶、EKS kubeconfig

# 2. 注入连接信息
export POSTGRES_DSN=$(terraform output -raw postgres_dsn)
export REDIS_ADDR=$(terraform output -raw redis_addr)
export S3_BUCKET=$(terraform output -raw s3_bucket)

# 3. External Secrets 同步凭证
kubectl apply -f externalsecret.yaml

# 4. 应用部署（无需自建基础设施）
helm upgrade --install minicc-gateway ./deploy/helm/gateway \
  --set config.postgresDsn=$POSTGRES_DSN \
  --set config.redisAddr=$REDIS_ADDR

# 5. 可选：监控（托管版）
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack
```

---

## 迁移路线图

| 阶段 | 任务 | 时间估计 |
|------|------|---------|
| **Phase 1** | 现有脚本不改，手动创建云托管服务，改 DSN 连接即可跑通 | 1-2 天 |
| **Phase 2** | 创建 Terraform module（选一个云平台先做），替换 deploy-infra.sh | 1 周 |
| **Phase 3** | IAM Role 替代静态凭证（S3/Secrets Manager），删除 .env 依赖 | 3-5 天 |
| **Phase 4** | 跨云/混合云（DNS/对象存储同步/多区域），仅高可用需求时需要 | 2 周 |

**建议从 AWS 开始**，因为 minio-go SDK 对 AWS S3 的支持最成熟（自动 endpoint 解析），且 Terraform AWS Provider 最完善。
