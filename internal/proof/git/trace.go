package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceurl"
)

var errTraceViolation = errors.New("closed trace policy violation")

type traceObservation struct {
	execs, endpoints, writes []string
	network                  []endpointFact
	files                    int
	forbiddenAccesses        int
	forbiddenAccessClasses   map[string]int
	resolverAccesses         int
	configurationAccesses    int
	hookFilterCheckouts      int
	unexpectedReads          int
	unexpectedReadClasses    map[string]int
	unknownEvents            int
	unknownEventClasses      map[string]int
	reviewedEventClasses     map[string]int
	markerSpills             int
	complete                 bool
}

type cappedBuffer struct {
	mu       sync.Mutex
	b        bytes.Buffer
	n, max   int64
	exceeded bool
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.n += int64(len(p))
	if w.n > w.max {
		w.exceeded = true
		return 0, errors.New("process output ceiling exceeded")
	}
	return w.b.Write(p)
}
func (w *cappedBuffer) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.b.Bytes()...)
}

type runner struct {
	git, tracer, supervisor, cwd, traceDir, owned string
	forbiddenMarker                               []byte
	env                                           []string
	wallTime                                      time.Duration
	allowLoopback                                 bool
	allowedLoopbackPort                           int
	allowedPublic                                 map[netip.Addr]bool
	delegatedUID                                  int
	allowedPublicPort                             int
	invocations                                   *[]Invocation
	access                                        *AccessObservation
	writes                                        *WriteObservation
	resources                                     *ResourceObservation
}

func (r *runner) run(ctx context.Context, label string, args ...string) ([]byte, error) {
	return r.runInput(ctx, label, nil, args...)
}

