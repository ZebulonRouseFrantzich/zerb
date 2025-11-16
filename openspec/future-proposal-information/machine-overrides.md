# 08-Machine-Specific Overrides

**Status**: Not Started  
**Last Updated**: 2025-11-11  
**Dependencies**: ✅ 02-lua-config (COMPLETED - Lua parsing ready), 07-git-operations (versioning)

---

## Overview

Machine-specific overrides enable users to customize their environment per-machine while maintaining a shared baseline configuration. Overrides can be plain (safe for public repos) or encrypted (for sensitive work configurations), and are fully git-versioned with disaster recovery support.

### Why This Matters

- **Flexibility**: Different machines need different configurations (work laptop vs personal desktop)
- **Privacy**: Encrypt sensitive work configs while keeping personal configs public
- **Reproducibility**: All overrides are git-versioned and recoverable
- **Disaster Recovery**: Passphrase-protected key backups enable recovery on new machines
- **Sharing**: Multi-recipient encryption allows team collaboration
- **Abstraction**: Users work with "profiles", not age/chezmoi internals

### Key Principles

- **Optional encryption**: Users choose plain or encrypted per profile
- **Git-friendly**: Both plain and encrypted profiles commit to git
- **Hybrid recovery**: Key-based encryption + passphrase backup for disaster recovery
- **Convertible**: Encrypt/decrypt existing profiles without data loss
- **Merge semantics**: Clear, predictable baseline + override merging
- **User-facing abstraction**: Never expose age/chezmoi in user messages

---

## Development Environment Dependencies

### Nix Flake Packages

The following packages are included in the Nix dev shell (`flake.nix`) for Component 08:

```nix
# From flake.nix - Component 08 section
age          # Encryption/decryption for profiles
```

**Purpose:**
- **age**: Modern encryption tool for profile encryption, key generation, and passphrase-protected backups

**Note**: age is already included in Component 00's flake.nix for secrets testing. Component 08 uses it for production profile encryption.

### Go Dependencies

No additional Go dependencies required beyond existing:

```go
require (
    github.com/yuin/gopher-lua v1.1.1     // Already in Component 02
    github.com/go-git/go-git/v5 v5.11.0   // Already in Component 07
)
```

**Why no age Go library?**
- Shell out to age binary (simpler, more reliable)
- age CLI is stable and well-documented
- Avoids CGo dependencies
- Easier to test and debug

### Testing Tools

Component 08 testing requires:

- **age**: Encryption/decryption operations
- **go test**: Unit tests for merge logic
- **Temporary files**: Test profile creation and encryption
- **Mock age operations**: Stub encryption for fast tests

### Development Workflow

```bash
# Enter Nix dev shell
nix develop

# Verify age available
age --version

# Run component tests
just test-one TestProfileOperations

# Test encryption workflow
just test-one TestProfileEncryption

# Test merge logic
just test-one TestConfigMerge

# Test disaster recovery
just test-one TestDisasterRecovery
```

### Testing Profile Operations

Create test profiles with encryption:

