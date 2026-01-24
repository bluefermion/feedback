# Feedback Service Makefile
# Built by Blue Fermion Labs - https://bluefermionlabs.com
#
# Quick Start:
#   make run              - Run Go server locally
#   make opencode-start   - Start OpenCode container for self-healing

.PHONY: all build run test clean help setup opencode-start opencode-stop opencode-logs \
       uat uat-setup uat-headed uat-submit uat-verify uat-full uat-task uat-clean \
       pr branch

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
	@echo "  make pr             - Create PR with AI-generated description"
	@echo ""
	@echo "Development:"
	@echo "  make build          - Build Go binary"
	@echo "  make test           - Run tests"
	@echo "  make lint           - Run linters"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Remove build artifacts"
	@echo ""
	@echo "UAT (LLM-Driven Browser Testing):"
	@echo "  make uat-setup      - Setup Python venv and dependencies"
	@echo "  make uat            - Run UAT (default: submit workflow)"
	@echo "  make uat-headed     - Run UAT with visible browser"
	@echo "  make uat-submit     - Run feedback submission test"
	@echo "  make uat-verify     - Run submission verification test"
	@echo "  make uat-full       - Run full workflow (submit + verify)"
	@echo "  make uat-demo       - Run full workflow with visible browser"
	@echo "  make uat-task TASK=\"...\" - Run custom natural language task"
	@echo "  make uat-clean      - Clean UAT artifacts"
	@echo ""
	@echo "Git Workflow (AI-Assisted):"
	@echo "  make branch NAME=x  - Create feature/x branch"
	@echo "  make pr             - Create PR with AI-generated description"
	@echo "  make pr-auto        - Create PR without preview"
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

# =============================================================================
# UAT (User Acceptance Testing with Browser-Use)
# =============================================================================

UAT_DIR=uat
UAT_VENV=$(UAT_DIR)/.venv
UAT_SCRIPT=$(UAT_DIR)/run_uat.sh

# Setup UAT environment (venv, dependencies, playwright)
uat-setup:
	@echo "Setting up UAT environment..."
	@chmod +x $(UAT_SCRIPT)
	@cd $(UAT_DIR) && \
		python3 -m venv .venv && \
		. .venv/bin/activate && \
		pip install --quiet --upgrade pip && \
		pip install --quiet -r requirements.txt && \
		playwright install chromium
	@rm -f $(UAT_VENV)/.deps_installed
	@touch $(UAT_VENV)/.deps_installed
	@echo ""
	@echo "UAT setup complete!"
	@echo "Run 'make uat' to execute tests"

# Run UAT (default: submit workflow)
uat:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT)

# Run UAT with visible browser
uat-headed:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --headed

# Run submit feedback workflow
uat-submit:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --workflow submit

# Run verify submission workflow
uat-verify:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --workflow verify

# Run full workflow (submit + verify)
uat-full:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --workflow full

# Run full workflow with visible browser
uat-demo:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --headed --workflow full

# Run custom task (usage: make uat-task TASK="your task here")
uat-task:
	@if [ -z "$(TASK)" ]; then \
		echo "Usage: make uat-task TASK=\"your natural language task\""; \
		echo "Example: make uat-task TASK=\"Click the feedback button and submit a bug report\""; \
		exit 1; \
	fi
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --task "$(TASK)"

# Run all page tests
uat-all:
	@chmod +x $(UAT_SCRIPT)
	@$(UAT_SCRIPT) --all

# Clean UAT artifacts
uat-clean:
	@echo "Cleaning UAT artifacts..."
	@rm -rf $(UAT_DIR)/screenshots/*.png
	@rm -rf $(UAT_DIR)/reports/*.json
	@rm -rf $(UAT_DIR)/reports/*.md
	@rm -rf $(UAT_DIR)/browser_state/
	@rm -rf $(UAT_DIR)/__pycache__/
	@echo "UAT artifacts cleaned"

# Deep clean UAT (including venv)
uat-clean-all: uat-clean
	@echo "Removing UAT virtual environment..."
	@rm -rf $(UAT_VENV)
	@echo "UAT fully cleaned"

# =============================================================================
# Git Workflow (AI-Assisted)
# =============================================================================

# Create a feature branch (usage: make branch NAME=feature-name)
branch:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make branch NAME=your-feature-name"; \
		echo "Example: make branch NAME=add-dark-mode"; \
		exit 1; \
	fi
	@git checkout -b "feature/$(NAME)"
	@echo "Created and switched to branch: feature/$(NAME)"

# Create PR with AI-generated title and description
pr:
	@chmod +x scripts/create-pr.sh
	@scripts/create-pr.sh

# Create PR without preview (auto-create)
pr-auto:
	@chmod +x scripts/create-pr.sh
	@scripts/create-pr.sh --no-preview
