package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

func encoderCandidateFixture(t *testing.T) receipt.Candidate {
	t.Helper()
	c := receipt.Candidate{Schema: 1, Domain: "zerb.candidate", Source: receipt.Source{URI: "https://example.com/repo.git", SourceID: strings.Repeat("a", 64), Commit: strings.Repeat("b", 40), RootTree: strings.Repeat("c", 40)}, ProfileID: "a-profile"}
	c.Source.SourceID = "sha256:" + c.Source.SourceID
	c.Parser = receipt.Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"}
	c.Policy = receipt.Policy{Schema: 1, Domain: "zerb.policy", ObjectModes: []string{"directory", "executable", "file"}, Declarations: []string{"home", "mise.lock", "mise.toml", "profile.toml"}, TemplateActions: []string{"if-false", "if-true", "literal-text"}, ProcessClasses: []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}, PolicyIDs: []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}, TargetMapperExecutableID: "chezmoi", ProviderFree: true, HostFactIndependent: true, DestinationRecheck: true, ExactReapplyOnly: true}
	c.Components = []receipt.Component{{ID: "a-profile", Path: "profiles/a-profile", Files: []receipt.File{}}, {ID: "base", Path: "base", Files: []receipt.File{{Path: "a", Mode: "directory"}, {Path: "quote\"<&\u2028/雪", Mode: "file", Hash: encoderFixtureToken()}}}}
	for i := range c.Components {
		b, err := json.Marshal(struct {
			Schema int            `json:"schema"`
			Domain string         `json:"domain"`
			Files  []receipt.File `json:"files"`
		}{1, "zerb.tree.v1", c.Components[i].Files})
		if err != nil {
			t.Fatal(err)
		}
		c.Components[i].TreeDigest = string(encoderFixtureDigest(b))
	}
	for i, id := range []string{"bash", "chezmoi", "git", "mise"} {
		c.Executables = append(c.Executables, receipt.Executable{ID: id, Version: []string{"5.3.15", "v2.70.5-zerb.1", "2.55.0", "2026.7.17"}[i], Path: "/opt/" + id, SHA256: encoderFixtureToken()})
	}
	c.Tools = []receipt.Tool{}
	c.HomeSources = []string{}
	c.Targets = []string{}
	return c
}
func encoderFixtureToken() string { return "sha256:" + strings.Repeat("0", 64) }
func encoderCompletedFixture(t *testing.T) receipt.Completed {
	t.Helper()
	c := encoderCandidateFixture(t)
	return receipt.Completed{Schema: 1, Domain: "zerb.completed", Candidate: receipt.Digest(encoderFixtureToken()), Source: c.Source, Components: c.Components, Zerb: receipt.Executable{ID: "zerb", Version: strings.Repeat("v", 200), Path: "/opt/zerb", SHA256: encoderFixtureToken()}, Git: c.Executables[2], Mise: c.Executables[3], Chezmoi: c.Executables[1], Parser: c.Parser, Policy: c.Policy, Platform: receipt.PlatformObservation{Distribution: "test", Kernel: "1", Architecture: "x86", Filesystem: "test"}, Tools: []receipt.ToolObservation{}, Targets: []string{}, Preconditions: []receipt.DestinationObservation{}, Results: []receipt.DestinationObservation{}, Phases: []string{"mutating-home-apply", "mutating-tool-install"}}
}

func encoderFixtureDigest(data []byte) receipt.Digest {
	hash := sha256.Sum256(data)
	return receipt.Digest("sha256:" + hex.EncodeToString(hash[:]))
}

func encoderPendingOracle(t *testing.T, facts receipt.Completed) (receipt.Digest, []byte) {
	t.Helper()
	digest, raw, err := receipt.ReceiptDigest(facts)
	if err != nil {
		t.Fatal(err)
	}
	var normalized receipt.Completed
	if err := json.Unmarshal(raw, &normalized); err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: digest, Facts: normalized})
	if err != nil {
		t.Fatal(err)
	}
	return digest, wire
}

