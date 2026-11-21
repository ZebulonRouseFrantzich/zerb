//go:build linux && amd64

package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-019: only owned group-pidfd authority, with an explicit
// cooperating R. No external reaper, numeric PID reuse, containment, authenticated
// role assembly or complete strace/session race qualification follows.
const groupProofScope = 1 << 2 // Linux PIDFD_SIGNAL_PROCESS_GROUP.

func TestPidfdGroupAuthority(t *testing.T) {
	for _, mode := range []string{"leader-reaped", "interrupted-registration", "control-signal"} {
		t.Run(mode, func(t *testing.T) {
			cmd := groupProofCommand("R", mode, t.TempDir())
			var output custodyOutput
			cmd.Stdout, cmd.Stderr = &output, &output
			err := cmd.Run()
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 77 {
				t.Skipf("unexercised group-pidfd capability: %s", output.Bytes())
			}
			if err != nil {
				t.Fatalf("isolated group row: %v\n%s", err, output.Bytes())
			}
		})
	}
}

func groupProofCommand(role, mode, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestPidfdGroupAuthorityChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_GROUP_ROLE="+role,
		"ZERB_GROUP_MODE="+mode, "GORACE=atexit_sleep_ms=0")
	return cmd
}

func TestPidfdGroupAuthorityChild(t *testing.T) {
	role := os.Getenv("ZERB_GROUP_ROLE")
	if role == "" {
		return
	}
	runtime.LockOSThread()
	unexercised := false
	switch role {
	case "R":
		unexercised = groupProofReaper(t)
	case "L":
		groupProofLeader(t)
	case "H", "C":
		groupProofMember(t, role)
	default:
		t.Fatal("unknown group proof role")
	}
	if t.Failed() {
		os.Exit(1)
	}
	if unexercised {
		os.Exit(77)
	}
	os.Exit(0)
}

