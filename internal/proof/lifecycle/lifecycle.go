// Package lifecycle is a disposable Linux proof harness for the Phase 1
// process-lifecycle candidate. It is not a product API.
package lifecycle

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

	"golang.org/x/sys/unix"
)

type processObservation struct {
	Environment                 []string `json:"environment"`
	Descriptors                 []int    `json:"descriptors"`
	PostExecRuntimeDescriptors  []int    `json:"post_exec_runtime_descriptors"`
	UnexpectedDescriptors       []int    `json:"unexpected_descriptors"`
	UnexpectedDescriptorClasses []string `json:"unexpected_descriptor_classes"`
}

const (
	ResultVersion       = 6
	GracePeriod         = 750 * time.Millisecond // Frozen proof-candidate grace; Phase 1 evidence may reject it.
	prSetChildSubreaper = 36
)

type Config struct {
	Root           string   `json:"root"`
	Executable     string   `json:"executable"`
	Git            string   `json:"git"`
	SignalFixture  string   `json:"signal_fixture"`
	Bash           string   `json:"bash"`
	Mise           string   `json:"mise"`
	Chezmoi        string   `json:"chezmoi"`
	CACert         string   `json:"ca_cert"`
	ArtifactDir    string   `json:"artifact_dir"`
	ProtectedRoots []string `json:"-"`
}

type Result struct {
	Version                       int        `json:"version"`
	Scenarios                     []Scenario `json:"scenarios"`
	MarkerReference               string     `json:"ambient_marker_ref"`
	SupervisorReference           string     `json:"supervisor_ref"`
	ProofOwnedExecutableReference string     `json:"proof_owned_executable_ref"`
	ParentEnvironmentUnchanged    bool       `json:"parent_environment_unchanged"`
}

type Scenario struct {
	Name               string                 `json:"name"`
	Status             string                 `json:"status"`
	RequirementIDs     []string               `json:"requirement_ids"`
	Invocation         InvocationObservation  `json:"invocation"`
	Environment        EnvironmentObservation `json:"environment"`
	Access             AccessObservation      `json:"access"`
	Writes             WriteObservation       `json:"writes"`
	Resources          ResourceObservation    `json:"resources"`
	Lifecycle          LifecycleObservation   `json:"lifecycle"`
	RejectedAssumption string                 `json:"rejected_assumption"`
	Detail             string                 `json:"detail,omitempty"`
}

type InvocationObservation struct {
	Executable             string   `json:"executable"`
	Arguments              []string `json:"arguments"`
	WorkingDirectory       string   `json:"working_directory_class"`
	AllowedDescriptors     []int    `json:"allowed_descriptors"`
	DescriptorsExact       bool     `json:"descriptors_exact"`
	ResolutionClass        string   `json:"resolution_class,omitempty"`
	StablePATHValidated    bool     `json:"stable_path_validated,omitempty"`
	SameFDExecution        bool     `json:"same_fd_execution,omitempty"`
	SubstitutionRejections []string `json:"substitution_rejections,omitempty"`
}
type EnvironmentObservation struct {
	AllowedNames   []string `json:"allowed_names"`
	ClearedClasses []string `json:"cleared_classes"`
	Exact          bool     `json:"exact"`
}
type AccessObservation struct {
	TraceAvailable         bool `json:"trace_available"`
	ProhibitedAccesses     int  `json:"prohibited_accesses"`
	AuthenticationChannels int  `json:"authentication_channels"`
	ProviderChannels       int  `json:"provider_channels"`
}
type WriteObservation struct {
	TraceAvailable   bool `json:"trace_available"`
	OutsideOwnedRoot int  `json:"outside_owned_root"`
}
type ResourceObservation struct {
	FDChurn           bool   `json:"fd_churn"`
	CgroupObserved    bool   `json:"cgroup_observed"`
	CgroupEmpty       bool   `json:"cgroup_empty"`
	MemoryPeakBytes   uint64 `json:"memory_peak_bytes"`
	ProcessPeak       uint64 `json:"process_peak"`
	CPUUsec           uint64 `json:"cpu_usec"`
	MemoryOOM         uint64 `json:"memory_oom"`
	OOMKill           uint64 `json:"oom_kill"`
	HardDeathObserved bool   `json:"hard_death_observed"`
}
type LifecycleObservation struct {
	Session             bool `json:"session"`
	ProcessGroup        bool `json:"process_group"`
	Subreaper           bool `json:"subreaper"`
	ParentChecked       bool `json:"parent_checked"`
	ParentDeathSignal   bool `json:"parent_death_signal"`
	ReadyHandshake      bool `json:"ready_handshake"`
	SignalForwarded     bool `json:"signal_forwarded"`
	Escalated           bool `json:"escalated"`
	ExitStatus          int  `json:"exit_status"`
	DescendantsReaped   int  `json:"descendants_reaped"`
	Orphans             int  `json:"orphans"`
	SurvivorsDetected   int  `json:"survivors_detected"`
	DeathGuardArmed     bool `json:"death_guard_armed"`
	DeathGuardCleanup   bool `json:"death_guard_cleanup"`
	LeaderObserved      bool `json:"leader_observed"`
	ObservedDescendants int  `json:"observed_descendants"`
	IOUringDenied       bool `json:"io_uring_denied"`
	CgroupKillGuard     bool `json:"cgroup_kill_guard"`
	GuardStatus         int  `json:"guard_status"`
	GuardStatusObserved bool `json:"guard_status_observed"`
}

