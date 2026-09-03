# Flagura Engine & Developer CLI Makefile
# High-performance, GitOps-native feature flag & experimentation platform.

.PHONY: all help dev build build-server build-cli templ templ-watch test test-race test-bench test-cover lint sec docker-up docker-down clean

# Default binary output directory
BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/flagura-server
CLI_BIN := $(BIN_DIR)/flagura

# Version & Build Flags
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.5.0")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.commit=$(COMMIT)' \
	-X 'main.date=$(BUILD_DATE)'

# Automatically load environment variables from .env.local or .env if present
-include .env
-include .env.local
export

all: build

## help: Show available Makefile targets with descriptions
help:
	@echo "⚡ Flagura Platform Local Development Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available Targets:"
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpDesc = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  \033[36m%-16s\033[0m %s\n", helpCommand, helpDesc; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)
	@echo ""

## Generate templates and run Flagura server locally in in-memory mode
dev: templ
	@echo "🚀 Starting Flagura Control Plane (In-Memory Engine) on http://localhost:3000 ..."
	go run ./cmd/server

## Build both flagura-server and flagura CLI binaries into bin/
build: templ build-server build-cli
	@echo "✅ Binaries built successfully in $(BIN_DIR)/"

## Compile flagura server daemon
build-server:
	@mkdir -p $(BIN_DIR)
	@echo "🔨 Compiling server binary -> $(SERVER_BIN) ..."
	go build -ldflags="$(LDFLAGS)" -o $(SERVER_BIN) ./cmd/server

## Compile flagura developer CLI
build-cli:
	@mkdir -p $(BIN_DIR)
	@echo "🔨 Compiling developer CLI -> $(CLI_BIN) ..."
	go build -ldflags="$(LDFLAGS)" -o $(CLI_BIN) ./cmd/cli

## Generate Go code from all *.templ component templates
templ:
	@echo "🎨 Generating templ views..."
	templ generate

## Run templ in watch mode for live browser re-compilation
templ-watch:
	@echo "👁️ Watching templ views for live reload..."
	templ generate --watch

## Run all unit and integration tests across the workspace
test: templ
	@echo "🧪 Running workspace tests..."
	go test -v ./...

## Run complete test suite with Go data race detector enabled
test-race: templ
	@echo "🏃 Running tests with -race detector enabled..."
	go test -v -race ./...

## Execute high-throughput microbenchmarks (stats & engine)
test-bench:
	@echo "📊 Running engine microbenchmarks..."
	go test -v -bench=. -benchmem ./pkg/stats/... ./pkg/engine/...

## Generate test coverage report
test-cover: templ
	@mkdir -p $(BIN_DIR)
	@echo "📈 Generating test coverage report..."
	go test -coverprofile=$(BIN_DIR)/coverage.out ./...
	go tool cover -func=$(BIN_DIR)/coverage.out

## Run code vetting and format checks
lint:
	@echo "🔍 Vetting Go source code..."
	go vet ./...

## Run gosec AST security vulnerability audit
sec:
	@echo "🔒 Running gosec security analysis..."
	gosec -quiet -fmt=text ./... 2>/dev/null || echo "Note: Run 'go install github.com/securego/gosec/v2/cmd/gosec@latest' if gosec is not installed."

## Launch Flagura server and PostgreSQL database in Docker
docker-up:
	@echo "🐳 Launching local Flagura environment with Docker Compose..."
	docker compose up -d

## Stop local Docker Compose services
docker-down:
	@echo "🛑 Stopping Docker Compose services..."
	docker compose down

## Clean build binaries and temporary coverage artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BIN_DIR) dist/
	@echo "✨ Workspace clean."
