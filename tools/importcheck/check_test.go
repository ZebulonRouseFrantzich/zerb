package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.24.0\n")
	return root
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func source(owner, imported, alias string, external bool) string {
	if external {
		owner += "_test"
	}
	return fmt.Sprintf("package %s\nimport %s %q\n", owner, alias, imported)
}

func TestAllOldTreesFailInProductionAndTests(t *testing.T) {
	for _, old := range []string{"config", "service", "git", "drift", "chezmoi", "transaction", "shell", "platform", "binary", "testutil"} {
		for _, suffix := range []string{"", "/nested/deep"} {
			for _, alias := range []string{"", "renamed", "_", "."} {
				for _, file := range []string{"source.go", "source_test.go"} {
					t.Run(old+suffix+"/"+alias+"/"+file, func(t *testing.T) {
						root := repository(t)
						write(t, root, "internal/source/"+file, source("source", modulePath+"/internal/"+old+suffix, alias, false))
						got, err := check(root)
						if err != nil || len(got.diagnostics) != 1 || !strings.Contains(got.diagnostics[0].message, "forbidden full old tree") {
							t.Fatalf("got %+v, %v", got, err)
						}
					})
				}
			}
		}
	}
}

func TestExactDAG(t *testing.T) {
	// Independent contract fixture: '*' denotes host's assembly-only access to
	// every production replacement package except itself; purpose needs review.
	want := map[string]string{
		"paths": "", "profile": "", "receipt": "", "safefs": "paths", "execenv": "safefs",
		"source":    "paths safefs execenv toolchain profile receipt",
		"toolchain": "paths safefs execenv receipt",
		"mise":      "paths safefs execenv toolchain source receipt",
		"home":      "paths safefs execenv toolchain source receipt",
		"state":     "paths safefs receipt",
		"recovery":  "paths source state receipt toolchain mise home",
		"plan":      "paths source profile toolchain mise home state receipt",
		"app":       "paths execenv source profile toolchain mise home plan state receipt recovery",
		"host":      "*", "testenv": "paths safefs",
	}
	if len(dag) != len(want) {
		t.Fatalf("replacement set has %d entries, want %d", len(dag), len(want))
	}
	for owner, allowed := range want {
		if _, exists := dag[owner]; !exists {
			t.Fatalf("missing replacement %s", owner)
		}
		for target := range want {
			for _, test := range []bool{false, true} {
				expected := strings.Contains(" "+allowed+" ", " "+target+" ") || (allowed == "*" && target != owner && target != "testenv") || (test && target == "testenv")
				got := forbidden(owner, modulePath+"/internal/"+target, test, false)
				if (got == "") != expected {
					t.Errorf("%s -> %s (test=%v): %q, allowed=%v", owner, target, test, got, expected)
				}
			}
		}
	}
}

