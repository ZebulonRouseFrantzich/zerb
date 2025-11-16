# Code Review Fixes

**Status:** Post-implementation review findings from @code-reviewer and @golang-pro
**Date:** 2025-11-16
**Reviewers:** code-reviewer subagent, golang-pro subagent

This document tracks fixes required based on code review of the initial implementation.

---

## Critical Priority Fixes

### CR-C1: Integrate Transaction System with Locking
**Status:** ❌ Not Started
**Files:** `internal/service/config_add.go`, `cmd/zerb/config_add.go`
**Issue:** Transaction and lock packages exist but are never used in the service layer.

**Current State:**
- No lock acquisition before operations
- No transaction state tracking
- No recovery from interruptions
- Concurrent operations can conflict

**Required Changes:**
```go
// In ConfigAddService.Execute():
func (s *ConfigAddService) Execute(ctx context.Context, req AddRequest) (*AddResult, error) {
    // 1. Acquire lock
    lockDir := filepath.Join(s.zerbDir, "tmp")
    lock, err := transaction.AcquireLock(lockDir)
    if err != nil {
        return nil, fmt.Errorf("acquire lock: %w", err)
    }
    defer lock.Release()
    
    // 2. Create transaction
    opts := make(map[string]transaction.AddOptions)
    for path, opt := range req.Options {
        opts[path] = transaction.AddOptions{
            Recursive: opt.Recursive,
            Template:  opt.Template,
            Secrets:   opt.Secrets,
            Private:   opt.Private,
        }
    }
    txn := transaction.New(req.Paths, opts)
    txnDir := filepath.Join(s.zerbDir, "tmp")
    defer func() {
        if saveErr := txn.Save(txnDir); saveErr != nil {
            // Log error but don't override return error
        }
    }()
    
    // 3. Process each path with state tracking
    for _, path := range result.AddedPaths {
        // Update state to in_progress
        txn.UpdatePathState(path, transaction.StateInProgress, nil, nil)
        txn.Save(txnDir)
        
        // Add to chezmoi
        if err := s.chezmoi.Add(ctx, path, chezmoiOpts); err != nil {
            txn.UpdatePathState(path, transaction.StateFailed, nil, err)
            txn.Save(txnDir)
            return nil, fmt.Errorf("add %q: %w", path, err)
        }
        
        // Mark completed
        txn.UpdatePathState(path, transaction.StateCompleted, createdFiles, nil)
        txn.Save(txnDir)
    }
    
    // 4. Only commit if ALL paths succeeded
    if !txn.AllPathsCompleted() {
        return nil, fmt.Errorf("not all paths completed successfully")
    }
    
    // ... continue with config update and git commit
    
    // 5. Clean up transaction on success
    os.Remove(filepath.Join(txnDir, fmt.Sprintf("txn-config-add-%s.json", txn.ID)))
    
    return result, nil
}
```

**Tests Required:**
- [ ] Test lock acquisition and release
- [ ] Test transaction state persistence
- [ ] Test concurrent operation prevention (lock conflict)
- [ ] Test transaction cleanup on success
- [ ] Test state tracking per path

---

### CR-C2: Fix Path Validation Symlink Escape Vulnerability
**Status:** ❌ Not Started
**File:** `internal/config/types.go`, lines 264-273
**Severity:** CRITICAL SECURITY ISSUE

**Vulnerability:**
Current code allows symlink resolution errors (except `IsNotExist`) to bypass validation, enabling:
- Permission errors to pass through
- Broken symlinks to be accepted
- Race conditions in TOCTOU attacks

**Attack Scenario:**
```bash
ln -s /etc/passwd ~/.config/evil
zerb config add ~/.config/evil  # May bypass validation
```

**Current Vulnerable Code:**
```go
evalPath, err := filepath.EvalSymlinks(absPath)
if err != nil && !os.IsNotExist(err) {
    return fmt.Errorf("cannot evaluate path: %w", err)
}
if err == nil {
    absPath = evalPath  // Only updates if no error
}
// Continues with potentially unresolved path - VULNERABLE!
```

