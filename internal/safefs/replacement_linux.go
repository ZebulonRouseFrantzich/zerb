//go:build linux

package safefs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type FileFacts struct {
	Identity            Identity
	ModifiedSeconds     int64
	ModifiedNanoseconds uint32
	ChangedSeconds      int64
	ChangedNanoseconds  uint32
	SHA256              string
}

type ReplacementFacts struct {
	Parent    Identity
	Target    string
	Temporary string
	Previous  FileFacts
	Staged    FileFacts
}

// Replacement value copies share one lifetime. Closing never removes names.
type Replacement struct{ state *replacementState }

type replacementState struct {
	mu              sync.Mutex
	chain           []edge
	owner           uint32
	previous        *pinnedFile
	proc            *procAccess
	reader          *os.File
	stage           *publication
	facts           ReplacementFacts
	linked          bool
	renamed         bool
	renameAttempted bool
	closed          bool
}

func replacementFacts(stamp fileStamp, hash string) FileFacts {
	return FileFacts{Identity: stamp.identity,
		ModifiedSeconds: stamp.modified.Sec, ModifiedNanoseconds: stamp.modified.Nsec,
		ChangedSeconds: stamp.changed.Sec, ChangedNanoseconds: stamp.changed.Nsec, SHA256: hash}
}

// StageReplacement requires an exact existing predecessor; it never upserts.
func (d *Dir) StageReplacement(target, temporary string, data []byte, mode uint32, previous Identity) (*Replacement, error) {
	if d == nil || d.state == nil || !validName(target) || !validName(temporary) || target == temporary ||
		!validPublicationMode(mode) || !validReadLimit(int64(len(data))) || previous == (Identity{}) {
		return nil, errDirectory
	}
	if d.state.gate != nil {
		return nil, errDirectory
	}
	ds := d.state
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if publicationChain(ds) != nil {
		return nil, errDirectory
	}
	chain, err := duplicateChain(ds.chain)
	if err != nil {
		return nil, errDirectory
	}
	s, err := replacementAt(chain, target, temporary, data, mode, previous)
	post := publicationChain(ds)
	if err != nil {
		return nil, errDirectory
	}
	if post != nil || s.validateAbsolute() != nil {
		return nil, s.refuse()
	}
	return &Replacement{state: s}, nil
}

// replacementAt takes ownership of the supplied ancestors, including on error.
// Private phases require the caller's serialization and absolute-root admission.
func replacementAt(chain []edge, target, temporary string, data []byte, mode uint32, previous Identity) (_ *replacementState, err error) {
	s := &replacementState{chain: chain}
	defer func() {
		if err != nil {
			if s.closeResources() != nil {
				err = errDirectory
			}
		}
	}()
	if len(chain) == 0 || !validName(target) || !validName(temporary) || target == temporary ||
		!validPublicationMode(mode) || !validReadLimit(int64(len(data))) || previous == (Identity{}) {
		return nil, errDirectory
	}
	parent := chain[len(chain)-1]
	s.owner = parent.id.Owner
	s.facts = ReplacementFacts{Parent: parent.id, Target: target, Temporary: temporary}
	if s.validateMetadata() != nil {
		return nil, errDirectory
	}
	s.previous, err = pinFile(parent, target, previous.Size)
	if err != nil || s.previous.before.identity != previous || s.validateMetadata() != nil {
		return nil, errDirectory
	}
	// Full caller identity comparison precedes readable reopening or content I/O.
	s.proc, err = openProcAccess()
	if err != nil {
		return nil, errDirectory
	}
	s.reader, err = s.proc.reopen(s.previous)
	if err != nil || s.validateMetadata() != nil {
		return nil, errDirectory
	}
	hash, err := s.hash(int(s.reader.Fd()), s.previous.before)
	if err != nil {
		return nil, errDirectory
	}
	s.facts.Previous = replacementFacts(s.previous.before, hash)
	s.stage, err = openPublication(parent, mode)
	if err != nil || s.validateMetadata() != nil {
		return nil, errDirectory
	}
	if s.stage.write(data) != nil || s.validateMetadata() != nil || s.stage.syncFile() != nil || s.validateMetadata() != nil {
		return nil, errDirectory
	}
	hash, err = s.hash(s.stage.fd, s.stage.stamp)
	want := sha256.Sum256(data)
	if err != nil || hash != hex.EncodeToString(want[:]) {
		return nil, errDirectory
	}
	s.facts.Staged = replacementFacts(s.stage.stamp, hash)
	if s.validateContents() != nil {
		return nil, errDirectory
	}
	return s, nil
}

func (s *replacementState) validateAbsolute() error {
	if s.closed || validateChain(s.chain, s.owner) != nil {
		return errDirectory
	}
	return nil
}