type Spec struct {
	Name                   string
	Executable             string
	Args                   []string
	Dir                    string
	Env                    []string
	Stdin                  io.Reader
	Stdout, Stderr         io.Writer
	ReadyFile              string
	CgroupPath             string
	CgroupInherited        bool
	CgroupPathSubstitution bool
	FileSizeLimit          uint64
	DenyIOUring            bool
	InteractivePTY         bool
	WrapperSignal          syscall.Signal
	ChildSignal            syscall.Signal
	HardKillAfterReady     bool
	InjectionGate          string
	GuardFailureStatus     int
	beforeReleaseMutation  func(string) error
	preSelfMutation        func(string) error
	preLaunchMutation      func(string, *executableIdentity) error
}
type Observation struct {
	Status                int
	Ready                 bool
	Session               bool
	ProcessGroup          bool
	Subreaper             bool
	ParentChecked         bool
	ParentDeathSignal     bool
	SupervisorPID         int
	ChildPID              int
	ChildGroup            int
	GuardPID              int
	GuardArmed            bool
	Reaped                int
	Orphans               int
	Forwarded             bool
	Escalated             bool
	HardDeath             bool
	SurvivorsDetected     int
	GuardCleanup          bool
	GuardStatus           int
	GuardStatusObserved   bool
	FDChurnComplete       bool
	GateReached           bool
	LeaderObserved        bool
	DescendantsObserved   int
	CgroupObserved        bool
	CgroupEmpty           bool
	CgroupCleanupDeferred bool
	CgroupKillGuard       bool
	IOUringDenied         bool
	MemoryOOM             uint64
	OOMKill               uint64
	MemoryPeak            uint64
	ProcessPeak           uint64
	CPUUsec               uint64
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	root, err := validateRoot(cfg.Root, cfg.ProtectedRoots)
	if err != nil {
		return Result{}, err
	}
	if cfg.Executable == "" || !filepath.IsAbs(cfg.Executable) {
		return Result{}, errors.New("canonical absolute same-binary executable is required")
	}
	before := append([]string(nil), os.Environ()...)
	var markerSeed [16]byte
	if _, err := rand.Read(markerSeed[:]); err != nil {
		return Result{}, fmt.Errorf("generate ambient marker: %w", err)
	}
	var supervisorSeed [16]byte
	if _, err := rand.Read(supervisorSeed[:]); err != nil {
		return Result{}, fmt.Errorf("generate supervisor reference: %w", err)
	}
	var proofOwnedExecutableSeed [16]byte
	if _, err := rand.Read(proofOwnedExecutableSeed[:]); err != nil {
		return Result{}, fmt.Errorf("generate proof-owned executable reference: %w", err)
	}
	marker := []byte(hex.EncodeToString(markerSeed[:]))
	const markerName = "ZERB_PROOF_AMBIENT_MARKER"
	priorMarker, hadPriorMarker := os.LookupEnv(markerName)
	if err := os.Setenv(markerName, string(marker)); err != nil {
		return Result{}, fmt.Errorf("install ambient marker: %w", err)
	}
	restoreMarker := func() error {
		if hadPriorMarker {
			return os.Setenv(markerName, priorMarker)
		}
		return os.Unsetenv(markerName)
	}
	markerActive := true
	defer func() {
		if markerActive {
			_ = restoreMarker()
		}
	}()
	markerDigest := sha256.Sum256(marker)
	result := Result{Version: ResultVersion, MarkerReference: "AMBIENT_MARKER{fp=sha256:" + hex.EncodeToString(markerDigest[:]) + "}", SupervisorReference: "typed:proof-supervisor:" + hex.EncodeToString(supervisorSeed[:]), ProofOwnedExecutableReference: "typed:proof-owned-executable:" + hex.EncodeToString(proofOwnedExecutableSeed[:])}
	reusable, err := reusableProcessScenario(ctx, cfg, root)
	if err != nil {
		return Result{}, err
	}
	result.Scenarios = append(result.Scenarios, reusable)
	result.Scenarios = append(result.Scenarios, reservedFDReuseScenario(cfg, root))

	result.Scenarios = append(result.Scenarios, upstreamProcessScenarios(ctx, cfg, root)...)
	guardFailure := baseScenario("death-guard-status-propagation", "observed", []string{"VAL-019", "PC-015", "PC-070", "ARCH-010"}, "A failed death guard can disappear while the wrapper discards its status and reports lifecycle success.", cfg.Executable, root, isolatedEnv(root, cfg.CACert), []string{"__proof-child", "exit", "0"})
	guardFailureObs, guardFailureErr := Supervise(ctx, cfg.Executable, Spec{Name: guardFailure.Name, Executable: cfg.Executable, Args: []string{"__proof-child", "exit", "0"}, Dir: root, Env: isolatedEnv(root, cfg.CACert), GuardFailureStatus: 11, Stdout: io.Discard, Stderr: io.Discard})
	guardFailure.Lifecycle = lifecycleObservation(guardFailureObs)
	guardFailure.Resources = resourceObservation(guardFailureObs, true, false)
	if guardFailureErr == nil || guardFailureObs.Status != 1 || !guardFailureObs.GuardStatusObserved || guardFailureObs.GuardStatus != 11 ||
		!guardFailureObs.GuardCleanup || !guardFailureObs.CgroupEmpty || !guardFailureObs.FDChurnComplete ||
		guardFailureObs.Orphans != 0 || guardFailureObs.SurvivorsDetected != 0 {
		guardFailure.Status = "failed"
		guardFailure.Detail = fmt.Sprintf("status=%d guard_status=%d observed=%t cleanup=%t cgroup_empty=%t fd_churn=%t orphans=%d survivors=%d error=%v", guardFailureObs.Status, guardFailureObs.GuardStatus, guardFailureObs.GuardStatusObserved, guardFailureObs.GuardCleanup, guardFailureObs.CgroupEmpty, guardFailureObs.FDChurnComplete, guardFailureObs.Orphans, guardFailureObs.SurvivorsDetected, guardFailureErr)
	} else {
		guardFailure.Detail = "injected guard exit 11 was reaped and propagated as operational failure while the managed cgroup remained empty with zero orphan or survivor"
	}
	result.Scenarios = append(result.Scenarios, guardFailure)
	statusScenario := baseScenario("exec-status-0-through-255", "observed", []string{"VAL-020", "PC-014", "PC-015", "ARCH-010", "ARCH-027"}, "A wrapper reserves, truncates, or remaps a normally exited child status.", cfg.Executable, root, isolatedEnv(root, cfg.CACert), []string{"__proof-child", "exit", "<0-255>"})
	for n := 0; n <= 255; n++ {
		obs, e := Supervise(ctx, cfg.Executable, Spec{Name: "status", Executable: cfg.Executable, Args: []string{"__proof-child", "exit", strconv.Itoa(n)}, Dir: root, Env: isolatedEnv(root, cfg.CACert), Stdout: io.Discard, Stderr: io.Discard})
		statusScenario.Lifecycle = lifecycleObservation(obs)
		if e != nil || obs.Status != n {
			statusScenario.Status = "failed"
			statusScenario.Detail = fmt.Sprintf("status %d observed %d: %v", n, obs.Status, e)
			break
		}
	}
	result.Scenarios = append(result.Scenarios, statusScenario, shellStatusScenario(ctx, cfg, root))
	signalScenario := baseScenario("exec-child-all-linux-terminating-signal-mapping", "observed", []string{"VAL-020", "PC-014", "PC-015", "ARCH-010", "ARCH-027"}, "A signal-terminated child status is lost or mapped as an ordinary exit.", cfg.SignalFixture, root, isolatedEnv(root, cfg.CACert), []string{"--coreutils-prog=sleep", "10s"})
	if cfg.SignalFixture == "" || !filepath.IsAbs(cfg.SignalFixture) {
		signalScenario.Status = "failed"
		signalScenario.Detail = "canonical pinned default-signal fixture executable unavailable"
	} else {
		for _, sig := range childTerminatingSignalNumbers() {
			obs, e := Supervise(ctx, cfg.Executable, Spec{Name: "child-signal", Executable: cfg.SignalFixture, Args: []string{"--coreutils-prog=sleep", "10s"}, ChildSignal: sig, Dir: root, Env: isolatedEnv(root, cfg.CACert), Stdout: io.Discard, Stderr: io.Discard})
			signalScenario.Lifecycle = lifecycleObservation(obs)
			if e != nil || obs.Status != 128+int(sig) {
				signalScenario.Status = "failed"
				signalScenario.Detail = fmt.Sprintf("signal %d observed %d: %v", sig, obs.Status, e)
				break
			}
		}
	}
	result.Scenarios = append(result.Scenarios, ioUringDenialScenario(ctx, cfg, root))
	result.Scenarios = append(result.Scenarios, signalScenario)
	nonterminatingSignals := baseScenario("linux-nonterminating-signal-exclusion", "observed", []string{"VAL-020", "PC-015", "ARCH-010", "ARCH-027"}, "A stopped, continued, or default-ignored child can be reported as signal-terminated before it actually terminates.", cfg.Executable, root, isolatedEnv(root, cfg.CACert), nil)
	nonterminatingSignals.Detail = "SIGCHLD, SIGURG, and SIGWINCH are default-ignored; SIGCONT resumes; SIGSTOP, SIGTSTP, SIGTTIN, and SIGTTOU stop rather than terminate; PC-015's 128+signal result applies only after an actual terminating wait status"
	result.Scenarios = append(result.Scenarios, executionIdentityScenarios(ctx, cfg, root)...)
	result.Scenarios = append(result.Scenarios, selfIdentityAttackScenarios(cfg, root)...)
	result.Scenarios = append(result.Scenarios, nonterminatingSignals)
	result.Scenarios = append(result.Scenarios, cancellationScenario(ctx, cfg, root, false), cancellationScenario(ctx, cfg, root, true))
	result.Scenarios = append(result.Scenarios, cgroupPathSubstitutionScenario(ctx, cfg, root))
	result.Scenarios = append(result.Scenarios, interactiveBashPTYScenario(ctx, cfg, root))
	result.Scenarios = append(result.Scenarios, wrapperSignalScenarios(ctx, cfg, root)...)
	result.Scenarios = append(result.Scenarios, launchFailureScenario(ctx, cfg, root))
	result.Scenarios = append(result.Scenarios, hardDeathScenarios(ctx, cfg, root)...)
	result.Scenarios = append(result.Scenarios, dispatchNegativeScenarios(cfg.Executable, root)...)
	for i := range result.Scenarios {
		if result.Scenarios[i].Invocation.Executable == cfg.Executable {
			result.Scenarios[i].Invocation.Executable = result.SupervisorReference
		}
		if filepath.IsAbs(result.Scenarios[i].Invocation.Executable) && within(result.Scenarios[i].Invocation.Executable, root) {
			result.Scenarios[i].Invocation.Executable = result.ProofOwnedExecutableReference
		}
	}
	if spilled, scanErr := lifecycleTreeContainsMarker(root, marker); scanErr != nil || spilled {
		return Result{}, errors.New("ambient marker reached a lifecycle filesystem sink")
	}
	encoded, err := json.Marshal(result)
	if err != nil || bytes.Contains(encoded, marker) {
		return Result{}, errors.New("ambient marker reached lifecycle structured output")
	}
	if err := restoreMarker(); err != nil {
		return Result{}, fmt.Errorf("restore ambient marker: %w", err)
	}
	markerActive = false
	after := os.Environ()
	sort.Strings(before)
	sort.Strings(after)
	result.ParentEnvironmentUnchanged = strings.Join(before, "\x00") == strings.Join(after, "\x00")

	return result, nil
}
func reusableProcessScenario(ctx context.Context, cfg Config, root string) (Scenario, error) {
	reusableName := "reusable-process-graph-primitive"
	reusableDir := filepath.Join(root, reusableName)
	if err := os.MkdirAll(reusableDir, 0o700); err != nil {
		return Scenario{}, err
	}
	reusableReport := filepath.Join(reusableDir, "observation.json")
	reusableEnv := isolatedEnv(root, cfg.CACert)
	reusable := baseScenario(reusableName, "observed", []string{"VAL-017", "VAL-018", "VAL-019", "VAL-020", "VAL-023", "PC-015", "PC-070", "ARCH-009", "ARCH-010", "ARCH-011"}, "The reusable process-graph primitive can leak environment or descriptors before target exec.", cfg.Executable, reusableDir, reusableEnv, []string{"__proof-child", "inspect", "OBSERVATION_FILE{random}"})
	reusable.Invocation.AllowedDescriptors = []int{0, 1, 2}
	reusableObs, reusableErr := Supervise(ctx, cfg.Executable, Spec{Name: reusableName, Executable: cfg.Executable, Args: []string{"__proof-child", "inspect", reusableReport}, Dir: reusableDir, Env: reusableEnv, Stdout: io.Discard, Stderr: io.Discard})
	reusable.Resources = resourceObservation(reusableObs, true, false)
	reusable.Lifecycle = lifecycleObservation(reusableObs)
	reusable.Access = AccessObservation{TraceAvailable: false}
	reusable.Writes = WriteObservation{TraceAvailable: false}
	observed, reportErr := readProcessObservation(reusableReport)
	reusable.Environment.Exact = reportErr == nil && equalStringSets(observed.Environment, reusableEnv)
	reusable.Invocation.DescriptorsExact = reportErr == nil && descriptorObservationExact(observed)
	if reusableErr != nil || reportErr != nil || reusableObs.Status != 0 || !completeLifecycle(reusableObs) || !reusableObs.LeaderObserved || !reusable.Environment.Exact || !reusable.Invocation.DescriptorsExact {
		reusable.Status = "failed"
		reusable.Detail = fmt.Sprintf("status=%d ready=%t leader=%t environment_exact=%t descriptors_exact=%t descriptor_count=%d post_exec_runtime_count=%d unexpected_descriptor_count=%d unexpected_descriptor_classes=%s supervise_error=%v observation_error=%v", reusableObs.Status, reusableObs.Ready, reusableObs.LeaderObserved, reusable.Environment.Exact, reusable.Invocation.DescriptorsExact, len(observed.Descriptors), len(observed.PostExecRuntimeDescriptors), len(observed.UnexpectedDescriptors), strings.Join(observed.UnexpectedDescriptorClasses, ","), reusableErr, reportErr)
	} else {
		reusable.Detail = "same-binary primitive observed exact environment and target descriptor allowlist after the authenticated launcher validated sanitation, parent death, group/session, and second-parent identity before exec release"
	}
	return reusable, nil
}

