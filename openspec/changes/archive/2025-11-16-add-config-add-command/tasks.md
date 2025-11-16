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

---

## 1. Core Command Implementation
- [x] 1.1 Create `cmd/zerb/config_add.go` with command structure
- [x] 1.2 Add command routing in `cmd/zerb/main.go` for `config add` subcommand
- [x] 1.3 Implement argument parsing (paths) and flag parsing (--recursive, --template, --secrets, --private)
- [x] 1.4 Add help text and usage examples
- [x] 1.5 Write unit tests for argument and flag parsing

## 2. Path Validation and Processing
- [x] 2.1 **CRITICAL FIX**: Rewrite `validateConfigPath()` in `internal/config/types.go` to fix security flaws
  - [x] 2.1a Write security tests for path traversal attacks, symlink escapes, absolute paths
  - [x] 2.1b Implement canonical path checking with filepath.EvalSymlinks
  - [x] 2.1c Use filepath.Rel instead of strings.Contains for traversal detection
  - [x] 2.1d Allow absolute paths within $HOME (fix current bug)
  - [x] 2.1e Verify all security tests pass
- [x] 2.2 Add tilde expansion for home directory paths
- [x] 2.3 **CHANGED**: Error if paths don't exist (fail fast, not warn)
  - [x] 2.3a Write tests for non-existent path rejection
  - [x] 2.3b Implement existence check before transaction creation
  - [x] 2.3c Return clear error message with path
- [x] 2.4 Detect directories and require `--recursive` flag (error with helpful message)
- [x] 2.5 Implement path normalization for duplicate detection
  - [x] 2.5a Write tests for tilde vs absolute path equivalence
  - [x] 2.5b Normalize all paths to canonical form for comparison
  - [x] 2.5c Handle trailing slashes and case sensitivity
- [x] 2.6 Write comprehensive path validation tests
  - [x] 2.6a Test path traversal attempts (~/../etc/passwd)
  - [x] 2.6b Test symlink escape attempts
  - [x] 2.6c Test symlinks within home (should pass)
  - [x] 2.6d Test absolute paths inside home (should pass)
  - [x] 2.6e Test paths with literal ".." in names
  - [x] 2.6f Test directory detection
  - [x] 2.6g Test non-existent path rejection

## 3. Chezmoi Integration (Wrapper)
- [x] 3.1 Define Chezmoi interface for testability
  - [x] 3.1a Write interface definition with Add(ctx, path, opts) method
  - [x] 3.1b Define AddOptions struct
  - [x] 3.1c Define error types (ErrChezmoiInvocation, etc.)
- [x] 3.2 Create `internal/chezmoi/` package structure
- [x] 3.3 Implement Client struct with context support
  - [x] 3.3a Write tests using stubbed binary
  - [x] 3.3b Implement NewClient(zerbDir) constructor
  - [x] 3.3c Implement Add method with exec.CommandContext
  - [x] 3.3d Add isolated flags (--source, --config)
  - [x] 3.3e Scrub environment variables for complete isolation
- [x] 3.4 Implement error abstraction layer
  - [x] 3.4a Write tests for translateChezmoiError function
  - [x] 3.4b Map common chezmoi errors to user-friendly messages
  - [x] 3.4c Ensure "chezmoi" never appears in user-facing errors
  - [x] 3.4d Redact sensitive information from stderr
- [x] 3.5 Write integration tests
  - [x] 3.5a Create stub chezmoi binary for tests
  - [x] 3.5b Test successful add operations
  - [x] 3.5c Test error conditions (permission denied, file not found)
  - [x] 3.5d Test context cancellation
  - [x] 3.5e Test context timeout

