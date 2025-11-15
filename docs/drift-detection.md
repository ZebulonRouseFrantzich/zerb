# Drift Detection Guide

## Overview

Drift detection is a core feature of ZERB that identifies discrepancies between your declared configuration and the actual state of your environment. This helps maintain reproducible environments and prevents "works on my machine" problems.

## Three-Way Comparison

ZERB performs a three-way comparison to detect drift:

1. **Baseline (Declared)** - What's defined in your `zerb.lua` configuration
2. **Managed (ZERB)** - What ZERB has actually installed via its tool manager
3. **Active (Environment)** - What's currently accessible in your PATH

This comprehensive approach catches issues that simpler two-way comparisons would miss, such as:
- External package managers installing tools (apt, brew, nvm, etc.)
- Manual tool installations taking precedence over ZERB's
- PATH configuration issues
- Version mismatches

## Running Drift Detection

### Basic Usage

```bash
# Check for drift and prompt for resolution
zerb drift

# Preview drift without making changes
zerb drift --dry-run

# Force refresh of version detection cache
zerb drift --force-refresh
```

### Example Output

```
Scanning environment...
✓ Parsed baseline (4 tools declared)
✓ Checked managed tools (3 tools installed)
✓ Checked active environment (4 tools in PATH)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
DRIFT REPORT
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[EXTERNAL OVERRIDE] ⚠️
  node
    Baseline:  20.11.0 (managed by ZERB)
    Active:    18.17.1 at /usr/bin/node
    
    → An external installation has taken precedence over ZERB

[VERSION MISMATCH]
  python
    Baseline:  3.12.1
    Active:    3.11.0 (managed by ZERB)
    
    → ZERB is managing this tool but the version doesn't match

[MISSING]
  go
    Baseline:  1.22.0
    Active:    (not installed)
    
    → Declared in baseline but not found

[OK] ✓
  1 tools match baseline

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SUMMARY: 3 drifts detected
  1 external override, 1 version mismatch, 1 missing
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

How would you like to resolve these drifts?
  1. Resolve individually (choose action for each drift)
  2. Adopt all changes (update baseline to match environment)
  3. Revert all changes (restore environment to match baseline)
  4. Show details only (no changes)
  5. Exit

Choice [1]: 
```

## Drift Types

### OK ✓

**Description:** Tool version matches across all three sources.

**No action needed** - Everything is working as intended.

---

### Version Mismatch

**Description:** ZERB is managing the tool but with the wrong version.

**Common causes:**
- You updated the tool manually using the tool manager
- You changed the version in `zerb.lua` but haven't synced yet
- Interrupted installation left inconsistent state

**Resolution options:**
- **Adopt** - Update baseline version to match currently installed version
- **Revert** - Reinstall the correct version from baseline
- **Skip** - Decide later

**Example:**
```
python
  Baseline:  3.12.1
  Active:    3.11.0 (managed by ZERB)
```

---

### Missing

**Description:** Tool is declared in baseline but not installed anywhere.

**Common causes:**
- Fresh clone of configuration on new machine
- Tool installation failed or was interrupted
- Tool was manually uninstalled

**Resolution options:**
- **Revert** - Install the missing tool
- **Adopt** - Remove from baseline (you decided not to use this tool)
- **Skip** - Decide later

**Example:**
```
go
  Baseline:  1.22.0
  Active:    (not installed)
```

---

### Extra

**Description:** Tool is installed by ZERB but not declared in baseline.

**Common causes:**
- You installed a tool manually via ZERB's tool manager
- Leftover from previous configuration
- Testing a new tool before adding to baseline

**Resolution options:**
- **Adopt** - Add tool to baseline configuration
- **Revert** - Uninstall the extra tool
- **Skip** - Decide later

**Example:**
```
rust
  Baseline:  (not declared)
  Active:    1.75.0 (managed by ZERB)
```

---

### External Override ⚠️

**Description:** An external installation (system package manager) is taking precedence over ZERB's installation.

**Common causes:**
- System package manager installed the tool (apt, brew, yum, etc.)
- User installed tool via version manager (nvm, pyenv, rbenv, etc.)
- Tool was in system PATH before ZERB activation
- Shell activation order causing system tools to take precedence

