//go:build linux

package safefs

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTreeCrashPrefixesKernelCore(t *testing.T) {
	if boundary := os.Getenv("ZERB_TREE_CRASH_BOUNDARY"); boundary != "" {
		treeCrashChild(t, boundary)
		return
	}
	for _, boundary := range []string{"created", "written", "synced", "renamed"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			outside := filepath.Join(root, "outside")
			if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestTreeCrashPrefixesKernelCore$")
			cmd.Env = append(os.Environ(), "ZERB_TREE_CRASH_BOUNDARY="+boundary, "ZERB_TREE_CRASH_ROOT="+root)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
					t.Error(err)
				}
				if !waited {
					if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
						t.Error(err)
					}
					if err := cmd.Wait(); err != nil {
						var exit *exec.ExitError
						if !errors.As(err, &exit) {
							t.Error(err)
						}
					}
				}
			})
			ready := make(chan string, 1)
			go func() {
				line, err := bufio.NewReader(stdout).ReadString('\n')
				if err != nil {
					line = ""
				}
				ready <- line
			}()
			select {
			case line := <-ready:
				if line != "ready\n" {
					t.Fatal("child did not reach exact boundary")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("crash boundary timed out")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			waited = true
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatal("hard-death child unexpectedly succeeded")
			}
			name := "stage"
			if boundary == "renamed" {
				name = "published"
			}
			info, err := os.Lstat(filepath.Join(root, name))
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Fatal("crash residue absent or repaired")
			}
			if boundary != "created" {
				if data, err := os.ReadFile(filepath.Join(root, name, "payload")); err != nil || !bytes.Equal(data, []byte("payload")) {
					t.Fatal("crash residue changed")
				}
			}
			if data, err := os.ReadFile(outside); err != nil || !bytes.Equal(data, []byte("outside")) {
				t.Fatal("hard death changed outside sentinel")
			}
		})
	}
}

func treeCrashChild(t *testing.T, boundary string) {
	t.Helper()
	fd, err := unix.Open(os.Getenv("ZERB_TREE_CRASH_ROOT"), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	parent := edge{fd: fd, id: id}
	defer func() {
		if err := closeChain([]edge{parent}); err != nil {
			t.Error(err)
		}
	}()
	s, err := createTreeAt([]edge{parent}, uint32(os.Geteuid()), "stage")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.dispose(); err != nil {
			t.Error(err)
		}
	}()
	stop := func() {
		if _, err := os.Stdout.WriteString("ready\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
	}
	if boundary == "created" {
		stop()
		return
	}
	payload, err := unix.Openat(s.root.fd, "payload", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(payload), "")
	if file == nil {
		t.Fatal("cannot own disposable payload descriptor")
	}
	if _, err := file.WriteString("payload"); err != nil {
		t.Fatal(err)
	}
	if boundary != "written" {
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if boundary == "written" {
		stop()
		return
	}
	if syncDirectory(s.root) != nil || syncDirectory(parent) != nil {
		t.Fatal("sync boundary refused")
	}
	if boundary == "synced" {
		stop()
		return
	}
	started, err := s.terminalStart()
	if err != nil || !started {
		t.Fatal("terminal admission refused")
	}
	check := func() error {
		if s.root.name == "published" {
			stop()
			return errDirectory
		}
		return s.validateEdges()
	}
	if _, _, err := s.promoteWithBoundary("published", check); err == nil {
		t.Fatal("renamed boundary unexpectedly completed")
	}
}
