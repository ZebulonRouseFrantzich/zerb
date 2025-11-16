# Initialization Capability - Git Repository Setup

## ADDED Requirements

### Requirement: Git Repository Initialization

The system SHALL initialize a git repository during `zerb init` in the ZERB directory (`~/.config/zerb/`) using the go-git library.

#### Scenario: Successful git initialization
- **WHEN** user runs `zerb init`
- **THEN** a git repository is created at `~/.config/zerb/.git/` using go-git
- **AND** git user.name is configured using environment variable fallback chain
- **AND** git user.email is configured using environment variable fallback chain
- **AND** configuration is written to repository-local git config (not global)
- **AND** initial success message indicates git repository was created

#### Scenario: Git initialization fails
- **WHEN** user runs `zerb init` and go-git initialization fails (permissions, disk space, etc.)
- **THEN** initialization continues without creating git repository
- **AND** warning message is displayed explaining git initialization failed
- **AND** marker file `.zerb-no-git` is created to track git-unavailable state
- **AND** clear instructions are provided for manual git setup later
- **AND** ZERB remains functional without git repository

#### Scenario: Git repository already exists
- **WHEN** user runs `zerb init` and `.git` directory already exists in ZERB directory
- **THEN** git initialization is skipped (detected via go-git PlainOpen)
- **AND** existing git repository is preserved
- **AND** success message indicates existing git repository was detected

### Requirement: Git User Configuration

The system SHALL configure git user information using a three-tier fallback strategy without accessing global git configuration.

#### Scenario: Environment variables available (ZERB-specific)
- **WHEN** `ZERB_GIT_NAME` and `ZERB_GIT_EMAIL` environment variables are set
- **THEN** those values are used for the repository-local git config
- **AND** no warning is displayed
- **AND** global git config is NOT read or modified

#### Scenario: Environment variables available (git standard)
- **WHEN** `ZERB_GIT_NAME` / `ZERB_GIT_EMAIL` are not set
- **AND** `GIT_AUTHOR_NAME` and `GIT_AUTHOR_EMAIL` environment variables are set
- **THEN** those values are used for repository-local git config
- **AND** no warning is displayed
- **AND** global git config is NOT read or modified

#### Scenario: ZERB config available (future)
- **WHEN** environment variables are not set
- **AND** `zerb.lua` contains `git.user.name` and `git.user.email` fields
- **THEN** config values are used for repository-local git config
- **AND** no warning is displayed
- **Note**: This scenario is for future implementation; not in scope for this change

#### Scenario: Placeholder values used
- **WHEN** no environment variables are set
- **AND** no ZERB config available
- **THEN** placeholder values ("ZERB User", "zerb@localhost") are used
- **AND** warning message is displayed indicating placeholder values
- **AND** instructions are provided to configure git user info via environment variables
- **AND** global git config is NOT read or modified

### Requirement: .gitignore Configuration

The system SHALL create a `.gitignore` file in the ZERB directory that excludes runtime files and includes versioned files.

