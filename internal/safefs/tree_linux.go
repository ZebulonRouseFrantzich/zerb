//go:build linux

package safefs

import (
	"errors"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	treeMaxEntries     = 16384
	treeMaxDepth       = 64
	treeMaxPaths       = 16 << 20
	treeMaxFile        = 256 << 20
	treeMaxPayload     = 512 << 20
	treeMaxReads       = uint64(4 << 30)
	treeMaxFDs         = 256
	treeMaxWorkspace   = 128 << 20
	treeMaxScratch     = 128 << 10
	treeFixedWorkspace = 1<<20 + 2048*treeMaxEntries + 4*treeMaxPaths
)

type treePhase uint8

const (
	treeActive treePhase = iota
	treeDraining
	treePublished
	treeRemoved
	treePreserve
)

var errTreeUncertain = errors.New("tree publication outcome uncertain")

type stageTicket struct {
	bytes uint64
	fds   uint32
	flags uint32
}

type treeEntry struct {
	stamp      fileStamp
	digest     [32]byte
	pathOffset uint32
	pathLength uint16
	depth      uint8
	parent     uint32
}

type treeEntryWork struct {
	inventory [4]treeEntry
	order     [4]uint32
}

type stageState struct {
	terminal                       sync.Mutex
	mu                             sync.Mutex
	drained                        *sync.Cond
	phase                          treePhase
	owner                          uint32
	parent                         []edge
	root                           edge
	lent                           *Dir
	lentTicket                     *stageTicket
	active                         uint32
	roleRefs                       uint32
	entries, paths, payload, reads uint64
	fds, workspace, scratch        uint64
	slots                          [4]bool
	work                           []treeEntryWork
	arenas                         [4][]byte
	tracked                        bool
	terminalReserved               bool
}

// TreeStage value copies share one bounded, revocable lifetime (ARCH-020/030).
type TreeStage struct{ state *stageState }

func treeWorkspace(fds, scratch, result uint64) (uint64, bool) {
	if fds > treeMaxFDs || scratch > treeMaxScratch || result > treeMaxWorkspace {
		return 0, false
	}
	value := uint64(treeFixedWorkspace) + 512*fds + scratch
	if result > treeMaxWorkspace-value {
		return 0, false
	}
	return value + result, true
}

func (s *stageState) reserveLocked(fds, scratch, result uint64) bool {
	if s.fds > treeMaxFDs || s.scratch > treeMaxScratch || s.workspace > treeMaxWorkspace {
		return false
	}
	if fds > treeMaxFDs-s.fds || scratch > treeMaxScratch-s.scratch || result > treeMaxWorkspace-s.workspace {
		return false
	}
	added := 512*fds + scratch + result
	if added > treeMaxWorkspace-s.workspace {
		return false
	}
	s.fds += fds
	s.scratch += scratch
	s.workspace += added
	return true
}

type stageOperation struct {
	stage                *stageState
	fds, scratch, result uint64
	slot                 int
	role                 bool
}

func (s *stageState) begin(fds, scratch, result uint64) (*stageOperation, error) {
	return s.beginOperation(fds, scratch, result, false)
}

func (s *stageState) beginRole(fds, scratch, result uint64) (*stageOperation, error) {
	return s.beginOperation(fds, scratch, result, true)
}

func (s *stageState) beginOperation(fds, scratch, result uint64, role bool) (*stageOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != treeActive || s.active == 4 {
		return nil, errDirectory
	}
	slot := -1
	for i := range s.slots {
		if !s.slots[i] {
			slot = i
			break
		}
	}
	if slot < 0 || !s.reserveLocked(fds, scratch, result) {
		return nil, errDirectory
	}
	s.slots[slot] = true
	s.active++
	if role {
		s.roleRefs++
	}
	return &stageOperation{stage: s, fds: fds, scratch: scratch, result: result, slot: slot, role: role}, nil
}

