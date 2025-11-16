# Design: `zerb config add` Command

## Context

ZERB aims to provide a unified, declarative environment management tool that wraps mature tools (mise for package management, chezmoi for config/dotfile management) with git-native versioning. Currently, users can initialize ZERB (`zerb init`) and add tools (`zerb add`), but there's no command to add configuration files to be tracked by chezmoi.

The `zerb config add` command needs to:
1. Integrate with chezmoi's isolated installation
2. Update the declarative Lua config file with new entries
3. Maintain ZERB's git-native timestamped versioning
4. Abstract away chezmoi details from the user

**Constraints:**
- Complete isolation: Never touch user's existing chezmoi installation
- Security-first: Validate all paths to prevent traversal attacks
- Git-native: All changes create timestamped configs and git commits
- User abstraction: Never expose "chezmoi" in user-facing messages

## Goals / Non-Goals

**Goals:**
- Provide a simple CLI to add config files/directories to ZERB tracking
- Support common use cases: dotfiles, config directories, templated files, secrets
- Maintain complete abstraction from chezmoi
- Generate appropriate git commits automatically
- Validate paths for security

**Non-Goals:**
- Auto-discovery of dotfiles (user explicitly specifies paths)
- Interactive file picker (use explicit path arguments)
- Editing configs (separate `zerb config edit` command, post-MVP)
- Removing configs (separate `zerb config remove` command, post-MVP)
- Chezmoi template syntax validation (delegated to chezmoi)

## Decisions

### Decision 1: Chezmoi Wrapper Package with Interface-Based Design

**What:** Create `internal/chezmoi/` package with interface-based design for testability and proper error abstraction.

**Why:**
- Centralize chezmoi isolation logic (--source, --config flags)
- Abstract away binary path and flag management
- Enable dependency injection for testing (accept interfaces, return structs)
- Provide proper error abstraction to hide chezmoi from user-facing messages
- Support context for cancellation and timeouts
- Parallel to existing `internal/binary/` pattern

**Alternatives considered:**
- Call chezmoi directly in command code → Harder to test, duplicates isolation logic
- Use chezmoi as a library → Chezmoi doesn't provide a Go library interface
- Concrete types only → Harder to test, violates Go best practices

**Implementation:**
```go
// internal/chezmoi/chezmoi.go
package chezmoi

import (
    "context"
    "fmt"
    "os/exec"
    "path/filepath"
    "strings"
)

// AddOptions configures the behavior of adding a config file
type AddOptions struct {
    Recursive bool
    Template  bool
    Secrets   bool
    Private   bool
}

// Chezmoi is the interface for chezmoi operations (accept interfaces, return structs)
type Chezmoi interface {
    Add(ctx context.Context, path string, opts AddOptions) error
}

// Client implements the Chezmoi interface
type Client struct {
    bin  string // ~/.config/zerb/bin/chezmoi
    src  string // ~/.config/zerb/chezmoi/source
    conf string // ~/.config/zerb/chezmoi/config.toml
}

// NewClient creates a new chezmoi client for the given ZERB directory
func NewClient(zerbDir string) *Client {
    return &Client{
        bin:  filepath.Join(zerbDir, "bin", "chezmoi"),
        src:  filepath.Join(zerbDir, "chezmoi", "source"),
        conf: filepath.Join(zerbDir, "chezmoi", "config.toml"),
    }
}

// Add adds a config file to chezmoi's source directory
func (c *Client) Add(ctx context.Context, path string, opts AddOptions) error {
    args := []string{
        "--source", c.src,
        "--config", c.conf,
        "add",
    }
    
    if opts.Template {
        args = append(args, "--template")
    }
    if opts.Recursive {
        args = append(args, "--recursive")
    }
    if opts.Secrets {
        args = append(args, "--encrypt") // Map to chezmoi equivalent
    }
    if opts.Private {
        args = append(args, "--private") // or appropriate flag for chmod 600
    }
    args = append(args, path)
    
    cmd := exec.CommandContext(ctx, c.bin, args...)
    
    // Scrub environment for complete isolation
    cmd.Env = []string{
        "HOME=" + os.Getenv("HOME"),
        // Explicitly prevent chezmoi from reading user's config
    }
    
    out, err := cmd.CombinedOutput()
    if err != nil {
        return translateChezmoiError(err, string(out))
    }
    return nil
}

// translateChezmoiError maps chezmoi errors to user-friendly ZERB errors
// This ensures we never expose "chezmoi" in user-facing messages
func translateChezmoiError(err error, stderr string) error {
    // Map common errors to user-friendly messages
    if strings.Contains(stderr, "no such file") {
        return fmt.Errorf("%w: file not found", ErrChezmoiInvocation)
    }
    if strings.Contains(stderr, "permission denied") {
        return fmt.Errorf("%w: permission denied", ErrChezmoiInvocation)
    }
    if strings.Contains(stderr, "is a directory") {
        return fmt.Errorf("%w: path is a directory (use --recursive)", ErrChezmoiInvocation)
    }
    
    // Generic fallback - redact sensitive info but preserve useful context
    sanitized := redactSensitiveInfo(stderr)
    return fmt.Errorf("%w: %s", ErrChezmoiInvocation, sanitized)
}

// Error types for user-facing errors
var (
    ErrInvalidPath              = errors.New("invalid path")
    ErrDirectoryRequiresRecursive = errors.New("directory requires --recursive flag")
    ErrChezmoiInvocation        = errors.New("failed to add configuration file")
    ErrTransactionExists        = errors.New("another configuration operation is in progress")
)
```

