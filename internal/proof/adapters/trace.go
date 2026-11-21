package adapters

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

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
)

type tracePolicy struct {
	owned                  string
	proofRoot              string
	writeRoot              string
	cwd                    string
	protected              []string
	forbidden              map[string]string
	syntheticDest          string
	authenticatedArchive   string
	forbiddenMarker        []byte
	forbidHostFacts        bool
	allowUnclassifiedReads bool
	allowMiseSystemReads   bool
	platformCgroupRoot     string
	managerExecutable      string
}

type adapterTrace struct {
	execs                     []string
	reads                     map[string]int
	writes                    map[string]int
	deniedWrites              map[string]int
	fileEvents                int
	processEvents             int
	socketEvents              int
	networkAttempts           int
	hostFactAccesses          int
	platformObservations      int
	platformClasses           map[string]int
	unknownEvents             int
	authenticatedArchiveReads int
	unknownClasses            map[string]int
	reviewedClasses           map[string]int
	forbiddenAccesses         map[string]int
	outsideWrites             int
	complete                  bool
}

type traceRecord struct {
	call       string
	line       string
	result     string
	paths      []tracePath
	writePaths []tracePath
}

type tracePath struct {
	value string
	quote int
}

func invokeTraced(ctx context.Context, cfg Config, name, executable string, args []string, dir string, env []string, policy tracePolicy) ([]byte, InvocationObservation, adapterTrace, error) {
	policy.managerExecutable = executable
	prefix := filepath.Join(policy.owned, ".adapter-trace-"+name)
	traceArgs := []string{"-f", "-qq", "-v", "-yy", "-s", "4096", "-e", "trace=all", "-o", prefix + ".log", executable}
	traceArgs = append(traceArgs, args...)
	var stdout, stderr bytes.Buffer
	cgroupPath := cfg.ExecutionCgroupPath
	var ownedCgroup *lifecycle.Cgroup
	if cgroupPath == "" {
		var cgroupErr error
		ownedCgroup, cgroupErr = lifecycle.NewCgroup()
		if cgroupErr != nil {
			return nil, InvocationObservation{Executable: executable, Arguments: redactArguments(args, policy.owned), Invoked: false}, adapterTrace{}, fmt.Errorf("mandatory adapter cgroup unavailable: %w", cgroupErr)
		}
		cgroupPath = ownedCgroup.Path()
	}
	policy.platformCgroupRoot = cgroupPath
	obs, superviseErr := lifecycle.Supervise(ctx, cfg.Executable, lifecycle.Spec{Name: name, Executable: cfg.Trace, Args: traceArgs, Dir: dir, Env: env, Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr, CgroupPath: cgroupPath, CgroupInherited: cfg.ExecutionCgroupPath != "", DenyIOUring: true})
	if ownedCgroup != nil {
		if closeErr := ownedCgroup.Close(); superviseErr == nil && closeErr != nil {
			superviseErr = closeErr
		}
	}
	trace, traceErr := parseAdapterTrace(prefix, policy)
	lifecycleComplete := obs.Ready && obs.Session && obs.ProcessGroup && obs.Subreaper && obs.ParentChecked && obs.ParentDeathSignal && obs.GuardArmed && obs.GuardCleanup && obs.CgroupKillGuard && obs.Orphans == 0 && obs.SurvivorsDetected == 0 && obs.CgroupObserved && (obs.CgroupEmpty || obs.CgroupCleanupDeferred) && obs.IOUringDenied && obs.MemoryOOM == 0 && obs.OOMKill == 0
	invocation := InvocationObservation{Executable: executable, Arguments: redactArguments(args, policy.owned), WorkingDirectory: typedRelation("proof-owned-working-directory"), Descriptors: []int{0, 1, 2}, Invoked: true, ExitStatus: obs.Status, TraceComplete: trace.complete, LifecycleComplete: lifecycleComplete, DeathGuardArmed: obs.GuardArmed, DeathGuardCleanup: obs.GuardCleanup, DescendantsReaped: obs.Reaped, Orphans: obs.Orphans, SurvivorsDetected: obs.SurvivorsDetected, FileEvents: trace.fileEvents, ProcessEvents: trace.processEvents, SocketEvents: trace.socketEvents, ExecutableClasses: executableClasses(trace.execs, executable, policy), CgroupObserved: obs.CgroupObserved, CgroupEmpty: obs.CgroupEmpty, CgroupCleanupDeferred: obs.CgroupCleanupDeferred, MemoryPeakBytes: obs.MemoryPeak, ProcessPeak: obs.ProcessPeak, CPUUsec: obs.CPUUsec, IOUringDenied: obs.IOUringDenied, MemoryOOM: obs.MemoryOOM, OOMKill: obs.OOMKill, CgroupKillGuard: obs.CgroupKillGuard}
	if len(policy.forbiddenMarker) != 0 && (bytes.Contains(stdout.Bytes(), policy.forbiddenMarker) || bytes.Contains(stderr.Bytes(), policy.forbiddenMarker)) {
		return nil, invocation, trace, errors.New("ambient marker reached a manager output channel")
	}
	if superviseErr != nil {
		detail := strings.TrimSpace(stderr.String())
		detail = strings.ReplaceAll(strings.ReplaceAll(detail, policy.owned, "<proof-owned>"), policy.proofRoot, "<proof-root>")
		if len(detail) > 1024 {
			detail = detail[:1024]
		}
		if detail == "" {
			return stdout.Bytes(), invocation, trace, superviseErr
		}
		return stdout.Bytes(), invocation, trace, fmt.Errorf("%w: %s", superviseErr, detail)
	}
	if traceErr != nil {
		return stdout.Bytes(), invocation, trace, fmt.Errorf("%s closed syscall trace: %w", name, traceErr)
	}
	if !lifecycleComplete {
		return stdout.Bytes(), invocation, trace, errors.New("managed invocation lifecycle boundary was incomplete")
	}
	if obs.Status != 0 {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = fmt.Sprintf("exit status %d", obs.Status)
		}
		return stdout.Bytes(), invocation, trace, errors.New(detail)
	}
	if !trace.complete || trace.unknownEvents != 0 {
		return stdout.Bytes(), invocation, trace, errors.New("trace contained unknown, truncated, or unresolved events")
	}
	managerExecs := 0
	for _, path := range trace.execs {
		if path == executable {
			managerExecs++
		}
	}
	if managerExecs != 1 {
		return stdout.Bytes(), invocation, trace, errors.New("trace did not contain exactly one named manager exec")
	}
	if trace.outsideWrites != 0 {
		return stdout.Bytes(), invocation, trace, errors.New("manager attempted a write outside its declared root")
	}
	for class, count := range trace.forbiddenAccesses {
		if count != 0 {
			return stdout.Bytes(), invocation, trace, fmt.Errorf("manager accessed forbidden %s class", class)
		}
	}
	if !policy.allowUnclassifiedReads && trace.reads["unexpected-absolute"] != 0 {
		return stdout.Bytes(), invocation, trace, errors.New("manager accessed an unclassified absolute path")
	}
	if trace.networkAttempts != 0 {
		return stdout.Bytes(), invocation, trace, errors.New("network activity occurred in a network-free manager invocation")
	}
	return stdout.Bytes(), invocation, trace, nil
}

