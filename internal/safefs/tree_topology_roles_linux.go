//go:build linux

package safefs

import (
	"runtime"
	"strings"
)

type treeRoleAdmission struct {
	gates      [64]*stageState
	ops        [64]*stageOperation
	count      int
	generation uint64
	baseFDs    int
}

func (r *Roles) hasStage(dir *Dir) bool {
	if dir != nil && dir.state != nil && dir.state.gate != nil {
		return true
	}
	if r == nil || r.state == nil {
		return false
	}
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if r.state.metadataGate != nil {
		return true
	}
	for _, entry := range r.state.entries {
		if entry.gate != nil {
			return true
		}
	}
	return false
}

func (r *Roles) admitTreeRoles(dir *Dir) (treeRoleAdmission, error) {
	if r == nil || r.state == nil {
		return treeRoleAdmission{}, errRoles
	}
	a := treeRoleAdmission{}
	s := r.state
	s.mu.Lock()
	if s.closed || s.poisoned {
		s.mu.Unlock()
		return treeRoleAdmission{}, errRoles
	}
	a.generation = s.generation
	add := func(gate *stageState) bool {
		if gate == nil {
			return true
		}
		for _, old := range a.gates[:a.count] {
			if old == gate {
				return true
			}
		}
		if a.count == len(a.gates) {
			return false
		}
		a.gates[a.count] = gate
		a.count++
		return true
	}
	for _, entry := range s.entries {
		if !add(entry.gate) {
			s.mu.Unlock()
			return treeRoleAdmission{}, errRoles
		}
	}
	if !add(s.metadataGate) {
		s.mu.Unlock()
		return treeRoleAdmission{}, errRoles
	}
	if dir != nil && dir.state != nil && !add(dir.state.gate) {
		s.mu.Unlock()
		return treeRoleAdmission{}, errRoles
	}
	s.mu.Unlock()
	for i, gate := range a.gates[:a.count] {
		// The temporary observer covers roster changes before we reacquire Roles.
		op, err := gate.beginTopology(true, 32<<10, treeTopologyBytes)
		if err != nil {
			a.finish(false)
			return treeRoleAdmission{}, errRoles
		}
		a.ops[i] = op
		a.baseFDs = max(a.baseFDs, treeMaxFDs-int(op.fds))
	}
	return a, nil
}
func (a *treeRoleAdmission) finish(failed bool) bool {
	ok := !failed
	for i, op := range a.ops[:a.count] {
		if op != nil {
			if !op.finish(failed) {
				ok = false
			}
			a.ops[i] = nil
		}
	}
	return ok
}
func (a *treeRoleAdmission) operation(gate *stageState) *stageOperation {
	for i, g := range a.gates[:a.count] {
		if g == gate {
			return a.ops[i]
		}
	}
	return nil
}
func (a *treeRoleAdmission) unchanged(s *rolesState) bool {
	return !s.closed && !s.poisoned && s.generation == a.generation
}

