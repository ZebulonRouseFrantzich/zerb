//go:build linux && amd64

package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-018/019/020: disposable same-image F->M and M=>S
// (natural F->S), followed by plain strace attachment. R is a cooperating
// surrogate subreaper, not evidence about arbitrary launch ancestry. This has
// narrow parent/socket provenance only: no nonce/role authentication, sealed
// image dispatch, G, cgroup, native descendants, full custody or race qualification.
func TestCloneParentTrace(t *testing.T) {
	if _, err := os.Stat("/usr/bin/strace"); err != nil {
		t.Skipf("unexercised: existing /usr/bin/strace unavailable: %v", err)
	}
	for _, mode := range []string{"live", "caller-death", "delayed-reaper", "denied-attachment"} {
		t.Run(mode, func(t *testing.T) {
			cmd := cloneTraceCommand("R", mode, t.TempDir())
			var output custodyOutput
			cmd.Stdout, cmd.Stderr = &output, &output
			err := cmd.Run()
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 77 {
				t.Skipf("unexercised capability/context: %s", output.Bytes())
			}
			if err != nil {
				t.Fatalf("isolated composed row: %v\n%s", err, output.Bytes())
			}
		})
	}
}

func cloneTraceCommand(role, mode, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCloneParentTraceChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_TRACE_ROLE="+role,
		"ZERB_TRACE_MODE="+mode, "ZERB_TRACE_ROOT="+root, "GORACE=atexit_sleep_ms=0")
	return cmd
}

// PR_GET_PDEATHSIG is per thread: inspect each managed initial exec thread before the
// testing package moves the helper onto a later runtime thread.
var cloneTraceInitialTID, cloneTraceInitialSignal = func() (int, syscall.Signal) {
	if role := os.Getenv("ZERB_TRACE_ROLE"); role == "M" || role == "payload" {
		return syscall.Gettid(), parentDeathSignal()
	}
	return 0, 0
}()

func TestCloneParentTraceChild(t *testing.T) {
	role := os.Getenv("ZERB_TRACE_ROLE")
	if role == "" {
		return
	}
	runtime.LockOSThread()
	unexercised := false
	switch role {
	case "R":
		unexercised = cloneTraceReaper(t)
	case "F":
		cloneTraceCaller(t)
	case "M":
		cloneTraceManaged(t)
	case "S":
		cloneTraceObserver(t)
	case "payload":
		if cloneTraceInitialTID != os.Getpid() || cloneTraceInitialSignal != syscall.SIGKILL {
			t.Fatal("ordinary exec lost initial-thread parent-death signal")
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("ZERB_TRACE_ROOT"), "payload-entered"), []byte("entered"), 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(17)
	default:
		t.Fatal("unknown composed role")
	}
	if t.Failed() {
		os.Exit(1)
	}
	if unexercised {
		os.Exit(77)
	}
	os.Exit(0)
}

// Linux v6.18 include/uapi/linux/pidfd.h: test-private 72-byte GET_INFO ABI.
// Always require the returned EXIT bit; a readable pidfd is not stored status.
type cloneTraceInfo struct {
	Mask, Cgroup                       uint64
	PID, TGID, PPID                    uint32
	RUID, RGID, EUID, EGID, SUID, SGID uint32
	FSUID, FSGID                       uint32
	Exit                               int32
	Core, Spare                        uint32
}

func cloneTraceGetInfo(fd int) (cloneTraceInfo, error) {
	info := cloneTraceInfo{Mask: 1<<0 | 1<<3}
	if unsafe.Sizeof(info) != 72 || unsafe.Offsetof(info.Exit) != 60 {
		return info, errors.New("unexpected pidfd info ABI")
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), 0xc048ff0b,
		uintptr(unsafe.Pointer(&info))) // #nosec G103 -- live 72-byte Linux amd64 UAPI object, checked above.
	if errno != 0 {
		return info, errno
	}
	return info, nil
}

