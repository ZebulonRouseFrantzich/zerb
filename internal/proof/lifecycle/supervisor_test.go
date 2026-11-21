package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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
	cmd.Env = append(isolatedEnv(t.TempDir(), ""), "ZERB_LIFECYCLE_UNIT_CHILD="+mode, "GORACE=atexit_sleep_ms=0")
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
	if strings.HasPrefix(mode, "exit-") {
		status, err := strconv.Atoi(strings.TrimPrefix(mode, "exit-"))
		if err != nil {
			t.Fatal(err)
		}
		os.Exit(status)
	}
	if strings.HasPrefix(mode, "scenario-") {
		switch strings.TrimPrefix(mode, "scenario-") {
		case "tree":
			testSharedGraceTree(t)
		case "expired":
			testExpiredGrace(t)
		case "cgroup":
			testCgroupVerificationAfterGrace(t)
		case "context":
			testContextSignalPrecedence(t)
		case "late-adopted":
			testLateAdoptedChildReaping(t)
		}
		return
	}
	if mode == "adopting-leader" || mode == "release-child" {
		control := os.NewFile(3, "unit-adoption-control")
		if mode == "adopting-leader" {
			child := exec.Command(os.Args[0], "-test.run=^TestLifecycleUnitChild$")
			child.Env = append(isolatedEnv(os.Getenv("HOME"), ""), "ZERB_LIFECYCLE_UNIT_CHILD=release-child", "GORACE=atexit_sleep_ms=0")
			child.ExtraFiles = []*os.File{control}
			child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			os.Exit(7)
		}
		if err := writeFrame(control, os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if _, err := readFrame(control, 256); err != nil {
			t.Fatal(err)
		}
		os.Exit(9)
	}
	if mode == "tree-leader" || mode == "ignore" || mode == "cooperate" || mode == "observe-signals" {
		control := os.NewFile(3, "unit-process-ready")
		if mode == "ignore" {
			signal.Ignore(catchableSignals()...)
			_, _ = control.Write([]byte{1})
			for {
				time.Sleep(time.Hour)
			}
		}
		sigs := make(chan os.Signal, 4)
		signal.Notify(sigs, catchableSignals()...)
		if mode == "observe-signals" {
			_, _ = control.Write([]byte{1})
			for sig := range sigs {
				_, _ = control.Write([]byte{byte(sig.(syscall.Signal))})
			}
		}
		if mode == "cooperate" {
			_, _ = control.Write([]byte{1})
			<-sigs
			os.Exit(0)
		}
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		descendant := exec.Command(os.Args[0], "-test.run=^TestLifecycleUnitChild$")
		descendant.Env = append(isolatedEnv(os.Getenv("HOME"), ""), "ZERB_LIFECYCLE_UNIT_CHILD=ignore", "GORACE=atexit_sleep_ms=0")
		descendant.ExtraFiles = []*os.File{write}
		if err := descendant.Start(); err != nil {
			t.Fatal(err)
		}
		write.Close()
		var ready [1]byte
		if _, err := io.ReadFull(read, ready[:]); err != nil {
			t.Fatal(err)
		}
		read.Close()
		_, _ = control.Write(ready[:])
		<-sigs
		time.Sleep(GracePeriod * 4 / 5)
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

// ARCH-010 / VAL-019/020: exact deadline checks complement disposable process
// timing checks; these are not a scheduler or total-cleanup latency guarantee.
func TestManagedTerminationDeadline(t *testing.T) {
	start := time.Now()
	var termination managedTermination
	termination.begin(start)
	for _, elapsed := range []time.Duration{0, GracePeriod / 2, GracePeriod, 2 * GracePeriod} {
		termination.begin(start.Add(elapsed))
		if want := start.Add(GracePeriod); termination.deadline != want {
			t.Fatalf("grace renewed at %s: got %s, want %s", elapsed, termination.deadline, want)
		}
		if got, want := termination.expired(start.Add(elapsed)), elapsed >= GracePeriod; got != want {
			t.Fatalf("grace expiry at %s: got %t, want %t", elapsed, got, want)
		}
	}
}

func TestSupervisorExitStatusPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		child      int
		requested  syscall.Signal
		canceled   bool
		cleanupErr bool
		guard      int
		want       int
		wantErr    bool
	}{
		{name: "child", child: 255, want: 255},
		{name: "context-only", child: 143, canceled: true, want: 1},
		{name: "signal", child: 7, requested: syscall.SIGINT, want: 130},
		{name: "signal-over-context", child: 143, requested: syscall.SIGINT, canceled: true, want: 130},
		{name: "cleanup-over-signal", child: 1, requested: syscall.SIGINT, cleanupErr: true, want: 1, wantErr: true},
		{name: "guard-over-signal", child: 1, requested: syscall.SIGINT, guard: 11, want: 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, err := supervisorExitStatus(wireResult{Status: tc.child, GuardStatusObserved: true, GuardStatus: tc.guard, CleanupFailed: tc.cleanupErr}, tc.requested, tc.canceled)
			if status != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("status=%d err=%v; want status=%d error=%t", status, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestManagedExitAndCatchableStatus(t *testing.T) {
	for _, status := range []int{0, 1, 2, 125, 126, 127, 128, 129, 130, 143, 255} {
		t.Run("exit-"+strconv.Itoa(status), func(t *testing.T) {
			managed := startLifecycleTestChild(t, "exit-"+strconv.Itoa(status), nil)
			result := testManagedResult(t, managed, nil, nil, true, func(...bool) (int, error) { return 0, nil })
			if result.Status != status || result.Forwarded || result.Escalated {
				t.Fatalf("normal exit %d: %+v", status, result)
			}
		})
	}
	for _, sig := range catchableSignals() {
		t.Run(sig.String(), func(t *testing.T) {
			managed := readyLifecycleChild(t, "cooperate")
			sigs := make(chan os.Signal, 1)
			sigs <- sig
			result := testManagedResult(t, managed, sigs, nil, true, func(...bool) (int, error) { return 0, nil })
			if result.Status != 128+int(sig.(syscall.Signal)) || !result.Forwarded || result.Escalated {
				t.Fatalf("catchable signal %s: %+v", sig, result)
			}
		})
	}
}

func testManagedResult(t *testing.T, managed *exec.Cmd, sigs <-chan os.Signal, cgroup *os.File, inherited bool, closeGuard func(...bool) (int, error)) wireResult {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	status := finishManagedProcess(write, cgroup, nil, cgroupIdentity{}, managed, sigs, inherited, closeGuard)
	var result wireResult
	if err := json.NewDecoder(read).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if status != result.Status {
		t.Fatalf("returned status %d differs from result %+v", status, result)
	}
	return result
}

func TestManagedCgroupRefusesUnverifiedDisarm(t *testing.T) {
	for _, scenario := range []string{"missing-count", "malformed-count", "kill-failure", "grace-kill-failure"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			if scenario != "missing-count" {
				count := "invalid\n"
				switch scenario {
				case "kill-failure":
					count = "1\n"
				case "grace-kill-failure":
					count = "0\n"
				}
				if err := os.WriteFile(filepath.Join(root, "pids.current"), []byte(count), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cgroup, err := os.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer cgroup.Close()
			guardKilled, guardCalls := false, 0
			managed := startLifecycleTestChild(t, "managed", nil)
			var sigs chan os.Signal
			if scenario == "grace-kill-failure" {
				_ = managed.Wait()
				managed = readyLifecycleChild(t, "ignore")
				sigs = make(chan os.Signal, 1)
				sigs <- syscall.SIGTERM
			}
			result := testManagedResult(t, managed, sigs, cgroup, false, func(kill ...bool) (int, error) {
				guardCalls++
				guardKilled = len(kill) != 0 && kill[0]
				return 0, nil
			})
			if result.Status != 1 || !result.CleanupFailed || !result.GuardStatusObserved || !guardKilled || guardCalls != 1 {
				t.Fatalf("unverified cleanup disarmed guard: kill=%t result=%+v", guardKilled, result)
			}
		})
	}
}

func TestManagedSharedGrace(t *testing.T) {
	for _, scenario := range []string{"tree", "expired", "cgroup", "context", "late-adopted"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleUnitChild$")
			cmd.Env = append(isolatedEnv(t.TempDir(), ""), "ZERB_LIFECYCLE_UNIT_CHILD=scenario-"+scenario, "GORACE=atexit_sleep_ms=0")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("isolated scenario: %v\n%s", err, output)
			}
		})
	}
}

func testLateAdoptedChildReaping(t *testing.T) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	parent, child, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	defer child.Close()
	managed := startLifecycleTestChild(t, "adopting-leader", child)
	child.Close()
	payload, err := readFrame(parent, 256)
	if err != nil {
		t.Fatal(err)
	}
	var adoptedPID int
	if err := decodeStrict(payload, &adoptedPID); err != nil || adoptedPID <= 0 {
		t.Fatalf("adopted child identity: pid=%d err=%v", adoptedPID, err)
	}
	adoptedReaped, released := false, false
	defer func() {
		if !adoptedReaped {
			_ = syscall.Kill(adoptedPID, syscall.SIGKILL)
		}
	}()
	defer func() {
		if !released {
			_ = writeFrame(parent, true)
		}
	}()
	guardRead, guardWrite, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer guardRead.Close()
	defer guardWrite.Close()
	guard := startLifecycleTestChild(t, "guard-0", guardRead)
	guardRead.Close()
	root := t.TempDir()
	// Disposable descriptor fixture models emptiness before child waitability.
	if err := os.WriteFile(filepath.Join(root, "pids.current"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cgroup, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cgroup.Close()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	guardBoundary := make(chan error, 1)
	tids := make(chan int, 1)
	finished := make(chan int, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tids <- unix.Gettid()
		finished <- finishManagedProcess(write, cgroup, nil, cgroupIdentity{}, managed, nil, false, func(kill ...bool) (int, error) {
			status, err := closeDeathGuard(guardWrite, guard, kill...)
			var waitStatus syscall.WaitStatus
			pid, waitErr := syscall.Wait4(adoptedPID, &waitStatus, syscall.WNOHANG, nil)
			if err == nil && (waitErr != nil || pid != 0) {
				err = fmt.Errorf("child not at adopted nonwaitable boundary: pid=%d err=%v", pid, waitErr)
			}
			if inheritedReaped, inheritedErr := reapManagedChildren(true); err == nil && (inheritedErr != nil || inheritedReaped != 0) {
				err = errors.New("inherited cleanup consumed outer-owned child")
			}
			guardBoundary <- err
			return status, err
		})
	}()
	tid := <-tids
	if err := <-guardBoundary; err != nil {
		t.Fatal(err)
	}
	// Observe the actual blocking syscall before releasing the real child. A
	// WNOHANG-only implementation returns early and cannot reach this boundary.
	watchdog := time.Now().Add(10 * time.Second)
	for {
		select {
		case status := <-finished:
			t.Fatalf("cleanup returned %d before adopted child became waitable", status)
		default:
		}
		state, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(tid) + "/syscall")
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(state))
		if len(fields) >= 4 && fields[0] == strconv.Itoa(syscall.SYS_WAIT4) && fields[1] == "0xffffffffffffffff" && fields[3] == "0x0" {
			break
		}
		if time.Now().After(watchdog) {
			t.Fatal("final blocking child wait was not observed before watchdog")
		}
		time.Sleep(time.Millisecond)
	}
	if err := writeFrame(parent, true); err != nil {
		t.Fatal(err)
	}
	released = true
	if status := <-finished; status != 7 {
		t.Fatalf("status=%d; want original leader status 7", status)
	}
	adoptedReaped = true
	var result wireResult
	if err := json.NewDecoder(read).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Reaped != 1 || !result.GuardStatusObserved || result.GuardStatus != 0 || result.CleanupFailed {
		t.Fatalf("late adopted reap or owned guard status missing: %+v", result)
	}
	var status syscall.WaitStatus
	if pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil); pid != -1 || err != syscall.ECHILD {
		t.Fatalf("final wait did not reach ECHILD: pid=%d err=%v", pid, err)
	}
}

