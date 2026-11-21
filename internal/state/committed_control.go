package state

import (
	"slices"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// Value correspondence only: no classification, source/install authentication,
// current completion, filesystem, liveness, lease, repair or publication authority.
func readCommittedControl(in committedControlInput) (committedControlLinks, error) {
	reservation, err := reserveCommittedControl(in)
	if err != nil {
		return committedControlLinks{}, err
	}
	if !reservation.Present[0] || !reservation.Present[2] ||
		!reservation.Present[4] && (reservation.Present[1] || reservation.Present[3]) {
		return committedControlLinks{}, errCommittedControlRecords
	}
	var values committedControlValues
	if reservation.Present[4] {
		values.Journal, err = decodeJournalRecord(in.Journal)
		if err != nil || !committedJournalMatches(&in.Current, &in.Writer, &values.Journal) {
			return committedControlLinks{}, errCommittedControlRecords
		}
	}
	for index := range values.Candidates {
		if reservation.Present[index] {
			values.Candidates[index], values.CandidateDigests[index], err = decodeCandidateRecord(in.Candidates[index])
			if err != nil {
				return committedControlLinks{}, errCommittedControlRecords
			}
		}
		if reservation.Present[index+2] {
			values.Receipts[index], err = decodePendingReceipt(in.Receipts[index])
			if err != nil {
				return committedControlLinks{}, errCommittedControlRecords
			}
		}
	}
	if values.CandidateDigests[0] != in.Current.Candidate || values.Receipts[0].Digest != in.Current.Receipt ||
		!completedChecksPassed(values.Receipts[0].Facts) ||
		!committedCandidateMatchesReceipt(&values.Candidates[0], &values.Receipts[0], values.CandidateDigests[0]) {
		return committedControlLinks{}, errCommittedControlRecords
	}
	if reservation.Present[4] && !committedCleanupMatches(&in, &values) {
		return committedControlLinks{}, errCommittedControlRecords
	}
	result := committedControlLinks{
		Candidate: in.Current.Candidate, Receipt: in.Current.Receipt,
		OperationID: in.Current.OperationID, OperationToken: in.Current.OperationToken,
		WriterGeneration: in.Current.WriterGeneration, WriterToken: in.Current.WriterToken,
	}
	if reservation.Present[4] {
		result.Phase = values.Journal.Phase
	}
	return result, nil
}

func committedJournalMatches(current *currentRecord, writer *writerOwner, journal *journalRecord) bool {
	if writer.Status != "held" || journal.Instance != current.Instance ||
		journal.OperationID != current.OperationID || journal.OperationToken != current.OperationToken ||
		journal.Candidate != current.Candidate || journal.Phase != PhasePendingReceipt && journal.Phase != PhaseCleanup {
		return false
	}
	same := journal.WriterGeneration == writer.Generation && journal.WriterToken == writer.Token &&
		journal.WriterPreviousToken == writer.PreviousToken
	ahead := journal.WriterGeneration > writer.Generation && journal.WriterGeneration-writer.Generation == 1 &&
		journal.WriterPreviousToken == writer.Token && journal.WriterToken != writer.Token &&
		journal.WriterToken != writer.Instance && journal.WriterToken != writer.PreviousToken
	if !same && !ahead {
		return false
	}
	if current.WriterGeneration == journal.WriterGeneration {
		return current.WriterToken == journal.WriterToken
	}
	if current.WriterGeneration > journal.WriterGeneration || journal.WriterPreviousToken == "" || current.WriterToken == journal.WriterToken {
		return false
	}
	predecessor := journal.WriterGeneration - 1 // The canonical journal requires a positive generation.
	if current.WriterGeneration == predecessor {
		return current.WriterToken == journal.WriterPreviousToken
	}
	return current.WriterGeneration < predecessor && current.WriterToken != journal.WriterPreviousToken
}

func committedCandidateMatchesReceipt(candidate *receipt.Candidate, pending *pendingReceipt, digest receipt.Digest) bool {
	facts := &pending.Facts
	if facts.Candidate != digest || facts.Source != candidate.Source || facts.Parser != candidate.Parser ||
		!committedComponentsEqual(candidate.Components, facts.Components) ||
		!committedPoliciesEqual(&candidate.Policy, &facts.Policy) || !slices.Equal(candidate.Targets, facts.Targets) ||
		len(candidate.Tools) != len(facts.Tools) || len(candidate.Executables) != 4 {
		return false
	}
	if facts.Git != candidate.Executables[2] || facts.Mise != candidate.Executables[3] || facts.Chezmoi != candidate.Executables[1] {
		return false
	}
	for index := range candidate.Tools {
		tool, observed := &candidate.Tools[index], &facts.Tools[index]
		if len(tool.Locks) != 1 || observed.ID != tool.ID || observed.Requested != tool.Version || observed.Installed != tool.Version ||
			observed.Integrity != tool.Locks[0].Value {
			return false
		}
	}
	return true
}

func committedComponentsEqual(first, second []receipt.Component) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].ID != second[index].ID || first[index].Path != second[index].Path ||
			first[index].TreeDigest != second[index].TreeDigest || !slices.Equal(first[index].Files, second[index].Files) {
			return false
		}
	}
	return true
}

