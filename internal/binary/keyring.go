package binary

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Embedded public keys for binary verification
// These are embedded at compile time and extracted to ~/.config/zerb/keyrings/ at runtime

//go:embed keyrings/mise.gpg
var miseKeyring []byte

//go:embed keyrings/chezmoi.pub
var chezmoiCosignKey []byte

// getKeyring returns the embedded GPG keyring for a binary
func getKeyring(binary Binary) ([]byte, error) {
	switch binary {
	case BinaryMise:
		if len(miseKeyring) == 0 {
			return nil, fmt.Errorf("mise keyring is empty (embed failed)")
		}
		return miseKeyring, nil
	case BinaryChezmoi:
		// chezmoi does not provide GPG signatures
		return nil, fmt.Errorf("chezmoi does not support GPG verification (use SHA256 instead)")
	default:
		return nil, fmt.Errorf("unknown binary: %s", binary)
	}
}

// extractKeyring extracts a single GPG keyring to the keyring directory
func extractKeyring(keyringDir string, binary Binary) error {
	keyring, err := getKeyring(binary)
	if err != nil {
		return fmt.Errorf("get keyring: %w", err)
	}

	// Create keyring directory if it doesn't exist
	if err := os.MkdirAll(keyringDir, 0755); err != nil {
		return fmt.Errorf("create keyring dir: %w", err)
	}

	// Write keyring file
	keyringPath := filepath.Join(keyringDir, fmt.Sprintf("%s.gpg", binary))
	if err := os.WriteFile(keyringPath, keyring, 0644); err != nil {
		return fmt.Errorf("write keyring file: %w", err)
	}

	return nil
}

// extractCosignKey extracts an embedded cosign public key to disk
func extractCosignKey(keyringDir string, binary Binary) error {
	// Get the embedded cosign key
	var keyData []byte
	switch binary {
	case BinaryChezmoi:
		keyData = chezmoiCosignKey
	default:
		return fmt.Errorf("no cosign key available for %s", binary)
	}

	if len(keyData) == 0 {
		return fmt.Errorf("cosign key data is empty for %s", binary)
	}

	// Create keyring directory if it doesn't exist
	if err := os.MkdirAll(keyringDir, 0755); err != nil {
		return fmt.Errorf("create keyring dir: %w", err)
	}

	// Write public key file
	keyPath := filepath.Join(keyringDir, fmt.Sprintf("%s.pub", binary))
	if err := os.WriteFile(keyPath, keyData, 0644); err != nil {
		return fmt.Errorf("write cosign key file: %w", err)
	}

	return nil
}

// extractAllKeyrings extracts all embedded keyrings to the keyring directory
func extractAllKeyrings(keyringDir string) error {
	// Extract GPG keyring for mise
	if err := extractKeyring(keyringDir, BinaryMise); err != nil {
		return fmt.Errorf("extract mise keyring: %w", err)
	}

	// Extract cosign public key for chezmoi
	if err := extractCosignKey(keyringDir, BinaryChezmoi); err != nil {
		return fmt.Errorf("extract chezmoi cosign key: %w", err)
	}

	return nil
}

// getKeyringPath returns the filesystem path to a keyring
func getKeyringPath(keyringDir string, binary Binary) string {
	return filepath.Join(keyringDir, fmt.Sprintf("%s.gpg", binary))
}

// keyringExists checks if a keyring file exists on disk
func keyringExists(keyringDir string, binary Binary) bool {
	path := getKeyringPath(keyringDir, binary)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Size() > 0
}
