package shell

// Environment variable names used by ZERB shell integration
const (
	// EnvZerbActive indicates ZERB is active in the current shell
	EnvZerbActive = "ZERB_ACTIVE"

	// EnvZerbDir specifies the ZERB installation directory
	EnvZerbDir = "ZERB_DIR"

	// EnvZerbDebug enables debug logging when set
	EnvZerbDebug = "ZERB_DEBUG"
)

// Activation and backup markers
const (
	// ActivationMarker is the string that must appear in activation commands
	ActivationMarker = "zerb activate"

	// BackupSuffix is the prefix for timestamped backup files
	BackupSuffix = ".zerb-backup"
)
