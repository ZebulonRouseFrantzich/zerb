package state

import (
	"testing"
	"unsafe"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// ARCH-018/024: exact existing representations, with extra wrapper and
// comparison bookkeeping charged before the new entrypoint is implemented.
func TestPrecommitControlLayoutReservation(t *testing.T) {
	basePreflight := unsafe.Sizeof(pendingReceipt{}) + unsafe.Sizeof(codecPlan{}) + unsafe.Sizeof(codecReader{}) + 24 + 32 +
		unsafe.Sizeof(committedControlInput{}) + unsafe.Sizeof(committedControlCounters{}) +
		2*unsafe.Sizeof(committedControlPreflightResult{}) + unsafe.Sizeof(codecBudget{})
	var err error
	bookkeeping := unsafe.Sizeof(committedControlInput{}) + 4*unsafe.Sizeof(committedControlCounters{}) +
		16*unsafe.Sizeof(uintptr(0)) + 4*unsafe.Sizeof(err) + 16*unsafe.Sizeof(int(0)) + 16*unsafe.Sizeof(false)
	if basePreflight != 1760 || basePreflight+bookkeeping != 2648 || basePreflight+bookkeeping > 3968 ||
		unsafe.Sizeof(receipt.Completed{})+bookkeeping+2*unsafe.Sizeof(committedControlLinks{}) > 3968 {
		t.Fatal("precommit fixed control schedule no longer fits")
	}
	if unsafe.Sizeof(committedControlInput{}) != 392 || unsafe.Sizeof(committedControlValues{}) != 3016 ||
		unsafe.Sizeof(committedControlLinks{}) != 104 || unsafe.Sizeof(committedControlCounters{}) != 40 ||
		unsafe.Sizeof(committedControlPreflightResult{}) != 16 || committedControlOuter != 8493 ||
		committedControlReservationLimit != 80_008_493 {
		t.Fatal("precommit representation differs from the reviewed selected schedule")
	}
}