func (op *stageOperation) finish(failed bool) bool {
	s := op.stage
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed && (s.phase == treeActive || s.phase == treeDraining) {
		s.phase = treePreserve
	}
	s.fds -= op.fds
	s.scratch -= op.scratch
	s.workspace -= 512*op.fds + op.scratch + op.result
	s.slots[op.slot] = false
	s.active--
	if op.role {
		s.roleRefs--
	}
	s.drained.Broadcast()
	return s.phase != treePreserve
}

func (s *stageState) beginTopology(role bool, scratch, result uint64) (*stageOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != treeActive || s.active == 4 {
		return nil, errDirectory
	}
	slot := -1
	for i := range s.slots {
		if !s.slots[i] {
			slot = i
			break
		}
	}
	if slot < 0 {
		return nil, errDirectory
	}
	fds := uint64(treeMaxFDs) - s.fds
	if !s.reserveLocked(fds, scratch, result) {
		return nil, errDirectory
	}
	s.active++
	s.slots[slot] = true
	if role {
		s.roleRefs++
	}
	return &stageOperation{stage: s, fds: fds, scratch: scratch, result: result, slot: slot, role: role}, nil
}

func (op *stageOperation) retainRole(fds, result uint64) *stageTicket {
	if !op.role {
		op.stage.mu.Lock()
		op.stage.roleRefs++
		op.stage.mu.Unlock()
	}
	ticket := op.retain(fds, result)
	ticket.flags = 1
	op.role = false
	return ticket
}

func (op *stageOperation) retain(fds, result uint64) *stageTicket {
	if fds > op.fds || result > op.result {
		panic("invalid stage reservation transfer")
	}
	op.fds -= fds
	op.result -= result
	return &stageTicket{fds: uint32(fds), bytes: result}
}

func (s *stageState) release(ticket *stageTicket, failed bool) {
	if ticket == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fds -= uint64(ticket.fds)
	s.workspace -= uint64(ticket.fds)*512 + ticket.bytes
	if ticket.flags&1 != 0 {
		s.roleRefs--
	}
	ticket.fds, ticket.bytes = 0, 0
	ticket.flags = 0
	if failed && (s.phase == treeActive || s.phase == treeDraining) {
		s.phase = treePreserve
	}
}

func (s *stageState) poison() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == treeActive || s.phase == treeDraining {
		s.phase = treePreserve
	}
}

func (s *stageState) live() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == treeActive || s.phase == treeDraining
}

func (s *stageState) readCharge(n uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.phase != treeActive && s.phase != treeDraining) || s.reads > treeMaxReads || n > treeMaxReads-s.reads {
		return errDirectory
	}
	s.reads += n
	return nil
}

func (s *stageState) addContent(pathBytes, payload uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.phase != treeActive && s.phase != treeDraining) || s.entries >= treeMaxEntries || s.paths > treeMaxPaths || s.payload > treeMaxPayload || pathBytes > treeMaxPaths-s.paths || payload > treeMaxPayload-s.payload {
		return false
	}
	s.entries++
	s.paths += pathBytes
	s.payload += payload
	return true
}

func stageRelative(ds *dirState, name string) (uint64, bool) {
	s := ds.gate
	depth := len(ds.chain) - len(s.parent)
	if depth < 1 || depth > treeMaxDepth || len(name) > 255 || !validName(name) {
		return 0, false
	}
	length := len(name)
	for i := len(s.parent) + 1; i < len(ds.chain); i++ {
		length += len(ds.chain[i].name) + 1
	}
	return uint64(length), length <= 1024
}

func duplicateTreeChain(chain []edge, extra int) (_ []edge, err error) {
	owned := make([]edge, 0, len(chain)+extra)
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain(owned))
		}
	}()
	for _, e := range chain {
		if len(e.name) > 255 {
			return nil, errDirectory
		}
		fd, failure := unix.FcntlInt(uintptr(e.fd), unix.F_DUPFD_CLOEXEC, 0)
		if failure != nil {
			return nil, errDirectory
		}
		e.fd, e.name = fd, strings.Clone(e.name)
		owned = append(owned, e)
	}
	return owned, nil
}

