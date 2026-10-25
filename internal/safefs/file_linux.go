//go:build linux

package safefs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var errRemovalUncertain = errors.New("file removal outcome uncertain")

// File retains exact regular-record facts; it supplies no semantic authority.
// Ordinary value copies share one serialized lifetime.
type File struct{ state *fileState }

type fileState struct {
	mu        sync.Mutex
	chain     []edge
	owner     uint32
	pin       *pinnedFile
	proc      *procAccess
	reader    *os.File
	facts     FileFacts
	unlinked  *fileStamp
	attempted bool
	closed    bool
}

// PinFile independently retains an exact 0600 regular child and returns its bytes.
func (d *Dir) PinFile(name string, limit int64) (*File, []byte, error) {
	if d == nil || d.state == nil || !validName(name) || !validReadLimit(limit) {
		return nil, nil, errDirectory
	}
	ds := d.state
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if publicationChain(ds) != nil {
		return nil, nil, errDirectory
	}
	chain, err := duplicateChain(ds.chain)
	if err != nil {
		return nil, nil, errDirectory
	}
	s, data, err := fileAt(chain, name, limit, ds.owner)
	post := publicationChain(ds)
	if err != nil {
		return nil, nil, errDirectory
	}
	if post != nil || validateChain(s.chain, s.owner) != nil {
		if s.closeResources() != nil {
			return nil, nil, errDirectory
		}
		return nil, nil, errDirectory
	}
	return &File{state: s}, data, nil
}

// fileAt owns chain, including on error; callers admit absolute roots.
func fileAt(chain []edge, name string, limit int64, owner uint32) (_ *fileState, _ []byte, err error) {
	s := &fileState{chain: chain, owner: owner}
	defer func() {
		if err != nil && s.closeResources() != nil {
			err = errDirectory
		}
	}()
	if !validName(name) || !validReadLimit(limit) || s.metadata() != nil {
		return nil, nil, errDirectory
	}
	parent := chain[len(chain)-1]
	s.pin, err = pinFile(parent, name, limit)
	if err != nil || s.pin.before.identity.Mode != unix.S_IFREG|0o600 {
		return nil, nil, errDirectory
	}
	// Exact restrictive mode is required before any readable reopen/content I/O.
	s.proc, err = openProcAccess()
	if err != nil {
		return nil, nil, errDirectory
	}
	s.reader, err = s.proc.reopen(s.pin)
	if err != nil || s.metadata() != nil {
		return nil, nil, errDirectory
	}
	data, hash, err := s.read(true)
	if err != nil {
		return nil, nil, errDirectory
	}
	s.facts = replacementFacts(s.pin.before, hash)
	if s.revalidate() != nil {
		return nil, nil, errDirectory
	}
	return s, data, nil
}

// Facts is a copied construction observation, not a fresh validation.
func (f *File) Facts() FileFacts {
	if f == nil || f.state == nil {
		return FileFacts{}
	}
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return FileFacts{}
	}
	return s.facts
}

// Revalidate closes and invalidates the capability on any observed failure.
func (f *File) Revalidate() error {
	if f == nil || f.state == nil {
		return errDirectory
	}
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectory
	}
	if validateChain(s.chain, s.owner) != nil || s.revalidate() != nil || validateChain(s.chain, s.owner) != nil {
		if s.closeResources() != nil {
			return errDirectory
		}
		return errDirectory
	}
	return nil
}

// Remove attempts one checked unlink and is terminal even on prevalidation error.
// Ordinary unlink is not inode CAS; a raced replacement may be displaced.
func (f *File) Remove() (err error) {
	if f == nil || f.state == nil {
		return errDirectory
	}
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectory
	}
	defer func() { err = s.finishRemoval(err) }()
	if validateChain(s.chain, s.owner) != nil {
		return errDirectory
	}
	if s.remove() != nil {
		return errDirectory
	}
	if validateChain(s.chain, s.owner) != nil {
		return errDirectory
	}
	return nil
}

// Close releases descriptors only; it never removes a name.
func (f *File) Close() error {
	if f == nil || f.state == nil {
		return nil
	}
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeResources()
}

func (s *fileState) expected() fileStamp {
	if s.unlinked != nil {
		return *s.unlinked
	}
	return s.pin.before
}

