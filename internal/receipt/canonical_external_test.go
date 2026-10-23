package receipt_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// All preimages here are deliberately public, synthetic fixtures. These hashes
// establish wire compatibility, never acquisition, destination or current authority.
func publicDigest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func publicTree(t testing.TB, files []receipt.File) string {
	t.Helper()
	copyFiles := append([]receipt.File{}, files...)
	slices.SortFunc(copyFiles, func(a, b receipt.File) int { return strings.Compare(a.Path, b.Path) })
	wire := struct {
		Schema int            `json:"schema"`
		Domain string         `json:"domain"`
		Files  []receipt.File `json:"files"`
	}{1, "zerb.tree.v1", copyFiles}
	return publicDigest(string(publicJSON(t, wire)))
}

func publicCandidate(t testing.TB) receipt.Candidate {
	t.Helper()
	baseFiles := []receipt.File{{Path: "home", Mode: "directory"}, {Path: "home/dot_public", Mode: "file", Hash: publicDigest("public base\n")}}
	selectedFiles := []receipt.File{{Path: "home", Mode: "directory"}, {Path: "home/dot_selected", Mode: "executable", Hash: publicDigest("public selected\n")}}
	return receipt.Candidate{
		Schema: 1, Domain: "zerb.candidate",
		Source:    receipt.Source{URI: "https://github.com/ZebulonRouseFrantzich/zerb.git", SourceID: "sha256:ca92a55154224f40a4e208e1412d8c2530c2afb345b2dfd1a55b9627a7b1b62b", Commit: strings.Repeat("1", 40), RootTree: strings.Repeat("2", 40)},
		ProfileID: "public",
		Components: []receipt.Component{
			{ID: "base", Path: "base", TreeDigest: publicTree(t, baseFiles), Files: baseFiles},
			{ID: "public", Path: "profiles/public", TreeDigest: publicTree(t, selectedFiles), Files: selectedFiles},
		},
		Tools:       []receipt.Tool{{ID: "node", Version: "24.4.1", Locks: []receipt.LockValue{{Name: "artifact", Value: publicDigest("public node archive")}}}},
		HomeSources: []string{"base/home/dot_public", "profiles/public/home/dot_selected"},
		Targets:     []string{".public", ".selected"},
		Executables: []receipt.Executable{
			{ID: "bash", Version: "5.3.15", Path: "/opt/public/bin/bash", SHA256: publicDigest("public bash")},
			{ID: "chezmoi", Version: "v2.70.5-zerb.1", Path: "/opt/public/bin/chezmoi", SHA256: publicDigest("public chezmoi")},
			{ID: "git", Version: "2.55.0", Path: "/opt/public/bin/git", SHA256: publicDigest("public git")},
			{ID: "mise", Version: "2026.7.17", Path: "/opt/public/bin/mise", SHA256: publicDigest("public mise")},
		},
		Parser: receipt.Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"},
		Policy: receipt.Policy{Schema: 1, Domain: "zerb.policy", ObjectModes: []string{"directory", "executable", "file"}, Declarations: []string{"home", "mise.lock", "mise.toml", "profile.toml"}, TemplateActions: []string{"if-false", "if-true", "literal-text"}, ProcessClasses: []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}, PolicyIDs: []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}, TargetMapperExecutableID: "chezmoi", ProviderFree: true, HostFactIndependent: true, DestinationRecheck: true, ExactReapplyOnly: true},
	}
}

