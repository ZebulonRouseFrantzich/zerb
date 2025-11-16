package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
)

// TestWriteGitignore tests .gitignore file creation
func TestWriteGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	gitignorePath := filepath.Join(tmpDir, ".gitignore")

	err := WriteGitignore(gitignorePath)
	if err != nil {
		t.Fatalf("WriteGitignore() error = %v, want nil", err)
	}

	// Verify file was created
	if _, err := os.Stat(gitignorePath); err != nil {
		t.Errorf("WriteGitignore() did not create file: %v", err)
	}

	// Verify file has correct permissions
	info, err := os.Stat(gitignorePath)
	if err != nil {
		t.Fatalf("failed to stat .gitignore: %v", err)
	}

	// Check permissions (0644)
	if info.Mode().Perm() != 0644 {
		t.Errorf("WriteGitignore() permissions = %o, want %o", info.Mode().Perm(), 0644)
	}

	// Verify file contains expected patterns
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)
	expectedPatterns := []string{
		"bin/",
		"cache/",
		"tmp/",
		"logs/",
		"mise/",
		".txn/",
		".direnv/",
		"mise/config.toml",
		"chezmoi/config.toml",
		"zerb.lua.active",
		".zerb-active",
		"keyrings/",
		".zerb-no-git",
	}

	for _, pattern := range expectedPatterns {
		if !contains(contentStr, pattern) {
			t.Errorf("WriteGitignore() missing pattern: %q", pattern)
		}
	}
}

// TestWriteGitignore_CreatesParentDir tests that WriteGitignore creates parent directory
func TestWriteGitignore_CreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", ".gitignore")

	err := WriteGitignore(nestedPath)
	if err != nil {
		t.Fatalf("WriteGitignore() error = %v, want nil", err)
	}

	// Verify file was created
	if _, err := os.Stat(nestedPath); err != nil {
		t.Errorf("WriteGitignore() did not create file in nested directory: %v", err)
	}
}

// TestGitignoreEffectiveness tests that .gitignore patterns work correctly
func TestGitignoreEffectiveness(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Write .gitignore
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := WriteGitignore(gitignorePath); err != nil {
		t.Fatalf("WriteGitignore() error = %v", err)
	}

	// Initialize git repo
	if err := client.InitRepo(ctx); err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	// Create files that should be ignored
	ignoredDirs := []string{
		"bin",
		"cache",
		"tmp",
		"logs",
		"mise",
		".txn",
		".direnv",
		"keyrings",
	}

	for _, dir := range ignoredDirs {
		dirPath := filepath.Join(tmpDir, dir)
		os.MkdirAll(dirPath, 0755)
		testFile := filepath.Join(dirPath, "test.txt")
		os.WriteFile(testFile, []byte("test"), 0644)
	}

	// Create files that should NOT be ignored
	trackedDirs := []string{
		"configs",
		filepath.Join("chezmoi", "source"),
	}

	for _, dir := range trackedDirs {
		dirPath := filepath.Join(tmpDir, dir)
		os.MkdirAll(dirPath, 0755)
		testFile := filepath.Join(dirPath, "test.txt")
		os.WriteFile(testFile, []byte("test"), 0644)
	}

	// Create specific ignored files
	os.WriteFile(filepath.Join(tmpDir, "zerb.lua.active"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".zerb-active"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".zerb-no-git"), []byte("test"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "mise"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "mise", "config.toml"), []byte("test"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "chezmoi"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "chezmoi", "config.toml"), []byte("test"), 0644)

	// Get repository status
	repo, err := gogit.PlainOpen(tmpDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	status, err := worktree.Status()
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	// Verify ignored files are not in status (except .gitignore itself)
	for file := range status {
		// .gitignore itself should be untracked
		if file == ".gitignore" {
			continue
		}

		// Check if file is in an ignored directory
		for _, ignoredDir := range ignoredDirs {
			if contains(file, ignoredDir+"/") {
				t.Errorf("Ignored file %q appears in status", file)
			}
		}

		// Check if file is specifically ignored
		ignoredFiles := []string{
			"zerb.lua.active",
			".zerb-active",
			".zerb-no-git",
			"mise/config.toml",
			"chezmoi/config.toml",
		}
		for _, ignoredFile := range ignoredFiles {
			if file == ignoredFile {
				t.Errorf("Ignored file %q appears in status", file)
			}
		}
	}

	// Verify tracked files ARE in status
	expectedTracked := []string{
		"configs/test.txt",
		"chezmoi/source/test.txt",
	}

	for _, trackedFile := range expectedTracked {
		if _, exists := status[trackedFile]; !exists {
			t.Errorf("Tracked file %q does not appear in status", trackedFile)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
