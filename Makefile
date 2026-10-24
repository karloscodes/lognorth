# LogNorth Makefile
# Storage defaults to ./storage (relative to CWD)

TMP_DIR := ./tmp

.PHONY: build dev watch-go watch-web dev-server test clean install deps seed seed-events build-docker

# Install all dependencies
install: deps
	cd web && npm install

# Install Go and npm dependencies
deps:
	go mod download

# Version from git (tag or commit hash)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build LogNorth application
build:
	cd web && npm run build
	go build -ldflags "-X main.Version=$(VERSION)" -o tmp/lognorth-server cmd/lognorth/main.go
	go build -ldflags "-X main.Version=$(VERSION)" -o tmp/lnctl cmd/lnctl/main.go
	@echo "Built lognorth-server + lbctl $(VERSION)"

# Build Docker image locally
build-docker:
	docker build -t karloscodes/lognorth:$(VERSION) -t karloscodes/lognorth:latest .

# Development mode with hot reload
dev:
	$(MAKE) -j 2 watch-web watch-go

# Watch for changes in Go files (auto-restart on change)
watch-go:
	@echo "Watching Go files..."
	@mkdir -p $(TMP_DIR)
	LOGNORTH_ENV=development watchexec -r -e go -w . -i web -i tmp -i storage -i node_modules -- go run cmd/lognorth/main.go

# Watch for changes in web files (vite dev server handles this)
watch-web:
	@echo "Starting web dev server..."
	cd web && npm run dev

# Run Go server once (without file watching)
dev-server:
	@mkdir -p $(TMP_DIR)
	LOGNORTH_ENV=development go build -o $(TMP_DIR)/lognorth-server cmd/lognorth/main.go
	LOGNORTH_ENV=development $(TMP_DIR)/lognorth-server

# Run Go tests
test:
	go test ./...

# Run SDK compatibility tests (TypeScript + Go)
test-sdk:
	./scripts/test-sdk-compatibility.sh

# Run all tests (Go unit tests + SDK compatibility)
test-all:
	@echo "Running Go unit tests..."
	@go test ./...
	@echo ""
	@echo "Running SDK compatibility tests..."
	@./scripts/test-sdk-compatibility.sh

# Seed database with sample data (3 apps, a week of events)
seed:
	go run cmd/seed-demo/main.go

# Seed 5000 realistic Rails/Sidekiq events (requires API key)
# Usage: make seed-events KEY=lb_xxx
seed-events:
	@if [ -z "$(KEY)" ]; then echo "Usage: make seed-events KEY=<api_key>"; exit 1; fi
	./scripts/seed_events.sh $(KEY)

# Run e2e tests (requires server to be running)
test-e2e:
	cd e2e && npm test

# Clean build artifacts
clean:
	rm -rf tmp/
	rm -rf web/dist/
