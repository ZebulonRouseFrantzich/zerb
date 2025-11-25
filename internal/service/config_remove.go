// Package service provides high-level business logic for ZERB operations.
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/transaction"
)

// ChezmoiRemover provides chezmoi remove functionality.
type ChezmoiRemover interface {
	Remove(ctx context.Context, path string) error
}

// ConfigRemoveService orchestrates the config remove operation.
type ConfigRemoveService struct {
	chezmoi   ChezmoiRemover
	git       GitClient
	parser    ConfigParser
	generator ConfigGenerator
	clock     Clock
	zerbDir   string
}

// GitClient provides git operations for config remove.
type GitClient interface {
	Stage(ctx context.Context, files ...string) error
	Commit(ctx context.Context, msg, body string) error
	GetHeadCommit(ctx context.Context) (string, error)
}

// NewConfigRemoveService creates a new config remove service with dependency injection.
func NewConfigRemoveService(
	chezmoiClient ChezmoiRemover,
	gitClient GitClient,
	parser ConfigParser,
	generator ConfigGenerator,
	clock Clock,
	zerbDir string,
) *ConfigRemoveService {
	return &ConfigRemoveService{
		chezmoi:   chezmoiClient,
		git:       gitClient,
		parser:    parser,
		generator: generator,
		clock:     clock,
		zerbDir:   zerbDir,
	}
}

// RemoveRequest contains the parameters for removing config files.
type RemoveRequest struct {
	Paths  []string
	DryRun bool
	Purge  bool // Also delete source file from disk (per CR-2 order)
}

// RemoveResult contains the results of the remove operation.
type RemoveResult struct {
	RemovedPaths  []string
	SkippedPaths  []string // Not tracked
	CommitHash    string
	ConfigVersion string
}

