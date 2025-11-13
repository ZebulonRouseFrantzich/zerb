package shell

import (
	"os"
	"testing"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name           string
		shellEnv       string
		wantShell      ShellType
		wantMethod     string
		wantConfidence string
	}{
		{
			name:           "Bash from SHELL",
			shellEnv:       "/bin/bash",
			wantShell:      ShellBash,
			wantMethod:     "$SHELL environment variable",
			wantConfidence: "high",
		},
		{
			name:           "Zsh from SHELL",
			shellEnv:       "/usr/bin/zsh",
			wantShell:      ShellZsh,
			wantMethod:     "$SHELL environment variable",
			wantConfidence: "high",
		},
		{
			name:           "Fish from SHELL",
			shellEnv:       "/usr/local/bin/fish",
			wantShell:      ShellFish,
			wantMethod:     "$SHELL environment variable",
			wantConfidence: "high",
		},
		{
			name:           "Unknown shell from SHELL",
			shellEnv:       "/bin/ksh",
			wantShell:      ShellUnknown,
			wantMethod:     "detection failed",
			wantConfidence: "none",
		},
		{
			name:           "Empty SHELL variable",
			shellEnv:       "",
			wantShell:      ShellUnknown,
			wantMethod:     "detection failed",
			wantConfidence: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			oldShell := os.Getenv("SHELL")
			defer os.Setenv("SHELL", oldShell)

			if tt.shellEnv != "" {
				os.Setenv("SHELL", tt.shellEnv)
			} else {
				os.Unsetenv("SHELL")
			}

			// Detect shell
			result, err := DetectShell()
			if err != nil {
				t.Fatalf("DetectShell() error = %v", err)
			}

			// Check shell type
			if result.Shell != tt.wantShell {
				t.Errorf("DetectShell() shell = %v, want %v", result.Shell, tt.wantShell)
			}

			// Check method
			if result.Method != tt.wantMethod {
				t.Errorf("DetectShell() method = %v, want %v", result.Method, tt.wantMethod)
			}

			// Check confidence
			if result.Confidence != tt.wantConfidence {
				t.Errorf("DetectShell() confidence = %v, want %v", result.Confidence, tt.wantConfidence)
			}
		})
	}
}

func TestParseShellFromPath(t *testing.T) {
	tests := []struct {
		name      string
		shellPath string
		want      ShellType
	}{
		{
			name:      "Bash - /bin/bash",
			shellPath: "/bin/bash",
			want:      ShellBash,
		},
		{
			name:      "Bash - /usr/bin/bash",
			shellPath: "/usr/bin/bash",
			want:      ShellBash,
		},
		{
			name:      "Zsh - /bin/zsh",
			shellPath: "/bin/zsh",
			want:      ShellZsh,
		},
		{
			name:      "Zsh - /usr/local/bin/zsh",
			shellPath: "/usr/local/bin/zsh",
			want:      ShellZsh,
		},
		{
			name:      "Fish - /usr/bin/fish",
			shellPath: "/usr/bin/fish",
			want:      ShellFish,
		},
		{
			name:      "Fish - /usr/local/bin/fish",
			shellPath: "/usr/local/bin/fish",
			want:      ShellFish,
		},
		{
			name:      "Unknown - /bin/ksh",
			shellPath: "/bin/ksh",
			want:      ShellUnknown,
		},
		{
			name:      "Unknown - /bin/csh",
			shellPath: "/bin/csh",
			want:      ShellUnknown,
		},
		{
			name:      "Unknown - /bin/tcsh",
			shellPath: "/bin/tcsh",
			want:      ShellUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseShellFromPath(tt.shellPath)
			if got != tt.want {
				t.Errorf("parseShellFromPath(%q) = %v, want %v", tt.shellPath, got, tt.want)
			}
		})
	}
}

func TestValidateShell(t *testing.T) {
	tests := []struct {
		name    string
		shell   ShellType
		wantErr bool
	}{
		{
			name:    "Valid - bash",
			shell:   ShellBash,
			wantErr: false,
		},
		{
			name:    "Valid - zsh",
			shell:   ShellZsh,
			wantErr: false,
		},
		{
			name:    "Valid - fish",
			shell:   ShellFish,
			wantErr: false,
		},
		{
			name:    "Invalid - unknown",
			shell:   ShellUnknown,
			wantErr: true,
		},
		{
			name:    "Invalid - custom",
			shell:   ShellType("ksh"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateShell(tt.shell)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateShell(%v) error = %v, wantErr %v", tt.shell, err, tt.wantErr)
			}

			// Check error type for invalid shells
			if tt.wantErr && err != nil {
				if _, ok := err.(*UnsupportedShellError); !ok {
					t.Errorf("ValidateShell(%v) error type = %T, want *UnsupportedShellError", tt.shell, err)
				}
			}
		})
	}
}

func TestGetSupportedShells(t *testing.T) {
	shells := GetSupportedShells()

	// Should return exactly 3 shells
	if len(shells) != 3 {
		t.Errorf("GetSupportedShells() returned %d shells, want 3", len(shells))
	}

	// Check that all expected shells are present
	expected := map[ShellType]bool{
		ShellBash: false,
		ShellZsh:  false,
		ShellFish: false,
	}

	for _, shell := range shells {
		if _, ok := expected[shell]; ok {
			expected[shell] = true
		} else {
			t.Errorf("GetSupportedShells() returned unexpected shell: %v", shell)
		}
	}

	// Check that all expected shells were found
	for shell, found := range expected {
		if !found {
			t.Errorf("GetSupportedShells() missing expected shell: %v", shell)
		}
	}
}

func TestShellType_String(t *testing.T) {
	tests := []struct {
		shell ShellType
		want  string
	}{
		{ShellBash, "bash"},
		{ShellZsh, "zsh"},
		{ShellFish, "fish"},
		{ShellUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.shell.String(); got != tt.want {
				t.Errorf("ShellType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShellType_IsValid(t *testing.T) {
	tests := []struct {
		shell ShellType
		want  bool
	}{
		{ShellBash, true},
		{ShellZsh, true},
		{ShellFish, true},
		{ShellUnknown, false},
		{ShellType("ksh"), false},
		{ShellType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			if got := tt.shell.IsValid(); got != tt.want {
				t.Errorf("ShellType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
