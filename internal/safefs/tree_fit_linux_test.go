//go:build linux

package safefs

import (
	"math"
	"testing"
	"unsafe"
)

// Provisional representation only: no filesystem or allocation enforcement proof.
type treeFitEntry struct {
	stamp      fileStamp
	digest     [32]byte
	pathOffset uint32
	pathLength uint16
	depth      uint8
	kind       uint8
	parent     uint32
	flags      uint32
}

type treeFitTraversal struct {
	stamp        listingStamp
	next, parent uint32
}

type treeFitPerEntry struct {
	inventory   [4]treeFitEntry
	order       [4]uint32
	traversal   [4]treeFitTraversal
	bookkeeping [4]uint64
}

type treeFitViewTicket struct {
	bytes uint64
	fds   uint32
	flags uint32
}

type treeFitDirState struct {
	dirState
	gate   *uint64 // Placeholder for the intended shared stage pointer.
	ticket *treeFitViewTicket
}

type treeFitRoleEntry struct {
	roleEntry
	gate   *uint64
	ticket *treeFitViewTicket
}

type treeFitTopologyID struct {
	id    uint64
	index uint32
}

type treeFitSpan struct{ start, length uint32 }

type treeFitTopologyRow struct {
	id, parent, device uint64
	root, point, kind  treeFitSpan
}

type treeFitTopologyCoordinate struct {
	device, inode       uint64
	kind, physicalStart uint32
	physicalLength      uint16
}

type treeFitTopologyCone struct {
	coordinate, path, next, flags uint32
}

func TestTreeFitTopologyLayout(t *testing.T) {
	if unsafe.Sizeof(treeFitTopologyRow{}) > 72 || unsafe.Sizeof(treeFitTopologyID{}) > 16 ||
		unsafe.Sizeof(treeFitTopologyCoordinate{}) > 32 || unsafe.Sizeof(treeFitTopologyCone{}) > 16 ||
		unsafe.Sizeof(topologyPath{}) > 96 {
		t.Fatal("topology record exceeds its conservative slot")
	}
	// Includes retained borrowed paths from both Add topology passes and one
	// current paths array; retained coordinate/namespace strings use arenas.
	aux := uint64(16384 + 4*16384 + 16384 + 4*16384 + 256*32 + 2*256*4 + 1024*16 + 128*16 +
		3*64*96 + 256*96 + 3*64*96 + 64*4 + 74*8 + 3*12289 + 64*4096 + 131072 + 8192)
	if aux > 1<<20 {
		t.Fatalf("topology auxiliary storage = %d", aux)
	}
	topology := uint64(3*(8<<20) + 256*8192 + 16384*72 + 16384*16 + (1 << 20))
	if topology != 29753344 {
		t.Fatalf("topology capacity = %d", topology)
	}
	total, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, topology)
	if !ok || total != 131727360 || total > math.MaxInt32 {
		t.Fatalf("combined stage/topology = %d, admitted %v", total, ok)
	}
	if _, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, topology+2490369); ok {
		t.Fatal("combined topology/result overflow admitted")
	}
	// Role target copies outlive the temporary topology slab.
	retainedTargets := uint64(64 * 4096)
	if total, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, topology+retainedTargets); !ok || total != 131989504 {
		t.Fatalf("retained role targets = %d, admitted %v", total, ok)
	}
	t.Logf("topology=%d auxiliary=%d combined=%d", topology, aux, total)
}

func treeFitReservation(n, p, f, s, r uint64) (uint64, bool) {
	total := uint64(1 << 20)
	for _, term := range [][2]uint64{{2048, n}, {4, p}, {512, f}, {1, s}, {1, r}} {
		if term[1] > math.MaxUint64/term[0] {
			return 0, false
		}
		value := term[0] * term[1]
		if value > math.MaxUint64-total {
			return 0, false
		}
		total += value
	}
	return total, total <= 128<<20
}

func TestTreeFitLayoutAndSimultaneousMaxima(t *testing.T) {
	if unsafe.Sizeof(treeFitRoleEntry{}) > 96 {
		t.Fatalf("intended role entry exceeds 96-byte slot: %d", unsafe.Sizeof(treeFitRoleEntry{}))
	}
	// Worst case is a one-edge view, so it cannot amortize wrapper costs.
	view := unsafe.Sizeof(treeFitDirState{}) + unsafe.Sizeof(Dir{}) + unsafe.Sizeof(edge{}) +
		unsafe.Sizeof(treeFitViewTicket{}) + 255
	if view > 512 {
		t.Fatalf("one-edge view exceeds per-FD charge: %d", view)
	}
	if unsafe.Sizeof(treeFitPerEntry{}) > 2048 {
		t.Fatalf("per-entry metadata exceeds reservation: %d", unsafe.Sizeof(treeFitPerEntry{}))
	}
	// Fixed path arenas coexist with all four metadata/sort/traversal snapshots.
	actual := uint64(unsafe.Sizeof(treeFitPerEntry{}))*16384 + 4*(16<<20)
	reserved := uint64(2048*16384 + 4*(16<<20))
	if actual > reserved {
		t.Fatalf("layout %d exceeds metadata/path reservation %d", actual, reserved)
	}
	base, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, 0)
	if !ok || base != 101974016 {
		t.Fatalf("simultaneous maxima = %d, admitted %v", base, ok)
	}
	for _, tc := range []struct {
		name string
		r    uint64
		ok   bool
	}{
		{"below", 32243711, true},
		{"exact", 32243712, true},
		{"above", 32243713, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, tc.r)
			if ok != tc.ok {
				t.Fatalf("result bytes %d: admitted %v", tc.r, ok)
			}
		})
	}
	t.Logf("entry=%d traversal=%d combined=%d actual arrays=%d", unsafe.Sizeof(treeFitEntry{}), unsafe.Sizeof(treeFitTraversal{}), unsafe.Sizeof(treeFitPerEntry{}), actual)
}

func TestTreeFitArithmeticAndDescriptorPeaks(t *testing.T) {
	for _, tc := range [][5]uint64{
		{math.MaxUint64, 0, 0, 0, 0},
		{0, math.MaxUint64, 0, 0, 0},
		{0, 0, math.MaxUint64, 0, 0},
		{0, 0, 0, math.MaxUint64, 0},
		{0, 0, 0, 0, math.MaxUint64},
		{math.MaxUint64/2048 + 1, 0, 0, 0, 0},
		{0, math.MaxUint64 / 4, 0, 0, 0},
	} {
		if _, ok := treeFitReservation(tc[0], tc[1], tc[2], tc[3], tc[4]); ok {
			t.Fatalf("overflow or overbudget input admitted: %v", tc)
		}
	}
	// These are ownership arithmetic examples, not opened-descriptor tests.
	for a := 1; 2*a+2+8 <= 256; a++ {
		if peak := a + 1 + 64 + 8; peak > 256 {
			t.Fatalf("admitted absolute chain %d cannot support bounded walk: %d", a, peak)
		}
	}
	// Two retained public chains coexist while a child is constructed.
	if peak := 7 + 70 + 71 + 1; peak != 149 {
		t.Fatalf("public child overlap = %d", peak)
	}
	// A read cannot use a caller's 256-MiB file ceiling as a workspace grant.
	if _, ok := treeFitReservation(16384, 16<<20, 256, 128<<10, 256<<20); ok {
		t.Fatal("whole maximum-sized file result admitted")
	}
}
