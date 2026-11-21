//go:build linux

package safefs

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/unix"
	"runtime"
	"strings"
	"testing"
)

const topologyTestRoot = "1 1 0:1 / / rw - tmpfs tmpfs rw\n"

func TestTopologyParser(t *testing.T) {
	tests := []struct {
		name, input string
		valid       bool
	}{
		{"self root", topologyTestRoot, true},
		{"raw CR root", "1 1 0:1 /a\rb / rw - tmpfs none rw\n", true},
		{"raw CR mountpoint", topologyTestRoot + "2 1 0:2 / /a\rb rw - tmpfs none rw\n", true},
		{"Linux byte string", "1 1 0:1 /a\xffb / rw - tmpfs none rw\n", true},
		{"unavailable root parent", "1 999 0:1 / / rw - tmpfs tmpfs rw\n", true},
		{"btrfs subtree and idmap", "1 1 0:42 /@home / rw,idmapped shared:1 master:2 propagate_from:3 - btrfs none rw\n", true},
		{"escaped bytes", "1 1 0:1 /a\\040b\\011c\\012d\\134e / rw - tmpfs none rw\n", true},
		{"unrelated stack", topologyTestRoot + "2 1 0:2 / /other rw - tmpfs none rw\n3 2 0:3 / /other rw - tmpfs none rw\n", true},
		{"unknown unrelated filesystem", topologyTestRoot + "2 1 0:2 / /other rw - overlay none rw\n", true},
		{"missing newline", strings.TrimSuffix(topologyTestRoot, "\n"), false},
		{"duplicate root", topologyTestRoot + "2 1 0:2 / / rw - tmpfs none rw\n", false},
		{"duplicate id", topologyTestRoot + "1 1 0:2 / /a rw - tmpfs none rw\n", false},
		{"disconnected", topologyTestRoot + "2 9 0:2 / /a rw - tmpfs none rw\n", false},
		{"cycle", topologyTestRoot + "2 3 0:2 / /a rw - tmpfs none rw\n3 2 0:3 / /a rw - tmpfs none rw\n", false},
		{"root hidden cycle", "1 2 0:1 / / rw - tmpfs none rw\n2 1 0:2 / /a rw - tmpfs none rw\n", false},
		{"noncontaining parent", topologyTestRoot + "2 1 0:2 / /a rw - tmpfs none rw\n3 2 0:3 / /b rw - tmpfs none rw\n", false},
		{"unknown optional", strings.Replace(topologyTestRoot, "rw -", "rw idmapped -", 1), false},
		{"duplicate optional", strings.Replace(topologyTestRoot, "rw -", "rw shared:1 shared:2 -", 1), false},
		{"zero optional", strings.Replace(topologyTestRoot, "rw -", "rw shared:0 -", 1), false},
		{"numeric overflow", strings.Replace(topologyTestRoot, "1 1", "18446744073709551616 1", 1), false},
		{"major overflow", strings.Replace(topologyTestRoot, "0:1", "4294967296:1", 1), false},
		{"dot path", strings.Replace(topologyTestRoot, "/ /", "/a/../b /", 1), false},
		{"bad escape", strings.Replace(topologyTestRoot, "/ /", `/a\041 /`, 1), false},
		{"NUL", strings.Replace(topologyTestRoot, "rw -", "rw\x00 -", 1), false},
		{"raw tab", strings.Replace(topologyTestRoot, "rw -", "rw\t -", 1), false},
		{"extra separator", strings.Replace(topologyTestRoot, "rw -", "rw - -", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTopology([]byte(tt.input))
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}

func TestTopologyBounds(t *testing.T) {
	if decoded, ok := topologyDecode(`/\134040`); !ok || decoded != `/\040` {
		t.Fatal("kernel escapes were decoded more than once")
	}

	for _, tt := range []struct {
		name, path string
		ok         bool
	}{
		{"path exact", "/" + strings.Repeat("a", topologyMaxPath-1), true},
		{"path overflow", "/" + strings.Repeat("a", topologyMaxPath), false},
		{"components exact", strings.Repeat("/a", topologyMaxComponents), true},
		{"components overflow", strings.Repeat("/a", topologyMaxComponents+1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := topologyDecode(tt.path)
			if ok != tt.ok {
				t.Fatalf("ok=%v", ok)
			}
		})
	}
	base := "/" + strings.Repeat("a", topologyMaxPath-1)
	joined, ok := topologyJoin(base, "/"+strings.Repeat("b", topologyMaxPath-1))
	if !ok || len(joined) != 2*topologyMaxPath {
		t.Fatal("physical bound")
	}
	joined, ok = topologyJoin(joined, "/"+strings.Repeat("c", topologyMaxPath-1))
	if !ok || len(joined) != topologyMaxDerived {
		t.Fatal("inverse bound")
	}
	if _, ok := topologyJoin(joined, "/x"); ok {
		t.Fatal("derived overflow admitted")
	}
	record := func(id int) string { return fmt.Sprintf("%d 1 0:1 / /m%d rw - tmpfs none rw\n", id, id) }
	var records strings.Builder
	records.WriteString(topologyTestRoot)
	for i := 2; i <= topologyMaxRecords; i++ {
		records.WriteString(record(i))
	}
	if _, err := parseTopology([]byte(records.String())); err != nil {
		t.Fatal("record boundary", err)
	}
	records.WriteString(record(topologyMaxRecords + 1))
	if _, err := parseTopology([]byte(records.String())); err == nil {
		t.Fatal("record overflow")
	}
	linePrefix := "1 1 0:1 / / rw - tmpfs none "
	line := linePrefix + strings.Repeat("x", topologyMaxLine-len(linePrefix)-1) + "\n"
	if _, err := parseTopology([]byte(line)); err != nil {
		t.Fatal("line boundary", err)
	}
	if _, err := parseTopology([]byte(line[:len(line)-1] + "x\n")); err == nil {
		t.Fatal("line overflow")
	}
	if _, err := parseTopology(make([]byte, topologyMaxBytes+1)); err == nil {
		t.Fatal("byte overflow")
	}
}

func TestTopologyRelevantClosure(t *testing.T) {
	table, err := parseTopology([]byte(topologyTestRoot + "2 1 0:2 / /other rw - tmpfs none rw\n3 2 0:3 / /other rw - overlay none rw\n"))
	if err != nil {
		t.Fatal(err)
	}
	top := &topology{table: table}
	if _, err := top.relevant([]topologyPath{{target: "/safe"}}); err != nil {
		t.Fatal("unrelated stack refused", err)
	}
	for _, target := range []string{"/other", "/other/future", "/"} {
		if _, err := top.relevant([]topologyPath{{target: target}}); err == nil {
			t.Fatal("relevant stack admitted")
		}
	}
	table, err = parseTopology([]byte(topologyTestRoot + "2 1 0:2 / /other rw - overlay none rw\n"))
	if err != nil {
		t.Fatal(err)
	}
	top.table = table
	if _, err := top.relevant([]topologyPath{{target: "/other/future"}}); err == nil {
		t.Fatal("unknown coordinate semantics admitted")
	}
}

func TestTopologyConeMatrix(t *testing.T) {
	cone := func(dev uint64, physical, namespace string) topologyFootprint {
		return topologyFootprint{[]topologyCone{{dev, physical, namespace}}}
	}
	tests := []struct {
		name       string
		a, b       topologyFootprint
		same, want bool
	}{
		{"natural containment", cone(1, "/a", "/a"), cone(1, "/a/b", "/a/b"), true, true},
		{"forbidden containment", cone(1, "/a", "/a"), cone(1, "/a/b", "/a/b"), false, false},
		{"sibling boundaries", cone(1, "/a", "/a"), cone(1, "/ab", "/ab"), false, true},
		{"sibling bind", cone(1, "/source", "/a"), cone(1, "/source", "/b"), false, false},
		{"missing projected bind", cone(1, "/source/future", "/a/future"), cone(1, "/source/future", "/b/future"), false, false},
		{"parent extra alias", cone(1, "/source", "/a"), cone(1, "/source/child", "/a/other"), true, false},
		{"independent tmpfs", cone(1, "/", "/a"), cone(2, "/", "/b"), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := topologyCompatible(tt.a, tt.b, tt.same); got != tt.want {
				t.Fatalf("compatible=%v", got)
			}
		})
	}
	natural := topologyFootprint{[]topologyCone{{1, "/source", "/a"}, {2, "/", "/a/private"}, {3, "/", "/a/private/archive"}}}
	if !topologyCompatible(natural, natural, true) {
		t.Fatal("nested independent tmpfs")
	}
	alias := topologyFootprint{[]topologyCone{{1, "/source", "/a"}, {1, "/source/child", "/a/other"}}}
	if topologyCompatible(alias, alias, true) {
		t.Fatal("interior self alias")
	}
	cross := topologyFootprint{[]topologyCone{{2, "/", "/b"}, {1, "/source/child", "/b/interior"}}}
	if topologyCompatible(natural, cross, false) {
		t.Fatal("interior cross domain alias")
	}
}

// This exercises metadata observation only; it is not a public Roles positive
// or Btrfs/bind/namespace/host qualification.
func TestTopologyKernelMetadataReader(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	top, err := openTopologyMetadata(nil)
	if err != nil {
		t.Skip("current kernel/proc metadata capability unavailable")
	}
	defer func() {
		if err := top.close(); err != nil {
			t.Error(err)
		}
	}()
	pin, err := top.retainNamespace()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pin.close(); err != nil {
			t.Error(err)
		}
	}()
	if err := top.finish(); err != nil {
		t.Fatal("stable metadata", err)
	}
	for _, field := range []*Identity{&top.chain[0].id, &top.chain[1].id, &top.chain[4].id, &top.nsLink.id, &top.namespace.id} {
		saved := *field
		field.Inode++
		if top.validate() == nil {
			t.Fatal("substituted retained kernel evidence admitted")
		}
		*field = saved
	}
	wrong := *pin
	wrong.id.Inode++
	if other, err := openTopologyMetadata(&wrong); err == nil {
		_ = other.close()
		t.Fatal("mismatched namespace admitted")
	}

	next, err := openTopologyMetadata(pin)
	if err != nil {
		t.Fatal("retained namespace", err)
	}
	if err := next.close(); err != nil {
		t.Fatal(err)
	}
	top.snapshot = append([]byte(nil), top.snapshot...)
	top.snapshot[0] ^= 1
	if top.finish() == nil {
		t.Fatal("changed snapshot admitted")
	}
	top.tid = "0"
	if top.validate() == nil {
		t.Fatal("thread mismatch admitted")
	}
}

func TestTopologyNamespaceAndCleanupRefusals(t *testing.T) {
	for _, s := range []string{"mnt:[0]", "mnt:[]", "mnt:[-1]", "mnt:[18446744073709551616]", "mnt:[1]x", "pid:[1]"} {
		if _, ok := namespaceNumber(s); ok {
			t.Fatalf("invalid namespace link admitted: %q", s)
		}
	}
	n, ok := namespaceNumber("mnt:[4026531840]")
	if !ok || n != 4026531840 {
		t.Fatal("namespace decimal inode")
	}
	pin := &namespacePin{fd: -1}
	if pin.validate() == nil {
		t.Fatal("closed namespace admitted")
	}
	for i := 0; i < 2; i++ {
		if pin.close() != nil {
			t.Fatal("idempotent namespace cleanup")
		}
	}
	top := &topology{self: edge{fd: -1}, nsLink: edge{fd: -1}, frontendFDs: topologyMaxFDs}
	if top.room(1) {
		t.Fatal("descriptor cap")
	}
	for i := 0; i < 2; i++ {
		if top.close() != nil {
			t.Fatal("idempotent reader cleanup")
		}
	}
	broken := &namespacePin{fd: 1 << 30}
	if broken.close() == nil || broken.fd != -1 || broken.close() != nil {
		t.Fatal("close failure was not cleared")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if top, err := openTopology(pin); err == nil {
		_ = top.close()
		t.Fatal("closed expected namespace admitted")
	}
	// No mounts, setns, unshare or privilege operations are used by these checks.
}

func FuzzTopologyParser(f *testing.F) {
	f.Add([]byte(topologyTestRoot))
	f.Add([]byte("1 1 0:1 / / rw shared:1 - btrfs none rw\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		table, err := parseTopology(data)
		if err == nil && (table.root < 0 || len(table.rows) > topologyMaxRecords || table.rows[table.root].point != "/") {
			t.Fatal("invalid accepted graph")
		}
	})
}

func TestTopologyCoordinateIdentity(t *testing.T) {
	top := &topology{coordinates: make(map[topologyCoordinate]topologyObject), objects: make(map[topologyObject]topologyCoordinate)}
	coordinate := topologyCoordinate{device: 42, physical: "/@home/role"}
	// Btrfs statx Device is an anonymous subvolume device, not row s_dev.
	id := Identity{Device: 900, Inode: 256, Mode: unix.S_IFDIR | 0700}
	if !top.recordCoordinate(coordinate, id) {
		t.Fatal("distinct statx/superblock devices refused")
	}
	mapped := id
	mapped.Owner = 1234
	mapped.MountID = 999
	if !top.recordCoordinate(coordinate, mapped) {
		t.Fatal("UID presentation or bind mount ID altered alias identity")
	}
	other := id
	other.Inode++
	if top.recordCoordinate(coordinate, other) {
		t.Fatal("coordinate identity contradiction admitted")
	}
	if top.recordCoordinate(topologyCoordinate{42, "/other"}, id) {
		t.Fatal("one object has contradictory coordinates")
	}
	if top.recordCoordinate(topologyCoordinate{43, coordinate.physical}, id) {
		t.Fatal("one object has contradictory physical domain")
	}
}

func TestTopologyDescriptorAssociation(t *testing.T) {
	row := topologyMount{id: 2, parent: 1, device: 42, root: "/@home", point: "/home", kind: "btrfs"}
	id := Identity{MountID: 2, Device: 900, Inode: 256}
	tests := []struct {
		name     string
		row      topologyMount
		id       Identity
		previous uint64
		current  string
		fs       int64
		want     bool
	}{
		{"transition", row, id, 1, "/home", unix.BTRFS_SUPER_MAGIC, true},
		{"same mount suffix", row, id, 2, "/home/role", unix.BTRFS_SUPER_MAGIC, true},
		{"wrong mount ID", row, Identity{MountID: 3}, 1, "/home", unix.BTRFS_SUPER_MAGIC, false},
		{"wrong parent", row, id, 9, "/home", unix.BTRFS_SUPER_MAGIC, false},
		{"wrong transition edge", row, id, 1, "/home/role", unix.BTRFS_SUPER_MAGIC, false},
		{"wrong namespace path", row, id, 2, "/elsewhere", unix.BTRFS_SUPER_MAGIC, false},
		{"wrong filesystem", row, id, 1, "/home", unix.TMPFS_MAGIC, false},
		{"unknown semantics", topologyMount{id: 2, parent: 1, device: 42, root: "/", point: "/home", kind: "overlay"}, id, 1, "/home", unix.OVERLAYFS_SUPER_MAGIC, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coordinate, ok := topologyAssociate(tt.row, tt.id, tt.previous, tt.current, tt.fs)
			if ok != tt.want {
				t.Fatalf("association=%v", ok)
			}
			if ok && (coordinate.device != 42 || !strings.HasPrefix(coordinate.physical, "/@home")) {
				t.Fatal("superblock coordinate")
			}
		})
	}
	root := topologyMount{id: 1, parent: 1, device: 1, root: "/@root", point: "/", kind: "btrfs"}
	if _, ok := topologyAssociate(root, Identity{MountID: 1, Device: 123}, 0, "/", unix.BTRFS_SUPER_MAGIC); !ok {
		t.Fatal("subvolume root")
	}
	if _, ok := topologyAssociate(root, Identity{MountID: 1}, 0, "/child", unix.BTRFS_SUPER_MAGIC); ok {
		t.Fatal("wrong root edge")
	}
}

func TestTopologyDescriptorValidationHeadroom(t *testing.T) {
	// Pure accounting only: no descriptors are acquired near the limit.
	top := &topology{chain: []edge{{fd: -1}}, self: edge{fd: -1}, nsLink: edge{fd: -1}, frontendFDs: topologyMaxFDs - 3}
	if !top.room(2) || top.room(3) {
		t.Fatal("two validation slots must remain")
	}
	if _, err := top.pinMount("/interior", 0); err == nil || len(top.pins) != 0 {
		t.Fatal("mount pin consumed validation headroom")
	}
	if _, err := top.retainNamespace(); err == nil {
		t.Fatal("namespace duplicate consumed validation headroom")
	}
	top.frontendFDs--
	if !top.room(3) {
		t.Fatal("one acquisition plus two validation slots should fit")
	}
}

func TestTopologyFinalInteriorObservation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	top, err := openTopologyMetadata(nil)
	if err != nil {
		t.Skip("current kernel/proc metadata capability unavailable")
	}
	defer func() {
		if err := top.close(); err != nil {
			t.Error(err)
		}
	}()
	// This isolated pin tests final metadata revalidation, not mount qualification.
	fd, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	top.pins = []topologyPath{{chain: []edge{{fd: fd}}, owner: uint32(unix.Getuid())}}
	id, err := observeFD(fd)
	if err != nil {
		t.Fatal(err)
	}
	top.pins[0].chain[0].id = id
	if top.validatePaths() != nil {
		t.Fatal("initial pin evidence")
	}
	fresh, err := top.readMountinfo()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(top.snapshot, fresh) {
		t.Skip("mount metadata changed during observation")
	}
	originalMode := id.Mode & 0o7777
	defer func() {
		if err := unix.Fchmod(fd, originalMode); err != nil {
			t.Error("interior mode restoration failed")
		}
	}()
	if err := unix.Fchmod(fd, originalMode^0o040); err != nil {
		t.Fatal(err)
	}
	if top.finishObservation(fresh) == nil {
		t.Fatal("unchanged mountinfo hid final interior metadata drift")
	}
}
