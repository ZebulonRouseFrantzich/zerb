package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// ARCH-024/025/026/027/031: synthetic selected values grant no host authority.
// Build the prior/current relationship independently of the committed reader.
func precommitTestInput(t testing.TB, prior string, phase Phase, pending bool) committedControlInput {
	t.Helper()
	b := committedTestCandidate(t)
	bd, bw, err := receipt.CandidateDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	in := committedControlInput{Writer: writerRecordFixture(3, "", false)}
	in.Writer.Token = strings.Repeat("5", 32)
	in.Candidates[0] = bw[:len(bw):len(bw)]
	j := journalRecord{Schema: 1, Domain: "zerb.journal", Instance: in.Writer.Instance,
		OperationID: strings.Repeat("6", 32), OperationToken: strings.Repeat("7", 32),
		WriterGeneration: in.Writer.Generation, WriterToken: in.Writer.Token,
		Candidate: bd, Phase: phase}
	if prior != "" {
		a, ad := b, bd
		if prior == "different" {
			a = committedTestCandidate(t)
			a.ProfileID, a.Components[1].ID, a.Components[1].Path = "other", "other", "profiles/other"
			var aw []byte
			ad, aw, err = receipt.CandidateDigest(a)
			if err != nil {
				t.Fatal(err)
			}
			in.Candidates[1] = aw[:len(aw):len(aw)]
		}
		in.Current = currentFixture()
		in.Current.Candidate = ad
		in.Current.Receipt = precommitTestPutReceipt(t, &in, 1, committedTestFacts(t, a, ad, receipt.Digest(codecTestToken())))
		j.Previous, j.PreviousReceipt = ad, in.Current.Receipt
	}
	facts := committedTestFacts(t, b, bd, j.Previous)
	j.Preconditions, j.Results = facts.Preconditions, facts.Results
	switch phase {
	case PhaseHomeApply:
		j.CompletedPhases = []Phase{PhaseToolInstall}
	case PhasePendingReceipt:
		j.CompletedPhases = []Phase{PhaseToolInstall, PhaseHomeApply}
	}
	if pending {
		precommitTestPutReceipt(t, &in, 0, facts)
	}
	committedTestPutJournal(t, &in, j)
	return in
}

func precommitTestPutReceipt(t testing.TB, in *committedControlInput, slot int, facts receipt.Completed) receipt.Digest {
	t.Helper()
	d, raw, err := codecReferencePending(facts)
	if err != nil {
		t.Fatal(err)
	}
	in.Receipts[slot] = raw[:len(raw):len(raw)]
	return d
}

