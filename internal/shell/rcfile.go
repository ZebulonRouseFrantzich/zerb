package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// Create parent directory if needed
	dir := filepath.Dir(rcPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
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
		if strings.Contains(line, "zerb activate") {
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

// BackupRCFile creates a backup of the RC file
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

	// Create backup path with timestamp
	backupPath := rcPath + ".zerb-backup"

	// Write backup
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
func AddActivationLine(rcPath string, activationCommand string) error {
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