### Decision 2: Command Structure - Subcommand Pattern

**What:** Use `zerb config add` as a two-level subcommand (not `zerb add-config`).

**Why:**
- Consistent with future `zerb config` operations (`list`, `remove`, `edit`, `diff`)
- Groups related operations under a namespace
- Matches user mental model: "config operations"

**Alternatives considered:**
- `zerb add-config` → Doesn't scale for future config operations
- `zerb add --config` → Conflicts with `zerb add <tool>` pattern

**Implementation:** Modify `cmd/zerb/main.go` to handle two-level subcommands:
```go
case "config":
    if len(os.Args) < 3 {
        // show config subcommand help
        return
    }
    switch os.Args[2] {
    case "add":
        runConfigAdd(os.Args[3:])
    // future: case "list", "remove", etc.
    }
```

### Decision 3: Config Update Flow - Read, Modify, Write with Timestamping

**What:** Follow the same pattern as tool addition:
1. Parse current active config
2. Add new ConfigFile entry
3. Generate new Lua config
4. Write timestamped config file to `configs/`
5. Update `.zerb-active` marker
6. Commit to git

**Why:**
- Consistent with existing `zerb add` behavior
- Maintains immutable timestamped configs
- Enables rollback and history
- Already has tested infrastructure (`internal/config`)

**Migration:** None needed - this is a new command.

### Decision 4: Duplicate Detection Strategy - Warn and Skip

