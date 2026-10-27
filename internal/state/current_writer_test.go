package state

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func TestCurrentWriterCorrespondence(t *testing.T) {
	token := strings.Repeat("4", 32)
	other := strings.Repeat("5", 32)
	third := strings.Repeat("6", 32)
	for _, tt := range []struct {
		name       string
		currentGen uint64
		writerGen  uint64
		writerTok  string
		previous   string
		released   bool
		want       bool
	}{
		{"held same", 3, 3, token, "", false, true},
		{"held same wrong token", 3, 3, other, "", false, false},
		{"held immediate transfer", 1, 2, other, token, false, true},
		{"held immediate wrong predecessor", 2, 3, other, third, false, false},
		{"held older transfer", 1, 4, other, third, false, true},
		{"held older reused predecessor", 1, 4, other, token, false, false},
		{"held older reused active token", 1, 4, token, third, false, false},
		{"held acquire older immediate", 2, 3, other, "", false, false},
		{"held acquire older distant", 1, 4, other, "", false, false},
		{"held future current", 4, 3, token, "", false, false},
		{"held max same", math.MaxUint64, math.MaxUint64, token, third, false, true},
		{"held max immediate", math.MaxUint64 - 1, math.MaxUint64, other, token, false, true},
		{"held max older", 1, math.MaxUint64, other, third, false, true},
		{"held future max current", math.MaxUint64, 1, token, "", false, false},
		{"released initial", 1, 0, "", "", true, false},
		{"released immediate", 1, 2, "", token, true, true},
		{"released even current", 2, 3, "", token, true, true},
		{"released immediate wrong token", 2, 3, "", other, true, false},
		{"released older", 1, 4, "", third, true, true},
		{"released older reused predecessor", 1, 4, "", token, true, false},
		{"released same generation", 4, 4, "", token, true, false},
		{"released future current", 5, 4, "", token, true, false},
		{"released max immediate", math.MaxUint64 - 1, math.MaxUint64, "", token, true, true},
		{"released max same", math.MaxUint64, math.MaxUint64, "", token, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current := currentFixture()
			current.WriterGeneration = tt.currentGen
			writer := writerRecordFixture(tt.writerGen, tt.previous, tt.released)
			writer.Token = tt.writerTok
			beforeCurrent, beforeWriter := current, writer
			if !validCurrentRecord(current) || !validWriterOwnerRecord(writer) {
				t.Fatal("relation case must reach correspondence with valid scalars")
			}
			if got := currentMatchesWriter(current, writer); got != tt.want {
				t.Fatalf("correspondence: got %v, want %v", got, tt.want)
			}
			if current != beforeCurrent || writer != beforeWriter {
				t.Fatal("relation mutated input")
			}
		})
	}
}

func TestCurrentWriterRejectsInvalidAndMismatchedRecords(t *testing.T) {
	for _, change := range []func(*currentRecord, *writerOwner){
		func(c *currentRecord, w *writerOwner) { c.Instance = strings.Repeat("9", 32) },
		func(c *currentRecord, w *writerOwner) { c.Schema = 2 },
		func(c *currentRecord, w *writerOwner) { c.Candidate = "" },
		func(c *currentRecord, w *writerOwner) { c.OperationToken = c.OperationID },
		func(c *currentRecord, w *writerOwner) { c.WriterGeneration = 0 },
		func(c *currentRecord, w *writerOwner) { w.Domain = "zerb.root" },
		func(c *currentRecord, w *writerOwner) { w.Process = processIdentity{} },
		func(c *currentRecord, w *writerOwner) { w.PreviousGeneration = math.MaxUint64 },
		func(c *currentRecord, w *writerOwner) { w.PreviousToken = w.Token },
		func(c *currentRecord, w *writerOwner) { w.Instance = strings.Repeat("9", 32) },
	} {
		current := currentFixture()
		current.WriterGeneration = 3
		writer := writerRecordFixture(3, "", false)
		if !currentMatchesWriter(current, writer) {
			t.Fatal("base relation invalid")
		}
		change(&current, &writer)
		if currentMatchesWriter(current, writer) {
			t.Fatal("invalid or instance-mismatched pair accepted")
		}
	}
}

// Big-integer distance is an independent oracle for all uint64 order edges.
func FuzzCurrentWriterGenerationDistance(f *testing.F) {
	for _, pair := range [][2]uint64{{1, 2}, {2, 3}, {3, 3}, {1, 4}, {math.MaxUint64, 3}, {math.MaxUint64 - 1, math.MaxUint64}, {0, 0}} {
		f.Add(pair[0], pair[1], uint8(0), false)
		f.Add(pair[0], pair[1], uint8(1), true)
	}
	f.Fuzz(func(t *testing.T, currentGen, writerGen uint64, selector uint8, released bool) {
		if currentGen == 0 || writerGen < 3 {
			return // Initial/first/second shapes have explicit rejection vectors.
		}
		current := currentFixture()
		current.WriterGeneration = currentGen
		token := current.WriterToken
		other, third := strings.Repeat("5", 32), strings.Repeat("6", 32)
		previous := []string{"", token, third}[selector%3]
		if released && previous == "" {
			previous = third
		}
		writer := writerRecordFixture(writerGen, previous, released)
		if selector&4 != 0 && previous != token {
			writer.Token = token
		} else if !released {
			writer.Token = other
		}
		if released {
			writer.Token = ""
		}
		if !validCurrentRecord(current) || !validWriterOwnerRecord(writer) {
			t.Fatal("generation oracle requires valid record shapes")
		}
		var distance, writerNumber, currentNumber big.Int
		writerNumber.SetUint64(writerGen)
		currentNumber.SetUint64(currentGen)
		distance.Sub(&writerNumber, &currentNumber)
		want := false
		if distance.Sign() == 0 && !released {
			want = current.WriterToken == writer.Token
		} else if distance.Sign() > 0 && (released || previous != "" && current.WriterToken != writer.Token) {
			if distance.IsUint64() && distance.Uint64() == 1 {
				want = current.WriterToken == previous
			} else {
				want = current.WriterToken != previous
			}
		}
		if got := currentMatchesWriter(current, writer); got != want {
			t.Fatalf("generation distance %s: got %v, want %v", distance.String(), got, want)
		}
	})
}
