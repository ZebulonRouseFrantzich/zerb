package state

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

func TestCodecCandidateDifferentialAndOwnership(t *testing.T) {
	c := codecTestCandidate(t)
	for _, mutate := range []func(*receipt.Candidate){
		func(*receipt.Candidate) {},
		func(v *receipt.Candidate) {
			v.Components[0], v.Components[1] = v.Components[1], v.Components[0]
			v.Policy.PolicyIDs[0], v.Policy.PolicyIDs[1] = v.Policy.PolicyIDs[1], v.Policy.PolicyIDs[0]
		},
		func(v *receipt.Candidate) {
			v.Targets = []string{"z", "a"}
			v.HomeSources = []string{"source/z", "source/a"}
			v.Tools = []receipt.Tool{{ID: "node", Version: "24.4.1", Locks: []receipt.LockValue{{Name: "artifact", Value: codecTestToken()}}}}
		},
		func(v *receipt.Candidate) { v.Executables[0].Path = "/" + strings.Repeat("<&雪\u2028", 100) },
	} {
		v := c
		v.Components = codecTightComponents(c.Components)
		v.Policy = codecTightPolicy(c.Policy)
		v.Executables = append([]receipt.Executable{}, c.Executables...)
		mutate(&v)
		digest, wire, err := receipt.CandidateDigest(v)
		if err != nil {
			t.Fatal(err)
		}
		before := bytes.Clone(wire)
		got, d, err := decodeCandidateRecord(wire)
		want, wantDigest, ok := codecReferenceCandidate(wire)
		if err != nil || !ok || d != digest || d != wantDigest || !reflect.DeepEqual(got, want) {
			t.Fatal("candidate differential mismatch", err)
		}
		if !bytes.Equal(wire, before) {
			t.Fatal("borrowed candidate input changed")
		}
		for i := range wire {
			wire[i] = '!'
		}
		d2, w2, err := receipt.CandidateDigest(got)
		if err != nil || d2 != digest || !bytes.Equal(w2, before) {
			t.Fatal("result retained input alias", err)
		}
		codecAssertExactCapacities(t, reflect.ValueOf(got))
	}
}

func TestCodecPendingAllDimensionsAndPhaseSubsets(t *testing.T) {
	for bits := 0; bits < 64; bits++ {
		for _, phases := range [][]string{{}, {"mutating-home-apply"}, {"mutating-tool-install"}, {"mutating-tool-install", "mutating-home-apply"}} {
			v := codecTestCompleted(t)
			v.Phases = phases
			if bits&32 != 0 {
				v.Previous = receipt.Digest(codecTestToken())
			}
			v.Dimensions = receipt.Dimensions{InputsPinned: bits&1 != 0, ArtifactIntegrity: bits&2 != 0, ArtifactAvailable: bits&4 != 0, HostCompatible: bits&8 != 0, ConvergenceVerified: bits&16 != 0}
			wantDigest, want, err := codecReferencePending(v)
			if err != nil {
				t.Fatal(err)
			}
			digest, wire, err := encodePendingReceipt(v)
			if err != nil || digest != wantDigest || !bytes.Equal(wire, want) {
				t.Fatal("pending encoding differs from public oracle", err)
			}
			before := bytes.Clone(wire)
			got, err := decodePendingReceipt(wire)
			reference, ok := codecReferencePendingDecode(wire)
			if err != nil || !ok || !reflect.DeepEqual(got, reference) || got.Digest != digest {
				t.Fatal("pending differential mismatch", err)
			}
			if !bytes.Equal(wire, before) {
				t.Fatal("borrowed pending input changed")
			}
			if completedChecksPassed(got.Facts) != (bits&27 == 27 && len(phases) == 2) {
				t.Fatal("pure codec promoted incomplete facts")
			}
			codecAssertExactCapacities(t, reflect.ValueOf(got))
		}
	}
}

