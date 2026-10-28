//go:build linux && amd64

package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-018/019/020: all five roles are authenticated and held.
// This private wire exercises no managed entry, strace, cgroup or armed guard.
func TestAuthenticatedHeldAssembly(t *testing.T) {
	for _, mode := range []string{"held", "foreign-sender", "wrong-parent", "wrong-nonce", "replayed-phase", "substituted-backup", "interrupted-S", "interrupted-G", "owner-loss", "queued-owner-ack", "marker-fd"} {
		t.Run(mode, func(t *testing.T) {
			cmd := heldAssemblyCommand("F", mode, t.TempDir())
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			err := cmd.Run()
			heldAssemblyHarnessWait(t, cmd, mode, "F")
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 77 {
				t.Skip("unexercised retained-status capability")
			}
			if err != nil {
				t.Fatalf("held assembly: %v", err)
			}
		})
	}
}

func heldAssemblyCommand(role, mode, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHeldAssemblyChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_ASSEMBLY_ROLE="+role, "ZERB_ASSEMBLY_MODE="+mode, "ZERB_ASSEMBLY_ROOT="+root, "GORACE=atexit_sleep_ms=0")
	return cmd
}

func TestHeldAssemblyChild(t *testing.T) {
	role := os.Getenv("ZERB_ASSEMBLY_ROLE")
	if role == "" {
		return
	}
	runtime.LockOSThread() // F's creator and M's creator/release thread live until exit.
	unexercised := false
	switch role {
	case "F":
		unexercised = heldAssemblyCaller(t)
	case "M", "O", "S", "G":
		heldAssemblyChild(t, role)
	default:
		t.Fatal("unknown held assembly role")
	}
	if t.Failed() {
		os.Exit(1)
	}
	if unexercised {
		os.Exit(77)
	}
	os.Exit(0)
}

const heldAssemblyTracePrefix = "ZERB_ASSEMBLY_TRACE "

var heldAssemblyTraceSequence atomic.Uint64

type heldAssemblyTraceRecord struct {
	Role      string  `json:"role"`
	Mode      string  `json:"mode"`
	Sequence  uint64  `json:"sequence"`
	Event     string  `json:"event"`
	Phase     string  `json:"phase"`
	Subject   string  `json:"subject"`
	RawStatus *uint32 `json:"raw_status,omitempty"`
}

func heldAssemblyTraceRole(value string) bool {
	switch value {
	case "F", "M", "O", "S", "G", "harness", "fixture":
		return true
	}
	return false
}

func heldAssemblyTraceMode(value string) bool {
	switch value {
	case "held", "foreign-sender", "wrong-parent", "wrong-nonce", "replayed-phase", "substituted-backup", "interrupted-S", "interrupted-G", "owner-loss", "queued-owner-ack", "marker-fd":
		return true
	}
	return false
}

func heldAssemblyTracePhase(value string) string {
	switch value {
	case "hello", "scope", "scoped", "birth", "birth-interrupted", "birth-refused", "register", "registered", "refused", "retire", "retired", "arm-request", "arm-refused", "stop", "stop-G", "child-status", "parent-scope", "parent-scoped", "parent-register", "parent-registered", "arm", "startup", "cleanup", "status", "unknown":
		return value
	}
	return "unknown"
}

func heldAssemblyTraceSubject(value string) string {
	if heldAssemblyTraceRole(value) && value != "harness" && value != "fixture" {
		return value
	}
	return "unknown"
}

func heldAssemblyTraceBytes(record heldAssemblyTraceRecord) ([]byte, error) {
	if !heldAssemblyTraceRole(record.Role) || !heldAssemblyTraceMode(record.Mode) || record.Sequence == 0 ||
		record.Phase != heldAssemblyTracePhase(record.Phase) || record.Subject != heldAssemblyTraceSubject(record.Subject) {
		return nil, errors.New("invalid closed assembly trace field")
	}
	statusEvent := false
	switch record.Event {
	case "wait", "retained-exit":
		statusEvent = true
	case "send", "received", "birth", "refusal", "error-send", "error-receive", "error-live-before", "error-live-after", "error-wait", "error-retained-exit", "error-birth":
	default:
		return nil, errors.New("invalid closed assembly trace event")
	}
	if statusEvent != (record.RawStatus != nil) || record.RawStatus != nil && *record.RawStatus > 65535 {
		return nil, errors.New("invalid assembly trace status")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, errors.New("assembly trace encoding failed")
	}
	line := append([]byte(heldAssemblyTracePrefix), payload...)
	line = append(line, '\n')
	if len(line) > 512 {
		return nil, errors.New("assembly trace record exceeds bound")
	}
	return line, nil
}

func heldAssemblyWriteTrace(fd int, record heldAssemblyTraceRecord) error {
	line, err := heldAssemblyTraceBytes(record)
	if err != nil {
		return err
	}
	n, err := unix.Write(fd, line) // One bounded write; no buffering or dynamic error text enters the trace.
	if n != len(line) {
		err = errors.Join(err, io.ErrShortWrite)
	}
	return err
}

func heldAssemblyRecord(event, phase, subject string, raw *uint32) error {
	return heldAssemblyWriteTrace(2, heldAssemblyTraceRecord{
		Role: os.Getenv("ZERB_ASSEMBLY_ROLE"), Mode: os.Getenv("ZERB_ASSEMBLY_MODE"), Sequence: heldAssemblyTraceSequence.Add(1),
		Event: event, Phase: heldAssemblyTracePhase(phase), Subject: heldAssemblyTraceSubject(subject), RawStatus: raw,
	})
}

// The diagnostic observer is outside the F/M/O/S/G authority tree. Its wait
// record describes only the actual child consumed by this test harness.
func heldAssemblyHarnessRecord(t *testing.T, event, phase, subject string, raw *uint32, mode string) {
	t.Helper()
	record := heldAssemblyTraceRecord{Role: "harness", Mode: mode, Sequence: heldAssemblyTraceSequence.Add(1), Event: event, Phase: heldAssemblyTracePhase(phase), Subject: heldAssemblyTraceSubject(subject), RawStatus: raw}
	if err := heldAssemblyWriteTrace(2, record); err != nil {
		t.Fatalf("harness trace write failed: %v", err)
	}
}

func heldAssemblyHarnessWait(t *testing.T, cmd *exec.Cmd, mode, subject string) {
	t.Helper()
	if cmd.ProcessState == nil {
		heldAssemblyHarnessRecord(t, "error-wait", "status", subject, nil, mode)
		t.Fatal("harness child wait unavailable")
	}
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		heldAssemblyHarnessRecord(t, "error-wait", "status", subject, nil, mode)
		t.Fatal("harness child wait status unavailable")
	}
	raw := uint32(status)
	heldAssemblyHarnessRecord(t, "wait", "status", subject, &raw, mode)
}

func heldAssemblyTrace(t *testing.T, event, phase, subject string, raw *uint32) {
	t.Helper()
	if err := heldAssemblyRecord(event, phase, subject, raw); err != nil {
		t.Fatalf("assembly trace write failed: %v", err)
	}
}