**Required Fix:**
```go
// For existing paths: MUST resolve and validate
evalPath, err := filepath.EvalSymlinks(absPath)
if err != nil {
    if os.IsNotExist(err) {
        // Non-existent path: validate parent directory is within home
        parent := filepath.Dir(absPath)
        parentEval, err := filepath.EvalSymlinks(parent)
        if err != nil {
            return fmt.Errorf("cannot validate parent directory: %w", err)
        }
        // Check parent is within home
        if !strings.HasPrefix(parentEval, homeAbs+string(filepath.Separator)) && parentEval != homeAbs {
            return fmt.Errorf("%w: parent directory outside home", ErrInvalidPath)
        }
        // Use cleaned path for non-existent file
        absPath = filepath.Clean(absPath)
    } else {
        // Other errors (permission, broken symlink, etc.) MUST fail
        return fmt.Errorf("cannot resolve path (may be broken symlink or permission issue): %w", err)
    }
} else {
    // Path exists and symlinks resolved successfully
    absPath = evalPath
}
```

**Tests Required:**
- [ ] Test symlink pointing outside home (must fail)
- [ ] Test broken symlink (must fail)
- [ ] Test symlink with permission errors (must fail)
- [ ] Test symlink within home pointing to valid target (must pass)
- [ ] Test non-existent path with parent outside home (must fail)
- [ ] Test non-existent path with parent in home (must pass)
- [ ] Add TOCTOU race condition test

---

### CR-C3: Fix CLI Flag Parsing to Return Errors
**Status:** ❌ Not Started
**File:** `cmd/zerb/config_add.go`, func `runConfigAdd`

**Issue:** Using `flag.ExitOnError` calls `os.Exit()` from library code, breaking composability and testability.

**Current Code:**
```go
fs := flag.NewFlagSet("config add", flag.ExitOnError)
```

**Required Fix:**
```go
fs := flag.NewFlagSet("config add", flag.ContinueOnError)
fs.SetOutput(io.Discard) // Suppress default error output

// ... define flags ...

if err := fs.Parse(args); err != nil {
    if err == flag.ErrHelp {
        fs.SetOutput(os.Stderr)
        fs.Usage()
        return nil // Help requested, not an error
    }
    return fmt.Errorf("parse flags: %w", err)
}
```

**Tests Required:**
- [ ] Test flag parsing errors return error instead of exiting
- [ ] Test --help returns nil and prints usage
- [ ] Test invalid flags return wrapped errors

---

## High Priority Fixes

### CR-H1: Use Correct Initialization Marker
**Status:** ❌ Not Started
**File:** `cmd/zerb/config_add.go`

**Issue:** Checks `zerb.lua.active` instead of authoritative `.zerb-active` marker.

**Current Code:**
```go
if _, err := os.Stat(filepath.Join(zerbDir, "zerb.lua.active")); err != nil {
    return fmt.Errorf("ZERB not initialized. Run 'zerb init' first")
}
```

**Required Fix:**
```go
// Check authoritative marker
activeMarker := filepath.Join(zerbDir, ".zerb-active")
markerData, err := os.ReadFile(activeMarker)
if err != nil {
    if os.IsNotExist(err) {
        return fmt.Errorf("ZERB not initialized. Run 'zerb init' first")
    }
    return fmt.Errorf("read active marker: %w", err)
}

// Read and validate active config path
activeFilename := strings.TrimSpace(string(markerData))
if activeFilename == "" {
    return fmt.Errorf("active marker is empty - corrupted state")
}

// Verify active config exists
activeConfigPath := filepath.Join(zerbDir, "configs", activeFilename)
if _, err := os.Stat(activeConfigPath); err != nil {
    return fmt.Errorf("active config %q not found: %w", activeFilename, err)
}
```

