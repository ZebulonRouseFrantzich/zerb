# Drift Detection Specification

## ADDED Requirements

### Requirement: Drift Detection Command

The system SHALL provide a `zerb drift` command that performs three-way comparison between baseline configuration, ZERB-managed tools, and active environment.

#### Scenario: Run drift detection with no drifts

- **WHEN** user runs `zerb drift` and all tools match across baseline, managed, and active
- **THEN** system displays "No drifts detected ✓" message
- **AND** exits with status code 0

#### Scenario: Run drift detection with drifts found

- **WHEN** user runs `zerb drift` and drifts are detected
- **THEN** system displays drift report with categorized drift types
- **AND** prompts user for resolution mode
- **AND** provides options: individual, adopt all, revert all, show only, exit

#### Scenario: Detect external override drift

- **WHEN** system tool (e.g., `/usr/bin/node`) takes precedence over ZERB-managed tool
- **THEN** drift type is classified as "EXTERNAL_OVERRIDE"
- **AND** report shows baseline version, active version, and active path
- **AND** explains external installation has taken precedence

#### Scenario: Detect version mismatch drift

- **WHEN** ZERB-managed tool version differs from baseline version
- **THEN** drift type is classified as "VERSION_MISMATCH"
- **AND** report shows baseline version and managed version
- **AND** explains ZERB is managing wrong version

#### Scenario: Detect missing tool drift

- **WHEN** tool is declared in baseline but not installed or active
- **THEN** drift type is classified as "MISSING"
- **AND** report shows baseline version
- **AND** indicates tool is not installed

#### Scenario: Detect extra tool drift

- **WHEN** ZERB has installed tool not declared in baseline
- **THEN** drift type is classified as "EXTRA"
- **AND** report shows managed version
- **AND** indicates tool is not in baseline

### Requirement: Interactive Drift Resolution

The system SHALL provide interactive resolution modes for handling detected drifts.

#### Scenario: Individual resolution mode

- **WHEN** user selects individual resolution mode
- **THEN** system prompts for action on each drift sequentially
- **AND** provides drift-specific action options (adopt, revert, skip)
- **AND** executes chosen action before moving to next drift

#### Scenario: Adopt all resolution mode

- **WHEN** user selects adopt all resolution mode
- **THEN** system updates baseline configuration to match environment for all drifts
- **AND** creates timestamped config file in `configs/` directory
- **AND** updates `.zerb-active` marker file
- **AND** updates `zerb.lua.active` symlink

#### Scenario: Revert all resolution mode

- **WHEN** user selects revert all resolution mode
- **THEN** system restores environment to match baseline for all drifts
- **AND** installs missing tools via mise
- **AND** reinstalls tools with version mismatches
- **AND** uninstalls extra tools

#### Scenario: Show only resolution mode

- **WHEN** user selects show only resolution mode
- **THEN** system displays drift report without making changes
- **AND** exits without prompting for further actions

### Requirement: Drift Action Application

The system SHALL apply drift resolution actions based on user choice.

#### Scenario: Apply adopt action for external override

- **WHEN** user chooses adopt for external override drift
- **THEN** system removes tool from baseline configuration
- **AND** acknowledges external management in updated config

#### Scenario: Apply revert action for external override

- **WHEN** user chooses revert for external override drift
- **THEN** system reinstalls tool via ZERB (mise)
- **AND** warns that it may conflict with system installation

#### Scenario: Apply adopt action for version mismatch

- **WHEN** user chooses adopt for version mismatch drift
- **THEN** system updates baseline version to match active version
- **AND** preserves tool backend specification if present

#### Scenario: Apply revert action for version mismatch

- **WHEN** user chooses revert for version mismatch drift
- **THEN** system reinstalls correct version via mise
- **AND** uses version from baseline configuration

#### Scenario: Apply adopt action for extra tool

- **WHEN** user chooses adopt for extra tool drift
- **THEN** system adds tool with current version to baseline
- **AND** includes tool in next config snapshot

#### Scenario: Apply revert action for extra tool

- **WHEN** user chooses revert for extra tool drift
- **THEN** system uninstalls tool via mise
- **AND** removes from managed tools list

### Requirement: Terminology Abstraction

The system SHALL abstract internal implementation details from user-facing output.

#### Scenario: Drift report uses ZERB terminology

- **WHEN** drift report is displayed to user
- **THEN** output uses "ZERB" to refer to managed tools
- **AND** does not expose "mise" or "chezmoi" in any messages

#### Scenario: Error messages use ZERB terminology

- **WHEN** error occurs during drift detection or resolution
- **THEN** error messages refer to ZERB operations
- **AND** do not expose internal tool names

### Requirement: Command Help and Documentation

The system SHALL provide clear help text and documentation for drift detection.

#### Scenario: Display drift command help

- **WHEN** user runs `zerb drift --help`
- **THEN** system displays command description
- **AND** explains three-way comparison model
- **AND** lists drift types that can be detected

#### Scenario: Display drift command in main help

- **WHEN** user runs `zerb --help`
- **THEN** drift command is listed with short description
- **AND** description indicates it detects environment drift

### Requirement: Configuration Immutability

The system SHALL maintain immutability of timestamped configuration files during drift resolution.

#### Scenario: Adopt action creates new config

- **WHEN** adopt action is applied
- **THEN** system creates new timestamped config file
- **AND** never modifies existing timestamped configs
- **AND** updates marker and symlink to point to new config

#### Scenario: Multiple adopt actions in sequence

- **WHEN** multiple adopt actions are applied in single session
- **THEN** each action creates new timestamped config
- **AND** configs form immutable history
- **AND** latest symlink always points to most recent

### Requirement: Isolation and Security

The system SHALL execute drift operations with proper isolation and without exposing secrets.

#### Scenario: Query managed tools with isolation

- **WHEN** system queries mise for managed tools
- **THEN** mise operations use ZERB-specific environment variables
- **AND** mise config points to `~/.config/zerb/mise/config.toml`
- **AND** mise data directory is `~/.config/zerb/mise/`

#### Scenario: Version detection without exposing paths

- **WHEN** system detects tool versions
- **THEN** only attempts `--version` and `-v` flags
- **AND** marks version as "unknown" if both fail
- **AND** does not log sensitive environment information

### Requirement: Test Coverage

The system SHALL include comprehensive tests for drift detection functionality.

#### Scenario: Unit tests for drift classification

- **WHEN** unit tests run for drift detector
- **THEN** all 7 drift types are tested
- **AND** code coverage is above 80%

#### Scenario: Integration tests for end-to-end workflow

- **WHEN** integration tests run
- **THEN** tests cover baseline parsing, managed query, active query, and drift detection
- **AND** tests use mock binaries and environments
- **AND** tests verify correct drift type classification
