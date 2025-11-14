package drift

import (
	"testing"
)

func TestResolutionMode_String(t *testing.T) {
	tests := []struct {
		mode ResolutionMode
		want string
	}{
		{ResolutionIndividual, "Individual"},
		{ResolutionAdoptAll, "Adopt All"},
		{ResolutionRevertAll, "Revert All"},
		{ResolutionShowOnly, "Show Only"},
		{ResolutionExit, "Exit"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("ResolutionMode.String() = %q, want %q", got, tt.want)
			}
			if got == "" {
				t.Error("ResolutionMode.String() returned empty string")
			}
		})
	}
}

func TestDriftAction_String(t *testing.T) {
	tests := []struct {
		action DriftAction
		want   string
	}{
		{ActionAdopt, "Adopt"},
		{ActionRevert, "Revert"},
		{ActionSkip, "Skip"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.action.String()
			if got != tt.want {
				t.Errorf("DriftAction.String() = %q, want %q", got, tt.want)
			}
			if got == "" {
				t.Error("DriftAction.String() returned empty string")
			}
		})
	}
}

// Note: PromptResolutionMode and PromptDriftAction require manual testing
// or stdin mocking, which is beyond the scope of unit tests.
// These functions are tested manually and in integration tests.
