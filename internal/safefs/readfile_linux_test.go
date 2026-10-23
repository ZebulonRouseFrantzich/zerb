//go:build linux

package safefs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func requireReadRefusal(t *testing.T, data []byte, id Identity, err error) {
	t.Helper()
	if data != nil || id != (Identity{}) || err == nil || err.Error() != errDirectory.Error() {
		t.Error("read did not return fixed refusal without bytes or identity")
	}
}

func TestReadFileContentsAndLimits(t *testing.T) {
	root, parent := localFixture(t)
	for _, test := range []struct {
		name, content string
		limit         int64
	}{
		{"empty", "", 0},
		{"exact", "abc", 3},
		{"room", "abc", 4},
		{"raw\xff name", "abc", 3},
	} {
		if err := os.WriteFile(filepath.Join(root, test.name), []byte(test.content), 0o600); err != nil {
			t.Fatal(err)
		}
		data, id, err := readFile(parent, test.name, test.limit)
		if err != nil || string(data) != test.content || !safeFile(id, parent.id.Owner, test.limit) || id.Size != int64(len(data)) {
			t.Fatal("bounded ordinary read failed")
		}
	}
	for _, limit := range []int64{-1, 0, 2, int64(int(^uint(0) >> 1))} {
		data, id, err := readFile(parent, "exact", limit)
		requireReadRefusal(t, data, id, err)
	}
	for _, name := range []string{"", ".", "..", "private/marker", "private\x00marker", "missing"} {
		data, id, err := readFile(parent, name, 10)
		requireReadRefusal(t, data, id, err)
	}
}

func TestReadContentsTruncationAndGrowth(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "content-")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("abc"); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{0, 2, 4, -1, int64(int(^uint(0) >> 1))} {
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		if data, err := readContents(file, size); err == nil || data != nil {
			t.Fatal("changed/invalid expected length retained bytes")
		}
	}
	const priorSize = 64 << 20
	if err := file.Truncate(priorSize); err != nil {
		t.Fatal(err)
	}
	if info, err := file.Stat(); err != nil || info.Size() != priorSize {
		t.Fatal("sparse pre-observation fixture unavailable")
	}
	if err := file.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	data, readErr := readContents(file, priorSize)
	runtime.ReadMemStats(&after)
	if readErr == nil || data != nil || after.TotalAlloc-before.TotalAlloc > 4<<20 {
		t.Fatal("truncation eagerly allocated from the prior claimed size or retained bytes")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := readContents(file, 1); data != nil || err == nil {
		t.Fatal("closed reader retained a result")
	}
	if data, err := readContents(nil, 1); data != nil || err == nil {
		t.Fatal("nil reader admitted")
	}
}

func TestReadCleanupFailure(t *testing.T) {
	for _, primary := range []error{nil, errors.New("private pathname marker")} {
		file, err := os.CreateTemp(t.TempDir(), "reader-")
		if err != nil {
			t.Fatal(err)
		}
		if primary == nil {
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		data, id, err := finishFileRead(file, []byte("private content marker"), Identity{Inode: 1}, primary)
		requireReadRefusal(t, data, id, err)
		if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatal("read cleanup left the descriptor open")
		}
	}
}

func TestReadFileRefusalAndConcurrentClose(t *testing.T) {
	_, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	// A local descriptor chain does not substitute for absolute-root admission.
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	copyDir := *dir
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, id, err := dir.ReadFile("entry", 10)
			requireReadRefusal(t, data, id, err)
			if err := copyDir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, invalid := range []*Dir{nil, {}, {state: &dirState{}}, dir} {
		data, id, err := invalid.ReadFile("entry", 10)
		requireReadRefusal(t, data, id, err)
	}
}

func TestReadFilePositiveAndCloseOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive ReadFile/Close requires an absolute topology admitted by the approved owner/mode policy")
	}
	if err := os.WriteFile(filepath.Join(root, "entry"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	data, _, err := dir.ReadFile("entry", 3)
	if err != nil || string(data) != "abc" {
		t.Fatal("public read failed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, id, err := dir.ReadFile("entry", 3)
			if err != nil {
				requireReadRefusal(t, data, id, err)
			} else if string(data) != "abc" {
				t.Error("public read returned changed bytes")
			}
			if err := dir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
