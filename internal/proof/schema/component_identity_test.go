package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// PC-040/043/050, VAL-016: component identity is independent of arrival/sort position.
func TestCandidateComponentIdentityOrder(t *testing.T) {
	for _, id := range []string{"0", "a", "aaa", "b", "bas", "base-", "base0", "personal", "z", strings.Repeat("a", 64)} {
		t.Run(id, func(t *testing.T) {
			base, selected := proofComponents()
			selected.ID = id
			t.Run("compose", func(t *testing.T) {
				candidate, digest, err := Compose(proofSource(), base, selected, proofExecutables(), proofPolicy())
				if err != nil || digest == "" {
					t.Fatalf("valid selected identity rejected: %v", err)
				}
				checkComponentOrder(t, candidate, digest)
			})
			t.Run("direct", func(t *testing.T) {
				input := baseCandidate()
				input.ProfileID = id
				input.Components[1].ID = id
				candidate, digest, err := DigestCandidate(input)
				if err != nil || digest == "" {
					t.Fatalf("valid direct selected identity rejected: %v", err)
				}
				checkComponentOrder(t, candidate, digest)
			})
		})
	}
}

func checkComponentOrder(t *testing.T, candidate CandidatePreimage, digest string) {
	t.Helper()
	if len(candidate.Components) != 2 || candidate.Components[0].ID >= candidate.Components[1].ID {
		t.Fatal("component identities are not in their existing lexical order")
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	reversed := cloneCandidate(candidate)
	reversed.Components = reverseCopy(reversed.Components)
	normalized, repeated, err := DigestCandidate(reversed)
	if err != nil || repeated != digest {
		t.Fatalf("arrival order changed candidate identity: %v", err)
	}
	reencoded, err := EncodeCandidate(normalized)
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatalf("arrival order changed canonical bytes: %v", err)
	}
}

func TestCandidateComponentIdentityRejectsInvalidMembership(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*CandidatePreimage)
	}{
		{"duplicate base selected as base", func(c *CandidatePreimage) { c.ProfileID = "base"; c.Components[1].ID = "base" }},
		{"base selected with unrelated component", func(c *CandidatePreimage) { c.ProfileID = "base" }},
		{"duplicate base", func(c *CandidatePreimage) { c.Components[1].ID = "base" }},
		{"duplicate selected", func(c *CandidatePreimage) { c.Components[0].ID = c.ProfileID }},
		{"missing base", func(c *CandidatePreimage) { c.Components[0].ID = "unrelated" }},
		{"missing selected", func(c *CandidatePreimage) { c.Components[1].ID = "unrelated" }},
		{"mismatched profile", func(c *CandidatePreimage) { c.ProfileID = "unrelated" }},
		{"no components", func(c *CandidatePreimage) { c.Components = nil }},
		{"one component", func(c *CandidatePreimage) { c.Components = c.Components[:1] }},
		{"three components", func(c *CandidatePreimage) { c.Components = append(c.Components, c.Components[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := baseCandidate()
			for _, reverse := range []bool{false, true} {
				changed := cloneCandidate(input)
				test.edit(&changed)
				if reverse {
					changed.Components = reverseCopy(changed.Components)
				}
				candidate, digest, err := DigestCandidate(cloneCandidate(changed))
				if err == nil || digest != "" || candidate.Schema != 0 || len(candidate.Components) != 0 {
					t.Error("invalid membership returned candidate authority")
				}
				encoded, err := EncodeCandidate(changed)
				if err == nil || encoded != nil {
					t.Error("invalid membership returned canonical bytes")
				}
			}
		})
	}
}

func TestDecodeZerbReservedBaseIdentity(t *testing.T) {
	for _, kind := range []string{"embedded", "gitlink"} {
		t.Run(kind, func(t *testing.T) {
			input := []byte(fmt.Sprintf("schema=1\n[base]\npath='profiles/base'\n[[profiles]]\nid='base'\nkind=%q\npath='profiles/selected'\n", kind))
			_, diagnostic, err := DecodeZerb(input)
			if err == nil || diagnostic == nil || err.Error() != "invalid profile id" {
				t.Fatal("reserved base identity was selectable")
			}
		})
	}
	for _, id := range []string{"aaa", "base-", "base0", "personal", strings.Repeat("a", 64)} {
		t.Run(id, func(t *testing.T) {
			input := []byte(fmt.Sprintf("schema=1\n[base]\npath='profiles/base'\n[[profiles]]\nid=%q\nkind='embedded'\npath='profiles/selected'\n", id))
			manifest, diagnostic, err := DecodeZerb(input)
			if err != nil || diagnostic != nil || len(manifest.Profiles) != 1 || manifest.Profiles[0].ID != id {
				t.Fatalf("nonreserved profile identity rejected: %v", err)
			}
		})
	}
}

func TestCandidateExistingPublicFixtureEncoding(t *testing.T) {
	base, selected := proofComponents()
	candidate, digest, err := Compose(proofSource(), base, selected, proofExecutables(), proofPolicy())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeCandidate(candidate)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("existing public synthetic fixture failed: %v", err)
	}
	// This complete hardcoded synthetic preimage was reviewed as non-sensitive.
	const previous = "sha256:52ec40825dda96e0d7d736b91f8a1e359d1715b039e1dd34a333a56904d1c7ee"
	if digest != previous {
		t.Fatal("existing public fixture encoding changed")
	}
}

func TestComponentIdentityReportedScenarios(t *testing.T) {
	parser, _ := parserCases()
	scenarios := append(parser, compositionCases()...)
	for _, name := range []string{"zerb-reserved-base-id", "component-membership-before-base", "candidate-reserved-base-selection"} {
		found := false
		for _, scenario := range scenarios {
			if scenario.Name == name {
				found = true
				if scenario.Status != "observed" {
					t.Errorf("proof scenario %s failed", name)
				}
			}
		}
		if !found {
			t.Errorf("proof scenario %s missing", name)
		}
	}
}