func publicJSON(t testing.TB, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func publicClone[T any](t testing.TB, value T) T {
	t.Helper()
	var copyValue T
	if err := json.Unmarshal(publicJSON(t, value), &copyValue); err != nil {
		t.Fatal(err)
	}
	return copyValue
}

func publicCandidateOracle(t testing.TB, input receipt.Candidate) []byte {
	t.Helper()
	c := publicClone(t, input)
	slices.SortFunc(c.Components, func(a, b receipt.Component) int { return strings.Compare(a.ID, b.ID) })
	for i := range c.Components {
		if c.Components[i].Files == nil {
			c.Components[i].Files = []receipt.File{}
		}
		slices.SortFunc(c.Components[i].Files, func(a, b receipt.File) int { return strings.Compare(a.Path, b.Path) })
	}
	if c.Tools == nil {
		c.Tools = []receipt.Tool{}
	}
	slices.SortFunc(c.Tools, func(a, b receipt.Tool) int { return strings.Compare(a.ID, b.ID) })
	for i := range c.Tools {
		slices.SortFunc(c.Tools[i].Locks, func(a, b receipt.LockValue) int { return strings.Compare(a.Name, b.Name) })
	}
	if c.HomeSources == nil {
		c.HomeSources = []string{}
	}
	if c.Targets == nil {
		c.Targets = []string{}
	}
	slices.Sort(c.HomeSources)
	slices.Sort(c.Targets)
	slices.SortFunc(c.Executables, func(a, b receipt.Executable) int { return strings.Compare(a.ID, b.ID) })
	for _, values := range [][]string{c.Policy.ObjectModes, c.Policy.Declarations, c.Policy.TemplateActions, c.Policy.ProcessClasses, c.Policy.PolicyIDs} {
		slices.Sort(values)
	}
	return publicJSON(t, c)
}

func checkPublicCandidate(t testing.TB, c receipt.Candidate) (receipt.Digest, []byte) {
	t.Helper()
	before := publicJSON(t, c)
	digest, wire, err := receipt.CandidateDigest(c)
	if err != nil {
		t.Fatalf("public candidate rejected: %v", err)
	}
	if !bytes.Equal(before, publicJSON(t, c)) {
		t.Fatal("canonicalization changed caller input")
	}
	want := publicCandidateOracle(t, c)
	if !bytes.Equal(wire, want) || string(digest) != publicDigest(string(want)) {
		t.Fatal("candidate differs from encoding/json oracle")
	}
	return digest, wire
}

func TestCandidatePublicWireContract(t *testing.T) {
	c := publicCandidate(t)
	_, wire := checkPublicCandidate(t, c)
	// This fixed digest covers the entire reviewed public synthetic preimage.
	const golden = "sha256:72a446c0ebdcec8fb689e2e08c9bd26ec37a73f91d500b4244b1815cd4494d34"
	if got := publicDigest(string(wire)); got != golden {
		t.Fatalf("candidate golden: %s", got)
	}
	permuted := publicClone(t, c)
	slices.Reverse(permuted.Components)
	for i := range permuted.Components {
		slices.Reverse(permuted.Components[i].Files)
	}
	slices.Reverse(permuted.Executables)
	slices.Reverse(permuted.HomeSources)
	slices.Reverse(permuted.Targets)
	for _, values := range [][]string{permuted.Policy.ObjectModes, permuted.Policy.Declarations, permuted.Policy.TemplateActions, permuted.Policy.ProcessClasses, permuted.Policy.PolicyIDs} {
		slices.Reverse(values)
	}
	_, got := checkPublicCandidate(t, permuted)
	if !bytes.Equal(got, wire) {
		t.Fatal("arrival order changed candidate identity")
	}
}

func TestCandidateJSONEscapingDifferential(t *testing.T) {
	for _, path := range []string{"home/quote\"and<>&", "home/control\b\f\n\r\t\x01", "home/雪\u2028\u2029"} {
		t.Run(path, func(t *testing.T) {
			c := publicCandidate(t)
			c.Components[0].Files[1].Path = path
			c.Components[0].TreeDigest = publicTree(t, c.Components[0].Files)
			c.HomeSources[0] = "base/" + path
			c.Targets[0] = path
			c.Executables[0].Path = "/opt/public/" + path
			checkPublicCandidate(t, c)
		})
	}
}

func TestCandidateEmptyCollectionsUseArrays(t *testing.T) {
	c := publicCandidate(t)
	c.Tools, c.HomeSources, c.Targets = nil, nil, nil
	for i := range c.Components {
		c.Components[i].Files = nil
		c.Components[i].TreeDigest = publicTree(t, nil)
	}
	_, nilWire := checkPublicCandidate(t, c)
	c.Tools, c.HomeSources, c.Targets = []receipt.Tool{}, []string{}, []string{}
	for i := range c.Components {
		c.Components[i].Files = []receipt.File{}
	}
	_, emptyWire := checkPublicCandidate(t, c)
	if !bytes.Equal(nilWire, emptyWire) || bytes.Contains(nilWire, []byte("null")) {
		t.Fatal("empty collections have multiple wire forms")
	}
}

func TestCandidateRejectsAmbiguousOrProhibitedFacts(t *testing.T) {
	cases := map[string]func(*receipt.Candidate){
		"duplicate-component": func(c *receipt.Candidate) { c.Components = append(c.Components, c.Components[0]) },
		"duplicate-file": func(c *receipt.Candidate) {
			c.Components[0].Files = append(c.Components[0].Files, c.Components[0].Files[0])
		},
		"duplicate-tool":                 func(c *receipt.Candidate) { c.Tools = append(c.Tools, c.Tools[0]) },
		"duplicate-lock":                 func(c *receipt.Candidate) { c.Tools[0].Locks = append(c.Tools[0].Locks, c.Tools[0].Locks[0]) },
		"duplicate-executable":           func(c *receipt.Candidate) { c.Executables = append(c.Executables, c.Executables[0]) },
		"duplicate-source":               func(c *receipt.Candidate) { c.HomeSources = append(c.HomeSources, c.HomeSources[0]) },
		"duplicate-target":               func(c *receipt.Candidate) { c.Targets = append(c.Targets, c.Targets[0]) },
		"duplicate-policy":               func(c *receipt.Candidate) { c.Policy.PolicyIDs = append(c.Policy.PolicyIDs, c.Policy.PolicyIDs[0]) },
		"required-components-nil":        func(c *receipt.Candidate) { c.Components = nil },
		"required-executables-empty":     func(c *receipt.Candidate) { c.Executables = []receipt.Executable{} },
		"required-policy-nil":            func(c *receipt.Candidate) { c.Policy.ObjectModes = nil },
		"invalid-utf8":                   func(c *receipt.Candidate) { c.Targets[0] = string([]byte{0xff}) },
		"git-object-format-mismatch":     func(c *receipt.Candidate) { c.Source.RootTree = strings.Repeat("2", 64) },
		"materialized-root-substitution": func(c *receipt.Candidate) { c.Source.RootTree = c.Components[0].TreeDigest },
		"credential-source":              func(c *receipt.Candidate) { c.Source.URI = "https://public@example.com/public.git" },
		"directory-content-hash":         func(c *receipt.Candidate) { c.Components[0].Files[0].Hash = publicDigest("directory") },
		"inconsistent-tree":              func(c *receipt.Candidate) { c.Components[0].TreeDigest = publicDigest("other public tree") },
	}
	for _, mode := range []string{"symlink", "gitlink", "device", "100644", ""} {
		cases["mode-"+mode] = func(c *receipt.Candidate) { c.Components[0].Files[1].Mode = mode }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := publicCandidate(t)
			mutate(&c)
			before := publicJSON(t, c)
			digest, wire, err := receipt.CandidateDigest(c)
			if err == nil || digest != "" || wire != nil {
				t.Fatal("invalid candidate returned an identity")
			}
			if !bytes.Equal(before, publicJSON(t, c)) {
				t.Fatal("failure changed caller input")
			}
		})
	}
}

