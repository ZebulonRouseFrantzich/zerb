package adapters

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceurl"
)

const (
	miseNamespaceMode = "ZERB_PROOF_MISE_NAMESPACE"
	miseRunnerMode    = "ZERB_PROOF_MISE_RUNNER"
	miseAttackMode    = "ZERB_PROOF_MISE_ARCHIVE_ATTACK"
	workerInputFD     = 3
	workerOutputFD    = 4
	maxInventoryItems = 200_000
	maxInventoryBytes = int64(1 << 30)
)

type miseNamespaceRequest struct {
	Config         Config   `json:"config"`
	MountPoint     string   `json:"mount_point"`
	ProtectedRoots []string `json:"protected_roots"`
	CgroupPath     string   `json:"cgroup_path"`
}

type miseRunnerRequest struct {
	Config             Config                      `json:"config"`
	Root               string                      `json:"root"`
	Directory          string                      `json:"directory"`
	ArchivePath        string                      `json:"archive_path"`
	Inventory          archiveInventory            `json:"inventory"`
	Injection          archiveInjectionObservation `json:"injection"`
	ProtectedRoots     []string                    `json:"protected_roots"`
	PlatformCgroupRoot string                      `json:"platform_cgroup_root"`
	CgroupPath         string                      `json:"cgroup_path"`
}

type archiveAttackRequest struct {
	ArchivePath     string `json:"archive_path"`
	ReplacementPath string `json:"replacement_path"`
}

type archiveInjectionObservation struct {
	UID             int  `json:"uid"`
	ModifyRejected  bool `json:"modify_rejected"`
	ReplaceRejected bool `json:"replace_rejected"`
	RemoveRejected  bool `json:"remove_rejected"`
}

type archiveInventory struct {
	Root    string                  `json:"root"`
	Entries map[string]archiveEntry `json:"entries"`
}

type archiveEntry struct {
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Target string `json:"target,omitempty"`
}