func TestPendingEncoderDifferentialAndCallerOwnership(t *testing.T) {
	for _, previous := range []receipt.Digest{"", receipt.Digest(encoderFixtureToken())} {
		for bits := range 32 {
			facts := encoderCompletedFixture(t)
			facts.Previous = previous
			facts.Zerb.Version = "synthetic\"\\<&>\u2028\u2029雪😀\n"
			facts.Platform.Kernel = "kernel\t\r\b\f\u0001"
			facts.Targets = []string{"b", "a"}
			facts.Preconditions = []receipt.DestinationObservation{{Target: "b", Kind: "absent"}, {Target: "a", Kind: "absent"}}
			facts.Results = []receipt.DestinationObservation{{Target: "a", Exists: true, Kind: "file", Mode: 07777, Owner: math.MaxUint32, Device: math.MaxUint64, Inode: math.MaxUint64, Links: math.MaxUint64, ContentHash: encoderFixtureToken()}, {Target: "b", Exists: true, Kind: "directory"}}
			facts.Phases = []string{string(PhaseToolInstall), string(PhaseHomeApply)}
			facts.Dimensions = receipt.Dimensions{InputsPinned: bits&1 != 0, ArtifactIntegrity: bits&2 != 0, ArtifactAvailable: bits&4 != 0, HostCompatible: bits&8 != 0, ConvergenceVerified: bits&16 != 0}
			slices.Reverse(facts.Components[1].Files)
			slices.Reverse(facts.Components)
			slices.Reverse(facts.Policy.PolicyIDs)
			before, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			wantDigest, want := encoderPendingOracle(t, facts)
			gotDigest, got, err := encodePendingReceipt(facts)
			if err != nil || gotDigest != wantDigest || !bytes.Equal(got, want) || len(got) != cap(got) {
				t.Fatalf("differential pending mismatch: %v", err)
			}
			after, err := json.Marshal(facts)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("encoder changed caller")
			}
			if bytes.Contains(got, []byte(`:null`)) || bytes.HasSuffix(got, []byte("\n")) {
				t.Fatal("noncanonical pending wire")
			}
		}
	}
	for _, phases := range [][]string{nil, {}, {string(PhaseToolInstall)}, {string(PhaseHomeApply)}} {
		facts := encoderCompletedFixture(t)
		facts.Phases = phases
		wantDigest, want := encoderPendingOracle(t, facts)
		gotDigest, got, err := encodePendingReceipt(facts)
		if err != nil || gotDigest != wantDigest || !bytes.Equal(got, want) {
			t.Fatal("pure codec strengthened completion")
		}
	}
}

func TestPendingEncoderExactReservationAndHiddenCapacity(t *testing.T) {
	facts := encoderCompletedFixture(t)
	facts.Results = make([]receipt.DestinationObservation, 0, 1)
	var budget codecBudget
	var size codecJSONWriter
	size.completed(facts)
	if !budget.add(1, 3968) || !budget.reserveCompleted(facts, true) || !codecSourceScratch(&budget, uint64(len(facts.Source.URI))) ||
		!budget.add(size.size, 1) || !budget.add(128, 1) || !codecReserveTrees(&budget, facts.Components) {
		t.Fatal("fixture schedule failed")
	}
	hidden := facts.Results[:1]
	hidden[0].ContentHash = strings.Repeat("x", int(codecReservationLimit-budget.used))
	if _, _, err := encodePendingReceipt(facts); err != nil {
		t.Fatal("exact budget refused")
	}
	if _, _, err := receipt.ReceiptDigest(facts); err != nil {
		t.Fatal("oracle schedule differs")
	}
	hidden[0].ContentHash += "x"
	if allocations := testing.AllocsPerRun(5, func() {
		digest, wire, err := encodePendingReceipt(facts)
		if err != errCanonicalStateRecord || digest != "" || wire != nil {
			panic("over-budget input accepted")
		}
	}); allocations != 0 {
		t.Fatalf("preflight allocated %g", allocations)
	}
	facts = encoderCompletedFixture(t)
	facts.Components = append(facts.Components, receipt.Component{Files: make([]receipt.File, 0, 180000)})[:2]
	if allocations := testing.AllocsPerRun(5, func() {
		digest, wire, err := encodePendingReceipt(facts)
		if err != errCanonicalStateRecord || digest != "" || wire != nil {
			panic("hidden nested capacity accepted")
		}
	}); allocations != 0 {
		t.Fatalf("hidden capacity preflight allocated %g", allocations)
	}
}

func TestPendingEncoderLargeEscapedWitnessAndHistory(t *testing.T) {
	large := encoderCompletedFixture(t)
	large.Zerb.Version = strings.Repeat("<", 1_000_000)
	wantDigest, want := encoderPendingOracle(t, large)
	gotDigest, got, err := encodePendingReceipt(large)
	if err != nil || len(got) < 6_000_000 || gotDigest != wantDigest || !bytes.Equal(got, want) || cap(got) != len(got) {
		t.Fatalf("large escaped witness failed: %v", err)
	}
	for range 3 {
		small := encoderCompletedFixture(t)
		wantDigest, want = encoderPendingOracle(t, small)
		gotDigest, got, err = encodePendingReceipt(small)
		if err != nil || gotDigest != wantDigest || !bytes.Equal(got, want) || cap(got) != len(got) {
			t.Fatal("large history retained output capacity")
		}
	}
}

