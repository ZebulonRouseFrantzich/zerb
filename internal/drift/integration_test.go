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

func TestDriftDetection_EndToEnd(t *testing.T) {
	// Comprehensive end-to-end test covering all drift types
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, ".config", "zerb")
	zerbInstallsDir := filepath.Join(zerbDir, "installs")

	// Create ZERB directory structure
	if err := os.MkdirAll(zerbInstallsDir, 0755); err != nil {
		t.Fatalf("failed to create zerb dir: %v", err)
	}

	// Setup baseline config with 5 tools
	configPath := filepath.Join(tmpDir, "zerb.lua")
	configContent := `zerb = {
		tools = {
			"node@20.11.0",      -- Will be OK (matches)
			"python@3.12.1",     -- Will be version mismatch (3.11.0 installed)
			"go@1.22.0",         -- Will be missing (not installed)
			"rust@1.75.0",       -- Will be external override (system version)
		}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Parse baseline
	baseline, err := QueryBaseline(configPath)
	if err != nil {
		t.Fatalf("QueryBaseline() error = %v", err)
	}

	// Setup mock managed tools (ZERB installations)
	// Create ZERB-managed binaries in ZERB installs directory
	nodeInstallDir := filepath.Join(zerbInstallsDir, "node", "20.11.0", "bin")
	if err := os.MkdirAll(nodeInstallDir, 0755); err != nil {
		t.Fatalf("failed to create node install dir: %v", err)
	}
	CreateMockBinary(t, nodeInstallDir, "node", "20.11.0")

	pythonInstallDir := filepath.Join(zerbInstallsDir, "python", "3.11.0", "bin")
	if err := os.MkdirAll(pythonInstallDir, 0755); err != nil {
		t.Fatalf("failed to create python install dir: %v", err)
	}
	CreateMockBinary(t, pythonInstallDir, "python", "3.11.0") // Wrong version

	// Extra tool: ripgrep not in baseline
	ripgrepInstallDir := filepath.Join(zerbInstallsDir, "ripgrep", "13.0.0", "bin")
	if err := os.MkdirAll(ripgrepInstallDir, 0755); err != nil {
		t.Fatalf("failed to create ripgrep install dir: %v", err)
	}
	CreateMockBinary(t, ripgrepInstallDir, "rg", "13.0.0")

	// Mock managed tools list
	managed := []Tool{
		{Name: "node", Version: "20.11.0", Path: filepath.Join(nodeInstallDir, "node")},
		{Name: "python", Version: "3.11.0", Path: filepath.Join(pythonInstallDir, "python")},
		{Name: "rg", Version: "13.0.0", Path: filepath.Join(ripgrepInstallDir, "rg")},
	}

	// Setup mock active environment (PATH)
	// Create system directory for external tools
	systemDir := filepath.Join(tmpDir, "usr", "bin")
	if err := os.MkdirAll(systemDir, 0755); err != nil {
		t.Fatalf("failed to create system dir: %v", err)
	}
	CreateMockBinary(t, systemDir, "rust", "1.76.0") // External override (different from baseline)

	// Set up PATH with both ZERB and system directories
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	testPATH := systemDir + ":" + nodeInstallDir + ":" + pythonInstallDir + ":" + ripgrepInstallDir
	os.Setenv("PATH", testPATH)

	// Query active environment
	toolNames := []string{"node", "python", "go", "rust", "rg"}
	active, err := QueryActive(toolNames)
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	// Detect drift
	results := DetectDrift(baseline, managed, active, zerbDir)

	// Verify results
	if len(results) != 5 { // 4 baseline + 1 extra (rg)
		t.Errorf("DetectDrift() returned %d results, want 5", len(results))
	}

	// Build results map for easier verification
	resultsMap := make(map[string]DriftResult)
	for _, r := range results {
		resultsMap[r.Tool] = r
	}

	// Test case 1: node - should be OK
	if r, exists := resultsMap["node"]; exists {
		if r.DriftType != DriftOK {
			t.Errorf("node drift type = %v, want DriftOK", r.DriftType)
		}
		if r.BaselineVersion != "20.11.0" || r.ManagedVersion != "20.11.0" || r.ActiveVersion != "20.11.0" {
			t.Errorf("node versions mismatch: baseline=%s, managed=%s, active=%s",
				r.BaselineVersion, r.ManagedVersion, r.ActiveVersion)
		}
	} else {
		t.Error("node result not found")
	}

	// Test case 2: python - should be VERSION_MISMATCH
	if r, exists := resultsMap["python"]; exists {
		if r.DriftType != DriftVersionMismatch {
			t.Errorf("python drift type = %v, want DriftVersionMismatch", r.DriftType)
		}
		if r.BaselineVersion != "3.12.1" || r.ManagedVersion != "3.11.0" {
			t.Errorf("python versions: baseline=%s (expected 3.12.1), managed=%s (expected 3.11.0)",
				r.BaselineVersion, r.ManagedVersion)
		}
	} else {
		t.Error("python result not found")
	}

	// Test case 3: go - should be MISSING
	if r, exists := resultsMap["go"]; exists {
		if r.DriftType != DriftMissing {
			t.Errorf("go drift type = %v, want DriftMissing", r.DriftType)
		}
		if r.BaselineVersion != "1.22.0" {
			t.Errorf("go baseline version = %s, want 1.22.0", r.BaselineVersion)
		}
	} else {
		t.Error("go result not found")
	}

	// Test case 4: rust - should be EXTERNAL_OVERRIDE
	if r, exists := resultsMap["rust"]; exists {
		if r.DriftType != DriftExternalOverride {
			t.Errorf("rust drift type = %v, want DriftExternalOverride", r.DriftType)
		}
		if r.BaselineVersion != "1.75.0" || r.ActiveVersion != "1.76.0" {
			t.Errorf("rust versions: baseline=%s, active=%s", r.BaselineVersion, r.ActiveVersion)
		}
		// Verify path is NOT ZERB-managed
		if IsZERBManaged(r.ActivePath, zerbDir) {
			t.Errorf("rust path %s should not be ZERB-managed", r.ActivePath)
		}
	} else {
		t.Error("rust result not found")
	}

	// Test case 5: rg (ripgrep) - should be EXTRA
	if r, exists := resultsMap["rg"]; exists {
		if r.DriftType != DriftExtra {
			t.Errorf("rg drift type = %v, want DriftExtra", r.DriftType)
		}
		if r.ManagedVersion != "13.0.0" {
			t.Errorf("rg managed version = %s, want 13.0.0", r.ManagedVersion)
		}
	} else {
		t.Error("rg result not found")
	}
}

func TestDriftDetection_ManagedButNotActive(t *testing.T) {
	// Test case where ZERB has installed tool but it's not in PATH
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, ".config", "zerb")
	zerbInstallsDir := filepath.Join(zerbDir, "installs")

	if err := os.MkdirAll(zerbInstallsDir, 0755); err != nil {
		t.Fatalf("failed to create zerb dir: %v", err)
	}

	// Setup baseline
	configPath := filepath.Join(tmpDir, "zerb.lua")
	configContent := `zerb = { tools = { "node@20.11.0" } }`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	baseline, err := QueryBaseline(configPath)
	if err != nil {
		t.Fatalf("QueryBaseline() error = %v", err)
	}

	// Setup managed tool (installed by ZERB)
	nodeInstallDir := filepath.Join(zerbInstallsDir, "node", "20.11.0", "bin")
	if err := os.MkdirAll(nodeInstallDir, 0755); err != nil {
		t.Fatalf("failed to create node install dir: %v", err)
	}
	CreateMockBinary(t, nodeInstallDir, "node", "20.11.0")

	managed := []Tool{
		{Name: "node", Version: "20.11.0", Path: filepath.Join(nodeInstallDir, "node")},
	}

	// Set PATH to empty directory (tool not in PATH)
	emptyDir := t.TempDir()
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	os.Setenv("PATH", emptyDir)

	// Query active (should find nothing)
	active, err := QueryActive([]string{"node"})
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	// Detect drift
	results := DetectDrift(baseline, managed, active, zerbDir)

	if len(results) != 1 {
		t.Fatalf("DetectDrift() returned %d results, want 1", len(results))
	}

	// Should be MANAGED_BUT_NOT_ACTIVE
	if results[0].DriftType != DriftManagedButNotActive {
		t.Errorf("drift type = %v, want DriftManagedButNotActive", results[0].DriftType)
	}
}

func TestDriftDetection_VersionUnknown(t *testing.T) {
	// Test case where tool is found but version cannot be detected
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, ".config", "zerb")
	zerbInstallsDir := filepath.Join(zerbDir, "installs")

	if err := os.MkdirAll(zerbInstallsDir, 0755); err != nil {
		t.Fatalf("failed to create zerb dir: %v", err)
	}

	// Setup baseline
	configPath := filepath.Join(tmpDir, "zerb.lua")
	configContent := `zerb = { tools = { "mystery@1.0.0" } }`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	baseline, err := QueryBaseline(configPath)
	if err != nil {
		t.Fatalf("QueryBaseline() error = %v", err)
	}

	// Create tool without version support
	mysteryInstallDir := filepath.Join(zerbInstallsDir, "mystery", "1.0.0", "bin")
	if err := os.MkdirAll(mysteryInstallDir, 0755); err != nil {
		t.Fatalf("failed to create mystery install dir: %v", err)
	}

	// Create binary that doesn't support version flags
	script := `#!/bin/sh
echo "mystery tool - no version info"
exit 1
`
	toolPath := filepath.Join(mysteryInstallDir, "mystery")
	if err := os.WriteFile(toolPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mystery binary: %v", err)
	}

	managed := []Tool{
		{Name: "mystery", Version: "1.0.0", Path: toolPath},
	}

	// Set PATH
	origPATH := os.Getenv("PATH")
	defer os.Setenv("PATH", origPATH)
	os.Setenv("PATH", mysteryInstallDir)

	// Query active
	active, err := QueryActive([]string{"mystery"})
	if err != nil {
		t.Fatalf("QueryActive() error = %v", err)
	}

	// Detect drift
	results := DetectDrift(baseline, managed, active, zerbDir)

	if len(results) != 1 {
		t.Fatalf("DetectDrift() returned %d results, want 1", len(results))
	}

	// Should be VERSION_UNKNOWN
	if results[0].DriftType != DriftVersionUnknown {
		t.Errorf("drift type = %v, want DriftVersionUnknown", results[0].DriftType)
	}
	if results[0].ActiveVersion != "unknown" {
		t.Errorf("active version = %q, want %q", results[0].ActiveVersion, "unknown")
	}
}
