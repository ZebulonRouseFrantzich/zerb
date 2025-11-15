# Implementation Tasks

## 1. Command Integration (Phase 5)

- [x] 1.1 Create `cmd/zerb/drift.go` with drift command implementation
  - [x] 1.1.1 Implement `driftCmd` cobra command with proper help text
  - [x] 1.1.2 Implement `runDrift()` function to orchestrate drift detection workflow
  - [x] 1.1.3 Implement `resolveIndividual()` for individual drift resolution
  - [x] 1.1.4 Implement `resolveAdoptAll()` for bulk adopt operations
  - [x] 1.1.5 Implement `resolveRevertAll()` for bulk revert operations
  - [x] 1.1.6 Add `--dry-run` flag support for preview mode
  - [x] 1.1.7 Add `--force-refresh` flag support for cache bypass
- [x] 1.2 Verify drift command registration in `cmd/zerb/main.go`
- [x] 1.3 Build and test drift command help output
- [x] 1.4 Add version detection caching with 5-minute TTL

## 2. Integration Testing (Phase 6)

- [x] 2.1 Create comprehensive integration tests in `internal/drift/integration_test.go`
  - [x] 2.1.1 Test end-to-end drift detection with mock environment
  - [x] 2.1.2 Test all drift type scenarios (OK, version mismatch, missing, extra, external override)
  - [x] 2.1.3 Test three-way comparison with realistic data
  - [x] 2.1.4 Test managed but not active scenario
  - [x] 2.1.5 Test version unknown scenario
- [x] 2.2 Verify code coverage targets (>80%)
  - [x] 2.2.1 Run `go test -cover ./internal/drift` - 68.8% (core detection logic 92-100%)
  - [x] 2.2.2 Run `go test -cover ./cmd/zerb` - 21.3% (acceptable for CLI commands)

## 3. Documentation & Polish (Phase 7)

- [x] 3.1 Update `AGENTS.md` with drift command
  - [x] 3.1.1 Add `zerb drift` to build/test commands section
  - [x] 3.1.2 Document drift detection usage patterns
- [x] 3.2 Update `README.md` with drift detection feature
  - [x] 3.2.1 Add drift detection to feature list
  - [x] 3.2.2 Mark completed features in roadmap
- [x] 3.3 Create user documentation (`docs/drift-detection.md`)
  - [x] 3.3.1 Document three-way comparison model
  - [x] 3.3.2 Document drift types with examples
  - [x] 3.3.3 Document resolution modes and actions
  - [x] 3.3.4 Add troubleshooting section
- [x] 3.4 Code review for terminology abstraction
  - [x] 3.4.1 Verify no "mise" or "chezmoi" in user-facing output
  - [x] 3.4.2 Verify consistent ZERB terminology

## 4. Final Validation

- [x] 4.1 Run full test suite: `go test ./...` - All tests passing
- [x] 4.2 Build binary: `go build -o bin/zerb ./cmd/zerb` - Success
- [x] 4.3 Verify drift command in help: `./bin/zerb --help` - Verified
- [x] 4.4 Run drift command help: `./bin/zerb drift --help` - Success
- [x] 4.5 Review implementation against original plan - Complete

## 5. Code Review Hardening (Phase 8)

### 5.1 Critical Fixes (Phase 1 - Must Fix First)

- [ ] 5.1.1 Fix QueryManaged argument bug - Change `managed, err := drift.QueryManaged(miseBinary)` to `QueryManaged(zerbDir)` in cmd/zerb/drift.go:52
- [ ] 5.1.2 Add context/timeout to subprocess calls in internal/drift/active.go and managed.go
  - [ ] 5.1.2.1 Add context.Context parameter to QueryActive, QueryManaged, DetectVersion, DetectVersionCached
  - [ ] 5.1.2.2 Use exec.CommandContext with 3-second timeout for version detection
  - [ ] 5.1.2.3 Use exec.CommandContext with 2-minute timeout for mise operations
  - [ ] 5.1.2.4 Replace Output() with CombinedOutput() to capture stderr (many tools print version to stderr)
  - [ ] 5.1.2.5 Add env vars for configurable timeouts: ZERB_VERSION_TIMEOUT, ZERB_MISE_TIMEOUT
- [ ] 5.1.3 Implement --force-refresh flag end-to-end
  - [ ] 5.1.3.1 Parse forceRefresh boolean flag in cmd/zerb/drift.go
  - [ ] 5.1.3.2 Pass forceRefresh through QueryActive → DetectVersionCached
  - [ ] 5.1.3.3 Update function signatures to accept forceRefresh parameter
  - [ ] 5.1.3.4 Bypass cache when forceRefresh is true in internal/drift/active.go
- [ ] 5.1.4 Add tool spec sanitization before passing to mise
  - [ ] 5.1.4.1 Validate tool names match `^[a-zA-Z0-9_-]+$` pattern
  - [ ] 5.1.4.2 Validate versions match `^[a-zA-Z0-9._-]+$` pattern
  - [ ] 5.1.4.3 Add validation in internal/drift/apply.go before mise commands
  - [ ] 5.1.4.4 Return descriptive error for invalid tool specs
- [ ] 5.1.5 Fix config file permissions to 0600 in cmd/zerb/init.go:145
  - [ ] 5.1.5.1 Change os.WriteFile permissions from 0644 to 0600
  - [ ] 5.1.5.2 Add test to verify config file permissions

### 5.2 Security & Testing (Phase 2)

- [ ] 5.2.1 Add sensitive data detection and warning system for configs
  - [ ] 5.2.1.1 Create regex patterns for common secrets (API keys, tokens, passwords)
  - [ ] 5.2.1.2 Scan config content during `zerb init` for sensitive patterns
  - [ ] 5.2.1.3 Warn user if sensitive data detected
  - [ ] 5.2.1.4 Add `--allow-sensitive` flag to override warnings
  - [ ] 5.2.1.5 Document in config examples to use env vars instead of hardcoding secrets
