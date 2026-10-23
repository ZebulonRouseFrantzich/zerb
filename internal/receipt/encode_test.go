package receipt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func unitCandidate(t *testing.T) Candidate {
	t.Helper()
	c := Candidate{Schema: 1, Domain: "zerb.candidate", Source: Source{URI: "https://example.com/repo.git", SourceID: strings.Repeat("a", 64), Commit: strings.Repeat("b", 40), RootTree: strings.Repeat("c", 40)}, ProfileID: "a-profile"}
	c.Source.SourceID = "sha256:" + c.Source.SourceID
	c.Parser = Parser{Schema: 1, Module: "github.com/pelletier/go-toml/v2", Version: "v2.4.3-zerb.1", ModuleSum: "h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=", GoModSum: "h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=", OriginCommit: "071a36c2a57244f2e70369bfc69889fda2a1f60f"}
	c.Policy = Policy{Schema: 1, Domain: "zerb.policy", ObjectModes: []string{"directory", "executable", "file"}, Declarations: []string{"home", "mise.lock", "mise.toml", "profile.toml"}, TemplateActions: []string{"if-false", "if-true", "literal-text"}, ProcessClasses: []string{"bash-shell", "chezmoi-static", "git-public", "mise-core-node"}, PolicyIDs: []string{"zerb.chezmoi-static.v2", "zerb.destination-recheck.v1", "zerb.execenv.v4", "zerb.git-public.v3", "zerb.lifecycle.v6", "zerb.mise-core-node.v3"}, TargetMapperExecutableID: "chezmoi", ProviderFree: true, HostFactIndependent: true, DestinationRecheck: true, ExactReapplyOnly: true}
	c.Components = []Component{{ID: "a-profile", Path: "profiles/a-profile", Files: []File{}}, {ID: "base", Path: "base", Files: []File{{Path: "quote\"<&\u2028/雪", Mode: "file", Hash: unitToken()}}}}
	for i := range c.Components {
		b, err := json.Marshal(struct {
			Schema int    `json:"schema"`
			Domain string `json:"domain"`
			Files  []File `json:"files"`
		}{1, "zerb.tree.v1", c.Components[i].Files})
		if err != nil {
			t.Fatal(err)
		}
		c.Components[i].TreeDigest = string(hashBytes(b))
	}
	for i, id := range []string{"bash", "chezmoi", "git", "mise"} {
		c.Executables = append(c.Executables, Executable{ID: id, Version: []string{"5.3.15", "v2.70.5-zerb.1", "2.55.0", "2026.7.17"}[i], Path: "/opt/" + id, SHA256: unitToken()})
	}
	c.Tools = []Tool{}
	c.HomeSources = []string{}
	c.Targets = []string{}
	return c
}
func unitToken() string { return "sha256:" + strings.Repeat("0", 64) }
func unitCompleted(t *testing.T) Completed {
	t.Helper()
	c := unitCandidate(t)
	return Completed{Schema: 1, Domain: "zerb.completed", Candidate: Digest(unitToken()), Source: c.Source, Components: c.Components, Zerb: Executable{ID: "zerb", Version: strings.Repeat("v", 200), Path: "/opt/zerb", SHA256: unitToken()}, Git: c.Executables[2], Mise: c.Executables[3], Chezmoi: c.Executables[1], Parser: c.Parser, Policy: c.Policy, Platform: PlatformObservation{Distribution: "test", Kernel: "1", Architecture: "x86", Filesystem: "test"}, Tools: []ToolObservation{}, Targets: []string{}, Preconditions: []DestinationObservation{}, Results: []DestinationObservation{}, Phases: []string{"mutating-home-apply", "mutating-tool-install"}}
}
func TestPrivateWriterMatchesStandardJSONStringOracle(t *testing.T) {
	inputs := []string{"", "\"\\\b\f\n\r\t<&>", "雪\u2028\u2029😀"}
	for i := 0; i < 256; i++ {
		inputs = append(inputs, string(rune(i)))
	}
	random := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- Deterministic PC-050 public string oracle; no security values.
	for range 1000 {
		var s strings.Builder
		for range random.IntN(80) {
			r := rune(random.IntN(0x110000))
			if r >= 0xd800 && r <= 0xdfff {
				r = 'x'
			}
			s.WriteRune(r)
		}
		inputs = append(inputs, s.String())
	}
	for _, input := range inputs {
		want, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var size jsonWriter
		size.string(input)
		actual := jsonWriter{out: make([]byte, 0, size.size), writing: true}
		actual.string(input)
		if actual.failed || size.failed || !bytes.Equal(actual.out, want) || size.size != uint64(len(want)) {
			t.Fatalf("JSON string mismatch for %q", input)
		}
	}
	for _, bad := range []string{string([]byte{255}), "a\xc0\xaf"} {
		var w jsonWriter
		w.string(bad)
		if !w.failed {
			t.Fatal("invalid UTF-8 replaced")
		}
	}
}

