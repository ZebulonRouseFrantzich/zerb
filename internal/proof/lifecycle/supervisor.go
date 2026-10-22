package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	supervisorControlFD = 3
	maxControlSpecBytes = 64 << 10
	prSetParentDeathSig = 1
	prGetParentDeathSig = 2
)

const (
	targetExecutableFD = 4
	selfExecutableFD   = 5
	guardSelfFD        = 4
)

type cgroupIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type executableIdentity struct {
	Path   string `json:"path"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Owner  uint32 `json:"owner"`
	Group  uint32 `json:"group"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type internalStageSpec struct {
	Nonce              string             `json:"nonce"`
	Stage              string             `json:"stage"`
	Cgroup             cgroupIdentity     `json:"cgroup,omitempty"`
	ParentPID          int                `json:"parent_pid"`
	Process            *wireSpec          `json:"process,omitempty"`
	Group              int                `json:"group,omitempty"`
	Self               executableIdentity `json:"self"`
	GuardFailureStatus int                `json:"guard_failure_status,omitempty"`
}

type wireChallenge struct {
	Nonce             string `json:"nonce"`
	ParentDeathSignal int    `json:"parent_death_signal"`
}
type wireSpec struct {
	ParentPID              int                `json:"parent_pid"`
	Executable             executableIdentity `json:"executable"`
	Arguments              []string           `json:"arguments"`
	Directory              string             `json:"directory"`
	Environment            []string           `json:"environment"`
	ReadyFile              string             `json:"ready_file,omitempty"`
	FileSizeLimit          uint64             `json:"file_size_limit,omitempty"`
	CgroupInherited        bool               `json:"cgroup_inherited,omitempty"`
	MutationBeforeRelease  bool               `json:"mutation_before_release,omitempty"`
	CgroupPath             string             `json:"cgroup_path,omitempty"`
	Cgroup                 cgroupIdentity     `json:"cgroup,omitempty"`
	CgroupPathSubstitution bool               `json:"cgroup_path_substitution,omitempty"`
	InjectionGate          string             `json:"injection_gate,omitempty"`
	DenyIOUring            bool               `json:"deny_io_uring,omitempty"`
	ChildSignal            int                `json:"child_signal,omitempty"`
	Self                   executableIdentity `json:"self"`
	InteractivePTY         bool               `json:"interactive_pty,omitempty"`
	GuardFailureStatus     int                `json:"guard_failure_status,omitempty"`
}
type wireReady struct {
	Ready            bool   `json:"ready"`
	Error            string `json:"error,omitempty"`
	SupervisorPID    int    `json:"supervisor_pid"`
	Session          int    `json:"session"`
	Group            int    `json:"group"`
	ChildPID         int    `json:"child_pid"`
	ChildGroup       int    `json:"child_group"`
	ParentChecked    bool   `json:"parent_checked"`
	Subreaper        bool   `json:"subreaper"`
	ChildParentDeath bool   `json:"child_parent_death"`
	GuardPID         int    `json:"guard_pid"`
	GuardArmed       bool   `json:"guard_armed"`
	Gate             string `json:"gate,omitempty"`
	LeaderObserved   bool   `json:"leader_observed"`
	DescendantCount  int    `json:"descendant_count"`
	IOUringDenied    bool   `json:"io_uring_denied"`
	CgroupKillGuard  bool   `json:"cgroup_kill_guard"`
}
type wireResult struct {
	Status              int  `json:"status"`
	Reaped              int  `json:"descendants_reaped"`
	Orphans             int  `json:"orphans"`
	Forwarded           bool `json:"forwarded"`
	Escalated           bool `json:"escalated"`
	GuardStatus         int  `json:"guard_status"`
	GuardStatusObserved bool `json:"guard_status_observed"`
}
type launcherCheckpoint struct {
	Name          string `json:"name,omitempty"`
	Error         string `json:"error,omitempty"`
	IOUringDenied bool   `json:"io_uring_denied,omitempty"`
}

type selfAttackRequest struct {
	Root   string `json:"root"`
	Helper string `json:"helper"`
	Report string `json:"report"`
}
type selfAttackResult struct {
	Status          int  `json:"status"`
	Replaced        bool `json:"replaced"`
	Exchanged       bool `json:"exchanged"`
	ContentRejected bool `json:"content_rejected"`
	Passed          bool `json:"passed"`
}
type reservedFDRequest struct {
	Root       string `json:"root"`
	Executable string `json:"executable"`
	CACert     string `json:"ca_cert"`
	Report     string `json:"report"`
	Inspect    string `json:"inspect"`
	CLOEXEC    bool   `json:"cloexec"`
}
type reservedFDResult struct {
	Status           int  `json:"status"`
	DescriptorStates bool `json:"descriptor_states"`
	DescriptorsExact bool `json:"descriptors_exact"`
	LifecycleExact   bool `json:"lifecycle_exact"`
	Passed           bool `json:"passed"`
}
type launcherRelease struct {
	Release bool `json:"release"`
}

type guardDisarm struct {
	Disarm bool `json:"disarm"`
	Kill   bool `json:"kill,omitempty"`
}

type guardArm struct {
	Armed bool   `json:"armed"`
	Error string `json:"error,omitempty"`
}

// Supervise resolves and opens the target before any child exists, requires a
// cgroup boundary, and then launches the unchanged-argv descriptor-only
// supervisor. The target descriptor is the sole execution authority.
func Supervise(ctx context.Context, proofExecutable string, spec Spec) (Observation, error) {
	ownedCgroup := false
	var cgroup *Cgroup
	var err error
	if spec.CgroupPath == "" {
		cgroup, err = NewCgroup()
		if err != nil {
			return Observation{Status: 1}, fmt.Errorf("mandatory cgroup unavailable: %w", err)
		}
		ownedCgroup = true
		spec.CgroupPath = cgroup.Path()
	} else {
		cgroup = &Cgroup{path: spec.CgroupPath}
	}
	activateFDChurn, stopFDChurn, churnErr := churnFDs(spec.Dir)
	if churnErr != nil {
		if ownedCgroup {
			_ = cgroup.Close()
		}
		return Observation{Status: 1}, fmt.Errorf("descriptor churn setup: %w", churnErr)
	}
	activateFDChurn()
	obs, runErr := superviseInCgroup(ctx, proofExecutable, spec)
	var observeErr error
	if spec.CgroupInherited {
		expected, identityErr := cgroupIdentityAtPath(spec.CgroupPath)
		obs.CgroupObserved = identityErr == nil && processInCgroupIdentity(expected)
		obs.CgroupCleanupDeferred = obs.CgroupObserved
		if !obs.CgroupObserved {
			observeErr = errors.New("inherited cgroup membership unobserved")
		}
	} else {
		var cgroupObs CgroupObservation
		cgroupObs, observeErr = cgroup.Observe()
		obs.CgroupObserved = observeErr == nil
		obs.CgroupEmpty = observeErr == nil && cgroupObs.Empty
		obs.MemoryPeak = cgroupObs.MemoryPeak
		obs.ProcessPeak = cgroupObs.ProcessPeak
		obs.CPUUsec = cgroupObs.CPUUsec
		obs.MemoryOOM = cgroupObs.MemoryOOM
		obs.OOMKill = cgroupObs.OOMKill
		if observeErr == nil && (cgroupObs.MemoryOOM != 0 || cgroupObs.OOMKill != 0) {
			observeErr = errors.New("managed cgroup recorded an OOM event")
		}
	}
	var closeErr error
	if ownedCgroup {
		closeErr = cgroup.Close()
	}
	obs.FDChurnComplete = stopFDChurn()
	if runErr == nil && observeErr != nil {
		runErr = observeErr
	}
	if runErr == nil && closeErr != nil {
		runErr = closeErr
	}
	if runErr == nil && !obs.FDChurnComplete {
		runErr = errors.New("descriptor churn matrix incomplete")
	}
	return obs, runErr
}

func superviseInCgroup(ctx context.Context, proofExecutable string, spec Spec) (Observation, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		return Observation{}, fmt.Errorf("outer subreaper setup: %v", errno)
	}
	if !isCanonicalAbsolute(proofExecutable) {
		return Observation{}, errors.New("supervisor requires a canonical absolute same-binary identity")
	}
	source, identity, err := openValidatedExecutable(spec.Executable, spec.Env)
	if err != nil {
		return Observation{Status: 1}, fmt.Errorf("resolve managed executable: %w", err)
	}
	defer source.Close()
	target, err := sealExecutableSnapshot(source, identity)
	if err != nil {
		return Observation{Status: 1}, fmt.Errorf("seal managed executable snapshot: %w", err)
	}
	defer target.Close()
	self, selfIdentity, err := openValidatedExecutable(proofExecutable, nil)
	if err != nil {
		return Observation{Status: 1}, fmt.Errorf("open same-binary supervisor identity: %w", err)
	}
	defer self.Close()
	if spec.preLaunchMutation != nil {
		if err := spec.preLaunchMutation(identity.Path, &identity); err != nil {
			return Observation{Status: 1}, fmt.Errorf("pre-launch identity race: %w", err)
		}
	}
	parent, child, err := socketPair()
	if err != nil {
		return Observation{}, fmt.Errorf("control socket: %w", err)
	}
	defer parent.Close()
	cmd := exec.Command("/proc/self/fd/5")
	cmd.Env = []string{"PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	cmd.Dir = spec.Dir
	if spec.preSelfMutation != nil {
		if err := spec.preSelfMutation(selfIdentity.Path); err != nil {
			return Observation{Status: 1}, fmt.Errorf("same-binary identity attack: %w", err)
		}
	}
	cmd.Stdin = spec.Stdin
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	cmd.ExtraFiles = []*os.File{child, target, self}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	if err = cmd.Start(); err != nil {
		child.Close()
		return Observation{Status: 1}, fmt.Errorf("start supervisor: %w", err)
	}
	child.Close()
	challengeBytes, err := readFrame(parent, 256)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Observation{Status: 1}, fmt.Errorf("read supervisor challenge: %w", err)
	}
	var challenge wireChallenge
	if err = decodeStrict(challengeBytes, &challenge); err != nil || !validNonce(challenge.Nonce) || !validInheritedParentDeathSignal(challenge.ParentDeathSignal, syscall.SIGKILL) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Observation{Status: 1}, errors.New("invalid supervisor challenge")
	}
	wire := wireSpec{ParentPID: os.Getpid(), Executable: identity, Arguments: append([]string(nil), spec.Args...), Directory: spec.Dir, Environment: append([]string(nil), spec.Env...), ReadyFile: spec.ReadyFile, CgroupPath: spec.CgroupPath, CgroupInherited: spec.CgroupInherited, CgroupPathSubstitution: spec.CgroupPathSubstitution, FileSizeLimit: spec.FileSizeLimit, InjectionGate: spec.InjectionGate, DenyIOUring: spec.DenyIOUring, InteractivePTY: spec.InteractivePTY, MutationBeforeRelease: spec.beforeReleaseMutation != nil, ChildSignal: int(spec.ChildSignal), Self: selfIdentity, GuardFailureStatus: spec.GuardFailureStatus}
	if err = validateWireSpec(wire); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Observation{Status: 1}, fmt.Errorf("validate supervisor spec: %w", err)
	}
	stage := internalStageSpec{Nonce: challenge.Nonce, Stage: "supervisor", ParentPID: os.Getpid(), Process: &wire, Self: selfIdentity}
	if err = writeFrame(parent, stage); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Observation{Status: 1}, fmt.Errorf("send supervisor spec: %w", err)
	}
	dec := json.NewDecoder(parent)
	var ready wireReady
	if err = dec.Decode(&ready); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return Observation{Status: 1}, fmt.Errorf("read supervisor readiness: %w", err)
	}
	obs := Observation{
		Status:              1,
		Ready:               ready.Ready,
		Session:             ready.Ready && ready.Session == ready.SupervisorPID,
		Subreaper:           ready.Subreaper,
		ParentChecked:       ready.ParentChecked,
		ParentDeathSignal:   ready.ChildParentDeath,
		ProcessGroup:        ready.Ready && ready.ChildPID > 0 && ready.Group == ready.SupervisorPID && ready.ChildGroup == ready.ChildPID,
		GuardPID:            ready.GuardPID,
		GuardArmed:          ready.GuardArmed,
		ChildPID:            ready.ChildPID,
		ChildGroup:          ready.ChildGroup,
		GateReached:         ready.Gate == spec.InjectionGate && ready.Gate != "",
		LeaderObserved:      ready.LeaderObserved,
		DescendantsObserved: ready.DescendantCount,
		IOUringDenied:       ready.IOUringDenied,
		CgroupKillGuard:     ready.CgroupKillGuard,
	}
	if ready.Error != "" {
		_ = cmd.Wait()
		return obs, errors.New(ready.Error)
	}
	if spec.beforeReleaseMutation != nil {
		if ready.Gate != "source-mutation-before-release" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return obs, errors.New("source mutation checkpoint was not reached")
		}
		if mutationErr := spec.beforeReleaseMutation(identity.Path); mutationErr != nil {
			_ = json.NewEncoder(parent).Encode(launcherRelease{Release: false})
			_ = cmd.Wait()
			return obs, fmt.Errorf("source mutation checkpoint: %w", mutationErr)
		}
		if err := json.NewEncoder(parent).Encode(launcherRelease{Release: true}); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return obs, errors.New("acknowledge source mutation checkpoint")
		}
	}
	forward := make(chan os.Signal, 4)
	signal.Notify(forward, catchableSignals()...)
	defer signal.Stop(forward)
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	if spec.HardKillAfterReady {
		return observeWrapperHardDeath(cmd, wait, ready, spec, obs)
	}
	if spec.WrapperSignal != 0 {
		if killErr := syscall.Kill(cmd.Process.Pid, spec.WrapperSignal); killErr != nil {
			_ = cmd.Process.Kill()
			<-wait
			return obs, fmt.Errorf("inject wrapper signal: %w", killErr)
		}
	}
	var requested syscall.Signal
	select {
	case <-wait:
	case sig := <-forward:
		requested = sig.(syscall.Signal)
		obs.Forwarded = syscall.Kill(cmd.Process.Pid, requested) == nil
		<-wait
	case <-ctx.Done():
		obs.Forwarded = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM) == nil
		<-wait
		requested = -1
	}
	var result wireResult
	if decodeErr := dec.Decode(&result); decodeErr != nil {
		return obs, fmt.Errorf("read supervisor result: %w", decodeErr)
	}
	obs.Status = result.Status
	obs.Reaped = result.Reaped
	obs.Orphans = result.Orphans
	obs.GuardStatus = result.GuardStatus
	obs.GuardStatusObserved = result.GuardStatusObserved
	if spec.WrapperSignal != 0 {
		obs.Forwarded = result.Forwarded
	} else {
		obs.Forwarded = obs.Forwarded && result.Forwarded
	}
	obs.Escalated = result.Escalated
	obs.GuardCleanup = ready.GuardArmed && !processExists(ready.GuardPID)
	if !obs.GuardCleanup {
		obs.SurvivorsDetected++
		obs.Orphans++
	}
	if requested == -1 {
		obs.Status = 1
	}
	if !obs.GuardStatusObserved || obs.GuardStatus != 0 {
		return obs, fmt.Errorf("death guard status unverified: observed=%t status=%d", obs.GuardStatusObserved, obs.GuardStatus)
	}
	return obs, nil
}

