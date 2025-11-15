package drift

import (
	"fmt"
	"regexp"
	"strings"
)

// versionRegex matches semantic versions including pre-release and build metadata
// Examples: 1.2.3, 1.2.3-beta.1, 1.2.3+build.456, 1.2.3-rc.1+build.123
var versionRegex = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?(?:\+[a-zA-Z0-9.]+)?`)

// ExtractVersion extracts semantic version from command output
// Supports standard semver format: X.Y.Z with optional pre-release and build metadata
func ExtractVersion(output string) (string, error) {
	matches := versionRegex.FindString(output)
	if matches == "" {
		return "", fmt.Errorf("no version found in output")
	}
	return matches, nil
}

// ParseToolSpec parses a tool specification into components
// Format: [backend:]name[@version]
// Examples: "node@20.11.0", "cargo:ripgrep@13.0.0", "ubi:sharkdp/bat"
func ParseToolSpec(spec string) (ToolSpec, error) {
	if spec == "" {
		return ToolSpec{}, fmt.Errorf("empty tool spec")
	}

	var backend, nameVersion string

	// Split backend if present
	if strings.Contains(spec, ":") {
		parts := strings.SplitN(spec, ":", 2)
		backend = parts[0]
		nameVersion = parts[1]
	} else {
		nameVersion = spec
	}

	// Split name and version
	var name, version string
	if strings.Contains(nameVersion, "@") {
		parts := strings.SplitN(nameVersion, "@", 2)
		name = parts[0]
		version = parts[1]
	} else {
		name = nameVersion
	}

	// Normalize name (extract binary name from repo path)
	// e.g., "sharkdp/bat" -> "bat"
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
	}

	return ToolSpec{
		Backend: backend,
		Name:    name,
		Version: version,
	}, nil
}
