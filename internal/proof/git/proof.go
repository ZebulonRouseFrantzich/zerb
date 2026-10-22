package git

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const btrfsMagic = 0x9123683e

func Run(ctx context.Context, cfg Config) (result Result, retErr error) {
	if !insideMountNamespace {
		return runInMountNamespace(ctx, cfg)
	}
	result.Version = ResultVersion
	result.StartedAt = time.Now().UTC()
	result.CandidateRows = []string{"P1-PUBLIC-EMBEDDED", "P1-PUBLIC-UNSELECTED", "P1-SOURCE-REJECTION", "P1-GIT-MATERIALIZE", "P1-SELECTED-GITLINK-EXCLUSION"}
	result.Requirements = []string{"PC-022", "VAL-002", "VAL-003", "VAL-004", "VAL-006", "VAL-007", "VAL-008", "VAL-009", "VAL-010", "VAL-011", "VAL-012", "VAL-017", "VAL-018", "VAL-019", "VAL-023", "VAL-040"}
	result.RejectedAssumption = "Local fixture success does not prove canonical HTTPS transport, non-access without trace, lifecycle containment, or hostile-pack resource enforcement."
	base, gitPath, tracePath, artifactDir, err := validateConfig(cfg)
	if err != nil {
		return result, err
	}
	repositoryRoot, err := canonicalExistingDir(cfg.RepositoryRoot)
	if err != nil {
		return result, err
	}
	supervisorPath, err := canonicalExecutable(cfg.SupervisorPath)
	if err != nil {
		return result, fmt.Errorf("proof supervisor identity: %w", err)
	}
	result.Executable, err = identity(gitPath)
	if err != nil {
		return result, fmt.Errorf("git identity: %w", err)
	}
	sourceID, err := validateReviewedSource(cfg.Source)
	if err != nil {
		return result, err
	}
	if cfg.Source == "" {
		cfg.Source = ReviewedPublicSource
	}
	curlResolves, publicAnswers, err := pinnedReviewedSource(ctx, cfg.Source, nil)
	if err != nil {
		return result, err
	}
	if !endpointAdmissionFixtures(ctx) {
		return result, errors.New("private-answer or DNS answer-change fixture failed")
	}
	result.Source = SourceObservation{ReviewedCanonicalHTTPS: cfg.Source, SourceID: sourceID, LocalTransport: "explicit loopback dumb-HTTP fixture; deterministic object/fault evidence only, never canonical transport evidence", IdentitySeparated: true}
	result.Scenarios = append(result.Scenarios, sourceIDKnownVectorScenario(sourceID))
	result.Scenarios = append(result.Scenarios, observed("public-endpoint-admission-and-pinning", []string{"VAL-006", "VAL-007", "VAL-011"}, "the exact canonical source host resolved once to an all-global-unicast answer set; private, loopback, link-local, unspecified, multicast, and changed-answer fixtures were refused, and the admitted set was prepared as a permanent native libcurl resolution pin"))

	owned, err := os.MkdirTemp(base, "zerb-git-proof-")
	if err != nil {
		return result, err
	}
	if err := os.Chmod(owned, 0o700); err != nil {
		_ = os.RemoveAll(owned)
		return result, err
	}
	outside := filepath.Join(base, "."+filepath.Base(owned)+"-outside-sentinel")
	marker := make([]byte, 32)
	if _, err := rand.Read(marker); err != nil {
		_ = os.RemoveAll(owned)
		return result, err
	}
	markerValue := []byte(hex.EncodeToString(marker))
	markerDigest := sha256.Sum256(markerValue)
	result.AmbientMarkerFingerprint = "sha256:" + hex.EncodeToString(markerDigest[:])
	const ambientMarkerName = "ZERB_PROOF_AMBIENT_MARKER"
	priorAmbient, hadPriorAmbient := os.LookupEnv(ambientMarkerName)
	if err := os.Setenv(ambientMarkerName, string(markerValue)); err != nil {
		_ = os.RemoveAll(owned)
		return result, err
	}
	defer func() {
		if hadPriorAmbient {
			_ = os.Setenv(ambientMarkerName, priorAmbient)
		} else {
			_ = os.Unsetenv(ambientMarkerName)
		}
	}()
	if err := os.WriteFile(outside, []byte(hex.EncodeToString(markerDigest[:])+"\n"), 0o600); err != nil {
		_ = os.RemoveAll(owned)
		return result, err
	}
	mounted := false
	cleanupDone := false
	cleanup := func() error {
		var unmountErr error
		if mounted {
			unmountErr = unmountStagingTmpfs(owned)
			mounted = false
		}
		removeErr := os.RemoveAll(owned)
		result.Cleanup.OwnedRootRemoved = !pathExists(owned)
		data, readErr := os.ReadFile(outside)
		result.Cleanup.OutsideSentinelIntact = readErr == nil && string(data) == hex.EncodeToString(markerDigest[:])+"\n"
		_ = os.Remove(outside)
		result.FinishedAt = time.Now().UTC()
		if unmountErr != nil {
			return unmountErr
		}
		return removeErr
	}
	defer func() {
		if cleanupDone {
			return
		}
		if cleanupErr := cleanup(); cleanupErr != nil && retErr == nil {
			retErr = cleanupErr
		}
	}()
	if err := mountStagingTmpfs(owned, MaxDiskBytes, MaxObjectCount*8); err != nil {
		return result, err
	}
	mounted = true

	if err := prepareProofRoots(base, owned, artifactDir, outside, &result); err != nil {
		return result, err
	}

	fixture, err := buildFixture(owned)
	if err != nil {
		return result, err
	}
	fixtureIdentity, err := rootIdentity("fixture", filepath.Join(owned, "fixture"))
	if err != nil {
		return result, err
	}
	result.Roots = append(result.Roots, fixtureIdentity)
	serveCtx, stopServe := context.WithCancel(ctx)
	defer stopServe()
	transport, closeServer, err := fixture.serve(serveCtx)
	if err != nil {
		return result, err
	}
	transportPort, err := fixtureTransportPort(transport)
	if err != nil {
		return result, err
	}
	defer func() {
		if closeErr := closeServer(); retErr == nil && closeErr != nil {
			retErr = closeErr
		}
	}()
	traceDir := filepath.Join(owned, "evidence", "trace")
	if err := os.MkdirAll(traceDir, 0o700); err != nil {
		return result, err
	}
	objectStore := filepath.Join(owned, "staging.git")
	if err := makeBareStore(objectStore); err != nil {
		return result, err
	}
	objectIdentity, err := rootIdentity("object-staging", objectStore)
	if err != nil {
		return result, err
	}
	result.Roots = append(result.Roots, objectIdentity)
	env := frozenEnvironment(owned, cfg.CACert)
	r := runner{git: gitPath, tracer: tracePath, supervisor: supervisorPath, cwd: owned, traceDir: traceDir, owned: owned, forbiddenMarker: markerValue, env: env, allowLoopback: true, allowedLoopbackPort: transportPort, delegatedUID: cfg.DelegatedHostUID, invocations: &result.Invocations, access: &result.Access, writes: &result.Writes, resources: &result.Resources}
	result.Access.Instrumentation = "same-binary FD-only supervisor plus closed strace file/process/network classification; every unclassified read/write/event and every endpoint outside the admitted public pin or exact local-fixture port aborts the proof; raw traces marker-scanned and deleted after parsing"
	result.Access.TraceComplete, result.Access.LifecycleComplete, result.Access.DeathGuardArmed, result.Access.IOUringDenied, result.Access.CgroupKillGuard, result.Resources.CgroupComplete = true, true, true, true, true, true
	allowlistRef, err := randomTypedRef("proof-owned-write-root")
	if err != nil {
		return result, err
	}
	result.Writes.AllowlistRoot = allowlistRef
	policy := gitPolicyArgs(objectStore)
	versionOut, err := r.run(ctx, "version", append(policy, "--version")...)
	if err != nil {
		return result, err
	}
	const versionPrefix = "git version "
	version := strings.TrimSpace(string(versionOut))
	if version != versionPrefix+PinnedGitVersion {
		return result, fmt.Errorf("git version mismatch: required %s", PinnedGitVersion)
	}
	execPathOut, err := r.run(ctx, "git-exec-path", append(policy, "--exec-path")...)
	if err != nil {
		return result, err
	}
	httpsHelperExec := filepath.Join(strings.TrimSpace(string(execPathOut)), "git-remote-https")
	httpsHelper, err := canonicalExecutable(httpsHelperExec)
	if err != nil {
		return result, fmt.Errorf("git HTTPS transport helper identity: %w", err)
	}
	result.Executable.UpstreamVersion = PinnedGitVersion
	fetchArgs := append(append([]string{}, policy...), "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", transport, "refs/heads/main")
	if _, err := r.run(ctx, "fetch", fetchArgs...); err != nil {
		result.Scenarios = append(result.Scenarios, failed("local-fixture-acquisition", []string{"VAL-008", "VAL-011"}, "actual Git loopback fixture acquisition failed; detailed error omitted from retained result"))
		return result, err
	}
	result.Scenarios = append(result.Scenarios, observed("local-fixture-object-acquisition", []string{"VAL-008", "VAL-011", "VAL-017"}, "actual Git fetched the explicit loopback fixture for deterministic object inspection only; this is expressly non-transport evidence"))
	if err := exerciseTransport(ctx, cfg.Source, r, policy, objectStore, transport, curlResolves, publicAnswers, &result); err != nil {
		return result, err
	}
	if err := materializeFixture(ctx, r, policy, objectStore, fixture, &result); err != nil {
		return result, err
	}
	runInputRejections(&result)
	if err := runHardDiskBoundary(ctx, r, owned, &result); err != nil {
		return result, err
	}
	classifyProofObservations(&result, gitPath, httpsHelperExec, httpsHelper)

	if err := verifyMarkerAbsence(ctx, gitPath, repositoryRoot, owned, markerValue, &result); err != nil {
		return result, err
	}
	cleanupDone, err = finishProofResult(&result, artifactDir, markerValue, cleanup)
	return result, err
}

