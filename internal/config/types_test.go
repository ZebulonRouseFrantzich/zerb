package config

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			config: &Config{
				Tools: []string{"node@20.11.0"},
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: &Config{
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
					{Path: "~/.config/nvim/", Recursive: true},
				},
				Git: GitConfig{
					Remote: "https://github.com/user/dotfiles",
					Branch: "main",
				},
				Options: Options{
					BackupRetention: 5,
				},
			},
			wantErr: false,
		},
		{
			name: "empty tool string",
			config: &Config{
				Tools: []string{""},
			},
			wantErr: true,
			errMsg:  "tool string cannot be empty",
		},
		{
			name: "empty config path",
			config: &Config{
				Configs: []ConfigFile{
					{Path: ""},
				},
			},
			wantErr: true,
			errMsg:  "path cannot be empty",
		},
		{
			name: "empty config",
			config: &Config{
				Tools:   []string{},
				Configs: []ConfigFile{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if err.Error() == "" || len(err.Error()) < len(tt.errMsg) {
					t.Errorf("Config.Validate() error = %v, want substring %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateToolString(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		wantErr bool
	}{
		{
			name:    "valid tool with version",
			tool:    "node@20.11.0",
			wantErr: false,
		},
		{
			name:    "valid cargo tool",
			tool:    "cargo:ripgrep",
			wantErr: false,
		},
		{
			name:    "valid npm tool",
			tool:    "npm:prettier",
			wantErr: false,
		},
		{
			name:    "valid ubi tool",
			tool:    "ubi:sharkdp/bat",
			wantErr: false,
		},
		{
			name:    "empty tool string",
			tool:    "",
			wantErr: true,
		},
		{
			name:    "tool without version (allowed for MVP)",
			tool:    "node",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToolString(tt.tool)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToolString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMetadata(t *testing.T) {
	now := time.Now()
	metadata := Metadata{
		Version:   1,
		Timestamp: now,
		GitCommit: "abc123",
	}

	if metadata.Version != 1 {
		t.Errorf("Metadata.Version = %d, want 1", metadata.Version)
	}
	if metadata.Timestamp != now {
		t.Errorf("Metadata.Timestamp = %v, want %v", metadata.Timestamp, now)
	}
	if metadata.GitCommit != "abc123" {
		t.Errorf("Metadata.GitCommit = %s, want abc123", metadata.GitCommit)
	}
}

func TestConfigVersion(t *testing.T) {
	now := time.Now()
	cv := ConfigVersion{
		Timestamp: now,
		Filename:  "zerb.20250115T143022Z.lua",
		IsActive:  true,
	}

	if cv.Timestamp != now {
		t.Errorf("ConfigVersion.Timestamp = %v, want %v", cv.Timestamp, now)
	}
	if cv.Filename != "zerb.20250115T143022Z.lua" {
		t.Errorf("ConfigVersion.Filename = %s, want zerb.20250115T143022Z.lua", cv.Filename)
	}
	if !cv.IsActive {
		t.Error("ConfigVersion.IsActive = false, want true")
	}
}

func TestOverride(t *testing.T) {
	override := Override{
		ToolsAdd:    []string{"kubectl@1.28.0"},
		ToolsRemove: []string{"rust"},
		ToolsOverride: map[string]string{
			"node": "21.0.0",
		},
		ConfigOverrides: map[string]interface{}{
			"gitconfig": map[string]interface{}{
				"user": map[string]interface{}{
					"email": "work@company.com",
				},
			},
		},
	}

	if len(override.ToolsAdd) != 1 {
		t.Errorf("Override.ToolsAdd length = %d, want 1", len(override.ToolsAdd))
	}
	if override.ToolsAdd[0] != "kubectl@1.28.0" {
		t.Errorf("Override.ToolsAdd[0] = %s, want kubectl@1.28.0", override.ToolsAdd[0])
	}
	if len(override.ToolsRemove) != 1 {
		t.Errorf("Override.ToolsRemove length = %d, want 1", len(override.ToolsRemove))
	}
	if override.ToolsRemove[0] != "rust" {
		t.Errorf("Override.ToolsRemove[0] = %s, want rust", override.ToolsRemove[0])
	}
	if override.ToolsOverride["node"] != "21.0.0" {
		t.Errorf("Override.ToolsOverride[node] = %s, want 21.0.0", override.ToolsOverride["node"])
	}
}

func TestConfig_FindConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	config := &Config{
		Configs: []ConfigFile{
			{Path: "~/.zshrc", Template: true},
			{Path: "~/.gitconfig", Private: true},
			{Path: "~/.config/nvim/", Recursive: true},
		},
	}

	tests := []struct {
		name       string
		searchPath string
		wantFound  bool
		wantPath   string
	}{
		{
			name:       "find exact match with tilde",
			searchPath: "~/.zshrc",
			wantFound:  true,
			wantPath:   "~/.zshrc",
		},
		{
			name:       "find with absolute path when stored as tilde",
			searchPath: homeDir + "/.zshrc",
			wantFound:  true,
			wantPath:   "~/.zshrc",
		},
		{
			name:       "not found",
			searchPath: "~/.bashrc",
			wantFound:  false,
		},
		{
			name:       "find directory path",
			searchPath: "~/.config/nvim/",
			wantFound:  true,
			wantPath:   "~/.config/nvim/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := config.FindConfig(tt.searchPath)
			if tt.wantFound {
				if found == nil {
					t.Errorf("FindConfig(%q) = nil, want non-nil", tt.searchPath)
					return
				}
				if found.Path != tt.wantPath {
					t.Errorf("FindConfig(%q).Path = %q, want %q", tt.searchPath, found.Path, tt.wantPath)
				}
			} else {
				if found != nil {
					t.Errorf("FindConfig(%q) = %+v, want nil", tt.searchPath, found)
				}
			}
		})
	}
}

func TestConfig_RemoveConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tests := []struct {
		name        string
		configs     []ConfigFile
		removePath  string
		wantCount   int
		wantRemoved bool
	}{
		{
			name: "remove existing config",
			configs: []ConfigFile{
				{Path: "~/.zshrc"},
				{Path: "~/.gitconfig"},
				{Path: "~/.tmux.conf"},
			},
			removePath:  "~/.gitconfig",
			wantCount:   2,
			wantRemoved: true,
		},
		{
			name: "remove with absolute path",
			configs: []ConfigFile{
				{Path: "~/.zshrc"},
				{Path: "~/.gitconfig"},
			},
			removePath:  homeDir + "/.zshrc",
			wantCount:   1,
			wantRemoved: true,
		},
		{
			name: "remove non-existent path",
			configs: []ConfigFile{
				{Path: "~/.zshrc"},
			},
			removePath:  "~/.bashrc",
			wantCount:   1,
			wantRemoved: false,
		},
		{
			name: "remove last config",
			configs: []ConfigFile{
				{Path: "~/.zshrc"},
			},
			removePath:  "~/.zshrc",
			wantCount:   0,
			wantRemoved: true,
		},
		{
			name:        "remove from empty configs",
			configs:     []ConfigFile{},
			removePath:  "~/.zshrc",
			wantCount:   0,
			wantRemoved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{Configs: tt.configs}
			newConfigs, removed := config.RemoveConfig(tt.removePath)

			if removed != tt.wantRemoved {
				t.Errorf("RemoveConfig(%q) removed = %v, want %v", tt.removePath, removed, tt.wantRemoved)
			}
			if len(newConfigs) != tt.wantCount {
				t.Errorf("RemoveConfig(%q) returned %d configs, want %d", tt.removePath, len(newConfigs), tt.wantCount)
			}

			// Verify original config is not modified
			if len(config.Configs) != len(tt.configs) {
				t.Error("Original config should not be modified")
			}
		})
	}
}

func TestDeduplicatePaths(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tests := []struct {
		name      string
		paths     []string
		wantCount int
	}{
		{
			name:      "no duplicates",
			paths:     []string{"~/.zshrc", "~/.gitconfig"},
			wantCount: 2,
		},
		{
			name:      "exact duplicates",
			paths:     []string{"~/.zshrc", "~/.zshrc"},
			wantCount: 1,
		},
		{
			name:      "tilde and absolute duplicates",
			paths:     []string{"~/.zshrc", homeDir + "/.zshrc"},
			wantCount: 1,
		},
		{
			name:      "preserves order (first occurrence kept)",
			paths:     []string{"~/.gitconfig", "~/.zshrc", homeDir + "/.gitconfig"},
			wantCount: 2,
		},
		{
			name:      "empty input",
			paths:     []string{},
			wantCount: 0,
		},
		{
			name:      "single path",
			paths:     []string{"~/.zshrc"},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeduplicatePaths(tt.paths)
			if len(result) != tt.wantCount {
				t.Errorf("DeduplicatePaths() returned %d paths, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestIsWithinHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "tilde path",
			path: "~/.zshrc",
			want: true,
		},
		{
			name: "absolute path within home",
			path: homeDir + "/.config/nvim",
			want: true,
		},
		{
			name: "exact home directory",
			path: homeDir,
			want: true,
		},
		{
			name: "tilde home",
			path: "~",
			want: true,
		},
		{
			name: "path outside home",
			path: "/etc/passwd",
			want: false,
		},
		{
			name: "root path",
			path: "/",
			want: false,
		},
		{
			name: "tmp path",
			path: "/tmp/test",
			want: false,
		},
		{
			name: "path that starts with home prefix but different dir",
			path: homeDir + "_other/file",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinHome(tt.path)
			if result != tt.want {
				t.Errorf("IsWithinHome(%q) = %v, want %v", tt.path, result, tt.want)
			}
		})
	}
}
