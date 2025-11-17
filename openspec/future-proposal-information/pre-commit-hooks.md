# Future Proposal: Pre-commit Hooks for ZERB Repositories

## Overview

Implement pre-commit hooks for ZERB repositories that enforce configuration immutability, validate syntax and schema, detect large files, and prevent secret commits. This ensures repository integrity and protects users from accidental data leaks.

## Problem

ZERB users need automated safeguards to prevent:
- Modifying timestamped config snapshots (breaks immutability guarantee)
- Committing invalid Lua syntax or schema violations
- Accidentally committing large binaries or cache files
- Exposing secrets (API keys, tokens, passwords) in git history

Without automated checks, users can easily violate ZERB's architectural invariants and create security/correctness issues that are hard to fix retroactively.

## User Stories

### Story 1: Prevent Config Modification
**As a** ZERB user  
**I want** timestamped configs to be immutable  
**So that** I can trust git history and safely rollback to any snapshot

**Acceptance:**
- Attempting to modify `configs/zerb.lua.YYYYMMDDTHHMMSS.sssZ` files fails the commit
- Clear error message explains immutability and suggests creating new config instead
- Modifying the active config (via `zerb config edit`) creates a new timestamped snapshot

### Story 2: Validate Configuration Before Commit
**As a** ZERB user  
**I want** invalid configs to be rejected before commit  
**So that** I never push broken configurations to my team or other machines

**Acceptance:**
- Lua syntax errors in `zerb.lua` files fail the commit with line numbers
- Schema violations (unknown fields, wrong types) fail with clear messages
- Valid configs commit successfully without friction

### Story 3: Prevent Secret Leaks
**As a** ZERB user  
**I want** to be warned when committing potential secrets  
**So that** I don't accidentally expose credentials in git history

**Acceptance:**
- Common secret patterns detected (API_KEY=, password:, token:, etc.)
- Clear warning with matched patterns and line numbers
- Option to override if false positive (e.g., `--no-verify`)
- Guidance to use external secret managers (1Password, Bitwarden, age)

### Story 4: Warn About Large Files
**As a** ZERB user  
**I want** to be warned about large files before commit  
**So that** I don't bloat my repository with binaries or cached data

**Acceptance:**
- Files >10MB trigger a warning
- User sees file size and path
- Commit proceeds (warning only, not blocking)
- Suggests checking `.gitignore` patterns

## Design Decisions

### Decision 1: Hook Installation Timing

**Choice:** Install hooks during `zerb init` (when git repo is initialized).

**Rationale:**
- Hooks protect from the very first user commit
- No window where invalid commits could slip through
- Users don't need to remember a separate setup step
- Aligns with "batteries included" philosophy

**Alternatives considered:**
- Install on first `zerb config edit`: Too late, user might commit manually before then
- Separate `zerb hooks install` command: Extra step, reduces adoption
- Never install, document manual setup: Poor UX, inconsistent enforcement

### Decision 2: Hook Implementation Language

**Choice:** Write hooks in Go, compile to standalone binaries, copy to `.git/hooks/`.

**Rationale:**
- Consistency: Same language as ZERB itself
- No runtime dependencies: Works without bash, python, etc.
- Fast execution: Compiled hooks are instant
- Easy testing: Standard Go test infrastructure
- Cross-platform: Same hooks work on Linux, macOS, Windows

**Implementation:**
- Hooks embedded in ZERB binary (via `go:embed` or as compiled assets)
- During `zerb init`, extract hooks to `.git/hooks/` with executable permissions
- Hook binaries named: `pre-commit`, `commit-msg`, etc. (standard git names)
- Hooks call back into `zerb` internal libraries for validation logic

**Alternatives considered:**
- Shell scripts: Fragile, platform-dependent, hard to test
- Python/Node: External runtime dependencies
- Git hooks as subcommands: `git config core.hooksPath` complexity

### Decision 3: Immutability Check Strategy

**Choice:** Reject any modifications to `configs/zerb.lua.*` files in the staging area.

**Implementation:**
- Hook runs: `git diff --cached --name-only`
- Filter for `configs/` directory
- Check if any files match pattern `zerb.lua.[0-9]{8}T[0-9]{6}\.[0-9]{3}Z`
- If found: reject commit with error message

**Error message:**
```
❌ Commit rejected: Timestamped configs are immutable

You attempted to modify:
  configs/zerb.lua.20250116T143052.123Z

Timestamped configs cannot be changed. To update your configuration:

  zerb config edit        # Edit active config
  zerb config activate    # Activate a different snapshot

This creates a new timestamped snapshot while preserving history.
```

### Decision 4: Lua Validation Strategy

