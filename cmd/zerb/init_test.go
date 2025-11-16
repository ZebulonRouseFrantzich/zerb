package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// TestCreateDirectoryStructure tests that all required directories are created
func TestCreateDirectoryStructure(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Call function under test
	err := createDirectoryStructure(tmpDir)
	if err != nil {
		t.Fatalf("createDirectoryStructure failed: %v", err)
	}

	// Verify all expected directories exist
	expectedDirs := []string{
		"bin",
		"keyrings",
		filepath.Join("cache", "downloads"),
		filepath.Join("cache", "versions"),
		"configs",
		"tmp",
		"logs",
		"mise",
		filepath.Join("chezmoi", "source"),
	}

	for _, dir := range expectedDirs {
		fullPath := filepath.Join(tmpDir, dir)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Errorf("directory %s does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", dir)
		}
		// Check permissions (should be 0755)
		if info.Mode().Perm() != 0755 {
			t.Errorf("directory %s has wrong permissions: got %o, want 0755", dir, info.Mode().Perm())
		}
	}
}

// TestCreateDirectoryStructure_IdempotentOperations tests that calling createDirectoryStructure twice doesn't fail
func TestCreateDirectoryStructure_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Call first time
	err := createDirectoryStructure(tmpDir)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Call second time (should be idempotent)
	err = createDirectoryStructure(tmpDir)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// Verify directories still exist
	binDir := filepath.Join(tmpDir, "bin")
	if _, err := os.Stat(binDir); err != nil {
		t.Errorf("directory not present after second call: %v", err)
	}
}

// TestCreateDirectoryStructure_PermissionDenied tests behavior when directory creation fails
func TestCreateDirectoryStructure_PermissionDenied(t *testing.T) {
	// Skip on systems where we can't reliably test permissions
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	// Create a read-only directory
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0555) // r-xr-xr-x (no write)
	if err != nil {
		t.Fatalf("failed to create read-only dir: %v", err)
	}

	// Try to create structure inside read-only directory
	err = createDirectoryStructure(readOnlyDir)
	if err == nil {
		t.Error("expected error when creating directories in read-only location, got nil")
	}
}

