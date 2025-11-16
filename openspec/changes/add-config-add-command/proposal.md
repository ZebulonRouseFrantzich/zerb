# Change: Add `zerb config add` Command

## Why

Users need a straightforward way to track configuration files and directories in their ZERB environment. Currently, ZERB can manage tools via mise, but there's no command to add dotfiles and config directories to be tracked and versioned by chezmoi. The `zerb config add` command will enable users to declaratively manage their entire development environment configuration alongside their tools.

## What Changes

**Core Functionality:**
- Add new `zerb config add` CLI command with support for:
  - Single file paths (`zerb config add ~/.zshrc`)
  - Directory paths with `--recursive` flag (required for directories)
  - Optional flags for template processing, secrets, and private files (`--template`, `--secrets`, `--private`)
  - Multiple files in one command (`zerb config add ~/.zshrc ~/.gitconfig`)
  - Transaction recovery flags (`--resume`, `--abort`)
- Integrate with chezmoi to add configs to the isolated source directory with complete abstraction
- Update `zerb.lua` config file (create new timestamped version) with the added config
- Generate appropriate git commit messages
- Provide clear user feedback during the add process

**Security Enhancements:**
- **CRITICAL FIX**: Repair path validation security flaws in `internal/config/types.go`
  - Current `validateConfigPath()` has critical vulnerabilities (uses insecure `strings.Contains(cleaned, "..")`)
  - Fix: Implement canonical path checking with `filepath.EvalSymlinks` and `filepath.Rel`
  - Allow absolute paths within `$HOME` (currently incorrectly rejected)
  - Properly handle symlinks to prevent escape from home directory
- Comprehensive path validation:
  - Prevent directory traversal attacks
  - Restrict to home directory with proper canonicalization
  - Require `--recursive` flag for directories
  - Reject non-existent paths (fail fast for clearer semantics)
  - Normalize paths for accurate duplicate detection

**Architectural Improvements:**
- **Interface-based design** for testability and maintainability:
  - `Chezmoi` interface for wrapper operations
  - `Git` interface for git operations
  - `Clock` interface for deterministic timestamps in tests
  - Service layer with dependency injection
- **Context support throughout** for cancellation and timeouts
- **Error abstraction layer** to hide chezmoi from user-facing messages
- **Proper environment isolation** for chezmoi (scrub CHEZMOI_* vars)

**Robust Transaction Management:**
- Transaction file with comprehensive safety:
  - Location: `~/.config/zerb/tmp/txn-config-add-<uuid>.json` (versioned schema)
  - Lock file mechanism prevents concurrent operations
  - Atomic writes (write-then-rename) with fsync for durability
  - Artifact tracking (`created_source_files`) enables automatic cleanup on abort
  - State machine: pending → in-progress → completed | failed
- Support `--resume` flag to continue interrupted operations (idempotent)
- Support `--abort` flag with automatic cleanup of created artifacts
- Context cancellation support (Ctrl+C handling)
- Single atomic git commit for all files (transaction integrity)

## Impact

**Affected specs:**
- `config-management` (new capability)

**Affected code:**
- `cmd/zerb/main.go` - Add new subcommand routing
- `cmd/zerb/config_add.go` (new) - Command implementation
- `internal/config/types.go` - **CRITICAL FIX** to path validation security (lines 196-227)
- `internal/chezmoi/` (new package) - Interface-based chezmoi wrapper with error abstraction
- `internal/git/` (new package) - Interface-based git operations wrapper
- `internal/txn/` or similar (new) - Transaction management with locking and atomic writes

**User Impact:**
- **Security**: Fixes critical path validation vulnerabilities that could affect all config operations
- **Functionality**: Enables users to track their dotfiles via ZERB with robust safety guarantees
- **Reliability**: Transaction-based operations prevent data loss from interruptions
- **UX**: Completes the core workflow: `zerb init` → `zerb add <tools>` → `zerb config add <files>` → `zerb push`
- **Consistency**: Maintains git-native versioning for all config changes with atomic commits

**Quality Impact:**
- Interface-based design improves testability (>80% coverage achievable)
- Context support enables proper timeout and cancellation handling
- Comprehensive test coverage for security (path traversal, symlink escapes)

## Post-Implementation Review (2025-11-16)

Initial implementation completed and reviewed by @code-reviewer and @golang-pro subagents.

**Status:** Implementation complete with 3 Critical, 8 High, 12 Medium, and 7 Low priority issues identified requiring fixes.

**Key Findings:**
- **Critical:** Transaction system implemented but not integrated; symlink validation vulnerability; CLI flag parsing issues
- **High:** Initialization marker check incorrect; race conditions in config update; missing git commit hash capture
- **Overall:** Solid architecture but requires security fixes and test coverage improvements before production

**Action Items:** See `code-review-fixes.md` for complete list of required fixes and implementation guidance.
