package binary

import (
	"time"
)

// Binary represents a tool binary managed by ZERB
type Binary string

const (
	// BinaryMise represents the mise binary
	BinaryMise Binary = "mise"
	// BinaryChezmoi represents the chezmoi binary
	BinaryChezmoi Binary = "chezmoi"
)

// String returns the string representation of the binary
func (b Binary) String() string {
	return string(b)
}

// Version specifies hard-coded versions for mise and chezmoi
type Version struct {
	Mise    string
	Chezmoi string
}

// DefaultVersions contains the hard-coded binary versions used by ZERB
// These versions are tested and verified to work together
var DefaultVersions = Version{
	Mise:    "2024.12.7",
	Chezmoi: "2.46.1",
}

// DownloadOptions configures binary download and installation
type DownloadOptions struct {
	Binary  Binary
	Version string
	// SkipGPG skips GPG verification (for testing only)
	SkipGPG bool
	// UseMockDownload uses mock HTTP server (for testing only)
	UseMockDownload bool
}

// VerificationMethod indicates how a binary was verified
type VerificationMethod int

const (
	// VerificationNone indicates no verification (should never happen in production)
	VerificationNone VerificationMethod = iota
	// VerificationGPG indicates GPG signature verification was used
	VerificationGPG
	// VerificationSHA256 indicates SHA256 checksum verification was used
	VerificationSHA256
)

// String returns the string representation of the verification method
func (v VerificationMethod) String() string {
	switch v {
	case VerificationGPG:
		return "GPG"
	case VerificationSHA256:
		return "SHA256"
	case VerificationNone:
		return "None"
	default:
		return "Unknown"
	}
}

// DownloadResult contains information about a completed download
type DownloadResult struct {
	Binary       Binary
	Version      string
	Path         string
	Verified     VerificationMethod
	DownloadTime time.Duration
}

// DownloadInfo contains metadata needed to download a binary
type DownloadInfo struct {
	Binary       Binary
	Version      string
	OS           string // "linux", "darwin", etc.
	Arch         string // "amd64", "arm64", etc.
	URL          string // Constructed download URL
	SignatureURL string // GPG signature URL (may be empty)
	ChecksumURL  string // SHA256 checksum URL (may be empty)
}

// VerificationResult contains the outcome of a verification attempt
type VerificationResult struct {
	Method  VerificationMethod
	Success bool
	Error   error
}
