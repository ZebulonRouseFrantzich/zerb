package state

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// These ordered-value checks do not authenticate source membership or trees.
// The decoder compares its raw tree hashes; the encoder hashes reserved buffers.
func codecValidateCandidate(v receipt.Candidate) bool {
	if v.Schema != 1 || v.Domain != "zerb.candidate" || !codecValidSource(v.Source) ||
		!codecValidID(v.ProfileID) || v.ProfileID == "base" || !codecValidParser(v.Parser) ||
		!codecValidComponents(v.Components, v.ProfileID) || !codecValidPaths(v.HomeSources) ||
		!codecValidPaths(v.Targets) || !codecValidPolicy(v.Policy) || len(v.Tools) > 1 || len(v.Executables) != 4 {
		return false
	}
	for _, tool := range v.Tools {
		if !codecValidTool(tool) {
			return false
		}
	}
	for i, id := range [...]string{"bash", "chezmoi", "git", "mise"} {
		if v.Executables[i].ID != id || !codecValidExecutable(v.Executables[i]) {
			return false
		}
	}
	count := uint64(6 + len(v.HomeSources) + len(v.Targets) + 4*len(v.Executables) + 5 + 2 +
		len(v.Policy.ObjectModes) + len(v.Policy.Declarations) + len(v.Policy.TemplateActions) + len(v.Policy.ProcessClasses) + len(v.Policy.PolicyIDs))
	for _, component := range v.Components {
		count += uint64(3 + 3*len(component.Files))
	}
	for _, tool := range v.Tools {
		count += uint64(2 + 2*len(tool.Locks))
	}
	return count <= 16384
}

func codecValidateCompleted(v receipt.Completed) bool {
	if v.Schema != 1 || v.Domain != "zerb.completed" || !codecValidDigest(string(v.Candidate)) ||
		v.Previous != "" && !codecValidDigest(string(v.Previous)) || !codecValidSource(v.Source) ||
		!codecValidParser(v.Parser) || !codecValidComponents(v.Components, "") || !codecValidPolicy(v.Policy) ||
		!codecValidPlatform(v.Platform) || !codecValidPaths(v.Targets) ||
		!codecValidObservations(v.Preconditions, v.Targets) || !codecValidObservations(v.Results, v.Targets) || len(v.Tools) > 1 {
		return false
	}
	if v.Zerb.ID != "zerb" || !codecValidExecutable(v.Zerb) || v.Git.ID != "git" || !codecValidExecutable(v.Git) ||
		v.Mise.ID != "mise" || !codecValidExecutable(v.Mise) || v.Chezmoi.ID != "chezmoi" || !codecValidExecutable(v.Chezmoi) {
		return false
	}
	for _, tool := range v.Tools {
		if tool.ID != "node" || tool.Requested != "24.4.1" || tool.Installed != tool.Requested || !codecValidDigest(tool.Integrity) {
			return false
		}
	}
	for i, phase := range v.Phases {
		if phase != string(PhaseToolInstall) && phase != string(PhaseHomeApply) || i > 0 && v.Phases[i-1] >= phase {
			return false
		}
	}
	return true
}

func codecValidComponents(v []receipt.Component, selected string) bool {
	if len(v) != 2 {
		return false
	}
	base, profile := false, false
	for i, component := range v {
		if !codecValidID(component.ID) || !codecValidPath(component.Path) || !codecValidDigest(component.TreeDigest) ||
			len(component.Files) > 4096 || i > 0 && (v[i-1].ID >= component.ID || v[i-1].Path == component.Path) {
			return false
		}
		if component.ID == "base" {
			base = true
		} else if selected == "" || component.ID == selected {
			profile = true
		} else {
			return false
		}
		for j, file := range component.Files {
			if !codecValidPath(file.Path) || j > 0 && component.Files[j-1].Path >= file.Path {
				return false
			}
			switch file.Mode {
			case "directory":
				if file.Hash != "" {
					return false
				}
			case "file", "executable":
				if !codecValidDigest(file.Hash) {
					return false
				}
			default:
				return false
			}
		}
	}
	return base && profile
}

func codecValidPaths(paths []string) bool {
	if len(paths) > 4096 {
		return false
	}
	for i, path := range paths {
		if !codecValidPath(path) || i > 0 && paths[i-1] >= path {
			return false
		}
	}
	return true
}

func codecValidPolicy(v receipt.Policy) bool {
	return v.Schema == 1 && v.Domain == "zerb.policy" && v.TargetMapperExecutableID == "chezmoi" &&
		v.ProviderFree && v.HostFactIndependent && v.DestinationRecheck && v.ExactReapplyOnly &&
		slices.Equal(v.ObjectModes, []string{"directory", "executable", "file"}) &&
		slices.Equal(v.Declarations, []string{"home", "mise.lock", "mise.toml", "profile.toml"}) &&
		slices.Equal(v.TemplateActions, []string{"if-false", "if-true", "literal-text"}) &&
		slices.Equal(v.ProcessClasses, []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}) &&
		slices.Equal(v.PolicyIDs, []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"})
}