func (r *runner) runInput(ctx context.Context, label string, stdin []byte, args ...string) ([]byte, error) {
	prefix := filepath.Join(r.traceDir, label)
	traceArgs := []string{"-ff", "-qq", "-v", "-yy", "-s", "4096", "-e", "trace=all", "-o", prefix, r.git}
	traceArgs = append(traceArgs, args...)
	wallTime := r.wallTime
	if wallTime <= 0 {
		wallTime = MaxWallTime
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, wallTime)
	defer cancel()
	out, errOut := &cappedBuffer{max: MaxProcessOutput}, &cappedBuffer{max: MaxProcessOutput}
	cgroup, err := lifecycle.NewCgroupForDelegatedUID(r.delegatedUID)
	if err != nil {
		return nil, fmt.Errorf("resource boundary unavailable for %s: %w", label, err)
	}
	started := time.Now()
	lifecycleObs, waitErr := lifecycle.Supervise(deadlineCtx, r.supervisor, lifecycle.Spec{
		Name:          label,
		Executable:    r.tracer,
		Args:          traceArgs,
		Dir:           r.cwd,
		Env:           append([]string(nil), r.env...),
		Stdout:        out,
		Stderr:        errOut,
		Stdin:         bytes.NewReader(stdin),
		CgroupPath:    cgroup.Path(),
		DenyIOUring:   true,
		FileSizeLimit: uint64(MaxDiskBytes),
	})
	duration := time.Since(started)
	cgroupObs, cgroupErr := cgroup.Observe()
	closeErr := cgroup.Close()
	if cgroupErr == nil {
		r.resources.CgroupInvocations++
		r.resources.CgroupComplete = r.resources.CgroupComplete && cgroupObs.Empty && cgroupObs.MemoryOOM == 0 && cgroupObs.OOMKill == 0
		if cgroupObs.MemoryPeak > r.resources.MemoryPeakBytes {
			r.resources.MemoryPeakBytes = cgroupObs.MemoryPeak
		}
		if cgroupObs.ProcessPeak > r.resources.ProcessPeak {
			r.resources.ProcessPeak = cgroupObs.ProcessPeak
		}
		r.resources.CPUUsec += cgroupObs.CPUUsec
		r.resources.MemoryOOM += cgroupObs.MemoryOOM
		r.resources.OOMKill += cgroupObs.OOMKill
	} else {
		r.resources.CgroupComplete = false
	}
	if closeErr != nil {
		r.resources.CgroupComplete = false
	}
	if waitErr == nil && cgroupErr != nil {
		waitErr = cgroupErr
	}
	if waitErr == nil && closeErr != nil {
		waitErr = closeErr
	}
	if waitErr == nil && lifecycleObs.Status != 0 {
		waitErr = fmt.Errorf("exit status %d", lifecycleObs.Status)
	}
	r.access.SupervisorInvocations++
	r.access.DescendantsReaped += lifecycleObs.Reaped
	r.access.Orphans += lifecycleObs.Orphans
	r.access.Survivors += lifecycleObs.SurvivorsDetected
	r.access.DeathGuardArmed = r.access.DeathGuardArmed && lifecycleObs.GuardArmed
	r.access.IOUringDenied = r.access.IOUringDenied && lifecycleObs.IOUringDenied
	r.access.CgroupKillGuard = r.access.CgroupKillGuard && lifecycleObs.CgroupKillGuard
	r.access.LifecycleComplete = r.access.LifecycleComplete &&
		lifecycleObs.Ready && lifecycleObs.Session && lifecycleObs.ProcessGroup &&
		lifecycleObs.Subreaper && lifecycleObs.ParentChecked && lifecycleObs.ParentDeathSignal &&
		lifecycleObs.GuardArmed && lifecycleObs.CgroupKillGuard && lifecycleObs.LeaderObserved && lifecycleObs.IOUringDenied &&
		lifecycleObs.Orphans == 0 && lifecycleObs.SurvivorsDetected == 0 &&
		lifecycleObs.MemoryOOM == 0 && lifecycleObs.OOMKill == 0
	obs, parseErr := parseTrace(prefix, r.owned, r.git, r.forbiddenMarker)
	if parseErr != nil {
		return nil, fmt.Errorf("%w: instrumentation unavailable for %s: %v", errTraceViolation, label, parseErr)
	}
	for _, endpoint := range obs.network {
		switch {
		case endpoint.ip.IsValid() && endpoint.ip.IsLoopback() && r.allowLoopback && endpoint.port == r.allowedLoopbackPort:
		case sourceurl.IsPublicAddr(endpoint.ip) && (len(r.allowedPublic) == 0 || r.allowedPublic[endpoint.ip.Unmap()] && endpoint.port == r.allowedPublicPort):
		default:
			return nil, fmt.Errorf("%w: unadmitted or non-public endpoint for %s", errTraceViolation, label)
		}
	}
	if !obs.complete || obs.unknownEvents != 0 {
		return nil, fmt.Errorf("%w: unclassified event for %s (%s)", errTraceViolation, label, countClassSummary(obs.unknownEventClasses))
	}
	if obs.forbiddenAccesses != 0 {
		return nil, fmt.Errorf("%w: forbidden escape or ambient access for %s (%s)", errTraceViolation, label, countClassSummary(obs.forbiddenAccessClasses))
	}
	if obs.unexpectedReads != 0 {
		return nil, fmt.Errorf("%w: unadmitted read for %s (%s)", errTraceViolation, label, countClassSummary(obs.unexpectedReadClasses))
	}
	for _, write := range obs.writes {
		if write == "outside-path" || write == "unresolved-relative-write" {
			return nil, fmt.Errorf("%w: unadmitted write for %s", errTraceViolation, label)
		}
	}
	gitObserved := false
	for _, executable := range obs.execs {
		if executable == r.git {
			gitObserved = true
		}
	}
	if !gitObserved {
		return nil, fmt.Errorf("%w: exact Git exec unobserved for %s", errTraceViolation, label)
	}
	if !obs.complete {
		r.access.TraceComplete = false
	}
	r.access.TraceComplete = r.access.TraceComplete && obs.complete
	r.access.TraceFiles += obs.files
	r.access.ResolverAccesses += obs.resolverAccesses
	r.access.ConfigurationAccesses += obs.configurationAccesses
	r.access.HookFilterCheckouts += obs.hookFilterCheckouts
	r.access.UnexpectedReads += obs.unexpectedReads
	r.access.UnknownEvents += obs.unknownEvents
	r.access.MarkerSpills += obs.markerSpills
	for class, count := range obs.unknownEventClasses {
		r.access.UnknownEventClasses = append(r.access.UnknownEventClasses, class+"="+strconv.Itoa(count))
	}
	for class, count := range obs.unexpectedReadClasses {
		r.access.UnexpectedReadClasses = append(r.access.UnexpectedReadClasses, label+"/"+class+"="+strconv.Itoa(count))
	}
	sort.Strings(r.access.UnexpectedReadClasses)
	sort.Strings(r.access.UnknownEventClasses)
	for _, endpoint := range obs.endpoints {
		r.access.Endpoints = append(r.access.Endpoints, endpoint+":"+label)
	}
	r.writes.ObservedPaths = append(r.writes.ObservedPaths, obs.writes...)
	r.access.AgentAccesses += obs.forbiddenAccesses
	for _, p := range obs.writes {
		if p == "outside-path" || p == "unresolved-relative-write" {
			r.writes.OutsideWrites = append(r.writes.OutsideWrites, p)
		}
	}
	exitCode := lifecycleObs.Status
	if waitErr != nil && exitCode == 0 {
		exitCode = 1
	}
	argv, redactErr := redactInvocationArgs(append([]string{r.git}, args...), r.owned)
	if redactErr != nil {
		return nil, redactErr
	}
	envNames := make([]string, 0, len(r.env))
	for _, e := range r.env {
		if i := strings.IndexByte(e, '='); i > 0 {
			envNames = append(envNames, e[:i])
		}
	}
	sort.Strings(envNames)
	cwdRef, refErr := randomTypedRef("proof-owned-cwd")
	if refErr != nil {
		return nil, refErr
	}
	*r.invocations = append(*r.invocations, Invocation{Executable: r.git, Argv: argv, EnvironmentNames: envNames, WorkingDirectoryRef: cwdRef, ExitCode: exitCode, OutputBytes: out.n + errOut.n, DurationMillis: duration.Milliseconds(), ObservedExecs: obs.execs, TraceComplete: obs.complete})
	if out.exceeded || errOut.exceeded {
		return nil, errors.New("Git process output ceiling exceeded")
	}
	if deadlineCtx.Err() != nil {
		return nil, fmt.Errorf("Git wall-time ceiling exceeded: %w", deadlineCtx.Err())
	}
	if waitErr != nil {
		return nil, fmt.Errorf("Git failed: %w: %s", waitErr, safeDiagnostic(errOut.Bytes()))
	}
	return out.Bytes(), nil
}

