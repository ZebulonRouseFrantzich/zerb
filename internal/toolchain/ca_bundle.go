package toolchain

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"strings"
	"unicode"
	"unicode/utf8"
)

type caPurposes uint8

const (
	caServer caPurposes = 1 << iota
	caEmail
	caOmission = "Traditional PEM block omitted: this certificate is not trusted for authenticating servers.\n"
)

// validateCABundle consumes every authenticated buildcatrust 0.5.1 record.
func validateCABundle(bundle []byte) error {
	if len(bundle) == 0 || len(bundle) > fixedCABytes {
		return errFixedCA
	}
	servers := 0
	for remaining := bundle; len(remaining) != 0; {
		label, rest, ok := bytes.Cut(remaining, []byte("\n"))
		if !ok || len(label) == 0 || !utf8.Valid(label) || bytes.Contains(label, []byte("-----")) || bytes.IndexFunc(label, unicode.IsControl) >= 0 {
			return errFixedCA
		}
		remaining = rest
		var ordinary *x509.Certificate
		if bytes.HasPrefix(remaining, []byte("-----BEGIN CERTIFICATE-----\n")) {
			block, rest, err := readCAPEM(remaining, "CERTIFICATE")
			if err != nil {
				return err
			}
			ordinary, err = parseCACertificate(block.Bytes)
			if err != nil || !bytes.HasPrefix(rest, []byte("\n")) {
				return errFixedCA
			}
			remaining = rest[1:]
		} else if bytes.HasPrefix(remaining, []byte(caOmission)) {
			remaining = remaining[len(caOmission):]
		} else {
			return errFixedCA
		}
		boundary := bytes.Index(remaining, []byte("-----"))
		if boundary < 0 {
			return errFixedCA
		}
		description := remaining[:boundary]
		block, rest, err := readCAPEM(remaining[boundary:], "TRUSTED CERTIFICATE")
		if err != nil {
			return err
		}
		certificate, trust, reject, err := readCATrust(block.Bytes)
		if err != nil {
			return err
		}
		if string(description) != caPurposeDescription("Trusted for:", trust)+caPurposeDescription("Rejected for:", reject) {
			return errFixedCA
		}
		if ordinary != nil {
			if !bytes.Equal(ordinary.Raw, certificate.Raw) || trust&caServer == 0 || reject != 0 {
				return errFixedCA
			}
			servers++
		} else if trust&caServer != 0 {
			return errFixedCA
		}
		if !bytes.HasPrefix(rest, []byte("\n")) {
			return errFixedCA
		}
		remaining = rest[1:]
	}
	if servers == 0 {
		return errFixedCA
	}
	return nil
}

func readCAPEM(input []byte, kind string) (*pem.Block, []byte, error) {
	if !bytes.HasPrefix(input, []byte("-----BEGIN "+kind+"-----\n")) {
		return nil, nil, errFixedCA
	}
	footer := []byte("-----END " + kind + "-----\n")
	end := bytes.Index(input, footer)
	if end < 0 {
		return nil, nil, errFixedCA
	}
	end += len(footer)
	encoded := input[:end]
	block, rest := pem.Decode(encoded)
	// Isolate and round-trip one block: pem.Decode otherwise skips bad prefixes.
	if block == nil || block.Type != kind || len(block.Headers) != 0 || len(rest) != 0 || !bytes.Equal(encoded, pem.EncodeToMemory(block)) {
		return nil, nil, errFixedCA
	}
	return block, input[end:], nil
}

func readCATrust(input []byte) (*x509.Certificate, caPurposes, caPurposes, error) {
	var cert, aux, trust, reject asn1.RawValue
	rest, err := asn1.Unmarshal(input, &cert)
	if err != nil || cert.Class != asn1.ClassUniversal || cert.Tag != asn1.TagSequence || !cert.IsCompound {
		return nil, 0, 0, errFixedCA
	}
	certificate, err := parseCACertificate(cert.FullBytes)
	if err != nil {
		return nil, 0, 0, errFixedCA
	}
	extra, err := asn1.Unmarshal(rest, &aux)
	if err != nil || len(extra) != 0 || aux.Class != asn1.ClassUniversal || aux.Tag != asn1.TagSequence || !aux.IsCompound {
		return nil, 0, 0, errFixedCA
	}
	rest, err = asn1.Unmarshal(aux.Bytes, &trust)
	if err != nil || trust.Class != asn1.ClassUniversal || trust.Tag != asn1.TagSequence || !trust.IsCompound {
		return nil, 0, 0, errFixedCA
	}
	trusted, err := readCAPurposes(trust.Bytes)
	if err != nil {
		return nil, 0, 0, err
	}
	var rejected caPurposes
	if len(rest) != 0 {
		extra, err = asn1.Unmarshal(rest, &reject)
		if err != nil || len(extra) != 0 || reject.Class != asn1.ClassContextSpecific || reject.Tag != 0 || !reject.IsCompound || len(reject.Bytes) == 0 {
			return nil, 0, 0, errFixedCA
		}
		rejected, err = readCAPurposes(reject.Bytes)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	if trusted&rejected != 0 {
		return nil, 0, 0, errFixedCA
	}
	return certificate, trusted, rejected, nil
}

func readCAPurposes(input []byte) (caPurposes, error) {
	var purposes, previous caPurposes
	for len(input) != 0 {
		var oid asn1.ObjectIdentifier
		rest, err := asn1.Unmarshal(input, &oid)
		if err != nil {
			return 0, errFixedCA
		}
		var purpose caPurposes
		switch oid.String() {
		case "1.3.6.1.5.5.7.3.1":
			purpose = caServer
		case "1.3.6.1.5.5.7.3.4":
			purpose = caEmail
		default:
			return 0, errFixedCA
		}
		if purpose <= previous {
			return 0, errFixedCA
		}
		purposes |= purpose
		previous = purpose
		input = rest
	}
	return purposes, nil
}

func caPurposeDescription(heading string, purposes caPurposes) string {
	if purposes == 0 {
		return ""
	}
	var text strings.Builder
	text.WriteString(heading + "\n")
	if purposes&caServer != 0 {
		text.WriteString("  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n")
	}
	if purposes&caEmail != 0 {
		text.WriteString("  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n")
	}
	return text.String()
}
