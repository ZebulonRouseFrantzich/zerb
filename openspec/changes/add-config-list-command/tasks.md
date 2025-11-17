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
- [ ] 1.1 Create `cmd/zerb/config_list.go` with command structure
  - [ ] 1.1a Write tests for command initialization
  - [ ] 1.1b Define command structure using flag package (consistent with config_add.go)
  - [ ] 1.1c Add basic error handling
- [ ] 1.2 Update `cmd/zerb/main.go` routing for `config list` subcommand
  - [ ] 1.2a Remove "(list and remove coming soon)" placeholder messages
  - [ ] 1.2b Add case for "list" in config subcommand switch
  - [ ] 1.2c Update help text to show available config actions
- [ ] 1.3 Implement flag parsing (MVP: --verbose and --timeout only)
  - [ ] 1.3a Write tests for flag combinations
  - [ ] 1.3b Add `--verbose` flag for detailed output
  - [ ] 1.3c Add `--timeout` flag (default 5m, consistent with config add)
  - [ ] 1.3d Add flag validation
- [ ] 1.4 Add help text and usage examples
  - [ ] 1.4a Write help text with clear descriptions
  - [ ] 1.4b Add examples showing common use cases
  - [ ] 1.4c Document status indicators and their meanings (Synced, Missing, Partial)

## 2. Config Parsing and Retrieval (Active Config Only)
- [ ] 2.1 Read active config from `.zerb-active` marker
  - [ ] 2.1a Write tests for marker file reading
  - [ ] 2.1b Handle missing marker (ZERB not initialized)
  - [ ] 2.1c Handle corrupted marker file
- [ ] 2.2 Parse active timestamped config
  - [ ] 2.2a Write tests using existing `config.Parser` interface
  - [ ] 2.2b Extract `Configs` array from parsed config
  - [ ] 2.2c Handle parsing errors gracefully

## 3. Status Detection Logic (MVP: Synced, Missing, Partial only)
- [ ] 3.1 Create `internal/config/status.go` with status types and interface
  - [ ] 3.1a Write tests for status type definitions
  - [ ] 3.1b Define `ConfigStatus` type as int enum (Synced, Missing, Partial)
  - [ ] 3.1c Add `String()` method for status names ("synced", "missing", "partial")
  - [ ] 3.1d Define `ConfigWithStatus` struct
  - [ ] 3.1e Define `StatusDetector` interface accepting context
- [ ] 3.2 Implement "Synced" status detection
  - [ ] 3.2a Write tests for synced configs
  - [ ] 3.2b Check: config in zerb.lua
  - [ ] 3.2c Check: file exists on disk (os.Stat)
  - [ ] 3.2d Check: file managed by ZERB (via chezmoi.Chezmoi interface)
  - [ ] 3.2e All checks pass → Synced
- [ ] 3.3 Implement "Missing" status detection
  - [ ] 3.3a Write tests for missing files
  - [ ] 3.3b Check: config in zerb.lua
  - [ ] 3.3c Check: file does NOT exist on disk
  - [ ] 3.3d Result → Missing
- [ ] 3.4 Implement "Partial" status detection
  - [ ] 3.4a Write tests for partial tracking
  - [ ] 3.4b Check: config in zerb.lua
  - [ ] 3.4c Check: file exists on disk
  - [ ] 3.4d Check: file NOT managed by ZERB (not in chezmoi source)
  - [ ] 3.4e Result → Partial
- [ ] 3.5 Add TODO for future drift detection
  - [ ] 3.5a Add comment: "Drift detection deferred - requires file hash comparison"
  - [ ] 3.5b Reserve `StatusDrift` in enum as comment
  - [ ] 3.5c Note future: Compare disk file hash vs managed file hash