func init() {
	mode := ""
	for _, candidate := range []string{miseNamespaceMode, miseRunnerMode, miseAttackMode} {
		if os.Getenv(candidate) == "1" {
			mode = candidate
			break
		}
	}
	if mode == "" {
		return
	}
	_ = os.Unsetenv(mode)
	input := os.NewFile(workerInputFD, "adapter-worker-input")
	output := os.NewFile(workerOutputFD, "adapter-worker-output")
	if input == nil || output == nil {
		os.Exit(125)
	}
	var err error
	switch mode {
	case miseNamespaceMode:
		var request miseNamespaceRequest
		err = decodeWorkerRequest(input, &request)
		if err == nil {
			var scenario Scenario
			scenario, err = runMiseNamespaceHarness(context.Background(), request)
			if err == nil {
				err = json.NewEncoder(output).Encode(scenario)
			}
		}
	case miseRunnerMode:
		var request miseRunnerRequest
		err = decodeWorkerRequest(input, &request)
		if err == nil {
			request.Config.ProtectedRoots = request.ProtectedRoots
			request.Config.PlatformCgroupRoot = request.PlatformCgroupRoot
			request.Config.ExecutionCgroupPath = request.CgroupPath
			scenario := runMiseManagerScenario(context.Background(), request.Config, request.Root, request.Directory, request.ArchivePath, request.Inventory, request.Injection, nil)
			err = json.NewEncoder(output).Encode(scenario)
		}
	case miseAttackMode:
		var request archiveAttackRequest
		err = decodeWorkerRequest(input, &request)
		if err == nil {
			err = json.NewEncoder(output).Encode(runArchiveAttack(request))
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(output, err.Error())
		os.Exit(125)
	}
	if err := output.Sync(); err != nil {
		os.Exit(125)
	}
	os.Exit(0)
}

func decodeWorkerRequest(file *os.File, target any) error {
	decoder := json.NewDecoder(io.LimitReader(file, 32<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("worker request contains trailing data")
	}
	return nil
}

func miseScenario(ctx context.Context, cfg Config, root string, _ []byte) Scenario {
	failed := Scenario{Name: "mise-node-24.4.1-authored-lock", Status: "failed", RequirementIDs: []string{"VAL-016", "VAL-017", "VAL-018", "VAL-019", "VAL-021", "VAL-023", "PC-031", "PC-043", "PC-050"}, RejectedAssumption: "An authored lock substring, post-install hash, or network-enabled manager extraction proves pre-use archive authentication."}
	if _, err := inspectLock([]byte(cfg.NodeLock)); err != nil {
		failed.Detail = err.Error() + "; manager invocations=0"
		return failed
	}
	unshare, err := filepath.EvalSymlinks("/usr/bin/unshare")
	if err != nil || !filepath.IsAbs(unshare) {
		failed.Detail = "private namespace immutability unavailable: canonical unshare executable unavailable; mise invocations=0"
		return failed
	}
	current, err := user.Current()
	if err != nil {
		failed.Detail = "private namespace immutability unavailable: current user identity unavailable; mise invocations=0"
		return failed
	}
	uidStart, uidCount, uidErr := subordinateIdentityRange("/etc/subuid", current.Username)
	gidStart, gidCount, gidErr := subordinateIdentityRange("/etc/subgid", current.Username)
	if uidErr != nil || gidErr != nil || uidCount == 0 || gidCount == 0 {
		failed.Detail = "private namespace immutability unavailable: subordinate UID/GID mapping unavailable; mise invocations=0"
		return failed
	}
	mountPoint, err := os.MkdirTemp("/tmp", "zerb-mise-private-")
	if err != nil {
		failed.Detail = "private namespace immutability unavailable: " + err.Error() + "; mise invocations=0"
		return failed
	}
	if err := os.Chmod(mountPoint, 0711); err != nil {
		os.Remove(mountPoint)
		failed.Detail = "private namespace immutability unavailable: " + err.Error() + "; mise invocations=0"
		return failed
	}
	defer os.RemoveAll(mountPoint)
	cgroup, err := lifecycle.NewCgroup()
	if err != nil {
		failed.Detail = "private namespace mandatory cgroup unavailable: " + err.Error() + "; mise invocations=0"
		return failed
	}
	defer cgroup.Close()
	cgroupDirectory, err := os.Open(cgroup.Path())
	if err != nil {
		failed.Detail = "private namespace mandatory cgroup descriptor unavailable: " + err.Error() + "; mise invocations=0"
		return failed
	}
	request := miseNamespaceRequest{Config: cfg, MountPoint: mountPoint, ProtectedRoots: cfg.ProtectedRoots, CgroupPath: cgroup.Path()}
	requestFile, reportFile, err := workerFiles(root, "mise-namespace", request)
	if err != nil {
		cgroupDirectory.Close()
		failed.Detail = "private namespace immutability unavailable: " + err.Error() + "; mise invocations=0"
		return failed
	}
	defer removeWorkerFiles(requestFile, reportFile)
	cmd := exec.CommandContext(ctx, unshare,
		"--map-users", fmt.Sprintf("0:%d:1", os.Getuid()),
		"--map-users", fmt.Sprintf("1:%d:1", uidStart),
		"--map-groups", fmt.Sprintf("0:%d:1", os.Getgid()),
		"--map-groups", fmt.Sprintf("1:%d:1", gidStart),
		"--mount", "--pid", "--fork", "--kill-child", "--mount-proc", "--setuid", "0", "--setgid", "0",
		cfg.Executable)
	cmd.Dir = root
	cmd.Env = []string{miseNamespaceMode + "=1"}
	cmd.ExtraFiles = []*os.File{requestFile, reportFile}
	cmd.SysProcAttr = &syscall.SysProcAttr{UseCgroupFD: true, CgroupFD: int(cgroupDirectory.Fd())}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	cgroupDirectory.Close()
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if _, seekErr := reportFile.Seek(0, io.SeekStart); seekErr == nil {
			if report, readErr := io.ReadAll(io.LimitReader(reportFile, 4096)); readErr == nil && len(bytes.TrimSpace(report)) != 0 {
				detail = strings.TrimSpace(string(report))
			}
		}
		detail = strings.ReplaceAll(strings.ReplaceAll(detail, root, "<proof-root>"), mountPoint, "<private-mount>")
		cgroupObservation, cgroupErr := cgroup.Observe()
		if detail == "" {
			detail = runErr.Error()
		}
		detail += fmt.Sprintf("; cgroup_memory_peak=%d,cgroup_process_peak=%d,cgroup_oom=%d,cgroup_oom_kill=%d,cgroup_empty=%t,cgroup_observation_error=%v", cgroupObservation.MemoryPeak, cgroupObservation.ProcessPeak, cgroupObservation.MemoryOOM, cgroupObservation.OOMKill, cgroupObservation.Empty, cgroupErr)
		failed.Detail = "private namespace immutability unavailable: " + detail + "; mise rows failed, manager evidence inconclusive"
		return failed
	}
	if _, err := reportFile.Seek(0, io.SeekStart); err != nil {
		failed.Detail = "private namespace report unavailable: " + err.Error()
		return failed
	}
	var scenario Scenario
	if err := decodeWorkerRequest(reportFile, &scenario); err != nil {
		failed.Detail = "private namespace report invalid: " + err.Error()
		return failed
	}
	cgroupObservation, observeErr := cgroup.Observe()
	closeErr := cgroup.Close()
	if observeErr != nil || closeErr != nil || !cgroupObservation.Empty || cgroupObservation.MemoryOOM != 0 || cgroupObservation.OOMKill != 0 {
		failed.Detail = fmt.Sprintf("private namespace cgroup evidence incomplete: observed=%v cleanup=%v empty=%t memory_oom=%d oom_kill=%d", observeErr, closeErr, cgroupObservation.Empty, cgroupObservation.MemoryOOM, cgroupObservation.OOMKill)
		return failed
	}
	scenario.Resources.CgroupComplete = scenario.Resources.CgroupComplete && cgroupObservation.Empty
	scenario.Resources.MemoryPeakBytes = cgroupObservation.MemoryPeak
	scenario.Resources.ProcessPeak = cgroupObservation.ProcessPeak
	scenario.Resources.CPUUsec = cgroupObservation.CPUUsec
	scenario.Resources.MemoryOOM = cgroupObservation.MemoryOOM
	scenario.Resources.OOMKill = cgroupObservation.OOMKill
	return scenario
}

func subordinateIdentityRange(name, username string) (uint64, uint64, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) != 3 || fields[0] != username {
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

func workerFiles(root, prefix string, request any) (*os.File, *os.File, error) {
	input, err := os.CreateTemp(root, "."+prefix+"-input-")
	if err != nil {
		return nil, nil, err
	}
	output, err := os.CreateTemp(root, "."+prefix+"-output-")
	if err != nil {
		input.Close()
		os.Remove(input.Name())
		return nil, nil, err
	}
	if err := json.NewEncoder(input).Encode(request); err != nil {
		removeWorkerFiles(input, output)
		return nil, nil, err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		removeWorkerFiles(input, output)
		return nil, nil, err
	}
	return input, output, nil
}

func removeWorkerFiles(files ...*os.File) {
	for _, file := range files {
		if file != nil {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
		}
	}
}

func runMiseNamespaceHarness(ctx context.Context, request miseNamespaceRequest) (Scenario, error) {
	if os.Geteuid() != 0 || os.Getegid() != 0 {
		return Scenario{}, errors.New("namespace harness lacks namespaced setup identity")
	}
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return Scenario{}, fmt.Errorf("make namespace mounts private: %w", err)
	}
	if err := syscall.Mount("tmpfs", request.MountPoint, "tmpfs", syscall.MS_NODEV|syscall.MS_NOSUID, "mode=0711,size=768m,nr_inodes=262144"); err != nil {
		return Scenario{}, fmt.Errorf("mount capped private workspace: %w", err)
	}
	workspaceMounted := true
	archiveMounted := false
	var archiveMount string
	defer func() {
		if archiveMounted {
			_ = syscall.Unmount(archiveMount, syscall.MNT_DETACH)
		}
		if workspaceMounted {
			_ = syscall.Unmount(request.MountPoint, syscall.MNT_DETACH)
		}
	}()
	lock, err := inspectLock([]byte(request.Config.NodeLock))
	if err != nil {
		return Scenario{}, err
	}
	root := filepath.Join(request.MountPoint, "proof")
	dir := filepath.Join(root, "mise")
	managed := []string{dir, filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "state"), filepath.Join(dir, "tmp"), filepath.Join(dir, "home"), filepath.Join(dir, "xdg-config"), filepath.Join(dir, "xdg-config-system"), filepath.Join(dir, "xdg-cache"), filepath.Join(dir, "xdg-data"), filepath.Join(dir, "xdg-data-system"), filepath.Join(dir, "xdg-state"), filepath.Join(dir, "xdg-runtime"), filepath.Join(dir, "system-config"), filepath.Join(dir, "system-data"), filepath.Join(dir, "zoneinfo")}
	for _, name := range managed {
		if err := os.MkdirAll(name, 0700); err != nil {
			return Scenario{}, err
		}
	}
	if err := chownTree(root, 1, 1); err != nil {
		return Scenario{}, err
	}
	workerExecutable := filepath.Join(root, "adapter-worker")
	if err := copyWorkerExecutable(request.Config.Executable, workerExecutable); err != nil {
		return Scenario{}, err
	}
	request.Config.Executable = workerExecutable
	if err := writeManagedFile(filepath.Join(dir, "mise.lock"), []byte(request.Config.NodeLock), 0600, 1, 1); err != nil {
		return Scenario{}, err
	}
	config := []byte("[tools]\nnode = \"24.4.1\"\n\n[settings]\nauto_install = false\nexec_auto_install = false\nnot_found_auto_install = false\nauto_env = false\nlockfile = true\nlegacy_version_file = false\nasdf_compat = false\nalways_keep_download = true\n")
	if err := writeManagedFile(filepath.Join(dir, "mise.toml"), config, 0600, 1, 1); err != nil {
		return Scenario{}, err
	}
	archiveMount = filepath.Join(dir, "data", "downloads", "node", lock.Version)
	if err := os.MkdirAll(archiveMount, 0700); err != nil {
		return Scenario{}, err
	}
	if err := chownTree(root, 1, 1); err != nil {
		return Scenario{}, err
	}
	archiveCap := lock.Size + 1<<20
	mountData := fmt.Sprintf("mode=0700,uid=1,gid=1,size=%d,nr_inodes=16", archiveCap)
	if err := syscall.Mount("tmpfs", archiveMount, "tmpfs", syscall.MS_NODEV|syscall.MS_NOSUID, mountData); err != nil {
		return Scenario{}, fmt.Errorf("mount capped archive cache: %w", err)
	}
	archiveMounted = true
	archivePath, descriptor, before, err := acquireArchiveDescriptor(ctx, archiveMount, lock, request.Config.CACert)
	if err != nil {
		return Scenario{}, err
	}
	defer descriptor.Close()
	replacementPath := filepath.Join(archiveMount, ".replacement-injection")
	if err := writeManagedFile(replacementPath, []byte("replacement\n"), 0600, 1, 1); err != nil {
		return Scenario{}, err
	}
	if err := descriptor.Chown(1, 1); err != nil {
		return Scenario{}, err
	}
	if err := descriptor.Sync(); err != nil {
		return Scenario{}, err
	}
	archiveDirectory, err := os.Open(archiveMount)
	if err != nil {
		return Scenario{}, err
	}
	if err := archiveDirectory.Sync(); err != nil {
		archiveDirectory.Close()
		return Scenario{}, err
	}
	archiveDirectory.Close()
	if err := syscall.Mount("", archiveMount, "tmpfs", syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_NODEV|syscall.MS_NOSUID, ""); err != nil {
		return Scenario{}, fmt.Errorf("remount authenticated archive cache read-only: %w", err)
	}
	var after syscall.Stat_t
	if err := syscall.Fstat(int(descriptor.Fd()), &after); err != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || after.Nlink != 1 {
		return Scenario{}, errors.New("authenticated archive descriptor identity changed across publication/remount")
	}
	if err := verifyPublishedArchiveIdentity(descriptor, archivePath); err != nil {
		return Scenario{}, err
	}
	inventory, err := inventoryAuthenticatedArchive(descriptor)
	if err != nil {
		return Scenario{}, err
	}
	injection, err := invokeArchiveAttack(request.Config.Executable, dir, archivePath, replacementPath)
	if err != nil {
		return Scenario{}, err
	}
	if injection.UID != 1 || !injection.ModifyRejected || !injection.ReplaceRejected || !injection.RemoveRejected {
		return Scenario{}, errors.New("same-UID post-hash archive attack was not rejected by the immutable mount boundary")
	}
	if err := verifyArchiveDescriptor(descriptor, lock, before); err != nil {
		return Scenario{}, err
	}
	runnerConfig := request.Config
	if err := verifyPublishedArchiveIdentity(descriptor, archivePath); err != nil {
		return Scenario{}, err
	}
	cgroupRoot, err := currentCgroupRoot()
	if err != nil {
		return Scenario{}, err
	}
	runnerConfig.Root = root
	runnerConfig.CACert = ""
	runnerConfig.ProtectedRoots = nil
	runnerConfig.PlatformCgroupRoot = cgroupRoot
	runnerConfig.ExecutionCgroupPath = request.CgroupPath
	runner := miseRunnerRequest{Config: runnerConfig, Root: root, Directory: dir, ArchivePath: archivePath, Inventory: inventory, Injection: injection, ProtectedRoots: request.ProtectedRoots, PlatformCgroupRoot: cgroupRoot, CgroupPath: request.CgroupPath}
	scenario, runErr := invokeMiseRunner(ctx, runner)
	if runErr == nil {
		runErr = verifyPublishedArchiveIdentity(descriptor, archivePath)
	}
	if runErr == nil {
		runErr = verifyArchiveDescriptor(descriptor, lock, before)
	}
	if closeErr := descriptor.Close(); runErr == nil && closeErr != nil {
		runErr = closeErr
	}
	if runErr != nil {
		return Scenario{}, runErr
	}
	if err := syscall.Unmount(archiveMount, 0); err != nil {
		return Scenario{}, fmt.Errorf("unmount immutable archive cache: %w", err)
	}
	archiveMounted = false
	if err := syscall.Unmount(request.MountPoint, 0); err != nil {
		return Scenario{}, fmt.Errorf("unmount private workspace: %w", err)
	}
	workspaceMounted = false
	if err := os.Remove(request.MountPoint); err != nil {
		return Scenario{}, fmt.Errorf("remove private workspace mountpoint: %w", err)
	}
	return scenario, nil
}

