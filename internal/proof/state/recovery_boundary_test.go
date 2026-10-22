package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

// ARCH-031 / VAL-030: exhausted generations cannot remove interrupted evidence.
func TestRecoveryGenerationOverflowPreservesResidue(t *testing.T) {
	for _, test := range []struct {
		name       string
		generation uint64
	}{
		{"max-minus-one", ^uint64(0) - 1},
		{"max", ^uint64(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			generation := test.generation
			s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "journal-prepared-publication")
			if err != nil {
				t.Fatal(err)
			}
			refs.generation = generation
			refs.writerToken = strings.Repeat("c", 32)
			refs.writerPredecessorToken = strings.Repeat("b", 32)
			dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
			owner := heldWriter(refs.instance, refs.writerToken, generation, dead, generation-1, refs.writerPredecessorToken)
			journal, ok := transitionJournalForStage("journal-prepared-publication", refs)
			if !ok || writeRecord(filepath.Join(s, "writer-owner.json"), owner) != nil ||
				writeRecord(filepath.Join(s, "journals", refs.journal+".json"), journal) != nil {
				t.Fatal("prepare exhausted-generation relation")
			}
			authority, residue := recoveryJournalResidue(t, s)
			before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err != nil {
				t.Fatal(err)
			}
			_, err = transitionCommandRouterInjected(s, d, external, refs, "recover-resume", authority)
			after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err == nil || inventoryErr != nil || !exists(residue) || !fixtureObjectsEqual(before, after) {
				t.Fatalf("err=%v inventory=%v residue=%t unchanged=%t", err, inventoryErr, exists(residue), fixtureObjectsEqual(before, after))
			}
		})
	}
}

// ARCH-023/031 / VAL-031: authority substitution must stop before residue unlink.
func TestRecoveryResidueCleanupRevalidatesAuthorityBeforeUnlink(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "journal-prepared-publication")
	if err != nil {
		t.Fatal(err)
	}
	authority, residue := recoveryJournalResidue(t, s)
	hit := false
	_, err = transitionCommandRouterInjected(s, d, external, refs, "recover-resume", authority, func(point string) error {
		if point != "recovery-residue-cleanup/before-unlink" {
			return nil
		}
		hit = true
		return durableRecordIdentitySubstitution(filepath.Join(s, "writer-owner.json"), nil)
	})
	if !hit || err == nil || !exists(residue) || exists(filepath.Join(external, "effect.json")) {
		t.Fatalf("hit=%t err=%v residue=%t external_effect=%t", hit, err, exists(residue), exists(filepath.Join(external, "effect.json")))
	}
}

// PC-016 / VAL-033: BAD journal-less prefixes require repair, never resume.
func TestJournalLessResumeDeniesBeforeWriterAcquisition(t *testing.T) {
	for _, kind := range []string{"bootstrap", "replacing", "exact-reapply", "multi-generation-exact-reapply"} {
		for _, stage := range []string{"writer-owner-held-publication", "candidate-publication"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), kind, stage)
				if err != nil {
					t.Fatal(err)
				}
				holder, err := acquireStableLease(s, struct {
					name string
					typ  int16
				}{"writer.lock", fWRLCK})
				if err != nil {
					t.Fatal(err)
				}
				before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
				if err != nil {
					holder.release()
					t.Fatal(err)
				}
				_, err = transitionCommandRouter(s, d, external, refs, "recover-resume")
				holder.release()
				after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
				if err == nil || errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) ||
					inventoryErr != nil || !fixtureObjectsEqual(before, after) {
					t.Fatalf("err=%v inventory=%v unchanged=%t", err, inventoryErr, fixtureObjectsEqual(before, after))
				}
				recovered, err := transitionCommandRouter(s, d, external, refs, "recover-repair")
				want := "STABLE(A)"
				if refs.a == "" {
					want = "READY(∅)"
				}
				if err != nil || recovered != want || !priorTransitionAuthorityPreserved(s, d, external, refs) {
					t.Fatalf("repair=%q err=%v prior_preserved=%t", recovered, err, priorTransitionAuthorityPreserved(s, d, external, refs))
				}
			})
		}
	}
}

func recoveryJournalResidue(t *testing.T, stateDir string) (*transitionRecoveryAuthority, string) {
	t.Helper()
	residue := filepath.Join(stateDir, "journals", ".proof-recovery-boundary")
	if err := os.WriteFile(residue, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(filepath.Dir(residue)); err != nil {
		t.Fatal(err)
	}
	return &transitionRecoveryAuthority{
		Boundary:      transitionBoundary{Point: string(prooffs.AfterTemp)},
		Identity:      processIdentity{PID: 999998, Start: 2, PIDNamespace: 1},
		ExpectResidue: true, RecoveryStage: "recovery-journal-transfer",
	}, residue
}

// PC-016 / VAL-033: completed STABLE operations deny both recovery forms under A-X.
func TestCompletedRecoveryDeniesBeforeWriterAcquisition(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "writer-owner-released-publication")
	if err != nil {
		t.Fatal(err)
	}
	if classify(s, d) != "STABLE(A)" {
		t.Fatal("completed fixture is not stable")
	}
	holder, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"writer.lock", fWRLCK})
	if err != nil {
		t.Fatal(err)
	}
	defer holder.release()
	for _, command := range []string{"recover-resume", "recover-repair"} {
		t.Run(command, func(t *testing.T) {
			before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := transitionCommandRouter(s, d, external, refs, command)
			after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err == nil || recovered != "" || errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) ||
				inventoryErr != nil || !fixtureObjectsEqual(before, after) {
				t.Fatalf("recovery=%q err=%v inventory=%v unchanged=%t", recovered, err, inventoryErr, fixtureObjectsEqual(before, after))
			}
		})
	}
}
