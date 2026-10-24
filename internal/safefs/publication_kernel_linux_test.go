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

// ARCH-020/030, VAL-027: process settings change only in disposable children.
func TestPublicationKernelSubprocess(t *testing.T) {
	mode := os.Getenv("ZERB_SAFEFS_PUBLICATION_HELPER")
	if mode == "" {
		for _, mode := range []string{"partial-write", "umask"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPublicationKernelSubprocess$")
			cmd.Env = append(os.Environ(), "ZERB_SAFEFS_PUBLICATION_HELPER="+mode)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("isolated publication check failed: %s", output)
			}
		}
		return
	}
	if mode == "death-before" || mode == "death-after" {
		root := os.Getenv("ZERB_SAFEFS_PUBLICATION_ROOT")
		fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := unix.Close(fd); err != nil {
				t.Error(err)
			}
		}()
		id, err := observeFD(fd)
		if err != nil {
			t.Fatal(err)
		}
		parent := edge{fd: fd, id: id}
		p := publicationFixture(t, parent, []byte("complete file across process death"), 2)
		if mode == "death-after" {
			if err := p.link("entry"); err != nil {
				t.Fatal(err)
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
	root, parent := localFixture(t)
	switch mode {
	case "partial-write":
		p := publicationFixture(t, parent, nil, 0)
		var limit unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		limit.Cur = 8
		if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
			t.Fatal(err)
		}
		signal.Ignore(syscall.SIGXFSZ)
		data := []byte("a complete payload must never be linked after a partial write")
		requirePublicationPhaseRefusal(t, p.write(data))
		got := make([]byte, len(data))
		n, err := unix.Pread(p.fd, got, 0)
		if err != nil || n != 8 || !bytes.Equal(got[:n], data[:n]) {
			t.Fatal("kernel did not exercise actual partial write before refusal")
		}
		requirePublicationPhaseRefusal(t, p.syncFile())
		requirePublicationPhaseRefusal(t, p.link("entry"))
		if p.attempted {
			t.Fatal("partial write reached link boundary")
		}
		if err := p.close(); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatal("partial write left named residue")
		}
	case "umask":
		for _, test := range []struct {
			mask int
			mode uint32
			want bool
		}{
			{0o777, 0o600, false}, {0o777, 0, true}, {0o377, 0o400, true},
			{0o077, 0o600, true}, {0, 0o640, true}, {0o022, 0o755, true},
		} {
			unix.Umask(test.mask)
			p, err := openPublication(parent, test.mode)
			if !test.want {
				if p != nil || err == nil || err.Error() != errDirectory.Error() {
					t.Fatal("masked mode was repaired or admitted")
				}
				continue
			}
			if err != nil || p == nil {
				t.Fatal("exact requested mode refused under compatible umask")
			}
			name := "mode-" + strconv.Itoa(test.mask) + "-" + strconv.FormatUint(uint64(test.mode), 8)
			data := []byte("retained descriptor verifies unreadable modes")
			for _, phase := range []func() error{func() error { return p.write(data) }, p.syncFile,
				func() error { return p.link(name) }, p.syncParent, func() error { return p.verify(name, data) }} {
				if err := phase(); err != nil {
					t.Fatal("compatible-umask publication phase failed")
				}
			}
			if p.stamp.identity.Mode != unix.S_IFREG|test.mode {
				t.Fatal("requested mode was changed")
			}
			if err := p.close(); err != nil {
				t.Fatal(err)
			}
		}
	case "exec":
		fd, err := strconv.Atoi(os.Getenv("ZERB_SAFEFS_PUBLICATION_FD"))
		if err != nil {
			t.Fatal(err)
		}
		inode, err := strconv.ParseUint(os.Getenv("ZERB_SAFEFS_PUBLICATION_INODE"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if id, err := observeFD(fd); err == nil && id.Inode == inode {
			t.Fatal("publication descriptor survived exec")
		}
	default:
		t.Fatal("unknown isolated publication check")
	}
}

// This is process-death visibility, not storage-failure or power-loss evidence.
func TestPublicationKernelProcessDeath(t *testing.T) {
	for _, mode := range []string{"death-before", "death-after"} {
		t.Run(mode, func(t *testing.T) {
			root, parent := localFixture(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestPublicationKernelSubprocess$")
			cmd.Env = append(os.Environ(), "ZERB_SAFEFS_PUBLICATION_HELPER="+mode, "ZERB_SAFEFS_PUBLICATION_ROOT="+root)
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
						t.Error("test holder cleanup kill failed")
					}
					select {
					case <-waited:
					case <-time.After(10 * time.Second):
						t.Error("test holder did not reap after kill")
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
					t.Fatal("test holder failed before publication boundary")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("test holder did not reach publication boundary")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-waited:
				reaped = true
				if err == nil {
					t.Fatal("test holder was not killed")
				}
				status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
				if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatal("test holder did not terminate by SIGKILL")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("test holder did not reap after kill")
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "death-before" {
				if len(entries) != 0 {
					t.Fatal("hard death before link left named residue")
				}
				return
			}
			if len(entries) != 1 || entries[0].Name() != "entry" {
				t.Fatal("hard death after link left unexpected named residue")
			}
			data, err := os.ReadFile(filepath.Join(root, "entry"))
			if err != nil || string(data) != "complete file across process death" {
				t.Fatal("hard death after link lost complete final bytes")
			}
			stamp, err := observeFile(parent.fd, "entry")
			if err != nil || stamp.identity.Links != 1 || stamp.identity.Mode != unix.S_IFREG|0o600 || stamp.identity.Size != int64(len(data)) {
				t.Fatal("hard death after link changed published metadata")
			}
		})
	}
}

func TestPublicationCloseOnExec(t *testing.T) {
	root, parent := localFixture(t)
	p := publicationFixture(t, parent, []byte("complete close-on-exec file"), 5)
	for _, fd := range []int{p.fd, p.pin.fd} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestPublicationKernelSubprocess$")
		cmd.Env = append(os.Environ(), "ZERB_SAFEFS_PUBLICATION_HELPER=exec",
			"ZERB_SAFEFS_PUBLICATION_FD="+strconv.Itoa(fd),
			"ZERB_SAFEFS_PUBLICATION_INODE="+strconv.FormatUint(p.stamp.identity.Inode, 10))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("publication exec check failed: %s", output)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "entry")); err != nil {
		t.Fatal("exec check lost completed publication")
	}
}
