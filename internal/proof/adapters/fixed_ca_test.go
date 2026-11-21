package adapters

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// ARCH-009/011/012/030, VAL-017/021: rejected CA authority must not reach DNS.
func TestArchiveAcquisitionRejectsUnreviewedCABeforeDNS(t *testing.T) {
	var lookups atomic.Int32
	prior := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		lookups.Add(1)
		return nil, errors.New("test DNS denied")
	}}
	t.Cleanup(func() { net.DefaultResolver = prior })
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "missing"},
		{name: "empty", data: []byte{}},
		{name: "malformed", data: []byte("invalid PEM\n")},
		{name: "wrong digest", data: testCAPEM(t, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookups.Store(0)
			root := t.TempDir()
			cache := filepath.Join(root, "cache")
			if err := os.Mkdir(cache, 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, file, _, err := acquireArchiveDescriptor(ctx, cache, lockFact{URL: "https://archive.example.test/node.tar.gz", Size: 1}, tc.data)
			if file != nil {
				file.Close()
				t.Fatal("rejected CA returned an archive descriptor")
			}
			if err == nil || !strings.Contains(err.Error(), "fixed CA") {
				t.Errorf("expected fixed CA rejection")
			}
			if lookups.Load() != 0 {
				t.Errorf("rejected CA reached DNS %d times", lookups.Load())
			}
			entries, readErr := os.ReadDir(cache)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("rejected CA changed acquisition cache")
			}
		})
	}
}

func TestRunRequiresFixedCABeforeRootAccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "missing"},
		{name: "empty", data: []byte{}},
		{name: "malformed", data: []byte("invalid PEM\n")},
		{name: "wrong digest", data: testCAPEM(t, 2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ca := ""
			if tc.data != nil {
				ca = filepath.Join(t.TempDir(), "ca.pem")
				if err := os.WriteFile(ca, tc.data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Run(context.Background(), Config{CACert: ca})
			if err == nil || !strings.Contains(err.Error(), "fixed CA") {
				t.Fatal("Run did not reject fixed CA before root validation")
			}
		})
	}
}

func TestCARejectionPrecedesArchiveURLParsing(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "absent-cache")
	_, file, _, err := acquireArchiveDescriptor(context.Background(), cache, lockFact{URL: "%"}, nil)
	if file != nil || err == nil || !strings.Contains(err.Error(), "fixed CA") {
		t.Fatal("archive URL parsing preceded CA rejection")
	}
	if _, err := os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected CA touched the absent cache")
	}
}

func TestNamespaceRejectsMissingCABeforeSetupOrLockParsing(t *testing.T) {
	_, err := runMiseNamespaceHarness(context.Background(), miseNamespaceRequest{Config: Config{NodeLock: "invalid"}})
	if err == nil || !strings.Contains(err.Error(), "fixed CA") {
		t.Fatal("namespace setup or authored lock parsing preceded CA rejection")
	}
}

func TestCAPoolConsumesCompleteCanonicalPEM(t *testing.T) {
	valid := testCAPEM(t, 3)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "empty"},
		{name: "prefix junk", data: append([]byte("junk\n"), valid...)},
		{name: "suffix junk", data: append(append([]byte(nil), valid...), []byte("junk\n")...)},
		{name: "invalid later certificate", data: append(append([]byte(nil), valid...), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1}})...)},
		{name: "other PEM type", data: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1}})},
		{name: "PEM headers", data: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"Comment": "invalid"}, Bytes: []byte{1}})},
		{name: "skipped malformed block", data: append([]byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"), valid...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := parseCAPool(tc.data)
			if err == nil || pool != nil {
				t.Fatal("incomplete or non-canonical bundle accepted")
			}
		})
	}
}

