# Initialization Capability - Git Repository Setup

## ADDED Requirements

### Requirement: Git Repository Initialization

The system SHALL initialize a git repository during `zerb init` in the ZERB directory (`~/.config/zerb/`).

#### Scenario: Successful git initialization
- **WHEN** user runs `zerb init` with git binary available
- **THEN** a git repository is created at `~/.config/zerb/.git/`
- **AND** git user.name is configured using system git config fallback chain
- **AND** git user.email is configured using system git config fallback chain
- **AND** initial success message indicates git repository was created

#### Scenario: Git binary not available
- **WHEN** user runs `zerb init` without git binary on PATH
- **THEN** initialization continues without creating git repository
- **AND** warning message is displayed explaining git is not installed
- **AND** clear instructions are provided for manual git setup
- **AND** ZERB remains functional without git repository

#### Scenario: Git repository already exists
- **WHEN** user runs `zerb init` and `.git` directory already exists in ZERB directory
- **THEN** git initialization is skipped
- **AND** existing git repository is preserved
- **AND** success message indicates existing git repository was detected

### Requirement: Git User Configuration

The system SHALL configure git user information using a three-tier fallback strategy.

#### Scenario: System git config available
- **WHEN** `git config --global user.name` and `git config --global user.email` return values
- **THEN** those values are used for the ZERB repository git config
- **AND** no warning is displayed

#### Scenario: Environment variables available
- **WHEN** system git config is not available
- **AND** `GIT_AUTHOR_NAME` and `GIT_AUTHOR_EMAIL` environment variables are set
- **THEN** environment variable values are used for git config
- **AND** no warning is displayed

#### Scenario: Placeholder values used
- **WHEN** system git config is not available
- **AND** environment variables are not set
- **THEN** placeholder values ("ZERB User", "zerb@localhost") are used
- **AND** warning message is displayed indicating placeholder values
- **AND** instructions are provided to configure git user info

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
- **AND** commit includes `configs/zerb.lua.YYYYMMDDTHHMMSS.sssZ` file
- **AND** commit includes `.gitignore` file
- **AND** commit author is configured git user

#### Scenario: No initial commit when git unavailable
- **WHEN** `zerb init` runs without git binary available
- **THEN** no git commit is created
- **AND** timestamped config file still exists on filesystem
- **AND** no error is raised

### Requirement: Initialization Step Ordering

The system SHALL execute git initialization steps in the correct sequence to ensure consistency.

#### Scenario: Correct step order
- **WHEN** `zerb init` runs with git available
- **THEN** steps execute in order:
  1. Create directory structure
  2. Write .gitignore file
  3. Initialize git repository
  4. Configure git user info
  5. Generate initial timestamped config
  6. Create initial git commit

#### Scenario: Directory structure precedes git init
- **WHEN** git initialization executes
- **THEN** all required directories (`configs/`, `bin/`, `cache/`, etc.) already exist
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
- **THEN** test covers git binary not found scenario
- **AND** test covers invalid existing git repository scenario
- **AND** test covers git command failures
- **AND** test covers placeholder git user values scenario

