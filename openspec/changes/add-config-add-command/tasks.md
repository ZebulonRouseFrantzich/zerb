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
- [ ] 1.1 Create `cmd/zerb/config_add.go` with command structure
- [ ] 1.2 Add command routing in `cmd/zerb/main.go` for `config add` subcommand
- [ ] 1.3 Implement argument parsing (paths) and flag parsing (--recursive, --template, --secrets, --private)
- [ ] 1.4 Add help text and usage examples
- [ ] 1.5 Write unit tests for argument and flag parsing

## 2. Path Validation and Processing
- [ ] 2.1 **CRITICAL FIX**: Rewrite `validateConfigPath()` in `internal/config/types.go` to fix security flaws
  - [ ] 2.1a Write security tests for path traversal attacks, symlink escapes, absolute paths
  - [ ] 2.1b Implement canonical path checking with filepath.EvalSymlinks
  - [ ] 2.1c Use filepath.Rel instead of strings.Contains for traversal detection
  - [ ] 2.1d Allow absolute paths within $HOME (fix current bug)
  - [ ] 2.1e Verify all security tests pass
- [ ] 2.2 Add tilde expansion for home directory paths
- [ ] 2.3 **CHANGED**: Error if paths don't exist (fail fast, not warn)
  - [ ] 2.3a Write tests for non-existent path rejection
  - [ ] 2.3b Implement existence check before transaction creation
  - [ ] 2.3c Return clear error message with path
- [ ] 2.4 Detect directories and require `--recursive` flag (error with helpful message)
- [ ] 2.5 Implement path normalization for duplicate detection
  - [ ] 2.5a Write tests for tilde vs absolute path equivalence
  - [ ] 2.5b Normalize all paths to canonical form for comparison
  - [ ] 2.5c Handle trailing slashes and case sensitivity
- [ ] 2.6 Write comprehensive path validation tests
  - [ ] 2.6a Test path traversal attempts (~/../etc/passwd)
  - [ ] 2.6b Test symlink escape attempts
  - [ ] 2.6c Test symlinks within home (should pass)
  - [ ] 2.6d Test absolute paths inside home (should pass)
  - [ ] 2.6e Test paths with literal ".." in names
  - [ ] 2.6f Test directory detection
  - [ ] 2.6g Test non-existent path rejection

## 3. Chezmoi Integration (Wrapper)
- [ ] 3.1 Define Chezmoi interface for testability
  - [ ] 3.1a Write interface definition with Add(ctx, path, opts) method
  - [ ] 3.1b Define AddOptions struct
  - [ ] 3.1c Define error types (ErrChezmoiInvocation, etc.)
- [ ] 3.2 Create `internal/chezmoi/` package structure
- [ ] 3.3 Implement Client struct with context support
  - [ ] 3.3a Write tests using stubbed binary
  - [ ] 3.3b Implement NewClient(zerbDir) constructor
  - [ ] 3.3c Implement Add method with exec.CommandContext
  - [ ] 3.3d Add isolated flags (--source, --config)
  - [ ] 3.3e Scrub environment variables for complete isolation
- [ ] 3.4 Implement error abstraction layer
  - [ ] 3.4a Write tests for translateChezmoiError function
  - [ ] 3.4b Map common chezmoi errors to user-friendly messages
  - [ ] 3.4c Ensure "chezmoi" never appears in user-facing errors
  - [ ] 3.4d Redact sensitive information from stderr
- [ ] 3.5 Write integration tests
  - [ ] 3.5a Create stub chezmoi binary for tests
  - [ ] 3.5b Test successful add operations
  - [ ] 3.5c Test error conditions (permission denied, file not found)
  - [ ] 3.5d Test context cancellation
  - [ ] 3.5e Test context timeout

## 4. Config File Updates
- [ ] 4.1 Read current active config (`zerb.lua.active`)
- [ ] 4.2 Parse existing config using `internal/config.Parser`
- [ ] 4.3 Add new ConfigFile entry to the Configs array
- [ ] 4.4 Detect duplicates and skip or warn appropriately
- [ ] 4.5 Generate new Lua config using `internal/config.Generator`
- [ ] 4.6 Create new timestamped config file in `configs/` directory
- [ ] 4.7 Update `.zerb-active` marker and `zerb.lua.active` symlink
- [ ] 4.8 Write tests for config update logic

## 5. Git Integration
- [ ] 5.1 Define Git interface for testability
  - [ ] 5.1a Write interface with Stage(ctx, files...) and Commit(ctx, msg, body) methods
  - [ ] 5.1b Create internal/git package
- [ ] 5.2 Implement Git client with context support
  - [ ] 5.2a Write tests using mock git commands
  - [ ] 5.2b Implement Stage method
  - [ ] 5.2c Implement Commit method
  - [ ] 5.2d Add proper error handling and wrapping
- [ ] 5.3 Generate appropriate commit messages
  - [ ] 5.3a Write tests for commit message generation
  - [ ] 5.3b Single file: "Add ~/.zshrc to tracked configs"
  - [ ] 5.3c Multiple files: "Add N configs to tracked configs" with body
  - [ ] 5.3d Handle long path lists (truncation if needed)
- [ ] 5.4 Integrate staging and committing
  - [ ] 5.4a Stage timestamped config file
  - [ ] 5.4b Stage chezmoi source files
  - [ ] 5.4c Create single commit for all changes
  - [ ] 5.4d Write integration tests

