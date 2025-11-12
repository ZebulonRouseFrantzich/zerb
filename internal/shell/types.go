package shell

import "fmt"

// ShellType represents a supported shell
type ShellType string

const (
	// ShellBash represents the Bash shell
	ShellBash ShellType = "bash"
	// ShellZsh represents the Z shell
	ShellZsh ShellType = "zsh"
	// ShellFish represents the Fish shell
	ShellFish ShellType = "fish"
	// ShellUnknown represents an unknown or unsupported shell
	ShellUnknown ShellType = "unknown"
)

// String returns the string representation of the shell type
func (s ShellType) String() string {
	return string(s)
}

// IsValid returns true if the shell type is supported
func (s ShellType) IsValid() bool {
	switch s {
	case ShellBash, ShellZsh, ShellFish:
		return true
	default:
		return false
	}
}

// Config holds configuration for the shell manager
type Config struct {
	// ZerbDir is the root ZERB directory (default: ~/.config/zerb)
	ZerbDir string
}

// SetupOptions holds options for shell integration setup
type SetupOptions struct {
	// Interactive enables user prompts before making changes
	Interactive bool
	// Force skips duplicate detection and adds activation unconditionally
	Force bool
	// Backup creates a backup of the rc file before modification
	Backup bool
	// DryRun shows what would be done without making changes
	DryRun bool
}

// SetupResult contains the result of shell integration setup
type SetupResult struct {
	// Shell is the detected or specified shell type
	Shell ShellType
	// RCFile is the path to the shell's configuration file
	RCFile string
	// Added indicates if the activation line was added
	Added bool
	// AlreadyPresent indicates if activation was already configured
	AlreadyPresent bool
	// BackupPath is the path to the backup file (if created)
	BackupPath string
	// ActivationCommand is the command that was added
	ActivationCommand string
}

// DetectionResult contains the result of shell detection
type DetectionResult struct {
	// Shell is the detected shell type
	Shell ShellType
	// Method describes how the shell was detected
	Method string
	// ShellPath is the filesystem path to the shell binary
	ShellPath string
	// Confidence is the confidence level (high, medium, low)
	Confidence string
}

// ValidationError represents a shell validation error
type ValidationError struct {
	Shell   ShellType
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("shell validation error for %s: %s", e.Shell, e.Message)
}

// UnsupportedShellError represents an unsupported shell error
type UnsupportedShellError struct {
	Shell string
}

func (e *UnsupportedShellError) Error() string {
	return fmt.Sprintf("unsupported shell: %s (supported: bash, zsh, fish)", e.Shell)
}

// RCFileError represents an error with shell rc file operations
type RCFileError struct {
	Path    string
	Message string
	Cause   error
}

func (e *RCFileError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("rc file error (%s): %s: %v", e.Path, e.Message, e.Cause)
	}
	return fmt.Sprintf("rc file error (%s): %s", e.Path, e.Message)
}

func (e *RCFileError) Unwrap() error {
	return e.Cause
}
