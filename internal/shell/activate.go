package shell

import "fmt"

// GenerateActivationCommand generates the shell activation command that users add to their RC files
// This command calls `zerb activate` which maintains abstraction from mise
func GenerateActivationCommand(shell ShellType) (string, error) {
	if err := ValidateShell(shell); err != nil {
		return "", err
	}

	switch shell {
	case ShellBash, ShellZsh:
		// For bash and zsh, use eval with command substitution
		return fmt.Sprintf(`eval "$(zerb activate %s)"`, shell), nil
	case ShellFish:
		// Fish uses pipe to source
		return fmt.Sprintf("zerb activate %s | source", shell), nil
	default:
		return "", &UnsupportedShellError{Shell: shell.String()}
	}
}

// GetMiseActivationCommand generates the internal mise activation command
// This is what `zerb activate` calls internally - NOT user-facing
func GetMiseActivationCommand(shell ShellType, miseBinaryPath string) ([]string, error) {
	if err := ValidateShell(shell); err != nil {
		return nil, err
	}

	// Build command arguments for mise activate
	// Command: /path/to/mise activate <shell>
	return []string{miseBinaryPath, "activate", shell.String()}, nil
}
