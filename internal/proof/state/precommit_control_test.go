//go:build linux

package state

import (
	"math"
	"path/filepath"
	"testing"
)

// ARCH-024/025/027/031; VAL-026/031/032: exercise the actual disposable graph.
// These fixtures retain independently valid old current and new operation IDs.
func proofPrecommitFixture(t *testing.T) (string, string, graphRefs, currentRecord, writerOwnerRecord, journalRecord) {
	t.Helper()
	s, d, refs := makeGraph(t.TempDir(), "TRANSITION(A,B)")
	assertProofClassificationPreserves(t, s, d, "TRANSITION(A,B)")
	var current currentRecord
	var writer writerOwnerRecord
	var journal journalRecord
	for _, record := range []struct {
		name  string
		value any
	}{
		{"current.json", &current}, {"writer-owner.json", &writer}, {filepath.Join("journals", refs.journal+".json"), &journal},
	} {
		if err := readRecord(filepath.Join(s, record.name), record.value); err != nil {
			t.Fatal(err)
		}
	}
	if current.Candidate != journal.Previous || current.Receipt != journal.PreviousReceipt ||
		current.OperationID == journal.OperationID || current.OperationToken == journal.OperationToken ||
		current.WriterGeneration != 1 || writer.Generation != 3 || writer.PreviousToken != "" ||
		journal.WriterGeneration != writer.Generation || journal.WriterToken != writer.Token || journal.WriterPreviousToken != writer.PreviousToken {
		t.Fatal("fixture did not retain the expected independent old-current/new-operation relation")
	}
	current.WriterGeneration = 3
	writer.Generation, writer.PreviousGeneration = 5, 4
	journal.WriterGeneration = writer.Generation
	return s, d, refs, current, writer, journal
}

