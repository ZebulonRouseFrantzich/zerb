package receipt

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

var errCanonical = errors.New("invalid canonical receipt value")

// CandidateDigest canonicalizes a private copy under PC-050/ARCH-030. It
// validates facts and syntax; acquisition and approval remain service authority.
func CandidateDigest(v Candidate) (Digest, []byte, error) {
	var budget reservation
	if !budget.add(1, 2048) || !budget.reserveCandidate(v, true) || !reserveSourceScratch(&budget, v.Source.URI) {
		return "", nil, errCanonical
	}
	var size jsonWriter
	size.candidate(v)
	if size.failed || size.strings > 16384 || !budget.add(size.size, 1) || !reserveTrees(&budget, v.Components) {
		return "", nil, errCanonical
	}
	if v.Schema != 1 || v.Domain != "zerb.candidate" || !validSource(v.Source) || !validID(v.ProfileID) || v.ProfileID == "base" || !validParser(v.Parser) {
		return "", nil, errCanonical
	}
	var ok bool
	v.Components, ok = canonicalComponents(v.Components, v.ProfileID)
	if !ok {
		return "", nil, errCanonical
	}
	v.Tools = copySlice(v.Tools)
	if len(v.Tools) > 1 {
		return "", nil, errCanonical
	}
	for i := range v.Tools {
		v.Tools[i].Locks = copySlice(v.Tools[i].Locks)
		if !validTool(v.Tools[i]) {
			return "", nil, errCanonical
		}
	}
	v.HomeSources, ok = canonicalPaths(v.HomeSources)
	if !ok {
		return "", nil, errCanonical
	}
	v.Targets, ok = canonicalPaths(v.Targets)
	if !ok {
		return "", nil, errCanonical
	}
	v.Executables = copySlice(v.Executables)
	slices.SortFunc(v.Executables, func(a, b Executable) int { return strings.Compare(a.ID, b.ID) })
	if len(v.Executables) != 4 {
		return "", nil, errCanonical
	}
	for i, id := range []string{"bash", "chezmoi", "git", "mise"} {
		if v.Executables[i].ID != id || !validExecutable(v.Executables[i]) {
			return "", nil, errCanonical
		}
	}
	v.Policy, ok = canonicalPolicy(v.Policy)
	if !ok {
		return "", nil, errCanonical
	}
	out := jsonWriter{out: make([]byte, 0, int(size.size)), writing: true}
	out.candidate(v)
	if out.failed || out.size != size.size {
		return "", nil, errCanonical
	}
	return hashBytes(out.out), out.out, nil
}

// ReceiptDigest encodes completed operation facts. Current-record authority and
// installed-tree authentication cannot be established by this pure boundary.
func ReceiptDigest(v Completed) (Digest, []byte, error) {
	var budget reservation
	if !budget.add(1, 4096) || !budget.reserveCompleted(v, true) || !reserveSourceScratch(&budget, v.Source.URI) {
		return "", nil, errCanonical
	}
	var size jsonWriter
	size.completed(v)
	if size.failed || !budget.add(size.size, 1) || !reserveTrees(&budget, v.Components) {
		return "", nil, errCanonical
	}
	if v.Schema != 1 || v.Domain != "zerb.completed" || !validDigest(string(v.Candidate)) || v.Previous != "" && !validDigest(string(v.Previous)) || !validSource(v.Source) || !validParser(v.Parser) {
		return "", nil, errCanonical
	}
	var ok bool
	v.Components, ok = canonicalComponents(v.Components, "")
	if !ok {
		return "", nil, errCanonical
	}
	if v.Zerb.ID != "zerb" || !validExecutable(v.Zerb) || v.Git.ID != "git" || !validExecutable(v.Git) || v.Mise.ID != "mise" || !validExecutable(v.Mise) || v.Chezmoi.ID != "chezmoi" || !validExecutable(v.Chezmoi) {
		return "", nil, errCanonical
	}
	v.Policy, ok = canonicalPolicy(v.Policy)
	if !ok || !validPlatform(v.Platform) {
		return "", nil, errCanonical
	}
	v.Tools = copySlice(v.Tools)
	if len(v.Tools) > 1 {
		return "", nil, errCanonical
	}
	for _, tool := range v.Tools {
		if tool.ID != "node" || tool.Requested != "24.4.1" || tool.Installed != tool.Requested || !validDigest(tool.Integrity) {
			return "", nil, errCanonical
		}
	}
	v.Targets, ok = canonicalPaths(v.Targets)
	if !ok {
		return "", nil, errCanonical
	}
	v.Preconditions, ok = canonicalObservations(v.Preconditions, v.Targets)
	if !ok {
		return "", nil, errCanonical
	}
	v.Results, ok = canonicalObservations(v.Results, v.Targets)
	if !ok {
		return "", nil, errCanonical
	}
	v.Phases = copySlice(v.Phases)
	slices.Sort(v.Phases)
	for i, phase := range v.Phases {
		if i > 0 && phase == v.Phases[i-1] {
			return "", nil, errCanonical
		}
		switch phase {
		case "mutating-tool-install", "mutating-home-apply":
		default:
			return "", nil, errCanonical
		}
	}
	out := jsonWriter{out: make([]byte, 0, int(size.size)), writing: true}
	out.completed(v)
	if out.failed || out.size != size.size {
		return "", nil, errCanonical
	}
	return hashBytes(out.out), out.out, nil
}

func reserveTrees(b *reservation, components []Component) bool {
	for _, c := range components {
		var size jsonWriter
		size.tree(c.Files)
		if size.failed || !b.add(size.size, 1) {
			return false
		}
	}
	return true
}