func TestInternalEdgesAndBoundaries(t *testing.T) {
	tests := []struct {
		name, imported, file, pkg string
		allowed                   bool
	}{
		{"allowed", modulePath + "/internal/paths", "input.go", "safefs", true},
		{"testenv production", modulePath + "/internal/testenv", "input.go", "safefs", false},
		{"testenv test", modulePath + "/internal/testenv", "input_test.go", "safefs", true},
		{"testenv external", modulePath + "/internal/testenv", "input_test.go", "safefs_test", true},
		{"external own package", modulePath + "/internal/safefs", "input_test.go", "safefs_test", true},
		{"internal test self", modulePath + "/internal/safefs", "input_test.go", "safefs", false},
		{"production external name", modulePath + "/internal/safefs", "input.go", "safefs_test", false},
		{"external disallowed edge", modulePath + "/internal/state", "input_test.go", "safefs_test", false},
		{"proof production", modulePath + "/internal/proof/process", "input.go", "safefs", false},
		{"proof tests", modulePath + "/internal/proof/process", "input_test.go", "safefs", false},
		{"unknown internal", modulePath + "/internal/bridge", "input.go", "safefs", false},
		{"unknown root package", modulePath + "/bridge", "input.go", "safefs", false},
		{"unknown replacement subpackage", modulePath + "/internal/paths/sub", "input.go", "safefs", false},
		{"testenv subtree", modulePath + "/internal/testenv/sub", "input_test.go", "safefs", false},
		{"old name prefix", modulePath + "/internal/configuration", "input.go", "safefs", false},
		{"lower host", modulePath + "/internal/host", "input.go", "safefs", false},
		{"lower app", modulePath + "/internal/app", "input.go", "safefs", false},
		{"lower command", modulePath + "/cmd/zerb", "input.go", "safefs", false},
		{"same module", modulePath, "input.go", "safefs", false},
		{"stdlib", "context", "input.go", "safefs", true},
		{"third party", "golang.org/x/sys/unix", "input.go", "safefs", true},
		{"module lookalike", modulePath + "-other/internal/config", "input.go", "safefs", true},
		{"module suffix lookalike", "example.com/" + modulePath + "/internal/config", "input.go", "safefs", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := repository(t)
			write(t, root, "internal/safefs/"+test.file, source(test.pkg, test.imported, "_", false))
			got, err := check(root)
			if err != nil || got.files != 1 || (len(got.diagnostics) == 0) != test.allowed {
				t.Fatalf("got %+v, %v; allowed=%v", got, err, test.allowed)
			}
		})
	}
}

