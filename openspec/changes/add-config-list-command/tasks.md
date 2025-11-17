# Implementation Tasks

**⚠️ CRITICAL: Test-Driven Development (TDD) Required**

All tasks MUST follow strict test-first methodology as mandated by project standards:

1. **RED Phase**: Write failing test(s) first
2. **GREEN Phase**: Write minimal code to make test(s) pass
3. **REFACTOR Phase**: Clean up code while keeping tests green

**For each task below:**
- Write unit/integration tests BEFORE implementing the feature
- Verify tests fail initially (RED)
- Implement only enough code to make tests pass (GREEN)
- Refactor as needed while maintaining >80% coverage

The tasks are organized by feature area for clarity, but implementation MUST proceed test-first within each task.

**MVP Scope**: This implementation focuses on table output format only. JSON/plain output, `--all` flag, and drift detection are deferred to future iterations.

---

## 1. Core Command Implementation
- [x] 1.1 Create `cmd/zerb/config_list.go` with command structure
  - [x] 1.1a Write tests for command initialization
  - [x] 1.1b Define command structure using flag package (consistent with config_add.go)
  - [x] 1.1c Add basic error handling
- [x] 1.2 Update `cmd/zerb/main.go` routing for `config list` subcommand
  - [x] 1.2a Remove "(list and remove coming soon)" placeholder messages
  - [x] 1.2b Add case for "list" in config subcommand switch
  - [x] 1.2c Update help text to show available config actions
- [x] 1.3 Implement flag parsing (MVP: --verbose and --timeout only)
  - [x] 1.3a Write tests for flag combinations
  - [x] 1.3b Add `--verbose` flag for detailed output
  - [x] 1.3c Add `--timeout` flag (default 5m, consistent with config add)
  - [x] 1.3d Add flag validation
- [x] 1.4 Add help text and usage examples
  - [x] 1.4a Write help text with clear descriptions
  - [x] 1.4b Add examples showing common use cases
  - [x] 1.4c Document status indicators and their meanings (Synced, Missing, Partial)

## 2. Config Parsing and Retrieval (Active Config Only)
- [x] 2.1 Read active config from `.zerb-active` marker
  - [x] 2.1a Write tests for marker file reading
  - [x] 2.1b Handle missing marker (ZERB not initialized)
  - [x] 2.1c Handle corrupted marker file
- [x] 2.2 Parse active timestamped config
  - [x] 2.2a Write tests using existing `config.Parser` interface
  - [x] 2.2b Extract `Configs` array from parsed config
  - [x] 2.2c Handle parsing errors gracefully

## 3. Status Detection Logic (MVP: Synced, Missing, Partial only)
- [x] 3.1 Create `internal/config/status.go` with status types and interface
  - [x] 3.1a Write tests for status type definitions
  - [x] 3.1b Define `ConfigStatus` type as int enum (Synced, Missing, Partial)
  - [x] 3.1c Add `String()` method for status names ("synced", "missing", "partial")
  - [x] 3.1d Define `ConfigWithStatus` struct
  - [x] 3.1e Define `StatusDetector` interface accepting context
- [x] 3.2 Implement "Synced" status detection
  - [x] 3.2a Write tests for synced configs
  - [x] 3.2b Check: config in zerb.lua
  - [x] 3.2c Check: file exists on disk (os.Stat)
  - [x] 3.2d Check: file managed by ZERB (via chezmoi.Chezmoi interface)
  - [x] 3.2e All checks pass → Synced
- [x] 3.3 Implement "Missing" status detection
  - [x] 3.3a Write tests for missing files
  - [x] 3.3b Check: config in zerb.lua
  - [x] 3.3c Check: file does NOT exist on disk
  - [x] 3.3d Result → Missing
- [x] 3.4 Implement "Partial" status detection
  - [x] 3.4a Write tests for partial tracking
  - [x] 3.4b Check: config in zerb.lua
  - [x] 3.4c Check: file exists on disk
  - [x] 3.4d Check: file NOT managed by ZERB (not in chezmoi source)
  - [x] 3.4e Result → Partial
- [x] 3.5 Add TODO for future drift detection
  - [x] 3.5a Add comment: "Drift detection deferred - requires file hash comparison"
  - [x] 3.5b Reserve `StatusDrift` in enum as comment
  - [x] 3.5c Note future: Compare disk file hash vs managed file hash

