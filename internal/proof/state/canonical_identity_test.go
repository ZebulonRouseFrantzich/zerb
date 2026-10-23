//go:build linux

package state

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// PC-022/050: fixture object IDs retain native syntax and empty files encode as arrays.
func TestCandidateFixtureNativeObjectsAndEmptyFiles(t *testing.T) {
	candidate := candidateTemplate("proof-next")
	for _, object := range []string{candidate.Source.Commit, candidate.Source.RootTree} {
		if _, err := hex.DecodeString(object); err != nil || len(object) != 40 || strings.ToLower(object) != object {
			t.Fatal("fixture source object is not a native lowercase SHA-1 object ID")
		}
	}
	if !bytes.Contains(recordBytes(candidate), []byte(`"files":[]`)) || bytes.Contains(recordBytes(candidate), []byte(":null")) {
		t.Fatal("fixture empty collection did not encode as an array")
	}
}

// ARCH-026: this synthetic fixture binds the archive subject, not an invented installed-tree digest.
func TestReceiptIntegrityUsesAuthoredArchiveSubject(t *testing.T) {
	candidate := candidateTemplate("proof-next")
	digest := string(candidateDigestFor("proof-next"))
	facts := receiptFacts(digest)
	if len(facts.Tools) != 1 || facts.Tools[0].Integrity != candidate.Tools[0].Locks[0].Value ||
		!validCompletedReceipt(facts, digest, "") {
		t.Fatal("synthetic receipt did not bind the authored archive checksum")
	}
	facts.Tools[0].Integrity = "sha256:" + recordDigest("node-installed")
	if validCompletedReceipt(facts, digest, "") {
		t.Fatal("synthetic receipt admitted a different integrity subject")
	}
}

// PC-050/ARCH-024: receipt facts sort as digest inputs; journals retain execution prefixes.
func TestReceiptPhasesSortedWithoutReorderingJournalPrefixes(t *testing.T) {
	r := graphRefs{instance: "proof", journal: "operation", operationToken: "operation-token", writerToken: "writer-token", b: string(candidateDigestFor("proof-next"))}
	facts := receiptFacts(r.b)
	if !slicesEqual(facts.Phases, []string{string(phaseHomeApply), string(phaseToolInstall)}) ||
		!slicesEqual(mutationPhases, []string{string(phaseToolInstall), string(phaseHomeApply)}) ||
		!validCompletedReceipt(facts, r.b, "") {
		t.Fatal("receipt phases are not canonical independently of execution order")
	}
	facts.Phases = append([]string{}, mutationPhases...)
	if validCompletedReceipt(facts, r.b, "") {
		t.Fatal("receipt admitted phases in execution order")
	}
	for _, test := range []struct {
		phase     phase
		completed []string
	}{
		{phaseToolInstall, nil},
		{phaseToolInstall, mutationPhases[:1]},
		{phaseHomeApply, mutationPhases[:1]},
		{phaseHomeApply, mutationPhases},
	} {
		journal := transitionJournalRecord(r, "mutating", string(test.phase), test.completed, 1, r.writerToken)
		if !slicesEqual(journal.CompletedPhases, exactCompletedPhases(test.completed)) || !validJournalPrefix(journal) {
			t.Fatal("journal did not preserve the exact completed execution prefix")
		}
	}
	journal := transitionJournalRecord(r, "pending-receipt", "", receiptPhases(), 1, r.writerToken)
	if validJournalPrefix(journal) {
		t.Fatal("journal admitted lexically sorted phases instead of execution order")
	}
	prepared := transitionJournalRecord(r, "prepared", "", nil, 1, r.writerToken)
	if !validJournalPrefix(prepared) || !bytes.Contains(recordBytes(prepared), []byte(`"results":[]`)) ||
		!bytes.Contains(recordBytes(prepared), []byte(`"completed_phases":[]`)) {
		t.Fatal("prepared journal did not encode empty included collections as arrays")
	}
}
