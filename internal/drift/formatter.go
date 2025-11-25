package drift

import (
	"fmt"
	"strings"
)

// FormatDriftReport formats drift results for user display
func FormatDriftReport(results []DriftResult) string {
	var sb strings.Builder
	// Pre-allocate for typical report size (header + entries + summary)
	sb.Grow(1024 + len(results)*256)

	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("DRIFT REPORT\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Count drift types
	counts := make(map[DriftType]int)
	for _, r := range results {
		counts[r.DriftType]++
	}

	// Display each drift (skip OK entries in detailed view)
	for _, r := range results {
		if r.DriftType == DriftOK {
			continue
		}

		sb.WriteString(formatDriftEntry(r))
		sb.WriteString("\n")
	}

	// Display OK entries summary
	okCount := counts[DriftOK]
	if okCount > 0 {
		sb.WriteString(fmt.Sprintf("[OK] ✓\n  %d tools match baseline\n\n", okCount))
	}

	// Summary
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	totalDrifts := len(results) - okCount
	if totalDrifts == 0 {
		sb.WriteString("SUMMARY: No drifts detected ✓\n")
	} else {
		sb.WriteString(fmt.Sprintf("SUMMARY: %d drifts detected\n", totalDrifts))

		var parts []string
		if counts[DriftExternalOverride] > 0 {
			parts = append(parts, fmt.Sprintf("%d external override", counts[DriftExternalOverride]))
		}
		if counts[DriftVersionMismatch] > 0 {
			parts = append(parts, fmt.Sprintf("%d version mismatch", counts[DriftVersionMismatch]))
		}
		if counts[DriftMissing] > 0 {
			parts = append(parts, fmt.Sprintf("%d missing", counts[DriftMissing]))
		}
		if counts[DriftExtra] > 0 {
			parts = append(parts, fmt.Sprintf("%d extra", counts[DriftExtra]))
		}
		if counts[DriftManagedButNotActive] > 0 {
			parts = append(parts, fmt.Sprintf("%d managed but not active", counts[DriftManagedButNotActive]))
		}
		if counts[DriftVersionUnknown] > 0 {
			parts = append(parts, fmt.Sprintf("%d version unknown", counts[DriftVersionUnknown]))
		}

		sb.WriteString("  " + strings.Join(parts, ", ") + "\n")
	}

	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return sb.String()
}

// formatDriftEntry formats a single drift entry
func formatDriftEntry(r DriftResult) string {
	var sb strings.Builder
	// Pre-allocate for typical entry size
	sb.Grow(512)

	switch r.DriftType {
	case DriftExternalOverride:
		sb.WriteString("[EXTERNAL OVERRIDE] ⚠️\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString(fmt.Sprintf("    Baseline:  %s (managed by ZERB)\n", r.BaselineVersion))
		sb.WriteString(fmt.Sprintf("    Active:    %s at %s\n", r.ActiveVersion, r.ActivePath))
		sb.WriteString("    \n")
		sb.WriteString("    → An external installation has taken precedence over ZERB\n")

	case DriftVersionMismatch:
		sb.WriteString("[VERSION MISMATCH]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString(fmt.Sprintf("    Baseline:  %s\n", r.BaselineVersion))
		sb.WriteString(fmt.Sprintf("    Active:    %s (managed by ZERB)\n", r.ActiveVersion))
		sb.WriteString("    \n")
		sb.WriteString("    → ZERB is managing this tool but the version doesn't match\n")

	case DriftMissing:
		sb.WriteString("[MISSING]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString(fmt.Sprintf("    Baseline:  %s\n", r.BaselineVersion))
		sb.WriteString("    Active:    (not installed)\n")
		sb.WriteString("    \n")
		sb.WriteString("    → Declared in baseline but not found\n")

	case DriftExtra:
		sb.WriteString("[EXTRA]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString("    Baseline:  (not declared)\n")
		sb.WriteString(fmt.Sprintf("    Active:    %s (managed by ZERB)\n", r.ActiveVersion))
		sb.WriteString("    \n")
		sb.WriteString("    → Tool is installed but not in baseline\n")

	case DriftManagedButNotActive:
		sb.WriteString("[MANAGED BUT NOT ACTIVE]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString(fmt.Sprintf("    Baseline:  %s\n", r.BaselineVersion))
		sb.WriteString(fmt.Sprintf("    Managed:   %s (managed by ZERB)\n", r.ManagedVersion))
		sb.WriteString("    Active:    (not in PATH)\n")
		sb.WriteString("    \n")
		sb.WriteString("    → ZERB has installed this tool but it's not accessible\n")

	case DriftVersionUnknown:
		sb.WriteString("[VERSION UNKNOWN]\n")
		sb.WriteString(fmt.Sprintf("  %s\n", r.Tool))
		sb.WriteString(fmt.Sprintf("    Baseline:  %s\n", r.BaselineVersion))
		if r.ActivePath != "" {
			sb.WriteString(fmt.Sprintf("    Active:    (version unknown) at %s\n", r.ActivePath))
		} else {
			sb.WriteString("    Active:    (version unknown)\n")
		}
		sb.WriteString("    \n")
		sb.WriteString("    → Version could not be detected\n")
	}

	return sb.String()
}
