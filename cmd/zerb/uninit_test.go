package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUninitFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantFlags *UninitFlags
		wantErr   bool
	}{
		{
			name: "No flags",
			args: []string{},
			wantFlags: &UninitFlags{
				force:       false,
				keepConfigs: false,
				keepCache:   false,
				keepBackups: false,
				noBackup:    false,
				dryRun:      false,
			},
			wantErr: false,
		},
		{
			name: "Force flag",
			args: []string{"--force"},
			wantFlags: &UninitFlags{
				force:       true,
				keepConfigs: false,
				keepCache:   false,
				keepBackups: false,
				noBackup:    false,
				dryRun:      false,
			},
			wantErr: false,
		},
		{
			name: "Short force flag",
			args: []string{"-f"},
			wantFlags: &UninitFlags{
				force:       true,
				keepConfigs: false,
				keepCache:   false,
				keepBackups: false,
				noBackup:    false,
				dryRun:      false,
			},
			wantErr: false,
		},
		{
			name: "Keep flags",
			args: []string{"--keep-configs", "--keep-cache", "--keep-backups"},
			wantFlags: &UninitFlags{
				force:       false,
				keepConfigs: true,
				keepCache:   true,
				keepBackups: true,
				noBackup:    false,
				dryRun:      false,
			},
			wantErr: false,
		},
		{
			name: "No backup flag",
			args: []string{"--no-backup"},
			wantFlags: &UninitFlags{
				force:       false,
				keepConfigs: false,
				keepCache:   false,
				keepBackups: false,
				noBackup:    true,
				dryRun:      false,
			},
			wantErr: false,
		},
		{
			name: "Dry run flag",
			args: []string{"--dry-run"},
			wantFlags: &UninitFlags{
				force:       false,
				keepConfigs: false,
				keepCache:   false,
				keepBackups: false,
				noBackup:    false,
				dryRun:      true,
			},
			wantErr: false,
		},
		{
			name: "Multiple flags",
			args: []string{"--force", "--keep-configs", "--dry-run"},
			wantFlags: &UninitFlags{
				force:       true,
				keepConfigs: true,
				keepCache:   false,
				keepBackups: false,
				noBackup:    false,
				dryRun:      true,
			},
			wantErr: false,
		},
		{
			name:      "Unknown flag",
			args:      []string{"--unknown"},
			wantFlags: nil,
			wantErr:   true,
		},
		{
			name:      "Help flag",
			args:      []string{"--help"},
			wantFlags: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseUninitFlags(tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseUninitFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if flags.force != tt.wantFlags.force {
				t.Errorf("force = %v, want %v", flags.force, tt.wantFlags.force)
			}
			if flags.keepConfigs != tt.wantFlags.keepConfigs {
				t.Errorf("keepConfigs = %v, want %v", flags.keepConfigs, tt.wantFlags.keepConfigs)
			}
			if flags.keepCache != tt.wantFlags.keepCache {
				t.Errorf("keepCache = %v, want %v", flags.keepCache, tt.wantFlags.keepCache)
			}
			if flags.keepBackups != tt.wantFlags.keepBackups {
				t.Errorf("keepBackups = %v, want %v", flags.keepBackups, tt.wantFlags.keepBackups)
			}
			if flags.noBackup != tt.wantFlags.noBackup {
				t.Errorf("noBackup = %v, want %v", flags.noBackup, tt.wantFlags.noBackup)
			}
			if flags.dryRun != tt.wantFlags.dryRun {
				t.Errorf("dryRun = %v, want %v", flags.dryRun, tt.wantFlags.dryRun)
			}
		})
	}
}

