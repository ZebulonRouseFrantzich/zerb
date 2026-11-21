//go:build linux

package state

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"slices"
	"sync"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

const freshParentLimit = 65536

// Ordinary value copies share all physical resources and one release operation.
type Lease struct{ state *freshSession }

type freshFrontier struct {
	role            safefs.Role
	target, current string
	dir             *safefs.Dir
	missing         []string
}

type freshFile struct {
	dir   *safefs.Dir
	name  string
	file  *safefs.File
	data  []byte
	facts safefs.FileFacts
}

type freshBoundary func(string) error

type freshSession struct {
	mu                                           sync.Mutex
	ctx                                          context.Context
	random                                       io.Reader
	intent                                       Intent
	owner                                        uint32
	roots                                        paths.Roots
	roles                                        *safefs.Roles
	frontiers                                    [3]freshFrontier
	dirs                                         []*safefs.Dir
	files                                        []*freshFile
	replacements                                 []*safefs.Replacement
	process                                      *currentProcess
	graph                                        freshGraph
	root, writer, dataOwner, cacheOwner, witness *freshFile
	stateDirs                                    [3]*safefs.Dir
	instances, instance                          *safefs.Dir
	initLock, admission, writerLock              *safefs.Lock
	initHeld, admissionHeld, writerHeld          bool
	mode                                         LeaseMode
	primary                                      error
	closed, success                              bool
	hook                                         freshBoundary
	overlay                                      *freshReplacement
}

type freshReplacement struct {
	cap               *safefs.Replacement
	facts             safefs.ReplacementFacts
	record            replacementRecord
	previous          *freshFile
	staged            *freshFile
	linked, published bool
}

func freshInitialize(ctx context.Context, roots paths.Roots, random io.Reader, intent Intent) (Acquisition, error) {
	return freshInitializeWithBoundary(ctx, roots, random, intent, nil)
}

func freshInitializeWithBoundary(ctx context.Context, roots paths.Roots, random io.Reader, intent Intent, hook freshBoundary) (Acquisition, error) {
	s := &freshSession{ctx: ctx, roots: roots, random: random, intent: intent, owner: uint32(os.Geteuid()), hook: hook}
	l := &Lease{state: s}
	err := s.initialize()
	if err != nil {
		s.primary = err
	}
	snapshot := Snapshot{}
	if err == nil {
		snapshot.Classification = Ready
	}
	mode := s.mode
	if err != nil {
		mode = 0
	}
	return Acquisition{Lease: l, Snapshot: snapshot, Mode: mode, Admitted: err == nil}, err
}

func (l *Lease) Mode() LeaseMode {
	if l == nil || l.state == nil {
		return 0
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.primary != nil {
		return 0
	}
	return s.mode
}

func (l *Lease) Revalidate() error {
	if l == nil || l.state == nil {
		return errFreshInit
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.primary != nil || !s.success {
		return errFreshInit
	}
	if err := s.validate(); err != nil {
		s.primary = err
		return err
	}
	return nil
}

func (l *Lease) Release() error {
	if l == nil || l.state == nil {
		return nil
	}
	s := l.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	// An uncertain operation is close-only: it has no semantic cleanup authority.
	if s.primary == nil && s.success {
		if err := s.validate(); err != nil {
			s.primary = err
		} else if s.intent == IntentBootstrap {
			s.ctx = context.Background()
			data, err := encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: s.graph.instance, Generation: 2, PreviousGeneration: 1, PreviousToken: s.graph.writerToken})
			if err == nil {
				err = s.replace("writer-owner.json", data, "writer-release", 1, s.graph.writerToken, freshReleased)
			}
			if err != nil {
				s.primary = err
			}
		}
	}
	s.closed = true
	var cleanup error
	for _, lock := range []*safefs.Lock{s.writerLock, s.admission, s.initLock} {
		if lock != nil {
			cleanup = errors.Join(cleanup, lock.Close())
		}
	}
	for i := len(s.replacements) - 1; i >= 0; i-- {
		cleanup = errors.Join(cleanup, s.replacements[i].Close())
	}
	for i := len(s.files) - 1; i >= 0; i-- {
		cleanup = errors.Join(cleanup, s.files[i].file.Close())
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		cleanup = errors.Join(cleanup, s.dirs[i].Close())
	}
	if s.roles != nil {
		cleanup = errors.Join(cleanup, s.roles.Close())
	}
	if s.process != nil {
		cleanup = errors.Join(cleanup, s.process.close())
	}
	return errors.Join(s.primary, cleanup)
}

func (s *freshSession) ownDir(d *safefs.Dir) *safefs.Dir { s.dirs = append(s.dirs, d); return d }

func (s *freshSession) pin(d *safefs.Dir, name string, limit int64, want []byte, identity safefs.Identity) (*freshFile, error) {
	f, data, err := d.PinFile(name, limit)
	if err != nil {
		return nil, err
	}
	p := &freshFile{d, name, f, bytes.Clone(data), f.Facts()}
	s.files = append(s.files, p)
	if !bytes.Equal(data, want) || identity != (safefs.Identity{}) && p.facts.Identity != identity {
		return nil, errFreshInit
	}
	return p, nil
}