## 4. Chezmoi Integration (Extend Interface)
- [x] 4.1 Extend `Chezmoi` interface in `internal/chezmoi/chezmoi.go`
  - [x] 4.1a Write tests for HasFile method
  - [x] 4.1b Add `HasFile(ctx context.Context, path string) (bool, error)` to interface
  - [x] 4.1c Implement method in `*Client`
  - [x] 4.1d Use `config.NormalizeConfigPath` for path canonicalization (security)
  - [x] 4.1e Check if normalized path exists in chezmoi source directory
  - [x] 4.1f Wrap ALL errors with `redactSensitiveInfo` (never expose internal paths)
- [x] 4.2 Add context support to query method
  - [x] 4.2a Accept context.Context for cancellation
  - [x] 4.2b Check context before filesystem operations
  - [x] 4.2c Return context errors appropriately

## 5. Service Layer Implementation
- [x] 5.1 Create `internal/service/config_list.go` with service struct
  - [x] 5.1a Write tests with mock dependencies (Parser, Chezmoi, StatusDetector interfaces)
  - [x] 5.1b Define `ConfigListService` struct
  - [x] 5.1c Add dependencies: Parser interface, Chezmoi interface, StatusDetector interface, zerbDir string
  - [x] 5.1d Define request/response types
- [x] 5.2 Implement List method for active config only
  - [x] 5.2a Write tests for active config listing
  - [x] 5.2b Read active marker from zerbDir (passed to constructor)
  - [x] 5.2c Parse active config via Parser interface
  - [x] 5.2d Detect status via StatusDetector interface
  - [x] 5.2e Return sorted results (alphabetical by path)
- [x] 5.3 Add context support throughout service
  - [x] 5.3a Accept context in all methods
  - [x] 5.3b Pass context to parser, chezmoi, status detector
  - [x] 5.3c Handle context cancellation gracefully

## 6. Output Formatting (MVP: Table and Verbose only)
- [x] 6.1 Implement table formatter (default output)
  - [x] 6.1a Write tests for table formatting
  - [x] 6.1b Create table header: STATUS, PATH, FLAGS
  - [x] 6.1c Format each row with aligned columns
  - [x] 6.1d Add summary line with counts (synced, missing, partial)
  - [x] 6.1e Preallocate slices based on len(configs) for efficiency
- [x] 6.2 Implement verbose formatter
  - [x] 6.2a Write tests for verbose output
  - [x] 6.2b Add SIZE column (get file size from disk via os.Stat)
  - [x] 6.2c Add LAST MODIFIED column (relative time: "2 hours ago")
  - [x] 6.2d Add notes section explaining status indicators
- [x] 6.3 Implement status indicator symbols (MVP: 3 statuses)
  - [x] 6.3a Write tests for status symbols
  - [x] 6.3b Map Synced → "✓"
  - [x] 6.3c Map Missing → "✗"
  - [x] 6.3d Map Partial → "?"
- [x] 6.4 Implement flags column formatting
  - [x] 6.4a Write tests for flag display
  - [x] 6.4b Show only enabled flags (omit false values)
  - [x] 6.4c Join multiple flags with ", " separator
  - [x] 6.4d Handle empty flags (no output)
  - [x] 6.6a Write tests for flag display
  - [x] 6.6b Show only enabled flags (omit false values)
  - [x] 6.6c Join multiple flags with ", " separator
  - [x] 6.6d Handle empty flags (no output)

## 7. Error Handling and Edge Cases
- [x] 7.1 Handle ZERB not initialized
  - [x] 7.1a Write tests for uninitialized state
  - [x] 7.1b Check for `.zerb-active` marker existence
  - [x] 7.1c Define sentinel error `ErrNotInitialized` in service layer
  - [x] 7.1d Return clear error: "ZERB not initialized. Run 'zerb init' first"
- [x] 7.2 Handle no configs tracked
  - [x] 7.2a Write tests for empty config list
  - [x] 7.2b Display friendly message: "No configs tracked yet"
  - [x] 7.2c Suggest: "Add configs with: zerb config add <path>"
