//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"math"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const treeNoParent = uint32(math.MaxUint32)

type treeInventory struct {
	count          int
	paths, payload uint64
}
type treeWalkFrame struct {
	dir               edge
	listing           listingStamp
	parent, next, end uint32
	depth             uint8
	owned             bool
}

func (s *stageState) path(slot int, e treeEntry) []byte {
	return s.arenas[slot][int(e.pathOffset) : int(e.pathOffset)+int(e.pathLength)]
}

func treeLeaf(path []byte) string {
	if i := bytes.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	return string(path)
}

func safeTreeEntry(stamp fileStamp, root Identity, owner uint32) bool {
	id := stamp.identity
	if id.Owner != owner || !sameMount(id, root) || id.Mode&0o7022 != 0 {
		return false
	}
	switch id.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		return safeDirectory(id)
	case unix.S_IFREG:
		return safeFile(id, owner, treeMaxFile)
	default:
		return false
	}
}

func (s *stageState) walk(slot int, synchronize bool) (treeInventory, error) {
	return s.walkWithBoundary(slot, synchronize, s.validate)
}

// The caller owns absolute admission; the callback revalidates it at every
// affected boundary. Local kernel tests exercise only this descriptor core.
func (s *stageState) walkWithBoundary(slot int, synchronize bool, boundary func() error) (result treeInventory, err error) {
	var frames [treeMaxDepth + 1]treeWalkFrame
	depth := 0
	frames[0] = treeWalkFrame{dir: s.root, parent: treeNoParent}
	defer func() {
		for i := depth; i > 0; i-- {
			if frames[i].owned {
				frames[i].owned = false
				err = errors.Join(err, closeChain([]edge{frames[i].dir}))
			}
		}
	}()
	var scratch [32 << 10]byte
	enumerate := func(frame *treeWalkFrame) error {
		before, failure := observeListing(frame.dir.fd)
		if failure != nil || !sameDirectory(before.identity, frame.dir.id) {
			return errDirectory
		}
		frame.listing = before
		frame.next = uint32(result.count)
		failure = scanTreeNames(frame.dir, scratch[:], func(name []byte) error {
			if result.count == treeMaxEntries || frame.depth == treeMaxDepth {
				return errDirectory
			}
			var prefix []byte
			if frame.parent != treeNoParent {
				prefix = s.path(slot, s.work[frame.parent].inventory[slot])
			}
			length := len(name)
			if len(prefix) > 0 {
				length += len(prefix) + 1
			}
			if length > 1024 || uint64(length) > treeMaxPaths-result.paths {
				return errDirectory
			}
			stamp, failure := observeFile(frame.dir.fd, string(name))
			if failure != nil || !safeTreeEntry(stamp, s.root.id, s.owner) {
				return errDirectory
			}
			if stamp.identity.Mode&unix.S_IFMT == unix.S_IFREG {
				if uint64(stamp.identity.Size) > treeMaxPayload-result.payload {
					return errDirectory
				}
				result.payload += uint64(stamp.identity.Size)
			}
			e := treeEntry{stamp: stamp, pathOffset: uint32(result.paths), pathLength: uint16(length), depth: frame.depth + 1, parent: frame.parent}
			at := int(result.paths)
			if len(prefix) > 0 {
				copy(s.arenas[slot][at:], prefix)
				at += len(prefix)
				s.arenas[slot][at] = '/'
				at++
			}
			copy(s.arenas[slot][at:], name)
			s.work[result.count].inventory[slot] = e
			result.count++
			result.paths += uint64(length)
			return nil
		})
		frame.end = uint32(result.count)
		if failure != nil || checkListing(frame.dir.fd, before) != nil {
			return errDirectory
		}
		return nil
	}
	if boundary() != nil || enumerate(&frames[0]) != nil {
		return result, errDirectory
	}
	for depth >= 0 {
		if s.validateFrames(frames[:depth+1], boundary) != nil {
			return result, errDirectory
		}
		frame := &frames[depth]
		if frame.next == frame.end {
			if checkListing(frame.dir.fd, frame.listing) != nil {
				return result, errDirectory
			}
			if synchronize && syncDirectory(frame.dir) != nil {
				return result, errDirectory
			}
			if checkListing(frame.dir.fd, frame.listing) != nil {
				return result, errDirectory
			}
			if frame.owned {
				frame.owned = false
				if closeChain([]edge{frame.dir}) != nil {
					return result, errDirectory
				}
			}
			depth--
			continue
		}
		index := frame.next
		frame.next++
		e := s.work[index].inventory[slot]
		name := treeLeaf(s.path(slot, e))
		if e.stamp.identity.Mode&unix.S_IFMT == unix.S_IFDIR {
			child, failure := openTreeObserved(frame.dir, name, e.stamp.identity)
			if failure != nil {
				return result, errDirectory
			}
			depth++
			frames[depth] = treeWalkFrame{dir: child, parent: index, depth: e.depth, owned: true}
			if enumerate(&frames[depth]) != nil {
				return result, errDirectory
			}
		} else {
			_, stamp, digest, failure := treeRead(s, frame.dir, name, treeMaxFile, false, synchronize)
			if failure != nil || stamp != e.stamp {
				return result, errDirectory
			}
			s.work[index].inventory[slot].digest = digest
		}
	}
	for i := 0; i < result.count; i++ {
		s.work[i].order[slot] = uint32(i)
	}
	order := treeInventoryOrder{s: s, slot: slot, count: result.count}
	sort.Sort(order)
	for i := 1; i < result.count; i++ {
		a := s.work[s.work[i-1].order[slot]].inventory[slot]
		b := s.work[s.work[i].order[slot]].inventory[slot]
		if bytes.Equal(s.path(slot, a), s.path(slot, b)) {
			return result, errDirectory
		}
	}
	return result, nil
}

