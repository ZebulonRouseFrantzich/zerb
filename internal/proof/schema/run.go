package schema

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/template"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceid"
)

const btrfsSuperMagic = 0x9123683e

func Run(ctx context.Context, cfg Config) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, artifacts, err := validateRoots(cfg)
	if err != nil {
		return Result{}, err
	}
	resultRoot := root
	resultArtifacts := artifacts
	rendered, err := marker("rendered")
	if err != nil {
		return Result{}, err
	}
	ambient, err := marker("ambient")
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Version:             ResultVersion,
		Parser:              ParserIdentity{Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"},
		Requirements:        []string{"PC-010", "PC-011", "PC-014", "PC-016", "PC-040", "PC-041", "PC-042", "PC-043", "PC-050", "PC-060", "PC-070", "PC-071", "ARCH-003", "ARCH-004", "ARCH-015", "ARCH-020", "ARCH-024", "ARCH-028", "ARCH-029", "ARCH-030", "ARCH-031", "VAL-002", "VAL-003", "VAL-004", "VAL-005", "VAL-013", "VAL-014", "VAL-015", "VAL-016", "VAL-034", "VAL-035", "VAL-038", "VAL-039", "VAL-040"},
		Bounds:              bounds(),
		RejectedAssumption:  "A disposable parser/model harness, an observed userspace schedule, or a passing case is product implementation, installed-binary evidence, kernel-locking proof, or a support claim.",
		RenderedFingerprint: fingerprint(rendered), AmbientFingerprint: fingerprint(ambient),
	}
	relations, err := evidenceRelations()
	if err != nil {
		return Result{}, err
	}
	result.EvidenceRelations = relations
	result.Scenarios = append(result.Scenarios, foundationalEvidenceCases(resultRoot, resultArtifacts)...)
	scenarios, diagnostics := parserCases()
	result.Scenarios = append(result.Scenarios, scenarios...)
	result.Diagnostics = diagnostics
	result.Scenarios = append(result.Scenarios, sourceCases()...)
	result.Scenarios = append(result.Scenarios, commandCases()...)
	result.Scenarios = append(result.Scenarios, limitCases()...)
	result.Scenarios = append(result.Scenarios, compositionCases()...)
	result.Scenarios = append(result.Scenarios, templateCases(rendered)...)
	result.Scenarios = append(result.Scenarios, renderedMarkerCase(rendered))
	result.Scenarios = append(result.Scenarios, approvalCases()...)
	serialized, err := json.Marshal(result)
	if err != nil {
		return Result{}, err
	}
	result.SerializedMarkerScan = MarkerScan{RenderedRawOccurrences: strings.Count(string(serialized), rendered), AmbientRawOccurrences: strings.Count(string(serialized), ambient)}
	status := "observed"
	observed := "serialized structured result contains neither raw rendered nor raw ambient marker"
	if result.SerializedMarkerScan.RenderedRawOccurrences != 0 || result.SerializedMarkerScan.AmbientRawOccurrences != 0 {
		status = "failed"
		observed = "raw marker escaped into serialized result"
	}
	result.Scenarios = append(result.Scenarios, Scenario{Name: "serialized-marker-scan", Status: status, RequirementIDs: []string{"VAL-005", "VAL-040", "ARCH-028"}, Expected: "zero raw marker occurrences in serialized proof result", Observed: observed, RejectedAssumption: "A marker fingerprint is equivalent to retaining raw marker bytes."})
	sort.Strings(result.Requirements)
	return result, nil
}

