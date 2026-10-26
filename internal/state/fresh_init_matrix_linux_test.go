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
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func freshTestComplete(t *testing.T, roots paths.Roots, intent Intent) {
	t.Helper()
	a, err := freshInitialize(context.Background(), roots, freshTestRandom(), intent)
	if a.Lease == nil {
		t.Fatal("producer lost resource ownership")
	}
	t.Cleanup(func() {
		if err := a.Lease.Release(); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := AdmissionExclusive
	if intent == IntentBootstrap {
		want = AdmissionWriterExclusive
	}
	if !a.Admitted || a.Mode != want || a.Lease.Mode() != want || a.Snapshot.Classification != Ready {
		t.Fatal("incorrect completed acquisition")
	}
	if err = a.Lease.Revalidate(); err != nil {
		t.Fatal(err)
	}
	state := freshTestRoot(t, roots.State())
	names, err := state.Names(8)
	if err != nil || !slices.Equal(names, freshNames(freshReady, "", "")) {
		t.Fatal("incorrect complete state layout", err)
	}
	if err = a.Lease.Release(); err != nil {
		t.Fatal(err)
	}
	data, _, err := state.ReadFile("writer-owner.json", initRecordLimit)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := decodeWriterOwner(data)
	if err != nil {
		t.Fatal(err)
	}
	wantGeneration := uint64(0)
	if intent == IntentBootstrap {
		wantGeneration = 2
	}
	if owner.Status != "released" || owner.Generation != wantGeneration {
		t.Fatal("incorrect completed release")
	}
}

func freshTestSameRoot(t *testing.T, name string, previous safefs.Identity) {
	t.Helper()
	d := freshTestRoot(t, name)
	if _, err := d.Names(freshParentLimit); err != nil {
		t.Fatal(err)
	}
	now := d.Identity()
	if now.Device != previous.Device || now.Inode != previous.Inode || now.MountID != previous.MountID || now.Owner != previous.Owner || now.Mode != previous.Mode {
		t.Fatal("existing empty root identity or mode changed")
	}
}

func TestFreshInitializeRemainingCleanPairs(t *testing.T) {
	for _, intent := range []Intent{IntentProfileList, IntentPlan, IntentBootstrap} {
		for _, test := range []struct {
			name  string
			empty [2]os.FileMode
		}{
			{"empty-absent", [2]os.FileMode{0o700, 0}},
			{"absent-empty", [2]os.FileMode{0, 0o750}},
			{"empty-empty", [2]os.FileMode{0o700, 0o755}},
		} {
			t.Run(fmt.Sprint(intent)+"/"+test.name, func(t *testing.T) {
				roots := freshTestLayout(t, [3]string{"state", "data", "cache"}, true, test.empty)
				var previous [2]safefs.Identity
				for i, name := range []string{roots.State(), roots.Data()} {
					if test.empty[i] != 0 {
						previous[i] = freshTestRoot(t, name).Identity()
					}
				}
				freshTestComplete(t, roots, intent)
				for i, name := range []string{roots.State(), roots.Data()} {
					if test.empty[i] != 0 {
						freshTestSameRoot(t, name, previous[i])
					}
				}
			})
		}
	}
}

func TestFreshInitializeMissingPrefixLayouts(t *testing.T) {
	for _, intent := range []Intent{IntentProfileList, IntentPlan, IntentBootstrap} {
		for _, test := range []struct {
			name  string
			bases [3]string
		}{
			{"shared", [3]string{"shared/nested/state", "shared/nested/data", "shared/nested/cache"}},
			{"independent", [3]string{"state-first/state-second/state-third", "data-first/data-second/data-third", "cache-first/cache-second/cache-third"}},
		} {
			t.Run(fmt.Sprint(intent)+"/"+test.name, func(t *testing.T) {
				roots := freshTestLayout(t, test.bases, false, [2]os.FileMode{})
				created := 0
				a, err := freshInitializeWithBoundary(context.Background(), roots, freshTestRandom(), intent, func(name string) error {
					if name == "after:prefix-create" {
						created++
					}
					return nil
				})
				if a.Lease == nil {
					t.Fatal("producer lost ownership")
				}
				t.Cleanup(func() {
					if err := a.Lease.Release(); err != nil {
						t.Error(err)
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				if !a.Admitted || a.Snapshot.Classification != Ready {
					t.Fatal("missing prefixes did not reach READY")
				}
				if err = a.Lease.Revalidate(); err != nil {
					t.Fatal(err)
				}
				want := 8
				if test.name == "independent" {
					want = 12
				}
				if created != want {
					t.Fatalf("created %d prefix edges; want %d", created, want)
				}
				if err = a.Lease.Release(); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{roots.State(), roots.Data(), roots.Cache()} {
					d := freshTestRoot(t, name)
					if _, err := d.Names(freshParentLimit); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestFreshInitializePreservesUnrelatedCache(t *testing.T) {
	for _, intent := range []Intent{IntentProfileList, IntentPlan, IntentBootstrap} {
		t.Run(fmt.Sprint(intent), func(t *testing.T) {
			roots := freshTestRoots(t)
			other := strings.Repeat("ab", 16)
			otherPath := filepath.Join(roots.Cache(), "instances", other)
			if err := os.MkdirAll(otherPath, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, item := range []struct {
				dir, name string
				data      []byte
			}{
				{roots.Cache(), "unrelated", []byte("unrelated cache sentinel")},
				{otherPath, "owner.json", []byte("unrelated instance sentinel, deliberately not an owner record")},
			} {
				if err := os.WriteFile(filepath.Join(item.dir, item.name), item.data, 0o600); err != nil {
					t.Fatal(err)
				}
				d := freshTestRoot(t, item.dir)
				pin, data, err := d.PinFile(item.name, initRecordLimit)
				if err != nil || !bytes.Equal(data, item.data) {
					t.Fatal("could not capture unrelated sentinel", err)
				}
				t.Cleanup(func() {
					if err := pin.Close(); err != nil {
						t.Error(err)
					}
				})
				t.Cleanup(func() {
					if err := pin.Revalidate(); err != nil {
						t.Error("unrelated cache changed", err)
					}
				})
			}
			freshTestComplete(t, roots, intent)
			instance := freshTestRoot(t, filepath.Join(roots.Cache(), "instances", strings.Repeat("01", 16)))
			names, err := instance.Names(1)
			if err != nil || !slices.Equal(names, []string{"owner.json"}) {
				t.Fatal("fresh cache instance not created correctly", err)
			}
			instances := freshTestRoot(t, filepath.Join(roots.Cache(), "instances"))
			names, err = instances.Names(2)
			if err != nil || !slices.Equal(names, []string{strings.Repeat("01", 16), other}) {
				t.Fatal("unrelated instance namespace changed", err)
			}
		})
	}
}

func TestFreshInitializeCacheInstanceCollisionRefuses(t *testing.T) {
	for _, intent := range []Intent{IntentProfileList, IntentPlan, IntentBootstrap} {
		t.Run(fmt.Sprint(intent), func(t *testing.T) {
			roots := freshTestRoots(t)
			instanceID := strings.Repeat("01", 16)
			instancePath := filepath.Join(roots.Cache(), "instances", instanceID)
			if err := os.MkdirAll(instancePath, 0o700); err != nil {
				t.Fatal(err)
			}
			instance := freshTestRoot(t, instancePath)
			previous := instance.Identity()
			// Even a matching marker must not turn a random-name collision into ownership.
			marker, err := encodeNamespaceOwner(namespaceOwner{Schema: 1, Domain: "zerb.cache-owner", Instance: instanceID})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(instancePath, "owner.json"), marker, 0o600); err != nil {
				t.Fatal(err)
			}
			pin, _, err := instance.PinFile("owner.json", initRecordLimit)
			if err != nil {
				t.Fatal(err)
			}
			defer pin.Close()
			a, err := freshInitialize(context.Background(), roots, freshTestRandom(), intent)
			if err == nil || a.Lease == nil || a.Admitted || a.Mode != 0 || a.Snapshot != (Snapshot{}) {
				t.Fatal("cache collision was adopted")
			}
			if a.Lease.Revalidate() == nil {
				t.Fatal("collision error lease permitted continuation")
			}
			if release := a.Lease.Release(); !errors.Is(release, err) {
				t.Fatal("collision release lost primary error", release)
			}
			if err = a.Lease.Release(); err != nil {
				t.Fatal("repeated collision release", err)
			}
			freshTestSameRoot(t, instancePath, previous)
			if err = pin.Revalidate(); err != nil {
				t.Fatal("collision marker changed", err)
			}
			names, err := instance.Names(1)
			if err != nil || !slices.Equal(names, []string{"owner.json"}) {
				t.Fatal("collision evidence changed", err)
			}
			root, _, err := freshTestRoot(t, roots.State()).ReadFile("root.json", initRecordLimit)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := decodeRootRecord(root)
			if err != nil || claim.Status != "initializing" {
				t.Fatal("collision completed initialization", err)
			}
		})
	}
}
