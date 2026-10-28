//go:build linux && amd64

package lifecycle

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-018/019/020: this disposable Go birth subset distinguishes
// creator M from natural parent F. It is not role authentication, opened-image
// dispatch, strace attachment, containment, complete custody or host qualification.
func TestCloneParentObserver(t *testing.T) {
	for _, mode := range []string{"parent", "ordinary", "wrong-death", "failed-exec"} {
		t.Run(mode, func(t *testing.T) {
			cmd := cloneParentCommand("F", mode, t.TempDir())
			var output custodyOutput
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Run(); err != nil {
				t.Fatalf("isolated clone-parent row: %v\n%s", err, output.Bytes())
			}
		})
	}
}

type cloneParentEvent struct {
	Gate                                string
	PID, Parent, SID, PGID, Signal, Err int
	OwnFD, CLOEXEC, Accepted            bool
	Raw                                 uint32
}

func cloneParentCommand(role, mode, root string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCloneParentObserverChild$")
	cmd.Env = append(isolatedEnv(root, ""), "ZERB_CLONE_ROLE="+role,
		"ZERB_CLONE_MODE="+mode, "GORACE=atexit_sleep_ms=0")
	return cmd
}

func cloneParentWrite(t *testing.T, control *os.File, event cloneParentEvent) {
	t.Helper()
	if err := writeFrame(control, event); err != nil {
		t.Fatal(err)
	}
}