func TestScanEveryReplacementAndIgnoredSource(t *testing.T) {
	root := repository(t)
	for _, owner := range []string{"paths", "profile", "receipt", "safefs", "execenv", "source", "toolchain", "mise", "home", "state", "recovery", "plan", "app", "host", "testenv"} {
		write(t, root, "internal/"+owner+"/excluded_windows_test.go", "//go:build windows && never\n\n"+source(owner, modulePath+"/internal/config/sub", "", false))
	}
	write(t, root, "internal/paths/_ignored.go", source("paths", modulePath+"/internal/service", "", false))
	write(t, root, "internal/paths/.hidden.go", source("paths", modulePath+"/internal/drift", "", false))
	write(t, root, "internal/paths/sub/nested.go", "package nested\n")
	got, err := check(root)
	if err != nil || got.files != 18 || len(got.diagnostics) != 18 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFoundationScopeAndMissingTargets(t *testing.T) {
	for _, targets := range []bool{false, true} {
		t.Run(fmt.Sprint(targets), func(t *testing.T) {
			root := repository(t)
			// Old entrypoint/tree, proof harness and tools remain separate owners.
			for _, path := range []string{"cmd/zerb/main.go", "internal/config/config.go", "internal/proof/proof.go", "tools/other/main.go"} {
				write(t, root, path, "intentionally invalid Go\n")
			}
			if targets {
				write(t, root, "internal/paths/input.go", "package paths\n")
			}
			got, err := check(root)
			wantFiles := 0
			if targets {
				wantFiles = 1
			}
			if err != nil || len(got.diagnostics) != 0 || got.files != wantFiles {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestMalformedGoFailsClosed(t *testing.T) {
	for _, input := range []string{
		"package paths\nimport \"unterminated\n",
		"package paths\nimport `bad\\path`\nfunc broken(\n",
		"package paths\nfunc broken() {\n",
		"not a package\n",
	} {
		t.Run(input, func(t *testing.T) {
			root := repository(t)
			write(t, root, "internal/paths/invalid.go", input)
			got, err := check(root)
			if err != nil || len(got.diagnostics) == 0 || !strings.Contains(got.diagnostics[0].message, "invalid Go source") {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestRootAndModuleIdentity(t *testing.T) {
	tests := []struct{ name, module string }{
		{"renamed", "module example.com/renamed\n"},
		{"lookalike", "module " + modulePath + "-other\n"},
		{"duplicate", "module " + modulePath + "\nmodule " + modulePath + "\n"},
		{"comment only", "/*\nmodule " + modulePath + "\n*/\n"},
		{"comment spoof", "/*\nmodule " + modulePath + "\n*/\nmodule example.com/other\n"},
		{"empty", ""},
		{"unclosed comment", "module " + modulePath + "\n/*"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := repository(t)
			write(t, root, "go.mod", test.module)
			if _, err := check(root); err == nil {
				t.Fatal("accepted unexpected module identity")
			}
		})
	}
	for _, module := range []string{"// comment\nmodule " + modulePath + " // trailing\n", "module \"" + modulePath + "\"\n", "/* comment */\nmodule " + modulePath + "\n"} {
		root := repository(t)
		write(t, root, "go.mod", module)
		if _, err := check(root); err != nil {
			t.Fatal(err)
		}
	}
	root := repository(t)
	for _, path := range []string{filepath.Join(root, "absent"), filepath.Join(root, "go.mod")} {
		if _, err := check(path); err == nil {
			t.Fatalf("accepted invalid root %s", path)
		}
	}
	if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := check(root); err == nil {
		t.Fatal("accepted missing go.mod")
	}
}

func TestSymlinksFailClosed(t *testing.T) {
	for _, link := range []string{"go.mod", "internal", "internal/paths", "internal/paths/source.go", "internal/paths/sub", "internal/paths/ignored.txt"} {
		t.Run(link, func(t *testing.T) {
			root := repository(t)
			outside := t.TempDir()
			write(t, outside, "source.go", "package paths\n")
			target := outside
			if strings.HasSuffix(link, ".go") || strings.HasSuffix(link, ".txt") || link == "go.mod" {
				target = filepath.Join(outside, "source.go")
			}
			path := filepath.Join(root, filepath.FromSlash(link))
			if link == "go.mod" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			got, err := check(root)
			if err == nil && len(got.diagnostics) == 0 {
				t.Fatal("accepted symlink")
			}
			if got.files != 0 {
				t.Fatal("followed source symlink")
			}
		})
	}
	root := repository(t)
	link := filepath.Join(t.TempDir(), "selected-root")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if got, err := check(link); err != nil || len(got.diagnostics) != 0 {
		t.Fatalf("explicit root link rejected: %+v, %v", got, err)
	}
}

func TestCLIFixture(t *testing.T) {
	root := repository(t)
	fixture, err := os.ReadFile("testdata/violations.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "internal/source/violations_test.go", string(fixture))
	want, err := os.ReadFile("testdata/violations.golden")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-root", root}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit %d; stderr=%q", code, stderr.String())
	}
	if stdout.String() != string(want) || stderr.Len() != 0 {
		t.Fatalf("stdout:\n%s\nstderr: %s\nwant:\n%s", stdout.String(), stderr.String(), want)
	}
	// Stable order and repository-relative output across repeated checks.
	for range 5 {
		var next bytes.Buffer
		if code := run([]string{"-root", root}, &next, &stderr); code != 1 || next.String() != stdout.String() {
			t.Fatalf("nondeterministic result: exit %d, %q", code, next.String())
		}
	}
}

func TestCLIOutcomes(t *testing.T) {
	root := repository(t)
	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"-root", root}, 0},
		{[]string{"-h"}, 0},
		{[]string{"-unknown"}, 2},
		{[]string{"operand"}, 2},
		{[]string{"-root", filepath.Join(root, "absent")}, 2},
	} {
		var stdout, stderr bytes.Buffer
		if got := run(test.args, &stdout, &stderr); got != test.code {
			t.Errorf("%v: exit %d, want %d", test.args, got, test.code)
		}
		if len(test.args) == 2 && test.args[1] == root && !strings.Contains(stdout.String(), "0 target Go file(s)") {
			t.Errorf("missing zero-file scope report: %q", stdout.String())
		}
	}
}
