//go:build linux

package execenv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func fingerprintFile(t *testing.T, contents []byte) (*os.File, string) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(name, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	closeFingerprintFile(t, file)
	return file, name
}

func closeFingerprintFile(t *testing.T, file *os.File) {
	t.Helper()
	t.Cleanup(func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error("close disposable file:", err)
		}
	})
}

func observeForTest(t *testing.T, file *os.File) executableFingerprint {
	t.Helper()
	got, err := observeExecutable(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestExecutableFingerprintPositionalAndIndependent(t *testing.T) {
	for _, contents := range [][]byte{nil, []byte("small image"), bytes.Repeat([]byte("image"), 20_000)} {
		file, name := fingerprintFile(t, contents)
		if _, err := file.Seek(3, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		// Construction finishes before concurrent use; duplication is CLOEXEC.
		fd, err := unix.FcntlInt(file.Fd(), unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		shared := os.NewFile(uintptr(fd), "shared image")
		closeFingerprintFile(t, shared)
		independent, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		closeFingerprintFile(t, independent)
		want := observeForTest(t, independent)
		if want.sha256 != sha256.Sum256(contents) || want.size != int64(len(contents)) {
			t.Fatal("fingerprint does not describe complete contents")
		}
		var native unix.Stat_t
		if err := unix.Fstat(int(independent.Fd()), &native); err != nil {
			t.Fatal(err)
		}
		if want.device != uint64(native.Dev) || want.inode != native.Ino ||
			want.owner != native.Uid || want.group != native.Gid || want.mode != native.Mode ||
			want.links != uint64(native.Nlink) || want.modified.Sec != native.Mtim.Sec ||
			want.modified.Nsec != native.Mtim.Nsec || want.changed.Sec != native.Ctim.Sec || want.changed.Nsec != native.Ctim.Nsec {
			t.Fatal("fingerprint metadata differs from native descriptor observation")
		}
		for _, opened := range []*os.File{file, shared, independent} {
			if got := observeForTest(t, opened); !sameExecutableFingerprint(got, want) {
				t.Fatal("same inode has different observed identity")
			}
		}
		for _, opened := range []*os.File{file, shared} {
			if offset, err := opened.Seek(0, io.SeekCurrent); err != nil || offset != 3 {
				t.Fatal("observation changed shared offset or closed caller file")
			}
		}
		other, _ := fingerprintFile(t, contents)
		if sameExecutableFingerprint(want, observeForTest(t, other)) {
			t.Fatal("identical bytes at another inode matched")
		}
	}
}

func TestExecutableFingerprintComparisonDimensions(t *testing.T) {
	file, _ := fingerprintFile(t, []byte("image"))
	want := observeForTest(t, file)
	for _, tt := range []struct {
		name string
		edit func(*executableFingerprint)
	}{
		{"device", func(v *executableFingerprint) { v.device++ }},
		{"inode", func(v *executableFingerprint) { v.inode++ }},
		{"owner", func(v *executableFingerprint) { v.owner++ }},
		{"group", func(v *executableFingerprint) { v.group++ }},
		{"mode", func(v *executableFingerprint) { v.mode ^= 0o100 }},
		{"type", func(v *executableFingerprint) { v.mode ^= unix.S_IFDIR }},
		{"size", func(v *executableFingerprint) { v.size++ }},
		{"links", func(v *executableFingerprint) { v.links++ }},
		{"modified", func(v *executableFingerprint) { v.modified.Sec++ }},
		{"changed", func(v *executableFingerprint) { v.changed.Nsec++ }},
		{"digest", func(v *executableFingerprint) { v.sha256[0] ^= 1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := want
			tt.edit(&changed)
			if sameExecutableFingerprint(changed, want) || sameExecutableFingerprint(want, changed) {
				t.Fatal("changed identity matched")
			}
		})
	}
}

func TestExecutableFingerprintComparisonRejectsInvalid(t *testing.T) {
	file, _ := fingerprintFile(t, nil)
	valid := observeForTest(t, file)
	if !sameExecutableFingerprint(valid, valid) {
		t.Fatal("empty regular file must remain a valid observation")
	}
	negative := valid
	negative.size = -1
	nonregular := valid
	nonregular.mode = unix.S_IFDIR | 0o700
	for _, invalid := range []executableFingerprint{{}, negative, nonregular} {
		if sameExecutableFingerprint(invalid, invalid) || sameExecutableFingerprint(invalid, valid) || sameExecutableFingerprint(valid, invalid) {
			t.Fatal("invalid observation compared equal")
		}
	}
}

func TestExecutableFingerprintRejectsInvalidInputs(t *testing.T) {
	file, name := fingerprintFile(t, []byte("private image"))
	writeOnly, err := os.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	closeFingerprintFile(t, writeOnly)
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	closeFingerprintFile(t, directory)
	pipe, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	closeFingerprintFile(t, pipe)
	closeFingerprintFile(t, writer)
	closed, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tt := range []struct {
		name string
		ctx  context.Context
		file *os.File
	}{
		{"nil context", nil, file},
		{"nil file", context.Background(), nil},
		{"closed", context.Background(), closed},
		{"read error", context.Background(), writeOnly},
		{"directory", context.Background(), directory},
		{"pipe", context.Background(), pipe},
		{"canceled", canceled, file},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := observeExecutable(tt.ctx, tt.file)
			if got != (executableFingerprint{}) || err != errExecutableFingerprint || err.Error() != "invalid executable observation" {
				t.Fatal("failure exposed identity or unsanitized error")
			}
		})
	}
	if _, err := file.Stat(); err != nil {
		t.Fatal("failed observation closed caller file")
	}
}

// A real context checkpoint injects changes after initial metadata capture;
// no alternate file reader or production test hook substitutes for the I/O.
type fingerprintCheckpoint struct {
	context.Context
	checks int
	change func()
}

func (c *fingerprintCheckpoint) Err() error {
	c.checks++
	if c.checks == 2 {
		c.change()
	}
	return c.Context.Err()
}

func TestExecutableFingerprintRejectsChangedObservation(t *testing.T) {
	for _, change := range []string{"content", "mode", "metadata", "truncate", "grow", "cancel", "close"} {
		t.Run(change, func(t *testing.T) {
			file, name := fingerprintFile(t, bytes.Repeat([]byte("image"), 10_000))
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &fingerprintCheckpoint{Context: base, change: func() {
				var err error
				switch change {
				case "content":
					err = os.WriteFile(name, bytes.Repeat([]byte("other"), 10_000), 0o600)
					if err == nil {
						err = os.Chtimes(name, time.Unix(10, 0), time.Unix(20, 0))
					}
				case "mode":
					err = os.Chmod(name, 0o700)
				case "metadata":
					err = os.Chtimes(name, time.Unix(10, 0), time.Unix(20, 0))
				case "truncate":
					err = os.Truncate(name, 1)
				case "grow":
					err = os.Truncate(name, 100_000)
				case "cancel":
					cancel()
				case "close":
					err = file.Close()
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			if got, err := observeExecutable(ctx, file); got != (executableFingerprint{}) || err != errExecutableFingerprint {
				t.Fatal("changed observation returned a fingerprint")
			}
		})
	}
}

func TestExecutableFingerprintConcurrentObservationAndClose(t *testing.T) {
	file, _ := fingerprintFile(t, bytes.Repeat([]byte("image"), 10_000))
	want := observeForTest(t, file)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 10 {
				if got, err := observeExecutable(context.Background(), file); err != nil || got != want {
					t.Error("concurrent observation changed identity")
				}
			}
		}()
	}
	workers.Wait()
	start := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		close(start)
		for range 100 {
			got, err := observeExecutable(context.Background(), file)
			if err == nil && got != want || err != nil && (err != errExecutableFingerprint || got != (executableFingerprint{})) {
				t.Error("close race returned unrelated identity or partial result")
			}
		}
	}()
	<-start
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	sentinel, _ := fingerprintFile(t, []byte("unrelated descriptor"))
	if _, err := sentinel.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if offset, err := sentinel.Seek(0, io.SeekCurrent); err != nil || offset != 2 {
		t.Fatal("close race disturbed unrelated descriptor")
	}
}
