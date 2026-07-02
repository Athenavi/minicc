.PHONY: all build run clean test lint fmt

APP=minicc
BUILD_DIR=build

all: fmt lint test build

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP) ./cmd/$(APP)
	@echo "build: $(BUILD_DIR)/$(APP)"

run:
	go run ./cmd/$(APP)

test:
	go test ./... -v -count=1 -timeout=30s

lint:
	go vet ./...
	@test -f $(shell which golangci-lint 2>/dev/null) && golangci-lint run || echo "golangci-lint not installed, skipping"

fmt:
	go fmt ./...

clean:
	rm -rf $(BUILD_DIR)

dev:
	@echo "starting dev server..."
	@echo "  POSTGRES_DSN=postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"
	@echo "  REDIS_ADDR=localhost:6379"
	$(MAKE) run

docker-build:
	docker build -t $(APP):latest -f Dockerfile .
	@echo "docker image: $(APP):latest"

docker-run:
	docker compose up -d

.PHONY: all build run clean test lint fmt dev docker-build docker-run
