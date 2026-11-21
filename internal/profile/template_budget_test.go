package profile

import (
	"math"
	"testing"
)

func TestTemplateReservationAcceptsCombinedPublicLimits(t *testing.T) {
	for _, size := range []int{maxTemplateBytes - 1, maxTemplateBytes} {
		for _, actions := range []uint64{maxTemplateActions - 1, maxTemplateActions} {
			if !reserveTemplate(make([]byte, size), actions) {
				t.Fatal("combined public byte/action limits rejected")
			}
		}
	}
}

func TestTemplateReservationRejectsExcessBeforeArithmetic(t *testing.T) {
	if reserveTemplate(make([]byte, maxTemplateBytes+1), 0) {
		t.Fatal("oversized source admitted")
	}
	for _, actions := range []uint64{maxTemplateActions + 1, math.MaxUint64} {
		if reserveTemplate(nil, actions) {
			t.Fatal("excess or overflowing action count admitted")
		}
	}
}

func TestTemplateReservationRequiresNoAllocation(t *testing.T) {
	source := make([]byte, maxTemplateBytes)
	if count := testing.AllocsPerRun(10, func() {
		if !reserveTemplate(source, maxTemplateActions) {
			panic("valid reservation rejected")
		}
	}); count != 0 {
		t.Fatalf("reservation allocated %g times", count)
	}
}
