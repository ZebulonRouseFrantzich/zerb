-- Platform-Specific ZERB Configuration Example
--
-- This demonstrates using platform conditionals to install
-- different tools on different operating systems and distributions.

zerb = {
  meta = {
    name = "Cross-Platform Development Environment",
    description = "Works on Linux, macOS, and Windows with platform-specific tools",
  },

  tools = {
    -- Universal tools (work everywhere)
    "node@20.11.0",
    "python@3.12.1",
    "cargo:ripgrep",
    
    -- OS-specific tools
    platform.is_linux and "cargo:i3-msg" or nil,
    platform.is_macos and "yabai" or nil,
    platform.is_windows and "scoop:windows-terminal" or nil,
    
    -- Linux distribution family-specific
    platform.is_debian_family and "ubi:sharkdp/fd" or nil,
    platform.is_arch_family and "yay" or nil,
    platform.is_rhel_family and "dnf:git-lfs" or nil,
    
    -- Architecture-specific
    platform.is_arm64 and "ubi:arm64-specific-tool" or nil,
    platform.is_amd64 and "ubi:amd64-specific-tool" or nil,
    
    -- Apple Silicon specific
    platform.is_apple_silicon and "rosetta-compatible-tool" or nil,
    
    -- Using the when() helper function
    platform.when(platform.is_linux, "linux-only-tool"),
    platform.when(platform.is_debian_family, "apt:build-essential"),
  },

  configs = {
    "~/.gitconfig",
    
    -- OS-specific config files
    platform.is_linux and "~/.config/i3/config" or nil,
    platform.is_macos and "~/.config/yabai/yabairc" or nil,
  },
}
