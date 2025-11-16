# Design: Git Repository Setup

## Context

ZERB uses git for version control of configuration files, enabling sync across machines, history tracking, and rollback capabilities. Before implementing advanced git operations (commits, sync, hooks, drift detection), we need to establish a git repository during initialization.

**Constraints:**
- Must work offline (no remote required initially)
- Must handle missing git binary gracefully
- Must not interfere with existing user git configurations
- Must follow ZERB isolation principles
- Must align with architecture decisions from 07-git-operations.md

**Stakeholders:**
- Users initializing ZERB for the first time
- Future git operations features (commits, sync, hooks)
- Drift detection system (relies on git history)

## Goals / Non-Goals

**Goals:**
- Initialize git repository during `zerb init`
- Create .gitignore to exclude runtime files per architecture
- Make initial commit with timestamped config
- Configure git user info with sensible fallbacks
- Handle errors gracefully with clear user guidance

**Non-Goals:**
- Remote repository setup (post-MVP)
- Git SSH key configuration
- Pre-commit hook installation (separate change)
- Git LFS setup
- Interactive git configuration wizard

## Decisions

### Decision 1: When to Initialize Git

**Choice:** Initialize during `zerb init`, after directory structure but before shell integration.

**Rationale:**
- Natural fit: config versioning is core to ZERB's value proposition
- Early initialization: enables all subsequent operations to assume git exists
- Order matters: need directories first, then .gitignore, then git init, then initial config, then commit

**Sequence:**
1. Create directory structure
2. Write .gitignore file
3. Run `git init`
4. Configure git (user.name, user.email)
5. Generate initial config
6. Create initial commit

**Alternatives considered:**
- Lazy initialization (first commit): Adds complexity, delays benefits
- Separate `zerb git init` command: Extra step, not discoverable
- Post-shell-integration: Doesn't affect decision, current placement is logical

### Decision 2: Git User Configuration Strategy

**Choice:** Three-tier fallback system

1. Try system git config (`git config --global user.name/email`)
2. Try environment variables (`GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`)
3. Use placeholder values (`ZERB User`, `zerb@localhost`)

**Rationale:**
- Most users have git configured globally (best case)
- Environment variables provide override capability (CI/CD, testing)
- Placeholder ensures init always succeeds (can fix later)
- Warning message guides users to fix placeholder values

**Implementation:**
```go
func detectGitUser() (name, email string) {
    // Try system git config
    if name = execGit("config", "--global", "user.name"); name != "" {
        email = execGit("config", "--global", "user.email")
        return
    }
    
    // Try environment variables
    if name = os.Getenv("GIT_AUTHOR_NAME"); name != "" {
        email = os.Getenv("GIT_AUTHOR_EMAIL")
        return
    }
    
    // Fallback
    return "ZERB User", "zerb@localhost"
}
```

**Alternatives considered:**
- Prompt user for name/email: Breaks non-interactive init, slows UX
- Require git config: Too strict, fails unnecessarily
- Use system username/hostname: Privacy concerns, not meaningful for commits

### Decision 3: .gitignore Pattern Strategy

**Choice:** Embedded template with explicit inclusions and exclusions

**Included (tracked by git):**
- `configs/` - All timestamped configuration snapshots (source of truth)
- `chezmoi/source/` - User's actual dotfiles (user content)

**Excluded (not tracked):**
- **Generated configs:** `mise/config.toml`, `chezmoi/config.toml` - Generated from `zerb.lua`
- **Runtime artifacts:** `bin/`, `cache/`, `tmp/`, `logs/` - Downloaded/temporary files
- **Tool state:** `mise/` - mise's internal data (MISE_DATA_DIR)
- **Transaction state:** `.txn/` - Transaction files (ephemeral)
- **Development env:** `.direnv/` - Nix development environment cache
- **Derived state:** `zerb.lua.active` - Symlink (recreated locally after pull/clone)
- **Deprecated:** `.zerb-active` - Old marker file (to be removed)
- **Embedded/extracted:** `keyrings/` - Extracted from binary at runtime (identical on all machines)

**Key Principle: Track Source, Exclude Derived**

ZERB follows a strict separation:
- **Track:** User-created content and source of truth (`zerb.lua`, dotfiles)
- **Exclude:** Generated/derived state (configs from `zerb.lua`, extracted keyrings)

**Rationale:**

