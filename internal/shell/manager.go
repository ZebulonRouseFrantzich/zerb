package shell

import "fmt"

// Manager orchestrates shell integration setup
type Manager struct {
	zerbDir string
}

// NewManager creates a new shell manager
func NewManager(config Config) (*Manager, error) {
	if config.ZerbDir == "" {
		return nil, fmt.Errorf("ZerbDir is required")
	}

	return &Manager{
		zerbDir: config.ZerbDir,
	}, nil
}

// SetupIntegration sets up shell integration for the user's shell
func (m *Manager) SetupIntegration(shell ShellType, opts SetupOptions) (*SetupResult, error) {
	// Validate shell
	if err := ValidateShell(shell); err != nil {
		return nil, err
	}

	// Get RC file path
	rcPath, err := GetRCFilePath(shell)
	if err != nil {
		return nil, fmt.Errorf("get RC file path: %w", err)
	}

	// Check if RC file exists
	exists, err := RCFileExists(rcPath)
	if err != nil {
		return nil, fmt.Errorf("check RC file: %w", err)
	}

	// Create RC file if it doesn't exist
	if !exists {
		if err := CreateRCFile(rcPath); err != nil {
			return nil, fmt.Errorf("create RC file: %w", err)
		}
	}

	// Check if activation line already exists
	hasActivation, err := HasActivationLine(rcPath)
	if err != nil {
		return nil, fmt.Errorf("check activation line: %w", err)
	}

	// If already present and not forcing, return early
	if hasActivation && !opts.Force {
		activationCmd, _ := GenerateActivationCommand(shell)
		return &SetupResult{
			Shell:             shell,
			RCFile:            rcPath,
			Added:             false,
			AlreadyPresent:    true,
			ActivationCommand: activationCmd,
		}, nil
	}

	// Generate activation command
	activationCmd, err := GenerateActivationCommand(shell)
	if err != nil {
		return nil, fmt.Errorf("generate activation command: %w", err)
	}

	// Backup RC file if requested
	var backupPath string
	if opts.Backup {
		backupPath, err = BackupRCFile(rcPath)
		if err != nil {
			return nil, fmt.Errorf("backup RC file: %w", err)
		}
	}

	// Add activation line
	if !opts.DryRun {
		if err := AddActivationLine(rcPath, activationCmd); err != nil {
			return nil, fmt.Errorf("add activation line: %w", err)
		}
	}

	return &SetupResult{
		Shell:             shell,
		RCFile:            rcPath,
		Added:             !opts.DryRun,
		AlreadyPresent:    hasActivation,
		BackupPath:        backupPath,
		ActivationCommand: activationCmd,
	}, nil
}

// DetectAndSetup detects the user's shell and sets up integration
func (m *Manager) DetectAndSetup(opts SetupOptions) (*SetupResult, error) {
	// Detect shell
	detection, err := DetectShell()
	if err != nil {
		return nil, fmt.Errorf("detect shell: %w", err)
	}

	// Check if detected shell is supported
	if !detection.Shell.IsValid() {
		return nil, &UnsupportedShellError{Shell: detection.ShellPath}
	}

	// Setup integration
	return m.SetupIntegration(detection.Shell, opts)
}
