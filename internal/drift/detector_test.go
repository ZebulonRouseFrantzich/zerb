package drift

import (
	"testing"
)

// Helper function for deep equality comparison of DriftResult structs
func driftResultsEqual(a, b DriftResult) bool {
	return a.Tool == b.Tool &&
		a.DriftType == b.DriftType &&
		a.BaselineVersion == b.BaselineVersion &&
		a.ManagedVersion == b.ManagedVersion &&
		a.ActiveVersion == b.ActiveVersion &&
		a.ActivePath == b.ActivePath
}

func TestDetectDrift(t *testing.T) {
	tests := []struct {
		name     string
		baseline []ToolSpec
		managed  []Tool
		active   []Tool
		zerbDir  string
		want     []DriftResult
	}{
		{
			name: "All in sync",
			baseline: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
			},
			managed: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
			},
			active: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftOK,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.11.0",
					ActiveVersion:   "20.11.0",
					ActivePath:      "/home/.config/zerb/installs/node/20.11.0/bin/node",
				},
			},
		},
		{
			name: "Version mismatch",
			baseline: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
			},
			managed: []Tool{
				{Name: "node", Version: "20.5.0", Path: "/home/.config/zerb/installs/node/20.5.0/bin/node"},
			},
			active: []Tool{
				{Name: "node", Version: "20.5.0", Path: "/home/.config/zerb/installs/node/20.5.0/bin/node"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftVersionMismatch,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.5.0",
					ActiveVersion:   "20.5.0",
					ActivePath:      "/home/.config/zerb/installs/node/20.5.0/bin/node",
				},
			},
		},
		{
			name: "External override",
			baseline: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
			},
			managed: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
			},
			active: []Tool{
				{Name: "node", Version: "20.15.0", Path: "/usr/bin/node"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftExternalOverride,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.11.0",
					ActiveVersion:   "20.15.0",
					ActivePath:      "/usr/bin/node",
				},
			},
		},
		{
			name: "Missing tool",
			baseline: []ToolSpec{
				{Name: "python", Version: "3.12.1"},
			},
			managed: []Tool{},
			active:  []Tool{},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "python",
					DriftType:       DriftMissing,
					BaselineVersion: "3.12.1",
					ManagedVersion:  "",
					ActiveVersion:   "",
					ActivePath:      "",
				},
			},
		},
		{
			name:     "Extra tool",
			baseline: []ToolSpec{},
			managed: []Tool{
				{Name: "rust", Version: "1.75.0", Path: "/home/.config/zerb/installs/rust/1.75.0/bin/rustc"},
			},
			active: []Tool{
				{Name: "rust", Version: "1.75.0", Path: "/home/.config/zerb/installs/rust/1.75.0/bin/rustc"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "rust",
					DriftType:       DriftExtra,
					BaselineVersion: "",
					ManagedVersion:  "1.75.0",
					ActiveVersion:   "1.75.0",
					ActivePath:      "/home/.config/zerb/installs/rust/1.75.0/bin/rustc",
				},
			},
		},
		{
			name: "Managed but not active",
			baseline: []ToolSpec{
				{Name: "go", Version: "1.22.0"},
			},
			managed: []Tool{
				{Name: "go", Version: "1.22.0", Path: "/home/.config/zerb/installs/go/1.22.0/bin/go"},
			},
			active:  []Tool{}, // Not in PATH
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "go",
					DriftType:       DriftManagedButNotActive,
					BaselineVersion: "1.22.0",
					ManagedVersion:  "1.22.0",
					ActiveVersion:   "",
					ActivePath:      "",
				},
			},
		},
		{
			name: "Version unknown",
			baseline: []ToolSpec{
				{Name: "mystery", Version: "1.0.0"},
			},
			managed: []Tool{
				{Name: "mystery", Version: "1.0.0", Path: "/home/.config/zerb/installs/mystery/1.0.0/bin/mystery"},
			},
			active: []Tool{
				{Name: "mystery", Version: "unknown", Path: "/home/.config/zerb/installs/mystery/1.0.0/bin/mystery"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "mystery",
					DriftType:       DriftVersionUnknown,
					BaselineVersion: "1.0.0",
					ManagedVersion:  "1.0.0",
					ActiveVersion:   "unknown",
					ActivePath:      "/home/.config/zerb/installs/mystery/1.0.0/bin/mystery",
				},
			},
		},
		{
			name: "Multiple tools mixed states",
			baseline: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
				{Name: "python", Version: "3.12.1"},
				{Name: "go", Version: "1.22.0"},
			},
			managed: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
				{Name: "python", Version: "3.11.0", Path: "/home/.config/zerb/installs/python/3.11.0/bin/python"},
				{Name: "rust", Version: "1.75.0", Path: "/home/.config/zerb/installs/rust/1.75.0/bin/rustc"},
			},
			active: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
				{Name: "python", Version: "3.12.1", Path: "/usr/bin/python"},
				{Name: "rust", Version: "1.75.0", Path: "/home/.config/zerb/installs/rust/1.75.0/bin/rustc"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftOK,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "20.11.0",
					ActiveVersion:   "20.11.0",
					ActivePath:      "/home/.config/zerb/installs/node/20.11.0/bin/node",
				},
				{
					Tool:            "python",
					DriftType:       DriftExternalOverride,
					BaselineVersion: "3.12.1",
					ManagedVersion:  "3.11.0",
					ActiveVersion:   "3.12.1",
					ActivePath:      "/usr/bin/python",
				},
				{
					Tool:            "go",
					DriftType:       DriftMissing,
					BaselineVersion: "1.22.0",
					ManagedVersion:  "",
					ActiveVersion:   "",
					ActivePath:      "",
				},
				{
					Tool:            "rust",
					DriftType:       DriftExtra,
					BaselineVersion: "",
					ManagedVersion:  "1.75.0",
					ActiveVersion:   "1.75.0",
					ActivePath:      "/home/.config/zerb/installs/rust/1.75.0/bin/rustc",
				},
			},
		},
		{
			name:     "Empty baseline",
			baseline: []ToolSpec{},
			managed: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
			},
			active: []Tool{
				{Name: "node", Version: "20.11.0", Path: "/home/.config/zerb/installs/node/20.11.0/bin/node"},
			},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftExtra,
					BaselineVersion: "",
					ManagedVersion:  "20.11.0",
					ActiveVersion:   "20.11.0",
					ActivePath:      "/home/.config/zerb/installs/node/20.11.0/bin/node",
				},
			},
		},
		{
			name: "Empty managed and active",
			baseline: []ToolSpec{
				{Name: "node", Version: "20.11.0"},
			},
			managed: []Tool{},
			active:  []Tool{},
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "node",
					DriftType:       DriftMissing,
					BaselineVersion: "20.11.0",
					ManagedVersion:  "",
					ActiveVersion:   "",
					ActivePath:      "",
				},
			},
		},
		{
			name:     "All empty",
			baseline: []ToolSpec{},
			managed:  []Tool{},
			active:   []Tool{},
			zerbDir:  "/home/.config/zerb",
			want:     []DriftResult{},
		},
		{
			name:     "Extra tool not in active",
			baseline: []ToolSpec{},
			managed: []Tool{
				{Name: "rust", Version: "1.75.0", Path: "/home/.config/zerb/installs/rust/1.75.0/bin/rustc"},
			},
			active:  []Tool{}, // Not in PATH
			zerbDir: "/home/.config/zerb",
			want: []DriftResult{
				{
					Tool:            "rust",
					DriftType:       DriftExtra,
					BaselineVersion: "",
					ManagedVersion:  "1.75.0",
					ActiveVersion:   "",
					ActivePath:      "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectDrift(tt.baseline, tt.managed, tt.active, tt.zerbDir)

			if len(got) != len(tt.want) {
				t.Errorf("DetectDrift() returned %d results, want %d", len(got), len(tt.want))
				t.Logf("got:  %+v", got)
				t.Logf("want: %+v", tt.want)
				return
			}

			// Compare results
			for i := range got {
				if !driftResultsEqual(got[i], tt.want[i]) {
					t.Errorf("DetectDrift()[%d] mismatch:\ngot:  %+v\nwant: %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClassifyDrift(t *testing.T) {
	zerbDir := "/home/.config/zerb"

	tests := []struct {
		name       string
		spec       ToolSpec
		managed    Tool
		hasManaged bool
		active     Tool
		hasActive  bool
		want       DriftType
	}{
		{
			name:       "Missing - not in managed or active",
			spec:       ToolSpec{Name: "tool", Version: "1.0.0"},
			hasManaged: false,
			hasActive:  false,
			want:       DriftMissing,
		},
		{
			name:       "Managed but not active",
			spec:       ToolSpec{Name: "tool", Version: "1.0.0"},
			managed:    Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasManaged: true,
			hasActive:  false,
			want:       DriftManagedButNotActive,
		},
		{
			name:       "External override",
			spec:       ToolSpec{Name: "tool", Version: "1.0.0"},
			managed:    Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasManaged: true,
			active:     Tool{Name: "tool", Version: "1.0.0", Path: "/usr/bin/tool"},
			hasActive:  true,
			want:       DriftExternalOverride,
		},
		{
			name:       "Version unknown",
			spec:       ToolSpec{Name: "tool", Version: "1.0.0"},
			managed:    Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasManaged: true,
			active:     Tool{Name: "tool", Version: "unknown", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasActive:  true,
			want:       DriftVersionUnknown,
		},
		{
			name:       "Version mismatch",
			spec:       ToolSpec{Name: "tool", Version: "2.0.0"},
			managed:    Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasManaged: true,
			active:     Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasActive:  true,
			want:       DriftVersionMismatch,
		},
		{
			name:       "All OK",
			spec:       ToolSpec{Name: "tool", Version: "1.0.0"},
			managed:    Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasManaged: true,
			active:     Tool{Name: "tool", Version: "1.0.0", Path: "/home/.config/zerb/installs/tool/1.0.0/bin/tool"},
			hasActive:  true,
			want:       DriftOK,
		},
		{
			name:       "Empty version strings - OK",
			spec:       ToolSpec{Name: "tool", Version: ""},
			managed:    Tool{Name: "tool", Version: "", Path: "/home/.config/zerb/installs/tool/bin/tool"},
			hasManaged: true,
			active:     Tool{Name: "tool", Version: "", Path: "/home/.config/zerb/installs/tool/bin/tool"},
			hasActive:  true,
			want:       DriftOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyDrift(tt.spec, tt.managed, tt.hasManaged, tt.active, tt.hasActive, zerbDir)
			if got != tt.want {
				t.Errorf("classifyDrift() = %v (%s), want %v (%s)", got, got.String(), tt.want, tt.want.String())
			}
		})
	}
}

func TestDriftResultsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    DriftResult
		b    DriftResult
		want bool
	}{
		{
			name: "Equal results",
			a: DriftResult{
				Tool:            "node",
				DriftType:       DriftOK,
				BaselineVersion: "20.11.0",
				ManagedVersion:  "20.11.0",
				ActiveVersion:   "20.11.0",
				ActivePath:      "/path/to/node",
			},
			b: DriftResult{
				Tool:            "node",
				DriftType:       DriftOK,
				BaselineVersion: "20.11.0",
				ManagedVersion:  "20.11.0",
				ActiveVersion:   "20.11.0",
				ActivePath:      "/path/to/node",
			},
			want: true,
		},
		{
			name: "Different tool names",
			a: DriftResult{
				Tool:      "node",
				DriftType: DriftOK,
			},
			b: DriftResult{
				Tool:      "python",
				DriftType: DriftOK,
			},
			want: false,
		},
		{
			name: "Different drift types",
			a: DriftResult{
				Tool:      "node",
				DriftType: DriftOK,
			},
			b: DriftResult{
				Tool:      "node",
				DriftType: DriftMissing,
			},
			want: false,
		},
		{
			name: "Different versions",
			a: DriftResult{
				Tool:            "node",
				DriftType:       DriftOK,
				BaselineVersion: "20.11.0",
			},
			b: DriftResult{
				Tool:            "node",
				DriftType:       DriftOK,
				BaselineVersion: "20.5.0",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := driftResultsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("driftResultsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectDrift_Integration(t *testing.T) {
	// This is a comprehensive integration test that tests the full drift detection workflow
	// with realistic data that could come from QueryBaseline, QueryManaged, and QueryActive

	zerbDir := "/home/user/.config/zerb"

	// Simulate a realistic scenario:
	// - Baseline declares: node@20.11.0, python@3.12.1, go@1.22.0, ripgrep@13.0.0
	// - ZERB has installed: node@20.11.0, python@3.11.0 (wrong version), ripgrep@13.0.0, rust@1.75.0 (extra)
	// - Active in PATH: node@20.11.0 (ZERB), python@3.12.1 (system), ripgrep@13.0.0 (ZERB)
	// - go is missing entirely
	// - rust is extra (not in baseline)

	baseline := []ToolSpec{
		{Name: "node", Version: "20.11.0"},
		{Name: "python", Version: "3.12.1"},
		{Name: "go", Version: "1.22.0"},
		{Name: "ripgrep", Version: "13.0.0"},
	}

	managed := []Tool{
		{Name: "node", Version: "20.11.0", Path: zerbDir + "/installs/node/20.11.0/bin/node"},
		{Name: "python", Version: "3.11.0", Path: zerbDir + "/installs/python/3.11.0/bin/python"},
		{Name: "ripgrep", Version: "13.0.0", Path: zerbDir + "/installs/ripgrep/13.0.0/bin/rg"},
		{Name: "rust", Version: "1.75.0", Path: zerbDir + "/installs/rust/1.75.0/bin/rustc"},
	}

	active := []Tool{
		{Name: "node", Version: "20.11.0", Path: zerbDir + "/installs/node/20.11.0/bin/node"},
		{Name: "python", Version: "3.12.1", Path: "/usr/bin/python"},
		{Name: "ripgrep", Version: "13.0.0", Path: zerbDir + "/installs/ripgrep/13.0.0/bin/rg"},
	}

	results := DetectDrift(baseline, managed, active, zerbDir)

	// Verify we got 5 results (4 baseline + 1 extra)
	if len(results) != 5 {
		t.Fatalf("Expected 5 drift results, got %d", len(results))
	}

	// Check each result
	expectedResults := map[string]DriftType{
		"node":    DriftOK,               // Everything matches
		"python":  DriftExternalOverride, // System python overriding ZERB's wrong version
		"go":      DriftMissing,          // Not installed anywhere
		"ripgrep": DriftOK,               // Everything matches
		"rust":    DriftExtra,            // ZERB installed but not in baseline
	}

	resultsByTool := make(map[string]DriftResult)
	for _, r := range results {
		resultsByTool[r.Tool] = r
	}

	for tool, expectedType := range expectedResults {
		result, exists := resultsByTool[tool]
		if !exists {
			t.Errorf("Expected drift result for %s, but not found", tool)
			continue
		}

		if result.DriftType != expectedType {
			t.Errorf("Tool %s: expected drift type %s, got %s", tool, expectedType.String(), result.DriftType.String())
		}
	}

	// Specific checks for interesting cases
	pythonResult := resultsByTool["python"]
	if pythonResult.DriftType != DriftExternalOverride {
		t.Errorf("Python should be DriftExternalOverride, got %s", pythonResult.DriftType.String())
	}
	if pythonResult.BaselineVersion != "3.12.1" {
		t.Errorf("Python baseline version should be 3.12.1, got %s", pythonResult.BaselineVersion)
	}
	if pythonResult.ManagedVersion != "3.11.0" {
		t.Errorf("Python managed version should be 3.11.0, got %s", pythonResult.ManagedVersion)
	}
	if pythonResult.ActiveVersion != "3.12.1" {
		t.Errorf("Python active version should be 3.12.1, got %s", pythonResult.ActiveVersion)
	}
	if pythonResult.ActivePath != "/usr/bin/python" {
		t.Errorf("Python active path should be /usr/bin/python, got %s", pythonResult.ActivePath)
	}

	rustResult := resultsByTool["rust"]
	if rustResult.DriftType != DriftExtra {
		t.Errorf("Rust should be DriftExtra, got %s", rustResult.DriftType.String())
	}
	if rustResult.BaselineVersion != "" {
		t.Errorf("Rust baseline version should be empty, got %s", rustResult.BaselineVersion)
	}
	if rustResult.ManagedVersion != "1.75.0" {
		t.Errorf("Rust managed version should be 1.75.0, got %s", rustResult.ManagedVersion)
	}
}
