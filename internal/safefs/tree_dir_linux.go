//go:build linux

package safefs

import (
	"encoding/binary"
	"errors"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

func (d *Dir) treeOpenDir(name string, create, exclusive bool) (result *Dir, err error) {
	ds := d.state
	s := ds.gate
	// A chain can only shrink on Close; read its length under the view lock.
	ds.mu.Lock()
	length := len(ds.chain)
	closed := ds.closed
	ds.mu.Unlock()
	if closed || len(name) > 255 {
		return nil, errDirectory
	}
	op, err := s.begin(uint64(length+2), 0, uint64(length+1)*80+65536)
	if err != nil {
		return nil, errDirectory
	}
	failed := false
	defer func() {
		if !op.finish(failed) {
			if result != nil {
				err = errors.Join(err, result.Close())
			}
			result = nil
			err = errDirectory
		}
	}()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return nil, errDirectory
	}
	pathBytes, ok := stageRelative(ds, name)
	if !ok {
		return nil, errDirectory
	}
	failed = true
	if publicationChain(ds) != nil {
		return nil, errDirectory
	}
	parent := ds.chain[len(ds.chain)-1]
	id, observed := observeAt(parent.fd, name)
	created := false
	if observed != nil && !errors.Is(observed, unix.ENOENT) {
		return nil, errDirectory
	}
	if observed == nil && exclusive {
		return nil, errDirectoryExists
	}
	if observed != nil {
		if !create || !s.addContent(pathBytes, 0) {
			return nil, errDirectory
		}
		if !s.live() || unix.Mkdirat(parent.fd, name, 0o700) != nil {
			return nil, errDirectory
		}
		created = true
		id, observed = observeAt(parent.fd, name)
		if observed != nil || id.Mode != unix.S_IFDIR|0o700 {
			return nil, errDirectory
		}
	}
	if id.Owner != ds.owner || id.Mode&0o7022 != 0 || !sameMount(id, parent.id) {
		return nil, errDirectory
	}
	child, err := openTreeObserved(parent, name, id)
	if err != nil {
		return nil, errDirectory
	}
	child.mkdir = created
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{child}))
		}
	}()
	owned, err := duplicateTreeChain(ds.chain, 1)
	if err != nil {
		return nil, errDirectory
	}
	child.name = strings.Clone(name)
	owned = append(owned, child)
	if validateChain(owned, ds.owner) != nil || publicationChain(ds) != nil {
		// The child defer owns its descriptor separately until successful return.
		return nil, errors.Join(errDirectory, closeChain(owned[:len(owned)-1]))
	}
	ticket := op.retain(uint64(len(owned)), 0)
	failed = false
	return &Dir{state: &dirState{chain: owned, owner: ds.owner, identity: id, mkdir: created, gate: s, ticket: ticket}}, nil
}

// scanTreeNames owns no children and uses one fixed getdents buffer. The visitor
// must copy any bytes it retains before the next read.
func scanTreeNames(parent edge, scratch []byte, visit func([]byte) error) (err error) {
	before, err := observeListing(parent.fd)
	if err != nil || !sameDirectory(before.identity, parent.id) {
		return errDirectory
	}
	fd, err := unix.Openat(parent.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errDirectory
	}
	defer func() { err = errors.Join(err, closeChain([]edge{{fd: fd}})) }()
	if checkListing(fd, before) != nil {
		return errDirectory
	}
	for {
		n, failure := unix.Getdents(fd, scratch)
		if failure != nil || n < 0 || n > len(scratch) {
			return errDirectory
		}
		if n == 0 {
			break
		}
		for pos := 0; pos < n; {
			if n-pos < 20 {
				return errDirectory
			}
			record := scratch[pos:n]
			length := int(binary.LittleEndian.Uint16(record[16:18]))
			if length < 20 || length > len(record) {
				return errDirectory
			}
			end := 19
			for end < length && record[end] != 0 {
				end++
			}
			if end == length {
				return errDirectory
			}
			name := record[19:end]
			if len(name) == 0 {
				return errDirectory
			}
			for _, value := range name {
				if value == '/' {
					return errDirectory
				}
			}
			dot := len(name) == 1 && name[0] == '.' || len(name) == 2 && name[0] == '.' && name[1] == '.'
			if !dot {
				if len(name) > 255 {
					return errDirectory
				}
				if err := visit(name); err != nil {
					return err
				}
			}
			pos += length
		}
		if checkListing(fd, before) != nil {
			return errDirectory
		}
	}
	return checkListing(fd, before)
}

func (d *Dir) treeNames(limit int) (result []string, err error) {
	ds := d.state
	s := ds.gate
	countLimit := min(limit, treeMaxEntries)
	op, err := s.begin(2, 32<<10, uint64(countLimit)*271+65536)
	if err != nil {
		return nil, errDirectory
	}
	failed := false
	defer func() {
		if !op.finish(failed) {
			result = nil
			err = errDirectory
		}
	}()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return nil, errDirectory
	}
	failed = true
	if publicationChain(ds) != nil {
		return nil, errDirectory
	}
	var scratch [32 << 10]byte
	count, used := 0, 0
	arena := s.arenas[op.slot]
	err = scanTreeNames(ds.chain[len(ds.chain)-1], scratch[:], func(name []byte) error {
		if _, ok := stageRelative(ds, string(name)); !ok {
			return errDirectory
		}
		if count == countLimit || len(name) > len(arena)-used {
			return errDirectory
		}
		entry := &s.work[count].inventory[op.slot]
		entry.pathOffset, entry.pathLength = uint32(used), uint16(len(name))
		copy(arena[used:], name)
		used += len(name)
		count++
		return nil
	})
	if err != nil || publicationChain(ds) != nil {
		return nil, errDirectory
	}
	names := make([]string, count)
	for i := range names {
		entry := s.work[i].inventory[op.slot]
		names[i] = string(arena[int(entry.pathOffset) : int(entry.pathOffset)+int(entry.pathLength)])
		if !validName(names[i]) {
			return nil, errDirectory
		}
	}
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			return nil, errDirectory
		}
	}
	failed = false
	return names, nil
}

func (d *Dir) treeSync() (err error) {
	ds := d.state
	op, err := ds.gate.begin(1, 0, 0)
	if err != nil {
		return errDirectory
	}
	failed := false
	defer func() {
		if !op.finish(failed) {
			err = errDirectory
		}
	}()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return errDirectory
	}
	failed = true
	if publicationChain(ds) != nil || syncDirectory(ds.chain[len(ds.chain)-1]) != nil || publicationChain(ds) != nil {
		return errDirectory
	}
	failed = false
	return nil
}
