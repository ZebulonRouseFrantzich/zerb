//go:build linux

package safefs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func replacementOwnedFDs(s *replacementState) []int {
	fds := []int{s.previous.fd, int(s.reader.Fd()), s.stage.fd, s.proc.self.fd}
	for _, e := range s.chain {
		fds = append(fds, e.fd)
	}
	for _, e := range s.proc.chain {
		fds = append(fds, e.fd)
	}
	if s.stage.pin != nil {
		fds = append(fds, s.stage.pin.fd)
	}
	return fds
}

func TestReplacementDescriptorFlagsAndNoLeaks(t *testing.T) {
	root, parent, s := replacementFixture(t, []byte("complete successor"), 0o600)
	if err := s.link(); err != nil {
		t.Fatal(err)
	}
	flags, err := unix.FcntlInt(s.reader.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY || flags&unix.O_NONBLOCK == 0 {
		t.Fatal("predecessor reader widened approved read-only nonblocking policy")
	}
	for _, fd := range replacementOwnedFDs(s) {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("replacement retained inheritable descriptor")
		}
		stamp, err := observeFD(fd)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestReplacementKernelSubprocess$")
		cmd.Env = append(os.Environ(), "ZERB_SAFEFS_REPLACEMENT_HELPER=exec",
			"ZERB_SAFEFS_REPLACEMENT_FD="+strconv.Itoa(fd),
			"ZERB_SAFEFS_REPLACEMENT_INODE="+strconv.FormatUint(stamp.Inode, 10))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("replacement descriptor survived exec: %s", output)
		}
	}
	if err := s.closeResources(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 40 {
		previous, err := observeAt(parent.fd, "target")
		if err != nil {
			t.Fatal(err)
		}
		chain, err := duplicateChain([]edge{parent})
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := replacementAt(chain, "target", ".next", []byte("next complete successor"), 0o600, previous)
		if err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if err := candidate.closeResources(); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := candidate.link(); err != nil {
				t.Fatal(err)
			}
			if _, err := candidate.publish(); err != nil {
				t.Fatal(err)
			}
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(after) != len(before) {
		t.Fatal("replacement leaked descriptors")
	}
	if _, err := os.Stat(filepath.Join(root, ".temporary")); err != nil {
		t.Fatal("FD-only Close removed abandoned linked stage")
	}
}

// ARCH-020/030, VAL-027: resource/process changes are confined to children.
func TestReplacementKernelSubprocess(t *testing.T) {
	mode := os.Getenv("ZERB_SAFEFS_REPLACEMENT_HELPER")
	if mode == "" {
		for _, mode := range []string{"partial-write", "resources", "umask"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestReplacementKernelSubprocess$")
			cmd.Env = append(os.Environ(), "ZERB_SAFEFS_REPLACEMENT_HELPER="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("isolated replacement check failed: %s", output)
			}
		}
		return
	}
	if mode == "exec" {
		fd, err := strconv.Atoi(os.Getenv("ZERB_SAFEFS_REPLACEMENT_FD"))
		if err != nil {
			t.Fatal(err)
		}
		inode, err := strconv.ParseUint(os.Getenv("ZERB_SAFEFS_REPLACEMENT_INODE"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if id, err := observeFD(fd); err == nil && id.Inode == inode {
			t.Fatal("replacement descriptor survived exec")
		}
		return
	}
	if mode == "death" {
		replacementDeathChild(t)
		return
	}
	root, parent := localFixture(t)
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("prior"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := observeFile(parent.fd, "target")
	if err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "partial-write":
		var limit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		limit.Cur = 8
		if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		signal.Ignore(syscall.SIGXFSZ)
		// Retain the anonymous inode to independently observe the kernel partial IO.
		p, err := openPublication(parent, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		writeErr := p.write([]byte("complete payload exceeds the kernel limit"))
		var got [64]byte
		n, readErr := unix.Pread(p.fd, got[:], 0)
		closeErr := p.close()
		if writeErr == nil || readErr != nil || closeErr != nil || n != 8 {
			t.Fatal("kernel did not produce an actual partial anonymous write")
		}
		chain, err := duplicateChain([]edge{parent})
		if err != nil {
			t.Fatal(err)
		}
		s, err := replacementAt(chain, "target", ".temporary", []byte("complete payload exceeds the kernel limit"), 0o600, previous.identity)
		if s != nil || !errors.Is(err, errDirectory) {
			t.Fatal("partial staging write returned a capability")
		}
	case "umask":
		unix.Umask(0o777)
		chain, err := duplicateChain([]edge{parent})
		if err != nil {
			t.Fatal(err)
		}
		s, err := replacementAt(chain, "target", ".temporary", []byte("complete"), 0o600, previous.identity)
		if s != nil || !errors.Is(err, errDirectory) {
			t.Fatal("restrictive umask was repaired or admitted")
		}
	case "resources":
		var original unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
			t.Fatal(err)
		}
		failed, succeeded := false, false
		for slots := range 18 {
			chain, err := duplicateChain([]edge{parent})
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			maxFD := 0
			for _, entry := range entries {
				fd, err := strconv.Atoi(entry.Name())
				if err != nil {
					t.Fatal(err)
				}
				if fd > maxFD {
					maxFD = fd
				}
			}
			limit := original
			limit.Cur = uint64(maxFD + slots)
			if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
				t.Fatal(err)
			}
			s, stageErr := replacementAt(chain, "target", ".temporary", []byte("complete"), 0o600, previous.identity)
			if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
				t.Fatal(err)
			}
			if stageErr != nil {
				failed = true
				if s != nil || !errors.Is(stageErr, errDirectory) {
					t.Fatal("descriptor exhaustion returned invalid results")
				}
			} else {
				succeeded = true
				if err := s.closeResources(); err != nil {
					t.Fatal(err)
				}
			}
			after, err := os.ReadDir("/proc/self/fd")
			if err != nil || len(after) != len(entries)-1 {
				t.Fatal("exhaustion path leaked owned descriptors")
			}
		}
		if !failed || !succeeded {
			t.Fatal("resource matrix did not exercise both refusal and success")
		}
	default:
		t.Fatal("unknown replacement subprocess mode")
	}
	after, err := observeFile(parent.fd, "target")
	entries, listing := os.ReadDir(root)
	if err != nil || after != previous || listing != nil || len(entries) != 1 {
		t.Fatal("refused staging changed predecessor or left named residue")
	}
}

func replacementDeathChild(t *testing.T) {
	root := os.Getenv("ZERB_SAFEFS_REPLACEMENT_ROOT")
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	parent := edge{fd: fd, id: id}
	previous, err := observeAt(fd, "target")
	if err != nil {
		t.Fatal(err)
	}
	s, err := replacementAt([]edge{parent}, "target", ".temporary", []byte("complete successor"), 0o600, previous)
	if err != nil {
		t.Fatal(err)
	}
	phase, err := strconv.Atoi(os.Getenv("ZERB_SAFEFS_REPLACEMENT_PHASE"))
	if err != nil {
		t.Fatal(err)
	}
	phases := []func() error{
		func() error { s.linked = true; return s.stage.link(".temporary") },
		s.stage.syncParent, s.verifyStage, s.rename, s.stage.syncParent, s.verifyStage,
	}
	for _, action := range phases[:phase] {
		if err := action(); err != nil {
			t.Fatal("crash fixture failed before requested boundary")
		}
	}
	if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
		t.Fatal(err)
	}
	for {
		if err := unix.Pause(); err != nil && !errors.Is(err, unix.EINTR) {
			t.Fatal(err)
		}
	}
}

