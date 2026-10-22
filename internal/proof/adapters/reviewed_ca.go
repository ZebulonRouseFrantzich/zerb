package adapters

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type reviewedPurpose uint8

const (
	reviewedServerAuth reviewedPurpose = 1 << iota
	reviewedEmailProtection
	reviewedCAOmission = "Traditional PEM block omitted: this certificate is not trusted for authenticating servers.\n"
)

// parseReviewedCAPool consumes buildcatrust 0.5.1's complete OpenSSL format.
// Its caller must first authenticate the exact frozen bundle (ARCH-011/012/030).
func parseReviewedCAPool(bundle []byte) (*x509.CertPool, error) {
	if len(bundle) == 0 || len(bundle) > maxFixedCABytes {
		return nil, errors.New("fixed CA bundle is empty or oversized")
	}
	roots := x509.NewCertPool()
	count := 0
	for remaining := bundle; len(remaining) != 0; {
		label, rest, found := bytes.Cut(remaining, []byte("\n"))
		if !found || len(label) == 0 || !utf8.Valid(label) || bytes.Contains(label, []byte("-----")) || bytes.IndexFunc(label, unicode.IsControl) >= 0 {
			return nil, errors.New("fixed CA bundle contains an invalid certificate label")
		}
		remaining = rest
		var ordinary *x509.Certificate
		if bytes.HasPrefix(remaining, []byte("-----BEGIN CERTIFICATE-----\n")) {
			block, rest, err := readReviewedPEM(remaining, "CERTIFICATE")
			if err != nil {
				return nil, err
			}
			ordinary, err = x509.ParseCertificate(block.Bytes)
			if err != nil || !bytes.HasPrefix(rest, []byte("\n")) {
				return nil, errors.New("fixed CA bundle contains an invalid ordinary certificate")
			}
			remaining = rest[1:]
		} else if bytes.HasPrefix(remaining, []byte(reviewedCAOmission)) {
			remaining = remaining[len(reviewedCAOmission):]
		} else {
			return nil, errors.New("fixed CA bundle lacks its ordinary certificate or omission record")
		}
		boundary := bytes.Index(remaining, []byte("-----"))
		if boundary < 0 {
			return nil, errors.New("fixed CA bundle lacks its trusted certificate record")
		}
		description := remaining[:boundary]
		block, rest, err := readReviewedPEM(remaining[boundary:], "TRUSTED CERTIFICATE")
		if err != nil {
			return nil, err
		}
		certificate, trust, reject, err := parseReviewedTrust(block.Bytes)
		if err != nil {
			return nil, err
		}
		if string(description) != reviewedPurposeDescription("Trusted for:", trust)+reviewedPurposeDescription("Rejected for:", reject) {
			return nil, errors.New("fixed CA bundle trust description disagrees with its AUX record")
		}
		if ordinary != nil {
			if !bytes.Equal(ordinary.Raw, certificate.Raw) || trust&reviewedServerAuth == 0 || reject != 0 {
				return nil, errors.New("fixed CA ordinary certificate lacks matching affirmative server trust")
			}
			roots.AddCert(ordinary)
			count++
		} else if trust&reviewedServerAuth != 0 {
			return nil, errors.New("fixed CA server trust lacks its ordinary certificate")
		}
		if !bytes.HasPrefix(rest, []byte("\n")) {
			return nil, errors.New("fixed CA bundle lacks its record separator")
		}
		remaining = rest[1:]
	}
	if count == 0 {
		return nil, errors.New("fixed CA bundle contains no server certificates")
	}
	return roots, nil
}

