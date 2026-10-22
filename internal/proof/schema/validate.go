package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceid"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/sourceurl"
)

var (
	profileIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	hexRE       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	objectIDRE  = regexp.MustCompile(`^[0-9a-f]+$`)
)

type validationError struct{ message string }

func (e validationError) Error() string { return e.message }

func DecodeZerb(data []byte) (ZerbManifest, *DiagnosticObservation, error) {
	var out ZerbManifest
	if err := predecode(data); err != nil {
		return out, diagnostic("zerb.toml", err), err
	}
	err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&out)
	if err != nil {
		return out, diagnostic("zerb.toml", err), err
	}
	if out.Schema != 1 {
		return out, diagnostic("zerb.toml", validationError{"schema must equal 1"}), validationError{"schema must equal 1"}
	}
	if err = validatePath(out.Base.Path); err != nil {
		return out, diagnostic("zerb.toml", err), fmt.Errorf("base path: %w", err)
	}
	if len(out.Profiles) > MaxProfiles {
		return out, diagnostic("zerb.toml", validationError{"profile count exceeds 256"}), validationError{"profile count exceeds 256"}
	}
	ids := make([]string, 0, len(out.Profiles))
	paths := make([]string, 0, len(out.Profiles)+1)
	paths = append(paths, out.Base.Path)
	for _, p := range out.Profiles {
		if !profileIDRE.MatchString(p.ID) {
			return out, diagnostic("zerb.toml", validationError{"invalid profile id"}), validationError{"invalid profile id"}
		}
		if p.Kind != "embedded" && p.Kind != "gitlink" {
			return out, diagnostic("zerb.toml", validationError{"invalid profile kind"}), validationError{"invalid profile kind"}
		}
		if err = validatePath(p.Path); err != nil {
			return out, diagnostic("zerb.toml", err), fmt.Errorf("profile path: %w", err)
		}
		ids = append(ids, p.ID)
		paths = append(paths, p.Path)
	}
	if duplicate(ids) {
		return out, diagnostic("zerb.toml", validationError{"duplicate profile id"}), validationError{"duplicate profile id"}
	}
	if duplicate(paths) {
		return out, diagnostic("zerb.toml", validationError{"duplicate normalized path"}), validationError{"duplicate normalized path"}
	}
	return out, nil, nil
}

func DecodeComponent(data []byte) (ComponentManifest, *DiagnosticObservation, error) {
	var out ComponentManifest
	if err := predecode(data); err != nil {
		return out, diagnostic("profile.toml", err), err
	}
	err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&out)
	if err != nil {
		return out, diagnostic("profile.toml", err), err
	}
	if out.Schema != 1 {
		return out, diagnostic("profile.toml", validationError{"schema must equal 1"}), validationError{"schema must equal 1"}
	}
	if len([]byte(out.Label)) > MaxProfileLabel || !utf8.ValidString(out.Label) {
		return out, diagnostic("profile.toml", validationError{"invalid or oversized label"}), validationError{"invalid or oversized label"}
	}
	return out, nil, nil
}

func predecode(data []byte) error {
	if len(data) > MaxDocumentBytes {
		return validationError{"document exceeds 1048576 bytes"}
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return validationError{"document is not valid UTF-8"}
	}
	// Dotted keys are intentionally outside the closed schema even when a decoder
	// could map them to the same Go field as an explicit table.
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if strings.ContainsAny(line, "'\"") || strings.Contains(strings.Trim(line, "[] \t"), ".") {
				return validationError{"dotted or quoted table alias rejected"}
			}
			continue
		}
		if i := strings.IndexByte(line, '='); i >= 0 {
			key := strings.TrimSpace(line[:i])
			if strings.Contains(key, ".") || strings.ContainsAny(key, "'\"") {
				return validationError{"dotted or quoted key alias rejected"}
			}
			rhs := strings.TrimSpace(line[i+1:])
			if strings.HasPrefix(rhs, "{") || strings.HasPrefix(rhs, "[") {
				return validationError{"inline table or array alias rejected"}
			}
		}
	}
	return nil
}

func diagnostic(name string, err error) *DiagnosticObservation {
	d := &DiagnosticObservation{Case: name, Message: err.Error()}
	var de *toml.DecodeError
	if errors.As(err, &de) {
		d.Row, d.Column = de.Position()
		d.Key = strings.Join(de.Key(), ".")
	}
	return d
}

