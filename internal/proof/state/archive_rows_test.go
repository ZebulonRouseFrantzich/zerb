package state

import (
	"os"
	"path/filepath"
	"testing"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

// VAL-028: local archive rows exercise integrity, publication and every failure prefix.
// This fixture deliberately does not invoke or qualify btrfs root admission.
func TestArchiveRowsRetainAllBoundaries(t *testing.T) {
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "outside-sentinel"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "outside-sentinel", "sentinel"), []byte("archive outside sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := directory.Close(); err != nil {
			t.Error(err)
		}
	})
	confined, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := confined.Close(); err != nil {
			t.Error(err)
		}
	})
	root := &prooffs.OwnedRoot{Path: path, Dir: directory, Root: confined}
	rows := make(map[string]Scenario)
	runArchives(root, func(row Scenario) {
		if _, duplicate := rows[row.ID]; duplicate {
			t.Errorf("duplicate archive row %s", row.ID)
		}
		rows[row.ID] = row
		if row.Status != Observed {
			t.Errorf("archive row failed: %s: %s", row.ID, row.Observation)
		}
	})
	for _, name := range []string{"valid", "regular-type-alias", "unsupported-metadata", "integrity-mismatch", "archive-mutated-independent-of-provenance", "provenance-mutated-independent-of-archive", "failure-prefixes", "no-replace", "sentinel-cleanup"} {
		if _, ok := rows["VAL-028/archive/"+name]; !ok {
			t.Errorf("missing archive boundary row %s", name)
		}
	}
	if _, ok := rows["VAL-028/parser-consumption"]; !ok {
		t.Fatal("missing aggregate parser-consumption row")
	}
	t.Logf("archive rows verified: %d", len(rows))
}
