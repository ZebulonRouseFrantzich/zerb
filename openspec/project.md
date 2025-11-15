# Project Context

## Purpose

ZERB (Zero-hassle Effortless Reproducible Builds) is a single-binary tool that provides declarative environment management by intelligently wrapping mature tools (mise for tools, chezmoi for configs) with git-native versioning and complete isolation.

**Key Goals:**
- Provide a single declarative config file (Lua-based) for entire development environment
- Complete isolation from system packages and tools (no conflicts)
- Git-native versioning with timestamped immutable configs
- Cross-platform support (MVP: Linux, Post-MVP: macOS, Windows)
- Platform-aware conditional logic in configurations
- Drift detection between declared and actual environment state
- Non-invasive shell integration (manual setup with clear instructions)

**Current Status:** Pre-pre-alpha, active development. The project plan is complete, but implementation is still in progress.

## Tech Stack

**Core:**
- Go 1.21+ (single binary distribution)
- Lua (via gopher-lua) for configuration DSL
- Git (go-git) for version control

**CLI Framework:**
- spf13/cobra (command structure)
- spf13/viper (config management)

**Key Dependencies:**
- yuin/gopher-lua (pure Go Lua VM, no CGO)
- go-git/go-git (pure Go git implementation)
- ProtonMail/go-crypto (GPG signature verification)
- sigstore/sigstore-go (cosign verification for binaries)
- shirou/gopsutil/v4/host (platform detection, scoped to host package only)

**External Tools (Wrapped):**
- mise (universal tool version manager)
- chezmoi (dotfile manager)

**Development Environment:**
- Nix flakes (reproducible dev environment)
- Just (task runner via Justfile)
- golangci-lint, goimports, gopls, delve (Go tooling)

## Project Conventions

### Code Style

**Go Standards:**
- Go 1.21+ required
- Imports: stdlib → third-party → local (use goimports)
- Naming: CamelCase (exportXXX/internalXXX), descriptive names, no abbreviations except common ones (ctx, err, cfg)
- Error Handling: Wrap with `fmt.Errorf("context: %w", err)`, never ignore errors, use named return for cleanup
- Types: Prefer explicit types, avoid interface{}, use context.Context for cancellation
- Comments: Package-level doc.go files, exported functions have doc comments

**Editor Configuration:**
- Go files: Use tabs (community standard). Set your preferred tab display width in your editor.
- Other files (YAML, JSON, Markdown, Nix): Use 2 spaces
- EditorConfig support required for consistent formatting

**File Organization:**
- One package per directory
- Test files alongside source files (*_test.go)
- Integration tests use _integration_test.go suffix
- Benchmark tests use _benchmark_test.go suffix
- Fuzz tests use _fuzz_test.go suffix

### Architecture Patterns

**Wrapping, Not Reinventing:**
- ZERB wraps mature tools (mise, chezmoi) rather than reimplementing functionality
- Complete isolation via environment variables and CLI flags
- Never touch system installations or user's existing tool setups

**Isolation Strategy:**
- mise: Isolated via `MISE_CONFIG_FILE`, `MISE_DATA_DIR`, `MISE_CACHE_DIR` environment variables
- chezmoi: Isolated via `--source` and `--config` CLI flags
- All ZERB-managed state lives in `~/.config/zerb/`

**User-Facing Abstraction:**
- Never expose internal tool names (mise, chezmoi, gopsutil) in user-facing messages
- Use abstracted terminology: "baseline", "managed by ZERB", "environment"
- Maintain clean abstraction layer for future implementation changes

**Configuration Management:**
- Lua-based declarative config with read-only platform table injection
- Timestamped immutable configs in `configs/` subdirectory
- `.zerb-active` marker file indicates active config
- Git-native versioning for full history and rollback

**Security-First:**
- GPG signature verification (preferred) with SHA256 checksum fallback
- Never expose secrets in logs (active redaction required)
- Embedded GPG keyrings for reproducibility
- Hard-coded binary versions for stable, tested combinations

**Error Handling:**
- Transaction-based resume for multi-step operations
- Active secret redaction in logs
- Graceful offline degradation with cached data
- Atomic writes for critical files
- Consistent error messages and exit codes

### Testing Strategy

**Test-Driven Development (TDD):**
- Strict test-first approach required
- Coverage goal: >80% for all packages
- Write tests before implementation

**Test Types:**
- Unit tests: Table-driven tests preferred
- Integration tests: Stub external dependencies (mise, chezmoi, gopsutil)
- Benchmark tests: For performance-critical code paths
- Fuzz tests: For parser and input validation
- Security tests: For GPG verification and secret redaction

