//go:build linux

package safefs

import (
	"errors"
	"sort"

	"golang.org/x/sys/unix"
)

// removeParent opens only snapshot-known directory edges from the live root.
// Its fixed frame set is independent of the absolute ancestor-chain depth.
func (s *stageState) removeParent(e treeEntry, boundary func() error) (_ edge, closeParents func() error, checkParents func() error, err error) {
	var ancestors [treeMaxDepth]uint32
	count := 0
	for parent := e.parent; parent != treeNoParent; parent = s.work[parent].inventory[0].parent {
		if count == len(ancestors) {
			return edge{}, nil, nil, errDirectory
		}
		ancestors[count] = parent
		count++
	}
	var opened [treeMaxDepth]edge
	used := 0
	closeParents = func() error {
		var failure error
		for used > 0 {
			used--
			failure = errors.Join(failure, closeChain(opened[used:used+1]))
		}
		return failure
	}
	checkParents = func() error {
		if boundary() != nil {
			return errDirectory
		}
		prior := s.root
		for i := 0; i < used; i++ {
			current, failure := observeFD(opened[i].fd)
			linked, linkedFailure := observeAt(prior.fd, opened[i].name)
			if failure != nil || linkedFailure != nil || !sameDirectory(current, opened[i].id) || !sameDirectory(current, linked) {
				return errDirectory
			}
			prior = opened[i]
		}
		return nil
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeParents())
		}
	}()
	parent := s.root
	if boundary() != nil {
		return edge{}, closeParents, checkParents, errDirectory
	}
	for i := count - 1; i >= 0; i-- {
		entry := s.work[ancestors[i]].inventory[0]
		child, failure := openTreeObserved(parent, treeLeaf(s.path(0, entry)), entry.stamp.identity)
		if failure != nil {
			return edge{}, closeParents, checkParents, errDirectory
		}
		opened[used] = child
		used++
		parent = child
	}
	return parent, closeParents, checkParents, nil
}

func (s *stageState) removeInventory(inventory treeInventory) error {
	return s.removeWithBoundary(inventory, s.validate, func() error { return validateChain(s.parent, s.owner) })
}

func (s *stageState) removeWithBoundary(inventory treeInventory, boundary, parentBoundary func() error) error {
	order := treeInventoryOrder{s: s, slot: 0, count: inventory.count, postorder: true}
	sort.Sort(order)
	for i := 0; i < inventory.count; i++ {
		e := s.work[s.work[i].order[0]].inventory[0]
		parent, closeParents, checkParents, err := s.removeParent(e, boundary)
		if err != nil {
			return errDirectory
		}
		err = s.removeEntry(parent, e, checkParents)
		cleanup := closeParents()
		if err != nil || cleanup != nil || boundary() != nil {
			return errDirectory
		}
	}
	var scratch [32 << 10]byte
	if scanTreeNames(s.root, scratch[:], func([]byte) error { return errDirectory }) != nil || boundary() != nil {
		return errDirectory
	}
	parent := s.parent[len(s.parent)-1]
	if unix.Unlinkat(parent.fd, s.root.name, unix.AT_REMOVEDIR) != nil {
		return errDirectory
	}
	id, err := observeFD(s.root.fd)
	if err != nil || !sameDirectory(id, s.root.id) || id.Links != 0 || syncDirectory(parent) != nil || parentBoundary() != nil {
		return errDirectory
	}
	if _, err := observeAt(parent.fd, s.root.name); !errors.Is(err, unix.ENOENT) {
		return errDirectory
	}
	return nil
}

func (s *stageState) removeEntry(parent edge, e treeEntry, checkParents func() error) (err error) {
	if checkParents() != nil {
		return errDirectory
	}
	name := treeLeaf(s.path(0, e))
	current, err := observeFile(parent.fd, name)
	if err != nil {
		return errDirectory
	}
	if e.stamp.identity.Mode&unix.S_IFMT == unix.S_IFDIR {
		if !sameDirectory(current.identity, e.stamp.identity) {
			return errDirectory
		}
		child, failure := openTreeObserved(parent, name, e.stamp.identity)
		if failure != nil {
			return errDirectory
		}
		defer func() { err = errors.Join(err, closeChain([]edge{child})) }()
		var scratch [32 << 10]byte
		if scanTreeNames(child, scratch[:], func([]byte) error { return errDirectory }) != nil {
			return errDirectory
		}
		linked, failure := observeAt(parent.fd, name)
		if failure != nil || !sameDirectory(linked, child.id) || checkParents() != nil || unix.Unlinkat(parent.fd, name, unix.AT_REMOVEDIR) != nil {
			return errDirectory
		}
		after, failure := observeFD(child.fd)
		if failure != nil || !sameDirectory(after, child.id) || after.Links != 0 {
			return errDirectory
		}
	} else {
		if current != e.stamp {
			return errDirectory
		}
		_, stamp, digest, failure := treeRead(s, parent, name, treeMaxFile, false, false)
		if failure != nil || stamp != e.stamp || digest != e.digest {
			return errDirectory
		}
		pin, failure := pinFile(parent, name, treeMaxFile)
		if failure != nil {
			return errDirectory
		}
		defer func() { err = errors.Join(err, pin.close()) }()
		if pin.before != e.stamp || pin.validate() != nil || checkParents() != nil || unix.Unlinkat(parent.fd, name, 0) != nil {
			return errDirectory
		}
		after, failure := observeFile(pin.fd, "")
		want := e.stamp.identity
		want.Links = 0
		if failure != nil || after.identity != want || after.modified != e.stamp.modified {
			return errDirectory
		}
	}
	if checkParents() != nil || syncDirectory(parent) != nil || checkParents() != nil {
		return errDirectory
	}
	if _, failure := observeAt(parent.fd, name); !errors.Is(failure, unix.ENOENT) {
		return errDirectory
	}
	return nil
}
