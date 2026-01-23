# Feedback Service Makefile
# Built by Blue Fermion Labs - https://bluefermionlabs.com
#
# Quick Start:
#   make run              - Run Go server locally
#   make opencode-start   - Start OpenCode container for self-healing

.PHONY: all build run test clean help setup opencode-start opencode-stop opencode-logs

# Go parameters
BINARY_NAME=feedback
BINARY_PATH=bin/$(BINARY_NAME)
MAIN_PATH=./cmd/server

# Default target
help:
	@echo "Feedback Service"
	@echo "Built by Blue Fermion Labs - https://bluefermionlabs.com"
	@echo ""
	@echo "Quick Start:"
	@echo "  make setup          - Create .env from template"
	@echo "  make run            - Run Go server locally"
	@echo "  make opencode-start - Start OpenCode container (for self-healing)"
	@echo ""
	@echo "Development:"
	@echo "  make build          - Build Go binary"
	@echo "  make test           - Run tests"
	@echo "  make lint           - Run linters"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Remove build artifacts"
	@echo ""
	@echo "OpenCode Container:"
	@echo "  make opencode-build - Build OpenCode container"
	@echo "  make opencode-start - Start container"
	@echo "  make opencode-stop  - Stop container"
	@echo "  make opencode-logs  - View container logs"
	@echo "  make opencode-shell - Shell into container"
	@echo "  make opencode-test  - Test analysis with sample data"
	@echo ""
	@echo "Demo: http://localhost:8080/demo"

# =============================================================================
# Setup
# =============================================================================

setup:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
		echo "Edit .env and set your LLM_API_KEY"; \
	else \
		echo ".env already exists"; \
	fi
	@chmod +x scripts/*.sh 2>/dev/null || true

# =============================================================================
# Build
# =============================================================================

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	go build -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "Built: $(BINARY_PATH)"

# =============================================================================
# Run Locally
# =============================================================================

run: build
	@echo ""
	@echo "Starting Feedback server..."
	@if [ -f .env ]; then \
		echo "Using .env configuration"; \
	else \
		echo "WARNING: No .env file found. Run 'make setup' to create one."; \
	fi
	@echo "Demo: http://localhost:8080/demo"
	@echo ""
	./$(BINARY_PATH)

# =============================================================================
# Testing
# =============================================================================

test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# =============================================================================
# OpenCode Container
# =============================================================================

CONTAINER_NAME=opencode-selfhealing

opencode-build:
	@echo "Building OpenCode container..."
	docker build -t opencode-selfhealing:latest -f Dockerfile.selfhealing .

opencode-start: opencode-build
	@if [ ! -f .env ]; then \
		echo "ERROR: .env file required"; \
		echo "Run 'make setup' and configure:"; \
		echo "  - LLM_API_KEY"; \
		echo "  - ADMIN_EMAILS (your email)"; \
		echo "  - OPENCODE_REPO_DIR (repo to analyze)"; \
		exit 1; \
	fi
	@# Stop existing container if running
	@docker rm -f $(CONTAINER_NAME) 2>/dev/null || true
	@echo ""
	@# Source .env and resolve REPO_DIR (handle relative paths)
	@. ./.env && \
		REPO_DIR=$$(cd "$${OPENCODE_REPO_DIR:-.}" 2>/dev/null && pwd || echo "$$PWD") && \
		echo "Repository: $$REPO_DIR" && \
		echo "Mounted at: /workspace (in container)" && \
		echo "" && \
		echo "Starting OpenCode container..." && \
		docker run -d \
			--name $(CONTAINER_NAME) \
			-e LLM_API_KEY="$$LLM_API_KEY" \
			-e GROQ_API_KEY="$$LLM_API_KEY" \
			-e LLM_BASE_URL="$${LLM_BASE_URL:-https://api.groq.com/openai/v1}" \
			-e LLM_MODEL="$${LLM_MODEL:-llama-3.3-70b-versatile}" \
			-e GITHUB_TOKEN="$$GITHUB_TOKEN" \
			-v "$$REPO_DIR:/workspace:rw" \
			opencode-selfhealing:latest
	@echo ""
	@echo "Container running. Setting up auth..."
	@sleep 2
	@docker exec $(CONTAINER_NAME) /app/setup-auth.sh
	@echo ""
	@echo "OpenCode ready!"
	@echo "Run 'make run' in another terminal to start the Go server"

opencode-stop:
	@echo "Stopping OpenCode container..."
	@docker rm -f $(CONTAINER_NAME) 2>/dev/null || true

opencode-logs:
	docker logs -f $(CONTAINER_NAME)

opencode-shell:
	docker exec -it $(CONTAINER_NAME) /bin/bash

opencode-test:
	@echo "Testing OpenCode analysis..."
	@echo "Model: $$(. ./.env && echo $${LLM_MODEL:-llama-3.3-70b-versatile})"
	@# Test directly in container with base64-encoded JSON
	@TEST_JSON='{"title":"Test bug","description":"The submit button does not work","type":"bug","url":"http://localhost/test"}' && \
		TEST_B64=$$(echo "$$TEST_JSON" | base64 -w0) && \
		docker exec $(CONTAINER_NAME) /app/analyze.sh "$$TEST_B64"

# =============================================================================
# Code Quality
# =============================================================================

lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...
	@which goimports > /dev/null && goimports -w . || true

# =============================================================================
# Cleanup
# =============================================================================

clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f feedback.db
	rm -f coverage.out coverage.html

clean-docker:
	@echo "Removing Docker images..."
	docker rmi opencode-selfhealing:latest 2>/dev/null || true

clean-all: clean clean-docker

# =============================================================================
# Dependencies
# =============================================================================

deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