func TestCandidatePreservesNestedBackingStorage(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		c := publicCandidate(t)
		files := append(c.Components[0].Files, receipt.File{Path: "spare backing storage"})
		c.Components[0].Files = files[:2]
		locks := append(c.Tools[0].Locks, receipt.LockValue{Name: "spare backing storage"})
		c.Tools[0].Locks = locks[:1]
		beforeFiles, beforeLocks := slices.Clone(files), slices.Clone(locks)
		slices.Reverse(c.Components)
		if invalid {
			c.Policy.Schema = 0
		}
		_, _, err := receipt.CandidateDigest(c)
		if (err != nil) != invalid {
			t.Fatalf("unexpected result: %v", err)
		}
		if !reflect.DeepEqual(files, beforeFiles) || !reflect.DeepEqual(locks, beforeLocks) {
			t.Fatal("nested caller backing storage changed")
		}
	}
}

func publicCompleted(t testing.TB) receipt.Completed {
	t.Helper()
	c := publicCandidate(t)
	digest, _ := checkPublicCandidate(t, c)
	return receipt.Completed{
		Schema: 1, Domain: "zerb.completed", Candidate: digest, Source: c.Source, Components: c.Components,
		Zerb: receipt.Executable{ID: "zerb", Version: "v0.0.1-alpha", Path: "/opt/public/bin/zerb", SHA256: publicDigest("public zerb")},
		Git:  c.Executables[2], Mise: c.Executables[3], Chezmoi: c.Executables[1], Parser: c.Parser, Policy: c.Policy,
		Platform:      receipt.PlatformObservation{Distribution: "Fedora Linux 44", Kernel: "7.1.10-200.fc44.x86_64", Architecture: "x86_64", Filesystem: "btrfs"},
		Tools:         []receipt.ToolObservation{{ID: "node", Requested: "24.4.1", Installed: "24.4.1", Integrity: publicDigest("public node archive")}},
		Targets:       c.Targets,
		Preconditions: []receipt.DestinationObservation{{Target: ".public", Kind: "absent"}, {Target: ".selected", Kind: "absent"}},
		Results: []receipt.DestinationObservation{
			{Target: ".public", Exists: true, Kind: "file", Mode: 0600, Owner: 1000, Device: 1, Inode: 2, Links: 1, ContentHash: publicDigest("public base\n")},
			{Target: ".selected", Exists: true, Kind: "directory", Mode: 0700, Owner: 1000, Device: 1, Inode: 3, Links: 1},
		},
		Phases:     []string{"mutating-home-apply", "mutating-tool-install"},
		Dimensions: receipt.Dimensions{InputsPinned: true, ArtifactIntegrity: true, ArtifactAvailable: true, HostCompatible: true, ConvergenceVerified: true},
	}
}

