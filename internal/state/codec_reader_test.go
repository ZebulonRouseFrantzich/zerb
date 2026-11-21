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

func TestCanonicalReaderStringGrammar(t *testing.T) {
	values := []string{"", "plain/slash", "\"\\<>&", "雪😀\u2028\u2029\ufffd"}
	for c := byte(0); c < 0x20; c++ {
		values = append(values, string([]byte{c}))
	}
	for _, value := range values {
		wire, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		original := bytes.Clone(wire)
		var plan codecPlan
		r := codecReader{data: wire}
		if got := r.text(&plan, true); got != "" || !r.done() || plan.strings != uint64(len(value)) || plan.valueStrings != 1 {
			t.Fatalf("preflight string %q: reader=%+v plan=%+v", value, r, plan)
		}
		if !bytes.Equal(wire, original) {
			t.Fatal("preflight mutated borrowed string")
		}
		private := bytes.Clone(wire)
		r = codecReader{data: private, materialize: true}
		got := r.text(&plan, true)
		if !r.done() || got != value {
			t.Fatalf("materialized %q; want %q", got, value)
		}
		clear(private)
		if got != value || !bytes.Equal(wire, original) {
			t.Fatal("decoded string aliases input")
		}
	}
	for _, wire := range []string{`"<"`, `">"`, `"&"`, `"\u003C"`, `"\u0041"`, `"\u0008"`, `"\u0009"`, `"\u000a"`, `"\u000c"`, `"\u000d"`, `"\/"`, `"\uD83D\uDE00"`, `"\ud83d\ude00"`, `"\u202A"`, `"\u202"`, `"\q"`, "\"\u2028\"", "\"\u2029\"", "\"\xff\"", "\"\x00\"", `"truncated`, `"\`} {
		r := codecReader{data: []byte(wire)}
		var p codecPlan
		r.text(&p, true)
		if r.done() {
			t.Fatalf("noncanonical string accepted: %q", wire)
		}
	}
}

func TestCanonicalReaderUnsignedGrammar(t *testing.T) {
	for _, tc := range []struct {
		raw         string
		limit, want uint64
		ok          bool
	}{
		{"0", math.MaxUint64, 0, true}, {"4294967295", math.MaxUint32, math.MaxUint32, true},
		{"18446744073709551615", math.MaxUint64, math.MaxUint64, true},
		{"4294967296", math.MaxUint32, 0, false}, {"18446744073709551616", math.MaxUint64, 0, false},
		{"-0", math.MaxUint64, 0, false}, {"-1", math.MaxUint64, 0, false}, {"01", math.MaxUint64, 0, false},
		{"1e0", math.MaxUint64, 0, false}, {"1.0", math.MaxUint64, 0, false}, {"+1", math.MaxUint64, 0, false},
		{"", math.MaxUint64, 0, false}, {" 1", math.MaxUint64, 0, false},
	} {
		r := codecReader{data: []byte(tc.raw)}
		got := r.unsigned(tc.limit)
		if r.done() != tc.ok || tc.ok && got != tc.want {
			t.Fatalf("unsigned %q: value=%d ok=%t", tc.raw, got, r.done())
		}
	}
}

func TestCanonicalReaderPreflightAllocatesNothing(t *testing.T) {
	candidate := codecTestCandidate(t)
	_, candidateWire, err := receipt.CandidateDigest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	_, pendingWire, err := codecReferencePending(codecTestCompleted(t))
	if err != nil {
		t.Fatal(err)
	}
	journal := journalRecord{Schema: 1, Domain: "zerb.journal", Instance: strings.Repeat("a", 32), OperationID: strings.Repeat("b", 32), OperationToken: strings.Repeat("c", 32), WriterGeneration: 1, WriterToken: strings.Repeat("d", 32), Candidate: receipt.Digest(codecTestToken()), Phase: PhasePrepared, Preconditions: []receipt.DestinationObservation{}, Results: []receipt.DestinationObservation{}, CompletedPhases: []Phase{}}
	journalWire, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind codecKind
		data []byte
	}{
		{codecCandidate, candidateWire}, {codecPending, pendingWire}, {codecJournal, journalWire},
	} {
		if allocations := testing.AllocsPerRun(20, func() {
			p := codecPlan{kind: tc.kind, wire: uint64(len(tc.data))}
			r := codecReader{data: tc.data}
			switch tc.kind {
			case codecCandidate:
				r.candidate(&p, &receipt.Candidate{})
			case codecPending:
				r.pending(&p, &pendingReceipt{})
			case codecJournal:
				r.journal(&p, &journalRecord{})
			}
			if !r.done() || !p.fits() {
				panic("canonical fixture failed preflight")
			}
		}); allocations != 0 {
			t.Fatalf("kind %d allocated %g before admission", tc.kind, allocations)
		}
	}
	if reflect.TypeOf(codecReader{}).Size() != 40 {
		t.Fatal("reader exceeded approved fixed layout")
	}
}

func TestCanonicalReaderPlanCounts(t *testing.T) {
	_, wire, err := receipt.CandidateDigest(codecTestCandidate(t))
	if err != nil {
		t.Fatal(err)
	}
	p := codecPlan{kind: codecCandidate, wire: uint64(len(wire))}
	r := codecReader{data: wire}
	r.candidate(&p, &receipt.Candidate{})
	if !r.done() || !p.fits() {
		t.Fatal("preflight rejected canonical candidate")
	}
	reference, _, ok := codecReferenceCandidate(wire)
	if !ok {
		t.Fatal("reference rejected candidate")
	}
	payload, stringsCount := codecTestStringPayload(reflect.ValueOf(reference))
	if p.strings != payload || p.valueStrings != stringsCount || p.uriBytes != uint64(len(reference.Source.URI)) {
		t.Fatalf("string accounting: payload=%d values=%d plan=%+v", payload, stringsCount, p)
	}
	var expectedTrees uint64
	for i, c := range reference.Components {
		span := p.spans[i+1]
		var files []receipt.File
		if err := json.Unmarshal(wire[span.start:span.end], &files); err != nil || !reflect.DeepEqual(files, c.Files) {
			t.Fatalf("component %d files span differs", i)
		}
		expectedTrees += uint64(len(codecReadTreePrefix) + span.end - span.start + 1)
	}
	if p.trees != expectedTrees {
		t.Fatalf("tree charge %d; want %d", p.trees, expectedTrees)
	}
}

func codecTestStringPayload(v reflect.Value) (uint64, uint64) {
	var payload, count uint64
	switch v.Kind() {
	case reflect.String:
		return uint64(v.Len()), 1
	case reflect.Struct:
		for i := range v.NumField() {
			p, c := codecTestStringPayload(v.Field(i))
			payload += p
			count += c
		}
	case reflect.Slice:
		for i := range v.Len() {
			p, c := codecTestStringPayload(v.Index(i))
			payload += p
			count += c
		}
	}
	return payload, count
}

func FuzzCanonicalReaderString(f *testing.F) {
	for _, seed := range []string{`""`, `"ascii"`, `"\u003c\u2028雪"`, `"\ud800"`, `"\u0008"`, `null`, `"\xff"`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			return
		}
		var value string
		err := json.Unmarshal(data, &value)
		canonical, _ := json.Marshal(value)
		want := err == nil && bytes.Equal(data, canonical)
		var p codecPlan
		r := codecReader{data: data}
		r.text(&p, true)
		if r.done() != want {
			t.Fatalf("string acceptance differs for %q", data)
		}
		if !want {
			return
		}
		private := bytes.Clone(data)
		r = codecReader{data: private, materialize: true}
		if got := r.text(&p, true); !r.done() || got != value {
			t.Fatalf("string decode differs: %q", data)
		}
	})
}

func TestCanonicalDecoderBorrowedOwnership(t *testing.T) {
	_, wire, err := receipt.CandidateDigest(codecTestCandidate(t))
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(wire)
	want, wantDigest, ok := codecReferenceCandidate(wire)
	if !ok {
		t.Fatal("reference candidate refused")
	}
	got, digest, err := decodeCandidateRecord(wire)
	if err != nil || digest != wantDigest || !reflect.DeepEqual(got, want) || !bytes.Equal(wire, before) {
		t.Fatalf("candidate decode differs: %v", err)
	}
	clear(wire)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("candidate retained borrowed bytes")
	}
	_, wire, err = codecReferencePending(codecTestCompleted(t))
	if err != nil {
		t.Fatal(err)
	}
	before = bytes.Clone(wire)
	wantPending, ok := codecReferencePendingDecode(wire)
	if !ok {
		t.Fatal("reference pending refused")
	}
	gotPending, err := decodePendingReceipt(wire)
	if err != nil || !reflect.DeepEqual(gotPending, wantPending) || !bytes.Equal(wire, before) {
		t.Fatalf("pending decode differs: %v", err)
	}
	clear(wire)
	if !reflect.DeepEqual(gotPending, wantPending) {
		t.Fatal("pending retained borrowed bytes")
	}
	journal := journalRecord{Schema: 1, Domain: "zerb.journal", Instance: strings.Repeat("a", 32), OperationID: strings.Repeat("b", 32), OperationToken: strings.Repeat("c", 32), WriterGeneration: 1, WriterToken: strings.Repeat("d", 32), Candidate: receipt.Digest(codecTestToken()), Phase: PhasePrepared, Preconditions: []receipt.DestinationObservation{}, Results: []receipt.DestinationObservation{}, CompletedPhases: []Phase{}}
	wire, err = json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	before = bytes.Clone(wire)
	gotJournal, err := decodeJournalRecord(wire)
	if err != nil || !reflect.DeepEqual(gotJournal, journal) || !bytes.Equal(wire, before) {
		t.Fatalf("journal decode differs: %v", err)
	}
	clear(wire)
	if !reflect.DeepEqual(gotJournal, journal) {
		t.Fatal("journal retained borrowed bytes")
	}
}

func TestCanonicalDecoderRefusalBeforeAllocation(t *testing.T) {
	for _, wire := range [][]byte{nil, []byte(`{"schema":1}`), make([]byte, int(codecReservationLimit)+1)} {
		if allocations := testing.AllocsPerRun(3, func() {
			_, d, err := decodeCandidateRecord(wire)
			if err != errCanonicalStateRecord || d != "" {
				panic("candidate failure exposed value")
			}
			_, err = decodePendingReceipt(wire)
			if err != errCanonicalStateRecord {
				panic("pending failure exposed value")
			}
			_, err = decodeJournalRecord(wire)
			if err != errCanonicalStateRecord {
				panic("journal failure exposed value")
			}
		}); allocations != 0 {
			t.Fatalf("pre-admission refusal allocated %g", allocations)
		}
		c, _, _ := decodeCandidateRecord(wire)
		p, _ := decodePendingReceipt(wire)
		j, _ := decodeJournalRecord(wire)
		if !reflect.DeepEqual(c, receipt.Candidate{}) || !reflect.DeepEqual(p, pendingReceipt{}) || !reflect.DeepEqual(j, journalRecord{}) {
			t.Fatal("refusal returned partial model")
		}
	}
}
