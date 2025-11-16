# Implementation Tasks

## Test-Driven Development Methodology

All tasks MUST follow strict test-first methodology as mandated by project standards (>80% coverage required):

1. **RED Phase**: Write failing test(s) first
2. **GREEN Phase**: Write minimal code to make test(s) pass
3. **REFACTOR Phase**: Clean up code while keeping tests green

**Process:**
- Write unit/integration tests BEFORE implementing the feature
- Verify tests fail initially (RED)
- Implement only enough code to make tests pass (GREEN)
- Refactor as needed while maintaining >80% coverage

The tasks are organized by feature area for clarity, but implementation MUST proceed test-first within each task.

---

## 1. Git Repository Setup

- [ ] 1.1 Add `Init()` method to `internal/git/git.go` for repository initialization
- [ ] 1.2 Add `Configure()` method to set git config (user.name, user.email)
- [ ] 1.3 Add `InitialCommit()` method to create first commit
- [ ] 1.4 Add unit tests for git initialization methods
- [ ] 1.5 Add integration test for full init workflow

## 2. .gitignore Template

- [ ] 2.1 Create `.gitignore` template as embedded file or string constant
- [ ] 2.2 Template should exclude: `mise/config.toml`, `chezmoi/config.toml`, `bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `.txn/`, `.direnv/`, `keyrings/`, `zerb.lua.active`, `.zerb-active`
- [ ] 2.3 Template should track: `configs/`, `chezmoi/source/`
- [ ] 2.4 Write .gitignore during init process
- [ ] 2.5 Test .gitignore effectiveness with real git commands
- [ ] 2.6 Document rationale for excluding generated configs (derived from zerb.lua)

## 3. Init Command Integration

- [ ] 3.1 Add git initialization step to `runInit()` in `cmd/zerb/init.go`
- [ ] 3.2 Add error handling for git initialization failures
- [ ] 3.3 Detect git user.name and user.email from system git config (fallback to defaults)
- [ ] 3.4 Order steps correctly: directory structure → .gitignore → git init → initial config → initial commit
- [ ] 3.5 Update success message to mention git repository creation
- [ ] 3.6 Handle case where git is not installed (graceful degradation with warning)

## 4. Git User Configuration

- [ ] 4.1 Implement fallback logic: system git config → environment variables → placeholder values
- [ ] 4.2 Warn user if placeholder values are used
- [ ] 4.3 Provide instructions to configure git user info after init
- [ ] 4.4 Test with various git config states (configured, partially configured, not configured)

## 5. Documentation

- [ ] 5.1 Update README.md to mention git repository in ZERB directory
- [ ] 5.2 Add troubleshooting section for git initialization issues
- [ ] 5.3 Document .gitignore patterns and their rationale
- [ ] 5.4 Update init command help text to mention git setup

## 6. Testing

### 6.1 Unit Tests (TDD - Write First)
- [ ] 6.1.1 Test `Init()` creates `.git` directory
- [ ] 6.1.2 Test `Init()` handles existing repository gracefully
- [ ] 6.1.3 Test `Init()` fails when git binary not found
- [ ] 6.1.4 Test `Configure()` sets user.name and user.email
- [ ] 6.1.5 Test `Configure()` uses system git config when available
- [ ] 6.1.6 Test `Configure()` falls back to environment variables
- [ ] 6.1.7 Test `Configure()` uses placeholder values as last resort
- [ ] 6.1.8 Test `InitialCommit()` creates commit with correct message
- [ ] 6.1.9 Test `InitialCommit()` includes timestamped config file
- [ ] 6.1.10 Test `InitialCommit()` includes `.gitignore` file

### 6.2 Integration Tests (TDD - Write First)
- [ ] 6.2.1 Test `zerb init` creates .git directory
- [ ] 6.2.2 Test initial commit includes timestamped config
- [ ] 6.2.3 Test .gitignore excludes runtime files (`bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `.txn/`, `.direnv/`, `keyrings/`)
- [ ] 6.2.4 Test .gitignore excludes generated configs (`mise/config.toml`, `chezmoi/config.toml`)
- [ ] 6.2.5 Test .gitignore excludes symlinks (`zerb.lua.active`, `.zerb-active`)
- [ ] 6.2.6 Test .gitignore tracks `configs/` directory
- [ ] 6.2.7 Test .gitignore tracks `chezmoi/source/` directory
- [ ] 6.2.8 Test git config is set correctly (user.name, user.email)
- [ ] 6.2.9 Test graceful handling when git binary not found
- [ ] 6.2.10 Test idempotency (running init twice should detect existing repo)
- [ ] 6.2.11 Test on clean system without global git config

### 6.3 Coverage Requirements
- [ ] 6.3.1 Run `go test -cover ./internal/git` and verify >80% coverage
- [ ] 6.3.2 Run `go test -cover ./cmd/zerb` and verify init command coverage
- [ ] 6.3.3 Generate coverage report: `go test -coverprofile=coverage.out ./...`
- [ ] 6.3.4 Review coverage report for untested edge cases

## 7. Validation

- [ ] 7.1 Verify `git log` shows initial commit after init
- [ ] 7.2 Verify `git status` shows clean working tree after init
- [ ] 7.3 Verify runtime directories are ignored by git
- [ ] 7.4 Verify generated config files (`mise/config.toml`, `chezmoi/config.toml`) are ignored by git
- [ ] 7.5 Verify keyrings directory is ignored by git
- [ ] 7.6 Run `go test ./...` and ensure all tests pass
- [ ] 7.7 Manual end-to-end test of complete init workflow

## 8. Out of Scope (Future Work)

**Note:** The following items are architectural dependencies documented in design.md but NOT implemented in this change:

- [ ] 8.1 Config generation from `zerb.lua` to `mise/config.toml` (separate change)
- [ ] 8.2 Config generation from `zerb.lua` data section to `chezmoi/config.toml` (separate change)
- [ ] 8.3 Add `data` field to `internal/config/types.go Config` struct (separate change)
- [ ] 8.4 Update `examples/full.lua` to use abstracted `data` field (separate change)
- [ ] 8.5 Remove mise/chezmoi references from user-facing documentation (separate change)
- [ ] 8.6 Implement `zerb activate` config regeneration logic (separate change)

**Rationale:** This change focuses exclusively on git repository initialization. Config generation is a separate architectural concern that will be implemented in a future change. The .gitignore patterns are forward-compatible with the planned config generation approach.