```go
// internal/profile/testutil.go
package profile

import (
    "os"
    "path/filepath"
    "testing"
)

// CreateTestProfile creates a test profile (plain or encrypted)
func CreateTestProfile(t *testing.T, name string, encrypted bool) string {
    t.Helper()
    
    dir := t.TempDir()
    profileDir := filepath.Join(dir, "overrides")
    os.MkdirAll(profileDir, 0755)
    
    content := `return {
        meta = { description = "Test profile" },
        tools_add = { "node@20.11.0" },
    }`
    
    if encrypted {
        // Generate test key
        keyPath := filepath.Join(dir, ".age-identity")
        GenerateTestKey(t, keyPath)
        
        // Encrypt profile
        profilePath := filepath.Join(profileDir, name+".lua.age")
        EncryptContent(t, content, profilePath, keyPath)
        
        return profilePath
    }
    
    // Plain profile
    profilePath := filepath.Join(profileDir, name+".lua")
    os.WriteFile(profilePath, []byte(content), 0644)
    return profilePath
}

// GenerateTestKey generates a test age key
func GenerateTestKey(t *testing.T, keyPath string) {
    t.Helper()
    
    cmd := exec.Command("age-keygen", "-o", keyPath)
    if err := cmd.Run(); err != nil {
        t.Fatalf("failed to generate test key: %v", err)
    }
}

// EncryptContent encrypts content with age
func EncryptContent(t *testing.T, content, outPath, keyPath string) {
    t.Helper()
    
    // Extract public key from identity
    pubKey := ExtractPublicKey(t, keyPath)
    
    // Encrypt
    cmd := exec.Command("age", "-r", pubKey, "-o", outPath)
    cmd.Stdin = strings.NewReader(content)
    if err := cmd.Run(); err != nil {
        t.Fatalf("failed to encrypt: %v", err)
    }
}

// DecryptContent decrypts age-encrypted content
func DecryptContent(t *testing.T, encPath, keyPath string) string {
    t.Helper()
    
    cmd := exec.Command("age", "-d", "-i", keyPath, encPath)
    output, err := cmd.Output()
    if err != nil {
        t.Fatalf("failed to decrypt: %v", err)
    }
    return string(output)
}
```

### Testing Merge Logic

Test baseline + override merging:

```go
func TestConfigMerge(t *testing.T) {
    tests := []struct {
        name     string
        baseline Config
        override Override
        want     Config
    }{
        {
            name: "Add tools",
            baseline: Config{
                Tools: []string{"node@20.11.0"},
            },
            override: Override{
                ToolsAdd: []string{"python@3.12.1"},
            },
            want: Config{
                Tools: []string{"node@20.11.0", "python@3.12.1"},
            },
        },
        {
            name: "Remove tools",
            baseline: Config{
                Tools: []string{"node@20.11.0", "python@3.11.0"},
            },
            override: Override{
                ToolsRemove: []string{"python"},
            },
            want: Config{
                Tools: []string{"node@20.11.0"},
            },
        },
        {
            name: "Override tool version",
            baseline: Config{
                Tools: []string{"node@20.11.0"},
            },
            override: Override{
                ToolsOverride: map[string]string{"node": "21.0.0"},
            },
            want: Config{
                Tools: []string{"node@21.0.0"},
            },
        },
        {
            name: "Config overrides (deep merge)",
            baseline: Config{
                Configs: map[string]interface{}{
                    "~/.gitconfig": map[string]interface{}{
                        "user": map[string]string{
                            "name": "Base User",
                            "email": "base@example.com",
                        },
                    },
                },
            },
            override: Override{
                ConfigOverrides: map[string]interface{}{
                    "~/.gitconfig": map[string]interface{}{
                        "user": map[string]string{
                            "email": "override@work.com",
                        },
                    },
                },
            },
            want: Config{
                Configs: map[string]interface{}{
                    "~/.gitconfig": map[string]interface{}{
                        "user": map[string]string{
                            "name": "Base User",
                            "email": "override@work.com",
                        },
                    },
                },
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := MergeConfig(tt.baseline, tt.override)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Testing Disaster Recovery

Test passphrase backup and recovery:

```go
func TestDisasterRecovery(t *testing.T) {
    // Create encrypted profile with passphrase backup
    dir := t.TempDir()
    
    // Generate key
    keyPath := filepath.Join(dir, ".age-identity")
    GenerateTestKey(t, keyPath)
    
    // Create passphrase-protected backup
    backupPath := filepath.Join(dir, "overrides/.identities/work.identity.age")
    passphrase := "test-passphrase-123"
    CreatePassphraseBackup(t, keyPath, backupPath, passphrase)
    
    // Simulate disaster: delete original key
    os.Remove(keyPath)
    
    // Recover from backup
    recoveredKeyPath := filepath.Join(dir, ".age-identity-recovered")
    RecoverFromBackup(t, backupPath, recoveredKeyPath, passphrase)
    
    // Verify recovered key works
    originalPubKey := ExtractPublicKey(t, keyPath)
    recoveredPubKey := ExtractPublicKey(t, recoveredKeyPath)
    assert.Equal(t, originalPubKey, recoveredPubKey)
}
```

### Environment Variables

Component 08 uses:

```bash
# Set by Nix dev shell
export ZERB_DEV=1              # Enable debug logging
export ZERB_TEST_MODE=1        # Use test directories

