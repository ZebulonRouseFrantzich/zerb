//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// ARCH-020/030, VAL-027: local phase checks do not admit an absolute root.
func localPublication(parent edge, name string, data []byte, mode uint32) (id Identity, err error) {
	if validateEdges([]edge{parent}, parent.id.Owner) != nil {
		return Identity{}, errDirectory
	}
	p, err := openPublication(parent, mode)
	if err != nil {
		return Identity{}, errDirectory
	}
	defer func() {
		before := validateEdges([]edge{parent}, parent.id.Owner)
		if p.linked {
			before = errors.Join(before, p.validate())
		}
		closeErr := p.close()
		after := validateEdges([]edge{parent}, parent.id.Owner)
		if p.linked {
			after = errors.Join(after, p.validateLinked())
		}
		if err != nil || before != nil || closeErr != nil || after != nil {
			id, err = Identity{}, errDirectory
			if p.attempted {
				err = errPublicationUncertain
			}
		}
	}()
	for _, phase := range []func() error{
		func() error { return p.write(data) }, p.syncFile,
		func() error { return p.link(name) }, p.syncParent,
		func() error { return p.verify(name, data) },
	} {
		if validateEdges([]edge{parent}, parent.id.Owner) != nil {
			return Identity{}, errDirectory
		}
		phaseErr := phase()
		if validateEdges([]edge{parent}, parent.id.Owner) != nil || phaseErr != nil {
			return Identity{}, errDirectory
		}
	}
	return p.stamp.identity, nil
}

func publicationFixture(t *testing.T, parent edge, data []byte, through int) *publication {
	t.Helper()
	p, err := openPublication(parent, 0o600)
	if err != nil {
		t.Fatal("cannot open disposable anonymous publication")
	}
	t.Cleanup(func() {
		if err := p.close(); err != nil {
			t.Error("publication cleanup failed")
		}
	})
	phases := []func() error{func() error { return p.write(data) }, p.syncFile,
		func() error { return p.link("entry") }, p.syncParent,
		func() error { return p.verify("entry", data) }}
	for _, phase := range phases[:through] {
		if validateEdges([]edge{parent}, parent.id.Owner) != nil {
			t.Fatal("local publication parent changed")
		}
		if err := phase(); err != nil {
			t.Fatal("publication fixture phase failed")
		}
		if validateEdges([]edge{parent}, parent.id.Owner) != nil {
			t.Fatal("local publication parent changed")
		}
	}
	return p
}

func requirePublicationRefusal(t *testing.T, id Identity, err error, uncertain bool) {
	t.Helper()
	want := errDirectory
	if uncertain {
		want = errPublicationUncertain
	}
	if id != (Identity{}) || err == nil || err.Error() != want.Error() {
		t.Fatal("publication did not clear identity and return its fixed refusal")
	}
}

func requirePublicationPhaseRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil || err.Error() != errDirectory.Error() {
		t.Fatal("publication phase did not return fixed refusal")
	}
}

