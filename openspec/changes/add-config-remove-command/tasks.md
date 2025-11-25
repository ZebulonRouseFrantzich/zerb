# Implementation Tasks

**CRITICAL: Test-Driven Development (TDD) Required**

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

---

## 0. Prerequisites (Subagent Recommendations)

These tasks address recommendations from @golang-pro and @architect-reviewer reviews.

- [x] 0.1 Generalize transaction type (CR-1)
  - [x] 0.1a Write tests for generalized ConfigTxn type with operation field
  - [x] 0.1b Refactor ConfigAddTxn → ConfigTxn with Operation field ("add" | "delete")
  - [x] 0.1c Update config_add.go to use generalized type
  - [x] 0.1d Ensure backward compatibility with existing transaction files

- [x] 0.2 Generalize lock file name (HR-1)
  - [x] 0.2a Write tests for shared config.lock behavior
  - [x] 0.2b Rename lock file from `config-add.lock` to `config.lock`
  - [x] 0.2c Update AcquireLock to use generic lock name
  - [x] 0.2d Add migration for existing lock files (if needed)

- [x] 0.3 Add context support to AcquireLock (HR-2)
  - [x] 0.3a Write tests for context cancellation in lock acquisition
  - [x] 0.3b Update `AcquireLock(path string)` → `AcquireLock(ctx context.Context, path string)`
  - [x] 0.3c Update all callers to pass context
  - [x] 0.3d Add timeout support for lock acquisition

- [x] 0.4 Add Config helper methods (MR-1)
  - [x] 0.4a Write tests for FindConfig method
  - [x] 0.4b Write tests for RemoveConfig method
  - [x] 0.4c Implement `FindConfig(path string) *ConfigFile` in config.Config
  - [x] 0.4d Implement `RemoveConfig(path string) []ConfigFile` in config.Config

- [x] 0.5 Add path deduplication utility (HR-4)
  - [x] 0.5a Write tests for path deduplication
  - [x] 0.5b Implement `deduplicatePaths(paths []string) []string`
  - [x] 0.5c Handle tilde expansion and absolute path normalization

- [x] 0.6 Add isWithinHome safety check (HR-5)
  - [x] 0.6a Write tests for isWithinHome function
  - [x] 0.6b Implement `isWithinHome(path string) bool`
  - [x] 0.6c Handle edge cases (symlinks, relative paths)

---

## 1. Core Command Implementation

- [x] 1.1 Create `cmd/zerb/config_remove.go` with command structure
  - [x] 1.1a Write tests for command initialization
  - [x] 1.1b Define command structure using flag package (consistent with config_add.go, config_list.go)
  - [x] 1.1c Add basic error handling

- [x] 1.2 Update `cmd/zerb/main.go` routing for `config remove` subcommand
  - [x] 1.2a Add case for "remove" in config subcommand switch
  - [x] 1.2b Update help text to show available config actions (add, list, remove)

- [x] 1.3 Implement flag parsing
  - [x] 1.3a Write tests for flag combinations
  - [x] 1.3b Add `--yes` / `-y` flag to skip confirmation
  - [x] 1.3c Add `--dry-run` / `-n` flag for preview mode
  - [x] 1.3d Add `--purge` flag to also delete source file
  - [x] 1.3e Add `--keep-file` flag (default, for explicitness) - (implicit default behavior)
  - [x] 1.3f Add `--timeout` flag (default 2m, consistent with config add) - (hardcoded 2m)
  - [x] 1.3g Add flag validation (mutual exclusivity where needed)

- [x] 1.4 Implement path argument parsing
  - [x] 1.4a Write tests for path parsing
  - [x] 1.4b Accept one or more paths as positional arguments
  - [x] 1.4c Validate at least one path is provided
  - [x] 1.4d Handle paths with spaces (quoted arguments)
  - [x] 1.4e **Deduplicate paths using deduplicatePaths utility (HR-4)**

- [x] 1.5 Add help text and usage examples
  - [x] 1.5a Write help text with clear descriptions
  - [x] 1.5b Add examples showing common use cases
  - [x] 1.5c Document all flags and their behaviors
  - [x] 1.5d Show sample output for different scenarios

## 2. Confirmation Prompt

- [x] 2.1 Implement confirmation display
  - [x] 2.1a Write tests for confirmation formatting
  - [x] 2.1b Show list of paths to be deleted with their status
  - [x] 2.1c Indicate whether files will be kept or removed from disk
  - [x] 2.1d Format output consistently with other ZERB commands