# Component-specific (for testing)
export ZERB_PROFILE_DIR=path/to/test/overrides
export ZERB_AGE_IDENTITY=path/to/test/key
export ZERB_SKIP_ENCRYPTION=1  # Skip encryption in tests (when needed)
```

### Manual Testing Checklist

Before completing Component 08:

- [ ] age available: `age --version`
- [ ] Create plain profile works
- [ ] Create encrypted profile works
- [ ] Encrypt existing profile works
- [ ] Decrypt existing profile works
- [ ] Passphrase backup creation works
- [ ] Disaster recovery from backup works
- [ ] Multi-recipient encryption works
- [ ] Config merge logic correct
- [ ] Profile switching works
- [ ] Profile editing (decrypts, edits, re-encrypts)
- [ ] Git commits encrypted profiles correctly
- [ ] Chezmoi receives merged data

---

## Design (Decided)

### File Structure

```
~/.config/zerb/
├── configs/
│   └── zerb.lua.20250115T143022Z    # Baseline (always plain)
├── overrides/
│   ├── personal.lua                 # Plain profile
│   ├── work.lua.age                 # Encrypted profile
│   ├── client-acme.lua.age          # Another encrypted profile
│   ├── .identities/                 # Encrypted key backups (tracked in git)
│   │   ├── work.identity.age
│   │   └── client-acme.identity.age
│   └── .age-keys                    # Public keys (tracked in git)
├── .age-identity                    # Private key (gitignored)
└── .machine                         # Active profile name (gitignored)
```

### Profile Types

**Plain Profiles** (`name.lua`):
- Not encrypted
- Safe for public repositories
- Use cases: personal configs, open-source projects
- Committed directly to git

**Encrypted Profiles** (`name.lua.age`):
- Age-encrypted with user's key
- Safe for private work details
- Use cases: work configs, client projects, API keys
- Committed to git as encrypted blobs
- Passphrase-protected backup in `.identities/`

### Override File Format (Lua)

```lua
-- overrides/work.lua (before encryption)
return {
  meta = {
    description = "Work laptop configuration",
    created = "2025-01-15T14:30:22Z",
    machine = "work-laptop-01",
  },
  
  -- Override config file contents (deep merge with baseline)
  config_overrides = {
    ["~/.gitconfig"] = {
      user = {
        email = "me@work.com",  -- Overrides baseline email
      },
      core = {
        sshCommand = "ssh -i ~/.ssh/work_rsa",  -- Adds new config
      },
    },
    
    ["~/.config/ai/config.json"] = {
      model = "claude-3-opus",  -- Work uses different AI model
      api_key = "sk-work-key-123",  -- Sensitive data (encrypt this profile!)
    },
  },
  
  -- Add tools not in baseline
  tools_add = {
    "kubectl@1.28.0",      -- Work-specific tool
    "terraform@1.6.0",     -- Work-specific tool
  },
  
  -- Remove tools from baseline
  tools_remove = {
    "steam",  -- Don't need games on work laptop
  },
  
  -- Override tool versions from baseline
  tools_override = {
    ["python"] = "3.11.0",  -- Work requires older Python
    ["node"] = "18.19.0",   -- Work project uses LTS
  },
}
```

### Merge Algorithm

**Merge Order**: Baseline + Override = Final Config

**Merge Rules**:

1. **Scalars** (strings, numbers, booleans):
   - Override replaces baseline
   - Example: `baseline.email = "personal@example.com"` + `override.email = "work@company.com"` → `"work@company.com"`

2. **Arrays** (tools list):
   - Use explicit directives: `tools_add`, `tools_remove`, `tools_override`
   - `tools_add`: Append to baseline
   - `tools_remove`: Remove by tool name (not version)
   - `tools_override`: Replace specific tool version
   - Example:
     ```lua
     baseline.tools = {"node@20.11.0", "python@3.12.1"}
     override.tools_add = {"rust@1.75.0"}
     override.tools_remove = {"python"}
     override.tools_override = {["node"] = "21.0.0"}
     → final.tools = {"node@21.0.0", "rust@1.75.0"}
     ```

3. **Tables** (nested objects):
   - Deep merge by key
   - Override keys replace baseline keys
   - New keys are added
   - Example:
     ```lua
     baseline.gitconfig = {user = {name = "Base", email = "base@example.com"}}
     override.gitconfig = {user = {email = "work@company.com"}, core = {editor = "vim"}}
     → final.gitconfig = {
         user = {name = "Base", email = "work@company.com"},
         core = {editor = "vim"}
       }
     ```

4. **Nil values**:
   - Explicit `nil` in override removes key from final config
   - Example: `override.some_key = nil` → key removed

5. **Precedence**:
   - Override > Baseline (always)

### Encryption Workflow

**Create Encrypted Profile**:

```bash
$ zerb profile create work --encrypt