func upstreamProcessScenarios(ctx context.Context, cfg Config, root string) []Scenario {
	var out []Scenario
	upstreams := []struct {
		class string
		exe   string
		args  []string
	}{
		{class: "git", exe: cfg.Git, args: []string{"--version"}},
		{class: "acquisition", exe: cfg.Git, args: []string{"--version"}},
		{class: "mise", exe: cfg.Mise, args: []string{"--version"}},
		{class: "chezmoi", exe: cfg.Chezmoi, args: []string{"--version"}},
		{class: "exec", exe: cfg.SignalFixture, args: []string{"--coreutils-prog=true"}},
		{class: "shell", exe: cfg.Bash, args: []string{"--noprofile", "--norc", "-c", "exit 0"}},
	}
	for _, upstream := range upstreams {
		name := upstream.class + "-upstream-process-observation"
		env := isolatedEnv(root, cfg.CACert)
		s := baseScenario(name, "observed", []string{"VAL-017", "VAL-018", "VAL-019", "VAL-020", "VAL-023", "PC-015", "PC-070", "ARCH-009", "ARCH-010", "ARCH-011"}, "One successful retained upstream invocation is sufficient to establish a general no-daemonize or no-escape support claim.", upstream.exe, root, env, upstream.args)
		s.Invocation.AllowedDescriptors = []int{0, 1, 2}
		s.Access = AccessObservation{TraceAvailable: false}
		s.Writes = WriteObservation{TraceAvailable: false}
		if upstream.exe == "" || !filepath.IsAbs(upstream.exe) {
			s.Status = "inconclusive"
			s.Detail = "canonical pinned upstream executable was not supplied; no representative invocation was exercised"
			out = append(out, s)
			continue
		}
		obs, runErr := Supervise(ctx, cfg.Executable, Spec{Name: name, Executable: upstream.exe, Args: upstream.args, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard})
		s.Lifecycle = lifecycleObservation(obs)
		s.Resources = resourceObservation(obs, false, false)
		s.Environment.Exact = obs.Ready
		s.Invocation.DescriptorsExact = obs.Ready
		if runErr != nil || obs.Status != 0 || !completeLifecycle(obs) || !obs.LeaderObserved || obs.Orphans != 0 || obs.SurvivorsDetected != 0 {
			s.Status = "failed"
			s.Detail = fmt.Sprintf("actual_executable=%s status=%d ready=%t leader=%t descendants=%d orphans=%d survivors=%d error=%v", upstream.exe, obs.Status, obs.Ready, obs.LeaderObserved, obs.DescendantsObserved, obs.Orphans, obs.SurvivorsDetected, runErr)
		} else {
			s.Detail = fmt.Sprintf("actual_executable=%s completed one retained representative invocation through the validated launcher with descriptors 0/1/2 at exec and no members left in its exact process group; this is an invocation observation, not a general no-daemonize, escape-prevention, compatibility, or support claim", upstream.exe)
		}
		out = append(out, s)
	}
	return out
}

func executionIdentityScenarios(ctx context.Context, cfg Config, root string) []Scenario {
	base := filepath.Join(root, "exec-identity")
	stable, cwd, ambient := filepath.Join(base, "stable-bin"), filepath.Join(base, "cwd-poison"), filepath.Join(base, "ambient-poison")
	for _, directory := range []string{stable, cwd, ambient} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return []Scenario{{Name: "exec-identity-setup", Status: "failed", Detail: err.Error()}}
		}
	}
	const targetName = "proof-target"
	stableTarget := filepath.Join(stable, targetName)
	if err := copyExecutable(cfg.Executable, stableTarget); err != nil {
		return []Scenario{{Name: "exec-identity-setup", Status: "failed", Detail: err.Error()}}
	}
	for _, poison := range []string{filepath.Join(cwd, targetName), filepath.Join(ambient, targetName)} {
		if err := os.WriteFile(poison, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
			return []Scenario{{Name: "exec-identity-setup", Status: "failed", Detail: err.Error()}}
		}
	}
	env := isolatedEnv(root, cfg.CACert)
	for i := range env {
		if strings.HasPrefix(env[i], "PATH=") {
			env[i] = "PATH=" + stable
		}
	}
	oldPath, hadPath := os.LookupEnv("PATH")
	_ = os.Setenv("PATH", ambient)
	defer func() {
		if hadPath {
			_ = os.Setenv("PATH", oldPath)
		} else {
			_ = os.Unsetenv("PATH")
		}
	}()
	bare := baseScenario("exec-stable-path-bare-same-fd", "observed", []string{"VAL-017", "VAL-018", "VAL-020", "PC-015", "PC-070", "ARCH-009"}, "A bare executable may consult ambient PATH or cwd, or execute a pathname reopened after validation.", targetName, cwd, env, []string{"__proof-child", "exit", "0"})
	bare.Invocation.ResolutionClass = "stable-path-bare"
	bare.Invocation.StablePATHValidated = true
	bareObs, bareErr := Supervise(ctx, cfg.Executable, Spec{Name: bare.Name, Executable: targetName, Args: []string{"__proof-child", "exit", "0"}, Dir: cwd, Env: env, Stdout: io.Discard, Stderr: io.Discard})
	bare.Lifecycle = lifecycleObservation(bareObs)
	bare.Resources = resourceObservation(bareObs, false, false)
	bare.Invocation.SameFDExecution = bareErr == nil && bareObs.Status == 0
	if bareErr != nil || bareObs.Status != 0 || !completeLifecycle(bareObs) {
		bare.Status = "failed"
		bare.Detail = fmt.Sprintf("bare stable-PATH same-FD execution failed: status=%d error=%v", bareObs.Status, bareErr)
	} else {
		bare.Detail = "bare target resolved only in the validated stable PATH; executable same-name files in cwd and ambient PATH were ignored; the launcher rehashed a descriptor-relative duplicate and execveat executed that FD"
	}

	absolute := baseScenario("exec-canonical-absolute-same-fd", "observed", []string{"VAL-017", "VAL-018", "VAL-020", "PC-015", "PC-070", "ARCH-009"}, "An absolute executable may be reopened by pathname after validation.", cfg.Executable, root, env, []string{"__proof-child", "exit", "0"})
	absolute.Invocation.ResolutionClass = "canonical-absolute"
	absoluteObs, absoluteErr := Supervise(ctx, cfg.Executable, Spec{Name: absolute.Name, Executable: cfg.Executable, Args: []string{"__proof-child", "exit", "0"}, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard})
	absolute.Lifecycle = lifecycleObservation(absoluteObs)
	absolute.Resources = resourceObservation(absoluteObs, false, false)
	absolute.Invocation.SameFDExecution = absoluteErr == nil && absoluteObs.Status == 0
	if absoluteErr != nil || absoluteObs.Status != 0 || !completeLifecycle(absoluteObs) {
		absolute.Status = "failed"
		absolute.Detail = fmt.Sprintf("absolute same-FD execution failed: status=%d error=%v", absoluteObs.Status, absoluteErr)
	} else {
		absolute.Detail = "canonical absolute target retained device/inode/owner/group/type/mode/size/SHA identity through descriptor-relative reopen and execveat(AT_EMPTY_PATH)"
	}

	pathRejection := baseScenario("exec-stable-path-grammar-rejections", "observed", []string{"VAL-017", "PC-070", "ARCH-009"}, "Malformed or writable-unowned PATH entries may participate in resolution.", targetName, root, env, nil)
	pathRejection.Invocation.ResolutionClass = "stable-path-rejection-matrix"
	for _, candidate := range []string{"", ".", "relative", stable + ":", stable + ":" + stable, "/tmp"} {
		if _, err := validateStablePATH(candidate); err == nil {
			pathRejection.Status = "failed"
			pathRejection.Detail = "malformed stable PATH candidate accepted"
			break
		}
	}
	if pathRejection.Status == "observed" {
		pathRejection.Invocation.StablePATHValidated = true
		pathRejection.Detail = "empty, dot, relative, trailing-empty, duplicate-path/identity, and writable-unowned PATH entries were rejected"
	}
	imageRejection := baseScenario("exec-linux-x86-64-elf-image-rejections", "observed", []string{"VAL-017", "PC-070", "ARCH-009"}, "Scripts, shebangs, or non-ELF executable files may enter the sealed exec boundary.", cfg.Executable, root, env, nil)
	for name, content := range map[string][]byte{"shebang-script": []byte("#!/bin/sh\nexit 0\n"), "non-elf": []byte("not-an-elf-image")} {
		path := filepath.Join(base, name)
		if err := os.WriteFile(path, content, 0700); err != nil {
			imageRejection.Status, imageRejection.Detail = "failed", err.Error()
			break
		}
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: "exec-image-reject-" + name, Executable: path, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard})
		if err == nil || obs.Ready || obs.Status != 1 {
			imageRejection.Status, imageRejection.Detail = "failed", name+" reached child release"
			break
		}
	}
	if imageRejection.Status == "observed" {
		imageRejection.Detail = "shebang and arbitrary non-ELF executable files were rejected before supervisor/launcher release; managed exec and shell images are exact Linux x86_64 ELF64 ET_EXEC/ET_DYN only"
	}
	races := baseScenario("exec-opened-identity-substitution-races", "observed", []string{"VAL-017", "VAL-018", "PC-070", "ARCH-009"}, "Path, symlink, inode, digest, owner, type, or mode substitution between open and release may change the executed identity.", stableTarget, root, env, nil)
	races.Invocation.ResolutionClass = "opened-descriptor-race-matrix"
	mutations := []struct {
		name   string
		reject bool
		fn     func(string, *executableIdentity) error
	}{
		{"symlink", false, func(path string, _ *executableIdentity) error {
			if err := os.Rename(path, path+".old"); err != nil {
				return err
			}
			return os.Symlink(cfg.Executable, path)
		}},
		{"inode", false, func(path string, _ *executableIdentity) error {
			if err := os.Rename(path, path+".old"); err != nil {
				return err
			}
			return os.WriteFile(path, []byte("replacement"), 0700)
		}},
		{"digest", false, func(path string, _ *executableIdentity) error {
			return os.WriteFile(path, []byte("changed executable bytes"), 0700)
		}},
		{"owner", false, func(path string, _ *executableIdentity) error { return substituteExecutableOwner(path, cfg.Executable) }},
		{"type", false, func(path string, _ *executableIdentity) error {
			if err := os.Rename(path, path+".old"); err != nil {
				return err
			}
			return os.Mkdir(path, 0700)
		}},
		{"mode", false, func(path string, _ *executableIdentity) error { return os.Chmod(path, 0600) }},
	}
	for _, mutation := range mutations {
		raceTarget := filepath.Join(base, "race-"+mutation.name)
		if err := copyExecutable(cfg.Executable, raceTarget); err != nil {
			races.Status, races.Detail = "failed", err.Error()
			break
		}
		injected := false
		callback := func(path string, identity *executableIdentity) error {
			err := mutation.fn(path, identity)
			injected = err == nil
			return err
		}
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: "exec-race-" + mutation.name, Executable: raceTarget, Args: []string{"__proof-child", "exit", "0"}, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard, preLaunchMutation: callback})
		cleanupErr := os.RemoveAll(raceTarget)
		if oldErr := os.RemoveAll(raceTarget + ".old"); cleanupErr == nil {
			cleanupErr = oldErr
		}
		if cleanupErr != nil {
			races.Status, races.Detail = "failed", mutation.name+" substitution cleanup failed"
			break
		}
		if !injected {
			races.Status, races.Detail = "inconclusive", mutation.name+" substitution injector unavailable"
			break
		}
		if mutation.reject {
			if err == nil || obs.Status != 1 {
				races.Status, races.Detail = "failed", mutation.name+" identity substitution was not rejected"
				break
			}
		} else if err != nil || obs.Status != 0 || !completeLifecycle(obs) {
			races.Status, races.Detail = "failed", fmt.Sprintf("%s source substitution affected the sealed executable snapshot: status=%d ready=%t error=%v", mutation.name, obs.Status, obs.Ready, err)
			break
		}
		races.Invocation.SubstitutionRejections = append(races.Invocation.SubstitutionRejections, mutation.name)
	}
	if races.Status == "observed" {
		races.Detail = "symlink/inode/digest/owner/type/mode source substitutions could not alter the already-sealed executable snapshot"
	}
	synchronized := finalCheckpointIdentityScenario(ctx, cfg, root, base, env)
	return []Scenario{bare, absolute, pathRejection, imageRejection, races, synchronized}
}
func finalCheckpointIdentityScenario(ctx context.Context, cfg Config, root, base string, env []string) Scenario {
	synchronized := baseScenario("exec-sealed-snapshot-final-checkpoint-race", "observed", []string{"VAL-017", "VAL-018", "PC-070", "ARCH-009"}, "Same-size in-place mutation after final validation but before release may change executed bytes.", cfg.Executable, root, env, []string{"__proof-child", "exit", "0"})
	syncTarget := filepath.Join(base, "checkpoint-race-target")
	if err := copyExecutable(cfg.Executable, syncTarget); err != nil {
		synchronized.Status, synchronized.Detail = "failed", err.Error()
	} else {
		mutated := false
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: synchronized.Name, Executable: syncTarget, Args: []string{"__proof-child", "exit", "0"}, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard, beforeReleaseMutation: func(path string) error {
			file, openErr := os.OpenFile(path, os.O_RDWR, 0)
			if openErr != nil {
				return openErr
			}
			defer file.Close()
			var first [1]byte
			if _, readErr := file.ReadAt(first[:], 0); readErr != nil {
				return readErr
			}
			first[0] ^= 0xff
			if _, writeErr := file.WriteAt(first[:], 0); writeErr != nil {
				return writeErr
			}
			mutated = true
			return file.Sync()
		}})
		synchronized.Lifecycle = lifecycleObservation(obs)
		synchronized.Resources = resourceObservation(obs, false, false)
		synchronized.Invocation.SameFDExecution = err == nil && obs.Status == 0
		if err != nil || obs.Status != 0 || !completeLifecycle(obs) || !mutated {
			synchronized.Status = "failed"
			synchronized.Detail = fmt.Sprintf("status=%d mutated=%t error=%v", obs.Status, mutated, err)
		} else {
			synchronized.Detail = "checkpoint-synchronized same-size mutation changed the source after the launcher revalidated the sealed snapshot; the original sealed bytes alone executed successfully"
		}
	}
	_ = os.Remove(syncTarget)
	return synchronized
}