func validatePath(s string) error {
	if s == "" || len([]byte(s)) > MaxPathBytes || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\x00") || strings.HasPrefix(s, "/") {
		return validationError{"invalid path"}
	}
	parts := strings.Split(s, "/")
	if len(parts) > MaxPathComponents {
		return validationError{"path depth exceeds limit"}
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || len([]byte(p)) > MaxPathComponent {
			return validationError{"invalid path component"}
		}
	}
	if path.Clean(s) != s {
		return validationError{"path is not canonical"}
	}
	return nil
}

func ValidateSource(s string) error {
	if len(s) > MaxSourceBytes {
		return validationError{"source exceeds byte bound"}
	}
	if err := sourceurl.ValidatePublicHTTPS(s); err != nil {
		return validationError{"invalid canonical public source: " + err.Error()}
	}
	return nil
}

func ValidateGitlinks(m ZerbManifest, facts []GitlinkFact, selected string) (bool, error) {
	for _, p := range m.Profiles {
		if p.ID == selected && p.Kind == "gitlink" {
			return false, validationError{"selected gitlink rejected before validator entry"}
		}
	}

	baseMatches := 0
	for _, f := range facts {
		if err := validatePath(f.Path); err != nil {
			return false, validationError{"invalid inspected tree fact path"}
		}
		if f.Path == m.Base.Path {
			baseMatches++
			if f.TreeObject != "tree" || f.Locator != "" {
				return false, validationError{"base must be exactly one ordinary tree"}
			}
		}
		if f.TreeObject == "gitlink" {
			listed := false
			for _, p := range m.Profiles {
				if p.Path == f.Path && p.Kind == "gitlink" {
					listed = true
					break
				}
			}
			if !listed {
				return false, validationError{"unlisted gitlink in inspected tree"}
			}
		}
	}
	if baseMatches != 1 {
		return false, validationError{"base must match exactly one inspected ordinary tree"}
	}

	for _, p := range m.Profiles {
		matches := 0
		for _, f := range facts {
			if f.Path != p.Path {
				continue
			}
			matches++
			if p.Kind == "gitlink" {
				if f.TreeObject != "gitlink" || validateGitlinkLocator(f.Locator) != nil {
					return false, validationError{"gitlink declaration mismatch"}
				}
			} else if f.TreeObject != "tree" || f.Locator != "" {
				return false, validationError{"embedded path is not an ordinary tree"}
			}
		}
		if matches != 1 {
			return false, validationError{"profile must match exactly one inspected tree fact"}
		}
	}
	return true, nil
}
func validateGitlinkLocator(s string) error {
	if _, err := sourceurl.ValidateUnselectedGitlink(s); err != nil {
		return validationError{"invalid static gitlink locator: " + err.Error()}
	}
	return nil
}

func ValidateComponent(c Component) error {
	if c.ID != "base" && !profileIDRE.MatchString(c.ID) {
		return validationError{"invalid component identity"}
	}
	if err := validatePath(c.Path); err != nil {
		return fmt.Errorf("component path: %w", err)
	}
	if !validDigest(c.TreeDigest) {
		return validationError{"invalid component tree digest"}
	}
	if c.Manifest.Schema != 1 || (!c.HasMise && !c.HasHome) || c.HasLock != c.HasMise || c.HasMise != (len(c.Tools) != 0) {
		return validationError{"invalid component payload"}
	}
	if len(c.Files) > MaxComponentFiles || len(c.Tools) > MaxTools || len(c.HomeSources) > MaxHomeSources || len(c.Targets) > MaxTargets {
		return validationError{"component count bound exceeded"}
	}
	paths := append(append([]string(nil), c.HomeSources...), c.Targets...)
	for _, p := range paths {
		if err := validatePath(p); err != nil {
			return err
		}
	}
	if duplicate(c.HomeSources) || duplicate(c.Targets) {
		return validationError{"duplicate component path"}
	}
	filePaths := make([]string, 0, len(c.Files))
	for _, f := range c.Files {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		switch f.Mode {
		case "directory":
			if f.Hash != "" {
				return validationError{"directory must not carry a content hash"}
			}
		case "file", "executable":
			if !validDigest(f.Hash) {
				return validationError{"invalid canonical file hash"}
			}
		default:
			return validationError{"unsupported canonical object mode"}
		}
		filePaths = append(filePaths, f.Path)
	}
	if duplicate(filePaths) {
		return validationError{"duplicate normalized file path"}
	}
	toolIDs := make([]string, len(c.Tools))
	for i, t := range c.Tools {
		if err := validateTool(t); err != nil {
			return err
		}
		toolIDs[i] = t.ID
	}
	if duplicate(toolIDs) {
		return validationError{"duplicate tool identity"}
	}
	stringCount, allocated := componentCost(c)
	if stringCount > MaxTotalStrings || allocated > MaxAllocatedBytes {
		return validationError{"aggregate string or allocation bound exceeded"}
	}
	return nil
}
func validateTool(t Tool) error {
	if len(t.ID) > MaxToolIDBytes || len(t.Version) > MaxVersionBytes || t.ID != "node" || t.Version != "24.4.1" {
		return validationError{"only exact node@24.4.1 is admitted"}
	}
	if len(t.Locks) != MaxLocksPerTool {
		return validationError{"complete authored lock required"}
	}
	for _, l := range t.Locks {
		if l.Name != "artifact" || len(l.Value) > MaxLockValueBytes || !validDigest(l.Value) {
			return validationError{"invalid authored lock"}
		}
	}
	return nil
}

