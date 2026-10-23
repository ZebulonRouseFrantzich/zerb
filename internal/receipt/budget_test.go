package receipt

import (
	"math"
	"strings"
	"testing"
)

func TestCanonicalReservationCheckedArithmeticAndExactBoundary(t *testing.T) {
	var b reservation
	if !b.add(maxReservationBytes, 1) || b.used != maxReservationBytes || b.add(1, 1) {
		t.Fatal("exact boundary mismatch")
	}
	for _, pair := range [][2]uint64{{math.MaxUint64, 2}, {2, math.MaxUint64}, {1, math.MaxUint64}} {
		var b reservation
		if b.add(pair[0], pair[1]) || b.used != 0 {
			t.Fatal("overflow accepted or budget changed")
		}
	}
	bad := reservation{used: math.MaxUint64}
	if bad.add(0, 0) {
		t.Fatal("invalid accumulator accepted")
	}
}
func TestCanonicalPreflightRejectsRetainedSpareBackingBeforeCopy(t *testing.T) {
	c := unitCandidate(t)
	c.Targets = make([]string, 0, 500001)
	if allocations := testing.AllocsPerRun(10, func() {
		d, b, err := CandidateDigest(c)
		if err == nil || d != "" || b != nil {
			panic("oversize accepted")
		}
	}); allocations != 0 {
		t.Fatalf("oversize structural preflight allocated %g", allocations)
	}
	c = unitCandidate(t)
	c.Components = append(c.Components, Component{Files: make([]File, 0, 180000)})[:2]
	if allocations := testing.AllocsPerRun(10, func() {
		d, b, err := CandidateDigest(c)
		if err == nil || d != "" || b != nil {
			panic("hidden oversize accepted")
		}
	}); allocations != 0 {
		t.Fatalf("spare nested backing preflight allocated %g", allocations)
	}
}
func TestCanonicalPreflightChargesHiddenPayloadWithoutValidatingIt(t *testing.T) {
	c := unitCandidate(t)
	c.Targets = append([]string{}, string([]byte{255}))[:0]
	if _, _, err := CandidateDigest(c); err != nil {
		t.Fatal("unencoded spare value semantically validated")
	}
	c.Targets = append(c.Targets[:0], strings.Repeat("x", 8_000_001))[:0]
	if allocations := testing.AllocsPerRun(10, func() {
		d, b, err := CandidateDigest(c)
		if err == nil || d != "" || b != nil {
			panic("hidden oversize accepted")
		}
	}); allocations != 0 {
		t.Fatalf("spare payload preflight allocated %g", allocations)
	}
}
func TestCanonicalReservationCoversTreeAndOutputTogether(t *testing.T) {
	c := unitCandidate(t)
	var b reservation
	if !b.add(1, 2048) || !b.reserveCandidate(c, true) || !reserveSourceScratch(&b, c.Source.URI) {
		t.Fatal("small candidate reservation failed")
	}
	before := b.used
	var size jsonWriter
	size.candidate(c)
	if !b.add(size.size, 1) || !reserveTrees(&b, c.Components) || b.used <= before+size.size {
		t.Fatal("tree passes not charged cumulatively")
	}
	b.used = maxReservationBytes - size.size
	if !b.add(size.size, 1) || reserveTrees(&b, c.Components) {
		t.Fatal("budget reset between output and tree")
	}
}

func TestCanonicalAPIsAcceptExactWholeCallBudgetAndRejectNextByte(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		c := unitCandidate(t)
		c.Targets = make([]string, 0, 1)
		var b reservation
		if !b.add(1, 2048) || !b.reserveCandidate(c, true) || !reserveSourceScratch(&b, c.Source.URI) {
			t.Fatal("fixture reservation failed")
		}
		var size jsonWriter
		size.candidate(c)
		if !b.add(size.size, 1) || !reserveTrees(&b, c.Components) {
			t.Fatal("fixture output reservation failed")
		}
		remaining := maxReservationBytes - b.used
		full := c.Targets[:1]
		full[0] = strings.Repeat("x", int(remaining))
		if _, _, err := CandidateDigest(c); err != nil {
			t.Fatal("exact complete budget rejected")
		}
		full[0] += "x"
		if allocations := testing.AllocsPerRun(10, func() {
			d, b, err := CandidateDigest(c)
			if err == nil || d != "" || b != nil {
				panic("next byte accepted")
			}
		}); allocations != 0 {
			t.Fatalf("over-budget preflight allocated %g", allocations)
		}
	})
	t.Run("receipt", func(t *testing.T) {
		r := unitCompleted(t)
		r.Results = make([]DestinationObservation, 0, 1)
		var b reservation
		if !b.add(1, 4096) || !b.reserveCompleted(r, true) || !reserveSourceScratch(&b, r.Source.URI) {
			t.Fatal("fixture reservation failed")
		}
		var size jsonWriter
		size.completed(r)
		if !b.add(size.size, 1) || !reserveTrees(&b, r.Components) {
			t.Fatal("fixture output reservation failed")
		}
		remaining := maxReservationBytes - b.used
		full := r.Results[:1]
		full[0].ContentHash = strings.Repeat("x", int(remaining))
		if _, _, err := ReceiptDigest(r); err != nil {
			t.Fatal("exact complete budget rejected")
		}
		full[0].ContentHash += "x"
		if allocations := testing.AllocsPerRun(10, func() {
			d, b, err := ReceiptDigest(r)
			if err == nil || d != "" || b != nil {
				panic("next byte accepted")
			}
		}); allocations != 0 {
			t.Fatalf("over-budget preflight allocated %g", allocations)
		}
	})
}

func TestPrivateWriterCannotGrowReservedCapacityOrWrapSize(t *testing.T) {
	w := jsonWriter{out: make([]byte, 0, 1), writing: true}
	w.raw("ab")
	if !w.failed || len(w.out) != 0 || cap(w.out) != 1 {
		t.Fatal("writer grew unreserved output")
	}
	w = jsonWriter{size: math.MaxUint64}
	w.raw("a")
	if !w.failed || w.size != math.MaxUint64 {
		t.Fatal("writer size overflow accepted")
	}
}
