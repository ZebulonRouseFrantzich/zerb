//go:build linux

// Package filesystem contains Linux-only disposable proof primitives. It is not
// production state code and deliberately has no fallback for other platforms.
package filesystem

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const btrfsMagic = 0x9123683e
const atSymlinkNoFollow = 0x100

// Config is the common disposable-proof root contract.
type Config struct {
	Root           string   `json:"root"`
	ArtifactDir    string   `json:"artifact_dir"`
	CACert         string   `json:"ca_cert,omitempty"`
	RepositoryRoot string   `json:"repository_root"`
	ProtectedRoots []string `json:"-"`
}

type RootIdentity struct {
	Path       string `json:"path"`
	Device     uint64 `json:"device"`
	Inode      uint64 `json:"inode"`
	Mode       uint32 `json:"mode"`
	Owner      uint32 `json:"owner"`
	Filesystem string `json:"filesystem"`
}

type OwnedRoot struct {
	Path       string
	Name       string
	Dir        *os.File
	ParentDir  *os.File
	Root       *os.Root
	ParentRoot *os.Root
	Identity   RootIdentity
}

// Prepare creates one confined run root below a pre-existing canonical btrfs
// base. It rejects overlap with the repository, the real HOME, and every real
// XDG root before creating anything.
func Prepare(cfg Config) (*OwnedRoot, error) {
	if cfg.Root == "" || !filepath.IsAbs(cfg.Root) {
		return nil, errors.New("root must be an explicit absolute path")
	}
	base, err := filepath.EvalSymlinks(filepath.Clean(cfg.Root))
	if err != nil {
		return nil, fmt.Errorf("canonical root: %w", err)
	}
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		return nil, errors.New("canonical root is not a directory")
	}
	if len(cfg.ProtectedRoots) == 0 {
		return nil, errors.New("canonical protected roots captured before environment poisoning are required")
	}
	for _, forbidden := range append([]string{cfg.RepositoryRoot}, cfg.ProtectedRoots...) {
		if forbidden == "" {
			continue
		}
		canon, e := canonicalExisting(forbidden)
		if e != nil {
			return nil, fmt.Errorf("canonicalize protected root: %w", e)
		}
		if overlaps(base, canon) {
			return nil, errors.New("disposable base overlaps a protected root")
		}
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(base, &fs); err != nil {
		return nil, fmt.Errorf("statfs: %w", err)
	}
	if uint64(fs.Type) != btrfsMagic {
		return nil, fmt.Errorf("disposable base is not btrfs (type=%#x)", uint64(fs.Type))
	}
	old := syscall.Umask(0)
	run, err := os.MkdirTemp(base, "zerb-stateproof-")
	syscall.Umask(old)
	if err != nil {
		return nil, err
	}
	cleanupPath := func() { _ = os.Remove(run) }
	if err := os.Chmod(run, 0700); err != nil {
		cleanupPath()
		return nil, err
	}
	parentDir, err := os.Open(base)
	if err != nil {
		cleanupPath()
		return nil, err
	}
	parentRoot, err := os.OpenRoot(base)
	if err != nil {
		_ = parentDir.Close()
		cleanupPath()
		return nil, err
	}
	safeRoot, err := os.OpenRoot(run)
	if err != nil {
		_ = parentRoot.Close()
		_ = parentDir.Close()
		cleanupPath()
		return nil, err
	}
	d, err := os.Open(run)
	if err != nil {
		_ = safeRoot.Close()
		_ = parentRoot.Close()
		_ = parentDir.Close()
		cleanupPath()
		return nil, err
	}
	id, err := identity(run, "btrfs")
	if err != nil {
		_ = d.Close()
		_ = safeRoot.Close()
		_ = parentRoot.Close()
		_ = parentDir.Close()
		cleanupPath()
		return nil, err
	}
	return &OwnedRoot{Path: run, Name: filepath.Base(run), Dir: d, ParentDir: parentDir, Root: safeRoot, ParentRoot: parentRoot, Identity: id}, nil
}