func CanonicalSourceID(uri string) (string, error) {
	if err := ValidateSource(uri); err != nil {
		return "", err
	}
	return sourceid.Digest(uri)
}

func Compose(source CanonicalSource, base, selected Component, executables []CanonicalExecutable, policy CanonicalPolicy) (CandidatePreimage, string, error) {
	if base.ID != "base" || selected.ID == "" || selected.ID == "base" || base.Path == selected.Path {
		return CandidatePreimage{}, "", validationError{"composition must contain one distinct base and one selected component"}
	}
	if err := validateCanonicalSource(source); err != nil {
		return CandidatePreimage{}, "", err
	}
	if err := ValidateComponent(base); err != nil {
		return CandidatePreimage{}, "", err
	}
	if err := ValidateComponent(selected); err != nil {
		return CandidatePreimage{}, "", err
	}
	tools := append(append([]Tool(nil), base.Tools...), selected.Tools...)
	homes := append(append([]string(nil), base.HomeSources...), selected.HomeSources...)
	targets := append(append([]string(nil), base.Targets...), selected.Targets...)
	ids := make([]string, len(tools))
	for i := range tools {
		ids[i] = tools[i].ID
	}
	if duplicate(ids) || duplicate(homes) || duplicate(targets) {
		return CandidatePreimage{}, "", validationError{"duplicate tool, raw home source, or normalized target"}
	}
	canonicalTools := make([]CanonicalTool, len(tools))
	for i, tool := range tools {
		locks := append([]ToolLock(nil), tool.Locks...)
		sort.Slice(locks, func(i, j int) bool {
			return locks[i].Name+"\x00"+locks[i].Value < locks[j].Name+"\x00"+locks[j].Value
		})
		canonicalTools[i] = CanonicalTool{ID: tool.ID, Version: tool.Version, Locks: locks}
	}
	components := []CanonicalComponent{
		{ID: base.ID, Path: base.Path, TreeDigest: base.TreeDigest, Files: sortedFiles(base.Files)},
		{ID: selected.ID, Path: selected.Path, TreeDigest: selected.TreeDigest, Files: sortedFiles(selected.Files)},
	}
	preimage := CandidatePreimage{
		Schema:      1,
		Domain:      "zerb.candidate",
		Source:      source,
		ProfileID:   selected.ID,
		Components:  components,
		Tools:       canonicalTools,
		HomeSources: homes,
		Targets:     targets,
		Executables: append([]CanonicalExecutable(nil), executables...),
		Parser:      proofParser(),
		Policy:      policy,
	}
	return DigestCandidate(preimage)
}