func TestCAPoolIsExclusiveFreshAndIndependentOfInput(t *testing.T) {
	bundle := testCAPEM(t, 4)
	block, _ := pem.Decode(bundle)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	other := testCAPEM(t, 5)
	otherBlock, _ := pem.Decode(other)
	otherCertificate, err := x509.ParseCertificate(otherBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	// These ambient paths must not add an otherwise valid root.
	ambient := filepath.Join(t.TempDir(), "ambient-ca.pem")
	if err := os.WriteFile(ambient, other, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", ambient)
	t.Setenv("SSL_CERT_DIR", filepath.Dir(ambient))
	pool, err := parseCAPool(bundle)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parseCAPool(bundle)
	if err != nil {
		t.Fatal(err)
	}
	second.AddCert(otherCertificate)
	for i := range bundle {
		bundle[i] = 0
	}
	options := x509.VerifyOptions{Roots: pool, CurrentTime: time.Unix(1700000000, 0)}
	if _, err := certificate.Verify(options); err != nil {
		t.Fatal("pool depended on caller's mutated bundle")
	}
	if _, err := otherCertificate.Verify(options); err == nil {
		t.Fatal("ambient or separately modified pool added a root")
	}
	third := testCAPEM(t, 6)
	combined, err := parseCAPool(append(third, other...))
	if err != nil {
		t.Fatal("complete multi-certificate bundle was not parsed")
	}
	thirdBlock, _ := pem.Decode(third)
	thirdCertificate, err := x509.ParseCertificate(thirdBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []*x509.Certificate{thirdCertificate, otherCertificate} {
		if _, err := root.Verify(x509.VerifyOptions{Roots: combined, CurrentTime: time.Unix(1700000000, 0)}); err != nil {
			t.Fatal("complete bundle did not trust each of its roots")
		}
	}
}

func TestFixedCAFileBoundary(t *testing.T) {
	root := t.TempDir()
	validPath := filepath.Join(root, "unreviewed.pem")
	if err := os.WriteFile(validPath, testCAPEM(t, 7), 0600); err != nil {
		t.Fatal(err)
	}
	// This reaches digest authentication, proving the path checks accepted the fixture.
	if _, err := readFixedCA(validPath); err == nil || !strings.Contains(err.Error(), "reviewed digest") {
		t.Fatal("canonical owned fixture did not reach digest authentication")
	}
	for _, tc := range []struct {
		name string
		make func(string) error
	}{
		{name: "symlink", make: func(p string) error { return os.Symlink(validPath, p) }},
		{name: "directory", make: func(p string) error { return os.Mkdir(p, 0700) }},
		{name: "fifo", make: func(p string) error { return syscall.Mkfifo(p, 0600) }},
		{name: "hardlink", make: func(p string) error { return os.Link(validPath, p) }},
		{name: "writable", make: func(p string) error {
			if err := os.WriteFile(p, []byte("invalid"), 0600); err != nil {
				return err
			}
			return os.Chmod(p, 0666)
		}},
		{name: "oversized", make: func(p string) error { return os.WriteFile(p, make([]byte, maxFixedCABytes+1), 0600) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := filepath.Join(root, tc.name)
			if err := tc.make(name); err != nil {
				t.Fatal(err)
			}
			if bundle, err := readFixedCA(name); err == nil || bundle != nil {
				t.Fatal("unsafe CA file accepted")
			}
		})
	}
	for _, name := range []string{"relative.pem", root + "/../" + filepath.Base(root) + "/unreviewed.pem"} {
		if _, err := readFixedCA(name); err == nil {
			t.Fatal("non-canonical CA path accepted")
		}
	}
	linkDir := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, linkDir); err != nil {
		t.Fatal(err)
	}
	if _, err := readFixedCA(filepath.Join(linkDir, "unreviewed.pem")); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	writableDir := filepath.Join(t.TempDir(), "writable")
	if err := os.Mkdir(writableDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(writableDir, "ca.pem"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writableDir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := readFixedCA(filepath.Join(writableDir, "ca.pem")); err == nil || !strings.Contains(err.Error(), "ancestor") {
		t.Fatal("writable CA ancestor accepted")
	}
}

func testCAPEM(t *testing.T, serial int64) []byte {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "disposable test CA"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, public, private)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
