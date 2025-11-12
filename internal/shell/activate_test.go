package shell

import (
	"strings"
	"testing"
)

func TestGenerateActivationCommand(t *testing.T) {
	tests := []struct {
		name    string
		shell   ShellType
		want    string
		wantErr bool
	}{
		{
			name:    "Bash activation",
			shell:   ShellBash,
			want:    `eval "$(zerb activate bash)"`,
			wantErr: false,
		},
		{
			name:    "Zsh activation",
			shell:   ShellZsh,
			want:    `eval "$(zerb activate zsh)"`,
			wantErr: false,
		},
		{
			name:    "Fish activation",
			shell:   ShellFish,
			want:    "zerb activate fish | source",
			wantErr: false,
		},
		{
			name:    "Unknown shell",
			shell:   ShellUnknown,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateActivationCommand(tt.shell)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateActivationCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GenerateActivationCommand() = %v, want %v", got, tt.want)
			}

			// Verify command contains activation marker (not "mise activate")
			if !tt.wantErr && !strings.Contains(got, ActivationMarker) {
				t.Errorf("GenerateActivationCommand() should contain '%s', got: %v", ActivationMarker, got)
			}

			// Verify command does NOT contain "mise" (abstraction test)
			if !tt.wantErr && strings.Contains(got, "mise") {
				t.Errorf("GenerateActivationCommand() should NOT contain 'mise' (abstraction violation), got: %v", got)
			}
		})
	}
}

func TestGetMiseActivationCommand(t *testing.T) {
	misePath := "/home/user/.config/zerb/bin/mise"

	tests := []struct {
		name    string
		shell   ShellType
		want    []string
		wantErr bool
	}{
		{
			name:    "Bash mise command",
			shell:   ShellBash,
			want:    []string{misePath, "activate", "bash"},
			wantErr: false,
		},
		{
			name:    "Zsh mise command",
			shell:   ShellZsh,
			want:    []string{misePath, "activate", "zsh"},
			wantErr: false,
		},
		{
			name:    "Fish mise command",
			shell:   ShellFish,
			want:    []string{misePath, "activate", "fish"},
			wantErr: false,
		},
		{
			name:    "Unknown shell",
			shell:   ShellUnknown,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMiseActivationCommand(tt.shell, misePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMiseActivationCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check length
			if len(got) != len(tt.want) {
				t.Errorf("GetMiseActivationCommand() length = %d, want %d", len(got), len(tt.want))
				return
			}

			// Check each element
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetMiseActivationCommand()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}

			// Verify first element is the mise binary path
			if got[0] != misePath {
				t.Errorf("GetMiseActivationCommand()[0] should be mise path %q, got %q", misePath, got[0])
			}

			// Verify command structure
			if len(got) >= 2 && got[1] != "activate" {
				t.Errorf("GetMiseActivationCommand()[1] should be 'activate', got %q", got[1])
			}
		})
	}
}

func TestActivationCommandAbstraction(t *testing.T) {
	// This test ensures our abstraction layer is working correctly
	// Users should ONLY see "zerb activate", never "mise activate"

	shells := []ShellType{ShellBash, ShellZsh, ShellFish}

	for _, shell := range shells {
		t.Run(shell.String(), func(t *testing.T) {
			// Get user-facing command
			userCmd, err := GenerateActivationCommand(shell)
			if err != nil {
				t.Fatalf("GenerateActivationCommand() error = %v", err)
			}

			// Verify abstraction
			if !strings.Contains(userCmd, ActivationMarker) {
				t.Errorf("User-facing command missing '%s': %q", ActivationMarker, userCmd)
			}

			if strings.Contains(userCmd, "mise") {
				t.Errorf("User-facing command exposes 'mise' (abstraction violation): %q", userCmd)
			}

			// Get internal command
			misePath := "/test/mise"
			internalCmd, err := GetMiseActivationCommand(shell, misePath)
			if err != nil {
				t.Fatalf("GetMiseActivationCommand() error = %v", err)
			}

			// Verify internal command uses mise
			if len(internalCmd) == 0 || !strings.Contains(internalCmd[0], "mise") {
				t.Errorf("Internal command should use mise binary: %v", internalCmd)
			}

			if len(internalCmd) < 2 || internalCmd[1] != "activate" {
				t.Errorf("Internal command should call 'activate': %v", internalCmd)
			}
		})
	}
}
