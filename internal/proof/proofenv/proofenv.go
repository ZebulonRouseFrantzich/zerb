// Package proofenv captures operator-owned paths before any proof process
// poisons HOME or XDG. It is disposable Phase 1 harness code.
package proofenv

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

var xdgNames = [...]string{"XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR"}

type PathIdentity struct {
	Class  string `json:"class"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type Protected struct {
	Paths      []string
	Identities []PathIdentity
}

// CaptureProtected must run before synthetic HOME/XDG values are installed.
// The passwd-database home is authoritative; a conflicting HOME is rejected.
func CaptureProtected(repository string) (Protected, error) {
	account, err := user.Current()
	if err != nil || account.HomeDir == "" {
		return Protected{}, errors.New("resolve account home outside poisoned environment")
	}
	home, err := canonicalExisting(account.HomeDir)
	if err != nil {
		return Protected{}, fmt.Errorf("canonicalize account home: %w", err)
	}
	if ambient := os.Getenv("HOME"); ambient != "" {
		canon, err := canonicalExisting(ambient)
		if err != nil || canon != home {
			return Protected{}, errors.New("HOME disagrees with account database before proof isolation")
		}
	}

	classes := []struct{ class, path string }{{"account-home", home}}
	if repository != "" {
		classes = append(classes, struct{ class, path string }{"repository", repository})
	}
	for _, name := range xdgNames {
		if value := os.Getenv(name); value != "" {
			classes = append(classes, struct{ class, path string }{strings.ToLower(name), value})
		}
	}
	for _, entry := range []struct{ class, path string }{
		{"ssh", filepath.Join(home, ".ssh")},
		{"git-config", filepath.Join(home, ".gitconfig")},
		{"mise", filepath.Join(home, ".local", "share", "mise")},
		{"chezmoi", filepath.Join(home, ".local", "share", "chezmoi")},
	} {
		if _, err := os.Lstat(entry.path); err == nil {
			classes = append(classes, entry)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Protected{}, fmt.Errorf("inspect protected %s path: %w", entry.class, err)
		}
	}

	seen := make(map[string]struct{}, len(classes))
	out := Protected{Paths: make([]string, 0, len(classes)), Identities: make([]PathIdentity, 0, len(classes))}
	for _, entry := range classes {
		canon, err := canonicalExisting(entry.path)
		if err != nil {
			return Protected{}, fmt.Errorf("canonicalize protected %s path: %w", entry.class, err)
		}
		if _, ok := seen[canon]; ok {
			continue
		}
		seen[canon] = struct{}{}
		var stat syscall.Stat_t
		if err := syscall.Lstat(canon, &stat); err != nil {
			return Protected{}, fmt.Errorf("identify protected %s path: %w", entry.class, err)
		}
		out.Paths = append(out.Paths, canon)
		out.Identities = append(out.Identities, PathIdentity{Class: entry.class, Device: uint64(stat.Dev), Inode: stat.Ino})
	}
	sort.Strings(out.Paths)
	sort.Slice(out.Identities, func(i, j int) bool {
		if out.Identities[i].Class != out.Identities[j].Class {
			return out.Identities[i].Class < out.Identities[j].Class
		}
		if out.Identities[i].Device != out.Identities[j].Device {
			return out.Identities[i].Device < out.Identities[j].Device
		}
		return out.Identities[i].Inode < out.Identities[j].Inode
	})
	return out, nil
}

func canonicalExisting(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
