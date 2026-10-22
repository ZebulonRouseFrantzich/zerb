//go:build linux

package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

func runInitializationAuthoritySubstitutionMatrix(root *prooffs.OwnedRoot) (int, int, []string) {
	verified, total, failures := runInitializationTakeoverSubstitutions(root)
	count, rows, sweepFailures := runInitializationCleanupSubstitutions(root)
	verified += count
	total += rows
	failures = append(failures, sweepFailures...)
	count, rows, sweepFailures = runInitializationCompletionSubstitutions(root)
	verified += count
	total += rows
	failures = append(failures, sweepFailures...)
	count, rows, sweepFailures = runInitializationRootSubstitutions(root)
	verified += count
	total += rows
	failures = append(failures, sweepFailures...)
	return verified, total, failures
}

func runFullInitializationPublication(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	runOrderedInitializationPrefixes(root, add)
	if cfg.SelfExecutable == "" {
		add(Scenario{ID: "VAL-025/full-initialization-publication", Requirements: []string{"VAL-025", "VAL-031"}, Status: Inconclusive, Fixture: "child initializer", Writes: []string{}, Observation: "SelfExecutable not supplied; child boundary matrix not run"})
		return
	}
	verified, publicationTotal := runInitialPublicationDeathMatrix(ctx, cfg, root)
	successorVerified, successorTotal := runInitializationSuccessorDeathMatrix(ctx, cfg, root)
	prefixTakeoverVerified, prefixTakeoverTotal := runInitializationPrefixTakeoverDeathMatrix(ctx, cfg, root)
	residueSuccessorVerified, residueTotal := runInitializationResidueDeathMatrix(ctx, cfg, root)
	repeatedSuccessorVerified, repeatedFailure := runRepeatedInitializationSuccessorDeaths(ctx, cfg, root)
	status := Observed
	if verified != publicationTotal || successorVerified != successorTotal ||
		prefixTakeoverVerified != prefixTakeoverTotal ||
		residueSuccessorVerified != residueTotal || repeatedSuccessorVerified != 1 {
		status = Failed
	}
	var repeatedFailures []string
	if repeatedFailure != "" {
		repeatedFailures = []string{"repeated-successor/" + repeatedFailure}
	}
	add(Scenario{
		ID: "VAL-025/full-initialization-publication", Requirements: []string{"VAL-025", "VAL-031"}, Status: status,
		Fixture: "child-killed initial publications, takeover/cleanup boundaries, and every repair-completion successor prefix", Lease: "stable I-X claim and inode-bound compare-transfer",
		Writes:             []string{"fixtures/<typed-initialization-prefix>/state", "fixtures/<typed-initialization-prefix>/data", "fixtures/<typed-initialization-prefix>/cache"},
		Observation:        fmt.Sprintf("verified initial publication prefixes=%d/%d, takeover/completion successor-death prefixes=%d/%d, linked-temporary successor death across every exact claimed INIT prefix=%d/%d, interrupted-temporary cleanup successor-death prefixes=%d/%d, and repeated committed-successor-then-linked-successor death=%d/1; every O_TMPFILE/link/rename/sync/reopen boundary preserved either the exact old claim plus an authenticated linked temporary or the exact monotonic successor, and recover-repair reopened the complete state/data/cache graph with released writer generation zero", verified, publicationTotal, successorVerified, successorTotal, prefixTakeoverVerified, prefixTakeoverTotal, residueSuccessorVerified, residueTotal, repeatedSuccessorVerified),
		RejectedAssumption: "a returned publication error or a two-goroutine race is not initializer crash evidence",
		FailedBoundaries:   repeatedFailures,
	})
	substitutions, substitutionTotal, substitutionFailures := runInitializationAuthoritySubstitutionMatrix(root)
	substitutionStatus := Observed
	if substitutions != substitutionTotal {
		substitutionStatus = Failed
	}
	add(Scenario{
		ID: "VAL-025/initialization-authority-substitution", Requirements: []string{"VAL-025", "VAL-031", "VAL-034"}, Status: substitutionStatus,
		Fixture: "noncooperating same-UID substitution across initialization takeover, residue cleanup, and completion", Lease: "stable I-X",
		Writes:             []string{"fixtures/<typed-initialization-substitution>/state", "fixtures/<typed-initialization-substitution>/data", "fixtures/<typed-initialization-substitution>/cache"},
		Observation:        fmt.Sprintf("rejected initialization authority substitutions=%d/%d: extra entries plus byte-identical claim, init-lock, and interrupted-temporary inode replacement across takeover liveness/compare-transfer, residue cleanup, every completion action, and state/data/cache root replacement; no later initialization action ran after first detection", substitutions, substitutionTotal),
		RejectedAssumption: "cooperative I-X excludes a noncooperating same-UID namespace substitution",
		FailedBoundaries:   substitutionFailures,
	})
}

