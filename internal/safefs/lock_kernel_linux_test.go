//go:build linux

package safefs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-020/030/031: these private-core checks do not admit an absolute root.
func kernelLock(t *testing.T, parent edge, name string, create bool) *lockState {
	t.Helper()
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal("cannot duplicate disposable lock parent")
	}
	s, err := lockAt(chain[0], name, create)
	if err != nil {
		if closeErr := closeChain(chain); closeErr != nil {
			t.Error("failed lock parent cleanup")
		}
		t.Fatal("cannot open disposable lock")
	}
	s.chain = chain
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.closeResources(); err != nil {
			t.Error("lock cleanup failed")
		}
	})
	return s
}

func kernelTry(t *testing.T, s *lockState, mode LockMode) error {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && validateEdges(s.chain, s.owner) != nil {
		t.Fatal("disposable lock parent chain changed")
	}
	return s.try(mode)
}

func kernelUnlock(t *testing.T, s *lockState) error {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && validateEdges(s.chain, s.owner) != nil {
		t.Fatal("disposable lock parent chain changed")
	}
	return s.unlock()
}

func requireKernelBusy(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, unix.EAGAIN) {
		t.Fatal("independent lock description did not report nonblocking contention")
	}
}

func requireKernelRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil || err.Error() != errDirectory.Error() || errors.Is(err, unix.EAGAIN) {
		t.Fatal("invalid lock authority did not receive fixed refusal")
	}
}

func TestLockKernelIndependentDescriptions(t *testing.T) {
	root, parent := localFixture(t)
	want := []byte("disposable lock content")
	writeAdversarialFile(t, root, "entry", want)
	before, err := observeFile(parent.fd, "entry")
	if err != nil {
		t.Fatal("cannot observe disposable lock")
	}
	a, b, c := kernelLock(t, parent, "entry", false), kernelLock(t, parent, "entry", false), kernelLock(t, parent, "entry", false)
	for _, s := range []*lockState{a, b} {
		if err := kernelTry(t, s, LockShared); err != nil {
			t.Fatal("independent shared descriptions did not coexist")
		}
	}
	requireKernelBusy(t, kernelTry(t, c, LockExclusive))
	// An independent syscall oracle rules out a process-associated lock fallback.
	fd, err := unix.Openat(parent.fd, "entry", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open independent syscall probe")
	}
	defer func() {
		if err := unix.Close(fd); err != nil {
			t.Error("syscall probe cleanup failed")
		}
	}()
	flock := unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart}
	if err := unix.FcntlFlock(uintptr(fd), unix.F_OFD_SETLK, &flock); !errors.Is(err, unix.EAGAIN) {
		t.Fatal("actual OFD syscall did not observe shared exclusion")
	}
	if err := kernelUnlock(t, a); err != nil {
		t.Fatal("shared unlock failed")
	}
	requireKernelBusy(t, kernelTry(t, c, LockExclusive))
	if err := kernelUnlock(t, b); err != nil {
		t.Fatal("last shared unlock failed")
	}
	if err := kernelTry(t, c, LockExclusive); err != nil {
		t.Fatal("exclusive acquisition after shared release failed")
	}
	if err := kernelTry(t, c, LockExclusive); err != nil {
		t.Fatal("same-mode revalidation failed")
	}
	requireKernelRefusal(t, kernelTry(t, c, LockShared))
	requireKernelBusy(t, kernelTry(t, a, LockShared))
	requireKernelBusy(t, kernelTry(t, b, LockExclusive))
	// Closing another description must not release an OFD-owned exclusion.
	unrelated, err := os.Open(filepath.Join(root, "entry"))
	if err != nil {
		t.Fatal("cannot open unrelated description")
	}
	if err := unrelated.Close(); err != nil {
		t.Fatal("unrelated description close failed")
	}
	requireKernelBusy(t, kernelTry(t, b, LockExclusive))
	if err := kernelUnlock(t, c); err != nil {
		t.Fatal("exclusive unlock failed")
	}
	if err := kernelTry(t, a, LockExclusive); err != nil {
		t.Fatal("one unlock did not release repeated same-mode acquisition")
	}
	if err := kernelUnlock(t, a); err != nil {
		t.Fatal("exclusive unlock failed")
	}
	if err := kernelUnlock(t, a); err != nil {
		t.Fatal("unheld unlock was not idempotent")
	}
	data, readErr := os.ReadFile(filepath.Join(root, "entry"))
	after, statErr := observeFile(parent.fd, "entry")
	if readErr != nil || statErr != nil || string(data) != string(want) || after != before {
		t.Fatal("lock operations changed existing contents or metadata")
	}
}

