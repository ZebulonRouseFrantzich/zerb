package platform

import (
	lua "github.com/yuin/gopher-lua"
)

// InjectPlatformTable creates a read-only platform table and injects it into the Lua state as a global.
// This should be called before loading any user configuration code.
func InjectPlatformTable(L *lua.LState, info *Info) error {
	// Create the main platform table
	platformTable := L.NewTable()

	// Basic OS and architecture
	L.SetField(platformTable, "os", lua.LString(info.OS))
	L.SetField(platformTable, "arch", lua.LString(info.Arch))
	L.SetField(platformTable, "arch_raw", lua.LString(info.ArchRaw))

	// OS booleans
	L.SetField(platformTable, "is_linux", lua.LBool(info.IsLinux()))
	L.SetField(platformTable, "is_macos", lua.LBool(info.IsMacOS()))
	L.SetField(platformTable, "is_windows", lua.LBool(info.IsWindows()))

	// Architecture booleans
	L.SetField(platformTable, "is_amd64", lua.LBool(info.IsAMD64()))
	L.SetField(platformTable, "is_arm64", lua.LBool(info.IsARM64()))
	L.SetField(platformTable, "is_apple_silicon", lua.LBool(info.IsAppleSilicon()))

	// Linux distribution (nil on non-Linux)
	distro := info.GetDistro()
	if distro != nil {
		distroTable := L.NewTable()
		L.SetField(distroTable, "id", lua.LString(distro.ID))
		L.SetField(distroTable, "family", lua.LString(distro.Family))
		L.SetField(distroTable, "version", lua.LString(distro.Version))
		L.SetField(platformTable, "distro", distroTable)
	} else {
		L.SetField(platformTable, "distro", lua.LNil)
	}

	// Linux family (nil on non-Linux)
	if info.IsLinux() && info.Family != "" {
		L.SetField(platformTable, "linux_family", lua.LString(info.Family))
	} else {
		L.SetField(platformTable, "linux_family", lua.LNil)
	}

	// Family booleans
	L.SetField(platformTable, "is_debian_family", lua.LBool(info.IsDebianFamily()))
	L.SetField(platformTable, "is_rhel_family", lua.LBool(info.IsRHELFamily()))
	L.SetField(platformTable, "is_fedora_family", lua.LBool(info.IsFedoraFamily()))
	L.SetField(platformTable, "is_suse_family", lua.LBool(info.IsSUSEFamily()))
	L.SetField(platformTable, "is_arch_family", lua.LBool(info.IsArchFamily()))
	L.SetField(platformTable, "is_alpine", lua.LBool(info.IsAlpine()))
	L.SetField(platformTable, "is_gentoo", lua.LBool(info.IsGentoo()))

	// Helper function: when(condition, value)
	// Returns value if condition is true, nil otherwise
	whenFunc := L.NewFunction(func(L *lua.LState) int {
		cond := L.CheckBool(1)
		value := L.Get(2)
		if cond {
			L.Push(value)
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	L.SetField(platformTable, "when", whenFunc)

	// Make the table read-only using a proxy table with metatable
	readOnlyTable := makeReadOnly(L, platformTable)

	// Set the read-only proxy as global
	L.SetGlobal("platform", readOnlyTable)

	return nil
}

// makeReadOnly makes a Lua table read-only by creating a proxy table with a metatable.
// The proxy redirects reads to the original table but prevents all writes.
func makeReadOnly(L *lua.LState, table *lua.LTable) *lua.LTable {
	mt := L.NewTable()

	// Redirect reads to the original table
	L.SetField(mt, "__index", table)

	// Prevent all writes (both new and existing keys)
	L.SetField(mt, "__newindex", L.NewFunction(func(L *lua.LState) int {
		L.RaiseError("platform table is read-only and cannot be modified")
		return 0
	}))

	// Prevent changing the metatable itself
	L.SetField(mt, "__metatable", lua.LString("protected"))

	// Create a new empty proxy table with the metatable
	// This proxy table will redirect reads to the original table
	// but prevent all writes
	proxy := L.NewTable()
	L.SetMetatable(proxy, mt)

	return proxy
}
