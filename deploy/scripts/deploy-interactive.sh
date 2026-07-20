#!/bin/bash
# =============================================================================
# MiniCC 交互式云部署向导
# 版本: 3.0.0
#
# 使用方法:
#   chmod +x deploy-interactive.sh
#   ./deploy-interactive.sh
#
# 交互流程:
#   步骤 1: 选择云平台
#   步骤 2: 数据库 (PostgreSQL) 配置
#   步骤 3: 缓存 (Redis) 配置
#   步骤 4: 对象存储配置
#   步骤 5: 镜像仓库配置
#   步骤 6: 监控与日志配置
#   步骤 7: 确认并执行
#
# 导航:
#   [n] 下一步   [b] 上一步   [r] 重置   [c] 取消   [q] 退出
# =============================================================================
set -euo pipefail

# ── 颜色 ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ── 全局状态 ──────────────────────────────────────────────────────────────
STEP=0
TOTAL_STEPS=7
declare -A CFG  # 配置存储

# 步骤标题
STEPS=(
    "选择云平台"
    "数据库 (PostgreSQL)"
    "缓存 (Redis)"
    "对象存储"
    "镜像仓库"
    "监控与日志"
    "确认并执行"
)

# ── 工具函数 ──────────────────────────────────────────────────────────────

clear_screen() { printf "\033c"; }

print_banner() {
    echo -e "${CYAN}${BOLD}"
    echo "  ╔══════════════════════════════════════════════════╗"
    echo "  ║           MiniCC 企业级云部署向导 v3.0           ║"
    echo "  ╠══════════════════════════════════════════════════╣"
    echo "  ║  步骤 $STEP/$TOTAL_STEPS: ${STEPS[$((STEP-1))]}"
    echo "  ╚══════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_nav() {
    echo ""
    echo -e "${BOLD}导航:${NC} ${GREEN}[n]${NC} 下一步  ${YELLOW}[b]${NC} 上一步  ${RED}[r]${NC} 重置  ${RED}[c]${NC} 取消  ${BLUE}[q]${NC} 退出"
    echo -ne "${BOLD}> ${NC}"
}

read_nav() {
    local choice
    read -r choice
    case "$choice" in
        n|N|"")  return 0 ;;   # 下一步
        b|B)     STEP=$((STEP-2)); return 1 ;;  # 上一步
        r|R)     STEP=-1; return 2 ;;  # 重置（后面会 STEP++）
        c|C)     echo -e "${YELLOW}部署已取消。${NC}"; exit 0 ;;
        q|Q)     echo -e "${YELLOW}退出向导。${NC}"; exit 0 ;;
        *)       echo -e "${RED}无效输入，请选择 n/b/r/c/q${NC}"; return 3 ;;
    esac
}

select_option() {
    local prompt="$1"; shift
    local options=("$@")
    local len=${#options[@]}

    echo -e "${BOLD}${prompt}${NC}"
    for i in $(seq 0 $((len-1))); do
        echo "  $((i+1)). ${options[$i]}"
    done

    while true; do
        echo -ne "${BOLD}请输入选项编号 (1-$len) [n/b/r/c/q]: ${NC}"
        read -r input
        case "$input" in
            [nN]) return 254 ;;
            [bB]) return 253 ;;
            [rR]) return 252 ;;
            [cC]) echo -e "${YELLOW}部署已取消。${NC}"; exit 0 ;;
            [qQ]) echo -e "${YELLOW}退出向导。${NC}"; exit 0 ;;
            *)
                if [[ "$input" =~ ^[0-9]+$ ]] && [ "$input" -ge 1 ] && [ "$input" -le "$len" ]; then
                    return $((input-1))
                fi
                echo -e "${RED}无效输入，请输入 1-$len 或 n/b/r/c/q${NC}"
                ;;
        esac
    done
}

input_text() {
    local prompt="$1" default_val="${2:-}"
    echo -e "${BOLD}${prompt}${NC}"
    echo -ne "  默认: ${default_val}\n  输入: "
    read -r input
    if [ -z "$input" ]; then
        echo "$default_val"
    else
        echo "$input"
    fi
}

