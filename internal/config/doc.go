// Package config provides secure Lua configuration parsing, generation, and management
// for ZERB's declarative environment management system.
//
// # Overview
//
// The config package enables users to define their development environment using Lua
// configuration files. It provides:
//   - Bidirectional conversion between Lua configs and Go structs
//   - Platform-aware conditional configurations
//   - Comprehensive security sandboxing
//   - Timestamped configuration versioning
//
// # Architecture
//
// The package uses gopher-lua, a pure Go Lua 5.1 VM, for safe sandboxed execution
// of user configuration files. Platform information from the platform package is
// injected as a read-only table, enabling cross-platform configurations.
//
// Key components:
//   - Parser: Lua → Go struct conversion with platform detection
//   - Generator: Go struct → Lua code generation
//   - Sandbox: Restricted Lua VM preventing dangerous operations
//   - Validator: Comprehensive input validation and security checks
//
// # Security Model
//
// ## Sandboxing
//
// User Lua code runs in a heavily restricted sandbox that prevents:
//   - System command execution (os.execute, os.exit, etc.)
//   - Filesystem access (io.open, io.popen, etc.)
//   - External code loading (require, dofile, loadfile, etc.)
//   - Metatable manipulation (getmetatable, setmetatable, rawget, rawset)
//   - Garbage collection control (collectgarbage)
//
// Safe operations preserved:
//   - String manipulation (string library)
//   - Table operations (table library)
//   - Math operations (math library)
//   - Basic utilities (type, tostring, tonumber, pairs, ipairs)
//
// ## Resource Limits
//
// To prevent denial-of-service attacks:
//   - Config size: Maximum 10MB
//   - Parse timeout: 5 seconds (configurable via context)
//   - Call stack depth: 256 levels
//   - Registry size: 8KB
//   - Tool count: Maximum 1000 tools
//   - Config file count: Maximum 500 files
//
// ## Input Validation
//
// All user inputs are validated:
//   - Tool strings: Must match pattern ^([a-z0-9_-]+:)?[a-z0-9_/-]+(@[a-z0-9._-]+)?$
//   - Config paths: No path traversal (..); restricted to home directory
//   - Git URLs: Must use https:// or http:// (or SSH format git@host:repo)
//   - String lengths: Maximum 256 characters for tool strings
//
// ## Error Handling
//
// Error messages are sanitized to prevent information leakage:
//   - Stack traces removed
//   - Internal implementation details replaced with user-friendly terms
//   - Line numbers preserved for debugging
//
// # Usage
//
// ## Basic Parsing
//
// Parse a Lua configuration string into a Go struct:
//
//	parser := config.NewParser(platformDetector)
//	cfg, err := parser.ParseString(ctx, luaCode)
//	if err != nil {
//	    log.Fatalf("Parse error: %v", err)
//	}
//
// ## Generating Configs
//
// Generate Lua code from a Go struct:
//
//	gen := config.NewGenerator()
//	lua, err := gen.Generate(ctx, cfg)
//	if err != nil {
//	    log.Fatalf("Generate error: %v", err)
//	}
//
// ## Platform Conditionals
//
// User configs can include platform-specific logic:
//
//	zerb = {
//	  tools = {
//	    "node@20.11.0",
//	    platform.is_linux and "cargo:i3-msg" or nil,
//	    platform.is_macos and "yabai" or nil,
//	  },
//	}
//
// ## Structured Logging
//
// Add logging to track config operations:
//
//	parser := config.NewParser(detector).WithLogger(myLogger)
//	cfg, err := parser.ParseString(ctx, luaCode)
//
// Logger interface:
//
//	type Logger interface {
//	    Debug(msg string, keysAndValues ...interface{})
//	    Info(msg string, keysAndValues ...interface{})
//	    Warn(msg string, keysAndValues ...interface{})
//	    Error(msg string, keysAndValues ...interface{})
//	}
//
// # Performance Characteristics
//
// Typical parsing times (benchmarked on Intel i7-1260P):
//   - Small config (5 tools): ~123μs
//   - Medium config (100 tools): ~256μs
//   - Large config (1000 tools): ~4.3ms
//
// Generation is significantly faster:
//   - Small config: ~1μs
//   - Medium config: ~18μs
//   - Large config: ~189μs
//
// Memory usage:
//   - Parser: ~260KB per parse operation (small config)
//   - Generator: ~750B per generate operation (small config)
//   - Concurrent use: Safe for concurrent access
//
// # Configuration Schema
//
// Lua configuration structure:
//
//	zerb = {
//	  meta = {
//	    name = "My Environment",
//	    description = "Development setup",
//	  },
//	  tools = {
//	    "node@20.11.0",              -- name@version
//	    "cargo:ripgrep",             -- backend:name
//	    "npm:prettier@3.0.0",        -- backend:name@version
//	  },
//	  configs = {
//	    "~/.zshrc",                  -- simple path
//	    {
//	      path = "~/.ssh/config",
//	      template = true,           -- process as template
//	      secrets = true,            -- encrypt with GPG
//	      private = true,            -- chmod 600
//	    },
//	  },
//	  git = {
//	    remote = "https://github.com/user/dotfiles",
//	    branch = "main",
//	  },
//	  config = {
//	    backup_retention = 5,        -- keep last 5 snapshots
//	  },
//	}
//
// # Context and Timeouts
//
// All parsing operations respect context cancellation and deadlines:
//
//	// With explicit timeout
//	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
//	defer cancel()
//	cfg, err := parser.ParseString(ctx, luaCode)
//
//	// Context cancellation
//	ctx, cancel := context.WithCancel(context.Background())
//	go func() {
//	    // Cancel after some condition
//	    cancel()
//	}()
//	cfg, err := parser.ParseString(ctx, luaCode)
//
// If no deadline is set, a default 5-second timeout is applied.
//
// # Thread Safety
//
// All components are safe for concurrent use:
//   - Parser instances can be used from multiple goroutines
//   - Generator instances can be used from multiple goroutines
//   - No shared mutable state between operations
//
// # Error Types
//
// The package defines specific error types:
//
//	type ParseError struct {
//	    Message string  // User-friendly message
//	    Detail  string  // Technical details (sanitized)
//	}
//
//	type ValidationError struct {
//	    Field   string  // Field that failed validation
//	    Message string  // Error description
//	}
//
// # Design Decisions
//
// ## Why gopher-lua?
//
// Chosen for:
//   - Pure Go implementation (no CGO dependencies)
//   - Fast compilation and cross-compilation
//   - Easy to embed and sandbox
//   - Lua 5.1 compatibility
//   - Active maintenance
//
// ## Why Lua for configs?
//
// Advantages over other formats:
//   - Programmatic (enables platform conditionals)
//   - Familiar to developers (Neovim, Hammerspoon, Nginx use Lua)
//   - Easy to generate programmatically
//   - Readable and maintainable
//
// ## Security-first design
//
// Every feature considers security:
//   - Sandboxing prevents arbitrary code execution
//   - Resource limits prevent DoS attacks
//   - Input validation prevents injection attacks
//   - Error sanitization prevents information leakage
//
// # Known Limitations
//
// - Maximum config size: 10MB
//   - Rationale: Prevents memory exhaustion
//   - Workaround: Split large configs into multiple files
//
// - Lua 5.1 only (not 5.2/5.3/5.4)
//   - Rationale: gopher-lua targets Lua 5.1
//   - Impact: Some newer Lua features unavailable
//
// - No external Lua modules
//   - Rationale: Security (prevent arbitrary code execution)
//   - Impact: Configs must be self-contained
//
// # Future Enhancements
//
// Post-MVP features planned:
//   - Config diffing and merging
//   - Schema versioning for migrations
//   - Interactive config builder
//   - Config linting and validation CLI
//   - IDE integration (LSP for zerb.lua)
//
// # Related Packages
//
//   - internal/platform: Provides platform detection for conditionals
//   - internal/testutil: Test utilities for config testing
//
// # References
//
//   - Design document: .ai-workflow/implementation-planning/components/02-lua-config.md
//   - gopher-lua: https://github.com/yuin/gopher-lua
//   - Lua 5.1 manual: https://www.lua.org/manual/5.1/
package config