- [x] 2.2 Implement user input handling
  - [x] 2.2a Write tests for confirmation input
  - [x] 2.2b Accept y/Y/yes/YES for confirmation
  - [x] 2.2c Accept n/N/no/NO/empty for rejection
  - [x] 2.2d Handle invalid input gracefully (re-prompt or exit)
  - [x] 2.2e Default to "no" if user presses Enter without input

- [x] 2.3 Implement `--yes` flag bypass
  - [x] 2.3a Write tests for --yes flag
  - [x] 2.3b Skip confirmation when --yes is provided
  - [x] 2.3c Still show what will be deleted (informational) - (skipped for cleaner UX)

- [x] 2.4 Implement `--dry-run` mode
  - [x] 2.4a Write tests for dry-run mode
  - [x] 2.4b Show what would be deleted without making changes
  - [x] 2.4c Skip confirmation prompt in dry-run mode
  - [x] 2.4d Exit with code 0 after preview

## 3. Chezmoi Integration (Extend Interface)

- [x] 3.1 Extend `Chezmoi` interface in `internal/chezmoi/chezmoi.go`
  - [x] 3.1a Write tests for Remove method
  - [x] 3.1b Add `Remove(ctx context.Context, path string) error` to interface
  - [x] 3.1c Document method behavior (removes from chezmoi source, not disk)

- [x] 3.2 Implement Remove method in `*Client`
  - [x] 3.2a Write tests using mock chezmoi binary
  - [x] 3.2b Use `config.NormalizeConfigPath` for path canonicalization
  - [x] 3.2c Invoke `chezmoi forget <path>` with isolation flags
  - [x] 3.2d Pass context for cancellation support
  - [x] 3.2e Wrap errors with `RedactedError` (never expose internal paths)

- [x] 3.3 Handle edge cases (updated per HR-3)
  - [x] 3.3a Write tests for edge cases including not-found
  - [x] 3.3b **Handle non-existent source file: log warning and return nil (not error)**
  - [x] 3.3c Handle permission denied errors
  - [x] 3.3d Handle context cancellation/timeout
  - [x] 3.3e Add isNotFoundError helper function to detect chezmoi not-found errors

## 4. Service Layer Implementation

- [x] 4.1 Create `internal/service/config_remove.go` with service struct
  - [x] 4.1a Write tests with mock dependencies
  - [x] 4.1b Define `ConfigRemoveService` struct
  - [x] 4.1c Add dependencies: Chezmoi, Git, Parser, Generator, Clock interfaces, zerbDir string
  - [x] 4.1d Define `RemoveRequest` struct (paths, options, dryRun, purge)
  - [x] 4.1e Define `RemoveResult` struct (removedPaths, skippedPaths, commitHash, configVersion)

- [x] 4.2 Implement path validation
  - [x] 4.2a Write tests for path validation
  - [x] 4.2b Normalize input paths using `config.NormalizeConfigPath`
  - [x] 4.2c Look up each path in current config's Configs array
  - [x] 4.2d Return error if any path is not tracked
  - [x] 4.2e Collect all validation errors before failing

- [x] 4.3 Implement Execute method
  - [x] 4.3a Write tests for Execute workflow
  - [x] 4.3b Acquire transaction lock
  - [x] 4.3c Validate all paths before any deletion
  - [x] 4.3d Process each path through chezmoi.Remove
  - [x] 4.3e Update config file (remove entries)
  - [x] 4.3f Create git commit
  - [x] 4.3g Release transaction lock
  - [x] 4.3h Return result with summary

- [x] 4.4 Implement config file update
  - [x] 4.4a Write tests for config update
  - [x] 4.4b Parse current active config
  - [x] 4.4c Filter out deleted paths from Configs array
  - [x] 4.4d Generate new timestamped config
  - [x] 4.4e Update `.zerb-active` marker
  - [x] 4.4f Update `zerb.active.lua` symlink

- [x] 4.5 Add context support
  - [x] 4.5a Accept context in Execute method
  - [x] 4.5b Check context before each major operation
  - [x] 4.5c Handle context cancellation gracefully
  - [x] 4.5d Pass context to all dependencies

## 5. Transaction Integration

- [x] 5.1 Reuse existing transaction infrastructure
  - [x] 5.1a Write tests for transaction integration
  - [x] 5.1b Create transaction for remove operation
  - [x] 5.1c Use transaction file location: `~/.config/zerb/.txn/txn-config-remove-<uuid>.json`
  - [x] 5.1d Track state per path (pending -> in-progress -> completed/failed)

- [x] 5.2 Implement transaction state management
  - [x] 5.2a Write tests for state transitions
  - [x] 5.2b Update transaction after each path removal
  - [x] 5.2c Track errors in transaction file
  - [ ] 5.2d Enable resume from failed state