func parseAdapterTrace(prefix string, policy tracePolicy) (adapterTrace, error) {
	files, err := filepath.Glob(prefix + ".*")
	if err != nil || len(files) == 0 {
		return adapterTrace{}, errors.New("no trace output")
	}
	defer func() {
		for _, path := range files {
			_ = os.Remove(path)
		}
	}()
	out := adapterTrace{reads: make(map[string]int), writes: make(map[string]int), deniedWrites: make(map[string]int), forbiddenAccesses: make(map[string]int), unknownClasses: make(map[string]int), reviewedClasses: make(map[string]int), platformClasses: make(map[string]int), complete: true}
	tracePIDs, childPIDs := make(map[int]bool), make(map[int]bool)
	executableByPID := make(map[int]string)
	cwdByPID := make(map[int]string)
	pending := make(map[int]string)
	seenExec := make(map[string]bool)
	if len(files) != 1 {
		out.markUnknown("trace-file-count")
	}
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return adapterTrace{}, readErr
		}
		if len(policy.forbiddenMarker) != 0 && bytes.Contains(data, policy.forbiddenMarker) {
			out.markUnknown("ambient-marker-trace-spill")
		}
		if len(data) > 0 && data[len(data)-1] != '\n' {
			out.markUnknown("unterminated-trace")
		}
		for _, raw := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			pid, line, lineErr := splitAdapterTraceLine(raw)
			if lineErr != nil {
				out.markUnknown("trace-pid-prefix")
				continue
			}
			tracePIDs[pid] = true
			if strings.HasPrefix(line, "<... ") {
				resumed := strings.Index(line, " resumed>")
				if resumed < 0 || pending[pid] == "" {
					out.markUnknown("unmatched-resumed-record")
					continue
				}
				line = pending[pid] + line[resumed+len(" resumed>"):]
				delete(pending, pid)
			}
			if strings.Contains(line, "<unfinished ...>") {
				if pending[pid] != "" {
					out.markUnknown("nested-unfinished-record-" + traceCallName(pending[pid]) + "-" + traceCallName(line))
				}
				pending[pid] = strings.TrimSuffix(line, " <unfinished ...>")
				continue
			}
			if line == "" || strings.HasPrefix(line, "--- SIG") || strings.HasPrefix(line, "+++ exited with ") || strings.HasPrefix(line, "+++ killed by ") {
				continue
			}
			record, recordErr := parseTraceRecord(line)
			if recordErr != nil {
				out.markUnknown("malformed-record")
				continue
			}
			out.reviewedClasses[adapterSyscallClass(record.call)+"/"+record.call]++
			if successful(record.result) {
				if escapeClass := forbiddenEscapeSyscall(record.call, record.line); escapeClass != "" {
					if record.call == "setpgid" {
						escapeClass += "/" + setpgidEscapeFact(record.line, pid, executableByPID[pid], policy)
					} else {
						escapeClass += "/caller-" + adapterEscapeCaller(executableByPID[pid], policy)
					}
					out.forbiddenAccesses[escapeClass]++
				}
			}
			if adapterHostFactSyscall(record.call) {
				if policy.forbidHostFacts && adapterChezmoiBootstrapSyscall(record.call) {
					out.platformObservations++
					out.platformClasses["syscall-chezmoi-bootstrap-"+record.call]++
				} else if policy.forbidHostFacts {
					out.hostFactAccesses++
					out.forbiddenAccesses["host-fact-syscall-"+record.call]++
				} else if policy.allowMiseSystemReads {
					if class, allowed := classifyMisePlatformSyscall(record.call); allowed {
						out.platformObservations++
						out.platformClasses[class]++
					} else {
						out.forbiddenAccesses["non-normative-platform-syscall-"+record.call]++
					}
				} else {
					out.platformObservations++
					out.platformClasses["syscall-"+record.call]++
				}
			}
			if isMountControlTrace(record.call) {
				out.forbiddenAccesses["manager-mount-control-attempt"]++
			}
			cwd := out.observeProcess(record, pid, policy, cwdByPID, executableByPID, childPIDs, seenExec)
			out.observeReads(record, pid, cwd, policy)
			out.observeWrites(record, cwd, policy)
			out.observeSocket(record, policy)

		}
	}
	for _, line := range pending {
		call := traceCallName(line)
		// A thread may exit while a side-effect-free wait remains blocked.
		if call == "nanosleep" || call == "epoll_wait" {
			out.reviewedClasses[adapterSyscallClass(call)+"/"+call]++
			continue
		}
		out.markUnknown("unfinished-record-at-eof-" + call)
	}
	roots := 0
	for pid := range tracePIDs {
		if !childPIDs[pid] {
			roots++
		}
	}
	if roots != 1 {
		out.markUnknown("trace-root-count")
	}
	sort.Strings(out.execs)
	if !out.complete {
		return out, errors.New("trace classification was not closed")
	}
	return out, nil
}

