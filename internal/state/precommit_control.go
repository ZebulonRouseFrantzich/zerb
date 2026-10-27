package state

import (
	"errors"
	"slices"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

var (
	errPrecommitControlRecords  = errors.New("invalid precommit control records")
	errPrecommitControlResource = errors.New("precommit control resources unavailable")
)

// Selected values only. Current is prior A or exactly zero, not committed B.
// No classification, admission, publication, recovery or completion authority.
func readPrecommitControl(in committedControlInput) (committedControlLinks, error) {
	reservation, err := reservePrecommitControl(in)
	if err != nil {
		return committedControlLinks{}, err
	}
	if !reservation.Present[0] || !reservation.Present[4] {
		return committedControlLinks{}, errPrecommitControlRecords
	}
	var values committedControlValues
	values.Journal, err = decodeJournalRecord(in.Journal)
	if err != nil || !precommitJournalMatches(&in, &values.Journal) {
		return committedControlLinks{}, errPrecommitControlRecords
	}
	for index := range values.Candidates {
		if reservation.Present[index] {
			values.Candidates[index], values.CandidateDigests[index], err = decodeCandidateRecord(in.Candidates[index])
			if err != nil {
				return committedControlLinks{}, errPrecommitControlRecords
			}
		}
		if reservation.Present[index+2] {
			values.Receipts[index], err = decodePendingReceipt(in.Receipts[index])
			if err != nil {
				return committedControlLinks{}, errPrecommitControlRecords
			}
		}
	}
	if values.CandidateDigests[0] != values.Journal.Candidate ||
		!journalProgressMatchesTargets(values.Journal.Phase, values.Journal.CompletedPhases,
			values.Candidates[0].Targets, values.Journal.Preconditions, values.Journal.Results) ||
		!precommitPriorMatches(&in, &values) {
		return committedControlLinks{}, errPrecommitControlRecords
	}
	result := committedControlLinks{
		Candidate: values.Journal.Candidate, OperationID: values.Journal.OperationID,
		OperationToken: values.Journal.OperationToken, WriterGeneration: values.Journal.WriterGeneration,
		WriterToken: values.Journal.WriterToken, Phase: values.Journal.Phase,
	}
	if reservation.Present[2] {
		if values.Journal.Phase != PhasePendingReceipt ||
			reservation.Present[3] && values.Receipts[0].Digest == values.Receipts[1].Digest ||
			!precommitPendingMatches(&values.Receipts[0], &values.Candidates[0], values.CandidateDigests[0], &values.Journal) {
			return committedControlLinks{}, errPrecommitControlRecords
		}
		result.Receipt = values.Receipts[0].Digest
	} else if values.Journal.Phase == PhasePendingReceipt && reservation.Present[3] &&
		precommitPendingMatches(&values.Receipts[1], &values.Candidates[0], values.CandidateDigests[0], &values.Journal) {
		// Earlier presence of RA is legitimate, but is a pending role only here.
		result.Receipt = values.Receipts[1].Digest
	}
	return result, nil
}

func reservePrecommitControl(in committedControlInput) (committedControlCounters, error) {
	if !validWriterOwnerRecord(in.Writer) || in.Writer.Status != "held" ||
		in.Current != (currentRecord{}) && !precommitCurrentMatchesWriter(&in.Current, &in.Writer) {
		return committedControlCounters{}, errPrecommitControlRecords
	}
	reservation, err := reserveSelectedControl(&in)
	if err == errCommittedControlResource {
		return committedControlCounters{}, errPrecommitControlResource
	}
	if err != nil {
		return committedControlCounters{}, errPrecommitControlRecords
	}
	return reservation, nil
}

func precommitCurrentMatchesWriter(current *currentRecord, writer *writerOwner) bool {
	if !validCurrentRecord(*current) || current.Instance != writer.Instance ||
		current.WriterGeneration >= writer.Generation || current.WriterToken == writer.Token ||
		current.WriterToken == writer.PreviousToken {
		return false
	}
	distance := writer.Generation - current.WriterGeneration
	return distance >= 2 && (writer.PreviousToken == "" || distance >= 3)
}

func precommitJournalMatches(in *committedControlInput, journal *journalRecord) bool {
	if journal.Instance != in.Writer.Instance || journal.Phase == PhaseCleanup {
		return false
	}
	same := journal.WriterGeneration == in.Writer.Generation && journal.WriterToken == in.Writer.Token &&
		journal.WriterPreviousToken == in.Writer.PreviousToken
	ahead := journal.WriterGeneration > in.Writer.Generation && journal.WriterGeneration-in.Writer.Generation == 1 &&
		journal.WriterPreviousToken == in.Writer.Token && journal.WriterToken != in.Writer.Token &&
		journal.WriterToken != in.Writer.Instance && journal.WriterToken != in.Writer.PreviousToken
	if !same && !ahead {
		return false
	}
	if in.Current == (currentRecord{}) {
		return journal.Previous == "" && journal.PreviousReceipt == ""
	}
	return journal.Previous == in.Current.Candidate && journal.PreviousReceipt == in.Current.Receipt &&
		journal.OperationID != in.Current.OperationID && journal.OperationToken != in.Current.OperationToken &&
		journal.WriterToken != in.Current.WriterToken
}

func precommitPriorMatches(in *committedControlInput, values *committedControlValues) bool {
	if in.Current == (currentRecord{}) {
		return in.Candidates[1] == nil && in.Receipts[1] == nil
	}
	if in.Receipts[1] == nil || values.Receipts[1].Digest != in.Current.Receipt ||
		!completedChecksPassed(values.Receipts[1].Facts) {
		return false
	}
	prior := &values.Candidates[0]
	if in.Current.Candidate == values.CandidateDigests[0] {
		if in.Candidates[1] != nil {
			return false
		}
	} else {
		if in.Candidates[1] == nil || values.CandidateDigests[1] != in.Current.Candidate {
			return false
		}
		prior = &values.Candidates[1]
	}
	return committedCandidateMatchesReceipt(prior, &values.Receipts[1], in.Current.Candidate)
}

func precommitPendingMatches(pending *pendingReceipt, candidate *receipt.Candidate, digest receipt.Digest, journal *journalRecord) bool {
	return completedChecksPassed(pending.Facts) &&
		committedCandidateMatchesReceipt(candidate, pending, digest) && pending.Facts.Previous == journal.Previous &&
		slices.Equal(pending.Facts.Preconditions, journal.Preconditions) && slices.Equal(pending.Facts.Results, journal.Results)
}