func (s *stageState) validateFrames(frames []treeWalkFrame, boundary func() error) error {
	if boundary() != nil {
		return errDirectory
	}
	for i := 1; i < len(frames); i++ {
		id, err := observeFD(frames[i].dir.fd)
		linked, linkedErr := observeAt(frames[i-1].dir.fd, frames[i].dir.name)
		if err != nil || linkedErr != nil || !sameDirectory(id, frames[i].dir.id) || !sameDirectory(id, linked) || !safeDirectory(id) || id.Owner != s.owner || !sameMount(id, s.root.id) {
			return errDirectory
		}
	}
	return nil
}

type treeInventoryOrder struct {
	s           *stageState
	slot, count int
	postorder   bool
}

func (o treeInventoryOrder) Len() int { return o.count }
func (o treeInventoryOrder) Swap(i, j int) {
	o.s.work[i].order[o.slot], o.s.work[j].order[o.slot] = o.s.work[j].order[o.slot], o.s.work[i].order[o.slot]
}
func (o treeInventoryOrder) Less(i, j int) bool {
	a := o.s.work[o.s.work[i].order[o.slot]].inventory[o.slot]
	b := o.s.work[o.s.work[j].order[o.slot]].inventory[o.slot]
	if o.postorder && a.depth != b.depth {
		return a.depth > b.depth
	}
	return bytes.Compare(o.s.path(o.slot, a), o.s.path(o.slot, b)) < 0
}

func (s *stageState) sameInventory(a, b treeInventory, left, right int) bool {
	if a != b {
		return false
	}
	for i := 0; i < a.count; i++ {
		x := s.work[s.work[i].order[left]].inventory[left]
		y := s.work[s.work[i].order[right]].inventory[right]
		if !bytes.Equal(s.path(left, x), s.path(right, y)) || x.stamp != y.stamp || x.digest != y.digest || x.depth != y.depth {
			return false
		}
	}
	return true
}

func (s *stageState) terminalStart() (bool, error) {
	s.mu.Lock()
	if s.phase != treeActive || s.roleRefs != 0 {
		s.mu.Unlock()
		return false, errDirectory
	}
	lentFDs := uint64(0)
	if s.lentTicket != nil {
		lentFDs = uint64(s.lentTicket.fds)
	}
	if s.fds-lentFDs+72 > treeMaxFDs || s.workspace-s.scratch-512*lentFDs+512*72+treeMaxScratch > treeMaxWorkspace {
		s.mu.Unlock()
		return false, errDirectory
	}
	s.phase = treeDraining
	for s.active != 0 {
		s.drained.Wait()
	}
	valid := s.phase == treeDraining
	s.mu.Unlock()
	if s.lent != nil && s.lent.Close() != nil {
		valid = false
		s.poison()
	}
	if !valid {
		return true, errDirectory
	}
	s.mu.Lock()
	valid = s.reserveLocked(72, treeMaxScratch, 0)
	if valid {
		s.terminalReserved = true
	}
	s.mu.Unlock()
	if !valid {
		s.poison()
		return true, errDirectory
	}
	return true, nil
}