confirm_yn() {
    local prompt="$1" default="${2:-y}"
    local default_display="Y/n"
    [ "$default" = "n" ] && default_display="y/N"

    echo -ne "${BOLD}${prompt} (${default_display}) [n/b/r/c/q]: ${NC}"
    read -r input
    case "$input" in
        [nN])      return 1 ;;
        [bB])      return 253 ;;
        [rR])      return 252 ;;
        [cC])      echo -e "${YELLOW}部署已取消。${NC}"; exit 0 ;;
        [qQ])      echo -e "${YELLOW}退出向导。${NC}"; exit 0 ;;
        ""|y|Y)    [ "$default" = "y" ] && return 0 || return 1 ;;
        *)         [ "$default" = "y" ] && return 0 || return 1 ;;
    esac
}

# ── 步骤函数 ──────────────────────────────────────────────────────────────

step_1_select_cloud() {
    clear_screen
    print_banner
    echo "  选择目标云平台："
    echo ""
    select_option "请选择云平台:" \
        "AWS" \
        "GCP (Google Cloud)" \
        "Azure" \
        "阿里云" \
        "自建机房 (Bare Metal / vSphere)" \
        "无 (仅生成 Helm 命令，不管理基础设施)"

    local rc=$?
    case $rc in
        0) CFG[cloud]="aws";       CFG[cloud_name]="AWS" ;;
        1) CFG[cloud]="gcp";       CFG[cloud_name]="GCP" ;;
        2) CFG[cloud]="azure";     CFG[cloud_name]="Azure" ;;
        3) CFG[cloud]="alicloud";  CFG[cloud_name]="阿里云" ;;
        4) CFG[cloud]="baremetal"; CFG[cloud_name]="自建机房" ;;
        5) CFG[cloud]="none";      CFG[cloud_name]="无" ;;
        254) return 0 ;;  # next
        253) STEP=-2; return 1 ;;  # back
        252) return 2 ;;  # reset
    esac
    CFG[cloud_managed]="no"
}

step_2_database() {
    clear_screen
    print_banner
    echo -e "  当前云平台: ${BOLD}${CFG[cloud_name]}${NC}"
    echo ""

    if [ "${CFG[cloud]}" = "none" ]; then
        CFG[db_type]="manual"
        CFG[db_desc]="手动配置 PostgreSQL (用户自行准备)"
        return 0
    fi

    select_option "请选择 PostgreSQL 部署方式:" \
        "使用云托管数据库 (推荐: RDS/Cloud SQL/ Azure DB/阿里云 RDS)" \
        "在 K8s 集群内自建 (Bitnami Helm Chart)" \
        "手动配置 (使用已有数据库连接)"

    local rc=$?
    case $rc in
        0)
            CFG[db_type]="managed"
            case "${CFG[cloud]}" in
                aws)      CFG[db_desc]="AWS RDS for PostgreSQL" ;;
                gcp)      CFG[db_desc]="GCP Cloud SQL for PostgreSQL" ;;
                azure)    CFG[db_desc]="Azure Database for PostgreSQL" ;;
                alicloud) CFG[db_desc]="阿里云 RDS PostgreSQL" ;;
                baremetal) CFG[db_desc]="自建 PostgreSQL" ;;
            esac
            # 如果使用托管服务，需要输入 DSN
            echo ""
            CFG[db_dsn]=$(input_text "请输入 PostgreSQL 连接串 (留空使用默认格式)" \
                "postgres://minicc:PASSWORD@postgres-${CFG[cloud]}:5432/minicc")
            ;;
        1)
            CFG[db_type]="helm"
            CFG[db_desc]="Helm Bitnami PostgreSQL (K8s 内自建)"
            CFG[db_dsn]="postgres://minicc:\${POSTGRES_PASSWORD}@postgres-primary:5432/minicc"
            CFG[db_storage_class]=$(input_text "请输入存储类名称 (云平台默认/留空)" "gp3")
            CFG[db_storage_size]=$(input_text "请输入存储大小 (Gi)" "100")
            ;;
        2)
            CFG[db_type]="manual"
            CFG[db_desc]="手动配置 PostgreSQL"
            CFG[db_dsn]=$(input_text "请输入 PostgreSQL 连接串" "")
            ;;
        254) return 0 ;;
        253) STEP=$((STEP-2)); return 1 ;;
        252) return 2 ;;
    esac
}

