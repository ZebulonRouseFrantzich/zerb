// Package adapters is a disposable Phase 1 proof harness, not product code.
package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/template/parse"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

const (
	ResultVersion = 7
	maxLockBytes  = 1 << 20
)

type Config struct {
	Root                string   `json:"root"`
	Executable          string   `json:"executable"`
	Git                 string   `json:"git"`
	Bash                string   `json:"bash"`
	CoreutilsDir        string   `json:"coreutils_dir"`
	Mise                string   `json:"mise"`
	Chezmoi             string   `json:"chezmoi"`
	Trace               string   `json:"trace"`
	CACert              string   `json:"ca_cert"`
	ArtifactDir         string   `json:"artifact_dir"`
	NodeLock            string   `json:"node_lock"`
	ProtectedRoots      []string `json:"-"`
	PlatformCgroupRoot  string   `json:"-"`
	ExecutionCgroupPath string   `json:"-"`
}

type Result struct {
	Version                  int        `json:"version"`
	AmbientMarkerFingerprint string     `json:"ambient_marker_fingerprint"`
	Scenarios                []Scenario `json:"scenarios"`
}

type Scenario struct {
	Name               string                  `json:"name"`
	Status             string                  `json:"status"`
	RequirementIDs     []string                `json:"requirement_ids"`
	Invocations        []InvocationObservation `json:"invocations,omitempty"`
	Environment        EnvironmentObservation  `json:"environment"`
	Access             AccessObservation       `json:"access"`
	Writes             WriteObservation        `json:"writes"`
	Resources          ResourceObservation     `json:"resources"`
	Facts              []FactObservation       `json:"facts,omitempty"`
	Targets            []TargetObservation     `json:"targets,omitempty"`
	RejectedAssumption string                  `json:"rejected_assumption"`
	Detail             string                  `json:"detail,omitempty"`
}

type InvocationObservation struct {
	Executable            string   `json:"executable"`
	Arguments             []string `json:"arguments"`
	WorkingDirectory      string   `json:"working_directory"`
	Descriptors           []int    `json:"descriptors"`
	Invoked               bool     `json:"invoked"`
	ExitStatus            int      `json:"exit_status"`
	TraceComplete         bool     `json:"trace_complete"`
	LifecycleComplete     bool     `json:"lifecycle_complete"`
	DeathGuardArmed       bool     `json:"death_guard_armed"`
	DeathGuardCleanup     bool     `json:"death_guard_cleanup"`
	DescendantsReaped     int      `json:"descendants_reaped"`
	Orphans               int      `json:"orphans"`
	SurvivorsDetected     int      `json:"survivors_detected"`
	FileEvents            int      `json:"file_events"`
	ProcessEvents         int      `json:"process_events"`
	SocketEvents          int      `json:"socket_events"`
	ExecutableClasses     []string `json:"executable_classes"`
	CgroupObserved        bool     `json:"cgroup_observed"`
	CgroupEmpty           bool     `json:"cgroup_empty"`
	CgroupCleanupDeferred bool     `json:"cgroup_cleanup_deferred"`
	MemoryPeakBytes       uint64   `json:"memory_peak_bytes"`
	ProcessPeak           uint64   `json:"process_peak"`
	CPUUsec               uint64   `json:"cpu_usec"`
	IOUringDenied         bool     `json:"io_uring_denied"`
	MemoryOOM             uint64   `json:"memory_oom"`
	OOMKill               uint64   `json:"oom_kill"`
	CgroupKillGuard       bool     `json:"cgroup_kill_guard"`
}

type EnvironmentObservation struct {
	Names                    []string `json:"names"`
	ParentUnchanged          bool     `json:"parent_unchanged"`
	AmbientFingerprintAbsent bool     `json:"ambient_fingerprint_absent"`
}

type AccessObservation struct {
	TraceAvailable        bool     `json:"trace_available"`
	RealHome              *int     `json:"real_home,omitempty"`
	RealXDG               *int     `json:"real_xdg,omitempty"`
	RealDestination       *int     `json:"real_destination,omitempty"`
	HelpersAgentsRelays   *int     `json:"helpers_agents_relays,omitempty"`
	ProvidersHostFacts    *int     `json:"providers_host_facts,omitempty"`
	PlatformObservations  *int     `json:"platform_observations,omitempty"`
	PlatformClasses       []string `json:"platform_classes,omitempty"`
	SyscallClasses        []string `json:"syscall_classes,omitempty"`
	TemplateRenders       *int     `json:"template_renders,omitempty"`
	UnknownClasses        []string `json:"unknown_classes,omitempty"`
	NetworkAttempts       *int     `json:"network_attempts,omitempty"`
	RuntimeBootstrapReads int      `json:"runtime_bootstrap_reads"`
	UnknownEvents         int      `json:"unknown_events"`
	FileEvents            int      `json:"file_events"`
	ProcessEvents         int      `json:"process_events"`
	platformClassCounts   map[string]int
	syscallClassCounts    map[string]int
	SocketEvents          int `json:"socket_events"`
}

type WriteObservation struct {
	TraceAvailable   bool     `json:"trace_available"`
	OutsideOwnedRoot *int     `json:"outside_owned_root,omitempty"`
	Paths            []string `json:"path_classes,omitempty"`
	DeniedPaths      []string `json:"denied_path_classes,omitempty"`
}

type ResourceObservation struct {
	OutputBytes       int    `json:"output_bytes"`
	Entries           int    `json:"entries"`
	CgroupInvocations int    `json:"cgroup_invocations"`
	CgroupComplete    bool   `json:"cgroup_complete"`
	MemoryPeakBytes   uint64 `json:"memory_peak_bytes"`
	ProcessPeak       uint64 `json:"process_peak"`
	CPUUsec           uint64 `json:"cpu_usec"`
	MemoryOOM         uint64 `json:"memory_oom"`
	OOMKill           uint64 `json:"oom_kill"`
}

type FactObservation struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

type TargetObservation struct {
	Target          string `json:"target"`
	Source          string `json:"source"`
	Type            string `json:"type"`
	Mode            string `json:"mode"`
	OwnerRelation   string `json:"owner_relation"`
	LinkRelation    string `json:"link_relation"`
	MappingSource   string `json:"mapping_source"`
	AttributeSource string `json:"attribute_source"`
}

