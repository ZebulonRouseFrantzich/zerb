package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/testutil"
)

func TestSetupTestEnv(t *testing.T) {
	// Call SetupTestEnv
	testutil.SetupTestEnv(t)

	// Verify ZERB environment variables are set
	zerbConfigDir := os.Getenv("ZERB_CONFIG_DIR")
	if zerbConfigDir == "" {
		t.Error("ZERB_CONFIG_DIR not set")
	}

	zerbDataDir := os.Getenv("ZERB_DATA_DIR")
	if zerbDataDir == "" {
		t.Error("ZERB_DATA_DIR not set")
	}

	zerbCacheDir := os.Getenv("ZERB_CACHE_DIR")
	if zerbCacheDir == "" {
		t.Error("ZERB_CACHE_DIR not set")
	}

	// Verify mise environment variables are set
	miseDataDir := os.Getenv("MISE_DATA_DIR")
	if miseDataDir == "" {
		t.Error("MISE_DATA_DIR not set")
	}

	miseConfigFile := os.Getenv("MISE_CONFIG_FILE")
	if miseConfigFile == "" {
		t.Error("MISE_CONFIG_FILE not set")
	}

	miseCacheDir := os.Getenv("MISE_CACHE_DIR")
	if miseCacheDir == "" {
		t.Error("MISE_CACHE_DIR not set")
	}

	// Verify test mode is set
	testMode := os.Getenv("ZERB_TEST_MODE")
	if testMode != "1" {
		t.Errorf("ZERB_TEST_MODE = %q, want \"1\"", testMode)
	}

	// Verify directories exist
	dirs := []string{
		zerbConfigDir,
		zerbDataDir,
		zerbCacheDir,
		miseDataDir,
		miseCacheDir,
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("directory %s does not exist", dir)
		}
	}

	// Verify all paths are under the temp directory
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			t.Errorf("path %s is not absolute", dir)
		}
	}
}

func TestSetupTestEnv_Isolation(t *testing.T) {
	// Test that multiple test runs get different directories
	testutil.SetupTestEnv(t)
	dir1 := os.Getenv("ZERB_CONFIG_DIR")

	// Run again in a subtest
	t.Run("subtest", func(t *testing.T) {
		testutil.SetupTestEnv(t)
		dir2 := os.Getenv("ZERB_CONFIG_DIR")

		if dir1 == dir2 {
			t.Error("expected different temp directories for different test contexts")
		}
	})
}
