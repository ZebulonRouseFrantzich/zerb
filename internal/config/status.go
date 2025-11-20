package config

import (
	"context"
	"fmt"
	"os"
)

// ConfigStatus represents the synchronization status of a configuration file.
type ConfigStatus int

const (
	// StatusSynced indicates the config is declared, exists, and managed by ZERB.
	StatusSynced ConfigStatus = iota

	// StatusMissing indicates the config is declared but the file doesn't exist.
	StatusMissing

	// StatusPartial indicates the config is declared, exists, but not managed by ZERB.
	// This typically happens when a file is added to zerb.lua but the add operation
	// was incomplete or failed.
	StatusPartial

	// TODO: Future enhancement - StatusDrift for when file exists and managed but content differs
	// This will require file hash comparison and integration with drift detection component.
)

// String returns the string representation of a ConfigStatus.
func (s ConfigStatus) String() string {
	switch s {
	case StatusSynced:
		return "synced"
	case StatusMissing:
		return "missing"
	case StatusPartial:
		return "partial"
	default:
		return "unknown"
	}
}

// Symbol returns the visual symbol for a ConfigStatus.
func (s ConfigStatus) Symbol() string {
	switch s {
	case StatusSynced:
		return "✓"
	case StatusMissing:
		return "✗"
	case StatusPartial:
		return "?"
	default:
		return "?"
	}
}

// ConfigWithStatus represents a configuration file with its detected status.
type ConfigWithStatus struct {
	ConfigFile ConfigFile
	Status     ConfigStatus
}

// StatusDetector detects the synchronization status of configuration files.
type StatusDetector interface {
	DetectStatus(ctx context.Context, configs []ConfigFile) ([]ConfigWithStatus, error)
}

// Chezmoi defines the interface for querying chezmoi state.
// This is a subset of the full chezmoi.Chezmoi interface needed for status detection.
type Chezmoi interface {
	HasFile(ctx context.Context, path string) (bool, error)
}

// DefaultStatusDetector implements StatusDetector using filesystem checks and chezmoi queries.
type DefaultStatusDetector struct {
	chezmoi Chezmoi
}

// NewDefaultStatusDetector creates a new DefaultStatusDetector.
func NewDefaultStatusDetector(cm Chezmoi) *DefaultStatusDetector {
	return &DefaultStatusDetector{
		chezmoi: cm,
	}
}

// DetectStatus determines the status of each configuration file.
//
// IMPORTANT: This method expects paths to be pre-normalized by the caller
// (e.g., tilde expansion completed). The service layer should normalize paths
// using NormalizeConfigPath before calling this method.
//
// Status detection logic:
// - StatusSynced: File exists on disk AND managed by ZERB
// - StatusMissing: File does NOT exist on disk
// - StatusPartial: File exists on disk but NOT managed by ZERB
//
// The method respects context cancellation and will stop processing if context is cancelled.
func (d *DefaultStatusDetector) DetectStatus(ctx context.Context, configs []ConfigFile) ([]ConfigWithStatus, error) {
	results := make([]ConfigWithStatus, 0, len(configs))

	for _, cfg := range configs {
		// Check for context cancellation
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result := ConfigWithStatus{
			ConfigFile: cfg,
		}

		// Check if file exists on disk
		_, err := os.Stat(cfg.Path)
		fileExists := err == nil

		if !fileExists {
			// File doesn't exist -> Missing
			result.Status = StatusMissing
		} else {
			// File exists -> check if managed by ZERB
			managed, err := d.chezmoi.HasFile(ctx, cfg.Path)
			if err != nil {
				return nil, fmt.Errorf("check if file %q is managed: %w", cfg.Path, err)
			}

			if managed {
				result.Status = StatusSynced
			} else {
				result.Status = StatusPartial
			}
		}

		results = append(results, result)
	}

	return results, nil
}
