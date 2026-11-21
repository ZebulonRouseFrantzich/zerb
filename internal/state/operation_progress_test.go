package state

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

func TestOperationPrefixExactTable(t *testing.T) {
	// Deliberately independent table: include the before/after-effect checkpoint
	// for a phase without claiming that an absent checkpoint proves no effect.
	rows := []struct {
		phase Phase
		want  []int
	}{
		{PhasePrepared, []int{0}}, {PhaseToolInstall, []int{0, 1}},
		{PhaseHomeApply, []int{1, 2}}, {PhasePendingReceipt, []int{2}}, {PhaseCleanup, []int{2}},
		{"", nil}, {"stable", nil}, {"tool-install", nil}, {"home-apply", nil},
	}
	prefixes := [][]Phase{nil, {PhaseToolInstall}, {PhaseToolInstall, PhaseHomeApply},
		{PhaseHomeApply}, {PhaseHomeApply, PhaseToolInstall}, {PhaseToolInstall, PhaseToolInstall},
		{PhasePrepared}, {PhaseToolInstall, PhaseHomeApply, PhaseCleanup}, {PhaseToolInstall, "unknown"}}
	for _, row := range rows {
		for i, prefix := range prefixes {
			t.Run(fmt.Sprintf("%s/%d", row.phase, i), func(t *testing.T) {
				before := slices.Clone(prefix)
				if got := validOperationPrefix(row.phase, prefix); got != slices.Contains(row.want, i) {
					t.Fatal("prefix table changed")
				}
				if !slices.Equal(prefix, before) {
					t.Fatal("execution prefix reordered")
				}
			})
		}
	}
	if !validOperationPrefix(PhasePrepared, []Phase{}) {
		t.Fatal("empty representation changed value semantics")
	}
}

func TestJournalProgressCoverageDoesNotClaimEffects(t *testing.T) {
	targets := []string{"a", "b/c"}
	pre := []receipt.DestinationObservation{{Target: "a", Kind: "absent"}, {Target: "b/c", Kind: "absent"}}
	for _, phase := range []Phase{PhasePrepared, PhaseToolInstall} {
		if !journalProgressMatchesTargets(phase, nil, targets, pre, pre) {
			t.Fatal("initial actual observations refused")
		}
	}
	latest := []receipt.DestinationObservation{{Target: "a", Exists: true, Kind: "directory"}, {Target: "b/c", Kind: "absent"}}
	if !journalProgressMatchesTargets(PhaseHomeApply, []Phase{PhaseToolInstall}, targets, pre, latest) {
		t.Fatal("latest observations confused with home completion")
	}
	for _, phase := range []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt, PhaseCleanup} {
		var prefix []Phase
		switch phase {
		case PhaseHomeApply:
			prefix = []Phase{PhaseToolInstall}
		case PhasePendingReceipt, PhaseCleanup:
			prefix = []Phase{PhaseToolInstall, PhaseHomeApply}
		}
		if journalProgressMatchesTargets(phase, prefix, targets, pre, nil) || journalProgressMatchesTargets(phase, prefix, targets, nil, latest) {
			t.Fatal("stage-dependent incomplete coverage accepted")
		}
	}
	if !journalProgressMatchesTargets(PhaseCleanup, []Phase{PhaseToolInstall, PhaseHomeApply}, nil, nil, nil) {
		t.Fatal("empty verified target inventory refused")
	}
}

func TestJournalProgressCoverageMalformedAndBoundary(t *testing.T) {
	for _, targets := range [][]string{{""}, {"a", "a"}, {"b", "a"}} {
		observed := make([]receipt.DestinationObservation, len(targets))
		for i, target := range targets {
			observed[i].Target = target
		}
		if observationCoverage(targets, observed) {
			t.Fatal("unsupported target order/identity coverage")
		}
	}
	for _, observed := range [][]receipt.DestinationObservation{nil, {{Target: "b"}}, {{Target: "a"}, {Target: "b"}}} {
		if observationCoverage([]string{"a"}, observed) {
			t.Fatal("missing, mismatched or extra observation")
		}
	}
	for _, count := range []int{4095, 4096, 4097} {
		targets := make([]string, count)
		observed := make([]receipt.DestinationObservation, count)
		for i := range count {
			targets[i] = fmt.Sprintf("target/%04d", i)
			observed[i].Target = targets[i]
		}
		if got := observationCoverage(targets, observed); got != (count <= 4096) {
			t.Fatal("PC042 target count boundary changed")
		}
	}
}