// PC-042/050: observable bytes and exact returned capacity do not depend on prior calls or JSON's pool.
func TestCanonicalEncoderLargeToSmallHistory(t *testing.T) {
	candidate := unitCandidate(t)
	wantDigest, wantBytes, err := CandidateDigest(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{8192, 1024, 128} {
		large := unitCompleted(t)
		large.Zerb.Version = strings.Repeat("public synthetic history ", count)
		_, encoded, err := ReceiptDigest(large)
		if err != nil || cap(encoded) != len(encoded) {
			t.Fatal("large receipt output violated its exact reservation")
		}
		// The standard encoder is deliberately used only as an unrelated test oracle history.
		if _, err := json.Marshal(large); err != nil {
			t.Fatal(err)
		}
		encoded[0] = '#'
		actualDigest, actualBytes, err := CandidateDigest(candidate)
		if err != nil || actualDigest != wantDigest || !bytes.Equal(actualBytes, wantBytes) || cap(actualBytes) != len(actualBytes) {
			t.Fatal("prior receipt or oracle storage changed the small candidate result")
		}
		actualBytes[0] = '#'
		if !bytes.Equal(wantBytes[:1], []byte{'{'}) {
			t.Fatal("successive outputs shared backing storage")
		}
	}
}
func TestPrivateWritersMatchStandardClosedSchemaOracle(t *testing.T) {
	c := unitCandidate(t)
	d, b, err := CandidateDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(c)
	if !bytes.Equal(want, b) || d != hashBytes(want) {
		t.Fatal("candidate differs from standard encoder")
	}
	r := unitCompleted(t)
	r.Previous = Digest(unitToken())
	r.Dimensions = Dimensions{InputsPinned: true, ArtifactAvailable: true}
	d, b, err = ReceiptDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	want, _ = json.Marshal(r)
	if !bytes.Equal(want, b) || d != hashBytes(want) {
		t.Fatal("receipt differs from standard encoder")
	}
	r.Previous = ""
	_, b, err = ReceiptDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	want, _ = json.Marshal(r)
	if !bytes.Equal(want, b) {
		t.Fatal("omitempty differs from standard encoder")
	}
	var w jsonWriter
	w.unsigned(math.MaxUint64)
	actual := jsonWriter{out: make([]byte, 0, w.size), writing: true}
	actual.unsigned(math.MaxUint64)
	if string(actual.out) != "18446744073709551615" {
		t.Fatal("uint64 encoded incorrectly")
	}
}
func TestCanonicalizationPreservesCallerOnLateSuccessAndFailure(t *testing.T) {
	c := unitCandidate(t)
	slices.Reverse(c.Components)
	slices.Reverse(c.Policy.PolicyIDs)
	c.Targets = []string{"z", "a"}
	before, _ := json.Marshal(c)
	_, _, err := CandidateDigest(c)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(c)
	if !bytes.Equal(before, after) {
		t.Fatal("candidate caller mutated")
	}
	c.Parser.Schema = 2
	before, _ = json.Marshal(c)
	_, _, err = CandidateDigest(c)
	after, _ = json.Marshal(c)
	if err == nil || !bytes.Equal(before, after) {
		t.Fatal("failure mutated caller or succeeded")
	}
	r := unitCompleted(t)
	slices.Reverse(r.Components)
	slices.Reverse(r.Policy.PolicyIDs)
	slices.Reverse(r.Phases)
	before, _ = json.Marshal(r)
	_, _, err = ReceiptDigest(r)
	after, _ = json.Marshal(r)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("receipt caller mutated or rejected")
	}
	r.Results = []DestinationObservation{{Target: "missing"}}
	before, _ = json.Marshal(r)
	_, _, err = ReceiptDigest(r)
	after, _ = json.Marshal(r)
	if err == nil || !bytes.Equal(before, after) {
		t.Fatal("receipt failure mutated caller or succeeded")
	}
}