func readyLifecycleChild(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	child := startLifecycleTestChild(t, mode, write)
	write.Close()
	var ready [1]byte
	if _, err := io.ReadFull(read, ready[:]); err != nil {
		t.Fatal(err)
	}
	return child
}

func testSharedGraceTree(t *testing.T) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	managed := readyLifecycleChild(t, "tree-leader")
	t.Cleanup(func() {
		_ = syscall.Kill(-managed.Process.Pid, syscall.SIGKILL)
		_ = reapAvailable()
	})
	sigs := make(chan os.Signal, 4)
	sigs <- syscall.SIGINT
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(GracePeriod / 3)
		sigs <- syscall.SIGHUP
		sigs <- syscall.SIGTERM
	}()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pids.current"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	kill := filepath.Join(root, "cgroup.kill")
	if err := os.WriteFile(kill, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cgroup, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cgroup.Close()
	started := time.Now()
	status := finishManagedProcess(write, cgroup, nil, cgroupIdentity{}, managed, sigs, false, func(...bool) (int, error) { return 0, nil })
	elapsed := time.Since(started)
	<-done
	var result wireResult
	if err := json.NewDecoder(read).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if status != 130 || result.Status != 130 || !result.Forwarded || !result.Escalated || result.Reaped != 1 || result.Orphans != 0 || processGroupExists(managed.Process.Pid) {
		t.Fatalf("status=%d result=%+v group remains=%t", status, result, processGroupExists(managed.Process.Pid))
	}
	if written, err := os.ReadFile(kill); err != nil || string(written) != "1" {
		t.Fatalf("group escalation missed cumulative cgroup kill: %q, %v", written, err)
	}
	if elapsed < GracePeriod {
		t.Fatalf("shared grace ended early: %s", elapsed)
	}
	t.Logf("disposable shared-grace tree cleanup: %s; scheduler-dependent observation", elapsed)
}

