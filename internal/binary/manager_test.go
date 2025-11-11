package binary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

func TestNewManager(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid_config",
			config: Config{
				ZerbDir: "/tmp/zerb",
				PlatformInfo: &platform.Info{
					OS:   "linux",
					Arch: "amd64",
				},
			},
			wantErr: false,
		},
		{
			name: "missing_zerb_dir",
			config: Config{
				PlatformInfo: &platform.Info{
					OS:   "linux",
					Arch: "amd64",
				},
			},
			wantErr: true,
		},
		{
			name: "missing_platform_info",
			config: Config{
				ZerbDir: "/tmp/zerb",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if manager == nil {
				t.Fatal("expected non-nil manager")
			}

			// Verify directories are set correctly
			expectedBinDir := filepath.Join(tt.config.ZerbDir, "bin")
			if manager.binDir != expectedBinDir {
				t.Errorf("binDir mismatch: got %s, want %s", manager.binDir, expectedBinDir)
			}
		})
	}
}

func TestManagerEnsureKeyrings(t *testing.T) {
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

	// Ensure keyrings
	if err := manager.EnsureKeyrings(); err != nil {
		t.Fatalf("EnsureKeyrings failed: %v", err)
	}

	// Verify mise keyring was extracted
	miseKeyringPath := filepath.Join(manager.keyringDir, "mise.gpg")
	if !fileExists(miseKeyringPath) {
		t.Error("mise keyring was not extracted")
	}

	// Verify keyring is valid
	keyringData, err := os.ReadFile(miseKeyringPath)
	if err != nil {
		t.Fatalf("failed to read keyring: %v", err)
	}

	if len(keyringData) == 0 {
		t.Error("keyring file is empty")
	}
}

func TestManagerIsInstalled(t *testing.T) {
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

	// Initially not installed
	installed, err := manager.IsInstalled(BinaryMise)
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if installed {
		t.Error("binary should not be installed initially")
	}

	// Create bin directory and add a mock binary
	binPath := manager.GetBinaryPath(BinaryMise)
	os.MkdirAll(filepath.Dir(binPath), 0755)
	os.WriteFile(binPath, []byte("#!/bin/sh\necho test"), 0755)

	// Now should be installed
	installed, err = manager.IsInstalled(BinaryMise)
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if !installed {
		t.Error("binary should be installed now")
	}
}

func TestManagerIsInstalled_NotExecutable(t *testing.T) {
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

	// Create binary without execute permissions
	binPath := manager.GetBinaryPath(BinaryMise)
	os.MkdirAll(filepath.Dir(binPath), 0755)
	os.WriteFile(binPath, []byte("test"), 0644) // Not executable

	// Should not be considered installed
	installed, err := manager.IsInstalled(BinaryMise)
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if installed {
		t.Error("non-executable binary should not be considered installed")
	}
}

func TestManagerGetInstalledVersion(t *testing.T) {
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

	// Not installed - should error
	_, err = manager.GetInstalledVersion(BinaryMise)
	if err == nil {
		t.Error("expected error for non-installed binary")
	}

	// Install mock binary
	binPath := manager.GetBinaryPath(BinaryMise)
	os.MkdirAll(filepath.Dir(binPath), 0755)
	os.WriteFile(binPath, []byte("#!/bin/sh\necho test"), 0755)

	// Get version
	version, err := manager.GetInstalledVersion(BinaryMise)
	if err != nil {
		t.Fatalf("GetInstalledVersion failed: %v", err)
	}

	if version != DefaultVersions.Mise {
		t.Errorf("version mismatch: got %s, want %s", version, DefaultVersions.Mise)
	}
}