func TestPendingEncoderInvalidValuesDifferential(t *testing.T) {
	for _, mutate := range []func(*receipt.Completed){
		func(v *receipt.Completed) { v.Schema = 2 },
		func(v *receipt.Completed) { v.Source.URI = "https://EXAMPLE.com/repo" },
		func(v *receipt.Completed) { v.Components[0].TreeDigest = encoderFixtureToken() },
		func(v *receipt.Completed) { v.Components[0].Path = v.Components[1].Path },
		func(v *receipt.Completed) { v.Components[1].Files[0].Mode = "link" },
		func(v *receipt.Completed) { v.Git.Version = "latest" },
		func(v *receipt.Completed) { v.Platform.Kernel = string([]byte{255}) },
		func(v *receipt.Completed) { v.Targets = []string{"missing"} },
		func(v *receipt.Completed) { v.Phases = []string{string(PhaseHomeApply), string(PhaseHomeApply)} },
		func(v *receipt.Completed) { v.Policy.ExactReapplyOnly = false },
	} {
		facts := encoderCompletedFixture(t)
		mutate(&facts)
		before, err := json.Marshal(facts)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receipt.ReceiptDigest(facts); err == nil {
			t.Fatal("invalid test fixture")
		}
		digest, wire, err := encodePendingReceipt(facts)
		if digest != "" || wire != nil || err != errCanonicalStateRecord {
			t.Fatal("invalid value exposed partial output")
		}
		after, err := json.Marshal(facts)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("failed encoder changed input")
		}
	}
}

func encoderJournalFixture() journalRecord {
	return journalRecord{Schema: 1, Domain: "zerb.journal", Instance: strings.Repeat("1", 32), OperationID: strings.Repeat("2", 32),
		OperationToken: strings.Repeat("3", 32), WriterGeneration: 1, WriterToken: strings.Repeat("4", 32), Candidate: receipt.Digest(encoderFixtureToken()),
		Phase: PhasePrepared, Preconditions: []receipt.DestinationObservation{{Target: "b", Kind: "absent"}, {Target: "a", Kind: "absent"}},
		Results: []receipt.DestinationObservation{{Target: "a", Exists: true, Kind: "file", Mode: 0600, ContentHash: encoderFixtureToken()}, {Target: "b", Kind: "absent"}}, CompletedPhases: []Phase{}}
}

func TestJournalEncoderCanonicalAndLimitedScalarRelations(t *testing.T) {
	for _, generation := range []uint64{1, 2, 3, math.MaxUint64} {
		for _, predecessor := range []string{"", strings.Repeat("5", 32), strings.Repeat("1", 32), strings.Repeat("2", 32)} {
			journal := encoderJournalFixture()
			journal.WriterGeneration = generation
			journal.WriterPreviousToken = predecessor
			before := slices.Clone(journal.Preconditions)
			want := journal
			want.Preconditions = slices.Clone(want.Preconditions)
			slices.SortFunc(want.Preconditions, codecObservationOrder)
			wire, err := encodeJournalRecord(journal)
			if err != nil {
				t.Fatal("codec invented writer history relation:", err)
			}
			oracle, err := json.Marshal(want)
			if err != nil || !bytes.Equal(wire, oracle) || cap(wire) != len(wire) || !slices.Equal(before, journal.Preconditions) {
				t.Fatal("journal wire or ownership mismatch")
			}
		}
	}
	for _, mutate := range []func(*journalRecord){
		func(v *journalRecord) { v.Instance = "" },
		func(v *journalRecord) { v.WriterGeneration = 0 },
		func(v *journalRecord) { v.OperationToken = v.WriterToken },
		func(v *journalRecord) { v.WriterPreviousToken = v.OperationToken },
		func(v *journalRecord) { v.Previous = receipt.Digest(encoderFixtureToken()) },
		func(v *journalRecord) { v.CompletedPhases = []Phase{PhaseHomeApply, PhaseToolInstall} },
		func(v *journalRecord) { v.Results = nil },
		func(v *journalRecord) { v.Results[0].Kind = "link" },
	} {
		journal := encoderJournalFixture()
		mutate(&journal)
		wire, err := encodeJournalRecord(journal)
		if wire != nil || err != errCanonicalStateRecord {
			t.Fatal("invalid journal accepted")
		}
	}
	journal := encoderJournalFixture()
	journal.Results = append(journal.Results, receipt.DestinationObservation{ContentHash: strings.Repeat("x", 8_000_001)})[:2]
	if allocations := testing.AllocsPerRun(5, func() {
		wire, err := encodeJournalRecord(journal)
		if wire != nil || err != errCanonicalStateRecord {
			panic("hidden journal input accepted")
		}
	}); allocations != 0 {
		t.Fatalf("journal preflight allocated %g", allocations)
	}
}

