#!/usr/bin/env bash
# ============================================================
# MiniCC 一键启动脚本 (Linux)
# ============================================================
# 用法:
#   ./scripts/start_linux.sh            后台启动所有服务
#   ./scripts/start_linux.sh --fg       前台启动（调试用）
#   ./scripts/start_linux.sh setup      首次安装依赖并启动
#   ./scripts/start_linux.sh status     查看服务状态
#   ./scripts/start_linux.sh stop       停止所有服务
#   ./scripts/start_linux.sh restart    重启所有服务
# ============================================================

set -euo pipefail

# ── 颜色定义 ─────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ── 路径设置 ─────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

echo ""
echo -e "${BOLD}╔══════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       MiniCC 一键启动 (Linux)        ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""

# ── 检查 Python ──────────────────────────────────────────────
PYTHON_CMD=""
if command -v python3 &>/dev/null; then
    PYTHON_CMD="python3"
elif command -v python &>/dev/null; then
    PYTHON_CMD="python"
else
    echo -e "${RED}[错误]${NC} 未找到 Python，请先安装 Python 3.9+"
    echo "        Ubuntu/Debian: sudo apt install python3 python3-pip"
    echo "        CentOS/RHEL:   sudo yum install python3 python3-pip"
    echo "        Arch:          sudo pacman -S python python-pip"
    exit 1
fi

PY_VER=$($PYTHON_CMD --version 2>&1)
echo -e "[信息] ${GREEN}${PY_VER}${NC}"

# ── 检查 Go（仅 setup/build/start 需要）──────────────────────
CMD="${1:-start}"

check_go() {
    if ! command -v go &>/dev/null; then
        echo -e "${RED}[错误]${NC} 未找到 Go，请先安装 Go 1.21+"
        echo "        下载地址: https://go.dev/dl/"
        exit 1
    fi
    GO_VER=$(go version 2>&1)
    echo -e "[信息] ${GREEN}${GO_VER}${NC}"
}

check_go_optional() {
    if ! command -v go &>/dev/null; then
        echo -e "${YELLOW}[警告]${NC} 未找到 Go，将跳过编译（如需编译请安装 Go 1.21+）"
        echo ""
    else
        GO_VER=$(go version 2>&1)
        echo -e "[信息] ${GREEN}${GO_VER}${NC}"
    fi
}

case "${CMD}" in
    setup)
        check_go
        ;;
    start|--fg)
        check_go_optional
        ;;
esac

# ── 执行 run.py ──────────────────────────────────────────────
run_cmd() {
    echo ""
    $PYTHON_CMD run.py "$@"
    return $?
}

EXIT_CODE=0

if [[ -z "${1:-}" ]]; then
    # 无参数：默认后台启动
    run_cmd start
    EXIT_CODE=$?
elif [[ "$1" == "--fg" ]]; then
    run_cmd start --fg
    EXIT_CODE=$?
elif [[ "$1" == "setup" ]]; then
    echo ""
    echo -e "${BOLD}[步骤 1/2]${NC} 安装依赖..."
    $PYTHON_CMD run.py setup
    if [[ $? -ne 0 ]]; then
        echo -e "${RED}[错误]${NC} 依赖安装失败"
        exit 1
    fi
    echo ""
    echo -e "${BOLD}[步骤 2/2]${NC} 启动服务..."
    run_cmd start
    EXIT_CODE=$?
else
    run_cmd "$@"
    EXIT_CODE=$?
fi

echo ""
if [[ $EXIT_CODE -eq 0 ]]; then
    echo -e "${GREEN}[完成]${NC} MiniCC 已启动"
    echo "        停止服务: ./scripts/start_linux.sh stop"
    echo "        查看状态: ./scripts/start_linux.sh status"
    echo "        查看日志: $PYTHON_CMD run.py logs"
else
    echo -e "${RED}[错误]${NC} 启动失败，请检查日志"
fi

exit $EXIT_CODE