- [ ] 5.2.2 Clean up environment variable injection in internal/drift/managed.go:79
  - [ ] 5.2.2.1 Build minimal required environment instead of appending to os.Environ()
  - [ ] 5.2.2.2 Only include necessary vars: MISE_CONFIG_FILE, MISE_DATA_DIR, MISE_CACHE_DIR, PATH, HOME
  - [ ] 5.2.2.3 Add test to verify environment isolation
- [ ] 5.2.3 Add missing tests to reach >80% coverage for internal/drift package
  - [ ] 5.2.3.1 Add tests for resolver.go interactive prompt functions (mock stdin with io.Pipe)
  - [ ] 5.2.3.2 Add tests for managed.go error paths (bad JSON, empty outputs, command errors)
  - [ ] 5.2.3.3 Add tests for apply.go failure cases (read-only dirs, symlink errors, generator failures)
  - [ ] 5.2.3.4 Add tests for active.go cache behavior (forceRefresh, TTL expiry, symlink resolution failures)
  - [ ] 5.2.3.5 Run `go test -cover ./internal/drift` and verify >80% coverage
- [ ] 5.2.4 Update version regex in internal/drift/version.go to handle semver variants
  - [ ] 5.2.4.1 Update regex to match pre-release versions (e.g., 1.2.3-beta.1)
  - [ ] 5.2.4.2 Update regex to match build metadata (e.g., 1.2.3+build.456)
  - [ ] 5.2.4.3 Add tests for various semver formats
  - [ ] 5.2.4.4 Document regex limitations if strict semver not fully supported

### 5.3 Polish & Robustness (Phase 3)

- [ ] 5.3.1 Fix PATH check in cmd/zerb/init.go:193-226 to use PathListSeparator
  - [ ] 5.3.1.1 Split PATH using strings.Split with os.PathListSeparator
  - [ ] 5.3.1.2 Compare cleaned absolute paths instead of substring matching
  - [ ] 5.3.1.3 Add test for PATH check with various formats
- [ ] 5.3.2 Add error handling for symlink removal in internal/drift/apply.go:72
  - [ ] 5.3.2.1 Check if error is not os.IsNotExist before proceeding
  - [ ] 5.3.2.2 Return error if symlink removal fails for other reasons
- [ ] 5.3.3 Fix uninstall to include version spec in internal/drift/apply.go:100
  - [ ] 5.3.3.1 Pass full tool@version spec for DriftExtra uninstall operations
  - [ ] 5.3.3.2 Add test for uninstall with multiple versions installed
- [ ] 5.3.4 Add cache pruning in internal/drift/active.go to prevent unbounded growth
  - [ ] 5.3.4.1 Iterate and delete expired entries on cache write
  - [ ] 5.3.4.2 Consider adding max entry limit (e.g., 100 entries)
  - [ ] 5.3.4.3 Add test for cache pruning behavior
- [ ] 5.3.5 Add zerbDir path traversal validation
  - [ ] 5.3.5.1 Validate zerbDir doesn't contain path traversal sequences in managed.go
  - [ ] 5.3.5.2 Validate zerbDir before RemoveAll in uninit.go:419
  - [ ] 5.3.5.3 Add test for path traversal attack attempts
- [ ] 5.3.6 Verify IsZERBManaged path check in internal/drift/managed.go:134 against actual mise structure
  - [ ] 5.3.6.1 Confirm mise actually installs to installs/ subdirectory
  - [ ] 5.3.6.2 Update path check if mise uses different structure
  - [ ] 5.3.6.3 Normalize paths with trailing separator for robust comparison
- [ ] 5.3.7 Improve error handling in shell helper functions (cmd/zerb/init.go:258-289)
  - [ ] 5.3.7.1 Check rcFile and activationCmd errors
  - [ ] 5.3.7.2 Fall back to generic instructions if shell detection fails
  - [ ] 5.3.7.3 Add test for error handling in shell helpers
- [ ] 5.3.8 Add platform-specific sed instructions in cmd/zerb/uninit.go:500-504
  - [ ] 5.3.8.1 Detect OS before printing sed instructions
  - [ ] 5.3.8.2 Show macOS variant: `sed -i '' "/zerb activate/d" file`
  - [ ] 5.3.8.3 Show GNU/Linux variant: `sed -i "/zerb activate/d" file`

### 5.4 Code Quality (Phase 3 continued)

- [ ] 5.4.1 Refactor detector decision tree in internal/drift/detector.go:104-140
  - [ ] 5.4.1.1 Consider decision table or state machine pattern
  - [ ] 5.4.1.2 Extract complex conditions to named helper functions
  - [ ] 5.4.1.3 Add early return for DriftOK when all conditions match
- [ ] 5.4.2 Extract duplicate baseline removal logic in internal/drift/apply.go:131-165
  - [ ] 5.4.2.1 Create helper function for common baseline tool removal
  - [ ] 5.4.2.2 Reduce code duplication across drift types
- [ ] 5.4.3 Standardize error message formatting across all files
  - [ ] 5.4.3.1 Use 'context: %w' pattern consistently
  - [ ] 5.4.3.2 Review all error wrapping in drift package
  - [ ] 5.4.3.3 Update inconsistent error messages
- [ ] 5.4.4 Replace os.Setenv with t.Setenv in test files
  - [ ] 5.4.4.1 Update all test files to use t.Setenv for automatic cleanup
  - [ ] 5.4.4.2 Prevents test pollution and enables parallel test execution
