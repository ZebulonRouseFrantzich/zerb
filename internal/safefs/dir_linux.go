//go:build linux

// Package safefs owns protected filesystem descriptors (ARCH-020/030).
package safefs

import (
	"errors"
	"math"
	"path"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type Kind uint8

const (
	KindDirectory Kind = iota + 1
	KindRegular
	KindExecutable
)

type Identity struct {
	Device  uint64
	Inode   uint64
	Owner   uint32
	Mode    uint32
	Links   uint64
	Size    int64
	MountID uint64
}

var errDirectory = errors.New("unsafe directory operation")

type edge struct {
	fd   int
	name string
	id   Identity
}

// A shared state keeps ordinary Go value copies from double-closing reused FDs.
type dirState struct {
	mu       sync.Mutex
	chain    []edge
	owner    uint32
	identity Identity
	closed   bool
}

type Dir struct{ state *dirState }

// OpenRoot opens an existing canonical absolute directory without following links.
// UID 0 may own the structural prefix; the final subtree belongs to owner.
func OpenRoot(name string, owner uint32) (_ *Dir, err error) {
	if !path.IsAbs(name) || path.Clean(name) != name || strings.IndexByte(name, 0) >= 0 {
		return nil, errDirectory
	}
	var chain []edge
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain(chain))
		}
	}()
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errDirectory
	}
	chain = append(chain, edge{fd: fd})
	id, e := observeFD(fd)
	if e != nil {
		return nil, e
	}
	chain[0].id = id
	owned, e := advanceOwner(id, owner, false)
	if e != nil {
		return nil, e
	}
	if name != "/" {
		for _, part := range strings.Split(name[1:], "/") {
			if e := validateChain(chain, owner); e != nil {
				return nil, e
			}
			parent := chain[len(chain)-1].fd
			expected, e := observeAt(parent, part)
			if e != nil {
				return nil, errDirectory
			}
			owned, e = advanceOwner(expected, owner, owned)
			if e != nil {
				return nil, e
			}
			child, e := openObserved(parent, part, expected)
			if e != nil {
				return nil, e
			}
			chain = append(chain, child)
			if e := validateChain(chain, owner); e != nil {
				return nil, e
			}
		}
	}
	if e := validateChain(chain, owner); e != nil || chain[len(chain)-1].id.Owner != owner {
		return nil, errDirectory
	}
	return &Dir{state: &dirState{chain: chain, owner: owner, identity: chain[len(chain)-1].id}}, nil
}

// OpenDir opens one child, optionally creating it. Every observed substitution
// fails; mkdir cannot identify an invisible replacement before first observation.
func (d *Dir) OpenDir(name string, create bool) (_ *Dir, err error) {
	if d == nil || d.state == nil || !validName(name) {
		return nil, errDirectory
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return nil, errDirectory
	}
	child, err := childAt(s.chain[len(s.chain)-1].fd, name, s.owner, create)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{child}))
		}
	}()
	candidate := make([]edge, len(s.chain)+1)
	copy(candidate, s.chain)
	candidate[len(s.chain)] = child
	if err := validateChain(candidate, s.owner); err != nil {
		return nil, err
	}
	owned, err := duplicateChain(s.chain)
	if err != nil {
		return nil, err
	}
	owned = append(owned, child)
	return &Dir{state: &dirState{chain: owned, owner: s.owner, identity: child.id}}, nil
}

// Identity is a captured observation, not a fresh authorization or revalidation.
func (d *Dir) Identity() Identity {
	if d == nil || d.state == nil {
		return Identity{}
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Identity{}
	}
	return s.identity
}