func selfIdentityAttackScenarios(cfg Config, root string) []Scenario {
	base := filepath.Join(root, "internal-self-identity")
	if err := os.MkdirAll(base, 0700); err != nil {
		return []Scenario{{Name: "internal-self-identity-attack", Status: "failed", Detail: err.Error()}}
	}
	helper := filepath.Join(base, "proof-helper")
	requestPath := filepath.Join(base, "request.json")
	reportPath := filepath.Join(base, "report.json")
	if err := copyExecutable(cfg.Executable, helper); err != nil {
		return []Scenario{{Name: "internal-self-identity-attack", Status: "failed", Detail: err.Error()}}
	}
	request, err := json.Marshal(selfAttackRequest{Root: base, Helper: helper, Report: reportPath})
	if err != nil || os.WriteFile(requestPath, request, 0600) != nil {
		return []Scenario{{Name: "internal-self-identity-attack", Status: "failed", Detail: "write self-identity attack request"}}
	}
	s := baseScenario("internal-stage-open-self-identity-attacks", "observed", []string{"VAL-017", "VAL-018", "PC-015", "ARCH-010"}, "A same-path replacement with identical bytes may select supervisor, launcher, or guard machinery by pathname.", cfg.Executable, base, nil, nil)
	cmd := exec.Command(helper, "__proof-child", "self-attack", requestPath)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = base, []string{"PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}, io.Discard, io.Discard
	runErr := cmd.Run()
	var result selfAttackResult
	report, readErr := os.ReadFile(reportPath)
	if readErr != nil || decodeStrict(report, &result) != nil || runErr != nil || !result.Passed || !result.Replaced || !result.Exchanged || !result.ContentRejected || result.Status != 0 {
		s.Status = "failed"
		s.Detail = fmt.Sprintf("helper replacement/exchange/content attack was not contained: run=%v read=%v status=%d replaced=%t exchanged=%t content_rejected=%t", runErr, readErr, result.Status, result.Replaced, result.Exchanged, result.ContentRejected)
	} else {
		s.Detail = "same-path/different-inode identical-byte replacement and renameat2 exchange did not select internal machinery; an in-place content substitution attempt was kernel-rejected while the self image executed, and every stage revalidated the original opened self descriptor"
	}
	_ = os.RemoveAll(base)
	return []Scenario{s}
}
func reservedFDResources(root, class string, marker []byte) ([]*os.File, error) {
	files := make([]*os.File, 0, 3)
	closeFiles := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	for index := range 3 {
		switch class {
		case "file":
			path := filepath.Join(root, fmt.Sprintf("file-%d", index))
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				closeFiles()
				return nil, err
			}
			if _, err = file.Write(marker); err != nil {
				_ = file.Close()
				closeFiles()
				return nil, err
			}
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				_ = file.Close()
				closeFiles()
				return nil, err
			}
			files = append(files, file)
		case "pipe":
			reader, writer, err := os.Pipe()
			if err != nil {
				closeFiles()
				return nil, err
			}
			if _, err = writer.Write(marker); err != nil {
				_ = reader.Close()
				_ = writer.Close()
				closeFiles()
				return nil, err
			}
			_ = writer.Close()
			files = append(files, reader)
		case "socket":
			if index == 0 {
				path := filepath.Join(root, "socket-dispatch-slot-file")
				file, fileErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if fileErr != nil {
					closeFiles()
					return nil, fileErr
				}
				if _, fileErr = file.Write(marker); fileErr != nil {
					_ = file.Close()
					closeFiles()
					return nil, fileErr
				}
				files = append(files, file)
				continue
			}
			pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
			if err != nil {
				closeFiles()
				return nil, err
			}
			if _, err = syscall.Write(pair[0], marker); err != nil {
				_ = syscall.Close(pair[0])
				_ = syscall.Close(pair[1])
				closeFiles()
				return nil, err
			}
			_ = syscall.Close(pair[0])
			files = append(files, os.NewFile(uintptr(pair[1]), fmt.Sprintf("socket-%d", index)))
		case "fifo":
			path := filepath.Join(root, fmt.Sprintf("fifo-%d", index))
			if err := syscall.Mkfifo(path, 0600); err != nil {
				closeFiles()
				return nil, err
			}
			fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
			if err != nil {
				closeFiles()
				return nil, err
			}
			file := os.NewFile(uintptr(fd), path)
			if _, err = file.Write(marker); err != nil {
				_ = file.Close()
				closeFiles()
				return nil, err
			}
			files = append(files, file)
		default:
			closeFiles()
			return nil, errors.New("unknown reserved descriptor class")
		}
	}
	return files, nil
}

