//go:build linux

package safefs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func requireSyncRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil || err.Error() != errDirectory.Error() {
		t.Fatal("sync did not return fixed redacted refusal")
	}
}

func TestSyncDirectoryDescriptor(t *testing.T) {
	root, parent := localFixture(t)
	for _, name := range []string{"first", "second"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syncDirectory(parent); err != nil {
			t.Fatal("directory syscall failed after ordinary size/link growth")
		}
	}
	if current, err := observeFD(parent.fd); err != nil || !sameDirectory(current, parent.id) {
		t.Fatal("sync lost retained descriptor identity")
	}
	if err := syncDirectory(parent); err != nil {
		t.Fatal("repeated directory sync failed")
	}
}

func TestSyncDescriptorFailures(t *testing.T) {
	root, parent := localFixture(t)
	requireSyncRefusal(t, syncDirectory(edge{fd: -1, id: parent.id}))
	pathfd, err := unix.Open(root, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	ownChild(t, edge{fd: pathfd})
	// statx succeeds, but fsync on an O_PATH descriptor fails with EBADF.
	if id, err := observeFD(pathfd); err != nil || !sameDirectory(id, parent.id) {
		t.Fatal("path-only descriptor fixture unavailable")
	}
	requireSyncRefusal(t, syncDirectory(edge{fd: pathfd, id: parent.id}))
	file, err := os.OpenFile(filepath.Join(root, "regular"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	id, err := observeFD(int(file.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	requireSyncRefusal(t, syncDirectory(edge{fd: int(file.Fd()), id: id}))
	if _, err := file.Write([]byte("disposable sentinel")); err != nil {
		t.Fatal("refusal closed the unrelated regular descriptor")
	}
}

func TestSyncRetainedIdentity(t *testing.T) {
	root, parent := localFixture(t)
	for _, change := range []func(*Identity){
		func(id *Identity) { id.Owner++ },
		func(id *Identity) { id.Mode ^= 0o200 },
		func(id *Identity) { id.Inode++ },
		func(id *Identity) { id.MountID++ },
		func(id *Identity) { id.Links = 0 },
	} {
		wrong := parent
		change(&wrong.id)
		requireSyncRefusal(t, syncDirectory(wrong))
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	requireSyncRefusal(t, syncDirectory(parent))
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	requireSyncRefusal(t, syncDirectory(parent))
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	// Zero-link directories fail even though the open descriptor still exists.
	requireSyncRefusal(t, syncDirectory(parent))
}

func TestSyncRefusalAndSharedClose(t *testing.T) {
	_, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	// A local descriptor chain is not an admitted absolute-root handle.
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	copyDir := *dir
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requireSyncRefusal(t, dir.Sync())
			if err := copyDir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, invalid := range []*Dir{nil, {}, {state: &dirState{}}, dir} {
		requireSyncRefusal(t, invalid.Sync())
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("closing shared handle closed the independently owned fixture")
	}
}

func TestSyncPositiveAndConcurrentCloseOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive Sync/Close requires an absolute topology admitted by the approved owner/mode policy")
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := os.Mkdir(filepath.Join(root, "entry"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := dir.Sync(); err != nil {
			t.Fatal("public directory sync failed")
		}
	}
	copyDir := *dir
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := dir.Sync(); err != nil {
				requireSyncRefusal(t, err)
			}
			if err := copyDir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
