// Package git provides an interface-based wrapper for Git operations
// with context support and proper error handling.
package git

import (
	"context"
	"errors"
	"fmt"
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

// Stage adds files to the git staging area using go-git.
// Files are relative to the repository root.
func (c *Client) Stage(ctx context.Context, files ...string) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
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

	// Stage each file
	for _, file := range files {
		if _, err := worktree.Add(file); err != nil {
			return fmt.Errorf("stage file %s: %w", file, err)
		}
	}

	return nil
}

// Commit creates a git commit with the given message and optional body using go-git.
// The message is required and becomes the commit subject line.
// The body is optional and provides additional commit details.
func (c *Client) Commit(ctx context.Context, msg, body string) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if msg == "" {
		return ErrEmptyMessage
	}

	repo, err := gogit.PlainOpen(c.repoPath)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	// Get user config for commit author
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("read repo config: %w", err)
	}

	// Combine message and body
	commitMsg := msg
	if body != "" {
		commitMsg = msg + "\n\n" + body
	}

	// Create commit
	_, err = worktree.Commit(commitMsg, &gogit.CommitOptions{
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

// GetHeadCommit returns the commit hash of HEAD using go-git.
func (c *Client) GetHeadCommit(ctx context.Context) (string, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled: %w", err)
	}

	repo, err := gogit.PlainOpen(c.repoPath)
	if err != nil {
		return "", fmt.Errorf("open repository: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD: %w", err)
	}

	return ref.Hash().String(), nil
}

// InitRepo initializes a new git repository using go-git.
// Returns ErrGitInitFailed if initialization fails.
func (c *Client) InitRepo(ctx context.Context) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	_, err := gogit.PlainInit(c.repoPath, false)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrGitInitFailed, err.Error())
	}
	return nil
}

// IsGitRepo checks if the path is a valid git repository.
// Returns (true, nil) if valid, (false, nil) if not exists, (false, err) if corrupted.
func (c *Client) IsGitRepo(ctx context.Context) (bool, error) {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context cancelled: %w", err)
	}

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
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

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
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

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
