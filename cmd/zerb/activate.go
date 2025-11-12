package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ZebulonRouseFrantzich/zerb/internal/shell"
)

// runActivate handles the `zerb activate <shell>` subcommand
// This is the key abstraction layer that hides mise from users
func runActivate(args []string) error {
	// Validate arguments
	if len(args) < 1 {
		return fmt.Errorf("usage: zerb activate <shell>\nSupported shells: bash, zsh, fish")
	}

	// Parse shell type
	shellName := args[0]
	var shellType shell.ShellType
	switch shellName {
	case "bash":
		shellType = shell.ShellBash
	case "zsh":
		shellType = shell.ShellZsh
	case "fish":
		shellType = shell.ShellFish
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported shells: bash, zsh, fish", shellName)
	}

	// Validate shell type
	if err := shell.ValidateShell(shellType); err != nil {
		return fmt.Errorf("invalid shell: %w", err)
	}

	// Get ZERB directory (default: ~/.config/zerb)
	zerbDir, err := getZerbDir()
	if err != nil {
		return fmt.Errorf("get ZERB directory: %w", err)
	}

	// Construct mise binary path
	miseBinaryPath := filepath.Join(zerbDir, "bin", "mise")

	// Check if mise binary exists
	if _, err := os.Stat(miseBinaryPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("mise binary not found at %s\nRun 'zerb init' to install ZERB first", miseBinaryPath)
		}
		return fmt.Errorf("check mise binary: %w", err)
	}

	// Get mise activation command
	miseArgs, err := shell.GetMiseActivationCommand(shellType, miseBinaryPath)
	if err != nil {
		return fmt.Errorf("get mise activation command: %w", err)
	}

	// Execute mise activate and pass through output
	// This is the key step: we call mise internally but users never see it
	cmd := exec.Command(miseArgs[0], miseArgs[1:]...)

	// Set up environment variables for mise
	cmd.Env = append(os.Environ(),
		"ZERB_ACTIVE=1",
		fmt.Sprintf("ZERB_DIR=%s", zerbDir),
	)

	// Capture stdout and stderr
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("mise activate failed: %s\nStderr: %s", err, string(exitErr.Stderr))
		}
		return fmt.Errorf("execute mise activate: %w", err)
	}

	// Write mise's output to stdout (this is what the shell will eval)
	fmt.Print(string(output))

	return nil
}

// getZerbDir returns the ZERB directory path
// First checks ZERB_DIR environment variable, then falls back to ~/.config/zerb
func getZerbDir() (string, error) {
	// Check environment variable
	if zerbDir := os.Getenv("ZERB_DIR"); zerbDir != "" {
		return zerbDir, nil
	}

	// Default to ~/.config/zerb
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "zerb"), nil
}
