package state

import (
	"testing"
	"unsafe"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

func TestCommittedControlLayoutReservation(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"input", unsafe.Sizeof(committedControlInput{}), 392},
		{"values", unsafe.Sizeof(committedControlValues{}), 3016},
		{"links", unsafe.Sizeof(committedControlLinks{}), 104},
		{"counters", unsafe.Sizeof(committedControlCounters{}), 40},
		{"preflight result", unsafe.Sizeof(committedControlPreflightResult{}), 16},
		{"candidate", unsafe.Sizeof(receipt.Candidate{}), 480},
		{"pending receipt", unsafe.Sizeof(pendingReceipt{}), 888},
		{"journal", unsafe.Sizeof(journalRecord{}), 248},
		{"current", unsafe.Sizeof(currentRecord{}), 144},
		{"writer", unsafe.Sizeof(writerOwner{}), 128},
		{"plan", unsafe.Sizeof(codecPlan{}), 304},
		{"reader", unsafe.Sizeof(codecReader{}), 40},
	} {
		if tt.got != tt.want {
			t.Fatalf("%s layout changed: got %d, reviewed %d", tt.name, tt.got, tt.want)
		}
	}
	// Schema strings and identities retained by the typed small records.
	smallPayload := len("zerb.current") + len("stable") + 4*32 + 2*71 + len("zerb.writer-owner") + len("held") + 3*32
	if smallPayload != 405 {
		t.Fatal("small-record payload schedule changed")
	}
	// The reserve callee's input copy and fixed bookkeeping coexist with preflight.
	preflight := unsafe.Sizeof(pendingReceipt{}) + unsafe.Sizeof(codecPlan{}) + unsafe.Sizeof(codecReader{}) + 24 + 32 +
		unsafe.Sizeof(committedControlInput{}) + unsafe.Sizeof(committedControlCounters{}) +
		2*unsafe.Sizeof(committedControlPreflightResult{}) + unsafe.Sizeof(codecBudget{})
	if preflight > 3968 {
		t.Fatal("serial preflight exceeds the reviewed fixed block")
	}
	outer := 2*unsafe.Sizeof(committedControlInput{}) + unsafe.Sizeof(committedControlValues{}) +
		2*unsafe.Sizeof(committedControlLinks{}) + 2*unsafe.Sizeof(committedControlCounters{}) + 2*16 + 3968 + uintptr(smallPayload)
	if uint64(outer) != committedControlOuter || committedControlOuter != 8493 ||
		committedControlWireBudget != 40_000_000 || committedControlReservationLimit != 80_008_493 {
		t.Fatalf("reservation changed: outer=%d, limit=%d", outer, committedControlReservationLimit)
	}
}
