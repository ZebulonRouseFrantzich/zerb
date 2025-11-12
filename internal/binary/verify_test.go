package binary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyGPG(t *testing.T) {
	// Setup: copy test keyring to temp directory
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "keyrings")
	if err := os.MkdirAll(keyringDir, 0755); err != nil {
		t.Fatalf("failed to create keyring dir: %v", err)
	}

	// Copy test key
	testKeyData, err := os.ReadFile("testdata/test-key.gpg")
	if err != nil {
		t.Fatalf("failed to read test key: %v", err)
	}

	testKeyPath := filepath.Join(keyringDir, "mise.gpg")
	if err := os.WriteFile(testKeyPath, testKeyData, 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	verifier := NewVerifier(keyringDir)

	tests := []struct {
		name          string
		binaryPath    string
		signaturePath string
		binary        Binary
		wantSuccess   bool
	}{
		{
			name:          "valid_signature",
			binaryPath:    "testdata/test-binary",
			signaturePath: "testdata/test-binary.asc",
			binary:        BinaryMise,
			wantSuccess:   true,
		},
		{
			name:          "invalid_signature",
			binaryPath:    "testdata/test-file.tar.gz",
			signaturePath: "testdata/test-binary.asc", // Wrong signature
			binary:        BinaryMise,
			wantSuccess:   false,
		},
		{
			name:          "missing_signature",
			binaryPath:    "testdata/test-binary",
			signaturePath: "testdata/nonexistent.asc",
			binary:        BinaryMise,
			wantSuccess:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := verifier.verifyGPG(tt.binaryPath, tt.signaturePath, tt.binary)

			if tt.wantSuccess {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				if result == nil || !result.Success {
					t.Error("expected successful verification")
				}
				if result != nil && result.Method != VerificationGPG {
					t.Errorf("expected GPG method, got %v", result.Method)
				}
			} else {
				if err == nil {
					t.Error("expected error but got none")
				}
				if result == nil || result.Success {
					t.Error("expected verification to fail")
				}
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	verifier := NewVerifier("")

	tests := []struct {
		name         string
		binaryPath   string
		checksumPath string
		wantSuccess  bool
	}{
		{
			name:         "valid_checksum",
			binaryPath:   "testdata/test-binary",
			checksumPath: "testdata/checksums.txt",
			wantSuccess:  true,
		},
		{
			name:         "valid_checksum_tar_gz",
			binaryPath:   "testdata/test-file.tar.gz",
			checksumPath: "testdata/checksums.txt",
			wantSuccess:  true,
		},
		{
			name:         "checksum_not_found",
			binaryPath:   "testdata/nonexistent-file.tar.gz",
			checksumPath: "testdata/checksums.txt",
			wantSuccess:  false,
		},
		{
			name:         "missing_checksum_file",
			binaryPath:   "testdata/test-binary",
			checksumPath: "testdata/nonexistent-checksums.txt",
			wantSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := verifier.verifySHA256(tt.binaryPath, tt.checksumPath)

			if tt.wantSuccess {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				if result == nil || !result.Success {
					t.Error("expected successful verification")
				}
				if result != nil && result.Method != VerificationSHA256 {
					t.Errorf("expected SHA256 method, got %v", result.Method)
				}
			} else {
				if err == nil {
					t.Error("expected error but got none")
				}
				if result == nil || result.Success {
					t.Error("expected verification to fail")
				}
			}
		})
	}
}

func TestCalculateSHA256(t *testing.T) {
	// Create a temp file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Calculate checksum
	checksum, err := calculateSHA256(testFile)
	if err != nil {
		t.Fatalf("calculateSHA256 failed: %v", err)
	}

	// Verify it's a valid hex string
	if len(checksum) != 64 {
		t.Errorf("expected 64-character hex string, got %d characters", len(checksum))
	}

	// Should be deterministic
	checksum2, err := calculateSHA256(testFile)
	if err != nil {
		t.Fatalf("second calculateSHA256 failed: %v", err)
	}

	if checksum != checksum2 {
		t.Error("checksums should be identical for same file")
	}
}

func TestFindChecksum(t *testing.T) {
	tests := []struct {
		name             string
		checksumContent  string
		filename         string
		expectedChecksum string
		wantErr          bool
	}{
		{
			name: "simple_match",
			checksumContent: `abc123  file1.tar.gz
def456  file2.tar.gz
789xyz  file3.tar.gz`,
			filename:         "file2.tar.gz",
			expectedChecksum: "def456",
			wantErr:          false,
		},
		{
			name: "with_path_prefix",
			checksumContent: `abc123  ./downloads/file1.tar.gz
def456  /tmp/file2.tar.gz`,
			filename:         "file2.tar.gz",
			expectedChecksum: "def456",
			wantErr:          false,
		},
		{
			name: "ambiguous_suffix_match",
			checksumContent: `abc123  foo-mise.tar.gz
def456  mise.tar.gz
789xyz  bar-mise.tar.gz`,
			filename:         "mise.tar.gz",
			expectedChecksum: "def456", // Should match exact, not first suffix match
			wantErr:          false,
		},
		{
			name: "basename_match_with_path",
			checksumContent: `abc123  /path/to/mise.tar.gz
def456  another/path/mise.tar.gz`,
			filename:         "mise.tar.gz",
			expectedChecksum: "abc123", // Should match first basename match
			wantErr:          false,
		},
		{
			name: "not_found",
			checksumContent: `abc123  file1.tar.gz
def456  file2.tar.gz`,
			filename: "file3.tar.gz",
			wantErr:  true,
		},
		{
			name:            "empty_file",
			checksumContent: "",
			filename:        "file1.tar.gz",
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp checksum file
			tmpDir := t.TempDir()
			checksumPath := filepath.Join(tmpDir, "checksums.txt")
			if err := os.WriteFile(checksumPath, []byte(tt.checksumContent), 0644); err != nil {
				t.Fatalf("failed to create checksum file: %v", err)
			}

			// Find checksum
			checksum, err := findChecksum(checksumPath, tt.filename)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if checksum != tt.expectedChecksum {
				t.Errorf("checksum mismatch:\ngot:  %s\nwant: %s", checksum, tt.expectedChecksum)
			}
		})
	}
}

func TestVerifyFile(t *testing.T) {
	// Setup keyring
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "keyrings")
	os.MkdirAll(keyringDir, 0755)

	testKeyData, _ := os.ReadFile("testdata/test-key.gpg")
	os.WriteFile(filepath.Join(keyringDir, "mise.gpg"), testKeyData, 0644)

	verifier := NewVerifier(keyringDir)

	tests := []struct {
		name           string
		binary         Binary
		binaryPath     string
		signaturePath  string
		checksumPath   string
		expectedMethod VerificationMethod
		wantSuccess    bool
		wantError      bool
	}{
		{
			name:           "mise_gpg_success",
			binary:         BinaryMise,
			binaryPath:     "testdata/test-binary",
			signaturePath:  "testdata/test-binary.asc",
			checksumPath:   "testdata/checksums.txt",
			expectedMethod: VerificationGPG,
			wantSuccess:    true,
			wantError:      false,
		},
		{
			name:           "mise_requires_gpg",
			binary:         BinaryMise,
			binaryPath:     "testdata/test-binary",
			signaturePath:  "", // No GPG signature - should fail for mise
			checksumPath:   "testdata/checksums.txt",
			expectedMethod: VerificationNone,
			wantSuccess:    false,
			wantError:      true, // mise REQUIRES GPG
		},
		{
			name:           "chezmoi_sha256_success",
			binary:         BinaryChezmoi,
			binaryPath:     "testdata/test-binary",
			signaturePath:  "", // chezmoi doesn't use GPG
			checksumPath:   "testdata/checksums.txt",
			expectedMethod: VerificationSHA256,
			wantSuccess:    true,
			wantError:      false,
		},
		{
			name:           "chezmoi_requires_checksum",
			binary:         BinaryChezmoi,
			binaryPath:     "testdata/test-binary",
			signaturePath:  "",
			checksumPath:   "", // No checksum - should fail
			expectedMethod: VerificationNone,
			wantSuccess:    false,
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &DownloadInfo{
				Binary: tt.binary,
			}

			result, err := verifier.VerifyFile(tt.binaryPath, tt.signaturePath, tt.checksumPath, "", info)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if tt.wantSuccess {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				if result == nil || !result.Success {
					t.Error("expected successful verification")
				}
				if result != nil && result.Method != tt.expectedMethod {
					t.Errorf("expected method %v, got %v", tt.expectedMethod, result.Method)
				}
			} else {
				if err == nil {
					t.Error("expected verification to fail with error")
				}
			}
		})
	}
}