func (out *adapterTrace) observeProcess(record traceRecord, pid int, policy tracePolicy, cwdByPID, executableByPID map[int]string, childPIDs map[int]bool, seenExec map[string]bool) string {
	cwd := cwdByPID[pid]
	if cwd == "" {
		cwd = policy.cwd
		cwdByPID[pid] = cwd
	}
	if isProcessTrace(record.call) {
		out.processEvents++
	}
	if record.call == "clone" || record.call == "clone3" || record.call == "fork" || record.call == "vfork" {
		fields := strings.Fields(record.result)
		if len(fields) == 0 {
			out.markUnknown("clone-result")
		} else if child, childErr := strconv.Atoi(fields[0]); childErr == nil && child > 0 {
			childPIDs[child] = true
			cwdByPID[child] = cwd
			executableByPID[child] = executableByPID[pid]
		}
	}
	if record.call == "chdir" && successful(record.result) {
		if len(record.paths) != 1 {
			out.markUnknown("chdir-arity")
		} else if resolved, ok := resolveAdapterPath(record.line, record.paths[0], cwd); ok {
			cwd = filepath.Clean(resolved)
			cwdByPID[pid] = cwd
		} else {
			out.markUnknown("chdir-resolution")
		}
	}
	if record.call == "fchdir" && successful(record.result) {
		open, close := strings.IndexByte(record.line, '<'), strings.IndexByte(record.line, '>')
		if open < 0 || close <= open || !filepath.IsAbs(record.line[open+1:close]) {
			out.markUnknown("fchdir-resolution")
		} else {
			cwd = filepath.Clean(record.line[open+1 : close])
			cwdByPID[pid] = cwd
		}
	}
	if record.call == "execve" || record.call == "execveat" {
		resolved, ok := "", false
		if record.call == "execveat" && strings.Contains(record.line, "AT_EMPTY_PATH") {
			resolved, ok = annotatedAdapterFDPath(record.line)
		} else if len(record.paths) != 0 {
			resolved, ok = resolveAdapterPath(record.line, record.paths[0], cwd)
		}
		if !ok {
			out.markUnknown("exec-resolution")
		} else {
			clean := filepath.Clean(resolved)
			executableByPID[pid] = clean
			if !seenExec[clean] {
				seenExec[clean] = true
				out.execs = append(out.execs, clean)
			}
		}
	}
	return cwd
}

func (out *adapterTrace) observeReads(record traceRecord, pid int, cwd string, policy tracePolicy) {
	if isAdapterFileTrace(record.call) {
		out.fileEvents++
		for _, candidate := range record.paths {
			resolved, ok := resolveAdapterPath(record.line, candidate, cwd)
			if !ok {
				out.markUnknown("file-resolution")
				continue
			}
			clean := filepath.Clean(resolved)
			class := classifyRead(clean, policy)
			if !successful(record.result) && within(clean, "/nonexistent") {
				class = "denied-synthetic-probe"
			} else if class == "unexpected-absolute" && policy.allowMiseSystemReads {
				if miseClass, allowed := classifyMiseSystemRead(clean, record.result, pid, policy); allowed {
					class = miseClass
				}
			}
			if strings.HasPrefix(class, "mise-platform-") {
				out.platformObservations++
				out.platformClasses["path-"+class]++
			}
			out.reads[class]++
			if class == "unexpected-absolute" {
				outcome := "denied"
				if successful(record.result) {
					outcome = "success"
				}
				out.unknownClasses["unexpected-read-"+record.call+"-"+diagnosticAdapterPathClass(clean, policy)+"-"+outcome]++
			}
			hostFactPath := adapterHostFactPath(clean) || (within(clean, "/proc") || within(clean, "/sys")) && !adapterRuntimeBootstrapPath(clean, policy)
			if hostFactPath && policy.forbidHostFacts {
				out.hostFactAccesses++
			}
			if forbiddenClass := forbiddenPathClass(clean, policy); forbiddenClass != "" {
				out.forbiddenAccesses[forbiddenClass]++
			}
			if policy.authenticatedArchive != "" && successful(record.result) && (record.call == "open" || record.call == "openat" || record.call == "openat2") && !isAdapterWriteTrace(record.call, record.line) {
				if clean == policy.authenticatedArchive {
					out.authenticatedArchiveReads++
				} else if strings.HasSuffix(clean, ".tar.gz") || strings.HasSuffix(clean, ".tar.xz") || strings.HasSuffix(clean, ".zip") {
					out.forbiddenAccesses["unauthenticated-archive-input"]++
				}
			}
		}
	}
}

