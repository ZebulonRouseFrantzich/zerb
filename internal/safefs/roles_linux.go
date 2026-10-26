//go:build linux

package safefs

import (
	"errors"
	"path"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
)

// Role is a closed protected-directory role (ARCH-020/030).
type Role uint8

const (
	RoleState Role = iota + 1
	RoleData
	RoleCache
	RoleSource
	RoleTree
	RoleToolchain
	RoleManager
	RoleStaging
	RoleDestination
)

var errRoles = errors.New("unsafe protected role operation")

// Reserve room for the topology reader's initial pins and validation scratch FD.
const rolesMaxOwned = 4096 - 65

type roleEntry struct {
	role    Role
	target  string
	chain   []edge
	missing []string
	owner   uint32
	gate    *stageState
	ticket  *stageTicket
}

type rolesState struct {
	mu             sync.Mutex
	entries        []roleEntry
	namespace      *namespacePin
	home           string
	poisoned       bool
	closed         bool
	generation     uint64
	metadataGate   *stageState
	metadataTicket *stageTicket
}

// Roles value copies share one descriptor-owned lifetime.
type Roles struct{ state *rolesState }

// OpenRoles reserves existing or prospective roots without creating directories.
func OpenRoles(roots paths.Roots, owner uint32) (_ *Roles, err error) {
	targets := []string{roots.State(), roots.Data(), roots.Cache()}
	if !rolePath(roots.Home()) {
		return nil, errRoles
	}
	for i, target := range targets {
		if !rolePath(target) {
			return nil, errRoles
		}
		for _, earlier := range targets[:i] {
			if roleContains(earlier, target) || roleContains(target, earlier) {
				return nil, errRoles
			}
		}
	}
	s := &rolesState{home: roots.Home()}
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		if err != nil {
			if s.closeLocked() != nil {
				err = errRoles
			}
			err = errRoles
		}
	}()
	t, e := openTopology(nil)
	if e != nil {
		return nil, errRoles
	}
	defer func() {
		if t != nil && t.close() != nil {
			err = errRoles
		}
	}()
	for i, target := range targets {
		entry, e := openRolePrefix(Role(i+1), target, owner, rolesMaxOwned-roleFDCount(s.entries))
		if e != nil {
			return nil, errRoles
		}
		s.entries = append(s.entries, entry)
	}
	t.frontendFDs = roleFDCount(s.entries)
	if !roleEvidence(s.entries, nil) || !roleTopology(t, s.entries, s.home) {
		return nil, errRoles
	}
	s.namespace, e = t.retainNamespace()
	if e != nil {
		return nil, errRoles
	}
	t.frontendFDs++
	if !roleEvidence(s.entries, nil) || t.finish() != nil {
		return nil, errRoles
	}
	e = t.close()
	t = nil
	if e != nil || !roleEvidence(s.entries, nil) {
		return nil, errRoles
	}
	return &Roles{state: s}, nil
}

