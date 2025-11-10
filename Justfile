# ZERB Development Commands
# =========================

# Default: show available commands
default:
    @just --list

# Run all tests
test:
    gotestsum --format pkgname-and-test-fails

# Run tests with verbose output
test-v:
    go test -v ./...

# Run single test by name
test-one TEST:
    go test -run {{TEST}} -v ./...

# Run tests with coverage
coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run tests with race detector
test-race:
    go test -race ./...

# Run linter
lint:
    golangci-lint run

# Format code
fmt:
    goimports -w .
    gofumpt -w .
    golines -w --max-len=120 .

# Run Go vet
vet:
    go vet ./...

# Build binary
build:
    @echo "Building ZERB..."
    go build -o bin/zerb ./cmd/zerb
    @echo "✓ Binary: bin/zerb"

# Build with version info
build-release VERSION:
    go build -ldflags "-X main.Version={{VERSION}}" -o bin/zerb ./cmd/zerb

# Clean build artifacts
clean:
    rm -rf bin/ coverage.out coverage.html .test-tmp/
    go clean

# Initialize Go module (first time setup)
init:
    go mod init github.com/ZebulonRouseFrantzich/zerb
    go mod tidy

# Update dependencies
deps:
    go get -u ./...
    go mod tidy

# Install git hooks
hooks:
    @echo "Installing pre-commit hooks..."
    pre-commit install
    @echo "✓ Hooks installed"

# Run all checks (lint + test + vet)
check: lint vet test

# Generate mocks (for testing)
mocks:
    @echo "Generating mocks..."
    # Add mockery or gomock commands here

# Show project status
status:
    @echo "Go Version: $(go version)"
    @echo "GOPATH: $GOPATH"
    @echo "Module: $(go list -m)"
    @echo "Dependencies: $(go list -m all | wc -l) modules"
