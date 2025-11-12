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
	defer func() { _ = archiveFile.Close() }()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0750); err != nil {
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
		// Use robust validation that handles edge cases
		if err := validateExtractPath(target, destDir); err != nil {
			return fmt.Errorf("illegal file path %s: %w", header.Name, err)
		}

		// Handle different file types
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}

		case tar.TypeReg:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", target, err)
			}

			// Create file
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}

			// Copy file contents with size limit to prevent decompression bombs
			lr := &io.LimitedReader{R: tarReader, N: header.Size}
			if _, err := io.Copy(outFile, lr); err != nil {
				_ = outFile.Close()
				_ = os.Remove(target) // Clean up partial file on error
				return fmt.Errorf("write file %s: %w", target, err)
			}

			if err := outFile.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", target, err)
			}

		case tar.TypeSymlink:
			// Validate symlink target doesn't escape destDir
			if err := validateSymlinkTarget(target, header.Linkname, destDir); err != nil {
				return fmt.Errorf("illegal symlink %s -> %s: %w", header.Name, header.Linkname, err)
			}

			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
				return fmt.Errorf("create parent dir for symlink %s: %w", target, err)
			}

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
	defer func() { _ = archiveFile.Close() }()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

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
			if err := os.MkdirAll(destDir, 0750); err != nil {
				return fmt.Errorf("create dest dir: %w", err)
			}

			// Create destination file with executable permissions
			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}

			// Copy binary contents with size limit
			lr := &io.LimitedReader{R: tarReader, N: header.Size}
			if _, err := io.Copy(outFile, lr); err != nil {
				_ = outFile.Close()
				_ = os.Remove(destPath) // Clean up partial file on error
				return fmt.Errorf("write file: %w", err)
			}

			if err := outFile.Close(); err != nil {
				return fmt.Errorf("close file: %w", err)
			}
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

// validateExtractPath ensures a file path is within the destination directory
// This prevents path traversal attacks (zip-slip vulnerability)
func validateExtractPath(targetPath, destDir string) error {
	// Clean both paths to resolve any . or .. components
	cleanTarget := filepath.Clean(targetPath)
	cleanDest := filepath.Clean(destDir)

	// Ensure target is within destDir or equal to destDir
	// We check three conditions:
	// 1. Exact match (target == destDir) - OK for "." entry
	// 2. Has prefix with separator - target is inside destDir
	// 3. Not an absolute path that differs from destDir
	if cleanTarget == cleanDest {
		return nil // Root directory is OK
	}

	if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
		return fmt.Errorf("path traversal attempt detected")
	}

	// Additional check: reject absolute paths outside destDir
	if filepath.IsAbs(cleanTarget) && !strings.HasPrefix(cleanTarget, cleanDest) {
		return fmt.Errorf("absolute path outside destination")
	}

	return nil
}

// validateSymlinkTarget ensures a symlink target doesn't escape the destination directory
func validateSymlinkTarget(symlinkPath, linkTarget, destDir string) error {
	// Clean paths
	cleanDest := filepath.Clean(destDir)

	// Resolve the symlink target relative to its location
	symlinkDir := filepath.Dir(symlinkPath)
	var resolvedTarget string

	if filepath.IsAbs(linkTarget) {
		// Absolute symlink target
		resolvedTarget = filepath.Clean(linkTarget)
	} else {
		// Relative symlink target - resolve relative to symlink location
		resolvedTarget = filepath.Clean(filepath.Join(symlinkDir, linkTarget))
	}

	// Check if resolved target is within destDir
	if resolvedTarget == cleanDest {
		return nil // Pointing to root is OK
	}

	if !strings.HasPrefix(resolvedTarget, cleanDest+string(os.PathSeparator)) {
		return fmt.Errorf("symlink points outside destination directory")
	}

	return nil
}
