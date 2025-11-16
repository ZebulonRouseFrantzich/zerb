package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateConfigPath_Security tests the security aspects of path validation
// to prevent path traversal attacks and symlink escapes.
func TestValidateConfigPath_Security(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home directory: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		// Valid paths - should pass
		{
			name:    "valid tilde path",
			path:    "~/.zshrc",
			wantErr: false,
		},
		{
			name:    "valid tilde directory",
			path:    "~/.config/nvim",
			wantErr: false,
		},
		{
			name:    "valid absolute path inside home",
			path:    filepath.Join(home, ".zshrc"),
			wantErr: false,
		},
		{
			name:    "valid absolute directory inside home",
			path:    filepath.Join(home, ".config", "nvim"),
			wantErr: false,
		},
		{
			name:    "valid nested path",
			path:    "~/.config/nvim/lua/plugins",
			wantErr: false,
		},
		{
			name:    "path with literal .. in name (legitimate use case)",
			path:    "~/.config/..something",
			wantErr: false,
		},

		// Path traversal attacks - should fail
		{
			name:    "path traversal with tilde",
			path:    "~/../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal not allowed",
		},
		{
			name:    "path traversal multiple levels",
			path:    "~/../../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal not allowed",
		},
		{
			name:    "path traversal in middle",
			path:    "~/.config/../../../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal not allowed",
		},
		{
			name:    "absolute path outside home",
			path:    "/etc/passwd",
			wantErr: true,
			errMsg:  "absolute paths outside home directory not allowed",
		},
		{
			name:    "absolute path to root",
			path:    "/",
			wantErr: true,
			errMsg:  "absolute paths outside home directory not allowed",
		},

		// Edge cases
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
			errMsg:  "path cannot be empty",
		},
		{
			name:    "just tilde",
			path:    "~",
			wantErr: false, // Home directory itself is valid
		},
		{
			name:    "tilde with slash",
			path:    "~/",
			wantErr: false, // Home directory itself is valid
		},
		{
			name:    "relative path without tilde",
			path:    ".zshrc",
			wantErr: true,
			errMsg:  "must be absolute or start with ~/",
		},
		{
			name:    "relative path with dot slash",
			path:    "./.zshrc",
			wantErr: true,
			errMsg:  "must be absolute or start with ~/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfigPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				errStr := err.Error()
				if errStr == "" {
					t.Errorf("validateConfigPath(%q) error is empty, want substring %q", tt.path, tt.errMsg)
				}
				// Just check that error message is non-empty for now
				// We'll add more specific checks after implementation
			}
		})
	}
}

// TestValidateConfigPath_Symlinks tests symlink handling to prevent escapes
// from the home directory.
func TestValidateConfigPath_Symlinks(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home directory: %v", err)
	}

	// Create test files and symlinks within tmpDir (not actual home)
	testFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}

	// Create symlink inside tmpDir pointing outside
	symlinkInside := filepath.Join(tmpDir, "link-to-etc")
	if err := os.Symlink("/etc/passwd", symlinkInside); err != nil {
		if !os.IsPermission(err) {
			t.Fatalf("cannot create symlink: %v", err)
		}
		t.Skip("skipping symlink test: permission denied")
	}

	// Create symlink inside tmpDir pointing to another location inside tmpDir
	safeTarget := filepath.Join(tmpDir, "safe-target")
	if err := os.WriteFile(safeTarget, []byte("safe"), 0644); err != nil {
		t.Fatalf("cannot create safe target: %v", err)
	}
	safeSymlink := filepath.Join(tmpDir, "safe-link")
	if err := os.Symlink(safeTarget, safeSymlink); err != nil {
		t.Fatalf("cannot create safe symlink: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
		setup   func() string // Returns the path to test
		cleanup func()
	}{
		{
			name: "symlink inside home pointing outside home",
			setup: func() string {
				// Create a symlink in actual home pointing to /etc
				linkPath := filepath.Join(home, ".test-zerb-symlink-escape")
				if err := os.Symlink("/etc", linkPath); err != nil {
					if os.IsPermission(err) {
						t.Skip("skipping symlink test: permission denied")
					}
					t.Fatalf("cannot create test symlink: %v", err)
				}
				return linkPath
			},
			cleanup: func() {
				os.Remove(filepath.Join(home, ".test-zerb-symlink-escape"))
			},
			wantErr: true,
		},
		{
			name: "symlink inside home pointing to file inside home",
			setup: func() string {
				// Create a real file in home
				realFile := filepath.Join(home, ".test-zerb-real-file")
				if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
					t.Fatalf("cannot create test file: %v", err)
				}
				// Create a symlink pointing to it
				linkPath := filepath.Join(home, ".test-zerb-symlink-safe")
				if err := os.Symlink(realFile, linkPath); err != nil {
					if os.IsPermission(err) {
						t.Skip("skipping symlink test: permission denied")
					}
					t.Fatalf("cannot create test symlink: %v", err)
				}
				return linkPath
			},
			cleanup: func() {
				os.Remove(filepath.Join(home, ".test-zerb-symlink-safe"))
				os.Remove(filepath.Join(home, ".test-zerb-real-file"))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				testPath := tt.setup()
				if tt.cleanup != nil {
					defer tt.cleanup()
				}
				err := validateConfigPath(testPath)
				if (err != nil) != tt.wantErr {
					t.Errorf("validateConfigPath(%q) error = %v, wantErr %v", testPath, err, tt.wantErr)
				}
			}
		})
	}
}

