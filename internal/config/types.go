// Package config provides Lua configuration parsing, generation, and management
// for ZERB's declarative environment management.
//
// It uses gopher-lua for safe, sandboxed Lua execution with platform detection
// integration. Configs are versioned using Git-tracked timestamped snapshots.
package config

import "time"

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
	// Tool validation
	for _, tool := range c.Tools {
		if err := validateToolString(tool); err != nil {
			return err
		}
	}

	// Config file validation
	for _, cf := range c.Configs {
		if cf.Path == "" {
			return &ValidationError{Field: "configs", Message: "path cannot be empty"}
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

// validateToolString validates a tool string format (name@version or backend:name).
func validateToolString(tool string) error {
	if tool == "" {
		return &ValidationError{Field: "tools", Message: "tool string cannot be empty"}
	}
	// Basic validation - tool must have content
	// More specific validation (e.g., @version format) can be added post-MVP
	return nil
}
