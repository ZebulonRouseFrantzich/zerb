//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func treeGateFixture() *stageState {
	s := &stageState{workspace: treeFixedWorkspace, root: edge{fd: -1}}
	s.drained = sync.NewCond(&s.mu)
	return s
}

// This fixture only enters descriptor cores. It neither constructs an exported
// Dir nor claims the disposable path's absolute ancestors satisfy policy.
func localTreeCore(t *testing.T) (string, *stageState, func() error, func() error) {
	t.Helper()
	base, parent := localFixture(t)
	if err := unix.Mkdirat(parent.fd, "stage", 0o700); err != nil {
		t.Fatal(err)
	}
	id, err := observeAt(parent.fd, "stage")
	if err != nil {
		t.Fatal(err)
	}
	root, err := openObserved(parent.fd, "stage", id)
	if err != nil {
		t.Fatal(err)
	}
	s := treeGateFixture()
	s.phase = treeDraining
	s.parent = []edge{parent}
	s.root = root
	s.owner = parent.id.Owner
	s.work = make([]treeEntryWork, treeMaxEntries)
	for i := range s.arenas {
		s.arenas[i] = make([]byte, treeMaxPaths)
	}
	t.Cleanup(func() {
		if err := closeChain([]edge{root}); err != nil {
			t.Error(err)
		}
	})
	parentBoundary := func() error {
		if validateEdges(s.parent, s.owner) != nil {
			return errDirectory
		}
		current, err := observeFD(parent.fd)
		if err != nil || !sameDirectory(current, parent.id) {
			return errDirectory
		}
		return nil
	}
	boundary := func() error {
		if parentBoundary() != nil {
			return errDirectory
		}
		current, err := observeFD(s.root.fd)
		linked, linkedErr := observeAt(parent.fd, s.root.name)
		if err != nil || linkedErr != nil || !sameDirectory(current, s.root.id) || !sameDirectory(current, linked) || current.Owner != s.owner || current.Mode != unix.S_IFDIR|0o700 || !sameMount(current, parent.id) {
			return errDirectory
		}
		return nil
	}
	return base, s, boundary, parentBoundary
}

func TestTreeGateResourcesAndRevocation(t *testing.T) {
	s := treeGateFixture()
	for _, tc := range []struct{ fds, scratch, result uint64 }{
		{257, 0, 0}, {0, treeMaxScratch + 1, 0}, {0, 0, treeMaxWorkspace}, {^uint64(0), 0, 0},
	} {
		if op, err := s.begin(tc.fds, tc.scratch, tc.result); err == nil || op != nil {
			t.Fatal("overbudget operation admitted")
		}
		if s.phase != treeActive || s.active != 0 {
			t.Fatal("resource refusal consumed active stage")
		}
	}
	var ops [4]*stageOperation
	for i := range ops {
		op, err := s.begin(1, 32<<10, 1)
		if err != nil {
			t.Fatal(err)
		}
		ops[i] = op
	}
	if op, err := s.begin(0, 0, 0); err == nil || op != nil {
		t.Fatal("fifth operation admitted")
	}
	for _, op := range ops {
		if !op.finish(false) {
			t.Fatal("clean operation poisoned")
		}
	}
	if s.active != 0 || s.fds != 0 || s.scratch != 0 || s.workspace != treeFixedWorkspace {
		t.Fatal("reservation not released")
	}
	op, err := s.beginRole(2, 0, 4096)
	if err != nil {
		t.Fatal(err)
	}
	first := op.retainRole(1, 2048)
	second := op.retainRole(1, 2048)
	if !op.finish(false) || s.roleRefs != 2 {
		t.Fatal("retained role observations lost")
	}
	if started, err := s.terminalStart(); err == nil || started || s.phase != treeActive {
		t.Fatal("observer refusal consumed stage")
	}
	s.release(first, false)
	s.release(second, false)
	if s.roleRefs != 0 || s.fds != 0 || s.workspace != treeFixedWorkspace {
		t.Fatal("role ownership not released")
	}
	a, err := s.begin(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.begin(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if b.finish(true) || a.finish(false) {
		t.Fatal("shared poison allowed successful completion")
	}
	if op, err := s.begin(0, 0, 0); op != nil || err == nil {
		t.Fatal("revoked gate readmitted")
	}
}

func TestTreeGateTerminalDrainsWithoutViewLock(t *testing.T) {
	s := treeGateFixture()
	op, err := s.begin(2, 32<<10, 4096)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.terminalStart(); done <- err }()
	deadline := time.After(5 * time.Second)
	for {
		s.mu.Lock()
		phase := s.phase
		s.mu.Unlock()
		if phase == treeDraining {
			break
		}
		select {
		case <-deadline:
			t.Fatal("terminal did not stop admission")
		default:
		}
		runtime.Gosched()
	}
	if next, err := s.begin(0, 0, 0); next != nil || err == nil {
		t.Fatal("new operation starved drain")
	}
	select {
	case <-done:
		t.Fatal("terminal did not drain")
	default:
	}
	if !op.finish(false) {
		t.Fatal("draining clean operation poisoned")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-deadline:
		t.Fatal("terminal deadlock")
	}
	if err := s.dispose(); err != nil {
		t.Fatal(err)
	}
}

func TestTreeQueuedClosedPublicationDoesNotPoison(t *testing.T) {
	s := treeGateFixture()
	ds := &dirState{gate: s}
	d := &Dir{state: ds}
	ds.mu.Lock()
	done := make(chan error, 1)
	go func() {
		id, err := d.PublishNoReplace("file", []byte("payload"), 0o600)
		if id != (Identity{}) {
			done <- errors.New("closed view returned identity")
			return
		}
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	for {
		s.mu.Lock()
		active := s.active
		s.mu.Unlock()
		if active == 1 {
			break
		}
		select {
		case <-deadline:
			ds.mu.Unlock()
			t.Fatal("writer did not enroll before view lock")
		default:
			runtime.Gosched()
		}
	}
	// The queued operation reaches the same closed state protected by Close's
	// mutex; no descriptor or pathname ever exists in this negative fixture.
	ds.closed = true
	ds.mu.Unlock()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed queued publication succeeded")
		}
	case <-deadline:
		t.Fatal("closed writer did not finish")
	}
	if s.phase != treeActive || s.entries != 0 || s.active != 0 || s.workspace != treeFixedWorkspace {
		t.Fatal("closed-view refusal poisoned or retained reservations")
	}
}

