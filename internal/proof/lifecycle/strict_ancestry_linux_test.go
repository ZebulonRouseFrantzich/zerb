//go:build linux && amd64

package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-019/020: disposable O(subreaper)->C->L models only
// ordered actual-C-death-before-continuation schedules. This is not authenticated
// dispatch, product FD policy, cgroup cleanup, a complete topology, or proof of
// zero candidate instructions in all concurrent death/exec races.
func TestStrictDirectAncestry(t *testing.T) {
	for _, row := range []struct{ gate, death string }{
		{"before-final-check", "enabled"},
		{"after-final-check", "enabled"},
		{"after-final-check", "disabled"},
		{"after-final-check", "live"},
	} {
		t.Run(row.gate+"/"+row.death, func(t *testing.T) {
			cmd := ancestryCommand("O", row.gate, row.death, t.TempDir())
			var output custodyOutput
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Run(); err != nil {
				t.Fatalf("isolated ancestry row: %v\n%s", err, output.Bytes())
			}
		})
	}
}

type ancestryEvent struct {
	Gate   string `json:"gate"`
	PID    int    `json:"pid"`
	Parent int    `json:"parent"`
	Signal int    `json:"signal"`
}

func ancestryCommand(role, gate, death, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestStrictAncestryChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_ANCESTRY_ROLE="+role,
		"ZERB_ANCESTRY_GATE="+gate, "ZERB_ANCESTRY_DEATH="+death,
		"GORACE=atexit_sleep_ms=0")
	return cmd
}

func TestStrictAncestryChild(t *testing.T) {
	gate, death := os.Getenv("ZERB_ANCESTRY_GATE"), os.Getenv("ZERB_ANCESTRY_DEATH")
	switch os.Getenv("ZERB_ANCESTRY_ROLE") {
	case "":
		return
	case "O":
		ancestryOwner(t, gate, death)
	case "C":
		// Do not return/unlock the L-creating thread before actual process exit.
		runtime.LockOSThread()
		defer os.Exit(1)
		ancestryCreator(t, gate, death)
		os.Exit(0)
	case "L":
		ancestryLauncher(t, gate, death)
	case "target":
		marker := os.NewFile(4, "ancestry-marker")
		defer marker.Close()
		if _, err := fmt.Fprintf(marker, "exec:%d\n", os.Getpid()); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown ancestry role")
	}
}