1. **Generated configs are redundant:**
   - `mise/config.toml` is a deterministic transformation of `zerb.lua` tools section
   - `chezmoi/config.toml` is generated from `zerb.lua` data section
   - Both can be perfectly regenerated from tracked `zerb.lua`
   - Tracking them = data duplication with zero benefit

2. **Abstraction principle:**
   - Users configure everything in `zerb.lua` (never mention "mise" or "chezmoi")
   - ZERB translates `zerb.lua` → tool-specific configs internally
   - Example: `data = {email = "user@example.com"}` in `zerb.lua` → `[data]` in `chezmoi/config.toml`

3. **Keyrings are reproducible:**
   - Embedded in ZERB binary at compile time
   - Extracted to `keyrings/` during `zerb init` via `EnsureKeyrings()`
   - Identical on all machines (not user-specific)
   - No benefit to tracking in git

4. **Symlink not committed:**
   - Cross-platform compatibility (Windows vs Unix)
   - Cleaner git diffs (symlinks show as full content)
   - Simpler sync logic (recreated after pull/clone)

5. **Configs are immutable:**
   - Timestamped files never modified
   - No merge conflicts on config snapshots

6. **Runtime files pollute git:**
   - Large binaries, frequently changing caches
   - No value in version control

**Template location:** Embedded in Go code as string constant (avoid file I/O dependency)

**Alternatives considered:**
- Track generated configs: Creates data duplication, confusing diffs
- Dynamic generation: Overkill for static patterns
- User-editable .gitignore: Increases support burden, pattern mistakes
- No .gitignore: Runtime files pollute repo, bad UX

### Decision 4: Initial Commit Content

**Choice:** Commit timestamped config file only

**Rationale:**
- Establishes git history from the start
- Includes meaningful content (not empty commit)
- Directory structure doesn't need to be tracked (recreated by init)
- Keeps commit focused and minimal

**Commit message:** `"Initialize ZERB environment"`

**Files in initial commit:**
- `configs/zerb.lua.YYYYMMDDTHHMMSS.sssZ` (initial empty config)
- `.gitignore` (ignored patterns)

**Alternatives considered:**
- Empty commit: Less meaningful, doesn't test git operations
- Include all directories: Unnecessary, git doesn't track empty directories
- Include keyrings: Security concern (even if public keys)

### Decision 5: Error Handling for Missing Git

**Choice:** Warn and continue (graceful degradation)

**Behavior:**
- Check if `git` binary exists before initialization
- If not found: print warning, skip git initialization, continue init
- User can manually run `git init` later if desired
- All other ZERB features work without git (drift detection uses current state)

**Warning message:**
```
⚠ Warning: git binary not found on PATH
  Git repository not initialized.
  
  Install git and run:
    cd ~/.config/zerb
    git init
    git add configs/ .gitignore
    git commit -m "Initialize ZERB environment"
```

**Rationale:**
- Don't block init: ZERB has value without git versioning
- Clear guidance: user knows exactly how to fix
- Aligns with isolation: ZERB doesn't require specific system tools

**Alternatives considered:**
- Fail init: Too strict, reduces ZERB adoption
- Silent skip: Confusing, user doesn't know why git features fail later
- Download git: Outside scope, violates isolation principles

### Decision 6: Repository Location

**Choice:** `~/.config/zerb/.git/` (repository root is ZERB directory)

**Rationale:**
- All ZERB-managed files in one place
- Consistent with `configs/`, `bin/`, etc. structure
- Matches architecture decision from 07-git-operations.md

**Alternatives considered:**
- Separate `~/.config/zerb-git/`: Unnecessary complexity
- Inside `configs/`: Violates separation of concerns

## Risks / Trade-offs

### Risk: Placeholder git user values

**Impact:** Commits have non-meaningful author info

**Mitigation:**
- Warn user during init if placeholders used
- Provide clear instructions to fix: `git config --global user.name "Real Name"`
- Low severity: can be fixed retroactively with `git rebase` if needed

### Risk: Git not installed

**Impact:** Git features unavailable, but ZERB still functional

**Mitigation:**
- Graceful degradation with clear warning
- Instructions to fix and manually initialize
- Document git as recommended dependency

### Risk: Insufficient test coverage

**Impact:** Bugs in git initialization could corrupt user repositories

