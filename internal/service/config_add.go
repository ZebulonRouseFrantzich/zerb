// Package service provides high-level business logic for ZERB operations.
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/chezmoi"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/git"
	"github.com/ZebulonRouseFrantzich/zerb/internal/transaction"
)

const (
	// ConfigDirPermissions sets the permission mode for config directories.
	ConfigDirPermissions = 0755
	// ConfigFilePermissions sets the permission mode for config files.
	ConfigFilePermissions = 0644
	// TmpDirPermissions sets the permission mode for temporary directories.
	TmpDirPermissions = 0700
)

// ConfigParser provides config parsing functionality.
type ConfigParser interface {
	ParseString(ctx context.Context, lua string) (*config.Config, error)
}

// ConfigGenerator provides config generation functionality.
type ConfigGenerator interface {
	GenerateTimestamped(ctx context.Context, cfg *config.Config, gitCommit string) (filename, content string, err error)
}

// ConfigAddService orchestrates the config add operation.
type ConfigAddService struct {
	chezmoi   chezmoi.Chezmoi
	git       git.Git
	parser    ConfigParser
	generator ConfigGenerator
	clock     Clock
	zerbDir   string
}

// NewConfigAddService creates a new config add service with dependency injection.
func NewConfigAddService(
	chezmoiClient chezmoi.Chezmoi,
	gitClient git.Git,
	parser ConfigParser,
	generator ConfigGenerator,
	clock Clock,
	zerbDir string,
) *ConfigAddService {
	return &ConfigAddService{
		chezmoi:   chezmoiClient,
		git:       gitClient,
		parser:    parser,
		generator: generator,
		clock:     clock,
		zerbDir:   zerbDir,
	}
}

// AddRequest contains the parameters for adding config files.
type AddRequest struct {
	Paths     []string
	Options   map[string]ConfigOptions
	DryRun    bool
	SkipCheck bool // Skip file existence check (for testing)
}

// ConfigOptions contains options for a single config file.
type ConfigOptions struct {
	Recursive bool
	Template  bool
	Secrets   bool
	Private   bool
}

// AddResult contains the results of the add operation.
type AddResult struct {
	AddedPaths    []string
	SkippedPaths  []string // Already tracked
	CommitHash    string
	ConfigVersion string
}

