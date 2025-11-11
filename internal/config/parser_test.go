package config

import (
	"context"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
	lua "github.com/yuin/gopher-lua"
)

// mockDetector is a test implementation of platform.Detector.
type mockDetector struct {
	info *platform.Info
	err  error
}

func (m *mockDetector) Detect(ctx context.Context) (*platform.Info, error) {
	return m.info, m.err
}

func TestParser_ParseString_Minimal(t *testing.T) {
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
			},
		}
	`

	parser := NewParser(nil) // No platform detection for minimal test
	config, err := parser.ParseString(context.Background(), luaCode)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if len(config.Tools) != 1 {
		t.Errorf("Tools length = %d, want 1", len(config.Tools))
	}
	if config.Tools[0] != "node@20.11.0" {
		t.Errorf("Tools[0] = %s, want node@20.11.0", config.Tools[0])
	}
}

func TestParser_ParseString_Full(t *testing.T) {
	luaCode := `
		zerb = {
			meta = {
				name = "My Dev Environment",
				description = "Full-stack development",
			},
			tools = {
				"node@20.11.0",
				"python@3.12.1",
				"cargo:ripgrep",
			},
			configs = {
				"~/.zshrc",
				"~/.gitconfig",
				{
					path = "~/.config/nvim/",
					recursive = true,
				},
				{
					path = "~/.ssh/config",
					template = true,
					secrets = true,
					private = true,
				},
			},
			git = {
				remote = "https://github.com/user/dotfiles",
				branch = "main",
			},
			config = {
				backup_retention = 5,
			},
		}
	`

	parser := NewParser(nil)
	config, err := parser.ParseString(context.Background(), luaCode)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	// Check meta
	if config.Meta.Name != "My Dev Environment" {
		t.Errorf("Meta.Name = %s, want My Dev Environment", config.Meta.Name)
	}
	if config.Meta.Description != "Full-stack development" {
		t.Errorf("Meta.Description = %s, want Full-stack development", config.Meta.Description)
	}

	// Check tools
	if len(config.Tools) != 3 {
		t.Errorf("Tools length = %d, want 3", len(config.Tools))
	}
	expectedTools := []string{"node@20.11.0", "python@3.12.1", "cargo:ripgrep"}
	for i, expected := range expectedTools {
		if i >= len(config.Tools) {
			break
		}
		if config.Tools[i] != expected {
			t.Errorf("Tools[%d] = %s, want %s", i, config.Tools[i], expected)
		}
	}

	// Check configs
	if len(config.Configs) != 4 {
		t.Errorf("Configs length = %d, want 4", len(config.Configs))
	}

	// First config (simple string)
	if config.Configs[0].Path != "~/.zshrc" {
		t.Errorf("Configs[0].Path = %s, want ~/.zshrc", config.Configs[0].Path)
	}

	// Third config (with recursive)
	if config.Configs[2].Path != "~/.config/nvim/" {
		t.Errorf("Configs[2].Path = %s, want ~/.config/nvim/", config.Configs[2].Path)
	}
	if !config.Configs[2].Recursive {
		t.Error("Configs[2].Recursive = false, want true")
	}

	// Fourth config (with all options)
	if config.Configs[3].Path != "~/.ssh/config" {
		t.Errorf("Configs[3].Path = %s, want ~/.ssh/config", config.Configs[3].Path)
	}
	if !config.Configs[3].Template {
		t.Error("Configs[3].Template = false, want true")
	}
	if !config.Configs[3].Secrets {
		t.Error("Configs[3].Secrets = false, want true")
	}
	if !config.Configs[3].Private {
		t.Error("Configs[3].Private = false, want true")
	}

	// Check git
	if config.Git.Remote != "https://github.com/user/dotfiles" {
		t.Errorf("Git.Remote = %s, want https://github.com/user/dotfiles", config.Git.Remote)
	}
	if config.Git.Branch != "main" {
		t.Errorf("Git.Branch = %s, want main", config.Git.Branch)
	}

	// Check options
	if config.Options.BackupRetention != 5 {
		t.Errorf("Options.BackupRetention = %d, want 5", config.Options.BackupRetention)
	}
}

func TestParser_ParseString_PlatformConditionals(t *testing.T) {
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
				platform.is_linux and "cargo:i3-msg" or nil,
				platform.is_macos and "yabai" or nil,
				platform.is_debian_family and "ubi:sharkdp/fd" or nil,
			},
		}
	`

	// Mock Linux Debian platform
	detector := &mockDetector{
		info: &platform.Info{
			OS:       "linux",
			Arch:     "amd64",
			ArchRaw:  "x86_64",
			Platform: "ubuntu",
			Family:   "debian",
			Version:  "22.04",
		},
	}

	parser := NewParser(detector)
	config, err := parser.ParseString(context.Background(), luaCode)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	// On Linux Debian, we should have node, i3-msg, and fd
	expectedTools := []string{"node@20.11.0", "cargo:i3-msg", "ubi:sharkdp/fd"}
	if len(config.Tools) != len(expectedTools) {
		t.Errorf("Tools length = %d, want %d", len(config.Tools), len(expectedTools))
	}

	for i, expected := range expectedTools {
		if i >= len(config.Tools) {
			break
		}
		if config.Tools[i] != expected {
			t.Errorf("Tools[%d] = %s, want %s", i, config.Tools[i], expected)
		}
	}
}

