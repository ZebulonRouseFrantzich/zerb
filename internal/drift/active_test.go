package drift

import (
	"os"
	"path/filepath"
	"testing"
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

	tools, err := QueryActive(toolNames)
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
	tools, err := QueryActive([]string{})
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("QueryActive([]) returned %d tools, want 0", len(tools))
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
			version, err := DetectVersion(mockPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if version != tt.wantVersion {
				t.Errorf("DetectVersion() = %q, want %q", version, tt.wantVersion)
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

	version, err := DetectVersion(toolPath)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}

	if version != "2.5.3" {
		t.Errorf("DetectVersion() = %q, want %q", version, "2.5.3")
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

	_, err = DetectVersion(toolPath)
	if err == nil {
		t.Error("DetectVersion() expected error for tool without version support")
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
	tools, err := QueryActive([]string{"node"})
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
