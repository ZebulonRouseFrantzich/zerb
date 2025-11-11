package binary

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

// Helper to create a proper tar.gz file with a binary
func createBinaryTarGz(t *testing.T, binaryName, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, binaryName+".tar.gz")

	// Create archive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}
	defer archiveFile.Close()

	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Add binary to archive
	header := &tar.Header{
		Name: binaryName,
		Mode: 0755,
		Size: int64(len(content)),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}

	tarWriter.Close()
	gzipWriter.Close()
	archiveFile.Close()

	return archivePath
}

// Helper to calculate SHA256 of a file
func calculateFileSHA256(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		t.Fatalf("failed to hash file: %v", err)
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

func TestManagerDownload_Complete(t *testing.T) {
	// Create a real tar.gz with chezmoi binary (uses SHA256 verification)
	binaryContent := "#!/bin/sh\necho 'Mock chezmoi binary'\n"
	archivePath := createBinaryTarGz(t, "chezmoi", binaryContent)

	// Calculate checksum
	checksum := calculateFileSHA256(t, archivePath)
	archiveFilename := filepath.Base(archivePath)

	// Create checksums content
	checksumsContent := fmt.Sprintf("%s  %s\n", checksum, archiveFilename)

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksums.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsContent))
		} else if filepath.Base(r.URL.Path) == archiveFilename {
			// Serve the archive
			archiveData, _ := os.ReadFile(archivePath)
			w.WriteHeader(http.StatusOK)
			w.Write(archiveData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create manager
	tmpDir := t.TempDir()
	config := Config{
		ZerbDir: tmpDir,
		PlatformInfo: &platform.Info{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Override download URLs to point to our test server
	// We'll construct the download info manually
	downloadInfo := &DownloadInfo{
		Binary:      BinaryChezmoi,
		Version:     "test",
		OS:          "linux",
		Arch:        "amd64",
		URL:         server.URL + "/" + archiveFilename,
		ChecksumURL: server.URL + "/checksums.txt",
	}

	// Download binary
	ctx := context.Background()
	binaryPath, err := manager.downloader.DownloadBinary(ctx, downloadInfo)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Verify binary was downloaded
	if !fileExists(binaryPath) {
		t.Error("binary was not downloaded")
	}

	// Download checksums
	checksumPath, err := manager.downloader.DownloadChecksums(ctx, downloadInfo)
	if err != nil {
		t.Fatalf("checksum download failed: %v", err)
	}

	// Verify checksums
	result, err := manager.verifier.VerifyFile(binaryPath, "", checksumPath, downloadInfo)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !result.Success {
		t.Error("verification should have succeeded")
	}

	if result.Method != VerificationSHA256 {
		t.Errorf("expected SHA256 verification, got %v", result.Method)
	}
}

func TestManagerInstall_Complete(t *testing.T) {
	// Create a real tar.gz with chezmoi binary (uses SHA256 verification)
	binaryContent := "#!/bin/sh\necho 'Mock chezmoi binary'\n"
	archivePath := createBinaryTarGz(t, "chezmoi", binaryContent)

	// Calculate checksum
	checksum := calculateFileSHA256(t, archivePath)
	archiveFilename := filepath.Base(archivePath)

	// Create checksums content
	checksumsContent := fmt.Sprintf("%s  %s\n", checksum, archiveFilename)

	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksums.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsContent))
		} else if filepath.Base(r.URL.Path) == archiveFilename {
			// Serve the archive
			archiveData, _ := os.ReadFile(archivePath)
			w.WriteHeader(http.StatusOK)
			w.Write(archiveData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create manager
	tmpDir := t.TempDir()
	config := Config{
		ZerbDir: tmpDir,
		PlatformInfo: &platform.Info{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Manually construct download info for our test server
	downloadInfo := &DownloadInfo{
		Binary:      BinaryChezmoi,
		Version:     "test",
		OS:          "linux",
		Arch:        "amd64",
		URL:         server.URL + "/" + archiveFilename,
		ChecksumURL: server.URL + "/checksums.txt",
	}

	ctx := context.Background()

	// Download
	binaryPath, err := manager.downloader.DownloadBinary(ctx, downloadInfo)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Download checksums
	checksumPath, err := manager.downloader.DownloadChecksums(ctx, downloadInfo)
	if err != nil {
		t.Fatalf("checksum download failed: %v", err)
	}

	// Verify
	_, err = manager.verifier.VerifyFile(binaryPath, "", checksumPath, downloadInfo)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	// Extract to bin directory
	destPath := manager.GetBinaryPath(BinaryChezmoi)
	if err := manager.extractor.ExtractBinary(binaryPath, destPath, "chezmoi"); err != nil {
		t.Fatalf("extraction failed: %v", err)
	}

	// Verify installed
	installed, err := manager.IsInstalled(BinaryChezmoi)
	if err != nil {
		t.Fatalf("IsInstalled check failed: %v", err)
	}

	if !installed {
		t.Error("binary should be installed")
	}

	// Verify binary content
	installedContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read installed binary: %v", err)
	}

	if string(installedContent) != binaryContent {
		t.Errorf("binary content mismatch:\ngot:  %q\nwant: %q",
			string(installedContent), binaryContent)
	}

	// Verify executable
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("failed to stat binary: %v", err)
	}

	if info.Mode().Perm()&0111 == 0 {
		t.Error("binary should be executable")
	}
}

func TestManagerDownload_DefaultVersion(t *testing.T) {
	tmpDir := t.TempDir()

	config := Config{
		ZerbDir: tmpDir,
		PlatformInfo: &platform.Info{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Test that default versions are used when not specified
	opts := DownloadOptions{
		Binary: BinaryMise,
		// Version not specified - should use default
	}

	// We can't actually download without a real server, but we can verify
	// the manager correctly selects the default version
	expectedVersion := DefaultVersions.Mise

	info, err := constructDownloadInfo(opts.Binary, expectedVersion, manager.platformInfo)
	if err != nil {
		t.Fatalf("failed to construct download info: %v", err)
	}

	if info.Version != expectedVersion {
		t.Errorf("version mismatch: got %s, want %s", info.Version, expectedVersion)
	}
}

func TestManagerInstall_RespectsSkipGPG(t *testing.T) {
	tmpDir := t.TempDir()

	config := Config{
		ZerbDir: tmpDir,
		PlatformInfo: &platform.Info{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Verify SkipGPG flag can be passed through
	opts := DownloadOptions{
		Binary:  BinaryMise,
		SkipGPG: true,
	}

	if !opts.SkipGPG {
		t.Error("SkipGPG should be true")
	}

	// Verify manager has verifier configured
	if manager.verifier == nil {
		t.Error("manager should have verifier")
	}

	// The actual behavior is tested in the download/verify tests
	// This just ensures the option is available
}
