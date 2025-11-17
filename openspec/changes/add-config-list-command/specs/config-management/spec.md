# Config List Capability - Specification Delta

**MVP Scope**: This change implements table output format only. JSON output (`--json`), plain output (`--plain`), historical listing (`--all`), and drift detection (file hash comparison) are deferred to future iterations.

## ADDED Requirements

### Requirement: Config List Command
The system SHALL provide a `zerb config list` command that displays tracked configuration files with their status and flags.

#### Scenario: List active config with default table format
- **WHEN** user runs `zerb config list`
- **THEN** the system displays configs from the active timestamped baseline
- **AND** output shows a table with columns: STATUS, PATH, FLAGS
- **AND** rows are sorted alphabetically by path
- **AND** a summary line shows total counts by status

#### Scenario: List with no configs tracked
- **WHEN** user runs `zerb config list` and no configs are in zerb.lua
- **THEN** message displayed: "No configs tracked yet"
- **AND** suggestion shown: "Add configs with: zerb config add <path>"
- **AND** exit code is 0 (success, not an error)

#### Scenario: List when ZERB not initialized
- **WHEN** user runs `zerb config list` in directory without ZERB
- **THEN** error displayed: "ZERB not initialized. Run 'zerb init' first"
- **AND** exit code is 1

### Requirement: Status Detection
The system SHALL detect and display the status of each tracked configuration file.

#### Scenario: Synced status (config fully managed)
- **WHEN** a config is declared in zerb.lua
- **AND** the source file exists on disk
- **AND** the file is managed by ZERB
- **THEN** status is "✓" (Synced)

#### Scenario: Missing status (file not found)
- **WHEN** a config is declared in zerb.lua
- **AND** the source file does NOT exist on disk
- **THEN** status is "✗" (Missing)

#### Scenario: Partial status (tracking incomplete)
- **WHEN** a config is declared in zerb.lua
- **AND** the source file exists on disk
- **AND** the file is NOT managed by ZERB
- **THEN** status is "?" (Partial)
- **AND** this indicates an incomplete `zerb config add` operation

**Note**: Drift detection (file hash comparison) is deferred to a future iteration. MVP only detects Synced, Missing, and Partial statuses.

### Requirement: Verbose Output
The system SHALL provide detailed information with the `--verbose` flag.

#### Scenario: Verbose output includes size and timestamps
- **WHEN** user runs `zerb config list --verbose`
- **THEN** output includes SIZE column with file sizes
- **AND** output includes LAST MODIFIED column with relative times
- **AND** a notes section explains status indicators
- **AND** missing/partial configs show explanatory notes

### Requirement: Flag Display
The system SHALL display config flags in a human-readable format.

#### Scenario: Show only enabled flags
- **WHEN** displaying a config with flags
- **THEN** only flags set to true are shown
- **AND** false flags are omitted from display
- **AND** multiple flags are joined with ", " separator

#### Scenario: Config with no flags
- **WHEN** displaying a config with all flags false
- **THEN** the FLAGS column is empty (no "false" values shown)

### Requirement: User Abstraction
The system SHALL abstract internal implementation details from user-facing output.

#### Scenario: Never mention "chezmoi" in output
- **WHEN** any `zerb config list` command is run
- **THEN** no output contains the word "chezmoi"
- **AND** managed state is described as "managed by ZERB"
- **AND** source tracking is described using ZERB-centric terminology

#### Scenario: User-friendly status descriptions
- **WHEN** verbose mode shows status explanations
- **THEN** descriptions use terms like "managed by ZERB", "tracked", "declared"
- **AND** descriptions do NOT reference internal tools or directories
- **AND** error messages are actionable without implementation knowledge

### Requirement: Alphabetical Sorting
The system SHALL sort config paths alphabetically for consistent output.

#### Scenario: Sort paths case-sensitively
- **WHEN** displaying config list
- **THEN** paths are sorted in lexicographic order
- **AND** uppercase letters sort before lowercase (standard ASCII ordering)
- **AND** sorting is consistent across all output formats

#### Scenario: Sort paths with tilde prefix
- **WHEN** displaying configs with ~/path notation
- **THEN** tilde (~) is treated as a character in sorting
- **AND** all paths starting with ~ sort together

### Requirement: Error Handling
The system SHALL handle errors gracefully with clear messages.

#### Scenario: Corrupted active config
- **WHEN** the active config file has invalid Lua syntax
- **THEN** error displayed: "Failed to parse active config: <filename>"
- **AND** Lua error details shown (line number, error message)
- **AND** exit code is 1

#### Scenario: Permission denied reading config
- **WHEN** the active config file cannot be read due to permissions
- **THEN** error displayed: "Permission denied reading config file"
- **AND** suggestion shown: "Check file permissions on <path>"
- **AND** exit code is 1

#### Scenario: Missing configs directory
- **WHEN** `~/.config/zerb/configs/` directory does not exist
- **THEN** treated as ZERB not initialized
- **AND** error displayed: "ZERB not initialized. Run 'zerb init' first"
- **AND** exit code is 1

### Requirement: Interface-Based Design
The system SHALL use interfaces for dependencies to enable testing and maintainability.

#### Scenario: Status detection via interface
- **WHEN** implementing status detection
- **THEN** a `StatusDetector` interface SHALL be defined in `internal/config`
- **AND** service layer SHALL depend on the interface, not concrete implementation
- **AND** interface SHALL accept context for cancellation

#### Scenario: Chezmoi query via interface
- **WHEN** checking if files are managed by ZERB
- **THEN** `Chezmoi` interface SHALL be extended with `HasFile(ctx, path) (bool, error)`
- **AND** service layer SHALL use the interface
- **AND** implementation SHALL reuse `config.NormalizeConfigPath` for path handling
- **AND** errors SHALL be wrapped to hide internal paths (chezmoi source directory)

### Requirement: Context Support
The system SHALL support cancellation and timeouts via context.

#### Scenario: User cancels operation with Ctrl+C
- **WHEN** user presses Ctrl+C during `zerb config list --all`
- **THEN** operation is cancelled gracefully
- **AND** no partial output is shown
- **AND** exit code is 130 (standard for SIGINT)

#### Scenario: Operation timeout
- **WHEN** operation takes longer than timeout (if specified)
- **THEN** operation is cancelled
- **AND** error displayed: "Operation timed out"
- **AND** exit code is 1
