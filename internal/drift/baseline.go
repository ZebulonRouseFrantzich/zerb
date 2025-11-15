package drift

import (
	"context"
	"fmt"
	"os"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// QueryBaseline parses the active config and returns declared tools
func QueryBaseline(configPath string) ([]ToolSpec, error) {
	// Read config file
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Parse Lua config
	parser := config.NewParser(nil) // No platform detection needed for drift
	cfg, err := parser.ParseString(context.Background(), string(content))
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Convert tool strings to ToolSpecs
	specs := make([]ToolSpec, 0, len(cfg.Tools))
	for _, toolStr := range cfg.Tools {
		spec, err := ParseToolSpec(toolStr)
		if err != nil {
			return nil, fmt.Errorf("parse tool spec %q: %w", toolStr, err)
		}
		specs = append(specs, spec)
	}

	return specs, nil
}
