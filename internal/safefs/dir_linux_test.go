//go:build linux

package safefs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOwnerPolicy(t *testing.T) {
	for _, test := range []struct {
		name   string
		owners []uint32
		bad    bool
	}{
		{"root-prefix", []uint32{0, 0, 1000, 1000}, false},
		{"owned-root", []uint32{1000, 1000}, false},
		{"foreign-prefix", []uint32{65534, 1000}, true},
		{"root-switchback", []uint32{0, 1000, 0}, true},
		{"foreign-subtree", []uint32{0, 1000, 1001}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			owned := false
			var err error
			for _, owner := range test.owners {
				owned, err = advanceOwner(Identity{Owner: owner, Mode: unix.S_IFDIR | 0o700, Links: 2, MountID: 7}, 1000, owned)
				if err != nil {
					break
				}
			}
			if (err != nil) != test.bad {
				t.Fatal("unexpected owner admission")
			}
		})
	}
	valid := Identity{Mode: unix.S_IFDIR | 0o755, Links: 2, MountID: 7}
	for _, bad := range []Identity{
		{Mode: unix.S_IFDIR | 0o777, Links: 2, MountID: 7},
		{Mode: unix.S_IFDIR | unix.S_ISVTX | 0o777, Links: 2, MountID: 7},
		{Mode: unix.S_IFDIR | 0o720, Links: 2, MountID: 7},
		{Mode: unix.S_IFLNK | 0o700, Links: 2, MountID: 7},
		{Mode: unix.S_IFREG | 0o600, Links: 1, MountID: 7},
		{Mode: unix.S_IFDIR | 0o700, Links: 0, MountID: 7},
		{Mode: unix.S_IFDIR | 0o700, Links: 2, MountID: 0},
	} {
		if safeDirectory(bad) {
			t.Fatal("unsafe directory admitted")
		}
	}
	if !safeDirectory(valid) {
		t.Fatal("safe structural directory rejected")
	}
}

func TestStableIdentity(t *testing.T) {
	a := Identity{Device: 1, Inode: 2, Owner: 3, Mode: unix.S_IFDIR | 0o700, MountID: 4, Links: 2, Size: 64}
	b := a
	b.Size, b.Links = 128, 3
	if !sameDirectory(a, b) {
		t.Fatal("ordinary directory growth invalidated identity")
	}
	for _, edit := range []func(*Identity){
		func(i *Identity) { i.Device++ }, func(i *Identity) { i.Inode++ },
		func(i *Identity) { i.MountID++ }, func(i *Identity) { i.Owner++ },
		func(i *Identity) { i.Mode ^= 0o100 },
	} {
		b := a
		edit(&b)
		if sameDirectory(a, b) {
			t.Fatal("changed identity accepted")
		}
	}
}

// These fixtures exercise private descriptor operations, not absolute-root admission.
func localFixture(t *testing.T) (string, edge) {
	t.Helper()
	directory := t.TempDir()
	fd, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open disposable fixture")
	}
	t.Cleanup(func() {
		if err := unix.Close(fd); err != nil {
			t.Error("fixture descriptor cleanup failed")
		}
	})
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal("descriptor identity unavailable")
	}
	return directory, edge{fd: fd, id: id}
}

func ownChild(t *testing.T, child edge) {
	t.Helper()
	t.Cleanup(func() {
		if err := closeChain([]edge{child}); err != nil {
			t.Error("child descriptor cleanup failed")
		}
	})
	flags, err := unix.FcntlInt(uintptr(child.fd), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("descriptor is not close-on-exec")
	}
}