func canonicalExisting(p string) (string, error) {
	a, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	return filepath.EvalSymlinks(a)
}
func overlaps(a, b string) bool {
	rel, e := filepath.Rel(a, b)
	if e == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return true
	}
	rel, e = filepath.Rel(b, a)
	return e == nil && (rel == "." || !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
func identity(p, fs string) (RootIdentity, error) {
	var s syscall.Stat_t
	if e := syscall.Lstat(p, &s); e != nil {
		return RootIdentity{}, e
	}
	return RootIdentity{Path: p, Device: uint64(s.Dev), Inode: s.Ino, Mode: uint32(s.Mode), Owner: s.Uid, Filesystem: fs}, nil
}

// Cleanup first atomically moves the still-identical owned directory to a
// private tombstone in its already-open parent. It then removes entries only
// through an os.Root bound to the original directory. A substituted binding is
// preserved and reported rather than recursively traversed.
func (r *OwnedRoot) Cleanup() (retErr error) {
	defer func() {
		for _, closer := range []interface{ Close() error }{r.Dir, r.Root, r.ParentRoot, r.ParentDir} {
			if err := closer.Close(); retErr == nil && err != nil {
				retErr = err
			}
		}
	}()

	current, err := r.ParentRoot.Lstat(r.Name)
	if err != nil {
		return err
	}
	opened, err := r.Dir.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(current, opened) {
		return errors.New("owned root was substituted; refusing cleanup")
	}

	tombstone := r.Name + ".cleanup"
	if _, err = r.ParentRoot.Lstat(tombstone); err == nil || !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			return errors.New("cleanup tombstone already exists")
		}
		return err
	}
	if err = unix.Renameat(int(r.ParentDir.Fd()), r.Name, int(r.ParentDir.Fd()), tombstone); err != nil {
		return err
	}
	moved, err := r.ParentRoot.Lstat(tombstone)
	if err != nil {
		return err
	}
	if !os.SameFile(moved, opened) {
		return errors.New("owned root changed during cleanup rename; preserving residue")
	}

	entries, err := fs.ReadDir(r.Root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = removeAllRoot(r.Root, entry.Name()); err != nil {
			return err
		}
	}
	if err = r.Dir.Sync(); err != nil {
		return err
	}
	if err = r.ParentDir.Sync(); err != nil {
		return err
	}
	final, err := r.ParentRoot.Lstat(tombstone)
	if err != nil {
		return err
	}
	if !os.SameFile(final, opened) {
		return errors.New("cleanup tombstone was substituted; preserving residue")
	}
	if err = r.ParentRoot.Remove(tombstone); err != nil {
		return err
	}
	return r.ParentDir.Sync()
}

func removeAllRoot(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return root.Remove(name)
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	entries, readErr := fs.ReadDir(child.FS(), ".")
	if readErr == nil {
		for _, entry := range entries {
			if err = removeAllRoot(child, entry.Name()); err != nil {
				break
			}
		}
	}
	closeErr := child.Close()
	if readErr != nil {
		return readErr
	}
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Remove(name)
}

func validateRel(rel string) ([]string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "\\") {
		return nil, errors.New("invalid relative path")
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return nil, errors.New("ambiguous relative path")
		}
	}
	return parts, nil
}

