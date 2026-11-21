//go:build linux

package state

import (
	"context"
	"slices"
	"testing"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

// VAL-031/034: every pre-transfer authority substitution is detected before convergence.
func TestRecoveryPreTransferAuthorityMatrix(t *testing.T) {
	verified, total, failures := runRecoveryAuthoritySubstitutionMatrix(context.Background(), Config{}, &prooffs.OwnedRoot{Path: t.TempDir()})
	if verified != 31 || total != 31 || len(failures) != 0 {
		t.Fatalf("recovery authority matrix: verified=%d/%d failures=%v", verified, total, failures)
	}
}

// VAL-025/031/034: takeover, cleanup, completion, and root substitutions all fail closed.
func TestInitializationAuthorityMatrix(t *testing.T) {
	verified, total, failures := runInitializationAuthoritySubstitutionMatrix(&prooffs.OwnedRoot{Path: t.TempDir()})
	if verified != 122 || total != 122 || len(failures) != 0 {
		t.Fatalf("initialization authority matrix: verified=%d/%d failures=%v", verified, total, failures)
	}
}

// VAL-025/030: local races retain all rows and their exact scenario order.
func TestLockRaceMatrix(t *testing.T) {
	var rows []Scenario
	runLockRaces(context.Background(), Config{}, &prooffs.OwnedRoot{Path: t.TempDir()}, func(row Scenario) { rows = append(rows, row) })
	want := []string{
		"VAL-030/locks/init-simultaneous-race", "VAL-030/locks/admission-simultaneous-race", "VAL-030/locks/writer-simultaneous-race",
		"VAL-025/two-initializers", "VAL-030/owner/stale-compare-transfer", "VAL-030/owner/live-process-identity",
		"VAL-030/locks/path-inode-substitution", "VAL-030/ABA", "VAL-030/owner/generation-overflow",
		"VAL-030/owner/pid-reuse", "VAL-030/owner/two-stale-reclaimers",
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.ID)
		expected := Observed
		if row.ID == "VAL-030/owner/pid-reuse" {
			expected = Failed
		}
		if row.Status != expected {
			t.Errorf("%s status=%s; want %s", row.ID, row.Status, expected)
		}
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("lock rows=%v; want %v", ids, want)
	}
}
