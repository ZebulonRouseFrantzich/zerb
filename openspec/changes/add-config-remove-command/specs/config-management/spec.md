## ADDED Requirements

### Requirement: Config Remove Command
The system SHALL provide a `zerb config remove` command that removes configuration files and directories from ZERB's tracked configs.

**Naming rationale**: "remove" is preferred over "delete" because the default behavior only untracks the config—it does not delete the actual file from disk.

#### Scenario: Remove single config file
- **WHEN** user runs `zerb config remove ~/.zshrc`
- **AND** user confirms the removal
- **THEN** the file is removed from chezmoi's source directory
- **AND** the ConfigFile entry is removed from zerb.lua
- **AND** a new timestamped config file is created in configs/
- **AND** a git commit is created with message "Remove ~/.zshrc from tracked configs"
- **AND** the source file on disk is NOT deleted (kept by default)

#### Scenario: Remove multiple config files
- **WHEN** user runs `zerb config remove ~/.zshrc ~/.gitconfig ~/.tmux.conf`
- **AND** user confirms the removal
- **THEN** all three files are removed from chezmoi's source directory
- **AND** all three ConfigFile entries are removed from zerb.lua
- **AND** a single git commit is created with message "Remove 3 configs from tracked configs"

#### Scenario: Remove with --purge flag (CR-2 order)
- **WHEN** user runs `zerb config remove ~/.zshrc --purge`
- **AND** user confirms the removal
- **THEN** the source file is deleted from disk FIRST
- **AND** then the file is removed from chezmoi's source directory
- **AND** the ConfigFile entry is removed from zerb.lua

#### Scenario: Remove with --yes flag (skip confirmation)
- **WHEN** user runs `zerb config remove ~/.zshrc --yes`
- **THEN** no confirmation prompt is shown
- **AND** the config is removed immediately

#### Scenario: Remove with --dry-run flag
- **WHEN** user runs `zerb config remove ~/.zshrc --dry-run`
- **THEN** a preview of what would be removed is shown
- **AND** no confirmation prompt is shown
- **AND** no files are modified
- **AND** exit code is 0

### Requirement: Config Remove Path Validation
The system SHALL validate that paths exist in the current config before attempting removal.

#### Scenario: Path not tracked
- **WHEN** user runs `zerb config remove ~/.nonexistent`
- **AND** the path is not in zerb.lua
- **THEN** the command errors with "Config not tracked: ~/.nonexistent"
- **AND** no files are modified
- **AND** exit code is 1

#### Scenario: Path normalization for lookup
- **WHEN** user provides `~/.zshrc`
- **AND** the config is stored as `/home/user/.zshrc` in zerb.lua
- **THEN** the paths are matched correctly after normalization
- **AND** the config is removed successfully

#### Scenario: ZERB not initialized
- **WHEN** user runs `zerb config remove ~/.zshrc`
- **AND** ZERB is not initialized (no .zerb-active marker)
- **THEN** error displayed: "ZERB not initialized. Run 'zerb init' first"
- **AND** exit code is 1

#### Scenario: Duplicate paths in arguments (HR-4)
- **WHEN** user runs `zerb config remove ~/.zshrc /home/user/.zshrc`
- **AND** both paths resolve to the same normalized path
- **THEN** the paths are deduplicated before processing
- **AND** the config is removed only once
- **AND** commit message reflects single removal

#### Scenario: Empty config after removal
- **WHEN** user runs `zerb config remove ~/.zshrc`
- **AND** this is the only config tracked
- **THEN** the config is removed successfully
- **AND** zerb.lua has an empty Configs array
- **AND** git commit is created normally

#### Scenario: Symlink path resolution
- **WHEN** user runs `zerb config remove ~/dotfiles/.zshrc`
- **AND** `~/dotfiles` is a symlink to `/home/user/.config`
- **AND** the config is stored as `/home/user/.config/.zshrc` in zerb.lua
- **THEN** the symlink is resolved before path matching
- **AND** the config is removed successfully

#### Scenario: Path outside home directory with --purge (HR-5)
- **WHEN** user runs `zerb config remove /etc/some-config --purge`
- **AND** the path is tracked in zerb.lua
- **AND** the path is outside $HOME
- **THEN** the command errors with "Cannot delete file outside home directory"
- **AND** no files are modified
- **AND** exit code is 1

### Requirement: Config Remove Confirmation
The system SHALL require confirmation before removing configs by default.

#### Scenario: Show confirmation prompt with status
- **WHEN** user runs `zerb config remove ~/.zshrc`
- **THEN** a confirmation prompt is displayed showing:
  ```
  The following configs will be removed from ZERB tracking:
    - ~/.zshrc (synced)

  Source files on disk will NOT be deleted (use --purge to also delete).

  Proceed? [y/N]: 
  ```
- **AND** the current status (synced/missing/partial) is shown for each path

#### Scenario: User confirms removal
- **WHEN** user is prompted and enters "y" or "yes"
- **THEN** the removal proceeds

