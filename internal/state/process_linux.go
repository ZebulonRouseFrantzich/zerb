//go:build linux

package state

import (
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

const (
	processStatLimit      = 4096
	processNamespaceLimit = 64
	processResolve        = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV
	processIdentityMask   = unix.STATX_TYPE | unix.STATX_MODE | unix.STATX_UID | unix.STATX_INO | unix.STATX_NLINK | unix.STATX_SIZE | unix.STATX_MNT_ID
)

var errCurrentProcess = errors.New("unsafe current process observation")

type processFD struct {
	owned bool
	fd    int
	name  string
	id    safefs.Identity
}

// The live initializer retains this current-process-only capability (ARCH-017).
// identity is cached evidence; only successful revalidation permits its use.
type currentProcess struct {
	mu                            sync.Mutex
	identity                      processIdentity
	pid                           string
	chain                         []processFD
	self, stat, nsLink, namespace processFD
	closed                        bool
}

func captureCurrentProcess() (_ *currentProcess, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := &currentProcess{pid: strconv.Itoa(unix.Getpid()), self: processFD{fd: -1}, stat: processFD{fd: -1}, nsLink: processFD{fd: -1}, namespace: processFD{fd: -1}}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.closeLocked())
		}
	}()
	fd, e := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errCurrentProcess
	}
	p.chain = []processFD{{fd: fd, owned: true}}
	p.chain[0].id, e = processObserve(fd, "")
	if e != nil || !processDirectory(p.chain[0].id) {
		return nil, errCurrentProcess
	}
	proc, e := processOpenDir(p.chain[0], "proc", unix.RESOLVE_BENEATH|unix.RESOLVE_NO_SYMLINKS)
	if e != nil {
		return nil, e
	}
	p.chain = append(p.chain, proc)
	p.self, e = p.openProcObject(proc, "self", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, unix.S_IFLNK)
	if e != nil {
		return nil, e
	}
	for _, name := range []string{p.pid, "ns"} {
		child, e := processOpenDir(p.chain[len(p.chain)-1], name, processResolve)
		if e != nil {
			return nil, e
		}
		p.chain = append(p.chain, child)
	}
	p.stat, e = p.openProcObject(p.chain[2], "stat", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, unix.S_IFREG)
	if e != nil {
		return nil, e
	}
	start, e := p.readStat()
	if e != nil {
		return nil, e
	}
	p.nsLink, e = p.openProcObject(p.chain[3], "pid", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, unix.S_IFLNK)
	if e != nil {
		return nil, e
	}
	p.namespace, e = p.openNamespace()
	if e != nil {
		return nil, e
	}
	p.identity = processIdentity{PID: unix.Getpid(), Start: start, PIDNamespace: p.namespace.id.Inode}
	if p.revalidateLocked() != nil {
		return nil, errCurrentProcess
	}
	return p, nil
}

func processObserve(fd int, name string) (safefs.Identity, error) {
	flags := unix.AT_SYMLINK_NOFOLLOW | unix.AT_NO_AUTOMOUNT
	if name == "" {
		flags |= unix.AT_EMPTY_PATH
	}
	var st unix.Statx_t
	if unix.Statx(fd, name, flags, processIdentityMask, &st) != nil || st.Mask&processIdentityMask != processIdentityMask || st.Mnt_id == 0 || st.Size > math.MaxInt64 {
		return safefs.Identity{}, errCurrentProcess
	}
	return safefs.Identity{Device: unix.Mkdev(st.Dev_major, st.Dev_minor), Inode: st.Ino, Owner: st.Uid, Mode: uint32(st.Mode), Links: uint64(st.Nlink), Size: int64(st.Size), MountID: st.Mnt_id}, nil
}

func processFilesystem(fd int, kind int64) bool {
	var fs unix.Statfs_t
	return unix.Fstatfs(fd, &fs) == nil && fs.Type == kind
}

func processDirectory(id safefs.Identity) bool {
	return id.Mode&unix.S_IFMT == unix.S_IFDIR && id.Mode&0022 == 0 && id.Links != 0 && id.MountID != 0
}

func processSameDirectory(a, b safefs.Identity) bool {
	// Kernel directory entry counts and sizes may change independently of identity.
	return a.Device == b.Device && a.Inode == b.Inode && a.Owner == b.Owner && a.Mode == b.Mode && a.MountID == b.MountID
}

func processSameMount(a, b safefs.Identity) bool {
	return a.Device == b.Device && a.MountID == b.MountID
}

