//go:build linux && amd64

package lifecycle

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ARCH-010 / VAL-019/020: this isolated C(subreaper)->O->G kernel subset proves
// status custody only, not role registration, authentication, cgroups, target
// release, or a production lifecycle. C records status before acknowledging O.
func TestStatusCustodyAcrossOwnerDeath(t *testing.T) {
	for _, gate := range []string{"before-observe", "before-send", "before-ack", "after-ack", "after-consume"} {
		for _, status := range []string{"0", "11", "137", "kill"} {
			t.Run(gate+"/"+status, func(t *testing.T) {
				cmd := custodyCommand("C", gate, status, t.TempDir(), nil)
				var output custodyOutput
				cmd.Stdout, cmd.Stderr = &output, &output
				if err := cmd.Run(); err != nil {
					t.Fatalf("isolated custody row: %v\n%s", err, output.Bytes())
				}
			})
		}
	}
}

// Only one bounded output writer is used by os/exec for the identical streams.
type custodyOutput struct{ bytes.Buffer }

func (output *custodyOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 8192 - output.Len(); remaining > 0 {
		_, _ = output.Buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
}

type custodyStatus struct {
	PID    int32 `json:"pid"`
	Code   int32 `json:"code"`
	Status int32 `json:"status"`
}

type custodyEvent struct {
	Gate   string        `json:"gate"`
	PID    int           `json:"pid"`
	Status custodyStatus `json:"status"`
}

func custodyCommand(role, gate, status, root string, control *os.File) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestStatusCustodyChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_CUSTODY_ROLE="+role,
		"ZERB_CUSTODY_GATE="+gate, "ZERB_CUSTODY_STATUS="+status, "GORACE=atexit_sleep_ms=0")
	if control != nil {
		cmd.ExtraFiles = []*os.File{control}
	}
	return cmd
}

func TestStatusCustodyChild(t *testing.T) {
	role, gate, status := os.Getenv("ZERB_CUSTODY_ROLE"), os.Getenv("ZERB_CUSTODY_GATE"), os.Getenv("ZERB_CUSTODY_STATUS")
	switch role {
	case "":
		return
	case "C":
		custodyRoot(t, gate, status)
	case "O":
		custodyOwner(t, gate, status)
	case "G":
		if status == "kill" {
			if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			select {}
		}
		code, err := strconv.Atoi(status)
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(code)
	default:
		t.Fatal("unknown custody role")
	}
}

func custodyRoot(t *testing.T, gate, status string) {
	t.Helper()
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	parent, child, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	defer child.Close()
	owner := custodyCommand("O", gate, status, os.Getenv("HOME"), child)
	owner.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	owner.Stdout, owner.Stderr = os.Stdout, os.Stderr
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	child.Close()
	// A watchdog fails the row and kills only this disposable owned group. It
	// supplies no cleanup deadline or success evidence for the product.
	watchdogDone := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() {
		defer close(watchdogDone)
		_ = syscall.Kill(-owner.Process.Pid, syscall.SIGKILL)
		_ = parent.Close()
	})
	watchdogStopped := false
	stopWatchdog := func() bool {
		if watchdogStopped {
			return true
		}
		watchdogStopped = true
		stopped := watchdog.Stop()
		if !stopped {
			<-watchdogDone
		}
		return stopped
	}
	defer func() {
		stopWatchdog()
		if owner.ProcessState == nil {
			_ = syscall.Kill(-owner.Process.Pid, syscall.SIGKILL)
			_ = owner.Wait()
		}
		for {
			var raw syscall.WaitStatus
			_, err := syscall.Wait4(-1, &raw, 0, nil)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				break
			}
		}
	}()
	ready := custodyRead(t, parent)
	if ready.Gate != "born" || ready.PID <= 0 {
		t.Fatalf("invalid owned child announcement: %+v", ready)
	}
	boundary := custodyRead(t, parent)
	var ledger custodyStatus
	observed := false
	if gate == "before-ack" || gate == "after-ack" || gate == "after-consume" {
		if boundary.Gate != "observed" || boundary.PID != ready.PID {
			t.Fatalf("missing exact status report: %+v", boundary)
		}
		ledger, observed = boundary.Status, true
		custodyCheck(t, ledger, ready.PID, status)
		if gate != "before-ack" {
			// The ledger assignment above happens before the root's ACK.
			custodyWrite(t, parent, custodyEvent{Gate: "ack", PID: ready.PID, Status: ledger})
			boundary = custodyRead(t, parent)
		}
	}
	if boundary.Gate != gate && (gate != "before-ack" || boundary.Gate != "observed") {
		t.Fatalf("wrong death boundary: %+v; want %s", boundary, gate)
	}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// Retain O's zombie until G is handled, pinning the cleanup group number.
	custodyCheck(t, custodyObserve(t, owner.Process.Pid), owner.Process.Pid, "kill")
	if gate != "after-consume" {
		adopted := custodyObserve(t, ready.PID)
		custodyCheck(t, adopted, ready.PID, status)
		if observed && adopted != ledger {
			t.Fatalf("adoption changed recorded status: got %+v; ledger %+v", adopted, ledger)
		}
		ledger, observed = adopted, true
		custodyConsume(t, ready.PID, ledger)
	}
	if !observed {
		t.Fatal("missing status cannot authorize success")
	}
	if got, want := ledger.Code == 1 && ledger.Status == 0, status == "0"; got != want {
		t.Fatalf("guard success=%t; want %t", got, want)
	}
	if !stopWatchdog() {
		t.Fatal("custody watchdog expired")
	}
	if err := owner.Wait(); err == nil || !owner.ProcessState.Sys().(syscall.WaitStatus).Signaled() || owner.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("owner was not killed at boundary: %v", err)
	}
	var raw syscall.WaitStatus
	if pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatalf("children remain: pid=%d raw=%#x err=%v", pid, uint32(raw), err)
	}
}

