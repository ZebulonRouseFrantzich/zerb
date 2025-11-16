package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	repoPath := "/home/user/.config/zerb"
	client := NewClient(repoPath)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.repoPath != repoPath {
		t.Errorf("Client.repoPath = %q, want %q", client.repoPath, repoPath)
	}
}

func TestClient_Stage_SingleFile(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("cannot initialize git repo: %v", err)
	}

	// Configure git user for commits
	exec.Command("git", "config", "user.name", "Test User").Dir = tmpDir
	exec.Command("git", "config", "user.email", "test@example.com").Dir = tmpDir

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Stage(ctx, "test.txt")
	if err != nil {
		t.Errorf("Stage() error = %v, want nil", err)
	}

	// Verify file was staged
	cmd = exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()
	if !strings.Contains(string(output), "test.txt") {
		t.Errorf("Stage() did not stage file, git diff --cached output: %s", output)
	}
}

func TestClient_Stage_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Fatalf("cannot initialize git repo: %v", err)
	}

	nameCmd := exec.Command("git", "config", "user.name", "Test")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	// Create multiple test files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("cannot create test file %s: %v", name, err)
		}
	}

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Stage(ctx, "file1.txt", "file2.txt", "file3.txt")
	if err != nil {
		t.Errorf("Stage() error = %v, want nil", err)
	}

	// Verify all files were staged
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()
	outputStr := string(output)

	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if !strings.Contains(outputStr, name) {
			t.Errorf("Stage() did not stage %s", name)
		}
	}
}

func TestClient_Commit(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Fatalf("cannot initialize git repo: %v", err)
	}

	nameCmd := exec.Command("git", "config", "user.name", "Test User")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	// Create and stage a file
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	addCmd.Run()

	client := NewClient(tmpDir)
	ctx := context.Background()

	msg := "Test commit message"
	body := "This is the commit body\nwith multiple lines"

	err := client.Commit(ctx, msg, body)
	if err != nil {
		t.Errorf("Commit() error = %v, want nil", err)
	}

	// Verify commit was created with correct message
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = tmpDir
	output, _ := cmd.Output()

	if !strings.Contains(string(output), msg) {
		t.Errorf("Commit() message not found in git log: %s", output)
	}
}

func TestClient_Commit_WithoutBody(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	nameCmd := exec.Command("git", "config", "user.name", "Test")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	addCmd.Run()

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Commit(ctx, "Simple message", "")
	if err != nil {
		t.Errorf("Commit() error = %v, want nil", err)
	}
}

func TestClient_Stage_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't initialize as git repo

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Stage(ctx, "file.txt")
	if err == nil {
		t.Error("Stage() in non-git repo should return error")
	}
}

func TestClient_Commit_NothingToCommit(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	nameCmd := exec.Command("git", "config", "user.name", "Test")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Commit(ctx, "Empty commit", "")
	if err == nil {
		t.Error("Commit() with nothing staged should return error")
	}
}

func TestClient_Stage_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	client := NewClient(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.Stage(ctx, "file.txt")
	if err == nil {
		t.Error("Stage() with cancelled context should return error")
	}
}

func TestClient_Commit_ContextTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	nameCmd := exec.Command("git", "config", "user.name", "Test")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	addCmd.Run()

	client := NewClient(tmpDir)

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout has passed

	err := client.Commit(ctx, "Test", "")
	if err == nil {
		t.Error("Commit() with expired context should return error")
	}
}

func TestClient_Stage_EmptyFileList(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Stage(ctx)
	if err == nil {
		t.Error("Stage() with no files should return error")
	}
}

func TestClient_Commit_EmptyMessage(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	initCmd.Run()

	nameCmd := exec.Command("git", "config", "user.name", "Test")
	nameCmd.Dir = tmpDir
	nameCmd.Run()

	emailCmd := exec.Command("git", "config", "user.email", "test@example.com")
	emailCmd.Dir = tmpDir
	emailCmd.Run()

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	addCmd.Run()

	client := NewClient(tmpDir)
	ctx := context.Background()

	err := client.Commit(ctx, "", "")
	if err == nil {
		t.Error("Commit() with empty message should return error")
	}
}