func processOpenDir(parent processFD, name string, resolve uint64) (_ processFD, err error) {
	fd, e := unix.Openat2(parent.fd, name, &unix.OpenHow{Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC, Resolve: resolve})
	if e != nil {
		return processFD{fd: -1}, errCurrentProcess
	}
	entry := processFD{fd: fd, name: name, owned: true}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeProcessFDs([]processFD{entry}))
		}
	}()
	entry.id, e = processObserve(fd, "")
	linked, e2 := processObserve(parent.fd, name)
	if e != nil || e2 != nil || !processDirectory(entry.id) || !processSameDirectory(entry.id, linked) || !processFilesystem(fd, unix.PROC_SUPER_MAGIC) || resolve&unix.RESOLVE_NO_XDEV != 0 && !processSameMount(parent.id, entry.id) {
		return processFD{fd: -1}, errCurrentProcess
	}
	return entry, nil
}

func (p *currentProcess) openProcObject(parent processFD, name string, flags uint64, kind uint32) (_ processFD, err error) {
	fd, e := unix.Openat2(parent.fd, name, &unix.OpenHow{Flags: flags, Resolve: processResolve})
	if e != nil {
		return processFD{fd: -1}, errCurrentProcess
	}
	entry := processFD{fd: fd, name: name, owned: true}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeProcessFDs([]processFD{entry}))
		}
	}()
	entry.id, e = processObserve(fd, "")
	if e != nil || p.checkProcObject(entry, parent, kind) != nil {
		return processFD{fd: -1}, errCurrentProcess
	}
	return entry, nil
}

func (p *currentProcess) checkProcObject(entry, parent processFD, kind uint32) error {
	id, e := processObserve(entry.fd, "")
	linked, e2 := processObserve(parent.fd, entry.name)
	if e != nil || e2 != nil || id != entry.id || linked != id || id.Mode&unix.S_IFMT != kind || id.Links != 1 || !processSameMount(id, p.chain[1].id) || !processFilesystem(entry.fd, unix.PROC_SUPER_MAGIC) {
		return errCurrentProcess
	}
	return nil
}

func (p *currentProcess) checkSelf() error {
	if p.pid != strconv.Itoa(unix.Getpid()) || p.checkProcObject(p.self, p.chain[1], unix.S_IFLNK) != nil {
		return errCurrentProcess
	}
	var value [32]byte
	n, err := unix.Readlinkat(p.self.fd, "", value[:])
	if err != nil || n <= 0 || n >= len(value) || string(value[:n]) != p.pid {
		return errCurrentProcess
	}
	return p.checkProcObject(p.self, p.chain[1], unix.S_IFLNK)
}

func processNumber(value string) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	return n, err == nil && n > 0
}

func processNamespaceNumber(value string) (uint64, bool) {
	if !strings.HasPrefix(value, "pid:[") || !strings.HasSuffix(value, "]") {
		return 0, false
	}
	return processNumber(value[5 : len(value)-1])
}

func (p *currentProcess) namespaceNumber() (uint64, error) {
	if p.checkProcObject(p.nsLink, p.chain[3], unix.S_IFLNK) != nil {
		return 0, errCurrentProcess
	}
	var value [processNamespaceLimit + 1]byte
	n, err := unix.Readlinkat(p.nsLink.fd, "", value[:])
	if err != nil || n <= 0 || n > processNamespaceLimit {
		return 0, errCurrentProcess
	}
	number, ok := processNamespaceNumber(string(value[:n]))
	if !ok || p.checkProcObject(p.nsLink, p.chain[3], unix.S_IFLNK) != nil {
		return 0, errCurrentProcess
	}
	return number, nil
}

func processNamespaceValid(entry processFD) bool {
	id, err := processObserve(entry.fd, "")
	kind, e := unix.IoctlRetInt(entry.fd, unix.NS_GET_NSTYPE)
	return err == nil && id == entry.id && id.Mode&unix.S_IFMT == unix.S_IFREG && processFilesystem(entry.fd, unix.NSFS_MAGIC) && e == nil && kind == unix.CLONE_NEWPID
}

func (p *currentProcess) openNamespace() (_ processFD, err error) {
	number, e := p.namespaceNumber()
	if e != nil {
		return processFD{fd: -1}, e
	}
	// This is the only intentional final namespace magic-link traversal.
	fd, e := unix.Openat(p.chain[3].fd, "pid", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return processFD{fd: -1}, errCurrentProcess
	}
	entry := processFD{fd: fd, name: "pid", owned: true}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeProcessFDs([]processFD{entry}))
		}
	}()
	entry.id, e = processObserve(fd, "")
	after, e2 := p.namespaceNumber()
	if e != nil || e2 != nil || entry.id.Inode != number || after != number || !processNamespaceValid(entry) {
		return processFD{fd: -1}, errCurrentProcess
	}
	return entry, nil
}

