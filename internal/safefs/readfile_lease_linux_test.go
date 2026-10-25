//go:build linux

package safefs

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-020/030, VAL-027: the local core does not qualify absolute-root admission.
func TestReadFileReopenLeaseNonblocking(t *testing.T) {
	if os.Getenv("ZERB_SAFEFS_READ_LEASE_HELPER") == "open" {
		fd, err := unix.Open(os.Getenv("ZERB_SAFEFS_LOCK_ROOT"), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal("cannot open disposable read parent")
		}
		defer func() {
			if err := unix.Close(fd); err != nil {
				t.Error("read parent cleanup failed")
			}
		}()
		id, err := observeFD(fd)
		if err != nil {
			t.Fatal("cannot observe disposable read parent")
		}
		before := adversarialFDCount(t)
		for range 32 {
			data, id, err := readFile(edge{fd: fd, id: id}, "entry", 64)
			assertAdversarialRefusal(t, data, id, err)
		}
		if adversarialFDCount(t) != before {
			t.Fatal("repeated leased read refusal leaked descriptors")
		}
		return
	}
	root, parent := localFixture(t)
	want := []byte("disposable leased read content")
	writeAdversarialFile(t, root, "entry", want)
	before, err := observeFile(parent.fd, "entry")
	if err != nil {
		t.Fatal("cannot observe leased read fixture")
	}
	// Reuse the established holder, which retains its write lease on SIGIO.
	holder := exec.Command(os.Args[0], "-test.run=^TestLockKernelReopenLeaseNonblocking$")
	holder.Env = append(os.Environ(), "ZERB_SAFEFS_LOCK_LEASE_HELPER=hold", "ZERB_SAFEFS_LOCK_ROOT="+root)
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal("cannot create read lease readiness channel")
	}
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal("cannot create read lease lifetime channel")
	}
	if err := holder.Start(); err != nil {
		t.Fatal("cannot start read lease holder")
	}
	holderWaited := false
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error("read lease lifetime channel cleanup failed")
		}
		if holderWaited {
			return
		}
		if err := holder.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error("read lease holder termination failed")
		}
		if err := holder.Wait(); err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Error("read lease holder reap failed")
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
			t.Skip("F_SETLEASE unavailable on disposable owner file; existing holder does not expose exact errno (eligible errors: EINVAL, ENOSYS, EOPNOTSUPP, EPERM, EACCES)")
		}
		if line != "leased\n" {
			t.Fatal("child did not establish file lease before read")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read file lease setup timed out")
	}
	// A separate process lets the watchdog kill and reap a blocking regression.
	opener := exec.Command(os.Args[0], "-test.run=^TestReadFileReopenLeaseNonblocking$")
	opener.Env = append(os.Environ(), "ZERB_SAFEFS_READ_LEASE_HELPER=open", "ZERB_SAFEFS_LOCK_ROOT="+root)
	if err := opener.Start(); err != nil {
		t.Fatal("cannot start leased read")
	}
	done := make(chan error, 1)
	go func() { done <- opener.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("leased read did not return fixed refusal without bytes or identity")
		}
	case <-time.After(5 * time.Second):
		if err := opener.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error("blocked read termination failed")
		}
		<-done
		t.Fatal("read blocked on a conflicting file lease")
	}
	// Inspection must wait until the lease is released, or it could itself block.
	if err := stdin.Close(); err != nil {
		t.Fatal("cannot release read lease lifetime channel")
	}
	if err := holder.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal("cannot terminate read lease holder before inspection")
	}
	err = holder.Wait()
	holderWaited = true
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal("read lease holder reap failed")
		}
	}
	data, id, readErr := readFile(parent, "entry", 64)
	after, statErr := observeFile(parent.fd, "entry")
	if readErr != nil || statErr != nil || string(data) != string(want) || id != before.identity || after != before {
		t.Fatal("leased read refusal changed fixture contents or metadata")
	}
}

func TestReadFileReopenFlagsAndContents(t *testing.T) {
	root, parent := localFixture(t)
	want := []byte("disposable ordinary read content")
	writeAdversarialFile(t, root, "entry", want)
	pin := adversarialPin(t, parent, "entry", 64)
	proc := adversarialProc(t)
	reader, err := proc.reopen(pin)
	if err != nil {
		t.Fatal("cannot reopen disposable ordinary file")
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error("ordinary reader cleanup failed")
		}
	})
	flags, err := unix.FcntlInt(reader.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY || flags&unix.O_NONBLOCK == 0 || flags&(unix.O_PATH|unix.O_APPEND|unix.O_TRUNC) != 0 {
		t.Fatal("ordinary read reopen has unexpected access flags")
	}
	flags, err = unix.FcntlInt(reader.Fd(), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("ordinary read reopen is not close-on-exec")
	}
	data, err := readContents(reader, pin.before.identity.Size)
	stamp, statErr := observeFile(int(reader.Fd()), "")
	if err != nil || statErr != nil || string(data) != string(want) || stamp != pin.before || pin.validate() != nil || proc.validate() != nil {
		t.Fatal("nonblocking ordinary read changed content or metadata")
	}
	data, id, err := readFile(parent, "entry", 64)
	if err != nil || string(data) != string(want) || id != pin.before.identity {
		t.Fatal("nonblocking read did not preserve exact content and identity")
	}
}
