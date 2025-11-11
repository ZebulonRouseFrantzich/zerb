// Package platform provides cross-platform detection and Lua integration
// for ZERB's declarative environment management.
//
// It detects OS, architecture, and Linux distribution details, then injects
// this information as a read-only table into Lua configurations. The package
// uses gopsutil for Linux distribution detection and provides graceful
// fallback behavior when detection fails.
package platform

import "context"

// Linux distribution family constants.
// These represent canonical family names for grouping related distributions.
const (
	FamilyDebian  = "debian"  // Debian, Ubuntu, Linux Mint
	FamilyRHEL    = "rhel"    // RHEL, CentOS, Rocky Linux, AlmaLinux
	FamilyFedora  = "fedora"  // Fedora
	FamilySUSE    = "suse"    // openSUSE, SLES
	FamilyArch    = "arch"    // Arch Linux, Manjaro
	FamilyAlpine  = "alpine"  // Alpine Linux
	FamilyGentoo  = "gentoo"  // Gentoo
	FamilyUnknown = "unknown" // Unrecognized distributions
)

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
	return i.OS == "linux" && i.Family == FamilyDebian
}

// IsRHELFamily returns true if the Linux distribution is RHEL-based.
func (i *Info) IsRHELFamily() bool {
	return i.OS == "linux" && i.Family == FamilyRHEL
}

// IsFedoraFamily returns true if the Linux distribution is Fedora-based.
func (i *Info) IsFedoraFamily() bool {
	return i.OS == "linux" && i.Family == FamilyFedora
}

// IsSUSEFamily returns true if the Linux distribution is SUSE-based.
func (i *Info) IsSUSEFamily() bool {
	return i.OS == "linux" && i.Family == FamilySUSE
}

// IsArchFamily returns true if the Linux distribution is Arch-based.
func (i *Info) IsArchFamily() bool {
	return i.OS == "linux" && i.Family == FamilyArch
}

// IsAlpine returns true if the Linux distribution is Alpine.
func (i *Info) IsAlpine() bool {
	return i.OS == "linux" && i.Family == FamilyAlpine
}

// IsGentoo returns true if the Linux distribution is Gentoo.
func (i *Info) IsGentoo() bool {
	return i.OS == "linux" && i.Family == FamilyGentoo
}

// Detector is the interface for platform detection.
type Detector interface {
	Detect(ctx context.Context) (*Info, error)
}
