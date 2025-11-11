package config

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestParser_InfiniteLoopProtection tests that infinite loops are caught by timeout.
func TestParser_InfiniteLoopProtection(t *testing.T) {
	luaCode := `
		zerb = { tools = {} }
		while true do end  -- Infinite loop
	`

	parser := NewParser(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := parser.ParseString(ctx, luaCode)
	if err == nil {
		t.Fatal("Expected timeout error for infinite loop, got nil")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// TestParser_MaxConfigSize tests that oversized configs are rejected.
func TestParser_MaxConfigSize(t *testing.T) {
	// Create a config larger than MaxConfigSize
	largeConfig := strings.Repeat("a", MaxConfigSize+1)

	parser := NewParser(nil)
	_, err := parser.ParseString(context.Background(), largeConfig)
	if err == nil {
		t.Fatal("Expected error for oversized config, got nil")
	}

	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("Expected 'too large' error, got: %v", err)
	}
}

// TestParser_MaxToolCount tests that configs with too many tools are rejected.
func TestParser_MaxToolCount(t *testing.T) {
	// Create config with more than MaxToolCount tools
	var tools strings.Builder
	tools.WriteString("zerb = {\n  tools = {\n")
	for i := 0; i < MaxToolCount+10; i++ {
		tools.WriteString("    \"tool")
		tools.WriteString(strings.Repeat("a", 10))
		tools.WriteString("@1.0.0\",\n")
	}
	tools.WriteString("  },\n}")

	parser := NewParser(nil)
	_, err := parser.ParseString(context.Background(), tools.String())
	if err == nil {
		t.Fatal("Expected error for too many tools, got nil")
	}

	if !strings.Contains(err.Error(), "too many tools") {
		t.Errorf("Expected 'too many tools' error, got: %v", err)
	}
}

// TestParser_MaxConfigFileCount tests that configs with too many files are rejected.
func TestParser_MaxConfigFileCount(t *testing.T) {
	// Create config with more than MaxConfigFileCount files
	var configs strings.Builder
	configs.WriteString("zerb = {\n  tools = {},\n  configs = {\n")
	for i := 0; i < MaxConfigFileCount+10; i++ {
		configs.WriteString("    \"~/")
		configs.WriteString(strings.Repeat("f", 10))
		configs.WriteString("\",\n")
	}
	configs.WriteString("  },\n}")

	parser := NewParser(nil)
	_, err := parser.ParseString(context.Background(), configs.String())
	if err == nil {
		t.Fatal("Expected error for too many config files, got nil")
	}

	if !strings.Contains(err.Error(), "too many config files") {
		t.Errorf("Expected 'too many config files' error, got: %v", err)
	}
}

// TestSandboxLuaVM_MetatableProtection tests that metatable manipulation is blocked.
func TestSandboxLuaVM_MetatableProtection(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "getmetatable blocked",
			code: `zerb = {}; mt = getmetatable("")`,
		},
		{
			name: "setmetatable blocked",
			code: `zerb = {}; setmetatable({}, {})`,
		},
		{
			name: "rawget blocked",
			code: `zerb = {}; rawget(_G, "os")`,
		},
		{
			name: "rawset blocked",
			code: `zerb = {}; rawset(_G, "os", {})`,
		},
		{
			name: "collectgarbage blocked",
			code: `zerb = {}; collectgarbage("collect")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(nil)
			_, err := parser.ParseString(context.Background(), tt.code)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestParser_PathTraversalProtection tests that path traversal attempts are blocked.
func TestParser_PathTraversalProtection(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "parent directory traversal",
			path: "../../etc/passwd",
		},
		{
			name: "absolute path outside home",
			path: "/etc/passwd",
		},
		{
			name: "windows path traversal",
			path: "..\\..\\Windows\\System32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			luaCode := `
				zerb = {
					configs = {
						"` + tt.path + `",
					},
				}
			`

			parser := NewParser(nil)
			_, err := parser.ParseString(context.Background(), luaCode)
			if err == nil {
				t.Errorf("Expected error for path traversal attempt: %s", tt.path)
			}
		})
	}
}

// TestParser_InvalidToolStrings tests that invalid tool strings are rejected.
func TestParser_InvalidToolStrings(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		shouldFail bool
	}{
		{
			name:       "valid tool",
			tool:       "node@20.11.0",
			shouldFail: false,
		},
		{
			name:       "too long",
			tool:       strings.Repeat("a", 300),
			shouldFail: true,
		},
		{
			name:       "invalid characters",
			tool:       "node@20.11.0; rm -rf /",
			shouldFail: true,
		},
		{
			name:       "spaces",
			tool:       "node @20",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			luaCode := `
				zerb = {
					tools = {
						"` + tt.tool + `",
					},
				}
			`

			parser := NewParser(nil)
			_, err := parser.ParseString(context.Background(), luaCode)
			if tt.shouldFail && err == nil {
				t.Errorf("Expected error for invalid tool string: %s", tt.tool)
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("Expected no error for valid tool string: %s, got: %v", tt.tool, err)
			}
		})
	}
}

// TestParser_InvalidGitURLs tests that invalid git URLs are rejected.
func TestParser_InvalidGitURLs(t *testing.T) {
	tests := []struct {
		name       string
		remote     string
		shouldFail bool
	}{
		{
			name:       "valid https",
			remote:     "https://github.com/user/repo",
			shouldFail: false,
		},
		{
			name:       "valid ssh",
			remote:     "git@github.com:user/repo.git",
			shouldFail: false,
		},
		{
			name:       "invalid scheme",
			remote:     "ftp://example.com/repo",
			shouldFail: true,
		},
		{
			name:       "malformed url",
			remote:     "://invalid",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			luaCode := `
				zerb = {
					tools = {},
					git = {
						remote = "` + tt.remote + `",
					},
				}
			`

			parser := NewParser(nil)
			_, err := parser.ParseString(context.Background(), luaCode)
			if tt.shouldFail && err == nil {
				t.Errorf("Expected error for invalid git URL: %s", tt.remote)
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("Expected no error for valid git URL: %s, got: %v", tt.remote, err)
			}
		})
	}
}

// TestParser_ContextCancellation tests that context cancellation is respected.
func TestParser_ContextCancellation(t *testing.T) {
	luaCode := `zerb = { tools = {"node@20.11.0"} }`

	parser := NewParser(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := parser.ParseString(ctx, luaCode)
	if err == nil {
		t.Fatal("Expected error for cancelled context, got nil")
	}

	if !strings.Contains(err.Error(), "cancel") {
		t.Errorf("Expected cancellation error, got: %v", err)
	}
}
