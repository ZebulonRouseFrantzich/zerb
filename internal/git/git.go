// Package git provides an interface-based wrapper for Git operations
// with context support and proper error handling.
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Common Git errors
var (
	ErrNotAGitRepo     = errors.New("not a git repository")
	ErrNothingToCommit = errors.New("nothing to commit")
	ErrEmptyMessage    = errors.New("commit message cannot be empty")
	ErrNoFiles         = errors.New("no files specified to stage")
	ErrGitInitFailed   = errors.New("git initialization failed")
	ErrInvalidRepo     = errors.New("invalid git repository")
)

// GitUserInfo contains git user configuration information
type GitUserInfo struct {
	Name       string
	Email      string
	FromEnv    bool
	FromConfig bool
	IsDefault  bool
}

// Git is the interface for Git operations.
// Following Go best practices: accept interfaces, return structs.
type Git interface {
	// Existing methods
	Stage(ctx context.Context, files ...string) error
	Commit(ctx context.Context, msg, body string) error
	GetHeadCommit(ctx context.Context) (string, error)

	// New initialization methods
	InitRepo(ctx context.Context) error
	ConfigureUser(ctx context.Context, userInfo GitUserInfo) error
	CreateInitialCommit(ctx context.Context, message string, files []string) error
	IsGitRepo(ctx context.Context) (bool, error)
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

	// Use multiple -m flags for better formatting
	args := []string{"commit", "-m", msg}
	if body != "" {
		args = append(args, "-m", body)
	}

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

// InitRepo initializes a new git repository using go-git.
// Returns ErrGitInitFailed if initialization fails.
func (c *Client) InitRepo(ctx context.Context) error {
	_, err := gogit.PlainInit(c.repoPath, false)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrGitInitFailed, err.Error())
	}
	return nil
}

// IsGitRepo checks if the path is a valid git repository.
// Returns (true, nil) if valid, (false, nil) if not exists, (false, err) if corrupted.
func (c *Client) IsGitRepo(ctx context.Context) (bool, error) {
	_, err := gogit.PlainOpen(c.repoPath)
	if err == gogit.ErrRepositoryNotExists {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %s", ErrInvalidRepo, err.Error())
	}
	return true, nil
}

// ConfigureUser sets the git user name and email in repository-local config.
// This never touches global git config to maintain ZERB isolation.
func (c *Client) ConfigureUser(ctx context.Context, userInfo GitUserInfo) error {
	repo, err := gogit.PlainOpen(c.repoPath)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}

	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("read repo config: %w", err)
	}

	cfg.User.Name = userInfo.Name
	cfg.User.Email = userInfo.Email

	if err := repo.Storer.SetConfig(cfg); err != nil {
		return fmt.Errorf("write repo config: %w", err)
	}

	return nil
}

// CreateInitialCommit creates a commit with the specified files.
// Files should be relative paths from the repository root.
func (c *Client) CreateInitialCommit(ctx context.Context, message string, files []string) error {
	if message == "" {
		return ErrEmptyMessage
	}
	if len(files) == 0 {
		return ErrNoFiles
	}

	repo, err := gogit.PlainOpen(c.repoPath)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	// Stage all specified files
	for _, file := range files {
		if _, err := worktree.Add(file); err != nil {
			return fmt.Errorf("stage file %s: %w", file, err)
		}
	}

	// Get user config for commit author
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("read repo config: %w", err)
	}

	// Create commit
	_, err = worktree.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  cfg.User.Name,
			Email: cfg.User.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	return nil
}
