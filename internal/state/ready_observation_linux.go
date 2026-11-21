//go:build linux

package state

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

var errReadyObservation = errors.New("initialized observation unavailable")

// Value copies retain one observation and the same non-upgradable admission OFD.
// Facts never confer command admission, writer ownership or completion authority.
type readyObservation struct{ state *readyObservationState }

type readyObservationState struct {
	mu                          sync.Mutex
	ctx                         context.Context
	roots                       paths.Roots
	owner                       uint32
	roles                       *safefs.Roles
	dirs                        []*safefs.Dir
	files                       []*safefs.File
	admission                   *safefs.Lock
	lockID                      safefs.Identity
	lockMode                    safefs.LockMode
	held                        bool
	closed                      bool
	stateDir, dataDir, cacheDir *safefs.Dir
	controls                    [3]*safefs.Dir
	graph                       readyGraph
	facts                       readyFacts
	hook                        freshBoundary
}

func readyAdmissionMode(intent Intent) (safefs.LockMode, bool) {
	switch intent {
	case IntentDoctor, IntentExec, IntentShell:
		return safefs.LockShared, true
	case IntentProfileList, IntentPlan, IntentBootstrap, IntentApply, IntentRecoverResume, IntentRecoverRepair:
		return safefs.LockExclusive, true
	default:
		return 0, false
	}
}

func observeReady(ctx context.Context, roots paths.Roots, intent Intent) (*readyObservation, error) {
	return observeReadyWithBoundary(ctx, roots, intent, nil)
}

func observeReadyWithBoundary(ctx context.Context, roots paths.Roots, intent Intent, hook freshBoundary) (*readyObservation, error) {
	mode, ok := readyAdmissionMode(intent)
	if !ok || ctx == nil || ctx.Err() != nil {
		return nil, errReadyObservation
	}
	s := &readyObservationState{ctx: ctx, roots: roots, owner: uint32(os.Geteuid()), lockMode: mode, hook: hook}
	if err := s.open(); err != nil {
		return nil, errors.Join(errReadyObservation, err, s.close())
	}
	return &readyObservation{state: s}, nil
}

