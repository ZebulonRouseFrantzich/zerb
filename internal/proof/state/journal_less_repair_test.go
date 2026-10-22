package state

import (
	"path/filepath"
	"testing"
)

// ARCH-027/030 and PC-012: state owns recognition; empty repair has no digest.
func TestStateOwnedJournalLessRepair(t *testing.T) {
	for _, kind := range []string{"bootstrap", "replacing", "exact-reapply", "multi-generation-exact-reapply"} {
		for _, stage := range []string{"writer-owner-held-publication", "candidate-publication", "journal-prepared-publication", "cleanup-journal-remove"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), kind, stage)
				if err != nil {
					t.Fatal(err)
				}
				lease, err := acquireStableLease(s, struct {
					name string
					typ  int16
				}{"admission.lock", fWRLCK}, struct {
					name string
					typ  int16
				}{"writer.lock", fWRLCK})
				if err != nil {
					t.Fatal(err)
				}
				defer lease.release()
				prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
				if err != nil {
					t.Fatal(err)
				}
				before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
				if err != nil {
					t.Fatal(err)
				}
				result, handled, err := repairJournalLessUnderLease(s, d, external, lease, prefix, nil)
				if stage == "journal-prepared-publication" {
					after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
					if err != nil || handled || inventoryErr != nil || !fixtureObjectsEqual(before, after) {
						t.Fatalf("journaled result=%+v handled=%t err=%v inventory=%v", result, handled, err, inventoryErr)
					}
					return
				}
				if err != nil || !handled {
					t.Fatalf("result=%+v handled=%t err=%v", result, handled, err)
				}
				wantCandidate, wantKind, wantState := refs.a, "CandidateRecovered", "STABLE(A)"
				if stage == "cleanup-journal-remove" {
					wantCandidate = refs.b
				} else if refs.a == "" {
					wantKind, wantState = "StateRepaired", "READY(∅)"
				}
				if result.candidate != receiptDigest(wantCandidate) || result.kind != wantKind || result.classification != wantState {
					t.Fatalf("result=%+v want=%s/%s/%s", result, wantKind, wantState, wantCandidate)
				}
				if stage != "cleanup-journal-remove" && !priorTransitionAuthorityPreserved(s, d, external, refs) {
					t.Fatal("journal-less repair changed prior authority")
				}
				if !leaseAttemptBlocked(s, "admission.lock", fWRLCK) || !leaseAttemptBlocked(s, "writer.lock", fWRLCK) {
					t.Fatal("state repair consumed caller lease")
				}
			})
		}
	}
}

func TestJournalLessRepairRejectsStaleAuthorityBeforeMutation(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "bootstrap", "candidate-publication")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fWRLCK}, struct {
		name string
		typ  int16
	}{"writer.lock", fWRLCK})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = durableRecordIdentitySubstitution(filepath.Join(s, "writer-owner.json"), nil); err != nil {
		t.Fatal(err)
	}
	before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		t.Fatal(err)
	}
	_, handled, repairErr := repairJournalLessUnderLease(s, d, external, lease, prefix, nil)
	after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if repairErr == nil || handled || inventoryErr != nil || !fixtureObjectsEqual(before, after) {
		t.Fatalf("stale authority handled=%t err=%v inventory=%v", handled, repairErr, inventoryErr)
	}
}
