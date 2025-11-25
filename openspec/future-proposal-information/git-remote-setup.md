# Git Remote Setup - Future Proposal

**Status:** Planning  
**Depends On:** `setup-git-repository` change  
**Target:** MVP  
**Created:** 2025-11-16  
**Last Updated:** 2025-11-25  

---

## Overview

This document captures the design for automated git remote repository configuration and synchronization. This builds on the local git repository infrastructure established by the `setup-git-repository` change.

**Design Philosophy:**
- Git is completely invisible to users except for providing the remote URL
- Users interact with "local baseline" and "remote baseline" concepts
- No git commands are ever shown or required from users

**Current State:**
- `zerb init` creates local git repository only
- Users must manually configure remotes (not acceptable for MVP)
- Basic sync commands not yet implemented

**Future Vision:**
- Users configure remote via `zerb remote set <url>` or `zerb init --from <url>`
- `zerb push` / `zerb pull` / `zerb status` for baseline synchronization
- Automatic merge when possible, interactive conflict resolution when needed
- Seamless first-machine vs second-machine setup

---

## Terminology

| Term | Meaning |
|------|---------|
| **Local Baseline** | The active config on this machine (`~/.config/zerb/zerb.active.lua`) |
| **Remote Baseline** | The config stored in the git remote (source of truth across machines) |

This maps to how users think about synchronization:
- "I changed my local setup, now I want to push it so my other machines can get it"
- "I want to pull the latest from my remote baseline"

---

## User Stories

### Story 1: First Machine Setup (Push New Config)

**Scenario:** Developer creates new ZERB environment and wants to push to GitHub.

```bash
$ zerb init
✓ ZERB initialized at ~/.config/zerb

To sync your baseline across machines, set up a git remote:

  1. Create a private git repository (GitHub, GitLab, etc.)
  2. Run: zerb remote set <repository-url>
  3. Run: zerb push

Example:
  zerb remote set git@github.com:username/dotfiles.git
  zerb push

$ zerb remote set git@github.com:myuser/dotfiles.git
✓ Remote configured: git@github.com:myuser/dotfiles.git

Run 'zerb push' to upload your baseline.

$ zerb push
Pushing local baseline to remote...
✓ Remote baseline updated
```

**Expected Behavior:**
1. Initialize local git repository
2. Show guidance about remote setup
3. User configures remote via `zerb remote set`
4. User explicitly pushes with `zerb push`
5. Remote URL stored in `zerb.lua` under `git.remote`

### Story 2: Second Machine Setup (Clone Existing Baseline)

**Scenario:** Developer sets up ZERB on new laptop, wants to clone existing config.

```bash
$ zerb init --from git@github.com:myuser/dotfiles.git
Downloading baseline from remote...
✓ ZERB initialized from remote baseline
✓ Remote configured

Installing tools...
  ✓ node@20.11.0
  ✓ python@3.12

Applying configs...
  ✓ ~/.zshrc
  ✓ ~/.gitconfig
```

**Expected Behavior:**
1. Create local directory structure
2. Initialize git repository
3. Add remote as `origin`
4. Pull existing configs from remote
5. Detect latest config timestamp
6. Activate latest config
7. Install tools and apply configs per baseline

### Story 3: Offline Development (Deferred Sync)

**Scenario:** Developer works offline, syncs later when online.

```bash
# Initial setup (offline)
$ zerb init
✓ ZERB initialized at ~/.config/zerb
# (guidance about remote setup shown)

# Work offline
$ zerb config add ~/.zshrc
$ zerb config add ~/.gitconfig

# Later, when online
$ zerb remote set git@github.com:myuser/dotfiles.git
✓ Remote configured

$ zerb push
Pushing local baseline to remote...
✓ Remote baseline updated
```

**Expected Behavior:**
1. Init works without network
2. Local changes are tracked automatically (git commits happen internally)
3. When remote configured + network available, user runs `zerb push`
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
# Initialize from existing remote (second machine)
zerb init --from <url>
  # Clones remote, pulls configs, activates latest baseline
  # Sets up local structure, installs tools

