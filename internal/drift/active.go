package drift

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// maxCacheEntries is the maximum number of entries in the version cache
const maxCacheEntries = 100

// Default timeouts for subprocess operations
const (
	defaultVersionTimeout = 3 * time.Second
)

// getVersionTimeout returns the version detection timeout from env or default
func getVersionTimeout() time.Duration {
	if val := os.Getenv("ZERB_VERSION_TIMEOUT"); val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultVersionTimeout
}

// QueryActive queries the active environment for tools in PATH
func QueryActive(ctx context.Context, toolNames []string, forceRefresh bool) ([]Tool, error) {
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
		version, err := DetectVersionCached(ctx, resolvedPath, forceRefresh)
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
func DetectVersionCached(ctx context.Context, binaryPath string, forceRefresh bool) (string, error) {
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
	version, err := DetectVersion(ctx, binaryPath)
	if err != nil {
		return "", err
	}

	// Update cache and prune expired entries if needed
	versionCache.Lock()
	versionCache.entries[binaryPath] = versionCacheEntry{
		version:   version,
		timestamp: time.Now(),
	}

	// Prune expired entries or enforce max size limit
	if len(versionCache.entries) > maxCacheEntries {
		pruneExpiredCacheEntries()
	}
	versionCache.Unlock()

	return version, nil
}

// pruneExpiredCacheEntries removes expired entries from the version cache
// Must be called with versionCache.Lock() held
func pruneExpiredCacheEntries() {
	now := time.Now()
	for path, entry := range versionCache.entries {
		if now.Sub(entry.timestamp) >= versionCacheTTL {
			delete(versionCache.entries, path)
		}
	}

	// If still over limit after pruning expired entries, remove oldest entries
	if len(versionCache.entries) > maxCacheEntries {
		// Find and remove oldest entries
		type pathWithTime struct {
			path string
			time time.Time
		}
		var entries []pathWithTime
		for path, entry := range versionCache.entries {
			entries = append(entries, pathWithTime{path, entry.timestamp})
		}
		// Sort by timestamp (oldest first)
		for i := 0; i < len(entries)-1; i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].time.After(entries[j].time) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		// Remove oldest entries until we're under the limit
		toRemove := len(entries) - maxCacheEntries
		for i := 0; i < toRemove; i++ {
			delete(versionCache.entries, entries[i].path)
		}
	}
}

// DetectVersion detects the version of a binary by executing it
// Tries --version flag first, then -v as fallback
// This function does NOT use caching - use DetectVersionCached for cached lookups
// Uses context with timeout to prevent hanging on misbehaving tools
func DetectVersion(ctx context.Context, binaryPath string) (string, error) {
	timeout := getVersionTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try --version first (most common)
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := cmd.CombinedOutput() // Capture both stdout and stderr
	if err == nil {
		version, err := ExtractVersion(string(output))
		if err == nil {
			return version, nil
		}
	}

	// Try -v as fallback
	cmd = exec.CommandContext(ctx, binaryPath, "-v")
	output, err = cmd.CombinedOutput() // Capture both stdout and stderr
	if err == nil {
		version, err := ExtractVersion(string(output))
		if err == nil {
			return version, nil
		}
	}

	return "", fmt.Errorf("failed to detect version for %s", binaryPath)
}