func TestCodecStrictMalformedDifferential(t *testing.T) {
	_, candidate, err := receipt.CandidateDigest(codecTestCandidate(t))
	if err != nil {
		t.Fatal(err)
	}
	_, pending, err := codecReferencePending(codecTestCompleted(t))
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func([]byte) []byte{
		func(b []byte) []byte { return append(b, '\n') },
		func(b []byte) []byte { return append(b, []byte(`{}`)...) },
		func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"schema":1`), []byte(`"schema":1,"schema":1`), 1)
		},
		func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"schema":1`), []byte(`"unknown":0,"schema":1`), 1)
		},
		func(b []byte) []byte { return bytes.Replace(b, []byte(`"schema":1`), []byte(`"Schema":1`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`"schema":1`), []byte(`"schema":1.0`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`"schema":1`), []byte(`"schema":01`), 1) },
		func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"schema":1`), []byte(`"schema":18446744073709551616`), 1)
		},
		func(b []byte) []byte { return bytes.Replace(b, []byte(`"targets":[]`), []byte(`"targets":null`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`\u003c`), []byte(`<`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`\u003c`), []byte(`\u003C`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`\u2028`), []byte("\u2028"), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`https://`), []byte(`https:\/\/`), 1) },
		func(b []byte) []byte { return bytes.Replace(b, []byte(`雪`), []byte{0xff}, 1) },
		func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"provider_free":true`), []byte(`"provider_free":null`), 1)
		},
	}
	for _, original := range [][]byte{candidate, pending} {
		for i, change := range mutations {
			data := change(bytes.Clone(original))
			if bytes.Equal(data, original) {
				t.Fatalf("mutation %d did not affect fixture", i)
			}
			before := bytes.Clone(data)
			if bytes.Equal(original, candidate) {
				got, d, err := decodeCandidateRecord(data)
				_, _, ok := codecReferenceCandidate(data)
				if err == nil || ok || d != "" || !reflect.DeepEqual(got, receipt.Candidate{}) {
					t.Fatalf("candidate mutation %d accepted or partial", i)
				}
			} else {
				got, err := decodePendingReceipt(data)
				_, ok := codecReferencePendingDecode(data)
				if err == nil || ok || !reflect.DeepEqual(got, pendingReceipt{}) {
					t.Fatalf("pending mutation %d accepted or partial", i)
				}
			}
			if !bytes.Equal(data, before) {
				t.Fatal("failure changed borrowed input")
			}
		}
	}
	// The envelope digest must be authenticated against facts, not just well-formed.
	data := bytes.Clone(pending)
	at := bytes.Index(data, []byte(`"digest":"sha256:`))
	if at < 0 {
		t.Fatal("digest fixture")
	}
	at += len(`"digest":"sha256:`)
	if data[at] == '0' {
		data[at] = '1'
	} else {
		data[at] = '0'
	}
	if got, err := decodePendingReceipt(data); err == nil || !reflect.DeepEqual(got, pendingReceipt{}) {
		t.Fatal("forged envelope digest accepted")
	}
}

func TestCodecLargeAcceptedWiresAndHistories(t *testing.T) {
	for _, length := range []int{1_000_000, 1_100_000, 1} {
		c := codecTestCandidate(t)
		c.Executables[0].Path = "/" + strings.Repeat("<", length)
		digest, wire, err := receipt.CandidateDigest(c)
		if err != nil {
			t.Fatal("old candidate witness rejected", err)
		}
		got, d, err := decodeCandidateRecord(wire)
		if err != nil || d != digest || got.Executables[0].Path != c.Executables[0].Path {
			t.Fatal("accepted large candidate failed", err)
		}
		facts := codecTestCompleted(t)
		facts.Zerb.Version = strings.Repeat("<", length)
		wantDigest, want, err := codecReferencePending(facts)
		if err != nil {
			t.Fatal("old receipt witness rejected", err)
		}
		digest, wire, err = encodePendingReceipt(facts)
		if err != nil || digest != wantDigest || !bytes.Equal(wire, want) {
			t.Fatal("accepted large pending encode failed", err)
		}
		decoded, err := decodePendingReceipt(wire)
		if err != nil || decoded.Facts.Zerb.Version != facts.Zerb.Version || decoded.Digest != digest {
			t.Fatal("accepted large pending decode failed", err)
		}
	}
}