func custodyOwner(t *testing.T, gate, status string) {
	t.Helper()
	control := os.NewFile(3, "custody-root")
	defer control.Close()
	syscall.CloseOnExec(3)
	guard := custodyCommand("G", gate, status, os.Getenv("HOME"), nil)
	guard.Stdout, guard.Stderr = io.Discard, io.Discard
	if err := guard.Start(); err != nil {
		t.Fatal(err)
	}
	pid := guard.Process.Pid
	custodyWrite(t, control, custodyEvent{Gate: "born", PID: pid})
	if gate == "before-observe" {
		custodyWrite(t, control, custodyEvent{Gate: gate, PID: pid})
		custodyPark(t, control)
	}
	observation := custodyObserve(t, pid)
	custodyCheck(t, observation, pid, status)
	if repeated := custodyObserve(t, pid); repeated != observation {
		t.Fatalf("WNOWAIT did not retain exact status: %+v then %+v", observation, repeated)
	}
	if gate == "before-send" {
		custodyWrite(t, control, custodyEvent{Gate: gate, PID: pid})
		custodyPark(t, control)
	}
	custodyWrite(t, control, custodyEvent{Gate: "observed", PID: pid, Status: observation})
	ack := custodyRead(t, control)
	if ack.Gate != "ack" || ack.PID != pid || ack.Status != observation {
		t.Fatal("no exact root ledger acknowledgement")
	}
	if gate == "after-ack" {
		custodyWrite(t, control, custodyEvent{Gate: gate, PID: pid})
		custodyPark(t, control)
	}
	custodyConsume(t, pid, observation)
	if err := guard.Process.Release(); err != nil {
		t.Fatal(err)
	}
	custodyWrite(t, control, custodyEvent{Gate: "after-consume", PID: pid})
	custodyPark(t, control)
}

func custodyPark(t *testing.T, control *os.File) {
	t.Helper()
	if _, err := readFrame(control, 512); err != nil {
		t.Fatalf("owner gate ended without injected death: %v", err)
	}
	t.Fatal("unexpected owner release")
}

func custodyRead(t *testing.T, control *os.File) custodyEvent {
	t.Helper()
	payload, err := readFrame(control, 512)
	if err != nil {
		t.Fatal(err)
	}
	var event custodyEvent
	if err := decodeStrict(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func custodyWrite(t *testing.T, control *os.File, event custodyEvent) {
	t.Helper()
	if err := writeFrame(control, event); err != nil {
		t.Fatal(err)
	}
}

func custodyObserve(t *testing.T, pid int) custodyStatus {
	t.Helper()
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	// Linux amd64 siginfo_t's SIGCHLD union starts at byte 16; x/sys exposes
	// only its header. This test-only overlay is checked against real wait4.
	child := (*struct {
		Signo, Errno, Code, Pad, PID, UID, Status int32
	})(unsafe.Pointer(&info)) // #nosec G103 -- live Linux amd64 siginfo_t; SIGCHLD fields fit its 128-byte storage and are checked against wait4.
	if child.Signo != int32(syscall.SIGCHLD) || child.Errno != 0 || child.PID != int32(pid) {
		t.Fatal("invalid waitid child identity/header")
	}
	return custodyStatus{PID: child.PID, Code: child.Code, Status: child.Status}
}

func custodyCheck(t *testing.T, got custodyStatus, pid int, status string) {
	t.Helper()
	want := custodyStatus{PID: int32(pid), Code: 1}
	if status == "kill" {
		want.Code, want.Status = 2, int32(syscall.SIGKILL)
	} else {
		code, err := strconv.ParseInt(status, 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		want.Status = int32(code)
	}
	if got != want {
		t.Fatalf("exact guard status %+v; want %+v", got, want)
	}
}

func custodyConsume(t *testing.T, pid int, observation custodyStatus) {
	t.Helper()
	var raw syscall.WaitStatus
	for {
		got, err := syscall.Wait4(pid, &raw, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || got != pid {
			t.Fatalf("consume exact child: pid=%d err=%v", got, err)
		}
		break
	}
	want := syscall.WaitStatus(observation.Status << 8)
	if observation.Code == 2 {
		want = syscall.WaitStatus(observation.Status)
	}
	if raw != want {
		t.Fatalf("WNOWAIT/wait4 mismatch: observation=%+v raw=%#x want=%#x", observation, uint32(raw), uint32(want))
	}
}
