//go:build linux && amd64

package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
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

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-018/019/020: a test-private M registration and live-F
// backup subset. F-created socket credentials authenticate F to O, not O to F.
// Exclusive inherited endpoint custody, held births and full image observations
// are limited provenance, not full authenticated F/O/M/S/G assembly.
func TestRegisteredBackupCustody(t *testing.T) {
	for _, mode := range []string{"live", "before-transfer", "before-ack", "after-ack", "before-consume", "after-consume"} {
		statuses := []string{"137"}
		if mode == "live" || mode == "before-consume" || mode == "after-consume" {
			statuses = []string{"0", "11", "137", "kill"}
		}
		for _, status := range statuses {
			t.Run(mode+"/"+status, func(t *testing.T) { backupProofRun(t, mode, status) })
		}
	}
	for _, mode := range []string{"wrong-nonce", "cross-role-nonce", "wrong-role", "wrong-pid", "wrong-owner", "wrong-image", "wrong-pidfd", "ordinary-file", "zero-rights", "multiple-rights", "truncated-rights", "extra-stream", "unreaped-manager", "unreaped-owner", "replay"} {
		t.Run(mode, func(t *testing.T) { backupProofRun(t, mode, "137") })
	}
}

func backupProofRun(t *testing.T, mode, status string) {
	t.Helper()
	cmd := backupProofCommand("F", mode, status, t.TempDir())
	var output custodyOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 77 {
		t.Skipf("unexercised retained-status capability: %s", output.Bytes())
	}
	if err != nil {
		t.Fatalf("registered backup row: %v\n%s", err, output.Bytes())
	}
}

func backupProofCommand(role, mode, status, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestRegisteredBackupChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_BACKUP_ROLE="+role, "ZERB_BACKUP_MODE="+mode,
		"ZERB_BACKUP_STATUS="+status, "ZERB_BACKUP_ROOT="+root, "GORACE=atexit_sleep_ms=0")
	return cmd
}

type backupProofFrame struct {
	Nonce, Phase, Role string
	PID, Parent, Owner int
	Self               executableIdentity
	Raw                uint32
}

func backupProofRead(control *os.File) (backupProofFrame, error) {
	var frame backupProofFrame
	payload, err := readFrame(control, 1024)
	if err == nil {
		err = decodeStrict(payload, &frame)
	}
	return frame, err
}

func backupProofExpect(t *testing.T, control *os.File, want backupProofFrame, phase string) {
	t.Helper()
	want.Phase = phase
	got, err := backupProofRead(control)
	if err != nil || got != want {
		t.Fatalf("bound registration phase %s not observed: %v", phase, err)
	}
}

func backupProofWrite(t *testing.T, control *os.File, frame backupProofFrame, phase string) {
	t.Helper()
	frame.Phase = phase
	if err := writeFrame(control, frame); err != nil {
		t.Fatal(err)
	}
}

func TestRegisteredBackupChild(t *testing.T) {
	role := os.Getenv("ZERB_BACKUP_ROLE")
	if role == "" {
		return
	}
	runtime.LockOSThread()
	unexercised := false
	switch role {
	case "F":
		unexercised = backupProofCaller(t)
	case "M", "O":
		backupProofChild(t, role)
	default:
		t.Fatal("unknown backup proof role")
	}
	if t.Failed() {
		os.Exit(1)
	}
	if unexercised {
		os.Exit(77)
	}
	os.Exit(0)
}

