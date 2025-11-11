package platform

import (
	"fmt"
	"strings"
)

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

// mapFamily maps gopsutil family strings to canonical family names.
// Uses a lookup table for explicit mapping.
func mapFamily(family string) string {
	familyMap := map[string]string{
		"debian":   "debian",
		"ubuntu":   "debian", // gopsutil might return ubuntu as family
		"rhel":     "rhel",
		"centos":   "rhel",
		"rocky":    "rhel",
		"fedora":   "fedora",
		"suse":     "suse",
		"opensuse": "suse",
		"arch":     "arch",
		"manjaro":  "arch",
		"alpine":   "alpine",
		"gentoo":   "gentoo",
	}

	normalized := strings.ToLower(strings.TrimSpace(family))
	if canonical, ok := familyMap[normalized]; ok {
		return canonical
	}

	// Return "unknown" for unrecognized families
	return "unknown"
}
