# Git Remote Setup - Future Proposal

**Status:** Planning  
**Depends On:** `setup-git-repository` change  
**Target:** Post-MVP  
**Created:** 2025-11-16  

---

## Overview

This document captures the design for automated git remote repository configuration and synchronization. This builds on the local git repository infrastructure established by the `setup-git-repository` change.

**Current State:**
- `zerb init` creates local git repository only
- Users must manually configure remotes: `git remote add origin <url>`
- Basic `zerb push`/`zerb pull` planned as simple git wrappers

**Future Vision:**
- Users declare remotes in `zerb.lua` (declarative approach)
- `zerb init` supports `--remote` and `--from` flags (imperative approach)
- Smart detection of existing ZERB repositories on remote
- Automatic baseline sync across machines
- Seamless first-machine vs second-machine setup

---

## User Stories

### Story 1: First Machine Setup (Push New Config)

**Scenario:** Developer creates new ZERB environment and wants to push to GitHub.

```bash
# Option A: Declarative (config-first)
cat > ~/.config/zerb/zerb.lua <<EOF
zerb = {
  tools = { "node@20" },
  git = {
    remote = "git@github.com:myuser/dotfiles.git"
  }
}
EOF
zerb init
# → Detects remote in config, pushes initial commit

# Option B: Imperative (flag-first)
zerb init --remote git@github.com:myuser/dotfiles.git
# → Adds remote, pushes initial commit, updates zerb.lua
```

**Expected Behavior:**
1. Initialize local git repository
2. Add remote as `origin`
3. Try to fetch from remote
4. If remote is empty/new → push initial commit
5. If remote has ZERB configs → warn about conflict, ask user to choose
6. Update `zerb.lua` to include `git.remote` (if using `--remote` flag)

### Story 2: Second Machine Setup (Clone Existing Baseline)

**Scenario:** Developer sets up ZERB on new laptop, wants to clone existing config.

```bash
# Option A: Clone existing config
zerb init --from git@github.com:myuser/dotfiles.git
# → Clones remote, activates latest config, sets up environment

# Option B: Pull after init
zerb init
zerb remote add git@github.com:myuser/dotfiles.git
zerb sync
# → Pulls existing baseline, resolves drift interactively
```

**Expected Behavior:**
1. Create local directory structure
2. Initialize git repository
3. Add remote as `origin`
4. Pull existing configs from remote
5. Detect latest config: `ls configs/ | sort -r | head -1`
6. Activate latest config (create symlink)
7. Install tools and apply configs per baseline

### Story 3: Offline Development (Deferred Sync)

**Scenario:** Developer works offline, syncs later when online.

```bash
# Initial setup (offline)
zerb init
# → Creates local repo, no remote configured

# Work offline
zerb add node@20
zerb config add ~/.zshrc

# Later, when online
cat >> ~/.config/zerb/configs/zerb.lua.active <<EOF
git = {
  remote = "git@github.com:myuser/dotfiles.git"
}
EOF
zerb sync
# → Pushes local commits to remote
```

**Expected Behavior:**
1. Init works without network
2. Local git commits created as normal
3. When remote configured + network available → sync
4. Graceful handling of unreachable remotes

---

## Proposed Design

### Configuration Schema

**In `zerb.lua`:**

```lua
zerb = {
  tools = { "node@20", "python@3.12" },
  configs = { "~/.zshrc", "~/.gitconfig" },
  
  git = {
    remote = "git@github.com:username/dotfiles.git",  -- Required for sync
    branch = "main",  -- Optional, defaults to "main"
  }
}
```

**Config Field Details:**
- `git.remote` (string): Git remote URL (SSH or HTTPS)
- `git.branch` (string): Branch name, defaults to `"main"`
- Both fields optional (local-only mode if omitted)

### CLI Interface

**New Flags for `zerb init`:**

```bash
# Initialize with remote (first machine)
zerb init --remote <url>
  # Adds remote, pushes initial commit
  # Updates zerb.lua to include git.remote

# Initialize from existing remote (second machine)
zerb init --from <url>
  # Clones remote, pulls configs, activates latest baseline
  # Sets up local structure, installs tools

# Local-only (current behavior)
zerb init
  # No remote configured
```