// These local observations log returned kernel facts before comparing expected
// values. Expected raw status is never substituted for an actual observation.
func heldAssemblyWait(t *testing.T, pid int, want uint32, subject string) uint32 {
	t.Helper()
	for {
		var status syscall.WaitStatus
		got, err := syscall.Wait4(pid, &status, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || got != pid {
			heldAssemblyTrace(t, "error-wait", "status", subject, nil)
			t.Fatalf("exact held wait failed: %v", err)
		}
		raw := uint32(status)
		heldAssemblyTrace(t, "wait", "status", subject, &raw)
		if raw != want {
			t.Fatalf("exact held wait raw=%#x want=%#x", raw, want)
		}
		return raw
	}
}

func heldAssemblyStored(t *testing.T, fd int, want uint32, subject string) {
	t.Helper()
	info, err := cloneTraceGetInfo(fd)
	if err != nil || info.Mask&(1<<3) == 0 {
		heldAssemblyTrace(t, "error-retained-exit", "status", subject, nil)
		t.Fatalf("held retained EXIT unavailable: %v", err)
	}
	raw := uint32(info.Exit)
	heldAssemblyTrace(t, "retained-exit", "status", subject, &raw)
	if raw != want {
		t.Fatalf("held retained EXIT raw=%#x want=%#x", raw, want)
	}
}

func heldAssemblySend(t *testing.T, control *os.File, frame assemblyMessage, phase string, rights ...int) {
	t.Helper()
	frame.Phase = phase
	if err := sendAssemblyPacket(control, frame, rights...); err != nil {
		heldAssemblyTrace(t, "error-send", phase, frame.Role, nil)
		t.Fatalf("assembly send %s: %v", phase, err)
	}
	heldAssemblyTrace(t, "send", phase, frame.Role, nil)
}

// The transport authenticates the writer; the held pidfd additionally refuses
// queued packets from a terminal role, even before its natural wait consumes it.
func heldAssemblyRead(t *testing.T, control *os.File, sender, senderFD, rights int) (assemblyMessage, []*os.File, error) {
	traceError := func(event string) error {
		err := heldAssemblyRecord(event, "unknown", "unknown", nil)
		if err != nil {
			t.Errorf("assembly trace write failed: %v", err)
		}
		return err
	}
	if err := heldAssemblyLive(senderFD, sender, -1); err != nil {
		return assemblyMessage{}, nil, errors.Join(err, traceError("error-live-before"))
	}
	frame, files, err := receiveAssemblyPacket(control, sender, uint32(os.Geteuid()), rights)
	if err != nil {
		for _, file := range files {
			err = errors.Join(err, file.Close())
		}
		return assemblyMessage{}, nil, errors.Join(err, traceError("error-receive"))
	}
	if err := heldAssemblyLive(senderFD, sender, -1); err != nil {
		for _, file := range files {
			err = errors.Join(err, file.Close())
		}
		return assemblyMessage{}, nil, errors.Join(err, traceError("error-live-after"))
	}
	if err := heldAssemblyRecord("received", frame.Phase, frame.Role, nil); err != nil {
		for _, file := range files {
			err = errors.Join(err, file.Close())
		}
		t.Errorf("assembly trace write failed: %v", err)
		return assemblyMessage{}, nil, err
	}
	return frame, files, nil
}

func heldAssemblyReceive(t *testing.T, control *os.File, sender, senderFD, rights int) (assemblyMessage, []*os.File) {
	t.Helper()
	frame, files, err := heldAssemblyRead(t, control, sender, senderFD, rights)
	if err != nil {
		heldAssemblyClose(t, files)
		t.Fatalf("assembly receive from held live sender: %v", err)
	}
	return frame, files
}

func heldAssemblyExpect(t *testing.T, control *os.File, sender, senderFD int, want assemblyMessage, phase string) {
	t.Helper()
	want.Phase = phase
	got, files := heldAssemblyReceive(t, control, sender, senderFD, 0)
	heldAssemblyClose(t, files)
	if got != want {
		heldAssemblyTrace(t, "refusal", phase, want.Role, nil)
		t.Fatalf("assembly phase %s refused or unbound", phase)
	}
}

func heldAssemblyRefuse(t *testing.T, control *os.File, sender, senderFD int, frame assemblyMessage) {
	t.Helper()
	heldAssemblyTrace(t, "refusal", "refused", frame.Role, nil)
	heldAssemblySend(t, control, frame, "refused")
	_, files, err := heldAssemblyRead(t, control, sender, senderFD, 0)
	heldAssemblyClose(t, files)
	if err == nil {
		t.Fatal("failed role accepted a subsequent assembly packet")
	}
}

func heldAssemblySubreaper(t *testing.T) {
	t.Helper()
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	var value int32
	// #nosec G103 -- PR_GET_CHILD_SUBREAPER writes exactly one live Linux int32.
	_, _, errno := unix.Syscall6(unix.SYS_PRCTL, unix.PR_GET_CHILD_SUBREAPER, uintptr(unsafe.Pointer(&value)), 0, 0, 0, 0)
	if errno != 0 || value != 1 {
		t.Fatalf("disposable subreaper attribute unavailable: %v", errno)
	}
}

func heldAssemblyClose(t *testing.T, files []*os.File) {
	t.Helper()
	for _, file := range files {
		if file == nil {
			continue
		}
		if err := file.Close(); err != nil {
			t.Errorf("close assembly right: %v", err)
		}
	}
}

func heldAssemblyNonce(t *testing.T) string {
	t.Helper()
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(nonce[:])
}

func heldAssemblyTuple(ids [5]int, complete bool) bool {
	n := 3
	if complete {
		n = 5
	}
	for i, pid := range ids {
		if i >= n {
			if pid != 0 {
				return false
			}
			continue
		}
		if pid <= 0 {
			return false
		}
		for j := 0; j < i; j++ {
			if ids[j] == pid {
				return false
			}
		}
	}
	return true
}

func heldAssemblyLive(fd, pid, parent int) error {
	info, err := cloneTraceGetInfo(fd)
	if err != nil {
		return err
	}
	if info.Mask&1 == 0 || info.Mask&(1<<3) != 0 || int(info.PID) != pid || int(info.TGID) != pid ||
		parent > 0 && int(info.PPID) != parent {
		return errors.New("held role identity differs")
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(poll, 0)
	if err != nil {
		return err
	}
	if n != 0 || poll[0].Revents != 0 {
		return errors.New("held role terminal or unavailable")
	}
	return nil
}

func heldAssemblyRole(fd, pid, parent, sid, pgid int) error {
	if err := heldAssemblyLive(fd, pid, parent); err != nil {
		return err
	}
	actualSID, sidErr := unix.Getsid(pid)
	actualPGID, pgidErr := unix.Getpgid(pid)
	if sidErr != nil || pgidErr != nil || actualSID != sid || actualPGID != pgid || !sameExecutable(pid, os.Getpid()) {
		return errors.New("held role session/group/image differs")
	}
	return nil
}

func heldAssemblyPair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	parent, child, err := assemblySocketPair()
	if err != nil {
		t.Fatal(err)
	}
	return parent, child
}

// Every birth executes the already opened image. Extra slots are restored to
// CLOEXEC by the child before it can perform its own next birth.
func heldAssemblyStart(t *testing.T, role, mode, root string, files []*os.File, attr *syscall.SysProcAttr) (int, int) {
	t.Helper()
	cmd := heldAssemblyCommand(role, mode, root)
	cmd.Path, cmd.Args[0] = "/proc/self/fd/4", "/proc/self/fd/4"
	cmd.ExtraFiles, cmd.Stdout, cmd.Stderr = files, os.Stdout, os.Stderr
	fd := -1
	attr.PidFD = &fd
	cmd.SysProcAttr = attr
	if err := cmd.Start(); err != nil {
		heldAssemblyTrace(t, "error-birth", "startup", role, nil)
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		t.Fatal(err)
	}
	if fd < 0 {
		t.Fatal("atomic birth capability absent")
	}
	groupProofFD(t, fd)
	if traceErr := heldAssemblyRecord("birth", "startup", role, nil); traceErr != nil {
		// Logging failure precedes handoff: revoke the exact birth capability, and
		// consume only a natural child. F still owns CLONE_PARENT S's actual wait.
		signalErr := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0)
		if signalErr == syscall.ESRCH {
			signalErr = nil
		}
		var waitErr error
		if signalErr == nil && attr.Cloneflags&syscall.CLONE_PARENT == 0 {
			var raw syscall.WaitStatus
			for {
				var got int
				got, waitErr = syscall.Wait4(pid, &raw, 0, nil)
				if waitErr == syscall.EINTR {
					continue
				}
				if waitErr == nil && got != pid {
					waitErr = errors.New("birth diagnostic cleanup wait differs")
				}
				break
			}
		}
		closeErr := unix.Close(fd)
		t.Fatalf("birth trace write failed: %v", errors.Join(traceErr, signalErr, waitErr, closeErr))
	}
	return pid, fd
}