func (out *adapterTrace) observeWrites(record traceRecord, cwd string, policy tracePolicy) {
	if isAdapterWriteTrace(record.call, record.line) {
		writePaths := record.writePaths
		if len(writePaths) == 0 {
			if path, ok := annotatedAdapterFDPath(record.line); ok {
				writePaths = []tracePath{{value: path}}
			} else if successful(record.result) {
				out.markUnknown("write-no-path")
			} else {
				out.deniedWrites["unresolved-"+record.call]++
			}
		}
		for _, candidate := range writePaths {
			resolved, ok := resolveAdapterPath(record.line, candidate, cwd)
			if !ok {
				if successful(record.result) {
					out.markUnknown("write-resolution")
				} else {
					out.deniedWrites["unresolved-"+record.call]++
				}
				continue
			}
			clean := filepath.Clean(resolved)
			writeClass := "outside-" + outsidePathClass(clean, policy)
			if clean == "/dev/null" {
				writeClass = "control-device"
			} else if within(clean, policy.writeRoot) {
				writeClass = "declared-manager-root"
			}
			if !successful(record.result) {
				out.deniedWrites[writeClass+"-"+record.call]++
				continue
			}
			if !within(clean, policy.writeRoot) && clean != "/dev/null" {
				out.outsideWrites++
			}
			out.writes[writeClass]++
		}
	}
}

func (out *adapterTrace) observeSocket(record traceRecord, policy tracePolicy) {
	if isSocketTrace(record.call) {
		out.socketEvents++
		if record.call == "connect" || (record.call == "sendto" || record.call == "sendmsg" || record.call == "sendmmsg") && strings.Contains(record.line, "sa_family=") {
			endpoint, endpointErr := classifyEndpoint(record.line)
			if endpointErr != nil {
				out.markUnknown("endpoint")
			} else if endpoint != "unix" && endpoint != "unspecified" && endpoint != "netlink" {
				out.networkAttempts++
			}
			if endpoint == "unix" && policy.allowMiseSystemReads && successful(record.result) {
				socketPath, present, pathErr := traceUnixSocketPath(record.line)
				if pathErr != nil {
					out.markUnknown("unix-endpoint")
				} else if present && !within(socketPath, policy.proofRoot) {
					out.forbiddenAccesses["non-normative-platform-unix-socket"]++
				}
			}
		}
	}
}

func traceCallName(line string) string {
	end := strings.IndexByte(line, '(')
	if end <= 0 {
		return "unknown"
	}
	name := strings.TrimSpace(line[:end])
	for _, r := range name {
		if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return "unknown"
		}
	}
	return name
}

func splitAdapterTraceLine(raw string) (int, string, error) {
	line := strings.TrimSpace(raw)
	space := strings.IndexByte(line, ' ')
	if space <= 0 {
		return 0, "", errors.New("missing trace PID prefix")
	}
	pid, err := strconv.Atoi(line[:space])
	if err != nil || pid <= 0 {
		return 0, "", errors.New("invalid trace PID prefix")
	}
	return pid, strings.TrimSpace(line[space+1:]), nil
}
func (o *adapterTrace) markUnknown(class ...string) {
	o.complete = false
	o.unknownEvents++
	name := "unclassified"
	if len(class) == 1 {
		name = class[0]
	}
	o.unknownClasses[name]++
}

func parseTraceRecord(line string) (traceRecord, error) {
	open := strings.IndexByte(line, '(')
	equal := strings.LastIndexByte(line, '=')
	close := -1
	if equal >= 0 {
		close = strings.LastIndexByte(line[:equal], ')')
	}
	if open <= 0 || equal < 0 || close < open {
		return traceRecord{}, errors.New("malformed trace record")
	}
	call := line[:open]
	arguments, err := traceArgumentSpans(line, open+1, close)
	if err != nil {
		return traceRecord{}, err
	}
	pathIndexes, writeIndexes := tracePathArgumentIndexes(call)
	paths, err := selectedTracePaths(line, arguments, pathIndexes)
	if err != nil {
		return traceRecord{}, err
	}
	writePaths, err := selectedTracePaths(line, arguments, writeIndexes)
	if err != nil {
		return traceRecord{}, err
	}
	return traceRecord{call: call, line: line, result: strings.TrimSpace(line[equal+1:]), paths: paths, writePaths: writePaths}, nil
}

type traceArgument struct {
	start int
	end   int
}

func traceArgumentSpans(line string, start, end int) ([]traceArgument, error) {
	var arguments []traceArgument
	depth, argumentStart := 0, start
	quoted, escaped := false, false
	appendArgument := func(argumentEnd int) {
		for argumentStart < argumentEnd && line[argumentStart] == ' ' {
			argumentStart++
		}
		for argumentEnd > argumentStart && line[argumentEnd-1] == ' ' {
			argumentEnd--
		}
		arguments = append(arguments, traceArgument{start: argumentStart, end: argumentEnd})
	}
	for i := start; i < end; i++ {
		if quoted {
			if escaped {
				escaped = false
			} else if line[i] == '\\' {
				escaped = true
			} else if line[i] == '"' {
				quoted = false
			}
			continue
		}
		switch line[i] {
		case '"':
			quoted = true
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return nil, errors.New("unbalanced trace arguments")
			}
			depth--
		case ',':
			if depth == 0 {
				appendArgument(i)
				argumentStart = i + 1
			}
		}
	}
	if quoted || escaped || depth != 0 {
		return nil, errors.New("unterminated trace argument")
	}
	if argumentStart < end {
		appendArgument(end)
	}
	return arguments, nil
}

func selectedTracePaths(line string, arguments []traceArgument, indexes []int) ([]tracePath, error) {
	paths := make([]tracePath, 0, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len(arguments) {
			return nil, errors.New("missing trace path argument")
		}
		argument := arguments[index]
		quote := strings.IndexByte(line[argument.start:argument.end], '"')
		if quote < 0 {
			continue
		}
		quote += argument.start
		value, err := decodeTraceCString(line, quote, argument.end)
		if err != nil {
			return nil, err
		}
		paths = append(paths, tracePath{value: value, quote: quote})
	}
	return paths, nil
}

