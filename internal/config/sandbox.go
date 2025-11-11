package config

import (
	lua "github.com/yuin/gopher-lua"
)

// sandboxLuaVM configures a Lua VM to run in a restricted sandbox.
// This disables dangerous functions that could:
// - Execute system commands (os.execute, os.exit)
// - Access the filesystem (io.open, io.popen)
// - Load external code (require, dofile, loadfile)
// - Bypass sandbox restrictions (metatable manipulation)
//
// Safe modules like string, table, and math are preserved.
// This ensures user configs are declarative and cannot perform unsafe operations.
func sandboxLuaVM(L *lua.LState) {
	// Remove os library completely (os.execute, os.exit, os.getenv, etc.)
	L.SetGlobal("os", lua.LNil)

	// Remove io library completely (io.open, io.popen, io.read, etc.)
	L.SetGlobal("io", lua.LNil)

	// Remove package/module loading functions
	L.SetGlobal("require", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("load", lua.LNil)
	L.SetGlobal("loadstring", lua.LNil)

	// Remove debug library (could be used to bypass sandbox)
	L.SetGlobal("debug", lua.LNil)

	// Remove metatable manipulation functions (prevent sandbox bypass)
	L.SetGlobal("getmetatable", lua.LNil)
	L.SetGlobal("setmetatable", lua.LNil)
	L.SetGlobal("rawget", lua.LNil)
	L.SetGlobal("rawset", lua.LNil)

	// Remove collectgarbage (prevent timing attacks and resource manipulation)
	L.SetGlobal("collectgarbage", lua.LNil)

	// Keep safe libraries:
	// - string (string manipulation)
	// - table (table operations)
	// - math (math operations)
	// - type, tostring, tonumber, etc. (basic utilities)

	// Note: The following are kept and are safe for declarative configs:
	// - string library (all functions are safe)
	// - table library (all functions are safe)
	// - math library (all functions are safe)
	// - Basic functions: type, tostring, tonumber, pairs, ipairs, next, etc.
}

// newSandboxedVM creates a new Lua VM with sandboxing and resource limits applied.
// This is the primary way to create a Lua state for config parsing.
//
// Resource limits enforced:
//   - Call stack size: 256 (prevents deep recursion)
//   - Registry size: 8KB (limits memory usage)
//   - No Go stack traces in errors (security)
func newSandboxedVM() *lua.LState {
	opts := lua.Options{
		CallStackSize:       256,      // Limit recursion depth
		RegistrySize:        1024 * 8, // Limit registry memory (8KB)
		IncludeGoStackTrace: false,    // Don't leak Go internals in errors
	}
	L := lua.NewState(opts)
	sandboxLuaVM(L)
	return L
}
