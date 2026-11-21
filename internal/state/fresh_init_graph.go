package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// These are the ARCH-030 values; this vertical does not expose Store routing.
type Classification uint8

const (
	Clean Classification = iota + 1
	Foreign
	Initializing
	BadUnsupported
	BadCorrupt
	Ready
	Stable
	Transition
	Cleanup
)

type LeaseMode uint8

const (
	DiagnosticSnapshot LeaseMode = iota + 1
	InitExclusive
	AdmissionShared
	AdmissionExclusive
	AdmissionWriterExclusive
)

type Intent uint8

const (
	IntentProfileList Intent = iota + 1
	IntentPlan
	IntentDoctor
	IntentRecoverResume
	IntentRecoverRepair
	IntentBootstrap
	IntentApply
	IntentExec
	IntentShell
)

type Phase string

const (
	PhasePrepared       Phase = "prepared"
	PhaseToolInstall    Phase = "mutating-tool-install"
	PhaseHomeApply      Phase = "mutating-home-apply"
	PhasePendingReceipt Phase = "pending-receipt"
	PhaseCleanup        Phase = "cleanup"
)

type Snapshot struct {
	Classification Classification
	Current        receipt.Digest
	Candidate      receipt.Digest
	Receipt        receipt.Digest
	Phase          Phase
}

type Acquisition struct {
	Lease    *Lease
	Snapshot Snapshot
	Mode     LeaseMode
	Admitted bool
}

var errFreshInit = errors.New("fresh initialization refused")

func freshStateMode(mode uint32) bool { return mode == 0o040700 }

type freshPrefix uint8

const (
	freshEmpty freshPrefix = iota
	freshInitLock
	freshClaim
	freshAdmission
	freshWriterLock
	freshWriterOwner
	freshCandidates
	freshJournals
	freshReceipts
	freshDataRoot
	freshDataOwner
	freshCacheRoot
	freshInstances
	freshInstance
	freshCacheOwner
	freshReady
	freshHeld
	freshReleased
)

var freshStateEntries = []struct {
	name string
	from freshPrefix
}{
	{"init.lock", freshInitLock}, {"root.json", freshClaim},
	{"admission.lock", freshAdmission}, {"writer.lock", freshWriterLock},
	{"writer-owner.json", freshWriterOwner}, {"candidates", freshCandidates},
	{"journals", freshJournals}, {"receipts", freshReceipts},
}

type freshGraph struct {
	prefix                              freshPrefix
	state, data, cache                  []string
	root, writer, dataOwner, cacheOwner []byte
	instance, initToken, writerToken    string
	process                             processIdentity
	issued                              []string
	witness, temporary                  string
	replacement                         *replacementRecord
}

func freshNames(prefix freshPrefix, witness, temporary string) []string {
	var names []string
	for _, entry := range freshStateEntries {
		if prefix >= entry.from {
			names = append(names, entry.name)
		}
	}
	if witness != "" {
		names = append(names, witness)
	}
	if temporary != "" {
		names = append(names, temporary)
	}
	sort.Strings(names)
	return names
}

// ARCH-017/024: this exact prefix check is shared by live I/O and pure tests.
func validFreshGraph(g freshGraph) bool {
	if g.prefix > freshReleased || !slices.Equal(g.state, freshNames(g.prefix, g.witness, g.temporary)) {
		return false
	}
	if (g.witness != "" && g.witness != "replace-witness.json") ||
		(g.temporary != "" && (g.witness == "" || len(g.temporary) != 41 || !strings.HasPrefix(g.temporary, ".replace-") || !replacementHex(g.temporary[len(".replace-"):], 32))) {
		return false
	}
	for i, value := range g.issued {
		if !replacementHex(value, 32) || slices.Contains(g.issued[:i], value) {
			return false
		}
	}
	if g.prefix < freshClaim {
		return g.witness == "" && g.temporary == "" && g.replacement == nil && len(g.root) == 0 && len(g.writer) == 0 && len(g.data) == 0 && len(g.cache) == 0 && len(g.dataOwner) == 0 && len(g.cacheOwner) == 0
	}
	if !replacementHex(g.instance, 32) || !replacementHex(g.initToken, 32) || g.instance == g.initToken ||
		!slices.Contains(g.issued, g.instance) || !slices.Contains(g.issued, g.initToken) ||
		g.process.PID <= 0 || g.process.Start == 0 || g.process.PIDNamespace == 0 {
		return false
	}
	if len(g.issued) < 2 || g.issued[0] != g.instance || g.issued[1] != g.initToken {
		return false
	}
	return validFreshRecords(g)
}

