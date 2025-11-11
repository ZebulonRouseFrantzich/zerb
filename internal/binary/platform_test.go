package binary

import (
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

func TestConstructMiseDownloadInfo(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		os          string
		arch        string
		expectedURL string
		expectedSig string
		wantErr     bool
	}{
		{
			name:        "linux_amd64",
			version:     "2024.12.7",
			os:          "linux",
			arch:        "amd64",
			expectedURL: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-x64.tar.gz",
			expectedSig: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-x64.tar.gz.sig",
			wantErr:     false,
		},
		{
			name:        "linux_arm64",
			version:     "2024.12.7",
			os:          "linux",
			arch:        "arm64",
			expectedURL: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-arm64.tar.gz",
			expectedSig: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-arm64.tar.gz.sig",
			wantErr:     false,
		},
		{
			name:        "linux_386",
			version:     "2024.12.7",
			os:          "linux",
			arch:        "386",
			expectedURL: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-x86.tar.gz",
			expectedSig: "https://github.com/jdx/mise/releases/download/v2024.12.7/mise-v2024.12.7-linux-x86.tar.gz.sig",
			wantErr:     false,
		},
		{
			name:    "unsupported_arch",
			version: "2024.12.7",
			os:      "linux",
			arch:    "mips",
			wantErr: true,
		},
		{
			name:    "unsupported_os",
			version: "2024.12.7",
			os:      "windows",
			arch:    "amd64",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &DownloadInfo{
				Binary:  BinaryMise,
				Version: tt.version,
				OS:      tt.os,
				Arch:    tt.arch,
			}

			result, err := constructMiseDownloadInfo(info, tt.version)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.URL != tt.expectedURL {
				t.Errorf("URL mismatch:\ngot:  %s\nwant: %s", result.URL, tt.expectedURL)
			}

			if result.SignatureURL != tt.expectedSig {
				t.Errorf("SignatureURL mismatch:\ngot:  %s\nwant: %s", result.SignatureURL, tt.expectedSig)
			}
		})
	}
}

func TestConstructChezmoiDownloadInfo(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		os             string
		arch           string
		expectedURL    string
		expectedSig    string
		expectedSum    string
		expectedBundle string
		wantErr        bool
	}{
		{
			name:           "linux_amd64",
			version:        "2.46.1",
			os:             "linux",
			arch:           "amd64",
			expectedURL:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi-2.46.1-x86_64-linux.tar.gz",
			expectedSig:    "", // chezmoi uses cosign, not GPG signatures
			expectedSum:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt",
			expectedBundle: "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt.sig",
			wantErr:        false,
		},
		{
			name:           "linux_arm64",
			version:        "2.46.1",
			os:             "linux",
			arch:           "arm64",
			expectedURL:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi-2.46.1-aarch64-linux.tar.gz",
			expectedSig:    "",
			expectedSum:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt",
			expectedBundle: "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt.sig",
			wantErr:        false,
		},
		{
			name:           "linux_386",
			version:        "2.46.1",
			os:             "linux",
			arch:           "386",
			expectedURL:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi-2.46.1-i686-linux.tar.gz",
			expectedSig:    "",
			expectedSum:    "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt",
			expectedBundle: "https://github.com/twpayne/chezmoi/releases/download/v2.46.1/chezmoi_2.46.1_checksums.txt.sig",
			wantErr:        false,
		},
		{
			name:    "unsupported_arch",
			version: "2.46.1",
			os:      "linux",
			arch:    "ppc64",
			wantErr: true,
		},
		{
			name:    "unsupported_os",
			version: "2.46.1",
			os:      "freebsd",
			arch:    "amd64",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &DownloadInfo{
				Binary:  BinaryChezmoi,
				Version: tt.version,
				OS:      tt.os,
				Arch:    tt.arch,
			}

			result, err := constructChezmoiDownloadInfo(info, tt.version)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.URL != tt.expectedURL {
				t.Errorf("URL mismatch:\ngot:  %s\nwant: %s", result.URL, tt.expectedURL)
			}

			if result.SignatureURL != tt.expectedSig {
				t.Errorf("SignatureURL mismatch:\ngot:  %s\nwant: %s", result.SignatureURL, tt.expectedSig)
			}

			if result.ChecksumURL != tt.expectedSum {
				t.Errorf("ChecksumURL mismatch:\ngot:  %s\nwant: %s", result.ChecksumURL, tt.expectedSum)
			}

			if result.BundleURL != tt.expectedBundle {
				t.Errorf("BundleURL mismatch:\ngot:  %s\nwant: %s", result.BundleURL, tt.expectedBundle)
			}
		})
	}
}

func TestConstructDownloadInfo(t *testing.T) {
	tests := []struct {
		name         string
		binary       Binary
		version      string
		platformInfo *platform.Info
		wantErr      bool
	}{
		{
			name:    "mise_linux_amd64",
			binary:  BinaryMise,
			version: "2024.12.7",
			platformInfo: &platform.Info{
				OS:   "linux",
				Arch: "amd64",
			},
			wantErr: false,
		},
		{
			name:    "chezmoi_linux_arm64",
			binary:  BinaryChezmoi,
			version: "2.46.1",
			platformInfo: &platform.Info{
				OS:   "linux",
				Arch: "arm64",
			},
			wantErr: false,
		},
		{
			name:         "nil_platform_info",
			binary:       BinaryMise,
			version:      "2024.12.7",
			platformInfo: nil,
			wantErr:      true,
		},
		{
			name:    "unknown_binary",
			binary:  Binary("unknown"),
			version: "1.0.0",
			platformInfo: &platform.Info{
				OS:   "linux",
				Arch: "amd64",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := constructDownloadInfo(tt.binary, tt.version, tt.platformInfo)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if info == nil {
				t.Fatal("expected non-nil info")
			}

			if info.Binary != tt.binary {
				t.Errorf("Binary mismatch: got %s, want %s", info.Binary, tt.binary)
			}

			if info.Version != tt.version {
				t.Errorf("Version mismatch: got %s, want %s", info.Version, tt.version)
			}
		})
	}
}

func TestBinaryString(t *testing.T) {
	tests := []struct {
		binary   Binary
		expected string
	}{
		{BinaryMise, "mise"},
		{BinaryChezmoi, "chezmoi"},
	}

	for _, tt := range tests {
		if got := tt.binary.String(); got != tt.expected {
			t.Errorf("Binary.String() = %q, want %q", got, tt.expected)
		}
	}
}

func TestVerificationMethodString(t *testing.T) {
	tests := []struct {
		method   VerificationMethod
		expected string
	}{
		{VerificationNone, "None"},
		{VerificationGPG, "GPG"},
		{VerificationSHA256, "SHA256"},
		{VerificationMethod(999), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.method.String(); got != tt.expected {
			t.Errorf("VerificationMethod.String() = %q, want %q", got, tt.expected)
		}
	}
}