func publicReceiptOracle(t testing.TB, input receipt.Completed) []byte {
	t.Helper()
	r := publicClone(t, input)
	slices.SortFunc(r.Components, func(a, b receipt.Component) int { return strings.Compare(a.ID, b.ID) })
	for i := range r.Components {
		if r.Components[i].Files == nil {
			r.Components[i].Files = []receipt.File{}
		}
		slices.SortFunc(r.Components[i].Files, func(a, b receipt.File) int { return strings.Compare(a.Path, b.Path) })
	}
	if r.Tools == nil {
		r.Tools = []receipt.ToolObservation{}
	}
	if r.Targets == nil {
		r.Targets = []string{}
	}
	if r.Preconditions == nil {
		r.Preconditions = []receipt.DestinationObservation{}
	}
	if r.Results == nil {
		r.Results = []receipt.DestinationObservation{}
	}
	if r.Phases == nil {
		r.Phases = []string{}
	}
	slices.SortFunc(r.Tools, func(a, b receipt.ToolObservation) int { return strings.Compare(a.ID, b.ID) })
	slices.Sort(r.Targets)
	slices.SortFunc(r.Preconditions, func(a, b receipt.DestinationObservation) int { return strings.Compare(a.Target, b.Target) })
	slices.SortFunc(r.Results, func(a, b receipt.DestinationObservation) int { return strings.Compare(a.Target, b.Target) })
	slices.Sort(r.Phases)
	for _, values := range [][]string{r.Policy.ObjectModes, r.Policy.Declarations, r.Policy.TemplateActions, r.Policy.ProcessClasses, r.Policy.PolicyIDs} {
		slices.Sort(values)
	}
	return publicJSON(t, r)
}

