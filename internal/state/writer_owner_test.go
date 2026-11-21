package state

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func writerRecordFixture(generation uint64, previous string, released bool) writerOwner {
	record := writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "held", Instance: strings.Repeat("1", 32),
		Generation: generation, Token: strings.Repeat("4", 32), Process: processIdentity{123, 456, 789}, PreviousToken: previous}
	if generation > 0 {
		record.PreviousGeneration = generation - 1
	}
	if released {
		record.Status, record.Token, record.Process = "released", "", processIdentity{}
	}
	return record
}

func writerRecordFixtures() []writerOwner {
	prior := strings.Repeat("5", 32)
	return []writerOwner{
		writerRecordFixture(0, "", true), writerRecordFixture(1, "", false),
		writerRecordFixture(2, prior, false), writerRecordFixture(2, prior, true),
		writerRecordFixture(3, "", false), writerRecordFixture(3, prior, false),
		writerRecordFixture(3, prior, true), writerRecordFixture(4, "", false),
		writerRecordFixture(4, prior, true), writerRecordFixture(math.MaxUint64, "", false),
		writerRecordFixture(math.MaxUint64, prior, false), writerRecordFixture(math.MaxUint64, prior, true),
	}
}

func TestWriterOwnerRecordVectorsAndFreshSubset(t *testing.T) {
	for i, record := range writerRecordFixtures() {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			generation := strconv.FormatUint(record.Generation, 10)
			previous := strconv.FormatUint(record.PreviousGeneration, 10)
			want := `{"schema":1,"domain":"zerb.writer-owner","status":"` + record.Status + `","instance":"11111111111111111111111111111111","generation":` + generation
			if record.Status == "held" {
				want += `,"token":"44444444444444444444444444444444","process":{"pid":123,"start":456,"pid_namespace":789}`
			} else {
				want += `,"process":{"pid":0,"start":0,"pid_namespace":0}`
			}
			want += `,"previous_generation":` + previous
			if record.PreviousToken != "" {
				want += `,"previous_token":"55555555555555555555555555555555"`
			}
			want += "}"
			data, err := encodeWriterOwnerRecord(record)
			oracle, oracleErr := json.Marshal(record)
			if err != nil || oracleErr != nil || string(data) != want || !bytes.Equal(data, oracle) {
				t.Fatalf("writer golden mismatch: %v, %v", err, oracleErr)
			}
			if got, err := decodeWriterOwnerRecord(data); err != nil || got != record {
				t.Fatalf("writer identity changed: %v", err)
			}
			fresh := record.Generation == 0 || record.Generation == 1 || record.Generation == 2 && record.Status == "released"
			if validWriterOwner(record) != fresh {
				t.Fatal("FreshInit accepted set changed")
			}
			encoded, encodeErr := encodeWriterOwner(record)
			decoded, decodeErr := decodeWriterOwner(data)
			if fresh {
				if encodeErr != nil || decodeErr != nil || !bytes.Equal(encoded, data) || decoded != record {
					t.Fatal("FreshInit wire/error behavior changed")
				}
			} else if encodeErr != errInitRecord || encoded != nil || decodeErr != errInitRecord || decoded != (writerOwner{}) {
				t.Fatal("general record escaped through FreshInit")
			}
		})
	}
}

func TestWriterOwnerRecordInvalidValues(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*writerOwner)
	}{
		{"schema", func(r *writerOwner) { r.Schema = 2 }},
		{"domain", func(r *writerOwner) { r.Domain = "zerb.root" }},
		{"status", func(r *writerOwner) { r.Status = "pending" }},
		{"instance empty", func(r *writerOwner) { r.Instance = "" }},
		{"instance uppercase", func(r *writerOwner) { r.Instance = strings.Repeat("A", 32) }},
		{"token short", func(r *writerOwner) { r.Token = strings.Repeat("4", 31) }},
		{"token long", func(r *writerOwner) { r.Token += "4" }},
		{"token nonhex", func(r *writerOwner) { r.Token = strings.Repeat("g", 32) }},
		{"token instance", func(r *writerOwner) { r.Token = r.Instance }},
		{"previous token short", func(r *writerOwner) { r.PreviousToken = "5" }},
		{"previous token uppercase", func(r *writerOwner) { r.PreviousToken = strings.Repeat("A", 32) }},
		{"previous token instance", func(r *writerOwner) { r.PreviousToken = r.Instance }},
		{"previous token reused", func(r *writerOwner) { r.PreviousToken = r.Token }},
		{"zero PID", func(r *writerOwner) { r.Process.PID = 0 }},
		{"negative PID", func(r *writerOwner) { r.Process.PID = -1 }},
		{"zero start", func(r *writerOwner) { r.Process.Start = 0 }},
		{"zero namespace", func(r *writerOwner) { r.Process.PIDNamespace = 0 }},
		{"held zero", func(r *writerOwner) { r.Generation, r.PreviousGeneration = 0, 0 }},
		{"held first predecessor", func(r *writerOwner) { r.Generation, r.PreviousGeneration = 1, 0 }},
		{"held second empty predecessor", func(r *writerOwner) { r.Generation, r.PreviousGeneration, r.PreviousToken = 2, 1, "" }},
		{"predecessor future", func(r *writerOwner) { r.PreviousGeneration = r.Generation }},
		{"predecessor gap", func(r *writerOwner) { r.PreviousGeneration = 0 }},
		{"predecessor wrap", func(r *writerOwner) { r.Generation, r.PreviousGeneration = 0, math.MaxUint64 }},
		{"predecessor max", func(r *writerOwner) { r.PreviousGeneration = math.MaxUint64 }},
		{"released token", func(r *writerOwner) { r.Status, r.Process = "released", processIdentity{} }},
		{"released process", func(r *writerOwner) { r.Status, r.Token = "released", "" }},
		{"released first", func(r *writerOwner) { *r = writerRecordFixture(1, r.PreviousToken, true) }},
		{"released empty predecessor", func(r *writerOwner) { *r = writerRecordFixture(3, "", true) }},
		{"initial previous token", func(r *writerOwner) { *r = writerRecordFixture(0, r.PreviousToken, true) }},
		{"initial previous generation", func(r *writerOwner) { *r = writerRecordFixture(0, "", true); r.PreviousGeneration = math.MaxUint64 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := writerRecordFixture(3, strings.Repeat("5", 32), false)
			tt.change(&r)
			before := r
			if validWriterOwnerRecord(r) {
				t.Fatal("invalid owner accepted")
			}
			if data, err := encodeWriterOwnerRecord(r); err != errWriterOwnerRecord || data != nil || r != before {
				t.Fatal("invalid encoding changed input or exposed bytes/error detail")
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodeWriterOwnerRecord(data); err != errWriterOwnerRecord || got != (writerOwner{}) {
				t.Fatal("invalid decoding exposed partial owner")
			}
		})
	}
}

