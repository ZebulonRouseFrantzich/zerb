# config-management Specification

## Purpose
TBD - created by archiving change add-config-add-command. Update Purpose after archive.
## Requirements
### Requirement: Config Add Command
The system SHALL provide a `zerb config add` command that adds configuration files and directories to ZERB's tracked configs managed by chezmoi.

#### Scenario: Add single config file
- **WHEN** user runs `zerb config add ~/.zshrc`
- **THEN** the file is added to chezmoi's source directory
- **AND** a new ConfigFile entry is appended to the configs array in zerb.lua
- **AND** a new timestamped config file is created in configs/
- **AND** a git commit is created with message "Add ~/.zshrc to tracked configs"

#### Scenario: Add directory recursively
- **WHEN** user runs `zerb config add ~/.config/nvim --recursive`
- **THEN** the directory and all its contents are added to chezmoi's source directory
- **AND** a new ConfigFile entry with recursive=true is added to zerb.lua
- **AND** a timestamped config and git commit are created

#### Scenario: Add with template flag
- **WHEN** user runs `zerb config add ~/.gitconfig --template`
- **THEN** the config is added with template=true in zerb.lua
- **AND** chezmoi is invoked with --template flag

#### Scenario: Add with multiple flags
- **WHEN** user runs `zerb config add ~/.ssh/config --template --secrets --private`
- **THEN** the ConfigFile entry has template=true, secrets=true, private=true
- **AND** chezmoi is invoked with appropriate flags

### Requirement: Config Path Validation
The system SHALL validate all config paths for security before adding them.

#### Scenario: Valid home directory path
- **WHEN** user provides `~/.zshrc`
- **THEN** the path is accepted and tilde is expanded

#### Scenario: Reject path traversal attempt
- **WHEN** user provides `~/../etc/passwd`
- **THEN** the command errors with "path traversal not allowed"
- **AND** no files are modified

#### Scenario: Reject absolute path outside home
- **WHEN** user provides `/etc/nginx/nginx.conf`
- **THEN** the command errors with "absolute paths outside home directory not allowed"

#### Scenario: Reject non-existent path
- **WHEN** user provides `~/.config/nonexistent` which does not exist
- **THEN** the command errors with "Path does not exist: ~/.config/nonexistent"
- **AND** no files are modified
- **AND** exit code is 1
- **AND** no transaction is created

#### Scenario: Reject directory without --recursive flag
- **WHEN** user provides `~/.config/nvim` which is a directory
- **AND** the `--recursive` flag is not provided
- **THEN** the command errors with helpful message:
  ```
  Error: ~/.config/nvim is a directory.
  Use --recursive to track it and its contents.
  
  Example:
    zerb config add ~/.config/nvim --recursive
  ```
- **AND** no files are modified
- **AND** exit code is 1

#### Scenario: Accept absolute path inside home directory
- **WHEN** user provides `/home/user/.zshrc` (absolute path within $HOME)
- **THEN** the path is accepted and validated correctly
- **AND** the file is added to tracked configs

#### Scenario: Detect symlink escape from home directory
- **WHEN** user provides `~/.config/link-to-etc` which is a symlink pointing to `/etc/sensitive`
- **THEN** the command errors with "path traversal not allowed"
- **AND** no files are modified

#### Scenario: Allow symlink within home directory
- **WHEN** user provides `~/.config/link` which is a symlink pointing to `~/.config/actual`
- **THEN** the path is accepted
- **AND** the symlink target is validated to be within home
- **AND** the file is added to tracked configs

#### Scenario: Reject path with traversal after normalization
- **WHEN** user provides `~/documents/../../etc/passwd`
- **THEN** the command errors with "path traversal not allowed"
- **AND** no files are modified

#### Scenario: Allow legitimate paths containing dot-dot
- **WHEN** user provides `~/.config/..something` (literal directory name)
- **THEN** the path is accepted if it exists and is within home
- **AND** not rejected due to containing ".." substring

### Requirement: Path Validation Security
The system SHALL implement robust path validation to prevent directory traversal attacks and unauthorized file access.

#### Scenario: Canonical path verification with symlink resolution
- **WHEN** validating any path
- **THEN** the system MUST resolve symlinks using filepath.EvalSymlinks
- **AND** verify the canonical path is within $HOME
- **AND** use filepath.Rel to ensure no directory traversal

#### Scenario: Fix existing validateConfigPath implementation
- **WHEN** implementing path validation
- **THEN** the system MUST NOT use strings.Contains(cleaned, "..") which is insecure
- **AND** MUST use canonical path checking instead
- **AND** MUST allow absolute paths within $HOME (not just ~/paths)
- **AND** MUST handle symlinks securely