func TestSourceURIAuthorityAndRepositoryHaveDistinctGrammar(t *testing.T) {
	for _, source := range []string{"https://example.com/repo@revision.git", "https://example.com/a%20b.git", "https://example.com:444/repo.git", "https://example.com/a!$&'()*+,;=:@.git", "https://example.com/", "https://example.com/repo/", "https://example.com/repo//file", "https://repository/repo.git", "https://8.8.8.8/repo.git", "https://[2606:4700:4700::1111]/repo.git", "https://" + strings.Repeat("x", 300) + "/repo.git"} {
		if !validSourceURI(source) {
			t.Fatalf("canonical source syntax rejected: %q", source)
		}
	}
	for _, source := range []string{"https://user@example.com/repo.git", "https://user:password@example.com/repo.git", "https://example.com:443/repo.git", "https://example.com:/repo.git", "https://example.com./repo.git", "https://example.com/repo%2Fname.git", "https://example.com/repo%41.git", "https://EXAMPLE.com/repo.git", "https://example.com/./repo.git", "https://example.com:65536/repo.git", "https://[invalid]/repo.git", "https://::1/repo.git", "https://[2606:4700:4700:0:0:0:0:1111]/repo.git", "https://example.com/repo%2f.git", "https://example.com/repo.git?", "https://example.com/repo.git#"} {
		if validSourceURI(source) {
			t.Fatalf("noncanonical authority or path accepted: %q", source)
		}
	}
}

func TestPrivateWriterAllReceiptScalarFormsAgainstOracle(t *testing.T) {
	r := unitCompleted(t)
	r.Targets = []string{"a"}
	r.Preconditions = []DestinationObservation{{Target: "a", Exists: true, Kind: "file", Mode: math.MaxUint32, Owner: math.MaxUint32, Device: math.MaxUint64, Inode: math.MaxUint64, Links: math.MaxUint64, ContentHash: unitToken()}}
	r.Results = []DestinationObservation{{Target: "a", Kind: "absent"}}
	r.Tools = []ToolObservation{{ID: "node", Requested: "24.4.1", Installed: "24.4.1", Integrity: unitToken()}}
	r.Dimensions = Dimensions{true, true, true, true, true}
	var size jsonWriter
	size.completed(r)
	out := jsonWriter{out: make([]byte, 0, size.size), writing: true}
	out.completed(r)
	want, err := json.Marshal(r)
	if err != nil || out.failed || !bytes.Equal(out.out, want) {
		t.Fatal("scalar encoding differs from standard encoder")
	}
}

func TestReceiptAllowsFullTargetInventoryWithinWholeCallBudget(t *testing.T) {
	r := unitCompleted(t)
	for i := range 4096 {
		target := fmt.Sprintf("target-%04d", i)
		r.Targets = append(r.Targets, target)
		observation := DestinationObservation{Target: target, Kind: "absent"}
		r.Preconditions = append(r.Preconditions, observation)
		r.Results = append(r.Results, observation)
	}
	var size jsonWriter
	size.completed(r)
	if size.strings <= 16384 {
		t.Fatal("fixture misses receipt-only string count regression")
	}
	if _, _, err := ReceiptDigest(r); err != nil {
		t.Fatal("valid 4096-target receipt rejected")
	}
	r.Targets = append(r.Targets, "target-4096")
	r.Preconditions = append(r.Preconditions, DestinationObservation{Target: "target-4096", Kind: "absent"})
	r.Results = append(r.Results, DestinationObservation{Target: "target-4096", Kind: "absent"})
	if d, b, err := ReceiptDigest(r); err == nil || d != "" || b != nil {
		t.Fatal("4097-target receipt accepted")
	}
}
