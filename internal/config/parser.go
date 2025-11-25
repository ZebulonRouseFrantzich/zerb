package config

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
	lua "github.com/yuin/gopher-lua"
)

// Configuration limits for security and resource management.
const (
	// MaxConfigSize is the maximum allowed size for a config file (10MB).
	MaxConfigSize = 10 * 1024 * 1024

	// MaxToolCount is the maximum number of tools allowed in a config.
	MaxToolCount = 1000

	// MaxConfigFileCount is the maximum number of config files allowed.
	MaxConfigFileCount = 500

	// DefaultParseTimeout is the default timeout for parsing a config (5 seconds).
	DefaultParseTimeout = 5 * time.Second
)

// Parser represents a Lua config parser with platform detection.
type Parser struct {
	detector platform.Detector
	logger   Logger
}

// NewParser creates a new config parser with the given platform detector.
// Pass nil for detector to skip platform detection.
func NewParser(detector platform.Detector) *Parser {
	return &Parser{
		detector: detector,
		logger:   defaultLogger(),
	}
}

// WithLogger returns a new Parser with the specified logger.
func (p *Parser) WithLogger(logger Logger) *Parser {
	if logger == nil {
		logger = defaultLogger()
	}
	return &Parser{
		detector: p.detector,
		logger:   logger,
	}
}

// ParseString parses a Lua config from a string.
// This is useful for testing and in-memory config generation.
//
// Security limits enforced:
//   - Config size: Maximum 10MB
//   - Parse timeout: 5 seconds (configurable via context)
//   - Resource limits: Call stack depth, memory usage
func (p *Parser) ParseString(ctx context.Context, luaCode string) (*Config, error) {
	p.logger.Debug("parsing config", "size", len(luaCode))
	start := time.Now()
	defer func() {
		p.logger.Debug("parse complete", "duration", time.Since(start))
	}()

	// Validate input size before parsing
	if len(luaCode) > MaxConfigSize {
		p.logger.Error("config file too large", "size", len(luaCode), "max", MaxConfigSize)
		return nil, &ParseError{
			Message: "config file too large",
			Detail:  fmt.Sprintf("size %d bytes exceeds maximum %d bytes", len(luaCode), MaxConfigSize),
		}
	}

	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before parsing: %w", err)
	}

	// Create timeout context if the provided context doesn't have a deadline
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultParseTimeout)
		defer cancel()
	}

	L := newSandboxedVM()
	defer L.Close()

	// Set context on Lua VM for timeout enforcement
	L.SetContext(ctx)

	// Detect platform and inject platform table
	if p.detector != nil {
		platformInfo, err := p.detector.Detect(ctx)
		if err != nil {
			return nil, fmt.Errorf("platform detection failed: %w", err)
		}
		if err := platform.InjectPlatformTable(L, platformInfo); err != nil {
			return nil, fmt.Errorf("inject platform table: %w", err)
		}
	}

	// Execute Lua code with timeout protection
	if err := L.DoString(luaCode); err != nil {
		// Check if timeout occurred
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &ParseError{
				Message: "config parsing timeout",
				Detail:  "parsing took longer than allowed time limit (possible infinite loop)",
			}
		}
		return nil, &ParseError{
			Message: "Lua syntax error",
			Detail:  sanitizeLuaError(err),
		}
	}

	// Extract config from the Lua state
	return extractConfig(L)
}

// ParseError represents a config parsing error with friendly message.
type ParseError struct {
	Message string // User-friendly message
	Detail  string // Technical details (raw Lua error)
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Message, e.Detail)
}