## 4. Chezmoi Integration (Extend Interface)
- [ ] 4.1 Extend `Chezmoi` interface in `internal/chezmoi/chezmoi.go`
  - [ ] 4.1a Write tests for HasFile method
  - [ ] 4.1b Add `HasFile(ctx context.Context, path string) (bool, error)` to interface
  - [ ] 4.1c Implement method in `*Client`
  - [ ] 4.1d Use `config.NormalizeConfigPath` for path canonicalization (security)
  - [ ] 4.1e Check if normalized path exists in chezmoi source directory
  - [ ] 4.1f Wrap ALL errors with `redactSensitiveInfo` (never expose internal paths)
- [ ] 4.2 Add context support to query method
  - [ ] 4.2a Accept context.Context for cancellation
  - [ ] 4.2b Check context before filesystem operations
  - [ ] 4.2c Return context errors appropriately

## 5. Service Layer Implementation
- [ ] 5.1 Create `internal/service/config_list.go` with service struct
  - [ ] 5.1a Write tests with mock dependencies (Parser, Chezmoi, StatusDetector interfaces)
  - [ ] 5.1b Define `ConfigListService` struct
  - [ ] 5.1c Add dependencies: Parser interface, Chezmoi interface, StatusDetector interface, zerbDir string
  - [ ] 5.1d Define request/response types
- [ ] 5.2 Implement List method for active config only
  - [ ] 5.2a Write tests for active config listing
  - [ ] 5.2b Read active marker from zerbDir (passed to constructor)
  - [ ] 5.2c Parse active config via Parser interface
  - [ ] 5.2d Detect status via StatusDetector interface
  - [ ] 5.2e Return sorted results (alphabetical by path)
- [ ] 5.3 Add context support throughout service
  - [ ] 5.3a Accept context in all methods
  - [ ] 5.3b Pass context to parser, chezmoi, status detector
  - [ ] 5.3c Handle context cancellation gracefully

## 6. Output Formatting (MVP: Table and Verbose only)
- [ ] 6.1 Implement table formatter (default output)
  - [ ] 6.1a Write tests for table formatting
  - [ ] 6.1b Create table header: STATUS, PATH, FLAGS
  - [ ] 6.1c Format each row with aligned columns
  - [ ] 6.1d Add summary line with counts (synced, missing, partial)
  - [ ] 6.1e Preallocate slices based on len(configs) for efficiency
- [ ] 6.2 Implement verbose formatter
  - [ ] 6.2a Write tests for verbose output
  - [ ] 6.2b Add SIZE column (get file size from disk via os.Stat)
  - [ ] 6.2c Add LAST MODIFIED column (relative time: "2 hours ago")
  - [ ] 6.2d Add notes section explaining status indicators
- [ ] 6.3 Implement status indicator symbols (MVP: 3 statuses)
  - [ ] 6.3a Write tests for status symbols
  - [ ] 6.3b Map Synced → "✓"
  - [ ] 6.3c Map Missing → "✗"
  - [ ] 6.3d Map Partial → "?"
- [ ] 6.4 Implement flags column formatting
  - [ ] 6.4a Write tests for flag display
  - [ ] 6.4b Show only enabled flags (omit false values)
  - [ ] 6.4c Join multiple flags with ", " separator
  - [ ] 6.4d Handle empty flags (no output)
  - [ ] 6.6a Write tests for flag display
  - [ ] 6.6b Show only enabled flags (omit false values)
  - [ ] 6.6c Join multiple flags with ", " separator
  - [ ] 6.6d Handle empty flags (no output)

## 7. Error Handling and Edge Cases
- [ ] 7.1 Handle ZERB not initialized
  - [ ] 7.1a Write tests for uninitialized state
  - [ ] 7.1b Check for `.zerb-active` marker existence
  - [ ] 7.1c Define sentinel error `ErrNotInitialized` in service layer
  - [ ] 7.1d Return clear error: "ZERB not initialized. Run 'zerb init' first"
- [ ] 7.2 Handle no configs tracked
  - [ ] 7.2a Write tests for empty config list
  - [ ] 7.2b Display friendly message: "No configs tracked yet"
  - [ ] 7.2c Suggest: "Add configs with: zerb config add <path>"
