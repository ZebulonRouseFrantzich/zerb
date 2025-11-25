# Design Document: Config Remove Command

## Overview

The `zerb config remove` command removes configuration files from ZERB tracking. This design document captures architectural decisions, trade-offs, and implementation approach following the established patterns from `config add` and `config list`.

**Naming rationale**: "remove" is preferred over "delete" because the default behavior only untracks the config—it does not delete the actual file from disk. This aligns with user expectations and reduces fear of data loss.

## Architecture

### Component Structure

```
cmd/zerb/
  └── config_remove.go         # CLI command layer (flag parsing, confirmation, output)

internal/service/
  └── config_remove.go         # Business logic layer (orchestration)

internal/chezmoi/
  └── chezmoi.go               # Extend Chezmoi interface with Remove method

internal/transaction/
  └── (existing)               # Reuse existing transaction infrastructure

internal/config/
  └── (existing)               # Reuse parser, generator, path validation
```

### Data Flow

```
User Command
    ↓
CLI Layer (config_remove.go)
    ↓ (parse flags, validate paths, show confirmation)
Service Layer (config_remove.go)
    ↓ (orchestrate, use transaction for safety)
    ├→ Acquire lock              [transaction.Lock]
    ├→ Validate paths            [config.NormalizeConfigPath]
    ├→ Read current config       [config.Parser]
    ├→ Verify paths exist        [lookup in Configs array]
    ├→ Remove from chezmoi       [chezmoi.Chezmoi.Remove]
    ├→ Update zerb.lua          [config.Generator]
    ├→ Create git commit        [git.Git]
    └→ Release lock
    ↓
User Output
```

## Key Design Decisions

### 1. Keep File by Default

**Decision**: By default, `zerb config remove` only removes the config from ZERB tracking. The source file on disk remains untouched.

**Rationale**:
- **Safety first**: Users may want to stop tracking a file without deleting it
- **Least surprise**: Untracking is different from deletion
- **Reversible**: User can re-add the file if needed
- **Explicit deletion**: Require `--purge` to delete source file

**Flags**:
- `--keep-file` (default behavior, can be omitted)
- `--purge` - Also delete the source file from disk

**Alternative Considered**: Interactive prompt asking whether to keep or delete
- Rejected: Adds complexity, better to have explicit flags

### 2. Confirmation Prompt

**Decision**: Require confirmation for all remove operations by default.

**Rationale**:
- Removal modifies git history going forward
- Users should consciously acknowledge the operation
- Consistent with other CLI tools (git, rm -i)

**Implementation**:
```
The following configs will be removed from ZERB tracking:
  - ~/.zshrc (synced)
  - ~/.gitconfig (missing)

Source files on disk will NOT be deleted (use --purge to also delete).

Proceed? [y/N]: 
```

**Flags**:
- `--yes` / `-y` - Skip confirmation (for scripts)
- `--dry-run` / `-n` - Preview only, no changes

### 3. Transaction Safety

**Decision**: Use generalized `ConfigTxn` type with operation field (updated per CR-1).

**Rationale**:
- Proven pattern that handles interruptions gracefully
- Provides atomic operations (all-or-nothing)
- Enables `--resume` and `--abort` for interrupted operations
- Consistent UX across config management commands
- **Generalized type enables reuse across add/delete/future operations**

**Transaction Type**:
```go
type ConfigTxn struct {
    ID        string      `json:"id"`
    Operation string      `json:"operation"` // "add" | "delete"
    Paths     []PathState `json:"paths"`
    StartedAt time.Time   `json:"started_at"`
}
```

**Transaction States**:
- `pending` - Path queued for removal
- `in-progress` - Currently removing from chezmoi
- `completed` - Successfully removed
- `failed` - Removal failed (can retry with `--resume`)

**Lock File**: Uses `config.lock` (shared across all config operations per HR-1)

### 4. Chezmoi Interface Extension

**Decision**: Add `Remove(ctx, path)` method to the `Chezmoi` interface with graceful not-found handling (per HR-3).

**Interface Extension**:
```go
type Chezmoi interface {
    Add(ctx context.Context, path string, opts AddOptions) error
    HasFile(ctx context.Context, path string) (bool, error)
    Remove(ctx context.Context, path string) error  // NEW
}
```

