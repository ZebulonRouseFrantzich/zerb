# Design Document: Config List Command

## Overview

The `zerb config list` command provides visibility into tracked configuration files with status detection and multiple output formats. This design document captures architectural decisions, trade-offs, and implementation approach.

## Architecture

### Component Structure

```
cmd/zerb/
  └── config_list.go          # CLI command layer (flag parsing, output)

internal/service/
  └── config_list.go          # Business logic layer (orchestration)

internal/config/
  └── status.go               # Status detection logic + StatusDetector interface (new)
  └── types.go                # Existing types (Config, ConfigFile)
  └── parser.go               # Existing parser

internal/chezmoi/
  └── chezmoi.go              # Extend Chezmoi interface with HasFile method
```

### Data Flow

```
User Command
    ↓
CLI Layer (config_list.go)
    ↓ (parse flags, create context)
Service Layer (config_list.go)
    ↓ (orchestrate, inject zerbDir)
    ├→ Read active marker
    ├→ Parse config           [config.Parser interface]
    ├→ Detect status          [config.StatusDetector interface]
    │   ├→ Check disk         [fs.FS or os abstraction]
    │   └→ Check managed      [chezmoi.Chezmoi.HasFile]
    └→ Format output
        ├→ Table (default)
        └→ Verbose (--verbose)
    ↓
User Output
```

**Note**: MVP supports table output only. JSON, plain, and `--all` are deferred.

## Key Design Decisions

### 1. Status Detection Strategy

**Decision**: Simplified status detection for MVP - three statuses only (Synced, Missing, Partial). Defer drift detection to future iteration.