Create encrypted profile 'work'? This will:
  1. Generate a new age encryption key
  2. Create a passphrase-protected backup (stored in git)
  3. Encrypt the profile with your key

Passphrase for key backup (min 12 chars): ****************
Confirm passphrase: ****************

✓ Generated encryption key
✓ Created passphrase backup: overrides/.identities/work.identity.age
✓ Created encrypted profile: overrides/work.lua.age
✓ Git committed: Add encrypted profile 'work'

Profile 'work' created. Edit with: zerb profile edit work
```

**Encryption Implementation**:

```bash
# Generate key
age-keygen -o ~/.config/zerb/.age-identity

# Extract public key
age-keygen -y ~/.config/zerb/.age-identity > ~/.config/zerb/overrides/.age-keys

# Create passphrase backup
age -p -o overrides/.identities/work.identity.age ~/.config/zerb/.age-identity
# User enters passphrase interactively

# Encrypt profile
age -r $(cat ~/.config/zerb/overrides/.age-keys) \
    -o overrides/work.lua.age \
    overrides/work.lua.tmp

# Commit to git
git add overrides/work.lua.age overrides/.identities/work.identity.age overrides/.age-keys
git commit -m "Add encrypted profile 'work'"
```

**Decrypt for Editing**:

```bash
$ zerb profile edit work

Decrypting profile 'work'...
✓ Decrypted

Opening in $EDITOR...
# User edits in vim/nano/etc.

Save changes? [Y/n]: y

Re-encrypting profile...
✓ Encrypted
✓ Git committed: Update profile 'work'
```

**Decryption Implementation**:

```bash
# Decrypt to temp file
age -d -i ~/.config/zerb/.age-identity \
    overrides/work.lua.age > /tmp/work.lua.tmp

# User edits /tmp/work.lua.tmp

# Re-encrypt
age -r $(cat ~/.config/zerb/overrides/.age-keys) \
    -o overrides/work.lua.age \
    /tmp/work.lua.tmp

# Clean up temp file
shred -u /tmp/work.lua.tmp
```

### Disaster Recovery Workflow

**Scenario**: New machine, need to recover encrypted profile

```bash
$ git clone https://github.com/user/dotfiles ~/.config/zerb
$ cd ~/.config/zerb
$ zerb profile use work

Profile 'work' is encrypted but no decryption key found.