**Mitigation:**
- Strict TDD methodology required (RED → GREEN → REFACTOR)
- Minimum 80% test coverage enforced
- Unit tests for all git operations methods
- Integration tests for end-to-end `zerb init` workflow
- Error path testing (missing git, invalid repo, config failures)
- Test with real git commands to verify go-git operations

### Trade-off: Auto-init vs manual git setup

**Trade-off:** Automatic initialization vs explicit user control

**Decision:** Auto-init with graceful degradation

**Reasoning:**
- Better UX: works out of the box for most users
- Opt-out available: user can delete .git if unwanted
- Clear warnings: user always knows what's happening

## Migration Plan

N/A - This is new functionality, not migrating existing behavior.

**Post-implementation:**
- Users who already ran `zerb init` will NOT have git repository
- Provide documentation for manual git setup in existing installations
- Consider future `zerb repair` command to add git to existing installs

## Open Questions

### Q1: Should we support git init flags (e.g., --initial-branch=main)?

**Decision:** Use git defaults for MVP. If git is configured to use `master`, respect that.

**Reasoning:**
- Minimal complexity: let git's own config control behavior
- Post-MVP: add `zerb config git` commands for customization

### Q2: Should .gitignore be user-editable?

**Decision:** Yes, but with clear documentation.

**Reasoning:**
- `.gitignore` is a standard git file users expect to customize
- Document recommended patterns and risks of removing them
- Future: `zerb doctor` can validate .gitignore correctness

### Q3: What if user wants to use existing git repo?

**Decision:** Skip git init if `.git` already exists.

