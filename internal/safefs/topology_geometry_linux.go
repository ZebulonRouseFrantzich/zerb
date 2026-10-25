//go:build linux

package safefs

import (
	"strings"

	"golang.org/x/sys/unix"
)

type topologyCoordinate struct {
	device   uint64
	physical string
}
type topologyObject struct {
	device, inode uint64
	kind          uint32
}

func (t *topology) relevant(paths []topologyPath) ([]bool, error) {
	relevant := make([]bool, len(t.table.rows))
	for i, row := range t.table.rows {
		for _, p := range paths {
			if topologyContains(row.point, p.target) || topologyContains(p.target, row.point) {
				relevant[i] = true
				break
			}
		}
	}
	for i := range relevant {
		if !relevant[i] {
			continue
		}
		for j := i; j != t.table.root; {
			j = t.table.ids[t.table.rows[j].parent]
			relevant[j] = true
		}
	}
	seen := make(map[string]bool)
	for i, row := range t.table.rows {
		if !relevant[i] {
			continue
		}
		if seen[row.point] || row.kind != "btrfs" && row.kind != "tmpfs" {
			return nil, errDirectory
		}
		seen[row.point] = true
	}
	return relevant, nil
}

func (t *topology) correlate(p topologyPath) (topologyCone, error) {
	if len(p.chain) == 0 || len(p.chain) > topologyMaxComponents+1 || !topologyAbsolute(p.target, topologyMaxPath, true) || !sameDirectory(p.chain[0].id, t.chain[0].id) || validateEdges(p.chain, p.owner) != nil {
		return topologyCone{}, errDirectory
	}
	current := "/"
	var last topologyCone
	for i, entry := range p.chain {
		if i > 0 {
			if !validName(entry.name) {
				return topologyCone{}, errDirectory
			}
			var ok bool
			current, ok = topologyJoin(current, "/"+entry.name)
			if !ok || !topologyAbsolute(current, topologyMaxPath, true) {
				return topologyCone{}, errDirectory
			}
		}
		index, ok := t.table.ids[entry.id.MountID]
		if !ok {
			return topologyCone{}, errDirectory
		}
		row := t.table.rows[index]
		if i == 0 && index != t.table.root {
			return topologyCone{}, errDirectory
		}
		previous := uint64(0)
		if i > 0 {
			previous = p.chain[i-1].id.MountID
		}
		var fs unix.Statfs_t
		if unix.Fstatfs(entry.fd, &fs) != nil {
			return topologyCone{}, errDirectory
		}
		coordinate, ok := topologyAssociate(row, entry.id, previous, current, fs.Type)
		if !ok {
			return topologyCone{}, errDirectory
		}
		if !t.recordCoordinate(coordinate, entry.id) {
			return topologyCone{}, errDirectory
		}
		last = topologyCone{device: row.device, physical: coordinate.physical, namespace: current}
	}
	for _, part := range p.missing {
		if !validName(part) {
			return topologyCone{}, errDirectory
		}
		var ok bool
		current, ok = topologyJoin(current, "/"+part)
		if !ok {
			return topologyCone{}, errDirectory
		}
		last.physical, ok = topologyJoin(last.physical, "/"+part)
		if !ok {
			return topologyCone{}, errDirectory
		}
	}
	if current != p.target {
		return topologyCone{}, errDirectory
	}
	last.namespace = current
	return last, nil
}