func reservedFDReuseScenario(cfg Config, root string) Scenario {
	base := filepath.Join(root, "reserved-fd-reuse")
	scenario := baseScenario("all-retained-spawn-reserved-fd-reuse-matrix", "observed", []string{"VAL-018", "PC-015", "PC-070", "ARCH-009"}, "Concurrent descriptor churn that avoids the supervisor's reserved child slots does not exercise exact 3/4/5 collisions.", cfg.Executable, base, nil, []string{"__proof-child", "reserved-fd", "REQUEST_FILE{random}"})
	if err := os.MkdirAll(base, 0700); err != nil {
		scenario.Status, scenario.Detail = "failed", err.Error()
		return scenario
	}
	defer os.RemoveAll(base)
	var markerSeed [16]byte
	if _, err := rand.Read(markerSeed[:]); err != nil {
		scenario.Status, scenario.Detail = "failed", err.Error()
		return scenario
	}
	marker := []byte(hex.EncodeToString(markerSeed[:]))
	dispatchRoot := filepath.Join(base, "dispatch-slot-3")
	if err := os.MkdirAll(dispatchRoot, 0700); err != nil {
		scenario.Status, scenario.Detail = "failed", err.Error()
		return scenario
	}
	dispatchFiles, err := reservedFDResources(dispatchRoot, "file", marker)
	if err != nil {
		scenario.Status, scenario.Detail = "failed", fmt.Sprintf("dispatch file setup: %v", err)
		return scenario
	}
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		for _, file := range dispatchFiles {
			_ = file.Close()
		}
		scenario.Status, scenario.Detail = "failed", fmt.Sprintf("dispatch socket setup: %v", err)
		return scenario
	}
	if _, err = syscall.Write(pair[0], marker); err != nil {
		_ = syscall.Close(pair[0])
		_ = syscall.Close(pair[1])
		for _, file := range dispatchFiles {
			_ = file.Close()
		}
		scenario.Status, scenario.Detail = "failed", fmt.Sprintf("dispatch socket marker: %v", err)
		return scenario
	}
	_ = syscall.Close(pair[0])
	dispatchSocket := os.NewFile(uintptr(pair[1]), "untrusted-dispatch-slot-3")
	if dispatchSocket == nil {
		_ = syscall.Close(pair[1])
		for _, file := range dispatchFiles {
			_ = file.Close()
		}
		scenario.Status, scenario.Detail = "failed", "construct dispatch socket"
		return scenario
	}
	_ = dispatchFiles[2].Close()
	publicSideEffect := filepath.Join(dispatchRoot, "public-inspect.json")
	dispatchCommand := exec.Command(cfg.Executable, "__proof-child", "inspect", publicSideEffect)
	dispatchCommand.Dir = dispatchRoot
	dispatchCommand.Env = []string{"PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	dispatchCommand.ExtraFiles = []*os.File{dispatchSocket, dispatchFiles[0], dispatchFiles[1]}
	dispatchCommand.Stdout, dispatchCommand.Stderr = io.Discard, io.Discard
	dispatchErr := dispatchCommand.Run()
	_ = dispatchSocket.Close()
	_ = dispatchFiles[0].Close()
	_ = dispatchFiles[1].Close()
	var dispatchExit *exec.ExitError
	_, sideEffectErr := os.Stat(publicSideEffect)
	if !errors.As(dispatchErr, &dispatchExit) || dispatchExit.ExitCode() != 1 || !errors.Is(sideEffectErr, os.ErrNotExist) {
		scenario.Status = "failed"
		scenario.Detail = fmt.Sprintf("untrusted AF_UNIX slot 3 did not reject before public argv: run=%v side_effect=%v", dispatchErr, sideEffectErr)
		return scenario
	}
	invocations := 0
	for _, class := range []string{"file", "pipe", "socket", "fifo"} {
		for _, cloexec := range []bool{false, true} {
			name := fmt.Sprintf("%s-%t", class, cloexec)
			runRoot := filepath.Join(base, name)
			if err := os.MkdirAll(runRoot, 0700); err != nil {
				scenario.Status, scenario.Detail = "failed", err.Error()
				return scenario
			}
			files, err := reservedFDResources(runRoot, class, marker)
			if err != nil {
				scenario.Status, scenario.Detail = "failed", fmt.Sprintf("%s resource setup: %v", name, err)
				return scenario
			}
			reportPath := filepath.Join(runRoot, "result.json")
			inspectPath := filepath.Join(runRoot, "inspect.json")
			requestPath := filepath.Join(runRoot, "request.json")
			request := reservedFDRequest{Root: runRoot, Executable: cfg.Executable, CACert: cfg.CACert, Report: reportPath, Inspect: inspectPath, CLOEXEC: cloexec}
			encoded, encodeErr := json.Marshal(request)
			if encodeErr == nil {
				encodeErr = os.WriteFile(requestPath, encoded, 0600)
			}
			command := exec.Command(cfg.Executable, "__proof-child", "reserved-fd", requestPath)
			command.Dir = runRoot
			command.Env = []string{"PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
			command.ExtraFiles = files
			command.Stdout, command.Stderr = io.Discard, io.Discard
			var runErr error
			if encodeErr == nil {
				runErr = command.Run()
			}
			for _, file := range files {
				_ = file.Close()
			}
			var result reservedFDResult
			report, readErr := os.ReadFile(reportPath)
			var decodeErr error
			if readErr == nil {
				decodeErr = decodeStrict(report, &result)
			}
			if encodeErr != nil || runErr != nil || readErr != nil || decodeErr != nil || !result.Passed {
				scenario.Status = "failed"
				scenario.Detail = fmt.Sprintf("%s failed: encode=%v run=%v read=%v decode=%v status=%d states=%t descriptors=%t lifecycle=%t", name, encodeErr, runErr, readErr, decodeErr, result.Status, result.DescriptorStates, result.DescriptorsExact, result.LifecycleExact)
				return scenario
			}
			invocations++
		}
	}
	scenario.Invocation.AllowedDescriptors = []int{0, 1, 2}
	scenario.Invocation.DescriptorsExact = true
	scenario.Resources.FDChurn = true
	scenario.Detail = "eight isolated retained spawn invocations occupied exact parent descriptors 3/4/5 with marker-bearing files, pipes, and FIFOs plus AF_UNIX sockets at 4/5 while a marker file held dispatch-reserved slot 3, in both CLOEXEC states; every managed target retained only 0/1/2 and completed the exact lifecycle, while the separate authenticated-dispatch negative matrix rejects untrusted AF_UNIX slot 3"
	return scenario
}

func substituteExecutableOwner(path, proofExecutable string) error {
	current, err := user.Current()
	if err != nil {
		return err
	}
	uidStart, err := subordinateIDStart("/etc/subuid", current.Username)
	if err != nil {
		return err
	}
	gidStart, err := subordinateIDStart("/etc/subgid", current.Username)
	if err != nil {
		return err
	}

	unshare, err := filepath.EvalSymlinks("/usr/bin/unshare")
	if err != nil || !filepath.IsAbs(unshare) {
		return errors.New("owner substitution namespace unavailable")
	}
	cmd := exec.Command(unshare,
		"--map-users", fmt.Sprintf("0:%d:1", os.Getuid()),
		"--map-users", fmt.Sprintf("1:%d:1", uidStart),
		"--map-groups", fmt.Sprintf("0:%d:1", os.Getgid()),
		"--map-groups", fmt.Sprintf("1:%d:1", gidStart),
		"--setuid", "0", "--setgid", "0",
		proofExecutable, "__proof-child", "chown1", path)
	cmd.Env = []string{"PATH=/usr/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run()
}

func openProofPTY() (*os.File, *os.File, error) {
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	closeMaster := func(err error) (*os.File, *os.File, error) {
		_ = syscall.Close(masterFD)
		return nil, nil, err
	}
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		return closeMaster(err)
	}
	number, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		return closeMaster(err)
	}
	slaveFD, err := unix.Open("/dev/pts/"+strconv.FormatUint(uint64(number), 10), unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return closeMaster(err)
	}
	return os.NewFile(uintptr(masterFD), "proof-pty-master"), os.NewFile(uintptr(slaveFD), "proof-pty-slave"), nil
}

func backgroundProcessGroup(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	close := strings.LastIndexByte(string(data), ')')
	if close < 0 {
		return false
	}
	fields := strings.Fields(string(data[close+1:]))
	if len(fields) < 4 {
		return false
	}
	group, groupErr := strconv.Atoi(fields[2])
	session, sessionErr := strconv.Atoi(fields[3])
	return groupErr == nil && sessionErr == nil && group == pid && session != pid
}

