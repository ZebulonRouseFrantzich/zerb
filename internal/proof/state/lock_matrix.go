//go:build linux

package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
)

func runLockRaces(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	runStableLockContenderRaces(root, add)
	p := filepath.Join(root.Path, "writer-owner-transfer.json")
	refs := freshGraphRefs()
	runInitializingClaimRace(root, add)

	deadIdentity := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	_ = writeRecord(p, heldWriter(refs.instance, refs.writerToken, 4, deadIdentity, 3, refs.priorWriterToken))
	ownerLock := filepath.Join(root.Path, "writer-owner-transfer.lock")
	_ = os.WriteFile(ownerLock, nil, 0600)
	beforeLock, _ := os.Lstat(ownerLock)
	lockFile, _ := os.OpenFile(ownerLock, os.O_RDWR, 0)
	lockErr := ofd(int(lockFile.Fd()), fWRLCK)
	var prior writerOwnerRecord
	priorErr := readRecord(p, &prior)
	liveIdentity, liveIdentityErr := processIdentityForPID(os.Getpid())
	nextGeneration, nextGenerationOK := generationSuccessor(prior.Generation)
	next := heldWriter(refs.instance, refs.journal, nextGeneration, liveIdentity, prior.Generation, prior.Token)
	pubErr := errors.New("writer generation exhausted")
	if nextGenerationOK {
		pubErr = publishRecord(filepath.Dir(p), filepath.Base(p), next)
	}
	var reopened writerOwnerRecord
	reopenErr := readRecord(p, &reopened)
	afterLock, _ := os.Lstat(ownerLock)
	stableLock := beforeLock != nil && afterLock != nil && os.SameFile(beforeLock, afterLock)
	_ = ofd(int(lockFile.Fd()), fUNLCK)
	_ = lockFile.Close()
	status := Observed
	if lockErr != nil || priorErr != nil || liveIdentityErr != nil || !nextGenerationOK ||
		pubErr != nil || reopenErr != nil || !stableLock ||
		reopened.Generation != 5 || reopened.Token != refs.journal || reopened.Process != liveIdentity ||
		reopened.PreviousGeneration != prior.Generation || reopened.PreviousToken != prior.Token {
		status = Failed
	}
	add(Scenario{ID: "VAL-030/owner/stale-compare-transfer", Requirements: []string{"VAL-030", "VAL-032"}, Status: status, Fixture: "stale durable process identity and generation-4 owner", Lease: "A-X+W-X", Writes: []string{"writer-owner-transfer.json"}, Observation: "under the stable writer lock, the durable predecessor tuple was reopened; generation advanced once with a fresh token and complete live PID/start/namespace identity; publication was reopened"})
	liveObserved := liveIdentityErr == nil && validProcessIdentity(liveIdentity)
	status = Observed
	if !liveObserved {
		status = Failed
	}
	add(Scenario{ID: "VAL-030/owner/live-process-identity", Requirements: []string{"VAL-030"}, Status: status, Fixture: "current durable PID/process-start/PID-namespace tuple", Lease: "A-X+W-X", Writes: []string{}, Observation: "complete PID plus process-start and PID-namespace identity was read and classified live"})
	pSub := filepath.Join(root.Path, "writer-substitution.lock")
	heldSub := pSub + ".held"
	_ = os.WriteFile(pSub, nil, 0600)
	beforeSub, _ := os.Lstat(pSub)
	heldFD, _ := os.OpenFile(pSub, os.O_RDWR, 0)
	acquireErr := ofd(int(heldFD.Fd()), fWRLCK)
	_ = os.Rename(pSub, heldSub)
	_ = os.WriteFile(pSub, nil, 0600)
	afterSub, _ := os.Lstat(pSub)
	fdInfo, _ := heldFD.Stat()
	replacement, _ := os.OpenFile(pSub, os.O_RDWR, 0)
	replacementAcquire := ofd(int(replacement.Fd()), fWRLCK) == nil
	stableFD := beforeSub != nil && fdInfo != nil && os.SameFile(beforeSub, fdInfo)
	pathChanged := beforeSub != nil && afterSub != nil && !os.SameFile(beforeSub, afterSub)
	status = Observed
	if acquireErr != nil || !stableFD || !pathChanged || !replacementAcquire {
		status = Failed
	}
	_ = ofd(int(replacement.Fd()), fUNLCK)
	_ = replacement.Close()
	_ = ofd(int(heldFD.Fd()), fUNLCK)
	_ = heldFD.Close()
	add(Scenario{ID: "VAL-030/locks/path-inode-substitution", Requirements: []string{"VAL-030", "VAL-031"}, Status: status, Fixture: "writer lock pathname replaced while original OFD remained locked", Lease: "A-X+W-X", Writes: []string{"writer-substitution.lock", "writer-substitution.lock.held"}, Observation: "original descriptor retained its inode; replacement pathname had a distinct inode and independent lock, so stable-inode revalidation rejected authority"})
	runRestoredAuthorityProof(root, pSub, heldSub, beforeSub, add)
	runGenerationOverflowProof(root, add)
	runPIDReuseProof(ctx, cfg, root, add)
	runTwoReclaimers(root, add)
}

