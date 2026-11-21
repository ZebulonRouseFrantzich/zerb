//go:build linux

package safefs

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func parsedTreeTopology(raw []byte) (*treeTopology, error) {
	t := &treeTopology{root: -1, decoded: make([]byte, 0, min(len(raw), topologyMaxBytes)), rows: make([]treeMount, 0, min(bytes.Count(raw, []byte{'\n'}), topologyMaxRecords)), ids: make([]treeMountID, 0, min(bytes.Count(raw, []byte{'\n'}), topologyMaxRecords))}
	return t, t.parse(raw)
}

func checkTreeTopologyParser(t *testing.T, raw []byte) {
	t.Helper()
	plain, plainErr := parseTopology(raw)
	bounded, boundedErr := parsedTreeTopology(raw)
	if (plainErr == nil) != (boundedErr == nil) {
		t.Fatalf("stage topology parser differs from ordinary parser: plain=%v bounded=%v", plainErr, boundedErr)
	}
	if plainErr != nil {
		return
	}
	if len(plain.rows) != len(bounded.rows) || plain.root != bounded.root {
		t.Fatal("stage topology graph differs")
	}
	for i, row := range plain.rows {
		got := bounded.rows[i]
		if row.id != got.id || row.parent != got.parent || row.device != got.device || row.root != string(bounded.data(got.root)) || row.point != string(bounded.data(got.point)) || row.kind != string(bounded.data(got.kind)) {
			t.Fatal("stage topology row differs")
		}
		if index, ok := bounded.find(row.id); !ok || index != i {
			t.Fatal("stage topology id index differs")
		}
	}
}

func TestTreeTopologyParserBoundsAndGrammar(t *testing.T) {
	for _, raw := range []string{
		topologyTestRoot,
		"1 1 0:1 /a\rb / rw - tmpfs none rw\n",
		topologyTestRoot + "2 1 0:2 / /a\rb rw - tmpfs none rw\n",
		"1 1 0:1 /a\xffb / rw - tmpfs none rw\n",
		"1 999 0:1 / / rw - tmpfs tmpfs rw\n",
		"1 1 0:42 /@home / rw,idmapped shared:1 master:2 propagate_from:3 - btrfs none rw\n",
		"1 1 0:1 /a\\040b\\011c\\012d\\134e / rw - tmpfs none rw\n",
		topologyTestRoot + "2 1 0:2 / /other rw - tmpfs none rw\n3 2 0:3 / /other rw - tmpfs none rw\n",
		topologyTestRoot + "2 1 0:2 / /other rw - overlay none rw\n",
		strings.TrimSuffix(topologyTestRoot, "\n"),
		topologyTestRoot + "2 1 0:2 / / rw - tmpfs none rw\n",
		topologyTestRoot + "1 1 0:2 / /a rw - tmpfs none rw\n",
		topologyTestRoot + "2 9 0:2 / /a rw - tmpfs none rw\n",
		topologyTestRoot + "2 3 0:2 / /a rw - tmpfs none rw\n3 2 0:3 / /a rw - tmpfs none rw\n",
		"1 2 0:1 / / rw - tmpfs none rw\n2 1 0:2 / /a rw - tmpfs none rw\n",
		topologyTestRoot + "2 1 0:2 / /a rw - tmpfs none rw\n3 2 0:3 / /b rw - tmpfs none rw\n",
		strings.Replace(topologyTestRoot, "rw -", "rw idmapped -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw shared:1 shared:2 -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw shared:0 -", 1),
		strings.Replace(topologyTestRoot, "1 1", "18446744073709551616 1", 1),
		strings.Replace(topologyTestRoot, "0:1", "4294967296:1", 1),
		strings.Replace(topologyTestRoot, "/ /", "/a/../b /", 1),
		strings.Replace(topologyTestRoot, "/ /", `/a\041 /`, 1),
		strings.Replace(topologyTestRoot, "rw -", "rw\x00 -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw\t -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw - -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw unbindable -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw unbindable:1 -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw shared:18446744073709551615 -", 1),
		strings.Replace(topologyTestRoot, "rw -", "rw shared:18446744073709551616 -", 1),
	} {
		checkTreeTopologyParser(t, []byte(raw))
	}
	for _, path := range []string{"/" + strings.Repeat("a", 4095), "/" + strings.Repeat("a", 4096), strings.Repeat("/a", 256), strings.Repeat("/a", 257), `/\134040`} {
		checkTreeTopologyParser(t, []byte("1 1 0:1 "+path+" / rw - tmpfs none rw\n"))
	}
	var records strings.Builder
	records.WriteString(topologyTestRoot)
	for i := 2; i <= topologyMaxRecords; i++ {
		fmt.Fprintf(&records, "%d 1 0:1 / /m%d rw - tmpfs none rw\n", i, i)
	}
	checkTreeTopologyParser(t, []byte(records.String()))
	records.WriteString("16385 1 0:1 / /overflow rw - tmpfs none rw\n")
	checkTreeTopologyParser(t, []byte(records.String()))
	prefix := "1 1 0:1 / / rw - tmpfs none "
	line := prefix + strings.Repeat("x", topologyMaxLine-len(prefix)-1) + "\n"
	checkTreeTopologyParser(t, []byte(line))
	checkTreeTopologyParser(t, []byte(line[:len(line)-1]+"x\n"))
	checkTreeTopologyParser(t, make([]byte, topologyMaxBytes+1))
}