func DigestCandidate(candidate CandidatePreimage) (CandidatePreimage, string, error) {
	if candidate.Schema != 1 || candidate.Domain != "zerb.candidate" {
		return CandidatePreimage{}, "", validationError{"invalid candidate schema or domain"}
	}
	if err := validateCanonicalSource(candidate.Source); err != nil {
		return CandidatePreimage{}, "", err
	}
	if !profileIDRE.MatchString(candidate.ProfileID) || len(candidate.Components) != 2 {
		return CandidatePreimage{}, "", validationError{"invalid selected profile or component count"}
	}
	sort.Slice(candidate.Components, func(i, j int) bool {
		return candidate.Components[i].ID < candidate.Components[j].ID
	})
	if candidate.Components[0].ID != "base" || candidate.Components[1].ID != candidate.ProfileID {
		return CandidatePreimage{}, "", validationError{"component identity does not match selected profile"}
	}
	if len(candidate.Tools) > MaxTools || len(candidate.HomeSources) > MaxHomeSources*2 || len(candidate.Targets) > MaxTargets*2 {
		return CandidatePreimage{}, "", validationError{"candidate declaration count bound exceeded"}
	}
	for i := range candidate.Components {
		component := &candidate.Components[i]
		if err := validatePath(component.Path); err != nil || !validDigest(component.TreeDigest) {
			return CandidatePreimage{}, "", validationError{"invalid canonical component identity"}
		}
		if len(component.Files) > MaxComponentFiles {
			return CandidatePreimage{}, "", validationError{"candidate component file count bound exceeded"}
		}
		component.Files = sortedFiles(component.Files)
		paths := make([]string, len(component.Files))
		for j, file := range component.Files {
			if err := validatePath(file.Path); err != nil {
				return CandidatePreimage{}, "", err
			}
			if file.Mode == "directory" {
				if file.Hash != "" {
					return CandidatePreimage{}, "", validationError{"directory must not carry a content hash"}
				}
			} else if (file.Mode != "file" && file.Mode != "executable") || !validDigest(file.Hash) {
				return CandidatePreimage{}, "", validationError{"invalid candidate file identity"}
			}
			paths[j] = file.Path
		}
		if duplicate(paths) {
			return CandidatePreimage{}, "", validationError{"duplicate candidate file path"}
		}
	}
	if candidate.Components[0].Path == candidate.Components[1].Path {
		return CandidatePreimage{}, "", validationError{"duplicate component path"}
	}
	sort.Slice(candidate.Tools, func(i, j int) bool { return candidate.Tools[i].ID < candidate.Tools[j].ID })
	toolIDs := make([]string, len(candidate.Tools))
	for i := range candidate.Tools {
		tool := &candidate.Tools[i]
		toolIDs[i] = tool.ID
		locks := append([]ToolLock(nil), tool.Locks...)
		sort.Slice(locks, func(i, j int) bool {
			return locks[i].Name+"\x00"+locks[i].Value < locks[j].Name+"\x00"+locks[j].Value
		})
		tool.Locks = locks
		if err := validateTool(Tool{ID: tool.ID, Version: tool.Version, Locks: tool.Locks}); err != nil {
			return CandidatePreimage{}, "", err
		}
	}
	if duplicate(toolIDs) {
		return CandidatePreimage{}, "", validationError{"duplicate candidate tool identity"}
	}
	if err := canonicalStrings(&candidate.HomeSources, validatePath); err != nil {
		return CandidatePreimage{}, "", err
	}
	if err := canonicalStrings(&candidate.Targets, validatePath); err != nil {
		return CandidatePreimage{}, "", err
	}
	if err := canonicalExecutables(&candidate.Executables); err != nil {
		return CandidatePreimage{}, "", err
	}
	if candidate.Parser != proofParser() {
		return CandidatePreimage{}, "", validationError{"unapproved parser identity"}
	}
	if err := canonicalPolicy(&candidate.Policy); err != nil {
		return CandidatePreimage{}, "", err
	}
	stringCount, allocated := candidateCost(candidate)
	if stringCount > MaxTotalStrings || allocated > MaxAllocatedBytes {
		return CandidatePreimage{}, "", validationError{"candidate aggregate string or allocation bound exceeded"}
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return CandidatePreimage{}, "", err
	}
	sum := sha256.Sum256(encoded)
	return candidate, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateCanonicalSource(source CanonicalSource) error {
	expected, err := CanonicalSourceID(source.URI)
	if err != nil || source.SourceID != expected || (len(source.Commit) != 40 && len(source.Commit) != 64) ||
		!objectIDRE.MatchString(source.Commit) || !validDigest(source.RootTree) {
		return validationError{"invalid canonical source identity"}
	}
	return nil
}

func proofParser() CanonicalParser {
	return CanonicalParser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"}
}

func canonicalExecutables(executables *[]CanonicalExecutable) error {
	sort.Slice(*executables, func(i, j int) bool { return (*executables)[i].ID < (*executables)[j].ID })
	want := []string{"bash", "chezmoi", "git", "mise"}
	ids := make([]string, len(*executables))
	for i, executable := range *executables {
		ids[i] = executable.ID
		if executable.Version == "" || len(executable.Version) > 128 || executable.Path == "" ||
			!path.IsAbs(executable.Path) || path.Clean(executable.Path) != executable.Path ||
			strings.Contains(executable.Path, "\\") || !validDigest(executable.SHA256) {
			return validationError{"invalid managed executable identity"}
		}
	}
	if !equalStrings(ids, want) {
		return validationError{"candidate must bind the closed managed executable set"}
	}
	return nil
}

func canonicalPolicy(policy *CanonicalPolicy) error {
	if policy.Schema != 1 || policy.Domain != "zerb.policy" || policy.TargetMapperExecutableID != "chezmoi" ||
		!policy.ProviderFree || !policy.HostFactIndependent || !policy.DestinationRecheck || !policy.ExactReapplyOnly {
		return validationError{"invalid closed candidate policy"}
	}
	checks := []struct {
		values *[]string
		want   []string
	}{
		{&policy.ObjectModes, []string{"directory", "executable", "file"}},
		{&policy.Declarations, []string{"home", "mise.lock", "mise.toml", "profile.toml"}},
		{&policy.TemplateActions, []string{"if-false", "if-true", "literal-text"}},
		{&policy.ProcessClasses, []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}},
		{&policy.PolicyIDs, []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}},
	}
	for _, check := range checks {
		sort.Strings(*check.values)
		if !equalStrings(*check.values, check.want) {
			return validationError{"candidate policy allowlist is not the closed Phase 1 set"}
		}
	}
	return nil
}

