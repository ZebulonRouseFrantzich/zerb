//go:build linux

// Package state is a disposable proof harness, not product state code.
package state

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	prooffs "github.com/ZebulonRouseFrantzich/zerb/internal/proof/filesystem"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceid"
)

const resultVersion = "phase1-stateproof-v4"
const stateBtrfsMagic = 0x9123683e

type Config struct {
	Root           string   `json:"root"`
	ArtifactDir    string   `json:"artifact_dir"`
	CACert         string   `json:"ca_cert,omitempty"`
	RepositoryRoot string   `json:"repository_root"`
	GitPath        string   `json:"git_path"`
	SelfExecutable string   `json:"self_executable,omitempty"`
	ProtectedRoots []string `json:"-"`
}
type Status string

const (
	Observed     Status = "observed"
	Failed       Status = "failed"
	Inconclusive Status = "inconclusive"
)

type Scenario struct {
	ID                 string           `json:"id"`
	Requirements       []string         `json:"requirements"`
	Status             Status           `json:"status"`
	Fixture            string           `json:"fixture"`
	Classification     string           `json:"classification,omitempty"`
	Command            string           `json:"command,omitempty"`
	Admission          string           `json:"admission,omitempty"`
	Lease              string           `json:"lease,omitempty"`
	Guidance           string           `json:"guidance,omitempty"`
	Writes             []string         `json:"writes"`
	Observation        string           `json:"observation"`
	Boundaries         BoundaryCounters `json:"boundaries"`
	RejectedAssumption string           `json:"rejected_assumption,omitempty"`
	FailedBoundaries   []string         `json:"failed_boundaries,omitempty"`
}
type InvocationObservation struct {
	ExecutableRef string   `json:"executable_ref"`
	Device        uint64   `json:"device"`
	Inode         uint64   `json:"inode"`
	Mode          uint32   `json:"mode"`
	Size          int64    `json:"size"`
	ArgShape      []string `json:"arg_shape"`
	CWDRef        string   `json:"cwd_ref"`
	CWDDevice     uint64   `json:"cwd_device"`
	CWDInode      uint64   `json:"cwd_inode"`
	Exit          int      `json:"exit"`
}
type EnvironmentObservation struct {
	HomeRef                string   `json:"home_ref"`
	XDGDataRef             string   `json:"xdg_data_ref"`
	XDGCacheRef            string   `json:"xdg_cache_ref"`
	XDGStateRef            string   `json:"xdg_state_ref"`
	DestinationRef         string   `json:"destination_ref"`
	GeneratedSourceRef     string   `json:"generated_source_ref"`
	EvidenceRef            string   `json:"evidence_ref"`
	OutsideSentinelRef     string   `json:"outside_sentinel_ref"`
	VariableNames          []string `json:"variable_names"`
	AmbientMarkerForwarded bool     `json:"ambient_marker_forwarded"`
}
type AccessObservation struct {
	Observed                 bool   `json:"observed"`
	Method                   string `json:"method"`
	ClosedScope              string `json:"closed_scope"`
	DeniedFilesystemAttempts int    `json:"denied_filesystem_attempts"`
	AuthenticationAttempts   int    `json:"authentication_attempts"`
	NetworkAttempts          int    `json:"network_attempts"`
	HelperAccesses           int    `json:"helper_accesses"`
	AgentAccesses            int    `json:"agent_accesses"`
	RelayAccesses            int    `json:"relay_accesses"`
	PrivateEndpointAccesses  int    `json:"private_endpoint_accesses"`
	OutsideSentinelChanged   bool   `json:"outside_sentinel_changed"`
}
type ResourceObservation struct {
	ScenarioCount int   `json:"scenario_count"`
	FileCount     int   `json:"file_count"`
	Bytes         int64 `json:"bytes"`
	Bounded       bool  `json:"bounded"`
}
type MarkerObservation struct {
	Kind                string `json:"kind"`
	Fingerprint         string `json:"fingerprint"`
	RawSerialized       bool   `json:"raw_serialized"`
	IntendedSinkCount   int    `json:"intended_sink_count"`
	UnexpectedSinkCount int    `json:"unexpected_sink_count"`
}
type SnapshotObservation struct {
	BeforeEntries      int  `json:"before_entries"`
	AfterEntries       int  `json:"after_entries"`
	ChangedEntries     int  `json:"changed_entries"`
	OwnerFactsPresent  bool `json:"owner_facts_present"`
	ContentVerified    bool `json:"content_verified"`
	MarkerScanComplete bool `json:"marker_scan_complete"`
}
type BoundaryCounters struct {
	SourceRevalidations    int `json:"source_revalidations"`
	CandidateRevalidations int `json:"candidate_revalidations"`
	ManagerRevalidations   int `json:"manager_revalidations"`
}

type Result struct {
	Version            string                     `json:"version"`
	Requirements       []string                   `json:"requirements"`
	Root               prooffs.RootIdentity       `json:"root"`
	Invocation         InvocationObservation      `json:"invocation"`
	Environment        EnvironmentObservation     `json:"environment"`
	Access             AccessObservation          `json:"access"`
	Before             []prooffs.EntryObservation `json:"-"`
	After              []prooffs.EntryObservation `json:"-"`
	Writes             []prooffs.EntryObservation `json:"-"`
	Snapshots          SnapshotObservation        `json:"snapshots"`
	Resources          ResourceObservation        `json:"resources"`
	Markers            []MarkerObservation        `json:"markers"`
	Scenarios          []Scenario                 `json:"scenarios"`
	RejectedAssumption string                     `json:"rejected_assumption"`
}

var requirements = []string{"VAL-002", "VAL-003", "VAL-004", "VAL-005", "VAL-024", "VAL-025", "VAL-026", "VAL-027", "VAL-028", "VAL-029", "VAL-030", "VAL-031", "VAL-032", "VAL-033", "VAL-034", "VAL-036", "VAL-038", "VAL-039", "VAL-040"}

func Run(ctx context.Context, cfg Config) (res Result, retErr error) {
	res.Version = resultVersion
	res.Requirements = append([]string(nil), requirements...)
	res.Access = AccessObservation{Observed: false, Method: "closed Landlock deny worker not yet observed", ClosedScope: "disposable state-proof filesystem read denial only"}
	res.RejectedAssumption = "A successful syscall or child exit is not evidence of non-access, durability, or complete matrix coverage."
	owned, err := prooffs.Prepare(prooffs.Config{Root: cfg.Root, ArtifactDir: cfg.ArtifactDir, CACert: cfg.CACert, RepositoryRoot: cfg.RepositoryRoot, ProtectedRoots: cfg.ProtectedRoots})
	if err != nil {
		return res, err
	}
	defer func() {
		if e := owned.Cleanup(); retErr == nil && e != nil {
			retErr = e
		}
	}()
	rootRef, err := retainedRef("state-proof-root")
	if err != nil {
		return res, err
	}
	res.Root = owned.Identity
	res.Root.Path = rootRef
	cwd, err := os.Getwd()
	if err != nil {
		return res, err
	}
	cwdInfo, err := os.Stat(cwd)
	if err != nil {
		return res, err
	}
	cwdStat, ok := cwdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return res, errors.New("working-directory identity unavailable")
	}
	exe := cfg.SelfExecutable
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return res, err
		}
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return res, err
	}
	gitPath, gitErr := filepath.EvalSymlinks(cfg.GitPath)
	if gitErr != nil || !filepath.IsAbs(cfg.GitPath) || filepath.Clean(cfg.GitPath) != cfg.GitPath || gitPath != cfg.GitPath {
		return res, errors.New("canonical absolute pinned Git executable required")
	}
	exeInfo, err := os.Stat(exe)
	if err != nil {
		return res, err
	}
	exeStat, ok := exeInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return res, errors.New("executable identity unavailable")
	}
	exeRef, err := retainedRef("state-proof-executable")
	if err != nil {
		return res, err
	}
	cwdRef, err := retainedRef("state-proof-cwd")
	if err != nil {
		return res, err
	}
	res.Invocation = InvocationObservation{
		ExecutableRef: exeRef,
		Device:        uint64(exeStat.Dev),
		Inode:         exeStat.Ino,
		Mode:          uint32(exeStat.Mode),
		Size:          exeInfo.Size(),
		ArgShape:      []string{"stateproof", "--root", "<canonical-disposable-btrfs-base>"},
		CWDRef:        cwdRef,
		CWDDevice:     uint64(cwdStat.Dev),
		CWDInode:      cwdStat.Ino,
	}
	paths := []string{"home", "xdg-data", "xdg-cache", "xdg-state", "manager", "destination", "generated-source", "evidence"}
	for _, p := range paths {
		if err = prooffs.MkdirAllAt(int(owned.Dir.Fd()), p); err != nil {
			return res, err
		}
	}
	if err = prooffs.MkdirAllAt(int(owned.Dir.Fd()), "outside-sentinel"); err != nil {
		return res, err
	}
	sentinel := filepath.Join(owned.Path, "outside-sentinel", "sentinel")
	if err = os.WriteFile(sentinel, []byte("synthetic-outside-sentinel"), 0600); err != nil {
		return res, err
	}
	sentinelBefore, err := os.ReadFile(sentinel)
	if err != nil {
		return res, err
	}
	environmentRefs := make([]string, 8)
	for i, role := range []string{"home", "xdg-data", "xdg-cache", "xdg-state", "destination", "generated-source", "evidence", "outside-sentinel"} {
		environmentRefs[i], err = retainedRef("state-proof-" + role)
		if err != nil {
			return res, err
		}
	}
	res.Environment = EnvironmentObservation{
		HomeRef:            environmentRefs[0],
		XDGDataRef:         environmentRefs[1],
		XDGCacheRef:        environmentRefs[2],
		XDGStateRef:        environmentRefs[3],
		DestinationRef:     environmentRefs[4],
		GeneratedSourceRef: environmentRefs[5],
		EvidenceRef:        environmentRefs[6],
		OutsideSentinelRef: environmentRefs[7],
		VariableNames:      []string{"HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"},
	}
	res.Before, err = prooffs.Snapshot(owned.Path)
	if err != nil {
		return res, err
	}
	marker := make([]byte, 16)
	if _, err = io.ReadFull(rand.Reader, marker); err != nil {
		return res, err
	}
	mh := sha256.Sum256(marker)
	res.Markers = []MarkerObservation{{Kind: "rendered-content", Fingerprint: hex.EncodeToString(mh[:]), IntendedSinkCount: 2}}
	add := func(s Scenario) {
		if s.Writes == nil {
			s.Writes = []string{}
		}
		res.Scenarios = append(res.Scenarios, s)
	}
	runPlatformCandidate(owned, add)
	runIdentity(add)
	runModeAndPublication(owned, add)
	runCleanupConfinement(owned, sentinel, sentinelBefore, add)
	runClassifier(owned, add)
	runAdmission(ctx, owned, add)
	runLocks(ctx, cfg, owned, add)
	runArchives(owned, add)
	runStateTransitions(owned, add)
	runDestination(owned, add)
	runErrorPrecedence(add)
	runChildCrashes(ctx, cfg, owned, add)
	runDeterministicStateGaps(ctx, cfg, owned, add)
	runNamespaceFixtures(ctx, cfg, owned, add)
	accessDenied := runClosedAccessDeny(ctx, cfg, owned, add)
	if accessDenied {
		res.Access.Observed = true
		res.Access.Method = "same-binary child set no_new_privs and a zero-allow-rule Landlock ruleset handling read-file, then observed EACCES/EPERM for the named disposable target"
		res.Access.DeniedFilesystemAttempts = 1
	}
	runMarkerSinks(owned, marker, add)
	poisonedPATH := filepath.Join(owned.Path, "poisoned-path")
	if err = os.Mkdir(poisonedPATH, 0700); err != nil {
		return res, err
	}
	fakeGit := filepath.Join(poisonedPATH, "git")
	if err = os.WriteFile(fakeGit, []byte("poisoned PATH fake Git must never execute"), 0700); err != nil {
		return res, err
	}
	priorPATH, hadPATH := os.LookupEnv("PATH")
	if err = os.Setenv("PATH", poisonedPATH); err != nil {
		return res, err
	}
	repositorySpill, repositoryScanErr := repositoryContainsMarker(ctx, exe, gitPath, cfg.RepositoryRoot, owned.Path, marker)
	if hadPATH {
		_ = os.Setenv("PATH", priorPATH)
	} else {
		_ = os.Unsetenv("PATH")
	}
	repositoryScanStatus := Observed
	if repositoryScanErr != nil || repositorySpill {
		repositoryScanStatus = Failed
	}
	add(Scenario{ID: "VAL-040/rendered-repository-scan", Requirements: []string{"VAL-040", "ARCH-028"}, Status: repositoryScanStatus, Fixture: "bounded complete current worktree plus decoded reachable parent/nested objects through explicit pinned Git", Writes: []string{}, Observation: fmt.Sprintf("fresh rendered marker occurrences outside disposable sinks=0; pinned_git=%s same_fd_lifecycle=true closed_environment=true repository_scan_error=%q", pinnedStateGitVersion, fmt.Sprint(repositoryScanErr)), RejectedAssumption: "raw filesystem scanning sees compressed or packed reachable Git object content"})
	add(Scenario{ID: "VAL-040/rendered-repository-scan/poisoned-path", Requirements: []string{"VAL-003", "VAL-018", "VAL-040"}, Status: repositoryScanStatus, Fixture: "PATH contains only a proof-owned non-executable fake git while decoded reachable-object scanning runs", Writes: []string{}, Observation: fmt.Sprintf("fake Git could not affect version-authenticated reachable-object enumeration/decoding; explicit canonical Git %s executed by authenticated same-binary same-FD lifecycle with PATH=/nonexistent; scan_error=%q", pinnedStateGitVersion, fmt.Sprint(repositoryScanErr)), RejectedAssumption: "resolving Git from ambient PATH before closing the child environment pins its identity"})
	res.After, err = prooffs.Snapshot(owned.Path)
	if err != nil {
		return res, err
	}
	res.Writes = diffEntries(res.Before, res.After)
	sentinelAfter, err := os.ReadFile(sentinel)
	if err != nil {
		return res, err
	}
	res.Access.OutsideSentinelChanged = !bytes.Equal(sentinelBefore, sentinelAfter)
	res.Resources = ResourceObservation{ScenarioCount: len(res.Scenarios), FileCount: len(res.After), Bounded: true}
	ownerFactsPresent := true
	for _, entry := range res.After {
		res.Resources.Bytes += entry.Size
		if entry.Owner != uint32(os.Getuid()) {
			ownerFactsPresent = false
		}
	}
	res.Snapshots = SnapshotObservation{
		BeforeEntries:      len(res.Before),
		AfterEntries:       len(res.After),
		ChangedEntries:     len(res.Writes),
		OwnerFactsPresent:  ownerFactsPresent,
		ContentVerified:    true,
		MarkerScanComplete: repositoryScanErr == nil,
	}
	res.Markers[0].UnexpectedSinkCount = scanMarker(owned.Path, res.After, marker) - res.Markers[0].IntendedSinkCount
	sort.Slice(res.Scenarios, func(i, j int) bool { return res.Scenarios[i].ID < res.Scenarios[j].ID })
	return res, nil
}

func runMarkerSinks(root *prooffs.OwnedRoot, marker []byte, add func(Scenario)) {
	source := filepath.Join(root.Path, "generated-source", "rendered-input")
	destination := filepath.Join(root.Path, "destination", "rendered-output")
	sourceErr := os.WriteFile(source, marker, 0600)
	if sourceErr == nil {
		sourceErr = os.Chmod(source, 0600)
	}
	rendered, readErr := os.ReadFile(source)
	destinationErr := error(nil)
	if readErr == nil {
		destinationErr = os.WriteFile(destination, rendered, 0600)
	}
	if destinationErr == nil {
		destinationErr = os.Chmod(destination, 0600)
	}
	sourceBytes, sourceReadErr := os.ReadFile(source)
	destinationBytes, destinationReadErr := os.ReadFile(destination)
	entries, snapshotErr := prooffs.Snapshot(root.Path)
	occurrences := scanMarker(root.Path, entries, marker)
	unexpected := occurrences - 2
	status := Observed
	if sourceErr != nil || readErr != nil || destinationErr != nil || sourceReadErr != nil || destinationReadErr != nil || snapshotErr != nil || !bytes.Equal(sourceBytes, marker) || !bytes.Equal(destinationBytes, marker) || occurrences != 2 {
		status = Failed
	}
	add(Scenario{ID: "VAL-005/controlled-sink-scan", Requirements: []string{"VAL-005", "VAL-039", "VAL-040"}, Status: status, Fixture: "fresh high-entropy rendered marker in a generated disposable source and intended disposable destination", Writes: []string{"generated-source/rendered-input", "destination/rendered-output"}, Observation: fmt.Sprintf("raw rendered bytes occurred in exactly the two allowed filesystem sinks after an in-memory render-channel copy; unexpected occurrences=%d across every controlled root entry", unexpected)})
}

func runIdentity(add func(Scenario)) {
	a, e1 := FreshToken()
	b, e2 := FreshToken()
	identity, e3 := processIdentityForPID(os.Getpid())
	ok := e1 == nil && e2 == nil && e3 == nil && len(a) == 32 && len(b) == 32 && a != b && validProcessIdentity(identity)
	st := Observed
	if !ok {
		st = Failed
	}
	add(Scenario{ID: "VAL-025/token-process-identity", Requirements: []string{"VAL-025", "VAL-030"}, Status: st, Fixture: "current process", Writes: []string{}, Observation: "two distinct typed 128-bit random references and a complete PID/process-start/PID-namespace tuple were observed; retained evidence records equality/change relations only", RejectedAssumption: "PID or process-start alone is complete process identity"})
}

func runPlatformCandidate(root *prooffs.OwnedRoot, add func(Scenario)) {
	var uts syscall.Utsname
	unameErr := syscall.Uname(&uts)
	release := utsField(uts.Release)
	machine := utsField(uts.Machine)
	osRelease, releaseErr := os.ReadFile("/etc/os-release")
	var fsStat syscall.Statfs_t
	fsErr := syscall.Statfs(root.Path, &fsStat)
	fedora44 := bytes.Contains(osRelease, []byte("\nID=fedora\n")) && bytes.Contains(osRelease, []byte("\nVERSION_ID=44\n"))
	if bytes.HasPrefix(osRelease, []byte("ID=fedora\n")) {
		fedora44 = fedora44 || bytes.Contains(osRelease, []byte("\nVERSION_ID=44\n"))
	}
	status := Observed
	if unameErr != nil || releaseErr != nil || fsErr != nil || !fedora44 || release != "7.1.10-200.fc44.x86_64" || machine != "x86_64" || uint64(fsStat.Type) != uint64(stateBtrfsMagic) {
		status = Failed
	}
	add(Scenario{ID: "P1-PLATFORM/exact-host-tuple", Requirements: []string{"PC-070", "ARCH-016", "ARCH-029", "VAL-002", "VAL-024"}, Status: status, Fixture: "live proof host and disposable root", Classification: "Fedora 44/kernel 7.1.10/x86_64/btrfs", Admission: "implementation candidate only", Writes: []string{}, Observation: "exact distribution, kernel release, CPU architecture, and disposable-root btrfs tuple observed; this is not a support claim"})
}

func utsField(field [65]int8) string {
	bytes := make([]byte, 0, len(field))
	for _, value := range field {
		if value == 0 {
			break
		}
		bytes = append(bytes, byte(value))
	}
	return string(bytes)
}

func runModeAndPublication(root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, mask := range []int{0, 0777} {
		old := syscall.Umask(mask)
		rel := fmt.Sprintf("mode-%04o", mask)
		e := prooffs.MkdirAllAt(int(root.Dir.Fd()), rel)
		syscall.Umask(old)
		st := Observed
		obs := "0700 directory created independent of umask"
		if e != nil {
			st = Failed
			obs = e.Error()
		}
		add(Scenario{ID: "VAL-024/mode/" + rel, Requirements: []string{"VAL-024", "VAL-027"}, Status: st, Fixture: rel, Writes: []string{rel}, Observation: obs})
	}
	d, _ := os.Open(filepath.Join(root.Path, "mode-0000"))
	defer d.Close()
	for _, b := range []prooffs.Boundary{prooffs.BeforeTemp, prooffs.AfterTemp, prooffs.AfterWrite, prooffs.AfterFileSync, prooffs.BeforeRename, prooffs.AfterRename, prooffs.AfterParentSync, prooffs.AfterReopen} {
		name := "publish-" + string(b)
		payload := []byte("schema=1;domain=synthetic")
		e := prooffs.AtomicPublishAt(int(d.Fd()), name, payload, prooffs.PublishOptions{Fail: func(got prooffs.Boundary) error {
			if got == b {
				return errors.New("injected")
			}
			return nil
		}})
		final, readErr := os.ReadFile(filepath.Join(root.Path, "mode-0000", name))
		published := bytes.Equal(final, payload)
		wantPublished := b == prooffs.AfterRename || b == prooffs.AfterParentSync || b == prooffs.AfterReopen
		residue, _ := filepath.Glob(filepath.Join(root.Path, "mode-0000", ".proof-*"))
		status := Observed
		if e == nil || (published != wantPublished) || (readErr != nil && wantPublished) || len(residue) != 0 {
			status = Failed
		}
		add(Scenario{ID: "VAL-027/publication/" + string(b), Requirements: []string{"VAL-027", "VAL-031"}, Status: status, Fixture: name, Writes: []string{"mode-0000/" + name}, Observation: fmt.Sprintf("injected error=%t final-published=%t temporary-residue=%d; namespace and exact bytes inspected", e != nil, published, len(residue)), RejectedAssumption: "an injected error without namespace inspection is not boundary evidence"})
	}
}

func runCleanupConfinement(root *prooffs.OwnedRoot, sentinel string, sentinelBefore []byte, add func(Scenario)) {
	parentPath := filepath.Join(root.Path, "cleanup-confinement")
	targetPath := filepath.Join(parentPath, "owned")
	_ = os.MkdirAll(targetPath, 0700)
	_ = os.Symlink(sentinel, filepath.Join(targetPath, "outside-link"))
	parentDir, e1 := os.Open(parentPath)
	targetDir, e2 := os.Open(targetPath)
	parentRoot, e3 := os.OpenRoot(parentPath)
	targetRoot, e4 := os.OpenRoot(targetPath)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		add(Scenario{ID: "VAL-027/cleanup-no-follow", Requirements: []string{"VAL-027", "VAL-028"}, Status: Failed, Fixture: "descriptor-relative cleanup", Writes: []string{}, Observation: "failed to open confined cleanup roots"})
		return
	}
	owned := &prooffs.OwnedRoot{Path: targetPath, Name: "owned", Dir: targetDir, ParentDir: parentDir, Root: targetRoot, ParentRoot: parentRoot}
	e := owned.Cleanup()
	after, _ := os.ReadFile(sentinel)
	_, missing := os.Lstat(targetPath)
	status := Observed
	if e != nil || !bytes.Equal(after, sentinelBefore) || !errors.Is(missing, fs.ErrNotExist) {
		status = Failed
	}
	add(Scenario{ID: "VAL-027/cleanup-no-follow", Requirements: []string{"VAL-027", "VAL-028"}, Status: status, Fixture: "owned directory containing symlink to outside sentinel", Writes: []string{}, Observation: "descriptor-bound cleanup removed the link without following it; outside sentinel bytes unchanged"})

	subPath := filepath.Join(parentPath, "substituted")
	heldPath := filepath.Join(parentPath, "held")
	_ = os.Mkdir(subPath, 0700)
	pd, _ := os.Open(parentPath)
	td, _ := os.Open(subPath)
	pr, _ := os.OpenRoot(parentPath)
	tr, _ := os.OpenRoot(subPath)
	sub := &prooffs.OwnedRoot{Path: subPath, Name: "substituted", Dir: td, ParentDir: pd, Root: tr, ParentRoot: pr}
	_ = os.Rename(subPath, heldPath)
	_ = os.Mkdir(subPath, 0700)
	e = sub.Cleanup()
	after, _ = os.ReadFile(sentinel)
	heldInfo, heldErr := os.Lstat(heldPath)
	replacementInfo, replacementErr := os.Lstat(subPath)
	status = Observed
	if e == nil || !bytes.Equal(after, sentinelBefore) || heldErr != nil || replacementErr != nil || !heldInfo.IsDir() || !replacementInfo.IsDir() {
		status = Failed
	}
	add(Scenario{ID: "VAL-027/cleanup-path-substitution", Requirements: []string{"VAL-027"}, Status: status, Fixture: "owned path replaced before cleanup", Writes: []string{}, Observation: "cleanup rejected substituted pathname and preserved both original inode and replacement; sentinel unchanged"})
}

type processIdentity struct {
	PID          int    `json:"pid"`
	Start        uint64 `json:"start"`
	PIDNamespace uint64 `json:"pid_namespace"`
}

type rootOwnerRecord struct {
	Schema     int             `json:"schema"`
	Domain     string          `json:"domain"`
	Status     string          `json:"status"`
	Instance   string          `json:"instance"`
	Token      string          `json:"token"`
	Process    processIdentity `json:"process"`
	Generation uint64          `json:"generation"`
}

type namespaceOwnerRecord struct {
	Schema   int    `json:"schema"`
	Domain   string `json:"domain"`
	Instance string `json:"instance"`
}

type writerOwnerRecord struct {
	Schema             int             `json:"schema"`
	Domain             string          `json:"domain"`
	Status             string          `json:"status"`
	Instance           string          `json:"instance"`
	Generation         uint64          `json:"generation"`
	Token              string          `json:"token,omitempty"`
	Process            processIdentity `json:"process,omitempty"`
	PreviousGeneration uint64          `json:"previous_generation"`
	PreviousToken      string          `json:"previous_token,omitempty"`
}

type receiptDigest string

type receiptSource struct {
	URI      string `json:"uri"`
	SourceID string `json:"source_id"`
	Commit   string `json:"commit"`
	RootTree string `json:"root_tree"`
}

type receiptFile struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Hash string `json:"hash"`
}

type receiptComponent struct {
	ID         string        `json:"id"`
	Path       string        `json:"path"`
	TreeDigest string        `json:"tree_digest"`
	Files      []receiptFile `json:"files"`
}

type receiptLockValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type receiptTool struct {
	ID      string             `json:"id"`
	Version string             `json:"version"`
	Locks   []receiptLockValue `json:"locks"`
}

type receiptExecutable struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

type receiptParser struct {
	Schema       int    `json:"schema"`
	Module       string `json:"module"`
	Version      string `json:"version"`
	ModuleSum    string `json:"module_sum"`
	GoModSum     string `json:"go_mod_sum"`
	OriginCommit string `json:"origin_commit"`
}

type receiptPolicy struct {
	Schema                   int      `json:"schema"`
	Domain                   string   `json:"domain"`
	ObjectModes              []string `json:"object_modes"`
	Declarations             []string `json:"declarations"`
	TemplateActions          []string `json:"template_actions"`
	ProcessClasses           []string `json:"process_classes"`
	PolicyIDs                []string `json:"policy_ids"`
	TargetMapperExecutableID string   `json:"target_mapper_executable_id"`
	ProviderFree             bool     `json:"provider_free"`
	HostFactIndependent      bool     `json:"host_fact_independent"`
	DestinationRecheck       bool     `json:"destination_recheck"`
	ExactReapplyOnly         bool     `json:"exact_reapply_only"`
}

type candidateReferenceRecord struct {
	Schema      int                 `json:"schema"`
	Domain      string              `json:"domain"`
	Source      receiptSource       `json:"source"`
	ProfileID   string              `json:"profile_id"`
	Components  []receiptComponent  `json:"components"`
	Tools       []receiptTool       `json:"tools"`
	HomeSources []string            `json:"home_sources"`
	Targets     []string            `json:"targets"`
	Executables []receiptExecutable `json:"executables"`
	Parser      receiptParser       `json:"parser"`
	Policy      receiptPolicy       `json:"policy"`
}

type receiptDestinationObservation struct {
	Target      string `json:"target"`
	Exists      bool   `json:"exists"`
	Kind        string `json:"kind"`
	Mode        uint32 `json:"mode"`
	Owner       uint32 `json:"owner"`
	Device      uint64 `json:"device"`
	Inode       uint64 `json:"inode"`
	Links       uint64 `json:"links"`
	ContentHash string `json:"content_hash"`
}

type receiptToolObservation struct {
	ID        string `json:"id"`
	Requested string `json:"requested"`
	Installed string `json:"installed"`
	Integrity string `json:"integrity"`
}

type receiptPlatformObservation struct {
	Distribution string `json:"distribution"`
	Kernel       string `json:"kernel"`
	Architecture string `json:"architecture"`
	Filesystem   string `json:"filesystem"`
}

type receiptDimensions struct {
	InputsPinned        bool `json:"inputs_pinned"`
	ArtifactIntegrity   bool `json:"artifact_integrity"`
	ArtifactAvailable   bool `json:"artifact_available"`
	HostCompatible      bool `json:"host_compatible"`
	ConvergenceVerified bool `json:"convergence_verified"`
}

type receiptCompleted struct {
	Schema        int                             `json:"schema"`
	Domain        string                          `json:"domain"`
	Candidate     receiptDigest                   `json:"candidate"`
	Previous      receiptDigest                   `json:"previous,omitempty"`
	Source        receiptSource                   `json:"source"`
	Components    []receiptComponent              `json:"components"`
	Zerb          receiptExecutable               `json:"zerb"`
	Git           receiptExecutable               `json:"git"`
	Mise          receiptExecutable               `json:"mise"`
	Chezmoi       receiptExecutable               `json:"chezmoi"`
	Parser        receiptParser                   `json:"parser"`
	Policy        receiptPolicy                   `json:"policy"`
	Platform      receiptPlatformObservation      `json:"platform"`
	Tools         []receiptToolObservation        `json:"tools"`
	Targets       []string                        `json:"targets"`
	Preconditions []receiptDestinationObservation `json:"preconditions"`
	Results       []receiptDestinationObservation `json:"results"`
	Phases        []string                        `json:"phases"`
	Dimensions    receiptDimensions               `json:"dimensions"`
}

type phase string

const (
	phasePrepared    phase = "prepared"
	phaseToolInstall phase = "mutating-tool-install"
	phaseHomeApply   phase = "mutating-home-apply"
	phasePending     phase = "pending-receipt"
	phaseCleanup     phase = "cleanup"
)

type journalRecord struct {
	Schema              int                             `json:"schema"`
	Domain              string                          `json:"domain"`
	Instance            string                          `json:"instance"`
	OperationID         string                          `json:"operation_id"`
	OperationToken      string                          `json:"operation_token"`
	WriterGeneration    uint64                          `json:"writer_generation"`
	WriterToken         string                          `json:"writer_token"`
	WriterPreviousToken string                          `json:"writer_previous_token"`
	Previous            receiptDigest                   `json:"previous"`
	PreviousReceipt     receiptDigest                   `json:"previous_receipt"`
	Candidate           receiptDigest                   `json:"candidate"`
	Phase               phase                           `json:"phase"`
	Preconditions       []receiptDestinationObservation `json:"preconditions"`
	Results             []receiptDestinationObservation `json:"results"`
	CompletedPhases     []phase                         `json:"completed_phases"`
}

type pendingReceiptRecord struct {
	Schema int              `json:"schema"`
	Domain string           `json:"domain"`
	Digest receiptDigest    `json:"digest"`
	Facts  receiptCompleted `json:"facts"`
}

type currentRecord struct {
	Schema           int           `json:"schema"`
	Domain           string        `json:"domain"`
	Status           string        `json:"status"`
	Instance         string        `json:"instance"`
	OperationID      string        `json:"operation_id"`
	OperationToken   string        `json:"operation_token"`
	WriterGeneration uint64        `json:"writer_generation"`
	WriterToken      string        `json:"writer_token"`
	Candidate        receiptDigest `json:"candidate"`
	Receipt          receiptDigest `json:"receipt"`
}

var stateReserved = []string{"admission.lock", "candidates", "current.json", "init.lock", "journals", "receipts", "root.json", "writer-owner.json", "writer.lock"}
var dataReserved = []string{"owner.json"}
var fixtureSequence int

func resetRoots(base string, stateExists, dataExists bool) (string, string) {
	fixtureSequence++
	pair := filepath.Join(base, "fixtures", fmt.Sprintf("%04d", fixtureSequence))
	_ = os.MkdirAll(pair, 0700)
	_ = os.Chmod(filepath.Join(base, "fixtures"), 0700)
	_ = os.Chmod(pair, 0700)
	s := filepath.Join(pair, "state")
	d := filepath.Join(pair, "data")
	if stateExists {
		_ = os.Mkdir(s, 0700)
		_ = os.Chmod(s, 0700)
	}
	if dataExists {
		_ = os.Mkdir(d, 0700)
		_ = os.Chmod(d, 0700)
	}
	return s, d
}

func classify(s, d string) string { return classifyUID(s, d, os.Getuid()) }

func classifyUID(s, d string, uid int) string {
	si, se := os.Lstat(s)
	di, de := os.Lstat(d)
	sa, da := errors.Is(se, fs.ErrNotExist), errors.Is(de, fs.ErrNotExist)
	sempty := se == nil && safeDirectoryInfo(si, uid) && emptyDir(s)
	dempty := de == nil && safeDirectoryInfo(di, uid) && emptyDir(d)
	if (sa || sempty) && (da || dempty) {
		return "CLEAN"
	}
	recognizable, unsupported, inspectErr := inspectRecognizable(s, d)
	if unsupported {
		return "BAD unsupported"
	}
	if inspectErr != nil {
		return "BAD corrupt"
	}
	if !recognizable {
		return "FOREIGN"
	}
	if exactInit(s, d, uid) {
		return "INIT"
	}
	if current := exactCurrent(s, d, uid); current != "" {
		return current
	}
	return "BAD corrupt"
}

func safeDirectoryInfo(i os.FileInfo, uid int) bool {
	if i == nil || !i.IsDir() || i.Mode().Perm() != 0700 {
		return false
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid && st.Nlink >= 1
}

const (
	maxRecognitionEntries = 1 << 16
	maxRecognitionBytes   = 64 << 20
	maxRecognitionFile    = 1 << 20
)

func inspectRecognizable(s, d string) (bool, bool, error) {
	recognizable, unsupported := false, false
	entries, total := 0, int64(0)
	var firstErr error
	remember := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for rootIndex, root := range []string{s, d} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if path == root && errors.Is(walkErr, fs.ErrNotExist) {
					return nil
				}
				remember(walkErr)
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if path == root {
				return nil
			}
			entries++
			if entries > maxRecognitionEntries {
				remember(errors.New("recognition entry ceiling exceeded"))
				return fs.SkipAll
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				remember(err)
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if reservedTopologyEntry(rootIndex, filepath.ToSlash(relative)) {
				recognizable = true
			}
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				remember(err)
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			if info.Size() < 0 || info.Size() > maxRecognitionFile {
				remember(errors.New("recognition file ceiling exceeded"))
				return nil
			}
			if info.Size() > maxRecognitionBytes-total {
				remember(errors.New("recognition byte ceiling exceeded"))
				return nil
			}
			content, err := readNoFollowBounded(path, maxRecognitionFile)
			if err != nil {
				remember(err)
				return nil
			}
			total += int64(len(content))
			tagged, badDomain, envelopeErr := inspectDurableEnvelope(content)
			recognizable = recognizable || tagged
			unsupported = unsupported || badDomain
			remember(envelopeErr)
			return nil
		})
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		remember(err)
	}
	return recognizable, unsupported, firstErr
}

func reservedTopologyEntry(rootIndex int, relative string) bool {
	if rootIndex == 1 {
		return relative == "owner.json"
	}
	for _, name := range stateReserved {
		if relative == name {
			return true
		}
	}
	for _, directory := range []string{"candidates/", "journals/", "receipts/"} {
		if strings.HasPrefix(relative, directory) {
			leaf := strings.TrimPrefix(relative, directory)
			return leaf != "" && !strings.Contains(leaf, "/") && strings.HasSuffix(leaf, ".json")
		}
	}
	return false
}

func readNoFollowBounded(path string, limit int64) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("construct no-follow file")
	}
	defer file.Close()
	var stat syscall.Stat_t
	if err = syscall.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Size < 0 || stat.Size > limit {
		return nil, errors.New("unsafe recognition entry")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("recognition file ceiling exceeded")
	}
	return content, nil
}

func inspectDurableEnvelope(content []byte) (bool, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	first, err := decoder.Token()
	if err != nil {
		return false, false, nil
	}
	delim, ok := first.(json.Delim)
	if !ok || delim != '{' {
		return false, false, nil
	}
	seenSchema, seenDomain := false, false
	schema, domain := 0, ""
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, keyOK := token.(string)
		if tokenErr != nil || !keyOK {
			return seenSchema || seenDomain, false, taggedEnvelopeError(seenSchema || seenDomain, "malformed durable key")
		}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return seenSchema || seenDomain || key == "schema" || key == "domain", false, taggedEnvelopeError(seenSchema || seenDomain || key == "schema" || key == "domain", "malformed durable value")
		}
		switch key {
		case "schema":
			if seenSchema || json.Unmarshal(raw, &schema) != nil {
				return true, false, errors.New("ambiguous durable schema")
			}
			seenSchema = true
		case "domain":
			if seenDomain || json.Unmarshal(raw, &domain) != nil {
				return true, false, errors.New("ambiguous durable domain")
			}
			seenDomain = true
		}
	}
	if _, err = decoder.Token(); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return seenSchema || seenDomain, false, taggedEnvelopeError(seenSchema || seenDomain, "malformed durable envelope")
	}
	tagged := seenSchema || seenDomain
	if !tagged {
		return false, false, nil
	}
	if !seenSchema || !seenDomain {
		return true, false, errors.New("partial durable envelope")
	}
	return true, schema != 1 || !supportedDurableDomain(domain), nil
}

func taggedEnvelopeError(tagged bool, message string) error {
	if tagged {
		return errors.New(message)
	}
	return nil
}

func supportedDurableDomain(domain string) bool {
	switch domain {
	case "zerb.root", "zerb.data-owner", "zerb.cache-owner", "zerb.writer-owner", "zerb.candidate", "zerb.policy", "zerb.journal", "zerb.receipt", "zerb.completed", "zerb.current":
		return true
	default:
		return false
	}
}

func emptyDir(p string) bool {
	f, e := os.Open(p)
	if e != nil {
		return false
	}
	defer f.Close()
	_, e = f.Readdirnames(1)
	return e == io.EOF
}

func safeRegularUID(p string, uid int) bool {
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1 && int(stat.Uid) == uid
}

func safeLockUID(p string, uid int) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Size() == 0 && safeRegularUID(p, uid)
}

func exactNames(p string, want []string) bool {
	entries, err := os.ReadDir(p)
	if err != nil || len(entries) != len(want) {
		return false
	}
	for index := range entries {
		if entries[index].Name() != want[index] {
			return false
		}
	}
	return true
}

func exactLeafNames(p string, want ...string) bool {
	sort.Strings(want)
	return exactNames(p, want)
}

func initializedRoot(instance string) rootOwnerRecord {
	return rootOwnerRecord{Schema: 1, Domain: "zerb.root", Status: "initialized", Instance: instance}
}

func initializingRoot(instance, token string, generation uint64, claimant processIdentity) rootOwnerRecord {
	return rootOwnerRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: instance, Token: token, Process: claimant, Generation: generation}
}

func namespaceOwner(instance, role string) namespaceOwnerRecord {
	domain := "zerb.data-owner"
	if role == "cache" {
		domain = "zerb.cache-owner"
	}
	return namespaceOwnerRecord{Schema: 1, Domain: domain, Instance: instance}
}

func releasedWriter(instance, _ string, generation, previousGeneration uint64, previousToken string) writerOwnerRecord {
	return writerOwnerRecord{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: instance, Generation: generation, PreviousGeneration: previousGeneration, PreviousToken: previousToken}
}

func heldWriter(instance, token string, generation uint64, holder processIdentity, previousGeneration uint64, previousToken string) writerOwnerRecord {
	return writerOwnerRecord{Schema: 1, Domain: "zerb.writer-owner", Status: "held", Instance: instance, Generation: generation, Token: token, Process: holder, PreviousGeneration: previousGeneration, PreviousToken: previousToken}
}

func generationSuccessor(generation uint64) (uint64, bool) {
	if generation == ^uint64(0) {
		return 0, false
	}
	return generation + 1, true
}

func isGenerationSuccessor(previous, current uint64) bool {
	next, ok := generationSuccessor(previous)
	return ok && current == next
}

func validWriterPredecessor(writer writerOwnerRecord) bool {
	if writer.Generation == 0 {
		return writer.Status == "released" && writer.PreviousGeneration == 0 && writer.PreviousToken == ""
	}
	if !isGenerationSuccessor(writer.PreviousGeneration, writer.Generation) {
		return false
	}
	if writer.Status == "released" {
		return writer.PreviousToken != ""
	}
	return writer.Status == "held" && writer.Token != "" &&
		(writer.PreviousToken == "" || writer.Token != writer.PreviousToken)
}

func candidateTemplate(profile string) candidateReferenceRecord {
	digest := func(label string) string {
		sum := sha256.Sum256([]byte(label))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	// Synthetic native SHA-1 object syntax; this fixture does not prove commit/tree membership.
	source := receiptSource{URI: sourceid.KnownVectorURI, SourceID: sourceid.KnownVectorDigest, Commit: strings.Repeat("c", 40), RootTree: strings.Repeat("a", 40)}
	components := []receiptComponent{
		{ID: "base", Path: "profiles/base", TreeDigest: digest("base-tree"), Files: []receiptFile{{Path: "home/managed/file", Mode: "file", Hash: digest("managed-file")}}},
		{ID: profile, Path: "profiles/" + profile, TreeDigest: digest(profile + "-tree"), Files: []receiptFile{}},
	}
	executable := func(id, version string) receiptExecutable {
		return receiptExecutable{ID: id, Version: version, Path: "/proof/bin/" + id, SHA256: digest(id + "-executable")}
	}
	return candidateReferenceRecord{
		Schema: 1, Domain: "zerb.candidate", Source: source, ProfileID: profile,
		Components:  components,
		Tools:       []receiptTool{{ID: "node", Version: "24.4.1", Locks: []receiptLockValue{{Name: "artifact", Value: digest("node-lock")}}}},
		HomeSources: []string{"profiles/base/home", "profiles/" + profile + "/home"},
		Targets:     []string{"managed/file"},
		Executables: []receiptExecutable{executable("bash", "5.3.15"), executable("chezmoi", "v2.70.5-zerb.1"), executable("git", "2.55.0"), executable("mise", "2026.7.17")},
		Parser:      receiptParser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"},
		Policy:      receiptPolicy{Schema: 1, Domain: "zerb.policy", ObjectModes: []string{"directory", "executable", "file"}, Declarations: []string{"home", "mise.lock", "mise.toml", "profile.toml"}, TemplateActions: []string{"if-false", "if-true", "literal-text"}, ProcessClasses: []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}, PolicyIDs: []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}, TargetMapperExecutableID: "chezmoi", ProviderFree: true, HostFactIndependent: true, DestinationRecheck: true, ExactReapplyOnly: true},
	}
}

func candidateDigestFor(profile string) receiptDigest {
	return receiptDigest("sha256:" + recordDigest(candidateTemplate(profile)))
}

func candidateReference(_ string, candidate string) candidateReferenceRecord {
	if receiptDigest(candidate) == candidateDigestFor("proof-next") {
		return candidateTemplate("proof-next")
	}
	return candidateTemplate("proof-current")
}

var mutationPhases = []string{string(phaseToolInstall), string(phaseHomeApply)}

func receiptPhases() []string {
	phases := append([]string{}, mutationPhases...)
	sort.Strings(phases)
	return phases
}

func liveSelfReceiptExecutable() (receiptExecutable, error) {
	fd, err := syscall.Open("/proc/self/exe", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return receiptExecutable{}, err
	}
	running := os.NewFile(uintptr(fd), "live-running-self")
	if running == nil {
		_ = syscall.Close(fd)
		return receiptExecutable{}, errors.New("live self descriptor unavailable")
	}
	defer running.Close()
	runningInfo, err := running.Stat()
	if err != nil || !runningInfo.Mode().IsRegular() || runningInfo.Mode().Perm()&0111 == 0 ||
		runningInfo.Mode().Perm()&0022 != 0 || runningInfo.Size() <= 0 {
		return receiptExecutable{}, errors.New("live running executable topology is invalid")
	}
	runningStat, ok := runningInfo.Sys().(*syscall.Stat_t)
	if !ok || runningStat.Uid != uint32(os.Geteuid()) || runningStat.Gid != uint32(os.Getegid()) || runningStat.Nlink != 1 {
		return receiptExecutable{}, errors.New("live running executable identity is invalid")
	}
	path, err := os.Readlink("/proc/self/exe")
	if err != nil || strings.HasSuffix(path, " (deleted)") {
		return receiptExecutable{}, errors.New("live running executable path is unavailable")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return receiptExecutable{}, errors.New("live self executable path is not canonical")
	}
	verifyPath := func() error {
		before, statErr := os.Lstat(path)
		if statErr != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0111 == 0 {
			return errors.New("live self executable path topology is invalid")
		}
		pathFD, openErr := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil {
			return openErr
		}
		pathFile := os.NewFile(uintptr(pathFD), "live-self-path")
		if pathFile == nil {
			_ = syscall.Close(pathFD)
			return errors.New("live self path descriptor unavailable")
		}
		defer pathFile.Close()
		opened, openStatErr := pathFile.Stat()
		after, afterErr := os.Lstat(path)
		if openStatErr != nil || afterErr != nil || !os.SameFile(before, opened) ||
			!os.SameFile(opened, after) || !os.SameFile(opened, runningInfo) {
			return errors.New("live self descriptor/path identity mismatch")
		}
		return nil
	}
	if err = verifyPath(); err != nil {
		return receiptExecutable{}, err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.NewSectionReader(running, 0, runningInfo.Size())); err != nil {
		return receiptExecutable{}, err
	}
	afterHashInfo, statErr := running.Stat()
	if statErr != nil || !os.SameFile(runningInfo, afterHashInfo) ||
		runningInfo.Mode() != afterHashInfo.Mode() || runningInfo.Size() != afterHashInfo.Size() {
		return receiptExecutable{}, errors.New("live running executable changed while hashing")
	}
	if err = verifyPath(); err != nil {
		return receiptExecutable{}, err
	}
	return receiptExecutable{
		ID: "zerb", Version: resultVersion, Path: path,
		SHA256: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func receiptFactsFor(candidate string, previous receiptDigest) receiptCompleted {
	candidateRecord := candidateReference("", candidate)
	byID := func(id string) receiptExecutable {
		for _, executable := range candidateRecord.Executables {
			if executable.ID == id {
				return executable
			}
		}
		return receiptExecutable{}
	}
	liveSelf, err := liveSelfReceiptExecutable()
	if err != nil {
		panic(err)
	}
	precondition := receiptDestinationObservation{Target: "managed/file", Exists: true, Kind: "file", Mode: 0600, Owner: uint32(os.Geteuid()), Device: 1, Inode: 1, Links: 1, ContentHash: "sha256:" + recordDigest("precondition")}
	if previous == "" {
		precondition = receiptDestinationObservation{Target: "managed/file", Exists: false, Kind: "absent"}
	}
	result := receiptDestinationObservation{Target: "managed/file", Exists: true, Kind: "file", Mode: 0600, Owner: uint32(os.Geteuid()), Device: 1, Inode: 2, Links: 1, ContentHash: "sha256:" + recordDigest("result")}
	var uts syscall.Utsname
	_ = syscall.Uname(&uts)
	return receiptCompleted{
		Schema: 1, Domain: "zerb.completed", Candidate: receiptDigest(candidate), Previous: previous,
		Source: candidateRecord.Source, Components: append([]receiptComponent(nil), candidateRecord.Components...),
		Zerb: liveSelf,
		Git:  byID("git"), Mise: byID("mise"), Chezmoi: byID("chezmoi"),
		Parser: candidateRecord.Parser, Policy: candidateRecord.Policy,
		Platform: receiptPlatformObservation{Distribution: "Fedora Linux 44", Kernel: utsField(uts.Release), Architecture: runtime.GOARCH, Filesystem: "btrfs"},
		Tools:    []receiptToolObservation{{ID: "node", Requested: "24.4.1", Installed: "24.4.1", Integrity: "sha256:" + recordDigest("node-installed")}},
		Targets:  append([]string(nil), candidateRecord.Targets...), Preconditions: []receiptDestinationObservation{precondition}, Results: []receiptDestinationObservation{result},
		Phases:     receiptPhases(),
		Dimensions: receiptDimensions{InputsPinned: true, ArtifactIntegrity: true, ArtifactAvailable: true, HostCompatible: true, ConvergenceVerified: true},
	}
}

func receiptFacts(candidate string) receiptCompleted {
	return receiptFactsFor(candidate, "")
}

func pendingReceiptFor(candidate string, previous receiptDigest) pendingReceiptRecord {
	facts := receiptFactsFor(candidate, previous)
	return pendingReceiptRecord{Schema: 1, Domain: "zerb.receipt", Digest: receiptDigest("sha256:" + recordDigest(facts)), Facts: facts}
}

func pendingReceipt(_ string, candidate, _ string) pendingReceiptRecord {
	facts := receiptFacts(candidate)
	return pendingReceiptRecord{Schema: 1, Domain: "zerb.receipt", Digest: receiptDigest("sha256:" + recordDigest(facts)), Facts: facts}
}

func stableCurrent(instance string, candidate, receipt, operation string) currentRecord {
	return currentRecord{Schema: 1, Domain: "zerb.current", Status: "stable", Instance: instance,
		OperationID: recordDigest("prior-operation:" + operation)[:32], OperationToken: recordDigest("prior-operation-token:" + operation)[:32],
		WriterGeneration: 1, WriterToken: recordDigest("prior-writer:" + operation)[:32], Candidate: receiptDigest(candidate), Receipt: receiptDigest(receipt)}
}

func baselineCurrent(r graphRefs) currentRecord {
	if r.priorCurrent.Schema != 0 {
		return r.priorCurrent
	}
	current := stableCurrent(r.instance, r.a, r.receiptA, r.journal)
	if r.generation > 2 {
		current.WriterGeneration = r.generation - 2
	}
	current.WriterToken = r.priorWriterToken
	return current
}

func validIdentityToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	for _, c := range token {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func validCurrentIdentity(current currentRecord, instance string) bool {
	return current.Schema == 1 && current.Domain == "zerb.current" && current.Status == "stable" &&
		current.Instance == instance && validIdentityToken(current.Instance) &&
		validIdentityToken(current.OperationID) && validIdentityToken(current.OperationToken) && validIdentityToken(current.WriterToken) &&
		current.WriterGeneration > 0 && current.OperationID != current.OperationToken &&
		current.OperationID != current.WriterToken && current.OperationToken != current.WriterToken
}

func currentMatchesJournal(current currentRecord, journal journalRecord) bool {
	return current.OperationID == journal.OperationID && current.OperationToken == journal.OperationToken &&
		current.WriterGeneration <= journal.WriterGeneration &&
		(current.WriterGeneration == journal.WriterGeneration && current.WriterToken == journal.WriterToken ||
			current.WriterGeneration < journal.WriterGeneration && journal.WriterPreviousToken != "" && current.WriterToken != journal.WriterToken &&
				(isGenerationSuccessor(current.WriterGeneration, journal.WriterGeneration) && current.WriterToken == journal.WriterPreviousToken ||
					!isGenerationSuccessor(current.WriterGeneration, journal.WriterGeneration) && current.WriterToken != journal.WriterPreviousToken))
}

func currentMatchesHeldWriter(current currentRecord, writer writerOwnerRecord) bool {
	return current.WriterGeneration == writer.Generation && current.WriterToken == writer.Token ||
		current.WriterGeneration < writer.Generation && writer.PreviousToken != "" && current.WriterToken != writer.Token &&
			(current.WriterGeneration == writer.PreviousGeneration && current.WriterToken == writer.PreviousToken ||
				current.WriterGeneration < writer.PreviousGeneration && current.WriterToken != writer.PreviousToken)
}

func currentPrecedesReleasedWriter(current currentRecord, writer writerOwnerRecord) bool {
	return current.WriterGeneration < writer.Generation &&
		(current.WriterGeneration == writer.PreviousGeneration && current.WriterToken == writer.PreviousToken ||
			current.WriterGeneration < writer.PreviousGeneration && current.WriterToken != writer.PreviousToken)
}

func exactPhase(state, oldPhase string) phase {
	switch state {
	case "prepared":
		return phasePrepared
	case string(phasePending):
		return phasePending
	case "cleanup":
		return phaseCleanup
	}
	switch oldPhase {
	case "tool-install", string(phaseToolInstall):
		return phaseToolInstall
	case "home-apply", string(phaseHomeApply):
		return phaseHomeApply
	default:
		return phase(oldPhase)
	}
}

func exactCompletedPhases(completed []string) []phase {
	result := make([]phase, len(completed))
	for index, completedPhase := range completed {
		result[index] = exactPhase("mutating", completedPhase)
	}
	return result
}

func transitionJournalRecord(r graphRefs, state, oldPhase string, completed []string, generation uint64, writerToken string) journalRecord {
	preconditions := receiptFactsFor(r.b, receiptDigest(r.a)).Preconditions
	results := receiptFactsFor(r.b, receiptDigest(r.a)).Results
	if len(completed) == 0 {
		results = []receiptDestinationObservation{}
	}
	return journalRecord{
		Schema: 1, Domain: "zerb.journal", Instance: r.instance, OperationID: r.journal, OperationToken: r.operationToken,
		WriterGeneration: generation, WriterToken: writerToken, WriterPreviousToken: r.writerPredecessorToken,
		Previous: receiptDigest(r.a), PreviousReceipt: receiptDigest(r.receiptA), Candidate: receiptDigest(r.b),
		Phase: exactPhase(state, oldPhase), Preconditions: append([]receiptDestinationObservation(nil), preconditions...),
		Results: append([]receiptDestinationObservation{}, results...), CompletedPhases: exactCompletedPhases(completed),
	}
}

func appliedObservation(_, candidate, _ string, completed []string) receiptDestinationObservation {
	return receiptDestinationObservation{Target: "managed/file", Exists: true, Kind: "file", Mode: 0600, Owner: uint32(os.Geteuid()), Device: 1, Inode: uint64(len(completed) + 1), Links: 1, ContentHash: "sha256:" + recordDigest(candidate+strings.Join(completed, ","))}
}

func exactInit(s, d string, uid int) bool {
	return exactInitAllowing(s, d, uid, "")
}

func exactInitAllowing(s, d string, uid int, allowedEntry string) bool {
	if !safeRootDir(s, uid) || !safeLockUID(filepath.Join(s, "init.lock"), uid) {
		return false
	}
	ordered := []string{"init.lock", "root.json", "admission.lock", "writer.lock", "writer-owner.json", "candidates", "journals", "receipts"}
	present := 0
	for present < len(ordered) && exists(filepath.Join(s, ordered[present])) {
		present++
	}
	if present == 0 {
		return false
	}
	want := append([]string(nil), ordered[:present]...)
	if allowedEntry != "" {
		want = append(want, allowedEntry)
	}
	sort.Strings(want)
	if !exactNames(s, want) {
		return false
	}
	for _, name := range []string{"admission.lock", "writer.lock"} {
		if exists(filepath.Join(s, name)) && !safeLockUID(filepath.Join(s, name), uid) {
			return false
		}
	}
	for _, name := range []string{"candidates", "journals", "receipts"} {
		if exists(filepath.Join(s, name)) && (!safeRootDir(filepath.Join(s, name), uid) || !emptyDir(filepath.Join(s, name))) {
			return false
		}
	}
	if present == 1 {
		if !exists(d) {
			return true
		}
		return safeRootDir(d, uid) && emptyDir(d)
	}
	var root rootOwnerRecord
	rootPath := filepath.Join(s, "root.json")
	if !safeRegularUID(rootPath, uid) || readRecord(rootPath, &root) != nil ||
		root.Schema != 1 || root.Domain != "zerb.root" || root.Status != "initializing" ||
		root.Instance == "" || root.Token == "" || !validProcessIdentity(root.Process) {
		return false
	}
	if exists(filepath.Join(s, "writer-owner.json")) {
		var writer writerOwnerRecord
		path := filepath.Join(s, "writer-owner.json")
		if !safeRegularUID(path, uid) || readRecord(path, &writer) != nil ||
			writer != releasedWriter(root.Instance, "", 0, 0, "") {
			return false
		}
	}
	if !exists(d) {
		return true
	}
	if !safeRootDir(d, uid) {
		return false
	}
	if emptyDir(d) {
		return true
	}
	return present == len(ordered) && exactNames(d, dataReserved) &&
		exactRecordBytes(filepath.Join(d, "owner.json"), namespaceOwner(root.Instance, "data"))
}

func readCurrentAuthority(s, d string, uid int) (rootOwnerRecord, writerOwnerRecord, bool) {
	if !safeRootDir(s, uid) || !safeRootDir(d, uid) || !exactNames(d, dataReserved) {
		return rootOwnerRecord{}, writerOwnerRecord{}, false
	}
	rootPath, ownerPath, writerPath := filepath.Join(s, "root.json"), filepath.Join(d, "owner.json"), filepath.Join(s, "writer-owner.json")
	for _, name := range []string{"init.lock", "admission.lock", "writer.lock"} {
		if !safeLockUID(filepath.Join(s, name), uid) {
			return rootOwnerRecord{}, writerOwnerRecord{}, false
		}
	}
	if !safeRegularUID(rootPath, uid) || !safeRegularUID(ownerPath, uid) || !safeRegularUID(writerPath, uid) {
		return rootOwnerRecord{}, writerOwnerRecord{}, false
	}
	var root rootOwnerRecord
	var owner namespaceOwnerRecord
	var writer writerOwnerRecord
	if readRecord(rootPath, &root) != nil || readRecord(ownerPath, &owner) != nil || readRecord(writerPath, &writer) != nil {
		return rootOwnerRecord{}, writerOwnerRecord{}, false
	}
	if root != initializedRoot(root.Instance) || root.Instance == "" || owner != namespaceOwner(root.Instance, "data") ||
		writer.Schema != 1 || writer.Domain != "zerb.writer-owner" || writer.Instance != root.Instance ||
		(writer.Status != "released" && writer.Status != "held") ||
		(writer.Status == "released" && (writer.Token != "" || validProcessIdentity(writer.Process))) ||
		(writer.Status == "held" && (writer.Token == "" || !validProcessIdentity(writer.Process))) ||
		!validWriterPredecessor(writer) {
		return rootOwnerRecord{}, writerOwnerRecord{}, false
	}
	for _, name := range []string{"candidates", "journals", "receipts"} {
		if !safeRootDir(filepath.Join(s, name), uid) {
			return rootOwnerRecord{}, writerOwnerRecord{}, false
		}
	}
	return root, writer, true
}

func readCurrentRelation(s string, uid int, instance string, currentPresent bool) (currentRecord, pendingReceiptRecord, bool) {
	var current currentRecord
	var currentReceipt pendingReceiptRecord
	if currentPresent {
		currentPath := filepath.Join(s, "current.json")
		if !safeRegularUID(currentPath, uid) || readRecord(currentPath, &current) != nil ||
			!validCurrentIdentity(current, instance) ||
			current.Candidate == "" || current.Receipt == "" ||
			!validCandidate(filepath.Join(s, "candidates", string(current.Candidate)+".json"), instance, string(current.Candidate)) ||
			readRecord(filepath.Join(s, "receipts", string(current.Receipt)+".json"), &currentReceipt) != nil ||
			currentReceipt.Digest != current.Receipt ||
			!validPendingReceiptFor(filepath.Join(s, "receipts", string(current.Receipt)+".json"), string(current.Candidate), currentReceipt.Facts.Previous) ||
			!currentDigestsMatch(s, current) {
			return currentRecord{}, pendingReceiptRecord{}, false
		}
		if currentReceipt.Facts.Previous != "" && !validReceiptDigest(string(currentReceipt.Facts.Previous)) {
			return currentRecord{}, pendingReceiptRecord{}, false
		}
	}
	return current, currentReceipt, true
}

func exactJournalLessCurrent(s, instance string, currentPresent bool, writer writerOwnerRecord, current currentRecord) string {
	if !currentPresent && writer.Status == "released" && writer != releasedWriter(instance, "", 0, 0, "") {
		return ""
	}
	if !currentPresent {
		if !validCandidateDirectory(filepath.Join(s, "candidates")) || !emptyDir(filepath.Join(s, "receipts")) {
			return ""
		}
		if writer.Status != "released" {
			return ""
		}
		return "READY(∅)"
	}
	if !validCandidateDirectory(filepath.Join(s, "candidates"), string(current.Candidate)) ||
		!exactLeafNames(filepath.Join(s, "receipts"), string(current.Receipt)+".json") {
		return ""
	}
	if writer.Status == "held" {
		if !currentMatchesHeldWriter(current, writer) {
			return ""
		}
		return "CLEANUP(B)"
	}
	if !currentPrecedesReleasedWriter(current, writer) {
		return ""
	}
	return "STABLE(A)"
}

func readCurrentJournal(s string, uid int, instance string, entry os.DirEntry, writer writerOwnerRecord) (journalRecord, bool) {
	if !entry.Type().IsRegular() || writer.Status != "held" {
		return journalRecord{}, false
	}
	journalPath := filepath.Join(s, "journals", entry.Name())
	journalID := strings.TrimSuffix(entry.Name(), ".json")
	var journal journalRecord
	if journalID == entry.Name() || !safeRegularUID(journalPath, uid) || readRecord(journalPath, &journal) != nil ||
		journal.Schema != 1 || journal.Domain != "zerb.journal" || journal.Instance != instance ||
		journal.OperationID != journalID || journal.Candidate == "" {
		return journalRecord{}, false
	}
	currentWriter := journal.WriterGeneration == writer.Generation &&
		journal.WriterToken == writer.Token && journal.WriterPreviousToken == writer.PreviousToken
	journalAhead := isGenerationSuccessor(writer.Generation, journal.WriterGeneration) &&
		journal.WriterPreviousToken == writer.Token
	if (!currentWriter && !journalAhead) || !validJournalPrefix(journal) ||
		!validCandidate(filepath.Join(s, "candidates", string(journal.Candidate)+".json"), instance, string(journal.Candidate)) {
		return journalRecord{}, false
	}
	return journal, true
}

func exactPriorTransition(s, instance string, journal journalRecord, nextReceipt pendingReceiptRecord, nextReceiptExists bool) string {
	if journal.Phase == phaseCleanup {
		return ""
	}
	requiredCandidates := []string{string(journal.Candidate)}
	if journal.Previous != journal.Candidate {
		requiredCandidates = append(requiredCandidates, string(journal.Previous))
	}
	requiredReceipts := []string{string(journal.PreviousReceipt) + ".json"}
	if nextReceiptExists && nextReceipt.Digest != journal.PreviousReceipt {
		requiredReceipts = append(requiredReceipts, string(nextReceipt.Digest)+".json")
	}
	if !validCandidate(filepath.Join(s, "candidates", string(journal.Previous)+".json"), instance, string(journal.Previous)) ||
		!validCandidateDirectory(filepath.Join(s, "candidates"), requiredCandidates...) ||
		(nextReceiptExists && nextReceipt.Digest != journal.PreviousReceipt && journal.Phase != phasePending) ||
		!exactLeafNames(filepath.Join(s, "receipts"), requiredReceipts...) {
		return ""
	}
	return "TRANSITION(A,B)"
}

func exactCommittedCleanup(s, instance string, journal journalRecord, nextReceipt pendingReceiptRecord) string {
	previousCandidateDistinct := journal.Previous != "" && journal.Previous != journal.Candidate
	previousReceiptDistinct := journal.PreviousReceipt != "" && journal.PreviousReceipt != nextReceipt.Digest
	previousCandidateExists := previousCandidateDistinct && exists(filepath.Join(s, "candidates", string(journal.Previous)+".json"))
	previousReceiptPath := filepath.Join(s, "receipts", string(journal.PreviousReceipt)+".json")
	previousReceiptExists := previousReceiptDistinct && exists(previousReceiptPath)
	if previousCandidateExists && !validCandidate(filepath.Join(s, "candidates", string(journal.Previous)+".json"), instance, string(journal.Previous)) {
		return ""
	}
	if previousReceiptExists {
		var previousReceipt pendingReceiptRecord
		if readRecord(previousReceiptPath, &previousReceipt) != nil ||
			previousReceipt.Digest != journal.PreviousReceipt ||
			previousReceipt.Facts.Candidate != journal.Previous ||
			!validPendingReceiptFor(previousReceiptPath, string(journal.Previous), previousReceipt.Facts.Previous) {
			return ""
		}
	}
	if previousCandidateExists && previousReceiptDistinct && !previousReceiptExists {
		return ""
	}
	requiredCandidates := []string{string(journal.Candidate)}
	requiredReceipts := []string{string(nextReceipt.Digest) + ".json"}
	if previousCandidateExists {
		requiredCandidates = append(requiredCandidates, string(journal.Previous))
	}
	if previousReceiptExists {
		requiredReceipts = append(requiredReceipts, string(journal.PreviousReceipt)+".json")
	}
	if !validCandidateDirectory(filepath.Join(s, "candidates"), requiredCandidates...) ||
		!exactLeafNames(filepath.Join(s, "receipts"), requiredReceipts...) ||
		(journal.Phase == phasePending && ((previousCandidateDistinct && !previousCandidateExists) || (previousReceiptDistinct && !previousReceiptExists))) {
		return ""
	}
	return "CLEANUP(B)"

}

func exactCurrent(s, d string, uid int) string {
	root, writer, valid := readCurrentAuthority(s, d, uid)
	if !valid {
		return ""
	}
	base := []string{"admission.lock", "candidates", "init.lock", "journals", "receipts", "root.json", "writer-owner.json", "writer.lock"}
	currentPresent := exists(filepath.Join(s, "current.json"))
	names := append([]string(nil), base...)
	if currentPresent {
		names = append(names, "current.json")
		sort.Strings(names)
	}
	if !exactNames(s, names) {
		return ""
	}
	journalEntries, err := os.ReadDir(filepath.Join(s, "journals"))
	if err != nil || len(journalEntries) > 1 {
		return ""
	}
	current, currentReceipt, valid := readCurrentRelation(s, uid, root.Instance, currentPresent)
	if !valid {
		return ""
	}
	if len(journalEntries) == 0 {
		return exactJournalLessCurrent(s, root.Instance, currentPresent, writer, current)
	}
	journal, valid := readCurrentJournal(s, uid, root.Instance, journalEntries[0], writer)
	if !valid {
		return ""
	}
	nextReceipt := pendingReceiptFor(string(journal.Candidate), journal.Previous)
	nextReceiptPath := filepath.Join(s, "receipts", string(nextReceipt.Digest)+".json")
	nextReceiptExists := exists(nextReceiptPath)
	if (journal.Previous == "") != (journal.PreviousReceipt == "") ||
		(journal.PreviousReceipt != "" && !validReceiptDigest(string(journal.PreviousReceipt))) ||
		(nextReceiptExists && !validPendingReceiptFor(nextReceiptPath, string(journal.Candidate), journal.Previous)) {
		return ""
	}
	if !currentPresent {
		if journal.Previous != "" || journal.PreviousReceipt != "" || journal.Phase == phaseCleanup ||
			!validCandidateDirectory(filepath.Join(s, "candidates"), string(journal.Candidate)) {
			return ""
		}
		if nextReceiptExists {
			if journal.Phase != phasePending || !exactLeafNames(filepath.Join(s, "receipts"), string(nextReceipt.Digest)+".json") {
				return ""
			}
		} else if !emptyDir(filepath.Join(s, "receipts")) {
			return ""
		}
		return "TRANSITION(∅,B)"
	}
	committedOperation := currentMatchesJournal(current, journal)
	beforeCommit := !committedOperation && current.OperationID != journal.OperationID &&
		current.OperationToken != journal.OperationToken && current.WriterGeneration < writer.Generation &&
		current.Candidate == journal.Previous && current.Receipt == journal.PreviousReceipt && journal.Previous != ""
	if beforeCommit {
		return exactPriorTransition(s, root.Instance, journal, nextReceipt, nextReceiptExists)
	}
	if !committedOperation || current.Candidate != journal.Candidate || current.Receipt != nextReceipt.Digest ||
		currentReceipt.Facts.Previous != journal.Previous ||
		(journal.Phase != phasePending && journal.Phase != phaseCleanup) {
		return ""
	}
	return exactCommittedCleanup(s, root.Instance, journal, nextReceipt)
}
func safeRootDir(p string, uid int) bool {
	i, e := os.Lstat(p)
	return e == nil && safeDirectoryInfo(i, uid)
}
func exists(p string) bool { _, e := os.Lstat(p); return e == nil }

func readRecord(p string, value any) error {
	content, err := readNoFollowBounded(p, maxRecognitionFile)
	if err != nil {
		return err
	}
	if err = rejectDuplicateJSONKeys(content); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func rejectDuplicateJSONKeys(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			return nil
		}
		switch delim {
		case '{':
			var keys []string
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("non-string JSON object key")
				}
				for _, prior := range keys {
					if prior == key {
						return errors.New("duplicate JSON object key")
					}
				}
				keys = append(keys, key)
				if err = visit(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return errors.New("malformed JSON object")
			}
		case '[':
			for decoder.More() {
				if err = visit(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return errors.New("malformed JSON array")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func writeRecord(p string, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(p, content, 0600)
}

func validCandidate(p, instance, candidate string) bool {
	return exactRecordBytes(p, candidateReference(instance, candidate))
}

func validCandidateDirectory(directory string, required ...string) bool {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	found := make([]bool, len(required))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return false
		}
		path := filepath.Join(directory, entry.Name())
		if !safeRegularUID(path, os.Getuid()) {
			return false
		}
		content, err := readNoFollowBounded(path, maxRecognitionFile)
		if err != nil {
			return false
		}
		var candidate candidateReferenceRecord
		if readRecord(path, &candidate) != nil || candidate.Schema != 1 || candidate.Domain != "zerb.candidate" ||
			candidate.Policy.Schema != 1 || candidate.Policy.Domain != "zerb.policy" ||
			!candidateCollectionsCanonical(candidate) || !bytes.Equal(content, recordBytes(candidate)) {
			return false
		}
		digest := "sha256:" + recordDigest(candidate)
		if entry.Name() != digest+".json" {
			return false
		}
		for index, requiredDigest := range required {
			if digest == requiredDigest {
				found[index] = true
			}
		}
	}
	for _, requiredFound := range found {
		if !requiredFound {
			return false
		}
	}
	return true
}

func candidateCollectionsCanonical(candidate candidateReferenceRecord) bool {
	componentIDs := make([]string, len(candidate.Components))
	for index, component := range candidate.Components {
		componentIDs[index] = component.ID
		filePaths := make([]string, len(component.Files))
		for fileIndex, file := range component.Files {
			filePaths[fileIndex] = file.Path
		}
		if !strictlySorted(filePaths) {
			return false
		}
	}
	toolIDs := make([]string, len(candidate.Tools))
	for index, tool := range candidate.Tools {
		toolIDs[index] = tool.ID
		lockNames := make([]string, len(tool.Locks))
		for lockIndex, lock := range tool.Locks {
			lockNames[lockIndex] = lock.Name
		}
		if !strictlySorted(lockNames) {
			return false
		}
	}
	executableIDs := make([]string, len(candidate.Executables))
	for index, executable := range candidate.Executables {
		executableIDs[index] = executable.ID
		if !validReceiptExecutable(executable, executable.ID) {
			return false
		}
	}
	policy := candidate.Policy
	return strictlySorted(componentIDs) && strictlySorted(toolIDs) &&
		strictlySorted(candidate.HomeSources) && strictlySorted(candidate.Targets) &&
		slicesEqual(executableIDs, []string{"bash", "chezmoi", "git", "mise"}) &&
		slicesEqual(policy.ObjectModes, []string{"directory", "executable", "file"}) &&
		slicesEqual(policy.Declarations, []string{"home", "mise.lock", "mise.toml", "profile.toml"}) &&
		slicesEqual(policy.TemplateActions, []string{"if-false", "if-true", "literal-text"}) &&
		slicesEqual(policy.ProcessClasses, []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}) &&
		slicesEqual(policy.PolicyIDs, []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}) &&
		policy.TargetMapperExecutableID == "chezmoi" && policy.ProviderFree &&
		policy.HostFactIndependent && policy.DestinationRecheck && policy.ExactReapplyOnly
}

func validReceiptDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validReceiptExecutable(executable receiptExecutable, wantID string) bool {
	return executable.ID == wantID && executable.Version != "" && len(executable.Version) <= 128 &&
		filepath.IsAbs(executable.Path) && filepath.Clean(executable.Path) == executable.Path &&
		!strings.Contains(executable.Path, "\\") && validReceiptDigest(executable.SHA256)
}

func validCompletedReceipt(facts receiptCompleted, candidate string, previous receiptDigest) bool {
	candidateRecord := candidateReference("", candidate)
	liveSelf, err := liveSelfReceiptExecutable()
	if err != nil {
		return false
	}
	want := receiptFactsFor(candidate, previous)
	return facts.Schema == 1 && facts.Domain == "zerb.completed" &&
		facts.Candidate == receiptDigest(candidate) && facts.Previous == previous &&
		facts.Source == candidateRecord.Source &&
		bytes.Equal(recordBytes(facts.Components), recordBytes(candidateRecord.Components)) &&
		validReceiptExecutable(facts.Zerb, "zerb") && facts.Zerb == liveSelf && facts.Zerb == want.Zerb &&
		validReceiptExecutable(facts.Git, "git") && facts.Git == candidateRecord.Executables[2] &&
		validReceiptExecutable(facts.Mise, "mise") && facts.Mise == candidateRecord.Executables[3] &&
		validReceiptExecutable(facts.Chezmoi, "chezmoi") && facts.Chezmoi == candidateRecord.Executables[1] &&
		facts.Parser == candidateRecord.Parser && bytes.Equal(recordBytes(facts.Policy), recordBytes(candidateRecord.Policy)) &&
		facts.Platform.Distribution != "" && facts.Platform.Kernel != "" &&
		facts.Platform.Architecture != "" && facts.Platform.Filesystem != "" &&
		slicesEqual(facts.Tools, want.Tools) && slicesEqual(facts.Targets, candidateRecord.Targets) &&
		slicesEqual(facts.Preconditions, want.Preconditions) && slicesEqual(facts.Results, want.Results) &&
		slicesEqual(facts.Phases, receiptPhases()) && facts.Dimensions == want.Dimensions
}

func strictlySorted(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

func validPendingReceiptFor(p, candidate string, previous receiptDigest) bool {
	var value pendingReceiptRecord
	if !safeRegularUID(p, os.Getuid()) || readRecord(p, &value) != nil {
		return false
	}
	want := pendingReceiptFor(candidate, previous)
	return value.Digest == want.Digest && validCompletedReceipt(value.Facts, candidate, previous) &&
		value.Digest == receiptDigest("sha256:"+recordDigest(value.Facts)) && exactRecordBytes(p, want)
}

func currentDigestsMatch(state string, current currentRecord) bool {
	candidatePath := filepath.Join(state, "candidates", string(current.Candidate)+".json")
	receiptPath := filepath.Join(state, "receipts", string(current.Receipt)+".json")
	candidateBytes, candidateErr := os.ReadFile(candidatePath)
	var receipt pendingReceiptRecord
	if candidateErr != nil || readRecord(receiptPath, &receipt) != nil {
		return false
	}
	candidateHash := sha256.Sum256(candidateBytes)
	return current.Candidate == receiptDigest("sha256:"+hex.EncodeToString(candidateHash[:])) &&
		current.Receipt == receipt.Digest &&
		receipt.Facts.Candidate == current.Candidate
}

func validJournalPrefix(journal journalRecord) bool {
	facts := receiptFactsFor(string(journal.Candidate), journal.Previous)
	expectedResults := []receiptDestinationObservation{}
	if len(journal.CompletedPhases) > 0 {
		expectedResults = facts.Results
	}
	if journal.OperationToken == "" || journal.OperationToken == journal.OperationID ||
		journal.OperationToken == journal.WriterToken ||
		(journal.WriterPreviousToken != "" &&
			(journal.WriterPreviousToken == journal.OperationToken || journal.WriterPreviousToken == journal.WriterToken)) ||
		!slicesEqual(journal.Preconditions, facts.Preconditions) ||
		!slicesEqual(journal.Results, expectedResults) {
		return false
	}
	if journal.Phase == phasePrepared {
		return len(journal.CompletedPhases) == 0
	}
	if journal.Phase == phasePending {
		return slicesEqual(journal.CompletedPhases, exactCompletedPhases(mutationPhases))
	}
	if journal.Phase == phaseCleanup {
		return slicesEqual(journal.CompletedPhases, exactCompletedPhases(mutationPhases))
	}
	for index, mutationPhase := range mutationPhases {
		if journal.Phase == exactPhase("mutating", mutationPhase) {
			return slicesEqual(journal.CompletedPhases, exactCompletedPhases(mutationPhases[:index])) ||
				slicesEqual(journal.CompletedPhases, exactCompletedPhases(mutationPhases[:index+1]))
		}
	}
	return false
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func classScenario(id, got, want, fixture string) Scenario {
	st := Observed
	if got != want {
		st = Failed
	}
	guidance := "evidence preserving; no move-aside"
	if want == "FOREIGN" {
		guidance = "generic move-aside/manual-clean-start"
	}
	return Scenario{ID: id, Requirements: []string{"VAL-024", "VAL-026"}, Status: st, Fixture: fixture, Classification: got, Guidance: guidance, Writes: []string{}, Observation: "complete on-disk topology and strict record graph classified as " + got}
}

type graphRefs struct {
	priorCurrent           currentRecord
	committedCurrent       currentRecord
	generation             uint64
	instance               string
	token                  string
	operationToken         string
	priorWriterToken       string // token named by the baseline released owner
	writerPredecessorToken string // current predecessor owner's Token; empty for a released predecessor
	writerToken            string
	a                      string
	b                      string
	receiptAPrevious       string
	receiptA               string
	receiptB               string
	journal                string
}

func freshGraphRefs() graphRefs {
	one := func() string {
		value, err := FreshToken()
		if err != nil {
			panic(err)
		}
		return value
	}
	a := string(candidateDigestFor("proof-current"))
	b := string(candidateDigestFor("proof-next"))
	receiptA := string(pendingReceiptFor(a, "").Digest)
	r := graphRefs{
		generation:             3,
		instance:               one(),
		token:                  one(),
		operationToken:         one(),
		priorWriterToken:       one(),
		writerPredecessorToken: "",
		writerToken:            one(),
		a:                      a,
		b:                      b,
		receiptA:               receiptA,
		receiptB:               string(pendingReceiptFor(b, receiptDigest(a)).Digest),
		journal:                one(),
	}
	r.priorWriterToken = stableCurrent(r.instance, r.a, r.receiptA, r.journal).WriterToken
	return r
}
func freshBootstrapRefs() graphRefs {
	r := freshGraphRefs()
	r.generation = 1
	r.a = ""
	r.receiptA = ""
	r.receiptB = string(pendingReceiptFor(r.b, "").Digest)
	return r
}

func makeGraph(base, graph string) (string, string, graphRefs) {
	s, d := resetRoots(base, true, true)
	r := freshGraphRefs()
	for _, name := range []string{"init.lock", "admission.lock", "writer.lock"} {
		_ = os.WriteFile(filepath.Join(s, name), nil, 0600)
	}
	for _, name := range []string{"candidates", "journals", "receipts"} {
		_ = os.Mkdir(filepath.Join(s, name), 0700)
	}
	_ = writeRecord(filepath.Join(s, "root.json"), initializedRoot(r.instance))
	_ = writeRecord(filepath.Join(d, "owner.json"), namespaceOwner(r.instance, "data"))
	_ = writeRecord(filepath.Join(s, "writer-owner.json"), releasedWriter(r.instance, "", 0, 0, ""))
	if graph == "READY(∅)" {
		r.generation = 1
		return s, d, r
	}
	r.priorCurrent = baselineCurrent(r)
	_ = writeRecord(filepath.Join(s, "writer-owner.json"), baselineReleasedWriter(r))
	_ = writeRecord(filepath.Join(s, "candidates", r.a+".json"), candidateReference(r.instance, r.a))
	_ = writeRecord(filepath.Join(s, "receipts", r.receiptA+".json"), pendingReceiptFor(r.a, ""))
	_ = writeRecord(filepath.Join(s, "current.json"), baselineCurrent(r))
	if graph == "STABLE(A)" {
		return s, d, r
	}
	transitionOwner := heldWriter(r.instance, r.writerToken, r.generation, processIdentity{PID: 999997, Start: 1, PIDNamespace: 1}, r.generation-1, transitionPreviousWriterToken(r))
	_ = writeRecord(filepath.Join(s, "writer-owner.json"), transitionOwner)
	_ = writeRecord(filepath.Join(s, "candidates", r.b+".json"), candidateReference(r.instance, r.b))
	journal := transitionJournalRecord(r, "mutating", string(phaseHomeApply), mutationPhases, r.generation, r.writerToken)
	_ = writeRecord(filepath.Join(s, "receipts", r.receiptB+".json"), transitionPendingReceipt(r))
	journal.Phase = phasePending
	if graph == "CLEANUP(B)" {
		journal.Phase = phaseCleanup
		_ = writeRecord(filepath.Join(s, "current.json"), transitionCurrent(r))
	}
	_ = writeRecord(filepath.Join(s, "journals", r.journal+".json"), journal)
	return s, d, r
}

func runClassifier(root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, x := range []struct {
		a, b bool
		name string
	}{{false, false, "absent-absent"}, {false, true, "absent-empty"}, {true, false, "empty-absent"}, {true, true, "empty-empty"}} {
		s, d := resetRoots(root.Path, x.a, x.b)
		add(classScenario("VAL-024/clean/"+x.name, classify(s, d), "CLEAN", x.name))
	}
	s, d := resetRoots(root.Path, true, true)
	_ = os.WriteFile(filepath.Join(s, "arbitrary"), []byte("unrecognized disposable fixture"), 0600)
	add(classScenario("VAL-024/foreign", classify(s, d), "FOREIGN", "arbitrary non-recognizable entry"))
	for _, graph := range []string{"READY(∅)", "STABLE(A)", "TRANSITION(A,B)", "CLEANUP(B)"} {
		s, d, _ = makeGraph(root.Path, graph)
		add(classScenario("VAL-026/graph/"+graph, classify(s, d), graph, graph+" exact graph"))
	}
	envelopeCases := []struct {
		name, content, want string
	}{
		{"whitespace-key-order", "{\n  \"domain\" : \"zerb.root\", \"schema\" : 1\n}", "BAD corrupt"},
		{"unsupported-schema-zero", "{\"domain\":\"zerb.root\",\"schema\":0}", "BAD unsupported"},
		{"unsupported-schema-negative", "{\"schema\":-1,\"domain\":\"zerb.root\"}", "BAD unsupported"},
		{"unsupported-schema-future", "{\"schema\":999,\"domain\":\"zerb.root\"}", "BAD unsupported"},
		{"unsupported-domain-empty", "{\"schema\":1,\"domain\":\"\"}", "BAD unsupported"},
		{"unsupported-domain-other", "{\"domain\":\"other\",\"schema\":1}", "BAD unsupported"},
		{"malformed-tagged-domain-prose", "arbitrary prose containing \"domain\" and \"zerb.root\" {", "FOREIGN"},
		{"malformed-tagged-schema-prose", "arbitrary prose containing \"schema\":1", "FOREIGN"},
		{"duplicate-schema", "{\"schema\":1,\"schema\":2,\"domain\":\"zerb.root\"}", "BAD corrupt"},
		{"duplicate-domain", "{\"schema\":1,\"domain\":\"zerb.root\",\"domain\":\"other\"}", "BAD corrupt"},
	}
	for _, test := range envelopeCases {
		s, d := resetRoots(root.Path, true, true)
		_ = os.WriteFile(filepath.Join(s, "opaque-evidence"), []byte(test.content), 0600)
		add(classScenario("VAL-026/envelope/"+test.name, classify(s, d), test.want, test.name))
	}
	for _, order := range []struct {
		name, corruptName, unsupportedName string
	}{
		{"corrupt-first", "a-corrupt.json", "z-unsupported.json"},
		{"unsupported-first", "z-corrupt.json", "a-unsupported.json"},
	} {
		s, d = resetRoots(root.Path, true, true)
		_ = os.WriteFile(filepath.Join(s, order.corruptName), []byte("{\"schema\":1}"), 0600)
		_ = os.WriteFile(filepath.Join(s, order.unsupportedName), []byte("{\"schema\":2,\"domain\":\"zerb.root\"}"), 0600)
		_, unsupportedFound, recognitionErr := inspectRecognizable(s, d)
		got := classify(s, d)
		status := Observed
		if recognitionErr == nil || !unsupportedFound || got != "BAD unsupported" {
			status = Failed
		}
		add(Scenario{ID: "VAL-026/envelope/same-root-unsupported-precedence/" + order.name, Requirements: []string{"VAL-026", "ARCH-016", "ARCH-024"}, Status: status, Fixture: "same-root independent corrupt and unsupported entries in " + order.name + " lexical order", Classification: got, Admission: "deny", Writes: []string{}, Observation: fmt.Sprintf("unsupported=%t first_corruption_retained=%t classification=%s", unsupportedFound, recognitionErr != nil, got)})
	}
	s, d = resetRoots(root.Path, true, true)
	inaccessible := filepath.Join(s, "inaccessible")
	_ = os.Mkdir(inaccessible, 0700)
	_ = os.WriteFile(filepath.Join(inaccessible, "evidence"), []byte("{\"schema\":1,\"domain\":\"zerb.root\"}"), 0600)
	_ = os.Chmod(inaccessible, 0000)
	_, _, inaccessibleErr := inspectRecognizable(s, d)
	inaccessibleClass := classify(s, d)
	inaccessibleStatus := Observed
	if inaccessibleErr == nil || inaccessibleClass != "BAD corrupt" {
		inaccessibleStatus = Failed
	}
	add(Scenario{ID: "VAL-026/envelope/inaccessible-entry", Requirements: []string{"VAL-026", "ARCH-016"}, Status: inaccessibleStatus, Fixture: "inaccessible entry beneath a candidate durable root", Classification: inaccessibleClass, Admission: "deny", Writes: []string{}, Observation: fmt.Sprintf("bounded no-follow recognition propagated inaccessible-entry error=%t and classified ambiguity as %s", inaccessibleErr != nil, inaccessibleClass)})
	_ = os.Chmod(inaccessible, 0700)
	s, d = resetRoots(root.Path, true, true)
	inaccessible = filepath.Join(s, "inaccessible")
	_ = os.Mkdir(inaccessible, 0700)
	_ = os.WriteFile(filepath.Join(inaccessible, "evidence"), []byte("{\"schema\":1,\"domain\":\"zerb.root\"}"), 0600)
	_ = os.Chmod(inaccessible, 0000)
	_ = writeRecord(filepath.Join(d, "owner.json"), struct {
		Schema int    `json:"schema"`
		Domain string `json:"domain"`
	}{Schema: 2, Domain: "zerb.data-owner"})
	_, unsupportedFound, precedenceErr := inspectRecognizable(s, d)
	precedenceClass := classify(s, d)
	precedenceStatus := Observed
	if precedenceErr == nil || !unsupportedFound || precedenceClass != "BAD unsupported" {
		precedenceStatus = Failed
	}
	add(Scenario{ID: "VAL-026/envelope/unsupported-precedence", Requirements: []string{"VAL-026", "ARCH-016", "ARCH-024"}, Status: precedenceStatus, Fixture: "unsupported exact data-owner envelope plus inaccessible state traversal", Classification: precedenceClass, Admission: "deny", Writes: []string{}, Observation: fmt.Sprintf("unsupported=%t traversal_error=%t classification=%s", unsupportedFound, precedenceErr != nil, precedenceClass)})
	_ = os.Chmod(inaccessible, 0700)
	s, d = resetRoots(root.Path, true, true)
	_ = os.Symlink("{\"schema\":2,\"domain\":\"zerb.root\"}", filepath.Join(s, "opaque-link"))
	add(classScenario("VAL-026/envelope/no-follow-symlink", classify(s, d), "FOREIGN", "symlink target bytes are not durable envelope content"))
	variants := []string{"valid", "absent", "unsupported-schema", "unsupported-domain", "malformed", "wrong-type", "wrong-mode", "hardlink", "symlink", "alias", "extra", "instance-mismatch", "cross-relation-mismatch"}
	for _, name := range append(append([]string{}, stateReserved...), dataReserved...) {
		for _, v := range variants {
			s, d, r := makeGraph(root.Path, "STABLE(A)")
			p := filepath.Join(s, name)
			if name == "owner.json" {
				p = filepath.Join(d, name)
			}
			want := "BAD corrupt"
			original, _ := os.Lstat(p)
			switch v {
			case "valid":
				want = "STABLE(A)"
			case "absent":
				_ = os.RemoveAll(p)
			case "unsupported-schema":
				_ = os.RemoveAll(p)
				_ = writeRecord(p, struct {
					Schema int    `json:"schema"`
					Domain string `json:"domain"`
				}{Schema: 2, Domain: "zerb.root"})
				want = "BAD unsupported"
			case "unsupported-domain":
				_ = os.RemoveAll(p)
				_ = writeRecord(p, struct {
					Schema int    `json:"schema"`
					Domain string `json:"domain"`
				}{Schema: 1, Domain: "unsupported"})
				want = "BAD unsupported"
			case "malformed":
				_ = os.RemoveAll(p)
				_ = os.WriteFile(p, []byte("{"), 0600)
			case "wrong-type":
				_ = os.RemoveAll(p)
				if original != nil && original.IsDir() {
					_ = os.WriteFile(p, []byte("wrong type"), 0600)
				} else {
					_ = os.Mkdir(p, 0700)
				}
			case "wrong-mode":
				_ = os.Chmod(p, 0644)
			case "hardlink":
				q := p + ".source"
				_ = os.WriteFile(q, []byte("x"), 0600)
				_ = os.RemoveAll(p)
				_ = os.Link(q, p)
			case "alias":
				_ = os.Link(filepath.Join(s, "current.json"), filepath.Join(s, "current-alias.json"))
			case "symlink":
				_ = os.RemoveAll(p)
				_ = os.Symlink("missing", p)
			case "extra":
				_ = os.WriteFile(filepath.Join(s, "unexpected"), []byte("x"), 0600)
			case "instance-mismatch":
				_ = os.RemoveAll(p)
				_ = writeRecord(p, namespaceOwner(r.b, "data"))
			case "cross-relation-mismatch":
				_ = writeRecord(filepath.Join(s, "current.json"), stableCurrent(r.instance, r.b, r.receiptA, r.journal))
			}
			add(classScenario("VAL-026/reserved/"+name+"/"+v, classify(s, d), want, name+":"+v))
			if v == "wrong-mode" {
				if info, err := os.Lstat(p); err == nil {
					mode := os.FileMode(0600)
					if info.IsDir() {
						mode = 0700
					}
					_ = os.Chmod(p, mode)
				}
			}
		}
	}
	s, d, _ = makeGraph(root.Path, "STABLE(A)")
	add(classScenario("VAL-026/wrong-owner-policy", classifyUID(s, d, os.Getuid()+1), "BAD corrupt", "valid bytes with mismatched required owner identity"))
	runClassifierRelations(root, add)
}

func runClassifierRelations(root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, name := range append(append([]string{}, stateReserved...), dataReserved...) {
		s, d := resetRoots(root.Path, true, true)
		p := filepath.Join(s, name)
		if name == "owner.json" {
			p = filepath.Join(d, name)
		}
		if name == "candidates" || name == "journals" || name == "receipts" {
			_ = os.Mkdir(p, 0700)
		} else {
			_ = os.WriteFile(p, nil, 0600)
		}
		want := "BAD corrupt"
		if name == "init.lock" {
			want = "INIT"
		}
		add(classScenario("VAL-026/reserved-alone/"+name, classify(s, d), want, "reserved name alone: "+name))
	}
	type relationCase struct {
		name   string
		mutate func(string, string, graphRefs)
	}
	cases := []relationCase{
		{"orphan-receipt", func(s, _ string, r graphRefs) {
			x, _ := FreshToken()
			_ = writeRecord(filepath.Join(s, "receipts", x+".json"), pendingReceipt(r.instance, r.a, r.journal))
		}},
		{"unrelated-journal", func(s, _ string, r graphRefs) {
			x, _ := FreshToken()
			value := transitionJournalRecord(r, "prepared", "", nil, 1, r.writerToken)
			value.OperationID = x
			_ = writeRecord(filepath.Join(s, "journals", x+".json"), value)
		}},
		{"second-current", func(s, _ string, r graphRefs) {
			_ = writeRecord(filepath.Join(s, "current.second.json"), baselineCurrent(r))
		}},
		{"candidate-filename-id-mismatch", func(s, _ string, r graphRefs) {
			_ = writeRecord(filepath.Join(s, "candidates", r.a+".json"), candidateReference(r.instance, r.b))
		}},
		{"receipt-candidate-mismatch", func(s, _ string, r graphRefs) {
			_ = writeRecord(filepath.Join(s, "receipts", r.receiptA+".json"), pendingReceipt(r.instance, r.b, r.journal))
		}},
		{"current-receipt-mismatch", func(s, _ string, r graphRefs) {
			_ = writeRecord(filepath.Join(s, "current.json"), stableCurrent(r.instance, r.a, r.receiptB, r.journal))
		}},
		{"duplicated-candidate-alias", func(s, _ string, r graphRefs) {
			_ = os.Link(filepath.Join(s, "candidates", r.a+".json"), filepath.Join(s, "candidates", r.a+"-alias.json"))
		}},
	}
	for _, c := range cases {
		s, d, r := makeGraph(root.Path, "STABLE(A)")
		c.mutate(s, d, r)
		add(classScenario("VAL-026/relation/"+c.name, classify(s, d), "BAD corrupt", c.name))
	}
	s, d, r := makeGraph(root.Path, "STABLE(A)")
	_ = writeRecord(filepath.Join(s, "candidates", r.b+".json"), candidateReference(r.instance, r.b))
	add(classScenario("VAL-026/relation/unreferenced-valid-candidate", classify(s, d), "STABLE(A)", "independently valid immutable candidate remains data, not completion authority"))
	for _, cacheCase := range []string{"absent", "empty-owned", "poisoned-foreign", "unrelated"} {
		s, d, _ := makeGraph(root.Path, "STABLE(A)")
		cache := filepath.Join(filepath.Dir(s), "cache")
		switch cacheCase {
		case "empty-owned":
			_ = os.Mkdir(cache, 0700)
		case "poisoned-foreign":
			_ = os.Mkdir(cache, 0700)
			_ = os.WriteFile(filepath.Join(cache, "managed-index.json"), []byte("{"), 0600)
		case "unrelated":
			_ = os.Mkdir(cache, 0700)
			_ = os.WriteFile(filepath.Join(cache, "other"), []byte("foreign"), 0600)
		}
		add(classScenario("VAL-026/cache/"+cacheCase, classify(s, d), "STABLE(A)", "cache is non-authoritative: "+cacheCase))
	}
	cleanupStages := []struct {
		name   string
		mutate func(string, graphRefs)
		want   string
	}{
		{"both-related-residues", func(string, graphRefs) {}, "CLEANUP(B)"},
		{"previous-candidate-removed", func(s string, r graphRefs) { _ = os.Remove(filepath.Join(s, "candidates", r.a+".json")) }, "CLEANUP(B)"},
		{"previous-receipt-removed", func(s string, r graphRefs) {
			_ = os.Remove(filepath.Join(s, "candidates", r.a+".json"))
			_ = os.Remove(filepath.Join(s, "receipts", r.receiptA+".json"))
		}, "CLEANUP(B)"},
		{"journal-removed", func(s string, r graphRefs) {
			_ = os.Remove(filepath.Join(s, "candidates", r.a+".json"))
			_ = os.Remove(filepath.Join(s, "receipts", r.receiptA+".json"))
			_ = os.Remove(filepath.Join(s, "journals", r.journal+".json"))
		}, "CLEANUP(B)"},
		{"owner-released", func(s string, r graphRefs) {
			_ = os.Remove(filepath.Join(s, "candidates", r.a+".json"))
			_ = os.Remove(filepath.Join(s, "receipts", r.receiptA+".json"))
			_ = os.Remove(filepath.Join(s, "journals", r.journal+".json"))
			_ = writeRecord(filepath.Join(s, "writer-owner.json"), releasedWriter(r.instance, "", r.generation+1, r.generation, r.writerToken))
		}, "STABLE(A)"},
	}
	for _, cleanupStage := range cleanupStages {
		s, d, r := makeGraph(root.Path, "CLEANUP(B)")
		cleanupStage.mutate(s, r)
		add(classScenario("VAL-026/cleanup-order/"+cleanupStage.name, classify(s, d), cleanupStage.want, "exact ordered cleanup prefix: "+cleanupStage.name))
	}
	s, d, r = makeGraph(root.Path, "CLEANUP(B)")
	_ = os.Remove(filepath.Join(s, "receipts", r.receiptA+".json"))
	add(classScenario("VAL-026/cleanup-order/gap-receipt-before-candidate", classify(s, d), "BAD corrupt", "cleanup cannot remove previous receipt before its related previous candidate"))
	s, d, _, ancestryRefs, ancestryErr := prepareExactReapplyFixture(root.Path, 3)
	ancestryClass := classify(s, d)
	ancestryStatus := Observed
	if ancestryErr != nil || ancestryClass != "STABLE(A)" {
		ancestryStatus = Failed
	}
	add(Scenario{ID: "VAL-026/receipt-ancestry/multi-generation-current", Requirements: []string{"VAL-026", "VAL-034"}, Status: ancestryStatus, Fixture: "stable current receipt with three exact-A receipt ancestors already cleaned", Classification: ancestryClass, Writes: []string{}, Observation: fmt.Sprintf("current receipt carries a nonempty exact receipt-digest ancestor=%t without requiring an initial-bootstrap candidate relation or retaining prior receipt files", ancestryRefs.receiptA != "")})
}

type proofLease struct {
	files []*os.File
	names []string
}

func (l *proofLease) release() {
	for i := len(l.files) - 1; i >= 0; i-- {
		_ = ofd(int(l.files[i].Fd()), fUNLCK)
		_ = l.files[i].Close()
	}
}

func (l *proofLease) acquireStableLock(stateDir, name string, typ int16) error {
	path := filepath.Join(stateDir, name)
	before, err := os.Lstat(path)
	if err != nil || !safeLockUID(path, os.Getuid()) {
		return fmt.Errorf("unsafe %s", name)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = ofd(int(f.Fd()), typ); err != nil {
		_ = f.Close()
		return err
	}
	opened, statErr := f.Stat()
	after, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = ofd(int(f.Fd()), fUNLCK)
		_ = f.Close()
		return fmt.Errorf("%s inode changed", name)
	}
	l.files = append(l.files, f)
	l.names = append(l.names, name)
	return nil
}

func acquireStableLease(stateDir string, requests ...struct {
	name string
	typ  int16
}) (*proofLease, error) {
	l := &proofLease{}
	for _, request := range requests {
		if err := l.acquireStableLock(stateDir, request.name, request.typ); err != nil {
			l.release()
			return nil, err
		}
	}
	return l, nil
}

func leaseAttemptBlocked(stateDir, name string, typ int16) bool {
	lease, err := acquireStableLease(stateDir, struct {
		name string
		typ  int16
	}{name, typ})
	if lease != nil {
		lease.release()
	}
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES)
}

func leaseAttemptAvailable(stateDir, name string, typ int16) bool {
	lease, err := acquireStableLease(stateDir, struct {
		name string
		typ  int16
	}{name, typ})
	if err != nil {
		return false
	}
	lease.release()
	return true
}

func admissionFixture(base, state string) (string, string) {
	switch state {
	case "READY(∅)", "STABLE(A)", "TRANSITION(A,B)", "CLEANUP(B)":
		s, d, _ := makeGraph(base, state)
		return s, d
	case "TRANSITION(∅,B)":
		s, d, r := makeGraph(base, "READY(∅)")
		r.a, r.receiptA = "", ""
		r.receiptB = string(pendingReceiptFor(r.b, "").Digest)
		identity, _ := processIdentityForPID(os.Getpid())
		_ = writeRecord(filepath.Join(s, "writer-owner.json"), heldWriter(r.instance, r.writerToken, 1, identity, 0, transitionPreviousWriterToken(r)))
		_ = writeRecord(filepath.Join(s, "candidates", r.b+".json"), transitionCandidate(r))
		_ = writeRecord(filepath.Join(s, "journals", r.journal+".json"), transitionJournal(r, "prepared", "", nil, 1, r.writerToken))
		return s, d
	case "INIT":
		s, d := resetRoots(base, true, false)
		_ = os.WriteFile(filepath.Join(s, "init.lock"), nil, 0600)
		return s, d
	case "BAD":
		s, d, _ := makeGraph(base, "STABLE(A)")
		_ = os.WriteFile(filepath.Join(s, "unexpected"), []byte("proof foreign extra"), 0600)
		return s, d
	case "FOREIGN":
		s, d := resetRoots(base, true, true)
		_ = os.WriteFile(filepath.Join(s, "arbitrary"), []byte("proof foreign"), 0600)
		return s, d
	default:
		return resetRoots(base, false, false)
	}
}

func runAdmission(ctx context.Context, root *prooffs.OwnedRoot, add func(Scenario)) {
	states := []string{"CLEAN", "FOREIGN", "INIT", "BAD", "READY(∅)", "STABLE(A)", "TRANSITION(∅,B)", "TRANSITION(A,B)", "CLEANUP(B)"}
	commands := []string{"version", "profile-list", "plan-exact", "plan-changed", "doctor", "recover-resume", "recover-repair", "bootstrap", "apply", "exec", "shell"}
	for _, state := range states {
		for _, command := range commands {
			s, d := admissionFixture(root.Path, state)
			add(routeCommand(s, d, state, command))
		}
	}
	runBootstrapHandoffFailures(root, add)
	runAdmissionContention(root, add)
	runAdmissionRaces(ctx, root, add)
}

func runBootstrapHandoffFailures(root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, injection := range []string{"held-owner-reopen-mismatch", "held-owner-reopen-altered", "released-owner-reopen-mismatch", "released-owner-reopen-altered"} {
		s, d := admissionFixture(root.Path, "CLEAN")
		failed := routeCommandInjected(s, d, "CLEAN", "bootstrap", injection)
		status := Observed
		boundary := "aborted before I-X consume and target unlock"
		if strings.HasPrefix(injection, "released-") {
			boundary = "aborted before target unlock after I-X consume"
		}
		if failed.Status != Failed || !strings.Contains(failed.Observation, boundary) ||
			!strings.Contains(failed.Observation, "admission_blocked=true") ||
			!strings.Contains(failed.Observation, "writer_blocked=true") {
			status = Failed
		}
		add(Scenario{
			ID:                 "VAL-033/admission/CLEAN/bootstrap/" + injection,
			Requirements:       []string{"VAL-030", "VAL-031", "VAL-033"},
			Status:             status,
			Fixture:            injection,
			Classification:     "READY(∅)",
			Command:            "bootstrap",
			Admission:          "fail closed",
			Lease:              "I-X+A-X+W-X",
			Writes:             []string{"fixtures/<typed-bootstrap-handoff>/state", "fixtures/<typed-bootstrap-handoff>/data", "fixtures/<typed-bootstrap-handoff>/cache", "fixtures/<typed-bootstrap-handoff>/external"},
			Observation:        failed.Observation,
			RejectedAssumption: "recording a failed scenario after consuming or unlocking the lease proves the handoff failed closed",
		})
	}
}

// initializationAcquisition models ARCH-030's returned ownership, including failed handoffs.
type initializationAcquisition struct {
	initialization *proofLease
	target         *proofLease
}

func (a *initializationAcquisition) consumeInitialization() {
	a.initialization.release()
	a.initialization = nil
}

func (a *initializationAcquisition) release() {
	if a.target != nil {
		a.target.release()
		a.target = nil
	}
	if a.initialization != nil {
		a.consumeInitialization()
	}
}

func publishBootstrapOwnerInjected(s, injection string, refs *graphRefs, identity processIdentity) (prooffs.PublicationIdentity, writerOwnerRecord, error) {
	var prior writerOwnerRecord
	token, tokenErr := FreshToken()
	priorErr := readRecord(filepath.Join(s, "writer-owner.json"), &prior)
	_, priorIdentityErr := exactRecordFile(filepath.Join(s, "writer-owner.json"), prior, nil)
	if tokenErr != nil || priorErr != nil || priorIdentityErr != nil || prior != releasedWriter(refs.instance, "", 0, 0, "") {
		return prooffs.PublicationIdentity{}, writerOwnerRecord{}, errors.New("initial writer predecessor was not exact")
	}
	refs.writerToken = token
	held := heldWriter(refs.instance, refs.writerToken, 1, identity, 0, "")
	heldIdentity, err := publishRecordIdentity(s, "writer-owner.json", held)
	if err != nil {
		return prooffs.PublicationIdentity{}, writerOwnerRecord{}, err
	}
	if injection == "held-owner-reopen-mismatch" || injection == "held-owner-reopen-altered" {
		substituted := held
		if injection == "held-owner-reopen-altered" {
			substituted.Instance = strings.Repeat("a", 32)
			if substituted.Instance == refs.instance {
				substituted.Instance = strings.Repeat("b", 32)
			}
		}
		if err = publishRecord(s, "writer-owner.json", substituted); err != nil {
			return prooffs.PublicationIdentity{}, writerOwnerRecord{}, err
		}
	}
	var reopened writerOwnerRecord
	_, heldIdentityErr := exactRecordFile(filepath.Join(s, "writer-owner.json"), held, &heldIdentity)
	ownerHeld := readRecord(filepath.Join(s, "writer-owner.json"), &reopened) == nil && reopened == held &&
		validWriterPredecessor(reopened) && heldIdentityErr == nil
	if !ownerHeld {
		return prooffs.PublicationIdentity{}, writerOwnerRecord{}, errors.New("held writer owner reopen mismatch")
	}
	released := releasedWriter(refs.instance, "", 2, 1, refs.writerToken)
	return heldIdentity, released, nil
}

func verifyReleasedBootstrapOwnerInjected(s, injection string, refs graphRefs, released writerOwnerRecord, releasedIdentity prooffs.PublicationIdentity) error {
	if injection == "released-owner-reopen-mismatch" || injection == "released-owner-reopen-altered" {
		substituted := released
		if injection == "released-owner-reopen-altered" {
			substituted.Instance = strings.Repeat("a", 32)
			if substituted.Instance == refs.instance {
				substituted.Instance = strings.Repeat("b", 32)
			}
		}
		if err := publishRecord(s, "writer-owner.json", substituted); err != nil {
			return err
		}
	}
	var releasedReopened writerOwnerRecord
	_, releasedIdentityErr := exactRecordFile(filepath.Join(s, "writer-owner.json"), released, &releasedIdentity)
	ownerReleased := readRecord(filepath.Join(s, "writer-owner.json"), &releasedReopened) == nil &&
		releasedReopened == released && validWriterPredecessor(releasedReopened) && releasedIdentityErr == nil
	if !ownerReleased {
		return errors.New("released writer owner reopen mismatch")
	}
	return nil
}

type initializationHandoffChecks struct {
	initialClassification, wantInitial                      string
	fullReady                                               bool
	contenderResult                                         commandContenderSnapshot
	continuedAfterConsume, ownerHeld, ownerReleased         bool
	initBlocked, admissionBlocked, writerBlocked            bool
	initConsumed, admissionStillBlocked, writerStillBlocked bool
}

func (c initializationHandoffChecks) failed() bool {
	return c.initialClassification != c.wantInitial || !c.fullReady || c.contenderResult.InitAttempts == 0 ||
		c.contenderResult.CurrentAttempts == 0 || c.contenderResult.Acquisitions != 0 || !c.continuedAfterConsume ||
		!c.ownerHeld || !c.ownerReleased || !c.initBlocked || !c.admissionBlocked || !c.writerBlocked || !c.initConsumed ||
		!c.admissionStillBlocked || !c.writerStillBlocked
}

func initializeCommandInjected(s, d, expectedState, command, injection string, scenario Scenario) (Scenario, initializationAcquisition) {
	acquisition := initializationAcquisition{}
	if expectedState == "CLEAN" {
		if err := os.MkdirAll(s, 0700); err != nil || createDurableEmpty(s, "init.lock") != nil {
			scenario.Status, scenario.Observation = Failed, "failed to create disposable stable initialization inode"
			return scenario, acquisition
		}
		scenario.Writes = []string{"fixtures/<typed-admission>/state"}
	}
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"init.lock", fWRLCK})
	if err != nil {
		scenario.Status, scenario.Observation = Failed, err.Error()
		return scenario, acquisition
	}
	acquisition.initialization = lease
	initialClassification := classify(s, d)
	scenario.Classification = initialClassification
	scenario.Lease = "I-X"
	scenario.Admission, scenario.Guidance = routePolicy(expectedState, command)
	if expectedState == "INIT" && command == "recover-repair" {
		scenario.Observation = "named router acquired and reopened the exact stable I-X inode for the separately exercised initialization-repair producer"
		return scenario, acquisition
	}
	refs := freshGraphRefs()
	if command == "bootstrap" {
		refs = freshBootstrapRefs()
	}
	identity, identityErr := processIdentityForPID(os.Getpid())
	if identityErr != nil {
		scenario.Status, scenario.Observation = Failed, identityErr.Error()
		return scenario, acquisition
	}
	cache := filepath.Join(filepath.Dir(s), "cache")
	if err = initializeReadyPrefix(s, d, cache, refs, identity, lease); err != nil {
		scenario.Status, scenario.Observation = Failed, err.Error()
		return scenario, acquisition
	}
	initializationAuthority, authorityErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "cache": cache})
	if authorityErr != nil {
		scenario.Status, scenario.Observation = Failed, authorityErr.Error()
		return scenario, acquisition
	}
	external := ""
	if command == "bootstrap" {
		external = filepath.Join(filepath.Dir(s), "external")
		if err = os.Mkdir(external, 0700); err != nil || syncDir(filepath.Dir(external)) != nil {
			scenario.Status, scenario.Observation = Failed, "failed to create disposable external transition root"
			return scenario, acquisition
		}
	}
	scenario.Writes = []string{"fixtures/<typed-admission>/state", "fixtures/<typed-admission>/data", "fixtures/<typed-admission>/cache"}
	if command == "bootstrap" {
		scenario.Writes = append(scenario.Writes, "fixtures/<typed-admission>/external")
	}
	requests := []struct {
		name string
		typ  int16
	}{{"admission.lock", fWRLCK}}
	scenario.Lease = "I-X+A-X"
	if command == "bootstrap" {
		requests = append(requests, struct {
			name string
			typ  int16
		}{"writer.lock", fWRLCK})
		scenario.Lease = "I-X+A-X+W-X"
	}
	commandContender := startContinuousCommandContender(s, d, command == "bootstrap")
	if !waitForContender(commandContender, func(snapshot commandContenderSnapshot) bool {
		return snapshot.InitAttempts > 0
	}) {
		commandContender.stopAndWait()
		scenario.Status, scenario.Observation = Failed, "current-command contender did not attempt I-X while root remained initializing"
		return scenario, acquisition
	}
	beforeTarget := commandContender.snapshot()
	if injection == "target-acquisition-failure" {
		commandContender.stopAndWait()
		scenario.Status, scenario.Observation = Failed, "target acquisition failed before I-X consume"
		return scenario, acquisition
	}
	currentLease, err := acquireStableLease(s, requests...)
	if err != nil {
		commandContender.stopAndWait()
		scenario.Status, scenario.Observation = Failed, err.Error()
		return scenario, acquisition
	}
	acquisition.target = currentLease
	if !waitForContender(commandContender, func(snapshot commandContenderSnapshot) bool {
		return snapshot.Attempts > beforeTarget.Attempts
	}) {
		commandContender.stopAndWait()
		scenario.Status, scenario.Observation = Failed, "current-command contender did not remain active across target acquisition"
		return scenario, acquisition
	}
	finalizeFailure := func(observation string) Scenario {
		commandContender.stopAndWait()
		admissionBlocked := leaseAttemptBlocked(s, "admission.lock", fWRLCK)
		writerBlocked := command != "bootstrap" || leaseAttemptBlocked(s, "writer.lock", fWRLCK)
		scenario.Status = Failed
		scenario.Observation = fmt.Sprintf("%s aborted before I-X consume and target unlock; admission_blocked=%t writer_blocked=%t", observation, admissionBlocked, writerBlocked)
		return scenario
	}
	reopenedPrefix, reopenErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "cache": cache})
	if reopenErr != nil || !fixtureObjectsEqual(initializationAuthority, reopenedPrefix) {
		return finalizeFailure("initialization protected prefix changed before final-root publication"), acquisition
	}
	if err = publishInitializedRoot(s, refs.instance); err != nil {
		return finalizeFailure(err.Error()), acquisition
	}
	publishedPrefix, publishErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "cache": cache})
	if publishErr != nil || !fixtureObjectsEqualExceptKeys(initializationAuthority, publishedPrefix, map[string]bool{"state:.": true, "state:root.json": true}) {
		return finalizeFailure("initialized-root publication changed unauthorized object"), acquisition
	}
	if !waitForContender(commandContender, func(snapshot commandContenderSnapshot) bool {
		return snapshot.CurrentAttempts > 0
	}) {
		return finalizeFailure("current-command contender did not attempt the visible initialized relation"), acquisition
	}
	ready := classify(s, d)
	fullReady := ready == "READY(∅)" && fullReadyGraphValid(s, d, cache, refs.instance)
	ownerHeld, ownerReleased := true, true
	var heldIdentity, releasedIdentity prooffs.PublicationIdentity
	var released writerOwnerRecord
	if command == "bootstrap" {
		heldIdentity, released, err = publishBootstrapOwnerInjected(s, injection, &refs, identity)
		if err != nil {
			return finalizeFailure(err.Error()), acquisition
		}
	}
	initBlocked := leaseAttemptBlocked(s, "init.lock", fWRLCK)
	admissionBlocked := leaseAttemptBlocked(s, "admission.lock", fWRLCK)
	writerBlocked := command != "bootstrap" || leaseAttemptBlocked(s, "writer.lock", fWRLCK)
	beforeConsume := commandContender.snapshot()
	acquisition.consumeInitialization()
	initConsumed := leaseAttemptAvailable(s, "init.lock", fWRLCK)
	admissionStillBlocked := leaseAttemptBlocked(s, "admission.lock", fWRLCK)
	writerStillBlocked := command != "bootstrap" || leaseAttemptBlocked(s, "writer.lock", fWRLCK)
	continuedAfterConsume := waitForContender(commandContender, func(snapshot commandContenderSnapshot) bool {
		return snapshot.Attempts > beforeConsume.Attempts && snapshot.CurrentAttempts > beforeConsume.CurrentAttempts
	})
	finalizeAfterConsume := func(observation string) Scenario {
		commandContender.stopAndWait()
		admissionHeld := leaseAttemptBlocked(s, "admission.lock", fWRLCK)
		writerHeld := command != "bootstrap" || leaseAttemptBlocked(s, "writer.lock", fWRLCK)
		scenario.Status = Failed
		scenario.Observation = fmt.Sprintf("%s aborted before target unlock after I-X consume; admission_blocked=%t writer_blocked=%t", observation, admissionHeld, writerHeld)
		return scenario
	}
	if command == "bootstrap" {
		if err = executeTransitionWithLease(s, d, external, refs, identity, nil, nil, currentLease, true, heldIdentity, &releasedIdentity); err != nil {
			return finalizeAfterConsume(err.Error()), acquisition
		}
		if err = verifyReleasedBootstrapOwnerInjected(s, injection, refs, released, releasedIdentity); err != nil {
			return finalizeAfterConsume(err.Error()), acquisition
		}
	}
	contenderResult := commandContender.snapshot()
	commandContender.stopAndWait()
	wantInitial := expectedState
	if expectedState == "CLEAN" {
		wantInitial = "INIT"
	}
	checks := initializationHandoffChecks{
		initialClassification: initialClassification, wantInitial: wantInitial,
		fullReady: fullReady, contenderResult: contenderResult,
		continuedAfterConsume: continuedAfterConsume, ownerHeld: ownerHeld, ownerReleased: ownerReleased,
		initBlocked: initBlocked, admissionBlocked: admissionBlocked, writerBlocked: writerBlocked,
		initConsumed: initConsumed, admissionStillBlocked: admissionStillBlocked, writerStillBlocked: writerStillBlocked,
	}
	if checks.failed() {
		scenario.Status = Failed
	}
	scenario.Observation = fmt.Sprintf("router retained stable I-X from initializing classification %s; published and revalidated the exact initialization prefix; continuous current-command contender attempted I-X before and across target acquisition, then exact A-X after final-root publication and I-X consume without acquiring: attempts=%d init=%d current=%d acquisitions=%d; reopened and classified exact READY=%t; incompatible locks blocked before I-X consume init=%t admission=%t writer=%t and after consume admission=%t writer=%t; returned_target_lease=true", initialClassification, contenderResult.Attempts, contenderResult.InitAttempts, contenderResult.CurrentAttempts, contenderResult.Acquisitions, fullReady, initBlocked, admissionBlocked, writerBlocked, admissionStillBlocked, writerStillBlocked)
	if command == "bootstrap" {
		scenario.Boundaries.CandidateRevalidations++
		scenario.Boundaries.ManagerRevalidations++
		scenario.Observation += fmt.Sprintf("; bootstrap alone retained W-X, reopened held writer authority through I-X consume, completed the candidate/journal/mutation/receipt/current/cleanup transition, and published/reopened released owner only at operation release=%t/%t", ownerHeld, ownerReleased)
	}
	return scenario, acquisition
}

func routeCommand(s, d, expectedState, command string) Scenario {
	return routeCommandInjected(s, d, expectedState, command, "")
}

func routeCommandInjected(s, d, expectedState, command, injection string) Scenario {
	scenario := Scenario{ID: "VAL-033/admission/" + expectedState + "/" + command, Requirements: []string{"VAL-033"}, Status: Observed, Fixture: expectedState, Command: command, Writes: []string{}}
	if command == "version" {
		scenario.Classification, scenario.Admission, scenario.Lease, scenario.Guidance = "not-opened", "admit", "none", "no root access"
		scenario.Observation = "named router parsed version and returned without opening or classifying either root"
		return scenario
	}
	if expectedState == "FOREIGN" {
		scenario.Classification = classify(s, d)
		scenario.Lease = "topology-only"
		scenario.Admission, scenario.Guidance = routePolicy(scenario.Classification, command)
		scenario.Observation = "named router classified foreign topology without creating or acquiring synchronization paths"
		return scenario
	}
	if expectedState == "CLEAN" || expectedState == "INIT" {
		initializes := expectedState == "CLEAN" && (command == "profile-list" || strings.HasPrefix(command, "plan-") || command == "bootstrap")
		repairsInitialization := expectedState == "INIT" && command == "recover-repair"
		if !initializes && !repairsInitialization {
			scenario.Classification = classify(s, d)
			scenario.Lease = "D-S"
			scenario.Admission, scenario.Guidance = routePolicy(expectedState, command)
			scenario.Observation = "named router took a read-only diagnostic topology snapshot, made no product-root write, and returned without initialization"
			return scenario
		}
		initialized, acquisition := initializeCommandInjected(s, d, expectedState, command, injection, scenario)
		returnedTarget := acquisition.target != nil
		acquisition.release()
		if initialized.Status == Observed && returnedTarget {
			admissionAvailable := leaseAttemptAvailable(s, "admission.lock", fWRLCK)
			writerAvailable := command != "bootstrap" || leaseAttemptAvailable(s, "writer.lock", fWRLCK)
			if !admissionAvailable || !writerAvailable {
				initialized.Status = Failed
			}
			initialized.Observation += fmt.Sprintf("; caller released returned acquisition: admission_available=%t writer_available=%t", admissionAvailable, writerAvailable)
		}
		return initialized
	}
	leaseName, typ := "A-X", int16(fWRLCK)
	if command == "doctor" || command == "exec" || command == "shell" {
		leaseName, typ = "A-S", fRDLCK
	}
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", typ})
	if err != nil {
		scenario.Status, scenario.Observation = Failed, err.Error()
		return scenario
	}
	defer lease.release()
	scenario.Classification = classify(s, d)
	scenario.Admission, scenario.Guidance = routePolicy(scenario.Classification, command)
	mutation := command == "recover-resume" || command == "recover-repair" || command == "bootstrap" || command == "apply"
	if scenario.Admission == "admit" && mutation {
		if err = lease.acquireStableLock(s, "writer.lock", fWRLCK); err != nil {
			scenario.Status, scenario.Observation = Failed, err.Error()
			return scenario
		}
		leaseName += "+W-X"
	}
	scenario.Lease = leaseName
	if scenario.Admission == "deny" {
		scenario.Observation = "named router parsed the operation; acquired and reopened the exact intent admission lease; classified topology and applied PC-016 under that lease; returned the denial with zero source, candidate, manager, destination, or child action; released the unchanged admission lease without upgrade, downgrade, or reacquire"
		return scenario
	}
	scenario.Observation = "named router parsed the operation; acquired and reopened the exact intent admission lease; classified topology and applied PC-016 under that lease; admitted mutation/recovery appended W-X only after classification while retaining the same admission descriptor; this routing proof performed zero source, candidate, manager, destination, or child action; released in reverse order without upgrade, downgrade, or admission reacquire"
	return scenario
}

type commandContenderSnapshot struct {
	Attempts        uint64
	InitAttempts    uint64
	CurrentAttempts uint64
	Acquisitions    uint64
}

type continuousCommandContender struct {
	stateDir      string
	dataDir       string
	writer        bool
	stop          chan struct{}
	done          chan struct{}
	mu            sync.Mutex
	snapshotValue commandContenderSnapshot
}

func startContinuousCommandContender(stateDir, dataDir string, writer bool) *continuousCommandContender {
	c := &continuousCommandContender{stateDir: stateDir, dataDir: dataDir, writer: writer, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for {
			select {
			case <-c.stop:
				return
			default:
			}
			initializing := classify(c.stateDir, c.dataDir) == "INIT"
			acquired := false
			if initializing {
				acquired = leaseAttemptAvailable(c.stateDir, "init.lock", fWRLCK)
			} else {
				requests := []struct {
					name string
					typ  int16
				}{{"admission.lock", fWRLCK}}
				if c.writer {
					requests = append(requests, struct {
						name string
						typ  int16
					}{"writer.lock", fWRLCK})
				}
				if lease, err := acquireStableLease(c.stateDir, requests...); err == nil {
					acquired = true
					lease.release()
				}
			}
			c.mu.Lock()
			c.snapshotValue.Attempts++
			if initializing {
				c.snapshotValue.InitAttempts++
			} else {
				c.snapshotValue.CurrentAttempts++
			}
			if acquired {
				c.snapshotValue.Acquisitions++
			}
			c.mu.Unlock()
			runtime.Gosched()
		}
	}()
	return c
}

func (c *continuousCommandContender) snapshot() commandContenderSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotValue
}

func waitForContender(c *continuousCommandContender, predicate func(commandContenderSnapshot) bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if predicate(c.snapshot()) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func (c *continuousCommandContender) stopAndWait() {
	close(c.stop)
	<-c.done
}

func initializeReadyPrefix(s, d, cache string, refs graphRefs, identity processIdentity, lease *proofLease) error {
	if len(lease.files) != 1 || len(lease.names) != 1 || lease.names[0] != "init.lock" ||
		!exactLeafNames(s, "init.lock") || exists(d) || exists(cache) {
		return errors.New("initialization did not begin from exact init-lock-only prefix")
	}
	opened, statErr := lease.files[0].Stat()
	pathInfo, pathErr := os.Lstat(filepath.Join(s, "init.lock"))
	if statErr != nil || pathErr != nil || !os.SameFile(opened, pathInfo) ||
		!safeLockUID(filepath.Join(s, "init.lock"), os.Geteuid()) {
		return errors.New("initialization lock authority substitution")
	}
	before, err := actualFixtureObjects(map[string]string{"state": s})
	if err != nil {
		return err
	}
	claim := initializingRoot(refs.instance, refs.token, 0, identity)
	if err = publishRecord(s, "root.json", claim); err != nil {
		return err
	}
	after, err := actualFixtureObjects(map[string]string{"state": s})
	if err != nil || !fixtureObjectsEqualExceptKeys(before, after, map[string]bool{"state:.": true, "state:root.json": true}) ||
		!exactRecordBytes(filepath.Join(s, "root.json"), claim) {
		return errors.New("initialization claim publication changed unauthorized object")
	}
	spec := workerSpecFor(s, d, cache, "", "", refs)
	return completeInitializationPrefixUnderLease(spec, claim, lease, false)
}

func publishInitializedRoot(s, instance string) error {
	root := initializedRoot(instance)
	if err := publishRecord(s, "root.json", root); err != nil {
		return err
	}
	if !exactRecordBytes(filepath.Join(s, "root.json"), root) {
		return errors.New("initialized root publication reopen mismatch")
	}
	return nil
}

func fullReadyGraphValid(s, d, cache, instance string) bool {
	return classify(s, d) == "READY(∅)" &&
		exactRecordBytes(filepath.Join(s, "root.json"), initializedRoot(instance)) &&
		exactRecordBytes(filepath.Join(d, "owner.json"), namespaceOwner(instance, "data")) &&
		exactLeafNames(cache, "owner.json") &&
		exactRecordBytes(filepath.Join(cache, "owner.json"), namespaceOwner(instance, "cache")) &&
		exactRecordBytes(filepath.Join(s, "writer-owner.json"), releasedWriter(instance, "", 0, 0, "")) &&
		safeLockUID(filepath.Join(s, "admission.lock"), os.Geteuid()) &&
		safeLockUID(filepath.Join(s, "writer.lock"), os.Geteuid()) &&
		emptyDir(filepath.Join(s, "candidates")) && emptyDir(filepath.Join(s, "journals")) &&
		emptyDir(filepath.Join(s, "receipts"))
}

func routePolicy(state, command string) (string, string) {
	if state == "FOREIGN" || strings.HasPrefix(state, "BAD") {
		if command == "doctor" {
			return "diagnose", "evidence preserving"
		}
		return "deny", "state denial before source or candidate use"
	}
	if state == "INIT" {
		if command == "doctor" {
			return "diagnose", "no implicit repair"
		}
		if command == "recover-repair" {
			return "admit", "exact initialization repair"
		}
		return "deny", "initialization incomplete"
	}
	if state == "CLEAN" {
		if command == "doctor" {
			return "diagnose", "read only"
		}
		if command == "profile-list" || strings.HasPrefix(command, "plan-") || command == "bootstrap" {
			return "admit", "initialize before current locks"
		}
		return "deny", "no stable operation"
	}
	if strings.HasPrefix(state, "TRANSITION(") {
		if command == "doctor" {
			return "diagnose", "read only"
		}
		if strings.HasPrefix(command, "recover-") {
			return "admit", "forward only"
		}
		return "deny", "recovery precedes operation"
	}
	if state == "CLEANUP(B)" {
		if command == "profile-list" || command == "doctor" || command == "exec" || command == "shell" {
			return "admit", "stable B"
		}
		if strings.HasPrefix(command, "recover-") {
			return "admit", "finish cleanup"
		}
		return "deny", "cleanup first"
	}
	if command == "profile-list" || command == "plan-exact" || command == "doctor" || command == "bootstrap" || command == "apply" {
		return "admit", "lease lifetime"
	}
	if command == "plan-changed" {
		return "reject changed result", "candidate identity changed before publication"
	}
	if (command == "exec" || command == "shell") && state == "STABLE(A)" {
		return "admit", "through child release and reap"
	}
	return "deny", "operation unavailable in topology"
}

func runAdmissionContention(root *prooffs.OwnedRoot, add func(Scenario)) {
	s, d, _ := makeGraph(root.Path, "STABLE(A)")
	holder, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fWRLCK})
	if err != nil {
		add(Scenario{ID: "VAL-033/router/incompatible-busy", Requirements: []string{"VAL-033"}, Status: Failed, Writes: []string{}, Observation: err.Error()})
		return
	}
	busy := routeCommand(s, d, "STABLE(A)", "doctor")
	holder.release()
	later := routeCommand(s, d, "STABLE(A)", "doctor")
	status := Observed
	if busy.Status != Failed || !strings.Contains(busy.Observation, "resource temporarily unavailable") || later.Status != Observed || later.Classification != "STABLE(A)" {
		status = Failed
	}
	add(Scenario{ID: "VAL-033/router/incompatible-busy", Requirements: []string{"VAL-030", "VAL-033"}, Status: status, Fixture: "A-X holder versus doctor A-S", Classification: later.Classification, Command: "doctor", Admission: "busy=1 then admit", Lease: "A-X versus A-S", Writes: []string{}, Boundaries: later.Boundaries, Observation: "incompatible nonblocking loser returned busy before semantic validation; later invocation acquired and reclassified after release"})
}

func runAdmissionRaces(ctx context.Context, root *prooffs.OwnedRoot, add func(Scenario)) {
	for _, race := range []struct{ name, holder, loser string }{{"plan-recovery", "recover-resume", "plan-exact"}, {"exec-apply", "exec", "apply"}} {
		s, d, _ := makeGraph(root.Path, "STABLE(A)")
		holderType := int16(fWRLCK)
		if race.holder == "exec" {
			holderType = fRDLCK
		}
		holder, err := acquireStableLease(s, struct {
			name string
			typ  int16
		}{"admission.lock", holderType})
		if err != nil {
			add(Scenario{ID: "VAL-036/race/" + race.name, Requirements: []string{"VAL-036"}, Status: Failed, Writes: []string{}, Observation: err.Error()})
			continue
		}
		loser := routeCommand(s, d, "STABLE(A)", race.loser)
		holder.release()
		later := routeCommand(s, d, "STABLE(A)", race.loser)
		status := Observed
		if loser.Status != Failed || later.Status != Observed {
			status = Failed
		}
		add(Scenario{ID: "VAL-036/race/" + race.name, Requirements: []string{"VAL-033", "VAL-036"}, Status: status, Fixture: race.holder + " holder versus " + race.loser, Classification: later.Classification, Command: race.loser, Admission: "busy=1 then reclassified", Lease: "incompatible admission leases", Writes: []string{}, Observation: "a fresh incompatible OFD lost nonblocking at the contention boundary; after holder release a fresh invocation acquired and reclassified; this row proves only lease incompatibility, not a plan lifetime"})
	}
	status, observation := exercisePlanLifetime(ctx, root)
	add(Scenario{
		ID: "VAL-036/plan-full-lifetime", Requirements: []string{"VAL-036"}, Status: status,
		Fixture: "single plan A-X OFD with staged cache, interruptible approval, and immutable candidate publication",
		Command: "plan-exact", Admission: "admit", Lease: "one continuous A-X OFD", Writes: []string{"fixtures/<typed-plan-lifetime>/state/candidates/<typed-candidate>.json"},
		Observation:        observation,
		RejectedAssumption: "boundary counters or an in-process boolean establish plan lease lifetime",
	})
	s, d, _ := makeGraph(root.Path, "STABLE(A)")
	_ = os.Remove(filepath.Join(s, "init.lock"))
	_ = os.Symlink("missing", filepath.Join(s, "init.lock"))
	got := classify(s, d)
	add(Scenario{ID: "VAL-033/router/unsafe-init-topology-only", Requirements: []string{"VAL-026", "VAL-033"}, Status: Observed, Fixture: "unsafe init.lock symlink", Classification: got, Command: "doctor", Admission: "diagnose", Lease: "topology-only", Writes: []string{}, Observation: "router refused to open unsafe init.lock and performed exact no-write topology diagnosis; no synchronization path was replaced"})
}

func planLeaseExcludesReaders(s string, lease *proofLease, lockIdentity os.FileInfo) bool {
	heldIdentity, statErr := lease.files[0].Stat()
	pathIdentity, pathErr := os.Lstat(filepath.Join(s, "admission.lock"))
	if statErr != nil || pathErr != nil || !os.SameFile(lockIdentity, heldIdentity) || !os.SameFile(heldIdentity, pathIdentity) {
		return false
	}
	competitor, acquireErr := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fRDLCK})
	if acquireErr == nil {
		competitor.release()
		return false
	}
	if !errors.Is(acquireErr, syscall.EAGAIN) && !errors.Is(acquireErr, syscall.EACCES) {
		return false
	}
	return true
}

func exercisePlanLifetime(ctx context.Context, root *prooffs.OwnedRoot) (Status, string) {
	s, d, refs := makeGraph(root.Path, "READY(∅)")
	cache := filepath.Join(filepath.Dir(s), "plan-cache")
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fWRLCK})
	if err != nil {
		return Failed, "failed to acquire the plan A-X OFD"
	}
	released := false
	defer func() {
		if !released {
			lease.release()
		}
	}()
	if len(lease.files) != 1 {
		return Failed, "plan acquired an unexpected descriptor count"
	}
	lockIdentity, err := lease.files[0].Stat()
	if err != nil {
		return Failed, "failed to identify the held plan OFD"
	}
	boundaries, blocked := 0, 0
	checkBoundary := func() bool {
		boundaries++
		if !planLeaseExcludesReaders(s, lease, lockIdentity) {
			return false
		}
		blocked++
		return true
	}
	if classify(s, d) != "READY(∅)" || !checkBoundary() {
		return Failed, "plan failed root revalidation under the held OFD"
	}
	if err = os.Mkdir(cache, 0700); err != nil || !checkBoundary() {
		return Failed, "plan cache root creation did not retain the held OFD"
	}
	cacheStage := filepath.Join(cache, "candidate.stage")
	cacheFile, err := os.OpenFile(cacheStage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Failed, "plan cache staging failed"
	}
	candidateBytes := recordBytes(candidateReference(refs.instance, refs.a))
	if _, err = cacheFile.Write(candidateBytes); err == nil {
		err = cacheFile.Chmod(0600)
	}
	if err == nil {
		err = cacheFile.Sync()
	}
	closeErr := cacheFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || syncDir(cache) != nil || !checkBoundary() {
		return Failed, "plan cache work was not durable while the OFD excluded a competitor"
	}
	approval := make(chan struct{})
	close(approval)
	select {
	case <-ctx.Done():
		return Failed, "plan approval wait was interrupted by the proof context"
	case <-approval:
	}
	if !checkBoundary() {
		return Failed, "plan approval boundary did not retain the OFD"
	}
	candidateDir := filepath.Join(s, "candidates")
	stagingName := ".candidate-" + refs.operationToken
	stagingPath := filepath.Join(candidateDir, stagingName)
	stagingFile, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Failed, "candidate staging create failed"
	}
	if _, err = stagingFile.Write(candidateBytes); err != nil || !checkBoundary() {
		_ = stagingFile.Close()
		return Failed, "candidate staging write did not retain the OFD"
	}
	if err = stagingFile.Chmod(0600); err == nil {
		err = stagingFile.Sync()
	}
	if err != nil || !checkBoundary() {
		_ = stagingFile.Close()
		return Failed, "candidate file sync did not retain the OFD"
	}
	if err = stagingFile.Close(); err != nil {
		return Failed, "candidate staging close failed"
	}
	candidatePath := filepath.Join(candidateDir, refs.a+".json")
	if err = os.Link(stagingPath, candidatePath); err != nil || !checkBoundary() {
		return Failed, "atomic no-replace candidate publication failed"
	}
	if err = os.Remove(stagingPath); err != nil {
		return Failed, "candidate staging cleanup failed"
	}
	if err = syncDir(candidateDir); err != nil || !checkBoundary() {
		return Failed, "candidate parent sync did not retain the OFD"
	}
	reopened, err := readNoFollowBounded(candidatePath, maxRecognitionFile)
	if err != nil || !bytes.Equal(reopened, candidateBytes) || !safeRegularUID(candidatePath, os.Geteuid()) || !checkBoundary() {
		return Failed, "candidate reopen verification failed under the held OFD"
	}
	if err = os.Remove(cacheStage); err != nil || os.Remove(cache) != nil || syncDir(filepath.Dir(cache)) != nil || !checkBoundary() {
		return Failed, "plan cache cleanup did not retain the OFD"
	}
	lease.release()
	released = true
	after, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fRDLCK})
	if err != nil {
		return Failed, "competitor could not acquire after the plan OFD release"
	}
	after.release()
	if !exerciseCancelledPlanApproval(root) {
		return Failed, "cancellation did not interrupt approval and clean staging under one A-X OFD"
	}
	return Observed, fmt.Sprintf("one A-X open-file-description remained identity-stable across %d root/cache/approval/publication/sync/reopen/cleanup boundaries; incompatible fresh OFD competitors lost at every boundary (%d/%d), exact candidate bytes were published atomically without replacement and reopened after parent sync, a canceled approval path cleaned cache staging before release, and a competitor acquired only after release", boundaries, blocked, boundaries)
}

func exerciseCancelledPlanApproval(root *prooffs.OwnedRoot) bool {
	s, _, refs := makeGraph(root.Path, "READY(∅)")
	cache := filepath.Join(filepath.Dir(s), "cancelled-plan-cache")
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fWRLCK})
	if err != nil {
		return false
	}
	if os.Mkdir(cache, 0700) != nil || os.WriteFile(filepath.Join(cache, "stage"), recordBytes(candidateReference(refs.instance, refs.a)), 0600) != nil {
		lease.release()
		return false
	}
	blocked := func() bool {
		competitor, acquireErr := acquireStableLease(s, struct {
			name string
			typ  int16
		}{"admission.lock", fRDLCK})
		if acquireErr == nil {
			competitor.release()
			return false
		}
		return errors.Is(acquireErr, syscall.EAGAIN) || errors.Is(acquireErr, syscall.EACCES)
	}
	if !blocked() {
		lease.release()
		return false
	}
	approval := make(chan struct{})
	waitContext, cancel := context.WithCancel(context.Background())
	cancel()
	interrupted := false
	select {
	case <-approval:
	case <-waitContext.Done():
		interrupted = true
	}
	cleaned := os.RemoveAll(cache) == nil && syncDir(filepath.Dir(cache)) == nil && blocked() &&
		emptyDir(filepath.Join(s, "candidates"))
	lease.release()
	after, afterErr := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fRDLCK})
	if afterErr == nil {
		after.release()
	}
	return interrupted && cleaned && afterErr == nil
}

const (
	fOFDSetLK = 37
	fRDLCK    = 0
	fWRLCK    = 1
	fUNLCK    = 2
)

func ofd(fd int, typ int16) error {
	l := syscall.Flock_t{Type: typ}
	return syscall.FcntlFlock(uintptr(fd), fOFDSetLK, &l)
}
func runLocks(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	p := filepath.Join(root.Path, "admission-proof.lock")
	_ = os.WriteFile(p, nil, 0600)
	a, _ := os.OpenFile(p, os.O_RDWR, 0)
	b, _ := os.OpenFile(p, os.O_RDWR, 0)
	defer a.Close()
	defer b.Close()
	e1 := ofd(int(a.Fd()), fRDLCK)
	e2 := ofd(int(b.Fd()), fRDLCK)
	add(lockScenario("A-S+A-S", e1 == nil && e2 == nil, "compatible shared OFD locks"))
	_ = ofd(int(a.Fd()), fUNLCK)
	_ = ofd(int(b.Fd()), fUNLCK)
	_ = ofd(int(a.Fd()), fWRLCK)
	e2 = ofd(int(b.Fd()), fRDLCK)
	add(lockScenario("A-X+A-S", e2 != nil, "nonblocking exclusive precedence"))
	_ = ofd(int(a.Fd()), fUNLCK)
	_ = ofd(int(a.Fd()), fWRLCK)
	e2 = ofd(int(b.Fd()), fWRLCK)
	add(lockScenario("A-X+A-X", e2 != nil, "simultaneous writers excluded"))
	_ = ofd(int(a.Fd()), fUNLCK)
	runLockRaces(ctx, cfg, root, add)
}
func runGenerationOverflowProof(root *prooffs.OwnedRoot, add func(Scenario)) {
	const total = 8
	verified := 0
	var failures []string
	maxGeneration := ^uint64(0)
	if !validWriterPredecessor(heldWriter(
		"overflow-instance", strings.Repeat("a", 32), 0,
		processIdentity{PID: 1, Start: 1, PIDNamespace: 1},
		maxGeneration, strings.Repeat("b", 32),
	)) {
		verified++
	} else {
		failures = append(failures, "writer-predecessor")
	}

	spec, err := prepareInitializationRepairFixture(root.Path, "")
	if err == nil {
		dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
		claim := initializingRoot(spec.Instance, spec.Token, maxGeneration, dead)
		err = publishRecord(spec.State, "root.json", claim)
		if err == nil {
			before, inventoryErr := actualFixtureObjects(initializationRoots(spec))
			_, recoveryErr := initializationCommandRouter(spec, dead, "recover-repair")
			after, afterErr := actualFixtureObjects(initializationRoots(spec))
			if inventoryErr == nil && afterErr == nil && recoveryErr != nil && fixtureObjectsEqual(before, after) {
				verified++
			} else {
				failures = append(failures, "initialization")
			}
		}
	}
	if err != nil {
		failures = append(failures, "initialization-prepare")
	}

	s, d, external, refs, err := prepareTransitionFixture(root.Path)
	if err == nil {
		refs.generation = maxGeneration
		owner := releasedWriter(
			refs.instance, "", maxGeneration, maxGeneration-1, strings.Repeat("a", 32),
		)
		if writeRecord(filepath.Join(s, "writer-owner.json"), owner) == nil {
			before, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			readyCalled := false
			identity, identityErr := processIdentityForPID(os.Getpid())
			runErr := executeTransition(s, d, external, refs, identity, func() error {
				readyCalled = true
				return nil
			}, nil)
			after, afterErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
			if inventoryErr == nil && identityErr == nil && afterErr == nil && runErr != nil &&
				!readyCalled && fixtureObjectsEqual(before, after) {
				verified++
			} else {
				failures = append(failures, "fresh-transition")
			}
		} else {
			failures = append(failures, "fresh-transition-fixture")
		}
	} else {
		failures = append(failures, "fresh-transition-prepare")
	}

	recoveryCases := []struct {
		name             string
		ownerGeneration  uint64
		activeGeneration uint64
		transferPending  bool
		residue          bool
	}{
		{"recovery-owner-max-minus-one", maxGeneration - 1, maxGeneration - 1, false, false},
		{"recovery-journal-ahead-max", maxGeneration - 1, maxGeneration, true, false},
		{"recovery-owner-max", maxGeneration, maxGeneration, false, false},
		{"recovery-residue-max-minus-one", maxGeneration - 1, maxGeneration - 1, false, true},
		{"recovery-residue-max", maxGeneration, maxGeneration, false, true},
	}
	for _, recoveryCase := range recoveryCases {
		s, d, external, refs, err = prepareTransitionRecoveryFixture(
			root.Path, "replacing", "journal-prepared-publication",
		)
		if err != nil {
			failures = append(failures, recoveryCase.name+"-prepare")
			continue
		}
		dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
		ownerToken := strings.Repeat("c", 32)
		previousToken := strings.Repeat("b", 32)
		activeToken := ownerToken
		activePredecessor := previousToken
		if recoveryCase.transferPending {
			activeToken = strings.Repeat("d", 32)
			activePredecessor = ownerToken
		}
		refs.generation = recoveryCase.activeGeneration
		refs.writerToken = activeToken
		refs.writerPredecessorToken = activePredecessor
		owner := heldWriter(
			refs.instance, ownerToken, recoveryCase.ownerGeneration, dead,
			recoveryCase.ownerGeneration-1, previousToken,
		)
		journal, ok := transitionJournalForStage("journal-prepared-publication", refs)
		if !ok ||
			writeRecord(filepath.Join(s, "writer-owner.json"), owner) != nil ||
			writeRecord(filepath.Join(s, "journals", refs.journal+".json"), journal) != nil {
			failures = append(failures, recoveryCase.name+"-fixture")
			continue
		}
		var authority *transitionRecoveryAuthority
		if recoveryCase.residue {
			if os.WriteFile(filepath.Join(s, "journals", ".proof-overflow-residue"), nil, 0600) != nil || syncDir(filepath.Join(s, "journals")) != nil {
				failures = append(failures, recoveryCase.name+"-residue")
				continue
			}
			authority = &transitionRecoveryAuthority{
				Boundary:      transitionBoundary{Point: string(prooffs.AfterTemp)},
				Identity:      processIdentity{PID: 999998, Start: 2, PIDNamespace: 1},
				ExpectResidue: true, RecoveryStage: "recovery-journal-transfer",
			}
		}
		before, inventoryErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
		_, recoveryErr := transitionCommandRouterInjected(s, d, external, refs, "recover-resume", authority)
		after, afterErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
		if inventoryErr == nil && afterErr == nil && recoveryErr != nil && fixtureObjectsEqual(before, after) {
			verified++
		} else {
			failures = append(failures, recoveryCase.name)
		}
	}

	status := Observed
	if verified != total {
		status = Failed
	}
	add(Scenario{
		ID:                 "VAL-030/owner/generation-overflow",
		Requirements:       []string{"ARCH-023", "ARCH-024", "ARCH-031", "VAL-030", "VAL-031", "VAL-032"},
		Status:             status,
		Fixture:            "maximum uint64 initialization, fresh-transition, recovered-owner, journal-ahead, and exact interrupted-residue generations",
		Lease:              "I-X and ordered A-X+W-X",
		Writes:             []string{"fixtures/<typed-generation-overflow>/state", "fixtures/<typed-generation-overflow>/data"},
		Observation:        fmt.Sprintf("rejected generation exhaustion before candidate, residue cleanup, journal-transfer, owner-transfer, external effect, or later publication=%d/%d; failures=%v", verified, total, failures),
		RejectedAssumption: "unsigned generation arithmetic remains monotonic when an admitted transition or either recovered ownership step lacks a successor",
		FailedBoundaries:   failures,
	})
}

type pidReuseReport struct {
	NumericPIDReused bool `json:"numeric_pid_reused"`
	StartChanged     bool `json:"start_changed"`
	NamespaceBound   bool `json:"namespace_bound"`
	DurableReopened  bool `json:"durable_reopened"`
	StaleRejected    bool `json:"stale_rejected"`
}

func runPIDReuseProof(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	status := Observed
	stage := "complete"
	base := filepath.Join(root.Path, "pid-reuse")
	reportPath := filepath.Join(base, "report.json")
	if err := os.Mkdir(base, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		status = Failed
		stage = "fixture-setup"
	}
	unshare, err := filepath.EvalSymlinks("/usr/bin/unshare")
	currentUser, userErr := user.Current()
	username := ""
	if userErr == nil {
		username = currentUser.Username
	}
	uidStart, uidCount, uidErr := subordinateRange("/etc/subuid", username)
	gidStart, gidCount, gidErr := subordinateRange("/etc/subgid", username)
	if err != nil || !filepath.IsAbs(unshare) || cfg.SelfExecutable == "" || userErr != nil || uidErr != nil || gidErr != nil || uidCount == 0 || gidCount == 0 {
		status = Failed
		stage = "namespace-prerequisites"
	}
	if status == Observed {
		cmd := exec.CommandContext(ctx, unshare,
			"--map-users", fmt.Sprintf("0:%d:1", os.Getuid()),
			"--map-users", fmt.Sprintf("1:%d:%d", uidStart, uidCount),
			"--map-groups", fmt.Sprintf("0:%d:1", os.Getgid()),
			"--map-groups", fmt.Sprintf("1:%d:%d", gidStart, gidCount),
			"--mount", "--pid", "--fork", "--kill-child", "--mount-proc", "--setuid", "0", "--setgid", "0",
			cfg.SelfExecutable, "--worker", "pid-reuse", "--worker-path", reportPath)
		cmd.Env = []string{"PATH=/usr/bin", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
		if err = cmd.Run(); err != nil {
			status = Failed
			stage = "namespace-worker"
		}
	}
	var report pidReuseReport
	if status == Observed {
		bytes, readErr := os.ReadFile(reportPath)
		decoder := json.NewDecoder(strings.NewReader(string(bytes)))
		decoder.DisallowUnknownFields()
		if readErr != nil || decoder.Decode(&report) != nil || decoder.Decode(&struct{}{}) != io.EOF || !report.NumericPIDReused || !report.StartChanged || !report.NamespaceBound || !report.DurableReopened || !report.StaleRejected {
			status = Failed
			stage = "report-validation"
		}
	}
	if err := os.Remove(reportPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		status = Failed
		stage = "report-cleanup"
	}
	add(Scenario{ID: "VAL-030/owner/pid-reuse", Requirements: []string{"VAL-030", "ARCH-023", "ARCH-031"}, Status: status, Fixture: "isolated user/PID/mount namespace with controlled ns_last_pid", Lease: "A-X+W-X", Writes: []string{"pid-reuse/writer.lock", "pid-reuse/writer-owner.json", "pid-reuse/report.json (transient)"}, Observation: fmt.Sprintf("numeric-pid-reused=%t, start-changed=%t, namespace-bound=%t, durable-tuple-reopened-under-stable-lock=%t, stale-rejected=%t; stage=%s", report.NumericPIDReused, report.StartChanged, report.NamespaceBound, report.DurableReopened, report.StaleRejected, stage), RejectedAssumption: "PID equality or an in-memory pre-death tuple proves owner liveness"})
}

func runTwoReclaimers(root *prooffs.OwnedRoot, add func(Scenario)) {
	dir := filepath.Join(root.Path, "two-reclaimers")
	_ = os.Mkdir(dir, 0700)
	lockPath, ownerPath := filepath.Join(dir, "writer.lock"), filepath.Join(dir, "writer-owner.json")
	_ = os.WriteFile(lockPath, nil, 0600)
	refs := freshGraphRefs()
	dead := processIdentity{PID: 999998, Start: 1, PIDNamespace: 1}
	_ = writeRecord(ownerPath, heldWriter(refs.instance, refs.writerToken, 7, dead, 6, refs.priorWriterToken))
	start, release := make(chan struct{}), make(chan struct{})
	results := make(chan bool, 2)
	for index := range 2 {
		go func(index int) {
			file, err := os.OpenFile(lockPath, os.O_RDWR, 0)
			if err != nil {
				results <- false
				return
			}
			defer file.Close()
			<-start
			if ofd(int(file.Fd()), fWRLCK) != nil {
				results <- false
				return
			}
			var prior writerOwnerRecord
			err = readRecord(ownerPath, &prior)
			if err == nil && prior.Generation == 7 && prior.Token == refs.writerToken && prior.Process == dead {
				token := refs.operationToken
				if index == 1 {
					token = refs.journal
				}
				identity, identityErr := processIdentityForPID(os.Getpid())
				if identityErr != nil {
					err = identityErr
				} else {
					err = publishRecord(dir, "writer-owner.json", heldWriter(refs.instance, token, 8, identity, prior.Generation, prior.Token))
				}
			}
			results <- err == nil
			<-release
			_ = ofd(int(file.Fd()), fUNLCK)
		}(index)
	}
	close(start)
	winners := 0
	for range 2 {
		if <-results {
			winners++
		}
	}
	close(release)
	var after writerOwnerRecord
	err := readRecord(ownerPath, &after)
	status := Observed
	if winners != 1 || err != nil || after.Generation != 8 || after.Token == refs.writerToken || after.PreviousGeneration != 7 || after.PreviousToken != refs.writerToken {
		status = Failed
	}
	add(Scenario{ID: "VAL-030/owner/two-stale-reclaimers", Requirements: []string{"VAL-030", "VAL-032"}, Status: status, Fixture: "two simultaneous compare-transfer reclaimers", Lease: "W-X", Writes: []string{"two-reclaimers/writer-owner.json"}, Observation: fmt.Sprintf("nonblocking stable-inode race produced %d compare-transfer winner; durable predecessor token/generation changed exactly once and publication was reopened", winners)})
}
func lockScenario(id string, ok bool, obs string) Scenario {
	st := Observed
	if !ok {
		st = Failed
	}
	return Scenario{ID: "VAL-030/locks/" + id, Requirements: []string{"VAL-030"}, Status: st, Fixture: id, Lease: id, Writes: []string{"admission-proof.lock"}, Observation: obs}
}
func tarBlob(entries []tar.Header) []byte {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for i := range entries {
		h := entries[i]
		_ = w.WriteHeader(&h)
		if h.Size > 0 {
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, int(h.Size)))
		}
	}
	_ = w.Close()
	return b.Bytes()
}
func archiveDigest(blob []byte) string {
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func rewrittenTarHeader(blob []byte, name string, typeflag byte) []byte {
	out := bytes.Clone(blob)
	for i := range 100 {
		out[i] = 0
	}
	copy(out[:100], name)
	out[156] = typeflag
	for i := 148; i < 156; i++ {
		out[i] = ' '
	}
	checksum := 0
	for _, b := range out[:512] {
		checksum += int(b)
	}
	copy(out[148:156], fmt.Sprintf("%06o\x00 ", checksum))
	return out
}

func archiveStageCount(root *prooffs.OwnedRoot) int {
	entries, err := fs.ReadDir(root.Root.FS(), ".")
	if err != nil {
		return -1
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prooffs.ArchiveStagingPrefix) {
			count++
		}
	}
	return count
}

func archiveTargetAbsent(root *prooffs.OwnedRoot, name string) bool {
	_, err := root.Root.Lstat(name)
	return errors.Is(err, fs.ErrNotExist)
}

func archiveSentinelBytes(root *prooffs.OwnedRoot) ([]byte, error) {
	file, err := root.Root.Open("outside-sentinel/sentinel")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func extractArchiveUnderHostileUmask(root *prooffs.OwnedRoot, name string, blob []byte, provenance prooffs.ArchiveProvenance, lim prooffs.ArchiveLimits, opt prooffs.ArchiveOptions) prooffs.ArchiveObservation {
	old := syscall.Umask(0777)
	defer syscall.Umask(old)
	return prooffs.ExtractAuthenticatedTarAt(int(root.Dir.Fd()), name, blob, provenance, lim, opt)
}

func runArchiveRejections(root *prooffs.OwnedRoot, lim prooffs.ArchiveLimits, add func(Scenario)) (int, bool) {
	allObserved := true
	baseFile := tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeReg}})
	malformed := bytes.Clone(baseFile)
	malformed[0] ^= 0xff
	truncated := bytes.Clone(baseFile[:len(baseFile)-1])
	trailing := append(bytes.Clone(baseFile), make([]byte, 512)...)
	pax := tarBlob([]tar.Header{{
		Name:       "x",
		Mode:       0600,
		Typeflag:   tar.TypeReg,
		Format:     tar.FormatPAX,
		PAXRecords: map[string]string{"VENDOR.proof": "frozen"},
	}})
	cases := []struct {
		name   string
		blob   []byte
		digest string
		want   string
	}{
		{"absolute", tarBlob([]tar.Header{{Name: "/x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "absolute-name"},
		{"traversal", tarBlob([]tar.Header{{Name: "../x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "traversal-name"},
		{"empty", rewrittenTarHeader(baseFile, "", tar.TypeReg), "", "empty-name"},
		{"ambiguous-dot", tarBlob([]tar.Header{{Name: "./x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "ambiguous-or-alias-name"},
		{"ambiguous-empty-component", tarBlob([]tar.Header{{Name: "a//x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "ambiguous-or-alias-name"},
		{"alias-backslash", tarBlob([]tar.Header{{Name: `a\x`, Mode: 0600, Typeflag: tar.TypeReg}}), "", "ambiguous-or-alias-name"},
		{"alias-trailing-slash", tarBlob([]tar.Header{{Name: "x/", Mode: 0700, Typeflag: tar.TypeDir}}), "", "ambiguous-or-alias-name"},
		{"alias-nul-padding", rewrittenTarHeader(baseFile, "x\x00alias", tar.TypeReg), "", "ambiguous-or-alias-name"},
		{"duplicate", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeReg}, {Name: "x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "duplicate"},
		{"duplicate-normalized", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeReg}, {Name: "./x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "duplicate-normalized"},
		{"symlink", tarBlob([]tar.Header{{Name: "x", Linkname: "../outside-sentinel/sentinel", Mode: 0700, Typeflag: tar.TypeSymlink}}), "", "link-device-socket-or-unsupported-type"},
		{"hardlink", tarBlob([]tar.Header{{Name: "x", Linkname: "config", Mode: 0600, Typeflag: tar.TypeLink}}), "", "link-device-socket-or-unsupported-type"},
		{"device", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeChar}}), "", "link-device-socket-or-unsupported-type"},
		{"block-device", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeBlock}}), "", "link-device-socket-or-unsupported-type"},
		{"fifo", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Typeflag: tar.TypeFifo}}), "", "link-device-socket-or-unsupported-type"},
		{"socket", rewrittenTarHeader(baseFile, "x", 's'), "", "link-device-socket-or-unsupported-type"},
		{"unsupported-type", rewrittenTarHeader(baseFile, "x", tar.TypeCont), "", "link-device-socket-or-unsupported-type"},
		{"regular-type-alias", rewrittenTarHeader(baseFile, "x", '\x00'), "", "link-device-socket-or-unsupported-type"},
		{"unsupported-metadata", pax, "", "unsupported-format-or-metadata"},
		{"mode", tarBlob([]tar.Header{{Name: "x", Mode: 0644, Typeflag: tar.TypeReg}}), "", "unsupported-mode"},
		{"executable-mode", tarBlob([]tar.Header{{Name: "x", Mode: 0755, Typeflag: tar.TypeReg}}), "", "unsupported-mode"},
		{"directory-mode", tarBlob([]tar.Header{{Name: "x", Mode: 0755, Typeflag: tar.TypeDir}}), "", "unsupported-mode"},
		{"special-mode", tarBlob([]tar.Header{{Name: "x", Mode: 04600, Typeflag: tar.TypeReg}}), "", "unsupported-mode"},
		{"oversize", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Size: 17, Typeflag: tar.TypeReg}}), "", "entry-size"},
		{"total-size", tarBlob([]tar.Header{{Name: "x", Mode: 0600, Size: 13, Typeflag: tar.TypeReg}, {Name: "y", Mode: 0600, Size: 13, Typeflag: tar.TypeReg}}), "", "total-size"},
		{"count", tarBlob([]tar.Header{{Name: "a", Mode: 0600, Typeflag: tar.TypeReg}, {Name: "b", Mode: 0600, Typeflag: tar.TypeReg}, {Name: "c", Mode: 0600, Typeflag: tar.TypeReg}, {Name: "d", Mode: 0600, Typeflag: tar.TypeReg}}), "", "count"},
		{"depth", tarBlob([]tar.Header{{Name: "a/b/c/d", Mode: 0600, Typeflag: tar.TypeReg}}), "", "depth"},
		{"missing-parent", tarBlob([]tar.Header{{Name: "a/x", Mode: 0600, Typeflag: tar.TypeReg}}), "", "missing-or-nondirectory-parent"},
		{"malformed", malformed, "", "partial-or-corrupt"},
		{"truncated", truncated, "", "malformed-framing"},
		{"trailing", trailing, "", "trailing-input"},
		{"integrity-mismatch", baseFile, strings.Repeat("0", 64), "integrity-before-parse"},
	}
	for _, c := range cases {
		digest := c.digest
		if digest == "" {
			digest = archiveDigest(c.blob)
		}
		before := archiveStageCount(root)
		o := prooffs.ValidateTarIntegrity(c.blob, digest, lim)
		after := archiveStageCount(root)
		ok := o.Rejection == c.want && o.Boundaries == 0 && !o.Extracted && !o.Published && before == after && !o.ProvenanceVerified
		if c.want == "integrity-before-parse" {
			ok = ok && !o.IntegrityVerified && !o.Parsed
		} else {
			ok = ok && o.IntegrityVerified
		}
		status := Observed
		if !ok {
			status = Failed
			allObserved = false
		}
		add(Scenario{
			ID:           "VAL-028/archive/" + c.name,
			Requirements: []string{"VAL-028"},
			Status:       status,
			Fixture:      c.name + " generic integrity-ordered archive structure; no authenticity claim",
			Writes:       []string{},
			Observation: fmt.Sprintf(
				"provenance=%t integrity=%t parsed=%t prevalidated=%t extraction-boundaries=%d staging-before=%d staging-after=%d rejection=%s",
				o.ProvenanceVerified, o.IntegrityVerified, o.Parsed, o.Prevalidated, o.Boundaries, before, after, o.Rejection,
			),
		})
	}

	return len(cases), allObserved
}

func archiveSuccessObserved(success prooffs.ArchiveObservation) bool {
	return success.Rejection == "" &&
		success.IntegrityVerified && success.ProvenanceVerified && success.Parsed && success.Prevalidated &&
		success.Extracted && success.Published && success.ParentSynced &&
		success.Reopened && success.Verified && !success.Cleaned &&
		success.Entries == 3 && success.ExpandedBytes == 8 &&
		success.Creates == 4 && success.Writes == 2 &&
		success.FileSyncs == 2 && success.DirectorySyncs == 2 &&
		success.Renames == 1 && success.ParentSyncs == 1 &&
		success.Reopens == 4 && success.Boundaries == 16
}

func runArchiveTamperChecks(root *prooffs.OwnedRoot, validBlob []byte, validProvenance prooffs.ArchiveProvenance, lim prooffs.ArchiveLimits, add func(Scenario)) bool {
	allObserved := true
	modified := append(bytes.Clone(validBlob), byte('x'))
	modifiedObservation := extractArchiveUnderHostileUmask(root, "archive-modified", modified, validProvenance, lim, prooffs.ArchiveOptions{})
	modifiedOK := modifiedObservation.ProvenanceVerified && !modifiedObservation.IntegrityVerified && !modifiedObservation.Parsed &&
		modifiedObservation.Boundaries == 0 && modifiedObservation.Rejection == "integrity-before-parse" &&
		archiveTargetAbsent(root, "archive-modified")
	modifiedStatus := Observed
	if !modifiedOK {
		modifiedStatus = Failed
		allObserved = false
	}
	add(Scenario{
		ID:           "VAL-028/archive/archive-mutated-independent-of-provenance",
		Requirements: []string{"VAL-028"},
		Status:       modifiedStatus,
		Fixture:      "reviewed provenance held fixed while complete frozen archive bytes changed",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"provenance=%t integrity=%t parsed=%t extraction-boundaries=%d rejection=%s",
			modifiedObservation.ProvenanceVerified, modifiedObservation.IntegrityVerified, modifiedObservation.Parsed, modifiedObservation.Boundaries, modifiedObservation.Rejection,
		),
	})

	modifiedProvenance := validProvenance
	modifiedProvenance.Fixture += "-tampered"
	modifiedProvenance.SHA256 = strings.Repeat("0", sha256.Size*2)
	provenanceObservation := extractArchiveUnderHostileUmask(root, "archive-provenance-modified", validBlob, modifiedProvenance, lim, prooffs.ArchiveOptions{})
	provenanceOK := !provenanceObservation.ProvenanceVerified && !provenanceObservation.IntegrityVerified && !provenanceObservation.Parsed &&
		provenanceObservation.Boundaries == 0 && provenanceObservation.Rejection == "provenance-before-parse" &&
		archiveTargetAbsent(root, "archive-provenance-modified")
	provenanceStatus := Observed
	if !provenanceOK {
		provenanceStatus = Failed
		allObserved = false
	}
	add(Scenario{
		ID:           "VAL-028/archive/provenance-mutated-independent-of-archive",
		Requirements: []string{"VAL-028"},
		Status:       provenanceStatus,
		Fixture:      "reviewed archive bytes held fixed while independently pinned provenance identity changed",
		Writes:       []string{},
		Observation:  fmt.Sprintf("provenance=%t integrity=%t parsed=%t extraction-boundaries=%d rejection=%s", provenanceObservation.ProvenanceVerified, provenanceObservation.IntegrityVerified, provenanceObservation.Parsed, provenanceObservation.Boundaries, provenanceObservation.Rejection),
	})

	return allObserved
}

func runArchives(root *prooffs.OwnedRoot, add func(Scenario)) {
	lim := prooffs.ArchiveLimits{EntryBytes: 16, TotalBytes: 24, Count: 3, Depth: 3}
	validBlob := tarBlob([]tar.Header{
		{Name: "bin", Mode: 0700, Typeflag: tar.TypeDir},
		{Name: "bin/tool", Mode: 0700, Size: 5, Typeflag: tar.TypeReg},
		{Name: "config", Mode: 0600, Size: 3, Typeflag: tar.TypeReg},
	})
	validProvenance := prooffs.StateProofArchiveProvenance()
	sentinelBefore, sentinelBeforeErr := archiveSentinelBytes(root)
	stagesBefore := archiveStageCount(root)
	allObserved := sentinelBeforeErr == nil && stagesBefore == 0

	success := extractArchiveUnderHostileUmask(root, "archive-success", validBlob, validProvenance, lim, prooffs.ArchiveOptions{})
	successOK := archiveSuccessObserved(success)
	successStatus := Observed
	if !successOK {
		successStatus = Failed
		allObserved = false
	}
	add(Scenario{
		ID:           "VAL-028/archive/valid",
		Requirements: []string{"VAL-028"},
		Status:       successStatus,
		Fixture:      "three-entry frozen regular-file and directory tree under 0777 umask",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"independent-provenance=%t integrity=%t parsed=%t prevalidated=%t extracted=%t published=%t parent-synced=%t reopened=%t verified=%t entries=%d bytes=%d tree-identity-checks=%d boundaries=%d creates=%d writes=%d file-syncs=%d directory-syncs=%d renames=%d parent-syncs=%d reopens=%d",
			success.ProvenanceVerified, success.IntegrityVerified, success.Parsed, success.Prevalidated, success.Extracted, success.Published,
			success.ParentSynced, success.Reopened, success.Verified, success.Entries, success.ExpandedBytes,
			success.Entries+1, success.Boundaries, success.Creates, success.Writes, success.FileSyncs,
			success.DirectorySyncs, success.Renames, success.ParentSyncs, success.Reopens,
		),
	})

	rejectionCount, rejectionsObserved := runArchiveRejections(root, lim, add)
	allObserved = allObserved && rejectionsObserved

	tamperObserved := runArchiveTamperChecks(root, validBlob, validProvenance, lim, add)
	allObserved = allObserved && tamperObserved

	injected := 0
	cleaned := 0
	recovered := 0
	prefixOK := successOK
	for target := 1; target <= success.Boundaries; target++ {
		name := fmt.Sprintf("archive-prefix-%02d", target)
		seen := 0
		o := extractArchiveUnderHostileUmask(root, name, validBlob, validProvenance, lim, prooffs.ArchiveOptions{
			Fail: func(prooffs.ArchiveBoundary) error {
				seen++
				if seen == target {
					return errors.New("injected archive boundary failure")
				}
				return nil
			},
		})
		injected++
		ok := o.Rejection == "injected-failure" && o.Boundaries == target && !o.CleanupFailed
		if o.Published {
			recovery := prooffs.VerifyAuthenticatedTarAt(int(root.Dir.Fd()), name, validBlob, validProvenance, lim)
			ok = ok && !o.Cleaned && recovery.ProvenanceVerified && recovery.IntegrityVerified && recovery.Prevalidated &&
				recovery.Reopened && recovery.Verified && recovery.Reopens == success.Reopens
			recovered++
		} else {
			ok = ok && o.Cleaned && !o.CleanupIdentityPreserved && archiveTargetAbsent(root, name)
			cleaned++
		}
		if !ok {
			prefixOK = false
		}
	}
	if archiveStageCount(root) != stagesBefore {
		prefixOK = false
	}
	prefixStatus := Observed
	if !prefixOK {
		prefixStatus = Failed
		allObserved = false
	}
	add(Scenario{
		ID:           "VAL-028/archive/failure-prefixes",
		Requirements: []string{"VAL-028"},
		Status:       prefixStatus,
		Fixture:      "one injected failure after every successful extraction and publication boundary",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"injected-prefixes=%d identity-verified-staging-cleanups=%d reopened-published-recoveries=%d residual-staging=%d; baseline boundaries=%d creates=%d writes=%d file-syncs=%d directory-syncs=%d renames=%d parent-syncs=%d reopens=%d",
			injected, cleaned, recovered, archiveStageCount(root)-stagesBefore, success.Boundaries,
			success.Creates, success.Writes, success.FileSyncs, success.DirectorySyncs,
			success.Renames, success.ParentSyncs, success.Reopens,
		),
	})

	noReplace := extractArchiveUnderHostileUmask(root, "archive-success", validBlob, validProvenance, lim, prooffs.ArchiveOptions{})
	existing := prooffs.VerifyAuthenticatedTarAt(int(root.Dir.Fd()), "archive-success", validBlob, validProvenance, lim)
	noReplaceOK := noReplace.Rejection == "publish-no-replace" && !noReplace.Published &&
		noReplace.Cleaned && !noReplace.CleanupFailed && noReplace.Renames == 0 &&
		existing.Verified && archiveStageCount(root) == stagesBefore
	noReplaceStatus := Observed
	if !noReplaceOK {
		noReplaceStatus = Failed
		allObserved = false
	}
	add(Scenario{
		ID:           "VAL-028/archive/no-replace",
		Requirements: []string{"VAL-028"},
		Status:       noReplaceStatus,
		Fixture:      "same-filesystem publication target already exists",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"published=%t cleaned=%t rename-successes=%d existing-tree-reopened=%t residual-staging=%d rejection=%s",
			noReplace.Published, noReplace.Cleaned, noReplace.Renames, existing.Verified,
			archiveStageCount(root)-stagesBefore, noReplace.Rejection,
		),
	})

	sentinelAfter, sentinelAfterErr := archiveSentinelBytes(root)
	sentinelUnchanged := sentinelBeforeErr == nil && sentinelAfterErr == nil && bytes.Equal(sentinelBefore, sentinelAfter)
	sentinelOK := sentinelUnchanged && archiveStageCount(root) == stagesBefore
	sentinelStatus := Observed
	if !sentinelOK {
		sentinelStatus = Failed
		allObserved = false
	}
	cleanupCount := cleaned
	if noReplace.Cleaned {
		cleanupCount++
	}
	add(Scenario{
		ID:           "VAL-028/archive/sentinel-cleanup",
		Requirements: []string{"VAL-028"},
		Status:       sentinelStatus,
		Fixture:      "outside sentinel adjacent to all fresh no-follow staging directories",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"outside-sentinel-unchanged=%t identity-verified-cleanups=%d residual-staging=%d",
			sentinelUnchanged, cleanupCount, archiveStageCount(root)-stagesBefore,
		),
	})

	parserStatus := Observed
	if !allObserved {
		parserStatus = Failed
	}
	add(Scenario{
		ID:           "VAL-028/parser-consumption",
		Requirements: []string{"VAL-028"},
		Status:       parserStatus,
		Fixture:      "independently pinned reviewed provenance, frozen integrity verification, full prevalidation, durable extraction, prefix recovery, and separate generic parser rejection matrix",
		Writes:       []string{},
		Observation: fmt.Sprintf(
			"entries=%d expanded-bytes=%d rejection-cases=%d injected-prefixes=%d cleaned-prefixes=%d recovered-prefixes=%d success-boundaries=%d sentinel-checks=1",
			success.Entries, success.ExpandedBytes, rejectionCount+1, injected, cleaned, recovered, success.Boundaries,
		),
	})
}
func runStateTransitions(root *prooffs.OwnedRoot, add func(Scenario)) {
	stages := []string{
		"candidate-publication", "journal-prepared-publication",
		"journal-mutating-tool-install-precondition", "journal-mutating-tool-install-result",
		"journal-mutating-home-apply-precondition", "journal-mutating-home-apply-result",
		"journal-pending-receipt-publication", "pending-receipt-publication",
		"current-publication", "journal-cleanup-publication", "owner-release-publication",
	}
	for _, stop := range stages {
		s, d, refs := makeGraph(root.Path, "STABLE(A)")
		err := driveTransition(s, refs, stop)
		got := classify(s, d)
		allowed := map[string]bool{"TRANSITION(A,B)": true, "CLEANUP(B)": true, "STABLE(A)": true}
		if stop == "current-publication" || stop == "journal-cleanup-publication" || stop == "owner-release-publication" {
			allowed = map[string]bool{"CLEANUP(B)": true, "STABLE(A)": true}
		}
		status := Observed
		if err != nil || !allowed[got] {
			status = Failed
		}
		add(Scenario{ID: "VAL-031/transition/" + stop, Requirements: []string{"VAL-031", "VAL-032", "VAL-034"}, Status: status, Fixture: "complete durable A-to-B graph stopped after " + stop, Classification: got, Lease: "A-X+W-X", Writes: []string{filepath.ToSlash(strings.TrimPrefix(s, root.Path+string(os.PathSeparator)))}, Observation: "restart classifier read exact prepared, mutating-tool-install, mutating-home-apply, pending-receipt, stable/current, and cleanup relations; state=" + got})
	}
	s, d, refs := makeGraph(root.Path, "STABLE(A)")
	err := driveTransition(s, refs, "owner-release-publication")
	got := classify(s, d)
	status := Observed
	if err != nil || got != "STABLE(A)" {
		status = Failed
	}
	add(Scenario{ID: "VAL-032/full-forward-recovery", Requirements: []string{"VAL-032", "VAL-034"}, Status: status, Fixture: "full A-to-B durable transition", Classification: got, Lease: "A-X+W-X", Writes: []string{filepath.ToSlash(strings.TrimPrefix(s, root.Path+string(os.PathSeparator)))}, Observation: "candidate; prepared; mutating-tool-install; mutating-home-apply; pending receipt; sole completion-authority current; cleanup; and released owner were persisted and reopened with no completed-receipt stage"})
}

func driveTransition(s string, r graphRefs, stop string) error {
	identity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	if err = publishRecord(s, "writer-owner.json", heldWriter(r.instance, r.writerToken, r.generation, identity, r.generation-1, transitionPreviousWriterToken(r))); err != nil {
		return err
	}
	stage := func(name string, action func() error) (bool, error) {
		if err := action(); err != nil {
			return true, err
		}
		return name == stop, nil
	}
	if done, stageErr := stage("candidate-publication", func() error {
		return publishCandidateRecord(filepath.Join(s, "candidates"), r.b+".json", candidateReference(r.instance, r.b))
	}); done {
		return stageErr
	}
	if done, stageErr := stage("journal-prepared-publication", func() error {
		return publishRecord(filepath.Join(s, "journals"), r.journal+".json", transitionJournalRecord(r, "prepared", "", nil, r.generation, r.writerToken))
	}); done {
		return stageErr
	}
	completed := make([]string, 0, len(mutationPhases))
	for _, phase := range mutationPhases {
		beforeName := "journal-" + phase + "-precondition"
		if done, stageErr := stage(beforeName, func() error {
			return publishRecord(filepath.Join(s, "journals"), r.journal+".json", transitionJournalRecord(r, "mutating", phase, completed, r.generation, r.writerToken))
		}); done {
			return stageErr
		}
		completed = append(completed, phase)
		afterName := "journal-" + phase + "-result"
		if done, stageErr := stage(afterName, func() error {
			return publishRecord(filepath.Join(s, "journals"), r.journal+".json", transitionJournalRecord(r, "mutating", phase, completed, r.generation, r.writerToken))
		}); done {
			return stageErr
		}
	}
	if done, stageErr := stage("journal-pending-receipt-publication", func() error {
		return publishRecord(filepath.Join(s, "journals"), r.journal+".json", transitionJournalRecord(r, string(phasePending), "", mutationPhases, r.generation, r.writerToken))
	}); done {
		return stageErr
	}
	if done, stageErr := stage("pending-receipt-publication", func() error {
		return publishImmutableRecord(filepath.Join(s, "receipts"), r.receiptB+".json", transitionPendingReceipt(r))
	}); done {
		return stageErr
	}
	if done, stageErr := stage("current-publication", func() error {
		return publishRecord(s, "current.json", transitionCurrent(r))
	}); done {
		return stageErr
	}
	if err = publishRecord(filepath.Join(s, "journals"), r.journal+".json", transitionJournalRecord(r, "cleanup", "", mutationPhases, r.generation, r.writerToken)); err != nil {
		return err
	}
	if stop == "journal-cleanup-publication" {
		return nil
	}
	for _, item := range []struct {
		path   string
		remove bool
	}{
		{filepath.Join(s, "candidates", r.a+".json"), r.a != "" && r.a != r.b},
		{filepath.Join(s, "receipts", r.receiptA+".json"), r.receiptA != "" && r.receiptA != r.receiptB},
		{filepath.Join(s, "journals", r.journal+".json"), true},
	} {
		if !item.remove {
			continue
		}
		if err = os.Remove(item.path); err != nil {
			return err
		}
		if err = syncDir(filepath.Dir(item.path)); err != nil {
			return err
		}
	}
	if err = syncDir(s); err != nil {
		return err
	}
	return publishRecord(s, "writer-owner.json", releasedWriter(r.instance, "", r.generation+1, r.generation, r.writerToken))
}
func publishRecord(dir, name string, value any) error {
	_, err := publishRecordIdentity(dir, name, value)
	return err
}

func publishRecordIdentity(dir, name string, value any) (prooffs.PublicationIdentity, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return prooffs.PublicationIdentity{}, err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return prooffs.PublicationIdentity{}, err
	}
	defer directory.Close()
	var identity prooffs.PublicationIdentity
	err = prooffs.AtomicPublishAt(int(directory.Fd()), name, content, prooffs.PublishOptions{
		Capture: func(published prooffs.PublicationIdentity) { identity = published },
	})
	return identity, err
}

func publishImmutableRecord(dir, name string, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	token, err := FreshToken()
	if err != nil {
		return err
	}
	stagingPath := filepath.Join(dir, ".immutable-"+token)
	staging, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = staging.Write(content); err == nil {
		err = staging.Chmod(0600)
	}
	if err == nil {
		err = staging.Sync()
	}
	closeErr := staging.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	finalPath := filepath.Join(dir, name)
	linkErr := os.Link(stagingPath, finalPath)
	removeErr := os.Remove(stagingPath)
	syncErr := syncDir(dir)
	if errors.Is(linkErr, fs.ErrExist) {
		if removeErr != nil {
			return removeErr
		}
		if syncErr != nil {
			return syncErr
		}
		if exactRecordBytes(finalPath, value) {
			return nil
		}
		return errors.New("existing immutable record does not match exact bytes")
	}
	if linkErr != nil {
		return linkErr
	}
	if removeErr != nil {
		return removeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if !exactRecordBytes(finalPath, value) {
		return errors.New("immutable record reopen mismatch")
	}
	return nil
}

func publishCandidateRecord(dir, name string, value candidateReferenceRecord) error {
	return publishImmutableRecord(dir, name, value)
}
func syncDir(p string) error {
	d, e := os.Open(p)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func runDestination(root *prooffs.OwnedRoot, add func(Scenario)) {
	d := filepath.Join(root.Path, "destination")
	target := filepath.Join(d, "managed")
	cases := []struct {
		name string
		mut  func()
	}{{"unchanged", func() {}}, {"modified-after-hash", func() { _ = os.WriteFile(target, []byte("changed"), 0600) }}, {"missing", func() { _ = os.Remove(target) }}, {"wrong-mode", func() { _ = os.Chmod(target, 0644) }}, {"same-bytes-inode-replacement", func() { _ = os.Remove(target); _ = os.WriteFile(target, []byte("expected"), 0600) }}, {"rename-swap", func() {
		swap := target + ".swap"
		_ = os.WriteFile(swap, []byte("expected"), 0600)
		_ = os.Rename(swap, target)
	}}, {"symlink", func() { _ = os.Remove(target); _ = os.Symlink("../outside-sentinel/sentinel", target) }}, {"hardlink", func() {
		_ = os.Remove(target)
		_ = os.Link(filepath.Join(root.Path, "outside-sentinel", "sentinel"), target)
	}}, {"wrong-type", func() { _ = os.Remove(target); _ = os.Mkdir(target, 0700) }}}
	for _, c := range cases {
		_ = os.RemoveAll(target)
		expected := []byte("expected")
		_ = os.WriteFile(target, expected, 0600)
		expectedHash := sha256.Sum256(expected)
		before, _ := os.Lstat(target)
		c.mut()
		now, e := os.Lstat(target)
		admit := e == nil && now.Mode().IsRegular() && now.Mode().Perm() == 0600 && now.Sys().(*syscall.Stat_t).Nlink == 1 && now.Sys().(*syscall.Stat_t).Uid == uint32(os.Getuid()) && before != nil
		if admit {
			b, _ := os.ReadFile(target)
			gotHash := sha256.Sum256(b)
			admit = gotHash == expectedHash && os.SameFile(before, now)
		}
		want := c.name == "unchanged"
		st := Observed
		if admit != want {
			st = Failed
		}
		decision := "refused"
		if admit {
			decision = "mutation admitted"
		}
		add(Scenario{ID: "VAL-029/destination/" + c.name, Requirements: []string{"VAL-029", "VAL-034"}, Status: st, Fixture: c.name, Admission: decision, Lease: "A-X+W-X", Writes: []string{}, Observation: "post-authorization no-follow inode/type/link/mode/owner/full-content-hash revalidation executed before mutation"})
	}
	ancestor := filepath.Join(root.Path, "destination-ancestor")
	_ = os.Mkdir(ancestor, 0700)
	before, _ := os.Lstat(ancestor)
	held, _ := os.Open(ancestor)
	_ = os.Rename(ancestor, ancestor+".held")
	_ = os.Mkdir(ancestor, 0700)
	after, _ := os.Lstat(ancestor)
	heldInfo, _ := held.Stat()
	refused := before != nil && after != nil && heldInfo != nil && os.SameFile(before, heldInfo) && !os.SameFile(heldInfo, after)
	_ = held.Close()
	status := Observed
	if !refused {
		status = Failed
	}
	add(Scenario{ID: "VAL-029/destination/ancestor-rename-replacement", Requirements: []string{"VAL-029", "VAL-034"}, Status: status, Fixture: "authorized destination ancestor renamed and replaced", Admission: "refused", Lease: "A-X+W-X", Writes: []string{}, Observation: "descriptor and pathname ancestor identities were reopened and compared immediately before mutation; replacement was refused despite cooperative lease"})
}
func runErrorPrecedence(add func(Scenario)) {
	primary := errors.New("proof primary mutation failure")
	cleanup := errors.New("proof cleanup failure")
	result := primary
	if result == nil {
		result = cleanup
	}
	status := Observed
	if !errors.Is(result, primary) || errors.Is(result, cleanup) {
		status = Failed
	}
	add(Scenario{ID: "VAL-031/primary-cleanup-error", Requirements: []string{"VAL-031"}, Status: status, Fixture: "simultaneous primary and cleanup failures", Admission: "primary returned", Writes: []string{}, Observation: "both injected error paths executed; deterministic precedence retained the primary mutation failure and did not replace it with cleanup failure"})
}

var deterministicTransitionStages = []string{
	"writer-owner-held-publication",
	"candidate-publication",
	"journal-prepared-publication",
	"journal-mutating-tool-install-publication",
	"external-tool-install",
	"journal-mutating-tool-install-result-publication",
	"journal-mutating-home-apply-publication",
	"external-home-apply",
	"journal-mutating-home-apply-result-publication",
	"journal-pending-receipt-publication",
	"pending-receipt-publication",
	"current-publication",
	"journal-cleanup-publication",
	"cleanup-candidate-remove",
	"cleanup-receipt-remove",
	"cleanup-journal-remove",
	"writer-owner-released-publication",
}

func transitionStages(r graphRefs) []string {
	stages := make([]string, 0, len(deterministicTransitionStages))
	for _, stage := range deterministicTransitionStages {
		if stage == "cleanup-candidate-remove" && (r.a == "" || r.a == r.b) {
			continue
		}
		if stage == "cleanup-receipt-remove" && (r.receiptA == "" || r.receiptA == r.receiptB) {
			continue
		}
		stages = append(stages, stage)
	}
	return stages
}

type transitionBoundary struct {
	Name  string
	Stage string
	Point string
	Kind  string
}

var atomicTransitionPoints = []string{
	string(prooffs.BeforeTemp), string(prooffs.AfterTemp), string(prooffs.AfterWrite),
	string(prooffs.AfterFileSync), string(prooffs.BeforeRename), string(prooffs.AfterRename),
	string(prooffs.AfterParentSync), string(prooffs.AfterReopen),
}

var removalTransitionPoints = []string{"before-unlink", "after-unlink", "after-parent-sync", "after-absence-reopen"}

func transitionBoundaries(r graphRefs) []transitionBoundary {
	var result []transitionBoundary
	for _, stage := range transitionStages(r) {
		for _, checkpoint := range []string{"pre", "post"} {
			for _, side := range []string{"before-revalidation", "after-revalidation"} {
				point := "authority-" + checkpoint + "/" + side
				result = append(result, transitionBoundary{Name: stage + "/" + point, Stage: stage, Point: point, Kind: "authority"})
			}
		}
		kind := "replaceable"
		points := atomicTransitionPoints
		switch stage {
		case "candidate-publication", "pending-receipt-publication":
			kind = "immutable-no-replace"
		case "cleanup-candidate-remove", "cleanup-receipt-remove", "cleanup-journal-remove":
			kind = "durable-removal"
			points = removalTransitionPoints
		}
		for _, point := range points {
			result = append(result, transitionBoundary{Name: stage + "/" + point, Stage: stage, Point: point, Kind: kind})
		}
	}
	return result
}

func transitionBoundaryByName(r graphRefs, name string) (transitionBoundary, bool) {
	for _, boundary := range transitionBoundaries(r) {
		if boundary.Name == name {
			return boundary, true
		}
	}
	return transitionBoundary{}, false
}

func transitionBoundaryLeavesResidue(boundary transitionBoundary) bool {
	if boundary.Kind == "authority" || boundary.Kind == "durable-removal" {
		return false
	}
	switch boundary.Point {
	case string(prooffs.AfterTemp), string(prooffs.AfterWrite),
		string(prooffs.AfterFileSync), string(prooffs.BeforeRename):
		return true
	default:
		return false
	}
}

func previousTransitionStage(stage string, r graphRefs) string {
	previous := "transition-base"
	for _, candidate := range transitionStages(r) {
		if candidate == stage {
			return previous
		}
		previous = candidate
	}
	return previous
}

func durableStageAfterAction(stage string, r graphRefs) string {
	if stage == "candidate-publication" && r.a != "" && r.a == r.b {
		return "writer-owner-held-publication"
	}
	if stage == "pending-receipt-publication" && r.receiptA != "" && r.receiptA == r.receiptB {
		return "journal-pending-receipt-publication"
	}
	return stage
}

func durableStageForBoundary(boundary transitionBoundary, r graphRefs) string {
	previous := durableStageAfterAction(previousTransitionStage(boundary.Stage, r), r)
	after := durableStageAfterAction(boundary.Stage, r)
	if boundary.Kind == "authority" {
		if strings.HasPrefix(boundary.Point, "authority-pre/") {
			return previous
		}
		return after
	}
	if boundary.Kind == "durable-removal" {
		if boundary.Point == "before-unlink" {
			return previous
		}
		return after
	}
	if boundary.Point == string(prooffs.AfterRename) || boundary.Point == string(prooffs.AfterParentSync) ||
		boundary.Point == string(prooffs.AfterReopen) {
		return after
	}
	return previous
}

type deterministicWorkerSpec struct {
	PriorCurrent     currentRecord    `json:"prior_current"`
	CommittedCurrent currentRecord    `json:"committed_current"`
	State            string           `json:"state"`
	Data             string           `json:"data"`
	Cache            string           `json:"cache"`
	External         string           `json:"external"`
	Boundary         string           `json:"boundary"`
	RecoveryBoundary string           `json:"recovery_boundary"`
	Instance         string           `json:"instance"`
	Token            string           `json:"initialization_token"`
	InputRoot        string           `json:"input_root,omitempty"`
	UnavailableInput string           `json:"unavailable_input,omitempty"`
	ResidueIdentity  *processIdentity `json:"residue_identity,omitempty"`
	OperationToken   string           `json:"operation_token"`
	PriorWriterToken string           `json:"prior_writer_token"`
	WriterToken      string           `json:"writer_token"`
	A                string           `json:"candidate_a"`
	B                string           `json:"candidate_b"`
	ReceiptAPrevious string           `json:"receipt_a_previous"`
	ReceiptA         string           `json:"receipt_a"`
	ReceiptB         string           `json:"receipt_b"`
	Journal          string           `json:"journal"`
	Generation       uint64           `json:"writer_generation"`
}

func (s deterministicWorkerSpec) refs() graphRefs {
	return graphRefs{priorCurrent: s.PriorCurrent, committedCurrent: s.CommittedCurrent, generation: s.Generation, instance: s.Instance, token: s.Token, operationToken: s.OperationToken, priorWriterToken: s.PriorWriterToken, writerToken: s.WriterToken, a: s.A, b: s.B, receiptAPrevious: s.ReceiptAPrevious, receiptA: s.ReceiptA, receiptB: s.ReceiptB, journal: s.Journal}
}

func workerSpecFor(s, d, cache, external, boundary string, r graphRefs) deterministicWorkerSpec {
	return deterministicWorkerSpec{
		PriorCurrent: r.priorCurrent, CommittedCurrent: r.committedCurrent, State: s, Data: d, Cache: cache, External: external, Boundary: boundary,
		Generation: r.generation, Instance: r.instance, Token: r.token, OperationToken: r.operationToken,
		PriorWriterToken: r.priorWriterToken, WriterToken: r.writerToken, A: r.a, B: r.b,
		ReceiptAPrevious: r.receiptAPrevious, ReceiptA: r.receiptA, ReceiptB: r.receiptB, Journal: r.journal,
	}
}

func writeDeterministicWorkerSpec(path string, spec deterministicWorkerSpec) error {
	b, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func readDeterministicWorkerSpec(path string) (deterministicWorkerSpec, error) {
	var spec deterministicWorkerSpec
	b, err := os.ReadFile(path)
	if err != nil {
		return spec, err
	}
	if len(b) > 16384 {
		return spec, errors.New("worker specification too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&spec); err != nil {
		return spec, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return spec, errors.New("trailing worker specification")
	}
	return spec, nil
}

func recordBytes(value any) []byte {
	content, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return content
}

func recordDigest(value any) string {
	sum := sha256.Sum256(recordBytes(value))
	return hex.EncodeToString(sum[:])
}

func transitionCandidate(r graphRefs) candidateReferenceRecord {
	return candidateReference(r.instance, r.b)
}

func transitionJournal(r graphRefs, state, phase string, completed []string, generation uint64, token string) journalRecord {
	return transitionJournalRecord(r, state, phase, completed, generation, token)
}

func transitionPendingReceipt(r graphRefs) pendingReceiptRecord {
	return pendingReceiptFor(r.b, receiptDigest(r.a))
}

func transitionCurrent(r graphRefs) currentRecord {
	if r.committedCurrent.Schema != 0 {
		return r.committedCurrent
	}
	return currentRecord{Schema: 1, Domain: "zerb.current", Status: "stable", Instance: r.instance,
		OperationID: r.journal, OperationToken: r.operationToken, WriterGeneration: r.generation, WriterToken: r.writerToken,
		Candidate: receiptDigest(r.b), Receipt: receiptDigest(r.receiptB)}
}

type gatedChild struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	identity processIdentity
	stderr   bytes.Buffer
	released bool
}

func startGatedChild(ctx context.Context, executable, mode, specPath string, root *prooffs.OwnedRoot) (*gatedChild, error) {
	cmd := exec.CommandContext(ctx, executable, "--worker", mode, "--worker-path", specPath)
	cmd.Env = []string{"PATH=/nonexistent", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	child := &gatedChild{cmd: cmd, stdin: stdin}
	cmd.Stderr = &child.stderr
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "READY\n" {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, errors.New("worker did not reach gated lease boundary")
	}
	child.identity, err = processIdentityForPID(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	return child, nil
}

func (c *gatedChild) releaseAndWaitForFailure() bool {
	if !c.released {
		_, _ = io.WriteString(c.stdin, "continue\n")
		_ = c.stdin.Close()
		c.released = true
	}
	waitErr := c.cmd.Wait()
	live, liveErr := processIdentityLive(c.identity)
	return waitErr != nil && liveErr == nil && !live
}

func (c *gatedChild) releaseAndWaitForKill() bool {
	if !c.released {
		_, _ = io.WriteString(c.stdin, "continue\n")
		_ = c.stdin.Close()
		c.released = true
	}
	err := c.cmd.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		return false
	}
	live, liveErr := processIdentityLive(c.identity)
	return liveErr == nil && !live
}

type protectedInventoryEntry struct {
	Ref           string
	Kind          string
	Mode          fs.FileMode
	Owner         uint32
	Device, Inode uint64
	Size          int64
	Digest        [sha256.Size]byte
}

func protectedInventory(roots map[string]string, refs map[string]string) (map[string]protectedInventoryEntry, int, error) {
	result := make(map[string]protectedInventoryEntry)
	fileCount := 0
	labels := make([]string, 0, len(roots))
	for label := range roots {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		root := roots[label]
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := label + ":" + filepath.ToSlash(rel)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return errors.New("inventory owner identity unavailable")
			}
			var kind string
			switch {
			case info.IsDir():
				kind = "directory"
			case info.Mode().IsRegular():
				kind = "file"
			default:
				return errors.New("inventory encountered unsupported object")
			}
			ref := refs[key]
			if ref == "" {
				ref, err = retainedRef("protected-" + kind)
				if err != nil {
					return err
				}
				refs[key] = ref
			}
			fact := protectedInventoryEntry{Ref: ref, Kind: kind, Mode: info.Mode(), Owner: st.Uid, Device: uint64(st.Dev), Inode: st.Ino, Size: info.Size()}
			if kind == "file" {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				fact.Digest = sha256.Sum256(content)
				fileCount++
			}
			result[key] = fact
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	return result, fileCount, nil
}

func inventoriesEqual(a, b map[string]protectedInventoryEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for key, before := range a {
		after, ok := b[key]
		if !ok || before != after {
			return false
		}
	}
	return true
}

func createDurableEmpty(dir, name string) error {
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return syncDir(dir)
}

func createDurableDirectory(parent, name string) error {
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0700); err != nil {
		return err
	}
	if err := syncDir(parent); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !safeDirectoryInfo(info, os.Geteuid()) {
		return errors.New("durable directory reopen mismatch")
	}
	return nil
}
func exactBytesFile(path string, want []byte, expected *prooffs.PublicationIdentity) (prooffs.PublicationIdentity, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 {
		return prooffs.PublicationIdentity{}, errors.New("record topology mismatch")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return prooffs.PublicationIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), "exact-record")
	if file == nil {
		_ = syscall.Close(fd)
		return prooffs.PublicationIdentity{}, errors.New("record descriptor unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	after, afterErr := os.Lstat(path)
	if err != nil || afterErr != nil || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		return prooffs.PublicationIdentity{}, errors.New("record path identity mismatch")
	}
	stat, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return prooffs.PublicationIdentity{}, errors.New("record owner or link mismatch")
	}
	identity := prooffs.PublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino}
	if expected != nil && identity != *expected {
		return prooffs.PublicationIdentity{}, errors.New("record publication identity mismatch")
	}
	got, err := io.ReadAll(io.LimitReader(file, maxRecognitionFile+1))
	if err != nil || !bytes.Equal(got, want) {
		return prooffs.PublicationIdentity{}, errors.New("record bytes mismatch")
	}
	return identity, nil
}

func exactRecordFile(path string, want any, expected *prooffs.PublicationIdentity) (prooffs.PublicationIdentity, error) {
	return exactBytesFile(path, recordBytes(want), expected)
}

func exactRecordBytes(path string, want any) bool {
	_, err := exactRecordFile(path, want, nil)
	return err == nil
}

type initializationPrefixAuthority struct {
	claimIdentity           prooffs.PublicationIdentity
	residuePath             string
	residueIdentity         prooffs.PublicationIdentity
	residueBytes            []byte
	takeoverResiduePath     string
	takeoverResidueIdentity prooffs.PublicationIdentity
	takeoverResidueClaim    rootOwnerRecord
	objects                 map[string]fixtureObject
}

func initializationRoots(spec deterministicWorkerSpec) map[string]string {
	roots := map[string]string{"state": spec.State}
	if exists(spec.Data) {
		roots["data"] = spec.Data
	}
	if exists(spec.Cache) {
		roots["cache"] = spec.Cache
	}
	return roots
}

func inspectInitializationPrefixAuthority(spec deterministicWorkerSpec, claim rootOwnerRecord) (initializationPrefixAuthority, error) {
	if claim.Schema != 1 || claim.Domain != "zerb.root" || claim.Status != "initializing" ||
		claim.Instance != spec.Instance || claim.Token == "" || !validProcessIdentity(claim.Process) {
		return initializationPrefixAuthority{}, errors.New("initialization claim mismatch")
	}
	claimIdentity, err := exactRecordFile(filepath.Join(spec.State, "root.json"), claim, nil)
	if err != nil {
		return initializationPrefixAuthority{}, err
	}
	entries, err := os.ReadDir(spec.State)
	if err != nil {
		return initializationPrefixAuthority{}, err
	}
	var temporary os.DirEntry
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".proof-") {
			continue
		}
		if temporary != nil {
			return initializationPrefixAuthority{}, errors.New("multiple initialization temporaries")
		}
		temporary = entry
	}
	allowedEntry := ""
	if temporary != nil {
		allowedEntry = temporary.Name()
	}
	if !exactInitAllowing(spec.State, spec.Data, os.Geteuid(), allowedEntry) {
		return initializationPrefixAuthority{}, errors.New("initialization prefix topology mismatch")
	}
	if exists(spec.Cache) {
		if !exists(spec.Data) || !safeRootDir(spec.Cache, os.Geteuid()) ||
			(!emptyDir(spec.Cache) &&
				(!exactLeafNames(spec.Cache, "owner.json") ||
					!exactRecordBytes(filepath.Join(spec.Cache, "owner.json"), namespaceOwner(claim.Instance, "cache")))) {
			return initializationPrefixAuthority{}, errors.New("initialization cache prefix mismatch")
		}
	}
	authority := initializationPrefixAuthority{claimIdentity: claimIdentity}
	if temporary != nil {
		path := filepath.Join(spec.State, temporary.Name())
		content, readErr := readNoFollowBounded(path, maxRecognitionFile)
		if readErr != nil {
			return initializationPrefixAuthority{}, readErr
		}
		identity, identityErr := exactBytesFile(path, content, nil)
		if identityErr != nil {
			return initializationPrefixAuthority{}, identityErr
		}
		var takeover rootOwnerRecord
		if readRecord(path, &takeover) == nil &&
			takeover.Schema == 1 && takeover.Domain == "zerb.root" && takeover.Status == "initializing" &&
			takeover.Instance == claim.Instance && takeover.Token != "" && takeover.Token != claim.Token &&
			isGenerationSuccessor(claim.Generation, takeover.Generation) &&
			validProcessIdentity(takeover.Process) {
			authority.takeoverResiduePath = path
			authority.takeoverResidueIdentity = identity
			authority.takeoverResidueClaim = takeover
		} else {
			owner := releasedWriter(spec.Instance, "", 0, 0, "")
			writerPublicationPrefix := []string{
				"init.lock",
				"root.json",
				"admission.lock",
				"writer.lock",
				temporary.Name(),
			}
			sort.Strings(writerPublicationPrefix)
			if exists(spec.Data) || exists(spec.Cache) ||
				!exactNames(spec.State, writerPublicationPrefix) ||
				(len(content) != 0 && !bytes.Equal(content, recordBytes(owner))) {
				return initializationPrefixAuthority{}, errors.New("initialization temporary prefix mismatch")
			}
			authority.residuePath = path
			authority.residueIdentity = identity
			authority.residueBytes = content
		}
	}
	authority.objects, err = actualFixtureObjects(initializationRoots(spec))
	if err != nil {
		return initializationPrefixAuthority{}, err
	}
	return authority, nil
}

func inspectInitializationPrefix(spec deterministicWorkerSpec, childIdentity processIdentity) bool {
	claim := initializingRoot(spec.Instance, spec.Token, 0, childIdentity)
	_, err := inspectInitializationPrefixAuthority(spec, claim)
	return err == nil
}

func revalidateInitializationTakeover(
	spec deterministicWorkerSpec,
	lease *proofLease,
	claim rootOwnerRecord,
	claimIdentity prooffs.PublicationIdentity,
	objects map[string]fixtureObject,
) error {
	if lease == nil || len(lease.files) != 1 || len(lease.names) != 1 || lease.names[0] != "init.lock" {
		return errors.New("initialization takeover lease mismatch")
	}
	opened, statErr := lease.files[0].Stat()
	pathInfo, pathErr := os.Lstat(filepath.Join(spec.State, "init.lock"))
	if statErr != nil || pathErr != nil || !os.SameFile(opened, pathInfo) ||
		!safeLockUID(filepath.Join(spec.State, "init.lock"), os.Geteuid()) {
		return errors.New("initialization takeover lock substitution")
	}
	if _, err := exactRecordFile(filepath.Join(spec.State, "root.json"), claim, &claimIdentity); err != nil {
		return errors.New("initialization takeover claim substitution")
	}
	actual, err := actualFixtureObjects(initializationRoots(spec))
	if err != nil || !fixtureObjectsEqual(actual, objects) {
		return errors.New("initialization takeover prefix substitution")
	}
	return nil
}

func validateInitializationTakeoverTemp(
	file *os.File,
	identity prooffs.PublicationIdentity,
	want []byte,
	links uint64,
) error {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != int64(len(want)) {
		return errors.New("initialization takeover temporary topology mismatch")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != links ||
		identity != (prooffs.PublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino}) {
		return errors.New("initialization takeover temporary identity mismatch")
	}
	if len(want) == 0 {
		return nil
	}
	got := make([]byte, len(want))
	if _, err = file.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
		return errors.New("initialization takeover temporary bytes mismatch")
	}
	return nil
}

func publishInitializationTakeover(
	spec deterministicWorkerSpec,
	lease *proofLease,
	prior rootOwnerRecord,
	priorIdentity prooffs.PublicationIdentity,
	priorObjects map[string]fixtureObject,
	next rootOwnerRecord,
	hook func(string) error,
) (prooffs.PublicationIdentity, map[string]fixtureObject, error) {
	validatePrior := func() error {
		return revalidateInitializationTakeover(spec, lease, prior, priorIdentity, priorObjects)
	}
	run := func(boundary prooffs.Boundary, validate func() error) error {
		if err := validate(); err != nil {
			return err
		}
		if hook != nil {
			if err := hook("takeover-publication/" + string(boundary)); err != nil {
				return err
			}
		}
		return validate()
	}
	if err := run(prooffs.BeforeTemp, validatePrior); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	directory, err := os.Open(spec.State)
	if err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	defer directory.Close()
	fd, err := unix.Openat(int(directory.Fd()), ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC, 0600)
	if err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	file := os.NewFile(uintptr(fd), "initialization-takeover")
	if file == nil {
		_ = unix.Close(fd)
		return prooffs.PublicationIdentity{}, nil, errors.New("initialization takeover temporary unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return prooffs.PublicationIdentity{}, nil, errors.New("initialization takeover temporary identity unavailable")
	}
	identity := prooffs.PublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino}
	validateUnlinked := func(want []byte) func() error {
		return func() error {
			if err := validatePrior(); err != nil {
				return err
			}
			return validateInitializationTakeoverTemp(file, identity, want, 0)
		}
	}
	if err = run(prooffs.AfterTemp, validateUnlinked(nil)); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	content := recordBytes(next)
	for offset := 0; offset < len(content); {
		written, writeErr := file.Write(content[offset:])
		if writeErr != nil {
			return prooffs.PublicationIdentity{}, nil, writeErr
		}
		offset += written
	}
	if err = run(prooffs.AfterWrite, validateUnlinked(content)); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if err = file.Sync(); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if err = run(prooffs.AfterFileSync, validateUnlinked(content)); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	token, err := FreshToken()
	if err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	tempName := ".proof-takeover-" + token
	linked := false
	renamed := false
	defer func() {
		if !linked || renamed {
			return
		}
		cleanupFD, cleanupErr := unix.Openat(
			int(directory.Fd()),
			tempName,
			unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0,
		)
		if cleanupErr != nil {
			return
		}
		cleanup := os.NewFile(uintptr(cleanupFD), tempName)
		if cleanup == nil {
			_ = unix.Close(cleanupFD)
			return
		}
		cleanupErr = validateInitializationTakeoverTemp(cleanup, identity, content, 1)
		_ = cleanup.Close()
		if cleanupErr == nil && unix.Unlinkat(int(directory.Fd()), tempName, 0) == nil {
			_ = directory.Sync()
		}
	}()
	if err = unix.Linkat(int(file.Fd()), "", int(directory.Fd()), tempName, unix.AT_EMPTY_PATH); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	linked = true
	linkedObjects, err := actualFixtureObjects(initializationRoots(spec))
	if err != nil || !fixtureObjectsEqualExceptKeys(
		priorObjects,
		linkedObjects,
		map[string]bool{"state:" + tempName: true},
	) {
		return prooffs.PublicationIdentity{}, nil, errors.New("initialization takeover link changed unrelated object")
	}
	validateLinked := func() error {
		if err := revalidateInitializationTakeover(spec, lease, prior, priorIdentity, linkedObjects); err != nil {
			return err
		}
		return validateInitializationTakeoverTemp(file, identity, content, 1)
	}
	if err = run(prooffs.BeforeRename, validateLinked); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if err = unix.Renameat(int(directory.Fd()), tempName, int(directory.Fd()), "root.json"); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	renamed = true
	after, err := actualFixtureObjects(initializationRoots(spec))
	if err != nil || !fixtureObjectsEqualExceptKeys(
		priorObjects,
		after,
		map[string]bool{"state:root.json": true},
	) {
		return prooffs.PublicationIdentity{}, nil, errors.New("initialization takeover rename changed unrelated object")
	}
	validateNext := func() error {
		return revalidateInitializationTakeover(spec, lease, next, identity, after)
	}
	if err = run(prooffs.AfterRename, validateNext); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if err = directory.Sync(); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if err = run(prooffs.AfterParentSync, validateNext); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	if _, err = exactRecordFile(filepath.Join(spec.State, "root.json"), next, &identity); err != nil {
		return prooffs.PublicationIdentity{}, nil, errors.New("initialization claim transfer reopen mismatch")
	}
	if err = run(prooffs.AfterReopen, validateNext); err != nil {
		return prooffs.PublicationIdentity{}, nil, err
	}
	return identity, after, nil
}

func transferInitializationClaim(
	spec deterministicWorkerSpec,
	durableClaim rootOwnerRecord,
	lease *proofLease,
	authority initializationPrefixAuthority,
	hooks ...func(string) error,
) error {
	if len(hooks) > 1 {
		return errors.New("multiple initialization takeover hooks")
	}
	var hook func(string) error
	if len(hooks) == 1 {
		hook = hooks[0]
	}
	hit := func(point string) error {
		if hook == nil {
			return nil
		}
		return hook("takeover-" + point)
	}
	if durableClaim.Schema != 1 || durableClaim.Domain != "zerb.root" || durableClaim.Status != "initializing" ||
		durableClaim.Instance != spec.Instance || durableClaim.Token == "" || !validProcessIdentity(durableClaim.Process) {
		return errors.New("initialization claim compare failed")
	}
	nextGeneration, ok := generationSuccessor(durableClaim.Generation)
	if !ok {
		return errors.New("initialization generation exhausted")
	}
	if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
		return err
	}
	if err := hit("before-liveness"); err != nil {
		return err
	}
	if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
		return err
	}
	live, liveErr := processIdentityLive(durableClaim.Process)
	if liveErr != nil {
		return fmt.Errorf("initialization claimant liveness unavailable: %w", liveErr)
	}
	if live {
		return errors.New("initialization claimant remains live")
	}
	if err := hit("after-liveness"); err != nil {
		return err
	}
	if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
		return err
	}
	cleanup := func(
		label, path string,
		content []byte,
		identity prooffs.PublicationIdentity,
		claimant processIdentity,
	) error {
		if path == "" {
			return nil
		}
		if validProcessIdentity(claimant) {
			residueLive, err := processIdentityLive(claimant)
			if err != nil {
				return fmt.Errorf("initialization residue claimant liveness unavailable: %w", err)
			}
			if residueLive {
				return errors.New("initialization residue claimant remains live")
			}
		}
		if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
			return err
		}
		if _, err := exactBytesFile(path, content, &identity); err != nil {
			return errors.New("initialization residue identity mismatch")
		}
		if err := hit(label + "-before-unlink"); err != nil {
			return err
		}
		if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
			return err
		}
		if _, err := exactBytesFile(path, content, &identity); err != nil {
			return errors.New("initialization residue identity mismatch")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := hit(label + "-after-unlink"); err != nil {
			return err
		}
		if err := syncDir(spec.State); err != nil {
			return err
		}
		expected := make(map[string]fixtureObject, len(authority.objects)-1)
		for key, object := range authority.objects {
			if key != "state:"+filepath.Base(path) {
				expected[key] = object
			}
		}
		if err := revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, expected); err != nil {
			return err
		}
		authority.objects = expected
		return nil
	}
	if authority.takeoverResiduePath != "" {
		if err := cleanup(
			"stale-residue",
			authority.takeoverResiduePath,
			recordBytes(authority.takeoverResidueClaim),
			authority.takeoverResidueIdentity,
			authority.takeoverResidueClaim.Process,
		); err != nil {
			return err
		}
	}
	if err := cleanup(
		"residue",
		authority.residuePath,
		authority.residueBytes,
		authority.residueIdentity,
		processIdentity{},
	); err != nil {
		return err
	}
	currentIdentity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	recoveryToken, err := FreshToken()
	if err != nil {
		return err
	}
	transferred := initializingRoot(spec.Instance, recoveryToken, nextGeneration, currentIdentity)
	if err = hit("before-publication"); err != nil {
		return err
	}
	if err = revalidateInitializationTakeover(spec, lease, durableClaim, authority.claimIdentity, authority.objects); err != nil {
		return err
	}
	transferredIdentity, afterTransfer, err := publishInitializationTakeover(
		spec,
		lease,
		durableClaim,
		authority.claimIdentity,
		authority.objects,
		transferred,
		hook,
	)
	if err != nil {
		return err
	}
	if err = hit("after-publication"); err != nil {
		return err
	}
	return revalidateInitializationTakeover(spec, lease, transferred, transferredIdentity, afterTransfer)
}

func completeInitializationUnderLease(
	spec deterministicWorkerSpec,
	claim rootOwnerRecord,
	lease *proofLease,
	hooks ...func(string) error,
) error {
	return completeInitializationPrefixUnderLease(spec, claim, lease, true, hooks...)
}

func initializationVisibleRoots(spec deterministicWorkerSpec) map[string]string {
	roots := map[string]string{"state": spec.State}
	if exists(spec.Data) {
		roots["data"] = spec.Data
	}
	if exists(spec.Cache) {
		roots["cache"] = spec.Cache
	}
	return roots
}

func validateInitializationCompletionAuthority(spec deterministicWorkerSpec, claim rootOwnerRecord, lease *proofLease, requireClaim bool, want map[string]fixtureObject) error {
	if len(lease.files) != 1 || len(lease.names) != 1 || lease.names[0] != "init.lock" {
		return errors.New("initialization lease set mismatch")
	}
	opened, statErr := lease.files[0].Stat()
	pathInfo, pathErr := os.Lstat(filepath.Join(spec.State, "init.lock"))
	if statErr != nil || pathErr != nil || !os.SameFile(opened, pathInfo) ||
		!safeLockUID(filepath.Join(spec.State, "init.lock"), os.Geteuid()) {
		return errors.New("initialization lock authority substitution")
	}
	if requireClaim && !exactRecordBytes(filepath.Join(spec.State, "root.json"), claim) {
		return errors.New("initialization claim authority substitution")
	}
	got, inventoryErr := actualFixtureObjects(initializationVisibleRoots(spec))
	if inventoryErr != nil || !fixtureObjectsEqual(want, got) {
		return errors.New("initialization protected-prefix substitution")
	}
	return nil
}

func ensureInitializationDirectory(parent, name string, owner any) error {
	path := filepath.Join(parent, name)
	if exists(path) {
		if !safeRootDir(path, os.Geteuid()) {
			return errors.New("initialization directory reopen mismatch")
		}
		if emptyDir(path) || (owner != nil && exactLeafNames(path, "owner.json") &&
			exactRecordBytes(filepath.Join(path, "owner.json"), owner)) {
			return nil
		}
		return errors.New("initialization directory contents mismatch")
	}
	if err := createDurableDirectory(parent, name); err != nil {
		return err
	}
	if !safeRootDir(path, os.Geteuid()) || !emptyDir(path) {
		return errors.New("initialization directory publication mismatch")
	}
	return nil
}

func completeInitializationPrefixUnderLease(
	spec deterministicWorkerSpec,
	claim rootOwnerRecord,
	lease *proofLease,
	publishFinal bool,
	hooks ...func(string) error,
) error {
	if len(hooks) > 1 {
		return errors.New("multiple initialization hooks")
	}
	var hook func(string) error
	if len(hooks) == 1 {
		hook = hooks[0]
	}
	expected, err := actualFixtureObjects(initializationVisibleRoots(spec))
	if err != nil {
		return err
	}
	validateAuthority := func(requireClaim bool, want map[string]fixtureObject) error {
		return validateInitializationCompletionAuthority(spec, claim, lease, requireClaim, want)
	}
	run := func(stage string, final bool, allowed map[string]bool, action func() error) error {
		if err := validateAuthority(true, expected); err != nil {
			return err
		}
		if hook != nil {
			if err := hook(stage + "/before-action"); err != nil {
				return err
			}
			if err := validateAuthority(true, expected); err != nil {
				return err
			}
		}
		if err := action(); err != nil {
			return err
		}
		after, inventoryErr := actualFixtureObjects(initializationVisibleRoots(spec))
		if inventoryErr != nil || !fixtureObjectsEqualExceptKeys(expected, after, allowed) {
			return errors.New("initialization action changed unauthorized object")
		}
		expected = after
		if final {
			if !exactRecordBytes(filepath.Join(spec.State, "root.json"), initializedRoot(spec.Instance)) {
				return errors.New("initialized root authority substitution")
			}
		} else if !exactRecordBytes(filepath.Join(spec.State, "root.json"), claim) {
			return errors.New("initialization claim authority substitution")
		}
		if hook != nil {
			if err := hook(stage); err != nil {
				return err
			}
		}
		return validateAuthority(!final, expected)
	}
	ensureEmpty := func(dir, name string) error {
		path := filepath.Join(dir, name)
		if exists(path) {
			if !safeLockUID(path, os.Geteuid()) {
				return errors.New("initialization lock reopen mismatch")
			}
			return nil
		}
		if err := createDurableEmpty(dir, name); err != nil {
			return err
		}
		if !safeLockUID(path, os.Geteuid()) {
			return errors.New("initialization lock publication mismatch")
		}
		return nil
	}
	for _, lock := range []struct {
		stage string
		name  string
	}{
		{stage: "admission-lock", name: "admission.lock"},
		{stage: "writer-lock", name: "writer.lock"},
	} {
		lock := lock
		allowed := map[string]bool{"state:.": true, "state:" + lock.name: true}
		if err := run(lock.stage, false, allowed, func() error { return ensureEmpty(spec.State, lock.name) }); err != nil {
			return err
		}
	}
	writer := releasedWriter(spec.Instance, "", 0, 0, "")
	if err := run("writer-owner", false, map[string]bool{"state:.": true, "state:writer-owner.json": true}, func() error {
		if err := publishRecord(spec.State, "writer-owner.json", writer); err != nil {
			return err
		}
		if !exactRecordBytes(filepath.Join(spec.State, "writer-owner.json"), writer) {
			return errors.New("writer-owner publication reopen mismatch")
		}
		return nil
	}); err != nil {
		return err
	}
	for _, name := range []string{"candidates", "journals", "receipts"} {
		name := name
		allowed := map[string]bool{"state:.": true, "state:" + name: true}
		if err := run(name+"-directory", false, allowed, func() error {
			return ensureInitializationDirectory(spec.State, name, nil)
		}); err != nil {
			return err
		}
	}
	pair := filepath.Dir(spec.State)
	for _, namespace := range []struct {
		stage, path, role, label string
	}{
		{"data-root", spec.Data, "data", "data"},
		{"cache-root", spec.Cache, "cache", "cache"},
	} {
		namespace := namespace
		owner := namespaceOwner(spec.Instance, namespace.role)
		allowed := map[string]bool{namespace.label + ":.": true}
		if err := run(namespace.stage, false, allowed, func() error {
			return ensureInitializationDirectory(pair, filepath.Base(namespace.path), owner)
		}); err != nil {
			return err
		}
	}
	for _, publication := range []struct {
		stage, dir, role, label string
	}{
		{"data-owner", spec.Data, "data", "data"},
		{"cache-owner", spec.Cache, "cache", "cache"},
	} {
		publication := publication
		allowed := map[string]bool{publication.label + ":.": true, publication.label + ":owner.json": true}
		if err := run(publication.stage, false, allowed, func() error {
			owner := namespaceOwner(spec.Instance, publication.role)
			if err := publishRecord(publication.dir, "owner.json", owner); err != nil {
				return err
			}
			if !exactRecordBytes(filepath.Join(publication.dir, "owner.json"), owner) {
				return errors.New("namespace owner publication reopen mismatch")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if !publishFinal {
		return validateAuthority(true, expected)
	}
	return run("initialized-root", true, map[string]bool{"state:.": true, "state:root.json": true}, func() error {
		initialized := initializedRoot(spec.Instance)
		if err := publishRecord(spec.State, "root.json", initialized); err != nil {
			return err
		}
		if !exactRecordBytes(filepath.Join(spec.State, "root.json"), initialized) {
			return errors.New("initialized root publication reopen mismatch")
		}
		return nil
	})
}

func fullInitializationValid(spec deterministicWorkerSpec) bool {
	return fullReadyGraphValid(spec.State, spec.Data, spec.Cache, spec.Instance) &&
		exactLeafNames(spec.Cache, "owner.json")
}

func exactInitializationRepairPrefix(spec deterministicWorkerSpec, claim rootOwnerRecord) bool {
	if classify(spec.State, spec.Data) != "INIT" ||
		!exactRecordBytes(filepath.Join(spec.State, "root.json"), claim) {
		return false
	}
	if !exists(spec.Cache) {
		return true
	}
	if !safeRootDir(spec.Cache, os.Geteuid()) {
		return false
	}
	return emptyDir(spec.Cache) ||
		(exactLeafNames(spec.Cache, "owner.json") &&
			exactRecordBytes(filepath.Join(spec.Cache, "owner.json"), namespaceOwner(spec.Instance, "cache")))
}

func initializationCommandRouter(
	spec deterministicWorkerSpec,
	_ processIdentity,
	command string,
	completionHooks ...func(string) error,
) (string, error) {
	lease, err := acquireStableLease(spec.State, struct {
		name string
		typ  int16
	}{"init.lock", fWRLCK})
	if err != nil {
		return "", err
	}
	defer lease.release()
	if fullInitializationValid(spec) {
		if command == "doctor" || command == "recover-repair" {
			return "READY(∅)", nil
		}
		return "", errors.New("initialization command denied")
	}
	var claim rootOwnerRecord
	if readRecord(filepath.Join(spec.State, "root.json"), &claim) != nil {
		return "", errors.New("initialization claim unavailable")
	}
	authority, prefixErr := inspectInitializationPrefixAuthority(spec, claim)
	if prefixErr != nil {
		if !exactInitializationRepairPrefix(spec, claim) {
			return "", errors.New("unrecognized initialization prefix")
		}
		authority.claimIdentity, err = exactRecordFile(filepath.Join(spec.State, "root.json"), claim, nil)
		if err != nil {
			return "", err
		}
		authority.objects, err = actualFixtureObjects(initializationRoots(spec))
		if err != nil {
			return "", err
		}
	}
	if command == "doctor" {
		return "INIT exact durable prefix", nil
	}
	if command != "recover-repair" {
		return "", errors.New("initialization command denied")
	}
	if err = transferInitializationClaim(spec, claim, lease, authority, completionHooks...); err != nil {
		return "", err
	}
	if readRecord(filepath.Join(spec.State, "root.json"), &claim) != nil {
		return "", errors.New("transferred initialization claim unavailable")
	}
	if err = completeInitializationUnderLease(spec, claim, lease, completionHooks...); err != nil {
		return "", err
	}
	if !fullInitializationValid(spec) {
		return "", errors.New("repaired initialization did not reopen as complete")
	}
	return "READY(∅)", nil
}

var orderedInitializationStages = []string{
	"init-lock", "initializing-root", "admission-lock", "writer-lock", "writer-owner",
	"candidates-directory", "journals-directory", "receipts-directory",
	"data-root", "cache-root", "data-owner", "cache-owner", "initialized-root",
}
var orderedInitializationLastDetail string

func buildOrderedInitializationPrefix(base string, stop int) (string, string, string, bool) {
	orderedInitializationLastDetail = ""
	s, d := resetRoots(base, true, false)
	cache := filepath.Join(filepath.Dir(s), "cache")
	r := freshGraphRefs()
	identity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		orderedInitializationLastDetail = "process-identity: " + err.Error()
		return s, d, cache, false
	}
	claim := initializingRoot(r.instance, r.token, 0, identity)
	actions := []func() error{
		func() error { return createDurableEmpty(s, "init.lock") },
		func() error { return publishRecord(s, "root.json", claim) },
		func() error { return createDurableEmpty(s, "admission.lock") },
		func() error { return createDurableEmpty(s, "writer.lock") },
		func() error { return publishRecord(s, "writer-owner.json", releasedWriter(r.instance, "", 0, 0, "")) },
		func() error { return createDurableDirectory(s, "candidates") },
		func() error { return createDurableDirectory(s, "journals") },
		func() error { return createDurableDirectory(s, "receipts") },
		func() error { return createDurableDirectory(filepath.Dir(s), filepath.Base(d)) },
		func() error { return createDurableDirectory(filepath.Dir(s), filepath.Base(cache)) },
		func() error { return publishRecord(d, "owner.json", namespaceOwner(r.instance, "data")) },
		func() error { return publishRecord(cache, "owner.json", namespaceOwner(r.instance, "cache")) },
		func() error { return publishRecord(s, "root.json", initializedRoot(r.instance)) },
	}
	for index := 0; index <= stop; index++ {
		if err = actions[index](); err != nil {
			orderedInitializationLastDetail = orderedInitializationStages[index] + ": " + err.Error()
			return s, d, cache, false
		}
	}
	want := "INIT"
	if stop == len(actions)-1 {
		want = "READY(∅)"
	}
	got := classify(s, d)
	valid := got == want
	if !valid {
		orderedInitializationLastDetail = "classification=" + got + ",want=" + want
	}
	if exists(cache) {
		valid = valid && safeRootDir(cache, os.Geteuid())
		if stop >= 11 {
			valid = valid && exactLeafNames(cache, "owner.json") &&
				exactRecordBytes(filepath.Join(cache, "owner.json"), namespaceOwner(r.instance, "cache"))
		} else {
			valid = valid && emptyDir(cache)
		}
		if !valid && orderedInitializationLastDetail == "" {
			orderedInitializationLastDetail = "cache-relation"
		}
	}
	return s, d, cache, valid
}

func verifyInitializationPublicationBoundaries(root *prooffs.OwnedRoot) (int, int, []string) {
	boundaries := []prooffs.Boundary{
		prooffs.BeforeTemp, prooffs.AfterTemp, prooffs.AfterWrite, prooffs.AfterFileSync,
		prooffs.BeforeRename, prooffs.AfterRename, prooffs.AfterParentSync, prooffs.AfterReopen,
	}
	type publication struct {
		name      string
		priorStop int
		dir       func(string, string, string) string
		file      string
		value     func(rootOwnerRecord, processIdentity) any
		final     bool
	}
	publications := []publication{
		{"initializing-root", 0, func(s, _, _ string) string { return s }, "root.json", func(_ rootOwnerRecord, identity processIdentity) any {
			r := freshGraphRefs()
			return initializingRoot(r.instance, r.token, 0, identity)
		}, false},
		{"writer-owner", 3, func(s, _, _ string) string { return s }, "writer-owner.json", func(root rootOwnerRecord, _ processIdentity) any {
			return releasedWriter(root.Instance, "", 0, 0, "")
		}, false},
		{"data-owner", 9, func(_, d, _ string) string { return d }, "owner.json", func(root rootOwnerRecord, _ processIdentity) any {
			return namespaceOwner(root.Instance, "data")
		}, false},
		{"cache-owner", 10, func(_, _, cache string) string { return cache }, "owner.json", func(root rootOwnerRecord, _ processIdentity) any {
			return namespaceOwner(root.Instance, "cache")
		}, false},
		{"initialized-root", 11, func(s, _, _ string) string { return s }, "root.json", func(root rootOwnerRecord, _ processIdentity) any {
			return initializedRoot(root.Instance)
		}, true},
	}
	verified, total := 0, len(boundaries)*len(publications)
	identity, identityErr := processIdentityForPID(os.Getpid())
	if identityErr != nil {
		return 0, total, []string{"process-identity"}
	}
	var failures []string
	for _, publication := range publications {
		for _, boundary := range boundaries {
			s, d, cache, validPrior := buildOrderedInitializationPrefix(root.Path, publication.priorStop)
			var claim rootOwnerRecord
			if publication.name != "initializing-root" && readRecord(filepath.Join(s, "root.json"), &claim) != nil {
				failures = append(failures, publication.name+"/"+string(boundary)+":root-read")
				continue
			}
			value := publication.value(claim, identity)
			dirPath := publication.dir(s, d, cache)
			dir, err := os.Open(dirPath)
			if err != nil {
				failures = append(failures, publication.name+"/"+string(boundary)+":dir-open")
				continue
			}
			injected := errors.New("initialization publication boundary")
			err = prooffs.AtomicPublishAt(int(dir.Fd()), publication.file, recordBytes(value), prooffs.PublishOptions{Fail: func(got prooffs.Boundary) error {
				if got == boundary {
					return injected
				}
				return nil
			}})
			_ = dir.Close()
			published := boundary == prooffs.AfterRename || boundary == prooffs.AfterParentSync || boundary == prooffs.AfterReopen
			finalExact := exactRecordBytes(filepath.Join(dirPath, publication.file), value)
			residue, _ := filepath.Glob(filepath.Join(dirPath, ".proof-*"))
			wantClass := "INIT"
			if publication.final && published {
				wantClass = "READY(∅)"
			}
			cacheValid := true
			if publication.name == "cache-owner" && published {
				cacheValid = finalExact
			}
			if validPrior && errors.Is(err, injected) && finalExact == published && len(residue) == 0 &&
				classify(s, d) == wantClass && cacheValid {
				verified++
			} else {
				failures = append(failures, publication.name+"/"+string(boundary))
			}
		}
	}
	return verified, total, failures
}

func verifyInitializationLockBoundaries(root *prooffs.OwnedRoot) (int, int, []string) {
	type lockCase struct {
		name      string
		priorStop int
	}
	cases := []lockCase{{"init.lock", -1}, {"admission.lock", 1}, {"writer.lock", 2}}
	boundaries := []string{"before-create", "after-create", "after-file-sync", "after-parent-sync", "after-reopen"}
	verified := 0
	var failures []string
	for _, lock := range cases {
		for boundaryIndex := range boundaries {
			var s, d string
			if lock.priorStop < 0 {
				s, d = resetRoots(root.Path, true, false)
			} else {
				s, d, _, _ = buildOrderedInitializationPrefix(root.Path, lock.priorStop)
			}
			path := filepath.Join(s, lock.name)
			var file *os.File
			var err error
			if boundaryIndex >= 1 {
				file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err == nil {
					err = file.Chmod(0600)
				}
			}
			if err == nil && boundaryIndex >= 2 {
				err = file.Sync()
			}
			if file != nil {
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
			if err == nil && boundaryIndex >= 3 {
				err = syncDir(s)
			}
			if err == nil && boundaryIndex >= 4 && !safeLockUID(path, os.Geteuid()) {
				err = errors.New("lock reopen mismatch")
			}
			want := "INIT"
			if lock.name == "init.lock" && boundaryIndex == 0 {
				want = "CLEAN"
			}
			if err == nil && classify(s, d) == want && (boundaryIndex == 0 || safeLockUID(path, os.Geteuid())) {
				verified++
			} else {
				failures = append(failures, lock.name+"/"+boundaries[boundaryIndex])
			}
		}
	}
	return verified, len(cases) * len(boundaries), failures
}

func runOrderedInitializationPrefixes(root *prooffs.OwnedRoot, add func(Scenario)) {
	verified := 0
	var prefixFailures []string
	for stop, stage := range orderedInitializationStages {
		_, _, _, valid := buildOrderedInitializationPrefix(root.Path, stop)
		if valid {
			verified++

		} else {
			prefixFailures = append(prefixFailures, stage+"("+orderedInitializationLastDetail+")")
		}
	}
	rejected := 0
	s, d, _, _ := buildOrderedInitializationPrefix(root.Path, 4)
	_ = os.Remove(filepath.Join(s, "admission.lock"))
	if classify(s, d) == "BAD corrupt" {
		rejected++
	}
	s, d, _, _ = buildOrderedInitializationPrefix(root.Path, 10)
	_ = writeRecord(filepath.Join(d, "owner.json"), namespaceOwner("mismatched-instance", "data"))
	if classify(s, d) == "BAD corrupt" {
		rejected++
	}
	s, d, _, _ = buildOrderedInitializationPrefix(root.Path, 10)
	_ = os.WriteFile(filepath.Join(s, "unexpected"), []byte("extra"), 0600)
	if classify(s, d) == "BAD corrupt" {
		rejected++
	}
	publicationVerified, publicationTotal, publicationFailures := verifyInitializationPublicationBoundaries(root)
	lockVerified, lockTotal, lockFailures := verifyInitializationLockBoundaries(root)
	status := Observed
	if verified != len(orderedInitializationStages) || rejected != 3 ||
		publicationVerified != publicationTotal || lockVerified != lockTotal {
		status = Failed
	}
	add(Scenario{
		ID: "VAL-025/ordered-initialization-prefixes", Requirements: []string{"VAL-025", "VAL-026", "ARCH-017"}, Status: status,
		Fixture: "independent before/after lock and atomic-publication boundaries, reopened ordered prefixes, and gap/malformed/extra variants", Classification: "INIT prefixes then READY(∅)",
		Writes:             []string{"fixtures/<typed-initialization-prefix>/state", "fixtures/<typed-initialization-prefix>/data", "fixtures/<typed-initialization-prefix>/cache"},
		Observation:        fmt.Sprintf("verified exact ordered prefixes=%d/%d failures=%v, atomic publication boundaries=%d/%d failures=%v, stable-lock create/sync/reopen boundaries=%d/%d failures=%v, and rejected variants=%d/3; coverage includes initializing-root, admission/writer locks, writer-owner, all three state collections, data/cache roots and owners, matching data owner while root remained initializing, and initialized-root publication", verified, len(orderedInitializationStages), prefixFailures, publicationVerified, publicationTotal, publicationFailures, lockVerified, lockTotal, lockFailures, rejected),
		RejectedAssumption: "init.lock plus an arbitrary subset of later publications is an initialization prefix",
	})
}
func prepareInitializationRepairFixture(base, recoveryBoundary string) (deterministicWorkerSpec, error) {
	return prepareInitializationPrefixRepairFixture(base, 4, recoveryBoundary)
}

func prepareInitializationPrefixRepairFixture(base string, stop int, recoveryBoundary string) (deterministicWorkerSpec, error) {
	s, d := resetRoots(base, true, false)
	cache := filepath.Join(filepath.Dir(s), "cache")
	refs := freshGraphRefs()
	spec := workerSpecFor(s, d, cache, "", string(prooffs.AfterReopen), refs)
	spec.RecoveryBoundary = recoveryBoundary
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	actions := []func() error{
		func() error { return createDurableEmpty(s, "init.lock") },
		func() error {
			return publishRecord(s, "root.json", initializingRoot(refs.instance, refs.token, 0, dead))
		},
		func() error { return createDurableEmpty(s, "admission.lock") },
		func() error { return createDurableEmpty(s, "writer.lock") },
		func() error {
			return publishRecord(s, "writer-owner.json", releasedWriter(refs.instance, "", 0, 0, ""))
		},
		func() error { return createDurableDirectory(s, "candidates") },
		func() error { return createDurableDirectory(s, "journals") },
		func() error { return createDurableDirectory(s, "receipts") },
		func() error { return createDurableDirectory(filepath.Dir(s), filepath.Base(d)) },
		func() error { return createDurableDirectory(filepath.Dir(s), filepath.Base(cache)) },
		func() error { return publishRecord(d, "owner.json", namespaceOwner(refs.instance, "data")) },
		func() error { return publishRecord(cache, "owner.json", namespaceOwner(refs.instance, "cache")) },
	}
	if stop < 1 || stop >= len(actions) {
		return spec, errors.New("invalid initialization repair prefix")
	}
	for index := 0; index <= stop; index++ {
		if err := actions[index](); err != nil {
			return spec, err
		}
	}
	if !inspectInitializationPrefix(spec, dead) {
		return spec, errors.New("initialization repair fixture did not reopen exactly")
	}
	return spec, nil
}

func prepareInitializationResidueFixture(base string) (deterministicWorkerSpec, string, error) {
	s, d := resetRoots(base, true, false)
	cache := filepath.Join(filepath.Dir(s), "cache")
	refs := freshGraphRefs()
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	spec := workerSpecFor(s, d, cache, "", string(prooffs.AfterWrite), refs)
	for _, action := range []func() error{
		func() error { return createDurableEmpty(s, "init.lock") },
		func() error {
			return publishRecord(s, "root.json", initializingRoot(refs.instance, refs.token, 0, dead))
		},
		func() error { return createDurableEmpty(s, "admission.lock") },
		func() error { return createDurableEmpty(s, "writer.lock") },
	} {
		if err := action(); err != nil {
			return spec, "", err
		}
	}
	residuePath := filepath.Join(s, ".proof-initialization-residue")
	if err := os.WriteFile(residuePath, recordBytes(releasedWriter(refs.instance, "", 0, 0, "")), 0600); err != nil {
		return spec, "", err
	}
	residueFile, err := os.OpenFile(residuePath, os.O_RDWR, 0)
	if err != nil {
		return spec, "", err
	}
	syncErr := residueFile.Sync()
	closeErr := residueFile.Close()
	if syncErr != nil {
		return spec, "", syncErr
	}
	if closeErr != nil {
		return spec, "", closeErr
	}
	if err := syncDir(s); err != nil {
		return spec, "", err
	}
	if !inspectInitializationPrefix(spec, dead) {
		return spec, "", errors.New("initialization residue fixture did not reopen exactly")
	}
	return spec, residuePath, nil
}

func prepareInitializationTakeoverResidueFixture(base string) (deterministicWorkerSpec, string, error) {
	spec, err := prepareInitializationRepairFixture(base, "")
	if err != nil {
		return spec, "", err
	}
	residuePath := filepath.Join(spec.State, ".proof-takeover-fixture")
	takeover := initializingRoot(
		spec.Instance,
		strings.Repeat("e", 32),
		1,
		processIdentity{PID: 999998, Start: 2, PIDNamespace: 1},
	)
	if err = publishRecord(spec.State, filepath.Base(residuePath), takeover); err != nil {
		return spec, "", err
	}
	if !inspectInitializationPrefix(spec, processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}) {
		return spec, "", errors.New("initialization takeover residue fixture did not reopen exactly")
	}
	return spec, residuePath, nil
}

type fixtureObject struct {
	Kind          string
	Mode          fs.FileMode
	Owner         uint32
	Device, Inode uint64
	Links         uint64
	Bytes         []byte
}

func actualFixtureObjects(roots map[string]string) (map[string]fixtureObject, error) {
	result := make(map[string]fixtureObject)
	for label, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return errors.New("fixture owner identity unavailable")
			}
			object := fixtureObject{Mode: info.Mode(), Owner: st.Uid, Device: uint64(st.Dev), Inode: st.Ino, Links: uint64(st.Nlink)}
			switch {
			case info.IsDir():
				object.Kind = "directory"
			case info.Mode().IsRegular():
				object.Kind = "file"
				object.Bytes, err = os.ReadFile(path)
				if err != nil {
					return err
				}
			default:
				return errors.New("fixture contained unsupported object")
			}
			result[label+":"+filepath.ToSlash(rel)] = object
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func fixtureObjectsEqual(a, b map[string]fixtureObject) bool {
	if len(a) != len(b) {
		return false
	}
	for key, x := range a {
		y, ok := b[key]
		sameIdentity := x.Device == 0 || y.Device == 0 ||
			(x.Device == y.Device && x.Inode == y.Inode && x.Links == y.Links)
		if !ok || x.Kind != y.Kind || x.Mode != y.Mode || x.Owner != y.Owner ||
			!sameIdentity || !bytes.Equal(x.Bytes, y.Bytes) {
			return false
		}
	}
	return true
}

func fixtureObjectsEqualExceptKeys(before, after map[string]fixtureObject, allowed map[string]bool) bool {
	for key, x := range before {
		if allowed[key] {
			continue
		}
		y, ok := after[key]
		sameIdentity := x.Device == 0 || y.Device == 0 ||
			(x.Device == y.Device && x.Inode == y.Inode && x.Links == y.Links)
		if !ok || x.Kind != y.Kind || x.Mode != y.Mode || x.Owner != y.Owner ||
			!sameIdentity || !bytes.Equal(x.Bytes, y.Bytes) {
			return false
		}
	}
	for key := range after {
		if !allowed[key] {
			if _, ok := before[key]; !ok {
				return false
			}
		}
	}
	return true
}

func transitionStageIndex(stage string) int {
	for i, candidate := range deterministicTransitionStages {
		if candidate == stage {
			return i
		}
	}
	return -1
}

func transitionJournalForStage(stage string, r graphRefs) (journalRecord, bool) {
	index := transitionStageIndex(stage)
	if index < transitionStageIndex("journal-prepared-publication") || index >= transitionStageIndex("cleanup-journal-remove") {
		return journalRecord{}, false
	}
	journal := transitionJournal(r, "prepared", "", nil, r.generation, r.writerToken)
	switch {
	case index >= transitionStageIndex("journal-cleanup-publication"):
		journal = transitionJournal(r, "cleanup", "", mutationPhases, r.generation, r.writerToken)
	case index >= transitionStageIndex("journal-pending-receipt-publication"):
		journal = transitionJournal(r, string(phasePending), "", mutationPhases, r.generation, r.writerToken)
	case index >= transitionStageIndex("journal-mutating-home-apply-result-publication"):
		journal = transitionJournal(r, "mutating", "home-apply", mutationPhases, r.generation, r.writerToken)
	case index >= transitionStageIndex("journal-mutating-home-apply-publication"):
		journal = transitionJournal(r, "mutating", "home-apply", mutationPhases[:1], r.generation, r.writerToken)
	case index >= transitionStageIndex("journal-mutating-tool-install-result-publication"):
		journal = transitionJournal(r, "mutating", "tool-install", mutationPhases[:1], r.generation, r.writerToken)
	case index >= transitionStageIndex("journal-mutating-tool-install-publication"):
		journal = transitionJournal(r, "mutating", "tool-install", nil, r.generation, r.writerToken)
	}
	return journal, true
}

func expectedTransitionObjects(stage string, r graphRefs, ownerIdentity processIdentity) map[string]fixtureObject {
	objects := make(map[string]fixtureObject)
	uid := uint32(os.Geteuid())
	addDir := func(key string) {
		objects[key] = fixtureObject{Kind: "directory", Mode: fs.ModeDir | 0700, Owner: uid}
	}
	addFile := func(key string, content []byte) {
		objects[key] = fixtureObject{Kind: "file", Mode: 0600, Owner: uid, Bytes: content}
	}
	addRecord := func(key string, value any) { addFile(key, recordBytes(value)) }
	addDir("state:.")
	addDir("data:.")
	addDir("external:.")
	for _, name := range []string{"candidates", "journals", "receipts"} {
		addDir("state:" + name)
	}
	for _, name := range []string{"init.lock", "admission.lock", "writer.lock"} {
		addFile("state:"+name, nil)
	}
	addRecord("state:root.json", initializedRoot(r.instance))
	addRecord("data:owner.json", namespaceOwner(r.instance, "data"))
	if r.a != "" {
		addRecord("state:candidates/"+r.a+".json", candidateReference(r.instance, r.a))
		addRecord("state:receipts/"+r.receiptA+".json", pendingReceiptFor(r.a, receiptDigest(r.receiptAPrevious)))
		addRecord("state:current.json", baselineCurrent(r))
	}
	index := transitionStageIndex(stage)
	owner := baselineReleasedWriter(r)
	if stage != "transition-base" {
		owner = heldWriter(r.instance, r.writerToken, r.generation, ownerIdentity, r.generation-1, transitionPreviousWriterToken(r))
	}
	if stage == "writer-owner-released-publication" {
		nextGeneration, ok := generationSuccessor(r.generation)
		if !ok {
			return nil
		}
		owner = releasedWriter(r.instance, "", nextGeneration, r.generation, r.writerToken)
	}
	addRecord("state:writer-owner.json", owner)
	if index >= transitionStageIndex("candidate-publication") {
		addRecord("state:candidates/"+r.b+".json", transitionCandidate(r))
	}
	if journal, ok := transitionJournalForStage(stage, r); ok {
		addRecord("state:journals/"+r.journal+".json", journal)
	}
	completed := []string(nil)
	switch {
	case index >= transitionStageIndex("external-home-apply"):
		completed = mutationPhases
	case index >= transitionStageIndex("external-tool-install"):
		completed = mutationPhases[:1]
	}
	if len(completed) > 0 {
		addRecord("external:effect.json", appliedObservation(r.instance, r.b, r.journal, completed))
	}
	if index >= transitionStageIndex("pending-receipt-publication") {
		addRecord("state:receipts/"+r.receiptB+".json", transitionPendingReceipt(r))
	}
	if index >= transitionStageIndex("current-publication") {
		addRecord("state:current.json", transitionCurrent(r))
	}
	if r.a != "" && r.a != r.b && index >= transitionStageIndex("cleanup-candidate-remove") {
		delete(objects, "state:candidates/"+r.a+".json")
	}
	if r.receiptA != "" && r.receiptA != r.receiptB && index >= transitionStageIndex("cleanup-receipt-remove") {
		delete(objects, "state:receipts/"+r.receiptA+".json")
	}
	return objects
}

func transitionPreviousWriterToken(r graphRefs) string {
	return r.writerPredecessorToken
}

func baselineReleasedWriter(r graphRefs) writerOwnerRecord {
	if r.generation <= 1 {
		return releasedWriter(r.instance, "", 0, 0, "")
	}
	return releasedWriter(r.instance, "", r.generation-1, r.generation-2, r.priorWriterToken)
}

func transitionPublicationSpec(s, external string, r graphRefs, stage string, identity processIdentity) (string, string, any, bool) {
	switch stage {
	case "writer-owner-held-publication":
		return s, "writer-owner.json", heldWriter(r.instance, r.writerToken, r.generation, identity, r.generation-1, transitionPreviousWriterToken(r)), true
	case "candidate-publication":
		return filepath.Join(s, "candidates"), r.b + ".json", transitionCandidate(r), true
	case "journal-prepared-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "prepared", "", nil, r.generation, r.writerToken), true
	case "journal-mutating-tool-install-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "mutating", "tool-install", nil, r.generation, r.writerToken), true
	case "external-tool-install":
		return external, "effect.json", appliedObservation(r.instance, r.b, r.journal, mutationPhases[:1]), true
	case "journal-mutating-tool-install-result-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "mutating", "tool-install", mutationPhases[:1], r.generation, r.writerToken), true
	case "journal-mutating-home-apply-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "mutating", "home-apply", mutationPhases[:1], r.generation, r.writerToken), true
	case "external-home-apply":
		return external, "effect.json", appliedObservation(r.instance, r.b, r.journal, mutationPhases), true
	case "journal-mutating-home-apply-result-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "mutating", "home-apply", mutationPhases, r.generation, r.writerToken), true
	case "journal-pending-receipt-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, string(phasePending), "", mutationPhases, r.generation, r.writerToken), true
	case "pending-receipt-publication":
		return filepath.Join(s, "receipts"), r.receiptB + ".json", transitionPendingReceipt(r), true
	case "current-publication":
		return s, "current.json", transitionCurrent(r), true
	case "journal-cleanup-publication":
		return filepath.Join(s, "journals"), r.journal + ".json", transitionJournal(r, "cleanup", "", mutationPhases, r.generation, r.writerToken), true
	case "writer-owner-released-publication":
		nextGeneration, ok := generationSuccessor(r.generation)
		if !ok {
			return "", "", nil, false
		}
		return s, "writer-owner.json", releasedWriter(r.instance, "", nextGeneration, r.generation, r.writerToken), true
	}
	return "", "", nil, false
}

type transitionRecoveryAuthority struct {
	Boundary      transitionBoundary
	Identity      processIdentity
	ExpectResidue bool
	RecoveryStage string
}

type transitionResidue struct {
	key      string
	path     string
	identity prooffs.PublicationIdentity
	object   fixtureObject
}

func validatedTransitionResidue(
	actual map[string]fixtureObject,
	s, external string,
	r graphRefs,
	authority *transitionRecoveryAuthority,
) (*transitionResidue, bool) {
	if authority == nil || authority.Boundary.Kind == "authority" || authority.Boundary.Kind == "durable-removal" {
		return nil, false
	}
	point := authority.Boundary.Point
	if point == string(prooffs.BeforeTemp) || point == string(prooffs.AfterRename) ||
		point == string(prooffs.AfterParentSync) || point == string(prooffs.AfterReopen) {
		return nil, false
	}
	dir, _, value, ok := transitionPublicationSpec(s, external, r, authority.Boundary.Stage, authority.Identity)
	if !ok {
		return nil, false
	}
	rootLabel, relDir := "state", "."
	switch {
	case dir == external:
		rootLabel, relDir = "external", "."
	case strings.HasPrefix(dir, filepath.Join(s, "candidates")):
		relDir = "candidates"
	case strings.HasPrefix(dir, filepath.Join(s, "journals")):
		relDir = "journals"
	case strings.HasPrefix(dir, filepath.Join(s, "receipts")):
		relDir = "receipts"
	}
	prefix := ".proof-"
	if authority.Boundary.Kind == "immutable-no-replace" {
		prefix = ".transition-"
	}
	var found *transitionResidue
	for key, object := range actual {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 || parts[0] != rootLabel || filepath.ToSlash(filepath.Dir(parts[1])) != relDir ||
			!strings.HasPrefix(filepath.Base(parts[1]), prefix) {
			continue
		}
		want := recordBytes(value)
		if point == string(prooffs.AfterTemp) {
			want = nil
		}
		if found != nil || object.Kind != "file" || object.Mode.Perm() != 0600 ||
			object.Owner != uint32(os.Geteuid()) || !bytes.Equal(object.Bytes, want) {
			return nil, false
		}
		root := s
		if rootLabel == "external" {
			root = external
		}
		found = &transitionResidue{
			key: key, path: filepath.Join(root, filepath.FromSlash(parts[1])),
			identity: prooffs.PublicationIdentity{Device: object.Device, Inode: object.Inode},
			object:   object,
		}
	}
	return found, found != nil
}

func findRecoveryTransferResidue(actual map[string]fixtureObject, s, residueDir string) (*transitionResidue, error) {
	var residue *transitionResidue
	for key, object := range actual {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 || parts[0] != "state" ||
			filepath.ToSlash(filepath.Dir(parts[1])) != residueDir ||
			!strings.HasPrefix(filepath.Base(parts[1]), ".proof-") {
			continue
		}
		if residue != nil || object.Kind != "file" || object.Mode.Perm() != 0600 ||
			object.Owner != uint32(os.Geteuid()) || object.Links != 1 {
			return nil, errors.New("recovery transfer residue topology mismatch")
		}
		path := filepath.Join(s, filepath.FromSlash(parts[1]))
		residue = &transitionResidue{
			key: key, path: path,
			identity: prooffs.PublicationIdentity{Device: object.Device, Inode: object.Inode},
			object:   object,
		}
	}
	if residue == nil {
		return nil, errors.New("recovery transfer residue unavailable")
	}
	return residue, nil
}

func recognizeRecoveryTransferResidue(
	s, d, external string,
	r graphRefs,
	authority *transitionRecoveryAuthority,
) (recoverableTransitionPrefix, error) {
	if authority == nil || !authority.ExpectResidue ||
		(authority.RecoveryStage != "recovery-journal-transfer" &&
			authority.RecoveryStage != "recovery-writer-owner-transfer" &&
			authority.RecoveryStage != "recovery-uncommitted-owner-release") {
		return recoverableTransitionPrefix{}, errors.New("recovery transfer residue authority unavailable")
	}
	actual, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		return recoverableTransitionPrefix{}, err
	}
	residueDir := "."
	if authority.RecoveryStage == "recovery-journal-transfer" {
		residueDir = "journals"
	}
	residue, err := findRecoveryTransferResidue(actual, s, residueDir)
	if err != nil {
		return recoverableTransitionPrefix{}, err
	}
	delete(actual, residue.key)
	var owner writerOwnerRecord
	if readRecord(filepath.Join(s, "writer-owner.json"), &owner) != nil ||
		owner.Status != "held" || owner.Instance != r.instance || !validWriterPredecessor(owner) {
		return recoverableTransitionPrefix{}, errors.New("recovery transfer predecessor owner mismatch")
	}
	ownerIdentity, err := exactRecordFile(filepath.Join(s, "writer-owner.json"), owner, nil)
	if err != nil {
		return recoverableTransitionPrefix{}, err
	}
	if owner.Generation != r.generation || owner.Token != r.writerToken {
		return recoverableTransitionPrefix{}, errors.New("recovery transfer predecessor token mismatch")
	}
	point := authority.Boundary.Point
	emptyResidue := point == string(prooffs.AfterTemp)
	if emptyResidue != (len(residue.object.Bytes) == 0) {
		return recoverableTransitionPrefix{}, errors.New("recovery transfer residue content phase mismatch")
	}
	switch authority.RecoveryStage {
	case "recovery-uncommitted-owner-release":
		if !fixtureObjectsEqual(actual, expectedTransitionObjects("writer-owner-held-publication", r, owner.Process)) {
			return recoverableTransitionPrefix{}, errors.New("uncommitted owner residue predecessor mismatch")
		}
		if !emptyResidue && !bytes.Equal(residue.object.Bytes, recordBytes(baselineReleasedWriter(r))) {
			return recoverableTransitionPrefix{}, errors.New("uncommitted owner residue bytes mismatch")
		}
		return recoverableTransitionPrefix{
			stage: "writer-owner-held-publication", refs: r, owner: owner, ownerIdentity: ownerIdentity,
			residue: residue, interruptedIdentity: authority.Identity,
		}, nil
	case "recovery-journal-transfer":
		for _, stage := range transitionStages(r) {
			if !fixtureObjectsEqual(actual, expectedTransitionObjects(stage, r, owner.Process)) {
				continue
			}
			if !emptyResidue {
				var journal journalRecord
				if json.Unmarshal(residue.object.Bytes, &journal) != nil ||
					!isGenerationSuccessor(owner.Generation, journal.WriterGeneration) ||
					journal.WriterPreviousToken != owner.Token || journal.WriterToken == "" {
					return recoverableTransitionPrefix{}, errors.New("recovery journal residue compare value mismatch")
				}
				next := r
				next.generation = journal.WriterGeneration
				next.writerToken = journal.WriterToken
				next.writerPredecessorToken = journal.WriterPreviousToken
				expectedJournal, ok := transitionJournalForStage(stage, next)
				if !ok || !bytes.Equal(residue.object.Bytes, recordBytes(expectedJournal)) {
					return recoverableTransitionPrefix{}, errors.New("recovery journal residue bytes mismatch")
				}
			}
			return recoverableTransitionPrefix{
				stage: stage, refs: r, owner: owner, ownerIdentity: ownerIdentity,
				residue: residue, interruptedIdentity: authority.Identity,
			}, nil
		}
	case "recovery-writer-owner-transfer":
		var journal journalRecord
		if readRecord(filepath.Join(s, "journals", r.journal+".json"), &journal) != nil ||
			!isGenerationSuccessor(owner.Generation, journal.WriterGeneration) ||
			journal.WriterPreviousToken != owner.Token || journal.WriterToken == "" {
			return recoverableTransitionPrefix{}, errors.New("recovery owner residue journal-ahead mismatch")
		}
		next := r
		next.generation = journal.WriterGeneration
		next.writerToken = journal.WriterToken
		next.writerPredecessorToken = journal.WriterPreviousToken
		for _, stage := range transitionStages(next) {
			expected := expectedTransitionObjects(stage, next, owner.Process)
			expected["state:writer-owner.json"] = fixtureObject{
				Kind: "file", Mode: 0600, Owner: uint32(os.Geteuid()), Bytes: recordBytes(owner),
			}
			if !fixtureObjectsEqual(actual, expected) {
				continue
			}
			if !emptyResidue {
				expectedOwner := heldWriter(
					next.instance, next.writerToken, next.generation, authority.Identity,
					next.generation-1, next.writerPredecessorToken,
				)
				if !bytes.Equal(residue.object.Bytes, recordBytes(expectedOwner)) {
					return recoverableTransitionPrefix{}, errors.New("recovery owner residue bytes mismatch")
				}
			}
			return recoverableTransitionPrefix{
				stage: stage, refs: next, owner: owner, ownerIdentity: ownerIdentity,
				residue: residue, interruptedIdentity: authority.Identity, transferPending: true,
			}, nil
		}
	}
	return recoverableTransitionPrefix{}, errors.New("recovery transfer residue prefix mismatch")
}

func recognizeTransitionPrefixDetails(
	s, d, external string,
	r graphRefs,
	authority *transitionRecoveryAuthority,
) (string, *transitionResidue, error) {
	var identityErr error
	r, identityErr = retainTransitionCurrent(s, r)
	if identityErr != nil {
		return "", nil, identityErr
	}

	var owner writerOwnerRecord
	if err := readRecord(filepath.Join(s, "writer-owner.json"), &owner); err != nil {
		return "", nil, err
	}
	if owner.Schema != 1 || owner.Domain != "zerb.writer-owner" || owner.Instance != r.instance || !validWriterPredecessor(owner) {
		return "", nil, errors.New("writer owner schema or instance mismatch")
	}
	actual, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		return "", nil, err
	}
	residue, hasResidue := validatedTransitionResidue(actual, s, external, r, authority)
	if hasResidue {
		delete(actual, residue.key)
	}
	stages := transitionStages(r)
	if owner == baselineReleasedWriter(r) {
		stages = append([]string{"transition-base"}, stages...)
	} else if owner.Status == "held" {
		if owner.Token != r.writerToken || owner.Generation != r.generation || !validProcessIdentity(owner.Process) {
			return "", nil, errors.New("writer owner compare value mismatch")
		}
	} else if owner.Status == "released" {
		if owner.Token != "" || !isGenerationSuccessor(r.generation, owner.Generation) || validProcessIdentity(owner.Process) {
			return "", nil, errors.New("released writer owner mismatch")
		}
	} else {
		return "", nil, errors.New("writer owner status mismatch")
	}
	for _, stage := range stages {
		if owner == baselineReleasedWriter(r) && stage != "transition-base" {
			continue
		}
		if owner.Status == "released" && owner != baselineReleasedWriter(r) && stage != "writer-owner-released-publication" {
			continue
		}
		if fixtureObjectsEqual(actual, expectedTransitionObjects(stage, r, owner.Process)) {
			if authority != nil {
				wantResidue := authority.ExpectResidue &&
					(authority.Boundary.Point == string(prooffs.AfterTemp) ||
						authority.Boundary.Point == string(prooffs.AfterWrite) ||
						authority.Boundary.Point == string(prooffs.AfterFileSync) ||
						authority.Boundary.Point == string(prooffs.BeforeRename))
				if hasResidue != wantResidue {
					return "", nil, errors.New("transition temporary residue relation mismatch")
				}
			}
			return stage, residue, nil
		}
	}
	return "", nil, errors.New("transition prefix was not exactly recognized")
}

func recognizeTransitionPrefixInjected(s, d, external string, r graphRefs, authority *transitionRecoveryAuthority) (string, error) {
	stage, _, err := recognizeTransitionPrefixDetails(s, d, external, r, authority)
	return stage, err
}

func recognizeTransitionPrefix(s, d, external string, r graphRefs) (string, error) {
	return recognizeTransitionPrefixInjected(s, d, external, r, nil)
}

type recoverableTransitionPrefix struct {
	stage               string
	refs                graphRefs
	owner               writerOwnerRecord
	ownerIdentity       prooffs.PublicationIdentity
	residue             *transitionResidue
	interruptedIdentity processIdentity
	transferPending     bool
}

func exactTransitionGraphWithOwner(s, d, external, stage string, r graphRefs, owner writerOwnerRecord) bool {
	actual, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		return false
	}
	expected := expectedTransitionObjects(stage, r, owner.Process)
	expected["state:writer-owner.json"] = fixtureObject{
		Kind:  "file",
		Mode:  0600,
		Owner: uint32(os.Geteuid()),
		Bytes: recordBytes(owner),
	}
	return fixtureObjectsEqual(actual, expected)
}

func retainTransitionCurrent(s string, r graphRefs) (graphRefs, error) {
	r.priorCurrent = baselineCurrent(r)
	var current currentRecord
	if err := readRecord(filepath.Join(s, "current.json"), &current); err != nil {
		if r.a == "" && errors.Is(err, fs.ErrNotExist) {
			return r, nil
		}
		return r, err
	}
	if current.OperationID != r.journal {
		return r, nil
	}
	var owner writerOwnerRecord
	if readRecord(filepath.Join(s, "writer-owner.json"), &owner) != nil ||
		!validCurrentIdentity(current, r.instance) || current.OperationToken != r.operationToken ||
		(r.committedCurrent.Schema != 0 && current != r.committedCurrent) ||
		current.Candidate != receiptDigest(r.b) || current.Receipt != receiptDigest(r.receiptB) ||
		(owner.Status == "held" && !currentMatchesHeldWriter(current, owner)) ||
		(owner.Status == "released" && !currentPrecedesReleasedWriter(current, owner)) {
		return r, errors.New("completed current identity mismatch")
	}
	r.committedCurrent = current
	return r, nil
}

func recognizeRecoverableTransitionPrefix(s, d, external string, r graphRefs, authority *transitionRecoveryAuthority) (recoverableTransitionPrefix, error) {
	var identityErr error
	r, identityErr = retainTransitionCurrent(s, r)
	if identityErr != nil {
		return recoverableTransitionPrefix{}, identityErr
	}

	if authority != nil && authority.RecoveryStage != "" && authority.ExpectResidue {
		return recognizeRecoveryTransferResidue(s, d, external, r, authority)
	}
	stage, residue, initialErr := recognizeTransitionPrefixDetails(s, d, external, r, authority)
	if initialErr == nil {
		if stage == "transition-base" && residue == nil {
			return recoverableTransitionPrefix{}, errors.New("no durable or related interrupted operation")
		}
		var owner writerOwnerRecord
		if readRecord(filepath.Join(s, "writer-owner.json"), &owner) != nil {
			return recoverableTransitionPrefix{}, errors.New("transition owner unavailable")
		}
		identity, identityErr := exactRecordFile(filepath.Join(s, "writer-owner.json"), owner, nil)
		if identityErr != nil {
			return recoverableTransitionPrefix{}, identityErr
		}
		interrupted := processIdentity{}
		if authority != nil {
			interrupted = authority.Identity
		}
		return recoverableTransitionPrefix{
			stage: stage, refs: r, owner: owner, ownerIdentity: identity, residue: residue,
			interruptedIdentity: interrupted,
		}, nil
	}
	var owner writerOwnerRecord
	if err := readRecord(filepath.Join(s, "writer-owner.json"), &owner); err != nil {
		return recoverableTransitionPrefix{}, err
	}
	if owner.Schema != 1 || owner.Domain != "zerb.writer-owner" || owner.Instance != r.instance || !validWriterPredecessor(owner) {
		return recoverableTransitionPrefix{}, errors.New("recovery owner schema or predecessor mismatch")
	}
	identity, err := exactRecordFile(filepath.Join(s, "writer-owner.json"), owner, nil)
	if err != nil {
		return recoverableTransitionPrefix{}, err
	}
	active := r
	switch owner.Status {
	case "held":
		active.generation = owner.Generation
		active.writerToken = owner.Token
		active.writerPredecessorToken = owner.PreviousToken
	case "released":
		if owner.Generation == 0 {
			return recoverableTransitionPrefix{}, errors.New("unrecognized released recovery owner")
		}
		active.generation = owner.PreviousGeneration
		active.writerToken = owner.PreviousToken
	default:
		return recoverableTransitionPrefix{}, errors.New("unrecognized recovery owner status")
	}
	activeStage, activeResidue, activeErr := recognizeTransitionPrefixDetails(s, d, external, active, authority)
	if activeErr == nil {
		interrupted := processIdentity{}
		if authority != nil {
			interrupted = authority.Identity
		}
		return recoverableTransitionPrefix{
			stage: activeStage, refs: active, owner: owner, ownerIdentity: identity,
			residue: activeResidue, interruptedIdentity: interrupted,
		}, nil
	}
	if owner.Status != "held" {
		return recoverableTransitionPrefix{}, errors.New("transition prefix was not exactly recognized")
	}
	var journal journalRecord
	if err = readRecord(filepath.Join(s, "journals", r.journal+".json"), &journal); err != nil ||
		!isGenerationSuccessor(owner.Generation, journal.WriterGeneration) ||
		journal.WriterPreviousToken != owner.Token ||
		journal.WriterToken == "" || journal.OperationToken != r.operationToken {
		return recoverableTransitionPrefix{}, errors.New("transition transfer journal was not exactly recognized")
	}
	next := r
	next.generation = journal.WriterGeneration
	next.writerToken = journal.WriterToken
	next.writerPredecessorToken = journal.WriterPreviousToken
	for _, stage := range transitionStages(next) {
		if _, ok := transitionJournalForStage(stage, next); !ok {
			continue
		}
		if exactTransitionGraphWithOwner(s, d, external, stage, next, owner) {
			return recoverableTransitionPrefix{
				stage: stage, refs: next, owner: owner, ownerIdentity: identity, transferPending: true,
			}, nil
		}
	}
	return recoverableTransitionPrefix{}, errors.New("transition transfer prefix was not exactly recognized")
}

func cleanupTransitionResidue(prefix recoverableTransitionPrefix, hook func(string) error, revalidate func(recoverableTransitionPrefix) error) error {
	if prefix.residue == nil {
		return nil
	}
	hit := func(point string) error {
		if hook == nil {
			return nil
		}
		return hook("recovery-residue-cleanup/" + point)
	}
	if err := hit("before-unlink"); err != nil {
		return err
	}
	info, err := os.Lstat(prefix.residue.path)
	if err != nil || !safeRegularUID(prefix.residue.path, os.Geteuid()) ||
		info.Sys().(*syscall.Stat_t).Nlink != 1 ||
		uint64(info.Sys().(*syscall.Stat_t).Dev) != prefix.residue.identity.Device ||
		info.Sys().(*syscall.Stat_t).Ino != prefix.residue.identity.Inode {
		return errors.New("transition temporary residue identity mismatch")
	}
	content, err := readNoFollowBounded(prefix.residue.path, maxRecognitionFile)
	if err != nil || !bytes.Equal(content, prefix.residue.object.Bytes) {
		return errors.New("transition temporary residue byte mismatch")
	}
	if err = revalidate(prefix); err != nil {
		return err
	}
	if err = os.Remove(prefix.residue.path); err != nil {
		return err
	}
	residuePath := prefix.residue.path
	prefix.residue = nil
	if err = hit("after-unlink"); err != nil {
		return err
	}
	if err = revalidate(prefix); err != nil {
		return err
	}
	if err = syncDir(filepath.Dir(residuePath)); err != nil {
		return err
	}
	if err = hit("after-parent-sync"); err != nil {
		return err
	}
	if err = revalidate(prefix); err != nil {
		return err
	}
	if exists(residuePath) {
		return errors.New("transition temporary residue absence reopen mismatch")
	}
	if err = hit("after-absence-reopen"); err != nil {
		return err
	}
	return revalidate(prefix)
}

func publishReplaceableTransition(dir, name string, value any, stage string, hook func(string) error, capture func(prooffs.PublicationIdentity)) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return prooffs.AtomicPublishAt(int(directory.Fd()), name, recordBytes(value), prooffs.PublishOptions{
		Fail: func(point prooffs.Boundary) error {
			if hook != nil {
				return hook(stage + "/" + string(point))
			}
			return nil
		},
		Capture: capture,
	})
}

func publishImmutableTransition(dir, name string, value any, stage string, hook func(string) error) (retErr error) {
	hit := func(point string) error {
		if hook != nil {
			return hook(stage + "/" + point)
		}
		return nil
	}
	if err := hit(string(prooffs.BeforeTemp)); err != nil {
		return err
	}
	token, err := FreshToken()
	if err != nil {
		return err
	}
	tempPath := filepath.Join(dir, ".transition-"+token)
	temp, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	tempInfo, err := temp.Stat()
	if err != nil {
		_ = temp.Close()
		return err
	}
	renamed := false
	defer func() {
		_ = temp.Close()
		if renamed {
			return
		}
		info, statErr := os.Lstat(tempPath)
		if statErr == nil && os.SameFile(tempInfo, info) {
			_ = os.Remove(tempPath)
		}
	}()
	if err = hit(string(prooffs.AfterTemp)); err != nil {
		return err
	}
	content := recordBytes(value)
	if _, err = temp.Write(content); err != nil {
		return err
	}
	if err = hit(string(prooffs.AfterWrite)); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = hit(string(prooffs.AfterFileSync)); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = hit(string(prooffs.BeforeRename)); err != nil {
		return err
	}
	finalPath := filepath.Join(dir, name)
	linkErr := os.Link(tempPath, finalPath)
	if linkErr != nil && !errors.Is(linkErr, fs.ErrExist) {
		return linkErr
	}
	if err = os.Remove(tempPath); err != nil {
		return err
	}
	renamed = true
	if err = hit(string(prooffs.AfterRename)); err != nil {
		return err
	}
	if err = syncDir(dir); err != nil {
		return err
	}
	if err = hit(string(prooffs.AfterParentSync)); err != nil {
		return err
	}
	if !exactRecordBytes(finalPath, value) {
		if errors.Is(linkErr, fs.ErrExist) {
			return errors.New("existing immutable record does not match exact bytes")
		}
		return errors.New("immutable record reopen mismatch")
	}
	return hit(string(prooffs.AfterReopen))
}

func removeDurableTransition(path, stage string, hook func(string) error) error {
	hit := func(point string) error {
		if hook != nil {
			return hook(stage + "/" + point)
		}
		return nil
	}
	if err := hit("before-unlink"); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := hit("after-unlink"); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := hit("after-parent-sync"); err != nil {
		return err
	}
	if exists(path) {
		return errors.New("durable removal absence reopen mismatch")
	}
	return hit("after-absence-reopen")
}

func revalidateTransitionAuthority(s, d, external string, lease *proofLease, r graphRefs, identity processIdentity, expectedStage string, ownerIdentity prooffs.PublicationIdentity) error {
	if len(lease.files) != 2 || len(lease.names) != 2 {
		return errors.New("transition lease set mismatch")
	}
	for i, name := range lease.names {
		opened, err := lease.files[i].Stat()
		path := filepath.Join(s, name)
		pathInfo, pathErr := os.Lstat(path)
		if err != nil || pathErr != nil || !os.SameFile(opened, pathInfo) || !safeLockUID(path, os.Geteuid()) {
			return errors.New("transition lock authority substitution")
		}
	}
	expected := expectedTransitionObjects(expectedStage, r, identity)
	var owner writerOwnerRecord
	if readRecord(filepath.Join(s, "writer-owner.json"), &owner) != nil ||
		!bytes.Equal(recordBytes(owner), expected["state:writer-owner.json"].Bytes) {
		return errors.New("transition owner authority substitution")
	}
	if _, err := exactRecordFile(filepath.Join(s, "writer-owner.json"), owner, &ownerIdentity); err != nil {
		return errors.New("transition owner authority substitution")
	}
	actual, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		return err
	}
	if !fixtureObjectsEqual(actual, expected) {
		return errors.New("transition protected-graph authority substitution")
	}
	return nil
}

func executeTransition(s, d, external string, r graphRefs, identity processIdentity, ready func() error, hook func(string) error) error {
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", fWRLCK})
	if err != nil {
		return err
	}
	defer lease.release()
	expected := "STABLE(A)"
	if r.a == "" {
		expected = "READY(∅)"
	}
	if classify(s, d) != expected {
		return errors.New("transition admission classification mismatch")
	}
	if err = lease.acquireStableLock(s, "writer.lock", fWRLCK); err != nil {
		return err
	}
	if classify(s, d) != expected {
		return errors.New("transition classification changed after writer acquisition")
	}
	return executeTransitionWithLease(s, d, external, r, identity, ready, hook, lease, false, prooffs.PublicationIdentity{}, nil)
}

func checkTransitionAuthorityBoundary(s, d, external string, lease *proofLease, r graphRefs, identity processIdentity, ownerIdentity prooffs.PublicationIdentity, stage, position, expectedStage string, hook func(string) error) error {
	prefix := stage + "/authority-" + position + "/"
	if hook != nil {
		if err := hook(prefix + "before-revalidation"); err != nil {
			return err
		}
	}
	if err := revalidateTransitionAuthority(s, d, external, lease, r, identity, expectedStage, ownerIdentity); err != nil {
		return err
	}
	if hook != nil {
		if err := hook(prefix + "after-revalidation"); err != nil {
			return err
		}
	}
	return revalidateTransitionAuthority(s, d, external, lease, r, identity, expectedStage, ownerIdentity)
}

func publishTransitionOwnerIdentity(dir, name string, value any, stage string, hook func(string) error, ownerIdentity *prooffs.PublicationIdentity) error {
	var published prooffs.PublicationIdentity
	var capture func(prooffs.PublicationIdentity)
	if name == "writer-owner.json" {
		capture = func(identity prooffs.PublicationIdentity) { published = identity }
	}
	if err := publishReplaceableTransition(dir, name, value, stage, hook, capture); err != nil {
		return err
	}
	if capture != nil {
		*ownerIdentity = published
	}
	return nil
}

func executeTransitionWithLease(s, d, external string, r graphRefs, identity processIdentity, ready func() error, hook func(string) error, provided *proofLease, heldAlready bool, providedOwner prooffs.PublicationIdentity, finalOwner *prooffs.PublicationIdentity, resumeStage ...string) error {
	lease := provided
	if lease == nil || len(lease.files) != 2 || len(lease.names) != 2 ||
		lease.names[0] != "admission.lock" || lease.names[1] != "writer.lock" {
		return errors.New("transition lease set mismatch")
	}
	var err error
	if !validProcessIdentity(identity) {
		return errors.New("invalid writer process identity")
	}
	releaseGeneration, ok := generationSuccessor(r.generation)
	if !ok {
		return errors.New("writer generation exhausted")
	}
	if ready != nil {
		if err = ready(); err != nil {
			return err
		}
	}
	var initialOwner writerOwnerRecord
	if readRecord(filepath.Join(s, "writer-owner.json"), &initialOwner) != nil {
		return errors.New("transition initial owner unavailable")
	}
	ownerIdentity := providedOwner
	if ownerIdentity == (prooffs.PublicationIdentity{}) {
		ownerIdentity, err = exactRecordFile(filepath.Join(s, "writer-owner.json"), initialOwner, nil)
	} else {
		_, err = exactRecordFile(filepath.Join(s, "writer-owner.json"), initialOwner, &ownerIdentity)
	}
	if err != nil {
		return err
	}
	checkpoint := func(stage, position, expectedStage string) error {
		return checkTransitionAuthorityBoundary(s, d, external, lease, r, identity, ownerIdentity, stage, position, expectedStage, hook)
	}
	currentStage := "transition-base"
	if heldAlready {
		currentStage = "writer-owner-held-publication"
	}
	if len(resumeStage) > 0 {
		if len(resumeStage) != 1 || transitionStageIndex(resumeStage[0]) < 0 {
			return errors.New("invalid transition resume stage")
		}
		currentStage = resumeStage[0]
	}
	runStage := func(stage string, action func() error) error {
		if transitionStageIndex(stage) <= transitionStageIndex(currentStage) {
			return nil
		}
		if err := checkpoint(stage, "pre", currentStage); err != nil {
			return err
		}
		if err := action(); err != nil {
			return err
		}
		if err := checkpoint(stage, "post", stage); err != nil {
			return err
		}
		currentStage = stage
		return nil
	}
	publish := func(stage, dir, name string, value any) error {
		return runStage(stage, func() error {
			return publishTransitionOwnerIdentity(dir, name, value, stage, hook, &ownerIdentity)
		})
	}
	if !heldAlready {
		if err = publish("writer-owner-held-publication", s, "writer-owner.json", heldWriter(r.instance, r.writerToken, r.generation, identity, r.generation-1, transitionPreviousWriterToken(r))); err != nil {
			return err
		}
	}
	if err = runStage("candidate-publication", func() error {
		return publishImmutableTransition(filepath.Join(s, "candidates"), r.b+".json", transitionCandidate(r), "candidate-publication", hook)
	}); err != nil {
		return err
	}
	if err = publish("journal-prepared-publication", filepath.Join(s, "journals"), r.journal+".json", transitionJournal(r, "prepared", "", nil, r.generation, r.writerToken)); err != nil {
		return err
	}
	completed := make([]string, 0, len(mutationPhases))
	for _, phase := range mutationPhases {
		stage := "journal-" + phase + "-publication"
		if err = publish(stage, filepath.Join(s, "journals"), r.journal+".json", transitionJournal(r, "mutating", phase, completed, r.generation, r.writerToken)); err != nil {
			return err
		}
		completed = append(completed, phase)
		if err = publish("external-"+strings.TrimPrefix(phase, "mutating-"), external, "effect.json", appliedObservation(r.instance, r.b, r.journal, completed)); err != nil {
			return err
		}
		if err = publish("journal-"+phase+"-result-publication", filepath.Join(s, "journals"), r.journal+".json", transitionJournal(r, "mutating", phase, completed, r.generation, r.writerToken)); err != nil {
			return err
		}
	}
	if err = publish("journal-pending-receipt-publication", filepath.Join(s, "journals"), r.journal+".json", transitionJournal(r, string(phasePending), "", mutationPhases, r.generation, r.writerToken)); err != nil {
		return err
	}
	if err = runStage("pending-receipt-publication", func() error {
		return publishImmutableTransition(filepath.Join(s, "receipts"), r.receiptB+".json", transitionPendingReceipt(r), "pending-receipt-publication", hook)
	}); err != nil {
		return err
	}
	if err = publish("current-publication", s, "current.json", transitionCurrent(r)); err != nil {
		return err
	}
	if err = publish("journal-cleanup-publication", filepath.Join(s, "journals"), r.journal+".json", transitionJournal(r, "cleanup", "", mutationPhases, r.generation, r.writerToken)); err != nil {
		return err
	}
	if r.a != "" && r.a != r.b {
		if err = runStage("cleanup-candidate-remove", func() error {
			return removeDurableTransition(filepath.Join(s, "candidates", r.a+".json"), "cleanup-candidate-remove", hook)
		}); err != nil {
			return err
		}
	}
	if r.receiptA != "" && r.receiptA != r.receiptB {
		if err = runStage("cleanup-receipt-remove", func() error {
			return removeDurableTransition(filepath.Join(s, "receipts", r.receiptA+".json"), "cleanup-receipt-remove", hook)
		}); err != nil {
			return err
		}
	}
	if err = runStage("cleanup-journal-remove", func() error {
		return removeDurableTransition(filepath.Join(s, "journals", r.journal+".json"), "cleanup-journal-remove", hook)
	}); err != nil {
		return err
	}
	err = publish("writer-owner-released-publication", s, "writer-owner.json", releasedWriter(r.instance, "", releaseGeneration, r.generation, r.writerToken))
	if err == nil && finalOwner != nil {
		*finalOwner = ownerIdentity
	}
	return err
}

func revalidateRecoveryTransferAuthority(s, d, external string, lease *proofLease, prefix recoverableTransitionPrefix) error {
	if len(lease.files) != 2 || len(lease.names) != 2 {
		return errors.New("recovery lease set mismatch")
	}
	for i, name := range lease.names {
		opened, err := lease.files[i].Stat()
		path := filepath.Join(s, name)
		pathInfo, pathErr := os.Lstat(path)
		if err != nil || pathErr != nil || !os.SameFile(opened, pathInfo) || !safeLockUID(path, os.Geteuid()) {
			return errors.New("recovery lock authority substitution")
		}
	}
	if _, err := exactRecordFile(filepath.Join(s, "writer-owner.json"), prefix.owner, &prefix.ownerIdentity); err != nil {
		return errors.New("recovery owner authority substitution")
	}
	actual, err := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
	if err != nil {
		return err
	}
	if prefix.residue != nil {
		object, ok := actual[prefix.residue.key]
		if !ok || object.Kind != prefix.residue.object.Kind || object.Mode != prefix.residue.object.Mode ||
			object.Owner != prefix.residue.object.Owner || object.Device != prefix.residue.identity.Device ||
			object.Inode != prefix.residue.identity.Inode || object.Links != prefix.residue.object.Links ||
			!bytes.Equal(object.Bytes, prefix.residue.object.Bytes) {
			return errors.New("recovery residue authority substitution")
		}
		delete(actual, prefix.residue.key)
	}
	expected := expectedTransitionObjects(prefix.stage, prefix.refs, prefix.owner.Process)
	expected["state:writer-owner.json"] = fixtureObject{
		Kind: "file", Mode: 0600, Owner: uint32(os.Geteuid()), Bytes: recordBytes(prefix.owner),
	}
	if !fixtureObjectsEqual(actual, expected) {
		return errors.New("recovery protected-graph authority substitution")
	}
	return nil
}

func convergeTransitionUnderLease(
	s, d, external string,
	lease *proofLease,
	prefix recoverableTransitionPrefix,
	hook func(string) error,
) error {
	hit := func(name string) error {
		if hook == nil {
			return nil
		}
		return hook(name)
	}
	currentIdentity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	switch prefix.owner.Status {
	case "held":
		if !validProcessIdentity(prefix.owner.Process) {
			return errors.New("writer owner compare-transfer failed")
		}
		live, liveErr := processIdentityLive(prefix.owner.Process)
		if liveErr != nil {
			return fmt.Errorf("writer owner liveness unavailable: %w", liveErr)
		}
		if live {
			return errors.New("writer owner remains live")
		}
	case "released":
		if prefix.stage != "transition-base" || prefix.residue == nil ||
			!validProcessIdentity(prefix.interruptedIdentity) {
			return errors.New("released owner has no related interrupted publication")
		}
		live, liveErr := processIdentityLive(prefix.interruptedIdentity)
		if liveErr != nil {
			return fmt.Errorf("interrupted publisher liveness unavailable: %w", liveErr)
		}
		if live {
			return errors.New("interrupted publisher remains live")
		}
	default:
		return errors.New("writer owner compare-transfer failed")
	}
	// Reserve the complete acquire/release generation budget before removing evidence.
	active := prefix.refs
	if transitionStageIndex(prefix.stage) >= transitionStageIndex("journal-prepared-publication") {
		if !prefix.transferPending {
			nextGeneration, successorOK := generationSuccessor(prefix.owner.Generation)
			if !successorOK {
				return errors.New("writer generation exhausted")
			}
			if _, releaseOK := generationSuccessor(nextGeneration); !releaseOK {
				return errors.New("writer release generation exhausted")
			}
			active.generation = nextGeneration
			active.writerToken, err = FreshToken()
			if err != nil {
				return err
			}
			active.writerPredecessorToken = prefix.owner.Token
		} else if _, releaseOK := generationSuccessor(active.generation); !releaseOK {
			return errors.New("writer release generation exhausted")
		}
	}
	if err = hit("recovery-predecessor-authority/before-revalidation"); err != nil {
		return err
	}
	if err = revalidateRecoveryTransferAuthority(s, d, external, lease, prefix); err != nil {
		return err
	}
	if err = hit("recovery-predecessor-authority/after-revalidation"); err != nil {
		return err
	}
	if err = cleanupTransitionResidue(prefix, hook, func(current recoverableTransitionPrefix) error {
		return revalidateRecoveryTransferAuthority(s, d, external, lease, current)
	}); err != nil {
		return err
	}
	prefix.residue = nil
	if err = revalidateRecoveryTransferAuthority(s, d, external, lease, prefix); err != nil {
		return err
	}
	if prefix.stage == "transition-base" {
		return nil
	}
	if transitionStageIndex(prefix.stage) < transitionStageIndex("journal-prepared-publication") {
		if prefix.stage == "candidate-publication" && prefix.refs.b != prefix.refs.a {
			if err = removeDurableTransition(
				filepath.Join(s, "candidates", prefix.refs.b+".json"),
				"recovery-uncommitted-candidate-remove", hook,
			); err != nil {
				return err
			}
			prefix.stage = "writer-owner-held-publication"
			if err = revalidateRecoveryTransferAuthority(s, d, external, lease, prefix); err != nil {
				return err
			}
		}
		released := baselineReleasedWriter(prefix.refs)
		var releasedIdentity prooffs.PublicationIdentity
		if err = publishReplaceableTransition(
			s, "writer-owner.json", released, "recovery-uncommitted-owner-release", hook,
			func(identity prooffs.PublicationIdentity) { releasedIdentity = identity },
		); err != nil {
			return err
		}
		if _, err = exactRecordFile(filepath.Join(s, "writer-owner.json"), released, &releasedIdentity); err != nil {
			return errors.New("uncommitted owner release reopen mismatch")
		}
		return hit("recovery-uncommitted-owner-release/after-action")
	}
	if !prefix.transferPending {
		if journal, ok := transitionJournalForStage(prefix.stage, active); ok {
			if err = publishReplaceableTransition(
				filepath.Join(s, "journals"), active.journal+".json", journal,
				"recovery-journal-transfer", hook, nil,
			); err != nil {
				return err
			}
			pending := prefix
			pending.refs = active
			pending.transferPending = true
			if err = revalidateRecoveryTransferAuthority(s, d, external, lease, pending); err != nil {
				return err
			}
			if err = hit("recovery-journal-transfer/after-action"); err != nil {
				return err
			}
			if err = revalidateRecoveryTransferAuthority(s, d, external, lease, pending); err != nil {
				return err
			}
		}
	}
	newOwner := heldWriter(
		active.instance, active.writerToken, active.generation, currentIdentity,
		active.generation-1, active.writerPredecessorToken,
	)
	var ownerIdentity prooffs.PublicationIdentity
	if err = publishReplaceableTransition(
		s, "writer-owner.json", newOwner, "recovery-writer-owner-transfer", hook,
		func(identity prooffs.PublicationIdentity) { ownerIdentity = identity },
	); err != nil {
		return err
	}
	if err = revalidateTransitionAuthority(s, d, external, lease, active, currentIdentity, prefix.stage, ownerIdentity); err != nil {
		return err
	}
	if err = hit("recovery-writer-owner-transfer/after-action"); err != nil {
		return err
	}
	if err = revalidateTransitionAuthority(s, d, external, lease, active, currentIdentity, prefix.stage, ownerIdentity); err != nil {
		return err
	}
	return executeTransitionWithLease(
		s, d, external, active, currentIdentity, nil, hook,
		lease, true, ownerIdentity, nil, prefix.stage,
	)
}

func transitionRecoveryAdmission(prefix recoverableTransitionPrefix, command string) error {
	if prefix.stage == "writer-owner-released-publication" && prefix.residue == nil {
		return errors.New("no interrupted operation remains")
	}
	if command == "recover-resume" && recoveryCommandForStage(prefix.stage) == "recover-repair" {
		return errors.New("journal-less interruption requires repair")
	}
	return nil
}

func transitionCommandRouterInjected(
	s, d, external string,
	r graphRefs,
	command string,
	authority *transitionRecoveryAuthority,
	recoveryHooks ...func(string) error,
) (string, error) {
	leaseType := int16(fRDLCK)
	if command == "recover-resume" || command == "recover-repair" {
		leaseType = fWRLCK
	} else if command != "doctor" {
		return "", errors.New("transition command denied")
	}
	lease, err := acquireStableLease(s, struct {
		name string
		typ  int16
	}{"admission.lock", leaseType})
	if err != nil {
		return "", err
	}
	defer lease.release()
	prefix, err := recognizeRecoverableTransitionPrefix(s, d, external, r, authority)
	if err != nil {
		if command == "doctor" {
			stage, recognizeErr := recognizeTransitionPrefixInjected(s, d, external, r, authority)
			if recognizeErr == nil {
				return stage, nil
			}
		}
		return "", err
	}
	if command == "doctor" {
		return prefix.stage, nil
	}
	if err = transitionRecoveryAdmission(prefix, command); err != nil {
		return "", err
	}
	if err = lease.acquireStableLock(s, "writer.lock", fWRLCK); err != nil {
		return "", err
	}
	prefix, err = recognizeRecoverableTransitionPrefix(s, d, external, r, authority)
	if err != nil {
		return "", err
	}
	if err = transitionRecoveryAdmission(prefix, command); err != nil {
		return "", err
	}
	if prefix.stage == "writer-owner-released-publication" {
		if err = revalidateRecoveryTransferAuthority(s, d, external, lease, prefix); err != nil {
			return "", err
		}
		if classify(s, d) != "STABLE(A)" {
			return "", errors.New("released transition did not classify stable")
		}
		return "STABLE(A)", nil
	}
	var hook func(string) error
	if len(recoveryHooks) > 1 {
		return "", errors.New("multiple recovery hooks")
	}
	if len(recoveryHooks) == 1 {
		hook = recoveryHooks[0]
	}
	if command == "recover-repair" {
		repaired, handled, repairErr := repairJournalLessUnderLease(s, d, external, lease, prefix, hook)
		if repairErr != nil {
			return "", repairErr
		}
		if handled {
			return repaired.classification, nil
		}
	}
	if err = convergeTransitionUnderLease(s, d, external, lease, prefix, hook); err != nil {
		return "", err
	}
	state := classify(s, d)
	if state != "STABLE(A)" && state != "READY(∅)" {
		return "", errors.New("transition recovery did not converge stable prior authority")
	}
	return state, nil
}

func transitionCommandRouter(s, d, external string, r graphRefs, command string) (string, error) {
	return transitionCommandRouterInjected(s, d, external, r, command, nil)
}

func verifyPendingCurrentInterval(s string, r graphRefs) bool {
	return exactRecordBytes(filepath.Join(s, "receipts", r.receiptB+".json"), transitionPendingReceipt(r)) &&
		exactRecordBytes(filepath.Join(s, "current.json"), baselineCurrent(r))
}

func verifyCompletionPublication(s string, r graphRefs, requireCurrent, requireClean bool) bool {
	candidatePath := filepath.Join(s, "candidates", r.b+".json")
	receiptPath := filepath.Join(s, "receipts", r.receiptB+".json")
	candidateBytes, candidateErr := os.ReadFile(candidatePath)
	receiptBytes, receiptErr := os.ReadFile(receiptPath)
	if candidateErr != nil || receiptErr != nil || !safeRegularUID(candidatePath, os.Geteuid()) || !safeRegularUID(receiptPath, os.Geteuid()) {
		return false
	}
	candidate := transitionCandidate(r)
	receipt := transitionPendingReceipt(r)
	if !bytes.Equal(candidateBytes, recordBytes(candidate)) || !bytes.Equal(receiptBytes, recordBytes(receipt)) {
		return false
	}
	candidateHash := sha256.Sum256(candidateBytes)
	if receipt.Facts.Candidate != receiptDigest(r.b) ||
		receipt.Digest != receiptDigest(r.receiptB) ||
		receiptDigest("sha256:"+hex.EncodeToString(candidateHash[:])) != receipt.Facts.Candidate {
		return false
	}
	if requireCurrent {
		var current currentRecord
		if readRecord(filepath.Join(s, "current.json"), &current) != nil || !validCurrentIdentity(current, r.instance) ||
			current.OperationID != r.journal || current.OperationToken != r.operationToken ||
			(r.committedCurrent.Schema != 0 && current != r.committedCurrent) ||
			current.Candidate != receipt.Facts.Candidate || current.Receipt != receipt.Digest {
			return false
		}
	}
	if requireClean {
		if !exactLeafNames(filepath.Join(s, "candidates"), r.b+".json") ||
			!exactLeafNames(filepath.Join(s, "receipts"), r.receiptB+".json") ||
			!emptyDir(filepath.Join(s, "journals")) {
			return false
		}
	}
	return candidate.Schema == 1 && candidate.Domain == "zerb.candidate" &&
		receipt.Schema == 1 && receipt.Domain == "zerb.receipt" &&
		receipt.Facts.Schema == 1 && receipt.Facts.Domain == "zerb.completed" &&
		slicesEqual(receipt.Facts.Phases, receiptPhases())
}

func syncFixtureTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			if err != nil {
				return err
			}
			return errors.New("unsupported durable fixture object")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		return err
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err = syncDir(directories[i]); err != nil {
			return err
		}
	}
	return nil
}

func prepareTransitionFixture(base string) (string, string, string, graphRefs, error) {
	s, d, refs := makeGraph(base, "STABLE(A)")
	external := filepath.Join(filepath.Dir(s), "external")
	if err := os.Mkdir(external, 0700); err != nil {
		return "", "", "", refs, err
	}
	if err := syncFixtureTree(s); err != nil {
		return "", "", "", refs, err
	}
	if err := syncFixtureTree(d); err != nil {
		return "", "", "", refs, err
	}
	if err := syncFixtureTree(external); err != nil {
		return "", "", "", refs, err
	}
	if err := syncDir(filepath.Dir(s)); err != nil || syncDir(filepath.Dir(filepath.Dir(s))) != nil {
		return "", "", "", refs, errors.New("failed to sync transition fixture parents")
	}
	return s, d, external, refs, nil
}

func prepareFirstBootstrapFixture(base string) (string, string, string, graphRefs, error) {
	s, d, refs := makeGraph(base, "READY(∅)")
	refs.a = ""
	refs.receiptA = ""
	refs.receiptB = string(pendingReceiptFor(refs.b, "").Digest)
	external := filepath.Join(filepath.Dir(s), "external")
	if err := os.Mkdir(external, 0700); err != nil {
		return "", "", "", refs, err
	}
	for _, path := range []string{s, d, external} {
		if err := syncFixtureTree(path); err != nil {
			return "", "", "", refs, err
		}
	}
	if err := syncDir(filepath.Dir(s)); err != nil || syncDir(filepath.Dir(filepath.Dir(s))) != nil {
		return "", "", "", refs, errors.New("failed to sync first-bootstrap fixture parents")
	}

	return s, d, external, refs, nil
}
func prepareExactReapplyFixture(base string, priorReapplies int) (string, string, string, graphRefs, error) {
	s, d, refs := makeGraph(base, "STABLE(A)")
	previousCandidate := ""
	for range priorReapplies {
		next := pendingReceiptFor(refs.a, receiptDigest(refs.a))
		if err := os.Remove(filepath.Join(s, "receipts", refs.receiptA+".json")); err != nil {
			return "", "", "", refs, err
		}
		if err := writeRecord(filepath.Join(s, "receipts", string(next.Digest)+".json"), next); err != nil {
			return "", "", "", refs, err
		}
		if err := writeRecord(filepath.Join(s, "current.json"), stableCurrent(refs.instance, refs.a, string(next.Digest), refs.journal)); err != nil {
			return "", "", "", refs, err
		}
		previousCandidate, refs.receiptA = refs.a, string(next.Digest)
	}
	refs.priorCurrent = currentRecord{}
	refs.generation = uint64(priorReapplies)*2 + 3
	if refs.generation > 1 {
		if err := writeRecord(filepath.Join(s, "writer-owner.json"), releasedWriter(refs.instance, "", refs.generation-1, refs.generation-2, refs.priorWriterToken)); err != nil {
			return "", "", "", refs, err
		}
	}
	refs.priorCurrent = baselineCurrent(refs)
	if err := writeRecord(filepath.Join(s, "current.json"), baselineCurrent(refs)); err != nil {
		return "", "", "", refs, err
	}
	refs.receiptAPrevious = previousCandidate
	refs.b = refs.a
	refs.receiptB = string(pendingReceiptFor(refs.b, receiptDigest(refs.a)).Digest)
	external := filepath.Join(filepath.Dir(s), "external")
	if err := os.Mkdir(external, 0700); err != nil {
		return "", "", "", refs, err
	}
	for _, path := range []string{s, d, external} {
		if err := syncFixtureTree(path); err != nil {
			return "", "", "", refs, err
		}
	}
	if err := syncDir(filepath.Dir(s)); err != nil || syncDir(filepath.Dir(filepath.Dir(s))) != nil {
		return "", "", "", refs, errors.New("failed to sync exact-reapply fixture parents")
	}
	return s, d, external, refs, nil
}

func transitionVariantRefs(kind string) graphRefs {
	switch kind {
	case "bootstrap":
		return freshBootstrapRefs()
	case "exact-reapply":
		r := freshGraphRefs()
		r.b = r.a
		r.receiptB = string(pendingReceiptFor(r.b, receiptDigest(r.a)).Digest)
		return r
	case "multi-generation-exact-reapply":
		r := freshGraphRefs()
		r.b = r.a
		r.receiptAPrevious = r.a
		r.receiptA = string(pendingReceiptFor(r.a, receiptDigest(r.a)).Digest)
		r.receiptB = r.receiptA
		r.generation = 9
		return r
	default:
		return freshGraphRefs()
	}
}

func prepareTransitionVariant(base, kind string) (string, string, string, graphRefs, error) {
	switch kind {
	case "bootstrap":
		return prepareFirstBootstrapFixture(base)
	case "exact-reapply":
		return prepareExactReapplyFixture(base, 0)
	case "multi-generation-exact-reapply":
		return prepareExactReapplyFixture(base, 3)
	default:
		return prepareTransitionFixture(base)
	}
}

func transitionTempResidueCount(s, external string) int {
	count := 0
	for _, dir := range []string{s, filepath.Join(s, "candidates"), filepath.Join(s, "journals"), filepath.Join(s, "receipts"), external} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".proof-") || strings.HasPrefix(entry.Name(), ".transition-") {
				count++
			}
		}
	}
	return count
}

func priorTransitionAuthorityPreserved(s, d, external string, refs graphRefs) bool {
	want := "STABLE(A)"
	currentPreserved := exactRecordBytes(
		filepath.Join(s, "current.json"),
		baselineCurrent(refs),
	)
	if refs.a == "" {
		want = "READY(∅)"
		currentPreserved = !exists(filepath.Join(s, "current.json"))
	}
	return classify(s, d) == want && currentPreserved &&
		!exists(filepath.Join(s, "journals", refs.journal+".json")) &&
		!exists(filepath.Join(external, "effect.json")) &&
		exactRecordBytes(filepath.Join(s, "writer-owner.json"), baselineReleasedWriter(refs)) &&
		transitionTempResidueCount(s, external) == 0
}

func recoveryConvergedAsExpected(s, d, external string, refs graphRefs, retainedStage, recovered string) bool {
	if transitionStageIndex(retainedStage) >= transitionStageIndex("journal-prepared-publication") {
		return recovered == "STABLE(A)" && transitionTempResidueCount(s, external) == 0 &&
			verifyCompletionPublication(s, refs, true, true)
	}
	wantRecovered := "STABLE(A)"
	if refs.a == "" {
		wantRecovered = "READY(∅)"
	}
	return recovered == wantRecovered && priorTransitionAuthorityPreserved(s, d, external, refs)
}

func transitionMatrixRecoveryExpected(
	s, d, external string,
	refs graphRefs,
	relatedResidue bool,
	durableStage, recovered string,
	recoveryErr error,
) bool {
	if durableStage == "transition-base" && !relatedResidue {
		return recovered == "" && recoveryErr != nil &&
			priorTransitionAuthorityPreserved(s, d, external, refs)
	}
	if durableStage == "writer-owner-released-publication" && !relatedResidue {
		return recovered == "" && recoveryErr != nil && verifyCompletionPublication(s, refs, true, true)
	}
	return recoveryErr == nil &&
		recoveryConvergedAsExpected(s, d, external, refs, durableStage, recovered)
}

func recoveryCommandForStage(stage string) string {
	if transitionStageIndex(stage) < transitionStageIndex("journal-prepared-publication") ||
		transitionStageIndex(stage) >= transitionStageIndex("cleanup-journal-remove") {
		return "recover-repair"
	}
	return "recover-resume"
}

type recoveryActionCase struct {
	start     string
	boundary  string
	wantPrior bool
	residue   bool
}

func recoveryActionCases(r graphRefs) []recoveryActionCase {
	var cases []recoveryActionCase
	for _, boundary := range transitionBoundaries(r) {
		if transitionStageIndex(boundary.Stage) > transitionStageIndex("journal-prepared-publication") {
			cases = append(cases, recoveryActionCase{
				start: "journal-prepared-publication", boundary: boundary.Name,
			})
		}
	}
	for _, boundary := range []string{
		"recovery-predecessor-authority/before-revalidation",
		"recovery-predecessor-authority/after-revalidation",
		"recovery-journal-transfer/after-action",
		"recovery-writer-owner-transfer/after-action",
	} {
		cases = append(cases, recoveryActionCase{start: "journal-prepared-publication", boundary: boundary})
	}
	for _, stage := range []string{"recovery-journal-transfer", "recovery-writer-owner-transfer"} {
		for _, point := range atomicTransitionPoints {
			cases = append(cases, recoveryActionCase{
				start: "journal-prepared-publication", boundary: stage + "/" + point,
			})
		}
	}
	if r.b != r.a {
		for _, point := range removalTransitionPoints {
			cases = append(cases, recoveryActionCase{
				start: "candidate-publication", boundary: "recovery-uncommitted-candidate-remove/" + point, wantPrior: true,
			})
		}
	}
	for _, point := range atomicTransitionPoints {
		cases = append(cases, recoveryActionCase{
			start: "candidate-publication", boundary: "recovery-uncommitted-owner-release/" + point, wantPrior: true,
		})
	}
	cases = append(cases, recoveryActionCase{
		start: "candidate-publication", boundary: "recovery-uncommitted-owner-release/after-action", wantPrior: true,
	})
	for _, point := range removalTransitionPoints {
		cases = append(cases, recoveryActionCase{
			start: "journal-prepared-publication", boundary: "recovery-residue-cleanup/" + point, residue: true,
		})
	}
	return cases
}

func prepareTransitionRecoveryFixture(base, kind, stage string) (string, string, string, graphRefs, error) {
	s, d, external, refs, err := prepareTransitionVariant(base, kind)
	if err != nil {
		return s, d, external, refs, err
	}
	if stage == "transition-base" {
		return s, d, external, refs, errors.New("stable transition base is not a recovery fixture")
	}
	dead := processIdentity{PID: 999999, Start: 1, PIDNamespace: 1}
	injected := errors.New("prepare recovery prefix")
	err = executeTransition(s, d, external, refs, dead, nil, func(name string) error {
		if name == stage+"/authority-post/after-revalidation" {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) {
		return s, d, external, refs, errors.New("failed to prepare exact recovery prefix")
	}
	got, recognizeErr := recognizeTransitionPrefix(s, d, external, refs)
	want := durableStageAfterAction(stage, refs)
	if recognizeErr != nil || got != want {
		return s, d, external, refs, errors.New("prepared recovery prefix did not reopen exactly")
	}
	return s, d, external, refs, nil
}

func runHardDeathTransitionMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, []string) {
	if cfg.SelfExecutable == "" {
		return 0, nil
	}
	verified := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		template := transitionVariantRefs(kind)
		for _, boundary := range transitionBoundaries(template) {
			s, d, external, refs, err := prepareTransitionVariant(root.Path, kind)
			if err != nil {
				failures = append(failures, "hard/"+kind+"/"+boundary.Name+"/prepare")
				continue
			}
			writerBefore, err := os.Lstat(filepath.Join(s, "writer.lock"))
			if err != nil {
				failures = append(failures, "hard/"+kind+"/"+boundary.Name+"/writer-stat")
				continue
			}
			spec := workerSpecFor(s, d, "", external, boundary.Name, refs)
			specPath := filepath.Join(filepath.Dir(s), "worker-spec.json")
			if writeDeterministicWorkerSpec(specPath, spec) != nil {
				failures = append(failures, "hard/"+kind+"/"+boundary.Name+"/spec")
				continue
			}
			child, err := startGatedChild(ctx, cfg.SelfExecutable, "transition-die", specPath, root)
			if err != nil {
				failures = append(failures, "hard/"+kind+"/"+boundary.Name+"/start")
				continue
			}
			baseStage, baseErr := recognizeTransitionPrefix(s, d, external, refs)
			_, busyErr := transitionCommandRouter(s, d, external, refs, "recover-resume")
			killed := child.releaseAndWaitForKill()
			writerAfter, statErr := os.Lstat(filepath.Join(s, "writer.lock"))
			authority := &transitionRecoveryAuthority{Boundary: boundary, Identity: child.identity, ExpectResidue: true}
			stage, prefixErr := recognizeTransitionPrefixInjected(s, d, external, refs, authority)
			inventoryRefs := make(map[string]string)
			doctorBefore, _, beforeErr := protectedInventory(map[string]string{"state": s, "data": d, "external": external}, inventoryRefs)
			doctorStage, doctorErr := transitionCommandRouterInjected(s, d, external, refs, "doctor", authority)
			doctorAfter, _, afterErr := protectedInventory(map[string]string{"state": s, "data": d, "external": external}, inventoryRefs)
			durableStage := durableStageForBoundary(boundary, refs)
			recovered, recoveryErr := transitionCommandRouterInjected(s, d, external, refs, recoveryCommandForStage(durableStage), authority)
			if baseErr == nil && baseStage == "transition-base" && busyErr != nil &&
				(errors.Is(busyErr, syscall.EAGAIN) || errors.Is(busyErr, syscall.EACCES)) &&
				killed && statErr == nil && os.SameFile(writerBefore, writerAfter) && prefixErr == nil && stage == durableStage &&
				beforeErr == nil && afterErr == nil && inventoriesEqual(doctorBefore, doctorAfter) &&
				doctorErr == nil && doctorStage == durableStage &&
				transitionMatrixRecoveryExpected(s, d, external, refs, transitionBoundaryLeavesResidue(boundary), durableStage, recovered, recoveryErr) {
				verified++
			} else {
				failures = append(failures, "hard/"+kind+"/"+boundary.Name)
			}
		}
	}
	return verified, failures
}

func runCatchableTransitionMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, cancellation bool) (int, []string) {
	if cfg.SelfExecutable == "" {
		return 0, nil
	}
	verified := 0
	var failures []string
	caseIndex := 0
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		template := transitionVariantRefs(kind)
		for _, boundary := range transitionBoundaries(template) {
			s, d, external, refs, err := prepareTransitionVariant(root.Path, kind)
			if err != nil {
				failures = append(failures, kind+"/"+boundary.Name+"/prepare")
				continue
			}
			spec := workerSpecFor(s, d, "", external, boundary.Name, refs)
			specPath := filepath.Join(filepath.Dir(s), "worker-spec.json")
			if writeDeterministicWorkerSpec(specPath, spec) != nil {
				failures = append(failures, kind+"/"+boundary.Name+"/spec")
				continue
			}
			mode := "transition-fail"
			if cancellation {
				mode = "transition-cancel"
			}
			child, err := startGatedChild(ctx, cfg.SelfExecutable, mode, specPath, root)
			if err != nil {
				failures = append(failures, kind+"/"+boundary.Name+"/start")
				continue
			}
			_, busyErr := transitionCommandRouter(s, d, external, refs, "recover-resume")
			exited := child.releaseAndWaitForFailure()
			authority := &transitionRecoveryAuthority{Boundary: boundary, Identity: child.identity}
			stage, prefixErr := recognizeTransitionPrefixInjected(s, d, external, refs, authority)
			inventoryRefs := make(map[string]string)
			doctorBefore, _, beforeErr := protectedInventory(map[string]string{"state": s, "data": d, "external": external}, inventoryRefs)
			doctorStage, doctorErr := transitionCommandRouterInjected(s, d, external, refs, "doctor", authority)
			doctorAfter, _, afterErr := protectedInventory(map[string]string{"state": s, "data": d, "external": external}, inventoryRefs)
			command := "recover-resume"
			durableStage := durableStageForBoundary(boundary, refs)
			if caseIndex%2 == 1 || recoveryCommandForStage(durableStage) == "recover-repair" {
				command = "recover-repair"
			}
			caseIndex++
			recovered, recoveryErr := transitionCommandRouterInjected(s, d, external, refs, command, authority)
			if busyErr != nil && (errors.Is(busyErr, syscall.EAGAIN) || errors.Is(busyErr, syscall.EACCES)) &&
				exited && prefixErr == nil && stage == durableStage && beforeErr == nil && afterErr == nil &&
				doctorErr == nil && doctorStage == durableStage && inventoriesEqual(doctorBefore, doctorAfter) &&
				transitionMatrixRecoveryExpected(s, d, external, refs, false, durableStage, recovered, recoveryErr) {
				verified++
			} else {
				class := "failure"
				if cancellation {
					class = "cancellation"
				}
				failures = append(failures, class+"/"+kind+"/"+boundary.Name)
			}
		}
	}
	return verified, failures
}

func recoveryActionAuthority(refs graphRefs, boundaryName, mode string, childIdentity processIdentity, hasResidue bool, residueIdentity *processIdentity) *transitionRecoveryAuthority {
	var authority *transitionRecoveryAuthority
	if mode == "transition-recover-die" {
		if boundary, ok := transitionBoundaryByName(refs, boundaryName); ok &&
			transitionBoundaryLeavesResidue(boundary) {
			authority = &transitionRecoveryAuthority{
				Boundary: boundary, Identity: childIdentity, ExpectResidue: true,
			}
		}
	}
	parts := strings.SplitN(boundaryName, "/", 2)
	if mode == "transition-recover-die" && len(parts) == 2 &&
		(parts[0] == "recovery-journal-transfer" || parts[0] == "recovery-writer-owner-transfer" ||
			parts[0] == "recovery-uncommitted-owner-release") &&
		(parts[1] == string(prooffs.AfterTemp) || parts[1] == string(prooffs.AfterWrite) ||
			parts[1] == string(prooffs.AfterFileSync) || parts[1] == string(prooffs.BeforeRename)) {
		authority = &transitionRecoveryAuthority{
			Boundary: transitionBoundary{Point: parts[1]},
			Identity: childIdentity, ExpectResidue: true, RecoveryStage: parts[0],
		}
	}
	if hasResidue && strings.HasSuffix(boundaryName, "/before-unlink") && residueIdentity != nil {
		authority = &transitionRecoveryAuthority{
			Boundary: transitionBoundary{Point: string(prooffs.AfterTemp)},
			Identity: *residueIdentity, ExpectResidue: true, RecoveryStage: "recovery-journal-transfer",
		}
	}
	return authority
}

func runRecoveryActionMatrix(
	ctx context.Context,
	cfg Config,
	root *prooffs.OwnedRoot,
	mode string,
) (int, int, []string) {
	if cfg.SelfExecutable == "" {
		return 0, 0, nil
	}
	verified := 0
	total := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		for _, testCase := range recoveryActionCases(transitionVariantRefs(kind)) {
			total++
			s, d, external, refs, err := prepareTransitionRecoveryFixture(root.Path, kind, testCase.start)
			if err != nil {
				failures = append(failures, mode+"/"+kind+"/"+testCase.start+"/"+testCase.boundary+"/prepare")
				continue
			}
			var residueIdentity *processIdentity
			if testCase.residue {
				residueSpec := workerSpecFor(s, d, "", external, "", refs)
				residueSpec.RecoveryBoundary = "recovery-journal-transfer/" + string(prooffs.AfterTemp)
				residueSpecPath := filepath.Join(filepath.Dir(s), "residue-worker-spec.json")
				if writeDeterministicWorkerSpec(residueSpecPath, residueSpec) != nil {
					failures = append(failures, mode+"/"+kind+"/"+testCase.boundary+"/residue-spec")
					continue
				}
				residueChild, startErr := startGatedChild(ctx, cfg.SelfExecutable, "transition-recover-die", residueSpecPath, root)
				if startErr != nil || !residueChild.releaseAndWaitForKill() {
					failures = append(failures, mode+"/"+kind+"/"+testCase.boundary+"/residue-worker")
					continue
				}
				identity := residueChild.identity
				residueIdentity = &identity
			}
			spec := workerSpecFor(s, d, "", external, "", refs)
			spec.RecoveryBoundary = testCase.boundary
			spec.ResidueIdentity = residueIdentity
			specPath := filepath.Join(filepath.Dir(s), "worker-spec.json")
			if writeDeterministicWorkerSpec(specPath, spec) != nil {
				failures = append(failures, mode+"/"+kind+"/"+testCase.start+"/"+testCase.boundary+"/spec")
				continue
			}
			child, err := startGatedChild(ctx, cfg.SelfExecutable, mode, specPath, root)
			if err != nil {
				failures = append(failures, mode+"/"+kind+"/"+testCase.start+"/"+testCase.boundary+"/start")
				continue
			}
			var exited bool
			if mode == "transition-recover-die" {
				exited = child.releaseAndWaitForKill()
			} else {
				exited = child.releaseAndWaitForFailure()
			}
			authority := recoveryActionAuthority(refs, testCase.boundary, mode, child.identity, testCase.residue, residueIdentity)
			prefix, prefixErr := recognizeRecoverableTransitionPrefix(s, d, external, refs, authority)
			recovered, recoveryErr := transitionCommandRouterInjected(s, d, external, refs, recoveryCommandForStage(prefix.stage), authority)
			success := exited && transitionTempResidueCount(s, external) == 0
			if testCase.wantPrior {
				wantRecovered := "STABLE(A)"
				currentPreserved := exactRecordBytes(
					filepath.Join(s, "current.json"),
					baselineCurrent(refs),
				)
				if refs.a == "" {
					wantRecovered = "READY(∅)"
					currentPreserved = !exists(filepath.Join(s, "current.json"))
				}
				priorExact := classify(s, d) == wantRecovered && currentPreserved &&
					!exists(filepath.Join(s, "journals", refs.journal+".json")) &&
					!exists(filepath.Join(external, "effect.json")) &&
					exactRecordBytes(filepath.Join(s, "writer-owner.json"), baselineReleasedWriter(refs))
				recoveredPrior := prefixErr == nil && prefix.stage != "" &&
					recoveryErr == nil && recovered == wantRecovered
				alreadyPrior := prefixErr != nil && recoveryErr != nil && recovered == ""
				success = success && priorExact && (recoveredPrior || alreadyPrior)
			} else {
				success = success && prefixErr == nil && prefix.stage != "" &&
					transitionMatrixRecoveryExpected(s, d, external, refs, prefix.residue != nil, prefix.stage, recovered, recoveryErr)
			}
			if success {
				verified++
			} else {
				failures = append(failures, mode+"/"+kind+"/"+testCase.start+"/"+testCase.boundary)
			}
		}
	}
	return verified, total, failures
}
func recoveryInputBytes(class string, r graphRefs) []byte {
	return []byte("zerb-proof-recovery-input-v1:" + class + ":" + r.operationToken)
}

func prepareRecoveryInputs(dir string, r graphRefs) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	for _, class := range []string{"source-object", "candidate-policy", "tool-readiness", "destination-precondition"} {
		path := filepath.Join(dir, class)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(recoveryInputBytes(class, r))
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return syncDir(dir)
}

func recoveryInputBoundary(class string) (string, string) {
	switch class {
	case "source-object", "candidate-policy":
		return "journal-mutating-tool-install-publication/authority-pre/before-revalidation", "journal-prepared-publication"
	case "tool-readiness":
		return "external-tool-install/authority-pre/before-revalidation", "journal-mutating-tool-install-publication"
	default:
		return "external-home-apply/authority-pre/before-revalidation", "journal-mutating-home-apply-publication"
	}
}

func runRecoveryUnavailableMatrix(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (int, int, []string) {
	if cfg.SelfExecutable == "" {
		return 0, 0, nil
	}
	classes := []string{"source-object", "candidate-policy", "tool-readiness", "destination-precondition"}
	verified := 0
	total := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		for _, unavailable := range classes {
			total++
			s, d, external, refs, err := prepareTransitionRecoveryFixture(root.Path, kind, "journal-prepared-publication")
			if err != nil {
				failures = append(failures, "unavailable/"+kind+"/"+unavailable+"/prepare")
				continue
			}
			inputRoot := filepath.Join(filepath.Dir(s), "recovery-inputs")
			if err = prepareRecoveryInputs(inputRoot, refs); err != nil {
				failures = append(failures, "unavailable/"+kind+"/"+unavailable+"/inputs")
				continue
			}
			inputPath := filepath.Join(inputRoot, unavailable)
			if unavailable == "source-object" || unavailable == "tool-readiness" {
				err = os.Remove(inputPath)
				if err == nil {
					err = syncDir(inputRoot)
				}
			} else {
				err = durableRecordSubstitution(inputPath)
			}
			if err != nil {
				failures = append(failures, "unavailable/"+kind+"/"+unavailable+"/obstruct")
				continue
			}
			boundary, wantStage := recoveryInputBoundary(unavailable)
			spec := workerSpecFor(s, d, "", external, "", refs)
			spec.RecoveryBoundary = boundary
			spec.InputRoot = inputRoot
			spec.UnavailableInput = unavailable
			specPath := filepath.Join(filepath.Dir(s), "worker-spec.json")
			if writeDeterministicWorkerSpec(specPath, spec) != nil {
				failures = append(failures, "unavailable/"+kind+"/"+unavailable+"/spec")
				continue
			}
			child, err := startGatedChild(ctx, cfg.SelfExecutable, "transition-recover-fail", specPath, root)
			if err != nil {
				failures = append(failures, "unavailable/"+kind+"/"+unavailable+"/start")
				continue
			}
			exited := child.releaseAndWaitForFailure()
			prefix, prefixErr := recognizeRecoverableTransitionPrefix(s, d, external, refs, nil)
			classification := classify(s, d)
			wantClassification := "TRANSITION(A,B)"
			currentUnchanged := exactRecordBytes(
				filepath.Join(s, "current.json"),
				baselineCurrent(refs),
			)
			if refs.a == "" {
				wantClassification = "TRANSITION(∅,B)"
				currentUnchanged = !exists(filepath.Join(s, "current.json"))
			}
			effectUnchanged := !exists(filepath.Join(external, "effect.json"))
			if unavailable == "destination-precondition" {
				effectUnchanged = exactRecordBytes(
					filepath.Join(external, "effect.json"),
					appliedObservation(refs.instance, refs.b, refs.journal, mutationPhases[:1]),
				)
			}
			if exited && prefixErr == nil && prefix.stage == wantStage &&
				classification == wantClassification && currentUnchanged && effectUnchanged {
				verified++
			} else {
				failures = append(failures, fmt.Sprintf(
					"unavailable/%s/%s/exited=%t/prefix-error=%t/stage=%s/want-stage=%s/class=%s/want-class=%s/current=%t/effect=%t",
					kind, unavailable, exited, prefixErr != nil, prefix.stage, wantStage,
					classification, wantClassification, currentUnchanged, effectUnchanged,
				))
			}
		}
	}
	return verified, total, failures
}

func transitionAuthorityExpectedStage(boundary transitionBoundary, r graphRefs) string {
	if strings.HasPrefix(boundary.Point, "authority-pre/") {
		return previousTransitionStage(boundary.Stage, r)
	}
	return boundary.Stage
}

func transitionAuthorityObjects(boundary transitionBoundary, r graphRefs) []string {
	expected := expectedTransitionObjects(transitionAuthorityExpectedStage(boundary, r), r, processIdentity{PID: 1, Start: 1, PIDNamespace: 1})
	var objects []string
	for key := range expected {
		if key == "state:admission.lock" || key == "state:writer.lock" || key == "state:writer-owner.json" ||
			key == "state:current.json" || strings.HasPrefix(key, "state:candidates/") ||
			strings.HasPrefix(key, "state:receipts/") || strings.HasPrefix(key, "state:journals/") {
			objects = append(objects, key)
		}
	}
	sort.Strings(objects)
	return objects
}
func transitionAuthoritySubstitutionCount(r graphRefs) int {
	total := 0
	for _, boundary := range transitionBoundaries(r) {
		if boundary.Kind == "authority" {
			objects := transitionAuthorityObjects(boundary, r)
			total += len(objects)
			for _, object := range objects {
				if object == "state:writer-owner.json" {
					total++
				}
			}
		}
	}
	return total
}

func fixtureObjectsEqualExcept(before, after map[string]fixtureObject, except string) bool {
	if len(before) != len(after) {
		return false
	}
	for key, value := range before {
		other, ok := after[key]
		if !ok {
			return false
		}
		sameIdentity := value.Device == 0 || other.Device == 0 ||
			(value.Device == other.Device && value.Inode == other.Inode && value.Links == other.Links)
		if key != except && (value.Kind != other.Kind || value.Mode != other.Mode || value.Owner != other.Owner ||
			!sameIdentity || !bytes.Equal(value.Bytes, other.Bytes)) {
			return false
		}
	}
	return true
}

func durableRecordSubstitution(path string) error {
	if err := os.WriteFile(path, []byte("proof-authority-substitution"), 0600); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func durableRecordIdentitySubstitution(path string, replacement []byte) error {
	content, err := readNoFollowBounded(path, maxRecognitionFile)
	if err != nil {
		return err
	}
	if replacement != nil {
		content = replacement
	}
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	var published prooffs.PublicationIdentity
	if err = prooffs.AtomicPublishAt(int(directory.Fd()), filepath.Base(path), content, prooffs.PublishOptions{
		Capture: func(identity prooffs.PublicationIdentity) { published = identity },
	}); err != nil {
		return err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := after.Sys().(*syscall.Stat_t)
	if !ok || os.SameFile(before, after) ||
		published != (prooffs.PublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino}) {
		return errors.New("record identity substitution was not exact")
	}
	return nil
}

func runAuthoritySubstitutionMatrix(root *prooffs.OwnedRoot) (int, int, []string) {
	verified := 0
	total := 0
	var failures []string
	for _, kind := range []string{"replacing", "bootstrap", "exact-reapply", "multi-generation-exact-reapply"} {
		template := transitionVariantRefs(kind)
		for _, boundary := range transitionBoundaries(template) {
			if boundary.Kind != "authority" {
				continue
			}
			for objectIndex, templateObject := range transitionAuthorityObjects(boundary, template) {
				alteredVariants := []bool{false}
				if templateObject == "state:writer-owner.json" {
					alteredVariants = append(alteredVariants, true)
				}
				for _, ownerAltered := range alteredVariants {
					total++
					variant := "replacement"
					if templateObject == "state:writer-owner.json" {
						variant = "identical"
						if ownerAltered {
							variant = "altered"
						}
					}
					s, d, external, refs, err := prepareTransitionVariant(root.Path, kind)
					if err != nil {
						failures = append(failures, fmt.Sprintf("substitution/%s/%s/object-%d/%s/prepare", kind, boundary.Name, objectIndex, variant))
						continue
					}
					objects := transitionAuthorityObjects(boundary, refs)
					if objectIndex >= len(objects) {
						failures = append(failures, "substitution/"+kind+"/"+boundary.Name+"/object-index/"+variant)
						continue
					}
					object := objects[objectIndex]
					identity, identityErr := processIdentityForPID(os.Getpid())
					if identityErr != nil {
						failures = append(failures, "substitution/"+kind+"/"+boundary.Name+"/"+object+"/"+variant+"/identity")
						continue
					}
					var before map[string]fixtureObject
					var beforeTarget os.FileInfo
					substituted := false
					runErr := executeTransition(s, d, external, refs, identity, nil, func(name string) error {
						if name != boundary.Name {
							return nil
						}
						expectedStage := transitionAuthorityExpectedStage(boundary, refs)
						var inventoryErr error
						before, inventoryErr = actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
						if inventoryErr != nil || !fixtureObjectsEqual(before, expectedTransitionObjects(expectedStage, refs, identity)) {
							return errors.New("substitution fixture was not the exact authority prefix")
						}
						rel := strings.TrimPrefix(object, "state:")
						path := filepath.Join(s, filepath.FromSlash(rel))
						if object == "state:admission.lock" || object == "state:writer.lock" {
							beforeTarget, inventoryErr = os.Lstat(path)
							if inventoryErr != nil {
								return inventoryErr
							}
							if inventoryErr = os.Remove(path); inventoryErr != nil {
								return inventoryErr
							}
							if inventoryErr = createDurableEmpty(s, filepath.Base(path)); inventoryErr != nil {
								return inventoryErr
							}
							afterTarget, statErr := os.Lstat(path)
							substituted = statErr == nil && !os.SameFile(beforeTarget, afterTarget)
							return statErr
						}
						if object == "state:writer-owner.json" {
							var replacement []byte
							if ownerAltered {
								var owner writerOwnerRecord
								if inventoryErr = readRecord(path, &owner); inventoryErr != nil {
									return inventoryErr
								}
								owner.Instance = strings.Repeat("a", 32)
								if refs.instance == owner.Instance {
									owner.Instance = strings.Repeat("b", 32)
								}
								replacement = recordBytes(owner)
							}
							inventoryErr = durableRecordIdentitySubstitution(path, replacement)
						} else {
							inventoryErr = durableRecordSubstitution(path)
						}
						if inventoryErr != nil {
							return inventoryErr
						}
						substituted = true
						return nil
					})
					after, afterErr := actualFixtureObjects(map[string]string{"state": s, "data": d, "external": external})
					preserved := before != nil && afterErr == nil && fixtureObjectsEqualExcept(before, after, object)
					if object == "state:writer-owner.json" {
						bytesChanged := !bytes.Equal(before[object].Bytes, after[object].Bytes)
						preserved = preserved && bytesChanged == ownerAltered &&
							(before[object].Device != after[object].Device || before[object].Inode != after[object].Inode)
					} else if object != "state:admission.lock" && object != "state:writer.lock" {
						preserved = preserved && !bytes.Equal(before[object].Bytes, after[object].Bytes)
					}
					if runErr != nil && strings.Contains(runErr.Error(), "authority substitution") && substituted && preserved {
						verified++
					} else {
						failures = append(failures, "substitution/"+kind+"/"+boundary.Name+"/"+object+"/"+variant)
					}
				}
			}
		}
	}
	return verified, total, failures
}

func runBootstrapClassifierPrefixes(root *prooffs.OwnedRoot) (int, int, []string) {
	template := freshBootstrapRefs()
	boundaries := transitionBoundaries(template)
	verified := 0
	var failures []string
	for _, boundary := range boundaries {
		s, d, external, refs, err := prepareFirstBootstrapFixture(root.Path)
		if err != nil {
			failures = append(failures, boundary.Name+"/prepare")
			continue
		}
		identity, err := processIdentityForPID(os.Getpid())
		if err != nil {
			failures = append(failures, boundary.Name+"/identity")
			continue
		}
		injected := errors.New("first-bootstrap classification boundary")
		runErr := executeTransition(s, d, external, refs, identity, nil, func(name string) error {
			if name == boundary.Name {
				return injected
			}
			return nil
		})
		durableStage := durableStageForBoundary(boundary, refs)
		want := "READY(∅)"
		switch {
		case durableStage == "writer-owner-released-publication":
			want = "STABLE(A)"
		case transitionStageIndex(durableStage) >= transitionStageIndex("current-publication"):
			want = "CLEANUP(B)"
		case transitionStageIndex(durableStage) >= transitionStageIndex("journal-prepared-publication"):
			want = "TRANSITION(∅,B)"
		case durableStage != "transition-base":
			want = "BAD corrupt"
		}
		stage, recognitionErr := recognizeTransitionPrefix(s, d, external, refs)
		classification := classify(s, d)
		if errors.Is(runErr, injected) && classification == want && recognitionErr == nil && stage == durableStage {
			verified++
		} else {
			failures = append(failures, fmt.Sprintf(
				"%s/want=%s/got=%s/stage=%s", boundary.Name, want, classification, stage,
			))
		}
	}
	return verified, len(boundaries), failures
}

func runAllDeathBoundaries(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	hard, hardFailures := runHardDeathTransitionMatrix(ctx, cfg, root)
	failures, failureFailures := runCatchableTransitionMatrix(ctx, cfg, root, false)
	cancellations, cancellationFailures := runCatchableTransitionMatrix(ctx, cfg, root, true)
	recoveryHard, recoveryHardTotal, recoveryHardFailures := runRecoveryActionMatrix(ctx, cfg, root, "transition-recover-die")
	recoveryFailure, recoveryFailureTotal, recoveryFailureFailures := runRecoveryActionMatrix(ctx, cfg, root, "transition-recover-fail")
	recoveryCancellation, recoveryCancellationTotal, recoveryCancellationFailures := runRecoveryActionMatrix(ctx, cfg, root, "transition-recover-cancel")
	recoveryUnavailable, recoveryUnavailableTotal, recoveryUnavailableFailures := runRecoveryUnavailableMatrix(ctx, cfg, root)
	substitutions, substitutionTotal, substitutionFailures := runAuthoritySubstitutionMatrix(root)
	recoverySubstitutions, recoverySubstitutionTotal, recoverySubstitutionFailures := runRecoveryAuthoritySubstitutionMatrix(ctx, cfg, root)
	failedBoundaries := append(append(append(append(append(append(append(
		[]string(nil), hardFailures...), failureFailures...), cancellationFailures...),
		recoveryHardFailures...), recoveryFailureFailures...), recoveryCancellationFailures...), substitutionFailures...)
	failedBoundaries = append(failedBoundaries, recoverySubstitutionFailures...)
	failedBoundaries = append(failedBoundaries, recoveryUnavailableFailures...)
	classifierVerified, classifierTotal, classifierFailures := runBootstrapClassifierPrefixes(root)
	failedBoundaries = append(failedBoundaries, classifierFailures...)
	replacingTotal := len(transitionBoundaries(transitionVariantRefs("replacing")))
	bootstrapTotal := len(transitionBoundaries(transitionVariantRefs("bootstrap")))
	exactTotal := len(transitionBoundaries(transitionVariantRefs("exact-reapply")))
	multiGenerationTotal := len(transitionBoundaries(transitionVariantRefs("multi-generation-exact-reapply")))
	replacingSubstitutions := transitionAuthoritySubstitutionCount(transitionVariantRefs("replacing"))
	bootstrapSubstitutions := transitionAuthoritySubstitutionCount(transitionVariantRefs("bootstrap"))
	exactSubstitutions := transitionAuthoritySubstitutionCount(transitionVariantRefs("exact-reapply"))
	multiGenerationSubstitutions := transitionAuthoritySubstitutionCount(transitionVariantRefs("multi-generation-exact-reapply"))
	expectedSubstitutions := replacingSubstitutions + bootstrapSubstitutions + exactSubstitutions + multiGenerationSubstitutions
	expected := replacingTotal + bootstrapTotal + exactTotal + multiGenerationTotal
	status := Observed
	if cfg.SelfExecutable == "" {
		status = Inconclusive
	} else if hard != expected || failures != expected || cancellations != expected ||
		recoveryHard != recoveryHardTotal || recoveryFailure != recoveryFailureTotal ||
		recoveryCancellation != recoveryCancellationTotal ||
		recoveryHardTotal != recoveryFailureTotal || recoveryHardTotal != recoveryCancellationTotal ||
		substitutions != substitutionTotal || substitutionTotal != expectedSubstitutions ||
		recoverySubstitutions != recoverySubstitutionTotal ||
		recoveryUnavailable != recoveryUnavailableTotal || classifierVerified != classifierTotal {
		status = Failed
	}
	add(Scenario{
		ID: "VAL-031/all-hard-death-boundaries", Requirements: []string{"VAL-031", "VAL-032", "VAL-033", "VAL-034"}, Status: status,
		Fixture: "generated concrete persistence boundaries for replacing A-to-B, bootstrap, exact A-to-A, and repeated A-to-A", Lease: "A-X admission classification then same lease appends stable W-X and owner compare-transfer",
		Command:            "doctor then intent-bound recovery; journal-less prefixes use repair and completed stable prefixes deny both forms",
		Writes:             []string{"fixtures/<typed-transition-prefix>/state", "fixtures/<typed-transition-prefix>/external", "fixtures/<typed-transition-prefix>/worker-spec.json"},
		Observation:        fmt.Sprintf("verified executable hard-death=%d/%d failure=%d/%d cancellation=%d/%d authority-object-substitution=%d/%d and bootstrap classifier boundaries=%d/%d; independent generated denominators replacing=%d bootstrap=%d exact-reapply=%d repeated-exact-reapply=%d. Each fresh transition acquires A-X, classifies the exact READY/STABLE admission row, appends W-X to the same retained lease, reclassifies, and only then begins writer compare-transfer. Every durable/external stage has pre-action and post-action checkpoints with injections immediately before and after revalidation of both held lock inodes, the exact expected writer owner, and the complete current/candidate/receipt/journal protected graph; substitutions independently cover admission lock, writer lock, owner, current, each present candidate and receipt, and present journal without advancing beyond the exact checkpoint prefix. Atomic, immutable no-replace, and removal rows inject their real persistence actions; exact owned hard-death temporary residue is recognized read-only and identity-validated before narrow recovery removal", hard, expected, failures, expected, cancellations, expected, substitutions, substitutionTotal, classifierVerified, classifierTotal, replacingTotal, bootstrapTotal, exactTotal, multiGenerationTotal),
		RejectedAssumption: "logical stage labels, prose-only checkpoints, or catchable errors substitute for injected persistence actions and hard-death recovery",
		FailedBoundaries:   failedBoundaries,
	})
	add(Scenario{
		ID: "VAL-032/recovery-action-matrix", Requirements: []string{"VAL-031", "VAL-032", "VAL-033", "VAL-034"}, Status: status,
		Fixture:            "stage-indexed recovery over replacing, bootstrap, exact-reapply, and repeated exact-reapply relations",
		Lease:              "A-X exact-prefix admission then W-X appended to the same retained descriptor set before dead-predecessor compare-transfer",
		Command:            "recover-resume after injected recovery hard death, failure, and cancellation",
		Writes:             []string{"fixtures/<typed-transition-recovery>/state", "fixtures/<typed-transition-recovery>/external", "fixtures/<typed-transition-recovery>/worker-spec.json"},
		Observation:        fmt.Sprintf("independent recovery-action denominators hard-death=%d/%d failure=%d/%d cancellation=%d/%d and pre-transfer authority substitutions=%d/%d; every recovery first acquires only A-X, recognizes the exact admitted recovery relation, appends W-X to that same retained lease, and re-recognizes before any compare-transfer or cleanup. Each relation exercises every ordinary persistence action from transition-base plus owner-only and journal-ahead compare-transfer actions, reopens the exact interrupted prefix, and restarts to stable without replaying an action at or before the recognized stage. Recovery substitution rows independently replace the admission lock, writer lock, exact owner inode with identical and altered bytes, current, each present candidate/receipt, and journal before compare-transfer", recoveryHard, recoveryHardTotal, recoveryFailure, recoveryFailureTotal, recoveryCancellation, recoveryCancellationTotal, recoverySubstitutions, recoverySubstitutionTotal),
		RejectedAssumption: "initial-transition injection, its authority matrix, or a single successful recovery proves failure, cancellation, successor death, and protected-graph substitution handling inside recovery itself",
		FailedBoundaries:   append(append(append(append([]string(nil), recoveryHardFailures...), recoveryFailureFailures...), recoveryCancellationFailures...), recoverySubstitutionFailures...),
	})
	add(Scenario{
		ID: "VAL-032/recovery-unavailable-inputs", Requirements: []string{"VAL-031", "VAL-032", "VAL-033"}, Status: status,
		Fixture:            "source object, candidate/policy, tool readiness, and destination precondition unavailable during each recovery relation",
		Lease:              "A-X+W-X released only after preserving exact dirty truth",
		Command:            "recover-resume",
		Writes:             []string{"fixtures/<typed-transition-recovery>/state", "fixtures/<typed-transition-recovery>/worker-spec.json"},
		Observation:        fmt.Sprintf("verified unavailable recovery inputs=%d/%d; every injected obstruction exited its claimant process, retained the exact prepared stage at a fresh compare-transferred writer generation, preserved current A or the explicit empty baseline, performed no external effect, and did not report stable", recoveryUnavailable, recoveryUnavailableTotal),
		RejectedAssumption: "forward recovery may replay, abandon, or report stable when a required source, candidate/policy, tool, or destination input remains unavailable",
		FailedBoundaries:   recoveryUnavailableFailures,
	})
	for _, matrix := range []struct {
		id, fixture, observation string
		rows, substitutionRows   int
	}{
		{"VAL-031/matrix/replacing-A-to-B", "distinct prior candidate A/receipt RA to candidate B/receipt RB", "replacement removes distinct A, then distinct RA, then journal", replacingTotal, replacingSubstitutions},
		{"VAL-031/matrix/bootstrap", "READY empty authority to first candidate and receipt", "bootstrap independently injects every applicable concrete publication, cleanup, and authority boundary", bootstrapTotal, bootstrapSubstitutions},
		{"VAL-031/matrix/exact-A-to-A", "exact candidate A reused with a new receipt descending from current RA", "candidate no-replace attempts reopen exact immutable A; distinct RA is removed before journal", exactTotal, exactSubstitutions},
		{"VAL-031/matrix/repeated-A-to-A", "three prior exact reapplies followed by another exact A-to-A across every concrete boundary", "candidate and receipt no-replace attempts reopen exact bytes; current ancestry begins beyond bootstrap", multiGenerationTotal, multiGenerationSubstitutions},
	} {
		add(Scenario{ID: matrix.id, Requirements: []string{"VAL-031", "VAL-032", "VAL-034"}, Status: status, Fixture: matrix.fixture, Lease: "A-X+W-X", Command: "hard-death, failure, cancellation, substitution, doctor, resume/repair", Writes: []string{"fixtures/<typed-transition-prefix>/state", "fixtures/<typed-transition-prefix>/external", "fixtures/<typed-transition-prefix>/worker-spec.json"}, Observation: fmt.Sprintf("%s; generated concrete denominator=%d hard/error/cancellation boundaries (%d executable rows) and %d independently generated authority-object substitution rows", matrix.observation, matrix.rows, matrix.rows*3, matrix.substitutionRows)})
	}
}

func receiptValidationCoverage() (int, int, bool) {
	candidate := string(candidateDigestFor("proof-next"))
	previousCandidate := string(candidateDigestFor("proof-current"))
	previous := receiptDigest(previousCandidate)
	baseline := receiptFactsFor(candidate, previous)
	clone := func() receiptCompleted {
		var copied receiptCompleted
		_ = json.Unmarshal(recordBytes(baseline), &copied)
		return copied
	}
	mutations := []func(*receiptCompleted){
		func(f *receiptCompleted) { f.Schema = 2 },
		func(f *receiptCompleted) { f.Domain = "zerb.receipt" },
		func(f *receiptCompleted) { f.Candidate = previous },
		func(f *receiptCompleted) { f.Previous = "" },
		func(f *receiptCompleted) { f.Source.Commit = "" },
		func(f *receiptCompleted) { f.Source.SourceID = "sha256:" + recordDigest("wrong-source-id") },
		func(f *receiptCompleted) { f.Components[0].Path = "" },
		func(f *receiptCompleted) { f.Parser.Version = "" },
		func(f *receiptCompleted) { f.Policy.PolicyIDs[4] = "zerb.lifecycle.v4" },
		func(f *receiptCompleted) { f.Platform.Kernel = "" },
		func(f *receiptCompleted) { f.Tools[0].Installed = "" },
		func(f *receiptCompleted) { f.Targets[0] = "other" },
		func(f *receiptCompleted) { f.Preconditions[0].Kind = "directory" },
		func(f *receiptCompleted) { f.Results[0].ContentHash = "" },
		func(f *receiptCompleted) { f.Phases = f.Phases[:1] },
		func(f *receiptCompleted) { f.Dimensions.ConvergenceVerified = false },
	}
	executables := []func(*receiptCompleted) *receiptExecutable{
		func(f *receiptCompleted) *receiptExecutable { return &f.Zerb },
		func(f *receiptCompleted) *receiptExecutable { return &f.Git },
		func(f *receiptCompleted) *receiptExecutable { return &f.Mise },
		func(f *receiptCompleted) *receiptExecutable { return &f.Chezmoi },
	}
	for _, selectExecutable := range executables {
		mutations = append(mutations,
			func(f *receiptCompleted) { selectExecutable(f).ID = "" },
			func(f *receiptCompleted) { selectExecutable(f).Version = "" },
			func(f *receiptCompleted) { selectExecutable(f).Path = "relative" },
			func(f *receiptCompleted) { selectExecutable(f).SHA256 = "sha256:00" },
		)
	}
	rejected := 0
	for _, mutate := range mutations {
		facts := clone()
		mutate(&facts)
		if !validCompletedReceipt(facts, candidate, previous) {
			rejected++
		}
	}
	zerbPresent := validReceiptExecutable(baseline.Zerb, "zerb") &&
		baseline.Zerb.Version != "" && baseline.Zerb.Path != "" && baseline.Zerb.SHA256 != ""
	return rejected, len(mutations), zerbPresent && validCompletedReceipt(baseline, candidate, previous)
}

type selfExecutableReplacementResult struct {
	BoundToRunningImage bool `json:"bound_to_running_image"`
	FailedClosed        bool `json:"failed_closed"`
	Restored            bool `json:"restored"`
}

func copyProofExecutable(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("proof executable source is not a regular file")
	}
	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	if copyErr == nil {
		copyErr = dst.Sync()
	}
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return syncDir(filepath.Dir(destination))
}

func runSelfExecutableReplacementProof(ctx context.Context, cfg Config, root *prooffs.OwnedRoot) (selfExecutableReplacementResult, error) {
	var result selfExecutableReplacementResult
	if cfg.SelfExecutable == "" {
		return result, errors.New("self executable unavailable")
	}
	fixture, err := os.MkdirTemp(root.Path, "self-executable-replacement-")
	if err != nil {
		return result, err
	}
	runner := filepath.Join(fixture, "runner")
	report := filepath.Join(fixture, "result.json")
	if err = copyProofExecutable(cfg.SelfExecutable, runner); err != nil {
		return result, err
	}
	cmd := exec.CommandContext(ctx, runner, "--worker", "self-executable-replacement", "--worker-path", report)
	cmd.Env = []string{"PATH=/nonexistent", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
	if err = cmd.Run(); err != nil {
		return result, err
	}
	content, err := os.ReadFile(report)
	if err != nil || len(content) > 4096 {
		return result, errors.New("invalid self executable replacement report")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return result, errors.New("invalid self executable replacement report")
	}
	if (!result.BoundToRunningImage && !result.FailedClosed) || !result.Restored {
		return result, errors.New("self executable replacement was not bound or fail-closed and restored")
	}
	return result, nil
}

func runSelfExecutableReplacementWorker(report string) (retErr error) {
	before, err := liveSelfReceiptExecutable()
	if err != nil {
		return err
	}
	original := before.Path
	if filepath.Dir(report) != filepath.Dir(original) {
		return errors.New("self executable replacement report must share the disposable executable directory")
	}
	backup := original + ".running"
	replacement := original + ".replacement"
	if err = os.Rename(original, backup); err != nil {
		return err
	}
	moved := true
	restore := func() error {
		if !moved {
			return nil
		}
		if removeErr := os.Remove(original); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		if renameErr := os.Rename(backup, original); renameErr != nil {
			return renameErr
		}
		moved = false
		return syncDir(filepath.Dir(original))
	}
	defer func() {
		if restoreErr := restore(); retErr == nil && restoreErr != nil {
			retErr = restoreErr
		}
	}()
	file, err := os.OpenFile(replacement, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, err = file.WriteString("#!/bin/sh\nexit 0\n")
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(replacement, original); err != nil {
		return err
	}
	if err = syncDir(filepath.Dir(original)); err != nil {
		return err
	}
	after, afterErr := liveSelfReceiptExecutable()
	result := selfExecutableReplacementResult{
		BoundToRunningImage: afterErr == nil && after.ID == before.ID && after.Version == before.Version && after.SHA256 == before.SHA256,
		FailedClosed:        afterErr != nil,
	}
	if err = restore(); err != nil {
		return err
	}
	restored, err := liveSelfReceiptExecutable()
	if err != nil {
		return err
	}
	result.Restored = restored.ID == before.ID && restored.Version == before.Version && restored.Path == original && restored.SHA256 == before.SHA256
	content := recordBytes(result)
	if err = os.WriteFile(report, content, 0600); err != nil {
		return err
	}
	reportFile, err := os.OpenFile(report, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	err = reportFile.Sync()
	closeErr = reportFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return syncDir(filepath.Dir(report))
}

func runReceiptAndInventoryProof(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	rejectedReceiptMutations, receiptMutationTotal, zerbPresent := receiptValidationCoverage()
	receiptValidationStatus := Observed
	if rejectedReceiptMutations != receiptMutationTotal || !zerbPresent {
		receiptValidationStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/completed-receipt-semantics", Requirements: []string{"VAL-034", "ARCH-024", "ARCH-026"}, Status: receiptValidationStatus,
		Fixture: "explicit ZERB runtime identity plus independently mutated completed-receipt fields", Writes: []string{},
		Observation:        fmt.Sprintf("ZERB live identity valid=%t; PC-022 source identity equals shared known vector %s; rejected semantic mutations=%d/%d across completed envelope/cross-references, source/components/parser/policy/platform/tools/targets/observations/phases/dimensions, and id/version/path/hash for ZERB, Git, mise, and chezmoi", zerbPresent, sourceid.KnownVectorDigest, rejectedReceiptMutations, receiptMutationTotal),
		RejectedAssumption: "receipt digest self-consistency or the managed candidate executable array supplies the runtime ZERB identity",
	})
	liveSelf, liveSelfErr := liveSelfReceiptExecutable()
	liveCandidate := string(candidateDigestFor("proof-next"))
	livePrevious := receiptDigest(string(candidateDigestFor("proof-current")))
	freshReceipt := pendingReceiptFor(liveCandidate, livePrevious)
	recoveryReceipt := pendingReceiptFor(liveCandidate, livePrevious)
	liveCurrent := stableCurrent("", liveCandidate, string(freshReceipt.Digest), "")
	liveBound := liveSelfErr == nil && freshReceipt.Facts.Zerb == liveSelf &&
		recoveryReceipt.Facts.Zerb == liveSelf && bytes.Equal(recordBytes(freshReceipt), recordBytes(recoveryReceipt)) &&
		validCompletedReceipt(freshReceipt.Facts, liveCandidate, livePrevious) &&
		validCompletedReceipt(recoveryReceipt.Facts, liveCandidate, livePrevious) &&
		liveCurrent.Candidate == receiptDigest(liveCandidate) && liveCurrent.Receipt == freshReceipt.Digest
	liveStatus := Observed
	if !liveBound {
		liveStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/live-zerb-receipt-identity", Requirements: []string{"ARCH-026", "ARCH-030", "VAL-034"}, Status: liveStatus,
		Fixture: "fresh running-image descriptor identity and independently rebuilt recovery receipt", Writes: []string{},
		Observation:        fmt.Sprintf("running image opened through /proc/self/exe was regular executable topology; its observed canonical path was opened no-follow before and after the full descriptor SHA-256 and remained the same inode=%t; fresh and recovery Completed.Zerb facts were identical=%t and both were bound through the same candidate/receipt/current digest relation=%t; fixed receipt version=%s and independent platform expectations were preserved", liveSelfErr == nil, freshReceipt.Facts.Zerb == recoveryReceipt.Facts.Zerb, liveBound, resultVersion),
		RejectedAssumption: "the launch pathname, a synthetic frozen manifest hash, or a candidate executable entry identifies the running ZERB binary",
	})
	replacementResult, replacementErr := runSelfExecutableReplacementProof(ctx, cfg, root)
	replacementStatus := Observed
	if replacementErr != nil {
		replacementStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/live-zerb-launch-path-replacement", Requirements: []string{"ARCH-026", "ARCH-030", "VAL-034"}, Status: replacementStatus,
		Fixture: "copied proof executable running from a disposable directory while its launch pathname is atomically replaced", Writes: []string{"fixtures/<self-executable-replacement>/runner", "fixtures/<self-executable-replacement>/result.json"},
		Observation:        fmt.Sprintf("after launch-path rename and distinct replacement, runtime identity stayed bound to the running image=%t or failed closed=%t; exact original executable identity was restored before worker exit=%t", replacementResult.BoundToRunningImage, replacementResult.FailedClosed, replacementResult.Restored),
		RejectedAssumption: "rehashing the launch pathname after process start proves the running executable identity",
	})
	s, d, external, refs, err := prepareTransitionFixture(root.Path)
	if err != nil {
		for _, id := range []string{"VAL-034/receipt-current-authority", "VAL-034/completion-verification", "VAL-034/hash-inventory"} {
			add(Scenario{ID: id, Requirements: []string{"VAL-034"}, Status: Failed, Fixture: "disposable transition preparation", Writes: []string{}, Observation: "failed to prepare the disposable protected transition fixture"})
		}
		return
	}
	pendingWithOldCurrent, currentAuthority := false, false
	if err == nil {
		identity, identityErr := processIdentityForPID(os.Getpid())
		if identityErr == nil {
			err = executeTransition(s, d, external, refs, identity, nil, func(boundary string) error {
				switch boundary {
				case "pending-receipt-publication/" + string(prooffs.AfterReopen):
					pendingWithOldCurrent = verifyPendingCurrentInterval(s, refs)
				case "current-publication/" + string(prooffs.AfterReopen):
					currentAuthority = verifyCompletionPublication(s, refs, true, false)
				}
				return nil
			})
		} else {
			err = identityErr
		}
	}
	completionValid := err == nil && classify(s, d) == "STABLE(A)" && verifyCompletionPublication(s, refs, true, true)
	atomicStatus := Observed
	if !pendingWithOldCurrent || !currentAuthority || !completionValid {
		atomicStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/receipt-current-authority", Requirements: []string{"VAL-034"}, Status: atomicStatus,
		Fixture: "reopened pending receipt and sole completion-authority current publication intervals", Classification: classify(s, d), Lease: "A-X+W-X",
		Writes:      []string{"fixtures/<typed-receipt-interval>/state"},
		Observation: fmt.Sprintf("pending-receipt-with-old-current=%t current-stable-authority=%t; no completed receipt status or publication exists, and each interval was read from separately durable exact files before forward cleanup", pendingWithOldCurrent, currentAuthority),
	})
	completionStatus := Observed
	if !completionValid {
		completionStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/completion-verification", Requirements: []string{"VAL-034"}, Status: completionStatus,
		Fixture: "candidate/pending-receipt/current graph", Classification: classify(s, d), Writes: []string{},
		Observation:        "strict decoders reopened the exact root/candidate/journal/pending-receipt/current graph; recomputed candidate and receipt.Completed digests matched filenames and current references; current stable(B) alone authorized completion",
		RejectedAssumption: "receipt existence or a completed-receipt status is completion authority",
	})
	cache := filepath.Join(filepath.Dir(s), "protected-cache")
	if err == nil {
		err = os.Mkdir(cache, 0700)
	}
	if err == nil {
		err = publishRecord(cache, "owner.json", namespaceOwner(refs.instance, "cache"))
	}
	inventoryRefs := make(map[string]string)
	before, beforeFiles, beforeErr := protectedInventory(map[string]string{"state": s, "data": d, "cache": cache, "external": external}, inventoryRefs)
	readOnlyValid := completionValid && verifyCompletionPublication(s, refs, true, true)
	after, afterFiles, afterErr := protectedInventory(map[string]string{"state": s, "data": d, "cache": cache, "external": external}, inventoryRefs)
	inventoryStatus := Observed
	if err != nil || beforeErr != nil || afterErr != nil || !readOnlyValid || beforeFiles != afterFiles || !inventoriesEqual(before, after) {
		inventoryStatus = Failed
	}
	add(Scenario{
		ID: "VAL-034/hash-inventory", Requirements: []string{"VAL-034"}, Status: inventoryStatus,
		Fixture: "complete disposable protected state/data/cache/external graph", Writes: []string{},
		Observation: fmt.Sprintf("typed objects=%d file-byte-hashes=%d; every protected fixture object was reopened before and after strict current-authority verification and type, mode, owner, size, typed reference, and SHA-256 equality held; no raw path or digest is retained", len(before), beforeFiles),
	})
}

type contenderResult struct {
	outcome        string
	classification string
	lease          *proofLease
}

func mutationContender(s, d string, ctx context.Context, hold bool) contenderResult {
	lease, err := acquireStableLease(s,
		struct {
			name string
			typ  int16
		}{"admission.lock", fWRLCK},
		struct {
			name string
			typ  int16
		}{"writer.lock", fWRLCK},
	)
	if err != nil {
		return contenderResult{outcome: "busy"}
	}
	classification := classify(s, d)
	if ctx.Err() != nil {
		lease.release()
		return contenderResult{outcome: "canceled", classification: classification}
	}
	if hold {
		return contenderResult{outcome: "acquired", classification: classification, lease: lease}
	}
	lease.release()
	return contenderResult{outcome: "classified", classification: classification}
}

func runConcurrencyCancellation(root *prooffs.OwnedRoot, add func(Scenario)) {
	s, d, _ := makeGraph(root.Path, "STABLE(A)")
	refs := make(map[string]string)
	before, _, beforeErr := protectedInventory(map[string]string{"state": s, "data": d}, refs)
	holder, holderErr := acquireStableLease(s,
		struct {
			name string
			typ  int16
		}{"admission.lock", fWRLCK},
		struct {
			name string
			typ  int16
		}{"writer.lock", fWRLCK},
	)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	start := make(chan struct{})
	results := make(chan contenderResult, 2)
	var waiting sync.WaitGroup
	waiting.Add(2)
	go func() {
		defer waiting.Done()
		<-start
		results <- mutationContender(s, d, canceledCtx, false)
	}()
	go func() {
		defer waiting.Done()
		<-start
		results <- mutationContender(s, d, context.Background(), false)
	}()
	close(start)
	firstBusy, secondBusy := <-results, <-results
	waiting.Wait()
	if holder != nil {
		holder.release()
	}
	raceStart := make(chan struct{})
	raceResults := make(chan contenderResult, 2)
	var racers sync.WaitGroup
	racers.Add(2)
	for range 2 {
		go func() {
			defer racers.Done()
			<-raceStart
			raceResults <- mutationContender(s, d, context.Background(), true)
		}()
	}
	close(raceStart)
	raceOne, raceTwo := <-raceResults, <-raceResults
	winners := 0
	for _, result := range []contenderResult{raceOne, raceTwo} {
		if result.outcome == "acquired" {
			winners++
			result.lease.release()
		}
	}
	racers.Wait()
	laterCanceled := mutationContender(s, d, canceledCtx, false)
	later := mutationContender(s, d, context.Background(), false)
	after, _, afterErr := protectedInventory(map[string]string{"state": s, "data": d}, refs)
	status := Observed
	if holderErr != nil || beforeErr != nil || afterErr != nil ||
		firstBusy.outcome != "busy" || secondBusy.outcome != "busy" || winners != 1 ||
		laterCanceled.outcome != "canceled" || laterCanceled.classification != "STABLE(A)" ||
		later.outcome != "classified" || later.classification != "STABLE(A)" || !inventoriesEqual(before, after) {
		status = Failed
	}
	add(Scenario{
		ID: "VAL-031/concurrency-cancel", Requirements: []string{"VAL-030", "VAL-031", "VAL-033"}, Status: status,
		Fixture: "two mutation contenders, held incompatible lease, and canceled context", Classification: later.classification,
		Admission: "busy before semantics, then canceled/classified", Lease: "A-X+W-X", Writes: []string{},
		Observation:        fmt.Sprintf("held-boundary busy losers=2; simultaneous post-release winners=%d; canceled contender later acquired and classified before honoring cancellation; following contender reclassified stable; protected inventory unchanged", winners),
		RejectedAssumption: "scheduler overlap without a held OFD lease is not contention evidence",
	})
}

func runDeterministicStateGaps(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	runFullInitializationPublication(ctx, cfg, root, add)
	runAllDeathBoundaries(ctx, cfg, root, add)
	runReceiptAndInventoryProof(ctx, cfg, root, add)
	runConcurrencyCancellation(root, add)
}

func runChildCrashes(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	if cfg.SelfExecutable == "" {
		add(Scenario{ID: "VAL-025/child-hard-death", Requirements: []string{"VAL-025", "VAL-030", "VAL-031"}, Status: Inconclusive, Fixture: "child worker", Writes: []string{}, Observation: "SelfExecutable not supplied; child mode not run"})
		return
	}
	lock := filepath.Join(root.Path, "child.lock")
	_ = os.WriteFile(lock, nil, 0600)
	cmd := exec.CommandContext(ctx, cfg.SelfExecutable, "--worker", "lock-die", "--worker-path", lock)
	cmd.Env = []string{"PATH=/nonexistent", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
	e := cmd.Run()
	f, _ := os.OpenFile(lock, os.O_RDWR, 0)
	acquired := ofd(int(f.Fd()), fWRLCK) == nil
	_ = ofd(int(f.Fd()), fUNLCK)
	f.Close()
	st := Observed
	if e == nil || !acquired {
		st = Failed
	}
	add(Scenario{ID: "VAL-025/child-hard-death", Requirements: []string{"VAL-025", "VAL-030", "VAL-031"}, Status: st, Fixture: "lock-die", Writes: []string{"child.lock"}, Observation: "child hard-exited holding OFD lock; independent OFD acquired after death"})
}

type namespaceFixtureSpec struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Foreign string `json:"foreign"`
	Report  string `json:"report"`
}

type namespaceFixtureReport struct {
	BindAlias    bool `json:"bind_alias"`
	ForeignOwner bool `json:"foreign_owner"`
}

func runNamespaceFixtures(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) {
	status := Observed
	observation := "isolated user/mount namespace created a real bind alias and a real foreign-UID object; identity/owner admission rejected both"
	base := filepath.Join(root.Path, "namespace-fixtures")
	source, target := filepath.Join(base, "source"), filepath.Join(base, "target")
	foreign, report, specPath := filepath.Join(base, "foreign"), filepath.Join(base, "report.json"), filepath.Join(base, "spec.json")
	for _, path := range []string{base, source, target} {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			status = Failed
		}
	}
	spec := namespaceFixtureSpec{Source: source, Target: target, Foreign: foreign, Report: report}
	specBytes, err := json.Marshal(spec)
	if err != nil || os.WriteFile(specPath, specBytes, 0600) != nil || os.WriteFile(report, nil, 0600) != nil {
		status = Failed
	}
	unshare, err := filepath.EvalSymlinks("/usr/bin/unshare")
	if err != nil || !filepath.IsAbs(unshare) {
		status = Failed
	}
	currentUser, userErr := user.Current()
	username := ""
	if userErr == nil {
		username = currentUser.Username
	}
	uidStart, uidCount, uidErr := subordinateRange("/etc/subuid", username)
	gidStart, gidCount, gidErr := subordinateRange("/etc/subgid", username)
	if userErr != nil || uidErr != nil || gidErr != nil || uidCount == 0 || gidCount == 0 {
		status = Failed
	}
	if status == Observed {
		cmd := exec.CommandContext(ctx, unshare,
			"--map-users", fmt.Sprintf("0:%d:1", os.Getuid()),
			"--map-users", fmt.Sprintf("1:%d:%d", uidStart, uidCount),
			"--map-groups", fmt.Sprintf("0:%d:1", os.Getgid()),
			"--map-groups", fmt.Sprintf("1:%d:%d", gidStart, gidCount),
			"--mount", "--fork", "--kill-child", "--setuid", "0", "--setgid", "0",
			cfg.SelfExecutable, "--worker", "namespace-fixture", "--worker-path", specPath)
		cmd.Env = []string{"PATH=/usr/bin", "HOME=" + filepath.Join(root.Path, "home"), "XDG_STATE_HOME=" + filepath.Join(root.Path, "xdg-state")}
		if err = cmd.Run(); err != nil {
			status = Failed
			observation = "isolated namespace worker failed before both owner/alias observations"
		}
	}
	var result namespaceFixtureReport
	if status == Observed {
		bytes, readErr := os.ReadFile(report)
		decoder := json.NewDecoder(strings.NewReader(string(bytes)))
		decoder.DisallowUnknownFields()
		if readErr != nil || decoder.Decode(&result) != nil || !result.BindAlias || !result.ForeignOwner {
			status = Failed
		}
		info, statErr := os.Lstat(foreign)
		stat, ok := infoSysStat(info)
		if statErr != nil || !ok || int(stat.Uid) == os.Getuid() {
			status = Failed
		}
	}
	if err := os.Remove(foreign); err != nil && !errors.Is(err, os.ErrNotExist) {
		status = Failed
	}
	add(Scenario{ID: "VAL-026/namespace-owner-bind-alias", Requirements: []string{"VAL-026", "ARCH-016", "ARCH-018"}, Status: status, Fixture: "isolated user and mount namespace", Classification: "BAD corrupt", Admission: "deny", Lease: "none", Writes: []string{"namespace-fixtures"}, Observation: observation, RejectedAssumption: "lexically distinct roots or caller-created objects necessarily have distinct mount identity and required ownership"})
}

func subordinateRange(path, name string) (uint64, uint64, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) != 3 || fields[0] != name {
			continue
		}
		start, startErr := strconv.ParseUint(fields[1], 10, 32)
		count, countErr := strconv.ParseUint(fields[2], 10, 32)
		if startErr != nil || countErr != nil || count == 0 {
			return 0, 0, errors.New("invalid subordinate identity range")
		}
		return start, count, nil
	}
	return 0, 0, errors.New("subordinate identity range unavailable")
}

func runNamespaceWorker(path string) error {
	var spec namespaceFixtureSpec
	bytes, err := os.ReadFile(path)
	if err != nil || len(bytes) > 4096 {
		return errors.New("invalid namespace fixture specification")
	}
	decoder := json.NewDecoder(strings.NewReader(string(bytes)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&spec); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalid namespace fixture specification")
	}
	if err = os.WriteFile(filepath.Join(spec.Source, "identity"), []byte("namespace-proof"), 0600); err != nil {
		return err
	}
	if err = syscall.Mount(spec.Source, spec.Target, "", syscall.MS_BIND, ""); err != nil {
		return err

	}
	sourceInfo, sourceErr := os.Stat(spec.Source)
	targetInfo, targetErr := os.Stat(spec.Target)
	sourceStat, sourceOK := infoSysStat(sourceInfo)
	targetStat, targetOK := infoSysStat(targetInfo)
	bindAlias := sourceErr == nil && targetErr == nil && sourceOK && targetOK && sourceStat.Dev == targetStat.Dev && sourceStat.Ino == targetStat.Ino
	if err = os.WriteFile(spec.Foreign, []byte("foreign-owner-proof"), 0600); err != nil {
		_ = syscall.Unmount(spec.Target, syscall.MNT_DETACH)
		return err
	}
	if err = os.Chown(spec.Foreign, 1, 1); err != nil {
		_ = syscall.Unmount(spec.Target, syscall.MNT_DETACH)
		return err
	}
	foreignInfo, foreignErr := os.Lstat(spec.Foreign)
	foreignStat, foreignOK := infoSysStat(foreignInfo)
	if err = syscall.Unmount(spec.Target, syscall.MNT_DETACH); err != nil {
		return err
	}
	result, err := json.Marshal(namespaceFixtureReport{BindAlias: bindAlias, ForeignOwner: foreignErr == nil && foreignOK && foreignStat.Uid == 1})
	if err != nil {
		return err
	}
	return os.WriteFile(spec.Report, result, 0600)
}
func runClosedAccessDeny(ctx context.Context, cfg Config, root *prooffs.OwnedRoot, add func(Scenario)) bool {
	target := filepath.Join(root.Path, "namespace-fixtures", "landlock-denied-read")
	if err := os.WriteFile(target, []byte("deny-observation"), 0600); err != nil {
		add(Scenario{ID: "VAL-027/access-deny", Requirements: []string{"VAL-027"}, Status: Failed, Fixture: "Landlock read-file deny", Writes: []string{"namespace-fixtures/landlock-denied-read"}, Observation: "failed to create disposable denied-read target"})
		return false
	}
	status := Observed
	observation := "same-binary child installed no_new_privs and a zero-allow-rule Landlock ruleset handling read-file; opening the named disposable regular file returned EACCES/EPERM"
	if cfg.SelfExecutable == "" {
		status = Inconclusive
		observation = "SelfExecutable was not supplied; closed Landlock deny worker did not run"
	} else {
		command := exec.CommandContext(ctx, cfg.SelfExecutable, "--worker", "access-deny", "--worker-path", target)
		command.Env = []string{"PATH=/nonexistent", "HOME=/nonexistent", "XDG_STATE_HOME=/nonexistent"}
		if err := command.Run(); err != nil {
			status = Failed
			observation = "closed Landlock deny worker failed before observing the named EACCES/EPERM result"
		}
	}
	add(Scenario{ID: "VAL-027/access-deny", Requirements: []string{"VAL-027"}, Status: status, Fixture: "Landlock read-file deny over one named disposable state-proof target", Admission: "deny", Writes: []string{"namespace-fixtures/landlock-denied-read"}, Observation: observation, RejectedAssumption: "zero configured authentication/network inputs proves general non-access; this observation is intentionally limited to the exercised filesystem-read denial"})
	return status == Observed
}

type landlockRulesetAttr struct {
	HandledAccessFS uint64
}

func runLandlockDenyWorker(target string) error {
	if target == "" || !filepath.IsAbs(target) {
		return errors.New("invalid Landlock deny target")
	}
	const (
		sysLandlockCreateRuleset = 444
		sysLandlockRestrictSelf  = 446
		landlockAccessFSReadFile = 1 << 2
		prSetNoNewPrivs          = 38
	)
	attr := landlockRulesetAttr{HandledAccessFS: landlockAccessFSReadFile}
	// #nosec G103 -- Landlock v1 reads this live, exact eight-byte ruleset attribute; no pointer escapes the synchronous syscall.
	fd, _, errno := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("create Landlock ruleset: %w", errno)
	}
	defer syscall.Close(int(fd))
	if _, _, errno = syscall.Syscall6(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("set no_new_privs: %w", errno)
	}
	if _, _, errno = syscall.Syscall(sysLandlockRestrictSelf, fd, 0, 0); errno != 0 {
		return fmt.Errorf("restrict self with Landlock: %w", errno)
	}
	file, err := os.Open(target)
	if err == nil {
		_ = file.Close()
		return errors.New("landlock unexpectedly allowed denied read")
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("unexpected Landlock read result: %w", err)
	}
	return nil
}

func infoSysStat(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return stat, ok
}

func diffEntries(a, b []prooffs.EntryObservation) []prooffs.EntryObservation {
	old := make(map[string]prooffs.EntryObservation, len(a))
	for _, x := range a {
		old[x.Path] = x
	}
	var out []prooffs.EntryObservation
	for _, x := range b {
		y, ok := old[x.Path]
		if !ok || x.Type != y.Type || x.Mode != y.Mode || x.Owner != y.Owner || x.Inode != y.Inode || x.SHA256 != y.SHA256 {
			out = append(out, x)
		}
	}
	return out
}

const (
	maxRepositoryEntries      = 1 << 20
	maxRepositories           = 1024
	maxRepositoryFileBytes    = int64(1 << 30)
	maxRepositoryBytes        = int64(4 << 30)
	maxRepositoryObjects      = 1 << 20
	maxRepositoryObjectList   = int64(128 << 20)
	maxRepositoryDecodedBytes = int64(4 << 30)
)

const pinnedStateGitVersion = "2.55.0"

type boundedGitBuffer struct {
	bytes.Buffer
	limit int64
}

func (buffer *boundedGitBuffer) Write(chunk []byte) (int, error) {
	if int64(len(chunk)) > buffer.limit-int64(buffer.Len()) {
		return 0, errors.New("git output byte ceiling exceeded")
	}
	return buffer.Buffer.Write(chunk)
}

func runPinnedGit(ctx context.Context, supervisor, gitPath, repository string, env, args []string, stdin io.Reader, stdout io.Writer) error {
	stderr := &boundedGitBuffer{limit: 1 << 20}
	observation, err := lifecycle.Supervise(ctx, supervisor, lifecycle.Spec{
		Name: "state-marker-git", Executable: gitPath, Args: args, Dir: repository,
		Env: env, Stdin: stdin, Stdout: stdout, Stderr: stderr, DenyIOUring: true,
	})
	complete := observation.Ready && observation.Session && observation.ProcessGroup &&
		observation.Subreaper && observation.ParentChecked && observation.ParentDeathSignal &&
		observation.GuardArmed && observation.GuardCleanup && observation.GuardStatusObserved &&
		observation.GuardStatus == 0 && observation.LeaderObserved &&
		observation.CgroupObserved && observation.CgroupEmpty && observation.IOUringDenied &&
		observation.FDChurnComplete && observation.Orphans == 0 && observation.SurvivorsDetected == 0 &&
		observation.MemoryOOM == 0 && observation.OOMKill == 0
	if err != nil || observation.Status != 0 || !complete {
		return errors.New("pinned Git same-FD lifecycle incomplete")
	}
	return nil
}

type boundedMarkerScanner struct {
	marker []byte
	tail   []byte
	bytes  int64
	limit  int64
	found  bool
}

func (scanner *boundedMarkerScanner) Write(chunk []byte) (int, error) {
	if int64(len(chunk)) > scanner.limit-scanner.bytes {
		return 0, errors.New("marker scan byte ceiling exceeded")
	}
	scanner.bytes += int64(len(chunk))
	if scanner.found {
		return len(chunk), nil
	}
	window := make([]byte, 0, len(scanner.tail)+len(chunk))
	window = append(window, scanner.tail...)
	window = append(window, chunk...)
	scanner.found = bytes.Contains(window, scanner.marker)
	keep := len(scanner.marker) - 1
	if keep > len(window) {
		keep = len(window)
	}
	scanner.tail = append(scanner.tail[:0], window[len(window)-keep:]...)
	return len(chunk), nil
}

func repositoryContainsMarker(ctx context.Context, supervisor, gitPath, root, proofRoot string, marker []byte) (bool, error) {
	if len(marker) == 0 {
		return false, errors.New("empty repository marker")
	}
	repositories := []string{root}
	seenRepositories := map[string]bool{root: true}
	entries, totalBytes := 0, int64(0)
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > maxRepositoryEntries {
			return errors.New("repository entry ceiling exceeded")
		}
		if path != root && entry.Name() == ".git" {
			repository := filepath.Dir(path)
			if !seenRepositories[repository] {
				if len(repositories) >= maxRepositories {
					return errors.New("nested repository ceiling exceeded")
				}
				seenRepositories[repository] = true
				repositories = append(repositories, repository)
			}
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			found = found || bytes.Contains([]byte(target), marker)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() < 0 || info.Size() > maxRepositoryFileBytes || info.Size() > maxRepositoryBytes-totalBytes {
			return errors.New("repository file byte ceiling exceeded")
		}
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), path)
		if file == nil {
			_ = syscall.Close(fd)
			return errors.New("construct repository no-follow file")
		}
		scanner := &boundedMarkerScanner{marker: marker, limit: info.Size()}
		_, copyErr := io.Copy(scanner, io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if scanner.bytes != info.Size() {
			return errors.New("repository file changed during scan")
		}
		totalBytes += scanner.bytes
		found = found || scanner.found
		return nil
	})
	if err != nil {
		return found, err
	}
	sort.Strings(repositories)
	env := []string{
		"HOME=" + filepath.Join(proofRoot, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(proofRoot, "xdg-config"),
		"PATH=/nonexistent",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_NO_REPLACE_OBJECTS=1",
	}
	version := &boundedGitBuffer{limit: 256}
	if err = runPinnedGit(ctx, supervisor, gitPath, root, env, []string{"--version"}, nil, version); err != nil ||
		strings.TrimSpace(version.String()) != "git version "+pinnedStateGitVersion {
		return found, errors.New("pinned Git version mismatch")
	}
	totalObjects, totalListedBytes, totalDecodedBytes := 0, int64(0), int64(0)
	for _, repository := range repositories {
		objectList := &boundedGitBuffer{limit: maxRepositoryObjectList - totalListedBytes}
		if err = runPinnedGit(ctx, supervisor, gitPath, repository, env, []string{"-C", repository, "rev-list", "--objects", "--no-object-names", "--all"}, nil, objectList); err != nil {
			return found, errors.New("enumerate reachable repository objects")
		}
		var objectInput bytes.Buffer
		lineScanner := bufio.NewScanner(bytes.NewReader(objectList.Bytes()))
		objects, listedBytes := 0, int64(0)
		for lineScanner.Scan() {
			line := lineScanner.Bytes()
			listedBytes += int64(len(line) + 1)
			fields := bytes.Fields(line)
			if len(fields) != 1 || (len(fields[0]) != 40 && len(fields[0]) != 64) || len(fields[0])%2 != 0 {
				return found, errors.New("unexpected repository object identity")
			}
			if _, decodeErr := hex.DecodeString(string(fields[0])); decodeErr != nil {
				return found, errors.New("malformed repository object identity")
			}
			objectInput.Write(fields[0])
			objectInput.WriteByte('\n')
			objects++
			if objects > maxRepositoryObjects-totalObjects {
				return found, errors.New("repository object ceiling exceeded")
			}
		}
		if scanErr := lineScanner.Err(); scanErr != nil {
			return found, scanErr
		}
		totalObjects += objects
		totalListedBytes += listedBytes
		decoded := &boundedMarkerScanner{marker: marker, limit: maxRepositoryDecodedBytes - totalDecodedBytes}
		if err = runPinnedGit(ctx, supervisor, gitPath, repository, env, []string{"-C", repository, "cat-file", "--batch"}, &objectInput, decoded); err != nil {
			return found, errors.New("stream decoded reachable repository objects")
		}
		totalDecodedBytes += decoded.bytes
		found = found || decoded.found
	}
	return found, nil
}

func scanMarker(root string, entries []prooffs.EntryObservation, marker []byte) int {
	count := 0
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err == nil && bytes.Contains(data, marker) {
			count++
		}
	}
	return count
}

func gatedWorkerReady() error {
	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		return err
	}
	_, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}

func hardKillSelf() error {
	if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
		return err
	}
	select {}
}

func runInitializationPublicationWorker(spec deterministicWorkerSpec) error {
	lease, err := acquireStableLease(spec.State, struct {
		name string
		typ  int16
	}{"init.lock", fWRLCK})
	if err != nil {
		return err
	}
	defer lease.release()
	identity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	claim := initializingRoot(spec.Instance, spec.Token, 0, identity)
	if err = publishRecord(spec.State, "root.json", claim); err != nil {
		return err
	}
	for _, name := range []string{"admission.lock", "writer.lock"} {
		if err = createDurableEmpty(spec.State, name); err != nil {
			return err
		}
	}
	if err = gatedWorkerReady(); err != nil {
		return err
	}
	owner := releasedWriter(spec.Instance, "", 0, 0, "")
	dir, err := os.Open(spec.State)
	if err != nil {
		return err
	}
	defer dir.Close()
	return prooffs.AtomicPublishAt(int(dir.Fd()), "writer-owner.json", recordBytes(owner), prooffs.PublishOptions{Fail: func(boundary prooffs.Boundary) error {
		if string(boundary) == spec.Boundary {
			return hardKillSelf()
		}
		return nil
	}})
}
func runPIDReuseWorker(reportPath string) error {
	if reportPath == "" || !filepath.IsAbs(reportPath) {
		return errors.New("invalid PID-reuse report path")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil || !filepath.IsAbs(executable) {
		return errors.New("PID-reuse executable identity unavailable")
	}
	startChild := func(lastPID int) (*exec.Cmd, processIdentity, error) {
		if err := os.WriteFile("/proc/sys/kernel/ns_last_pid", []byte(strconv.Itoa(lastPID)), 0600); err != nil {
			return nil, processIdentity{}, err
		}
		cmd := exec.Command(executable, "--worker", "pid-reuse-hold")
		cmd.Env = []string{"PATH=/nonexistent", "HOME=/nonexistent", "XDG_STATE_HOME=/nonexistent"}
		if err := cmd.Start(); err != nil {
			return nil, processIdentity{}, err
		}
		identity, err := processIdentityForPID(cmd.Process.Pid)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, processIdentity{}, err
		}
		return cmd, identity, nil
	}
	lockPath := filepath.Join(filepath.Dir(reportPath), "writer.lock")
	ownerPath := filepath.Join(filepath.Dir(reportPath), "writer-owner.json")
	if err = createDurableEmpty(filepath.Dir(reportPath), filepath.Base(lockPath)); err != nil {
		return err
	}
	lockInfo, err := os.Lstat(lockPath)
	if err != nil {
		return err
	}
	withStableLock := func(action func() error) (retErr error) {
		file, lockErr := os.OpenFile(lockPath, os.O_RDWR, 0)
		if lockErr != nil {
			return lockErr
		}
		defer file.Close()
		if lockErr = ofd(int(file.Fd()), fWRLCK); lockErr != nil {
			return lockErr
		}
		defer func() {
			if unlockErr := ofd(int(file.Fd()), fUNLCK); retErr == nil && unlockErr != nil {
				retErr = unlockErr
			}
		}()
		reopenedLock, statErr := os.Lstat(lockPath)
		if statErr != nil || !os.SameFile(lockInfo, reopenedLock) {
			return errors.New("PID-reuse stable lock changed")
		}
		return action()
	}
	first, firstIdentity, err := startChild(99)
	if err != nil {
		return err
	}
	refs := freshGraphRefs()
	if err = withStableLock(func() error {
		return publishRecord(filepath.Dir(ownerPath), filepath.Base(ownerPath), heldWriter(refs.instance, refs.writerToken, 7, firstIdentity, 6, refs.priorWriterToken))
	}); err != nil {
		_ = first.Process.Kill()
		_ = first.Wait()
		return err
	}
	_ = first.Process.Kill()
	if err := first.Wait(); err == nil {
		return errors.New("PID-reuse first child did not report hard termination")
	}
	time.Sleep(25 * time.Millisecond)
	var second *exec.Cmd
	var secondIdentity processIdentity
	for range 4096 {
		candidate, identity, startErr := startChild(firstIdentity.PID - 1)
		if startErr != nil {
			return startErr
		}
		if identity.PID == firstIdentity.PID {
			second, secondIdentity = candidate, identity
			break
		}
		_ = candidate.Process.Kill()
		_ = candidate.Wait()
		time.Sleep(time.Millisecond)
	}
	if second == nil {
		return errors.New("PID allocator did not recycle the selected numeric identity")
	}
	durableReopened, staleRejected := false, false
	if err = withStableLock(func() error {
		var durable writerOwnerRecord
		if readErr := readRecord(ownerPath, &durable); readErr != nil {
			return readErr
		}
		durableReopened = durable.Status == "held" && durable.Generation == 7 && durable.Token == refs.writerToken && durable.Process == firstIdentity
		if !durableReopened {
			return errors.New("PID-reuse durable owner tuple mismatch")
		}
		live, liveErr := processIdentityLive(durable.Process)
		staleRejected = liveErr == nil && !live
		return nil
	}); err != nil {
		_ = second.Process.Kill()
		_ = second.Wait()
		return err
	}
	_ = second.Process.Kill()
	if err := second.Wait(); err == nil {
		return errors.New("PID-reuse second child did not report hard termination")
	}
	report := pidReuseReport{
		NumericPIDReused: firstIdentity.PID == secondIdentity.PID,
		StartChanged:     firstIdentity.Start != secondIdentity.Start,
		NamespaceBound:   firstIdentity.PIDNamespace != 0 && firstIdentity.PIDNamespace == secondIdentity.PIDNamespace,
		DurableReopened:  durableReopened,

		StaleRejected: staleRejected,
	}
	content, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return os.WriteFile(reportPath, content, 0600)
}

func validProcessIdentity(identity processIdentity) bool {
	return identity.PID > 0 && identity.Start != 0 && identity.PIDNamespace != 0
}

func processIdentityLive(expected processIdentity) (bool, error) {
	return processIdentityLiveInjected(expected, processIdentityForPID)
}

func processIdentityLiveInjected(
	expected processIdentity,
	readIdentity func(int) (processIdentity, error),
) (bool, error) {
	if !validProcessIdentity(expected) {
		return false, errors.New("invalid expected process identity")
	}
	actual, err := readIdentity(expected.PID)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return actual == expected, nil
}

func validateRecoveryInput(spec deterministicWorkerSpec) error {
	if spec.InputRoot == "" || !filepath.IsAbs(spec.InputRoot) ||
		spec.UnavailableInput == "" || filepath.Base(spec.UnavailableInput) != spec.UnavailableInput {
		return errors.New("invalid recovery input specification")
	}
	path := filepath.Join(spec.InputRoot, spec.UnavailableInput)
	content, err := readNoFollowBounded(path, 4096)
	if err != nil {
		return err
	}
	if !safeRegularUID(path, os.Geteuid()) ||
		!bytes.Equal(content, recoveryInputBytes(spec.UnavailableInput, spec.refs())) {
		return errors.New("recovery input identity or content mismatch")
	}
	return nil
}

func runInitializationContender(spec deterministicWorkerSpec) error {
	lease, err := acquireStableLease(spec.State, struct {
		name string
		typ  int16
	}{"init.lock", fWRLCK})
	if err == nil {
		lease.release()
		return errors.New("initializer contender unexpectedly acquired")
	}
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
		return nil
	}
	return err
}

func runInitializationRepairWorker(spec deterministicWorkerSpec) error {
	var claim rootOwnerRecord
	if readRecord(filepath.Join(spec.State, "root.json"), &claim) != nil {
		return errors.New("initialization repair claim unavailable")
	}
	if err := gatedWorkerReady(); err != nil {
		return err
	}
	_, err := initializationCommandRouter(
		spec, claim.Process, "recover-repair",
		func(stage string) error {
			if stage == spec.RecoveryBoundary {
				return hardKillSelf()
			}
			return nil
		},
	)
	return err
}

func runTransitionRecoveryWorker(spec deterministicWorkerSpec, mode string) error {
	if err := gatedWorkerReady(); err != nil {
		return err
	}
	hook := func(stage string) error {
		if stage != spec.RecoveryBoundary {
			return nil
		}
		if spec.UnavailableInput != "" {
			if err := validateRecoveryInput(spec); err != nil {
				return fmt.Errorf("%s unavailable: %w", spec.UnavailableInput, err)
			}
			return errors.New("configured unavailable recovery input unexpectedly validated")
		}
		switch mode {
		case "transition-recover-die":
			return hardKillSelf()
		case "transition-recover-cancel":
			return context.Canceled
		default:
			return errors.New("deterministic recovery failure")
		}
	}
	var authority *transitionRecoveryAuthority
	if spec.ResidueIdentity != nil {
		authority = &transitionRecoveryAuthority{
			Boundary: transitionBoundary{Point: string(prooffs.AfterTemp)},
			Identity: *spec.ResidueIdentity, ExpectResidue: true, RecoveryStage: "recovery-journal-transfer",
		}
	}
	// Both journal-less and journaled injections admit repair; classification stays under A-X.
	_, err := transitionCommandRouterInjected(
		spec.State, spec.Data, spec.External, spec.refs(), "recover-repair", authority, hook,
	)
	if err == nil {
		return errors.New("recovery boundary was not reached")
	}
	return err
}

func runTransitionWorker(spec deterministicWorkerSpec) error {
	identity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	ready := func() error { return gatedWorkerReady() }
	hook := func(stage string) error {
		if stage == spec.Boundary {
			return hardKillSelf()
		}
		return nil
	}
	return executeTransition(spec.State, spec.Data, spec.External, spec.refs(), identity, ready, hook)
}

func runCatchableTransitionWorker(spec deterministicWorkerSpec, cancellation bool) error {
	identity, err := processIdentityForPID(os.Getpid())
	if err != nil {
		return err
	}
	injected := errors.New("deterministic transition failure")
	if cancellation {
		injected = context.Canceled
	}
	hook := func(stage string) error {
		if stage == spec.Boundary {
			return injected
		}
		return nil
	}
	err = executeTransition(
		spec.State, spec.Data, spec.External, spec.refs(), identity,
		func() error { return gatedWorkerReady() }, hook,
	)
	if err == nil {
		return errors.New("catchable transition boundary was not reached")
	}
	return err
}

func Worker(mode, p string) error {
	switch mode {
	case "lock-die":
		f, e := os.OpenFile(p, os.O_RDWR, 0)
		if e != nil {
			return e
		}
		if e = ofd(int(f.Fd()), fWRLCK); e != nil {
			return e
		}
		os.Exit(91)
	case "init-publication-die":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runInitializationPublicationWorker(spec)
	case "init-contender":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runInitializationContender(spec)
	case "init-repair-die":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runInitializationRepairWorker(spec)
	case "transition-recover-die", "transition-recover-fail", "transition-recover-cancel":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runTransitionRecoveryWorker(spec, mode)
	case "transition-die":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runTransitionWorker(spec)
	case "transition-fail", "transition-cancel":
		spec, err := readDeterministicWorkerSpec(p)
		if err != nil {
			return err
		}
		return runCatchableTransitionWorker(spec, mode == "transition-cancel")
	case "self-executable-replacement":
		return runSelfExecutableReplacementWorker(p)
	case "publish-die":
		d, e := os.Open(filepath.Dir(p))
		if e != nil {
			return e
		}
		defer d.Close()
		return prooffs.AtomicPublishAt(int(d.Fd()), filepath.Base(p), []byte("synthetic"), prooffs.PublishOptions{Fail: func(prooffs.Boundary) error { os.Exit(92); return nil }})
	case "pid-reuse":
		return runPIDReuseWorker(p)
	case "pid-reuse-hold":
		time.Sleep(time.Minute)
		return nil
	case "access-deny":
		return runLandlockDenyWorker(p)
	case "namespace-fixture":
		return runNamespaceWorker(p)
	}
	return errors.New("unknown worker mode")
}

func processIdentityForPID(pid int) (processIdentity, error) {
	var identity processIdentity
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return identity, err
	}
	text := string(content)
	index := strings.LastIndex(text, ") ")
	if index < 0 {
		return identity, errors.New("malformed proc stat")
	}
	fields := strings.Fields(text[index+2:])
	if len(fields) < 20 {
		return identity, errors.New("short proc stat")
	}
	namespace, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "ns", "pid"))
	if err != nil {
		return identity, fmt.Errorf("PID namespace identity unavailable: %w", err)
	}
	if namespace == "" {
		return identity, errors.New("PID namespace identity unavailable")
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || start == 0 {
		return identity, errors.New("invalid process start identity")
	}
	namespaceText := strings.TrimSuffix(strings.TrimPrefix(namespace, "pid:["), "]")
	namespaceID, err := strconv.ParseUint(namespaceText, 10, 64)
	if err != nil || namespaceID == 0 {
		return identity, errors.New("invalid PID namespace identity")
	}
	return processIdentity{PID: pid, Start: start, PIDNamespace: namespaceID}, nil
}
func FreshToken() (string, error) {
	b := make([]byte, 16)
	if _, e := io.ReadFull(rand.Reader, b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}

func retainedRef(kind string) (string, error) {
	token, err := FreshToken()
	if err != nil {
		return "", err
	}
	return "typed:" + kind + ":" + token, nil
}
