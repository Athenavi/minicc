# Backend - Multi-stage build
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/minicc ./cmd/minicc/
RUN CGO_ENABLED=0 go build -o /build/migrate ./cmd/migrate/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/minicc /app/
COPY --from=builder /build/migrate /app/
COPY --from=builder /build/migrations /app/migrations/
EXPOSE 8080
CMD ["./minicc"]