- [ ] 7.3 Handle corrupted config files
  - [ ] 7.3a Write tests for Lua parse errors
  - [ ] 7.3b Catch parser errors gracefully via error wrapping (%w)
  - [ ] 7.3c Show error with config filename (sanitized via existing helpers)
  - [ ] 7.3d Suggest recovery options
- [ ] 7.4 Handle permission errors
  - [ ] 7.4a Write tests for permission denied scenarios
  - [ ] 7.4b Use errors.Is to detect permission errors
  - [ ] 7.4c Show clear error message with redacted paths
  - [ ] 7.4d Suggest checking file permissions

## 8. Integration and End-to-End Testing (MVP Scope)
- [ ] 8.1 Test default list (active config only)
  - [ ] 8.1a Create test fixture with sample zerb.lua
  - [ ] 8.1b Run `zerb config list`
  - [ ] 8.1c Verify table output format
  - [ ] 8.1d Verify correct status detection (Synced, Missing, Partial)
- [ ] 8.2 Test `--verbose` flag
  - [ ] 8.2a Create configs with known sizes
  - [ ] 8.2b Run `zerb config list --verbose`
  - [ ] 8.2c Verify SIZE and LAST MODIFIED columns
  - [ ] 8.2d Verify notes section present
- [ ] 8.3 Test status detection accuracy
  - [ ] 8.3a Create synced config (exists, managed)
  - [ ] 8.3b Create missing config (delete file after add)
  - [ ] 8.3c Create partial config (in zerb.lua but not managed)
  - [ ] 8.3d Verify correct status for each
- [ ] 8.4 Test with no configs
  - [ ] 8.4a Initialize ZERB but don't add configs
  - [ ] 8.4b Run `zerb config list`
  - [ ] 8.4c Verify friendly "no configs" message
- [ ] 8.5 Test with uninitialized ZERB
  - [ ] 8.5a Run in directory without ZERB
  - [ ] 8.5b Run `zerb config list`
  - [ ] 8.5c Verify clear initialization error
- [ ] 8.6 Test context cancellation (Ctrl+C)
  - [ ] 8.6a Send SIGINT during operation
  - [ ] 8.6b Verify no partial output shown
  - [ ] 8.6c Verify exit code 130

## 9. Documentation and Help
- [ ] 9.1 Add command to main help output
  - [ ] 9.1a Update `cmd/zerb/main.go` help text
  - [ ] 9.1b Show `zerb config list` in command list
  - [ ] 9.1c Add brief description
- [ ] 9.2 Create comprehensive `--help` output
  - [ ] 9.2a Document flags (--verbose, --timeout)
  - [ ] 9.2b Add usage examples
  - [ ] 9.2c Explain status indicators (✓, ✗, ?)
  - [ ] 9.2d Show sample output
- [ ] 9.3 Add inline code comments
  - [ ] 9.3a Document status detection logic
  - [ ] 9.3b Explain formatter implementations
  - [ ] 9.3c Add package-level documentation

## 10. Performance and Polish (MVP)
- [ ] 10.1 Optimize for large config lists
  - [ ] 10.1a Preallocate slices based on len(Configs)
  - [ ] 10.1b Benchmark with 100+ configs
  - [ ] 10.1c Ensure sub-second response time
- [ ] 10.2 Handle terminal width gracefully (optional)
  - [ ] 10.2a Detect terminal width
  - [ ] 10.2b Truncate long paths if needed
  - [ ] 10.2c Add "..." indicator for truncation

## Task Summary

**MVP Task Count**: ~45 tasks (reduced from original ~85)
**Test Coverage Goal**: >80% for all new code
**TDD Compliance**: MANDATORY for all tasks

**Deferred to Future**:
- JSON output format (~10 tasks)
- Plain output format (~5 tasks)
- Historical config listing with `--all` (~15 tasks)
- Drift detection with file hash comparison (~8 tasks)
- Color support (~4 tasks)

**Dependencies**:
- Existing: `internal/config` (Parser)
- Existing: `internal/chezmoi` (Client)
- New: `internal/config/status.go`
- New: `internal/service/config_list.go`
- New: `cmd/zerb/config_list.go`