**Rationale**:
- **Synced**: Easy to detect (exists + managed by ZERB)
- **Missing**: Easy to detect (declared but file doesn't exist)
- **Partial**: Easy to detect (exists but not managed by ZERB - incomplete add operation)
- **Drift**: Complex - requires file hash comparison, modification time tracking, and integration with drift detection component

**MVP Implementation**:
- Define `ConfigStatus` enum with Synced, Missing, Partial
- Detect only these three statuses
- **Do not show drift symbol** or status until detection is implemented
- Reserve drift logic for dedicated future change

**Future Enhancement** (separate change proposal):
- Add Drift status to enum
- Hash comparison: SHA256 of disk file vs managed file
- Modification time: Compare mtime
- Integration with existing drift detection component
- Show "⚠" symbol for drifted configs

### 2. Interface-Based Design

**Decision**: Use interfaces for all major dependencies to enable testing and future flexibility.

**Interfaces Required**:

1. **`config.StatusDetector`** (new):
```go
type StatusDetector interface {
    DetectStatus(ctx context.Context, configs []ConfigFile) ([]ConfigWithStatus, error)
}
```

2. **`chezmoi.Chezmoi`** (extend existing):
```go
type Chezmoi interface {
    Add(ctx context.Context, path string, opts AddOptions) error
    HasFile(ctx context.Context, path string) (bool, error)  // NEW
}
```

**Rationale**:
- Follows Go best practices: "accept interfaces, return structs"
- Enables comprehensive unit testing with mocks
- Reduces coupling between layers
- Consistent with existing `ConfigAddService` pattern

### 3. "Managed by ZERB" Terminology

**Decision**: Abstract away "chezmoi" completely from user-facing output.

**Rationale**:
- ZERB wraps chezmoi as implementation detail
- Users think in terms of "ZERB managing configs", not "chezmoi managing configs"
- Maintains clean abstraction layer for future implementation changes

**Terminology Mapping**:
- ❌ "in chezmoi source directory"
- ✅ "managed by ZERB"
- ❌ "chezmoi has file"
- ✅ "tracked by ZERB"

### 4. Output Format Design

**Decision**: Two output formats for MVP - table (default) and verbose. Defer JSON/plain to future iteration.

**Formats** (MVP):
1. **Table (default)**: Human-readable, status indicators, aligned columns
2. **Verbose (--verbose)**: Enhanced table with size and timestamps

**Deferred to Future** (separate change proposal):
3. **Plain (--plain)**: Simple paths for scripting/piping
4. **JSON (--json)**: Structured data for automation

**Exclusivity Rules**:
- `--verbose` enhances the default table format
- No format conflicts in MVP (only one alternative format)

**Rationale**:
- Table: Most common use case (quick overview)
- Verbose: Troubleshooting and detailed inspection
- Plain/JSON deferred: Keep MVP focused, add automation support later

### 5. Chezmoi Query Interface

**Decision**: Extend `Chezmoi` interface with read-only query method, use direct filesystem access for implementation.

**Interface Extension**:
```go
type Chezmoi interface {
    Add(ctx context.Context, path string, opts AddOptions) error
    HasFile(ctx context.Context, path string) (bool, error)
}
```

**Implementation** (in `*Client`):
```go
// HasFile checks if a path is managed by ZERB
// Returns true if file exists in chezmoi source directory
func (c *Client) HasFile(ctx context.Context, path string) (bool, error) {
    // 1. Use config.NormalizeConfigPath to get canonical path (security)
    // 2. Map to chezmoi source-relative path
    // 3. Check filesystem directly
    // 4. Wrap errors with RedactedError (preserve error chain while hiding paths)
}
```

**Key Requirements**:
- **Interface-based**: Service layer depends on `Chezmoi` interface, not `*Client`
- **Path safety**: MUST reuse `config.NormalizeConfigPath` for canonical paths
- **Error redaction**: Use `RedactedError` wrapper type to preserve error chain while hiding sensitive info
- **Fast**: Direct filesystem check, no chezmoi binary invocation

**Alternative Considered**: Invoke `chezmoi managed` command
- Rejected: Slower, more complex, same result

**Error Handling Design**:
- Implement `RedactedError` type with `Unwrap()` method
- Preserves error chain for `errors.Is/errors.As` checks
- Redacts sensitive information (paths, "chezmoi" mentions)
- Enables upstream code to handle specific error types while maintaining security

### 6. Historical Config Listing (--all)

**Decision**: Defer `--all` flag to future iteration. MVP lists only active config.

**Rationale**:
- Simplifies MVP scope
- Active config is primary use case
- Historical listing requires more complex UI and schema design

**Future Implementation** (separate change proposal):
- Parse all timestamped configs in `configs/` directory
- Group by version, sorted newest first
- Status detection against CURRENT disk state
- Clear marking of active version
- Consider JSON schema implications

### 7. Sorting Strategy

**Decision**: Alphabetical sorting by path (case-sensitive).

**Rationale**:
- Predictable, consistent output
- Easy to find specific paths
- Matches user mental model (alphabetical file listing)

**Implementation**:
```go
sort.Slice(configs, func(i, j int) {
    return configs[i].Path < configs[j].Path
})
```

### 8. Error Handling

**Decision**: Service layer returns structured errors using sentinel values + wrapping, CLI layer formats for users.

**Error Strategy**:
- Define sentinel errors in `internal/service` or `internal/config`
- Wrap underlying errors with `%w` for error chains
- CLI uses `errors.Is` to detect and format user-friendly messages
- Keep error values generic, messages separate

**Error Types**:
- `ErrNotInitialized`: ZERB not initialized
- `ErrConfigParse`: Config parsing failed  
- `ErrPermission`: Permission denied
- `ErrInvalidPath`: Path validation failed

**Error Redaction**:
- All errors from `HasFile` MUST be wrapped with `RedactedError` type
- `RedactedError` implements `Unwrap()` to preserve error chains
- Never expose internal paths (`~/.config/zerb/chezmoi/source`)
- Never expose "chezmoi" in user-facing messages
- Enables `errors.Is/errors.As` checks while maintaining security

**User-Facing Messages**:
- Clear, actionable
- No internal implementation details
- Suggest next steps

Example:
```
Error: ZERB not initialized
Run: zerb init
```

### 9. Path Normalization Strategy

**Decision**: Service layer normalizes paths before passing to detector (Option A).

**Rationale**:
- **Single normalization point**: Happens once for all operations
- **Detector receives clean data**: Simpler detector implementation
- **Better error context**: Service can provide user-friendly errors about which config failed
- **Consistent with other layers**: Service orchestrates, detector executes
- **Easier testing**: Detector tests can assume normalized paths

**Data Flow**:
```
Service.List()
  ├─ For each config:
  │   ├─ normalizedPath = NormalizeConfigPath(cfg.Path)
  │   ├─ Update cfg.Path with normalized path
  │   └─ Pass normalized configs to detector
  └─ Detector uses pre-normalized paths directly
```

**Implementation Location**:
- **Service layer** (`internal/service/config_list.go`): Normalizes before detection
- **Detector** (`internal/config/status.go`): Uses pre-normalized paths (documents this assumption)
- **Chezmoi** (`internal/chezmoi/chezmoi.go`): HasFile already normalizes (unchanged)

**Alternatives Considered**:
- **Option B (Detector normalizes)**: Rejected - would normalize twice (os.Stat + HasFile)
- **Option C (Hybrid with extra field)**: Rejected - over-engineered, requires type changes

**Benefits**:
- Fixes tilde path bug (paths like `~/.zshrc` now work correctly)
- Single normalization per config (performance)
- Clear separation of concerns (service validates, detector executes)
- No breaking changes to types

### 10. Context Support

**Decision**: Full context support for cancellation and timeouts via `--timeout` flag.

**Implementation**:
```go
// CLI layer: derive context from --timeout flag (default 5m)
timeout := fs.Duration("timeout", 5*time.Minute, "Operation timeout")
ctx, cancel := context.WithTimeout(context.Background(), *timeout)
defer cancel()

// Service layer: accept context, don't create timeouts
result, err := service.List(ctx, request)
```

**Benefits**:
- User can cancel with Ctrl+C (exit code 130)
- Prevents hanging on slow operations
- Consistent with `config add` pattern
- Service computes full result before CLI prints (no partial output on cancel)

**Requirements**:
- Add `--timeout` flag to CLI layer
- Service returns complete result struct before any output
- CLI prints only after successful service call
- Context cancellation treated as error path

## Testing Strategy

### Unit Tests

**Parser/Config Layer**:
- Test status detection logic in isolation
- Mock filesystem and chezmoi client
- Test edge cases (missing files, permission errors)
- **CRITICAL**: Test tilde paths (`~/.zshrc`) using `t.Setenv("HOME")`
- Test nested paths (`~/.config/nvim/init.lua`)

**Service Layer**:
- Test with mock parser, chezmoi, filesystem
- Test path normalization before detection
- Test error conditions (empty marker, missing config)
- **Target**: >80% coverage (currently 78.1%, needs 2 more test cases)

**CLI Layer**:
- **CRITICAL**: Create `cmd/zerb/config_list_test.go` (currently missing)
- Test flag parsing (`--verbose`, `--timeout`)
- Test output formatting (mock service layer)
- Test help text
- **Integration tests**: Real service with temp filesystem (CI/CD safe)

**Chezmoi Layer**:
- Test `HasFile` method with various path types
- Test `RedactedError` preserves error chain
- Test error redaction (paths, "chezmoi" → "config manager")
- Test context cancellation

### Integration Tests

**End-to-End**:
- Create real ZERB environment
- Add configs with `zerb config add`
- Run `zerb config list` and verify output
- Test all flags and combinations

**Status Detection**:
- Create synced config (add with `zerb config add`)
- Create missing config (delete file after add)
- Create partial config (add to zerb.lua but not to chezmoi)
- Verify correct status for each

### Test Coverage Goal

>80% coverage for all new code:
- `cmd/zerb/config_list.go`: >80% (**Currently 0% - SHIP BLOCKER**)
- `internal/service/config_list.go`: >80% (**Currently 78.1% - needs 2 test cases**)
- `internal/config/status.go`: >80% (**Currently 100% ✓**)
- `internal/chezmoi/chezmoi.go` (HasFile): >80% (**Currently 84.2% ✓**)

**Ship Blockers**:
1. Add CLI test file with 15-20 test cases
2. Complete service layer coverage (empty marker, missing config tests)

## Performance Considerations

### Expected Performance

**Typical Use Case** (10-20 configs):
- Read 1 config file: <1ms
- Parse Lua: <5ms
- Status detection (20 configs): <10ms
- Format output: <1ms
- **Total**: <20ms

**Large Use Case** (100+ configs):
- Parse Lua: <10ms
- Status detection (100 configs): ~50ms (filesystem checks)
- Format output: <5ms
- **Total**: <100ms

### Optimization Opportunities (Future)

1. **Parallel status detection**: Check multiple files concurrently
2. **Caching**: Cache parsed configs in memory
3. **Lazy loading**: Only parse configs when needed (for --all)
4. **Pagination**: For very large lists, page output

## Security Considerations

### Path Handling

- Reuse existing `validateConfigPath` from config add
- Tilde expansion handled securely
- No path traversal vulnerabilities

### Permission Errors

- Gracefully handle permission denied
- Don't expose sensitive file contents
- Clear error messages

### Information Disclosure

- Only show paths from user's ZERB config
- Don't leak system paths
- Redact any secrets in error messages (though unlikely here)

## Future Enhancements

### Full Drift Detection

**What**:
- Compare file hashes (SHA256)
- Detect actual content changes
- Show what changed (diff preview)

**Implementation**:
```go
func detectDrift(diskPath, managedPath string) (bool, error) {
    diskHash, err := hashFile(diskPath)
    managedHash, err := hashFile(managedPath)
    return diskHash != managedHash, nil
}
```

### Color Support

**What**:
- Green for synced (✓)
- Yellow for drift (⚠)
- Red for missing (✗)
- Gray for partial (?)

**Implementation**:
- Detect terminal color support
- Respect `NO_COLOR` env var
- Add `--no-color` flag

### Interactive Mode

**What**:
- Select configs for actions (remove, edit)
- Fuzzy finding
- TUI interface

**Tools**:
- bubbletea (TUI framework)
- fzf-style selection

### Config Diffing

**What**:
- Show changes between versions
- `zerb config list --diff`
- Compare historical configs

**Output**:
```
Changes from 20250115T140000Z to 20250116T143022Z:
  + ~/.config/nvim/  (added)
  ~ ~/.gitconfig     (flags changed: template enabled)
  - ~/.tmux.conf     (removed)
```

## Open Questions

### Q1: Should we cache parsed configs?

**Options**:
1. Parse on every invocation (simple, always fresh)
2. Cache in memory for session (fast, but adds complexity)
3. Cache on disk (fastest, but stale data risk)

**Recommendation**: Option 1 for MVP. Parsing is fast enough (<10ms).

### Q2: Should status detection be pluggable?

**Options**:
1. Hard-coded logic (simple)
2. Strategy pattern (extensible)

**Recommendation**: Option 1 for MVP. Add interface later if needed.

### Q3: Should we show chezmoi source paths in verbose mode?

**Options**:
1. Never show (full abstraction)
2. Show in verbose mode (helpful for debugging)

**Recommendation**: Option 1. Maintain abstraction even in verbose mode.

## References

- **Existing Specs**: `openspec/specs/config-management/spec.md`
- **Related Commands**: `zerb config add` (similar patterns)
- **Related Components**: Drift detection (similar status detection)
- **External Tools**: None (pure Go implementation)

## Code Review Findings & Resolutions

**Review Date**: 2025-11-17  
**Reviewers**: golang-pro, test-automator, code-reviewer  
**Overall Score**: 93/100

### Critical Issues: None ✅

### High Priority Issues (Ship Blockers)

**H1: Status Detection Tilde Path Bug** - RESOLVED
- **Issue**: `os.Stat(cfg.Path)` doesn't expand `~/.zshrc`
- **Resolution**: Service layer normalizes paths before detection (Option A)
- **Implementation**: See section 9 "Path Normalization Strategy"

**H2: CLI Layer Untested** - IN PROGRESS
- **Issue**: 0% coverage on `cmd/zerb/config_list.go` (314 lines)
- **Resolution**: Create test file with unit + integration tests
- **Target**: >80% coverage with 15-20 test cases

**H3: Service Coverage Below Threshold** - IN PROGRESS
- **Issue**: 78.1% vs 80% required
- **Resolution**: Add 2 test cases (empty marker, missing config)

### Medium Priority Issues

**M1: RedactedError Implementation** - RESOLVED
- **Issue**: Error wrapping loses type information for `errors.Is/errors.As`
- **Resolution**: Implement `RedactedError` type with `Unwrap()` method
- **Location**: `internal/chezmoi/chezmoi.go`

**M2: Duplicate StatusDetector Interface** - TRACKED
- **Issue**: Interface defined in both `service` and `config` packages
- **Resolution**: Remove from service, use `config.StatusDetector`

**M3-M6: Error Wrapping Consistency** - TRACKED
- Context errors should preserve chain with `%w`
- Service layer config errors should wrap with `%w`
- Flag parse errors should include context

### Design Decisions from Review

1. **Path Normalization**: Service layer (Option A) - See section 9
2. **Error Handling**: RedactedError wrapper type - See section 5
3. **Interface Deduplication**: Use config.StatusDetector everywhere
4. **Testing Strategy**: CLI unit tests + integration tests - See Testing Strategy

**Test-to-Code Ratio**: 1.2:1 (739 test lines / 608 impl lines) ✓
