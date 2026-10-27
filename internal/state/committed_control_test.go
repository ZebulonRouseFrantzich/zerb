package state

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// These synthetic records test value links, never source or host authority.
func committedTestFacts(t testing.TB, c receipt.Candidate, digest, previous receipt.Digest) receipt.Completed {
	t.Helper()
	v := codecTestCompleted(t)
	v.Candidate, v.Previous, v.Source, v.Components = digest, previous, c.Source, c.Components
	v.Parser, v.Policy, v.Targets = c.Parser, c.Policy, c.Targets
	v.Git, v.Mise, v.Chezmoi = c.Executables[2], c.Executables[3], c.Executables[1]
	v.Dimensions = receipt.Dimensions{InputsPinned: true, ArtifactIntegrity: true, HostCompatible: true, ConvergenceVerified: true}
	v.Tools = []receipt.ToolObservation{{ID: "node", Requested: "24.4.1", Installed: "24.4.1", Integrity: codecTestToken()}}
	v.Preconditions, v.Results = nil, nil
	for _, target := range c.Targets {
		v.Preconditions = append(v.Preconditions, receipt.DestinationObservation{Target: target, Kind: "absent"})
		v.Results = append(v.Results, receipt.DestinationObservation{Target: target, Exists: true, Kind: "directory", Mode: 0700, Owner: 1, Device: 2, Inode: 3, Links: 1})
	}
	return v
}

func committedTestCandidate(t testing.TB) receipt.Candidate {
	c := codecTestCandidate(t)
	c.Tools = []receipt.Tool{{ID: "node", Version: "24.4.1", Locks: []receipt.LockValue{{Name: "artifact", Value: codecTestToken()}}}}
	c.Targets = []string{"config/node", "settings"}
	c.HomeSources = []string{"dot_config/node"}
	return c
}

func committedTestPutReceipt(t testing.TB, in *committedControlInput, slot int, facts receipt.Completed) receipt.Digest {
	t.Helper()
	digest, wire, err := codecReferencePending(facts)
	if err != nil {
		t.Fatal(err)
	}
	in.Receipts[slot] = wire[:len(wire):len(wire)]
	if slot == 0 {
		in.Current.Receipt = digest
	}
	return digest
}

func committedTestPutJournal(t testing.TB, in *committedControlInput, j journalRecord) {
	t.Helper()
	if j.Preconditions == nil {
		j.Preconditions = []receipt.DestinationObservation{}
	}
	if j.Results == nil {
		j.Results = []receipt.DestinationObservation{}
	}
	if j.CompletedPhases == nil {
		j.CompletedPhases = []Phase{}
	}
	wire, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	in.Journal = wire[:len(wire):len(wire)]
}