func canonicalComponents(input []Component, selected string) ([]Component, bool) {
	if len(input) != 2 {
		return nil, false
	}
	out := copySlice(input)
	slices.SortFunc(out, func(a, b Component) int { return strings.Compare(a.ID, b.ID) })
	base, profile := false, false
	for i := range out {
		c := &out[i]
		if !validID(c.ID) || !validPath(c.Path) || !validDigest(c.TreeDigest) || len(c.Files) > 4096 {
			return nil, false
		}
		if c.ID == "base" {
			base = true
		} else if selected == "" || c.ID == selected {
			profile = true
		} else {
			return nil, false
		}
		if i > 0 && (c.ID == out[i-1].ID || c.Path == out[i-1].Path) {
			return nil, false
		}
		c.Files = copySlice(c.Files)
		slices.SortFunc(c.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
		for j, f := range c.Files {
			if !validPath(f.Path) || j > 0 && f.Path == c.Files[j-1].Path {
				return nil, false
			}
			switch f.Mode {
			case "directory":
				if f.Hash != "" {
					return nil, false
				}
			case "file", "executable":
				if !validDigest(f.Hash) {
					return nil, false
				}
			default:
				return nil, false
			}
		}
		var size jsonWriter
		size.tree(c.Files)
		encoded := jsonWriter{out: make([]byte, 0, int(size.size)), writing: true}
		encoded.tree(c.Files)
		if encoded.failed || string(hashBytes(encoded.out)) != c.TreeDigest {
			return nil, false
		}
	}
	return out, base && profile
}

func copySlice[T any](input []T) []T { out := make([]T, len(input)); copy(out, input); return out }

func canonicalPaths(input []string) ([]string, bool) {
	if len(input) > 4096 {
		return nil, false
	}
	out := copySlice(input)
	slices.Sort(out)
	for i, p := range out {
		if !validPath(p) || i > 0 && p == out[i-1] {
			return nil, false
		}
	}
	return out, true
}

func canonicalPolicy(v Policy) (Policy, bool) {
	if v.Schema != 1 || v.Domain != "zerb.policy" || v.TargetMapperExecutableID != "chezmoi" || !v.ProviderFree || !v.HostFactIndependent || !v.DestinationRecheck || !v.ExactReapplyOnly {
		return Policy{}, false
	}
	checks := []struct {
		field *[]string
		want  []string
	}{
		{&v.ObjectModes, []string{"directory", "executable", "file"}},
		{&v.Declarations, []string{"home", "mise.lock", "mise.toml", "profile.toml"}},
		{&v.TemplateActions, []string{"if-false", "if-true", "literal-text"}},
		{&v.ProcessClasses, []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}},
		{&v.PolicyIDs, []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}},
	}
	for _, check := range checks {
		out := copySlice(*check.field)
		slices.Sort(out)
		if !slices.Equal(out, check.want) {
			return Policy{}, false
		}
		*check.field = out
	}
	return v, true
}

func canonicalObservations(input []DestinationObservation, targets []string) ([]DestinationObservation, bool) {
	if len(input) != len(targets) {
		return nil, false
	}
	out := copySlice(input)
	slices.SortFunc(out, func(a, b DestinationObservation) int { return strings.Compare(a.Target, b.Target) })
	for i, o := range out {
		if o.Target != targets[i] || o.Mode > 07777 {
			return nil, false
		}
		if !o.Exists {
			if o.Kind != "absent" || o.Mode != 0 || o.Owner != 0 || o.Device != 0 || o.Inode != 0 || o.Links != 0 || o.ContentHash != "" {
				return nil, false
			}
		} else {
			switch o.Kind {
			case "file":
				if !validDigest(o.ContentHash) {
					return nil, false
				}
			case "directory":
				if o.ContentHash != "" {
					return nil, false
				}
			default:
				return nil, false
			}
		}
	}
	return out, true
}

func validDigest(s string) bool { _, err := ParseDigest(s); return err == nil }
func validID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-') {
			continue
		}
		return false
	}
	return true
}
func validPath(s string) bool {
	if len(s) == 0 || len(s) > 1024 || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	depth := 0
	for part := range strings.SplitSeq(s, "/") {
		depth++
		if part == "" || part == "." || part == ".." || len(part) > 255 || depth > 64 {
			return false
		}
	}
	return true
}
func validAbsolute(s string) bool {
	if len(s) < 2 || s[0] != '/' || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	for part := range strings.SplitSeq(s[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func validTool(v Tool) bool {
	return v.ID == "node" && v.Version == "24.4.1" && len(v.Locks) == 1 && v.Locks[0].Name == "artifact" && validDigest(v.Locks[0].Value)
}
func validExecutable(v Executable) bool {
	if !validAbsolute(v.Path) || !validDigest(v.SHA256) || v.Version == "" || !utf8.ValidString(v.Version) || strings.ContainsRune(v.Version, 0) {
		return false
	}
	switch v.ID {
	case "git":
		return v.Version == "2.55.0"
	case "bash":
		return v.Version == "5.3.15"
	case "mise":
		return v.Version == "2026.7.17"
	case "chezmoi":
		return v.Version == "v2.70.5-zerb.1"
	case "zerb":
		return true
	}
	return false
}
func validParser(v Parser) bool {
	return v == (Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"})
}
func validPlatform(v PlatformObservation) bool {
	for _, s := range []string{v.Distribution, v.Kernel, v.Architecture, v.Filesystem} {
		if s == "" || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}