func (d *Dir) Close() error {
	if d == nil || d.state == nil {
		return nil
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	chain := s.chain
	s.chain, s.identity = nil, Identity{}
	return closeChain(chain)
}

func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

func observeFD(fd int) (Identity, error) {
	id, err := observeAt(fd, "")
	if err != nil {
		return Identity{}, errDirectory
	}
	return id, nil
}

func observeAt(fd int, name string) (Identity, error) {
	flags := unix.AT_SYMLINK_NOFOLLOW | unix.AT_NO_AUTOMOUNT
	if name == "" {
		flags |= unix.AT_EMPTY_PATH
	}
	const mask = unix.STATX_TYPE | unix.STATX_MODE | unix.STATX_UID | unix.STATX_INO | unix.STATX_NLINK | unix.STATX_SIZE | unix.STATX_MNT_ID
	var st unix.Statx_t
	if err := unix.Statx(fd, name, flags, mask, &st); err != nil {
		if err == unix.ENOENT && name != "" {
			return Identity{}, unix.ENOENT
		}
		return Identity{}, errDirectory
	}
	if st.Mask&mask != mask || st.Mnt_id == 0 || st.Size > math.MaxInt64 {
		return Identity{}, errDirectory
	}
	return Identity{Device: unix.Mkdev(st.Dev_major, st.Dev_minor), Inode: st.Ino, Owner: st.Uid,
		Mode: uint32(st.Mode), Links: uint64(st.Nlink), Size: int64(st.Size), MountID: st.Mnt_id}, nil
}

func safeDirectory(id Identity) bool {
	return id.Mode&unix.S_IFMT == unix.S_IFDIR && id.Mode&0o022 == 0 && id.Links != 0 && id.MountID != 0
}

func advanceOwner(id Identity, owner uint32, owned bool) (bool, error) {
	if !safeDirectory(id) {
		return false, errDirectory
	}
	if id.Owner == owner {
		return true, nil
	}
	if owned || id.Owner != 0 {
		return false, errDirectory
	}
	return false, nil
}

// Directory entry count and size change during legitimate child creation.
func sameDirectory(a, b Identity) bool {
	return a.Device == b.Device && a.Inode == b.Inode && a.MountID == b.MountID && a.Owner == b.Owner && a.Mode == b.Mode
}

func validateEdges(chain []edge, owner uint32) error {
	if len(chain) == 0 {
		return errDirectory
	}
	owned := false
	for i, entry := range chain {
		current, err := observeFD(entry.fd)
		if err != nil || !sameDirectory(current, entry.id) {
			return errDirectory
		}
		owned, err = advanceOwner(current, owner, owned)
		if err != nil {
			return err
		}
		if i != 0 {
			if !validName(entry.name) {
				return errDirectory
			}
			linked, err := observeAt(chain[i-1].fd, entry.name)
			if err != nil || !sameDirectory(linked, current) {
				return errDirectory
			}
		}
	}
	return nil
}

func validateChain(chain []edge, owner uint32) (err error) {
	if len(chain) == 0 {
		return errDirectory
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return errDirectory
	}
	defer func() { err = errors.Join(err, closeChain([]edge{{fd: fd}})) }()
	current, e := observeFD(fd)
	if e != nil || !sameDirectory(current, chain[0].id) {
		return errDirectory
	}
	return validateEdges(chain, owner)
}

func openObserved(parent int, name string, expected Identity) (_ edge, err error) {
	if !validName(name) || !safeDirectory(expected) {
		return edge{}, errDirectory
	}
	fd, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return edge{}, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	id, e := observeFD(fd)
	if e != nil || !sameDirectory(id, expected) {
		return edge{}, errDirectory
	}
	linked, e := observeAt(parent, name)
	if e != nil || !sameDirectory(id, linked) {
		return edge{}, errDirectory
	}
	return edge{fd: fd, name: name, id: id}, nil
}

func childAt(parent int, name string, owner uint32, create bool) (edge, error) {
	if !validName(name) {
		return edge{}, errDirectory
	}
	id, err := observeAt(parent, name)
	if err != nil {
		if !create || !errors.Is(err, unix.ENOENT) {
			return edge{}, errDirectory
		}
		return createChild(parent, name, owner)
	}
	if id.Owner != owner {
		return edge{}, errDirectory
	}
	return openObserved(parent, name, id)
}

func createChild(parent int, name string, owner uint32) (edge, error) {
	if !validName(name) || unix.Mkdirat(parent, name, 0o700) != nil {
		return edge{}, errDirectory
	}
	id, err := observeAt(parent, name)
	// Never chmod: mkdir cannot prove that the first observed inode was created
	// by this operation. Restrictive umasks can therefore prevent admission.
	if err != nil || id.Owner != owner || id.Mode != unix.S_IFDIR|0o700 {
		return edge{}, errDirectory
	}
	return openObserved(parent, name, id)
}

func duplicateChain(chain []edge) (_ []edge, err error) {
	var owned []edge
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain(owned))
		}
	}()
	for _, entry := range chain {
		fd, e := unix.FcntlInt(uintptr(entry.fd), unix.F_DUPFD_CLOEXEC, 0)
		if e != nil {
			return nil, errDirectory
		}
		entry.fd = fd
		owned = append(owned, entry)
	}
	return owned, nil
}

func closeChain(chain []edge) error {
	var err error
	for i := len(chain) - 1; i >= 0; i-- {
		// Linux may already have released a number on close failure; never retry.
		if unix.Close(chain[i].fd) != nil {
			err = errDirectory
		}
	}
	return err
}