func observeWrapperHardDeath(cmd *exec.Cmd, wait <-chan error, ready wireReady, spec Spec, obs Observation) (Observation, error) {
	if spec.InjectionGate != "" && !obs.GateReached {
		_ = cmd.Process.Kill()
		<-wait
		return obs, fmt.Errorf("requested injection gate %q was not reached", spec.InjectionGate)
	}
	if killErr := syscall.Kill(cmd.Process.Pid, syscall.SIGKILL); killErr != nil {
		<-wait
		return obs, fmt.Errorf("inject wrapper hard death: %w", killErr)
	}
	<-wait
	obs.HardDeath = true
	deadline := time.Now().Add(3 * time.Second)
	guardStatus := -1
	reap := func() {
		obs.Reaped += reapAvailableWithGuard(ready.GuardPID, &guardStatus)
	}
	for ((ready.ChildGroup > 0 && processGroupExists(ready.ChildGroup)) || processExists(ready.GuardPID)) && time.Now().Before(deadline) {
		reap()
		time.Sleep(time.Millisecond)
	}
	reap()
	obs.GuardStatus = guardStatus
	obs.GuardStatusObserved = guardStatus >= 0
	obs.GuardCleanup = ready.GuardArmed &&
		(ready.ChildGroup <= 0 || !processGroupExists(ready.ChildGroup)) && !processExists(ready.GuardPID)
	if !obs.GuardCleanup {
		obs.SurvivorsDetected = 1
		obs.Orphans = 1
	}
	if !obs.GuardStatusObserved || guardStatus != 0 {
		return obs, fmt.Errorf("death guard exited with status %d", guardStatus)
	}
	return obs, nil
}

// TrySupervisorMode authenticates the reserved descriptor before inspecting a
// bounded stage frame. Public argv is never consumed or changed by dispatch.
func TrySupervisorMode() (bool, int) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	parentPID, err := validateInternalControl(supervisorControlFD)
	if err != nil {
		return false, 0
	}
	control := os.NewFile(supervisorControlFD, "proof-control")
	if control == nil {
		return false, 0
	}
	_ = control.SetDeadline(time.Now().Add(2 * time.Second))
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		return true, 1
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	inheritedParentDeathSignal := parentDeathSignal()
	if err := writeFrame(control, wireChallenge{Nonce: nonce, ParentDeathSignal: int(inheritedParentDeathSignal)}); err != nil {
		return true, 1
	}
	stageBytes, err := readFrame(control, maxControlSpecBytes)
	if err != nil {
		return true, 1
	}
	var stage internalStageSpec
	if err := decodeStrict(stageBytes, &stage); err != nil || validateNonce(stage.Nonce, nonce) != nil || stage.ParentPID != parentPID {
		return true, 1
	}
	_ = control.SetDeadline(time.Time{})
	switch stage.Stage {
	case "supervisor", "launcher":
		parentDeathErr := establishStageParentDeath(supervisorControlFD, parentPID, inheritedParentDeathSignal, syscall.SIGKILL)
		stageProblem := ""
		switch {
		case parentDeathErr != nil:
			stageProblem = "parent-death relation: " + parentDeathErr.Error()
		case stage.Process == nil:
			stageProblem = "missing process specification"
		case stage.Group != 0:
			stageProblem = "unexpected group specification"
		case validateWireSpec(*stage.Process) != nil:
			stageProblem = "invalid process specification"
		case stage.Process.ParentPID != parentPID:
			stageProblem = "parent identity"
		}
		if stageProblem != "" {
			message := "pre-dispatch " + stageProblem + " validation failed"
			if stage.Stage == "launcher" {
				_ = writeFrame(control, launcherCheckpoint{Error: message})
			} else {
				_ = sendReadyError(control, message)
			}
			return true, 1
		}
		if err := sanitizeInheritedDescriptors(supervisorControlFD, targetExecutableFD, selfExecutableFD); err != nil {
			if stage.Stage == "launcher" {
				_ = writeFrame(control, launcherCheckpoint{Error: "pre-dispatch descriptor sanitation failed"})
			} else {
				_ = sendReadyError(control, "pre-dispatch descriptor sanitation failed")
			}
			return true, 1
		}
		if err := validateDescriptorAllowlist(supervisorControlFD, targetExecutableFD, selfExecutableFD); err != nil {
			if stage.Stage == "launcher" {
				_ = writeFrame(control, launcherCheckpoint{Error: "pre-dispatch descriptor allowlist failed"})
			} else {
				_ = sendReadyError(control, "pre-dispatch descriptor allowlist failed")
			}
			return true, 1
		}
		target := os.NewFile(targetExecutableFD, "verified-target-snapshot")
		self := os.NewFile(selfExecutableFD, "verified-self")
		if target == nil || self == nil || validateSnapshotFD(target, stage.Process.Executable) != nil || validateSelfFD(self, stage.Self) != nil || !sameIdentity(stage.Self, stage.Process.Self) {
			if stage.Stage == "launcher" {
				_ = writeFrame(control, launcherCheckpoint{Error: "pre-dispatch executable descriptor identity invalid"})
			} else {
				_ = sendReadyError(control, "pre-dispatch executable descriptor identity invalid")
			}
			return true, 1
		}
		if stage.Stage == "supervisor" {
			return true, supervisorMode(control, target, self, *stage.Process, parentPID)
		}
		return true, childLauncher(control, target, self, *stage.Process, parentPID)
	case "guard":
		failGuard := func(message string) (bool, int) {
			_ = writeFrame(control, guardArm{Error: message})
			return true, 1
		}
		if stage.Process != nil || stage.Group < 0 {
			return failGuard("guard-stage-specification")
		}
		signal.Ignore(syscall.SIGUSR1)
		if establishStageParentDeath(supervisorControlFD, parentPID, inheritedParentDeathSignal, syscall.SIGUSR1) != nil {
			return failGuard("guard-parent-death-relation")
		}
		if err := sanitizeInheritedDescriptors(supervisorControlFD, guardSelfFD, selfExecutableFD); err != nil {
			return failGuard("guard-descriptor-sanitation")
		}
		if err := validateDescriptorAllowlist(supervisorControlFD, guardSelfFD, selfExecutableFD); err != nil {
			return failGuard("guard-descriptor-allowlist")
		}
		self := os.NewFile(guardSelfFD, "verified-guard-self")
		cgroupFile := os.NewFile(selfExecutableFD, "verified-guard-cgroup")
		if self == nil || cgroupFile == nil {
			return failGuard("guard-descriptor-files")
		}
		if err := validateSelfFD(self, stage.Self); err != nil {
			return failGuard("guard-self-identity")
		}
		if err := validateCgroupDescriptor(cgroupFile, stage.Cgroup); err != nil {
			return failGuard("guard-cgroup-identity")
		}
		return true, deathGuardMode(control, self, cgroupFile, stage.Group, stage.GuardFailureStatus)
	default:
		return true, 1
	}
}

