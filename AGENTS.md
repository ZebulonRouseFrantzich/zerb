# Agent Guidelines for ZERB

## Build/Test Commands
- `go test ./...` - Run all tests
- `go test -run TestName ./path/to/package` - Run single test
- `go build -o bin/zerb ./cmd/zerb` - Build binary
- `go vet ./...` - Run Go vet
- `golangci-lint run` - Run linter (when configured)

## Code Style
- **Go Version**: 1.21+ required
- **Imports**: stdlib → third-party → local (use goimports)
- **Naming**: Camel case (exportXXX/internalXXX), descriptive names, no abbreviations except common ones (ctx, err, cfg)
- **Error Handling**: Wrap with fmt.Errorf("context: %w", err), never ignore errors, use named return for cleanup
- **Types**: Prefer explicit types, avoid interface{}, use context.Context for cancellation
- **Testing**: Table-driven tests, >80% coverage required, stub external dependencies (mise, chezmoi, gopsutil)

## Architecture Constraints
- **Isolation**: mise/chezmoi must use env vars/flags for complete isolation - never touch system installations
- **Security**: GPG verification preferred, SHA256 fallback - never expose secrets in logs (active redaction required)
- **User-facing**: Never mention internal tools (gopsutil, mise, chezmoi) - use abstracted terminology
- **Config**: Lua-based declarative config with read-only platform table injection
- **Git**: Timestamped immutable configs in `configs/` subdirectory with `.zerb-active` marker file
