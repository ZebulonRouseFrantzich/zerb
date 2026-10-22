package paths

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInputSnapshot(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home", "..", "captured")
	environ := []string{"HOME=opaque", "name with space=a=b\n\xff", "EMPTY=", "DUP=first", "DUP=second"}
	want := slices.Clone(environ)
	input, err := NewInput(home, environ)
	if err != nil {
		t.Fatal(err)
	}
	environ[0] = "HOME=changed"
	copyEnv := input.Environ()
	copyEnv[1] = "changed"
	if input.Home() != home || !slices.Equal(input.Environ(), want) {
		t.Fatal("input changed through caller-owned storage")
	}
	for _, test := range []struct {
		name, value string
		present     bool
	}{{"HOME", "opaque", true}, {"name with space", "a=b\n\xff", true}, {"EMPTY", "", true}, {"DUP", "first", true}, {"absent", "", false}, {"", "", false}, {"HOME=opaque", "", false}} {
		if value, present := input.LookupEnv(test.name); value != test.value || present != test.present {
			t.Fatalf("lookup mismatch for test key %q", test.name)
		}
	}
}

func TestInputRejectsMalformedBytes(t *testing.T) {
	for _, test := range []struct {
		home string
		env  []string
	}{{"PrivateHome\x00Marker", nil}, {"", []string{"PrivateEntryMarker"}}, {"", []string{"=PrivateValueMarker"}}, {"", []string{"KEY=Private\x00ValueMarker"}}, {"", []string{"Private\x00Key=value"}}} {
		input, err := NewInput(test.home, test.env)
		if err == nil || err.Error() != "invalid path input" || input.Home() != "" || input.Environ() != nil {
			t.Fatal("malformed input must return fixed error and zero input")
		}
		if _, err := Resolve(input); err == nil {
			t.Fatal("failed construction produced usable roots")
		}
	}
}

func TestRootDefaultsAndOverrides(t *testing.T) {
	base := filepath.ToSlash(t.TempDir())
	home := base + "/home//child/.."
	cleanHome := path.Clean(home)
	for _, test := range []struct {
		name string
		env  []string
		want [3]string
	}{
		{"unset", nil, [3]string{cleanHome + "/.local/share/zerb", cleanHome + "/.cache/zerb", cleanHome + "/.local/state/zerb"}},
		{"empty", []string{"XDG_DATA_HOME=", "XDG_CACHE_HOME=", "XDG_STATE_HOME="}, [3]string{cleanHome + "/.local/share/zerb", cleanHome + "/.cache/zerb", cleanHome + "/.local/state/zerb"}},
		{"relative", []string{"XDG_DATA_HOME=relative", "XDG_CACHE_HOME=../cache", "XDG_STATE_HOME=~/state"}, [3]string{cleanHome + "/.local/share/zerb", cleanHome + "/.cache/zerb", cleanHome + "/.local/state/zerb"}},
		{"absolute", []string{"XDG_DATA_HOME=" + base + "/data//child/..", "XDG_CACHE_HOME=" + base + "/cache/", "XDG_STATE_HOME=" + base + "/state/./"}, [3]string{base + "/data/zerb", base + "/cache/zerb", base + "/state/zerb"}},
		{"opaque", []string{"HOME=" + base + "/other-home", "XDG_CONFIG_HOME=relative", "XDG_CONFIG_HOME=/ignored", "ZERB_DATA_HOME=/ignored", "XDG_RUNTIME_DIR=/ignored"}, [3]string{cleanHome + "/.local/share/zerb", cleanHome + "/.cache/zerb", cleanHome + "/.local/state/zerb"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := NewInput(home, test.env)
			if err != nil {
				t.Fatal(err)
			}
			roots, err := Resolve(input)
			if err != nil {
				t.Fatal(err)
			}
			if roots.Home() != cleanHome || [3]string{roots.Data(), roots.Cache(), roots.State()} != test.want {
				t.Fatal("unexpected resolved roots")
			}
			if input.Home() != home {
				t.Fatal("resolution changed captured input")
			}
		})
	}
}

func TestResolutionRejectsAmbiguity(t *testing.T) {
	base := filepath.ToSlash(t.TempDir())
	names := []string{"XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"}
	for _, name := range names {
		for _, values := range [][2]string{{base, base}, {base, base + "/other"}, {"", ""}, {"relative", "relative"}} {
			input, err := NewInput(base, []string{name + "=" + values[0], name + "=" + values[1]})
			if err != nil {
				t.Fatal("capture performed root semantics")
			}
			refuseRoots(t, input)
		}
	}
	for i := range names {
		for j := range names {
			if i == j {
				continue
			}
			for _, second := range []string{base + "/shared", base + "/shared/./", base + "/shared/zerb/nested"} {
				input, err := NewInput(base, []string{names[i] + "=" + base + "/shared", names[j] + "=" + second})
				if err != nil {
					t.Fatal(err)
				}
				refuseRoots(t, input)
			}
		}
	}
}

func TestResolutionAllowsDisjointProductLeaves(t *testing.T) {
	base := filepath.ToSlash(t.TempDir())
	input, err := NewInput(base, []string{"XDG_DATA_HOME=" + base + "/base", "XDG_CACHE_HOME=" + base + "/base/cache", "XDG_STATE_HOME=" + base + "/base/zerb-extra"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(input); err != nil {
		t.Fatal("non-overlapping leaves rejected due to base containment or textual prefix")
	}
}

func TestResolutionDefersHomeSemantics(t *testing.T) {
	refuseRoots(t, Input{})
	for _, home := range []string{"", ".", "relative", "~/PrivateHomeMarker"} {
		input, err := NewInput(home, nil)
		if err != nil {
			t.Fatal("capture performed root semantics")
		}
		refuseRoots(t, input)
	}
}

func TestResolutionHasNoAmbientOrFilesystemEffects(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(name, filepath.Join(base, "ambient", name))
	}
	home := filepath.Join(base, "absent", "home")
	input, err := NewInput(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := Resolve(input)
	if err != nil || roots.Home() != home || roots.Data() != path.Join(home, ".local/share/zerb") {
		t.Fatal("ambient state influenced lexical roots")
	}
	if _, err := os.Lstat(filepath.Join(base, "absent")); !os.IsNotExist(err) {
		t.Fatal("resolution created a path")
	}
	file := filepath.Join(base, "ordinary-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	input, err = NewInput(file, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(input); err != nil {
		t.Fatal("lexical resolution inspected filesystem type")
	}
}

func refuseRoots(t *testing.T, input Input) {
	t.Helper()
	roots, err := Resolve(input)
	if err == nil || err.Error() != "invalid protected roots" || roots != (Roots{}) || strings.Contains(err.Error(), input.Home()) && input.Home() != "" {
		t.Fatal("invalid root relation must return fixed error and zero roots")
	}
}
