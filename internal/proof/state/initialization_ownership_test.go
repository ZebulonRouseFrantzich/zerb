package state

import "testing"

// ARCH-030: a failed handoff returns the actual acquisition for caller cleanup.
func TestInitializationReturnsOwnedAcquisition(t *testing.T) {
	for _, tc := range []struct {
		injection  string
		initHeld   bool
		targetHeld bool
	}{
		{"target-acquisition-failure", true, false},
		{"held-owner-reopen-mismatch", true, true},
		{"held-owner-reopen-altered", true, true},
		{"released-owner-reopen-mismatch", false, true},
		{"released-owner-reopen-altered", false, true},
		{"", false, true},
	} {
		t.Run(tc.injection, func(t *testing.T) {
			s, d := admissionFixture(t.TempDir(), "CLEAN")
			got, acquisition := initializeCommandInjected(s, d, "CLEAN", "bootstrap", tc.injection, Scenario{Status: Observed})
			defer acquisition.release()
			if (got.Status == Failed) != (tc.injection != "") {
				t.Fatalf("unexpected handoff outcome: %+v", got)
			}
			if (acquisition.initialization != nil) != tc.initHeld || (acquisition.target != nil) != tc.targetHeld {
				t.Fatalf("returned ownership init=%t target=%t; want %t/%t", acquisition.initialization != nil, acquisition.target != nil, tc.initHeld, tc.targetHeld)
			}
			if leaseAttemptBlocked(s, "init.lock", fWRLCK) != tc.initHeld {
				t.Fatal("returned initialization handle does not own the expected exclusion")
			}
			for _, name := range []string{"admission.lock", "writer.lock"} {
				if leaseAttemptBlocked(s, name, fWRLCK) != tc.targetHeld {
					t.Fatalf("returned target handle does not own %s exclusion", name)
				}
			}
			acquisition.release()
			for _, name := range []string{"init.lock", "admission.lock", "writer.lock"} {
				if !leaseAttemptAvailable(s, name, fWRLCK) {
					t.Fatalf("caller cleanup left %s held", name)
				}
			}
		})
	}
}

func TestPlanningInitializationReturnsAdmissionOnly(t *testing.T) {
	for _, command := range []string{"profile-list", "plan-exact"} {
		t.Run(command, func(t *testing.T) {
			s, d := admissionFixture(t.TempDir(), "CLEAN")
			got, acquisition := initializeCommandInjected(s, d, "CLEAN", command, "", Scenario{Status: Observed})
			defer acquisition.release()
			if got.Status != Observed || acquisition.initialization != nil || acquisition.target == nil {
				t.Fatalf("failed target handoff: %+v", got)
			}
			if !leaseAttemptBlocked(s, "admission.lock", fWRLCK) || !leaseAttemptAvailable(s, "writer.lock", fWRLCK) {
				t.Fatal("planning handoff returned a different lease mode")
			}
		})
	}
}