func TestLockKernelEmptyCreationAndIndependentLifetime(t *testing.T) {
	root, parent := localFixture(t)
	chain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal("cannot duplicate disposable directory")
	}
	dir := &Dir{state: &dirState{chain: chain, owner: parent.id.Owner, identity: parent.id}}
	defer dir.Close()
	s := kernelLock(t, chain[0], "entry", true)
	if err := dir.Close(); err != nil {
		t.Fatal("directory close failed")
	}
	if err := kernelTry(t, s, LockExclusive); err != nil {
		t.Fatal("lock did not retain independent directory lifetime")
	}
	contender := kernelLock(t, parent, "entry", false)
	requireKernelBusy(t, kernelTry(t, contender, LockExclusive))
	lock := Lock{state: s}
	copyLock := lock
	fd := s.fd
	// An unadmitted local topology refuses public validation but must still close.
	requireKernelRefusal(t, copyLock.Close())
	if lock.Identity() != (Identity{}) || copyLock.Identity() != (Identity{}) {
		t.Fatal("closed value copy retained identity")
	}
	if err := kernelTry(t, contender, LockExclusive); err != nil {
		t.Fatal("close did not release kernel exclusion")
	}
	probe, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, fd)
	if err != nil {
		t.Fatal("cannot reuse a released descriptor number")
	}
	defer func() {
		if err := unix.Close(probe); err != nil {
			t.Error("reused descriptor cleanup failed")
		}
	}()
	if probe != fd {
		t.Fatal("released lock descriptor was not available for reuse")
	}
	if err := lock.Close(); err != nil {
		t.Fatal("copied lock close was not idempotent")
	}
	if _, err := observeFD(probe); err != nil {
		t.Fatal("copied lock close closed a reused descriptor")
	}
	data, err := os.ReadFile(filepath.Join(root, "entry"))
	if err != nil || len(data) != 0 {
		t.Fatal("empty lock creation or use wrote file contents")
	}
}