**Resolution options:**
- **Adopt** - Remove from ZERB baseline, acknowledge external management
- **Revert** - Reinstall via ZERB (may conflict with system installation)
- **Skip** - Decide later

**Example:**
```
node
  Baseline:  20.11.0 (managed by ZERB)
  Active:    18.17.1 at /usr/bin/node
```

**Note:** Choosing "Revert" will reinstall via ZERB, but the external installation may still take precedence depending on your PATH configuration. Consider adjusting your shell configuration or removing the system installation.

---

### Managed But Not Active

**Description:** ZERB has installed the tool but it's not accessible in PATH.

**Common causes:**
- Shell integration not activated (`eval "$(zerb activate bash)"` not in rc file)
- New shell session needs to be started after ZERB activation
- PATH configuration issue
- Shell activation script not executed

**Resolution options:**
- **Adopt** - Remove from baseline if you don't need it in PATH
- **Skip** - Investigate PATH configuration

**Example:**
```
ripgrep
  Baseline:  13.0.0
  Managed:   13.0.0 (managed by ZERB)
  Active:    (not in PATH)
```

**How to fix:**
1. Check if shell integration is active: `echo $ZERB_ACTIVE`
2. If not active, ensure your shell rc file has the activation line
3. Restart your shell or source the rc file
4. Run `zerb drift` again

---

### Version Unknown

**Description:** Tool is found but version cannot be detected.

**Common causes:**
- Tool doesn't support `--version` or `-v` flags
- Tool version output is in non-standard format
- Binary is a wrapper script that doesn't pass through version flag

**Resolution options:**
- **Adopt** - Remove from baseline if version detection isn't important
- **Revert** - Reinstall to potentially fix detection
- **Skip** - Accept unknown version for now

**Example:**
```
mystery-tool
  Baseline:  1.0.0
  Active:    unknown (managed by ZERB)
```

**Note:** ZERB only tries `--version` and `-v` flags. If your tool uses a different format (e.g., `version` subcommand), it will be marked as unknown.

## Resolution Modes

### Individual Mode (Default)

Choose an action for each drift one at a time. This gives you fine-grained control and is recommended when you have multiple types of drift.

**When to use:**
- You want different actions for different drifts
- You need to carefully review each discrepancy
- Mixed drift types requiring different resolutions

**Example workflow:**
```
node (external override)
  1. Adopt (remove from baseline)
  2. Revert (reinstall via ZERB)
  3. Skip

Choice [1]: 1
✓ Applied Adopt for node

python (version mismatch)
  1. Adopt (update baseline to 3.11.0)
  2. Revert (reinstall 3.12.1)
  3. Skip

Choice [1]: 2
✓ Applied Revert for python
```

---

### Adopt All

Update your baseline configuration to match your current environment for **all** detected drifts.

**When to use:**
- You've made many manual changes you want to keep
- Current environment is your desired state
- You want baseline to reflect reality

**What it does:**
- External overrides → Removes from baseline
- Version mismatches → Updates baseline version
- Extra tools → Adds to baseline
- Missing tools → Removes from baseline

**Example:**
```bash
$ zerb drift
# ... drift report shows 5 drifts ...

Choice: 2 (Adopt All)

✓ Adopted node
✓ Adopted python
✓ Adopted go
✓ Adopted rust
✓ Adopted ripgrep

Adopted 5 drift(s). Baseline updated to match environment.
```

---

### Revert All

Restore your environment to match your baseline configuration for **all** detected drifts.

**When to use:**
- Baseline is your source of truth
- Environment has drifted and needs to be reset
- Fresh setup after configuration pull

**What it does:**
- External overrides → Reinstalls via ZERB
- Version mismatches → Reinstalls correct version
- Extra tools → Uninstalls
- Missing tools → Installs

**Example:**
```bash
$ zerb drift
# ... drift report shows 5 drifts ...

Choice: 3 (Revert All)

✓ Reverted node
✓ Reverted python
✓ Reverted go
✓ Reverted rust
✓ Reverted ripgrep

Reverted 5 drift(s). Environment restored to match baseline.
```