// MkdirAllAt creates descriptor-relative directories with restrictive mode
// established at creation and corrected independently of the caller's umask.
func MkdirAllAt(rootfd int, rel string) error {
	parts, err := validateRel(rel)
	if err != nil {
		return err
	}
	fd := rootfd
	owned := []int{}
	defer func() {
		for _, x := range owned {
			syscall.Close(x)
		}
	}()
	for _, p := range parts {
		e := syscall.Mkdirat(fd, p, 0700)
		created := e == nil
		if e != nil && e != syscall.EEXIST {
			return e
		}
		if created {
			if e = syscall.Fchmodat(fd, p, 0700, atSymlinkNoFollow); e != nil {
				return e
			}
		}
		n, e := syscall.Openat(fd, p, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if e != nil {
			return e
		}
		var s syscall.Stat_t
		if e = syscall.Fstat(n, &s); e != nil {
			syscall.Close(n)
			return e
		}
		if s.Mode&syscall.S_IFMT != syscall.S_IFDIR || s.Mode&0777 != 0700 || s.Uid != uint32(os.Geteuid()) {
			syscall.Close(n)
			return errors.New("unsafe directory")
		}
		owned = append(owned, n)
		fd = n
	}
	return syscall.Fsync(fd)
}

type Boundary string

const (
	BeforeTemp      Boundary = "before-temp"
	AfterTemp       Boundary = "after-temp"
	AfterWrite      Boundary = "after-write"
	AfterFileSync   Boundary = "after-file-sync"
	BeforeRename    Boundary = "before-rename"
	AfterRename     Boundary = "after-rename"
	AfterParentSync Boundary = "after-parent-sync"
	AfterReopen     Boundary = "after-reopen"
)

type PublicationIdentity struct {
	Device uint64
	Inode  uint64
}

type PublishOptions struct {
	Fail    func(Boundary) error
	Capture func(PublicationIdentity)
}

func hit(o PublishOptions, b Boundary) error {
	if o.Fail != nil {
		return o.Fail(b)
	}
	return nil
}

// AtomicPublishAt performs same-directory temp, complete write, file fsync,
// rename, parent fsync, and no-follow reopen verification.
func AtomicPublishAt(dirfd int, name string, data []byte, opt PublishOptions) error {
	if _, e := validateRel(name); e != nil || strings.Contains(name, "/") {
		return errors.New("publication name must be one component")
	}
	if e := hit(opt, BeforeTemp); e != nil {
		return e
	}
	random := make([]byte, 12)
	if _, e := io.ReadFull(rand.Reader, random); e != nil {
		return e
	}
	tmp := ".proof-" + hex.EncodeToString(random)
	fd, e := syscall.Openat(dirfd, tmp, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	closed, renamed := false, false
	var created syscall.Stat_t
	if e = syscall.Fstat(fd, &created); e != nil {
		syscall.Close(fd)
		return e
	}
	if opt.Capture != nil {
		opt.Capture(PublicationIdentity{Device: uint64(created.Dev), Inode: created.Ino})
	}
	defer func() {
		if !closed {
			_ = syscall.Close(fd)
		}
		if renamed {
			return
		}
		check, x := syscall.Openat(dirfd, tmp, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if x != nil {
			return
		}
		defer syscall.Close(check)
		var current syscall.Stat_t
		if syscall.Fstat(check, &current) == nil && current.Dev == created.Dev && current.Ino == created.Ino {
			_ = syscall.Unlinkat(dirfd, tmp)
		}
	}()
	if e = syscall.Fchmod(fd, 0600); e != nil {
		return e
	}
	if e = hit(opt, AfterTemp); e != nil {
		return e
	}
	for off := 0; off < len(data); {
		n, x := syscall.Write(fd, data[off:])
		if x != nil {
			return x
		}
		off += n
	}
	if e = hit(opt, AfterWrite); e != nil {
		return e
	}
	if e = syscall.Fsync(fd); e != nil {
		return e
	}
	if e = hit(opt, AfterFileSync); e != nil {
		return e
	}
	if e = syscall.Close(fd); e != nil {
		return e
	}
	closed = true
	if e = hit(opt, BeforeRename); e != nil {
		return e
	}
	if e = syscall.Renameat(dirfd, tmp, dirfd, name); e != nil {
		return e
	}
	renamed = true
	if e = hit(opt, AfterRename); e != nil {
		return e
	}
	if e = syscall.Fsync(dirfd); e != nil {
		return e
	}
	if e = hit(opt, AfterParentSync); e != nil {
		return e
	}
	r, e := syscall.Openat(dirfd, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer syscall.Close(r)
	var s syscall.Stat_t
	if e = syscall.Fstat(r, &s); e != nil {
		return e
	}
	if s.Mode&syscall.S_IFMT != syscall.S_IFREG || s.Mode&0777 != 0600 || s.Nlink != 1 ||
		s.Dev != created.Dev || s.Ino != created.Ino {
		return errors.New("published file failed topology or identity verification")
	}
	got := make([]byte, len(data)+1)
	n, e := syscall.Read(r, got)
	if e != nil {
		return e
	}
	if n != len(data) || !bytes.Equal(got[:n], data) {
		return errors.New("published bytes mismatch")
	}
	return hit(opt, AfterReopen)
}

type EntryObservation struct {
	Path, Type, SHA256   string
	Mode, Owner          uint32
	Device, Inode, Links uint64
	Size                 int64
}

func Snapshot(root string) ([]EntryObservation, error) {
	var out []EntryObservation
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if p == root {
			return nil
		}
		info, e := os.Lstat(p)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		s := info.Sys().(*syscall.Stat_t)
		typ := "other"
		sum := ""
		if info.Mode().IsRegular() {
			typ = "file"
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			h := sha256.Sum256(b)
			sum = hex.EncodeToString(h[:])
		} else if info.IsDir() {
			typ = "directory"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typ = "symlink"
		}
		out = append(out, EntryObservation{Path: filepath.ToSlash(rel), Type: typ, SHA256: sum, Mode: uint32(info.Mode().Perm()), Owner: s.Uid, Device: uint64(s.Dev), Inode: s.Ino, Links: uint64(s.Nlink), Size: info.Size()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

type ArchiveLimits struct {
	EntryBytes, TotalBytes int64
	Count, Depth           int
}

type ArchiveBoundary string

const (
	ArchiveCreateStage   ArchiveBoundary = "create-stage"
	ArchiveCreateDir     ArchiveBoundary = "create-directory"
	ArchiveCreateFile    ArchiveBoundary = "create-file"
	ArchiveWriteFile     ArchiveBoundary = "write-file"
	ArchiveFileSync      ArchiveBoundary = "file-sync"
	ArchiveDirectorySync ArchiveBoundary = "directory-sync"
	ArchiveRename        ArchiveBoundary = "rename-no-replace"
	ArchiveParentSync    ArchiveBoundary = "parent-sync"
	ArchiveReopen        ArchiveBoundary = "reopen"
	ArchiveStagingPrefix                 = ".archive-stage-"
)

type ArchiveOptions struct {
	Fail func(ArchiveBoundary) error
}

type ArchiveObservation struct {
	IntegrityVerified        bool
	ProvenanceVerified       bool
	Parsed                   bool
	Prevalidated             bool
	Extracted                bool
	Published                bool
	ParentSynced             bool
	Reopened                 bool
	Verified                 bool
	Cleaned                  bool
	CleanupIdentityPreserved bool
	CleanupFailed            bool
	Entries                  int
	ExpandedBytes            int64
	Boundaries               int
	Creates                  int
	Writes                   int
	FileSyncs                int
	DirectorySyncs           int
	Renames                  int
	ParentSyncs              int
	Reopens                  int
	LastBoundary             ArchiveBoundary
	Rejection                string
}

type archiveEntry struct {
	name string
	mode uint32
	dir  bool
	data []byte
}

type frozenArchive struct {
	bytes   []byte
	entries []archiveEntry
}

type archiveIdentity struct {
	dev, ino, links uint64
	mode, owner     uint32
}

func archiveIdentityFromStat(st *unix.Stat_t) archiveIdentity {
	return archiveIdentity{
		dev:   uint64(st.Dev),
		ino:   st.Ino,
		links: uint64(st.Nlink),
		mode:  uint32(st.Mode),
		owner: st.Uid,
	}
}

func sameArchiveIdentity(a, b archiveIdentity) bool {
	return a.dev == b.dev && a.ino == b.ino && a.links == b.links && a.mode == b.mode && a.owner == b.owner
}

func zeroBlock(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func exactTarString(field []byte) (string, bool) {
	for i, b := range field {
		if b != 0 {
			continue
		}
		if !zeroBlock(field[i:]) {
			return "", false
		}
		return string(field[:i]), true
	}
	return string(field), true
}

func validateArchiveName(name string, exact, normalized map[string]struct{}) (string, string) {
	clean := path.Clean(name)
	if _, exists := normalized[clean]; exists {
		if _, duplicate := exact[name]; duplicate {
			return "", "duplicate"
		}
		return "", "duplicate-normalized"
	}
	if name == "" || strings.ContainsRune(name, 0) {
		return "", "empty-name"
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", "absolute-name"
	}
	if strings.Contains(name, "\\") {
		return "", "ambiguous-or-alias-name"
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == ".." {
			return "", "traversal-name"
		}
		if part == "" || part == "." {
			return "", "ambiguous-or-alias-name"
		}
	}
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", "traversal-name"
	}
	if clean != name {
		return "", "ambiguous-or-alias-name"
	}
	exact[name] = struct{}{}
	normalized[clean] = struct{}{}
	return clean, ""
}

type ArchiveProvenance struct {
	Fixture string
	SHA256  string
}

const reviewedStateArchiveFixture = "state-proof-reviewed-archive-v1"
const reviewedStateArchiveSHA256 = "9273063b4c2c56372c3da3695c1896f8b15c5ac597a9bbccfce86f5edd6d0d00"

// StateProofArchiveProvenance returns the independently reviewed fixture
// identity and pinned digest. It is deliberately not derived from caller bytes.
func StateProofArchiveProvenance() ArchiveProvenance {
	return ArchiveProvenance{Fixture: reviewedStateArchiveFixture, SHA256: reviewedStateArchiveSHA256}
}

func validateTarIntegrity(blob []byte, expectedSHA256 string, provenanceVerified bool, lim ArchiveLimits) (frozenArchive, ArchiveObservation) {
	var o ArchiveObservation
	o.ProvenanceVerified = provenanceVerified
	frozen := bytes.Clone(blob)
	sum := sha256.Sum256(frozen)
	expected, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(expected) != sha256.Size || subtle.ConstantTimeCompare(sum[:], expected) != 1 {
		o.Rejection = "integrity-before-parse"
		return frozenArchive{}, o
	}
	o.IntegrityVerified = true
	if lim.EntryBytes < 0 || lim.TotalBytes < 0 || lim.Count <= 0 || lim.Depth <= 0 {
		o.Rejection = "invalid-limits"
		return frozenArchive{}, o
	}
	if len(frozen) < 1024 || len(frozen)%512 != 0 || !zeroBlock(frozen[len(frozen)-1024:]) {
		o.Rejection = "malformed-framing"
		return frozenArchive{}, o
	}

	reader := bytes.NewReader(frozen)
	tr := tar.NewReader(reader)
	o.Parsed = true
	exact := make(map[string]struct{})
	normalized := make(map[string]struct{})
	var entries []archiveEntry
	var rawOffset int64
	for {
		hdr, nextErr := tr.Next()
		if nextErr == io.EOF {
			if reader.Len() != 0 || rawOffset != int64(len(frozen)-1024) {
				o.Rejection = "trailing-input"
				return frozenArchive{}, o
			}
			break
		}
		if nextErr != nil && !(hdr != nil && errors.Is(nextErr, tar.ErrInsecurePath)) {
			o.Rejection = "partial-or-corrupt"
			return frozenArchive{}, o
		}
		if rawOffset+512 > int64(len(frozen)-1024) {
			o.Rejection = "partial-or-corrupt"
			return frozenArchive{}, o
		}
		rawType := frozen[rawOffset+156]
		switch rawType {
		case tar.TypeReg, tar.TypeDir:
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink, tar.TypeGNUSparse:
			o.Rejection = "unsupported-format-or-metadata"
			return frozenArchive{}, o
		default:
			o.Rejection = "link-device-socket-or-unsupported-type"
			return frozenArchive{}, o
		}
		if hdr.Typeflag != rawType {
			o.Rejection = "link-device-socket-or-unsupported-type"
			return frozenArchive{}, o
		}
		rawName, exactName := exactTarString(frozen[rawOffset : rawOffset+100])
		rawPrefix, exactPrefix := exactTarString(frozen[rawOffset+345 : rawOffset+500])
		if rawPrefix != "" {
			rawName = rawPrefix + "/" + rawName
		}
		if !exactName || !exactPrefix || rawName != hdr.Name {
			o.Rejection = "ambiguous-or-alias-name"
			return frozenArchive{}, o
		}
		rawLink, exactLink := exactTarString(frozen[rawOffset+157 : rawOffset+257])
		if !exactLink || rawLink != hdr.Linkname {
			o.Rejection = "unsupported-format-or-metadata"
			return frozenArchive{}, o
		}
		name, rejection := validateArchiveName(hdr.Name, exact, normalized)
		if rejection != "" {
			o.Rejection = rejection
			return frozenArchive{}, o
		}
		o.Entries++
		if o.Entries > lim.Count {
			o.Rejection = "count"
			return frozenArchive{}, o
		}
		if strings.Count(name, "/")+1 > lim.Depth {
			o.Rejection = "depth"
			return frozenArchive{}, o
		}
		if hdr.Format != tar.FormatUSTAR || len(hdr.PAXRecords) != 0 || len(hdr.Xattrs) != 0 {
			o.Rejection = "unsupported-format-or-metadata"
			return frozenArchive{}, o
		}
		entry := archiveEntry{name: name}
		switch hdr.Typeflag {
		case tar.TypeReg:
			if hdr.Mode != 0600 && hdr.Mode != 0700 {
				o.Rejection = "unsupported-mode"
				return frozenArchive{}, o
			}
			entry.mode = uint32(hdr.Mode)
		case tar.TypeDir:
			if hdr.Mode != 0700 {
				o.Rejection = "unsupported-mode"
				return frozenArchive{}, o
			}
			if hdr.Size != 0 {
				o.Rejection = "directory-size"
				return frozenArchive{}, o
			}
			entry.mode = 0700
			entry.dir = true
		default:
			o.Rejection = "link-device-socket-or-unsupported-type"
			return frozenArchive{}, o
		}
		if hdr.Linkname != "" {
			o.Rejection = "unsupported-format-or-metadata"
			return frozenArchive{}, o
		}
		if hdr.Size < 0 || hdr.Size > lim.EntryBytes {
			o.Rejection = "entry-size"
			return frozenArchive{}, o
		}
		if hdr.Size > lim.TotalBytes-o.ExpandedBytes {
			o.Rejection = "total-size"
			return frozenArchive{}, o
		}
		if hdr.Size > int64(len(frozen)) {
			o.Rejection = "partial-or-corrupt"
			return frozenArchive{}, o
		}
		o.ExpandedBytes += hdr.Size
		if hdr.Size > 0 {
			entry.data = make([]byte, int(hdr.Size))
			if _, err = io.ReadFull(tr, entry.data); err != nil {
				o.Rejection = "partial-or-corrupt"
				return frozenArchive{}, o
			}
		}
		rawOffset += 512 + ((hdr.Size+511)/512)*512
		if rawOffset > int64(len(frozen)-1024) {
			o.Rejection = "partial-or-corrupt"
			return frozenArchive{}, o
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		o.Rejection = "empty-archive"
		return frozenArchive{}, o
	}
	tree := make(map[string]archiveEntry, len(entries))
	for _, entry := range entries {
		tree[entry.name] = entry
	}
	for _, entry := range entries {
		parent := path.Dir(entry.name)
		if parent == "." {
			continue
		}
		parentEntry, ok := tree[parent]
		if !ok || !parentEntry.dir {
			o.Rejection = "missing-or-nondirectory-parent"
			return frozenArchive{}, o
		}
	}
	o.Prevalidated = true
	return frozenArchive{bytes: frozen, entries: entries}, o
}

// ValidateTarIntegrity proves generic digest-before-parser ordering. It does not
// claim authenticity or provenance because the expected digest is caller data.
func ValidateTarIntegrity(blob []byte, expectedSHA256 string, lim ArchiveLimits) ArchiveObservation {
	_, o := validateTarIntegrity(blob, expectedSHA256, false, lim)
	return o
}

func authenticateTar(blob []byte, provenance ArchiveProvenance, lim ArchiveLimits) (frozenArchive, ArchiveObservation) {
	if provenance != StateProofArchiveProvenance() {
		return frozenArchive{}, ArchiveObservation{Rejection: "provenance-before-parse"}
	}
	return validateTarIntegrity(blob, reviewedStateArchiveSHA256, true, lim)
}

func hitArchiveBoundary(o *ArchiveObservation, opt ArchiveOptions, boundary ArchiveBoundary) error {
	o.Boundaries++
	o.LastBoundary = boundary
	switch boundary {
	case ArchiveCreateStage, ArchiveCreateDir, ArchiveCreateFile:
		o.Creates++
	case ArchiveWriteFile:
		o.Writes++
	case ArchiveFileSync:
		o.FileSyncs++
	case ArchiveDirectorySync:
		o.DirectorySyncs++
	case ArchiveRename:
		o.Renames++
	case ArchiveParentSync:
		o.ParentSyncs++
	case ArchiveReopen:
		o.Reopens++
	}
	if opt.Fail != nil {
		return opt.Fail(boundary)
	}
	return nil
}

func makeArchiveStage(parentfd int) (string, int, archiveIdentity, error) {
	for range 8 {
		random := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, random); err != nil {
			return "", -1, archiveIdentity{}, err
		}
		name := ArchiveStagingPrefix + hex.EncodeToString(random)
		if err := unix.Mkdirat(parentfd, name, 0700); err != nil {
			if errors.Is(err, syscall.EEXIST) {
				continue
			}
			return "", -1, archiveIdentity{}, err
		}
		if err := unix.Fchmodat(parentfd, name, 0700, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			_ = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR)
			return "", -1, archiveIdentity{}, err
		}
		fd, err := unix.Openat(parentfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			_ = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR)
			return "", -1, archiveIdentity{}, err
		}
		if err = unix.Fchmod(fd, 0700); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR)
			return "", -1, archiveIdentity{}, err
		}
		var st unix.Stat_t
		if err = unix.Fstat(fd, &st); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR)
			return "", -1, archiveIdentity{}, err
		}
		identity := archiveIdentityFromStat(&st)
		if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 || st.Uid != uint32(os.Geteuid()) {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR)
			return "", -1, archiveIdentity{}, errors.New("unsafe archive staging directory")
		}
		return name, fd, identity, nil
	}
	return "", -1, archiveIdentity{}, errors.New("archive staging name collisions")
}