func supervisorMode(control, target, self *os.File, spec wireSpec, firstParent int) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer control.Close()
	defer target.Close()
	defer self.Close()
	if os.Getppid() != firstParent {
		return sendReadyError(control, "parent identity changed before lifecycle setup")
	}
	if validateSnapshotFD(target, spec.Executable) != nil || validateSelfFD(self, spec.Self) != nil {
		return sendReadyError(control, "managed executable identity invalid")
	}
	syscall.CloseOnExec(supervisorControlFD)
	syscall.CloseOnExec(targetExecutableFD)
	syscall.CloseOnExec(selfExecutableFD)
	cgroupFD, err := unix.Open(spec.CgroupPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return sendReadyError(control, "open delegated cgroup authority failed")
	}
	cgroupFile := os.NewFile(uintptr(cgroupFD), "delegated-cgroup-authority")
	if cgroupFile == nil {
		_ = syscall.Close(cgroupFD)
		return sendReadyError(control, "open delegated cgroup authority failed")
	}
	defer cgroupFile.Close()
	cgroupID, err := cgroupDescriptorIdentity(cgroupFile)
	if err != nil {
		return sendReadyError(control, "validate delegated cgroup authority failed")
	}
	if spec.CgroupInherited {
		contained, containErr := cgroupDescriptorContainsPID(cgroupFile, os.Getpid())
		if containErr != nil || !contained {
			return sendReadyError(control, "inherited cgroup membership unobserved")
		}
	}
	spec.Cgroup = cgroupID
	var substitutionFile *os.File
	var substitutionID cgroupIdentity
	if spec.CgroupPathSubstitution {
		substitutionFile, substitutionID, err = createCgroupPathSubstitute(spec.CgroupPath, cgroupID)
		if err != nil {
			return sendReadyError(control, err.Error())
		}
		spec.CgroupPath += ".substitute"
		defer func() {
			_ = substitutionFile.Close()
			_ = os.Remove(spec.CgroupPath)
		}()
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		return sendReadyError(control, "subreaper setup unavailable")
	}
	if os.Getppid() != firstParent {
		return sendReadyError(control, "parent identity changed after lifecycle setup")
	}
	if spec.InteractivePTY {
		if _, _, errno := syscall.RawSyscall(syscall.SYS_IOCTL, 0, unix.TIOCSCTTY, 0); errno != 0 {
			return sendReadyError(control, "acquire interactive controlling terminal failed")
		}
	}
	guardRead, guardWrite, err := socketPair()
	if err != nil {
		return sendReadyError(control, "death guard channel unavailable")
	}
	guard := exec.Command("/proc/self/fd/4")
	guard.Env = []string{"PATH=/nonexistent", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	guard.Dir, guard.Stdout, guard.Stderr, guard.ExtraFiles = spec.Directory, io.Discard, io.Discard, []*os.File{guardRead, self, cgroupFile}
	signal.Ignore(syscall.SIGUSR1)
	guard.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGUSR1}
	startErr := guard.Start()
	signal.Reset(syscall.SIGUSR1)
	if startErr != nil {
		guardRead.Close()
		guardWrite.Close()
		return sendReadyError(control, "death guard launch failed")
	}
	guardRead.Close()
	if err := authenticateChildStage(guardWrite, internalStageSpec{Stage: "guard", ParentPID: os.Getpid(), Group: 0, Self: spec.Self, Cgroup: cgroupID, GuardFailureStatus: spec.GuardFailureStatus}); err != nil {
		guardWrite.Close()
		_ = guard.Process.Kill()
		_ = guard.Wait()
		return sendReadyError(control, "death guard authentication failed")
	}
	closeGuard := func(kill ...bool) (int, error) {
		return closeDeathGuard(guardWrite, guard, kill...)
	}
	discardGuard := func(kill ...bool) {
		_, _ = closeGuard(kill...)
	}
	gate := func(name string, childPID, childGroup int, armed bool) {
		if spec.InjectionGate != name {
			return
		}
		leader, descendants := observeGroup(childGroup, childPID)
		_ = json.NewEncoder(control).Encode(wireReady{Ready: true, Gate: name, SupervisorPID: os.Getpid(), Session: sessionID(), Group: syscall.Getpgrp(), ChildPID: childPID, ChildGroup: childGroup, ParentChecked: os.Getppid() == firstParent, Subreaper: true, ChildParentDeath: childPID > 0, GuardPID: guard.Process.Pid, GuardArmed: armed, LeaderObserved: leader, DescendantCount: descendants, CgroupKillGuard: true})
		select {}
	}
	if spec.InjectionGate == "before-clone" {
		if err := armDeathGuard(guardWrite, 0, "pre-clone death guard"); err != nil {
			discardGuard()
			return sendReadyError(control, err.Error())
		}
		gate("before-clone", 0, 0, true)
	}
	cmd, launcherControl, err := startManagedLauncher(target, self, cgroupFile, substitutionFile, spec)
	if err != nil {
		discardGuard()
		return sendReadyError(control, err.Error())
	}
	closeLauncherChannels := func() { launcherControl.Close() }
	if err := armDeathGuard(guardWrite, cmd.Process.Pid, "death guard"); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		closeLauncherChannels()
		discardGuard()
		return sendReadyError(control, err.Error())
	}
	gate("after-launcher-start", cmd.Process.Pid, cmd.Process.Pid, true)
	ioUringDenied, err := awaitLauncherCheckpoints(launcherControl, spec, cmd.Process.Pid, gate)
	if err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		closeLauncherChannels()
		discardGuard()
		return sendReadyError(control, err.Error())
	}
	leaderBeforeExec, _ := observeGroup(cmd.Process.Pid, cmd.Process.Pid)
	if !leaderBeforeExec {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		closeLauncherChannels()
		discardGuard()
		return sendReadyError(control, "managed launcher leader disappeared before exec release")
	}
	readySent := false
	if spec.MutationBeforeRelease {
		ready := wireReady{Ready: true, Gate: "source-mutation-before-release", SupervisorPID: os.Getpid(), Session: sessionID(), Group: syscall.Getpgrp(), ChildPID: cmd.Process.Pid, ChildGroup: cmd.Process.Pid, ParentChecked: os.Getppid() == firstParent, Subreaper: true, ChildParentDeath: true, GuardPID: guard.Process.Pid, GuardArmed: true, LeaderObserved: leaderBeforeExec, IOUringDenied: ioUringDenied, CgroupKillGuard: true}
		if err := json.NewEncoder(control).Encode(ready); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			closeLauncherChannels()
			discardGuard()
			return 1
		}
		var release launcherRelease
		if err := json.NewDecoder(control).Decode(&release); err != nil || !release.Release {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			closeLauncherChannels()
			discardGuard()
			return sendReadyError(control, "source mutation checkpoint acknowledgement failed")
		}
		readySent = true
	}
	if err := writeFrame(launcherControl, launcherRelease{Release: true}); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		closeLauncherChannels()
		discardGuard()
		return sendReadyError(control, "managed exec release failed")
	}
	closeLauncherChannels()
	if spec.ReadyFile != "" {
		if err := waitForReadyFile(spec.ReadyFile, time.Second); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			discardGuard()
			return sendReadyError(control, err.Error())
		}
	}
	leaderAfterExec, descendants := observeGroup(cmd.Process.Pid, cmd.Process.Pid)
	leaderObserved := leaderBeforeExec || leaderAfterExec
	if spec.ReadyFile != "" && descendants < 1 && !spec.InteractivePTY {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		discardGuard()
		return sendReadyError(control, "ready descendant was not observed in the exact child group")
	}
	gate("after-first-descendant", cmd.Process.Pid, cmd.Process.Pid, true)
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, catchableSignals()...)
	defer signal.Stop(sigs)
	if !readySent {
		ready := wireReady{Ready: true, SupervisorPID: os.Getpid(), Session: sessionID(), Group: syscall.Getpgrp(), ChildPID: cmd.Process.Pid, ChildGroup: processGroup(cmd.Process.Pid), ParentChecked: os.Getppid() == firstParent, Subreaper: true, ChildParentDeath: true, GuardPID: guard.Process.Pid, GuardArmed: true, LeaderObserved: leaderObserved, DescendantCount: descendants, IOUringDenied: ioUringDenied, CgroupKillGuard: true}
		if err := json.NewEncoder(control).Encode(ready); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			discardGuard()
			return 1
		}
	}
	if spec.ChildSignal != 0 {
		if err := waitForManagedImage(cmd.Process.Pid, spec.Executable, time.Second); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			discardGuard()
			_ = json.NewEncoder(control).Encode(wireResult{Status: 1})
			return 1
		}
		if err := syscall.Kill(cmd.Process.Pid, syscall.Signal(spec.ChildSignal)); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			discardGuard()
			_ = json.NewEncoder(control).Encode(wireResult{Status: 1})
			return 1
		}
	}
	return finishManagedProcess(control, cgroupFile, substitutionFile, substitutionID, cmd, sigs, spec.CgroupInherited, closeGuard)
}

