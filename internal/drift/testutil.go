package drift

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CreateMockBinary creates a shell script that reports a specific version
func CreateMockBinary(t *testing.T, dir, name, version string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ] || [ "$1" = "-v" ]; then
    echo "%s version %s"
else
    echo "Mock %s binary"
fi
`, name, version, name)

	err := os.WriteFile(path, []byte(script), 0755)
	if err != nil {
		t.Fatalf("failed to create mock binary: %v", err)
	}

	return path
}

// SetupTestPATH creates a test PATH with mock binaries
func SetupTestPATH(t *testing.T, binaries map[string]string) string {
	t.Helper()

	tmpDir := t.TempDir()

	for name, version := range binaries {
		CreateMockBinary(t, tmpDir, name, version)
	}

	// Return new PATH prefix
	return tmpDir + ":" + os.Getenv("PATH")
}

// MockMiseOutput returns mock JSON output for mise ls
func MockMiseOutput(tools []Tool) string {
	var entries []string
	for _, tool := range tools {
		entry := fmt.Sprintf(`{
			"name": "%s",
			"version": "%s",
			"install_path": "%s"
		}`, tool.Name, tool.Version, tool.Path)
		entries = append(entries, entry)
	}
	return "[" + strings.Join(entries, ",") + "]"
}
