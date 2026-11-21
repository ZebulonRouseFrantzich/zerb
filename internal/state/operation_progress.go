package state

import "github.com/ZebulonRouseFrantzich/zerb/internal/receipt"

// ARCH-024/025: execution order, never receipt-fact sorting or action authority.
func validOperationPrefix(phase Phase, completed []Phase) bool {
	if len(completed) > 2 {
		return false
	}
	tool := len(completed) >= 1 && completed[0] == PhaseToolInstall
	both := len(completed) == 2 && tool && completed[1] == PhaseHomeApply
	switch phase {
	case PhasePrepared:
		return len(completed) == 0
	case PhaseToolInstall:
		return len(completed) == 0 || len(completed) == 1 && tool
	case PhaseHomeApply:
		return len(completed) == 1 && tool || both
	case PhasePendingReceipt, PhaseCleanup:
		return both
	default:
		return false
	}
}

// Targets must already be canonical candidate facts. This checks only coverage,
// not observation grammar, freshness, filesystem identity or candidate authority.
func observationCoverage(targets []string, observed []receipt.DestinationObservation) bool {
	if len(targets) > 4096 || len(targets) != len(observed) {
		return false
	}
	for i, target := range targets {
		if target == "" || i > 0 && targets[i-1] >= target || observed[i].Target != target {
			return false
		}
	}
	return true
}

func journalProgressMatchesTargets(phase Phase, completed []Phase, targets []string, preconditions, results []receipt.DestinationObservation) bool {
	return validOperationPrefix(phase, completed) && observationCoverage(targets, preconditions) && observationCoverage(targets, results)
}

// ARCH-026: necessary value checks only. Stable commit still requires the full
// authenticated candidate/receipt/runtime/lease graph and atomic current record.
func completedChecksPassed(facts receipt.Completed) bool {
	d := facts.Dimensions
	return d.InputsPinned && d.ArtifactIntegrity && d.HostCompatible && d.ConvergenceVerified &&
		len(facts.Phases) == 2 && facts.Phases[0] == string(PhaseHomeApply) && facts.Phases[1] == string(PhaseToolInstall)
}
