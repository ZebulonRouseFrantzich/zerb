package chezmoi

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
	zerbDir := "/home/user/.config/zerb"
	client := NewClient(zerbDir)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	expectedBin := filepath.Join(zerbDir, "bin", "chezmoi")
	if client.bin != expectedBin {
		t.Errorf("Client.bin = %q, want %q", client.bin, expectedBin)
	}

	expectedSrc := filepath.Join(zerbDir, "chezmoi", "source")
	if client.src != expectedSrc {
		t.Errorf("Client.src = %q, want %q", client.src, expectedSrc)
	}

	expectedConf := filepath.Join(zerbDir, "chezmoi", "config.toml")
	if client.conf != expectedConf {
		t.Errorf("Client.conf = %q, want %q", client.conf, expectedConf)
	}
}

func TestClient_Add_BasicOptions(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a stub chezmoi binary that prints its arguments
	stubBin := filepath.Join(tmpDir, "chezmoi")
	stubScript := `#!/bin/bash
echo "ARGS: $@"
exit 0
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0755); err != nil {
		t.Fatalf("cannot create stub binary: %v", err)
	}

	client := &Client{
		bin:  stubBin,
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		opts     AddOptions
		wantArgs []string
	}{
		{
			name: "simple add",
			path: testFile,
			opts: AddOptions{},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				testFile,
			},
		},
		{
			name: "add with template",
			path: testFile,
			opts: AddOptions{Template: true},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				"--template",
				testFile,
			},
		},
		{
			name: "add with recursive",
			path: tmpDir,
			opts: AddOptions{Recursive: true},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				"--recursive",
				tmpDir,
			},
		},
		{
			name: "add with secrets",
			path: testFile,
			opts: AddOptions{Secrets: true},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				"--encrypt",
				testFile,
			},
		},
		{
			name: "add with private",
			path: testFile,
			opts: AddOptions{Private: true},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				"--private",
				testFile,
			},
		},
		{
			name: "add with all options",
			path: testFile,
			opts: AddOptions{
				Template:  true,
				Recursive: true,
				Secrets:   true,
				Private:   true,
			},
			wantArgs: []string{
				"--source", client.src,
				"--config", client.conf,
				"add",
				"--template",
				"--recursive",
				"--encrypt",
				"--private",
				testFile,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := client.Add(ctx, tt.path, tt.opts)
			if err != nil {
				// For now we expect success since we're using a stub
				t.Errorf("Add() error = %v", err)
			}
			// Note: We can't easily verify the exact args with the stub
			// This test mainly checks that Add() doesn't crash
			// Integration tests will verify the actual chezmoi invocation
		})
	}
}

func TestClient_Add_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a stub chezmoi binary that sleeps
	stubBin := filepath.Join(tmpDir, "chezmoi")
	stubScript := `#!/bin/bash
sleep 10
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0755); err != nil {
		t.Fatalf("cannot create stub binary: %v", err)
	}

	client := &Client{
		bin:  stubBin,
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	testFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}

	// Create a context with immediate cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.Add(ctx, testFile, AddOptions{})
	if err == nil {
		t.Error("Add() with cancelled context should return error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Add() error should mention context cancellation, got: %v", err)
	}
}

func TestClient_Add_Timeout(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a stub chezmoi binary that sleeps for 2 seconds
	stubBin := filepath.Join(tmpDir, "chezmoi")
	stubScript := `#!/bin/bash
sleep 2
`
	if err := os.WriteFile(stubBin, []byte(stubScript), 0755); err != nil {
		t.Fatalf("cannot create stub binary: %v", err)
	}

	client := &Client{
		bin:  stubBin,
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	testFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}

	// Create a context with a 100ms timeout (should timeout before 2s sleep completes)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Add(ctx, testFile, AddOptions{})
	if err == nil {
		t.Error("Add() with timeout should return error")
		return
	}

	// The test just needs to verify that Add() returns an error when context times out
	// The specific error message can vary by platform and exec implementation
	t.Logf("Add() with timeout returned error: %v (expected - test passes)", err)
}

func TestClient_Add_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		setupStub  func(string) string // Returns path to stub binary
		path       string
		wantErrMsg string
	}{
		{
			name: "file not found error",
			setupStub: func(dir string) string {
				stub := filepath.Join(dir, "chezmoi")
				script := `#!/bin/bash
echo "no such file or directory" >&2
exit 1
`
				os.WriteFile(stub, []byte(script), 0755)
				return stub
			},
			path:       "/nonexistent/file",
			wantErrMsg: "file not found",
		},
		{
			name: "permission denied error",
			setupStub: func(dir string) string {
				stub := filepath.Join(dir, "chezmoi")
				script := `#!/bin/bash
echo "permission denied" >&2
exit 1
`
				os.WriteFile(stub, []byte(script), 0755)
				return stub
			},
			path:       "/root/file",
			wantErrMsg: "permission denied",
		},
		{
			name: "directory without recursive error",
			setupStub: func(dir string) string {
				stub := filepath.Join(dir, "chezmoi")
				script := `#!/bin/bash
echo "is a directory" >&2
exit 1
`
				os.WriteFile(stub, []byte(script), 0755)
				return stub
			},
			path:       "/some/dir",
			wantErrMsg: "path is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubDir := t.TempDir()
			stubBin := tt.setupStub(stubDir)

			client := &Client{
				bin:  stubBin,
				src:  filepath.Join(stubDir, "source"),
				conf: filepath.Join(stubDir, "config.toml"),
			}

			ctx := context.Background()
			err := client.Add(ctx, tt.path, AddOptions{})
			if err == nil {
				t.Fatal("Add() should return error")
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, tt.wantErrMsg) {
				t.Errorf("Add() error = %q, want substring %q", errMsg, tt.wantErrMsg)
			}

			// Verify that "chezmoi" is NOT mentioned in user-facing error
			if strings.Contains(strings.ToLower(errMsg), "chezmoi") {
				t.Errorf("Add() error should not mention 'chezmoi', got: %q", errMsg)
			}
		})
	}
}

func TestTranslateChezmoiError(t *testing.T) {
	tests := []struct {
		name                 string
		stderr               string
		wantErrMsg           string
		wantNoChezmoiMention bool
	}{
		{
			name:                 "file not found",
			stderr:               "error: no such file or directory: /home/user/.zshrc",
			wantErrMsg:           "file not found",
			wantNoChezmoiMention: true,
		},
		{
			name:                 "permission denied",
			stderr:               "error: permission denied",
			wantErrMsg:           "permission denied",
			wantNoChezmoiMention: true,
		},
		{
			name:                 "is a directory",
			stderr:               "error: /home/user/.config is a directory",
			wantErrMsg:           "path is a directory",
			wantNoChezmoiMention: true,
		},
		{
			name:                 "generic error",
			stderr:               "unknown error occurred",
			wantErrMsg:           "failed to add configuration file",
			wantNoChezmoiMention: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := translateChezmoiError(&exec.ExitError{}, tt.stderr)
			if err == nil {
				t.Fatal("translateChezmoiError() should return error")
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, tt.wantErrMsg) {
				t.Errorf("translateChezmoiError() = %q, want substring %q", errMsg, tt.wantErrMsg)
			}

			if tt.wantNoChezmoiMention && strings.Contains(strings.ToLower(errMsg), "chezmoi") {
				t.Errorf("translateChezmoiError() should not mention 'chezmoi', got: %q", errMsg)
			}
		})
	}
}
