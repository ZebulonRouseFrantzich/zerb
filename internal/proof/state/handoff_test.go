package state

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

func TestBootstrapHandoffRecordReopen(t *testing.T) {
	base := t.TempDir()
	for i := 0; i < 32; i++ {
		s, d := admissionFixture(base, "CLEAN")
		got := routeCommandInjected(s, d, "CLEAN", "bootstrap", "")
		if got.Status != Observed {
			t.Fatalf("iteration %d: %s", i, got.Observation)
		}
	}
}

func TestBootstrapHandoffRejectsOwnerInodeSubstitution(t *testing.T) {
	for _, injection := range []string{"held-owner-reopen-mismatch", "held-owner-reopen-altered", "released-owner-reopen-mismatch", "released-owner-reopen-altered"} {
		t.Run(injection, func(t *testing.T) {
			s, d := admissionFixture(t.TempDir(), "CLEAN")
			got := routeCommandInjected(s, d, "CLEAN", "bootstrap", injection)
			if got.Status != Failed {
				t.Fatalf("substituted owner publication admitted: %s", got.Observation)
			}
		})
	}
}

func TestBootstrapClassifierBoundaries(t *testing.T) {
	root := t.TempDir()
	template := freshBootstrapRefs()
	for _, boundary := range transitionBoundaries(template) {
		s, d, external, refs, err := prepareFirstBootstrapFixture(root)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := processIdentityForPID(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		injected := errors.New("first-bootstrap classification boundary")
		runErr := executeTransition(s, d, external, refs, identity, nil, func(name string) error {
			if name == boundary.Name {
				return injected
			}
			return nil
		})
		durableStage := durableStageForBoundary(boundary, refs)
		want := "READY(∅)"
		if transitionStageIndex(durableStage) >= transitionStageIndex("writer-owner-held-publication") &&
			transitionStageIndex(durableStage) < transitionStageIndex("journal-prepared-publication") {
			want = "BAD corrupt"
		} else if transitionStageIndex(durableStage) >= transitionStageIndex("journal-prepared-publication") {
			want = "TRANSITION(∅,B)"
		}
		if transitionStageIndex(durableStage) >= transitionStageIndex("current-publication") {
			want = "CLEANUP(B)"
		}
		if durableStage == "writer-owner-released-publication" {
			want = "STABLE(A)"
		}
		stage, recognitionErr := recognizeTransitionPrefix(s, d, external, refs)
		if !errors.Is(runErr, injected) || classify(s, d) != want || recognitionErr != nil || stage != durableStage {
			var writer writerOwnerRecord
			var journal journalRecord
			_ = readRecord(s+"/writer-owner.json", &writer)
			_ = readRecord(s+"/journals/"+refs.journal+".json", &journal)
			t.Fatalf("%s: run=%v classify=%s exact_current=%q want=%s recognize=%s recognition_err=%v durable=%s writer=%+v journal=%+v", boundary.Name, runErr, classify(s, d), exactCurrent(s, d, os.Geteuid()), want, stage, recognitionErr, durableStage, writer, journal)
		}
	}
}

func TestDiagnosticRoutingDoesNotInitializeCleanOrInit(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		s, d := admissionFixture(t.TempDir(), "CLEAN")
		got := routeCommandInjected(s, d, "CLEAN", "doctor", "")
		if got.Status != Observed || got.Classification != "CLEAN" || got.Lease != "D-S" {
			t.Fatalf("unexpected CLEAN diagnostic route: %+v", got)
		}
		if exists(s) || exists(d) || len(got.Writes) != 0 {
			t.Fatalf("CLEAN diagnostic created product roots: state=%t data=%t writes=%v", exists(s), exists(d), got.Writes)
		}
	})
	t.Run("init", func(t *testing.T) {
		s, d := admissionFixture(t.TempDir(), "INIT")
		got := routeCommandInjected(s, d, "INIT", "doctor", "")
		if got.Status != Observed || got.Classification != "INIT" || got.Lease != "D-S" {
			t.Fatalf("unexpected INIT diagnostic route: %+v", got)
		}
		if !exactLeafNames(s, "init.lock") || exists(d) || len(got.Writes) != 0 {
			t.Fatalf("INIT diagnostic changed product roots: data=%t writes=%v", exists(d), got.Writes)
		}
	})
}