func chownTree(root string, uid, gid int) error {
	return filepath.Walk(root, func(name string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(name, uid, gid)
	})
}

func writeManagedFile(name string, content []byte, mode os.FileMode, uid, gid int) error {
	if err := writeExclusive(name, content, mode); err != nil {
		return err
	}
	return os.Chown(name, uid, gid)
}

func copyWorkerExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0555)
	if err != nil {
		return err
	}
	copied, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	sourceInfo, statErr := input.Stat()
	if copyErr != nil {
		return copyErr
	}
	if statErr != nil || copied != sourceInfo.Size() {
		return errors.New("namespaced worker executable copy is incomplete")
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chown(destination, 1, 1)
}

func acquireArchiveDescriptor(ctx context.Context, cache string, lock lockFact, caCert string) (string, *os.File, syscall.Stat_t, error) {
	parsed, err := url.Parse(lock.URL)
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	base := path.Base(parsed.Path)
	if base == "." || base == "/" || base == "" {
		return "", nil, syscall.Stat_t{}, errors.New("archive URL has no canonical cache name")
	}
	part := filepath.Join(cache, ".unauthenticated")
	file, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	keep := false
	defer func() {
		if !keep {
			file.Close()
			os.Remove(part)
		}
	}()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, lock.URL, nil)
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	_, pinnedAddresses, err := sourceurl.PinnedPublicHTTPS(ctx, lock.URL, net.DefaultResolver)
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	expectedAddress := net.JoinHostPort(parsed.Hostname(), port)
	dialer := &net.Dialer{}
	transport := &http.Transport{DisableCompression: true, Proxy: nil}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		if address != expectedAddress {
			return nil, errors.New("archive acquisition attempted an unbound endpoint")
		}
		var failures []error
		for _, pinned := range pinnedAddresses {
			connection, dialErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(pinned.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			failures = append(failures, dialErr)
		}
		return nil, errors.Join(failures...)
	}
	if caCert != "" {
		roots := x509.NewCertPool()
		pem, readErr := os.ReadFile(caCert)
		if readErr != nil || !roots.AppendCertsFromPEM(pem) {
			return "", nil, syscall.Stat_t{}, errors.New("configured provenance CA bundle is invalid")
		}
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: parsed.Hostname()}
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("archive provenance redirect rejected")
	}, Transport: transport}
	response, err := client.Do(request)
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", nil, syscall.Stat_t{}, fmt.Errorf("archive provenance returned HTTP status %d", response.StatusCode)
	}
	written, err := io.Copy(file, io.LimitReader(response.Body, lock.Size+1))
	if err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	if written != lock.Size {
		return "", nil, syscall.Stat_t{}, fmt.Errorf("archive size mismatch: got %d bytes", written)
	}
	if err := verifyArchiveDescriptor(file, lock, syscall.Stat_t{}); err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	var identity syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &identity); err != nil || identity.Nlink != 1 || identity.Size != lock.Size {
		return "", nil, syscall.Stat_t{}, errors.New("authenticated archive descriptor identity is invalid")
	}
	authenticated := filepath.Join(cache, base)
	if err := os.Rename(part, authenticated); err != nil {
		return "", nil, syscall.Stat_t{}, err
	}
	readOnly, err := os.OpenFile(authenticated, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		_ = os.Remove(authenticated)
		return "", nil, syscall.Stat_t{}, err
	}
	var reopened syscall.Stat_t
	if err := syscall.Fstat(int(readOnly.Fd()), &reopened); err != nil || reopened.Dev != identity.Dev || reopened.Ino != identity.Ino || reopened.Size != identity.Size || reopened.Nlink != 1 {
		readOnly.Close()
		_ = os.Remove(authenticated)
		return "", nil, syscall.Stat_t{}, errors.New("authenticated archive read-only descriptor identity mismatch")
	}
	if err := file.Close(); err != nil {
		readOnly.Close()
		_ = os.Remove(authenticated)
		return "", nil, syscall.Stat_t{}, err
	}
	keep = true
	return authenticated, readOnly, reopened, nil
}