---

### CR-H2: Implement Atomic Active Config Update
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`, lines 194-200

**Issue:** Non-atomic remove + symlink creates race condition window.

**Current Code:**
```go
os.Remove(activeConfigPath)
if err := os.Symlink(filepath.Join("configs", newConfigFilename), activeConfigPath); err != nil {
    // Fallback
}
```

**Required Fix:**
```go
// Create symlink to temp location first
tmpLink := activeConfigPath + ".tmp"
target := filepath.Join("configs", newConfigFilename)

if err := os.Symlink(target, tmpLink); err != nil {
    // Check if it's truly unsupported or another error
    if strings.Contains(err.Error(), "not supported") || strings.Contains(err.Error(), "not implemented") {
        // Fallback to copy on systems without symlink support
        if err := os.WriteFile(activeConfigPath, []byte(newConfigContent), 0644); err != nil {
            return nil, fmt.Errorf("update active config: %w", err)
        }
    } else {
        return nil, fmt.Errorf("create symlink: %w", err)
    }
} else {
    // Atomic rename (overwrites existing)
    if err := os.Rename(tmpLink, activeConfigPath); err != nil {
        os.Remove(tmpLink) // Clean up temp
        return nil, fmt.Errorf("update active config link: %w", err)
    }
}
```

---

### CR-H3: Extend Git Interface to Capture Commit Hash
**Status:** ❌ Not Started
**Files:** `internal/git/git.go`, `internal/service/config_add.go`

**Issue:** Spec requires commit hash in metadata but git.Commit() doesn't return it.

**Required Changes:**

1. **Update Git interface:**
```go
// In internal/git/git.go
type Git interface {
    Stage(ctx context.Context, files ...string) error
    Commit(ctx context.Context, msg, body string) error
    CommitHash(ctx context.Context, msg, body string) (hash string, err error) // NEW
    GetLastCommitHash(ctx context.Context) (string, error) // NEW - for existing commits
}
```

2. **Implement in Client:**
```go
func (c *Client) CommitHash(ctx context.Context, msg, body string) (string, error) {
    if err := c.Commit(ctx, msg, body); err != nil {
        return "", err
    }
    return c.GetLastCommitHash(ctx)
}

func (c *Client) GetLastCommitHash(ctx context.Context) (string, error) {
    cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
    cmd.Dir = c.repoPath
    
    out, err := cmd.Output()
    if err != nil {
        return "", translateGitError(err, string(out))
    }
    
    hash := strings.TrimSpace(string(out))
    if len(hash) != 40 { // SHA-1 is 40 hex chars
        return "", fmt.Errorf("invalid commit hash: %q", hash)
    }
    
    return hash, nil
}
```

3. **Update service to use it:**
```go
// In Execute(), replace:
if err := s.git.Commit(ctx, commitMsg, commitBody); err != nil {
    return nil, fmt.Errorf("create commit: %w", err)
}

// With:
commitHash, err := s.git.CommitHash(ctx, commitMsg, commitBody)
if err != nil {
    return nil, fmt.Errorf("create commit: %w", err)
}
result.CommitHash = commitHash

// Also regenerate config with commit hash
newFilename, newContent, err := s.generator.GenerateTimestamped(ctx, currentConfig, commitHash)
// ... update the config file with commit metadata
```

**Tests Required:**
- [ ] Test CommitHash returns valid SHA-1
- [ ] Test GetLastCommitHash returns correct hash
- [ ] Test error handling when git rev-parse fails
- [ ] Test commit hash is populated in AddResult

---

### CR-H4: Fix Directory Validation and Option Lookup
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`, lines 90-96

**Issue:** Uses original path as key after normalization, causing lookup failures for absolute paths.

**Current Code:**
```go
if info, _ := os.Stat(normalized); info != nil && info.IsDir() {
    opts := req.Options[path]  // Uses original path - may fail
    if !opts.Recursive {
        return nil, fmt.Errorf("path is a directory, use --recursive flag: %s", path)
    }
}
```