func TestDeniedRoutesRetainIntentLeaseAndPerformNoSemanticAction(t *testing.T) {
	for _, tc := range []struct {
		state   string
		command string
		lease   string
	}{
		{"READY(∅)", "exec", "A-S"},
		{"STABLE(A)", "recover-resume", "A-X"},
		{"TRANSITION(A,B)", "apply", "A-X"},
		{"CLEANUP(B)", "apply", "A-X"},
	} {
		t.Run(tc.state+"/"+tc.command, func(t *testing.T) {
			s, d := admissionFixture(t.TempDir(), tc.state)
			got := routeCommand(s, d, tc.state, tc.command)
			if got.Status != Observed || got.Admission != "deny" || got.Lease != tc.lease ||
				got.Boundaries.SourceRevalidations != 0 || got.Boundaries.CandidateRevalidations != 0 ||
				got.Boundaries.ManagerRevalidations != 0 || len(got.Writes) != 0 ||
				!strings.Contains(got.Observation, "zero source, candidate, manager, destination, or child action") {
				t.Fatalf("denied route claimed semantic work: %+v", got)
			}
		})
	}
}

func TestCleanPlanningInitializationDoesNotClaimBootstrapWork(t *testing.T) {
	for _, command := range []string{"profile-list", "plan-exact"} {
		t.Run(command, func(t *testing.T) {
			s, d := admissionFixture(t.TempDir(), "CLEAN")
			got := routeCommand(s, d, "CLEAN", command)
			if got.Status != Observed || got.Admission != "admit" || got.Lease != "I-X+A-X" ||
				got.Boundaries.SourceRevalidations != 0 || got.Boundaries.CandidateRevalidations != 0 ||
				got.Boundaries.ManagerRevalidations != 0 || strings.Contains(got.Observation, "candidate/journal/mutation") {
				t.Fatalf("planning route claimed bootstrap work: %+v", got)
			}
		})
	}
}

func TestReadyRejectsImpossibleReleasedGenerationOne(t *testing.T) {
	s, d, refs := makeGraph(t.TempDir(), "READY(∅)")
	if err := writeRecord(s+"/writer-owner.json", releasedWriter(refs.instance, "", 1, 0, refs.priorWriterToken)); err != nil {
		t.Fatal(err)
	}

	if got := classify(s, d); !strings.HasPrefix(got, "BAD") {
		t.Fatalf("generation-one initialization writer admitted as %s", got)
	}
}
func TestRecoveryTransferResiduesRetainExactAuthority(t *testing.T) {
	for _, transfer := range []string{"recovery-journal-transfer", "recovery-writer-owner-transfer"} {
		t.Run(transfer, func(t *testing.T) {
			s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "journal-prepared-publication")
			if err != nil {
				t.Fatal(err)
			}
			interrupted := processIdentity{PID: 999998, Start: 2, PIDNamespace: 1}
			point := string(prooffs.AfterTemp)
			residueDir := s + "/journals"
			residueBytes := []byte(nil)
			if transfer == "recovery-writer-owner-transfer" {
				point = string(prooffs.AfterWrite)
				next := refs
				next.generation = refs.generation + 1
				next.writerToken = strings.Repeat("c", 32)
				next.writerPredecessorToken = refs.writerToken
				journal, ok := transitionJournalForStage("journal-prepared-publication", next)
				if !ok {
					t.Fatal("prepared journal unavailable")
				}
				if err = publishRecord(s+"/journals", refs.journal+".json", journal); err != nil {
					t.Fatal(err)
				}
				residueDir = s
				residueBytes = recordBytes(heldWriter(
					next.instance, next.writerToken, next.generation, interrupted,
					next.generation-1, next.writerPredecessorToken,
				))
			}
			residuePath := residueDir + "/.proof-retained-residue"
			if err = os.WriteFile(residuePath, residueBytes, 0600); err != nil || syncDir(residueDir) != nil {
				t.Fatalf("residue publication: %v", err)
			}
			authority := &transitionRecoveryAuthority{
				Boundary: transitionBoundary{Point: point},
				Identity: interrupted, ExpectResidue: true, RecoveryStage: transfer,
			}
			prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, authority)
			if err != nil || prefix.residue == nil || prefix.residue.path != residuePath {
				t.Fatalf("recognition=%+v err=%v", prefix, err)
			}
			recovered, err := transitionCommandRouterInjected(s, d, external, refs, "recover-resume", authority)
			if err != nil || recovered != "STABLE(A)" || exists(residuePath) ||
				!verifyCompletionPublication(s, refs, true, true) {
				t.Fatalf("recovery=%q err=%v residue=%t", recovered, err, exists(residuePath))
			}
		})
	}
}