func TestTreeTopologyActualStorageAndAdmission(t *testing.T) {
	if unsafe.Sizeof(treeMount{}) > 72 || unsafe.Sizeof(treeMountID{}) > 16 || unsafe.Sizeof(treeCoordinate{}) > 40 || unsafe.Sizeof(treeCone{}) > 16 {
		t.Fatal("production topology record exceeds reservation")
	}
	// Headers, all fixed arrays and path scratch fit the reviewed auxiliary slab.
	// Remaining space covers role controls, tight targets and metadata wrappers.
	aux := uint64(unsafe.Sizeof(treeTopology{})) + 3*64*96 + 64*4096 + 131072 + 8192
	if aux > 1<<20 {
		t.Fatalf("production topology auxiliary storage exceeds 1 MiB: %d", aux)
	}
	if n := testing.AllocsPerRun(10, func() {
		if top, err := openTreeTopology(nil, treeMaxFDs-10, nil); top != nil || err == nil {
			t.Fatal("descriptor preflight admitted")
		}
	}); n != 0 {
		t.Fatal("descriptor refusal allocated topology storage")
	}
	top := &topology{fdLimit: 256, frontendFDs: 245, self: edge{fd: -1}, nsLink: edge{fd: -1}}
	if !top.room(11) || top.room(12) {
		t.Fatal("stage metadata descriptor headroom is wrong")
	}
}