func canonicalStrings(values *[]string, validate func(string) error) error {
	sort.Strings(*values)
	if duplicate(*values) {
		return validationError{"duplicate canonical string"}
	}
	for _, value := range *values {
		if err := validate(value); err != nil {
			return err
		}
	}
	return nil
}

func equalStrings(a, b []string) bool {
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

func validDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && hexRE.MatchString(strings.TrimPrefix(value, "sha256:"))
}

func componentCost(component Component) (int, int) {
	count, size := 4, len(component.ID)+len(component.Path)+len(component.TreeDigest)+len(component.Manifest.Label)
	for _, value := range component.HomeSources {
		count++
		size += len(value)
	}
	for _, value := range component.Targets {
		count++
		size += len(value)
	}
	for _, file := range component.Files {
		count += 3
		size += len(file.Path) + len(file.Mode) + len(file.Hash)
	}
	for _, tool := range component.Tools {
		count += 2
		size += len(tool.ID) + len(tool.Version)
		for _, lock := range tool.Locks {
			count += 2
			size += len(lock.Name) + len(lock.Value)
		}
	}
	return count, size
}

func candidateCost(candidate CandidatePreimage) (int, int) {
	count, size := 3, len(candidate.Domain)+len(candidate.ProfileID)
	sourceValues := []string{candidate.Source.URI, candidate.Source.SourceID, candidate.Source.Commit, candidate.Source.RootTree}
	count += len(sourceValues)
	for _, value := range sourceValues {
		size += len(value)
	}
	for _, component := range candidate.Components {
		count += 3
		size += len(component.ID) + len(component.Path) + len(component.TreeDigest)
		for _, file := range component.Files {
			count += 3
			size += len(file.Path) + len(file.Mode) + len(file.Hash)
		}
	}
	for _, tool := range candidate.Tools {
		count += 2
		size += len(tool.ID) + len(tool.Version)
		for _, lock := range tool.Locks {
			count += 2
			size += len(lock.Name) + len(lock.Value)
		}
	}
	for _, value := range candidate.HomeSources {
		count++
		size += len(value)
	}
	for _, value := range candidate.Targets {
		count++
		size += len(value)
	}
	for _, executable := range candidate.Executables {
		count += 4
		size += len(executable.ID) + len(executable.Version) + len(executable.Path) + len(executable.SHA256)
	}
	parserValues := []string{candidate.Parser.Module, candidate.Parser.Version, candidate.Parser.ModuleSum, candidate.Parser.GoModSum, candidate.Parser.OriginCommit}
	count += len(parserValues)
	for _, value := range parserValues {
		size += len(value)
	}
	policyValues := [][]string{candidate.Policy.ObjectModes, candidate.Policy.Declarations, candidate.Policy.TemplateActions, candidate.Policy.ProcessClasses, candidate.Policy.PolicyIDs}
	count += 2
	size += len(candidate.Policy.Domain) + len(candidate.Policy.TargetMapperExecutableID)
	for _, values := range policyValues {
		count += len(values)
		for _, value := range values {
			size += len(value)
		}
	}
	return count, size
}