func openArchiveDirAt(rootfd int, rel string) (int, error) {
	fd, err := unix.Openat(rootfd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if rel == "." || rel == "" {
		return fd, nil
	}
	for _, component := range strings.Split(rel, "/") {
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	return fd, nil
}

func sortedArchiveEntries(entries []archiveEntry, directories bool) []archiveEntry {
	out := make([]archiveEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.dir == directories {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		leftDepth := strings.Count(out[i].name, "/")
		rightDepth := strings.Count(out[j].name, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return out[i].name < out[j].name
	})
	return out
}

func createArchiveDirectory(rootfd int, entry archiveEntry) (archiveIdentity, error) {
	parent, base := path.Split(entry.name)
	parentfd, err := openArchiveDirAt(rootfd, strings.TrimSuffix(parent, "/"))
	if err != nil {
		return archiveIdentity{}, err
	}
	defer unix.Close(parentfd)
	if err = unix.Mkdirat(parentfd, base, 0700); err != nil {
		return archiveIdentity{}, err
	}
	if err = unix.Fchmodat(parentfd, base, 0700, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return archiveIdentity{}, err
	}
	fd, err := unix.Openat(parentfd, base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return archiveIdentity{}, err
	}
	defer unix.Close(fd)
	if err = unix.Fchmod(fd, 0700); err != nil {
		return archiveIdentity{}, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return archiveIdentity{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0777 != 0700 || st.Uid != uint32(os.Geteuid()) {
		return archiveIdentity{}, errors.New("created archive directory failed verification")
	}
	return archiveIdentityFromStat(&st), nil
}

func createArchiveFile(rootfd int, entry archiveEntry, o *ArchiveObservation, opt ArchiveOptions) (archiveIdentity, error) {
	parent, base := path.Split(entry.name)
	parentfd, err := openArchiveDirAt(rootfd, strings.TrimSuffix(parent, "/"))
	if err != nil {
		return archiveIdentity{}, err
	}
	defer unix.Close(parentfd)
	fd, err := unix.Openat(parentfd, base, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, entry.mode)
	if err != nil {
		return archiveIdentity{}, err
	}
	defer unix.Close(fd)
	if err = unix.Fchmod(fd, entry.mode); err != nil {
		return archiveIdentity{}, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return archiveIdentity{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != entry.mode || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return archiveIdentity{}, errors.New("created archive file failed verification")
	}
	identity := archiveIdentityFromStat(&st)
	if err = hitArchiveBoundary(o, opt, ArchiveCreateFile); err != nil {
		return archiveIdentity{}, err
	}
	for offset := 0; offset < len(entry.data); {
		n, writeErr := unix.Write(fd, entry.data[offset:])
		if writeErr != nil {
			return archiveIdentity{}, writeErr
		}
		if n == 0 {
			return archiveIdentity{}, io.ErrShortWrite
		}
		offset += n
	}
	if err = hitArchiveBoundary(o, opt, ArchiveWriteFile); err != nil {
		return archiveIdentity{}, err
	}
	if err = unix.Fsync(fd); err != nil {
		return archiveIdentity{}, err
	}
	if err = hitArchiveBoundary(o, opt, ArchiveFileSync); err != nil {
		return archiveIdentity{}, err
	}
	return identity, nil
}

func refreshArchiveIdentity(rootfd int, rel string) (archiveIdentity, error) {
	fd, err := openArchiveDirAt(rootfd, rel)
	if err != nil {
		return archiveIdentity{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return archiveIdentity{}, err
	}
	return archiveIdentityFromStat(&st), nil
}

func readArchiveDirectory(fd int) ([]string, error) {
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "archive-directory")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("archive directory descriptor unavailable")
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	sort.Strings(names)
	return names, nil
}

func readArchiveFile(fd int, size int) ([]byte, error) {
	data := make([]byte, size+1)
	offset := 0
	for offset < len(data) {
		n, err := unix.Read(fd, data[offset:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		offset += n
	}
	return data[:offset], nil
}

func verifyArchiveTreeAt(parentfd int, name string, archive frozenArchive, expected map[string]archiveIdentity, rootExpected *archiveIdentity, onReopen func() error) error {
	rootfd, err := unix.Openat(parentfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(rootfd)
	if onReopen != nil {
		if err = onReopen(); err != nil {
			return err
		}
	}
	var rootStat unix.Stat_t
	if err = unix.Fstat(rootfd, &rootStat); err != nil {
		return err
	}
	rootIdentity := archiveIdentityFromStat(&rootStat)
	if rootStat.Mode&unix.S_IFMT != unix.S_IFDIR || rootStat.Mode&0777 != 0700 || rootStat.Uid != uint32(os.Geteuid()) {
		return errors.New("published archive root failed verification")
	}
	if rootExpected != nil && !sameArchiveIdentity(rootIdentity, *rootExpected) {
		return errors.New("published archive root identity changed")
	}

	byName := make(map[string]archiveEntry, len(archive.entries))
	children := make(map[string][]string)
	for _, entry := range archive.entries {
		byName[entry.name] = entry
		parent := path.Dir(entry.name)
		children[parent] = append(children[parent], path.Base(entry.name))
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	seenInodes := map[[2]uint64]struct{}{{rootIdentity.dev, rootIdentity.ino}: {}}
	var verifyDirectory func(int, string) error
	verifyDirectory = func(dirfd int, rel string) error {
		actual, listErr := readArchiveDirectory(dirfd)
		if listErr != nil {
			return listErr
		}
		want := children[rel]
		if len(actual) != len(want) {
			return errors.New("published archive contains an unexpected tree entry")
		}
		for i := range actual {
			if actual[i] != want[i] {
				return errors.New("published archive tree names mismatch")
			}
		}
		for _, base := range want {
			entryName := base
			if rel != "." {
				entryName = rel + "/" + base
			}
			entry := byName[entryName]
			flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
			if entry.dir {
				flags |= unix.O_DIRECTORY
			}
			fd, openErr := unix.Openat(dirfd, base, flags, 0)
			if openErr != nil {
				return openErr
			}
			if onReopen != nil {
				if openErr = onReopen(); openErr != nil {
					_ = unix.Close(fd)
					return openErr
				}
			}
			var st unix.Stat_t
			statErr := unix.Fstat(fd, &st)
			if statErr != nil {
				_ = unix.Close(fd)
				return statErr
			}
			identity := archiveIdentityFromStat(&st)
			key := [2]uint64{identity.dev, identity.ino}
			if identity.dev != rootIdentity.dev {
				_ = unix.Close(fd)
				return errors.New("published archive crossed filesystems")
			}
			if _, duplicate := seenInodes[key]; duplicate {
				_ = unix.Close(fd)
				return errors.New("published archive contains an inode alias")
			}
			seenInodes[key] = struct{}{}
			if identity.owner != uint32(os.Geteuid()) || identity.mode&0777 != entry.mode {
				_ = unix.Close(fd)
				return errors.New("published archive mode or owner mismatch")
			}
			if wantedIdentity, ok := expected[entryName]; ok && !sameArchiveIdentity(identity, wantedIdentity) {
				_ = unix.Close(fd)
				return errors.New("published archive entry identity changed")
			}
			if entry.dir {
				if st.Mode&unix.S_IFMT != unix.S_IFDIR {
					_ = unix.Close(fd)
					return errors.New("published archive directory type mismatch")
				}
				statErr = verifyDirectory(fd, entryName)
				_ = unix.Close(fd)
				if statErr != nil {
					return statErr
				}
				continue
			}
			if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Size != int64(len(entry.data)) {
				_ = unix.Close(fd)
				return errors.New("published archive file topology mismatch")
			}
			got, readErr := readArchiveFile(fd, len(entry.data))
			_ = unix.Close(fd)
			if readErr != nil {
				return readErr
			}
			if !bytes.Equal(got, entry.data) {
				return errors.New("published archive file bytes mismatch")
			}
		}
		return nil
	}
	return verifyDirectory(rootfd, ".")
}

func removeArchiveTreeFD(dirfd int) error {
	names, err := readArchiveDirectory(dirfd)
	if err != nil {
		return err
	}
	for _, name := range names {
		var st unix.Stat_t
		if err = unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			child, openErr := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if openErr != nil {
				return openErr
			}
			openErr = removeArchiveTreeFD(child)
			_ = unix.Close(child)
			if openErr != nil {
				return openErr
			}
			if err = unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
		case unix.S_IFREG:
			if err = unix.Unlinkat(dirfd, name, 0); err != nil {
				return err
			}
		default:
			return errors.New("refusing to clean unverified archive staging entry")
		}
	}
	return unix.Fsync(dirfd)
}

func cleanupArchiveStage(parentfd int, name string, expected archiveIdentity) (cleaned, identityPreserved bool, err error) {
	fd, err := unix.Openat(parentfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, true, nil
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return false, false, err
	}
	current := archiveIdentityFromStat(&st)
	if current.dev != expected.dev || current.ino != expected.ino || current.mode&0777 != 0700 || current.owner != expected.owner {
		_ = unix.Close(fd)
		return false, true, nil
	}
	if err = removeArchiveTreeFD(fd); err != nil {
		_ = unix.Close(fd)
		return false, false, err
	}
	if err = unix.Close(fd); err != nil {
		return false, false, err
	}
	if err = unix.Unlinkat(parentfd, name, unix.AT_REMOVEDIR); err != nil {
		return false, false, err
	}
	if err = unix.Fsync(parentfd); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// ExtractAuthenticatedTarAt freezes and verifies independently pinned reviewed
// provenance plus complete-input integrity before constructing a tar reader. It
// fully validates and copies the archive into a private plan before performing
// descriptor-relative extraction and durable, no-replace publication.
func ExtractAuthenticatedTarAt(parentfd int, publishName string, blob []byte, provenance ArchiveProvenance, lim ArchiveLimits, opt ArchiveOptions) (o ArchiveObservation) {
	if _, err := validateRel(publishName); err != nil || strings.Contains(publishName, "/") {
		o.Rejection = "invalid-publication-name"
		return o
	}
	archive, validated := authenticateTar(blob, provenance, lim)
	o = validated
	if o.Rejection != "" {
		return o
	}

	stageName, stagefd, stageIdentity, err := makeArchiveStage(parentfd)
	if err != nil {
		o.Rejection = "staging-create"
		return o
	}
	defer func() {
		_ = unix.Close(stagefd)
		if o.Published {
			return
		}
		cleaned, preserved, cleanupErr := cleanupArchiveStage(parentfd, stageName, stageIdentity)
		o.Cleaned = cleaned
		o.CleanupIdentityPreserved = preserved
		if cleanupErr != nil {
			o.CleanupFailed = true
			if o.Rejection == "" {
				o.Rejection = "staging-cleanup"
			}
		}
	}()
	if err = hitArchiveBoundary(&o, opt, ArchiveCreateStage); err != nil {
		o.Rejection = "injected-failure"
		return o
	}

	identities := make(map[string]archiveIdentity, len(archive.entries))
	directories := sortedArchiveEntries(archive.entries, true)
	for _, entry := range directories {
		identity, createErr := createArchiveDirectory(stagefd, entry)
		if createErr != nil {
			o.Rejection = "directory-create"
			return o
		}
		identities[entry.name] = identity
		if err = hitArchiveBoundary(&o, opt, ArchiveCreateDir); err != nil {
			o.Rejection = "injected-failure"
			return o
		}
	}
	for _, entry := range sortedArchiveEntries(archive.entries, false) {
		identity, createErr := createArchiveFile(stagefd, entry, &o, opt)
		if createErr != nil {
			if o.LastBoundary == ArchiveCreateFile || o.LastBoundary == ArchiveWriteFile || o.LastBoundary == ArchiveFileSync {
				o.Rejection = "injected-failure"
			} else {
				o.Rejection = "file-create-write-or-sync"
			}
			return o
		}
		identities[entry.name] = identity
	}
	for i := len(directories) - 1; i >= 0; i-- {
		fd, openErr := openArchiveDirAt(stagefd, directories[i].name)
		if openErr != nil {
			o.Rejection = "directory-reopen-for-sync"
			return o
		}
		syncErr := unix.Fsync(fd)
		_ = unix.Close(fd)
		if syncErr != nil {
			o.Rejection = "directory-sync"
			return o
		}
		if err = hitArchiveBoundary(&o, opt, ArchiveDirectorySync); err != nil {
			o.Rejection = "injected-failure"
			return o
		}
	}
	if err = unix.Fsync(stagefd); err != nil {
		o.Rejection = "directory-sync"
		return o
	}
	if err = hitArchiveBoundary(&o, opt, ArchiveDirectorySync); err != nil {
		o.Rejection = "injected-failure"
		return o
	}
	for _, entry := range directories {
		identity, refreshErr := refreshArchiveIdentity(stagefd, entry.name)
		if refreshErr != nil {
			o.Rejection = "identity-refresh"
			return o
		}
		identities[entry.name] = identity
	}
	stageIdentity, err = refreshArchiveIdentity(stagefd, ".")
	if err != nil {
		o.Rejection = "identity-refresh"
		return o
	}
	o.Extracted = true

	if err = unix.Renameat2(parentfd, stageName, parentfd, publishName, unix.RENAME_NOREPLACE); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			o.Rejection = "publish-no-replace"
		} else {
			o.Rejection = "publication-rename"
		}
		return o
	}
	o.Published = true
	if err = hitArchiveBoundary(&o, opt, ArchiveRename); err != nil {
		o.Rejection = "injected-failure"
		return o
	}
	if err = unix.Fsync(parentfd); err != nil {
		o.Rejection = "publication-parent-sync"
		return o
	}
	o.ParentSynced = true
	if err = hitArchiveBoundary(&o, opt, ArchiveParentSync); err != nil {
		o.Rejection = "injected-failure"
		return o
	}
	firstReopen := true
	err = verifyArchiveTreeAt(parentfd, publishName, archive, identities, &stageIdentity, func() error {
		if firstReopen {
			o.Reopened = true
			firstReopen = false
		}
		return hitArchiveBoundary(&o, opt, ArchiveReopen)
	})
	if err != nil {
		if o.LastBoundary == ArchiveReopen {
			o.Rejection = "injected-failure"
		} else {
			o.Rejection = "reopen-verification"
		}
		return o
	}
	o.Verified = true
	return o
}

// VerifyAuthenticatedTarAt is the restart/recovery side of extraction. It
// verifies the same independently pinned provenance and frozen input, then
// descriptor-relatively reopens and verifies the published exact tree.
func VerifyAuthenticatedTarAt(parentfd int, publishName string, blob []byte, provenance ArchiveProvenance, lim ArchiveLimits) (o ArchiveObservation) {
	if _, err := validateRel(publishName); err != nil || strings.Contains(publishName, "/") {
		o.Rejection = "invalid-publication-name"
		return o
	}
	archive, validated := authenticateTar(blob, provenance, lim)
	o = validated
	if o.Rejection != "" {
		return o
	}
	firstReopen := true
	err := verifyArchiveTreeAt(parentfd, publishName, archive, nil, nil, func() error {
		o.Boundaries++
		o.Reopens++
		o.LastBoundary = ArchiveReopen
		if firstReopen {
			o.Reopened = true
			firstReopen = false
		}
		return nil
	})
	if err != nil {
		o.Rejection = "recovery-verification"
		return o
	}
	o.Verified = true
	return o
}