func checkPublicReceipt(t testing.TB, r receipt.Completed) (receipt.Digest, []byte) {
	t.Helper()
	before := publicJSON(t, r)
	digest, wire, err := receipt.ReceiptDigest(r)
	if err != nil {
		t.Fatalf("public completed facts rejected: %v", err)
	}
	if !bytes.Equal(before, publicJSON(t, r)) {
		t.Fatal("receipt canonicalization changed caller input")
	}
	want := publicReceiptOracle(t, r)
	if !bytes.Equal(wire, want) || string(digest) != publicDigest(string(want)) {
		t.Fatal("receipt differs from encoding/json oracle")
	}
	return digest, wire
}

func TestReceiptPublicWireContract(t *testing.T) {
	r := publicCompleted(t)
	_, wire := checkPublicReceipt(t, r)
	const golden = "sha256:4809b7a12c96f312022d1826442a0908ba6b271555d1ae93e5d9b1f317e6525b"
	if got := publicDigest(string(wire)); got != golden {
		t.Fatalf("receipt golden: %s", got)
	}
	slices.Reverse(r.Components)
	for i := range r.Components {
		slices.Reverse(r.Components[i].Files)
	}
	slices.Reverse(r.Targets)
	slices.Reverse(r.Preconditions)
	slices.Reverse(r.Results)
	slices.Reverse(r.Phases)
	_, permuted := checkPublicReceipt(t, r)
	if !bytes.Equal(wire, permuted) {
		t.Fatal("receipt fact order changed identity")
	}
}

func TestReceiptObservationsAndCoverageRejectInconsistency(t *testing.T) {
	cases := map[string]func(*receipt.Completed){
		"missing-precondition":      func(r *receipt.Completed) { r.Preconditions = r.Preconditions[:1] },
		"missing-result":            func(r *receipt.Completed) { r.Results = r.Results[:1] },
		"duplicate-target":          func(r *receipt.Completed) { r.Targets = append(r.Targets, r.Targets[0]) },
		"duplicate-precondition":    func(r *receipt.Completed) { r.Preconditions = append(r.Preconditions, r.Preconditions[0]) },
		"duplicate-result":          func(r *receipt.Completed) { r.Results = append(r.Results, r.Results[0]) },
		"unknown-result-target":     func(r *receipt.Completed) { r.Results[0].Target = "unlisted" },
		"duplicate-tool":            func(r *receipt.Completed) { r.Tools = append(r.Tools, r.Tools[0]) },
		"duplicate-phase":           func(r *receipt.Completed) { r.Phases = append(r.Phases, r.Phases[0]) },
		"absent-exists":             func(r *receipt.Completed) { r.Preconditions[0].Exists = true },
		"absent-mode":               func(r *receipt.Completed) { r.Preconditions[0].Mode = 0600 },
		"absent-owner":              func(r *receipt.Completed) { r.Preconditions[0].Owner = 1000 },
		"absent-device":             func(r *receipt.Completed) { r.Preconditions[0].Device = 1 },
		"absent-inode":              func(r *receipt.Completed) { r.Preconditions[0].Inode = 1 },
		"absent-links":              func(r *receipt.Completed) { r.Preconditions[0].Links = 1 },
		"absent-hash":               func(r *receipt.Completed) { r.Preconditions[0].ContentHash = publicDigest("public content") },
		"present-without-exists":    func(r *receipt.Completed) { r.Results[0].Exists = false },
		"unsupported-kind":          func(r *receipt.Completed) { r.Results[0].Kind = "symlink" },
		"file-missing-hash":         func(r *receipt.Completed) { r.Results[0].ContentHash = "" },
		"directory-hash":            func(r *receipt.Completed) { r.Results[1].ContentHash = publicDigest("public content") },
		"mode-file-type-bits":       func(r *receipt.Completed) { r.Results[0].Mode = 0100600 },
		"tool-integrity-not-digest": func(r *receipt.Completed) { r.Tools[0].Integrity = "verified" },
		"invalid-utf8":              func(r *receipt.Completed) { r.Platform.Kernel = string([]byte{0xff}) },
	}
	for _, phase := range []string{"prepared", "pending-receipt", "cleanup", "unrecognized"} {
		cases["phase-"+phase] = func(r *receipt.Completed) { r.Phases = []string{phase} }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := publicCompleted(t)
			mutate(&r)
			before := publicJSON(t, r)
			digest, wire, err := receipt.ReceiptDigest(r)
			if err == nil || digest != "" || wire != nil {
				t.Fatal("inconsistent completed facts returned an identity")
			}
			if !bytes.Equal(before, publicJSON(t, r)) {
				t.Fatal("receipt failure changed caller input")
			}
		})
	}
}