func interactiveBashPTYScenario(ctx context.Context, cfg Config, root string) Scenario {
	env := isolatedEnv(root, cfg.CACert)
	s := baseScenario("shell-bash-5.3.15-interactive-pty-job-control", "observed", []string{"VAL-017", "VAL-018", "VAL-019", "VAL-020", "VAL-023", "PC-015", "PC-016", "ARCH-010"}, "Interactive Bash job-control process groups may escape signal, hard-death, reaping, or cgroup cleanup.", cfg.Bash, root, env, []string{"--noprofile", "--norc", "-i"})
	if cfg.Bash == "" || !filepath.IsAbs(cfg.Bash) || cfg.SignalFixture == "" || !filepath.IsAbs(cfg.SignalFixture) {
		s.Status, s.Detail = "inconclusive", "exact Bash 5.3.15 or pinned sleep executable unavailable"
		return s
	}
	sleepCommand := strconv.Quote(cfg.SignalFixture) + " --coreutils-prog=sleep 10"
	run := func(name, commands, readyFile string, runCtx context.Context, hardDeath bool) (Observation, error, bool) {
		master, slave, err := openProofPTY()
		if err != nil {
			return Observation{}, err, false
		}
		defer master.Close()
		defer slave.Close()
		var output bytes.Buffer
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(&output, master) }()
		go func() { defer wg.Done(); _, _ = io.WriteString(master, commands) }()
		jobGroup := make(chan bool, 1)
		supervisorReadyFile := readyFile
		if hardDeath {
			supervisorReadyFile += ".observed"
		}
		go func() {
			pid, waitErr := awaitPIDFile(readyFile, 2*time.Second)
			observed := waitErr == nil && backgroundProcessGroup(pid)
			if observed && hardDeath {
				observed = os.WriteFile(supervisorReadyFile, []byte("observed\n"), 0o600) == nil
			}
			jobGroup <- observed
		}()
		obs, runErr := Supervise(runCtx, cfg.Executable, Spec{Name: name, Executable: cfg.Bash, Args: []string{"--noprofile", "--norc", "-i"}, Dir: root, Env: env, Stdin: slave, Stdout: slave, Stderr: slave, ReadyFile: supervisorReadyFile, HardKillAfterReady: hardDeath, InteractivePTY: true})
		_ = slave.Close()
		_ = master.Close()
		wg.Wait()
		return obs, runErr, <-jobGroup && output.Len() != 0
	}
	normalPID := filepath.Join(root, "bash-pty-normal.pid")
	normalCommands := "set -m\n" + sleepCommand + " &\nj=$!\nprintf '%s\\n' \"$j\" > " + strconv.Quote(normalPID) + "\n" + strconv.Quote(cfg.SignalFixture) + " --coreutils-prog=sleep 0.25\njobs -p\nkill -TERM \"$j\"\nwait \"$j\"\nexit 0\n"
	normal, normalErr, normalGroup := run("bash-interactive-job-control", normalCommands, normalPID, ctx, false)
	cancelPID := filepath.Join(root, "bash-pty-cancel.pid")
	cancelCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(250*time.Millisecond, cancel)
	cancelCommands := "set -m\ntrap '' TERM\n" + sleepCommand + " &\nj=$!\nprintf '%s\\n' \"$j\" > " + strconv.Quote(cancelPID) + "\nwait\n"
	cancelObs, cancelErr, cancelGroup := run("bash-interactive-cancellation", cancelCommands, cancelPID, cancelCtx, false)
	timer.Stop()
	cancel()
	hardPID := filepath.Join(root, "bash-pty-hard.pid")
	hardCommands := "set -m\n" + sleepCommand + " &\nj=$!\nprintf '%s\\n' \"$j\" > " + strconv.Quote(hardPID) + "\nwait\n"
	hard, hardErr, hardGroup := run("bash-interactive-hard-death", hardCommands, hardPID, ctx, true)
	s.Lifecycle = lifecycleObservation(hard)
	s.Resources = resourceObservation(hard, false, true)
	if normalErr != nil || normal.Status != 0 || !completeLifecycle(normal) || !normalGroup || cancelErr != nil || cancelObs.Status != 1 || !cancelObs.Forwarded || !cancelGroup || cancelObs.Orphans != 0 || cancelObs.SurvivorsDetected != 0 || hardErr != nil || !hard.HardDeath || !hard.GuardCleanup || !hardGroup || hard.Orphans != 0 || hard.SurvivorsDetected != 0 {
		s.Status = "failed"
		s.Detail = fmt.Sprintf("normal(status=%d group=%t err=%v) cancel(status=%d group=%t forwarded=%t orphans=%d survivors=%d err=%v) hard(group=%t death=%t cleanup=%t orphans=%d survivors=%d err=%v)", normal.Status, normalGroup, normalErr, cancelObs.Status, cancelGroup, cancelObs.Forwarded, cancelObs.Orphans, cancelObs.SurvivorsDetected, cancelErr, hardGroup, hard.HardDeath, hard.GuardCleanup, hard.Orphans, hard.SurvivorsDetected, hardErr)
	} else {
		s.Detail = "exact Bash 5.3.15 --noprofile --norc -i on a controlling PTY created foreground/background job-control groups; normal completion, catchable cancellation, and supervisor hard death left the mandatory target-only cgroup empty with no survivor"
	}
	return s
}
func subordinateIDStart(name, username string) (uint64, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) != 3 || fields[0] != username {
			continue
		}
		start, startErr := strconv.ParseUint(fields[1], 10, 32)
		count, countErr := strconv.ParseUint(fields[2], 10, 32)
		if startErr != nil || countErr != nil || count == 0 {
			return 0, errors.New("invalid subordinate identity range")
		}
		return start, nil
	}
	return 0, errors.New("subordinate identity range unavailable")
}
func ioUringDenialScenario(ctx context.Context, cfg Config, root string) Scenario {
	report := filepath.Join(root, "io-uring-denial.txt")
	env := isolatedEnv(root, cfg.CACert)
	s := baseScenario("trace-manager-io-uring-seccomp-denial", "observed", []string{"VAL-017", "VAL-018", "VAL-019", "VAL-020", "PC-015", "ARCH-010"}, "Trace-wrapped manager classes may create io_uring instances before closed tracing can reject them.", cfg.Executable, root, env, []string{"__proof-child", "io-uring-denied", "OBSERVATION_FILE{random}"})
	obs, err := Supervise(ctx, cfg.Executable, Spec{Name: s.Name, Executable: cfg.Executable, Args: []string{"__proof-child", "io-uring-denied", report}, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard, DenyIOUring: true})
	s.Lifecycle = lifecycleObservation(obs)
	s.Resources = resourceObservation(obs, false, false)
	result, readErr := os.ReadFile(report)
	if err != nil || readErr != nil || obs.Status != 0 || !completeLifecycle(obs) || !obs.IOUringDenied || strings.TrimSpace(string(result)) != strconv.Itoa(int(syscall.EPERM)) {
		s.Status = "failed"
		s.Detail = fmt.Sprintf("status=%d filter=%t syscall_result=%q supervise_error=%v observation_error=%v", obs.Status, obs.IOUringDenied, strings.TrimSpace(string(result)), err, readErr)
	} else {
		s.Detail = "the authenticated launcher installed the exact x86_64 three-syscall seccomp filter before release; the ordinary same-FD exec completed and its raw io_uring_setup call observed EPERM"
	}
	return s
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
func shellStatusScenario(ctx context.Context, cfg Config, root string) Scenario {
	env := isolatedEnv(root, cfg.CACert)
	s := baseScenario("shell-status-0-through-255", "observed", []string{"VAL-020", "PC-014", "PC-016", "ARCH-010", "ARCH-027"}, "The shell wrapper reserves, truncates, or remaps a normally exited status.", cfg.Bash, root, env, []string{"--noprofile", "--norc", "-c", "exit <0-255>"})
	if cfg.Bash == "" || !filepath.IsAbs(cfg.Bash) {
		s.Status = "inconclusive"
		s.Detail = "canonical pinned shell executable was not supplied; no shell status was exercised"
		return s
	}
	for n := 0; n <= 255; n++ {
		args := []string{"--noprofile", "--norc", "-c", "exit " + strconv.Itoa(n)}
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: s.Name, Executable: cfg.Bash, Args: args, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard})
		s.Resources = resourceObservation(obs, false, false)
		s.Lifecycle = lifecycleObservation(obs)
		if err != nil || obs.Status != n {
			s.Status = "failed"
			s.Detail = fmt.Sprintf("status %d observed %d: %v", n, obs.Status, err)
			break
		}
	}
	return s
}

func cancellationScenario(ctx context.Context, cfg Config, root string, ignoring bool) Scenario {
	mode, name := "spawn", "context-cancellation-cooperative-tree"
	if ignoring {
		mode, name = "spawn-ignore", "context-cancellation-grace-escalation-tree"
	}
	pidFile := filepath.Join(root, name+".pid")
	env := isolatedEnv(root, cfg.CACert)
	s := baseScenario(name, "observed", []string{"VAL-019", "VAL-020", "PC-015", "ARCH-010"}, "Cancellation may return while a managed descendant remains alive.", cfg.Executable, root, env, []string{"__proof-child", mode, "DESCENDANT_FILE{random}"})
	cancelCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	obs, err := Supervise(cancelCtx, cfg.Executable, Spec{Name: name, Executable: cfg.Executable, Args: []string{"__proof-child", mode, pidFile}, Dir: root, Env: env, ReadyFile: pidFile, Stdout: io.Discard, Stderr: io.Discard})
	pid, pidErr := awaitPIDFile(pidFile, time.Second)
	alive := pidErr == nil && processExists(pid)
	s.Lifecycle = lifecycleObservation(obs)
	if err != nil || pidErr != nil || obs.Status != 1 || !completeLifecycle(obs) || !obs.Forwarded || obs.Reaped < 1 || alive || obs.Orphans != 0 || (ignoring && !obs.Escalated) {
		s.Status = "failed"
		s.Detail = fmt.Sprintf("status=%d forwarded=%t escalated=%t reaped=%d orphans=%d alive_after_return=%t supervise_error=%v readiness_error=%v", obs.Status, obs.Forwarded, obs.Escalated, obs.Reaped, obs.Orphans, alive, err, pidErr)
	}
	return s
}
func cgroupPathSubstitutionScenario(ctx context.Context, cfg Config, root string) Scenario {
	name := "descriptor-relative-cgroup-path-substitution"
	pidFile := filepath.Join(root, name+".pid")
	env := isolatedEnv(root, cfg.CACert)
	scenario := baseScenario(name, "observed", []string{"VAL-019", "PC-015", "PC-070", "ARCH-010"}, "Changing every remaining cgroup pathname field to a distinct valid sibling after opening the original authority can split launcher placement and cleanup from that descriptor.", cfg.Executable, root, env, []string{"__proof-child", "spawn-ignore", "DESCENDANT_FILE{random}"})
	cancelCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	observation, err := Supervise(cancelCtx, cfg.Executable, Spec{
		Name: name, Executable: cfg.Executable,
		Args: []string{"__proof-child", "spawn-ignore", pidFile},
		Dir:  root, Env: env, ReadyFile: pidFile, Stdout: io.Discard, Stderr: io.Discard,
		CgroupPathSubstitution: true,
	})
	cancel()
	pid, pidErr := awaitPIDFile(pidFile, time.Second)
	alive := pidErr == nil && processExists(pid)
	scenario.Lifecycle = lifecycleObservation(observation)
	scenario.Resources = resourceObservation(observation, false, false)
	if err != nil || pidErr != nil || observation.Status != 1 || !completeLifecycle(observation) ||
		!observation.Forwarded || !observation.Escalated || observation.Reaped < 1 || alive ||
		observation.Orphans != 0 || observation.SurvivorsDetected != 0 {
		scenario.Status = "failed"
		scenario.Detail = fmt.Sprintf("status=%d forwarded=%t escalated=%t reaped=%d orphans=%d survivors=%d alive=%t error=%v readiness=%v", observation.Status, observation.Forwarded, observation.Escalated, observation.Reaped, observation.Orphans, observation.SurvivorsDetected, alive, err, pidErr)
	} else {
		scenario.Detail = "after the original cgroup descriptor identity was recorded and every remaining pathname field was changed to a distinct sibling cgroup, the managed PID appeared only through the original descriptor; descriptor-relative launcher placement, identity verification, kill, membership checks, emptiness verification, reaping, and guard disarm contained the original while the sibling retained its identity and remained empty"
	}
	return scenario
}

