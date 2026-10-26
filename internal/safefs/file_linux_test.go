//go:build linux

package safefs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// Actual local descriptors exercise the same kernel core; full admission is separate.
func localFileCapability(t *testing.T) (string, edge, *fileState, []byte) {
	t.Helper()
	root, parent := localFixture(t)
	want := []byte("private disposable record bytes")
	if err := os.WriteFile(filepath.Join(root, "record"), want, 0o600); err != nil {
		t.Fatal(err)
	}
	callerChain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	caller := &Dir{state: &dirState{chain: callerChain, owner: parent.id.Owner, identity: parent.id}}
	t.Cleanup(func() {
		if err := caller.Close(); err != nil {
			t.Error(err)
		}
	})
	chain, err := duplicateChain(caller.state.chain)
	if err != nil {
		t.Fatal(err)
	}
	s, data, err := fileAt(chain, "record", int64(len(want)), parent.id.Owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.closeResources(); err != nil {
			t.Error(err)
		}
	})
	if err := caller.Close(); err != nil {
		t.Fatal(err)
	}
	if s.revalidate() != nil {
		t.Fatal("caller closure invalidated retained file")
	}
	return root, parent, s, data
}

func TestFilePinFactsOwnershipAndLifetime(t *testing.T) {
	_, _, s, data := localFileCapability(t)
	file := &File{state: s}
	facts := file.Facts()
	hash := sha256.Sum256(data)
	if facts.Identity.Mode != unix.S_IFREG|0o600 || facts.Identity.Links != 1 || facts.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("pin facts did not bind exact record")
	}
	data[0] ^= 1
	if s.revalidate() != nil || file.Facts() != facts {
		t.Fatal("returned bytes mutated retained facts")
	}
	fds := []int{s.pin.fd, int(s.reader.Fd()), s.proc.self.fd}
	for _, entry := range s.chain {
		fds = append(fds, entry.fd)
	}
	for _, entry := range s.proc.chain {
		fds = append(fds, entry.fd)
	}
	for _, fd := range fds {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("retained file resource lacks CLOEXEC")
		}
	}
	copyFile := *file
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copyFile.Facts()
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if file.Facts() != (FileFacts{}) || copyFile.Facts() != (FileFacts{}) {
		t.Fatal("closed copy retained facts")
	}
	if copyFile.Revalidate() != errDirectory || copyFile.Remove() != errDirectory {
		t.Fatal("closed copy retained authority")
	}
}

