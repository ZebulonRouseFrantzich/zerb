package binary

import (
	"fmt"

	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
)

// constructDownloadInfo builds download URLs based on platform and binary type
func constructDownloadInfo(binary Binary, version string, platformInfo *platform.Info) (*DownloadInfo, error) {
	if platformInfo == nil {
		return nil, fmt.Errorf("platform info is required")
	}

	info := &DownloadInfo{
		Binary:  binary,
		Version: version,
		OS:      platformInfo.OS,
		Arch:    platformInfo.Arch,
	}

	switch binary {
	case BinaryMise:
		return constructMiseDownloadInfo(info, version)
	case BinaryChezmoi:
		return constructChezmoiDownloadInfo(info, version)
	default:
		return nil, fmt.Errorf("unknown binary: %s", binary)
	}
}

// constructMiseDownloadInfo constructs mise download URLs
// Pattern: https://github.com/jdx/mise/releases/download/v{version}/mise-v{version}-{os}-{arch}.tar.gz
func constructMiseDownloadInfo(info *DownloadInfo, version string) (*DownloadInfo, error) {
	// Map Go arch to mise arch naming
	archName, err := mapMiseArch(info.Arch)
	if err != nil {
		return nil, err
	}

	// Map Go OS to mise OS naming
	osName, err := mapMiseOS(info.OS)
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("https://github.com/jdx/mise/releases/download/v%s", version)
	binaryName := fmt.Sprintf("mise-v%s-%s-%s.tar.gz", version, osName, archName)

	info.URL = fmt.Sprintf("%s/%s", baseURL, binaryName)
	info.SignatureURL = fmt.Sprintf("%s/%s.sig", baseURL, binaryName)
	// mise provides checksums but we'll prefer GPG
	info.ChecksumURL = ""

	return info, nil
}

// constructChezmoiDownloadInfo constructs chezmoi download URLs
// Pattern: https://github.com/twpayne/chezmoi/releases/download/v{version}/chezmoi-{version}-{arch}-{os}.tar.gz
func constructChezmoiDownloadInfo(info *DownloadInfo, version string) (*DownloadInfo, error) {
	// Map Go arch to chezmoi arch naming
	archName, err := mapChezmoiArch(info.Arch)
	if err != nil {
		return nil, err
	}

	// Map Go OS to chezmoi OS naming
	osName, err := mapChezmoiOS(info.OS)
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("https://github.com/twpayne/chezmoi/releases/download/v%s", version)
	// Note: chezmoi uses {arch}-{os} order (reversed from mise)
	binaryName := fmt.Sprintf("chezmoi-%s-%s-%s.tar.gz", version, archName, osName)

	info.URL = fmt.Sprintf("%s/%s", baseURL, binaryName)
	info.SignatureURL = fmt.Sprintf("%s/%s.sig", baseURL, binaryName)
	info.ChecksumURL = fmt.Sprintf("%s/checksums.txt", baseURL)

	return info, nil
}

// mapMiseArch maps Go GOARCH values to mise architecture names
func mapMiseArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	case "arm":
		return "armv7", nil
	case "386":
		return "x86", nil
	default:
		return "", fmt.Errorf("unsupported architecture for mise: %s", goarch)
	}
}

// mapMiseOS maps Go GOOS values to mise OS names
func mapMiseOS(goos string) (string, error) {
	switch goos {
	case "linux":
		return "linux", nil
	case "darwin":
		return "darwin", nil
	default:
		return "", fmt.Errorf("unsupported OS for mise: %s (MVP supports Linux only)", goos)
	}
}

// mapChezmoiArch maps Go GOARCH values to chezmoi architecture names
func mapChezmoiArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x86_64", nil
	case "arm64":
		return "aarch64", nil
	case "arm":
		return "arm", nil
	case "386":
		return "i686", nil
	default:
		return "", fmt.Errorf("unsupported architecture for chezmoi: %s", goarch)
	}
}

// mapChezmoiOS maps Go GOOS values to chezmoi OS names
func mapChezmoiOS(goos string) (string, error) {
	switch goos {
	case "linux":
		return "linux", nil
	case "darwin":
		return "darwin", nil
	default:
		return "", fmt.Errorf("unsupported OS for chezmoi: %s (MVP supports Linux only)", goos)
	}
}