func verifyArchiveDescriptor(file *os.File, lock lockFact, expected syscall.Stat_t) error {
	var identity syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &identity); err != nil {
		return err
	}
	if identity.Size != lock.Size || identity.Nlink != 1 {
		return errors.New("archive descriptor size or link count changed")
	}
	if expected.Ino != 0 && (identity.Dev != expected.Dev || identity.Ino != expected.Ino || identity.Size != expected.Size) {
		return errors.New("archive descriptor identity changed")
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	read, err := io.Copy(hash, file)
	if err != nil || read != lock.Size {
		return errors.New("archive descriptor could not be completely authenticated")
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != lock.Checksum {
		return errors.New("archive descriptor checksum disagrees with independently authored lock")
	}
	return nil
}
func verifyPublishedArchiveIdentity(file *os.File, name string) error {
	descriptorInfo, err := file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(name)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(descriptorInfo, pathInfo) {
		return errors.New("exact published cache name does not identify the authenticated archive descriptor")
	}
	return nil
}

func inventoryAuthenticatedArchive(file *os.File) (archiveInventory, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return archiveInventory{}, err
	}
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return archiveInventory{}, fmt.Errorf("authenticated archive gzip framing: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	inventory := archiveInventory{Entries: make(map[string]archiveEntry)}
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return archiveInventory{}, fmt.Errorf("authenticated archive tar framing: %w", err)
		}
		if len(inventory.Entries) >= maxInventoryItems || header.Size < 0 || len(header.Name) == 0 || len(header.Name) > 4096 || strings.IndexByte(header.Name, 0) >= 0 || !utf8.ValidString(header.Name) {
			return archiveInventory{}, errors.New("archive inventory bound exceeded")
		}
		trimmed := strings.TrimSuffix(header.Name, "/")
		clean := path.Clean(trimmed)
		if path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != trimmed {
			return archiveInventory{}, errors.New("archive contains traversal or non-canonical path")
		}
		root, relative, found := strings.Cut(clean, "/")
		if inventory.Root == "" {
			inventory.Root = root
		}
		if root != inventory.Root {
			return archiveInventory{}, errors.New("archive does not have exactly one stripped root")
		}
		if !found {
			if header.Typeflag != tar.TypeDir {
				return archiveInventory{}, errors.New("archive root is not a directory")
			}
			continue
		}
		if relative == "" || path.Clean(relative) != relative || strings.HasPrefix(relative, "../") {
			return archiveInventory{}, errors.New("archive stripped path is not canonical")
		}
		if _, duplicate := inventory.Entries[relative]; duplicate {
			return archiveInventory{}, errors.New("archive contains a canonical path alias")
		}
		entry := archiveEntry{Mode: uint32(header.Mode) & 07777}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			total += header.Size
			if total > maxInventoryBytes {
				return archiveInventory{}, errors.New("archive expanded-byte bound exceeded")
			}
			hash := sha256.New()
			read, copyErr := io.Copy(hash, io.LimitReader(reader, header.Size+1))
			if copyErr != nil || read != header.Size {
				return archiveInventory{}, errors.New("archive regular file was not completely inventoried")
			}
			entry.Type, entry.Size, entry.SHA256 = "regular", header.Size, hex.EncodeToString(hash.Sum(nil))
		case tar.TypeDir:
			entry.Type = "directory"
		case tar.TypeSymlink:
			if header.Size != 0 || len(header.Linkname) == 0 || len(header.Linkname) > 4096 || !utf8.ValidString(header.Linkname) || path.IsAbs(header.Linkname) || path.Clean(header.Linkname) != header.Linkname {
				return archiveInventory{}, errors.New("archive contains invalid symlink target")
			}
			resolved := path.Clean(path.Join(path.Dir(relative), header.Linkname))
			if resolved == ".." || strings.HasPrefix(resolved, "../") {
				return archiveInventory{}, errors.New("archive symlink traverses outside stripped root")
			}
			entry.Type, entry.Mode, entry.Target = "symlink", 0777, header.Linkname
		default:
			return archiveInventory{}, errors.New("archive contains alias or unsupported entry type")
		}
		inventory.Entries[relative] = entry
	}
	if inventory.Root == "" || len(inventory.Entries) == 0 {
		return archiveInventory{}, errors.New("archive inventory is empty")
	}
	return inventory, nil
}

