//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// ARCH-020/030, VAL-027: private fixture checks do not qualify absolute roots.
func TestReadFileAdversarialTypesBeforeContent(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "socket", "directory", "hardlink", "group-write", "world-write", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			name := filepath.Join(root, "entry")
			switch kind {
			case "symlink":
				if err := os.Symlink("missing", name); err != nil {
					t.Fatal("cannot create disposable symlink")
				}
			case "fifo":
				// No reader or writer opens the FIFO; rejection must use metadata only.
				if err := unix.Mkfifo(name, 0o600); err != nil {
					t.Fatal("cannot create disposable FIFO")
				}
			case "socket":
				fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
				if err != nil {
					t.Fatal("cannot create disposable socket")
				}
				defer func() {
					if err := unix.Close(fd); err != nil {
						t.Error("socket close failed")
					}
				}()
				// The owned FD spelling keeps the Unix socket name below sun_path's limit.
				socketName := "/proc/self/fd/" + strconv.Itoa(parent.fd) + "/entry"
				if err := unix.Bind(fd, &unix.SockaddrUnix{Name: socketName}); err != nil {
					t.Fatal("cannot bind disposable socket")
				}
			case "directory":
				if err := os.Mkdir(name, 0o700); err != nil {
					t.Fatal("cannot create disposable directory")
				}
			default:
				if err := os.WriteFile(name, []byte("fixture"), 0o600); err != nil {
					t.Fatal("cannot create disposable file")
				}
				switch kind {
				case "hardlink":
					if err := os.Link(name, filepath.Join(root, "alias")); err != nil {
						t.Fatal("cannot create disposable hardlink")
					}
				case "group-write", "world-write":
					mode := os.FileMode(0o620)
					if kind == "world-write" {
						mode = 0o602
					}
					if err := os.Chmod(name, mode); err != nil {
						t.Fatal("cannot set disposable mode")
					}
				}
			}
			limit := int64(32)
			if kind == "oversize" {
				limit = 1
			}
			pin, err := pinFile(parent, "entry", limit)
			if pin != nil {
				if closeErr := pin.close(); closeErr != nil {
					t.Error("unexpected pin cleanup failed")
				}
				t.Fatal("unsafe object acquired content authority")
			}
			if err == nil || err.Error() != errDirectory.Error() {
				t.Fatal("unsafe object did not receive fixed refusal")
			}
			data, id, err := readFile(parent, "entry", limit)
			assertAdversarialRefusal(t, data, id, err)
		})
	}
}

func TestReadFileAdversarialMetadata(t *testing.T) {
	good := Identity{Mode: unix.S_IFREG | 0o600, Owner: 1000, Links: 1, MountID: 7, Size: 3}
	if !safeFile(good, 1000, 3) {
		t.Fatal("safe ordinary file rejected")
	}
	for _, edit := range []func(*Identity){
		func(id *Identity) { id.Owner++ },
		func(id *Identity) { id.Links = 0 },
		func(id *Identity) { id.Links = 2 },
		func(id *Identity) { id.Size = -1 },
		func(id *Identity) { id.Size = 4 },
		func(id *Identity) { id.MountID = 0 },
		func(id *Identity) { id.Mode = unix.S_IFCHR | 0o600 },
		func(id *Identity) { id.Mode |= 0o020 },
		func(id *Identity) { id.Mode |= 0o002 },
	} {
		bad := good
		edit(&bad)
		if safeFile(bad, 1000, 3) {
			t.Fatal("unsafe synthetic metadata accepted")
		}
	}
	// Device metadata is obtained with O_PATH, never with a driver-readable open.
	fd, err := unix.Open("/dev/null", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skip("optional metadata-only device fixture unavailable")
	}
	defer func() {
		if err := unix.Close(fd); err != nil {
			t.Error("device metadata descriptor cleanup failed")
		}
	}()
	id, err := observeFD(fd)
	if err != nil || id.Mode&unix.S_IFMT != unix.S_IFCHR || safeFile(id, id.Owner, 32) {
		t.Fatal("device metadata was not refused")
	}
}

