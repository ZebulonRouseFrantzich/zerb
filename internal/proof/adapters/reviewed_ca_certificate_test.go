package adapters

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"os"
	"testing"
)

func reviewedDERTestASN(t *testing.T, value any) []byte {
	t.Helper()
	der, err := asn1.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// Insert a NULL without changing any existing child; rebuild ancestor lengths.
func reviewedDERTestInsert(t *testing.T, der []byte, path ...int) []byte {
	t.Helper()
	var value asn1.RawValue
	rest, err := asn1.Unmarshal(der, &value)
	if err != nil || len(rest) != 0 || !value.IsCompound {
		t.Fatalf("invalid fixture DER container: %v", err)
	}
	if len(path) == 0 {
		value.Bytes = append(bytes.Clone(value.Bytes), 5, 0)
	} else {
		var children [][]byte
		for remaining := value.Bytes; len(remaining) != 0; {
			var child asn1.RawValue
			rest, err := asn1.Unmarshal(remaining, &child)
			if err != nil {
				t.Fatal(err)
			}
			children = append(children, child.FullBytes)
			remaining = rest
		}
		if path[0] < 0 || path[0] >= len(children) {
			t.Fatal("fixture DER child index out of bounds")
		}
		children[path[0]] = reviewedDERTestInsert(t, children[path[0]], path[1:]...)
		value.Bytes = bytes.Join(children, nil)
	}
	value.FullBytes = nil
	return reviewedDERTestASN(t, value)
}

func reviewedDERTestRecord(t *testing.T, ordinary, certificate []byte, purpose int) []byte {
	t.Helper()
	oid := reviewedDERTestASN(t, asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, purpose})
	trust := reviewedDERTestASN(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: oid})
	aux := reviewedDERTestASN(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: trust})
	var record bytes.Buffer
	record.WriteString("disposable public DER parity record\n")
	if ordinary == nil {
		record.WriteString("Traditional PEM block omitted: this certificate is not trusted for authenticating servers.\n")
	} else {
		record.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ordinary}))
		record.WriteByte('\n')
	}
	if purpose == 1 {
		record.WriteString("Trusted for:\n  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n")
	} else {
		record.WriteString("Trusted for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n")
	}
	record.Write(pem.EncodeToMemory(&pem.Block{Type: "TRUSTED CERTIFICATE", Bytes: append(bytes.Clone(certificate), aux...)}))
	record.WriteByte('\n')
	return record.Bytes()
}

// ARCH-030/VAL-017/021/022: proof readers must consume the same complete DER.
func TestReviewedCACertificateDERParity(t *testing.T) {
	bundle, err := os.ReadFile("testdata/reviewed-ca-bundle.pem")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bundle)
	if len(bundle) != 472033 || hex.EncodeToString(digest[:]) != "3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a" {
		t.Fatal("reviewed public fixture identity changed")
	}
	first, _ := pem.Decode(bundle)
	if first == nil || first.Type != "CERTIFICATE" {
		t.Fatal("fixture lacks its first ordinary certificate")
	}
	valid := reviewedDERTestRecord(t, first.Bytes, first.Bytes, 1)
	plain := pem.EncodeToMemory(first)
	t.Run("positive controls", func(t *testing.T) {
		ordinary := make(map[string]struct{})
		for remaining := bundle; len(remaining) != 0; {
			block, rest := pem.Decode(remaining)
			if block == nil {
				t.Fatal("fixture contains an incomplete certificate inventory")
			}
			if block.Type == "CERTIFICATE" {
				ordinary[string(block.Bytes)] = struct{}{}
			}
			remaining = bytes.TrimSpace(rest)
		}
		want := x509.NewCertPool()
		if !want.AppendCertsFromPEM(bundle) || len(ordinary) != 121 {
			t.Fatal("fixture ordinary root inventory changed")
		}
		for _, parse := range []func([]byte) (*x509.CertPool, error){reviewedCAPool, parseReviewedCAPool} {
			original := bytes.Clone(bundle)
			pool, err := parse(bundle)
			if err != nil || pool == nil || !pool.Equal(want) || !bytes.Equal(bundle, original) {
				t.Fatalf("reviewed 121-root bundle rejected or changed: %v", err)
			}
		}
		one := x509.NewCertPool()
		if !one.AppendCertsFromPEM(plain) {
			t.Fatal("original certificate could not form a root pool")
		}
		for _, test := range []struct {
			input []byte
			parse func([]byte) (*x509.CertPool, error)
		}{
			{plain, parseCAPool},
			{valid, parseReviewedCAPool},
			{append(bytes.Clone(valid), reviewedDERTestRecord(t, nil, first.Bytes, 4)...), parseReviewedCAPool},
		} {
			original := bytes.Clone(test.input)
			pool, err := test.parse(test.input)
			if err != nil || pool == nil || !pool.Equal(one) || !bytes.Equal(test.input, original) {
				t.Fatalf("original complete certificate rejected or changed: %v", err)
			}
		}
	})
	mutations := map[string][]byte{
		"certificate":          reviewedDERTestInsert(t, first.Bytes),
		"TBSCertificate":       reviewedDERTestInsert(t, first.Bytes, 0),
		"signature algorithms": reviewedDERTestInsert(t, reviewedDERTestInsert(t, first.Bytes, 0, 2), 1),
		"validity":             reviewedDERTestInsert(t, first.Bytes, 0, 4),
		"SPKI":                 reviewedDERTestInsert(t, first.Bytes, 0, 6),
		"SPKI algorithm":       reviewedDERTestInsert(t, first.Bytes, 0, 6, 0),
		"issuer attribute":     reviewedDERTestInsert(t, first.Bytes, 0, 3, 0, 0),
		"subject attribute":    reviewedDERTestInsert(t, first.Bytes, 0, 5, 0, 0),
		"extensions wrapper":   reviewedDERTestInsert(t, first.Bytes, 0, 7),
		"extension":            reviewedDERTestInsert(t, first.Bytes, 0, 7, 0, 0),
	}
	for name, certificate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := x509.ParseCertificate(certificate); err != nil {
				t.Fatalf("fixture mutation no longer reaches unconsumed DER: %v", err)
			}
			for _, test := range []struct {
				name, wantError string
				input           []byte
				parse           func([]byte) (*x509.CertPool, error)
			}{
				{"matched records", "fixed CA bundle contains an invalid ordinary certificate", reviewedDERTestRecord(t, certificate, certificate, 1), parseReviewedCAPool},
				{"AUX-only trusted", "fixed CA bundle contains invalid certificate trust DER", append(bytes.Clone(valid), reviewedDERTestRecord(t, nil, certificate, 4)...), parseReviewedCAPool},
				{"ordinary PEM", "fixed CA bundle contains an invalid certificate", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), parseCAPool},
			} {
				t.Run(test.name, func(t *testing.T) {
					original := bytes.Clone(test.input)
					pool, err := test.parse(test.input)
					if pool != nil || err == nil {
						t.Error("proof parser accepted an unconsumed certificate DER child")
					} else if err.Error() != test.wantError {
						t.Error("certificate rejection did not return the fixed redacted error")
					}
					pool, err = reviewedCAPool(test.input)
					if pool != nil || err == nil || err.Error() != "fixed CA bundle disagrees with reviewed digest" {
						t.Error("substituted certificate bypassed reviewed digest authentication")
					}
					if !bytes.Equal(test.input, original) {
						t.Error("proof parser changed caller bytes")
					}
				})
			}
		})
	}
}