func invokeArchiveAttack(executable, dir, archivePath, replacementPath string) (archiveInjectionObservation, error) {
	request := archiveAttackRequest{ArchivePath: archivePath, ReplacementPath: replacementPath}
	input, output, err := workerFiles(dir, "archive-attack", request)
	if err != nil {
		return archiveInjectionObservation{}, err
	}
	defer removeWorkerFiles(input, output)
	cmd := exec.Command(executable)
	cmd.Dir = dir
	cmd.Env = []string{miseAttackMode + "=1"}
	cmd.ExtraFiles = []*os.File{input, output}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1, Gid: 1, NoSetGroups: true}, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Run(); err != nil {
		return archiveInjectionObservation{}, fmt.Errorf("same-UID archive attack process: %w", err)
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return archiveInjectionObservation{}, err
	}
	var observation archiveInjectionObservation
	if err := decodeWorkerRequest(output, &observation); err != nil {
		return archiveInjectionObservation{}, err
	}
	return observation, nil
}

func runArchiveAttack(request archiveAttackRequest) archiveInjectionObservation {
	observation := archiveInjectionObservation{UID: os.Geteuid()}
	file, err := os.OpenFile(request.ArchivePath, os.O_WRONLY, 0)
	if err == nil {
		_, err = file.WriteAt([]byte{0}, 0)
		file.Close()
	}
	observation.ModifyRejected = errors.Is(err, syscall.EROFS)
	err = os.Rename(request.ReplacementPath, request.ArchivePath)
	observation.ReplaceRejected = errors.Is(err, syscall.EROFS)
	err = os.Remove(request.ArchivePath)
	observation.RemoveRejected = errors.Is(err, syscall.EROFS)
	return observation
}