# Local-only (current behavior, shows guidance)
zerb init
  # No remote configured, shows setup instructions
```

**New Subcommands:**

```bash
# Manage remotes
zerb remote set <url>   # Configure remote repository URL
zerb remote show        # Display current remote configuration
zerb remote clear       # Remove remote configuration

# Sync operations
zerb push               # Push local baseline to remote
zerb pull               # Pull remote baseline to local (auto-merge + conflict resolution)
zerb status             # Compare local vs remote baseline
```

### Example Messages

**`zerb remote set`:**
```
$ zerb remote set git@github.com:user/dotfiles.git
✓ Remote configured: git@github.com:user/dotfiles.git

Run 'zerb push' to upload your baseline.
```

**`zerb remote show`:**
```
$ zerb remote show
Remote: git@github.com:user/dotfiles.git
```

**`zerb remote show` (no remote):**
```
$ zerb remote show
No remote configured.

Run 'zerb remote set <url>' to configure one.
```

**`zerb push` (success):**
```
$ zerb push
Pushing local baseline to remote...
✓ Remote baseline updated
```

**`zerb push` (no remote):**
```
$ zerb push
No remote configured.

Run 'zerb remote set <url>' to configure one.
```

**`zerb pull` (success, no changes):**
```
$ zerb pull
Pulling remote baseline...
✓ Already up to date
```

**`zerb pull` (success, with auto-merge):**
```
$ zerb pull
Pulling remote baseline...
✓ Merged changes from remote

Changes:
  + Added: ~/.config/starship.toml
  ~ Updated: node@20.10.0 → node@20.11.0
```

**`zerb pull` (conflict - see Conflict Resolution section):**
```
$ zerb pull
Pulling remote baseline...

Conflict detected between local and remote baselines.

[CONFLICT] python
  Local:   python@3.12
  Remote:  python@3.11

Resolution:
  1. Keep local (python@3.12)
  2. Accept remote (python@3.11)

Choice [1-2]: 1

✓ Resolved: keeping local version
✓ Local baseline updated

Run 'zerb push' to update remote with your resolution.
```

**`zerb status` (in sync):**
```
$ zerb status
Local baseline:  zerb.lua.20250115T143022Z
Remote baseline: zerb.lua.20250115T143022Z

✓ Local and remote baselines are in sync
```

**`zerb status` (local ahead):**
```
$ zerb status
Local baseline:  zerb.lua.20250115T160000Z
Remote baseline: zerb.lua.20250115T143022Z

Local is ahead of remote by 2 changes.

Run 'zerb push' to update remote baseline.
```

**`zerb status` (remote ahead):**
```
$ zerb status
Local baseline:  zerb.lua.20250115T143022Z
Remote baseline: zerb.lua.20250115T180000Z

Remote is ahead of local by 3 changes.

Run 'zerb pull' to update local baseline.
```

**`zerb status` (diverged):**
```
$ zerb status
Local baseline:  zerb.lua.20250115T160000Z
Remote baseline: zerb.lua.20250115T180000Z

Local and remote have diverged.
  Local:  1 unpushed change
  Remote: 2 new changes

Run 'zerb pull' to merge remote changes, then 'zerb push'.
```

### Remote Detection Logic

**Decision Tree:**

```
Is git.remote configured? (in zerb.lua or --from flag)
├─ No → Skip remote setup (local-only mode)
│        User can add remote later via `zerb remote set`
│
└─ Yes → Try to fetch from remote
    │
    ├─ Fetch fails (timeout, 404, auth failure)
    │  └─ Warn: "Remote configured but not reachable"
    │     Continue with local-only initialization
    │     User can run `zerb push` later
    │
    └─ Fetch succeeds → Check remote contents
        │
        ├─ Remote is empty (no commits)
        │  └─ Ready for push
        │     Success: "Remote configured, run 'zerb push' to upload"
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
        fmt.Fprintf(os.Stderr, "Remote configured but not reachable: %v\n", err)
        fmt.Fprintf(os.Stderr, "Continuing with local-only setup.\n")
        fmt.Fprintf(os.Stderr, "Run 'zerb push' when remote is ready.\n")
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
        // Remote is empty, ready for push
        return nil
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

