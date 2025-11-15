package drift

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MiseTool represents a tool from mise ls --json output
type MiseTool struct {
	Version     string `json:"version"`
	InstallPath string `json:"install_path"`
	Source      struct {
		Type string `json:"type"`
		Path string `json:"path"`
	} `json:"source"`
}

// Default timeout for mise operations
const defaultMiseTimeout = 2 * time.Minute

// getMiseTimeout returns the mise operation timeout from env or default
func getMiseTimeout() time.Duration {
	if val := os.Getenv("ZERB_MISE_TIMEOUT"); val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultMiseTimeout
}

// QueryManaged queries mise for ZERB-installed tools
func QueryManaged(ctx context.Context, zerbDir string) ([]Tool, error) {
	misePath := filepath.Join(zerbDir, "bin", "mise")

	// Execute mise ls --json to get all installed tools
	jsonOutput, err := executeMiseCommand(ctx, misePath, zerbDir, "ls", "--json")
	if err != nil {
		return nil, fmt.Errorf("execute mise ls --json: %w", err)
	}

	// Parse JSON output
	miseTools, err := parseMiseJSON(jsonOutput)
	if err != nil {
		return nil, err
	}

	// Execute mise ls --current to get active versions
	currentOutput, err := executeMiseCommand(ctx, misePath, zerbDir, "ls", "--current")
	if err != nil {
		return nil, fmt.Errorf("execute mise ls --current: %w", err)
	}

	// Parse current versions
	currentVersions, err := parseMiseCurrent(currentOutput)
	if err != nil {
		return nil, err
	}

	// Build tool list from current versions
	var tools []Tool
	for toolName, version := range currentVersions {
		// Find the matching tool in the JSON output
		miseToolVersions, exists := miseTools[toolName]
		if !exists || len(miseToolVersions) == 0 {
			continue
		}

		// Find the tool with matching version
		var installPath string
		for _, mt := range miseToolVersions {
			if mt.Version == version {
				installPath = mt.InstallPath
				break
			}
		}

		tools = append(tools, Tool{
			Name:    toolName,
			Version: version,
			Path:    installPath,
		})
	}

	return tools, nil
}

// executeMiseCommand executes a mise command with proper isolation
func executeMiseCommand(ctx context.Context, misePath, zerbDir string, args ...string) (string, error) {
	timeout := getMiseTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, misePath, args...)

	// Build minimal clean environment for mise (instead of inheriting all env vars)
	// Only include variables that mise actually needs
	cmd.Env = []string{
		"MISE_CONFIG_FILE=" + filepath.Join(zerbDir, "mise/config.toml"),
		"MISE_DATA_DIR=" + filepath.Join(zerbDir, "mise"),
		"MISE_CACHE_DIR=" + filepath.Join(zerbDir, "cache/mise"),
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
		"TERM=" + os.Getenv("TERM"), // For better output formatting
	}

	output, err := cmd.CombinedOutput() // Capture both stdout and stderr
	if err != nil {
		return "", fmt.Errorf("%w (output: %s)", err, string(output))
	}

	return string(output), nil
}

// parseMiseJSON parses mise ls --json output
// Format: {"tool_name": [{"version": "1.0.0", "install_path": "...", ...}]}
func parseMiseJSON(jsonOutput string) (map[string][]MiseTool, error) {
	var result map[string][]MiseTool
	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
		return nil, fmt.Errorf("parse mise JSON: %w", err)
	}

	return result, nil
}

// parseMiseCurrent parses mise ls --current output
// Format: "tool_name    version\n..."
func parseMiseCurrent(output string) (map[string]string, error) {
	result := make(map[string]string)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split on whitespace (can be tabs or spaces)
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			toolName := fields[0]
			version := fields[1]
			result[toolName] = version
		}
	}

	return result, nil
}

// IsZERBManaged checks if a binary path is managed by ZERB
func IsZERBManaged(binaryPath, zerbDir string) bool {
	installsDir := filepath.Join(zerbDir, "installs")
	return strings.HasPrefix(binaryPath, installsDir)
}