func invokeMiseRunner(ctx context.Context, request miseRunnerRequest) (Scenario, error) {
	input, output, err := workerFiles(request.Directory, "mise-runner", request)
	if err != nil {
		return Scenario{}, err
	}
	defer removeWorkerFiles(input, output)
	cmd := exec.CommandContext(ctx, request.Config.Executable)
	cmd.Dir = request.Directory
	cmd.Env = []string{miseRunnerMode + "=1"}
	cmd.ExtraFiles = []*os.File{input, output}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1, Gid: 1, NoSetGroups: true}}
	if err := cmd.Run(); err != nil {
		return Scenario{}, fmt.Errorf("unprivileged mise runner: %w", err)
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return Scenario{}, err
	}
	var scenario Scenario
	if err := decodeWorkerRequest(output, &scenario); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

func compareInstalledArchiveTree(root string, inventory archiveInventory) (int, string, error) {
	seen := make(map[string]bool, len(inventory.Entries))
	executableDigest := ""
	entries := 0
	err := filepath.Walk(root, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if name == root {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		expected, ok := inventory.Entries[relative]
		if !ok {
			return fmt.Errorf("installed tree contains entry absent from authenticated archive inventory: %s", relative)
		}
		if seen[relative] {
			return errors.New("installed tree contains a canonical path alias")
		}
		seen[relative] = true
		entries++
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("installed identity unavailable: %s", relative)
		}
		actualMode := uint32(stat.Mode) & 07777
		switch expected.Type {
		case "directory":
			if !info.IsDir() || actualMode != expected.Mode {
				return fmt.Errorf("installed directory identity mismatch: %s", relative)
			}
		case "symlink":
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(name)
				if err != nil || target != expected.Target || actualMode != 0777 {
					return fmt.Errorf("installed symlink identity mismatch: %s", relative)
				}
				break
			}
			digest, err := fileSHA256(name)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("installed normalized symlink is not a readable regular file: %s", relative)
			}
			matched := false
			for _, target := range archiveRegularTargets(inventory, relative, expected.Target) {
				if info.Size() == target.Size && actualMode == target.Mode && digest == target.SHA256 {
					matched = true
					break
				}
			}
			if !matched {
				if wrapper, ok := expectedMiseWrapper(relative); ok && info.Size() == wrapper.Size && actualMode == wrapper.Mode && digest == wrapper.SHA256 {
					matched = true
				}
			}
			if !matched {
				return fmt.Errorf("installed normalized symlink content has no authenticated or frozen-mise relation: %s size=%d mode=%#o sha256=%s authenticated_content_matches=%s", relative, info.Size(), actualMode, digest, strings.Join(archiveRegularContentMatches(inventory, info.Size(), actualMode, digest), ","))
			}
		case "regular":
			if !info.Mode().IsRegular() || info.Size() != expected.Size || actualMode != expected.Mode {
				return fmt.Errorf("installed regular-file identity mismatch: %s", relative)
			}
			digest, err := fileSHA256(name)
			if err != nil || digest != expected.SHA256 {
				return fmt.Errorf("installed regular-file content mismatch: %s", relative)
			}
			if relative == "bin/node" {
				if expected.Mode&0111 == 0 {
					return errors.New("authenticated archive node identity is not executable")
				}
				executableDigest = digest
			}
		default:
			return errors.New("authenticated inventory contains unsupported type")
		}
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	if len(seen) != len(inventory.Entries) {
		missing := make([]string, 0)
		for name := range inventory.Entries {
			if !seen[name] {
				missing = append(missing, name)
			}
		}

		sort.Strings(missing)
		return 0, "", fmt.Errorf("installed tree omitted authenticated archive entry: %s", missing[0])
	}
	if executableDigest == "" {
		return 0, "", errors.New("authenticated archive inventory lacks executable bin/node identity")
	}
	return entries, executableDigest, nil
}
func archiveRegularTargets(inventory archiveInventory, source, target string) []archiveEntry {
	resolved := path.Clean(path.Join(path.Dir(source), target))
	candidates := []string{resolved, path.Join(path.Dir(resolved), path.Base(source))}
	seen := make(map[string]bool, len(candidates))
	out := make([]archiveEntry, 0, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if entry, ok := resolveArchiveRegular(inventory, candidate); ok {
			out = append(out, entry)
		}
	}
	return out
}

