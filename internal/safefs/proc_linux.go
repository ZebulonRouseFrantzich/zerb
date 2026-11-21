//go:build linux

package safefs

import (
	"errors"
	"strconv"

	"golang.org/x/sys/unix"
)

const procResolve = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV

// This operation-owned capability admits only the current process's FD directory.
type procAccess struct {
	chain  []edge
	self   edge
	pid    string
	closed bool
}

func openProcAccess() (*procAccess, error) {
	fd, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errDirectory
	}
	return openProcRoot(fd)
}

// openProcRoot takes ownership of rootfd, including on failure.
func openProcRoot(rootfd int) (_ *procAccess, err error) {
	p := &procAccess{chain: []edge{{fd: rootfd}}, self: edge{fd: -1}, pid: strconv.Itoa(unix.Getpid())}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.close())
		}
	}()
	id, err := observeFD(rootfd)
	if err != nil || !safeDirectory(id) {
		return nil, errDirectory
	}
	p.chain[0].id = id
	// Entering the real proc mount is deliberate; later traversal stays on it.
	proc, err := openProcDir(p.chain[0], "proc", unix.RESOLVE_BENEATH|unix.RESOLVE_NO_SYMLINKS)
	if err != nil {
		return nil, err
	}
	p.chain = append(p.chain, proc)
	fd, err := unix.Openat2(proc.fd, "self", &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: procResolve,
	})
	if err != nil {
		return nil, errDirectory
	}
	p.self = edge{fd: fd, name: "self"}
	p.self.id, err = observeFD(fd)
	if err != nil || p.checkSelf() != nil {
		return nil, errDirectory
	}
	for _, name := range []string{p.pid, "fd"} {
		child, err := openProcDir(p.chain[len(p.chain)-1], name, procResolve)
		if err != nil {
			return nil, err
		}
		p.chain = append(p.chain, child)
	}
	if p.validate() != nil {
		return nil, errDirectory
	}
	return p, nil
}

func procFilesystem(fd int) bool {
	var st unix.Statfs_t
	return unix.Fstatfs(fd, &st) == nil && st.Type == unix.PROC_SUPER_MAGIC
}

func sameMount(a, b Identity) bool {
	return a.MountID == b.MountID && a.Device == b.Device
}

func openProcDir(parent edge, name string, resolve uint64) (_ edge, err error) {
	if !validName(name) {
		return edge{}, errDirectory
	}
	fd, err := unix.Openat2(parent.fd, name, &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: resolve,
	})
	if err != nil {
		return edge{}, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	id, err := observeFD(fd)
	if err != nil || !safeDirectory(id) || !procFilesystem(fd) || resolve&unix.RESOLVE_NO_XDEV != 0 && !sameMount(id, parent.id) {
		return edge{}, errDirectory
	}
	linked, err := observeAt(parent.fd, name)
	if err != nil || !sameDirectory(id, linked) {
		return edge{}, errDirectory
	}
	return edge{fd: fd, name: name, id: id}, nil
}

func (p *procAccess) checkSelf() error {
	if p == nil || p.closed || len(p.chain) < 2 || p.self.fd < 0 || p.pid != strconv.Itoa(unix.Getpid()) {
		return errDirectory
	}
	id, err := observeFD(p.self.fd)
	if err != nil || id != p.self.id || id.Mode&unix.S_IFMT != unix.S_IFLNK || id.Links != 1 ||
		!sameMount(id, p.chain[1].id) || !procFilesystem(p.self.fd) {
		return errDirectory
	}
	linked, err := observeAt(p.chain[1].fd, "self")
	if err != nil || linked != id {
		return errDirectory
	}
	var value [32]byte
	n, err := unix.Readlinkat(p.self.fd, "", value[:])
	if err != nil || n <= 0 || n >= len(value) || string(value[:n]) != p.pid {
		return errDirectory
	}
	return nil
}

func (p *procAccess) validate() (err error) {
	if p == nil || p.closed || len(p.chain) != 4 || p.chain[1].name != "proc" || p.chain[2].name != p.pid || p.chain[3].name != "fd" {
		return errDirectory
	}
	fd, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errDirectory
	}
	defer func() { err = errors.Join(err, closeChain([]edge{{fd: fd}})) }()
	root, err := observeFD(fd)
	if err != nil || !sameDirectory(root, p.chain[0].id) {
		return errDirectory
	}
	for i, entry := range p.chain {
		id, err := observeFD(entry.fd)
		if err != nil || !safeDirectory(id) || !sameDirectory(id, entry.id) {
			return errDirectory
		}
		if i == 0 {
			continue
		}
		linked, err := observeAt(p.chain[i-1].fd, entry.name)
		if err != nil || !sameDirectory(linked, id) || !procFilesystem(entry.fd) || !sameMount(id, p.chain[1].id) {
			return errDirectory
		}
	}
	return p.checkSelf()
}

func (p *procAccess) close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	owned := p.chain
	if p.self.fd >= 0 {
		owned = append(owned, p.self)
	}
	p.chain, p.self = nil, edge{fd: -1}
	return closeChain(owned)
}
