package platform

import "context"

// Info contains platform detection information.
type Info struct {
	OS       string // "linux", "darwin", "windows"
	Arch     string // "amd64", "arm64" (normalized)
	ArchRaw  string // original GOARCH (e.g., "x86_64", "aarch64")
	Platform string // distro ID (Linux only, e.g., "ubuntu", "arch")
	Family   string // canonical family (e.g., "debian", "rhel", "arch")
	Version  string // distro version (Linux only, e.g., "22.04")
}

// Distro contains Linux distribution information.
// This is nil on non-Linux platforms.
type Distro struct {
	ID      string // distro ID (e.g., "ubuntu")
	Family  string // canonical family (e.g., "debian")
	Version string // version (e.g., "22.04")
}

// GetDistro returns distro information if this is a Linux platform.
// Returns nil for non-Linux platforms or if distro detection failed.
func (i *Info) GetDistro() *Distro {
	if i.OS != "linux" || i.Platform == "" {
		return nil
	}
	return &Distro{
		ID:      i.Platform,
		Family:  i.Family,
		Version: i.Version,
	}
}

// IsLinux returns true if the platform is Linux.
func (i *Info) IsLinux() bool {
	return i.OS == "linux"
}

// IsMacOS returns true if the platform is macOS.
func (i *Info) IsMacOS() bool {
	return i.OS == "darwin"
}

// IsWindows returns true if the platform is Windows.
func (i *Info) IsWindows() bool {
	return i.OS == "windows"
}

// IsAMD64 returns true if the architecture is amd64.
func (i *Info) IsAMD64() bool {
	return i.Arch == "amd64"
}

// IsARM64 returns true if the architecture is arm64.
func (i *Info) IsARM64() bool {
	return i.Arch == "arm64"
}

// IsAppleSilicon returns true if running on Apple Silicon (macOS + arm64).
func (i *Info) IsAppleSilicon() bool {
	return i.OS == "darwin" && i.Arch == "arm64"
}

// IsDebianFamily returns true if the Linux distribution is Debian-based.
func (i *Info) IsDebianFamily() bool {
	return i.OS == "linux" && i.Family == "debian"
}

// IsRHELFamily returns true if the Linux distribution is RHEL-based.
func (i *Info) IsRHELFamily() bool {
	return i.OS == "linux" && i.Family == "rhel"
}

// IsFedoraFamily returns true if the Linux distribution is Fedora-based.
func (i *Info) IsFedoraFamily() bool {
	return i.OS == "linux" && i.Family == "fedora"
}

// IsSUSEFamily returns true if the Linux distribution is SUSE-based.
func (i *Info) IsSUSEFamily() bool {
	return i.OS == "linux" && i.Family == "suse"
}

// IsArchFamily returns true if the Linux distribution is Arch-based.
func (i *Info) IsArchFamily() bool {
	return i.OS == "linux" && i.Family == "arch"
}

// IsAlpine returns true if the Linux distribution is Alpine.
func (i *Info) IsAlpine() bool {
	return i.OS == "linux" && i.Family == "alpine"
}

// IsGentoo returns true if the Linux distribution is Gentoo.
func (i *Info) IsGentoo() bool {
	return i.OS == "linux" && i.Family == "gentoo"
}

// Detector is the interface for platform detection.
type Detector interface {
	Detect(ctx context.Context) (*Info, error)
}