func cloneParentRead(t *testing.T, control *os.File) cloneParentEvent {
	t.Helper()
	payload, err := readFrame(control, 512)
	if err != nil {
		t.Fatal(err)
	}
	var event cloneParentEvent
	if err := decodeStrict(payload, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestCloneParentObserverChild(t *testing.T) {
	mode := os.Getenv("ZERB_CLONE_MODE")
	switch os.Getenv("ZERB_CLONE_ROLE") {
	case "":
		return
	case "F":
		runtime.LockOSThread()
		cloneParentCaller(t, mode)
		if t.Failed() {
			os.Exit(1)
		}
		os.Exit(0)
	case "M":
		runtime.LockOSThread()
		cloneParentCreator(t, mode)
		if t.Failed() {
			os.Exit(1)
		}
		os.Exit(0)
	case "S":
		runtime.LockOSThread()
		control := os.NewFile(3, "observer-control")
		defer control.Close()
		syscall.CloseOnExec(3)
		parent, err := validateInternalControl(3)
		cloneParentWrite(t, control, cloneParentEvent{Gate: "observer", PID: os.Getpid(),
			Parent: os.Getppid(), SID: sessionID(), PGID: syscall.Getpgrp(),
			Signal: int(parentDeathSignal()), Accepted: err == nil && parent == os.Getppid()})
		if mode == "ordinary" {
			os.Exit(0)
		}
		if cloneParentRead(t, control).Gate != "release" {
			t.Fatal("missing observer release")
		}
		os.Exit(17)
	default:
		t.Fatal("unknown clone-parent role")
	}
}

func cloneParentCaller(t *testing.T, mode string) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
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
	cmd := cloneParentCommand("M", mode, os.Getenv("HOME"))
	cmd.ExtraFiles = []*os.File{child, observerChild}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	child.Close()
	observerChild.Close()
	// Closing F's sole socket endpoints releases held fixture children even if
	// M fails before reporting S. No unknown numeric PID/group is addressed.
	done := make(chan struct{})
	watchdog := time.AfterFunc(10*time.Second, func() {
		defer close(done)
		_ = parent.Close()
		_ = observer.Close()
		_ = cmd.Process.Kill()
	})
	stopped := false
	stop := func() bool {
		if stopped {
			return true
		}
		stopped = true
		if watchdog.Stop() {
			return true
		}
		<-done
		return false
	}
	defer func() {
		stop()
		_ = parent.Close()
		_ = observer.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		for {
			var status syscall.WaitStatus
			_, waitErr := syscall.Wait4(-1, &status, 0, nil)
			if waitErr == syscall.EINTR {
				continue
			}
			if waitErr != nil {
				if waitErr != syscall.ECHILD {
					t.Errorf("drain clone-parent children: %v", waitErr)
				}
				break
			}
		}
	}()
	created := cloneParentRead(t, parent)
	if created.Gate != "created" {
		t.Fatalf("unexpected birth event: %+v", created)
	}
	var status syscall.WaitStatus
	if mode == "failed-exec" {
		if created.PID != 0 || created.Err != int(syscall.ENOENT) || created.OwnFD || created.CLOEXEC {
			t.Fatalf("failed exec unexpectedly retained creator ownership: %+v", created)
		}
		pid, waitErr := syscall.Wait4(-1, &status, 0, nil)
		if waitErr != nil || pid <= 0 || pid == cmd.Process.Pid || !status.Exited() || status.ExitStatus() != 253 {
			t.Fatalf("original F did not reap interrupted birth: pid=%d raw=%#x err=%v", pid, uint32(status), waitErr)
		}
	} else {
		if created.PID <= 0 || created.Err != 0 || !created.OwnFD || !created.CLOEXEC {
			t.Fatalf("atomic observer pidfd absent: %+v", created)
		}
		if mode != "wrong-death" {
			seen := cloneParentRead(t, observer)
			wantParent := os.Getpid()
			if mode == "ordinary" {
				wantParent = cmd.Process.Pid
			}
			if seen.Gate != "observer" || seen.PID != created.PID || seen.Parent != wantParent ||
				seen.SID != cmd.Process.Pid || seen.PGID != seen.PID || seen.Signal != 0 || seen.Accepted != (mode == "parent") {
				t.Fatalf("wrong observer relation: %+v", seen)
			}
		}
		if mode == "parent" {
			cloneParentWrite(t, observer, cloneParentEvent{Gate: "release"})
		}
		pid, waitErr := syscall.Wait4(created.PID, &status, 0, nil)
		if mode == "ordinary" {
			if pid != -1 || waitErr != syscall.ECHILD {
				t.Fatalf("F acquired ordinary child wait: pid=%d err=%v", pid, waitErr)
			}
		} else if waitErr != nil || pid != created.PID ||
			(mode == "parent" && (!status.Exited() || status.ExitStatus() != 17)) ||
			(mode == "wrong-death" && (!status.Signaled() || status.Signal() != syscall.SIGKILL)) {
			t.Fatalf("wrong natural-parent raw wait: pid=%d raw=%#x err=%v", pid, uint32(status), waitErr)
		}
		if mode == "wrong-death" {
			if payload, err := readFrame(observer, 512); len(payload) != 0 || err != io.EOF {
				t.Fatalf("self-killed observer emitted a helper frame: bytes=%d err=%v", len(payload), err)
			}
		}
	}
	cloneParentWrite(t, parent, cloneParentEvent{Gate: "finish"})
	finished := cloneParentRead(t, parent)
	if finished.Gate != "finished" || (mode == "ordinary" && (finished.Err != 0 || finished.Raw != 0)) ||
		(mode != "ordinary" && finished.Err != int(syscall.ECHILD)) {
		t.Fatalf("wrong creator wait ownership: %+v", finished)
	}
	if !stop() {
		t.Fatal("clone-parent watchdog expired")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	pid, waitErr := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
	if pid != -1 || waitErr != syscall.ECHILD {
		t.Fatalf("clone-parent children remain: pid=%d err=%v", pid, waitErr)
	}
}

func cloneParentCreator(t *testing.T, mode string) {
	control := os.NewFile(3, "creator-control")
	defer control.Close()
	syscall.CloseOnExec(3)
	observer := os.NewFile(4, "observer-child")
	defer observer.Close()
	syscall.CloseOnExec(4)
	if _, err := validateInternalControl(3); err != nil || sessionID() != os.Getpid() {
		t.Fatalf("invalid creator setup: %v", err)
	}
	child := cloneParentCommand("S", mode, os.Getenv("HOME"))
	path := os.Args[0]
	if mode == "failed-exec" {
		path = filepath.Join(os.Getenv("HOME"), "absent-observer")
	}
	pidfd := -1
	attr := &syscall.SysProcAttr{Setpgid: true, Cloneflags: syscall.CLONE_PARENT, PidFD: &pidfd}
	if mode == "ordinary" {
		attr.Cloneflags = 0
	}
	if mode == "wrong-death" {
		attr.Pdeathsig = syscall.SIGKILL
	}
	pid, startErr := syscall.ForkExec(path, child.Args, &syscall.ProcAttr{
		Env: child.Env, Files: []uintptr{0, 1, 2, observer.Fd()}, Sys: attr})
	observer.Close()
	var startErrno syscall.Errno
	if startErr != nil && !errors.As(startErr, &startErrno) {
		t.Fatal(startErr)
	}
	created := cloneParentEvent{Gate: "created", PID: pid, Err: int(startErrno), OwnFD: pidfd >= 0}
	if pidfd >= 0 {
		defer syscall.Close(pidfd)
		flags, err := unix.FcntlInt(uintptr(pidfd), unix.F_GETFD, 0)
		if err != nil {
			t.Fatal(err)
		}
		created.CLOEXEC = flags&unix.FD_CLOEXEC != 0
		if mode == "parent" {
			if err := unix.PidfdSendSignal(pidfd, 0, nil, 0); err != nil {
				t.Fatalf("kernel-returned observer pidfd unusable: %v", err)
			}
		}
	}
	cloneParentWrite(t, control, created)
	if cloneParentRead(t, control).Gate != "finish" {
		t.Fatal("missing creator finish")
	}
	finished := cloneParentEvent{Gate: "finished"}
	if pid > 0 {
		var status syscall.WaitStatus
		got, err := syscall.Wait4(pid, &status, 0, nil)
		if err != nil {
			var errno syscall.Errno
			if !errors.As(err, &errno) {
				t.Fatal(err)
			}
			finished.Err = int(errno)
		} else if got == pid {
			finished.Raw = uint32(status)
		} else {
			t.Fatal("unexpected creator wait PID")
		}
	} else {
		// The failed exec child belongs to F despite ForkExec returning no PID.
		var status syscall.WaitStatus
		_, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err != syscall.ECHILD {
			t.Fatalf("failed creator unexpectedly has a child: %v", err)
		}
		finished.Err = int(syscall.ECHILD)
	}
	cloneParentWrite(t, control, finished)
}