func proofPrecommitWrite(t *testing.T, s string, refs graphRefs, current currentRecord, writer writerOwnerRecord, journal journalRecord) {
	t.Helper()
	// Refusal must concern the cross-record lifecycle, not a malformed scalar.
	if !validCurrentIdentity(current, refs.instance) || !validWriterPredecessor(writer) || !validJournalPrefix(journal) {
		t.Fatal("precommit mutation broke an independent scalar/prefix fixture")
	}
	for _, record := range []struct {
		name  string
		value any
	}{
		{"current.json", current}, {"writer-owner.json", writer}, {filepath.Join("journals", refs.journal+".json"), journal},
	} {
		if err := writeRecord(filepath.Join(s, record.name), record.value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProofPrecommitControlOwnerDistances(t *testing.T) {
	for _, test := range []struct {
		name            string
		current, writer uint64
		transfer, ahead bool
		want            string
	}{
		{"fresh-gap-one", 3, 4, false, false, "BAD corrupt"},
		{"fresh-gap-two", 3, 5, false, false, "TRANSITION(A,B)"},
		{"fresh-gap-one-journal-ahead", 3, 4, false, true, "BAD corrupt"},
		{"fresh-gap-two-journal-ahead", 3, 5, false, true, "TRANSITION(A,B)"},
		{"transfer-gap-two", 3, 5, true, false, "BAD corrupt"},
		{"transfer-gap-three", 3, 6, true, false, "TRANSITION(A,B)"},
		{"transfer-gap-two-journal-ahead", 3, 5, true, true, "BAD corrupt"},
		{"transfer-gap-three-journal-ahead", 3, 6, true, true, "TRANSITION(A,B)"},
		{"fresh-max-gap-one", math.MaxUint64 - 1, math.MaxUint64, false, false, "BAD corrupt"},
		{"fresh-max-gap-two", math.MaxUint64 - 2, math.MaxUint64, false, false, "TRANSITION(A,B)"},
		{"transfer-max-gap-two", math.MaxUint64 - 2, math.MaxUint64, true, false, "BAD corrupt"},
		{"transfer-max-gap-three", math.MaxUint64 - 3, math.MaxUint64, true, false, "TRANSITION(A,B)"},
		{"transfer-max-journal-ahead", math.MaxUint64 - 4, math.MaxUint64 - 1, true, true, "TRANSITION(A,B)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, d, refs, current, writer, journal := proofPrecommitFixture(t)
			current.WriterGeneration = test.current
			writer.Generation, writer.PreviousGeneration = test.writer, test.writer-1
			if test.transfer {
				writer.PreviousToken = refs.token
			}
			journal.WriterGeneration, journal.WriterPreviousToken = writer.Generation, writer.PreviousToken
			if test.ahead {
				journal.WriterGeneration++
				journal.WriterPreviousToken = writer.Token
				var err error
				journal.WriterToken, err = FreshToken()
				if err != nil {
					t.Fatal(err)
				}
			}
			proofPrecommitWrite(t, s, refs, current, writer, journal)
			assertProofClassificationPreserves(t, s, d, test.want)
		})
	}
}

func TestProofPrecommitControlWriterTokenExclusions(t *testing.T) {
	for _, name := range []string{"legal-current-writer", "legal-journal-ahead", "old-current-token-as-writer", "old-current-token-as-predecessor", "old-current-token-as-ahead-journal", "instance-as-ahead-journal", "predecessor-as-ahead-journal"} {
		t.Run(name, func(t *testing.T) {
			s, d, refs, current, writer, journal := proofPrecommitFixture(t)
			// The transferred distance is legal before introducing one token collision.
			writer.Generation, writer.PreviousGeneration, writer.PreviousToken = 6, 5, refs.token
			journal.WriterGeneration, journal.WriterPreviousToken = writer.Generation, writer.PreviousToken
			want := "BAD corrupt"
			switch name {
			case "legal-current-writer":
				want = "TRANSITION(A,B)"
			case "old-current-token-as-writer":
				writer.Token, journal.WriterToken = current.WriterToken, current.WriterToken
			case "old-current-token-as-predecessor":
				writer.PreviousToken, journal.WriterPreviousToken = current.WriterToken, current.WriterToken
			default:
				journal.WriterGeneration++
				journal.WriterPreviousToken = writer.Token
				var err error
				journal.WriterToken, err = FreshToken()
				if err != nil {
					t.Fatal(err)
				}
				switch name {
				case "legal-journal-ahead":
					want = "TRANSITION(A,B)"
				case "old-current-token-as-ahead-journal":
					journal.WriterToken = current.WriterToken
				case "instance-as-ahead-journal":
					journal.WriterToken = refs.instance
				case "predecessor-as-ahead-journal":
					journal.WriterToken = writer.PreviousToken
				}
			}
			proofPrecommitWrite(t, s, refs, current, writer, journal)
			assertProofClassificationPreserves(t, s, d, want)
		})
	}
}

// ARCH-024/027/031: the journal-ahead interval cannot advance committed
// current beyond the independently retained writer-owner publication.
func TestProofPrecommitControlRetainedWriterAfterCommit(t *testing.T) {
	for _, futureCurrent := range []bool{false, true} {
		name, want := "retained-current-journal-ahead", "CLEANUP(B)"
		if futureCurrent {
			name, want = "future-current-journal-ahead", "BAD corrupt"
		}
		t.Run(name, func(t *testing.T) {
			s, d, refs := makeGraph(t.TempDir(), "CLEANUP(B)")
			assertProofClassificationPreserves(t, s, d, "CLEANUP(B)")
			var current currentRecord
			var writer writerOwnerRecord
			var journal journalRecord
			for _, record := range []struct {
				name  string
				value any
			}{
				{"current.json", &current}, {"writer-owner.json", &writer}, {filepath.Join("journals", refs.journal+".json"), &journal},
			} {
				if err := readRecord(filepath.Join(s, record.name), record.value); err != nil {
					t.Fatal(err)
				}
			}
			if current.WriterGeneration != 3 || current.WriterGeneration != writer.Generation || journal.WriterGeneration != writer.Generation ||
				current.WriterToken != writer.Token || journal.WriterToken != writer.Token ||
				current.OperationID != journal.OperationID || current.OperationToken != journal.OperationToken || current.Candidate != journal.Candidate {
				t.Fatal("cleanup fixture did not retain the committed writer/operation relation")
			}
			journal.WriterGeneration++
			journal.WriterPreviousToken = writer.Token
			var err error
			journal.WriterToken, err = FreshToken()
			if err != nil {
				t.Fatal(err)
			}
			if futureCurrent {
				current.WriterGeneration, current.WriterToken = journal.WriterGeneration, journal.WriterToken
			}
			proofPrecommitWrite(t, s, refs, current, writer, journal)
			assertProofClassificationPreserves(t, s, d, want)
		})
	}
}
