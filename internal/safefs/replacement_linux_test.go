//go:build linux

package safefs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// ARCH-020/030, VAL-027: private fixtures do not admit an absolute root.
func replacementFixture(t *testing.T, data []byte, mode uint32) (string, edge, *replacementState) {
	t.Helper()
	root, parent := localFixture(t)
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("complete predecessor\x00bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := observeAt(parent.fd, "target")
	if err != nil {
		t.Fatal(err)
	}
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	s, err := replacementAt(chain, "target", ".temporary", data, mode, previous)
	if err != nil {
		t.Fatal("cannot stage disposable replacement")
	}
	t.Cleanup(func() {
		if err := s.closeResources(); err != nil {
			t.Error("replacement cleanup failed")
		}
	})
	return root, parent, s
}

func requireReplacementError(t *testing.T, id Identity, err error, uncertain bool) {
	t.Helper()
	want := errDirectory
	if uncertain {
		want = errPublicationUncertain
	}
	if id != (Identity{}) || !errors.Is(err, want) {
		t.Fatal("replacement did not return zero identity and fixed error")
	}
}

func TestReplacementCompleteStagingAndTerminalPublication(t *testing.T) {
	for _, mode := range []uint32{0, 0o400, 0o600, 0o700} {
		for _, data := range [][]byte{nil, []byte("complete\x00successor"), bytes.Repeat([]byte{0, 0x91, 0x7f}, 24000)} {
			t.Run(fmt.Sprintf("%o/%d", mode, len(data)), func(t *testing.T) {
				root, parent, s := replacementFixture(t, data, mode)
				facts := s.facts
				want := sha256.Sum256(data)
				if facts.Staged.SHA256 != hex.EncodeToString(want[:]) || facts.Staged.Identity.Links != 0 ||
					facts.Staged.Identity.Mode != unix.S_IFREG|mode || facts.Previous.Identity.Links != 1 ||
					facts.Staged.Identity.Inode == facts.Previous.Identity.Inode || s.validateContents() != nil {
					t.Fatal("staging did not capture complete immutable facts")
				}
				if _, err := os.Stat(filepath.Join(root, ".temporary")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("anonymous stage exposed a temporary name")
				}
				original := append([]byte(nil), data...)
				for i := range data {
					data[i] ^= 0xff
				}
				if err := s.link(); err != nil {
					t.Fatal("complete no-clobber link failed")
				}
				if s.facts != facts || s.stage.stamp.identity.Links != 1 || s.stage.pin == nil {
					t.Fatal("link changed construction facts or skipped verification")
				}
				verifyFD, err := unix.FcntlInt(uintptr(s.stage.fd), unix.F_DUPFD_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				id, err := s.publish()
				if err != nil || id.Inode != facts.Staged.Identity.Inode || id.Links != 1 || !s.closed {
					t.Fatal("terminal publication failed")
				}
				got := make([]byte, len(original)+1)
				n, readErr := unix.Pread(verifyFD, got, 0)
				closeErr := unix.Close(verifyFD)
				if readErr != nil || closeErr != nil || n != len(original) || !bytes.Equal(got[:n], original) {
					t.Fatal("publication retained caller buffer or changed complete bytes")
				}
				stamp, err := observeFile(parent.fd, "target")
				if err != nil || stamp.identity != id || stamp.modified.Sec != facts.Staged.ModifiedSeconds ||
					stamp.modified.Nsec != facts.Staged.ModifiedNanoseconds {
					t.Fatal("publication changed unexpected metadata")
				}
				if _, err := observeAt(parent.fd, ".temporary"); !errors.Is(err, unix.ENOENT) {
					t.Fatal("rename left temporary entry")
				}
				if s.facts != (ReplacementFacts{}) || len(s.chain) != 0 || s.reader != nil {
					t.Fatal("terminal publication retained facts or descriptors")
				}
				id, err = s.publish()
				requireReplacementError(t, id, err, true)
			})
		}
	}
}

func TestReplacementFullPredecessorIdentityBeforeContent(t *testing.T) {
	for i, edit := range []func(*Identity){
		func(id *Identity) { *id = Identity{} }, func(id *Identity) { id.Device++ },
		func(id *Identity) { id.Inode++ }, func(id *Identity) { id.Owner++ },
		func(id *Identity) { id.Mode ^= 0o100 }, func(id *Identity) { id.Links++ },
		func(id *Identity) { id.Size++ }, func(id *Identity) { id.MountID++ },
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			root, parent := localFixture(t)
			if err := os.WriteFile(filepath.Join(root, "target"), []byte("predecessor"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := observeFile(parent.fd, "target")
			if err != nil {
				t.Fatal(err)
			}
			previous := before.identity
			edit(&previous)
			chain, err := duplicateChain([]edge{parent})
			if err != nil {
				t.Fatal(err)
			}
			s, err := replacementAt(chain, "target", ".temporary", []byte("successor"), 0o600, previous)
			if s != nil || !errors.Is(err, errDirectory) {
				t.Fatal("mismatched predecessor identity admitted")
			}
			after, err := observeFile(parent.fd, "target")
			entries, listing := os.ReadDir(root)
			if err != nil || after != before || listing != nil || len(entries) != 1 {
				t.Fatal("rejected staging changed namespace or predecessor")
			}
		})
	}
}

func TestReplacementRejectsUnsafePredecessorsBeforeContent(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "fifo", "directory", "hardlink", "group-write"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			name := filepath.Join(root, "target")
			var err error
			switch kind {
			case "missing":
			case "symlink":
				err = os.Symlink("outside-sentinel", name)
			case "fifo":
				// No writer exists: an accidental readable FIFO open would block.
				err = unix.Mkfifo(name, 0o600)
			case "directory":
				err = os.Mkdir(name, 0o700)
			default:
				err = os.WriteFile(name, []byte("complete prior"), 0o600)
				if err == nil && kind == "hardlink" {
					err = os.Link(name, filepath.Join(root, "alias"))
				}
				if err == nil && kind == "group-write" {
					err = os.Chmod(name, 0o620)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			prior, observation := observeAt(parent.fd, "target")
			if kind == "missing" {
				if !errors.Is(observation, unix.ENOENT) {
					t.Fatal("missing observation was ambiguous")
				}
				prior = Identity{Inode: 1, Size: 1}
			} else if observation != nil {
				t.Fatal(observation)
			}
			chain, err := duplicateChain([]edge{parent})
			if err != nil {
				t.Fatal(err)
			}
			s, err := replacementAt(chain, "target", ".temporary", []byte("successor"), 0o600, prior)
			if s != nil || !errors.Is(err, errDirectory) {
				t.Fatal("unsafe predecessor acquired content or replacement authority")
			}
			if _, err := observeAt(parent.fd, ".temporary"); !errors.Is(err, unix.ENOENT) {
				t.Fatal("unsafe predecessor created temporary namespace entry")
			}
		})
	}
}

func TestReplacementCollisionAndAbandonNeverUnlink(t *testing.T) {
	for _, kind := range []string{"regular", "directory", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
			name := filepath.Join(root, ".temporary")
			var err error
			switch kind {
			case "regular":
				err = os.WriteFile(name, []byte("collision preserved"), 0o600)
			case "directory":
				err = os.Mkdir(name, 0o700)
			case "symlink":
				err = os.Symlink("outside-sentinel", name)
			case "fifo":
				err = unix.Mkfifo(name, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := observeFile(parent.fd, ".temporary")
			if err != nil {
				t.Fatal(err)
			}
			requirePublicationPhaseRefusal(t, s.link())
			if !s.closed || !s.stage.attempted || s.facts != (ReplacementFacts{}) {
				t.Fatal("failed link retained live capability")
			}
			after, err := observeFile(parent.fd, ".temporary")
			if err != nil || after != before {
				t.Fatal("collision was adopted or removed")
			}
		})
	}
	for _, linked := range []bool{false, true} {
		root, _, s := replacementFixture(t, []byte("complete successor"), 0o600)
		if linked {
			if err := s.link(); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.closeResources(); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		want := 1
		if linked {
			want++
		}
		if err != nil || len(entries) != want {
			t.Fatal("abandon removed an entry or named an anonymous inode")
		}
	}
}

func TestReplacementBothRenameOperandsRevalidated(t *testing.T) {
	for _, operand := range []string{"target", ".temporary"} {
		for _, kind := range []string{"replacement", "symlink", "hardlink", "mode", "bytes", "unlink"} {
			t.Run(operand+"/"+kind, func(t *testing.T) {
				root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
				if err := s.link(); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, operand)
				var err error
				switch kind {
				case "replacement", "symlink":
					if err = os.Rename(path, filepath.Join(root, "moved")); err == nil {
						if kind == "replacement" {
							err = os.WriteFile(path, []byte("foreign object"), 0o600)
						} else {
							err = os.Symlink("moved", path)
						}
					}
				case "hardlink":
					err = os.Link(path, filepath.Join(root, "alias"))
				case "mode":
					err = os.Chmod(path, 0o400)
				case "bytes":
					err = os.WriteFile(path, []byte("modified bytes"), 0o600)
				case "unlink":
					err = os.Remove(path)
				}
				if err != nil {
					t.Fatal(err)
				}
				before, beforeErr := observeAt(parent.fd, operand)
				id, err := s.publish()
				requireReplacementError(t, id, err, false)
				if s.renameAttempted || !s.closed {
					t.Fatal("observed operand change reached rename")
				}
				after, afterErr := observeAt(parent.fd, operand)
				if before != after || !errors.Is(beforeErr, afterErr) {
					t.Fatal("refusal changed observed operand")
				}
			})
		}
	}
}

func TestReplacementCompleteHashesAndPriorAfterRename(t *testing.T) {
	for _, operand := range []string{"previous", "staged"} {
		_, _, s := replacementFixture(t, []byte("complete successor"), 0o600)
		if operand == "previous" {
			s.facts.Previous.SHA256 = strings.Repeat("0", 64)
		} else {
			s.facts.Staged.SHA256 = strings.Repeat("0", 64)
		}
		requirePublicationPhaseRefusal(t, s.link())
		if s.stage.attempted {
			t.Fatal("hash mismatch reached link")
		}
	}
	root, _, s := replacementFixture(t, []byte("complete successor"), 0o600)
	if err := s.link(); err != nil {
		t.Fatal(err)
	}
	oldReader := int(s.reader.Fd())
	if err := s.rename(); err != nil {
		t.Fatal(err)
	}
	if s.previous.before.identity.Links != 0 || int(s.reader.Fd()) != oldReader || s.validateContents() != nil {
		t.Fatal("rename did not retain a verified zero-link predecessor reader")
	}
	// A still-owned writer reaches the unlinked predecessor; its mutation refuses.
	fd, err := unix.Openat(s.proc.chain[3].fd, fmt.Sprint(s.previous.fd), unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := unix.Pwrite(fd, []byte("changed"), 0)
	closeErr := unix.Close(fd)
	if writeErr != nil || closeErr != nil {
		t.Fatal("cannot mutate disposable unlinked predecessor")
	}
	id, err := s.finish(Identity{}, s.validateContents())
	requireReplacementError(t, id, err, true)
	got, err := os.ReadFile(filepath.Join(root, "target"))
	if err != nil || string(got) != "complete successor" {
		t.Fatal("post-rename refusal removed successor")
	}
}

// ARCH-020: observed checks detect displacement; ordinary rename is not CAS.
func TestReplacementRenameBoundaryErrorsAndBothOperandRaces(t *testing.T) {
	for _, race := range []string{"missing-source", "source", "target"} {
		t.Run(race, func(t *testing.T) {
			root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
			if err := s.link(); err != nil {
				t.Fatal(err)
			}
			if err := s.validateContents(); err != nil {
				t.Fatal(err)
			}
			operand := ".temporary"
			if race == "target" {
				operand = "target"
			}
			if race == "missing-source" {
				if err := os.Remove(filepath.Join(root, operand)); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(filepath.Join(root, operand), filepath.Join(root, "moved")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, operand), []byte("unexpected inode"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := observeAt(parent.fd, "target")
			if err != nil {
				t.Fatal(err)
			}
			// This is the real last-check/syscall interval, with no syscall hooks.
			renameErr := s.renameAttempt()
			id, err := s.finish(Identity{}, renameErr)
			requireReplacementError(t, id, err, true)
			if renameErr == nil || !s.renameAttempted || !s.closed {
				t.Fatal("raced rename returned success or retained authority")
			}
			after, err := observeAt(parent.fd, "target")
			if err != nil {
				t.Fatal(err)
			}
			if race == "missing-source" {
				if after != before {
					t.Fatal("failed ENOENT rename changed predecessor")
				}
			} else if after.Inode == before.Inode {
				t.Fatal("test failed to demonstrate actual ordinary-rename displacement")
			}
			got, err := os.ReadFile(filepath.Join(root, "target"))
			want := "complete predecessor\x00bytes"
			switch race {
			case "source":
				want = "unexpected inode"
			case "target":
				want = "complete successor"
			}
			if err != nil || string(got) != want {
				t.Fatal("uncertain error claimed restoration or changed syscall outcome")
			}
		})
	}
}

func TestReplacementProcAndAncestorSubstitutionRefuse(t *testing.T) {
	for _, kind := range []string{"proc-filesystem", "proc-self", "ancestor", "parent-mode"} {
		t.Run(kind, func(t *testing.T) {
			root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
			_, foreign := localFixture(t)
			switch kind {
			case "proc-filesystem":
				original := s.proc.chain[1]
				s.proc.chain[1] = foreign
				err := s.validateContents()
				s.proc.chain[1] = original
				requirePublicationPhaseRefusal(t, err)
			case "proc-self":
				original := s.proc.self
				s.proc.self = foreign
				err := s.validateContents()
				s.proc.self = original
				requirePublicationPhaseRefusal(t, err)
			case "ancestor":
				oldFD := s.chain[0].fd
				s.chain[0].fd = foreign.fd
				err := s.validateContents()
				s.chain[0].fd = oldFD
				requirePublicationPhaseRefusal(t, err)
			case "parent-mode":
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal(err)
				}
				err := s.validateContents()
				if err := os.Chmod(root, 0o700); err != nil {
					t.Fatal(err)
				}
				requirePublicationPhaseRefusal(t, err)
			}
			if err := s.closeResources(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(root)
			prior, observation := observeAt(parent.fd, "target")
			if err != nil || len(entries) != 1 || observation != nil || prior.Links != 1 {
				t.Fatal("refused facility/substitution check changed namespace")
			}
		})
	}
}

func TestReplacementParentSyncAndCloseErrorsAreTerminal(t *testing.T) {
	root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
	if err := s.link(); err != nil {
		t.Fatal(err)
	}
	if err := s.rename(); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	stageParent := s.stage.parent
	s.stage.parent.fd = fd
	syncErr := s.stage.syncParent() // Real EBADF: O_PATH cannot be fsynced.
	s.stage.parent = stageParent
	closeErr := unix.Close(fd)
	if syncErr == nil || closeErr != nil {
		t.Fatal("did not exercise actual parent-sync failure")
	}
	id, err := s.finish(Identity{}, syncErr)
	requireReplacementError(t, id, err, true)
	got, err := os.ReadFile(filepath.Join(root, "target"))
	if err != nil || string(got) != "complete successor" {
		t.Fatal("uncertain publication lost complete successor")
	}

	_, parent, s = replacementFixture(t, []byte("complete successor"), 0o600)
	if err := s.link(); err != nil {
		t.Fatal(err)
	}
	id, err = s.publishPhases()
	if err != nil {
		t.Fatal(err)
	}
	broken := s.previous.fd
	if err := unix.Close(broken); err != nil {
		t.Fatal(err)
	}
	// Mark the intentionally closed slot so validation cannot open/reuse it.
	s.previous.fd = -1
	id, err = s.finish(id, nil)
	requireReplacementError(t, id, err, true)
	if !s.closed || len(s.chain) != 0 {
		t.Fatal("cleanup failure retained ownership")
	}
	if err := s.closeResources(); err != nil {
		t.Fatal("failed cleanup was retried")
	}
	if err := unix.Dup3(parent.fd, broken, unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	if err := s.closeResources(); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.FcntlInt(uintptr(broken), unix.F_GETFD, 0); err != nil {
		t.Fatal("repeated cleanup closed a reused descriptor number")
	}
	if err := unix.Close(broken); err != nil {
		t.Fatal(err)
	}
}

func TestReplacementOneShotAndPublishBeforeLink(t *testing.T) {
	for _, linked := range []bool{false, true} {
		root, _, s := replacementFixture(t, []byte("complete successor"), 0o600)
		if linked {
			if err := s.link(); err != nil {
				t.Fatal(err)
			}
			requirePublicationPhaseRefusal(t, s.link())
		} else {
			id, err := s.publish()
			requireReplacementError(t, id, err, false)
		}
		if !s.closed || s.facts != (ReplacementFacts{}) || len(s.chain) != 0 {
			t.Fatal("failed one-shot action retained owned resources or facts")
		}
		got, err := os.ReadFile(filepath.Join(root, "target"))
		if err != nil || string(got) != "complete predecessor\x00bytes" {
			t.Fatal("pre-rename refusal changed predecessor")
		}
	}
}

func TestReplacementCopiesSerializeCloseAndInvalidateFacts(t *testing.T) {
	root, _, s := replacementFixture(t, []byte("complete successor"), 0o600)
	r := Replacement{state: s}
	copy := r
	facts := r.Facts()
	facts.Target = "changed"
	facts.Previous.Identity.Inode++
	if r.Facts() != s.facts || copy.Facts() != s.facts {
		t.Fatal("Facts exposed mutable capability storage")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = copy.Facts()
			// Local fixture intentionally lacks public root admission; Close still
			// consumes its descriptors, and only that first refusal may be nonnil.
			err := copy.Close()
			if err != nil && !errors.Is(err, errDirectory) {
				t.Error("unexpected concurrent Close error")
			}
		}()
	}
	wg.Wait()
	if r.Facts() != (ReplacementFacts{}) || copy.Facts() != (ReplacementFacts{}) || len(s.chain) != 0 {
		t.Fatal("copied capability retained facts or descriptors after Close")
	}
	if err := r.Close(); err != nil {
		t.Fatal("Close was not idempotent")
	}
	got, err := os.ReadFile(filepath.Join(root, "target"))
	if err != nil || string(got) != "complete predecessor\x00bytes" {
		t.Fatal("Close changed predecessor")
	}
}

func TestReplacementInvalidAndPublicBoundary(t *testing.T) {
	for _, r := range []*Replacement{nil, {}, {state: &replacementState{closed: true}}} {
		if r.Facts() != (ReplacementFacts{}) {
			t.Fatal("invalid Facts were nonzero")
		}
		requirePublicationPhaseRefusal(t, r.Link())
		id, err := r.Publish()
		requireReplacementError(t, id, err, false)
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	d, err := OpenRoot(root, uint32(os.Geteuid()))
	if err != nil {
		t.Skip("positive StageReplacement/Link/Publish/Close requires an absolute topology admitted by the approved owner/mode policy")
	}
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("prior"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, previous, err := d.ReadFile("target", 5)
	if err != nil {
		t.Fatal(err)
	}
	r, err := d.StageReplacement("target", ".temporary", []byte("next"), 0o600, previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Link(); err != nil {
		t.Fatal("replacement borrowed caller directory lifetime")
	}
	if _, err := r.Publish(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
