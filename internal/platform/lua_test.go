package platform

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestInjectPlatformTable_Linux(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:       "linux",
		Arch:     "amd64",
		ArchRaw:  "x86_64",
		Platform: "ubuntu",
		Family:   "debian",
		Version:  "22.04",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	// Verify the platform table exists
	if err := L.DoString(`
		if platform == nil then
			error("platform table not found")
		end
	`); err != nil {
		t.Fatalf("platform table not found: %v", err)
	}

	// Test basic fields
	tests := []struct {
		name string
		code string
		want lua.LValue
	}{
		{"os", `return platform.os`, lua.LString("linux")},
		{"arch", `return platform.arch`, lua.LString("amd64")},
		{"arch_raw", `return platform.arch_raw`, lua.LString("x86_64")},
		{"is_linux", `return platform.is_linux`, lua.LTrue},
		{"is_macos", `return platform.is_macos`, lua.LFalse},
		{"is_windows", `return platform.is_windows`, lua.LFalse},
		{"is_amd64", `return platform.is_amd64`, lua.LTrue},
		{"is_arm64", `return platform.is_arm64`, lua.LFalse},
		{"is_apple_silicon", `return platform.is_apple_silicon`, lua.LFalse},
		{"distro.id", `return platform.distro.id`, lua.LString("ubuntu")},
		{"distro.family", `return platform.distro.family`, lua.LString("debian")},
		{"distro.version", `return platform.distro.version`, lua.LString("22.04")},
		{"linux_family", `return platform.linux_family`, lua.LString("debian")},
		{"is_debian_family", `return platform.is_debian_family`, lua.LTrue},
		{"is_rhel_family", `return platform.is_rhel_family`, lua.LFalse},
		{"is_fedora_family", `return platform.is_fedora_family`, lua.LFalse},
		{"is_suse_family", `return platform.is_suse_family`, lua.LFalse},
		{"is_arch_family", `return platform.is_arch_family`, lua.LFalse},
		{"is_alpine", `return platform.is_alpine`, lua.LFalse},
		{"is_gentoo", `return platform.is_gentoo`, lua.LFalse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("failed to execute code: %v", err)
			}
			got := L.Get(-1)
			L.Pop(1)

			if got.Type() != tt.want.Type() {
				t.Errorf("type mismatch: got %v, want %v", got.Type(), tt.want.Type())
				return
			}

			if got.String() != tt.want.String() {
				t.Errorf("value mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInjectPlatformTable_MacOS(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:      "darwin",
		Arch:    "arm64",
		ArchRaw: "arm64",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	tests := []struct {
		name string
		code string
		want lua.LValue
	}{
		{"os", `return platform.os`, lua.LString("darwin")},
		{"arch", `return platform.arch`, lua.LString("arm64")},
		{"is_linux", `return platform.is_linux`, lua.LFalse},
		{"is_macos", `return platform.is_macos`, lua.LTrue},
		{"is_windows", `return platform.is_windows`, lua.LFalse},
		{"is_arm64", `return platform.is_arm64`, lua.LTrue},
		{"is_apple_silicon", `return platform.is_apple_silicon`, lua.LTrue},
		{"distro is nil", `return platform.distro`, lua.LNil},
		{"linux_family is nil", `return platform.linux_family`, lua.LNil},
		{"is_debian_family", `return platform.is_debian_family`, lua.LFalse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("failed to execute code: %v", err)
			}
			got := L.Get(-1)
			L.Pop(1)

			if got.Type() != tt.want.Type() {
				t.Errorf("type mismatch: got %v, want %v", got.Type(), tt.want.Type())
				return
			}

			if got.String() != tt.want.String() {
				t.Errorf("value mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInjectPlatformTable_Windows(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:      "windows",
		Arch:    "amd64",
		ArchRaw: "amd64",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	tests := []struct {
		name string
		code string
		want lua.LValue
	}{
		{"os", `return platform.os`, lua.LString("windows")},
		{"is_windows", `return platform.is_windows`, lua.LTrue},
		{"is_linux", `return platform.is_linux`, lua.LFalse},
		{"is_macos", `return platform.is_macos`, lua.LFalse},
		{"distro is nil", `return platform.distro`, lua.LNil},
		{"linux_family is nil", `return platform.linux_family`, lua.LNil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("failed to execute code: %v", err)
			}
			got := L.Get(-1)
			L.Pop(1)

			if got.Type() != tt.want.Type() {
				t.Errorf("type mismatch: got %v, want %v", got.Type(), tt.want.Type())
				return
			}

			if got.String() != tt.want.String() {
				t.Errorf("value mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlatformTable_ReadOnly(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:   "linux",
		Arch: "amd64",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	// Test that modifying the platform table raises an error
	tests := []struct {
		name string
		code string
	}{
		{"modify os", `platform.os = "windows"`},
		{"add new field", `platform.new_field = "value"`},
		{"modify boolean", `platform.is_linux = false`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := L.DoString(tt.code)
			if err == nil {
				t.Error("expected error when modifying read-only table, got nil")
			}
			// Expected: error message mentions read-only protection
		})
	}
}

func TestPlatformTable_WhenHelper(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:   "linux",
		Arch: "amd64",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	tests := []struct {
		name string
		code string
		want lua.LValue
	}{
		{
			name: "when true returns value",
			code: `return platform.when(true, "tool")`,
			want: lua.LString("tool"),
		},
		{
			name: "when false returns nil",
			code: `return platform.when(false, "tool")`,
			want: lua.LNil,
		},
		{
			name: "when with platform boolean true",
			code: `return platform.when(platform.is_linux, "linux-tool")`,
			want: lua.LString("linux-tool"),
		},
		{
			name: "when with platform boolean false",
			code: `return platform.when(platform.is_macos, "macos-tool")`,
			want: lua.LNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("failed to execute code: %v", err)
			}
			got := L.Get(-1)
			L.Pop(1)

			if got.Type() != tt.want.Type() {
				t.Errorf("type mismatch: got %v, want %v", got.Type(), tt.want.Type())
				return
			}

			if got.String() != tt.want.String() {
				t.Errorf("value mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlatformTable_UsageExample(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	info := &Info{
		OS:       "linux",
		Arch:     "amd64",
		Platform: "ubuntu",
		Family:   "debian",
		Version:  "22.04",
	}

	err := InjectPlatformTable(L, info)
	if err != nil {
		t.Fatalf("InjectPlatformTable() error = %v", err)
	}

	// Test realistic usage example from the component doc
	code := `
		tools = {}
		
		-- OS-specific tools
		if platform.is_linux then
			table.insert(tools, "cargo:i3-msg")
		end
		
		if platform.is_macos then
			table.insert(tools, "yabai")
		end
		
		-- Family-specific tools
		if platform.is_debian_family then
			table.insert(tools, "ubi:sharkdp/fd")
		end
		
		-- Helper function usage
		local tool = platform.when(platform.is_apple_silicon, "rosetta-tool")
		if tool then
			table.insert(tools, tool)
		end
		
		return #tools
	`

	if err := L.DoString(code); err != nil {
		t.Fatalf("failed to execute usage example: %v", err)
	}

	result := L.Get(-1)
	L.Pop(1)

	// Should have 2 tools: cargo:i3-msg and ubi:sharkdp/fd
	if result.Type() != lua.LTNumber {
		t.Fatalf("expected number, got %v", result.Type())
	}

	count := int(result.(lua.LNumber))
	if count != 2 {
		t.Errorf("expected 2 tools, got %d", count)
	}
}
