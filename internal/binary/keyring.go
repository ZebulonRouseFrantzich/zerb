package binary

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Embedded GPG public keys for mise and chezmoi
// These are embedded at compile time and extracted to ~/.config/zerb/keyrings/ at runtime

//go:embed keyrings/mise.gpg
var miseKeyring []byte

// Note: chezmoi does not provide GPG signatures for releases, only SHA256 checksums
// var chezmoiKeyring []byte - not embedded

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

// extractAllKeyrings extracts all embedded keyrings to the keyring directory
func extractAllKeyrings(keyringDir string) error {
	// Only extract keyrings for binaries that support GPG
	binaries := []Binary{BinaryMise}

	for _, binary := range binaries {
		if err := extractKeyring(keyringDir, binary); err != nil {
			return fmt.Errorf("extract %s keyring: %w", binary, err)
		}
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