func TestPublicationAnonymousAndExactModes(t *testing.T) {
	for _, mode := range []uint32{0, 0o400, 0o600, 0o700} {
		for _, data := range [][]byte{nil, []byte("complete\x00publication"), bytes.Repeat([]byte{0x91, 0x00, 0x7f}, 30000)} {
			t.Run(fmt.Sprintf("%o/%d", mode, len(data)), func(t *testing.T) {
				root, parent := localFixture(t)
				p, err := openPublication(parent, mode)
				if err != nil {
					t.Fatal("anonymous publication creation failed")
				}
				defer func() {
					if err := p.close(); err != nil {
						t.Error("publication close failed")
					}
				}()
				before, err := observeFile(p.fd, "")
				if err != nil || before.identity.Mode != unix.S_IFREG|mode || before.identity.Links != 0 || before.identity.Size != 0 || !sameMount(before.identity, parent.id) {
					t.Fatal("anonymous inode lacks the exact immediate metadata")
				}
				flags, err := unix.FcntlInt(uintptr(p.fd), unix.F_GETFL, 0)
				if err != nil || flags&unix.O_TMPFILE != unix.O_TMPFILE || flags&unix.O_ACCMODE != unix.O_RDWR || flags&(unix.O_APPEND|unix.O_TRUNC) != 0 {
					t.Fatal("publication descriptor lacks anonymous read/write flags")
				}
				flags, err = unix.FcntlInt(uintptr(p.fd), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					t.Fatal("publication descriptor is not close-on-exec")
				}
				if _, err := unix.Seek(p.fd, 17, 0); err != nil {
					t.Fatal(err)
				}
				if err := p.write(data); err != nil {
					t.Fatal("complete anonymous write failed")
				}
				if err := p.syncFile(); err != nil {
					t.Fatal("anonymous file synchronization failed")
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 0 {
					t.Fatal("publication exposed a name before atomic link")
				}
				if err := p.link("entry"); err != nil {
					t.Fatal("atomic no-replace link failed")
				}
				if err := p.syncParent(); err != nil {
					t.Fatal("publication parent synchronization failed")
				}
				if err := p.verify("entry", data); err != nil {
					t.Fatal("published exact-inode verification failed")
				}
				if offset, err := unix.Seek(p.fd, 0, 1); err != nil || offset != 17 {
					t.Fatal("positional write or verification changed descriptor offset")
				}
				after, err := observeFile(parent.fd, "entry")
				if err != nil || after != p.stamp || after.identity.Inode != before.identity.Inode || after.identity.Links != 1 || after.identity.Size != int64(len(data)) || after.identity.Mode != unix.S_IFREG|mode {
					t.Fatal("publication did not preserve exact inode, mode, size and single link")
				}
				got := make([]byte, len(data)+1)
				n, err := unix.Pread(p.fd, got, 0)
				if err != nil || n != len(data) || !bytes.Equal(got[:n], data) {
					t.Fatal("independent retained-descriptor read found incomplete bytes")
				}
				flags, err = unix.FcntlInt(uintptr(p.pin.fd), unix.F_GETFL, 0)
				if err != nil || flags&unix.O_PATH == 0 {
					t.Fatal("final verification pin grants content access")
				}
			})
		}
	}
}

func TestPublicationCollisionPreservesExistingObject(t *testing.T) {
	for _, kind := range []string{"regular", "symlink", "fifo", "socket", "directory", "hardlink", "group-write"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			name := filepath.Join(root, "entry")
			switch kind {
			case "symlink":
				if err := os.Symlink("outside-sentinel", name); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(name, 0o600); err != nil {
					t.Fatal(err)
				}
			case "socket":
				fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := unix.Close(fd); err != nil {
						t.Error(err)
					}
				}()
				if err := unix.Bind(fd, &unix.SockaddrUnix{Name: "/proc/self/fd/" + strconv.Itoa(parent.fd) + "/entry"}); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(name, 0o700); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(name, []byte("preserved existing bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
				if kind == "hardlink" {
					if err := os.Link(name, filepath.Join(root, "alias")); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "group-write" {
					if err := os.Chmod(name, 0o620); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := observeFile(parent.fd, "entry")
			if err != nil {
				t.Fatal(err)
			}
			id, err := localPublication(parent, "entry", []byte("must never replace"), 0o600)
			requirePublicationRefusal(t, id, err, true)
			after, err := observeFile(parent.fd, "entry")
			if err != nil || before != after {
				t.Fatal("collision changed existing object metadata")
			}
			if kind == "regular" || kind == "hardlink" || kind == "group-write" {
				data, err := os.ReadFile(name)
				if err != nil || string(data) != "preserved existing bytes" {
					t.Fatal("collision changed existing content")
				}
			}
			if kind == "symlink" {
				target, err := os.Readlink(name)
				if err != nil || target != "outside-sentinel" {
					t.Fatal("collision changed symlink")
				}
			}
		})
	}
}

func TestPublicationInvalidInputsAndClosedHandles(t *testing.T) {
	root, parent := localFixture(t)
	for _, mode := range []uint32{0o602, 0o620, 0o777, 0o1000, unix.S_ISUID | 0o600, unix.S_IFREG | 0o600, ^uint32(0)} {
		p, err := openPublication(parent, mode)
		if p != nil || err == nil || err.Error() != errDirectory.Error() {
			t.Fatal("unsafe publication mode admitted")
		}
	}
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	defer dir.Close()
	for _, d := range []*Dir{nil, {}, {state: &dirState{}}, dir} {
		for _, name := range []string{"", ".", "..", "/", "a/b", "private\x00marker", "entry"} {
			id, err := d.PublishNoReplace(name, []byte("private bytes"), 0o600)
			requirePublicationRefusal(t, id, err, false)
		}
	}
	if err := dir.Close(); err != nil {
		t.Fatal(err)
	}
	id, err := dir.PublishNoReplace("entry", nil, 0o600)
	requirePublicationRefusal(t, id, err, false)
	for _, name := range []string{"", ".", "..", "a/b", "private\x00marker"} {
		p := publicationFixture(t, parent, nil, 2)
		requirePublicationPhaseRefusal(t, p.link(name))
		if p.attempted || p.linked {
			t.Fatal("invalid component reached link boundary")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("refused publication performed named I/O")
	}
}

func TestPublicationRequiresPhaseOrder(t *testing.T) {
	_, parent := localFixture(t)
	p := publicationFixture(t, parent, nil, 0)
	for _, phase := range []func() error{p.syncFile, func() error { return p.link("entry") }, p.syncParent, func() error { return p.verify("entry", nil) }} {
		requirePublicationPhaseRefusal(t, phase())
	}
	if p.attempted {
		t.Fatal("unordered publication attempted link")
	}
	if err := p.write(nil); err != nil {
		t.Fatal(err)
	}
	requirePublicationPhaseRefusal(t, p.write(nil))
	requirePublicationPhaseRefusal(t, p.link("entry"))
	if err := p.syncFile(); err != nil {
		t.Fatal(err)
	}
	if err := p.link("entry"); err != nil {
		t.Fatal(err)
	}
	requirePublicationPhaseRefusal(t, p.verify("entry", nil))
	requirePublicationPhaseRefusal(t, p.link("second"))
	if err := p.syncParent(); err != nil {
		t.Fatal(err)
	}
	if err := p.verify("entry", nil); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationAnonymousAbortAndDescriptorLifetime(t *testing.T) {
	root, parent := localFixture(t)
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for range 32 {
		p := publicationFixture(t, parent, []byte("anonymous abort"), 2)
		fd := p.fd
		if err := p.close(); err != nil {
			t.Fatal(err)
		}
		if !p.closed || p.fd != -1 {
			t.Fatal("closed publication retained descriptor authority")
		}
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			t.Fatal("publication close leaked its descriptor")
		}
		if err := p.close(); err != nil {
			t.Fatal("publication close is not idempotent")
		}
		requirePublicationPhaseRefusal(t, p.write(nil))
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("anonymous abort leaked descriptors")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("anonymous abort left named residue")
	}
	if _, err := observeFD(parent.fd); err != nil {
		t.Fatal("publication closed borrowed parent")
	}
}

func TestPublicationConcurrentNoClobber(t *testing.T) {
	root, parent := localFixture(t)
	type result struct {
		id   Identity
		err  error
		data []byte
	}
	results := make(chan result, 16)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := []byte(fmt.Sprintf("complete contender %d", i))
			id, err := localPublication(parent, "entry", data, 0o600)
			results <- result{id, err, data}
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	var winner result
	for r := range results {
		if r.err == nil {
			winners++
			winner = r
		} else {
			requirePublicationRefusal(t, r.id, r.err, true)
		}
	}
	if winners != 1 {
		t.Fatal("concurrent publication did not have exactly one winner")
	}
	got, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || !bytes.Equal(got, winner.data) {
		t.Fatal("concurrent publication exposed mixed or overwritten bytes")
	}
	stamp, err := observeFile(parent.fd, "entry")
	if err != nil || stamp.identity != winner.id || stamp.identity.Links != 1 {
		t.Fatal("concurrent publication changed winning inode")
	}
	id, err := localPublication(parent, "entry", []byte("later replacement"), 0o600)
	requirePublicationRefusal(t, id, err, true)
	after, err := observeFile(parent.fd, "entry")
	if err != nil || after != stamp {
		t.Fatal("repeated invocation changed winning inode")
	}
}

func TestPublicationObservedParentChangesAtEveryPhase(t *testing.T) {
	for through, name := range []string{"write", "file-sync", "link", "parent-sync", "verify"} {
		t.Run(name, func(t *testing.T) {
			root, parent := localFixture(t)
			data := []byte("complete protected bytes")
			p := publicationFixture(t, parent, data, through)
			if err := os.Chmod(root, 0o500); err != nil {
				t.Fatal(err)
			}
			phases := []func() error{func() error { return p.write(data) }, p.syncFile,
				func() error { return p.link("entry") }, p.syncParent, func() error { return p.verify("entry", data) }}
			err := phases[through]()
			if restoreErr := os.Chmod(root, 0o700); restoreErr != nil {
				t.Fatal(restoreErr)
			}
			requirePublicationPhaseRefusal(t, err)
			if through <= 2 && p.attempted {
				t.Fatal("changed parent reached link boundary")
			}
			if through >= 3 {
				got, err := os.ReadFile(filepath.Join(root, "entry"))
				if err != nil || !bytes.Equal(got, data) {
					t.Fatal("post-link refusal removed or changed published file")
				}
			}
		})
	}
}

func TestPublicationObservedLeafChanges(t *testing.T) {
	for _, through := range []int{3, 4} {
		for _, kind := range []string{"replacement", "symlink", "hardlink", "mode", "same-size-bytes", "truncation", "unlink"} {
			t.Run(fmt.Sprintf("%d/%s", through, kind), func(t *testing.T) {
				root, parent := localFixture(t)
				data := []byte("complete original")
				p := publicationFixture(t, parent, data, through)
				name := filepath.Join(root, "entry")
				switch kind {
				case "replacement", "symlink":
					if err := os.Rename(name, filepath.Join(root, "moved")); err != nil {
						t.Fatal(err)
					}
					if kind == "replacement" {
						if err := os.WriteFile(name, data, 0o600); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Symlink("moved", name); err != nil {
						t.Fatal(err)
					}
				case "hardlink":
					if err := os.Link(name, filepath.Join(root, "alias")); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(name, 0o400); err != nil {
						t.Fatal(err)
					}
				case "same-size-bytes":
					if err := os.WriteFile(name, bytes.Repeat([]byte{'x'}, len(data)), 0o600); err != nil {
						t.Fatal(err)
					}
				case "truncation":
					if err := os.Truncate(name, 2); err != nil {
						t.Fatal(err)
					}
				case "unlink":
					if err := os.Remove(name); err != nil {
						t.Fatal(err)
					}
				}
				before, beforeErr := observeFile(p.fd, "")
				var err error
				if through == 3 {
					err = p.syncParent()
				} else {
					err = p.verify("entry", data)
				}
				requirePublicationPhaseRefusal(t, err)
				if err := p.close(); err != nil {
					t.Fatal(err)
				}
				// Refusal must not clean up the caller's changed entry or the original inode.
				if kind != "unlink" {
					if _, err := os.Lstat(name); err != nil {
						t.Fatal("refusal unlinked the changed entry")
					}
				}
				if kind == "replacement" || kind == "symlink" {
					moved, err := observeFile(parent.fd, "moved")
					if beforeErr != nil || err != nil || moved != before {
						t.Fatal("refusal changed the retained published inode")
					}
				}
			})
		}
	}
}

func publicationPathDescriptor(t *testing.T, fd int) int {
	t.Helper()
	pathfd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(fd), unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot obtain disposable metadata-only descriptor")
	}
	return pathfd
}

func TestPublicationRealDescriptorFailures(t *testing.T) {
	for _, through := range []int{0, 1, 4} {
		t.Run(strconv.Itoa(through), func(t *testing.T) {
			root, parent := localFixture(t)
			data := []byte("complete file before descriptor refusal")
			p := publicationFixture(t, parent, data, through)
			pathfd := publicationPathDescriptor(t, p.fd)
			if err := unix.Close(p.fd); err != nil {
				t.Fatal(err)
			}
			p.fd = pathfd
			if stamp, err := observeFile(pathfd, ""); err != nil || stamp != p.stamp {
				t.Fatal("metadata-only error fixture changed identity")
			}
			var err error
			switch through {
			case 0:
				err = p.write(data)
			case 1:
				err = p.syncFile()
			case 4:
				err = p.verify("entry", data)
			}
			// Actual pwrite/fsync/pread on O_PATH fails; no syscall is mocked.
			requirePublicationPhaseRefusal(t, err)
			if err := p.close(); err != nil {
				t.Fatal(err)
			}
			if through == 4 {
				got, err := os.ReadFile(filepath.Join(root, "entry"))
				if err != nil || !bytes.Equal(got, data) {
					t.Fatal("verification error removed or changed complete publication")
				}
			}
		})
	}
	root, parent := localFixture(t)
	data := []byte("complete file survives parent sync failure")
	p := publicationFixture(t, parent, data, 3)
	pathfd := publicationPathDescriptor(t, parent.fd)
	ownChild(t, edge{fd: pathfd})
	p.parent.fd = pathfd
	requirePublicationPhaseRefusal(t, p.syncParent())
	before := p.stamp
	if err := p.close(); err != nil {
		t.Fatal(err)
	}
	after, err := observeFile(parent.fd, "entry")
	if err != nil || after != before {
		t.Fatal("parent sync error removed or changed complete publication")
	}
	got, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("parent sync error changed published bytes")
	}
}

func TestPublicationRetainedIdentityAndExactByteComparison(t *testing.T) {
	root, parent := localFixture(t)
	data := []byte("complete original")
	p := publicationFixture(t, parent, data, 4)
	before := p.stamp
	for _, edit := range []func(*fileStamp){
		func(s *fileStamp) { s.identity.Device++ },
		func(s *fileStamp) { s.identity.Inode++ },
		func(s *fileStamp) { s.identity.MountID++ },
		func(s *fileStamp) { s.identity.Owner++ },
		func(s *fileStamp) { s.identity.Mode ^= 0o100 },
		func(s *fileStamp) { s.identity.Links++ },
		func(s *fileStamp) { s.identity.Size++ },
		func(s *fileStamp) { s.modified.Nsec++ },
		func(s *fileStamp) { s.changed.Nsec++ },
	} {
		edit(&p.stamp)
		err := p.validate()
		p.stamp = before
		requirePublicationPhaseRefusal(t, err)
	}
	// Metadata is still exact; only an actual byte comparison can reject this.
	requirePublicationPhaseRefusal(t, p.verify("entry", bytes.Repeat([]byte{'x'}, len(data))))
	if p.pin == nil {
		t.Fatal("exact-byte failure did not retain its admitted final pin for cleanup")
	}
	if err := p.close(); err != nil {
		t.Fatal(err)
	}
	after, err := observeFile(parent.fd, "entry")
	if err != nil || after != before {
		t.Fatal("byte mismatch changed publication metadata")
	}
	got, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("byte mismatch changed publication content")
	}
}

func TestPublicationCloseFailurePreservesPublishedInode(t *testing.T) {
	root, parent := localFixture(t)
	data := []byte("complete file survives cleanup error")
	p := publicationFixture(t, parent, data, 5)
	fd, pinfd := p.fd, p.pin.fd
	before := p.stamp
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	requirePublicationPhaseRefusal(t, p.close())
	if !p.closed || p.fd != -1 {
		t.Fatal("failed close retained publication authority")
	}
	if _, err := unix.FcntlInt(uintptr(pinfd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatal("failed close leaked final pin")
	}
	if err := p.close(); err != nil {
		t.Fatal("failed close was retried")
	}
	after, err := observeFile(parent.fd, "entry")
	if err != nil || after != before {
		t.Fatal("cleanup error changed published inode")
	}
	got, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("cleanup error changed published bytes")
	}
}

func TestPublicationDescriptorSubstitutionAtEveryPhase(t *testing.T) {
	for _, target := range []string{"file", "parent"} {
		for through := range 5 {
			t.Run(fmt.Sprintf("%s/%d", target, through), func(t *testing.T) {
				root, parent := localFixture(t)
				_, replacementParent := localFixture(t)
				data := []byte("complete protected bytes")
				p := publicationFixture(t, parent, data, through)
				if err := os.WriteFile(filepath.Join(root, "outside-sentinel"), []byte("untouched sentinel"), 0o600); err != nil {
					t.Fatal(err)
				}
				sentinel, err := observeFile(parent.fd, "outside-sentinel")
				if err != nil {
					t.Fatal(err)
				}
				oldfd, oldparent := p.fd, p.parent
				if target == "file" {
					fd, err := unix.Openat(parent.fd, "outside-sentinel", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
					if err != nil {
						t.Fatal(err)
					}
					ownChild(t, edge{fd: fd})
					p.fd = fd
				} else {
					p.parent.fd = replacementParent.fd
				}
				phases := []func() error{func() error { return p.write(data) }, p.syncFile,
					func() error { return p.link("entry") }, p.syncParent, func() error { return p.verify("entry", data) }}
				err = phases[through]()
				p.fd, p.parent = oldfd, oldparent
				requirePublicationPhaseRefusal(t, err)
				after, err := observeFile(parent.fd, "outside-sentinel")
				if err != nil || after != sentinel {
					t.Fatal("descriptor substitution changed outside sentinel metadata")
				}
				got, err := os.ReadFile(filepath.Join(root, "outside-sentinel"))
				if err != nil || string(got) != "untouched sentinel" {
					t.Fatal("descriptor substitution changed outside sentinel content")
				}
			})
		}
	}
}

func TestPublicationFinalEdgeAfterClose(t *testing.T) {
	for _, kind := range []string{"unchanged", "replacement", "unlink", "hardlink", "mode", "bytes", "parent-mode"} {
		t.Run(kind, func(t *testing.T) {
			root, parent := localFixture(t)
			data := []byte("complete original")
			p := publicationFixture(t, parent, data, 5)
			if err := p.close(); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(root, "entry")
			switch kind {
			case "replacement":
				if err := os.Rename(name, filepath.Join(root, "moved")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, data, 0o600); err != nil {
					t.Fatal(err)
				}
			case "unlink":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(name, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(name, 0o400); err != nil {
					t.Fatal(err)
				}
			case "bytes":
				if err := os.WriteFile(name, bytes.Repeat([]byte{'x'}, len(data)), 0o600); err != nil {
					t.Fatal(err)
				}
			case "parent-mode":
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal(err)
				}
			}
			err := p.validateLinked()
			if kind == "parent-mode" {
				if err := os.Chmod(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unchanged" {
				if err != nil {
					t.Fatal("unchanged final edge failed after descriptor cleanup")
				}
			} else {
				requirePublicationPhaseRefusal(t, err)
			}
		})
	}
}

func TestPublicationPositiveAndConcurrentCloseOnAdmissibleTopology(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(root, owner) {
		t.Skip("positive PublishNoReplace/Close requires an absolute topology admitted by the approved owner/mode policy")
	}
	dir, err := OpenRoot(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	id, err := dir.PublishNoReplace("entry", []byte("complete public publication"), 0o600)
	if err != nil || id == (Identity{}) {
		t.Fatal("public publication failed")
	}
	id, err = dir.PublishNoReplace("entry", []byte("replacement"), 0o600)
	requirePublicationRefusal(t, id, err, true)
	copyDir := *dir
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := dir.PublishNoReplace(fmt.Sprintf("concurrent-%d", i), nil, 0o600)
			if err != nil {
				requirePublicationRefusal(t, id, err, false)
			}
			if err := copyDir.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