func TestCodecConcurrentIndependentInvocations(t *testing.T) {
	_, wire, err := receipt.CandidateDigest(codecTestCandidate(t))
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(wire)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				v, _, err := decodeCandidateRecord(wire)
				if err != nil || v.ProfileID != "work" {
					t.Error("concurrent candidate invocation changed")
				}
			}
		}()
	}
	wg.Wait()
	if !bytes.Equal(wire, before) {
		t.Fatal("concurrent decode changed borrowed wire")
	}
}

func codecAssertExactCapacities(t testing.TB, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			codecAssertExactCapacities(t, v.Field(i))
		}
	case reflect.Slice:
		if v.Cap() != v.Len() {
			t.Fatal("decoded slice retains spare capacity")
		}
		for i := 0; i < v.Len(); i++ {
			codecAssertExactCapacities(t, v.Index(i))
		}
	}
}

func FuzzCandidateCanonicalDecoder(f *testing.F) {
	_, wire, err := receipt.CandidateDigest(codecTestCandidate(f))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(wire)
	f.Add([]byte(`null`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32<<10 {
			return
		}
		before := bytes.Clone(data)
		got, d, err := decodeCandidateRecord(data)
		want, wantDigest, ok := codecReferenceCandidate(data)
		if (err == nil) != ok {
			t.Fatal("candidate acceptance differs from independent oracle")
		}
		if ok && (d != wantDigest || !reflect.DeepEqual(got, want)) {
			t.Fatal("candidate value/digest differs")
		}
		if !ok && (d != "" || !reflect.DeepEqual(got, receipt.Candidate{})) {
			t.Fatal("candidate failure exposes partial result")
		}
		if !bytes.Equal(data, before) {
			t.Fatal("candidate input changed")
		}
	})
}

func FuzzPendingCanonicalDecoder(f *testing.F) {
	_, wire, err := codecReferencePending(codecTestCompleted(f))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(wire)
	f.Add([]byte(`null`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32<<10 {
			return
		}
		before := bytes.Clone(data)
		got, err := decodePendingReceipt(data)
		want, ok := codecReferencePendingDecode(data)
		if (err == nil) != ok {
			t.Fatal("pending acceptance differs from independent oracle")
		}
		if ok && !reflect.DeepEqual(got, want) {
			t.Fatal("pending value differs")
		}
		if !ok && !reflect.DeepEqual(got, pendingReceipt{}) {
			t.Fatal("pending failure exposes partial result")
		}
		if !bytes.Equal(data, before) {
			t.Fatal("pending input changed")
		}
	})
}

// Keep JSON itself an independent encoder oracle, rather than the new writer.
func TestCodecCanonicalFrameOverhead(t *testing.T) {
	v := codecTestCompleted(t)
	digest, facts, err := receipt.ReceiptDigest(v)
	if err != nil {
		t.Fatal(err)
	}
	var normalized receipt.Completed
	if err = json.Unmarshal(facts, &normalized); err != nil {
		t.Fatal(err)
	}
	frame, err := json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: digest, Facts: normalized})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame)-len(facts) != 128 {
		t.Fatal("pending frame overhead changed")
	}
}