**What:** If a config path is already tracked, warn the user and skip adding it (don't error).

**Why:**
- User-friendly: Doesn't break workflows if user re-runs command
- Idempotent: Same command can be run multiple times safely
- Clear feedback: User knows the path is already tracked

**Alternatives considered:**
- Error and exit → Breaks idempotency, annoying for users
- Silently skip → User might think command failed
- Update flags on existing entry → Complex, unclear semantics

**Implementation:**
```go
for _, existing := range config.Configs {
    if existing.Path == normalizedPath {
        fmt.Printf("⚠ Config already tracked: %s\n", path)
        return nil
    }
}
```

### Decision 5: Path Validation - Fix Security Flaws and Strengthen Validation

**What:** Fix critical security flaws in `internal/config/types.go` `validateConfigPath()` and add robust validation.

**Why:**
- **CRITICAL:** Current implementation has path traversal vulnerabilities (uses `strings.Contains(cleaned, "..")` which is insecure)
- Current implementation incorrectly rejects absolute paths inside `$HOME`
- Need canonical path checking with symlink resolution to prevent escapes from home directory
- Consistent validation across parsing and command line

**Security Issues in Current Code (`internal/config/types.go:216-227`):**
1. Line 216-219: Absolute path check rejects valid paths like `/home/user/.zshrc` even though they're inside `$HOME`
2. Line 221-224: `strings.Contains(cleaned, "..")` is fundamentally flawed:
   - Flags legitimate paths containing `..` (e.g., `~/.config/..something`)
   - Misses normalized traversal attacks
   - Doesn't handle symlink escapes

**Fixed Implementation:**
```go
func validateConfigPath(path string) error {
    if path == "" {
        return fmt.Errorf("path cannot be empty")
    }
    
    // Get home directory
    home, err := os.UserHomeDir()
    if err != nil {
        return fmt.Errorf("cannot determine home directory: %w", err)
    }
    homeAbs := filepath.Clean(home)
    
    // Expand and normalize the path
    var absPath string
    if strings.HasPrefix(path, "~/") {
        absPath = filepath.Join(home, path[2:])
    } else if filepath.IsAbs(path) {
        absPath = path
    } else {
        return fmt.Errorf("%w: must be absolute or start with ~/", ErrInvalidPath)
    }
    
    // Clean and resolve symlinks for canonical path
    absPath = filepath.Clean(absPath)
    
    // Try to resolve symlinks (allow non-existent paths)
    evalPath, err := filepath.EvalSymlinks(absPath)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("cannot evaluate path: %w", err)
    }
    if err == nil {
        absPath = evalPath // Use resolved path if it exists
    }
    
    // Verify the path is within home directory
    if !strings.HasPrefix(absPath, homeAbs+string(filepath.Separator)) && absPath != homeAbs {
        return fmt.Errorf("%w: absolute paths outside home directory not allowed: %s", ErrInvalidPath, path)
    }
    
    // Use filepath.Rel to verify no directory traversal
    rel, err := filepath.Rel(homeAbs, absPath)
    if err != nil || strings.HasPrefix(rel, "..") {
        return fmt.Errorf("%w: path traversal not allowed: %s", ErrInvalidPath, path)
    }
    
    return nil
}
```

**Additional validation for config add command:**
- Check if path exists on filesystem (error for non-existent paths - see Decision 8)
- Detect directories and require `--recursive` flag
- Normalize paths for duplicate detection

### Decision 6: User Confirmation - Show Diff, Prompt to Apply

**What:** Show a preview of config changes and prompt user to confirm (similar to `zerb add` pattern).

**Why:**
- User visibility into what's changing
- Prevents accidental adds
- Matches established UX pattern from MVP roadmap

**Optional:** Support `--yes` / `-y` flag to skip confirmation for scripting.

### Decision 7: Context Support Throughout

**What:** Use `context.Context` for all blocking operations to support cancellation and timeouts.

**Why:**
- Idiomatic Go: all blocking operations should accept context
- Enables user to cancel long-running operations (Ctrl+C)
- Allows setting timeouts to prevent hangs
- Required for proper resource cleanup
- Testability: can test timeout and cancellation scenarios

**Implementation:**
```go
func runConfigAdd(args []string) error {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    // Pass context to all blocking operations
    for _, path := range paths {
        if err := chezmoiClient.Add(ctx, path, opts); err != nil {
            return err
        }
        // Update transaction state
        if err := txn.UpdateState(ctx, path, "completed"); err != nil {
            return err
        }
    }
    
    // Commit to git with context
    if err := gitClient.Commit(ctx, msg, body); err != nil {
        return err
    }
    
    return nil
}
```

### Decision 8: Non-Existent Path Behavior - Fail Fast

**What:** Reject non-existent paths and do not proceed with commit until all paths exist and are successfully added.

**Why:**
- **Simpler semantics:** Clear "all or nothing" behavior - either all paths are added or none are committed
- **Clearer user expectations:** User knows immediately if a path doesn't exist
- **Atomic behavior:** Maintains transaction integrity - only commit when all operations succeed
- **Easier to implement and test:** No special cases for "skipped" vs "completed" states
- **Cross-machine scenarios already handled:** Machine-specific profiles (Component 08) address files that exist on some machines but not others

**Alternatives considered:**
- Warn but allow non-existent paths → Creates confusion about what "added to config" means when chezmoi can't track it
- Skip chezmoi add but add to config anyway → Violates atomic commit requirement

**Implementation:**
- Validate path existence before calling chezmoi
- If path doesn't exist, return clear error: `Error: Path does not exist: ~/.config/nonexistent`
- Do not create transaction entry or modify config
- For batch operations, validate all paths upfront before starting transaction

**Exception:** Non-existent paths may be allowed with explicit `--allow-missing` flag (post-MVP) for advanced use cases.

### Decision 9: Interface-Based Design for Testability

**What:** Use interface-based design with dependency injection for all external dependencies.

**Why:**
- Idiomatic Go: "Accept interfaces, return structs"
- Enables comprehensive testing with mocks/stubs
- Decouples command logic from implementation details
- Makes code more maintainable and flexible

**Interfaces to define:**
```go
// Chezmoi operations (defined in Decision 1)
type Chezmoi interface {
    Add(ctx context.Context, path string, opts AddOptions) error
}

// Git operations
type Git interface {
    Stage(ctx context.Context, files ...string) error
    Commit(ctx context.Context, msg, body string) error
}

// Clock for deterministic timestamps in tests
type Clock interface {
    Now() time.Time
}

// Service layer that composes interfaces
type ConfigAddService struct {
    chezmoi Chezmoi
    git     Git
    config  *config.Parser
    clock   Clock
    zerbDir string
}
```

**Benefits:**
- Test the command logic by injecting mock implementations
- No need to shell out to actual binaries in unit tests
- Can simulate error conditions easily
- Deterministic tests (clock interface)

## Risks / Trade-offs

### Risk 1: Chezmoi Binary Not Yet Implemented
**Mitigation:** The binary manager already downloads chezmoi during `zerb init` (Component 03 is complete). The wrapper just needs to invoke it correctly.

### Risk 2: Chezmoi Source Directory Structure
**Trade-off:** ZERB relies on chezmoi's naming conventions (dot_ prefix, etc.). This is acceptable because:
- Chezmoi is stable and widely used
- The wrapper abstracts this from users
- Users can inspect `~/.config/zerb/chezmoi/source/` if needed

### Risk 3: Multiple Files in One Command
**Decision:** Support `zerb config add <path1> <path2> ...` to add multiple files.
**Mitigation:** Process each path individually, collect errors, show summary at the end.

### Risk 4: Path Existence Validation
**Trade-off:** Should we error if path doesn't exist?
**Decision:** Error and fail fast (see Decision 8). Do not proceed with commit until all paths exist.

### Risk 5: Transaction Atomicity and Concurrency
**Challenge:** Multi-step operation (validate → chezmoi → config update → git commit) can fail at any step, leaving inconsistent state. Concurrent invocations could corrupt transaction state.

**Comprehensive Mitigation Strategy:**

**1. Transaction File Location and Permissions:**
- Location: `~/.config/zerb/tmp/txn-config-add-<uuid>.json` (not root of zerb dir)
- Permissions: 0600 (file), tmp directory 0700
- Why: Aligns with project conventions, prevents information leakage, proper isolation

**2. Locking Mechanism:**
```go
// Acquire exclusive lock before starting transaction
lockPath := filepath.Join(zerbDir, "tmp", "config-add.lock")
lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
if err != nil {
    if os.IsExist(err) {
        return ErrTransactionExists // "Another operation is in progress"
    }
    return fmt.Errorf("create lock: %w", err)
}
defer func() {
    lockFile.Close()
    os.Remove(lockPath)
}()
```

**Lock file details:**
- Global lock prevents concurrent `zerb config add` operations
- Lock released on normal completion or panic (via defer)
- Optional: `--force-stale-lock` flag to override locks older than 10 minutes
- Lock file contains PID and timestamp for debugging

**3. Atomic Transaction Writes:**
```go
func writeTxnAtomic(dir, name string, t *ConfigAddTxn) error {
    data, err := json.MarshalIndent(t, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    
    // Write to temporary file
    tmp := filepath.Join(dir, name+".tmp")
    if err := os.WriteFile(tmp, data, 0600); err != nil {
        return fmt.Errorf("write temp: %w", err)
    }
    
    // Atomic rename (only operation that's atomic on POSIX)
    final := filepath.Join(dir, name)
    if err := os.Rename(tmp, final); err != nil {
        return fmt.Errorf("rename: %w", err)
    }
    
    // Sync directory for durability
    d, _ := os.Open(dir)
    if d != nil {
        d.Sync()
        d.Close()
    }
    
    return nil
}
```

**4. Enhanced Transaction Schema with Artifact Tracking:**
```json
{
  "version": 1,
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "operation": "config-add",
  "timestamp": "2025-01-15T14:30:00Z",
  "paths": [
    {
      "path": "~/.zshrc",
      "state": "completed",
      "recursive": false,
      "template": false,
      "secrets": false,
      "private": false,
      "created_source_files": [
        "~/.config/zerb/chezmoi/source/dot_zshrc"
      ],
      "last_error": null
    },
    {
      "path": "~/.config/nvim",
      "state": "pending",
      "recursive": true,
      "template": false,
      "secrets": false,
      "private": false,
      "created_source_files": [],
      "last_error": null
    }
  ],
  "config_updated": false,
  "git_committed": false
}
```

**Key schema additions:**
- `version`: Schema version for future evolution
- `id`: UUID for unique transaction identification
- `created_source_files`: Track artifacts for automatic cleanup on abort
- `last_error`: Error context for resume/debugging
- State transitions: `pending` → `in_progress` → `completed` | `failed`

**5. Abort Behavior with Automatic Cleanup:**
```bash
$ zerb config add --abort

Aborting transaction 550e8400...
Cleaning up added files:
  ✓ Removed ~/.config/zerb/chezmoi/source/dot_zshrc
  ✓ Removed transaction file
  ✓ Released lock

Transaction aborted successfully.
```

**If cleanup fails:**
```
⚠ Could not remove some files automatically:
  - ~/.config/zerb/chezmoi/source/dot_config_nvim/init.lua (permission denied)

Manual cleanup required:
  rm -rf ~/.config/zerb/chezmoi/source/dot_config_nvim
```

**6. Resume Behavior:**
- `--resume` continues from saved transaction state
- Skips `completed` paths (idempotent)
- Retries `failed` or `pending` paths
- Only creates git commit after ALL paths succeed

**Benefits:**
- **Atomic commits:** All files in single git commit or none
- **Safe interruption recovery:** Ctrl+C, crashes, network failures all handled
- **Concurrency safety:** Lock prevents corruption from parallel invocations
- **Automatic cleanup:** Abort removes artifacts, no manual intervention needed
- **Durability:** Fsync ensures transaction state persists across crashes
- **Clear state visibility:** User can inspect transaction file to see progress

## Migration Plan

N/A - This is a new command with no existing behavior to migrate.

## Testing Strategy

1. **Unit Tests:**
   - Argument parsing
   - Path validation (including security tests)
   - Path normalization for duplicate detection
   - Duplicate detection
   - Config update logic
   - Error type mapping and abstraction

2. **Integration Tests:**
   - Chezmoi wrapper with stubbed binary
   - Git operations (commit generation)
   - Interface mocking (Chezmoi, Git, Clock)
   - End-to-end: init → config add → verify config file and git commit

3. **Path Validation Security Tests:**
   - Path traversal attempts (e.g., `~/../etc/passwd`)
   - Absolute paths inside home (should pass)
   - Absolute paths outside home (should fail)
   - Symlinks pointing outside home (should fail)
   - Symlinks inside home (should pass)
   - Paths with `..` in legitimate names (e.g., `~/.config/..something`)
   - Tilde expansion
   - Directory detection
   - Case sensitivity (macOS)

4. **Error Cases:**
   - Invalid paths (path traversal, outside home)
   - Directory without `--recursive` flag
   - Non-existent paths (should error with Decision 8)
   - Permission errors
   - Chezmoi errors (verify abstraction - no "chezmoi" in user messages)

5. **Transaction Tests:**
   - Transaction file creation with UUID and versioned schema
   - Atomic transaction writes (write + rename pattern)
   - Lock file acquisition and release
   - Concurrent invocation prevention (lock conflict)
   - Resume from interrupted operation
   - Abort transaction with automatic cleanup
   - Multiple paths with partial failures
   - Idempotent resume (skip completed paths)
   - Context cancellation mid-operation
   - Transaction state persistence across crashes

6. **Concurrency Tests:**
   - Run with `go test -race` to detect race conditions
   - Simulate concurrent `zerb config add` invocations
   - Verify lock prevents corruption
   - Test stale lock detection and `--force-stale-lock`

7. **Context Tests:**
   - Context cancellation (Ctrl+C simulation)
   - Timeout handling
   - Verify transaction state saved before exit on cancellation

## Open Questions

1. **Should we support glob patterns?** (e.g., `zerb config add ~/.config/nvim/*.lua`)
   - **Decision:** No for MVP. Users can add parent directory with `--recursive`.

2. **Should we auto-detect if a path is a directory and set recursive=true?**
   - **Decision:** No. Directories MUST use explicit `--recursive` flag. Non-recursive directory tracking is not supported.
   - **Reason:** Explicit is better than implicit - prevents accidentally tracking large directories, makes intent clear in command history.
   - **Error message:** When user tries to add a directory without `--recursive`, show helpful error:
     ```
     Error: ~/.config/nvim is a directory.
     Use --recursive to track it and its contents.
     
     Example:
       zerb config add ~/.config/nvim --recursive
     ```

3. **Should we support removing a config in the same command?** (e.g., `zerb config add --remove ~/.zshrc`)
   - **Decision:** No. Use separate `zerb config remove` command (post-MVP).

4. **How to handle config files that exist in multiple machines?**
   - **Answer:** This is what machine-specific profiles (Component 08) handles. MVP supports baseline configs shared across all machines.

5. **How to handle non-existent paths?**
   - **Decision:** Error and do not proceed with commit until all paths exist (see Decision 8).
   - **Reason:** Simpler semantics with clear "all or nothing" behavior. Atomic commit integrity maintained.
   - **Behavior:** Validate path existence before starting transaction. Return clear error if path doesn't exist.
   - **Cross-machine scenarios:** Handled by machine-specific profiles (Component 08), not by allowing non-existent paths in baseline.
   - **Future:** May add `--allow-missing` flag for advanced use cases (post-MVP).

6. **What happens when chezmoi source already has the file?** (e.g., from a previous operation or pull)
   - **Decision:** Follow chezmoi's default behavior (overwrite with current version from filesystem).
   - **Note:** This is unlikely in normal usage since ZERB is the only interface to chezmoi. Users won't be manually modifying `~/.config/zerb/chezmoi/source/`.
   - **Edge case:** If user pulls from another machine and then runs `zerb config add` for the same file, chezmoi will update the source with the local version.

7. **Should we support chezmoi's `--exact` flag for exact directory tracking?**
   - **Decision:** No for MVP. This is an advanced use case.
   - **Reason:** The `--exact` flag makes chezmoi remove files from the destination that aren't in the source. This could surprise users and cause data loss.
   - **Future:** Users can manually configure exact mode in chezmoi's config if needed (post-MVP feature).

8. **How to handle partial failures during multi-step operation?**
   - **Decision:** Implement transaction-based resume using transaction files (similar to drift detection).
   - **Flow:**
     1. Create transaction file with list of paths to add
     2. For each path: validate → add to chezmoi → update transaction state
     3. After all paths succeed: update zerb.lua → create git commit → delete transaction
     4. On interruption: transaction file remains for resume
   - **Resume behavior:** 
     - `zerb config add --resume` continues from transaction file
     - Skips already-completed paths (idempotent)
     - Retries failed paths
     - Completes config update and git commit once all paths succeed
   - **Abort behavior:**
     - `zerb config add --abort` removes transaction file
     - Provides instructions for manual rollback if needed (remove files from chezmoi source)
   - **Benefits:**
     - Multiple configs added in single atomic commit
     - Safe recovery from interruptions
     - Consistent state across chezmoi + zerb.lua + git
