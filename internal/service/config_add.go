// Package service provides high-level business logic for ZERB operations.
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZebulonRouseFrantzich/zerb/internal/chezmoi"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/git"
)

// ConfigAddService orchestrates the config add operation.
type ConfigAddService struct {
	chezmoi   chezmoi.Chezmoi
	git       git.Git
	parser    *config.Parser
	generator *config.Generator
	clock     Clock
	zerbDir   string
}

// NewConfigAddService creates a new config add service with dependency injection.
func NewConfigAddService(
	chezmoiClient chezmoi.Chezmoi,
	gitClient git.Git,
	parser *config.Parser,
	generator *config.Generator,
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
		AddedPaths:   []string{},
		SkippedPaths: []string{},
	}

	// 1. Validate and normalize all paths
	normalizedPaths := make(map[string]string) // original -> normalized
	for _, path := range req.Paths {
		// Validate and normalize path
		normalized, err := config.NormalizeConfigPath(path)
		if err != nil {
			return nil, fmt.Errorf("invalid path %q: %w", path, err)
		}

		// Check if path exists (unless skipped for testing)
		if !req.SkipCheck {
			if _, err := os.Stat(normalized); err != nil {
				return nil, fmt.Errorf("path does not exist: %s", path)
			}

			// Check if directory and require --recursive
			if info, _ := os.Stat(normalized); info != nil && info.IsDir() {
				opts := req.Options[path]
				if !opts.Recursive {
					return nil, fmt.Errorf("path is a directory, use --recursive flag: %s", path)
				}
			}
		}

		normalizedPaths[path] = normalized
	}

	// 2. Read current config
	activeConfigPath := filepath.Join(s.zerbDir, "zerb.lua.active")
	cfgData, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("read active config: %w", err)
	}

	currentConfig, err := s.parser.ParseString(ctx, string(cfgData))
	if err != nil {
		return nil, fmt.Errorf("parse current config: %w", err)
	}

	// 3. Check for duplicates
	for origPath, normalized := range normalizedPaths {
		isDuplicate := false
		for _, existing := range currentConfig.Configs {
			existingNorm, _ := config.NormalizeConfigPath(existing.Path)
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

	// 4. If dry run, stop here
	if req.DryRun {
		return result, nil
	}

	// 5. Add files to chezmoi
	for _, path := range result.AddedPaths {
		opts := req.Options[path]
		chezmoiOpts := chezmoi.AddOptions{
			Recursive: opts.Recursive,
			Template:  opts.Template,
			Secrets:   opts.Secrets,
			Private:   opts.Private,
		}

		if err := s.chezmoi.Add(ctx, path, chezmoiOpts); err != nil {
			return nil, fmt.Errorf("add %q to config manager: %w", path, err)
		}
	}

	// 6. Update config file
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

	// 7. Generate new timestamped config
	// Note: we don't have the git commit yet, so pass empty string
	newConfigFilename, newConfigContent, err := s.generator.GenerateTimestamped(ctx, currentConfig, "")
	if err != nil {
		return nil, fmt.Errorf("generate config: %w", err)
	}

	configsDir := filepath.Join(s.zerbDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return nil, fmt.Errorf("create configs directory: %w", err)
	}

	newConfigPath := filepath.Join(configsDir, newConfigFilename)

	if err := os.WriteFile(newConfigPath, []byte(newConfigContent), 0644); err != nil {
		return nil, fmt.Errorf("write new config: %w", err)
	}

	result.ConfigVersion = newConfigFilename

	// 8. Update .zerb-active marker
	activeMarkerPath := filepath.Join(s.zerbDir, ".zerb-active")
	if err := os.WriteFile(activeMarkerPath, []byte(newConfigFilename+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("update active marker: %w", err)
	}

	// 9. Update zerb.lua.active symlink (or copy on Windows)
	os.Remove(activeConfigPath) // Remove old symlink/file
	if err := os.Symlink(filepath.Join("configs", newConfigFilename), activeConfigPath); err != nil {
		// Fallback to copy on systems without symlink support
		if err := os.WriteFile(activeConfigPath, []byte(newConfigContent), 0644); err != nil {
			return nil, fmt.Errorf("update active config: %w", err)
		}
	}

	// 10. Stage files in git
	filesToStage := []string{
		filepath.Join("configs", newConfigFilename),
		".zerb-active",
		"zerb.lua.active",
	}

	// Also stage chezmoi source files
	chezmoiSourceDir := filepath.Join("chezmoi", "source")
	filesToStage = append(filesToStage, chezmoiSourceDir)

	if err := s.git.Stage(ctx, filesToStage...); err != nil {
		return nil, fmt.Errorf("stage files: %w", err)
	}

	// 11. Create git commit
	commitMsg := s.generateCommitMessage(result.AddedPaths)
	commitBody := s.generateCommitBody(result.AddedPaths)

	if err := s.git.Commit(ctx, commitMsg, commitBody); err != nil {
		return nil, fmt.Errorf("create commit: %w", err)
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

	body := "Added configurations:\n"
	for _, path := range paths {
		body += fmt.Sprintf("- %s\n", path)
	}
	return body
}
