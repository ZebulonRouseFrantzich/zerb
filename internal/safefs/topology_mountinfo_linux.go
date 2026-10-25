//go:build linux

package safefs

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	topologyMaxBytes      = 8 << 20
	topologyMaxRecords    = 16384
	topologyMaxLine       = 64 << 10
	topologyMaxPath       = 4096
	topologyMaxComponents = 256
	topologyMaxDerived    = 3 * topologyMaxPath
	topologyMaxPaths      = 64
	topologyMaxMounts     = 256
	topologyMaxCones      = 1024
	topologyMaxFDs        = 4096
)

type topologyMount struct {
	id, parent, device uint64
	root, point, kind  string
}
type topologyTable struct {
	rows []topologyMount
	ids  map[uint64]int
	root int
}

func topologyNumber(s string, bits int) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, bits)
	return n, err == nil
}

func topologyAbsolute(s string, limit int, components bool) bool {
	if len(s) == 0 || len(s) > limit || s[0] != '/' || strings.IndexByte(s, 0) >= 0 {
		return false
	}
	if s == "/" {
		return true
	}
	if components && strings.Count(s, "/") > topologyMaxComponents {
		return false
	}
	for _, c := range strings.Split(s[1:], "/") {
		if c == "" || c == "." || c == ".." {
			return false
		}
	}
	return true
}

func topologyDecode(s string) (string, bool) {
	if len(s) > 4*topologyMaxPath {
		return "", false
	}
	out := make([]byte, 0, min(len(s), topologyMaxPath))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			if len(s)-i < 4 {
				return "", false
			}
			switch s[i : i+4] {
			case `\040`:
				c = ' '
			case `\011`:
				c = '\t'
			case `\012`:
				c = '\n'
			case `\134`:
				c = '\\'
			default:
				return "", false
			}
			i += 3
		}
		if len(out) == topologyMaxPath || c == 0 {
			return "", false
		}
		out = append(out, c)
	}
	result := string(out)
	return result, topologyAbsolute(result, topologyMaxPath, true)
}

func parseTopology(data []byte) (*topologyTable, error) {
	if len(data) == 0 || len(data) > topologyMaxBytes || data[len(data)-1] != '\n' || bytes.IndexByte(data, 0) >= 0 {
		return nil, errDirectory
	}
	t := &topologyTable{ids: make(map[uint64]int), root: -1}
	for len(data) != 0 {
		end := bytes.IndexByte(data, '\n')
		if end < 0 || end+1 > topologyMaxLine || len(t.rows) == topologyMaxRecords {
			return nil, errDirectory
		}
		// Fields are exactly ASCII-space separated; escaped whitespace stays encoded.
		spaces := bytes.Count(data[:end], []byte{' '})
		if spaces < 9 || spaces > 9+64 {
			return nil, errDirectory
		}
		fields := strings.Split(string(data[:end]), " ")
		data = data[end+1:]
		row, err := parseTopologyRow(fields)
		if err != nil {
			return nil, err
		}
		if _, duplicate := t.ids[row.id]; duplicate {
			return nil, errDirectory
		}

		if row.point == "/" {
			if t.root != -1 {
				return nil, errDirectory
			}
			t.root = len(t.rows)
		}
		t.ids[row.id] = len(t.rows)
		t.rows = append(t.rows, row)
	}
	if t.root < 0 {
		return nil, errDirectory
	}
	root := t.rows[t.root]
	if _, exists := t.ids[root.parent]; exists && root.parent != root.id {
		return nil, errDirectory
	}
	// A rooted functional graph can be checked once without recursive stack growth.
	colors := make([]uint8, len(t.rows))
	colors[t.root] = 2
	for i := range t.rows {
		if colors[i] == 2 {
			continue
		}
		var trail []int
		j := i
		for colors[j] != 2 {
			if colors[j] == 1 {
				return nil, errDirectory
			}
			colors[j] = 1
			trail = append(trail, j)
			row := t.rows[j]
			parent, ok := t.ids[row.parent]
			if !ok || parent == j || !topologyContains(t.rows[parent].point, row.point) {
				return nil, errDirectory
			}
			j = parent
		}
		for _, index := range trail {
			colors[index] = 2
		}
	}
	return t, nil
}