func parseTrace(prefix, owned, manager string, forbiddenMarker []byte) (traceObservation, error) {
	files, err := filepath.Glob(prefix + "*")
	if err != nil || len(files) == 0 {
		return traceObservation{}, errors.New("no trace output")
	}
	defer func() {
		for _, name := range files {
			_ = os.Remove(name)
		}
	}()
	o := traceObservation{
		files:                  len(files),
		complete:               true,
		forbiddenAccessClasses: make(map[string]int),
		unexpectedReadClasses:  make(map[string]int),
		unknownEventClasses:    make(map[string]int),
		reviewedEventClasses:   make(map[string]int),
	}
	markUnknown := func(class string) {
		o.complete = false
		o.unknownEvents++
		o.unknownEventClasses[class]++
	}
	seenExec, seenEndpoint, seenWrite := map[string]bool{}, map[string]bool{}, map[string]bool{}
	tracePIDs, childPIDs := map[int]bool{}, map[int]bool{}
	executableByPID := map[int]string{}
	cwdByPID := map[int]string{}
	for _, name := range files {
		pid, pidErr := strconv.Atoi(strings.TrimPrefix(name, prefix+"."))
		if pidErr != nil || pid <= 0 {
			markUnknown("trace-file-name")
		} else {
			tracePIDs[pid] = true
		}
		if cwdByPID[pid] == "" {
			cwdByPID[pid] = owned
		}
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			return traceObservation{}, readErr
		}
		if len(forbiddenMarker) != 0 && bytes.Contains(data, forbiddenMarker) {
			o.markerSpills++
		}
		if len(data) == 0 || data[len(data)-1] != '\n' {
			markUnknown("unterminated-trace-file")
		}
		pending := ""
		for _, raw := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "--- SIG") || strings.HasPrefix(line, "+++ exited with ") || strings.HasPrefix(line, "+++ killed by ") {
				continue
			}
			if strings.HasPrefix(line, "<... ") {
				resumed := strings.Index(line, " resumed>")
				if resumed < 0 || pending == "" {
					markUnknown("unmatched-resumed-record")
					continue
				}
				line = pending + line[resumed+len(" resumed>"):]
				pending = ""
			}
			if strings.Contains(line, "<unfinished ...>") {
				if pending != "" {
					markUnknown("nested-unfinished-record")
				}
				pending = strings.TrimSuffix(line, " <unfinished ...>")
				continue
			}
			if strings.Contains(line, "<unavailable>") || strings.Contains(line, "???") {
				markUnknown("truncated-record")
				continue
			}
			open := strings.IndexByte(line, '(')
			resultText, resultOK := syscallResult(line)
			if open <= 0 || !resultOK {
				markUnknown("malformed-record")
				continue
			}
			call := line[:open]
			o.reviewedEventClasses[gitSyscallClass(call)+"/"+call]++
			if traceSuccessful(resultText) {
				if class := forbiddenEscapeSyscall(call, line); class != "" {
					class += "/" + gitEscapeCallerFact(call, line, pid, executableByPID[pid], manager, owned)
					o.forbiddenAccesses++
					o.forbiddenAccessClasses[class]++
				}
			}
			var paths []tracedPath
			if call == "execve" || call == "execveat" || isFileTrace(call) || isWriteTrace(call, line) {
				var pathErr error
				paths, pathErr = quotedPaths(line)
				if pathErr != nil {
					markUnknown("malformed-quoted-field")
					continue
				}
			}
			switch call {
			case "getxattr", "lgetxattr", "listxattr", "llistxattr", "setxattr", "lsetxattr", "removexattr", "lremovexattr":
				if len(paths) > 1 {
					paths = paths[:1]
				}
			}
			if call == "clone" || call == "clone3" || call == "fork" || call == "vfork" {
				child, childErr := strconv.Atoi(strings.Fields(resultText)[0])
				if childErr != nil {
					markUnknown("malformed-clone-result")
				} else if child > 0 {
					childPIDs[child] = true
					cwdByPID[child] = cwdByPID[pid]
					executableByPID[child] = executableByPID[pid]
				}
			}
			if call == "execve" || call == "execveat" {
				executable := ""
				if call == "execveat" && strings.Contains(line, "AT_EMPTY_PATH") {
					openFD, closeFD := strings.IndexByte(line, '<'), strings.IndexByte(line, '>')
					if openFD >= 0 && closeFD > openFD {
						executable = strings.TrimSuffix(line[openFD+1:closeFD], " (deleted)")
					}
				} else if len(paths) != 0 {
					executable = paths[0].value
				}
				if !filepath.IsAbs(executable) {
					markUnknown("missing-exec-path")
					continue
				}
				executableByPID[pid] = executable
				if !seenExec[executable] {
					seenExec[executable] = true
					o.execs = append(o.execs, executable)
				}
			}
			if call == "connect" || (call == "sendto" || call == "sendmsg" || call == "sendmmsg") && strings.Contains(line, "sa_family=") {
				endpoint, endpointErr := parseConnectFact(line)
				if endpointErr != nil {
					markUnknown("unclassified-endpoint")
					continue
				}
				o.network = append(o.network, endpoint)
				if !seenEndpoint[endpoint.display] {
					seenEndpoint[endpoint.display] = true
					o.endpoints = append(o.endpoints, endpoint.display)
				}
				if endpoint.resolver {
					o.resolverAccesses++
				}
			}
			fileEvent := isFileTrace(call)
			if fileEvent {
				for _, traced := range paths {
					resolved, ok := resolveTracedPath(line, traced, cwdByPID[pid])
					if !ok {
						markUnknown("unresolved-read-path")
						continue
					}
					clean := filepath.Clean(resolved)
					base := filepath.Base(clean)
					ambientGit := filepath.Join(owned, ".git")
					ambientHomeConfig := filepath.Join(owned, "home", ".gitconfig")
					if clean == ambientHomeConfig || clean == filepath.Join(owned, ".gitattributes") || within(ambientGit, clean) {
						o.forbiddenAccesses++
						o.forbiddenAccessClasses["ambient-git-state"]++
					}
					switch {
					case hasPathComponent(clean, ".ssh") || base == ".git-credentials" || base == "known_hosts" || base == "SSH_AUTH_SOCK":
						o.forbiddenAccesses++
						o.forbiddenAccessClasses["authentication-path"]++
					case base == "resolv.conf" || base == "hosts" || base == "nsswitch.conf" || strings.HasPrefix(base, "libnss_"):
						o.resolverAccesses++
					case base == ".gitconfig" || clean == "/etc/gitconfig" || base == "config" && hasPathComponent(clean, ".git"):
						o.configurationAccesses++
					case hasPathComponent(clean, "hooks") || base == ".gitattributes" || base == "attributes" && hasPathComponent(clean, "info"):
						o.hookFilterCheckouts++
					}
					if filepath.IsAbs(clean) && !within(owned, clean) && !within(clean, owned) && !expectedSystemRead(clean) {
						o.unexpectedReads++
						o.unexpectedReadClasses[unexpectedReadClass(clean)]++
					}
				}
			}
			if call == "chdir" && traceSuccessful(resultText) {
				if len(paths) != 1 {
					markUnknown("chdir-path")
				} else if resolved, ok := resolveTracedPath(line, paths[0], cwdByPID[pid]); ok {
					cwdByPID[pid] = filepath.Clean(resolved)
				} else {
					markUnknown("chdir-resolution")
				}
			}
			if call == "fchdir" && traceSuccessful(resultText) {
				openFD, closeFD := strings.IndexByte(line, '<'), strings.IndexByte(line, '>')
				if openFD < 0 || closeFD <= openFD || !filepath.IsAbs(line[openFD+1:closeFD]) {
					markUnknown("fchdir-resolution")
				} else {
					cwdByPID[pid] = filepath.Clean(line[openFD+1 : closeFD])
				}
			}
			if isWriteTrace(call, line) {
				if len(paths) == 0 {
					markUnknown("missing-write-path")
					continue
				}
				for _, traced := range paths {
					p, resolved := resolveTracedPath(line, traced, cwdByPID[pid])
					display := "unresolved-relative-write"
					if resolved {
						p = filepath.Clean(p)
						display = "outside-path"
						if p == "/dev/null" {
							display = "control:/dev/null"
						} else if within(owned, p) {
							display = "owned-path"
						}
					}
					if !seenWrite[display] {
						seenWrite[display] = true
						o.writes = append(o.writes, display)
					}
				}
			}
		}
		if pending != "" {
			markUnknown("unfinished-record-at-eof")
		}
	}
	roots := 0
	for pid := range tracePIDs {
		if !childPIDs[pid] {
			roots++
		}
	}
	if roots != 1 {
		markUnknown("process-graph-root-count")
	}
	return o, nil
}
func syscallResult(line string) (string, bool) {
	equal := strings.LastIndexByte(line, '=')
	if equal < 0 || strings.LastIndexByte(line[:equal], ')') < 0 {
		return "", false
	}
	result := strings.TrimSpace(line[equal+1:])
	return result, result != ""
}