func heldAssemblyCaller(t *testing.T) bool {
	root, mode := os.Getenv("ZERB_ASSEMBLY_ROOT"), os.Getenv("ZERB_ASSEMBLY_MODE")
	rootFD, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		t.Fatal(err)
	}
	rootFile := os.NewFile(uintptr(rootFD), "root-capability")
	defer heldAssemblyClose(t, []*os.File{rootFile})
	_, err = cloneTraceGetInfo(rootFD)
	if err == syscall.ENOTTY || err == syscall.EINVAL || err == syscall.ENOSYS {
		fmt.Fprintln(os.Stderr, "retained-status ioctl unavailable before held birth:", err)
		return true
	}
	if err != nil {
		t.Fatal(err)
	}
	heldAssemblySubreaper(t)
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	defer self.Close()
	image, err := descriptorContentIdentity(self, true)
	if err != nil {
		t.Fatal(err)
	}
	m, mc := heldAssemblyPair(t)
	o, oc := heldAssemblyPair(t)
	s, sc := heldAssemblyPair(t)
	g, gc := heldAssemblyPair(t)
	controls := []*os.File{m, o, s, g}
	defer func() { heldAssemblyClose(t, []*os.File{mc, oc, sc, gc}) }()
	var foreign *os.File
	if mode == "foreign-sender" {
		fd, err := unix.FcntlInt(gc.Fd(), unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		foreign = os.NewFile(uintptr(fd), "negative-foreign-G-peer")
		defer foreign.Close()
	}
	var mu sync.Mutex
	owned := make(map[int]int)
	waited := make(map[int]uint32)
	subjects := make(map[int]string)
	wait := func(pid int, raw uint32) { waited[pid] = heldAssemblyWait(t, pid, raw, subjects[pid]) }
	add := func(pid, fd int) { mu.Lock(); owned[pid] = fd; mu.Unlock() }
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, control := range controls {
			if err := groupProofShutdown(control); err != nil {
				t.Errorf("held channel revocation: %v", err)
			}
		}
		for _, fd := range owned {
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil && err != syscall.ESRCH {
				t.Errorf("held cleanup capability: %v", err)
			}
		}
	}
	expired := make(chan struct{})
	watchdog := time.AfterFunc(20*time.Second, func() { cleanup(); close(expired) })
	defer func() {
		if !watchdog.Stop() {
			<-expired
			t.Error("held assembly watchdog expired")
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
			if waitErr != nil || pid == 0 && time.Now().After(deadline) {
				t.Errorf("held cleanup unresolved: %v", waitErr)
				if err := heldAssemblyRecord("error-wait", "cleanup", "unknown", nil); err != nil {
					t.Errorf("cleanup trace write failed: %v", err)
				}
				break
			}
			if pid == 0 {
				time.Sleep(time.Millisecond)
				continue
			}
			observedRaw := uint32(raw)
			waited[pid] = observedRaw
			if err := heldAssemblyRecord("wait", "cleanup", subjects[pid], &observedRaw); err != nil {
				t.Errorf("cleanup trace write failed: %v", err)
			}
		}
		// A registered G may already have been consumed by O. ECHILD alone cannot
		// supply that fact; require retained EXIT from every capability we accepted.
		retained := make(map[int]uint32, len(owned))
		for pid, fd := range owned {
			info, err := cloneTraceGetInfo(fd)
			if err != nil || info.Mask&(1<<3) == 0 {
				t.Errorf("held cleanup retained EXIT unresolved: %v", err)
				if traceErr := heldAssemblyRecord("error-retained-exit", "cleanup", subjects[pid], nil); traceErr != nil {
					t.Errorf("cleanup trace write failed: %v", traceErr)
				}
			} else {
				retained[pid] = uint32(info.Exit)
				observedRaw := retained[pid]
				if err := heldAssemblyRecord("retained-exit", "cleanup", subjects[pid], &observedRaw); err != nil {
					t.Errorf("cleanup trace write failed: %v", err)
				}
				if raw, ok := waited[pid]; ok && retained[pid] != raw {
					t.Error("retained EXIT differs from F actual wait")
				}
			}
			if err := unix.Close(fd); err != nil {
				t.Errorf("close held status: %v", err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "entered")); !errors.Is(err, os.ErrNotExist) {
			t.Error("held assembly crossed managed entry")
		}
	}()
	mFiles := []*os.File{mc, self, sc, rootFile}
	oFiles := []*os.File{oc, self, gc, rootFile}
	var marker *os.File
	if mode == "marker-fd" {
		marker, err = os.CreateTemp(root, "inherited-marker-")
		if err != nil {
			t.Fatal(err)
		}
		defer marker.Close()
		if _, err := marker.WriteString("disposable-inherited-marker"); err != nil {
			t.Fatal(err)
		}
		mFiles = append(mFiles, nil, marker)
		oFiles = append(oFiles, nil, marker)
	}
	mPID, mFD := heldAssemblyStart(t, "M", mode, root, mFiles, &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL})
	add(mPID, mFD)
	subjects[mPID] = "M"
	if err := mc.Close(); err != nil {
		t.Fatal(err)
	}
	mc = nil
	if err := sc.Close(); err != nil {
		t.Fatal(err)
	}
	sc = nil
	oPID, oFD := heldAssemblyStart(t, "O", mode, root, oFiles, &syscall.SysProcAttr{Setsid: true})
	add(oPID, oFD)
	subjects[oPID] = "O"
	if err := oc.Close(); err != nil {
		t.Fatal(err)
	}
	oc = nil
	if err := gc.Close(); err != nil {
		t.Fatal(err)
	}
	gc = nil
	ids := [5]int{os.Getpid(), mPID, oPID, 0, 0}
	bind := func(control *os.File, role string, pid, fd int) assemblyMessage {
		if err := heldAssemblyRole(fd, pid, ids[0], pid, pid); err != nil {
			t.Fatal(err)
		}
		hello, files := heldAssemblyReceive(t, control, pid, fd, 0)
		heldAssemblyClose(t, files)
		if hello.Phase != "hello" || hello.Role != role || !validNonce(hello.Nonce) || hello.IDs != [5]int{} || hello.Image != image || hello.Armed || hello.Raw != 0 || hello.ChildPID != 0 {
			t.Fatal("invalid initial role challenge")
		}
		frame := assemblyMessage{Role: role, Nonce: hello.Nonce, IDs: ids, Image: image}
		heldAssemblySend(t, control, frame, "scope")
		heldAssemblyExpect(t, control, pid, fd, frame, "scoped")
		return frame
	}
	mf := bind(m, "M", mPID, mFD)
	of := bind(o, "O", oPID, oFD)
	if mf.Nonce == of.Nonce {
		t.Fatal("initial role nonce reused")
	}
	failed, registered := false, false
	fail := func() {
		failed = true
		if registered {
			t.Fatal("failed registration became complete")
		}
	}
	heldAssemblySend(t, m, mf, "birth")
	if mode == "interrupted-S" {
		heldAssemblyExpect(t, m, mPID, mFD, mf, "birth-interrupted")
		fail()
		return false
	}
	birth, rights := heldAssemblyReceive(t, m, mPID, mFD, 1)
	if birth.Phase != "birth" || birth.Role != "M" || birth.Nonce != mf.Nonce || birth.IDs != ids || birth.Image != image || birth.ChildPID <= 0 || birth.Raw != 0 || birth.Armed {
		heldAssemblyClose(t, rights)
		t.Fatal("unbound S birth")
	}
	sPID := birth.ChildPID
	if err := heldAssemblyRole(int(rights[0].Fd()), sPID, ids[0], mPID, sPID); err != nil {
		heldAssemblyClose(t, rights)
		t.Fatal(err)
	}
	sFD, err := unix.FcntlInt(rights[0].Fd(), unix.F_DUPFD_CLOEXEC, 0)
	heldAssemblyClose(t, rights)
	if err != nil {
		t.Fatal(err)
	}
	add(sPID, sFD)
	subjects[sPID] = "S"
	heldAssemblySend(t, o, of, "birth")
	if mode == "interrupted-G" {
		heldAssemblyExpect(t, o, oPID, oFD, of, "birth-interrupted")
		fail()
		return false
	}
	birth, rights = heldAssemblyReceive(t, o, oPID, oFD, 1)
	if mode == "wrong-parent" {
		if birth.Phase != "birth-refused" || birth.Nonce != of.Nonce || birth.Role != "O" || birth.Raw != 23<<8 || birth.ChildPID <= 0 {
			heldAssemblyClose(t, rights)
			t.Fatal("wrong-parent refusal absent")
		}
		defer heldAssemblyClose(t, rights)
		heldAssemblyStored(t, int(rights[0].Fd()), birth.Raw, "G")
		fail()
		return false
	}
	if birth.Phase != "birth" || birth.Role != "O" || birth.Nonce != of.Nonce || birth.IDs != ids || birth.Image != image || birth.ChildPID <= 0 || birth.Raw != 0 || birth.Armed {
		heldAssemblyClose(t, rights)
		t.Fatal("unbound G birth")
	}
	gPID := birth.ChildPID
	if err := heldAssemblyRole(int(rights[0].Fd()), gPID, oPID, oPID, gPID); err != nil {
		heldAssemblyClose(t, rights)
		t.Fatal(err)
	}
	gFD, err := unix.FcntlInt(rights[0].Fd(), unix.F_DUPFD_CLOEXEC, 0)
	heldAssemblyClose(t, rights)
	if err != nil {
		t.Fatal(err)
	}
	add(gPID, gFD)
	subjects[gPID] = "G"
	ids[3], ids[4] = sPID, gPID
	if !heldAssemblyTuple(ids, true) {
		t.Fatal("incomplete or repeated all-role tuple")
	}
	direct := func(control *os.File, role string, pid, fd int) assemblyMessage {
		hello, files := heldAssemblyReceive(t, control, pid, fd, 0)
		heldAssemblyClose(t, files)
		if hello.Phase != "hello" || hello.Role != role || !validNonce(hello.Nonce) || hello.IDs != [5]int{} || hello.Image != image || hello.ChildPID != 0 || hello.Raw != 0 || hello.Armed {
			t.Fatal("invalid direct child challenge")
		}
		return assemblyMessage{Role: role, Nonce: hello.Nonce, IDs: ids, Image: image}
	}
	sf, gf := direct(s, "S", sPID, sFD), direct(g, "G", gPID, gFD)
	nonces := []string{mf.Nonce, of.Nonce, sf.Nonce, gf.Nonce}
	for i, nonce := range nonces {
		for _, earlier := range nonces[:i] {
			if nonce == earlier {
				t.Fatal("role challenge reused")
			}
		}
	}
	mf.IDs, of.IDs = ids, ids
	if mode == "owner-loss" {
		if err := unix.PidfdSendSignal(oFD, syscall.SIGKILL, nil, 0); err != nil {
			t.Fatal(err)
		}
		wait(oPID, uint32(syscall.SIGKILL))
		heldAssemblyStored(t, oFD, uint32(syscall.SIGKILL), "O")
		fail()
		return false
	}
	heldAssemblySend(t, m, mf, "register")
	heldAssemblyExpect(t, m, mPID, mFD, mf, "registered")
	backupM, backupS := mFD, sFD
	if mode == "substituted-backup" {
		backupM, backupS = sFD, mFD
	}
	heldAssemblySend(t, o, of, "register", backupM, backupS)
	if mode == "substituted-backup" {
		heldAssemblyExpect(t, o, oPID, oFD, of, "refused")
		fail()
		return false
	}
	if mode == "queued-owner-ack" {
		custodyCheck(t, custodyObserve(t, oPID), oPID, "29") // Actual WNOWAIT observation; O remains unconsumed.
		_, files, err := heldAssemblyRead(t, o, oPID, oFD, 0)
		heldAssemblyClose(t, files)
		if err == nil {
			t.Fatal("queued registration ACK from terminal O accepted")
		}
		fail()
		wait(oPID, 29<<8)
		heldAssemblyStored(t, oFD, 29<<8, "O")
		return false
	}
	heldAssemblyExpect(t, o, oPID, oFD, of, "registered")
	sent := sf
	if mode == "wrong-nonce" {
		sent.Nonce = mf.Nonce
	}
	heldAssemblySend(t, s, sent, "register", mFD)
	if mode == "wrong-nonce" {
		heldAssemblyExpect(t, s, sPID, sFD, sf, "refused")
		fail()
		return false
	}
	heldAssemblyExpect(t, s, sPID, sFD, sf, "registered")
	if mode == "foreign-sender" {
		heldAssemblySend(t, foreign, gf, "registered")
		heldAssemblySend(t, g, gf, "register", mFD)
		_, unexpected, err := heldAssemblyRead(t, g, gPID, gFD, 0)
		heldAssemblyClose(t, unexpected)
		if err == nil {
			t.Fatal("actual foreign writer accepted matching G frame")
		}
		fail()
		return false
	}
	phase := "register"
	if mode == "replayed-phase" {
		phase = "scoped"
	}
	heldAssemblySend(t, g, gf, phase, mFD)
	if mode == "replayed-phase" {
		heldAssemblyExpect(t, g, gPID, gFD, gf, "refused")
		fail()
		return false
	}
	heldAssemblyExpect(t, g, gPID, gFD, gf, "registered")
	for _, role := range []struct{ pid, fd int }{{ids[0], rootFD}, {mPID, mFD}, {oPID, oFD}, {sPID, sFD}, {gPID, gFD}} {
		if err := heldAssemblyLive(role.fd, role.pid, -1); err != nil {
			t.Fatalf("final held-role liveness refused: %v", err)
		}
	}
	registered = true // Ordered live observations, not a global atomic death fence.
	if failed {
		t.Fatal("latched assembly failure reset")
	}
	heldAssemblySend(t, m, mf, "retire")
	heldAssemblyExpect(t, m, mPID, mFD, mf, "retired")
	heldAssemblySend(t, o, of, "arm-request")
	heldAssemblyExpect(t, g, gPID, gFD, gf, "arm-refused") // This fact is sent by G itself.
	heldAssemblyExpect(t, o, oPID, oFD, of, "arm-refused")
	heldAssemblySend(t, m, mf, "stop")
	heldAssemblySend(t, s, sf, "stop")
	wait(mPID, 11<<8)
	heldAssemblyStored(t, mFD, 11<<8, "M")
	wait(sPID, 137<<8)
	heldAssemblyStored(t, sFD, 137<<8, "S")
	heldAssemblySend(t, o, of, "stop-G")
	status := of
	status.ChildPID = gPID
	heldAssemblyExpect(t, o, oPID, oFD, status, "child-status") // O consumed raw unarmed G zero.
	heldAssemblyStored(t, gFD, 0, "G")
	heldAssemblySend(t, o, of, "stop")
	wait(oPID, 0)
	heldAssemblyStored(t, oFD, 0, "O")
	groupProofNoChildren(t)
	return false
}

