package drift

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestQueryBaseline(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		want    []ToolSpec
		wantErr bool
	}{
		{
			name: "Simple tools",
			config: `zerb = {
				tools = {
					"node@20.11.0",
					"python@3.12.1",
				}
			}`,
			want: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
				{Name: "python", Version: "3.12.1"},
			},
		},
		{
			name: "Tools with backends",
			config: `zerb = {
				tools = {
					"cargo:ripgrep@13.0.0",
					"ubi:sharkdp/bat@0.24.0",
				}
			}`,
			want: []ToolSpec{
				{Backend: "cargo", Name: "ripgrep", Version: "13.0.0"},
				{Backend: "ubi", Name: "bat", Version: "0.24.0"},
			},
		},
		{
			name: "Mixed tools and backends",
			config: `zerb = {
				tools = {
					"node@20.11.0",
					"npm:prettier@3.0.0",
					"go@1.22.0",
				}
			}`,
			want: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
				{Backend: "npm", Name: "prettier", Version: "3.0.0"},
				{Name: "go", Version: "1.22.0"},
			},
		},
		{
			name:   "Empty tools list",
			config: `zerb = { tools = {} }`,
			want:   []ToolSpec{},
		},
		{
			name:    "Invalid Lua syntax",
			config:  "invalid lua syntax {{{",
			wantErr: true,
		},
		{
			name:   "No tools field",
			config: `zerb = { configs = {} }`,
			want:   []ToolSpec{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "zerb.lua")
			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			if err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			// Test QueryBaseline
			got, err := QueryBaseline(context.Background(), configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("QueryBaseline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("QueryBaseline() returned %d tools, want %d", len(got), len(tt.want))
					t.Errorf("got: %+v", got)
					t.Errorf("want: %+v", tt.want)
					return
				}

				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("QueryBaseline()[%d] = %+v, want %+v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestQueryBaseline_FileErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "Nonexistent file",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/zerb.lua"
			},
			wantErr: true,
		},
		{
			name: "Directory instead of file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := tt.setup(t)
			_, err := QueryBaseline(context.Background(), configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("QueryBaseline() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