step_3_cache() {
    clear_screen
    print_banner
    echo -e "  当前云平台: ${BOLD}${CFG[cloud_name]}${NC}  |  数据库: ${BOLD}${CFG[db_desc]}${NC}"
    echo ""

    if [ "${CFG[cloud]}" = "none" ]; then
        CFG[cache_type]="manual"
        CFG[cache_desc]="手动配置 Redis"
        return 0
    fi

    select_option "请选择 Redis 部署方式:" \
        "使用云托管缓存 (推荐: ElastiCache/Memorystore/ Azure Cache/阿里云 Redis)" \
        "在 K8s 集群内自建 (Bitnami Helm Chart)" \
        "手动配置 (使用已有 Redis)"

    local rc=$?
    case $rc in
        0)
            CFG[cache_type]="managed"
            case "${CFG[cloud]}" in
                aws)      CFG[cache_desc]="AWS ElastiCache for Redis" ;;
                gcp)      CFG[cache_desc]="GCP Memorystore for Redis" ;;
                azure)    CFG[cache_desc]="Azure Cache for Redis" ;;
                alicloud) CFG[cache_desc]="阿里云云数据库 Redis" ;;
                baremetal) CFG[cache_desc]="自建 Redis" ;;
            esac
            CFG[cache_addr]=$(input_text "请输入 Redis 地址 (留空使用默认)" \
                "redis-master:6379")
            CFG[cache_password]=$(input_text "请输入 Redis 密码 (留空无密码)" "")
            ;;
        1)
            CFG[cache_type]="helm"
            CFG[cache_desc]="Helm Bitnami Redis (K8s 内自建)"
            ;;
        2)
            CFG[cache_type]="manual"
            CFG[cache_desc]="手动配置 Redis"
            CFG[cache_addr]=$(input_text "请输入 Redis 地址" "")
            CFG[cache_password]=$(input_text "请输入 Redis 密码 (留空无密码)" "")
            ;;
        254) return 0 ;;
        253) STEP=$((STEP-2)); return 1 ;;
        252) return 2 ;;
    esac
}

step_4_storage() {
    clear_screen
    print_banner
    echo -e "  当前云平台: ${BOLD}${CFG[cloud_name]}${NC}"
    echo ""

    if [ "${CFG[cloud]}" = "none" ]; then
        CFG[storage_type]="manual"
        CFG[storage_desc]="手动配置对象存储"
        return 0
    fi

    select_option "请选择对象存储部署方式:" \
        "使用云对象存储 (推荐: S3/GCS/Azure Blob/阿里云 OSS)" \
        "在 K8s 集群内自建 (MinIO Helm Chart)" \
        "本地文件系统 (仅开发/测试)"

    local rc=$?
    case $rc in
        0)
            CFG[storage_type]="managed"
            case "${CFG[cloud]}" in
                aws)      CFG[storage_desc]="AWS S3";       CFG[storage_endpoint]="" ;;
                gcp)      CFG[storage_desc]="GCP GCS";      CFG[storage_endpoint]="https://storage.googleapis.com" ;;
                azure)    CFG[storage_desc]="Azure Blob";   CFG[storage_endpoint]="https://ACCOUNT.blob.core.windows.net" ;;
                alicloud) CFG[storage_desc]="阿里云 OSS";   CFG[storage_endpoint]="https://oss-REGION.aliyuncs.com" ;;
                baremetal) CFG[storage_desc]="自建 MinIO";  CFG[storage_endpoint]=$(input_text "请输入 MinIO Endpoint" "http://minio:9000") ;;
            esac
            CFG[storage_bucket]=$(input_text "请输入存储桶名称" "minicc-media")
            if confirm_yn "是否使用 IAM Role 替代静态凭证？" "y"; then
                CFG[storage_iam]="yes"
            else
                CFG[storage_iam]="no"
                CFG[storage_access_key]=$(input_text "请输入 Access Key" "")
                CFG[storage_secret_key]=$(input_text "请输入 Secret Key" "")
            fi
            ;;
        1)
            CFG[storage_type]="helm"
            CFG[storage_desc]="Helm MinIO (K8s 内自建)"
            CFG[storage_storage_class]=$(input_text "请输入存储类名称" "${CFG[db_storage_class]:-gp3}")
            ;;
        2)
            CFG[storage_type]="local"
            CFG[storage_desc]="本地文件系统"
            CFG[storage_root]=$(input_text "请输入存储根目录" "./workspace")
            ;;
        254) return 0 ;;
        253) STEP=$((STEP-2)); return 1 ;;
        252) return 2 ;;
    esac
}

