package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
	lua "github.com/yuin/gopher-lua"
)

// Parser represents a Lua config parser with platform detection.
type Parser struct {
	detector platform.Detector
}

// NewParser creates a new config parser with the given platform detector.
func NewParser(detector platform.Detector) *Parser {
	return &Parser{detector: detector}
}

// ParseString parses a Lua config from a string.
// This is useful for testing and in-memory config generation.
func (p *Parser) ParseString(ctx context.Context, luaCode string) (*Config, error) {
	L := newSandboxedVM()
	defer L.Close()

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

	// Execute Lua code
	if err := L.DoString(luaCode); err != nil {
		return nil, &ParseError{
			Message: "Lua syntax error",
			Detail:  err.Error(),
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
	zerbTable := L.GetGlobal("zerb")
	if zerbTable.Type() != lua.LTTable {
		return nil, &ParseError{
			Message: "missing or invalid 'zerb' table",
			Detail:  fmt.Sprintf("expected table, got %s", zerbTable.Type()),
		}
	}

	config := &Config{}
	table := zerbTable.(*lua.LTable)

	// Extract meta
	if metaVal := table.RawGetString("meta"); metaVal.Type() == lua.LTTable {
		meta, err := extractMeta(metaVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Meta = meta
	}

	// Extract tools
	if toolsVal := table.RawGetString("tools"); toolsVal.Type() == lua.LTTable {
		tools, err := extractTools(toolsVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Tools = tools
	}

	// Extract configs
	if configsVal := table.RawGetString("configs"); configsVal.Type() == lua.LTTable {
		configs, err := extractConfigFiles(configsVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Configs = configs
	}

	// Extract git
	if gitVal := table.RawGetString("git"); gitVal.Type() == lua.LTTable {
		git, err := extractGitConfig(gitVal.(*lua.LTable))
		if err != nil {
			return nil, err
		}
		config.Git = git
	}

	// Extract options (or config, depending on schema)
	if optionsVal := table.RawGetString("config"); optionsVal.Type() == lua.LTTable {
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

	if nameVal := table.RawGetString("name"); nameVal.Type() == lua.LTString {
		meta.Name = nameVal.String()
	}

	if descVal := table.RawGetString("description"); descVal.Type() == lua.LTString {
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
			if pathVal := cfTable.RawGetString("path"); pathVal.Type() == lua.LTString {
				cf.Path = pathVal.String()
			}

			// Optional: recursive
			if recVal := cfTable.RawGetString("recursive"); recVal.Type() == lua.LTBool {
				cf.Recursive = bool(recVal.(lua.LBool))
			}

			// Optional: template
			if tmplVal := cfTable.RawGetString("template"); tmplVal.Type() == lua.LTBool {
				cf.Template = bool(tmplVal.(lua.LBool))
			}

			// Optional: secrets
			if secVal := cfTable.RawGetString("secrets"); secVal.Type() == lua.LTBool {
				cf.Secrets = bool(secVal.(lua.LBool))
			}

			// Optional: private
			if privVal := cfTable.RawGetString("private"); privVal.Type() == lua.LTBool {
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

	if remoteVal := table.RawGetString("remote"); remoteVal.Type() == lua.LTString {
		git.Remote = remoteVal.String()
	}

	if branchVal := table.RawGetString("branch"); branchVal.Type() == lua.LTString {
		git.Branch = branchVal.String()
	}

	return git, nil
}

// extractOptions extracts options from a Lua table.
func extractOptions(table *lua.LTable) (Options, error) {
	options := Options{}

	if retentionVal := table.RawGetString("backup_retention"); retentionVal.Type() == lua.LTNumber {
		options.BackupRetention = int(lua.LVAsNumber(retentionVal))
	}

	return options, nil
}

// FormatError formats a ParseError for user display.
// In verbose mode, show the raw Lua error. Otherwise, show friendly message.
func FormatError(err error, verbose bool) string {
	if parseErr, ok := err.(*ParseError); ok {
		if verbose {
			return fmt.Sprintf("%s\n\nDetails:\n%s", parseErr.Message, parseErr.Detail)
		}
		// Extract the most relevant part of the error
		detail := parseErr.Detail
		if idx := strings.Index(detail, "stack traceback"); idx > 0 {
			detail = strings.TrimSpace(detail[:idx])
		}
		return fmt.Sprintf("%s: %s", parseErr.Message, detail)
	}
	return err.Error()
}
