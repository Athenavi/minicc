#!/bin/bash
# 生成 Go 和 Python 的 gRPC 代码
# 使用方法: bash proto/generate.sh

set -e

echo "=== 生成 Go gRPC 代码 ==="
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/common.proto proto/agent.proto proto/rag.proto proto/memory.proto proto/knowledge.proto

echo "=== 生成 Python gRPC 代码 ==="
mkdir -p python-engine/app/grpc
python -m grpc_tools.protoc -I proto \
       --python_out=python-engine/app/grpc \
       --grpc_python_out=python-engine/app/grpc \
       proto/common.proto proto/agent.proto proto/rag.proto proto/memory.proto proto/knowledge.proto

# 创建 __init__.py
touch python-engine/app/grpc/__init__.py

echo "=== 代码生成完成 ==="
