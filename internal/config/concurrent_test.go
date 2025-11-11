package config

import (
	"context"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

// TestParser_Concurrent tests that the parser is safe for concurrent use.
func TestParser_Concurrent(t *testing.T) {
	parser := NewParser(nil)
	luaCode := `zerb = { tools = {"node@20.11.0", "python@3.12.1"} }`

	const numGoroutines = 100
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := parser.ParseString(context.Background(), luaCode)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent parse failed: %v", err)
	}
}

// TestGenerator_Concurrent tests that the generator is safe for concurrent use.
func TestGenerator_Concurrent(t *testing.T) {
	gen := NewGenerator()
	config := &Config{
		Tools: []string{"node@20.11.0", "python@3.12.1"},
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gen.Generate(context.Background(), config)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent generation failed: %v", err)
	}
}

// TestParser_ConcurrentWithPlatform tests concurrent parsing with platform detection.
func TestParser_ConcurrentWithPlatform(t *testing.T) {
	detector := &mockDetector{
		info: &platform.Info{
			OS:      "linux",
			Arch:    "amd64",
			ArchRaw: "x86_64",
		},
	}
	parser := NewParser(detector)
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
				platform.is_linux and "linux-tool" or nil,
			},
		}
	`

	const numGoroutines = 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config, err := parser.ParseString(context.Background(), luaCode)
			if err != nil {
				errors <- err
				return
			}
			// Verify the config was parsed correctly
			if len(config.Tools) < 1 {
				errors <- &ValidationError{Message: "expected at least 1 tool"}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent parse with platform failed: %v", err)
	}
}