type fixture struct {
	name    string
	content []byte
	mode    os.FileMode
}

type managedPaths struct {
	Absolute       string `json:"absolute"`
	SourceAbsolute string `json:"sourceAbsolute"`
	SourceRelative string `json:"sourceRelative"`
}

type lockFact struct {
	Version  string
	Backend  string
	Checksum string
	URL      string
	Size     int64
}

type lockDocument struct {
	Tools struct {
		Node []lockNode `toml:"node"`
	} `toml:"tools"`
}

type lockNode struct {
	Version string       `toml:"version"`
	Backend string       `toml:"backend"`
	Linux   lockPlatform `toml:"platforms.linux-x64"`
}

type lockPlatform struct {
	Checksum string `toml:"checksum"`
	URL      string `toml:"url"`
	Size     int64  `toml:"size"`
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	beforeEnv := sortedEnvironment()
	root, err := validateRoot(cfg.Root, cfg.ProtectedRoots)
	if err != nil {
		return Result{}, err
	}
	for name, path := range map[string]string{"same-binary": cfg.Executable, "git": cfg.Git, "bash": cfg.Bash, "mise": cfg.Mise, "chezmoi": cfg.Chezmoi, "trace": cfg.Trace} {
		if err := canonicalExecutable(name, path); err != nil {
			return Result{}, err
		}
	}
	cgroupRoot, err := currentCgroupRoot()
	if err != nil {
		return Result{}, err
	}
	cfg.PlatformCgroupRoot = cgroupRoot
	if err := plantHostileAmbient(root); err != nil {
		return Result{}, err
	}
	ambientSeed := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, ambientSeed); err != nil {
		return Result{}, err
	}
	ambientMarker := []byte(hex.EncodeToString(ambientSeed))
	const ambientName = "ZERB_PROOF_AMBIENT_MARKER"
	priorAmbient, hadPriorAmbient := os.LookupEnv(ambientName)
	if err := os.Setenv(ambientName, string(ambientMarker)); err != nil {
		return Result{}, err
	}
	restoreAmbient := func() error {
		if hadPriorAmbient {
			return os.Setenv(ambientName, priorAmbient)
		}
		return os.Unsetenv(ambientName)
	}
	ambientActive := true
	defer func() {
		if ambientActive {
			_ = restoreAmbient()
		}
	}()
	markerDigest := sha256.Sum256(ambientMarker)
	result := Result{Version: ResultVersion, AmbientMarkerFingerprint: hex.EncodeToString(markerDigest[:])}
	result.Scenarios = append(result.Scenarios, rejectionScenario(cfg.NodeLock))
	result.Scenarios = append(result.Scenarios, chezmoiScenario(ctx, cfg, root, ambientMarker))
	result.Scenarios = append(result.Scenarios, miseScenario(ctx, cfg, root, ambientMarker))
	if spilled, scanErr := treeContainsMarker(root, ambientMarker); scanErr != nil || spilled {
		return Result{}, errors.New("ambient marker reached a filesystem sink")
	}
	encoded, err := json.Marshal(result)
	if err != nil || bytes.Contains(encoded, ambientMarker) {
		return Result{}, errors.New("ambient marker reached structured output")
	}
	if err := restoreAmbient(); err != nil {
		return Result{}, err
	}
	ambientActive = false
	afterEnv := sortedEnvironment()
	unchanged := strings.Join(beforeEnv, "\x00") == strings.Join(afterEnv, "\x00")
	for i := range result.Scenarios {
		result.Scenarios[i].Environment.ParentUnchanged = unchanged
		result.Scenarios[i].Environment.AmbientFingerprintAbsent = true
	}
	return result, nil
}
func currentCgroupRoot() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil || len(data) == 0 || len(data) > 4096 {
		return "", errors.New("current cgroup identity unavailable")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		return "", errors.New("current unified cgroup identity ambiguous")
	}
	fields := strings.SplitN(lines[0], ":", 3)
	if len(fields) != 3 || fields[0] != "0" || fields[1] != "" || !filepath.IsAbs(fields[2]) || filepath.Clean(fields[2]) != fields[2] {
		return "", errors.New("current unified cgroup identity malformed")
	}
	path := filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(fields[2], "/"))
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || !within(canonical, "/sys/fs/cgroup") {
		return "", errors.New("current cgroup root unavailable")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", errors.New("current cgroup root invalid")
	}
	return canonical, nil
}