func cloneTraceStored(t *testing.T, fd int, want uint32) {
	t.Helper()
	info, err := cloneTraceGetInfo(fd)
	if err != nil || info.Mask&(1<<3) == 0 || uint32(info.Exit) != want {
		t.Fatalf("stored EXIT absent or wrong: mask=%#x raw=%#x want=%#x err=%v", info.Mask, uint32(info.Exit), want, err)
	}
}

// A single-byte SCM_RIGHTS message precedes each framed message on this stream.
func cloneTraceSendFD(t *testing.T, control *os.File, fd int) {
	t.Helper()
	if err := unix.Sendmsg(int(control.Fd()), []byte{1}, unix.UnixRights(fd), nil, 0); err != nil {
		t.Fatal(err)
	}
}

func cloneTraceReceiveFD(t *testing.T, control *os.File) int {
	t.Helper()
	var payload [1]byte
	oob := make([]byte, unix.CmsgSpace(4))
	n, oobn, flags, _, err := unix.Recvmsg(int(control.Fd()), payload[:], oob, unix.MSG_CMSG_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || n != 1 || payload[0] != 1 || flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) != 0 || len(messages) != 1 {
		t.Fatal("invalid fixture descriptor message")
	}
	fds, err := unix.ParseUnixRights(&messages[0])
	if err != nil || len(fds) != 1 {
		t.Fatal("invalid fixture descriptor rights")
	}
	flagsFD, err := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	if err != nil || flagsFD&unix.FD_CLOEXEC == 0 {
		t.Fatal("received descriptor lacks CLOEXEC")
	}
	return fds[0]
}

func cloneTraceControl(fd uintptr) *os.File {
	control := os.NewFile(fd, "composed-control")
	syscall.CloseOnExec(int(fd))
	return control
}

