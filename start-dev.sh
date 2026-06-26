#!/bin/bash

# SQL panel 开发环境一键启动脚本
# 同时启动 Go 后端 (8080) 和 Vite 前端 (5173)
# Vite 会自动将 /sqlpanel/api 请求代理到 Go 后端
#
# 环境变量:
#   ENCRYPT=true      启用 API 加密传输 (Go 后端读取 os.Getenv("ENCRYPT"))
#   VITE_ENCRYPT=true  启用前端加密 (App.vue 读取 import.meta.env.VITE_ENCRYPT)
#
# 用法:
#   ./start-dev.sh              # 不加密（默认）
#   ENCRYPT=true ./start-dev.sh # 启用加密
#   ./start-dev.sh --encrypt    # 启用加密

set -e

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 解析命令行参数
ENCRYPT="${ENCRYPT:-false}"
for arg in "$@"; do
    case "$arg" in
        --encrypt)
            ENCRYPT="true"
            ;;
    esac
done

# 前端通过 VITE_ 前缀读取，后端直接读 ENCRYPT
export ENCRYPT
export VITE_ENCRYPT="$ENCRYPT"

cleanup() {
    echo ""
    echo "正在关闭服务..."
    if [ -n "$GO_PID" ] && kill -0 "$GO_PID" 2>/dev/null; then
        kill -9 "$GO_PID" 2>/dev/null
        wait "$GO_PID" 2>/dev/null
        echo "Go 后端已关闭"
    fi
    exit 0
}

trap cleanup SIGINT SIGTERM EXIT

echo "================================="
echo "  SQL panel 开发环境"
if [ "$ENCRYPT" = "true" ]; then
    echo "  API 加密: 已启用"
fi
echo "================================="
echo ""

# 先清理可能残留的 8080 端口进程
EXISTING_PID=$(lsof -ti :8080 2>/dev/null || true)
if [ -n "$EXISTING_PID" ]; then
    echo "发现端口 8080 被占用 (PID: $EXISTING_PID)，正在释放..."
    kill -9 $EXISTING_PID 2>/dev/null || true
    sleep 1
fi

# 启动 Go 后端
echo "[1/2] 启动 Go 后端 (端口 8080)..."
cd "$PROJECT_DIR/server"
go run . &
GO_PID=$!
sleep 2

if ! kill -0 "$GO_PID" 2>/dev/null; then
    echo "错误: Go 后端启动失败"
    exit 1
fi
echo "Go 后端已启动 (PID: $GO_PID)"

# 启动 Vite 前端
echo "[2/2] 启动 Vite 前端 (端口 5173)..."
cd "$PROJECT_DIR/web"
echo ""
echo "================================="
echo "  前端: http://localhost:5173"
echo "  后端: http://localhost:8080"
echo "  按 Ctrl+C 停止所有服务"
echo "================================="
echo ""

npm run dev

cleanup