// Derive the real canonicalizer boundary, rather than impose a new field cap.
func TestCodecDecoderMinimumReservationBoundary(t *testing.T) {
	for _, pending := range []bool{false, true} {
		c := codecTightCandidate(codecTestCandidate(t))
		v := codecTightCompleted(codecTestCompleted(t))
		var wire []byte
		var err error
		if pending {
			_, wire, err = codecReferencePending(v)
		} else {
			_, wire, err = receipt.CandidateDigest(c)
		}
		if err != nil {
			t.Fatal(err)
		}
		p := codecPlan{kind: codecCandidate, wire: uint64(len(wire))}
		r := codecReader{data: wire}
		control, factsWire := uint64(2048), uint64(len(wire))
		if pending {
			p.kind = codecPending
			var record pendingReceipt
			r.pending(&p, &record)
			control, factsWire = 4096, uint64(len(wire)-128)
		} else {
			var record receipt.Candidate
			r.candidate(&p, &record)
		}
		var budget codecBudget
		if !r.done() || !budget.add(1, control) || !codecSourceScratch(&budget, p.uriBytes) ||
			!budget.add(factsWire, 1) || !budget.add(p.strings, 1) || !budget.add(p.arrays, 2) || !budget.add(p.trees, 1) {
			t.Fatal("baseline does not fit")
		}
		// Each '<' adds six canonical bytes and one retained string byte.
		n := int((codecReservationLimit - budget.used) / 7)
		if pending {
			v.Zerb.Version += strings.Repeat("<", n)
			_, wire, err = codecReferencePending(v)
			if err != nil {
				t.Fatal("last pending value rejected by existing oracle", err)
			}
			if _, err := decodePendingReceipt(wire); err != nil {
				t.Fatal("last pending value rejected", err)
			}
			v.Zerb.Version += "<"
			if _, _, err := receipt.ReceiptDigest(v); err == nil {
				t.Fatal("next pending value admitted by existing oracle")
			}
			facts, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			wire, err = json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: codecTestHash(facts), Facts: v})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodePendingReceipt(wire); err == nil || !reflect.DeepEqual(got, pendingReceipt{}) {
				t.Fatal("over-budget pending not rejected completely")
			}
			if allocations := testing.AllocsPerRun(3, func() { _, _ = decodePendingReceipt(wire) }); allocations != 0 {
				t.Fatalf("pending rejection allocated %g", allocations)
			}
		} else {
			c.Executables[0].Path += strings.Repeat("<", n)
			_, wire, err = receipt.CandidateDigest(c)
			if err != nil {
				t.Fatal("last candidate value rejected by existing oracle", err)
			}
			if _, _, err := decodeCandidateRecord(wire); err != nil {
				t.Fatal("last candidate value rejected", err)
			}
			c.Executables[0].Path += "<"
			if _, _, err := receipt.CandidateDigest(c); err == nil {
				t.Fatal("next candidate value admitted by existing oracle")
			}
			wire, err = json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if got, digest, err := decodeCandidateRecord(wire); err == nil || digest != "" || !reflect.DeepEqual(got, receipt.Candidate{}) {
				t.Fatal("over-budget candidate not rejected completely")
			}
			if allocations := testing.AllocsPerRun(3, func() { _, _, _ = decodeCandidateRecord(wire) }); allocations != 0 {
				t.Fatalf("candidate rejection allocated %g", allocations)
			}
		}
	}
}

