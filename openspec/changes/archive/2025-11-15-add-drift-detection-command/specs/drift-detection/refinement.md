# Drift Detection Implementation Refinement

## Overview

This document tracks the code review findings and hardening improvements applied to the drift detection implementation after initial completion of Phases 1-7.

**Review Date:** November 14, 2025
**Reviewers:** AI code-reviewer agent, AI golang-pro agent
**Files Reviewed:** 24 files (cmd/zerb/, internal/drift/, docs/)
**Issues Identified:** 42 total (4 Critical, 6 High, 9 Medium, 9 Low, 4 Security, 8 Best Practices, 2 Code Quality)

## Review Methodology

Two specialized agents reviewed all changed files by comparing against the `main` branch:

1. **code-reviewer** - Focused on edge cases, security issues, error handling, and best practices
2. **golang-pro** - Focused on Go idioms, performance, concurrency safety, and test coverage

Each agent provided file-specific findings with:
- Severity classification
- Exact file:line references
- Issue description
- Fix recommendations

## Critical Issues (Must Fix)

### 1. QueryManaged Argument Bug (cmd/zerb/drift.go:52)
**Severity:** Critical - Breaks drift detection entirely
**Issue:** Passes full `miseBinary` path instead of `zerbDir`, resulting in double `bin/mise` path construction
**Fix:** Change `managed, err := drift.QueryManaged(miseBinary)` to `QueryManaged(zerbDir)`
**Impact:** Blocking bug preventing drift detection from functioning

### 2. Test Coverage Below Project Requirement (internal/drift/)
**Severity:** Critical - Policy violation
**Issue:** Current coverage ~68.8%, project requires >80%
**Gaps:** 
- resolver.go interactive paths
- managed.go error cases
- apply.go failure scenarios
- active.go cache/fallback paths
**Fix:** Add table-driven tests for all error paths and edge cases
**Impact:** CI may block merges, incomplete validation of error handling

### 3. Missing IsZERBManaged Path Verification (internal/drift/managed.go:134)
**Severity:** Critical - Incorrect drift classification
**Issue:** Checks for `installs` subdirectory but actual mise structure may differ
**Fix:** Verify actual mise installation paths and update check accordingly
**Impact:** External overrides may be incorrectly classified

### 4. No Validation for Empty Mise Output (internal/drift/managed.go:52)
**Severity:** Critical - Silent failure
**Issue:** QueryManaged doesn't validate mise returned data
**Fix:** Add validation that at least one command succeeded with data
**Impact:** Empty results treated as success, leading to incorrect drift reports

## High Priority Issues

### 5. No Timeouts for Subprocess Calls (internal/drift/active.go, managed.go)
**Severity:** High - System hangs
**Issue:** `exec.Command` can hang indefinitely; stderr ignored (many tools print version there)
**Fix:**
- Use `exec.CommandContext` with timeouts (3s for version detection, 2m for mise)
- Replace `Output()` with `CombinedOutput()` to capture stderr
- Add env vars: `ZERB_VERSION_TIMEOUT`, `ZERB_MISE_TIMEOUT`
**Impact:** User commands can freeze, requiring manual kill

### 6. Force-Refresh Flag Not Implemented (cmd/zerb/drift.go:20-23)
**Severity:** High - Documented feature missing
**Issue:** Flag accepted but unused; caching exists without bypass
**Fix:** Plumb flag through QueryActive → DetectVersionCached; update signatures
**Impact:** Users cannot force cache refresh when needed

### 7. PATH Check via Substring (cmd/zerb/init.go:193-226)
**Severity:** High - False positives
**Issue:** String contains check instead of proper path comparison
**Fix:** Split by `os.PathListSeparator` and compare cleaned absolute paths
**Impact:** Incorrect PATH warnings or missing actual issues

### 8. Environment Variable Injection Risk (internal/drift/managed.go:79)
**Severity:** High - Security
**Issue:** Appends to full `os.Environ()` instead of clean environment
**Fix:** Build minimal required environment with only necessary vars
**Impact:** Malicious env vars could affect mise behavior

### 9. Symlink Removal Errors Ignored (internal/drift/apply.go:72)
**Severity:** High - Operation failures
**Issue:** `os.Remove(symlinkPath)` error ignored
**Fix:** Check error is not `os.IsNotExist` and handle appropriately
**Impact:** Symlink creation can fail silently

### 10. Uninstall Missing Version Spec (internal/drift/apply.go:100)
**Severity:** High - Wrong version removed
**Issue:** Only passes tool name, could uninstall wrong version
**Fix:** Pass `tool@version` for uninstall operations
**Impact:** Multiple versions installed = unpredictable behavior

## Implementation Phases

### Phase 1: Critical Fixes (Estimated: 2-3 hours)
1. Fix QueryManaged argument (drift.go:52) - 5 min
2. Add subprocess timeouts with context - 1-1.5 hours
3. Implement --force-refresh end-to-end - 45 min
4. Add tool spec sanitization - 30 min
5. Fix config permissions to 0600 - 15 min

**Deliverable:** Core bugs fixed, system doesn't hang, basic security hardened

### Phase 2: Security & Testing (Estimated: 3-4 hours)
1. Sensitive data detection system - 1.5 hours
2. Environment variable cleanup - 45 min
3. Test coverage >80% - 1.5-2 hours
4. Version regex improvements - 30 min

**Deliverable:** Security hardened, test coverage meets requirements, comprehensive validation

### Phase 3: Polish (Estimated: 2-3 hours)
1. PATH check, error handling, cache management - 1 hour
2. Path traversal validation - 30 min
3. Code quality refactoring - 1-1.5 hours

**Deliverable:** Production-ready, maintainable, robust implementation

**Total Estimated Effort:** 7-10 hours

## Validation Criteria

Before marking refinement complete:

- [ ] All critical issues resolved
- [ ] `go test ./...` passes with no failures
- [ ] `go test -cover ./internal/drift` shows >80% coverage
- [ ] `go vet ./...` passes with no warnings
- [ ] `./bin/zerb drift --help` shows correct documentation
- [ ] `./bin/zerb drift --dry-run` executes without hanging
- [ ] `./bin/zerb drift --force-refresh` bypasses cache
- [ ] Config files created with 0600 permissions
- [ ] Tool spec sanitization blocks malicious input
- [ ] Sensitive data detection warns appropriately
- [ ] All subprocess calls have timeouts
- [ ] Error messages follow consistent format

## Benefits of Refinement

**Reliability:**
- Eliminates blocking bug (QueryManaged)
- Prevents system hangs (subprocess timeouts)
- Improves error handling throughout

**Security:**
- Blocks command injection attacks
- Prevents path traversal exploits
- Protects sensitive data in configs
- Restricts file permissions

**Maintainability:**
- Test coverage >80% validates correctness
- Standardized error formatting aids debugging
- Reduced code duplication
- Clearer decision logic

**User Experience:**
- Force-refresh works as documented
- Better error messages
- Platform-specific instructions
- Faster execution (cache improvements)

## References

- Original proposal: `openspec/changes/add-drift-detection-command/proposal.md`
- Implementation tasks: `openspec/changes/add-drift-detection-command/tasks.md`
- Specification: `openspec/changes/add-drift-detection-command/specs/drift-detection/spec.md`
- Code review agents: code-reviewer, golang-pro
- Review date: November 14, 2025