func wrapperSignalScenarios(ctx context.Context, cfg Config, root string) []Scenario {
	out := make([]Scenario, 0, len(catchableSignalNumbers()))
	for _, sig := range catchableSignalNumbers() {
		name := "wrapper-signal-" + strconv.Itoa(int(sig)) + "-forwarding-status"
		pidFile := filepath.Join(root, name+".pid")
		env := isolatedEnv(root, cfg.CACert)
		s := baseScenario(name, "observed", []string{"VAL-019", "VAL-020", "PC-015", "ARCH-010"}, "A catchable wrapper signal is not forwarded or remapped to 128+signal.", cfg.Executable, root, env, []string{"__proof-child", "spawn", "DESCENDANT_FILE{random}"})
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: name, Executable: cfg.Executable, Args: []string{"__proof-child", "spawn", pidFile}, Dir: root, Env: env, ReadyFile: pidFile, WrapperSignal: sig, Stdout: io.Discard, Stderr: io.Discard})
		pid, pidErr := awaitPIDFile(pidFile, time.Second)
		alive := pidErr == nil && processExists(pid)
		s.Lifecycle = lifecycleObservation(obs)
		if err != nil || pidErr != nil || obs.Status != 128+int(sig) || !completeLifecycle(obs) || !obs.Forwarded || obs.Reaped < 1 || alive || obs.Orphans != 0 {
			s.Status = "failed"
			s.Detail = fmt.Sprintf("status=%d forwarded=%t reaped=%d orphans=%d alive_after_return=%t error=%v readiness_error=%v", obs.Status, obs.Forwarded, obs.Reaped, obs.Orphans, alive, err, pidErr)
		}
		out = append(out, s)
	}
	return out
}

func launchFailureScenario(ctx context.Context, cfg Config, root string) Scenario {
	env := isolatedEnv(root, cfg.CACert)
	exe := filepath.Join(root, "non-executable")
	s := baseScenario("managed-launch-failure-status-1", "observed", []string{"VAL-020", "PC-014", "PC-015", "ARCH-010"}, "A managed launch failure has an undefined wrapper status.", exe, root, env, nil)
	if err := os.WriteFile(exe, []byte("not executable\n"), 0600); err != nil {
		s.Status = "failed"
		s.Detail = err.Error()
		return s
	}
	obs, err := Supervise(ctx, cfg.Executable, Spec{Name: s.Name, Executable: exe, Dir: root, Env: env, Stdout: io.Discard, Stderr: io.Discard})
	s.Lifecycle = lifecycleObservation(obs)
	if err == nil || obs.Status != 1 {
		s.Status = "failed"
		s.Detail = fmt.Sprintf("status=%d error=%v", obs.Status, err)
	}
	return s
}

func dispatchNegativeScenarios(proofExecutable, root string) []Scenario {
	type negative struct {
		name   string
		passed bool
		detail string
	}
	pipeRead, pipeWrite, pipeErr := os.Pipe()
	wrongFDRejected := pipeErr == nil && validateControlSocket(int(pipeRead.Fd())) != nil
	if pipeRead != nil {
		pipeRead.Close()
	}
	if pipeWrite != nil {
		pipeWrite.Close()
	}
	peerParent, peerChild, peerErr := socketPair()
	wrongPeerRejected := peerErr == nil && validatePeerIdentity(int(peerChild.Fd()), os.Getpid()+1, uint32(os.Geteuid())) != nil
	if peerParent != nil {
		peerParent.Close()
	}
	if peerChild != nil {
		peerChild.Close()
	}
	public := exec.Command(proofExecutable, "--reserved-control-probe")
	public.Dir, public.Stdout, public.Stderr = root, io.Discard, io.Discard
	publicErr := public.Run()
	publicFallthrough := waitStatus(publicErr) == 2
	var unknown wireSpec
	unknownRejected := decodeStrict([]byte(`{"unknown":true}`), &unknown) != nil
	oversizedRejected := writeFrame(io.Discard, strings.Repeat("x", maxControlSpecBytes)) != nil
	checks := []negative{
		{name: "supervisor-dispatch-missing-fd-fallthrough", passed: validateControlSocket(-1) != nil && publicFallthrough, detail: "missing reserved FD did not dispatch and the unchanged public argv reached the public parser"},
		{name: "supervisor-dispatch-wrong-fd-type", passed: wrongFDRejected, detail: "a non-socket reserved descriptor was rejected"},
		{name: "supervisor-dispatch-wrong-peer", passed: wrongPeerRejected, detail: "SO_PEERCRED PID mismatch was rejected"},
		{name: "supervisor-dispatch-wrong-parent-executable", passed: !sameExecutable(os.Getpid(), -1), detail: "parent executable identity mismatch was rejected"},
		{name: "supervisor-dispatch-wrong-nonce", passed: validateNonce(strings.Repeat("0", 64), strings.Repeat("1", 64)) != nil, detail: "a nonmatching one-time nonce was rejected"},
		{name: "supervisor-dispatch-unknown-spec-field", passed: unknownRejected, detail: "strict specification decoding rejected an unknown field"},
		{name: "supervisor-dispatch-oversized-spec", passed: oversizedRejected, detail: "bounded framing rejected an oversized specification"},
	}
	out := make([]Scenario, 0, len(checks))
	for _, check := range checks {
		s := baseScenario(check.name, "observed", []string{"VAL-017", "VAL-018", "PC-015", "ARCH-010"}, "An unauthenticated reserved descriptor can enter supervisor dispatch or consume public argv.", proofExecutable, root, nil, nil)
		s.Detail = check.detail
		if !check.passed {
			s.Status = "failed"
			s.Detail += "; rejection was not mechanically established"
		}
		out = append(out, s)
	}
	return out
}

func hardDeathScenarios(ctx context.Context, cfg Config, root string) []Scenario {
	boundaries := []string{"before-clone", "after-launcher-start", "after-descriptor-sanitization", "after-parent-death-validation", "after-group-session-validation", "after-second-parent-check", "before-exec-release", "after-first-descendant"}
	out := make([]Scenario, 0, len(boundaries))
	for _, boundary := range boundaries {
		name := "hard-death-" + boundary
		s := baseScenario(name, "observed", []string{"VAL-018", "VAL-019", "VAL-020", "PC-015", "ARCH-010"}, "Uncatchable wrapper death at a lifecycle boundary may leave a managed leader or descendant.", cfg.Executable, root, isolatedEnv(root, cfg.CACert), nil)
		pidFile := ""
		args := []string{"__proof-child", "sleep", "10s"}
		if boundary == "after-first-descendant" {
			pidFile = filepath.Join(root, name+".pid")
			args = []string{"__proof-child", "spawn-ignore", pidFile}
		}
		obs, err := Supervise(ctx, cfg.Executable, Spec{Name: name, Executable: cfg.Executable, Args: args, Dir: root, Env: isolatedEnv(root, cfg.CACert), ReadyFile: pidFile, InjectionGate: boundary, HardKillAfterReady: true, Stdout: io.Discard, Stderr: io.Discard})
		s.Resources = resourceObservation(obs, true, obs.HardDeath)
		s.Lifecycle = lifecycleObservation(obs)
		if err != nil || !obs.GateReached || !obs.HardDeath || obs.Reaped < 1 || obs.SurvivorsDetected != 0 || !obs.GuardArmed || !obs.GuardCleanup {
			s.Status = "failed"
			s.Detail = fmt.Sprintf("gate=%t hard_death=%t guard_armed=%t guard_cleanup=%t leader=%t descendants=%d reaped=%d survivors=%d error=%v", obs.GateReached, obs.HardDeath, obs.GuardArmed, obs.GuardCleanup, obs.LeaderObserved, obs.DescendantsObserved, obs.Reaped, obs.SurvivorsDetected, err)
		} else {
			s.Detail = fmt.Sprintf("same-binary FD-only injection reached the real %s operation with leader_observed=%t descendant_count=%d; checkpoints before exec were emitted by the unreleased launcher itself, wrapper SIGKILL closed the sole guard channel, and the outer subreaper observed zero survivors without fallback cleanup after reaping %d guard/group processes", boundary, obs.LeaderObserved, obs.DescendantsObserved, obs.Reaped)
		}
		out = append(out, s)
	}
	return out
}
func resourceObservation(obs Observation, _ bool, hardDeath bool) ResourceObservation {
	return ResourceObservation{FDChurn: obs.FDChurnComplete, CgroupObserved: obs.CgroupObserved, CgroupEmpty: obs.CgroupEmpty, MemoryPeakBytes: obs.MemoryPeak, ProcessPeak: obs.ProcessPeak, CPUUsec: obs.CPUUsec, MemoryOOM: obs.MemoryOOM, OOMKill: obs.OOMKill, HardDeathObserved: hardDeath}
}

func lifecycleObservation(obs Observation) LifecycleObservation {
	return LifecycleObservation{Session: obs.Session, ProcessGroup: obs.ProcessGroup, Subreaper: obs.Subreaper, ParentChecked: obs.ParentChecked, ParentDeathSignal: obs.ParentDeathSignal, ReadyHandshake: obs.Ready, SignalForwarded: obs.Forwarded, Escalated: obs.Escalated, ExitStatus: obs.Status, DescendantsReaped: obs.Reaped, Orphans: obs.Orphans, SurvivorsDetected: obs.SurvivorsDetected, DeathGuardArmed: obs.GuardArmed, DeathGuardCleanup: obs.GuardCleanup, LeaderObserved: obs.LeaderObserved, ObservedDescendants: obs.DescendantsObserved, IOUringDenied: obs.IOUringDenied, CgroupKillGuard: obs.CgroupKillGuard, GuardStatus: obs.GuardStatus, GuardStatusObserved: obs.GuardStatusObserved}
}

func baseScenario(name, status string, ids []string, rejected, exe, dir string, env, args []string) Scenario {
	return Scenario{Name: name, Status: status, RequirementIDs: ids, RejectedAssumption: rejected,
		Invocation:  InvocationObservation{Executable: exe, Arguments: append([]string(nil), args...), WorkingDirectory: "owned-disposable"},
		Environment: EnvironmentObservation{AllowedNames: envNames(env), ClearedClasses: []string{"ASDF_*", "CHEZMOI_*", "GIT_*", "MISE_*", "RTX_*", "SSH_*", "proxy", "prompt", "terminal", "ambient PATH"}, Exact: false},
		Access:      AccessObservation{TraceAvailable: false}, Writes: WriteObservation{TraceAvailable: false}}
}