func TestReadFileAdversarialFakeProcOwnership(t *testing.T) {
	for _, kind := range []string{"missing", "wrong-filesystem", "symlink-proc", "fake-self"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			if kind != "missing" {
				if err := os.Mkdir(filepath.Join(root, "proc"), 0o700); err != nil {
					t.Fatal("cannot create fake proc fixture")
				}
			}
			if kind == "fake-self" {
				if err := os.Symlink(strconv.Itoa(unix.Getpid()), filepath.Join(root, "proc", "self")); err != nil {
					t.Fatal("cannot create fake self link")
				}
			}
			if kind == "symlink-proc" {
				if err := os.Rename(filepath.Join(root, "proc"), filepath.Join(root, "fake")); err != nil {
					t.Fatal("cannot prepare fake proc link")
				}
				if err := os.Symlink("fake", filepath.Join(root, "proc")); err != nil {
					t.Fatal("cannot create fake proc link")
				}
			}
			for range 8 {
				fd, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, 0)
				if err != nil {
					t.Fatal("cannot duplicate owned fake root")
				}
				p, err := openProcRoot(fd)
				if p != nil || err == nil || err.Error() != errDirectory.Error() {
					if p != nil {
						if closeErr := p.close(); closeErr != nil {
							t.Error("unexpected proc cleanup failed")
						}
					}
					t.Fatal("fake proc authority accepted")
				}
				if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
					t.Fatal("failed proc acquisition retained owned root descriptor")
				}
				if _, err := observeFD(parent.fd); err != nil {
					t.Fatal("borrowed fixture descriptor was closed")
				}
			}
		})
	}
}

func TestReadFileAdversarialProcCapability(t *testing.T) {
	root, parent := localFixture(t)
	writeAdversarialFile(t, root, "entry", []byte("original"))
	pin := adversarialPin(t, parent, "entry", 32)
	p := adversarialProc(t)
	for _, mutate := range []func() func(){
		func() func() { old := p.pid; p.pid = "0"; return func() { p.pid = old } },
		func() func() { p.closed = true; return func() { p.closed = false } },
		func() func() { old := p.self.id; p.self.id.Inode++; return func() { p.self.id = old } },
		func() func() { old := p.chain[2].id; p.chain[2].id.MountID++; return func() { p.chain[2].id = old } },
		func() func() { old := p.chain[3].id; p.chain[3].id.Inode++; return func() { p.chain[3].id = old } },
		func() func() {
			old := p.chain[3].name
			p.chain[3].name = "outside"
			return func() { p.chain[3].name = old }
		},
	} {
		restore := mutate()
		err := p.validate()
		reader, reopenErr := p.reopen(pin)
		restore() // Restore ownership before any failure can trigger cleanup.
		if reader != nil {
			if err := reader.Close(); err != nil {
				t.Error("unexpected reader close failed")
			}
		}
		if err == nil || reopenErr == nil || reader != nil {
			t.Fatal("altered proc capability retained authority")
		}
		if err := p.validate(); err != nil {
			t.Fatal("restored proc capability failed validation")
		}
	}
	if err := p.close(); err != nil {
		t.Fatal("proc close failed")
	}
	if err := p.validate(); err == nil {
		t.Fatal("closed proc capability accepted")
	}
	if reader, err := p.reopen(pin); err == nil || reader != nil {
		t.Fatal("closed proc capability reopened content")
	}
}

func TestReadFileAdversarialPinReplacement(t *testing.T) {
	root, parent := localFixture(t)
	writeAdversarialFile(t, root, "entry", []byte("original"))
	pin := adversarialPin(t, parent, "entry", 32)
	p := adversarialProc(t)
	reader, err := p.reopen(pin)
	if err != nil {
		t.Fatal("validated proc did not reopen pinned ordinary file")
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error("reader close failed")
		}
	}()
	if err := os.Rename(filepath.Join(root, "entry"), filepath.Join(root, "moved")); err != nil {
		t.Fatal("cannot rename pinned fixture")
	}
	writeAdversarialFile(t, root, "entry", []byte("replacement"))
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "original" {
		t.Fatal("reader followed replacement pathname")
	}
	if pin.validate() == nil {
		t.Fatal("replaced original name edge accepted")
	}
	// Rename changes ctime on Linux: a later reopen must reject that changed stamp.
	if reader, err := p.reopen(pin); err == nil || reader != nil {
		if reader != nil {
			if err := reader.Close(); err != nil {
				t.Error("unexpected reader close failed")
			}
		}
		t.Fatal("changed pinned stamp reopened after replacement")
	}
}

func TestReadFileAdversarialPinnedMetadataMutation(t *testing.T) {
	for _, mutation := range []string{"truncate", "grow", "safe-mode-change", "add-hardlink"} {
		t.Run(mutation, func(t *testing.T) {
			root, parent := localFixture(t)
			writeAdversarialFile(t, root, "entry", []byte("original"))
			pin := adversarialPin(t, parent, "entry", 32)
			p := adversarialProc(t)
			name := filepath.Join(root, "entry")
			var err error
			switch mutation {
			case "truncate":
				err = os.Truncate(name, 1)
			case "grow":
				err = os.Truncate(name, 10)
			case "safe-mode-change":
				err = os.Chmod(name, 0o400)
			case "add-hardlink":
				err = os.Link(name, filepath.Join(root, "alias"))
			}
			if err != nil {
				t.Fatal("cannot mutate disposable pinned file")
			}
			if pin.descriptor() == nil || pin.validate() == nil {
				t.Fatal("changed pinned inode retained authority")
			}
			reader, err := p.reopen(pin)
			if reader != nil {
				if closeErr := reader.Close(); closeErr != nil {
					t.Error("unexpected reader close failed")
				}
			}
			if reader != nil || err == nil || err.Error() != errDirectory.Error() {
				t.Fatal("changed pinned inode acquired content authority")
			}
		})
	}
}