**Implementation**:
```go
// Remove removes a path from chezmoi's managed state.
// Returns nil (not an error) if the chezmoi source file doesn't exist.
func (c *Client) Remove(ctx context.Context, path string) error {
    // 1. Normalize path using config.NormalizeConfigPath
    // 2. Map to chezmoi source-relative path
    // 3. Invoke chezmoi forget (removes from source state)
    // 4. If not found, log warning and return nil (cleanup scenario)
    // 5. Wrap other errors with RedactedError
}
```

**Chezmoi Command Used**: `chezmoi forget <path>`
- Removes the source file from chezmoi's source directory
- Does NOT remove the target file from disk
- This matches our "keep file by default" behavior

**For `--purge` flag** (updated per CR-2):
- Delete the source file FIRST using `os.Remove` (before chezmoi.Remove)
- Then call `chezmoi forget` to update tracking
- This order ensures file deletion takes priority in failure scenarios
- Verify path is within $HOME before deletion (HR-5)

### 5. Config File Update Strategy

**Decision**: Create new timestamped config version with path removed.

**Workflow**:
1. Parse current active config
2. Filter out the deleted path(s) from `Configs` array
3. Generate new timestamped config using existing `Generator`
4. Update `.zerb-active` marker
5. Update `zerb.active.lua` symlink
6. Stage and commit all changes

**Rationale**:
- Maintains immutability of timestamped configs
- Full git history preserved
- Easy to see when configs were removed

### 6. Path Validation

**Decision**: Reuse existing `config.NormalizeConfigPath` and validate against current config.

**Validation Steps**:
1. Normalize input path (tilde expansion, canonicalization)
2. Look up path in current config's `Configs` array
3. If not found: error "Config not tracked: <path>"
4. If found: proceed with removal

**No Security Concerns**:
- Unlike `config add`, we're not adding new paths
- We only operate on paths already in the config
- Still use normalized comparison to handle tilde vs absolute

### 7. Multiple Path Removal

**Decision**: Support removing multiple paths in a single command with atomic commit and path deduplication (per HR-4).

**Behavior**:
```bash
zerb config remove ~/.zshrc ~/.gitconfig
```

- **Deduplicate paths** before processing (e.g., `~/.zshrc` and `/home/user/.zshrc` resolve to same path)
- All paths validated before any removal
- All removals performed as single transaction
- Single git commit for all removals
- If any path fails, transaction can be resumed/aborted

**Commit Message**:
- Single path: "Remove ~/.zshrc from tracked configs"
- Multiple paths: "Remove 3 configs from tracked configs" with body listing paths

### 8. Status Display in Confirmation

**Decision**: Show current status of each config in confirmation prompt.

**Rationale**:
- Helps user understand what they're removing
- Highlights if config is already missing (cleanup scenario)
- Consistent with `config list` output

**Display**:
```
The following configs will be removed from ZERB tracking:
  - ~/.zshrc (synced)
  - ~/.gitconfig (missing)
  - ~/.tmux.conf (partial)
```

### 9. Error Handling

**Decision**: Fail fast on validation, continue on removal errors with transaction.

**Validation Errors** (fail immediately):
- Path not tracked by ZERB
- ZERB not initialized
- Lock acquisition failed

**Removal Errors** (continue with transaction):
- Chezmoi forget fails for one path
- Mark as failed in transaction
- Continue with other paths
- Report summary at end
- Enable `--resume` to retry

### 10. Git Commit Content

**Decision**: Commit includes removed config version and chezmoi source changes.

**Files Staged**:
- `configs/<new-timestamped-config>.lua` (new version without removed paths)
- `.zerb-active` (updated marker)
- `zerb.active.lua` (updated symlink)
- `chezmoi/source/` (removed files)

**Commit Message Format**:
- Single: "Remove ~/.zshrc from tracked configs"
- Multiple: "Remove N configs from tracked configs"
- Body lists all removed paths

## Testing Strategy

### Unit Tests

**CLI Layer** (`cmd/zerb/config_remove_test.go`):
- Flag parsing (`--yes`, `--dry-run`, `--purge`, `--keep-file`)
- Confirmation prompt behavior
- Output formatting
- Help text
- Target: >80% coverage

