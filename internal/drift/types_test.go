package drift

import "testing"

func TestDriftType_String(t *testing.T) {
	tests := []struct {
		name string
		dt   DriftType
		want string
	}{
		{"OK", DriftOK, "OK"},
		{"Version Mismatch", DriftVersionMismatch, "VERSION_MISMATCH"},
		{"Missing", DriftMissing, "MISSING"},
		{"Extra", DriftExtra, "EXTRA"},
		{"External Override", DriftExternalOverride, "EXTERNAL_OVERRIDE"},
		{"Managed But Not Active", DriftManagedButNotActive, "MANAGED_BUT_NOT_ACTIVE"},
		{"Version Unknown", DriftVersionUnknown, "VERSION_UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dt.String()
			if got != tt.want {
				t.Errorf("DriftType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