**New Subcommands:**

```bash
# Manage remotes
zerb remote add <url>       # Add remote to config and git
zerb remote remove          # Remove remote from config and git
zerb remote show            # Display current remote configuration
zerb remote set-url <url>   # Change remote URL

# Sync operations (already planned in Component 07)
zerb sync                   # Pull + drift detection + interactive resolution
zerb push                   # Push local commits to remote
zerb pull                   # Pull remote commits to local
```

### Remote Detection Logic

**Decision Tree:**

```
Is git.remote configured? (in zerb.lua or --remote flag)
├─ No → Skip remote setup (local-only mode)
│        User can add remote later via config or command
│
└─ Yes → Try to fetch from remote
    │
    ├─ Fetch fails (timeout, 404, auth failure)
    │  └─ Warn: "Remote configured but not reachable"
    │     Continue with local-only initialization
    │     User can run `zerb push` or `zerb sync` later
    │
    └─ Fetch succeeds → Check remote contents
        │
        ├─ Remote is empty (no commits)
        │  └─ Push initial commit
        │     Success: "Pushed initial configuration to remote"
        │
        ├─ Remote has commits but NO configs/ directory
        │  └─ Warn: "Remote exists but doesn't contain ZERB configuration"
        │     Options:
        │       1. Push and overwrite (--force required)
        │       2. Skip remote setup
        │       3. Abort init
        │
        └─ Remote has configs/ directory (existing ZERB repo)
           └─ Pull existing baseline
              Detect latest config timestamp
              Activate latest config
              Success: "Pulled existing configuration from remote"
```

**Implementation Pseudocode:**

```go
func setupRemote(cfg *Config, zerbDir string) error {
    // Check if remote configured
    if cfg.Git.Remote == "" {
        return nil // Local-only mode
    }
    
    // Validate URL format
    if !isValidGitURL(cfg.Git.Remote) {
        return fmt.Errorf("invalid git remote URL: %s", cfg.Git.Remote)
    }
    
    // Add remote
    repo, _ := git.PlainOpen(zerbDir)
    _, err := repo.CreateRemote(&config.RemoteConfig{
        Name: "origin",
        URLs: []string{cfg.Git.Remote},
    })
    if err != nil {
        return fmt.Errorf("failed to add remote: %w", err)
    }
    
    // Try to fetch
    err = repo.Fetch(&git.FetchOptions{
        RemoteName: "origin",
        Depth:      1, // Shallow clone for faster init
    })
    
    if err == git.NoErrAlreadyUpToDate {
        // Remote exists and is reachable
        return handleExistingRemote(repo, zerbDir)
    } else if err != nil {
        // Remote unreachable (offline, auth failure, doesn't exist)
        fmt.Fprintf(os.Stderr, "⚠️  Remote configured but not reachable: %v\n", err)
        fmt.Fprintf(os.Stderr, "   Continuing with local-only setup.\n")
        fmt.Fprintf(os.Stderr, "   Run 'zerb push' when remote is ready.\n")
        return nil
    }
    
    return nil
}

func handleExistingRemote(repo *git.Repository, zerbDir string) error {
    // Check if remote has configs/ directory
    if hasConfigsDirectory(repo) {
        // Pull existing baseline
        return pullExistingBaseline(repo, zerbDir)
    } else if hasCommits(repo) {
        // Remote exists but not ZERB
        return fmt.Errorf("remote exists but doesn't contain ZERB configuration")
    } else {
        // Remote is empty, push initial commit
        return pushInitialCommit(repo)
    }
}
```

### URL Validation

**Supported Formats:**

1. **SSH (recommended):**
   - `git@github.com:username/repo.git`
   - `ssh://git@github.com/username/repo.git`
   - `git@gitlab.com:group/subgroup/repo.git`

2. **HTTPS:**
   - `https://github.com/username/repo.git`
   - `https://gitlab.com/group/subgroup/repo.git`