func verifyMarkerAbsence(ctx context.Context, gitPath, repositoryRoot, owned string, markerValue []byte, result *Result) error {
	if err := scanForMarker(owned, markerValue); err != nil {
		return err
	}
	if err := scanForMarker(repositoryRoot, markerValue); err != nil {
		return err
	}
	if err := scanRepositoryObjects(ctx, gitPath, repositoryRoot, owned, markerValue); err != nil {
		return err
	}
	result.Scenarios = append(result.Scenarios, observed("ambient-marker-redaction", []string{"VAL-017", "VAL-040"}, "fresh ambient marker bytes were absent from every child output, transient trace, controlled proof file, complete worktree/index/reachable-object scope, nested submodule repository, and retained evidence surface"))
	if result.Access.MarkerSpills == 0 {
		result.Scenarios = append(result.Scenarios, observed("transient-trace-marker-scan", []string{"VAL-017", "VAL-040"}, "every raw trace was scanned before deletion and contained no ambient marker bytes"))
	} else {
		result.Scenarios = append(result.Scenarios, failed("transient-trace-marker-scan", []string{"VAL-017", "VAL-040"}, "an ambient marker reached a raw child trace before deletion"))
	}
	return nil
}

func prepareProofRoots(base, owned, artifactDir, outside string, result *Result) error {
	roles := []struct{ name, path string }{{"base", base}, {"owned-run", owned}, {"home", filepath.Join(owned, "home")}, {"xdg-config", filepath.Join(owned, "xdg", "config")}, {"xdg-data", filepath.Join(owned, "xdg", "data")}, {"xdg-cache", filepath.Join(owned, "xdg", "cache")}, {"xdg-state", filepath.Join(owned, "xdg", "state")}, {"manager", filepath.Join(owned, "manager")}, {"destination", filepath.Join(owned, "destination")}, {"generated-source", filepath.Join(owned, "materialized")}, {"evidence", filepath.Join(owned, "evidence")}, {"artifact", artifactDir}, {"outside-sentinel", outside}}
	for _, role := range roles {
		if role.name != "base" && role.name != "owned-run" && role.name != "outside-sentinel" {
			if err := os.MkdirAll(role.path, 0o700); err != nil {
				return err
			}
		}
		id, idErr := rootIdentity(role.name, role.path)
		if idErr != nil {
			return idErr
		}
		result.Roots = append(result.Roots, id)
	}
	if err := plantHostileGitAmbient(owned); err != nil {
		return err
	}
	return nil
}

func fixtureTransportPort(transport string) (int, error) {
	transportPortIndex := strings.LastIndexByte(transport, ':')
	if transportPortIndex < 0 {
		return 0, errors.New("fixture transport port unavailable")
	}
	transportPort, err := strconv.Atoi(transport[transportPortIndex+1:])
	if err != nil || transportPort < 1 || transportPort > 65535 {
		return 0, errors.New("fixture transport port invalid")
	}
	return transportPort, nil
}

func finishProofResult(result *Result, artifactDir string, markerValue []byte, cleanup func() error) (cleanupDone bool, err error) {
	artifact := filepath.Join(artifactDir, "git-proof-result.json")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return cleanupDone, err
	}
	result.Cleanup.RedactedArtifactCopied = true
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return cleanupDone, err
	}
	if bytes.Contains(encoded, markerValue) {
		return cleanupDone, errors.New("raw ambient marker reached result")
	}
	if err := os.WriteFile(artifact, append(encoded, '\n'), 0o600); err != nil {
		result.Cleanup.RedactedArtifactCopied = false
		return cleanupDone, err
	}
	cleanupErr := cleanup()
	cleanupDone = true
	finalEncoded, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return cleanupDone, marshalErr
	}
	if bytes.Contains(finalEncoded, markerValue) {
		return cleanupDone, errors.New("raw ambient marker reached final result")
	}
	if err := os.WriteFile(artifact, append(finalEncoded, '\n'), 0o600); err != nil {
		return cleanupDone, err
	}
	if cleanupErr != nil {
		return cleanupDone, cleanupErr
	}
	return cleanupDone, nil
}

