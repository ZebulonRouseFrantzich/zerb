package binary

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 5 * time.Minute
	// DefaultRetries is the default number of download retries
	DefaultRetries = 3
	// DefaultUserAgent is the User-Agent header sent with requests
	DefaultUserAgent = "ZERB/1.0"
)

// Downloader handles HTTP downloads with retry logic
type Downloader struct {
	client    *http.Client
	cacheDir  string
	userAgent string
	retries   int
}

// NewDownloader creates a new downloader
func NewDownloader(cacheDir string) *Downloader {
	return &Downloader{
		client: &http.Client{
			Timeout: DefaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Allow up to 10 redirects
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		cacheDir:  cacheDir,
		userAgent: DefaultUserAgent,
		retries:   DefaultRetries,
	}
}

// DownloadToFile downloads a URL to a specific file path
func (d *Downloader) DownloadToFile(ctx context.Context, url, destPath string) error {
	var lastErr error

	for attempt := 0; attempt <= d.retries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := d.downloadOnce(ctx, url, destPath)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on context cancellation or certain HTTP errors
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("download failed after %d retries: %w", d.retries, lastErr)
}

// downloadOnce performs a single download attempt
func (d *Downloader) downloadOnce(ctx context.Context, url, destPath string) error {
	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", d.userAgent)

	// Execute request
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create destination directory
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Create temporary file
	tmpPath := destPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	// Track whether we need to clean up the temp file
	cleanupNeeded := true
	defer func() {
		tmpFile.Close()
		if cleanupNeeded {
			os.Remove(tmpPath) // Clean up on error
		}
	}()

	// Copy response body to file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("copy response body: %w", err)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Success - don't clean up the temp file (it's been renamed)
	cleanupNeeded = false
	return nil
}

// DownloadBinary downloads a binary archive to the cache directory
func (d *Downloader) DownloadBinary(ctx context.Context, info *DownloadInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("download info is nil")
	}

	// Construct cache path: cache/{binary}/{version}/{filename}
	filename := filepath.Base(info.URL)
	cachePath := filepath.Join(d.cacheDir, info.Binary.String(), info.Version, filename)

	// Check if already cached
	if fileExists(cachePath) {
		return cachePath, nil
	}

	// Download to cache
	if err := d.DownloadToFile(ctx, info.URL, cachePath); err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}

	return cachePath, nil
}

// DownloadSignature downloads a GPG signature file
func (d *Downloader) DownloadSignature(ctx context.Context, info *DownloadInfo) (string, error) {
	if info == nil || info.SignatureURL == "" {
		return "", fmt.Errorf("no signature URL available")
	}

	// Construct cache path for signature
	filename := filepath.Base(info.SignatureURL)
	cachePath := filepath.Join(d.cacheDir, info.Binary.String(), info.Version, filename)

	// Check if already cached
	if fileExists(cachePath) {
		return cachePath, nil
	}

	// Download signature
	if err := d.DownloadToFile(ctx, info.SignatureURL, cachePath); err != nil {
		return "", fmt.Errorf("download signature: %w", err)
	}

	return cachePath, nil
}

// DownloadChecksums downloads a checksum file
func (d *Downloader) DownloadChecksums(ctx context.Context, info *DownloadInfo) (string, error) {
	if info == nil || info.ChecksumURL == "" {
		return "", fmt.Errorf("no checksum URL available")
	}

	// Construct cache path for checksums
	filename := filepath.Base(info.ChecksumURL)
	cachePath := filepath.Join(d.cacheDir, info.Binary.String(), info.Version, filename)

	// Check if already cached
	if fileExists(cachePath) {
		return cachePath, nil
	}

	// Download checksums
	if err := d.DownloadToFile(ctx, info.ChecksumURL, cachePath); err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}

	return cachePath, nil
}

// DownloadBundle downloads a cosign bundle file
func (d *Downloader) DownloadBundle(ctx context.Context, info *DownloadInfo) (string, error) {
	if info == nil || info.BundleURL == "" {
		return "", fmt.Errorf("no bundle URL available")
	}

	// Construct cache path for bundle
	filename := filepath.Base(info.BundleURL)
	cachePath := filepath.Join(d.cacheDir, info.Binary.String(), info.Version, filename)

	// Check if already cached
	if fileExists(cachePath) {
		return cachePath, nil
	}

	// Download bundle
	if err := d.DownloadToFile(ctx, info.BundleURL, cachePath); err != nil {
		return "", fmt.Errorf("download bundle: %w", err)
	}

	return cachePath, nil
}

// fileExists checks if a file exists and is not empty
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Size() > 0
}
