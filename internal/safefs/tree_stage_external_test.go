//go:build linux

package safefs_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func TestTreeStagePublicInvalidParent(t *testing.T) {
	for _, parent := range []*safefs.Dir{nil, {}} {
		for _, name := range []string{"stage", "", ".", "..", "a/b", "bad\x00name"} {
			for _, mode := range []uint32{0, 0o700, 0o755, 0o777} {
				stage, err := parent.StageTree(name, mode)
				if err == nil || stage != nil {
					t.Fatal("invalid parent granted a tree capability")
				}
			}
		}
	}
}

// This preflight recognizes policy refusal, never substitutes for OpenRoot.
func treePublicParent(t *testing.T) (string, *safefs.Dir) {
	t.Helper()
	root := t.TempDir()
	var ancestors []string
	for current := root; ; current = filepath.Dir(current) {
		ancestors = append(ancestors, current)
		if current == "/" {
			break
		}
	}
	slices.Reverse(ancestors)
	owner, owned := uint32(os.Geteuid()), false
	for _, name := range ancestors {
		info, err := os.Lstat(name)
		if err != nil {
			t.Fatal("disposable ancestor inspection failed")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
			t.Skip("positive public TreeStage path requires non-writable, no-follow ancestor topology")
		}
		if stat.Uid == owner {
			owned = true
		} else if stat.Uid != 0 || owned {
			t.Skip("positive public TreeStage path requires the approved structural-root and supplied-owner topology")
		}
	}
	parent, err := safefs.OpenRoot(root, owner)
	if err != nil {
		t.Fatal("admissible disposable public parent refused")
	}
	t.Cleanup(func() {
		if err := parent.Close(); err != nil {
			t.Error("public parent close failed")
		}
	})
	return root, parent
}