func (t *topology) pinMount(point string, owner uint32) (topologyPath, error) {
	for _, p := range t.pins {
		if p.target == point && p.owner == owner {
			return p, nil
		}
	}
	if len(t.pins) == topologyMaxMounts || !t.room(3) {
		return topologyPath{}, errDirectory
	}
	fd, err := unix.FcntlInt(uintptr(t.chain[0].fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return topologyPath{}, errDirectory
	}
	index := len(t.pins)
	t.pins = append(t.pins, topologyPath{target: point, owner: owner, chain: []edge{{fd: fd, id: t.chain[0].id}}})
	owned, err := advanceOwner(t.chain[0].id, owner, false)
	if err != nil {
		return topologyPath{}, err
	}
	if point != "/" {
		for _, part := range strings.Split(point[1:], "/") {
			if !t.room(3) {
				return topologyPath{}, errDirectory
			}
			chain := t.pins[index].chain
			expected, e := observeAt(chain[len(chain)-1].fd, part)
			if e != nil {
				return topologyPath{}, errDirectory
			}
			owned, e = advanceOwner(expected, owner, owned)
			if e != nil {
				return topologyPath{}, e
			}
			child, e := openObserved(chain[len(chain)-1].fd, part, expected)
			if e != nil {
				return topologyPath{}, e
			}
			t.pins[index].chain = append(chain, child)
		}
	}
	p := t.pins[index]
	if p.chain[len(p.chain)-1].id.Owner != owner || validateEdges(p.chain, owner) != nil {
		return topologyPath{}, errDirectory
	}
	return p, nil
}

func (t *topology) footprints(paths []topologyPath) ([]topologyFootprint, error) {
	if t == nil || t.closed || len(paths) == 0 || len(paths) > topologyMaxPaths || t.validate() != nil {
		return nil, errDirectory
	}
	for _, p := range paths {
		if !topologyAbsolute(p.target, topologyMaxPath, true) {
			return nil, errDirectory
		}
	}
	relevant, err := t.relevant(paths)
	if err != nil {
		return nil, err
	}
	if t.coordinates == nil {
		t.coordinates = make(map[topologyCoordinate]topologyObject)
		t.objects = make(map[topologyObject]topologyCoordinate)
	}
	result := make([]topologyFootprint, len(paths))
	for i, p := range paths {
		cone, e := t.correlate(p)
		if e != nil {
			return nil, e
		}
		if t.cones == topologyMaxCones {
			return nil, errDirectory
		}
		t.cones++
		result[i].cones = []topologyCone{cone}
		for j, row := range t.table.rows {
			if !relevant[j] || row.point == p.target || !topologyContains(p.target, row.point) {
				continue
			}
			pin, e := t.pinMount(row.point, p.owner)
			if e != nil {
				return nil, e
			}
			cone, e := t.correlate(pin)
			if e != nil {
				return nil, e
			}
			// Correlation must observe precisely this row, never a hidden stacked row.
			if pin.chain[len(pin.chain)-1].id.MountID != row.id {
				return nil, errDirectory
			}
			if t.cones == topologyMaxCones {
				return nil, errDirectory
			}
			t.cones++
			result[i].cones = append(result[i].cones, cone)
		}
		if !topologyCompatible(result[i], result[i], true) {
			return nil, errDirectory
		}
	}
	t.borrowed = append(t.borrowed, paths...)
	return result, nil
}

func (t *topology) recordCoordinate(coordinate topologyCoordinate, id Identity) bool {
	object := topologyObject{id.Device, id.Inode, id.Mode & unix.S_IFMT}
	if prior, exists := t.coordinates[coordinate]; exists && prior != object {
		return false
	}
	if prior, exists := t.objects[object]; exists && prior != coordinate {
		return false
	}
	if len(t.coordinates) >= topologyMaxFDs {
		if _, exists := t.coordinates[coordinate]; !exists {
			return false
		}
	}
	t.coordinates[coordinate] = object
	t.objects[object] = coordinate
	return true
}

func topologyAssociate(row topologyMount, id Identity, previous uint64, current string, fsType int64) (topologyCoordinate, bool) {
	if row.id != id.MountID || !topologyContains(row.point, current) {
		return topologyCoordinate{}, false
	}
	if previous == 0 {
		if current != "/" || row.point != "/" {
			return topologyCoordinate{}, false
		}
	} else if id.MountID != previous && (current != row.point || row.parent != previous) {
		return topologyCoordinate{}, false
	}
	if row.kind == "btrfs" && fsType != unix.BTRFS_SUPER_MAGIC || row.kind == "tmpfs" && fsType != unix.TMPFS_MAGIC || row.kind != "btrfs" && row.kind != "tmpfs" {
		return topologyCoordinate{}, false
	}
	physical, ok := topologyJoin(row.root, topologySuffix(current, row.point))
	return topologyCoordinate{row.device, physical}, ok
}
