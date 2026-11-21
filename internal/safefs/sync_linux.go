//go:build linux

package safefs

import "golang.org/x/sys/unix"

// Sync synchronizes this directory, not child file contents or other directories.
func (d *Dir) Sync() error {
	if d == nil || d.state == nil {
		return errDirectory
	}
	if d.state.gate != nil {
		return d.treeSync()
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return errDirectory
	}
	err := syncDirectory(s.chain[len(s.chain)-1])
	post := validateChain(s.chain, s.owner)
	if err != nil || post != nil {
		return errDirectory
	}
	return nil
}

func syncDirectory(expected edge) error {
	if !safeDirectory(expected.id) {
		return errDirectory
	}
	before, err := observeFD(expected.fd)
	if err != nil || !safeDirectory(before) || !sameDirectory(before, expected.id) {
		return errDirectory
	}
	syncErr := unix.Fsync(expected.fd)
	after, err := observeFD(expected.fd)
	if syncErr != nil || err != nil || !safeDirectory(after) || !sameDirectory(after, expected.id) {
		return errDirectory
	}
	return nil
}
