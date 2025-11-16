# 06-Error Handling & Recovery

**Status**: Not Started  
**Last Updated**: 2025-11-10  
**Dependencies**: All (cross-cutting concern)

---

## Overview

Error handling provides robust recovery mechanisms, transaction-based resume capability, and active secret redaction. This ensures ZERB never corrupts state and can safely resume after interruptions.

### Why This Matters

- Preserves data integrity
- Enables safe interruption
- Protects sensitive information
- Provides clear, actionable errors
- Maintains user control

---

## Development Environment Dependencies

### Nix Flake Packages

Error handling testing uses tools from the base dev shell:

```nix
# From flake.nix - Component 06 section
delve        # Go debugger for debugging error scenarios
# Plus all tools from Components 01-05 (error handling is cross-cutting)
```

**Purpose:**
- **delve**: Debug complex error scenarios, inspect state during failures, set breakpoints in error handlers
- Base dev shell tools: Component 06 tests interact with all other components

**Note**: Error handling is a cross-cutting concern that touches every component.

### Go Dependencies

Uses standard library for error handling:

```go
import (
    "errors"       // Error creation and wrapping
    "fmt"          // Error formatting
    "context"      // Context cancellation and timeouts
    "syscall"      // File locking (flock)
    "os"           // File operations
    "encoding/json" // Transaction file serialization
)
```

**No additional third-party libraries required for MVP.**

**Error Wrapping Pattern:**
```go
func DoSomething() error {
    if err := SubOperation(); err != nil {
        return fmt.Errorf("do something: %w", err)
    }
    return nil
}
```

### Testing Tools

Component 06 testing requires:

- **delve**: Interactive debugging of error scenarios
- **gotestsum**: Better test output for error cases
- **go test -race**: Detect race conditions in transaction management
- **Mock tools**: Simulate disk full, permission errors, network failures

### Development Workflow

```bash
# Enter Nix dev shell
nix develop

# Run component tests
just test-one TestTransactionManagement

# Test with race detector
go test -race ./internal/errors/...

# Test atomic writes
just test-one TestAtomicWrite

# Test secret redaction
just test-one TestSecretRedaction

# Debug error scenario with delve
dlv test ./internal/errors -- -test.run TestSpecificError
```

### Testing Transaction Management

Test transaction file creation and recovery:

```go
func TestTransactionManagement(t *testing.T) {
    tmpDir := t.TempDir()
    txnFile := filepath.Join(tmpDir, "txn-test.json")
    
    // Create transaction
    txn := &Transaction{
        Command:   "drift-resolve",
        StartedAt: time.Now(),
        Steps: []Step{
            {ID: 1, Action: "resolve-node", Status: "pending"},
            {ID: 2, Action: "resolve-python", Status: "pending"},
        },
        CurrentStep: 1,
    }
    
    // Write transaction
    err := WriteTransaction(txnFile, txn)
    assert.NoError(t, err)
    
    // Read transaction back
    loaded, err := ReadTransaction(txnFile)
    assert.NoError(t, err)
    assert.Equal(t, txn.Command, loaded.Command)
    assert.Len(t, loaded.Steps, 2)
    
    // Update progress
    txn.Steps[0].Status = "completed"
    txn.CurrentStep = 2
    err = WriteTransaction(txnFile, txn)
    assert.NoError(t, err)
    
    // Clean up (on success)
    err = os.Remove(txnFile)
    assert.NoError(t, err)
}
```

### Testing Atomic Writes

Test atomic file operations:

```go
func TestAtomicWrite(t *testing.T) {
    tmpDir := t.TempDir()
    targetFile := filepath.Join(tmpDir, "config.lua")
    
    // Write atomically
    content := []byte("zerb = { tools = {} }")
    err := AtomicWrite(targetFile, content)
    assert.NoError(t, err)
    
    // Verify file exists and content matches
    read, err := os.ReadFile(targetFile)
    assert.NoError(t, err)
    assert.Equal(t, content, read)
    
    // Verify no .tmp file left behind
    tmpFile := targetFile + ".tmp"
    _, err = os.Stat(tmpFile)
    assert.True(t, os.IsNotExist(err))
}

func TestAtomicWriteFailure(t *testing.T) {
    // Test disk full scenario (simulated)
    targetFile := "/dev/full/config.lua" // Special Linux device
    content := []byte("data")
    
    err := AtomicWrite(targetFile, content)
    assert.Error(t, err)
    
    // Verify original file unchanged (if it existed)
}
```