func committedTestReadJournal(t testing.TB, in committedControlInput) journalRecord {
	t.Helper()
	j, err := decodeJournalRecord(in.Journal)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func committedTestReadReceipt(t testing.TB, in committedControlInput, slot int) receipt.Completed {
	t.Helper()
	r, err := decodePendingReceipt(in.Receipts[slot])
	if err != nil {
		t.Fatal(err)
	}
	return r.Facts
}

func committedTestInput(t testing.TB, prior bool) committedControlInput {
	t.Helper()
	c := committedTestCandidate(t)
	digest, wire, err := receipt.CandidateDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	in := committedControlInput{Current: currentFixture(), Writer: writerRecordFixture(3, "", false)}
	in.Current.WriterGeneration, in.Current.Candidate = 3, digest
	in.Candidates[0] = wire[:len(wire):len(wire)]
	j := journalRecord{Schema: 1, Domain: "zerb.journal", Instance: in.Current.Instance,
		OperationID: in.Current.OperationID, OperationToken: in.Current.OperationToken,
		WriterGeneration: 3, WriterToken: in.Writer.Token, Candidate: digest, Phase: PhaseCleanup,
		CompletedPhases: []Phase{PhaseToolInstall, PhaseHomeApply}}
	if prior {
		a := committedTestCandidate(t)
		a.ProfileID, a.Components[1].ID, a.Components[1].Path = "other", "other", "profiles/other"
		previous, priorWire, err := receipt.CandidateDigest(a)
		if err != nil {
			t.Fatal(err)
		}
		in.Candidates[1] = priorWire[:len(priorWire):len(priorWire)]
		j.Previous = previous
		j.PreviousReceipt = committedTestPutReceipt(t, &in, 1, committedTestFacts(t, a, previous, receipt.Digest(codecTestToken())))
	}
	facts := committedTestFacts(t, c, digest, j.Previous)
	committedTestPutReceipt(t, &in, 0, facts)
	j.Preconditions, j.Results = facts.Preconditions, facts.Results
	committedTestPutJournal(t, &in, j)
	return in
}

func committedTestAssert(t testing.TB, in committedControlInput, want error) {
	t.Helper()
	before := in
	var wires [5][]byte
	for i, raw := range [...][]byte{in.Candidates[0], in.Candidates[1], in.Receipts[0], in.Receipts[1], in.Journal} {
		wires[i] = bytes.Clone(raw)
	}
	got, err := readCommittedControl(in)
	if err != want {
		t.Fatalf("got error %v, want %v", err, want)
	}
	if err != nil && got != (committedControlLinks{}) {
		t.Fatal("failure returned partial links")
	}
	if err == nil {
		phase := Phase("")
		if in.Journal != nil {
			phase = committedTestReadJournal(t, in).Phase
		}
		wantLinks := committedControlLinks{Candidate: in.Current.Candidate, Receipt: in.Current.Receipt,
			OperationID: in.Current.OperationID, OperationToken: in.Current.OperationToken,
			WriterGeneration: in.Current.WriterGeneration, WriterToken: in.Current.WriterToken, Phase: phase}
		if got != wantLinks {
			t.Fatal("links did not preserve exact committed identity")
		}
	}
	if in.Current != before.Current || in.Writer != before.Writer {
		t.Fatal("typed records changed")
	}
	for i, raw := range [...][]byte{in.Candidates[0], in.Candidates[1], in.Receipts[0], in.Receipts[1], in.Journal} {
		if !bytes.Equal(raw, wires[i]) {
			t.Fatal("borrowed canonical wire changed")
		}
	}
}

func TestCommittedControlCleanupPrefixes(t *testing.T) {
	for _, phase := range []Phase{PhasePendingReceipt, PhaseCleanup} {
		for mask := 0; mask < 4; mask++ {
			in := committedTestInput(t, true)
			j := committedTestReadJournal(t, in)
			j.Phase = phase
			committedTestPutJournal(t, &in, j)
			if mask&1 != 0 {
				in.Candidates[1] = nil
			}
			if mask&2 != 0 {
				in.Receipts[1] = nil
			}
			want := errCommittedControlRecords
			if mask == 0 || phase == PhaseCleanup && (mask == 1 || mask == 3) {
				want = nil
			}
			committedTestAssert(t, in, want)
		}
	}
	committedTestAssert(t, committedTestInput(t, false), nil)
}

func TestCommittedControlPriorAliasesAndHistoricalFacts(t *testing.T) {
	in := committedTestInput(t, false)
	j := committedTestReadJournal(t, in)
	facts := committedTestReadReceipt(t, in, 0)
	facts.Previous = in.Current.Candidate
	committedTestPutReceipt(t, &in, 0, facts)
	j.Previous, j.PreviousReceipt = in.Current.Candidate, in.Current.Receipt
	committedTestPutJournal(t, &in, j)
	committedTestAssert(t, in, nil)
	for _, candidate := range []bool{false, true} {
		duplicate := in
		if candidate {
			duplicate.Candidates[1] = bytes.Clone(in.Candidates[0])
		} else {
			duplicate.Receipts[1] = bytes.Clone(in.Receipts[0])
		}
		committedTestAssert(t, duplicate, errCommittedControlRecords)
	}
	// Same candidate with a distinct prior receipt whose own predecessor is historical.
	prior := facts
	prior.Previous = receipt.Digest(codecTestToken())
	j.PreviousReceipt = committedTestPutReceipt(t, &in, 1, prior)
	committedTestPutJournal(t, &in, j)
	committedTestAssert(t, in, nil)
	prior.Dimensions.HostCompatible = false
	j.PreviousReceipt = committedTestPutReceipt(t, &in, 1, prior)
	committedTestPutJournal(t, &in, j)
	committedTestAssert(t, in, errCommittedControlRecords)
	// A receipt cannot alias across distinct candidate identities.
	in = committedTestInput(t, true)
	j = committedTestReadJournal(t, in)
	j.PreviousReceipt = in.Current.Receipt
	in.Receipts[1] = nil
	committedTestPutJournal(t, &in, j)
	committedTestAssert(t, in, errCommittedControlRecords)
}

func TestCommittedControlWriterAndJournalRelations(t *testing.T) {
	for _, mode := range []string{"same", "immediate transfer", "older transfer", "ahead", "max same", "max transfer", "released", "released max"} {
		t.Run(mode, func(t *testing.T) {
			in := committedTestInput(t, false)
			j := committedTestReadJournal(t, in)
			other, third := strings.Repeat("5", 32), strings.Repeat("6", 32)
			switch mode {
			case "immediate transfer":
				in.Writer = writerRecordFixture(4, in.Current.WriterToken, false)
				in.Writer.Token = other
				j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = 4, other, in.Current.WriterToken
			case "older transfer":
				in.Writer = writerRecordFixture(6, third, false)
				in.Writer.Token = other
				j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = 6, other, third
			case "ahead":
				j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = 4, other, in.Writer.Token
			case "max same":
				in.Current.WriterGeneration = math.MaxUint64
				in.Writer = writerRecordFixture(math.MaxUint64, third, false)
				j.WriterGeneration, j.WriterPreviousToken = math.MaxUint64, third
			case "max transfer":
				in.Current.WriterGeneration = math.MaxUint64 - 1
				in.Writer = writerRecordFixture(math.MaxUint64, in.Current.WriterToken, false)
				in.Writer.Token = other
				j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = math.MaxUint64, other, in.Current.WriterToken
			case "released":
				in.Writer = writerRecordFixture(4, in.Current.WriterToken, true)
				in.Journal = nil
			case "released max":
				in.Current.WriterGeneration = math.MaxUint64 - 1
				in.Writer = writerRecordFixture(math.MaxUint64, in.Current.WriterToken, true)
				in.Journal = nil
			}
			if in.Journal != nil {
				committedTestPutJournal(t, &in, j)
			}
			committedTestAssert(t, in, nil)
		})
	}
	for _, change := range []func(*committedControlInput, *journalRecord){
		func(in *committedControlInput, j *journalRecord) { j.WriterPreviousToken = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { j.WriterToken = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { j.OperationID = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { j.OperationToken = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { j.Instance = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { j.Phase, j.CompletedPhases = PhasePrepared, nil },
		func(in *committedControlInput, j *journalRecord) { j.WriterGeneration = 5 },
		func(in *committedControlInput, j *journalRecord) {
			j.WriterGeneration, j.WriterPreviousToken = 4, in.Writer.Token
			j.WriterToken = strings.Repeat("5", 32)
			in.Current.WriterGeneration, in.Current.WriterToken = j.WriterGeneration, j.WriterToken
		},
		func(in *committedControlInput, j *journalRecord) { in.Writer.Instance = strings.Repeat("5", 32) },
		func(in *committedControlInput, j *journalRecord) { in.Writer.Process = processIdentity{} },
	} {
		in := committedTestInput(t, false)
		j := committedTestReadJournal(t, in)
		change(&in, &j)
		committedTestPutJournal(t, &in, j)
		committedTestAssert(t, in, errCommittedControlRecords)
	}
	// Typed PID/start fields are shape facts; no process liveness query is implied.
	in := committedTestInput(t, false)
	in.Writer.Process = processIdentity{math.MaxInt32, math.MaxUint64, math.MaxUint64}
	committedTestAssert(t, in, nil)
}

func TestCommittedControlReceiptLinks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*receipt.Completed)
	}{
		{"candidate", func(v *receipt.Completed) { v.Candidate = receipt.Digest(codecTestToken()) }},
		{"source", func(v *receipt.Completed) { v.Source.URI = "https://example.com/other.git" }},
		{"source ID", func(v *receipt.Completed) { v.Source.SourceID = "sha256:" + strings.Repeat("a", 64) }},
		{"source root tree", func(v *receipt.Completed) { v.Source.RootTree = strings.Repeat("d", 40) }},
		{"components", func(v *receipt.Completed) { v.Components[1].Path = "different" }},
		{"component files", func(v *receipt.Completed) {
			component := &v.Components[1]
			component.Files[0].Hash = "sha256:" + strings.Repeat("a", 64)
			tree, err := json.Marshal(struct {
				Schema int            `json:"schema"`
				Domain string         `json:"domain"`
				Files  []receipt.File `json:"files"`
			}{1, "zerb.tree.v1", component.Files})
			if err != nil {
				t.Fatal(err)
			}
			component.TreeDigest = string(codecTestHash(tree))
		}},
		{"targets", func(v *receipt.Completed) {
			v.Targets[0] = "config/other"
			v.Preconditions[0].Target, v.Results[0].Target = v.Targets[0], v.Targets[0]
		}},
		{"git", func(v *receipt.Completed) { v.Git.Path = "/other/git" }},
		{"mise", func(v *receipt.Completed) { v.Mise.SHA256 = "sha256:" + strings.Repeat("a", 64) }},
		{"chezmoi", func(v *receipt.Completed) { v.Chezmoi.Path = "/other/chezmoi" }},
		{"tool inventory", func(v *receipt.Completed) { v.Tools = nil }},
		{"artifact lock", func(v *receipt.Completed) { v.Tools[0].Integrity = "sha256:" + strings.Repeat("a", 64) }},
		{"prior", func(v *receipt.Completed) { v.Previous = receipt.Digest(codecTestToken()) }},
		{"preconditions", func(v *receipt.Completed) { v.Preconditions[0] = v.Results[0] }},
		{"results", func(v *receipt.Completed) { v.Results[0].Inode++ }},
		{"phase", func(v *receipt.Completed) { v.Phases = []string{string(PhaseToolInstall)} }},
		{"pinned", func(v *receipt.Completed) { v.Dimensions.InputsPinned = false }},
		{"integrity", func(v *receipt.Completed) { v.Dimensions.ArtifactIntegrity = false }},
		{"host", func(v *receipt.Completed) { v.Dimensions.HostCompatible = false }},
		{"convergence", func(v *receipt.Completed) { v.Dimensions.ConvergenceVerified = false }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := committedTestInput(t, false)
			facts := committedTestReadReceipt(t, in, 0)
			tt.change(&facts)
			committedTestPutReceipt(t, &in, 0, facts)
			committedTestAssert(t, in, errCommittedControlRecords)
		})
	}
	// Availability, observed platform and ZERB identity are independent receipt facts.
	in := committedTestInput(t, false)
	facts := committedTestReadReceipt(t, in, 0)
	facts.Dimensions.ArtifactAvailable = true
	facts.Platform.Kernel = "another"
	facts.Zerb.Version = "another"
	committedTestPutReceipt(t, &in, 0, facts)
	committedTestAssert(t, in, nil)
}

func TestCommittedControlEmptyAndCandidateOnlyInventories(t *testing.T) {
	in := committedTestInput(t, false)
	c, _, err := decodeCandidateRecord(in.Candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	c.Targets, c.Tools = nil, nil
	// Neither field has a receipt counterpart; the candidate still binds both.
	c.HomeSources = []string{"another/source"}
	c.Executables[0].Path = "/another/bash"
	digest, wire, err := receipt.CandidateDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	in.Current.Candidate, in.Candidates[0] = digest, wire
	facts := committedTestFacts(t, c, digest, "")
	facts.Tools = nil
	committedTestPutReceipt(t, &in, 0, facts)
	j := committedTestReadJournal(t, in)
	j.Candidate, j.Preconditions, j.Results = digest, nil, nil
	committedTestPutJournal(t, &in, j)
	committedTestAssert(t, in, nil)
	// A cleaned current keeps its receipt's historical Previous after prior removal.
	in = committedTestInput(t, true)
	in.Journal, in.Candidates[1], in.Receipts[1] = nil, nil, nil
	in.Writer = writerRecordFixture(4, in.Current.WriterToken, true)
	committedTestAssert(t, in, nil)
	// A transferred recovery owner can remain after the journal was removed.
	in.Writer = writerRecordFixture(4, in.Current.WriterToken, false)
	in.Writer.Token = strings.Repeat("5", 32)
	committedTestAssert(t, in, nil)
}

func TestCommittedControlCanonicalAndSelectedSlots(t *testing.T) {
	for slot := 0; slot < 5; slot++ {
		for _, malformed := range [][]byte{{}, []byte("null"), []byte("{}"), []byte(" ")} {
			in := committedTestInput(t, true)
			raws := [...]*[]byte{&in.Candidates[0], &in.Candidates[1], &in.Receipts[0], &in.Receipts[1], &in.Journal}
			*raws[slot] = append([]byte{}, malformed...)
			committedTestAssert(t, in, errCommittedControlRecords)
		}
	}
	for _, suffix := range []string{"\n", "{}", "null"} {
		in := committedTestInput(t, false)
		in.Candidates[0] = append(bytes.Clone(in.Candidates[0]), suffix...)
		committedTestAssert(t, in, errCommittedControlRecords)
	}
	in := committedTestInput(t, true)
	in.Journal = nil
	committedTestAssert(t, in, errCommittedControlRecords)
	in = committedTestInput(t, false)
	in.Journal = nil
	committedTestAssert(t, in, nil)
	for _, change := range []func(*committedControlInput){
		func(in *committedControlInput) { in.Candidates[0] = nil },
		func(in *committedControlInput) { in.Receipts[0] = nil },
		func(in *committedControlInput) { in.Current = currentRecord{} },
		func(in *committedControlInput) { in.Current.Receipt = in.Current.Candidate },
	} {
		in = committedTestInput(t, false)
		change(&in)
		committedTestAssert(t, in, errCommittedControlRecords)
	}
}

func TestCommittedControlResourcePreflight(t *testing.T) {
	in := committedTestInput(t, true)
	// Five disjoint visible capacities exhaust the exact aggregate boundary.
	backing := make([]byte, int(committedControlWireBudget)+1)
	raws := [...]*[]byte{&in.Candidates[0], &in.Candidates[1], &in.Receipts[0], &in.Receipts[1], &in.Journal}
	for i, raw := range raws {
		start := i * int(codecReservationLimit)
		copy(backing[start:], *raw)
		*raw = backing[start : start+len(*raw) : start+int(codecReservationLimit)]
	}
	counters, err := reserveCommittedControl(in)
	if err != nil || counters.Borrowed != committedControlWireBudget || counters.Reservation > committedControlReservationLimit {
		t.Fatalf("exact capacity boundary: %+v, %v", counters, err)
	}
	last := raws[4]
	*last = backing[4*int(codecReservationLimit) : 4*int(codecReservationLimit)+len(*last) : len(backing)]
	committedTestAssert(t, in, errCommittedControlResource)
	if allocations := testing.AllocsPerRun(3, func() { _, _ = readCommittedControl(in) }); allocations != 0 {
		t.Fatalf("upfront resource refusal allocated %g", allocations)
	}
	// Identical overlapping views still count five times; pointer dedup is invalid.
	in = committedTestInput(t, false)
	shared := backing[: 1 : int(codecReservationLimit)+1]
	in.Candidates, in.Receipts, in.Journal = [2][]byte{shared, shared}, [2][]byte{shared, shared}, shared
	committedTestAssert(t, in, errCommittedControlResource)
	// An empty but nonnil optional view still carries its visible retained capacity.
	in = committedTestInput(t, false)
	in.Candidates[1] = backing[:0]
	committedTestAssert(t, in, errCommittedControlResource)
	// A later over-budget wire refuses before decoding earlier valid wires.
	in = committedTestInput(t, false)
	facts := committedTestReadReceipt(t, in, 0)
	facts.Zerb.Version = strings.Repeat("<", 1_200_000)
	wire, marshalErr := json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: receipt.Digest(codecTestToken()), Facts: facts})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	in.Receipts[1] = wire
	committedTestAssert(t, in, errCommittedControlResource)
	if allocations := testing.AllocsPerRun(3, func() { _, _ = readCommittedControl(in) }); allocations != 0 {
		t.Fatalf("later codec-budget refusal allocated %g", allocations)
	}
	if counters, err := reserveCommittedControl(in); err != errCommittedControlResource || counters != (committedControlCounters{}) {
		t.Fatal("resource refusal returned partial counters")
	}
}

func TestCommittedControlPreflightSchemaAndLateGrammar(t *testing.T) {
	in := committedTestInput(t, false)
	c := committedTestCandidate(t)
	c.Source.URI = "https://example.com/" + strings.Repeat("a", 2048)
	wire, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	in.Candidates[1] = wire
	committedTestAssert(t, in, errCommittedControlRecords)
	in = committedTestInput(t, false)
	in.Journal = []byte("{}")
	committedTestAssert(t, in, errCommittedControlRecords)
	if allocations := testing.AllocsPerRun(3, func() { _, _ = readCommittedControl(in) }); allocations != 0 {
		t.Fatalf("late grammar refusal allocated %g", allocations)
	}
	if counters, err := reserveCommittedControl(in); err != errCommittedControlRecords || counters != (committedControlCounters{}) {
		t.Fatal("grammar refusal returned partial counters")
	}
}

func FuzzCommittedControlReceiptCorrespondence(f *testing.F) {
	for i := uint8(0); i < 10; i++ {
		f.Add(i, uint64(i))
	}
	f.Fuzz(func(t *testing.T, selector uint8, value uint64) {
		in := committedTestInput(t, false)
		facts := committedTestReadReceipt(t, in, 0)
		before := codecTightCompleted(facts)
		want := error(nil)
		switch selector % 10 {
		case 0:
			facts.Dimensions.ArtifactAvailable = value&1 != 0
		case 1:
			facts.Platform.Kernel = strings.Repeat("k", int(value%32)+1)
		case 2:
			facts.Zerb.Version = strings.Repeat("z", int(value%32)+1)
		case 3:
			facts.Results[0].Inode += value%100 + 1
			want = errCommittedControlRecords
		case 4:
			facts.Source.URI = "https://example.com/other.git"
			want = errCommittedControlRecords
		case 5:
			facts.Tools[0].Integrity = "sha256:" + strings.Repeat("a", 64)
			want = errCommittedControlRecords
		case 6, 7, 8, 9:
			dimensions := []*bool{&facts.Dimensions.InputsPinned, &facts.Dimensions.ArtifactIntegrity, &facts.Dimensions.HostCompatible, &facts.Dimensions.ConvergenceVerified}
			*dimensions[int(selector%10)-6] = value&1 != 0
			if value&1 == 0 {
				want = errCommittedControlRecords
			}
		}
		committedTestPutReceipt(t, &in, 0, facts)
		committedTestAssert(t, in, want)
		// Encoding and reading may not sort or mutate the retained caller model.
		if selector%10 <= 2 && (!reflect.DeepEqual(before.Components, facts.Components) || !reflect.DeepEqual(before.Tools, facts.Tools)) {
			t.Fatal("receipt inputs changed")
		}
	})
}