func heldAssemblyChild(t *testing.T, role string) {
	mode, root := os.Getenv("ZERB_ASSEMBLY_MODE"), os.Getenv("ZERB_ASSEMBLY_ROOT")
	last := 6
	if role == "S" {
		last = 5
	}
	if role == "G" {
		last = 7
	}
	for fd := 4; fd <= last; fd++ {
		syscall.CloseOnExec(fd)
	}
	if err := sanitizeInheritedDescriptors(3); err != nil {
		t.Fatal(err)
	}
	if err := validateDescriptorAllowlist(3); err != nil {
		t.Fatal(err)
	}
	if mode == "marker-fd" && (role == "M" || role == "O") {
		if _, err := unix.FcntlInt(8, unix.F_GETFD, 0); err != syscall.EBADF {
			t.Fatal("inherited marker survived sanitation")
		}
	}
	control, self := os.NewFile(3, "held-bootstrap"), os.NewFile(4, "held-self")
	defer control.Close()
	defer self.Close()
	parent, err := validateInternalControl(3)
	if err != nil {
		if role == "G" && mode == "wrong-parent" {
			heldAssemblyTrace(t, "refusal", "startup", "G", nil)
			os.Exit(23)
		}
		t.Fatalf("exact-parent bootstrap refused: %v", err)
	}
	image, err := descriptorContentIdentity(self, true)
	if err != nil || !sameExecutable(os.Getpid(), parent) {
		t.Fatal("actual same-image bootstrap differs")
	}
	rootSlot := 6
	if role == "S" {
		rootSlot = 5
	}
	rootCap := os.NewFile(uintptr(rootSlot), "held-F-pidfd")
	defer rootCap.Close()
	rootPID := parent
	if role == "G" {
		rootPID = 0
	}
	if role != "G" && heldAssemblyLive(int(rootCap.Fd()), parent, -1) != nil {
		t.Fatal("borrowed root identity rejected")
	}
	if role == "M" {
		if sessionID() != os.Getpid() || syscall.Getpgrp() != os.Getpid() || establishStageParentDeath(3, parent, parentDeathSignal(), syscall.SIGKILL) != nil {
			t.Fatal("M creator/release parent-death stage differs")
		}
	} else if role == "G" {
		if inherited := parentDeathSignal(); inherited != 0 && inherited != syscall.SIGUSR1 {
			t.Fatal("unexpected G inherited parent-death action")
		}
	} else if parentDeathSignal() != 0 {
		t.Fatal("surviving role inherited parent-death action")
	}
	if role == "O" {
		heldAssemblySubreaper(t)
	}
	if role == "O" && (sessionID() != os.Getpid() || syscall.Getpgrp() != os.Getpid()) {
		t.Fatal("O isolation differs")
	}
	if role == "S" && (sessionID() == parent || syscall.Getpgrp() != os.Getpid()) {
		t.Fatal("S session/group differs")
	}
	hello := assemblyMessage{Phase: "hello", Role: role, Nonce: heldAssemblyNonce(t), Image: image}
	if role == "G" {
		heldAssemblyGuard(t, control, self, rootCap, parent, image, hello)
		return
	}
	heldAssemblySend(t, control, hello, "hello")
	if role == "S" {
		heldAssemblyLeaf(t, control, rootCap, rootPID, image, hello, "S")
		return
	}
	scope, files := heldAssemblyReceive(t, control, parent, int(rootCap.Fd()), 0)
	heldAssemblyClose(t, files)
	roleIndex := 1
	if role == "O" {
		roleIndex = 2
	}
	if scope.Phase != "scope" || scope.Role != role || scope.Nonce != hello.Nonce || !heldAssemblyTuple(scope.IDs, false) || scope.IDs[0] != parent || scope.IDs[roleIndex] != os.Getpid() || scope.Image != image || scope.Raw != 0 || scope.Armed || scope.ChildPID != 0 || validateSelfFD(self, scope.Image) != nil {
		t.Fatal("initial partial birth scope rejected")
	}
	heldAssemblySend(t, control, scope, "scoped")
	heldAssemblyExpect(t, control, parent, int(rootCap.Fd()), scope, "birth")
	temporary := os.NewFile(5, "held-child-endpoint")
	defer temporary.Close()
	if role == "M" {
		heldAssemblyCreator(t, control, self, temporary, rootCap, scope, mode, root)
		return
	}
	heldAssemblyOwner(t, control, self, temporary, rootCap, scope, mode, root)
}

