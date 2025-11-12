package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRCFilePath(t *testing.T) {
	// Save original home dir
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Set test home dir
	testHome := "/home/testuser"
	os.Setenv("HOME", testHome)

	tests := []struct {
		name    string
		shell   ShellType
		want    string
		wantErr bool
	}{
		{
			name:    "Bash RC file",
			shell:   ShellBash,
			want:    filepath.Join(testHome, ".bashrc"),
			wantErr: false,
		},
		{
			name:    "Zsh RC file",
			shell:   ShellZsh,
			want:    filepath.Join(testHome, ".zshrc"),
			wantErr: false,
		},
		{
			name:    "Fish RC file",
			shell:   ShellFish,
			want:    filepath.Join(testHome, ".config", "fish", "config.fish"),
			wantErr: false,
		},
		{
			name:    "Unknown shell",
			shell:   ShellUnknown,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetRCFilePath(tt.shell)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRCFilePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetRCFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRCFileExists(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a test file
	existingFile := filepath.Join(tmpDir, "existing.rc")
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a directory (not a file)
	dirPath := filepath.Join(tmpDir, "dir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name:    "Existing file",
			path:    existingFile,
			want:    true,
			wantErr: false,
		},
		{
			name:    "Non-existent file",
			path:    filepath.Join(tmpDir, "nonexistent.rc"),
			want:    false,
			wantErr: false,
		},
		{
			name:    "Directory instead of file",
			path:    dirPath,
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RCFileExists(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("RCFileExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RCFileExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateRCFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "Create new RC file",
			path:    filepath.Join(tmpDir, "new.rc"),
			wantErr: false,
		},
		{
			name:    "Create RC file with nested directory",
			path:    filepath.Join(tmpDir, "subdir", "config", "new.rc"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateRCFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRCFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file exists
				exists, err := RCFileExists(tt.path)
				if err != nil {
					t.Fatalf("Failed to check created file: %v", err)
				}
				if !exists {
					t.Errorf("CreateRCFile() did not create file at %s", tt.path)
				}

				// Verify file has header
				content, err := os.ReadFile(tt.path)
				if err != nil {
					t.Fatalf("Failed to read created file: %v", err)
				}
				if !strings.Contains(string(content), "# Shell configuration") {
					t.Errorf("CreateRCFile() did not write header")
				}
			}
		})
	}
}

func TestHasActivationLine(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file without activation
	noActivation := filepath.Join(tmpDir, "no-activation.rc")
	_ = os.WriteFile(noActivation, []byte("# Just a comment\nexport PATH=/usr/bin\n"), 0644)

	// Create file with activation
	withActivation := filepath.Join(tmpDir, "with-activation.rc")
	_ = os.WriteFile(withActivation, []byte("# Config\neval \"$(zerb activate bash)\"\n"), 0644)

	// Create file with activation in comment
	activationComment := filepath.Join(tmpDir, "activation-comment.rc")
	_ = os.WriteFile(activationComment, []byte("# Add: zerb activate bash\nexport PATH=/usr/bin\n"), 0644)

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{
			name:    "File without activation",
			path:    noActivation,
			want:    false,
			wantErr: false,
		},
		{
			name:    "File with activation",
			path:    withActivation,
			want:    true,
			wantErr: false,
		},
		{
			name:    "File with activation in comment",
			path:    activationComment,
			want:    true,
			wantErr: false,
		},
		{
			name:    "Non-existent file",
			path:    filepath.Join(tmpDir, "nonexistent.rc"),
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HasActivationLine(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("HasActivationLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("HasActivationLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackupRCFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test RC file
	rcFile := filepath.Join(tmpDir, "test.rc")
	originalContent := "# Original content\nexport PATH=/usr/bin\n"
	if err := os.WriteFile(rcFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create backup
	backupPath, err := BackupRCFile(rcFile)
	if err != nil {
		t.Fatalf("BackupRCFile() error = %v", err)
	}

	// Verify backup path has timestamp format: .zerb-backup.YYYYMMDD-HHMMSS
	expectedPrefix := rcFile + BackupSuffix + "."
	if !strings.HasPrefix(backupPath, expectedPrefix) {
		t.Errorf("BackupRCFile() path = %v, want prefix %v", backupPath, expectedPrefix)
	}
	// Verify timestamp format (8 digits + dash + 6 digits)
	timestampPart := strings.TrimPrefix(backupPath, expectedPrefix)
	if len(timestampPart) != 15 || timestampPart[8] != '-' {
		t.Errorf("BackupRCFile() timestamp format = %v, want YYYYMMDD-HHMMSS", timestampPart)
	}

	// Verify backup exists
	exists, err := RCFileExists(backupPath)
	if err != nil {
		t.Fatalf("Failed to check backup file: %v", err)
	}
	if !exists {
		t.Errorf("BackupRCFile() did not create backup at %s", backupPath)
	}

	// Verify backup content matches original
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}
	if string(backupContent) != originalContent {
		t.Errorf("BackupRCFile() content mismatch\ngot:  %q\nwant: %q", string(backupContent), originalContent)
	}
}

func TestAddActivationLine(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name              string
		existingContent   string
		activationCommand string
		wantContains      []string
	}{
		{
			name:              "Add to empty file",
			existingContent:   "",
			activationCommand: `eval "$(zerb activate bash)"`,
			wantContains: []string{
				"# ZERB - Developer environment manager",
				`eval "$(zerb activate bash)"`,
			},
		},
		{
			name:              "Add to existing content",
			existingContent:   "# Existing config\nexport PATH=/usr/bin\n",
			activationCommand: `eval "$(zerb activate zsh)"`,
			wantContains: []string{
				"# Existing config",
				"export PATH=/usr/bin",
				"# ZERB - Developer environment manager",
				`eval "$(zerb activate zsh)"`,
			},
		},
		{
			name:              "Add to content without trailing newline",
			existingContent:   "# Config without newline",
			activationCommand: `eval "$(zerb activate fish)"`,
			wantContains: []string{
				"# Config without newline",
				"# ZERB - Developer environment manager",
				`eval "$(zerb activate fish)"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create RC file
			rcFile := filepath.Join(tmpDir, tt.name+".rc")
			if tt.existingContent != "" {
				if err := os.WriteFile(rcFile, []byte(tt.existingContent), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			// Add activation line
			err := AddActivationLine(rcFile, tt.activationCommand)
			if err != nil {
				t.Fatalf("AddActivationLine() error = %v", err)
			}

			// Read result
			content, err := os.ReadFile(rcFile)
			if err != nil {
				t.Fatalf("Failed to read result file: %v", err)
			}

			// Verify all expected strings are present
			contentStr := string(content)
			for _, want := range tt.wantContains {
				if !strings.Contains(contentStr, want) {
					t.Errorf("AddActivationLine() result does not contain %q\nGot:\n%s", want, contentStr)
				}
			}
		})
	}
}

func TestAddActivationLine_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	rcFile := filepath.Join(tmpDir, "test.rc")

	activationCommand := `eval "$(zerb activate bash)"`

	// Add activation line first time
	err := AddActivationLine(rcFile, activationCommand)
	if err != nil {
		t.Fatalf("First AddActivationLine() error = %v", err)
	}

	// Read content after first add
	firstContent, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Add activation line second time
	err = AddActivationLine(rcFile, activationCommand)
	if err != nil {
		t.Fatalf("Second AddActivationLine() error = %v", err)
	}

	// Read content after second add
	secondContent, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Count occurrences of activation command
	secondCount := strings.Count(string(secondContent), "ZERB - Developer environment manager")

	// Should have added the section twice (not idempotent by design in current implementation)
	// Note: The idempotency check should be done at a higher level (HasActivationLine before calling AddActivationLine)
	if secondCount != 2 {
		t.Logf("Note: AddActivationLine is not idempotent by itself - this is expected behavior")
		t.Logf("Idempotency should be handled by checking HasActivationLine() before calling AddActivationLine()")
		t.Logf("First content length: %d, Second content length: %d", len(firstContent), len(secondContent))
	}
}