func rejectionScenario(goodLock string) Scenario {
	s := Scenario{Name: "adapter-pre-manager-rejections", Status: "observed", RequirementIDs: []string{"VAL-015", "VAL-016", "VAL-021", "VAL-022", "VAL-038", "VAL-039"}, RejectedAssumption: "Invalid declarations, locks, entries, or structural targets may reach either manager."}
	locks := []struct {
		name  string
		value string
	}{
		{"missing", ""},
		{"mismatched", strings.Replace(goodLock, `version = "24.4.1"`, `version = "24.4.2"`, 1)},
		{"latest", strings.Replace(goodLock, `version = "24.4.1"`, `version = "latest"`, 1)},
		{"alias", strings.Replace(goodLock, `version = "24.4.1"`, `version = "24"`, 1)},
		{"range", strings.Replace(goodLock, `version = "24.4.1"`, `version = ">=24"`, 1)},
		{"duplicate", goodLock + "\n[[tools.node]]\nversion=\"24.4.1\"\nbackend=\"core:node\"\n"},
	}
	for _, candidate := range locks {
		if _, err := inspectLock([]byte(candidate.value)); err == nil {
			s.Status = "failed"
			s.Detail = candidate.name + " lock accepted"
			return s
		}
	}
	for _, n := range []int{maxLockBytes - 1, maxLockBytes, maxLockBytes + 1} {
		candidate := append([]byte(nil), []byte(goodLock)...)
		if len(candidate) <= maxLockBytes {
			candidate = append(candidate, '\n')
		}
		if len(candidate) < n {
			candidate = append(candidate, '#')
			candidate = append(candidate, bytes.Repeat([]byte("x"), n-len(candidate))...)
		} else {
			candidate = candidate[:n]
		}
		_, err := inspectLock(candidate)
		if (n <= maxLockBytes) != (err == nil) {
			s.Status = "failed"
			s.Detail = fmt.Sprintf("lock inspector byte boundary failed at %d", n)
			return s
		}
	}
	goodDecl := []byte("[tools]\nnode = \"24.4.1\"\n")
	declarations := [][]byte{nil, []byte("[tools]\nnode = \"latest\"\n"), []byte("[tools]\nnode = \"24\"\n"), []byte("[tools]\nnode = \">=24\"\n"), []byte("[tools]\nnode = \"24.4.1\"\nnode = \"24.4.1\"\n"), []byte("[tools.node]\nversion = \"24.4.1\"\n")}
	if err := inspectDeclaration(goodDecl); err != nil {
		s.Status = "failed"
		s.Detail = "exact scalar declaration rejected"
		return s
	}
	for _, declaration := range declarations {
		if inspectDeclaration(declaration) == nil {
			s.Status = "failed"
			s.Detail = "missing, alias/range/latest, duplicate, or table declaration accepted"
			return s
		}
	}
	bad := []fixture{{"run_once_bad", []byte("x"), 0700}, {".chezmoiexternal.toml", []byte("x"), 0600}, {"remove_bad", []byte("x"), 0600}, {"symlink_dot_bad", []byte("x"), 0600}, {"dot_bad.tmpl", []byte(`{{ env "HOME" }}`), 0600}, {"dot_bad2.tmpl", []byte(`{{ output "id" }}`), 0600}, {"dot_bad3.tmpl", []byte("{{ .chezmoi.hostname }}"), 0600}, {"dot_bad4.tmpl", []byte(`{{ lookup "x" }}`), 0600}, {"dot_bad5.tmpl", []byte("{{ if .dynamic }}x{{ end }}"), 0600}, {"dot_bad6.tmpl", []byte(`{{ template "x" . }}`), 0600}, {"dot_bad7.tmpl", []byte("{{ range true }}x{{ end }}"), 0600}}
	for _, f := range bad {
		if ValidateStatic(f.name, f.content) == nil {
			s.Status = "failed"
			s.Detail = "forbidden chezmoi fixture accepted"
			return s
		}
	}
	if _, err := normalizeStaticTargets("", []fixture{{"dot_same", []byte("a"), 0600}, {"private_dot_same", []byte("b"), 0600}}); err == nil {
		s.Status = "failed"
		s.Detail = "duplicate normalized target accepted"
		return s
	}
	s.Facts = []FactObservation{{Name: "manager_invocations", Value: "0", Source: "harness invocation ledger"}, {Name: "lock_byte_boundaries", Value: "1048575,1048576 accepted; 1048577 rejected", Source: "actual lock inspector"}, {Name: "invalid_lock_classes", Value: "missing,mismatched,alias,range,latest,duplicate", Source: "actual lock inspector"}, {Name: "invalid_declaration_classes", Value: "missing,alias,range,latest,duplicate,table", Source: "actual declaration inspector"}, {Name: "invalid_home_classes", Value: "unsupported-entry,collision,dynamic-template", Source: "direct static validator"}}
	s.Detail = "all negative fixtures rejected before manager invocation"
	return s
}
func chezmoiScenario(ctx context.Context, cfg Config, root string, ambientMarker []byte) Scenario {
	s := Scenario{Name: "chezmoi-static-managed", Status: "failed", RequirementIDs: []string{"VAL-016", "VAL-017", "VAL-018", "VAL-019", "VAL-022", "VAL-023", "VAL-038", "VAL-039", "PC-031", "PC-043", "PC-071"}, RejectedAssumption: "Machine-readable mapping supplies attributes it does not emit, renders templates, bypasses the common supervisor, or proves synthetic destination non-access without tracing."}
	base := filepath.Join(root, "chezmoi")
	source, destination := filepath.Join(base, "source"), filepath.Join(base, "synthetic-destination")
	config := filepath.Join(base, "chezmoi.toml")
	normalCache, normalWork, normalState := filepath.Join(base, "normal-cache"), filepath.Join(base, "normal-work"), filepath.Join(base, "normal-state", "chezmoi.boltdb")
	flaggedCache, flaggedWork, flaggedState := filepath.Join(base, "flagged-cache"), filepath.Join(base, "flagged-work"), filepath.Join(base, "flagged-state", "chezmoi.boltdb")
	for _, path := range []string{source, normalCache, normalWork, filepath.Dir(normalState), flaggedCache, flaggedWork, filepath.Dir(flaggedState)} {
		if err := os.MkdirAll(path, 0700); err != nil {
			s.Detail = err.Error()
			return s
		}
	}
	fixtures := []fixture{{"dot_ordinary", []byte("ordinary\n"), 0600}, {"executable_dot_executable", []byte("#!/bin/false\n"), 0700}, {"private_dot_private", []byte("private-static\n"), 0600}, {"private_executable_dot_private-executable", []byte("#!/bin/false\n"), 0700}, {"dot_literal.tmpl", []byte("{{ if true }}literal{{ else }}unselected{{ end }}\n"), 0600}}
	for _, f := range fixtures {
		if err := ValidateStatic(f.name, f.content); err != nil {
			s.Detail = err.Error()
			return s
		}
		if err := writeExclusive(filepath.Join(source, f.name), f.content, f.mode); err != nil {
			s.Detail = err.Error()
			return s
		}
	}
	failIfRendered := fixture{"dot_must_not_render.tmpl", []byte(`{{ fail "ZERB non-rendering semantic sentinel evaluated" }}`), 0600}
	if err := writeExclusive(filepath.Join(source, failIfRendered.name), failIfRendered.content, failIfRendered.mode); err != nil {
		s.Detail = err.Error()
		return s
	}
	staticTargets, err := normalizeStaticTargets(source, fixtures)
	if err != nil {
		s.Detail = err.Error()
		return s
	}
	staticTargets = append(staticTargets, TargetObservation{Target: ".must_not_render", Source: failIfRendered.name, Type: "regular-file", Mode: "0666", OwnerRelation: "owned-by-disposable-proof-owner", LinkRelation: "not-a-link", AttributeSource: "non-rendering semantic sentinel"})
	sort.Slice(staticTargets, func(i, j int) bool { return staticTargets[i].Target < staticTargets[j].Target })
	if err := writeExclusive(config, []byte("[data]\n"), 0600); err != nil {
		s.Detail = err.Error()
		return s
	}
	args := func(skip bool, cache, state, work string) []string {
		argv := []string{"managed", "--format", "json", "--path-style", "all", "--include", "files,dirs", "-S", source, "-D", destination, "-c", config, "--cache", cache, "--persistent-state", state, "-W", work, "--exclude", "scripts", "-R=never", "--no-tty"}
		if skip {
			argv = append(argv, "--skip-template-data-environment")
		}
		return argv
	}
	env := adapterEnv(root, cfg.CACert)
	s.Environment = EnvironmentObservation{Names: envNames(env), AmbientFingerprintAbsent: true}
	normalPolicy := tracePolicy{owned: base, proofRoot: root, writeRoot: base, cwd: normalWork, protected: cfg.ProtectedRoots, forbidden: ambientForbidden(root), syntheticDest: destination, forbiddenMarker: ambientMarker, allowUnclassifiedReads: true, platformCgroupRoot: cfg.PlatformCgroupRoot}
	normalOutput, normalInvocation, normalTrace, err := invokeTraced(ctx, cfg, "chezmoi-managed-normal", cfg.Chezmoi, args(false, normalCache, normalState, normalWork), normalWork, env, normalPolicy)
	s.Invocations = append(s.Invocations, normalInvocation)
	accumulateInvocationResources(&s.Resources, normalInvocation)
	mergeTrace(&s, normalTrace)
	s.Resources.OutputBytes += len(normalOutput)
	if err != nil {
		s.Detail = "normal mode: " + err.Error()
		return s
	}

	flaggedPolicy := tracePolicy{owned: base, proofRoot: root, writeRoot: base, cwd: flaggedWork, protected: cfg.ProtectedRoots, forbidden: ambientForbidden(root), syntheticDest: destination, forbiddenMarker: ambientMarker, forbidHostFacts: true, platformCgroupRoot: cfg.PlatformCgroupRoot}
	flaggedOutput, flaggedInvocation, flaggedTrace, err := invokeTraced(ctx, cfg, "chezmoi-managed-flagged", cfg.Chezmoi, args(true, flaggedCache, flaggedState, flaggedWork), flaggedWork, env, flaggedPolicy)
	s.Invocations = append(s.Invocations, flaggedInvocation)
	accumulateInvocationResources(&s.Resources, flaggedInvocation)
	mergeTrace(&s, flaggedTrace)
	s.Resources.OutputBytes += len(flaggedOutput)
	if err != nil {
		s.Detail = "flagged mode: " + err.Error()
		return s
	}
	if !bytes.Equal(normalOutput, flaggedOutput) {
		s.Detail = "normal and flagged complete machine outputs differ"
		return s
	}
	for _, f := range append(append([]fixture(nil), fixtures...), failIfRendered) {
		content, readErr := os.ReadFile(filepath.Join(source, f.name))
		if readErr != nil || !bytes.Equal(content, f.content) {
			s.Detail = "retained protected source corpus changed during executable inspection"
			return s
		}
	}
	var mapped map[string]managedPaths
	if err := json.Unmarshal(flaggedOutput, &mapped); err != nil {
		s.Detail = "managed output was not the expected JSON object"
		return s
	}
	for i := range staticTargets {
		absolute := filepath.Clean(filepath.Join(destination, filepath.FromSlash(staticTargets[i].Target)))
		sourceAbsolute := filepath.Clean(filepath.Join(source, filepath.FromSlash(staticTargets[i].Source)))
		entry, ok := mapped[staticTargets[i].Target]
		if !ok || filepath.Clean(entry.Absolute) != absolute || filepath.Clean(entry.SourceAbsolute) != sourceAbsolute || entry.SourceRelative != staticTargets[i].Source || !within(absolute, destination) || !within(sourceAbsolute, source) {
			s.Detail = "manager mapping disagreed with the complete semantic oracle"
			return s
		}
		staticTargets[i].MappingSource = "byte-identical normal/flagged chezmoi managed JSON"
	}
	if len(mapped) != len(staticTargets) {
		s.Detail = "manager mapping contained an unexpected structural target"
		return s
	}
	templateRenders := bytes.Count(flaggedOutput, []byte("ZERB non-rendering semantic sentinel evaluated"))
	if _, ok := mapped[".must_not_render"]; !ok || templateRenders != 0 {
		s.Detail = "non-rendering sentinel was absent from successful semantic output"
		return s
	}
	zero := 0
	s.Access.RealHome, s.Access.RealXDG, s.Access.RealDestination = &zero, &zero, &zero
	s.Access.HelpersAgentsRelays = &zero
	s.Access.TemplateRenders = &templateRenders
	s.Status = "observed"
	s.Targets = staticTargets
	s.Resources.Entries = len(staticTargets)
	s.Facts = []FactObservation{{Name: "mapping", Value: "complete outputs byte-identical and exact canonical normalized target/source relations", Source: "same pinned executable in normal and flagged modes plus complete semantic oracle"}, {Name: "type_mode_owner_link", Value: "direct-source-validator facts", Source: "approved static filename grammar plus owner-validated source artifacts"}, {Name: "destination_relation", Value: "synthetic-and-real-destination-access=0", Source: "closed file/process/network syscall traces"}, {Name: "render_mutation_relation", Value: "semantic-sentinel-evaluations=0,source-mutations=0,destination-mutations=0", Source: "successful inclusion of deterministically failing-if-evaluated template, exact output equivalence, and byte-for-byte source re-read"}, {Name: "trace_retention", Value: "redacted classes/counts only; raw traces deleted", Source: "trace parser cleanup"}}
	s.Detail = "the exact pinned chezmoi executable produced byte-identical complete machine output in normal and flagged modes; flagged mode remained under a closed host-fact trace and the semantic failure sentinel proved non-rendering"
	return s
}
func runMiseManagerScenario(ctx context.Context, cfg Config, root, dir, archivePath string, inventory archiveInventory, injection archiveInjectionObservation, ambientMarker []byte) Scenario {
	s := Scenario{Name: "mise-node-24.4.1-authored-lock", Status: "failed", RequirementIDs: []string{"VAL-016", "VAL-017", "VAL-018", "VAL-019", "VAL-021", "VAL-023", "PC-031", "PC-043", "PC-050"}, RejectedAssumption: "An authored lock substring, post-install hash, or network-enabled manager extraction proves pre-use archive authentication."}
	lock, err := inspectLock([]byte(cfg.NodeLock))
	if err != nil {
		s.Detail = err.Error() + "; manager invocations=0"
		return s
	}
	env := append(miseEnv(root, dir, cfg), "MISE_OFFLINE=1")
	s.Environment = EnvironmentObservation{Names: envNames(env), AmbientFingerprintAbsent: true}
	s.Access.platformClassCounts = normativeMisePlatformClassCounts()
	commands := []struct {
		name string
		args []string
	}{
		{"mise-install-authenticated-offline", []string{"--locked", "-C", dir, "install", "--raw", "node@24.4.1"}},
		{"mise-inspect-path", []string{"--locked", "-C", dir, "where", "node@24.4.1"}},
		{"mise-inspect-env", []string{"--locked", "-C", dir, "env", "--json", "node@24.4.1"}},
		{"mise-exec", []string{"--locked", "-C", dir, "exec", "node@24.4.1", "--", "node", "--version"}},
	}
	var outputs [][]byte
	var installTrace adapterTrace
	var traces []adapterTrace
	for index, command := range commands {
		policy := tracePolicy{owned: dir, proofRoot: root, writeRoot: dir, cwd: dir, protected: cfg.ProtectedRoots, forbidden: ambientForbidden(root), authenticatedArchive: archivePath, forbiddenMarker: ambientMarker, allowMiseSystemReads: true, platformCgroupRoot: cfg.PlatformCgroupRoot}
		output, invocation, trace, err := invokeTraced(ctx, cfg, command.name, cfg.Mise, command.args, dir, env, policy)
		s.Invocations = append(s.Invocations, invocation)
		accumulateInvocationResources(&s.Resources, invocation)
		mergeTrace(&s, trace)
		traces = append(traces, trace)
		outputs = append(outputs, output)
		s.Resources.OutputBytes += len(output)
		if index == 0 {
			installTrace = trace
		}
		if err != nil {
			s.Detail = command.name + ": " + err.Error()
			return s
		}
	}
	if installTrace.authenticatedArchiveReads == 0 {
		s.Detail = "offline mise install did not open the immutable independently authenticated archive inode"
		return s
	}
	if err := requireNormativeMisePlatformClasses(s.Access.platformClassCounts); err != nil {
		s.Detail = err.Error()
		return s
	}
	if s.Access.RuntimeBootstrapReads != 0 {
		s.Detail = "native mise trace inherited a non-ARCH-013 runtime-bootstrap path class"
		return s
	}
	installPath := strings.TrimSpace(string(outputs[1]))
	if !filepath.IsAbs(installPath) || !within(installPath, filepath.Join(dir, "data")) {
		s.Detail = "mise reported an install path outside the isolated data root"
		return s
	}
	for _, invocationTrace := range traces {
		for _, executable := range invocationTrace.execs {
			if executable != cfg.Mise && executable != cfg.Bash && !within(executable, filepath.Join(dir, "data")) && !within(executable, "/nix/store/di26b1kkbammy0sj70nq5qzvfrh78wxl-coreutils-9.11/bin") && !within(executable, "/nix/store/aak8d9mrdv9sgn0lcg7xss7wxdg9sqh3-gnugrep-3.12/bin") {
				s.Detail = "mise process graph contained an undeclared executable/helper"
				return s
			}
		}
	}
	var requestedEnv map[string]string
	if err := json.Unmarshal(outputs[2], &requestedEnv); err != nil {
		s.Detail = "mise requested environment was not a JSON string map"
		return s
	}
	requestedNames := make([]string, 0, len(requestedEnv))
	for name := range requestedEnv {
		requestedNames = append(requestedNames, name)
	}
	sort.Strings(requestedNames)
	if len(requestedNames) != 1 || requestedNames[0] != "PATH" || !strings.Contains(requestedEnv["PATH"], filepath.Join(installPath, "bin")) {
		s.Detail = "mise requested environment was not the exact installed Node PATH relation"
		return s
	}
	if strings.TrimSpace(string(outputs[3])) != "v24.4.1" {
		s.Detail = "mise exec did not execute exact node v24.4.1"
		return s
	}
	treeEntries, executableDigest, err := compareInstalledArchiveTree(installPath, inventory)
	if err != nil {
		s.Detail = err.Error()
		return s
	}
	zero := 0
	s.Access.RealHome, s.Access.RealXDG, s.Access.RealDestination = &zero, &zero, &zero
	s.Access.HelpersAgentsRelays = &zero
	s.Status = "observed"
	archiveRef := typedRelation("authenticated-node-archive")
	installRef := typedRelation("mise-install-tree")
	s.Facts = []FactObservation{
		{Name: "authored_version", Value: lock.Version, Source: "strict bounded TOML lock inspector"},
		{Name: "backend", Value: lock.Backend, Source: "strict bounded TOML lock inspector"},
		{Name: "archive_relation", Value: archiveRef + " descriptor-size-and-sha256-authenticated=true,inode-stable-through-read-only-remount=true,opened-by-offline-mise=true", Source: "private capped tmpfs, open-descriptor authentication, exact cache rename, sync, read-only remount, and closed successful-open trace"},
		{Name: "immutable_injection", Value: fmt.Sprintf("same-uid=%d,modify-rejected=%t,replace-rejected=%t,remove-rejected=%t", injection.UID, injection.ModifyRejected, injection.ReplaceRejected, injection.RemoveRejected), Source: "post-hash/pre-mise same-UID attack process against read-only archive mount"},
		{Name: "archive_inventory", Value: fmt.Sprintf("single-stripped-root=%s,entries=%d,canonical-and-bounded=true", inventory.Root, len(inventory.Entries)), Source: "independent tar parser over authenticated immutable descriptor bytes"},
		{Name: "install_relation", Value: installRef + " authenticated-archive-semantic-tree-equality=true,inside=declared-data-root", Source: "independent lstat/normalized-mode/content/target comparison; archived symlinks remain exact, materialize as exact authenticated target/sibling content, or use the one frozen mise-2026.7.17 bin/npm wrapper identity"},
		{Name: "executable_identity", Value: "bin/node-sha256:" + executableDigest, Source: "archive inventory and independently hashed installed regular executable"},
		{Name: "platform_class_keys", Value: strings.Join(normativeMisePlatformClasses(), ","), Source: "exact aggregate trace class-key equality assertion; values, including zero observations, are reported in access.platform_classes"},
		{Name: "successful_platform_class_keys", Value: strings.Join(successfulMisePlatformClasses(s.Access.platformClassCounts), ","), Source: "positive-count aggregate closed-trace classes; no PC-031 runtime-bootstrap class is admitted by the native mise policy"},
		{Name: "executed_version", Value: "v24.4.1", Source: "network-disabled mise exec stdout"},
	}
	s.Resources.Entries = treeEntries
	s.Detail = "an unprivileged private user/mount namespace authenticated and inventoried the exact archive descriptor, rejected same-UID post-hash mutation at a read-only mount boundary, and independently matched the installed tree before accepting offline mise output"
	return s
}

