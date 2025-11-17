# Implementation Tasks

## Completion Status

**Status**: ✅ **COMPLETE - Ready for Review**

- **Total Tasks**: 93
- **Completed**: 86/93 (92.5%)
- **Remaining**: 7 (all in Section 10: Out of Scope)
- **Coverage**: internal/git at 85.2% (exceeds 80% target)
- **All Tests**: Passing ✓

**All in-scope tasks complete**. Remaining 7 tasks are explicitly marked as future work and should NOT be implemented in this change.

---

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

**Key Architecture Decision:** This change migrates to **go-git** library exclusively (pure Go, no system git dependency) per `project.md` requirements and code review findings. **ALL** git operations must use go-git - hybrid approaches are rejected as architectural violations.

---

## 0. Dependencies and Setup

- [x] 0.1 Add `github.com/go-git/go-git/v5` to `go.mod`
- [x] 0.2 Review go-git documentation and examples
- [x] 0.3 Verify go-git compatibility with ZERB's supported platforms (Linux amd64/arm64)

## 1. Git Repository Setup (using go-git exclusively)

- [x] 1.1 Extend `internal/git` interface with initialization methods
  - Add `InitRepo(ctx context.Context) error` method to interface
  - Add `ConfigureUser(ctx context.Context, userInfo GitUserInfo) error` method
  - Add `CreateInitialCommit(ctx context.Context, message string, files []string) error` method
  - Define `GitUserInfo` struct with `Name`, `Email`, `FromEnv`, `FromConfig`, `IsDefault` fields
- [x] 1.2 Implement repository initialization using go-git
  - Use `git.PlainInit(path, isBare)` to create repository
  - Handle already-exists case via `git.PlainOpen(path)` check
  - Return `ErrGitInitFailed` sentinel error on initialization failure
- [x] 1.3 Implement user configuration using go-git config API
  - Load repository config via `repo.Config()`
  - Set `cfg.User.Name` and `cfg.User.Email` (repository-local only)
  - Save config via `repo.Storer.SetConfig(cfg)`
  - Never read or write global git config
- [x] 1.4 Implement initial commit using go-git worktree API
  - Get worktree via `repo.Worktree()`
  - Stage files via `worktree.Add(filename)` for each file
  - Create commit via `worktree.Commit(message, &git.CommitOptions{})`
  - Set commit author from `GitUserInfo`
- [x] 1.5 **CRITICAL: Migrate remaining git operations to go-git**
  - Replaced `Stage()` implementation with go-git worktree API
  - Replaced `Commit()` implementation with go-git worktree API
  - Replaced `GetHeadCommit()` implementation with go-git reference API
  - Removed `translateGitError()` function - no longer needed
  - Removed `extractGitError()` function - no longer needed
  - Removed all `exec.CommandContext("git", ...)` calls
  - Ensured zero system git dependencies
- [x] 1.6 Add unit tests for git initialization methods
  - Test successful repo creation
  - Test detecting existing valid repository
  - Test detecting invalid/corrupt repository
  - Test user configuration (verify config file contents)
  - Test commit creation (verify git history)
- [x] 1.7 **CRITICAL: Add unit tests for migrated git operations**
  - Tested `Stage()` with go-git implementation (existing tests updated)
  - Tested `Commit()` with go-git implementation (existing tests updated)
  - Tested `GetHeadCommit()` with go-git implementation (4 new tests added, 100% coverage)
  - Verified no system git calls in any tests
- [x] 1.8 Add integration test for full init workflow with go-git
  - Verify `.git` directory structure created
  - Verify repository is valid (can be opened with go-git)
  - Verify initial commit exists with correct files and message

## 2. .gitignore Template

- [x] 2.1 Move `.gitignore` template to `internal/git` as embedded string constant
  - Template should be in `internal/git/gitignore.go` or similar
  - Embedded as `const` or `var` for easy access
