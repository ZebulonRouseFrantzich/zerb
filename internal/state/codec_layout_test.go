package state

import (
	"crypto/sha256"
	"reflect"
	"testing"
	"unsafe"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// Bind the reservation proof to the concrete private implementation layouts.
func TestCodecImplementationLayout(t *testing.T) {
	for _, item := range []struct {
		name             string
		actual, reserved uintptr
	}{
		{"candidate", unsafe.Sizeof(receipt.Candidate{}), 480},
		{"completed", unsafe.Sizeof(receipt.Completed{}), 848},
		{"pending", unsafe.Sizeof(pendingReceipt{}), 888},
		{"journal", unsafe.Sizeof(journalRecord{}), 248},
		{"component", unsafe.Sizeof(receipt.Component{}), 72},
		{"file", unsafe.Sizeof(receipt.File{}), 48},
		{"tool", unsafe.Sizeof(receipt.Tool{}), 56},
		{"lock", unsafe.Sizeof(receipt.LockValue{}), 32},
		{"executable", unsafe.Sizeof(receipt.Executable{}), 64},
		{"tool observation", unsafe.Sizeof(receipt.ToolObservation{}), 64},
		{"destination observation", unsafe.Sizeof(receipt.DestinationObservation{}), 96},
		{"string", unsafe.Sizeof(""), 16},
		{"reader", unsafe.Sizeof(codecReader{}), 40},
		{"writer", unsafe.Sizeof(codecJSONWriter{}), 48},
		{"reservation and scalar plan", unsafe.Offsetof(codecPlan{}.counts) + unsafe.Sizeof(codecBudget{}), 64},
		{"collection counters", unsafe.Sizeof(codecPlan{}.counts), 104},
		{"spans", unsafe.Sizeof(codecPlan{}.spans), 48},
		{"hashes", unsafe.Sizeof(codecPlan{}.hashes), 96},
	} {
		if item.actual > item.reserved {
			t.Fatalf("%s layout %d exceeds %d", item.name, item.actual, item.reserved)
		}
	}
	// Fixed model/control storage is separate from charged wire, strings, slices,
	// tree buffers and source scratch. It is not a runtime-memory ceiling.
	candidate := 2*unsafe.Sizeof(receipt.Candidate{}) + 40 + 96 + 64 + 104 + 48 + 120 + 96 + 72 + 72 + 48 + 24
	pending := candidate - 2*unsafe.Sizeof(receipt.Candidate{}) + 2*unsafe.Sizeof(pendingReceipt{}) + 88
	if candidate != 1744 || pending != 2648 || candidate > 2048 || pending > 3968 {
		t.Fatalf("fixed storage does not fit: candidate=%d pending=%d", candidate, pending)
	}
	if size := reflect.TypeOf(sha256.New()).Elem().Size(); size > 120 {
		t.Fatalf("hash state %d exceeds fixed plan", size)
	}
}