func (s *freshSession) mint() (string, error) {
	if s.random == nil {
		return "", errFreshInit
	}
	var value [16]byte
	if _, err := io.ReadFull(s.random, value[:]); err != nil {
		return "", errFreshInit
	}
	id := hex.EncodeToString(value[:])
	if slices.Contains(s.graph.issued, id) {
		return "", errFreshInit
	}
	s.graph.issued = append(s.graph.issued, id)
	return id, nil
}

func (s *freshSession) boundary(name string, action func() error) error {
	if s.ctx == nil {
		return errFreshInit
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
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
		return errors.Join(err, s.validate())
	}
	if s.hook != nil {
		if err := s.hook("after:" + name); err != nil {
			return err
		}
	}
	if err := s.validate(); err != nil {
		return err
	}
	return s.ctx.Err()
}

func (s *freshSession) initialize() error {
	for _, step := range []func() error{s.prepareInitialization, s.publishInitialState, s.publishNamespaces, s.handoff} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (s *freshSession) prepareInitialization() error {
	if s.ctx == nil || s.ctx.Err() != nil || s.random == nil || (s.intent != IntentProfileList && s.intent != IntentPlan && s.intent != IntentBootstrap) {
		return errFreshInit
	}
	var err error
	s.roles, err = safefs.OpenRoles(s.roots, s.owner)
	if err != nil {
		return err
	}
	for i, item := range []struct {
		role safefs.Role
		name string
	}{{safefs.RoleState, s.roots.State()}, {safefs.RoleData, s.roots.Data()}, {safefs.RoleCache, s.roots.Cache()}} {
		s.frontiers[i], err = s.discover(item.role, item.name)
		if err != nil {
			return err
		}
	}
	// ARCH-020's immutable witness requires a 0700 parent, even when Dir admits more.
	if d := s.product(0); d != nil {
		if !freshStateMode(d.Identity().Mode) {
			return errFreshInit
		}
		if _, err = d.Names(0); err != nil {
			return err
		}
	}
	if d := s.product(1); d != nil {
		if _, err = d.Names(0); err != nil {
			return err
		}
	}
	if err = s.validate(); err != nil {
		return err
	}
	if err = s.materialize(0); err != nil {
		return err
	}
	state := s.product(0)
	if err = s.syncAttachment(state, s.roots.State(), "state-root"); err != nil {
		return err
	}
	if err = s.boundary("init-lock-open", func() error {
		s.initLock, err = state.OpenLock("init.lock", true)
		if err != nil {
			return err
		}
		if s.initLock.Identity().Size != 0 {
			return errFreshInit
		}
		_, err = s.pin(state, "init.lock", 0, nil, s.initLock.Identity())
		if err != nil {
			return err
		}
		s.graph.prefix = freshInitLock
		return nil
	}); err != nil {
		return err
	}
	if err = s.boundary("init-lock-parent-sync", state.Sync); err != nil {
		return err
	}
	if err = s.boundary("init-lock-exclusive", func() error {
		if err := s.initLock.Try(safefs.LockExclusive); err != nil {
			return err
		}
		s.initHeld = true
		s.mode = InitExclusive
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *freshSession) publishInitialState() error {
	var err error
	state := s.product(0)
	if err = s.boundary("claim-identity", func() error {
		s.graph.instance, err = s.mint()
		if err != nil {
			return err
		}
		s.graph.initToken, err = s.mint()
		if err != nil {
			return err
		}
		s.process, err = captureCurrentProcess()
		if err != nil {
			return err
		}
		s.graph.process = s.process.identity
		return nil
	}); err != nil {
		return err
	}
	claim, err := encodeRootRecord(rootRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: s.graph.instance, Token: s.graph.initToken, Process: s.graph.process})
	if err != nil {
		return err
	}
	if err = s.publishRecord("root-claim", state, "root.json", claim, freshClaim, &s.root); err != nil {
		return err
	}
	for _, item := range []struct {
		name   string
		prefix freshPrefix
		slot   **safefs.Lock
	}{{"admission.lock", freshAdmission, &s.admission}, {"writer.lock", freshWriterLock, &s.writerLock}} {
		if err = s.boundary(item.name+"-publish", func() error {
			id, err := state.PublishNoReplace(item.name, nil, 0o600)
			if err != nil {
				return err
			}
			lock, err := state.OpenLock(item.name, false)
			if err != nil {
				return err
			}
			*item.slot = lock
			if lock.Identity() != id || id.Size != 0 {
				return errFreshInit
			}
			if _, err = s.pin(state, item.name, 0, nil, id); err != nil {
				return err
			}
			s.graph.prefix = item.prefix
			return nil
		}); err != nil {
			return err
		}
		if err = s.boundary(item.name+"-parent-sync", state.Sync); err != nil {
			return err
		}
	}
	owner, err := encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: s.graph.instance})
	if err != nil {
		return err
	}
	if err = s.publishRecord("writer-released-zero", state, "writer-owner.json", owner, freshWriterOwner, &s.writer); err != nil {
		return err
	}
	for i, item := range []struct {
		name   string
		prefix freshPrefix
	}{{"candidates", freshCandidates}, {"journals", freshJournals}, {"receipts", freshReceipts}} {
		if err = s.boundary(item.name+"-create", func() error {
			d, err := state.CreateDir(item.name)
			if err != nil {
				return err
			}
			s.stateDirs[i] = s.ownDir(d)
			s.graph.prefix = item.prefix
			return nil
		}); err != nil {
			return err
		}
		if err = s.syncAttachment(s.stateDirs[i], path.Join(s.roots.State(), item.name), item.name); err != nil {
			return err
		}
	}
	return nil
}