func testExpiredGrace(t *testing.T) {
	managed := readyLifecycleChild(t, "ignore")
	termination := managedTermination{deadline: time.Now().Add(-time.Second), requested: syscall.SIGTERM}
	deadline := termination.deadline
	started := time.Now()
	reaped, orphans, escalated := terminateAndReap(managed.Process.Pid, &termination, nil, func() {
		_ = syscall.Kill(-managed.Process.Pid, syscall.SIGKILL)
	})
	if termination.deadline != deadline || !termination.expired(started) {
		t.Fatal("group cleanup renewed expired grace")
	}
	t.Logf("disposable expired-grace cleanup: %s; scheduler-dependent observation", time.Since(started))
	if reaped != 1 || orphans != 0 || !escalated || processGroupExists(managed.Process.Pid) {
		t.Fatalf("reaped=%d orphans=%d escalated=%t", reaped, orphans, escalated)
	}
}

func testCgroupVerificationAfterGrace(t *testing.T) {
	// Regular disposable files exercise descriptor cleanup sequencing only.
	root := t.TempDir()
	current := filepath.Join(root, "pids.current")
	kill := filepath.Join(root, "cgroup.kill")
	if err := os.WriteFile(current, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kill, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cgroup, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cgroup.Close()
	managed := startLifecycleTestChild(t, "managed", nil)
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	cleared := make(chan error, 1)
	go func() {
		for {
			info, err := os.Stat(kill)
			if err != nil {
				cleared <- err
				return
			}
			if info.Size() != 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(GracePeriod + 20*time.Millisecond)
		file, err := os.OpenFile(current, os.O_WRONLY, 0)
		if err == nil {
			_, err = file.WriteAt([]byte("0\n"), 0)
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
		}
		cleared <- err
	}()
	guardCalled := false
	status := finishManagedProcess(write, cgroup, nil, cgroupIdentity{}, managed, nil, false, func(kill ...bool) (int, error) {
		guardCalled = true
		members, err := cgroupDescriptorHasMembers(cgroup)
		if err != nil || members || len(kill) != 0 {
			return 0, errors.New("guard disarmed before verified emptiness")
		}
		return 0, nil
	})
	if err := <-cleared; err != nil {
		t.Fatal(err)
	}
	var result wireResult
	if err := json.NewDecoder(read).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if status != 7 || !guardCalled || !result.Escalated || !result.GuardStatusObserved || result.GuardStatus != 0 || result.Orphans != 0 {
		t.Fatalf("status=%d guard called=%t result=%+v", status, guardCalled, result)
	}
}

func testContextSignalPrecedence(t *testing.T) {
	for _, signals := range [][]os.Signal{nil, {syscall.SIGINT, syscall.SIGTERM}} {
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer read.Close()
		defer write.Close()
		managed := startLifecycleTestChild(t, "observe-signals", write)
		write.Close()
		var ready [1]byte
		if _, err := io.ReadFull(read, ready[:]); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sigs := make(chan os.Signal, 4)
		wait := make(chan error, 1)
		observed := make(chan error, 1)
		go func() {
			var received [1]byte
			_, err := io.ReadFull(read, received[:])
			if err == nil && received[0] != byte(syscall.SIGTERM) {
				err = errors.New("context did not forward SIGTERM")
			}
			observed <- err
			for _, sig := range signals {
				sigs <- sig
			}
			_ = managed.Process.Kill()
			wait <- managed.Wait()
		}()
		requested, canceled, forwarded := awaitSupervisor(ctx, managed.Process, wait, sigs)
		if err := <-observed; err != nil {
			t.Fatal(err)
		}
		want := syscall.Signal(0)
		if len(signals) != 0 {
			want = syscall.SIGINT
		}
		if requested != want || !canceled || !forwarded {
			t.Fatalf("requested=%s canceled=%t forwarded=%t; want requested=%s", requested, canceled, forwarded, want)
		}
	}
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