func (s *fileState) metadata() error {
	if s.closed || validateEdges(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return errDirectory
	}
	if s.proc != nil && s.proc.validate() != nil {
		return errDirectory
	}
	if s.pin != nil {
		if s.pin.closed || s.pin.before.identity.Mode != unix.S_IFREG|0o600 {
			return errDirectory
		}
		if s.unlinked == nil {
			if s.pin.validate() != nil {
				return errDirectory
			}
		} else {
			// Zero links are admitted only after this capability's successful unlink.
			if !s.attempted || s.unlinked.identity.Links != 0 {
				return errDirectory
			}
			stamp, err := observeFile(s.pin.fd, "")
			if err != nil || stamp != *s.unlinked {
				return errDirectory
			}
		}
		if s.reader != nil {
			stamp, err := observeFile(int(s.reader.Fd()), "")
			if err != nil || stamp != s.expected() {
				return errDirectory
			}
		}
	}
	return nil
}

// Positional reads preserve the retained descriptor's offset and bound scratch.
func (s *fileState) read(collect bool) ([]byte, string, error) {
	if s.reader == nil || s.metadata() != nil {
		return nil, "", errDirectory
	}
	stamp := s.expected()
	size := stamp.identity.Size
	if !validReadLimit(size) {
		return nil, "", errDirectory
	}
	var data []byte
	if collect {
		data = []byte{}
	}
	h := sha256.New()
	var buffer [32 << 10]byte
	for offset := int64(0); ; {
		if s.metadata() != nil {
			return nil, "", errDirectory
		}
		want := size - offset
		if want > int64(len(buffer)) {
			want = int64(len(buffer))
		}
		if want == 0 {
			want = 1
		}
		n, err := unix.Pread(int(s.reader.Fd()), buffer[:int(want)], offset)
		if err != nil || n < 0 || n > int(want) || s.metadata() != nil {
			return nil, "", errDirectory
		}
		if offset == size {
			if n != 0 {
				return nil, "", errDirectory
			}
			return data, hex.EncodeToString(h.Sum(nil)), nil
		}
		if n == 0 {
			return nil, "", errDirectory
		}
		if _, err := h.Write(buffer[:n]); err != nil {
			return nil, "", errDirectory
		}
		if collect {
			data = append(data, buffer[:n]...)
		}
		offset += int64(n)
	}
}

func (s *fileState) revalidate() error {
	_, hash, err := s.read(false)
	if err != nil || hash != s.facts.SHA256 {
		return errDirectory
	}
	return nil
}

func (s *fileState) remove() error {
	if s.attempted || s.revalidate() != nil {
		return errDirectory
	}
	parent := s.chain[len(s.chain)-1]
	s.attempted = true
	if unix.Unlinkat(parent.fd, s.pin.name, 0) != nil {
		return errDirectory
	}
	stamp, err := observeFile(s.pin.fd, "")
	want := s.pin.before.identity
	want.Links = 0
	if err != nil || stamp.identity != want || stamp.modified != s.pin.before.modified {
		return errDirectory
	}
	s.unlinked = &stamp
	if s.revalidate() != nil || syncDirectory(parent) != nil || s.revalidate() != nil {
		return errDirectory
	}
	if _, err := observeAt(parent.fd, s.pin.name); !errors.Is(err, unix.ENOENT) {
		return errDirectory
	}
	return s.metadata()
}

func (s *fileState) finishRemoval(primary error) error {
	cleanup := s.closeResources()
	if primary != nil || cleanup != nil {
		if s.attempted {
			return errRemovalUncertain
		}
		return errDirectory
	}
	return nil
}

func (s *fileState) closeResources() error {
	if s.closed {
		return nil
	}
	s.closed = true
	reader, pin, proc, chain := s.reader, s.pin, s.proc, s.chain
	s.reader, s.pin, s.proc, s.chain = nil, nil, nil, nil
	s.facts, s.unlinked = FileFacts{}, nil
	var readerErr error
	if reader != nil {
		readerErr = reader.Close()
	}
	if errors.Join(readerErr, pin.close(), proc.close(), closeChain(chain)) != nil {
		return errDirectory
	}
	return nil
}