func precommitTestJournal(t testing.TB, in committedControlInput) journalRecord {
	t.Helper()
	var j journalRecord
	if err := json.Unmarshal(in.Journal, &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func precommitTestFacts(t testing.TB, in committedControlInput, slot int) receipt.Completed {
	t.Helper()
	v, ok := codecReferencePendingDecode(in.Receipts[slot])
	if !ok {
		t.Fatal("invalid oracle receipt fixture")
	}
	return v.Facts
}

func precommitTestWires(in *committedControlInput) [5]*[]byte {
	return [5]*[]byte{&in.Candidates[0], &in.Candidates[1], &in.Receipts[0], &in.Receipts[1], &in.Journal}
}

func precommitTestAssert(t testing.TB, in committedControlInput, want error, pending receipt.Digest) {
	t.Helper()
	before := in
	var wires [5][]byte
	for i, p := range precommitTestWires(&in) {
		wires[i] = bytes.Clone(*p)
	}
	got, err := readPrecommitControl(in)
	if err != want {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if err != nil {
		if got != (committedControlLinks{}) {
			t.Fatal("refusal returned partial links")
		}
	} else {
		j := precommitTestJournal(t, in)
		wantLinks := committedControlLinks{Candidate: j.Candidate, Receipt: pending,
			OperationID: j.OperationID, OperationToken: j.OperationToken,
			WriterGeneration: j.WriterGeneration, WriterToken: j.WriterToken, Phase: j.Phase}
		if got != wantLinks {
			t.Fatal("returned links differ from selected journal and receipt")
		}
	}
	if in.Current != before.Current || in.Writer != before.Writer {
		t.Fatal("typed caller records changed")
	}
	for i, p := range precommitTestWires(&in) {
		if !bytes.Equal(*p, wires[i]) {
			t.Fatal("borrowed caller bytes changed")
		}
	}
}

func precommitTestPendingDigest(t testing.TB, in committedControlInput) receipt.Digest {
	t.Helper()
	var r pendingReceipt
	if err := json.Unmarshal(in.Receipts[0], &r); err != nil {
		t.Fatal(err)
	}
	return r.Digest
}

func TestPrecommitControlPhasesAndCheckpoints(t *testing.T) {
	phases := []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt, PhaseCleanup}
	prefixes := [][]Phase{nil, {PhaseToolInstall}, {PhaseToolInstall, PhaseHomeApply}, {PhaseHomeApply}, {PhaseHomeApply, PhaseToolInstall}, {PhaseToolInstall, PhaseToolInstall}}
	for _, prior := range []string{"", "same", "different"} {
		for _, phase := range phases {
			for checkpoint, prefix := range prefixes {
				t.Run(fmt.Sprintf("%s/%s/%d", prior, phase, checkpoint), func(t *testing.T) {
					in := precommitTestInput(t, prior, phase, false)
					j := precommitTestJournal(t, in)
					j.CompletedPhases = prefix
					committedTestPutJournal(t, &in, j)
					allowed := phase == PhasePrepared && checkpoint == 0 || phase == PhaseToolInstall && checkpoint <= 1 ||
						phase == PhaseHomeApply && (checkpoint == 1 || checkpoint == 2) || phase == PhasePendingReceipt && checkpoint == 2
					var want error
					if !allowed {
						want = errPrecommitControlRecords
					}
					precommitTestAssert(t, in, want, "")
				})
			}
		}
	}
	for _, phase := range phases[:4] {
		for _, field := range []string{"preconditions", "results"} {
			in := precommitTestInput(t, "different", phase, false)
			j := precommitTestJournal(t, in)
			if field == "preconditions" {
				j.Preconditions = j.Preconditions[:1]
			} else {
				j.Results = j.Results[:1]
			}
			committedTestPutJournal(t, &in, j)
			precommitTestAssert(t, in, errPrecommitControlRecords, "")
		}
	}
}

func TestPrecommitControlWriterDistancesAndFreshness(t *testing.T) {
	for _, transferred := range []bool{false, true} {
		for distance := uint64(0); distance <= 4; distance++ {
			for _, ahead := range []bool{false, true} {
				in := precommitTestInput(t, "same", PhasePrepared, false)
				previous := ""
				if transferred {
					previous = strings.Repeat("8", 32)
				}
				in.Writer = writerRecordFixture(1+distance, previous, false)
				in.Writer.Token = strings.Repeat("5", 32)
				j := precommitTestJournal(t, in)
				j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = in.Writer.Generation, in.Writer.Token, previous
				if ahead {
					j.WriterGeneration++
					j.WriterPreviousToken, j.WriterToken = in.Writer.Token, strings.Repeat("9", 32)
				}
				committedTestPutJournal(t, &in, j)
				var want error
				if distance < 2 || transferred && distance < 3 {
					want = errPrecommitControlRecords
				}
				precommitTestAssert(t, in, want, "")
			}
		}
	}
	for _, generation := range []uint64{1, 3, math.MaxUint64} {
		for _, prior := range []string{"", "same"} {
			in := precommitTestInput(t, prior, PhasePrepared, false)
			in.Writer = writerRecordFixture(generation, "", false)
			in.Writer.Token = strings.Repeat("5", 32)
			j := precommitTestJournal(t, in)
			j.WriterGeneration = generation
			committedTestPutJournal(t, &in, j)
			var want error
			if prior != "" && generation == 1 {
				want = errPrecommitControlRecords
			}
			precommitTestAssert(t, in, want, "")
		}
	}
	for _, change := range []func(*committedControlInput, *journalRecord){
		func(in *committedControlInput, j *journalRecord) { j.OperationID = in.Current.OperationID },
		func(in *committedControlInput, j *journalRecord) { j.OperationToken = in.Current.OperationToken },
		func(in *committedControlInput, j *journalRecord) {
			j.OperationID, j.OperationToken = in.Current.OperationID, in.Current.OperationToken
		},
		func(in *committedControlInput, j *journalRecord) { j.WriterToken = in.Current.WriterToken },
		func(in *committedControlInput, j *journalRecord) {
			in.Writer.Token, j.WriterToken = in.Current.WriterToken, in.Current.WriterToken
		},
		func(in *committedControlInput, j *journalRecord) {
			in.Writer.PreviousToken, j.WriterPreviousToken = in.Current.WriterToken, in.Current.WriterToken
		},
		func(in *committedControlInput, j *journalRecord) {
			j.WriterGeneration += 2
			j.WriterPreviousToken, j.WriterToken = in.Writer.Token, strings.Repeat("9", 32)
		},
		func(in *committedControlInput, j *journalRecord) {
			j.WriterGeneration++
			j.WriterPreviousToken, j.WriterToken = strings.Repeat("8", 32), strings.Repeat("9", 32)
		},
		func(in *committedControlInput, j *journalRecord) { j.WriterPreviousToken = strings.Repeat("8", 32) },
		func(in *committedControlInput, j *journalRecord) { in.Writer.Status = "released" },
		func(in *committedControlInput, j *journalRecord) { in.Writer.PreviousGeneration-- },
		func(in *committedControlInput, j *journalRecord) { in.Writer.Process.PID = 0 },
		func(in *committedControlInput, j *journalRecord) { j.Instance = strings.Repeat("a", 32) },
		func(in *committedControlInput, j *journalRecord) { in.Current.Instance = strings.Repeat("a", 32) },
	} {
		in := precommitTestInput(t, "same", PhasePrepared, false)
		j := precommitTestJournal(t, in)
		change(&in, &j)
		committedTestPutJournal(t, &in, j)
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	// A journal one ahead must never compensate for the retained owner's short distance.
	in := precommitTestInput(t, "same", PhasePrepared, false)
	in.Writer = writerRecordFixture(math.MaxUint64, "", false)
	in.Writer.Token = strings.Repeat("5", 32)
	in.Current.WriterGeneration = math.MaxUint64 - 2
	j := precommitTestJournal(t, in)
	j.WriterGeneration = math.MaxUint64
	committedTestPutJournal(t, &in, j)
	precommitTestAssert(t, in, nil, "")
	in.Current.WriterGeneration++
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
	for _, ahead := range []bool{false, true} {
		in := precommitTestInput(t, "same", PhasePrepared, false)
		generation := uint64(math.MaxUint64)
		if ahead {
			generation--
		}
		in.Current.WriterGeneration = generation - 3
		in.Writer = writerRecordFixture(generation, strings.Repeat("8", 32), false)
		in.Writer.Token = strings.Repeat("5", 32)
		j := precommitTestJournal(t, in)
		j.WriterGeneration, j.WriterPreviousToken = generation, in.Writer.PreviousToken
		if ahead {
			j.WriterGeneration++
			j.WriterPreviousToken, j.WriterToken = in.Writer.Token, strings.Repeat("9", 32)
		}
		committedTestPutJournal(t, &in, j)
		precommitTestAssert(t, in, nil, "")
		in.Current.WriterGeneration++
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	for _, generation := range []uint64{2, 3, math.MaxUint64} {
		in := precommitTestInput(t, "", PhasePrepared, false)
		in.Writer = writerRecordFixture(generation, strings.Repeat("8", 32), false)
		in.Writer.Token = strings.Repeat("5", 32)
		j := precommitTestJournal(t, in)
		j.WriterGeneration, j.WriterPreviousToken = generation, in.Writer.PreviousToken
		committedTestPutJournal(t, &in, j)
		precommitTestAssert(t, in, nil, "")
		if generation == math.MaxUint64 {
			// A future generation cannot wrap the maximum retained generation.
			j.WriterGeneration, j.WriterPreviousToken, j.WriterToken = 1, in.Writer.Token, strings.Repeat("9", 32)
			committedTestPutJournal(t, &in, j)
			precommitTestAssert(t, in, errPrecommitControlRecords, "")
		}
	}
	for _, corrupt := range []func(*writerOwner){
		func(w *writerOwner) { w.Process.Start = 0 },
		func(w *writerOwner) { w.PreviousGeneration = 0 },
		func(w *writerOwner) { w.Domain = "zerb.other" },
	} {
		in := precommitTestInput(t, "same", PhasePrepared, false)
		j := precommitTestJournal(t, in)
		j.WriterGeneration++
		j.WriterPreviousToken, j.WriterToken = in.Writer.Token, strings.Repeat("9", 32)
		committedTestPutJournal(t, &in, j)
		corrupt(&in.Writer)
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	for _, reused := range []string{strings.Repeat("1", 32), strings.Repeat("8", 32)} {
		in := precommitTestInput(t, "same", PhasePrepared, false)
		in.Writer = writerRecordFixture(4, strings.Repeat("8", 32), false)
		in.Writer.Token = strings.Repeat("5", 32)
		j := precommitTestJournal(t, in)
		j.WriterGeneration, j.WriterPreviousToken, j.WriterToken = 5, in.Writer.Token, reused
		committedTestPutJournal(t, &in, j)
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	in = precommitTestInput(t, "same", PhasePrepared, false)
	j = precommitTestJournal(t, in)
	j.WriterGeneration++
	j.WriterPreviousToken, j.WriterToken = in.Writer.Token, in.Current.WriterToken
	committedTestPutJournal(t, &in, j)
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
	in = precommitTestInput(t, "same", PhasePrepared, false)
	in.Writer = writerRecordFixture(4, in.Current.WriterToken, false)
	in.Writer.Token = strings.Repeat("5", 32)
	j = precommitTestJournal(t, in)
	j.WriterGeneration, j.WriterPreviousToken = in.Writer.Generation, in.Current.WriterToken
	committedTestPutJournal(t, &in, j)
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
}

func TestPrecommitControlPriorAndPendingAliases(t *testing.T) {
	for _, prior := range []string{"", "same", "different"} {
		for _, phase := range []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt} {
			in := precommitTestInput(t, prior, phase, true)
			want := error(errPrecommitControlRecords)
			if phase == PhasePendingReceipt {
				want = nil
			}
			precommitTestAssert(t, in, want, precommitTestPendingDigest(t, in))
		}
	}
	for _, phase := range []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt} {
		in := precommitTestInput(t, "same", phase, false)
		j := precommitTestJournal(t, in)
		facts := precommitTestFacts(t, in, 1)
		facts.Previous = j.Previous
		in.Current.Receipt = precommitTestPutReceipt(t, &in, 1, facts)
		j.PreviousReceipt = in.Current.Receipt
		committedTestPutJournal(t, &in, j)
		var selected receipt.Digest
		if phase == PhasePendingReceipt {
			selected = in.Current.Receipt
		}
		precommitTestAssert(t, in, nil, selected)
		duplicate := in
		duplicate.Receipts[0] = bytes.Clone(in.Receipts[1])
		precommitTestAssert(t, duplicate, errPrecommitControlRecords, "")
		if phase == PhasePendingReceipt {
			// A valid historical RA with different observations is not inferred as pending.
			facts.Results[0].Inode++
			in.Current.Receipt = precommitTestPutReceipt(t, &in, 1, facts)
			j.PreviousReceipt = in.Current.Receipt
			committedTestPutJournal(t, &in, j)
			precommitTestAssert(t, in, nil, "")
		}
	}
	for _, mutate := range []func(*committedControlInput, *journalRecord){
		func(in *committedControlInput, j *journalRecord) { in.Candidates[1] = bytes.Clone(in.Candidates[0]) },
		func(in *committedControlInput, j *journalRecord) { j.Previous = receipt.Digest(codecTestToken()) },
		func(in *committedControlInput, j *journalRecord) {
			j.PreviousReceipt = receipt.Digest(codecTestToken())
		},
		func(in *committedControlInput, j *journalRecord) { j.Candidate = receipt.Digest(codecTestToken()) },
		func(in *committedControlInput, j *journalRecord) {
			in.Current.Candidate = receipt.Digest(codecTestToken())
		},
		func(in *committedControlInput, j *journalRecord) {
			in.Current.Receipt = receipt.Digest(codecTestToken())
		},
	} {
		in := precommitTestInput(t, "same", PhasePrepared, false)
		j := precommitTestJournal(t, in)
		mutate(&in, &j)
		committedTestPutJournal(t, &in, j)
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	in := precommitTestInput(t, "different", PhasePrepared, false)
	in.Candidates[0], in.Candidates[1] = in.Candidates[1], in.Candidates[0]
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
}

func TestPrecommitControlCompletionDoesNotNarrowScalarCodecs(t *testing.T) {
	for slot := 0; slot < 2; slot++ {
		for bits := 0; bits < 32; bits++ {
			for subset := 0; subset < 4; subset++ {
				in := precommitTestInput(t, "different", PhasePendingReceipt, true)
				facts := precommitTestFacts(t, in, slot)
				facts.Dimensions = receipt.Dimensions{InputsPinned: bits&1 != 0, ArtifactIntegrity: bits&2 != 0,
					ArtifactAvailable: bits&4 != 0, HostCompatible: bits&8 != 0, ConvergenceVerified: bits&16 != 0}
				facts.Phases = nil
				if subset&1 != 0 {
					facts.Phases = append(facts.Phases, string(PhaseHomeApply))
				}
				if subset&2 != 0 {
					facts.Phases = append(facts.Phases, string(PhaseToolInstall))
				}
				d := precommitTestPutReceipt(t, &in, slot, facts)
				if _, err := decodePendingReceipt(in.Receipts[slot]); err != nil {
					t.Fatalf("scalar codec narrowed at bits %d subset %d: %v", bits, subset, err)
				}
				if slot == 1 {
					in.Current.Receipt = d
					j := precommitTestJournal(t, in)
					j.PreviousReceipt = d
					committedTestPutJournal(t, &in, j)
				}
				var want error
				if bits&27 != 27 || subset != 3 {
					want = errPrecommitControlRecords
				}
				precommitTestAssert(t, in, want, precommitTestPendingDigest(t, in))
			}
		}
	}
}

func TestPrecommitControlStaticFactsAndIndependentHistory(t *testing.T) {
	changes := []struct {
		name   string
		change func(*receipt.Completed)
	}{
		{"source", func(v *receipt.Completed) { v.Source.URI = "https://example.com/other.git" }},
		{"candidate", func(v *receipt.Completed) { v.Candidate = receipt.Digest(codecTestToken()) }},
		{"component path", func(v *receipt.Completed) { v.Components[1].Path = "profiles/elsewhere" }},
		{"component file", func(v *receipt.Completed) {
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
		{"tool count", func(v *receipt.Completed) { v.Tools = nil }},
		{"tool integrity", func(v *receipt.Completed) { v.Tools[0].Integrity = "sha256:" + strings.Repeat("a", 64) }},
	}
	for slot := 0; slot < 2; slot++ {
		for _, tt := range changes {
			t.Run(fmt.Sprintf("slot%d/%s", slot, tt.name), func(t *testing.T) {
				in := precommitTestInput(t, "different", PhasePendingReceipt, true)
				facts := precommitTestFacts(t, in, slot)
				tt.change(&facts)
				d := precommitTestPutReceipt(t, &in, slot, facts)
				// Rehashing corruption removes a digest-integrity explanation for refusal.
				if _, err := decodePendingReceipt(in.Receipts[slot]); err != nil {
					t.Fatal("mutation is not scalar-valid", err)
				}
				if slot == 1 {
					in.Current.Receipt = d
					j := precommitTestJournal(t, in)
					j.PreviousReceipt = d
					committedTestPutJournal(t, &in, j)
				}
				precommitTestAssert(t, in, errPrecommitControlRecords, "")
			})
		}
	}
	for _, change := range []func(*receipt.Completed){
		func(v *receipt.Completed) { v.Previous = receipt.Digest(codecTestToken()) },
		func(v *receipt.Completed) { v.Preconditions[0] = v.Results[0] },
		func(v *receipt.Completed) { v.Results[0].Inode++ },
	} {
		in := precommitTestInput(t, "different", PhasePendingReceipt, true)
		facts := precommitTestFacts(t, in, 0)
		change(&facts)
		precommitTestPutReceipt(t, &in, 0, facts)
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	// RA's predecessor and old observations are historical, not the new journal's.
	in := precommitTestInput(t, "different", PhasePrepared, false)
	facts := precommitTestFacts(t, in, 1)
	facts.Previous = "sha256:" + receipt.Digest(strings.Repeat("c", 64))
	facts.Results[0].Inode += 42
	in.Current.Receipt = precommitTestPutReceipt(t, &in, 1, facts)
	j := precommitTestJournal(t, in)
	j.PreviousReceipt = in.Current.Receipt
	committedTestPutJournal(t, &in, j)
	precommitTestAssert(t, in, nil, "")
}

func TestPrecommitControlSelectedRolesAndExactZero(t *testing.T) {
	for slot := 0; slot < 5; slot++ {
		for _, malformed := range [][]byte{{}, []byte("null"), []byte("{}"), []byte(" ")} {
			in := precommitTestInput(t, "different", PhasePendingReceipt, true)
			*precommitTestWires(&in)[slot] = append([]byte{}, malformed...)
			precommitTestAssert(t, in, errPrecommitControlRecords, "")
		}
		in := precommitTestInput(t, "different", PhasePendingReceipt, true)
		*precommitTestWires(&in)[slot] = nil
		var want error
		if slot != 2 {
			want = errPrecommitControlRecords
		}
		precommitTestAssert(t, in, want, "")
		for _, suffix := range []string{"\n", "{}", "null"} {
			in := precommitTestInput(t, "different", PhasePendingReceipt, true)
			p := precommitTestWires(&in)[slot]
			*p = append(bytes.Clone(*p), suffix...)
			precommitTestAssert(t, in, errPrecommitControlRecords, "")
		}
	}
	for _, partial := range []currentRecord{
		{Schema: 1}, {Domain: "zerb.current"}, {Status: "stable"}, {Instance: strings.Repeat("1", 32)},
		{OperationID: strings.Repeat("2", 32)}, {OperationToken: strings.Repeat("3", 32)},
		{WriterGeneration: 1}, {WriterToken: strings.Repeat("4", 32)},
		{Candidate: receipt.Digest(codecTestToken())}, {Receipt: receipt.Digest(codecTestToken())},
	} {
		in := precommitTestInput(t, "", PhasePrepared, false)
		in.Current = partial
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	for _, slot := range []int{1, 3} {
		in := precommitTestInput(t, "", PhasePrepared, false)
		other := precommitTestInput(t, "different", PhasePrepared, false)
		*precommitTestWires(&in)[slot] = *precommitTestWires(&other)[slot]
		precommitTestAssert(t, in, errPrecommitControlRecords, "")
	}
	in := precommitTestInput(t, "", PhasePrepared, false)
	j := precommitTestJournal(t, in)
	j.Previous, j.PreviousReceipt = receipt.Digest(codecTestToken()), receipt.Digest(codecTestToken())
	committedTestPutJournal(t, &in, j)
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
}

func TestPrecommitControlEmptyTargetsAndNoOpCheckpoints(t *testing.T) {
	for _, phase := range []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt} {
		in := precommitTestInput(t, "", phase, phase == PhasePendingReceipt)
		c := committedTestCandidate(t)
		c.Targets, c.Tools = nil, nil
		c.HomeSources = []string{"another/source"}
		c.Executables[0].Path = "/another/bash"
		digest, wire, err := receipt.CandidateDigest(c)
		if err != nil {
			t.Fatal(err)
		}
		in.Candidates[0] = wire
		j := precommitTestJournal(t, in)
		j.Candidate, j.Preconditions, j.Results = digest, nil, nil
		committedTestPutJournal(t, &in, j)
		var pending receipt.Digest
		if phase == PhasePendingReceipt {
			facts := committedTestFacts(t, c, digest, "")
			facts.Tools = nil
			pending = precommitTestPutReceipt(t, &in, 0, facts)
		}
		precommitTestAssert(t, in, nil, pending)
		if phase == PhaseHomeApply || phase == PhasePendingReceipt {
			j.CompletedPhases = nil
			committedTestPutJournal(t, &in, j)
			precommitTestAssert(t, in, errPrecommitControlRecords, "")
		}
	}
}

func TestPrecommitControlResourcePreflight(t *testing.T) {
	in := precommitTestInput(t, "different", PhasePendingReceipt, true)
	backing := make([]byte, int(committedControlWireBudget)+1)
	for i, p := range precommitTestWires(&in) {
		start := i * int(codecReservationLimit)
		copy(backing[start:], *p)
		*p = backing[start : start+len(*p) : start+int(codecReservationLimit)]
	}
	counters, err := reservePrecommitControl(in)
	if err != nil || counters.Borrowed != 40_000_000 || counters.Reservation > 80_008_493 {
		t.Fatalf("exact selected capacity boundary: %+v %v", counters, err)
	}
	precommitTestAssert(t, in, nil, precommitTestPendingDigest(t, in))
	// The limit is inclusive and each byte of visible capacity is charged.
	in.Journal = in.Journal[: len(in.Journal) : cap(in.Journal)-1]
	if counters, err := reservePrecommitControl(in); err != nil || counters.Borrowed != 39_999_999 {
		t.Fatal("one below selected capacity limit refused or miscounted")
	}
	in.Journal = backing[4*int(codecReservationLimit) : 4*int(codecReservationLimit)+len(in.Journal) : len(backing)]
	precommitTestAssert(t, in, errPrecommitControlResource, "")
	precommitTestNoAllocation(t, in, errPrecommitControlResource)
	// Each actual codec wire retains its own unchanged 8 MB limit.
	in = precommitTestInput(t, "", PhasePrepared, false)
	in.Journal = backing[: int(codecReservationLimit)+1 : int(codecReservationLimit)+1]
	precommitTestAssert(t, in, errPrecommitControlResource, "")
	precommitTestNoAllocation(t, in, errPrecommitControlResource)
	// Overlapping visible capacities count independently; backing identity is irrelevant.
	in = precommitTestInput(t, "", PhasePrepared, false)
	shared := backing[: 1 : int(codecReservationLimit)+1]
	in.Candidates, in.Receipts, in.Journal = [2][]byte{shared, shared}, [2][]byte{shared, shared}, shared
	precommitTestAssert(t, in, errPrecommitControlResource, "")
	precommitTestNoAllocation(t, in, errPrecommitControlResource)
	in = precommitTestInput(t, "", PhasePrepared, false)
	in.Receipts[1] = backing[:0]
	precommitTestAssert(t, in, errPrecommitControlResource, "")
	precommitTestNoAllocation(t, in, errPrecommitControlResource)
	// A late grammar error must be found before materializing earlier valid records.
	in = precommitTestInput(t, "different", PhasePendingReceipt, true)
	in.Journal = []byte("{}")
	precommitTestAssert(t, in, errPrecommitControlRecords, "")
	precommitTestNoAllocation(t, in, errPrecommitControlRecords)
	// Grammar-valid 7.2 MB wire exceeds the codec's full decoder reservation.
	in = precommitTestInput(t, "different", PhasePendingReceipt, true)
	facts := precommitTestFacts(t, in, 1)
	facts.Zerb.Version = strings.Repeat("<", 1_200_000)
	wire, err := json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: receipt.Digest(codecTestToken()), Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	in.Receipts[1] = wire[:len(wire):len(wire)]
	precommitTestAssert(t, in, errPrecommitControlResource, "")
	precommitTestNoAllocation(t, in, errPrecommitControlResource)
}

func precommitTestNoAllocation(t *testing.T, in committedControlInput, want error) {
	t.Helper()
	if counters, err := reservePrecommitControl(in); err != want || counters != (committedControlCounters{}) {
		t.Fatal("refusal returned partial reservation or wrong error")
	}
	if allocations := testing.AllocsPerRun(3, func() { _, _ = readPrecommitControl(in) }); allocations != 0 {
		t.Fatalf("upfront refusal allocated %g", allocations)
	}
}

func TestPrecommitControlConcurrentCallerPreservation(t *testing.T) {
	in := precommitTestInput(t, "different", PhasePendingReceipt, true)
	pending := precommitTestPendingDigest(t, in)
	j := precommitTestJournal(t, in)
	want := committedControlLinks{Candidate: j.Candidate, Receipt: pending, OperationID: j.OperationID,
		OperationToken: j.OperationToken, WriterGeneration: j.WriterGeneration, WriterToken: j.WriterToken, Phase: j.Phase}
	var before [5][]byte
	for i, p := range precommitTestWires(&in) {
		before[i] = bytes.Clone(*p)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				if got, err := readPrecommitControl(in); err != nil || got != want {
					t.Error("concurrent read changed result", err)
				}
			}
		}()
	}
	wg.Wait()
	for i, p := range precommitTestWires(&in) {
		if !bytes.Equal(*p, before[i]) {
			t.Fatal("concurrent readers changed caller bytes")
		}
	}
}

func FuzzPrecommitControlIndependentReceiptFacts(f *testing.F) {
	for i := uint8(0); i < 8; i++ {
		f.Add(i, uint64(i))
	}
	f.Fuzz(func(t *testing.T, selector uint8, value uint64) {
		in := precommitTestInput(t, "different", PhasePendingReceipt, true)
		facts := precommitTestFacts(t, in, 0)
		var want error
		switch selector % 8 {
		case 0:
			facts.Dimensions.ArtifactAvailable = value&1 != 0
		case 1:
			facts.Platform.Kernel = strings.Repeat("k", int(value%32)+1)
		case 2:
			facts.Zerb.Version = strings.Repeat("z", int(value%32)+1)
		case 3:
			facts.Results[0].Inode += value%100 + 1
			want = errPrecommitControlRecords
		case 4:
			facts.Preconditions[0] = facts.Results[0]
			want = errPrecommitControlRecords
		case 5:
			facts.Previous = receipt.Digest(codecTestToken())
			want = errPrecommitControlRecords
		case 6:
			facts.Source.URI = "https://example.com/" + strings.Repeat("a", int(value%32)+1) + ".git"
			want = errPrecommitControlRecords
		case 7:
			facts.Dimensions.ConvergenceVerified = value&1 != 0
			if value&1 == 0 {
				want = errPrecommitControlRecords
			}
		}
		pending := precommitTestPutReceipt(t, &in, 0, facts)
		precommitTestAssert(t, in, want, pending)
	})
}

func FuzzPrecommitControlOwnerDistance(f *testing.F) {
	f.Add(uint64(1), uint64(2), false, false)
	f.Add(uint64(math.MaxUint64-3), uint64(3), true, true)
	f.Fuzz(func(t *testing.T, prior, distance uint64, transfer, ahead bool) {
		prior = prior%(math.MaxUint64-8) + 1
		distance %= 7
		in := precommitTestInput(t, "same", PhasePrepared, false)
		in.Current.WriterGeneration = prior
		previous := ""
		if transfer {
			previous = strings.Repeat("8", 32)
		}
		in.Writer = writerRecordFixture(prior+distance, previous, false)
		in.Writer.Token = strings.Repeat("5", 32)
		j := precommitTestJournal(t, in)
		j.WriterGeneration, j.WriterToken, j.WriterPreviousToken = in.Writer.Generation, in.Writer.Token, previous
		if ahead {
			j.WriterGeneration++
			j.WriterToken, j.WriterPreviousToken = strings.Repeat("9", 32), in.Writer.Token
		}
		committedTestPutJournal(t, &in, j)
		var want error
		if distance < 2 || transfer && distance < 3 {
			want = errPrecommitControlRecords
		}
		precommitTestAssert(t, in, want, "")
	})
}