See: zerb remote --help
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

### Conflict Resolution

**Design Philosophy:**
- Git auto-merges when possible
- Show "Merged changes from remote" message on successful auto-merge
- When conflicts occur, present them as baseline differences (drift-style), not git markers
- Binary choice for MVP: keep local or accept remote
- Always interactive for MVP (no scripting flags)

**How Conflict Resolution Works:**

1. `zerb pull` does a `git fetch` (hidden from user)
2. Attempts `git merge` (hidden from user)
3. If merge succeeds:
   - Update local baseline
   - Show "Merged changes from remote" with summary of changes
4. If merge conflicts:
   - Parse both versions of zerb.lua programmatically
   - Compare the configs to identify specific differences
   - Present conflicts as baseline differences (similar to drift detection)
   - User chooses: keep local or accept remote
   - Zerb generates the resolved config and commits internally

**Conflict Resolution UX:**

```
$ zerb pull
Pulling remote baseline...

Conflict detected between local and remote baselines.

[CONFLICT] python
  Local:   python@3.12
  Remote:  python@3.11

Resolution:
  1. Keep local (python@3.12)
  2. Accept remote (python@3.11)

Choice [1-2]: 1

✓ Resolved: keeping local version
✓ Local baseline updated

Run 'zerb push' to update remote with your resolution.
```

**Multiple Conflicts:**

```
$ zerb pull
Pulling remote baseline...

Conflict detected between local and remote baselines.

[CONFLICT] python
  Local:   python@3.12
  Remote:  python@3.11

[CONFLICT] ~/.config/nvim/
  Local:   (added)
  Remote:  (not present)

Resolution for python:
  1. Keep local (python@3.12)
  2. Accept remote (python@3.11)

Choice [1-2]: 1

Resolution for ~/.config/nvim/:
  1. Keep local (added)
  2. Accept remote (not present)

Choice [1-2]: 1

✓ Resolved: keeping local versions
✓ Local baseline updated

Run 'zerb push' to update remote with your resolution.
```

**Benefits of This Approach:**
- User never sees git conflict markers
- Resolution is domain-aware (tools, configs) not line-based
- Consistent UX with existing drift detection
- Git is purely an implementation detail

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
        return fmt.Errorf("remote has changed, run 'zerb pull' first")
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

### MVP Features:

- [ ] Read `git.remote` and `git.branch` from `zerb.lua`
- [ ] Add `--from <url>` flag to `zerb init`
- [ ] Add guidance message after `zerb init` (no remote)
- [ ] Remote URL validation (SSH and HTTPS)
- [ ] Smart detection of existing ZERB repos
- [ ] Push initial commit on first machine (`zerb push`)
- [ ] Pull existing baseline on second machine (`zerb pull`)
- [ ] Auto-merge with "Merged changes from remote" message
- [ ] Interactive conflict resolution (drift-style UX)
- [ ] Graceful offline handling (warn and continue)
- [ ] Add `zerb remote set/show/clear` subcommands
- [ ] Add `zerb push` command
- [ ] Add `zerb pull` command
- [ ] Add `zerb status` command
- [ ] Integration tests for all remote scenarios

### Post-MVP Features (Future):

- [ ] Scripting flags for conflict resolution (`--theirs` / `--ours`)
- [ ] Multiple remotes support (origin, backup, etc.)
- [ ] Remote URL auto-detection from GitHub CLI (`gh repo view --json url`)
- [ ] Interactive remote setup wizard (`zerb init --interactive`)
- [ ] Remote health checks
- [ ] Automatic remote creation (`zerb init --create-remote`)
- [ ] GitLab/Bitbucket/Gitea URL templates
- [ ] Remote backup and restore workflows
- [ ] Granular conflict resolution (per-item choices)

---

## Resolved Design Questions

### Q1: Should `--from` clone or pull?

**Decision:** Pull into existing structure