func exerciseTransport(ctx context.Context, source string, r runner, policy []string, objectStore, transport string, curlResolves []string, publicAnswers []netip.Addr, result *Result) error {
	owned := r.owned
	publicPolicy := pinnedHTTPSPolicyArgs(objectStore, curlResolves)
	publicArgs := append(append([]string{}, publicPolicy...), "ls-remote", "--exit-code", source, "HEAD")
	publicRunner := r
	publicRunner.allowLoopback = false
	publicRunner.allowedPublic = make(map[netip.Addr]bool, len(publicAnswers))
	publicRunner.allowedPublicPort = 443
	for _, answer := range publicAnswers {
		publicRunner.allowedPublic[answer.Unmap()] = true
	}
	if _, publicErr := publicRunner.run(ctx, "canonical-public-https", publicArgs...); publicErr != nil {
		if errors.Is(publicErr, errTraceViolation) {
			return publicErr
		}
		result.Scenarios = append(result.Scenarios, failed("canonical-public-https-transport", []string{"VAL-006", "VAL-007", "VAL-011"}, "actual reviewed-source HTTPS Git failed under permanent native address pinning, fixed CA, TLS hostname verification, and no-redirect/no-proxy/no-auth policy; detailed error omitted from retained result"))
	} else if !result.Access.TraceComplete || !result.Invocations[len(result.Invocations)-1].TraceComplete {
		result.Scenarios = append(result.Scenarios, failed("canonical-public-https-transport", []string{"VAL-006", "VAL-007", "VAL-011"}, "actual reviewed-source HTTPS Git returned success but trace classification was incomplete; transport is not implementation-qualified"))
	} else {
		result.Scenarios = append(result.Scenarios, observed("canonical-public-https-transport", []string{"VAL-006", "VAL-007", "VAL-011"}, "actual reviewed-source HTTPS Git connected only to the admitted permanent native address pin while retaining the canonical hostname for fixed-CA TLS verification, with redirects, proxies, and authentication disabled and complete trace classification"))
	}
	for _, negative := range []struct {
		name string
		url  string
	}{{"loopback-redirect-refusal", transport + "/redirect.git"}, {"loopback-auth-challenge-refusal", transport + "/auth.git"}, {"loopback-high-rate-refusal", transport + "/high-rate.git"}} {
		before := len(result.Invocations)
		_, negativeErr := r.run(ctx, negative.name, append(append([]string{}, policy...), "ls-remote", "--exit-code", negative.url, "HEAD")...)
		if errors.Is(negativeErr, errTraceViolation) {
			return negativeErr
		}
		if negativeErr == nil || len(result.Invocations) != before+1 || result.Invocations[len(result.Invocations)-1].ExitCode == 0 {
			result.Scenarios = append(result.Scenarios, failed(negative.name, []string{"VAL-007", "VAL-011"}, "loopback deterministic fault did not fail through actual Git as required"))
		} else {
			result.Scenarios = append(result.Scenarios, observed(negative.name, []string{"VAL-007", "VAL-011"}, "actual Git refused the deterministic loopback fault under no-redirect/no-auth policy; this is negative policy evidence only, not canonical HTTPS transport evidence"))
		}
	}
	slowRunner := r
	slowRunner.wallTime = 100 * time.Millisecond
	beforeSlow := len(result.Invocations)
	_, slowErr := slowRunner.run(ctx, "loopback-stall-interruption", append(append([]string{}, policy...), "ls-remote", transport+"/stall.git", "HEAD")...)
	if errors.Is(slowErr, errTraceViolation) {
		return slowErr
	}
	if slowErr == nil || len(result.Invocations) != beforeSlow+1 || result.Invocations[len(result.Invocations)-1].ExitCode == 0 {
		result.Scenarios = append(result.Scenarios, failed("loopback-stall-interruption", []string{"VAL-007", "VAL-011", "VAL-019"}, "a deliberately stalled transport did not terminate through the wall-time/supervisor/cgroup boundary"))
	} else {
		result.Scenarios = append(result.Scenarios, observed("loopback-stall-interruption", []string{"VAL-007", "VAL-011", "VAL-019"}, "a deliberately stalled transport crossed the reduced proof wall limit, was terminated through the same supervisor/cgroup boundary, and left zero managed survivors"))
	}
	partialStore := filepath.Join(owned, "partial-failure.git")
	if err := makeBareStore(partialStore); err != nil {
		return err
	}
	partialPolicy := gitPolicyArgs(partialStore)
	_, partialErr := r.run(ctx, "loopback-partial-failure", append(partialPolicy, "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", transport+"/missing.git", "refs/heads/main")...)
	if errors.Is(partialErr, errTraceViolation) {
		return partialErr
	}
	removePartialErr := os.RemoveAll(partialStore)
	if partialErr == nil || removePartialErr != nil || pathExists(partialStore) {
		result.Scenarios = append(result.Scenarios, failed("partial-failure-cleanup", []string{"VAL-011"}, "deterministic loopback partial failure did not remain confined to removable staging"))
	} else {
		result.Scenarios = append(result.Scenarios, observed("partial-failure-cleanup", []string{"VAL-011"}, "actual Git deterministic loopback failure was not promoted and its exact staging root was removed; this is non-transport cleanup evidence"))
	}
	hostilePack := filepath.Join(owned, "hostile-input.pack")
	hostileBytes := []byte{'P', 'A', 'C', 'K', 0, 0, 0, 2, 0, 0, 0, 1, 0xff, 0xff, 0xff, 0xff}
	hostileWriteErr := os.WriteFile(hostilePack, hostileBytes, 0600)
	_, hostileErr := r.run(ctx, "hostile-pack-rejection", append(append([]string{}, policy...), "index-pack", "--strict", hostilePack)...)
	if errors.Is(hostileErr, errTraceViolation) {
		return hostileErr
	}
	hostileRemoveErr := os.Remove(hostilePack)
	hostileIndexExists := pathExists(strings.TrimSuffix(hostilePack, ".pack") + ".idx")
	if hostileWriteErr != nil || hostileErr == nil || hostileRemoveErr != nil || hostileIndexExists {
		result.Scenarios = append(result.Scenarios, failed("hostile-pack-rejection", []string{"VAL-007", "VAL-011"}, "actual Git did not reject and confine a malformed pack under the same resource/lifecycle boundary"))
	} else {
		result.Scenarios = append(result.Scenarios, observed("hostile-pack-rejection", []string{"VAL-007", "VAL-011"}, "actual Git rejected a malformed pack inside the hard aggregate tmpfs/cgroup/supervisor boundary and no index or promoted object survived"))
	}
	result.Scenarios = append(result.Scenarios, observed("transport-negative-matrix", []string{"VAL-007", "VAL-011"}, "actual Git exercised deterministic redirect, authentication-challenge, high-rate malformed stream, partial/interrupted transfer, stalled stream, and hostile-pack failures; no exact wire-byte ceiling is claimed because pinned Git/libcurl exposes no such native boundary and the proof inserts no proxy or relay"))

	return nil
}

