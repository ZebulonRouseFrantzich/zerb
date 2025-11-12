package binary

import (
	"bufio"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp" //nolint:staticcheck // Using ProtonMail's maintained fork
	"github.com/sigstore/sigstore/pkg/signature"
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
// - chezmoi: Uses cosign verification if bundlePath provided, falls back to SHA256
func (v *Verifier) VerifyFile(binaryPath, signaturePath, checksumPath, bundlePath string, info *DownloadInfo) (*VerificationResult, error) {
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
		// chezmoi: Use cosign verification (key-based) with embedded public key
		if signaturePath != "" && checksumPath != "" {
			result, err := v.verifyCosign(binaryPath, signaturePath, checksumPath, info)
			if err != nil {
				return nil, fmt.Errorf("cosign verification failed for chezmoi: %w", err)
			}
			return result, nil
		}

		// Fallback to SHA256 if no signature (backward compatibility)
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

// verifyCosign verifies a checksums file using a cosign signature (key-based),
// then verifies the binary checksum against the verified checksums file
func (v *Verifier) verifyCosign(binaryPath, signaturePath, checksumPath string, info *DownloadInfo) (*VerificationResult, error) {
	// Step 1: Verify the cosign signature on the checksums file
	if err := v.verifyCosignSignature(signaturePath, checksumPath, info.Binary); err != nil {
		return &VerificationResult{
			Method:  VerificationCosign,
			Success: false,
			Error:   fmt.Errorf("cosign signature verification failed: %w", err),
		}, err
	}

	// Step 2: Now that checksums file is verified, check binary checksum
	result, err := v.verifySHA256(binaryPath, checksumPath)
	if err != nil || !result.Success {
		return &VerificationResult{
			Method:  VerificationCosign,
			Success: false,
			Error:   fmt.Errorf("checksum verification failed after cosign: %w", err),
		}, err
	}

	// Success: Both cosign signature and binary checksum verified
	return &VerificationResult{
		Method:  VerificationCosign,
		Success: true,
		Error:   nil,
	}, nil
}

// verifyCosignSignature verifies a cosign signature (key-based) against an artifact
// This uses the embedded public key for the binary
func (v *Verifier) verifyCosignSignature(signaturePath, artifactPath string, binary Binary) error {
	// Load public key for this binary
	publicKey, err := v.loadCosignPublicKey(binary)
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}

	// Read signature file (base64-encoded ECDSA signature)
	sigBytes, err := os.ReadFile(signaturePath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}

	// Decode base64 signature
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBytes)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Read artifact (checksums file)
	artifactData, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}

	// Create verifier with public key
	verifier, err := signature.LoadVerifier(publicKey, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("create verifier: %w", err)
	}

	// Verify signature
	if err := verifier.VerifySignature(strings.NewReader(string(sig)), strings.NewReader(string(artifactData))); err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}

	return nil
}

// loadCosignPublicKey loads the embedded cosign public key for a binary
func (v *Verifier) loadCosignPublicKey(binary Binary) (crypto.PublicKey, error) {
	// Get keyring path
	keyPath := filepath.Join(v.keyringDir, binary.String()+".pub")

	// Read PEM-encoded public key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}

	// Decode PEM
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Parse public key
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	// Verify it's an ECDSA key (cosign uses ECDSA)
	ecdsaKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ECDSA (got %T)", pubKey)
	}

	return ecdsaKey, nil
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