- [x] 7.3 Handle corrupted config files
  - [x] 7.3a Write tests for Lua parse errors
  - [x] 7.3b Catch parser errors gracefully via error wrapping (%w)
  - [x] 7.3c Show error with config filename (sanitized via existing helpers)
  - [x] 7.3d Suggest recovery options
- [x] 7.4 Handle permission errors
  - [x] 7.4a Write tests for permission denied scenarios
  - [x] 7.4b Use errors.Is to detect permission errors
  - [x] 7.4c Show clear error message with redacted paths
  - [x] 7.4d Suggest checking file permissions

## 8. Integration and End-to-End Testing (MVP Scope)
- [x] 8.1 Test default list (active config only)
  - [x] 8.1a Create test fixture with sample zerb.lua
  - [x] 8.1b Run `zerb config list`
  - [x] 8.1c Verify table output format
  - [x] 8.1d Verify correct status detection (Synced, Missing, Partial)
- [x] 8.2 Test `--verbose` flag
  - [x] 8.2a Create configs with known sizes
  - [x] 8.2b Run `zerb config list --verbose`
  - [x] 8.2c Verify SIZE and LAST MODIFIED columns
  - [x] 8.2d Verify notes section present
- [x] 8.3 Test status detection accuracy
  - [x] 8.3a Create synced config (exists, managed)
  - [x] 8.3b Create missing config (delete file after add)
  - [x] 8.3c Create partial config (in zerb.lua but not managed)
  - [x] 8.3d Verify correct status for each
- [x] 8.4 Test with no configs
  - [x] 8.4a Initialize ZERB but don't add configs
  - [x] 8.4b Run `zerb config list`
  - [x] 8.4c Verify friendly "no configs" message
- [x] 8.5 Test with uninitialized ZERB
  - [x] 8.5a Run in directory without ZERB
  - [x] 8.5b Run `zerb config list`
  - [x] 8.5c Verify clear initialization error
- [x] 8.6 Test context cancellation (Ctrl+C)
  - [x] 8.6a Send SIGINT during operation
  - [x] 8.6b Verify no partial output shown
  - [x] 8.6c Verify exit code 130

## 9. Documentation and Help
- [x] 9.1 Add command to main help output
  - [x] 9.1a Update `cmd/zerb/main.go` help text
  - [x] 9.1b Show `zerb config list` in command list
  - [x] 9.1c Add brief description
- [x] 9.2 Create comprehensive `--help` output
  - [x] 9.2a Document flags (--verbose, --timeout)
  - [x] 9.2b Add usage examples
  - [x] 9.2c Explain status indicators (✓, ✗, ?)
  - [x] 9.2d Show sample output
- [x] 9.3 Add inline code comments
  - [x] 9.3a Document status detection logic
  - [x] 9.3b Explain formatter implementations
  - [x] 9.3c Add package-level documentation

## 10. Performance and Polish (MVP)
- [x] 10.1 Optimize for large config lists
  - [x] 10.1a Preallocate slices based on len(Configs)
  - [x] 10.1b Benchmark with 100+ configs
  - [x] 10.1c Ensure sub-second response time
- [x] 10.2 Handle terminal width gracefully (optional)
  - [x] 10.2a Detect terminal width
  - [x] 10.2b Truncate long paths if needed
  - [x] 10.2c Add "..." indicator for truncation

## 11. Code Review Fixes (Ship Blockers)

**⚠️ CRITICAL: Must complete before merge**

Based on code review findings (2025-11-17), these tasks address HIGH priority issues:

- [x] 11.1 Implement RedactedError wrapper type
  - [x] 11.1a Add RedactedError struct to `internal/chezmoi/chezmoi.go`
  - [x] 11.1b Implement Error() and Unwrap() methods
  - [x] 11.1c Add newRedactedError() helper function
  - [x] 11.1d Write tests for error chain preservation
  - [x] 11.1e Write tests for redaction behavior
- [x] 11.2 Update HasFile to use RedactedError
  - [x] 11.2a Replace line 122 with newRedactedError
  - [x] 11.2b Replace line 135 with newRedactedError
  - [x] 11.2c Replace line 149 with newRedactedError
  - [x] 11.2d Verify errors.Is/errors.As work correctly