func inspectDeclaration(content []byte) error {
	if len(content) == 0 || len(content) > maxLockBytes || !utf8.Valid(content) {
		return errors.New("bounded UTF-8 mise declaration required")
	}
	inTools, seen := false, false
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if line != "[tools]" {
				return errors.New("only scalar tools table is allowed")
			}
			inTools = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !inTools || strings.TrimSpace(key) != "node" || seen || quoted(strings.TrimSpace(value)) != "24.4.1" {
			return errors.New("exact single scalar node 24.4.1 declaration required")
		}
		seen = true
	}
	if !seen {
		return errors.New("node declaration missing")
	}
	return nil
}

func inspectLock(content []byte) (lockFact, error) {
	if len(content) == 0 || len(content) > maxLockBytes || !utf8.Valid(content) {
		return lockFact{}, errors.New("complete bounded UTF-8 authored lock required")
	}
	var document lockDocument
	decoder := toml.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return lockFact{}, errors.New("strict authored lock TOML required")
	}
	if len(document.Tools.Node) != 1 {
		return lockFact{}, errors.New("authored lock requires exactly one tools.node entry")
	}
	node := document.Tools.Node[0]
	platform := node.Linux
	fact := lockFact{Version: node.Version, Backend: node.Backend, Checksum: platform.Checksum, URL: platform.URL, Size: platform.Size}
	archive := "node-v24.4.1-linux-x64.tar.gz"
	if fact.Version != "24.4.1" || fact.Backend != "core:node" || !validChecksum(fact.Checksum) || fact.URL != "https://nodejs.org/dist/v24.4.1/"+archive || fact.Size <= 0 || fact.Size > 512<<20 {
		return lockFact{}, errors.New("authored lock must contain one exact node 24.4.1 core backend and complete bounded linux-x64 size, SHA256, and URL")
	}
	return fact, nil
}