## 4. Config File Updates
- [x] 4.1 Read current active config (`zerb.lua.active`)
- [x] 4.2 Parse existing config using `internal/config.Parser`
- [x] 4.3 Add new ConfigFile entry to the Configs array
- [x] 4.4 Detect duplicates and skip or warn appropriately
- [x] 4.5 Generate new Lua config using `internal/config.Generator`
- [x] 4.6 Create new timestamped config file in `configs/` directory
- [x] 4.7 Update `.zerb-active` marker and `zerb.lua.active` symlink
- [x] 4.8 Write tests for config update logic

## 5. Git Integration
- [x] 5.1 Define Git interface for testability
  - [x] 5.1a Write interface with Stage(ctx, files...) and Commit(ctx, msg, body) methods
  - [x] 5.1b Create internal/git package
- [x] 5.2 Implement Git client with context support
  - [x] 5.2a Write tests using mock git commands
  - [x] 5.2b Implement Stage method
  - [x] 5.2c Implement Commit method
  - [x] 5.2d Add proper error handling and wrapping
- [x] 5.3 Generate appropriate commit messages
  - [x] 5.3a Write tests for commit message generation
  - [x] 5.3b Single file: "Add ~/.zshrc to tracked configs"
  - [x] 5.3c Multiple files: "Add N configs to tracked configs" with body
  - [x] 5.3d Handle long path lists (truncation if needed)
- [x] 5.4 Integrate staging and committing
  - [x] 5.4a Stage timestamped config file
  - [x] 5.4b Stage chezmoi source files
  - [x] 5.4c Create single commit for all changes
  - [x] 5.4d Write integration tests

## 6. User Feedback and UX
- [x] 6.1 Show preview of config changes before applying
- [x] 6.2 Add confirmation prompt (optional: support --yes flag to skip)
- [x] 6.3 Display success message with next steps
- [x] 6.4 Handle errors gracefully with actionable messages
- [x] 6.5 Implement transaction file for multi-path operations
- [x] 6.6 Add `--resume` flag to continue interrupted operations
- [x] 6.7 Add `--abort` flag to cancel incomplete transactions
- [x] 6.8 Provide rollback instructions on fatal errors
- [ ] 6.9 Write user acceptance tests for happy path

## 7. Service Layer and Interface-Based Design
- [x] 7.1 Define Clock interface for deterministic timestamps
  - [x] 7.1a Write interface with Now() method
  - [x] 7.1b Implement RealClock and TestClock
- [x] 7.2 Create ConfigAddService with dependency injection
  - [x] 7.2a Write tests using mock interfaces
  - [x] 7.2b Define service struct with Chezmoi, Git, Config, Clock interfaces
  - [x] 7.2c Implement Execute method with full workflow
  - [x] 7.2d Accept context for cancellation support
- [x] 7.3 Wire up command with service layer
  - [x] 7.3a Instantiate real implementations (Client, Git, etc.)
  - [x] 7.3b Inject into service
  - [x] 7.3c Call service.Execute from runConfigAdd
  - [x] 7.3d Handle errors and user messages at command boundary

## 8. Documentation and Examples
- [ ] 8.1 Update README.md with `zerb config add` examples
- [ ] 8.2 Add command documentation
- [x] 8.3 Update examples/full.lua with config examples (already exists)
- [ ] 8.4 Add troubleshooting section for common errors
- [ ] 8.5 Document transaction recovery (--resume, --abort)
- [ ] 8.6 Document security model (path validation)

## 9. Integration Testing
- [ ] 9.1 End-to-end test: `zerb config add ~/.zshrc`
- [ ] 9.2 End-to-end test: `zerb config add ~/.config/nvim --recursive`
- [ ] 9.3 Test duplicate detection with normalized paths
- [ ] 9.4 Test with multiple files in one command
- [ ] 9.5 Test error cases (invalid paths, permissions issues)
- [ ] 9.6 Test directory without `--recursive` flag (error)
- [ ] 9.7 Test non-existent path rejection
- [ ] 9.8 Test transaction resume after interruption
- [ ] 9.9 Test transaction abort with automatic cleanup
- [ ] 9.10 Test concurrent invocation prevention
- [ ] 9.11 Test context cancellation (Ctrl+C)
- [x] 9.12 Test path validation security (symlink escape, traversal)
- [x] 9.13 Run all tests with -race flag