// extractConfig extracts the config from a Lua state.
// It expects a global "zerb" table with the config structure.
func extractConfig(L *lua.LState) (*Config, error) {
	zerbTable := L.GetGlobal(luaGlobalZerb)
	if zerbTable.Type() != lua.LTTable {
		return nil, &ParseError{
			Message: "missing or invalid 'zerb' table",
			Detail:  fmt.Sprintf("expected table, got %s", zerbTable.Type()),
		}
	}

	config := &Config{}
	table := zerbTable.(*lua.LTable)

	// Extract meta
	if metaVal := table.RawGetString(luaFieldMeta); metaVal.Type() == lua.LTTable {
		meta, err := extractMeta(metaVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Meta = meta
	}

	// Extract tools
	if toolsVal := table.RawGetString(luaFieldTools); toolsVal.Type() == lua.LTTable {
		tools, err := extractTools(toolsVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Tools = tools
	}

	// Extract configs
	if configsVal := table.RawGetString(luaFieldConfigs); configsVal.Type() == lua.LTTable {
		configs, err := extractConfigFiles(configsVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Configs = configs
	}

	// Extract git
	if gitVal := table.RawGetString(luaFieldGit); gitVal.Type() == lua.LTTable {
		git, err := extractGitConfig(gitVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Git = git
	}

	// Extract options (or config, depending on schema)
	if optionsVal := table.RawGetString(luaFieldConfig); optionsVal.Type() == lua.LTTable {
		options, err := extractOptions(optionsVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Options = options
	}

	// Validate the extracted config
	if err := config.Validate(); err != nil {
		return nil, &ParseError{
			Message: "config validation failed",
			Detail:  err.Error(),
		}
	}

	return config, nil
}

// extractMeta extracts metadata from a Lua table.
func extractMeta(table *lua.LTable) (Meta, error) {
	meta := Meta{}

	if nameVal := table.RawGetString(luaFieldName); nameVal.Type() == lua.LTString {
		meta.Name = nameVal.String()
	}

	if descVal := table.RawGetString(luaFieldDesc); descVal.Type() == lua.LTString {
		meta.Description = descVal.String()
	}

	return meta, nil
}

// extractTools extracts tools array from a Lua table.
// It filters out nil values from platform conditionals.
func extractTools(table *lua.LTable) ([]string, error) {
	var tools []string

	// Iterate over array elements
	table.ForEach(func(key, value lua.LValue) {
		// Skip nil values (from platform conditionals like: platform.is_linux and "tool" or nil)
		if value.Type() == lua.LTNil {
			return
		}

		// Skip non-string values
		if value.Type() != lua.LTString {
			return
		}

		tool := value.String()
		// Keep all strings, even empty ones (validation will catch them later)
		tools = append(tools, tool)
	})

	return tools, nil
}

// extractConfigFiles extracts config files array from a Lua table.
func extractConfigFiles(table *lua.LTable) ([]ConfigFile, error) {
	var configs []ConfigFile

	table.ForEach(func(key, value lua.LValue) {
		// Handle string entries (simple path)
		if value.Type() == lua.LTString {
			configs = append(configs, ConfigFile{
				Path: value.String(),
			})
			return
		}

		// Handle table entries (with options)
		if value.Type() == lua.LTTable {
			cfTable := value.(*lua.LTable)
			cf := ConfigFile{}

			// Required: path
			if pathVal := cfTable.RawGetString(luaFieldPath); pathVal.Type() == lua.LTString {
				cf.Path = pathVal.String()
			}

			// Optional: recursive
			if recVal := cfTable.RawGetString(luaFieldRecursive); recVal.Type() == lua.LTBool {
				cf.Recursive = bool(recVal.(lua.LBool))
			}

			// Optional: template
			if tmplVal := cfTable.RawGetString(luaFieldTemplate); tmplVal.Type() == lua.LTBool {
				cf.Template = bool(tmplVal.(lua.LBool))
			}

			// Optional: secrets
			if secVal := cfTable.RawGetString(luaFieldSecrets); secVal.Type() == lua.LTBool {
				cf.Secrets = bool(secVal.(lua.LBool))
			}

			// Optional: private
			if privVal := cfTable.RawGetString(luaFieldPrivate); privVal.Type() == lua.LTBool {
				cf.Private = bool(privVal.(lua.LBool))
			}

			configs = append(configs, cf)
		}
	})

	return configs, nil
}

// extractGitConfig extracts git configuration from a Lua table.
func extractGitConfig(table *lua.LTable) (GitConfig, error) {
	git := GitConfig{}

	if remoteVal := table.RawGetString(luaFieldRemote); remoteVal.Type() == lua.LTString {
		git.Remote = remoteVal.String()
	}

	if branchVal := table.RawGetString(luaFieldBranch); branchVal.Type() == lua.LTString {
		git.Branch = branchVal.String()
	}

	return git, nil
}

// extractOptions extracts options from a Lua table.
func extractOptions(table *lua.LTable) (Options, error) {
	options := Options{}

	if retentionVal := table.RawGetString(luaFieldBackupRetention); retentionVal.Type() == lua.LTNumber {
		options.BackupRetention = int(lua.LVAsNumber(retentionVal))
	}

	return options, nil
}

// sanitizeLuaError sanitizes Lua VM error messages for user display.
// It removes stack traces and internal implementation details.
func sanitizeLuaError(err error) string {
	errStr := err.Error()

	// Remove stack traces (they leak internal implementation details)
	if idx := strings.Index(errStr, "stack traceback"); idx > 0 {
		errStr = strings.TrimSpace(errStr[:idx])
	}

	// Replace internal references with user-friendly terms
	errStr = strings.ReplaceAll(errStr, "<string>", "config")
	errStr = strings.ReplaceAll(errStr, "gopher-lua", "Lua")

	return strings.TrimSpace(errStr)
}

// FormatError formats a ParseError for user display.
// In verbose mode, show the raw Lua error. Otherwise, show friendly message.
func FormatError(err error, verbose bool) string {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		if verbose {
			return fmt.Sprintf("%s\n\nDetails:\n%s", parseErr.Message, parseErr.Detail)
		}
		// Detail is already sanitized, just return it
		return fmt.Sprintf("%s: %s", parseErr.Message, parseErr.Detail)
	}
	return err.Error()
}