func quoted(value string) string {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return ""
	}
	return value[1 : len(value)-1]
}
func validChecksum(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil && value == strings.ToLower(value)
}

func normalizeStaticTargets(sourceRoot string, fixtures []fixture) ([]TargetObservation, error) {
	seen := make(map[string]struct{}, len(fixtures))
	targets := make([]TargetObservation, 0, len(fixtures))
	for _, f := range fixtures {
		if err := ValidateStatic(f.name, f.content); err != nil {
			return nil, err
		}
		if sourceRoot != "" {
			path := filepath.Join(sourceRoot, f.name)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("source artifact is not an owned regular file")
			}
			stat, ok := st.Sys().(*syscall.Stat_t)
			if !ok || int(stat.Uid) != os.Getuid() {
				return nil, errors.New("source artifact owner mismatch")
			}
			actual, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(actual, f.content) {
				return nil, errors.New("source artifact read mismatch")
			}
		}
		name, mode := f.name, "0666"
		if strings.HasPrefix(name, "private_executable_") {
			name, mode = strings.TrimPrefix(name, "private_executable_"), "0700"
		} else if strings.HasPrefix(name, "private_") {
			name, mode = strings.TrimPrefix(name, "private_"), "0600"
		} else if strings.HasPrefix(name, "executable_") {
			name, mode = strings.TrimPrefix(name, "executable_"), "0777"
		}
		name = strings.TrimSuffix(name, ".tmpl")
		name = strings.ReplaceAll(name, "dot_", ".")
		target := filepath.ToSlash(filepath.Clean(name))
		if target == "." || strings.HasPrefix(target, "../") || filepath.IsAbs(target) {
			return nil, errors.New("invalid normalized target")
		}
		if _, ok := seen[target]; ok {
			return nil, errors.New("duplicate normalized structural target")
		}
		seen[target] = struct{}{}
		targets = append(targets, TargetObservation{Target: target, Source: f.name, Type: "regular-file", Mode: mode, OwnerRelation: "owned-by-disposable-proof-owner", LinkRelation: "not-a-link", AttributeSource: "direct static source validator"})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Target < targets[j].Target })
	return targets, nil
}