func codecValidObservation(v receipt.DestinationObservation) bool {
	if !codecValidPath(v.Target) || v.Mode > 07777 {
		return false
	}
	if !v.Exists {
		return v.Kind == "absent" && v.Mode == 0 && v.Owner == 0 && v.Device == 0 && v.Inode == 0 && v.Links == 0 && v.ContentHash == ""
	}
	return v.Kind == "file" && codecValidDigest(v.ContentHash) || v.Kind == "directory" && v.ContentHash == ""
}

func codecValidObservations(observed []receipt.DestinationObservation, targets []string) bool {
	if len(observed) != len(targets) {
		return false
	}
	for i, observation := range observed {
		if observation.Target != targets[i] || !codecValidObservation(observation) {
			return false
		}
	}
	return true
}

func codecTreeDigestMatches(digest string, hash [sha256.Size]byte) bool {
	if !codecValidDigest(digest) {
		return false
	}
	const digits = "0123456789abcdef"
	for i, value := range hash {
		if digest[7+2*i] != digits[value>>4] || digest[8+2*i] != digits[value&15] {
			return false
		}
	}
	return true
}

// This scalar codec does not infer owner history, predecessor generation or
// token relations beyond the declared domains; the complete graph owns them.
func codecValidateJournal(v journalRecord) bool {
	if v.Schema != 1 || v.Domain != "zerb.journal" || !replacementHex(v.Instance, 32) ||
		!replacementHex(v.OperationID, 32) || !replacementHex(v.OperationToken, 32) || !replacementHex(v.WriterToken, 32) ||
		v.WriterGeneration == 0 || v.OperationID == v.OperationToken || v.OperationID == v.WriterToken || v.OperationToken == v.WriterToken ||
		(v.Previous == "") != (v.PreviousReceipt == "") || !codecValidDigest(string(v.Candidate)) ||
		v.Previous != "" && (!codecValidDigest(string(v.Previous)) || !codecValidDigest(string(v.PreviousReceipt))) ||
		!validOperationPrefix(v.Phase, v.CompletedPhases) || len(v.Preconditions) > 4096 || len(v.Preconditions) != len(v.Results) {
		return false
	}
	if v.WriterPreviousToken != "" && (!replacementHex(v.WriterPreviousToken, 32) || v.WriterPreviousToken == v.WriterToken || v.WriterPreviousToken == v.OperationToken) {
		return false
	}
	for i, observed := range v.Preconditions {
		if !codecValidObservation(observed) || !codecValidObservation(v.Results[i]) || observed.Target != v.Results[i].Target ||
			i > 0 && v.Preconditions[i-1].Target >= observed.Target {
			return false
		}
	}
	return true
}

func codecDigestFromHash(hash [sha256.Size]byte) receipt.Digest {
	var token [71]byte
	copy(token[:], "sha256:")
	hex.Encode(token[7:], hash[:])
	return receipt.Digest(token[:])
}

func codecValidDigest(s string) bool { _, err := receipt.ParseDigest(s); return err == nil }
func codecValidID(s string) bool {
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
func codecValidPath(s string) bool {
	if len(s) == 0 || len(s) > 1024 || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	depth := 0
	start := 0
	for end := 0; end <= len(s); end++ {
		if end < len(s) && s[end] != '/' {
			continue
		}
		part := s[start:end]
		depth++
		if part == "" || part == "." || part == ".." || len(part) > 255 || depth > 64 {
			return false
		}
		start = end + 1
	}
	return true
}
func codecValidAbsolute(s string) bool {
	if len(s) < 2 || s[0] != '/' || !utf8.ValidString(s) || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	start := 1
	for end := 1; end <= len(s); end++ {
		if end < len(s) && s[end] != '/' {
			continue
		}
		part := s[start:end]
		if part == "" || part == "." || part == ".." {
			return false
		}
		start = end + 1
	}
	return true
}
func codecValidTool(v receipt.Tool) bool {
	return v.ID == "node" && v.Version == "24.4.1" && len(v.Locks) == 1 && v.Locks[0].Name == "artifact" && codecValidDigest(v.Locks[0].Value)
}
func codecValidExecutable(v receipt.Executable) bool {
	if !codecValidAbsolute(v.Path) || !codecValidDigest(v.SHA256) || v.Version == "" || !utf8.ValidString(v.Version) || strings.ContainsRune(v.Version, 0) {
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
func codecValidParser(v receipt.Parser) bool {
	return v == (receipt.Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"})
}
func codecValidPlatform(v receipt.PlatformObservation) bool {
	for _, s := range []string{v.Distribution, v.Kernel, v.Architecture, v.Filesystem} {
		if s == "" || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}
