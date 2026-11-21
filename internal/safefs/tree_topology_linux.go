//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const treeTopologyBytes = 3*(8<<20) + 256*8192 + 16384*72 + 16384*16 + (1 << 20)

type treeSpan struct{ start, length uint32 }
type treeMount struct {
	id, parent, device uint64
	root, point, kind  treeSpan
}
type treeMountID struct {
	id    uint64
	index uint32
}
type treeCoordinate struct {
	device, objectDevice, inode uint64
	kind                        uint32
	physical                    treeSpan
}
type treeCone struct{ coordinate, path, next, flags uint32 }
type treeFootprint struct{ start, count uint32 }
type treeMountPin struct {
	point treeSpan
	chain []edge
	owner uint32
}

// Every variable capacity is reserved before opening metadata or copying paths.
// Rows and strings are owned flat storage; cones refer to stable path spans.
type treeTopology struct {
	meta                          *topology
	ops                           []*stageOperation
	raw, fresh, decoded, physical []byte
	rows                          []treeMount
	ids                           []treeMountID
	root                          int
	colors                        [topologyMaxRecords]byte
	trail                         [topologyMaxRecords]uint32
	relevant                      [topologyMaxRecords]byte
	points                        [topologyMaxRecords]uint32
	coordinates                   [treeMaxFDs]treeCoordinate
	coordinateCount               int
	cones                         [topologyMaxCones]treeCone
	coneCount                     int
	paths                         [2 * topologyMaxPaths]topologyPath
	pathCount                     int
	pins                          [treeMaxFDs]treeMountPin
	pinCount, pinFDs              int
	frontend                      int
	work                          [3][topologyMaxDerived + 1]byte
}

func openTreeTopology(expected *namespacePin, frontend int, ops []*stageOperation) (_ *treeTopology, err error) {
	if frontend < 0 || frontend > treeMaxFDs-11 {
		return nil, errDirectory
	}
	t := &treeTopology{ops: ops, root: -1, frontend: frontend}
	t.raw = make([]byte, topologyMaxBytes)
	t.fresh = make([]byte, topologyMaxBytes)
	t.decoded = make([]byte, 0, topologyMaxBytes)
	t.physical = make([]byte, 0, treeMaxFDs*8192)
	t.rows = make([]treeMount, 0, topologyMaxRecords)
	t.ids = make([]treeMountID, 0, topologyMaxRecords)
	defer func() {
		if err != nil {
			err = errors.Join(err, t.close())
		}
	}()
	t.meta, err = openTopologyMetadataWith(expected, frontend, treeMaxFDs, func(m *topology) ([]byte, error) { return t.readMountinfo(m, t.raw) })
	if err != nil {
		return nil, errDirectory
	}
	if err = t.parse(t.meta.snapshot); err != nil {
		return nil, errDirectory
	}
	if t.rows[t.root].id != t.meta.chain[0].id.MountID {
		return nil, errDirectory
	}
	return t, nil
}

func (t *treeTopology) charge(n uint64) error {
	for _, op := range t.ops {
		if err := op.stage.readCharge(n); err != nil {
			return errDirectory
		}
	}
	return nil
}

func (t *treeTopology) readMountinfo(m *topology, storage []byte) (_ []byte, err error) {
	if !m.room(1) {
		return nil, errDirectory
	}
	parent := m.chain[4]
	fd, e := unix.Openat2(parent.fd, "mountinfo", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC, Resolve: procResolve})
	if e != nil {
		return nil, errDirectory
	}
	defer func() { err = errors.Join(err, closeChain([]edge{{fd: fd}})) }()
	before, e := observeFD(fd)
	linked, e2 := observeAt(parent.fd, "mountinfo")
	if e != nil || e2 != nil || before != linked || before.Mode&unix.S_IFMT != unix.S_IFREG || !sameMount(before, parent.id) || !procFilesystem(fd) {
		return nil, errDirectory
	}
	used := 0
	for used < len(storage) {
		want := min(32<<10, len(storage)-used)
		if t.charge(uint64(want)) != nil {
			return nil, errDirectory
		}
		n, e := unix.Read(fd, storage[used:used+want])
		if e != nil || n < 0 || n > want {
			return nil, errDirectory
		}
		if n == 0 {
			break
		}
		used += n
	}
	if used == len(storage) {
		var probe [1]byte
		if t.charge(1) != nil {
			return nil, errDirectory
		}
		n, e := unix.Read(fd, probe[:])
		if e != nil || n != 0 {
			return nil, errDirectory
		}
	}
	after, e := observeFD(fd)
	linked, e2 = observeAt(parent.fd, "mountinfo")
	if e != nil || e2 != nil || after != before || linked != before {
		return nil, errDirectory
	}
	return storage[:used], nil
}