**Key Test Areas:**
- Binary management (download, GPG verification, SHA256 fallback)
- Config versioning (timestamped files, rollback)
- mise wrapper (tool installation, version resolution, isolation)
- chezmoi wrapper (isolation verification, flag passing)
- Drift detection (detection accuracy, user prompts)
- Git operations (commit generation, sync, pre-commit hook, conflicts)
- Shell integration (activation script generation)
- Platform detection (distro detection, family booleans)

**Test Commands:**
- `go test ./...` - Run all tests
- `go test -run TestName ./path/to/package` - Run single test
- `just test` - Run tests via Justfile
- `just coverage` - Generate coverage report

### Git Workflow

**Commit Message Format:**
- Simple, readable format (no conventional commits in MVP)
- Examples:
  - "Add python@3.12.1"
  - "Update node: 20.11.0 → 21.0.0"
  - "Remove python@3.11.0"
  - "Add ~/.config/nvim/ to tracked configs"

**Pre-commit Hook (ZERB repos):**
1. Prevent timestamped config modifications (immutability)
2. Validate Lua syntax
3. Validate ZERB schema
4. Check large files (warn >10MB)
5. Detect secrets

**Branching:**
- Main branch for stable code
- Feature branches for development
- No strict naming convention (yet)

## Domain Context

**Declarative Environment Management:**
- ZERB is NOT a package manager - it wraps package managers
- Focus on reproducibility, not just convenience
- Declarative config defines desired state, drift detection finds differences

**Three-Way Drift Detection:**
1. **Baseline (declared):** What's in `zerb.lua`
2. **Managed (ZERB):** What ZERB has installed via mise
3. **Active (environment):** What's actually in PATH

This enables detection of:
- External package manager interference (apt, brew, nvm, etc.)
- Version mismatches
- Missing tools
- Extra tools not in baseline
- System installations taking precedence over ZERB's

**Platform-Aware Configuration:**
- Read-only platform table injected into Lua VM at initialization
- Supports conditional logic based on OS, architecture, Linux distro/family
- Example: `platform.is_linux and "cargo:i3-msg" or nil`

**Git-Native Versioning:**
- Every config change creates timestamped snapshot
- All configs tracked in git with full history
- Immutable files (never modified after creation)
- Sync across machines via standard git operations

**Complete Isolation:**
- ZERB never conflicts with system package managers
- ZERB never conflicts with existing mise/chezmoi installations
- Tools remain isolated but globally accessible via shell integration
- Clean uninstall: just delete `~/.config/zerb/`

## Important Constraints

**MVP Platform Support:**
- Linux: Full support (primary target: Linux Mint)
- Architectures: amd64 and arm64 ONLY (error on i386, arm 32-bit)
- macOS/Windows: Post-MVP (basic GOOS/GOARCH detection only)

**Isolation Requirements:**
- mise/chezmoi MUST use env vars/flags for complete isolation
- Never touch system installations
- Never modify user's existing tool setups

**Security Constraints:**
- GPG verification preferred, SHA256 fallback
- Never expose secrets in logs (active redaction required)
- No mirror fallback (security risk - fail if GitHub unavailable)
- Hard-coded binary versions for reproducible builds

**User Experience Constraints:**
- Non-invasive shell setup (manual with clear instructions)
- Never auto-modify rc files
- Never mention internal tools (mise, chezmoi, gopsutil) in user-facing messages
- Manual drift resolution (no auto-fix in MVP)

**Implementation Constraints:**
- Lua config must be read-only (no runtime modification)
- Platform table injected at VM initialization, immutable
- gopsutil usage scoped to host.PlatformInformation() only (MVP)
- No CGO dependencies (pure Go for cross-platform builds)

**Testing Constraints:**
- >80% coverage required for all packages
- TDD approach mandatory
- Stub external dependencies in tests

## External Dependencies

**GitHub Releases (Binary Downloads):**
- mise: https://github.com/jdx/mise/releases
- chezmoi: https://github.com/twpayne/chezmoi/releases
- No fallback mirrors (security-first)

**Wrapped Tools (Isolated):**
- mise: Universal tool version manager
  - Handles tool installation (cargo, npm, ubi, github backends)
  - Shell integration via `mise activate`
- chezmoi: Dotfile manager
  - Template processing
  - Secrets integration (1Password, Bitwarden, age)

**Platform Detection:**
- gopsutil v4: OS/distro detection (scoped to host package only)
- runtime.GOOS/GOARCH: Base OS and architecture

**Git Operations:**
- go-git: Pure Go git implementation (no system git dependency)
- Git repository in `~/.config/zerb/.git/`

**Cryptography:**
- ProtonMail/go-crypto: GPG signature verification
- sigstore/sigstore-go: Cosign verification for binaries

**No External Services:**
- No version registries or APIs
- No telemetry or analytics
- No external metadata services (MVP)
- Works completely offline with cached data