func armDeathGuard(guardWrite *os.File, group int, context string) error {
	if err := writeFrame(guardWrite, group); err != nil {
		return fmt.Errorf("%s arm failed", context)
	}
	armedBytes, armErr := readFrame(guardWrite, 256)
	var armed guardArm
	decodeErr := decodeStrict(armedBytes, &armed)
	if armErr != nil || decodeErr != nil || !armed.Armed || armed.Error != "" {
		return fmt.Errorf("%s acknowledgement failed: read=%v decode=%v stage=%s", context, armErr, decodeErr, armed.Error)
	}
	return nil
}

func closeDeathGuard(guardWrite *os.File, guard *exec.Cmd, kill ...bool) (int, error) {
	shouldKill := len(kill) != 0 && kill[0]
	writeErr := writeFrame(guardWrite, guardDisarm{Disarm: !shouldKill, Kill: shouldKill})
	closeErr := guardWrite.Close()
	waitErr := guard.Wait()
	status := waitStatus(waitErr)
	if writeErr != nil {
		return status, writeErr
	}
	if closeErr != nil {
		return status, closeErr
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return status, waitErr
	}
	return status, nil
}

func createCgroupPathSubstitute(cgroupPath string, cgroupID cgroupIdentity) (*os.File, cgroupIdentity, error) {
	replacementPath := cgroupPath + ".substitute"
	if err := os.Mkdir(replacementPath, 0700); err != nil {
		return nil, cgroupIdentity{}, errors.New("create delegated cgroup path substitute failed")
	}
	replacementFD, openErr := unix.Open(replacementPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if openErr != nil {
		_ = os.Remove(replacementPath)
		return nil, cgroupIdentity{}, errors.New("open delegated cgroup path substitute failed")
	}
	substitutionFile := os.NewFile(uintptr(replacementFD), "delegated-cgroup-path-substitute")
	if substitutionFile == nil {
		_ = syscall.Close(replacementFD)
		_ = os.Remove(replacementPath)
		return nil, cgroupIdentity{}, errors.New("open delegated cgroup path substitute failed")
	}
	substitutionID, err := cgroupDescriptorIdentity(substitutionFile)
	if err != nil || substitutionID == cgroupID {
		_ = substitutionFile.Close()
		_ = os.Remove(replacementPath)
		return nil, cgroupIdentity{}, errors.New("validate delegated cgroup path substitute failed")
	}
	return substitutionFile, substitutionID, nil
}

func finishManagedProcess(control, cgroupFile, substitutionFile *os.File, substitutionID cgroupIdentity, cmd *exec.Cmd, sigs <-chan os.Signal, cgroupInherited bool, closeGuard func(...bool) (int, error)) int {
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var childErr error
	var requested syscall.Signal
	escalated := false
	select {
	case childErr = <-wait:
	case s := <-sigs:
		requested = s.(syscall.Signal)
		_ = syscall.Kill(-cmd.Process.Pid, requested)
		select {
		case childErr = <-wait:
		case <-time.After(GracePeriod):
			escalated = true
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			childErr = <-wait
		}
	}
	result := wireResult{Status: waitStatus(childErr), Forwarded: requested != 0, Escalated: escalated}
	if requested != 0 {
		result.Status = 128 + int(requested)
	}
	var cleanupEscalated bool
	result.Reaped, result.Orphans, cleanupEscalated = terminateAndReap(cmd.Process.Pid)
	result.Escalated = result.Escalated || cleanupEscalated
	var substitutionErr error
	if substitutionFile != nil {
		if err := validateCgroupDescriptor(substitutionFile, substitutionID); err != nil {
			substitutionErr = err
		} else if members, err := cgroupDescriptorHasMembers(substitutionFile); err != nil {
			substitutionErr = err
		} else if members {
			substitutionErr = errors.New("substituted cgroup path gained a member")
		}
	}
	var cleanupGuardErr error
	if !cgroupInherited {
		hasMembers, membershipErr := cgroupDescriptorHasMembers(cgroupFile)
		if membershipErr == nil && hasMembers {
			killFD, killErr := unix.Openat(int(cgroupFile.Fd()), "cgroup.kill", unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if killErr == nil {
				_, killErr = syscall.Write(killFD, []byte("1"))
				_ = syscall.Close(killFD)
			}
			deadline := time.Now().Add(GracePeriod)
			for killErr == nil && time.Now().Before(deadline) {
				hasMembers, membershipErr = cgroupDescriptorHasMembers(cgroupFile)
				if membershipErr != nil || !hasMembers {
					break
				}
				result.Reaped += reapAvailable()
				time.Sleep(time.Millisecond)
			}
			result.Escalated = true
			if membershipErr == nil {
				hasMembers, membershipErr = cgroupDescriptorHasMembers(cgroupFile)
			}
			if membershipErr != nil || hasMembers {
				result.Orphans++
			}
			for {
				reaped := reapAvailable()
				result.Reaped += reaped
				if reaped == 0 {
					break
				}
			}
			if killErr != nil || membershipErr != nil || hasMembers {
				result.GuardStatus, cleanupGuardErr = closeGuard(true)
				if cleanupGuardErr == nil {
					switch {
					case killErr != nil:
						cleanupGuardErr = killErr
					case membershipErr != nil:
						cleanupGuardErr = membershipErr
					default:
						cleanupGuardErr = errors.New("managed cgroup remained populated after supervisor kill")
					}
				}
			} else {
				result.GuardStatus, cleanupGuardErr = closeGuard()
			}
		} else if membershipErr != nil {
			result.GuardStatus, cleanupGuardErr = closeGuard(true)
			if cleanupGuardErr == nil {
				cleanupGuardErr = membershipErr
			}
		} else {
			result.GuardStatus, cleanupGuardErr = closeGuard()
		}
	} else {
		result.GuardStatus, cleanupGuardErr = closeGuard()
	}
	result.GuardStatusObserved = cleanupGuardErr == nil
	if cleanupGuardErr == nil {
		cleanupGuardErr = substitutionErr
	}
	if cleanupGuardErr != nil || result.GuardStatus != 0 {
		result.Status = 1

	}
	_ = json.NewEncoder(control).Encode(result)
	return result.Status
}

func awaitLauncherCheckpoints(launcherControl *os.File, spec wireSpec, childPID int, gate func(string, int, int, bool)) (bool, error) {
	ioUringDenied := false
	for _, checkpoint := range []string{"after-descriptor-sanitization", "after-parent-death-validation", "after-group-session-validation", "after-second-parent-check", "before-exec-release"} {
		checkpointBytes, readErr := readFrame(launcherControl, 1024)
		var reached launcherCheckpoint
		decodeErr := readErr
		if decodeErr == nil {
			decodeErr = decodeStrict(checkpointBytes, &reached)
		}
		if decodeErr != nil || reached.Error != "" || reached.Name != checkpoint {
			return false, fmt.Errorf("managed launcher checkpoint failed: %s: decode=%v launcher=%s", checkpoint, decodeErr, reached.Error)
		}
		if checkpoint == "before-exec-release" {
			ioUringDenied = reached.IOUringDenied
			if ioUringDenied != spec.DenyIOUring {
				return false, errors.New("managed launcher io_uring policy mismatch")
			}
		}
		gate(checkpoint, childPID, childPID, true)
	}
	return ioUringDenied, nil
}

func startManagedLauncher(target, self, cgroupFile, substitutionFile *os.File, spec wireSpec) (*exec.Cmd, *os.File, error) {
	launcherControl, launcherChild, err := socketPair()
	if err != nil {
		return nil, nil, errors.New("launcher control channel unavailable")
	}
	closeLauncherChannels := func() { launcherControl.Close() }
	cmd := exec.Command("/proc/self/fd/5")
	cmd.Env, cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = append([]string(nil), spec.Environment...), spec.Directory, os.Stdin, os.Stdout, os.Stderr
	cmd.ExtraFiles = []*os.File{launcherChild, target, self}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	var oldFileSize syscall.Rlimit
	fileSizeChanged := false
	if spec.FileSizeLimit != 0 {
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &oldFileSize); err != nil {
			launcherControl.Close()
			launcherChild.Close()
			return nil, nil, errors.New("read file-size boundary failed")
		}
		limit := oldFileSize
		if spec.FileSizeLimit < limit.Cur {
			limit.Cur = spec.FileSizeLimit
		}
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
			launcherControl.Close()
			launcherChild.Close()
			return nil, nil, errors.New("set file-size boundary failed")
		}
		fileSizeChanged = true
	}
	startErr := cmd.Start()
	launcherChild.Close()
	if fileSizeChanged {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &oldFileSize); err != nil {
			if startErr == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			closeLauncherChannels()
			return nil, nil, errors.New("restore supervisor file-size limit failed")
		}
	}
	if startErr != nil {
		closeLauncherChannels()
		return nil, nil, errors.New("managed launcher clone failed")
	}
	if !spec.CgroupInherited {
		if err := writeCgroupProcess(cgroupFile, cmd.Process.Pid); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			closeLauncherChannels()
			return nil, nil, errors.New("move gated launcher into delegated cgroup failed")
		}
		contained, containErr := cgroupDescriptorContainsPID(cgroupFile, cmd.Process.Pid)
		if containErr != nil || !contained {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			closeLauncherChannels()
			return nil, nil, errors.New("verify gated launcher cgroup membership failed")
		}
		if substitutionFile != nil {
			replacementMembers, replacementErr := cgroupDescriptorHasMembers(substitutionFile)
			if replacementErr != nil || replacementMembers {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				_ = cmd.Wait()
				closeLauncherChannels()
				return nil, nil, errors.New("gated launcher entered substituted cgroup path")
			}
		}
	}
	if spec.InteractivePTY {
		if err := unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, cmd.Process.Pid); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			closeLauncherChannels()
			return nil, nil, errors.New("set interactive foreground process group failed")
		}
	}
	launchSpec := spec
	launchSpec.ParentPID = os.Getpid()
	if err := authenticateChildStage(launcherControl, internalStageSpec{Stage: "launcher", ParentPID: os.Getpid(), Process: &launchSpec, Self: spec.Self}); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		closeLauncherChannels()
		return nil, nil, errors.New("managed launcher authentication failed")
	}
	return cmd, launcherControl, nil
}