func treeNumber(b []byte) (uint64, bool) {
	if len(b) == 0 {
		return 0, false
	}
	var n uint64
	for _, c := range b {
		if c < '0' || c > '9' || n > (^uint64(0)-uint64(c-'0'))/10 {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	return n, true
}
func treeAbsolute(b []byte, limit int, components bool) bool {
	if len(b) == 0 || len(b) > limit || b[0] != '/' || bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	if len(b) == 1 {
		return true
	}
	parts := 0
	start := 1
	for i := 1; i <= len(b); i++ {
		if i != len(b) && b[i] != '/' {
			continue
		}
		part := b[start:i]
		if len(part) == 0 || bytes.Equal(part, []byte(".")) || bytes.Equal(part, []byte("..")) {
			return false
		}
		parts++
		start = i + 1
	}
	return !components || parts <= topologyMaxComponents
}
func treeContains(parent, child []byte) bool {
	return bytes.Equal(parent, child) || bytes.Equal(parent, []byte("/")) && len(child) > 0 && child[0] == '/' || len(child) > len(parent) && bytes.Equal(parent, child[:len(parent)]) && child[len(parent)] == '/'
}
func (t *treeTopology) data(s treeSpan) []byte { return t.decoded[s.start : s.start+s.length] }
func (t *treeTopology) decode(raw []byte) (treeSpan, bool) {
	if len(raw) > 4*topologyMaxPath {
		return treeSpan{}, false
	}
	start := len(t.decoded)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\\' {
			if i+4 > len(raw) {
				return treeSpan{}, false
			}
			switch string(raw[i : i+4]) {
			case `\040`:
				c = ' '
			case `\011`:
				c = '\t'
			case `\012`:
				c = '\n'
			case `\134`:
				c = '\\'
			default:
				return treeSpan{}, false
			}
			i += 3
		}
		if len(t.decoded)-start == topologyMaxPath || c == 0 || len(t.decoded) == cap(t.decoded) {
			return treeSpan{}, false
		}
		t.decoded = append(t.decoded, c)
	}
	span := treeSpan{uint32(start), uint32(len(t.decoded) - start)}
	return span, treeAbsolute(t.data(span), topologyMaxPath, true)
}
func (t *treeTopology) parseRow(line []byte) (treeMount, error) {
	var fields [74][]byte
	count, start := 0, 0
	for i := 0; i <= len(line); i++ {
		if i != len(line) && line[i] != ' ' {
			if line[i] == '\t' || line[i] == 0 {
				return treeMount{}, errDirectory
			}
			continue
		}
		if count == len(fields) || i == start {
			return treeMount{}, errDirectory
		}
		fields[count] = line[start:i]
		count++
		start = i + 1
	}
	if count < 10 || count > 74 {
		return treeMount{}, errDirectory
	}
	sep := count - 4
	if !bytes.Equal(fields[sep], []byte("-")) || sep < 6 || sep-6 > 64 {
		return treeMount{}, errDirectory
	}
	id, ok := treeNumber(fields[0])
	if !ok || id == 0 {
		return treeMount{}, errDirectory
	}
	parent, ok := treeNumber(fields[1])
	if !ok || parent == 0 {
		return treeMount{}, errDirectory
	}
	colon := bytes.IndexByte(fields[2], ':')
	if colon < 0 || bytes.IndexByte(fields[2][colon+1:], ':') >= 0 {
		return treeMount{}, errDirectory
	}
	major, ok := treeNumber(fields[2][:colon])
	if !ok || major > uint64(^uint32(0)) {
		return treeMount{}, errDirectory
	}
	minor, ok := treeNumber(fields[2][colon+1:])
	if !ok || minor > uint64(^uint32(0)) {
		return treeMount{}, errDirectory
	}
	root, ok := t.decode(fields[3])
	if !ok {
		return treeMount{}, errDirectory
	}
	point, ok := t.decode(fields[4])
	if !ok {
		return treeMount{}, errDirectory
	}
	seen := uint8(0)
	for _, field := range fields[6:sep] {
		i := bytes.IndexByte(field, ':')
		tag := field
		var value []byte
		if i >= 0 {
			tag, value = field[:i], field[i+1:]
		}
		var bit uint8
		switch string(tag) {
		case "unbindable":
			bit = 1
			if i >= 0 {
				return treeMount{}, errDirectory
			}
		case "shared":
			bit = 2
		case "master":
			bit = 4
		case "propagate_from":
			bit = 8
		default:
			return treeMount{}, errDirectory
		}
		if seen&bit != 0 {
			return treeMount{}, errDirectory
		}
		seen |= bit
		if bit != 1 {
			n, ok := treeNumber(value)
			if i < 0 || !ok || n == 0 {
				return treeMount{}, errDirectory
			}
		}
	}
	if len(fields[sep+1]) > cap(t.decoded)-len(t.decoded) {
		return treeMount{}, errDirectory
	}
	kind := treeSpan{uint32(len(t.decoded)), uint32(len(fields[sep+1]))}
	t.decoded = append(t.decoded, fields[sep+1]...)
	return treeMount{id, parent, unix.Mkdev(uint32(major), uint32(minor)), root, point, kind}, nil
}
func (t *treeTopology) find(id uint64) (int, bool) {
	i := sort.Search(len(t.ids), func(i int) bool { return t.ids[i].id >= id })
	if i == len(t.ids) || t.ids[i].id != id {
		return 0, false
	}
	return int(t.ids[i].index), true
}
func (t *treeTopology) parse(raw []byte) error {
	if len(raw) == 0 || len(raw) > topologyMaxBytes || raw[len(raw)-1] != '\n' || bytes.IndexByte(raw, 0) >= 0 {
		return errDirectory
	}
	for len(raw) > 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 || i+1 > topologyMaxLine || len(t.rows) == topologyMaxRecords {
			return errDirectory
		}
		row, err := t.parseRow(raw[:i])
		if err != nil {
			return errDirectory
		}
		raw = raw[i+1:]
		if bytes.Equal(t.data(row.point), []byte("/")) {
			if t.root >= 0 {
				return errDirectory
			}
			t.root = len(t.rows)
		}
		t.ids = append(t.ids, treeMountID{row.id, uint32(len(t.rows))})
		t.rows = append(t.rows, row)
	}
	if t.root < 0 {
		return errDirectory
	}
	sort.Slice(t.ids, func(i, j int) bool { return t.ids[i].id < t.ids[j].id })
	for i := 1; i < len(t.ids); i++ {
		if t.ids[i-1].id == t.ids[i].id {
			return errDirectory
		}
	}
	root := t.rows[t.root]
	if _, ok := t.find(root.parent); ok && root.parent != root.id {
		return errDirectory
	}
	t.colors[t.root] = 2
	for i := range t.rows {
		if t.colors[i] == 2 {
			continue
		}
		j, n := i, 0
		for t.colors[j] != 2 {
			if t.colors[j] == 1 {
				return errDirectory
			}
			t.colors[j] = 1
			t.trail[n] = uint32(j)
			n++
			row := t.rows[j]
			parent, ok := t.find(row.parent)
			if !ok || parent == j || !treeContains(t.data(t.rows[parent].point), t.data(row.point)) {
				return errDirectory
			}
			j = parent
		}
		for _, index := range t.trail[:n] {
			t.colors[index] = 2
		}
	}
	return nil
}

