<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Agent Guidelines for ZERB

## Build/Test Commands
- `go test ./...` - Run all tests
- `go test -run TestName ./path/to/package` - Run single test
- `go test -cover ./internal/drift` - Run drift tests with coverage
- `go build -o bin/zerb ./cmd/zerb` - Build binary
- `go vet ./...` - Run Go vet
- `golangci-lint run` - Run linter (when configured)
- `./bin/zerb drift --help` - Show drift command help
- `./bin/zerb drift --dry-run` - Check for drift without making changes

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
