package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ARCH-024/025; VAL-031/032: digest reuse cannot hide the atomic commit boundary.
func TestCurrentIdentitySeparatesPreJournalAndCommittedCleanup(t *testing.T) {
	for _, kind := range []string{"bootstrap", "replacing", "exact-reapply", "multi-generation-exact-reapply"} {
		for _, stage := range []string{"writer-owner-held-publication", "journal-pending-receipt-publication", "current-publication", "cleanup-journal-remove", "writer-owner-released-publication"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				s, d, external, r, err := prepareTransitionRecoveryFixture(t.TempDir(), kind, stage)
				if err != nil {
					t.Fatal(err)
				}
				want := "CLEANUP(B)"
				switch stage {
				case "writer-owner-held-publication":
					want = "BAD corrupt"
				case "journal-pending-receipt-publication":
					want = "TRANSITION(A,B)"
					if kind == "bootstrap" {
						want = "TRANSITION(∅,B)"
					}
				case "writer-owner-released-publication":
					want = "STABLE(A)"
				}
				if got := classify(s, d); got != want {
					t.Fatalf("classification=%q want=%q", got, want)
				}
				got, err := recognizeTransitionPrefix(s, d, external, r)
				if err != nil || got != stage {
					t.Fatalf("stage=%q err=%v want=%q", got, err, stage)
				}
				if stage == "current-publication" {
					var current currentRecord
					if err = readRecord(filepath.Join(s, "current.json"), &current); err != nil {
						t.Fatal(err)
					}
					if current != transitionCurrent(r) || !validCurrentIdentity(current, r.instance) {
						t.Fatal("current did not bind the committing operation and writer")
					}
					if kind == "multi-generation-exact-reapply" && (current.Candidate != baselineCurrent(r).Candidate || current.Receipt != baselineCurrent(r).Receipt || current == baselineCurrent(r)) {
						t.Fatal("exact digest reuse did not produce a distinct commit identity")
					}
				}
			})
		}
	}
}

// ARCH-024; VAL-034: identity grammar and cross-record mismatches fail closed.
func TestCurrentIdentityRejectsMalformedAndMismatchedRecords(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*currentRecord)
	}{
		{"instance", func(c *currentRecord) { c.Instance = strings.Repeat("a", 32) }},
		{"missing-instance", func(c *currentRecord) { c.Instance = "" }},
		{"missing-operation", func(c *currentRecord) { c.OperationID = "" }},
		{"operation-token", func(c *currentRecord) { c.OperationToken = strings.Repeat("a", 32) }},
		{"operation-id", func(c *currentRecord) { c.OperationID = strings.Repeat("a", 32) }},
		{"uppercase-token", func(c *currentRecord) { c.OperationToken = strings.Repeat("A", 32) }},
		{"short-token", func(c *currentRecord) { c.WriterToken = "a" }},
		{"writer-token", func(c *currentRecord) { c.WriterToken = strings.Repeat("a", 32) }},
		{"zero-generation", func(c *currentRecord) { c.WriterGeneration = 0 }},
		{"future-generation", func(c *currentRecord) { c.WriterGeneration++ }},
		{"reused-operation-token", func(c *currentRecord) { c.OperationToken = c.WriterToken }},
		{"reused-operation-id", func(c *currentRecord) { c.OperationID = c.OperationToken }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			s, d, external, r, err := prepareTransitionRecoveryFixture(t.TempDir(), "multi-generation-exact-reapply", "current-publication")
			if err != nil {
				t.Fatal(err)
			}
			current := transitionCurrent(r)
			test.change(&current)
			if err = writeRecord(filepath.Join(s, "current.json"), current); err != nil {
				t.Fatal(err)
			}
			before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err != nil {
				t.Fatal(err)
			}
			if got := classify(s, d); !strings.HasPrefix(got, "BAD") {
				t.Fatalf("mismatched identity admitted as %q", got)
			}
			if _, err = transitionCommandRouter(s, d, external, r, "recover-repair"); err == nil {
				t.Fatal("mismatched identity recovered")
			}
			after, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err != nil || !fixtureObjectsEqual(before, after) {
				t.Fatalf("rejected identity changed evidence: %v", err)
			}
		})
	}
	for _, fragment := range []string{`,"operation_id":"duplicate"}`, `,"unknown_identity":true}`} {
		t.Run(fragment, func(t *testing.T) {
			s, d, _, r, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "current-publication")
			if err != nil {
				t.Fatal(err)
			}
			content := recordBytes(transitionCurrent(r))
			content = append(content[:len(content)-1], fragment...)
			if err = os.WriteFile(filepath.Join(s, "current.json"), content, 0600); err != nil {
				t.Fatal(err)
			}
			if got := classify(s, d); !strings.HasPrefix(got, "BAD") {
				t.Fatalf("invalid schema admitted as %q", got)
			}
		})
	}
}

