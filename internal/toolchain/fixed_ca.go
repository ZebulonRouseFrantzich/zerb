// Package toolchain owns fixed acquisition authority under ARCH-030.
package toolchain

import (
	"bytes"
	"crypto/sha256"
	"errors"
)

const fixedCABytes = 472033

var errFixedCA = errors.New("invalid fixed CA bundle")

// FixedCA owns only the authenticated canonical bundle, never ambient roots.
type FixedCA struct {
	bundle []byte
	digest [32]byte
}

// NewFixedCA copies and completely validates the one reviewed ARCH-030 bundle.
func NewFixedCA(canonicalPEM []byte, reviewedSHA256 [32]byte) (*FixedCA, error) {
	if len(canonicalPEM) != fixedCABytes || reviewedSHA256 != [32]byte{
		0x3b, 0xa2, 0x00, 0x0f, 0xe3, 0xd5, 0x5c, 0xb6,
		0xc4, 0x13, 0x23, 0xfa, 0xbb, 0xb1, 0x05, 0x74,
		0x3e, 0x2c, 0x94, 0x22, 0x15, 0x1e, 0xea, 0x21,
		0x33, 0x11, 0x53, 0x8b, 0x07, 0xb2, 0x3f, 0x5a,
	} {
		return nil, errFixedCA
	}
	bundle := bytes.Clone(canonicalPEM)
	if sha256.Sum256(bundle) != reviewedSHA256 {
		return nil, errFixedCA
	}
	if err := validateCABundle(bundle); err != nil {
		return nil, err
	}
	return &FixedCA{bundle: bundle, digest: reviewedSHA256}, nil
}

// Bundle returns an independent copy; nil and zero capabilities have no bytes.
func (c *FixedCA) Bundle() []byte {
	if c == nil {
		return nil
	}
	return bytes.Clone(c.bundle)
}

func (c *FixedCA) SHA256() [32]byte {
	if c == nil {
		return [32]byte{}
	}
	return c.digest
}