type endpointFact struct {
	display  string
	ip       netip.Addr
	port     int
	resolver bool
}

func parseConnectFact(line string) (endpointFact, error) {
	if strings.Contains(line, "sa_family=AF_UNIX") {
		if strings.Contains(line, `sun_path="/var/run/nscd/socket"`) || strings.Contains(line, `sun_path="/run/systemd/resolve/`) {
			return endpointFact{display: "resolver-unix-socket", resolver: true}, nil
		}
		return endpointFact{display: "unix-socket"}, nil
	}
	if strings.Contains(line, "sa_family=AF_UNSPEC") {
		return endpointFact{display: "unspecified-socket"}, nil
	}
	family := ""
	if strings.Contains(line, "sa_family=AF_INET6") {
		family = "tcp6"
	} else if strings.Contains(line, "sa_family=AF_INET") {
		family = "tcp4"
	} else {
		return endpointFact{}, errors.New("unclassified socket family")
	}
	portKey := "sin_port=htons("
	if family == "tcp6" {
		portKey = "sin6_port=htons("
	}
	pi := strings.Index(line, portKey)
	if pi < 0 {
		return endpointFact{}, errors.New("missing socket port")
	}
	portEnd := strings.IndexByte(line[pi+len(portKey):], ')')
	if portEnd < 0 {
		return endpointFact{}, errors.New("truncated socket port")
	}
	port, err := strconv.Atoi(line[pi+len(portKey) : pi+len(portKey)+portEnd])
	if err != nil || port < 0 || port > 65535 {
		return endpointFact{}, errors.New("invalid socket port")
	}
	addrKey := "inet_addr(\""
	if family == "tcp6" {
		addrKey = "inet_pton(AF_INET6, \""
	}
	ai := strings.Index(line, addrKey)
	if ai < 0 {
		return endpointFact{}, errors.New("missing socket address")
	}
	addrStart := ai + len(addrKey)
	addrEnd := strings.IndexByte(line[addrStart:], '"')
	if addrEnd < 0 {
		return endpointFact{}, errors.New("truncated socket address")
	}
	addr, err := netip.ParseAddr(line[addrStart : addrStart+addrEnd])
	if err != nil {
		return endpointFact{}, errors.New("invalid socket address")
	}
	addr = addr.Unmap()
	class := "public"
	if addr.IsLoopback() {
		class = "loopback"
	} else if !sourceurl.IsPublicAddr(addr) {
		class = "nonpublic"
	}
	return endpointFact{display: fmt.Sprintf("%s-%s:%d", class, family, port), ip: addr, port: port, resolver: port == 53}, nil
}