func groupProofReaper(t *testing.T) bool {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	l, lc, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()
	h, hc, err := socketPair()
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	defer hc.Close()
	c, cc, err := socketPair()
	if err != nil {
		l.Close()
		h.Close()
		t.Fatal(err)
	}
	defer cc.Close()
	var mu sync.Mutex
	var owned []int
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		// Sole peer endpoints revoke even an unacknowledged H birth by EOF.
		for _, control := range []*os.File{l, h, c} {
			if err := groupProofShutdown(control); err != nil {
				t.Errorf("revoke fixture socket: %v", err)
			}
		}
		for _, fd := range owned {
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil && err != syscall.ESRCH {
				t.Errorf("owned pidfd cleanup: %v", err)
			}
		}
	}
	done := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() { cleanup(); close(done) })
	defer func() {
		if !watchdog.Stop() {
			<-done
			t.Error("group fixture watchdog expired")
		}
		cleanup()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var raw syscall.WaitStatus
			pid, waitErr := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil)
			if waitErr == syscall.ECHILD {
				break
			}
			if waitErr == syscall.EINTR {
				continue
			}
			if waitErr != nil {
				t.Errorf("drain group fixture: %v", waitErr)
				break
			}
			if pid == 0 {
				if time.Now().After(deadline) {
					t.Error("group fixture retains unconsumed children")
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		for _, fd := range owned {
			if err := unix.Close(fd); err != nil {
				t.Errorf("close owned pidfd: %v", err)
			}
		}
	}()
	start := func(role string, files []*os.File) (int, int) {
		cmd := groupProofCommand(role, os.Getenv("ZERB_GROUP_MODE"), os.Getenv("HOME"))
		cmd.ExtraFiles = files
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		fd := -1
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, PidFD: &fd}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Process.Release()
		if fd < 0 {
			t.Fatal("birth omitted atomic pidfd")
		}
		mu.Lock()
		owned = append(owned, fd)
		mu.Unlock()
		groupProofFD(t, fd)
		return cmd.Process.Pid, fd
	}
	lPID, lFD := start("L", []*os.File{lc, hc})
	lc.Close()
	hc.Close()
	cPID, cFD := start("C", []*os.File{cc})
	cc.Close()
	born := cloneParentRead(t, l)
	hReady, cReady := cloneParentRead(t, h), cloneParentRead(t, c)
	if born.Gate != "born" || born.PID != lPID || born.Parent <= 0 || born.SID != lPID || born.PGID != lPID ||
		hReady.Gate != "ready" || hReady.PID != born.Parent || hReady.Parent != os.Getpid() || hReady.SID != lPID || hReady.PGID != lPID ||
		cReady.Gate != "ready" || cReady.PID != cPID || cReady.Parent != os.Getpid() || cReady.SID != cPID || cReady.PGID != cPID || syscall.Getpgrp() == lPID {
		t.Fatal("unowned group/session/parent topology")
	}
	if os.Getenv("ZERB_GROUP_MODE") == "interrupted-registration" {
		// H is alive but its atomic pidfd has not been acknowledged or received.
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		cloneTraceWait(t, hReady.PID, 0)
		cloneParentWrite(t, l, cloneParentEvent{Gate: "exit"})
		cloneTraceWait(t, lPID, 17<<8)
		cloneParentWrite(t, c, cloneParentEvent{Gate: "exit"})
		cloneTraceWait(t, cPID, 0)
		groupProofNoChildren(t)
		return false
	}
	cloneParentWrite(t, l, cloneParentEvent{Gate: "register"})
	hFD := cloneTraceReceiveFD(t, l)
	mu.Lock()
	owned = append(owned, hFD)
	mu.Unlock()
	groupProofFD(t, hFD)
	if os.Getenv("ZERB_GROUP_MODE") == "control-signal" {
		if err := unix.PidfdSendSignal(cFD, syscall.SIGTERM, nil, 0); err != nil {
			t.Fatal(err)
		}
		cloneTraceWait(t, cPID, uint32(syscall.SIGTERM))
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		cloneTraceWait(t, hReady.PID, 0)
		cloneParentWrite(t, l, cloneParentEvent{Gate: "exit"})
		cloneTraceWait(t, lPID, 17<<8)
		groupProofNoChildren(t)
		return false
	}
	// Both identities remain held through the exact consuming waits below.
	if err := unix.PidfdSendSignal(lFD, 0, nil, groupProofScope); err == syscall.EINVAL || err == syscall.ENOSYS {
		fmt.Fprintln(os.Stderr, "owned live leader group scope unavailable:", err)
		return true // Dispatcher first checks deferred cleanup and actual ECHILD.
	} else if err != nil {
		t.Fatal(err)
	}
	for _, fd := range []int{lFD, hFD, cFD} {
		if err := unix.PidfdSendSignal(fd, 0, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	ping := func(control *os.File) {
		cloneParentWrite(t, control, cloneParentEvent{Gate: "ping"})
		if cloneParentRead(t, control).Gate != "pong" {
			t.Fatal("group member/control lost responsiveness")
		}
	}
	send := func() {
		if err := unix.PidfdSendSignal(lFD, syscall.SIGTERM, nil, groupProofScope); err != nil {
			t.Fatal(err)
		}
		if cloneParentRead(t, h).Gate != "signal" {
			t.Fatal("owned H did not acknowledge group signal")
		}
		ping(c)
	}
	send()
	cloneParentWrite(t, l, cloneParentEvent{Gate: "exit"})
	cloneTraceWait(t, lPID, 17<<8)
	if err := unix.PidfdSendSignal(lFD, 0, nil, 0); err != syscall.ESRCH {
		t.Fatalf("consumed leader still individually signalable: %v", err)
	}
	send() // Same held leader kernel identity after its actual reap.
	if err := unix.PidfdSendSignal(hFD, 0, nil, groupProofScope); err != syscall.ESRCH {
		t.Fatalf("nonleader group request did not refuse: %v", err)
	}
	if err := unix.PidfdSendSignal(lFD, 0, nil, groupProofScope|2); err != syscall.EINVAL {
		t.Fatalf("conflicting signal scopes did not refuse: %v", err)
	}
	ping(h)
	ping(c)
	if err := unix.PidfdSendSignal(lFD, syscall.SIGKILL, nil, groupProofScope); err != nil {
		t.Fatal(err)
	}
	cloneTraceWait(t, hReady.PID, uint32(syscall.SIGKILL))
	if err := unix.PidfdSendSignal(lFD, 0, nil, groupProofScope); err != syscall.ESRCH {
		t.Fatalf("empty retained group identity did not refuse: %v", err)
	}
	ping(c)
	cloneParentWrite(t, c, cloneParentEvent{Gate: "exit"})
	cloneTraceWait(t, cPID, 0)
	groupProofNoChildren(t)
	return false
}

// Close alone does not interrupt an in-flight read on these blocking sockets.
func groupProofShutdown(control *os.File) error {
	raw, err := control.SyscallConn()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	if err != nil {
		return err
	}
	var shutdownErr error
	err = raw.Control(func(fd uintptr) {
		shutdownErr = unix.Shutdown(int(fd), unix.SHUT_RDWR)
	})
	closeErr := control.Close()
	// RawConn can return an unwrapped internal close error; File.Close supplies
	// the public sentinel after a previous revocation of this owned endpoint.
	if errors.Is(closeErr, os.ErrClosed) && shutdownErr == nil {
		return nil
	}
	return errors.Join(err, shutdownErr, closeErr)
}

func TestGroupProofShutdown(t *testing.T) {
	local, peer, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	defer peer.Close()
	result := make(chan error, 2)
	started := make(chan struct{}, 2)
	for _, endpoint := range []*os.File{local, peer} {
		go func() {
			started <- struct{}{}
			var b [1]byte
			_, readErr := endpoint.Read(b[:])
			result <- readErr
		}()
	}
	<-started
	<-started
	select {
	case err := <-result:
		t.Fatalf("unrevoked socket unexpectedly completed: %v", err)
	default:
	}
	if err := groupProofShutdown(local); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case err := <-result:
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				t.Fatalf("revoked read did not stop: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("socket revocation left a pending read")
		}
	}
	if err := groupProofShutdown(local); err != nil {
		t.Fatalf("repeated revocation: %v", err)
	}
}

