// Package git provides an interface-based wrapper for Git operations
// with context support and proper error handling.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Common Git errors
var (
	ErrNotAGitRepo     = errors.New("not a git repository")
	ErrNothingToCommit = errors.New("nothing to commit")
	ErrEmptyMessage    = errors.New("commit message cannot be empty")
	ErrNoFiles         = errors.New("no files specified to stage")
)

// Git is the interface for Git operations.
// Following Go best practices: accept interfaces, return structs.
type Git interface {
	Stage(ctx context.Context, files ...string) error
	Commit(ctx context.Context, msg, body string) error
	GetHeadCommit(ctx context.Context) (string, error)
}

// Client implements the Git interface.
type Client struct {
	repoPath string // Path to the git repository
}

// NewClient creates a new Git client for the given repository path.
func NewClient(repoPath string) *Client {
	return &Client{
		repoPath: repoPath,
	}
}

// Stage adds files to the git staging area.
// Files are relative to the repository root.
func (c *Client) Stage(ctx context.Context, files ...string) error {
	if len(files) == 0 {
		return ErrNoFiles
	}

	args := append([]string{"add"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.repoPath

	out, err := cmd.CombinedOutput()
	if err != nil {
		return translateGitError(err, string(out))
	}

	return nil
}

// Commit creates a git commit with the given message and optional body.
// The message is required and becomes the commit subject line.
// The body is optional and provides additional commit details.
func (c *Client) Commit(ctx context.Context, msg, body string) error {
	if msg == "" {
		return ErrEmptyMessage
	}

	// Build commit message
	fullMsg := msg
	if body != "" {
		fullMsg = msg + "\n\n" + body
	}

	args := []string{"commit", "-m", fullMsg}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.repoPath

	out, err := cmd.CombinedOutput()
	if err != nil {
		return translateGitError(err, string(out))
	}

	return nil
}

// GetHeadCommit returns the commit hash of HEAD.
func (c *Client) GetHeadCommit(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = c.repoPath

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", translateGitError(err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

// translateGitError maps git errors to user-friendly errors.
func translateGitError(err error, stderr string) error {
	// Check for context errors first
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("operation cancelled: %w", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
		return fmt.Errorf("operation timed out: %w", context.DeadlineExceeded)
	}

	stderrLower := strings.ToLower(stderr)

	// Check for "not a git repository"
	if strings.Contains(stderrLower, "not a git repository") {
		return fmt.Errorf("%w: %s", ErrNotAGitRepo, extractGitError(stderr))
	}

	// Check for "nothing to commit"
	if strings.Contains(stderrLower, "nothing to commit") || strings.Contains(stderrLower, "no changes added") {
		return ErrNothingToCommit
	}

	// Generic git error with sanitized message
	sanitized := extractGitError(stderr)
	return fmt.Errorf("git operation failed: %s", sanitized)
}

// extractGitError extracts the useful error message from git output.
func extractGitError(output string) string {
	// Git error messages often start with "error:" or "fatal:"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "error:") ||
			strings.HasPrefix(strings.ToLower(line), "fatal:") {
			// Remove the "error:" or "fatal:" prefix
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
			return line
		}
	}

	// If no specific error line found, return first non-empty line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	return "unknown error"
}
