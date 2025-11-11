# ZERB

**Zero-hassle Effortless Reproducible Builds**

[![Development Status](https://img.shields.io/badge/status-pre--pre--alpha-red)](https://github.com/ZebulonRouseFrantzich/zerb)
[![License](https://img.shields.io/badge/license-MIT--0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21%2B-00ADD8)](https://go.dev/)

> ⚠️ **ACTIVE DEVELOPMENT - PRE-PRE-ALPHA STAGE**
> 
> ZERB is in early development and not yet ready for production use. APIs, commands, and configuration formats may change without notice. Use at your own risk and expect breaking changes.

---

## What is ZERB?

ZERB is a single-binary tool that provides a **declarative environment management** by intelligently wrapping mature tools ([mise](https://github.com/jdx/mise) for tools, [chezmoi](https://github.com/twpayne/chezmoi) for configs) with git-native versioning and complete isolation.
ZERB acts as the glue between these mature tools to make it much easier to manage your environment.

**One config file. One command. Reproducible environments everywhere.**

```lua
-- ~/.config/zerb/zerb.lua
zerb = {
  tools = {
    "node@20.11.0",
    "python@3.12.1",
    "cargo:ripgrep",
    "ubi:sharkdp/bat",
    platform.is_linux and "cargo:i3-msg" or nil,
  },
  
  configs = {
    "~/.zshrc",
    "~/.gitconfig",
    { path = "~/.config/nvim/", recursive = true },
  },
  
  git = {
    remote = "https://github.com/username/dotfiles",
    branch = "main",
  },
}
```

```bash
$ zerb sync  # Install tools, apply configs, sync across machines
```

---

## Why ZERB?

### The Problem

Managing development environments is fragile and time-consuming:

- **Tool chaos**: Multiple version managers (nvm, pyenv, rbenv, etc.) with different interfaces
- **Config drift**: Dotfiles scattered across machines, manually synced, easily out of date
- **System conflicts**: Global installations interfere with each other
- **No reproducibility**: "Works on my machine" because environments diverge
- **Manual tracking**: No single source of truth for what's installed

### The ZERB Solution

ZERB acts as **intelligent glue** between battle-tested tools:

1. **mise** - Handles ALL user-space tools (dev tools, CLI utilities, binaries)
2. **chezmoi** - Manages dotfiles, templates, and secrets
3. **ZERB** - Provides unified config, git integration, drift detection, and UX

**Key advantages:**

- ✅ **Single declarative config** - One Lua file defines your entire environment
- ✅ **Complete isolation** - Never conflicts with system packages or other tools
- ✅ **Git-native versioning** - Full history, rollback, sync across machines
- ✅ **Platform-aware** - Conditional logic for Linux distros, macOS, Windows
- ✅ **Drift detection** - Know when your environment diverges from declared state
- ✅ **Interactive UX** - Smart prompts for version selection and conflict resolution
- ✅ **Security-first** - GPG signature verification with SHA256 fallback
- ✅ **Transaction-based** - Resume interrupted operations safely

---

## Key Concepts

### Wrapping, Not Reinventing

ZERB doesn't reimplement package management or config management. Instead, it wraps mature tools with complete isolation:

**mise wrapper:**
- Installs tools via multiple backends (cargo, npm, ubi, github, core)
- Complete isolation via environment variables (`MISE_CONFIG_FILE`, `MISE_DATA_DIR`, `MISE_CACHE_DIR`)
- Shell integration via `mise activate` for global tool access
- Never conflicts with system mise installations

**chezmoi wrapper:**
- Manages dotfiles with template processing and secrets integration
- Complete isolation via CLI flags (`--source`, `--config`)
- Never touches existing chezmoi setups

**ZERB's role:**
- Parse Lua config and generate mise/chezmoi configs
- Manage git versioning with timestamped snapshots
- Detect drift between declared and actual state
- Provide unified, user-friendly interface

### Declarative Configuration

Everything is declared in `zerb.lua` using Lua for cross-platform logic:

```lua
zerb = {
  tools = {
    -- Exact version pinning
    "node@20.11.0",
    "python@3.12.1",
    
    -- Multiple backends
    "cargo:ripgrep",              -- From crates.io
    "npm:prettier",               -- From npm
    "ubi:sharkdp/bat",           -- Binary from GitHub
    
    -- Platform-specific conditionals
    platform.is_linux and "cargo:i3-msg" or nil,
    platform.is_macos and "yabai" or nil,
    platform.is_debian_family and "ubi:sharkdp/fd" or nil,
    platform.is_arch_family and "yay" or nil,
  },
  
  configs = {
    "~/.zshrc",
    "~/.gitconfig",
    {
      path = "~/.ssh/config",
      template = true,
      secrets = true,
      private = true,  -- chmod 600
    },
  },
}
```

### Git-Native Versioning

Every config change creates an immutable timestamped snapshot:

```
~/.config/zerb/
├── configs/
│   ├── zerb.lua.20250115T143022Z  # Latest
│   ├── zerb.lua.20250115T142510Z  # Previous
│   └── zerb.lua.20250115T141203Z  # Older
├── .zerb-active                    # Marker: "20250115T143022Z"
└── zerb.lua.active -> configs/...  # Symlink (local convenience)
```

- All configs tracked in git
- Full history and rollback capability
- Sync across machines via git push/pull
- Timestamped files never modified (immutable)

### Complete Isolation

ZERB maintains complete isolation from system tools:

```
~/.config/zerb/
├── bin/                    # ZERB's private binaries
│   ├── mise               # Isolated mise binary
│   └── chezmoi            # Isolated chezmoi binary
├── mise/                   # mise data directory
│   ├── config.toml        # Auto-generated from zerb.lua
│   ├── installs/          # Tools installed here
│   └── shims/             # Added to PATH via shell activation
└── chezmoi/               # chezmoi data directory
    └── source/            # Dotfiles source
```

**Benefits:**
- No conflicts with system package managers (apt, brew, etc.)
- No conflicts with existing mise/chezmoi installations
- Tools remain isolated but globally accessible via shell integration
- Clean uninstall (just delete `~/.config/zerb/`)

### Drift Detection

ZERB performs three-way state comparison to detect drift:

1. **Baseline** (declared): What's in `zerb.lua`
2. **Managed** (ZERB): What ZERB has installed
3. **Active** (environment): What's actually in PATH

This detects:
- External package manager interference (apt, brew, nvm, etc.)
- Version mismatches
- Missing tools
- Extra tools not in baseline
- System installations taking precedence over ZERB's

Interactive resolution with three modes:
- **Individual**: Choose action for each drift
- **Adopt all**: Update baseline to match environment
- **Revert all**: Restore environment to match baseline

---

## Quick Start

> **Note:** ZERB is not yet ready for installation. This section is a preview of the planned workflow.

```bash
# Initialize ZERB
$ zerb init

# Add tools interactively
$ zerb add python
# Select version from list, ZERB installs and updates baseline

# Add tools with specific versions
$ zerb add node@20.11.0 rust@1.75.0

# Track configuration files
$ zerb config add ~/.zshrc
$ zerb config add ~/.config/nvim/ --recursive

# Check for drift
$ zerb drift
# Interactive resolution of any differences

# Sync to remote
$ zerb push

# On another machine
$ zerb pull
# Automatically installs tools and applies configs
```

---

## Features

### v1.0 Roadmap

#### Core Tool Management
- [ ] Download and verify mise/chezmoi binaries (GPG + SHA256)
- [ ] Install tools via mise (all backends: cargo, npm, ubi, github, core)
- [ ] Complete isolation (environment variables + CLI flags)
- [ ] Shell integration (`mise activate`)
- [ ] Interactive version selection with caching (24-hour TTL)
- [ ] Non-interactive version flags (`@version`, `--latest`)
- [ ] Exact version pinning (no ranges in MVP)
- [ ] Tool upgrade management
- [ ] Tool removal

#### Configuration Management
- [ ] Track dotfiles via chezmoi
- [ ] Recursive directory tracking
- [ ] Template processing support
- [ ] Secrets integration (1Password, Bitwarden, age)
- [ ] Private file permissions (chmod 600)
- [ ] Config diff and preview
- [ ] Config rollback

#### Platform Detection
- [ ] Linux distro detection (Ubuntu, Arch, Fedora, Alpine, RHEL/CentOS, openSUSE, Gentoo)
- [ ] Linux family detection (Debian, RHEL, Fedora, Arch, Alpine, SUSE, Gentoo)
- [ ] Architecture detection and normalization (amd64, arm64 only in MVP)
- [ ] Graceful fallback if distro detection fails
- [ ] Platform-aware conditionals in Lua
- [ ] Read-only platform table injection at VM initialization
- [ ] `zerb platform` command for debugging
- [ ] macOS detection (basic GOOS/GOARCH, post-MVP)
- [ ] Windows detection (basic GOOS/GOARCH, post-MVP)

#### Git Integration
- [ ] Automatic git initialization
- [ ] Timestamped config snapshots in `configs/` subdirectory
- [ ] Simple, readable commit messages
- [ ] Pre-commit hook with 5 integrity checks
- [ ] Comprehensive .gitignore template
- [ ] Config history and rollback
- [ ] Remote sync (push/pull)
- [ ] Stash recovery workflow
- [ ] Baseline comparison
- [ ] ZERB-guided conflict resolution
- [ ] Stash management commands
- [ ] Pre-push validation

#### Drift Detection & Resolution
- [ ] Three-way state comparison (baseline, managed, active)
- [ ] External override detection (system package managers)
- [ ] Interactive drift resolution (individual mode)
- [ ] Bulk resolution modes (adopt all, revert all)
- [ ] Resume capability for interrupted operations
- [ ] Drift-aware sync behavior
- [ ] User-facing terminology abstraction
- [ ] No persistent ignore (conscious decision-making)

#### Error Handling & Recovery
- [ ] Transaction-based resume for multi-step operations
- [ ] Active secret redaction in logs
- [ ] Corrupted config recovery with rollback
- [ ] Graceful offline degradation
- [ ] Preflight checks (permissions, disk space, network)
- [ ] Atomic writes for critical files
- [ ] Consistent error messages and exit codes
- [ ] Retry logic with exponential backoff
- [ ] Config validation (`zerb config validate`)
- [ ] Interactive repair tool (`zerb config repair`)
- [ ] Log management and auto-cleanup (7-day retention)

#### Security Features
- [ ] GPG signature verification (preferred)
- [ ] SHA256 checksum verification (fallback)
- [ ] Embedded GPG keyrings
- [ ] No mirror fallback (security-first)
- [ ] Hard-coded binary versions (reproducible builds)
- [ ] Secret detection in pre-commit hook
- [ ] Comprehensive .gitignore (prevent credential leaks)
- [ ] Active secret redaction in logs

#### User Experience
- [ ] Interactive version selection with pagination
- [ ] Smart prompts for conflict resolution
- [ ] Progress indicators for long operations
- [ ] Helpful error messages with suggestions
- [ ] Consistent command structure
- [ ] Non-interactive mode support
- [ ] Verbose logging flag
- [ ] Dry-run mode
- [ ] Shell completion (bash, zsh, fish)

---

## Platform Support

### MVP (v1.0)
- **Linux**: Linux Mint (primary target)
  - Full distro detection (Ubuntu, Arch, Fedora, Alpine, RHEL/CentOS, openSUSE, Gentoo)
  - Family detection (Debian, RHEL, Fedora, Arch, Alpine, SUSE, Gentoo)
  - Platform-aware conditionals in Lua config
  - Architectures: amd64 and arm64 only (error on i386, arm 32-bit)
  - Graceful fallback if distro detection fails (continues with OS/arch only)

### Post-MVP
- **macOS**: Basic support (GOOS/GOARCH only)
  - No distro detection (distro field will be nil)
  - Apple Silicon detection via runtime.GOARCH
  - Rosetta 2: Reports binary's compiled architecture
- **Windows**: Basic support (GOOS/GOARCH only)
  - No distro detection (distro field will be nil)

---

## Configuration

### Lua-Based Declarative Config

ZERB uses Lua for configuration, providing:
- Cross-platform conditional logic
- Programmatic generation (easy CLI modification)
- Future-proof (can migrate implementations transparently)
- Familiar syntax (used by Neovim, Hammerspoon, Nginx)

### Platform API

Read-only platform table injected by ZERB:

```lua
platform = {
  os = "linux",            -- "linux" | "darwin" | "windows"
  arch = "amd64",          -- normalized: "amd64" | "arm64"
  arch_raw = "x86_64",     -- original GOARCH if needed
  
  -- Boolean helpers
  is_linux = true,
  is_macos = false,
  is_windows = false,
  is_amd64 = true,
  is_arm64 = false,
  is_apple_silicon = false,
  
  -- Linux-only (nil on macOS/Windows)
  distro = { id = "ubuntu", family = "debian", version = "22.04" },
  linux_family = "debian",
  
  -- Family booleans
  is_debian_family = true,
  is_rhel_family = false,
  is_arch_family = false,
  is_alpine = false,
  
  -- Helper function
  when = function(cond, value) return cond and value or nil end,
}
```

### Configuration Schema

```lua
zerb = {
  -- Metadata
  meta = {
    name = "My Development Environment",
    description = "Full-stack web development setup",
  },
  
  -- Tool Management (via mise)
  tools = {
    "node@20.11.0",
    "python@3.12.1",
    "cargo:ripgrep",
    "npm:prettier",
    "ubi:sharkdp/bat",
    platform.is_linux and "cargo:i3-msg" or nil,
  },
  
  -- Configuration Files (via chezmoi)
  configs = {
    "~/.zshrc",
    "~/.gitconfig",
    {
      path = "~/.config/nvim/",
      recursive = true,
    },
    {
      path = "~/.ssh/config",
      template = true,
      secrets = true,
      private = true,
    },
  },
  
  -- Git Integration
  git = {
    remote = "https://github.com/username/dotfiles",
    branch = "main",
  },
  
  -- Configuration
  config = {
    backup_retention = 5,  -- Keep last 5 timestamped configs
  },
}
```

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     USER INTERFACE                          │
│  $ zerb add node@20                                         │
│  $ zerb config add ~/.zshrc                                 │
│  $ zerb sync                                                │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    ZERB CORE (Go)                           │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │ CLI Handler  │  │  Lua Parser  │  │  Git Manager     │   │
│  │ (cobra)      │  │ (gopher-lua) │  │  (go-git)        │   │
│  └──────────────┘  └──────────────┘  └──────────────────┘   │
│                                                             │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Config Manager                                 │ │
│  │  - Parses zerb.lua                                     │ │
│  │  - Generates mise/chezmoi configs                      │ │
│  │  - Manages timestamped configs                         │ │
│  │  - Handles drift detection                             │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Binary Manager                                 │ │
│  │  - Downloads mise/chezmoi from GitHub                  │ │
│  │  - GPG signature verification                          │ │
│  │  - SHA256 checksum verification                        │ │
│  └────────────────────────────────────────────────────────┘ │
└────────┬────────────────────────┬───────────────────────────┘
         │                        │
    ┌────▼────┐              ┌────▼────────┐
    │  mise   │              │   chezmoi   │
    │ Wrapper │              │   Wrapper   │
    └────┬────┘              └────┬────────┘
         │                        │
    ┌────▼────────────┐       ┌───▼─────────────┐
    │ ZERB's Private  │       │ ZERB's Private  │
    │ mise Binary     │       │ chezmoi Binary  │
    │                 │       │                 │
    │ Complete        │       │ Complete        │
    │ Isolation       │       │ Isolation       │
    └─────────────────┘       └─────────────────┘
```

### Technology Stack

- **Language**: Go 1.21+ (single binary)
- **CLI**: spf13/cobra, spf13/viper
- **Lua**: yuin/gopher-lua (pure Go, no CGO)
- **Git**: go-git/go-git
- **Security**: x/crypto/openpgp
- **Platform**: github.com/shirou/gopsutil/v4/host (scoped to host package only)

---

## Development Status & Roadmap

### Current Status: Pre-Pre-Alpha

ZERB is in active development. The project plan is complete, but implementation has not yet begun.

### Success Criteria (MVP)

- ✅ Works on Linux (Linux Mint, specifically)
- ✅ Downloads and verifies mise/chezmoi binaries
- ✅ Installs tools via mise (all backends)
- ✅ Manages configs via chezmoi
- ✅ Complete isolation (no system conflicts)
- ✅ Shell integration working
- ✅ Git versioning with timestamped configs
- ✅ Interactive version selection
- ✅ Drift detection working
- ✅ Config rollback functional
- ✅ Git operations complete
- ✅ >80% test coverage
- ✅ Comprehensive documentation

### Post-MVP Features

- macOS/Windows full support
- WSL/WSL2 detection
- LTS metadata management
- Fuzzy version matching
- Version recommendations
- Semantic version parsing
- System package managers (apt/brew)
- Rosetta detection
- Auto-drift correction (optional)
- Conventional commit messages (optional)

---

## Contributing

> **Note:** ZERB is not yet accepting contributions as the codebase is in early development. Once the MVP is complete and the API stabilizes, we'll welcome contributions.

### Development Approach

- **Test-Driven Development (TDD)**: Strict test-first approach
- **Coverage Goal**: >80% for all packages
- **Go Version**: 1.21+

### Development Environment

ZERB uses a Nix flake for reproducible development environments with all necessary tools and dependencies.

#### Quick Start

```bash
# Install Nix (if not already installed)
curl -L https://nixos.org/nix/install | sh

# Enable flakes (add to ~/.config/nix/nix.conf)
experimental-features = nix-command flakes

# Clone repository
git clone https://github.com/ZebulonRouseFrantzich/zerb.git
cd zerb

# Enter development environment
nix develop

# Initialize Go module (first time only)
just init

# Build and test
just build
just test
```

#### What's Included

The Nix dev shell provides:
- **Go 1.22** - Core language and toolchain
- **Development Tools** - golangci-lint, goimports, gopls, delve
- **Testing Tools** - gotestsum, go-junit-report
- **Component-Specific Tools** - Tools for each component (added as implemented)
  - Lua interpreter and luacheck
  - GPG tools for binary verification
  - Shell testing tools (bash, zsh, fish)
  - Git and go-git dependencies
  - Platform detection utilities
- **Task Runner** - Just (Justfile) for common commands
- **Documentation Tools** - markdownlint-cli

#### Available Commands (via Justfile)

```bash
just test         # Run all tests
just lint         # Run linters
just build        # Build binary
just coverage     # Generate coverage report
just fmt          # Format code
just vet          # Run Go vet
just check        # Run all checks (lint + vet + test)
```

#### Directory Integration (Optional)

Enable automatic environment activation with direnv:

```bash
# Allow direnv for this directory
direnv allow
```

Now the development environment loads automatically when you `cd` into the project!

#### Editor Configuration

ZERB uses EditorConfig to maintain consistent coding styles:

- **Go files**: Use tabs (community standard). Set your preferred tab display width in your editor.
- **Other files** (YAML, JSON, Markdown, Nix): Use 2 spaces.

**For Neovim users:**
- If you use `vim-sleuth`, it will automatically detect the project's indentation style
- Set your personal tab display width preference in your config:
  ```lua
  vim.opt.tabstop = 2       -- Display tabs as 2 spaces wide (adjust to your preference)
  vim.opt.shiftwidth = 2    -- Indent by 2 spaces when using >> or <<
  ```
- Ensure EditorConfig support is enabled (built-in for Neovim 0.9+, or use `editorconfig/editorconfig-vim` plugin)

### AI-Assisted Development

ZERB is being built with the assistance of **[OpenCode](https://opencode.ai/)**, an AI-powered coding assistant that helps accelerate development while maintaining code quality and consistency.

#### How OpenCode Assists ZERB Development

OpenCode is used throughout the development process.

#### Agent Guidelines

ZERB uses an `AGENTS.md` file to provide OpenCode with project-specific context and guidelines:

- **Build/Test Commands** - Standard commands for testing, building, and linting
- **Code Style** - Go coding standards, naming conventions, and best practices
- **Architecture Constraints** - Isolation requirements, security guidelines, and design principles
- **Testing Requirements** - TDD approach, coverage goals, and testing strategies

This approach ensures that AI assistance is aligned with ZERB's specific architectural decisions, coding standards, and quality requirements. The `AGENTS.md` file serves as a contract between the project and AI tools, maintaining consistency across all AI-assisted development.

For more details on how to use AI tools effectively with ZERB, see the [`AGENTS.md`](AGENTS.md) file in the repository root.

### Testing Strategy

Key test areas:
- Binary management (download, GPG verification, SHA256 fallback)
- Config versioning (timestamped files, rollback)
- mise wrapper (tool installation, version resolution, isolation)
- chezmoi wrapper (isolation verification, flag passing)
- Drift detection (detection accuracy, user prompts)
- Git operations (commit generation, sync, pre-commit hook, conflicts)
- Shell integration (activation script generation)
- Platform detection (distro detection, family booleans)

---

## License

MIT-0 (MIT No Attribution) License - See [LICENSE](LICENSE) for details

ZERB is released under the MIT-0 license, which provides maximum freedom to use, modify, and distribute the software without requiring attribution. You can use ZERB in any project (personal, commercial, or otherwise) without needing to include copyright notices.

---

## Acknowledgments

ZERB stands on the shoulders of giants:

- **[mise](https://github.com/jdx/mise)** by @jdx - Universal tool version manager
- **[chezmoi](https://github.com/twpayne/chezmoi)** by @twpayne - Dotfile manager
- **[gopher-lua](https://github.com/yuin/gopher-lua)** by @yuin - Lua VM in Go
- **[cobra](https://github.com/spf13/cobra)** by @spf13 - CLI framework
- **[go-git](https://github.com/go-git/go-git)** - Pure Go git implementation

---

## Contact

- **Issues**: [GitHub Issues](https://github.com/ZebulonRouseFrantzich/zerb/issues)

---

<p align="center">
  <sub>Built with ❤️ for developers who value reproducibility and simplicity</sub>
</p>
