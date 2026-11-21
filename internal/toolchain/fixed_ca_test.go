package toolchain

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixedCATestBundle(t testing.TB) ([]byte, [32]byte) {
	t.Helper()
	bundle, err := os.ReadFile("../proof/adapters/testdata/reviewed-ca-bundle.pem")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := hex.DecodeString("3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a")
	if err != nil {
		t.Fatal(err)
	}
	var digest [32]byte
	copy(digest[:], encoded)
	if len(bundle) != 472033 || sha256.Sum256(bundle) != digest {
		t.Fatal("reviewed public fixture identity changed")
	}
	return bundle, digest
}

// ARCH-030: only the reviewed bytes create a capability, and no caller owns its storage.
func TestFixedCAIdentityAndCopies(t *testing.T) {
	bundle, digest := fixedCATestBundle(t)
	original := bytes.Clone(bundle)
	first, err := NewFixedCA(bundle, digest)
	if err != nil || first == nil {
		t.Fatalf("reviewed bundle rejected: %v", err)
	}
	second, err := NewFixedCA(bundle, digest)
	if err != nil || second == nil || first == second {
		t.Fatalf("independent construction failed: %v", err)
	}
	clear(bundle)
	returned := first.Bundle()
	if !bytes.Equal(returned, original) || first.SHA256() != digest {
		t.Fatal("caller input mutation changed capability identity")
	}
	clear(returned)
	if !bytes.Equal(first.Bundle(), original) || !bytes.Equal(second.Bundle(), original) || second.SHA256() != digest {
		t.Fatal("accessor mutation changed a capability")
	}
	gotDigest := first.SHA256()
	clear(gotDigest[:])
	if first.SHA256() != digest {
		t.Fatal("digest accessor exposed mutable storage")
	}
}

func TestFixedCAConcurrentAccessors(t *testing.T) {
	bundle, digest := fixedCATestBundle(t)
	capability, err := NewFixedCA(bundle, digest)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 8 {
				copy := capability.Bundle()
				if !bytes.Equal(copy, bundle) || capability.SHA256() != digest {
					t.Error("concurrent access changed capability identity")
					return
				}
				clear(copy)
			}
		}()
	}
	workers.Wait()
	if !bytes.Equal(capability.Bundle(), bundle) {
		t.Fatal("concurrent accessor mutation reached capability")
	}
}

func TestFixedCANilAndZero(t *testing.T) {
	for _, capability := range []*FixedCA{nil, new(FixedCA)} {
		if capability.Bundle() != nil || capability.SHA256() != ([32]byte{}) {
			t.Fatal("uninitialized capability exposed authority")
		}
	}
}

func TestFixedCARejectsSubstitution(t *testing.T) {
	bundle, digest := fixedCATestBundle(t)
	mutated := bytes.Clone(bundle)
	mutated[0] ^= 1 // A label change preserves a valid builder record and the exact length.
	if err := validateCABundle(mutated); err != nil {
		t.Fatalf("alternate label must remain valid parser input: %v", err)
	}
	first, _ := caTestCertificates(t)
	alternate := caTestRecord(first, first, caTestSequence(t, caTestSequence(t, caTestOID(t, 1))), caTestServerDescription)
	if err := validateCABundle(alternate); err != nil {
		t.Fatalf("alternate record must remain valid parser input: %v", err)
	}
	wrongDigest := digest
	wrongDigest[0] ^= 1
	cases := []struct {
		name   string
		bundle []byte
		digest [32]byte
	}{
		{"nil", nil, digest},
		{"empty", []byte{}, digest},
		{"truncated", bundle[:len(bundle)-1], digest},
		{"oversized", append(bytes.Clone(bundle), '\n'), digest},
		{"wrong digest", bundle, wrongDigest},
		{"zero digest", bundle, [32]byte{}},
		{"same length changed bytes", mutated, digest},
		{"self hashed same length alternate", mutated, sha256.Sum256(mutated)},
		{"self hashed alternate record", alternate, sha256.Sum256(alternate)},
		{"self hashed malformed", []byte("untrusted CA input"), sha256.Sum256([]byte("untrusted CA input"))},
	}
	var fixedError string
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			capability, err := NewFixedCA(test.bundle, test.digest)
			if capability != nil || err == nil {
				t.Fatal("substituted CA input created authority")
			}
			if fixedError == "" {
				fixedError = err.Error()
			}
			if err.Error() != fixedError || bytes.Contains([]byte(err.Error()), []byte("untrusted CA input")) || bytes.Contains([]byte(err.Error()), []byte("3ba2000fe3d55cb6")) {
				t.Fatal("constructor error is not fixed and redacted")
			}
		})
	}
}

