// Package shell provides shell integration functionality for ZERB.
//
// This package handles:
//   - Detecting the user's shell (bash, zsh, fish)
//   - Locating shell configuration files (rc files)
//   - Generating activation commands for ZERB
//   - Safely modifying shell configuration files
//   - Managing ZERB shell activation
//
// # User-Facing Abstraction
//
// This package maintains ZERB's core design principle: users should never
// know that mise or chezmoi exist. All shell integration uses ZERB-branded
// commands:
//
//	# Users add to their shell config:
//	eval "$(zerb activate bash)"
//
//	# NOT (this exposes mise):
//	eval "$(~/.config/zerb/bin/mise activate bash)"
//
// The `zerb activate` command is implemented in cmd/zerb/activate.go and
// internally calls mise, but this is completely abstracted from users.
//
// # Shell Detection
//
// Shell detection tries multiple methods:
//  1. $SHELL environment variable (most reliable)
//  2. Parent process name detection (fallback)
//  3. Interactive prompt (last resort)
//
// # RC File Management
//
// The package knows how to locate and safely modify shell configuration files:
//   - bash: ~/.bashrc
//   - zsh: ~/.zshrc
//   - fish: ~/.config/fish/config.fish
//
// All modifications are:
//   - Idempotent (safe to run multiple times)
//   - Backed up before changes
//   - Atomic (using temp file + rename)
//   - Validated before writing
//
// # Activation Flow
//
// 1. Detect user's shell
// 2. Generate appropriate activation command
// 3. Check if activation already exists in rc file
// 4. If not present, prompt user to add it
// 5. Safely add activation line to rc file
// 6. Verify activation works
//
// # Example Usage
//
//	// Create shell manager
//	manager := shell.NewManager(shell.Config{
//	    ZerbDir: "~/.config/zerb",
//	})
//
//	// Detect shell
//	shellType, err := manager.DetectShell()
//
//	// Setup shell integration
//	err = manager.SetupIntegration(shellType, shell.SetupOptions{
//	    Interactive: true, // Prompt user before changes
//	})
package shell