func waitForManagedImage(pid int, expected executableIdentity, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		fd, err := syscall.Open("/proc/"+strconv.Itoa(pid)+"/exe", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
		if err == nil {
			file := os.NewFile(uintptr(fd), "managed-executable")
			if file == nil {
				_ = syscall.Close(fd)
				return errors.New("construct managed executable descriptor")
			}
			actual, identityErr := descriptorContentIdentity(file, false)
			closeErr := file.Close()
			if identityErr == nil && closeErr == nil && actual.Size == expected.Size && actual.SHA256 == expected.SHA256 {
				return nil
			}
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("managed target did not reach exact executable image")
}

func sendReadyError(f *os.File, msg string) int {
	_ = json.NewEncoder(f).Encode(wireReady{Error: msg})
	return 1
}
func terminateAndReap(group int) (reaped, orphans int, escalated bool) {
	if processGroupExists(group) {
		_ = syscall.Kill(-group, syscall.SIGTERM)
		deadline := time.Now().Add(GracePeriod)
		for processGroupExists(group) && time.Now().Before(deadline) {
			reaped += reapAvailable()
			time.Sleep(time.Millisecond)
		}
	}
	if processGroupExists(group) {
		escalated = true
		_ = syscall.Kill(-group, syscall.SIGKILL)
	}
	for processGroupExists(group) {
		reaped += reapAvailable()
		time.Sleep(time.Millisecond)
	}
	for {
		n := reapAvailable()
		reaped += n
		if n == 0 {
			break
		}
	}
	if processGroupExists(group) {
		orphans = 1
	}
	return reaped, orphans, escalated
}
func reapAvailable() int {
	count := 0
	for {
		var st syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &st, syscall.WNOHANG, nil)
		if err != nil || pid == 0 {
			return count
		}
		count++
	}
}
func processGroupExists(group int) bool {
	err := syscall.Kill(-group, 0)
	return err == nil || err == syscall.EPERM
}

func observeGroup(group, leaderPID int) (bool, int) {
	if group <= 0 || leaderPID <= 0 {
		return false, 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, 0
	}
	leader := false
	members := 0
	for _, entry := range entries {
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil {
			continue
		}
		pgid, pgErr := syscall.Getpgid(pid)
		if pgErr == nil && pgid == group {
			members++
			leader = leader || pid == leaderPID
		}
	}
	if members == 0 {
		return leader, 0
	}
	return leader, members - 1
}

func reapAvailableWithGuard(guardPID int, guardStatus *int) int {
	count := 0
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err != nil || pid == 0 {
			return count
		}
		count++
		if pid != guardPID {
			continue
		}
		switch {
		case status.Exited():
			*guardStatus = status.ExitStatus()
		case status.Signaled():
			*guardStatus = 128 + int(status.Signal())
		default:
			*guardStatus = 1
		}
	}
}
func processGroup(pid int) int {
	group, err := syscall.Getpgid(pid)
	if err != nil {
		return -1
	}
	return group
}
func catchableSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
}
func sessionID() int {
	r, _, e := syscall.RawSyscall(syscall.SYS_GETSID, 0, 0, 0)
	if e != 0 {
		return -1
	}
	return int(r)
}
func waitForReadyFile(path string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return errors.New("managed descendant did not become ready")
}
func waitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if st, ok := ee.Sys().(syscall.WaitStatus); ok {
			if st.Exited() {
				return st.ExitStatus()
			}
			if st.Signaled() {
				return 128 + int(st.Signal())
			}
		}
	}
	return 1
}
func socketPair() (*os.File, *os.File, error) {
	fds, e := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if e != nil {
		return nil, nil, e
	}
	return os.NewFile(uintptr(fds[0]), "proof-parent"), os.NewFile(uintptr(fds[1]), "proof-child"), nil
}
func isCanonicalAbsolute(p string) bool {
	if p == "" || !filepathIsAbs(p) {
		return false
	}
	c, e := filepathEval(p)
	return e == nil && c == p
}

// Kept as tiny variables so this Linux-only harness has no path rewriting.
var filepathIsAbs = func(p string) bool { return len(p) > 0 && p[0] == '/' }
var filepathEval = func(p string) (string, error) { return filepath.EvalSymlinks(p) }

func sanitizeInheritedDescriptors(controlFDs ...int) error {
	allowed := make(map[int]bool, len(controlFDs))
	for _, fd := range controlFDs {
		allowed[fd] = true
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fd, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || fd <= 2 || allowed[fd] {
			continue
		}
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFD), 0)
		if errno == syscall.EBADF {
			continue
		}
		if errno != 0 {
			return errno
		}
		if flags&syscall.FD_CLOEXEC == 0 {
			if err := syscall.Close(fd); err != nil && !errors.Is(err, syscall.EBADF) {
				return err
			}
		}
	}
	return nil
}

func descriptorTargetClass(target string) string {
	switch {
	case strings.HasPrefix(target, "anon_inode:"):
		return "anon-inode"
	case strings.HasPrefix(target, "pipe:"):
		return "pipe"
	case strings.HasPrefix(target, "socket:"):
		return "socket"
	case strings.HasPrefix(target, "/dev/"):
		return "device"
	case strings.HasPrefix(target, "/proc/"):
		return "proc"
	case filepath.IsAbs(target):
		return "absolute-file"
	default:
		return "other"
	}
}
func validateInternalControl(fd int) (int, error) {
	if err := validateControlSocket(fd); err != nil {
		return 0, err
	}
	parentPID := os.Getppid()
	if err := validatePeerIdentity(fd, parentPID, uint32(os.Geteuid())); err != nil {
		return 0, err
	}
	if !sameExecutable(parentPID, os.Getpid()) {
		return 0, errors.New("control parent executable identity differs")
	}
	return parentPID, nil
}

func validatePeerIdentity(fd, parentPID int, uid uint32) error {
	peer, err := syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		return err
	}
	if int(peer.Pid) != parentPID || peer.Uid != uid {
		return errors.New("control peer is not the process parent")
	}
	return nil
}

func validateControlSocket(fd int) error {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return errors.New("reserved control descriptor is not a Unix socket")
	}
	var domain int
	domain, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_DOMAIN)
	if err != nil || domain != syscall.AF_UNIX {
		return errors.New("reserved control descriptor is not an AF_UNIX socket")
	}
	return nil
}

func validateDescriptorAllowlist(controlFDs ...int) error {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return err
	}
	observed := make([]int, 0, 3+len(controlFDs))
	for _, entry := range entries {
		fd, convErr := strconv.Atoi(entry.Name())
		if convErr != nil {
			continue
		}
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFD), 0)
		if errno == syscall.EBADF {
			continue
		}
		if errno != 0 {
			return errno
		}
		if flags&syscall.FD_CLOEXEC == 0 {
			observed = append(observed, fd)
		}
	}
	expected := append([]int{0, 1, 2}, controlFDs...)
	sort.Ints(observed)
	sort.Ints(expected)
	if !equalInts(observed, expected) {
		return fmt.Errorf("non-CLOEXEC descriptor allowlist differs: %v", observed)
	}
	return nil
}

func sameExecutable(a, b int) bool {
	openIdentity := func(pid int) (executableIdentity, error) {
		fd, err := syscall.Open("/proc/"+strconv.Itoa(pid)+"/exe", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
		if err != nil {
			return executableIdentity{}, err
		}
		file := os.NewFile(uintptr(fd), "process-executable")
		if file == nil {
			_ = syscall.Close(fd)
			return executableIdentity{}, errors.New("process executable unavailable")
		}
		defer file.Close()
		return descriptorContentIdentity(file, true)
	}
	left, leftErr := openIdentity(a)
	right, rightErr := openIdentity(b)
	return leftErr == nil && rightErr == nil && sameIdentity(left, right)
}

func parentDeathSignal() syscall.Signal {
	var sig int32
	// PR_GET_PDEATHSIG writes one C int; x/sys has no typed getter (ARCH-009).
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prGetParentDeathSig, uintptr(unsafe.Pointer(&sig)), 0) // #nosec G103 -- kernel writes only the live int32 above.
	if errno != 0 {
		return 0
	}
	return syscall.Signal(sig)
}

func validInheritedParentDeathSignal(actual int, expected syscall.Signal) bool {
	return actual == 0 || actual == int(expected)
}

func establishStageParentDeath(fd, parentPID int, inherited, expected syscall.Signal) error {
	if inherited != 0 && inherited != expected {
		return errors.New("unexpected inherited parent-death signal")
	}
	if inherited == 0 {
		if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetParentDeathSig, uintptr(expected), 0); errno != 0 {
			return fmt.Errorf("set parent-death signal: %w", errno)
		}
	}
	if os.Getppid() != parentPID {
		return errors.New("parent changed during parent-death setup")
	}
	if err := validatePeerIdentity(fd, parentPID, uint32(os.Geteuid())); err != nil {
		return err
	}
	if !sameExecutable(parentPID, os.Getpid()) {
		return errors.New("parent executable changed during parent-death setup")
	}
	if parentDeathSignal() != expected {
		return errors.New("parent-death signal unavailable after setup")
	}
	return nil
}

func validNonce(nonce string) bool {
	if len(nonce) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(nonce)
	return err == nil && len(decoded) == 32
}

func validateNonce(actual, expected string) error {
	if !validNonce(actual) || actual != expected {
		return errors.New("control nonce mismatch")
	}
	return nil
}