func resolveArchiveRegular(inventory archiveInventory, current string) (archiveEntry, bool) {
	for range 32 {
		if current == "." || current == ".." || strings.HasPrefix(current, "../") {
			return archiveEntry{}, false
		}
		entry, ok := inventory.Entries[current]
		if !ok {
			return archiveEntry{}, false
		}
		switch entry.Type {
		case "regular":
			return entry, true
		case "symlink":
			current = path.Clean(path.Join(path.Dir(current), entry.Target))
		default:
			return archiveEntry{}, false
		}
	}
	return archiveEntry{}, false
}

func archiveRegularContentMatches(inventory archiveInventory, size int64, mode uint32, digest string) []string {
	matches := make([]string, 0)
	for name, entry := range inventory.Entries {
		if entry.Type == "regular" && entry.Size == size && entry.Mode == mode && entry.SHA256 == digest {
			matches = append(matches, name)
		}
	}

	sort.Strings(matches)
	return matches
}

// expectedMiseWrapper freezes the sole content transformation observed for the
// exact mise 2026.7.17 / Node 24.4.1 tuple. Every other archived symlink must
// remain a symlink or match authenticated archive regular-file content.
func expectedMiseWrapper(name string) (archiveEntry, bool) {
	switch name {
	case "bin/npm":
		return archiveEntry{Type: "regular", Mode: 0755, Size: 1831, SHA256: "a74679065974f6fd0a02f7ea8d4690d7bccd6437981924201ed37fc010112e22"}, true
	default:
		return archiveEntry{}, false
	}
}
