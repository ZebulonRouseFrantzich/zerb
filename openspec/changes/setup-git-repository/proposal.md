# Change: Setup Git Repository Infrastructure

## Why

The git operations component (07-git-operations.md) requires a git repository to exist before implementing version control features like commit generation, pre-commit hooks, sync operations, and drift tracking. Currently, `zerb init` creates the directory structure and initial config files but does not initialize a git repository. This change establishes the git repository as part of the initialization process, providing the foundation for all git-based versioning features.

## What Changes

- Add git repository initialization during `zerb init`
- Configure initial git settings (user.name, user.email with fallbacks)
- Create initial commit with timestamped config and directory structure
- Set up `.gitignore` to exclude runtime files (bin/, cache/, logs/, etc.)
- Track only configs/ and chezmoi/source/ directories per architecture decision
- Handle git initialization errors gracefully with clear user guidance

**Explicitly out of scope (deferred to future change):**
- Remote repository configuration (`git.remote` in config)
- `--remote` and `--from` flags for `zerb init`
- Smart detection and cloning of existing ZERB repos
- See `openspec/future-proposal-information/git-remote-setup.md` for planned remote setup approach

## Impact

- **Affected specs**: initialization (new capability)
- **Affected code**: 
  - `cmd/zerb/init.go` - Add git initialization step
  - `internal/git/git.go` - Add repository initialization methods
  - New `.gitignore` template for ZERB directories
- **Dependencies**: Enables future implementation of git operations component (07-git-operations.md)
- **Future work**: Remote repository setup deferred to separate change (see `openspec/future-proposal-information/git-remote-setup.md`)
- **User experience**: Adds one additional step to `zerb init` with minimal user interaction
- **Breaking changes**: None - this is additive functionality

## Architecture Decisions Reference

From `.ai-workflow/implementation-planning/components/07-git-operations.md`:
- Git repository lives at `~/.config/zerb/.git/`
- Symlink `zerb.lua.active` NOT committed (local-only, recreated after pull/clone)
- Files committed: `configs/zerb.lua.TIMESTAMP` (single source of truth), `chezmoi/source/*` (user's actual dotfiles)
- Files excluded: 
  - Generated configs: `mise/config.toml`, `chezmoi/config.toml` (derived from zerb.lua)
  - Runtime data: `bin/`, `cache/`, `tmp/`, `logs/`, `mise/`
  - Transaction state: `.txn/`
  - Symlinks: `zerb.lua.active`, `.zerb-active`
  - Embedded/extracted: `keyrings/` (extracted from binary at init)