func groupProofFD(t *testing.T, fd int) {
	t.Helper()
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatalf("pidfd not close-on-exec: %v", err)
	}
}

func groupProofNoChildren(t *testing.T) {
	t.Helper()
	var raw syscall.WaitStatus
	pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil)
	if pid != -1 || err != syscall.ECHILD {
		t.Fatalf("actual reap incomplete: pid=%d raw=%#x err=%v", pid, uint32(raw), err)
	}
}

func groupProofLeader(t *testing.T) {
	control, member := cloneTraceControl(3), cloneTraceControl(4)
	defer control.Close()
	defer member.Close()
	signal.Ignore(syscall.SIGTERM)
	cmd := groupProofCommand("H", os.Getenv("ZERB_GROUP_MODE"), os.Getenv("HOME"))
	cmd.ExtraFiles = []*os.File{member}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	fd := -1
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_PARENT, PidFD: &fd}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	defer cmd.Process.Release()
	member.Close()
	groupProofFD(t, fd)
	cloneParentWrite(t, control, cloneParentEvent{Gate: "born", PID: os.Getpid(),
		Parent: cmd.Process.Pid, SID: sessionID(), PGID: syscall.Getpgrp()})
	command := cloneParentRead(t, control).Gate
	if command == "register" {
		cloneTraceSendFD(t, control, fd)
		command = cloneParentRead(t, control).Gate
	}
	if command != "exit" {
		t.Fatal("leader has no exit instruction")
	}
	os.Exit(17)
}

func groupProofMember(t *testing.T, role string) {
	control := cloneTraceControl(3)
	defer control.Close()
	signals := make(chan os.Signal, 1)
	if role == "H" {
		signal.Notify(signals, syscall.SIGTERM)
		defer signal.Stop(signals)
	} else {
		// C must terminate on an unintended TERM; its final raw zero matters.
		signal.Reset(syscall.SIGTERM)
		if signal.Ignored(syscall.SIGTERM) {
			t.Fatal("control inherited an ignored TERM disposition")
		}
	}
	cloneParentWrite(t, control, cloneParentEvent{Gate: "ready", PID: os.Getpid(),
		Parent: os.Getppid(), SID: sessionID(), PGID: syscall.Getpgrp()})
	commands := make(chan string, 1)
	go func() {
		defer close(commands)
		for {
			payload, err := readFrame(control, 512)
			if errors.Is(err, io.EOF) {
				return
			}
			var command cloneParentEvent
			if err != nil || decodeStrict(payload, &command) != nil {
				commands <- "invalid"
				return
			}
			commands <- command.Gate
		}
	}()
	for {
		select {
		case <-signals:
			cloneParentWrite(t, control, cloneParentEvent{Gate: "signal"})
		case command, ok := <-commands:
			if !ok || command == "exit" {
				return
			}
			if command != "ping" {
				t.Fatal("invalid member command")
			}
			cloneParentWrite(t, control, cloneParentEvent{Gate: "pong"})
		}
	}
}
