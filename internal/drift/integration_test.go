package drift

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataCollection_Integration(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()

	// 1. Create test config
	configPath := filepath.Join(tmpDir, "zerb.lua")
	configContent := `zerb = {
		tools = {
			"node@20.11.0",
			"python@3.12.1",
			"go@1.22.0",
		}
	}`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// 2. Test baseline collection
	baseline, err := QueryBaseline(configPath)
	if err != nil {
		t.Fatalf("QueryBaseline() error = %v", err)
	}

	if len(baseline) != 3 {
		t.Errorf("QueryBaseline() returned %d tools, want 3", len(baseline))
	}

	// Verify baseline tools
	expectedTools := map[string]string{
		"node":   "20.11.0",
		"python": "3.12.1",
		"go":     "1.22.0",
	}

	for _, spec := range baseline {
		expectedVersion, exists := expectedTools[spec.Name]
		if !exists {
			t.Errorf("Unexpected tool in baseline: %s", spec.Name)
			continue
		}
		if spec.Version != expectedVersion {
			t.Errorf("Baseline tool %s version = %s, want %s", spec.Name, spec.Version, expectedVersion)
		}
	}

	// 3. Setup mock PATH (ONLY mock tools, no system PATH)
	mockDir := t.TempDir()
	CreateMockBinary(t, mockDir, "node", "20.11.0")
	CreateMockBinary(t, mockDir, "python", "3.12.1")

	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	// Use ONLY mock directory as PATH to avoid finding system tools
	os.Setenv("PATH", mockDir)

	// 4. Test active collection
	toolNames := []string{"node", "python", "go"}
	active, err := QueryActive(toolNames)
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	// Should only find node and python (go is not in mock PATH)
	if len(active) != 2 {
		t.Errorf("QueryActive() returned %d tools, want 2 (only node and python)", len(active))
	}

	// 5. Verify data structure consistency for all tools
	for _, tool := range active {
		if tool.Name == "" || tool.Path == "" {
			t.Errorf("Active tool has empty required fields: %+v", tool)
		}
		// Version can be "unknown" for tools without version support
	}

	// Verify our mock tools are present with correct versions
	foundTools := make(map[string]string)
	for _, tool := range active {
		foundTools[tool.Name] = tool.Version
	}

	// Check that node and python (our mocks) are found
	for _, expectedTool := range []string{"node", "python"} {
		version, found := foundTools[expectedTool]
		if !found {
			t.Errorf("Expected mock tool %s not found in active tools", expectedTool)
			continue
		}
		expectedVersion := expectedTools[expectedTool]
		if version != expectedVersion {
			t.Errorf("Active tool %s version = %s, want %s", expectedTool, version, expectedVersion)
		}
	}
}

func TestDataCollection_WithBackends(t *testing.T) {
	// Test that backend-prefixed tools are parsed correctly
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "zerb.lua")
	configContent := `zerb = {
		tools = {
			"cargo:ripgrep@13.0.0",
			"ubi:sharkdp/bat@0.24.0",
			"npm:prettier@3.0.0",
		}
	}`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	baseline, err := QueryBaseline(configPath)
	if err != nil {
		t.Fatalf("QueryBaseline() error = %v", err)
	}

	if len(baseline) != 3 {
		t.Errorf("QueryBaseline() returned %d tools, want 3", len(baseline))
	}

	// Verify backend parsing
	expectedBackends := map[string]string{
		"ripgrep":  "cargo",
		"bat":      "ubi",
		"prettier": "npm",
	}

	for _, spec := range baseline {
		expectedBackend, exists := expectedBackends[spec.Name]
		if !exists {
			t.Errorf("Unexpected tool: %s", spec.Name)
			continue
		}
		if spec.Backend != expectedBackend {
			t.Errorf("Tool %s backend = %s, want %s", spec.Name, spec.Backend, expectedBackend)
		}
	}
}

func TestDataCollection_VersionUnknown(t *testing.T) {
	// Test that tools without version info are marked as "unknown"
	tmpDir := t.TempDir()

	// Create binary that doesn't support --version or -v
	script := `#!/bin/sh
echo "usage: mystery-tool [options]"
exit 1
`
	toolPath := filepath.Join(tmpDir, "mystery-tool")
	err := os.WriteFile(toolPath, []byte(script), 0755)
	if err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Set PATH
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	os.Setenv("PATH", tmpDir+":"+origPATH)

	// Query active
	active, err := QueryActive([]string{"mystery-tool"})
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("QueryActive() returned %d tools, want 1", len(active))
	}

	// Version should be "unknown"
	if active[0].Version != "unknown" {
		t.Errorf("Tool version = %q, want %q", active[0].Version, "unknown")
	}
}