func TestLockKernelConcurrentTryUnlockClose(t *testing.T) {
	_, parent := localFixture(t)
	s := kernelLock(t, parent, "entry", true)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 16 {
				var err error
				switch i % 3 {
				case 0:
					err = kernelTry(t, s, LockExclusive)
				case 1:
					err = kernelUnlock(t, s)
				case 2:
					s.mu.Lock()
					err = s.closeResources()
					s.mu.Unlock()
				}
				if err != nil {
					requireKernelRefusal(t, err)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	contender := kernelLock(t, parent, "entry", false)
	if err := kernelTry(t, contender, LockExclusive); err != nil {
		t.Fatal("concurrent close retained an exclusion")
	}
}

func TestLockKernelSubstitutionOverridesBusy(t *testing.T) {
	for _, mutation := range []string{"leaf", "parent-mode", "content", "hardlink"} {
		t.Run(mutation, func(t *testing.T) {
			root, parent := localFixture(t)
			holder := kernelLock(t, parent, "entry", true)
			contender := kernelLock(t, parent, "entry", false)
			if err := kernelTry(t, holder, LockExclusive); err != nil {
				t.Fatal("cannot hold disposable exclusion")
			}
			requireKernelBusy(t, kernelTry(t, contender, LockExclusive))
			probe, err := unix.Openat(parent.fd, "entry", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal("cannot open release probe")
			}
			defer func() {
				if err := unix.Close(probe); err != nil {
					t.Error("release probe cleanup failed")
				}
			}()
			switch mutation {
			case "leaf":
				if err := os.Rename(filepath.Join(root, "entry"), filepath.Join(root, "moved")); err != nil {
					t.Fatal("cannot replace disposable lock")
				}
				writeAdversarialFile(t, root, "entry", nil)
			case "parent-mode":
				if err := os.Chmod(root, 0o500); err != nil {
					t.Fatal("cannot change disposable parent mode")
				}
				t.Cleanup(func() {
					if err := os.Chmod(root, 0o700); err != nil {
						t.Error("parent mode restoration failed")
					}
				})
			case "content":
				writeAdversarialFile(t, root, "entry", []byte("changed"))
			case "hardlink":
				if err := os.Link(filepath.Join(root, "entry"), filepath.Join(root, "alias")); err != nil {
					t.Fatal("cannot add disposable alias")
				}
			}
			// Core resource checks are shared by public Try; parent-chain policy is
			// separately enforced by its wrapper and is not bypassed here.
			contender.mu.Lock()
			chainErr := validateEdges(contender.chain, contender.owner)
			err = contender.try(LockExclusive)
			closed := contender.closed
			contender.mu.Unlock()
			requireKernelRefusal(t, err)
			if (chainErr != nil) != (mutation == "parent-mode") {
				t.Fatal("unexpected parent-chain substitution result")
			}
			if !closed {
				t.Fatal("known substitution retained lock authority")
			}
			holder.mu.Lock()
			chainErr = validateEdges(holder.chain, holder.owner)
			if mutation == "leaf" || mutation == "content" {
				err = holder.unlock()
			} else {
				err = holder.try(LockExclusive)
			}
			closed = holder.closed
			holder.mu.Unlock()
			requireKernelRefusal(t, err)
			if (chainErr != nil) != (mutation == "parent-mode") {
				t.Fatal("unexpected held parent-chain substitution result")
			}
			if !closed {
				t.Fatal("known substitution retained held kernel exclusion")
			}
			if err := unix.FcntlFlock(uintptr(probe), unix.F_OFD_SETLK, &unix.Flock_t{Type: unix.F_WRLCK, Whence: unix.SEEK_SET}); err != nil {
				t.Fatal("known substitution did not release the original inode exclusion")
			}
		})
	}
}

func TestLockKernelRetainedParentEdge(t *testing.T) {
	root, ancestor := localFixture(t)
	child, err := childAt(ancestor.fd, "parent", ancestor.id.Owner, true)
	if err != nil {
		t.Fatal("cannot create disposable lock parent")
	}
	ownChild(t, child)
	s := kernelLock(t, child, "entry", true)
	prefix, err := duplicateChain([]edge{ancestor})
	if err != nil {
		t.Fatal("cannot retain disposable ancestor")
	}
	s.chain = append(prefix, s.chain...)
	if err := kernelTry(t, s, LockExclusive); err != nil {
		t.Fatal("cannot acquire disposable parent-edge lock")
	}
	probe, err := unix.Openat(child.fd, "entry", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open original-inode probe")
	}
	defer func() {
		if err := unix.Close(probe); err != nil {
			t.Error("original-inode probe cleanup failed")
		}
	}()
	if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "moved")); err != nil {
		t.Fatal("cannot move disposable parent")
	}
	if err := os.Mkdir(filepath.Join(root, "parent"), 0o700); err != nil {
		t.Fatal("cannot substitute disposable parent")
	}
	writeAdversarialFile(t, filepath.Join(root, "parent"), "entry", nil)
	// A still-valid leaf pin cannot establish authority over a replaced parent.
	s.mu.Lock()
	resourceErr := s.validate()
	chainErr := validateEdges(s.chain, s.owner)
	s.mu.Unlock()
	if resourceErr != nil || chainErr == nil {
		t.Fatal("retained ancestor edge did not distinguish parent substitution")
	}
	// The public wrapper always requires an admitted absolute chain, including
	// here; this assertion does not claim a positive public-root traversal.
	lock := Lock{state: s}
	requireKernelRefusal(t, lock.Try(LockExclusive))
	if err := unix.FcntlFlock(uintptr(probe), unix.F_OFD_SETLK, &unix.Flock_t{Type: unix.F_WRLCK, Whence: unix.SEEK_SET}); err != nil {
		t.Fatal("parent refusal did not release original-inode exclusion")
	}
}

