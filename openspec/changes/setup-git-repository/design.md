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

### Decision 0: Git Implementation Strategy (go-git vs system git)

**Choice:** Use go-git library exclusively for all git operations; remove system git binary dependency.

**Rationale:**
- **Project architecture alignment**: `project.md` explicitly states "go-git (pure Go git implementation, no system git dependency)"
- **Complete isolation**: No dependency on system PATH or external binaries
- **Cross-platform compatibility**: Pure Go works identically on Linux, macOS, Windows without platform-specific git variants
- **Security**: No risk of PATH hijacking or executing untrusted git binaries
- **Deterministic behavior**: Library calls are more predictable than CLI output parsing
- **Testing**: Easier to mock and test; no need for system git in test environments
- **Minimal environments**: Can run in containers/systems without git installed

**Implementation:**
```go
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/object"
    "github.com/go-git/go-git/v5/config"
)

// Initialize repository
repo, err := git.PlainInit(zerbDir, false)

// Configure user
cfg, _ := repo.Config()
cfg.User.Name = "User Name"
cfg.User.Email = "user@example.com"
repo.Storer.SetConfig(cfg)

// Create commit
worktree, _ := repo.Worktree()
worktree.Add(".gitignore")
worktree.Add("configs/zerb.lua.20250116T143052.123Z")
worktree.Commit("Initialize ZERB environment", &git.CommitOptions{
    Author: &object.Signature{
        Name:  cfg.User.Name,
        Email: cfg.User.Email,
        When:  time.Now(),
    },
})
```

**Migration path:**
- Current `internal/git/git.go` uses `exec.CommandContext("git", ...)`
- This change migrates to go-git for: `Init`, `Configure`, `Add`, `Commit`
- Future Component 07 (git operations) will continue with go-git for: `Status`, `Log`, `Remote`, `Fetch`, `Pull`, `Push`

**Alternatives considered:**
- Continue with system git: Violates project architecture, creates security/isolation risks
- Hybrid approach (go-git for some, system git for others): Inconsistent, maintains security risks
- Shell out with trusted paths: Still violates isolation principle, complex platform handling

**Impact:**
- Add `github.com/go-git/go-git/v5` dependency to `go.mod`
- All git operations in this change use go-git APIs
- No CLI subprocess management needed
- Error handling uses Go errors, not stderr parsing

### Decision 0.1: ZERB Directory Security (Permissions)

**Choice:** Create ZERB root directory (`~/.config/zerb`) with `0700` permissions (user-only access).

**Rationale:**
- **Security**: Config files use `0600` because they may contain sensitive data; directory must also be restrictive
- **Git history protection**: On multi-user systems with non-`0700` `~/.config`, other users could read `.git/objects` even if config files are `0600`, exposing secrets through git history
- **Defense in depth**: Even if individual files have restrictive permissions, directory should prevent enumeration and access
- **Consistency**: All ZERB-managed content (configs, git history, binaries, cache) should be private to the user

**Implementation:**
```go
func createDirectoryStructure(zerbDir string) error {
    // Create ZERB root with user-only permissions
    if err := os.MkdirAll(zerbDir, 0700); err != nil {
        return fmt.Errorf("create ZERB directory: %w", err)
    }
    
    // Create subdirectories (inherit parent permissions or use explicit 0700)
    subdirs := []string{
        filepath.Join(zerbDir, "bin"),
        filepath.Join(zerbDir, "keyrings"),
        filepath.Join(zerbDir, "cache", "downloads"),
        // ... etc
    }
    
    for _, dir := range subdirs {
        if err := os.MkdirAll(dir, 0700); err != nil {
            return fmt.Errorf("create directory %s: %w", dir, err)
        }
    }
    
    return nil
}
```

**Verification:**
- Add test asserting `zerbDir` has mode `0700` after init
- Add test ensuring `.git` subdirectory inherits restrictive permissions
- Document in security guidelines that `~/.config/zerb` should remain private

**Alternatives considered:**
- Keep `0755` for directories: Rejected - exposes git history on multi-user systems
- Only protect specific files: Insufficient - `.git/objects/` would still be readable
- Rely on `~/.config` being private: Not guaranteed on all systems; not defensive