func (s *readyObservationState) open() error {
	var err error
	s.roles, err = safefs.OpenRoles(s.roots, s.owner)
	if err != nil {
		return err
	}
	s.stateDir, err = s.openRoot(s.roots.State())
	if err != nil {
		return err
	}
	// Before admission only safe topology and this exact stable inode are read.
	s.admission, err = s.stateDir.OpenLock("admission.lock", false)
	if err != nil || s.admission.Identity().Size != 0 {
		return errReadyObservation
	}
	s.lockID = s.admission.Identity()
	if err = s.admission.Try(s.lockMode); err != nil {
		return err
	}
	s.held = true
	if err = s.step("admission-held", func() error { return nil }); err != nil {
		return err
	}
	if err = s.step("root-record", func() error {
		s.graph.root, err = s.pin(s.stateDir, "root.json", initRecordLimit)
		if err != nil {
			return err
		}
		root, err := decodeRootRecord(s.graph.root)
		if err != nil || root.Status != "initialized" {
			return errReadyObservation
		}
		s.facts.instance = root.Instance
		return nil
	}); err != nil {
		return err
	}
	for _, name := range []string{"init.lock", "writer.lock"} {
		if err = s.step(name, func() error {
			_, err := s.pin(s.stateDir, name, 0)
			return err
		}); err != nil {
			return err
		}
	}
	if err = s.step("writer-record", func() error {
		s.graph.writer, err = s.pin(s.stateDir, "writer-owner.json", writerOwnerRecordLimit)
		return err
	}); err != nil {
		return err
	}
	for i, name := range []string{"candidates", "journals", "receipts"} {
		if err = s.step(name, func() error {
			s.controls[i], err = s.openDir(s.stateDir, name)
			if err != nil {
				return errReadyObservation
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err = s.step("data-owner", func() error {
		s.dataDir, err = s.openRoot(s.roots.Data())
		if err != nil {
			return err
		}
		s.graph.dataOwner, err = s.pin(s.dataDir, "owner.json", initRecordLimit)
		return err
	}); err != nil {
		return err
	}
	if err = s.step("cache-owner", func() error {
		cache, err := s.openRoot(s.roots.Cache())
		if err != nil {
			return err
		}
		instances, err := s.openDir(cache, "instances")
		if err != nil {
			return err
		}
		s.cacheDir, err = s.openDir(instances, s.facts.instance)
		if err != nil {
			return errReadyObservation
		}
		s.graph.cacheOwner, err = s.pin(s.cacheDir, "owner.json", initRecordLimit)
		return err
	}); err != nil {
		return err
	}
	if err = s.step("complete", s.validateGraph); err != nil {
		return err
	}
	if err = s.validateGraph(); err != nil {
		return err
	}
	return s.validate()
}

func (s *readyObservationState) openRoot(name string) (*safefs.Dir, error) {
	d, err := safefs.OpenRoot(name, s.owner)
	if err == nil {
		s.dirs = append(s.dirs, d)
	}
	return d, err
}

func (s *readyObservationState) openDir(parent *safefs.Dir, name string) (*safefs.Dir, error) {
	d, err := parent.OpenDir(name, false)
	if err == nil {
		s.dirs = append(s.dirs, d)
	}
	return d, err
}

func (s *readyObservationState) pin(parent *safefs.Dir, name string, limit int64) ([]byte, error) {
	f, data, err := parent.PinFile(name, limit)
	if err == nil {
		s.files = append(s.files, f)
	}
	return data, err
}

func (s *readyObservationState) validate() error {
	if s.closed || !s.held || s.ctx == nil || s.ctx.Err() != nil || s.roles.Validate() != nil ||
		s.admission.Identity() != s.lockID || s.admission.Try(s.lockMode) != nil {
		return errReadyObservation
	}
	for _, f := range s.files {
		if f.Revalidate() != nil {
			return errReadyObservation
		}
	}
	return s.roles.Validate()
}

func (s *readyObservationState) step(name string, action func() error) error {
	if err := s.validate(); err != nil {
		return err
	}
	if s.hook != nil {
		if err := s.hook("before:" + name); err != nil {
			return err
		}
		if err := s.validate(); err != nil {
			return err
		}
	}
	if err := action(); err != nil {
		return err
	}
	if s.hook != nil {
		if err := s.hook("after:" + name); err != nil {
			return err
		}
	}
	return s.validate()
}

func (s *readyObservationState) validateGraph() error {
	g := s.graph
	var err error
	for _, item := range []struct {
		dir   *safefs.Dir
		names *[]string
		limit int
	}{
		{s.stateDir, &g.state, len(readyStateNames)}, {s.dataDir, &g.data, 1}, {s.cacheDir, &g.cache, 1},
	} {
		*item.names, err = item.dir.Names(item.limit)
		if err != nil {
			return err
		}
	}
	for i, d := range s.controls {
		g.controls[i], err = d.Names(0)
		if err != nil {
			return err
		}
	}
	facts, ok := readReadyGraph(g)
	if !ok || facts.instance != s.facts.instance {
		return errReadyObservation
	}
	s.facts = facts
	return nil
}

func (o *readyObservation) facts() (readyFacts, error) {
	if o == nil || o.state == nil {
		return readyFacts{}, errReadyObservation
	}
	s := o.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validate() != nil || s.validateGraph() != nil || s.validate() != nil {
		return readyFacts{}, errors.Join(errReadyObservation, s.close())
	}
	return s.facts, nil
}

func (o *readyObservation) close() error {
	if o == nil || o.state == nil {
		return nil
	}
	s := o.state
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.close()
}

// Resource release only: never releases a published owner or removes evidence.
func (s *readyObservationState) close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.facts = readyFacts{}
	var err error
	for i := len(s.files) - 1; i >= 0; i-- {
		err = errors.Join(err, s.files[i].Close())
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		err = errors.Join(err, s.dirs[i].Close())
	}
	if s.roles != nil {
		err = errors.Join(err, s.roles.Close())
	}
	if s.admission != nil {
		err = errors.Join(err, s.admission.Close())
	}
	return err
}
