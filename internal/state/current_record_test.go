package state

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

const currentGolden = `{"schema":1,"domain":"zerb.current","status":"stable","instance":"11111111111111111111111111111111","operation_id":"22222222222222222222222222222222","operation_token":"33333333333333333333333333333333","writer_generation":1,"writer_token":"44444444444444444444444444444444","candidate":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","receipt":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`

func currentFixture() currentRecord {
	return currentRecord{
		Schema: 1, Domain: "zerb.current", Status: "stable",
		Instance: strings.Repeat("1", 32), OperationID: strings.Repeat("2", 32),
		OperationToken: strings.Repeat("3", 32), WriterGeneration: 1,
		WriterToken: strings.Repeat("4", 32),
		Candidate:   receipt.Digest("sha256:" + strings.Repeat("a", 64)),
		Receipt:     receipt.Digest("sha256:" + strings.Repeat("b", 64)),
	}
}

// ARCH-024: these values check bytes and scalar grammar, never graph authority.
func TestCurrentRecordVectorsAndBound(t *testing.T) {
	record := currentFixture()
	// Count JSON syntax and fixed-width ASCII values independently of the codec.
	keys := []string{"schema", "domain", "status", "instance", "operation_id", "operation_token", "writer_generation", "writer_token", "candidate", "receipt"}
	keyBytes := 0
	for _, key := range keys {
		keyBytes += len(key)
	}
	bound := 2 + 9 + 10 + 2*10 + 2*8 + keyBytes + 1 + len("zerb.current") + len("stable") + 4*32 + 2*71 + len(strconv.FormatUint(math.MaxUint64, 10))
	if bound != 464 {
		t.Fatalf("independent maximum canonical bound: %d", bound)
	}
	if currentRecordLimit != bound {
		t.Fatalf("codec maximum: got %d, want %d", currentRecordLimit, bound)
	}
	generations := []uint64{1, 2, 9, 10, 99, 100, math.MaxUint64}
	for power := uint64(1000); power <= math.MaxUint64/10; power *= 10 {
		generations = append(generations, power-1, power)
	}
	for _, generation := range generations {
		t.Run(strconv.FormatUint(generation, 10), func(t *testing.T) {
			r := record
			r.WriterGeneration = generation
			want := strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":`+strconv.FormatUint(generation, 10), 1)
			data, err := encodeCurrentRecord(r)
			oracle, oracleErr := json.Marshal(r)
			if err != nil || oracleErr != nil || string(data) != want || !bytes.Equal(data, oracle) {
				t.Fatalf("golden bytes mismatch: %v, %v", err, oracleErr)
			}
			if len(data) != bound-20+len(strconv.FormatUint(generation, 10)) {
				t.Fatal("unexpected generation width")
			}
			if decoded, err := decodeCurrentRecord(data); err != nil || decoded != r {
				t.Fatalf("round trip lost completion identity: %v", err)
			}
			if generation == math.MaxUint64 && len(data) != currentRecordLimit {
				t.Fatal("maximum generation did not reach the byte boundary")
			}
		})
	}
	for _, change := range []func(*currentRecord){
		func(r *currentRecord) { r.Instance = r.OperationID },
		func(r *currentRecord) { r.Instance = r.OperationToken },
		func(r *currentRecord) { r.Instance = r.WriterToken },
		func(r *currentRecord) { r.Receipt = r.Candidate },
		func(r *currentRecord) { r.Instance = strings.Repeat("0", 32) },
	} {
		r := record
		change(&r)
		data, err := encodeCurrentRecord(r)
		if err != nil {
			t.Fatal("codec introduced an unspecified identity inequality")
		}
		if decoded, err := decodeCurrentRecord(data); err != nil || decoded != r {
			t.Fatal("valid scalar relation changed on round trip")
		}
	}
}

func TestCurrentRecordRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		change func(*currentRecord)
	}{
		{"schema", func(r *currentRecord) { r.Schema = 2 }},
		{"negative schema", func(r *currentRecord) { r.Schema = -1 }},
		{"domain", func(r *currentRecord) { r.Domain = "zerb.journal" }},
		{"status", func(r *currentRecord) { r.Status = "pending" }},
		{"zero generation", func(r *currentRecord) { r.WriterGeneration = 0 }},
		{"operation identity collision", func(r *currentRecord) { r.OperationID = r.OperationToken }},
		{"operation writer collision", func(r *currentRecord) { r.OperationID = r.WriterToken }},
		{"token writer collision", func(r *currentRecord) { r.OperationToken = r.WriterToken }},
		{"empty candidate", func(r *currentRecord) { r.Candidate = "" }},
		{"digest algorithm", func(r *currentRecord) { r.Candidate = receipt.Digest("SHA256:" + strings.Repeat("a", 64)) }},
		{"digest uppercase", func(r *currentRecord) { r.Receipt = receipt.Digest("sha256:" + strings.Repeat("A", 64)) }},
		{"digest short", func(r *currentRecord) { r.Candidate = "sha256:" + receipt.Digest(strings.Repeat("a", 63)) }},
		{"digest long", func(r *currentRecord) { r.Receipt += "0" }},
	}
	for _, field := range []struct {
		name string
		set  func(*currentRecord, string)
	}{
		{"instance", func(r *currentRecord, v string) { r.Instance = v }},
		{"operation ID", func(r *currentRecord, v string) { r.OperationID = v }},
		{"operation token", func(r *currentRecord, v string) { r.OperationToken = v }},
		{"writer token", func(r *currentRecord, v string) { r.WriterToken = v }},
	} {
		for _, value := range []string{"", strings.Repeat("0", 31), strings.Repeat("0", 33), strings.Repeat("A", 32), strings.Repeat("0", 31) + "g", strings.Repeat("0", 30) + "é", strings.Repeat("0", 31) + "\x00"} {
			setter, invalid := field.set, value
			tests = append(tests, struct {
				name   string
				change func(*currentRecord)
			}{field.name + "/" + strconv.Itoa(len(tests)), func(r *currentRecord) { setter(r, invalid) }})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := currentFixture()
			tt.change(&record)
			before := record
			if validCurrentRecord(record) {
				t.Fatal("accepted invalid value")
			}
			if data, err := encodeCurrentRecord(record); err != errCurrentRecord || data != nil || record != before {
				t.Fatal("invalid encode returned bytes, changed input or leaked an error")
			}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if decoded, err := decodeCurrentRecord(data); err != errCurrentRecord || decoded != (currentRecord{}) {
				t.Fatal("invalid decode exposed a partial record")
			}
		})
	}
}