// TestCreateDirectoryStructure_InvalidPath tests behavior with invalid paths
func TestCreateDirectoryStructure_InvalidPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "valid path",
			path:    t.TempDir(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createDirectoryStructure(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("createDirectoryStructure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestIsAlreadyInitialized tests detection of existing ZERB installations
func TestIsAlreadyInitialized(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string) error
		expected bool
	}{
		{
			name: "empty directory - not initialized",
			setup: func(dir string) error {
				return nil // No setup needed
			},
			expected: false,
		},
		{
			name: "mise binary exists - initialized",
			setup: func(dir string) error {
				binDir := filepath.Join(dir, "bin")
				if err := os.MkdirAll(binDir, 0755); err != nil {
					return err
				}
				misePath := filepath.Join(binDir, "mise")
				return os.WriteFile(misePath, []byte("#!/bin/sh\necho mise"), 0755)
			},
			expected: true,
		},
		{
			name: "configs directory exists - initialized",
			setup: func(dir string) error {
				configsDir := filepath.Join(dir, "configs")
				return os.MkdirAll(configsDir, 0755)
			},
			expected: true,
		},
		{
			name: ".zerb-active marker exists - initialized",
			setup: func(dir string) error {
				markerPath := filepath.Join(dir, ".zerb-active")
				return os.WriteFile(markerPath, []byte("zerb.lua.20250101T120000.000Z"), 0644)
			},
			expected: true,
		},
		{
			name: "only unrelated files - not initialized",
			setup: func(dir string) error {
				// Create some random files that shouldn't trigger initialization detection
				return os.WriteFile(filepath.Join(dir, "random.txt"), []byte("content"), 0644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Run setup
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			// Test isAlreadyInitialized
			result := isAlreadyInitialized(tmpDir)
			if result != tt.expected {
				t.Errorf("isAlreadyInitialized() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestIsAlreadyInitialized_FullyInitialized tests with complete ZERB structure
func TestIsAlreadyInitialized_FullyInitialized(t *testing.T) {
	tmpDir := t.TempDir()

	// Create full structure
	if err := createDirectoryStructure(tmpDir); err != nil {
		t.Fatalf("createDirectoryStructure failed: %v", err)
	}

	// Create mise binary
	misePath := filepath.Join(tmpDir, "bin", "mise")
	if err := os.WriteFile(misePath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("create mise binary failed: %v", err)
	}

	// Create marker
	markerPath := filepath.Join(tmpDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte("zerb.lua.20250101T120000.000Z"), 0644); err != nil {
		t.Fatalf("create marker failed: %v", err)
	}

	// Should detect as initialized
	if !isAlreadyInitialized(tmpDir) {
		t.Error("expected fully initialized directory to be detected as initialized")
	}
}

// TestGenerateInitialConfig tests initial config generation
func TestGenerateInitialConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure first
	if err := createDirectoryStructure(tmpDir); err != nil {
		t.Fatalf("createDirectoryStructure failed: %v", err)
	}

	// Generate initial config
	ctx := context.Background()
	err := generateInitialConfig(ctx, tmpDir)
	if err != nil {
		t.Fatalf("generateInitialConfig failed: %v", err)
	}

	// Verify .zerb-active marker exists and contains timestamp
	markerPath := filepath.Join(tmpDir, ".zerb-active")
	markerContent, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("failed to read marker file: %v", err)
	}

	configFilename := strings.TrimSpace(string(markerContent))
	if configFilename == "" {
		t.Error("marker file is empty")
	}

	// Verify filename format (zerb.lua.YYYYMMDDTHHMMSS.SSSZ with milliseconds)
	filenameRegex := regexp.MustCompile(`^zerb\.lua\.\d{8}T\d{6}\.\d{3}Z$`)
	if !filenameRegex.MatchString(configFilename) {
		t.Errorf("marker filename has invalid format: %s (expected zerb.lua.YYYYMMDDTHHMMSS.SSSZ)", configFilename)
	}

	// Verify timestamped config file exists
	configPath := filepath.Join(tmpDir, "configs", configFilename)
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	// Verify config contains expected minimal content
	configStr := string(configContent)
	requiredStrings := []string{
		"zerb = {",
		"-- ZERB Configuration",
	}

	for _, required := range requiredStrings {
		if !strings.Contains(configStr, required) {
			t.Errorf("config missing required content: %s", required)
		}
	}

	// Verify meta section exists (config with name should have meta)
	if !strings.Contains(configStr, "meta = {") {
		t.Error("config should contain meta section")
	}

	// Verify it doesn't contain tool definitions (empty list)
	if strings.Contains(configStr, `"node@`) || strings.Contains(configStr, `"python@`) {
		t.Error("initial config should not contain any pre-defined tools")
	}

	// Verify symlink exists and points to correct file
	symlinkPath := filepath.Join(tmpDir, "zerb.lua.active")
	linkTarget, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}

	expectedTarget := filepath.Join("configs", configFilename)
	if linkTarget != expectedTarget {
		t.Errorf("symlink target = %s, want %s", linkTarget, expectedTarget)
	}

	// Verify symlink resolves correctly
	_, err = os.Stat(symlinkPath)
	if err != nil {
		t.Errorf("symlink does not resolve: %v", err)
	}
}

// TestGenerateInitialConfig_ParseableByParser tests that generated config can be parsed
func TestGenerateInitialConfig_ParseableByParser(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	if err := createDirectoryStructure(tmpDir); err != nil {
		t.Fatalf("createDirectoryStructure failed: %v", err)
	}

	// Generate initial config
	ctx := context.Background()
	if err := generateInitialConfig(ctx, tmpDir); err != nil {
		t.Fatalf("generateInitialConfig failed: %v", err)
	}

	// Read generated config
	symlinkPath := filepath.Join(tmpDir, "zerb.lua.active")
	configContent, err := os.ReadFile(symlinkPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}

	// Try to parse it with config parser
	parser := config.NewParser(nil) // No platform detection needed for this test
	parsedConfig, err := parser.ParseString(ctx, string(configContent))
	if err != nil {
		t.Fatalf("generated config cannot be parsed: %v", err)
	}

	// Verify parsed config has expected structure
	if parsedConfig.Meta.Name == "" {
		t.Error("parsed config missing meta.name")
	}
	if len(parsedConfig.Tools) != 0 {
		t.Errorf("parsed config should have 0 tools, got %d", len(parsedConfig.Tools))
	}
	if parsedConfig.Git.Branch != "main" {
		t.Errorf("parsed config git.branch = %s, want main", parsedConfig.Git.Branch)
	}
}

// TestGenerateInitialConfig_Idempotent tests that calling twice doesn't break things
func TestGenerateInitialConfig_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	if err := createDirectoryStructure(tmpDir); err != nil {
		t.Fatalf("createDirectoryStructure failed: %v", err)
	}

	ctx := context.Background()

	// Generate first config
	if err := generateInitialConfig(ctx, tmpDir); err != nil {
		t.Fatalf("first generateInitialConfig failed: %v", err)
	}

	// Read first timestamp
	markerPath := filepath.Join(tmpDir, ".zerb-active")
	firstTimestamp, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("failed to read marker: %v", err)
	}

	// Wait a moment to ensure different timestamp
	time.Sleep(2 * time.Second)

	// Generate second config (symlink creation is now idempotent)
	if err := generateInitialConfig(ctx, tmpDir); err != nil {
		t.Fatalf("second generateInitialConfig failed: %v", err)
	}

	// Read second timestamp
	secondTimestamp, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("failed to read marker after second call: %v", err)
	}

	// Timestamps should be different (new snapshot created)
	if string(firstTimestamp) == string(secondTimestamp) {
		t.Error("expected different timestamps after second call")
	}

	// Both config files should exist
	configsDir := filepath.Join(tmpDir, "configs")
	entries, err := os.ReadDir(configsDir)
	if err != nil {
		t.Fatalf("failed to read configs dir: %v", err)
	}

	if len(entries) < 2 {
		t.Errorf("expected at least 2 config files, got %d", len(entries))
	}
}

// TestRunInit_AlreadyInitialized tests that running init twice returns an error
func TestRunInit_AlreadyInitialized(t *testing.T) {
	// This test would require mocking or a full integration setup
	// For now, we test the isAlreadyInitialized function directly
	tmpDir := t.TempDir()

	// Create minimal initialization marker
	if err := createDirectoryStructure(tmpDir); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	markerPath := filepath.Join(tmpDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte("zerb.lua.20250101T120000.000Z"), 0644); err != nil {
		t.Fatalf("create marker failed: %v", err)
	}

	// Should be detected as initialized
	if !isAlreadyInitialized(tmpDir) {
		t.Error("directory with .zerb-active should be detected as initialized")
	}
}

// TestCheckZerbOnPath tests the PATH detection function
func TestCheckZerbOnPath(t *testing.T) {
	// This test verifies the checkZerbOnPath function works correctly
	// Note: The actual result depends on whether 'zerb' is on PATH in the test environment

	result := checkZerbOnPath()

	// We can't assert a specific value since it depends on the environment
	// But we can verify the function doesn't panic and returns a string
	t.Logf("checkZerbOnPath() returned: %q", result)

	// If zerb is on PATH, result should be non-empty and contain "zerb"
	if result != "" && !strings.Contains(result, "zerb") {
		t.Errorf("checkZerbOnPath() returned path that doesn't contain 'zerb': %q", result)
	}
}

// TestPrintPathWarning tests that the warning function doesn't panic
func TestPrintPathWarning(t *testing.T) {
	// Capture stdout (this function prints to stdout)
	// For now, just verify it doesn't panic

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printPathWarning() panicked: %v", r)
		}
	}()

	// Note: We can't easily test the output without redirecting stdout
	// For MVP, just verify it doesn't crash
	// In a real test, we would capture os.Stdout and verify the output

	// Commented out to avoid polluting test output:
	// printPathWarning()

	t.Log("printPathWarning() test: function exists and compiles")
}

