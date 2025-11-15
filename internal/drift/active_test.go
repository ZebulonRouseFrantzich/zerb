package drift

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryActive(t *testing.T) {
	// Setup mock binaries in test PATH
	testPATH := SetupTestPATH(t, map[string]string{
		"node":   "20.11.0",
		"python": "3.12.1",
		"go":     "1.22.0",
	})

	// Save original PATH
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)

	// Set test PATH
	os.Setenv("PATH", testPATH)

	// Test with tool names (including one that doesn't exist)
	toolNames := []string{"node", "python", "go", "nonexistent"}

	tools, err := QueryActive(context.Background(), toolNames, false)
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	// Should find 3 tools, skip nonexistent
	if len(tools) != 3 {
		t.Errorf("QueryActive() returned %d tools, want 3", len(tools))
	}

	// Verify each tool
	want := map[string]string{
		"node":   "20.11.0",
		"python": "3.12.1",
		"go":     "1.22.0",
	}

	for _, tool := range tools {
		expectedVersion, exists := want[tool.Name]
		if !exists {
			t.Errorf("Unexpected tool found: %s", tool.Name)
			continue
		}

		if tool.Version != expectedVersion {
			t.Errorf("Tool %s version = %s, want %s", tool.Name, tool.Version, expectedVersion)
		}

		if tool.Path == "" {
			t.Errorf("Tool %s has empty path", tool.Name)
		}
	}
}

func TestQueryActive_EmptyList(t *testing.T) {
	tools, err := QueryActive(context.Background(), []string{}, false)
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("QueryActive(context.Background(), [], false) returned %d tools, want 0", len(tools))
	}
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		toolVersion string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "Node.js with --version",
			toolName:    "node",
			toolVersion: "20.11.0",
			wantVersion: "20.11.0",
		},
		{
			name:        "Python with --version",
			toolName:    "python",
			toolVersion: "3.12.1",
			wantVersion: "3.12.1",
		},
		{
			name:        "Go with --version",
			toolName:    "go",
			toolVersion: "1.22.0",
			wantVersion: "1.22.0",
		},
		{
			name:        "Ripgrep with --version",
			toolName:    "rg",
			toolVersion: "13.0.0",
			wantVersion: "13.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock binary
			tmpDir := t.TempDir()
			mockPath := CreateMockBinary(t, tmpDir, tt.toolName, tt.toolVersion)

			// Detect version
			version, err := DetectVersion(context.Background(), mockPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectVersion(context.Background(), ) error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if version != tt.wantVersion {
				t.Errorf("DetectVersion(context.Background(), ) = %q, want %q", version, tt.wantVersion)
			}
		})
	}
}

func TestDetectVersion_Fallback(t *testing.T) {
	// Test that -v fallback works when --version fails
	tmpDir := t.TempDir()

	// Create binary that only responds to -v
	script := `#!/bin/sh
if [ "$1" = "-v" ]; then
    echo "test-tool 2.5.3"
elif [ "$1" = "--version" ]; then
    exit 1
fi
`

	toolPath := filepath.Join(tmpDir, "test-tool")
	err := os.WriteFile(toolPath, []byte(script), 0755)
	if err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	version, err := DetectVersion(context.Background(), toolPath)
	if err != nil {
		t.Fatalf("DetectVersion(context.Background(), ) error = %v", err)
	}

	if version != "2.5.3" {
		t.Errorf("DetectVersion(context.Background(), ) = %q, want %q", version, "2.5.3")
	}
}

func TestDetectVersion_NoVersion(t *testing.T) {
	// Test binary that doesn't support version flags
	tmpDir := t.TempDir()

	script := `#!/bin/sh
echo "usage: test-tool [options]"
exit 1
`

	toolPath := filepath.Join(tmpDir, "test-tool")
	err := os.WriteFile(toolPath, []byte(script), 0755)
	if err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	_, err = DetectVersion(context.Background(), toolPath)
	if err == nil {
		t.Error("DetectVersion(context.Background(), ) expected error for tool without version support")
	}
}

