package binary

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloaderDownloadToFile(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{
			name:       "successful_download",
			statusCode: http.StatusOK,
			body:       "test binary content",
			wantErr:    false,
		},
		{
			name:       "404_not_found",
			statusCode: http.StatusNotFound,
			body:       "not found",
			wantErr:    true,
		},
		{
			name:       "500_server_error",
			statusCode: http.StatusInternalServerError,
			body:       "server error",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify User-Agent header
				if r.Header.Get("User-Agent") != DefaultUserAgent {
					t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
				}

				w.WriteHeader(tt.statusCode)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer server.Close()

			// Create downloader
			tmpDir := t.TempDir()
			downloader := NewDownloader(tmpDir)
			// Reduce retries for faster tests
			downloader.retries = 1

			// Download to temp file
			destPath := filepath.Join(tmpDir, "test-file")
			err := downloader.DownloadToFile(context.Background(), server.URL, destPath)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify file was downloaded
			content, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("failed to read downloaded file: %v", err)
			}

			if string(content) != tt.body {
				t.Errorf("content mismatch:\ngot:  %q\nwant: %q", string(content), tt.body)
			}
		})
	}
}

func TestDownloaderRetryLogic(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Fail first two attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on third attempt
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("success")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)
	downloader.retries = 3

	destPath := filepath.Join(tmpDir, "test-file")
	err := downloader.DownloadToFile(context.Background(), server.URL, destPath)

	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	content, _ := os.ReadFile(destPath)
	if string(content) != "success" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestDownloaderContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("too late")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	destPath := filepath.Join(tmpDir, "test-file")
	err := downloader.DownloadToFile(ctx, server.URL, destPath)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestDownloaderDownloadBinary(t *testing.T) {
	mockContent := "mock binary content for testing"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(mockContent)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	info := &DownloadInfo{
		Binary:  BinaryMise,
		Version: "2024.12.7",
		URL:     server.URL + "/mise-v2024.12.7-linux-x64.tar.gz",
	}

	// First download
	cachePath1, err := downloader.DownloadBinary(context.Background(), info)
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}

	// Verify file exists
	if !fileExists(cachePath1) {
		t.Error("downloaded file does not exist")
	}

	// Verify content
	content, err := os.ReadFile(cachePath1)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(content) != mockContent {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", string(content), mockContent)
	}

	// Second download should use cache (no HTTP request)
	requestCount := 0
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		t.Error("unexpected HTTP request - should use cache")
	})

	cachePath2, err := downloader.DownloadBinary(context.Background(), info)
	if err != nil {
		t.Fatalf("second download failed: %v", err)
	}

	if cachePath1 != cachePath2 {
		t.Errorf("cache paths don't match:\nfirst:  %s\nsecond: %s", cachePath1, cachePath2)
	}

	if requestCount > 0 {
		t.Error("cache was not used for second download")
	}
}

func TestDownloaderDownloadSignature(t *testing.T) {
	mockSig := "-----BEGIN PGP SIGNATURE-----\ntest signature\n-----END PGP SIGNATURE-----"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".asc") || strings.HasSuffix(r.URL.Path, ".sig") {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(mockSig)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	info := &DownloadInfo{
		Binary:       BinaryMise,
		Version:      "2024.12.7",
		SignatureURL: server.URL + "/mise-v2024.12.7-linux-x64.tar.gz.sig",
	}

	sigPath, err := downloader.DownloadSignature(context.Background(), info)
	if err != nil {
		t.Fatalf("download signature failed: %v", err)
	}

	// Verify signature file
	content, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("failed to read signature: %v", err)
	}

	if string(content) != mockSig {
		t.Errorf("signature mismatch:\ngot:  %q\nwant: %q", string(content), mockSig)
	}
}

func TestDownloaderDownloadChecksums(t *testing.T) {
	mockChecksums := "abc123  mise-v2024.12.7-linux-x64.tar.gz\ndef456  mise-v2024.12.7-linux-arm64.tar.gz"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(mockChecksums)); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	info := &DownloadInfo{
		Binary:      BinaryChezmoi,
		Version:     "2.46.1",
		ChecksumURL: server.URL + "/checksums.txt",
	}

	checksumPath, err := downloader.DownloadChecksums(context.Background(), info)
	if err != nil {
		t.Fatalf("download checksums failed: %v", err)
	}

	// Verify checksums file
	content, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("failed to read checksums: %v", err)
	}

	if string(content) != mockChecksums {
		t.Errorf("checksums mismatch:\ngot:  %q\nwant: %q", string(content), mockChecksums)
	}
}

func TestDownloaderCreatesNestedDirectories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("test")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	// Download to deeply nested path
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "d", "file.txt")
	err := downloader.DownloadToFile(context.Background(), server.URL, deepPath)

	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if !fileExists(deepPath) {
		t.Error("file was not created in nested directory")
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "existing_file",
			setup: func() string {
				path := filepath.Join(tmpDir, "exists.txt")
				if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
				return path
			},
			expected: true,
		},
		{
			name: "empty_file",
			setup: func() string {
				path := filepath.Join(tmpDir, "empty.txt")
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
				return path
			},
			expected: false, // Empty files return false
		},
		{
			name: "directory",
			setup: func() string {
				path := filepath.Join(tmpDir, "dir")
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
				return path
			},
			expected: false,
		},
		{
			name: "non_existent",
			setup: func() string {
				return filepath.Join(tmpDir, "doesnotexist.txt")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			result := fileExists(path)
			if result != tt.expected {
				t.Errorf("fileExists(%s) = %v, want %v", path, result, tt.expected)
			}
		})
	}
}

func TestDownloaderRedirectHandling(t *testing.T) {
	redirectCount := 0
	finalContent := "final content after redirects"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirectCount < 3 {
			redirectCount++
			http.Redirect(w, r, fmt.Sprintf("/redirect-%d", redirectCount), http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(finalContent)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := NewDownloader(tmpDir)

	destPath := filepath.Join(tmpDir, "redirected-file")
	err := downloader.DownloadToFile(context.Background(), server.URL, destPath)

	if err != nil {
		t.Fatalf("download with redirects failed: %v", err)
	}

	content, _ := os.ReadFile(destPath)
	if string(content) != finalContent {
		t.Errorf("unexpected content after redirects: %s", string(content))
	}

	if redirectCount != 3 {
		t.Errorf("expected 3 redirects, got %d", redirectCount)
	}
}
