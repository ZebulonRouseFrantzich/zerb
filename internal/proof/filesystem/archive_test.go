package filesystem

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

// VAL-028: decoded aliases/metadata cannot widen the raw USTAR grammar.
func TestArchiveRawGrammar(t *testing.T) {
	for _, tc := range []struct {
		name      string
		header    tar.Header
		legacy    bool
		rejection string
	}{
		{name: "regular", header: tar.Header{Name: "x", Typeflag: tar.TypeReg, Mode: 0600, Format: tar.FormatUSTAR}},
		{name: "legacy-regular-alias", header: tar.Header{Name: "x", Typeflag: tar.TypeReg, Mode: 0600, Format: tar.FormatUSTAR}, legacy: true, rejection: "link-device-socket-or-unsupported-type"},
		{name: "symlink", header: tar.Header{Name: "x", Typeflag: tar.TypeSymlink, Linkname: "y", Mode: 0600, Format: tar.FormatUSTAR}, rejection: "link-device-socket-or-unsupported-type"},
		{name: "xattr", header: tar.Header{Name: "x", Typeflag: tar.TypeReg, Mode: 0600, Format: tar.FormatPAX, PAXRecords: map[string]string{"SCHILY.xattr.user.test": "value"}}, rejection: "unsupported-format-or-metadata"},
		{name: "unsupported-mode", header: tar.Header{Name: "x", Typeflag: tar.TypeReg, Mode: 0644, Format: tar.FormatUSTAR}, rejection: "unsupported-mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := tar.NewWriter(&buf)
			if err := writer.WriteHeader(&tc.header); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			blob := buf.Bytes()
			if tc.legacy {
				blob[156] = '\x00'
				for i := 148; i < 156; i++ {
					blob[i] = ' '
				}
				var sum int
				for _, value := range blob[:512] {
					sum += int(value)
				}
				copy(blob[148:156], fmt.Sprintf("%06o\x00 ", sum))
			}
			digest := sha256.Sum256(blob)
			lim := ArchiveLimits{EntryBytes: 1024, TotalBytes: 4096, Count: 8, Depth: 4}
			got := ValidateTarIntegrity(blob, hex.EncodeToString(digest[:]), lim)
			if got.Rejection != tc.rejection || got.Prevalidated != (tc.rejection == "") || !got.IntegrityVerified {
				t.Fatalf("archive admission = %+v; want rejection %q", got, tc.rejection)
			}
			badDigest := sha256.Sum256([]byte("different bytes"))
			got = ValidateTarIntegrity(blob, hex.EncodeToString(badDigest[:]), lim)
			if got.Rejection != "integrity-before-parse" || got.Parsed {
				t.Fatalf("integrity precedence = %+v", got)
			}
		})
	}
}
