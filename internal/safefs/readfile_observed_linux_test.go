//go:build linux

package safefs

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestObservedReadPassesAndShortReads(t *testing.T) {
	for _, collect := range []bool{false, true} {
		for _, size := range []int{0, 1, (32 << 10) - 1, 32 << 10, (32 << 10) + 1} {
			content := bytes.Repeat([]byte("x"), size)
			var charged uint64
			eof, checks := 0, 0
			data, digest, err := readObservedContents(int64(size), collect, func(buffer []byte, offset int64) (int, error) {
				if offset == int64(size) {
					eof++
					return 0, nil
				}
				return copy(buffer, content[int(offset):]), nil
			}, func() error { checks++; return nil }, func(n uint64) error { charged += n; return nil })
			passes := 1
			if collect {
				passes = 2
			}
			if err != nil || digest != sha256.Sum256(content) || eof != passes || charged != uint64(passes*(size+1)) || checks == 0 {
				t.Fatal("read passes, hashes or EOF charges differ", collect, size, eof, charged, err)
			}
			if collect && (data == nil || !bytes.Equal(data, content) || len(data) != cap(data)) || !collect && data != nil {
				t.Fatal("unexpected collected storage", collect, size)
			}
		}
	}
	content := []byte("short")
	var charged uint64
	reads := 0
	data, digest, err := readObservedContents(int64(len(content)), true, func(buffer []byte, offset int64) (int, error) {
		reads++
		if offset == int64(len(content)) {
			return 0, nil
		}
		buffer[0] = content[offset]
		return 1, nil
	}, nil, func(n uint64) error { charged += n; return nil })
	if err != nil || !bytes.Equal(data, content) || cap(data) != len(data) || digest != sha256.Sum256(content) || reads != 12 || charged != 32 {
		t.Fatal("short positive reads or requested-byte charges differ", reads, charged, err)
	}
}

func TestObservedReadClearsEveryPassFailure(t *testing.T) {
	for _, failure := range []string{"first-read", "second-read", "negative-read", "oversized-read", "early-eof", "first-growth", "second-growth", "changed-bytes", "measure-completion", "second-validation", "first-eof-charge", "second-read-charge", "second-eof-charge"} {
		t.Run(failure, func(t *testing.T) {
			content := []byte("abc")
			reads, eof, checks := 0, 0, 0
			data, digest, err := readObservedContents(3, true, func(buffer []byte, offset int64) (int, error) {
				reads++
				if failure == "first-read" || failure == "second-read" && eof == 1 {
					buffer[0] = 'x'
					return 1, errors.New("private I/O marker")
				}
				if failure == "negative-read" {
					return -1, nil
				}
				if failure == "oversized-read" {
					return len(buffer) + 1, nil
				}
				if failure == "early-eof" {
					return 0, nil
				}
				if offset == 3 {
					eof++
					if failure == "first-growth" || failure == "second-growth" && eof == 2 {
						buffer[0] = 'x'
						return 1, nil
					}
					return 0, nil
				}
				n := copy(buffer, content)
				if failure == "changed-bytes" && eof == 1 {
					buffer[0] = 'x'
				}
				return n, nil
			}, func() error {
				checks++
				if failure == "measure-completion" && checks == 5 || failure == "second-validation" && eof == 1 && reads == 3 {
					return errors.New("private observation marker")
				}
				return nil
			}, func(n uint64) error {
				if failure == "first-eof-charge" && reads == 1 || failure == "second-read-charge" && eof == 1 || failure == "second-eof-charge" && eof == 1 && n == 1 {
					return errDirectory
				}
				return nil
			})
			if data != nil || digest != ([32]byte{}) || err != errDirectory {
				t.Fatal("failed read retained bytes, hash or private error", failure, err)
			}
			if failure == "measure-completion" && reads != 2 {
				t.Fatal("measurement completion failure reached reread")
			}
		})
	}
	for _, size := range []int64{-1, int64(int(^uint(0) >> 1))} {
		data, digest, err := readObservedContents(size, true, func([]byte, int64) (int, error) { t.Fatal("invalid size reached I/O"); return 0, nil }, nil, nil)
		if data != nil || digest != ([32]byte{}) || err != errDirectory {
			t.Fatal("invalid size retained a result")
		}
	}
}

func TestTreeObservedReadPassesAndLifetimeCharges(t *testing.T) {
	base, s, _, _ := localTreeCore(t)
	content := []byte("bounded observed tree read")
	if err := os.WriteFile(filepath.Join(base, "stage", "record"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, collect := range []bool{true, false} {
		s.reads = 0
		data, stamp, digest, err := treeRead(s, s.root, "record", int64(len(content)), collect, false)
		passes := uint64(1)
		if collect {
			passes = 2
		}
		if err != nil || stamp.identity.Size != int64(len(content)) || digest != sha256.Sum256(content) || s.reads != passes*uint64(len(content)+1) {
			t.Fatal("tree kernel read passes or charges differ", collect, s.reads, err)
		}
		if collect && (!bytes.Equal(data, content) || len(data) != cap(data)) || !collect && data != nil {
			t.Fatal("unexpected tree result storage")
		}
	}
	for _, remaining := range []uint64{uint64(len(content) + 1), uint64(2*len(content) + 1)} {
		s.reads = treeMaxReads - remaining
		data, stamp, digest, err := treeRead(s, s.root, "record", int64(len(content)), true, false)
		if data != nil || stamp != (fileStamp{}) || digest != ([32]byte{}) || err != errDirectory {
			t.Fatal("second pass or EOF charge refusal retained tree results", remaining, err)
		}
	}
}

func TestObservedPinnedReadPreservesDescriptorOffset(t *testing.T) {
	_, _, state, want := localFileCapability(t)
	const offset = int64(7)
	if _, err := state.reader.Seek(offset, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	for _, collect := range []bool{true, false} {
		got, _, err := state.read(collect)
		if err != nil || collect && (!bytes.Equal(got, want) || cap(got) != len(got)) || !collect && got != nil {
			t.Fatal("retained positional read failed", err)
		}
		position, err := state.reader.Seek(0, io.SeekCurrent)
		if err != nil || position != offset {
			t.Fatal("retained descriptor offset changed", position, err)
		}
	}
	if err := state.revalidate(); err != nil {
		t.Fatal(err)
	}
	position, err := state.reader.Seek(0, io.SeekCurrent)
	if err != nil || position != offset {
		t.Fatal("revalidation changed retained offset", position, err)
	}
}
