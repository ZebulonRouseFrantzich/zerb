//go:build linux

package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	mountNamespaceMode = "__zerb-git-proof-mount-namespace"
	tmpfsMagic         = 0x01021994
)

var insideMountNamespace bool

type namespaceResult struct {
	Result Result `json:"result"`
	Error  string `json:"error,omitempty"`
}

func init() {
	if len(os.Args) != 2 || os.Args[1] != mountNamespaceMode {
		return
	}
	insideMountNamespace = true
	var cfg Config
	if err := json.NewDecoder(os.Stdin).Decode(&cfg); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(namespaceResult{Error: "decode mount-namespace proof configuration"})
		os.Exit(1)
	}
	result, err := Run(context.Background(), cfg)
	out := namespaceResult{Result: result}
	if err != nil {
		out.Error = err.Error()
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(out); encodeErr != nil {
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func runInMountNamespace(ctx context.Context, cfg Config) (Result, error) {
	executable, err := os.Executable()
	if err != nil {
		return Result{}, errors.New("resolve proof executable for mount namespace")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return Result{}, errors.New("canonicalize proof executable for mount namespace")
	}
	cfg.DelegatedHostUID = os.Getuid()
	payload, err := json.Marshal(cfg)
	if err != nil {
		return Result{}, errors.New("encode mount-namespace proof configuration")
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, executable, mountNamespaceMode)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		Credential:                 &syscall.Credential{Uid: 0, Gid: 0},
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	runErr := cmd.Run()
	var out namespaceResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		if runErr != nil {
			return Result{}, fmt.Errorf("enter proof user/mount namespace: %w", runErr)
		}
		return Result{}, errors.New("decode mount-namespace proof result")
	}
	if runErr != nil || out.Error != "" {
		if out.Error == "" {
			out.Error = "mount-namespace proof process failed"
		}
		return out.Result, errors.New(out.Error)
	}
	return out.Result, nil
}

func mountStagingTmpfs(path string, size int64, inodeLimit int) error {
	if !insideMountNamespace {
		return errors.New("staging tmpfs requires the proof user/mount namespace")
	}
	if size <= 0 || inodeLimit <= 0 {
		return errors.New("invalid staging tmpfs boundary")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make proof mounts private: %w", err)
	}
	options := "size=" + strconv.FormatInt(size, 10) + ",nr_inodes=" + strconv.Itoa(inodeLimit) + ",mode=0700"
	if err := syscall.Mount("tmpfs", path, "tmpfs", syscall.MS_NODEV|syscall.MS_NOEXEC|syscall.MS_NOSUID, options); err != nil {
		return fmt.Errorf("mount capped staging tmpfs: %w", err)
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil || uint64(stat.Type) != tmpfsMagic {
		_ = syscall.Unmount(path, 0)
		return errors.New("capped staging tmpfs identity unavailable")
	}
	if int64(stat.Blocks)*int64(stat.Bsize) > size {
		_ = syscall.Unmount(path, 0)
		return errors.New("staging tmpfs exceeds requested aggregate boundary")
	}
	info, err := os.Stat(path)
	if err != nil {
		_ = syscall.Unmount(path, 0)
		return errors.New("staging tmpfs root identity unavailable")
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(identity.Uid) != os.Getuid() || !info.IsDir() || info.Mode().Perm() != 0o700 {
		_ = syscall.Unmount(path, 0)
		return errors.New("staging tmpfs root identity mismatch")
	}
	return nil
}

func unmountStagingTmpfs(path string) error {
	if err := syscall.Unmount(path, 0); err != nil {
		return fmt.Errorf("unmount capped staging tmpfs: %w", err)
	}
	return nil
}

func stagingAvailability(path string) (int64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil || uint64(stat.Type) != tmpfsMagic {
		return 0, 0, errors.New("staging tmpfs availability unavailable")
	}
	return int64(stat.Bavail) * int64(stat.Bsize), stat.Ffree, nil
}