step_5_registry() {
    clear_screen
    print_banner
    echo ""

    select_option "请选择容器镜像仓库:" \
        "使用云平台镜像仓库 (ECR/GCR/ACR/阿里云 ACR)" \
        "使用自建 Harbor / Docker Registry" \
        "使用 Docker Hub (开发/测试)"

    local rc=$?
    case $rc in
        0)
            CFG[registry_type]="cloud"
            case "${CFG[cloud]}" in
                aws)      CFG[registry_url]=$(input_text "请输入 ECR 仓库 URL" "ACCOUNT.dkr.ecr.REGION.amazonaws.com/minicc") ;;
                gcp)      CFG[registry_url]=$(input_text "请输入 GCR 仓库 URL" "gcr.io/PROJECT/minicc") ;;
                azure)    CFG[registry_url]=$(input_text "请输入 ACR 仓库 URL" "ACR.azurecr.io/minicc") ;;
                alicloud) CFG[registry_url]=$(input_text "请输入 ACR 仓库 URL" "REGISTRY-vpc.cn-hangzhou.cr.aliyuncs.com/minicc") ;;
                *)        CFG[registry_url]=$(input_text "请输入镜像仓库 URL" "registry.example.com/minicc") ;;
            esac
            if confirm_yn "是否使用 IAM Role 进行镜像拉取认证？" "y"; then
                CFG[registry_iam]="yes"
            else
                CFG[registry_iam]="no"
                CFG[registry_user]=$(input_text "请输入镜像仓库用户名" "AWS")
                CFG[registry_pass]=$(input_text "请输入镜像仓库密码/密钥" "")
            fi
            ;;
        1)
            CFG[registry_type]="self"
            CFG[registry_url]=$(input_text "请输入镜像仓库 URL" "harbor.example.com/minicc")
            CFG[registry_user]=$(input_text "请输入镜像仓库用户名" "admin")
            CFG[registry_pass]=$(input_text "请输入镜像仓库密码" "")
            ;;
        2)
            CFG[registry_type]="dockerhub"
            CFG[registry_url]="docker.io/minicc"
            ;;
        254) return 0 ;;
        253) STEP=$((STEP-2)); return 1 ;;
        252) return 2 ;;
    esac
}

step_6_monitoring() {
    clear_screen
    print_banner
    echo ""

    select_option "请选择监控与日志方案:" \
        "使用云平台托管监控 (推荐: AWS AMP + Grafana / GCP Cloud Monitoring)" \
        "在 K8s 内自建 (Prometheus + Loki + Grafana)" \
        "不部署 (仅运行应用)"

    local rc=$?
    case $rc in
        0)
            CFG[monitor_type]="managed"
            CFG[monitor_desc]="云托管监控"
            case "${CFG[cloud]}" in
                aws)
                    CFG[amp_workspace]=$(input_text "请输入 AMP (Amazon Managed Prometheus) Workspace ID" "")
                    CFG[grafana_url]=$(input_text "请输入 Grafana 工作区 URL" "")
                    ;;
                gcp)
                    echo -e "${GREEN}GCP Cloud Monitoring 默认集成，无需额外配置${NC}"
                    CFG[monitor_desc]="GCP Cloud Monitoring"
                    ;;
                azure)
                    echo -e "${GREEN}Azure Monitor 默认集成，无需额外配置${NC}"
                    CFG[monitor_desc]="Azure Monitor"
                    ;;
                *)
                    CFG[monitor_desc]="自建监控"
                    CFG[monitor_type]="self"
                    ;;
            esac
            ;;
        1)
            CFG[monitor_type]="self"
            CFG[monitor_desc]="自建 Prometheus + Loki + Grafana"
            CFG[grafana_password]=$(input_text "请输入 Grafana admin 密码" "admin123")
            ;;
        2)
            CFG[monitor_type]="none"
            CFG[monitor_desc]="不部署监控"
            ;;
        254) return 0 ;;
        253) STEP=$((STEP-2)); return 1 ;;
        252) return 2 ;;
    esac
}

