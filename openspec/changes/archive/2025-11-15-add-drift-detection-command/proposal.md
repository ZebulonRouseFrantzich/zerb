# Change: Add Drift Detection Command

## Why

ZERB needs a user-facing drift detection command to identify and resolve discrepancies between declared configuration (`zerb.lua`), ZERB-managed tools (via mise), and the active environment (PATH). This three-way comparison enables users to maintain reproducible environments and detect external interference from system package managers.

The core drift detection logic (Phases 1-4) is complete. This change proposal covers the remaining work to expose drift detection through the CLI and complete the feature (Phases 5-7).

## What Changes

- Add `zerb drift` command to CLI for detecting and resolving environment drift
- Integrate drift formatter, resolver, and apply functions into command workflow
- Add interactive resolution modes: individual, adopt all, revert all, show only
- Add comprehensive integration tests for end-to-end drift detection
- Update documentation and agent guidelines with drift command usage

## Impact

- Affected specs: `drift-detection` (new capability)
- Affected code:
  - `cmd/zerb/drift.go` (new) - CLI command implementation
  - `cmd/zerb/main.go` - Command registration
  - `internal/drift/integration_test.go` - End-to-end tests
  - `AGENTS.md` - Build/test command updates
  - Documentation files

## Dependencies

- Phases 1-4 completed:
  - Phase 1: Core types and version utilities (✅ Complete)
  - Phase 2: Data collection functions (✅ Complete)
  - Phase 3: Drift detection logic (✅ Complete)
  - Phase 4: User interface components (✅ Complete)

## Completion Status

Remaining phases:
- Phase 5: Command Integration (~55% remaining effort)
- Phase 6: Integration & Testing (~20% remaining effort)
- Phase 7: Documentation & Polish (~25% remaining effort)

## Design Decisions

The following design decisions were made during Phases 1-4 and are reflected in the completed implementation:

### Version Detection Strategy
- **Primary approach**: Try `--version` flag first
- **Fallback**: Try `-v` flag if `--version` fails  
- **Unknown handling**: Mark version as "unknown" (DriftVersionUnknown type) if both fail
- **Parsing**: Extract semantic version using regex pattern `\d+\.\d+\.\d+`
- **Caching**: 5-minute TTL cache for version detection results to avoid repeated subprocess calls
  - Cache key: binary path
  - Cache invalidation: Automatic after 5 minutes
  - Override: `--force-refresh` flag (to be implemented in Phase 5) bypasses cache

### Backend Handling
- Tool specs support backend prefixes (e.g., `cargo:ripgrep`, `ubi:sharkdp/bat`)
- Normalized tool names extracted for comparison (e.g., `sharkdp/bat` → `bat`)
- Drift detection compares final binary in PATH, not installation method
- Backend prefix preserved in baseline configuration during adopt actions

### External Override Detection  
- **Method**: Check if binary path starts with `~/.config/zerb/mise/`
- **Symlink handling**: Resolve with `filepath.EvalSymlinks()` before path checking
- **ZERB path detection**: Implemented in `IsZERBManaged()` function in `internal/drift/managed.go`

### Terminology Abstraction
- All user-facing output uses "ZERB" terminology
- Internal tool names ("mise", "chezmoi") never exposed to users
- Install path obfuscation: `~/.config/zerb/installs/` (no "mise" in path shown to users)