func TestReadFileAdversarialDescriptorLifetimes(t *testing.T) {
	root, parent := localFixture(t)
	want := []byte("ordinary content")
	writeAdversarialFile(t, root, "entry", want)
	for _, minimum := range []int{0, 128} {
		pin := adversarialPin(t, parent, "entry", 32)
		fd, err := unix.FcntlInt(uintptr(pin.fd), unix.F_DUPFD_CLOEXEC, minimum)
		if err != nil {
			t.Fatal("pin duplication failed")
		}
		if err := unix.Close(pin.fd); err != nil {
			t.Fatal("old pin close failed")
		}
		pin.fd = fd
		p := adversarialProc(t)
		reader, err := p.reopen(pin)
		if err != nil {
			t.Fatal("reopen failed after FD-number churn")
		}
		for _, fd := range []int{pin.fd, int(reader.Fd()), p.self.fd, p.chain[0].fd, p.chain[1].fd, p.chain[2].fd, p.chain[3].fd} {
			flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
			if err != nil || flags&unix.FD_CLOEXEC == 0 {
				t.Fatal("retained descriptor lacks close-on-exec")
			}
		}
		if err := pin.close(); err != nil {
			t.Fatal("pin close failed")
		}
		if err := p.close(); err != nil {
			t.Fatal("proc close failed")
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(data, want) {
			t.Fatal("reader lost independent descriptor lifetime")
		}
		if pin.validate() == nil {
			t.Fatal("closed pin retained authority")
		}
	}
	pin := adversarialPin(t, parent, "entry", 32)
	p := adversarialProc(t)
	reader, err := p.reopen(pin)
	if err != nil {
		t.Fatal("reader reopen failed")
	}
	if err := reader.Close(); err != nil {
		t.Fatal("reader close failed")
	}
	if err := pin.validate(); err != nil {
		t.Fatal("reader close revoked independent pin")
	}
	if err := p.validate(); err != nil {
		t.Fatal("reader close revoked independent proc capability")
	}
}

func TestReadFileAdversarialMovedParentReopensPinnedInode(t *testing.T) {
	root, ancestor := localFixture(t)
	child, err := childAt(ancestor.fd, "parent", ancestor.id.Owner, true)
	if err != nil {
		t.Fatal("cannot create owned child fixture")
	}
	ownChild(t, child)
	writeAdversarialFile(t, filepath.Join(root, "parent"), "entry", []byte("original"))
	pin := adversarialPin(t, child, "entry", 32)
	p := adversarialProc(t)
	if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "moved")); err != nil {
		t.Fatal("cannot rename parent fixture")
	}
	if err := os.Mkdir(filepath.Join(root, "parent"), 0o700); err != nil {
		t.Fatal("cannot create replacement parent fixture")
	}
	writeAdversarialFile(t, filepath.Join(root, "parent"), "entry", []byte("replacement"))
	reader, err := p.reopen(pin)
	if err != nil {
		t.Fatal("validated proc could not reopen unchanged pinned inode")
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || string(data) != "original" {
		t.Fatal("reopening pin followed replacement parent pathname")
	}
	if err := pin.validate(); err != nil {
		t.Fatal("unchanged pinned child edge rejected")
	}
	if err := validateEdges([]edge{ancestor, child}, ancestor.id.Owner); err == nil {
		t.Fatal("full retained chain accepted substituted parent edge")
	}
}