step_7_confirm() {
    clear_screen
    echo -e "${CYAN}${BOLD}"
    echo "  ╔══════════════════════════════════════════════════╗"
    echo "  ║           部署配置确认                           ║"
    echo "  ╚══════════════════════════════════════════════════╝"
    echo -e "${NC}"

    echo -e "${BOLD}┌─ 云平台${NC}"
    echo -e "│  ${GREEN}${CFG[cloud_name]}${NC}"

    echo -e "${BOLD}├─ 数据库${NC}"
    echo -e "│  类型: ${GREEN}${CFG[db_desc]}${NC}"
    echo -e "│  DSN:  ${CFG[db_dsn]:-(待配置)}"

    echo -e "${BOLD}├─ 缓存${NC}"
    echo -e "│  类型: ${GREEN}${CFG[cache_desc]}${NC}"
    echo -e "│  地址: ${CFG[cache_addr]:-(待配置)}"

    echo -e "${BOLD}├─ 对象存储${NC}"
    echo -e "│  类型: ${GREEN}${CFG[storage_desc]}${NC}"
    echo -e "│  桶:   ${CFG[storage_bucket]:-(待配置)}"

    echo -e "${BOLD}├─ 镜像仓库${NC}"
    echo -e "│  URL:  ${CFG[registry_url]:-(未配置)}"

    echo -e "${BOLD}├─ 监控${NC}"
    echo -e "│  ${GREEN}${CFG[monitor_desc]}${NC}"

    echo -e "${BOLD}└─${NC}"
    echo ""

    if ! confirm_yn "确认以上配置并开始部署？" "y"; then
        return 1  # 回到上一步
    fi

    # 执行部署
    do_deploy
}

# ── 部署执行 ──────────────────────────────────────────────────────────────

do_deploy() {
    echo ""
    echo -e "${GREEN}${BOLD}开始部署 MiniCC ...${NC}"
    echo ""

    # 1. 基础设施
    if [ "${CFG[cloud]}" != "none" ]; then
        echo -e "${CYAN}[1/4] 部署基础设施 ...${NC}"

        if [ "${CFG[db_type]}" = "helm" ]; then
            helm upgrade --install postgres bitnami/postgresql \
                --namespace minicc-infra --create-namespace \
                --set auth.database=minicc,auth.username=minicc \
                ${CFG[db_storage_class]:+--set primary.persistence.storageClass="${CFG[db_storage_class]}"} \
                --set primary.persistence.size="${CFG[db_storage_size]:-100}Gi" \
                --wait --timeout 10m
            echo -e "  ${GREEN}✓${NC} PostgreSQL 部署完成"
        fi

        if [ "${CFG[cache_type]}" = "helm" ]; then
            helm upgrade --install redis bitnami/redis \
                --namespace minicc-infra \
                --set architecture=replication \
                --set replica.replicaCount=3 \
                --wait --timeout 5m
            echo -e "  ${GREEN}✓${NC} Redis 部署完成"
        fi

        if [ "${CFG[storage_type]}" = "helm" ]; then
            helm upgrade --install minio bitnami/minio \
                --namespace minicc-infra \
                --set persistence.size=500Gi \
                --set defaultBuckets="${CFG[storage_bucket]:-minicc-media}" \
                --wait --timeout 5m
            echo -e "  ${GREEN}✓${NC} MinIO 部署完成"
        fi
    fi

    # 2. 镜像仓库认证
    if [ "${CFG[registry_iam]:-no}" = "no" ] && [ -n "${CFG[registry_user]:-}" ]; then
        echo -e "${CYAN}[2/4] 配置镜像仓库认证 ...${NC}"
        kubectl create secret docker-registry minicc-registry \
            --namespace minicc \
            --docker-server="${CFG[registry_url]}" \
            --docker-username="${CFG[registry_user]}" \
            --docker-password="${CFG[registry_pass]}" \
            --dry-run=client -o yaml | kubectl apply -f -
        echo -e "  ${GREEN}✓${NC} 镜像仓库认证配置完成"
    fi

    # 3. 部署应用
    echo -e "${CYAN}[3/4] 部署应用组件 ...${NC}"
    
    # Go Gateway
    helm upgrade --install minicc-gateway ./deploy/helm/gateway \
        --namespace minicc --create-namespace \
        --set replicaCount=2 \
        --set image.repository="${CFG[registry_url]:+${CFG[registry_url]}-gateway}" \
        ${CFG[db_dsn]:+--set config.postgresDsn="${CFG[db_dsn]}"} \
        --wait --timeout 5m
    echo -e "  ${GREEN}✓${NC} Go Gateway 部署完成"

    # Python Engine
    helm upgrade --install minicc-python ./deploy/helm/python-engine \
        --namespace minicc \
        --set replicaCount=2 \
        --set image.repository="${CFG[registry_url]:+${CFG[registry_url]}-python}" \
        ${CFG[db_dsn]:+--set config.postgresDsn="${CFG[db_dsn]}"} \
        --wait --timeout 5m
    echo -e "  ${GREEN}✓${NC} Python Engine 部署完成"

    # Frontend
    helm upgrade --install minicc-frontend ./deploy/helm/frontend \
        --namespace minicc \
        --set replicaCount=2 \
        --set image.repository="${CFG[registry_url]:+${CFG[registry_url]}-frontend}" \
        --wait --timeout 3m
    echo -e "  ${GREEN}✓${NC} Frontend 部署完成"

    # 4. 监控
    if [ "${CFG[monitor_type]}" = "self" ]; then
        echo -e "${CYAN}[4/4] 部署监控栈 ...${NC}"
        helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
            --namespace minicc-monitoring --create-namespace \
            --set grafana.adminPassword="${CFG[grafana_password]:-admin123}" \
            --wait --timeout 10m
        echo -e "  ${GREEN}✓${NC} 监控栈部署完成"
    fi

    # ── 完成 ──
    echo ""
    echo -e "${GREEN}${BOLD}========================================${NC}"
    echo -e "${GREEN}${BOLD}  MiniCC 部署成功！${NC}"
    echo -e "${GREEN}${BOLD}========================================${NC}"
    echo ""
    echo "  查看状态:"
    echo "    kubectl get pods -n minicc -w"
    echo "    kubectl get svc -n minicc"
    echo ""
    echo "  查看日志:"
    echo "    kubectl logs -l app=minicc-gateway -n minicc --tail=50"
    echo "    kubectl logs -l app=minicc-python -n minicc --tail=50"
    echo ""
}

