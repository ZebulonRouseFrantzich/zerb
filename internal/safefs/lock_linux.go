//go:build linux

package safefs

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"
)

type LockMode uint8

const (
	LockShared LockMode = iota + 1
	LockExclusive
)

var errLockBusy = fmt.Errorf("lock busy: %w", unix.EAGAIN)

type lockPin struct {
	fd     int
	parent edge
	name   string
	before fileStamp
	closed bool
}

func safeLock(id Identity, owner uint32) bool {
	return id.Mode == unix.S_IFREG|0o600 && id.Owner == owner && id.Links == 1 && id.MountID != 0 && id.Size >= 0
}

func pinLock(parent edge, name string) (_ *lockPin, err error) {
	if !validName(name) {
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
	p := &lockPin{fd: fd, parent: parent, name: name}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.close())
		}
	}()
	p.before, err = observeFile(fd, "")
	if err != nil || p.validate() != nil {
		return nil, errDirectory
	}
	return p, nil
}

func (p *lockPin) descriptor() error {
	if p == nil || p.closed || !safeLock(p.before.identity, p.parent.id.Owner) || !sameMount(p.before.identity, p.parent.id) {
		return errDirectory
	}
	now, err := observeFile(p.fd, "")
	if err != nil || now != p.before {
		return errDirectory
	}
	return nil
}

func (p *lockPin) validate() error {
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

func (p *lockPin) close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	fd := p.fd
	p.fd = -1
	return closeChain([]edge{{fd: fd}})
}