func (s *freshSession) publishNamespaces() error {
	var err error
	if err = s.materialize(1); err != nil {
		return err
	}
	if err = s.boundary("data-root-prefix", func() error { s.graph.prefix = freshDataRoot; return nil }); err != nil {
		return err
	}
	if err = s.syncAttachment(s.product(1), s.roots.Data(), "data-root"); err != nil {
		return err
	}
	dataOwner, err := encodeNamespaceOwner(namespaceOwner{Schema: 1, Domain: "zerb.data-owner", Instance: s.graph.instance})
	if err != nil {
		return err
	}
	if err = s.publishRecord("data-owner", s.product(1), "owner.json", dataOwner, freshDataOwner, &s.dataOwner); err != nil {
		return err
	}
	if err = s.materialize(2); err != nil {
		return err
	}
	if err = s.boundary("cache-root-prefix", func() error { s.graph.prefix = freshCacheRoot; return nil }); err != nil {
		return err
	}
	if err = s.syncAttachment(s.product(2), s.roots.Cache(), "cache-root"); err != nil {
		return err
	}
	if err = s.boundary("cache-instances-open", func() error {
		names, err := s.product(2).Names(freshParentLimit)
		if err != nil {
			return err
		}
		var d *safefs.Dir
		if slices.Contains(names, "instances") {
			d, err = s.product(2).OpenDir("instances", false)
		} else {
			d, err = s.product(2).CreateDir("instances")
		}
		if err != nil {
			return err
		}
		s.instances = s.ownDir(d)
		s.graph.prefix = freshInstances
		return nil
	}); err != nil {
		return err
	}
	if err = s.syncAttachment(s.instances, path.Join(s.roots.Cache(), "instances"), "cache-instances"); err != nil {
		return err
	}
	if err = s.boundary("cache-instance-create", func() error {
		d, err := s.instances.CreateDir(s.graph.instance)
		if err != nil {
			return err
		}
		s.instance = s.ownDir(d)
		s.graph.prefix = freshInstance
		return nil
	}); err != nil {
		return err
	}
	if err = s.syncAttachment(s.instance, path.Join(s.roots.Cache(), "instances", s.graph.instance), "cache-instance"); err != nil {
		return err
	}
	cacheOwner, err := encodeNamespaceOwner(namespaceOwner{Schema: 1, Domain: "zerb.cache-owner", Instance: s.graph.instance})
	if err != nil {
		return err
	}
	if err = s.publishRecord("cache-owner", s.instance, "owner.json", cacheOwner, freshCacheOwner, &s.cacheOwner); err != nil {
		return err
	}
	return nil
}

func (s *freshSession) handoff() error {
	var err error
	if err = s.boundary("target-admission", func() error {
		if err := s.admission.Try(safefs.LockExclusive); err != nil {
			return err
		}
		s.admissionHeld = true
		s.mode = AdmissionExclusive
		return nil
	}); err != nil {
		return err
	}
	if s.intent == IntentBootstrap {
		if err = s.boundary("target-writer", func() error {
			if err := s.writerLock.Try(safefs.LockExclusive); err != nil {
				return err
			}
			s.writerHeld = true
			s.mode = AdmissionWriterExclusive
			return nil
		}); err != nil {
			return err
		}
	}
	ready, err := encodeRootRecord(rootRecord{Schema: 1, Domain: "zerb.root", Status: "initialized", Instance: s.graph.instance})
	if err != nil {
		return err
	}
	if err = s.replace("root.json", ready, "initialization", 0, s.graph.initToken, freshReady); err != nil {
		return err
	}
	if s.intent == IntentBootstrap {
		if err = s.boundary("writer-token", func() error { s.graph.writerToken, err = s.mint(); return err }); err != nil {
			return err
		}
		held, err := encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "held", Instance: s.graph.instance, Generation: 1, Token: s.graph.writerToken, Process: s.graph.process})
		if err != nil {
			return err
		}
		if err = s.replace("writer-owner.json", held, "writer-acquire", 0, "", freshHeld); err != nil {
			return err
		}
	}
	if err = s.boundary("init-handoff", func() error {
		if err := s.initLock.Unlock(); err != nil {
			return err
		}
		s.initHeld = false
		return nil
	}); err != nil {
		return err
	}
	s.success = true
	return nil
}
