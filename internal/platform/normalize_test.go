package platform

import (
	"testing"
)

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"amd64", "amd64", "amd64", false},
		{"x86_64", "x86_64", "amd64", false},
		{"arm64", "arm64", "arm64", false},
		{"aarch64", "aarch64", "arm64", false},
		{"i386 unsupported", "i386", "", true},
		{"arm unsupported", "arm", "", true},
		{"unknown", "unknown", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeArch(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeArch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeArch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizePlatform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ubuntu", "ubuntu", "ubuntu"},
		{"Ubuntu uppercase", "Ubuntu", "ubuntu"},
		{"UBUNTU all caps", "UBUNTU", "ubuntu"},
		{"with spaces", "  ubuntu  ", "ubuntu"},
		{"arch", "arch", "arch"},
		{"fedora", "fedora", "fedora"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePlatform(tt.input)
			if got != tt.want {
				t.Errorf("normalizePlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapFamily(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Canonical families
		{"debian", "debian", "debian"},
		{"rhel", "rhel", "rhel"},
		{"fedora", "fedora", "fedora"},
		{"suse", "suse", "suse"},
		{"arch", "arch", "arch"},
		{"alpine", "alpine", "alpine"},
		{"gentoo", "gentoo", "gentoo"},

		// Aliases
		{"ubuntu maps to debian", "ubuntu", "debian"},
		{"centos maps to rhel", "centos", "rhel"},
		{"rocky maps to rhel", "rocky", "rhel"},
		{"opensuse maps to suse", "opensuse", "suse"},
		{"manjaro maps to arch", "manjaro", "arch"},

		// Case insensitive
		{"Debian uppercase", "Debian", "debian"},
		{"RHEL all caps", "RHEL", "rhel"},

		// With spaces
		{"with spaces", "  debian  ", "debian"},

		// Unknown
		{"unknown family", "unknown", "unknown"},
		{"empty", "", "unknown"},
		{"unrecognized", "somethingelse", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapFamily(tt.input)
			if got != tt.want {
				t.Errorf("mapFamily() = %v, want %v", got, tt.want)
			}
		})
	}
}