**Service Layer** (`internal/service/config_remove_test.go`):
- Mock dependencies (Chezmoi, Git, Parser, Generator)
- Path validation (tracked vs not tracked)
- Single and multiple path removal
- Transaction state management
- Error handling scenarios
- Context cancellation
- Target: >80% coverage

**Chezmoi Layer** (`internal/chezmoi/chezmoi_test.go`):
- `Remove` method with mock binary
- Error redaction
- Context support
- Target: >80% coverage

### Integration Tests

- Create ZERB environment with multiple configs
- Remove single config, verify removal
- Remove multiple configs, verify atomic commit
- Test `--dry-run` mode
- Test `--purge` vs `--keep-file`
- Test transaction resume/abort
- Test concurrent operation prevention

### Test Coverage Goal

>80% coverage for all new code:
- `cmd/zerb/config_remove.go`: >80%
- `internal/service/config_remove.go`: >80%
- Chezmoi Remove method: >80%

## Performance Considerations

### Expected Performance

**Typical Use Case** (1-5 configs):
- Path validation: <5ms
- Chezmoi forget (per file): <50ms
- Config generation: <10ms
- Git commit: <100ms
- **Total**: <300ms

**Large Use Case** (10+ configs):
- Path validation: <10ms
- Chezmoi forget (10 files): ~500ms
- Config generation: <10ms
- Git commit: <200ms
- **Total**: <1 second

## Security Considerations

### Path Validation

- Reuse existing `validateConfigPath` security measures
- Only operate on paths already in config (no new path attacks)
- Normalize paths for consistent comparison

### File Deletion (`--purge`)

- **Verify path is within $HOME before deletion** (HR-5 safety check)
- Delete source file BEFORE chezmoi.Remove (CR-2 order)
- Only delete files that are tracked in config
- Handle file not found gracefully (not an error)
- Clear confirmation prompt when `--purge` is used

### Information Disclosure

- Use RedactedError for all chezmoi operations
- Never expose chezmoi source paths in user messages
- Sanitize error messages

## Subagent Review Recommendations

The following recommendations were collected from @golang-pro and @architect-reviewer reviews:

### Critical Priority

#### CR-1: Generalize Transaction Type
**From**: @architect-reviewer  
**Current**: Plan to reuse `ConfigAddTxn` from config add command  
**Recommendation**: Create a generalized `ConfigTxn` type with an operation field

```go
type ConfigTxn struct {
    ID        string           `json:"id"`
    Operation string           `json:"operation"` // "add" | "delete"
    Paths     []PathState      `json:"paths"`
    StartedAt time.Time        `json:"started_at"`
    // ...
}
```

**Rationale**: Prevents code duplication and enables future config operations (modify, rename) to use the same infrastructure.

**Impact**: Modify `internal/transaction/transaction.go` to use generalized type.

#### CR-2: Reverse File Deletion Order
**From**: @architect-reviewer  
**Current**: Design shows chezmoi.Remove → then file deletion  
**Recommendation**: Delete source file BEFORE chezmoi.Remove when using `--remove-file`

**Rationale**: 
- If chezmoi.Remove fails after file deletion, we've only lost tracking (recoverable)
- If file deletion fails after chezmoi.Remove, we've lost tracking but file remains (orphaned state)
- Safer to prioritize data preservation in failure scenarios

**Implementation**:
```go
if opts.RemoveFile {
    if err := os.Remove(normalizedPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to delete source file: %w", err)
    }
}
if err := chezmoi.Remove(ctx, path); err != nil {
    // Log warning but continue - file is already gone
}
```

### High Priority

#### HR-1: Generalize Lock File Name
**From**: @golang-pro  
**Current**: Using `config-add.lock` hardcoded name  
**Recommendation**: Use `config.lock` for all config operations

**Rationale**: A single lock for all config operations prevents races between add/delete/list operations that modify state.

**Implementation**: Update `internal/transaction/lock.go` to use `config.lock`.

#### HR-2: Add Context to AcquireLock
**From**: @golang-pro  
**Current**: `AcquireLock(path string) error`  
**Recommendation**: `AcquireLock(ctx context.Context, path string) error`

**Rationale**: Enables timeout and cancellation support for lock acquisition.