func openTreeObserved(parent edge, name string, expected Identity) (_ edge, err error) {
	if !validName(name) || len(name) > 255 || !safeDirectory(expected) || expected.Mode&0o7022 != 0 || expected.Owner != parent.id.Owner || !sameMount(expected, parent.id) {
		return edge{}, errDirectory
	}
	id, failure := observeFD(parent.fd)
	if failure != nil || !sameDirectory(id, parent.id) {
		return edge{}, errDirectory
	}
	fd, failure := unix.Openat2(parent.fd, name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: procResolve})
	if failure != nil {
		return edge{}, errDirectory
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain([]edge{{fd: fd}}))
		}
	}()
	opened, failure := observeFD(fd)
	linked, linkedFailure := observeAt(parent.fd, name)
	if failure != nil || linkedFailure != nil || !sameDirectory(opened, expected) || !sameDirectory(opened, linked) {
		return edge{}, errDirectory
	}
	return edge{fd: fd, name: name, id: opened}, nil
}

// StageTree exclusively creates an exact-0700 same-parent staging capability.
// A failed constructor closes known-owned descriptors and preserves names.
func (d *Dir) StageTree(name string, mode uint32) (_ *TreeStage, err error) {
	if d == nil || d.state == nil || !validName(name) || len(name) > 255 || mode != 0o700 || d.state.gate != nil {
		return nil, errDirectory
	}
	ds := d.state
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.chain) > treeMaxFDs {
		return nil, errDirectory
	}
	if _, ok := treeWorkspace(2*uint64(len(ds.chain))+10, 0, 0); !ok {
		return nil, errDirectory
	}
	if publicationChain(ds) != nil {
		return nil, errDirectory
	}
	s, err := createTreeAt(ds.chain, ds.owner, name)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	reserved := s.reserveLocked(8, 0, 0)
	s.mu.Unlock()
	if !reserved {
		return nil, errors.Join(errDirectory, s.dispose())
	}
	valid := publicationChain(ds) == nil && s.validate() == nil
	s.mu.Lock()
	s.fds -= 8
	s.workspace -= 512 * 8
	s.mu.Unlock()
	if !valid {
		return nil, errors.Join(errDirectory, s.dispose())
	}
	return &TreeStage{state: s}, nil
}