type tracedPath struct {
	value string
	quote int
}

func quotedPaths(line string) ([]tracedPath, error) {
	var paths []tracedPath
	for offset := 0; offset < len(line); {
		start := strings.IndexByte(line[offset:], '"')
		if start < 0 {
			break
		}
		start += offset
		i := start + 1
		escaped := false
		for ; i < len(line); i++ {
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			if line[i] == '"' {
				break
			}
		}
		if i == len(line) {
			return nil, errors.New("unterminated quoted field")
		}
		value, err := strconv.Unquote(line[start : i+1])
		if err != nil {
			return nil, err
		}
		paths = append(paths, tracedPath{value: value, quote: start})
		offset = i + 1
	}
	return paths, nil
}

func resolveTracedPath(line string, traced tracedPath, cwd string) (string, bool) {
	if filepath.IsAbs(traced.value) {
		return traced.value, true
	}
	prefix := line[:traced.quote]
	open := strings.LastIndexByte(prefix, '<')
	close := strings.LastIndexByte(prefix, '>')
	if open >= 0 && close > open {
		base := strings.TrimSuffix(prefix[open+1:close], " (deleted)")
		if filepath.IsAbs(base) {
			return filepath.Join(base, traced.value), true
		}
	}
	dirfdPrefix := strings.TrimSuffix(strings.TrimSpace(prefix), ",")
	separator := strings.LastIndexByte(dirfdPrefix, ',')
	if openParen := strings.LastIndexByte(dirfdPrefix, '('); openParen > separator {
		separator = openParen
	}
	dirfd := strings.TrimSpace(dirfdPrefix[separator+1:])
	if strings.Contains(dirfd, "AT_FDCWD") || !strings.Contains(line[:strings.IndexByte(line, '(')+1], "at") {
		return filepath.Join(cwd, traced.value), true
	}
	return "", false
}