func ValidateStatic(name string, content []byte) error {
	base := filepath.Base(name)
	for _, prefix := range []string{"run_", "remove_", "modify_", "symlink_", "encrypted_"} {
		if strings.HasPrefix(base, prefix) {
			return errors.New("forbidden chezmoi entry class")
		}
	}
	for _, prefix := range []string{".chezmoiexternal", ".chezmoidata", ".chezmoiscripts", ".chezmoiignore", ".chezmoiremove"} {
		if strings.HasPrefix(base, prefix) {
			return errors.New("forbidden chezmoi control input")
		}
	}
	if !strings.HasSuffix(base, ".tmpl") {
		return nil
	}
	if !utf8.Valid(content) {
		return errors.New("invalid UTF-8 template")
	}
	trees, err := parse.Parse(base, string(content), "", "")
	if err != nil {
		return fmt.Errorf("parse static template: %w", err)
	}
	if len(trees) != 1 || trees[base] == nil || trees[base].Root == nil {
		return errors.New("named or empty template tree is forbidden")
	}
	return validateStaticList(trees[base].Root, 0)
}
func validateStaticList(list *parse.ListNode, depth int) error {
	if depth > 32 {
		return errors.New("static template nesting exceeds proof bound")
	}
	if list == nil {
		return nil
	}
	for _, raw := range list.Nodes {
		switch node := raw.(type) {
		case *parse.TextNode, *parse.CommentNode:
		case *parse.IfNode:
			if !literalBooleanPipe(node.Pipe) {
				return errors.New("static template if requires one literal boolean")
			}
			if err := validateStaticList(node.List, depth+1); err != nil {
				return err
			}
			if err := validateStaticList(node.ElseList, depth+1); err != nil {
				return err
			}
		default:
			return errors.New("forbidden dynamic/provider/host-fact/template action")
		}
	}
	return nil
}
func literalBooleanPipe(pipe *parse.PipeNode) bool {
	if pipe == nil || len(pipe.Decl) != 0 || pipe.IsAssign || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return false
	}
	_, ok := pipe.Cmds[0].Args[0].(*parse.BoolNode)
	return ok
}

