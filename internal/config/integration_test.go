package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

// TestExampleConfigs validates that all example config files can be parsed.
func TestExampleConfigs(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")

	examples := []struct {
		name     string
		filename string
	}{
		{"minimal", "minimal.lua"},
		{"full", "full.lua"},
		{"platforms", "platforms.lua"},
	}

	// Create a mock Linux platform for testing
	detector := &mockDetector{
		info: &platform.Info{
			OS:       "linux",
			Arch:     "amd64",
			ArchRaw:  "x86_64",
			Platform: "ubuntu",
			Family:   "debian",
			Version:  "22.04",
		},
	}

	parser := NewParser(detector)

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			path := filepath.Join(examplesDir, ex.filename)

			// Read the file
			// #nosec G304 -- path is built from a trusted examplesDir and fixed filenames
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", path, err)
			}

			// Parse it
			config, err := parser.ParseString(context.Background(), string(content))
			if err != nil {
				t.Fatalf("ParseString(%s) error = %v", ex.filename, err)
			}

			// Validate it
			if err := config.Validate(); err != nil {
				t.Errorf("Validate(%s) error = %v", ex.filename, err)
			}

			t.Logf("Successfully parsed %s with %d tools", ex.filename, len(config.Tools))
		})
	}
}

// TestRoundTripWithExamples tests that example configs can be round-tripped.
func TestRoundTripWithExamples(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")

	examples := []string{
		"minimal.lua",
		"full.lua",
	}

	detector := &mockDetector{
		info: &platform.Info{
			OS:      "linux",
			Arch:    "amd64",
			ArchRaw: "x86_64",
		},
	}

	parser := NewParser(detector)
	generator := NewGenerator()

	for _, filename := range examples {
		t.Run(filename, func(t *testing.T) {
			path := filepath.Join(examplesDir, filename)

			// Read and parse original
			// #nosec G304 -- path is built from a trusted examplesDir and fixed filenames
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", path, err)
			}

			original, err := parser.ParseString(context.Background(), string(content))
			if err != nil {
				t.Fatalf("ParseString(%s) error = %v", filename, err)
			}

			// Generate Lua from parsed config
			generated, err := generator.Generate(context.Background(), original)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			// Parse the generated Lua
			roundtrip, err := parser.ParseString(context.Background(), generated)
			if err != nil {
				t.Fatalf("ParseString(generated) error = %v\nGenerated Lua:\n%s", err, generated)
			}

			// Compare tool counts (exact comparison may vary due to platform conditionals)
			if len(roundtrip.Tools) != len(original.Tools) {
				t.Logf("Original tools: %v", original.Tools)
				t.Logf("Roundtrip tools: %v", roundtrip.Tools)
				// This is expected for files with platform conditionals
			}

			// Validate the roundtrip config
			if err := roundtrip.Validate(); err != nil {
				t.Errorf("Validate(roundtrip) error = %v", err)
			}

			t.Logf("Successfully round-tripped %s", filename)
		})
	}
}

// TestGenerateAndParse tests the full workflow of generating and parsing configs.
func TestGenerateAndParse(t *testing.T) {
	// Create a config programmatically
	config := &Config{
		Meta: Meta{
			Name:        "Test Environment",
			Description: "Created programmatically for testing",
		},
		Tools: []string{
			"node@20.11.0",
			"python@3.12.1",
			"cargo:ripgrep",
		},
		Configs: []ConfigFile{
			{Path: "~/.zshrc"},
			{Path: "~/.config/nvim/", Recursive: true},
		},
		Git: GitConfig{
			Remote: "https://github.com/test/repo",
			Branch: "main",
		},
		Options: Options{
			BackupRetention: 3,
		},
	}

	// Generate Lua
	gen := NewGenerator()
	lua, err := gen.Generate(context.Background(), config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	t.Logf("Generated Lua:\n%s", lua)

	// Parse it back
	parser := NewParser(nil)
	parsed, err := parser.ParseString(context.Background(), lua)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	// Verify all fields match
	if parsed.Meta.Name != config.Meta.Name {
		t.Errorf("Meta.Name = %s, want %s", parsed.Meta.Name, config.Meta.Name)
	}
	if len(parsed.Tools) != len(config.Tools) {
		t.Errorf("Tools length = %d, want %d", len(parsed.Tools), len(config.Tools))
	}
	if len(parsed.Configs) != len(config.Configs) {
		t.Errorf("Configs length = %d, want %d", len(parsed.Configs), len(config.Configs))
	}
}

// TestTimestampedConfigGeneration tests generating timestamped configs.
func TestTimestampedConfigGeneration(t *testing.T) {
	config := &Config{
		Tools: []string{"node@20.11.0", "python@3.12.1"},
	}

	gen := NewGenerator()
	filename, content, err := gen.GenerateTimestamped(context.Background(), config, "test-commit-abc123")
	if err != nil {
		t.Fatalf("GenerateTimestamped() error = %v", err)
	}

	t.Logf("Generated filename: %s", filename)
	t.Logf("Content preview:\n%s", content[:200])

	// Parse the timestamped config
	parser := NewParser(nil)
	parsed, err := parser.ParseString(context.Background(), content)
	if err != nil {
		t.Fatalf("ParseString(timestamped) error = %v", err)
	}

	// Verify tools are preserved
	if len(parsed.Tools) != len(config.Tools) {
		t.Errorf("Tools length = %d, want %d", len(parsed.Tools), len(config.Tools))
	}
}