func traceSuccessful(result string) bool {
	return result != "" && result != "?" && !strings.HasPrefix(result, "-1 ")
}

func gitSyscallClass(call string) string {
	switch call {
	case "uname", "sysinfo", "getuid", "geteuid", "getgid", "getegid", "getgroups", "getresuid", "getresgid", "getcpu", "sched_getaffinity", "getrlimit", "prlimit64":
		return "host-observation"
	case "socket", "socketpair", "bind", "listen", "accept", "accept4", "connect", "getsockname", "getpeername", "sendto", "recvfrom", "sendmsg", "recvmsg", "shutdown", "setsockopt", "getsockopt", "recvmmsg", "sendmmsg":
		return "network-observation"
	}
	if isFileTrace(call) || isWriteTrace(call, "") {
		return "file-observation"
	}
	switch call {
	case "read", "readv", "pread64", "preadv", "preadv2", "write", "writev", "pwrite64", "pwritev", "pwritev2", "close", "close_range", "lseek", "fcntl", "flock", "fsync", "fdatasync", "sync", "syncfs", "sync_file_range", "fstat", "statfs", "fstatfs", "getdents", "getdents64", "copy_file_range", "sendfile", "splice", "tee", "vmsplice", "readahead", "fadvise64", "mmap", "mmap2", "mprotect", "munmap", "msync", "mincore", "mremap", "madvise", "memfd_create", "name_to_handle_at", "open_by_handle_at", "inotify_init", "inotify_init1", "inotify_add_watch", "inotify_rm_watch", "fanotify_init", "fanotify_mark", "fchdir":
		return "file-observation"
	case "execve", "execveat", "clone", "clone3", "fork", "vfork", "wait4", "waitid", "exit", "exit_group", "kill", "tkill", "tgkill", "pidfd_open", "pidfd_getfd", "pidfd_send_signal", "set_tid_address", "set_robust_list", "get_robust_list", "rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "rt_sigsuspend", "rt_sigpending", "rt_sigtimedwait", "sigaltstack", "getpid", "getppid", "gettid", "getpgid", "getpgrp", "getsid", "setpgid", "setsid", "prctl", "arch_prctl", "sched_yield", "sched_getaffinity", "sched_setaffinity", "sched_getattr", "sched_setattr", "setpriority", "getpriority", "setns", "unshare", "chroot", "pivot_root", "ptrace", "seccomp", "capget", "capset":
		return "process-observation"
	default:
		return "reviewed-non-boundary"
	}
}