func readReviewedPEM(input []byte, kind string) (*pem.Block, []byte, error) {
	if !bytes.HasPrefix(input, []byte("-----BEGIN "+kind+"-----\n")) {
		return nil, nil, errors.New("fixed CA bundle contains an unexpected PEM boundary")
	}
	footer := []byte("-----END " + kind + "-----\n")
	end := bytes.Index(input, footer)
	if end < 0 {
		return nil, nil, errors.New("fixed CA bundle contains incomplete PEM")
	}
	end += len(footer)
	encoded := input[:end]
	block, rest := pem.Decode(encoded)
	// Decode only this bounded block, then compare all bytes: pem.Decode may skip malformed prefixes.
	if block == nil || block.Type != kind || len(block.Headers) != 0 || len(rest) != 0 || !bytes.Equal(encoded, pem.EncodeToMemory(block)) {
		return nil, nil, errors.New("fixed CA bundle contains invalid or non-canonical PEM")
	}
	return block, input[end:], nil
}

func parseReviewedTrust(input []byte) (*x509.Certificate, reviewedPurpose, reviewedPurpose, error) {
	invalid := errors.New("fixed CA bundle contains invalid certificate trust DER")
	var certificateDER, aux, trustDER, rejectDER asn1.RawValue
	remaining, err := asn1.Unmarshal(input, &certificateDER)
	if err != nil || certificateDER.Class != asn1.ClassUniversal || certificateDER.Tag != asn1.TagSequence || !certificateDER.IsCompound {
		return nil, 0, 0, invalid
	}
	certificate, err := x509.ParseCertificate(certificateDER.FullBytes)
	if err != nil {
		return nil, 0, 0, invalid
	}
	rest, err := asn1.Unmarshal(remaining, &aux)
	if err != nil || len(rest) != 0 || aux.Class != asn1.ClassUniversal || aux.Tag != asn1.TagSequence || !aux.IsCompound {
		return nil, 0, 0, invalid
	}
	remaining, err = asn1.Unmarshal(aux.Bytes, &trustDER)
	if err != nil || trustDER.Class != asn1.ClassUniversal || trustDER.Tag != asn1.TagSequence || !trustDER.IsCompound {
		return nil, 0, 0, invalid
	}
	trust, err := parseReviewedPurposes(trustDER.Bytes)
	if err != nil {
		return nil, 0, 0, err
	}
	var reject reviewedPurpose
	if len(remaining) != 0 {
		rest, err = asn1.Unmarshal(remaining, &rejectDER)
		// The builder emits nonempty [0] IMPLICIT SEQUENCE OF OID, never an explicit wrapper.
		if err != nil || len(rest) != 0 || rejectDER.Class != asn1.ClassContextSpecific || rejectDER.Tag != 0 || !rejectDER.IsCompound || len(rejectDER.Bytes) == 0 {
			return nil, 0, 0, invalid
		}
		reject, err = parseReviewedPurposes(rejectDER.Bytes)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	if trust&reject != 0 {
		return nil, 0, 0, errors.New("fixed CA bundle contains conflicting trust purposes")
	}
	return certificate, trust, reject, nil
}

func parseReviewedPurposes(input []byte) (reviewedPurpose, error) {
	var purposes, previous reviewedPurpose
	for len(input) != 0 {
		var oid asn1.ObjectIdentifier
		rest, err := asn1.Unmarshal(input, &oid)
		if err != nil {
			return 0, errors.New("fixed CA bundle contains an invalid trust purpose")
		}
		var purpose reviewedPurpose
		switch oid.String() {
		case "1.3.6.1.5.5.7.3.1":
			purpose = reviewedServerAuth
		case "1.3.6.1.5.5.7.3.4":
			purpose = reviewedEmailProtection
		default:
			return 0, errors.New("fixed CA bundle contains an unreviewed trust purpose")
		}
		if purpose <= previous {
			return 0, errors.New("fixed CA bundle contains duplicate or unordered trust purposes")
		}
		purposes |= purpose
		previous = purpose
		input = rest
	}
	return purposes, nil
}

func reviewedPurposeDescription(heading string, purposes reviewedPurpose) string {
	if purposes == 0 {
		return ""
	}
	var description strings.Builder
	description.WriteString(heading + "\n")
	if purposes&reviewedServerAuth != 0 {
		description.WriteString("  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n")
	}
	if purposes&reviewedEmailProtection != 0 {
		description.WriteString("  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n")
	}
	return description.String()
}