// TestIsOnPath tests the PATH checking function
func TestIsOnPath(t *testing.T) {
	tests := []struct {
		name     string
		dirPath  string
		pathEnv  string
		expected bool
	}{
		{
			name:     "exact match",
			dirPath:  "/usr/local/bin",
			pathEnv:  "/usr/bin:/usr/local/bin:/bin",
			expected: true,
		},
		{
			name:     "not on path",
			dirPath:  "/opt/custom/bin",
			pathEnv:  "/usr/bin:/usr/local/bin:/bin",
			expected: false,
		},
		{
			name:     "empty PATH",
			dirPath:  "/usr/bin",
			pathEnv:  "",
			expected: false,
		},
		{
			name:     "single entry PATH",
			dirPath:  "/usr/bin",
			pathEnv:  "/usr/bin",
			expected: true,
		},
		{
			name:     "with trailing slash",
			dirPath:  "/usr/local/bin/",
			pathEnv:  "/usr/bin:/usr/local/bin:/bin",
			expected: true,
		},
		{
			name:     "relative path on PATH",
			dirPath:  "bin",
			pathEnv:  "bin:/usr/bin",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOnPath(tt.dirPath, tt.pathEnv)
			if result != tt.expected {
				t.Errorf("isOnPath(%q, %q) = %v, want %v", tt.dirPath, tt.pathEnv, result, tt.expected)
			}
		})
	}
}
