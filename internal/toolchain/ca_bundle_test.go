package toolchain

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"testing"
)

const (
	caTestServerDescription = "Trusted for:\n  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n"
	caTestEmailDescription  = "Trusted for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"
	caTestBothDescription   = caTestServerDescription + "  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"
	caTestEmailRejection    = "Rejected for:\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"
	caTestBothRejection     = "Rejected for:\n  - 1.3.6.1.5.5.7.3.1 (RFC5280: serverAuth key usage)\n  - 1.3.6.1.5.5.7.3.4 (RFC5280: emailProtection key usage)\n"
)

func caTestCertificates(t testing.TB) ([]byte, []byte) {
	t.Helper()
	bundle, _ := fixedCATestBundle(t)
	var certificates [][]byte
	for remaining := bundle; len(certificates) < 2; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			t.Fatal("fixture lacks two ordinary certificates")
			return nil, nil
		}
		if block.Type == "CERTIFICATE" {
			certificates = append(certificates, block.Bytes)
		}
		remaining = rest
	}
	return certificates[0], certificates[1]
}

func caTestASN(t testing.TB, value any) []byte {
	t.Helper()
	encoded, err := asn1.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func caTestOID(t testing.TB, purpose int) []byte {
	t.Helper()
	return caTestASN(t, asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, purpose})
}

func caTestSequence(t testing.TB, children ...[]byte) []byte {
	t.Helper()
	return caTestASN(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: bytes.Join(children, nil)})
}

func caTestReject(t testing.TB, contents []byte) []byte {
	t.Helper()
	return caTestASN(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: contents})
}

func caTestRecord(ordinary, certificate, aux []byte, description string) []byte {
	var record bytes.Buffer
	record.WriteString("disposable public CA record\n")
	if ordinary == nil {
		record.WriteString("Traditional PEM block omitted: this certificate is not trusted for authenticating servers.\n")
	} else {
		record.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ordinary}))
		record.WriteByte('\n')
	}
	record.WriteString(description)
	record.Write(pem.EncodeToMemory(&pem.Block{Type: "TRUSTED CERTIFICATE", Bytes: append(bytes.Clone(certificate), aux...)}))
	record.WriteByte('\n')
	return record.Bytes()
}

func TestCABundleAcceptsCompleteBuilderRecords(t *testing.T) {
	first, second := caTestCertificates(t)
	server := caTestOID(t, 1)
	email := caTestOID(t, 4)
	valid := caTestRecord(first, first, caTestSequence(t, caTestSequence(t, server)), caTestServerDescription)
	for name, input := range map[string][]byte{
		"ordinary server":         valid,
		"server and email":        caTestRecord(first, first, caTestSequence(t, caTestSequence(t, server, email)), caTestBothDescription),
		"AUX-only email":          append(bytes.Clone(valid), caTestRecord(nil, second, caTestSequence(t, caTestSequence(t, email)), caTestEmailDescription)...),
		"AUX-only rejected email": append(bytes.Clone(valid), caTestRecord(nil, second, caTestSequence(t, caTestSequence(t), caTestReject(t, email)), caTestEmailRejection)...),
		"AUX-only empty purposes": append(bytes.Clone(valid), caTestRecord(nil, second, caTestSequence(t, caTestSequence(t)), "")...),
	} {
		t.Run(name, func(t *testing.T) {
			original := bytes.Clone(input)
			if err := validateCABundle(input); err != nil {
				t.Fatalf("valid complete builder records rejected: %v", err)
			}
			if !bytes.Equal(input, original) {
				t.Fatal("validation mutated caller bytes")
			}
		})
	}
}