Recover from passphrase backup? [Y/n]: y

Enter backup passphrase: ****************

✓ Recovered encryption key from backup
✓ Saved to ~/.config/zerb/.age-identity
✓ Activated profile 'work'

Applying configuration...
✓ Installed kubectl@1.28.0
✓ Installed terraform@1.6.0
✓ Updated ~/.gitconfig
✓ Updated ~/.config/ai/config.json

Profile 'work' is now active.
```

**Recovery Implementation**:

```bash
# Decrypt passphrase-protected backup
age -d overrides/.identities/work.identity.age > ~/.config/zerb/.age-identity
# User enters passphrase interactively

# Verify key works
age -d -i ~/.config/zerb/.age-identity overrides/work.lua.age > /dev/null
```

### Multi-Recipient Encryption

**Add Recipient** (for team sharing):

```bash
$ zerb profile add-recipient work teammate-pubkey.txt

Adding recipient to profile 'work'...
✓ Re-encrypted with 2 recipients
✓ Git committed: Add recipient to profile 'work'

Your teammate can now decrypt this profile with their key.
```

**Implementation**:

```bash
# Re-encrypt with multiple recipients
age -r $(cat ~/.config/zerb/overrides/.age-keys) \
    -r $(cat teammate-pubkey.txt) \
    -o overrides/work.lua.age \
    overrides/work.lua.tmp
```

### Profile Lifecycle Commands

**Create**:
```bash
zerb profile create <name> [--encrypt]
```

**List**:
```bash
$ zerb profile list

Available profiles:
  personal       (plain)
  work           (encrypted) ← active
  client-acme    (encrypted)
```

**Use** (activate):
```bash
zerb profile use <name>
```

**Edit**:
```bash
zerb profile edit <name>
# Decrypts if encrypted, opens in $EDITOR, re-encrypts on save
```

**Show** (view without editing):
```bash
zerb profile show <name>
# Decrypts if encrypted, displays content, doesn't save
```

**Delete**:
```bash
zerb profile delete <name>
# Removes profile and backup (if encrypted)
```

**Current**:
```bash
$ zerb profile current
work
```

### Conversion Commands

**Encrypt** (plain → encrypted):
```bash
$ zerb profile encrypt personal

Convert profile 'personal' to encrypted? This will:
  1. Generate a new encryption key (or use existing)
  2. Create a passphrase-protected backup
  3. Encrypt the profile

Continue? [Y/n]: y

Passphrase for key backup: ****************
Confirm passphrase: ****************

✓ Encrypted profile 'personal'
✓ Created backup: overrides/.identities/personal.identity.age
✓ Git committed: Encrypt profile 'personal'
```

**Decrypt** (encrypted → plain):
```bash
$ zerb profile decrypt work

⚠️  WARNING: This will decrypt profile 'work' and store it as plain text.
   Sensitive data will be visible in git history.

Continue? [y/N]: y

✓ Decrypted profile 'work'
✓ Removed backup: overrides/.identities/work.identity.age
✓ Git committed: Decrypt profile 'work'
```

**Rekey** (change encryption key):
```bash
$ zerb profile rekey work

Generate new encryption key for profile 'work'? This will:
  1. Generate a new age key
  2. Create a new passphrase-protected backup
  3. Re-encrypt the profile with the new key

Old key will be invalidated.

Continue? [Y/n]: y

Passphrase for new key backup: ****************
Confirm passphrase: ****************

✓ Generated new encryption key
✓ Re-encrypted profile 'work'
✓ Created new backup
✓ Git committed: Rekey profile 'work'
```

### Sharing Commands

**Export** (share key with teammate):
```bash
$ zerb profile export work

Export encryption key for profile 'work'?

⚠️  WARNING: Anyone with this key can decrypt the profile.
   Only share with trusted recipients.

Export to: work-key.txt

✓ Exported public key to work-key.txt

