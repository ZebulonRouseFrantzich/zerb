# Change: Setup Git Repository Infrastructure

## Why

The git operations component (07-git-operations.md) requires a git repository to exist before implementing version control features like commit generation, pre-commit hooks, sync operations, and drift tracking. Currently, `zerb init` creates the directory structure and initial config files but does not initialize a git repository. This change establishes the git repository as part of the initialization process, providing the foundation for all git-based versioning features.

## What Changes

- Add git repository initialization during `zerb init` using go-git library
- Configure repository-local git settings (user.name, user.email with environment variable fallbacks)
- Create initial commit with both `.gitignore` and timestamped config
- Set up `.gitignore` to exclude runtime files (bin/, cache/, logs/, etc.) and generated configs
- Track only configs/ and chezmoi/source/ directories per architecture decision
- Handle git initialization errors gracefully with clear user guidance
- Create ZERB directory with 0700 permissions for security
- Add persistent warning on `zerb activate` when git is not initialized

**Explicitly out of scope (deferred to future change):**
- Remote repository configuration (`git.remote` in config)
- `--remote` and `--from` flags for `zerb init`
- Smart detection and cloning of existing ZERB repos
- Pre-commit hooks for validation and secret detection
- See `openspec/future-proposal-information/git-remote-setup.md` for planned remote setup approach
- See `openspec/future-proposal-information/pre-commit-hooks.md` for planned hook implementation

## Impact

- **Affected specs**: initialization (new capability)
- **Affected code**: 
  - `cmd/zerb/init.go` - Add git initialization step (reordered sequence)
  - `internal/git/git.go` - Migrate to go-git library, add repository initialization methods
  - New `.gitignore` template in `internal/git`
  - `cmd/zerb/activate.go` - Add warning for git-unavailable state
- **Dependencies**: 
  - Add `github.com/go-git/go-git/v5` to go.mod
  - Enables future implementation of git operations component (07-git-operations.md)
  - Enables future pre-commit hooks (see `openspec/future-proposal-information/pre-commit-hooks.md`)
- **Future work**: 
  - Remote repository setup deferred to separate change (see `openspec/future-proposal-information/git-remote-setup.md`)
  - Pre-commit hooks deferred to separate change (see `openspec/future-proposal-information/pre-commit-hooks.md`)
- **User experience**: Adds git initialization step to `zerb init` with minimal user interaction
- **Security**: ZERB directory created with 0700 permissions to protect git history and config files
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