func ancestryRead(t *testing.T, control *os.File) ancestryEvent {
	t.Helper()
	payload, err := readFrame(control, 512)
	if err != nil {
		t.Fatal(err)
	}
	var event ancestryEvent
	if err := decodeStrict(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func ancestryWrite(t *testing.T, control *os.File, event ancestryEvent) {
	t.Helper()
	if err := writeFrame(control, event); err != nil {
		t.Fatal(err)
	}
}

func ancestryOwner(t *testing.T, gate, death string) {
	t.Helper()
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	ownerControl, creatorControl, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer ownerControl.Close()
	defer creatorControl.Close()
	launcherControl, childControl, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer launcherControl.Close()
	defer childControl.Close()
	markerRead, markerWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer markerRead.Close()
	defer markerWrite.Close()
	creator := ancestryCommand("C", gate, death, os.Getenv("HOME"))
	creator.ExtraFiles = []*os.File{creatorControl, childControl, markerWrite}
	creator.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	creator.Stdout, creator.Stderr = os.Stdout, os.Stderr
	if err := creator.Start(); err != nil {
		t.Fatal(err)
	}
	creatorControl.Close()
	childControl.Close()
	markerWrite.Close()
	// Only this owned C group is addressed. The timer never kills O; O must
	// remain alive to reap. Expiry is failure, not a product cleanup deadline.
	watchdogDone := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() {
		defer close(watchdogDone)
		_ = syscall.Kill(-creator.Process.Pid, syscall.SIGKILL)
		_ = ownerControl.Close()
		_ = launcherControl.Close()
		_ = markerRead.Close()
	})
	stopped := false
	stopWatchdog := func() bool {
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
	defer func() {
		stopWatchdog()
		if creator.ProcessState == nil {
			_ = syscall.Kill(-creator.Process.Pid, syscall.SIGKILL)
			// Once C has exited it cannot spawn another child. Retain its
			// zombie through the last group kill, then reap every adoption.
			var info unix.Siginfo
			for unix.Waitid(unix.P_PID, creator.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil) == syscall.EINTR {
			}
			_ = syscall.Kill(-creator.Process.Pid, syscall.SIGKILL)
			_ = creator.Wait()
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
	born := ancestryRead(t, ownerControl)
	if born.Gate != "born" || born.PID <= 0 || born.Parent != creator.Process.Pid {
		t.Fatalf("invalid launcher birth: %+v", born)
	}
	reached := ancestryRead(t, launcherControl)
	wantSignal := int(syscall.SIGKILL)
	if death == "disabled" {
		wantSignal = 0
	}
	if reached != (ancestryEvent{Gate: gate, PID: born.PID, Parent: creator.Process.Pid, Signal: wantSignal}) {
		t.Fatalf("wrong held launcher checkpoint: %+v", reached)
	}
	if death != "live" {
		if err := creator.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		// This exact kernel exit observation precedes any release attempt.
		custodyCheck(t, custodyObserve(t, creator.Process.Pid), creator.Process.Pid, "kill")
	}
	// Attempt continuation before waiting for L and before any cleanup kill.
	// A killed L may already have closed the endpoint; EPIPE is expected then.
	releaseErr := writeFrame(launcherControl, ancestryEvent{Gate: "continue"})
	if death == "live" || death == "disabled" {
		if releaseErr != nil {
			t.Fatal(releaseErr)
		}
	} else if releaseErr != nil && !errors.Is(releaseErr, syscall.EPIPE) && !errors.Is(releaseErr, syscall.ECONNRESET) {
		t.Fatal(releaseErr)
	}
	if death != "live" {
		status := "kill"
		if death == "disabled" {
			status = "0"
		}
		observed := custodyObserve(t, born.PID)
		custodyCheck(t, observed, born.PID, status)
		custodyConsume(t, born.PID, observed)
	} else {
		ancestryWrite(t, ownerControl, ancestryEvent{Gate: "finish"})
		custodyCheck(t, custodyObserve(t, creator.Process.Pid), creator.Process.Pid, "0")
	}
	marker, err := io.ReadAll(io.LimitReader(markerRead, 513))
	if err != nil {
		t.Fatal(err)
	}
	wantMarker := []byte(nil)
	if death == "disabled" || death == "live" {
		wantMarker = []byte("exec:" + strconv.Itoa(born.PID) + "\n")
	}
	if !bytes.Equal(marker, wantMarker) {
		t.Fatalf("target entry marker %q; want %q", marker, wantMarker)
	}
	if !stopWatchdog() {
		t.Fatal("ancestry watchdog expired")
	}
	err = creator.Wait()
	if death == "live" {
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil || creator.ProcessState.Sys().(syscall.WaitStatus) != syscall.WaitStatus(syscall.SIGKILL) {
		t.Fatalf("wrong C wait status: %v", err)
	}
	var raw syscall.WaitStatus
	if pid, err := syscall.Wait4(-1, &raw, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatalf("ancestry children remain: pid=%d raw=%#x err=%v", pid, uint32(raw), err)
	}
}

func ancestryCreator(t *testing.T, gate, death string) {
	t.Helper()
	control := os.NewFile(3, "ancestry-owner")
	defer control.Close()
	launcherControl := os.NewFile(4, "ancestry-launcher")
	defer launcherControl.Close()
	marker := os.NewFile(5, "ancestry-marker")
	defer marker.Close()
	for _, fd := range []int{3, 4, 5} {
		syscall.CloseOnExec(fd)
	}
	cmd := ancestryCommand("L", gate, death, os.Getenv("HOME"))
	cmd.ExtraFiles = []*os.File{launcherControl, marker}
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if death != "disabled" {
		cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	launcherControl.Close()
	marker.Close()
	ancestryWrite(t, control, ancestryEvent{Gate: "born", PID: cmd.Process.Pid, Parent: os.Getpid()})
	if event := ancestryRead(t, control); event.Gate != "finish" || death != "live" {
		t.Fatal("unexpected creator continuation")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func ancestryLauncher(t *testing.T, gate, death string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	control := os.NewFile(3, "ancestry-owner")
	defer control.Close()
	syscall.CloseOnExec(3)
	parent := os.Getppid()
	wantSignal := syscall.SIGKILL
	if death == "disabled" {
		wantSignal = 0
	}
	// The Go runtime may enter this helper on another thread than fork/exec.
	// Bind the child-side relation and all remaining checks/exec to this thread.
	if inherited := parentDeathSignal(); inherited != 0 && inherited != wantSignal {
		t.Fatal("unexpected inherited parent-death signal")
	} else if inherited == 0 && wantSignal != 0 {
		if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(wantSignal), 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	checkParent := func() {
		if os.Getppid() != parent {
			t.Fatal("creator changed before final check")
		}
	}
	checkParent()
	if parentDeathSignal() != wantSignal {
		t.Fatal("wrong direct parent-death signal")
	}
	if gate == "after-final-check" {
		checkParent()
	}
	ancestryWrite(t, control, ancestryEvent{Gate: gate, PID: os.Getpid(), Parent: parent, Signal: int(wantSignal)})
	if event := ancestryRead(t, control); event.Gate != "continue" {
		t.Fatal("unexpected launcher continuation")
	}
	if gate == "before-final-check" {
		checkParent()
	}
	// The final-gap control intentionally has no later parent check. Only the
	// installed kernel relation can close this ordered check-to-exec gap.
	target := ancestryCommand("target", gate, death, os.Getenv("HOME"))
	if err := syscall.Exec(os.Args[0], target.Args, target.Env); err != nil {
		t.Fatal(err)
	}
}