func TestQueryActive_SymlinkResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create actual binary
	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0755)
	actualPath := CreateMockBinary(t, binDir, "node", "20.11.0")

	// Create symlink directory
	symlinkDir := filepath.Join(tmpDir, "links")
	os.MkdirAll(symlinkDir, 0755)
	symlinkPath := filepath.Join(symlinkDir, "node")
	err := os.Symlink(actualPath, symlinkPath)
	if err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Set PATH to symlink directory
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	os.Setenv("PATH", symlinkDir+":"+origPATH)

	// Query active tools
	tools, err := QueryActive(context.Background(), []string{"node"}, false)
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("QueryActive() returned %d tools, want 1", len(tools))
	}

	// Path should be resolved to actual binary, not symlink
	if tools[0].Path != actualPath {
		t.Errorf("QueryActive() resolved path = %q, want %q", tools[0].Path, actualPath)
	}
}

func TestGetVersionTimeout(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    string // duration as string for comparison
		wantDef bool   // expect default timeout
	}{
		{
			name:    "Default when no env var",
			envVal:  "",
			wantDef: true,
		},
		{
			name:   "Custom timeout from env",
			envVal: "5",
			want:   "5s",
		},
		{
			name:   "Large timeout from env",
			envVal: "30",
			want:   "30s",
		},
		{
			name:    "Invalid env var - use default",
			envVal:  "not-a-number",
			wantDef: true,
		},
		{
			name:    "Empty string - use default",
			envVal:  "",
			wantDef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env
			origVal := os.Getenv("ZERB_VERSION_TIMEOUT")
			defer func() {
				if origVal != "" {
					os.Setenv("ZERB_VERSION_TIMEOUT", origVal)
				} else {
					os.Unsetenv("ZERB_VERSION_TIMEOUT")
				}
			}()

			// Set test env
			if tt.envVal != "" {
				os.Setenv("ZERB_VERSION_TIMEOUT", tt.envVal)
			} else {
				os.Unsetenv("ZERB_VERSION_TIMEOUT")
			}

			got := getVersionTimeout()

			if tt.wantDef {
				if got != defaultVersionTimeout {
					t.Errorf("getVersionTimeout() = %v, want default %v", got, defaultVersionTimeout)
				}
			} else {
				if got.String() != tt.want {
					t.Errorf("getVersionTimeout() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestDetectVersionCached(t *testing.T) {
	// Clear cache before test
	versionCache.Lock()
	versionCache.entries = make(map[string]versionCacheEntry)
	versionCache.Unlock()

	tmpDir := t.TempDir()
	mockPath := CreateMockBinary(t, tmpDir, "test-tool", "1.0.0")

	// First call - cache miss
	version1, err := DetectVersionCached(context.Background(), mockPath, false)
	if err != nil {
		t.Fatalf("DetectVersionCached() first call error = %v", err)
	}
	if version1 != "1.0.0" {
		t.Errorf("DetectVersionCached() first call = %q, want %q", version1, "1.0.0")
	}

	// Second call - cache hit (should return same version without subprocess)
	version2, err := DetectVersionCached(context.Background(), mockPath, false)
	if err != nil {
		t.Fatalf("DetectVersionCached() second call error = %v", err)
	}
	if version2 != "1.0.0" {
		t.Errorf("DetectVersionCached() second call = %q, want %q", version2, "1.0.0")
	}

	// Third call with forceRefresh - should bypass cache
	version3, err := DetectVersionCached(context.Background(), mockPath, true)
	if err != nil {
		t.Fatalf("DetectVersionCached() forceRefresh call error = %v", err)
	}
	if version3 != "1.0.0" {
		t.Errorf("DetectVersionCached() forceRefresh call = %q, want %q", version3, "1.0.0")
	}
}

func TestDetectVersionCached_Expiry(t *testing.T) {
	// Clear cache before test
	versionCache.Lock()
	versionCache.entries = make(map[string]versionCacheEntry)
	versionCache.Unlock()

	tmpDir := t.TempDir()
	mockPath := CreateMockBinary(t, tmpDir, "test-tool", "2.0.0")

	// Populate cache with expired entry
	versionCache.Lock()
	versionCache.entries[mockPath] = versionCacheEntry{
		version:   "1.0.0",                           // Old version
		timestamp: time.Now().Add(-10 * time.Minute), // Expired (TTL is 5 minutes)
	}
	versionCache.Unlock()

	// Call should detect new version because cache is expired
	version, err := DetectVersionCached(context.Background(), mockPath, false)
	if err != nil {
		t.Fatalf("DetectVersionCached() error = %v", err)
	}
	if version != "2.0.0" {
		t.Errorf("DetectVersionCached() = %q, want %q (cache should have expired)", version, "2.0.0")
	}
}

func TestQueryActive_ForceRefresh(t *testing.T) {
	// Setup mock binaries in test PATH
	testPATH := SetupTestPATH(t, map[string]string{
		"node": "20.11.0",
	})

	// Save original PATH
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)

	// Set test PATH
	os.Setenv("PATH", testPATH)

	// Clear cache
	versionCache.Lock()
	versionCache.entries = make(map[string]versionCacheEntry)
	versionCache.Unlock()

	// First call without force refresh
	tools1, err := QueryActive(context.Background(), []string{"node"}, false)
	if err != nil {
		t.Fatalf("QueryActive() first call error = %v", err)
	}
	if len(tools1) != 1 {
		t.Fatalf("QueryActive() returned %d tools, want 1", len(tools1))
	}

	// Second call with force refresh
	tools2, err := QueryActive(context.Background(), []string{"node"}, true)
	if err != nil {
		t.Fatalf("QueryActive() second call error = %v", err)
	}
	if len(tools2) != 1 {
		t.Fatalf("QueryActive() returned %d tools, want 1", len(tools2))
	}

	// Both should return same version
	if tools1[0].Version != tools2[0].Version {
		t.Errorf("QueryActive() versions differ: %q vs %q", tools1[0].Version, tools2[0].Version)
	}
}

func TestCachePruning(t *testing.T) {
	// Clear cache before test
	versionCache.Lock()
	versionCache.entries = make(map[string]versionCacheEntry)
	versionCache.Unlock()

	tmpDir := t.TempDir()

	// Create mock binaries
	for i := 0; i < 110; i++ {
		toolName := fmt.Sprintf("tool%d", i)
		CreateMockBinary(t, tmpDir, toolName, "1.0.0")
	}

	// Add entries to fill cache beyond maxCacheEntries
	for i := 0; i < 110; i++ {
		toolPath := filepath.Join(tmpDir, fmt.Sprintf("tool%d", i))
		_, err := DetectVersionCached(context.Background(), toolPath, false)
		if err != nil {
			t.Fatalf("DetectVersionCached() error = %v", err)
		}
	}

	// Check that cache was pruned
	versionCache.RLock()
	cacheSize := len(versionCache.entries)
	versionCache.RUnlock()

	if cacheSize > maxCacheEntries {
		t.Errorf("Cache size = %d, want <= %d", cacheSize, maxCacheEntries)
	}
}

func TestCachePruning_ExpiredEntries(t *testing.T) {
	// Clear cache before test
	versionCache.Lock()
	versionCache.entries = make(map[string]versionCacheEntry)
	versionCache.Unlock()

	tmpDir := t.TempDir()
	mockPath := CreateMockBinary(t, tmpDir, "test-tool", "1.0.0")

	// Add an expired entry
	versionCache.Lock()
	versionCache.entries[mockPath] = versionCacheEntry{
		version:   "1.0.0",
		timestamp: time.Now().Add(-10 * time.Minute), // Expired (TTL is 5 minutes)
	}
	// Add many more entries to trigger pruning
	for i := 0; i < 105; i++ {
		fakePath := fmt.Sprintf("/fake/path/%d", i)
		versionCache.entries[fakePath] = versionCacheEntry{
			version:   "1.0.0",
			timestamp: time.Now().Add(-10 * time.Minute), // All expired
		}
	}
	versionCache.Unlock()

	// Trigger cache pruning by adding a new entry
	newPath := filepath.Join(tmpDir, "new-tool")
	CreateMockBinary(t, tmpDir, "new-tool", "2.0.0")
	_, err := DetectVersionCached(context.Background(), newPath, false)
	if err != nil {
		t.Fatalf("DetectVersionCached() error = %v", err)
	}

	// Check that expired entries were pruned
	versionCache.RLock()
	cacheSize := len(versionCache.entries)
	versionCache.RUnlock()

	if cacheSize > maxCacheEntries {
		t.Errorf("Cache size after pruning = %d, want <= %d", cacheSize, maxCacheEntries)
	}
}