func TestLoadKeyring(t *testing.T) {
	tmpDir := t.TempDir()

	// Copy test keyring
	testKeyData, err := os.ReadFile("testdata/test-key.gpg")
	if err != nil {
		t.Fatalf("failed to read test key: %v", err)
	}

	testKeyPath := filepath.Join(tmpDir, "mise.gpg")
	if err := os.WriteFile(testKeyPath, testKeyData, 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	verifier := NewVerifier(tmpDir)

	tests := []struct {
		name    string
		binary  Binary
		wantErr bool
	}{
		{
			name:    "load_existing_keyring",
			binary:  BinaryMise,
			wantErr: false,
		},
		{
			name:    "load_nonexistent_keyring",
			binary:  BinaryChezmoi,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring, err := verifier.loadKeyring(tt.binary)

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
		})
	}
}

func TestVerificationResultString(t *testing.T) {
	// Test that VerificationMethod.String() works correctly
	tests := []struct {
		method   VerificationMethod
		expected string
	}{
		{VerificationNone, "None"},
		{VerificationGPG, "GPG"},
		{VerificationCosign, "Cosign"},
		{VerificationSHA256, "SHA256"},
	}

	for _, tt := range tests {
		if got := tt.method.String(); got != tt.expected {
			t.Errorf("VerificationMethod(%d).String() = %q, want %q", tt.method, got, tt.expected)
		}
	}
}

func TestCalculateSHA256_NonExistentFile(t *testing.T) {
	_, err := calculateSHA256("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestFindChecksum_MalformedFile(t *testing.T) {
	tmpDir := t.TempDir()
	checksumPath := filepath.Join(tmpDir, "malformed.txt")

	// Create malformed checksum file (missing filename)
	malformedContent := "abc123\ndef456"
	os.WriteFile(checksumPath, []byte(malformedContent), 0644)

	_, err := findChecksum(checksumPath, "test.txt")
	if err == nil {
		t.Error("expected error for checksum not found")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