// ARCH-030 requires complete record, PEM and AUX validation after authentication.
func TestCABundleRejectsMalformedRecords(t *testing.T) {
	first, second := caTestCertificates(t)
	server, email, unknown := caTestOID(t, 1), caTestOID(t, 4), caTestOID(t, 3)
	sequence := func(children ...[]byte) []byte { return caTestSequence(t, children...) }
	reject := func(contents []byte) []byte { return caTestReject(t, contents) }
	serverAux := sequence(sequence(server))
	record := func(ordinary, certificate, aux []byte, description string) []byte {
		return caTestRecord(ordinary, certificate, aux, description)
	}
	valid := record(first, first, serverAux, caTestServerDescription)
	badTrusted := bytes.Replace(valid, []byte("-----BEGIN TRUSTED CERTIFICATE-----\n"), []byte("-----BEGIN TRUSTED CERTIFICATE-----\ninvalid\n-----END TRUSTED CERTIFICATE-----\n-----BEGIN TRUSTED CERTIFICATE-----\n"), 1)
	for name, input := range map[string][]byte{
		"empty":                             nil,
		"oversized":                         make([]byte, 472034),
		"only non-server roots":             record(nil, second, sequence(sequence(email)), caTestEmailDescription),
		"server lacks ordinary pair":        record(nil, first, serverAux, caTestServerDescription),
		"mismatched pair":                   record(first, second, serverAux, caTestServerDescription),
		"ordinary lacks server trust":       record(first, first, sequence(sequence(email)), caTestEmailDescription),
		"ordinary empty trust":              record(first, first, sequence(sequence()), ""),
		"unsupported purpose":               record(first, first, sequence(sequence(unknown)), ""),
		"duplicate trust":                   record(first, first, sequence(sequence(server, server)), caTestServerDescription),
		"unordered trust":                   record(first, first, sequence(sequence(email, server)), caTestBothDescription),
		"duplicate reject":                  append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(bytes.Join([][]byte{email, email}, nil))), caTestEmailRejection)...),
		"unordered reject":                  append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(bytes.Join([][]byte{email, server}, nil))), caTestBothRejection)...),
		"unsupported reject":                append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(unknown)), "")...),
		"conflicting purposes":              append(bytes.Clone(valid), record(nil, second, sequence(sequence(email), reject(email)), caTestEmailDescription+caTestEmailRejection)...),
		"ordinary has rejected purpose":     record(first, first, sequence(sequence(server), reject(email)), caTestServerDescription+caTestEmailRejection),
		"explicit reject wrapper":           append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(sequence(email))), caTestEmailRejection)...),
		"empty reject":                      append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(nil)), "")...),
		"primitive reject":                  append(bytes.Clone(valid), record(nil, second, sequence(sequence(), caTestASN(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: email})), caTestEmailRejection)...),
		"wrong reject tag":                  append(bytes.Clone(valid), record(nil, second, sequence(sequence(), caTestASN(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: email})), caTestEmailRejection)...),
		"missing mandatory trust":           append(bytes.Clone(valid), record(nil, second, sequence(), "")...),
		"reject instead of mandatory trust": append(bytes.Clone(valid), record(nil, second, sequence(reject(email)), caTestEmailRejection)...),
		"missing AUX":                       record(first, first, nil, caTestServerDescription),
		"extra AUX field":                   record(first, first, sequence(sequence(server), caTestASN(t, "alias")), caTestServerDescription),
		"extra AUX sequence":                record(first, first, sequence(sequence(server), sequence()), caTestServerDescription),
		"field after reject":                append(bytes.Clone(valid), record(nil, second, sequence(sequence(), reject(email), sequence()), caTestEmailRejection)...),
		"trailing DER":                      record(first, first, append(bytes.Clone(serverAux), 5, 0), caTestServerDescription),
		"invalid trusted DER":               record(first, []byte{0x30, 0}, serverAux, caTestServerDescription),
		"invalid ordinary DER":              record([]byte{0x30, 0}, first, serverAux, caTestServerDescription),
		"ordinary trailing DER":             record(append(bytes.Clone(first), 5, 0), first, serverAux, caTestServerDescription),
		"description disagrees":             record(first, first, serverAux, caTestEmailDescription),
		"missing description":               record(first, first, serverAux, ""),
		"description extra text":            record(first, first, serverAux, caTestServerDescription+"extra description\n"),
		"empty label":                       bytes.Replace(valid, []byte("disposable public CA record\n"), []byte("\n"), 1),
		"control label":                     bytes.Replace(valid, []byte("disposable public CA record\n"), []byte("label\tvalue\n"), 1),
		"invalid UTF8 label":                bytes.Replace(valid, []byte("disposable public CA record\n"), []byte{0xff, '\n'}, 1),
		"PEM boundary in label":             bytes.Replace(valid, []byte("disposable public CA record\n"), []byte("label-----value\n"), 1),
		"missing ordinary separator":        bytes.Replace(valid, []byte("-----END CERTIFICATE-----\n\n"), []byte("-----END CERTIFICATE-----\n"), 1),
		"missing final separator":           bytes.TrimSuffix(valid, []byte("\n")),
		"trailing text":                     append(bytes.Clone(valid), []byte("trailing bytes\n")...),
		"trailing certificate":              append(bytes.Clone(valid), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: second})...),
		"unknown PEM type":                  bytes.ReplaceAll(valid, []byte("TRUSTED CERTIFICATE"), []byte("PUBLIC KEY")),
		"missing trusted record":            valid[:bytes.Index(valid, []byte("-----BEGIN TRUSTED CERTIFICATE-----"))],
		"incomplete PEM":                    bytes.Replace(valid, []byte("-----END TRUSTED CERTIFICATE-----"), []byte("-----END TRUSTED CERTIFICATE----"), 1),
		"PEM headers":                       bytes.Replace(valid, []byte("-----BEGIN TRUSTED CERTIFICATE-----\n"), []byte("-----BEGIN TRUSTED CERTIFICATE-----\nComment: unsupported\n\n"), 1),
		"noncanonical PEM newline":          bytes.Replace(valid, []byte("-----BEGIN CERTIFICATE-----\n"), []byte("-----BEGIN CERTIFICATE-----\r\n"), 1),
		"skippable ordinary prefix":         bytes.Replace(valid, []byte("-----BEGIN CERTIFICATE-----\n"), []byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\n"), 1),
		"skippable trusted prefix":          badTrusted,
		"malformed later record":            append(bytes.Clone(valid), badTrusted...),
	} {
		t.Run(name, func(t *testing.T) {
			original := bytes.Clone(input)
			if err := validateCABundle(input); err == nil {
				t.Fatal("incomplete or inconsistent builder record accepted")
			}
			if !bytes.Equal(input, original) {
				t.Fatal("failed validation mutated caller bytes")
			}
		})
	}
}