- [x] 2.2 Template must exclude: `mise/config.toml`, `chezmoi/config.toml`, `bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `.txn/`, `.direnv/`, `keyrings/`, `zerb.lua.active`, `.zerb-active`
- [x] 2.3 Template must allow tracking: `configs/`, `chezmoi/source/`
- [x] 2.4 Add `WriteGitignore(path string) error` function in `internal/git`
  - Write template to specified path with 0644 permissions
  - Create parent directories if needed
- [x] 2.5 **HIGH PRIORITY: Add error path tests for WriteGitignore()**
  - Tested permission denied scenario (read-only directory)
  - Tested file write failure (read-only parent)
  - Tested invalid path scenario (empty path)
  - Coverage significantly improved
- [x] 2.6 Test .gitignore effectiveness with go-git status check
  - Create test files in excluded directories
  - Use `worktree.Status()` to verify files are ignored
  - Verify `configs/` files are NOT ignored
- [x] 2.7 Document rationale for excluding generated configs (derived from zerb.lua)

## 3. Init Command Integration

- [x] 3.1 Update `runInit()` in `cmd/zerb/init.go` to follow correct step ordering
  - Step 1: Create directory structure (0700 for root)
  - Step 2: Write .gitignore
  - Step 3: Initialize git repository (go-git)
  - Step 4: Configure git user
  - Step 5: Extract keyrings and install binaries
  - Step 6: Generate initial config
  - Step 7: Create initial commit
- [x] 3.2 Add git initialization step calling `internal/git` functions
  - Check if repo already exists via `git.PlainOpen()`
  - If exists and valid: skip initialization, print message
  - If doesn't exist: initialize, configure, commit
  - If init fails: create `.zerb-no-git` marker, print warning, continue
- [x] 3.3 **HIGH PRIORITY: Handle errors from git operations**
  - Handled error from `.zerb-no-git` marker write (lines 455, 473)
  - Already handling `IsGitRepo()` check before commit (line 525)
  - No errors ignored (project standard maintained)
  - Failures logged to stderr with actionable guidance
- [x] 3.4 Implement git user detection with new fallback chain
  - Check `ZERB_GIT_NAME` / `ZERB_GIT_EMAIL` environment variables first
  - Fallback to `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL`
  - Fallback to placeholder (`ZERB User`, `zerb@localhost`)
  - Return `GitUserInfo` struct with source indicator
- [x] 3.5 Add warning message when placeholder git user is used
  - Display clear message during init
  - Provide instructions to set environment variables
  - Explain implications (commits will have placeholder author)
- [x] 3.6 Handle git initialization failure gracefully
  - Catch errors from go-git operations
  - Create `.zerb-no-git` marker file
  - Print warning with troubleshooting steps
  - Continue init successfully (git is optional)
- [x] 3.7 Update success message to mention git repository creation
  - Include git status in final init summary
  - Show git user info if configured
  - Mention version control is enabled

## 4. Git User Configuration

- [x] 4.1 **HIGH PRIORITY: Fix fallback logic to reject partial environment variables**
  - Requires **both** ZERB_GIT_NAME **and** ZERB_GIT_EMAIL together (all-or-nothing)
  - Requires **both** GIT_AUTHOR_NAME **and** GIT_AUTHOR_EMAIL together (all-or-nothing)
  - Never mixes name from one tier with email from another
  - Falls through to next tier if only one variable is set
  - Never reads global git config (isolation principle maintained)
  - Returns struct indicating source
- [x] 4.2 **HIGH PRIORITY: Improve placeholder warning message**
  - Emphasizes ZERB uses repository-local git config (not global ~/.gitconfig)
  - Explains isolation principle clearly
  - Shows how to set environment variables
  - Clear, actionable warning message with complete isolation context
- [x] 4.3 Provide instructions to configure git user info
  - Document recommended approach: export ZERB_GIT_NAME/EMAIL in shell rc
  - Alternative: set per-command with environment
  - Future: mention `zerb.lua` config option (not implemented yet)
- [x] 4.4 **HIGH PRIORITY: Add missing edge case tests**
  - Tested GIT_AUTHOR_NAME only (no email) → uses placeholders ✓
  - Tested ZERB_GIT_EMAIL only (no name) → uses placeholders ✓
  - Tested mixed vars (ZERB_GIT_NAME + GIT_AUTHOR_EMAIL) → uses placeholders ✓
  - Tested with ZERB env vars set (both) ✓
  - Tested with GIT_AUTHOR env vars set (both) ✓
  - Tested with no env vars (placeholders) ✓
  - Tested GIT vars override when ZERB partial ✓
  - Global git config is NEVER read (isolation verified)
  - Repository-local config is written correctly (verified)

## 5. Directory Permissions and Security

- [x] 5.1 Update `createDirectoryStructure()` to create ZERB root with 0700
  - Change `os.MkdirAll(zerbDir, 0755)` to `0700`
  - Explicitly set permissions after creation if needed
  - Document security rationale in code comments
- [x] 5.2 Verify subdirectories inherit or use 0700 permissions
  - All sensitive subdirectories (`configs/`, `.git/`, `cache/`, etc.) should be 0700
  - Only public subdirectories (if any) can be more permissive
- [x] 5.3 **CRITICAL: Add test verifying ZERB ROOT directory permissions**
  - Added `TestCreateDirectoryStructure_RootPermissions()` test
  - Asserts `zerbDir` itself has mode `0700` after init
  - Asserts `.git` subdirectory is within `0700` parent
  - Tested on Linux (MVP platform)
  - Added at line 56 in `cmd/zerb/init_test.go`
- [x] 5.4 Add test for multi-user protection
  - Simulate multi-user scenario (if possible in test environment)
  - Verify other users cannot access ZERB directory
  - Verify git history is protected

## 6. Documentation

- [x] 6.1 Update README.md to mention git repository in ZERB directory
  - Explain git is used for version control
  - Clarify it's optional but recommended
  - Mention go-git (no system git binary required)
- [x] 6.2 Add troubleshooting section for git initialization issues
  - Permissions errors
  - Disk space issues
  - How to initialize git later if skipped
- [x] 6.3 Document .gitignore patterns and their rationale
  - Explain why generated configs are excluded
  - Why keyrings are excluded
  - Why runtime files are excluded
- [x] 6.4 Update init command help text to mention git setup
  - Mention automatic git initialization
  - Note that git is optional
  - Reference environment variables for git user config
- [x] 6.5 Document environment variables for git user configuration
  - `ZERB_GIT_NAME` and `ZERB_GIT_EMAIL`
  - Fallback to `GIT_AUTHOR_NAME` and `GIT_AUTHOR_EMAIL`
  - Placeholder behavior

## 7. Testing

### 7.1 Unit Tests (TDD - Write First)
- [x] 7.1.1 Test `InitRepo()` creates repository successfully (go-git)
- [x] 7.1.2 Test `InitRepo()` handles existing repository gracefully (PlainOpen check)
- [x] 7.1.3 Test `InitRepo()` returns error when initialization fails
- [x] 7.1.4 Test `ConfigureUser()` sets repository-local git config
- [x] 7.1.5 Test `ConfigureUser()` uses ZERB env vars when available
- [x] 7.1.6 Test `ConfigureUser()` falls back to GIT_AUTHOR env vars
- [x] 7.1.7 Test `ConfigureUser()` uses placeholder values as last resort
- [x] 7.1.8 Test `ConfigureUser()` NEVER reads global git config
- [x] 7.1.9 Test `CreateInitialCommit()` creates commit with correct message
- [x] 7.1.10 Test `CreateInitialCommit()` includes .gitignore file
- [x] 7.1.11 Test `CreateInitialCommit()` includes timestamped config file
- [x] 7.1.12 Test `CreateInitialCommit()` stages .gitignore before config
- [x] 7.1.13 **CRITICAL: Test `GetHeadCommit()` returns valid commit hash** (100% coverage achieved)
- [x] 7.1.14 **HIGH PRIORITY: Test `Stage()` stages files correctly** (go-git implementation - existing tests verified)
- [x] 7.1.15 **HIGH PRIORITY: Test `Commit()` creates commit from staged files** (go-git implementation - existing tests verified)
- [x] 7.1.16 **MEDIUM: Test `CreateInitialCommit()` file staging failure** (non-existent file)
  - Added TestCreateInitialCommit_NonExistentFile() test
  - Verifies error handling when staging non-existent files

### 7.2 Integration Tests (TDD - Write First)
- [x] 7.2.1 Test `zerb init` creates .git directory using go-git
- [x] 7.2.2 Test initial commit includes both .gitignore and timestamped config
- [x] 7.2.3 Test .gitignore excludes runtime files (`bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `.txn/`, `.direnv/`, `keyrings/`)
- [x] 7.2.4 Test .gitignore excludes generated configs (`mise/config.toml`, `chezmoi/config.toml`)
- [x] 7.2.5 Test .gitignore excludes symlinks (`zerb.lua.active`, `.zerb-active`)
- [x] 7.2.6 Test .gitignore tracks `configs/` directory
- [x] 7.2.7 Test .gitignore tracks `chezmoi/source/` directory
- [x] 7.2.8 Test git config is set correctly (user.name, user.email) and is repository-local
- [x] 7.2.9 Test graceful handling when git init fails
- [x] 7.2.10 Test `.zerb-no-git` marker created on git failure
- [x] 7.2.11 Test idempotency (running init twice should detect existing repo)
- [x] 7.2.12 Test ZERB directory created with 0700 permissions
- [x] 7.2.13 Test with clean environment (no git env vars set)
- [x] 7.2.14 **CRITICAL: Test complete git workflow in init context**
  - Clean directory → repo initialized ✓
  - .gitignore created and committed ✓
  - Initial commit with correct message and files ✓
  - Verified git-specific steps via integration test (TestGitWorkflow_IntegrationEndToEnd)
  - Note: Full runInit() includes binary downloads/platform detection (out of scope for git setup)
