package drift

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// QueryActive queries the active environment for tools in PATH
func QueryActive(toolNames []string) ([]Tool, error) {
	var tools []Tool

	for _, name := range toolNames {
		// Find tool in PATH
		path, err := exec.LookPath(name)
		if err != nil {
			// Tool not found in PATH, skip
			continue
		}

		// Resolve symlinks to get actual binary path
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			// If symlink resolution fails, use original path
			resolvedPath = path
		}

		// Detect version
		version, err := DetectVersion(resolvedPath)
		if err != nil {
			// Mark as unknown if version detection fails
			version = "unknown"
		}

		tools = append(tools, Tool{
			Name:    name,
			Version: version,
			Path:    resolvedPath,
		})
	}

	return tools, nil
}

// DetectVersion detects the version of a binary by executing it
// Tries --version flag first, then -v as fallback
func DetectVersion(binaryPath string) (string, error) {
	// Try --version first (most common)
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.Output()
	if err == nil {
		version, err := ExtractVersion(string(output))
		if err == nil {
			return version, nil
		}
	}

	// Try -v as fallback
	cmd = exec.Command(binaryPath, "-v")
	output, err = cmd.Output()
	if err == nil {
		version, err := ExtractVersion(string(output))
		if err == nil {
			return version, nil
		}
	}

	return "", fmt.Errorf("failed to detect version for %s", binaryPath)
}
