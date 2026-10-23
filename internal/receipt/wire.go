package receipt

// These closed-schema writers preserve ARCH-030 field order and default JSON escaping.
func (w *jsonWriter) source(v Source) {
	w.raw(`{"uri":`)
	w.string(v.URI)
	w.raw(`,"source_id":`)
	w.string(v.SourceID)
	w.raw(`,"commit":`)
	w.string(v.Commit)
	w.raw(`,"root_tree":`)
	w.string(v.RootTree)
	w.raw(`}`)
}
func (w *jsonWriter) file(v File) {
	w.raw(`{"path":`)
	w.string(v.Path)
	w.raw(`,"mode":`)
	w.string(v.Mode)
	w.raw(`,"hash":`)
	w.string(v.Hash)
	w.raw(`}`)
}
func (w *jsonWriter) component(v Component) {
	w.raw(`{"id":`)
	w.string(v.ID)
	w.raw(`,"path":`)
	w.string(v.Path)
	w.raw(`,"tree_digest":`)
	w.string(v.TreeDigest)
	w.raw(`,"files":`)
	w.raw(`[`)
	for i, item := range v.Files {
		if i != 0 {
			w.raw(`,`)
		}
		w.file(item)
	}
	w.raw(`]`)
	w.raw(`}`)
}
func (w *jsonWriter) lockValue(v LockValue) {
	w.raw(`{"name":`)
	w.string(v.Name)
	w.raw(`,"value":`)
	w.string(v.Value)
	w.raw(`}`)
}
func (w *jsonWriter) tool(v Tool) {
	w.raw(`{"id":`)
	w.string(v.ID)
	w.raw(`,"version":`)
	w.string(v.Version)
	w.raw(`,"locks":`)
	w.raw(`[`)
	for i, item := range v.Locks {
		if i != 0 {
			w.raw(`,`)
		}
		w.lockValue(item)
	}
	w.raw(`]`)
	w.raw(`}`)
}
func (w *jsonWriter) executable(v Executable) {
	w.raw(`{"id":`)
	w.string(v.ID)
	w.raw(`,"version":`)
	w.string(v.Version)
	w.raw(`,"path":`)
	w.string(v.Path)
	w.raw(`,"sha256":`)
	w.string(v.SHA256)
	w.raw(`}`)
}
func (w *jsonWriter) parser(v Parser) {
	w.raw(`{"schema":`)
	w.integer(v.Schema)
	w.raw(`,"module":`)
	w.string(v.Module)
	w.raw(`,"version":`)
	w.string(v.Version)
	w.raw(`,"module_sum":`)
	w.string(v.ModuleSum)
	w.raw(`,"go_mod_sum":`)
	w.string(v.GoModSum)
	w.raw(`,"origin_commit":`)
	w.string(v.OriginCommit)
	w.raw(`}`)
}
func (w *jsonWriter) policy(v Policy) {
	w.raw(`{"schema":`)
	w.integer(v.Schema)
	w.raw(`,"domain":`)
	w.string(v.Domain)
	w.raw(`,"object_modes":`)
	w.raw(`[`)
	for i, item := range v.ObjectModes {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"declarations":`)
	w.raw(`[`)
	for i, item := range v.Declarations {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"template_actions":`)
	w.raw(`[`)
	for i, item := range v.TemplateActions {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"process_classes":`)
	w.raw(`[`)
	for i, item := range v.ProcessClasses {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"policy_ids":`)
	w.raw(`[`)
	for i, item := range v.PolicyIDs {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"target_mapper_executable_id":`)
	w.string(v.TargetMapperExecutableID)
	w.raw(`,"provider_free":`)
	w.boolean(v.ProviderFree)
	w.raw(`,"host_fact_independent":`)
	w.boolean(v.HostFactIndependent)
	w.raw(`,"destination_recheck":`)
	w.boolean(v.DestinationRecheck)
	w.raw(`,"exact_reapply_only":`)
	w.boolean(v.ExactReapplyOnly)
	w.raw(`}`)
}
func (w *jsonWriter) candidate(v Candidate) {
	w.raw(`{"schema":`)
	w.integer(v.Schema)
	w.raw(`,"domain":`)
	w.string(v.Domain)
	w.raw(`,"source":`)
	w.source(v.Source)
	w.raw(`,"profile_id":`)
	w.string(v.ProfileID)
	w.raw(`,"components":`)
	w.raw(`[`)
	for i, item := range v.Components {
		if i != 0 {
			w.raw(`,`)
		}
		w.component(item)
	}
	w.raw(`]`)
	w.raw(`,"tools":`)
	w.raw(`[`)
	for i, item := range v.Tools {
		if i != 0 {
			w.raw(`,`)
		}
		w.tool(item)
	}
	w.raw(`]`)
	w.raw(`,"home_sources":`)
	w.raw(`[`)
	for i, item := range v.HomeSources {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"targets":`)
	w.raw(`[`)
	for i, item := range v.Targets {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"executables":`)
	w.raw(`[`)
	for i, item := range v.Executables {
		if i != 0 {
			w.raw(`,`)
		}
		w.executable(item)
	}
	w.raw(`]`)
	w.raw(`,"parser":`)
	w.parser(v.Parser)
	w.raw(`,"policy":`)
	w.policy(v.Policy)
	w.raw(`}`)
}
func (w *jsonWriter) destinationObservation(v DestinationObservation) {
	w.raw(`{"target":`)
	w.string(v.Target)
	w.raw(`,"exists":`)
	w.boolean(v.Exists)
	w.raw(`,"kind":`)
	w.string(v.Kind)
	w.raw(`,"mode":`)
	w.unsigned32(v.Mode)
	w.raw(`,"owner":`)
	w.unsigned32(v.Owner)
	w.raw(`,"device":`)
	w.unsigned(v.Device)
	w.raw(`,"inode":`)
	w.unsigned(v.Inode)
	w.raw(`,"links":`)
	w.unsigned(v.Links)
	w.raw(`,"content_hash":`)
	w.string(v.ContentHash)
	w.raw(`}`)
}
func (w *jsonWriter) toolObservation(v ToolObservation) {
	w.raw(`{"id":`)
	w.string(v.ID)
	w.raw(`,"requested":`)
	w.string(v.Requested)
	w.raw(`,"installed":`)
	w.string(v.Installed)
	w.raw(`,"integrity":`)
	w.string(v.Integrity)
	w.raw(`}`)
}
func (w *jsonWriter) platformObservation(v PlatformObservation) {
	w.raw(`{"distribution":`)
	w.string(v.Distribution)
	w.raw(`,"kernel":`)
	w.string(v.Kernel)
	w.raw(`,"architecture":`)
	w.string(v.Architecture)
	w.raw(`,"filesystem":`)
	w.string(v.Filesystem)
	w.raw(`}`)
}
func (w *jsonWriter) dimensions(v Dimensions) {
	w.raw(`{"inputs_pinned":`)
	w.boolean(v.InputsPinned)
	w.raw(`,"artifact_integrity":`)
	w.boolean(v.ArtifactIntegrity)
	w.raw(`,"artifact_available":`)
	w.boolean(v.ArtifactAvailable)
	w.raw(`,"host_compatible":`)
	w.boolean(v.HostCompatible)
	w.raw(`,"convergence_verified":`)
	w.boolean(v.ConvergenceVerified)
	w.raw(`}`)
}
func (w *jsonWriter) completed(v Completed) {
	w.raw(`{"schema":`)
	w.integer(v.Schema)
	w.raw(`,"domain":`)
	w.string(v.Domain)
	w.raw(`,"candidate":`)
	w.digest(v.Candidate)
	if v.Previous != "" {
		w.raw(`,"previous":`)
		w.digest(v.Previous)
	}
	w.raw(`,"source":`)
	w.source(v.Source)
	w.raw(`,"components":`)
	w.raw(`[`)
	for i, item := range v.Components {
		if i != 0 {
			w.raw(`,`)
		}
		w.component(item)
	}
	w.raw(`]`)
	w.raw(`,"zerb":`)
	w.executable(v.Zerb)
	w.raw(`,"git":`)
	w.executable(v.Git)
	w.raw(`,"mise":`)
	w.executable(v.Mise)
	w.raw(`,"chezmoi":`)
	w.executable(v.Chezmoi)
	w.raw(`,"parser":`)
	w.parser(v.Parser)
	w.raw(`,"policy":`)
	w.policy(v.Policy)
	w.raw(`,"platform":`)
	w.platformObservation(v.Platform)
	w.raw(`,"tools":`)
	w.raw(`[`)
	for i, item := range v.Tools {
		if i != 0 {
			w.raw(`,`)
		}
		w.toolObservation(item)
	}
	w.raw(`]`)
	w.raw(`,"targets":`)
	w.raw(`[`)
	for i, item := range v.Targets {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"preconditions":`)
	w.raw(`[`)
	for i, item := range v.Preconditions {
		if i != 0 {
			w.raw(`,`)
		}
		w.destinationObservation(item)
	}
	w.raw(`]`)
	w.raw(`,"results":`)
	w.raw(`[`)
	for i, item := range v.Results {
		if i != 0 {
			w.raw(`,`)
		}
		w.destinationObservation(item)
	}
	w.raw(`]`)
	w.raw(`,"phases":`)
	w.raw(`[`)
	for i, item := range v.Phases {
		if i != 0 {
			w.raw(`,`)
		}
		w.string(item)
	}
	w.raw(`]`)
	w.raw(`,"dimensions":`)
	w.dimensions(v.Dimensions)
	w.raw(`}`)
}
