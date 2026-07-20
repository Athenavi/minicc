#!/bin/bash
# ============================================================================
# MiniCC 监控栈部署脚本
# 版本: 3.0.0
# 用法: ./deploy-monitoring.sh <grafana-password>
# ============================================================================
set -euo pipefail

GRAFANA_PASSWORD="${1:?Usage: $0 <grafana-password>}"
NAMESPACE="minicc-monitoring"

echo "=========================================="
echo "  MiniCC 监控栈部署"
echo "=========================================="

# ── Prometheus Stack (Prometheus + Grafana + AlertManager) ──
echo "[1/3] Deploying Prometheus Stack..."
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --namespace "$NAMESPACE" --create-namespace \
  --set grafana.enabled=true \
  --set grafana.adminPassword="$GRAFANA_PASSWORD" \
  --set grafana.persistence.enabled=true \
  --set grafana.persistence.size=10Gi \
  --set alertmanager.enabled=true \
  --set alertmanager.persistence.enabled=true \
  --wait --timeout 10m

# ── Loki + Promtail (日志聚合) ──
echo "[2/3] Deploying Loki + Promtail..."
helm upgrade --install loki grafana/loki-stack \
  --namespace "$NAMESPACE" \
  --set promtail.enabled=true \
  --set loki.persistence.enabled=true \
  --set loki.persistence.size=50Gi \
  --wait --timeout 5m

# ── 导入 MiniCC 告警规则 ──
echo "[3/3] Applying alerting rules..."
kubectl apply -f - << 'EOF'
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: minicc-alerts
  namespace: minicc-monitoring
spec:
  groups:
    - name: minicc
      rules:
        - alert: HighErrorRate
          expr: rate(minicc_http_requests_total{status=~"5.."}[5m]) / rate(minicc_http_requests_total[5m]) > 0.05
          for: 5m
          labels: { severity: critical }
          annotations:
            summary: "MiniCC 错误率超过 5%"
        - alert: HighLLMLatency
          expr: histogram_quantile(0.95, rate(minicc_llm_request_duration_ms_bucket[5m])) > 10000
          for: 5m
          labels: { severity: warning }
          annotations:
            summary: "LLM P95 延迟超过 10 秒"
        - alert: CircuitBreakerOpen
          expr: minicc_circuit_breaker_state > 0
          for: 1m
          labels: { severity: critical }
          annotations:
            summary: "LLM Provider 熔断器已打开"
        - alert: CacheHitRateDrop
          expr: minicc_cache_hit_ratio < 0.5
          for: 10m
          labels: { severity: warning }
          annotations:
            summary: "语义缓存命中率低于 50%"
EOF

echo "=========================================="
echo "  MiniCC 监控栈部署完成"
echo "  Prometheus:  kube-prometheus-stack-prometheus.$NAMESPACE:9090"
echo "  Grafana:     kube-prometheus-stack-grafana.$NAMESPACE:80"
echo "  AlertManager: kube-prometheus-stack-alertmanager.$NAMESPACE:9093"
echo "  Loki:        loki.$NAMESPACE:3100"
echo "=========================================="