// Append an ASN.1 NULL inside one constructed value, updating only ancestor lengths.
func caTestAppendDERChild(t testing.TB, der []byte, path ...int) []byte {
	t.Helper()
	var container asn1.RawValue
	rest, err := asn1.Unmarshal(der, &container)
	if err != nil || len(rest) != 0 || !container.IsCompound {
		t.Fatalf("invalid DER mutation container: %v", err)
	}
	if len(path) == 0 {
		container.Bytes = append(bytes.Clone(container.Bytes), 5, 0)
	} else {
		var children [][]byte
		for remaining := container.Bytes; len(remaining) != 0; {
			var child asn1.RawValue
			rest, err := asn1.Unmarshal(remaining, &child)
			if err != nil {
				t.Fatal(err)
			}
			children = append(children, child.FullBytes)
			remaining = rest
		}
		if path[0] < 0 || path[0] >= len(children) {
			t.Fatal("DER mutation child index outside fixture")
		}
		children[path[0]] = caTestAppendDERChild(t, children[path[0]], path[1:]...)
		container.Bytes = bytes.Join(children, nil)
	}
	container.FullBytes = nil
	return caTestASN(t, container)
}

func TestCABundleRejectsUnconsumedCertificateDER(t *testing.T) {
	first, _ := caTestCertificates(t)
	_, reviewedDigest := fixedCATestBundle(t)
	aux := caTestSequence(t, caTestSequence(t, caTestOID(t, 1)))
	valid := caTestRecord(first, first, aux, caTestServerDescription)
	if err := validateCABundle(valid); err != nil {
		t.Fatalf("original complete certificate rejected: %v", err)
	}
	mutations := map[string][]byte{
		"certificate":          caTestAppendDERChild(t, first),
		"TBSCertificate":       caTestAppendDERChild(t, first, 0),
		"signature algorithms": caTestAppendDERChild(t, caTestAppendDERChild(t, first, 0, 2), 1),
		"validity":             caTestAppendDERChild(t, first, 0, 4),
		"SPKI":                 caTestAppendDERChild(t, first, 0, 6),
		"SPKI algorithm":       caTestAppendDERChild(t, first, 0, 6, 0),
		"issuer attribute":     caTestAppendDERChild(t, first, 0, 3, 0, 0),
		"subject attribute":    caTestAppendDERChild(t, first, 0, 5, 0, 0),
		"extensions wrapper":   caTestAppendDERChild(t, first, 0, 7),
		"extension":            caTestAppendDERChild(t, first, 0, 7, 0, 0),
	}
	for name, certificate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := x509.ParseCertificate(certificate); err != nil {
				t.Fatalf("regression no longer reaches the semantic parser's unconsumed DER: %v", err)
			}
			input := caTestRecord(certificate, certificate, aux, caTestServerDescription)
			original := bytes.Clone(input)
			if err := validateCABundle(input); err == nil {
				t.Error("matched ordinary/trusted records accepted an extra certificate DER child")
			}
			if !bytes.Equal(input, original) {
				t.Error("certificate DER rejection changed caller bytes")
			}
			auxOnly := caTestRecord(nil, certificate, caTestSequence(t, caTestSequence(t, caTestOID(t, 4))), caTestEmailDescription)
			if err := validateCABundle(append(bytes.Clone(valid), auxOnly...)); err == nil {
				t.Error("AUX-only trusted record accepted an extra certificate DER child")
			}
			for _, digest := range [][32]byte{reviewedDigest, sha256.Sum256(input)} {
				capability, err := NewFixedCA(input, digest)
				if capability != nil || err == nil || err.Error() != "invalid fixed CA bundle" {
					t.Error("certificate DER substitution bypassed the fixed capability gate")
				}
			}
		})
	}
}

func FuzzValidateCABundle(f *testing.F) {
	bundle, _ := fixedCATestBundle(f)
	first, _ := caTestCertificates(f)
	valid := caTestRecord(first, first, caTestSequence(f, caTestSequence(f, caTestOID(f, 1))), caTestServerDescription)
	f.Add(bundle)
	f.Add(valid)
	f.Add([]byte{})
	f.Add(append(bytes.Clone(valid), []byte("trailing bytes\n")...))
	mutated := caTestAppendDERChild(f, first, 0)
	f.Add(caTestRecord(mutated, mutated, caTestSequence(f, caTestSequence(f, caTestOID(f, 1))), caTestServerDescription))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 472033 {
			t.Skip()
		}
		original := bytes.Clone(input)
		err := validateCABundle(input)
		if !bytes.Equal(input, original) {
			t.Fatal("validation mutated caller bytes")
		}
		if err == nil && validateCABundle(append(bytes.Clone(input), []byte("trailing bytes\n")...)) == nil {
			t.Fatal("complete parser accepted trailing non-record bytes")
		}
	})
}
