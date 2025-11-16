// Package chezmoi provides an interface-based wrapper for chezmoi operations
// with complete isolation and error abstraction.
//
// It ensures that ZERB's integrated chezmoi never touches the user's existing
// chezmoi installation and provides user-friendly error messages.
package chezmoi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Error types for user-facing errors (never mention "chezmoi")
var (
	ErrInvalidPath                = errors.New("invalid path")
	ErrDirectoryRequiresRecursive = errors.New("directory requires --recursive flag")
	ErrChezmoiInvocation          = errors.New("failed to add configuration file")
	ErrTransactionExists          = errors.New("another configuration operation is in progress")
)

// AddOptions configures the behavior of adding a config file.
type AddOptions struct {
	Recursive bool // Add directory recursively
	Template  bool // Enable template processing
	Secrets   bool // Encrypt with GPG
	Private   bool // Set file permissions to 600
}

// Chezmoi is the interface for chezmoi operations.
// Following Go best practices: accept interfaces, return structs.
type Chezmoi interface {
	Add(ctx context.Context, path string, opts AddOptions) error
}

// Client implements the Chezmoi interface.
type Client struct {
	bin  string // Path to chezmoi binary (e.g., ~/.config/zerb/bin/chezmoi)
	src  string // Path to chezmoi source directory (e.g., ~/.config/zerb/chezmoi/source)
	conf string // Path to chezmoi config file (e.g., ~/.config/zerb/chezmoi/config.toml)
}

// NewClient creates a new chezmoi client for the given ZERB directory.
func NewClient(zerbDir string) *Client {
	return &Client{
		bin:  filepath.Join(zerbDir, "bin", "chezmoi"),
		src:  filepath.Join(zerbDir, "chezmoi", "source"),
		conf: filepath.Join(zerbDir, "chezmoi", "config.toml"),
	}
}

// Add adds a config file to chezmoi's source directory.
// It uses complete isolation flags to prevent touching the user's chezmoi installation.
func (c *Client) Add(ctx context.Context, path string, opts AddOptions) error {
	args := []string{
		"--source", c.src,
		"--config", c.conf,
		"add",
	}

	// Add optional flags
	if opts.Template {
		args = append(args, "--template")
	}
	if opts.Recursive {
		args = append(args, "--recursive")
	}
	if opts.Secrets {
		args = append(args, "--encrypt") // Map to chezmoi's encrypt flag
	}
	if opts.Private {
		args = append(args, "--private") // chezmoi sets permissions to 600
	}

	// Add the path as the last argument
	args = append(args, path)

	// Create command with context for cancellation/timeout support
	cmd := exec.CommandContext(ctx, c.bin, args...)

	// Scrub environment for complete isolation
	// Only pass through essential variables
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"USER=" + os.Getenv("USER"),
		"LANG=" + os.Getenv("LANG"),
	}
	// Explicitly do NOT pass CHEZMOI_* environment variables

	// Capture combined output for error reporting
	out, err := cmd.CombinedOutput()
	if err != nil {
		return translateChezmoiError(err, string(out))
	}

	return nil
}

// translateChezmoiError maps chezmoi errors to user-friendly ZERB errors.
// This ensures we never expose "chezmoi" in user-facing messages.
func translateChezmoiError(err error, stderr string) error {
	// Check for context cancellation/timeout first
	// Use errors.Is for wrapped errors and string check as fallback
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("operation cancelled: %w", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
		return fmt.Errorf("operation timed out: %w", context.DeadlineExceeded)
	}

	// Map common chezmoi errors to user-friendly messages
	stderrLower := strings.ToLower(stderr)

	if strings.Contains(stderrLower, "no such file") || strings.Contains(stderrLower, "does not exist") {
		return fmt.Errorf("%w: file not found", ErrChezmoiInvocation)
	}

	if strings.Contains(stderrLower, "permission denied") {
		return fmt.Errorf("%w: permission denied", ErrChezmoiInvocation)
	}

	if strings.Contains(stderrLower, "is a directory") {
		return fmt.Errorf("%w: path is a directory (use --recursive)", ErrChezmoiInvocation)
	}

	// Generic fallback - redact sensitive info but preserve useful context
	sanitized := redactSensitiveInfo(stderr)
	return fmt.Errorf("%w: %s", ErrChezmoiInvocation, sanitized)
}

// redactSensitiveInfo removes potentially sensitive information from error messages.
// Redacts paths that might contain usernames and limits message length.
func redactSensitiveInfo(msg string) string {
	// Limit message length
	const maxLen = 200
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}

	// Redact the word "chezmoi"
	msg = strings.ReplaceAll(msg, "chezmoi", "config manager")
	msg = strings.ReplaceAll(msg, "Chezmoi", "Config Manager")
	msg = strings.ReplaceAll(msg, "CHEZMOI", "CONFIG MANAGER")

	// Redact absolute paths that might contain usernames
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		msg = strings.ReplaceAll(msg, home, "$HOME")
	}

	// Redact /home/username patterns
	re := regexp.MustCompile(`/home/[^/\s]+`)
	msg = re.ReplaceAllString(msg, "/home/<user>")

	// Redact /Users/username patterns (macOS)
	re = regexp.MustCompile(`/Users/[^/\s]+`)
	msg = re.ReplaceAllString(msg, "/Users/<user>")

	return msg
}