func TestRecoveryRecognizesPostTransferActionResidue(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(
		t.TempDir(), "replacing", "journal-prepared-publication",
	)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := processIdentity{PID: 999998, Start: 2, PIDNamespace: 1}
	active := refs
	active.generation = refs.generation + 1
	active.writerPredecessorToken = refs.writerToken
	active.writerToken = strings.Repeat("d", 32)
	journal, ok := transitionJournalForStage("journal-prepared-publication", active)
	if !ok {
		t.Fatal("prepared journal unavailable")
	}
	if err = publishRecord(s+"/journals", refs.journal+".json", journal); err != nil {
		t.Fatal(err)
	}
	if err = publishRecord(
		s, "writer-owner.json",
		heldWriter(
			active.instance, active.writerToken, active.generation, interrupted,
			active.generation-1, active.writerPredecessorToken,
		),
	); err != nil {
		t.Fatal(err)
	}
	residuePath := s + "/journals/.proof-post-transfer-action"
	if err = os.WriteFile(residuePath, nil, 0600); err != nil || syncDir(s+"/journals") != nil {
		t.Fatalf("residue publication: %v", err)
	}
	boundary, ok := transitionBoundaryByName(
		active, "journal-mutating-tool-install-publication/"+string(prooffs.AfterTemp),
	)
	if !ok {
		t.Fatal("action boundary unavailable")
	}
	authority := &transitionRecoveryAuthority{
		Boundary: boundary, Identity: interrupted, ExpectResidue: true,
	}
	prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, authority)
	if err != nil || prefix.stage != "journal-prepared-publication" ||
		prefix.refs.generation != active.generation || prefix.residue == nil ||
		prefix.residue.path != residuePath {
		t.Fatalf("recognition=%+v err=%v", prefix, err)
	}
	recovered, err := transitionCommandRouterInjected(
		s, d, external, refs, "recover-resume", authority,
	)
	if err != nil || recovered != "STABLE(A)" || exists(residuePath) ||
		!verifyCompletionPublication(s, refs, true, true) {
		t.Fatalf("recovery=%q err=%v residue=%t", recovered, err, exists(residuePath))
	}
}

func TestProofTokenDomainsAreDistinct(t *testing.T) {
	refs := freshGraphRefs()
	tokens := []string{refs.token, refs.operationToken, refs.priorWriterToken, refs.writerToken}
	for i, token := range tokens {
		if token == "" {
			t.Fatalf("token domain %d empty", i)
		}
		for j := 0; j < i; j++ {
			if token == tokens[j] {
				t.Fatalf("token domains %d and %d reused", i, j)
			}
		}
	}
	if !validWriterPredecessor(heldWriter(refs.instance, refs.writerToken, 1, processIdentity{}, 0, "")) {
		t.Fatal("held generation-one owner rejected its exact released-generation-zero predecessor")
	}
	if validWriterPredecessor(releasedWriter(refs.instance, "", 1, 0, "")) {
		t.Fatal("released generation-one owner admitted without the prior held token")
	}
	if validWriterPredecessor(releasedWriter(refs.instance, "", 1, 0, refs.writerToken)) {
		t.Fatal("released generation-one owner admitted an impossible held-generation-zero predecessor")
	}
	if validWriterPredecessor(heldWriter(refs.instance, refs.writerToken, 2, processIdentity{}, 1, refs.writerToken)) {
		t.Fatal("held writer admitted its own token as predecessor")
	}
	equalOperation := refs
	equalOperation.operationToken = refs.writerToken
	if validJournalPrefix(transitionJournal(equalOperation, "prepared", "", nil, 1, refs.writerToken)) {
		t.Fatal("journal admitted one token for operation and active writer domains")
	}
	equalPredecessor := refs
	equalPredecessor.writerPredecessorToken = refs.operationToken
	if validJournalPrefix(transitionJournal(equalPredecessor, "prepared", "", nil, 2, refs.writerToken)) {
		t.Fatal("journal admitted one token for operation and predecessor-writer domains")
	}
}