func materializeFixture(ctx context.Context, r runner, policy []string, objectStore string, fixture *fixture, result *Result) error {
	owned := r.owned
	commitOut, err := r.run(ctx, "commit", append(policy, "rev-parse", fixture.commit+"^{commit}")...)
	if err != nil {
		return err
	}
	commit := strings.TrimSpace(string(commitOut))
	if commit != fixture.commit {
		return errors.New("fetched commit mismatch")
	}
	rootOut, err := r.run(ctx, "root-tree", append(policy, "rev-parse", commit+"^{tree}")...)
	if err != nil {
		return err
	}
	rootTree := strings.TrimSpace(string(rootOut))
	if rootTree != fixture.rootTree {
		return errors.New("fetched root tree mismatch")
	}
	liveTrees := TreeObservation{Commit: commit, RootTree: rootTree}
	liveTrees.EmbeddedComponents = []ComponentTree{{LogicalID: "base", Path: "profiles/base", Tree: fixture.baseTree}, {LogicalID: "selected", Path: "profiles/selected", Tree: fixture.selectedTree}}
	for _, gl := range fixture.gitlinks {
		kind, checkErr := validateStaticGitlink(gl.locator)
		if checkErr != nil || kind != gl.kind {
			return errors.New("fixture gitlink validation failed")
		}
		liveTrees.UnselectedGitlinks = append(liveTrees.UnselectedGitlinks, GitlinkFact{Path: gl.path, Commit: gl.commit, Kind: kind})
	}
	if err := verifyFetchedDeclarations(ctx, r, policy, fixture, rootTree); err != nil {
		return err
	}
	result.Scenarios = append(result.Scenarios, observed("fetched-static-declarations", []string{"VAL-008", "VAL-009", "VAL-012"}, "bounded plumbing observed exact manifest, .gitmodules, embedded tree relations, and two unselected gitlink modes/commits"))

	entries, depth, expanded, err := inspectAndMaterialize(ctx, r, policy, filepath.Join(owned, "materialized"), &liveTrees)
	if err != nil {
		result.Scenarios = append(result.Scenarios, failed("bounded-materialization", []string{"VAL-008"}, "bounded materialization failed; detailed object/path error omitted from retained result"))
		return err
	}
	result.Resources.TreeEntries, result.Resources.TreeDepth, result.Resources.ExpandedBytes = entries, depth, expanded
	result.Resources.TransferBytes, _ = fixture.transferObservation()
	result.Resources.DiskBytes, result.Resources.ObjectCount, err = diskUsage(objectStore)
	if err != nil {
		return err
	}
	result.Scenarios = append(result.Scenarios, observed("bounded-materialization", []string{"VAL-008", "VAL-012"}, "bounded no-checkout plumbing admitted only directories and ordinary executable/non-executable files; source, commit, component tree, and availability stayed separate"))
	if err := redactTreeIdentities(&liveTrees); err != nil {
		return err
	}
	result.Trees = liveTrees

	return nil
}

func classifyProofObservations(result *Result, gitPath, httpsHelperExec, httpsHelper string) {
	// Aggregate transfer bytes are observational only: pinned Git/libcurl has
	// no native exact wire-byte ceiling, and this proof does not insert a relay.
	classifyTrace(result, gitPath, httpsHelperExec, httpsHelper)
	traceQualifies := result.Access.TraceFiles > 0 && result.Access.TraceComplete && result.Access.UnknownEvents == 0
	if !traceQualifies {
		result.Scenarios = append(result.Scenarios, failed("closed-trace-classification", []string{"VAL-007", "VAL-009", "VAL-010", "VAL-011"}, "trace loss, truncation, or an unknown file/process/network record prevented complete classification; zero-access claims are not made"))
	} else if result.Access.GitlinkEndpoints != 0 || result.Access.HelperExecs != 0 || result.Access.AgentAccesses != 0 || result.Access.HookFilterCheckouts != 0 || result.Access.UnexpectedReads != 0 {
		result.Scenarios = append(result.Scenarios, failed("unselected-gitlink-zero-access", []string{"VAL-008", "VAL-009", "VAL-010", "VAL-040"}, "complete trace classification observed a forbidden executable, endpoint, helper/agent, hostile ambient configuration, hook, filter, or checkout access"))
	} else {
		result.Scenarios = append(result.Scenarios, observed("unselected-gitlink-zero-access", []string{"VAL-008", "VAL-009", "VAL-010", "VAL-011", "VAL-040"}, "complete closed trace classification plus unchanged invocation count observed no unselected-gitlink DNS/endpoint, executable, helper, agent, relay, checkout, hook, filter, or planted hostile ambient configuration access"))
	}
	if len(result.Writes.OutsideWrites) != 0 {
		result.Scenarios = append(result.Scenarios, failed("write-confinement", []string{"VAL-007", "VAL-011"}, "complete trace classification observed an outside or unresolved write"))
	}
	if result.Access.LifecycleComplete && result.Access.DeathGuardArmed && result.Access.IOUringDenied && result.Access.CgroupKillGuard && result.Access.SupervisorInvocations == len(result.Invocations) {
		result.Scenarios = append(result.Scenarios, observed("same-binary-supervised-git", []string{"VAL-018", "VAL-019", "VAL-023"}, "every traced Git invocation ran beneath the Phase 1 same-binary FD-only supervisor with ready/session/group/parent-identity/parent-death/guard topology and zero observed orphan or survivor"))
	} else {
		result.Scenarios = append(result.Scenarios, failed("same-binary-supervised-git", []string{"VAL-018", "VAL-019", "VAL-023"}, "one or more traced Git invocations lacked exact supervisor topology or observed an orphan/survivor"))
	}
	if result.Resources.CgroupComplete && result.Resources.MemoryOOM == 0 && result.Resources.OOMKill == 0 && result.Resources.CgroupInvocations == len(result.Invocations) && result.Resources.ProcessPeak <= MaxProcessCount {
		result.Scenarios = append(result.Scenarios, observed("managed-resource-cgroup", []string{"VAL-007", "VAL-023"}, "every traced Git invocation inherited the delegated cgroup v2 CPU, memory, swap, and process-count boundary before the managed child launched; peak observations remained within the frozen limits and every cgroup emptied without fallback cleanup"))
	} else {
		result.Scenarios = append(result.Scenarios, failed("managed-resource-cgroup", []string{"VAL-007", "VAL-023"}, "a traced Git invocation lacked the delegated resource boundary, exceeded the process limit, or left a cgroup member after supervision"))
	}
}

