//go:build linux

package safefs

import (
	"bytes"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type topologyPath struct {
	target  string
	chain   []edge
	missing []string
	owner   uint32
}
type namespacePin struct {
	fd       int
	id, root Identity
}
type topology struct {
	chain        []edge
	self, nsLink edge
	namespace    *namespacePin
	pid, tid     string
	snapshot     []byte
	table        *topologyTable
	pins         []topologyPath
	borrowed     []topologyPath
	coordinates  map[topologyCoordinate]topologyObject
	objects      map[topologyObject]topologyCoordinate
	cones        int
	frontendFDs  int
	fdLimit      int
	closed       bool
}

func (t *topology) room(n int) bool {
	count := len(t.chain) + t.frontendFDs
	if t.self.fd >= 0 {
		count++
	}
	if t.nsLink.fd >= 0 {
		count++
	}
	if t.namespace != nil && t.namespace.fd >= 0 {
		count++
	}
	for _, p := range t.pins {
		count += len(p.chain)
	}
	limit := t.fdLimit
	if limit == 0 {
		limit = topologyMaxFDs
	}
	return n >= 0 && count <= limit-n
}

func openTopology(expected *namespacePin) (_ *topology, err error) {
	t, err := openTopologyMetadata(expected)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.close())
		}
	}()
	t.table, err = parseTopology(t.snapshot)
	if err != nil || t.table.rows[t.table.root].id != t.chain[0].id.MountID {
		return nil, errDirectory
	}
	return t, nil
}

func openTopologyMetadata(expected *namespacePin) (_ *topology, err error) {
	return openTopologyMetadataWith(expected, 0, topologyMaxFDs, (*topology).readMountinfo)
}

// The stage reader supplies its already-reserved byte and descriptor limits.
func openTopologyMetadataWith(expected *namespacePin, frontend, limit int, read func(*topology) ([]byte, error)) (_ *topology, err error) {
	t := &topology{self: edge{fd: -1}, nsLink: edge{fd: -1}, pid: strconv.Itoa(unix.Getpid()), tid: strconv.Itoa(unix.Gettid()), frontendFDs: frontend, fdLimit: limit}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.close())
		}
	}()
	if !t.room(11) {
		return nil, errDirectory
	}
	fd, e := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errDirectory
	}
	t.chain = []edge{{fd: fd}}
	t.chain[0].id, e = observeFD(fd)
	if e != nil || !safeDirectory(t.chain[0].id) {
		return nil, errDirectory
	}
	proc, e := openProcDir(t.chain[0], "proc", unix.RESOLVE_BENEATH|unix.RESOLVE_NO_SYMLINKS)
	if e != nil {
		return nil, e
	}
	t.chain = append(t.chain, proc)
	fd, e = unix.Openat2(proc.fd, "self", &unix.OpenHow{Flags: unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: procResolve})
	if e != nil {
		return nil, errDirectory
	}
	t.self = edge{fd: fd, name: "self"}
	t.self.id, e = observeFD(fd)
	if e != nil {
		return nil, e
	}
	for _, name := range []string{t.pid, "task", t.tid, "ns"} {
		child, e := openProcDir(t.chain[len(t.chain)-1], name, procResolve)
		if e != nil {
			return nil, e
		}
		t.chain = append(t.chain, child)
	}
	fd, e = unix.Openat2(t.chain[5].fd, "mnt", &unix.OpenHow{Flags: unix.O_PATH | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: procResolve})
	if e != nil {
		return nil, errDirectory
	}
	t.nsLink = edge{fd: fd, name: "mnt"}
	t.nsLink.id, e = observeFD(fd)
	if e != nil {
		return nil, e
	}
	ns, e := t.openNamespace()
	if e != nil {
		return nil, e
	}
	t.namespace = ns
	if expected != nil && (expected.fd < 0 || expected.id != ns.id || !sameDirectory(expected.root, t.chain[0].id) || expected.validate() != nil) {
		return nil, errDirectory
	}
	if t.validate() != nil {
		return nil, errDirectory
	}
	t.snapshot, e = read(t)
	if e != nil {
		return nil, e
	}
	return t, nil
}

func (t *topology) checkSelf() error {
	p := &procAccess{chain: t.chain, self: t.self, pid: t.pid}
	return p.checkSelf()
}

func namespaceNumber(link string) (uint64, bool) {
	if !strings.HasPrefix(link, "mnt:[") || !strings.HasSuffix(link, "]") {
		return 0, false
	}
	n, ok := topologyNumber(link[5:len(link)-1], 64)
	return n, ok && n > 0
}

func (t *topology) namespaceLink() (uint64, error) {
	id, err := observeFD(t.nsLink.fd)
	if err != nil || id != t.nsLink.id || id.Mode&unix.S_IFMT != unix.S_IFLNK || id.Links != 1 || !sameMount(id, t.chain[1].id) || !procFilesystem(t.nsLink.fd) {
		return 0, errDirectory
	}
	linked, err := observeAt(t.chain[5].fd, "mnt")
	if err != nil || linked != id {
		return 0, errDirectory
	}
	var value [64]byte
	n, err := unix.Readlinkat(t.nsLink.fd, "", value[:])
	if err != nil || n <= 0 || n >= len(value) {
		return 0, errDirectory
	}
	number, ok := namespaceNumber(string(value[:n]))
	if !ok {
		return 0, errDirectory
	}
	return number, nil
}