func gitEscapeCallerFact(call, line string, tracePID int, executable, manager, owned string) string {
	caller := "pre-exec-trace-root"
	switch {
	case executable == manager:
		caller = "named-git"
	case executable != "" && within(owned, executable):
		caller = "proof-owned-descendant-" + filepath.Base(executable)
	case executable != "":
		caller = "external-descendant-" + filepath.Base(executable)
	}
	if call == "setsid" {
		return "caller-" + caller + "/no-arguments"
	}
	open, close := strings.IndexByte(line, '('), strings.IndexByte(line, ')')
	if open < 0 || close <= open {
		return "caller-" + caller + "/arguments-malformed"
	}
	fields := strings.Split(line[open+1:close], ",")
	if len(fields) != 2 {
		return "caller-" + caller + "/arguments-malformed"
	}
	pidArg, pidErr := strconv.Atoi(strings.TrimSpace(fields[0]))
	groupArg, groupErr := strconv.Atoi(strings.TrimSpace(fields[1]))
	if pidErr != nil || groupErr != nil {
		return "caller-" + caller + "/arguments-malformed"
	}
	pidClass := "other-process"
	switch pidArg {
	case 0:
		pidClass = "calling-process"
	case tracePID:
		pidClass = "calling-process-explicit"
	}
	groupClass := "other-group"
	switch groupArg {
	case 0:
		groupClass = "calling-process-group"
	case tracePID:
		groupClass = "calling-process-explicit"
	case pidArg:
		groupClass = "same-as-pid-argument"
	}
	return "caller-" + caller + "/pid-" + pidClass + "/pgid-" + groupClass
}

func forbiddenEscapeSyscall(call, line string) string {
	switch call {
	case "setsid", "setpgid":
		return "successful-session-or-group-escape"
	case "setns", "unshare":
		return "successful-namespace-escape"
	case "chroot", "pivot_root":
		return "successful-root-escape"
	case "mount", "umount", "umount2", "mount_setattr", "fsopen", "fsconfig", "fsmount", "move_mount", "open_tree":
		return "successful-mount-control"
	case "ptrace":
		return "successful-ptrace-control"
	case "bpf":
		return "successful-bpf-control"
	case "io_uring_setup", "io_uring_register", "io_uring_enter":
		return "successful-io-uring-control"
	case "clone", "clone3":
		if strings.Contains(line, "CLONE_INTO_CGROUP") {
			return "successful-cgroup-movement"
		}
		if strings.Contains(line, "CLONE_NEW") {
			return "successful-clone-namespace-escape"
		}
	case "write", "writev", "pwrite64", "pwritev", "pwritev2":
		if strings.Contains(line, "cgroup.procs") || strings.Contains(line, "cgroup.threads") {
			return "successful-cgroup-movement"
		}
	}
	return ""
}

