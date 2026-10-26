//go:build linux

package safefs

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

var errDirectoryExists = fmt.Errorf("directory creation collision: %w", unix.EEXIST)

// CreateDir creates exactly one new child; an existing entry always refuses.
// It does not synchronize the new directory or its parent.
func (d *Dir) CreateDir(name string) (*Dir, error) {
	if d == nil || d.state == nil || !validName(name) {
		return nil, errDirectory
	}
	if d.state.gate != nil {
		return d.treeOpenDir(name, true, true)
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if publicationChain(s) != nil {
		return nil, errDirectory
	}
	chain, err := duplicateChain(s.chain)
	if err != nil {
		return nil, errDirectory
	}
	child, err := createDirectoryAt(chain, name, s.owner)
	post := publicationChain(s)
	if err != nil {
		if post != nil {
			return nil, errDirectory
		}
		return nil, err
	}
	if post != nil || validateChain(child.chain, child.owner) != nil {
		if closeChain(child.chain) != nil {
			return nil, errDirectory
		}
		return nil, errDirectory
	}
	return &Dir{state: child}, nil
}

// createDirectoryAt owns chain, including on error; callers admit absolute roots.
func createDirectoryAt(chain []edge, name string, owner uint32) (_ *dirState, err error) {
	defer func() {
		if err != nil {
			if closeChain(chain) != nil {
				err = errDirectory
			}
		}
	}()
	if !validName(name) || validateEdges(chain, owner) != nil || chain[len(chain)-1].id.Owner != owner {
		return nil, errDirectory
	}
	parent := chain[len(chain)-1]
	if err := unix.Mkdirat(parent.fd, name, 0o700); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, errDirectoryExists
		}
		return nil, errDirectory
	}
	// mkdir supplies no FD: an invisible replacement before this observation
	// remains outside the observed-boundary guarantee.
	id, err := observeAt(parent.fd, name)
	if err != nil || id.Owner != owner || id.Mode != unix.S_IFDIR|0o700 || !sameMount(id, parent.id) {
		return nil, errDirectory
	}
	child, err := openObserved(parent.fd, name, id)
	if err != nil {
		return nil, errDirectory
	}
	child.mkdir = true
	chain = append(chain, child)
	if validateEdges(chain, owner) != nil {
		return nil, errDirectory
	}
	return &dirState{chain: chain, owner: owner, identity: child.id, mkdir: true}, nil
}