**Reasoning:**
- Allow advanced users to pre-configure git (e.g., with remotes)
- Verify it's a valid git repo with `git rev-parse --git-dir`
- If invalid, warn and skip git init (don't overwrite)

**Implementation:**
```go
if isGitRepo(zerbDir) {
    fmt.Println("✓ Git repository already exists")
    return nil
}
```

### Q4: Should we validate git version?

**Decision:** No version check for MVP.

**Reasoning:**
- Modern git features (since ~2.0) are widely available
- Avoid arbitrary version requirements
- Future: add version check if specific features require newer git

### Decision 7: Config Generation Strategy

**Choice:** Generate both `mise/config.toml` and `chezmoi/config.toml` from `zerb.lua`

**Rationale:**
- **Single source of truth:** `zerb.lua` contains all user configuration
- **Deterministic transformation:** Config files are pure functions of `zerb.lua` content
- **No manual editing:** Tool configs are always derived, never hand-edited
- **Consistency:** Same pattern for both tools (read `zerb.lua` → generate config)

**Implementation approach:**
1. `mise/config.toml` - Generated from `tools` section (already planned)
   ```lua
   tools = { "node@20", "python@3.11" }
   ```
   → `[tools]\nnode = "20"\npython = "3.11"`

2. `chezmoi/config.toml` - Generated from `data` section (NEW)
   ```lua
   data = { email = "user@example.com", name = "John Doe" }
   ```
   → `[data]\nemail = "user@example.com"\nname = "John Doe"`

**Consequences:**
- Git ignores both generated configs (see Decision 3)
- `zerb activate` regenerates configs from active `zerb.lua`
- After `git pull`, user runs `zerb activate` to update derived state
- Config generation is idempotent (same input → same output)

**Alternatives considered:**
- Track tool configs in git: Creates redundancy, merge conflicts, confusion about source of truth
- Let users manually edit tool configs: Breaks abstraction, defeats purpose of `zerb.lua`
- Generate only mise config: Inconsistent, chezmoi would need manual configuration

**Note:** Config generation implementation is out of scope for this change. This decision documents the architectural relationship between git tracking and config generation.

### Decision 8: Abstraction Principle

**Choice:** Users NEVER see "mise" or "chezmoi" in `zerb.lua` configuration

**Rationale:**
- **Tool independence:** ZERB owns the abstraction, can swap implementations
- **Simplified mental model:** Users think in terms of "tools", "configs", "data", not tool names
- **Reduced cognitive load:** One configuration language, not three (zerb.lua + mise.toml + chezmoi.toml)
- **Future flexibility:** Can replace mise/chezmoi without breaking user configs

**Abstraction mapping:**

| User Concept (in zerb.lua) | ZERB Translation | Tool Implementation |
|----------------------------|------------------|---------------------|
| `tools = {"node@20"}` | Parse tools section | → `mise/config.toml` |
| `configs = {"~/.zshrc"}` | Parse configs section | → `chezmoi add ~/.zshrc` |
| `data = {email = "..."}` | Parse data section | → `chezmoi/config.toml` |

**User-facing terminology:**
- ✅ "tools" - Binary dependencies managed by ZERB
- ✅ "configs" - Configuration files managed by ZERB
- ✅ "data" - Template variables for configuration files
- ❌ "mise tools" - Internal implementation detail
- ❌ "chezmoi dotfiles" - Internal implementation detail

**Examples:**

```lua
-- Good: Abstracted configuration
zerb = {
  tools = {
    "node@20",
    "python@3.11",
  },
  configs = {
    "~/.zshrc",
    "~/.gitconfig",
  },
  data = {
    email = "user@example.com",
    name = "John Doe",
  }
}
```

```lua
-- Bad: Tool names exposed (never do this)
zerb = {
  mise = {
    tools = {"node@20"}
  },
  chezmoi = {
    configs = {"~/.zshrc"}
  }
}
```

**Consequences:**
- ZERB documentation uses abstracted terminology exclusively
- Error messages never mention mise/chezmoi (e.g., "Failed to install tool node@20" not "mise install failed")
- Help text describes features, not underlying tools
- Internal logs may mention tools (with LOG_LEVEL=debug), but never user-facing output

**Alternatives considered:**
- Expose tool names: Leaks implementation, limits future flexibility
- Hybrid approach (some abstracted, some exposed): Confusing, inconsistent
- Complete reimplementation (no mise/chezmoi): Massive scope, reinventing wheels

**Alignment with architecture:**
- Matches 07-git-operations.md: "Users never directly interact with git internals"
- Extends isolation principle to all underlying tools
- Reinforces `zerb.lua` as the single source of truth

---

## Future Work: Remote Repository Setup

This change establishes **local git repository infrastructure only**. Remote repository configuration and sync operations are explicitly deferred to a future change to keep this proposal focused and testable.

**See:** `openspec/future-proposal-information/git-remote-setup.md` for detailed planning.

### Summary of Planned Remote Setup (Post-MVP)

**User Configuration:**

Users will declare remotes in `zerb.lua`:
```lua
zerb = {
  tools = { "node@20" },
  git = {
    remote = "git@github.com:username/dotfiles.git",  -- SSH or HTTPS
    branch = "main",  -- optional, defaults to "main"
  }
}
```

**Initialization Modes:**

1. **Local-only** (current change):
   ```bash
   zerb init  # Creates local repo, no remote
   ```

2. **With remote** (future - first machine):
   ```bash
   zerb init --remote git@github.com:user/dotfiles.git
   # Adds remote, pushes initial commit
   ```

3. **From existing remote** (future - second machine):
   ```bash
   zerb init --from git@github.com:user/dotfiles.git
   # Pulls existing baseline, activates latest config
   ```

**Remote Detection Strategy:**

When `git.remote` is configured:
1. Try to fetch from remote
2. If unreachable → Warn and continue (offline mode)
3. If empty repo → Push initial commit
4. If has `configs/` directory → Pull existing baseline (existing ZERB repo)
5. If has commits but no `configs/` → Warn about non-ZERB repo

**Key Design Decisions:**
- **Declarative-first**: Config in `zerb.lua` takes precedence over flags
- **Graceful degradation**: Works offline, warns on unreachable remotes
- **Smart detection**: Automatically detect existing ZERB repos vs empty repos
- **URL validation**: Support both SSH (`git@host:path`) and HTTPS (`https://host/path.git`)
- **No auth management**: Rely on system SSH agent and git credential helpers

**Implementation Scope (Future Change):**
- Add `--remote` and `--from` flags to `zerb init`
- Read `git.remote` from config during initialization
- Smart detection of existing ZERB repositories
- URL validation (SSH and HTTPS formats)
- Add `zerb remote add/remove/show` subcommands
- Integrate with `zerb push`/`zerb pull`/`zerb sync` commands

**Dependencies:**
- Builds on: This change (local git repository)
- Requires: Basic `zerb push`/`zerb pull` commands from Component 07
- Enables: Multi-machine sync workflows

**When to Create Formal Proposal:**

Create `openspec/changes/setup-git-remote/` when:
- This change is merged and deployed
- Component 07 Git Operations has basic push/pull working
- User feedback confirms remote is a priority feature
- Design questions are validated with real-world usage

**Estimated Timeline:** Post-MVP (Q1 2026)