## 10. Transaction Management
- [x] 9.1 Design transaction file JSON schema with versioning
  - [x] 9.1a Define ConfigAddTxn struct with version, id (UUID), timestamp
  - [x] 9.1b Define PathTxn struct with state, flags, created_source_files, last_error
  - [x] 9.1c Write JSON marshaling tests
- [x] 9.2 Implement lock file mechanism
  - [x] 9.2a Write tests for lock acquisition with O_CREATE|O_EXCL
  - [x] 9.2b Implement lock creation in ~/.config/zerb/tmp/config-add.lock
  - [x] 9.2c Handle lock conflicts (return ErrTransactionExists)
  - [x] 9.2d Implement lock release on defer
  - [x] 9.2e Add optional --force-stale-lock for locks >10 minutes old
- [x] 9.3 Implement atomic transaction file writes
  - [x] 9.3a Write tests for atomic write-then-rename pattern
  - [x] 9.3b Implement writeTxnAtomic(dir, name, txn) function
  - [x] 9.3c Write to temp file (.tmp)
  - [x] 9.3d Atomic rename to final path
  - [x] 9.3e Fsync directory for durability
  - [x] 9.3f Set permissions to 0600
- [x] 9.4 **CHANGED**: Update transaction location to ~/.config/zerb/tmp/txn-config-add-<uuid>.json
  - [x] 9.4a Create tmp directory with 0700 permissions if needed
  - [x] 9.4b Generate UUID for unique transaction ID
  - [x] 9.4c Write tests for location and permissions
- [x] 9.5 Implement state tracking for each path
  - [x] 9.5a Track state transitions: pending → in-progress → completed | failed
  - [x] 9.5b Persist transaction after each state change (atomic write)
  - [x] 9.5c Record created_source_files array for each path
  - [x] 9.5d Record errors in last_error field
- [x] 9.6 Implement `--resume` logic
  - [x] 9.6a Write tests for resume behavior
  - [x] 9.6b Read existing transaction file
  - [x] 9.6c Skip paths in "completed" state (idempotent)
  - [x] 9.6d Retry paths in "failed" or "pending" state
  - [x] 9.6e Complete config update and git commit after all succeed
  - [x] 9.6f Delete transaction and release lock on success
- [x] 9.7 Implement `--abort` logic with automatic cleanup
  - [x] 9.7a Write tests for abort with file cleanup
  - [x] 9.7b Read created_source_files from transaction
  - [x] 9.7c Attempt to remove each created file
  - [x] 9.7d Delete transaction file
  - [x] 9.7e Release lock
  - [x] 9.7f Provide manual instructions if cleanup fails
- [x] 9.8 Ensure single git commit after all paths succeed
  - [x] 9.8a Only update zerb.lua after all paths completed
  - [x] 9.8b Only git commit after config update succeeds
  - [x] 9.8c Include all changes in single commit
- [x] 9.9 Implement context support in transaction operations
  - [x] 9.9a Pass context to all blocking operations
  - [x] 9.9b Check context.Err() between path processing
  - [x] 9.9c Persist transaction state before returning on cancellation
  - [x] 9.9d Write tests for Ctrl+C simulation
- [ ] 9.10 Write comprehensive transaction tests
  - [ ] 9.10a Test full transaction lifecycle (create → process → commit → cleanup)
  - [ ] 9.10b Test resume from interrupted state
  - [ ] 9.10c Test abort with cleanup
  - [ ] 9.10d Test concurrent invocation prevention
  - [ ] 9.10e Test partial failures
  - [ ] 9.10f Test context cancellation
  - [ ] 9.10g Test atomic writes with simulated crashes
  - [ ] 9.10h Run with -race flag to detect races