func cloneTraceWait(t *testing.T, pid int, want uint32) {
	t.Helper()
	var raw syscall.WaitStatus
	for {
		got, err := syscall.Wait4(pid, &raw, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || got != pid || uint32(raw) != want {
			t.Fatalf("exact wait pid=%d raw=%#x want=%#x err=%v", got, uint32(raw), want, err)
		}
		return
	}
}

func cloneTraceReaper(t *testing.T) bool {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	probe, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pidfd open unavailable:", err)
		os.Exit(77)
	}
	_, probeErr := cloneTraceGetInfo(probe)
	unix.Close(probe)
	if probeErr != nil {
		fmt.Fprintln(os.Stderr, "pidfd GET_INFO unavailable:", probeErr)
		os.Exit(77)
	}
	root, mode := os.Getenv("ZERB_TRACE_ROOT"), os.Getenv("ZERB_TRACE_MODE")
	parent, child, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	defer child.Close()
	cmd := cloneTraceCommand("F", mode, root)
	cmd.ExtraFiles = []*os.File{child}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	fFD := -1
	cmd.SysProcAttr = &syscall.SysProcAttr{PidFD: &fFD}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	child.Close()
	defer syscall.Close(fFD)
	var mu sync.Mutex
	owned := []int{fFD}
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, fd := range owned {
			if fd >= 0 {
				_ = unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0)
			}
		}
		_ = cmd.Process.Kill()
		_ = parent.Close() // F's EOF closes its sole pre-attachment tokens.
	}
	done := make(chan struct{})
	watchdog := time.AfterFunc(15*time.Second, func() { cleanup(); close(done) })
	defer func() {
		if !watchdog.Stop() {
			<-done
			t.Error("composed fixture watchdog expired")
		}
		cleanup()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
		// F's pidfd remains held throughout orphan draining; no group guesses.
		deadline := time.Now().Add(3 * time.Second)
		for {
			var raw syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil)
			if err == syscall.ECHILD {
				break
			}
			if err != nil && err != syscall.EINTR {
				t.Errorf("drain composed orphans: %v", err)
				break
			}
			if pid == 0 {
				if time.Now().After(deadline) {
					t.Error("unresolved composed orphans after fixture cleanup")
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		for _, fd := range owned[1:] {
			unix.Close(fd)
		}
	}()
	if fFD < 0 {
		t.Fatal("F birth did not return an atomic pidfd")
	}
	mFD := cloneTraceReceiveFD(t, parent)
	mu.Lock()
	owned = append(owned, mFD)
	mu.Unlock()
	sFD := cloneTraceReceiveFD(t, parent)
	mu.Lock()
	owned = append(owned, sFD)
	mu.Unlock()
	registered := cloneParentRead(t, parent)
	if registered.Gate != "registered" || registered.PID <= 0 || registered.Parent <= 0 {
		t.Fatalf("missing held role registration: %+v", registered)
	}
	for i, fd := range []int{mFD, sFD} {
		wantPID := []int{registered.PID, registered.Parent}[i]
		info, err := cloneTraceGetInfo(fd)
		if err != nil || info.Mask&1 == 0 || info.Mask&(1<<3) != 0 || info.PID != uint32(wantPID) || info.TGID != uint32(wantPID) || info.PPID != uint32(cmd.Process.Pid) {
			t.Fatalf("preheld live role relation: %+v err=%v", info, err)
		}
	}
	cloneParentWrite(t, parent, cloneParentEvent{Gate: "inspect"})
	ready := cloneParentRead(t, parent)
	if ready.PID != registered.PID || ready.Parent != registered.Parent {
		t.Fatalf("attachment result changed registered identities: %+v", ready)
	}
	if ready.Gate == "refused" {
		if ready.Err != int(syscall.EPERM) || ready.Raw != 1<<8 || ready.PID <= 0 || ready.Parent <= 0 {
			t.Fatalf("invalid cleaned attach refusal: %+v", ready)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("refusal F cleanup failed: %v", err)
		}
		cloneTraceStored(t, mFD, uint32(syscall.SIGKILL))
		cloneTraceStored(t, sFD, 1<<8)
		cloneTraceNoPayloadOrChildren(t, root)
		if mode != "denied-attachment" {
			fmt.Fprintln(os.Stderr, "unexercised: exact strace PTRACE_SEIZE permission denial; owned M/S consumed, F zero, payload absent, no children")
			return true // Helper dispatcher checks deferred cleanup errors first.
		}
		return false
	}
	if ready.Gate != "attached" {
		t.Fatalf("observer did not attach; no release: %+v", ready)
	}
	if mode == "denied-attachment" {
		t.Fatal("competing-tracer negative row unexpectedly attached")
	}
	if mode == "live" {
		cloneParentWrite(t, parent, cloneParentEvent{Gate: "continue"})
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		cloneTraceStored(t, mFD, 17<<8)
		cloneTraceStored(t, sFD, 0)
	} else {
		if err := unix.PidfdSendSignal(fFD, syscall.SIGKILL, nil, 0); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err == nil || cmd.ProcessState.Sys().(syscall.WaitStatus) != syscall.WaitStatus(syscall.SIGKILL) {
			t.Fatalf("actual F death absent: %v", err)
		}
		observed := custodyObserve(t, ready.PID)
		custodyCheck(t, observed, ready.PID, "kill")
		if mode == "delayed-reaper" {
			// Two actual WNOWAIT observations straddle the missing EXIT checks;
			// this ordered interval is not a production reap deadline.
			for range 2 {
				info, err := cloneTraceGetInfo(mFD)
				if err != nil || info.Mask&(1<<3) != 0 {
					t.Fatalf("unconsumed M fabricated EXIT: %+v err=%v", info, err)
				}
				custodyCheck(t, custodyObserve(t, ready.PID), ready.PID, "kill")
			}
		}
		cloneTraceWait(t, ready.PID, uint32(syscall.SIGKILL))
		cloneTraceStored(t, mFD, uint32(syscall.SIGKILL))
		cloneTraceWait(t, ready.Parent, 0) // S survives F and completes separately.
		cloneTraceStored(t, sFD, 0)
	}
	marker, markerErr := os.ReadFile(filepath.Join(root, "payload-entered"))
	if mode == "live" {
		if markerErr != nil || string(marker) != "entered" {
			t.Fatal("released same-image payload did not enter")
		}
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		t.Fatal("held payload entered after ordered F death")
	}
	trace, err := os.ReadFile(filepath.Join(root, "trace"))
	if err != nil || len(trace) == 0 {
		t.Fatalf("missing observer trace: %v", err)
	}
	if mode == "live" && (!bytes.Contains(trace, []byte("execve(")) || !bytes.Contains(trace, []byte("payload-entered")) || !bytes.Contains(trace, []byte("exit_group(17"))) {
		t.Fatal("trace omitted payload exec, entry or raw exit")
	}
	if mode != "live" && !bytes.Contains(trace, []byte("killed by SIGKILL")) {
		t.Fatal("trace omitted actual M SIGKILL")
	}
	return false
}

func cloneTraceNoPayloadOrChildren(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "payload-entered")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refusal payload absence not established: %v", err)
	}
	var raw syscall.WaitStatus
	pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil)
	if pid != -1 || err != syscall.ECHILD {
		t.Fatalf("refusal retained an unconsumed child: pid=%d raw=%#x err=%v", pid, uint32(raw), err)
	}
}

