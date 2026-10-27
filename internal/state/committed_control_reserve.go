package state

import (
	"errors"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

const (
	committedControlOuter            = uint64(8493)
	committedControlWireBudget       = 5 * codecReservationLimit
	committedControlReservationLimit = 2*committedControlWireBudget + committedControlOuter
)

var (
	errCommittedControlRecords  = errors.New("invalid committed control records")
	errCommittedControlResource = errors.New("committed control resources unavailable")
)

// Only nil means an absent selected object. Slot zero is current; slot one is a
// distinct prior object. This is not a namespace inventory or filesystem proof.
type committedControlInput struct {
	Current    currentRecord
	Writer     writerOwner
	Candidates [2][]byte
	Receipts   [2][]byte
	Journal    []byte
}

type committedControlLinks struct {
	Candidate, Receipt          receipt.Digest
	OperationID, OperationToken string
	WriterGeneration            uint64
	WriterToken                 string
	Phase                       Phase
}

type committedControlValues struct {
	Candidates       [2]receipt.Candidate
	Receipts         [2]pendingReceipt
	Journal          journalRecord
	CandidateDigests [2]receipt.Digest
}

type committedControlCounters struct {
	Borrowed, Decoders, Reservation uint64
	Index                           int
	Kind                            codecKind
	Present                         [5]bool
}

type committedControlPreflightResult struct {
	Reservation        uint64
	GrammarValid, Fits bool
}

func committedControlWire(in *committedControlInput, index int) ([]byte, codecKind) {
	if index < 2 {
		return in.Candidates[index], codecCandidate
	}
	if index < 4 {
		return in.Receipts[index-2], codecPending
	}
	return in.Journal, codecJournal
}

// This complete fixed-role reservation is independent of any Store/history
// envelope. Keep all retained inputs and decoder schedules charged together.
func reserveCommittedControl(in committedControlInput) (committedControlCounters, error) {
	var counter committedControlCounters
	if !currentMatchesWriter(in.Current, in.Writer) {
		return counter, errCommittedControlRecords
	}
	for counter.Index = 0; counter.Index < len(counter.Present); counter.Index++ {
		wire, kind := committedControlWire(&in, counter.Index)
		counter.Kind = kind
		counter.Present[counter.Index] = wire != nil
		if uint64(len(wire)) > codecReservationLimit || uint64(cap(wire)) > committedControlWireBudget-counter.Borrowed {
			return committedControlCounters{}, errCommittedControlResource
		}
		counter.Borrowed += uint64(cap(wire))
	}
	for counter.Index = 0; counter.Index < len(counter.Present); counter.Index++ {
		if !counter.Present[counter.Index] {
			continue
		}
		wire, kind := committedControlWire(&in, counter.Index)
		counter.Kind = kind
		plan := preflightCommittedControlWire(wire, kind)
		if !plan.GrammarValid {
			return committedControlCounters{}, errCommittedControlRecords
		}
		if !plan.Fits || plan.Reservation > committedControlWireBudget-counter.Decoders {
			return committedControlCounters{}, errCommittedControlResource
		}
		counter.Decoders += plan.Reservation
	}
	counter.Reservation = committedControlOuter
	if counter.Borrowed > committedControlReservationLimit-counter.Reservation {
		return committedControlCounters{}, errCommittedControlResource
	}
	counter.Reservation += counter.Borrowed
	if counter.Decoders > committedControlReservationLimit-counter.Reservation {
		return committedControlCounters{}, errCommittedControlResource
	}
	counter.Reservation += counter.Decoders
	return counter, nil
}

func preflightCommittedControlWire(wire []byte, kind codecKind) committedControlPreflightResult {
	p := codecPlan{kind: kind, wire: uint64(len(wire))}
	r := codecReader{data: wire}
	switch kind {
	case codecCandidate:
		var value receipt.Candidate
		r.candidate(&p, &value)
	case codecPending:
		var value pendingReceipt
		r.pending(&p, &value)
	case codecJournal:
		var value journalRecord
		r.journal(&p, &value)
	default:
		return committedControlPreflightResult{}
	}
	if !r.done() || kind == codecCandidate && p.valueStrings > 16384 ||
		kind != codecJournal && p.uriBytes > 2048 || kind == codecPending && p.wire < 128 {
		return committedControlPreflightResult{}
	}
	result := committedControlPreflightResult{GrammarValid: true}
	if !p.fits() {
		return result
	}
	var budget codecBudget
	control := uint64(2048)
	if kind == codecPending {
		control = 3968
	}
	if !budget.add(1, control) || kind != codecJournal && !codecSourceScratch(&budget, p.uriBytes) ||
		!budget.add(p.wire, 1) || !budget.add(p.strings, 1) || !budget.add(p.arrays, 1) {
		return result
	}
	result.Reservation, result.Fits = budget.used, true
	return result
}