**Validation Regex:**

```go
func isValidGitURL(url string) bool {
    // SSH format: git@host:path or ssh://git@host/path
    sshPattern := `^(git@[\w\.\-]+:[\w\.\-/]+|ssh://git@[\w\.\-]+/[\w\.\-/]+)(\.git)?$`
    
    // HTTPS format: https://host/path.git
    httpsPattern := `^https://[\w\.\-]+/[\w\.\-/]+(\.git)?$`
    
    sshMatch, _ := regexp.MatchString(sshPattern, url)
    httpsMatch, _ := regexp.MatchString(httpsPattern, url)
    
    return sshMatch || httpsMatch
}
```

**Error Messages:**

```
Invalid git remote URL: "example.com/repo"

Supported formats:
  SSH:   git@github.com:username/repo.git
  HTTPS: https://github.com/username/repo.git

See: zerb help remote
```

### Smart Repository Detection

**Detection Strategy:**

1. **Check for `configs/` directory** - Primary indicator of ZERB repo
2. **Check for `.gitignore` with ZERB patterns** - Secondary indicator
3. **Parse commit messages** - Look for "Initialize ZERB environment" message

**Implementation:**

```go
func hasConfigsDirectory(repo *git.Repository) bool {
    ref, err := repo.Head()
    if err != nil {
        return false
    }
    
    commit, err := repo.CommitObject(ref.Hash())
    if err != nil {
        return false
    }
    
    tree, err := commit.Tree()
    if err != nil {
        return false
    }
    
    // Check for configs/ directory
    _, err = tree.FindEntry("configs")
    return err == nil
}

