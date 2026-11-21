package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir("testdata/valid", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel("testdata/valid", path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFixture(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func rendered(diagnostics []diagnostic) string {
	var lines []string
	for _, d := range diagnostics {
		lines = append(lines, d.String())
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestValidFixture(t *testing.T) {
	root := fixture(t)
	diagnostics, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatal(rendered(diagnostics))
	}
}

func TestInvalidFixtureDeterministic(t *testing.T) {
	root := fixture(t)
	data, err := os.ReadFile("testdata/invalid.md")
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, roadmap+"/broken.md", string(data))
	want, err := os.ReadFile("testdata/invalid.golden")
	if err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 5; repeat++ {
		diagnostics, err := check(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := rendered(diagnostics); got != string(want) {
			t.Fatalf("got:\n%swant:\n%s", got, want)
		}
	}
}

func TestOwningDefinitions(t *testing.T) {
	tests := []struct{ name, file, content, want string }{
		{"duplicate PC", owners["PC"], "### PC-001 — One\n### PC-001 — Two\n", "PC-001 has 2 owning definitions"},
		{"duplicate ARCH", owners["ARCH"], "### ARCH-001 — One\n### ARCH-001 — Two\n", "ARCH-001 has 2 owning definitions"},
		{"duplicate VAL", owners["VAL"], "| VAL-001 | One |\n| `VAL-001` | Two |\n", "VAL-001 has 2 owning definitions"},
		{"wrong owner", roadmap + "/notes.md", "### PC-888 — Not authority\n", "PC-888 has no owning definition"},
		{"PC table is not authority", owners["PC"], "| PC-001 | Not a heading |\n", "PC-001 has no owning definition"},
		{"VAL heading is not authority", owners["VAL"], "### VAL-001 — Not a row\n", "VAL-001 has no owning definition"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			writeFixture(t, root, test.file, test.content)
			diagnostics, err := check(root)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rendered(diagnostics), test.want) {
				t.Fatal(rendered(diagnostics))
			}
		})
	}
}

func TestHistoricalWarnings(t *testing.T) {
	root := fixture(t)
	writeFixture(t, root, roadmap+"/proof-results.md", "PC-999\n[Old anchor](product-contract.md#old-title)\n")
	diagnostics, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatal(rendered(diagnostics))
	}
	for _, d := range diagnostics {
		if !d.warning {
			t.Fatal(d)
		}
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"-root", root}, &stdout, &stderr); status != 0 {
		t.Fatalf("status %d: %s", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0 error(s), 2 warning(s)") {
		t.Fatal(stdout.String())
	}
}

func TestMissingOwnersAndReadFailures(t *testing.T) {
	root := fixture(t)
	if err := os.Remove(filepath.Join(root, owners["VAL"])); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, roadmap, "directory.md"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, roadmap+"/notes.md", "[Directory fragment](directory.md#nope)\n")
	diagnostics, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"owning document missing or unreadable", "VAL-001 has no owning definition", "cannot read Markdown file"} {
		if !strings.Contains(rendered(diagnostics), want) {
			t.Fatal(rendered(diagnostics))
		}
	}
	if _, err := check(t.TempDir()); err == nil {
		t.Fatal("missing roadmap accepted")
	}
	if _, err := check(filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing root accepted")
	}
}

func TestSymlinksAndSpecialFiles(t *testing.T) {
	root := fixture(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink("product-contract.md", filepath.Join(root, roadmap, "alias.md")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, roadmap+"/notes.md", "[Symlink](../../linked/secret.md)\n[Alias](alias.md)\n")
	diagnostics, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(rendered(diagnostics), "symlink target is not checked"); count != 3 {
		t.Fatal(rendered(diagnostics))
	}
}

func TestCLI(t *testing.T) {
	root := fixture(t)
	writeFixture(t, root, roadmap+"/broken.md", "PC-999\n")
	for _, test := range []struct {
		name   string
		args   []string
		status int
	}{
		{"help", []string{"-h"}, 0},
		{"unknown flag", []string{"-bad"}, 2},
		{"operand", []string{"unexpected"}, 2},
		{"missing root", []string{"-root", filepath.Join(root, "absent")}, 2},
		{"document error", []string{"-root", root}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if status := run(test.args, &stdout, &stderr); status != test.status {
				t.Fatalf("status %d, want %d; stdout=%s stderr=%s", status, test.status, stdout.String(), stderr.String())
			}
		})
	}
}

func TestCodeAndCommentBoundaries(t *testing.T) {
	for _, test := range []struct{ name, example string }{
		{"backtick fence", "```html\n<!--\n[example](missing.md)\n```"},
		{"tilde fence", "~~~~\n<!--\n~~~\n[example](missing.md)\n~~~~"},
		{"quoted fence", "> ```\n> [example](missing.md)\n> ```"},
		{"list fence", "- ```\n  [example](missing.md)\n  ```"},
		{"ordered list fence", "1. ```\n   [example](missing.md)\n   ```"},
		{"indented code", "    <!--\n    [example](missing.md)"},
		{"tab code", "\t<!-- [example](missing.md)"},
		{"inline code", "`<!-- [example](missing.md)`"},
		{"long inline delimiter", "`` ` [example](missing.md) ``"},
		{"comment", "<!--\n```\n[example](missing.md)\n-->"},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := parseMarkdown(test.example + "\n\n### PC-001 — First\n### PC-001 — Duplicate\n[real](missing.md)\n")
			if len(doc.definitions) != 2 || len(doc.links) != 1 || doc.links[0].value != "missing.md" {
				t.Fatalf("definitions=%v links=%v", doc.definitions, doc.links)
			}
		})
	}
	doc := parseMarkdown("`<a id=\"fake\"></a>`\n[bad](#fake)\n")
	if doc.anchors["fake"] {
		t.Fatal("inline example created anchor")
	}
	if parseMarkdown("\\<a id=\"fake\"></a>\n").anchors["fake"] {
		t.Fatal("escaped HTML created anchor")
	}
	if !parseMarkdown("\\\\<a id=\"real\"></a>\n").anchors["real"] {
		t.Fatal("even backslash run hid real anchor")
	}
}

func TestMarkdownBoundaries(t *testing.T) {
	for _, test := range []struct{ name, input, anchor string }{
		{"closing hashes", "## Heading ###\n", "heading"},
		{"unspaced hash", "## Heading#\n", "heading"},
		{"reference label", "## [A][label]\n[label]: #a\n", "a"},
		{"collapsed label", "## [A][]\n[a]: #a\n", "a"},
		{"HTML anchor", "<a name='named'></a>\n", "named"},
		{"entities", "## A &amp; B\r\n", "a--b"},
		{"quoted heading", "> ## Quoted\n", "quoted"},
		{"quoted reference", "> ## [Quoted][q]\n> [q]: #quoted\n", "quoted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !parseMarkdown(test.input).anchors[test.anchor] {
				t.Fatalf("missing %q", test.anchor)
			}
		})
	}
	for _, input := range []string{"[x](a(b)\n", "[x](<unclosed)\n", "[x](path 'unfinished)\n", "[x]:\n", "[x]: path\n[X]: other\n"} {
		if len(parseMarkdown(input).issues) == 0 {
			t.Fatalf("missing syntax diagnostic: %q", input)
		}
	}
	// Ordinary unmatched brackets, escaped punctuation and unmatched ticks are prose.
	if doc := parseMarkdown("[unfinished\n`literal [x](ok.md)\n\\[escaped](missing)\n"); len(doc.links) != 1 {
		t.Fatal(doc.links)
	}
}
