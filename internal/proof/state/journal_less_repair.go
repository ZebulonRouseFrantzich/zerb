package state

import "errors"

type journalLessRepairResult struct {
	classification string
	kind           string
	candidate      receiptDigest
}

// ARCH-027/030: only state recognizes and repairs the journal-less authority.
func repairJournalLessUnderLease(s, d, external string, lease *proofLease, prefix recoverableTransitionPrefix, hook func(string) error) (journalLessRepairResult, bool, error) {
	if err := revalidateRecoveryTransferAuthority(s, d, external, lease, prefix); err != nil {
		return journalLessRepairResult{}, false, err
	}
	journaled := transitionStageIndex(prefix.stage) >= transitionStageIndex("journal-prepared-publication") &&
		transitionStageIndex(prefix.stage) < transitionStageIndex("cleanup-journal-remove")
	if journaled {
		return journalLessRepairResult{}, false, nil
	}
	if err := convergeTransitionUnderLease(s, d, external, lease, prefix, hook); err != nil {
		return journalLessRepairResult{}, true, err
	}
	classification := classify(s, d)
	switch classification {
	case "READY(∅)":
		return journalLessRepairResult{classification: classification, kind: "StateRepaired"}, true, nil
	case "STABLE(A)":
		candidate := prefix.refs.a
		if transitionStageIndex(prefix.stage) >= transitionStageIndex("current-publication") {
			candidate = prefix.refs.b
		}
		return journalLessRepairResult{classification: classification, kind: "CandidateRecovered", candidate: receiptDigest(candidate)}, true, nil
	default:
		return journalLessRepairResult{}, true, errors.New("journal-less repair did not restore prior authority")
	}
}