func TestCodecOrderedCorpusAndJournalRoundTrip(t *testing.T) {
	candidate := encoderCandidateFixture(t) // selected ID sorts before base.
	candidate.Source.URI = "https://example.com/repo&public.git"
	candidate.Source.Commit = strings.Repeat("b", 64)
	candidate.Source.RootTree = strings.Repeat("c", 64)
	_, wire, err := receipt.CandidateDigest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	got, digest, err := decodeCandidateRecord(wire)
	want, wantDigest, ok := codecReferenceCandidate(wire)
	if err != nil || !ok || digest != wantDigest || !reflect.DeepEqual(got, want) {
		t.Fatal("ordered corpus mismatch", err)
	}
	for _, phase := range []Phase{PhasePrepared, PhaseToolInstall, PhaseHomeApply, PhasePendingReceipt, PhaseCleanup} {
		for _, prefix := range [][]Phase{{}, {PhaseToolInstall}, {PhaseToolInstall, PhaseHomeApply}} {
			v := journalRecord{Schema: 1, Domain: "zerb.journal", Instance: strings.Repeat("a", 32), OperationID: strings.Repeat("b", 32), OperationToken: strings.Repeat("c", 32), WriterGeneration: 1, WriterToken: strings.Repeat("d", 32), Candidate: receipt.Digest(codecTestToken()), Phase: phase, Preconditions: []receipt.DestinationObservation{{Target: "a", Kind: "absent"}}, Results: []receipt.DestinationObservation{{Target: "a", Kind: "directory", Exists: true, Mode: 0700}}, CompletedPhases: prefix}
			encoded, err := encodeJournalRecord(v)
			if !validOperationPrefix(phase, prefix) {
				if err == nil {
					t.Fatal("invalid prefix encoded")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			standard, err := json.Marshal(v)
			if err != nil || !bytes.Equal(encoded, standard) {
				t.Fatal("journal differs from independent JSON encoding", err)
			}
			decoded, err := decodeJournalRecord(encoded)
			if err != nil || !reflect.DeepEqual(decoded, v) {
				t.Fatal("journal value mismatch", err)
			}
			codecAssertExactCapacities(t, reflect.ValueOf(decoded))
		}
	}
}

func FuzzCanonicalModelDifferential(f *testing.F) {
	for _, text := range []string{"plain", "<>&雪\u2028\u2029", "\"\\\n\r\t\b\f\u0001", "\xff", "", strings.Repeat("x", 1025)} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 8192 {
			return
		}
		candidate := codecTestCandidate(t)
		candidate.Executables[0].Path = "/" + text
		digest, wire, err := receipt.CandidateDigest(candidate)
		if err == nil {
			got, gotDigest, err := decodeCandidateRecord(wire)
			want, _, ok := codecReferenceCandidate(wire)
			if err != nil || !ok || gotDigest != digest || !reflect.DeepEqual(got, want) {
				t.Fatal("valid candidate model drift", err)
			}
		}
		facts := codecTestCompleted(t)
		facts.Zerb.Version = text
		facts.Platform.Kernel = text
		wantDigest, want, oracleErr := codecReferencePending(facts)
		gotDigest, got, err := encodePendingReceipt(facts)
		if (err == nil) != (oracleErr == nil) {
			t.Fatal("pending model acceptance drift")
		}
		if err == nil {
			if gotDigest != wantDigest || !bytes.Equal(got, want) {
				t.Fatal("pending model wire drift")
			}
			decoded, err := decodePendingReceipt(got)
			reference, ok := codecReferencePendingDecode(want)
			if err != nil || !ok || !reflect.DeepEqual(decoded, reference) {
				t.Fatal("valid pending model drift", err)
			}
		}
	})
}

func FuzzJournalCanonicalDecoder(f *testing.F) {
	v := encoderJournalFixture()
	wire, err := encodeJournalRecord(v)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(wire)
	f.Add([]byte(`null`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32<<10 {
			return
		}
		before := bytes.Clone(data)
		got, err := decodeJournalRecord(data)
		var want journalRecord
		oracle := json.NewDecoder(bytes.NewReader(data))
		oracle.DisallowUnknownFields()
		accepted := oracle.Decode(&want) == nil && oracle.Decode(new(any)) == io.EOF && codecValidateJournal(want)
		canonical, marshalErr := json.Marshal(want)
		accepted = accepted && marshalErr == nil && bytes.Equal(canonical, data)
		if (err == nil) != accepted {
			t.Fatal("journal canonical grammar drift")
		}
		if accepted && !reflect.DeepEqual(got, want) || !accepted && !reflect.DeepEqual(got, journalRecord{}) {
			t.Fatal("journal value/failure drift")
		}
		if !bytes.Equal(data, before) {
			t.Fatal("journal borrowed input changed")
		}
	})
}
