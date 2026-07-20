#!/usr/bin/env bash
# ============================================================
# MiniCC 一键启动脚本 (macOS)
# ============================================================
# 用法:
#   ./scripts/start_macos.sh            后台启动所有服务
#   ./scripts/start_macos.sh --fg       前台启动（调试用）
#   ./scripts/start_macos.sh setup      首次安装依赖并启动
#   ./scripts/start_macos.sh status     查看服务状态
#   ./scripts/start_macos.sh stop       停止所有服务
#   ./scripts/start_macos.sh restart    重启所有服务
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
echo -e "${BOLD}║       MiniCC 一键启动 (macOS)        ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""

# ── 检查系统依赖 ─────────────────────────────────────────────
# macOS 上 Xcode Command Line Tools 提供了必要的编译工具
check_xcode_tools() {
    if ! xcode-select -p &>/dev/null; then
        echo -e "${YELLOW}[提示]${NC} 未检测到 Xcode Command Line Tools"
        echo "        部分功能（如 Go 编译）可能需要它"
        echo "        安装命令: xcode-select --install"
        echo ""
    fi
}

check_xcode_tools

# ── 检查 Python ──────────────────────────────────────────────
# macOS 系统自带的 Python 可能版本较低，优先查找 Homebrew 安装的版本
PYTHON_CMD=""

# 优先查找 python3
if command -v python3 &>/dev/null; then
    PYTHON_CMD="python3"
elif command -v python &>/dev/null; then
    PYTHON_CMD="python"
else
    echo -e "${RED}[错误]${NC} 未找到 Python，请先安装 Python 3.9+"
    echo ""
    echo "  推荐使用 Homebrew 安装:"
    echo "    /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
    echo "    brew install python@3.12"
    echo ""
    echo "  或从官网下载: https://www.python.org/downloads/macos/"
    exit 1
fi

PY_VER=$($PYTHON_CMD --version 2>&1)
echo -e "[信息] ${GREEN}${PY_VER}${NC}"

# ── 检查 Go（仅 setup/build/start 需要）──────────────────────
CMD="${1:-start}"

check_go() {
    if ! command -v go &>/dev/null; then
        echo -e "${RED}[错误]${NC} 未找到 Go，请先安装 Go 1.21+"
        echo ""
        echo "  推荐使用 Homebrew 安装:"
        echo "    brew install go"
        echo ""
        echo "  或从官网下载: https://go.dev/dl/"
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
    echo "        停止服务: ./scripts/start_macos.sh stop"
    echo "        查看状态: ./scripts/start_macos.sh status"
    echo "        查看日志: $PYTHON_CMD run.py logs"
else
    echo -e "${RED}[错误]${NC} 启动失败，请检查日志"
fi

exit $EXIT_CODE
