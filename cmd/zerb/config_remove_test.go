package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRemove_ParseFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantHelp   bool
		wantDryRun bool
		wantPurge  bool
		wantYes    bool
		wantPaths  []string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "single path",
			args:      []string{"~/.zshrc"},
			wantPaths: []string{"~/.zshrc"},
		},
		{
			name:      "multiple paths",
			args:      []string{"~/.zshrc", "~/.gitconfig"},
			wantPaths: []string{"~/.zshrc", "~/.gitconfig"},
		},
		{
			name:       "dry run flag",
			args:       []string{"--dry-run", "~/.zshrc"},
			wantDryRun: true,
			wantPaths:  []string{"~/.zshrc"},
		},
		{
			name:       "dry run short flag",
			args:       []string{"-n", "~/.zshrc"},
			wantDryRun: true,
			wantPaths:  []string{"~/.zshrc"},
		},
		{
			name:      "purge flag",
			args:      []string{"--purge", "~/.zshrc"},
			wantPurge: true,
			wantPaths: []string{"~/.zshrc"},
		},
		{
			name:      "yes flag",
			args:      []string{"--yes", "~/.zshrc"},
			wantYes:   true,
			wantPaths: []string{"~/.zshrc"},
		},
		{
			name:      "yes short flag",
			args:      []string{"-y", "~/.zshrc"},
			wantYes:   true,
			wantPaths: []string{"~/.zshrc"},
		},
		{
			name:     "help flag",
			args:     []string{"--help"},
			wantHelp: true,
		},
		{
			name:     "help short flag",
			args:     []string{"-h"},
			wantHelp: true,
		},
		{
			name:       "all flags combined",
			args:       []string{"--dry-run", "--purge", "--yes", "~/.zshrc"},
			wantDryRun: true,
			wantPurge:  true,
			wantYes:    true,
			wantPaths:  []string{"~/.zshrc"},
		},
		{
			name:       "unknown flag",
			args:       []string{"--unknown", "~/.zshrc"},
			wantErr:    true,
			wantErrMsg: "unknown option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseConfigRemoveArgs(tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseConfigRemoveArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.wantErrMsg != "" && err != nil {
					if !containsStr(err.Error(), tt.wantErrMsg) {
						t.Errorf("parseConfigRemoveArgs() error = %q, want containing %q", err.Error(), tt.wantErrMsg)
					}
				}
				return
			}

			if opts.showHelp != tt.wantHelp {
				t.Errorf("showHelp = %v, want %v", opts.showHelp, tt.wantHelp)
			}
			if opts.dryRun != tt.wantDryRun {
				t.Errorf("dryRun = %v, want %v", opts.dryRun, tt.wantDryRun)
			}
			if opts.purge != tt.wantPurge {
				t.Errorf("purge = %v, want %v", opts.purge, tt.wantPurge)
			}
			if opts.yes != tt.wantYes {
				t.Errorf("yes = %v, want %v", opts.yes, tt.wantYes)
			}

			if len(opts.paths) != len(tt.wantPaths) {
				t.Errorf("paths = %v, want %v", opts.paths, tt.wantPaths)
			} else {
				for i := range opts.paths {
					if opts.paths[i] != tt.wantPaths[i] {
						t.Errorf("paths[%d] = %q, want %q", i, opts.paths[i], tt.wantPaths[i])
					}
				}
			}
		})
	}
}

func TestConfigRemove_NoPathsError(t *testing.T) {
	// Create a temp zerb directory
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	zerbDir := filepath.Join(homeDir, ".config", "zerb")
	os.MkdirAll(zerbDir, 0755)
	os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
	os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)

	// Test with no paths
	err := runConfigRemove([]string{})
	if err == nil {
		t.Error("runConfigRemove() should return error when no paths provided")
	}
	if err != nil && !containsStr(err.Error(), "no paths specified") {
		t.Errorf("runConfigRemove() error = %q, want containing 'no paths specified'", err.Error())
	}
}

func TestConfigRemove_NotInitializedError(t *testing.T) {
	// Create a temp directory without zerb initialization
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Don't create zerb directory

	err := runConfigRemove([]string{"~/.zshrc", "--yes"})
	if err == nil {
		t.Error("runConfigRemove() should return error when ZERB not initialized")
	}
	if err != nil && !containsStr(err.Error(), "not initialized") {
		t.Errorf("runConfigRemove() error = %q, want containing 'not initialized'", err.Error())
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
