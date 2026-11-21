package receipt_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// PC-020/050: net/url round trips are insufficient to admit URI authority bytes.
func TestSourceAuthorityRejectsReservedRegNameBytes(t *testing.T) {
	for _, host := range []string{"example.com]", "exa<mple.com", "exa>mple.com", "exa\"mple.com"} {
		for _, suffix := range []string{"/environment.git", ":444/environment.git", "/"} {
			uri := "https://" + host + suffix
			t.Run(uri, func(t *testing.T) {
				candidate := publicCandidate(t)
				candidate.Source.URI = uri
				before := publicJSON(t, candidate)
				if digest, wire, err := receipt.CandidateDigest(candidate); err == nil || digest != "" || wire != nil {
					t.Error("malformed authority returned candidate identity")
				}
				if !bytes.Equal(before, publicJSON(t, candidate)) {
					t.Error("candidate rejection changed caller input")
				}
				completed := publicCompleted(t)
				completed.Source.URI = uri
				before = publicJSON(t, completed)
				if digest, wire, err := receipt.ReceiptDigest(completed); err == nil || digest != "" || wire != nil {
					t.Error("malformed authority returned receipt identity")
				}
				if !bytes.Equal(before, publicJSON(t, completed)) {
					t.Error("receipt rejection changed caller input")
				}
			})
		}
	}
}

func TestSourceAuthorityPreservesCanonicalSyntaxWithoutDNSPolicy(t *testing.T) {
	for _, uri := range []string{
		"https://example.com:444/environment.git",
		"https://exa!$&'()*+,;=mple.example/environment.git",
		"https://example%25name.example/environment.git",
		"https://[2606:4700:4700::1111]:444/environment.git",
		"https://[fe80::1%25eth0]/environment.git",
		"https://" + strings.Repeat("a", 300) + "/environment.git",
		"https://example.com/a%5D%3C%3E%22.git",
	} {
		t.Run(uri, func(t *testing.T) {
			// These are syntax-only controls. They grant no DNS, routability,
			// acquisition, source-ID binding or candidate approval authority.
			candidate := publicCandidate(t)
			candidate.Source.URI = uri
			checkPublicCandidate(t, candidate)
			completed := publicCompleted(t)
			completed.Source.URI = uri
			checkPublicReceipt(t, completed)
		})
	}
}
