package drift

import (
	"strings"
	"testing"
)

func TestFormatDriftReport(t *testing.T) {
	tests := []struct {
		name            string
		results         []DriftResult
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "Mixed drift types",
			results: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftVersionMismatch,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.5.0",
					ActiveVersion:   "20.5.0",
				},
				{
					Tool:            "python",
					DriftType:       DriftMissing,
					BaselineVersion: "3.12.1",
				},
				{
					Tool:            "ripgrep",
					DriftType:       DriftOK,
					BaselineVersion: "13.0.0",
					ManagedVersion:  "13.0.0",
					ActiveVersion:   "13.0.0",
				},
			},
			wantContains: []string{
				"DRIFT REPORT",
				"VERSION MISMATCH",
				"MISSING",
				"OK",
				"node",
				"python",
				"SUMMARY",
				"2 drifts detected",
			},
			wantNotContains: []string{"mise", "chezmoi"},
		},
		{
			name: "All OK - no drifts",
			results: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftOK,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.11.0",
					ActiveVersion:   "20.11.0",
				},
				{
					Tool:            "python",
					DriftType:       DriftOK,
					BaselineVersion: "3.12.1",
					ManagedVersion:  "3.12.1",
					ActiveVersion:   "3.12.1",
				},
			},
			wantContains: []string{
				"DRIFT REPORT",
				"OK",
				"2 tools match baseline",
				"No drifts detected",
			},
			wantNotContains: []string{"mise", "chezmoi"},
		},
		{
			name: "External override",
			results: []DriftResult{
				{
					Tool:            "python",
					DriftType:       DriftExternalOverride,
					BaselineVersion: "3.12.1",
					ManagedVersion:  "3.12.1",
					ActiveVersion:   "3.11.6",
					ActivePath:      "/usr/bin/python",
				},
			},
			wantContains: []string{
				"EXTERNAL OVERRIDE",
				"python",
				"3.12.1",
				"3.11.6",
				"/usr/bin/python",
				"external installation",
				"managed by ZERB",
			},
			wantNotContains: []string{"mise", "chezmoi"},
		},
		{
			name: "Extra tool",
			results: []DriftResult{
				{
					Tool:           "rust",
					DriftType:      DriftExtra,
					ManagedVersion: "1.75.0",
					ActiveVersion:  "1.75.0",
				},
			},
			wantContains: []string{
				"EXTRA",
				"rust",
				"not declared",
				"managed by ZERB",
			},
			wantNotContains: []string{"mise", "chezmoi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatDriftReport(tt.results)

			// Check for required strings
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("FormatDriftReport() output missing %q\nGot:\n%s", want, output)
				}
			}

			// Check for forbidden strings (internal terminology)
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(strings.ToLower(output), strings.ToLower(notWant)) {
					t.Errorf("FormatDriftReport() output contains forbidden term %q\nGot:\n%s", notWant, output)
				}
			}
		})
	}
}

func TestFormatDriftEntry_VersionMismatch(t *testing.T) {
	result := DriftResult{
		Tool:            "node",
		DriftType:       DriftVersionMismatch,
		BaselineVersion: "20.11.0",
		ManagedVersion:  "20.5.0",
		ActiveVersion:   "20.5.0",
	}

	output := formatDriftEntry(result)

	wantContains := []string{
		"VERSION MISMATCH",
		"node",
		"20.11.0",
		"20.5.0",
		"managed by ZERB",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("formatDriftEntry() missing %q\nGot:\n%s", want, output)
		}
	}

	// Verify no internal terminology
	if strings.Contains(strings.ToLower(output), "mise") {
		t.Error("formatDriftEntry() contains 'mise'")
	}
}

func TestFormatDriftEntry_ExternalOverride(t *testing.T) {
	result := DriftResult{
		Tool:            "python",
		DriftType:       DriftExternalOverride,
		BaselineVersion: "3.12.1",
		ManagedVersion:  "3.12.1",
		ActiveVersion:   "3.11.6",
		ActivePath:      "/usr/bin/python",
	}

	output := formatDriftEntry(result)

	wantContains := []string{
		"EXTERNAL OVERRIDE",
		"⚠",
		"python",
		"3.12.1",
		"3.11.6",
		"/usr/bin/python",
		"managed by ZERB",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("formatDriftEntry() missing %q\nGot:\n%s", want, output)
		}
	}
}

func TestFormatDriftEntry_Missing(t *testing.T) {
	result := DriftResult{
		Tool:            "go",
		DriftType:       DriftMissing,
		BaselineVersion: "1.22.0",
	}

	output := formatDriftEntry(result)

	wantContains := []string{
		"MISSING",
		"go",
		"1.22.0",
		"not installed",
		"Declared in baseline",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("formatDriftEntry() missing %q\nGot:\n%s", want, output)
		}
	}
}

func TestFormatDriftEntry_Extra(t *testing.T) {
	result := DriftResult{
		Tool:           "rust",
		DriftType:      DriftExtra,
		ManagedVersion: "1.75.0",
		ActiveVersion:  "1.75.0",
	}

	output := formatDriftEntry(result)

	wantContains := []string{
		"EXTRA",
		"rust",
		"1.75.0",
		"not declared",
		"managed by ZERB",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("formatDriftEntry() missing %q\nGot:\n%s", want, output)
		}
	}
}

func TestFormatDriftEntry_ManagedButNotActive(t *testing.T) {
	result := DriftResult{
		Tool:            "node",
		DriftType:       DriftManagedButNotActive,
		BaselineVersion: "20.11.0",
		ManagedVersion:  "20.11.0",
	}

	output := formatDriftEntry(result)

	wantContains := []string{
		"MANAGED BUT NOT ACTIVE",
		"node",
		"20.11.0",
		"not in PATH",
		"managed by ZERB",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Errorf("formatDriftEntry() missing %q\nGot:\n%s", want, output)
		}
	}
}