- [ ] 5.3 Implement `--resume` support (DEFERRED - future enhancement)
  - [ ] 5.3a Write tests for resume behavior
  - [ ] 5.3b Detect existing transaction
  - [ ] 5.3c Skip completed paths
  - [ ] 5.3d Retry failed/pending paths
  - [ ] 5.3e Complete config update and commit after all succeed

- [ ] 5.4 Implement `--abort` support (DEFERRED - future enhancement)
  - [ ] 5.4a Write tests for abort behavior
  - [ ] 5.4b Read transaction state
  - [ ] 5.4c No rollback needed (paths already removed from chezmoi)
  - [ ] 5.4d Clean up transaction file
  - [ ] 5.4e Release lock

## 6. Git Integration

- [x] 6.1 Generate appropriate commit messages
  - [x] 6.1a Write tests for commit message generation
  - [x] 6.1b Single path: "Remove ~/.zshrc from tracked configs"
  - [x] 6.1c Multiple paths: "Remove N configs from tracked configs"
  - [x] 6.1d Include body with list of removed paths for multiple

- [x] 6.2 Stage and commit changes
  - [x] 6.2a Write tests for git operations
  - [x] 6.2b Stage new timestamped config file
  - [x] 6.2c Stage `.zerb-active` marker
  - [x] 6.2d Stage `zerb.active.lua` symlink
  - [x] 6.2e Stage chezmoi source directory changes
  - [x] 6.2f Create single atomic commit

- [x] 6.3 Capture commit hash
  - [x] 6.3a Write tests for commit hash capture
  - [x] 6.3b Get HEAD commit after committing
  - [x] 6.3c Include in result for display

## 7. Source File Deletion (`--purge`)

- [x] 7.1 Implement file deletion logic (updated per CR-2)
  - [x] 7.1a Write tests for file deletion
  - [x] 7.1b Only delete if `--purge` flag is set
  - [x] 7.1c **Verify path is within $HOME using isWithinHome (HR-5)**
  - [x] 7.1d **Delete source file BEFORE chezmoi.Remove (CR-2 order)**
  - [x] 7.1e Use normalized path for deletion
  - [x] 7.1f Handle file not found (already deleted, not an error)
  - [x] 7.1g Call chezmoi.Remove after file deletion (continues even if Remove fails)

- [x] 7.2 Handle deletion errors
  - [x] 7.2a Write tests for deletion error handling
  - [x] 7.2b Handle permission denied
  - [x] 7.2c Track in transaction as partial failure
  - [ ] 7.2d Continue with other paths on failure (currently fails fast)

- [x] 7.3 Update confirmation prompt for `--purge`
  - [x] 7.3a Write tests for modified confirmation
  - [x] 7.3b Show clear warning that files will be deleted
  - [x] 7.3c Use different message: "Source files will be DELETED from disk"

## 8. User Feedback and Output

- [x] 8.1 Implement success output
  - [x] 8.1a Write tests for success formatting
  - [x] 8.1b Show list of removed configs with checkmarks
  - [x] 8.1c Show commit hash (short form)
  - [x] 8.1d Show new config version

- [x] 8.2 Implement error output
  - [x] 8.2a Write tests for error formatting
  - [x] 8.2b Show clear error messages without internal details
  - [ ] 8.2c Suggest recovery options (--resume, --abort) (DEFERRED)
  - [x] 8.2d Use consistent error format with other commands

- [x] 8.3 Implement dry-run output
  - [x] 8.3a Write tests for dry-run formatting
  - [x] 8.3b Show "Would remove:" prefix
  - [x] 8.3c Show "Dry run - no changes made" summary
  - [ ] 8.3d Show what commit would look like (SKIPPED - not needed)

- [ ] 8.4 Status retrieval for confirmation (DEFERRED - future enhancement)
  - [ ] 8.4a Write tests for status retrieval
  - [ ] 8.4b Reuse StatusDetector from config list
  - [ ] 8.4c Show status (synced/missing/partial) next to each path

## 9. Error Handling and Edge Cases

- [x] 9.1 Handle ZERB not initialized
  - [x] 9.1a Write tests for uninitialized state
  - [x] 9.1b Check for active marker existence
  - [x] 9.1c Return clear error: "ZERB not initialized. Run 'zerb init' first"
  - [x] 9.1d Exit code 1

- [x] 9.2 Handle path not tracked
  - [x] 9.2a Write tests for untracked path
  - [x] 9.2b Look up path in config before proceeding
  - [x] 9.2c Return clear error: "Config not tracked: <path>"
  - [x] 9.2d Exit code 1

