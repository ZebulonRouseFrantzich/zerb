//go:build linux

package safefs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTreeConstructorCollisionAndIndependentPinsCore(t *testing.T) {
	base, parent := localFixture(t)
	if err := os.WriteFile(filepath.Join(base, "file"), []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(base, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file", "directory", "link", "fifo"} {
		before := adversarialFDCount(t)
		if state, err := createTreeAt([]edge{parent}, parent.id.Owner, name); state != nil || !errors.Is(err, unix.EEXIST) {
			t.Fatal("collision adopted or lost errno")
		}
		if adversarialFDCount(t) != before {
			t.Fatal("collision leaked descriptor")
		}
	}
	input, err := duplicateTreeChain([]edge{parent}, 0)
	if err != nil {
		t.Fatal(err)
	}
	s, err := createTreeAt(input, parent.id.Owner, "stage")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.dispose(); err != nil {
			t.Error(err)
		}
	}()
	if err := closeChain(input); err != nil {
		t.Fatal(err)
	}
	borrowed := s.lent
	stage := &TreeStage{state: s}
	copied := *stage
	if borrowed != stage.Root() || borrowed != copied.Root() {
		t.Fatal("Root minted a view")
	}
	if err := borrowed.Close(); err != nil {
		t.Fatal(err)
	}
	if (&TreeStage{state: s}).Root() != nil || borrowed.Identity() != (Identity{}) {
		t.Fatal("closed borrowed view retained authority")
	}
	if _, err := observeFD(s.parent[0].fd); err != nil {
		t.Fatal("supplied parent close revoked private pins")
	}
	if _, err := observeFD(s.root.fd); err != nil {
		t.Fatal("borrowed root close revoked private root")
	}
	for _, e := range append(s.parent[:len(s.parent):len(s.parent)], s.root) {
		flags, err := unix.FcntlInt(uintptr(e.fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("stage descriptor lacks CLOEXEC")
		}
	}
}

func TestTreeConstructorRestrictiveUmaskCore(t *testing.T) {
	if os.Getenv("ZERB_TREE_UMASK_HELPER") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTreeConstructorRestrictiveUmaskCore$")
		cmd.Env = append(os.Environ(), "ZERB_TREE_UMASK_HELPER=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated tree mode test failed: %s", output)
		}
		return
	}
	base, parent := localFixture(t)
	before := adversarialFDCount(t)
	old := unix.Umask(0o777)
	state, err := createTreeAt([]edge{parent}, parent.id.Owner, "restricted")
	unix.Umask(old)
	if state != nil || err == nil {
		t.Fatal("restrictive umask admitted a stage")
	}
	info, err := os.Lstat(filepath.Join(base, "restricted"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0 {
		t.Fatal("uncertain creation residue removed or repaired")
	}
	if adversarialFDCount(t) != before {
		t.Fatal("failed constructor leaked descriptors")
	}
}

func TestTreePromotionCollisionAndPostRenameUncertaintyCore(t *testing.T) {
	for _, kind := range []string{"directory", "file", "symlink", "post-rename"} {
		t.Run(kind, func(t *testing.T) {
			base, s, boundary, _ := localTreeCore(t)
			target := filepath.Join(base, "published")
			switch kind {
			case "directory":
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("outside", target); err != nil {
					t.Fatal(err)
				}
			}
			var original Identity
			if kind != "post-rename" {
				var err error
				original, err = observeAt(s.parent[0].fd, "published")
				if err != nil {
					t.Fatal(err)
				}
			}
			checked := func() error {
				if s.root.name == "published" && kind == "post-rename" {
					return errDirectory
				}
				return boundary()
			}
			id, attempted, err := s.promoteWithBoundary("published", checked)
			if id != (Identity{}) || !attempted || !errors.Is(err, errTreeUncertain) {
				t.Fatal("post-attempt error claimed identity/success")
			}
			if kind != "post-rename" {
				after, err := observeAt(s.parent[0].fd, "published")
				if err != nil || after != original {
					t.Fatal("destination collision was changed")
				}
				if _, err := os.Lstat(filepath.Join(base, "stage")); err != nil {
					t.Fatal("collision deleted stage")
				}
			} else if _, err := os.Lstat(target); err != nil {
				t.Fatal("uncertain promoted name not preserved")
			}
		})
	}
}

func TestTreePrevalidationAndPartialCleanupPreserveCore(t *testing.T) {
	for _, kind := range []string{"preflight", "new-entry", "changed-entry", "partial"} {
		t.Run(kind, func(t *testing.T) {
			base, s, boundary, parentBoundary := localTreeCore(t)
			root := filepath.Join(base, "stage")
			if err := os.Mkdir(filepath.Join(root, "a"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "b"), 0o700); err != nil {
				t.Fatal(err)
			}
			if kind == "preflight" {
				if err := os.Symlink("../outside", filepath.Join(root, "bad")); err != nil {
					t.Fatal(err)
				}
				if _, err := s.walkWithBoundary(0, false, boundary); err == nil {
					t.Fatal("unsafe prevalidation passed")
				}
				if _, err := os.Lstat(filepath.Join(root, "a")); err != nil {
					t.Fatal("prevalidation removed known entry")
				}
				return
			}
			inventory, err := s.walkWithBoundary(0, false, boundary)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "new-entry" {
				if err := os.Mkdir(filepath.Join(root, "extra"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "changed-entry" {
				if err := os.Rename(filepath.Join(root, "a"), filepath.Join(base, "old-a")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(root, "a"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			checked := func() error {
				if kind == "partial" {
					if _, err := os.Lstat(filepath.Join(root, "a")); errors.Is(err, os.ErrNotExist) {
						return errDirectory
					}
				}
				return boundary()
			}
			if err := s.removeWithBoundary(inventory, checked, parentBoundary); err == nil {
				t.Fatal("changed cleanup claimed completion")
			}
			if _, err := os.Lstat(root); err != nil {
				t.Fatal("uncertain cleanup removed container")
			}
			if kind == "partial" {
				if _, err := os.Lstat(filepath.Join(root, "b")); err != nil {
					t.Fatal("partial cleanup continued after failure")
				}
			}
		})
	}
}

func TestTreeTerminalStatesAndParameterRefusals(t *testing.T) {
	for _, phase := range []treePhase{treePublished, treeRemoved, treePreserve} {
		s := treeGateFixture()
		s.phase = phase
		stage := &TreeStage{state: s}
		copyStage := *stage
		if stage.Root() != nil {
			t.Fatal("consumed stage lent root")
		}
		if id, err := copyStage.PublishNoReplace("target"); id != (Identity{}) || err == nil {
			t.Fatal("consumed publication admitted")
		}
		err := stage.Cleanup()
		if (err == nil) != (phase != treePreserve) {
			t.Fatal("terminal cleanup did not preserve status")
		}
		if phase != treePreserve && stage.Cleanup() != nil {
			t.Fatal("terminal cleanup not idempotent")
		}
	}
	s := treeGateFixture()
	s.root.name = "stage"
	stage := &TreeStage{state: s}
	for _, name := range []string{"", ".", "..", "stage", "a/b", strings.Repeat("n", 256)} {
		if id, err := stage.PublishNoReplace(name); id != (Identity{}) || err == nil || s.phase != treeActive {
			t.Fatal("argument-only refusal consumed stage")
		}
	}
}

func TestTreeDupAndFinalCloseFailures(t *testing.T) {
	base, parent := localFixture(t)
	before := adversarialFDCount(t)
	if chain, err := duplicateTreeChain([]edge{parent, {fd: -1}}, 0); chain != nil || err == nil {
		t.Fatal("failed duplicate returned partial ownership")
	}
	if adversarialFDCount(t) != before {
		t.Fatal("partial duplication leaked")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("failed duplication closed caller parent")
	}
	s, err := createTreeAt([]edge{parent}, parent.id.Owner, "stage")
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(s.root.fd); err != nil {
		t.Fatal(err)
	}
	// No intervening open may recycle the fault-injected descriptor number.
	if err := s.terminalFinish(treePublished, nil); err == nil || s.phase != treePreserve {
		t.Fatal("final close failure survived as success")
	}
	if _, err := os.Lstat(filepath.Join(base, "stage")); err != nil {
		t.Fatal("failed cleanup removed possible residue")
	}
	if err := (&TreeStage{state: s}).Cleanup(); err == nil {
		t.Fatal("close uncertainty recovered authority")
	}
}
