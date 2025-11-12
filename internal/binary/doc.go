// Package binary provides functionality for downloading, verifying, and managing
// the mise and chezmoi binaries that ZERB wraps.
//
// # Security Model
//
// Binary management is a critical security component of ZERB. All binaries are:
//   - Downloaded only from official GitHub releases
//   - Verified using GPG signatures (preferred) or SHA256 checksums (fallback)
//   - Never installed without successful verification
//
// # Verification Strategy
//
// 1. GPG Signature Verification (Preferred)
//   - Downloads .sig or .asc signature file
//   - Verifies using embedded GPG public keys
//   - Provides both authenticity and integrity verification
//
// 2. SHA256 Checksum Verification (Fallback)
//   - Downloads checksum file from GitHub release
//   - Verifies file integrity only (not authenticity)
//   - Used when GPG verification unavailable or fails
//
// # Usage
//
//	// Create a manager
//	mgr, err := binary.NewManager("/home/user/.config/zerb", platformInfo)
//	if err != nil {
//	    return err
//	}
//
//	// Extract embedded GPG keyrings
//	if err := mgr.ExtractKeyrings(); err != nil {
//	    return err
//	}
//
//	// Download and install mise
//	err = mgr.Install(ctx, binary.DownloadOptions{
//	    Binary:  binary.BinaryMise,
//	    Version: binary.DefaultVersions.Mise,
//	})
//
// # Architecture
//
// The package is organized into several components:
//   - Manager: High-level orchestration of download, verify, install
//   - Downloader: HTTP download with retry logic and caching
//   - Verifier: GPG and SHA256 verification
//   - Extractor: Archive extraction (tar.gz)
//   - Platform: Platform-specific URL construction
package binary