// createTreeAt owns only its duplicates. Public callers independently admit and
// revalidate the absolute chain; a failed core preserves all possible residue.
func createTreeAt(chain []edge, owner uint32, name string) (_ *stageState, err error) {
	if !validName(name) || len(name) > 255 {
		return nil, errDirectory
	}
	if len(chain) > treeMaxFDs {
		return nil, errDirectory
	}
	count := 2*uint64(len(chain)) + 2 + 8
	if _, ok := treeWorkspace(count, 0, 0); !ok || len(chain) == 0 || validateEdges(chain, owner) != nil || chain[len(chain)-1].id.Owner != owner {
		return nil, errDirectory
	}
	s := &stageState{owner: owner, root: edge{fd: -1}, workspace: treeFixedWorkspace + 512*count, fds: count}
	s.drained = sync.NewCond(&s.mu)
	defer func() {
		if err != nil {
			err = errors.Join(err, s.dispose())
		}
	}()
	s.work = make([]treeEntryWork, treeMaxEntries)
	for i := range s.arenas {
		s.arenas[i] = make([]byte, treeMaxPaths)
	}
	s.parent, err = duplicateTreeChain(chain, 0)
	if err != nil {
		return nil, errDirectory
	}
	parent := s.parent[len(s.parent)-1]
	if failure := unix.Mkdirat(parent.fd, name, 0o700); failure != nil {
		if errors.Is(failure, unix.EEXIST) {
			return nil, errDirectoryExists
		}
		return nil, errDirectory
	}
	id, failure := observeAt(parent.fd, name)
	if failure != nil || id.Mode != unix.S_IFDIR|0o700 || id.Owner != s.owner || !sameMount(id, parent.id) {
		return nil, errDirectory
	}
	s.root, err = openTreeObserved(parent, name, id)
	if err != nil {
		return nil, errDirectory
	}
	s.root.name = strings.Clone(name)
	s.root.mkdir = true
	lentChain, failure := duplicateTreeChain(s.parent, 1)
	if failure != nil {
		return nil, errDirectory
	}
	fd, failure := unix.FcntlInt(uintptr(s.root.fd), unix.F_DUPFD_CLOEXEC, 0)
	if failure != nil {
		return nil, errors.Join(errDirectory, closeChain(lentChain))
	}
	lentEdge := s.root
	lentEdge.fd = fd
	lentEdge.name = strings.Clone(name)
	lentChain = append(lentChain, lentEdge)
	s.fds = uint64(len(s.parent) + 1 + len(lentChain))
	s.workspace = treeFixedWorkspace + 512*s.fds
	s.tracked = true
	s.lent = &Dir{state: &dirState{chain: lentChain, owner: s.owner, identity: id, mkdir: true, gate: s, ticket: &stageTicket{fds: uint32(len(lentChain))}}}
	s.lentTicket = s.lent.state.ticket
	if validateEdges(s.parent, s.owner) != nil || validateEdges(lentChain, s.owner) != nil {
		return nil, errDirectory
	}
	return s, nil
}

func (t *TreeStage) Root() *Dir {
	if t == nil || t.state == nil {
		return nil
	}
	s := t.state
	s.mu.Lock()
	active := s.phase == treeActive
	lent := s.lent
	s.mu.Unlock()
	if !active || lent == nil {
		return nil
	}
	lent.state.mu.Lock()
	closed := lent.state.closed
	lent.state.mu.Unlock()
	if closed {
		return nil
	}
	return lent
}

func (s *stageState) validate() error {
	if !s.live() || len(s.parent) == 0 || s.root.fd < 0 || validateChain(s.parent, s.owner) != nil {
		return errDirectory
	}
	return s.validateEdges()
}

func (s *stageState) validateEdges() error {
	if len(s.parent) == 0 || s.root.fd < 0 || validateEdges(s.parent, s.owner) != nil || s.parent[len(s.parent)-1].id.Owner != s.owner {
		return errDirectory
	}
	parent := s.parent[len(s.parent)-1]
	id, err := observeFD(s.root.fd)
	if err != nil || !sameDirectory(id, s.root.id) || id.Mode != unix.S_IFDIR|0o700 || id.Owner != s.owner || !sameMount(id, parent.id) {
		return errDirectory
	}
	linked, err := observeAt(parent.fd, s.root.name)
	if err != nil || !sameDirectory(linked, id) {
		return errDirectory
	}
	return nil
}

func (s *stageState) dispose() error {
	var result error
	if s.lent != nil {
		result = s.lent.Close()
	}
	parent, root := s.parent, s.root
	s.parent = nil
	s.root = edge{fd: -1}
	if root.fd >= 0 {
		result = errors.Join(result, closeChain([]edge{root}))
	}
	result = errors.Join(result, closeChain(parent))
	s.mu.Lock()
	if s.tracked {
		count := uint64(len(parent))
		if root.fd >= 0 {
			count++
		}
		s.fds -= count
		s.workspace -= 512 * count
	}
	if s.terminalReserved {
		s.fds -= 72
		s.scratch -= treeMaxScratch
		s.workspace -= 512*72 + treeMaxScratch
		s.terminalReserved = false
	}
	s.mu.Unlock()
	s.work = nil
	for i := range s.arenas {
		s.arenas[i] = nil
	}
	return result
}