func TestCalculateDirectorySize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	subdir := filepath.Join(tmpDir, "subdir")
	file3 := filepath.Join(subdir, "file3.txt")

	// Write files with known sizes
	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("world!!!"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(file3, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file3: %v", err)
	}

	// Calculate size
	size, err := calculateDirectorySize(tmpDir)
	if err != nil {
		t.Fatalf("calculateDirectorySize() error = %v", err)
	}

	// Expected size: 5 + 8 + 4 = 17 bytes
	expectedSize := int64(17)
	if size != expectedSize {
		t.Errorf("calculateDirectorySize() = %d, want %d", size, expectedSize)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "Bytes",
			bytes: 500,
			want:  "500 B",
		},
		{
			name:  "Kilobytes",
			bytes: 1024,
			want:  "1.0 KB",
		},
		{
			name:  "Megabytes",
			bytes: 1024 * 1024,
			want:  "1.0 MB",
		},
		{
			name:  "Gigabytes",
			bytes: 1024 * 1024 * 1024,
			want:  "1.0 GB",
		},
		{
			name:  "Mixed KB",
			bytes: 1536,
			want:  "1.5 KB",
		},
		{
			name:  "Mixed MB",
			bytes: 2.5 * 1024 * 1024,
			want:  "2.5 MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindActivationLineNumber(t *testing.T) {
	tmpDir := t.TempDir()
	rcPath := filepath.Join(tmpDir, "test.rc")

	content := `# My bashrc
export PATH=$PATH:/usr/local/bin

# ZERB - Developer environment manager
eval "$(zerb activate bash)"

alias ll='ls -la'
`

	if err := os.WriteFile(rcPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test RC file: %v", err)
	}

	lineNum := findActivationLineNumber(rcPath)
	expectedLine := 5 // The eval line is on line 5

	if lineNum != expectedLine {
		t.Errorf("findActivationLineNumber() = %d, want %d", lineNum, expectedLine)
	}
}

func TestFindBackupFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test backup files
	backupFiles := []string{
		filepath.Join(tmpDir, ".bashrc.zerb-backup.20231112-120000"),
		filepath.Join(tmpDir, ".bashrc.zerb-backup.20231113-130000"),
		filepath.Join(tmpDir, ".zshrc.zerb-backup.20231114-140000"),
	}

	for _, backup := range backupFiles {
		if err := os.WriteFile(backup, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backup file: %v", err)
		}
	}

	// Also create a non-backup file
	normalFile := filepath.Join(tmpDir, ".bashrc")
	if err := os.WriteFile(normalFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create normal file: %v", err)
	}

	// Find backups
	found := findBackupFiles(tmpDir)

	// Should find 3 backup files
	if len(found) != 3 {
		t.Errorf("findBackupFiles() found %d files, want 3", len(found))
	}
}

func TestRemoveShellIntegrations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test RC file with ZERB integration
	rcPath := filepath.Join(tmpDir, ".bashrc")
	content := `export PATH=$PATH:/usr/local/bin

# ZERB - Developer environment manager
eval "$(zerb activate bash)"

alias ll='ls -la'
`
	if err := os.WriteFile(rcPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create RC file: %v", err)
	}

	// Create plan
	plan := &RemovalPlan{
		ShellIntegrations: []ShellIntegration{
			{
				Shell:  "bash",
				RCFile: rcPath,
				Line:   4,
			},
		},
	}

	// Test removal
	flags := &UninitFlags{
		noBackup: true, // Skip backup for test
	}

	err := removeShellIntegrations(plan, flags)
	if err != nil {
		t.Fatalf("removeShellIntegrations() error = %v", err)
	}

	// Read result
	result, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	// Should not contain activation line
	if strings.Contains(string(result), "zerb activate") {
		t.Error("RC file still contains activation line")
	}

	// Should still contain other content
	if !strings.Contains(string(result), "export PATH") {
		t.Error("RC file missing original content")
	}
}

func TestRemoveShellIntegrations_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test RC file with ZERB integration
	rcPath := filepath.Join(tmpDir, ".bashrc")
	originalContent := `export PATH=$PATH:/usr/local/bin

# ZERB - Developer environment manager
eval "$(zerb activate bash)"

alias ll='ls -la'
`
	if err := os.WriteFile(rcPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create RC file: %v", err)
	}

	// Create plan
	plan := &RemovalPlan{
		ShellIntegrations: []ShellIntegration{
			{
				Shell:  "bash",
				RCFile: rcPath,
				Line:   4,
			},
		},
	}

	// Test dry run - should not modify file
	flags := &UninitFlags{
		dryRun: true,
	}

	err := removeShellIntegrations(plan, flags)
	if err != nil {
		t.Fatalf("removeShellIntegrations() error = %v", err)
	}

	// Read result - should be unchanged
	result, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	if string(result) != originalContent {
		t.Error("Dry run modified the file")
	}
}

func TestRemoveZerbDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, "zerb")

	// Create ZERB directory structure
	dirs := []string{
		filepath.Join(zerbDir, "bin"),
		filepath.Join(zerbDir, "configs"),
		filepath.Join(zerbDir, "cache"),
		filepath.Join(zerbDir, "keyrings"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Create some files
	if err := os.WriteFile(filepath.Join(zerbDir, "bin", "mise"), []byte("test"), 0755); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(zerbDir, "configs", "config.lua"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	flags := &UninitFlags{}

	err := removeZerbDirectory(zerbDir, flags)
	if err != nil {
		t.Fatalf("removeZerbDirectory() error = %v", err)
	}

	// Directory should be removed
	if _, err := os.Stat(zerbDir); !os.IsNotExist(err) {
		t.Error("ZERB directory still exists")
	}
}

func TestRemoveZerbDirectory_KeepConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, "zerb")

	// Set HOME for backup directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create ZERB directory structure
	configsDir := filepath.Join(zerbDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Create config file
	configFile := filepath.Join(configsDir, "config.lua")
	if err := os.WriteFile(configFile, []byte("test config"), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	flags := &UninitFlags{
		keepConfigs: true,
	}

	err := removeZerbDirectory(zerbDir, flags)
	if err != nil {
		t.Fatalf("removeZerbDirectory() error = %v", err)
	}

	// ZERB directory should be removed
	if _, err := os.Stat(zerbDir); !os.IsNotExist(err) {
		t.Error("ZERB directory still exists")
	}

	// Configs should be backed up
	backupPattern := filepath.Join(tmpDir, ".zerb-configs-backup-*")
	matches, _ := filepath.Glob(backupPattern)
	if len(matches) != 1 {
		t.Errorf("Expected 1 config backup, found %d", len(matches))
	}

	// Check config file exists in backup
	if len(matches) > 0 {
		backupConfig := filepath.Join(matches[0], "config.lua")
		content, err := os.ReadFile(backupConfig)
		if err != nil {
			t.Errorf("Failed to read backup config: %v", err)
		}
		if string(content) != "test config" {
			t.Errorf("Backup config content mismatch")
		}
	}
}

func TestRemoveZerbDirectory_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, "zerb")

	// Create ZERB directory
	if err := os.MkdirAll(zerbDir, 0755); err != nil {
		t.Fatalf("Failed to create ZERB directory: %v", err)
	}

	flags := &UninitFlags{
		dryRun: true,
	}

	err := removeZerbDirectory(zerbDir, flags)
	if err != nil {
		t.Fatalf("removeZerbDirectory() error = %v", err)
	}

	// Directory should still exist (dry run)
	if _, err := os.Stat(zerbDir); err != nil {
		t.Error("ZERB directory was removed in dry run mode")
	}
}

func TestRemoveBackupFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test backup files
	backupFiles := []string{
		filepath.Join(tmpDir, "backup1.txt"),
		filepath.Join(tmpDir, "backup2.txt"),
	}

	for _, backup := range backupFiles {
		if err := os.WriteFile(backup, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create backup file: %v", err)
		}
	}

	flags := &UninitFlags{}

	err := removeBackupFiles(backupFiles, flags)
	if err != nil {
		t.Fatalf("removeBackupFiles() error = %v", err)
	}

	// Files should be removed
	for _, backup := range backupFiles {
		if _, err := os.Stat(backup); !os.IsNotExist(err) {
			t.Errorf("Backup file %s still exists", backup)
		}
	}
}

func TestRemoveBackupFiles_KeepBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test backup file
	backupFile := filepath.Join(tmpDir, "backup.txt")
	if err := os.WriteFile(backupFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	flags := &UninitFlags{
		keepBackups: true,
	}

	err := removeBackupFiles([]string{backupFile}, flags)
	if err != nil {
		t.Fatalf("removeBackupFiles() error = %v", err)
	}

	// File should still exist
	if _, err := os.Stat(backupFile); err != nil {
		t.Error("Backup file was removed despite keepBackups flag")
	}
}

func TestAnalyzeInstallation(t *testing.T) {
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, "zerb")

	// Create ZERB directory structure
	binDir := filepath.Join(zerbDir, "bin")
	configsDir := filepath.Join(zerbDir, "configs")
	cacheDir := filepath.Join(zerbDir, "cache")

	for _, dir := range []string{binDir, configsDir, cacheDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Create binaries
	if err := os.WriteFile(filepath.Join(binDir, "mise"), []byte("mise binary"), 0755); err != nil {
		t.Fatalf("Failed to create mise binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "chezmoi"), []byte("chezmoi binary"), 0755); err != nil {
		t.Fatalf("Failed to create chezmoi binary: %v", err)
	}

	// Create config file
	if err := os.WriteFile(filepath.Join(configsDir, "config.lua"), []byte("config"), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create cache file
	if err := os.WriteFile(filepath.Join(cacheDir, "cache.dat"), []byte("cache data"), 0644); err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Analyze
	plan, err := analyzeInstallation(nil, zerbDir)
	if err != nil {
		t.Fatalf("analyzeInstallation() error = %v", err)
	}

	// Check results
	if !plan.ZerbDirExists {
		t.Error("ZerbDirExists should be true")
	}

	if len(plan.Binaries) != 2 {
		t.Errorf("Expected 2 binaries, got %d", len(plan.Binaries))
	}

	if plan.ConfigCount != 1 {
		t.Errorf("Expected 1 config, got %d", plan.ConfigCount)
	}

	if plan.ZerbDirSize == 0 {
		t.Error("ZerbDirSize should be > 0")
	}

	if plan.CacheSize == 0 {
		t.Error("CacheSize should be > 0")
	}
}

func TestAnalyzeInstallation_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	zerbDir := filepath.Join(tmpDir, "nonexistent-zerb")

	// Analyze non-existent installation
	plan, err := analyzeInstallation(nil, zerbDir)
	if err != nil {
		t.Fatalf("analyzeInstallation() error = %v", err)
	}

	// Check results
	if plan.ZerbDirExists {
		t.Error("ZerbDirExists should be false")
	}

	if plan.ZerbDirSize != 0 {
		t.Error("ZerbDirSize should be 0 for non-existent directory")
	}
}