func TestRecoveryContinuesOnlyAfterRecognizedDurableStage(t *testing.T) {
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	template := freshGraphRefs()
	var stages []string
	for _, stage := range transitionStages(template) {
		if transitionStageIndex(stage) >= transitionStageIndex("journal-prepared-publication") {
			stages = append(stages, stage)
		}
	}
	for _, stop := range stages {
		t.Run(stop, func(t *testing.T) {
			s, d, external, refs, err := prepareTransitionFixture(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			boundary := stop + "/authority-post/after-revalidation"
			injected := errors.New("durable-stage stop")
			runErr := executeTransition(s, d, external, refs, dead, nil, func(name string) error {
				if name == boundary {
					return injected
				}
				return nil
			})
			if !errors.Is(runErr, injected) {
				t.Fatalf("transition did not stop at %s: %v", boundary, runErr)
			}
			gotStage, err := recognizeTransitionPrefix(s, d, external, refs)
			if err != nil || gotStage != stop {
				t.Fatalf("recognized stage=%q err=%v want=%q", gotStage, err, stop)
			}
			var regressed string
			recovered, err := transitionCommandRouterInjected(
				s, d, external, refs, recoveryCommandForStage(stop), nil,
				func(name string) error {
					if strings.HasPrefix(name, "recovery-") {
						return nil
					}
					actionStage := strings.SplitN(name, "/", 2)[0]
					if transitionStageIndex(actionStage) <= transitionStageIndex(stop) {
						regressed = actionStage
						return errors.New("recovery regressed durable stage")
					}
					return nil
				},
			)
			if stop == "writer-owner-released-publication" {
				if err == nil || recovered != "" || regressed != "" || classify(s, d) != "STABLE(A)" {
					t.Fatalf("completed recovery=%q err=%v regressed=%q", recovered, err, regressed)
				}
				return
			}
			if err != nil || recovered != "STABLE(A)" || regressed != "" {
				t.Fatalf("recovery=%q err=%v regressed=%q", recovered, err, regressed)
			}
		})
	}
}

func TestRecoveryRejectsStableTransitionBaseWithoutWrites(t *testing.T) {
	s, d, external, refs, err := prepareTransitionFixture(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	refsByObject := make(map[string]string)
	before, _, err := protectedInventory(
		map[string]string{"state": s, "data": d, "external": external},
		refsByObject,
	)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, recoverErr := transitionCommandRouter(s, d, external, refs, "recover-resume"); recoverErr == nil {
		t.Fatalf("stable base recovered as %q", recovered)
	}
	after, _, err := protectedInventory(
		map[string]string{"state": s, "data": d, "external": external},
		refsByObject,
	)
	if err != nil || !inventoriesEqual(before, after) {
		t.Fatalf("stable base changed: err=%v", err)
	}
}

func TestJournalLessRecoveryRestoresPriorAuthorityWithoutExecutingCandidate(t *testing.T) {
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	for _, stop := range []string{"writer-owner-held-publication", "candidate-publication"} {
		t.Run(stop, func(t *testing.T) {
			s, d, external, refs, err := prepareTransitionFixture(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("journal-less stop")
			err = executeTransition(s, d, external, refs, dead, nil, func(name string) error {
				if name == stop+"/authority-post/after-revalidation" {
					return injected
				}
				return nil
			})
			if !errors.Is(err, injected) {
				t.Fatalf("transition stop: %v", err)
			}
			recovered, err := transitionCommandRouter(s, d, external, refs, "recover-repair")
			if err != nil || recovered != "STABLE(A)" {
				t.Fatalf("recovery=%q err=%v", recovered, err)
			}
			if classify(s, d) != "STABLE(A)" ||
				!exactRecordBytes(s+"/writer-owner.json", baselineReleasedWriter(refs)) ||
				exists(s+"/journals/"+refs.journal+".json") ||
				exists(external+"/effect.json") ||
				!exactRecordBytes(s+"/current.json", baselineCurrent(refs)) {
				t.Fatal("journal-less repair executed or changed candidate authority")
			}
		})
	}
}

func TestJournalAheadRecoveryTransferIsRecognized(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(t.TempDir(), "replacing", "journal-prepared-publication")
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireStableLease(s,
		struct {
			name string
			typ  int16
		}{"admission.lock", fWRLCK},
		struct {
			name string
			typ  int16
		}{"writer.lock", fWRLCK},
	)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("journal-ahead stop")
	err = convergeTransitionUnderLease(s, d, external, lease, prefix, func(name string) error {
		if name == "recovery-journal-transfer/after-action" {
			return injected
		}
		return nil
	})
	lease.release()
	if !errors.Is(err, injected) {
		t.Fatalf("recovery did not stop after journal transfer: %v", err)
	}
	pending, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
	if err != nil || !pending.transferPending || pending.stage != "journal-prepared-publication" {
		t.Fatalf("journal-ahead prefix=%+v err=%v", pending, err)
	}
	if got := classify(s, d); got != "TRANSITION(A,B)" {
		t.Fatalf("journal-ahead interval classified %q", got)
	}
	recovered, err := transitionCommandRouter(s, d, external, refs, "recover-resume")
	if err != nil || recovered != "STABLE(A)" {
		t.Fatalf("journal-ahead recovery=%q err=%v", recovered, err)
	}
}

func TestExactReapplyCandidateStageUsesRecognizableDurablePredecessor(t *testing.T) {
	for _, kind := range []string{"exact-reapply", "multi-generation-exact-reapply"} {
		t.Run(kind, func(t *testing.T) {
			s, d, external, refs, err := prepareTransitionRecoveryFixture(
				t.TempDir(), kind, "candidate-publication",
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := recognizeTransitionPrefix(s, d, external, refs)
			if err != nil || got != "writer-owner-held-publication" {
				t.Fatalf("recognized stage=%q err=%v", got, err)
			}
		})
	}
}

func TestRecoveryFailurePreservesMultiGenerationPreparedStage(t *testing.T) {
	s, d, external, refs, err := prepareTransitionRecoveryFixture(
		t.TempDir(), "multi-generation-exact-reapply", "journal-prepared-publication",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := classify(s, d); got != "TRANSITION(A,B)" {
		t.Fatalf("prepared classification=%q", got)
	}
	prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireStableLease(s,
		struct {
			name string
			typ  int16
		}{"admission.lock", fWRLCK},
		struct {
			name string
			typ  int16
		}{"writer.lock", fWRLCK},
	)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("unavailable recovery input")
	err = convergeTransitionUnderLease(s, d, external, lease, prefix, func(name string) error {
		if name == "journal-mutating-tool-install-publication/authority-pre/before-revalidation" {
			return injected
		}
		return nil
	})
	lease.release()
	if !errors.Is(err, injected) {
		t.Fatalf("recovery did not stop at unavailable input boundary: %v", err)
	}
	reopened, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
	if err != nil || reopened.stage != "journal-prepared-publication" {
		t.Fatalf("reopened prefix=%+v err=%v", reopened, err)
	}
	if got := classify(s, d); got != "TRANSITION(A,B)" {
		t.Fatalf("classification=%q", got)
	}
	if !exactRecordBytes(
		s+"/current.json",
		baselineCurrent(refs),
	) {
		t.Fatal("current authority changed")
	}
	if exists(external + "/effect.json") {
		t.Fatal("external effect occurred")
	}
}

func TestProcessIdentityLivenessFailsClosed(t *testing.T) {
	expected := processIdentity{PID: 42, Start: 7, PIDNamespace: 9}
	unavailable := errors.New("identity source unavailable")
	for _, test := range []struct {
		name      string
		expected  processIdentity
		actual    processIdentity
		readErr   error
		wantLive  bool
		wantError bool
	}{
		{name: "exact-live", expected: expected, actual: expected, wantLive: true},
		{name: "reused-pid", expected: expected, actual: processIdentity{PID: 42, Start: 8, PIDNamespace: 9}},
		{name: "absent", expected: expected, readErr: os.ErrNotExist},
		{name: "unavailable", expected: expected, readErr: unavailable, wantError: true},
		{name: "invalid-expected", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			live, err := processIdentityLiveInjected(
				test.expected,
				func(int) (processIdentity, error) { return test.actual, test.readErr },
			)
			if live != test.wantLive || (err != nil) != test.wantError {
				t.Fatalf("live=%t err=%v", live, err)
			}
		})
	}
}

func TestInitializationTakeoverRejectsClaimInodeSubstitution(t *testing.T) {
	spec, err := prepareInitializationRepairFixture(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	_, err = initializationCommandRouter(
		spec,
		processIdentity{PID: 999999, Start: 1, PIDNamespace: 1},
		"recover-repair",
		func(point string) error {
			if point != "takeover-after-liveness" {
				return nil
			}
			hit = true
			return durableRecordIdentitySubstitution(spec.State+"/root.json", nil)
		},
	)
	if !hit || err == nil || fullInitializationValid(spec) {
		t.Fatalf("hit=%t err=%v full=%t", hit, err, fullInitializationValid(spec))
	}
}

func TestInitializationResidueCleanupRejectsInodeSubstitution(t *testing.T) {
	spec, residuePath, err := prepareInitializationResidueFixture(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hit := false
	_, err = initializationCommandRouter(
		spec,
		processIdentity{PID: 999999, Start: 1, PIDNamespace: 1},
		"recover-repair",
		func(point string) error {
			if point != "takeover-residue-before-unlink" {
				return nil
			}
			hit = true
			return durableRecordIdentitySubstitution(residuePath, nil)
		},
	)
	if !hit || err == nil || !exists(residuePath) || fullInitializationValid(spec) {
		t.Fatalf("hit=%t err=%v residue=%t full=%t", hit, err, exists(residuePath), fullInitializationValid(spec))
	}
}

func TestInitializationTakeoverResidueReopensEveryClaimedPrefix(t *testing.T) {
	for stop := 1; stop < len(orderedInitializationStages)-1; stop++ {
		t.Run(orderedInitializationStages[stop], func(t *testing.T) {
			spec, err := prepareInitializationPrefixRepairFixture(t.TempDir(), stop, "")
			if err != nil {
				t.Fatal(err)
			}
			var claim rootOwnerRecord
			if err = readRecord(spec.State+"/root.json", &claim); err != nil {
				t.Fatal(err)
			}
			takeover := initializingRoot(
				claim.Instance,
				strings.Repeat("e", 32),
				claim.Generation+1,
				processIdentity{PID: 999998, Start: 2, PIDNamespace: 1},
			)
			if err = os.WriteFile(spec.State+"/.proof-takeover-regression", recordBytes(takeover), 0600); err != nil {
				t.Fatal(err)
			}
			if err = syncDir(spec.State); err != nil {
				t.Fatal(err)
			}
			authority, err := inspectInitializationPrefixAuthority(spec, claim)
			if err != nil || authority.takeoverResiduePath == "" {
				t.Fatalf("authority=%+v err=%v", authority, err)
			}
			repaired, err := initializationCommandRouter(spec, claim.Process, "recover-repair")
			if err != nil || repaired != "READY(∅)" || !fullInitializationValid(spec) {
				t.Fatalf("repaired=%q err=%v full=%t", repaired, err, fullInitializationValid(spec))
			}
		})
	}
}

func TestInitializationRejectsWriterPublicationResidueAtWrongPrefix(t *testing.T) {
	spec, err := prepareInitializationPrefixRepairFixture(t.TempDir(), 1, "")
	if err != nil {
		t.Fatal(err)
	}
	var claim rootOwnerRecord
	if err = readRecord(spec.State+"/root.json", &claim); err != nil {
		t.Fatal(err)
	}
	residue := recordBytes(releasedWriter(spec.Instance, "", 0, 0, ""))
	if err = os.WriteFile(spec.State+"/.proof-forged-owner", residue, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = inspectInitializationPrefixAuthority(spec, claim); err == nil {
		t.Fatal("writer publication residue accepted before its exact predecessor prefix")
	}
}

func TestRecoveryClassifiesBeforeWriterLockAcquisition(t *testing.T) {
	s, d, external, refs, err := prepareTransitionFixture(t.TempDir())
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
	defer holder.release()
	_, err = transitionCommandRouter(s, d, external, refs, "recover-resume")
	if err == nil || errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
		t.Fatalf("recovery returned pre-classification writer-lock result: %v", err)
	}
}

func TestGenerationOverflowFailsWithoutMutation(t *testing.T) {
	maxGeneration := ^uint64(0)
	if validWriterPredecessor(heldWriter(
		"instance", strings.Repeat("a", 32), 0,
		processIdentity{PID: 1, Start: 1, PIDNamespace: 1},
		maxGeneration, strings.Repeat("b", 32),
	)) {
		t.Fatal("wrapped writer generation accepted")
	}

	t.Run("initialization", func(t *testing.T) {
		spec, err := prepareInitializationRepairFixture(t.TempDir(), "")
		if err != nil {
			t.Fatal(err)
		}
		dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
		claim := initializingRoot(spec.Instance, spec.Token, maxGeneration, dead)
		if err = publishRecord(spec.State, "root.json", claim); err != nil {
			t.Fatal(err)
		}
		before, err := actualFixtureObjects(initializationRoots(spec))
		if err != nil {
			t.Fatal(err)
		}
		_, err = initializationCommandRouter(spec, dead, "recover-repair")
		after, inventoryErr := actualFixtureObjects(initializationRoots(spec))
		if err == nil || inventoryErr != nil || !fixtureObjectsEqual(before, after) {
			t.Fatalf("err=%v inventoryErr=%v unchanged=%t", err, inventoryErr, fixtureObjectsEqual(before, after))
		}
	})

	t.Run("fresh-transition", func(t *testing.T) {
		s, d, external, refs, err := prepareTransitionFixture(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		refs.generation = maxGeneration
		owner := releasedWriter(refs.instance, "", maxGeneration, maxGeneration-1, strings.Repeat("a", 32))
		if err = writeRecord(s+"/writer-owner.json", owner); err != nil {
			t.Fatal(err)
		}
		before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
		if err != nil {
			t.Fatal(err)
		}
		identity, err := processIdentityForPID(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		readyCalled := false
		err = executeTransition(s, d, external, refs, identity, func() error {
			readyCalled = true
			return nil
		}, nil)
		after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
		if err == nil || readyCalled || inventoryErr != nil || !fixtureObjectsEqual(before, after) {
			t.Fatalf("err=%v ready=%t inventoryErr=%v unchanged=%t", err, readyCalled, inventoryErr, fixtureObjectsEqual(before, after))
		}
	})

	for _, test := range []struct {
		name             string
		ownerGeneration  uint64
		activeGeneration uint64
		transferPending  bool
	}{
		{"recovery-owner-max-minus-one", maxGeneration - 1, maxGeneration - 1, false},
		{"recovery-journal-ahead-max", maxGeneration - 1, maxGeneration, true},
		{"recovery-owner-max", maxGeneration, maxGeneration, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, d, external, refs, err := prepareTransitionRecoveryFixture(
				t.TempDir(), "replacing", "journal-prepared-publication",
			)
			if err != nil {
				t.Fatal(err)
			}
			dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
			ownerToken := strings.Repeat("c", 32)
			previousToken := strings.Repeat("b", 32)
			activeToken := ownerToken
			activePredecessor := previousToken
			if test.transferPending {
				activeToken = strings.Repeat("d", 32)
				activePredecessor = ownerToken
			}
			refs.generation = test.activeGeneration
			refs.writerToken = activeToken
			refs.writerPredecessorToken = activePredecessor
			owner := heldWriter(
				refs.instance,
				ownerToken,
				test.ownerGeneration,
				dead,
				test.ownerGeneration-1,
				previousToken,
			)
			journal, ok := transitionJournalForStage("journal-prepared-publication", refs)
			if !ok ||
				writeRecord(s+"/writer-owner.json", owner) != nil ||
				writeRecord(s+"/journals/"+refs.journal+".json", journal) != nil {
				t.Fatal("failed to prepare exhausted-generation recovery")
			}
			before, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err != nil {
				t.Fatal(err)
			}
			_, err = transitionCommandRouter(s, d, external, refs, "recover-resume")
			after, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if err == nil || inventoryErr != nil || !fixtureObjectsEqual(before, after) {
				t.Fatalf("err=%v inventoryErr=%v unchanged=%t", err, inventoryErr, fixtureObjectsEqual(before, after))
			}
		})
	}
}
