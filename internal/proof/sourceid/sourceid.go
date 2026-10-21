// Package sourceid defines the one canonical source identity encoding used by
// every Phase 1 proof slice. It is disposable proof code, not a product API.
package sourceid

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceurl"
)

const (
	schemaVersion = 1
	domain        = "zerb.source.v1"

	KnownVectorURI    = "https://github.com/ZebulonRouseFrantzich/zerb.git"
	KnownVectorDigest = "sha256:ca92a55154224f40a4e208e1412d8c2530c2afb345b2dfd1a55b9627a7b1b62b"
)

type preimage struct {
	Schema int    `json:"schema"`
	Domain string `json:"domain"`
	URI    string `json:"uri"`
}

// Encode returns the versioned, map-free canonical JSON preimage for uri.
func Encode(uri string) ([]byte, error) {
	if err := sourceurl.ValidatePublicHTTPS(uri); err != nil {
		return nil, err
	}
	return json.Marshal(preimage{Schema: schemaVersion, Domain: domain, URI: uri})
}

// Digest returns the source identity of the canonical encoded preimage.
func Digest(uri string) (string, error) {
	encoded, err := Encode(uri)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
