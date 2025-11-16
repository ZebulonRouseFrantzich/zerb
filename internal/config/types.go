// Package config provides Lua configuration parsing, generation, and management
// for ZERB's declarative environment management.
//
// It uses gopher-lua for safe, sandboxed Lua execution with platform detection
// integration. Configs are versioned using Git-tracked timestamped snapshots.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Config represents the complete ZERB configuration.
// This matches the Lua schema defined in the design document.
type Config struct {
	// Metadata about the configuration
	Meta Meta `json:"meta,omitempty"`

	// Tools to install via mise (with exact versions)
	Tools []string `json:"tools,omitempty"`

	// Configuration files to manage via chezmoi
	Configs []ConfigFile `json:"configs,omitempty"`

	// Git repository settings
	Git GitConfig `json:"git,omitempty"`

	// ZERB configuration options
	Options Options `json:"options,omitempty"`
}

// Meta contains metadata about the configuration.
type Meta struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ConfigFile represents a configuration file or directory to manage.
type ConfigFile struct {
	// Path to the config file or directory (supports ~)
	Path string `json:"path"`

	// Recursive directory management (for directories)
	Recursive bool `json:"recursive,omitempty"`

	// Template processing with chezmoi templates
	Template bool `json:"template,omitempty"`

	// Secrets management (GPG encryption)
	Secrets bool `json:"secrets,omitempty"`

	// Private file (chmod 600)
	Private bool `json:"private,omitempty"`
}

