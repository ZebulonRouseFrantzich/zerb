package state

import "github.com/ZebulonRouseFrantzich/zerb/internal/receipt"

func (b *codecBudget) reserveJournal(v journalRecord) bool {
	for _, value := range [...]string{v.Domain, v.Instance, v.OperationID, v.OperationToken, v.WriterToken, v.WriterPreviousToken,
		string(v.Previous), string(v.PreviousReceipt), string(v.Candidate), string(v.Phase)} {
		if !b.add(uint64(len(value)), 1) {
			return false
		}
	}
	for _, observations := range [...][]receipt.DestinationObservation{v.Preconditions, v.Results} {
		if !b.add(uint64(cap(observations)), 96) || !b.add(uint64(len(observations)), 96) {
			return false
		}
		for _, observation := range observations[:cap(observations)] {
			if !b.reserveDestinationObservation(observation) {
				return false
			}
		}
	}
	if !b.add(uint64(cap(v.CompletedPhases)), 16) || !b.add(uint64(len(v.CompletedPhases)), 16) {
		return false
	}
	for _, phase := range v.CompletedPhases[:cap(v.CompletedPhases)] {
		if !b.add(uint64(len(phase)), 1) {
			return false
		}
	}
	return true
}

// PC-042: reserve full caller capacities and payloads, exact copied slice lengths,
// and all output buffers in one checked schedule before any copy or encoding.
func (b *codecBudget) reserveSource(v receipt.Source) bool {
	if !b.add(uint64(len(v.URI)), 1) {
		return false
	}
	if !b.add(uint64(len(v.SourceID)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Commit)), 1) {
		return false
	}
	if !b.add(uint64(len(v.RootTree)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reserveFile(v receipt.File) bool {
	if !b.add(uint64(len(v.Path)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Mode)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Hash)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reserveComponent(v receipt.Component, copying bool) bool {
	if !b.add(uint64(len(v.ID)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Path)), 1) {
		return false
	}
	if !b.add(uint64(len(v.TreeDigest)), 1) {
		return false
	}
	if !b.add(uint64(cap(v.Files)), 48) || copying && !b.add(uint64(len(v.Files)), 48) {
		return false
	}
	for _, item := range v.Files[:cap(v.Files)] {
		if !b.reserveFile(item) {
			return false
		}
	}
	return true
}
func (b *codecBudget) reserveExecutable(v receipt.Executable) bool {
	if !b.add(uint64(len(v.ID)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Version)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Path)), 1) {
		return false
	}
	if !b.add(uint64(len(v.SHA256)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reserveParser(v receipt.Parser) bool {
	if !b.add(uint64(len(v.Module)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Version)), 1) {
		return false
	}
	if !b.add(uint64(len(v.ModuleSum)), 1) {
		return false
	}
	if !b.add(uint64(len(v.GoModSum)), 1) {
		return false
	}
	if !b.add(uint64(len(v.OriginCommit)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reservePolicy(v receipt.Policy, copying bool) bool {
	if !b.add(uint64(len(v.Domain)), 1) {
		return false
	}
	if !b.add(uint64(cap(v.ObjectModes)), 16) || copying && !b.add(uint64(len(v.ObjectModes)), 16) {
		return false
	}
	for _, item := range v.ObjectModes[:cap(v.ObjectModes)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	if !b.add(uint64(cap(v.Declarations)), 16) || copying && !b.add(uint64(len(v.Declarations)), 16) {
		return false
	}
	for _, item := range v.Declarations[:cap(v.Declarations)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	if !b.add(uint64(cap(v.TemplateActions)), 16) || copying && !b.add(uint64(len(v.TemplateActions)), 16) {
		return false
	}
	for _, item := range v.TemplateActions[:cap(v.TemplateActions)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	if !b.add(uint64(cap(v.ProcessClasses)), 16) || copying && !b.add(uint64(len(v.ProcessClasses)), 16) {
		return false
	}
	for _, item := range v.ProcessClasses[:cap(v.ProcessClasses)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	if !b.add(uint64(cap(v.PolicyIDs)), 16) || copying && !b.add(uint64(len(v.PolicyIDs)), 16) {
		return false
	}
	for _, item := range v.PolicyIDs[:cap(v.PolicyIDs)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	return b.add(uint64(len(v.TargetMapperExecutableID)), 1)
}
func (b *codecBudget) reserveDestinationObservation(v receipt.DestinationObservation) bool {
	if !b.add(uint64(len(v.Target)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Kind)), 1) {
		return false
	}
	if !b.add(uint64(len(v.ContentHash)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reserveToolObservation(v receipt.ToolObservation) bool {
	if !b.add(uint64(len(v.ID)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Requested)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Installed)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Integrity)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reservePlatformObservation(v receipt.PlatformObservation) bool {
	if !b.add(uint64(len(v.Distribution)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Kernel)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Architecture)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Filesystem)), 1) {
		return false
	}
	return true
}
func (b *codecBudget) reserveCompleted(v receipt.Completed, copying bool) bool {
	if !b.add(uint64(len(v.Domain)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Candidate)), 1) {
		return false
	}
	if !b.add(uint64(len(v.Previous)), 1) {
		return false
	}
	if !b.reserveSource(v.Source) {
		return false
	}
	if !b.add(uint64(cap(v.Components)), 72) || copying && !b.add(uint64(len(v.Components)), 72) {
		return false
	}
	for i, item := range v.Components[:cap(v.Components)] {
		if !b.reserveComponent(item, copying && i < len(v.Components)) {
			return false
		}
	}
	if !b.reserveExecutable(v.Zerb) {
		return false
	}
	if !b.reserveExecutable(v.Git) {
		return false
	}
	if !b.reserveExecutable(v.Mise) {
		return false
	}
	if !b.reserveExecutable(v.Chezmoi) {
		return false
	}
	if !b.reserveParser(v.Parser) {
		return false
	}
	if !b.reservePolicy(v.Policy, copying) {
		return false
	}
	if !b.reservePlatformObservation(v.Platform) {
		return false
	}
	if !b.add(uint64(cap(v.Tools)), 64) || copying && !b.add(uint64(len(v.Tools)), 64) {
		return false
	}
	for _, item := range v.Tools[:cap(v.Tools)] {
		if !b.reserveToolObservation(item) {
			return false
		}
	}
	if !b.add(uint64(cap(v.Targets)), 16) || copying && !b.add(uint64(len(v.Targets)), 16) {
		return false
	}
	for _, item := range v.Targets[:cap(v.Targets)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	if !b.add(uint64(cap(v.Preconditions)), 96) || copying && !b.add(uint64(len(v.Preconditions)), 96) {
		return false
	}
	for _, item := range v.Preconditions[:cap(v.Preconditions)] {
		if !b.reserveDestinationObservation(item) {
			return false
		}
	}
	if !b.add(uint64(cap(v.Results)), 96) || copying && !b.add(uint64(len(v.Results)), 96) {
		return false
	}
	for _, item := range v.Results[:cap(v.Results)] {
		if !b.reserveDestinationObservation(item) {
			return false
		}
	}
	if !b.add(uint64(cap(v.Phases)), 16) || copying && !b.add(uint64(len(v.Phases)), 16) {
		return false
	}
	for _, item := range v.Phases[:cap(v.Phases)] {
		if !b.add(uint64(len(item)), 1) {
			return false
		}
	}
	return true
}