// ARCH-027/031; VAL-031/032: cleanup transfer never republishes a completed current.
func TestCurrentIdentitySurvivesRepeatedCleanupTransferAndNextPreJournalWriter(t *testing.T) {
	s, d, external, r, err := prepareTransitionRecoveryFixture(t.TempDir(), "multi-generation-exact-reapply", "current-publication")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s, "current.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		injected := errors.New("interrupted cleanup transfer")
		_, err = transitionCommandRouterInjected(s, d, external, r, "recover-repair", nil, func(point string) error {
			if point == "recovery-writer-owner-transfer/after-action" {
				return injected
			}
			if strings.HasPrefix(point, "current-publication/") {
				t.Fatal("completed current publication replayed")
			}
			return nil
		})
		if !errors.Is(err, injected) {
			t.Fatalf("transfer: %v", err)
		}
		var owner writerOwnerRecord
		if err = readRecord(filepath.Join(s, "writer-owner.json"), &owner); err != nil {
			t.Fatal(err)
		}
		owner.Process = processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
		if err = publishRecord(s, "writer-owner.json", owner); err != nil {
			t.Fatal(err)
		}
		if got := classify(s, d); got != "CLEANUP(B)" {
			t.Fatalf("transferred cleanup=%q", got)
		}
	}
	got, err := transitionCommandRouterInjected(s, d, external, r, "recover-repair", nil, func(point string) error {
		if strings.HasPrefix(point, "current-publication/") || strings.HasPrefix(point, "external-") {
			t.Fatalf("completed action replayed: %s", point)
		}
		return nil
	})
	if err != nil || got != "STABLE(A)" {
		t.Fatalf("cleanup=%q err=%v", got, err)
	}
	after, err := os.ReadFile(path)
	afterInfo, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !bytes.Equal(before, after) || !os.SameFile(beforeInfo, afterInfo) {
		t.Fatal("cleanup replaced original current identity")
	}
	var current currentRecord
	var released writerOwnerRecord
	if readRecord(path, &current) != nil || readRecord(filepath.Join(s, "writer-owner.json"), &released) != nil {
		t.Fatal("completed records unavailable")
	}
	next := freshGraphRefs()
	next.instance = r.instance
	next.a = r.b
	next.b = r.b
	next.receiptAPrevious = r.a
	next.receiptA = r.receiptB
	next.receiptB = r.receiptB
	next.priorCurrent = current
	next.generation = released.Generation + 1
	next.priorWriterToken = released.PreviousToken
	if err = os.Remove(filepath.Join(external, "effect.json")); err != nil {
		t.Fatal(err)
	}
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	injected := errors.New("new writer stopped before journal")
	err = executeTransition(s, d, external, next, dead, nil, func(point string) error {
		if point == "writer-owner-held-publication/authority-post/after-revalidation" {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("next operation: %v", err)
	}
	if next.generation == current.WriterGeneration+2 {
		t.Fatal("fixture did not retain cleanup transfer generations")
	}
	if got = classify(s, d); !strings.HasPrefix(got, "BAD") {
		t.Fatalf("fresh pre-journal writer admitted as %q", got)
	}
	got, err = transitionCommandRouter(s, d, external, next, "recover-repair")
	if err != nil || got != "STABLE(A)" || !exactRecordBytes(path, current) || !exactRecordBytes(filepath.Join(s, "writer-owner.json"), released) {
		t.Fatalf("prior authority repair=%q err=%v", got, err)
	}
}

// PC-016/ARCH-027: journal removal leaves only state-owned committed cleanup repair.
func TestCurrentIdentityCommittedNoJournalRepairPreservesCurrent(t *testing.T) {
	for _, kind := range []string{"bootstrap", "replacing", "exact-reapply", "multi-generation-exact-reapply"} {
		t.Run(kind, func(t *testing.T) {
			s, d, external, r, err := prepareTransitionRecoveryFixture(t.TempDir(), kind, "cleanup-journal-remove")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s, "current.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = transitionCommandRouter(s, d, external, r, "recover-resume"); err == nil {
				t.Fatal("journal-less resume admitted")
			}
			got, err := transitionCommandRouterInjected(s, d, external, r, "recover-repair", nil, func(point string) error {
				if strings.HasPrefix(point, "current-publication/") || strings.HasPrefix(point, "external-") || strings.HasPrefix(point, "journal-") {
					t.Fatalf("completed action replayed: %s", point)
				}
				return nil
			})
			if err != nil || got != "STABLE(A)" {
				t.Fatalf("repair=%q err=%v", got, err)
			}
			after, err := os.ReadFile(path)
			afterIdentity, statErr := os.Lstat(path)
			if err != nil || statErr != nil || !bytes.Equal(before, after) || !os.SameFile(identity, afterIdentity) {
				t.Fatal("committed current replaced during no-journal repair")
			}
		})
	}
}
