package drift

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// versionCacheEntry stores a cached version with timestamp
type versionCacheEntry struct {
	version   string
	timestamp time.Time
}

// versionCache is a global cache for version detection results
var versionCache = struct {
	sync.RWMutex
	entries map[string]versionCacheEntry
}{
	entries: make(map[string]versionCacheEntry),
}

// versionCacheTTL is the time-to-live for cached version entries (5 minutes)
const versionCacheTTL = 5 * time.Minute

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

		// Detect version (with caching)
		version, err := DetectVersionCached(resolvedPath, false)
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

// DetectVersionCached detects the version of a binary with caching
// Uses a 5-minute TTL cache to avoid repeated subprocess calls
// Set forceRefresh to true to bypass the cache
func DetectVersionCached(binaryPath string, forceRefresh bool) (string, error) {
	// Check cache unless force refresh is requested
	if !forceRefresh {
		versionCache.RLock()
		if entry, exists := versionCache.entries[binaryPath]; exists {
			// Check if cache entry is still valid (within TTL)
			if time.Since(entry.timestamp) < versionCacheTTL {
				versionCache.RUnlock()
				return entry.version, nil
			}
		}
		versionCache.RUnlock()
	}

	// Cache miss or expired - detect version
	version, err := DetectVersion(binaryPath)
	if err != nil {
		return "", err
	}

	// Update cache
	versionCache.Lock()
	versionCache.entries[binaryPath] = versionCacheEntry{
		version:   version,
		timestamp: time.Now(),
	}
	versionCache.Unlock()

	return version, nil
}

// DetectVersion detects the version of a binary by executing it
// Tries --version flag first, then -v as fallback
// This function does NOT use caching - use DetectVersionCached for cached lookups
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
