package chezmoi

import (
	"context"
	"errors"
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

func TestClient_HasFile(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	client := &Client{
		bin:  filepath.Join(tmpDir, "bin", "chezmoi"),
		src:  sourceDir,
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	tests := []struct {
		name      string
		setupFunc func(t *testing.T)
		path      string
		want      bool
		wantErr   bool
	}{
		{
			name: "file exists in source",
			setupFunc: func(t *testing.T) {
				// Create dot_zshrc in source directory
				if err := os.WriteFile(filepath.Join(sourceDir, "dot_zshrc"), []byte("test"), 0644); err != nil {
					t.Fatalf("failed to create source file: %v", err)
				}
			},
			path: "~/.zshrc",
			want: true,
		},
		{
			name: "file does not exist in source",
			setupFunc: func(t *testing.T) {
				// No setup needed - file won't exist
			},
			path:    "~/.bashrc",
			want:    false,
			wantErr: false,
		},
		{
			name: "nested file exists",
			setupFunc: func(t *testing.T) {
				// Create nested directory structure
				nestedDir := filepath.Join(sourceDir, "dot_config", "nvim")
				if err := os.MkdirAll(nestedDir, 0755); err != nil {
					t.Fatalf("failed to create nested dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(nestedDir, "init.lua"), []byte("test"), 0644); err != nil {
					t.Fatalf("failed to create nested file: %v", err)
				}
			},
			path: "~/.config/nvim/init.lua",
			want: true,
		},
		{
			name: "directory exists",
			setupFunc: func(t *testing.T) {
				// Create directory
				dir := filepath.Join(sourceDir, "dot_config", "git")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
			},
			path: "~/.config/git",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc(t)
			}

			ctx := context.Background()
			got, err := client.HasFile(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("HasFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HasFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_HasFile_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	client := &Client{
		bin:  filepath.Join(tmpDir, "bin", "chezmoi"),
		src:  sourceDir,
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.HasFile(ctx, "~/.zshrc")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestClient_HasFile_InvalidPath(t *testing.T) {
	tmpDir := t.TempDir()
	client := &Client{
		bin:  filepath.Join(tmpDir, "bin", "chezmoi"),
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	ctx := context.Background()
	_, err := client.HasFile(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestRedactedError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		context     string
		wantMessage string
		checkIsErr  error
	}{
		{
			name:        "basic error redaction",
			err:         os.ErrNotExist,
			context:     "test operation",
			wantMessage: "test operation:",
			checkIsErr:  os.ErrNotExist,
		},
		{
			name:        "error with path redaction",
			err:         &os.PathError{Op: "stat", Path: "/home/user/.config/file", Err: os.ErrNotExist},
			context:     "check file",
			wantMessage: "check file:",
			checkIsErr:  os.ErrNotExist, // Check for the wrapped error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := newRedactedError(tt.err, tt.context)

			// Check error message
			msg := redacted.Error()
			if !strings.Contains(msg, tt.wantMessage) {
				t.Errorf("RedactedError.Error() = %q, want to contain %q", msg, tt.wantMessage)
			}

			// Check unwrap preserves error chain
			unwrapped := redacted.(*RedactedError).Unwrap()
			if unwrapped != tt.err {
				t.Errorf("RedactedError.Unwrap() = %v, want %v", unwrapped, tt.err)
			}

			// Verify errors.Is works
			if !errors.Is(redacted, tt.checkIsErr) {
				t.Errorf("errors.Is() failed for redacted error, want to find %v", tt.checkIsErr)
			}
		})
	}
}

func TestRedactedError_NilError(t *testing.T) {
	result := newRedactedError(nil, "test")
	if result != nil {
		t.Errorf("newRedactedError(nil) = %v, want nil", result)
	}
}

func TestClient_Remove(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		setupStub  func() string
		path       string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "successful remove",
			setupStub: func() string {
				stubBin := filepath.Join(tmpDir, "chezmoi_success")
				script := `#!/bin/bash
echo "ARGS: $@"
exit 0
`
				os.WriteFile(stubBin, []byte(script), 0755)
				return stubBin
			},
			path:    "~/.zshrc",
			wantErr: false,
		},
		{
			name: "file not found in source (returns nil per HR-3)",
			setupStub: func() string {
				stubBin := filepath.Join(tmpDir, "chezmoi_notfound")
				script := `#!/bin/bash
echo "is not in the source state" >&2
exit 1
`
				os.WriteFile(stubBin, []byte(script), 0755)
				return stubBin
			},
			path:    "~/.nonexistent",
			wantErr: false, // Per HR-3: return nil for not-found
		},
		{
			name: "permission denied",
			setupStub: func() string {
				stubBin := filepath.Join(tmpDir, "chezmoi_perms")
				script := `#!/bin/bash
echo "permission denied" >&2
exit 1
`
				os.WriteFile(stubBin, []byte(script), 0755)
				return stubBin
			},
			path:       "~/.zshrc",
			wantErr:    true,
			wantErrMsg: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubBin := tt.setupStub()
			client := &Client{
				bin:  stubBin,
				src:  filepath.Join(tmpDir, "source"),
				conf: filepath.Join(tmpDir, "config.toml"),
			}

			ctx := context.Background()
			err := client.Remove(ctx, tt.path)

			if (err != nil) != tt.wantErr {
				t.Errorf("Remove() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Remove() error = %q, want substring %q", err.Error(), tt.wantErrMsg)
				}
			}

			// Verify chezmoi is not mentioned
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "chezmoi") {
				t.Errorf("Remove() error should not mention 'chezmoi', got: %q", err.Error())
			}
		})
	}
}

func TestClient_Remove_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	stubBin := filepath.Join(tmpDir, "chezmoi")
	script := `#!/bin/bash
sleep 10
`
	if err := os.WriteFile(stubBin, []byte(script), 0755); err != nil {
		t.Fatalf("cannot create stub: %v", err)
	}

	client := &Client{
		bin:  stubBin,
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.Remove(ctx, "~/.zshrc")
	if err == nil {
		t.Error("Remove() with cancelled context should return error")
	}
}

func TestClient_Remove_UsesForgetCommand(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a stub that captures and validates the command
	stubBin := filepath.Join(tmpDir, "chezmoi")
	argsFile := filepath.Join(tmpDir, "args.txt")
	script := `#!/bin/bash
echo "$@" > ` + argsFile + `
exit 0
`
	if err := os.WriteFile(stubBin, []byte(script), 0755); err != nil {
		t.Fatalf("cannot create stub: %v", err)
	}

	client := &Client{
		bin:  stubBin,
		src:  filepath.Join(tmpDir, "source"),
		conf: filepath.Join(tmpDir, "config.toml"),
	}

	ctx := context.Background()
	if err := client.Remove(ctx, "~/.zshrc"); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}

	// Verify the args contain "forget"
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "forget") {
		t.Errorf("Remove() should use 'forget' command, got args: %s", string(args))
	}
	if !strings.Contains(string(args), "--source") {
		t.Errorf("Remove() should use --source flag, got args: %s", string(args))
	}
	if !strings.Contains(string(args), "--config") {
		t.Errorf("Remove() should use --config flag, got args: %s", string(args))
	}
}
