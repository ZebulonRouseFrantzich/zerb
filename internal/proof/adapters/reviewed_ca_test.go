package adapters

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// VAL-017/021/022: the actual approved bundle must supply exactly its TLS roots.
func TestReviewedCAFrozenBundle(t *testing.T) {
	bundle, err := os.ReadFile("testdata/reviewed-ca-bundle.pem")
	if err != nil {
		t.Fatal(err)
	}
	roots, err := reviewedCAPool(bundle)
	if err != nil {
		t.Fatalf("approved bundle rejected: %v", err)
	}
	want := x509.NewCertPool()
	if !want.AppendCertsFromPEM(bundle) || !roots.Equal(want) || bytes.Count(bundle, []byte("-----BEGIN CERTIFICATE-----\n")) != 121 {
		t.Fatal("reviewed pool differs from the approved bundle's 121 ordinary TLS certificates")
	}
	name := filepath.Join(t.TempDir(), "approved-ca.pem")
	if err := os.WriteFile(name, bundle, 0600); err != nil {
		t.Fatal(err)
	}
	opened, err := readFixedCA(name)
	if err != nil || !bytes.Equal(opened, bundle) {
		t.Fatalf("approved opened CA bytes rejected or changed: %v", err)
	}
}

func TestReviewedCAPoolExcludesAUXOnlyRootsAndAmbientTrust(t *testing.T) {
	bundle, err := os.ReadFile("testdata/reviewed-ca-bundle.pem")
	if err != nil {
		t.Fatal(err)
	}
	ambientPEM := testCAPEM(t, 31)
	ambientBlock, _ := pem.Decode(ambientPEM)
	ambientCertificate, err := x509.ParseCertificate(ambientBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ambient := filepath.Join(t.TempDir(), "ambient-ca.pem")
	if err := os.WriteFile(ambient, ambientPEM, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", ambient)
	t.Setenv("SSL_CERT_DIR", filepath.Dir(ambient))
	roots, err := reviewedCAPool(bundle)
	if err != nil {
		t.Fatal(err)
	}
	independent, err := reviewedCAPool(bundle)
	if err != nil {
		t.Fatal(err)
	}
	independent.AddCert(ambientCertificate)
	want := x509.NewCertPool()
	if !want.AppendCertsFromPEM(bundle) || !roots.Equal(want) || roots.Equal(independent) {
		t.Fatal("ambient trust or a separately modified pool changed the reviewed pool")
	}
	ordinary := make(map[string]bool)
	var trusted []*x509.Certificate
	for remaining := bundle; ; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			ordinary[string(block.Bytes)] = true
		} else {
			var der asn1.RawValue
			if _, err := asn1.Unmarshal(block.Bytes, &der); err != nil {
				t.Fatal(err)
			}
			certificate, err := x509.ParseCertificate(der.FullBytes)
			if err != nil {
				t.Fatal(err)
			}
			trusted = append(trusted, certificate)
		}
		remaining = rest
	}
	clear(bundle)
	if len(ordinary) != 121 || len(trusted) != 172 {
		t.Fatal("approved fixture changed its ordinary or trusted certificate inventory")
	}
	excluded := 0
	for _, certificate := range trusted {
		// Each exact root is checked as its own trust anchor, inside its validity period.
		_, err := certificate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: certificate.NotBefore.Add(time.Second), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
		if ordinary[string(certificate.Raw)] {
			if err != nil {
				t.Fatalf("ordinary server root lost after input mutation: %v", err)
			}
		} else {
			excluded++
			if err == nil {
				t.Fatal("AUX-only root entered the TLS pool")
			}
		}
	}
	if excluded != 51 {
		t.Fatalf("excluded %d AUX-only roots; want 51", excluded)
	}
}

