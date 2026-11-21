//go:build linux

package state

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

func runRecoveryAuthoritySubstitutionMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int, []string) {
	verified, total, failures := runRecoveryPreTransferSubstitutionMatrix(root)
	if cfg.SelfExecutable != "" {
		postVerified, postTotal, postFailures := runRecoveryPostTransferSubstitutionMatrix(ctx, cfg, root)
		verified += postVerified
		total += postTotal
		failures = append(failures, postFailures...)
	}
	return verified, total, failures
}

func runRecoveryPreTransferSubstitutionMatrix(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		sample := transitionVariantRefs(kind)
		expected := expectedTransitionObjects(
			"journal-prepared-publication", sample,
			processIdentity{PID: 999999, Start: 1, PIDNamespace: 1},
		)
		templateObjects := recoverySubstitutionObjects(expected)
		for objectIndex, templateObject := range templateObjects {
			alteredVariants := []bool{false}
			if templateObject == "state:writer-owner.json" {
				alteredVariants = append(alteredVariants, true)
			}
			for _, ownerAltered := range alteredVariants {
				total++
				s, d, external, refs, err := prepareTransitionRecoveryFixture(root.Path, kind, "journal-prepared-publication")
				if err != nil {
					failures = append(failures, fmt.Sprintf("recovery-substitution/%s/object-%d/prepare", kind, objectIndex))
					continue
				}
				prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
				if err != nil {
					failures = append(failures, fmt.Sprintf("recovery-substitution/%s/object-%d/recognize", kind, objectIndex))
					continue
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
					failures = append(failures, fmt.Sprintf("recovery-substitution/%s/object-%d/lease", kind, objectIndex))
					continue
				}
				before, beforeErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
				objects := recoverySubstitutionObjects(expectedTransitionObjects("journal-prepared-publication", refs, prefix.owner.Process))
				if beforeErr != nil || objectIndex >= len(objects) {
					lease.release()
					failures = append(failures, fmt.Sprintf("recovery-substitution/%s/object-%d/inventory", kind, objectIndex))
					continue
				}
				object := objects[objectIndex]
				substituted, err := substituteRecoveryAuthorityObject(s, object, prefix.owner, refs, ownerAltered)
				runErr := err
				if runErr == nil {
					runErr = convergeTransitionUnderLease(s, d, external, lease, prefix, nil)
				}
				lease.release()
				after, afterErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
				preserved := before != nil && afterErr == nil && fixtureObjectsEqualExcept(before, after, object)
				if object == "state:writer-owner.json" {
					bytesChanged := !bytes.Equal(before[object].Bytes, after[object].Bytes)
					preserved = preserved && bytesChanged == ownerAltered &&
						(before[object].Device != after[object].Device || before[object].Inode != after[object].Inode)
				} else if object != "state:admission.lock" && object != "state:writer.lock" {
					preserved = preserved && !bytes.Equal(before[object].Bytes, after[object].Bytes)
				}
				if runErr != nil && strings.Contains(runErr.Error(), "authority substitution") && substituted && preserved {
					verified++
				} else {
					failures = append(failures, "recovery-substitution/"+kind+"/"+object)
				}
			}
		}
	}
	return verified, total, failures
}

func runRecoveryPostTransferSubstitutionMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		sample := transitionVariantRefs(kind)
		targets := []struct {
			boundary string
			stage    string
		}{{"recovery-writer-owner-transfer/after-action", "journal-prepared-publication"}}
		for _, boundary := range transitionBoundaries(sample) {
			if boundary.Kind == "authority" &&
				transitionStageIndex(boundary.Stage) > transitionStageIndex("journal-prepared-publication") {
				retainedStage := durableStageAfterAction(transitionAuthorityExpectedStage(boundary, sample), sample)
				targets = append(targets, struct {
					boundary string
					stage    string
				}{boundary.Name, retainedStage})
			}
		}
		for _, target := range targets {
			recoveryBoundary := target.boundary
			sampleStage := target.stage
			templateObjects := recoverySubstitutionObjects(expectedTransitionObjects(sampleStage, sample, processIdentity{PID: 1, Start: 1, PIDNamespace: 1}))
			for objectIndex, templateObject := range templateObjects {
				alteredVariants := []bool{false}
				if templateObject == "state:writer-owner.json" {
					alteredVariants = append(alteredVariants, true)
				}
				for _, ownerAltered := range alteredVariants {
					total++
					s, d, external, refs, err := prepareTransitionRecoveryFixture(root.Path, kind, "journal-prepared-publication")
					if err != nil {
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/prepare", kind, recoveryBoundary, objectIndex))
						continue
					}
					spec := workerSpecFor(s, d, "", external, "", refs)
					spec.RecoveryBoundary = recoveryBoundary
					specPath := filepath.Join(filepath.Dir(s), "worker-spec.json")
					if writeDeterministicWorkerSpec(specPath, spec) != nil {
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/spec", kind, recoveryBoundary, objectIndex))
						continue
					}
					child, startErr := startGatedChild(ctx, cfg.SelfExecutable, "transition-recover-fail", specPath, root)
					if startErr != nil || !child.releaseAndWaitForFailure() {
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/worker", kind, recoveryBoundary, objectIndex))
						continue
					}
					prefix, recognizeErr := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
					if recognizeErr != nil || prefix.stage != sampleStage {
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/recognize", kind, recoveryBoundary, objectIndex))
						continue
					}
					lease, leaseErr := acquireStableLease(s,
						struct {
							name string
							typ  int16
						}{"admission.lock", fWRLCK},
						struct {
							name string
							typ  int16
						}{"writer.lock", fWRLCK},
					)
					if leaseErr != nil {
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/lease", kind, recoveryBoundary, objectIndex))
						continue
					}
					before, beforeErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
					objects := recoverySubstitutionObjects(expectedTransitionObjects(prefix.stage, refs, prefix.owner.Process))
					if beforeErr != nil || objectIndex >= len(objects) {
						lease.release()
						failures = append(failures, fmt.Sprintf("recovery-substitution/post/%s/%s/object-%d/inventory", kind, recoveryBoundary, objectIndex))
						continue
					}
					object := objects[objectIndex]
					substituted, err := substituteRecoveryAuthorityObject(s, object, prefix.owner, refs, ownerAltered)
					runErr := err
					if runErr == nil {
						if prefix.stage == "writer-owner-released-publication" {
							runErr = revalidateRecoveryTransferAuthority(s, d, external, lease, prefix)
						} else {
							runErr = convergeTransitionUnderLease(s, d, external, lease, prefix, nil)
						}
					}
					lease.release()
					after, afterErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
					preserved := before != nil && afterErr == nil && fixtureObjectsEqualExcept(before, after, object)
					if object == "state:writer-owner.json" {
						bytesChanged := !bytes.Equal(before[object].Bytes, after[object].Bytes)
						preserved = preserved && bytesChanged == ownerAltered &&
							(before[object].Device != after[object].Device || before[object].Inode != after[object].Inode)
					} else if object != "state:admission.lock" && object != "state:writer.lock" {
						preserved = preserved && !bytes.Equal(before[object].Bytes, after[object].Bytes)
					}
					if runErr != nil && strings.Contains(runErr.Error(), "authority substitution") && substituted && preserved {
						verified++
					} else {
						failures = append(failures, "recovery-substitution/post/"+kind+"/"+recoveryBoundary+"/"+object)
					}
				}
			}
		}
	}
	return verified, total, failures
}

func recoverySubstitutionObjects(expected map[string]fixtureObject) []string {
	var objects []string
	for object := range expected {
		if object == "state:admission.lock" || object == "state:writer.lock" || object == "state:writer-owner.json" ||
			object == "state:current.json" || strings.HasPrefix(object, "state:candidates/") ||
			strings.HasPrefix(object, "state:receipts/") || strings.HasPrefix(object, "state:journals/") {
			objects = append(objects, object)
		}
	}
	sort.Strings(objects)
	return objects
}

func substituteRecoveryAuthorityObject(s, object string, owner writerOwnerRecord, refs graphRefs, ownerAltered bool) (bool, error) {
	path := filepath.Join(s, filepath.FromSlash(strings.TrimPrefix(object, "state:")))
	switch object {
	case "state:admission.lock", "state:writer.lock":
		beforeTarget, err := os.Lstat(path)
		if err == nil {
			err = os.Remove(path)
		}
		if err == nil {
			err = createDurableEmpty(s, filepath.Base(path))
		}
		if err != nil {
			return false, err
		}
		afterTarget, err := os.Lstat(path)
		return err == nil && !os.SameFile(beforeTarget, afterTarget), err
	case "state:writer-owner.json":
		var replacement []byte
		if ownerAltered {
			altered := owner
			altered.Instance = strings.Repeat("a", 32)
			if altered.Instance == refs.instance {
				altered.Instance = strings.Repeat("b", 32)
			}
			replacement = recordBytes(altered)
		}
		err := durableRecordIdentitySubstitution(path, replacement)
		return err == nil, err
	default:
		err := durableRecordSubstitution(path)
		return err == nil, err
	}
}