**Required Fix:**
```go
// Single os.Stat, check errors
info, err := os.Stat(normalized)
if err != nil {
    return nil, fmt.Errorf("stat %q: %w", path, err)
}

// Verify file is readable
if file, err := os.Open(normalized); err != nil {
    return nil, fmt.Errorf("cannot read %q: %w", path, err)
} else {
    file.Close()
}

// Check if directory
if info.IsDir() {
    opts := req.Options[path]
    if !opts.Recursive {
        return nil, fmt.Errorf(`%s is a directory.
Use --recursive to track it and its contents.

Example:
  zerb config add %s --recursive`, path, path)
    }
}
```

---

### CR-H5: Fix Duplicate Detection Error Handling
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`, lines 115-128

**Issue:** Silently ignores normalization errors for existing config entries.

**Current Code:**
```go
for _, existing := range currentConfig.Configs {
    existingNorm, _ := config.NormalizeConfigPath(existing.Path) // Error ignored!
    if existingNorm == normalized {
        isDuplicate = true
```

**Required Fix:**
```go
for _, existing := range currentConfig.Configs {
    existingNorm, err := config.NormalizeConfigPath(existing.Path)
    if err != nil {
        // Malformed existing entry - log warning and skip comparison
        fmt.Fprintf(os.Stderr, "Warning: cannot normalize existing config path %q: %v\n", existing.Path, err)
        continue
    }
    if existingNorm == normalized {
        isDuplicate = true
        result.SkippedPaths = append(result.SkippedPaths, origPath)
        break
    }
}
```

---

### CR-H6: Implement Error Recovery and Rollback
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`

**Issue:** After chezmoi adds files, failures leave orphaned files with no cleanup.

**Required Fix:** Use transaction tracking to enable cleanup:

```go
// Track created files in transaction
for _, path := range result.AddedPaths {
    opts := req.Options[path]
    chezmoiOpts := chezmoi.AddOptions{ /* ... */ }
    
    // Before adding
    txn.UpdatePathState(path, transaction.StateInProgress, nil, nil)
    txn.Save(txnDir)
    
    if err := s.chezmoi.Add(ctx, path, chezmoiOpts); err != nil {
        txn.UpdatePathState(path, transaction.StateFailed, nil, err)
        txn.Save(txnDir)
        
        // Provide rollback instructions
        return nil, fmt.Errorf(`failed to add %q: %w

To recover:
  1. Review transaction state: cat %s/txn-config-add-%s.json
  2. Abort transaction: zerb config add --abort
  3. Or resume: zerb config add --resume`, path, err, txnDir, txn.ID)
    }
    
    // Track created chezmoi files (would need to query chezmoi source dir)
    createdFiles := []string{ /* list of created files */ }
    txn.UpdatePathState(path, transaction.StateCompleted, createdFiles, nil)
    txn.Save(txnDir)
}
```

---

## Medium Priority Fixes

### CR-M1: Use Multiple -m Flags for Commit Message
**Status:** ❌ Not Started
**File:** `internal/git/git.go`, func `Commit`

**Current:**
```go
fullMsg := msg
if body != "" {
    fullMsg = msg + "\n\n" + body
}
args := []string{"commit", "-m", fullMsg}
```

**Fix:**
```go
args := []string{"commit", "-m", msg}
if body != "" {
    args = append(args, "-m", body)
}
```

---

### CR-M2: Define Parser/Generator Interfaces in Service
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`

**Issue:** Concrete types limit testability.

**Required:**
```go
// In internal/service/config_add.go
type ConfigParser interface {
    ParseString(ctx context.Context, lua string) (*config.Config, error)
}

type ConfigGenerator interface {
    GenerateTimestamped(ctx context.Context, cfg *config.Config, gitCommit string) (filename, content string, err error)
}

type ConfigAddService struct {
    chezmoi   chezmoi.Chezmoi
    git       git.Git
    parser    ConfigParser  // Interface, not *config.Parser
    generator ConfigGenerator  // Interface, not *config.Generator
    clock     Clock
    zerbDir   string
}
```

---

### CR-M3: Fix Transaction Dir Sync and Variable Shadowing
**Status:** ❌ Not Started
**File:** `internal/transaction/transaction.go`, func `Save`

**Current:**
```go
// Sync directory for durability
if dir, err := os.Open(dir); err == nil {  // Shadows parameter!
    dir.Sync()
    dir.Close()
}
```

**Fix:**
```go
// Sync directory for durability
if df, err := os.Open(dir); err == nil {
    if err := df.Sync(); err != nil {
        df.Close()
        return fmt.Errorf("sync directory: %w", err)
    }
    df.Close()
}
```

---

### CR-M4: Improve Error Redaction in Chezmoi
**Status:** ❌ Not Started
**File:** `internal/chezmoi/chezmoi.go`, func `redactSensitiveInfo`

**Current:** Only removes "chezmoi" word, not paths.

**Fix:**
```go
func redactSensitiveInfo(msg string) string {
    // Limit length
    const maxLen = 200
    if len(msg) > maxLen {
        msg = msg[:maxLen] + "..."
    }
    
    // Redact the word "chezmoi"
    msg = strings.ReplaceAll(msg, "chezmoi", "config manager")
    msg = strings.ReplaceAll(msg, "Chezmoi", "Config Manager")
    msg = strings.ReplaceAll(msg, "CHEZMOI", "CONFIG MANAGER")
    
    // Redact absolute paths that might contain usernames
    home, _ := os.UserHomeDir()
    if home != "" {
        msg = strings.ReplaceAll(msg, home, "$HOME")
    }
    
    // Redact /home/username patterns
    re := regexp.MustCompile(`/home/[^/\s]+`)
    msg = re.ReplaceAllString(msg, "/home/<user>")
    
    return msg
}
```

---

### CR-M5: Add Consistent Error Wrapping
**Status:** ❌ Not Started
**Files:** Multiple

**Fix all instances like:**
```go
// Before:
return nil, fmt.Errorf("path does not exist: %s", path)

// After:
return nil, fmt.Errorf("path %q does not exist: %w", path, err)
```

---

### CR-M6: Make CLI Timeout Configurable
**Status:** ❌ Not Started
**File:** `cmd/zerb/config_add.go`

**Add flag:**
```go
timeout := fs.Duration("timeout", 5*time.Minute, "Operation timeout")

// Use in context:
ctx, cancel := context.WithTimeout(context.Background(), *timeout)
```

---

### CR-M7: Add Context Checks Before Blocking I/O
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`

**Add checks:**
```go
// Before file I/O:
if err := ctx.Err(); err != nil {
    return nil, fmt.Errorf("operation cancelled: %w", err)
}

cfgData, err := os.ReadFile(activeConfigPath)
```

---

### CR-M8: Validate Config File Count Limit
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`

**Add before appending:**
```go
if len(currentConfig.Configs) + len(result.AddedPaths) > config.MaxConfigFileCount {
    return nil, fmt.Errorf("would exceed maximum config file count (%d)", config.MaxConfigFileCount)
}
```

---

### CR-M9: Implement Confirmation Prompt
**Status:** ❌ Not Started
**File:** `cmd/zerb/config_add.go`

**Add flag and prompt:**
```go
yes := fs.Bool("yes", false, "Skip confirmation prompt")

// After building request but before Execute:
if !*yes && !*dryRun {
    fmt.Printf("\nWill add %d config file(s):\n", len(paths))
    for _, path := range paths {
        fmt.Printf("  - %s\n", path)
    }
    fmt.Print("\nApply? [Y/n] ")
    
    reader := bufio.NewReader(os.Stdin)
    response, err := reader.ReadString('\n')
    if err != nil {
        return fmt.Errorf("read confirmation: %w", err)
    }
    
    response = strings.TrimSpace(strings.ToLower(response))
    if response != "" && response != "y" && response != "yes" {
        fmt.Println("Aborted.")
        return nil
    }
}
```

---

### CR-M10: Implement --resume and --abort Flags
**Status:** ❌ Not Started
**File:** `cmd/zerb/config_add.go`

**Add flags:**
```go
resume := fs.Bool("resume", false, "Resume interrupted transaction")
abort := fs.Bool("abort", false, "Abort and cleanup incomplete transaction")

// Handle mutually exclusive flags
if *resume && *abort {
    return fmt.Errorf("--resume and --abort are mutually exclusive")
}

if *abort {
    return runConfigAddAbort(zerbDir)
}

if *resume {
    return runConfigAddResume(ctx, zerbDir, svc)
}
```

**Implement handlers:**
```go
func runConfigAddAbort(zerbDir string) error {
    txnDir := filepath.Join(zerbDir, "tmp")
    
    // Find transaction files
    files, err := filepath.Glob(filepath.Join(txnDir, "txn-config-add-*.json"))
    if err != nil {
        return fmt.Errorf("find transactions: %w", err)
    }
    
    if len(files) == 0 {
        fmt.Println("No active transactions to abort.")
        return nil
    }
    
    // Load and abort each
    for _, file := range files {
        txn, err := transaction.Load(file)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Warning: cannot load %s: %v\n", file, err)
            continue
        }
        
        fmt.Printf("Aborting transaction %s...\n", txn.ID)
        
        // Clean up created files
        for _, path := range txn.Paths {
            for _, created := range path.CreatedSourceFiles {
                if err := os.Remove(created); err != nil && !os.IsNotExist(err) {
                    fmt.Fprintf(os.Stderr, "Warning: cannot remove %s: %v\n", created, err)
                }
            }
        }
        
        // Remove transaction file
        os.Remove(file)
        
        // Release lock
        lockPath := filepath.Join(txnDir, "config-add.lock")
        os.Remove(lockPath)
        
        fmt.Println("Transaction aborted successfully.")
    }
    
    return nil
}

func runConfigAddResume(ctx context.Context, zerbDir string, svc *service.ConfigAddService) error {
    txnDir := filepath.Join(zerbDir, "tmp")
    
    // Find transaction file
    files, err := filepath.Glob(filepath.Join(txnDir, "txn-config-add-*.json"))
    if err != nil {
        return fmt.Errorf("find transactions: %w", err)
    }
    
    if len(files) == 0 {
        return fmt.Errorf("no transaction to resume")
    }
    
    if len(files) > 1 {
        return fmt.Errorf("multiple transactions found - use --abort first")
    }
    
    // Load transaction
    txn, err := transaction.Load(files[0])
    if err != nil {
        return fmt.Errorf("load transaction: %w", err)
    }
    
    fmt.Printf("Resuming transaction %s...\n", txn.ID)
    
    // Build request from pending/failed paths
    var paths []string
    options := make(map[string]service.ConfigOptions)
    
    for _, path := range txn.Paths {
        if path.State == transaction.StatePending || path.State == transaction.StateFailed {
            paths = append(paths, path.Path)
            options[path.Path] = service.ConfigOptions{
                Recursive: path.Recursive,
                Template:  path.Template,
                Secrets:   path.Secrets,
                Private:   path.Private,
            }
        }
    }
    
    if len(paths) == 0 {
        fmt.Println("All paths already completed. Finalizing...")
        // Just do final commit
    }
    
    // Execute with existing transaction
    request := service.AddRequest{
        Paths:   paths,
        Options: options,
        DryRun:  false,
    }
    
    result, err := svc.Execute(ctx, request)
    if err != nil {
        return fmt.Errorf("resume execution: %w", err)
    }
    
    fmt.Printf("Resumed and completed %d paths.\n", len(result.AddedPaths))
    return nil
}
```

---

## Low Priority Fixes

### CR-L1: Fix CLI Help Text
**Status:** ❌ Not Started
**File:** `cmd/zerb/main.go`

**Change:**
```go
// From:
fmt.Println("Usage: zerb config <add|list|remove>")

// To:
fmt.Println("Usage: zerb config add [options] <path>...")
fmt.Println("       (list and remove coming soon)")
```

---

### CR-L2: Define Magic Number Constants
**Status:** ❌ Not Started
**Files:** `internal/transaction/lock.go`, `internal/service/config_add.go`

**Add constants:**
```go
// In internal/transaction/lock.go
const StaleLockThreshold = 10 * time.Minute

// In internal/service/config_add.go
const (
    ConfigDirPermissions = 0755
    ConfigFilePermissions = 0644
    TmpDirPermissions = 0700
)
```

---

### CR-L3: Optimize String Building
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`, func `generateCommitBody`

**Change:**
```go
func (s *ConfigAddService) generateCommitBody(paths []string) string {
    if len(paths) == 1 {
        return ""
    }

    var sb strings.Builder
    sb.WriteString("Added configurations:\n")
    for _, path := range paths {
        sb.WriteString("- ")
        sb.WriteString(path)
        sb.WriteString("\n")
    }
    return sb.String()
}
```

---

### CR-L4: Add Missing Godoc
**Status:** ❌ Not Started
**Files:** Multiple

**Add documentation for all exported functions.**

---

### CR-L5: Preallocate Slices
**Status:** ❌ Not Started
**File:** `internal/service/config_add.go`

**Change:**
```go
result := &AddResult{
    AddedPaths:   make([]string, 0, len(req.Paths)),
    SkippedPaths: make([]string, 0, len(req.Paths)),
}
```

---

## Test Coverage Requirements

All fixes MUST include comprehensive tests. Target: >80% coverage.

### Required New Tests:

**Service Tests (`internal/service/config_add_test.go`):**
- [ ] Test Execute with single file
- [ ] Test Execute with multiple files
- [ ] Test Execute with --dry-run
- [ ] Test Execute with directory without --recursive (error)
- [ ] Test Execute with directory with --recursive
- [ ] Test duplicate detection
- [ ] Test error handling (chezmoi fails, git fails, etc.)
- [ ] Test context cancellation
- [ ] Test lock acquisition and release
- [ ] Test transaction state tracking

**Transaction Tests (`internal/transaction/transaction_test.go`, `lock_test.go`):**
- [ ] Test transaction creation and save
- [ ] Test lock acquisition (success and conflict)
- [ ] Test stale lock detection
- [ ] Test state transitions
- [ ] Test JSON marshaling/unmarshaling
- [ ] Test concurrent lock attempts (-race flag)

**Integration Tests:**
- [ ] End-to-end test with actual git repo
- [ ] Test resume functionality
- [ ] Test abort functionality
- [ ] Test confirmation prompt

---

## Testing Checklist

All tests must be run before merging fixes:

```bash
# Unit tests
go test ./internal/service -v -cover
go test ./internal/transaction -v -cover
go test ./internal/git -v -cover
go test ./internal/chezmoi -v -cover
go test ./internal/config -v -cover

# Race detection
go test ./... -race

# Coverage report
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Must achieve >80% coverage for:
# - internal/service/config_add.go
# - internal/transaction/transaction.go
# - internal/transaction/lock.go
```

---

## Sign-off Criteria

Before marking this change complete:

- [ ] All Critical fixes implemented and tested
- [ ] All High priority fixes implemented and tested
- [ ] All Medium priority fixes implemented and tested
- [ ] Test coverage >80% for service and transaction packages
- [ ] All tests pass with -race flag
- [ ] Integration tests added and passing
- [ ] Documentation updated
- [ ] Code review approval from both @code-reviewer and @golang-pro
