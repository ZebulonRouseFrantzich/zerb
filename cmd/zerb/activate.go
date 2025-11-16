package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/shell"
)

// runActivate handles the `zerb activate <shell>` subcommand
// This is the key abstraction layer that hides mise from users
func runActivate(args []string) error {
	// Create context with timeout (30 seconds should be plenty for activation)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup logger (only logs in debug mode via ZERB_DEBUG env var)
	logger := slog.Default()
	if os.Getenv(shell.EnvZerbDebug) != "" {
		logger.Debug("activating shell", "args", args)
	}

	// Validate arguments
	if len(args) < 1 {
		return fmt.Errorf("usage: zerb activate <shell>\nSupported shells: bash, zsh, fish")
	}

	// Parse shell type
	shellName := args[0]
	logger.Debug("parsed shell type", "shell", shellName)
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
	logger.Debug("using ZERB directory", "dir", zerbDir)

	// Validate ZERB directory path (security: prevent command injection)
	if !filepath.IsAbs(zerbDir) {
		logger.Error("invalid ZERB directory", "dir", zerbDir, "error", "not absolute")
		return fmt.Errorf("invalid ZERB directory: path must be absolute")
	}

	// Construct internal binary path
	miseBinaryPath := filepath.Join(zerbDir, "bin", "mise")

	// Check if ZERB environment is initialized
	if _, err := os.Stat(miseBinaryPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("ZERB environment not initialized\nRun 'zerb init' to set up ZERB first")
		}
		return fmt.Errorf("failed to access ZERB environment: %w", err)
	}

	// Check for .zerb-no-git marker and warn if git is not initialized
	noGitMarkerPath := filepath.Join(zerbDir, ".zerb-no-git")
	if _, err := os.Stat(noGitMarkerPath); err == nil {
		fmt.Fprintf(os.Stderr, "\n⚠ Note: Git versioning not initialized\n")
		fmt.Fprintf(os.Stderr, "  \n")
		fmt.Fprintf(os.Stderr, "  Your ZERB environment is working, but configuration changes\n")
		fmt.Fprintf(os.Stderr, "  are not being tracked in version history.\n")
		fmt.Fprintf(os.Stderr, "  \n")
		fmt.Fprintf(os.Stderr, "  To enable versioning and sync:\n")
		fmt.Fprintf(os.Stderr, "    zerb git init\n")
		fmt.Fprintf(os.Stderr, "  \n")
		fmt.Fprintf(os.Stderr, "  (This message appears once per activate until git is set up)\n\n")
	}

	// Additional security: verify path is within expected ZERB directory
	cleanPath := filepath.Clean(miseBinaryPath)
	expectedPrefix := filepath.Clean(filepath.Join(zerbDir, "bin")) + string(filepath.Separator)
	if !strings.HasPrefix(cleanPath+string(filepath.Separator), expectedPrefix) {
		return fmt.Errorf("security error: invalid binary path")
	}

	// Get internal activation command
	miseArgs, err := shell.GetMiseActivationCommand(shellType, miseBinaryPath)
	if err != nil {
		return fmt.Errorf("prepare shell activation: %w", err)
	}

	// Execute internal activation and pass through output
	// This is the key step: we call mise internally but users never see it
	//nolint:gosec // G204: Command args are generated internally by GetMiseActivationCommand with validated zerbDir path
	cmd := exec.CommandContext(ctx, miseArgs[0], miseArgs[1:]...)

	// Set up environment variables
	cmd.Env = append(os.Environ(),
		shell.EnvZerbActive+"=1",
		fmt.Sprintf("%s=%s", shell.EnvZerbDir, zerbDir),
	)

	// Capture stdout and stderr
	output, err := cmd.Output()
	if err != nil {
		logger.Error("activation command failed", "error", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Debug("activation stderr", "stderr", string(exitErr.Stderr))
			return fmt.Errorf("shell activation failed: %s\nDetails: %s\n\nRun 'zerb init' to repair your installation", err, string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to activate shell environment: %w\n\nRun 'zerb init' to repair your installation", err)
	}

	logger.Debug("activation successful", "output_length", len(output))

	// Write activation output to stdout (this is what the shell will eval)
	fmt.Print(string(output))

	return nil
}

// getZerbDir returns the ZERB directory path
// First checks ZERB_DIR environment variable, then falls back to ~/.config/zerb
func getZerbDir() (string, error) {
	// Check environment variable
	if zerbDir := os.Getenv(shell.EnvZerbDir); zerbDir != "" {
		return zerbDir, nil
	}

	// Default to ~/.config/zerb
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "zerb"), nil
}