Share this file with your teammate. They can import with:
  zerb profile import work work-key.txt
```

**Import** (receive shared profile):
```bash
$ zerb profile import work teammate-key.txt

Import encryption key for profile 'work'?
This will allow you to decrypt this profile.

✓ Imported key
✓ Profile 'work' is now accessible

Use with: zerb profile use work
```

**Add Recipient**:
```bash
zerb profile add-recipient <name> <pubkey-file>
```

**Remove Recipient**:
```bash
zerb profile remove-recipient <name> <pubkey-file>
```

### Backup Commands

**Create Backup**:
```bash
$ zerb profile backup create work

Create passphrase-protected backup for profile 'work'?

Passphrase (min 12 chars): ****************
Confirm passphrase: ****************

✓ Created backup: overrides/.identities/work.identity.age
✓ Git committed: Add backup for profile 'work'
```

**Restore Backup**:
```bash
$ zerb profile backup restore work

Restore encryption key from backup?

Enter backup passphrase: ****************

✓ Restored encryption key
✓ Saved to ~/.config/zerb/.age-identity
```

**Test Backup**:
```bash
$ zerb profile backup test work

Testing backup for profile 'work'...

Enter backup passphrase: ****************

✓ Backup is valid
✓ Can decrypt profile successfully
```

### Chezmoi Integration

**Data Injection**:

ZERB injects the merged config (baseline + override) into chezmoi templates:

```go
// Merge baseline + active override
mergedConfig := MergeConfig(baseline, activeOverride)

// Convert to chezmoi data format
chezmoiData := map[string]interface{}{
    "zerb": mergedConfig,
}

// Write to chezmoi data file
WriteChezmoiData("~/.config/zerb/chezmoi-data.json", chezmoiData)

// chezmoi templates can now access:
// {{ .zerb.tools }}
// {{ .zerb.configs }}
// {{ .zerb.git }}
```

**Template Example**:

```
# ~/.config/zerb/managed/dot_gitconfig.tmpl
[user]
    name = {{ .zerb.git.user.name }}
    email = {{ .zerb.git.user.email }}

[core]
    editor = {{ .zerb.editor | default "vim" }}
{{- if .zerb.git.core.sshCommand }}
    sshCommand = {{ .zerb.git.core.sshCommand }}
{{- end }}
```

---

## Open Questions

### 3.1 Merge Semantics

🔴 **Question:** What are the exact merge semantics for nested objects with conflicting keys at different depths?

**Example:**
```lua
baseline.config = {
  section = {
    key1 = "base",
    key2 = "base",
    nested = {
      deep = "base"
    }
  }
}

override.config = {
  section = {
    key2 = "override",
    nested = {
      other = "override"
    }
  }
}

