package drift

import (
	"context"
	"fmt"
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
		{
			name:     "Keep unparseable tool spec",
			tools:    []string{"invalid-spec", "node@20.11.0"},
			toolName: "node",
			want:     []string{"invalid-spec"},
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
		{
			name:       "Keep unparseable tool spec",
			tools:      []string{"invalid-spec", "node@20.11.0"},
			toolName:   "node",
			newVersion: "20.15.0",
			want:       []string{"invalid-spec", "node@20.15.0"},
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
		{
			name:  "ManagedButNotActive - adopt (remove)",
			tools: []string{"python@3.12.1", "node@20.11.0"},
			result: DriftResult{
				Tool:      "python",
				DriftType: DriftManagedButNotActive,
			},
			action: ActionAdopt,
			want:   []string{"node@20.11.0"},
		},
		{
			name:  "VersionUnknown - adopt (remove)",
			tools: []string{"python@3.12.1", "node@20.11.0"},
			result: DriftResult{
				Tool:      "python",
				DriftType: DriftVersionUnknown,
			},
			action: ActionAdopt,
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
	if err := os.WriteFile(markerPath, []byte("zerb.lua.20250113T120000.000Z"), 0644); err != nil {
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
	newConfigFilename := strings.TrimSpace(string(markerContent))

	// Verify new config content
	newConfigPath := filepath.Join(configsDir, newConfigFilename)
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
	expectedTarget := filepath.Join("configs", newConfigFilename)
	if symlinkTarget != expectedTarget {
		t.Errorf("symlink target = %q, want %q", symlinkTarget, expectedTarget)
	}
}

func TestApplyDriftAction(t *testing.T) {
	tests := []struct {
		name       string
		action     DriftAction
		result     DriftResult
		wantErr    bool
		errPattern string // substring that should be in error message
	}{
		{
			name:   "ActionSkip - no error",
			action: ActionSkip,
			result: DriftResult{
				Tool:      "node",
				DriftType: DriftVersionMismatch,
			},
			wantErr: false,
		},
		{
			name:   "Unknown action",
			action: DriftAction(999),
			result: DriftResult{
				Tool:      "node",
				DriftType: DriftVersionMismatch,
			},
			wantErr:    true,
			errPattern: "unknown action",
		},
		{
			name:   "ActionAdopt - version mismatch",
			action: ActionAdopt,
			result: DriftResult{
				Tool:          "node",
				DriftType:     DriftVersionMismatch,
				ActiveVersion: "20.15.0",
			},
			wantErr: false,
		},
		{
			name:   "ActionRevert - version mismatch",
			action: ActionRevert,
			result: DriftResult{
				Tool:            "python",
				DriftType:       DriftVersionMismatch,
				BaselineVersion: "3.12.1",
				ActiveVersion:   "3.11.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			configsDir := filepath.Join(tmpDir, "configs")
			os.MkdirAll(configsDir, 0755)
			binDir := filepath.Join(tmpDir, "bin")
			os.MkdirAll(binDir, 0755)

			// Create a basic config file
			initialConfig := `zerb = {
  tools = {
    "node@20.11.0",
  }
}`
			configPath := filepath.Join(configsDir, "zerb.lua.20250113T120000.000Z")
			os.WriteFile(configPath, []byte(initialConfig), 0644)

			// Create marker and symlink
			markerPath := filepath.Join(tmpDir, ".zerb-active")
			os.WriteFile(markerPath, []byte("zerb.lua.20250113T120000.000Z"), 0644)
			symlinkPath := filepath.Join(tmpDir, "zerb.lua.active")
			os.Symlink(filepath.Join("configs", "zerb.lua.20250113T120000.000Z"), symlinkPath)

			// Create mock mise
			miseScript := `#!/bin/sh
echo "mock mise: $@"
exit 0
`
			misePath := filepath.Join(binDir, "mise")
			os.WriteFile(misePath, []byte(miseScript), 0755)

			err := ApplyDriftAction(context.Background(), tt.result, tt.action, configPath, tmpDir, misePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyDriftAction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errPattern != "" {
				if !strings.Contains(err.Error(), tt.errPattern) {
					t.Errorf("ApplyDriftAction() error = %q, want error containing %q", err.Error(), tt.errPattern)
				}
			}
		})
	}
}

func TestApplyRevert(t *testing.T) {
	tests := []struct {
		name       string
		result     DriftResult
		wantErr    bool
		errPattern string // substring that should be in error message
	}{
		{
			name: "Invalid tool name - shell metacharacter",
			result: DriftResult{
				Tool:            "node; rm -rf /",
				DriftType:       DriftVersionMismatch,
				BaselineVersion: "20.11.0",
			},
			wantErr:    true,
			errPattern: "invalid tool name",
		},
		{
			name: "Invalid version - shell metacharacter",
			result: DriftResult{
				Tool:            "node",
				DriftType:       DriftVersionMismatch,
				BaselineVersion: "20.11.0; rm -rf /",
			},
			wantErr:    true,
			errPattern: "invalid",
		},
		{
			name: "DriftManagedButNotActive - manual intervention required",
			result: DriftResult{
				Tool:            "node",
				DriftType:       DriftManagedButNotActive,
				BaselineVersion: "20.11.0",
			},
			wantErr:    true,
			errPattern: "manual PATH investigation",
		},
		{
			name: "DriftExternalOverride - install baseline version",
			result: DriftResult{
				Tool:            "node",
				DriftType:       DriftExternalOverride,
				BaselineVersion: "20.11.0",
				ActiveVersion:   "20.15.0",
			},
			wantErr: false,
		},
		{
			name: "DriftVersionMismatch - install baseline version",
			result: DriftResult{
				Tool:            "python",
				DriftType:       DriftVersionMismatch,
				BaselineVersion: "3.12.1",
				ActiveVersion:   "3.11.0",
			},
			wantErr: false,
		},
		{
			name: "DriftMissing - install missing tool",
			result: DriftResult{
				Tool:            "go",
				DriftType:       DriftMissing,
				BaselineVersion: "1.22.0",
			},
			wantErr: false,
		},
		{
			name: "DriftExtra - uninstall extra tool",
			result: DriftResult{
				Tool:           "rust",
				DriftType:      DriftExtra,
				ManagedVersion: "1.75.0",
			},
			wantErr: false,
		},
		{
			name: "DriftVersionUnknown - reinstall to fix version detection",
			result: DriftResult{
				Tool:            "node",
				DriftType:       DriftVersionUnknown,
				BaselineVersion: "20.11.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with mock mise
			tmpDir := t.TempDir()
			binDir := filepath.Join(tmpDir, "bin")
			os.MkdirAll(binDir, 0755)

			// Create mock mise that always succeeds
			miseScript := `#!/bin/sh
echo "mock mise: $@"
exit 0
`
			misePath := filepath.Join(binDir, "mise")
			os.WriteFile(misePath, []byte(miseScript), 0755)

			err := applyRevert(context.Background(), tt.result, misePath, tmpDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyRevert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errPattern != "" {
				if !strings.Contains(err.Error(), tt.errPattern) {
					t.Errorf("applyRevert() error = %q, want error containing %q", err.Error(), tt.errPattern)
				}
			}
		})
	}
}

func TestExecuteMiseInstallOrUninstall(t *testing.T) {
	tests := []struct {
		name         string
		miseExitCode int
		wantErr      bool
	}{
		{
			name:         "Successful install",
			miseExitCode: 0,
			wantErr:      false,
		},
		{
			name:         "Failed install",
			miseExitCode: 1,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with mock mise
			tmpDir := t.TempDir()
			binDir := filepath.Join(tmpDir, "bin")
			os.MkdirAll(binDir, 0755)

			// Create mock mise that exits with specified code
			miseScript := fmt.Sprintf(`#!/bin/sh
exit %d
`, tt.miseExitCode)
			misePath := filepath.Join(binDir, "mise")
			os.WriteFile(misePath, []byte(miseScript), 0755)

			err := executeMiseInstallOrUninstall(context.Background(), misePath, tmpDir, "install", "node@20.11.0")
			if (err != nil) != tt.wantErr {
				t.Errorf("executeMiseInstallOrUninstall() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyAdopt_ErrorCases(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func() (configPath, zerbDir string)
		result     DriftResult
		wantErr    bool
		errPattern string
	}{
		{
			name: "Config file not found",
			setupFunc: func() (string, string) {
				return "/nonexistent/config.lua", t.TempDir()
			},
			result: DriftResult{
				Tool:          "node",
				DriftType:     DriftVersionMismatch,
				ActiveVersion: "20.15.0",
			},
			wantErr:    true,
			errPattern: "read config",
		},
		{
			name: "Invalid config content",
			setupFunc: func() (string, string) {
				tmpDir := t.TempDir()
				configsDir := filepath.Join(tmpDir, "configs")
				os.MkdirAll(configsDir, 0755)
				configPath := filepath.Join(configsDir, "zerb.lua.test")
				// Write invalid Lua
				os.WriteFile(configPath, []byte("this is not valid lua @@@@"), 0644)
				return configPath, tmpDir
			},
			result: DriftResult{
				Tool:          "node",
				DriftType:     DriftVersionMismatch,
				ActiveVersion: "20.15.0",
			},
			wantErr:    true,
			errPattern: "parse config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath, zerbDir := tt.setupFunc()
			err := applyAdopt(tt.result, configPath, zerbDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyAdopt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errPattern != "" {
				if !strings.Contains(err.Error(), tt.errPattern) {
					t.Errorf("applyAdopt() error = %q, want error containing %q", err.Error(), tt.errPattern)
				}
			}
		})
	}
}
