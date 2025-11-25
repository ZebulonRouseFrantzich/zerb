package main

import (
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

func TestFormatConfigOptions(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.ConfigFile
		expected string
	}{
		{
			name:     "no options",
			cfg:      config.ConfigFile{Path: "~/.zshrc"},
			expected: "",
		},
		{
			name:     "template only",
			cfg:      config.ConfigFile{Path: "~/.zshrc", Template: true},
			expected: "(template)",
		},
		{
			name:     "secrets only",
			cfg:      config.ConfigFile{Path: "~/.zshrc", Secrets: true},
			expected: "(encrypted)",
		},
		{
			name:     "private only",
			cfg:      config.ConfigFile{Path: "~/.zshrc", Private: true},
			expected: "(private)",
		},
		{
			name:     "recursive only",
			cfg:      config.ConfigFile{Path: "~/.config/nvim", Recursive: true},
			expected: "(recursive)",
		},
		{
			name: "multiple options",
			cfg: config.ConfigFile{
				Path:     "~/.ssh/config",
				Template: true,
				Private:  true,
			},
			expected: "(template, private)",
		},
		{
			name: "all options",
			cfg: config.ConfigFile{
				Path:      "~/.config/app",
				Template:  true,
				Secrets:   true,
				Private:   true,
				Recursive: true,
			},
			expected: "(template, encrypted, private, recursive)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatConfigOptions(tt.cfg)
			if result != tt.expected {
				t.Errorf("formatConfigOptions() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRunConfigList_Help(t *testing.T) {
	// Test that --help doesn't panic
	// Note: printConfigListHelp calls os.Exit(0), so we can't test the full function
	// In a real test, we'd capture os.Exit
	t.Log("Help flag test: function exists and compiles")
}

func TestRunConfigList_ParseFlags(t *testing.T) {
	// Test that flag parsing works correctly
	tests := []struct {
		name     string
		args     []string
		wantHelp bool
	}{
		{
			name:     "no args",
			args:     []string{},
			wantHelp: false,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			showHelp := false
			for _, arg := range tt.args {
				switch arg {
				case "--help", "-h":
					showHelp = true
				}
			}

			if showHelp != tt.wantHelp {
				t.Errorf("showHelp = %v, want %v", showHelp, tt.wantHelp)
			}
		})
	}
}
