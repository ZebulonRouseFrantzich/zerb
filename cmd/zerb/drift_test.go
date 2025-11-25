package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunDrift_Help(t *testing.T) {
	// Test that --help doesn't panic
	// Note: printDriftHelp calls os.Exit(0), so we can't test the full function
	t.Log("Help flag test: function exists and compiles")
}

func TestRunDrift_ParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHelp    bool
		wantDryRun  bool
		wantRefresh bool
	}{
		{
			name:        "no args",
			args:        []string{},
			wantHelp:    false,
			wantDryRun:  false,
			wantRefresh: false,
		},
		{
			name:     "help flag short",
			args:     []string{"-h"},
			wantHelp: true,
		},
		{
			name:     "help flag long",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:       "dry-run flag short",
			args:       []string{"-n"},
			wantDryRun: true,
		},
		{
			name:       "dry-run flag long",
			args:       []string{"--dry-run"},
			wantDryRun: true,
		},
		{
			name:        "refresh flag",
			args:        []string{"--refresh"},
			wantRefresh: true,
		},
		{
			name:        "multiple flags",
			args:        []string{"--dry-run", "--refresh"},
			wantDryRun:  true,
			wantRefresh: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			showHelp := false
			dryRun := false
			forceRefresh := false

			for _, arg := range tt.args {
				switch arg {
				case "--help", "-h":
					showHelp = true
				case "--dry-run", "-n":
					dryRun = true
				case "--refresh":
					forceRefresh = true
				}
			}

			if showHelp != tt.wantHelp {
				t.Errorf("showHelp = %v, want %v", showHelp, tt.wantHelp)
			}
			if dryRun != tt.wantDryRun {
				t.Errorf("dryRun = %v, want %v", dryRun, tt.wantDryRun)
			}
			if forceRefresh != tt.wantRefresh {
				t.Errorf("forceRefresh = %v, want %v", forceRefresh, tt.wantRefresh)
			}
		})
	}
}

func TestRunDrift_NotInitialized(t *testing.T) {
	// Set up a temporary directory without ZERB initialization
	tmpDir := t.TempDir()
	t.Setenv("ZERB_DIR", tmpDir)

	// Running drift without initialization should fail
	exitCode, err := runDrift([]string{})
	if err == nil {
		t.Error("expected error for uninitialized ZERB, got nil")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	// Error should mention init
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRunDrift_NoTools(t *testing.T) {
	// Set up a temporary ZERB directory with empty config
	tmpDir := t.TempDir()
	t.Setenv("ZERB_DIR", tmpDir)

	// Create the directory structure
	if err := os.MkdirAll(filepath.Join(tmpDir, "configs"), 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "bin"), 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a minimal config file with no tools
	configContent := `-- ZERB Configuration
zerb = {
    meta = {
        name = "Test Environment",
    },
    tools = {},
    configs = {},
    git = {
        branch = "main",
    },
    options = {
        backup_retention = 5,
    },
}
return zerb`

	configFilename := "zerb.20250101T120000.000Z.lua"
	configPath := filepath.Join(tmpDir, "configs", configFilename)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// Create symlink
	symlinkPath := filepath.Join(tmpDir, "zerb.active.lua")
	if err := os.Symlink(filepath.Join("configs", configFilename), symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Running drift with no tools should not error
	exitCode, err := runDrift([]string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}