func TestTreeTopologyCoordinateIdentityAndCapacity(t *testing.T) {
	top := &treeTopology{physical: make([]byte, 0, 256*8192)}
	id := Identity{Device: 99, Inode: 1, Mode: unix.S_IFDIR}
	i, err := top.record(7, []byte("/physical"), id)
	if err != nil {
		t.Fatal(err)
	}
	used := len(top.physical)
	if j, err := top.record(7, []byte("/physical"), id); err != nil || j != i || len(top.physical) != used {
		t.Fatal("reobservation allocated or lost mapped/actual device distinction")
	}
	if _, err := top.record(8, []byte("/alias"), id); err == nil {
		t.Fatal("same object acquired another coordinate")
	}
	changed := id
	changed.Inode++
	if _, err := top.record(7, []byte("/physical"), changed); err == nil {
		t.Fatal("same coordinate acquired another object")
	}
	for n := 1; n < 256; n++ {
		next := Identity{Device: 99, Inode: uint64(n + 1), Mode: unix.S_IFDIR}
		if _, err := top.record(7, []byte(fmt.Sprintf("/p%d", n)), next); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := top.record(7, []byte("/overflow"), Identity{Device: 99, Inode: 257, Mode: unix.S_IFDIR}); err == nil {
		t.Fatal("coordinate overflow admitted")
	}
}

func TestTreeTopologyConeEquivalence(t *testing.T) {
	paths := []string{"/", "/a", "/a/b", "/ab", "/source", "/source/child", "/source/future", "/a/other", "/b", "/b/interior"}
	for _, physicalA := range paths {
		for _, physicalB := range paths {
			for _, namespaceA := range paths {
				for _, namespaceB := range paths {
					top := &treeTopology{physical: make([]byte, 0, len(physicalA)+len(physicalB))}
					a, _ := top.record(1, []byte(physicalA), Identity{Device: 11, Inode: 1, Mode: unix.S_IFDIR})
					// Cones can share a physical route; a second record is unnecessary then.
					b := a
					if physicalA != physicalB {
						b, _ = top.record(1, []byte(physicalB), Identity{Device: 11, Inode: 2, Mode: unix.S_IFDIR})
					}
					top.paths[0].target = namespaceA
					top.paths[1].target = namespaceB
					top.pathCount = 2
					top.cones[0] = treeCone{coordinate: a, path: 0}
					top.cones[1] = treeCone{coordinate: b, path: 1}
					top.coneCount = 2
					left := topologyFootprint{[]topologyCone{{1, physicalA, namespaceA}}}
					right := topologyFootprint{[]topologyCone{{1, physicalB, namespaceB}}}
					for _, allow := range []bool{false, true} {
						if got, want := top.compatible(treeFootprint{0, 1}, treeFootprint{1, 1}, allow), topologyCompatible(left, right, allow); got != want {
							t.Fatalf("cone result differs physical=%q/%q namespace=%q/%q allow=%v", physicalA, physicalB, namespaceA, namespaceB, allow)
						}
					}
				}
			}
		}
	}
}

func testTreeRoleGate() *stageState {
	s := &stageState{phase: treeActive, workspace: treeFixedWorkspace}
	s.drained = sync.NewCond(&s.mu)
	return s
}

func TestTreeRolesRoutingAndRosterAdmission(t *testing.T) {
	gate := testTreeRoleGate()
	roles := &Roles{state: &rolesState{}}
	if roles.hasStage(nil) {
		t.Fatal("empty roster has a stage")
	}
	// Another Add can commit after dispatch but before the generic path locks.
	roles.state.entries = []roleEntry{{gate: gate}}
	roles.state.generation++
	if roles.validateWithoutStage() == nil || roles.addWithoutStage(RoleDestination, &Dir{}) == nil || roles.state.poisoned {
		t.Fatal("stale generic dispatch performed stage I/O")
	}
	other := testTreeRoleGate()
	roles.state.entries = append(roles.state.entries, roleEntry{gate: other})
	a, err := roles.admitTreeRoles(nil)
	if err != nil {
		t.Fatal(err)
	}
	if gate.roleRefs != 1 || other.roleRefs != 1 || gate.active != 1 || other.active != 1 {
		t.Fatal("multi-stage roster did not enroll every gate")
	}
	roles.state.generation++
	if a.unchanged(roles.state) {
		t.Fatal("changed role roster was accepted")
	}
	a.finish(false)
	if gate.roleRefs != 0 || other.roleRefs != 0 || gate.active != 0 || other.active != 0 || gate.workspace != treeFixedWorkspace || other.workspace != treeFixedWorkspace {
		t.Fatal("roster refusal leaked reservations")
	}
	gate.workspace = treeMaxWorkspace - 1
	if _, err := roles.admitTreeRoles(nil); err == nil || gate.phase != treeActive || other.active != 0 {
		t.Fatal("workspace admission was not a pre-effect refusal")
	}
	gate.workspace = treeFixedWorkspace
	roles.state.generation = ^uint64(0)
	beforeFDs, beforeWorkspace := gate.fds, gate.workspace
	if roles.Add(RoleDestination, &Dir{state: &dirState{gate: gate}}) == nil || gate.phase != treeActive || gate.fds != beforeFDs || gate.workspace != beforeWorkspace || roles.state.poisoned {
		t.Fatal("generation overflow was not a pre-effect refusal")
	}
	plain := &Roles{state: &rolesState{generation: ^uint64(0)}}
	if plain.Add(RoleDestination, &Dir{}) == nil || plain.state.poisoned {
		t.Fatal("ordinary generation overflow entered I/O")
	}
	if plain.Close() != nil || !plain.state.closed || plain.state.generation != ^uint64(0) {
		t.Fatal("maximum generation blocked close or wrapped")
	}
}

func TestTreeRolesParameterPreflightDoesNotPoison(t *testing.T) {
	gate := testTreeRoleGate()
	roles := &Roles{state: &rolesState{home: "/home", entries: []roleEntry{{role: RoleDestination, target: "/dest", gate: gate}}}}
	dir := &Dir{state: &dirState{gate: gate, chain: []edge{{fd: -1}, {fd: -1, name: "new"}}}}
	// This placement is lexically impossible. Invalid FDs ensure any attempted
	// operational validation would poison the stage and fail this assertion.
	if roles.Add(RoleSource, dir) == nil || gate.phase != treeActive || roles.state.poisoned || gate.active != 0 || gate.roleRefs != 0 {
		t.Fatal("parameter refusal performed admitted filesystem work")
	}
	if roles.Add(RoleState, dir) == nil || gate.phase != treeActive || roles.state.poisoned {
		t.Fatal("invalid root advance poisoned a valid lifetime")
	}
	if roles.Add(RoleDestination, dir) == nil || gate.phase != treePreserve {
		t.Fatal("admitted descriptor failure did not poison the stage")
	}
}

func TestTreeRolesRetainedMetadataAccounting(t *testing.T) {
	gate := testTreeRoleGate()
	state := &rolesState{}
	var old []roleEntry
	for count := 1; count <= 64; count++ {
		op, err := gate.beginTopology(true, 32<<10, treeTopologyBytes)
		if err != nil {
			t.Fatal(err)
		}
		candidate := make([]roleEntry, count)
		for i := range candidate {
			candidate[i].target = "/retained"
		}
		old = state.entries
		state.retainTreeRoleEntries(candidate, gate, op)
		if !op.finish(false) {
			t.Fatal("metadata transfer poisoned active lifetime")
		}
		if len(state.entries) != count || cap(state.entries) != count || state.metadataTicket.bytes != uint64(96*count+16) || gate.workspace != uint64(treeFixedWorkspace+96*count+16) {
			t.Fatal("retained metadata capacity lacks a persistent reservation")
		}
		for _, entry := range old {
			if entry.target != "" {
				t.Fatal("old metadata retained paths after reservation release")
			}
		}
	}
	if err := (&Roles{state: state}).Close(); err != nil || gate.workspace != treeFixedWorkspace || state.metadataTicket != nil || state.metadataGate != nil {
		t.Fatal("role close leaked persistent metadata reservation")
	}
}

func TestTreeRolesSeveralMetadataLifetimesShareOneBudget(t *testing.T) {
	gate := testTreeRoleGate()
	var candidate [64]roleEntry
	var sets []*Roles
	defer func() {
		for _, set := range sets {
			if err := set.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	for len(sets) < 512 {
		before := gate.workspace
		op, err := gate.beginTopology(true, 32<<10, treeTopologyBytes)
		if err != nil {
			if gate.phase != treeActive || gate.workspace != before || gate.active != 0 {
				t.Fatal("shared metadata exhaustion changed admitted lifetime")
			}
			break
		}
		state := &rolesState{}
		state.retainTreeRoleEntries(candidate[:], gate, op)
		if !op.finish(false) {
			t.Fatal("metadata-only bookkeeping poisoned lifetime")
		}
		sets = append(sets, &Roles{state: state})
		if gate.workspace != uint64(treeFixedWorkspace+len(sets)*(64*96+16)) {
			t.Fatal("separate Roles metadata lifetimes escaped shared workspace")
		}
	}
	if len(sets) < 2 || len(sets) == 512 {
		t.Fatal("separate Roles metadata did not encounter bounded admission")
	}
	for _, set := range sets {
		if err := set.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if gate.workspace != treeFixedWorkspace || gate.roleRefs != 0 {
		t.Fatal("separate metadata lifetime cleanup leaked reservation")
	}
}

func TestTreeRolesCloseReleasesObserver(t *testing.T) {
	_, parent := localFixture(t)
	fd, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	gate := testTreeRoleGate()
	op, err := gate.beginRole(1, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	ticket := op.retainRole(1, 4)
	op.finish(false)
	roles := &Roles{state: &rolesState{entries: []roleEntry{{gate: gate, ticket: ticket, chain: []edge{{fd: fd}}, target: "/one"}}}}
	if gate.roleRefs != 1 || gate.fds != 1 {
		t.Fatal("role observer reservation missing")
	}
	for range 2 {
		if err := roles.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if gate.roleRefs != 0 || gate.fds != 0 || gate.workspace != treeFixedWorkspace {
		t.Fatal("role close did not release exactly once")
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
		t.Fatal("role close leaked descriptor")
	}
}

// Real current-thread proc/namespace I/O, not a positive role or host proof.
func TestTreeTopologyKernelReaderAndFinalObservations(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	probe, err := openTopologyMetadata(nil)
	if err != nil {
		t.Skip("current kernel/proc metadata capability unavailable")
	}
	checkTreeTopologyParser(t, probe.snapshot)
	if err := probe.close(); err != nil {
		t.Fatal(err)
	}
	gate := testTreeRoleGate()
	op, err := gate.beginTopology(false, 32<<10, treeTopologyBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer op.finish(false)
	top := &treeTopology{ops: []*stageOperation{op}, raw: make([]byte, topologyMaxBytes), fresh: make([]byte, topologyMaxBytes)}
	top.meta, err = openTopologyMetadataWith(nil, 0, treeMaxFDs, func(meta *topology) ([]byte, error) { return top.readMountinfo(meta, top.raw) })
	if err != nil {
		t.Fatal("reserved topology metadata failed", err)
	}
	defer func() {
		if err := top.close(); err != nil {
			t.Error(err)
		}
	}()
	if err := top.finish(); err != nil {
		t.Fatal("stable reserved topology metadata failed", err)
	}
	// Retained custom paths are checked independently of meta.borrowed.
	_, parent := localFixture(t)
	top.paths[0] = topologyPath{chain: []edge{parent}, owner: parent.id.Owner}
	top.pathCount = 1
	if err := top.finish(); err != nil {
		t.Fatal("stable borrowed custom path failed", err)
	}
	top.paths[0].chain[0].id.Inode++
	if top.finish() == nil {
		t.Fatal("changed borrowed custom edge escaped final validation")
	}
	top.pathCount = 0
	top.meta.snapshot[0] ^= 1
	if top.finish() == nil {
		t.Fatal("changed raw metadata snapshot escaped final validation")
	}
	if gate.reads == 0 {
		t.Fatal("topology metadata reads escaped lifetime charge")
	}
}

func TestTreeTopologyKernelConstructor(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	probe, err := openTopologyMetadata(nil)
	if err != nil {
		t.Skip("current kernel/proc metadata capability unavailable")
	}
	table, parseErr := parseTopology(probe.snapshot)
	associated := parseErr == nil && table.rows[table.root].id == probe.chain[0].id.MountID
	if err := probe.close(); err != nil {
		t.Fatal(err)
	}
	if parseErr != nil {
		t.Skip("current mount metadata fails the ordinary topology parser grammar")
	}
	if !associated {
		t.Skip("ordinary parsed mountinfo root ID differs from the pinned absolute root statx mount ID")
	}
	gate := testTreeRoleGate()
	op, err := gate.beginTopology(false, 32<<10, treeTopologyBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer op.finish(false)
	top, err := openTreeTopology(nil, 0, []*stageOperation{op})
	if err != nil {
		t.Fatal("admitted reserved topology constructor refused", err)
	}
	if top.finish() != nil || top.close() != nil {
		t.Fatal("admitted reserved topology final observation failed")
	}
}

func TestTreeRolesCloseFailurePoisonsStageWithoutRetry(t *testing.T) {
	_, parent := localFixture(t)
	fd, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	gate := testTreeRoleGate()
	op, err := gate.beginRole(1, 0, 4)
	if err != nil {
		t.Fatal(err)
	}
	ticket := op.retainRole(1, 4)
	op.finish(false)
	roles := &Roles{state: &rolesState{entries: []roleEntry{{gate: gate, ticket: ticket, chain: []edge{{fd: fd}}, target: "/one"}}}}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if roles.Close() == nil || gate.phase != treePreserve || gate.roleRefs != 0 || gate.fds != 0 {
		t.Fatal("role close failure did not revoke and release")
	}
	other, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, fd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(other); err != nil {
			t.Error(err)
		}
	}()
	if roles.Close() != nil {
		t.Fatal("consumed role close retried")
	}
	if _, err := unix.FcntlInt(uintptr(other), unix.F_GETFD, 0); err != nil {
		t.Fatal("role close retried a released descriptor number")
	}
}
