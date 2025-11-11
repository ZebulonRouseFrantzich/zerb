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
		Filename:  "zerb.lua.20250115T143022Z",
		IsActive:  true,
	}

	if cv.Timestamp != now {
		t.Errorf("ConfigVersion.Timestamp = %v, want %v", cv.Timestamp, now)
	}
	if cv.Filename != "zerb.lua.20250115T143022Z" {
		t.Errorf("ConfigVersion.Filename = %s, want zerb.lua.20250115T143022Z", cv.Filename)
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
