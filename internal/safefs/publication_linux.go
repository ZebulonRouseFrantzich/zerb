//go:build linux

package safefs

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

var errPublicationUncertain = errors.New("publication outcome uncertain")

// PublishNoReplace publishes complete, synced bytes without replacing a child.
// An error never proves absence; an attempted link may already be visible.
func (d *Dir) PublishNoReplace(name string, data []byte, mode uint32) (id Identity, err error) {
	if d == nil || d.state == nil || !validName(name) || !validPublicationMode(mode) || !validReadLimit(int64(len(data))) {
		return Identity{}, errDirectory
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if publicationChain(s) != nil {
		return Identity{}, errDirectory
	}
	var p *publication
	defer func() {
		before := publicationChain(s)
		var beforeFile, afterFile error
		if p != nil && p.linked {
			beforeFile = p.validate()
		}
		cleanup := p.close()
		if p != nil && p.linked {
			afterFile = p.validateLinked()
		}
		after := publicationChain(s)
		if err != nil || before != nil || beforeFile != nil || cleanup != nil || afterFile != nil || after != nil {
			id, err = Identity{}, errDirectory
			if p != nil && p.attempted {
				err = errPublicationUncertain
			}
		}
	}()
	p, err = openPublication(s.chain[len(s.chain)-1], mode)
	post := publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	err = p.write(data)
	post = publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	err = p.syncFile()
	post = publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	err = p.link(name)
	post = publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	err = p.syncParent()
	post = publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	err = p.verify(name, data)
	post = publicationChain(s)
	if err != nil || post != nil {
		return Identity{}, errDirectory
	}
	return p.stamp.identity, nil
}

func publicationChain(s *dirState) error {
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return errDirectory
	}
	return nil
}

func validPublicationMode(mode uint32) bool {
	return mode&^uint32(0o777) == 0 && mode&0o022 == 0
}

type publication struct {
	parent edge
	fd     int
	stamp  fileStamp
	pin    *pinnedFile
	name   string

	written      bool
	synced       bool
	attempted    bool
	linked       bool
	parentSynced bool
	closed       bool
}

func publicationParent(parent edge) error {
	id, err := observeFD(parent.fd)
	if err != nil || !safeDirectory(id) || !sameDirectory(id, parent.id) {
		return errDirectory
	}
	return nil
}

func openPublication(parent edge, mode uint32) (_ *publication, err error) {
	if !validPublicationMode(mode) || publicationParent(parent) != nil {
		return nil, errDirectory
	}
	fd, openErr := unix.Openat(parent.fd, ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, mode)
	post := publicationParent(parent)
	if openErr != nil {
		return nil, errDirectory
	}
	p := &publication{parent: parent, fd: fd}
	defer func() {
		if err != nil {
			if p.close() != nil {
				err = errDirectory
			}
		}
	}()
	p.stamp, err = observeFile(fd, "")
	id := p.stamp.identity
	if err != nil || post != nil || id.Mode != unix.S_IFREG|mode || id.Owner != parent.id.Owner ||
		id.Links != 0 || id.Size != 0 || !sameMount(id, parent.id) || p.validate() != nil {
		return nil, errDirectory
	}
	return p, nil
}

func (p *publication) validate() error {
	if p == nil || p.closed || publicationParent(p.parent) != nil {
		return errDirectory
	}
	id := p.stamp.identity
	links := uint64(0)
	if p.linked {
		links = 1
	}
	if id.Mode&unix.S_IFMT != unix.S_IFREG || !validPublicationMode(id.Mode&^uint32(unix.S_IFMT)) ||
		id.Owner != p.parent.id.Owner || id.Links != links || id.Size < 0 || !sameMount(id, p.parent.id) {
		return errDirectory
	}
	now, err := observeFile(p.fd, "")
	if err != nil || now != p.stamp {
		return errDirectory
	}
	if p.pin != nil && (p.pin.before != p.stamp || p.pin.validate() != nil) {
		return errDirectory
	}
	if p.linked && p.validateLinked() != nil {
		return errDirectory
	}
	return nil
}