func heldAssemblyRegistered(frame, hello assemblyMessage, role string, rootPID, ownPID int) bool {
	index := 3
	if role == "G" {
		index = 4
	}
	if role == "M" {
		index = 1
	}
	if role == "O" {
		index = 2
	}
	return frame.Phase == "register" && frame.Role == role && frame.Nonce == hello.Nonce && heldAssemblyTuple(frame.IDs, true) && frame.IDs[0] == rootPID && frame.IDs[index] == ownPID && frame.Image == hello.Image && frame.ChildPID == 0 && frame.Raw == 0 && !frame.Armed
}

func heldAssemblyCreator(t *testing.T, control, self, temporary, rootCap *os.File, scope assemblyMessage, mode, root string) {
	parent := scope.IDs[0]
	pid, fd := heldAssemblyStart(t, "S", mode, root, []*os.File{temporary, self, rootCap}, &syscall.SysProcAttr{Cloneflags: syscall.CLONE_PARENT, Setpgid: true})
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	retired := false
	defer func() {
		if !retired {
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil && err != syscall.ESRCH {
				t.Errorf("unacknowledged S cleanup: %v", err)
			}
		}
		if fd >= 0 {
			if err := unix.Close(fd); err != nil {
				t.Errorf("close S creator capability: %v", err)
			}
		}
	}()
	if err := heldAssemblyRole(fd, pid, parent, os.Getpid(), pid); err != nil {
		t.Fatal(err)
	}
	var raw syscall.WaitStatus
	if got, err := syscall.Wait4(pid, &raw, syscall.WNOHANG, nil); got != -1 || err != syscall.ECHILD {
		t.Fatal("S consumption belongs to F")
	}
	birth := scope
	birth.ChildPID = pid
	if mode == "interrupted-S" {
		heldAssemblySend(t, control, scope, "birth-interrupted")
		_, files, _ := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
		heldAssemblyClose(t, files)
		return
	}
	heldAssemblySend(t, control, birth, "birth", fd)
	frame, files, err := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
	heldAssemblyClose(t, files)
	if err != nil {
		return
	}
	if !heldAssemblyRegistered(frame, scope, "M", parent, os.Getpid()) || frame.IDs[2] != scope.IDs[2] || frame.IDs[3] != pid {
		t.Fatal("M complete registration differs from birth")
	}
	heldAssemblySend(t, control, frame, "registered")
	command, files, err := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
	heldAssemblyClose(t, files)
	if err != nil {
		return
	}
	want := frame
	want.Phase = "retire"
	if command != want || heldAssemblyLive(int(rootCap.Fd()), parent, -1) != nil {
		heldAssemblyRefuse(t, control, parent, int(rootCap.Fd()), frame)
		return
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	fd = -1
	retired = true
	heldAssemblySend(t, control, frame, "retired")
	heldAssemblyExpect(t, control, parent, int(rootCap.Fd()), frame, "stop")
	os.Exit(11) // Administrative held-role disposal, never managed execution.
}

func heldAssemblyLeaf(t *testing.T, control, rootCap *os.File, rootPID int, image executableIdentity, hello assemblyMessage, role string) {
	frame, files, err := heldAssemblyRead(t, control, rootPID, int(rootCap.Fd()), 1)
	if err != nil {
		heldAssemblyClose(t, files)
		return
	}
	defer heldAssemblyClose(t, files)
	valid := heldAssemblyRegistered(frame, hello, role, rootPID, os.Getpid()) && frame.Image == image &&
		heldAssemblyRole(int(files[0].Fd()), frame.IDs[1], rootPID, frame.IDs[1], frame.IDs[1]) == nil &&
		heldAssemblyLive(int(rootCap.Fd()), rootPID, -1) == nil
	if role == "S" {
		valid = valid && sessionID() == frame.IDs[1] && syscall.Getpgrp() == os.Getpid()
	}
	if !valid {
		frame.Role, frame.Nonce, frame.Image = role, hello.Nonce, image
		frame.Raw, frame.ChildPID, frame.Armed = 0, 0, false
		heldAssemblyRefuse(t, control, rootPID, int(rootCap.Fd()), frame)
		return
	}
	heldAssemblySend(t, control, frame, "registered")
	command, extras, err := heldAssemblyRead(t, control, rootPID, int(rootCap.Fd()), 0)
	heldAssemblyClose(t, extras)
	if err != nil {
		return
	}
	want := frame
	want.Phase = "stop"
	if command != want {
		heldAssemblyRefuse(t, control, rootPID, int(rootCap.Fd()), frame)
		return
	}
	os.Exit(137)
}

func heldAssemblyOwner(t *testing.T, control, self, temporary, rootCap *os.File, scope assemblyMessage, mode, root string) {
	parent := scope.IDs[0]
	guard, gc := heldAssemblyPair(t)
	defer guard.Close()
	defer gc.Close()
	ownFD, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		t.Fatal(err)
	}
	own := os.NewFile(uintptr(ownFD), "O-self-pidfd")
	defer own.Close()
	if err := heldAssemblyLive(ownFD, os.Getpid(), parent); err != nil {
		t.Fatal(err)
	}
	bootstrap, custody := gc, temporary
	if mode == "wrong-parent" {
		bootstrap, custody = temporary, gc
	}
	pid, fd := heldAssemblyStart(t, "G", mode, root, []*os.File{bootstrap, self, custody, rootCap, own}, &syscall.SysProcAttr{Setpgid: true})
	if err := own.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	consumed := false
	defer func() {
		if !consumed {
			if err := groupProofShutdown(guard); err != nil {
				t.Errorf("O guard channel cleanup: %v", err)
			}
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil && err != syscall.ESRCH {
				t.Errorf("O child cleanup: %v", err)
			}
			var raw syscall.WaitStatus
			for {
				got, err := syscall.Wait4(pid, &raw, 0, nil)
				if err == syscall.EINTR {
					continue
				}
				if err != nil || got != pid {
					t.Errorf("O actual child cleanup wait: %v", err)
					if err := heldAssemblyRecord("error-wait", "cleanup", "G", nil); err != nil {
						t.Errorf("O cleanup trace write failed: %v", err)
					}
				} else {
					observedRaw := uint32(raw)
					if err := heldAssemblyRecord("wait", "cleanup", "G", &observedRaw); err != nil {
						t.Errorf("O cleanup trace write failed: %v", err)
					}
					info, err := cloneTraceGetInfo(fd)
					if err != nil || info.Mask&(1<<3) == 0 {
						t.Errorf("O cleanup retained EXIT unavailable: %v", err)
						if err := heldAssemblyRecord("error-retained-exit", "cleanup", "G", nil); err != nil {
							t.Errorf("O cleanup trace write failed: %v", err)
						}
					} else {
						retainedRaw := uint32(info.Exit)
						if err := heldAssemblyRecord("retained-exit", "cleanup", "G", &retainedRaw); err != nil {
							t.Errorf("O cleanup trace write failed: %v", err)
						}
						if retainedRaw != observedRaw {
							t.Error("O cleanup retained EXIT differs from actual wait")
						}
					}
				}
				break
			}
		}
		if err := unix.Close(fd); err != nil {
			t.Errorf("close O G capability: %v", err)
		}
	}()
	birth := scope
	birth.ChildPID = pid
	if mode == "wrong-parent" {
		heldAssemblyWait(t, pid, 23<<8, "G")
		heldAssemblyStored(t, fd, 23<<8, "G")
		consumed = true
		birth.Raw = 23 << 8
		heldAssemblySend(t, control, birth, "birth-refused", fd)
		_, files, _ := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
		heldAssemblyClose(t, files)
		return
	}
	if err := heldAssemblyRole(fd, pid, os.Getpid(), os.Getpid(), pid); err != nil {
		t.Fatal(err)
	}
	hello, files := heldAssemblyReceive(t, guard, pid, fd, 0)
	heldAssemblyClose(t, files)
	if hello.Phase != "hello" || hello.Role != "G" || !validNonce(hello.Nonce) || hello.Image != scope.Image || hello.IDs != [5]int{} || hello.Armed || hello.Raw != 0 || hello.ChildPID != 0 {
		t.Fatal("G parent challenge invalid")
	}
	gf := assemblyMessage{Role: "G", Nonce: hello.Nonce, IDs: scope.IDs, Image: scope.Image}
	gf.IDs[4] = pid
	heldAssemblySend(t, guard, gf, "parent-scope")
	heldAssemblyExpect(t, guard, pid, fd, gf, "parent-scoped")
	if mode == "interrupted-G" {
		heldAssemblySend(t, control, scope, "birth-interrupted")
		_, files, _ := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
		heldAssemblyClose(t, files)
		return
	}
	heldAssemblySend(t, control, birth, "birth", fd)
	frame, files, err := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 2)
	if err != nil {
		heldAssemblyClose(t, files)
		return
	}
	defer heldAssemblyClose(t, files)
	if !heldAssemblyRegistered(frame, scope, "O", parent, os.Getpid()) || frame.IDs[1] != scope.IDs[1] || frame.IDs[4] != pid ||
		heldAssemblyRole(int(files[0].Fd()), frame.IDs[1], parent, frame.IDs[1], frame.IDs[1]) != nil ||
		heldAssemblyRole(int(files[1].Fd()), frame.IDs[3], parent, frame.IDs[1], frame.IDs[3]) != nil {
		heldAssemblyRefuse(t, control, parent, int(rootCap.Fd()), frame)
		return
	}
	heldAssemblySend(t, control, frame, "registered")
	if mode == "queued-owner-ack" {
		os.Exit(29)
	}
	gf.IDs = frame.IDs
	heldAssemblySend(t, guard, gf, "parent-register")
	heldAssemblyExpect(t, guard, pid, fd, gf, "parent-registered")
	command, extras, err := heldAssemblyRead(t, control, parent, int(rootCap.Fd()), 0)
	heldAssemblyClose(t, extras)
	if err != nil {
		return
	}
	want := frame
	want.Phase = "arm-request"
	if command != want {
		heldAssemblyRefuse(t, control, parent, int(rootCap.Fd()), frame)
		return
	}
	heldAssemblySend(t, guard, gf, "arm")
	heldAssemblyExpect(t, guard, pid, fd, gf, "arm-refused")
	heldAssemblySend(t, control, frame, "arm-refused")
	heldAssemblyExpect(t, control, parent, int(rootCap.Fd()), frame, "stop-G")
	heldAssemblySend(t, guard, gf, "stop")
	heldAssemblyWait(t, pid, 0, "G")
	heldAssemblyStored(t, fd, 0, "G")
	consumed = true
	report := frame
	report.ChildPID = pid
	heldAssemblySend(t, control, report, "child-status")
	heldAssemblyExpect(t, control, parent, int(rootCap.Fd()), frame, "stop")
	groupProofNoChildren(t)
}

