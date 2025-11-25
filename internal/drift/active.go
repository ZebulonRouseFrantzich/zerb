package drift

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// VersionCache provides caching for version detection results.
// Implementations must be safe for concurrent use.
type VersionCache interface {
	// Get returns the cached version for a binary path, or ("", false) if not cached or expired.
	Get(binaryPath string) (version string, ok bool)
	// Set stores a version in the cache for the given binary path.
	Set(binaryPath string, version string)
}

// versionCacheEntry stores a cached version with timestamp
type versionCacheEntry struct {
	version   string
	timestamp time.Time
}

// InMemoryVersionCache is a thread-safe in-memory implementation of VersionCache.
type InMemoryVersionCache struct {
	mu         sync.RWMutex
	entries    map[string]versionCacheEntry
	ttl        time.Duration
	maxEntries int
}

// NewVersionCache creates a new in-memory version cache with default settings.
func NewVersionCache() *InMemoryVersionCache {
	return NewVersionCacheWithOptions(versionCacheTTL, maxCacheEntries)
}

// NewVersionCacheWithOptions creates a new in-memory version cache with custom TTL and max entries.
func NewVersionCacheWithOptions(ttl time.Duration, maxEntries int) *InMemoryVersionCache {
	return &InMemoryVersionCache{
		entries:    make(map[string]versionCacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// Get returns the cached version for a binary path, or ("", false) if not cached or expired.
func (c *InMemoryVersionCache) Get(binaryPath string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, exists := c.entries[binaryPath]; exists {
		if time.Since(entry.timestamp) < c.ttl {
			return entry.version, true
		}
	}
	return "", false
}

// Set stores a version in the cache for the given binary path.
func (c *InMemoryVersionCache) Set(binaryPath string, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[binaryPath] = versionCacheEntry{
		version:   version,
		timestamp: time.Now(),
	}

	// Prune if needed
	if len(c.entries) > c.maxEntries {
		c.pruneExpiredEntries()
	}
}

// pruneExpiredEntries removes expired entries from the cache.
// Must be called with c.mu.Lock() held.
func (c *InMemoryVersionCache) pruneExpiredEntries() {
	now := time.Now()
	for path, entry := range c.entries {
		if now.Sub(entry.timestamp) >= c.ttl {
			delete(c.entries, path)
		}
	}

	// If still over limit after pruning expired entries, remove oldest entries
	if len(c.entries) > c.maxEntries {
		type pathWithTime struct {
			path string
			time time.Time
		}
		var entries []pathWithTime
		for path, entry := range c.entries {
			entries = append(entries, pathWithTime{path, entry.timestamp})
		}
		// Sort by timestamp (oldest first) using O(n log n) sort
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].time.Before(entries[j].time)
		})
		// Remove oldest entries until we're under the limit
		toRemove := len(entries) - c.maxEntries
		for i := 0; i < toRemove; i++ {
			delete(c.entries, entries[i].path)
		}
	}
}

// versionCacheTTL is the time-to-live for cached version entries (5 minutes)
const versionCacheTTL = 5 * time.Minute

// maxCacheEntries is the maximum number of entries in the version cache
const maxCacheEntries = 100

// Default timeouts for subprocess operations
const (
	defaultVersionTimeout = 3 * time.Second
)

// defaultVersionCache is the package-level default cache for backwards compatibility.
// New code should prefer passing a VersionCache explicitly.
var defaultVersionCache = NewVersionCache()

// getVersionTimeout returns the version detection timeout from env or default
func getVersionTimeout() time.Duration {
	if val := os.Getenv("ZERB_VERSION_TIMEOUT"); val != "" {
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultVersionTimeout
}

// QueryActive queries the active environment for tools in PATH.
// Uses the default package-level cache for version detection.
func QueryActive(ctx context.Context, toolNames []string, forceRefresh bool) ([]Tool, error) {
	return QueryActiveWithCache(ctx, toolNames, forceRefresh, defaultVersionCache)
}

// QueryActiveWithCache queries the active environment for tools in PATH using the provided cache.
func QueryActiveWithCache(ctx context.Context, toolNames []string, forceRefresh bool, cache VersionCache) ([]Tool, error) {
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
		version, err := DetectVersionWithCache(ctx, resolvedPath, forceRefresh, cache)
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

// DetectVersionCached detects the version of a binary with caching.
// Uses the default package-level cache. For testing, use DetectVersionWithCache.
func DetectVersionCached(ctx context.Context, binaryPath string, forceRefresh bool) (string, error) {
	return DetectVersionWithCache(ctx, binaryPath, forceRefresh, defaultVersionCache)
}

// DetectVersionWithCache detects the version of a binary using the provided cache.
// Uses a TTL cache to avoid repeated subprocess calls.
// Set forceRefresh to true to bypass the cache.
func DetectVersionWithCache(ctx context.Context, binaryPath string, forceRefresh bool, cache VersionCache) (string, error) {
	// Check cache unless force refresh is requested
	if !forceRefresh && cache != nil {
		if version, ok := cache.Get(binaryPath); ok {
			return version, nil
		}
	}

	// Cache miss or expired - detect version
	version, err := DetectVersion(ctx, binaryPath)
	if err != nil {
		return "", err
	}

	// Update cache
	if cache != nil {
		cache.Set(binaryPath, version)
	}

	return version, nil
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

// ResetDefaultCache clears the default package-level cache.
// This is primarily useful for testing.
func ResetDefaultCache() {
	defaultVersionCache = NewVersionCache()
}
