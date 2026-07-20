#!/bin/bash
# ============================================================================
# MiniCC 应用一键部署脚本
# 版本: 3.0.0
# 用法: ./deploy-app.sh <namespace> <image-tag>
# 环境变量: REGISTRY (默认: registry.example.com/minicc)
# ============================================================================
set -euo pipefail

NAMESPACE="${1:?Usage: $0 <namespace> <image-tag>}"
TAG="${2:?}"
REGISTRY="${REGISTRY:-registry.example.com/minicc}"

echo "=========================================="
echo "  MiniCC 应用部署"
echo "  命名空间: $NAMESPACE"
echo "  镜像标签: $TAG"
echo "  镜像仓库: $REGISTRY"
echo "=========================================="

# 创建命名空间
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# ── Go Gateway ──
echo "[1/3] Deploying Go Gateway..."
helm upgrade --install minicc-gateway ./deploy/helm/gateway \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-gateway" \
  --set image.tag="$TAG" \
  --set image.pullPolicy=Always \
  --set hpa.enabled=true \
  --set hpa.minReplicas=2 \
  --set hpa.maxReplicas=20 \
  --set resources.requests.cpu=500m \
  --set resources.requests.memory=512Mi \
  --wait --timeout 5m

# ── Python Engine ──
echo "[2/3] Deploying Python Engine..."
helm upgrade --install minicc-python ./deploy/helm/python-engine \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-python" \
  --set image.tag="$TAG" \
  --set image.pullPolicy=Always \
  --set hpa.enabled=true \
  --set hpa.minReplicas=2 \
  --set hpa.maxReplicas=50 \
  --set resources.requests.cpu=1 \
  --set resources.requests.memory=2Gi \
  --wait --timeout 5m

# ── Frontend ──
echo "[3/3] Deploying Frontend..."
helm upgrade --install minicc-frontend ./deploy/helm/frontend \
  --namespace "$NAMESPACE" \
  --set replicaCount=2 \
  --set image.repository="${REGISTRY}-frontend" \
  --set image.tag="$TAG" \
  --set image.pullPolicy=Always \
  --set resources.requests.cpu=100m \
  --set resources.requests.memory=128Mi \
  --wait --timeout 3m

echo "=========================================="
echo "  MiniCC 应用部署完成"
echo "  Gateway: minicc-gateway.$NAMESPACE:8080"
echo "  Python:  minicc-python.$NAMESPACE:8000"
echo "  Frontend: minicc-frontend.$NAMESPACE:80"
echo "=========================================="
echo ""
echo "查看部署状态:"
echo "  kubectl get pods -n $NAMESPACE -w"
echo "  kubectl get hpa -n $NAMESPACE"
echo "  kubectl logs -l app=minicc-gateway -n $NAMESPACE --tail=50"
