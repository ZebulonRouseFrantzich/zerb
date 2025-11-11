package platform

import (
	"context"
	"runtime"
	"testing"
)

// MockDetector is a test implementation of Detector.
type MockDetector struct {
	info *Info
	err  error
}

// NewMockDetector creates a mock detector with specified return values.
func NewMockDetector(info *Info, err error) Detector {
	return &MockDetector{info: info, err: err}
}

// Detect returns the pre-configured info and error.
func (m *MockDetector) Detect(ctx context.Context) (*Info, error) {
	return m.info, m.err
}

func TestRealDetector_Detect(t *testing.T) {
	detector := NewDetector()
	ctx := context.Background()

	info, err := detector.Detect(ctx)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	// Verify OS detection
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %v, want %v", info.OS, runtime.GOOS)
	}

	// Verify architecture detection
	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if info.Arch != "amd64" && info.Arch != "arm64" {
		t.Errorf("Arch = %v, want amd64 or arm64", info.Arch)
	}

	// Verify ArchRaw is set
	if info.ArchRaw != runtime.GOARCH {
		t.Errorf("ArchRaw = %v, want %v", info.ArchRaw, runtime.GOARCH)
	}

	// On Linux, verify distro fields (may be empty if detection fails)
	if runtime.GOOS == "linux" {
		// Platform may be set or empty (graceful fallback)
		// If platform is set, family should also be set
		if info.Platform != "" && info.Family == "" {
			t.Error("If Platform is set, Family should also be set")
		}

		// Family should never be empty string if Platform is set
		// It should be "unknown" at minimum
		if info.Platform != "" && info.Family == "" {
			t.Error("Family should be set when Platform is set")
		}
	}

	// On non-Linux, distro fields should be empty
	if runtime.GOOS != "linux" {
		if info.Platform != "" {
			t.Errorf("Platform should be empty on non-Linux, got %v", info.Platform)
		}
		if info.Family != "" {
			t.Errorf("Family should be empty on non-Linux, got %v", info.Family)
		}
		if info.Version != "" {
			t.Errorf("Version should be empty on non-Linux, got %v", info.Version)
		}
	}
}

