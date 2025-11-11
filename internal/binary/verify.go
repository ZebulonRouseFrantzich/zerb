package binary

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp" //nolint:staticcheck // Using ProtonMail's maintained fork
)

// Verifier handles cryptographic verification of binaries
type Verifier struct {
	keyringDir string
	skipGPG    bool // For testing only
}

// NewVerifier creates a new verifier
func NewVerifier(keyringDir string) *Verifier {
	return &Verifier{
		keyringDir: keyringDir,
		skipGPG:    false,
	}
}

// VerifyFile verifies a downloaded binary file
// It uses the appropriate verification method based on the binary type:
// - mise: REQUIRES GPG verification (no fallback)
// - chezmoi: Uses SHA256 verification (cosign TODO)
func (v *Verifier) VerifyFile(binaryPath, signaturePath, checksumPath string, info *DownloadInfo) (*VerificationResult, error) {
	if info == nil {
		return nil, fmt.Errorf("download info is required")
	}

	// Route verification based on binary type
	switch info.Binary {
	case BinaryMise:
		// mise REQUIRES GPG verification - no fallback
		if signaturePath == "" || v.skipGPG {
			return nil, fmt.Errorf("GPG signature required for mise but not available")
		}

		result, err := v.verifyGPG(binaryPath, signaturePath, info.Binary)
		if err != nil {
			return nil, fmt.Errorf("GPG verification failed for mise: %w", err)
		}

		if !result.Success {
			return nil, fmt.Errorf("GPG verification failed: %v", result.Error)
		}

		return result, nil

	case BinaryChezmoi:
		// chezmoi: Use SHA256 for now (TODO: add cosign verification)
		if checksumPath == "" {
			return nil, fmt.Errorf("checksum file required for chezmoi but not available")
		}

		result, err := v.verifySHA256(binaryPath, checksumPath)
		if err != nil {
			return nil, fmt.Errorf("SHA256 verification failed for chezmoi: %w", err)
		}

		if !result.Success {
			return nil, fmt.Errorf("checksum verification failed: %v", result.Error)
		}

		return result, nil

	default:
		return nil, fmt.Errorf("unknown binary type: %s", info.Binary)
	}
}

// verifyGPG verifies a file using GPG signature
func (v *Verifier) verifyGPG(binaryPath, signaturePath string, binary Binary) (*VerificationResult, error) {
	// Load keyring for this binary
	keyring, err := v.loadKeyring(binary)
	if err != nil {
		return &VerificationResult{
			Method:  VerificationGPG,
			Success: false,
			Error:   fmt.Errorf("load keyring: %w", err),
		}, err
	}

	// Open binary file
	binaryFile, err := os.Open(binaryPath)
	if err != nil {
		return &VerificationResult{
			Method:  VerificationGPG,
			Success: false,
			Error:   fmt.Errorf("open binary: %w", err),
		}, err
	}
	defer binaryFile.Close()

	// Open signature file
	sigFile, err := os.Open(signaturePath)
	if err != nil {
		return &VerificationResult{
			Method:  VerificationGPG,
			Success: false,
			Error:   fmt.Errorf("open signature: %w", err),
		}, err
	}
	defer sigFile.Close()

	// Reset binary file to beginning
	binaryFile.Seek(0, io.SeekStart)

	// Verify signature (try armored first)
	_, err = openpgp.CheckArmoredDetachedSignature(keyring, binaryFile, sigFile, nil)
	if err != nil {
		// Try non-armored signature
		binaryFile.Seek(0, io.SeekStart)
		sigFile.Seek(0, io.SeekStart)
		_, err = openpgp.CheckDetachedSignature(keyring, binaryFile, sigFile, nil)
	}
	if err != nil {
		return &VerificationResult{
			Method:  VerificationGPG,
			Success: false,
			Error:   fmt.Errorf("verify signature: %w", err),
		}, err
	}

	return &VerificationResult{
		Method:  VerificationGPG,
		Success: true,
		Error:   nil,
	}, nil
}

// verifySHA256 verifies a file using SHA256 checksum
func (v *Verifier) verifySHA256(binaryPath, checksumPath string) (*VerificationResult, error) {
	// Calculate SHA256 of binary
	actualChecksum, err := calculateSHA256(binaryPath)
	if err != nil {
		return &VerificationResult{
			Method:  VerificationSHA256,
			Success: false,
			Error:   fmt.Errorf("calculate checksum: %w", err),
		}, err
	}

	// Find expected checksum in checksum file
	expectedChecksum, err := findChecksum(checksumPath, filepath.Base(binaryPath))
	if err != nil {
		return &VerificationResult{
			Method:  VerificationSHA256,
			Success: false,
			Error:   fmt.Errorf("find checksum: %w", err),
		}, err
	}

	// Compare checksums (case-insensitive)
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return &VerificationResult{
			Method:  VerificationSHA256,
			Success: false,
			Error: fmt.Errorf("checksum mismatch:\nactual:   %s\nexpected: %s",
				actualChecksum, expectedChecksum),
		}, fmt.Errorf("checksum mismatch")
	}

	return &VerificationResult{
		Method:  VerificationSHA256,
		Success: true,
		Error:   nil,
	}, nil
}

// loadKeyring loads a GPG keyring from the keyring directory
func (v *Verifier) loadKeyring(binary Binary) (openpgp.EntityList, error) {
	keyringPath := getKeyringPath(v.keyringDir, binary)

	keyringFile, err := os.Open(keyringPath)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}
	defer keyringFile.Close()

	keyring, err := openpgp.ReadArmoredKeyRing(keyringFile)
	if err != nil {
		// Try reading as non-armored keyring
		keyringFile.Seek(0, io.SeekStart)
		keyring, err = openpgp.ReadKeyRing(keyringFile)
		if err != nil {
			return nil, fmt.Errorf("read keyring: %w", err)
		}
	}

	if len(keyring) == 0 {
		return nil, fmt.Errorf("keyring is empty")
	}

	return keyring, nil
}

// calculateSHA256 calculates the SHA256 checksum of a file
func calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// findChecksum finds the checksum for a specific filename in a checksum file
// Format: "abc123def456  filename.tar.gz"
func findChecksum(checksumPath, filename string) (string, error) {
	file, err := os.Open(checksumPath)
	if err != nil {
		return "", fmt.Errorf("open checksum file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Check if this line is for our file
		// Use exact match first, then basename comparison for files with paths
		checksumFilename := parts[1]
		if checksumFilename == filename {
			return parts[0], nil
		}

		// Also check basename (for checksums like "/path/to/file.tar.gz")
		if filepath.Base(checksumFilename) == filename {
			return parts[0], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan checksum file: %w", err)
	}

	return "", fmt.Errorf("checksum not found for %s", filename)
}
