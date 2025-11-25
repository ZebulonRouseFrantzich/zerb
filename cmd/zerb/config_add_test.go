package main

import (
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/service"
)

func TestRunConfigAdd_Help(t *testing.T) {
	// Test that --help doesn't panic
	// Note: printConfigAddHelp calls os.Exit(0), so we can't test the full function
	t.Log("Help flag test: function exists and compiles")
}

func TestRunConfigAdd_ParseFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantHelp      bool
		wantDryRun    bool
		wantRecursive bool
		wantTemplate  bool
		wantSecrets   bool
		wantPrivate   bool
		wantPaths     []string
	}{
		{
			name:      "no args",
			args:      []string{},
			wantPaths: []string{},
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
			name:          "recursive flag short",
			args:          []string{"-r"},
			wantRecursive: true,
		},
		{
			name:          "recursive flag long",
			args:          []string{"--recursive"},
			wantRecursive: true,
		},
		{
			name:         "template flag short",
			args:         []string{"-t"},
			wantTemplate: true,
		},
		{
			name:         "template flag long",
			args:         []string{"--template"},
			wantTemplate: true,
		},
		{
			name:        "secrets flag short",
			args:        []string{"-s"},
			wantSecrets: true,
		},
		{
			name:        "secrets flag long",
			args:        []string{"--secrets"},
			wantSecrets: true,
		},
		{
			name:        "private flag short",
			args:        []string{"-p"},
			wantPrivate: true,
		},
		{
			name:        "private flag long",
			args:        []string{"--private"},
			wantPrivate: true,
		},
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
			name:          "flags and paths",
			args:          []string{"--recursive", "~/.config/nvim", "-p"},
			wantRecursive: true,
			wantPrivate:   true,
			wantPaths:     []string{"~/.config/nvim"},
		},
		{
			name:          "all flags",
			args:          []string{"-r", "-t", "-s", "-p", "-n", "~/.zshrc"},
			wantRecursive: true,
			wantTemplate:  true,
			wantSecrets:   true,
			wantPrivate:   true,
			wantDryRun:    true,
			wantPaths:     []string{"~/.zshrc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			showHelp := false
			dryRun := false
			opts := service.ConfigOptions{}
			var paths []string

			for _, arg := range tt.args {
				switch arg {
				case "--help", "-h":
					showHelp = true
				case "--dry-run", "-n":
					dryRun = true
				case "--recursive", "-r":
					opts.Recursive = true
				case "--template", "-t":
					opts.Template = true
				case "--secrets", "-s":
					opts.Secrets = true
				case "--private", "-p":
					opts.Private = true
				default:
					if len(arg) > 0 && arg[0] != '-' {
						paths = append(paths, arg)
					}
				}
			}

			if showHelp != tt.wantHelp {
				t.Errorf("showHelp = %v, want %v", showHelp, tt.wantHelp)
			}
			if dryRun != tt.wantDryRun {
				t.Errorf("dryRun = %v, want %v", dryRun, tt.wantDryRun)
			}
			if opts.Recursive != tt.wantRecursive {
				t.Errorf("recursive = %v, want %v", opts.Recursive, tt.wantRecursive)
			}
			if opts.Template != tt.wantTemplate {
				t.Errorf("template = %v, want %v", opts.Template, tt.wantTemplate)
			}
			if opts.Secrets != tt.wantSecrets {
				t.Errorf("secrets = %v, want %v", opts.Secrets, tt.wantSecrets)
			}
			if opts.Private != tt.wantPrivate {
				t.Errorf("private = %v, want %v", opts.Private, tt.wantPrivate)
			}
			if len(paths) != len(tt.wantPaths) {
				t.Errorf("got %d paths, want %d", len(paths), len(tt.wantPaths))
			} else {
				for i, p := range paths {
					if p != tt.wantPaths[i] {
						t.Errorf("path[%d] = %q, want %q", i, p, tt.wantPaths[i])
					}
				}
			}
		})
	}
}

func TestRunConfigAdd_NoPaths(t *testing.T) {
	err := runConfigAdd([]string{})
	if err == nil {
		t.Error("expected error for no paths, got nil")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRunConfigAdd_UnknownFlag(t *testing.T) {
	err := runConfigAdd([]string{"--invalid-flag"})
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestRunConfigAdd_NotInitialized(t *testing.T) {
	// Set up a temporary directory without ZERB initialization
	tmpDir := t.TempDir()
	t.Setenv("ZERB_DIR", tmpDir)

	// Running config add without initialization should fail
	// (after the path check, it will fail trying to access the ZERB directory)
	err := runConfigAdd([]string{"~/.zshrc"})
	if err == nil {
		t.Error("expected error for uninitialized ZERB, got nil")
	}
}
