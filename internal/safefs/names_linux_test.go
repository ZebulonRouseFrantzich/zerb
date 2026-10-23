//go:build linux

package safefs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func requireListingRefusal(t *testing.T, names []string, err error) {
	t.Helper()
	if names != nil || err == nil || err.Error() != errDirectory.Error() {
		t.Fatal("listing did not return a redacted refusal without partial names")
	}
}

func TestListingLimitsAndRawNames(t *testing.T) {
	root, parent := localFixture(t)
	if names, err := listNames(parent, 0); err != nil || len(names) != 0 {
		t.Fatal("empty zero-limit directory rejected")
	}
	for _, limit := range []int{-1, int(^uint(0) >> 1)} {
		names, err := listNames(edge{fd: -1}, limit)
		requireListingRefusal(t, names, err)
	}
	want := []string{"z", "a", "space name", "line\nbreak", "back\\slash", "invalid\xff", "link", "fifo", "directory"}
	for _, name := range want[:6] {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "sentinel")
	const marker = "untouched external marker"
	if err := os.WriteFile(outside, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	sort.Strings(want)
	offset, err := unix.Seek(parent.fd, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{len(want), len(want) + 1} {
		for repeat := 0; repeat < 3; repeat++ {
			got, err := listNames(parent, limit)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("listing did not return every sorted raw name with an independent cursor")
			}
		}
	}
	for _, limit := range []int{0, 1, len(want) - 1} {
		names, err := listNames(parent, limit)
		requireListingRefusal(t, names, err)
	}
	if after, err := unix.Seek(parent.fd, 0, 1); err != nil || after != offset {
		t.Fatal("listing moved the retained directory cursor")
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != marker {
		t.Fatal("listing affected the external sentinel")
	}
}

func TestListingConcurrentIndependentCursors(t *testing.T) {
	root, parent := localFixture(t)
	if err := os.WriteFile(filepath.Join(root, "entry"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names, err := listNames(parent, 1)
			if err != nil || !reflect.DeepEqual(names, []string{"entry"}) {
				t.Error("simultaneous listing cursors interfered")
			}
		}()
	}
	wg.Wait()
}

func TestListingCursorIdentityAndCleanup(t *testing.T) {
	root, parent := localFixture(t)
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stamp, err := observeListing(parent.fd)
	if err != nil {
		t.Fatal(err)
	}
	first, err := openListing(parent.fd, stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := openListing(parent.fd, stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, cursor := range []*os.File{first, second} {
		flags, err := unix.FcntlInt(cursor.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("listing cursor is not close-on-exec")
		}
	}
	if names, err := first.Readdirnames(1); err != nil || len(names) != 1 {
		t.Fatal("cannot advance first cursor")
	}
	if names, err := second.Readdirnames(4); err != nil || len(names) != 3 {
		t.Fatal("independent cursor inherited another cursor's offset")
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	wrong := stamp
	wrong.identity.Inode++
	for i := 0; i < 64; i++ {
		if cursor, err := openListing(parent.fd, wrong); err == nil || cursor != nil {
			t.Fatal("different listing identity admitted")
		}
		names, err := listNames(parent, 0)
		requireListingRefusal(t, names, err)
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("refused listing leaked a descriptor")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("listing closed its retained parent")
	}
}

func TestListingObservedChanges(t *testing.T) {
	root, parent := localFixture(t)
	before, err := observeListing(parent.fd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkListing(parent.fd, before); err == nil {
		t.Fatal("changed listing metadata accepted")
	}
	after, err := observeListing(parent.fd)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*listingStamp){
		func(s *listingStamp) { s.modified.Nsec++ },
		func(s *listingStamp) { s.changed.Nsec++ },
		func(s *listingStamp) { s.identity.Size++ },
		func(s *listingStamp) { s.identity.Links++ },
	} {
		wrong := after
		change(&wrong)
		if err := checkListing(parent.fd, wrong); err == nil {
			t.Fatal("changed listing stamp accepted")
		}
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	names, err := listNames(parent, 10)
	requireListingRefusal(t, names, err)
	if _, err := observeListing(-1); err == nil {
		t.Fatal("invalid listing descriptor admitted")
	}
}

func TestListingBindsRetainedIdentity(t *testing.T) {
	root, parent := localFixture(t)
	wrong := parent
	wrong.id.Owner++
	names, err := listNames(wrong, 1)
	requireListingRefusal(t, names, err)
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(root, 0o700); err != nil {
			t.Error(err)
		}
	}()
	// Both modes are safe, but the observed change must not become a new baseline.
	if fresh, err := observeListing(parent.fd); err != nil || !safeDirectory(fresh.identity) {
		t.Fatal("safe-mode fixture unavailable")
	}
	names, err = listNames(parent, 1)
	requireListingRefusal(t, names, err)
}

func TestListingValidationAndCloseFailures(t *testing.T) {
	for _, input := range [][]string{{"a", "a"}, {""}, {"."}, {".."}, {"private/marker"}, {"private\x00marker"}} {
		names, err := checkedNames(input, 10)
		requireListingRefusal(t, names, err)
	}
	_, parent := localFixture(t)
	for _, primary := range []error{nil, errors.New("private pathname marker")} {
		stamp, err := observeListing(parent.fd)
		if err != nil {
			t.Fatal(err)
		}
		cursor, err := openListing(parent.fd, stamp)
		if err != nil {
			t.Fatal(err)
		}
		if primary == nil {
			if err := cursor.Close(); err != nil {
				t.Fatal(err)
			}
		}
		names, err := finishListing(cursor, []string{"private marker"}, primary)
		requireListingRefusal(t, names, err)
		if _, err := cursor.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatal("failed listing retained its cursor")
		}
	}
}

func TestNamesRefusalAndConcurrentClose(t *testing.T) {
	_, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	// This local chain is deliberately not an admitted absolute-root handle.
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names, err := dir.Names(1)
			requireListingRefusal(t, names, err)
			if err := dir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, invalid := range []*Dir{nil, {}, dir} {
		names, err := invalid.Names(1)
		requireListingRefusal(t, names, err)
	}
}

func TestNamesPositiveAndConcurrentCloseOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive Names/Close requires an absolute topology admitted by the approved owner/mode policy")
	}
	if err := os.WriteFile(filepath.Join(root, "entry"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		names, err := dir.Names(1)
		if err != nil || !reflect.DeepEqual(names, []string{"entry"}) {
			t.Fatal("public listing failed")
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names, err := dir.Names(1)
			if err != nil {
				requireListingRefusal(t, names, err)
			} else if !reflect.DeepEqual(names, []string{"entry"}) {
				t.Error("concurrent listing returned partial names")
			}
			if err := dir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