func (t *topology) openNamespace() (_ *namespacePin, err error) {
	number, e := t.namespaceLink()
	if e != nil || !t.room(1) {
		return nil, errDirectory
	}
	// Only this fixed final, already pinned namespace magic link may be followed.
	fd, e := unix.Openat(t.chain[5].fd, "mnt", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errDirectory
	}
	pin := &namespacePin{fd: fd, root: t.chain[0].id}
	defer func() {
		if err != nil {
			err = errors.Join(err, pin.close())
		}
	}()
	pin.id, e = observeFD(fd)
	if e != nil || pin.id.Inode != number || pin.validate() != nil {
		return nil, errDirectory
	}
	after, e := t.namespaceLink()
	if e != nil || after != number {
		return nil, errDirectory
	}
	return pin, nil
}

func (n *namespacePin) validate() error {
	if n == nil || n.fd < 0 {
		return errDirectory
	}
	id, err := observeFD(n.fd)
	var fs unix.Statfs_t
	kind, e := unix.IoctlRetInt(n.fd, unix.NS_GET_NSTYPE)
	if err != nil || id != n.id || id.Mode&unix.S_IFMT != unix.S_IFREG || unix.Fstatfs(n.fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || e != nil || kind != unix.CLONE_NEWNS {
		return errDirectory
	}
	return nil
}

func (t *topology) retainNamespace() (*namespacePin, error) {
	if t == nil || t.closed || !t.room(3) || t.validate() != nil {
		return nil, errDirectory
	}
	fd, err := unix.FcntlInt(uintptr(t.namespace.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, errDirectory
	}
	return &namespacePin{fd: fd, id: t.namespace.id, root: t.chain[0].id}, nil
}

func (t *topology) validate() (err error) {
	if t == nil || t.closed || len(t.chain) != 6 || t.pid != strconv.Itoa(unix.Getpid()) || t.tid != strconv.Itoa(unix.Gettid()) || !t.room(2) {
		return errDirectory
	}
	fd, e := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return errDirectory
	}
	defer func() { err = errors.Join(err, closeChain([]edge{{fd: fd}})) }()
	root, e := observeFD(fd)
	if e != nil || !sameDirectory(root, t.chain[0].id) {
		return errDirectory
	}
	for i, entry := range t.chain {
		id, e := observeFD(entry.fd)
		if e != nil || !sameDirectory(id, entry.id) || !safeDirectory(id) {
			return errDirectory
		}
		if i > 0 {
			linked, e := observeAt(t.chain[i-1].fd, entry.name)
			if e != nil || !sameDirectory(id, linked) || !procFilesystem(entry.fd) || !sameMount(id, t.chain[1].id) {
				return errDirectory
			}
		}
	}
	if t.checkSelf() != nil || t.namespace.validate() != nil {
		return errDirectory
	}
	current, e := t.openNamespace()
	if e != nil {
		return e
	}
	same := current.id == t.namespace.id
	e = current.close()
	if !same || e != nil {
		return errDirectory
	}
	return nil
}

func (t *topology) readMountinfo() (_ []byte, err error) {
	if !t.room(1) {
		return nil, errDirectory
	}
	parent := t.chain[4]
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
	var result []byte
	var chunk [32768]byte
	for {
		// One byte beyond the accepted bound is only an EOF/overflow probe.
		n, e := unix.Read(fd, chunk[:min(len(chunk), topologyMaxBytes-len(result)+1)])
		if e != nil || n > topologyMaxBytes-len(result) {
			return nil, errDirectory
		}
		if n == 0 {
			break
		}
		result = append(result, chunk[:n]...)
	}
	after, e := observeFD(fd)
	linked, e2 = observeAt(parent.fd, "mountinfo")
	if e != nil || e2 != nil || after != before || linked != before {
		return nil, errDirectory
	}
	return result, nil
}

func (n *namespacePin) close() error {
	if n == nil || n.fd < 0 {
		return nil
	}
	fd := n.fd
	n.fd = -1
	return closeChain([]edge{{fd: fd}})
}

func (t *topology) close() error {
	if t == nil || t.closed {
		return nil
	}
	t.closed = true
	owned := t.chain
	t.chain = nil
	for _, p := range t.pins {
		owned = append(owned, p.chain...)
	}
	t.pins = nil
	t.borrowed = nil
	if t.self.fd >= 0 {
		owned = append(owned, t.self)
	}
	t.self = edge{fd: -1}
	if t.nsLink.fd >= 0 {
		owned = append(owned, t.nsLink)
	}
	t.nsLink = edge{fd: -1}
	ns := t.namespace
	t.namespace = nil
	return errors.Join(closeChain(owned), ns.close())
}

func (t *topology) validatePaths() error {
	for _, paths := range [][]topologyPath{t.borrowed, t.pins} {
		for _, p := range paths {
			if validateEdges(p.chain, p.owner) != nil {
				return errDirectory
			}
		}
	}
	return nil
}

func (t *topology) finish() error {
	if t.validate() != nil || t.validatePaths() != nil {
		return errDirectory
	}
	fresh, err := t.readMountinfo()
	if err != nil {
		return err
	}
	return t.finishObservation(fresh)
}

func (t *topology) finishObservation(fresh []byte) error {
	if !bytes.Equal(t.snapshot, fresh) || t.validatePaths() != nil {
		return errDirectory
	}
	return t.validate()
}
