//go:build linux

package state

import (
	"bytes"
	"errors"
	"path"
	"slices"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func (s *freshSession) product(index int) *safefs.Dir {
	f := &s.frontiers[index]
	if len(f.missing) != 0 {
		return nil
	}
	return f.dir
}

// Failure to open never proves absence: names on the longest admitted parent do.
func (s *freshSession) discover(role safefs.Role, target string) (freshFrontier, error) {
	for current := target; ; current = path.Dir(current) {
		d, err := safefs.OpenRoot(current, s.owner)
		if err == nil {
			s.ownDir(d)
			f := freshFrontier{role: role, target: target, current: current, dir: d}
			if current != target {
				suffix := strings.TrimPrefix(target, strings.TrimSuffix(current, "/")+"/")
				f.missing = strings.Split(suffix, "/")
				names, err := d.Names(freshParentLimit)
				if err != nil || slices.Contains(names, f.missing[0]) {
					return freshFrontier{}, errFreshInit
				}
			}
			return f, nil
		}
		if current == "/" {
			return freshFrontier{}, errFreshInit
		}
	}
}

func (s *freshSession) materialize(index int) error {
	f := &s.frontiers[index]
	if err := s.syncPrefixAttachments(f.dir, f.current); err != nil {
		return err
	}
	for len(f.missing) != 0 {
		parent, parentPath, name := f.dir, f.current, f.missing[0]
		childPath := path.Join(parentPath, name)
		var child *safefs.Dir
		if err := s.boundary("prefix-create", func() error {
			var err error
			child, err = parent.CreateDir(name)
			if err != nil {
				return err
			}
			s.ownDir(child)
			if err = s.roles.Add(f.role, child); err != nil {
				return err
			}
			// Roles.Add advances every reservation sharing this exact next edge.
			for i := range s.frontiers {
				other := &s.frontiers[i]
				if other.current == parentPath && len(other.missing) > 0 && other.missing[0] == name {
					other.current, other.dir, other.missing = childPath, child, other.missing[1:]
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if err := s.syncAttachment(child, childPath, "prefix"); err != nil {
			return err
		}
	}
	return nil
}

func (s *freshSession) syncPrefixAttachments(d *safefs.Dir, current string) error {
	for {
		if err := s.syncAttachment(d, current, "prefix-attachment"); err != nil {
			return err
		}
		if current == "/" {
			return nil
		}
		parent, err := safefs.OpenRoot(path.Dir(current), s.owner)
		// syncAttachment already synchronized the exact root-owned structural parent.
		if err != nil {
			if s.owner != 0 {
				structural, proofErr := safefs.OpenRoot(path.Dir(current), 0)
				if proofErr != nil {
					return proofErr
				}
				s.ownDir(structural)
				return structural.Close()
			}
			return err
		}
		d = s.ownDir(parent)
		current = path.Dir(current)
	}
}

// A root-owned structural parent is admitted solely for Sync and Close.
func (s *freshSession) syncAttachment(child *safefs.Dir, childPath, label string) error {
	if child == nil || childPath == "" {
		return errFreshInit
	}
	var parent *safefs.Dir
	if childPath != "/" {
		if err := s.boundary(label+"-parent-open", func() error {
			var err error
			parent, err = safefs.OpenRoot(path.Dir(childPath), s.owner)
			if err != nil && s.owner != 0 {
				parent, err = safefs.OpenRoot(path.Dir(childPath), 0)
			}
			if err != nil {
				return err
			}
			s.ownDir(parent)
			return nil
		}); err != nil {
			return err
		}
	}
	if err := s.boundary(label+"-child-sync", child.Sync); err != nil {
		return err
	}
	if parent != nil {
		if err := s.boundary(label+"-parent-sync", parent.Sync); err != nil {
			return err
		}
	}
	if err := s.boundary(label+"-reopen", func() error {
		reopened, err := safefs.OpenRoot(childPath, s.owner)
		if err != nil {
			return err
		}
		s.ownDir(reopened)
		a, b := child.Identity(), reopened.Identity()
		if a.Device != b.Device || a.Inode != b.Inode || a.MountID != b.MountID || a.Owner != b.Owner || a.Mode != b.Mode {
			return errFreshInit
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *freshSession) publishRecord(label string, d *safefs.Dir, name string, data []byte, next freshPrefix, slot **freshFile) error {
	return s.boundary(label+"-publish", func() error {
		id, err := d.PublishNoReplace(name, data, 0o600)
		if err != nil {
			return err
		}
		p, err := s.pin(d, name, initRecordLimit, data, id)
		if err != nil {
			return err
		}
		*slot = p
		s.graph.prefix = next
		return nil
	})
}

func (s *freshSession) validate() error {
	if s.roles == nil {
		return errFreshInit
	}
	if err := s.roles.Validate(); err != nil {
		return err
	}
	for i := range s.frontiers {
		f := &s.frontiers[i]
		if f.dir == nil {
			return errFreshInit
		}
		if len(f.missing) != 0 {
			names, err := f.dir.Names(freshParentLimit)
			if err != nil || slices.Contains(names, f.missing[0]) {
				return errFreshInit
			}
		}
	}
	if s.process != nil {
		if s.process.identity != s.graph.process {
			return errFreshInit
		}
		if err := s.process.revalidate(); err != nil {
			return err
		}
	}
	if err := s.validateFilesAndLocks(); err != nil {
		return err
	}
	g := s.graph
	if s.overlay != nil {
		record := s.overlay.record
		g.replacement = &record
	}
	var err error
	if d := s.product(0); d != nil {
		g.state, err = d.Names(len(freshNames(g.prefix, g.witness, g.temporary)))
		if err != nil {
			return err
		}
	}
	if d := s.product(1); d != nil {
		limit := 0
		if g.prefix >= freshDataOwner {
			limit = 1
		}
		g.data, err = d.Names(limit)
		if err != nil {
			return err
		}
	}
	if d := s.product(2); d != nil {
		if _, err = d.Names(freshParentLimit); err != nil {
			return err
		}
	}
	for _, d := range s.stateDirs {
		if d != nil {
			if _, err = d.Names(0); err != nil {
				return err
			}
		}
	}
	if s.instances != nil {
		names, err := s.instances.Names(freshParentLimit)
		if err != nil {
			return err
		}
		if s.instance == nil && slices.Contains(names, g.instance) {
			return errFreshInit
		}
	}
	if s.instance != nil {
		limit := 0
		if g.prefix >= freshCacheOwner {
			limit = 1
		}
		g.cache, err = s.instance.Names(limit)
		if err != nil {
			return err
		}
	}
	if s.root != nil {
		g.root = s.root.data
	}
	if s.writer != nil {
		g.writer = s.writer.data
	}
	if s.dataOwner != nil {
		g.dataOwner = s.dataOwner.data
	}
	if s.cacheOwner != nil {
		g.cacheOwner = s.cacheOwner.data
	}
	if !validFreshGraph(g) {
		return errFreshInit
	}
	if g.prefix >= freshClaim && !s.initHeld && !s.success && !s.admissionHeld {
		return errFreshInit
	}
	if g.prefix >= freshReady && (!s.admissionHeld || s.intent == IntentBootstrap && !s.writerHeld) {
		return errFreshInit
	}
	if s.overlay != nil {
		if err := s.validateOverlay(); err != nil {
			return err
		}
	}
	return s.roles.Validate()
}

func (s *freshSession) replacementAuthority(kind string, generation uint64, token string) bool {
	if !s.admissionHeld || s.process == nil || s.graph.prefix < freshCacheOwner {
		return false
	}
	switch kind {
	case "initialization":
		return s.initHeld && generation == 0 && token == s.graph.initToken && s.graph.prefix == freshCacheOwner
	case "writer-acquire":
		return s.writerHeld && s.graph.prefix == freshReady && generation == 0 && token == ""
	case "writer-release":
		return s.writerHeld && s.graph.prefix == freshHeld && generation == 1 && token == s.graph.writerToken
	}
	return false
}

func (s *freshSession) replace(target string, data []byte, kind string, generation uint64, token string, next freshPrefix) error {
	if !s.replacementAuthority(kind, generation, token) {
		return errFreshInit
	}
	previous := s.root
	if target == "writer-owner.json" {
		previous = s.writer
	}
	var id string
	if err := s.boundary(kind+"-stage", func() error {
		var err error
		id, err = s.mint()
		if err != nil {
			return err
		}
		cap, err := s.product(0).StageReplacement(target, ".replace-"+id, data, 0o600, previous.facts.Identity)
		if err != nil {
			return err
		}
		s.replacements = append(s.replacements, cap)
		facts := cap.Facts()
		if facts.Previous != previous.facts {
			return errFreshInit
		}
		record := replacementRecord{Schema: 1, Domain: "zerb.replace-witness", Instance: s.graph.instance, ID: id, Publisher: s.graph.process, Authority: replacementAuthority{Kind: kind, Generation: generation, Token: token}, Files: replacementWireFiles(facts)}
		if _, err = encodeReplacementRecord(record); err != nil {
			return err
		}
		s.overlay = &freshReplacement{cap: cap, facts: facts, record: record, previous: previous}
		return nil
	}); err != nil {
		return err
	}
	witnessBytes, err := encodeReplacementRecord(s.overlay.record)
	if err != nil {
		return err
	}
	if err = s.boundary(kind+"-witness", func() error {
		identity, err := s.product(0).PublishNoReplace("replace-witness.json", witnessBytes, 0o600)
		if err != nil {
			return err
		}
		s.witness, err = s.pin(s.product(0), "replace-witness.json", replacementRecordLimit, witnessBytes, identity)
		if err != nil {
			return err
		}
		s.graph.witness = "replace-witness.json"
		return nil
	}); err != nil {
		return err
	}
	if err = s.boundary(kind+"-link", func() error {
		if err := s.overlay.cap.Link(); err != nil {
			return err
		}
		p, err := s.pin(s.product(0), s.overlay.facts.Temporary, initRecordLimit, data, safefs.Identity{})
		if err != nil {
			return err
		}
		if !replacementObservedStage(p.facts, s.overlay.record.Files.Staged) {
			return errFreshInit
		}
		s.overlay.staged = p
		s.overlay.linked = true
		s.graph.temporary = s.overlay.facts.Temporary
		return nil
	}); err != nil {
		return err
	}
	if err = s.boundary(kind+"-publish", func() error {
		// Full predecessor facts and live session are bound before the single rename.
		if previous.facts != s.overlay.facts.Previous {
			return errFreshInit
		}
		identity, err := s.overlay.cap.Publish()
		if err != nil {
			return err
		}
		s.overlay.published = true
		p, err := s.pin(s.product(0), target, initRecordLimit, data, identity)
		if err != nil {
			return err
		}
		if !replacementObservedStage(p.facts, s.overlay.record.Files.Staged) {
			return errFreshInit
		}
		if target == "root.json" {
			s.root = p
		} else {
			s.writer = p
		}
		s.graph.prefix = next
		s.graph.temporary = ""
		// Expected rename invalidates the old name edge, so close without Revalidate.
		closeErr := errors.Join(previous.file.Close(), s.overlay.staged.file.Close())
		previous.facts, s.overlay.staged.facts = safefs.FileFacts{}, safefs.FileFacts{}
		return closeErr
	}); err != nil {
		return err
	}
	if err = s.boundary(kind+"-retire-witness", func() error {
		if !s.overlay.published || s.witness == nil {
			return errFreshInit
		}
		if err := s.witness.file.Remove(); err != nil {
			return err
		}
		s.witness.facts = safefs.FileFacts{}
		s.witness = nil
		s.graph.witness = ""
		s.overlay = nil
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *freshSession) validateOverlay() error {
	o := s.overlay
	if o.record.Publisher != s.graph.process || o.record.Instance != s.graph.instance || !slices.Contains(s.graph.issued, o.record.ID) ||
		o.facts.Previous != o.previous.facts && !o.published || o.record.Files != replacementWireFiles(o.facts) {
		return errFreshInit
	}
	if s.witness == nil {
		if o.linked || o.published {
			return errFreshInit
		}
		return nil
	}
	record, err := decodeReplacementRecord(s.witness.data)
	if err != nil || record != o.record {
		return errFreshInit
	}
	target := o.previous.facts
	if o.published {
		if o.facts.Target == "root.json" {
			target = s.root.facts
		} else {
			target = s.writer.facts
		}
	}
	observed := replacementObservations{parentState: replacementPresent, parent: s.product(0).Identity(), target: replacementFileObservation{replacementPresent, target}, temporary: replacementFileObservation{state: replacementAbsent}}
	if o.linked && !o.published {
		observed.temporary = replacementFileObservation{replacementPresent, o.staged.facts}
	}
	expected := replacementExpectation{instance: s.graph.instance, id: o.record.ID, publisher: s.graph.process, authority: o.record.Authority, files: o.facts}
	want := retireRecord
	if o.published {
		want = verifyThenRetire
	} else if o.linked {
		want = removeTempThenRecord
	}
	if recognizeReplacementPrefix(&record, expected, observed) != want {
		return errFreshInit
	}
	// Shape is insufficient: the original live lease conjunction remains required.
	if !s.admissionHeld || o.record.Authority.Kind == "initialization" && !s.initHeld || o.record.Authority.Kind != "initialization" && !s.writerHeld {
		return errFreshInit
	}
	return nil
}

func (s *freshSession) validateFilesAndLocks() error {
	for _, p := range s.files {
		// A verified replacement consumes the old pathname edge; its pin is close-only.
		if p.file == nil {
			continue
		}
		if s.overlay != nil && (p == s.overlay.previous && s.overlay.published || p == s.overlay.staged && s.overlay.published) {
			continue
		}
		if p.facts == (safefs.FileFacts{}) {
			continue
		}
		if err := p.file.Revalidate(); err != nil {
			return err
		}
		if p.file.Facts() != p.facts {
			return errFreshInit
		}
		data, id, err := p.dir.ReadFile(p.name, int64(len(p.data)))
		if err != nil || id != p.facts.Identity || !bytes.Equal(data, p.data) {
			return errFreshInit
		}
	}
	for _, lock := range []struct {
		l    *safefs.Lock
		held bool
	}{{s.initLock, s.initHeld}, {s.admission, s.admissionHeld}, {s.writerLock, s.writerHeld}} {
		if lock.l == nil {
			continue
		}
		if lock.l.Identity().Size != 0 || lock.l.Identity().Inode == 0 {
			return errFreshInit
		}
		if lock.held {
			if err := lock.l.Try(safefs.LockExclusive); err != nil {
				return err
			}
		}
	}
	return nil
}