#### HR-3: Handle Missing Chezmoi Source Gracefully
**From**: @golang-pro, @architect-reviewer  
**Current**: Q3 in Open Questions suggests "warn and continue"  
**Decision**: Implement Option 2 - warn and continue with zerb.lua update

**Implementation**:
```go
func (c *Client) Remove(ctx context.Context, path string) error {
    err := c.runCommand(ctx, "forget", path)
    if isNotFoundError(err) {
        c.logger.Warn("chezmoi source not found, continuing with config removal", "path", path)
        return nil // Not an error for delete operation
    }
    return err
}
```

#### HR-4: Add Path Deduplication
**From**: @golang-pro  
**Current**: No handling for duplicate arguments  
**Recommendation**: Deduplicate paths before processing

**Implementation**:
```go
func deduplicatePaths(paths []string) []string {
    seen := make(map[string]struct{})
    result := make([]string, 0, len(paths))
    for _, p := range paths {
        normalized := config.NormalizeConfigPath(p)
        if _, ok := seen[normalized]; !ok {
            seen[normalized] = struct{}{}
            result = append(result, p)
        }
    }
    return result
}
```

#### HR-5: Safety Checks for --remove-file
**From**: @golang-pro  
**Current**: Relies on path validation  
**Recommendation**: Add explicit safety check that path is within $HOME

**Implementation**:
```go
func isWithinHome(path string) bool {
    home, err := os.UserHomeDir()
    if err != nil {
        return false
    }
    absPath, err := filepath.Abs(path)
    if err != nil {
        return false
    }
    return strings.HasPrefix(absPath, home)
}
```

### Medium Priority

#### MR-1: Add Config Helper Methods
**From**: @architect-reviewer  
**Recommendation**: Add `FindConfig` and `RemoveConfig` helpers to `config.Config`

```go
// FindConfig returns the ConfigFile matching the given path, or nil if not found
func (c *Config) FindConfig(path string) *ConfigFile {
    normalized := NormalizeConfigPath(path)
    for i := range c.Configs {
        if NormalizeConfigPath(c.Configs[i].Path) == normalized {
            return &c.Configs[i]
        }
    }
    return nil
}

// RemoveConfig returns a new Configs slice with the path removed
func (c *Config) RemoveConfig(path string) []ConfigFile {
    normalized := NormalizeConfigPath(path)
    result := make([]ConfigFile, 0, len(c.Configs))
    for _, cfg := range c.Configs {
        if NormalizeConfigPath(cfg.Path) != normalized {
            result = append(result, cfg)
        }
    }
    return result
}
```

#### MR-2: Keep Confirmation Prompts in CLI Layer
**From**: @architect-reviewer  
**Current**: Design already shows this correctly  
**Confirmation**: Confirmation logic should remain in `cmd/zerb/config_delete.go`, not in service layer

### Low Priority

#### LR-1: Consider Future Symlink Support
**From**: @architect-reviewer  
**Note**: Current design doesn't explicitly handle symlinks  
**Recommendation**: Document that symlinks are resolved before deletion

## Open Questions

### Q1: Should we support glob patterns?

**Options**:
1. No glob support (explicit paths only)
2. Support `~/.config/*` style patterns
3. Support with `--glob` flag

**Recommendation**: Option 1 for MVP. Glob patterns add complexity and edge cases.

### Q2: Should `--purge` require additional confirmation?

**Options**:
1. Single confirmation covers both tracking removal and file deletion
2. Separate confirmation: "Also delete source files? [y/N]"
3. Always require `--yes` with `--purge`

**Recommendation**: Option 1. The confirmation prompt clearly states file deletion behavior.

### Q3: What if chezmoi source file doesn't exist? [RESOLVED]

**Scenario**: Config is in zerb.lua but chezmoi source was manually deleted.

**Decision**: Option 2 - Warn and continue with zerb.lua update only (per HR-3)

This is a valid cleanup scenario. The Remove method returns nil (not an error) when the source file doesn't exist, allowing the config entry to be removed from zerb.lua.

## References

- **Existing Specs**: `openspec/specs/config-management/spec.md`
- **Related Commands**: `zerb config add`, `zerb config list`
- **Transaction System**: `internal/transaction/`
- **Chezmoi Docs**: `chezmoi forget` command