func TestReceiptIndependentDimensionsAndObservationIdentity(t *testing.T) {
	r := publicCompleted(t)
	original, _ := checkPublicReceipt(t, r)
	for i := range 5 {
		changed := publicClone(t, r)
		fields := []*bool{&changed.Dimensions.InputsPinned, &changed.Dimensions.ArtifactIntegrity, &changed.Dimensions.ArtifactAvailable, &changed.Dimensions.HostCompatible, &changed.Dimensions.ConvergenceVerified}
		*fields[i] = false
		digest, _ := checkPublicReceipt(t, changed)
		if digest == original {
			t.Fatal("independent dimension was omitted")
		}
	}
	r.Dimensions = receipt.Dimensions{}
	checkPublicReceipt(t, r)
	r.Results[0].Mode = 04700
	r.Results[0].Owner = 0
	checkPublicReceipt(t, r)
	r.Tools[0].Integrity = publicDigest("different public authenticated archive")
	changed, _ := checkPublicReceipt(t, r)
	if changed == original {
		t.Fatal("observed identity change omitted")
	}
	r.Targets, r.Preconditions, r.Results, r.Tools, r.Phases = nil, nil, nil, nil, nil
	_, nilWire := checkPublicReceipt(t, r)
	r.Targets, r.Preconditions, r.Results, r.Tools, r.Phases = []string{}, []receipt.DestinationObservation{}, []receipt.DestinationObservation{}, []receipt.ToolObservation{}, []string{}
	_, emptyWire := checkPublicReceipt(t, r)
	if !bytes.Equal(nilWire, emptyWire) || bytes.Contains(nilWire, []byte("null")) {
		t.Fatal("receipt empty collections differ")
	}
}

func FuzzCandidateJSONEscapingDifferential(f *testing.F) {
	for _, seed := range []string{"plain", "quote\"<>&\n\u2028\u2029雪"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) == 0 || len(name) > 240 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
			t.Skip()
		}
		c := publicCandidate(t)
		c.Components[0].Files[1].Path = "home/" + name
		c.Components[0].TreeDigest = publicTree(t, c.Components[0].Files)
		c.HomeSources[0] = "base/home/" + name
		c.Targets[0] = "target/" + name
		checkPublicCandidate(t, c)
	})
}

func TestReceiptObservedMetadataIsData(t *testing.T) {
	r := publicCompleted(t)
	original, _ := checkPublicReceipt(t, r)
	r.Zerb.Version = "public-observation-" + strings.Repeat("v", 160) + "\"<>&\n雪\u2028\u2029"
	r.Platform = receipt.PlatformObservation{Distribution: "Public synthetic distribution", Kernel: strings.Repeat("k", 160), Architecture: "public\"<>&\n雪\u2028\u2029", Filesystem: "public synthetic filesystem"}
	changed, _ := checkPublicReceipt(t, r)
	if changed == original {
		t.Fatal("observed metadata was omitted")
	}
}

func TestCandidateObjectAndSourceIdentityRemainDistinctFacts(t *testing.T) {
	c := publicCandidate(t)
	original, _ := checkPublicCandidate(t, c)
	c.Source.Commit = strings.Repeat("3", 64)
	c.Source.RootTree = strings.Repeat("4", 64)
	changed, _ := checkPublicCandidate(t, c)
	if changed == original {
		t.Fatal("native object identity omitted")
	}
	// Authentication of the source ID's URI relation belongs to the source owner.
	c.Source.SourceID = publicDigest("different public source identity")
	other, _ := checkPublicCandidate(t, c)
	if other == changed {
		t.Fatal("source identity omitted")
	}
}

