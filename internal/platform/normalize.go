package platform

import (
	"fmt"
	"strings"
)

// familyMap maps distribution names to their canonical family names.
// This is used to normalize variations of family strings from gopsutil.
var familyMap = map[string]string{
	"debian":   FamilyDebian,
	"ubuntu":   FamilyDebian, // gopsutil might return ubuntu as family
	"rhel":     FamilyRHEL,
	"centos":   FamilyRHEL,
	"rocky":    FamilyRHEL,
	"fedora":   FamilyFedora,
	"suse":     FamilySUSE,
	"opensuse": FamilySUSE,
	"arch":     FamilyArch,
	"manjaro":  FamilyArch,
	"alpine":   FamilyAlpine,
	"gentoo":   FamilyGentoo,
}

// normalizeArch converts GOARCH values to normalized architecture names.
// MVP supports only amd64 and arm64.
func normalizeArch(arch string) (string, error) {
	switch arch {
	case "amd64", "x86_64":
		return "amd64", nil
	case "arm64", "aarch64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s (MVP supports amd64 and arm64 only)", arch)
	}
}

// normalizePlatform converts platform IDs to lowercase for consistency.
func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

// mapFamily maps distribution family strings to canonical family names.
// Uses a package-level lookup table for explicit mapping.
func mapFamily(family string) string {
	normalized := strings.ToLower(strings.TrimSpace(family))
	if canonical, ok := familyMap[normalized]; ok {
		return canonical
	}

	// Return "unknown" for unrecognized families
	return FamilyUnknown
}
