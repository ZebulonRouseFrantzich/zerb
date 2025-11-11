package config

import (
	"context"
	"strings"
	"testing"
)

func TestGenerator_Generate_Minimal(t *testing.T) {
	config := &Config{
		Tools: []string{"node@20.11.0"},
	}

	gen := NewGenerator()
	lua, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Check that it contains the expected elements
	if !strings.Contains(lua, "zerb = {") {
		t.Error("Generated Lua missing 'zerb = {'")
	}
	if !strings.Contains(lua, "tools = {") {
		t.Error("Generated Lua missing 'tools = {'")
	}
	if !strings.Contains(lua, `"node@20.11.0"`) {
		t.Error("Generated Lua missing tool")
	}
}

func TestGenerator_Generate_Full(t *testing.T) {
	config := &Config{
		Meta: Meta{
			Name:        "My Dev Environment",
			Description: "Full-stack development",
		},
		Tools: []string{
			"node@20.11.0",
			"python@3.12.1",
			"cargo:ripgrep",
		},
		Configs: []ConfigFile{
			{Path: "~/.zshrc"},
			{Path: "~/.gitconfig"},
			{Path: "~/.config/nvim/", Recursive: true},
			{Path: "~/.ssh/config", Template: true, Secrets: true, Private: true},
		},
		Git: GitConfig{
			Remote: "https://github.com/user/dotfiles",
			Branch: "main",
		},
		Options: Options{
			BackupRetention: 5,
		},
	}

	gen := NewGenerator()
	lua, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Check meta
	if !strings.Contains(lua, "meta = {") {
		t.Error("Generated Lua missing meta section")
	}
	if !strings.Contains(lua, `name = "My Dev Environment"`) {
		t.Error("Generated Lua missing meta.name")
	}

	// Check tools
	if !strings.Contains(lua, `"node@20.11.0"`) {
		t.Error("Generated Lua missing node tool")
	}
	if !strings.Contains(lua, `"python@3.12.1"`) {
		t.Error("Generated Lua missing python tool")
	}
	if !strings.Contains(lua, `"cargo:ripgrep"`) {
		t.Error("Generated Lua missing ripgrep tool")
	}

	// Check configs
	if !strings.Contains(lua, `"~/.zshrc"`) {
		t.Error("Generated Lua missing .zshrc config")
	}
	if !strings.Contains(lua, `recursive = true`) {
		t.Error("Generated Lua missing recursive option")
	}
	if !strings.Contains(lua, `template = true`) {
		t.Error("Generated Lua missing template option")
	}

	// Check git
	if !strings.Contains(lua, `git = {`) {
		t.Error("Generated Lua missing git section")
	}
	if !strings.Contains(lua, `remote = "https://github.com/user/dotfiles"`) {
		t.Error("Generated Lua missing git remote")
	}

	// Check options
	if !strings.Contains(lua, `backup_retention = 5`) {
		t.Error("Generated Lua missing backup_retention")
	}
}

func TestGenerator_GenerateTimestamped(t *testing.T) {
	config := &Config{
		Tools: []string{"node@20.11.0"},
	}

	gen := NewGenerator()
	filename, content, err := gen.GenerateTimestamped(config, "abc123")
	if err != nil {
		t.Fatalf("GenerateTimestamped() error = %v", err)
	}

	// Check filename format
	if !strings.HasPrefix(filename, "zerb.lua.") {
		t.Errorf("filename = %s, want prefix 'zerb.lua.'", filename)
	}
	if !strings.Contains(filename, "T") {
		t.Errorf("filename = %s, want ISO 8601 timestamp format", filename)
	}

	// Check content structure
	if !strings.Contains(content, "-- ZERB CONFIG - Timestamped Snapshot") {
		t.Error("Missing timestamped header")
	}
	if !strings.Contains(content, "local _metadata = {") {
		t.Error("Missing metadata section")
	}
	if !strings.Contains(content, `git_commit = "abc123"`) {
		t.Error("Missing git commit in metadata")
	}
	if !strings.Contains(content, "return zerb") {
		t.Error("Missing return statement")
	}
	if !strings.Contains(content, "-- ACTUAL CONFIG") {
		t.Error("Missing actual config marker")
	}
}

