package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// PC-022/050: native object syntax is separate from materialized content digests.
func TestCandidateNativeSourceObjectIDs(t *testing.T) {
	for _, test := range []struct {
		name       string
		commit     string
		root       string
		wantReject bool
	}{
		{"sha1", strings.Repeat("c", 40), strings.Repeat("a", 40), false},
		{"sha256-syntax", strings.Repeat("c", 64), strings.Repeat("a", 64), false},
		{"materialized-root", strings.Repeat("c", 40), proofDigest("a"), true},
		{"materialized-commit", proofDigest("c"), strings.Repeat("a", 64), true},
		{"sha1-commit-sha256-root", strings.Repeat("c", 40), strings.Repeat("a", 64), true},
		{"sha256-commit-sha1-root", strings.Repeat("c", 64), strings.Repeat("a", 40), true},
		{"uppercase-root", strings.Repeat("c", 40), strings.Repeat("A", 40), true},
		{"nonhex-root", strings.Repeat("c", 40), strings.Repeat("g", 40), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := baseCandidate()
			input.Source.Commit, input.Source.RootTree = test.commit, test.root
			candidate, digest, err := DigestCandidate(input)
			if (err != nil) != test.wantReject {
				t.Fatalf("source syntax rejected=%t want=%t", err != nil, test.wantReject)
			}
			if test.wantReject && (candidate.Schema != 0 || digest != "") {
				t.Fatal("invalid source syntax returned candidate authority")
			}
		})
	}
}

// PC-050: successful and rejected canonicalization preserve every caller-owned backing store.
func TestCandidatePopulatedCollectionsPreserveCaller(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(CandidatePreimage) error
	}{
		{"digest", func(input CandidatePreimage) error { _, _, err := DigestCandidate(input); return err }},
		{"encode", func(input CandidatePreimage) error { _, err := EncodeCandidate(input); return err }},
	} {
		for _, reject := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reject=%t", operation.name, reject), func(t *testing.T) {
				input := baseCandidate()
				for index := range input.Components {
					input.Components[index].Files = callerSliceWithSentinel(input.Components[index].Files, CanonicalFile{Path: "spare-file"})
				}
				input.Components = callerSliceWithSentinel(input.Components, CanonicalComponent{ID: "spare-component"})
				for index := range input.Tools {
					input.Tools[index].Locks = callerSliceWithSentinel(input.Tools[index].Locks, ToolLock{Name: "spare-lock"})
				}
				input.Tools = callerSliceWithSentinel(input.Tools, CanonicalTool{ID: "spare-tool"})
				input.HomeSources = callerSliceWithSentinel(input.HomeSources, "spare-home")
				input.Targets = callerSliceWithSentinel(input.Targets, "spare-target")
				input.Executables = callerSliceWithSentinel(input.Executables, CanonicalExecutable{ID: "spare-executable"})
				input.Policy.ObjectModes = callerSliceWithSentinel(input.Policy.ObjectModes, "spare-mode")
				input.Policy.Declarations = callerSliceWithSentinel(input.Policy.Declarations, "spare-declaration")
				input.Policy.TemplateActions = callerSliceWithSentinel(input.Policy.TemplateActions, "spare-action")
				input.Policy.ProcessClasses = callerSliceWithSentinel(input.Policy.ProcessClasses, "spare-class")
				input.Policy.PolicyIDs = callerSliceWithSentinel(input.Policy.PolicyIDs, "spare-policy")
				if reject {
					input.Policy.PolicyIDs[0] = "unsupported-policy"
				}
				before, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				backingBefore := candidateBackingSnapshot(t, input)
				err = operation.run(input)
				if (err != nil) != reject {
					t.Fatalf("rejected=%t want=%t", err != nil, reject)
				}
				after, err := json.Marshal(input)
				if err != nil || !bytes.Equal(before, after) {
					t.Error("canonicalization changed caller bytes")
				}
				if !bytes.Equal(backingBefore, candidateBackingSnapshot(t, input)) {
					t.Error("canonicalization changed caller backing storage or spare-capacity sentinels")
				}
			})
		}
	}
}

func callerSliceWithSentinel[T any](values []T, sentinel T) []T {
	out := make([]T, len(values)+1)
	copy(out, reverseCopy(values))
	out[len(values)] = sentinel
	return out[:len(values)]
}