func topologyContains(parent, child string) bool {
	return parent == child || parent == "/" && strings.HasPrefix(child, "/") || strings.HasPrefix(child, parent+"/")
}

func topologyJoin(base, suffix string) (string, bool) {
	if suffix == "" {
		return base, len(base) <= topologyMaxDerived
	}
	if suffix[0] != '/' {
		return "", false
	}
	if base == "/" {
		base = ""
	}
	if len(base) > topologyMaxDerived-len(suffix) {
		return "", false
	}
	result := base + suffix
	return result, topologyAbsolute(result, topologyMaxDerived, false)
}

func topologySuffix(child, parent string) string {
	if child == parent {
		return ""
	}
	if parent == "/" {
		return child
	}
	return child[len(parent):]
}

type topologyCone struct {
	device              uint64
	physical, namespace string
}
type topologyFootprint struct{ cones []topologyCone }

func topologyCompatible(a, b topologyFootprint, allowSameRoute bool) bool {
	if len(a.cones) == 0 || len(b.cones) == 0 {
		return false
	}
	for _, x := range a.cones {
		for _, y := range b.cones {
			if x.device != y.device {
				continue
			}
			intersection := ""
			if topologyContains(x.physical, y.physical) {
				intersection = y.physical
			} else if topologyContains(y.physical, x.physical) {
				intersection = x.physical
			}
			if intersection == "" {
				continue
			}
			if !allowSameRoute {
				return false
			}
			left, ok := topologyJoin(x.namespace, topologySuffix(intersection, x.physical))
			if !ok {
				return false
			}
			right, ok := topologyJoin(y.namespace, topologySuffix(intersection, y.physical))
			if !ok || left != right {
				return false
			}
		}
	}
	return true
}

func parseTopologyRow(fields []string) (topologyMount, error) {
	if len(fields) < 10 || len(fields) > 10+64 {
		return topologyMount{}, errDirectory
	}
	for _, f := range fields {
		if f == "" || strings.ContainsAny(f, "\t\n") {
			return topologyMount{}, errDirectory
		}
	}
	sep := len(fields) - 4
	if fields[sep] != "-" || sep < 6 || sep-6 > 64 {
		return topologyMount{}, errDirectory
	}
	id, ok := topologyNumber(fields[0], 64)
	if !ok || id == 0 {
		return topologyMount{}, errDirectory
	}
	parent, ok := topologyNumber(fields[1], 64)
	if !ok || parent == 0 {
		return topologyMount{}, errDirectory
	}
	dev := strings.Split(fields[2], ":")
	if len(dev) != 2 {
		return topologyMount{}, errDirectory
	}
	major, ok := topologyNumber(dev[0], 32)
	if !ok {
		return topologyMount{}, errDirectory
	}
	minor, ok := topologyNumber(dev[1], 32)
	if !ok {
		return topologyMount{}, errDirectory
	}
	root, ok := topologyDecode(fields[3])
	if !ok {
		return topologyMount{}, errDirectory
	}
	point, ok := topologyDecode(fields[4])
	if !ok {
		return topologyMount{}, errDirectory
	}
	seen := make(map[string]bool)
	for _, f := range fields[6:sep] {
		tag, value, hasValue := strings.Cut(f, ":")
		if seen[tag] {
			return topologyMount{}, errDirectory
		}
		seen[tag] = true
		switch tag {
		case "unbindable":
			if hasValue {
				return topologyMount{}, errDirectory
			}
		case "shared", "master", "propagate_from":
			n, valid := topologyNumber(value, 64)
			if !hasValue || !valid || n == 0 {
				return topologyMount{}, errDirectory
			}
		default:
			return topologyMount{}, errDirectory
		}
	}
	return topologyMount{id: id, parent: parent, device: unix.Mkdev(uint32(major), uint32(minor)), root: root, point: point, kind: fields[sep+1]}, nil
}