func TestReviewedCARejectsIncompleteOrInconsistentRecords(t *testing.T) {
	ordinary, _ := pem.Decode(testCAPEM(t, 32))
	other, _ := pem.Decode(testCAPEM(t, 33))
	server := reviewedTestASN(t, asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1})
	email := reviewedTestASN(t, asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 4})
	unknown := reviewedTestASN(t, asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 3})
	sequence := func(children ...[]byte) []byte {
		return reviewedTestASN(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: bytes.Join(children, nil)})
	}
	implicit := func(contents []byte) []byte {
		return reviewedTestASN(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: contents})
	}
	serverAux := sequence(sequence(server))
	serverDescription := "Trusted for:\n  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n"
	emailDescription := "Trusted for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"
	record := func(plain, certificate, aux []byte, description string) []byte {
		return reviewedTestRecord(plain, certificate, aux, description)
	}
	valid := record(ordinary.Bytes, ordinary.Bytes, serverAux, serverDescription)
	for name, input := range map[string][]byte{
		"empty":                          nil,
		"only email root":                record(nil, other.Bytes, sequence(sequence(email)), emailDescription),
		"missing ordinary pair":          record(nil, ordinary.Bytes, serverAux, serverDescription),
		"mismatched pair":                record(ordinary.Bytes, other.Bytes, serverAux, serverDescription),
		"ordinary lacks server trust":    record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(email)), emailDescription),
		"ordinary has empty trust":       record(ordinary.Bytes, ordinary.Bytes, sequence(sequence()), ""),
		"unknown purpose":                record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(unknown)), ""),
		"duplicate purpose":              record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(server, server)), serverDescription),
		"unordered purposes":             record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(email, server)), serverDescription),
		"conflicting purposes":           record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(server), implicit(server)), serverDescription),
		"server reject":                  record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(), implicit(server)), "Rejected for:\n  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n"),
		"ordinary has another reject":    record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(server), implicit(email)), serverDescription+"Rejected for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"),
		"explicit reject":                record(nil, ordinary.Bytes, sequence(sequence(), implicit(sequence(email))), ""),
		"empty reject":                   record(nil, ordinary.Bytes, sequence(sequence(), implicit(nil)), ""),
		"unknown AUX field":              record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(server), reviewedTestASN(t, "alias")), serverDescription),
		"extra inner AUX sequence":       record(ordinary.Bytes, ordinary.Bytes, sequence(sequence(server), sequence()), serverDescription),
		"extra inner field after reject": record(nil, ordinary.Bytes, sequence(sequence(), implicit(email), sequence()), ""),
		"missing mandatory trust":        record(nil, ordinary.Bytes, sequence(), ""),
		"missing AUX":                    record(ordinary.Bytes, ordinary.Bytes, nil, serverDescription),
		"trailing DER":                   record(ordinary.Bytes, ordinary.Bytes, append(bytes.Clone(serverAux), 5, 0), serverDescription),
		"invalid trusted certificate":    record(ordinary.Bytes, []byte{0x30, 0}, serverAux, serverDescription),
		"ordinary trailing DER":          record(append(bytes.Clone(ordinary.Bytes), 5, 0), ordinary.Bytes, serverAux, serverDescription),
		"description disagrees":          record(ordinary.Bytes, ordinary.Bytes, serverAux, emailDescription),
		"missing description":            record(ordinary.Bytes, ordinary.Bytes, serverAux, ""),
		"missing final separator":        bytes.TrimSuffix(valid, []byte("\n")),
		"trailing text":                  append(bytes.Clone(valid), []byte("junk\n")...),
		"unknown PEM type":               bytes.ReplaceAll(valid, []byte("TRUSTED CERTIFICATE"), []byte("PUBLIC KEY")),
		"PEM headers":                    bytes.Replace(valid, []byte("-----BEGIN TRUSTED CERTIFICATE-----\n"), []byte("-----BEGIN TRUSTED CERTIFICATE-----\nComment: unsupported\n\n"), 1),
		"noncanonical PEM":               bytes.Replace(valid, []byte("-----BEGIN CERTIFICATE-----\n"), []byte("-----BEGIN CERTIFICATE-----\r\n"), 1),
		"malformed earlier PEM":          bytes.Replace(valid, []byte("-----BEGIN CERTIFICATE-----\n"), []byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\n"), 1),
		"malformed later PEM":            append(bytes.Clone(valid), bytes.Replace(valid, []byte("-----BEGIN TRUSTED CERTIFICATE-----\n"), []byte("-----BEGIN TRUSTED CERTIFICATE-----\ninvalid\n-----END TRUSTED CERTIFICATE-----\n-----BEGIN TRUSTED CERTIFICATE-----\n"), 1)...),
	} {
		t.Run(name, func(t *testing.T) {
			if pool, err := parseReviewedCAPool(input); err == nil || pool != nil {
				t.Fatal("incomplete or inconsistent reviewed CA record accepted")
			}
		})
	}
	// An implicit rejection is valid builder grammar, but never adds an AUX-only root.
	auxOnly := record(nil, other.Bytes, sequence(sequence(), implicit(email)), "Rejected for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n")
	pool, err := parseReviewedCAPool(append(bytes.Clone(valid), auxOnly...))
	want, wantErr := parseCAPool(pem.EncodeToMemory(ordinary))
	if err != nil || wantErr != nil || !pool.Equal(want) {
		t.Fatalf("implicit rejection changed the ordinary server roots: %v %v", err, wantErr)
	}
	if pool, err := reviewedCAPool(valid); err == nil || pool != nil || !strings.Contains(err.Error(), "reviewed digest") {
		t.Fatal("valid alternate builder input bypassed the exact digest gate")
	}
	if pool, err := reviewedCAPool([]byte("malformed")); err == nil || pool != nil || !strings.Contains(err.Error(), "reviewed digest") {
		t.Fatal("format parsing preceded exact digest authentication")
	}
}

func reviewedTestASN(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := asn1.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func reviewedTestRecord(ordinary, certificate, aux []byte, description string) []byte {
	var record bytes.Buffer
	record.WriteString("disposable test CA\n")
	if ordinary == nil {
		record.WriteString(reviewedCAOmission)
	} else {
		record.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ordinary}))
		record.WriteByte('\n')
	}
	record.WriteString(description)
	record.Write(pem.EncodeToMemory(&pem.Block{Type: "TRUSTED CERTIFICATE", Bytes: append(bytes.Clone(certificate), aux...)}))
	record.WriteByte('\n')
	return record.Bytes()
}
