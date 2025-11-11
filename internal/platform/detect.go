package platform

import (
	"context"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/host"
)

// RealDetector implements Detector using actual platform detection.
type RealDetector struct{}

// NewDetector creates a new platform detector.
func NewDetector() Detector {
	return &RealDetector{}
}

// Detect performs platform detection and returns platform information.
// It uses runtime.GOOS and runtime.GOARCH for OS and architecture,
// and gopsutil for Linux distribution details.
//
// On Linux, if gopsutil fails to detect the distribution, it sets
// distro fields to empty strings and continues (graceful fallback).
// This allows basic OS/arch detection to work even when distro
// detection fails.
func (d *RealDetector) Detect(ctx context.Context) (*Info, error) {
	info := &Info{
		OS:      runtime.GOOS,
		ArchRaw: runtime.GOARCH,
	}

	// Normalize architecture (MVP: only amd64 and arm64 supported)
	arch, err := normalizeArch(runtime.GOARCH)
	if err != nil {
		return nil, fmt.Errorf("platform detection failed: %w", err)
	}
	info.Arch = arch

	// Detect Linux distribution details using gopsutil (Linux only)
	if runtime.GOOS == "linux" {
		platform, family, version, err := host.PlatformInformationWithContext(ctx)
		if err != nil {
			// Check if context was cancelled - this is a hard failure
			if ctx.Err() != nil {
				return nil, fmt.Errorf("platform detection cancelled: %w", ctx.Err())
			}
			// Graceful fallback for detection failures only
			// Continue with OS/arch only - most configs won't need distro-specific logic
			return info, nil
		}

		// Normalize and validate platform information
		platform = normalizePlatform(platform)
		family = mapFamily(family)
		version = normalizePlatform(version)

		// Only set fields if we got valid data
		if platform != "" {
			info.Platform = platform
			info.Family = family
			info.Version = version
		}
	}

	return info, nil
}
