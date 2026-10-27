//go:build linux

package execenv

import (
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var errStreams = errors.New("process streams unavailable")

// ARCH-009/030: caller files remain borrowed; this lifetime owns duplicates.
// Offsets and status flags remain shared with their original open descriptions.
type ownedStreams struct {
	mu    sync.Mutex
	files [3]*os.File
	err   error
}

// All three explicit endpoints are required at this private boundary. A future
// trusted null owner supplies omitted Runner streams before calling it.
func duplicateStreams(files [3]*os.File) (result *ownedStreams, err error) {
	for _, file := range files {
		if file == nil {
			return nil, errStreams
		}
	}
	owned := &ownedStreams{}
	defer func() {
		if err != nil && owned.Close() != nil {
			err = errStreams
		}
	}()
	for index, file := range files {
		raw, failure := file.SyscallConn()
		if failure != nil {
			return nil, errStreams
		}
		fd := -1
		var duplicateErr error
		failure = raw.Control(func(original uintptr) {
			fd, duplicateErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
		})
		if fd >= 0 && duplicateErr == nil {
			owned.files[index] = os.NewFile(uintptr(fd), "process stream")
			if owned.files[index] == nil {
				if unix.Close(fd) != nil {
					return nil, errStreams
				}
				return nil, errStreams
			}
		}
		if failure != nil || duplicateErr != nil {
			return nil, errStreams
		}
	}
	return owned, nil
}

func (owned *ownedStreams) Close() error {
	if owned == nil {
		return nil
	}
	owned.mu.Lock()
	defer owned.mu.Unlock()
	for index, file := range owned.files {
		owned.files[index] = nil
		if file != nil && file.Close() != nil {
			owned.err = errStreams
		}
	}
	return owned.err
}