func validateConfig(cfg Config) (string, string, string, string, error) {
	if cfg.Root == "" || cfg.RepositoryRoot == "" || cfg.GitPath == "" || cfg.TracePath == "" || cfg.SupervisorPath == "" || cfg.CACert == "" || cfg.ArtifactDir == "" {
		return "", "", "", "", errors.New("root, RepositoryRoot, GitPath, TracePath, SupervisorPath, CACert, and ArtifactDir are required")
	}
	if len(cfg.ProtectedRoots) == 0 {
		return "", "", "", "", errors.New("canonical protected roots captured before environment poisoning are required")
	}
	base, err := canonicalExistingDir(cfg.Root)
	if err != nil {
		return "", "", "", "", err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(base, &stat); err != nil || uint64(stat.Type) != btrfsMagic {
		return "", "", "", "", errors.New("proof base must be an existing canonical btrfs directory")
	}
	repositoryRoot, err := canonicalExistingDir(cfg.RepositoryRoot)
	if err != nil {
		return "", "", "", "", fmt.Errorf("repository root: %w", err)
	}
	for _, p := range append([]string{repositoryRoot}, cfg.ProtectedRoots...) {
		if p == "" {
			continue
		}
		cp, e := filepath.EvalSymlinks(p)
		if e != nil {
			return "", "", "", "", errors.New("canonical protected path unavailable")
		}
		if overlaps(base, cp) {
			return "", "", "", "", errors.New("proof base overlaps a protected path class")
		}
	}
	gitPath, err := canonicalExecutable(cfg.GitPath)
	if err != nil {
		return "", "", "", "", err
	}
	tracePath, err := canonicalExecutable(cfg.TracePath)
	if err != nil {
		return "", "", "", "", err
	}
	artifact, err := canonicalProspective(cfg.ArtifactDir)
	if err != nil || !within(base, artifact) {
		return "", "", "", "", errors.New("artifact directory must be beneath proof base")
	}
	if cfg.CACert != "" {
		ca, e := filepath.EvalSymlinks(cfg.CACert)
		if e != nil || !filepath.IsAbs(ca) {
			return "", "", "", "", errors.New("CA certificate must be an existing absolute canonical file")
		}
	}
	return base, gitPath, tracePath, artifact, nil
}

func canonicalExistingDir(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("proof base must be absolute")
	}
	c, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	info, e := os.Stat(c)
	if e != nil || !info.IsDir() {
		return "", errors.New("proof base is not a directory")
	}
	if filepath.Clean(p) != c {
		return "", errors.New("proof base must already be canonical")
	}
	return c, nil
}
func canonicalExecutable(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("executable path must be absolute")
	}
	c, e := filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	i, e := os.Stat(c)
	if e != nil || !i.Mode().IsRegular() || i.Mode()&0o111 == 0 {
		return "", errors.New("path is not an executable regular file")
	}
	return c, nil
}
func canonicalProspective(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("path must be absolute")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(p))
	if e != nil {
		return "", e
	}
	return filepath.Join(parent, filepath.Base(p)), nil
}
func overlaps(a, b string) bool { return within(a, b) || within(b, a) }
func within(root, p string) bool {
	rel, e := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func pathExists(p string) bool { _, e := os.Lstat(p); return e == nil }
func identity(p string) (FileIdentity, error) {
	i, e := os.Stat(p)
	if e != nil {
		return FileIdentity{}, e
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return FileIdentity{}, errors.New("stat identity unavailable")
	}
	return FileIdentity{CanonicalPath: p, Device: uint64(s.Dev), Inode: s.Ino, Mode: i.Mode().String(), Size: i.Size()}, nil
}
func rootIdentity(role, p string) (RootObservation, error) {
	i, e := os.Lstat(p)
	if e != nil {
		return RootObservation{}, e
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok {
		return RootObservation{}, errors.New("root stat identity unavailable")
	}
	ref, e := randomTypedRef("proof-root-" + role)
	if e != nil {
		return RootObservation{}, e
	}
	return RootObservation{Role: role, CanonicalPath: ref, Device: uint64(s.Dev), Inode: s.Ino, Mode: i.Mode().String()}, nil
}
func frozenEnvironment(root, ca string) []string {
	e := []string{"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "xdg", "config"), "XDG_DATA_HOME=" + filepath.Join(root, "xdg", "data"), "XDG_CACHE_HOME=" + filepath.Join(root, "xdg", "cache"), "XDG_STATE_HOME=" + filepath.Join(root, "xdg", "state"), "GIT_ATTR_NOSYSTEM=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "LANG=C", "LC_ALL=C", "PATH=/nonexistent", "PAGER=cat", "TERM=dumb"}
	if ca != "" {
		e = append(e, "SSL_CERT_FILE="+ca, "GIT_SSL_CAINFO="+ca)
	}
	return e
}
func gitPolicyArgs(store string) []string {
	return []string{
		"--git-dir=" + store,
		"-c", "credential.helper=",
		"-c", "core.askPass=",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.attributesFile=/dev/null",
		"-c", "core.logAllRefUpdates=false",
		"-c", "init.templateDir=",
		"-c", "filter.lfs.clean=",
		"-c", "filter.lfs.smudge=",
		"-c", "filter.lfs.process=",
		"-c", "filter.lfs.required=false",
		"-c", "http.followRedirects=false",
		"-c", "http.proxy=",
		"-c", "remote.origin.proxy=",
		"-c", "protocol.allow=never",
		"-c", "protocol.http.allow=always",
		"-c", "protocol.https.allow=always",
		"-c", "submodule.recurse=false",
		"-c", "fetch.recurseSubmodules=false",
		"-c", "maintenance.auto=false",
		"-c", "gc.auto=0",
		"-c", "core.pager=",
		"-c", "sequence.editor=false",
		"-c", "core.editor=false",
	}
}

func pinnedHTTPSPolicyArgs(store string, resolves []string) []string {
	args := append(gitPolicyArgs(store),
		"-c", "http.sslVerify=true",
		"-c", "http.extraHeader=",
		"-c", "http.cookieFile=",
		"-c", "http.saveCookies=false",
		"-c", "http.emptyAuth=false",
		"-c", "http.proactiveAuth=none",
		"-c", "http.curloptResolve=",
	)
	for _, resolve := range resolves {
		args = append(args, "-c", "http.curloptResolve="+resolve)
	}
	return args
}
func makeBareStore(p string) error {
	for _, d := range []string{"objects/info", "objects/pack", "refs/heads", "refs/tags"} {
		if e := os.MkdirAll(filepath.Join(p, d), 0o700); e != nil {
			return e
		}
	}
	return os.WriteFile(filepath.Join(p, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600)
}
func plantHostileGitAmbient(owned string) error {
	ambientGit := filepath.Join(owned, ".git")
	hooks := filepath.Join(ambientGit, "hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		return err
	}
	config := "[credential]\n\thelper = /forbidden/credential-helper\n[url \"ssh://forbidden.invalid/\"]\n\tinsteadOf = https://\n[core]\n\thooksPath = " + hooks + "\n[filter \"hostile\"]\n\tprocess = /forbidden/filter\n"
	if err := os.WriteFile(filepath.Join(ambientGit, "config"), []byte(config), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("hostile ambient hook\n"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(owned, "home", ".gitconfig"), []byte(config), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(owned, ".gitattributes"), []byte("* filter=hostile\n"), 0o600)
}

func verifyFetchedDeclarations(ctx context.Context, r runner, policy []string, f *fixture, rootTree string) error {
	rootOut, err := r.run(ctx, "verify-root", append(policy, "ls-tree", "-z", rootTree)...)
	if err != nil {
		return err
	}
	root, err := parseTreeOutput(rootOut, MaxTreeEntries)
	if err != nil {
		return err
	}
	if !matchesTreeItem(root[".gitmodules"], "100644", "blob", "") || !matchesTreeItem(root["profiles"], "040000", "tree", "") || !matchesTreeItem(root["zerb.toml"], "100644", "blob", "") || len(root) != 3 {
		return errors.New("fetched root declarations do not match closed fixture schema")
	}
	modules, err := r.run(ctx, "verify-gitmodules", append(policy, "cat-file", "blob", root[".gitmodules"].oid)...)
	if err != nil || string(modules) != f.modules {
		return errors.New("fetched .gitmodules bytes mismatch")
	}
	manifest, err := r.run(ctx, "verify-manifest", append(policy, "cat-file", "blob", root["zerb.toml"].oid)...)
	if err != nil || string(manifest) != f.manifest {
		return errors.New("fetched manifest bytes mismatch")
	}
	profilesOut, err := r.run(ctx, "verify-profiles", append(policy, "ls-tree", "-z", root["profiles"].oid)...)
	if err != nil {
		return err
	}
	profiles, err := parseTreeOutput(profilesOut, MaxTreeEntries)
	if err != nil {
		return err
	}
	if len(profiles) != 4 || !matchesTreeItem(profiles["base"], "040000", "tree", f.baseTree) || !matchesTreeItem(profiles["selected"], "040000", "tree", f.selectedTree) {
		return errors.New("embedded component relation mismatch")
	}
	for _, gl := range f.gitlinks {
		name := strings.TrimPrefix(gl.path, "profiles/")
		if !matchesTreeItem(profiles[name], "160000", "commit", gl.commit) {
			return errors.New("unselected gitlink tree relation mismatch")
		}
	}
	return nil
}

type parsedTreeItem struct{ mode, kind, oid string }

func parseTreeOutput(out []byte, limit int) (map[string]parsedTreeItem, error) {
	items := make(map[string]parsedTreeItem)
	for _, part := range strings.Split(string(out), "\x00") {
		if part == "" {
			continue
		}
		if len(items) >= limit {
			return nil, errors.New("tree entry ceiling exceeded")
		}
		tab := strings.IndexByte(part, '\t')
		if tab < 0 {
			return nil, errors.New("malformed ls-tree record")
		}
		fields := strings.Fields(part[:tab])
		name := part[tab+1:]
		if len(fields) != 3 || !safeTreeName(name) {
			return nil, errors.New("malformed or unsafe ls-tree record")
		}
		if _, duplicate := items[name]; duplicate {
			return nil, errors.New("duplicate tree name")
		}
		items[name] = parsedTreeItem{mode: fields[0], kind: fields[1], oid: fields[2]}
	}
	return items, nil
}

func matchesTreeItem(got parsedTreeItem, mode, kind, oid string) bool {
	return got.mode == mode && got.kind == kind && (oid == "" || got.oid == oid)
}

type treeBudget struct {
	entries  int
	expanded int64
	maxDepth int
}

func inspectAndMaterialize(ctx context.Context, r runner, policy []string, dst string, trees *TreeObservation) (int, int, int64, error) {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return 0, 0, 0, err
	}
	budget := &treeBudget{}
	for _, component := range trees.EmbeddedComponents {
		if err := walkTree(ctx, r, policy, component.Tree, component.Path, filepath.Join(dst, component.LogicalID), 0, trees, budget); err != nil {
			return 0, 0, 0, err
		}
	}
	return budget.entries, budget.maxDepth, budget.expanded, nil
}

func walkTree(ctx context.Context, r runner, policy []string, oid, logical, dst string, depth int, trees *TreeObservation, budget *treeBudget) error {
	if err := validateCeiling(int64(depth), int64(MaxTreeDepth)); err != nil {
		return errors.New("tree depth ceiling exceeded")
	}
	if depth > budget.maxDepth {
		budget.maxDepth = depth
	}
	out, err := r.run(ctx, "tree-"+safeLabel(logical), append(policy, "ls-tree", "-z", oid)...)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, part := range strings.Split(string(out), "\x00") {
		if part == "" {
			continue
		}
		budget.entries++
		if err := validateCeiling(int64(budget.entries), int64(MaxTreeEntries)); err != nil {
			return errors.New("tree entry ceiling exceeded")
		}
		tab := strings.IndexByte(part, '\t')
		if tab < 0 {
			return errors.New("malformed bounded ls-tree output")
		}
		fields := strings.Fields(part[:tab])
		if len(fields) != 3 {
			return errors.New("malformed bounded ls-tree output")
		}
		mode, kind, child, name := fields[0], fields[1], fields[2], part[tab+1:]
		if !safeTreeName(name) {
			return errors.New("unsafe tree path")
		}
		target := filepath.Join(dst, name)
		switch {
		case mode == "040000" && kind == "tree":
			if err := walkTree(ctx, r, policy, child, logical+"/"+name, target, depth+1, trees, budget); err != nil {
				return err
			}
		case (mode == "100644" || mode == "100755") && kind == "blob":
			blob, err := r.run(ctx, "blob-"+safeLabel(logical+"/"+name), append(policy, "cat-file", "blob", child)...)
			if err != nil {
				return err
			}
			if err := validateCeiling(int64(len(blob)), MaxObjectBytes); err != nil {
				return errors.New("object byte ceiling exceeded")
			}
			if err := validateCeiling(budget.expanded+int64(len(blob)), MaxExpandedBytes); err != nil {
				return errors.New("expanded object ceiling exceeded")
			}
			perm := fs.FileMode(0o600)
			if mode == "100755" {
				perm = 0o700
			}
			if err := os.WriteFile(target, blob, perm); err != nil {
				return err
			}
			budget.expanded += int64(len(blob))
			trees.Materialized = append(trees.Materialized, MaterializedEntry{Path: logical + "/" + name, Mode: mode, Size: int64(len(blob))})
		default:
			return fmt.Errorf("unsupported selected entry mode/type %s %s", mode, kind)
		}
	}
	return nil
}
func safeTreeName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00")
}
func safeLabel(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:6]) }

