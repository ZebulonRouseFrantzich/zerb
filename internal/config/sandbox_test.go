package config

import (
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestSandboxLuaVM(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
		errMsg  string
	}{
		// Safe operations that should work
		{
			name:    "string operations allowed",
			code:    `x = string.upper("hello")`,
			wantErr: false,
		},
		{
			name:    "table operations allowed",
			code:    `t = {1, 2, 3}; table.insert(t, 4)`,
			wantErr: false,
		},
		{
			name:    "math operations allowed",
			code:    `x = math.sqrt(16)`,
			wantErr: false,
		},
		{
			name:    "basic functions allowed",
			code:    `x = type("hello"); y = tostring(123); z = tonumber("456")`,
			wantErr: false,
		},
		{
			name:    "pairs and ipairs allowed",
			code:    `t = {a=1, b=2}; for k,v in pairs(t) do end`,
			wantErr: false,
		},

		// Dangerous operations that should fail
		{
			name:    "os.execute blocked",
			code:    `os.execute("ls")`,
			wantErr: true,
			errMsg:  "attempt to index",
		},
		{
			name:    "os.getenv blocked",
			code:    `x = os.getenv("PATH")`,
			wantErr: true,
			errMsg:  "attempt to index",
		},
		{
			name:    "io.open blocked",
			code:    `f = io.open("/etc/passwd")`,
			wantErr: true,
			errMsg:  "attempt to index",
		},
		{
			name:    "io.popen blocked",
			code:    `f = io.popen("ls")`,
			wantErr: true,
			errMsg:  "attempt to index",
		},
		{
			name:    "require blocked",
			code:    `socket = require("socket")`,
			wantErr: true,
			errMsg:  "attempt to call",
		},
		{
			name:    "dofile blocked",
			code:    `dofile("/tmp/evil.lua")`,
			wantErr: true,
			errMsg:  "attempt to call",
		},
		{
			name:    "loadfile blocked",
			code:    `f = loadfile("/tmp/evil.lua")`,
			wantErr: true,
			errMsg:  "attempt to call",
		},
		{
			name:    "load blocked",
			code:    `f = load("return 1+1")`,
			wantErr: true,
			errMsg:  "attempt to call",
		},
		{
			name:    "loadstring blocked",
			code:    `f = loadstring("return 1+1")`,
			wantErr: true,
			errMsg:  "attempt to call",
		},
		{
			name:    "debug blocked",
			code:    `debug.getinfo(1)`,
			wantErr: true,
			errMsg:  "attempt to index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := newSandboxedVM()
			defer L.Close()

			err := L.DoString(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("sandboxLuaVM() with code %q: error = %v, wantErr %v", tt.code, err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("sandboxLuaVM() with code %q: error = %v, want substring %q", tt.code, err, tt.errMsg)
				}
			}
		})
	}
}

func TestSandboxLuaVM_StringLibrary(t *testing.T) {
	L := newSandboxedVM()
	defer L.Close()

	// Test various string functions
	code := `
		result = {}
		result.upper = string.upper("hello")
		result.lower = string.lower("WORLD")
		result.len = string.len("test")
		result.sub = string.sub("hello", 1, 2)
		result.rep = string.rep("x", 3)
		result.format = string.format("%s %d", "test", 42)
		result.match = string.match("hello world", "world")
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("string library functions failed: %v", err)
	}

	// Verify results
	result := L.GetGlobal("result").(*lua.LTable)
	if result.RawGetString("upper").String() != "HELLO" {
		t.Errorf("string.upper failed")
	}
	if result.RawGetString("lower").String() != "world" {
		t.Errorf("string.lower failed")
	}
}

func TestSandboxLuaVM_TableLibrary(t *testing.T) {
	L := newSandboxedVM()
	defer L.Close()

	code := `
		t = {1, 2, 3}
		table.insert(t, 4)
		table.insert(t, 1, 0)  -- insert at position 1
		result_len = #t
		table.remove(t, 1)
		result_concat = table.concat(t, ",")
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("table library functions failed: %v", err)
	}

	// Verify length after insert
	resultLen := L.GetGlobal("result_len")
	if resultLen.Type() != lua.LTNumber || lua.LVAsNumber(resultLen) != 5 {
		t.Errorf("table operations failed: len = %v, want 5", resultLen)
	}

	// Verify concat after remove
	resultConcat := L.GetGlobal("result_concat")
	if resultConcat.String() != "1,2,3,4" {
		t.Errorf("table.concat = %s, want '1,2,3,4'", resultConcat.String())
	}
}

func TestSandboxLuaVM_MathLibrary(t *testing.T) {
	L := newSandboxedVM()
	defer L.Close()

	code := `
		result = {}
		result.sqrt = math.sqrt(16)
		result.floor = math.floor(3.7)
		result.ceil = math.ceil(3.2)
		result.abs = math.abs(-5)
		result.min = math.min(1, 2, 3)
		result.max = math.max(1, 2, 3)
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("math library functions failed: %v", err)
	}

	result := L.GetGlobal("result").(*lua.LTable)

	sqrt := result.RawGetString("sqrt")
	if sqrt.Type() != lua.LTNumber || lua.LVAsNumber(sqrt) != 4 {
		t.Errorf("math.sqrt(16) = %v, want 4", sqrt)
	}

	floor := result.RawGetString("floor")
	if floor.Type() != lua.LTNumber || lua.LVAsNumber(floor) != 3 {
		t.Errorf("math.floor(3.7) = %v, want 3", floor)
	}
}

func TestNewSandboxedVM(t *testing.T) {
	L := newSandboxedVM()
	defer L.Close()

	// Verify it's sandboxed by checking os is nil
	os := L.GetGlobal("os")
	if os.Type() != lua.LTNil {
		t.Errorf("newSandboxedVM() os = %v, want nil", os.Type())
	}

	// Verify string is available
	str := L.GetGlobal("string")
	if str.Type() != lua.LTTable {
		t.Errorf("newSandboxedVM() string = %v, want table", str.Type())
	}
}

func TestSandboxLuaVM_BasicFunctions(t *testing.T) {
	L := newSandboxedVM()
	defer L.Close()

	code := `
		result = {}
		result.type_string = type("hello")
		result.type_number = type(123)
		result.type_table = type({})
		result.tostring = tostring(123)
		result.tonumber = tonumber("456")
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("basic functions failed: %v", err)
	}

	result := L.GetGlobal("result").(*lua.LTable)

	typeString := result.RawGetString("type_string")
	if typeString.String() != "string" {
		t.Errorf("type('hello') = %s, want 'string'", typeString.String())
	}

	tostring := result.RawGetString("tostring")
	if tostring.String() != "123" {
		t.Errorf("tostring(123) = %s, want '123'", tostring.String())
	}

	tonumber := result.RawGetString("tonumber")
	if tonumber.Type() != lua.LTNumber || lua.LVAsNumber(tonumber) != 456 {
		t.Errorf("tonumber('456') = %v, want 456", tonumber)
	}
}