#### Scenario: Path normalization for comparison
- **WHEN** comparing paths for duplicates
- **THEN** paths MUST be normalized to canonical form
- **AND** tilde-prefixed and absolute forms of same path MUST match
- **AND** trailing slashes MUST be ignored
- **AND** comparison MUST be case-sensitive on Linux, case-insensitive on macOS

### Requirement: Duplicate Detection
The system SHALL detect duplicate config entries and handle them gracefully.

#### Scenario: Path already tracked
- **WHEN** user runs `zerb config add ~/.zshrc` and ~/.zshrc is already in configs
- **THEN** a warning is displayed: "Config already tracked: ~/.zshrc"
- **AND** no changes are made to the config file
- **AND** exit code is 0 (success, idempotent)

#### Scenario: Path with different flags already tracked
- **WHEN** user runs `zerb config add ~/.zshrc --template` and ~/.zshrc already exists without template flag
- **THEN** a warning is displayed: "Config already tracked with different flags: ~/.zshrc"
- **AND** user is prompted: "Update flags? [y/N]"
- **AND** if yes, the ConfigFile entry is updated with new flags

### Requirement: Chezmoi Isolation
The system SHALL invoke chezmoi with complete isolation flags to prevent interference with user's existing chezmoi setup.

#### Scenario: Chezmoi invocation with isolation
- **WHEN** the command invokes chezmoi
- **THEN** it MUST use `--source ~/.config/zerb/chezmoi/source`
- **AND** it MUST use `--config ~/.config/zerb/chezmoi/config.toml`

#### Scenario: Chezmoi binary path
- **WHEN** the command needs to invoke chezmoi
- **THEN** it MUST use the ZERB-managed binary at `~/.config/zerb/bin/chezmoi`
- **AND** it MUST NOT use system chezmoi or user's chezmoi installation

### Requirement: User Abstraction
The system SHALL abstract chezmoi implementation details from user-facing messages.

#### Scenario: Success message abstraction
- **WHEN** config add succeeds
- **THEN** success messages MUST NOT mention "chezmoi"
- **AND** messages use "ZERB" or "tracked configs" terminology

#### Scenario: Error message abstraction
- **WHEN** chezmoi returns an error
- **THEN** the error MUST be translated to user-friendly terms
- **AND** MUST NOT expose chezmoi-specific error messages directly

### Requirement: Config Preview and Confirmation
The system SHALL show a preview of config changes and prompt for confirmation before applying.

#### Scenario: Show config diff before applying
- **WHEN** user runs `zerb config add ~/.zshrc`
- **THEN** a preview is displayed showing the new ConfigFile entry
- **AND** user is prompted: "Apply? [Y/n]"
- **AND** if user confirms, changes are applied

#### Scenario: Skip confirmation with --yes flag
- **WHEN** user runs `zerb config add ~/.zshrc --yes`
- **THEN** no confirmation prompt is shown
- **AND** changes are applied immediately

#### Scenario: User declines confirmation
- **WHEN** user is prompted "Apply? [Y/n]" and enters "n"
- **THEN** no changes are made
- **AND** message displayed: "Config add cancelled"
- **AND** exit code is 0

### Requirement: Multiple Files Support
The system SHALL support adding multiple config files in a single command.

#### Scenario: Add multiple files
- **WHEN** user runs `zerb config add ~/.zshrc ~/.gitconfig ~/.tmux.conf`
- **THEN** all three files are added to chezmoi
- **AND** three ConfigFile entries are added to zerb.lua
- **AND** a single git commit is created with message "Add 3 configs to tracked configs"

#### Scenario: Multiple files with mixed results
- **WHEN** user runs `zerb config add ~/.zshrc ~/.existing` where ~/.existing is already tracked
- **THEN** ~/.zshrc is added successfully
- **AND** ~/.existing is skipped with warning
- **AND** summary shows: "Added 1 config, skipped 1 duplicate"

### Requirement: Git Integration
The system SHALL create appropriate git commits for config additions.

#### Scenario: Git commit message for single file
- **WHEN** adding `~/.zshrc`
- **THEN** commit message is "Add ~/.zshrc to tracked configs"

#### Scenario: Git commit message for directory
- **WHEN** adding `~/.config/nvim/` with --recursive
- **THEN** commit message is "Add ~/.config/nvim/ to tracked configs"

#### Scenario: Git commit message for multiple files
- **WHEN** adding 3 files in one command
- **THEN** commit message is "Add 3 configs to tracked configs"
- **AND** commit body lists all added paths

#### Scenario: Git commit includes timestamped config
- **WHEN** config add succeeds
- **THEN** the git commit MUST include the new timestamped config file in configs/
- **AND** the git commit MUST include files added to chezmoi/source/

### Requirement: Command Help and Documentation
The system SHALL provide clear help text for the config add command.