// Synthetic retained-observation corruption is not a host proc/mount proof.
func TestLockKernelSyntheticProcInvalidationReleasesOFD(t *testing.T) {
	for _, operation := range []string{"same-mode-try", "unlock"} {
		for _, observation := range []string{"pid", "mount"} {
			t.Run(operation+"/"+observation, func(t *testing.T) {
				_, parent := localFixture(t)
				s := kernelLock(t, parent, "entry", true)
				probe, err := unix.Openat(parent.fd, "entry", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if err != nil {
					t.Fatal("cannot open original-inode release probe")
				}
				defer func() {
					if err := unix.Close(probe); err != nil {
						t.Error("original-inode release probe cleanup failed")
					}
				}()
				if err := kernelTry(t, s, LockExclusive); err != nil {
					t.Fatal("cannot acquire real exclusion before synthetic invalidation")
				}
				flock := unix.Flock_t{Type: unix.F_WRLCK, Whence: unix.SEEK_SET}
				if err := unix.FcntlFlock(uintptr(probe), unix.F_OFD_SETLK, &flock); !errors.Is(err, unix.EAGAIN) {
					t.Fatal("independent probe did not observe the held OFD")
				}
				s.mu.Lock()
				if validateEdges(s.chain, s.owner) != nil {
					s.mu.Unlock()
					t.Fatal("disposable parent edges changed")
				}
				if observation == "pid" {
					s.proc.pid = "0"
				} else {
					s.proc.chain[3].id.MountID++
				}
				if operation == "same-mode-try" {
					err = s.try(LockExclusive)
				} else {
					err = s.unlock()
				}
				cleared := s.closed && s.mode == 0 && s.identity == (Identity{}) && s.fd == -1 && len(s.chain) == 0 &&
					s.pin.closed && s.pin.fd == -1 && s.proc.closed && len(s.proc.chain) == 0 && s.proc.self.fd == -1
				s.mu.Unlock()
				requireKernelRefusal(t, err)
				if !cleared || (&Lock{state: s}).Identity() != (Identity{}) {
					t.Fatal("invalid proc observation retained lock capability")
				}
				if err := unix.FcntlFlock(uintptr(probe), unix.F_OFD_SETLK, &flock); err != nil {
					t.Fatal("proc invalidation did not release original-inode OFD exclusion")
				}
			})
		}
	}
}

func TestLockKernelProcessAndExec(t *testing.T) {
	mode := os.Getenv("ZERB_SAFEFS_LOCK_HELPER")
	if mode != "" {
		lockKernelChild(t, mode)
		return
	}
	root, parent := localFixture(t)
	s := kernelLock(t, parent, "entry", true)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockKernelProcessAndExec$")
	cmd.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_HELPER=hold", "ZERB_SAFEFS_LOCK_ROOT="+root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("cannot create child readiness channel")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("cannot create child lifetime channel")
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal("cannot start lock holder")
	}
	waited := false
	defer func() {
		if !waited {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error("child termination failed")
			}
			if err := cmd.Wait(); err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Error("child reap failed")
				}
			}
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		ready <- err == nil && line == "locked\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child did not acquire independent lock")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock holder readiness timed out")
	}
	requireKernelBusy(t, kernelTry(t, s, LockExclusive))
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal("cannot terminate lock holder")
	}
	err = cmd.Wait()
	waited = true
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatal("lock holder did not terminate by signal")
	}
	if err := kernelTry(t, s, LockExclusive); err != nil {
		t.Fatal("kernel did not release exclusion after holder death")
	}
	for _, fd := range []int{s.fd, s.pin.fd, s.pin.parent.fd, s.proc.self.fd, s.proc.chain[0].fd, s.proc.chain[1].fd, s.proc.chain[2].fd, s.proc.chain[3].fd} {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("retained lock descriptor lacks close-on-exec")
		}
	}
	probe := exec.Command(os.Args[0], "-test.run=^TestLockKernelProcessAndExec$")
	probe.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_HELPER=exec", "ZERB_SAFEFS_LOCK_DEVICE="+strconv.FormatUint(s.identity.Device, 10), "ZERB_SAFEFS_LOCK_INODE="+strconv.FormatUint(s.identity.Inode, 10))
	if _, err := probe.CombinedOutput(); err != nil {
		t.Fatal("actual exec inherited a lock descriptor or child probe failed")
	}
	contender := kernelLock(t, parent, "entry", false)
	requireKernelBusy(t, kernelTry(t, contender, LockExclusive))
}