#### Scenario: .gitignore creation
- **WHEN** `zerb init` runs
- **THEN** a `.gitignore` file is created at `~/.config/zerb/.gitignore`
- **AND** file excludes: `mise/config.toml`, `chezmoi/config.toml`, `bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `.txn/`, `.direnv/`, `keyrings/`, `zerb.lua.active`, `.zerb-active`
- **AND** file does NOT exclude: `configs/`, `chezmoi/source/`

#### Scenario: Runtime files ignored by git
- **WHEN** files are created in `bin/`, `cache/`, `tmp/`, `logs/`, `mise/`, `keyrings/`, or `.txn/` directories
- **OR** generated config files `mise/config.toml` or `chezmoi/config.toml` are created
- **THEN** `git status` does not show those files as untracked
- **AND** `git add .` does not stage those files

#### Scenario: Config files tracked by git
- **WHEN** files are created in `configs/` directory
- **THEN** `git status` shows those files as untracked
- **AND** `git add .` stages those files

### Requirement: Initial Commit

The system SHALL create an initial git commit containing the timestamped configuration file and .gitignore.

#### Scenario: Initial commit creation
- **WHEN** `zerb init` completes successfully
- **AND** git repository was initialized
- **THEN** an initial commit exists in git history
- **AND** commit message is "Initialize ZERB environment"
- **AND** commit includes both `.gitignore` file and `configs/zerb.lua.YYYYMMDDTHHMMSS.sssZ` file
- **AND** commit author is configured git user
- **AND** `.gitignore` is staged first (before config) to ensure ignore patterns are active

#### Scenario: No initial commit when git unavailable
- **WHEN** `zerb init` runs and git initialization fails
- **THEN** no git commit is created
- **AND** timestamped config file still exists on filesystem
- **AND** `.zerb-no-git` marker file exists
- **AND** no error is raised

### Requirement: Initialization Step Ordering

The system SHALL execute git initialization steps in the correct sequence to ensure consistency.

#### Scenario: Correct step order
- **WHEN** `zerb init` runs with git available
- **THEN** steps execute in order:
  1. Create directory structure (with 0700 permissions for ZERB root)
  2. Write .gitignore file
  3. Initialize git repository (using go-git)
  4. Configure git user info (repository-local)
  5. Extract keyrings and install binaries
  6. Generate initial timestamped config
  7. Create initial git commit (add .gitignore and config)

#### Scenario: Directory structure precedes git init
- **WHEN** git initialization executes
- **THEN** all required directories (`configs/`, `bin/`, `cache/`, etc.) already exist
- **AND** ZERB root directory has 0700 permissions
- **AND** .gitignore file already exists

### Requirement: Error Handling

The system SHALL handle git initialization errors gracefully and provide clear user guidance.

#### Scenario: Git command fails
- **WHEN** a git command fails during initialization (e.g., permissions issue)
- **THEN** clear error message is displayed with git command output
- **AND** initialization process stops
- **AND** partially created files are left in place for debugging

#### Scenario: Invalid existing git repository
- **WHEN** `.git` directory exists but is not a valid git repository
- **THEN** warning is displayed about corrupted git repository
- **AND** git initialization is skipped
- **AND** user is advised to manually fix or remove .git directory

### Requirement: Idempotency

The system SHALL safely handle multiple init invocations without duplicating git setup.

#### Scenario: Repeated init with git already initialized
- **WHEN** user runs `zerb init` after git is already initialized
- **THEN** error message indicates ZERB is already initialized
- **AND** git repository is not re-initialized
- **AND** no new commits are created
- **AND** existing git history is preserved

### Requirement: Generated Config Exclusion

The system SHALL exclude generated configuration files from git tracking, as they are derived from `zerb.lua`.

#### Scenario: Generated configs not tracked
- **WHEN** `mise/config.toml` or `chezmoi/config.toml` exist in the ZERB directory
- **THEN** `git status` does not show them as untracked
- **AND** `.gitignore` includes patterns for `mise/config.toml` and `chezmoi/config.toml`
- **AND** only source `configs/zerb.lua.*` files are tracked

#### Scenario: Config regeneration after git pull
- **WHEN** user pulls changes to `configs/` from remote
- **AND** runs `zerb activate` to apply a config
- **THEN** `mise/config.toml` and `chezmoi/config.toml` are regenerated from the active `zerb.lua`
- **AND** no git changes are shown for generated configs
- **AND** derived state matches source of truth

**Note:** Config generation implementation is out of scope for this change. This requirement documents the architectural relationship between git tracking and the config generation system.

### Requirement: Git Unavailable Warning on Activate

The system SHALL warn users about missing git versioning on `zerb activate` when git was not initialized.

#### Scenario: Warning displayed on activate when git unavailable
- **WHEN** `.zerb-no-git` marker file exists
- **AND** user runs `zerb activate`
- **THEN** warning message is displayed explaining git is not initialized
- **AND** message includes instructions to run `zerb git init` (future command)
- **AND** warning emphasizes lack of version history and sync capability
- **AND** activation proceeds normally (non-blocking warning)

#### Scenario: No warning when git is initialized
- **WHEN** `.git` directory exists and is valid
- **AND** user runs `zerb activate`
- **THEN** no git-related warning is displayed
- **AND** activation proceeds normally

#### Scenario: Warning is persistent but not intrusive
- **WHEN** `.zerb-no-git` marker exists
- **THEN** warning appears on every `zerb activate` invocation
- **BUT** does NOT appear on other commands (`zerb drift`, `zerb config list`, etc.)
- **AND** warning can be dismissed by initializing git or is accepted as working without versioning

### Requirement: ZERB Directory Security

The system SHALL create the ZERB root directory with restrictive permissions to protect sensitive data and git history.

#### Scenario: ZERB directory permissions
- **WHEN** `zerb init` creates `~/.config/zerb` directory
- **THEN** directory is created with 0700 permissions (user-only access)
- **AND** no other users on the system can read, write, or execute
- **AND** subdirectories inherit restrictive permissions

#### Scenario: Git history protection
- **WHEN** git repository is initialized
- **THEN** `.git` subdirectory and all contents are only accessible to the owning user
- **AND** `git ls-tree` and object database are protected by directory permissions
- **AND** even on multi-user systems with less restrictive `~/.config`, ZERB data remains private

#### Scenario: Config file permissions combined with directory permissions
- **WHEN** config files are created with 0600 permissions
- **AND** ZERB root directory has 0700 permissions
- **THEN** config files are doubly protected (file + directory permissions)
- **AND** no information leakage through directory enumeration or git history access

### Requirement: Test Coverage

The system SHALL have comprehensive test coverage for all git initialization functionality.

#### Scenario: Unit test coverage
- **WHEN** running `go test -cover ./internal/git`
- **THEN** coverage is at least 80%
- **AND** all git initialization methods have unit tests
- **AND** all error paths are tested

#### Scenario: Integration test coverage
- **WHEN** running integration tests for `zerb init`
- **THEN** git repository initialization is tested end-to-end
- **AND** tests verify `.git` directory creation
- **AND** tests verify initial commit contents
- **AND** tests verify `.gitignore` effectiveness
- **AND** tests verify git user configuration

#### Scenario: Test-driven development
- **WHEN** implementing git initialization features
- **THEN** tests are written before implementation (RED phase)
- **AND** minimal code is written to make tests pass (GREEN phase)
- **AND** code is refactored while maintaining passing tests (REFACTOR phase)

#### Scenario: Error condition testing
- **WHEN** testing error handling
- **THEN** test covers git initialization failure scenario (using go-git)
- **AND** test covers invalid existing git repository scenario
- **AND** test covers git commit failures
- **AND** test covers placeholder git user values scenario
- **AND** test covers directory permission scenarios (0700 enforcement)

