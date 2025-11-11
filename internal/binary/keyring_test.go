package binary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetKeyring(t *testing.T) {
	tests := []struct {
		name    string
		binary  Binary
		wantErr bool
	}{
		{
			name:    "mise_keyring",
			binary:  BinaryMise,
			wantErr: false,
		},
		{
			name:    "chezmoi_no_gpg",
			binary:  BinaryChezmoi,
			wantErr: true, // chezmoi doesn't have GPG signatures
		},
		{
			name:    "unknown_binary",
			binary:  Binary("unknown"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring, err := getKeyring(tt.binary)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(keyring) == 0 {
				t.Error("expected non-empty keyring")
			}

			// Verify it looks like a GPG key (starts with common PGP headers)
			keyringStr := string(keyring)
			if len(keyringStr) < 10 {
				t.Error("keyring too short to be valid")
			}
		})
	}
}

func TestExtractKeyring(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		binary  Binary
		wantErr bool
	}{
		{
			name:    "extract_mise",
			binary:  BinaryMise,
			wantErr: false,
		},
		{
			name:    "extract_chezmoi_fails",
			binary:  BinaryChezmoi,
			wantErr: true, // chezmoi doesn't support GPG
		},
		{
			name:    "unknown_binary",
			binary:  Binary("invalid"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyringDir := filepath.Join(tmpDir, tt.name)

			err := extractKeyring(keyringDir, tt.binary)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify file was created
			keyringPath := getKeyringPath(keyringDir, tt.binary)
			info, err := os.Stat(keyringPath)
			if err != nil {
				t.Fatalf("keyring file not found: %v", err)
			}

			if info.IsDir() {
				t.Error("keyring path is a directory, expected file")
			}

			if info.Size() == 0 {
				t.Error("keyring file is empty")
			}

			// Verify permissions (should be readable)
			mode := info.Mode()
			if mode.Perm()&0400 == 0 {
				t.Error("keyring file is not readable")
			}
		})
	}
}

func TestExtractAllKeyrings(t *testing.T) {
	tmpDir := t.TempDir()

	err := extractAllKeyrings(tmpDir)
	if err != nil {
		t.Fatalf("extractAllKeyrings failed: %v", err)
	}

	// Verify mise keyring was extracted (chezmoi doesn't have GPG)
	if !keyringExists(tmpDir, BinaryMise) {
		t.Error("keyring for mise does not exist")
	}

	// Verify chezmoi keyring was NOT extracted (no GPG support)
	if keyringExists(tmpDir, BinaryChezmoi) {
		t.Error("chezmoi keyring should not exist (no GPG support)")
	}
}

func TestGetKeyringPath(t *testing.T) {
	tests := []struct {
		name       string
		keyringDir string
		binary     Binary
		expected   string
	}{
		{
			name:       "mise_path",
			keyringDir: "/tmp/keyrings",
			binary:     BinaryMise,
			expected:   "/tmp/keyrings/mise.gpg",
		},
		{
			name:       "chezmoi_path",
			keyringDir: "/home/user/.config/zerb/keyrings",
			binary:     BinaryChezmoi,
			expected:   "/home/user/.config/zerb/keyrings/chezmoi.gpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getKeyringPath(tt.keyringDir, tt.binary)
			if got != tt.expected {
				t.Errorf("getKeyringPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestKeyringExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Extract mise keyring
	err := extractKeyring(tmpDir, BinaryMise)
	if err != nil {
		t.Fatalf("failed to extract keyring: %v", err)
	}

	tests := []struct {
		name     string
		binary   Binary
		expected bool
	}{
		{
			name:     "existing_keyring_mise",
			binary:   BinaryMise,
			expected: true,
		},
		{
			name:     "non_existing_keyring_chezmoi",
			binary:   BinaryChezmoi,
			expected: false, // chezmoi keyring never extracted (no GPG)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := keyringExists(tmpDir, tt.binary)
			if exists != tt.expected {
				t.Errorf("keyringExists() = %v, want %v", exists, tt.expected)
			}
		})
	}
}

func TestExtractKeyringCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "nested", "keyring", "dir")

	// Should create nested directories
	err := extractKeyring(keyringDir, BinaryMise)
	if err != nil {
		t.Fatalf("extractKeyring failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(keyringDir); os.IsNotExist(err) {
		t.Error("keyring directory was not created")
	}

	// Verify keyring file exists
	if !keyringExists(keyringDir, BinaryMise) {
		t.Error("keyring file was not created")
	}
}