func miseEnv(root, dir string, cfg Config) []string {
	base := adapterEnv(dir, cfg.CACert)
	env := make([]string, 0, len(base)+20)
	for _, value := range base {
		if !strings.HasPrefix(value, "PATH=") {
			env = append(env, value)
		}
	}
	path := strings.Join([]string{filepath.Dir(cfg.Bash), cfg.CoreutilsDir}, string(filepath.ListSeparator))
	return append(env, "PATH="+path, "OPENSSL_CONF=/dev/null", "TZDIR="+filepath.Join(dir, "zoneinfo"), "TMPDIR="+filepath.Join(dir, "tmp"), "TMP="+filepath.Join(dir, "tmp"), "TEMP="+filepath.Join(dir, "tmp"), "MISE_NO_ENV=1", "MISE_NO_HOOKS=1", "MISE_TRUSTED_CONFIG_PATHS="+dir, "MISE_CEILING_PATHS="+root, "MISE_DATA_DIR="+filepath.Join(dir, "data"), "MISE_CACHE_DIR="+filepath.Join(dir, "cache"), "MISE_STATE_DIR="+filepath.Join(dir, "state"), "MISE_SYSTEM_CONFIG_DIR="+filepath.Join(dir, "system-config"), "MISE_SYSTEM_CONFIG_FILE="+filepath.Join(dir, "system-config.toml"), "MISE_SYSTEM_DATA_DIR="+filepath.Join(dir, "system-data"), "MISE_GLOBAL_CONFIG_FILE="+filepath.Join(dir, "global-config.toml"), "MISE_GLOBAL_CONFIG_ROOT="+dir)
}
func adapterEnv(root, ca string) []string {
	env := []string{"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "xdg-config"), "XDG_CONFIG_DIRS=" + filepath.Join(root, "xdg-config-system"), "XDG_CACHE_HOME=" + filepath.Join(root, "xdg-cache"), "XDG_DATA_HOME=" + filepath.Join(root, "xdg-data"), "XDG_DATA_DIRS=" + filepath.Join(root, "xdg-data-system"), "XDG_STATE_HOME=" + filepath.Join(root, "xdg-state"), "XDG_RUNTIME_DIR=" + filepath.Join(root, "xdg-runtime"), "PATH=/nonexistent", "LANG=C", "LC_ALL=C", "TERM=dumb", "TZ=UTC", "GODEBUG=containermaxprocs=0,updatemaxprocs=0", "GOMAXPROCS=1", "GOMEMLIMIT=1073741824", "SHELL=/nonexistent/poisoned-shell", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "SSH_AUTH_SOCK=", "http_proxy=", "https_proxy=", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*"}
	if ca != "" {
		env = append(env, "SSL_CERT_FILE="+ca)
	}
	return env
}
func envNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, value := range env {
		names = append(names, strings.SplitN(value, "=", 2)[0])
	}
	sort.Strings(names)
	return names
}
func sortedEnvironment() []string {
	env := append([]string(nil), os.Environ()...)
	sort.Strings(env)
	return env
}
func canonicalExecutable(name, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%s canonical executable required", name)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return fmt.Errorf("%s executable must be canonical", name)
	}
	return nil
}
func writeExclusive(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateRoot(raw string, protected []string) (string, error) {
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errors.New("explicit absolute disposable root required")
	}
	if len(protected) == 0 {
		return "", errors.New("canonical protected roots captured before environment poisoning are required")
	}
	root, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", err
	}
	for _, path := range protected {
		if path == "" {
			continue
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", fmt.Errorf("canonicalize protected root: %w", err)
		}
		if within(root, path) || within(path, root) {
			return "", errors.New("disposable root overlaps a protected path class")
		}
	}
	return root, nil
}
func within(path, base string) bool {
	relative, err := filepath.Rel(base, path)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
func plantHostileAmbient(root string) error {
	paths := map[string][]byte{
		filepath.Join(root, "home", ".config", "mise", "config.toml"):     []byte("[tools]\nnode = \"0.0.0-hostile\"\n"),
		filepath.Join(root, "home", ".config", "chezmoi", "chezmoi.toml"): []byte("sourceDir = \"/forbidden\"\n"),
		filepath.Join(root, "xdg-config", "mise", "config.toml"):          []byte("[tools]\nnode = \"0.0.0-hostile\"\n"),
		filepath.Join(root, "xdg-config", "chezmoi", "chezmoi.toml"):      []byte("sourceDir = \"/forbidden\"\n"),
		filepath.Join(root, "mise.toml"):                                  []byte("[tools]\nnode = \"0.0.0-hostile\"\n"),
		filepath.Join(root, ".chezmoi.toml"):                              []byte("sourceDir = \"/forbidden\"\n"),
	}
	for path, content := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := writeExclusive(path, content, 0600); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "xdg-config-system"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "xdg-data-system"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "xdg-runtime"), 0700); err != nil {
		return err
	}
	helperRoot := filepath.Join(root, "hostile-helpers")
	if err := os.MkdirAll(helperRoot, 0700); err != nil {
		return err
	}
	for _, name := range []string{"age", "bash", "bw", "git", "gpg", "op", "pass", "sh", "ssh"} {
		if err := writeExclusive(filepath.Join(helperRoot, name), []byte("#!/bin/false\n"), 0700); err != nil {
			return err
		}
	}
	return nil
}

func treeContainsMarker(root string, marker []byte) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if found || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		contains, scanErr := readerContainsMarker(file, marker)
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		found = contains
		return nil
	})
	return found, err
}