func heldAssemblyGuard(t *testing.T, control, self, rootCap *os.File, parent int, image executableIdentity, hello assemblyMessage) {
	parentCap := os.NewFile(7, "held-O-pidfd")
	defer parentCap.Close()
	info, err := cloneTraceGetInfo(int(parentCap.Fd()))
	if err != nil || heldAssemblyLive(int(parentCap.Fd()), parent, -1) != nil || info.PPID == 0 {
		t.Fatal("G borrowed O identity rejected")
	}
	rootPID := int(info.PPID)
	heldAssemblySend(t, control, hello, "hello")
	scope, files := heldAssemblyReceive(t, control, parent, int(parentCap.Fd()), 0)
	heldAssemblyClose(t, files)
	partial := scope.IDs
	partial[4] = 0
	if scope.Phase != "parent-scope" || scope.Role != "G" || scope.Nonce != hello.Nonce || !heldAssemblyTuple(partial, false) || scope.IDs[0] != rootPID || scope.IDs[2] != parent || scope.IDs[4] != os.Getpid() || scope.Image != image || scope.ChildPID != 0 || scope.Raw != 0 || scope.Armed || validateSelfFD(self, scope.Image) != nil || heldAssemblyLive(int(rootCap.Fd()), rootPID, -1) != nil || sessionID() != parent || syscall.Getpgrp() != os.Getpid() || scope.IDs[1] == sessionID() {
		t.Fatal("G authenticated parent scope rejected")
	}
	signal.Ignore(syscall.SIGUSR1)
	if err := establishStageParentDeath(3, parent, parentDeathSignal(), syscall.SIGUSR1); err != nil {
		t.Fatalf("G O-parent survival stage refused: %v", err)
	}
	heldAssemblySend(t, control, scope, "parent-scoped")
	custody := os.NewFile(5, "held-F-custody")
	defer custody.Close()
	if err := validatePeerIdentity(5, rootPID, uint32(os.Geteuid())); err != nil || !sameExecutable(rootPID, os.Getpid()) {
		t.Fatal("G secondary F route rejected")
	}
	direct := hello
	for direct.Nonce == hello.Nonce {
		direct.Nonce = heldAssemblyNonce(t)
	}
	heldAssemblySend(t, custody, direct, "hello")
	// Parent and F registration order is independent. O validates backup rights
	// before it sends this complete parent binding; G never treats it as F's ACK.
	parentFrame, files, err := heldAssemblyRead(t, control, parent, int(parentCap.Fd()), 0)
	heldAssemblyClose(t, files)
	if err != nil {
		return
	}
	if parentFrame.Phase != "parent-register" || parentFrame.Role != "G" || parentFrame.Nonce != hello.Nonce || !heldAssemblyTuple(parentFrame.IDs, true) || parentFrame.IDs[0] != rootPID || parentFrame.IDs[1] != scope.IDs[1] || parentFrame.IDs[2] != parent || parentFrame.IDs[4] != os.Getpid() || parentFrame.Image != image || parentFrame.ChildPID != 0 || parentFrame.Raw != 0 || parentFrame.Armed {
		t.Fatal("G parent complete registration rejected")
	}
	heldAssemblySend(t, control, parentFrame, "parent-registered")
	frame, rights, err := heldAssemblyRead(t, custody, rootPID, int(rootCap.Fd()), 1)
	if err != nil {
		heldAssemblyClose(t, rights)
		return
	}
	defer heldAssemblyClose(t, rights)
	if !heldAssemblyRegistered(frame, direct, "G", rootPID, os.Getpid()) || frame.IDs != parentFrame.IDs ||
		heldAssemblyRole(int(rights[0].Fd()), frame.IDs[1], rootPID, frame.IDs[1], frame.IDs[1]) != nil || heldAssemblyLive(int(rootCap.Fd()), rootPID, -1) != nil {
		frame.Role, frame.Nonce, frame.Image = "G", direct.Nonce, image
		frame.Raw, frame.ChildPID, frame.Armed = 0, 0, false
		heldAssemblyRefuse(t, custody, rootPID, int(rootCap.Fd()), frame)
		return
	}
	heldAssemblySend(t, custody, frame, "registered")
	arm, files, err := heldAssemblyRead(t, control, parent, int(parentCap.Fd()), 0)
	heldAssemblyClose(t, files)
	if err != nil {
		return
	}
	want := parentFrame
	want.Phase = "arm"
	if arm != want || heldAssemblyLive(int(parentCap.Fd()), parent, rootPID) != nil {
		t.Fatal("unbound G parent arm request")
	}
	// No cgroup capability exists. A request cannot arm this held fixture.
	heldAssemblySend(t, custody, frame, "arm-refused")
	heldAssemblySend(t, control, parentFrame, "arm-refused")
	command, files, err := heldAssemblyRead(t, control, parent, int(parentCap.Fd()), 0)
	heldAssemblyClose(t, files)
	if err != nil {
		return
	}
	want.Phase = "stop"
	if command != want || heldAssemblyLive(int(parentCap.Fd()), parent, rootPID) != nil {
		t.Fatal("unbound G administrative stop")
	}
	// Zero is solely the unarmed G process status, with no guard-cleanup claim.
}

