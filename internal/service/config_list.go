package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// ErrNotInitialized is returned when ZERB is not initialized.
var ErrNotInitialized = errors.New("ZERB not initialized")

// ConfigListService orchestrates the config list operation.
type ConfigListService struct {
	parser   ConfigParser
	detector config.StatusDetector
	zerbDir  string
}

// NewConfigListService creates a new config list service.
func NewConfigListService(parser ConfigParser, detector config.StatusDetector, zerbDir string) *ConfigListService {
	return &ConfigListService{
		parser:   parser,
		detector: detector,
		zerbDir:  zerbDir,
	}
}

// ListRequest contains parameters for listing configs.
type ListRequest struct {
	// Future: add flags like --all, --format, etc.
}

// ListResult contains the results of the list operation.
type ListResult struct {
	Configs       []config.ConfigWithStatus
	ActiveVersion string
}

// List retrieves and displays configuration files with their status.
func (s *ConfigListService) List(ctx context.Context, req ListRequest) (*ListResult, error) {
	// Check context first
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Read active marker to get active config version
	activeMarker := filepath.Join(s.zerbDir, ".zerb-active")
	markerData, err := os.ReadFile(activeMarker)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotInitialized
		}
		return nil, fmt.Errorf("read active marker: %w", err)
	}

	activeFilename := strings.TrimSpace(string(markerData))
	if activeFilename == "" {
		return nil, fmt.Errorf("active marker is empty - corrupted state")
	}

	// Check context before reading config
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Read active config file
	activeConfigPath := filepath.Join(s.zerbDir, "configs", activeFilename)
	configContent, err := os.ReadFile(activeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("active config not found: %s", activeFilename)
		}
		return nil, fmt.Errorf("read active config: %w", err)
	}

	// Parse config
	cfg, err := s.parser.ParseString(ctx, string(configContent))
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// If no configs, return empty result
	if len(cfg.Configs) == 0 {
		return &ListResult{
			Configs:       []config.ConfigWithStatus{},
			ActiveVersion: activeFilename,
		}, nil
	}

	// Normalize paths before status detection
	// This ensures tilde paths like "~/.zshrc" are expanded correctly
	for i := range cfg.Configs {
		normalizedPath, err := config.NormalizeConfigPath(cfg.Configs[i].Path)
		if err != nil {
			return nil, fmt.Errorf("normalize path %q: %w", cfg.Configs[i].Path, err)
		}
		cfg.Configs[i].Path = normalizedPath
	}

	// Check context before status detection
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Detect status of each config
	configsWithStatus, err := s.detector.DetectStatus(ctx, cfg.Configs)
	if err != nil {
		return nil, fmt.Errorf("detect status: %w", err)
	}

	// Sort alphabetically by path
	sort.Slice(configsWithStatus, func(i, j int) bool {
		return configsWithStatus[i].ConfigFile.Path < configsWithStatus[j].ConfigFile.Path
	})

	return &ListResult{
		Configs:       configsWithStatus,
		ActiveVersion: activeFilename,
	}, nil
}