**Trade-offs:**
- Slightly less convenient for debugging (can't easily `ls` as another user)
- **Benefit**: Strong security guarantee even on shared systems

### Decision 1: When to Initialize Git

**Choice:** Initialize during `zerb init`, after directory structure but before shell integration.

**Rationale:**
- Natural fit: config versioning is core to ZERB's value proposition
- Early initialization: enables all subsequent operations to assume git exists
- Order matters: need directories first, then .gitignore, then git init, then initial config, then commit

**Sequence:**
1. Create directory structure (with 0700 permissions)
2. Write .gitignore file
3. Initialize git repository (go-git)
4. Configure git user (repo-local, using go-git)
5. Extract keyrings and install binaries
6. Generate initial config
7. Create initial commit (add .gitignore and config)

**Alternatives considered:**
- Lazy initialization (first commit): Adds complexity, delays benefits
- Separate `zerb git init` command: Extra step, not discoverable
- Post-shell-integration: Doesn't affect decision, current placement is logical

### Decision 2: Git User Configuration Strategy

**Choice:** Three-tier fallback system (environment → ZERB config → placeholders)

1. Try environment variables (`ZERB_GIT_NAME`, `ZERB_GIT_EMAIL`, or `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`)
2. Try ZERB-local configuration (future: `git.user.*` in `zerb.lua`)
3. Use placeholder values (`ZERB User`, `zerb@localhost`)

**Rationale:**
- **Isolation principle**: Never read or write global git config (`~/.gitconfig`) to maintain complete ZERB isolation
- Environment variables provide explicit override capability (CI/CD, testing, user preference)
- Future ZERB config integration allows per-environment git identity
- Placeholder ensures init always succeeds (can fix later via environment or config)
- Warning message guides users to fix placeholder values

**Implementation:**
```go
type GitUserInfo struct {
    Name      string
    Email     string
    FromEnv   bool
    FromConfig bool
    IsDefault bool
}

func detectGitUser(cfg *config.Config) GitUserInfo {
    // 1. Try ZERB-specific environment variables first
    if name := os.Getenv("ZERB_GIT_NAME"); name != "" {
        email := os.Getenv("ZERB_GIT_EMAIL")
        return GitUserInfo{Name: name, Email: email, FromEnv: true}
    }
    
    // 2. Try standard git environment variables
    if name := os.Getenv("GIT_AUTHOR_NAME"); name != "" {
        email := os.Getenv("GIT_AUTHOR_EMAIL")
        return GitUserInfo{Name: name, Email: email, FromEnv: true}
    }
    
    // 3. Try ZERB config (future: cfg.Git.User.Name / cfg.Git.User.Email)
    // Not implemented in this change
    
    // 4. Fallback to placeholders
    return GitUserInfo{
        Name:      "ZERB User",
        Email:     "zerb@localhost",
        IsDefault: true,
    }
}
```

**Using go-git:**
```go
import "github.com/go-git/go-git/v5/config"

func configureGitUser(repo *git.Repository, userInfo GitUserInfo) error {
    cfg, err := repo.Config()
    if err != nil {
        return fmt.Errorf("read repo config: %w", err)
    }
    
    cfg.User.Name = userInfo.Name
    cfg.User.Email = userInfo.Email
    
    if err := repo.Storer.SetConfig(cfg); err != nil {
        return fmt.Errorf("write repo config: %w", err)
    }
    
    return nil
}
```

**Alternatives considered:**
- Read global git config: **Rejected** - violates ZERB isolation principle
- Prompt user for name/email: Breaks non-interactive init, slows UX
- Require git config: Too strict, fails unnecessarily
- Use system username/hostname: Privacy concerns, not meaningful for commits

**Note:** This decision aligns with ZERB's isolation architecture, ensuring ZERB never modifies or depends on system-wide git configuration.

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

**Template location:** Embedded in `internal/git` package as string constant

**Security note:** The ZERB root directory is created with `0700` permissions (Decision 0.1), ensuring that `.git/objects/` and all tracked/untracked files are only accessible to the owning user, even on multi-user systems.

**Alternatives considered:**
- Track generated configs: Creates data duplication, confusing diffs
- Dynamic generation: Overkill for static patterns
- User-editable .gitignore: Increases support burden, pattern mistakes
- No .gitignore: Runtime files pollute repo, bad UX

### Decision 4: Initial Commit Content

**Choice:** Commit both `.gitignore` and timestamped config file

**Rationale:**
- Establishes git history from the start with meaningful content
- `.gitignore` is source of truth for tracking policy - should be versioned from day 1
- Prevents accidental commits of runtime/generated files immediately
- Users can see what's ignored via `git show` or `git log`
- Directory structure doesn't need to be tracked (recreated by init)
- Clean git history showing both setup files together

**Commit message:** `"Initialize ZERB environment"`

**Files in initial commit:**
1. `.gitignore` (tracking policy - must be first)
2. `configs/zerb.lua.YYYYMMDDTHHMMSS.sssZ` (initial empty config)

**Commit order matters:**
- Write `.gitignore` to disk **before** `git init`
- This ensures the ignore patterns are in effect before the initial `git add`
- Prevents any risk of accidentally staging runtime files

**Alternatives considered:**
- Config only: Conflicts with spec requirement; `.gitignore` wouldn't be versioned initially
- Empty commit: Less meaningful, doesn't test git operations
- Include all directories: Unnecessary, git doesn't track empty directories
- Include keyrings: Security concern (even if public keys)

### Decision 5: Error Handling for Missing Git Library

**Choice:** Warn and continue (graceful degradation), with persistent warnings

**Behavior:**
- Attempt to initialize git repository using go-git library
- If initialization fails (library issue, permissions, etc.): print warning, skip git initialization, continue init
- Create marker file `.zerb-no-git` to track git-unavailable state
- On `zerb activate`: check for marker and show warning prompting git setup
- User can manually initialize git later if desired
- All other ZERB features work without git (drift detection uses current state)

**Warning message (during init):**
```
⚠ Warning: Unable to initialize git repository
  
  Git versioning not available.
  
  To set up git versioning later:
    1. Ensure write permissions in ~/.config/zerb
    2. Run: zerb git init
  
  ZERB will continue without version control.
```

**Warning message (on activate, if `.zerb-no-git` exists):**
```
⚠ Note: Git versioning not initialized
  
  Your ZERB environment is working, but configuration changes
  are not being tracked in version history.
  
  To enable versioning and sync:
    zerb git init
  
  (This message appears once per activate until git is set up)
```

**Implementation approach:**
Using go-git library:
```go
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/object"
)

func initGitRepo(zerbDir string) error {
    repo, err := git.PlainInit(zerbDir, false)
    if err != nil {
        return fmt.Errorf("initialize git repository: %w", err)
    }
    return nil
}
```

**Rationale:**
- Don't block init: ZERB has value without git versioning (tool management, config generation)
- Clear guidance: user knows exactly how to enable git later
- Aligns with isolation: ZERB uses go-git library, not system git binary
- Persistent warning educates users without being annoying (only on activate, not every command)
- Provides future `zerb git init` command for deferred setup

**Alternatives considered:**
- Fail init: Too strict, reduces ZERB adoption for users who don't need git
- Silent skip: Confusing, user doesn't know why git features fail later
- Warn every command: Too noisy, user fatigue

**Future enhancement:**
- `zerb git init` command to initialize git in already-initialized ZERB environment
- `zerb doctor` check for git status and recommendations

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
- Verify it's a valid git repo using go-git
- If invalid, warn and skip git init (don't overwrite)

**Implementation:**
```go
import "github.com/go-git/go-git/v5"

func isGitRepo(dir string) (bool, error) {
    _, err := git.PlainOpen(dir)
    if err == git.ErrRepositoryNotExists {
        return false, nil
    }
    if err != nil {
        return false, err // Corrupt or invalid repo
    }
    return true, nil
}

// In runInit:
if valid, err := isGitRepo(zerbDir); err != nil {
    fmt.Fprintf(os.Stderr, "⚠ Warning: Invalid git repository detected\n")
    fmt.Fprintf(os.Stderr, "  Skipping git initialization.\n")
    fmt.Fprintf(os.Stderr, "  Fix or remove .git directory to enable versioning.\n")
    // Continue init without git
} else if valid {
    fmt.Println("✓ Git repository already exists")
    // Continue init, skip git initialization
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

## Future Work: Pre-commit Hooks

Git repository initialization enables future pre-commit hooks that will enforce:
1. Timestamped config immutability (prevent modifications to `configs/zerb.lua.*` snapshots)
2. Lua syntax validation
3. ZERB schema validation
4. Large file warnings (>10MB)
5. Secret detection (prevent credential leaks)

**See:** `openspec/future-proposal-information/pre-commit-hooks.md` for detailed planning.

**Dependencies:** This change (git repo must exist) → Component 07 (git operations) → Pre-commit hooks implementation

**Timeline:** Post-MVP, after remote sync is implemented and secret detection patterns are validated.

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