// Process-death visibility is not power-loss or filesystem qualification.
func TestReplacementKernelProcessDeathBoundaries(t *testing.T) {
	for phase := range 7 {
		t.Run(strconv.Itoa(phase), func(t *testing.T) {
			root, parent := localFixture(t)
			if err := os.WriteFile(filepath.Join(root, "target"), []byte("prior"), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := observeFile(parent.fd, "target")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestReplacementKernelSubprocess$")
			cmd.Env = append(os.Environ(), "ZERB_SAFEFS_REPLACEMENT_HELPER=death",
				"ZERB_SAFEFS_REPLACEMENT_ROOT="+root, "ZERB_SAFEFS_REPLACEMENT_PHASE="+strconv.Itoa(phase))
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			reaped := false
			t.Cleanup(func() {
				if !reaped {
					if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
						t.Error(err)
					}
					select {
					case <-waited:
					case <-time.After(10 * time.Second):
						t.Error("crash fixture did not reap")
					}
				}
			})
			ready := make(chan bool, 1)
			go func() {
				line, err := bufio.NewReader(stdout).ReadString('\n')
				ready <- err == nil && line == "ready\n"
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatalf("crash fixture failed: %s", stderr.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatal("crash fixture did not reach boundary")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-waited:
				reaped = true
				status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
				if err == nil || !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatal("crash fixture did not die by SIGKILL")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("crash fixture did not reap")
			}
			wantTarget := "prior"
			wantEntries := 1
			if phase >= 1 && phase <= 3 {
				wantEntries = 2
				got, err := os.ReadFile(filepath.Join(root, ".temporary"))
				if err != nil || string(got) != "complete successor" {
					t.Fatal("linked crash prefix lost complete stage")
				}
			}
			if phase >= 4 {
				wantTarget = "complete successor"
			}
			got, err := os.ReadFile(filepath.Join(root, "target"))
			entries, listing := os.ReadDir(root)
			if err != nil || string(got) != wantTarget || listing != nil || len(entries) != wantEntries {
				t.Fatal("hard death left unexpected namespace/content prefix")
			}
			after, err := observeFile(parent.fd, "target")
			if err != nil || after.identity.Links != 1 || after.identity.Mode != unix.S_IFREG|0o600 ||
				(phase < 4 && before != after) || (phase >= 4 && before.identity.Inode == after.identity.Inode) {
				t.Fatal("hard death left unexpected target identity")
			}
		})
	}
}