func committedPoliciesEqual(first, second *receipt.Policy) bool {
	return first.Schema == second.Schema && first.Domain == second.Domain &&
		first.TargetMapperExecutableID == second.TargetMapperExecutableID && first.ProviderFree == second.ProviderFree &&
		first.HostFactIndependent == second.HostFactIndependent && first.DestinationRecheck == second.DestinationRecheck &&
		first.ExactReapplyOnly == second.ExactReapplyOnly && slices.Equal(first.ObjectModes, second.ObjectModes) &&
		slices.Equal(first.Declarations, second.Declarations) && slices.Equal(first.TemplateActions, second.TemplateActions) &&
		slices.Equal(first.ProcessClasses, second.ProcessClasses) && slices.Equal(first.PolicyIDs, second.PolicyIDs)
}

func committedCleanupMatches(in *committedControlInput, values *committedControlValues) bool {
	journal, currentReceipt := &values.Journal, &values.Receipts[0]
	if currentReceipt.Facts.Previous != journal.Previous ||
		!journalProgressMatchesTargets(journal.Phase, journal.CompletedPhases, values.Candidates[0].Targets, journal.Preconditions, journal.Results) ||
		!slices.Equal(currentReceipt.Facts.Preconditions, journal.Preconditions) || !slices.Equal(currentReceipt.Facts.Results, journal.Results) {
		return false
	}
	if journal.Previous == "" {
		return in.Candidates[1] == nil && in.Receipts[1] == nil
	}
	var priorCandidate *receipt.Candidate
	var priorReceipt *pendingReceipt
	candidateDistinct := journal.Previous != in.Current.Candidate
	receiptDistinct := journal.PreviousReceipt != in.Current.Receipt
	if !candidateDistinct {
		if in.Candidates[1] != nil {
			return false
		}
		priorCandidate = &values.Candidates[0]
	} else if in.Candidates[1] != nil {
		if values.CandidateDigests[1] != journal.Previous {
			return false
		}
		priorCandidate = &values.Candidates[1]
	}
	if !receiptDistinct {
		if in.Receipts[1] != nil {
			return false
		}
		priorReceipt = &values.Receipts[0]
	} else if in.Receipts[1] != nil {
		if values.Receipts[1].Digest != journal.PreviousReceipt {
			return false
		}
		priorReceipt = &values.Receipts[1]
	}
	if journal.Phase == PhasePendingReceipt && (priorCandidate == nil || priorReceipt == nil) ||
		candidateDistinct && priorCandidate != nil && receiptDistinct && priorReceipt == nil {
		return false
	}
	if priorReceipt != nil {
		if priorReceipt.Facts.Candidate != journal.Previous || !completedChecksPassed(priorReceipt.Facts) {
			return false
		}
		if priorCandidate != nil && !committedCandidateMatchesReceipt(priorCandidate, priorReceipt, journal.Previous) {
			return false
		}
	}
	return true
}
