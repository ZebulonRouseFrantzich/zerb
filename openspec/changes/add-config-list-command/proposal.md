# Change: Add `zerb config list` Command

## Why

Users need visibility into which configuration files and directories are currently tracked by ZERB. After adding configs with `zerb config add`, there's no easy way to see what's being managed without manually inspecting the `zerb.lua` file. The `zerb config list` command provides a clear, formatted view of tracked configurations with their flags and sync status, enabling users to audit and understand their configuration management state.

## What Changes

**Core Functionality:**
- Add new `zerb config list` CLI command with support for:
  - List configs from active timestamped baseline (default behavior)
  - Table format with columns: Status, Path, Flags
  - Alphabetical sorting by path for easy scanning
  - Optional `--verbose` flag for detailed information (size, last modified)
  
**Status Indicators** (using "managed by ZERB" terminology):
- **✓ Synced**: Config is declared in zerb.lua and managed by ZERB
- **✗ Missing**: Config is declared but source file no longer exists
- **? Partial**: Config is declared but not yet managed by ZERB (tracking incomplete)

**Note**: MVP focuses on table output only. JSON/plain output formats and `--all` flag for historical configs are deferred to future iterations. Drift detection (comparing file hashes) is also deferred.

**Display Format (Default):**
```
Active configuration (zerb.lua.20250116T143022Z):

STATUS  PATH                    FLAGS
✓       ~/.gitconfig            template
✓       ~/.zshrc                
✗       ~/.tmux.conf            private
?       ~/.ssh/config           private, secrets

4 configs tracked (2 synced, 1 missing, 1 partial)
```

**Verbose Output** (`--verbose`):
```
Active configuration (zerb.lua.20250116T143022Z):

STATUS  PATH                    FLAGS                 SIZE     LAST MODIFIED
✓       ~/.gitconfig            template              2.1 KB   2 hours ago
✓       ~/.zshrc                                      8.4 KB   3 days ago
✗       ~/.tmux.conf            private               -        (file not found)
?       ~/.ssh/config           private, secrets      -        (not managed)

4 configs tracked (2 synced, 1 missing, 1 partial)

Notes:
  - Missing files may need to be restored or removed from config
  - Partial tracking indicates incomplete 'zerb config add' operation
```

## Impact

**Affected specs:**
- `config-management` (add list capability)

**Affected code:**
- `cmd/zerb/main.go` - Update routing to support `config list` subcommand (remove "coming soon" message)
- `cmd/zerb/config_list.go` (new) - Command implementation
- `internal/service/config_list.go` (new) - Service layer for list operations
- `internal/config/status.go` (new) - Status detection logic and `StatusDetector` interface
- `internal/chezmoi/chezmoi.go` - Extend `Chezmoi` interface with `HasFile(ctx, path)` query method

**User Impact:**
- **Visibility**: Users can quickly see what configs are tracked and their status
- **Troubleshooting**: Status indicators help identify configuration issues (missing files, incomplete tracking)
- **Auditing**: Easy verification that all intended configs are being managed
- **User Abstraction**: Never exposes internal implementation (chezmoi) to users
- **UX**: Completes the config management workflow: `add` → `list` → (future: `remove`)

**Quality Impact:**
- Follows same interface-based design patterns as `config add`
- Test-driven development with >80% coverage goal
- Context support for cancellation and timeouts
- Clear separation of concerns (command → service → data access)
- User-facing abstraction maintained (no "chezmoi" in any messages)

## Dependencies

**Required implementations:**
- Existing config parsing (`internal/config.Parser`)
- Existing chezmoi integration (`internal/chezmoi.Client`)
- File system access for status detection

**Enables future work:**
- `zerb config remove` command (will need similar listing logic)
- `zerb config edit` command (show before/after via list)
- `zerb config sync` command (show what would be synced)
- Dashboard/TUI showing configuration state
- Machine-specific overrides display (future profile feature)
- JSON/plain output formats (deferred from MVP)
- Historical config listing with `--all` flag (deferred from MVP)
- Drift detection with file hash comparison (deferred from MVP)

## Out of Scope

Explicitly deferred to future changes:
- `zerb config remove` command - separate change proposal needed
- `zerb config edit` command - separate change proposal needed
- Config diffing (showing what changed between versions) - separate feature
- Interactive selection/filtering (TUI/fuzzy finder) - post-MVP feature
- Showing full template output - separate feature
- Showing which machine profiles override configs - deferred to machine-overrides feature (Component 08)
- Drift detection (file hash comparison) - deferred to future iteration
- JSON output format (`--json`) - deferred to future iteration
- Plain output format (`--plain`) - deferred to future iteration
- Historical config listing (`--all` flag) - deferred to future iteration
