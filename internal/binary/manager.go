package binary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

// Manager orchestrates binary download, verification, and installation
type Manager struct {
	binDir       string
	keyringDir   string
	cacheDir     string
	platformInfo *platform.Info
	downloader   *Downloader
	verifier     *Verifier
	extractor    *Extractor
}

// Config holds configuration for the binary manager
type Config struct {
	// ZerbDir is the root ZERB directory (default: ~/.config/zerb)
	ZerbDir string
	// PlatformInfo contains OS and architecture information
	PlatformInfo *platform.Info
}

// NewManager creates a new binary manager
func NewManager(config Config) (*Manager, error) {
	if config.ZerbDir == "" {
		return nil, fmt.Errorf("ZerbDir is required")
	}

	if config.PlatformInfo == nil {
		return nil, fmt.Errorf("PlatformInfo is required")
	}

	// Construct directory paths
	binDir := filepath.Join(config.ZerbDir, "bin")
	keyringDir := filepath.Join(config.ZerbDir, "keyrings")
	cacheDir := filepath.Join(config.ZerbDir, "cache", "downloads")

	// Create manager
	manager := &Manager{
		binDir:       binDir,
		keyringDir:   keyringDir,
		cacheDir:     cacheDir,
		platformInfo: config.PlatformInfo,
		downloader:   NewDownloader(cacheDir),
		verifier:     NewVerifier(keyringDir),
		extractor:    NewExtractor(),
	}

	return manager, nil
}

// EnsureKeyrings extracts embedded GPG keyrings to disk
func (m *Manager) EnsureKeyrings() error {
	// Extract all embedded keyrings
	if err := extractAllKeyrings(m.keyringDir); err != nil {
		return fmt.Errorf("extract keyrings: %w", err)
	}

	return nil
}

// IsInstalled checks if a binary is already installed and executable
func (m *Manager) IsInstalled(binary Binary) (bool, error) {
	binaryPath := filepath.Join(m.binDir, binary.String())

	// Check if file exists
	info, err := os.Stat(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat binary: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return false, nil
	}

	// Check if it's executable
	if info.Mode().Perm()&0111 == 0 {
		return false, nil
	}

	return true, nil
}

// GetInstalledVersion returns the version of an installed binary
// For now, this returns the hard-coded version since we know what we installed
func (m *Manager) GetInstalledVersion(binary Binary) (string, error) {
	installed, err := m.IsInstalled(binary)
	if err != nil {
		return "", err
	}

	if !installed {
		return "", fmt.Errorf("binary %s is not installed", binary)
	}

	// Return the hard-coded version we installed
	switch binary {
	case BinaryMise:
		return DefaultVersions.Mise, nil
	case BinaryChezmoi:
		return DefaultVersions.Chezmoi, nil
	default:
		return "", fmt.Errorf("unknown binary: %s", binary)
	}
}

// Download downloads and verifies a binary (but doesn't install it)
func (m *Manager) Download(ctx context.Context, opts DownloadOptions) (*DownloadResult, error) {
	startTime := time.Now()

	// Use default version if not specified
	if opts.Version == "" {
		switch opts.Binary {
		case BinaryMise:
			opts.Version = DefaultVersions.Mise
		case BinaryChezmoi:
			opts.Version = DefaultVersions.Chezmoi
		default:
			return nil, fmt.Errorf("unknown binary: %s", opts.Binary)
		}
	}

	// Construct download info
	downloadInfo, err := constructDownloadInfo(opts.Binary, opts.Version, m.platformInfo)
	if err != nil {
		return nil, fmt.Errorf("construct download info: %w", err)
	}

	// Download binary
	binaryPath, err := m.downloader.DownloadBinary(ctx, downloadInfo)
	if err != nil {
		return nil, fmt.Errorf("download binary: %w", err)
	}

	// Download verification files
	var signaturePath, checksumPath string

	if downloadInfo.SignatureURL != "" && !opts.SkipGPG {
		signaturePath, _ = m.downloader.DownloadSignature(ctx, downloadInfo)
		// Ignore error - we'll fall back to SHA256 if signature download fails
	}

	if downloadInfo.ChecksumURL != "" {
		checksumPath, err = m.downloader.DownloadChecksums(ctx, downloadInfo)
		if err != nil {
			return nil, fmt.Errorf("download checksums: %w", err)
		}
	}

	// Verify binary
	verifyResult, err := m.verifier.VerifyFile(binaryPath, signaturePath, checksumPath, downloadInfo)
	if err != nil {
		return nil, fmt.Errorf("verify binary: %w", err)
	}

	if !verifyResult.Success {
		return nil, fmt.Errorf("verification failed: %v", verifyResult.Error)
	}

	// Return result
	result := &DownloadResult{
		Binary:       opts.Binary,
		Version:      opts.Version,
		Path:         binaryPath,
		Verified:     verifyResult.Method,
		DownloadTime: time.Since(startTime),
	}

	return result, nil
}

// Install downloads, verifies, extracts, and installs a binary
func (m *Manager) Install(ctx context.Context, opts DownloadOptions) error {
	// Check if already installed
	installed, err := m.IsInstalled(opts.Binary)
	if err != nil {
		return fmt.Errorf("check if installed: %w", err)
	}

	if installed {
		// Already installed, skip
		return nil
	}

	// Download and verify
	result, err := m.Download(ctx, opts)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// Create bin directory
	if err := os.MkdirAll(m.binDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	// Extract binary to bin directory
	destPath := filepath.Join(m.binDir, opts.Binary.String())
	if err := m.extractor.ExtractBinary(result.Path, destPath, opts.Binary.String()); err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	// Ensure it's executable (should already be set by extractor)
	if err := SetExecutable(destPath); err != nil {
		return fmt.Errorf("set executable: %w", err)
	}

	return nil
}

// InstallAll installs both mise and chezmoi binaries
func (m *Manager) InstallAll(ctx context.Context) error {
	binaries := []Binary{BinaryMise, BinaryChezmoi}

	for _, binary := range binaries {
		opts := DownloadOptions{
			Binary: binary,
		}

		if err := m.Install(ctx, opts); err != nil {
			return fmt.Errorf("install %s: %w", binary, err)
		}
	}

	return nil
}

// GetBinaryPath returns the filesystem path to an installed binary
func (m *Manager) GetBinaryPath(binary Binary) string {
	return filepath.Join(m.binDir, binary.String())
}