func (t *treeTopology) relevantRows(paths []topologyPath) error {
	clear(t.relevant[:])
	n := 0
	for i, row := range t.rows {
		for _, p := range paths {
			point := t.data(row.point)
			if treeContainsString(point, p.target) || treeStringContains(p.target, point) {
				t.relevant[i] = 1
				break
			}
		}
	}
	for i := range t.rows {
		if t.relevant[i] == 0 {
			continue
		}
		for j := i; j != t.root; {
			j, _ = t.find(t.rows[j].parent)
			t.relevant[j] = 1
		}
	}
	for i, row := range t.rows {
		if t.relevant[i] == 0 {
			continue
		}
		kind := t.data(row.kind)
		if !bytes.Equal(kind, []byte("btrfs")) && !bytes.Equal(kind, []byte("tmpfs")) {
			return errDirectory
		}
		t.points[n] = uint32(i)
		n++
	}
	sort.Slice(t.points[:n], func(i, j int) bool {
		return bytes.Compare(t.data(t.rows[t.points[i]].point), t.data(t.rows[t.points[j]].point)) < 0
	})
	for i := 1; i < n; i++ {
		if bytes.Equal(t.data(t.rows[t.points[i-1]].point), t.data(t.rows[t.points[i]].point)) {
			return errDirectory
		}
	}
	return nil
}
func treeContainsString(parent []byte, child string) bool {
	if len(parent) == 1 && parent[0] == '/' {
		return strings.HasPrefix(child, "/")
	}
	return len(child) >= len(parent) && string(parent) == child[:len(parent)] && (len(child) == len(parent) || child[len(parent)] == '/')
}
func treeStringContains(parent string, child []byte) bool {
	if parent == "/" {
		return len(child) > 0 && child[0] == '/'
	}
	return len(child) >= len(parent) && parent == string(child[:len(parent)]) && (len(child) == len(parent) || child[len(parent)] == '/')
}

