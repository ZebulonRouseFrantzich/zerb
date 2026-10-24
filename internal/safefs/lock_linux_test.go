//go:build linux

package safefs

import (
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// ARCH-020/030/031: lock metadata has no file-content allocation limit.
func TestLockMetadata(t *testing.T) {
	good := Identity{Device: 1, Inode: 2, Mode: unix.S_IFREG | 0o600, Owner: 1000, Links: 1, MountID: 3, Size: math.MaxInt64}
	if !safeLock(good, 1000) {
		t.Fatal("ordinary lock metadata rejected")
	}
	for _, edit := range []func(*Identity){
		func(id *Identity) { id.Owner++ },
		func(id *Identity) { id.Mode = unix.S_IFDIR | 0o600 },
		func(id *Identity) { id.Mode = unix.S_IFIFO | 0o600 },
		func(id *Identity) { id.Mode = unix.S_IFLNK | 0o600 },
		func(id *Identity) { id.Mode = unix.S_IFCHR | 0o600 },
		func(id *Identity) { id.Mode |= 0o040 },
		func(id *Identity) { id.Mode |= unix.S_ISUID },
		func(id *Identity) { id.Mode &^= 0o200 },
		func(id *Identity) { id.Links = 0 },
		func(id *Identity) { id.Links = 2 },
		func(id *Identity) { id.MountID = 0 },
		func(id *Identity) { id.Size = -1 },
	} {
		bad := good
		edit(&bad)
		if safeLock(bad, 1000) {
			t.Fatal("unsafe lock metadata admitted")
		}
	}
}

func TestLockCreationAndCollision(t *testing.T) {
	root, parent := localFixture(t)
	if state, err := lockAt(parent, "missing", false); err == nil || state != nil {
		t.Fatal("missing lock opened without creation")
	}
	state, err := lockAt(parent, "entry", true)
	if err != nil {
		t.Fatal("exclusive lock creation failed")
	}
	t.Cleanup(func() {
		if err := state.closeResources(); err != nil {
			t.Error("lock cleanup failed")
		}
	})
	if !safeLock(state.identity, parent.id.Owner) || state.identity.Size != 0 || state.validate() != nil {
		t.Fatal("created lock did not retain the exact safe inode")
	}
	flags, err := unix.FcntlInt(uintptr(state.pin.fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_PATH == 0 {
		t.Fatal("initial lock pin grants content access")
	}
	flags, err = unix.FcntlInt(uintptr(state.fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDWR || flags&unix.O_NONBLOCK == 0 || flags&(unix.O_APPEND|unix.O_TRUNC) != 0 {
		t.Fatal("lock reopen has unexpected access flags")
	}
	if fd, _, err := createLock(parent, "entry"); err == nil || fd != -1 {
		t.Fatal("observed-absence creation collision adopted existing inode")
	}
	if err := state.closeResources(); err != nil {
		t.Fatal("lock resource cleanup failed")
	}
	if err := os.WriteFile(filepath.Join(root, "entry"), []byte("preserved"), 0o600); err != nil {
		t.Fatal("cannot populate disposable existing lock")
	}
	existing, err := lockAt(parent, "entry", true)
	if err != nil {
		t.Fatal("existing nonempty lock rejected")
	}
	if err := existing.closeResources(); err != nil {
		t.Fatal("existing lock cleanup failed")
	}
	data, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || string(data) != "preserved" {
		t.Fatal("lock open changed existing contents")
	}
	for _, name := range []string{"", ".", "..", "a/b", "private\x00marker"} {
		if state, err := lockAt(parent, name, true); state != nil || err == nil || err.Error() != errDirectory.Error() {
			t.Fatal("invalid lock component did not receive fixed refusal")
		}
	}
}

func TestLockUnsafeObjectsBeforeReopen(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "socket", "directory", "hardlink", "read-only", "group-read", "group-write"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			name := filepath.Join(root, "entry")
			switch kind {
			case "symlink":
				if err := os.Symlink("missing", name); err != nil {
					t.Fatal("cannot create disposable symlink")
				}
			case "fifo":
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
						t.Error("socket cleanup failed")
					}
				}()
				socketName := "/proc/self/fd/" + strconv.Itoa(parent.fd) + "/entry"
				if err := unix.Bind(fd, &unix.SockaddrUnix{Name: socketName}); err != nil {
					t.Fatal("cannot bind disposable socket")
				}
			case "directory":
				if err := os.Mkdir(name, 0o700); err != nil {
					t.Fatal("cannot create disposable directory")
				}
			default:
				if err := os.WriteFile(name, []byte("untouched"), 0o600); err != nil {
					t.Fatal("cannot create disposable file")
				}
				if kind == "hardlink" {
					if err := os.Link(name, filepath.Join(root, "alias")); err != nil {
						t.Fatal("cannot create disposable alias")
					}
				} else {
					mode := os.FileMode(0o400)
					switch kind {
					case "group-read":
						mode = 0o640
					case "group-write":
						mode = 0o620
					}
					if err := os.Chmod(name, mode); err != nil {
						t.Fatal("cannot set disposable unsafe mode")
					}
				}
			}
			before, err := observeAt(parent.fd, "entry")
			if err != nil {
				t.Fatal("cannot observe unsafe fixture")
			}
			count := adversarialFDCount(t)
			if pin, err := pinLock(parent, "entry"); pin != nil || err == nil || err.Error() != errDirectory.Error() {
				if pin != nil {
					if closeErr := pin.close(); closeErr != nil {
						t.Error("unexpected pin cleanup failed")
					}
				}
				t.Fatal("unsafe object received a lock pin")
			}
			if state, err := lockAt(parent, "entry", true); state != nil || err == nil || err.Error() != errDirectory.Error() {
				if state != nil {
					if closeErr := state.closeResources(); closeErr != nil {
						t.Error("unexpected state cleanup failed")
					}
				}
				t.Fatal("unsafe object received writable lock access")
			}
			after, err := observeAt(parent.fd, "entry")
			if err != nil || before != after || adversarialFDCount(t) != count {
				t.Fatal("unsafe lock refusal changed the entry or leaked descriptors")
			}
		})
	}
}