func TestTreeContentAndReadCounters(t *testing.T) {
	for _, which := range []string{"entries", "paths", "payload"} {
		t.Run(which, func(t *testing.T) {
			s := treeGateFixture()
			s.entries = treeMaxEntries - 1
			s.paths = treeMaxPaths - 1
			s.payload = treeMaxPayload - 1
			if !s.addContent(1, 1) {
				t.Fatal("exact simultaneous content maximum refused")
			}
			if s.addContent(0, 0) {
				t.Fatal("entry overflow admitted")
			}
			s.entries = 0
			if s.addContent(1, 0) || s.addContent(0, 1) {
				t.Fatal("path/payload overflow admitted")
			}
		})
	}
	s := treeGateFixture()
	if s.readCharge(treeMaxReads-1) != nil || s.readCharge(1) != nil || s.readCharge(1) == nil || s.reads != treeMaxReads {
		t.Fatal("lifetime read limit not monotonic")
	}
	s.phase = treePreserve
	if s.readCharge(0) == nil {
		t.Fatal("preserved stage allowed read")
	}
}

func TestTreeEscapingCapabilitiesRefuseBeforeIO(t *testing.T) {
	s := treeGateFixture()
	d := &Dir{state: &dirState{gate: s}}
	if got, err := d.StageTree("child", 0o700); got != nil || err == nil {
		t.Fatal("nested stage escaped")
	}
	if got, err := d.OpenLock("lock", true); got != nil || err == nil {
		t.Fatal("lock escaped")
	}
	if got, data, err := d.PinFile("record", 8); got != nil || data != nil || err == nil {
		t.Fatal("retained file escaped")
	}
	if got, err := d.StageReplacement("old", "temp", nil, 0o600, Identity{Inode: 1}); got != nil || err == nil {
		t.Fatal("replacement escaped")
	}
	if s.phase != treeActive || s.active != 0 || s.fds != 0 {
		t.Fatal("capability refusal affected stage")
	}
}

