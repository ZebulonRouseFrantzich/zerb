//go:build linux

package execenv

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"syscall"
)

var errExecutableFingerprint = errors.New("invalid executable observation")

// ARCH-009: observed bytes and metadata alone confer no execution authority.
type executableFingerprint struct {
	device, inode uint64
	owner, group  uint32
	mode          uint32
	size          int64
	links         uint64
	modified      syscall.Timespec
	changed       syscall.Timespec
	sha256        [sha256.Size]byte
}

func executableMetadata(file *os.File) (executableFingerprint, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
		return executableFingerprint{}, errExecutableFingerprint
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return executableFingerprint{}, errExecutableFingerprint
	}
	return executableFingerprint{
		device: uint64(stat.Dev), inode: stat.Ino,
		owner: stat.Uid, group: stat.Gid, mode: stat.Mode, size: stat.Size,
		// Access time can change because this observer reads the file.
		links: uint64(stat.Nlink), modified: stat.Mtim, changed: stat.Ctim,
	}, nil
}

// The caller retains the file. File methods synchronize with concurrent Close;
// positional reads preserve even an independently shared open-file offset.
func observeExecutable(ctx context.Context, file *os.File) (executableFingerprint, error) {
	if ctx == nil || file == nil || ctx.Err() != nil {
		return executableFingerprint{}, errExecutableFingerprint
	}
	before, err := executableMetadata(file)
	if err != nil {
		return executableFingerprint{}, errExecutableFingerprint
	}
	hash := sha256.New()
	var scratch [32 * 1024]byte
	for offset := int64(0); offset < before.size; {
		if ctx.Err() != nil {
			return executableFingerprint{}, errExecutableFingerprint
		}
		count := int64(len(scratch))
		if remaining := before.size - offset; remaining < count {
			count = remaining
		}
		data := scratch[:int(count)]
		n, err := file.ReadAt(data, offset)
		if err != nil || n != len(data) {
			return executableFingerprint{}, errExecutableFingerprint
		}
		if _, err := hash.Write(data); err != nil {
			return executableFingerprint{}, errExecutableFingerprint
		}
		offset += int64(n)
	}
	if ctx.Err() != nil {
		return executableFingerprint{}, errExecutableFingerprint
	}
	if n, err := file.ReadAt(scratch[:1], before.size); n != 0 || err != io.EOF {
		return executableFingerprint{}, errExecutableFingerprint
	}
	after, err := executableMetadata(file)
	if err != nil || before != after || ctx.Err() != nil {
		return executableFingerprint{}, errExecutableFingerprint
	}
	copy(before.sha256[:], hash.Sum(scratch[:0]))
	return before, nil
}

func sameExecutableFingerprint(a, b executableFingerprint) bool {
	return a.mode&syscall.S_IFMT == syscall.S_IFREG && b.mode&syscall.S_IFMT == syscall.S_IFREG &&
		a.size >= 0 && b.size >= 0 && a == b
}
