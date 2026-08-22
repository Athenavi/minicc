# Backend - Multi-stage build
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/minicc ./cmd/minicc/
RUN CGO_ENABLED=0 go build -o /build/migrate ./cmd/migrate/

FROM alpine:3.20
# S 安全加固：安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata wget
# S 安全加固：非 root 用户运行
RUN addgroup -g 1001 -S appgroup && adduser -u 1001 -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /build/minicc /app/
COPY --from=builder /build/migrate /app/
COPY --from=builder /build/migrations /app/migrations/
# 创建 workspace 目录并设置权限
RUN mkdir -p /app/workspace /app/data/plugins && chown -R appuser:appgroup /app
USER appuser
EXPOSE 8080
# S 安全加固：健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1
CMD ["./minicc"]