func TestTreeInventoryAndPromotionKernelCore(t *testing.T) {
	base, s, boundary, _ := localTreeCore(t)
	for _, name := range []string{"nested", "nested/deeper", strings.Repeat("n", 255), "raw\xff"} {
		if err := os.MkdirAll(filepath.Join(base, "stage", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("synthetic bounded tree bytes\x00")
	if err := os.WriteFile(filepath.Join(base, "stage", "nested", "payload"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "stage", "empty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeFDs := adversarialFDCount(t)
	calls := 0
	checked := func() error { calls++; return boundary() }
	got, attempted, err := s.promoteWithBoundary("published", checked)
	if err != nil {
		t.Fatal("descriptor promotion refused", err)
	}
	if !attempted || calls < 8 {
		t.Fatal("critical boundary revalidation missing")
	}
	actual, err := observeAt(s.parent[0].fd, "published")
	if err != nil || got != actual {
		t.Fatal("promotion returned stale root identity")
	}
	if _, err := os.Stat(filepath.Join(base, "stage")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old stage spelling retained")
	}
	if _, err := os.Stat(filepath.Join(base, "published", "nested", "deeper")); err != nil {
		t.Fatal("complete nested inventory not preserved")
	}
	if got, err := os.ReadFile(filepath.Join(base, "published", "nested", "payload")); err != nil || !bytes.Equal(got, content) {
		t.Fatal("promoted content differs")
	}
	if s.reads != uint64(3*(len(content)+2)) {
		t.Fatal("verification passes/lookahead not charged", s.reads)
	}
	if adversarialFDCount(t) != beforeFDs {
		t.Fatal("bounded walk leaked descriptors")
	}
}

func TestTreeCleanupKernelCore(t *testing.T) {
	base, s, boundary, parentBoundary := localTreeCore(t)
	if err := os.MkdirAll(filepath.Join(base, "stage", "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "stage", "a", "b", "file"), []byte("remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := s.walkWithBoundary(0, false, boundary)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.removeWithBoundary(inventory, boundary, parentBoundary); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "stage")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("active root remains")
	}
	if got, err := os.ReadFile(filepath.Join(base, "outside")); err != nil || !bytes.Equal(got, []byte("outside")) {
		t.Fatal("outside sentinel changed")
	}
}

func TestTreeInventoryUnsafeAndBoundaries(t *testing.T) {
	for _, tc := range []string{"symlink", "fifo", "hardlink", "suid", "file-size", "total-size", "depth"} {
		t.Run(tc, func(t *testing.T) {
			base, s, boundary, _ := localTreeCore(t)
			root := filepath.Join(base, "stage")
			switch tc {
			case "symlink":
				if err := os.Symlink("../outside", filepath.Join(root, "bad")); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(filepath.Join(root, "bad"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.WriteFile(filepath.Join(root, "one"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(filepath.Join(root, "one"), filepath.Join(root, "two")); err != nil {
					t.Fatal(err)
				}
			case "suid":
				if err := os.Mkdir(filepath.Join(root, "bad"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := unix.Chmod(filepath.Join(root, "bad"), 0o4700); err != nil {
					t.Fatal(err)
				}
			case "file-size", "total-size":
				sizes := []int64{treeMaxFile + 1}
				if tc == "total-size" {
					sizes = []int64{treeMaxFile, treeMaxFile, 1}
				}
				for i, size := range sizes {
					file, err := os.OpenFile(filepath.Join(root, fmt.Sprint(i)), os.O_CREATE|os.O_RDWR, 0o600)
					if err != nil {
						t.Fatal(err)
					}
					if err := file.Truncate(size); err != nil {
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			case "depth":
				path := root
				for range 65 {
					path = filepath.Join(path, "d")
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := adversarialFDCount(t)
			if _, err := s.walkWithBoundary(0, false, boundary); err == nil {
				t.Fatal("unsafe or oversized inventory admitted")
			}
			if adversarialFDCount(t) != before {
				t.Fatal("failed inventory leaked descriptors")
			}
			if _, err := os.Lstat(root); err != nil {
				t.Fatal("failed prevalidation removed root")
			}
		})
	}
}

func TestTreeDepth64AndSnapshotMismatch(t *testing.T) {
	base, s, boundary, _ := localTreeCore(t)
	path := filepath.Join(base, "stage")
	for range treeMaxDepth {
		path = filepath.Join(path, "d")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	before := adversarialFDCount(t)
	a, err := s.walkWithBoundary(0, false, boundary)
	if err != nil || a.count != 64 {
		t.Fatal("legal relative depth64 refused", err)
	}
	if adversarialFDCount(t) != before {
		t.Fatal("deep cursor retained frames")
	}
	if err := os.Mkdir(filepath.Join(base, "stage", "added"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := s.walkWithBoundary(1, false, boundary)
	if err != nil || s.sameInventory(a, b, 0, 1) {
		t.Fatal("complete-inventory change not detected")
	}
}

func TestTreeRelativePathAdmissionBoundaries(t *testing.T) {
	s := treeGateFixture()
	s.parent = make([]edge, 1)
	ds := &dirState{gate: s, chain: []edge{{}, {}, {name: strings.Repeat("a", 255)}, {name: strings.Repeat("b", 255)}, {name: strings.Repeat("c", 255)}, {name: strings.Repeat("d", 254)}}}
	if length, ok := stageRelative(ds, "x"); !ok || length != 1024 {
		t.Fatal("exact raw relative path refused")
	}
	ds.chain[5].name += "d"
	if _, ok := stageRelative(ds, "x"); ok {
		t.Fatal("relative path overflow admitted")
	}
	ds.chain = make([]edge, 65)
	for i := 2; i < len(ds.chain); i++ {
		ds.chain[i].name = "d"
	}
	if _, ok := stageRelative(ds, "d"); !ok {
		t.Fatal("relative depth64 refused")
	}
	ds.chain = append(ds.chain, edge{name: "d"})
	if _, ok := stageRelative(ds, "d"); ok {
		t.Fatal("relative depth65 admitted")
	}
}

func TestTreeEntryInventoryBoundaries(t *testing.T) {
	_, s, boundary, _ := localTreeCore(t)
	for i := 0; i < treeMaxEntries-1; i++ {
		if err := unix.Mkdirat(s.root.fd, fmt.Sprint(i), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for count := treeMaxEntries - 1; count <= treeMaxEntries+1; count++ {
		if count >= treeMaxEntries {
			if err := unix.Mkdirat(s.root.fd, fmt.Sprint(count-1), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		before := adversarialFDCount(t)
		inventory, err := s.walkWithBoundary(0, false, boundary)
		if (err == nil) != (count <= treeMaxEntries) {
			t.Fatal("entry boundary admission differs", count, err)
		}
		if err == nil && inventory.count != count {
			t.Fatal("complete inventory lost entries")
		}
		if adversarialFDCount(t) != before {
			t.Fatal("entry boundary leaked cursor")
		}
	}
}
