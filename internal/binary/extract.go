package binary

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extractor handles archive extraction
type Extractor struct{}

// NewExtractor creates a new extractor
func NewExtractor() *Extractor {
	return &Extractor{}
}

// ExtractTarGz extracts a .tar.gz archive to a destination directory
func (e *Extractor) ExtractTarGz(archivePath, destDir string) error {
	// Open archive file
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archiveFile.Close()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Extract each file
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Construct target path
		target := filepath.Join(destDir, header.Name)

		// Security check: prevent path traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", header.Name)
		}

		// Handle different file types
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}

		case tar.TypeReg:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", target, err)
			}

			// Create file
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}

			// Copy file contents
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}

			outFile.Close()

		case tar.TypeSymlink:
			// Create symlink
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("create symlink %s: %w", target, err)
			}

		default:
			// Skip other types (char devices, block devices, etc.)
			continue
		}
	}

	return nil
}

// ExtractBinary extracts a specific binary file from a tar.gz archive
// This is optimized for extracting just the binary we need
func (e *Extractor) ExtractBinary(archivePath, destPath, binaryName string) error {
	// Open archive file
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archiveFile.Close()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Search for binary
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return fmt.Errorf("binary %s not found in archive", binaryName)
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Check if this is the binary we want
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == binaryName {
			// Create parent directory if needed
			destDir := filepath.Dir(destPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return fmt.Errorf("create dest dir: %w", err)
			}

			// Create destination file with executable permissions
			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}

			// Copy binary contents
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("write file: %w", err)
			}

			outFile.Close()
			return nil
		}
	}
}

// SetExecutable sets executable permissions on a file
func SetExecutable(path string) error {
	// Set permissions to 0755 (rwxr-xr-x)
	if err := os.Chmod(path, 0755); err != nil {
		return fmt.Errorf("set executable: %w", err)
	}
	return nil
}
