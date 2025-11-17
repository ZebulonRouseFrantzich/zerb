# Future Proposal: Deferred Git Initialization

## Overview

Add `zerb git init` command to initialize git repository in an already-initialized ZERB environment. This enables users who skipped git during initial setup (or experienced git initialization failures) to add version control later without re-running `zerb init`.

## Why Deferred?

The `setup-git-repository` change initializes git during `zerb init` by default. However, users may need to set up git later because:

1. **Git initialization failed** during `zerb init` (permissions, disk space, etc.)
2. **User skipped git** intentionally (didn't want version control initially)
3. **Repair scenario** - `.git` directory was deleted or corrupted
4. **Testing/CI environments** - Initialize ZERB without git, add git only when needed

## User Stories

### Story 1: Repair After Failure
**As a user** whose git initialization failed during `zerb init`,  
**I want to** set up git without re-running full initialization,  
**So that** I can enable version control without disrupting my working environment.

### Story 2: Opt-In Version Control
**As a user** who initially skipped git features,  
**I want to** add version control to my existing ZERB setup,  
**So that** I can start tracking configuration history and sync across machines.

### Story 3: CI/Development Workflow
**As a developer** setting up ZERB in CI or test environments,  
**I want to** initialize ZERB quickly without git, then add git selectively,  
**So that** I can optimize workflow speed and test git-related features separately.

## Current Workaround (Temporary)

Users experiencing git initialization failures currently see this warning on `zerb activate`:

```
⚠ Note: Git versioning not initialized
  
  Your ZERB environment is working, but configuration changes
  are not being tracked in version history.
  
  To enable versioning and sync (temporary workaround):
    rm ~/.config/zerb/.zerb-no-git
    zerb uninit && zerb init
  
  (This message appears once per activate until git is set up)
```

**Problems with this approach:**
- Requires full re-initialization (disrupts workflow)
- Loses current configuration state if not careful
- Uninit/init cycle is heavyweight for adding one feature
- Not discoverable for users who don't see the warning

## Design Decisions

### Decision 1: Command Naming and Structure

**Choice:** `zerb git init` subcommand under `git` namespace

**Rationale:**
- Aligns with future `zerb git` commands (`status`, `log`, `sync`, etc.)
- Familiar to git users (`git init` → `zerb git init`)
- Namespace separation keeps root command clean

**Alternatives considered:**
- `zerb init --git-only`: Confusing, suggests partial initialization
- `zerb repair git`: Too generic, repair implies fixing corruption
- `zerb enable git`: Doesn't convey initialization semantics

### Decision 2: Preconditions and Validation

**Choice:** Strict validation before initializing git

**Preconditions:**
1. ZERB directory exists (`~/.config/zerb`)
2. ZERB is initialized (check for required directories and files)
3. Git repository does NOT exist (`.git` directory absent)
4. `.zerb-no-git` marker may or may not exist

**Validation logic:**
```go
func (g *GitService) CanInitGit() error {
    // Check ZERB initialized
    if !isZerbInitialized(g.zerbDir) {
        return errors.New("ZERB not initialized. Run 'zerb init' first")
    }
    
    // Check git doesn't exist
    if isGitRepo, _ := g.gitClient.IsGitRepo(); isGitRepo {
        return errors.New("git repository already exists")
    }
    
    // Check .git isn't corrupted
    gitPath := filepath.Join(g.zerbDir, ".git")
    if stat, err := os.Stat(gitPath); err == nil {
        if stat.IsDir() {
            return errors.New("corrupted .git directory detected. Remove it first: rm -rf ~/.config/zerb/.git")
        }
    }
    
    return nil // Safe to initialize
}
```

**Rationale:**
- Prevents accidentally reinitializing git (destructive)
- Provides clear error messages for each failure mode
- Guides user to fix issues (remove corrupted .git, run init, etc.)

### Decision 3: Initialization Sequence

**Choice:** Mirror the git-related steps from `zerb init`

**Sequence:**
1. Validate preconditions (Decision 2)
2. Write `.gitignore` file (if missing or outdated)
3. Initialize git repository using go-git
4. Configure git user (same fallback chain: ZERB env vars → GIT_AUTHOR → placeholders)
5. Create initial commit with existing config files
6. Remove `.zerb-no-git` marker (if exists)
7. Display success message with git status

**Rationale:**
- Reuses existing, tested git initialization code from `internal/git`
- Ensures consistency with `zerb init` behavior
- Creates valid git history immediately

### Decision 4: Handling Existing Configurations

**Choice:** Commit all existing timestamped configs in initial commit

**Behavior:**
- Find all `configs/zerb.lua.*` files
- Include `.gitignore` and all configs in initial commit
- Commit message: `"Initialize ZERB git repository (deferred setup)"`
- Preserves configuration history by timestamping existing files

**Rationale:**
- Users may have multiple configs already (from `zerb config add`)
- Initial commit should represent current ZERB state
- Differentiate from `zerb init` commit with distinct message

**Example:**
```
Initial commit contains:
  .gitignore
  configs/zerb.lua.20250110T120000.000Z  (original)
  configs/zerb.lua.20250115T140000.000Z  (user added)
  configs/zerb.lua.20250116T160000.000Z  (current active)
```

### Decision 5: Git User Configuration

**Choice:** Same three-tier fallback as `zerb init`

**Fallback chain:**
1. `ZERB_GIT_NAME` + `ZERB_GIT_EMAIL` (both required)
2. `GIT_AUTHOR_NAME` + `GIT_AUTHOR_EMAIL` (both required)
3. Placeholders: `ZERB User <zerb@localhost>`

**Warning behavior:**
- If placeholders used, display same warning as `zerb init`
- Remind user to set environment variables
- Explain repository-local config (not global `~/.gitconfig`)

**Rationale:**
- Consistency with `zerb init` reduces cognitive load
- Users expect same configuration approach
- Isolation principle maintained

### Decision 6: Idempotency

**Choice:** Fail fast if git already initialized

**Behavior:**
```bash
$ zerb git init
Error: git repository already exists at ~/.config/zerb/.git

If you need to reinitialize git:
  1. Back up your ZERB directory
  2. Remove git: rm -rf ~/.config/zerb/.git
  3. Run: zerb git init
```

**Rationale:**
- Prevents accidental data loss (git history destruction)
- Forces user to be explicit about reinitialization
- Provides clear recovery path

### Decision 7: `.zerb-no-git` Marker Handling

**Choice:** Remove marker on successful initialization

**Behavior:**
- Check if `.zerb-no-git` exists
- If exists: remove after git init succeeds
- If doesn't exist: no action needed
- On failure: leave marker in place (or create if it didn't exist)

**Rationale:**
- Marker indicates "git unavailable" state
- Successful `zerb git init` makes git available
- Future `zerb activate` won't show warning after successful setup

## Implementation Scope

### New Files
- `cmd/zerb/git_init.go` - Command implementation
- `cmd/zerb/git_init_test.go` - Command tests

### Modified Files
- `cmd/zerb/main.go` - Register `git init` subcommand
- `internal/git/git.go` - Expose initialization methods (may need refactoring)

### New Functions
```go
// cmd/zerb/git_init.go
func newGitInitCmd() *cobra.Command
func runGitInit(cmd *cobra.Command, args []string) error

// internal/service/git.go (new service layer)
func (s *GitService) InitializeGit() error
func (s *GitService) CanInitGit() error
func (s *GitService) CommitExistingConfigs() error
```

### Reused Functions (from `setup-git-repository`)
- `internal/git.InitRepo()`
- `internal/git.ConfigureUser()`
- `internal/git.CreateInitialCommit()`
- `internal/git.WriteGitignore()`
- `internal/git.IsGitRepo()`
- Git user detection logic

### Testing Requirements
- Unit tests for validation logic
- Integration test: deferred init after `zerb init --skip-git` (if flag exists)
- Integration test: deferred init after git failure
- Integration test: idempotency (run `zerb git init` twice)
- Integration test: commit multiple existing configs
- Error case tests: ZERB not initialized, git already exists, corrupted .git

## Open Questions

### Q1: Should we add `--force` flag to reinitialize git?

**Options:**
1. **No `--force` flag** (recommended) - Always fail if git exists, user must manually remove `.git`
2. **Add `--force` flag** - Allow destructive reinitialization with explicit opt-in

**Recommendation:** Start without `--force`. Add later if user feedback indicates need. Prevents accidental data loss in MVP.

### Q2: Should `zerb init` support `--skip-git` flag?

**Options:**
1. **No flag** - Git initialization always attempted (current behavior)
2. **Add `--skip-git`** - Explicit opt-out, creates `.zerb-no-git` marker

**Recommendation:** Add `--skip-git` in same change as `zerb git init` for symmetry. Enables intentional git-less workflows.

### Q3: Should we support git initialization during `zerb uninit`/`zerb repair`?

**Recommendation:** Out of scope for MVP. `zerb git init` provides explicit control. Future `zerb doctor` command can suggest `zerb git init` if git is missing.

### Q4: What if `.gitignore` already exists but is outdated?

**Options:**
1. **Overwrite** - Always write current template
2. **Preserve** - Don't touch existing `.gitignore`
3. **Merge** - Add missing patterns to existing file

**Recommendation:** Overwrite with warning. Display diff if file exists. User can restore from backup if needed. Ensures consistency with current ZERB architecture.

## When to Create Formal Proposal

Create `openspec/changes/git-deferred-init/` when:

1. ✅ `setup-git-repository` change is merged and deployed
2. User feedback confirms deferred git init is needed (MVP usage validates demand)
3. `zerb git` namespace is established (may happen with Component 07 git operations)
4. Design questions (Q1-Q4) are validated with real-world usage

**Estimated Timeline:** Post-MVP (Q1 2026), after git operations component is implemented

## Dependencies

**Builds on:**
- `setup-git-repository` (must be merged first)
- Git initialization code in `internal/git` (reused)

**Enables:**
- Graceful recovery from git initialization failures
- Opt-in version control workflows
- Better testing/CI support (initialize without git, add later)

**Related work:**
- Component 07: Git Operations (may influence `git` subcommand structure)
- `zerb doctor` command (can suggest `zerb git init` as fix)

## Example Usage

### Scenario 1: Repair After Failure
```bash
# Initial setup failed
$ zerb init
...
⚠ Warning: Unable to initialize git repository
  Git versioning not available.
...

# Later, user fixes permissions
$ chmod 700 ~/.config/zerb
$ zerb git init
✓ Git repository initialized
✓ Initial commit created: "Initialize ZERB git repository (deferred setup)"
✓ Git user: ZERB User <zerb@localhost>

⚠ Note: Using placeholder git user
  Set ZERB_GIT_NAME and ZERB_GIT_EMAIL to use your identity.
```

### Scenario 2: Opt-In Version Control
```bash
# User initializes without git
$ zerb init --skip-git
✓ ZERB initialized at ~/.config/zerb
ℹ Git version control skipped

# User adds some configs
$ zerb config add node@20
$ zerb config add python@3.11

# User decides to enable git
$ export ZERB_GIT_NAME="Jane Doe"
$ export ZERB_GIT_EMAIL="jane@example.com"
$ zerb git init
✓ Git repository initialized
✓ Initial commit created with 3 existing configs
✓ Git user: Jane Doe <jane@example.com>
```

### Scenario 3: Idempotency Check
```bash
$ zerb git init
✓ Git repository initialized

$ zerb git init
Error: git repository already exists at ~/.config/zerb/.git

If you need to reinitialize git:
  1. Back up your ZERB directory
  2. Remove git: rm -rf ~/.config/zerb/.git
  3. Run: zerb git init
```

## Benefits

- **User-friendly recovery:** Clear path to fix git initialization failures
- **Opt-in flexibility:** Users can defer git until needed
- **Testing support:** CI/test environments can skip git initially
- **Reuses existing code:** Minimal new implementation (mostly orchestration)
- **Consistent behavior:** Same git setup as `zerb init`
- **Clear error messages:** Guides users to fix issues

## References

- Design Decision 5 from `setup-git-repository/design.md` (graceful degradation)
- `internal/git` package implementation (reused for deferred init)
- Future `zerb git` subcommands (namespace planning)

---

**Status:** Planning document  
**Created:** 2025-11-16  
**Next Step:** Validate demand during MVP usage, then create formal proposal
