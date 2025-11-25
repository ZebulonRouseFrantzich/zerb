# Change: Add `zerb config remove` Command

## Why

Users need the ability to remove configuration files from ZERB tracking. After adding configs with `zerb config add`, there's currently no way to untrack a config short of manually editing `zerb.lua` and cleaning up chezmoi's source directory. The `zerb config remove` command completes the config management lifecycle (`add` -> `list` -> `remove`), enabling users to cleanly remove configs they no longer want ZERB to manage.

**Naming rationale**: "remove" is preferred over "delete" because the default behavior only untracks the config—it does not delete the actual file from disk. This aligns with user expectations and reduces fear of data loss.

**Common use cases:**
- Removing a config that's no longer needed (e.g., switched to a different shell)
- Cleaning up experimental configs that were added for testing
- Untracking a sensitive config that shouldn't be version-controlled
- Correcting a mistake when the wrong path was added

## What Changes

**Core Functionality:**
- Add new `zerb config remove` CLI command with support for:
  - Single path removal (`zerb config remove ~/.zshrc`)
  - Multiple paths in one command (`zerb config remove ~/.zshrc ~/.gitconfig`)
  - `--keep-file` flag to retain the actual file on disk (default: keep file)
  - `--purge` flag to also delete the source file from disk
  - `--yes` flag to skip confirmation prompt
  - `--dry-run` flag to preview what would be removed
- Remove config entry from `zerb.lua` (create new timestamped version)
- Remove file from chezmoi's source directory (managed state)
- Generate appropriate git commit messages
- Provide clear user feedback during the remove process

**Safety Features:**
- **Confirmation prompt by default** for operations that modify tracking
- **Dry-run mode** to preview changes before applying
- **Keep file by default** - only removes from ZERB tracking, not disk
- **Explicit flag required** (`--purge`) to delete source file from disk
- **Path validation** reuses existing security validation

**Display Format (Confirmation):**
```
The following configs will be removed from ZERB tracking:
  - ~/.zshrc (synced)
  - ~/.gitconfig (synced)

Source files on disk will NOT be deleted (use --purge to also delete).

Proceed? [y/N]: 
```

**Display Format (Success):**
```
Removed from ZERB tracking:
  - ~/.zshrc
  - ~/.gitconfig

Committed: abc1234
Config version: zerb.lua.20251125T143022Z
```

## Impact

**Affected specs:**
- `config-management` (add remove capability)

**Affected code:**
- `cmd/zerb/main.go` - Add routing for `config remove` subcommand
- `cmd/zerb/config_remove.go` (new) - Command implementation
- `internal/service/config_remove.go` (new) - Service layer for remove operations
- `internal/chezmoi/chezmoi.go` - Extend `Chezmoi` interface with `Remove(ctx, path)` method
- `internal/transaction/` - Reuse existing transaction infrastructure

**User Impact:**
- **Completeness**: Users can now manage the full config lifecycle (add, list, remove)
- **Safety**: Confirmation prompts and dry-run prevent accidental removals
- **Clarity**: Keep-file-by-default ensures users don't accidentally delete source files
- **User Abstraction**: Never exposes internal implementation (chezmoi) to users
- **Git History**: Removals are tracked with meaningful commit messages

**Quality Impact:**
- Follows same interface-based design patterns as `config add` and `config list`
- Test-driven development with >80% coverage goal
- Context support for cancellation and timeouts
- Clear separation of concerns (command -> service -> data access)
- User-facing abstraction maintained (no "chezmoi" in any messages)
- Reuses existing transaction infrastructure for safety

## Dependencies

**Required implementations:**
- Existing config parsing (`internal/config.Parser`)
- Existing chezmoi integration (`internal/chezmoi.Client`)
- Existing git integration (`internal/git.Client`)
- Existing transaction infrastructure (`internal/transaction`)
- Existing path validation (`internal/config.NormalizeConfigPath`)

**Enables future work:**
- `zerb config edit` command (modify tracked configs)
- `zerb config move` command (rename/relocate tracked configs)
- Interactive config management (TUI/fuzzy selection)
- Batch operations across multiple machines

## Out of Scope

Explicitly deferred to future changes:
- Interactive selection with fuzzy finder (TUI) - post-MVP feature
- Bulk removal with glob patterns (`zerb config remove ~/.config/*`) - separate feature
- Recursive directory untracking logic - follow chezmoi semantics
- Undoing a remove operation - use git revert
- Force removal without confirmation in non-interactive mode - security concern