// VAL-017/021/022: offline pool membership is checked for every authenticated anchor.
func TestFixedCAOrdinaryPoolExcludesAUXAndAmbientRoots(t *testing.T) {
	bundle, digest := fixedCATestBundle(t)
	ordinary := make(map[string]*x509.Certificate)
	var trusted []*x509.Certificate
	for remaining := bundle; ; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		var certificate *x509.Certificate
		var err error
		switch block.Type {
		case "CERTIFICATE":
			certificate, err = x509.ParseCertificate(block.Bytes)
			if err == nil {
				ordinary[string(certificate.Raw)] = certificate
			}
		case "TRUSTED CERTIFICATE":
			var der asn1.RawValue
			if _, err = asn1.Unmarshal(block.Bytes, &der); err == nil {
				certificate, err = x509.ParseCertificate(der.FullBytes)
			}
			trusted = append(trusted, certificate)
		default:
			t.Fatal("unexpected authenticated fixture block")
		}
		if err != nil {
			t.Fatal(err)
		}
		remaining = rest
	}
	if len(ordinary) != 121 || len(trusted) != 172 {
		t.Fatalf("ordinary/trusted inventory is %d/%d; want 121/172", len(ordinary), len(trusted))
	}
	var ambient *x509.Certificate
	for _, certificate := range trusted {
		if ordinary[string(certificate.Raw)] == nil {
			ambient = certificate
			break
		}
	}
	if ambient == nil {
		t.Fatal("fixture lacks a non-server certificate for ambient poisoning")
	}
	ambientDir := t.TempDir()
	ambientFile := filepath.Join(ambientDir, "ambient-ca.pem")
	if err := os.WriteFile(ambientFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ambient.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", ambientFile)
	t.Setenv("SSL_CERT_DIR", ambientDir)
	t.Setenv("CURL_CA_BUNDLE", ambientFile)
	t.Setenv("REQUESTS_CA_BUNDLE", ambientFile)
	t.Setenv("GIT_SSL_CAINFO", ambientFile)
	t.Setenv("GIT_SSL_CAPATH", ambientDir)
	capability, err := NewFixedCA(bundle, digest)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(capability.Bundle()) {
		t.Fatal("capability lacks ordinary pool certificates")
	}
	want := x509.NewCertPool()
	for _, certificate := range ordinary {
		want.AddCert(certificate)
	}
	if !roots.Equal(want) {
		t.Fatal("ordinary PEM pool differs from the independent 121-root inventory")
	}
	clear(bundle)
	clear(capability.Bundle())
	accepted, excluded := 0, 0
	for _, certificate := range trusted {
		_, err := certificate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: certificate.NotBefore.Add(time.Second), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
		if ordinary[string(certificate.Raw)] != nil {
			accepted++
			if err != nil {
				t.Fatalf("ordinary anchor rejected: %v", err)
			}
		} else {
			excluded++
			if err == nil {
				t.Fatal("AUX-only or ambient anchor entered ordinary pool")
			}
		}
	}
	if accepted != 121 || excluded != 51 {
		t.Fatalf("anchor membership is %d included/%d excluded; want 121/51", accepted, excluded)
	}
	independent := x509.NewCertPool()
	if !independent.AppendCertsFromPEM(capability.Bundle()) {
		t.Fatal("independent pool construction failed")
	}
	independent.AddCert(ambient)
	if roots.Equal(independent) || !roots.Equal(want) {
		t.Fatal("separate consumer pool mutation changed fixed roots")
	}
}