func TestParser_ParseString_Errors(t *testing.T) {
	tests := []struct {
		name    string
		luaCode string
		wantErr string
	}{
		{
			name:    "syntax error",
			luaCode: `zerb = { invalid syntax`,
			wantErr: "Lua syntax error",
		},
		{
			name:    "missing zerb table",
			luaCode: `config = { tools = {} }`,
			wantErr: "missing or invalid 'zerb' table",
		},
		{
			name: "empty tool string",
			luaCode: `
				zerb = {
					tools = { "" },
				}
			`,
			wantErr: "config validation failed",
		},
		{
			name: "config file without path",
			luaCode: `
				zerb = {
					configs = {
						{ recursive = true },
					},
				}
			`,
			wantErr: "path cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(nil)
			_, err := parser.ParseString(context.Background(), tt.luaCode)
			if err == nil {
				t.Fatal("ParseString() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseString() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestParser_ParseString_EmptyConfig(t *testing.T) {
	luaCode := `
		zerb = {
			tools = {},
			configs = {},
		}
	`

	parser := NewParser(nil)
	config, err := parser.ParseString(context.Background(), luaCode)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	if len(config.Tools) != 0 {
		t.Errorf("Tools length = %d, want 0", len(config.Tools))
	}
	if len(config.Configs) != 0 {
		t.Errorf("Configs length = %d, want 0", len(config.Configs))
	}
}

func TestParser_ParseString_HelperFunction(t *testing.T) {
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
				platform.when(platform.is_linux, "linux-tool"),
				platform.when(platform.is_macos, "macos-tool"),
			},
		}
	`

	// Mock Linux platform
	detector := &mockDetector{
		info: &platform.Info{
			OS:      "linux",
			Arch:    "amd64",
			ArchRaw: "x86_64",
		},
	}

	parser := NewParser(detector)
	config, err := parser.ParseString(context.Background(), luaCode)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	// On Linux, we should have node and linux-tool (not macos-tool)
	expectedTools := []string{"node@20.11.0", "linux-tool"}
	if len(config.Tools) != len(expectedTools) {
		t.Errorf("Tools length = %d, want %d", len(config.Tools), len(expectedTools))
	}

	for i, expected := range expectedTools {
		if i >= len(config.Tools) {
			break
		}
		if config.Tools[i] != expected {
			t.Errorf("Tools[%d] = %s, want %s", i, config.Tools[i], expected)
		}
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		verbose bool
		want    string
	}{
		{
			name: "parse error non-verbose",
			err: &ParseError{
				Message: "Lua syntax error",
				Detail:  "<string>:1: unexpected symbol near 'invalid'\nstack traceback:\n\t[G]: ?",
			},
			verbose: false,
			want:    "Lua syntax error",
		},
		{
			name: "parse error verbose",
			err: &ParseError{
				Message: "Lua syntax error",
				Detail:  "<string>:1: unexpected symbol near 'invalid'",
			},
			verbose: true,
			want:    "Lua syntax error\n\nDetails:\n<string>:1: unexpected symbol near 'invalid'",
		},
		{
			name:    "regular error",
			err:     &ValidationError{Field: "tools", Message: "invalid"},
			verbose: false,
			want:    "config validation failed for tools: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatError(tt.err, tt.verbose)
			if !strings.Contains(got, tt.want) {
				t.Errorf("FormatError() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestExtractTools_FiltersNils(t *testing.T) {
	luaCode := `
		return {
			"tool1",
			nil,
			"tool2",
			nil,
			"tool3",
		}
	`

	L := newSandboxedVM()
	defer L.Close()

	if err := L.DoString(luaCode); err != nil {
		t.Fatalf("Lua execution failed: %v", err)
	}

	table := L.Get(-1).(*lua.LTable)
	tools, err := extractTools(table)
	if err != nil {
		t.Fatalf("extractTools() error = %v", err)
	}

	expected := []string{"tool1", "tool2", "tool3"}
	if len(tools) != len(expected) {
		t.Errorf("extractTools() length = %d, want %d", len(tools), len(expected))
	}

	for i, exp := range expected {
		if i >= len(tools) {
			break
		}
		if tools[i] != exp {
			t.Errorf("extractTools()[%d] = %s, want %s", i, tools[i], exp)
		}
	}
}