func sortedFiles(in []CanonicalFile) []CanonicalFile {
	out := append([]CanonicalFile(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
func duplicate(in []string) bool {
	out := append([]string(nil), in...)
	sort.Strings(out)
	for i := 1; i < len(out); i++ {
		if out[i] == out[i-1] {
			return true
		}
	}
	return false
}

func EncodeCandidate(candidate CandidatePreimage) ([]byte, error) {
	normalized, _, err := DigestCandidate(candidate)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func CanonicalExecutableFromBytes(id, version, executablePath string, data []byte) (CanonicalExecutable, error) {
	if id == "" || version == "" || !path.IsAbs(executablePath) || path.Clean(executablePath) != executablePath || strings.Contains(executablePath, "\\") {
		return CanonicalExecutable{}, validationError{"invalid executable identity fields"}
	}
	sum := sha256.Sum256(data)
	return CanonicalExecutable{ID: id, Version: version, Path: executablePath, SHA256: "sha256:" + hex.EncodeToString(sum[:])}, nil
}

type ParsedInvocation struct {
	Command      string
	Source       string
	ProfileID    string
	Candidate    string
	Trust        string
	Yes          bool
	Payload      []string
	ShellID      string
	RequiresRoot bool
}

func ParseInvocation(argv []string) (ParsedInvocation, error) {
	if len(argv) == 1 && (argv[0] == "version" || argv[0] == "--version") {
		return ParsedInvocation{Command: "version"}, nil
	}
	if len(argv) == 0 {
		return ParsedInvocation{}, validationError{"missing command"}
	}
	parsed := ParsedInvocation{Command: argv[0], RequiresRoot: true}
	switch argv[0] {
	case "profile":
		if len(argv) != 3 || argv[1] != "list" || ValidateSource(argv[2]) != nil {
			return ParsedInvocation{}, validationError{"usage: profile list source"}
		}
		parsed.Source = argv[2]
	case "plan", "bootstrap":
		if len(argv) < 2 || ValidateSource(argv[1]) != nil {
			return ParsedInvocation{}, validationError{"invalid source"}
		}
		parsed.Source = argv[1]
		fs := flag.NewFlagSet(argv[0], flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.StringVar(&parsed.ProfileID, "profile", "", "")
		fs.StringVar(&parsed.Trust, "trust-candidate", "", "")
		if argv[0] == "bootstrap" {
			fs.BoolVar(&parsed.Yes, "yes", false, "")
		}
		if !exactFlagSpelling(argv[2:], "--profile", "--trust-candidate", "--yes") || repeatedSingleton(argv[2:], "--profile", "--trust-candidate", "--yes") || fs.Parse(argv[2:]) != nil || fs.NArg() != 0 || !profileIDRE.MatchString(parsed.ProfileID) || parsed.Trust != "" && !validDigest(parsed.Trust) {
			return ParsedInvocation{}, validationError{"invalid plan/bootstrap grammar"}
		}
	case "apply":
		fs := flag.NewFlagSet("apply", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.StringVar(&parsed.Candidate, "candidate", "", "")
		fs.BoolVar(&parsed.Yes, "yes", false, "")
		if !exactFlagSpelling(argv[1:], "--candidate", "--yes") || repeatedSingleton(argv[1:], "--candidate", "--yes") || fs.Parse(argv[1:]) != nil || fs.NArg() != 0 || !validDigest(parsed.Candidate) {
			return ParsedInvocation{}, validationError{"invalid apply grammar"}
		}
	case "doctor":
		if len(argv) != 1 {
			return ParsedInvocation{}, validationError{"doctor takes no operands"}
		}
	case "recover":
		if len(argv) != 2 || argv[1] != "resume" && argv[1] != "repair" {
			return ParsedInvocation{}, validationError{"invalid recovery grammar"}
		}
	case "exec":
		if len(argv) < 3 || argv[1] != "--" || !validCommand(argv[2]) {
			return ParsedInvocation{}, validationError{"invalid exec grammar"}
		}
		parsed.Payload = append([]string(nil), argv[2:]...)
	case "shell":
		fs := flag.NewFlagSet("shell", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.StringVar(&parsed.ShellID, "shell", "", "")
		if !exactFlagSpelling(argv[1:], "--shell") || repeatedSingleton(argv[1:], "--shell") || fs.Parse(argv[1:]) != nil || fs.NArg() != 0 || parsed.ShellID != "bash" {
			return ParsedInvocation{}, validationError{"invalid shell grammar"}
		}
	default:
		return ParsedInvocation{}, validationError{"unknown command"}
	}
	return parsed, nil
}

func exactFlagSpelling(args []string, names ...string) bool {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && (!allowed[arg] || strings.Contains(arg, "=")) {
			return false
		}
	}
	return true
}

func repeatedSingleton(args []string, names ...string) bool {
	for _, name := range names {
		count := 0
		for _, arg := range args {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				count++
			}
		}
		if count > 1 {
			return true
		}
	}
	return false
}

func validCommand(command string) bool {
	if command == "" {
		return false
	}
	if !strings.Contains(command, "/") {
		return command != "." && command != ".." && !strings.HasPrefix(command, "-")
	}
	return path.IsAbs(command) && path.Clean(command) == command && !strings.Contains(command, "\\")
}

func ValidateCompleteTree(component Component, expected []CanonicalFile) error {
	if err := ValidateComponent(component); err != nil {
		return err
	}
	if len(component.Files) != len(expected) {
		return validationError{"tree inventory is incomplete"}
	}
	actual := sortedFiles(component.Files)
	want := sortedFiles(expected)
	for i := range actual {
		if actual[i] != want[i] {
			return validationError{"tree inventory entry mismatch"}
		}
	}
	return nil
}

func CheckCandidateApproval(candidate CandidatePreimage, presented string) error {
	_, digest, err := DigestCandidate(candidate)
	if err != nil {
		return err
	}
	if !validDigest(presented) || presented != digest {
		return validationError{"candidate approval mismatch"}
	}
	return nil
}

func ValidateTemplate(name string, data []byte) error {
	if len(data) > MaxTemplateBytes || !utf8.Valid(data) {
		return validationError{"invalid or oversized template"}
	}
	text := string(data)
	actions := 0
	depth := 0
	maxDepth := 0
	for rest := text; ; {
		a := strings.Index(rest, "{{")
		if a < 0 {
			break
		}
		b := strings.Index(rest[a+2:], "}}")
		if b < 0 {
			return validationError{"unterminated template action"}
		}
		token := strings.TrimSpace(rest[a+2 : a+2+b])
		actions++
		switch token {
		case "if true", "if false":
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		case "else":
			if depth == 0 {
				return validationError{"else without if"}
			}
		case "end":
			depth--
			if depth < 0 {
				return validationError{"end without if"}
			}
		default:
			return validationError{"unapproved template action"}
		}
		rest = rest[a+2+b+2:]
	}
	if depth != 0 || actions > MaxTemplateActions || maxDepth > MaxTemplateDepth {
		return validationError{"template action/depth bound exceeded"}
	}
	t, err := parse.Parse(name, text, "{{", "}}", nil)
	if err != nil {
		return err
	}
	for _, tree := range t {
		if err = validateNode(tree.Root, 0); err != nil {
			return err
		}
	}
	return nil
}
func validateNode(n parse.Node, depth int) error {
	if depth > MaxTemplateDepth {
		return validationError{"template AST depth exceeded"}
	}
	switch x := n.(type) {
	case *parse.ListNode:
		for _, c := range x.Nodes {
			if err := validateNode(c, depth); err != nil {
				return err
			}
		}
	case *parse.TextNode:
	case *parse.IfNode:
		if x.Pipe == nil || len(x.Pipe.Cmds) != 1 || len(x.Pipe.Cmds[0].Args) != 1 {
			return validationError{"if condition must be literal boolean"}
		}
		if _, ok := x.Pipe.Cmds[0].Args[0].(*parse.BoolNode); !ok {
			return validationError{"if condition must be literal boolean"}
		}
		if err := validateNode(x.List, depth+1); err != nil {
			return err
		}
		if x.ElseList != nil {
			return validateNode(x.ElseList, depth+1)
		}
	default:
		return validationError{"unapproved template AST node"}
	}
	return nil
}
