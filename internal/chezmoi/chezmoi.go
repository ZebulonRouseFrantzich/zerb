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

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// Error types for user-facing errors (never mention "chezmoi")
var (
	ErrInvalidPath                = errors.New("invalid path")
	ErrDirectoryRequiresRecursive = errors.New("directory requires --recursive flag")
	ErrChezmoiInvocation          = errors.New("failed to add configuration file")
	ErrTransactionExists          = errors.New("another configuration operation is in progress")
)

// RedactedError wraps an error with a user-friendly message while preserving
// the error chain for errors.Is/errors.As checks.
type RedactedError struct {
	message string
	wrapped error
}

// Error returns the redacted error message.
func (e *RedactedError) Error() string {
	return e.message
}

// Unwrap returns the wrapped error, preserving the error chain.
func (e *RedactedError) Unwrap() error {
	return e.wrapped
}

// newRedactedError creates a RedactedError with sensitive information removed.
func newRedactedError(err error, context string) error {
	if err == nil {
		return nil
	}

	// Redact the error message
	redactedMsg := redactSensitiveInfo(err.Error())

	// Combine context with redacted message
	message := fmt.Sprintf("%s: %s", context, redactedMsg)

	return &RedactedError{
		message: message,
		wrapped: err,
	}
}

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
	HasFile(ctx context.Context, path string) (bool, error)
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

// HasFile checks if a path is managed by ZERB.
// Returns true if the file exists in the chezmoi source directory.
//
// This method performs direct filesystem checks rather than invoking the chezmoi binary
// for performance. It uses NormalizeConfigPath for security and error redaction.
func (c *Client) HasFile(ctx context.Context, path string) (bool, error) {
	// Check context first
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Use NormalizeConfigPath for canonical path and security
	normalizedPath, err := config.NormalizeConfigPath(path)
	if err != nil {
		return false, newRedactedError(err, "normalize path")
	}

	// Get home directory for mapping to chezmoi source path
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("get home directory: %w", err)
	}

	// Convert user path to chezmoi source path
	// For example: ~/.zshrc -> dot_zshrc, ~/.config/nvim/init.lua -> dot_config/nvim/init.lua
	relPath, err := filepath.Rel(home, normalizedPath)
	if err != nil {
		return false, newRedactedError(err, "compute relative path")
	}

	// Map path to chezmoi naming convention
	sourcePath := pathToChezmoiSource(relPath)
	fullSourcePath := filepath.Join(c.src, sourcePath)

	// Check if file or directory exists in source
	_, err = os.Stat(fullSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Other errors (permission denied, etc.)
		return false, newRedactedError(err, "check source path")
	}

	return true, nil
}

// pathToChezmoiSource converts a relative home path to chezmoi source naming.
// Examples:
//
//	.zshrc -> dot_zshrc
//	.config/nvim/init.lua -> dot_config/nvim/init.lua
//	.ssh/config -> dot_ssh/config
func pathToChezmoiSource(relPath string) string {
	parts := strings.Split(relPath, string(filepath.Separator))

	// Convert first component if it starts with dot
	if len(parts) > 0 && strings.HasPrefix(parts[0], ".") {
		parts[0] = "dot_" + parts[0][1:]
	}

	return filepath.Join(parts...)
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
