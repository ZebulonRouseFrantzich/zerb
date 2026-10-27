package state

import (
	"crypto/sha256"
	"math"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

const codecReadTreePrefix = `{"schema":1,"domain":"zerb.tree.v1","files":`

func decodeCandidateRecord(data []byte) (receipt.Candidate, receipt.Digest, error) {
	var p codecPlan
	p.kind, p.wire = codecCandidate, uint64(len(data))
	if p.wire > codecReservationLimit {
		return receipt.Candidate{}, "", errCanonicalStateRecord
	}
	r := codecReader{data: data}
	var value receipt.Candidate
	r.candidate(&p, &value)
	if !r.done() || !p.fits() {
		return receipt.Candidate{}, "", errCanonicalStateRecord
	}
	owned := make([]byte, len(data))
	copy(owned, data)
	codecReadHashes(owned, &p)
	r = codecReader{data: owned, materialize: true}
	r.candidate(&p, &value)
	if !r.done() || !codecValidateCandidate(value) || !codecReadTreesMatch(value.Components, &p) {
		return receipt.Candidate{}, "", errCanonicalStateRecord
	}
	return value, codecDigestFromHash(p.hashes[0]), nil
}

func decodePendingReceipt(data []byte) (pendingReceipt, error) {
	var p codecPlan
	p.kind, p.wire = codecPending, uint64(len(data))
	if p.wire > codecReservationLimit {
		return pendingReceipt{}, errCanonicalStateRecord
	}
	r := codecReader{data: data}
	var value pendingReceipt
	r.pending(&p, &value)
	if !r.done() || !p.fits() {
		return pendingReceipt{}, errCanonicalStateRecord
	}
	owned := make([]byte, len(data))
	copy(owned, data)
	codecReadHashes(owned, &p)
	r = codecReader{data: owned, materialize: true}
	r.pending(&p, &value)
	if !r.done() || !codecTreeDigestMatches(string(value.Digest), p.hashes[0]) || !codecValidateCompleted(value.Facts) || !codecReadTreesMatch(value.Facts.Components, &p) {
		return pendingReceipt{}, errCanonicalStateRecord
	}
	return value, nil
}

func decodeJournalRecord(data []byte) (journalRecord, error) {
	var p codecPlan
	p.kind, p.wire = codecJournal, uint64(len(data))
	if p.wire > codecReservationLimit {
		return journalRecord{}, errCanonicalStateRecord
	}
	r := codecReader{data: data}
	var value journalRecord
	r.journal(&p, &value)
	if !r.done() || !p.fits() {
		return journalRecord{}, errCanonicalStateRecord
	}
	owned := make([]byte, len(data))
	copy(owned, data)
	r = codecReader{data: owned, materialize: true}
	r.journal(&p, &value)
	if !r.done() || !codecValidateJournal(value) {
		return journalRecord{}, errCanonicalStateRecord
	}
	return value, nil
}

func codecReadHashes(data []byte, p *codecPlan) {
	hash := sha256.New()
	span := p.spans[0]
	_, _ = hash.Write(data[span.start:span.end])
	_ = hash.Sum(p.hashes[0][:0])
	for i := 1; i < len(p.spans); i++ {
		hash.Reset()
		span = p.spans[i]
		_, _ = hash.Write([]byte(codecReadTreePrefix))
		_, _ = hash.Write(data[span.start:span.end])
		_, _ = hash.Write([]byte("}"))
		_ = hash.Sum(p.hashes[i][:0])
	}
}

func codecReadTreesMatch(components []receipt.Component, p *codecPlan) bool {
	return len(components) == 2 && codecTreeDigestMatches(components[0].TreeDigest, p.hashes[1]) && codecTreeDigestMatches(components[1].TreeDigest, p.hashes[2])
}

func (r *codecReader) pendingDigest(p *codecPlan) receipt.Digest {
	start := r.pos
	value := r.text(p, false)
	if r.failed {
		return ""
	}
	if r.pos-start != 73 || string(r.data[start+1:start+8]) != "sha256:" {
		r.failed = true
		return ""
	}
	for _, c := range r.data[start+8 : r.pos-1] {
		if codecHexDigit(c) < 0 {
			r.failed = true
			return ""
		}
	}
	return receipt.Digest(value)
}

func (r *codecReader) pending(p *codecPlan, v *pendingReceipt) {
	r.take(`{"schema":1,"domain":`)
	v.Schema = 1
	v.Domain = r.fixedText(p, "zerb.receipt", false)
	r.take(`,"digest":`)
	v.Digest = r.pendingDigest(p)
	r.take(`,"facts":`)
	r.completed(p, &v.Facts)
	r.take(`}`)
}
func (r *codecReader) source(p *codecPlan) receipt.Source {
	var v receipt.Source
	r.take(`{"uri":`)
	before := p.strings
	v.URI = r.text(p, true)
	if !r.materialize {
		p.uriBytes = p.strings - before
	}
	r.take(`,"source_id":`)
	v.SourceID = r.text(p, true)
	r.take(`,"commit":`)
	v.Commit = r.text(p, true)
	r.take(`,"root_tree":`)
	v.RootTree = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) file(p *codecPlan) receipt.File {
	var v receipt.File
	r.take(`{"path":`)
	v.Path = r.text(p, true)
	r.take(`,"mode":`)
	v.Mode = r.text(p, true)
	r.take(`,"hash":`)
	v.Hash = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) executable(p *codecPlan) receipt.Executable {
	var v receipt.Executable
	r.take(`{"id":`)
	v.ID = r.text(p, true)
	r.take(`,"version":`)
	v.Version = r.text(p, true)
	r.take(`,"path":`)
	v.Path = r.text(p, true)
	r.take(`,"sha256":`)
	v.SHA256 = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) parser(p *codecPlan) receipt.Parser {
	var v receipt.Parser
	r.take(`{"schema":`)
	r.take("1")
	v.Schema = 1
	r.take(`,"module":`)
	v.Module = r.text(p, true)
	r.take(`,"version":`)
	v.Version = r.text(p, true)
	r.take(`,"module_sum":`)
	v.ModuleSum = r.text(p, true)
	r.take(`,"go_mod_sum":`)
	v.GoModSum = r.text(p, true)
	r.take(`,"origin_commit":`)
	v.OriginCommit = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) policy(p *codecPlan) receipt.Policy {
	var v receipt.Policy
	r.take(`{"schema":`)
	r.take("1")
	v.Schema = 1
	r.take(`,"domain":`)
	v.Domain = r.fixedText(p, "zerb.policy", true)
	r.take(`,"object_modes":`)
	v.ObjectModes = codecReadTexts(r, p, codecPolicyModes)
	r.take(`,"declarations":`)
	v.Declarations = codecReadTexts(r, p, codecPolicyDeclarations)
	r.take(`,"template_actions":`)
	v.TemplateActions = codecReadTexts(r, p, codecPolicyActions)
	r.take(`,"process_classes":`)
	v.ProcessClasses = codecReadTexts(r, p, codecPolicyClasses)
	r.take(`,"policy_ids":`)
	v.PolicyIDs = codecReadTexts(r, p, codecPolicyIDs)
	r.take(`,"target_mapper_executable_id":`)
	v.TargetMapperExecutableID = r.text(p, true)
	r.take(`,"provider_free":`)
	v.ProviderFree = r.boolean()
	r.take(`,"host_fact_independent":`)
	v.HostFactIndependent = r.boolean()
	r.take(`,"destination_recheck":`)
	v.DestinationRecheck = r.boolean()
	r.take(`,"exact_reapply_only":`)
	v.ExactReapplyOnly = r.boolean()
	r.take(`}`)
	return v
}
func (r *codecReader) candidate(p *codecPlan, v *receipt.Candidate) {
	start := r.pos
	r.take(`{"schema":`)
	r.take("1")
	v.Schema = 1
	r.take(`,"domain":`)
	v.Domain = r.fixedText(p, "zerb.candidate", true)
	r.take(`,"source":`)
	v.Source = r.source(p)
	r.take(`,"profile_id":`)
	v.ProfileID = r.text(p, true)
	r.take(`,"components":`)
	v.Components = codecReadComponents(r, p)
	r.take(`,"tools":`)
	v.Tools = codecReadTools(r, p)
	r.take(`,"home_sources":`)
	v.HomeSources = codecReadTexts(r, p, codecHomeSources)
	r.take(`,"targets":`)
	v.Targets = codecReadTexts(r, p, codecTargets)
	r.take(`,"executables":`)
	v.Executables = codecReadExecutables(r, p)
	r.take(`,"parser":`)
	v.Parser = r.parser(p)
	r.take(`,"policy":`)
	v.Policy = r.policy(p)
	r.take(`}`)
	if !r.materialize {
		p.spans[0] = codecSpan{start, r.pos}
	}
}
func (r *codecReader) destination(p *codecPlan) receipt.DestinationObservation {
	var v receipt.DestinationObservation
	r.take(`{"target":`)
	v.Target = r.text(p, true)
	r.take(`,"exists":`)
	v.Exists = r.boolean()
	r.take(`,"kind":`)
	v.Kind = r.text(p, true)
	r.take(`,"mode":`)
	v.Mode = uint32(r.unsigned(math.MaxUint32))
	r.take(`,"owner":`)
	v.Owner = uint32(r.unsigned(math.MaxUint32))
	r.take(`,"device":`)
	v.Device = r.unsigned(math.MaxUint64)
	r.take(`,"inode":`)
	v.Inode = r.unsigned(math.MaxUint64)
	r.take(`,"links":`)
	v.Links = r.unsigned(math.MaxUint64)
	r.take(`,"content_hash":`)
	v.ContentHash = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) observedTool(p *codecPlan) receipt.ToolObservation {
	var v receipt.ToolObservation
	r.take(`{"id":`)
	v.ID = r.text(p, true)
	r.take(`,"requested":`)
	v.Requested = r.text(p, true)
	r.take(`,"installed":`)
	v.Installed = r.text(p, true)
	r.take(`,"integrity":`)
	v.Integrity = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) platform(p *codecPlan) receipt.PlatformObservation {
	var v receipt.PlatformObservation
	r.take(`{"distribution":`)
	v.Distribution = r.text(p, true)
	r.take(`,"kernel":`)
	v.Kernel = r.text(p, true)
	r.take(`,"architecture":`)
	v.Architecture = r.text(p, true)
	r.take(`,"filesystem":`)
	v.Filesystem = r.text(p, true)
	r.take(`}`)
	return v
}
func (r *codecReader) dimensions(p *codecPlan) receipt.Dimensions {
	var v receipt.Dimensions
	r.take(`{"inputs_pinned":`)
	v.InputsPinned = r.boolean()
	r.take(`,"artifact_integrity":`)
	v.ArtifactIntegrity = r.boolean()
	r.take(`,"artifact_available":`)
	v.ArtifactAvailable = r.boolean()
	r.take(`,"host_compatible":`)
	v.HostCompatible = r.boolean()
	r.take(`,"convergence_verified":`)
	v.ConvergenceVerified = r.boolean()
	r.take(`}`)
	return v
}
func (r *codecReader) completed(p *codecPlan, v *receipt.Completed) {
	start := r.pos
	r.take(`{"schema":`)
	r.take("1")
	v.Schema = 1
	r.take(`,"domain":`)
	v.Domain = r.fixedText(p, "zerb.completed", true)
	r.take(`,"candidate":`)
	v.Candidate = receipt.Digest(r.text(p, true))
	if r.has(`,"previous":`) {
		r.take(`,"previous":`)
		previousStart := r.pos
		v.Previous = receipt.Digest(r.text(p, true))
		if r.pos-previousStart == 2 {
			r.failed = true
		}
	}
	r.take(`,"source":`)
	v.Source = r.source(p)
	r.take(`,"components":`)
	v.Components = codecReadComponents(r, p)
	r.take(`,"zerb":`)
	v.Zerb = r.executable(p)
	r.take(`,"git":`)
	v.Git = r.executable(p)
	r.take(`,"mise":`)
	v.Mise = r.executable(p)
	r.take(`,"chezmoi":`)
	v.Chezmoi = r.executable(p)
	r.take(`,"parser":`)
	v.Parser = r.parser(p)
	r.take(`,"policy":`)
	v.Policy = r.policy(p)
	r.take(`,"platform":`)
	v.Platform = r.platform(p)
	r.take(`,"tools":`)
	v.Tools = codecReadObservedTools(r, p)
	r.take(`,"targets":`)
	v.Targets = codecReadTexts(r, p, codecTargets)
	r.take(`,"preconditions":`)
	v.Preconditions = codecReadObservations(r, p, codecLocks)
	r.take(`,"results":`)
	v.Results = codecReadObservations(r, p, codecHomeSources)
	r.take(`,"phases":`)
	v.Phases = codecReadTexts(r, p, codecExecutables)
	r.take(`,"dimensions":`)
	v.Dimensions = r.dimensions(p)
	r.take(`}`)
	if !r.materialize {
		p.spans[0] = codecSpan{start, r.pos}
	}
}
func (r *codecReader) journal(p *codecPlan, v *journalRecord) {
	r.take(`{"schema":`)
	r.take("1")
	v.Schema = 1
	r.take(`,"domain":`)
	v.Domain = r.fixedText(p, "zerb.journal", true)
	r.take(`,"instance":`)
	v.Instance = r.text(p, true)
	r.take(`,"operation_id":`)
	v.OperationID = r.text(p, true)
	r.take(`,"operation_token":`)
	v.OperationToken = r.text(p, true)
	r.take(`,"writer_generation":`)
	v.WriterGeneration = r.unsigned(math.MaxUint64)
	r.take(`,"writer_token":`)
	v.WriterToken = r.text(p, true)
	r.take(`,"writer_previous_token":`)
	v.WriterPreviousToken = r.text(p, true)
	r.take(`,"previous":`)
	v.Previous = receipt.Digest(r.text(p, true))
	r.take(`,"previous_receipt":`)
	v.PreviousReceipt = receipt.Digest(r.text(p, true))
	r.take(`,"candidate":`)
	v.Candidate = receipt.Digest(r.text(p, true))
	r.take(`,"phase":`)
	v.Phase = Phase(r.text(p, true))
	r.take(`,"preconditions":`)
	v.Preconditions = codecReadObservations(r, p, codecLocks)
	r.take(`,"results":`)
	v.Results = codecReadObservations(r, p, codecHomeSources)
	r.take(`,"completed_phases":`)
	v.CompletedPhases = codecReadPhases(r, p)
	r.take(`}`)
}
func codecReadComponent(r *codecReader, p *codecPlan, index int) receipt.Component {
	var v receipt.Component
	r.take(`{"id":`)
	v.ID = r.text(p, true)
	r.take(`,"path":`)
	v.Path = r.text(p, true)
	r.take(`,"tree_digest":`)
	v.TreeDigest = r.text(p, true)
	r.take(`,"files":`)
	start := r.pos
	v.Files = codecReadFiles(r, p, codecFiles0+index)
	if !r.materialize {
		p.spans[index+1] = codecSpan{start, r.pos}
		r.add(&p.trees, uint64(r.pos-start+len(codecReadTreePrefix)+1), 1)
	}
	r.take(`}`)
	return v
}

func codecReadTool(r *codecReader, p *codecPlan, _ int) receipt.Tool {
	var v receipt.Tool
	r.take(`{"id":`)
	v.ID = r.text(p, true)
	r.take(`,"version":`)
	v.Version = r.text(p, true)
	r.take(`,"locks":`)
	v.Locks = codecReadLocks(r, p)
	r.take(`}`)
	return v
}

func codecReadLock(r *codecReader, p *codecPlan, _ int) receipt.LockValue {
	var v receipt.LockValue
	r.take(`{"name":`)
	v.Name = r.text(p, true)
	r.take(`,"value":`)
	v.Value = r.text(p, true)
	r.take(`}`)
	return v
}

func codecReadComponents(r *codecReader, p *codecPlan) []receipt.Component {
	values := codecReadArrayStart[receipt.Component](r, p, codecComponents)
	count := 0
	for r.arrayItem(count, 2, len(values)) {
		value := codecReadComponent(r, p, count)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecComponents, 72, count, len(values))
	return values
}

func codecReadTools(r *codecReader, p *codecPlan) []receipt.Tool {
	values := codecReadArrayStart[receipt.Tool](r, p, codecTools)
	count := 0
	for r.arrayItem(count, 1, len(values)) {
		value := codecReadTool(r, p, count)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecTools, 56, count, len(values))
	return values
}

func codecReadExecutables(r *codecReader, p *codecPlan) []receipt.Executable {
	values := codecReadArrayStart[receipt.Executable](r, p, codecExecutables)
	count := 0
	for r.arrayItem(count, 4, len(values)) {
		value := r.executable(p)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecExecutables, 64, count, len(values))
	return values
}

func codecReadObservedTools(r *codecReader, p *codecPlan) []receipt.ToolObservation {
	values := codecReadArrayStart[receipt.ToolObservation](r, p, codecTools)
	count := 0
	for r.arrayItem(count, 1, len(values)) {
		value := r.observedTool(p)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecTools, 64, count, len(values))
	return values
}

func codecReadObservations(r *codecReader, p *codecPlan, slot int) []receipt.DestinationObservation {
	values := codecReadArrayStart[receipt.DestinationObservation](r, p, slot)
	count := 0
	for r.arrayItem(count, 4096, len(values)) {
		value := r.destination(p)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, slot, 96, count, len(values))
	return values
}

func codecReadTexts(r *codecReader, p *codecPlan, slot int) []string {
	values := codecReadArrayStart[string](r, p, slot)
	count := 0
	for r.arrayItem(count, 4096, len(values)) {
		value := r.text(p, true)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, slot, 16, count, len(values))
	return values
}

func codecReadPhases(r *codecReader, p *codecPlan) []Phase {
	values := codecReadArrayStart[Phase](r, p, codecExecutables)
	count := 0
	for r.arrayItem(count, 2, len(values)) {
		value := Phase(r.text(p, true))
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecExecutables, 16, count, len(values))
	return values
}

func codecReadFiles(r *codecReader, p *codecPlan, slot int) []receipt.File {
	values := codecReadArrayStart[receipt.File](r, p, slot)
	count := 0
	for r.arrayItem(count, 4096, len(values)) {
		value := r.file(p)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, slot, 48, count, len(values))
	return values
}

func codecReadLocks(r *codecReader, p *codecPlan) []receipt.LockValue {
	values := codecReadArrayStart[receipt.LockValue](r, p, codecLocks)
	count := 0
	for r.arrayItem(count, 1, len(values)) {
		value := codecReadLock(r, p, count)
		if r.materialize {
			values[count] = value
		}
		count++
	}
	r.arrayEnd(p, codecLocks, 32, count, len(values))
	return values
}