func TestReadFileAdversarialConcurrentFDChurn(t *testing.T) {
	root, parent := localFixture(t)
	want := []byte("stable pinned content")
	writeAdversarialFile(t, root, "entry", want)
	pin := adversarialPin(t, parent, "entry", 32)
	p := adversarialProc(t)
	started := make(chan struct{})
	stop := make(chan struct{})
	done := make(chan struct{})
	var churnErr error
	go func() {
		defer close(done)
		signaled := false
		defer func() {
			if !signaled {
				close(started)
			}
		}()
		for range 1024 {
			select {
			case <-stop:
				return
			default:
			}
			fd, err := unix.Openat(parent.fd, "entry", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				churnErr = err
				return
			}
			duplicate, dupErr := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 128)
			closeErr := unix.Close(fd)
			var duplicateCloseErr error
			if dupErr == nil {
				duplicateCloseErr = unix.Close(duplicate)
			}
			if err := errors.Join(dupErr, closeErr, duplicateCloseErr); err != nil {
				churnErr = err
				return
			}
			if !signaled {
				close(started)
				signaled = true
			}
			runtime.Gosched()
		}
	}()
	defer func() {
		close(stop)
		<-done
		if churnErr != nil {
			t.Error("owned descriptor churn failed")
		}
	}()
	<-started
	for range 32 {
		reader, err := p.reopen(pin)
		if err != nil {
			t.Fatal("stable pin reopen failed during unrelated descriptor churn")
		}
		data, readErr := readContents(reader, int64(len(want)))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(data, want) || pin.validate() != nil {
			t.Fatal("descriptor churn changed pinned content or authority")
		}
	}
}

func TestReadFileAdversarialRaceAndCleanup(t *testing.T) {
	root, parent := localFixture(t)
	want := []byte("permitted fixture")
	outside := t.TempDir()
	writeAdversarialFile(t, outside, "sentinel", []byte("outside fixture"))
	writeAdversarialFile(t, root, "entry", want)
	if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(root, "swap")); err != nil {
		t.Fatal("cannot create disposable substitution link")
	}
	before := adversarialFDCount(t)
	for range 16 {
		data, id, err := readFile(parent, "entry", 64)
		if err != nil || !bytes.Equal(data, want) || id.Size != int64(len(want)) {
			t.Fatal("stable private read failed before substitution race")
		}
	}
	stop := make(chan struct{})
	started := make(chan struct{})
	var done sync.WaitGroup
	var swapErr error
	done.Add(1)
	go func() {
		defer done.Done()
		swapErr = unix.Renameat2(parent.fd, "entry", parent.fd, "swap", unix.RENAME_EXCHANGE)
		close(started)
		if swapErr != nil {
			return
		}
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := unix.Renameat2(parent.fd, "entry", parent.fd, "swap", unix.RENAME_EXCHANGE); err != nil {
				swapErr = err
				return
			}
		}
	}()
	<-started
	var invalid bool
	for range 64 {
		data, id, err := readFile(parent, "entry", 64)
		if err != nil {
			invalid = invalid || data != nil || id != (Identity{}) || err.Error() != errDirectory.Error()
		} else {
			invalid = invalid || !bytes.Equal(data, want) || id.Mode&unix.S_IFMT != unix.S_IFREG || id.Links != 1
		}
	}
	close(stop)
	done.Wait()
	if swapErr != nil {
		t.Fatal("disposable exchange race failed")
	}
	if invalid {
		t.Fatal("substitution returned outside bytes or partial failure authority")
	}
	for range 32 {
		data, id, err := readFile(parent, "missing", 64)
		assertAdversarialRefusal(t, data, id, err)
	}
	if after := adversarialFDCount(t); after != before {
		t.Fatal("repeated read/refusal leaked descriptors")
	}
	got, err := os.ReadFile(filepath.Join(outside, "sentinel"))
	if err != nil || string(got) != "outside fixture" {
		t.Fatal("outside sentinel changed")
	}
}

func adversarialPin(t *testing.T, parent edge, name string, limit int64) *pinnedFile {
	t.Helper()
	pin, err := pinFile(parent, name, limit)
	if err != nil {
		t.Fatal("cannot pin safe disposable file")
	}
	t.Cleanup(func() {
		if err := pin.close(); err != nil {
			t.Error("pin cleanup failed")
		}
	})
	flags, err := unix.FcntlInt(uintptr(pin.fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_PATH == 0 {
		t.Fatal("initial file pin grants content access")
	}
	return pin
}

func adversarialProc(t *testing.T) *procAccess {
	t.Helper()
	p, err := openProcAccess()
	if err != nil {
		t.Fatal("required real proc capability unavailable")
	}
	t.Cleanup(func() {
		if err := p.close(); err != nil {
			t.Error("proc cleanup failed")
		}
	})
	return p
}

func writeAdversarialFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
		t.Fatal("cannot write disposable fixture")
	}
}

func assertAdversarialRefusal(t *testing.T, data []byte, id Identity, err error) {
	t.Helper()
	if data != nil || id != (Identity{}) || err == nil || err.Error() != errDirectory.Error() {
		t.Fatal("failure exposed partial bytes, identity, or non-redacted error")
	}
}

func adversarialFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("cannot count disposable-process descriptors")
	}
	return len(entries)
}