func decodeTraceCString(line string, quote, end int) (string, error) {
	value := make([]byte, 0, end-quote)
	for i := quote + 1; i < end; i++ {
		if line[i] == '"' {
			return string(value), nil
		}
		if line[i] != '\\' {
			value = append(value, line[i])
			continue
		}
		i++
		if i >= end {
			return "", errors.New("unterminated trace escape")
		}
		switch line[i] {
		case '\\', '"':
			value = append(value, line[i])
		case 'a':
			value = append(value, '\a')
		case 'b':
			value = append(value, '\b')
		case 'f':
			value = append(value, '\f')
		case 'n':
			value = append(value, '\n')
		case 'r':
			value = append(value, '\r')
		case 't':
			value = append(value, '\t')
		case 'v':
			value = append(value, '\v')
		case 'x':
			decoded, consumed, ok := traceEscapedByte(line, i+1, end, 16, 2)
			if !ok {
				return "", errors.New("invalid hexadecimal trace escape")
			}
			value = append(value, decoded)
			i += consumed
		default:
			if line[i] < '0' || line[i] > '7' {
				return "", errors.New("invalid trace escape")
			}
			decoded, consumed, ok := traceEscapedByte(line, i, end, 8, 3)
			if !ok {
				return "", errors.New("invalid octal trace escape")
			}
			value = append(value, decoded)
			i += consumed - 1
		}
	}
	return "", errors.New("unterminated quoted trace field")
}

func traceEscapedByte(line string, start, end, base, limit int) (byte, int, bool) {
	value, consumed := 0, 0
	for start+consumed < end && consumed < limit {
		digit := int(line[start+consumed] - '0')
		if line[start+consumed] >= 'a' && line[start+consumed] <= 'f' {
			digit = int(line[start+consumed]-'a') + 10
		} else if line[start+consumed] >= 'A' && line[start+consumed] <= 'F' {
			digit = int(line[start+consumed]-'A') + 10
		}
		if digit < 0 || digit >= base {
			break
		}
		value = value*base + digit
		consumed++
	}
	return byte(value), consumed, consumed > 0 && value <= 255
}

func tracePathArgumentIndexes(call string) (paths, writes []int) {
	switch call {
	case "execve", "open", "creat", "access", "stat", "lstat", "readlink", "chdir", "mkdir", "rmdir", "unlink", "chmod", "chown", "lchown", "truncate", "utime", "utimes", "mknod", "getcwd", "statfs", "getxattr", "lgetxattr", "listxattr", "llistxattr", "setxattr", "lsetxattr", "removexattr", "lremovexattr", "chroot", "acct":
		return []int{0}, []int{0}
	case "execveat", "openat", "openat2", "faccessat", "faccessat2", "newfstatat", "statx", "readlinkat", "mkdirat", "unlinkat", "fchmodat", "fchmodat2", "fchownat", "utimensat", "mknodat", "name_to_handle_at", "inotify_add_watch":
		return []int{1}, []int{1}
	case "rename":
		return []int{0, 1}, []int{0, 1}
	case "renameat", "renameat2":
		return []int{1, 3}, []int{1, 3}
	case "link":
		return []int{0, 1}, []int{1}
	case "linkat":
		return []int{1, 3}, []int{3}
	case "symlink":
		return []int{1}, []int{1}
	case "symlinkat":
		return []int{2}, []int{2}
	default:
		return nil, nil
	}
}
func annotatedAdapterFDPath(line string) (string, bool) {
	open := strings.IndexByte(line, '<')
	close := strings.IndexByte(line, '>')
	if open < 0 || close <= open {
		return "", false
	}
	path := strings.TrimSuffix(line[open+1:close], " (deleted)")
	return path, filepath.IsAbs(path)
}

func resolveAdapterPath(line string, traced tracePath, cwd string) (string, bool) {
	if filepath.IsAbs(traced.value) {
		return traced.value, true
	}
	prefix := line[:traced.quote]
	open, close := strings.LastIndexByte(prefix, '<'), strings.LastIndexByte(prefix, '>')
	if open >= 0 && close > open {
		base := strings.TrimSuffix(prefix[open+1:close], " (deleted)")
		if filepath.IsAbs(base) {
			return filepath.Join(base, traced.value), true
		}
	}
	return filepath.Join(cwd, traced.value), true
}

func classifyRead(path string, policy tracePolicy) string {
	switch {
	case within(path, policy.owned):
		return "proof-owned"
	case policy.proofRoot != "" && within(path, policy.proofRoot):
		return "proof-owned"
	case policy.proofRoot != "" && within(policy.proofRoot, path):
		return "proof-root-ancestor"
	case adapterRuntimeBootstrapClass(path, policy) != "":
		return adapterRuntimeBootstrapClass(path, policy)
	case expectedAdapterSystemRead(path):
		return "immutable-runtime-store-or-loader"
	default:
		for _, root := range policy.protected {
			if within(path, root) {
				return "protected-operator-root"
			}
		}
		return "unexpected-absolute"
	}
}

func outsidePathClass(path string, policy tracePolicy) string {
	if policy.proofRoot != "" && within(path, policy.proofRoot) {
		relative, err := filepath.Rel(policy.proofRoot, path)
		if err == nil {
			component := strings.Split(filepath.ToSlash(relative), "/")[0]
			if component != "" && component != "." && component != ".." {
				return "proof-root-" + component
			}
		}
		return "proof-root-other"
	}
	for _, candidate := range []struct {
		root  string
		class string
	}{
		{"/tmp", "tmp"},
		{"/var/tmp", "var-tmp"},
		{"/home", "home"},
		{"/run", "run"},
		{"/etc", "etc"},
		{"/dev", "dev"},
		{"/proc", "proc"},
		{"/sys", "sys"},
	} {
		if within(path, candidate.root) {
			return candidate.class
		}
	}
	if class := classifyRead(path, policy); class != "unexpected-absolute" {
		return class
	}
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/")
	if component, _, ok := strings.Cut(clean, "/"); ok && component != "" {
		return "top-" + component
	}
	return "unexpected-absolute"
}