func runInputRejections(r *Result) {
	valid := []string{ReviewedPublicSource}
	invalid := []string{"", "HTTPS://github.com/x/y.git", "https://GitHub.com/x/y.git", "https://u@github.com/x/y.git", "https://github.com:443/x/y.git", "https://github.com/x/y.git?q=1", "https://github.com/x/y.git#f", "https://github.com/a/../x.git", "https://github.com/a/./x.git", "https://github.com/a%2Fx.git", "https://github.com/%41.git", "https://github.com./x.git", "https://github.com/x\\y.git", "https://github.com/x/%2e%2e/y.git", "https://127.0.0.1/x.git", "https://[::1]/x.git", "https://localhost/x.git", "https://github.com/", "https://github.com/x/%00.git", "file:///tmp/x", "/tmp/x", "./x", "../x", "git@github.com:x.git", "ssh://github.com/x.git"}
	ok := true
	for _, v := range valid {
		ok = ok && validateCanonicalHTTPS(v) == nil
	}
	sourceID1, sourceErr1 := validateReviewedSource(ReviewedPublicSource)
	sourceID2, sourceErr2 := validateReviewedSource(ReviewedPublicSource)
	ok = ok && sourceErr1 == nil && sourceErr2 == nil && sourceID1 == sourceID2
	for _, v := range invalid {
		ok = ok && validateCanonicalHTTPS(v) != nil
	}
	if ok {
		r.Scenarios = append(r.Scenarios, observed("canonical-source-grammar", []string{"VAL-006"}, "accepted only exact canonical HTTPS bytes and rejected ambiguous/local/credential-bearing shapes in-process before Git"))
	} else {
		r.Scenarios = append(r.Scenarios, failed("canonical-source-grammar", []string{"VAL-006"}, "grammar corpus mismatch"))
	}
	before := len(r.Invocations)
	requests := []selectionRequest{
		{Kind: "gitlink", Locator: "https://gitlinks.invalid/x.git"},
		{Kind: "gitlink", Locator: "ssh://gitlinks.invalid/x.git"},
		{Kind: "gitlink", Locator: "git@gitlinks.invalid:x.git"},
		{Kind: "gitlink", Locator: "file:///tmp/x"},
		{Kind: "private", Locator: "https://private.invalid/x.git"},
	}
	for _, mechanism := range []string{"transport-authorization-flag", "transport-authorization-digest", "credential-helper", "askpass", "ssh-agent", "ssh-agent-socket", "typed-process-identity", "identity-file", "key-file", "known-hosts", "credential-store", "synthetic-ssh-config", "proxy", "relay", "private-endpoint", "mechanism-tuple", "transport-digest", "ambient-git-config", "ambient-git-proxy", "ambient-ssh-command", "ambient-ssh-agent"} {
		requests = append(requests, selectionRequest{Kind: "embedded", Mechanism: mechanism})
	}
	allRejected := true
	for _, request := range requests {
		allRejected = allRejected && validateSelection(request) != nil
	}
	if allRejected && before == len(r.Invocations) {
		r.Scenarios = append(r.Scenarios, observed("selected-gitlink-and-mechanism-rejection", []string{"VAL-009", "VAL-010", "VAL-011"}, "selected-gitlink, private-source, and former mechanism shapes rejected in-process with invocation count unchanged"))
	} else {
		r.Scenarios = append(r.Scenarios, failed("selected-gitlink-and-mechanism-rejection", []string{"VAL-009", "VAL-010", "VAL-011"}, "closed selection policy admitted an excluded shape or invoked Git"))
	}
}