-- Result?
final.config = {
  section = {
    key1 = "base",        -- Kept from baseline
    key2 = "override",    -- Replaced by override
    nested = {
      deep = "base",      -- Kept from baseline
      other = "override"  -- Added from override
    }
  }
}
```

**Recommendation:** Deep merge recursively. Override keys replace, new keys add, missing keys keep baseline.

---

🔴 **Question:** How do we handle merge conflicts when both baseline and override change the same nested key between syncs?

**Scenario:**
- Machine A: Baseline has `email = "old@example.com"`
- Machine A: Override changes to `email = "new@example.com"`
- Machine B: Baseline changes to `email = "different@example.com"`
- Machine A pulls from B

**Options:**
1. Override always wins (ignore baseline changes)
2. Detect conflict, prompt user
3. Baseline wins, override becomes stale
4. Three-way merge with conflict markers

**Recommendation:** Option 1 - Override always wins. Overrides are machine-specific by design.

---

🟡 **Question:** Should we support array merge strategies beyond add/remove/override?

**Potential strategies:**
- Prepend (add to beginning)
- Insert at index
- Sort after merge
- Deduplicate

**Recommendation:** Not for MVP. Add/remove/override covers 95% of use cases.

---

### 3.2 Encryption & Security

🔴 **Question:** How do we warn users about weak passphrases without being annoying?

**Options:**
1. Enforce minimum length (12 chars) and complexity
2. Use zxcvbn-style strength meter
3. Warn but allow weak passphrases
4. Require strong passphrases, no exceptions

**Recommendation:** Option 1 for MVP - Enforce 12 char minimum, warn if no special chars/numbers.

---

🟡 **Question:** Should we support age version compatibility checks across machines?

**Scenario:** Machine A uses age 1.1.0, Machine B uses age 1.2.0. Are encrypted files compatible?

**Recommendation:** Document minimum age version (1.1.0+), warn if version mismatch detected.

---

🟡 **Question:** How do we handle the case where a user forgets their passphrase?

**Options:**
1. No recovery possible (document clearly)
2. Support multiple backup passphrases
3. Support recovery questions
4. Support hardware key backup

**Recommendation:** Option 1 for MVP - No recovery. Document clearly: "Store passphrase in password manager."

---

### 3.3 Profile Management

🟡 **Question:** Should profiles support inheritance (profile extends another profile)?

**Example:**
```lua
-- overrides/work-base.lua
return {
  tools_add = {"kubectl", "terraform"},
}