// Validate enrolls all retained stage gates before taking the operational lock.
func (r *Roles) Validate() (err error) {
	if !r.hasStage(nil) {
		return r.validateWithoutStage()
	}
	a, err := r.admitTreeRoles(nil)
	if err != nil {
		return errRoles
	}
	s := r.state
	s.mu.Lock()
	if !a.unchanged(s) {
		s.mu.Unlock()
		a.finish(false)
		return errRoles
	}
	defer s.mu.Unlock()
	failed := true
	defer func() {
		if !a.finish(failed) {
			err = errRoles
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	frontend := a.baseFDs + roleFDCount(s.entries) + 1
	if frontend > treeMaxFDs-11 {
		failed = false
		return errRoles
	}
	t, err := openTreeTopology(s.namespace, frontend, a.ops[:a.count])
	if err != nil {
		s.poisoned = true
		return errRoles
	}
	ok := roleEvidence(s.entries, nil) && treeRoleTopology(t, s.entries, s.home) && roleEvidence(s.entries, nil)
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
	failed = false
	return nil
}

// Add transfers the exact stage-derived chain and target reservation to Roles.
func (r *Roles) Add(role Role, dir *Dir) (err error) {
	if !r.hasStage(dir) {
		return r.addWithoutStage(role, dir)
	}
	if role < RoleState || role > RoleDestination || dir == nil || dir.state == nil {
		return errRoles
	}
	a, err := r.admitTreeRoles(dir)
	if err != nil {
		return errRoles
	}
	s := r.state
	s.mu.Lock()
	if !a.unchanged(s) {
		s.mu.Unlock()
		a.finish(false)
		return errRoles
	}
	defer s.mu.Unlock()
	failed := true
	defer func() {
		if !a.finish(failed) {
			err = errRoles
		}
	}()
	if s.generation == ^uint64(0) {
		failed = false
		return errRoles
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := dir.state
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		failed = false
		return errRoles
	}
	if len(d.chain) == 0 {
		d.mu.Unlock()
		return errRoles
	}
	if len(d.chain) > treeMaxFDs {
		d.mu.Unlock()
		failed = false
		return errRoles
	}
	// Reserve every possible new root-advance chain before the first validation.
	newChains := 1
	if role <= RoleCache {
		newChains = 3
	}
	frontend := a.baseFDs + roleFDCount(s.entries) + newChains*len(d.chain) + 1
	if frontend > treeMaxFDs-11 {
		d.mu.Unlock()
		failed = false
		return errRoles
	}
	target, valid := roleChainPath(d.chain)
	if !valid {
		d.mu.Unlock()
		failed = false
		return errRoles
	}
	var candidate [64]roleEntry
	if len(s.entries) > len(candidate) {
		d.mu.Unlock()
		s.poisoned = true
		return errRoles
	}
	copy(candidate[:], s.entries)
	count := len(s.entries)
	var affected []int
	if role <= RoleCache {
		var ok bool
		affected, ok = roleAdvance(s.entries, role, d, target)
		if !ok {
			d.mu.Unlock()
			failed = false
			return errRoles
		}
	} else {
		if count == len(candidate) {
			d.mu.Unlock()
			failed = false
			return errRoles
		}
		affected = []int{count}
		candidate[count] = roleEntry{role: role, owner: d.owner}
		count++
	}
	metadataGate := s.metadataGate
	if metadataGate == nil {
		metadataGate = a.gates[0]
	}
	metadataOp := a.operation(metadataGate)
	if count > cap(s.entries) && metadataOp == nil {
		d.mu.Unlock()
		s.poisoned = true
		return errRoles
	}
	if !s.prepareTreeRolePlacement(role, d, target, candidate[:count], affected) {
		d.mu.Unlock()
		failed = false
		return errRoles
	}
	if validateChain(d.chain, d.owner) != nil || d.chain[len(d.chain)-1].id.Owner != d.owner {
		d.mu.Unlock()
		return errRoles
	}
	var added [64][]edge
	var retired [64]roleEntry
	committed := false
	defer func() {
		if !committed {
			for i, chain := range added {
				added[i] = nil
				if closeChain(chain) != nil {
					err = errRoles
					failed = true
				}
			}
		}
	}()
	for i, index := range affected {
		var old roleEntry
		if index < len(s.entries) {
			old = s.entries[index]
		}
		gate := d.gate
		if gate == nil {
			gate = old.gate
		}
		if old.gate != nil && d.gate != nil && old.gate != d.gate {
			d.mu.Unlock()
			s.poisoned = true
			return errRoles
		}
		chain, e := duplicateTreeChain(d.chain, 0)
		if e != nil {
			d.mu.Unlock()
			return errRoles
		}
		added[i] = chain
		retired[i] = old
		candidate[index].chain = chain
		candidate[index].target = strings.Clone(candidate[index].target)
		candidate[index].gate = gate
		candidate[index].ticket = nil
	}
	d.mu.Unlock()
	frontend = a.baseFDs + roleFDCount(s.entries) + roleChainCount(added[:]) + 1
	t, err := openTreeTopology(s.namespace, frontend, a.ops[:a.count])
	if err != nil {
		s.poisoned = true
		return errRoles
	}
	if !s.verifyTreeRoleCandidate(t, candidate[:count], affected, role) {
		return errRoles
	}
	if !s.retireTreeRoleEntries(affected, retired[:len(affected)], candidate[:count]) {
		return errRoles
	}
	for _, index := range affected {
		entry := &candidate[index]
		if entry.gate != nil {
			op := a.operation(entry.gate)
			if op == nil {
				s.poisoned = true
				return errRoles
			}
			entry.ticket = op.retainRole(uint64(len(entry.chain)), uint64(len(entry.target)))
		}
	}
	s.retainTreeRoleEntries(candidate[:count], metadataGate, metadataOp)
	s.generation++
	committed = true
	failed = false
	return nil
}

func (s *rolesState) retainTreeRoleEntries(candidate []roleEntry, gate *stageState, op *stageOperation) {
	if len(candidate) <= cap(s.entries) {
		s.entries = s.entries[:len(candidate)]
		copy(s.entries, candidate)
		return
	}
	next := make([]roleEntry, len(candidate))
	copy(next, candidate)
	ticket := op.retain(0, uint64(len(candidate))*96+16)
	old, oldGate, oldTicket := s.entries, s.metadataGate, s.metadataTicket
	s.entries, s.metadataGate, s.metadataTicket = next, gate, ticket
	clear(old)
	if oldGate != nil {
		oldGate.release(oldTicket, false)
	}
}

// Pure placement and relation checks precede the first named observation.
func (s *rolesState) prepareTreeRolePlacement(role Role, d *dirState, target string, candidate []roleEntry, affected []int) bool {
	for _, index := range affected {
		entry := &candidate[index]
		if entry.gate != nil && d.gate != nil && entry.gate != d.gate {
			return false
		}
		if role > RoleCache {
			entry.target = target
		}
		entry.chain = d.chain
		if role <= RoleCache {
			entry.missing = entry.missing[1:]
		}
	}
	// Pure role grammar is a parameter preflight, before any named observation.
	for i, entry := range candidate {
		if !rolePlacement(candidate, entry, s.home) {
			return false
		}
		for j := 0; j < i; j++ {
			if _, valid := roleRelation(candidate, entry, candidate[j]); !valid {
				return false
			}
		}
	}
	return true
}

func (s *rolesState) verifyTreeRoleCandidate(t *treeTopology, candidate []roleEntry, affected []int, role Role) bool {
	priorOK := roleEvidence(s.entries, affected) && treeRoleTopology(t, s.entries, s.home)
	candidateOK := priorOK && roleEvidence(candidate, nil) && treeRoleTopology(t, candidate, s.home)
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
		return false
	}
	if !candidateOK {
		if role <= RoleCache {
			s.poisoned = true
		}
		return false
	}
	return true
}

func (s *rolesState) retireTreeRoleEntries(affected []int, retired, candidate []roleEntry) bool {
	// Clear ownership before retirement; failed closes cannot be retried.
	for _, index := range affected {
		if index < len(s.entries) {
			s.entries[index] = roleEntry{}
		}
	}
	retireFailed := false
	for i, entry := range retired {
		retired[i].chain = nil
		retired[i].target = ""
		retired[i].missing = nil
		if closeChain(entry.chain) != nil {
			retireFailed = true
		}
	}
	for i, entry := range retired {
		retired[i] = roleEntry{}
		if entry.gate != nil {
			entry.gate.release(entry.ticket, retireFailed)
		}
	}
	if retireFailed || !roleEvidence(candidate, nil) {
		s.poisoned = true
		return false
	}
	return true
}