func detectZerbRepo(repo *git.Repository) (bool, error) {
    // Check 1: configs/ directory exists
    if hasConfigsDirectory(repo) {
        return true, nil
    }
    
    // Check 2: Look for ZERB init commit message
    iter, _ := repo.Log(&git.LogOptions{})
    defer iter.Close()
    
    for {
        commit, err := iter.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return false, err
        }
        
        if strings.Contains(commit.Message, "Initialize ZERB environment") {
            return true, nil
        }
    }
    
    return false, nil
}
```

### Authentication Handling

**SSH Authentication:**
- Rely on system SSH agent and `~/.ssh/config`
- ZERB does NOT manage SSH keys
- User must set up SSH keys with git hosting provider

**HTTPS Authentication:**
- Rely on git credential helpers
- Respect user's existing git credential configuration
- No credential storage in ZERB

**Error Handling:**

```go
if err := repo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
    if err == git.ErrNonFastForwardUpdate {
        return fmt.Errorf("remote has diverged, pull first: zerb sync")
    } else if strings.Contains(err.Error(), "authentication") {
        return fmt.Errorf("authentication failed\n\n" +
            "For SSH: Ensure SSH keys are configured with git hosting provider\n" +
            "For HTTPS: Configure git credential helper\n\n" +
            "See: git help credentials")
    } else {
        return fmt.Errorf("push failed: %w", err)
    }
}
```

---

## Implementation Scope

### MVP Features (Post-MVP, Pre-1.0):

- [ ] Read `git.remote` and `git.branch` from `zerb.lua`
- [ ] Add `--remote <url>` flag to `zerb init`
- [ ] Add `--from <url>` flag to `zerb init`
- [ ] Remote URL validation (SSH and HTTPS)
- [ ] Smart detection of existing ZERB repos
- [ ] Push initial commit on first machine
- [ ] Pull existing baseline on second machine
- [ ] Graceful offline handling (warn and continue)
- [ ] Add `zerb remote add/remove/show` subcommands
- [ ] Update `zerb sync` to use configured remote
- [ ] Integration tests for all remote scenarios

### Post-1.0 Features (Future):

- [ ] Multiple remotes support (origin, backup, etc.)
- [ ] Remote URL auto-detection from GitHub CLI (`gh repo view --json url`)
- [ ] Interactive remote setup wizard (`zerb init --interactive`)
- [ ] Remote health checks (`zerb remote status`)
- [ ] Automatic remote creation (`zerb init --create-remote`)
- [ ] GitLab/Bitbucket/Gitea URL templates
- [ ] Remote backup and restore workflows

---

## Open Questions

### Q1: Should `--from` clone or pull?

**Options:**
1. Clone entire repo, replace local structure
2. Pull into existing structure (preserves local uncommitted work)

**Recommendation:** Pull into existing structure (Option 2)

**Rationale:**
- More flexible (user can have local changes)
- Consistent with `zerb sync` workflow
- Safer (doesn't delete local work)

### Q2: How to handle auth failures?

**Options:**
1. Retry with exponential backoff
2. Surface error immediately and fail
3. Fallback to local-only with warning

**Recommendation:** Option 3 (fallback to local-only)

**Rationale:**
- Auth setup is outside ZERB's control
- Retrying won't fix missing SSH keys
- User can fix auth and run `zerb push` later
- Aligns with graceful degradation philosophy

### Q3: Should we validate SSH keys before attempting operations?

**Options:**
1. Pre-check: `ssh -T git@github.com` before git operations
2. No pre-check: let git handle auth, surface errors

**Recommendation:** Option 2 (no pre-check)

**Rationale:**
- git/SSH already provide good error messages
- Pre-checking adds complexity and latency
- Different hosts (GitHub, GitLab, self-hosted) need different checks
- ZERB shouldn't reinvent git's auth layer

### Q4: What if remote and local both have unpushed commits?

**Options:**
1. Auto-merge (risky)
2. Fail and require manual resolution
3. Interactive conflict resolution

**Recommendation:** Option 2 for MVP, Option 3 post-MVP

**Rationale:**
- Auto-merge can lose data (especially with immutable timestamped configs)
- Manual resolution keeps user in control
- Post-MVP: add interactive resolution via `zerb sync --resolve`

### Q5: Should `git.remote` be required or optional?

**Recommendation:** Optional

**Rationale:**
- Local-only workflows are valid (single machine, testing, etc.)
- Users can add remote later
- Avoids forcing sync on every user
- Aligns with progressive disclosure (simple first, advanced later)

---

## When to Create Formal OpenSpec Change

Create `openspec/changes/setup-git-remote/` proposal when:

- [x] `setup-git-repository` change is merged and deployed
- [ ] Component 07 Git Operations has basic `push`/`pull` implemented
- [ ] User feedback confirms remote is high priority
- [ ] At least one team member has tested local-only workflow
- [ ] Design questions above are answered with real-world data

**Estimated Timeline:** Q1 2026 (post-MVP release)

---

## References

### Current Work
- `openspec/changes/setup-git-repository/` - Local git repository setup
- `.ai-workflow/implementation-planning/components/07-git-operations.md` - Git operations architecture

### Related Components
- Component 02: Lua Config - Where `git.remote` is parsed
- Component 05: Drift Detection - Integrated with `zerb sync`
- Component 07: Git Operations - Push/pull/conflict resolution

### External Documentation
- [go-git](https://github.com/go-git/go-git) - Pure Go git implementation
- [Git Authentication](https://git-scm.com/book/en/v2/Git-on-the-Server-The-Protocols) - SSH vs HTTPS
- [Git Credentials](https://git-scm.com/docs/gitcredentials) - Credential helpers

---

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Config location** | `git.remote` in `zerb.lua` | Single source of truth, declarative |
| **CLI flags** | `--remote` and `--from` | Imperative alternative for quick setup |
| **URL formats** | SSH and HTTPS | Standard git, broad compatibility |
| **Auth handling** | Defer to git/SSH | Don't reinvent auth, use system config |
| **Offline behavior** | Warn and continue | Graceful degradation, no blocking |
| **Remote detection** | Check for `configs/` dir | Reliable indicator of ZERB repo |
| **Conflict handling** | Fail, require manual resolution (MVP) | Safety first, avoid data loss |
| **Remote requirement** | Optional | Support local-only workflows |

---

**Document Version:** 1.0  
**Last Updated:** 2025-11-16  
**Status:** Ready for review and refinement based on MVP feedback