-- overrides/work-client-a.lua
return {
  extends = "work-base",
  tools_add = {"helm"},  -- Adds to work-base tools
}
```

**Recommendation:** Not for MVP. Adds complexity. Post-MVP feature.

---

🟢 **Question:** Should we support profile templates/scaffolding?

**Example:**
```bash
zerb profile create work --template=kubernetes
# Pre-fills with kubectl, helm, k9s, etc.
```

**Recommendation:** Post-MVP. Nice-to-have but not essential.

---

🟢 **Question:** Should we support multiple active profiles simultaneously?

**Example:**
```bash
zerb profile use work,personal
# Merges both: baseline + work + personal
```

**Recommendation:** Post-MVP. Adds significant complexity to merge logic.

---

### 3.4 Integration

🟢 **Question:** Should we integrate with password managers for passphrase storage?

**Options:**
1. Support 1Password CLI (`op read`)
2. Support Bitwarden CLI (`bw get`)
3. Support pass (Unix password manager)
4. Support all of the above
5. Manual only (MVP)

**Recommendation:** Option 5 for MVP. Post-MVP can add password manager integration.

---

🟡 **Question:** How do we handle profile switching when there are uncommitted changes?

**Options:**
1. Require clean state (no uncommitted changes)
2. Auto-stash changes
3. Prompt user to commit or stash
4. Allow switching, warn about potential conflicts

**Recommendation:** Option 3 - Prompt user. Explicit is better than implicit.

---

🔴 **Question:** Should profile activation trigger immediate sync, or require explicit `zerb sync`?

**Options:**
1. Auto-sync on profile activation
2. Require explicit `zerb sync`
3. Prompt user: "Apply changes now?"

**Recommendation:** Option 3 - Prompt user. Gives control over when changes apply.

---

## Implementation Tracking

### Completed
- [ ] 

### In Progress
- [ ] 

### Blocked
- [ ] 

### Notes

_Space for implementation notes, decisions made during coding, etc._

---

## Testing Requirements

### Unit Tests

**Profile Creation:**
- [ ] Test creating plain profile
- [ ] Test creating encrypted profile
- [ ] Test profile name validation
- [ ] Test duplicate profile detection

**Encryption/Decryption:**
- [ ] Test age key generation
- [ ] Test public key extraction
- [ ] Test profile encryption
- [ ] Test profile decryption
- [ ] Test passphrase backup creation
- [ ] Test passphrase backup recovery
- [ ] Test multi-recipient encryption
- [ ] Test encryption with invalid key
- [ ] Test decryption with wrong key

**Merge Logic:**
- [ ] Test scalar override (string, number, boolean)
- [ ] Test array add
- [ ] Test array remove
- [ ] Test array override
- [ ] Test deep table merge
- [ ] Test nil value removal
- [ ] Test complex nested merge
- [ ] Test merge with empty override
- [ ] Test merge with empty baseline

**Profile Management:**
- [ ] Test profile listing
- [ ] Test profile activation
- [ ] Test profile editing (plain)
- [ ] Test profile editing (encrypted)
- [ ] Test profile deletion
- [ ] Test profile current
- [ ] Test profile show

**Conversion:**
- [ ] Test encrypt (plain → encrypted)
- [ ] Test decrypt (encrypted → plain)
- [ ] Test rekey (change encryption key)

**Sharing:**
- [ ] Test export public key
- [ ] Test import public key
- [ ] Test add recipient
- [ ] Test remove recipient

**Backup:**
- [ ] Test backup creation
- [ ] Test backup restoration
- [ ] Test backup validation

### Integration Tests

**End-to-End Workflows:**
- [ ] Create plain profile → edit → use → sync
- [ ] Create encrypted profile → edit → use → sync
- [ ] Encrypt existing profile → edit → sync
- [ ] Disaster recovery: clone repo → recover from backup → use profile
- [ ] Multi-recipient: share profile → teammate imports → both can decrypt
- [ ] Profile switching: use profile A → use profile B → verify configs change
- [ ] Merge conflict: baseline changes → override changes → verify override wins

**Chezmoi Integration:**
- [ ] Verify merged data injected into chezmoi
- [ ] Verify chezmoi templates receive correct data
- [ ] Verify config files generated correctly

**Git Integration:**
- [ ] Verify encrypted profiles commit correctly
- [ ] Verify passphrase backups commit correctly
- [ ] Verify .age-identity is gitignored
- [ ] Verify .machine is gitignored
- [ ] Verify commit messages correct

### Security Tests

**Encryption Security:**
- [ ] Verify encrypted files are not readable without key
- [ ] Verify passphrase backup requires correct passphrase
- [ ] Verify temp files are securely deleted (shred)
- [ ] Verify keys have correct permissions (0600)
- [ ] Verify no secrets in git history

**Passphrase Strength:**
- [ ] Test minimum length enforcement (12 chars)
- [ ] Test weak passphrase warning
- [ ] Test passphrase confirmation mismatch

### Error Handling Tests

**Missing Dependencies:**
- [ ] Test when age binary not found
- [ ] Test when age version too old

**Corrupted Files:**
- [ ] Test corrupted encrypted profile
- [ ] Test corrupted passphrase backup
- [ ] Test corrupted public key file

**Invalid Input:**
- [ ] Test invalid profile name
- [ ] Test invalid Lua syntax in override
- [ ] Test invalid merge directives
- [ ] Test missing required fields

**Permission Errors:**
- [ ] Test read-only profile directory
- [ ] Test unwritable key file
- [ ] Test unreadable encrypted profile

---

## References

### External Documentation
- [age encryption](https://github.com/FiloSottile/age)
- [age specification](https://age-encryption.org/)
- [Lua tables](https://www.lua.org/pil/2.5.html)
- [Deep merge algorithms](https://en.wikipedia.org/wiki/Merge_algorithm)

### Related Components
- [02-lua-config.md](02-lua-config.md) - Lua parsing and config structure
- [07-git-operations.md](07-git-operations.md) - Git versioning and commits
- [04-shell-integration.md](04-shell-integration.md) - Chezmoi integration

### Design Decisions
- [Decision 13: Machine-Specific Overrides](../decisions.md#decision-13-machine-specific-overrides) - Architecture and rationale

### Tools
- **age**: Modern encryption tool (https://age-encryption.org/)
- **chezmoi**: Dotfile manager (https://www.chezmoi.io/)
- **gopher-lua**: Lua VM in Go (https://github.com/yuin/gopher-lua)
