//go:build linux

package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func readyTestInitialized(t *testing.T) paths.Roots {
	t.Helper()
	roots := freshTestRoots(t)
	a, err := freshInitialize(context.Background(), roots, freshTestRandom(), IntentPlan)
	if err != nil {
		if a.Lease != nil {
			err = errors.Join(err, a.Lease.Release())
		}
		t.Fatal(err)
	}
	if err = a.Lease.Release(); err != nil {
		t.Fatal(err)
	}
	return roots
}

func TestReadyObservationIntentAndInvalidLifetime(t *testing.T) {
	shared := []Intent{IntentDoctor, IntentExec, IntentShell}
	for value := 0; value <= 255; value++ {
		mode, ok := readyAdmissionMode(Intent(value))
		valid := Intent(value) >= IntentProfileList && Intent(value) <= IntentShell
		if ok != valid {
			t.Fatal("intent admission grammar changed")
		}
		if valid {
			want := safefs.LockExclusive
			if slices.Contains(shared, Intent(value)) {
				want = safefs.LockShared
			}
			if mode != want {
				t.Fatal("wrong intent lock")
			}
		} else if mode != 0 {
			t.Fatal("invalid intent exposed mode")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []context.Context{nil, ctx} {
		if o, err := observeReady(c, paths.Roots{}, IntentPlan); err == nil || o != nil {
			t.Fatal("invalid context retained observation")
		}
	}
	for _, intent := range []Intent{0, 255, IntentPlan} {
		if o, err := observeReady(context.Background(), paths.Roots{}, intent); err == nil || o != nil {
			t.Fatal("invalid roots observed")
		}
	}
	var nilObservation *readyObservation
	for _, o := range []*readyObservation{nilObservation, {}, {state: &readyObservationState{closed: true}}} {
		if got, err := o.facts(); err == nil || got != (readyFacts{}) {
			t.Fatal("invalid lifetime produced facts")
		}
		if err := o.close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadyObservationPublicReopenAndContentPreservation(t *testing.T) {
	roots := readyTestInitialized(t)
	ownerPath := filepath.Join(roots.State(), "writer-owner.json")
	for i, writer := range writerRecordFixtures() {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			original, err := os.ReadFile(filepath.Join(roots.State(), "root.json"))
			if err != nil {
				t.Fatal(err)
			}
			root, err := decodeRootRecord(original)
			if err != nil {
				t.Fatal(err)
			}
			writer.Instance = root.Instance
			data, err := encodeWriterOwnerRecord(writer)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(ownerPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			for intent := IntentProfileList; intent <= IntentShell; intent++ {
				o, err := observeReady(context.Background(), roots, intent)
				if err != nil {
					t.Fatal(err)
				}
				got, err := o.facts()
				if err != nil || got.instance != root.Instance || got.writer != writer {
					t.Fatal("wrong reopened facts", err)
				}
				if err = o.close(); err != nil {
					t.Fatal(err)
				}
			}
			unchanged, err := os.ReadFile(ownerPath)
			if err != nil || !bytes.Equal(data, unchanged) {
				t.Fatal("observer changed owner", err)
			}
		})
	}
}

func TestReadyObservationPublicLockLifetime(t *testing.T) {
	roots := readyTestInitialized(t)
	first, err := observeReady(context.Background(), roots, IntentDoctor)
	if err != nil {
		t.Fatal(err)
	}
	copyHandle := *first
	second, err := observeReady(context.Background(), roots, IntentExec)
	if err != nil {
		t.Fatal(err)
	}
	if o, err := observeReady(context.Background(), roots, IntentPlan); err == nil || o != nil {
		t.Fatal("exclusive observation bypassed readers")
	}
	if err = first.close(); err != nil {
		t.Fatal(err)
	}
	if got, err := copyHandle.facts(); err == nil || got != (readyFacts{}) {
		t.Fatal("copy outlived shared capability")
	}
	if o, err := observeReady(context.Background(), roots, IntentPlan); err == nil || o != nil {
		t.Fatal("one reader close released another")
	}
	if err = second.close(); err != nil {
		t.Fatal(err)
	}
	exclusive, err := observeReady(context.Background(), roots, IntentPlan)
	if err != nil {
		t.Fatal(err)
	}
	if o, err := observeReady(context.Background(), roots, IntentDoctor); err == nil || o != nil {
		t.Fatal("reader bypassed exclusive observation")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				_, _ = exclusive.facts()
				_ = exclusive.close()
			}
		}()
	}
	wg.Wait()
	if err = exclusive.close(); err != nil {
		t.Fatal(err)
	}
	again, err := observeReady(context.Background(), roots, IntentPlan)
	if err != nil {
		t.Fatal(err)
	}
	if err = again.close(); err != nil {
		t.Fatal(err)
	}
}

func TestReadyObservationPublicFailureFrontiers(t *testing.T) {
	roots := readyTestInitialized(t)
	for _, label := range []string{"admission-held", "root-record", "init.lock", "writer.lock", "writer-record", "candidates", "journals", "receipts", "data-owner", "cache-owner", "complete"} {
		for _, when := range []string{"before:", "after:"} {
			t.Run(when+label, func(t *testing.T) {
				injected := errors.New("observation interrupted")
				seen := false
				o, err := observeReadyWithBoundary(context.Background(), roots, IntentPlan, func(name string) error {
					if seen {
						t.Error("continued after failure")
					}
					if name == when+label {
						seen = true
						return injected
					}
					return nil
				})
				if !seen || o != nil || !errors.Is(err, injected) {
					t.Fatal("frontier failure lost", err)
				}
				// A leaked admission descriptor would prevent this exclusive reopen.
				again, err := observeReady(context.Background(), roots, IntentPlan)
				if err != nil {
					t.Fatal(err)
				}
				if err = again.close(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestReadyObservationPublicSubstitutionRevokes(t *testing.T) {
	for _, name := range []string{"root.json", "writer-owner.json", "init.lock", "admission.lock", "writer.lock", "candidates"} {
		t.Run(name, func(t *testing.T) {
			roots := readyTestInitialized(t)
			o, err := observeReady(context.Background(), roots, IntentPlan)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(roots.State(), name)
			if name == "candidates" {
				if err = os.Rename(target, target+".retained"); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(target+".substitute", data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err = os.Rename(target+".substitute", target); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := o.facts(); err == nil || got != (readyFacts{}) {
				t.Fatal("substitution retained observation")
			}
			if err = o.close(); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Lstat(target); err != nil {
				t.Fatal("close removed substituted evidence", err)
			}
		})
	}
}

func TestReadyObservationPublicCacheAndCancellation(t *testing.T) {
	roots := readyTestInitialized(t)
	outside := filepath.Join(roots.Cache(), "unrelated")
	if err := os.WriteFile(outside, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	o, err := observeReady(ctx, roots, IntentPlan)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := o.facts()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if got, err := o.facts(); err == nil || got != (readyFacts{}) {
		t.Fatal("cancelled observation retained facts")
	}
	if err = o.close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(roots.Cache(), "instances", facts.instance, "owner.json")); err != nil {
		t.Fatal(err)
	}
	if got, err := observeReady(context.Background(), roots, IntentPlan); err == nil || got != nil {
		t.Fatal("missing cache owner yielded complete observation")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "fixture" {
		t.Fatal("unrelated cache changed", err)
	}
	root, err := os.ReadFile(filepath.Join(roots.State(), "root.json"))
	if err != nil || !strings.Contains(string(root), `"status":"initialized"`) {
		t.Fatal("cache refusal changed durable authority", err)
	}
}
