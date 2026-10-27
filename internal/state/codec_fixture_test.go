package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

func codecTestHash(data []byte) receipt.Digest {
	sum := sha256.Sum256(data)
	return receipt.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func codecTestToken() string { return "sha256:" + strings.Repeat("0", 64) }

// Synthetic scalar fixtures carry no source, executable, host or completion authority.
func codecTestCandidate(t testing.TB) receipt.Candidate {
	t.Helper()
	c := receipt.Candidate{Schema: 1, Domain: "zerb.candidate", ProfileID: "work",
		Source:     receipt.Source{URI: "https://example.com/repo.git", SourceID: codecTestToken(), Commit: strings.Repeat("b", 40), RootTree: strings.Repeat("c", 40)},
		Parser:     receipt.Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"},
		Policy:     receipt.Policy{Schema: 1, Domain: "zerb.policy", ObjectModes: []string{"directory", "executable", "file"}, Declarations: []string{"home", "mise.lock", "mise.toml", "profile.toml"}, TemplateActions: []string{"if-false", "if-true", "literal-text"}, ProcessClasses: []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}, PolicyIDs: []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}, TargetMapperExecutableID: "chezmoi", ProviderFree: true, HostFactIndependent: true, DestinationRecheck: true, ExactReapplyOnly: true},
		Components: []receipt.Component{{ID: "base", Path: "base", Files: []receipt.File{}}, {ID: "work", Path: "profiles/work", Files: []receipt.File{{Path: "quote\"<&\u2028/雪", Mode: "file", Hash: codecTestToken()}}}},
		Tools:      []receipt.Tool{}, HomeSources: []string{}, Targets: []string{}, Executables: []receipt.Executable{},
	}
	for i := range c.Components {
		data, err := json.Marshal(struct {
			Schema int            `json:"schema"`
			Domain string         `json:"domain"`
			Files  []receipt.File `json:"files"`
		}{1, "zerb.tree.v1", c.Components[i].Files})
		if err != nil {
			t.Fatal(err)
		}
		c.Components[i].TreeDigest = string(codecTestHash(data))
	}
	for i, id := range []string{"bash", "chezmoi", "git", "mise"} {
		c.Executables = append(c.Executables, receipt.Executable{ID: id, Version: []string{"5.3.15", "v2.70.5-zerb.1", "2.55.0", "2026.7.17"}[i], Path: "/opt/" + id, SHA256: codecTestToken()})
	}
	if _, _, err := receipt.CandidateDigest(c); err != nil {
		t.Fatal("invalid fixture", err)
	}
	return c
}

func codecTestCompleted(t testing.TB) receipt.Completed {
	t.Helper()
	c := codecTestCandidate(t)
	return receipt.Completed{Schema: 1, Domain: "zerb.completed", Candidate: receipt.Digest(codecTestToken()), Source: c.Source, Components: c.Components,
		Zerb: receipt.Executable{ID: "zerb", Version: "test", Path: "/opt/zerb", SHA256: codecTestToken()}, Git: c.Executables[2], Mise: c.Executables[3], Chezmoi: c.Executables[1], Parser: c.Parser, Policy: c.Policy,
		Platform: receipt.PlatformObservation{Distribution: "test", Kernel: "test", Architecture: "test", Filesystem: "test"}, Tools: []receipt.ToolObservation{}, Targets: []string{}, Preconditions: []receipt.DestinationObservation{}, Results: []receipt.DestinationObservation{}, Phases: []string{"mutating-home-apply", "mutating-tool-install"}}
}

