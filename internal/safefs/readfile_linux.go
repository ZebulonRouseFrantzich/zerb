//go:build linux

package safefs

import (
	"errors"
	"io"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

type fileStamp struct {
	identity          Identity
	modified, changed unix.StatxTimestamp
}

// ReadFile reads one validated regular child through its pinned inode.
func (d *Dir) ReadFile(name string, limit int64) ([]byte, Identity, error) {
	if d == nil || d.state == nil || !validName(name) || !validReadLimit(limit) {
		return nil, Identity{}, errDirectory
	}
	if d.state.gate != nil {
		return d.treeReadFile(name, limit)
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return nil, Identity{}, errDirectory
	}
	data, id, err := readFile(s.chain[len(s.chain)-1], name, limit)
	post := validateChain(s.chain, s.owner)
	if err != nil || post != nil {
		return nil, Identity{}, errDirectory
	}
	return data, id, nil
}

func validReadLimit(limit int64) bool {
	return limit >= 0 && limit < int64(int(^uint(0)>>1))
}

func observeFile(fd int, name string) (fileStamp, error) {
	flags := unix.AT_SYMLINK_NOFOLLOW | unix.AT_NO_AUTOMOUNT
	if name == "" {
		flags |= unix.AT_EMPTY_PATH
	}
	const times = unix.STATX_MTIME | unix.STATX_CTIME
	var st unix.Statx_t
	if unix.Statx(fd, name, flags, identityMask|times, &st) != nil || st.Mask&times != times {
		return fileStamp{}, errDirectory
	}
	id, err := identityFromStatx(st)
	if err != nil {
		return fileStamp{}, errDirectory
	}
	return fileStamp{identity: id, modified: st.Mtime, changed: st.Ctime}, nil
}

func safeFile(id Identity, owner uint32, limit int64) bool {
	return validReadLimit(limit) && id.Mode&unix.S_IFMT == unix.S_IFREG && id.Mode&0o022 == 0 &&
		id.Owner == owner && id.Links == 1 && id.MountID != 0 && id.Size >= 0 && id.Size <= limit
}

type pinnedFile struct {
	fd     int
	parent edge
	name   string
	before fileStamp
	limit  int64
	closed bool
}

func pinFile(parent edge, name string, limit int64) (_ *pinnedFile, err error) {
	if !validName(name) || !validReadLimit(limit) {
		return nil, errDirectory
	}
	id, err := observeFD(parent.fd)
	if err != nil || !safeDirectory(id) || !sameDirectory(id, parent.id) {
		return nil, errDirectory
	}
	fd, err := unix.Openat2(parent.fd, name, &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: procResolve,
	})
	if err != nil {
		return nil, errDirectory
	}
	p := &pinnedFile{fd: fd, parent: parent, name: name, limit: limit}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.close())
		}
	}()
	p.before, err = observeFile(fd, "")
	if err != nil || !safeFile(p.before.identity, parent.id.Owner, limit) || !sameMount(p.before.identity, parent.id) || p.validate() != nil {
		return nil, errDirectory
	}
	return p, nil
}

func (p *pinnedFile) descriptor() error {
	if p == nil || p.closed || !safeFile(p.before.identity, p.parent.id.Owner, p.limit) {
		return errDirectory
	}
	now, err := observeFile(p.fd, "")
	if err != nil || now != p.before {
		return errDirectory
	}
	return nil
}

func (p *pinnedFile) validate() error {
	if p.descriptor() != nil {
		return errDirectory
	}
	id, err := observeFD(p.parent.fd)
	if err != nil || !safeDirectory(id) || !sameDirectory(id, p.parent.id) {
		return errDirectory
	}
	linked, err := observeFile(p.parent.fd, p.name)
	if err != nil || linked != p.before {
		return errDirectory
	}
	return nil
}

func (p *pinnedFile) close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	fd := p.fd
	p.fd = -1
	return closeChain([]edge{{fd: fd}})
}

func (p *procAccess) reopen(pin *pinnedFile) (_ *os.File, err error) {
	if p.validate() != nil || pin.descriptor() != nil {
		return nil, errDirectory
	}
	// Only this generated owned-FD magic link may cross out of procfs.
	fd, err := unix.Openat(p.chain[3].fd, strconv.Itoa(pin.fd), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	id, err := observeFile(fd, "")
	if err != nil || id != pin.before || pin.descriptor() != nil || p.validate() != nil {
		return nil, errDirectory
	}
	file := os.NewFile(uintptr(fd), "")
	if file == nil {
		return nil, errDirectory
	}
	return file, nil
}

func readContents(file *os.File, size int64) ([]byte, error) {
	if file == nil || !validReadLimit(size) {
		return nil, errDirectory
	}
	data, err := io.ReadAll(io.LimitReader(file, size+1))
	if err != nil || int64(len(data)) != size {
		return nil, errDirectory
	}
	return data, nil
}

func finishFileRead(file *os.File, data []byte, id Identity, primary error) ([]byte, Identity, error) {
	var closeErr error
	if file != nil {
		closeErr = file.Close()
	}
	if primary != nil || closeErr != nil {
		return nil, Identity{}, errDirectory
	}
	return data, id, nil
}

func readFile(parent edge, name string, limit int64) (data []byte, id Identity, err error) {
	var pin *pinnedFile
	var proc *procAccess
	var reader *os.File
	defer func() {
		data, id, err = finishFileRead(reader, data, id, err)
		if cleanupErr := errors.Join(proc.close(), pin.close()); cleanupErr != nil {
			data, id, err = nil, Identity{}, errDirectory
		}
	}()
	pin, err = pinFile(parent, name, limit)
	if err != nil {
		return nil, Identity{}, errDirectory
	}
	proc, err = openProcAccess()
	if err != nil {
		return nil, Identity{}, errDirectory
	}
	reader, err = proc.reopen(pin)
	if err != nil || pin.validate() != nil {
		return nil, Identity{}, errDirectory
	}
	data, err = readContents(reader, pin.before.identity.Size)
	if err != nil {
		return nil, Identity{}, errDirectory
	}
	stamp, err := observeFile(int(reader.Fd()), "")
	if err != nil || stamp != pin.before || pin.validate() != nil || proc.validate() != nil {
		return nil, Identity{}, errDirectory
	}
	return data, pin.before.identity, nil
}