func appendTreeComponent(dst []byte, part string, limit int) ([]byte, bool) {
	if !validName(part) {
		return dst, false
	}
	separator := 1
	if len(dst) == 1 && dst[0] == '/' {
		separator = 0
	}
	if len(dst)+separator+len(part) > limit {
		return dst, false
	}
	if separator != 0 {
		dst = append(dst, '/')
	}
	return append(dst, part...), true
}
func (t *treeTopology) record(device uint64, physical []byte, id Identity) (uint32, error) {
	for i, c := range t.coordinates[:t.coordinateCount] {
		same := c.device == device && bytes.Equal(t.physical[c.physical.start:c.physical.start+c.physical.length], physical)
		object := c.objectDevice == id.Device && c.inode == id.Inode && c.kind == id.Mode&unix.S_IFMT
		if same != object {
			return 0, errDirectory
		}
		if same {
			return uint32(i), nil
		}
	}
	if t.coordinateCount == len(t.coordinates) || len(physical) > 8192 || len(physical) > cap(t.physical)-len(t.physical) {
		return 0, errDirectory
	}
	i := t.coordinateCount
	t.coordinateCount++
	span := treeSpan{uint32(len(t.physical)), uint32(len(physical))}
	t.physical = append(t.physical, physical...)
	t.coordinates[i] = treeCoordinate{device, id.Device, id.Inode, id.Mode & unix.S_IFMT, span}
	return uint32(i), nil
}

func (t *treeTopology) correlate(chain []edge, missing []string, owner uint32, target []byte) (uint32, error) {
	if len(chain) == 0 || len(chain) > 257 || !treeAbsolute(target, topologyMaxPath, true) || !sameDirectory(chain[0].id, t.meta.chain[0].id) || validateEdges(chain, owner) != nil {
		return 0, errDirectory
	}
	current := t.work[0][:1]
	current[0] = '/'
	var last uint32
	for i, e := range chain {
		if i > 0 {
			var ok bool
			current, ok = appendTreeComponent(current, e.name, topologyMaxPath)
			if !ok {
				return 0, errDirectory
			}
		}
		index, ok := t.find(e.id.MountID)
		if !ok {
			return 0, errDirectory
		}
		row := t.rows[index]
		point := t.data(row.point)
		if !treeContains(point, current) || i == 0 && (index != t.root || !bytes.Equal(current, []byte("/"))) {
			return 0, errDirectory
		}
		if i > 0 && chain[i-1].id.MountID != e.id.MountID && (!bytes.Equal(current, point) || row.parent != chain[i-1].id.MountID) {
			return 0, errDirectory
		}
		var fs unix.Statfs_t
		if unix.Fstatfs(e.fd, &fs) != nil {
			return 0, errDirectory
		}
		kind := t.data(row.kind)
		if bytes.Equal(kind, []byte("btrfs")) {
			if fs.Type != unix.BTRFS_SUPER_MAGIC {
				return 0, errDirectory
			}
		} else if !bytes.Equal(kind, []byte("tmpfs")) || fs.Type != unix.TMPFS_MAGIC {
			return 0, errDirectory
		}
		suffix := current[len(point):]
		if len(point) == 1 {
			suffix = current
		}
		if bytes.Equal(point, current) {
			suffix = nil
		}
		base := t.data(row.root)
		if bytes.Equal(base, []byte("/")) && len(suffix) > 0 {
			base = nil
		}
		physical := t.work[1][:0]
		physical = append(physical, base...)
		physical = append(physical, suffix...)
		if !treeAbsolute(physical, topologyMaxDerived, false) {
			return 0, errDirectory
		}
		coordinate, err := t.record(row.device, physical, e.id)
		if err != nil {
			return 0, errDirectory
		}
		last = coordinate
	}
	for _, part := range missing {
		var ok bool
		current, ok = appendTreeComponent(current, part, topologyMaxPath)
		if !ok {
			return 0, errDirectory
		}
	}
	if !bytes.Equal(current, target) {
		return 0, errDirectory
	}
	return last, nil
}