// Execute performs the config add operation.
func (s *ConfigAddService) Execute(ctx context.Context, req AddRequest) (*AddResult, error) {
	result := &AddResult{
		AddedPaths:   make([]string, 0, len(req.Paths)),
		SkippedPaths: make([]string, 0, len(req.Paths)),
	}

	// 1. Acquire transaction lock
	txnDir := filepath.Join(s.zerbDir, ".txn")
	lock, err := transaction.AcquireLock(txnDir)
	if err != nil {
		return nil, fmt.Errorf("acquire transaction lock: %w", err)
	}
	defer func() { _ = lock.Release() }()

	// 2. Validate and normalize all paths
	normalizedPaths := make(map[string]string) // original -> normalized
	for _, path := range req.Paths {
		// Validate and normalize path
		normalized, err := config.NormalizeConfigPath(path)
		if err != nil {
			return nil, fmt.Errorf("invalid path %q: %w", path, err)
		}

		// Check if path exists (unless skipped for testing)
		if !req.SkipCheck {
			// Stat the normalized path
			info, err := os.Stat(normalized)
			if err != nil {
				return nil, fmt.Errorf("stat %q: %w", path, err)
			}

			// Verify file is readable
			file, err := os.Open(normalized)
			if err != nil {
				return nil, fmt.Errorf("cannot read %q: %w", path, err)
			}
			file.Close()

			// Check if directory and require --recursive
			if info.IsDir() {
				opts := req.Options[path]
				if !opts.Recursive {
					return nil, fmt.Errorf(`%s is a directory.
Use --recursive to track it and its contents.

Example:
  zerb config add %s --recursive`, path, path)
				}
			}
		}

		normalizedPaths[path] = normalized
	}

	// 3. Read current config
	// Check context before blocking I/O
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("operation cancelled: %w", err)
	}

	activeConfigPath := filepath.Join(s.zerbDir, "zerb.active.lua")
	cfgData, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read active config: %w", err)
	}

	currentConfig, err := s.parser.ParseString(ctx, string(cfgData))
	if err != nil {
		return nil, fmt.Errorf("parse current config: %w", err)
	}

	// 4. Check for duplicates
	for origPath, normalized := range normalizedPaths {
		isDuplicate := false
		for _, existing := range currentConfig.Configs {
			existingNorm, err := config.NormalizeConfigPath(existing.Path)
			if err != nil {
				// Malformed existing entry - log warning and skip comparison
				fmt.Fprintf(os.Stderr, "Warning: cannot normalize existing config path %q: %v\n", existing.Path, err)
				continue
			}
			if existingNorm == normalized {
				isDuplicate = true
				result.SkippedPaths = append(result.SkippedPaths, origPath)
				break
			}
		}
		if !isDuplicate {
			result.AddedPaths = append(result.AddedPaths, origPath)
		}
	}

	// If all paths are duplicates, return early
	if len(result.AddedPaths) == 0 {
		return result, nil
	}

	// 5. If dry run, stop here
	if req.DryRun {
		return result, nil
	}

	// 6. Create transaction
	txnOpts := make(map[string]transaction.AddOptions)
	for _, path := range result.AddedPaths {
		opts := req.Options[path]
		txnOpts[path] = transaction.AddOptions{
			Recursive: opts.Recursive,
			Template:  opts.Template,
			Secrets:   opts.Secrets,
			Private:   opts.Private,
		}
	}
	txn := transaction.New(result.AddedPaths, txnOpts)

	// Save initial transaction state
	if err := txn.Save(txnDir); err != nil {
		return nil, fmt.Errorf("save transaction: %w", err)
	}

	// 7. Add files to chezmoi (track state per path)
	for _, path := range result.AddedPaths {
		opts := req.Options[path]
		chezmoiOpts := chezmoi.AddOptions{
			Recursive: opts.Recursive,
			Template:  opts.Template,
			Secrets:   opts.Secrets,
			Private:   opts.Private,
		}

		// Update transaction state to in_progress
		txn.UpdatePathState(path, transaction.StateInProgress, nil, nil)
		if err := txn.Save(txnDir); err != nil {
			return nil, fmt.Errorf("save transaction: %w", err)
		}

		// Perform chezmoi add
		if err := s.chezmoi.Add(ctx, path, chezmoiOpts); err != nil {
			// Mark as failed and save transaction
			txn.UpdatePathState(path, transaction.StateFailed, nil, err)
			if saveErr := txn.Save(txnDir); saveErr != nil {
				// Log warning but continue with the primary error
				fmt.Fprintf(os.Stderr, "Warning: failed to save transaction state: %v\n", saveErr)
			}

			// Provide recovery instructions
			txnFile := filepath.Join(txnDir, fmt.Sprintf("txn-config-add-%s.json", txn.ID))
			return nil, fmt.Errorf("failed to add %q to config manager: %w (transaction state saved to %s)", path, err, txnFile)
		}

		// Mark as completed
		txn.UpdatePathState(path, transaction.StateCompleted, nil, nil)
		if err := txn.Save(txnDir); err != nil {
			return nil, fmt.Errorf("save transaction: %w", err)
		}
	}

	// 8. Update config file
	// Check if we would exceed the maximum config file count
	if len(currentConfig.Configs)+len(result.AddedPaths) > config.MaxConfigFileCount {
		return nil, fmt.Errorf("would exceed maximum config file count (%d)", config.MaxConfigFileCount)
	}

	for _, path := range result.AddedPaths {
		opts := req.Options[path]
		currentConfig.Configs = append(currentConfig.Configs, config.ConfigFile{
			Path:      path,
			Recursive: opts.Recursive,
			Template:  opts.Template,
			Secrets:   opts.Secrets,
			Private:   opts.Private,
		})
	}

	// 9. Generate new timestamped config
	// Note: we don't have the git commit yet, so pass empty string
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

	// 10. Update .zerb-active marker
	activeMarkerPath := filepath.Join(s.zerbDir, ".zerb-active")
	if err := os.WriteFile(activeMarkerPath, []byte(newConfigFilename+"\n"), ConfigFilePermissions); err != nil {
		return nil, fmt.Errorf("update active marker: %w", err)
	}

	// 11. Update zerb.lua.active symlink atomically (or copy on Windows)
	tmpLink := activeConfigPath + ".tmp"
	target := filepath.Join("configs", newConfigFilename)

	// Try to create symlink to temp location first
	err = os.Symlink(target, tmpLink)
	if err != nil {
		// Check if symlinks are unsupported (Windows without dev mode)
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
		// Atomic rename (overwrites existing)
		if err := os.Rename(tmpLink, activeConfigPath); err != nil {
			os.Remove(tmpLink) // Clean up temp
			return nil, fmt.Errorf("update active config link: %w", err)
		}
	}

	// 12. Stage files in git
	filesToStage := []string{
		filepath.Join("configs", newConfigFilename),
		".zerb-active",
		"zerb.active.lua",
	}

	// Also stage chezmoi source files
	chezmoiSourceDir := filepath.Join("chezmoi", "source")
	filesToStage = append(filesToStage, chezmoiSourceDir)

	if err := s.git.Stage(ctx, filesToStage...); err != nil {
		return nil, fmt.Errorf("stage files: %w", err)
	}

	// 13. Create git commit
	commitMsg := s.generateCommitMessage(result.AddedPaths)
	commitBody := s.generateCommitBody(result.AddedPaths)

	if err := s.git.Commit(ctx, commitMsg, commitBody); err != nil {
		return nil, fmt.Errorf("create commit: %w", err)
	}

	// 14. Mark git as committed and get commit hash
	txn.GitCommitted = true

	// Get the commit hash
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

// generateCommitMessage creates the commit subject line.
func (s *ConfigAddService) generateCommitMessage(paths []string) string {
	if len(paths) == 1 {
		return fmt.Sprintf("Add %s to tracked configs", paths[0])
	}
	return fmt.Sprintf("Add %d configs to tracked configs", len(paths))
}

// generateCommitBody creates the commit body with details.
func (s *ConfigAddService) generateCommitBody(paths []string) string {
	if len(paths) == 1 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Added configurations:\n")
	for _, path := range paths {
		sb.WriteString("- ")
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	return sb.String()
}