func diagnosticAdapterPathClass(path string, policy tracePolicy) string {
	class := outsidePathClass(path, policy)
	switch class {
	case "proc", "sys":
		relative := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/"+class+"/")
		return class + "-" + strings.ReplaceAll(relative, "/", "_")
	case "dev", "etc", "run", "top-lib64", "top-usr", "top-var", "unexpected-absolute":
		base := filepath.Base(filepath.Clean(path))
		if base == "." || base == string(filepath.Separator) || base == "" {
			base = "root"
		}
		return class + "-" + base
	default:
		return class
	}
}

func forbiddenPathClass(path string, policy tracePolicy) string {
	if policy.forbidHostFacts && adapterHostFactPath(path) {
		return "host-fact-path"
	}
	if policy.forbidHostFacts && (within(path, "/proc") || within(path, "/sys")) && !adapterRuntimeBootstrapPath(path, policy) {
		return "unapproved-kernel-fact-path-" + diagnosticAdapterPathClass(path, policy)
	}
	for candidate, class := range policy.forbidden {
		if path == candidate || within(path, candidate) {
			return class
		}
	}
	if policy.syntheticDest != "" && within(path, policy.syntheticDest) {
		return "synthetic-destination"
	}
	for _, root := range policy.protected {
		if within(path, root) {
			return "real-home-xdg-or-destination"
		}
	}
	return ""
}

func adapterHostFactSyscall(call string) bool {
	switch call {
	case "uname", "sysinfo", "getuid", "geteuid", "getgid", "getegid", "getgroups", "getresuid", "getresgid", "getcpu", "sched_getaffinity", "getrlimit", "prlimit64":
		return true
	default:
		return false
	}
}
func adapterChezmoiBootstrapSyscall(call string) bool {
	return call == "prlimit64" || call == "sched_getaffinity"
}
func classifyMisePlatformSyscall(call string) (string, bool) {
	switch call {
	case "uname":
		return "syscall-uname", true
	case "getuid", "geteuid", "getgid", "getegid", "getgroups", "getresuid", "getresgid":
		return "syscall-current-identity", true
	case "sched_getaffinity":
		return "syscall-runtime-affinity", true
	case "getrlimit", "prlimit64":
		return "syscall-runtime-limits", true
	default:
		return "", false
	}
}

func isMountControlTrace(call string) bool {
	switch call {
	case "mount", "umount2", "mount_setattr", "fsopen", "fsconfig", "fsmount", "move_mount", "open_tree":
		return true
	default:
		return false
	}
}

func normativeMisePlatformClasses() []string {
	return []string{
		"path-mise-platform-abi",
		"path-mise-platform-cpu",
		"path-mise-platform-current-cgroup",
		"path-mise-platform-current-process",
		"path-mise-platform-memory",
		"path-mise-platform-os",
		"path-mise-platform-timezone",
		"syscall-current-identity",
		"syscall-runtime-affinity",
		"syscall-runtime-limits",
		"syscall-uname",
	}
}

func normativeMisePlatformClassCounts() map[string]int {
	counts := make(map[string]int)
	for _, class := range normativeMisePlatformClasses() {
		counts[class] = 0
	}
	return counts
}

func requireNormativeMisePlatformClasses(counts map[string]int) error {
	actual := make([]string, 0, len(counts))
	for class := range counts {
		actual = append(actual, class)
	}
	sort.Strings(actual)
	expected := normativeMisePlatformClasses()
	sort.Strings(expected)
	if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
		return fmt.Errorf("aggregate platform class keys are not the finite ARCH-013 set: got %s", strings.Join(actual, ","))
	}
	return nil
}
func successfulMisePlatformClasses(counts map[string]int) []string {
	classes := make([]string, 0, len(counts))
	for class, count := range counts {
		if count > 0 {
			classes = append(classes, class)
		}
	}
	sort.Strings(classes)
	return classes
}

func adapterHostFactPath(path string) bool {
	for _, exact := range []string{
		"/etc/group",
		"/etc/hostname",
		"/etc/gai.conf",
		"/etc/host.conf",
		"/etc/hosts",
		"/etc/kernel-release",
		"/etc/lsb-release",
		"/etc/machine-id",
		"/etc/gshadow",
		"/etc/login.defs",
		"/etc/resolv.conf",
		"/etc/nsswitch.conf",
		"/etc/os-release",
		"/etc/passwd",
		"/etc/shadow",
		"/etc/subgid",
		"/etc/subuid",
		"/proc/cpuinfo",
		"/proc/meminfo",
		"/proc/version",
		"/usr/lib/os-release",
		"/var/lib/dbus/machine-id",
	} {
		if path == exact {
			return true
		}
	}
	for _, root := range []string{"/proc/sys", "/sys/class", "/sys/devices", "/sys/firmware"} {
		if within(path, root) {
			return true
		}
	}
	return false
}

func adapterRuntimeBootstrapClass(path string, policy tracePolicy) string {
	if policy.allowMiseSystemReads {
		return ""
	}
	switch path {
	case "/proc/self/exe":
		return "runtime-bootstrap-current-executable"
	case "/proc/self/mountinfo":
		return "runtime-bootstrap-current-mounts"
	case "/sys/kernel/mm/transparent_hugepage/hpage_pmd_size":
		return "runtime-bootstrap-go-hugepage"
	}
	if path == "/proc/self/cgroup" {
		return "runtime-bootstrap-current-cgroup"
	}
	if cgroupHierarchyFile(path, policy.platformCgroupRoot, "cpu.max") {
		return "runtime-bootstrap-current-cgroup-cpu"
	}
	return ""
}