func runInitializationTakeoverSubstitutions(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	takeoverPoints := []string{
		"takeover-before-liveness",
		"takeover-after-liveness",
		"takeover-before-publication",
		"takeover-after-publication",
	}
	for _, boundary := range []prooffs.Boundary{
		prooffs.BeforeTemp,
		prooffs.AfterTemp,
		prooffs.AfterWrite,
		prooffs.AfterFileSync,
		prooffs.BeforeRename,
		prooffs.AfterRename,
		prooffs.AfterParentSync,
		prooffs.AfterReopen,
	} {
		takeoverPoints = append(takeoverPoints, "takeover-publication/"+string(boundary))
	}
	for _, point := range takeoverPoints {
		variants := []string{"extra-entry", "claim-identity", "init-lock-identity"}
		if point == "takeover-publication/"+string(prooffs.BeforeRename) {
			variants = append(variants, "takeover-temp-identity")
		}
		for _, variant := range variants {
			total++
			spec, err := prepareInitializationRepairFixture(root.Path, point)
			if err != nil {
				failures = append(failures, point+"/"+variant+"/prepare")
				continue
			}
			hit := false
			var substitutedTakeoverPath string
			_, repairErr := initializationCommandRouter(spec, processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}, "recover-repair", func(name string) error {
				if name != point {
					return nil
				}
				hit = true
				switch variant {
				case "extra-entry":
					return os.WriteFile(filepath.Join(spec.State, "unexpected"), []byte("noncooperating initialization substitution"), 0600)
				case "claim-identity":
					return durableRecordIdentitySubstitution(filepath.Join(spec.State, "root.json"), nil)
				case "takeover-temp-identity":
					entries, err := os.ReadDir(spec.State)
					if err != nil {
						return err
					}
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".proof-takeover-") {
							substitutedTakeoverPath = filepath.Join(spec.State, entry.Name())
							return durableRecordIdentitySubstitution(substitutedTakeoverPath, nil)
						}
					}
					return errors.New("takeover temporary unavailable")
				default:
					if err := os.Remove(filepath.Join(spec.State, "init.lock")); err != nil {
						return err
					}
					return createDurableEmpty(spec.State, "init.lock")
				}
			})
			replacementPreserved := variant != "takeover-temp-identity" ||
				(substitutedTakeoverPath != "" && exists(substitutedTakeoverPath))
			if hit && repairErr != nil && replacementPreserved && !fullInitializationValid(spec) {
				verified++
			} else {
				failures = append(failures, point+"/"+variant)
			}
		}
	}
	return verified, total, failures
}

func runInitializationCleanupSubstitutions(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	cleanupCases := []struct {
		point string
		stale bool
	}{
		{"takeover-residue-before-unlink", false},
		{"takeover-residue-after-unlink", false},
		{"takeover-stale-residue-before-unlink", true},
		{"takeover-stale-residue-after-unlink", true},
	}
	for _, cleanupCase := range cleanupCases {
		for _, variant := range []string{"extra-entry", "claim-identity", "init-lock-identity", "residue-identity"} {
			total++
			spec, residuePath, err := prepareInitializationResidueFixture(root.Path)
			if cleanupCase.stale {
				spec, residuePath, err = prepareInitializationTakeoverResidueFixture(root.Path)
			}
			if err != nil {
				failures = append(failures, cleanupCase.point+"/"+variant+"/prepare")
				continue
			}
			residueContent, err := readNoFollowBounded(residuePath, maxRecognitionFile)
			if err != nil {
				failures = append(failures, cleanupCase.point+"/"+variant+"/residue")
				continue
			}
			hit := false
			_, repairErr := initializationCommandRouter(spec, processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}, "recover-repair", func(name string) error {
				if name != cleanupCase.point {
					return nil
				}
				hit = true
				switch variant {
				case "extra-entry":
					return os.WriteFile(filepath.Join(spec.State, "unexpected"), []byte("noncooperating initialization substitution"), 0600)
				case "claim-identity":
					return durableRecordIdentitySubstitution(filepath.Join(spec.State, "root.json"), nil)
				case "init-lock-identity":
					if err := os.Remove(filepath.Join(spec.State, "init.lock")); err != nil {
						return err
					}
					return createDurableEmpty(spec.State, "init.lock")
				default:
					if exists(residuePath) {
						return durableRecordIdentitySubstitution(residuePath, nil)
					}
					if err := os.WriteFile(residuePath, residueContent, 0600); err != nil {
						return err
					}
					return syncDir(spec.State)
				}
			})
			if hit && repairErr != nil && !fullInitializationValid(spec) {
				verified++
			} else {
				failures = append(failures, cleanupCase.point+"/"+variant)
			}
		}
	}
	return verified, total, failures
}