# ── 主循环 ────────────────────────────────────────────────────────────────

main() {
    # 检查依赖
    for cmd in helm kubectl; do
        if ! command -v "$cmd" &>/dev/null; then
            echo -e "${RED}错误: 未找到 $cmd 命令，请先安装。${NC}"
            exit 1
        fi
    done

    # 重置配置
    reset_config() {
        for key in "${!CFG[@]}"; do unset CFG["$key"]; done
        CFG[cloud]="none"
        CFG[cloud_name]="未选择"
        CFG[db_type]="manual"
        CFG[db_desc]="待配置"
        CFG[cache_type]="manual"
        CFG[cache_desc]="待配置"
        CFG[storage_type]="local"
        CFG[storage_desc]="待配置"
        CFG[registry_type]="dockerhub"
        CFG[registry_url]=""
        CFG[monitor_type]="none"
        CFG[monitor_desc]="不部署"
    }
    reset_config

    # 交互循环
    while true; do
        STEP=$((STEP+1))

        case $STEP in
            1) step_1_select_cloud; rc=$? ;;
            2) step_2_database;     rc=$? ;;
            3) step_3_cache;        rc=$? ;;
            4) step_4_storage;      rc=$? ;;
            5) step_5_registry;     rc=$? ;;
            6) step_6_monitoring;   rc=$? ;;
            7) step_7_confirm;      rc=$? ;;
            *)
                if [ $STEP -gt $TOTAL_STEPS ]; then
                    break
                fi
                ;;
        esac

        # 处理导航返回码
        case ${rc:-0} in
            0)  ;;  # 正常下一步
            1)  ;;  # back 已在函数中处理 STEP
            2)  reset_config; STEP=0 ;;  # reset
            3)  STEP=$((STEP-1)) ;;  # 无效输入，重试当前步骤
            254) ;;  # next
            253) STEP=$((STEP-2)) ;;  # back
            252) reset_config; STEP=0 ;;  # reset
        esac

        # 确保 STEP 不越界
        if [ $STEP -lt 0 ]; then STEP=0; fi
    done
}

main "$@"
