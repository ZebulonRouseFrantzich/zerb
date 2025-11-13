# ZERB Restore Command Enhancement

**Status**: Future Enhancement (Post-MVP)  
**Version**: 1.0  
**Last Updated**: 2025-11-12  
**Related Commands**: `zerb uninit`, `zerb init`

---

## Table of Contents

- [Overview](#overview)
- [Command Specification](#command-specification)
- [Restore Scenarios](#restore-scenarios)
- [Technical Requirements](#technical-requirements)
- [User Experience](#user-experience)
- [Edge Cases](#edge-cases)
- [Implementation Phases](#implementation-phases)
- [Related Commands](#related-commands)
- [Safety Considerations](#safety-considerations)
- [Examples](#examples)

---

## Overview

### Purpose

The `zerb restore` command enables users to recover their ZERB environment from backups created during `zerb uninit` operations. This provides a safety net for users who want to temporarily remove ZERB or who accidentally uninstall it.

### Why It's Useful

- **Undo Capability**: Allows users to reverse `zerb uninit` operations
- **Experimentation Safety**: Users can safely try removing ZERB knowing they can restore
- **Migration Support**: Facilitates testing alternative setups without losing current configuration
- **Disaster Recovery**: Provides recovery path if ZERB directory is accidentally deleted
- **Confidence Building**: Reduces anxiety about running destructive operations

### Key Features

- Restore complete ZERB environment from timestamped backups
- Selective restoration (directory only, shell integration only, or both)
- Interactive backup selection when multiple backups exist
- Conflict detection and resolution
- Validation of backup integrity before restoration
- Dry-run mode to preview restoration actions

---

## Command Specification

### Basic Syntax

```bash
zerb restore [flags]
```

### Flags and Options

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--backup` | `-b` | string | latest | Backup timestamp to restore from |
| `--list` | `-l` | bool | false | List available backups and exit |
| `--directory-only` | | bool | false | Restore ZERB directory only (skip shell integration) |
| `--shell-only` | | bool | false | Restore shell integration only (skip directory) |
| `--force` | `-f` | bool | false | Skip confirmation prompts |
| `--dry-run` | | bool | false | Show what would be restored without making changes |
| `--keep-existing` | | bool | false | Keep existing ZERB installation (merge/overlay) |
| `--interactive` | `-i` | bool | true | Enable interactive mode (default) |
| `--verbose` | `-v` | bool | false | Show detailed restoration progress |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Restoration successful |
| 1 | General error (invalid arguments, backup not found) |
| 2 | Backup validation failed (corrupted or incomplete) |
| 3 | Conflict detected (ZERB already initialized) |
| 4 | Permission denied (cannot write to target locations) |
| 5 | User cancelled operation |

---

## Restore Scenarios

### Scenario 1: Full Restore (Default)

**Use Case**: User ran `zerb uninit` and wants to completely restore their environment.

**Command**:
```bash
zerb restore
```

**Actions**:
1. Detect latest backup automatically
2. Restore `~/.config/zerb/` directory structure
3. Restore shell integration to RC files
4. Verify restoration integrity
5. Display summary of restored components

**Expected Output**:
```
🔍 Scanning for ZERB backups...

Found backup from 2025-11-12 14:30:22 (2 hours ago)
  Location: ~/.config/zerb-uninit-backup.20251112-143022/
  Components:
    ✓ ZERB directory (1.2 GB)
    ✓ Shell integration (.bashrc, .zshrc)
    ✓ Metadata (uninit.json)

Restore this backup? [Y/n]: y

📦 Restoring ZERB directory...
  ✓ Restored configs/ (45 files)
  ✓ Restored bin/ (2 binaries)
  ✓ Restored mise/ (12 tools)
  ✓ Restored chezmoi/ (23 dotfiles)

🐚 Restoring shell integration...
  ✓ Restored .bashrc activation line
  ✓ Restored .zshrc activation line

✅ Restoration complete!

Next steps:
  1. Restart your shell or run: source ~/.zshrc
  2. Verify tools are available: zerb drift
  3. Check configuration: zerb status
```

### Scenario 2: Partial Restore (Directory Only)

**Use Case**: User wants to restore ZERB directory but manually manage shell integration.

**Command**:
```bash
zerb restore --directory-only
```

**Actions**:
1. Restore `~/.config/zerb/` directory
2. Skip shell RC file modifications
3. Provide instructions for manual shell setup

**Expected Output**:
```
📦 Restoring ZERB directory only...
  ✓ Restored ~/.config/zerb/ (1.2 GB)

⚠️  Shell integration NOT restored.

To complete setup, add this line to your shell RC file:
  eval "$(zerb activate bash)"  # or zsh, fish

✅ Directory restoration complete!
```

### Scenario 3: Partial Restore (Shell Only)

**Use Case**: User has ZERB directory but shell integration was removed.

**Command**:
```bash
zerb restore --shell-only
```

**Actions**:
1. Skip directory restoration
2. Restore shell integration from backup metadata
3. Verify ZERB directory exists

**Expected Output**:
```
🐚 Restoring shell integration only...
  ✓ Verified ZERB directory exists at ~/.config/zerb/
  ✓ Restored .bashrc activation line
  ✓ Restored .zshrc activation line

✅ Shell integration restored!

Restart your shell or run: source ~/.zshrc
```

### Scenario 4: Restore from Specific Backup

**Use Case**: Multiple backups exist; user wants to restore from a specific timestamp.

**Command**:
```bash
# First, list available backups
zerb restore --list

# Then restore specific backup
zerb restore --backup 20251110-093045
```

**Expected Output (--list)**:
```
Available ZERB backups:

  1. 2025-11-12 14:30:22 (2 hours ago)
     Location: ~/.config/zerb-uninit-backup.20251112-143022/
     Size: 1.2 GB
     Components: directory + shell integration

  2. 2025-11-10 09:30:45 (2 days ago)
     Location: ~/.config/zerb-uninit-backup.20251110-093045/
     Size: 1.1 GB
     Components: directory + shell integration

  3. 2025-11-08 16:15:30 (4 days ago)
     Location: ~/.config/zerb-uninit-backup.20251108-161530/
     Size: 1.0 GB
     Components: directory only (partial backup)

Use: zerb restore --backup <timestamp>
```

### Scenario 5: Conflict Resolution

**Use Case**: ZERB is already initialized when restore is attempted.

**Command**:
```bash
zerb restore
```

**Expected Output**:
```
⚠️  ZERB is already initialized at ~/.config/zerb/

Options:
  1. Backup current installation and restore from backup
  2. Merge backup with current installation (advanced)
  3. Cancel restoration

Choice [1-3]: 1

📦 Creating backup of current installation...
  ✓ Backed up to ~/.config/zerb-backup.20251112-163045/

📦 Restoring from backup...
  ✓ Restoration complete!

Note: Your previous installation is backed up at:
  ~/.config/zerb-backup.20251112-163045/
```

---

## Technical Requirements

### Backup Metadata Structure

To enable restoration, `zerb uninit` must create a comprehensive backup with metadata.

#### Directory Structure

```
~/.config/zerb-uninit-backup.20251112-143022/
├── uninit.json                    # Metadata file
├── zerb/                          # Complete ZERB directory backup
│   ├── configs/
│   ├── bin/
│   ├── mise/
│   ├── chezmoi/
│   └── ...
└── shell-integration/             # Shell RC file backups
    ├── bashrc.backup
    ├── bashrc.metadata.json
    ├── zshrc.backup
    └── zshrc.metadata.json
```

#### Metadata File Format (uninit.json)

```json
{
  "version": "1.0",
  "timestamp": "2025-11-12T14:30:22Z",
  "uninit_command": "zerb uninit",
  "uninit_flags": {
    "keep_configs": false,
    "keep_cache": false,
    "force": false
  },
  "system_info": {
    "hostname": "dev-machine",
    "username": "developer",
    "os": "linux",
    "distro": "ubuntu",
    "arch": "amd64"
  },
  "backup_components": {
    "directory": {
      "included": true,
      "path": "zerb/",
      "size_bytes": 1288490188,
      "file_count": 1247,
      "checksum": "sha256:abc123..."
    },
    "shell_integration": {
      "included": true,
      "shells": [
        {
          "type": "bash",
          "rc_file": "/home/developer/.bashrc",
          "backup_file": "shell-integration/bashrc.backup",
          "activation_line": "eval \"$(zerb activate bash)\"",
          "line_number": 42,
          "checksum": "sha256:def456..."
        },
        {
          "type": "zsh",
          "rc_file": "/home/developer/.zshrc",
          "backup_file": "shell-integration/zshrc.backup",
          "activation_line": "eval \"$(zerb activate zsh)\"",
          "line_number": 38,
          "checksum": "sha256:ghi789..."
        }
      ]
    }
  },
  "zerb_version": "v0.1.0-alpha",
  "config_snapshot": {
    "active_config": "zerb.lua.20251112T143000Z",
    "tool_count": 12,
    "config_file_count": 8
  }
}
```

#### Shell RC Metadata (bashrc.metadata.json)

```json
{
  "original_path": "/home/developer/.bashrc",
  "backup_timestamp": "2025-11-12T14:30:22Z",
  "file_size": 3456,
  "checksum": "sha256:def456...",
  "activation_line": {
    "content": "eval \"$(zerb activate bash)\"",
    "line_number": 42,
    "surrounding_context": {
      "before": "# User configuration",
      "after": "# End of .bashrc"
    }
  },
  "permissions": "0644",
  "owner": "developer",
  "group": "developer"
}
```

### Validation Requirements

Before restoration, `zerb restore` must validate:

1. **Backup Integrity**
   - Verify `uninit.json` exists and is valid JSON
   - Verify checksums match for all backed-up files
   - Ensure backup directory structure is complete

2. **Version Compatibility**
   - Check if backup was created by compatible ZERB version
   - Warn if restoring from older/newer version
   - Provide migration path if schema changed

3. **System Compatibility**
   - Verify OS/architecture matches (warn if different)
   - Check if target paths are writable
   - Ensure sufficient disk space

4. **Conflict Detection**
   - Check if `~/.config/zerb/` already exists
   - Check if shell RC files already have ZERB activation
   - Detect if current ZERB version differs from backup

### Restoration Process

#### Phase 1: Pre-flight Checks

```go
type RestoreValidator struct {
    BackupPath string
    Metadata   *UninitMetadata
}

func (v *RestoreValidator) Validate() error {
    // 1. Verify backup exists
    if !backupExists(v.BackupPath) {
        return ErrBackupNotFound
    }
    
    // 2. Load and validate metadata
    metadata, err := loadMetadata(v.BackupPath)
    if err != nil {
        return fmt.Errorf("invalid metadata: %w", err)
    }
    
    // 3. Verify checksums
    if err := verifyChecksums(metadata); err != nil {
        return fmt.Errorf("checksum validation failed: %w", err)
    }
    
    // 4. Check version compatibility
    if !isCompatibleVersion(metadata.ZerbVersion) {
        return ErrIncompatibleVersion
    }
    
    // 5. Check system compatibility
    if err := checkSystemCompatibility(metadata.SystemInfo); err != nil {
        // Warning only, not fatal
        log.Warn("System mismatch: %v", err)
    }
    
    // 6. Check for conflicts
    if zerbExists() {
        return ErrZerbAlreadyExists
    }
    
    return nil
}
```

#### Phase 2: Directory Restoration

```go
func restoreDirectory(backupPath string, metadata *UninitMetadata) error {
    targetDir := filepath.Join(os.Getenv("HOME"), ".config", "zerb")
    sourceDir := filepath.Join(backupPath, "zerb")
    
    // 1. Create target directory
    if err := os.MkdirAll(targetDir, 0755); err != nil {
        return fmt.Errorf("create target directory: %w", err)
    }
    
    // 2. Copy directory structure
    if err := copyDir(sourceDir, targetDir); err != nil {
        return fmt.Errorf("copy directory: %w", err)
    }
    
    // 3. Verify restoration
    if err := verifyRestoration(targetDir, metadata); err != nil {
        // Rollback on failure
        os.RemoveAll(targetDir)
        return fmt.Errorf("verification failed: %w", err)
    }
    
    return nil
}
```

#### Phase 3: Shell Integration Restoration

```go
func restoreShellIntegration(backupPath string, metadata *UninitMetadata) error {
    for _, shell := range metadata.BackupComponents.ShellIntegration.Shells {
        // 1. Read backup RC file
        backupFile := filepath.Join(backupPath, shell.BackupFile)
        content, err := os.ReadFile(backupFile)
        if err != nil {
            return fmt.Errorf("read backup file: %w", err)
        }
        
        // 2. Check if RC file exists
        rcExists, err := fileExists(shell.RCFile)
        if err != nil {
            return err
        }
        
        if !rcExists {
            // Restore entire RC file
            if err := os.WriteFile(shell.RCFile, content, 0644); err != nil {
                return fmt.Errorf("restore RC file: %w", err)
            }
        } else {
            // Add activation line only
            if err := addActivationLine(shell.RCFile, shell.ActivationLine); err != nil {
                return fmt.Errorf("add activation line: %w", err)
            }
        }
    }
    
    return nil
}
```

---

## User Experience

### Interactive Restoration Flow

```
$ zerb restore

🔍 Scanning for ZERB backups...

Found 2 backups:

  1. 2025-11-12 14:30:22 (2 hours ago) - RECOMMENDED
     Size: 1.2 GB | Components: Full backup

  2. 2025-11-10 09:30:45 (2 days ago)
     Size: 1.1 GB | Components: Full backup

Which backup would you like to restore? [1-2]: 1

📋 Restoration Plan:
  ✓ Restore ZERB directory (~/.config/zerb/)
    - 1,247 files (1.2 GB)
    - 12 tools installed
    - 8 configuration files
  
  ✓ Restore shell integration
    - .bashrc (line 42)
    - .zshrc (line 38)

⚠️  This will:
  • Create ~/.config/zerb/ directory
  • Modify your shell RC files
  • Make ZERB tools available globally

Continue with restoration? [Y/n]: y

📦 Restoring ZERB directory...
  [████████████████████████████████] 100% (1.2 GB)
  ✓ Restored configs/ (45 files)
  ✓ Restored bin/ (2 binaries)
  ✓ Restored mise/ (12 tools)
  ✓ Restored chezmoi/ (23 dotfiles)

🐚 Restoring shell integration...
  ✓ Restored .bashrc activation line
  ✓ Restored .zshrc activation line

🔍 Verifying restoration...
  ✓ Directory structure intact
  ✓ File checksums match
  ✓ Shell integration valid

✅ Restoration complete!

Summary:
  • ZERB directory: ~/.config/zerb/
  • Tools restored: 12
  • Configs restored: 8
  • Shell integration: bash, zsh

Next steps:
  1. Restart your shell: exec $SHELL
  2. Verify installation: zerb status
  3. Check for drift: zerb drift

Your ZERB environment has been fully restored!
```

### Error Handling Examples

#### Error: Backup Not Found

```
$ zerb restore --backup 20251101-120000

❌ Error: Backup not found

No backup found with timestamp: 20251101-120000

Available backups:
  • 20251112-143022 (2 hours ago)
  • 20251110-093045 (2 days ago)

Use: zerb restore --list
```

#### Error: Corrupted Backup

```
$ zerb restore

🔍 Scanning for ZERB backups...

Found backup from 2025-11-12 14:30:22

⚠️  Backup validation failed:
  ✗ Checksum mismatch: zerb/bin/mise
  ✗ Missing file: zerb/configs/zerb.lua.20251112T143000Z

This backup appears to be corrupted or incomplete.

Options:
  1. Try restoring anyway (not recommended)
  2. Try different backup
  3. Cancel

Choice [1-3]: 2
```

#### Error: ZERB Already Exists

```
$ zerb restore

⚠️  ZERB is already initialized at ~/.config/zerb/

Current installation:
  • Version: v0.1.0-alpha
  • Tools: 15 installed
  • Last modified: 2025-11-12 16:30:00

Backup to restore:
  • Created: 2025-11-12 14:30:22
  • Tools: 12 installed

Options:
  1. Backup current and restore from backup
  2. Cancel restoration

Choice [1-2]: 1

📦 Creating backup of current installation...
  ✓ Backed up to ~/.config/zerb-backup.20251112-163500/

📦 Restoring from backup...
  ✓ Restoration complete!
```

### Progress Indicators

For long-running operations, provide detailed progress:

```
📦 Restoring ZERB directory...

Copying files: [████████████████████████████████] 1247/1247 files
Progress: 1.2 GB / 1.2 GB (100%)
Speed: 45.2 MB/s
Time remaining: 0s

  ✓ configs/ (45 files, 2.3 MB)
  ✓ bin/ (2 files, 45.8 MB)
  ⏳ mise/ (1200 files, 1.1 GB) - 87%
  ⏳ chezmoi/ (23 files, 5.4 MB) - pending
```

---

## Edge Cases

### Edge Case 1: Multiple Backups Available

**Scenario**: User has multiple backups from different uninit operations.

**Handling**:
- Default to most recent backup
- Provide `--list` flag to show all backups
- Allow selection via `--backup` flag or interactive prompt
- Display backup age, size, and completeness

**Implementation**:
```go
func findBackups() ([]BackupInfo, error) {
    homeDir, _ := os.UserHomeDir()
    pattern := filepath.Join(homeDir, ".config", "zerb-uninit-backup.*")
    
    matches, err := filepath.Glob(pattern)
    if err != nil {
        return nil, err
    }
    
    var backups []BackupInfo
    for _, path := range matches {
        metadata, err := loadBackupMetadata(path)
        if err != nil {
            log.Warn("Skipping invalid backup: %s", path)
            continue
        }
        backups = append(backups, BackupInfo{
            Path:      path,
            Timestamp: metadata.Timestamp,
            Metadata:  metadata,
        })
    }
    
    // Sort by timestamp (newest first)
    sort.Slice(backups, func(i, j int) bool {
        return backups[i].Timestamp.After(backups[j].Timestamp)
    })
    
    return backups, nil
}
```

### Edge Case 2: Partial Backups (Interrupted Uninit)

**Scenario**: `zerb uninit` was interrupted, creating incomplete backup.

**Handling**:
- Detect incomplete backups via metadata validation
- Mark as "partial" in backup list
- Warn user before restoration
- Allow restoration with `--force` flag
- Provide detailed information about missing components

**Detection**:
```go
func validateBackupCompleteness(metadata *UninitMetadata) error {
    var missing []string
    
    // Check directory backup
    if metadata.BackupComponents.Directory.Included {
        dirPath := filepath.Join(backupPath, "zerb")
        if !dirExists(dirPath) {
            missing = append(missing, "ZERB directory")
        }
    }
    
    // Check shell integration backups
    if metadata.BackupComponents.ShellIntegration.Included {
        for _, shell := range metadata.BackupComponents.ShellIntegration.Shells {
            backupFile := filepath.Join(backupPath, shell.BackupFile)
            if !fileExists(backupFile) {
                missing = append(missing, fmt.Sprintf("%s RC file", shell.Type))
            }
        }
    }
    
    if len(missing) > 0 {
        return &IncompleteBackupError{
            MissingComponents: missing,
        }
    }
    
    return nil
}
```

### Edge Case 3: ZERB Already Initialized

**Scenario**: User attempts restore when ZERB is already installed.

**Handling**:
- Detect existing ZERB installation
- Offer three options:
  1. Backup current and restore from backup (safest)
  2. Merge backup with current (advanced)
  3. Cancel operation
- Create timestamped backup of current installation
- Provide rollback capability

**Conflict Resolution**:
```go
func handleExistingInstallation(opts RestoreOptions) error {
    if !opts.Force {
        choice := promptConflictResolution()
        switch choice {
        case "backup-and-restore":
            if err := backupCurrentInstallation(); err != nil {
                return err
            }
            if err := removeCurrentInstallation(); err != nil {
                return err
            }
        case "merge":
            return mergeBackupWithCurrent(opts)
        case "cancel":
            return ErrUserCancelled
        }
    }
    
    return nil
}
```

### Edge Case 4: Backup File Permissions Issues

**Scenario**: Backup files have incorrect permissions or ownership.

**Handling**:
- Detect permission issues during validation
- Attempt to fix permissions automatically
- Warn user if manual intervention required
- Provide clear instructions for resolution

**Permission Handling**:
```go
func restoreWithPermissions(sourcePath, targetPath string, metadata FileMetadata) error {
    // Copy file
    if err := copyFile(sourcePath, targetPath); err != nil {
        return err
    }
    
    // Restore original permissions
    mode := os.FileMode(metadata.Permissions)
    if err := os.Chmod(targetPath, mode); err != nil {
        log.Warn("Could not restore permissions for %s: %v", targetPath, err)
        // Non-fatal, continue
    }
    
    // Restore ownership (if running as root)
    if os.Geteuid() == 0 {
        if err := os.Chown(targetPath, metadata.UID, metadata.GID); err != nil {
            log.Warn("Could not restore ownership for %s: %v", targetPath, err)
        }
    }
    
    return nil
}
```

### Edge Case 5: Cross-Machine Restoration

**Scenario**: User restores backup on different machine (different username, paths, etc.).

**Handling**:
- Detect system differences (username, hostname, paths)
- Warn user about potential issues
- Offer path remapping for common scenarios
- Update absolute paths in configuration files
- Validate tool availability on target system

**Path Remapping**:
```go
func remapPaths(metadata *UninitMetadata) error {
    currentUser := os.Getenv("USER")
    backupUser := metadata.SystemInfo.Username
    
    if currentUser != backupUser {
        log.Warn("Restoring backup from different user: %s -> %s", backupUser, currentUser)
        
        // Update paths in metadata
        for i := range metadata.BackupComponents.ShellIntegration.Shells {
            shell := &metadata.BackupComponents.ShellIntegration.Shells[i]
            shell.RCFile = strings.Replace(
                shell.RCFile,
                "/home/"+backupUser,
                "/home/"+currentUser,
                1,
            )
        }
    }
    
    return nil
}
```

### Edge Case 6: Version Mismatch

**Scenario**: Backup created by different ZERB version.

**Handling**:
- Detect version differences
- Check compatibility matrix
- Warn if major version differs
- Provide migration path if schema changed
- Allow restoration with `--force` for minor version differences

**Version Compatibility**:
```go
func checkVersionCompatibility(backupVersion, currentVersion string) error {
    backupMajor := parseMajorVersion(backupVersion)
    currentMajor := parseMajorVersion(currentVersion)
    
    if backupMajor != currentMajor {
        return &VersionMismatchError{
            BackupVersion:  backupVersion,
            CurrentVersion: currentVersion,
            Message:        "Major version mismatch - migration required",
        }
    }
    
    if backupVersion != currentVersion {
        log.Warn("Version mismatch: backup=%s, current=%s", backupVersion, currentVersion)
        log.Warn("Restoration may require manual adjustments")
    }
    
    return nil
}
```

---

## Implementation Phases

### Phase 1: MVP (Core Functionality)

**Goal**: Basic restoration capability for common use cases.

**Features**:
- ✅ Restore from latest backup (automatic detection)
- ✅ Full restoration (directory + shell integration)
- ✅ Basic validation (metadata, checksums)
- ✅ Interactive confirmation prompts
- ✅ Simple conflict detection (ZERB exists)
- ✅ Progress indicators for long operations

**Scope**:
- Single backup support (latest only)
- No partial restoration
- No merge capability
- Basic error messages

**Success Criteria**:
- Can restore from `zerb uninit` backup
- Validates backup integrity
- Detects and handles conflicts
- Provides clear user feedback
- >80% test coverage

**Estimated Effort**: 2-3 days

### Phase 2: Enhanced Selection

**Goal**: Support multiple backups and selective restoration.

**Features**:
- ✅ List all available backups (`--list`)
- ✅ Restore from specific backup (`--backup`)
- ✅ Partial restoration (`--directory-only`, `--shell-only`)
- ✅ Backup age and size display
- ✅ Interactive backup selection

**Scope**:
- Multiple backup management
- Selective component restoration
- Enhanced backup metadata
- Improved user prompts

**Success Criteria**:
- Can list and select from multiple backups
- Can restore individual components
- Clear indication of backup completeness
- Intuitive selection interface

**Estimated Effort**: 2-3 days

### Phase 3: Advanced Features

**Goal**: Handle edge cases and provide advanced capabilities.

**Features**:
- ✅ Merge mode (`--keep-existing`)
- ✅ Dry-run mode (`--dry-run`)
- ✅ Cross-machine restoration
- ✅ Path remapping
- ✅ Version migration
- ✅ Partial backup handling
- ✅ Detailed validation reports

**Scope**:
- Complex conflict resolution
- Backup migration between versions
- Advanced merge strategies
- Comprehensive validation

**Success Criteria**:
- Handles all documented edge cases
- Provides migration path for version changes
- Supports cross-machine scenarios
- Detailed error reporting

**Estimated Effort**: 3-4 days

### Phase 4: Polish & Optimization

**Goal**: Improve performance and user experience.

**Features**:
- ✅ Parallel file copying
- ✅ Incremental restoration
- ✅ Backup compression support
- ✅ Automatic backup cleanup
- ✅ Restoration history tracking
- ✅ Enhanced progress indicators
- ✅ Rollback capability

**Scope**:
- Performance optimization
- UX improvements
- Additional safety features
- Documentation

**Success Criteria**:
- Fast restoration (>50 MB/s)
- Smooth progress indicators
- Comprehensive documentation
- User satisfaction

**Estimated Effort**: 2-3 days

### Total Estimated Effort: 9-13 days

---

## Related Commands

### Integration with `zerb uninit`

The `zerb restore` command is designed to work seamlessly with `zerb uninit`.

#### Uninit Responsibilities

`zerb uninit` must:
1. Create comprehensive backup with metadata
2. Use consistent timestamp format (YYYYMMDD-HHMMSS)
3. Include all necessary restoration information
4. Validate backup creation before removal
5. Display backup location to user

#### Restore Expectations

`zerb restore` expects:
1. Backup directory at `~/.config/zerb-uninit-backup.TIMESTAMP/`
2. Valid `uninit.json` metadata file
3. Complete directory structure
4. Shell integration backups with metadata
5. Checksums for validation

#### Example Workflow

```bash
# User uninitializes ZERB
$ zerb uninit
✓ Created backup at ~/.config/zerb-uninit-backup.20251112-143022/
✓ Removed ZERB directory
✓ Removed shell integration

# Later, user wants to restore
$ zerb restore
🔍 Found backup from 2025-11-12 14:30:22
✓ Restoration complete!
```

### Integration with `zerb init`

The `zerb restore` command should be aware of `zerb init` state.

#### Conflict Scenarios

1. **ZERB Already Initialized**
   - `zerb restore` detects existing installation
   - Offers to backup current before restoring
   - Prevents data loss

2. **Fresh System**
   - `zerb restore` works like `zerb init` + configuration
   - Restores complete environment
   - No conflicts

#### Recommended Workflow

```bash
# On new machine with backup
$ zerb restore
✓ Restored ZERB environment
✓ Shell integration configured

# Equivalent to:
$ zerb init
$ # Manual configuration restoration
```

### Complementary Commands

#### `zerb backup` (Future)

Create manual backups (not just during uninit):

```bash
$ zerb backup
✓ Created backup at ~/.config/zerb-backup.20251112-163000/

$ zerb backup --name "before-major-upgrade"
✓ Created backup at ~/.config/zerb-backup.before-major-upgrade/
```

#### `zerb backup list` (Future)

List all backups (uninit + manual):

```bash
$ zerb backup list

Backups:
  1. uninit-20251112-143022 (2 hours ago) - From uninit
  2. before-major-upgrade (1 day ago) - Manual backup
  3. uninit-20251110-093045 (2 days ago) - From uninit
```

#### `zerb backup clean` (Future)

Remove old backups:

```bash
$ zerb backup clean --older-than 30d
Removed 3 backups older than 30 days
```

---

## Safety Considerations

### What Should NOT Be Restored

To avoid security issues, certain items should never be restored automatically:

#### 1. Secrets and Credentials

**Risk**: Secrets may be compromised or outdated.

**Handling**:
- Never restore GPG keys automatically
- Never restore SSH keys automatically
- Never restore API tokens or passwords
- Prompt user to re-enter secrets after restoration

**Implementation**:
```go
var sensitivePatterns = []string{
    "*.key",
    "*.pem",
    "*.gpg",
    "*_rsa",
    "*_ed25519",
    "*.token",
    "*.secret",
}

func shouldSkipFile(path string) bool {
    for _, pattern := range sensitivePatterns {
        if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
            log.Warn("Skipping sensitive file: %s", path)
            return true
        }
    }
    return false
}
```

#### 2. Machine-Specific Configuration

**Risk**: Configuration may not apply to target machine.

**Handling**:
- Warn about machine-specific settings
- Provide option to skip machine-specific configs
- Update paths for current machine

**Examples**:
- Absolute paths to tools
- Hardware-specific settings
- Network configuration
- Display settings

#### 3. Cached Data

**Risk**: Cache may be stale or corrupted.

**Handling**:
- Skip cache directories by default
- Provide `--include-cache` flag for explicit inclusion
- Regenerate cache after restoration

**Directories to Skip**:
- `~/.config/zerb/cache/`
- `~/.config/zerb/mise/downloads/`
- `~/.config/zerb/tmp/`
- `~/.config/zerb/logs/`

#### 4. Temporary Files

**Risk**: Temporary files may be invalid or corrupted.

**Handling**:
- Never restore transaction files
- Never restore lock files
- Clean up temporary files after restoration

### Validation Steps Before Restoration

#### 1. Backup Integrity

```go
func validateBackupIntegrity(metadata *UninitMetadata) error {
    // Verify metadata structure
    if err := validateMetadataSchema(metadata); err != nil {
        return fmt.Errorf("invalid metadata schema: %w", err)
    }
    
    // Verify checksums
    for _, component := range metadata.BackupComponents {
        if err := verifyChecksum(component); err != nil {
            return fmt.Errorf("checksum mismatch: %w", err)
        }
    }
    
    // Verify completeness
    if err := validateCompleteness(metadata); err != nil {
        return fmt.Errorf("incomplete backup: %w", err)
    }
    
    return nil
}
```

#### 2. System Compatibility

```go
func validateSystemCompatibility(metadata *UninitMetadata) error {
    // Check OS compatibility
    if runtime.GOOS != metadata.SystemInfo.OS {
        return fmt.Errorf("OS mismatch: backup=%s, current=%s",
            metadata.SystemInfo.OS, runtime.GOOS)
    }
    
    // Check architecture compatibility
    if runtime.GOARCH != metadata.SystemInfo.Arch {
        log.Warn("Architecture mismatch: backup=%s, current=%s",
            metadata.SystemInfo.Arch, runtime.GOARCH)
    }
    
    // Check disk space
    requiredSpace := metadata.BackupComponents.Directory.SizeBytes
    availableSpace := getAvailableDiskSpace()
    if availableSpace < requiredSpace {
        return fmt.Errorf("insufficient disk space: need %d, have %d",
            requiredSpace, availableSpace)
    }
    
    return nil
}
```

#### 3. Permission Validation

```go
func validatePermissions() error {
    // Check write permission for ZERB directory
    zerbDir := filepath.Join(os.Getenv("HOME"), ".config", "zerb")
    if err := checkWritePermission(filepath.Dir(zerbDir)); err != nil {
        return fmt.Errorf("cannot write to %s: %w", filepath.Dir(zerbDir), err)
    }
    
    // Check write permission for shell RC files
    for _, shell := range []string{"bash", "zsh", "fish"} {
        rcPath, _ := getRCFilePath(shell)
        if fileExists(rcPath) {
            if err := checkWritePermission(rcPath); err != nil {
                log.Warn("Cannot write to %s: %v", rcPath, err)
            }
        }
    }
    
    return nil
}
```

#### 4. Conflict Detection

```go
func detectConflicts() ([]Conflict, error) {
    var conflicts []Conflict
    
    // Check if ZERB directory exists
    zerbDir := filepath.Join(os.Getenv("HOME"), ".config", "zerb")
    if dirExists(zerbDir) {
        conflicts = append(conflicts, Conflict{
            Type:        "directory",
            Path:        zerbDir,
            Description: "ZERB directory already exists",
            Resolution:  "backup-and-replace",
        })
    }
    
    // Check if shell integration exists
    for _, shell := range []string{"bash", "zsh", "fish"} {
        rcPath, _ := getRCFilePath(shell)
        if hasActivationLine(rcPath) {
            conflicts = append(conflicts, Conflict{
                Type:        "shell-integration",
                Path:        rcPath,
                Description: "ZERB activation already present",
                Resolution:  "skip-or-replace",
            })
        }
    }
    
    return conflicts, nil
}
```

### Rollback Capability

If restoration fails, provide automatic rollback:

```go
func restoreWithRollback(opts RestoreOptions) error {
    // Create rollback point
    rollback := NewRollbackManager()
    defer rollback.Cleanup()
    
    // Phase 1: Backup current state (if exists)
    if zerbExists() {
        if err := rollback.BackupCurrent(); err != nil {
            return fmt.Errorf("failed to create rollback point: %w", err)
        }
    }
    
    // Phase 2: Restore directory
    if err := restoreDirectory(opts); err != nil {
        rollback.Restore()
        return fmt.Errorf("directory restoration failed: %w", err)
    }
    rollback.Checkpoint("directory")
    
    // Phase 3: Restore shell integration
    if err := restoreShellIntegration(opts); err != nil {
        rollback.Restore()
        return fmt.Errorf("shell integration failed: %w", err)
    }
    rollback.Checkpoint("shell")
    
    // Phase 4: Verify restoration
    if err := verifyRestoration(opts); err != nil {
        rollback.Restore()
        return fmt.Errorf("verification failed: %w", err)
    }
    
    // Success - commit changes
    rollback.Commit()
    return nil
}
```

---

## Examples

### Example 1: Simple Restoration

```bash
# Restore from latest backup
$ zerb restore

🔍 Scanning for ZERB backups...
Found backup from 2025-11-12 14:30:22 (2 hours ago)

Restore this backup? [Y/n]: y

📦 Restoring ZERB directory...
  ✓ Restored configs/ (45 files)
  ✓ Restored bin/ (2 binaries)
  ✓ Restored mise/ (12 tools)
  ✓ Restored chezmoi/ (23 dotfiles)

🐚 Restoring shell integration...
  ✓ Restored .bashrc activation line
  ✓ Restored .zshrc activation line

✅ Restoration complete!
```

### Example 2: List and Select Backup

```bash
# List available backups
$ zerb restore --list

Available ZERB backups:

  1. 2025-11-12 14:30:22 (2 hours ago)
     Location: ~/.config/zerb-uninit-backup.20251112-143022/
     Size: 1.2 GB
     Components: directory + shell integration

  2. 2025-11-10 09:30:45 (2 days ago)
     Location: ~/.config/zerb-uninit-backup.20251110-093045/
     Size: 1.1 GB
     Components: directory + shell integration

# Restore specific backup
$ zerb restore --backup 20251110-093045

📦 Restoring from backup: 2025-11-10 09:30:45
✓ Restoration complete!
```

### Example 3: Directory Only Restoration

```bash
# Restore directory without shell integration
$ zerb restore --directory-only

📦 Restoring ZERB directory only...
  ✓ Restored ~/.config/zerb/ (1.2 GB)

⚠️  Shell integration NOT restored.

To complete setup, add this line to your shell RC file:
  eval "$(zerb activate bash)"  # or zsh, fish

✅ Directory restoration complete!
```

### Example 4: Dry Run

```bash
# Preview restoration without making changes
$ zerb restore --dry-run

🔍 Scanning for ZERB backups...
Found backup from 2025-11-12 14:30:22

📋 Restoration Plan (DRY RUN):

Would restore:
  ✓ ZERB directory (~/.config/zerb/)
    - 1,247 files (1.2 GB)
    - configs/ (45 files)
    - bin/ (2 binaries)
    - mise/ (1,200 files)
    - chezmoi/ (23 files)
  
  ✓ Shell integration
    - .bashrc (line 42)
    - .zshrc (line 38)

No changes made (dry run mode).
```

### Example 5: Force Restoration

```bash
# Restore without prompts
$ zerb restore --force

📦 Restoring ZERB directory...
  ✓ Restored configs/ (45 files)
  ✓ Restored bin/ (2 binaries)
  ✓ Restored mise/ (12 tools)
  ✓ Restored chezmoi/ (23 dotfiles)

🐚 Restoring shell integration...
  ✓ Restored .bashrc activation line
  ✓ Restored .zshrc activation line

✅ Restoration complete!
```

### Example 6: Verbose Output

```bash
# Show detailed restoration progress
$ zerb restore --verbose

🔍 Scanning for ZERB backups...
  Searching: ~/.config/zerb-uninit-backup.*
  Found: ~/.config/zerb-uninit-backup.20251112-143022/

📋 Loading backup metadata...
  Reading: uninit.json
  Version: 1.0
  Timestamp: 2025-11-12T14:30:22Z
  Components: directory, shell-integration

🔍 Validating backup...
  ✓ Metadata schema valid
  ✓ Checksum: zerb/ (sha256:abc123...)
  ✓ Checksum: bashrc.backup (sha256:def456...)
  ✓ Checksum: zshrc.backup (sha256:ghi789...)
  ✓ Backup complete and valid

📦 Restoring ZERB directory...
  Creating: ~/.config/zerb/
  Copying: configs/ (45 files, 2.3 MB)
  Copying: bin/ (2 files, 45.8 MB)
  Copying: mise/ (1200 files, 1.1 GB)
  Copying: chezmoi/ (23 files, 5.4 MB)
  ✓ Directory restored

🐚 Restoring shell integration...
  Processing: .bashrc
    Reading backup: shell-integration/bashrc.backup
    Checking existing: /home/developer/.bashrc
    Adding activation line at line 42
  Processing: .zshrc
    Reading backup: shell-integration/zshrc.backup
    Checking existing: /home/developer/.zshrc
    Adding activation line at line 38
  ✓ Shell integration restored

🔍 Verifying restoration...
  ✓ Directory structure intact
  ✓ File count matches: 1247
  ✓ Total size matches: 1.2 GB
  ✓ Shell integration valid

✅ Restoration complete!
```

---

## Appendix: Implementation Checklist

### Core Functionality
- [ ] Backup discovery and listing
- [ ] Metadata parsing and validation
- [ ] Checksum verification
- [ ] Directory restoration
- [ ] Shell integration restoration
- [ ] Progress indicators
- [ ] Error handling and rollback

### User Interface
- [ ] Interactive backup selection
- [ ] Confirmation prompts
- [ ] Progress bars
- [ ] Success/error messages
- [ ] Verbose mode output
- [ ] Dry-run mode

### Safety Features
- [ ] Backup integrity validation
- [ ] Conflict detection
- [ ] Rollback capability
- [ ] Permission validation
- [ ] Sensitive file filtering
- [ ] System compatibility checks

### Edge Cases
- [ ] Multiple backups handling
- [ ] Partial backup detection
- [ ] ZERB already exists handling
- [ ] Permission issues handling
- [ ] Cross-machine restoration
- [ ] Version mismatch handling

### Testing
- [ ] Unit tests for all functions
- [ ] Integration tests for full restoration
- [ ] Edge case tests
- [ ] Error handling tests
- [ ] Rollback tests
- [ ] >80% code coverage

### Documentation
- [ ] Command reference
- [ ] User guide
- [ ] Examples
- [ ] Troubleshooting guide
- [ ] API documentation

---

## Conclusion

The `zerb restore` command provides a comprehensive solution for recovering ZERB environments from backups. By implementing this feature in phases, we can deliver core functionality quickly while building toward a robust, user-friendly restoration system that handles edge cases gracefully.

The command's design prioritizes:
- **Safety**: Extensive validation and rollback capability
- **Usability**: Clear prompts and progress indicators
- **Flexibility**: Multiple restoration modes and options
- **Reliability**: Comprehensive error handling and recovery

This enhancement will significantly improve user confidence in ZERB by providing a safety net for destructive operations and enabling easy environment recovery across machines.