func (t *treeTopology) pinMount(point treeSpan, owner uint32) (int, error) {
	for i, p := range t.pins[:t.pinCount] {
		if p.owner == owner && bytes.Equal(t.data(p.point), t.data(point)) {
			return i, nil
		}
	}
	name := t.data(point)
	count := bytes.Count(name, []byte("/")) + 1
	if len(name) == 1 {
		count = 1
	}
	if t.pinCount == len(t.pins) || !t.meta.room(count+2) {
		return 0, errDirectory
	}
	chain := make([]edge, 0, count)
	fd, err := unix.FcntlInt(uintptr(t.meta.chain[0].fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return 0, errDirectory
	}
	index := t.pinCount
	t.pinCount++
	t.pins[index] = treeMountPin{point, append(chain, edge{fd: fd, id: t.meta.chain[0].id}), owner}
	t.pinFDs++
	t.meta.frontendFDs = t.frontend + t.pinFDs
	owned, err := advanceOwner(t.meta.chain[0].id, owner, false)
	if err != nil {
		return 0, errDirectory
	}
	for start := 1; start < len(name); {
		end := start
		for end < len(name) && name[end] != '/' {
			end++
		}
		part := string(name[start:end])
		start = end + 1
		p := &t.pins[index]
		parent := p.chain[len(p.chain)-1]
		expected, e := observeAt(parent.fd, part)
		if e != nil {
			return 0, errDirectory
		}
		owned, e = advanceOwner(expected, owner, owned)
		if e != nil {
			return 0, errDirectory
		}
		child, e := openObserved(parent.fd, part, expected)
		if e != nil {
			return 0, errDirectory
		}
		p.chain = append(p.chain, child)
		t.pinFDs++
		t.meta.frontendFDs = t.frontend + t.pinFDs
	}
	p := &t.pins[index]
	if p.chain[len(p.chain)-1].id.Owner != owner || validateEdges(p.chain, owner) != nil {
		return 0, errDirectory
	}
	return index, nil
}

func (t *treeTopology) footprints(paths []topologyPath) ([]treeFootprint, error) {
	if len(paths) == 0 || len(paths) > 64 || t.pathCount+len(paths) > len(t.paths) || t.meta.validate() != nil {
		return nil, errDirectory
	}
	if t.relevantRows(paths) != nil {
		return nil, errDirectory
	}
	var result [64]treeFootprint
	base := t.pathCount
	for i, p := range paths {
		if !rolePath(p.target) {
			return nil, errDirectory
		}
		t.paths[t.pathCount] = p
		t.pathCount++
		coordinate, err := t.correlate(p.chain, p.missing, p.owner, []byte(p.target))
		if err != nil {
			return nil, errDirectory
		}
		start := t.coneCount
		if t.addCone(treeCone{coordinate, uint32(base + i), 0, 0}) != nil {
			return nil, errDirectory
		}
		for j, row := range t.rows {
			point := t.data(row.point)
			if t.relevant[j] == 0 || !treeStringContains(p.target, point) || string(point) == p.target {
				continue
			}
			pin, err := t.pinMount(row.point, p.owner)
			if err != nil {
				return nil, errDirectory
			}
			chain := t.pins[pin].chain
			coordinate, err := t.correlate(chain, nil, p.owner, point)
			if err != nil || chain[len(chain)-1].id.MountID != row.id {
				return nil, errDirectory
			}
			if t.addCone(treeCone{coordinate, uint32(j), 0, 1}) != nil {
				return nil, errDirectory
			}
		}
		result[i] = treeFootprint{uint32(start), uint32(t.coneCount - start)}
		if !t.compatible(result[i], result[i], true) {
			return nil, errDirectory
		}
	}
	return result[:len(paths)], nil
}
func (t *treeTopology) addCone(c treeCone) error {
	if t.coneCount == len(t.cones) {
		return errDirectory
	}
	t.cones[t.coneCount] = c
	t.coneCount++
	return nil
}
func (t *treeTopology) conePhysical(c treeCone, dst []byte) ([]byte, bool) {
	coordinate := t.coordinates[c.coordinate]
	dst = append(dst, t.physical[coordinate.physical.start:coordinate.physical.start+coordinate.physical.length]...)
	if c.flags == 0 {
		for _, part := range t.paths[c.path].missing {
			var ok bool
			dst, ok = appendTreeComponent(dst, part, topologyMaxDerived)
			if !ok {
				return nil, false
			}
		}
	}
	return dst, true
}
func (t *treeTopology) namespace(c treeCone, dst []byte) []byte {
	if c.flags == 1 {
		return append(dst, t.data(t.rows[c.path].point)...)
	}
	return append(dst, t.paths[c.path].target...)
}
func (t *treeTopology) route(c treeCone, intersection, physical, dst []byte) ([]byte, bool) {
	suffix := intersection[len(physical):]
	if len(physical) == 1 && physical[0] == '/' {
		suffix = intersection
	}
	if bytes.Equal(physical, intersection) {
		suffix = nil
	}
	dst = t.namespace(c, dst)
	if len(dst) == 1 && dst[0] == '/' && len(suffix) > 0 {
		dst = dst[:0]
	}
	if len(dst)+len(suffix) > topologyMaxDerived {
		return nil, false
	}
	dst = append(dst, suffix...)
	return dst, treeAbsolute(dst, topologyMaxDerived, false)
}
func (t *treeTopology) compatible(a, b treeFootprint, allow bool) bool {
	if a.count == 0 || b.count == 0 {
		return false
	}
	for _, x := range t.cones[a.start : a.start+a.count] {
		for _, y := range t.cones[b.start : b.start+b.count] {
			if t.coordinates[x.coordinate].device != t.coordinates[y.coordinate].device {
				continue
			}
			xp, ok := t.conePhysical(x, t.work[0][:0])
			if !ok {
				return false
			}
			yp, ok := t.conePhysical(y, t.work[1][:0])
			if !ok {
				return false
			}
			var intersection, destination []byte
			if treeContains(xp, yp) {
				intersection = yp
				destination = t.work[0][:0]
			} else if treeContains(yp, xp) {
				intersection = xp
				destination = t.work[1][:0]
			} else {
				continue
			}
			if !allow {
				return false
			}
			left, ok := t.route(x, intersection, xp, t.work[2][:0])
			if !ok {
				return false
			}
			right, ok := t.route(y, intersection, yp, destination)
			if !ok || !bytes.Equal(left, right) {
				return false
			}
		}
	}
	return true
}
func (t *treeTopology) validatePaths() error {
	for _, p := range t.paths[:t.pathCount] {
		if validateEdges(p.chain, p.owner) != nil {
			return errDirectory
		}
	}
	for _, p := range t.pins[:t.pinCount] {
		if validateEdges(p.chain, p.owner) != nil {
			return errDirectory
		}
	}
	return nil
}
func (t *treeTopology) finish() error {
	if t.meta.validate() != nil || t.validatePaths() != nil {
		return errDirectory
	}
	fresh, err := t.readMountinfo(t.meta, t.fresh)
	if err != nil || !bytes.Equal(t.meta.snapshot, fresh) || t.validatePaths() != nil || t.meta.validate() != nil {
		return errDirectory
	}
	return nil
}
func (t *treeTopology) close() error {
	if t == nil {
		return nil
	}
	var failed bool
	for i := 0; i < t.pinCount; i++ {
		chain := t.pins[i].chain
		t.pins[i].chain = nil
		if closeChain(chain) != nil {
			failed = true
		}
	}
	t.pinCount, t.pinFDs = 0, 0
	clear(t.pins[:])
	clear(t.paths[:])
	t.pathCount = 0
	if t.meta != nil {
		meta := t.meta
		t.meta = nil
		if meta.close() != nil {
			failed = true
		}
		meta.snapshot = nil
	}
	t.raw, t.fresh, t.decoded, t.physical = nil, nil, nil, nil
	t.rows, t.ids, t.ops = nil, nil, nil
	if failed {
		return errDirectory
	}
	return nil
}

func treeRoleTopology(t *treeTopology, entries []roleEntry, home string) bool {
	var paths [64]topologyPath
	if len(entries) > len(paths) {
		return false
	}
	for i, e := range entries {
		paths[i] = topologyPath{e.target, e.chain, e.missing, e.owner}
	}
	footprints, err := t.footprints(paths[:len(entries)])
	if err != nil {
		return false
	}
	for i, e := range entries {
		if !rolePlacement(entries, e, home) || !t.compatible(footprints[i], footprints[i], true) {
			return false
		}
		for j := 0; j < i; j++ {
			allowed, valid := roleRelation(entries, e, entries[j])
			if !valid || !t.compatible(footprints[i], footprints[j], allowed) {
				return false
			}
		}
	}
	return true
}