### Testing Secret Redaction

Test active secret redaction:

```go
func TestSecretRedaction(t *testing.T) {
    tests := []struct {
        name   string
        input  string
        want   string
    }{
        {"Plain password", "PASSWORD=secret123", "PASSWORD=***"},
        {"API key", "GITHUB_TOKEN=ghp_abc123", "GITHUB_TOKEN=[REDACTED]"},
        {"URL with auth", "https://user:pass@github.com", "https://***:***@github.com"},
        {"Normal text", "This is normal text", "This is normal text"},
        {"Mixed", "user=admin password=secret", "user=admin password=***"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := RedactSecrets(tt.input)
            assert.Equal(t, tt.want, got)
            assert.NotContains(t, got, "secret")
            assert.NotContains(t, got, "pass")
        })
    }
}
```

### Testing File Locking

Test concurrent operation prevention:

```go
func TestFileLocking(t *testing.T) {
    lockFile := filepath.Join(t.TempDir(), "zerb.lock")
    
    // Acquire lock
    lock, err := AcquireLock(lockFile)
    assert.NoError(t, err)
    defer lock.Close()
    
    // Try to acquire again (should fail)
    _, err = AcquireLock(lockFile)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "another ZERB operation is in progress")
    
    // Release lock
    lock.Close()
    
    // Now should succeed
    lock2, err := AcquireLock(lockFile)
    assert.NoError(t, err)
    lock2.Close()
}
```

### Simulating Error Conditions

Create test helpers for common error scenarios:

```go
// internal/errors/testutil.go
package errors

import (
    "fmt"
    "os"
    "testing"
)

// SimulateDiskFull creates a file system that appears full
func SimulateDiskFull(t *testing.T) func() {
    // Implementation depends on OS
    // Linux: Use /dev/full
    // Others: Mock filesystem
}

// SimulatePermissionError creates a file with restricted permissions
func SimulatePermissionError(t *testing.T, path string) {
    t.Helper()
    os.WriteFile(path, []byte("data"), 0000) // No permissions
}

// SimulateNetworkError returns a mock HTTP handler that fails
func SimulateNetworkError(statusCode int) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(statusCode)
    }
}

// SimulateCorruptedFile creates a file with invalid content
func SimulateCorruptedFile(t *testing.T, path, content string) {
    t.Helper()
    os.WriteFile(path, []byte(content), 0644)
}
```

### Testing Recovery Workflows

Test recovery from various failure scenarios:

```go
func TestRecoverFromCorruptedConfig(t *testing.T) {
    // Setup: Create corrupted config
    tmpDir := t.TempDir()
    activeConfig := filepath.Join(tmpDir, "configs", "zerb.lua.20250115T143022Z")
    prevConfig := filepath.Join(tmpDir, "configs", "zerb.lua.20250115T142510Z")
    
    os.MkdirAll(filepath.Dir(activeConfig), 0755)
    
    // Write corrupted active config
    os.WriteFile(activeConfig, []byte("invalid lua {{{"), 0644)
    
    // Write valid previous config
    os.WriteFile(prevConfig, []byte("zerb = { tools = {} }"), 0644)
    
    // Attempt recovery
    err := RecoverFromCorruption(tmpDir)
    assert.NoError(t, err)
    
    // Verify rolled back to previous config
    marker := filepath.Join(tmpDir, ".zerb-active")
    content, _ := os.ReadFile(marker)
    assert.Contains(t, string(content), "20250115T142510Z")
}
```

### Debugging with Delve

Use delve for interactive debugging:

```bash
# Debug specific test
dlv test ./internal/errors -- -test.run TestAtomicWrite

# In delve session:
(dlv) break AtomicWrite
(dlv) continue
(dlv) print targetPath
(dlv) next
(dlv) step
```