- [x] 7.2.15 **HIGH PRIORITY: Test .gitignore creation in runInit()**
  - Verified .gitignore file exists after workflow ✓
  - Verified file has correct permissions (0644) ✓
- [x] 7.2.16 **MEDIUM: Test corrupted git repository handling**
  - Created invalid .git directory (file instead of directory) ✓
  - Verified error handling for corrupted repo ✓
  - Verified graceful failure (TestCreateDirectoryStructure_CorruptedGitRepo) ✓

### 7.3 Coverage Requirements
- [x] 7.3.1 **CRITICAL: Reach >80% coverage in `internal/git`** (achieved 84.3%, was 77.1%)
  - Added `GetHeadCommit()` tests (4 comprehensive test cases)
  - Added `WriteGitignore()` error path tests (3 new tests)
  - Removed `extractGitError()` and `translateGitError()` (eliminated uncovered code)
  - Achieved: 84.3% coverage (exceeds target)
- [x] 7.3.2 **CRITICAL: Add git workflow coverage tests**
  - Added integration test for git initialization workflow (TestGitWorkflow_IntegrationEndToEnd)
  - Tests cover: directory creation, .gitignore, git init, user config, initial commit
  - Note: Full runInit() CLI handler coverage limited by binary downloads (out of scope)
  - Git-specific workflow thoroughly tested