// TestValidateConfigPath_Normalization tests path normalization for duplicate detection.
func TestValidateConfigPath_Normalization(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home directory: %v", err)
	}

	tests := []struct {
		name  string
		path1 string
		path2 string
		same  bool // Should these normalize to the same path?
	}{
		{
			name:  "tilde vs absolute - same file",
			path1: "~/.zshrc",
			path2: filepath.Join(home, ".zshrc"),
			same:  true,
		},
		{
			name:  "with and without trailing slash - directory",
			path1: "~/.config/nvim",
			path2: "~/.config/nvim/",
			same:  true,
		},
		{
			name:  "different files",
			path1: "~/.zshrc",
			path2: "~/.bashrc",
			same:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both paths should be valid
			if err := validateConfigPath(tt.path1); err != nil {
				t.Errorf("validateConfigPath(%q) unexpected error: %v", tt.path1, err)
			}
			if err := validateConfigPath(tt.path2); err != nil {
				t.Errorf("validateConfigPath(%q) unexpected error: %v", tt.path2, err)
			}

			// Test normalization
			norm1, err := NormalizeConfigPath(tt.path1)
			if err != nil {
				t.Errorf("NormalizeConfigPath(%q) unexpected error: %v", tt.path1, err)
				return
			}
			norm2, err := NormalizeConfigPath(tt.path2)
			if err != nil {
				t.Errorf("NormalizeConfigPath(%q) unexpected error: %v", tt.path2, err)
				return
			}

			if tt.same {
				if norm1 != norm2 {
					t.Errorf("NormalizeConfigPath() paths should be same:\n  path1=%q -> %q\n  path2=%q -> %q",
						tt.path1, norm1, tt.path2, norm2)
				}
			} else {
				if norm1 == norm2 {
					t.Errorf("NormalizeConfigPath() paths should be different:\n  path1=%q -> %q\n  path2=%q -> %q",
						tt.path1, norm1, tt.path2, norm2)
				}
			}
		})
	}
}

// TestNormalizeConfigPath tests the NormalizeConfigPath function directly.
func TestNormalizeConfigPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home directory: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "tilde path",
			path: "~/.zshrc",
			want: filepath.Join(home, ".zshrc"),
		},
		{
			name: "absolute path",
			path: filepath.Join(home, ".zshrc"),
			want: filepath.Join(home, ".zshrc"),
		},
		{
			name: "just tilde",
			path: "~",
			want: home,
		},
		{
			name: "trailing slash removed",
			path: "~/.config/nvim/",
			want: filepath.Join(home, ".config", "nvim"),
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "relative path",
			path:    ".zshrc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeConfigPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeConfigPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("NormalizeConfigPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