// This opt-in failure is for validating the redacted retention path without
// starting any proof roles or making a host-qualification claim.
func TestHeldAssemblyTraceFailureFixture(t *testing.T) {
	if os.Getenv("ZERB_ASSEMBLY_TRACE_FIXTURE") != "intentional-failure" {
		return
	}
	t.Setenv("ZERB_ASSEMBLY_ROLE", "fixture")
	t.Setenv("ZERB_ASSEMBLY_MODE", "held")
	heldAssemblyTrace(t, "error-wait", "status", "M", nil)
	t.Fatal("intentional assembly trace retention fixture failure")
}

func TestHeldAssemblyTraceEncoding(t *testing.T) {
	zero := uint32(0)
	valid := heldAssemblyTraceRecord{Role: "F", Mode: "held", Sequence: 1, Event: "wait", Phase: "status", Subject: "G", RawStatus: &zero}
	payload, err := heldAssemblyTraceBytes(valid)
	if err != nil {
		t.Fatal(err)
	}
	want := heldAssemblyTracePrefix + `{"role":"F","mode":"held","sequence":1,"event":"wait","phase":"status","subject":"G","raw_status":0}` + "\n"
	if string(payload) != want {
		t.Fatal("bounded trace encoding differs")
	}
	tooLarge := uint32(65536)
	for _, field := range []string{"role", "mode", "sequence", "event", "phase", "subject", "missing-status", "extra-status", "status-bound"} {
		t.Run(field, func(t *testing.T) {
			record := valid
			switch field {
			case "role":
				record.Role = "undeclared"
			case "mode":
				record.Mode = "undeclared"
			case "sequence":
				record.Sequence = 0
			case "event":
				record.Event = "undeclared"
			case "phase":
				record.Phase = "undeclared"
			case "subject":
				record.Subject = "undeclared"
			case "missing-status":
				record.RawStatus = nil
			case "extra-status":
				record.Event = "send"
			case "status-bound":
				record.RawStatus = &tooLarge
			}
			if payload, err := heldAssemblyTraceBytes(record); err == nil || payload != nil {
				t.Fatal("invalid trace field reached output")
			}
		})
	}
	if err := heldAssemblyWriteTrace(-1, valid); !errors.Is(err, syscall.EBADF) {
		t.Fatal("raw stderr write error was lost")
	}
	if heldAssemblyTracePhase("undeclared") != "unknown" || heldAssemblyTraceSubject("undeclared") != "unknown" || heldAssemblyTraceSubject("harness") != "unknown" || !heldAssemblyTraceRole("harness") || heldAssemblyTraceSubject("fixture") != "unknown" {
		t.Fatal("untrusted trace field was not reduced to its closed tag")
	}
}

// These opt-in fixtures test streaming retention beyond the old bounded prefix.
// They create no authenticated role and provide no lifecycle proof evidence.
func TestHeldAssemblyTraceOuterFailureFixture(t *testing.T) {
	if os.Getenv("ZERB_ASSEMBLY_TRACE_FIXTURE") != "streamed-late-failure" {
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHeldAssemblyTraceLateFailureFixture$")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err := cmd.Run()
	heldAssemblyHarnessWait(t, cmd, "held", "unknown")
	if err != nil {
		t.Fatalf("intentional streamed fixture child failure: %v", err)
	}
	t.Fatal("intentional streamed fixture unexpectedly succeeded")
}

func TestHeldAssemblyTraceLateFailureFixture(t *testing.T) {
	if os.Getenv("ZERB_ASSEMBLY_TRACE_FIXTURE") != "streamed-late-failure" {
		return
	}
	t.Setenv("ZERB_ASSEMBLY_ROLE", "fixture")
	t.Setenv("ZERB_ASSEMBLY_MODE", "held")
	for range 128 {
		heldAssemblyTrace(t, "refusal", "unknown", "unknown", nil)
	}
	t.Fatal("intentional late assembly trace retention fixture failure")
}