func backupProofLive(fd, pid, parent int) error {
	info, err := cloneTraceGetInfo(fd)
	if err != nil {
		return err
	}
	if info.Mask&1 == 0 || info.Mask&(1<<3) != 0 || info.PID != uint32(pid) || info.TGID != uint32(pid) || info.PPID != uint32(parent) {
		return errors.New("pidfd does not bind the expected live birth")
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(poll, 0)
	if err != nil {
		return err
	}
	if n != 0 || poll[0].Revents != 0 {
		return errors.New("role pidfd is terminal or unavailable")
	}
	return nil
}

func backupProofCaller(t *testing.T) bool {
	probe, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, probeErr := cloneTraceGetInfo(probe)
	if err := unix.Close(probe); err != nil {
		t.Fatal(err)
	}
	if probeErr == syscall.ENOTTY || probeErr == syscall.EINVAL || probeErr == syscall.ENOSYS {
		fmt.Fprintln(os.Stderr, "retained-status ioctl unavailable before child birth:", probeErr)
		return true
	}
	if probeErr != nil {
		t.Fatal(probeErr)
	}
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	defer self.Close()
	identity, err := descriptorContentIdentity(self, true)
	if err != nil {
		t.Fatal(err)
	}
	m, mc, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer mc.Close()
	o, oc, err := socketPair()
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	defer oc.Close()
	var mu sync.Mutex
	var owned []int
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, control := range []*os.File{m, o} {
			if err := groupProofShutdown(control); err != nil {
				t.Errorf("backup control revocation: %v", err)
			}
		}
		for _, fd := range owned {
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil && err != syscall.ESRCH {
				t.Errorf("birth-held cleanup: %v", err)
			}
		}
	}
	done := make(chan struct{})
	watchdog := time.AfterFunc(15*time.Second, func() { cleanup(); close(done) })
	defer func() {
		if !watchdog.Stop() {
			<-done
			t.Error("backup proof watchdog expired")
		}
		cleanup()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var raw syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil)
			if err == syscall.ECHILD {
				break
			}
			if err == syscall.EINTR {
				continue
			}
			if err != nil || pid == 0 && time.Now().After(deadline) {
				t.Errorf("unresolved backup cleanup: pid=%d err=%v", pid, err)
				break
			}
			if pid == 0 {
				time.Sleep(time.Millisecond)
			}
		}
		for _, fd := range owned {
			if err := unix.Close(fd); err != nil {
				t.Errorf("close backup capability: %v", err)
			}
		}
	}()
	mode, status, root := os.Getenv("ZERB_BACKUP_MODE"), os.Getenv("ZERB_BACKUP_STATUS"), os.Getenv("ZERB_BACKUP_ROOT")
	start := func(role string, child *os.File) (int, int) {
		cmd := backupProofCommand(role, mode, status, root)
		cmd.Path, cmd.Args[0] = "/proc/self/fd/4", "/proc/self/fd/4"
		cmd.ExtraFiles = []*os.File{child, self}
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		fd := -1
		cmd.SysProcAttr = &syscall.SysProcAttr{PidFD: &fd}
		if role == "M" {
			cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Process.Release()
		if fd < 0 {
			t.Fatal("backup birth omitted atomic pidfd")
		}
		mu.Lock()
		owned = append(owned, fd)
		mu.Unlock()
		groupProofFD(t, fd)
		child.Close()
		return cmd.Process.Pid, fd
	}
	mPID, mFD := start("M", mc)
	oPID, oFD := start("O", oc)
	bind := func(control *os.File, role string, fd, pid int) backupProofFrame {
		challenge, err := backupProofRead(control)
		if err != nil || challenge.Phase != "challenge" || !validNonce(challenge.Nonce) {
			t.Fatalf("missing child nonce: %v", err)
		}
		if err := backupProofLive(fd, pid, os.Getpid()); err != nil || !sameExecutable(pid, os.Getpid()) {
			t.Fatalf("held child/image provenance failed: %v", err)
		}
		frame := backupProofFrame{Nonce: challenge.Nonce, Role: role, PID: mPID,
			Parent: os.Getpid(), Owner: oPID, Self: identity}
		backupProofWrite(t, control, frame, "scope")
		backupProofExpect(t, control, frame, "scoped")
		return frame
	}
	mScope := bind(m, "M", mFD, mPID)
	oScope := bind(o, "O", oFD, oPID)
	if mScope.Nonce == oScope.Nonce {
		t.Fatal("role challenges unexpectedly reused a nonce")
	}
	failed, released := false, false
	killOwner := func() {
		failed = true
		if err := unix.PidfdSendSignal(oFD, syscall.SIGKILL, nil, 0); err != nil {
			t.Fatal(err)
		}
		cloneTraceWait(t, oPID, uint32(syscall.SIGKILL))
		cloneTraceStored(t, oFD, uint32(syscall.SIGKILL))
	}
	finishHeld := func() {
		if !failed || released {
			t.Fatal("failed registration cannot authorize a new release")
		}
		if err := groupProofShutdown(m); err != nil {
			t.Fatal(err)
		}
		cloneTraceWait(t, mPID, 0)
		cloneTraceStored(t, mFD, 0)
		if _, err := os.Stat(filepath.Join(root, "entered")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed registration entered the synthetic target")
		}
		groupProofNoChildren(t)
	}
	if mode == "before-transfer" {
		killOwner()
		finishHeld()
		return false
	}
	if mode == "unreaped-manager" {
		if err := groupProofShutdown(m); err != nil {
			t.Fatal(err)
		}
		custodyCheck(t, custodyObserve(t, mPID), mPID, "0")
		info, err := cloneTraceGetInfo(mFD)
		if err != nil || info.Mask&1 == 0 || info.PID != uint32(mPID) || info.TGID != uint32(mPID) || info.PPID != uint32(os.Getpid()) || info.Mask&(1<<3) != 0 {
			t.Fatal("unconsumed M did not retain the misleading identity-without-EXIT fixture")
		}
	}
	registration := oScope
	registration.Role = "M"
	sent := registration
	right := mFD
	var ordinary *os.File
	switch mode {
	case "wrong-nonce":
		sent.Nonce = strings.Repeat("0", 64)
	case "cross-role-nonce":
		sent.Nonce = mScope.Nonce
	case "wrong-role":
		sent.Role = "S"
	case "wrong-pid":
		sent.PID = oPID
	case "wrong-owner":
		sent.Owner = mPID
	case "wrong-image":
		sent.Self.SHA256 = strings.Repeat("0", 64)
	case "wrong-pidfd":
		right = oFD
	case "ordinary-file":
		ordinary, err = os.CreateTemp(root, "ordinary-")
		if err != nil {
			t.Fatal(err)
		}
		defer ordinary.Close()
		right = int(ordinary.Fd())
	}
	rights := unix.UnixRights(right)
	marker := []byte{1}
	switch mode {
	case "zero-rights":
		rights = nil
	case "multiple-rights":
		rights = unix.UnixRights(mFD, oFD)
	case "truncated-rights":
		rights = unix.UnixRights(mFD, oFD, mFD)
	case "extra-stream":
		marker = []byte{1, 2}
	}
	if err := unix.Sendmsg(int(o.Fd()), marker, rights, nil, 0); err != nil {
		t.Fatal(err)
	}
	backupProofWrite(t, o, sent, "register")
	if strings.HasPrefix(mode, "wrong-") || mode == "cross-role-nonce" || mode == "ordinary-file" || strings.HasSuffix(mode, "-rights") || mode == "extra-stream" || mode == "unreaped-manager" {
		backupProofExpect(t, o, registration, "refused")
		cloneTraceWait(t, oPID, 0)
		cloneTraceStored(t, oFD, 0)
		failed = true
		finishHeld()
		return false
	}
	backupProofExpect(t, o, registration, "received")
	if mode == "before-ack" {
		killOwner()
		finishHeld()
		return false
	}
	backupProofWrite(t, o, registration, "permit-ack")
	backupProofExpect(t, o, registration, "registered")
	if mode == "after-ack" {
		killOwner()
		finishHeld()
		return false
	}
	if mode == "unreaped-owner" {
		if err := unix.PidfdSendSignal(oFD, syscall.SIGKILL, nil, 0); err != nil {
			t.Fatal(err)
		}
		custodyCheck(t, custodyObserve(t, oPID), oPID, "kill")
		info, err := cloneTraceGetInfo(oFD)
		if err != nil || info.Mask&1 == 0 || info.PID != uint32(oPID) || info.TGID != uint32(oPID) || info.PPID != uint32(os.Getpid()) || info.Mask&(1<<3) != 0 || backupProofLive(oFD, oPID, os.Getpid()) == nil {
			t.Fatal("terminal unconsumed O admitted a release prerequisite")
		}
		cloneTraceWait(t, oPID, uint32(syscall.SIGKILL))
		cloneTraceStored(t, oFD, uint32(syscall.SIGKILL))
		failed = true
		finishHeld()
		return false
	}
	if mode == "replay" {
		backupProofWrite(t, o, registration, "register")
		backupProofExpect(t, o, registration, "refused")
		cloneTraceWait(t, oPID, 0)
		failed = true
		finishHeld()
		return false
	}
	if err := backupProofLive(oFD, oPID, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	backupProofWrite(t, m, mScope, "release")
	released = true
	raw := uint32(syscall.SIGKILL)
	if status != "kill" {
		value, err := strconv.Atoi(status)
		if err != nil {
			t.Fatal(err)
		}
		raw = uint32(value << 8)
	}
	if mode == "before-consume" {
		custodyCheck(t, custodyObserve(t, mPID), mPID, status)
		info, err := cloneTraceGetInfo(mFD)
		if err != nil || info.Mask&(1<<3) != 0 {
			t.Fatal("terminal M was not retained before consumption")
		}
		killOwner() // M is terminal but F has not consumed it yet.
	}
	cloneTraceWait(t, mPID, raw) // Natural consumption precedes O's EXIT request.
	cloneTraceStored(t, mFD, raw)
	if mode == "after-consume" || mode == "before-consume" {
		if mode == "after-consume" {
			killOwner()
		}
		cloneTraceStored(t, mFD, raw)
		if !failed {
			t.Fatal("known statuses must not erase owner-loss failure")
		}
	} else {
		backupProofWrite(t, o, registration, "observe")
		observed := registration
		observed.Raw = raw
		backupProofExpect(t, o, observed, "observed")
		cloneTraceWait(t, oPID, 0)
		cloneTraceStored(t, oFD, 0)
	}
	if marker, err := os.ReadFile(filepath.Join(root, "entered")); err != nil || string(marker) != "entered" {
		t.Fatal("released synthetic target did not enter")
	}
	groupProofNoChildren(t)
	return false
}

func backupProofChild(t *testing.T, role string) {
	control, self := cloneTraceControl(3), os.NewFile(4, "backup-self")
	defer control.Close()
	defer self.Close()
	syscall.CloseOnExec(4)
	parent, err := validateInternalControl(3)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	challenge := backupProofFrame{Nonce: hex.EncodeToString(nonce[:]), Phase: "challenge"}
	if err := writeFrame(control, challenge); err != nil {
		t.Fatal(err)
	}
	scope, err := backupProofRead(control)
	if err != nil || validateNonce(scope.Nonce, challenge.Nonce) != nil || scope.Phase != "scope" || scope.Role != role ||
		scope.Parent != parent || scope.PID <= 0 || scope.Owner <= 0 || scope.Raw != 0 || validateSelfFD(self, scope.Self) != nil ||
		!sameExecutable(os.Getpid(), parent) || os.Getppid() != parent {
		t.Fatalf("invalid parent-bound role scope: %v", err)
	}
	if role == "M" {
		if scope.PID != os.Getpid() || establishStageParentDeath(3, parent, parentDeathSignal(), syscall.SIGKILL) != nil {
			t.Fatal("held M parent-death relation is not established")
		}
	} else if scope.Owner != os.Getpid() || parentDeathSignal() != 0 {
		t.Fatal("O survival stage is not exact")
	}
	backupProofWrite(t, control, scope, "scoped")
	if role == "M" {
		release, err := backupProofRead(control)
		if errors.Is(err, io.EOF) {
			return // Unreleased target refuses without entering.
		}
		want := scope
		want.Phase = "release"
		if err != nil || release != want {
			t.Fatal("unbound synthetic target release")
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("ZERB_BACKUP_ROOT"), "entered"), []byte("entered"), 0600); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("ZERB_BACKUP_STATUS") == "kill" {
			if err := unix.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			select {}
		}
		status, err := strconv.Atoi(os.Getenv("ZERB_BACKUP_STATUS"))
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(status)
	}
	want := scope
	want.Role, want.Phase = "M", "register"
	fd, receiveErr := receiveRegistrationRights(control)
	if fd != nil {
		defer func() {
			if err := fd.Close(); err != nil {
				t.Errorf("close received role capability: %v", err)
			}
		}()
	}
	registration, readErr := backupProofRead(control)
	if receiveErr != nil || fd == nil || readErr != nil || registration != want || backupProofLive(int(fd.Fd()), scope.PID, parent) != nil {
		backupProofWrite(t, control, want, "refused")
		return // Rejected capabilities are close-only, never signalled.
	}
	var unowned syscall.WaitStatus
	if pid, err := syscall.Wait4(scope.PID, &unowned, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatal("O unexpectedly owns M consumption")
	}
	backupProofWrite(t, control, want, "received")
	backupProofExpect(t, control, want, "permit-ack")
	backupProofWrite(t, control, want, "registered")
	command, err := backupProofRead(control)
	want.Phase = "observe"
	if err != nil || command != want {
		backupProofWrite(t, control, want, "refused")
		return
	}
	info, err := cloneTraceGetInfo(int(fd.Fd()))
	if err != nil || info.Mask&(1<<3) == 0 {
		t.Fatal("stored EXIT unavailable; no completion result")
	}
	want.Raw = uint32(info.Exit)
	backupProofWrite(t, control, want, "observed")
}
