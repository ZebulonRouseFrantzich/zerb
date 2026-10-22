package receipt_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

var digestSyntax = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)

func TestParseDigestPreservesExactCanonicalToken(t *testing.T) {
	for _, payload := range []string{strings.Repeat("0", 64), strings.Repeat("f", 64), strings.Repeat("0123456789abcdef", 4)} {
		value := "sha256:" + payload
		got, err := receipt.ParseDigest(value)
		if err != nil || string(got) != value {
			t.Fatalf("canonical token was changed or rejected: error %v", err)
		}
	}
}

func TestParseDigestChecksEveryByteAgainstOwnedGrammar(t *testing.T) {
	canonical := "sha256:" + strings.Repeat("0", 64)
	for position := range len(canonical) {
		for value := range 256 {
			input := []byte(canonical)
			input[position] = byte(value)
			checkDigest(t, string(input))
		}
	}
}

func TestParseDigestRejectsLengthAliasesAndNonCanonicalForms(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	cases := []string{
		"", "sha256:", strings.Repeat("a", 64), valid[:70], valid + "a",
		" " + valid, valid + " ", valid + "\n", valid + "\x00", "\n" + valid,
		"SHA256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64),
		"sha256:" + strings.Repeat("0", 63) + "g", "sha512:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("ａ", 64), "sha256:" + strings.Repeat("a", 63) + "\u202e",
		strings.Repeat("a", 1<<20),
	}
	for index, input := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) { checkDigest(t, input) })
	}
}

func TestParseDigestErrorDoesNotExposeInput(t *testing.T) {
	const marker = "private-authored-metadata"
	input := "sha256:" + marker
	got, err := receipt.ParseDigest(input)
	if err == nil || got != "" || strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), input) {
		t.Fatal("invalid token was accepted, returned partially, or exposed in the error")
	}
}

var (
	digestSink receipt.Digest
	errorSink  error
)

func TestParseDigestDoesNotAllocatePerCall(t *testing.T) {
	for _, input := range []string{"sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64), strings.Repeat("a", 1<<20)} {
		allocations := testing.AllocsPerRun(100, func() {
			digestSink, errorSink = receipt.ParseDigest(input)
		})
		if allocations != 0 {
			t.Fatalf("fixed-token boundary allocated %g times per call", allocations)
		}
	}
}

func FuzzParseDigest(f *testing.F) {
	for _, seed := range []string{"", "sha256:" + strings.Repeat("0", 64), "sha256:" + strings.Repeat("f", 64), "sha256:" + strings.Repeat("a", 64) + "\n", string([]byte{0xff})} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) { checkDigest(t, input) })
}

func checkDigest(t *testing.T, input string) {
	t.Helper()
	got, err := receipt.ParseDigest(input)
	if digestSyntax.MatchString(input) {
		if err != nil || string(got) != input {
			t.Fatal("canonical token was rejected or normalized")
		}
	} else if err == nil || got != "" {
		t.Fatal("invalid token was accepted or returned partially")
	}
}
