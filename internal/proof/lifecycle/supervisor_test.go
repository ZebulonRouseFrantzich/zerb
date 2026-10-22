package lifecycle

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// These local protocol/status tests do not qualify a cgroup or an upstream tuple.
func TestManagedResultPreservesGuardWaitStatus(t *testing.T) {
	for _, guardStatus := range []int{0, 11, 128 + int(syscall.SIGKILL)} {
		t.Run(strconv.Itoa(guardStatus), func(t *testing.T) {
			guardRead, guardWrite, err := socketPair()
			if err != nil {
				t.Fatal(err)
			}
			defer guardRead.Close()
			defer guardWrite.Close()
			guard := startLifecycleTestChild(t, "guard-"+strconv.Itoa(guardStatus), guardRead)
			guardRead.Close()
			managed := startLifecycleTestChild(t, "managed", nil)
			resultRead, resultWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer resultRead.Close()
			defer resultWrite.Close()
			status := finishManagedProcess(resultWrite, nil, nil, cgroupIdentity{}, managed, make(chan os.Signal), true, func(kill ...bool) (int, error) {
				return closeDeathGuard(guardWrite, guard, kill...)
			})
			var result wireResult
			if err := json.NewDecoder(resultRead).Decode(&result); err != nil {
				t.Fatal(err)
			}
			want := 7
			if guardStatus != 0 {
				want = 1
			}
			if status != want || result.Status != want || !result.GuardStatusObserved || result.GuardStatus != guardStatus {
				t.Fatalf("status=%d result=%+v; want status=%d, observed guard=%d", status, result, want, guardStatus)
			}
		})
	}
}

func startLifecycleTestChild(t *testing.T, mode string, control *os.File) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleUnitChild$")
	cmd.Env = append(isolatedEnv(t.TempDir(), ""), "ZERB_LIFECYCLE_UNIT_CHILD="+mode)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if control != nil {
		cmd.ExtraFiles = []*os.File{control}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd
}

func TestLifecycleUnitChild(t *testing.T) {
	mode := os.Getenv("ZERB_LIFECYCLE_UNIT_CHILD")
	if mode == "" {
		return
	}
	if mode == "managed" {
		os.Exit(7)
	}
	control := os.NewFile(3, "unit-guard-control")
	payload, err := readFrame(control, 256)
	var disarm guardDisarm
	if err != nil || decodeStrict(payload, &disarm) != nil || !disarm.Disarm || disarm.Kill {
		os.Exit(90)
	}
	status, err := strconv.Atoi(strings.TrimPrefix(mode, "guard-"))
	if err != nil {
		os.Exit(91)
	}
	if status == 128+int(syscall.SIGKILL) {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
	os.Exit(status)
}

func TestLauncherCheckpointsRejectUnreleasedProtocolViolations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func([]launcherCheckpoint) []launcherCheckpoint
		deny      bool
		wantGates int
		wantErr   bool
	}{
		{name: "payload", wantGates: 5},
		{name: "manager", deny: true, wantGates: 5},
		{name: "out-of-order", wantGates: 1, wantErr: true, change: func(checks []launcherCheckpoint) []launcherCheckpoint {
			checks[1], checks[2] = checks[2], checks[1]
			return checks
		}},
		{name: "missing-final", wantGates: 4, wantErr: true, change: func(checks []launcherCheckpoint) []launcherCheckpoint { return checks[:4] }},
		{name: "launcher-error", wantGates: 2, wantErr: true, change: func(checks []launcherCheckpoint) []launcherCheckpoint {
			checks[2].Error = "group validation failed"
			return checks
		}},
		{name: "manager-filter-missing", deny: true, wantGates: 4, wantErr: true, change: func(checks []launcherCheckpoint) []launcherCheckpoint {
			checks[4].IOUringDenied = false
			return checks
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks := []launcherCheckpoint{{Name: "after-descriptor-sanitization"}, {Name: "after-parent-death-validation"}, {Name: "after-group-session-validation"}, {Name: "after-second-parent-check"}, {Name: "before-exec-release", IOUringDenied: tc.deny}}
			if tc.change != nil {
				checks = tc.change(checks)
			}
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			for _, checkpoint := range checks {
				if err := writeFrame(write, checkpoint); err != nil {
					t.Fatal(err)
				}
			}
			write.Close()
			gates := 0
			denied, err := awaitLauncherCheckpoints(read, wireSpec{DenyIOUring: tc.deny}, 123, func(_ string, pid, group int, armed bool) {
				if pid != 123 || group != 123 || !armed {
					t.Fatalf("checkpoint lost leader/group/guard identity: %d %d %t", pid, group, armed)
				}
				gates++
			})
			if (err != nil) != tc.wantErr || gates != tc.wantGates || (!tc.wantErr && denied != tc.deny) {
				t.Fatalf("denied=%t gates=%d err=%v; want denied=%t gates=%d error=%t", denied, gates, err, tc.deny, tc.wantGates, tc.wantErr)
			}
		})
	}
}

func TestProofPTYDescriptors(t *testing.T) {
	master, slave, err := openProofPTY()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	for _, file := range []*os.File{master, slave} {
		flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatalf("PTY descriptor is not close-on-exec: flags=%d err=%v", flags, err)
		}
	}
	if _, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS); err != nil {
		t.Fatalf("unlocked PTY slave is not a terminal: %v", err)
	}
}
