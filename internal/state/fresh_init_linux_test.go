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

	"golang.org/x/sys/unix"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func freshTestRoots(t *testing.T) paths.Roots {
	t.Helper()
	base := t.TempDir()
	for _, name := range []string{"home", "state", "data", "cache"} {
		if err := os.Mkdir(filepath.Join(base, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	input, err := paths.NewInput(filepath.Join(base, "home"), []string{"XDG_STATE_HOME=" + filepath.Join(base, "state"), "XDG_DATA_HOME=" + filepath.Join(base, "data"), "XDG_CACHE_HOME=" + filepath.Join(base, "cache")})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := paths.Resolve(input)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := safefs.OpenRoles(roots, uint32(os.Geteuid()))
	if err != nil {
		t.Skip("exported protected-root/topology admission unavailable; skipped before product mutation")
	}
	if err = roles.Validate(); err != nil {
		t.Fatal(err)
	}
	if err = roles.Close(); err != nil {
		t.Fatal(err)
	}
	return roots
}

func freshTestRandom() *bytes.Reader {
	var data []byte
	for i := byte(1); i <= 6; i++ {
		data = append(data, bytes.Repeat([]byte{i}, 16)...)
	}
	return bytes.NewReader(data)
}

func freshTestRoot(t *testing.T, name string) *safefs.Dir {
	t.Helper()
	d, err := safefs.OpenRoot(name, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	return d
}

func TestFreshInitializeRefusalOwnsLease(t *testing.T) {
	for _, intent := range []Intent{0, IntentDoctor, IntentApply, IntentExec, IntentShell, IntentRecoverRepair, IntentRecoverResume, 255} {
		a, err := freshInitialize(context.Background(), paths.Roots{}, freshTestRandom(), intent)
		if err == nil || a.Lease == nil || a.Admitted || a.Mode != 0 || a.Snapshot != (Snapshot{}) {
			t.Fatal("unsupported intent acquired authority")
		}
		if a.Lease.Mode() != 0 {
			t.Fatal("errored lease advertised an active mode")
		}
		if a.Lease.Revalidate() == nil {
			t.Fatal("errored lease usable")
		}
		if !errors.Is(a.Lease.Release(), err) {
			t.Fatal("primary error lost on release")
		}
		if err := a.Lease.Release(); err != nil {
			t.Fatal("repeated release", err)
		}
	}
	failed := &Lease{state: &freshSession{mode: AdmissionWriterExclusive, primary: errFreshInit}}
	if failed.Mode() != 0 {
		t.Fatal("failed session advertised cached lock mode")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, err := freshInitialize(ctx, paths.Roots{}, freshTestRandom(), IntentPlan)
	if err == nil || a.Lease == nil {
		t.Fatal("cancelled producer lost ownership")
	}
	if a.Lease.Release() == nil {
		t.Fatal("cancelled release lost error")
	}
}

func TestFreshInitializeObservedSubstitutionPreservesEvidence(t *testing.T) {
	for _, test := range []struct {
		name, boundary, relative string
		directory, alter         bool
	}{
		{"init-inode", "after:init-lock-exclusive", "init.lock", false, false},
		{"admission-inode", "after:admission.lock-publish", "admission.lock", false, false},
		{"writer-inode", "after:writer.lock-publish", "writer.lock", false, false},
		{"claim-inode", "after:root-claim-publish", "root.json", false, false},
		{"claim-bytes", "after:root-claim-publish", "root.json", false, true},
		{"writer-owner-inode", "after:writer-released-zero-publish", "writer-owner.json", false, false},
		{"control-directory", "after:candidates-create", "candidates", true, false},
		{"witness-inode", "after:initialization-witness", "replace-witness.json", false, false},
		{"stage-inode", "after:initialization-link", ".replace-" + strings.Repeat("03", 16), false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			roots := freshTestRoots(t)
			changed := false
			var evidence []byte
			target := filepath.Join(roots.State(), test.relative)
			a, err := freshInitializeWithBoundary(context.Background(), roots, freshTestRandom(), IntentBootstrap, func(name string) error {
				if changed {
					t.Error("advanced after observed substitution")
					return errors.New("advanced after substitution")
				}
				if name != test.boundary {
					return nil
				}
				if test.directory {
					if err := os.Rename(target, target+".displaced"); err != nil {
						return err
					}
					if err := os.Mkdir(target, 0o700); err != nil {
						return err
					}
				} else {
					var err error
					evidence, err = os.ReadFile(target)
					if err != nil {
						return err
					}
					if test.alter {
						evidence = append(evidence, ' ')
						if err = os.WriteFile(target, evidence, 0o600); err != nil {
							return err
						}
					} else {
						if err = os.WriteFile(target+".substitute", evidence, 0o600); err != nil {
							return err
						}
						if err = os.Rename(target+".substitute", target); err != nil {
							return err
						}
					}
				}
				changed = true
				return nil
			})
			if !changed || err == nil || a.Admitted || a.Mode != 0 || a.Lease == nil || a.Lease.Mode() != 0 {
				t.Fatal("substituted graph admitted or lost ownership", err)
			}
			if a.Lease.Revalidate() == nil {
				t.Fatal("errored substitution lease usable")
			}
			if release := a.Lease.Release(); release == nil {
				t.Fatal("substitution primary lost")
			}
			if test.directory {
				info, err := os.Lstat(target)
				if err != nil || !info.IsDir() {
					t.Fatal("substituted directory removed", err)
				}
				if _, err = os.Lstat(target + ".displaced"); err != nil {
					t.Fatal("displaced evidence removed", err)
				}
			} else {
				actual, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(actual, evidence) {
					t.Fatal("substituted evidence changed or removed", err)
				}
			}
			if err = a.Lease.Release(); err != nil {
				t.Fatal("repeat release", err)
			}
		})
	}
}

func TestFreshInitializeAdmissionAndRelease(t *testing.T) {
	for _, intent := range []Intent{IntentProfileList, IntentPlan, IntentBootstrap} {
		t.Run(fmt.Sprint(intent), func(t *testing.T) {
			roots := freshTestRoots(t)
			a, err := freshInitialize(context.Background(), roots, freshTestRandom(), intent)
			if err != nil {
				if release := a.Lease.Release(); release == nil {
					t.Error("error lease lost primary")
				}
				t.Fatal(err)
			}
			defer a.Lease.Release()
			want := AdmissionExclusive
			if intent == IntentBootstrap {
				want = AdmissionWriterExclusive
			}
			if !a.Admitted || a.Mode != want || a.Lease.Mode() != want || a.Snapshot.Classification != Ready {
				t.Fatal("incorrect returned acquisition")
			}
			if err = a.Lease.Revalidate(); err != nil {
				t.Fatal(err)
			}
			state := freshTestRoot(t, roots.State())
			names, err := state.Names(8)
			if err != nil || !slices.Equal(names, freshNames(freshReady, "", "")) {
				t.Fatal("incomplete READY namespace", err)
			}
			competitor, err := state.OpenLock("admission.lock", false)
			if err != nil {
				t.Fatal(err)
			}
			if err = competitor.Try(safefs.LockExclusive); !errors.Is(err, unix.EAGAIN) {
				t.Fatal("handoff dropped admission exclusion", err)
			}
			if err = competitor.Close(); err != nil {
				t.Fatal(err)
			}
			init, err := state.OpenLock("init.lock", false)
			if err != nil {
				t.Fatal(err)
			}
			if err = init.Try(safefs.LockExclusive); err != nil {
				t.Fatal("I-X not consumed", err)
			}
			if err = init.Close(); err != nil {
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
			if intent == IntentBootstrap {
				if owner.Status != "held" || owner.Generation != 1 {
					t.Fatal("bootstrap did not acquire held1")
				}
			} else if owner.Status != "released" || owner.Generation != 0 {
				t.Fatal("plan/profile changed writer owner")
			}
			if err = a.Lease.Release(); err != nil {
				t.Fatal(err)
			}
			if intent == IntentBootstrap {
				data, _, err = state.ReadFile("writer-owner.json", initRecordLimit)
				if err != nil {
					t.Fatal(err)
				}
				owner, err = decodeWriterOwner(data)
				if err != nil || owner.Status != "released" || owner.Generation != 2 {
					t.Fatal("release did not publish released2", err)
				}
			}
			competitor, err = state.OpenLock("admission.lock", false)
			if err != nil {
				t.Fatal(err)
			}
			if err = competitor.Try(safefs.LockExclusive); err != nil {
				t.Fatal("release stranded exclusion", err)
			}
			if err = competitor.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFreshInitializeNon0700StateRefusesBeforeMutation(t *testing.T) {
	for _, mode := range []os.FileMode{0o750, 0o755} {
		t.Run(fmt.Sprintf("%o", mode), func(t *testing.T) {
			roots := freshTestRoots(t)
			if err := os.Mkdir(roots.State(), mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(roots.State(), mode); err != nil {
				t.Fatal(err)
			}
			d := freshTestRoot(t, roots.State())
			if _, err := d.Names(0); err != nil {
				t.Fatal(err)
			}
			a, err := freshInitialize(context.Background(), roots, freshTestRandom(), IntentPlan)
			if err == nil || a.Snapshot != (Snapshot{}) || a.Mode != 0 {
				t.Fatal("unfinishable state mode admitted")
			}
			if a.Lease.Release() == nil {
				t.Fatal("error lost")
			}
			if _, err = d.Names(0); err != nil {
				t.Fatal("refusal wrote state", err)
			}
			if d.Identity().Mode&0o777 != uint32(mode) {
				t.Fatal("state mode changed")
			}
			for _, name := range []string{roots.Data(), roots.Cache()} {
				if _, err := os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("refusal wrote another product root")
				}
			}
		})
	}
}

func TestFreshInitializeExistingEvidenceAndUnsafePresence(t *testing.T) {
	for _, name := range []string{"init.lock", "root.json", "unknown"} {
		t.Run(name, func(t *testing.T) {
			roots := freshTestRoots(t)
			if err := os.Mkdir(roots.State(), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(roots.State(), name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			a, err := freshInitialize(context.Background(), roots, freshTestRandom(), IntentPlan)
			if err == nil || a.Admitted || a.Snapshot != (Snapshot{}) {
				t.Fatal("pre-existing evidence adopted")
			}
			if a.Lease.Release() == nil {
				t.Fatal("error lost")
			}
			names, err := freshTestRoot(t, roots.State()).Names(1)
			if err != nil || !slices.Equal(names, []string{name}) {
				t.Fatal("evidence modified", err)
			}
		})
	}
	t.Run("unsafe-present-directory", func(t *testing.T) {
		roots := freshTestRoots(t)
		if err := os.Symlink(roots.Home(), roots.State()); err != nil {
			t.Fatal(err)
		}
		a, err := freshInitialize(context.Background(), roots, freshTestRandom(), IntentPlan)
		if err == nil {
			t.Fatal("unsafe presence treated as absent")
		}
		if a.Lease.Release() == nil {
			t.Fatal("error lost")
		}
		target, err := os.Readlink(roots.State())
		if err != nil || target != roots.Home() {
			t.Fatal("unsafe presence changed")
		}
	})
}

func TestFreshInitializeBoundaryFailureOwnership(t *testing.T) {
	// Real successful I/O supplies the matrix; no mock claims an executed boundary.
	roots := freshTestRoots(t)
	var trace []string
	a, err := freshInitializeWithBoundary(context.Background(), roots, freshTestRandom(), IntentBootstrap, func(name string) error { trace = append(trace, name); return nil })
	if err != nil {
		_ = a.Lease.Release()
		t.Fatal(err)
	}
	if err = a.Lease.Release(); err != nil {
		t.Fatal(err)
	}
	for index, name := range trace {
		t.Run(fmt.Sprintf("%03d-%s", index, name), func(t *testing.T) {
			roots := freshTestRoots(t)
			failure := errors.New("injected boundary failure")
			ordinal := 0
			a, err := freshInitializeWithBoundary(context.Background(), roots, freshTestRandom(), IntentBootstrap, func(string) error {
				at := ordinal
				ordinal++
				if at == index {
					return failure
				}
				return nil
			})
			if err == nil {
				err = a.Lease.Release()
			} else {
				if a.Lease.Revalidate() == nil {
					t.Fatal("errored lease admitted semantic use")
				}
				if release := a.Lease.Release(); !errors.Is(release, failure) {
					t.Fatal("release lost injected failure", release)
				}
			}
			if !errors.Is(err, failure) {
				t.Fatal("boundary not reached or failure lost", err)
			}
			if err := a.Lease.Release(); err != nil {
				t.Fatal("repeat release", err)
			}
			if state, openErr := safefs.OpenRoot(roots.State(), uint32(os.Geteuid())); openErr == nil {
				defer state.Close()
				for _, lockName := range []string{"init.lock", "admission.lock", "writer.lock"} {
					names, listErr := state.Names(11)
					if listErr != nil {
						t.Fatal(listErr)
					}
					if !slices.Contains(names, lockName) {
						continue
					}
					lock, lockErr := state.OpenLock(lockName, false)
					if lockErr != nil {
						t.Fatal(lockErr)
					}
					if lockErr = lock.Try(safefs.LockExclusive); lockErr != nil {
						t.Fatal("error release stranded exclusion", lockErr)
					}
					if lockErr = lock.Close(); lockErr != nil {
						t.Fatal(lockErr)
					}
				}
			}
		})
	}
}

func TestFreshInitializeCancellationAndContender(t *testing.T) {
	for _, boundary := range []string{"after:init-lock-exclusive", "after:target-admission", "after:initialization-publish", "after:writer-acquire-publish", "after:init-handoff"} {
		t.Run(boundary, func(t *testing.T) {
			roots := freshTestRoots(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, err := freshInitializeWithBoundary(ctx, roots, freshTestRandom(), IntentBootstrap, func(name string) error {
				if name == boundary {
					cancel()
				}
				return nil
			})
			if !errors.Is(err, context.Canceled) || a.Lease == nil {
				t.Fatal("cancelled boundary lost owned acquisition", err)
			}
			if release := a.Lease.Release(); !errors.Is(release, context.Canceled) {
				t.Fatal("cancelled release lost primary", release)
			}
		})
	}
	t.Run("continuously-held-handoff", func(t *testing.T) {
		roots := freshTestRoots(t)
		a, err := freshInitializeWithBoundary(context.Background(), roots, freshTestRandom(), IntentPlan, func(name string) error {
			if !strings.HasPrefix(name, "after:initialization-") && name != "after:init-handoff" {
				return nil
			}
			d, err := safefs.OpenRoot(roots.State(), uint32(os.Geteuid()))
			if err != nil {
				return err
			}
			defer d.Close()
			lock, err := d.OpenLock("admission.lock", false)
			if err != nil {
				return err
			}
			defer lock.Close()
			if err = lock.Try(safefs.LockExclusive); !errors.Is(err, unix.EAGAIN) {
				return errors.New("admission exclusion was dropped")
			}
			return nil
		})
		if err != nil {
			_ = a.Lease.Release()
			t.Fatal(err)
		}
		if err = a.Lease.Release(); err != nil {
			t.Fatal(err)
		}
	})
}