func validateWireSpec(spec wireSpec) error {
	if spec.ParentPID <= 1 || !isLexicalCanonicalAbsolute(spec.Executable.Path) || !isCanonicalAbsolute(spec.Directory) {
		return errors.New("invalid process identities")
	}
	if spec.Executable.Device == 0 || spec.Executable.Inode == 0 || spec.Executable.Size < 0 || len(spec.Executable.SHA256) != 64 {
		return errors.New("invalid executable identity")
	}
	if _, err := hex.DecodeString(spec.Executable.SHA256); err != nil {
		return errors.New("invalid executable digest")
	}
	if !isCanonicalAbsolute(spec.Self.Path) || spec.Self.Device == 0 || spec.Self.Inode == 0 || spec.Self.Size < 0 || len(spec.Self.SHA256) != 64 || spec.Self.Mode&syscall.S_IFMT != syscall.S_IFREG || spec.Self.Mode&0o111 == 0 {
		return errors.New("invalid same-binary identity")
	}
	if _, err := hex.DecodeString(spec.Self.SHA256); err != nil {
		return errors.New("invalid same-binary digest")
	}
	if spec.MutationBeforeRelease && (spec.ReadyFile != "" || spec.InjectionGate != "") {
		return errors.New("source mutation checkpoint conflicts with another readiness gate")
	}
	if spec.GuardFailureStatus < 0 || spec.GuardFailureStatus > 255 {
		return errors.New("invalid guard failure status")
	}
	if (spec.Cgroup.Device == 0) != (spec.Cgroup.Inode == 0) {
		return errors.New("invalid cgroup identity")
	}
	if spec.CgroupPathSubstitution && spec.CgroupInherited {
		return errors.New("invalid inherited cgroup substitution injection")
	}
	if spec.Executable.Mode&syscall.S_IFMT != syscall.S_IFREG || spec.Executable.Mode&0o111 == 0 {
		return errors.New("invalid executable type or mode")
	}
	if len(spec.Arguments) > 128 || len(spec.Environment) > 256 {
		return errors.New("process specification count exceeds bound")
	}
	total := len(spec.Executable.Path) + len(spec.Executable.SHA256) + len(spec.Self.Path) + len(spec.Self.SHA256) + len(spec.Directory) + len(spec.ReadyFile) + len(spec.CgroupPath) + len(spec.InjectionGate)
	for _, value := range append(append([]string(nil), spec.Arguments...), spec.Environment...) {
		if len(value) > 4096 || strings.IndexByte(value, 0) >= 0 {
			return errors.New("process specification string is invalid")
		}
		total += len(value)
	}
	if total > maxControlSpecBytes-1024 || strings.IndexByte(spec.Executable.Path, 0) >= 0 || strings.IndexByte(spec.Directory, 0) >= 0 {
		return errors.New("process specification size exceeds bound")
	}
	switch spec.InjectionGate {
	case "", "before-clone", "after-launcher-start", "after-descriptor-sanitization", "after-parent-death-validation", "after-group-session-validation", "after-second-parent-check", "before-exec-release", "after-first-descendant":
	default:
		return errors.New("unknown lifecycle injection gate")
	}
	if spec.ChildSignal < 0 || spec.ChildSignal > 64 {
		return errors.New("invalid child signal")
	}
	return nil
}

func isLexicalCanonicalAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func writeFrame(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > maxControlSpecBytes {
		return errors.New("control frame exceeds bound")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func readFrame(r io.Reader, max int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size <= 0 || size > max {
		return nil, errors.New("control frame size is invalid")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeStrict(payload []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple control values")
		}
		return err
	}
	return nil
}

func authenticateChildStage(control *os.File, stage internalStageSpec) error {
	_ = control.SetDeadline(time.Now().Add(2 * time.Second))
	challengeBytes, err := readFrame(control, 256)
	if err != nil {
		return err
	}
	var challenge wireChallenge
	expectedSignal := syscall.SIGKILL
	if stage.Stage == "guard" {
		expectedSignal = syscall.SIGUSR1
	}
	if err := decodeStrict(challengeBytes, &challenge); err != nil || !validNonce(challenge.Nonce) || !validInheritedParentDeathSignal(challenge.ParentDeathSignal, expectedSignal) {
		return errors.New("invalid internal-stage challenge")
	}
	stage.Nonce = challenge.Nonce
	if err := writeFrame(control, stage); err != nil {
		return err
	}
	_ = control.SetDeadline(time.Time{})
	return nil
}

func openValidatedExecutable(name string, environment []string) (*os.File, executableIdentity, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return nil, executableIdentity{}, errors.New("empty executable")
	}
	var resolved string
	if strings.ContainsRune(name, '/') {
		if !isCanonicalAbsolute(name) {
			return nil, executableIdentity{}, errors.New("slash-containing executable must be canonical absolute")
		}
		resolved = name
	} else {
		pathValue := ""
		pathCount := 0
		for _, entry := range environment {
			if strings.HasPrefix(entry, "PATH=") {
				pathValue = strings.TrimPrefix(entry, "PATH=")
				pathCount++
			}
		}
		if pathCount != 1 {
			return nil, executableIdentity{}, errors.New("stable PATH must occur exactly once")
		}
		directories, err := validateStablePATH(pathValue)
		if err != nil {
			return nil, executableIdentity{}, err
		}
		for _, directory := range directories {
			candidate := filepath.Join(directory, name)
			fd, openErr := syscall.Open(candidate, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
			if openErr != nil {
				if errors.Is(openErr, syscall.ENOENT) || errors.Is(openErr, syscall.ENOTDIR) {
					continue
				}
				return nil, executableIdentity{}, errors.New("stable PATH executable open failed")
			}
			file := os.NewFile(uintptr(fd), "resolved-executable")
			identity, identityErr := executableIdentityForFD(file, candidate)
			if identityErr == nil {
				return file, identity, nil
			}
			file.Close()
			return nil, executableIdentity{}, identityErr
		}
		return nil, executableIdentity{}, errors.New("executable absent from stable PATH")
	}
	fd, err := syscall.Open(resolved, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, executableIdentity{}, errors.New("absolute executable open failed")
	}
	file := os.NewFile(uintptr(fd), "resolved-executable")
	identity, err := executableIdentityForFD(file, resolved)
	if err != nil {
		file.Close()
		return nil, executableIdentity{}, err
	}
	return file, identity, nil
}

func validateStablePATH(value string) ([]string, error) {
	if value == "" {
		return nil, errors.New("stable PATH is empty")
	}
	parts := strings.Split(value, ":")
	seenPath := make(map[string]bool, len(parts))
	seenIdentity := make(map[[2]uint64]bool, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || !filepath.IsAbs(part) || filepath.Clean(part) != part {
			return nil, errors.New("stable PATH contains empty, dot, relative, or non-canonical entry")
		}
		canonical, err := filepath.EvalSymlinks(part)
		if err != nil || canonical != part || seenPath[part] {
			return nil, errors.New("stable PATH contains unavailable, aliased, or duplicate entry")
		}
		info, err := os.Stat(part)
		if err != nil || !info.IsDir() {
			return nil, errors.New("stable PATH entry is not a directory")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink == 0 || stat.Uid != 0 && int(stat.Uid) != os.Geteuid() || stat.Uid != uint32(os.Geteuid()) && info.Mode().Perm()&0o022 != 0 {
			return nil, errors.New("stable PATH entry owner or mode is unsafe")
		}
		key := [2]uint64{uint64(stat.Dev), stat.Ino}
		if seenIdentity[key] {
			return nil, errors.New("stable PATH contains duplicate directory identity")
		}
		seenPath[part], seenIdentity[key] = true, true
	}
	return parts, nil
}

func validateLinuxX8664ELF(source *os.File, original executableIdentity) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return errors.New("managed executable image policy is frozen to Linux x86_64")
	}
	var header [20]byte
	if _, err := source.ReadAt(header[:], 0); err != nil {
		return errors.New("managed executable is not a complete ELF64 image")
	}
	if !bytes.Equal(header[0:4], []byte{0x7f, 'E', 'L', 'F'}) || header[4] != 2 || header[5] != 1 || header[6] != 1 {
		return errors.New("managed executable is not little-endian ELF64")
	}
	imageType := binary.LittleEndian.Uint16(header[16:18])
	machine := binary.LittleEndian.Uint16(header[18:20])
	if (imageType != 2 && imageType != 3) || machine != 62 || original.Size < int64(len(header)) {
		return errors.New("managed executable is not an x86_64 ET_EXEC/ET_DYN image")
	}
	return nil
}
func executableIdentityForFD(file *os.File, expectedPath string) (executableIdentity, error) {

	var st syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &st); err != nil {
		return executableIdentity{}, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Mode&0o111 == 0 || st.Nlink == 0 {
		return executableIdentity{}, errors.New("executable is not a linked executable regular file")
	}
	actualPath, err := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(file.Fd())))
	if err != nil || actualPath != expectedPath || !isCanonicalAbsolute(actualPath) {
		return executableIdentity{}, errors.New("opened executable path identity differs")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(file, 0, st.Size))
	if err != nil || n != st.Size {
		return executableIdentity{}, errors.New("executable digest read incomplete")
	}
	return executableIdentity{Path: actualPath, Device: uint64(st.Dev), Inode: st.Ino, Owner: st.Uid, Group: st.Gid, Mode: st.Mode, Size: st.Size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func descriptorContentIdentity(file *os.File, requireLinked bool) (executableIdentity, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &st); err != nil {
		return executableIdentity{}, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Mode&0o111 == 0 || requireLinked && st.Nlink == 0 {
		return executableIdentity{}, errors.New("descriptor is not an executable regular file")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(file, 0, st.Size))
	if err != nil || n != st.Size {
		return executableIdentity{}, errors.New("descriptor digest read incomplete")
	}
	return executableIdentity{Device: uint64(st.Dev), Inode: st.Ino, Owner: st.Uid, Group: st.Gid, Mode: st.Mode, Size: st.Size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func sameIdentity(a, b executableIdentity) bool {
	return a.Device == b.Device && a.Inode == b.Inode && a.Owner == b.Owner && a.Group == b.Group && a.Mode == b.Mode && a.Size == b.Size && a.SHA256 == b.SHA256
}

func validateSelfFD(file *os.File, expected executableIdentity) error {
	actual, err := descriptorContentIdentity(file, true)
	if err != nil {
		return err
	}
	if !sameIdentity(actual, expected) {
		return errors.New("same-binary descriptor identity changed")
	}
	return nil
}

func sealExecutableSnapshot(source *os.File, original executableIdentity) (*os.File, error) {
	if err := validateLinuxX8664ELF(source, original); err != nil {
		return nil, err
	}
	fd, err := unix.MemfdCreate("zerb-proof-executable", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	snapshot := os.NewFile(uintptr(fd), "sealed-executable-snapshot")
	if snapshot == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("create executable snapshot descriptor")
	}
	fail := func(err error) (*os.File, error) {
		snapshot.Close()
		return nil, err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	n, err := io.Copy(snapshot, source)
	if err != nil || n != original.Size {
		return fail(errors.New("executable snapshot copy incomplete"))
	}
	if err := snapshot.Chmod(os.FileMode(original.Mode & 0o777)); err != nil {
		return fail(err)
	}
	const requiredSeals = unix.F_SEAL_WRITE | unix.F_SEAL_GROW | unix.F_SEAL_SHRINK | unix.F_SEAL_SEAL
	if _, err := unix.FcntlInt(snapshot.Fd(), unix.F_ADD_SEALS, requiredSeals); err != nil {
		return fail(err)
	}
	if err := validateSnapshotFD(snapshot, original); err != nil {
		return fail(err)
	}
	return snapshot, nil
}

func validateSnapshotFD(snapshot *os.File, original executableIdentity) error {
	actual, err := descriptorContentIdentity(snapshot, false)
	if err != nil {
		return err
	}
	if actual.Size != original.Size || actual.SHA256 != original.SHA256 || actual.Mode&syscall.S_IFMT != syscall.S_IFREG || actual.Mode&0o111 == 0 {
		return errors.New("sealed executable snapshot content differs")
	}
	const requiredSeals = unix.F_SEAL_WRITE | unix.F_SEAL_GROW | unix.F_SEAL_SHRINK | unix.F_SEAL_SEAL
	seals, err := unix.FcntlInt(snapshot.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || seals&requiredSeals != requiredSeals {
		return errors.New("executable snapshot seals incomplete")
	}
	return nil
}

func execveatFD(fd int, argv, environment []string) error {
	path, err := syscall.BytePtrFromString("")
	if err != nil {
		return err
	}
	argvp, err := syscall.SlicePtrFromStrings(argv)
	if err != nil {
		return err
	}
	envp, err := syscall.SlicePtrFromStrings(environment)
	if err != nil {
		return err
	}
	// x/sys has no execveat wrapper; AT_EMPTY_PATH executes the sealed FD (ARCH-009).
	_, _, errno := unix.RawSyscall6(unix.SYS_EXECVEAT, uintptr(fd), uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&argvp[0])), uintptr(unsafe.Pointer(&envp[0])), uintptr(unix.AT_EMPTY_PATH), 0) // #nosec G103 -- live NUL-terminated path and argv/env pointer arrays use the Linux execveat ABI.
	if errno != 0 {
		return errno
	}
	return nil
}

func cgroupDescriptorIdentity(file *os.File) (cgroupIdentity, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &st); err != nil {
		return cgroupIdentity{}, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Ino == 0 {
		return cgroupIdentity{}, errors.New("cgroup authority is not a directory")
	}
	return cgroupIdentity{Device: uint64(st.Dev), Inode: st.Ino}, nil
}

func validateCgroupDescriptor(file *os.File, expected cgroupIdentity) error {
	actual, err := cgroupDescriptorIdentity(file)
	if err != nil {
		return err
	}
	if actual != expected || expected.Device == 0 || expected.Inode == 0 {
		return errors.New("cgroup authority identity changed")
	}
	return nil
}
func readCgroupDescriptorFile(cgroupFile *os.File, name string, limit int) ([]byte, error) {
	fd, err := unix.Openat(int(cgroupFile.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)
	content := make([]byte, limit+1)
	n, err := syscall.Read(fd, content)
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, errors.New("cgroup control file exceeds proof bound")
	}
	return content[:n], nil
}

func writeCgroupProcess(cgroupFile *os.File, pid int) error {
	fd, err := unix.Openat(int(cgroupFile.Fd()), "cgroup.procs", unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	_, writeErr := syscall.Write(fd, []byte(strconv.Itoa(pid)))
	closeErr := syscall.Close(fd)
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func cgroupDescriptorContainsPID(cgroupFile *os.File, pid int) (bool, error) {
	content, err := readCgroupDescriptorFile(cgroupFile, "cgroup.procs", 1<<20)
	if err != nil {
		return false, err
	}
	expected := strconv.Itoa(pid)
	for _, line := range strings.Fields(string(content)) {
		if line == expected {
			return true, nil
		}
	}
	return false, nil
}

func cgroupDescriptorHasMembers(cgroupFile *os.File) (bool, error) {
	content, err := readCgroupDescriptorFile(cgroupFile, "pids.current", 32)
	if err != nil {
		return false, err
	}
	current, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return false, err
	}
	return current != 0, nil
}

func cgroupIdentityAtPath(path string) (cgroupIdentity, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return cgroupIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), "cgroup-path-identity")
	if file == nil {
		_ = syscall.Close(fd)
		return cgroupIdentity{}, errors.New("construct cgroup path descriptor")
	}
	defer file.Close()
	return cgroupDescriptorIdentity(file)
}

func processInCgroupIdentity(expected cgroupIdentity) bool {
	current, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	fields := strings.SplitN(strings.TrimSpace(string(current)), ":", 3)
	if len(fields) != 3 || fields[0] != "0" || fields[1] != "" || !strings.HasPrefix(fields[2], "/") {
		return false
	}
	fd, err := unix.Open(filepath.Join("/sys/fs/cgroup", filepath.Clean(fields[2])), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	file := os.NewFile(uintptr(fd), "current-cgroup")
	if file == nil {
		_ = syscall.Close(fd)
		return false
	}
	defer file.Close()
	return validateCgroupDescriptor(file, expected) == nil
}

func installIOUringDenial() error {
	if runtime.GOARCH != "amd64" {
		return errors.New("io_uring seccomp policy is frozen to x86_64")
	}
	const (
		bpfLoadWordAbsolute      = 0x20
		bpfJumpEqual             = 0x15
		bpfReturn                = 0x06
		auditArchX8664           = 0xc000003e
		seccompSetModeFilter     = 1
		seccompReturnKillProcess = 0x80000000
		seccompReturnErrno       = 0x00050000
		seccompReturnAllow       = 0x7fff0000
		prSetNoNewPrivileges     = 38
	)
	filters := []unix.SockFilter{
		{Code: bpfLoadWordAbsolute, K: 4},
		{Code: bpfJumpEqual, Jt: 1, Jf: 0, K: auditArchX8664},
		{Code: bpfReturn, K: seccompReturnKillProcess},
		{Code: bpfLoadWordAbsolute, K: 0},
		{Code: bpfJumpEqual, Jt: 0, Jf: 1, K: uint32(unix.SYS_IO_URING_SETUP)},
		{Code: bpfReturn, K: seccompReturnErrno | uint32(syscall.EPERM)},
		{Code: bpfJumpEqual, Jt: 0, Jf: 1, K: uint32(unix.SYS_IO_URING_REGISTER)},
		{Code: bpfReturn, K: seccompReturnErrno | uint32(syscall.EPERM)},
		{Code: bpfJumpEqual, Jt: 0, Jf: 1, K: uint32(unix.SYS_IO_URING_ENTER)},
		{Code: bpfReturn, K: seccompReturnErrno | uint32(syscall.EPERM)},
		{Code: bpfReturn, K: seccompReturnAllow},
	}
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	if _, _, errno := unix.RawSyscall(unix.SYS_PRCTL, prSetNoNewPrivileges, 1, 0); errno != 0 {
		return fmt.Errorf("set no-new-privileges: %w", errno)
	}
	// x/sys has no seccomp wrapper; SockFprog bounds the live filter array (ARCH-009).
	// #nosec G103 -- Linux SECCOMP_SET_MODE_FILTER copies this typed SockFprog synchronously.
	if _, _, errno := unix.RawSyscall(unix.SYS_SECCOMP, seccompSetModeFilter, 0, uintptr(unsafe.Pointer(&program))); errno != 0 {
		return fmt.Errorf("install io_uring seccomp filter: %w", errno)
	}
	return nil
}

func childLauncher(control, target, self *os.File, spec wireSpec, firstParent int) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer control.Close()
	defer target.Close()
	defer self.Close()
	checkpoint := func(name string) bool {
		return writeFrame(control, launcherCheckpoint{Name: name}) == nil
	}
	checkpointError := func(err error) int {
		_ = writeFrame(control, launcherCheckpoint{Error: err.Error()})
		return 1
	}
	if validateSnapshotFD(target, spec.Executable) != nil || validateSelfFD(self, spec.Self) != nil {
		return checkpointError(errors.New("executable descriptor identity invalid"))
	}
	if err := validateDescriptorAllowlist(supervisorControlFD, targetExecutableFD, selfExecutableFD); err != nil {
		return checkpointError(fmt.Errorf("descriptor allowlist: %w", err))
	}
	syscall.CloseOnExec(supervisorControlFD)
	syscall.CloseOnExec(targetExecutableFD)
	syscall.CloseOnExec(selfExecutableFD)
	if !checkpoint("after-descriptor-sanitization") {
		return 1
	}
	if parentDeathSignal() != syscall.SIGKILL || !processInCgroupIdentity(spec.Cgroup) || !checkpoint("after-parent-death-validation") {
		return 1
	}

	if syscall.Getpgrp() != os.Getpid() || sessionID() != firstParent || !checkpoint("after-group-session-validation") {
		return 1
	}
	if os.Getppid() != firstParent || !sameExecutable(firstParent, os.Getpid()) || !checkpoint("after-second-parent-check") {
		return 1
	}
	ioUringDenied := false
	if spec.DenyIOUring {
		if err := installIOUringDenial(); err != nil {
			return checkpointError(err)
		}
		ioUringDenied = true
	}
	execFD, err := unix.FcntlInt(target.Fd(), unix.F_DUPFD_CLOEXEC, selfExecutableFD+1)
	if err != nil {
		return checkpointError(errors.New("descriptor-relative executable reopen failed"))
	}
	execFile := os.NewFile(uintptr(execFD), "same-fd-executable")
	if execFile == nil {
		_ = syscall.Close(execFD)
		return checkpointError(errors.New("descriptor-relative executable reopen failed"))
	}
	defer execFile.Close()
	if err := validateSnapshotFD(execFile, spec.Executable); err != nil {
		return checkpointError(fmt.Errorf("reopened executable snapshot identity: %w", err))
	}
	if writeFrame(control, launcherCheckpoint{Name: "before-exec-release", IOUringDenied: ioUringDenied}) != nil {
		return 1
	}
	releaseBytes, err := readFrame(control, 1024)
	var release launcherRelease
	if err != nil || decodeStrict(releaseBytes, &release) != nil || !release.Release {
		return 1
	}
	syscall.CloseOnExec(supervisorControlFD)
	argv := append([]string{spec.Executable.Path}, spec.Arguments...)
	if err := execveatFD(execFD, argv, spec.Environment); err != nil {
		_ = writeFrame(control, launcherCheckpoint{Error: "managed same-fd target exec failed"})
		return 1
	}
	return 1
}

func deathGuardMode(control, self, cgroupFile *os.File, _ int, failureStatus int) int {
	defer control.Close()
	defer self.Close()
	defer cgroupFile.Close()
	groupBytes, err := readFrame(control, 64)
	var group int
	if err != nil || decodeStrict(groupBytes, &group) != nil || group < 0 {
		return 1
	}
	if err := writeFrame(control, guardArm{Armed: true}); err != nil {
		return 1
	}
	disarmBytes, disarmErr := readFrame(control, 64)
	var disarm guardDisarm
	if disarmErr == nil && decodeStrict(disarmBytes, &disarm) == nil && disarm.Disarm {
		if failureStatus != 0 {
			return failureStatus
		}
		return 0
	}
	if group > 0 {
		_ = syscall.Kill(-group, syscall.SIGKILL)
	}
	killFD, err := unix.Openat(int(cgroupFile.Fd()), "cgroup.kill", unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return 11
	}
	_, writeErr := syscall.Write(killFD, []byte("1"))
	_ = syscall.Close(killFD)
	if writeErr != nil {
		return 12
	}
	if err := waitCgroupDescriptorEmpty(cgroupFile, 2*time.Second); err != nil {
		return 13
	}
	if failureStatus != 0 {
		return failureStatus
	}
	return 0
}

func waitCgroupDescriptorEmpty(cgroupFile *os.File, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		currentFD, err := unix.Openat(int(cgroupFile.Fd()), "pids.current", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		var content [32]byte
		n, readErr := syscall.Read(currentFD, content[:])
		_ = syscall.Close(currentFD)
		if readErr != nil {
			return readErr
		}
		if strings.TrimSpace(string(content[:n])) == "0" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("managed cgroup remained populated after kill")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ChildMode implements deterministic target payloads only. Internal lifecycle
// stages are unreachable through argv.

func runSelfAttackParent(requestPath string) int {
	payload, err := os.ReadFile(requestPath)
	if err != nil || len(payload) > 4096 {
		return 1
	}
	var request selfAttackRequest
	if decodeStrict(payload, &request) != nil || !isCanonicalAbsolute(request.Helper) || !isCanonicalAbsolute(request.Root) {
		return 1
	}
	old := request.Helper + ".opened"
	replaced := false
	first, firstErr := Supervise(context.Background(), request.Helper, Spec{Name: "internal-self-identity-replace", Executable: request.Helper, Args: []string{"__proof-child", "exit", "0"}, Dir: request.Root, Env: isolatedEnv(request.Root, ""), Stdout: io.Discard, Stderr: io.Discard, preSelfMutation: func(path string) error {
		if err := os.Rename(path, old); err != nil {
			return err
		}
		if err := copyExecutable(old, path); err != nil {
			_ = os.Rename(old, path)
			return err
		}
		replaced = true
		return nil
	}})
	_ = os.Remove(request.Helper)
	firstRestoreErr := os.Rename(old, request.Helper)
	exchangePath := request.Helper + ".exchange"
	exchanged := false
	copyErr := copyExecutable(request.Helper, exchangePath)
	second, secondErr := Observation{Status: 1}, error(nil)
	secondRestoreErr := error(nil)
	if copyErr == nil {
		second, secondErr = Supervise(context.Background(), request.Helper, Spec{Name: "internal-self-identity-exchange", Executable: request.Helper, Args: []string{"__proof-child", "exit", "0"}, Dir: request.Root, Env: isolatedEnv(request.Root, ""), Stdout: io.Discard, Stderr: io.Discard, preSelfMutation: func(path string) error {
			if err := unix.Renameat2(unix.AT_FDCWD, path, unix.AT_FDCWD, exchangePath, unix.RENAME_EXCHANGE); err != nil {
				return err
			}
			exchanged = true
			return nil
		}})
		if exchanged {
			secondRestoreErr = unix.Renameat2(unix.AT_FDCWD, request.Helper, unix.AT_FDCWD, exchangePath, unix.RENAME_EXCHANGE)
		}
	}
	_ = os.Remove(exchangePath)
	contentRejected := false
	third, thirdErr := Supervise(context.Background(), request.Helper, Spec{Name: "internal-self-content-substitution", Executable: request.Helper, Args: []string{"__proof-child", "exit", "0"}, Dir: request.Root, Env: isolatedEnv(request.Root, ""), Stdout: io.Discard, Stderr: io.Discard, preSelfMutation: func(path string) error {
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			contentRejected = errors.Is(err, syscall.ETXTBSY)
			return err
		}
		defer file.Close()
		var value [1]byte
		if _, err := file.ReadAt(value[:], 0); err != nil {
			return err
		}
		value[0] ^= 0xff
		_, err = file.WriteAt(value[:], 0)
		return err
	}})
	contentRejected = contentRejected && thirdErr != nil && third.Status == 1
	passed := firstErr == nil && firstRestoreErr == nil && replaced && first.Status == 0 && completeLifecycle(first) && copyErr == nil && secondErr == nil && secondRestoreErr == nil && exchanged && second.Status == 0 && completeLifecycle(second) && contentRejected
	result := selfAttackResult{Status: second.Status, Replaced: replaced, Exchanged: exchanged, ContentRejected: contentRejected, Passed: passed}
	encoded, err := json.Marshal(result)
	if err != nil || os.WriteFile(request.Report, encoded, 0600) != nil {
		return 1
	}
	if !result.Passed {
		return 1
	}
	return 0
}
func runReservedFDParent(requestPath string) int {
	payload, err := os.ReadFile(requestPath)
	var request reservedFDRequest
	if err != nil || decodeStrict(payload, &request) != nil {
		return 1
	}
	statesExact := true
	for fd := 3; fd <= 5; fd++ {
		flags, flagErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if flagErr != nil {
			statesExact = false
			continue
		}
		if request.CLOEXEC {
			flags |= unix.FD_CLOEXEC
		} else {
			flags &^= unix.FD_CLOEXEC
		}
		if _, flagErr = unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags); flagErr != nil {
			statesExact = false
			continue
		}
		observed, observedErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		statesExact = statesExact && observedErr == nil && (observed&unix.FD_CLOEXEC != 0) == request.CLOEXEC
	}
	observation, superviseErr := Supervise(context.Background(), request.Executable, Spec{
		Name:       "reserved-fd-reuse",
		Executable: request.Executable,
		Args:       []string{"__proof-child", "inspect", request.Inspect},
		Dir:        request.Root,
		Env:        isolatedEnv(request.Root, request.CACert),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	})
	inspected, inspectErr := readProcessObservation(request.Inspect)
	descriptorsExact := inspectErr == nil && descriptorObservationExact(inspected)
	lifecycleExact := superviseErr == nil && observation.Status == 0 && completeLifecycle(observation)
	result := reservedFDResult{
		Status:           observation.Status,
		DescriptorStates: statesExact,
		DescriptorsExact: descriptorsExact,
		LifecycleExact:   lifecycleExact,
		Passed:           statesExact && descriptorsExact && lifecycleExact,
	}
	encoded, encodeErr := json.Marshal(result)
	if encodeErr != nil || os.WriteFile(request.Report, encoded, 0600) != nil || !result.Passed {
		return 1
	}
	return 0
}

func ChildMode(args []string) int {
	if len(args) != 2 {
		return 1
	}
	switch args[0] {
	case "self-attack":
		return runSelfAttackParent(args[1])
	case "reserved-fd":
		return runReservedFDParent(args[1])
	case "chown1":
		if os.Geteuid() != 0 || os.Getegid() != 0 || os.Chown(args[1], 1, 1) != nil {
			return 1
		}
		return 0
	case "io-uring-denied":
		var params [256]byte
		// x/sys has no io_uring_setup wrapper; this zeroed buffer exceeds the 120-byte Linux io_uring_params ABI (VAL-019).
		_, _, errno := unix.RawSyscall(unix.SYS_IO_URING_SETUP, 1, uintptr(unsafe.Pointer(&params[0])), 0) // #nosec G103 -- live writable ABI buffer; this child requires EPERM before any ring is created.
		if os.WriteFile(args[1], []byte(strconv.Itoa(int(errno))), 0600) != nil || errno != syscall.EPERM {
			return 1
		}
		return 0
	case "inspect":
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			return 1
		}
		fds := make([]int, 0, len(entries))
		var runtimeDescriptors, unexpectedDescriptors []int
		var unexpectedDescriptorClasses []string
		for _, entry := range entries {
			fd, convErr := strconv.Atoi(entry.Name())
			if convErr != nil {
				continue
			}
			target, linkErr := os.Readlink("/proc/self/fd/" + entry.Name())
			if linkErr != nil {
				continue
			}
			fds = append(fds, fd)
			if fd <= 2 {
				continue
			}
			flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(syscall.F_GETFD), 0)
			runtimeOwned := strings.HasPrefix(target, "anon_inode:[eventpoll]") || strings.HasPrefix(target, "anon_inode:[eventfd]")
			if errno == 0 && flags&syscall.FD_CLOEXEC != 0 && runtimeOwned {
				runtimeDescriptors = append(runtimeDescriptors, fd)
			} else {
				unexpectedDescriptors = append(unexpectedDescriptors, fd)
				unexpectedDescriptorClasses = append(unexpectedDescriptorClasses, descriptorTargetClass(target))
			}
		}
		sort.Ints(fds)
		sort.Ints(runtimeDescriptors)
		sort.Ints(unexpectedDescriptors)
		sort.Strings(unexpectedDescriptorClasses)
		b, err := json.Marshal(processObservation{Environment: os.Environ(), Descriptors: fds, PostExecRuntimeDescriptors: runtimeDescriptors, UnexpectedDescriptors: unexpectedDescriptors, UnexpectedDescriptorClasses: unexpectedDescriptorClasses})
		if err != nil || os.WriteFile(args[1], b, 0600) != nil {
			return 1
		}
		time.Sleep(100 * time.Millisecond)
		return 0
	case "exit":
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 0 || n > 255 {
			return 1
		}
		return n
	case "signal":
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			return 1
		}
		signal.Reset(syscall.Signal(n))
		_ = syscall.Kill(os.Getpid(), syscall.Signal(n))
		time.Sleep(time.Second)
		return 1
	case "sleep":
		d, err := time.ParseDuration(args[1])
		if err != nil {
			return 1
		}
		time.Sleep(d)
		return 0
	case "ignore":
		signal.Ignore(catchableSignals()...)
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
			return 1
		}
		select {}
	case "spawn", "spawn-ignore":
		mode := "cooperate"
		if args[0] == "spawn-ignore" {
			mode = "ignore"
		}
		cmd := exec.Command(os.Args[0], "__proof-child", mode, args[1])
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			return 1
		}
		return waitStatus(cmd.Wait())
	case "cooperate":
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
			return 1
		}
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, catchableSignals()...)
		defer signal.Stop(sigs)
		<-sigs
		return 0
	default:
		return 1
	}
}
