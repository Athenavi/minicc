# Backend - Multi-stage build
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/chiron ./cmd/chiron/
RUN CGO_ENABLED=0 go build -o /build/migrate ./cmd/migrate/

FROM alpine:3.20
# S 瀹夊叏鍔犲浐锛氬畨瑁呰繍琛屾椂渚濊禆
RUN apk add --no-cache ca-certificates tzdata wget
# S 瀹夊叏鍔犲浐锛氶潪 root 鐢ㄦ埛杩愯
RUN addgroup -g 1001 -S appgroup && adduser -u 1001 -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /build/chiron /app/
COPY --from=builder /build/migrate /app/
COPY --from=builder /build/migrations /app/migrations/
# 鍒涘缓 workspace 鐩綍骞惰缃潈闄?RUN mkdir -p /app/workspace /app/data/plugins && chown -R appuser:appgroup /app
USER appuser
EXPOSE 8080
# S 瀹夊叏鍔犲浐锛氬仴搴锋鏌?HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1
CMD ["./chiron"]