func TestInfo_GetDistro(t *testing.T) {
	tests := []struct {
		name string
		info *Info
		want *Distro
	}{
		{
			name: "Linux with distro info",
			info: &Info{
				OS:       "linux",
				Arch:     "amd64",
				Platform: "ubuntu",
				Family:   "debian",
				Version:  "22.04",
			},
			want: &Distro{
				ID:      "ubuntu",
				Family:  "debian",
				Version: "22.04",
			},
		},
		{
			name: "Linux without distro info",
			info: &Info{
				OS:   "linux",
				Arch: "amd64",
			},
			want: nil,
		},
		{
			name: "macOS",
			info: &Info{
				OS:   "darwin",
				Arch: "arm64",
			},
			want: nil,
		},
		{
			name: "Windows",
			info: &Info{
				OS:   "windows",
				Arch: "amd64",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.GetDistro()
			if got == nil && tt.want == nil {
				return
			}
			if got == nil || tt.want == nil {
				t.Errorf("GetDistro() = %v, want %v", got, tt.want)
				return
			}
			if got.ID != tt.want.ID || got.Family != tt.want.Family || got.Version != tt.want.Version {
				t.Errorf("GetDistro() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInfo_BooleanMethods(t *testing.T) {
	tests := []struct {
		name   string
		info   *Info
		checks map[string]bool
	}{
		{
			name: "Linux amd64 Debian",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "debian",
			},
			checks: map[string]bool{
				"IsLinux":        true,
				"IsMacOS":        false,
				"IsWindows":      false,
				"IsAMD64":        true,
				"IsARM64":        false,
				"IsAppleSilicon": false,
				"IsDebianFamily": true,
				"IsRHELFamily":   false,
				"IsFedoraFamily": false,
				"IsSUSEFamily":   false,
				"IsArchFamily":   false,
				"IsAlpine":       false,
				"IsGentoo":       false,
			},
		},
		{
			name: "macOS arm64 (Apple Silicon)",
			info: &Info{
				OS:   "darwin",
				Arch: "arm64",
			},
			checks: map[string]bool{
				"IsLinux":        false,
				"IsMacOS":        true,
				"IsWindows":      false,
				"IsAMD64":        false,
				"IsARM64":        true,
				"IsAppleSilicon": true,
				"IsDebianFamily": false,
				"IsRHELFamily":   false,
			},
		},
		{
			name: "macOS amd64 (Intel)",
			info: &Info{
				OS:   "darwin",
				Arch: "amd64",
			},
			checks: map[string]bool{
				"IsLinux":        false,
				"IsMacOS":        true,
				"IsWindows":      false,
				"IsAMD64":        true,
				"IsARM64":        false,
				"IsAppleSilicon": false,
			},
		},
		{
			name: "Windows amd64",
			info: &Info{
				OS:   "windows",
				Arch: "amd64",
			},
			checks: map[string]bool{
				"IsLinux":   false,
				"IsMacOS":   false,
				"IsWindows": true,
				"IsAMD64":   true,
				"IsARM64":   false,
			},
		},
		{
			name: "Linux arm64 Arch",
			info: &Info{
				OS:     "linux",
				Arch:   "arm64",
				Family: "arch",
			},
			checks: map[string]bool{
				"IsLinux":        true,
				"IsARM64":        true,
				"IsArchFamily":   true,
				"IsDebianFamily": false,
			},
		},
		{
			name: "Linux amd64 RHEL",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "rhel",
			},
			checks: map[string]bool{
				"IsLinux":      true,
				"IsRHELFamily": true,
			},
		},
		{
			name: "Linux amd64 Fedora",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "fedora",
			},
			checks: map[string]bool{
				"IsLinux":        true,
				"IsFedoraFamily": true,
			},
		},
		{
			name: "Linux amd64 SUSE",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "suse",
			},
			checks: map[string]bool{
				"IsLinux":      true,
				"IsSUSEFamily": true,
			},
		},
		{
			name: "Linux amd64 Alpine",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "alpine",
			},
			checks: map[string]bool{
				"IsLinux":  true,
				"IsAlpine": true,
			},
		},
		{
			name: "Linux amd64 Gentoo",
			info: &Info{
				OS:     "linux",
				Arch:   "amd64",
				Family: "gentoo",
			},
			checks: map[string]bool{
				"IsLinux":  true,
				"IsGentoo": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for methodName, expected := range tt.checks {
				var got bool
				switch methodName {
				case "IsLinux":
					got = tt.info.IsLinux()
				case "IsMacOS":
					got = tt.info.IsMacOS()
				case "IsWindows":
					got = tt.info.IsWindows()
				case "IsAMD64":
					got = tt.info.IsAMD64()
				case "IsARM64":
					got = tt.info.IsARM64()
				case "IsAppleSilicon":
					got = tt.info.IsAppleSilicon()
				case "IsDebianFamily":
					got = tt.info.IsDebianFamily()
				case "IsRHELFamily":
					got = tt.info.IsRHELFamily()
				case "IsFedoraFamily":
					got = tt.info.IsFedoraFamily()
				case "IsSUSEFamily":
					got = tt.info.IsSUSEFamily()
				case "IsArchFamily":
					got = tt.info.IsArchFamily()
				case "IsAlpine":
					got = tt.info.IsAlpine()
				case "IsGentoo":
					got = tt.info.IsGentoo()
				default:
					t.Fatalf("Unknown method: %s", methodName)
				}

				if got != expected {
					t.Errorf("%s() = %v, want %v", methodName, got, expected)
				}
			}
		})
	}
}

func TestMockDetector(t *testing.T) {
	expectedInfo := &Info{
		OS:       "linux",
		Arch:     "amd64",
		Platform: "ubuntu",
		Family:   "debian",
		Version:  "22.04",
	}

	detector := NewMockDetector(expectedInfo, nil)
	info, err := detector.Detect(context.Background())

	if err != nil {
		t.Fatalf("MockDetector.Detect() error = %v", err)
	}

	if info != expectedInfo {
		t.Errorf("MockDetector.Detect() = %+v, want %+v", info, expectedInfo)
	}
}
