package state

import (
	"crypto/sha256"
	"reflect"
	"testing"
	"unsafe"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// Layout evidence before codec implementation; replace the planned private slots
// with actual private types when they exist. These assertions grant no runtime fit.
func TestCodecModelLayoutBeforeImplementation(t *testing.T) {
	type pendingModel struct {
		Schema int
		Domain string
		Digest receipt.Digest
		Facts  receipt.Completed
	}
	type journalModel struct {
		Schema                                        int
		Domain, Instance, OperationID, OperationToken string
		WriterGeneration                              uint64
		WriterToken, WriterPreviousToken              string
		Previous, PreviousReceipt, Candidate          receipt.Digest
		Phase                                         Phase
		Preconditions, Results                        []receipt.DestinationObservation
		CompletedPhases                               []Phase
	}
	for _, item := range []struct {
		name             string
		actual, reserved uintptr
	}{
		{"candidate", unsafe.Sizeof(receipt.Candidate{}), 480},
		{"completed", unsafe.Sizeof(receipt.Completed{}), 848},
		{"pending", unsafe.Sizeof(pendingModel{}), 888},
		{"journal", unsafe.Sizeof(journalModel{}), 248},
		{"component", unsafe.Sizeof(receipt.Component{}), 72},
		{"file", unsafe.Sizeof(receipt.File{}), 48},
		{"tool", unsafe.Sizeof(receipt.Tool{}), 56},
		{"lock", unsafe.Sizeof(receipt.LockValue{}), 32},
		{"executable", unsafe.Sizeof(receipt.Executable{}), 64},
		{"tool observation", unsafe.Sizeof(receipt.ToolObservation{}), 64},
		{"destination observation", unsafe.Sizeof(receipt.DestinationObservation{}), 96},
		{"string", unsafe.Sizeof(""), 16},
	} {
		if item.actual > item.reserved {
			t.Fatalf("%s layout %d exceeds %d", item.name, item.actual, item.reserved)
		}
	}
	if size := reflect.TypeOf(sha256.New()).Elem().Size(); size > 120 {
		t.Fatalf("hash state %d exceeds fixed plan", size)
	}
}
