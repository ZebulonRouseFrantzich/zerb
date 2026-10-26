//go:build linux

package safefs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// Local cores exercise real mkdir/descriptor behavior, not absolute admission.
func TestCreateDirectoryExclusiveCore(t *testing.T) {
	root, parent := localFixture(t)
	create := func(name string) (*dirState, error) {
		chain, err := duplicateChain([]edge{parent})
		if err != nil {
			t.Fatal(err)
		}
		return createDirectoryAt(chain, name, parent.id.Owner)
	}
	state, err := create("created")
	if err != nil {
		t.Fatal(err)
	}
	child := &Dir{state: state}
	t.Cleanup(func() {
		if err := child.Close(); err != nil {
			t.Error(err)
		}
	})
	if !state.mkdir || !state.chain[len(state.chain)-1].mkdir || state.identity.Mode != unix.S_IFDIR|0o700 || !sameMount(state.identity, parent.id) {
		t.Fatal("exclusive creation lost required facts")
	}
	if err := validateEdges(state.chain, parent.id.Owner); err != nil {
		t.Fatal(err)
	}
	for _, entry := range state.chain {
		flags, err := unix.FcntlInt(uintptr(entry.fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("creation descriptor lacks CLOEXEC")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("created", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	before := adversarialFDCount(t)
	for range 16 {
		for _, name := range []string{"created", "file", "link"} {
			if got, err := create(name); got != nil || !errors.Is(err, unix.EEXIST) || err.Error() != errDirectoryExists.Error() {
				t.Fatal("existing entry was adopted or collision not preserved")
			}
		}
		for _, name := range []string{"", ".", "..", "a/b", "private\x00marker"} {
			if got, err := create(name); got != nil || err != errDirectory {
				t.Fatal("invalid component created")
			}
		}
	}
	if adversarialFDCount(t) != before {
		t.Fatal("failed exclusive creation leaked descriptors")
	}
	if data, err := os.ReadFile(filepath.Join(root, "file")); err != nil || string(data) != "preserved" {
		t.Fatal("collision changed existing bytes")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("creation closed caller parent")
	}
}

func TestCreateDirectoryRestrictiveUmask(t *testing.T) {
	if os.Getenv("ZERB_CREATE_DIRECTORY_UMASK") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCreateDirectoryRestrictiveUmask$")
		cmd.Env = append(os.Environ(), "ZERB_CREATE_DIRECTORY_UMASK=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated creation check failed: %s", output)
		}
		return
	}
	root, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	previous := unix.Umask(0o777)
	defer unix.Umask(previous)
	if got, err := createDirectoryAt(chain, "restricted", parent.id.Owner); got != nil || err != errDirectory {
		t.Fatal("restrictive umask admitted or returned a handle")
	}
	info, err := os.Lstat(filepath.Join(root, "restricted"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0 {
		t.Fatal("failed creation residue removed or repaired")
	}
}

func TestCreateDirPublicRefusal(t *testing.T) {
	for _, dir := range []*Dir{nil, {}, {state: &dirState{closed: true}}} {
		if got, err := dir.CreateDir("child"); got != nil || err != errDirectory {
			t.Fatal("invalid directory created child")
		}
	}
}