// Execute performs the config remove operation.
func (s *ConfigRemoveService) Execute(ctx context.Context, req RemoveRequest) (*RemoveResult, error) {
	result := &RemoveResult{
		RemovedPaths: make([]string, 0, len(req.Paths)),
		SkippedPaths: make([]string, 0),
	}

	// Validate paths are provided
	if len(req.Paths) == 0 {
		return nil, fmt.Errorf("no paths specified")
	}

	// Check context before any operations
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("operation cancelled: %w", err)
	}

	// Deduplicate paths (HR-4)
	deduplicatedPaths := config.DeduplicatePaths(req.Paths)

	// 1. Acquire transaction lock (unless dry run)
	var lock *transaction.Lock
	if !req.DryRun {
		txnDir := filepath.Join(s.zerbDir, ".txn")
		var err error
		lock, err = transaction.AcquireLock(ctx, txnDir)
		if err != nil {
			return nil, fmt.Errorf("acquire transaction lock: %w", err)
		}
		defer func() { _ = lock.Release() }()
	}

	// 2. Read current config
	activeConfigPath := filepath.Join(s.zerbDir, "zerb.active.lua")
	cfgData, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read active config: %w", err)
	}

	currentConfig, err := s.parser.ParseString(ctx, string(cfgData))
	if err != nil {
		return nil, fmt.Errorf("parse current config: %w", err)
	}

	// 3. Validate all paths are tracked before any removal
	for _, path := range deduplicatedPaths {
		found := currentConfig.FindConfig(path)
		if found == nil {
			return nil, fmt.Errorf("config not tracked: %s", path)
		}
		result.RemovedPaths = append(result.RemovedPaths, path)
	}

	// 4. If dry run, stop here
	if req.DryRun {
		return result, nil
	}

	// 5. Create transaction
	txnDir := filepath.Join(s.zerbDir, ".txn")
	txnOpts := make(map[string]transaction.RemoveOptions)
	for _, path := range result.RemovedPaths {
		txnOpts[path] = transaction.RemoveOptions{
			Purge: req.Purge,
		}
	}
	txn := transaction.NewRemove(result.RemovedPaths, txnOpts)

	// Save initial transaction state
	if err := txn.Save(txnDir); err != nil {
		return nil, fmt.Errorf("save transaction: %w", err)
	}

	// 6. For each path: optionally delete file, then remove from chezmoi
	for _, path := range result.RemovedPaths {
		// Update transaction state to in_progress
		txn.UpdatePathState(path, transaction.StateInProgress, nil, nil)
		if err := txn.Save(txnDir); err != nil {
			return nil, fmt.Errorf("save transaction: %w", err)
		}

		// Per CR-2: If --purge, delete source file FIRST (before chezmoi.Remove)
		if req.Purge {
			normalizedPath, err := config.NormalizeConfigPath(path)
			if err != nil {
				txn.UpdatePathState(path, transaction.StateFailed, nil, err)
				if saveErr := txn.Save(txnDir); saveErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save transaction state: %v\n", saveErr)
				}
				return nil, fmt.Errorf("normalize path %q: %w", path, err)
			}

			// Per HR-5: Safety check - verify path is within $HOME
			if !config.IsWithinHome(normalizedPath) {
				err := fmt.Errorf("cannot delete file outside home directory: %s", path)
				txn.UpdatePathState(path, transaction.StateFailed, nil, err)
				if saveErr := txn.Save(txnDir); saveErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save transaction state: %v\n", saveErr)
				}
				return nil, err
			}

			// Delete the source file
			if err := os.Remove(normalizedPath); err != nil && !os.IsNotExist(err) {
				txn.UpdatePathState(path, transaction.StateFailed, nil, err)
				if saveErr := txn.Save(txnDir); saveErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save transaction state: %v\n", saveErr)
				}
				return nil, fmt.Errorf("delete source file %q: %w", path, err)
			}
		}

		// Remove from chezmoi (per HR-3: returns nil if not found)
		if err := s.chezmoi.Remove(ctx, path); err != nil {
			txn.UpdatePathState(path, transaction.StateFailed, nil, err)
			if saveErr := txn.Save(txnDir); saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save transaction state: %v\n", saveErr)
			}
			return nil, fmt.Errorf("remove %q from config manager: %w", path, err)
		}

		// Mark as completed
		txn.UpdatePathState(path, transaction.StateCompleted, nil, nil)
		if err := txn.Save(txnDir); err != nil {
			return nil, fmt.Errorf("save transaction: %w", err)
		}
	}

	// 7. Update config file (remove entries)
	for _, path := range result.RemovedPaths {
		newConfigs, _ := currentConfig.RemoveConfig(path)
		currentConfig.Configs = newConfigs
	}

	// 8. Generate new timestamped config
	newConfigFilename, newConfigContent, err := s.generator.GenerateTimestamped(ctx, currentConfig, "")
	if err != nil {
		return nil, fmt.Errorf("generate config: %w", err)
	}

	configsDir := filepath.Join(s.zerbDir, "configs")
	if err := os.MkdirAll(configsDir, ConfigDirPermissions); err != nil {
		return nil, fmt.Errorf("create configs directory: %w", err)
	}

	newConfigPath := filepath.Join(configsDir, newConfigFilename)
	if err := os.WriteFile(newConfigPath, []byte(newConfigContent), ConfigFilePermissions); err != nil {
		return nil, fmt.Errorf("write new config: %w", err)
	}

	result.ConfigVersion = newConfigFilename

	// Mark config as updated in transaction
	txn.ConfigUpdated = true
	if err := txn.Save(txnDir); err != nil {
		return nil, fmt.Errorf("save transaction: %w", err)
	}

	// 9. Update .zerb-active marker
	activeMarkerPath := filepath.Join(s.zerbDir, ".zerb-active")
	if err := os.WriteFile(activeMarkerPath, []byte(newConfigFilename+"\n"), ConfigFilePermissions); err != nil {
		return nil, fmt.Errorf("update active marker: %w", err)
	}

	// 10. Update zerb.active.lua symlink atomically
	tmpLink := activeConfigPath + ".tmp"
	target := filepath.Join("configs", newConfigFilename)

	err = os.Symlink(target, tmpLink)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not supported") || strings.Contains(errStr, "not implemented") {
			// Fallback to copy on systems without symlink support
			if err := os.WriteFile(activeConfigPath, []byte(newConfigContent), ConfigFilePermissions); err != nil {
				return nil, fmt.Errorf("update active config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("create symlink: %w", err)
		}
	} else {
		// Atomic rename
		if err := os.Rename(tmpLink, activeConfigPath); err != nil {
			os.Remove(tmpLink)
			return nil, fmt.Errorf("update active config link: %w", err)
		}
	}

	// 11. Stage files in git
	filesToStage := []string{
		filepath.Join("configs", newConfigFilename),
		".zerb-active",
		"zerb.active.lua",
	}

	// Also stage chezmoi source directory changes
	chezmoiSourceDir := filepath.Join("chezmoi", "source")
	filesToStage = append(filesToStage, chezmoiSourceDir)

	if err := s.git.Stage(ctx, filesToStage...); err != nil {
		return nil, fmt.Errorf("stage files: %w", err)
	}

	// 12. Create git commit
	commitMsg := s.generateCommitMessage(result.RemovedPaths)
	commitBody := s.generateCommitBody(result.RemovedPaths)

	if err := s.git.Commit(ctx, commitMsg, commitBody); err != nil {
		return nil, fmt.Errorf("create commit: %w", err)
	}

	// 13. Mark git as committed and get commit hash
	txn.GitCommitted = true

	commitHash, err := s.git.GetHeadCommit(ctx)
	if err == nil {
		result.CommitHash = commitHash
	}

	// Save final transaction state
	if err := txn.Save(txnDir); err != nil {
		return nil, fmt.Errorf("save transaction: %w", err)
	}

	return result, nil
}

// generateCommitMessage creates the commit subject line for remove operations.
func (s *ConfigRemoveService) generateCommitMessage(paths []string) string {
	if len(paths) == 1 {
		return fmt.Sprintf("Remove %s from tracked configs", paths[0])
	}
	return fmt.Sprintf("Remove %d configs from tracked configs", len(paths))
}

// generateCommitBody creates the commit body with details for remove operations.
func (s *ConfigRemoveService) generateCommitBody(paths []string) string {
	if len(paths) == 1 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Removed configurations:\n")
	for _, path := range paths {
		sb.WriteString("- ")
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	return sb.String()
}