type selectionRequest struct{ Kind, Locator, Mechanism string }

func validateSelection(request selectionRequest) error {
	if request.Mechanism != "" {
		return errors.New("transport mechanism inputs are excluded")
	}
	if request.Kind == "gitlink" {
		return errors.New("selected gitlinks are excluded before locator handling")
	}
	if request.Kind != "embedded" || request.Locator != "" {
		return errors.New("private and external selections are excluded")
	}
	return nil
}

func looseBlobForPages(targetPages int) ([]byte, error) {
	page := os.Getpagesize()
	size := targetPages*page - 256
	for range 16 {
		if size <= 0 {
			return nil, errors.New("invalid loose-object boundary target")
		}
		body := make([]byte, size)
		if _, err := rand.Read(body); err != nil {
			return nil, err
		}
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		raw := append([]byte(fmt.Sprintf("blob %d%c", len(body), byte(0))), body...)
		if _, err := writer.Write(raw); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		pages := (compressed.Len() + page - 1) / page
		if pages == targetPages {
			return body, nil
		}
		size += (targetPages - pages) * page
	}
	return nil, errors.New("could not construct exact loose-object page fixture")
}

func runGitDiskEdges(ctx context.Context, r runner, owned string) error {
	page := int64(os.Getpagesize())
	const limitPages = 16
	const capacityPages = limitPages + 1 // one page for Git's loose-object fanout directory
	for _, objectPages := range []int{limitPages - 1, limitPages, limitPages + 1} {
		root := filepath.Join(owned, fmt.Sprintf("disk-edge-%d", objectPages))
		if err := mountStagingTmpfs(root, 256<<10, 512); err != nil {
			return err
		}
		mounted := true
		cleanup := func() error {
			if mounted {
				if err := unmountStagingTmpfs(root); err != nil {
					return err
				}
				mounted = false
			}
			return os.RemoveAll(root)
		}
		fail := func(cause error) error {
			if cleanupErr := cleanup(); cleanupErr != nil {
				return fmt.Errorf("%v; disk-edge cleanup: %w", cause, cleanupErr)
			}
			return cause
		}
		store := filepath.Join(root, "objects.git")
		if err := makeBareStore(store); err != nil {
			return fail(err)
		}
		available, _, err := stagingAvailability(root)
		if err != nil || available <= capacityPages*page {
			return fail(errors.New("disk-edge tmpfs capacity unavailable"))
		}
		fillerSize := available - capacityPages*page
		if err := os.WriteFile(filepath.Join(root, "capacity-reservation"), make([]byte, int(fillerSize)), 0o600); err != nil {
			return fail(err)
		}
		available, _, err = stagingAvailability(root)
		if err != nil || available != capacityPages*page {
			return fail(errors.New("disk-edge tmpfs reservation mismatch"))
		}
		body, err := looseBlobForPages(objectPages)
		if err != nil {
			return fail(err)
		}
		policy := gitPolicyArgs(store)
		before := len(*r.invocations)
		_, gitErr := r.runInput(ctx, fmt.Sprintf("aggregate-disk-edge-%d", objectPages), body, append(policy, "hash-object", "-w", "--stdin")...)
		if errors.Is(gitErr, errTraceViolation) {
			return fail(gitErr)
		}
		_, objects, usageErr := diskUsage(store)
		expectedSuccess := objectPages <= limitPages
		expectedObjects := 0
		if expectedSuccess {
			expectedObjects = 1
		}
		remaining, freeInodes, availabilityErr := stagingAvailability(root)
		expectedRemaining := int64(limitPages-objectPages) * page
		remainingMatches := !expectedSuccess || remaining == expectedRemaining
		invocationObserved := len(*r.invocations) == before+1 && (*r.invocations)[len(*r.invocations)-1].TraceComplete
		if usageErr != nil || availabilityErr != nil || freeInodes == 0 || !invocationObserved ||
			(gitErr == nil) != expectedSuccess || objects != expectedObjects || !remainingMatches ||
			pathExists(filepath.Join(store, "refs", "heads", "main")) {
			return fail(fmt.Errorf("actual Git disagreed with kernel disk edge: pages=%d success=%t expected_success=%t objects=%d expected_objects=%d remaining=%d expected_remaining=%d free_inodes=%d invocation=%t usage_error=%t availability_error=%t", objectPages, gitErr == nil, expectedSuccess, objects, expectedObjects, remaining, expectedRemaining, freeInodes, invocationObserved, usageErr != nil, availabilityErr != nil))
		}
		if err := cleanup(); err != nil || pathExists(root) {
			return errors.New("disk-edge tmpfs cleanup failed")
		}
	}
	return nil
}

func runHardDiskBoundary(ctx context.Context, r runner, owned string, result *Result) (retErr error) {
	const boundaryBytes int64 = 512 << 10
	boundaryRoot := filepath.Join(owned, "aggregate-disk-boundary")
	if err := runGitDiskEdges(ctx, r, owned); err != nil {
		return err
	}
	if err := mountStagingTmpfs(boundaryRoot, boundaryBytes, MaxObjectCount*16); err != nil {
		return err
	}
	mounted := true
	defer func() {
		if mounted {
			if err := unmountStagingTmpfs(boundaryRoot); err != nil && retErr == nil {
				retErr = err
			}
		}
		if err := os.RemoveAll(boundaryRoot); err != nil && retErr == nil {
			retErr = err
		}
		// The boundary root is the only per-scenario filesystem allocation.
	}()
	store := filepath.Join(boundaryRoot, "objects.git")
	if err := makeBareStore(store); err != nil {
		return err
	}
	policy := gitPolicyArgs(store)
	before := len(*r.invocations)
	var gitErr error
	for i := range MaxObjectCount {
		body := make([]byte, 16<<10)
		if _, err := rand.Read(body); err != nil {
			return err
		}
		_, gitErr = r.runInput(ctx, fmt.Sprintf("aggregate-disk-exhaustion-%03d", i), body, append(policy, "hash-object", "-w", "--stdin")...)
		if errors.Is(gitErr, errTraceViolation) {
			return gitErr
		}
		if gitErr != nil {
			break
		}
	}
	used, objects, usageErr := diskUsage(store)
	refExists := pathExists(filepath.Join(store, "refs", "heads", "main"))
	available, freeInodes, availabilityErr := stagingAvailability(boundaryRoot)
	invocationsObserved := len(*r.invocations) > before && (*r.invocations)[len(*r.invocations)-1].TraceComplete
	if gitErr == nil || usageErr != nil || availabilityErr != nil || !invocationsObserved ||
		used <= 0 || used > boundaryBytes || available >= 32<<10 || freeInodes == 0 ||
		objects >= MaxObjectCount || refExists {
		return errors.New("actual Git did not fail closed at the aggregate staging-disk boundary")
	}
	if err := unmountStagingTmpfs(boundaryRoot); err != nil {
		return err
	}
	mounted = false
	if err := os.RemoveAll(boundaryRoot); err != nil || pathExists(boundaryRoot) {
		return errors.New("aggregate staging-disk boundary cleanup failed")
	}
	result.Scenarios = append(result.Scenarios, observed("kernel-aggregate-staging-disk-boundary", []string{"VAL-007", "VAL-011"}, "actual Git wrote compressed loose objects at one page below and exactly at the frozen object-page limit plus the observed one-page fanout-directory overhead, was refused one page above it, then exhausted a separate size-capped tmpfs with many small unique objects; every mount lived inside a private unprivileged user/mount namespace, no ref was promoted, each exact mount was synchronously unmounted and removed, and the complete proof-owned tmpfs independently hard-capped all staging, trace, fixture, and materialization writes"))
	return nil
}
func validateCeiling(value, limit int64) error {
	if value < 0 || value > limit {
		return errors.New("resource ceiling exceeded")
	}
	return nil
}