func (s *replacementState) validateMetadata() error {
	if s.closed || validateEdges(s.chain, s.owner) != nil || s.chain[len(s.chain)-1].id.Owner != s.owner {
		return errDirectory
	}
	if s.proc != nil && s.proc.validate() != nil {
		return errDirectory
	}
	if p := s.previous; p != nil {
		if p.closed || publicationParent(p.parent) != nil {
			return errDirectory
		}
		now, err := observeFile(p.fd, "")
		if err != nil || now != p.before {
			return errDirectory
		}
		if !s.renamed && p.validate() != nil {
			return errDirectory
		}
		if s.renamed && p.before.identity.Links != 0 {
			return errDirectory
		}
		if s.reader != nil {
			readable, err := observeFile(int(s.reader.Fd()), "")
			if err != nil || readable != p.before {
				return errDirectory
			}
		}
	}
	if s.stage != nil && s.stage.validate() != nil {
		return errDirectory
	}
	return nil
}

// Positional reads bound scratch space and preserve the retained reader offset.
func (s *replacementState) hash(fd int, stamp fileStamp) (string, error) {
	if !validReadLimit(stamp.identity.Size) || s.validateMetadata() != nil {
		return "", errDirectory
	}
	h := sha256.New()
	var buffer [32 << 10]byte
	for offset := int64(0); ; {
		if s.validateMetadata() != nil {
			return "", errDirectory
		}
		want := stamp.identity.Size - offset
		if want > int64(len(buffer)) {
			want = int64(len(buffer))
		}
		if want == 0 {
			want = 1
		}
		n, err := unix.Pread(fd, buffer[:int(want)], offset)
		now, observation := observeFile(fd, "")
		post := s.validateMetadata()
		if err != nil || observation != nil || now != stamp || post != nil || n < 0 || n > int(want) {
			return "", errDirectory
		}
		if offset == stamp.identity.Size {
			if n != 0 {
				return "", errDirectory
			}
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if n == 0 {
			return "", errDirectory
		}
		if _, err := h.Write(buffer[:n]); err != nil {
			return "", errDirectory
		}
		offset += int64(n)
	}
}

func (s *replacementState) validateContents() error {
	if s.validateMetadata() != nil || s.reader == nil || s.stage == nil {
		return errDirectory
	}
	previous, err := s.hash(int(s.reader.Fd()), s.previous.before)
	if err != nil || previous != s.facts.Previous.SHA256 {
		return errDirectory
	}
	staged, err := s.hash(s.stage.fd, s.stage.stamp)
	if err != nil || staged != s.facts.Staged.SHA256 {
		return errDirectory
	}
	return s.validateMetadata()
}

func (r *Replacement) Facts() ReplacementFacts {
	if r == nil || r.state == nil {
		return ReplacementFacts{}
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ReplacementFacts{}
	}
	return s.facts
}

func (s *replacementState) verifyStage() error {
	if s.validateContents() != nil || !s.stage.parentSynced || s.stage.pin != nil {
		return errDirectory
	}
	var err error
	s.stage.pin, err = pinFile(s.stage.parent, s.stage.name, s.stage.stamp.identity.Size)
	if err != nil || s.stage.pin.before != s.stage.stamp || s.validateContents() != nil {
		return errDirectory
	}
	return nil
}

func (s *replacementState) link() (err error) {
	defer func() {
		if err != nil {
			err = s.refuse()
		}
	}()
	if s.closed || s.linked {
		return errDirectory
	}
	s.linked = true // Every call is one-shot, including a precondition refusal.
	if s.validateContents() != nil || s.stage.link(s.facts.Temporary) != nil || s.validateContents() != nil ||
		s.stage.syncParent() != nil || s.validateContents() != nil || s.verifyStage() != nil {
		return errDirectory
	}
	return nil
}

// Link exposes only the complete, synced stage and never replaces an entry.
func (r *Replacement) Link() error {
	if r == nil || r.state == nil {
		return errDirectory
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validateAbsolute() != nil || s.link() != nil || s.validateAbsolute() != nil {
		return s.refuse()
	}
	return nil
}

// rename performs one ordinary rename, not an inode compare-and-swap.
func (s *replacementState) rename() error {
	if !s.linked || s.renamed || s.validateContents() != nil || s.stage.pin == nil {
		return errDirectory
	}
	return s.renameAttempt()
}

// The caller holds serialization and validated both operands immediately before
// this boundary. Ordinary rename cannot make those observations an inode CAS.
func (s *replacementState) renameAttempt() error {
	p := s.stage
	s.renameAttempted = true
	err := unix.Renameat(p.parent.fd, s.facts.Temporary, p.parent.fd, s.facts.Target)
	staged, stageErr := observeFile(p.fd, "")
	previous, previousErr := observeFile(s.previous.fd, "")
	parentErr := publicationParent(p.parent)
	if err != nil || stageErr != nil || previousErr != nil || parentErr != nil {
		return errDirectory
	}
	expected := s.previous.before.identity
	expected.Links = 0
	if staged.identity != p.stamp.identity || staged.modified != p.stamp.modified ||
		previous.identity != expected || previous.modified != s.previous.before.modified {
		return errDirectory
	}
	// Only rename-induced ctime and the predecessor's one-to-zero links change.
	s.renamed, p.stamp, s.previous.before = true, staged, previous
	p.name, p.parentSynced = s.facts.Target, false
	oldPin := p.pin
	p.pin = nil
	closeErr := oldPin.close()
	_, absence := observeAt(p.parent.fd, s.facts.Temporary)
	if closeErr != nil || !errors.Is(absence, unix.ENOENT) || s.validateContents() != nil {
		return errDirectory
	}
	return nil
}

// publish is terminal, including when called before Link or after a failure.
func (s *replacementState) publish() (Identity, error) {
	id, err := s.publishPhases()
	return s.finish(id, err)
}

func (s *replacementState) publishPhases() (Identity, error) {
	if s.closed || s.rename() != nil || s.stage.syncParent() != nil || s.validateContents() != nil || s.verifyStage() != nil {
		return Identity{}, errDirectory
	}
	return s.stage.stamp.identity, nil
}

func (s *replacementState) finish(id Identity, primary error) (Identity, error) {
	id, primary = s.finishFiles(id, primary)
	cleanup := s.closeAncestors()
	if primary != nil || cleanup != nil {
		return Identity{}, s.failure()
	}
	return id, nil
}

func (s *replacementState) finishFiles(id Identity, primary error) (Identity, error) {
	var validation error
	if primary == nil {
		validation = s.validateContents()
	}
	cleanup := s.closeFiles()
	if primary != nil || validation != nil || cleanup != nil {
		return Identity{}, s.failure()
	}
	return id, nil
}

// Publish consumes the capability even when no rename can be attempted.
func (r *Replacement) Publish() (Identity, error) {
	if r == nil || r.state == nil {
		return Identity{}, errDirectory
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.validateAbsolute() != nil {
		return Identity{}, s.refuse()
	}
	id, err := s.publishPhases()
	post := s.validateAbsolute()
	if post != nil {
		err = errDirectory
	}
	id, err = s.finishFiles(id, err)
	post = validateChain(s.chain, s.owner)
	cleanup := s.closeAncestors()
	if err != nil || post != nil || cleanup != nil {
		return Identity{}, s.failure()
	}
	return id, nil
}

func (s *replacementState) failure() error {
	if s.renameAttempted {
		return errPublicationUncertain
	}
	return errDirectory
}

func (s *replacementState) refuse() error {
	if s.closeResources() != nil {
		return s.failure()
	}
	return s.failure()
}

func (s *replacementState) closeResources() error {
	fileErr := s.closeFiles()
	ancestorErr := s.closeAncestors()
	if fileErr != nil || ancestorErr != nil {
		return errDirectory
	}
	return nil
}

func (s *replacementState) closeFiles() error {
	if s.closed {
		return nil
	}
	s.closed, s.facts = true, ReplacementFacts{}
	reader := s.reader
	beforeChain := validateEdges(s.chain, s.owner)
	before := s.validateOwnedFiles()
	s.reader = nil
	var readerErr error
	if reader != nil {
		readerErr = reader.Close()
	}
	stageErr := s.stage.close()
	after := s.validatePrevious()
	previousErr := s.previous.close()
	procErr := s.proc.close()
	var linkedErr error
	if s.stage != nil && s.stage.linked {
		linkedErr = s.stage.validateLinked()
	}
	afterChain := validateEdges(s.chain, s.owner)
	if errors.Join(beforeChain, before, readerErr, stageErr, after, previousErr, procErr, linkedErr, afterChain) != nil {
		return errDirectory
	}
	return nil
}

func (s *replacementState) validatePrevious() error {
	p := s.previous
	if p == nil {
		return nil
	}
	stamp, err := observeFile(p.fd, "")
	if err != nil || stamp != p.before || publicationParent(p.parent) != nil {
		return errDirectory
	}
	if !s.renamed {
		return p.validate()
	}
	return nil
}

func (s *replacementState) validateOwnedFiles() error {
	if s.validatePrevious() != nil || s.stage != nil && s.stage.validate() != nil || s.proc != nil && s.proc.validate() != nil {
		return errDirectory
	}
	if s.reader != nil {
		stamp, err := observeFile(int(s.reader.Fd()), "")
		if err != nil || stamp != s.previous.before {
			return errDirectory
		}
	}
	return nil
}

func (s *replacementState) closeAncestors() error {
	if len(s.chain) == 0 {
		return nil
	}
	chain := s.chain
	s.chain = nil
	before := validateEdges(chain, s.owner)
	cleanup := closeChain(chain)
	if before != nil || cleanup != nil {
		return errDirectory
	}
	return nil
}

func (r *Replacement) Close() error {
	if r == nil || r.state == nil {
		return nil
	}
	s := r.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed && len(s.chain) == 0 {
		return nil
	}
	files := s.closeFiles()
	post := validateChain(s.chain, s.owner)
	ancestors := s.closeAncestors()
	if files != nil || post != nil || ancestors != nil {
		return errDirectory
	}
	return nil
}
