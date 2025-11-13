package shell

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectShell detects the user's shell using multiple methods
func DetectShell() (*DetectionResult, error) {
	// Method 1: Try $SHELL environment variable (most reliable)
	if shell := os.Getenv("SHELL"); shell != "" {
		shellType := parseShellFromPath(shell)
		if shellType.IsValid() {
			return &DetectionResult{
				Shell:      shellType,
				Method:     "$SHELL environment variable",
				ShellPath:  shell,
				Confidence: "high",
			}, nil
		}
	}

	// Method 2: Try parent process (fallback)
	if shellType, shellPath := detectFromParentProcess(); shellType.IsValid() {
		return &DetectionResult{
			Shell:      shellType,
			Method:     "parent process",
			ShellPath:  shellPath,
			Confidence: "medium",
		}, nil
	}

	// Method 3: Could not detect shell
	return &DetectionResult{
		Shell:      ShellUnknown,
		Method:     "detection failed",
		ShellPath:  "",
		Confidence: "none",
	}, nil
}

// parseShellFromPath extracts the shell type from a shell binary path
// Examples:
//   - /bin/bash -> bash
//   - /usr/bin/zsh -> zsh
//   - /usr/local/bin/fish -> fish
func parseShellFromPath(shellPath string) ShellType {
	// Get the base name (e.g., "/bin/bash" -> "bash")
	baseName := filepath.Base(shellPath)

	// Normalize to lowercase
	baseName = strings.ToLower(baseName)

	// Map to known shell types
	switch baseName {
	case "bash":
		return ShellBash
	case "zsh":
		return ShellZsh
	case "fish":
		return ShellFish
	default:
		return ShellUnknown
	}
}

// detectFromParentProcess attempts to detect the shell from the parent process
// This is a fallback when $SHELL is not set
func detectFromParentProcess() (ShellType, string) {
	// Read /proc/self/stat to get parent process ID (Linux only)
	// For MVP, we'll skip this complex implementation and return unknown
	// This can be enhanced post-MVP using:
	// - /proc filesystem on Linux
	// - ps command on macOS
	// - gopsutil library for cross-platform support

	return ShellUnknown, ""
}

// ValidateShell validates that a shell type is supported
func ValidateShell(shell ShellType) error {
	if !shell.IsValid() {
		return &UnsupportedShellError{Shell: shell.String()}
	}
	return nil
}

// GetSupportedShells returns a list of supported shells
func GetSupportedShells() []ShellType {
	return []ShellType{ShellBash, ShellZsh, ShellFish}
}