**Common delve commands:**
```bash
break <func>    # Set breakpoint
continue        # Continue execution
next            # Step over
step            # Step into
print <var>     # Print variable
list            # Show source code
goroutines      # List goroutines
```

### Environment Variables

Component 06 uses:

```bash
# Set by Nix dev shell
export ZERB_DEV=1              # Enable detailed error logging
export ZERB_TEST_MODE=1        # Use test error handlers

# Component-specific (for testing)
export ZERB_DISABLE_REDACTION=1    # Show full errors in tests
export ZERB_SIMULATE_DISK_FULL=1   # Trigger disk full errors
export ZERB_TRANSACTION_DIR=/tmp/test-txn  # Test transaction storage
export ZERB_DISABLE_FLOCK=1        # Disable file locking in tests
```

### Error Message Guidelines

Ensure consistent, helpful error messages:

```go
// Good: Actionable error
fmt.Errorf("failed to write config: disk full (try: rm -rf ~/.cache/zerb/)")

// Good: Context provided
fmt.Errorf("parse config %s: %w", configPath, err)

// Bad: Generic
fmt.Errorf("error")

// Bad: Exposes internals
fmt.Errorf("gopsutil.PlatformInformation failed")
```

### Manual Testing Checklist

Before completing Component 06:

- [ ] Transaction files created correctly
- [ ] Transaction resume works after interruption
- [ ] Atomic writes never leave partial files
- [ ] File locking prevents concurrent operations
- [ ] Secret redaction works on all patterns
- [ ] Corrupted config recovery works
- [ ] Error messages are clear and actionable
- [ ] Exit codes are consistent
- [ ] delve can attach and debug
- [ ] No race conditions detected with `-race` flag

### Testing Exit Codes

Verify consistent exit codes:

```go
func TestExitCodes(t *testing.T) {
    tests := []struct {
        name     string
        command  []string
        wantExit int
    }{
        {"Success", []string{"test"}, ExitSuccess},
        {"Validation error", []string{"init", "--invalid"}, ExitValidationError},
        {"Network error", []string{"pull", "--offline"}, ExitNetworkError},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Run command, capture exit code
            exitCode := RunCommand(tt.command...)
            assert.Equal(t, tt.wantExit, exitCode)
        })
    }
}
```

---

## Design (Decided)

### Core Principles

1. **Preserve integrity**: Never corrupt active local baseline or environment
2. **Be explicit**: No auto-fix; always prompt for decisions
3. **Be resumable**: Persist progress; allow resume after interruptions
4. **Be predictable**: Consistent messages, codes, and behaviors
5. **Operate atomically per step**: Make each action small and recoverable
6. **Degrade gracefully offline**: Use caches where feasible; warn clearly

### Error Taxonomy

```go
const (
    ExitSuccess         = 0
    ExitGenericError    = 1
    ExitValidationError = 2  // baseline/config/args
    ExitNetworkError    = 3  // online required but unavailable
    ExitInterrupted     = 4  // user abort or signal
    ExitUnresolvedDrift = 5  // user aborted before completion
)
```

### Transaction-Based Resume

**Transaction file:** `~/.config/zerb/tmp/txn-<cmd>-<timestamp>.json`

```json
{
  "command": "drift-resolve",
  "started_at": "2025-01-15T14:30:22Z",
  "steps": [
    {"id": 1, "action": "resolve-drift-neovim", "status": "completed"},
    {"id": 2, "action": "resolve-drift-node", "status": "in_progress"},
    {"id": 3, "action": "resolve-drift-python", "status": "pending"}
  ],
  "current_step": 2
}
```

**Behavior:**
- Write at start of multi-step operations
- Update after each step completion
- Remove on full success
- On startup: If transaction exists, prompt "Resuming last operation"

### Active Secret Redaction

```go
secretPatterns := []string{
    "password", "passwd", "pwd",
    "secret", "token", "key",
    "apikey", "api_key",
    "credentials", "auth",
}

// Examples of redacted output:
PASSWORD=*** 
GITHUB_TOKEN=[REDACTED]
https://***:***@github.com
```

### Atomic Writes