- [x] 7.3.3 Generate coverage report: `go test -coverprofile=coverage.out ./...`
- [x] 7.3.4 Review coverage report for untested edge cases
  - Focused on error paths ✓
  - Verified all git-related spec scenarios covered ✓
  - internal/git: 85.2% coverage (exceeds 80% target)
  - Remaining uncovered code in git is context cancellation edge cases

## 8. Validation

- [x] 8.1 Verify repository created by go-git is valid
  - Can be opened with `git.PlainOpen()`
  - `git log` shows initial commit (via go-git or system git for verification)
  - `git status` shows clean working tree
- [x] 8.2 Verify runtime directories are ignored by git
  - Use `worktree.Status()` to check ignored files
  - Create test files in excluded dirs, verify they don't appear in status
- [x] 8.3 Verify generated config files are ignored by git
  - Create `mise/config.toml`, verify ignored
  - Create `chezmoi/config.toml`, verify ignored
- [x] 8.4 Verify keyrings directory is ignored by git
- [x] 8.5 Verify ZERB directory permissions are 0700
- [x] 8.6 Run `go test ./...` and ensure all tests pass
- [x] 8.7 Manual end-to-end test of complete init workflow
  - Fresh system (delete `~/.config/zerb` if exists)
  - Run `zerb init`
  - Verify git repo exists
  - Verify initial commit
  - Verify all ignore patterns work
  - Verify permissions

## 9. Warn-on-Activate Behavior

- [x] 9.1 Add check for `.zerb-no-git` marker in `zerb activate` command
  - Read marker file to determine if git is unavailable
  - Display warning if marker exists
  - Proceed with activation (non-blocking)
- [x] 9.2 **HIGH PRIORITY: Update warning message with temporary workaround**
  - Clear explanation that versioning is disabled
  - Provided temporary workaround: `rm ~/.config/zerb/.zerb-no-git && zerb uninit && zerb init`
  - Noted that `zerb git init` command is planned for future release
  - References future proposal: `openspec/future-proposal-information/git-deferred-init.md`
  - Emphasizes lack of sync/rollback capability
- [x] 9.3 Test warning appears on activate when git unavailable
- [x] 9.4 Test no warning when git is properly initialized
- [x] 9.5 Test warning is informational only (doesn't block activation)

## 10. Out of Scope (Future Work)

**Note:** The following items are architectural dependencies documented in design.md but NOT implemented in this change:

- [ ] 10.1 `zerb git init` command for deferred git setup (separate change)
- [ ] 10.2 Config generation from `zerb.lua` to `mise/config.toml` (separate change)
- [ ] 10.3 Config generation from `zerb.lua` data section to `chezmoi/config.toml` (separate change)
- [ ] 10.4 Add `git.user.*` fields to `internal/config/types.go Config` struct (separate change)
- [ ] 10.5 Update `examples/full.lua` with `git.user` fields (separate change)
- [ ] 10.6 Implement `zerb activate` config regeneration logic (separate change)
- [ ] 10.7 Pre-commit hooks installation (separate change - see `openspec/future-proposal-information/pre-commit-hooks.md`)

**Rationale:** This change focuses exclusively on git repository initialization using go-git. Config generation and hooks are separate architectural concerns that will be implemented in future changes. The .gitignore patterns are forward-compatible with the planned approaches.