func isFileTrace(call string) bool {
	switch call {
	case "open", "openat", "openat2", "creat", "access", "faccessat", "faccessat2", "stat", "lstat", "newfstatat", "statx", "readlink", "readlinkat", "statfs", "getxattr", "lgetxattr", "listxattr", "llistxattr", "setxattr", "lsetxattr", "removexattr", "lremovexattr", "chdir", "chroot", "acct", "name_to_handle_at", "inotify_add_watch":
		return true
	default:
		return false
	}
}

func isWriteTrace(call, line string) bool {
	switch call {
	case "mkdir", "mkdirat", "rmdir", "rename", "renameat", "renameat2", "unlink", "unlinkat", "symlink", "symlinkat", "link", "linkat", "creat", "chmod", "fchmod", "fchmodat", "fchmodat2", "chown", "fchown", "lchown", "fchownat", "truncate", "ftruncate", "utime", "utimes", "utimensat", "mknod", "mknodat", "setxattr", "lsetxattr", "fsetxattr", "removexattr", "lremovexattr", "fremovexattr":
		return true
	case "open", "openat", "openat2":
		return strings.Contains(line, "O_WRONLY") || strings.Contains(line, "O_RDWR") || strings.Contains(line, "O_CREAT") || strings.Contains(line, "O_TRUNC")
	default:
		return false
	}
}

func hasPathComponent(p, wanted string) bool {
	for _, component := range strings.Split(filepath.ToSlash(filepath.Clean(p)), "/") {
		if component == wanted {
			return true
		}
	}
	return false
}

func expectedSystemRead(p string) bool {
	p = filepath.Clean(p)
	if within("/nix/store", p) || within("/dev/null", p) {
		return true
	}
	switch p {
	case "/dev/null",
		"/dev/urandom",
		"/etc/ld.so.cache",
		"/etc/ld-nix.so.preload",
		"/etc/localtime",
		"/sys/devices/system/cpu/online":
		return true
	default:
		return false
	}
}

func unexpectedReadClass(p string) string {
	clean := filepath.Clean(p)
	for _, root := range []string{"/proc", "/sys"} {
		if within(root, clean) {
			relative := strings.TrimPrefix(filepath.ToSlash(clean), root+"/")
			return strings.TrimPrefix(root, "/") + "-" + strings.ReplaceAll(relative, "/", "_")
		}
	}
	base := filepath.Base(clean)
	switch {
	case within("/home", clean):
		return "home-" + base
	case within("/etc", clean):
		return "etc-" + base
	case within("/run", clean):
		return "run-" + base
	case within("/var/tmp", clean) || within("/tmp", clean):
		return "temp-" + base
	case within("/var", clean):
		return "var-" + base
	default:
		return "other-absolute-" + strings.ReplaceAll(strings.TrimPrefix(filepath.ToSlash(clean), "/"), "/", "_")
	}
}

func countClassSummary(counts map[string]int) string {
	classes := make([]string, 0, len(counts))
	for class, count := range counts {
		classes = append(classes, class+"="+strconv.Itoa(count))
	}
	sort.Strings(classes)
	return strings.Join(classes, ",")
}

func redactInvocationArgs(argv []string, owned string) ([]string, error) {
	redacted := make([]string, len(argv))
	for i, arg := range argv {
		redacted[i] = arg
		if strings.Contains(arg, owned) {
			ref, err := randomTypedRef("proof-owned-argv-path")
			if err != nil {
				return nil, err
			}
			redacted[i] = ref
			continue
		}
		if strings.HasPrefix(arg, "http.curloptResolve=") {
			ref, err := randomTypedRef("public-address-pin")
			if err != nil {
				return nil, err
			}
			redacted[i] = ref
			continue
		}
		if strings.HasPrefix(arg, "http://127.0.0.1:") {
			ref, err := randomTypedRef("loopback-fixture-endpoint")
			if err != nil {
				return nil, err
			}
			redacted[i] = ref
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(arg, "^{commit}"), "^{tree}")
		if len(base) == 40 && isLowerHex(base) {
			ref, err := randomTypedRef("fixture-object-argument")
			if err != nil {
				return nil, err
			}
			redacted[i] = ref
		}
	}
	return redacted, nil
}

func safeDiagnostic(b []byte) string {
	const max = 512
	if len(b) > max {
		b = b[:max]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, string(b))
}