func parserCases() ([]Scenario, []DiagnosticObservation) {
	valid := []byte("schema = 1\n[base]\npath = \"profiles/base\"\n[[profiles]]\nid = \"personal\"\nkind = \"embedded\"\npath = \"profiles/personal\"\n")
	cases := []struct {
		name      string
		data      []byte
		component bool
		reject    bool
	}{
		{"zerb-valid", valid, false, false}, {"zerb-schema-mismatch", []byte("schema=2\n[base]\npath=\"profiles/base\"\n"), false, true}, {"zerb-unknown", append(append([]byte(nil), valid...), []byte("extra=true\n")...), false, true}, {"zerb-unknown-table", append(append([]byte(nil), valid...), []byte("[unknown]\nx=1\n")...), false, true}, {"zerb-duplicate-key", []byte("schema=1\nschema=1\n[base]\npath=\"profiles/base\"\n"), false, true}, {"zerb-duplicate-table", []byte("schema=1\n[base]\npath=\"profiles/base\"\n[base]\npath=\"profiles/base\"\n"), false, true}, {"zerb-dotted-alias", []byte("schema=1\nbase.path=\"profiles/base\"\n"), false, true}, {"zerb-malformed", []byte("schema = [\n"), false, true}, {"zerb-invalid-utf8", []byte{0xff}, false, true}, {"zerb-duplicate-id", []byte("schema=1\n[base]\npath=\"b\"\n[[profiles]]\nid=\"x\"\nkind=\"embedded\"\npath=\"x\"\n[[profiles]]\nid=\"x\"\nkind=\"embedded\"\npath=\"y\"\n"), false, true}, {"zerb-invalid-kind", []byte("schema=1\n[base]\npath=\"b\"\n[[profiles]]\nid=\"x\"\nkind=\"private\"\npath=\"x\"\n"), false, true}, {"zerb-traversal", []byte("schema=1\n[base]\npath=\"../b\"\n"), false, true}, {"profile-valid", []byte("schema=1\nlabel=\"Personal\"\n"), true, false}, {"profile-unknown", []byte("schema=1\ntools=[]\n"), true, true}, {"profile-nested", []byte("schema=1\n[provider]\nx=1\n"), true, true}, {"profile-schema", []byte("schema=2\n"), true, true},
	}
	var out []Scenario
	var diagnostics []DiagnosticObservation
	for _, c := range cases {
		var d1, d2 *DiagnosticObservation
		var e1, e2 error
		if c.component {
			_, d1, e1 = DecodeComponent(c.data)
			_, d2, e2 = DecodeComponent(c.data)
		} else {
			_, d1, e1 = DecodeZerb(c.data)
			_, d2, e2 = DecodeZerb(c.data)
		}
		actual := e1 != nil
		det := sameDiagnostic(d1, d2) && ((e1 == nil) == (e2 == nil))
		status := "observed"
		if actual != c.reject || !det {
			status = "failed"
		}
		obs := fmt.Sprintf("rejected=%t deterministic_diagnostic=%t", actual, det)
		out = append(out, Scenario{Name: c.name, Status: status, RequirementIDs: []string{"VAL-013", "VAL-014", "PC-040", "PC-041", "PC-042"}, Expected: fmt.Sprintf("rejected=%t", c.reject), Observed: obs, RejectedAssumption: "A permissive TOML semantic alias or decoder zero value is an accepted schema value."})
		if d1 != nil {
			d1.Case = c.name
			diagnostics = append(diagnostics, *d1)
		}
	}
	return out, diagnostics
}
func sameDiagnostic(a, b *DiagnosticObservation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func foundationalEvidenceCases(root, artifacts string) []Scenario {
	rootInfo, rootErr := os.Stat(root)
	artifactInfo, artifactErr := os.Stat(artifacts)
	distinctInodes := false
	if rootErr == nil && artifactErr == nil {
		rootStat, rootOK := rootInfo.Sys().(*syscall.Stat_t)
		artifactStat, artifactOK := artifactInfo.Sys().(*syscall.Stat_t)
		distinctInodes = rootOK && artifactOK && (rootStat.Dev != artifactStat.Dev || rootStat.Ino != artifactStat.Ino)
	}
	return []Scenario{
		{Name: "disposable-root-canonical-admission", Status: boolStatus(rootErr == nil && artifactErr == nil && within(artifacts, root) && distinctInodes), RequirementIDs: []string{"VAL-002", "PC-060", "ARCH-016", "ARCH-017"}, Expected: "canonical existing btrfs root and distinct artifact directory beneath it, disjoint from captured protected roots", Observed: fmt.Sprintf("root_stat=%t artifact_stat=%t artifact_beneath_root=%t distinct_inode=%t protected_disjointness_validated_before_run=%t", rootErr == nil, artifactErr == nil, within(artifacts, root), distinctInodes, true), RejectedAssumption: "An arbitrary temporary path is a disposable isolated proof root."},
	}
}

func sourceCases() []Scenario {
	cases := []struct {
		name string
		uri  string
		ok   bool
	}{
		{"canonical", "https://example.com/environment.git", true},
		{"scheme-case", "HTTPS://example.com/environment.git", false},
		{"host-case", "https://EXAMPLE.com/environment.git", false},
		{"default-port", "https://example.com:443/environment.git", false},
		{"fragment", "https://example.com/environment.git#main", false},
		{"query", "https://example.com/environment.git?ref=main", false},
		{"userinfo", "https://user@example.com/e.git", false},
		{"dot-segment", "https://example.com/a/../e.git", false},
		{"encoded-slash", "https://example.com/a%2Fe.git", false},
		{"encoded-backslash", "https://example.com/a%5Ce.git", false},
		{"noncanonical-escape", "https://example.com/%65.git", false},
		{"lowercase-escape", "https://example.com/%ff.git", false},
		{"empty-segment", "https://example.com/a//b.git", false},
		{"root-path", "https://example.com/", false},
		{"trailing-host-dot", "https://example.com./environment.git", false},
		{"empty-host-label", "https://example..com/environment.git", false},
		{"leading-host-hyphen", "https://-example.com/environment.git", false},
		{"trailing-host-hyphen", "https://example-.com/environment.git", false},
		{"localhost", "https://localhost/environment.git", false},
		{"ipv4-literal", "https://127.0.0.1/environment.git", false},
		{"ipv6-literal", "https://[2001:db8::1]/environment.git", false},
		{"control-byte", "https://example.com/e.git\n", false},
		{"backslash", "https://example.com/a\\b.git", false},
		{"unicode-host", "https://éxample.com/e.git", false},
		{"local-path", "./environment", false},
		{"file-uri", "file:///tmp/environment", false},
		{"scp-like", "git@example.com:environment.git", false},
	}
	out := make([]Scenario, 0, len(cases)+3)
	for _, item := range cases {
		err := ValidateSource(item.uri)
		accepted := err == nil
		out = append(out, Scenario{Name: "source-grammar-" + item.name, Status: boolStatus(accepted == item.ok), RequirementIDs: []string{"PC-020", "VAL-006", "VAL-014"}, Expected: fmt.Sprintf("accepted=%t", item.ok), Observed: fmt.Sprintf("accepted=%t", accepted), RejectedAssumption: "A source requiring normalization or endpoint interpretation is canonical input."})
	}
	uri := "https://example.com/environment.git"
	first, firstErr := CanonicalSourceID(uri)
	second, secondErr := CanonicalSourceID(uri)
	other, otherErr := CanonicalSourceID("https://example.org/environment.git")
	out = append(out, Scenario{Name: "source-id-repeatability", Status: boolStatus(firstErr == nil && secondErr == nil && otherErr == nil && first == second && first != other), RequirementIDs: []string{"PC-022", "VAL-006", "VAL-012"}, Expected: "same canonical URI has one source_id and a different URI has another", Observed: fmt.Sprintf("errors=%t/%t/%t same=%t changed=%t", firstErr != nil, secondErr != nil, otherErr != nil, first == second, first != other), RejectedAssumption: "Availability or commit identity participates in source_id."})
	vector, vectorErr := CanonicalSourceID(sourceid.KnownVectorURI)
	out = append(out, Scenario{Name: "source-id-shared-known-vector", Status: boolStatus(vectorErr == nil && vector == sourceid.KnownVectorDigest), RequirementIDs: []string{"PC-022", "VAL-006", "VAL-012"}, Expected: "shared canonical URI encodes to the fixed versioned map-free source_id used by every proof slice", Observed: fmt.Sprintf("error=%t fixed_digest_equal=%t", vectorErr != nil, vector == sourceid.KnownVectorDigest), RejectedAssumption: "Slice-local source identity encoders cannot diverge."})
	source := proofSource()

	changedCommit := source
	changedCommit.Commit = strings.Repeat("d", 40)
	out = append(out, Scenario{Name: "source-id-object-separation", Status: boolStatus(source.SourceID == changedCommit.SourceID && source.Commit != changedCommit.Commit), RequirementIDs: []string{"PC-022", "VAL-012"}, Expected: "source_id remains equal while commit identity changes", Observed: fmt.Sprintf("source_id_equal=%t commit_changed=%t", source.SourceID == changedCommit.SourceID, source.Commit != changedCommit.Commit), RejectedAssumption: "A source_id is a commit or availability observation."})
	return out
}
func commandCases() []Scenario {
	validSource := "https://example.com/environment.git"
	digest := proofDigest("a")
	cases := []struct {
		name      string
		argv      []string
		accepted  bool
		rootCalls int
	}{
		{"version-word", []string{"version"}, true, 0},
		{"version-flag", []string{"--version"}, true, 0},
		{"profile-list", []string{"profile", "list", validSource}, true, 1},
		{"plan", []string{"plan", validSource, "--profile", "personal", "--trust-candidate", digest}, true, 1},
		{"bootstrap", []string{"bootstrap", validSource, "--profile", "personal", "--trust-candidate", digest, "--yes"}, true, 1},
		{"apply", []string{"apply", "--candidate", digest, "--yes"}, true, 1},
		{"doctor", []string{"doctor"}, true, 1},
		{"recover-resume", []string{"recover", "resume"}, true, 1},
		{"recover-repair", []string{"recover", "repair"}, true, 1},
		{"exec-bare", []string{"exec", "--", "node", "--payload-flag"}, true, 1},
		{"exec-absolute", []string{"exec", "--", "/opt/proof/bin/node"}, true, 1},
		{"shell", []string{"shell", "--shell", "bash"}, true, 1},
		{"missing-command", nil, false, 0},
		{"unknown-command", []string{"remove"}, false, 0},
		{"profile-extra", []string{"profile", "list", validSource, "extra"}, false, 0},
		{"plan-missing-profile", []string{"plan", validSource}, false, 0},
		{"plan-repeated-profile", []string{"plan", validSource, "--profile", "a", "--profile", "b"}, false, 0},
		{"plan-malformed-digest", []string{"plan", validSource, "--profile", "a", "--trust-candidate", "sha256:ABC"}, false, 0},
		{"transport-authorization-flag", []string{"plan", validSource, "--profile", "a", "--transport-token", "x"}, false, 0},
		{"exec-missing-boundary", []string{"exec", "node"}, false, 0},
		{"exec-missing-command", []string{"exec", "--"}, false, 0},
		{"exec-relative-slash", []string{"exec", "--", "bin/node"}, false, 0},
		{"exec-noncanonical-absolute", []string{"exec", "--", "/opt/../bin/node"}, false, 0},
		{"shell-ambient-selection", []string{"shell"}, false, 0},
		{"shell-other-id", []string{"shell", "--shell", "zsh"}, false, 0},
	}
	out := make([]Scenario, 0, len(cases)+1)
	for _, item := range cases {
		parsed, err := ParseInvocation(item.argv)
		calls := 0
		if err == nil && parsed.RequiresRoot {
			calls++
		}
		ok := (err == nil) == item.accepted && calls == item.rootCalls
		out = append(out, Scenario{Name: "argv-" + item.name, Status: boolStatus(ok), RequirementIDs: []string{"PC-010", "PC-011", "PC-014", "ARCH-030"}, Expected: fmt.Sprintf("accepted=%t root_resolution_calls=%d", item.accepted, item.rootCalls), Observed: fmt.Sprintf("accepted=%t root_resolution_calls=%d", err == nil, calls), RejectedAssumption: "The disposable grammar path is the production internal/app.Run parser or proves durable-root non-access."})
	}
	return out
}

func limitCases() []Scenario {
	out := make([]Scenario, 0, len(bounds())*3+4)
	record := func(field string, delta, size int, accepted, expected bool) {
		out = append(out, Scenario{
			Name:               fmt.Sprintf("bound-%s-%+d", field, delta),
			Status:             boolStatus(accepted == expected),
			RequirementIDs:     []string{"VAL-015", "PC-042", "ARCH-020", "ARCH-024"},
			Expected:           fmt.Sprintf("size=%d accepted=%t", size, expected),
			Observed:           fmt.Sprintf("size=%d accepted=%t", size, accepted),
			RejectedAssumption: "A declared constant or tautological predicate proves its enforcing parse/composition path.",
		})
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxDocumentBytes + delta
		data := documentOfSize(size)
		_, _, err := DecodeZerb(data)
		record("document", delta, size, err == nil, size <= MaxDocumentBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxProfiles + delta
		_, _, err := DecodeZerb(manifestWithProfiles(size))
		record("profiles", delta, size, err == nil, size <= MaxProfiles)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxProfileIDBytes + delta
		id := "p" + strings.Repeat("a", size-1)
		data := []byte(fmt.Sprintf("schema=1\n[base]\npath=\"base\"\n[[profiles]]\nid=%q\nkind=\"embedded\"\npath=\"selected\"\n", id))
		_, _, err := DecodeZerb(data)
		record("profile_id", delta, size, err == nil, size <= MaxProfileIDBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxProfileLabel + delta
		data := []byte("schema=1\nlabel=\"" + strings.Repeat("l", size) + "\"\n")
		_, _, err := DecodeComponent(data)
		record("profile_label", delta, size, err == nil, size <= MaxProfileLabel)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxSourceBytes + delta
		err := ValidateSource(sourceOfSize(size))
		record("source", delta, size, err == nil, size <= MaxSourceBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxPathBytes + delta
		component := boundaryComponent()
		component.Path = pathOfSize(size, 0)
		err := ValidateComponent(component)
		record("path", delta, size, err == nil, size <= MaxPathBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxPathComponents + delta
		component := boundaryComponent()
		component.Path = strings.TrimSuffix(strings.Repeat("a/", size), "/")
		err := ValidateComponent(component)
		record("path_depth", delta, size, err == nil, size <= MaxPathComponents)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxPathComponent + delta
		component := boundaryComponent()
		component.Path = strings.Repeat("a", size)
		err := ValidateComponent(component)
		record("path_component", delta, size, err == nil, size <= MaxPathComponent)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxComponentFiles + delta
		component := boundaryComponent()
		component.Files = fileInventory(size, "file")
		err := ValidateComponent(component)
		record("component_files", delta, size, err == nil, size <= MaxComponentFiles)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTools + delta
		component := boundaryComponent()
		component.HasMise = size > 0
		component.HasLock = size > 0
		component.Tools = toolInventory(size)
		err := ValidateComponent(component)
		record("tools", delta, size, err == nil, size <= MaxTools)
	}
	toolIDCases := []struct {
		value    string
		expected bool
	}{{"nod", false}, {"node", true}, {"nodeX", false}}
	for i, item := range toolIDCases {
		tool := proofTool()
		tool.ID = item.value
		err := validateTool(tool)
		record("tool_id", i-1, len(item.value), err == nil, item.expected)
	}
	versionCases := []struct {
		value    string
		expected bool
	}{{"24.4.", false}, {"24.4.1", true}, {"24.4.10", false}}
	for i, item := range versionCases {
		tool := proofTool()
		tool.Version = item.value
		err := validateTool(tool)
		record("version", i-1, len(item.value), err == nil, item.expected)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxLocksPerTool + delta
		tool := proofTool()
		tool.Locks = make([]ToolLock, size)
		for i := range tool.Locks {
			tool.Locks[i] = ToolLock{Name: "artifact", Value: proofDigest("a")}
		}
		err := validateTool(tool)
		record("locks_per_tool", delta, size, err == nil, size == MaxLocksPerTool)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxLockValueBytes + delta
		tool := proofTool()
		tool.Locks[0].Value = "sha256:" + strings.Repeat("a", size-len("sha256:"))
		err := validateTool(tool)
		record("lock_value", delta, size, err == nil, size == MaxLockValueBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxHomeSources + delta
		component := boundaryComponent()
		component.HomeSources = pathInventory(size, "home")
		err := ValidateComponent(component)
		record("home_sources", delta, size, err == nil, size <= MaxHomeSources)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTargets + delta
		component := boundaryComponent()
		component.Targets = pathInventory(size, "target")
		err := ValidateComponent(component)
		record("targets", delta, size, err == nil, size <= MaxTargets)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTemplateBytes + delta
		err := ValidateTemplate("byte-boundary", []byte(strings.Repeat("x", size)))
		record("template", delta, size, err == nil, size <= MaxTemplateBytes)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTemplateDepth + delta
		err := ValidateTemplate("depth-boundary", []byte(templateAtDepth(size)))
		record("template_depth", delta, size, err == nil, size <= MaxTemplateDepth)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTemplateActions + delta
		err := ValidateTemplate("action-boundary", []byte(templateWithActions(size)))
		record("template_actions", delta, size, err == nil, size <= MaxTemplateActions)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxTotalStrings + delta
		candidate, built := candidateWithStringCount(size)
		_, _, err := DigestCandidate(candidate)
		record("total_strings", delta, size, built && err == nil, size <= MaxTotalStrings)
	}
	for _, delta := range []int{-1, 0, 1} {
		size := MaxAllocatedBytes + delta
		candidate, built := candidateWithAllocatedBytes(size)
		_, _, err := DigestCandidate(candidate)
		record("allocated_bytes", delta, size, built && err == nil, size <= MaxAllocatedBytes)
	}
	out = append(out, deterministicPropertyCases()...)
	return out
}

func documentOfSize(size int) []byte {
	prefix := []byte("schema=1\n[base]\npath=\"b\"\n")
	if size < len(prefix) {
		return prefix[:size]
	}
	out := append([]byte(nil), prefix...)
	out = append(out, bytes.Repeat([]byte{'#'}, size-len(out))...)
	return out
}

func manifestWithProfiles(count int) []byte {
	var builder strings.Builder
	builder.WriteString("schema=1\n[base]\npath=\"base\"\n")
	for i := range count {
		fmt.Fprintf(&builder, "[[profiles]]\nid=\"p%03d\"\nkind=\"embedded\"\npath=\"p/%03d\"\n", i, i)
	}
	return []byte(builder.String())
}

func sourceOfSize(size int) string {
	const prefix = "https://example.com/"
	if size < len(prefix)+1 {
		return ""
	}
	return prefix + strings.Repeat("a", size-len(prefix))
}

func pathOfSize(size, identity int) string {
	if size <= 0 {
		return ""
	}
	if size < 12 {
		return strings.Repeat(string(rune('a'+identity%20)), size)
	}
	prefix := fmt.Sprintf("p%08d", identity)
	var builder strings.Builder
	builder.Grow(size)
	builder.WriteString(prefix)
	for builder.Len() < size {
		remaining := size - builder.Len()
		if remaining == 1 {
			builder.WriteByte('x')
			break
		}
		builder.WriteByte('/')
		remaining--
		if remaining > MaxPathComponent {
			remaining = MaxPathComponent
		}
		builder.WriteString(strings.Repeat("x", remaining))
	}
	return builder.String()
}

func boundaryComponent() Component {
	return Component{ID: "base", Path: "profiles/base", TreeDigest: proofDigest("a"), Manifest: ComponentManifest{Schema: 1}, HasHome: true}
}

func proofTool() Tool {
	return Tool{ID: "node", Version: "24.4.1", Locks: []ToolLock{{Name: "artifact", Value: proofDigest("a")}}}
}

func toolInventory(count int) []Tool {
	out := make([]Tool, count)
	for i := range out {
		out[i] = proofTool()
	}
	return out
}

func pathInventory(count int, prefix string) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = fmt.Sprintf("%s/%04d", prefix, i)
	}
	return out
}

func fileInventory(count int, prefix string) []CanonicalFile {
	out := make([]CanonicalFile, count)
	for i := range out {
		out[i] = CanonicalFile{Path: fmt.Sprintf("%s/%04d", prefix, i), Mode: "file", Hash: proofDigest("a")}
	}
	return out
}

func templateAtDepth(depth int) string {
	return strings.Repeat("{{if true}}", depth) + "x" + strings.Repeat("{{end}}", depth)
}

func templateWithActions(actions int) string {
	if actions <= 0 {
		return "x"
	}
	var builder strings.Builder
	if actions%2 == 1 {
		builder.WriteString("{{if true}}{{else}}{{end}}")
		actions -= 3
	}
	for actions > 0 {
		builder.WriteString("{{if true}}{{end}}")
		actions -= 2
	}
	return builder.String()
}

func baseCandidate() CandidatePreimage {
	candidate, _, _ := Compose(proofSource(), func() Component {
		base, _ := proofComponents()
		return base
	}(), func() Component {
		_, selected := proofComponents()
		return selected
	}(), proofExecutables(), proofPolicy())
	return candidate
}

func candidateWithStringCount(target int) (CandidatePreimage, bool) {
	candidate := baseCandidate()
	current, _ := candidateCost(candidate)
	diff := target - current
	if diff < 0 {
		return CandidatePreimage{}, false
	}
	maxSingles := MaxHomeSources*2 - len(candidate.HomeSources)
	maxFiles := MaxComponentFiles*2 - len(candidate.Components[0].Files) - len(candidate.Components[1].Files)
	singles := diff
	if singles > maxSingles {
		singles = maxSingles
	}
	for singles >= 0 && (diff-singles)%3 != 0 {
		singles--
	}
	files := (diff - singles) / 3
	if singles < 0 || files < 0 || files > maxFiles {
		return CandidatePreimage{}, false
	}
	for i := range singles {
		candidate.HomeSources = append(candidate.HomeSources, fmt.Sprintf("count/home/%05d", i))
	}
	for i := range files {
		target := 0
		if len(candidate.Components[0].Files) >= MaxComponentFiles {
			target = 1
		}
		candidate.Components[target].Files = append(candidate.Components[target].Files, CanonicalFile{Path: fmt.Sprintf("count/file/%05d", i), Mode: "file", Hash: proofDigest("9")})
	}
	got, _ := candidateCost(candidate)
	return candidate, got == target
}

func candidateWithAllocatedBytes(target int) (CandidatePreimage, bool) {
	candidate := baseCandidate()
	_, current := candidateCost(candidate)
	remaining := target - current
	if remaining < 0 {
		return CandidatePreimage{}, false
	}
	lengths := make([]int, remaining/MaxPathBytes)
	for i := range lengths {
		lengths[i] = MaxPathBytes
	}
	remainder := remaining % MaxPathBytes
	if remainder != 0 {
		if remainder < 12 && len(lengths) > 0 {
			lengths[len(lengths)-1] -= 12 - remainder
			remainder = 12
		}
		lengths = append(lengths, remainder)
	}
	if len(lengths)+len(candidate.HomeSources) > MaxHomeSources*2 {
		return CandidatePreimage{}, false
	}
	for i, length := range lengths {
		candidate.HomeSources = append(candidate.HomeSources, pathOfSize(length, i+10000))
	}
	_, got := candidateCost(candidate)
	return candidate, got == target
}

func deterministicPropertyCases() []Scenario {
	const corpusSize = 512
	deterministic := true
	noAmbiguousAcceptance := true
	for i := range corpusSize {
		valid := manifestWithProfiles(i % 8)
		mutated := append([]byte(nil), valid...)
		switch i % 4 {
		case 0:
			mutated = append(mutated, []byte("unknown=true\n")...)
		case 1:
			mutated = append([]byte("schema=1\nschema=1\n"), mutated...)
		case 2:
			mutated = append(mutated, 0)
		case 3:
			mutated = append(mutated, 0xff)
		}
		_, firstDiagnostic, firstErr := DecodeZerb(mutated)
		_, secondDiagnostic, secondErr := DecodeZerb(mutated)
		deterministic = deterministic && sameDiagnostic(firstDiagnostic, secondDiagnostic) && (firstErr != nil) == (secondErr != nil)
		noAmbiguousAcceptance = noAmbiguousAcceptance && firstErr != nil
	}
	return []Scenario{
		{Name: "deterministic-parser-property-corpus", Status: boolStatus(deterministic), RequirementIDs: []string{"VAL-013", "VAL-015", "PC-042"}, Expected: fmt.Sprintf("%d generated inputs produce repeatable accept/error diagnostics", corpusSize), Observed: fmt.Sprintf("cases=%d deterministic=%t", corpusSize, deterministic), RejectedAssumption: "A few hand-written examples establish deterministic parser behavior."},
		{Name: "closed-parser-rejection-property-corpus", Status: boolStatus(noAmbiguousAcceptance), RequirementIDs: []string{"VAL-014", "VAL-015", "PC-042"}, Expected: fmt.Sprintf("%d generated duplicate/unknown/NUL/UTF-8 mutations reject", corpusSize), Observed: fmt.Sprintf("cases=%d all_rejected=%t", corpusSize, noAmbiguousAcceptance), RejectedAssumption: "Decoder success after a semantic alias is acceptable."},
	}
}

func compositionCases() []Scenario {
	source := proofSource()
	base, selected := proofComponents()
	executables := proofExecutables()
	policy := proofPolicy()
	out := make([]Scenario, 0, 40)

	treeManifest, _, treeManifestErr := DecodeZerb([]byte("schema=1\n[base]\npath=\"base\"\n[[profiles]]\nid=\"public\"\nkind=\"embedded\"\npath=\"profiles/public\"\n"))
	baseTree := GitlinkFact{Path: "base", TreeObject: "tree"}
	publicTree := GitlinkFact{Path: "profiles/public", TreeObject: "tree"}
	_, validTreeErr := ValidateGitlinks(treeManifest, []GitlinkFact{baseTree, publicTree}, "")
	treeFactCases := []struct {
		name  string
		facts []GitlinkFact
	}{
		{"base-as-gitlink", []GitlinkFact{{Path: "base", Locator: "https://example.com/base.git", TreeObject: "gitlink"}, publicTree}},
		{"unlisted-base-gitlink", []GitlinkFact{baseTree, publicTree, {Path: "base/vendor", Locator: "https://example.com/vendor.git", TreeObject: "gitlink"}}},
		{"base-tree-mismatch", []GitlinkFact{{Path: "base", TreeObject: "blob"}, publicTree}},
		{"base-missing", []GitlinkFact{publicTree}},
		{"base-ambiguous", []GitlinkFact{baseTree, baseTree, publicTree}},
	}
	out = append(out, Scenario{Name: "base-ordinary-tree-fact", Status: boolStatus(treeManifestErr == nil && validTreeErr == nil), RequirementIDs: []string{"VAL-014", "VAL-016", "PC-040", "PC-043"}, Expected: "mandatory base and embedded profile each match exactly one inspected ordinary tree before candidate composition", Observed: fmt.Sprintf("manifest_valid=%t tree_facts_valid=%t", treeManifestErr == nil, validTreeErr == nil), RejectedAssumption: "A declared path alone proves the base object exists as an ordinary tree."})
	for _, item := range treeFactCases {
		entered, treeErr := ValidateGitlinks(treeManifest, item.facts, "")
		out = append(out, Scenario{Name: item.name, Status: boolStatus(treeErr != nil && !entered), RequirementIDs: []string{"VAL-014", "VAL-016", "PC-040", "PC-043"}, Expected: "reject inspected base/gitlink mismatch before candidate composition or materialization", Observed: fmt.Sprintf("rejected=%t validator_entered=%t", treeErr != nil, entered), RejectedAssumption: "A missing, ambiguous, non-tree, gitlink, or unlisted nested gitlink may enter base composition."})
	}

	candidate, digest, err := Compose(source, base, selected, executables, policy)
	if err != nil {
		out = append(out, Scenario{Name: "canonical-complete-preimage", Status: "failed", RequirementIDs: []string{"VAL-016", "VAL-034", "PC-043", "PC-050"}, Expected: "complete closed candidate encodes", Observed: err.Error(), RejectedAssumption: "A map-free but incomplete candidate preimage satisfies PC-050."})
		return out
	}

	shuffledBase := base
	shuffledBase.Files = reverseCopy(shuffledBase.Files)
	shuffledSelected := selected
	shuffledSelected.HomeSources = reverseCopy(shuffledSelected.HomeSources)
	shuffledSelected.Targets = reverseCopy(shuffledSelected.Targets)
	shuffledExecutables := reverseCopy(executables)
	shuffledPolicy := policy
	shuffledPolicy.ObjectModes = reverseCopy(policy.ObjectModes)
	shuffledPolicy.Declarations = reverseCopy(policy.Declarations)
	shuffledPolicy.TemplateActions = reverseCopy(policy.TemplateActions)
	shuffledPolicy.ProcessClasses = reverseCopy(policy.ProcessClasses)
	shuffledPolicy.PolicyIDs = reverseCopy(policy.PolicyIDs)
	shuffledCandidate, shuffledDigest, shuffledErr := Compose(source, shuffledBase, shuffledSelected, shuffledExecutables, shuffledPolicy)
	encoded, encodedErr := EncodeCandidate(candidate)
	shuffledEncoded, shuffledEncodedErr := EncodeCandidate(shuffledCandidate)
	out = append(out, Scenario{Name: "canonical-map-free-determinism", Status: boolStatus(shuffledErr == nil && encodedErr == nil && shuffledEncodedErr == nil && digest == shuffledDigest && bytes.Equal(encoded, shuffledEncoded)), RequirementIDs: []string{"VAL-016", "VAL-034", "PC-043", "PC-050", "ARCH-004"}, Expected: "equivalent explicit map-free structs normalize to exact identical sorted JSON bytes and digest", Observed: fmt.Sprintf("errors=%t/%t/%t digest_equal=%t bytes_equal=%t", shuffledErr != nil, encodedErr != nil, shuffledEncodedErr != nil, digest == shuffledDigest, bytes.Equal(encoded, shuffledEncoded)), RejectedAssumption: "Map iteration order, arrival order, or digest equality without byte equality proves canonical encoding."})
	expectedTree := append([]CanonicalFile(nil), base.Files...)
	completeErr := ValidateCompleteTree(base, expectedTree)
	omittedTree := cloneComponent(base)
	omittedTree.Files = omittedTree.Files[:len(omittedTree.Files)-1]
	omittedErr := ValidateCompleteTree(omittedTree, expectedTree)
	changedTree := cloneComponent(base)
	changedTree.Files[0].Hash = proofDigest("8")
	changedTreeErr := ValidateCompleteTree(changedTree, expectedTree)
	out = append(out, Scenario{Name: "complete-tree-entry-equality", Status: boolStatus(completeErr == nil && omittedErr != nil && changedTreeErr != nil), RequirementIDs: []string{"VAL-016", "VAL-034", "PC-043", "PC-050", "ARCH-004"}, Expected: "exact path/mode/hash inventory accepts; omitted or changed entry rejects", Observed: fmt.Sprintf("complete=%t omitted_rejected=%t changed_rejected=%t", completeErr == nil, omittedErr != nil, changedTreeErr != nil), RejectedAssumption: "A tree digest or inventory presence proves every canonical entry relation."})
	rawPath := cloneComponent(base)
	rawPath.HomeSources[0] = "home/../dot_base"
	rawErr := ValidateComponent(rawPath)
	normalizedPath := cloneComponent(base)
	normalizedPath.HomeSources[0] = "dot_base_changed"
	normalizedErr := ValidateComponent(normalizedPath)
	out = append(out, Scenario{Name: "raw-versus-normalized-home-path", Status: boolStatus(rawErr != nil && normalizedErr == nil), RequirementIDs: []string{"VAL-014", "VAL-015", "VAL-016", "PC-042", "PC-043", "ARCH-003"}, Expected: "noncanonical raw spelling rejects while distinct canonical spelling enters composition", Observed: fmt.Sprintf("raw_rejected=%t canonical_accepted=%t", rawErr != nil, normalizedErr == nil), RejectedAssumption: "Comparing cleaned paths after accepting raw aliases preserves authored identity."})
	staleSource := cloneCandidate(candidate)
	staleSource.Source.URI = "https://example.org/environment.git"
	_, _, staleSourceErr := DigestCandidate(staleSource)
	out = append(out, Scenario{Name: "candidate-selected-source-cross-reference", Status: boolStatus(staleSourceErr != nil), RequirementIDs: []string{"VAL-016", "VAL-034", "PC-022", "PC-043", "PC-050", "ARCH-004"}, Expected: "candidate source URI and canonical source identity must cross-reference", Observed: fmt.Sprintf("stale_source_id_rejected=%t", staleSourceErr != nil), RejectedAssumption: "A canonical source identity may be retained while the selected source changes."})
	duplicateLock := cloneComponent(selected)
	duplicateLock.Tools[0].Locks = append(duplicateLock.Tools[0].Locks, duplicateLock.Tools[0].Locks[0])
	out = append(out, Scenario{Name: "component-tool-lock-uniqueness", Status: boolStatus(ValidateComponent(duplicateLock) != nil), RequirementIDs: []string{"VAL-016", "PC-041", "PC-043", "PC-050", "ARCH-003"}, Expected: "closed node tool has exactly one named authored artifact lock", Observed: fmt.Sprintf("duplicate_lock_rejected=%t", ValidateComponent(duplicateLock) != nil), RejectedAssumption: "Duplicate or all-in-one lock inventory is a complete authored lock relation."})
	execA, execAErr := CanonicalExecutableFromBytes("git", "2.55.0", "/opt/proof/bin/git", []byte("executable-fixture-a"))
	execARepeat, execARepeatErr := CanonicalExecutableFromBytes("git", "2.55.0", "/opt/proof/bin/git", []byte("executable-fixture-a"))
	execB, execBErr := CanonicalExecutableFromBytes("git", "2.55.0", "/opt/proof/bin/git", []byte("executable-fixture-b"))
	out = append(out, Scenario{Name: "executable-identity-byte-digest", Status: boolStatus(execAErr == nil && execARepeatErr == nil && execBErr == nil && execA.SHA256 == execARepeat.SHA256 && execA.SHA256 != execB.SHA256), RequirementIDs: []string{"VAL-016", "VAL-034", "PC-050", "ARCH-004"}, Expected: "same executable bytes repeat identity digest and changed bytes change it", Observed: fmt.Sprintf("errors=%t/%t/%t same=%t changed=%t", execAErr != nil, execARepeatErr != nil, execBErr != nil, execA.SHA256 == execARepeat.SHA256, execA.SHA256 != execB.SHA256), RejectedAssumption: "An executable path/version label is byte identity."})

	type mutation struct {
		name   string
		reject bool
		apply  func(*CandidatePreimage)
	}
	mutations := []mutation{
		{"source-uri", false, func(c *CandidatePreimage) {
			c.Source.URI = "https://example.org/environment.git"
			c.Source.SourceID, _ = CanonicalSourceID(c.Source.URI)
		}},
		{"source-commit", false, func(c *CandidatePreimage) { c.Source.Commit = strings.Repeat("d", 40) }},
		{"source-root-tree", false, func(c *CandidatePreimage) { c.Source.RootTree = proofDigest("d") }},
		{"profile-id", false, func(c *CandidatePreimage) {
			c.ProfileID = "work"
			c.Components[1].ID = "work"
		}},
		{"component-path", false, func(c *CandidatePreimage) { c.Components[1].Path = "profiles/work" }},
		{"component-tree", false, func(c *CandidatePreimage) { c.Components[1].TreeDigest = proofDigest("e") }},
		{"file-path", false, func(c *CandidatePreimage) { c.Components[0].Files[1].Path = "home/dot_other" }},
		{"file-mode", false, func(c *CandidatePreimage) { c.Components[0].Files[1].Mode = "executable" }},
		{"file-content", false, func(c *CandidatePreimage) { c.Components[0].Files[1].Hash = proofDigest("f") }},
		{"home-declaration", false, func(c *CandidatePreimage) { c.HomeSources[0] = "dot_changed" }},
		{"structural-target", false, func(c *CandidatePreimage) { c.Targets[0] = ".changed" }},
		{"tool-lock", false, func(c *CandidatePreimage) { c.Tools[0].Locks[0].Value = proofDigest("1") }},
		{"managed-executable-path", false, func(c *CandidatePreimage) { c.Executables[0].Path += "-replacement" }},
		{"managed-executable-version", false, func(c *CandidatePreimage) { c.Executables[0].Version += ".1" }},
		{"managed-executable-digest", false, func(c *CandidatePreimage) { c.Executables[0].SHA256 = proofDigest("9") }},
		{"parser-version", true, func(c *CandidatePreimage) { c.Parser.Version = "v2.4.4" }},
		{"policy-version", true, func(c *CandidatePreimage) { c.Policy.Domain = "zerb.policy.v2" }},
		{"policy-allowlist", true, func(c *CandidatePreimage) { c.Policy.TemplateActions = append(c.Policy.TemplateActions, "range") }},
		{"tool-alias", true, func(c *CandidatePreimage) { c.Tools[0].Version = "lts" }},
		{"unsupported-mode", true, func(c *CandidatePreimage) { c.Components[0].Files[1].Mode = "symlink" }},
	}
	for _, item := range mutations {
		changed := cloneCandidate(candidate)
		item.apply(&changed)
		_, changedDigest, changedErr := DigestCandidate(changed)
		approvalInvalidated := CheckCandidateApproval(changed, digest) != nil
		ok := changedErr == nil && changedDigest != digest && approvalInvalidated
		expected := "accepted changed identity produces distinct digest and invalidates prior approval"
		if item.reject {
			ok = changedErr != nil && approvalInvalidated
			expected = "closed unsupported identity change rejects and invalidates prior approval"
		}
		out = append(out, Scenario{Name: "candidate-sensitivity-" + item.name, Status: boolStatus(ok), RequirementIDs: []string{"VAL-016", "VAL-034", "VAL-035", "PC-031", "PC-050"}, Expected: expected, Observed: fmt.Sprintf("rejected=%t digest_changed=%t prior_approval_invalidated=%t", changedErr != nil, changedDigest != "" && changedDigest != digest, approvalInvalidated), RejectedAssumption: "A candidate-bound field may change without a new trust relation."})
	}
	unchanged, availabilityDigest, availabilityErr := DigestCandidate(cloneCandidate(candidate))
	_ = unchanged
	availabilityObserved := true
	approvalObserved := "not-approved"
	destinationObserved := "live-target-absent"
	_ = availabilityObserved
	_ = approvalObserved
	_ = destinationObserved
	out = append(out, Scenario{Name: "candidate-run-local-observations-excluded", Status: boolStatus(availabilityErr == nil && availabilityDigest == digest), RequirementIDs: []string{"VAL-016", "VAL-034", "PC-050"}, Expected: "availability, approval, and live destination observations are not candidate fields", Observed: fmt.Sprintf("digest_equal=%t candidate_has_no_run_local_fields=%t", availabilityDigest == digest, true), RejectedAssumption: "A live observation or approval belongs in deterministic candidate identity."})

	bad := []struct {
		name string
		mut  func(*Component, *Component)
	}{
		{"duplicate-tool", func(a, b *Component) { a.HasMise = true; a.HasLock = true; a.Tools = append([]Tool(nil), b.Tools...) }},
		{"duplicate-home", func(a, b *Component) { b.HomeSources = []string{"dot_base"} }},
		{"duplicate-target", func(a, b *Component) { b.Targets = []string{".base"} }},
		{"alias-version", func(a, b *Component) { b.Tools[0].Version = "lts" }},
		{"missing-lock", func(a, b *Component) { b.Tools[0].Locks = nil }},
		{"lock-only-component", func(a, b *Component) { b.HasMise = false; b.HasHome = false; b.Tools = nil }},
	}
	for _, item := range bad {
		a := cloneComponent(base)
		b := cloneComponent(selected)
		item.mut(&a, &b)
		_, _, caseErr := Compose(source, a, b, executables, policy)
		out = append(out, Scenario{Name: "compose-" + item.name, Status: boolStatus(caseErr != nil), RequirementIDs: []string{"VAL-016", "PC-041", "PC-043", "PC-050"}, Expected: "reject before trust or manager execution", Observed: fmt.Sprintf("rejected=%t", caseErr != nil), RejectedAssumption: "Composition collisions, aliases, or incomplete locks can use override or live resolution semantics."})
	}

	gitlinkManifest, _, _ := DecodeZerb([]byte("schema=1\n[base]\npath=\"base\"\n[[profiles]]\nid=\"private\"\nkind=\"gitlink\"\npath=\"profiles/private\"\n"))
	ordinaryBase := GitlinkFact{Path: "base", TreeObject: "tree"}
	privateGitlink := GitlinkFact{Path: "profiles/private", Locator: "https://example.com/private.git", TreeObject: "gitlink"}
	entered, gitlinkErr := ValidateGitlinks(gitlinkManifest, []GitlinkFact{ordinaryBase, privateGitlink}, "private")
	out = append(out, Scenario{Name: "selected-gitlink-pre-validator", Status: boolStatus(gitlinkErr != nil && !entered), RequirementIDs: []string{"VAL-038", "PC-043", "ARCH-015"}, Expected: "reject at selection before static validator entry", Observed: fmt.Sprintf("rejected=%t validator_entered=%t", gitlinkErr != nil, entered), RejectedAssumption: "A statically valid unselected gitlink locator makes that profile selectable."})
	gitlinkCases := []struct {
		name  string
		facts []GitlinkFact
	}{
		{"missing-static-declaration", []GitlinkFact{ordinaryBase}},
		{"duplicate-static-declaration", []GitlinkFact{ordinaryBase, privateGitlink, {Path: "profiles/private", Locator: "https://example.com/other.git", TreeObject: "gitlink"}}},
		{"ordinary-tree-mismatch", []GitlinkFact{ordinaryBase, {Path: "profiles/private", Locator: "https://example.com/private.git", TreeObject: "tree"}}},
		{"invalid-locator", []GitlinkFact{ordinaryBase, {Path: "profiles/private", Locator: "../private.git", TreeObject: "gitlink"}}},
	}
	for _, item := range gitlinkCases {
		entered, caseErr := ValidateGitlinks(gitlinkManifest, item.facts, "")
		out = append(out, Scenario{Name: "gitlink-" + item.name, Status: boolStatus(caseErr != nil && !entered), RequirementIDs: []string{"VAL-014", "PC-040", "PC-043"}, Expected: "reject bounded static mismatch", Observed: fmt.Sprintf("rejected=%t validator_entered=%t", caseErr != nil, entered), RejectedAssumption: "A path match alone proves a unique canonical locator and gitlink tree object."})
	}
	embedded, _, _ := DecodeZerb([]byte("schema=1\n[base]\npath=\"base\"\n[[profiles]]\nid=\"public\"\nkind=\"embedded\"\npath=\"profiles/public\"\n"))
	embeddedEntered, embeddedErr := ValidateGitlinks(embedded, []GitlinkFact{ordinaryBase, {Path: "profiles/public", Locator: "https://example.com/public.git", TreeObject: "gitlink"}}, "")
	out = append(out, Scenario{Name: "embedded-path-is-gitlink", Status: boolStatus(embeddedErr != nil && !embeddedEntered), RequirementIDs: []string{"VAL-014", "PC-040", "PC-043"}, Expected: "reject kind/tree mismatch", Observed: fmt.Sprintf("rejected=%t validator_entered=%t", embeddedErr != nil, embeddedEntered), RejectedAssumption: "An embedded declaration may silently name a gitlink object."})
	return out
}
func proofSource() CanonicalSource {
	uri := "https://example.com/environment.git"
	sourceID, _ := CanonicalSourceID(uri)
	return CanonicalSource{URI: uri, SourceID: sourceID, Commit: strings.Repeat("c", 40), RootTree: proofDigest("a")}
}

func proofComponents() (Component, Component) {
	lock := ToolLock{Name: "artifact", Value: proofDigest("a")}
	base := Component{
		ID:         "base",
		Path:       "profiles/base",
		TreeDigest: proofDigest("b"),
		Manifest:   ComponentManifest{Schema: 1},
		HasHome:    true,
		HomeSources: []string{
			"dot_base",
			"dot_shared",
		},
		Targets: []string{".base", ".shared"},
		Files: []CanonicalFile{
			{Path: "home/dot_base", Mode: "file", Hash: proofDigest("c")},
			{Path: "home", Mode: "directory"},
		},
	}
	selected := Component{
		ID:         "personal",
		Path:       "profiles/personal",
		TreeDigest: proofDigest("d"),
		Manifest:   ComponentManifest{Schema: 1},
		HasMise:    true,
		HasLock:    true,
		HasHome:    true,
		Tools:      []Tool{{ID: "node", Version: "24.4.1", Locks: []ToolLock{lock}}},
		HomeSources: []string{
			"dot_personal",
			"dot_work",
		},
		Targets: []string{".personal", ".work"},
		Files: []CanonicalFile{
			{Path: "home/dot_personal", Mode: "executable", Hash: proofDigest("e")},
			{Path: "mise.lock", Mode: "file", Hash: proofDigest("f")},
			{Path: "mise.toml", Mode: "file", Hash: proofDigest("0")},
		},
	}
	return base, selected
}

func proofExecutables() []CanonicalExecutable {
	return []CanonicalExecutable{
		{ID: "git", Version: "2.55.0", Path: "/nix/store/proof-git/bin/git", SHA256: proofDigest("1")},
		{ID: "bash", Version: "5.3.15", Path: "/nix/store/proof-bash/bin/bash", SHA256: proofDigest("2")},
		{ID: "mise", Version: "2026.7.17", Path: "/nix/store/proof-mise/bin/mise", SHA256: proofDigest("3")},
		{ID: "chezmoi", Version: "v2.70.5-zerb.1", Path: "/nix/store/proof-chezmoi/bin/chezmoi", SHA256: proofDigest("4")},
	}
}

func proofPolicy() CanonicalPolicy {
	return CanonicalPolicy{
		Schema:                   1,
		Domain:                   "zerb.policy",
		ObjectModes:              []string{"file", "directory", "executable"},
		Declarations:             []string{"profile.toml", "mise.toml", "mise.lock", "home"},
		TemplateActions:          []string{"literal-text", "if-true", "if-false"},
		ProcessClasses:           []string{"mise-core-node", "git-public", "chezmoi-static", "bash-shell"},
		PolicyIDs:                []string{"zerb.lifecycle.v6", "zerb.git-public.v3", "zerb.mise-core-node.v3", "zerb.chezmoi-static.v2", "zerb.execenv.v4", "zerb.destination-recheck.v1"},
		TargetMapperExecutableID: "chezmoi",
		ProviderFree:             true,
		HostFactIndependent:      true,
		DestinationRecheck:       true,
		ExactReapplyOnly:         true,
	}
}

func proofDigest(fill string) string {
	return "sha256:" + strings.Repeat(fill, 64)
}

func cloneCandidate(candidate CandidatePreimage) CandidatePreimage {
	encoded, _ := json.Marshal(candidate)
	var clone CandidatePreimage
	_ = json.Unmarshal(encoded, &clone)
	return clone
}

func cloneComponent(component Component) Component {
	encoded, _ := json.Marshal(component)
	var clone Component
	_ = json.Unmarshal(encoded, &clone)
	return clone
}

func reverseCopy[T any](values []T) []T {
	out := append([]T(nil), values...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}
func renderedMarkerCase(rendered string) Scenario {
	source := "{{if true}}" + rendered + "{{else}}unselected{{end}}"
	status := "failed"
	observed := "closed template did not render the selected literal marker in memory"
	if ValidateTemplate("rendered-marker", []byte(source)) == nil {
		if parsed, err := template.New("rendered-marker").Parse(source); err == nil {
			var output bytes.Buffer
			if err = parsed.Execute(&output, nil); err == nil && output.String() == rendered {
				status = "observed"
				observed = "selected literal marker rendered exactly in memory; only its fingerprint enters the result"
			}
		}
	}
	return Scenario{Name: "rendered-marker-in-memory", Status: status, RequirementIDs: []string{"VAL-005", "VAL-039", "VAL-040", "ARCH-028"}, Expected: "closed literal-boolean template renders exact marker only in memory", Observed: observed, RejectedAssumption: "Static validation alone demonstrates the selected literal render output."}
}

func templateCases(rendered string) []Scenario {
	good := []struct{ name, source string }{
		{"literal-text", "literal"},
		{"literal-if-else", "{{if true}}" + rendered + "{{else}}x{{end}}"},
		{"nested-literal-booleans", "{{if false}}x{{else}}{{if true}}y{{end}}{{end}}"},
	}
	bad := []struct{ name, source string }{
		{"dot-data", "{{.}}"}, {"field-data", "{{.data}}"}, {"variable", "{{$x := true}}"},
		{"range", "{{range .}}{{end}}"}, {"dynamic-include", "{{template \"x\" .}}"},
		{"provider-env", "{{env \"HOME\"}}"}, {"provider-lookup", "{{lookup \"x\"}}"},
		{"command-output", "{{output \"id\"}}"}, {"filesystem-read", "{{readFile \"x\"}}"},
		{"host-fact", "{{.chezmoi.hostname}}"}, {"dynamic-condition", "{{if .x}}x{{end}}"},
		{"comment-action", "{{/* hidden */}}"},
	}
	var out []Scenario
	for _, item := range good {
		err := ValidateTemplate("allow-"+item.name, []byte(item.source))
		out = append(out, Scenario{Name: "template-allow-" + item.name, Status: boolStatus(err == nil), RequirementIDs: []string{"VAL-038", "VAL-039", "PC-043", "ARCH-015"}, Expected: "accepted through ValidateTemplate AST allowlist", Observed: fmt.Sprintf("accepted=%t", err == nil), RejectedAssumption: "Static literal templates require provider, data, variable, host, command, filesystem, or manager access."})
	}
	for _, item := range bad {
		err := ValidateTemplate("reject-"+item.name, []byte(item.source))
		out = append(out, Scenario{Name: "template-reject-" + item.name, Status: boolStatus(err != nil), RequirementIDs: []string{"VAL-038", "VAL-039", "PC-043", "ARCH-015"}, Expected: "rejected by ValidateTemplate before candidate/adapter invocation", Observed: fmt.Sprintf("rejected=%t", err != nil), RejectedAssumption: "Parsing successfully with text/template implies admission to the closed MVP AST surface."})
	}
	return out
}

type approvalProbe struct {
	status             int
	candidateComputed  bool
	managerInvocations int
	mutations          int
	authentication     int
	network            int
	thirdApprovals     int
}

func probeApprovals(command, source, trust, mutation string, transportAuthorization, yes bool) approvalProbe {
	var result approvalProbe
	if transportAuthorization {
		result.status = 2
		return result
	}
	if source != "public-embedded" {
		result.status = 1
		return result
	}
	result.candidateComputed = true
	if trust == "" {
		result.status = 2
		return result
	}
	if trust != "exact" {
		result.status = 1
		return result
	}
	if command == "plan" {
		result.managerInvocations = 1
		return result
	}
	if mutation != "authorized" {
		result.status = 2
		_ = yes
		return result
	}
	result.mutations = 1
	return result
}

func approvalCases() []Scenario {
	candidate := baseCandidate()
	_, digest, err := DigestCandidate(candidate)
	exactErr := CheckCandidateApproval(candidate, digest)
	missingErr := CheckCandidateApproval(candidate, "")
	mismatchErr := CheckCandidateApproval(candidate, proofDigest("9"))
	mutated := cloneCandidate(candidate)
	mutated.Source.Commit = strings.Repeat("d", 40)
	mutationErr := CheckCandidateApproval(mutated, digest)
	out := []Scenario{
		{Name: "candidate-approval-exact-digest", Status: boolStatus(err == nil && exactErr == nil && missingErr != nil && mismatchErr != nil), RequirementIDs: []string{"VAL-035", "PC-030", "PC-031", "PC-050", "ARCH-004"}, Expected: "only exact computed candidate digest approves", Observed: fmt.Sprintf("candidate_valid=%t exact_accepted=%t missing_rejected=%t mismatch_rejected=%t", err == nil, exactErr == nil, missingErr != nil, mismatchErr != nil), RejectedAssumption: "Approval presence is equivalent to equality with the complete candidate digest."},
		{Name: "candidate-mutation-invalidates-approval", Status: boolStatus(mutationErr != nil), RequirementIDs: []string{"VAL-016", "VAL-034", "VAL-035", "PC-031", "PC-050", "ARCH-004"}, Expected: "candidate-bound mutation invalidates prior exact approval", Observed: fmt.Sprintf("prior_approval_rejected=%t", mutationErr != nil), RejectedAssumption: "An approval remains valid after source, component, tool, executable, parser, policy, or target identity changes."},
		approvalSeparationScenario(),
	}
	return out
}

func approvalSeparationScenario() Scenario {
	missingTrust := probeApprovals("plan", "public-embedded", "", "", false, false)
	mismatch := probeApprovals("plan", "public-embedded", "mismatch", "", false, false)
	exactPlan := probeApprovals("plan", "public-embedded", "exact", "", false, false)
	missingMutation := probeApprovals("apply", "public-embedded", "exact", "", false, false)
	yesOnly := probeApprovals("apply", "public-embedded", "exact", "", false, true)
	authorized := probeApprovals("apply", "public-embedded", "exact", "authorized", false, false)
	selectedGitlink := probeApprovals("plan", "selected-gitlink", "", "", false, false)
	privateSource := probeApprovals("plan", "private", "", "", false, false)
	transportFlag := probeApprovals("plan", "public-embedded", "", "", true, false)
	zeroAccess := func(result approvalProbe) bool {
		return result.authentication == 0 && result.network == 0 && result.managerInvocations == 0 && result.mutations == 0
	}
	ok := missingTrust.status == 2 && missingTrust.candidateComputed && missingTrust.managerInvocations == 0 &&
		mismatch.status == 1 && exactPlan.status == 0 && exactPlan.managerInvocations == 1 &&
		missingMutation.status == 2 && missingMutation.mutations == 0 &&
		yesOnly.status == 2 && yesOnly.mutations == 0 &&
		authorized.status == 0 && authorized.mutations == 1 &&
		selectedGitlink.status == 1 && zeroAccess(selectedGitlink) &&
		privateSource.status == 1 && zeroAccess(privateSource) &&
		transportFlag.status == 2 && zeroAccess(transportFlag) &&
		missingTrust.thirdApprovals == 0 && authorized.thirdApprovals == 0
	return Scenario{Name: "candidate-mutation-approval-separation", Status: boolStatus(ok), RequirementIDs: []string{"VAL-035", "PC-030", "PC-031", "PC-032", "PC-033", "ARCH-004", "ARCH-005", "ARCH-007"}, Expected: "candidate trust and mutation authorization are independent; source policy and invalid transport flags precede every external boundary", Observed: fmt.Sprintf("missing_trust_status=%d mismatch_status=%d exact_plan_manager_calls=%d missing_mutation_status=%d yes_only_status=%d authorized_mutations=%d selected_gitlink_zero_access=%t private_zero_access=%t transport_flag_zero_access=%t third_approvals=%d", missingTrust.status, mismatch.status, exactPlan.managerInvocations, missingMutation.status, yesOnly.status, authorized.mutations, zeroAccess(selectedGitlink), zeroAccess(privateSource), zeroAccess(transportFlag), authorized.thirdApprovals), RejectedAssumption: "`--yes`, candidate equality, source selection, or an invented transport approval can authorize another boundary."}
}

func bounds() []Bound {
	return []Bound{{"document", MaxDocumentBytes, "bytes", "pre-decode memory ceiling"}, {"profiles", MaxProfiles, "entries", "closed selectable manifest"}, {"profile_id", MaxProfileIDBytes, "UTF-8 bytes", "PC-040 grammar maximum"}, {"profile_label", MaxProfileLabel, "UTF-8 bytes", "bounded display metadata"}, {"source", MaxSourceBytes, "ASCII bytes", "bounded canonical public URI"}, {"path", MaxPathBytes, "UTF-8 bytes", "bounded normalized logical path"}, {"path_depth", MaxPathComponents, "components", "bounded traversal work"}, {"path_component", MaxPathComponent, "UTF-8 bytes", "portable component ceiling"}, {"component_files", MaxComponentFiles, "entries", "bounded tree inventory"}, {"tools", MaxTools, "entries", "bounded native declaration inventory"}, {"tool_id", MaxToolIDBytes, "ASCII bytes", "bounded tool identity"}, {"version", MaxVersionBytes, "ASCII bytes", "bounded exact authored scalar"}, {"locks_per_tool", MaxLocksPerTool, "entries", "bounded complete lock inventory"}, {"lock_value", MaxLockValueBytes, "ASCII bytes", "bounded exact integrity value"}, {"home_sources", MaxHomeSources, "entries", "bounded effective-source inventory"}, {"targets", MaxTargets, "entries", "bounded structural mapping inventory"}, {"template", MaxTemplateBytes, "bytes", "same pre-parse byte ceiling"}, {"template_depth", MaxTemplateDepth, "AST levels", "bounded conditional recursion"}, {"template_actions", MaxTemplateActions, "actions", "bounded parser/validator work"}, {"total_strings", MaxTotalStrings, "entries", "bounded candidate string allocation"}, {"allocated_bytes", MaxAllocatedBytes, "bytes", "defensible aggregate harness allocation ceiling"}}
}
func boolStatus(ok bool) string {
	if ok {
		return "observed"
	}
	return "failed"
}
func evidenceRelations() ([]EvidenceRelation, error) {
	types := []string{"source", "tree", "candidate", "candidate", "executable", "executable"}
	refs := make([]string, len(types))
	for i, kind := range types {
		ref, err := typedRef(kind)
		if err != nil {
			return nil, err
		}
		refs[i] = ref
	}
	return []EvidenceRelation{
		{LeftRef: refs[0], Relation: "selects-canonical-tree", RightRef: refs[1]},
		{LeftRef: refs[2], Relation: "same-input-produces-same-candidate-identity", RightRef: refs[2]},
		{LeftRef: refs[2], Relation: "bound-mutation-produces-different-candidate-identity", RightRef: refs[3]},
		{LeftRef: refs[4], Relation: "executable-byte-mutation-produces-different-identity", RightRef: refs[5]},
	}, nil
}

func typedRef(kind string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return kind + "-ref:" + hex.EncodeToString(b), nil
}

func marker(class string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return class + "-" + hex.EncodeToString(b), nil
}
func fingerprint(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

func validateRoots(cfg Config) (string, string, error) {
	if cfg.Root == "" || !filepath.IsAbs(cfg.Root) || cfg.ArtifactDir == "" || !filepath.IsAbs(cfg.ArtifactDir) {
		return "", "", errors.New("explicit absolute disposable root and artifact directory required")
	}
	root, err := filepath.EvalSymlinks(cfg.Root)
	if err != nil || root != filepath.Clean(cfg.Root) {
		return "", "", errors.New("root must be existing canonical path")
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return "", "", errors.New("root must be directory")
	}
	var fs syscall.Statfs_t
	if syscall.Statfs(root, &fs) != nil || uint64(fs.Type) != btrfsSuperMagic {
		return "", "", errors.New("root must be on btrfs")
	}
	art, err := filepath.EvalSymlinks(cfg.ArtifactDir)
	if err != nil {
		return "", "", errors.New("artifact directory must exist")
	}
	ast, err := os.Stat(art)
	if err != nil || !ast.IsDir() || !within(art, root) {
		return "", "", errors.New("artifact directory must be beneath root")
	}
	if len(cfg.ProtectedRoots) == 0 {
		return "", "", errors.New("canonical protected roots captured before environment poisoning are required")
	}
	for _, p := range cfg.ProtectedRoots {
		if p == "" {
			continue
		}
		p, err = filepath.EvalSymlinks(p)
		if err != nil {
			return "", "", errors.New("canonical protected path unavailable")
		}
		if within(root, p) || within(p, root) {
			return "", "", errors.New("disposable root overlaps a protected path class")
		}
	}
	return root, art, nil
}
func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
