package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ARCH-024/031: validate the closed retained predecessor shapes, without
// reconstructing history or treating a scalar-valid owner as live authority.
func TestProofWriterPredecessorShapes(t *testing.T) {
	refs := freshGraphRefs()
	for _, test := range []struct {
		name       string
		status     string
		generation uint64
		previous   uint64
		token      bool
		want       bool
	}{
		{"released-zero", "released", 0, 0, false, true},
		{"released-zero-token", "released", 0, 0, true, false},
		{"released-zero-predecessor", "released", 0, 1, false, false},
		{"held-zero", "held", 0, 0, false, false},
		{"held-one", "held", 1, 0, false, true},
		{"held-one-token", "held", 1, 0, true, false},
		{"released-one", "released", 1, 0, false, false},
		{"released-one-token", "released", 1, 0, true, false},
		{"held-two", "held", 2, 1, true, true},
		{"held-two-empty", "held", 2, 1, false, false},
		{"released-two", "released", 2, 1, true, true},
		{"released-two-empty", "released", 2, 1, false, false},
		{"held-three-released-predecessor", "held", 3, 2, false, true},
		{"held-three-held-predecessor", "held", 3, 2, true, true},
		{"released-three", "released", 3, 2, true, true},
		{"released-four", "released", 4, 3, true, true},
		{"held-skipped-predecessor", "held", 4, 2, true, false},
		{"released-skipped-predecessor", "released", 4, 2, true, false},
		{"held-max", "held", ^uint64(0), ^uint64(0) - 1, true, true},
		{"released-max", "released", ^uint64(0), ^uint64(0) - 1, true, true},
		{"wrapped-zero", "held", 0, ^uint64(0), true, false},
		{"unknown-status", "other", 3, 2, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			previousToken := ""
			if test.token {
				previousToken = refs.priorWriterToken
			}
			writer := releasedWriter(refs.instance, "", test.generation, test.previous, previousToken)
			writer.Status = test.status
			if test.status == "held" {
				writer.Token = refs.writerToken
			}
			if got := validWriterPredecessor(writer); got != test.want {
				t.Fatalf("validWriterPredecessor=%t, want %t", got, test.want)
			}
		})
	}
}

// PC-061 / ARCH-018/024/031: READY without a current is not limited to the
// freshly initialized generation. Classification itself must preserve evidence.
func TestProofCurrentlessReleasedWriterClassification(t *testing.T) {
	for _, generation := range []uint64{0, 2, 3, 4, ^uint64(0)} {
		t.Run(fmt.Sprint(generation), func(t *testing.T) {
			s, d, refs := makeGraph(t.TempDir(), "READY(∅)")
			writer := releasedWriter(refs.instance, "", 0, 0, "")
			if generation != 0 {
				writer = releasedWriter(refs.instance, "", generation, generation-1, refs.priorWriterToken)
			}
			if err := writeRecord(filepath.Join(s, "writer-owner.json"), writer); err != nil {
				t.Fatal(err)
			}
			// Fully validated immutable candidates may remain unreferenced.
			if err := writeRecord(filepath.Join(s, "candidates", refs.a+".json"), candidateReference(refs.instance, refs.a)); err != nil {
				t.Fatal(err)
			}
			assertProofClassificationPreserves(t, s, d, "READY(∅)")
		})
	}
}

func TestProofCurrentlessInvalidWriterClassification(t *testing.T) {
	for _, name := range []string{
		"released-one", "released-empty-predecessor", "released-skipped-predecessor",
		"released-pid", "released-start", "released-namespace", "released-process",
		"held-one-token", "held-two-empty", "held-zero", "held-without-current",
		"orphan-receipt", "extra-entry",
	} {
		t.Run(name, func(t *testing.T) {
			s, d, refs := makeGraph(t.TempDir(), "READY(∅)")
			writer := releasedWriter(refs.instance, "", 2, 1, refs.priorWriterToken)
			if name == "released-pid" || name == "released-start" || name == "released-namespace" || name == "released-process" {
				writer = releasedWriter(refs.instance, "", 0, 0, "")
			}
			switch name {
			case "released-one":
				writer.Generation, writer.PreviousGeneration = 1, 0
			case "released-empty-predecessor":
				writer.PreviousToken = ""
			case "released-skipped-predecessor":
				writer.PreviousGeneration = 0
			case "released-pid":
				writer.Process.PID = 1
			case "released-start":
				writer.Process.Start = 1
			case "released-namespace":
				writer.Process.PIDNamespace = 1
			case "released-process":
				writer.Process = processIdentity{PID: 1, Start: 1, PIDNamespace: 1}
			case "held-one-token", "held-two-empty", "held-zero", "held-without-current":
				writer = heldWriter(refs.instance, refs.writerToken, 1, processIdentity{PID: 1, Start: 1, PIDNamespace: 1}, 0, "")
				switch name {
				case "held-one-token":
					writer.PreviousToken = refs.priorWriterToken
				case "held-two-empty":
					writer.Generation, writer.PreviousGeneration = 2, 1
				case "held-zero":
					writer.Generation = 0
				}
			case "orphan-receipt":
				if err := writeRecord(filepath.Join(s, "receipts", refs.receiptA+".json"), pendingReceiptFor(refs.a, "")); err != nil {
					t.Fatal(err)
				}
			case "extra-entry":
				if err := writeRecord(filepath.Join(s, "extra.json"), initializedRoot(refs.instance)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeRecord(filepath.Join(s, "writer-owner.json"), writer); err != nil {
				t.Fatal(err)
			}
			if name == "released-pid" || name == "released-start" || name == "released-namespace" || name == "released-process" {
				if _, _, ok := readCurrentAuthority(s, d, os.Getuid()); ok {
					t.Fatal("released owner admitted a nonzero process tuple")
				}
			}
			assertProofClassificationPreserves(t, s, d, "BAD")
		})
	}
}

func assertProofClassificationPreserves(t *testing.T, s, d, want string) {
	t.Helper()
	roots := map[string]string{"state": s, "data": d}
	before, err := actualFixtureObjects(roots)
	if err != nil {
		t.Fatal(err)
	}
	got := classify(s, d)
	after, err := actualFixtureObjects(roots)
	if err != nil {
		t.Fatal(err)
	}
	if (want == "BAD" && !strings.HasPrefix(got, want)) || (want != "BAD" && got != want) {
		t.Fatalf("classify=%q, want %q", got, want)
	}
	if !fixtureObjectsEqual(before, after) {
		t.Fatal("classification changed disposable fixture evidence")
	}
}
