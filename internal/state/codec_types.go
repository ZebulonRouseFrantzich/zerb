package state

import (
	"errors"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

const codecReservationLimit = uint64(8_000_000)

var errCanonicalStateRecord = errors.New("invalid canonical state record")

type codecKind uint8

const (
	codecCandidate codecKind = iota + 1
	codecPending
	codecJournal
)

type pendingReceipt struct {
	Schema int               `json:"schema"`
	Domain string            `json:"domain"`
	Digest receipt.Digest    `json:"digest"`
	Facts  receipt.Completed `json:"facts"`
}

type journalRecord struct {
	Schema              int                              `json:"schema"`
	Domain              string                           `json:"domain"`
	Instance            string                           `json:"instance"`
	OperationID         string                           `json:"operation_id"`
	OperationToken      string                           `json:"operation_token"`
	WriterGeneration    uint64                           `json:"writer_generation"`
	WriterToken         string                           `json:"writer_token"`
	WriterPreviousToken string                           `json:"writer_previous_token"`
	Previous            receipt.Digest                   `json:"previous"`
	PreviousReceipt     receipt.Digest                   `json:"previous_receipt"`
	Candidate           receipt.Digest                   `json:"candidate"`
	Phase               Phase                            `json:"phase"`
	Preconditions       []receipt.DestinationObservation `json:"preconditions"`
	Results             []receipt.DestinationObservation `json:"results"`
	CompletedPhases     []Phase                          `json:"completed_phases"`
}

type codecSpan struct{ start, end int }

// The fixed schema reuses slots across disjoint top-level record kinds.
const (
	codecComponents = iota
	codecFiles0
	codecFiles1
	codecTools
	codecLocks // Completed/journal preconditions.
	codecTargets
	codecHomeSources // Completed/journal results.
	codecExecutables // Completed/journal phases.
	codecPolicyModes
	codecPolicyDeclarations
	codecPolicyActions
	codecPolicyClasses
	codecPolicyIDs
	codecCollectionSlots
)

type codecPlan struct {
	wire, strings, arrays, trees, uriBytes, valueStrings uint64
	kind                                                 codecKind
	counts                                               [codecCollectionSlots]uint64
	// Complete candidate/facts, then the two component files arrays.
	spans [3]codecSpan
	// Complete candidate/facts SHA256, then the two tree SHA256 values.
	hashes [3][32]byte
}

type codecBudget struct{ used uint64 }

func (b *codecBudget) add(count, size uint64) bool {
	if b.used > codecReservationLimit || size != 0 && count > (codecReservationLimit-b.used)/size {
		return false
	}
	b.used += count * size
	return true
}

func codecSourceScratch(b *codecBudget, uriBytes uint64) bool {
	return uriBytes <= 2048 && b.add(1, 2048) && b.add(uriBytes, 16)
}

// Gate both actual decoding storage and the old minimum-capacity encoder
// schedule before materialization. Efficiency never widens the accepted set.
func (p *codecPlan) fits() bool {
	var budget codecBudget
	control, wire := uint64(2048), p.wire
	switch p.kind {
	case codecCandidate:
		if p.valueStrings > 16384 {
			return false
		}
	case codecPending:
		if wire < 128 {
			return false
		}
		control, wire = 4096, wire-128
	case codecJournal:
	default:
		return false
	}
	if !budget.add(1, control) {
		return false
	}
	if p.kind != codecJournal && !codecSourceScratch(&budget, p.uriBytes) {
		return false
	}
	if !budget.add(wire, 1) || !budget.add(p.strings, 1) || !budget.add(p.arrays, 2) || !budget.add(p.trees, 1) {
		return false
	}
	if p.kind == codecPending {
		control = 3968
	}
	budget = codecBudget{}
	if !budget.add(1, control) {
		return false
	}
	if p.kind != codecJournal && !codecSourceScratch(&budget, p.uriBytes) {
		return false
	}
	return budget.add(p.wire, 1) && budget.add(p.strings, 1) && budget.add(p.arrays, 1)
}
