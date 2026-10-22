package profile

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestReservationRejectsOverflowWithoutChangingOwnership(t *testing.T) {
	var budget reservation
	if !budget.add(maxReservationBytes-1, 1) || !budget.add(1, 1) {
		t.Fatal("exact reservation limit rejected")
	}
	for _, charge := range [][2]uint64{{1, 1}, {math.MaxUint64, 2}, {2, math.MaxUint64}} {
		if budget.add(charge[0], charge[1]) || budget.used != maxReservationBytes {
			t.Fatal("over-limit or overflowing charge accepted or changed reservation")
		}
	}
	var empty reservation
	if empty.add(math.MaxUint64, 2) || empty.used != 0 {
		t.Fatal("multiplication overflow accepted")
	}
	if !empty.add(math.MaxUint64, 0) || empty.used != 0 {
		t.Fatal("zero-sized charge changed reservation")
	}
	invalid := reservation{used: math.MaxUint64}
	if invalid.add(0, 0) || invalid.used != math.MaxUint64 {
		t.Fatal("invalid existing reservation accepted or wrapped")
	}
}

func TestDocumentLinesMatchesIndependentWindowOracle(t *testing.T) {
	for _, input := range []string{"", "abc", "\n", "\n\n", "a\r\nb\n", strings.Repeat("long-line\n", 9), "123456789\n\n\n\n\n\n\nend", "a\nbb\nccc\ndddd\neeeee\nffffff\nggggggg\nhhhhhhhh"} {
		parts := bytes.Split([]byte(input), []byte{'\n'})
		var maximum, window uint64
		for index, part := range parts {
			if uint64(len(part)) > maximum {
				maximum = uint64(len(part))
			}
			var total uint64
			for end := index; end < len(parts) && end < index+7; end++ {
				total += uint64(len(parts[end]))
				if end < len(parts)-1 {
					total++
				}
			}
			if total > window {
				window = total
			}
		}
		lines, longest, got := documentLines([]byte(input))
		if lines != uint64(len(parts)) || longest != maximum || got != window {
			t.Fatalf("line/context reservation mismatch: lines %d longest %d window %d", lines, longest, got)
		}
	}
}

func TestDocumentReservationEnforcesCombinedLimits(t *testing.T) {
	if !reserveDocument(nil, 0, 0) || !reserveDocument([]byte("schema=1"), 3, 0) {
		t.Fatal("small document rejected")
	}
	if !reserveDocument([]byte(strings.Repeat("#"+strings.Repeat("x", 79)+"\n", 12_000)), 0, 0) {
		t.Fatal("bounded short-line document rejected")
	}
	for _, test := range []struct {
		data           []byte
		tokens, quoted uint64
	}{
		{[]byte(strings.Repeat("\n", 240_000)), 0, 0},
		{[]byte(strings.Repeat("x", 1<<20)), 0, 0},
		{[]byte(strings.Repeat("x", 1<<20+1)), 0, 0},
		{nil, 4097, 0}, {nil, 0, 1}, {nil, math.MaxUint64, 0},
	} {
		if reserveDocument(test.data, test.tokens, test.quoted) {
			t.Fatal("unbounded document storage accepted")
		}
	}
}

func TestDocumentReservationHasNoInputDependentAllocation(t *testing.T) {
	input := []byte(strings.Repeat("# bounded comment\n", 10_000))
	if allocations := testing.AllocsPerRun(10, func() {
		if !reserveDocument(input, 0, 0) {
			panic("reservation unexpectedly rejected")
		}
	}); allocations != 0 {
		t.Fatalf("reservation preflight allocated %g times", allocations)
	}
}