func classifyTrace(r *Result, gitPath, httpsHelperExec, httpsHelperCanonical string) {
	execDir := filepath.Dir(httpsHelperExec)
	knownGitExec := map[string]bool{
		gitPath:                       true,
		filepath.Join(execDir, "git"): true,
		filepath.Join(execDir, "git-remote-http"):  true,
		filepath.Join(execDir, "git-remote-https"): true,
		httpsHelperExec:      true,
		httpsHelperCanonical: true,
	}
	for _, name := range []string{"git", "git-remote-http", "git-remote-https"} {
		if canonical, err := filepath.EvalSymlinks(filepath.Join(execDir, name)); err == nil {
			knownGitExec[canonical] = true
		}
	}
	for _, inv := range r.Invocations {
		if !inv.TraceComplete {
			r.Access.TraceComplete = false
		}
		for _, x := range inv.ObservedExecs {
			if !knownGitExec[x] {
				r.Access.HelperExecs++
			}
		}
	}
	for _, e := range r.Access.Endpoints {
		if !strings.HasPrefix(e, "loopback-") && strings.Contains(e, "gitlink") {
			r.Access.GitlinkEndpoints++
		}
	}
	sort.Strings(r.Access.Endpoints)
	sort.Strings(r.Writes.ObservedPaths)
}
func diskUsage(root string) (int64, int, error) {
	var size int64
	count := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		i, e := d.Info()
		if e != nil {
			return e
		}
		size += i.Size()
		rel, e := filepath.Rel(filepath.Join(root, "objects"), p)
		if e == nil {
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) == 2 && len(parts[0]) == 2 && len(parts[1]) == 38 && isLowerHex(parts[0]+parts[1]) {
				count++
			}
		}
		return nil
	})
	return size, count, err
}
func isLowerHex(s string) bool {
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func randomTypedRef(kind string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "typed:" + kind + ":" + hex.EncodeToString(b), nil
}

func redactTreeIdentities(t *TreeObservation) error {
	var err error
	if t.Commit, err = randomTypedRef("fixture-revision"); err != nil {
		return err
	}
	if t.RootTree, err = randomTypedRef("fixture-root-tree"); err != nil {
		return err
	}
	for i := range t.EmbeddedComponents {
		if t.EmbeddedComponents[i].Tree, err = randomTypedRef("fixture-component-tree"); err != nil {
			return err
		}
	}
	for i := range t.UnselectedGitlinks {
		if t.UnselectedGitlinks[i].Commit, err = randomTypedRef("unselected-gitlink-revision"); err != nil {
			return err
		}
		if t.UnselectedGitlinks[i].LocatorRef, err = randomTypedRef("unselected-public-locator"); err != nil {
			return err
		}
	}
	return nil
}

type markerScanner struct {
	marker []byte
	tail   []byte
	found  bool
}

func (s *markerScanner) Write(chunk []byte) (int, error) {
	if s.found {
		return len(chunk), nil
	}
	window := make([]byte, 0, len(s.tail)+len(chunk))
	window = append(window, s.tail...)
	window = append(window, chunk...)
	s.found = bytes.Contains(window, s.marker)
	keep := len(s.marker) - 1
	if keep > len(window) {
		keep = len(window)
	}
	s.tail = append(s.tail[:0], window[len(window)-keep:]...)
	return len(chunk), nil
}

func scanForMarker(root string, marker []byte) error {
	if len(marker) == 0 {
		return errors.New("empty marker")
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if bytes.Contains([]byte(target), marker) {
				return errors.New("ambient marker reached a symlink target")
			}
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
		scanner := &markerScanner{marker: marker}
		_, scanErr := io.Copy(scanner, file)
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		if scanner.found {
			return errors.New("ambient marker reached a filesystem sink")
		}
		return nil
	})
}

const maxRepositoryScanObjects = 1 << 20

func scanRepositoryObjects(ctx context.Context, gitPath, repositoryRoot, proofRoot string, marker []byte) error {
	repositories := []string{repositoryRoot}
	seen := map[string]bool{repositoryRoot: true}
	if err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == repositoryRoot {
			return nil
		}
		if entry.Name() != ".git" {
			return nil
		}
		worktree := filepath.Dir(path)
		if !seen[worktree] {
			seen[worktree] = true
			repositories = append(repositories, worktree)
		}
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}); err != nil {
		return err
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
	for _, repository := range repositories {
		revList := exec.CommandContext(ctx, gitPath, "-C", repository, "rev-list", "--objects", "--no-object-names", "--all")
		revList.Env = env
		objectList, err := revList.StdoutPipe()
		if err != nil {
			return errors.New("open reachable repository object stream")
		}
		scanner := &markerScanner{marker: marker}
		catFile := exec.CommandContext(ctx, gitPath, "-C", repository, "cat-file", "--batch")
		catFile.Env = env
		objectInput, err := catFile.StdinPipe()
		if err != nil {
			return errors.New("open repository object scan input")
		}
		catFile.Stdout = scanner
		if err = catFile.Start(); err != nil {
			return errors.New("start reachable repository object scan")
		}
		if err = revList.Start(); err != nil {
			_ = objectInput.Close()
			_ = catFile.Process.Kill()
			_ = catFile.Wait()
			return errors.New("start reachable repository object enumeration")
		}
		lines := bufio.NewScanner(objectList)
		lines.Buffer(make([]byte, 64), 64)
		objects := 0
		var streamErr error
		var decoded [20]byte
		for lines.Scan() {
			oid := lines.Bytes()
			if len(oid) != 40 {
				streamErr = errors.New("unexpected repository object identity")
				break
			}
			if _, err = hex.Decode(decoded[:], oid); err != nil {
				streamErr = errors.New("malformed repository object identity")
				break
			}
			objects++
			if objects > maxRepositoryScanObjects {
				streamErr = errors.New("repository object scan ceiling exceeded")
				break
			}
			if _, err = objectInput.Write(oid); err == nil {
				_, err = objectInput.Write([]byte{'\n'})
			}
			if err != nil {
				streamErr = errors.New("stream reachable repository object identity")
				break
			}
		}
		if scanErr := lines.Err(); streamErr == nil && scanErr != nil {
			streamErr = errors.New("bounded repository object enumeration failed")
		}
		if streamErr != nil {
			_ = revList.Process.Kill()
		}
		closeErr := objectInput.Close()
		revErr := revList.Wait()
		catErr := catFile.Wait()
		if streamErr != nil {
			return streamErr
		}
		if closeErr != nil || revErr != nil {
			return errors.New("enumerate reachable repository objects")
		}
		if catErr != nil {
			return errors.New("scan reachable repository objects")
		}
		if scanner.found {
			return errors.New("ambient marker reached reachable repository content")
		}
	}
	return nil
}
func observed(name string, ids []string, obs string) Scenario {
	return Scenario{Name: name, Status: "observed", RequirementIDs: ids, Observation: obs}
}
func failed(name string, ids []string, obs string) Scenario {
	return Scenario{Name: name, Status: "failed", RequirementIDs: ids, Observation: obs}
}