- [x] 11.3 Add path normalization in service layer
  - [x] 11.3a Add normalization loop before status detection (service/config_list.go)
  - [x] 11.3b Handle normalization errors with proper context
  - [x] 11.3c Update detector to expect pre-normalized paths
  - [x] 11.3d Update detector documentation to note assumption
- [x] 11.4 Add tilde path tests
  - [x] 11.4a Write test in service layer using t.Setenv("HOME")
  - [x] 11.4b Test ~/.zshrc path
  - [x] 11.4c Test ~/.config/nvim/init.lua nested path
  - [x] 11.4d Verify correct status detection
- [x] 11.5 Create CLI test file
  - [x] 11.5a Create `cmd/zerb/config_list_test.go`
  - [x] 11.5b Add flag parsing tests (--verbose, --timeout)
  - [x] 11.5c Add not initialized error test
  - [x] 11.5d Add empty config list test
  - [x] 11.5e Add table formatting tests with mock service
  - [x] 11.5f Add context timeout test
  - [x] 11.5g Add context cancellation test
  - [x] 11.5h Target: >80% coverage (achieved 87.5%)
- [x] 11.6 Add CLI integration tests
  - [x] 11.6a Add integration tests with real service + temp filesystem
  - [x] 11.6b Test end-to-end with real config files
  - [x] 11.6c Verify CI/CD compatibility (no external deps)
- [x] 11.7 Complete service layer coverage
  - [x] 11.7a Add test for empty active marker
  - [x] 11.7b Add test for missing active config file
  - [x] 11.7c Verify >80% coverage achieved (achieved 86.5%)
- [x] 11.8 Fix context error wrapping
  - [x] 11.8a Update runConfigList context.DeadlineExceeded handling
  - [x] 11.8b Update runConfigList context.Canceled handling
  - [x] 11.8c Use %w to preserve error chain
- [x] 11.9 Remove duplicate StatusDetector interface
  - [x] 11.9a Remove interface from internal/service/config_list.go
  - [x] 11.9b Update service to use config.StatusDetector
  - [x] 11.9c Update constructor signature
  - [x] 11.9d Update all tests
- [x] 11.10 Run coverage verification
  - [x] 11.10a Run `go test -cover ./cmd/zerb` (87.5% for config_list.go)
  - [x] 11.10b Run `go test -cover ./internal/service` (86.5% for config_list.go)
  - [x] 11.10c Run `go test -cover ./internal/config` (100% for status.go)
  - [x] 11.10d Run `go test -cover ./internal/chezmoi` (84.2% for HasFile, 100% for RedactedError)
  - [x] 11.10e Verify all >80% (✅ ALL PASSED)


## Task Summary

**Original MVP Task Count**: ~45 tasks
**Code Review Additions**: ~35 tasks
**Total Tasks**: ~80 tasks
**Test Coverage Goal**: >80% for all new code (MANDATORY)
**TDD Compliance**: MANDATORY for all tasks

**Current Status**:
- Sections 1-10: ✓ COMPLETE (all checkboxes marked)
- Section 11: ✅ **COMPLETE** (all ship blockers resolved)

**Final Coverage Results**:
- CLI (`cmd/zerb/config_list.go`): **87.5%** ✅
- Service (`internal/service/config_list.go`): **86.5%** ✅
- Config/Status (`internal/config/status.go`): **100%** ✅
- Chezmoi/HasFile (`internal/chezmoi/chezmoi.go`): **84.2%** ✅

**Implementation Status**: ✅ **READY TO SHIP**

All ship blockers have been resolved and test coverage exceeds 80% for all components.

**Deferred to Future**:
- JSON output format (~10 tasks)
- Plain output format (~5 tasks)
- Historical config listing with `--all` (~15 tasks)
- Drift detection with file hash comparison (~8 tasks)
- Color support (~4 tasks)
- Medium priority fixes (~8 tasks)
- Low priority polish (~5 tasks)

**Dependencies**:
- Existing: `internal/config` (Parser, NormalizeConfigPath) ✅
- Existing: `internal/chezmoi` (Client) ✅
- New: `internal/config/status.go` ✅ COMPLETE
- New: `internal/service/config_list.go` ✅ COMPLETE
- New: `cmd/zerb/config_list.go` ✅ COMPLETE