func TestCodecOrderedSemanticChecksAndTreeComparison(t *testing.T) {
	candidate := encoderCandidateFixture(t)
	digest, wire, err := receipt.CandidateDigest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	var ordered receipt.Candidate
	if err := json.Unmarshal(wire, &ordered); err != nil {
		t.Fatal(err)
	}
	if !codecValidateCandidate(ordered) || digest == "" {
		t.Fatal("canonical candidate refused")
	}
	slices.Reverse(candidate.Components)
	if codecValidateCandidate(candidate) {
		t.Fatal("unsorted candidate admitted")
	}
	for _, component := range ordered.Components {
		var writer codecJSONWriter
		writer.tree(component.Files)
		if component.TreeDigest == "" || writer.failed {
			t.Fatal("invalid component fixture")
		}
	}
	old := append([]receipt.Component(nil), ordered.Components...)
	if allocations := testing.AllocsPerRun(10, func() { codecValidComponents(ordered.Components, ordered.ProfileID) }); allocations != 0 {
		t.Fatalf("ordered component validation allocated %g", allocations)
	}
	if !reflect.DeepEqual(old, ordered.Components) {
		t.Fatal("semantic validation changed components")
	}
	hash := sha256.Sum256([]byte("fixture"))
	if !codecTreeDigestMatches(string(encoderFixtureDigest([]byte("fixture"))), hash) || codecTreeDigestMatches(encoderFixtureToken(), hash) || codecTreeDigestMatches("invalid", hash) {
		t.Fatal("tree digest comparison mismatch")
	}
	if allocations := testing.AllocsPerRun(10, func() { codecTreeDigestMatches(string(digest), hash) }); allocations != 0 {
		t.Fatalf("tree comparison allocated %g", allocations)
	}
}

func TestPendingEncoderFullTargetInventoryAndCandidateStringLimit(t *testing.T) {
	facts := encoderCompletedFixture(t)
	for i := range 4096 {
		target := "t" + strings.Repeat("0", 4-len(strconv.Itoa(i))) + strconv.Itoa(i)
		facts.Targets = append(facts.Targets, target)
		facts.Preconditions = append(facts.Preconditions, receipt.DestinationObservation{Target: target, Kind: "absent"})
		facts.Results = append(facts.Results, receipt.DestinationObservation{Target: target, Kind: "absent"})
	}
	wantDigest, want := encoderPendingOracle(t, facts)
	digest, wire, err := encodePendingReceipt(facts)
	if err != nil || digest != wantDigest || !bytes.Equal(wire, want) {
		t.Fatal("completed facts acquired candidate-only string cap")
	}
	candidate := encoderCandidateFixture(t)
	for i := range candidate.Components {
		files := make([]receipt.File, 3000)
		for j := range files {
			files[j] = receipt.File{Path: "t" + strings.Repeat("0", 4-len(strconv.Itoa(j))) + strconv.Itoa(j), Mode: "directory"}
		}
		candidate.Components[i].Files = files
		preimage, err := json.Marshal(struct {
			Schema int            `json:"schema"`
			Domain string         `json:"domain"`
			Files  []receipt.File `json:"files"`
		}{1, "zerb.tree.v1", files})
		if err != nil {
			t.Fatal(err)
		}
		candidate.Components[i].TreeDigest = string(encoderFixtureDigest(preimage))
	}
	if _, _, err := receipt.CandidateDigest(candidate); err == nil || codecValidateCandidate(candidate) {
		t.Fatal("candidate string count widened")
	}
	facts = encoderCompletedFixture(t)
	facts.Components = candidate.Components
	wantDigest, want = encoderPendingOracle(t, facts)
	digest, wire, err = encodePendingReceipt(facts)
	if err != nil || digest != wantDigest || !bytes.Equal(wire, want) {
		t.Fatal("completed component inventory acquired candidate-only string cap")
	}
}

func TestCodecEncoderActualStorageAndNoGrowth(t *testing.T) {
	if unsafe.Sizeof(codecJSONWriter{}) > 48 || unsafe.Sizeof(codecBudget{}) > 8 || unsafe.Sizeof(pendingReceipt{}) > 888 || unsafe.Sizeof(journalRecord{}) > 248 {
		t.Fatal("actual codec values exceed approved fixed storage")
	}
	writer := codecJSONWriter{out: make([]byte, 0, 1), writing: true}
	writer.raw("ab")
	if !writer.failed || len(writer.out) != 0 || cap(writer.out) != 1 {
		t.Fatal("writer grew beyond reservation")
	}
	writer = codecJSONWriter{size: math.MaxUint64}
	writer.raw("a")
	if !writer.failed {
		t.Fatal("writer size wrapped")
	}
	for _, pair := range [][2]uint64{{math.MaxUint64, 2}, {2, math.MaxUint64}, {1, math.MaxUint64}} {
		var budget codecBudget
		if budget.add(pair[0], pair[1]) || budget.used != 0 {
			t.Fatal("reservation overflow")
		}
	}
}