**Choice:** Parse all staged `.lua` files with gopher-lua and validate against ZERB schema.

**Implementation:**
- Hook finds all staged `*.lua` files in `configs/` directory
- Parse each file using `github.com/yuin/gopher-lua`
- Validate structure matches expected schema (from `internal/config`)
- Reject commit if any validation fails

**Validation checks:**
- Lua syntax (parser errors)
- Required fields present (`zerb = {}` table exists)
- Field types correct (`tools` is array, `meta.name` is string, etc.)
- No unknown top-level fields (typo detection)

**Error message format:**
```
❌ Commit rejected: Invalid configuration

configs/zerb.lua.20250116T143052.123Z:15: field 'tool' should be 'tools'
configs/zerb.lua.20250116T143052.123Z:23: meta.name must be a string, got number

Fix these errors and try again.
```

### Decision 5: Secret Detection Strategy

**Choice:** Pattern-based detection with common secret indicators, warning (non-blocking).

**Patterns to detect:**
- Environment variable assignments: `export API_KEY=`, `PASSWORD=`, `TOKEN=`
- Common credential fields: `password:`, `secret:`, `api_key:`, `bearer:`
- High-entropy strings: Long base64/hex strings (potential keys)
- Cloud provider patterns: AWS keys, GCP tokens, GitHub PATs

**Implementation:**
- Scan staged file contents line-by-line
- Match against predefined regex patterns
- Collect all matches with file:line:column
- If matches found: print warning, allow commit with `--no-verify` override

**Warning message:**
```
⚠ Warning: Potential secrets detected

configs/zerb.lua.20250116T143052.123Z:42: api_key = "sk_live_..."
configs/zerb.lua.20250116T143052.123Z:67: password = "..."

ZERB configs may be synced to git remotes. Do NOT commit secrets.

Recommended: Use external secret managers
  - 1Password: https://...
  - Bitwarden: https://...
  - age encryption: https://...

To proceed anyway (not recommended):
  git commit --no-verify
```

**Rationale for warning vs blocking:**
- False positives are common (example values, documentation)
- Users may have legitimate use cases (encrypted secrets, test data)
- Friction should educate, not prevent all commits
- Git provides `--no-verify` for override path

**Future enhancement:** 
- Integrate with external tools like `gitleaks` or `trufflehog`
- Allow custom patterns in `zerb.lua` (project-specific secrets)

### Decision 6: Large File Detection

**Choice:** Warn on files >10MB, non-blocking.

**Implementation:**
- Check size of all staged files
- If any file >10MB: print warning with file path and size
- Suggest reviewing `.gitignore` patterns
- Allow commit to proceed (informational only)

**Rationale:**
- Users occasionally need to commit large files (documentation, assets)
- Hard blocking creates bad UX for legitimate cases
- Warning prompts user to double-check `.gitignore`
- Catches accidental `git add .` including `cache/` or `bin/`

**Warning message:**
```
⚠ Warning: Large files detected

  cache/downloads/mise-v2024.1.0 (45.2 MB)
  
Large files should typically be excluded via .gitignore.

Check your .gitignore patterns:
  - bin/
  - cache/
  - tmp/
  - logs/

To proceed: git commit (no action needed)
```

### Decision 7: Hook Update Strategy

**Choice:** Version hooks and re-extract on ZERB upgrades.

**Implementation:**
- Embed version metadata in hook binaries
- On `zerb init`: write hooks with current version
- On `zerb upgrade` or version mismatch detection: offer to update hooks
- Add `zerb hooks update` command to manually refresh

**Rationale:**
- Validation logic evolves with ZERB (new schema fields, better secret detection)
- Users should benefit from improved checks automatically
- Manual update path for users who customize hooks

**Future consideration:**
- Detect user-modified hooks (checksum comparison)
- Merge strategy for custom + ZERB hooks

## Implementation Scope

### Phase 1: Core Hook Infrastructure (Depends on setup-git-repository)

- [ ] Design hook binary build system (embedded in `zerb` or separate `zerb-hooks`)
- [ ] Implement hook extraction during `zerb init`
- [ ] Add hook versioning and update detection
- [ ] Test hook installation on Linux (MVP platform)

### Phase 2: Immutability Check

- [ ] Implement timestamped config modification detection
- [ ] Write error message and user guidance
- [ ] Add tests: attempt to modify immutable config, verify rejection
- [ ] Add tests: creating new configs works (not blocked)

### Phase 3: Lua Validation

- [ ] Integrate gopher-lua parser into hook
- [ ] Implement schema validation (reuse `internal/config` logic)
- [ ] Write error messages with line numbers
- [ ] Add tests: valid config passes, syntax errors fail, schema violations fail

