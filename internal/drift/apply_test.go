package drift

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

func TestRemoveToolFromList(t *testing.T) {
	tests := []struct {
		name     string
		tools    []string
		toolName string
		want     []string
	}{
		{
			name:     "Remove existing tool",
			tools:    []string{"node@20.11.0", "python@3.12.1"},
			toolName: "node",
			want:     []string{"python@3.12.1"},
		},
		{
			name:     "Remove tool with backend",
			tools:    []string{"cargo:ripgrep@13.0.0", "node@20.11.0"},
			toolName: "ripgrep",
			want:     []string{"node@20.11.0"},
		},
		{
			name:     "Remove non-existent tool",
			tools:    []string{"node@20.11.0"},
			toolName: "python",
			want:     []string{"node@20.11.0"},
		},
		{
			name:     "Remove from empty list",
			tools:    []string{},
			toolName: "node",
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeToolFromList(tt.tools, tt.toolName)
			if len(got) != len(tt.want) {
				t.Errorf("removeToolFromList() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("removeToolFromList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestUpdateToolVersion(t *testing.T) {
	tests := []struct {
		name       string
		tools      []string
		toolName   string
		newVersion string
		want       []string
	}{
		{
			name:       "Update simple tool",
			tools:      []string{"node@20.11.0"},
			toolName:   "node",
			newVersion: "20.15.0",
			want:       []string{"node@20.15.0"},
		},
		{
			name:       "Update tool with backend - cargo",
			tools:      []string{"cargo:ripgrep@13.0.0"},
			toolName:   "ripgrep",
			newVersion: "14.0.0",
			want:       []string{"cargo:ripgrep@14.0.0"},
		},
		{
			name:       "Update tool with backend - ubi",
			tools:      []string{"ubi:sharkdp/bat@0.24.0"},
			toolName:   "bat",
			newVersion: "0.25.0",
			want:       []string{"ubi:sharkdp/bat@0.25.0"},
		},
		{
			name:       "Update non-existent tool",
			tools:      []string{"node@20.11.0"},
			toolName:   "python",
			newVersion: "3.12.1",
			want:       []string{"node@20.11.0"},
		},
		{
			name:       "Update among multiple tools",
			tools:      []string{"node@20.11.0", "python@3.12.0", "go@1.22.0"},
			toolName:   "python",
			newVersion: "3.12.1",
			want:       []string{"node@20.11.0", "python@3.12.1", "go@1.22.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateToolVersion(tt.tools, tt.toolName, tt.newVersion)
			if len(got) != len(tt.want) {
				t.Errorf("updateToolVersion() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("updateToolVersion()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestUpdateToolsArray(t *testing.T) {
	tests := []struct {
		name   string
		tools  []string
		result DriftResult
		action DriftAction
		want   []string
	}{
		{
			name:  "External override - adopt (remove)",
			tools: []string{"python@3.12.1", "node@20.11.0"},
			result: DriftResult{
				Tool:      "python",
				DriftType: DriftExternalOverride,
			},
			action: ActionAdopt,
			want:   []string{"node@20.11.0"},
		},
		{
			name:  "Version mismatch - adopt (update)",
			tools: []string{"node@20.11.0"},
			result: DriftResult{
				Tool:          "node",
				DriftType:     DriftVersionMismatch,
				ActiveVersion: "20.15.0",
			},
			action: ActionAdopt,
			want:   []string{"node@20.15.0"},
		},
		{
			name:  "Extra tool - adopt (add)",
			tools: []string{"node@20.11.0"},
			result: DriftResult{
				Tool:           "rust",
				DriftType:      DriftExtra,
				ManagedVersion: "1.75.0",
			},
			action: ActionAdopt,
			want:   []string{"node@20.11.0", "rust@1.75.0"},
		},
		{
			name:  "Missing tool - adopt (remove)",
			tools: []string{"python@3.12.1", "node@20.11.0"},
			result: DriftResult{
				Tool:      "python",
				DriftType: DriftMissing,
			},
			action: ActionAdopt,
			want:   []string{"node@20.11.0"},
		},
		{
			name:  "Revert action - no change to tools array",
			tools: []string{"node@20.11.0"},
			result: DriftResult{
				Tool:      "node",
				DriftType: DriftVersionMismatch,
			},
			action: ActionRevert,
			want:   []string{"node@20.11.0"},
		},
		{
			name:  "Skip action - no change",
			tools: []string{"node@20.11.0"},
			result: DriftResult{
				Tool:      "node",
				DriftType: DriftVersionMismatch,
			},
			action: ActionSkip,
			want:   []string{"node@20.11.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateToolsArray(tt.tools, tt.result, tt.action)
			if len(got) != len(tt.want) {
				t.Errorf("updateToolsArray() length = %d, want %d\nGot: %v\nWant: %v", len(got), len(tt.want), got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("updateToolsArray()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestApplyAdopt_Integration(t *testing.T) {
	// Setup temp directory structure
	tmpDir := t.TempDir()
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}

	// Create initial config
	initialConfig := `zerb = {
  tools = {
    "node@20.11.0",
    "python@3.12.1",
  }
}`
	initialConfigPath := filepath.Join(configsDir, "zerb.lua.20250113T120000.000Z")
	if err := os.WriteFile(initialConfigPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Create marker file
	markerPath := filepath.Join(tmpDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte("20250113T120000.000Z"), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Create symlink
	symlinkPath := filepath.Join(tmpDir, "zerb.lua.active")
	symlinkTarget := filepath.Join("configs", "zerb.lua.20250113T120000.000Z")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Apply version mismatch drift (update node version)
	result := DriftResult{
		Tool:          "node",
		DriftType:     DriftVersionMismatch,
		ActiveVersion: "20.15.0",
	}

	err := applyAdopt(result, initialConfigPath, tmpDir)
	if err != nil {
		t.Fatalf("applyAdopt() error = %v", err)
	}

	// Verify new config was created
	entries, err := os.ReadDir(configsDir)
	if err != nil {
		t.Fatalf("failed to read configs dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 config files, got %d", len(entries))
	}

	// Read marker to find new config
	markerContent, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("failed to read marker: %v", err)
	}
	newTimestamp := string(markerContent)

	// Verify new config content
	newConfigPath := filepath.Join(configsDir, "zerb.lua."+newTimestamp)
	newConfigContent, err := os.ReadFile(newConfigPath)
	if err != nil {
		t.Fatalf("failed to read new config: %v", err)
	}

	// Parse and verify tools array
	parser := config.NewParser(nil)
	cfg, err := parser.ParseString(context.Background(), string(newConfigContent))
	if err != nil {
		t.Fatalf("failed to parse new config: %v", err)
	}

	// Check that node version was updated
	foundNode := false
	foundPython := false
	for _, tool := range cfg.Tools {
		if strings.HasPrefix(tool, "node@") {
			if tool != "node@20.15.0" {
				t.Errorf("expected node@20.15.0, got %s", tool)
			}
			foundNode = true
		}
		if strings.HasPrefix(tool, "python@") {
			if tool != "python@3.12.1" {
				t.Errorf("expected python@3.12.1, got %s", tool)
			}
			foundPython = true
		}
	}
	if !foundNode {
		t.Error("node not found in updated config")
	}
	if !foundPython {
		t.Error("python not found in updated config")
	}

	// Verify symlink was updated
	symlinkTarget, err = os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	expectedTarget := filepath.Join("configs", "zerb.lua."+newTimestamp)
	if symlinkTarget != expectedTarget {
		t.Errorf("symlink target = %q, want %q", symlinkTarget, expectedTarget)
	}
}