func TestChildDescriptorOperations(t *testing.T) {
	root, parent := localFixture(t)
	owner := parent.id.Owner
	if _, err := childAt(parent.fd, "missing", owner, false); err == nil {
		t.Fatal("missing child unexpectedly opened")
	}
	child, err := childAt(parent.fd, "new", owner, true)
	if err != nil || child.id.Mode != unix.S_IFDIR|0o700 || child.id.MountID == 0 {
		t.Fatal("safe child creation failed")
	}
	ownChild(t, child)
	if err := validateEdges([]edge{parent, child}, owner); err != nil {
		t.Fatal("valid local edges rejected")
	}
	existing, err := childAt(parent.fd, "new", owner, false)
	if err != nil || !sameDirectory(existing.id, child.id) {
		t.Fatal("existing child identity changed")
	}
	ownChild(t, existing)
	for _, name := range []string{"", ".", "..", "/", "a/b", "Private\x00Marker"} {
		if _, err := childAt(parent.fd, name, owner, true); err == nil {
			t.Fatal("invalid child name accepted")
		}
	}
	if _, err := createChild(parent.fd, "new", owner); err == nil {
		t.Fatal("missing-to-existing collision adopted")
	}
	if _, err := childAt(parent.fd, "new", owner+1, false); err == nil {
		t.Fatal("wrong-owner child accepted")
	}
	if err := os.Symlink("new", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link", "file"} {
		if _, err := childAt(parent.fd, name, owner, true); err == nil {
			t.Fatal("non-directory child accepted")
		}
	}
	if err := os.Chmod(filepath.Join(root, "new"), 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := childAt(parent.fd, "new", owner, true); err == nil {
		t.Fatal("unsafe existing child repaired or accepted")
	}
	info, err := os.Lstat(filepath.Join(root, "new"))
	if err != nil || info.Mode().Perm() != 0o777 {
		t.Fatal("unsafe existing mode was changed")
	}
}

func TestObservedSubstitutionAndRename(t *testing.T) {
	root, parent := localFixture(t)
	child, err := childAt(parent.fd, "child", parent.id.Owner, true)
	if err != nil {
		t.Fatal(err)
	}
	ownChild(t, child)
	if err := os.Rename(filepath.Join(root, "child"), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateEdges([]edge{parent, child}, parent.id.Owner); err == nil {
		t.Fatal("renamed/replaced edge accepted")
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for range 64 {
		if _, err := openObserved(parent.fd, "child", child.id); err == nil {
			t.Fatal("replacement between observation and open accepted")
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("failed identity comparison leaked descriptors")
	}
	if err := os.Remove(filepath.Join(root, "child")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("moved", filepath.Join(root, "child")); err != nil {
		t.Fatal(err)
	}
	if _, err := openObserved(parent.fd, "child", child.id); err == nil {
		t.Fatal("symlink substituted after observation followed")
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := validateEdges([]edge{parent}, parent.id.Owner); err == nil {
		t.Fatal("ancestor mode substitution accepted")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestDescriptorOwnershipAndConcurrentClose(t *testing.T) {
	_, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	childChain, err := duplicateChain(chain)
	if err != nil {
		t.Fatal(err)
	}
	child := &Dir{state: &dirState{chain: childChain, owner: parent.id.Owner, identity: parent.id}}
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	copyDir := *dir
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 20 {
				copyDir.Identity()
				if err := dir.Close(); err != nil {
					t.Error("close failed")
				}
			}
		}()
	}
	wait.Wait()
	if dir.Identity() != (Identity{}) || copyDir.Identity() != (Identity{}) {
		t.Fatal("closed copy retained authority")
	}
	if err := validateEdges(child.state.chain, parent.id.Owner); err != nil {
		t.Fatal("independent child descriptors were closed with parent")
	}
	for _, e := range child.state.chain {
		flags, err := unix.FcntlInt(uintptr(e.fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("duplicate is not close-on-exec")
		}
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []*Dir{nil, {}, &copyDir} {
		if _, err := dir.OpenDir("child", true); err == nil || !strings.HasPrefix(err.Error(), errDirectory.Error()) {
			t.Fatal("invalid handle retained operational authority")
		}
		if err := dir.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPublicRootRefusal(t *testing.T) {
	for _, name := range []string{"", ".", "relative", "/a/../b", "/a//b", "/Private\x00Marker"} {
		if _, err := OpenRoot(name, uint32(os.Geteuid())); err == nil || err.Error() != errDirectory.Error() {
			t.Fatal("noncanonical root did not receive fixed refusal")
		}
	}
	root := t.TempDir()
	if !admissibleFixture(root, uint32(os.Geteuid())) {
		if dir, err := OpenRoot(root, uint32(os.Geteuid())); err == nil || dir != nil {
			t.Fatal("inadmissible cloud topology accepted")
		}
	}
}

func TestPublicPositiveOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive absolute-root topology unavailable under approved owner/mode policy")
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal("admissible root failed")
	}
	defer func() {
		if err := dir.Close(); err != nil {
			t.Error(err)
		}
	}()
	child, err := dir.OpenDir("child", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := child.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := dir.Close(); err != nil {
		t.Fatal(err)
	}
	grandchild, err := child.OpenDir("grandchild", true)
	if err != nil {
		t.Fatal("child did not survive parent closure")
	}
	if err := grandchild.Close(); err != nil {
		t.Fatal(err)
	}
}

func admissibleFixture(name string, owner uint32) bool {
	owned := false
	current := "/"
	parts := append([]string{""}, strings.Split(strings.TrimPrefix(name, "/"), "/")...)
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			return false
		}
		var st unix.Stat_t
		if unix.Lstat(current, &st) != nil {
			return false
		}
		if st.Uid == owner {
			owned = true
		} else if owned || st.Uid != 0 {
			return false
		}
	}
	return owned
}

func TestDescriptorHelpersSubprocess(t *testing.T) {
	mode := os.Getenv("ZERB_SAFEFS_HELPER")
	if mode == "" {
		for _, mode := range []string{"umask", "pressure"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDescriptorHelpersSubprocess$")
			cmd.Env = append(os.Environ(), "ZERB_SAFEFS_HELPER="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("isolated descriptor check failed: %s", output)
			}
		}
		return
	}
	root, parent := localFixture(t)
	if mode == "umask" {
		previous := unix.Umask(0o777)
		defer unix.Umask(previous)
		if _, err := createChild(parent.fd, "restricted", parent.id.Owner); err == nil {
			t.Fatal("restrictive umask silently admitted")
		}
		info, err := os.Lstat(filepath.Join(root, "restricted"))
		if err != nil || info.Mode().Perm() != 0 {
			t.Fatal("restrictive object repaired")
		}
		return
	}
	if mode != "pressure" {
		t.Fatal("unknown subprocess mode")
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	previous := limit
	defer func() {
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &previous); err != nil {
			t.Error("subprocess descriptor limit restoration failed")
		}
	}()
	limit.Cur = 32
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	chain := make([]edge, 64)
	for i := range chain {
		chain[i] = parent
	}
	if _, err := duplicateChain(chain); err == nil {
		t.Fatal("descriptor exhaustion unexpectedly succeeded")
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("partial descriptor duplication leaked")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("original descriptor closed during failed duplication")
	}
	if _, err := observeFD(-1); !errors.Is(err, errDirectory) {
		t.Fatal("invalid descriptor did not fail closed")
	}
}