func lockKernelChild(t *testing.T, mode string) {
	t.Helper()
	if mode == "exec" {
		device, err := strconv.ParseUint(os.Getenv("ZERB_SAFEFS_LOCK_DEVICE"), 10, 64)
		if err != nil {
			t.Fatal("invalid exec probe identity")
		}
		inode, err := strconv.ParseUint(os.Getenv("ZERB_SAFEFS_LOCK_INODE"), 10, 64)
		if err != nil {
			t.Fatal("invalid exec probe identity")
		}
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal("cannot enumerate child descriptors")
		}
		for _, entry := range entries {
			fd, err := strconv.Atoi(entry.Name())
			if err != nil {
				t.Fatal("invalid child descriptor entry")
			}
			var st unix.Stat_t
			if err := unix.Fstat(fd, &st); errors.Is(err, unix.EBADF) {
				continue // ReadDir's own descriptor has already closed.
			} else if err != nil {
				t.Fatal("cannot inspect child descriptor")
			}
			if st.Dev == device && st.Ino == inode {
				t.Fatal("lock descriptor survived exec")
			}
		}
		return
	}
	if mode != "hold" {
		t.Fatal("invalid lock child mode")
	}
	fd, err := unix.Open(os.Getenv("ZERB_SAFEFS_LOCK_ROOT"), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open disposable child fixture")
	}
	defer func() {
		if err := unix.Close(fd); err != nil {
			t.Error("child fixture cleanup failed")
		}
	}()
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal("cannot observe disposable child fixture")
	}
	s := kernelLock(t, edge{fd: fd, id: id}, "entry", false)
	if err := kernelTry(t, s, LockExclusive); err != nil {
		t.Fatal("child exclusive lock failed")
	}
	if _, err := fmt.Fprintln(os.Stdout, "locked"); err != nil {
		t.Fatal("child readiness failed")
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal("child lifetime channel failed")
	}
}