func candidateBackingSnapshot(t *testing.T, candidate CandidatePreimage) []byte {
	t.Helper()
	candidate.Components = append([]CanonicalComponent{}, candidate.Components[:cap(candidate.Components)]...)
	for index := range candidate.Components {
		files := candidate.Components[index].Files
		candidate.Components[index].Files = files[:cap(files)]
	}
	candidate.Tools = append([]CanonicalTool{}, candidate.Tools[:cap(candidate.Tools)]...)
	for index := range candidate.Tools {
		locks := candidate.Tools[index].Locks
		candidate.Tools[index].Locks = locks[:cap(locks)]
	}
	candidate.HomeSources = candidate.HomeSources[:cap(candidate.HomeSources)]
	candidate.Targets = candidate.Targets[:cap(candidate.Targets)]
	candidate.Executables = candidate.Executables[:cap(candidate.Executables)]
	candidate.Policy.ObjectModes = candidate.Policy.ObjectModes[:cap(candidate.Policy.ObjectModes)]
	candidate.Policy.Declarations = candidate.Policy.Declarations[:cap(candidate.Policy.Declarations)]
	candidate.Policy.TemplateActions = candidate.Policy.TemplateActions[:cap(candidate.Policy.TemplateActions)]
	candidate.Policy.ProcessClasses = candidate.Policy.ProcessClasses[:cap(candidate.Policy.ProcessClasses)]
	candidate.Policy.PolicyIDs = candidate.Policy.PolicyIDs[:cap(candidate.Policy.PolicyIDs)]
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// PC-042: effective home-source and target counts apply to the complete candidate.
func TestCandidateAggregateHomeAndTargetBounds(t *testing.T) {
	for _, field := range []struct {
		name  string
		limit int
	}{{"home-sources", MaxHomeSources}, {"targets", MaxTargets}} {
		for _, count := range []int{field.limit - 1, field.limit, field.limit + 1} {
			t.Run(fmt.Sprintf("%s/%d", field.name, count), func(t *testing.T) {
				input := baseCandidate()
				base, selected := proofComponents()
				paths := make([]string, count)
				for index := range paths {
					paths[index] = fmt.Sprintf("bounded/path-%04d", index)
				}
				if field.name == "home-sources" {
					input.HomeSources = paths
					base.HomeSources, selected.HomeSources = paths[:count/2], paths[count/2:]
				} else {
					input.Targets = paths
					base.Targets, selected.Targets = paths[:count/2], paths[count/2:]
				}
				candidate, digest, err := DigestCandidate(input)
				if (err != nil) != (count > field.limit) {
					t.Fatalf("rejected=%t at count=%d", err != nil, count)
				}
				if err != nil && (candidate.Schema != 0 || digest != "") {
					t.Fatal("over-limit candidate returned authority")
				}
				candidate, digest, err = Compose(proofSource(), base, selected, proofExecutables(), proofPolicy())
				if (err != nil) != (count > field.limit) {
					t.Fatalf("composed candidate rejected=%t at count=%d", err != nil, count)
				}
				if err != nil && (candidate.Schema != 0 || digest != "") {
					t.Fatal("over-limit composition returned authority")
				}
			})
		}
	}
}

// Existing synthetic payload costs do not prove PC-042's complete reservation budget.
func TestCandidatePayloadFixturesRespectCollectionBounds(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		stringsInput, stringsBuilt := candidateWithStringCount(MaxTotalStrings + delta)
		bytesInput, bytesBuilt := candidateWithAllocatedBytes(MaxAllocatedBytes + delta)
		if !stringsBuilt || !bytesBuilt {
			t.Fatal("synthetic payload boundary fixture could not be built")
		}
		for _, input := range []CandidatePreimage{stringsInput, bytesInput} {
			if len(input.HomeSources) > MaxHomeSources || len(input.Targets) > MaxTargets {
				t.Fatal("synthetic payload fixture exceeded a collection count bound")
			}
		}
		count, _ := candidateCost(stringsInput)
		_, size := candidateCost(bytesInput)
		if count != MaxTotalStrings+delta || size != MaxAllocatedBytes+delta {
			t.Fatal("synthetic payload boundary fixture changed its modeled cost")
		}
	}
}

// PC-050: optional included collections have one empty encoding and retain caller ownership.
func TestCandidateEmptyCollectionsCanonicalizeToArrays(t *testing.T) {
	nilInput := baseCandidate()
	for index := range nilInput.Components {
		nilInput.Components[index].Files = nil
	}
	nilInput.Tools, nilInput.HomeSources, nilInput.Targets = nil, nil, nil
	emptyInput := cloneCandidate(nilInput)
	for index := range emptyInput.Components {
		emptyInput.Components[index].Files = []CanonicalFile{}
	}
	emptyInput.Tools = []CanonicalTool{}
	emptyInput.HomeSources, emptyInput.Targets = []string{}, []string{}
	normalized, nilDigest, err := DigestCandidate(nilInput)
	if err != nil {
		t.Fatal(err)
	}
	_, emptyDigest, err := DigestCandidate(emptyInput)
	if err != nil || nilDigest != emptyDigest {
		t.Fatalf("nil and empty collections changed identity: %v", err)
	}
	nilBytes, err := EncodeCandidate(nilInput)
	if err != nil {
		t.Fatal(err)
	}
	emptyBytes, err := EncodeCandidate(emptyInput)
	if err != nil || !bytes.Equal(nilBytes, emptyBytes) || bytes.Contains(nilBytes, []byte(":null")) {
		t.Fatalf("nil and empty collections changed canonical bytes: %v", err)
	}
	if nilInput.Tools != nil || nilInput.HomeSources != nil || nilInput.Targets != nil ||
		normalized.Tools == nil || normalized.HomeSources == nil || normalized.Targets == nil {
		t.Fatal("empty normalization changed the caller or retained null collections")
	}
	for index := range nilInput.Components {
		if nilInput.Components[index].Files != nil || normalized.Components[index].Files == nil {
			t.Fatal("nested empty normalization changed the caller or retained null files")
		}
	}
}
