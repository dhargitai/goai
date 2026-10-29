.PHONY: build test test-coverage test-race integration fmt lint vet vuln clean deps check help

# Default target
.DEFAULT_GOAL := help

# Build all examples (api and pipeline are separate modules)
build: ## Build all example binaries
	@echo "Building CLI example..."
	@go build -o bin/cli ./examples/simple-cli
	@echo "Building API example..."
	@cd examples/api && go build -o ../../bin/api ./cmd/server
	@echo "Building pipeline example..."
	@cd examples/pipeline && go build -o ../../bin/pipeline ./cmd/pipeline
	@echo "Build complete!"

# Run all tests
test: ## Run all tests
	@echo "Running tests..."
	@go test ./... -v

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with race detector
test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test ./... -race

# Run live tests against the OpenAI API (needs OPENAI_API_KEY; costs a little)
integration: ## Run integration tests against the OpenAI API
	@echo "Running integration tests..."
	@go test -tags=integration -count=1 ./provider/openai/...

# Format code
fmt: ## Format all Go code
	@echo "Formatting code..."
	@gofmt -w .
	@echo "Code formatted!"

# Run linter (requires golangci-lint)
lint: ## Run golangci-lint
	@echo "Running linter..."
	@golangci-lint run || echo "golangci-lint not installed, skipping..."

# Run go vet
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

# Scan all modules for known vulnerabilities
vuln: ## Run govulncheck on the library and examples
	@echo "Scanning for vulnerabilities..."
	@go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@cd examples/api && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@cd examples/pipeline && go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean
	@echo "Clean complete!"

# Download dependencies
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Run all checks (fmt, vet, test)
check: fmt vet test ## Run all checks (fmt, vet, test)

# Help target
help: ## Show this help message
	@echo "GoAI - Type-Safe AI Agent Framework"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