// Tighten only the oracle's incidental decoder capacities. The wire has no Go
// capacity history; the approved decoder compares the old minimum-capacity gate.
func codecTightPolicy(p receipt.Policy) receipt.Policy {
	p.ObjectModes = append([]string{}, p.ObjectModes...)
	p.Declarations = append([]string{}, p.Declarations...)
	p.TemplateActions = append([]string{}, p.TemplateActions...)
	p.ProcessClasses = append([]string{}, p.ProcessClasses...)
	p.PolicyIDs = append([]string{}, p.PolicyIDs...)
	// append can round capacity. The full-slice expression is intentional here.
	p.ObjectModes = p.ObjectModes[:len(p.ObjectModes):len(p.ObjectModes)]
	p.Declarations = p.Declarations[:len(p.Declarations):len(p.Declarations)]
	p.TemplateActions = p.TemplateActions[:len(p.TemplateActions):len(p.TemplateActions)]
	p.ProcessClasses = p.ProcessClasses[:len(p.ProcessClasses):len(p.ProcessClasses)]
	p.PolicyIDs = p.PolicyIDs[:len(p.PolicyIDs):len(p.PolicyIDs)]
	return p
}

func codecTightComponents(c []receipt.Component) []receipt.Component {
	out := make([]receipt.Component, len(c))
	copy(out, c)
	for i := range out {
		f := make([]receipt.File, len(out[i].Files))
		copy(f, out[i].Files)
		out[i].Files = f
	}
	return out
}

func codecTightCandidate(v receipt.Candidate) receipt.Candidate {
	v.Components = codecTightComponents(v.Components)
	tools := make([]receipt.Tool, len(v.Tools))
	copy(tools, v.Tools)
	for i := range tools {
		locks := make([]receipt.LockValue, len(tools[i].Locks))
		copy(locks, tools[i].Locks)
		tools[i].Locks = locks
	}
	v.Tools = tools
	home := make([]string, len(v.HomeSources))
	copy(home, v.HomeSources)
	v.HomeSources = home
	targets := make([]string, len(v.Targets))
	copy(targets, v.Targets)
	v.Targets = targets
	executables := make([]receipt.Executable, len(v.Executables))
	copy(executables, v.Executables)
	v.Executables = executables
	v.Policy = codecTightPolicy(v.Policy)
	return v
}

func codecTightCompleted(v receipt.Completed) receipt.Completed {
	v.Components = codecTightComponents(v.Components)
	v.Policy = codecTightPolicy(v.Policy)
	tools := make([]receipt.ToolObservation, len(v.Tools))
	copy(tools, v.Tools)
	v.Tools = tools
	targets := make([]string, len(v.Targets))
	copy(targets, v.Targets)
	v.Targets = targets
	pre := make([]receipt.DestinationObservation, len(v.Preconditions))
	copy(pre, v.Preconditions)
	v.Preconditions = pre
	results := make([]receipt.DestinationObservation, len(v.Results))
	copy(results, v.Results)
	v.Results = results
	phases := make([]string, len(v.Phases))
	copy(phases, v.Phases)
	v.Phases = phases
	return v
}

func codecReferenceCandidate(data []byte) (receipt.Candidate, receipt.Digest, bool) {
	var v receipt.Candidate
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		return receipt.Candidate{}, "", false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return receipt.Candidate{}, "", false
	}
	v = codecTightCandidate(v)
	digest, canonical, err := receipt.CandidateDigest(v)
	if err != nil || !bytes.Equal(canonical, data) {
		return receipt.Candidate{}, "", false
	}
	return v, digest, true
}

func codecReferencePending(facts receipt.Completed) (receipt.Digest, []byte, error) {
	digest, wire, err := receipt.ReceiptDigest(facts)
	if err != nil {
		return "", nil, err
	}
	var normalized receipt.Completed
	if err = json.Unmarshal(wire, &normalized); err != nil {
		return "", nil, err
	}
	out, err := json.Marshal(pendingReceipt{Schema: 1, Domain: "zerb.receipt", Digest: digest, Facts: normalized})
	return digest, out, err
}

func codecReferencePendingDecode(data []byte) (pendingReceipt, bool) {
	var v pendingReceipt
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		return pendingReceipt{}, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return pendingReceipt{}, false
	}
	if v.Schema != 1 || v.Domain != "zerb.receipt" {
		return pendingReceipt{}, false
	}
	v.Facts = codecTightCompleted(v.Facts)
	digest, canonical, err := codecReferencePending(v.Facts)
	if err != nil || digest != v.Digest || !bytes.Equal(canonical, data) {
		return pendingReceipt{}, false
	}
	return v, true
}