### Phase 4: Secret Detection

- [ ] Define secret detection patterns (regex library)
- [ ] Implement file scanning logic
- [ ] Write warning message and secret manager guidance
- [ ] Add tests: common patterns detected, `--no-verify` override works
- [ ] Document recommended secret management patterns

### Phase 5: Large File Detection

- [ ] Implement file size checking (10MB threshold)
- [ ] Write warning message with file sizes
- [ ] Add tests: large files trigger warning, commit proceeds

### Phase 6: Hook Updates

- [ ] Implement version detection and comparison
- [ ] Add `zerb hooks update` command
- [ ] Integrate hook update into `zerb upgrade` workflow
- [ ] Test upgrade scenarios

## Open Questions

### Q1: Should hooks support user customization?

**Options:**
- A) ZERB hooks are immutable, users add separate custom hooks
- B) Allow `.git/hooks/pre-commit.local` for user extensions
- C) Provide hook configuration in `zerb.lua` (enable/disable checks)

**Lean toward:** Option A for MVP (simplicity), Option C for future (flexibility)

### Q2: How to handle `--no-verify` abuse?

**Concern:** Users might habitually use `--no-verify` to bypass checks.

**Mitigations:**
- Track `--no-verify` commits in a log file (for audit)
- Warn in `zerb status` if recent commits bypassed hooks
- Provide `zerb doctor` check for "risky commits" (bypassed secret detection)

**Decision:** Start with education (clear warnings), add monitoring in future if needed.

### Q3: Should hooks work without `zerb` binary on PATH?

**Scenario:** User clones ZERB repo on new machine, `zerb` not yet installed.

**Options:**
- A) Hooks fail gracefully with "install zerb" message
- B) Hooks are fully standalone (embed all validation logic)

**Lean toward:** Option A (hooks depend on `zerb` being installed)

**Rationale:**
- ZERB repos assume ZERB is available (same as mise/chezmoi)
- Standalone hooks duplicate code and bloat
- Clear error message guides user to install ZERB

### Q4: How to handle repositories cloned before hooks existed?

**Scenario:** User initialized repo with older ZERB version (no hooks).

**Options:**
- A) `zerb status` detects missing hooks and prompts to install
- B) Hooks auto-install on first `zerb` command in repo
- C) Manual `zerb hooks install` command required

**Lean toward:** Option A (detect + prompt), with Option C as manual fallback

## Dependencies

### Depends On:
- **setup-git-repository change**: Git repository must exist to install hooks
- **Component 07 (git operations)**: May provide additional git helper functions

### Enables:
- Safe multi-user config sharing (immutability enforced)
- Better onboarding (invalid configs caught before push)
- Security compliance (secret detection for teams)

## When to Create Formal Proposal

Create `openspec/changes/pre-commit-hooks/` when:

1. ✅ **setup-git-repository** is merged and deployed (git repo exists in ZERB)
2. ✅ User feedback confirms hooks are a priority (complaints about manual validation)
3. ✅ At least one real-world case of broken config pushed to shared repo
4. ⏳ Component 07 git operations has basic commit functionality working

**Estimated Timeline:** Post-MVP, likely Q1 2026 (after remote sync is working)

## Security Considerations

### Secret Detection Limitations

**Important:** Pattern-based detection is not foolproof.

**Will detect:**
- Common patterns (`API_KEY=`, `password:`)
- High-entropy strings (likely keys)

**Will NOT detect:**
- Encrypted secrets (appear as random data)
- Novel patterns not in regex list
- Secrets split across lines or obfuscated

**User education required:**
- Hooks are a safety net, not a guarantee
- Best practice: NEVER store secrets in `zerb.lua`
- Use external secret managers (1Password, Bitwarden, age)

### Hook Bypass

Git allows `--no-verify` to skip hooks. This is intentional (emergency override).

**Mitigations:**
- Clear warnings explain risks
- Future `zerb doctor` can detect bypassed commits
- Team workflows can enforce hooks server-side (GitHub Actions, GitLab CI)

### Hook Tampering

Users with write access can modify `.git/hooks/` files.

**Mitigations:**
- ZERB repos are single-user by design (MVP)
- Future: checksums or signatures for hook verification
- Server-side enforcement for shared repositories

## Related Work

- **error-handling.md**: Mentions "active secret redaction" in logs (complementary)
- **git-remote-setup.md**: Remote sync increases secret leak risk (hooks more critical)
- **Component 07 (git operations)**: Provides git infrastructure hooks depend on

---

**Status:** Planning document  
**Created:** 2025-11-16  
**Next Review:** After setup-git-repository is merged