func cloneTraceCaller(t *testing.T) {
	control := cloneTraceControl(3)
	defer control.Close()
	parent, child, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	defer child.Close()
	observer, observerChild, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	defer observerChild.Close()
	root, mode := os.Getenv("ZERB_TRACE_ROOT"), os.Getenv("ZERB_TRACE_MODE")
	observerError, err := os.OpenFile(filepath.Join(root, "observer-stderr"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer observerError.Close()
	cmd := cloneTraceCommand("M", mode, root)
	cmd.ExtraFiles = []*os.File{child, observerChild, observerError}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	mFD := -1
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL, PidFD: &mFD}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	child.Close()
	observerChild.Close()
	defer syscall.Close(mFD)
	defer func() {
		_ = unix.PidfdSendSignal(mFD, syscall.SIGKILL, nil, 0)
		_ = parent.Close()
		_ = observer.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	}()
	if mFD < 0 {
		_ = cmd.Process.Kill()
		t.Fatal("M birth did not return an atomic pidfd")
	}
	cloneTraceSendFD(t, control, mFD)
	sFD := cloneTraceReceiveFD(t, parent)
	defer syscall.Close(sFD)
	defer unix.PidfdSendSignal(sFD, syscall.SIGKILL, nil, 0)
	cloneTraceSendFD(t, control, sFD)
	created := cloneParentRead(t, parent)
	seen := cloneParentRead(t, observer)
	if created.Gate != "created" || seen.Gate != "observer" || created.PID != seen.PID ||
		seen.Parent != os.Getpid() || seen.SID != cmd.Process.Pid || seen.PGID != seen.PID || seen.Signal != 0 || !seen.Accepted {
		t.Fatalf("wrong pre-attach observer relation: created=%+v observer=%+v", created, seen)
	}
	cloneParentWrite(t, control, cloneParentEvent{Gate: "registered", PID: cmd.Process.Pid, Parent: seen.PID})
	if cloneParentRead(t, control).Gate != "inspect" {
		t.Fatal("missing pre-attachment custody acknowledgement")
	}
	if mode == "denied-attachment" {
		// A real competing tracer causes kernel EPERM without changing policy.
		// If this ancestor itself is denied, S must still prove its own denial.
		if err := unix.PtraceSeize(cmd.Process.Pid); err != nil && err != syscall.EPERM {
			t.Fatal(err)
		}
	}
	cloneParentWrite(t, observer, cloneParentEvent{Gate: "attach", PID: cmd.Process.Pid})
	if err := cloneTraceAttached(cmd.Process.Pid, seen.PID); err != nil {
		if _, err := observerError.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		diagnostic, readErr := io.ReadAll(io.LimitReader(observerError, 1025))
		if !errors.Is(err, errCloneTraceObserverExited) || readErr != nil || !cloneTracePermissionDenied(diagnostic, cmd.Process.Pid) {
			t.Fatalf("attachment failed without verified permission refusal: %v (stderr read=%v, bytes=%d)", err, readErr, len(diagnostic))
		}
		custodyCheck(t, custodyObserve(t, seen.PID), seen.PID, "1")
		if err := unix.PidfdSendSignal(mFD, syscall.SIGKILL, nil, 0); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err == nil || cmd.ProcessState.Sys().(syscall.WaitStatus) != syscall.WaitStatus(syscall.SIGKILL) {
			t.Fatalf("held M refusal cleanup did not retain raw SIGKILL: %v", err)
		}
		cloneTraceWait(t, seen.PID, 1<<8)
		cloneTraceStored(t, mFD, uint32(syscall.SIGKILL))
		cloneTraceStored(t, sFD, 1<<8)
		cloneTraceNoPayloadOrChildren(t, root)
		cloneParentWrite(t, control, cloneParentEvent{Gate: "refused", PID: cmd.Process.Pid, Parent: seen.PID, Err: int(syscall.EPERM), Raw: 1 << 8})
		return
	}
	cloneParentWrite(t, control, cloneParentEvent{Gate: "attached", PID: cmd.Process.Pid, Parent: seen.PID})
	if cloneParentRead(t, control).Gate != "continue" {
		t.Fatal("missing R continuation")
	}
	cloneParentWrite(t, parent, cloneParentEvent{Gate: "continue"})
	if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 17 {
		t.Fatalf("payload raw exit 17 absent: %v", err)
	}
	cloneTraceWait(t, seen.PID, 0)
}

func cloneTraceManaged(t *testing.T) {
	control := cloneTraceControl(3)
	defer control.Close()
	observer := cloneTraceControl(4)
	defer observer.Close()
	observerError := cloneTraceControl(5)
	defer observerError.Close()
	parent, err := validateInternalControl(3)
	if err != nil || parent != os.Getppid() || sessionID() != os.Getpid() || cloneTraceInitialTID != os.Getpid() || cloneTraceInitialSignal != syscall.SIGKILL {
		t.Fatalf("wrong direct F/M setup: %v", err)
	}
	// The initial exec thread and the locked release/exec thread can differ.
	if err := establishStageParentDeath(3, parent, parentDeathSignal(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd := cloneTraceCommand("S", os.Getenv("ZERB_TRACE_MODE"), os.Getenv("ZERB_TRACE_ROOT"))
	sFD := -1
	pid, err := syscall.ForkExec(os.Args[0], cmd.Args, &syscall.ProcAttr{
		Env: cmd.Env, Files: []uintptr{0, 1, observerError.Fd(), observer.Fd()},
		Sys: &syscall.SysProcAttr{Cloneflags: syscall.CLONE_PARENT, Setpgid: true, PidFD: &sFD},
	})
	observer.Close()
	observerError.Close()
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || sFD < 0 {
		t.Fatal("S birth did not return an atomic pidfd")
	}
	defer syscall.Close(sFD)
	cloneTraceSendFD(t, control, sFD)
	cloneParentWrite(t, control, cloneParentEvent{Gate: "created", PID: pid})
	payloadFrame, err := readFrame(control, 512)
	if errors.Is(err, io.EOF) {
		// EOF can precede parent-death delivery during F exit. Refuse the
		// payload and let the actual direct SIGKILL determine M's status.
		for {
			_ = syscall.Pause()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var continuation cloneParentEvent
	if err := decodeStrict(payloadFrame, &continuation); err != nil || continuation.Gate != "continue" {
		t.Fatalf("missing sole F continuation: %v", err)
	}
	if err := establishStageParentDeath(3, parent, parentDeathSignal(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	payload := cloneTraceCommand("payload", os.Getenv("ZERB_TRACE_MODE"), os.Getenv("ZERB_TRACE_ROOT"))
	if err := syscall.Exec(os.Args[0], payload.Args, payload.Env); err != nil {
		t.Fatal(err)
	}
}

func cloneTraceObserver(t *testing.T) {
	control := cloneTraceControl(3)
	defer control.Close()
	parent, err := validateInternalControl(3)
	cloneParentWrite(t, control, cloneParentEvent{Gate: "observer", PID: os.Getpid(), Parent: os.Getppid(),
		SID: sessionID(), PGID: syscall.Getpgrp(), Signal: int(parentDeathSignal()), Accepted: err == nil && parent == os.Getppid()})
	if err != nil {
		t.Fatal(err)
	}
	attach := cloneParentRead(t, control)
	if attach.Gate != "attach" || attach.PID != sessionID() || syscall.Getpgrp() != os.Getpid() || parentDeathSignal() != 0 {
		t.Fatal("invalid observer attachment relation")
	}
	log := filepath.Join(os.Getenv("ZERB_TRACE_ROOT"), "trace")
	file, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"/usr/bin/strace", "-f", "-qq", "-v", "-yy", "-s", "4096", "-e", "trace=all", "-o", log, "-p", strconv.Itoa(attach.PID)}
	if err := syscall.Exec(args[0], args, isolatedEnv(os.Getenv("ZERB_TRACE_ROOT"), "")); err != nil {
		t.Fatal(err)
	}
}

var errCloneTraceObserverExited = errors.New("observer exited before attachment")

func cloneTracePermissionDenied(diagnostic []byte, pid int) bool {
	want := fmt.Sprintf(": attach: ptrace(PTRACE_SEIZE, %d): Operation not permitted\n", pid)
	// strace versions label diagnostics with either argv[0] or its basename.
	return bytes.Equal(diagnostic, []byte("/usr/bin/strace"+want)) || bytes.Equal(diagnostic, []byte("strace"+want))
}

// Check every currently held Go thread, including the locked exec thread.
// shortcut: this ordered stable snapshot is not adversarial thread-churn proof;
// qualify descriptor/thread races separately before production use.
func cloneTraceAttached(pid, observer int) error {
	installed, err := os.Open("/usr/bin/strace")
	if err != nil {
		return err
	}
	defer installed.Close()
	expected, err := descriptorContentIdentity(installed, true)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	path := fmt.Sprintf("/proc/%d/task", pid)
	for time.Now().Before(deadline) {
		var terminal unix.Siginfo
		if err := unix.Waitid(unix.P_PID, observer, &terminal, unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil); err != nil && err != syscall.EINTR {
			return err
		}
		if terminal.Signo != 0 {
			return errCloneTraceObserverExited
		}
		threads, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		attached := len(threads) > 0
		for _, thread := range threads {
			status, err := os.ReadFile(filepath.Join(path, thread.Name(), "status"))
			if err != nil || !strings.Contains(string(status), "TracerPid:\t"+strconv.Itoa(observer)+"\n") {
				attached = false
				break
			}
		}
		after, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(after) != len(threads) {
			attached = false
		} else {
			for i := range threads {
				if threads[i].Name() != after[i].Name() {
					attached = false
				}
			}
		}
		if attached {
			image, err := os.Open(fmt.Sprintf("/proc/%d/exe", observer))
			if err == nil {
				actual, identityErr := descriptorContentIdentity(image, true)
				closeErr := image.Close()
				sid, sidErr := unix.Getsid(observer)
				pgid, pgidErr := syscall.Getpgid(observer)
				if identityErr == nil && closeErr == nil && sameIdentity(actual, expected) && sidErr == nil && sid == pid && pgidErr == nil && pgid == observer {
					return nil
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("exact plain strace observer did not attach all held threads; target held")
}