**Rationale:**
- More flexible (user can have local changes)
- Consistent with `zerb pull` workflow
- Safer (doesn't delete local work)

### Q2: How to handle auth failures?

**Decision:** Fallback to local-only with warning

**Rationale:**
- Auth setup is outside ZERB's control
- Retrying won't fix missing SSH keys
- User can fix auth and run `zerb push` later
- Aligns with graceful degradation philosophy

### Q3: Should we validate SSH keys before attempting operations?

**Decision:** No pre-check, let git handle auth

**Rationale:**
- git/SSH already provide good error messages
- Pre-checking adds complexity and latency
- Different hosts (GitHub, GitLab, self-hosted) need different checks
- ZERB shouldn't reinvent git's auth layer

### Q4: What if remote and local both have unpushed commits?

**Decision:** Auto-merge when possible, interactive resolution for conflicts

**Rationale:**
- Auto-merge handles the common case silently with a message
- Conflicts require user input to avoid data loss
- Drift-style UX keeps git invisible

### Q5: Should `git.remote` be required or optional?

**Decision:** Optional

**Rationale:**
- Local-only workflows are valid (single machine, testing, etc.)
- Users can add remote later
- Avoids forcing sync on every user
- Aligns with progressive disclosure (simple first, advanced later)

### Q6: Should push be automatic after local changes?

**Decision:** No, user must explicitly run `zerb push`

**Rationale:**
- User controls when changes are shared
- Avoids unexpected network activity
- Works well with offline workflows
- Explicit is better than implicit

### Q7: How visible should git be?

**Decision:** Completely invisible except for providing remote URL

**Rationale:**
- Users shouldn't need to know git to use zerb
- Remote URL is the only git-specific concept users need
- All sync operations use zerb terminology (push/pull/status)
- Error messages avoid git jargon where possible

---

## When to Create Formal OpenSpec Change

Create `openspec/changes/setup-git-remote/` proposal when:

- [x] `setup-git-repository` change is merged and deployed
- [x] Design decisions finalized (this document)
- [ ] Ready to begin implementation
- [ ] Component 07 Git Operations architecture reviewed

**Estimated Timeline:** MVP implementation

---

## References

### Current Work
- `openspec/changes/setup-git-repository/` - Local git repository setup

### Related Components
- Component 02: Lua Config - Where `git.remote` is parsed
- Component 05: Drift Detection - UX pattern for conflict resolution
- Component 07: Git Operations - Push/pull/conflict resolution

### External Documentation
- [go-git](https://github.com/go-git/go-git) - Pure Go git implementation
- [Git Authentication](https://git-scm.com/book/en/v2/Git-on-the-Server-The-Protocols) - SSH vs HTTPS
- [Git Credentials](https://git-scm.com/docs/gitcredentials) - Credential helpers

---

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Commands** | `zerb push`, `zerb pull`, `zerb status` | Short, memorable, familiar pattern |
| **Remote management** | `zerb remote set/show/clear` | Simple imperative interface |
| **Config location** | `git.remote` in `zerb.lua` | Single source of truth, declarative |
| **Init flag** | `--from <url>` only | Clone from existing baseline |
| **Init guidance** | Show setup instructions | Help users understand next steps |
| **URL formats** | SSH and HTTPS | Standard git, broad compatibility |
| **Auth handling** | Defer to git/SSH | Don't reinvent auth, use system config |
| **Offline behavior** | Warn and continue | Graceful degradation, no blocking |
| **Remote detection** | Check for `configs/` dir | Reliable indicator of ZERB repo |
| **Auto-push** | No | User explicitly pushes changes |
| **Auto-merge** | Yes, with message | Show "Merged changes from remote" |
| **Conflict handling** | Interactive, drift-style UX | Binary choice: local or remote |
| **Conflict scripting** | Post-MVP | `--theirs`/`--ours` flags later |
| **Git visibility** | Invisible except remote URL | Users don't need to know git |
| **Remote requirement** | Optional | Support local-only workflows |

---

**Document Version:** 2.0  
**Last Updated:** 2025-11-25  
**Status:** Design finalized, ready for implementation planning