func TestFilePinRefusalCorpusAndCleanup(t *testing.T) {
	root, parent := localFixture(t)
	for _, test := range []struct {
		name string
		mode os.FileMode
	}{{"valid", 0o600}, {"readable", 0o644}, {"executable", 0o700}, {"writable", 0o622}, {"setid", 0o600 | os.ModeSetuid}} {
		if err := os.WriteFile(filepath.Join(root, test.name), []byte("record"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, test.name), test.mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("valid", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hard"), []byte("hard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "hard"), filepath.Join(root, "hard-other")); err != nil {
		t.Fatal(err)
	}
	before := adversarialFDCount(t)
	for range 8 {
		for _, test := range []struct {
			name  string
			limit int64
			owner uint32
		}{
			{"missing", 64, parent.id.Owner}, {"readable", 64, parent.id.Owner}, {"executable", 64, parent.id.Owner}, {"writable", 64, parent.id.Owner}, {"setid", 64, parent.id.Owner},
			{"directory", 64, parent.id.Owner}, {"link", 64, parent.id.Owner}, {"fifo", 64, parent.id.Owner}, {"hard", 64, parent.id.Owner},
			{"valid", 5, parent.id.Owner}, {"valid", -1, parent.id.Owner}, {"valid", math.MaxInt64, parent.id.Owner}, {"valid", 64, parent.id.Owner + 1}, {"../valid", 64, parent.id.Owner},
		} {
			chain, err := duplicateChain([]edge{parent})
			if err != nil {
				t.Fatal(err)
			}
			if s, data, err := fileAt(chain, test.name, test.limit, test.owner); s != nil || data != nil || err != errDirectory {
				t.Fatal("unsafe file admitted or error leaked details")
			}
		}
	}
	if adversarialFDCount(t) != before {
		t.Fatal("failed file admission leaked descriptors")
	}
	for _, dir := range []*Dir{nil, {}, {state: &dirState{closed: true}}} {
		if file, data, err := dir.PinFile("record", 64); file != nil || data != nil || err != errDirectory {
			t.Fatal("invalid Dir admitted File")
		}
	}
	for _, file := range []*File{nil, {}} {
		if file.Facts() != (FileFacts{}) || file.Revalidate() != errDirectory || file.Remove() != errDirectory || file.Close() != nil {
			t.Fatal("zero File retained authority")
		}
	}
}

func TestFileObservedMutationRefuses(t *testing.T) {
	for _, action := range []string{"bytes", "mode", "link", "rename", "parent-mode"} {
		t.Run(action, func(t *testing.T) {
			root, _, s, _ := localFileCapability(t)
			name := filepath.Join(root, "record")
			switch action {
			case "bytes":
				if err := os.WriteFile(name, []byte("private disposable altered bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(name, 0o400); err != nil {
					t.Fatal(err)
				}
			case "link":
				if err := os.Link(name, name+"-link"); err != nil {
					t.Fatal(err)
				}
			case "rename":
				if err := os.Rename(name, name+"-moved"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte("replacement"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "parent-mode":
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal(err)
				}
			}
			if s.revalidate() == nil || s.remove() == nil || s.attempted {
				t.Fatal("observed mismatch permitted unlink")
			}
			if action == "parent-mode" {
				if err := os.Chmod(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestFileUnlinkCoreAndGenericAdmission(t *testing.T) {
	root, parent, s, want := localFileCapability(t)
	before := s.pin.before
	if err := s.remove(); err != nil {
		t.Fatal(err)
	}
	if !s.attempted || s.unlinked == nil || s.unlinked.identity.Links != 0 || s.pin.before != before {
		t.Fatal("one-to-zero transition changed generic pin admission")
	}
	if s.pin.descriptor() == nil {
		t.Fatal("generic file pin admitted zero links")
	}
	if _, err := os.Lstat(filepath.Join(root, "record")); !os.IsNotExist(err) {
		t.Fatal("successful unlink left name")
	}
	data, _, err := s.read(true)
	if err != nil || string(data) != string(want) {
		t.Fatal("retained pre-unlink reader lost original bytes")
	}
	if err := os.WriteFile(filepath.Join(root, "record"), []byte("replacement preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s.remove() == nil {
		t.Fatal("repeated unlink attempted")
	}
	if err := s.closeResources(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "record")); err != nil || string(data) != "replacement preserved" {
		t.Fatal("repeated attempt removed replacement")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("retirement closed caller descriptor")
	}
}

func TestFilePublicAbsoluteRefusalIsTerminal(t *testing.T) {
	root, _, s, _ := localFileCapability(t)
	file := &File{state: s}
	// The local fixture is deliberately not an absolute-root-admitted chain.
	if file.Remove() != errDirectory || !s.closed || s.attempted || file.Facts() != (FileFacts{}) {
		t.Fatal("prevalidation refusal was not terminal and nonmutating")
	}
	if _, err := os.Lstat(filepath.Join(root, "record")); err != nil {
		t.Fatal("prevalidation refusal removed entry")
	}
	if file.Close() != nil || file.Remove() != errDirectory {
		t.Fatal("terminal file resumed")
	}
}

func TestPublicFileAndCreateDirOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive absolute-root topology unavailable under unchanged owner/mode policy")
	}
	parent, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := parent.Close(); err != nil {
			t.Error(err)
		}
	})
	child, err := parent.CreateDir("child")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := child.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := parent.CreateDir("child"); err == nil {
		t.Fatal("exclusive child adopted collision")
	}
	if _, err := child.PublishNoReplace("record", []byte("exact bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, data, err := child.PinFile("record", 64)
	if err != nil || string(data) != "exact bytes" {
		t.Fatal("public exact pin failed")
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Revalidate(); err != nil {
		t.Fatal("caller closure invalidated file")
	}
	copyFile := *file
	results := make(chan error, 16)
	for range 16 {
		go func() { results <- copyFile.Remove() }()
	}
	successes := 0
	for range 16 {
		err := <-results
		if err == nil {
			successes++
		} else if err != errDirectory {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent retirement did not attempt exactly once")
	}
	if copyFile.Facts() != (FileFacts{}) || copyFile.Remove() != errDirectory {
		t.Fatal("retired value copy resumed")
	}
	if _, err := os.Lstat(filepath.Join(root, "child", "record")); !os.IsNotExist(err) {
		t.Fatal("public retirement left record")
	}
}

func TestFilePostUnlinkFailureIsUncertainAndTerminal(t *testing.T) {
	for _, failure := range []string{"changed-unlinked-bytes", "close-error"} {
		t.Run(failure, func(t *testing.T) {
			root, _, s, _ := localFileCapability(t)
			// A pre-existing writer demonstrates that retained pins prevent inode
			// recycling, while exact metadata/content observations still matter.
			writer, err := unix.Open(filepath.Join(root, "record"), unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if unix.Close(writer) != nil {
					t.Error("writer cleanup failed")
				}
			}()
			if err := s.remove(); err != nil {
				t.Fatal(err)
			}
			var primary error
			if failure == "changed-unlinked-bytes" {
				if n, err := unix.Pwrite(writer, []byte("changed"), 0); err != nil || n != 7 {
					t.Fatal("cannot mutate retained unlinked fixture")
				}
				primary = s.revalidate()
				if primary == nil {
					t.Fatal("unlinked content change was accepted")
				}
			} else {
				// Transfer this FD to test cleanup and make the owned close fail,
				// without freeing a descriptor number that the runtime might reuse.
				pinFD := s.pin.fd
				s.pin.fd = -1
				defer func() {
					if unix.Close(pinFD) != nil {
						t.Error("transferred pin cleanup failed")
					}
				}()
			}
			if err := s.finishRemoval(primary); err != errRemovalUncertain {
				t.Fatal("post-attempt failure lost removal uncertainty")
			}
			file := &File{state: s}
			if !s.closed || file.Facts() != (FileFacts{}) || file.Remove() != errDirectory || file.Close() != nil {
				t.Fatal("failed removal was not terminal")
			}
			if err := os.WriteFile(filepath.Join(root, "record"), []byte("replacement preserved"), 0o600); err != nil {
				t.Fatal(err)
			}
			if file.Remove() != errDirectory {
				t.Fatal("terminal capability retried unlink")
			}
			if data, err := os.ReadFile(filepath.Join(root, "record")); err != nil || string(data) != "replacement preserved" {
				t.Fatal("retry displaced replacement")
			}
		})
	}
}

func TestFileReadBoundaries(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
	}{{"empty", 0}, {"one", 1}, {"before-chunk", (32 << 10) - 1}, {"chunk", 32 << 10}, {"after-chunk", (32 << 10) + 1}, {"two-chunks", 64 << 10}} {
		t.Run(test.name, func(t *testing.T) {
			root, parent := localFixture(t)
			want := bytes.Repeat([]byte("x"), test.size)
			if err := os.WriteFile(filepath.Join(root, "record"), want, 0o600); err != nil {
				t.Fatal(err)
			}
			chain, err := duplicateChain([]edge{parent})
			if err != nil {
				t.Fatal(err)
			}
			s, data, err := fileAt(chain, "record", int64(test.size), parent.id.Owner)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := s.closeResources(); err != nil {
					t.Error(err)
				}
			}()
			if data == nil || !bytes.Equal(data, want) || s.revalidate() != nil {
				t.Fatal("exact bounded read or hash failed")
			}
			if test.size > 0 {
				chain, err := duplicateChain([]edge{parent})
				if err != nil {
					t.Fatal(err)
				}
				if got, data, err := fileAt(chain, "record", int64(test.size-1), parent.id.Owner); got != nil || data != nil || err != errDirectory {
					t.Fatal("over-limit file read")
				}
			}
		})
	}
}

func TestFilePublicRevalidationRefusalInvalidates(t *testing.T) {
	root, _, s, _ := localFileCapability(t)
	file := &File{state: s}
	if file.Revalidate() != errDirectory || !s.closed || file.Facts() != (FileFacts{}) {
		t.Fatal("absolute admission failure did not invalidate pin")
	}
	if _, err := os.Lstat(filepath.Join(root, "record")); err != nil {
		t.Fatal("failed revalidation removed entry")
	}
	if file.Close() != nil {
		t.Fatal("invalidated pin did not close idempotently")
	}
}
