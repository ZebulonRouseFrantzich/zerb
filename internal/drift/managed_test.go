package drift

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestQueryManaged(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()

	// Create mock mise script that returns JSON for both commands
	miseScript := `#!/bin/sh
if [ "$1" = "ls" ] && [ "$2" = "--json" ]; then
    cat << 'EOF'
{
  "node": [
    {
      "version": "20.11.0",
      "install_path": "/home/user/.config/zerb/installs/node/20.11.0",
      "source": {
        "type": "mise.toml",
        "path": "/home/user/.config/zerb/mise/config.toml"
      }
    }
  ],
  "python": [
    {
      "version": "3.12.1",
      "install_path": "/home/user/.config/zerb/installs/python/3.12.1",
      "source": {
        "type": "mise.toml",
        "path": "/home/user/.config/zerb/mise/config.toml"
      }
    }
  ]
}
EOF
elif [ "$1" = "ls" ] && [ "$2" = "--current" ]; then
    cat << 'EOF'
node     20.11.0
python   3.12.1
EOF
fi
`

	// Create mock mise binary
	misePath := filepath.Join(tmpDir, "bin", "mise")
	os.MkdirAll(filepath.Dir(misePath), 0755)
	err := os.WriteFile(misePath, []byte(miseScript), 0755)
	if err != nil {
		t.Fatalf("failed to create mock mise: %v", err)
	}

	// Test QueryManaged
	tools, err := QueryManaged(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("QueryManaged(context.Background(), ) error = %v", err)
	}

	// Verify results
	want := []Tool{
		{Name: "node", Version: "20.11.0", Path: "/home/user/.config/zerb/installs/node/20.11.0"},
		{Name: "python", Version: "3.12.1", Path: "/home/user/.config/zerb/installs/python/3.12.1"},
	}

	if len(tools) != len(want) {
		t.Errorf("QueryManaged(context.Background(), ) returned %d tools, want %d", len(tools), len(want))
		t.Errorf("got: %+v", tools)
		t.Errorf("want: %+v", want)
		return
	}

	// Check that all wanted tools are present (order may vary)
	toolMap := make(map[string]Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	for _, wantTool := range want {
		gotTool, exists := toolMap[wantTool.Name]
		if !exists {
			t.Errorf("QueryManaged(context.Background(), ) missing tool %s", wantTool.Name)
			continue
		}
		if gotTool != wantTool {
			t.Errorf("QueryManaged(context.Background(), ) tool %s = %+v, want %+v", wantTool.Name, gotTool, wantTool)
		}
	}
}

func TestParseMiseJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    map[string][]MiseTool
		wantErr bool
	}{
		{
			name: "Multiple tools",
			json: `{
				"node": [
					{
						"version": "20.11.0",
						"install_path": "/home/.config/zerb/installs/node/20.11.0",
						"source": {
							"type": "mise.toml",
							"path": "/home/.config/zerb/mise/config.toml"
						}
					}
				],
				"python": [
					{
						"version": "3.12.1",
						"install_path": "/home/.config/zerb/installs/python/3.12.1",
						"source": {
							"type": "mise.toml",
							"path": "/home/.config/zerb/mise/config.toml"
						}
					}
				]
			}`,
			want: map[string][]MiseTool{
				"node": {
					{
						Version:     "20.11.0",
						InstallPath: "/home/.config/zerb/installs/node/20.11.0",
					},
				},
				"python": {
					{
						Version:     "3.12.1",
						InstallPath: "/home/.config/zerb/installs/python/3.12.1",
					},
				},
			},
		},
		{
			name: "Empty object",
			json: "{}",
			want: map[string][]MiseTool{},
		},
		{
			name:    "Invalid JSON",
			json:    "not json",
			wantErr: true,
		},
		{
			name:    "Malformed JSON",
			json:    `{"node": [{"version": "20.0.0"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMiseJSON(tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMiseJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("parseMiseJSON() returned %d tools, want %d", len(got), len(tt.want))
					return
				}

				for toolName, wantTools := range tt.want {
					gotTools, exists := got[toolName]
					if !exists {
						t.Errorf("parseMiseJSON() missing tool %s", toolName)
						continue
					}

					if len(gotTools) != len(wantTools) {
						t.Errorf("parseMiseJSON() tool %s has %d versions, want %d", toolName, len(gotTools), len(wantTools))
						continue
					}

					// Compare tool versions (ignore source field for simplicity)
					for i := range gotTools {
						if gotTools[i].Version != wantTools[i].Version || gotTools[i].InstallPath != wantTools[i].InstallPath {
							t.Errorf("parseMiseJSON() tool %s[%d] = {Version:%s, Path:%s}, want {Version:%s, Path:%s}",
								toolName, i, gotTools[i].Version, gotTools[i].InstallPath, wantTools[i].Version, wantTools[i].InstallPath)
						}
					}
				}
			}
		})
	}
}

func TestParseMiseCurrent(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "Multiple tools",
			output: `node     20.11.0
python   3.12.1
go       1.22.0`,
			want: map[string]string{
				"node":   "20.11.0",
				"python": "3.12.1",
				"go":     "1.22.0",
			},
		},
		{
			name:   "Single tool",
			output: "node     20.11.0",
			want: map[string]string{
				"node": "20.11.0",
			},
		},
		{
			name:   "Empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name:   "Whitespace variations",
			output: "node	20.11.0\npython    3.12.1",
			want: map[string]string{
				"node":   "20.11.0",
				"python": "3.12.1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMiseCurrent(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMiseCurrent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMiseCurrent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsZERBManaged(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		zerbDir string
		want    bool
	}{
		{
			name:    "ZERB installs path",
			path:    "/home/user/.config/zerb/installs/node/20.11.0/bin/node",
			zerbDir: "/home/user/.config/zerb",
			want:    true,
		},
		{
			name:    "System path",
			path:    "/usr/bin/node",
			zerbDir: "/home/user/.config/zerb",
			want:    false,
		},
		{
			name:    "User local path",
			path:    "/usr/local/bin/python",
			zerbDir: "/home/user/.config/zerb",
			want:    false,
		},
		{
			name:    "Homebrew path",
			path:    "/opt/homebrew/bin/python",
			zerbDir: "/home/user/.config/zerb",
			want:    false,
		},
		{
			name:    "NVM path",
			path:    "/home/user/.nvm/versions/node/v20.11.0/bin/node",
			zerbDir: "/home/user/.config/zerb",
			want:    false,
		},
		{
			name:    "Similar path but not ZERB",
			path:    "/home/user/.config/zerb-backup/installs/node/20.11.0/bin/node",
			zerbDir: "/home/user/.config/zerb",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsZERBManaged(tt.path, tt.zerbDir)
			if got != tt.want {
				t.Errorf("IsZERBManaged(%q, %q) = %v, want %v", tt.path, tt.zerbDir, got, tt.want)
			}
		})
	}
}

func TestQueryManaged_VersionMismatch(t *testing.T) {
	// Test case where ls --current shows a version not in ls --json
	tmpDir := t.TempDir()

	// Create mock mise script that returns JSON for ls --json but different version for ls --current
	miseScript := `#!/bin/sh
if [ "$1" = "ls" ] && [ "$2" = "--json" ]; then
    cat << 'EOF'
{
  "node": [
    {
      "version": "20.11.0",
      "install_path": "/home/user/.config/zerb/installs/node/20.11.0",
      "source": {
        "type": "mise.toml",
        "path": "/home/user/.config/zerb/mise/config.toml"
      }
    }
  ]
}
EOF
elif [ "$1" = "ls" ] && [ "$2" = "--current" ]; then
    cat << 'EOF'
node     20.15.0
python   3.12.1
EOF
fi
`

	// Create mock mise binary
	misePath := filepath.Join(tmpDir, "bin", "mise")
	os.MkdirAll(filepath.Dir(misePath), 0755)
	err := os.WriteFile(misePath, []byte(miseScript), 0755)
	if err != nil {
		t.Fatalf("failed to create mock mise: %v", err)
	}

	// Test QueryManaged
	tools, err := QueryManaged(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("QueryManaged(context.Background(), ) error = %v", err)
	}

	// Should skip node (version mismatch: 20.15.0 in current but only 20.11.0 in JSON)
	// Should skip python (not in JSON at all)
	// Result should be 1 tool with empty installPath or skipped entirely
	if len(tools) > 1 {
		t.Errorf("QueryManaged(context.Background(), ) returned %d tools, expected 0 or 1", len(tools))
	}
}

func TestValidateZerbDir(t *testing.T) {
	tests := []struct {
		name    string
		zerbDir string
		wantErr bool
	}{
		{
			name:    "Valid path",
			zerbDir: "/home/user/.config/zerb",
			wantErr: false,
		},
		{
			name:    "Path traversal - double dot",
			zerbDir: "/home/user/../../../etc",
			wantErr: true,
		},
		{
			name:    "Root directory",
			zerbDir: "/",
			wantErr: true,
		},
		{
			name:    "Relative path with traversal",
			zerbDir: "../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateZerbDir(tt.zerbDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateZerbDir(%q) error = %v, wantErr %v", tt.zerbDir, err, tt.wantErr)
			}
		})
	}
}

func TestGetMiseTimeout(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    string // duration as string for comparison
		wantDef bool   // expect default timeout
	}{
		{
			name:    "Default when no env var",
			envVal:  "",
			wantDef: true,
		},
		{
			name:   "Custom timeout from env",
			envVal: "30",
			want:   "30s",
		},
		{
			name:   "Large timeout from env",
			envVal: "300",
			want:   "5m0s",
		},
		{
			name:    "Invalid env var - use default",
			envVal:  "not-a-number",
			wantDef: true,
		},
		{
			name:    "Empty string - use default",
			envVal:  "",
			wantDef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env
			origVal := os.Getenv("ZERB_MISE_TIMEOUT")
			defer func() {
				if origVal != "" {
					os.Setenv("ZERB_MISE_TIMEOUT", origVal)
				} else {
					os.Unsetenv("ZERB_MISE_TIMEOUT")
				}
			}()

			// Set test env
			if tt.envVal != "" {
				os.Setenv("ZERB_MISE_TIMEOUT", tt.envVal)
			} else {
				os.Unsetenv("ZERB_MISE_TIMEOUT")
			}

			got := getMiseTimeout()

			if tt.wantDef {
				if got != defaultMiseTimeout {
					t.Errorf("getMiseTimeout() = %v, want default %v", got, defaultMiseTimeout)
				}
			} else {
				if got.String() != tt.want {
					t.Errorf("getMiseTimeout() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