func validFreshRecords(g freshGraph) bool {
	root := rootRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: g.instance, Token: g.initToken, Process: g.process}
	if g.prefix >= freshReady {
		root.Status, root.Token, root.Process = "initialized", "", processIdentity{}
	}
	rootBytes, err := encodeRootRecord(root)
	if err != nil || !bytes.Equal(g.root, rootBytes) {
		return false
	}
	if g.prefix >= freshWriterOwner {
		writer := writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: g.instance}
		if g.prefix >= freshHeld {
			if !replacementHex(g.writerToken, 32) || g.writerToken == g.instance || g.writerToken == g.initToken || !slices.Contains(g.issued, g.writerToken) {
				return false
			}
			if len(g.issued) < 4 || g.issued[3] != g.writerToken {
				return false
			}
			if g.prefix == freshHeld {
				writer.Status, writer.Generation, writer.Token, writer.Process = "held", 1, g.writerToken, g.process
			} else {
				writer.Generation, writer.PreviousGeneration, writer.PreviousToken = 2, 1, g.writerToken
			}
		}
		data, err := encodeWriterOwner(writer)
		if err != nil || !bytes.Equal(g.writer, data) {
			return false
		}
	} else if len(g.writer) != 0 {
		return false
	}
	for _, owner := range []struct {
		present         bool
		names, expected []string
		data            []byte
		domain          string
	}{
		{g.prefix >= freshDataOwner, g.data, []string{"owner.json"}, g.dataOwner, "zerb.data-owner"},
		{g.prefix >= freshCacheOwner, g.cache, []string{"owner.json"}, g.cacheOwner, "zerb.cache-owner"},
	} {
		if !owner.present {
			if len(owner.names) != 0 || len(owner.data) != 0 {
				return false
			}
			continue
		}
		want, err := encodeNamespaceOwner(namespaceOwner{Schema: 1, Domain: owner.domain, Instance: g.instance})
		if err != nil || !slices.Equal(owner.names, owner.expected) || !bytes.Equal(want, owner.data) {
			return false
		}
	}
	return validFreshReplacement(g)
}

func validFreshReplacement(g freshGraph) bool {
	r := g.replacement
	if r == nil {
		return g.witness == "" && g.temporary == ""
	}
	if !validReplacementRecord(*r) || r.Instance != g.instance || r.Publisher != g.process || r.ID == g.initToken || r.ID == g.writerToken || !slices.Contains(g.issued, r.ID) {
		return false
	}
	if g.temporary != "" && g.temporary != r.Files.Temporary {
		return false
	}
	var old, new []byte
	var err error
	successor := false
	issuedIndex := 2
	switch r.Authority.Kind {
	case "initialization":
		successor = g.prefix == freshReady
		if g.prefix != freshCacheOwner && g.prefix != freshReady || r.Authority.Generation != 0 || r.Authority.Token != g.initToken {
			return false
		}
		old, err = encodeRootRecord(rootRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: g.instance, Token: g.initToken, Process: g.process})
		new, _ = encodeRootRecord(rootRecord{Schema: 1, Domain: "zerb.root", Status: "initialized", Instance: g.instance})
	case "writer-acquire":
		issuedIndex = 4
		successor = g.prefix == freshHeld
		if g.prefix != freshReady && g.prefix != freshHeld || r.Authority.Generation != 0 || r.Authority.Token != "" {
			return false
		}
		old, err = encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: g.instance})
		new, _ = encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "held", Instance: g.instance, Generation: 1, Token: g.writerToken, Process: g.process})
	case "writer-release":
		issuedIndex = 5
		successor = g.prefix == freshReleased
		if g.prefix != freshHeld && g.prefix != freshReleased || r.Authority.Generation != 1 || r.Authority.Token != g.writerToken {
			return false
		}
		old, err = encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "held", Instance: g.instance, Generation: 1, Token: g.writerToken, Process: g.process})
		new, _ = encodeWriterOwner(writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: g.instance, Generation: 2, PreviousGeneration: 1, PreviousToken: g.writerToken})
	default:
		return false
	}
	if len(g.issued) <= issuedIndex || g.issued[issuedIndex] != r.ID {
		return false
	}
	if successor && (g.witness != "replace-witness.json" || g.temporary != "") {
		return false
	}
	if err != nil || len(new) == 0 {
		return false
	}
	oldHash, newHash := sha256.Sum256(old), sha256.Sum256(new)
	return r.Files.Previous.Identity.Size == int64(len(old)) && r.Files.Staged.Identity.Size == int64(len(new)) && r.Files.Previous.SHA256 == hex.EncodeToString(oldHash[:]) && r.Files.Staged.SHA256 == hex.EncodeToString(newHash[:])
}