```go
func AtomicWrite(path string, data []byte) error {
    tmpFile := path + ".tmp"
    
    // Write to temp file
    if err := os.WriteFile(tmpFile, data, 0644); err != nil {
        return err
    }
    
    // Fsync to ensure data on disk
    f, _ := os.Open(tmpFile)
    f.Sync()
    f.Close()
    
    // Atomic rename
    return os.Rename(tmpFile, path)
}
```

### Corrupted Config Recovery

```bash
Error: Active local baseline is corrupted
File: configs/zerb.lua.20250115T143022Z
Issue: Lua syntax error at line 42

Recovery options:
  1. Roll back to previous local baseline (20250115T142510Z - 1 hour ago)
  2. Reset to remote baseline (last synced 30 minutes ago)
  3. Exit for manual repair

Choice [1]: _
```

---

## Open Questions

### 6.1 Transaction Management

🔴 **Question:** What exact fields should be in txn-<cmd>-<timestamp>.json?

**Proposed structure:**
```json
{
  "command": "string",
  "started_at": "RFC3339 timestamp",
  "steps": [
    {
      "id": "int",
      "action": "string",
      "status": "pending|in_progress|completed|failed",
      "error": "string (if failed)",
      "completed_at": "RFC3339 timestamp (if completed)"
    }
  ],
  "current_step": "int"
}
```

---

🔴 **Question:** How do we handle transaction cleanup if ZERB crashes mid-operation?

**Approach:** On next startup, detect transaction file and prompt user to resume or abort.

---

🟡 **Question:** Should we have a global transaction lock to prevent concurrent operations?

**Options:**
1. Lock file (flock)
2. PID file
3. No lock (allow concurrent)

**Recommendation:** Option 1 - Lock file for safety.

---

🔴 **Question:** How do we implement resume logic? Should we prompt the user or resume automatically?

**Recommendation:** Always prompt. User should confirm resume.

---

### 6.2 Atomic Operations

🟡 **Question:** What operations need atomic writes beyond baseline updates and cache files?

**List:**
- Baseline updates (configs/zerb.lua.*)
- .zerb-active marker file
- Cache files
- Transaction files
- Lock files

---

🔴 **Question:** Should we use flock or another mechanism for file locking?

**Approach:**
```go
import "syscall"

func acquireLock(path string) (*os.File, error) {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return nil, err
    }
    
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
        f.Close()
        return nil, fmt.Errorf("another ZERB operation is in progress")
    }
    
    return f, nil
}
```

---

🟡 **Question:** How do we handle atomic writes across NFS or other network filesystems?

**Recommendation:** Document limitation. NFS doesn't guarantee atomic rename.

---

## Implementation Tracking

### Completed
- [ ] 

### In Progress
- [ ] 

### Blocked
- [ ] 

### Notes

---

## Testing Requirements

### Unit Tests

**Transaction Management:**
- [ ] Test creating transaction file
- [ ] Test updating transaction progress
- [ ] Test resuming from transaction
- [ ] Test cleaning up completed transaction
- [ ] Test handling corrupted transaction file

**Secret Redaction:**
- [ ] Test redacting passwords
- [ ] Test redacting tokens
- [ ] Test redacting URLs with credentials
- [ ] Test preserving non-secret data

**Atomic Writes:**
- [ ] Test atomic write success
- [ ] Test atomic write failure (disk full)
- [ ] Test atomic write with fsync
- [ ] Test handling existing temp files

**Error Messages:**
- [ ] Test validation error formatting
- [ ] Test network error formatting
- [ ] Test actionable suggestions

### Integration Tests

**End-to-End:**
- [ ] Interrupt operation, resume successfully
- [ ] Corrupt config, recover from previous
- [ ] Corrupt config, recover from remote
- [ ] Network failure, graceful degradation
- [ ] Disk full, clear error message

---

## References

### External Documentation
- [Go error handling](https://go.dev/blog/error-handling-and-go)
- [Atomic file operations](https://lwn.net/Articles/457667/)

### Related Components
- All components use error handling
- [05-drift-detection.md](05-drift-detection.md) - Uses transaction resume

### Design Decisions
- [Decision 10: Error Handling Strategy](../decisions.md#decision-10-error-handling-strategy)
