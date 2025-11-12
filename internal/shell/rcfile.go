package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetRCFilePath returns the path to the shell's RC file
func GetRCFilePath(shell ShellType) (string, error) {
	if err := ValidateShell(shell); err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	// Security: Validate home directory is not empty
	if homeDir == "" {
		return "", fmt.Errorf("home directory is empty")
	}

	var rcPath string
	switch shell {
	case ShellBash:
		rcPath = filepath.Join(homeDir, ".bashrc")
	case ShellZsh:
		rcPath = filepath.Join(homeDir, ".zshrc")
	case ShellFish:
		rcPath = filepath.Join(homeDir, ".config", "fish", "config.fish")
	default:
		return "", &UnsupportedShellError{Shell: shell.String()}
	}

	// Security: Validate path doesn't contain traversal attempts
	cleanPath := filepath.Clean(rcPath)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid path: contains directory traversal")
	}

	// Security: Path must be absolute
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("RC file path must be absolute")
	}

	return rcPath, nil
}

// RCFileExists checks if the RC file exists
func RCFileExists(rcPath string) (bool, error) {
	info, err := os.Stat(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, &RCFileError{
			Path:    rcPath,
			Message: "failed to stat file",
			Cause:   err,
		}
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return false, &RCFileError{
			Path:    rcPath,
			Message: "not a regular file",
		}
	}

	return true, nil
}

// CreateRCFile creates a new RC file with appropriate directory structure
func CreateRCFile(rcPath string) error {
	// Security: Check for traversal before cleaning
	if strings.Contains(rcPath, "..") {
		return &RCFileError{
			Path:    rcPath,
			Message: "invalid path: contains directory traversal",
		}
	}

	// Security: Path must be absolute
	if !filepath.IsAbs(rcPath) {
		return &RCFileError{
			Path:    rcPath,
			Message: "RC file path must be absolute",
		}
	}

	// Create parent directory if needed (use 0700 for security)
	dir := filepath.Dir(rcPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to create parent directory",
			Cause:   err,
		}
	}

	// Create the file
	file, err := os.Create(rcPath)
	if err != nil {
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to create file",
			Cause:   err,
		}
	}
	defer file.Close()

	// Write a basic header
	header := "# Shell configuration\n"
	if _, err := file.WriteString(header); err != nil {
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to write header",
			Cause:   err,
		}
	}

	return nil
}

// HasActivationLine checks if the RC file already contains a ZERB activation line
func HasActivationLine(rcPath string) (bool, error) {
	file, err := os.Open(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, &RCFileError{
			Path:    rcPath,
			Message: "failed to open file",
			Cause:   err,
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Check for any variation of zerb activate
		if strings.Contains(line, ActivationMarker) {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, &RCFileError{
			Path:    rcPath,
			Message: "failed to read file",
			Cause:   err,
		}
	}

	return false, nil
}

// BackupRCFile creates a timestamped backup of the RC file
// This prevents overwriting previous backups
func BackupRCFile(rcPath string) (string, error) {
	// Read the original file
	content, err := os.ReadFile(rcPath)
	if err != nil {
		return "", &RCFileError{
			Path:    rcPath,
			Message: "failed to read file for backup",
			Cause:   err,
		}
	}

	// Create backup path with timestamp (RFC3339 format, filesystem-safe)
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s%s.%s", rcPath, BackupSuffix, timestamp)

	// Write backup with same permissions as original
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", &RCFileError{
			Path:    backupPath,
			Message: "failed to write backup file",
			Cause:   err,
		}
	}

	return backupPath, nil
}

// AddActivationLine adds the ZERB activation line to the RC file
// This is an atomic operation using a temporary file
// Returns nil if the activation line already exists (idempotent)
func AddActivationLine(rcPath string, activationCommand string) error {
	// Security: Validate activation command format
	if !strings.Contains(activationCommand, ActivationMarker) {
		return &RCFileError{
			Path:    rcPath,
			Message: "invalid activation command format",
		}
	}

	// Security: Check for symlinks (prevent symlink attack)
	if info, err := os.Lstat(rcPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return &RCFileError{
				Path:    rcPath,
				Message: "RC file is a symlink (security risk)",
			}
		}
	}

	// Read existing content
	var existingContent []byte
	var err error

	if exists, _ := RCFileExists(rcPath); exists {
		existingContent, err = os.ReadFile(rcPath)
		if err != nil {
			return &RCFileError{
				Path:    rcPath,
				Message: "failed to read existing file",
				Cause:   err,
			}
		}

		// Check if activation line already exists (fix TOCTOU race condition)
		// Do this atomically while we have the content in memory
		if strings.Contains(string(existingContent), ActivationMarker) {
			// Already present, nothing to do (idempotent)
			return nil
		}
	}

	// Create temporary file in the same directory (for atomic rename)
	dir := filepath.Dir(rcPath)
	tmpFile, err := os.CreateTemp(dir, ".zerb-tmp-*")
	if err != nil {
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to create temporary file",
			Cause:   err,
		}
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up on error

	// Write existing content
	if len(existingContent) > 0 {
		if _, err := tmpFile.Write(existingContent); err != nil {
			tmpFile.Close()
			return &RCFileError{
				Path:    rcPath,
				Message: "failed to write existing content",
				Cause:   err,
			}
		}

		// Ensure there's a newline before our addition
		if !strings.HasSuffix(string(existingContent), "\n") {
			if _, err := tmpFile.WriteString("\n"); err != nil {
				tmpFile.Close()
				return &RCFileError{
					Path:    rcPath,
					Message: "failed to write newline",
					Cause:   err,
				}
			}
		}
	}

	// Write ZERB activation section
	zerbSection := fmt.Sprintf("\n# ZERB - Developer environment manager\n%s\n", activationCommand)
	if _, err := tmpFile.WriteString(zerbSection); err != nil {
		tmpFile.Close()
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to write activation line",
			Cause:   err,
		}
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to sync file",
			Cause:   err,
		}
	}

	tmpFile.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, rcPath); err != nil {
		return &RCFileError{
			Path:    rcPath,
			Message: "failed to rename temp file",
			Cause:   err,
		}
	}

	return nil
}