func runInitializationCompletionSubstitutions(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	stages := orderedInitializationStages[2:]
	for _, stage := range stages {
		for _, point := range []string{stage + "/before-action", stage} {
			for _, variant := range []string{"extra-entry", "claim-identity", "init-lock-identity"} {
				total++
				spec, err := prepareInitializationRepairFixture(root.Path, stage)
				if err != nil {
					failures = append(failures, point+"/"+variant+"/prepare")
					continue
				}
				hit := false
				_, repairErr := initializationCommandRouter(spec, processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}, "recover-repair", func(name string) error {
					if name != point {
						return nil
					}
					hit = true
					switch variant {
					case "extra-entry":
						return os.WriteFile(filepath.Join(spec.State, "unexpected"), []byte("noncooperating initialization substitution"), 0600)
					case "claim-identity":
						return durableRecordIdentitySubstitution(filepath.Join(spec.State, "root.json"), nil)
					default:
						if err := os.Remove(filepath.Join(spec.State, "init.lock")); err != nil {
							return err
						}
						return createDurableEmpty(spec.State, "init.lock")
					}
				})
				// A post-publication replacement can preserve a semantically valid
				// initialized graph; the required outcome is identity detection and
				// refusal, because there is no later initialization action to suppress.
				if hit && repairErr != nil && (stage == "initialized-root" || !fullInitializationValid(spec)) {
					verified++
				} else {
					failures = append(failures, point+"/"+variant)
				}
			}
		}
	}
	return verified, total, failures
}

func runInitializationRootSubstitutions(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	for _, variant := range []string{"state-root", "data-root", "cache-root"} {
		total++
		spec, err := prepareInitializationRepairFixture(root.Path, variant)
		if err != nil {
			failures = append(failures, variant+"/prepare")
			continue
		}
		point := "admission-lock/before-action"
		switch variant {
		case "data-root":
			point = "data-root"
		case "cache-root":
			point = "cache-root"
		}
		hit := false
		_, repairErr := initializationCommandRouter(spec, processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}, "recover-repair", func(name string) error {
			if name != point {
				return nil
			}
			hit = true
			path := spec.State
			switch variant {
			case "data-root":
				path = spec.Data
			case "cache-root":
				path = spec.Cache
			}
			if err := os.Rename(path, path+".substituted"); err != nil {
				return err
			}
			if err := os.Mkdir(path, 0700); err != nil {
				return err
			}
			return syncDir(filepath.Dir(path))
		})
		if hit && repairErr != nil && !fullInitializationValid(spec) {
			verified++
		} else {
			failures = append(failures, variant)
		}
	}
	return verified, total, failures
}

func runInitialPublicationDeathMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int) {
	boundaries := []prooffs.Boundary{prooffs.AfterTemp, prooffs.AfterWrite, prooffs.AfterFileSync, prooffs.AfterRename, prooffs.AfterParentSync, prooffs.AfterReopen}
	verified := 0
	for _, boundary := range boundaries {
		s, d := resetRoots(root.Path, true, false)
		pair := filepath.Dir(s)
		cache := filepath.Join(pair, "cache")
		if err := createDurableEmpty(s, "init.lock"); err != nil || syncDir(pair) != nil || syncDir(filepath.Dir(pair)) != nil {
			continue
		}
		lockBefore, err := os.Lstat(filepath.Join(s, "init.lock"))
		if err != nil {
			continue
		}
		refs := freshGraphRefs()
		spec := workerSpecFor(s, d, cache, "", string(boundary), refs)
		specPath := filepath.Join(pair, "worker-spec.json")
		if writeDeterministicWorkerSpec(specPath, spec) != nil {
			continue
		}
		child, err := startGatedChild(ctx, cfg.SelfExecutable, "init-publication-die", specPath, root)
		if err != nil {
			continue
		}
		claim := initializingRoot(spec.Instance, spec.Token, 0, child.identity)
		refsByObject := make(map[string]string)
		beforeLoser, _, beforeErr := protectedInventory(map[string]string{"state": s}, refsByObject)
		loser := exec.CommandContext(ctx, cfg.SelfExecutable, "--worker", "init-contender", "--worker-path", specPath)
		loser.Env = []string{"PATH=/nonexistent", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
		loserErr := loser.Run()
		afterLoser, _, afterErr := protectedInventory(map[string]string{"state": s}, refsByObject)
		claimExact := exactRecordBytes(filepath.Join(s, "root.json"), claim)
		killed := child.releaseAndWaitForKill()
		lockAfter, statErr := os.Lstat(filepath.Join(s, "init.lock"))
		stableInode := statErr == nil && os.SameFile(lockBefore, lockAfter)
		prefixExact := inspectInitializationPrefix(spec, child.identity)
		doctorBefore, _, doctorBeforeErr := protectedInventory(map[string]string{"state": s}, refsByObject)
		diagnosis, doctorErr := initializationCommandRouter(spec, child.identity, "doctor")
		doctorAfter, _, doctorAfterErr := protectedInventory(map[string]string{"state": s}, refsByObject)
		repaired, repairErr := initializationCommandRouter(spec, child.identity, "recover-repair")
		if beforeErr == nil && afterErr == nil && loserErr == nil && inventoriesEqual(beforeLoser, afterLoser) && claimExact && killed && stableInode && prefixExact &&
			doctorBeforeErr == nil && doctorAfterErr == nil && inventoriesEqual(doctorBefore, doctorAfter) && doctorErr == nil && diagnosis == "INIT exact durable prefix" &&
			repairErr == nil && repaired == "READY(∅)" && fullInitializationValid(spec) {
			verified++
		}
	}
	return verified, len(boundaries)
}

func runInitializationSuccessorDeathMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int) {
	successorStages := []string{
		"takeover-before-liveness",
		"takeover-after-liveness",
		"takeover-before-publication",
		"takeover-after-publication",
	}
	for _, boundary := range []prooffs.Boundary{
		prooffs.BeforeTemp,
		prooffs.AfterTemp,
		prooffs.AfterWrite,
		prooffs.AfterFileSync,
		prooffs.BeforeRename,
		prooffs.AfterRename,
		prooffs.AfterParentSync,
		prooffs.AfterReopen,
	} {
		successorStages = append(successorStages, "takeover-publication/"+string(boundary))
	}
	successorStages = append(successorStages, orderedInitializationStages[2:]...)
	successorVerified := 0
	for _, stage := range successorStages {
		spec, err := prepareInitializationRepairFixture(root.Path, stage)
		if err != nil {
			continue
		}
		specPath := filepath.Join(filepath.Dir(spec.State), "worker-spec.json")
		if writeDeterministicWorkerSpec(specPath, spec) != nil {
			continue
		}
		child, err := startGatedChild(ctx, cfg.SelfExecutable, "init-repair-die", specPath, root)
		if err != nil {
			continue
		}
		killed := child.releaseAndWaitForKill()
		var successorClaim rootOwnerRecord
		prefixExact := fullInitializationValid(spec)
		if !prefixExact {
			prefixExact = readRecord(filepath.Join(spec.State, "root.json"), &successorClaim) == nil &&
				(exactInitializationRepairPrefix(spec, successorClaim) ||
					inspectInitializationPrefix(spec, successorClaim.Process))
		}
		repaired, repairErr := initializationCommandRouter(spec, child.identity, "recover-repair")
		if killed && prefixExact && repairErr == nil && repaired == "READY(∅)" && fullInitializationValid(spec) {
			successorVerified++
		}
	}
	return successorVerified, len(successorStages)
}

func runInitializationPrefixTakeoverDeathMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int) {
	prefixTakeoverTotal := len(orderedInitializationStages) - 2
	prefixTakeoverVerified := 0
	for stop := 1; stop < len(orderedInitializationStages)-1; stop++ {
		spec, err := prepareInitializationPrefixRepairFixture(
			root.Path,
			stop,
			"takeover-publication/"+string(prooffs.BeforeRename),
		)
		if err != nil {
			continue
		}
		specPath := filepath.Join(filepath.Dir(spec.State), "worker-spec.json")
		if writeDeterministicWorkerSpec(specPath, spec) != nil {
			continue
		}
		child, err := startGatedChild(ctx, cfg.SelfExecutable, "init-repair-die", specPath, root)
		if err != nil {
			continue
		}
		killed := child.releaseAndWaitForKill()
		var durableClaim rootOwnerRecord
		prefixExact := readRecord(filepath.Join(spec.State, "root.json"), &durableClaim) == nil &&
			inspectInitializationPrefix(spec, durableClaim.Process)
		repaired, repairErr := initializationCommandRouter(spec, durableClaim.Process, "recover-repair")
		if killed && prefixExact && repairErr == nil && repaired == "READY(∅)" && fullInitializationValid(spec) {
			prefixTakeoverVerified++
		}
	}
	return prefixTakeoverVerified, prefixTakeoverTotal
}

func runInitializationResidueDeathMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int) {
	residueStages := []struct {
		name  string
		stale bool
	}{
		{"takeover-residue-before-unlink", false},
		{"takeover-residue-after-unlink", false},
		{"takeover-stale-residue-before-unlink", true},
		{"takeover-stale-residue-after-unlink", true},
	}
	residueSuccessorVerified := 0
	for _, stage := range residueStages {
		spec, _, err := prepareInitializationResidueFixture(root.Path)
		if stage.stale {
			spec, _, err = prepareInitializationTakeoverResidueFixture(root.Path)
		}
		if err != nil {
			continue
		}
		spec.RecoveryBoundary = stage.name
		specPath := filepath.Join(filepath.Dir(spec.State), "worker-spec.json")
		if writeDeterministicWorkerSpec(specPath, spec) != nil {
			continue
		}
		child, err := startGatedChild(ctx, cfg.SelfExecutable, "init-repair-die", specPath, root)
		if err != nil {
			continue
		}
		killed := child.releaseAndWaitForKill()
		var successorClaim rootOwnerRecord
		prefixExact := readRecord(filepath.Join(spec.State, "root.json"), &successorClaim) == nil &&
			(exactInitializationRepairPrefix(spec, successorClaim) ||
				inspectInitializationPrefix(spec, successorClaim.Process))
		repaired, repairErr := initializationCommandRouter(spec, successorClaim.Process, "recover-repair")
		if killed && prefixExact && repairErr == nil && repaired == "READY(∅)" && fullInitializationValid(spec) {
			residueSuccessorVerified++
		}
	}
	return residueSuccessorVerified, len(residueStages)
}

func runRepeatedInitializationSuccessorDeaths(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, string) {
	repeatedSuccessorVerified := 0
	var repeatedFailure string
	spec, err := prepareInitializationRepairFixture(root.Path, "takeover-publication/"+string(prooffs.AfterRename))
	if err != nil {
		repeatedFailure = "prepare"
	}
	specPath := filepath.Join(filepath.Dir(spec.State), "worker-spec.json")
	if repeatedFailure == "" && writeDeterministicWorkerSpec(specPath, spec) != nil {
		repeatedFailure = "first-spec"
	}
	var first *gatedChild
	if repeatedFailure == "" {
		first, err = startGatedChild(ctx, cfg.SelfExecutable, "init-repair-die", specPath, root)
		if err != nil {
			repeatedFailure = "first-start"
		}
	}
	if repeatedFailure == "" && !first.releaseAndWaitForKill() {
		repeatedFailure = "first-death"
	}
	spec.RecoveryBoundary = "takeover-publication/" + string(prooffs.BeforeRename)
	if repeatedFailure == "" && writeDeterministicWorkerSpec(specPath, spec) != nil {
		repeatedFailure = "second-spec"
	}
	var second *gatedChild
	if repeatedFailure == "" {
		second, err = startGatedChild(ctx, cfg.SelfExecutable, "init-repair-die", specPath, root)
		if err != nil {
			repeatedFailure = "second-start"
		}
	}
	if repeatedFailure == "" && !second.releaseAndWaitForKill() {
		repeatedFailure = "second-death"
	}
	var durableClaim rootOwnerRecord
	if repeatedFailure == "" && readRecord(filepath.Join(spec.State, "root.json"), &durableClaim) != nil {
		repeatedFailure = "claim-reopen"
	}
	if repeatedFailure == "" {
		if _, inspectErr := inspectInitializationPrefixAuthority(spec, durableClaim); inspectErr != nil {
			repeatedFailure = "prefix-reopen"
		}
	}
	if repeatedFailure == "" {
		repaired, repairErr := initializationCommandRouter(spec, durableClaim.Process, "recover-repair")
		if repairErr != nil {
			repeatedFailure = "repair"
		} else if repaired != "READY(∅)" || !fullInitializationValid(spec) {
			repeatedFailure = "completion-reopen"
		} else {
			repeatedSuccessorVerified++
		}
	}
	return repeatedSuccessorVerified, repeatedFailure
}