func TestManagerDownload(t *testing.T) {
	// Create mock HTTP server
	mockBinaryContent := "mock binary content for testing download"
	mockChecksums := "abc123def456  mise-v2024.12.7-linux-x64.tar.gz"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve different content based on URL
		if strings.Contains(r.URL.Path, ".tar.gz") {
			// Create a minimal tar.gz with binary
			// For testing, just return mock content
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockBinaryContent))
		} else if strings.Contains(r.URL.Path, "checksums") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockChecksums))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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

	// Note: This test would need a more complex setup to actually verify
	// the complete download flow with proper tar.gz and signatures.
	// For now, we test that the basic structure works.

	// The download will fail because we don't have a proper tar.gz,
	// but we can verify the manager is set up correctly
	if manager.downloader == nil {
		t.Error("downloader should not be nil")
	}

	if manager.verifier == nil {
		t.Error("verifier should not be nil")
	}

	if manager.extractor == nil {
		t.Error("extractor should not be nil")
	}
}

func TestManagerGetBinaryPath(t *testing.T) {
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

	tests := []struct {
		binary       Binary
		expectedName string
	}{
		{BinaryMise, "mise"},
		{BinaryChezmoi, "chezmoi"},
	}

	for _, tt := range tests {
		path := manager.GetBinaryPath(tt.binary)

		if !strings.HasSuffix(path, tt.expectedName) {
			t.Errorf("path should end with %s, got %s", tt.expectedName, path)
		}

		if !strings.Contains(path, filepath.Join(tmpDir, "bin")) {
			t.Errorf("path should contain bin directory: %s", path)
		}
	}
}

func TestManagerInstall_SkipIfAlreadyInstalled(t *testing.T) {
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

	// Pre-install a mock binary
	binPath := manager.GetBinaryPath(BinaryMise)
	os.MkdirAll(filepath.Dir(binPath), 0755)
	originalContent := []byte("original content")
	os.WriteFile(binPath, originalContent, 0755)

	// Try to install - should skip
	ctx := context.Background()
	err = manager.Install(ctx, DownloadOptions{Binary: BinaryMise})

	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify original content is unchanged
	content, _ := os.ReadFile(binPath)
	if string(content) != string(originalContent) {
		t.Error("binary content was changed (should have been skipped)")
	}
}

func TestManagerConfig_DirectoryStructure(t *testing.T) {
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

	// Verify directory structure
	expectedDirs := map[string]string{
		"binDir":     filepath.Join(tmpDir, "bin"),
		"keyringDir": filepath.Join(tmpDir, "keyrings"),
		"cacheDir":   filepath.Join(tmpDir, "cache", "downloads"),
	}

	if manager.binDir != expectedDirs["binDir"] {
		t.Errorf("binDir mismatch: got %s, want %s", manager.binDir, expectedDirs["binDir"])
	}

	if manager.keyringDir != expectedDirs["keyringDir"] {
		t.Errorf("keyringDir mismatch: got %s, want %s", manager.keyringDir, expectedDirs["keyringDir"])
	}

	if manager.cacheDir != expectedDirs["cacheDir"] {
		t.Errorf("cacheDir mismatch: got %s, want %s", manager.cacheDir, expectedDirs["cacheDir"])
	}
}

func TestManagerInstallAll(t *testing.T) {
	// This is a basic structural test - full integration would require
	// actual binary downloads which we'll test separately

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

	// Pre-install both binaries to test that InstallAll works
	for _, binary := range []Binary{BinaryMise, BinaryChezmoi} {
		binPath := manager.GetBinaryPath(binary)
		os.MkdirAll(filepath.Dir(binPath), 0755)
		os.WriteFile(binPath, []byte("test"), 0755)
	}

	// InstallAll should skip both (already installed)
	ctx := context.Background()
	err = manager.InstallAll(ctx)

	if err != nil {
		t.Fatalf("InstallAll failed: %v", err)
	}

	// Verify both are installed
	for _, binary := range []Binary{BinaryMise, BinaryChezmoi} {
		installed, err := manager.IsInstalled(binary)
		if err != nil {
			t.Errorf("IsInstalled failed for %s: %v", binary, err)
		}
		if !installed {
			t.Errorf("%s should be installed", binary)
		}
	}
}