func TestLockKernelReopenLeaseNonblocking(t *testing.T) {
	if mode := os.Getenv("ZERB_SAFEFS_LOCK_LEASE_HELPER"); mode != "" {
		lockKernelLeaseChild(t, mode)
		return
	}
	root, parent := localFixture(t)
	want := []byte("disposable leased lock content")
	writeAdversarialFile(t, root, "entry", want)
	before, err := observeFile(parent.fd, "entry")
	if err != nil {
		t.Fatal("cannot observe leased fixture")
	}
	holder := exec.Command(os.Args[0], "-test.run=^TestLockKernelReopenLeaseNonblocking$")
	holder.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_LEASE_HELPER=hold", "ZERB_SAFEFS_LOCK_ROOT="+root)
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal("cannot create lease readiness channel")
	}
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal("cannot create lease lifetime channel")
	}
	if err := holder.Start(); err != nil {
		t.Fatal("cannot start lease holder")
	}
	holderWaited := false
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error("lease lifetime channel cleanup failed")
		}
		if holderWaited {
			return
		}
		if err := holder.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error("lease holder termination failed")
		}
		if err := holder.Wait(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Error("lease holder reap failed")
			}
		}
	})
	ready := make(chan string, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil {
			line = ""
		}
		ready <- line
	}()
	select {
	case line := <-ready:
		if line == "unsupported\n" {
			t.Skip("F_SETLEASE unavailable on disposable owner-file fixture")
		}
		if line != "leased\n" {
			t.Fatal("child did not establish file lease before lock construction")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("file lease setup timed out")
	}
	// The constructor runs in another process so a blocking-open regression can
	// always be killed and reaped without leaving a blocked test goroutine.
	opener := exec.Command(os.Args[0], "-test.run=^TestLockKernelReopenLeaseNonblocking$")
	opener.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_LEASE_HELPER=open", "ZERB_SAFEFS_LOCK_ROOT="+root)
	if err := opener.Start(); err != nil {
		t.Fatal("cannot start leased lock constructor")
	}
	done := make(chan error, 1)
	go func() { done <- opener.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("leased lock construction did not return fixed operational refusal")
		}
	case <-time.After(5 * time.Second):
		if err := opener.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error("blocked lock constructor termination failed")
		}
		<-done
		t.Fatal("lock construction blocked on a conflicting file lease")
	}
	// Inspect bytes only after releasing the write lease; reading earlier would
	// itself block and obscure which production open is under test.
	if err := stdin.Close(); err != nil {
		t.Fatal("cannot release lease lifetime channel")
	}
	if err := holder.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal("cannot terminate lease holder before inspection")
	}
	err = holder.Wait()
	holderWaited = true
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal("lease holder reap failed")
		}
	}
	data, readErr := os.ReadFile(filepath.Join(root, "entry"))
	after, statErr := observeFile(parent.fd, "entry")
	if readErr != nil || statErr != nil || string(data) != string(want) || after != before {
		t.Fatal("leased lock refusal changed fixture contents or metadata")
	}
}

func lockKernelLeaseChild(t *testing.T, mode string) {
	t.Helper()
	fd, err := unix.Open(os.Getenv("ZERB_SAFEFS_LOCK_ROOT"), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open disposable lease parent")
	}
	defer func() {
		if err := unix.Close(fd); err != nil {
			t.Error("lease parent cleanup failed")
		}
	}()
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal("cannot observe disposable lease parent")
	}
	if mode == "open" {
		// This is the production constructor on a local descriptor fixture, not
		// a positive test of exported absolute-root admission.
		s, err := lockAt(edge{fd: fd, id: id}, "entry", false)
		if s != nil {
			if err := s.closeResources(); err != nil {
				t.Error("unexpected lock cleanup failed")
			}
			t.Fatal("leased inode acquired lock authority")
		}
		requireKernelRefusal(t, err)
		return
	}
	if mode != "hold" {
		t.Fatal("invalid lease helper mode")
	}
	file, err := unix.Openat(fd, "entry", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open disposable lease file")
	}
	defer func() {
		if err := unix.Close(file); err != nil {
			t.Error("lease file cleanup failed")
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGIO)
	defer signal.Stop(signals)
	// No O_PATH pin or other file description exists before F_SETLEASE.
	if _, err := unix.FcntlInt(uintptr(file), unix.F_SETLEASE, unix.F_WRLCK); err != nil {
		if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			if _, err := fmt.Fprintln(os.Stdout, "unsupported"); err != nil {
				t.Fatal("lease capability response failed")
			}
			return
		}
		t.Fatal("owner-file write lease setup failed")
	}
	if _, err := fmt.Fprintln(os.Stdout, "leased"); err != nil {
		t.Fatal("lease readiness failed")
	}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, os.Stdin)
		done <- err
	}()
	for {
		select {
		case <-signals:
			// Keep the lease while the production nonblocking open is checked.
		case err := <-done:
			if err != nil {
				t.Fatal("lease lifetime channel failed")
			}
			return
		}
	}
}