## 6. User Feedback and UX
- [ ] 6.1 Show preview of config changes before applying
- [ ] 6.2 Add confirmation prompt (optional: support --yes flag to skip)
- [ ] 6.3 Display success message with next steps
- [ ] 6.4 Handle errors gracefully with actionable messages
- [ ] 6.5 Implement transaction file for multi-path operations
- [ ] 6.6 Add `--resume` flag to continue interrupted operations
- [ ] 6.7 Add `--abort` flag to cancel incomplete transactions
- [ ] 6.8 Provide rollback instructions on fatal errors
- [ ] 6.9 Write user acceptance tests for happy path

## 7. Service Layer and Interface-Based Design
- [ ] 7.1 Define Clock interface for deterministic timestamps
  - [ ] 7.1a Write interface with Now() method
  - [ ] 7.1b Implement RealClock and TestClock
- [ ] 7.2 Create ConfigAddService with dependency injection
  - [ ] 7.2a Write tests using mock interfaces
  - [ ] 7.2b Define service struct with Chezmoi, Git, Config, Clock interfaces
  - [ ] 7.2c Implement Execute method with full workflow
  - [ ] 7.2d Accept context for cancellation support
- [ ] 7.3 Wire up command with service layer
  - [ ] 7.3a Instantiate real implementations (Client, Git, etc.)
  - [ ] 7.3b Inject into service
  - [ ] 7.3c Call service.Execute from runConfigAdd
  - [ ] 7.3d Handle errors and user messages at command boundary

## 8. Documentation and Examples
- [ ] 8.1 Update README.md with `zerb config add` examples
- [ ] 8.2 Add command documentation
- [ ] 8.3 Update examples/full.lua with config examples (already exists)
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
- [ ] 9.12 Test path validation security (symlink escape, traversal)
- [ ] 9.13 Run all tests with -race flag

## 10. Transaction Management
- [ ] 9.1 Design transaction file JSON schema with versioning
  - [ ] 9.1a Define ConfigAddTxn struct with version, id (UUID), timestamp
  - [ ] 9.1b Define PathTxn struct with state, flags, created_source_files, last_error
  - [ ] 9.1c Write JSON marshaling tests
- [ ] 9.2 Implement lock file mechanism
  - [ ] 9.2a Write tests for lock acquisition with O_CREATE|O_EXCL
  - [ ] 9.2b Implement lock creation in ~/.config/zerb/tmp/config-add.lock
  - [ ] 9.2c Handle lock conflicts (return ErrTransactionExists)
  - [ ] 9.2d Implement lock release on defer
  - [ ] 9.2e Add optional --force-stale-lock for locks >10 minutes old
- [ ] 9.3 Implement atomic transaction file writes
  - [ ] 9.3a Write tests for atomic write-then-rename pattern
  - [ ] 9.3b Implement writeTxnAtomic(dir, name, txn) function
  - [ ] 9.3c Write to temp file (.tmp)
  - [ ] 9.3d Atomic rename to final path
  - [ ] 9.3e Fsync directory for durability
  - [ ] 9.3f Set permissions to 0600
- [ ] 9.4 **CHANGED**: Update transaction location to ~/.config/zerb/tmp/txn-config-add-<uuid>.json
  - [ ] 9.4a Create tmp directory with 0700 permissions if needed
  - [ ] 9.4b Generate UUID for unique transaction ID
  - [ ] 9.4c Write tests for location and permissions
- [ ] 9.5 Implement state tracking for each path
  - [ ] 9.5a Track state transitions: pending → in-progress → completed | failed
  - [ ] 9.5b Persist transaction after each state change (atomic write)
  - [ ] 9.5c Record created_source_files array for each path
  - [ ] 9.5d Record errors in last_error field
- [ ] 9.6 Implement `--resume` logic
  - [ ] 9.6a Write tests for resume behavior
  - [ ] 9.6b Read existing transaction file
  - [ ] 9.6c Skip paths in "completed" state (idempotent)
  - [ ] 9.6d Retry paths in "failed" or "pending" state
  - [ ] 9.6e Complete config update and git commit after all succeed
  - [ ] 9.6f Delete transaction and release lock on success
- [ ] 9.7 Implement `--abort` logic with automatic cleanup
  - [ ] 9.7a Write tests for abort with file cleanup
  - [ ] 9.7b Read created_source_files from transaction
  - [ ] 9.7c Attempt to remove each created file
  - [ ] 9.7d Delete transaction file
  - [ ] 9.7e Release lock
  - [ ] 9.7f Provide manual instructions if cleanup fails
- [ ] 9.8 Ensure single git commit after all paths succeed
  - [ ] 9.8a Only update zerb.lua after all paths completed
  - [ ] 9.8b Only git commit after config update succeeds
  - [ ] 9.8c Include all changes in single commit
- [ ] 9.9 Implement context support in transaction operations
  - [ ] 9.9a Pass context to all blocking operations
  - [ ] 9.9b Check context.Err() between path processing
  - [ ] 9.9c Persist transaction state before returning on cancellation
  - [ ] 9.9d Write tests for Ctrl+C simulation
- [ ] 9.10 Write comprehensive transaction tests
  - [ ] 9.10a Test full transaction lifecycle (create → process → commit → cleanup)
  - [ ] 9.10b Test resume from interrupted state
  - [ ] 9.10c Test abort with cleanup
  - [ ] 9.10d Test concurrent invocation prevention
  - [ ] 9.10e Test partial failures
  - [ ] 9.10f Test context cancellation
  - [ ] 9.10g Test atomic writes with simulated crashes
  - [ ] 9.10h Run with -race flag to detect races