func (s *stageState) terminalFinish(phase treePhase, primary error) error {
	cleanup := s.dispose()
	s.mu.Lock()
	defer s.mu.Unlock()
	if primary != nil || cleanup != nil || s.phase == treePreserve {
		s.phase = treePreserve
		return errDirectory
	}
	s.phase = phase
	return nil
}

func (t *TreeStage) PublishNoReplace(name string) (Identity, error) {
	if t == nil || t.state == nil || !validName(name) || len(name) > 255 {
		return Identity{}, errDirectory
	}
	s := t.state
	s.terminal.Lock()
	defer s.terminal.Unlock()
	same := name == s.root.name
	if same {
		return Identity{}, errDirectory
	}
	started, err := s.terminalStart()
	if err != nil {
		if started {
			return Identity{}, errors.Join(errDirectory, s.terminalFinish(treePreserve, err))
		}
		return Identity{}, errDirectory
	}
	id, attempted, err := s.promoteWithBoundary(name, s.validate)
	if s.terminalFinish(treePublished, err) != nil {
		if attempted {
			return Identity{}, errTreeUncertain
		}
		return Identity{}, errDirectory
	}
	return id, nil
}

func (s *stageState) promoteWithBoundary(name string, boundary func() error) (Identity, bool, error) {
	before, err := s.walkWithBoundary(0, true, boundary)
	if err == nil {
		after, failure := s.walkWithBoundary(1, false, boundary)
		if failure != nil || !s.sameInventory(before, after, 0, 1) {
			err = errDirectory
		}
	}
	if err != nil || boundary() != nil {
		return Identity{}, false, errDirectory
	}
	parent := s.parent[len(s.parent)-1]
	err = unix.Renameat2(parent.fd, s.root.name, parent.fd, name, unix.RENAME_NOREPLACE)
	if err == nil {
		s.root.name = strings.Clone(name)
		if syncDirectory(parent) != nil || boundary() != nil {
			err = errDirectory
		}
	}
	if err == nil {
		reopened, failure := openTreeObserved(parent, name, s.root.id)
		if failure != nil {
			err = errDirectory
		} else {
			original := s.root
			s.root = reopened
			final, failure := s.walkWithBoundary(2, false, boundary)
			if failure != nil || !s.sameInventory(before, final, 0, 2) || boundary() != nil {
				err = errDirectory
			}
			s.root = original
			if closeChain([]edge{reopened}) != nil {
				err = errDirectory
			}
		}
	}
	id, observed := observeFD(s.root.fd)
	if observed != nil || !sameDirectory(id, s.root.id) {
		err = errDirectory
	}
	if err != nil {
		return Identity{}, true, errTreeUncertain
	}
	return id, true, nil
}

func (t *TreeStage) Cleanup() error {
	if t == nil || t.state == nil {
		return errDirectory
	}
	s := t.state
	s.terminal.Lock()
	defer s.terminal.Unlock()
	s.mu.Lock()
	phase := s.phase
	if phase == treePublished || phase == treeRemoved {
		s.mu.Unlock()
		return nil
	}
	if phase == treePreserve {
		for s.active != 0 {
			s.drained.Wait()
		}
		s.mu.Unlock()
		return errors.Join(errDirectory, s.dispose())
	}
	s.mu.Unlock()
	started, err := s.terminalStart()
	if err != nil {
		if started {
			return errors.Join(errDirectory, s.terminalFinish(treePreserve, err))
		}
		return errDirectory
	}
	inventory, err := s.walk(0, false)
	if err == nil {
		err = s.removeInventory(inventory)
	}
	return s.terminalFinish(treeRemoved, err)
}