#### Scenario: User rejects removal
- **WHEN** user is prompted and enters "n" or "no" or empty
- **THEN** no changes are made
- **AND** message displayed: "Config remove cancelled"
- **AND** exit code is 0

#### Scenario: Confirmation prompt with --purge
- **WHEN** user runs `zerb config remove ~/.zshrc --purge`
- **THEN** the confirmation prompt shows:
  ```
  The following configs will be removed from ZERB tracking:
    - ~/.zshrc (synced)

  WARNING: Source files will be DELETED from disk.

  Proceed? [y/N]: 
  ```

### Requirement: Chezmoi Integration for Remove
The system SHALL invoke chezmoi with complete isolation flags to remove files from managed state.

#### Scenario: Chezmoi forget invocation
- **WHEN** the command removes a config from tracking
- **THEN** it MUST invoke `chezmoi forget <path>` with isolation flags
- **AND** it MUST use `--source ~/.config/zerb/chezmoi/source`
- **AND** it MUST use `--config ~/.config/zerb/chezmoi/config.toml`

#### Scenario: Chezmoi binary path
- **WHEN** the command needs to invoke chezmoi
- **THEN** it MUST use the ZERB-managed binary at `~/.config/zerb/bin/chezmoi`
- **AND** it MUST NOT use system chezmoi or user's chezmoi installation

#### Scenario: Chezmoi source file not found (HR-3)
- **WHEN** removing a config that is declared but source file was manually deleted
- **THEN** the system logs a warning: "chezmoi source not found, continuing with config removal"
- **AND** continues with removing the config entry from zerb.lua
- **AND** creates the git commit
- **AND** this is NOT treated as an error

### Requirement: User Abstraction for Remove
The system SHALL abstract chezmoi implementation details from user-facing messages.

#### Scenario: Success message abstraction
- **WHEN** config remove succeeds
- **THEN** success messages MUST NOT mention "chezmoi"
- **AND** messages use "ZERB" or "tracked configs" terminology

#### Scenario: Error message abstraction
- **WHEN** chezmoi returns an error
- **THEN** the error MUST be translated to user-friendly terms
- **AND** MUST NOT expose chezmoi-specific error messages directly

### Requirement: Transaction Safety for Remove
The system SHALL provide transaction-based safety for remove operations.

#### Scenario: Create transaction for remove
- **WHEN** user runs `zerb config remove ~/.zshrc ~/.gitconfig`
- **THEN** a lock file is created at `~/.config/zerb/.txn/config.lock`
- **AND** a transaction file is created at `~/.config/zerb/.txn/txn-config-remove-<uuid>.json`
- **AND** each path is processed sequentially
- **AND** the transaction file is updated after each successful removal

#### Scenario: Resume interrupted remove operation
- **WHEN** a previous `zerb config remove` was interrupted
- **AND** user runs `zerb config remove --resume`
- **THEN** the command reads the existing transaction file
- **AND** already-completed paths are skipped
- **AND** pending paths are processed
- **AND** after all paths succeed, zerb.lua is updated and git commit is created

#### Scenario: Abort remove transaction
- **WHEN** a previous `zerb config remove` was interrupted
- **AND** user runs `zerb config remove --abort`
- **THEN** the transaction file is deleted
- **AND** the lock file is released
- **AND** message displayed: "Transaction aborted"
- **AND** already-removed chezmoi source files are NOT restored

#### Scenario: Concurrent remove prevention
- **WHEN** a `zerb config remove` operation is in progress
- **AND** user attempts to run another config operation
- **THEN** the second command MUST fail immediately
- **AND** error message displayed: "Another configuration operation is in progress"

### Requirement: Git Integration for Remove
The system SHALL create appropriate git commits for config removals.

#### Scenario: Git commit message for single file
- **WHEN** removing `~/.zshrc`
- **THEN** commit message is "Remove ~/.zshrc from tracked configs"

#### Scenario: Git commit message for multiple files
- **WHEN** removing 3 files in one command
- **THEN** commit message is "Remove 3 configs from tracked configs"
- **AND** commit body lists all removed paths

#### Scenario: Git commit includes all changes
- **WHEN** config remove succeeds
- **THEN** the git commit MUST include the new timestamped config file in configs/
- **AND** the git commit MUST include the removed files from chezmoi/source/

### Requirement: Command Help and Documentation for Remove
The system SHALL provide clear help text for the config remove command.

#### Scenario: Show help with --help
- **WHEN** user runs `zerb config remove --help`
- **THEN** usage information is displayed
- **AND** all flags are documented (--yes, --dry-run, --purge, --keep-file, --resume, --abort)
- **AND** examples are shown

#### Scenario: Show error for missing arguments
- **WHEN** user runs `zerb config remove` with no arguments
- **THEN** error is displayed: "no paths specified"
- **AND** usage hint is shown
- **AND** exit code is 1