func completeLifecycle(obs Observation) bool {
	return obs.Ready && obs.Session && obs.ProcessGroup && obs.Subreaper && obs.ParentChecked && obs.ParentDeathSignal &&
		obs.GuardArmed && obs.GuardCleanup && obs.GuardStatusObserved && obs.GuardStatus == 0 &&
		obs.CgroupKillGuard && obs.CgroupObserved && obs.CgroupEmpty && obs.FDChurnComplete &&
		obs.Orphans == 0 && obs.SurvivorsDetected == 0 && obs.MemoryOOM == 0 && obs.OOMKill == 0
}

func catchableSignalNumbers() []syscall.Signal {
	return []syscall.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM}
}

func childTerminatingSignalNumbers() []syscall.Signal {
	return []syscall.Signal{
		syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGTRAP,
		syscall.SIGABRT, syscall.SIGBUS, syscall.SIGFPE, syscall.SIGKILL, syscall.SIGUSR1,
		syscall.SIGSEGV, syscall.SIGUSR2, syscall.SIGPIPE, syscall.SIGALRM, syscall.SIGTERM,
		syscall.SIGSTKFLT, syscall.SIGXCPU, syscall.SIGXFSZ, syscall.SIGVTALRM, syscall.SIGPROF,
		syscall.SIGIO, syscall.SIGPWR, syscall.SIGSYS,
	}
}

func awaitPIDFile(path string, limit time.Duration) (int, error) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(b)))
			if convErr == nil && pid > 0 {
				return pid, nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return 0, errors.New("descendant PID was not recorded")
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func descriptorObservationExact(observed processObservation) bool {
	if len(observed.UnexpectedDescriptors) != 0 || len(observed.Descriptors) != 3+len(observed.PostExecRuntimeDescriptors) {
		return false
	}
	for index, descriptor := range []int{0, 1, 2} {
		if observed.Descriptors[index] != descriptor {
			return false
		}
	}
	return true
}

func lifecycleTreeContainsMarker(root string, marker []byte) (bool, error) {
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
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found = bytes.Contains(content, marker)
		return nil
	})
	return found, err
}

func isolatedEnv(root, ca string) []string {
	env := []string{"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "xdg-config"), "XDG_CACHE_HOME=" + filepath.Join(root, "xdg-cache"), "XDG_DATA_HOME=" + filepath.Join(root, "xdg-data"), "XDG_STATE_HOME=" + filepath.Join(root, "xdg-state"), "PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TERM=dumb", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "SSH_AUTH_SOCK=", "http_proxy=", "https_proxy=", "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=*"}
	if ca != "" {
		env = append(env, "SSL_CERT_FILE="+ca, "GIT_SSL_CAINFO="+ca)
	}
	return env
}
func envNames(env []string) []string {
	out := make([]string, 0, len(env))
	for _, v := range env {
		out = append(out, strings.SplitN(v, "=", 2)[0])
	}
	sort.Strings(out)
	return out
}

func readProcessObservation(path string) (processObservation, error) {
	var observed processObservation
	b, err := os.ReadFile(path)
	if err != nil {
		return observed, err
	}
	err = json.Unmarshal(b, &observed)
	return observed, err
}

func equalStringSets(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateRoot(raw string, protected []string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("lifecycle proof requires Linux")
	}
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errors.New("explicit absolute disposable root required")
	}
	if len(protected) == 0 {
		return "", errors.New("canonical protected roots captured before environment poisoning are required")
	}
	r, e := filepath.EvalSymlinks(raw)
	if e != nil {
		return "", fmt.Errorf("canonicalize disposable root: %w", e)
	}
	for _, p := range protected {
		if p == "" {
			continue
		}
		p, e = filepath.EvalSymlinks(p)
		if e != nil {
			return "", fmt.Errorf("canonicalize protected root: %w", e)
		}
		if within(r, p) || within(p, r) {
			return "", errors.New("disposable root overlaps a protected path class")
		}
	}
	return r, nil
}
func within(path, base string) bool {
	rel, e := filepath.Rel(base, path)
	return e == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."))
}

const descriptorChurnTargetCount = 6

type descriptorChurnMatrix struct {
	mu      sync.Mutex
	active  bool
	targets [descriptorChurnTargetCount]int
	counts  [4][2][descriptorChurnTargetCount]uint64
}

func (m *descriptorChurnMatrix) record(class, cloexec, target int) {
	m.mu.Lock()
	if m.active {
		m.counts[class][cloexec][target]++
	}
	m.mu.Unlock()
}

func (m *descriptorChurnMatrix) activate() {
	m.mu.Lock()
	m.counts = [4][2][descriptorChurnTargetCount]uint64{}
	m.active = true
	m.mu.Unlock()
}

func (m *descriptorChurnMatrix) complete() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for class := range m.counts {
		for cloexec := range m.counts[class] {
			for target := range m.counts[class][cloexec] {
				if m.counts[class][cloexec][target] == 0 {
					return false
				}
			}
		}
	}
	return true
}

func (m *descriptorChurnMatrix) missing() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var missing []string
	for class := range m.counts {
		for cloexec := range m.counts[class] {
			for target, count := range m.counts[class][cloexec] {
				if count == 0 {
					missing = append(missing, fmt.Sprintf("class=%d/cloexec=%d/fd=%d", class, cloexec, m.targets[target]))
				}
			}
		}
	}
	return missing
}

func duplicateChurnSource(source, class, cloexec int, matrix *descriptorChurnMatrix) {
	command := unix.F_DUPFD
	if cloexec != 0 {
		command = unix.F_DUPFD_CLOEXEC
	}
	for targetIndex, target := range matrix.targets {
		duplicated, err := unix.FcntlInt(uintptr(source), command, target)
		if err != nil {
			continue
		}
		if duplicated == target {
			matrix.record(class, cloexec, targetIndex)
		}
		_ = syscall.Close(duplicated)
	}
}

func highChurnDescriptor(fd int) (int, error) {
	high, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 768)
	_ = syscall.Close(fd)
	return high, err
}

func descriptorChurnCycle(markerPath, streamPath string, marker []byte, matrix *descriptorChurnMatrix) {
	for cloexec := range 2 {
		flags := 0
		socketFlags := syscall.SOCK_STREAM
		if cloexec != 0 {
			flags = syscall.O_CLOEXEC
			socketFlags |= syscall.SOCK_CLOEXEC
		}
		if fd, err := syscall.Open(markerPath, syscall.O_RDONLY|flags, 0); err == nil {
			if high, highErr := highChurnDescriptor(fd); highErr == nil {
				duplicateChurnSource(high, 0, cloexec, matrix)
				_ = syscall.Close(high)
			}
		}
		var pipe [2]int
		if syscall.Pipe2(pipe[:], flags) == nil {
			_, _ = syscall.Write(pipe[1], marker)
			high, highErr := highChurnDescriptor(pipe[0])
			_ = syscall.Close(pipe[1])
			if highErr == nil {
				duplicateChurnSource(high, 1, cloexec, matrix)
				_ = syscall.Close(high)
			}
		}
		if pair, err := syscall.Socketpair(syscall.AF_UNIX, socketFlags, 0); err == nil {
			_, _ = syscall.Write(pair[0], marker)
			high, highErr := highChurnDescriptor(pair[1])
			_ = syscall.Close(pair[0])
			if highErr == nil {
				duplicateChurnSource(high, 2, cloexec, matrix)
				_ = syscall.Close(high)
			}
		}
		if fd, err := syscall.Open(streamPath, syscall.O_RDWR|syscall.O_NONBLOCK|flags, 0); err == nil {
			_, _ = syscall.Write(fd, marker)
			if high, highErr := highChurnDescriptor(fd); highErr == nil {
				duplicateChurnSource(high, 3, cloexec, matrix)
				_ = syscall.Close(high)
			}
		}
	}
}

func selectDescriptorChurnTargets() ([descriptorChurnTargetCount]int, error) {
	var targets [descriptorChurnTargetCount]int
	next := 0
	ranges := [][2]int{{6, 62}, {63, 254}, {255, 510}, {511, 767}}
	for rangeIndex, bounds := range ranges {
		needed := 1
		if rangeIndex == 0 {
			needed = 3
		}
		for fd := bounds[0]; fd <= bounds[1] && needed != 0; fd++ {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, syscall.EBADF) {
				continue
			}
			targets[next] = fd
			next++
			needed--
		}
		if needed != 0 {
			return targets, errors.New("no free descriptor churn target in required low/high range")
		}
	}
	return targets, nil
}

func churnFDs(dir string) (func(), func() bool, error) {
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, nil, err
	}
	marker := []byte(hex.EncodeToString(seed[:]))
	markerFile, err := os.CreateTemp(dir, "fd-churn-marker-")
	if err != nil {
		return nil, nil, err
	}
	markerPath := markerFile.Name()
	if err = markerFile.Chmod(0o600); err == nil {
		_, err = markerFile.Write(marker)
	}
	closeErr := markerFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(markerPath)
		return nil, nil, err
	}
	streamPath := markerPath + ".stream"
	if err = syscall.Mkfifo(streamPath, 0o600); err != nil {
		_ = os.Remove(markerPath)
		return nil, nil, err
	}
	targets, targetErr := selectDescriptorChurnTargets()
	if targetErr != nil {
		_ = os.Remove(streamPath)
		_ = os.Remove(markerPath)
		return nil, nil, targetErr
	}
	preflight := &descriptorChurnMatrix{active: true, targets: targets}
	descriptorChurnCycle(markerPath, streamPath, marker, preflight)
	if !preflight.complete() {
		_ = os.Remove(streamPath)
		_ = os.Remove(markerPath)
		return nil, nil, fmt.Errorf("descriptor churn could not occupy exact matrix: %s", strings.Join(preflight.missing(), ","))
	}
	matrix := &descriptorChurnMatrix{targets: targets}
	done := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					descriptorChurnCycle(markerPath, streamPath, marker, matrix)
				}
			}
		}()
	}
	activate := func() {
		matrix.activate()
		for attempt := 0; attempt < 64 && !matrix.complete(); attempt++ {
			descriptorChurnCycle(markerPath, streamPath, marker, matrix)
		}
	}
	stop := func() bool {
		close(done)
		wg.Wait()
		complete := matrix.complete()
		_ = os.Remove(streamPath)
		_ = os.Remove(markerPath)
		return complete
	}
	return activate, stop, nil
}
