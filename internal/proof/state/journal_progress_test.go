//go:build linux

package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ARCH-024/025/026: these fixture observations cover targets independently of
// execution checkpoints; unchanged tool-stage targets do not prove a no-op ran.
func TestJournalProgressAndLatestObservations(t *testing.T) {
	r := freshGraphRefs()
	facts := receiptFactsFor(r.b, receiptDigest(r.a))
	prefixes := [][]phase{
		{}, {phaseToolInstall}, {phaseHomeApply}, {phaseToolInstall, phaseHomeApply},
		{phaseHomeApply, phaseToolInstall}, {phaseToolInstall, phaseToolInstall},
		{phaseToolInstall, phaseHomeApply, phaseHomeApply},
	}
	for _, current := range []phase{phasePrepared, phaseToolInstall, phaseHomeApply, phasePending, phaseCleanup, "unsupported"} {
		for index, completed := range prefixes {
			journal := transitionJournalRecord(r, string(current), string(current), nil, r.generation, r.writerToken)
			journal.CompletedPhases = append([]phase{}, completed...)
			full := index == 3
			if full {
				journal.Results = append([]receiptDestinationObservation{}, facts.Results...)
			}
			want := current == phasePrepared && index == 0 ||
				current == phaseToolInstall && (index == 0 || index == 1) ||
				current == phaseHomeApply && (index == 1 || full) ||
				(current == phasePending || current == phaseCleanup) && full
			if got := validJournalPrefix(journal); got != want {
				t.Fatalf("phase=%s completed=%v: got=%t want=%t", current, completed, got, want)
			}
		}
	}
	for _, current := range []struct {
		phase     phase
		completed []string
	}{
		{phasePrepared, nil},
		{phaseToolInstall, nil},
		{phaseToolInstall, []string{string(phaseToolInstall)}},
		{phaseHomeApply, []string{string(phaseToolInstall)}},
		{phaseHomeApply, []string{string(phaseToolInstall), string(phaseHomeApply)}},
		{phasePending, []string{string(phaseToolInstall), string(phaseHomeApply)}},
		{phaseCleanup, []string{string(phaseToolInstall), string(phaseHomeApply)}},
	} {
		journal := transitionJournalRecord(r, string(current.phase), string(current.phase), current.completed, r.generation, r.writerToken)
		want := facts.Preconditions
		if len(current.completed) == 2 {
			want = facts.Results
		}
		if !validJournalPrefix(journal) || !slicesEqual(journal.Preconditions, facts.Preconditions) || !slicesEqual(journal.Results, want) {
			t.Fatalf("fixture lost latest target observations: phase=%s completed=%v", current.phase, current.completed)
		}
		for _, field := range []string{"preconditions", "results"} {
			for _, invalid := range []string{"empty", "duplicate", "wrong target", "unsupported kind"} {
				changed := journal
				observations := &changed.Preconditions
				if field == "results" {
					observations = &changed.Results
				}
				*observations = append([]receiptDestinationObservation{}, (*observations)...)
				switch invalid {
				case "empty":
					*observations = []receiptDestinationObservation{}
				case "duplicate":
					*observations = append(*observations, (*observations)[0])
				case "wrong target":
					(*observations)[0].Target = "other/target"
				case "unsupported kind":
					(*observations)[0].Kind = "symlink"
				}
				if validJournalPrefix(changed) {
					t.Fatalf("phase=%s admitted %s %s", current.phase, invalid, field)
				}
			}
		}
	}
}

// ARCH-026: availability is independently observed. Each variant publishes its
// own canonical receipt/digest and matching current once, without rewriting it.
func TestStableReceiptCompletionDimensions(t *testing.T) {
	for _, available := range []bool{false, true} {
		for _, missing := range []string{"", "inputs", "integrity", "host", "convergence"} {
			s, d, r := makeGraph(t.TempDir(), "READY(∅)")
			facts := receiptFactsFor(r.b, "")
			facts.Dimensions.ArtifactAvailable = available
			switch missing {
			case "inputs":
				facts.Dimensions.InputsPinned = false
			case "integrity":
				facts.Dimensions.ArtifactIntegrity = false
			case "host":
				facts.Dimensions.HostCompatible = false
			case "convergence":
				facts.Dimensions.ConvergenceVerified = false
			}
			receipt := pendingReceiptRecord{Schema: 1, Domain: "zerb.receipt", Digest: receiptDigest("sha256:" + recordDigest(facts)), Facts: facts}
			current := stableCurrent(r.instance, r.b, string(receipt.Digest), r.journal)
			receiptPath := filepath.Join(s, "receipts", string(receipt.Digest)+".json")
			for _, publication := range []struct {
				path  string
				value any
			}{
				{filepath.Join(s, "candidates", r.b+".json"), candidateReference(r.instance, r.b)},
				{receiptPath, receipt},
				{filepath.Join(s, "current.json"), current},
				{filepath.Join(s, "writer-owner.json"), releasedWriter(r.instance, "", 2, 1, current.WriterToken)},
			} {
				if err := writeRecord(publication.path, publication.value); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(s, "current.json"))
			if err != nil {
				t.Fatal(err)
			}
			want := missing == ""
			if validCompletedReceipt(facts, r.b, "") != want || validPendingReceiptFor(receiptPath, r.b, "") != want {
				t.Fatalf("availability=%t missing=%q: wrong receipt completion", available, missing)
			}
			got := classify(s, d)
			if want && got != "STABLE(A)" || !want && !strings.HasPrefix(got, "BAD") {
				t.Fatalf("availability=%t missing=%q: classification=%q", available, missing, got)
			}
			if !currentDigestsMatch(s, current) || !exactRecordBytes(filepath.Join(s, "current.json"), current) {
				t.Fatal("fixture receipt digest/current relation did not remain exact")
			}
			after, err := os.ReadFile(filepath.Join(s, "current.json"))
			if err != nil || string(before) != string(after) {
				t.Fatal("read-only classification changed committed current")
			}
			if !available && want && receipt.Digest == pendingReceiptFor(r.b, "").Digest {
				t.Fatal("changed availability reused a different canonical receipt digest")
			}
		}
	}
}
