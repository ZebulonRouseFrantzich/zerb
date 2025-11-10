// Package testutil provides utilities for testing ZERB in isolation.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// SetupTestEnv creates isolated test directories for each test.
// This ensures ZERB tests never interfere with:
// - System installations
// - User's actual ZERB configuration
// - Other development tools (mise, chezmoi, etc.)
//
// The cleanup function is automatically handled by t.TempDir(),
// so callers don't need to manually clean up.
func SetupTestEnv(t *testing.T) {
	t.Helper()

	// Create temp directory (auto-cleaned by testing framework)
	tmpDir := t.TempDir()

	// Set ZERB paths to temp location
	t.Setenv("ZERB_CONFIG_DIR", filepath.Join(tmpDir, "config"))
	t.Setenv("ZERB_DATA_DIR", filepath.Join(tmpDir, "data"))
	t.Setenv("ZERB_CACHE_DIR", filepath.Join(tmpDir, "cache"))

	// Ensure mise/chezmoi in tests use isolated paths
	t.Setenv("MISE_DATA_DIR", filepath.Join(tmpDir, "mise"))
	t.Setenv("MISE_CONFIG_FILE", filepath.Join(tmpDir, "mise/config.toml"))
	t.Setenv("MISE_CACHE_DIR", filepath.Join(tmpDir, "mise-cache"))

	// Mark as test mode
	t.Setenv("ZERB_TEST_MODE", "1")

	// Create the directories
	dirs := []string{
		filepath.Join(tmpDir, "config"),
		filepath.Join(tmpDir, "data"),
		filepath.Join(tmpDir, "cache"),
		filepath.Join(tmpDir, "mise"),
		filepath.Join(tmpDir, "mise-cache"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("failed to create test directory %s: %v", dir, err)
		}
	}
}
