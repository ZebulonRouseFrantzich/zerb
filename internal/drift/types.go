// Package drift provides drift detection functionality for ZERB.
// It compares three sources of truth: baseline config, ZERB-managed tools, and active environment.
package drift

// DriftType represents the type of drift detected
type DriftType int

const (
	DriftOK DriftType = iota
	DriftVersionMismatch
	DriftMissing
	DriftExtra
	DriftExternalOverride
	DriftManagedButNotActive
	DriftVersionUnknown
)

// String returns human-readable drift type name
func (d DriftType) String() string {
	switch d {
	case DriftOK:
		return "OK"
	case DriftVersionMismatch:
		return "VERSION_MISMATCH"
	case DriftMissing:
		return "MISSING"
	case DriftExtra:
		return "EXTRA"
	case DriftExternalOverride:
		return "EXTERNAL_OVERRIDE"
	case DriftManagedButNotActive:
		return "MANAGED_BUT_NOT_ACTIVE"
	case DriftVersionUnknown:
		return "VERSION_UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

// Tool represents a tool with version and location
type Tool struct {
	Name    string
	Version string
	Path    string
}

// ToolSpec represents a parsed tool specification
type ToolSpec struct {
	Backend string // "cargo", "npm", "ubi", "" (core)
	Name    string // Normalized tool name
	Version string // Exact version or "latest"
}

// DriftResult represents a single drift detection result
type DriftResult struct {
	Tool            string
	DriftType       DriftType
	BaselineVersion string
	ManagedVersion  string
	ActiveVersion   string
	ActivePath      string
}