// GitConfig contains Git repository settings for config versioning.
type GitConfig struct {
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// Options contains ZERB configuration options.
type Options struct {
	// Number of timestamped config backups to retain
	BackupRetention int `json:"backup_retention,omitempty"`
}

// Metadata contains internal metadata for timestamped configs.
// This is stored in the Lua file but not exposed to users.
type Metadata struct {
	Version   int       `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	GitCommit string    `json:"git_commit,omitempty"`
}

// ConfigVersion represents a timestamped config snapshot.
type ConfigVersion struct {
	Timestamp time.Time
	Filename  string
	IsActive  bool
}

// Override represents machine-specific configuration overrides.
// Overrides are merged with the baseline config at runtime.
type Override struct {
	// Tools to add to the baseline
	ToolsAdd []string `json:"tools_add,omitempty"`

	// Tools to remove from the baseline (by name, not version)
	ToolsRemove []string `json:"tools_remove,omitempty"`

	// Tool versions to override (map[tool_name]new_version)
	ToolsOverride map[string]string `json:"tools_override,omitempty"`

	// Config file overrides (deep merge with baseline)
	ConfigOverrides map[string]interface{} `json:"config_overrides,omitempty"`

	// Git settings override
	Git *GitConfig `json:"git,omitempty"`

	// Options override
	Options *Options `json:"options,omitempty"`
}

// Validate performs basic validation on a Config.
func (c *Config) Validate() error {
	// Tool count validation
	if len(c.Tools) > MaxToolCount {
		return &ValidationError{
			Field:   "tools",
			Message: fmt.Sprintf("too many tools (%d), maximum is %d", len(c.Tools), MaxToolCount),
		}
	}

	// Tool validation
	for i, tool := range c.Tools {
		if err := validateToolString(tool); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("tools[%d]", i),
				Message: err.Error(),
			}
		}
	}

	// Config file count validation
	if len(c.Configs) > MaxConfigFileCount {
		return &ValidationError{
			Field:   "configs",
			Message: fmt.Sprintf("too many config files (%d), maximum is %d", len(c.Configs), MaxConfigFileCount),
		}
	}

	// Config file validation
	for i, cf := range c.Configs {
		if cf.Path == "" {
			return &ValidationError{Field: fmt.Sprintf("configs[%d]", i), Message: "path cannot be empty"}
		}
		if err := validateConfigPath(cf.Path); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("configs[%d].path", i),
				Message: err.Error(),
			}
		}
	}

	// Git config validation
	if c.Git.Remote != "" {
		if err := validateGitRemote(c.Git.Remote); err != nil {
			return &ValidationError{Field: "git.remote", Message: err.Error()}
		}
	}

	return nil
}

// ValidationError represents a config validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return "config validation failed for " + e.Field + ": " + e.Message
	}
	return "config validation failed: " + e.Message
}

// toolStringPattern matches valid tool strings: name@version, backend:name, backend:name@version
var toolStringPattern = regexp.MustCompile(`^([a-z0-9_-]+:)?[a-z0-9_/-]+(@[a-z0-9._-]+)?$`)

// validateToolString validates a tool string format (name@version or backend:name).
func validateToolString(tool string) error {
	if tool == "" {
		return fmt.Errorf("tool string cannot be empty")
	}

	// Check length
	if len(tool) > 256 {
		return fmt.Errorf("tool string too long (%d chars, max 256)", len(tool))
	}

	// Validate format
	if !toolStringPattern.MatchString(tool) {
		return fmt.Errorf("invalid tool string format: %q (expected: name@version or backend:name)", tool)
	}

	return nil
}

// NormalizeConfigPath normalizes a config path to a canonical form for duplicate detection.
// It expands tilde, resolves symlinks, and cleans the path.
// Returns the normalized absolute path or an error if the path is invalid.
func NormalizeConfigPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Expand tilde
	var absPath string
	if strings.HasPrefix(path, "~/") {
		absPath = filepath.Join(home, path[2:])
	} else if path == "~" {
		absPath = home
	} else if filepath.IsAbs(path) {
		absPath = path
	} else {
		return "", fmt.Errorf("must be absolute or start with ~/")
	}

	// Clean and resolve symlinks
	absPath = filepath.Clean(absPath)
	evalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist - validate parent directory instead
			parentDir := filepath.Dir(absPath)
			parentEval, parentErr := filepath.EvalSymlinks(parentDir)
			if parentErr != nil {
				// Parent doesn't exist or has resolution issues - use cleaned path
				// This is acceptable for NormalizeConfigPath as it's used for duplicate detection
				// and the path will be validated separately by validateConfigPath
				return absPath, nil
			}
			// Use the resolved parent with the original filename
			absPath = filepath.Join(parentEval, filepath.Base(absPath))
		} else {
			// Other error (permission denied, symlink loop, etc.) - use cleaned path
			// Similar reasoning as above
			return absPath, nil
		}
	} else {
		// Path exists, use resolved canonical path
		absPath = evalPath
	}

	return absPath, nil
}

// validateConfigPath validates a config file path for security.
// It prevents path traversal attacks and restricts to home directory.
// Uses canonical path checking with symlink resolution to prevent escapes.
func validateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	homeAbs := filepath.Clean(home)

	// Expand and normalize the path
	var absPath string
	if strings.HasPrefix(path, "~/") {
		absPath = filepath.Join(home, path[2:])
	} else if path == "~" {
		absPath = home
	} else if filepath.IsAbs(path) {
		absPath = path
	} else {
		return fmt.Errorf("must be absolute or start with ~/")
	}

	// Clean the path
	absPath = filepath.Clean(absPath)

	// Try to resolve symlinks for canonical path (allow non-existent paths)
	evalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist - try to validate parent directory instead
			parentDir := filepath.Dir(absPath)
			parentEval, parentErr := filepath.EvalSymlinks(parentDir)
			if parentErr != nil {
				if os.IsNotExist(parentErr) {
					// Parent also doesn't exist - this is OK for validation
					// The path will be checked for existence at add time
					// For now, just use the cleaned path
					absPath = filepath.Clean(absPath)
				} else {
					// Other error (permission denied, symlink loop, etc.)
					return fmt.Errorf("cannot evaluate parent directory: %w", parentErr)
				}
			} else {
				// Use the resolved parent with the original filename
				absPath = filepath.Join(parentEval, filepath.Base(absPath))
			}
		} else {
			// Other error (permission denied, symlink loop, etc.)
			return fmt.Errorf("cannot evaluate path: %w", err)
		}
	} else {
		// Path exists, use resolved canonical path
		absPath = evalPath
	}

	// Verify the path is within home directory
	// Check prefix carefully - must have separator or be exact match
	if !strings.HasPrefix(absPath, homeAbs+string(filepath.Separator)) && absPath != homeAbs {
		return fmt.Errorf("absolute paths outside home directory not allowed: %s", path)
	}

	// Use filepath.Rel to verify no directory traversal
	// This is the secure way to check - it computes the relative path
	// and if it starts with "..", then there's traversal happening
	rel, err := filepath.Rel(homeAbs, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	return nil
}

// validateGitRemote validates a Git remote URL.
// Supports both HTTPS and SSH formats.
func validateGitRemote(remote string) error {
	if remote == "" {
		return fmt.Errorf("git remote cannot be empty")
	}

	// Support SSH format: git@github.com:user/repo.git
	if strings.HasPrefix(remote, "git@") {
		parts := strings.Split(remote, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid SSH git URL format")
		}
		return nil
	}

	// HTTPS format
	u, err := url.Parse(remote)
	if err != nil {
		return fmt.Errorf("invalid git URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("git URL must use https:// or http:// scheme (got: %s)", u.Scheme)
	}

	return nil
}