func runStableLockContenderRaces(root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, kind := range []string{"init", "admission", "writer"} {
		p := filepath.Join(root.Path, kind+"-race.lock")
		_ = os.WriteFile(p, nil, 0600)
		before, _ := os.Lstat(p)
		start := make(chan struct{})
		release := make(chan struct{})
		results := make(chan bool, 2)
		for range 2 {
			go func() {
				f, e := os.OpenFile(p, os.O_RDWR, 0)
				if e != nil {
					results <- false
					return
				}
				defer f.Close()
				<-start
				if ofd(int(f.Fd()), fWRLCK) != nil {
					results <- false
					return
				}
				results <- true
				<-release
				_ = ofd(int(f.Fd()), fUNLCK)
			}()
		}
		close(start)
		successes := 0
		for range 2 {
			if <-results {
				successes++
			}
		}
		close(release)
		after, _ := os.Lstat(p)
		stable := before != nil && after != nil && os.SameFile(before, after)
		status := Observed
		if successes != 1 || !stable {
			status = Failed
		}
		add(Scenario{ID: "VAL-030/locks/" + kind + "-simultaneous-race", Requirements: []string{"VAL-025", "VAL-030"}, Status: status, Fixture: kind + " stable inode with two simultaneous OFD contenders", Lease: strings.ToUpper(string(kind[0])) + "-X", Writes: []string{kind + "-race.lock"}, Observation: fmt.Sprintf("exclusive winners=%d; pathname inode stable=%t", successes, stable)})
	}
}

func runInitializingClaimRace(root *prooffs.OwnedRoot, add func(Scenario)) {
	initDir := filepath.Join(root.Path, "two-initializers")
	_ = os.Mkdir(initDir, 0700)
	initLock := filepath.Join(initDir, "init.lock")
	_ = os.WriteFile(initLock, nil, 0600)
	refsInit := freshGraphRefs()
	startInit := make(chan struct{})
	releaseInit := make(chan struct{})
	initResults := make(chan bool, 2)
	for range 2 {
		go func() {
			f, e := os.OpenFile(initLock, os.O_RDWR, 0)
			if e != nil {
				initResults <- false
				return
			}
			defer f.Close()
			<-startInit
			if ofd(int(f.Fd()), fWRLCK) != nil {
				initResults <- false
				return
			}
			identity, identityErr := processIdentityForPID(os.Getpid())
			if identityErr != nil {
				initResults <- false
				return
			}
			claim := initializingRoot(refsInit.instance, refsInit.token, 0, identity)
			e = publishRecord(initDir, "root.json", claim)
			initResults <- e == nil
			<-releaseInit
			_ = ofd(int(f.Fd()), fUNLCK)
		}()
	}
	close(startInit)
	initWinners := 0
	for range 2 {
		if <-initResults {
			initWinners++
		}
	}
	close(releaseInit)
	var claim rootOwnerRecord
	claimErr := readRecord(filepath.Join(initDir, "root.json"), &claim)
	initEntries, _ := os.ReadDir(initDir)
	status := Observed
	if initWinners != 1 || claimErr != nil || claim.Instance != refsInit.instance || claim.Token != refsInit.token || len(initEntries) != 2 {
		status = Failed
	}
	add(Scenario{ID: "VAL-025/two-initializers", Requirements: []string{"VAL-025"}, Status: status, Fixture: "two simultaneous OFD init-lock contenders", Lease: "I-X", Writes: []string{"two-initializers/init.lock", "two-initializers/root.json"}, Observation: fmt.Sprintf("durable claim winners=%d; exact namespace entries=%d; loser made no publication", initWinners, len(initEntries))})
}