#### Scenario: Show help with --help
- **WHEN** user runs `zerb config add --help`
- **THEN** usage information is displayed
- **AND** all flags are documented (--recursive, --template, --secrets, --private, --yes, --resume, --abort)
- **AND** examples are shown

#### Scenario: Show error for missing arguments
- **WHEN** user runs `zerb config add` with no arguments
- **THEN** error is displayed: "Usage: zerb config add <path> [paths...] [flags]"
- **AND** exit code is 1

### Requirement: Transaction Safety
The system SHALL provide transaction-based safety for multi-path operations to ensure atomic commits, recovery from interruptions, and concurrency control.

#### Scenario: Create transaction with lock and UUID
- **WHEN** user runs `zerb config add ~/.zshrc ~/.gitconfig ~/.tmux.conf`
- **THEN** a lock file is created at `~/.config/zerb/tmp/config-add.lock` using O_CREATE|O_EXCL
- **AND** a transaction file is created at `~/.config/zerb/tmp/txn-config-add-<uuid>.json` with 0600 permissions
- **AND** the transaction file has version:1, unique UUID, and timestamp in RFC3339 format
- **AND** the transaction file contains all three paths with state "pending"
- **AND** each path is processed sequentially
- **AND** the transaction file is atomically updated after each successful chezmoi add

#### Scenario: Resume interrupted operation
- **WHEN** a previous `zerb config add` was interrupted (leaving transaction file)
- **AND** user runs `zerb config add --resume`
- **THEN** the command reads the existing transaction file
- **AND** already-completed paths are skipped (idempotent)
- **AND** pending paths are processed
- **AND** after all paths succeed, zerb.lua is updated and git commit is created
- **AND** transaction file is deleted on complete success

#### Scenario: Abort transaction with automatic cleanup
- **WHEN** a previous `zerb config add` was interrupted
- **AND** user runs `zerb config add --abort`
- **THEN** the system reads created_source_files from transaction
- **AND** attempts to automatically remove each created file
- **AND** transaction file is deleted
- **AND** lock file is released
- **AND** if automatic cleanup succeeds, message: "Transaction aborted successfully"
- **AND** if cleanup fails, manual instructions are provided with specific files to remove

#### Scenario: Transaction file cleanup on success
- **WHEN** all paths in a multi-file add succeed
- **AND** zerb.lua is updated
- **AND** git commit is created
- **THEN** the transaction file MUST be deleted automatically
- **AND** the lock file MUST be released
- **AND** no transaction remnants remain

#### Scenario: Concurrent invocation prevention
- **WHEN** a `zerb config add` operation is in progress (lock file exists)
- **AND** user attempts to run another `zerb config add` command
- **THEN** the second command MUST fail immediately
- **AND** error message displayed: "Another configuration operation is in progress"
- **AND** exit code is 1
- **AND** optional: suggest `--force-stale-lock` if lock is older than 10 minutes

#### Scenario: Atomic transaction file writes
- **WHEN** updating transaction state after each operation
- **THEN** changes MUST be written to a temporary file first
- **AND** atomically renamed to the final transaction file path
- **AND** directory MUST be fsynced for durability
- **AND** file permissions MUST be 0600

#### Scenario: Transaction artifact tracking
- **WHEN** chezmoi adds a file
- **THEN** the created source file path(s) MUST be recorded in created_source_files array
- **AND** this enables automatic cleanup on abort
- **AND** each path entry tracks all artifacts it created

#### Scenario: Context cancellation during operation
- **WHEN** user presses Ctrl+C during a config add operation
- **THEN** the current operation MUST be cancelled via context
- **AND** transaction state MUST be persisted before exit
- **AND** already-completed paths remain in "completed" state
- **AND** in-progress path reverts to "pending" state
- **AND** user can resume with `--resume` flag

#### Scenario: All operations in single git commit
- **WHEN** user runs `zerb config add ~/.zshrc ~/.gitconfig ~/.tmux.conf`
- **THEN** all three files are added to chezmoi individually
- **AND** the transaction tracks completion of each chezmoi add
- **AND** after all three succeed, a SINGLE timestamped config is created
- **AND** a SINGLE git commit includes all three files
- **AND** the commit message is "Add 3 configs to tracked configs"

#### Scenario: Transaction state tracking
- **WHEN** adding multiple files with transaction
- **THEN** each path has a state field in the transaction file
- **AND** state transitions are: pending → in-progress → completed
- **AND** the transaction file persists after each state change
- **AND** if interrupted at any point, resume can continue from last saved state

#### Scenario: Error handling in transaction
- **WHEN** adding multiple files and one fails (e.g., permission denied)
- **THEN** the error is recorded in the transaction file
- **AND** the command continues processing remaining files
- **AND** at the end, a summary shows successes and failures
- **AND** the transaction file remains for `--resume` to retry failed paths
- **AND** no git commit is created until ALL paths succeed