func parseProcessStat(data []byte, pid string) (uint64, error) {
	if len(data) == 0 || len(data) > processStatLimit || data[len(data)-1] != '\n' || strings.IndexByte(string(data), 0) >= 0 {
		return 0, errCurrentProcess
	}
	text := string(data[:len(data)-1])
	prefix := pid + " ("
	end := strings.LastIndex(text, ") ")
	if !strings.HasPrefix(text, prefix) || end < len(prefix) || len(text)-end < 4 {
		return 0, errCurrentProcess
	}
	suffix := text[end+2:]
	if strings.ContainsAny(suffix, "\t\r\n") {
		return 0, errCurrentProcess
	}
	fields := strings.Split(suffix, " ")
	if len(fields) < 20 || len(fields[0]) != 1 {
		return 0, errCurrentProcess
	}
	for _, field := range fields {
		if field == "" {
			return 0, errCurrentProcess
		}
	}
	start, ok := processNumber(fields[19])
	if !ok {
		return 0, errCurrentProcess
	}
	return start, nil
}

func (p *currentProcess) readStat() (uint64, error) {
	if p.checkProcObject(p.stat, p.chain[2], unix.S_IFREG) != nil {
		return 0, errCurrentProcess
	}
	if _, err := unix.Seek(p.stat.fd, 0, unix.SEEK_SET); err != nil {
		return 0, errCurrentProcess
	}
	var data [processStatLimit + 1]byte
	used := 0
	for used < len(data) {
		n, err := unix.Read(p.stat.fd, data[used:])
		if err != nil || n < 0 {
			return 0, errCurrentProcess
		}
		used += n
		if used > processStatLimit {
			return 0, errCurrentProcess
		}
		if n == 0 {
			break
		}
	}
	if p.checkProcObject(p.stat, p.chain[2], unix.S_IFREG) != nil {
		return 0, errCurrentProcess
	}
	return parseProcessStat(data[:used], p.pid)
}

func (p *currentProcess) validateEdges() (err error) {
	if p.closed || len(p.chain) != 4 || p.pid != strconv.Itoa(unix.Getpid()) {
		return errCurrentProcess
	}
	fd, e := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return errCurrentProcess
	}
	defer func() { err = errors.Join(err, closeProcessFDs([]processFD{{fd: fd, owned: true}})) }()
	root, e := processObserve(fd, "")
	if e != nil || !processSameDirectory(root, p.chain[0].id) {
		return errCurrentProcess
	}
	for i, entry := range p.chain {
		id, e := processObserve(entry.fd, "")
		if e != nil || !processSameDirectory(id, entry.id) || !processDirectory(id) {
			return errCurrentProcess
		}
		if i > 0 {
			linked, e := processObserve(p.chain[i-1].fd, entry.name)
			if e != nil || !processSameDirectory(id, linked) || !processSameMount(id, p.chain[1].id) || !processFilesystem(entry.fd, unix.PROC_SUPER_MAGIC) {
				return errCurrentProcess
			}
		}
	}
	if p.checkSelf() != nil || p.checkProcObject(p.stat, p.chain[2], unix.S_IFREG) != nil || !processNamespaceValid(p.namespace) {
		return errCurrentProcess
	}
	number, e := p.namespaceNumber()
	if e != nil || number != p.namespace.id.Inode {
		return errCurrentProcess
	}
	return nil
}

func (p *currentProcess) revalidateLocked() (err error) {
	if p.identity.PID != unix.Getpid() || p.identity.Start == 0 || p.identity.PIDNamespace != p.namespace.id.Inode || p.validateEdges() != nil {
		return errCurrentProcess
	}
	start, e := p.readStat()
	if e != nil || start != p.identity.Start {
		return errCurrentProcess
	}
	current, e := p.openNamespace()
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, closeProcessFDs([]processFD{current})) }()
	if current.id != p.namespace.id {
		return errCurrentProcess
	}
	start, e = p.readStat()
	if e != nil || start != p.identity.Start {
		return errCurrentProcess
	}
	return p.validateEdges()
}

func (p *currentProcess) revalidate() error {
	if p == nil {
		return errCurrentProcess
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := p.revalidateLocked(); err != nil {
		return errors.Join(err, p.closeLocked())
	}
	return nil
}

func closeProcessFDs(owned []processFD) error {
	var err error
	for i := len(owned) - 1; i >= 0; i-- {
		entry := owned[i]
		owned[i] = processFD{fd: -1}
		if entry.owned && unix.Close(entry.fd) != nil {
			err = errors.Join(err, errCurrentProcess)
		}
	}
	return err
}

func (p *currentProcess) closeLocked() error {
	if p.closed {
		return nil
	}
	p.closed = true
	p.identity = processIdentity{}
	owned := p.chain
	p.chain = nil
	for _, entry := range []*processFD{&p.self, &p.stat, &p.nsLink, &p.namespace} {
		if entry.owned {
			owned = append(owned, *entry)
		}
		*entry = processFD{fd: -1}
	}
	return closeProcessFDs(owned)
}

func (p *currentProcess) close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return p.closeLocked()
}