**Warning:** This will reinstall/uninstall tools and may take time. Consider using `--dry-run` first to preview changes.

---

### Show Only

Display the drift report without making any changes. Useful for periodic checks or CI/CD integration.

**When to use:**
- Checking drift status without resolution
- CI/CD drift detection tests
- Periodic environment health checks
- Understanding current state before manual fixes

---

## Version Detection Caching

ZERB caches version detection results to avoid repeated subprocess calls when checking large tool lists.

**Cache behavior:**
- **TTL:** 5 minutes
- **Cache key:** Binary path
- **Invalidation:** Automatic after expiration
- **Override:** Use `--force-refresh` flag to bypass cache

**Why caching?**
- Detecting versions requires executing binaries
- Large tool lists can take significant time
- Multiple drift checks in quick succession benefit from caching
- Balances performance with accuracy

**When to use `--force-refresh`:**
- You just updated a tool manually
- Cache might contain stale data
- Debugging version detection issues

```bash
# Use cached versions (default)
zerb drift

# Bypass cache and detect fresh
zerb drift --force-refresh
```

## Best Practices

### Regular Drift Checks

Run `zerb drift` regularly to catch issues early:

```bash
# Weekly check (add to crontab or shell startup)
zerb drift --dry-run
```

### After External Tool Installation

If you install a tool via system package manager, check for drift:

```bash
# Install via apt/brew
sudo apt install nodejs

# Check drift
zerb drift

# Decide: adopt (use system version) or revert (use ZERB version)
```

### Before Syncing to Remote

Check for drift before pushing configuration changes:

```bash
zerb drift --dry-run
# Review drift report
# Resolve as needed
zerb sync
```

### On New Machine Setup

After pulling configuration on a new machine:

```bash
zerb pull
zerb drift
# Should show missing tools
# Choose "Revert all" to install everything
```

## Troubleshooting

### Drift Detection Finds Nothing

**Symptom:** `zerb drift` shows no drifts even though tools seem wrong.

**Possible causes:**
1. ZERB not initialized - Run `zerb init`
2. Shell integration not active - Check activation in rc file
3. No tools in baseline - Add tools to `zerb.lua`

**Solution:**
```bash
# Verify ZERB is initialized
ls ~/.config/zerb/zerb.lua.active

# Check shell activation
echo $ZERB_ACTIVE  # Should output "1" or similar

# Verify baseline has tools
cat ~/.config/zerb/zerb.lua.active
```

---

### External Override Won't Resolve

**Symptom:** After choosing "Revert", tool still shows as external override.

**Possible causes:**
1. System PATH takes precedence over ZERB's PATH
2. Shell rc file has wrong order of PATH additions
3. Multiple shell integrations conflicting

**Solution:**
```bash
# Check PATH order
echo $PATH | tr ':' '\n'

# Ensure ZERB's path comes first
# In your rc file (~/.bashrc or ~/.zshrc):
eval "$(zerb activate bash)"  # Should be early in file
```

---

### Version Detection Fails (Unknown)

**Symptom:** Tools consistently show "version unknown" in drift report.

**Possible causes:**
1. Tool doesn't support standard version flags
2. Tool is a shell alias or wrapper
3. Binary is corrupted

**Solution:**
```bash
# Test version detection manually
/path/to/tool --version
/path/to/tool -v

# Check if it's an alias
type tool-name

# Verify binary is executable
ls -la /path/to/tool
```

---

### Drift Persists After Resolution

**Symptom:** Same drifts appear even after resolving.

**Possible causes:**
1. Resolution action failed (check error messages)
2. Configuration not saved properly
3. Shell needs restart to pick up changes

**Solution:**
```bash
# Check for error messages in drift output
zerb drift -v  # Verbose mode

# Verify configuration was updated
cat ~/.config/zerb/zerb.lua.active

# Restart shell
exec $SHELL
```

## Related Commands

- `zerb init` - Initialize ZERB environment
- `zerb sync` - Sync configuration and tools
- `zerb activate <shell>` - Generate shell activation script

## See Also

- [Configuration Guide](../README.md#configuration)
- [Architecture Overview](../README.md#architecture-overview)
- [Quick Start Guide](../README.md#quick-start)