func adapterRuntimeBootstrapPath(path string, policy tracePolicy) bool {
	return adapterRuntimeBootstrapClass(path, policy) != ""
}
func cgroupHierarchyFile(path, leaf, name string) bool {
	if leaf == "" || !within(leaf, "/sys/fs/cgroup") {
		return false
	}
	for current := leaf; ; current = filepath.Dir(current) {
		if path == filepath.Join(current, name) {
			return true
		}
		if current == "/sys/fs/cgroup" {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current || !within(parent, "/sys/fs/cgroup") {
			return false
		}
	}
}

func expectedAdapterSystemRead(path string) bool {
	if within(path, "/nix/store") {
		return true
	}
	switch path {
	case "/dev/null", "/dev/urandom", "/etc/ld.so.cache", "/etc/ld-nix.so.preload":
		return true
	default:
		return false
	}
}

func misePlatformRuntimeRead(path string) bool {
	switch path {
	case "/lib",
		"/lib64",
		"/lib64/libc.so.6",
		"/lib64/libdl.so.2",
		"/lib64/libgcc_s.so.1",
		"/lib64/libm.so.6",
		"/lib64/libpthread.so.0",
		"/lib64/libstdc++.so.6":
		return true
	default:
		return false
	}
}
func classifyMiseSystemRead(path, result string, tracePID int, policy tracePolicy) (string, bool) {
	if !successful(result) {
		switch path {
		case "/nix", "/node_modules", "/package.json", "/proc/version_signature", "/tmp":
			return "denied-mise-discovery-probe", true
		}
		if within(path, "/sys/devices/system/cpu") && filepath.Base(path) == "tsc_freq_khz" {
			return "denied-mise-discovery-probe", true
		}
		for _, root := range []string{"/dev", "/etc", "/lib", "/lib64", "/opt", "/run", "/tmp", "/usr", "/var"} {
			if within(path, root) {
				return "denied-mise-system-probe", true
			}
		}
	}
	if misePlatformRuntimeRead(path) {
		return "mise-platform-abi", true
	}
	if within(path, "/usr/share/zoneinfo") {
		return "mise-platform-timezone", true
	}
	if path == "/proc/self/exe" || path == "/proc/self/maps" || path == "/proc/sys/vm/overcommit_memory" {
		return "mise-platform-current-process", true
	}
	if path == "/proc/meminfo" {
		return "mise-platform-memory", true
	}
	if path == "/etc/os-release" || path == "/usr/lib/os-release" {
		return "mise-platform-os", true
	}
	if path == "/proc/cpuinfo" || path == "/sys/devices/system/cpu/online" {
		return "mise-platform-cpu", true
	}
	if path == "/proc/self/cgroup" || path == fmt.Sprintf("/proc/%d/cgroup", tracePID) || cgroupHierarchyFile(path, policy.platformCgroupRoot, "cpu.max") {
		return "mise-platform-current-cgroup", true
	}
	if policy.platformCgroupRoot != "" {
		for _, name := range []string{"cgroup.controllers", "memory.high", "memory.max"} {
			if path == filepath.Join(policy.platformCgroupRoot, name) {
				return "mise-platform-current-cgroup", true
			}
		}
	}
	return "", false
}
func traceUnixSocketPath(line string) (string, bool, error) {
	const key = "sun_path="
	start := strings.Index(line, key)
	if start < 0 {
		return "", false, nil
	}
	quote := start + len(key)
	if quote >= len(line) || line[quote] != '"' {
		return "", false, errors.New("malformed unix socket path")
	}
	value, err := decodeTraceCString(line, quote, len(line))
	if err != nil {
		return "", false, err
	}
	return filepath.Clean(value), true, nil
}

func classifyEndpoint(line string) (string, error) {
	if strings.Contains(line, "sa_family=AF_UNIX") {
		return "unix", nil
	}
	if strings.Contains(line, "sa_family=AF_NETLINK") {
		return "netlink", nil
	}
	if strings.Contains(line, "sa_family=AF_UNSPEC") {
		return "unspecified", nil
	}
	family, portKey, addressKey := "", "", ""
	if strings.Contains(line, "sa_family=AF_INET6") {
		family, portKey, addressKey = "tcp6", "sin6_port=htons(", "inet_pton(AF_INET6, \""
	} else if strings.Contains(line, "sa_family=AF_INET") {
		family, portKey, addressKey = "tcp4", "sin_port=htons(", "inet_addr(\""
	} else {
		return "", errors.New("unknown socket family")
	}
	pi := strings.Index(line, portKey)
	if pi < 0 {
		return "", errors.New("missing port")
	}
	portEnd := strings.IndexByte(line[pi+len(portKey):], ')')
	if portEnd < 0 {
		return "", errors.New("truncated port")
	}
	port, err := strconv.Atoi(line[pi+len(portKey) : pi+len(portKey)+portEnd])
	if err != nil || port < 0 || port > 65535 {
		return "", errors.New("invalid port")
	}
	ai := strings.Index(line, addressKey)
	if ai < 0 {
		return "", errors.New("missing address")
	}
	start := ai + len(addressKey)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return "", errors.New("truncated address")
	}
	address, err := netip.ParseAddr(line[start : start+end])
	if err != nil {
		return "", errors.New("invalid address")
	}
	class := "public"
	if address.IsLoopback() {
		class = "loopback"
	} else if address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() {
		class = "nonpublic"
	}
	return fmt.Sprintf("%s-%s:%d", class, family, port), nil
}

func successful(result string) bool {
	return !strings.HasPrefix(result, "-1 ") && result != "?"
}