func runRestoredAuthorityProof(root *prooffs.OwnedRoot, pSub, heldSub string, beforeSub os.FileInfo, add func(Scenario)) {
	abaStatus := Observed
	removeReplacementErr := os.Remove(pSub)
	restoreLockErr := error(nil)
	if removeReplacementErr == nil {
		restoreLockErr = os.Rename(heldSub, pSub)
	}
	restoredSub, restoredSubErr := os.Lstat(pSub)
	lockIdentityRepeated := restoredSubErr == nil && beforeSub != nil && os.SameFile(beforeSub, restoredSub)
	abaDir := filepath.Join(root.Path, "owner-record-aba")
	ownerPath := filepath.Join(abaDir, "writer-owner.json")
	ownerHeld := ownerPath + ".held"
	abaRefs := freshGraphRefs()
	oldOwner := heldWriter(abaRefs.instance, abaRefs.writerToken, 4, processIdentity{PID: 41, Start: 1, PIDNamespace: 1}, 3, abaRefs.token)
	newOwner := heldWriter(abaRefs.instance, abaRefs.journal, 5, processIdentity{PID: 42, Start: 2, PIDNamespace: 1}, 4, abaRefs.writerToken)
	mkdirErr := os.Mkdir(abaDir, 0700)
	oldWriteErr := writeRecord(ownerPath, oldOwner)
	oldInfo, oldStatErr := os.Lstat(ownerPath)
	holdErr := os.Rename(ownerPath, ownerHeld)
	newWriteErr := writeRecord(ownerPath, newOwner)
	middleInfo, middleStatErr := os.Lstat(ownerPath)
	middleChanged := oldStatErr == nil && middleStatErr == nil && !os.SameFile(oldInfo, middleInfo)
	removeNewErr := os.Remove(ownerPath)
	restoreOwnerErr := os.Rename(ownerHeld, ownerPath)
	finalInfo, finalStatErr := os.Lstat(ownerPath)
	var restoredOwner writerOwnerRecord
	finalReadErr := readRecord(ownerPath, &restoredOwner)
	ownerIdentityRepeated := oldStatErr == nil && finalStatErr == nil && os.SameFile(oldInfo, finalInfo)
	fullRestorationObserved := removeReplacementErr == nil && restoreLockErr == nil && lockIdentityRepeated && mkdirErr == nil && oldWriteErr == nil &&
		holdErr == nil && newWriteErr == nil && middleChanged && removeNewErr == nil && restoreOwnerErr == nil && ownerIdentityRepeated &&
		finalReadErr == nil && restoredOwner == oldOwner
	if !fullRestorationObserved {
		abaStatus = Failed
	}
	add(Scenario{ID: "VAL-030/ABA", Requirements: []string{"VAL-030", "ARCH-023", "ARCH-031"}, Status: abaStatus, Fixture: "stable lock and owner paths forced through complete A-to-B-to-A inode and byte restoration", Lease: "A-X+W-X", Writes: []string{"writer-substitution.lock", "owner-record-aba/writer-owner.json"}, Observation: fmt.Sprintf("full lock+owner A-B-A restoration observed=%t; endpoint inode/bytes/generation/token comparisons cannot reveal the intervening authority and therefore do not prove ABA exclusion", fullRestorationObserved), RejectedAssumption: "A-versus-B inequality, or equality at two endpoints, proves an original holder cannot advance after full authority restoration; a normative threat model must forbid synchronization-inode/owner-record restoration by an actor able to rename within the protected parent, or add an independently cumulative non-restorable authority"})
}
