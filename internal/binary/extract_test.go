package binary

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// Helper function to create a test tar.gz archive
func createTestTarGz(t *testing.T, files map[string]string) string {
	t.Helper()

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create archive file
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}
	defer archiveFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Add files to archive
	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("failed to write header for %s: %v", name, err)
		}

		// Write content
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write content for %s: %v", name, err)
		}
	}

	return archivePath
}

func TestExtractTarGz(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
	}{
		{
			name: "simple_extraction",
			files: map[string]string{
				"file1.txt": "content1",
				"file2.txt": "content2",
			},
			wantErr: false,
		},
		{
			name: "nested_directories",
			files: map[string]string{
				"dir1/file1.txt":      "content1",
				"dir1/dir2/file2.txt": "content2",
				"dir3/file3.txt":      "content3",
			},
			wantErr: false,
		},
		{
			name: "executable_binary",
			files: map[string]string{
				"bin/myapp": "#!/bin/sh\necho hello",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test archive
			archivePath := createTestTarGz(t, tt.files)

			// Extract to temp directory
			destDir := t.TempDir()
			extractor := NewExtractor()
			err := extractor.ExtractTarGz(archivePath, destDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("extraction failed: %v", err)
			}

			// Verify extracted files
			for name, expectedContent := range tt.files {
				extractedPath := filepath.Join(destDir, name)

				// Check file exists
				if !fileExists(extractedPath) {
					t.Errorf("file %s was not extracted", name)
					continue
				}

				// Check content
				content, err := os.ReadFile(extractedPath)
				if err != nil {
					t.Errorf("failed to read extracted file %s: %v", name, err)
					continue
				}

				if string(content) != expectedContent {
					t.Errorf("content mismatch for %s:\ngot:  %q\nwant: %q",
						name, string(content), expectedContent)
				}
			}
		})
	}
}

func TestExtractBinary(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string
		binaryName   string
		wantErr      bool
		expectToFind bool
	}{
		{
			name: "extract_specific_binary",
			files: map[string]string{
				"mise":      "mise binary content",
				"README.md": "readme content",
				"LICENSE":   "license content",
			},
			binaryName:   "mise",
			expectToFind: true,
			wantErr:      false,
		},
		{
			name: "binary_in_subdirectory",
			files: map[string]string{
				"bin/chezmoi": "chezmoi binary content",
				"bin/other":   "other content",
				"docs/README": "readme",
			},
			binaryName:   "chezmoi",
			expectToFind: true,
			wantErr:      false,
		},
		{
			name: "binary_not_found",
			files: map[string]string{
				"file1.txt": "content1",
				"file2.txt": "content2",
			},
			binaryName:   "nonexistent",
			expectToFind: false,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test archive
			archivePath := createTestTarGz(t, tt.files)

			// Extract binary to temp location
			destDir := t.TempDir()
			destPath := filepath.Join(destDir, tt.binaryName)

			extractor := NewExtractor()
			err := extractor.ExtractBinary(archivePath, destPath, tt.binaryName)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("extraction failed: %v", err)
			}

			// Verify binary was extracted
			if !fileExists(destPath) {
				t.Error("binary was not extracted")
				return
			}

			// Verify content
			expectedContent := ""
			for name, content := range tt.files {
				if filepath.Base(name) == tt.binaryName {
					expectedContent = content
					break
				}
			}

			actualContent, err := os.ReadFile(destPath)
			if err != nil {
				t.Fatalf("failed to read extracted binary: %v", err)
			}

			if string(actualContent) != expectedContent {
				t.Errorf("content mismatch:\ngot:  %q\nwant: %q",
					string(actualContent), expectedContent)
			}

			// Verify permissions are executable
			info, err := os.Stat(destPath)
			if err != nil {
				t.Fatalf("failed to stat binary: %v", err)
			}

			if info.Mode().Perm()&0111 == 0 {
				t.Error("binary is not executable")
			}
		})
	}
}

func TestExtractTarGz_PathTraversal(t *testing.T) {
	// Create an archive with a path traversal attempt
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "malicious.tar.gz")

	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}
	defer archiveFile.Close()

	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Add file with path traversal
	header := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: 4,
	}

	tarWriter.WriteHeader(header)
	tarWriter.Write([]byte("bad\n"))

	tarWriter.Close()
	gzipWriter.Close()
	archiveFile.Close()

	// Attempt extraction
	destDir := filepath.Join(tmpDir, "extract")
	extractor := NewExtractor()
	err = extractor.ExtractTarGz(archivePath, destDir)

	// Should fail due to path traversal protection
	if err == nil {
		t.Error("expected error for path traversal attempt")
	}

	// Verify malicious file was not created
	maliciousPath := filepath.Join(tmpDir, "etc", "passwd")
	if fileExists(maliciousPath) {
		t.Error("path traversal protection failed - malicious file was created")
	}
}

func TestSetExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-file")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Verify initial permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Mode().Perm()&0111 != 0 {
		t.Error("file should not be executable initially")
	}

	// Set executable
	if err := SetExecutable(testFile); err != nil {
		t.Fatalf("SetExecutable failed: %v", err)
	}

	// Verify new permissions
	info, err = os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat file after SetExecutable: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("permissions mismatch: got %o, want 0755", info.Mode().Perm())
	}
}

func TestExtractTarGz_CorruptedArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a corrupted archive
	corruptedPath := filepath.Join(tmpDir, "corrupted.tar.gz")
	if err := os.WriteFile(corruptedPath, []byte("not a valid gzip file"), 0644); err != nil {
		t.Fatalf("failed to create corrupted file: %v", err)
	}

	// Attempt extraction
	destDir := filepath.Join(tmpDir, "extract")
	extractor := NewExtractor()
	err := extractor.ExtractTarGz(corruptedPath, destDir)

	if err == nil {
		t.Error("expected error for corrupted archive")
	}
}

func TestExtractBinary_CreatesNestedDirectories(t *testing.T) {
	// Create test archive
	files := map[string]string{
		"bin/myapp": "app content",
	}
	archivePath := createTestTarGz(t, files)

	// Extract to deeply nested path
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "a", "b", "c", "d", "myapp")

	extractor := NewExtractor()
	err := extractor.ExtractBinary(archivePath, destPath, "myapp")

	if err != nil {
		t.Fatalf("extraction failed: %v", err)
	}

	if !fileExists(destPath) {
		t.Error("binary was not extracted to nested directory")
	}
}