func TestCurrentRecordRejectsNoncanonicalBytes(t *testing.T) {
	parts := strings.Split(currentGolden[1:len(currentGolden)-1], ",")
	cases := []string{"", "null", "{}", "[]", " " + currentGolden, currentGolden + "\n", currentGolden + "{}", currentGolden[:len(currentGolden)-1], strings.Repeat("x", currentRecordLimit+1)}
	for i, part := range parts {
		without := append([]string(nil), parts[:i]...)
		without = append(without, parts[i+1:]...)
		cases = append(cases, "{"+strings.Join(without, ",")+"}", "{"+part+","+currentGolden[1:])
		key, _, ok := strings.Cut(part, ":")
		if !ok {
			t.Fatal("bad fixed test vector")
		}
		for _, replacement := range []string{key + ":null", strings.ToUpper(key) + part[len(key):], key + ":true", key + ":{}"} {
			copyParts := append([]string(nil), parts...)
			copyParts[i] = replacement
			cases = append(cases, "{"+strings.Join(copyParts, ",")+"}")
		}
	}
	reversed := append([]string(nil), parts...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	cases = append(cases,
		"{"+strings.Join(reversed, ",")+"}",
		strings.Replace(currentGolden, `"schema":1`, `"schema":1,"x":0`, 1),
		strings.Replace(currentGolden, `"schema"`, `"sche\u006da"`, 1),
		strings.Replace(currentGolden, `"zerb.current"`, `"zerb\u002ecurrent"`, 1),
		strings.Replace(currentGolden, `"schema":1`, `"schema":1.0`, 1),
		strings.Replace(currentGolden, `"schema":1`, `"schema":"1"`, 1),
		strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":18446744073709551616`, 1),
		strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":-1`, 1),
		strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":1e0`, 1),
		strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":01`, 1),
		strings.Replace(currentGolden, `"stable"`, "\"stab\xffe\"", 1),
	)
	// These last-wins duplicates fit the input bound; they reach strict decode.
	for _, duplicate := range []string{`"instance":null`, `"operation_id":""`, `"candidate":null`, `"receipt":null`, `"writer_token":""`} {
		input := "{" + duplicate + "," + currentGolden[1:]
		if len(input) > currentRecordLimit {
			t.Fatal("duplicate regression no longer reaches the decoder")
		}
		cases = append(cases, input)
	}
	for i, input := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			data := []byte(input)
			before := bytes.Clone(data)
			if record, err := decodeCurrentRecord(data); err != errCurrentRecord || record != (currentRecord{}) {
				t.Fatal("accepted noncanonical bytes or returned partial identity")
			}
			if !bytes.Equal(data, before) {
				t.Fatal("decoder mutated input")
			}
		})
	}
	if _, err := decodeCurrentRecord(nil); err != errCurrentRecord {
		t.Fatal("nil input accepted")
	}
}

func TestCurrentRecordOwnershipAndConcurrentUse(t *testing.T) {
	input := []byte(currentGolden)
	record, err := decodeCurrentRecord(input)
	if err != nil {
		t.Fatal(err)
	}
	clear(input)
	first, err := encodeCurrentRecord(record)
	if err != nil || string(first) != currentGolden {
		t.Fatal("decoded record retained a mutable input alias")
	}
	clear(first)
	if record != currentFixture() {
		t.Fatal("encoded output mutated input record")
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 32; j++ {
				data, err := encodeCurrentRecord(record)
				if err != nil || string(data) != currentGolden {
					t.Error("shared record changed")
					return
				}
				if decoded, err := decodeCurrentRecord(data); err != nil || decoded != record {
					t.Error("concurrent codec lost identity")
					return
				}
			}
		}()
	}
	group.Wait()
}

func FuzzCurrentRecordCanonicalDecode(f *testing.F) {
	f.Add([]byte(currentGolden))
	f.Add([]byte(strings.Replace(currentGolden, `"writer_generation":1`, `"writer_generation":18446744073709551615`, 1)))
	f.Add([]byte(`{"schema":1,"schema":1}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		before := bytes.Clone(data)
		record, err := decodeCurrentRecord(data)
		if !bytes.Equal(data, before) {
			t.Fatal("decoder mutated input")
		}
		if err != nil {
			if err != errCurrentRecord || record != (currentRecord{}) {
				t.Fatal("rejection leaked identity or error detail")
			}
			return
		}
		encoded, err := encodeCurrentRecord(record)
		if err != nil || !bytes.Equal(encoded, data) || len(encoded) > currentRecordLimit {
			t.Fatal("decoder accepted a noncanonical representation")
		}
		if decoded, err := decodeCurrentRecord(encoded); err != nil || decoded != record {
			t.Fatal("round trip changed identity")
		}
	})
}
