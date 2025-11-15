package drift

// DetectDrift performs three-way comparison of baseline, managed, and active tools
// and returns drift results for each tool.
//
// The algorithm:
//  1. Build lookup maps for managed and active tools (O(1) access)
//  2. Iterate through baseline tools, classify each drift, remove from maps
//  3. Iterate through remaining managed tools (extras not in baseline)
//  4. Return all drift results
//
// Parameters:
//   - baseline: Tools declared in zerb.lua configuration
//   - managed: Tools installed by ZERB (from QueryManaged)
//   - active: Tools found in PATH (from QueryActive)
//   - zerbDir: ZERB directory path (e.g., ~/.config/zerb) for path detection
//
// Returns: Slice of DriftResult, one per tool (baseline tools + extras)
func DetectDrift(baseline []ToolSpec, managed []Tool, active []Tool, zerbDir string) []DriftResult {
	var results []DriftResult

	// Build lookup maps for O(1) access
	managedMap := make(map[string]Tool)
	for _, t := range managed {
		managedMap[t.Name] = t
	}

	activeMap := make(map[string]Tool)
	for _, t := range active {
		activeMap[t.Name] = t
	}

	// Process each baseline tool
	for _, spec := range baseline {
		result := DriftResult{
			Tool:            spec.Name,
			BaselineVersion: spec.Version,
		}

		// Look up tool in managed and active maps
		managedTool, hasManaged := managedMap[spec.Name]
		activeTool, hasActive := activeMap[spec.Name]

		// Populate version info
		if hasManaged {
			result.ManagedVersion = managedTool.Version
		}

		if hasActive {
			result.ActiveVersion = activeTool.Version
			result.ActivePath = activeTool.Path
		}

		// Classify drift type
		result.DriftType = classifyDrift(spec, managedTool, hasManaged, activeTool, hasActive, zerbDir)

		results = append(results, result)

		// Remove from maps to detect extras later
		delete(managedMap, spec.Name)
		delete(activeMap, spec.Name)
	}

	// Process extra tools (in managed but not in baseline)
	for name, tool := range managedMap {
		result := DriftResult{
			Tool:           name,
			DriftType:      DriftExtra,
			ManagedVersion: tool.Version,
		}

		// Check if also in active (extra might not be in PATH)
		if activeTool, exists := activeMap[name]; exists {
			result.ActiveVersion = activeTool.Version
			result.ActivePath = activeTool.Path
		}

		results = append(results, result)
	}

	return results
}

// classifyDrift determines the drift type based on three-way comparison.
//
// Decision tree (priority order - first match wins):
//  1. Missing: Not in managed or active
//  2. ManagedButNotActive: ZERB has it but not in PATH
//  3. ExternalOverride: Active is not ZERB-managed
//  4. VersionUnknown: Version detection failed
//  5. VersionMismatch: ZERB managing wrong version
//  6. OK: Everything matches
//  7. Default: VersionMismatch (fallback)
//
// Parameters:
//   - spec: Baseline tool specification
//   - managed: Managed tool (if exists)
//   - hasManaged: Whether tool exists in managed map
//   - active: Active tool (if exists)
//   - hasActive: Whether tool exists in active map
//   - zerbDir: ZERB directory for path detection
//
// Returns: DriftType classification
func classifyDrift(spec ToolSpec, managed Tool, hasManaged bool, active Tool, hasActive bool, zerbDir string) DriftType {
	// 1. Missing: Not in managed or active
	if !hasManaged && !hasActive {
		return DriftMissing
	}

	// 2. Managed but not active: ZERB has it but not in PATH
	if hasManaged && !hasActive {
		return DriftManagedButNotActive
	}

	// 3. External override: Active is not ZERB-managed
	if hasActive && !IsZERBManaged(active.Path, zerbDir) {
		return DriftExternalOverride
	}

	// 4. Version unknown: Version detection failed
	if hasActive && active.Version == "unknown" {
		return DriftVersionUnknown
	}

	// 5. Version mismatch: ZERB managing wrong version
	if hasManaged && managed.Version != spec.Version {
		return DriftVersionMismatch
	}

	// 6. All OK: Everything matches
	if hasManaged && hasActive &&
		managed.Version == spec.Version &&
		active.Version == spec.Version &&
		IsZERBManaged(active.Path, zerbDir) {
		return DriftOK
	}

	// 7. Default fallback (should rarely hit this)
	return DriftVersionMismatch
}
