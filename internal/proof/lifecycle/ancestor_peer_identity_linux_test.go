//go:build linux && amd64

package lifecycle

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-019/020: only kernel peer/executable identity in disposable
// O(subreaper)->C->L. No nonce/stage protocol, parent-death setup, target release,
// role registration, status ledger or production dispatch is exercised here.
func TestAncestorPeerIdentity(t *testing.T) {
	for _, creator := range []string{"C", "O"} {
		t.Run(creator+"-created-child-channel", func(t *testing.T) {
			cmd := ancestorPeerCommand("O", creator, t.TempDir())
			var output custodyOutput
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Run(); err != nil {
				t.Fatalf("isolated peer row: %v\n%s", err, output.Bytes())
			}
		})
	}
}

func ancestorPeerCommand(role, creator, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestAncestorPeerChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_PEER_ROLE="+role,
		"ZERB_PEER_CREATOR="+creator, "GORACE=atexit_sleep_ms=0")
	return cmd
}

func TestAncestorPeerChild(t *testing.T) {
	creator := os.Getenv("ZERB_PEER_CREATOR")
	switch os.Getenv("ZERB_PEER_ROLE") {
	case "":
		return
	case "O":
		ancestorPeerOwner(t, creator)
	case "C":
		ancestorPeerCaller(t, creator)
	case "L":
		control := os.NewFile(3, "peer-caller")
		defer control.Close()
		syscall.CloseOnExec(3)
		peer := ancestorPeerPID(t, control)
		parent, err := validateInternalControl(3)
		accepted := err == nil
		if !sameExecutable(os.Getppid(), os.Getpid()) || accepted != (creator == "C") || (accepted && parent != os.Getppid()) {
			t.Fatalf("wrong immediate-parent validation: parent=%d peer=%d accepted=%t err=%v", parent, peer, accepted, err)
		}
		ancestorPeerWrite(t, control, ancestorPeerEvent{Gate: "launcher", PID: os.Getpid(), Parent: os.Getppid(), Peer: peer, Accepted: accepted})
	default:
		t.Fatal("unknown peer role")
	}
}

type ancestorPeerEvent struct {
	Gate              string
	PID, Parent, Peer int
	Accepted          bool
}

func ancestorPeerWrite(t *testing.T, control *os.File, event ancestorPeerEvent) {
	t.Helper()
	if err := writeFrame(control, event); err != nil {
		t.Fatal(err)
	}
}

func ancestorPeerRead(t *testing.T, control *os.File) ancestorPeerEvent {
	t.Helper()
	payload, err := readFrame(control, 512)
	if err != nil {
		t.Fatal(err)
	}
	var event ancestorPeerEvent
	if err := decodeStrict(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func ancestorPeerPID(t *testing.T, control *os.File) int {
	t.Helper()
	peer, err := syscall.GetsockoptUcred(int(control.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil || peer == nil || peer.Uid != uint32(os.Geteuid()) {
		t.Fatalf("invalid kernel peer credentials: %v", err)
	}
	return int(peer.Pid)
}

func ancestorPeerOwner(t *testing.T, creator string) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	parent, child, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	defer child.Close()
	cmd := ancestorPeerCommand("C", creator, os.Getenv("HOME"))
	cmd.ExtraFiles = []*os.File{child}
	if creator == "O" {
		left, right, pairErr := socketPair()
		if pairErr != nil {
			t.Fatal(pairErr)
		}
		defer left.Close()
		defer right.Close()
		cmd.ExtraFiles = append(cmd.ExtraFiles, left, right)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for _, file := range cmd.ExtraFiles {
		file.Close()
	}
	// C stays unreaped until the watchdog is joined, pinning this owned group.
	done := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() {
		defer close(done)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = parent.Close()
	})
	stopped, timely := false, false
	stop := func() bool {
		if !stopped {
			stopped, timely = true, watchdog.Stop()
			if !timely {
				<-done
			}
		}
		return timely
	}
	defer func() {
		stop()
		if cmd.ProcessState == nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			var info unix.Siginfo
			for unix.Waitid(unix.P_PID, cmd.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil) == syscall.EINTR {
			}
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
		}
		for {
			var raw syscall.WaitStatus
			_, waitErr := syscall.Wait4(-1, &raw, 0, nil)
			if waitErr == syscall.EINTR {
				continue
			}
			if waitErr != nil {
				if waitErr != syscall.ECHILD {
					t.Errorf("drain adopted child: %v", waitErr)
				}
				break
			}
		}
	}()
	caller := ancestorPeerRead(t, parent)
	if caller != (ancestorPeerEvent{Gate: "caller", PID: cmd.Process.Pid, Parent: os.Getpid(), Peer: os.Getpid(), Accepted: true}) {
		t.Fatalf("wrong caller observation: %+v", caller)
	}
	// Both views name O. This is not symmetric C authentication: spawned PID
	// plus sameExecutable is only the separate, narrow fixture observation.
	if ancestorPeerPID(t, parent) != os.Getpid() || !sameExecutable(cmd.Process.Pid, os.Getpid()) {
		t.Fatal("wrong socket creator or launched caller executable")
	}
	ancestorPeerWrite(t, parent, ancestorPeerEvent{Gate: "continue"})
	launcher := ancestorPeerRead(t, parent)
	wantPeer := cmd.Process.Pid
	if creator == "O" {
		wantPeer = os.Getpid()
	}
	if launcher.Gate != "launcher" || launcher.PID <= 0 || launcher.Parent != cmd.Process.Pid || launcher.Peer != wantPeer || launcher.Accepted != (creator == "C") {
		t.Fatalf("wrong child peer observation: %+v", launcher)
	}
	custodyCheck(t, custodyObserve(t, cmd.Process.Pid), cmd.Process.Pid, "0")
	if !stop() {
		t.Fatal("peer watchdog expired")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	var raw syscall.WaitStatus
	if pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatalf("peer children remain: pid=%d status=%#x err=%v", pid, uint32(raw), err)
	}
}

func ancestorPeerCaller(t *testing.T, creator string) {
	control := os.NewFile(3, "peer-owner")
	defer control.Close()
	syscall.CloseOnExec(3)
	parent, err := validateInternalControl(3)
	if err != nil || parent != os.Getppid() {
		t.Fatalf("caller cannot validate ancestor peer: %v", err)
	}
	ancestorPeerWrite(t, control, ancestorPeerEvent{Gate: "caller", PID: os.Getpid(), Parent: parent, Peer: ancestorPeerPID(t, control), Accepted: true})
	if ancestorPeerRead(t, control).Gate != "continue" {
		t.Fatal("missing caller observation acknowledgement")
	}
	var left, right *os.File
	if creator == "C" {
		left, right, err = socketPair()
		if err != nil {
			t.Fatal(err)
		}
	} else {
		left, right = os.NewFile(4, "inherited-peer-left"), os.NewFile(5, "inherited-peer-right")
	}
	defer left.Close()
	defer right.Close()
	syscall.CloseOnExec(int(left.Fd()))
	syscall.CloseOnExec(int(right.Fd()))
	cmd := ancestorPeerCommand("L", creator, os.Getenv("HOME"))
	cmd.ExtraFiles = []*os.File{right}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	right.Close()
	launcher := ancestorPeerRead(t, left)
	if launcher.PID != cmd.Process.Pid || launcher.Parent != os.Getpid() {
		t.Fatalf("wrong launched child identity: %+v", launcher)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	ancestorPeerWrite(t, control, launcher)
}