// The named observation remains available after both owned descriptors close.
func (p *publication) validateLinked() error {
	if p == nil || !p.linked || publicationParent(p.parent) != nil {
		return errDirectory
	}
	linked, err := observeFile(p.parent.fd, p.name)
	if err != nil || linked != p.stamp {
		return errDirectory
	}
	return nil
}

func (p *publication) write(data []byte) error {
	if p.validate() != nil || p.written || p.attempted || p.stamp.identity.Size != 0 || !validReadLimit(int64(len(data))) {
		return errDirectory
	}
	for offset := 0; offset < len(data); {
		if p.validate() != nil {
			return errDirectory
		}
		n, writeErr := unix.Pwrite(p.fd, data[offset:], int64(offset))
		now, observeErr := observeFile(p.fd, "")
		post := publicationParent(p.parent)
		if writeErr != nil || observeErr != nil || post != nil || n <= 0 || n > len(data)-offset {
			return errDirectory
		}
		expected := p.stamp.identity
		expected.Size = int64(offset + n)
		if now.identity != expected {
			return errDirectory
		}
		p.stamp = now
		offset += n
	}
	if p.compare(data) != nil {
		return errDirectory
	}
	p.written = true
	return nil
}

// Positional reads preserve the owned descriptor's offset and bound scratch space.
func (p *publication) compare(data []byte) error {
	if p.validate() != nil || int64(len(data)) != p.stamp.identity.Size {
		return errDirectory
	}
	var buffer [32 << 10]byte
	for offset := 0; ; {
		if p.validate() != nil {
			return errDirectory
		}
		want := len(data) - offset
		if want > len(buffer) {
			want = len(buffer)
		}
		if want == 0 {
			want = 1
		}
		n, readErr := unix.Pread(p.fd, buffer[:want], int64(offset))
		post := p.validate()
		if readErr != nil || post != nil || n < 0 || n > want {
			return errDirectory
		}
		if offset == len(data) {
			if n != 0 {
				return errDirectory
			}
			return nil
		}
		if n == 0 || !bytes.Equal(buffer[:n], data[offset:offset+n]) {
			return errDirectory
		}
		offset += n
	}
}

func (p *publication) syncFile() error {
	if p.validate() != nil || !p.written || p.attempted {
		return errDirectory
	}
	err := unix.Fsync(p.fd)
	post := p.validate()
	if err != nil || post != nil {
		return errDirectory
	}
	p.synced = true
	return nil
}

func (p *publication) link(name string) error {
	if !validName(name) || p.validate() != nil || !p.synced || p.attempted {
		return errDirectory
	}
	p.attempted = true
	err := unix.Linkat(p.fd, "", p.parent.fd, name, unix.AT_EMPTY_PATH)
	if err == nil {
		p.linked = true
		p.name = name
	}
	now, observeErr := observeFile(p.fd, "")
	post := publicationParent(p.parent)
	if err != nil || observeErr != nil || post != nil {
		return errDirectory
	}
	expected := p.stamp.identity
	expected.Links = 1
	// Linking changes ctime and nlink; every other captured field remains exact.
	if now.identity != expected || now.modified != p.stamp.modified {
		return errDirectory
	}
	p.stamp = now
	return p.validate()
}

func (p *publication) syncParent() error {
	if p.validate() != nil || !p.linked {
		return errDirectory
	}
	err := syncDirectory(p.parent)
	post := p.validate()
	if err != nil || post != nil {
		return errDirectory
	}
	p.parentSynced = true
	return nil
}

func (p *publication) verify(name string, data []byte) error {
	if !validName(name) || p.validate() != nil || !p.parentSynced || p.pin != nil || !validReadLimit(int64(len(data))) {
		return errDirectory
	}
	var err error
	p.pin, err = pinFile(p.parent, name, int64(len(data)))
	post := p.validate()
	if err != nil || post != nil || p.pin.before != p.stamp || p.compare(data) != nil {
		return errDirectory
	}
	return p.validate()
}

func (p *publication) close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	pinErr := p.pin.close()
	fd := p.fd
	p.fd = -1
	if closeErr := unix.Close(fd); pinErr != nil || closeErr != nil {
		return errDirectory
	}
	return nil
}
