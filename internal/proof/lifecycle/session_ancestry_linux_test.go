//go:build linux && amd64

package lifecycle

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-010 / VAL-019: a kernel constraint experiment, not terminal ownership
// or a production bootstrap. All process-group changes are disposable.
func TestAncestorSessionRetainedForegroundGroup(t *testing.T) {
	cmd := sessionAncestryCommand("owner", t.TempDir(), nil)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output custodyOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("isolated session constraint: %v\n%s", err, output.Bytes())
	}
}

func sessionAncestryCommand(role, root string, control *os.File) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSessionAncestryChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_SESSION_ANCESTRY_ROLE="+role, "GORACE=atexit_sleep_ms=0")
	if control != nil {
		cmd.ExtraFiles = []*os.File{control}
	}
	return cmd
}

func TestSessionAncestryChild(t *testing.T) {
	switch os.Getenv("ZERB_SESSION_ANCESTRY_ROLE") {
	case "":
		return
	case "peer":
		control := os.NewFile(3, "session-peer-control")
		defer control.Close()
		syscall.CloseOnExec(3)
		custodyWrite(t, control, custodyEvent{Gate: "ready", PID: os.Getpid()})
		if _, err := readFrame(control, 512); err == nil {
			t.Fatal("unexpected peer message")
		}
		os.Exit(0)
	case "owner":
		sessionAncestryOwner(t)
	default:
		t.Fatal("unknown session role")
	}
}

func sessionAncestryOwner(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ownerPID := os.Getpid()
	originalSession, err := unix.Getsid(0)
	if err != nil || syscall.Getpgrp() != ownerPID {
		t.Fatalf("owner did not begin as group leader: %v", err)
	}
	var commands [2]*exec.Cmd
	var controls [2]*os.File
	defer func() {
		for i, cmd := range commands {
			if controls[i] != nil {
				_ = controls[i].Close()
			}
			if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	}()
	for i := range commands {
		parent, child, pairErr := socketPair()
		if pairErr != nil {
			t.Fatal(pairErr)
		}
		controls[i] = parent
		cmd := sessionAncestryCommand("peer", os.Getenv("HOME"), child)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if i == 1 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}
		commands[i] = cmd
		startErr := cmd.Start()
		_ = child.Close()
		if startErr != nil {
			t.Fatal(startErr)
		}
	}
	// Keep both child identities unreaped until the watchdog is stopped/joined.
	watchdogDone := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() {
		defer close(watchdogDone)
		for i, cmd := range commands {
			_ = cmd.Process.Kill()
			_ = controls[i].Close()
		}
	})
	stopped := false
	stop := func() bool {
		if stopped {
			return true
		}
		stopped = true
		ok := watchdog.Stop()
		if !ok {
			<-watchdogDone
		}
		return ok
	}
	defer stop()
	for i, cmd := range commands {
		ready := custodyRead(t, controls[i])
		if ready.Gate != "ready" || ready.PID != cmd.Process.Pid {
			t.Fatal("wrong peer identity")
		}
	}
	childPID, helperPID := commands[0].Process.Pid, commands[1].Process.Pid
	if err := syscall.Setpgid(0, helperPID); err != nil {
		t.Fatal(err)
	}
	if syscall.Getpgrp() != helperPID || helperPID == ownerPID {
		t.Fatal("owner is still a process-group leader")
	}
	if group, err := syscall.Getpgid(childPID); err != nil || group != ownerPID {
		t.Fatalf("child lost original foreground-group candidate: %v", err)
	}
	// A different current PGID is insufficient: a group with ID ownerPID exists.
	if sid, err := syscall.Setsid(); err != syscall.EPERM || sid != -1 {
		t.Fatalf("setsid with retained original group: sid=%d err=%v", sid, err)
	}
	if sid, err := unix.Getsid(0); err != nil || sid != originalSession {
		t.Fatal("refused setsid changed the session")
	}
	for i, cmd := range commands {
		_ = controls[i].Close()
		custodyCheck(t, custodyObserve(t, cmd.Process.Pid), cmd.Process.Pid, "0")
	}
	if !stop() {
		t.Fatal("session constraint watchdog expired")
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	var status syscall.WaitStatus
	if pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatal("session experiment left children")
	}
	// A real positive control excludes a blanket setsid permission failure.
	if sid, err := syscall.Setsid(); err != nil || sid != ownerPID {
		t.Fatalf("setsid after original group is gone: sid=%d err=%v", sid, err)
	}
	if sid, err := unix.Getsid(0); err != nil || sid != ownerPID || syscall.Getpgrp() != ownerPID {
		t.Fatal("successful setsid did not create the expected session and group")
	}
}