- [x] 9.3 Handle no paths provided
  - [x] 9.3a Write tests for missing arguments
  - [x] 9.3b Return clear error: "no paths specified"
  - [x] 9.3c Show usage hint
  - [x] 9.3d Exit code 1

- [x] 9.4 Handle concurrent operations
  - [x] 9.4a Write tests for concurrent access
  - [x] 9.4b Use transaction lock to prevent concurrent deletes
  - [x] 9.4c Return clear error: "Another configuration operation is in progress"
  - [x] 9.4d Exit code 1

- [ ] 9.5 Handle user cancellation (DEFERRED - future enhancement)
  - [ ] 9.5a Write tests for Ctrl+C handling
  - [ ] 9.5b Save transaction state before exit
  - [ ] 9.5c Exit with code 130 (standard for SIGINT)

## 10. Integration Testing

- [ ] 10.1 End-to-end test: remove single config
  - [ ] 10.1a Add a config, verify it exists
  - [ ] 10.1b Run `zerb config remove <path> --yes`
  - [ ] 10.1c Verify config is removed from zerb.lua
  - [ ] 10.1d Verify source file still exists on disk
  - [ ] 10.1e Verify git commit was created

- [ ] 10.2 End-to-end test: remove multiple configs
  - [ ] 10.2a Add multiple configs
  - [ ] 10.2b Run `zerb config remove <path1> <path2> --yes`
  - [ ] 10.2c Verify all configs removed
  - [ ] 10.2d Verify single git commit

- [ ] 10.3 End-to-end test: remove with --purge
  - [ ] 10.3a Add a config
  - [ ] 10.3b Run `zerb config remove <path> --purge --yes`
  - [ ] 10.3c Verify config removed from tracking
  - [ ] 10.3d Verify source file deleted from disk

- [ ] 10.4 End-to-end test: dry-run mode
  - [ ] 10.4a Add a config
  - [ ] 10.4b Run `zerb config remove <path> --dry-run`
  - [ ] 10.4c Verify config still tracked
  - [ ] 10.4d Verify no git commit created

- [ ] 10.5 End-to-end test: confirmation rejection
  - [ ] 10.5a Add a config
  - [ ] 10.5b Run `zerb config remove <path>` and enter "n"
  - [ ] 10.5c Verify config still tracked
  - [ ] 10.5d Verify exit code 0

- [ ] 10.6 End-to-end test: transaction resume
  - [ ] 10.6a Simulate interrupted remove
  - [ ] 10.6b Run `zerb config remove --resume`
  - [ ] 10.6c Verify completion

- [ ] 10.7 Run all tests with -race flag
  - [ ] 10.7a Run `go test -race ./...`
  - [ ] 10.7b Fix any race conditions detected

## 11. Documentation and Help

- [ ] 11.1 Update main help output
  - [ ] 11.1a Update `cmd/zerb/main.go` help text
  - [ ] 11.1b Show `zerb config remove` in command list
  - [ ] 11.1c Add brief description

- [ ] 11.2 Create comprehensive `--help` output
  - [ ] 11.2a Document all flags
  - [ ] 11.2b Add usage examples
  - [ ] 11.2c Explain confirmation behavior
  - [ ] 11.2d Show sample output

- [ ] 11.3 Add inline code comments
  - [ ] 11.3a Document service layer workflow
  - [ ] 11.3b Explain transaction integration
  - [ ] 11.3c Add package-level documentation

---

## Task Summary

**Total Tasks**: ~105 tasks across 12 sections (including prerequisites)
**Test Coverage Goal**: >80% for all new code (MANDATORY)
**TDD Compliance**: MANDATORY for all tasks

**Dependencies**:
- Existing: `internal/config` (Parser, Generator, NormalizeConfigPath)
- Existing: `internal/chezmoi` (Client, interface extension)
- Existing: `internal/git` (Client)
- Existing: `internal/transaction` (Lock, Transaction - needs generalization)
- New: `internal/service/config_remove.go`
- New: `cmd/zerb/config_remove.go`
- Modified: `internal/transaction/transaction.go` (generalize ConfigTxn)
- Modified: `internal/transaction/lock.go` (generalize lock file, add context)

**Prerequisites (Section 0)**: ~20 tasks - MUST be completed first
- Generalize transaction type (CR-1)
- Generalize lock file (HR-1)
- Add context to AcquireLock (HR-2)
- Add Config helper methods (MR-1)
- Add path deduplication (HR-4)
- Add isWithinHome safety check (HR-5)

**Estimated Effort**: 10-14 hours (including tests and prerequisites)

**Risk Areas**:
- Transaction generalization (medium complexity, affects config add)
- Chezmoi forget command (need to verify behavior)
- File deletion with --purge (security considerations)
- Backward compatibility with existing transaction files