func TestCompletedChecksIndependentAvailability(t *testing.T) {
	for bits := 0; bits < 32; bits++ {
		facts := receipt.Completed{Phases: []string{string(PhaseHomeApply), string(PhaseToolInstall)}, Dimensions: receipt.Dimensions{
			InputsPinned: bits&1 != 0, ArtifactIntegrity: bits&2 != 0, ArtifactAvailable: bits&4 != 0, HostCompatible: bits&8 != 0, ConvergenceVerified: bits&16 != 0}}
		if got := completedChecksPassed(facts); got != (bits&27 == 27) {
			t.Fatal("independent completion dimensions changed")
		}
	}
	facts := receipt.Completed{Dimensions: receipt.Dimensions{InputsPinned: true, ArtifactIntegrity: true, HostCompatible: true, ConvergenceVerified: true}}
	for _, phases := range [][]string{nil, {}, {string(PhaseToolInstall)}, {string(PhaseToolInstall), string(PhaseHomeApply)}, {string(PhaseHomeApply), string(PhaseHomeApply)}, {string(PhaseHomeApply), string(PhaseToolInstall), string(PhaseCleanup)}} {
		facts.Phases = phases
		if completedChecksPassed(facts) {
			t.Fatal("missing, unsorted, repeated or extra completion phase accepted")
		}
	}
}

func TestOperationChecksAreReadOnlyConcurrent(t *testing.T) {
	targets := []string{"a"}
	observed := []receipt.DestinationObservation{{Target: "a", Kind: "absent"}}
	prefix := []Phase{PhaseToolInstall, PhaseHomeApply}
	facts := receipt.Completed{Phases: []string{string(PhaseHomeApply), string(PhaseToolInstall)}, Dimensions: receipt.Dimensions{InputsPinned: true, ArtifactIntegrity: true, HostCompatible: true, ConvergenceVerified: true}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if !journalProgressMatchesTargets(PhaseCleanup, prefix, targets, observed, observed) || !completedChecksPassed(facts) {
					t.Error("read-only facts changed")
				}
			}
		}()
	}
	wg.Wait()
	if targets[0] != "a" || observed[0].Target != "a" || prefix[0] != PhaseToolInstall || facts.Phases[0] != string(PhaseHomeApply) {
		t.Fatal("caller input modified")
	}
}

func FuzzOperationPrefix(f *testing.F) {
	for _, phase := range []string{"prepared", "mutating-tool-install", "mutating-home-apply", "pending-receipt", "cleanup", ""} {
		f.Add(phase, []byte{0, 1})
	}
	f.Fuzz(func(t *testing.T, phase string, encoded []byte) {
		if len(phase) > 64 || len(encoded) > 4 {
			return
		}
		values := []Phase{PhaseToolInstall, PhaseHomeApply, PhasePrepared, PhaseCleanup, "unknown"}
		prefix := make([]Phase, len(encoded))
		for i, b := range encoded {
			prefix[i] = values[int(b)%len(values)]
		}
		if validOperationPrefix(Phase(phase), prefix) {
			if len(prefix) > 2 || len(prefix) > 0 && prefix[0] != PhaseToolInstall || len(prefix) > 1 && prefix[1] != PhaseHomeApply {
				t.Fatal("non-execution prefix accepted")
			}
			if (phase == string(PhasePendingReceipt) || phase == string(PhaseCleanup)) && len(prefix) != 2 {
				t.Fatal("incomplete completion state")
			}
		}
	})
}