func TestWriterOwnerRecordStrictBytes(t *testing.T) {
	r := writerRecordFixture(3, strings.Repeat("5", 32), false)
	data, err := encodeWriterOwnerRecord(r)
	if err != nil {
		t.Fatal(err)
	}
	base := string(data)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	cases := []string{"", "null", "[]", "{}", base + "{}", " " + base, base + "\n", strings.Repeat(" ", writerOwnerRecordLimit), strings.Repeat(" ", writerOwnerRecordLimit+1), `{"schema":1,` + strings.Repeat("[", 64)}
	for key, value := range fields {
		part := `"` + key + `":` + string(value)
		cases = append(cases, "{"+part+","+base[1:], "{"+`"`+key+`":null,`+base[1:])
		for _, replacement := range []string{`"` + key + `":null`, `"` + strings.ToUpper(key) + `":` + string(value)} {
			cases = append(cases, strings.Replace(base, part, replacement, 1))
		}
		missing := make(map[string]json.RawMessage, len(fields)-1)
		for name, raw := range fields {
			if name != key {
				missing[name] = raw
			}
		}
		omitted, err := json.Marshal(missing)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, string(omitted))
	}
	for _, pair := range [][2]string{
		{`"schema":1`, `"schema":1,"x":1`}, {`"schema":1`, `"schema":1.0`},
		{`"schema"`, `"sche\u006da"`}, {`"generation":3`, `"generation":18446744073709551616`},
		{`"generation":3`, `"generation":-1`}, {`"generation":3`, `"generation":3e0`},
		{`"previous_generation":2`, `"previous_generation":18446744073709551616`},
		{`"pid":123`, `"pid":9223372036854775808`}, {`"pid":123`, `"pid":123,"PID":123`},
		{`"start":456`, `"start":18446744073709551616`}, {`"pid_namespace":789`, `"pid_namespace":null`},
		{`"process":{"pid":123,"start":456,"pid_namespace":789}`, `"process":{"start":456,"pid":123,"pid_namespace":789}`},
	} {
		cases = append(cases, strings.Replace(base, pair[0], pair[1], 1))
	}
	reordered, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases, string(reordered))
	for i, input := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			data := []byte(input)
			before := bytes.Clone(data)
			if got, err := decodeWriterOwnerRecord(data); err != errWriterOwnerRecord || got != (writerOwner{}) || !bytes.Equal(data, before) {
				t.Fatal("noncanonical bytes accepted or partial/input/error detail changed")
			}
		})
	}
	if got, err := decodeWriterOwnerRecord(nil); err != errWriterOwnerRecord || got != (writerOwner{}) {
		t.Fatal("nil accepted")
	}
}

func TestWriterOwnerRecordOwnershipAndConcurrentUse(t *testing.T) {
	r := writerRecordFixture(3, strings.Repeat("5", 32), false)
	data, err := encodeWriterOwnerRecord(r)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(data)
	decoded, err := decodeWriterOwnerRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	if decoded != r {
		t.Fatal("decoder retained mutable input")
	}
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 32; j++ {
				out, err := encodeWriterOwnerRecord(decoded)
				if err != nil || !bytes.Equal(out, want) {
					t.Error("shared owner changed")
					return
				}
				clear(out)
				if decoded != r {
					t.Error("encoding exposed mutable owner")
				}
			}
		}()
	}
	group.Wait()
}

func FuzzWriterOwnerRecordCanonical(f *testing.F) {
	for _, record := range writerRecordFixtures() {
		data, err := json.Marshal(record)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		before := bytes.Clone(data)
		r, err := decodeWriterOwnerRecord(data)
		if !bytes.Equal(data, before) {
			t.Fatal("mutated input")
		}
		if err != nil {
			if err != errWriterOwnerRecord || r != (writerOwner{}) {
				t.Fatal("partial owner or error detail escaped")
			}
			return
		}
		encoded, err := encodeWriterOwnerRecord(r)
		if err != nil || !bytes.Equal(data, encoded) || len(encoded) > writerOwnerRecordLimit {
			t.Fatal("noncanonical acceptance")
		}
		if decoded, err := decodeWriterOwnerRecord(encoded); err != nil || decoded != r {
			t.Fatal("round trip lost owner identity")
		}
	})
}
