//go:build linux

package safefs

import (
	"errors"
	"io"
	"os"
	"sort"

	"golang.org/x/sys/unix"
)

// Names returns sorted raw entry names without following or opening children.
// Observed changes fail; this does not promise an atomic directory snapshot.
func (d *Dir) Names(limit int) ([]string, error) {
	if d == nil || d.state == nil || !validListingLimit(limit) {
		return nil, errDirectory
	}
	s := d.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || validateChain(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return nil, errDirectory
	}
	names, err := listNames(s.chain[len(s.chain)-1], limit)
	post := validateChain(s.chain, s.owner)
	if err != nil || post != nil {
		return nil, errDirectory
	}
	return names, nil
}

func validListingLimit(limit int) bool {
	return limit >= 0 && limit < int(^uint(0)>>1)
}

type listingStamp struct {
	identity          Identity
	modified, changed unix.StatxTimestamp
}

func observeListing(fd int) (listingStamp, error) {
	const times = unix.STATX_MTIME | unix.STATX_CTIME
	var st unix.Statx_t
	if unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW|unix.AT_NO_AUTOMOUNT,
		identityMask|times, &st) != nil || st.Mask&times != times {
		return listingStamp{}, errDirectory
	}
	id, err := identityFromStatx(st)
	if err != nil || !safeDirectory(id) {
		return listingStamp{}, errDirectory
	}
	return listingStamp{identity: id, modified: st.Mtime, changed: st.Ctime}, nil
}

func openListing(parent int, before listingStamp) (*os.File, error) {
	// dup would share the retained descriptor's directory cursor.
	fd, err := unix.Openat(parent, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errDirectory
	}
	current, err := observeListing(fd)
	if err != nil || current != before {
		return nil, errors.Join(errDirectory, closeChain([]edge{{fd: fd}}))
	}
	cursor := os.NewFile(uintptr(fd), "")
	if cursor == nil {
		return nil, errors.Join(errDirectory, closeChain([]edge{{fd: fd}}))
	}
	return cursor, nil
}

func checkListing(fd int, before listingStamp) error {
	after, err := observeListing(fd)
	if err != nil || after != before {
		return errDirectory
	}
	return nil
}

func listNames(parent edge, limit int) (names []string, err error) {
	if !validListingLimit(limit) {
		return nil, errDirectory
	}
	before, err := observeListing(parent.fd)
	if err != nil || !sameDirectory(before.identity, parent.id) {
		return nil, errDirectory
	}
	cursor, err := openListing(parent.fd, before)
	if err != nil {
		return nil, err
	}
	defer func() { names, err = finishListing(cursor, names, err) }()
	names, readErr := cursor.Readdirnames(limit + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errDirectory
	}
	if err := checkListing(int(cursor.Fd()), before); err != nil {
		return nil, err
	}
	return checkedNames(names, limit)
}

func checkedNames(names []string, limit int) ([]string, error) {
	if !validListingLimit(limit) || len(names) > limit {
		return nil, errDirectory
	}
	sort.Strings(names)
	for i, name := range names {
		if !validName(name) || i > 0 && names[i-1] == name {
			return nil, errDirectory
		}
	}
	return names, nil
}

func finishListing(cursor *os.File, names []string, primary error) ([]string, error) {
	closeErr := cursor.Close()
	if primary != nil || closeErr != nil {
		return nil, errDirectory
	}
	return names, nil
}