// This separate capability never broadens the read-only file-reopening policy.
func (p *procAccess) reopenLock(pin *lockPin) (_ int, err error) {
	if p.validate() != nil || pin.validate() != nil {
		return -1, errDirectory
	}
	// A same-owner kernel file lease must refuse rather than block reopening.
	fd, err := unix.Openat(p.chain[3].fd, strconv.Itoa(pin.fd), unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return -1, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	id, err := observeFile(fd, "")
	if err != nil || id != pin.before || pin.validate() != nil || p.validate() != nil {
		return -1, errDirectory
	}
	return fd, nil
}

// Retain the exclusive-create descriptor until pin and reopen identity match.
func createLock(parent edge, name string) (_ int, stamp fileStamp, err error) {
	if !validName(name) {
		return -1, fileStamp{}, errDirectory
	}
	id, err := observeFD(parent.fd)
	if err != nil || !safeDirectory(id) || !sameDirectory(id, parent.id) {
		return -1, fileStamp{}, errDirectory
	}
	fd, err := unix.Openat2(parent.fd, name, &unix.OpenHow{
		Flags: unix.O_CREAT | unix.O_EXCL | unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK,
		Mode:  0o600, Resolve: procResolve,
	})
	if err != nil {
		return -1, fileStamp{}, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	stamp, err = observeFile(fd, "")
	if err != nil || !safeLock(stamp.identity, parent.id.Owner) || stamp.identity.Size != 0 || !sameMount(stamp.identity, parent.id) {
		return -1, fileStamp{}, errDirectory
	}
	return fd, stamp, nil
}

type lockState struct {
	mu       sync.Mutex
	chain    []edge
	owner    uint32
	identity Identity
	pin      *lockPin
	proc     *procAccess
	fd       int
	mode     LockMode
	closed   bool
}

// Lock value copies share one lease and one descriptor lifetime.
type Lock struct{ state *lockState }

// OpenLock opens one stable synchronization inode, optionally creating it.
func (d *Dir) OpenLock(name string, create bool) (_ *Lock, err error) {
	if d == nil || d.state == nil || !validName(name) {
		return nil, errDirectory
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return nil, errDirectory
	}
	owned, err := duplicateChain(s.chain)
	if err != nil {
		return nil, errDirectory
	}
	defer func() {
		if closeChain(owned) != nil {
			err = errDirectory
		}
	}()
	state, err := lockAt(owned[len(owned)-1], name, create)
	if err != nil {
		return nil, errDirectory
	}
	state.chain, owned = owned, nil
	if state.validateAbsolute() != nil || state.validate() != nil {
		return nil, state.refuse()
	}
	return &Lock{state: state}, nil
}

// lockAt owns the file/proc descriptors; its caller supplies owned ancestors.
func lockAt(parent edge, name string, create bool) (_ *lockState, err error) {
	s := &lockState{fd: -1, owner: parent.id.Owner}
	created := -1
	defer func() {
		if created >= 0 && unix.Close(created) != nil {
			err = errDirectory
		}
		if err != nil {
			err = errors.Join(errDirectory, s.closeResources())
		}
	}()
	if !validName(name) {
		return nil, errDirectory
	}
	expected, err := observeAt(parent.fd, name)
	var createdStamp fileStamp
	if err != nil {
		if !create || !errors.Is(err, unix.ENOENT) {
			return nil, errDirectory
		}
		created, createdStamp, err = createLock(parent, name)
		if err != nil {
			return nil, errDirectory
		}
		expected = createdStamp.identity
	}
	s.pin, err = pinLock(parent, name)
	if err != nil || s.pin.before.identity != expected || created >= 0 && s.pin.before != createdStamp {
		return nil, errDirectory
	}
	s.proc, err = openProcAccess()
	if err != nil {
		return nil, errDirectory
	}
	s.fd, err = s.proc.reopenLock(s.pin)
	if err != nil || s.validate() != nil {
		return nil, errDirectory
	}
	if created >= 0 {
		now, err := observeFile(created, "")
		if err != nil || now != s.pin.before {
			return nil, errDirectory
		}
	}
	if s.syncFile() != nil {
		return nil, errDirectory
	}
	s.identity = s.pin.before.identity
	return s, nil
}

// Every opener synchronizes the same inode, including a creator's contender.
func (s *lockState) syncFile() error {
	if s.validate() != nil {
		return s.refuse()
	}
	syncErr := unix.Fsync(s.fd)
	post := s.validate()
	if syncErr != nil || post != nil {
		return s.refuse()
	}
	return nil
}

func (s *lockState) validateAbsolute() error {
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return errDirectory
	}
	return nil
}

func (s *lockState) validate() error {
	if s.closed || s.pin.validate() != nil || s.proc.validate() != nil {
		return errDirectory
	}
	id, err := observeFile(s.fd, "")
	if err != nil || id != s.pin.before {
		return errDirectory
	}
	return nil
}

// The private transitions require the caller to hold mu and its ancestor checks.
func (s *lockState) try(mode LockMode) error {
	if mode != LockShared && mode != LockExclusive {
		return errDirectory
	}
	if s.validate() != nil {
		return s.refuse()
	}
	if s.mode != 0 {
		if s.mode != mode {
			return errDirectory
		}
		if s.validate() != nil {
			return s.refuse()
		}
		return nil
	}
	kind := int16(unix.F_RDLCK)
	if mode == LockExclusive {
		kind = unix.F_WRLCK
	}
	err := unix.FcntlFlock(uintptr(s.fd), unix.F_OFD_SETLK, &unix.Flock_t{Type: kind, Whence: int16(unix.SEEK_SET)})
	if err == nil {
		s.mode = mode
	}
	if s.validate() != nil {
		return s.refuse()
	}
	if errors.Is(err, unix.EAGAIN) {
		return errLockBusy
	}
	if err != nil {
		return errDirectory
	}
	return nil
}

// Try acquires non-blockingly. Repeating the held mode only revalidates one lease.
func (l *Lock) Try(mode LockMode) error {
	if l == nil || l.state == nil || mode != LockShared && mode != LockExclusive {
		return errDirectory
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validateAbsolute() != nil {
		return s.refuse()
	}
	err := s.try(mode)
	if s.validateAbsolute() != nil {
		return s.refuse()
	}
	return err
}

func (s *lockState) unlock() error {
	if s.validate() != nil {
		return s.refuse()
	}
	var err error
	if s.mode != 0 {
		err = unix.FcntlFlock(uintptr(s.fd), unix.F_OFD_SETLK, &unix.Flock_t{Type: unix.F_UNLCK, Whence: int16(unix.SEEK_SET)})
		if err == nil {
			s.mode = 0
		}
	}
	if s.validate() != nil || err != nil {
		return s.refuse()
	}
	return nil
}

// Unlock is idempotent on an open, validated handle with no held lease.
func (l *Lock) Unlock() error {
	if l == nil || l.state == nil {
		return errDirectory
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validateAbsolute() != nil {
		return s.refuse()
	}
	err := s.unlock()
	if s.validateAbsolute() != nil {
		return s.refuse()
	}
	return err
}

// Identity is a cached observation, not a fresh authorization.
func (l *Lock) Identity() Identity {
	if l == nil || l.state == nil {
		return Identity{}
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Identity{}
	}
	return s.identity
}

func (s *lockState) refuse() error {
	// Closing the owned OFD releases even a lease whose validation failed.
	if err := s.closeResources(); err != nil {
		return err
	}
	return errDirectory
}

func (s *lockState) closeResources() error {
	if s.closed {
		return nil
	}
	s.closed, s.mode, s.identity = true, 0, Identity{}
	fd, chain := s.fd, s.chain
	s.fd, s.chain = -1, nil
	var fdErr error
	if fd >= 0 {
		fdErr = closeChain([]edge{{fd: fd}})
	}
	if errors.Join(fdErr, s.proc.close(), s.pin.close(), closeChain(chain)) != nil {
		return errDirectory
	}
	return nil
}

// Close always releases resources, including after a validation failure.
func (l *Lock) Close() error {
	if l == nil || l.state == nil {
		return nil
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	validation := errors.Join(s.validateAbsolute(), s.validate())
	if cleanup := s.closeResources(); validation != nil || cleanup != nil {
		return errDirectory
	}
	return nil
}