func TestGenerator_QuoteLuaString(t *testing.T) {
	gen := NewGenerator()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  `"hello"`,
		},
		{
			name:  "string with double quotes",
			input: `say "hello"`,
			want:  `"say \"hello\""`,
		},
		{
			name:  "string with backslashes",
			input: `C:\Users\test`,
			want:  `"C:\\Users\\test"`,
		},
		{
			name:  "string with newlines",
			input: "line1\nline2",
			want:  `"line1\nline2"`,
		},
		{
			name:  "string with tabs",
			input: "tab\there",
			want:  `"tab\there"`,
		},
		{
			name:  "empty string",
			input: "",
			want:  `""`,
		},
		{
			name:  "complex string",
			input: `path\to\"file"\nwith\ttabs`,
			want:  `"path\\to\\\"file\"\\nwith\\ttabs"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gen.quoteLuaString(tt.input)
			if got != tt.want {
				t.Errorf("quoteLuaString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerator_RoundTrip(t *testing.T) {
	// Create a config
	original := &Config{
		Meta: Meta{
			Name:        "Test Environment",
			Description: "Round-trip test",
		},
		Tools: []string{
			"node@20.11.0",
			"python@3.12.1",
		},
		Configs: []ConfigFile{
			{Path: "~/.zshrc"},
			{Path: "~/.config/nvim/", Recursive: true},
		},
		Git: GitConfig{
			Remote: "https://github.com/test/repo",
			Branch: "main",
		},
		Options: Options{
			BackupRetention: 3,
		},
	}

	// Generate Lua
	gen := NewGenerator()
	lua, err := gen.Generate(original)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Parse it back
	parser := NewParser(nil)
	parsed, err := parser.ParseString(context.Background(), lua)
	if err != nil {
		t.Fatalf("ParseString() error = %v\nGenerated Lua:\n%s", err, lua)
	}

	// Compare
	if parsed.Meta.Name != original.Meta.Name {
		t.Errorf("Meta.Name = %s, want %s", parsed.Meta.Name, original.Meta.Name)
	}
	if parsed.Meta.Description != original.Meta.Description {
		t.Errorf("Meta.Description = %s, want %s", parsed.Meta.Description, original.Meta.Description)
	}

	if len(parsed.Tools) != len(original.Tools) {
		t.Errorf("Tools length = %d, want %d", len(parsed.Tools), len(original.Tools))
	}
	for i := range original.Tools {
		if i >= len(parsed.Tools) {
			break
		}
		if parsed.Tools[i] != original.Tools[i] {
			t.Errorf("Tools[%d] = %s, want %s", i, parsed.Tools[i], original.Tools[i])
		}
	}

	if len(parsed.Configs) != len(original.Configs) {
		t.Errorf("Configs length = %d, want %d", len(parsed.Configs), len(original.Configs))
	}

	if parsed.Git.Remote != original.Git.Remote {
		t.Errorf("Git.Remote = %s, want %s", parsed.Git.Remote, original.Git.Remote)
	}
	if parsed.Git.Branch != original.Git.Branch {
		t.Errorf("Git.Branch = %s, want %s", parsed.Git.Branch, original.Git.Branch)
	}

	if parsed.Options.BackupRetention != original.Options.BackupRetention {
		t.Errorf("Options.BackupRetention = %d, want %d", parsed.Options.BackupRetention, original.Options.BackupRetention)
	}
}

func TestGenerator_EmptyConfig(t *testing.T) {
	config := &Config{
		Tools:   []string{},
		Configs: []ConfigFile{},
	}

	gen := NewGenerator()
	lua, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should still have valid Lua structure
	if !strings.Contains(lua, "zerb = {") {
		t.Error("Generated Lua missing 'zerb = {'")
	}

	// Should be parseable
	parser := NewParser(nil)
	_, err = parser.ParseString(context.Background(), lua)
	if err != nil {
		t.Errorf("ParseString() error = %v\nGenerated Lua:\n%s", err, lua)
	}
}

func TestGenerator_ConfigFileFormatting(t *testing.T) {
	tests := []struct {
		name   string
		config ConfigFile
		want   string
	}{
		{
			name:   "simple path",
			config: ConfigFile{Path: "~/.zshrc"},
			want:   `"~/.zshrc"`,
		},
		{
			name:   "with recursive",
			config: ConfigFile{Path: "~/.config/nvim/", Recursive: true},
			want:   "recursive = true",
		},
		{
			name:   "with all options",
			config: ConfigFile{Path: "~/.ssh/config", Template: true, Secrets: true, Private: true},
			want:   "template = true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Configs: []ConfigFile{tt.config},
			}

			gen := NewGenerator()
			lua, err := gen.Generate(config)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			if !strings.Contains(lua, tt.want) {
				t.Errorf("Generated Lua missing expected content %q\nGenerated:\n%s", tt.want, lua)
			}
		})
	}
}

func TestGenerator_SpecialCharacters(t *testing.T) {
	config := &Config{
		Meta: Meta{
			Name: `Test "with" quotes`,
		},
		Tools: []string{
			`tool@1.0.0`,
			`path\to\tool`,
		},
		Configs: []ConfigFile{
			{Path: `C:\Users\test\.config`},
		},
	}

	gen := NewGenerator()
	lua, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should escape quotes
	if !strings.Contains(lua, `\"with\"`) {
		t.Error("Quotes not properly escaped")
	}

	// Should escape backslashes
	if !strings.Contains(lua, `\\`) {
		t.Error("Backslashes not properly escaped")
	}

	// Should be parseable
	parser := NewParser(nil)
	parsed, err := parser.ParseString(context.Background(), lua)
	if err != nil {
		t.Errorf("ParseString() error = %v\nGenerated Lua:\n%s", err, lua)
	}

	// Should preserve original values
	if parsed.Meta.Name != config.Meta.Name {
		t.Errorf("Meta.Name = %q, want %q", parsed.Meta.Name, config.Meta.Name)
	}
}
