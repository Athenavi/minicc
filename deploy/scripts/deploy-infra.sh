#!/bin/bash
# ============================================================================
# MiniCC 企业级一键部署脚本
# 版本: 3.0.0
# 用法: ./deploy-infra.sh <postgres-password> <redis-password> <minio-password>
# ============================================================================
set -euo pipefail

POSTGRES_PASSWORD="${1:?Usage: $0 <postgres-password> <redis-password> <minio-password>}"
REDIS_PASSWORD="${2:?}"
MINIO_PASSWORD="${3:?}"
NAMESPACE="minicc-infra"

echo "=========================================="
echo "  MiniCC 基础设施部署"
echo "=========================================="

# ── PostgreSQL (主从) ──
echo "[1/4] Deploying PostgreSQL..."
helm upgrade --install postgres bitnami/postgresql \
  --namespace "$NAMESPACE" --create-namespace \
  --set auth.database=minicc \
  --set auth.username=minicc \
  --set auth.password="$POSTGRES_PASSWORD" \
  --set architecture=replication \
  --set readReplicas.count=2 \
  --set primary.resources.requests.cpu=2 \
  --set primary.resources.requests.memory=4Gi \
  --set primary.persistence.size=100Gi \
  --wait --timeout 10m

# ── Redis (主从) ──
echo "[2/4] Deploying Redis..."
helm upgrade --install redis bitnami/redis \
  --namespace "$NAMESPACE" \
  --set auth.password="$REDIS_PASSWORD" \
  --set architecture=replication \
  --set replica.replicaCount=3 \
  --set master.resources.requests.cpu=1 \
  --set master.resources.requests.memory=2Gi \
  --set metrics.enabled=true \
  --wait --timeout 5m

# ── Milvus ──
echo "[3/4] Deploying Milvus..."
helm upgrade --install milvus milvus/milvus \
  --namespace "$NAMESPACE" \
  --set cluster.enabled=false \
  --set persistence.size=200Gi \
  --wait --timeout 10m

# ── MinIO ──
echo "[4/4] Deploying MinIO..."
helm upgrade --install minio bitnami/minio \
  --namespace "$NAMESPACE" \
  --set auth.rootUser=minicc \
  --set auth.rootPassword="$MINIO_PASSWORD" \
  --set persistence.size=500Gi \
  --set defaultBuckets=minicc-media \
  --wait --timeout 5m

echo "=========================================="
echo "  MiniCC 基础设施部署完成"
echo "  PostgreSQL: postgres-primary.$NAMESPACE:5432"
echo "  Redis:      redis-master.$NAMESPACE:6379"
echo "  Milvus:     milvus.$NAMESPACE:19530"
echo "  MinIO:      minio.$NAMESPACE:9000"
echo "=========================================="
