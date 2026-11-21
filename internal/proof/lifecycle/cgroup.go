//go:build linux

package lifecycle

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	cgroup2Magic       = 0x63677270
	proofMemoryMax     = 768 << 20
	proofProcessMax    = 96
	proofCPUQuotaUsec  = 100000
	proofCPUPeriodUsec = 100000
)

// Cgroup is a disposable Phase 1 resource boundary. It is not a product API.
type Cgroup struct {
	path string
}

type CgroupObservation struct {
	MemoryPeak  uint64
	ProcessPeak uint64
	CPUUsec     uint64
	MemoryOOM   uint64
	OOMKill     uint64
	Empty       bool
}

func NewCgroup() (*Cgroup, error) {
	return NewCgroupForDelegatedUID(os.Getuid())
}

// NewCgroupForDelegatedUID addresses a host-user cgroup from a user namespace
// where that host identity is mapped to the caller's current namespace UID.
func NewCgroupForDelegatedUID(delegatedUID int) (*Cgroup, error) {
	base, err := delegatedCgroupBase(delegatedUID)
	if err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, fmt.Errorf("cgroup random name: %w", err)
	}
	path := filepath.Join(base, fmt.Sprintf("zerb-proof-%x", random[:]))
	if err = os.Mkdir(path, 0o755); err != nil {
		return nil, fmt.Errorf("create proof cgroup: %w", err)
	}
	c := &Cgroup{path: path}
	if err = c.write("memory.max", strconv.FormatUint(proofMemoryMax, 10)); err == nil {
		err = c.write("memory.swap.max", "0")
	}
	if err == nil {
		err = c.write("pids.max", strconv.FormatUint(proofProcessMax, 10))
	}
	if err == nil {
		err = c.write("cpu.max", fmt.Sprintf("%d %d", proofCPUQuotaUsec, proofCPUPeriodUsec))
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return c, nil
}

func (c *Cgroup) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

func (c *Cgroup) Observe() (CgroupObservation, error) {
	if c == nil || c.path == "" {
		return CgroupObservation{}, errors.New("cgroup unavailable")
	}
	memory, err := c.readUint("memory.peak")
	if err != nil {
		return CgroupObservation{}, err
	}
	processes, err := c.readUint("pids.peak")
	if err != nil {
		return CgroupObservation{}, err
	}
	current, err := c.readUint("pids.current")
	if err != nil {
		return CgroupObservation{}, err
	}
	events, err := os.ReadFile(filepath.Join(c.path, "memory.events"))
	if err != nil {
		return CgroupObservation{}, fmt.Errorf("read cgroup memory events: %w", err)
	}
	var memoryOOM, oomKill uint64
	for _, line := range strings.Split(string(events), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil {
			return CgroupObservation{}, errors.New("malformed cgroup memory event")
		}
		switch fields[0] {
		case "oom":
			memoryOOM = value
		case "oom_kill":
			oomKill = value
		}
	}
	cpuBytes, err := os.ReadFile(filepath.Join(c.path, "cpu.stat"))
	if err != nil {
		return CgroupObservation{}, fmt.Errorf("read cgroup CPU observation: %w", err)
	}
	var cpu uint64
	for _, line := range strings.Split(string(cpuBytes), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			cpu, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return CgroupObservation{}, errors.New("malformed cgroup CPU observation")
			}
		}
	}
	return CgroupObservation{MemoryPeak: memory, ProcessPeak: processes, CPUUsec: cpu, MemoryOOM: memoryOOM, OOMKill: oomKill, Empty: current == 0}, nil
}

func (c *Cgroup) Close() error {
	if c == nil || c.path == "" {
		return nil
	}
	obs, observeErr := c.Observe()
	if observeErr == nil && !obs.Empty {
		_ = c.write("cgroup.kill", "1")
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if current, err := c.readUint("pids.current"); err == nil && current == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		observeErr = errors.New("managed cgroup was not empty after supervisor completion")
	}
	removeErr := os.Remove(c.path)
	c.path = ""
	if observeErr != nil {
		return observeErr
	}
	if removeErr != nil {
		return fmt.Errorf("remove proof cgroup: %w", removeErr)
	}
	return nil
}

func (c *Cgroup) write(name, value string) error {
	if err := os.WriteFile(filepath.Join(c.path, name), []byte(value), 0o600); err != nil {
		return fmt.Errorf("configure proof cgroup %s: %w", name, err)
	}
	return nil
}

func (c *Cgroup) readUint(name string) (uint64, error) {
	value, err := os.ReadFile(filepath.Join(c.path, name))
	if err != nil {
		return 0, fmt.Errorf("read proof cgroup %s: %w", name, err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse proof cgroup %s: %w", name, err)
	}
	return n, nil
}

func delegatedCgroupBase(delegatedUID int) (string, error) {
	if delegatedUID < 0 {
		return "", errors.New("delegated cgroup UID invalid")
	}
	rel := fmt.Sprintf("/user.slice/user-%d.slice/user@%d.service", delegatedUID, delegatedUID)
	base := filepath.Join("/sys/fs/cgroup", rel)
	var fs syscall.Statfs_t
	if err := syscall.Statfs(base, &fs); err != nil || uint64(fs.Type) != cgroup2Magic {
		return "", errors.New("delegated cgroup v2 base unavailable")
	}
	info, err := os.Stat(base)
	if err != nil {
		return "", errors.New("delegated cgroup base unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() || !info.IsDir() {
		return "", errors.New("delegated cgroup base ownership mismatch")
	}
	controllers, err := os.ReadFile(filepath.Join(base, "cgroup.controllers"))
	if err != nil {
		return "", errors.New("delegated cgroup controllers unavailable")
	}
	enabled, err := os.ReadFile(filepath.Join(base, "cgroup.subtree_control"))
	if err != nil {
		return "", errors.New("delegated cgroup controls unavailable")
	}
	for _, required := range []string{"cpu", "memory", "pids"} {
		if !hasWord(string(controllers), required) || !hasWord(string(enabled), required) {
			return "", fmt.Errorf("required delegated cgroup controller %s unavailable", required)
		}
	}
	return base, nil
}

func hasWord(value, expected string) bool {
	for _, word := range strings.Fields(value) {
		if strings.TrimPrefix(word, "+") == expected {
			return true
		}
	}
	return false
}