// Validate never adopts an appeared prefix or forgets a registered directory.
func (r *Roles) validateWithoutStage() error {
	if r == nil || r.state == nil {
		return errRoles
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stageFreeLocked() {
		return errRoles
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return s.validateLocked()
}

func (s *rolesState) validateLocked() error {
	if s.closed || s.poisoned {
		return errRoles
	}
	t, err := openTopology(s.namespace)
	if err != nil {
		s.poisoned = true
		return errRoles
	}
	t.frontendFDs = roleFDCount(s.entries) + 1
	ok := roleEvidence(s.entries, nil) && roleTopology(t, s.entries, s.home) && roleEvidence(s.entries, nil)
	if ok {
		ok = t.finish() == nil
	}
	if t.close() != nil {
		ok = false
	}
	if ok {
		ok = roleEvidence(s.entries, nil)
	}
	if !ok {
		s.poisoned = true
		return errRoles
	}
	return nil
}

// Add binds one privately-created prefix edge, or retains an operational Dir.
func (r *Roles) addWithoutStage(role Role, dir *Dir) (err error) {
	if r == nil || r.state == nil {
		return errRoles
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stageFreeLocked() {
		return errRoles
	}
	if s.generation == ^uint64(0) {
		return errRoles
	}
	if dir != nil && dir.state != nil && dir.state.gate != nil {
		return errRoles
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if s.closed || s.poisoned {
		return errRoles
	}
	if role < RoleState || role > RoleDestination || dir == nil || dir.state == nil {
		return s.rejectLocked()
	}
	d := dir.state
	d.mu.Lock()
	if d.closed || len(d.chain) == 0 || validateChain(d.chain, d.owner) != nil || d.chain[len(d.chain)-1].id.Owner != d.owner {
		d.mu.Unlock()
		return s.rejectLocked()
	}
	target, valid := roleChainPath(d.chain)
	if !valid {
		d.mu.Unlock()
		return s.rejectLocked()
	}
	candidate := append([]roleEntry(nil), s.entries...)
	var added, retired [][]edge
	committed := false
	defer func() {
		if !committed && closeRoleChains(added) != nil {
			err = errRoles
		}
	}()
	var affected []int
	if role <= RoleCache {
		var ok bool
		affected, ok = roleAdvance(s.entries, role, d, target)
		if !ok {
			d.mu.Unlock()
			return s.rejectLocked()
		}
		for _, index := range affected {
			if roleFDCount(s.entries)+roleChainCount(added)+len(d.chain)+1 > rolesMaxOwned {
				d.mu.Unlock()
				return s.rejectLocked()
			}
			chain, err := duplicateChain(d.chain)
			if err != nil {
				d.mu.Unlock()
				return s.rejectLocked()
			}
			added = append(added, chain)
			retired = append(retired, candidate[index].chain)
			candidate[index].chain = chain
			candidate[index].missing = candidate[index].missing[1:]
		}
	} else {
		if len(candidate) >= 64 || roleFDCount(s.entries)+len(d.chain)+1 > rolesMaxOwned {
			d.mu.Unlock()
			return s.rejectLocked()
		}
		chain, err := duplicateChain(d.chain)
		if err != nil {
			d.mu.Unlock()
			return s.rejectLocked()
		}
		added = append(added, chain)
		candidate = append(candidate, roleEntry{role: role, target: target, chain: chain, owner: d.owner})
	}
	d.mu.Unlock()
	t, err := openTopology(s.namespace)
	if err != nil {
		s.poisoned = true
		return errRoles
	}
	t.frontendFDs = roleFDCount(s.entries) + roleChainCount(added) + 1
	// The sole expected absent-to-present edge is checked through the candidate.
	priorOK := roleEvidence(s.entries, affected) && roleTopology(t, s.entries, s.home)
	candidateOK := priorOK && roleEvidence(candidate, nil) && roleTopology(t, candidate, s.home)
	priorOK = priorOK && roleEvidence(s.entries, affected)
	if candidateOK {
		candidateOK = roleEvidence(candidate, nil)
	}
	finishOK := t.finish() == nil
	closeOK := t.close() == nil
	priorOK = priorOK && roleEvidence(s.entries, affected)
	if candidateOK {
		candidateOK = roleEvidence(candidate, nil)
	}
	if !priorOK || !finishOK || !closeOK {
		s.poisoned = true
		return errRoles
	}
	if !candidateOK {
		if len(affected) != 0 {
			s.poisoned = true
		}
		return errRoles
	}
	// Candidate duplicates keep every retained identity pinned during retirement.
	for _, index := range affected {
		s.entries[index].chain = nil
	}
	if closeRoleChains(retired) != nil || !roleEvidence(candidate, nil) {
		s.poisoned = true
		return errRoles
	}
	s.entries = candidate
	s.generation++
	committed = true
	return nil
}

func (s *rolesState) rejectLocked() error {
	if s.validateLocked() != nil {
		return errRoles
	}
	return errRoles
}

func (s *rolesState) stageFreeLocked() bool {
	if s.metadataGate != nil {
		return false
	}
	for _, entry := range s.entries {
		if entry.gate != nil {
			return false
		}
	}
	return true
}

// Close clears ownership before close and never closes caller Dirs.
func (r *Roles) Close() error {
	if r == nil || r.state == nil {
		return nil
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return s.closeLocked()
}

func (s *rolesState) closeLocked() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.generation != ^uint64(0) {
		s.generation++
	}
	entries, namespace := s.entries, s.namespace
	metadataGate, metadataTicket := s.metadataGate, s.metadataTicket
	s.entries, s.namespace = nil, nil
	s.metadataGate, s.metadataTicket = nil, nil
	var failed bool
	for i, entry := range entries {
		entries[i].chain = nil
		if closeChain(entry.chain) != nil {
			failed = true
		}
	}
	if namespace != nil && namespace.close() != nil {
		failed = true
	}
	for i, entry := range entries {
		entries[i] = roleEntry{}
		if entry.gate != nil {
			entry.gate.release(entry.ticket, failed)
		}
	}
	if metadataGate != nil {
		metadataGate.release(metadataTicket, failed)
	}
	if failed {
		return errRoles
	}
	return nil
}

func openRolePrefix(role Role, target string, owner uint32, budget int) (_ roleEntry, err error) {
	entry := roleEntry{role: role, target: target, owner: owner}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeChain(entry.chain))
		}
	}()
	if !rolePath(target) || budget < 1 {
		return roleEntry{}, errRoles
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return roleEntry{}, errRoles
	}
	entry.chain = append(entry.chain, edge{fd: fd})
	id, e := observeFD(fd)
	if e != nil {
		return roleEntry{}, errRoles
	}
	entry.chain[0].id = id
	owned, e := advanceOwner(id, owner, false)
	if e != nil {
		return roleEntry{}, errRoles
	}
	var parts []string
	if target != "/" {
		parts = strings.Split(target[1:], "/")
	}
	for i, part := range parts {
		if validateChain(entry.chain, owner) != nil {
			return roleEntry{}, errRoles
		}
		id, e := observeAt(entry.chain[len(entry.chain)-1].fd, part)
		if errors.Is(e, unix.ENOENT) {
			entry.missing = parts[i:]
			break
		}
		if e != nil {
			return roleEntry{}, errRoles
		}
		owned, e = advanceOwner(id, owner, owned)
		if e != nil || len(entry.chain) >= budget {
			return roleEntry{}, errRoles
		}
		child, e := openObserved(entry.chain[len(entry.chain)-1].fd, part, id)
		if e != nil {
			return roleEntry{}, errRoles
		}
		entry.chain = append(entry.chain, child)
	}
	if validateChain(entry.chain, owner) != nil || (len(entry.missing) == 0 && entry.chain[len(entry.chain)-1].id.Owner != owner) {
		return roleEntry{}, errRoles
	}
	return entry, nil
}

func roleEvidence(entries []roleEntry, skip []int) bool {
	for i, entry := range entries {
		if validateChain(entry.chain, entry.owner) != nil {
			return false
		}
		if len(entry.missing) == 0 {
			if entry.chain[len(entry.chain)-1].id.Owner != entry.owner {
				return false
			}
			continue
		}
		ignore := false
		for _, index := range skip {
			if i == index {
				ignore = true
				break
			}
		}
		if !ignore {
			if _, err := observeAt(entry.chain[len(entry.chain)-1].fd, entry.missing[0]); !errors.Is(err, unix.ENOENT) {
				return false
			}
		}
	}
	return true
}

func roleAdvance(entries []roleEntry, role Role, d *dirState, target string) ([]int, bool) {
	var base *roleEntry
	for i := range entries {
		if entries[i].role == role {
			base = &entries[i]
			break
		}
	}
	if base == nil || len(base.missing) == 0 || !d.mkdir || d.owner != base.owner || len(d.chain) != len(base.chain)+1 {
		return nil, false
	}
	prefix, ok := roleChainPath(base.chain)
	if !ok || target != path.Join(prefix, base.missing[0]) {
		return nil, false
	}
	last := d.chain[len(d.chain)-1]
	if last.name != base.missing[0] || last.id.Mode != unix.S_IFDIR|0o700 || last.id.Owner != base.owner {
		return nil, false
	}
	var affected []int
	for i, entry := range entries {
		if len(entry.missing) == 0 {
			continue
		}
		name, ok := roleChainPath(entry.chain)
		if !ok {
			return nil, false
		}
		if name != prefix || entry.missing[0] != last.name {
			continue
		}
		if entry.owner != d.owner || len(entry.chain)+1 != len(d.chain) {
			return nil, false
		}
		for j, old := range entry.chain {
			if old.name != d.chain[j].name || !sameDirectory(old.id, d.chain[j].id) {
				return nil, false
			}
		}
		affected = append(affected, i)
	}
	return affected, len(affected) > 0
}

func roleTopology(t *topology, entries []roleEntry, home string) bool {
	paths := make([]topologyPath, len(entries))
	for i, entry := range entries {
		paths[i] = topologyPath{target: entry.target, chain: entry.chain, missing: entry.missing, owner: entry.owner}
	}
	footprints, err := t.footprints(paths)
	if err != nil || len(footprints) != len(entries) {
		return false
	}
	for i, entry := range entries {
		if !rolePlacement(entries, entry, home) {
			return false
		}
		if !topologyCompatible(footprints[i], footprints[i], true) {
			return false
		}
		for j := 0; j < i; j++ {
			allowed, valid := roleRelation(entries, entry, entries[j])
			if !valid || !topologyCompatible(footprints[i], footprints[j], allowed) {
				return false
			}
		}
	}
	return true
}

func rolePlacement(entries []roleEntry, entry roleEntry, home string) bool {
	if entry.role == RoleDestination {
		return entry.target != home
	}
	if entry.role <= RoleCache {
		return true
	}
	for _, parent := range entries {
		if entry.role == RoleStaging && parent.role == RoleCache && roleStagingPath(parent.target, entry.target) {
			return true
		}
		if roleOperational(entry.role) && (parent.role == RoleData || parent.role == RoleStaging) && roleStrictContains(parent.target, entry.target) {
			_, valid := roleRelation(entries, parent, entry)
			if valid {
				return true
			}
		}
	}
	return false
}

// The boolean authorizes only identical namespace-route containment.
func roleRelation(entries []roleEntry, a, b roleEntry) (bool, bool) {
	if a.target == b.target {
		return false, false
	}
	if !roleContains(a.target, b.target) && !roleContains(b.target, a.target) {
		return false, true
	}
	parent, child := a, b
	if roleContains(b.target, a.target) {
		parent, child = b, a
	}
	if parent.role == RoleData {
		branch := ""
		switch child.role {
		case RoleSource:
			branch = "sources"
		case RoleTree:
			branch = "trees"
		case RoleToolchain:
			branch = "toolchain"
		case RoleManager:
			branch = "installs"
		}
		return true, branch != "" && roleContains(path.Join(parent.target, branch), child.target)
	}
	if parent.role == RoleCache {
		if child.role == RoleStaging {
			return true, roleStagingPath(parent.target, child.target)
		}
		if roleOperational(child.role) {
			for _, stage := range entries {
				if stage.role == RoleStaging && roleStagingPath(parent.target, stage.target) && roleStrictContains(stage.target, child.target) {
					return true, true
				}
			}
		}
	}
	if parent.role == RoleStaging && roleOperational(child.role) {
		return true, true
	}
	return false, false
}

func roleOperational(role Role) bool { return role >= RoleSource && role <= RoleManager }
func roleStagingPath(cache, target string) bool {
	if !roleStrictContains(cache, target) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(target, cache+"/"), "/")
	if len(parts) < 4 || parts[0] != "instances" || len(parts[1]) != 32 || parts[2] != "downloads" {
		return false
	}
	for _, c := range parts[1] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func roleContains(parent, child string) bool {
	return parent == child || roleStrictContains(parent, child)
}
func roleStrictContains(parent, child string) bool {
	if parent == "/" {
		return len(child) > 1 && child[0] == '/'
	}
	return strings.HasPrefix(child, parent+"/")
}
func rolePath(name string) bool {
	return len(name) > 0 && len(name) <= 4096 && path.IsAbs(name) && path.Clean(name) == name && !strings.ContainsRune(name, 0) && (name == "/" || strings.Count(name, "/") <= 256)
}
func roleChainPath(chain []edge) (string, bool) {
	if len(chain) == 0 || chain[0].name != "" || len(chain) > 257 {
		return "", false
	}
	var b strings.Builder
	for _, entry := range chain[1:] {
		if !validName(entry.name) || b.Len()+1+len(entry.name) > 4096 {
			return "", false
		}
		b.WriteByte('/')
		b.WriteString(entry.name)
	}
	if b.Len() == 0 {
		return "/", true
	}
	return b.String(), true
}
func roleFDCount(entries []roleEntry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.chain)
	}
	return total
}
func roleChainCount(chains [][]edge) int {
	total := 0
	for _, chain := range chains {
		total += len(chain)
	}
	return total
}
func closeRoleChains(chains [][]edge) error {
	var err error
	for _, chain := range chains {
		err = errors.Join(err, closeChain(chain))
	}
	return err
}