func TestTreeStagePublicLifecycle(t *testing.T) {
	root, parent := treePublicParent(t)
	data := []byte("synthetic complete tree bytes\x00")
	stage, err := parent.StageTree("building", 0o700)
	if err != nil || stage == nil || stage.Root() == nil {
		t.Fatal("admitted staging failed")
	}
	t.Cleanup(func() {
		if err := stage.Cleanup(); err != nil {
			t.Error("tree capability cleanup refused")
		}
	})
	borrowed := stage.Root()
	if stage.Root() != borrowed {
		t.Fatal("Root minted a new view")
	}
	initial := borrowed.Identity()
	if _, err := borrowed.PublishNoReplace("payload", data, 0o600); err != nil {
		t.Fatal("staged file publication failed")
	}
	read, identity, err := borrowed.ReadFile("payload", int64(len(data)))
	if err != nil || !bytes.Equal(read, data) || len(read) != cap(read) || identity.Size != int64(len(data)) {
		t.Fatal("observed staged read failed")
	}
	child, err := borrowed.CreateDir("nested")
	if err != nil {
		t.Fatal("staged directory creation failed")
	}
	t.Cleanup(func() {
		if err := child.Close(); err != nil {
			t.Error("derived child close failed")
		}
	})
	if _, err := child.PublishNoReplace("empty", nil, 0o600); err != nil {
		t.Fatal("nested empty file failed")
	}
	if err := borrowed.Close(); err != nil || stage.Root() != nil {
		t.Fatal("borrowed root close did not affect only its view")
	}
	if err := parent.Close(); err != nil {
		t.Fatal("caller parent close failed")
	}
	copy := *stage
	id, err := stage.PublishNoReplace("installed")
	if err != nil || id.Inode != initial.Inode || id.Device != initial.Device || id.Owner != initial.Owner || id.Mode != initial.Mode || id.MountID != initial.MountID {
		t.Fatal("independently pinned tree promotion failed")
	}
	installed, err := os.Lstat(filepath.Join(root, "installed"))
	if err != nil {
		t.Fatal("promoted root observation failed")
	}
	installedStat, ok := installed.Sys().(*syscall.Stat_t)
	if !ok || id.Links != uint64(installedStat.Nlink) || id.Size != installed.Size() || id.Mode != installedStat.Mode {
		t.Fatal("promotion returned stale root metadata")
	}
	if id, err := copy.PublishNoReplace("second"); err == nil || id != (safefs.Identity{}) {
		t.Fatal("consumed tree copy republished")
	}
	if names, err := child.Names(10); err == nil || names != nil {
		t.Fatal("retained descendant remained live after promotion")
	}
	if err := copy.Cleanup(); err != nil {
		t.Fatal("published cleanup was not idempotent")
	}
	got, err := os.ReadFile(filepath.Join(root, "installed", "payload"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("published content was removed or changed")
	}
	entries, err := os.ReadDir(filepath.Join(root, "installed"))
	if err != nil || len(entries) != 2 || entries[0].Name() != "nested" || entries[1].Name() != "payload" {
		t.Fatal("promoted complete inventory changed")
	}
	nested, err := os.Lstat(filepath.Join(root, "installed", "nested"))
	if err != nil || !nested.IsDir() || nested.Mode().Perm() != 0o700 {
		t.Fatal("promoted directory metadata changed")
	}
	entries, err = os.ReadDir(filepath.Join(root, "installed", "nested"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "empty" {
		t.Fatal("promoted nested inventory changed")
	}
	empty, err := os.Lstat(filepath.Join(root, "installed", "nested", "empty"))
	if err != nil || !empty.Mode().IsRegular() || empty.Mode().Perm() != 0o600 || empty.Size() != 0 {
		t.Fatal("promoted empty-file metadata changed")
	}
	if _, err := os.Lstat(filepath.Join(root, "building")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("promotion left the original stage name")
	}

	parent, err = safefs.OpenRoot(root, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal("public parent reopen failed")
	}
	t.Cleanup(func() {
		if err := parent.Close(); err != nil {
			t.Error("reopened parent close failed")
		}
	})
	if _, err := parent.PublishNoReplace("outside-sentinel", data, 0o600); err != nil {
		t.Fatal("disposable outside sentinel setup failed")
	}
	sentinelBefore, err := os.Lstat(filepath.Join(root, "outside-sentinel"))
	if err != nil {
		t.Fatal("outside sentinel observation failed")
	}
	abandoned, err := parent.StageTree("abandoned", 0o700)
	if err != nil || abandoned == nil {
		t.Fatal("cleanup stage construction failed")
	}
	t.Cleanup(func() {
		if err := abandoned.Cleanup(); err != nil {
			t.Error("abandoned capability cleanup refused")
		}
	})
	abandonedRoot := abandoned.Root()
	if _, err := abandonedRoot.PublishNoReplace("payload", data, 0o600); err != nil {
		t.Fatal("cleanup payload setup failed")
	}
	if err := abandoned.Cleanup(); err != nil || abandoned.Root() != nil || abandonedRoot.Identity() != (safefs.Identity{}) {
		t.Fatal("active cleanup failed to retire its own root")
	}
	if err := abandoned.Cleanup(); err != nil {
		t.Fatal("removed tree cleanup repeated filesystem work")
	}
	if _, err := os.Lstat(filepath.Join(root, "abandoned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("active cleanup left its stage")
	}
	got, err = os.ReadFile(filepath.Join(root, "outside-sentinel"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("cleanup changed the outside sentinel")
	}
	sentinelAfter, err := os.Lstat(filepath.Join(root, "outside-sentinel"))
	if err != nil {
		t.Fatal("outside sentinel reobservation failed")
	}
	before, beforeOK := sentinelBefore.Sys().(*syscall.Stat_t)
	after, afterOK := sentinelAfter.Sys().(*syscall.Stat_t)
	if !beforeOK || !afterOK || before.Dev != after.Dev || before.Ino != after.Ino || before.Uid != after.Uid || before.Mode != after.Mode || before.Nlink != after.Nlink || before.Size != after.Size {
		t.Fatal("cleanup changed outside-sentinel identity or metadata")
	}
}

func TestTreeStagePublicConcurrentPublication(t *testing.T) {
	root, parent := treePublicParent(t)
	for _, closeView := range []bool{false, true} {
		t.Run(strconv.FormatBool(closeView), func(t *testing.T) {
			suffix := strconv.FormatBool(closeView)
			stage, err := parent.StageTree("stage-"+suffix, 0o700)
			if err != nil {
				t.Fatal("concurrent fixture stage failed")
			}
			t.Cleanup(func() {
				if err := stage.Cleanup(); err != nil {
					t.Error("concurrent fixture cleanup failed")
				}
			})
			view := stage.Root()
			start := make(chan struct{})
			written := make(chan string, 32)
			var group sync.WaitGroup
			data := []byte("synthetic concurrent complete bytes")
			for i := range 32 {
				group.Add(1)
				go func() {
					defer group.Done()
					<-start
					name := "file-" + strconv.Itoa(i)
					if _, err := view.PublishNoReplace(name, data, 0o600); err == nil {
						written <- name
					}
				}()
			}
			closed := make(chan error, 1)
			if closeView {
				go func() { <-start; closed <- view.Close() }()
			}
			close(start)
			id, err := stage.PublishNoReplace("installed-" + suffix)
			group.Wait()
			close(written)
			if closeView {
				if err := <-closed; err != nil {
					t.Fatal("concurrent borrowed-view close failed")
				}
			}
			if err != nil || id == (safefs.Identity{}) {
				t.Fatal("admitted competing publication failed")
			}
			var names []string
			for name := range written {
				names = append(names, name)
			}
			slices.Sort(names)
			entries, err := os.ReadDir(filepath.Join(root, "installed-"+suffix))
			if err != nil || len(entries) != len(names) {
				t.Fatal("publication lost or exposed an unreported write")
			}
			for i, entry := range entries {
				got, err := os.ReadFile(filepath.Join(root, "installed-"+suffix, entry.Name()))
				if entry.Name() != names[i] || err != nil || !bytes.Equal(got, data) {
					t.Fatal("concurrent publication exposed incomplete content")
				}
			}
			if got, err := view.PublishNoReplace("late", data, 0o600); err == nil || got != (safefs.Identity{}) {
				t.Fatal("revoked view mutated installed content")
			}
			if _, err := os.Lstat(filepath.Join(root, "installed-"+suffix, "late")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("revoked operation created a late entry")
			}
		})
	}
}