func TestLockCreationRestrictiveUmask(t *testing.T) {
	if os.Getenv("ZERB_SAFEFS_LOCK_UMASK") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLockCreationRestrictiveUmask$")
		cmd.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_UMASK=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated lock umask check failed: %s", output)
		}
		return
	}
	root, parent := localFixture(t)
	previous := unix.Umask(0o777)
	defer unix.Umask(previous)
	before := adversarialFDCount(t)
	if state, err := lockAt(parent, "restricted", true); state != nil || err == nil || err.Error() != errDirectory.Error() {
		t.Fatal("restrictive umask silently admitted or exposed lock authority")
	}
	info, err := os.Lstat(filepath.Join(root, "restricted"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0 || info.Size() != 0 {
		t.Fatal("failed lock publication was repaired, removed or written")
	}
	if after := adversarialFDCount(t); after != before {
		t.Fatal("failed creation leaked its exclusive descriptor")
	}
}

func TestLockInvalidHandles(t *testing.T) {
	for _, lock := range []*Lock{nil, {}, {state: &lockState{fd: -1, closed: true}}, {state: &lockState{fd: -1}}} {
		for _, mode := range []LockMode{0, LockShared, LockExclusive, 255} {
			if err := lock.Try(mode); err == nil || err.Error() != errDirectory.Error() {
				t.Fatal("invalid lock acquired authority")
			}
		}
		if err := lock.Unlock(); err == nil || err.Error() != errDirectory.Error() {
			t.Fatal("invalid lock unlocked successfully")
		}
		if lock.Identity() != (Identity{}) {
			t.Fatal("invalid lock retained identity")
		}
		if err := lock.Close(); err != nil {
			t.Fatal("invalid or closed lock cleanup failed")
		}
	}
	_, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal("cannot duplicate local fixture")
	}
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	defer dir.Close()
	for _, invalid := range []*Dir{nil, {}, dir} {
		if lock, err := invalid.OpenLock("entry", true); lock != nil || err == nil || err.Error() != errDirectory.Error() {
			t.Fatal("invalid absolute topology admitted a public lock")
		}
	}
}

func TestLockValueCopiesCloseAfterTopologyFailure(t *testing.T) {
	_, parent := localFixture(t)
	owned, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal("cannot duplicate fixture")
	}
	state, err := lockAt(owned[0], "entry", true)
	if err != nil {
		if closeErr := closeChain(owned); closeErr != nil {
			t.Error("failed fixture cleanup")
		}
		t.Fatal("cannot create private lock fixture")
	}
	state.chain = owned
	lock := &Lock{state: state}
	copyLock := *lock
	if lock.Identity() == (Identity{}) || copyLock.Identity() != lock.Identity() {
		t.Fatal("value copy lost cached observation")
	}
	fds := []int{state.fd, state.pin.fd, state.proc.self.fd}
	for _, entry := range append(append([]edge{}, state.chain...), state.proc.chain...) {
		fds = append(fds, entry.fd)
	}
	var wait sync.WaitGroup
	for range 24 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 8 {
				copyLock.Identity()
				if err := copyLock.Close(); err != nil && err.Error() != errDirectory.Error() {
					t.Error("close exposed a non-redacted error")
				}
				if err := lock.Try(LockShared); err == nil || err.Error() != errDirectory.Error() {
					t.Error("local helper topology became absolute authority")
				}
			}
		}()
	}
	wait.Wait()
	if lock.Identity() != (Identity{}) || copyLock.Identity() != (Identity{}) {
		t.Fatal("closed copy retained identity")
	}
	for _, fd := range fds {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			t.Fatal("topology refusal leaked an owned descriptor")
		}
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("lock cleanup closed borrowed fixture descriptor")
	}
}

func TestLockPublicPositiveOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive OpenLock requires an absolute topology admitted by the approved owner/mode policy")
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal("admissible root refused")
	}
	defer dir.Close()
	lock, err := dir.OpenLock("entry", true)
	if err != nil {
		t.Fatal("public lock creation refused")
	}
	defer lock.Close()
	if err := dir.Close(); err != nil {
		t.Fatal("parent close failed")
	}
	if err := lock.Try(LockExclusive); err != nil {
		t.Fatal("lock did not survive parent close")
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal("public lock unlock failed")
	}
	if err := lock.Close(); err != nil || lock.Identity() != (Identity{}) {
		t.Fatal("public lock cleanup failed")
	}
}