func TestReceiptObservationIntegerWireWidths(t *testing.T) {
	r := publicCompleted(t)
	r.Results[0].Mode = 07777
	r.Results[0].Owner = ^uint32(0)
	r.Results[0].Device = ^uint64(0)
	r.Results[0].Inode = ^uint64(0)
	r.Results[0].Links = ^uint64(0)
	checkPublicReceipt(t, r)
}

func TestReceiptPreservesNestedBackingStorage(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		r := publicCompleted(t)
		files := append(r.Components[0].Files, receipt.File{Path: "spare backing storage"})
		r.Components[0].Files = files[:2]
		results := append(r.Results, receipt.DestinationObservation{Target: "spare backing storage"})
		r.Results = results[:2]
		slices.Reverse(r.Components[0].Files)
		slices.Reverse(r.Results)
		beforeFiles, beforeResults := slices.Clone(files), slices.Clone(results)
		before := publicJSON(t, r)
		if invalid {
			r.Results[0].Kind = "symlink"
			beforeResults = slices.Clone(results)
			before = publicJSON(t, r)
		}
		_, _, err := receipt.ReceiptDigest(r)
		if (err != nil) != invalid {
			t.Fatalf("unexpected result: %v", err)
		}
		if !bytes.Equal(before, publicJSON(t, r)) || !reflect.DeepEqual(files, beforeFiles) || !reflect.DeepEqual(results, beforeResults) {
			t.Fatal("receipt changed caller backing storage")
		}
	}
}

func TestCandidateAndReceiptSourceURISyntaxWithoutAcquisitionAuthority(t *testing.T) {
	valid := []string{
		"https://example.com/",
		"https://example.com/public/",
		"https://example.com/public//tree.git",
		"https://public/environment.git",
		"https://1.1.1.1/environment.git",
		"https://[2606:4700:4700::1111]/environment.git",
		"https://example.com/public@tree.git",
		"https://" + strings.Repeat("a", 64) + ".example/environment.git",
		"https://" + strings.Repeat(strings.Repeat("a", 63)+".", 4) + "example/environment.git",
	}
	for _, uri := range valid {
		t.Run(uri, func(t *testing.T) {
			// Pure encoding performs no resolution or public-endpoint admission and
			// cannot authenticate the source ID's relation to these URI bytes.
			c := publicCandidate(t)
			c.Source.URI = uri
			checkPublicCandidate(t, c)
			r := publicCompleted(t)
			r.Source.URI = uri
			checkPublicReceipt(t, r)
		})
	}
	invalid := []string{
		"https://public@example.com/environment.git",
		"https://example.com:443/environment.git",
		"https://example.com:0443/environment.git",
		"https://example.com/%65nvironment.git",
		"https://example.com/public%2ftree.git",
		"https://example.com/public%2Ftree.git",
		"https://example.com/public%5Ctree.git",
		"https://example.com/public/../tree.git",
		"https://example.com/public/./tree.git",
		"HTTPS://example.com/environment.git",
		"https://EXAMPLE.com/environment.git",
		"https://example.com./environment.git",
		"https://example.com",
		"https://example.com/environment.git?public",
		"https://example.com/environment.git#public",
	}
	for _, uri := range invalid {
		t.Run(uri, func(t *testing.T) {
			c := publicCandidate(t)
			c.Source.URI = uri
			if digest, wire, err := receipt.CandidateDigest(c); err == nil || digest != "" || wire != nil {
				t.Fatal("noncanonical source syntax returned candidate identity")
			}
			r := publicCompleted(t)
			r.Source.URI = uri
			if digest, wire, err := receipt.ReceiptDigest(r); err == nil || digest != "" || wire != nil {
				t.Fatal("noncanonical source syntax returned receipt identity")
			}
		})
	}
}
