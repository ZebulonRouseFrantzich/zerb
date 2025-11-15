package drift

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

var (
	// validToolNameRegex matches valid tool names (alphanumeric, underscore, hyphen, slash for repos)
	validToolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_/-]+$`)
	// validVersionRegex matches valid version strings (alphanumeric, dot, hyphen, plus)
	validVersionRegex = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)
)

// validateToolName checks if a tool name is safe to use in commands
func validateToolName(name string) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if !validToolNameRegex.MatchString(name) {
		return fmt.Errorf("invalid tool name %q: must contain only alphanumeric characters, underscores, hyphens, and slashes", name)
	}
	return nil
}

// validateVersion checks if a version string is safe to use in commands
func validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	if !validVersionRegex.MatchString(version) {
		return fmt.Errorf("invalid version %q: must contain only alphanumeric characters, dots, hyphens, and plus signs", version)
	}
	return nil
}

// ApplyDriftAction applies a drift resolution action
func ApplyDriftAction(ctx context.Context, result DriftResult, action DriftAction, configPath, zerbDir, miseBinary string) error {
	switch action {
	case ActionAdopt:
		return applyAdopt(result, configPath, zerbDir)
	case ActionRevert:
		return applyRevert(ctx, result, miseBinary, zerbDir)
	case ActionSkip:
		return nil // No action
	default:
		return fmt.Errorf("unknown action: %v", action)
	}
}

// applyAdopt updates baseline to match environment
func applyAdopt(result DriftResult, configPath string, zerbDir string) error {
	// Read current config
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Parse config
	parser := config.NewParser(nil)
	cfg, err := parser.ParseString(context.Background(), string(content))
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Update tools array based on drift type
	cfg.Tools = updateToolsArray(cfg.Tools, result, ActionAdopt)

	// Generate new config
	generator := config.NewGenerator()
	luaCode, err := generator.Generate(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	// Create timestamped config
	timestamp := time.Now().UTC().Format("20060102T150405.000Z")
	configsDir := filepath.Join(zerbDir, "configs")
	newConfigFilename := fmt.Sprintf("zerb.lua.%s", timestamp)
	newConfigPath := filepath.Join(configsDir, newConfigFilename)

	// Write new config (0600 for security - may contain sensitive data)
	if err := os.WriteFile(newConfigPath, []byte(luaCode), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Update .zerb-active marker (0600 for consistency)
	markerPath := filepath.Join(zerbDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte(timestamp), 0600); err != nil {
		return fmt.Errorf("update marker: %w", err)
	}

	// Update symlink
	symlinkPath := filepath.Join(zerbDir, "zerb.lua.active")
	os.Remove(symlinkPath) // Remove old symlink (ignore error)
	symlinkTarget := filepath.Join("configs", newConfigFilename)
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		return fmt.Errorf("update symlink: %w", err)
	}

	return nil
}

// applyRevert restores environment to match baseline
func applyRevert(ctx context.Context, result DriftResult, miseBinary string, zerbDir string) error {
	// Validate tool name before any operations
	if err := validateToolName(result.Tool); err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}

	switch result.DriftType {
	case DriftExternalOverride, DriftVersionMismatch:
		// Validate version
		if err := validateVersion(result.BaselineVersion); err != nil {
			return fmt.Errorf("invalid baseline version: %w", err)
		}
		// Reinstall correct version via mise
		toolSpec := fmt.Sprintf("%s@%s", result.Tool, result.BaselineVersion)
		if err := executeMiseInstallOrUninstall(ctx, miseBinary, zerbDir, "install", toolSpec); err != nil {
			return fmt.Errorf("install %s: %w", toolSpec, err)
		}

	case DriftMissing:
		// Validate version
		if err := validateVersion(result.BaselineVersion); err != nil {
			return fmt.Errorf("invalid baseline version: %w", err)
		}
		// Install missing tool
		toolSpec := fmt.Sprintf("%s@%s", result.Tool, result.BaselineVersion)
		if err := executeMiseInstallOrUninstall(ctx, miseBinary, zerbDir, "install", toolSpec); err != nil {
			return fmt.Errorf("install %s: %w", toolSpec, err)
		}

	case DriftExtra:
		// Uninstall extra tool (no version needed for uninstall)
		if err := executeMiseInstallOrUninstall(ctx, miseBinary, zerbDir, "uninstall", result.Tool); err != nil {
			return fmt.Errorf("uninstall %s: %w", result.Tool, err)
		}

	case DriftManagedButNotActive:
		// This is typically a PATH issue, not something we can fix with mise
		// But we can try re-activating the shell or do nothing
		// For now, do nothing (this should be handled by the user)
		return fmt.Errorf("drift type %s requires manual PATH investigation", result.DriftType)

	case DriftVersionUnknown:
		// Reinstall to hopefully fix version detection
		toolSpec := fmt.Sprintf("%s@%s", result.Tool, result.BaselineVersion)
		if err := executeMiseInstallOrUninstall(ctx, miseBinary, zerbDir, "install", toolSpec); err != nil {
			return fmt.Errorf("install %s: %w", toolSpec, err)
		}
	}

	return nil
}

// executeMiseInstallOrUninstall is a wrapper around executeMiseCommand that discards output
func executeMiseInstallOrUninstall(ctx context.Context, miseBinary string, zerbDir string, args ...string) error {
	_, err := executeMiseCommand(ctx, miseBinary, zerbDir, args...)
	if err != nil {
		return fmt.Errorf("mise command failed: %w", err)
	}
	return nil
}

// updateToolsArray updates the tools array based on drift type and action
func updateToolsArray(tools []string, result DriftResult, action DriftAction) []string {
	if action != ActionAdopt {
		// Revert and Skip don't modify the tools array
		return tools
	}

	switch result.DriftType {
	case DriftExternalOverride:
		// Remove tool from baseline (acknowledge external management)
		return removeToolFromList(tools, result.Tool)

	case DriftVersionMismatch:
		// Update version in baseline
		return updateToolVersion(tools, result.Tool, result.ActiveVersion)

	case DriftExtra:
		// Add tool to baseline
		toolSpec := fmt.Sprintf("%s@%s", result.Tool, result.ManagedVersion)
		return append(tools, toolSpec)

	case DriftMissing:
		// Remove from baseline (user decided not to install)
		return removeToolFromList(tools, result.Tool)

	case DriftManagedButNotActive:
		// PATH issue - optionally remove from baseline
		return removeToolFromList(tools, result.Tool)

	case DriftVersionUnknown:
		// Version detection failed - optionally remove from baseline
		return removeToolFromList(tools, result.Tool)
	}

	return tools
}

// removeToolFromList removes a tool from the tools list
func removeToolFromList(tools []string, toolName string) []string {
	var result []string
	for _, t := range tools {
		spec, err := ParseToolSpec(t)
		if err != nil {
			// Keep tools that can't be parsed
			result = append(result, t)
			continue
		}
		if spec.Name != toolName {
			result = append(result, t)
		}
	}
	return result
}

// updateToolVersion updates the version of a tool in the tools list
// Preserves backend prefixes (e.g., cargo:ripgrep@13.0.0 -> cargo:ripgrep@14.0.0)
func updateToolVersion(tools []string, toolName string, newVersion string) []string {
	var result []string
	for _, t := range tools {
		spec, err := ParseToolSpec(t)
		if err != nil {
			// Keep tools that can't be parsed
			result = append(result, t)
			continue
		}

		if spec.Name == toolName {
			// Reconstruct tool spec with new version
			var newSpec string
			if spec.Backend != "" {
				// Preserve backend prefix
				// Need to get the original name part (before @) from the original string
				parts := strings.SplitN(t, "@", 2)
				nameWithBackend := parts[0]
				newSpec = fmt.Sprintf("%s@%s", nameWithBackend, newVersion)
			} else {
				newSpec = fmt.Sprintf("%s@%s", toolName, newVersion)
			}
			result = append(result, newSpec)
		} else {
			result = append(result, t)
		}
	}
	return result
}
