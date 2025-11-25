package transaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// StaleLockThreshold is the maximum age of a lock before it's considered stale.
	StaleLockThreshold = 10 * time.Minute
)

var (
	ErrLockExists = errors.New("transaction lock exists: another operation may be in progress")
	ErrStaleLock  = errors.New("stale lock detected")
)

// Lock represents a transaction lock.
type Lock struct {
	path string
	file *os.File
}

// AcquireLock attempts to acquire an exclusive lock for config operations.
// Uses O_CREATE|O_EXCL for atomic lock creation.
func AcquireLock(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	lockPath := filepath.Join(dir, "config-add.lock")

	// Try to create lock file exclusively
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Lock exists - check if it's stale
			if isStale, _ := isLockStale(lockPath); isStale {
				// Remove stale lock and retry once
				os.Remove(lockPath)
				file, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if err != nil {
					return nil, ErrLockExists
				}
			} else {
				return nil, ErrLockExists
			}
		} else {
			return nil, fmt.Errorf("create lock file: %w", err)
		}
	}

	// Write lock metadata (PID and timestamp)
	lockData := fmt.Sprintf("pid=%d\ntimestamp=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if _, err := file.WriteString(lockData); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("write lock data: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("sync lock file: %w", err)
	}

	return &Lock{
		path: lockPath,
		file: file,
	}, nil
}

// Release releases the lock.
func (l *Lock) Release() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}

	if l.path != "" {
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove lock file: %w", err)
		}
	}

	return nil
}

// isLockStale checks if a lock file is older than the stale lock threshold.
func isLockStale(lockPath string) (bool, error) {
	info, err := os.Stat(lockPath)
	if err != nil {
		return false, err
	}

	age := time.Since(info.ModTime())
	return age > StaleLockThreshold, nil
}