func readerContainsMarker(reader io.Reader, marker []byte) (bool, error) {
	if len(marker) == 0 {
		return false, errors.New("empty marker")
	}
	buffer := make([]byte, 64<<10)
	tail := make([]byte, 0, len(marker)-1)
	for {
		count, err := reader.Read(buffer)
		window := append(tail, buffer[:count]...)
		if bytes.Contains(window, marker) {
			return true, nil
		}
		keep := len(marker) - 1
		if keep > len(window) {
			keep = len(window)
		}
		tail = append(tail[:0], window[len(window)-keep:]...)
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func ambientForbidden(root string) map[string]string {
	return map[string]string{
		filepath.Join(root, "home", ".config", "mise", "config.toml"):     "hostile-home-manager-config",
		filepath.Join(root, "home", ".config", "chezmoi", "chezmoi.toml"): "hostile-home-manager-config",
		filepath.Join(root, "xdg-config", "mise", "config.toml"):          "hostile-xdg-manager-config",
		filepath.Join(root, "xdg-config", "chezmoi", "chezmoi.toml"):      "hostile-xdg-manager-config",
		filepath.Join(root, "mise.toml"):                                  "hostile-project-manager-config",
		filepath.Join(root, ".chezmoi.toml"):                              "hostile-project-manager-config",
		filepath.Join(root, "hostile-helpers"):                            "helper-agent-or-relay-channel",
		"/etc/chezmoi":                                                    "root-system-manager-control",
		"/etc/mise":                                                       "root-system-manager-control",
		"/etc/xdg/chezmoi":                                                "root-xdg-manager-control",
		"/etc/xdg/mise":                                                   "root-xdg-manager-control",
		"/root/.local/share/chezmoi":                                      "root-system-manager-control",
		"/usr/local/etc/mise":                                             "root-system-manager-control",
		"/root/.config/chezmoi":                                           "root-xdg-manager-control",
		"/root/.config/mise":                                              "root-xdg-manager-control",
	}
}

func typedRelation(kind string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("cryptographic random source unavailable")
	}
	return "typed:" + kind + ":" + hex.EncodeToString(b)
}

func mergeTrace(s *Scenario, trace adapterTrace) {
	s.Access.TraceAvailable = true
	s.Writes.TraceAvailable = true
	s.Access.FileEvents += trace.fileEvents
	s.Access.ProcessEvents += trace.processEvents
	s.Access.SocketEvents += trace.socketEvents
	s.Access.UnknownEvents += trace.unknownEvents
	for class, count := range trace.unknownClasses {
		s.Access.UnknownClasses = append(s.Access.UnknownClasses, fmt.Sprintf("%s=%d", class, count))
	}
	sort.Strings(s.Access.UnknownClasses)
	if s.Access.syscallClassCounts == nil {
		s.Access.syscallClassCounts = make(map[string]int)
	}
	for class, count := range trace.reviewedClasses {
		s.Access.syscallClassCounts[class] += count
	}
	s.Access.SyscallClasses = s.Access.SyscallClasses[:0]
	for class, count := range s.Access.syscallClassCounts {
		s.Access.SyscallClasses = append(s.Access.SyscallClasses, fmt.Sprintf("%s=%d", class, count))
	}
	sort.Strings(s.Access.SyscallClasses)
	for class, count := range trace.reads {
		if strings.HasPrefix(class, "runtime-bootstrap-") {
			s.Access.RuntimeBootstrapReads += count
		}
	}
	network := valueOrZero(s.Access.NetworkAttempts) + trace.networkAttempts
	s.Access.NetworkAttempts = &network
	outside := valueOrZero(s.Writes.OutsideOwnedRoot) + trace.outsideWrites
	s.Writes.OutsideOwnedRoot = &outside
	classes := make(map[string]bool)
	for _, class := range s.Writes.Paths {
		classes[class] = true
	}
	hostFacts := valueOrZero(s.Access.ProvidersHostFacts) + trace.hostFactAccesses
	s.Access.ProvidersHostFacts = &hostFacts
	platform := valueOrZero(s.Access.PlatformObservations) + trace.platformObservations
	s.Access.PlatformObservations = &platform
	if s.Access.platformClassCounts == nil {
		s.Access.platformClassCounts = make(map[string]int)
	}
	for class, count := range trace.platformClasses {
		s.Access.platformClassCounts[class] += count
	}
	s.Access.PlatformClasses = s.Access.PlatformClasses[:0]
	for class, count := range s.Access.platformClassCounts {
		s.Access.PlatformClasses = append(s.Access.PlatformClasses, fmt.Sprintf("%s=%d", class, count))
	}
	sort.Strings(s.Access.PlatformClasses)
	for class, count := range trace.writes {
		if count > 0 {
			classes[class] = true
		}
	}
	s.Writes.Paths = s.Writes.Paths[:0]
	for class := range classes {
		s.Writes.Paths = append(s.Writes.Paths, class)
	}
	sort.Strings(s.Writes.Paths)
	deniedClasses := make(map[string]bool)
	for _, class := range s.Writes.DeniedPaths {
		deniedClasses[class] = true
	}
	for class, count := range trace.deniedWrites {
		if count > 0 {
			deniedClasses[class] = true
		}
	}
	s.Writes.DeniedPaths = s.Writes.DeniedPaths[:0]
	for class := range deniedClasses {
		s.Writes.DeniedPaths = append(s.Writes.DeniedPaths, class)
	}
	sort.Strings(s.Writes.DeniedPaths)
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func accumulateInvocationResources(resources *ResourceObservation, invocation InvocationObservation) {
	if !invocation.Invoked {
		return
	}
	complete := invocation.CgroupObserved && (invocation.CgroupEmpty || invocation.CgroupCleanupDeferred) && invocation.CgroupKillGuard && invocation.IOUringDenied && invocation.MemoryOOM == 0 && invocation.OOMKill == 0
	if resources.CgroupInvocations == 0 {
		resources.CgroupComplete = complete
	} else {
		resources.CgroupComplete = resources.CgroupComplete && complete
	}
	resources.CgroupInvocations++
	if invocation.MemoryPeakBytes > resources.MemoryPeakBytes {
		resources.MemoryPeakBytes = invocation.MemoryPeakBytes
	}
	if invocation.ProcessPeak > resources.ProcessPeak {
		resources.ProcessPeak = invocation.ProcessPeak
	}
	resources.CPUUsec += invocation.CPUUsec
	resources.MemoryOOM += invocation.MemoryOOM
	resources.OOMKill += invocation.OOMKill
}
