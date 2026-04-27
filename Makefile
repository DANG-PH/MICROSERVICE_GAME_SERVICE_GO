# ══════════════════════════════════════════════════════
# game-service-go — Makefile
# Dùng: make <command>
# ══════════════════════════════════════════════════════

.PHONY: run dev build test test/cover lint fmt vet tidy clean docker/build docker/run help

help:
	@echo ""
	@echo "  Commands:"
	@echo ""
	@echo "  make dev          Chạy server với hot-reload (Air) ← dùng hàng ngày"
	@echo "  make run          Chạy server (go run, fallback)"
	@echo "  make build        Compile binary → bin/api"
	@echo "  make test         Chạy tests với race detector"
	@echo "  make test/cover   Chạy tests + xuất coverage.html"
	@echo "  make lint         Chạy golangci-lint"
	@echo "  make fmt          Format code (go fmt)"
	@echo "  make vet          Static check (go vet)"
	@echo "  make tidy         Dọn go.mod và go.sum"
	@echo "  make clean        Xóa bin/, coverage, tmp/"
	@echo "  make docker/build Build Docker image"
	@echo "  make docker/run   Chạy container với .env"
	@echo ""

# ── Development ───────────────────────────────────────
dev:
	air

# Fallback nếu không dùng Air
run:
	go run cmd/api/main.go

# Compile ra binary tĩnh vào bin/api
build:
	go build -ldflags="-s -w" -o bin/api cmd/api/main.go

# ── Testing ───────────────────────────────────────────

test:
	go test ./... -race -cover

test/cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# ── Code Quality ──────────────────────────────────────

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# ── Cleanup ───────────────────────────────────────────

clean:
	rm -rf bin/ coverage.out coverage.html tmp/

# ── Docker ────────────────────────────────────────────

docker/build:
	docker build -t game-service-go .

docker/run:
	docker run --env-file .env -p 8080:8080 game-service-go