func adapterSyscallClass(call string) string {
	if adapterHostFactSyscall(call) {
		return "host-observation"
	}
	if isSocketTrace(call) {
		return "network-observation"
	}
	switch call {
	case "read", "readv", "pread64", "preadv", "preadv2", "write", "writev", "pwrite64", "pwritev", "pwritev2", "close", "close_range", "lseek", "fcntl", "flock", "fsync", "fdatasync", "sync", "syncfs", "sync_file_range", "fstat", "statfs", "fstatfs", "getdents", "getdents64", "copy_file_range", "sendfile", "splice", "tee", "vmsplice", "readahead", "fadvise64", "mmap", "mmap2", "mprotect", "munmap", "msync", "mincore", "mremap", "madvise", "memfd_create", "name_to_handle_at", "open_by_handle_at", "inotify_init", "inotify_init1", "inotify_add_watch", "inotify_rm_watch", "fanotify_init", "fanotify_mark":
		return "file-observation"
	}
	if isAdapterFileTrace(call) || isAdapterWriteTrace(call, "") {
		return "file-observation"
	}
	switch call {
	case "execve", "execveat", "clone", "clone3", "fork", "vfork", "wait4", "waitid", "exit", "exit_group", "kill", "tkill", "tgkill", "pidfd_open", "pidfd_getfd", "pidfd_send_signal", "set_tid_address", "set_robust_list", "get_robust_list", "rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "rt_sigsuspend", "rt_sigpending", "rt_sigtimedwait", "sigaltstack", "getpid", "getppid", "gettid", "getpgid", "getpgrp", "getsid", "setpgid", "setsid", "prctl", "arch_prctl", "sched_yield", "sched_getaffinity", "sched_setaffinity", "sched_getattr", "sched_setattr", "setpriority", "getpriority", "setns", "unshare", "chroot", "pivot_root", "ptrace", "seccomp", "capget", "capset":
		return "process-observation"
	}
	return "reviewed-non-boundary"
}

func adapterEscapeCaller(executable string, policy tracePolicy) string {
	switch {
	case executable == policy.managerExecutable:
		return "named-manager"
	case executable != "" && within(executable, policy.writeRoot):
		return "manager-owned-installed-helper"
	case executable != "" && expectedAdapterSystemRead(executable):
		return "pinned-runtime-helper"
	case executable != "" && within(executable, policy.proofRoot):
		return "proof-owned-descendant-" + filepath.Base(executable)
	case executable != "":
		return "external-descendant-" + filepath.Base(executable)
	default:
		return "pre-exec-trace-root"
	}
}

func setpgidEscapeFact(line string, tracePID int, executable string, policy tracePolicy) string {
	caller := adapterEscapeCaller(executable, policy)
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

func isProcessTrace(call string) bool {
	switch call {
	case "execve", "execveat", "clone", "clone3", "fork", "vfork", "wait4", "waitid", "exit", "exit_group", "kill", "tkill", "tgkill", "pidfd_open", "pidfd_getfd", "pidfd_send_signal", "set_tid_address", "set_robust_list", "get_robust_list", "rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "rt_sigsuspend", "rt_sigpending", "rt_sigtimedwait", "sigaltstack", "getpid", "getppid", "gettid", "getpgid", "getpgrp", "getsid", "setpgid", "setsid", "prctl", "arch_prctl", "sched_yield", "sched_getaffinity", "sched_setaffinity", "sched_getattr", "sched_setattr", "setpriority", "getpriority", "setns", "unshare", "chroot", "pivot_root", "ptrace", "seccomp", "capget", "capset":
		return true
	default:
		return false
	}
}

func isAdapterFileTrace(call string) bool {
	paths, _ := tracePathArgumentIndexes(call)
	return len(paths) != 0
}

func isAdapterWriteTrace(call, line string) bool {
	switch call {
	case "mkdir", "mkdirat", "rmdir", "rename", "renameat", "renameat2", "unlink", "unlinkat", "symlink", "symlinkat", "link", "linkat", "creat", "chmod", "fchmod", "fchmodat", "fchmodat2", "chown", "fchown", "lchown", "fchownat", "truncate", "ftruncate", "utime", "utimes", "utimensat", "mknod", "mknodat", "setxattr", "lsetxattr", "fsetxattr", "removexattr", "lremovexattr", "fremovexattr":
		return true
	case "open", "openat", "openat2":
		return strings.Contains(line, "O_WRONLY") || strings.Contains(line, "O_RDWR") || strings.Contains(line, "O_CREAT") || strings.Contains(line, "O_TRUNC")
	default:
		return false
	}
}

func isSocketTrace(call string) bool {
	switch call {
	case "socket", "socketpair", "bind", "listen", "accept", "accept4", "connect", "getsockname", "getpeername", "sendto", "recvfrom", "sendmsg", "recvmsg", "sendmmsg", "recvmmsg", "setsockopt", "getsockopt", "shutdown":
		return true
	default:
		return false
	}
}

func executableClasses(paths []string, manager string, policy tracePolicy) []string {
	counts := make(map[string]int)
	for _, path := range paths {
		class := "unknown-executable"
		switch {
		case path == manager:
			class = "named-manager"
		case within(path, policy.writeRoot):
			class = "manager-owned-installed-helper"
		case expectedAdapterSystemRead(path):
			class = "pinned-runtime-helper"
		}
		counts[class]++
	}
	classes := make([]string, 0, len(counts))
	for class, count := range counts {
		classes = append(classes, class+"="+strconv.Itoa(count))
	}
	sort.Strings(classes)
	return classes
}

func redactArguments(args []string, owned string) []string {
	redacted := append([]string(nil), args...)
	for i, arg := range redacted {
		if strings.Contains(arg, owned) {
			redacted[i] = typedRelation("proof-owned-argument")
		}
	}
	return redacted
}
