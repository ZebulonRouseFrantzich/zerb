package main

import (
	"errors"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	textscanner "text/scanner"
)

const modulePath = "github.com/ZebulonRouseFrantzich/zerb"

// This is ARCH-030's exact replacement-package table. Host's assembly purpose
// and all other non-import semantics still require review.
var dag = map[string][]string{
	"paths":     {},
	"profile":   {},
	"receipt":   {},
	"safefs":    {"paths"},
	"execenv":   {"safefs"},
	"source":    {"paths", "safefs", "execenv", "toolchain", "profile", "receipt"},
	"toolchain": {"paths", "safefs", "execenv", "receipt"},
	"mise":      {"paths", "safefs", "execenv", "toolchain", "source", "receipt"},
	"home":      {"paths", "safefs", "execenv", "toolchain", "source", "receipt"},
	"state":     {"paths", "safefs", "receipt"},
	"recovery":  {"paths", "source", "state", "receipt", "toolchain", "mise", "home"},
	"plan":      {"paths", "source", "profile", "toolchain", "mise", "home", "state", "receipt"},
	"app":       {"paths", "execenv", "source", "profile", "toolchain", "mise", "home", "plan", "state", "receipt", "recovery"},
	"host":      {"paths", "profile", "receipt", "safefs", "execenv", "source", "toolchain", "mise", "home", "state", "recovery", "plan", "app"},
	"testenv":   {"paths", "safefs"},
}

var oldTrees = []string{"config", "service", "git", "drift", "chezmoi", "transaction", "shell", "platform", "binary", "testutil"}

type diagnostic struct {
	path    string
	line    int
	column  int
	message string
}

func (d diagnostic) String() string {
	if d.line != 0 {
		return fmt.Sprintf("%s:%d:%d: %s", d.path, d.line, d.column, d.message)
	}
	return d.path + ": " + d.message
}

type report struct {
	files       int
	diagnostics []diagnostic
}

func (r *report) add(path string, line, column int, message string) {
	r.diagnostics = append(r.diagnostics, diagnostic{filepath.ToSlash(path), line, column, message})
}

func check(root string) (report, error) {
	var result report
	abs, err := filepath.Abs(root)
	if err != nil {
		return result, errors.New("invalid repository root")
	}
	// A caller-selected root may itself be a symlink. After canonicalization no
	// repository-relative link is followed. This is a developer checker, not a
	// filesystem sandbox against a concurrent hostile writer.
	root, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return result, errors.New("invalid repository root")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return result, errors.New("repository root is not a directory")
	}
	if err := readModule(root); err != nil {
		return result, err
	}
	internal := filepath.Join(root, "internal")
	info, err = os.Lstat(internal)
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("internal: expected a readable directory without symlinks")
	}
	for name := range dag {
		rel := filepath.Join("internal", name)
		dir := filepath.Join(root, rel)
		_, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue // Later phases introduce packages without compile-only scaffolds.
		}
		if err != nil {
			result.add(rel, 0, 0, "cannot inspect replacement tree")
			continue
		}
		err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return errors.New("cannot determine repository-relative path")
			}
			if walkErr != nil {
				result.add(relPath, 0, 0, "cannot read replacement tree entry")
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				result.add(relPath, 0, 0, "symlink in replacement tree is forbidden")
				return nil
			}
			if path == dir && !entry.IsDir() {
				result.add(relPath, 0, 0, "replacement tree must be a directory")
				return nil
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			if !entry.Type().IsRegular() {
				result.add(relPath, 0, 0, "Go source must be a regular file")
				return nil
			}
			result.files++
			if filepath.Dir(path) != dir {
				result.add(relPath, 0, 0, "undeclared replacement subpackage (ARCH-030)")
			}
			result.scan(path, relPath, name)
			return nil
		})
		if err != nil {
			return result, errors.New("cannot walk replacement tree")
		}
	}
	sort.Slice(result.diagnostics, func(i, j int) bool {
		a, b := result.diagnostics[i], result.diagnostics[j]
		if a.path != b.path {
			return a.path < b.path
		}
		if a.line != b.line {
			return a.line < b.line
		}
		if a.column != b.column {
			return a.column < b.column
		}
		return a.message < b.message
	})
	return result, nil
}

func (r *report) scan(path, rel, owner string) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.add(rel, 0, 0, "cannot read Go source")
		return
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, rel, data, parser.AllErrors)
	if err != nil {
		var list scanner.ErrorList
		if errors.As(err, &list) {
			for _, item := range list {
				r.add(rel, item.Pos.Line, item.Pos.Column, "invalid Go source: "+item.Msg)
			}
		} else {
			r.add(rel, 0, 0, "invalid Go source")
		}
		return
	}
	test := strings.HasSuffix(filepath.Base(path), "_test.go")
	external := test && file.Name.Name == owner+"_test"
	for _, imp := range file.Imports {
		value, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			r.add(rel, set.Position(imp.Pos()).Line, set.Position(imp.Pos()).Column, "invalid import string")
			continue
		}
		if reason := forbidden(owner, value, test, external); reason != "" {
			pos := set.Position(imp.Path.Pos())
			r.add(rel, pos.Line, pos.Column, fmt.Sprintf("import %q: %s", value, reason))
		}
	}
}

func within(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func forbidden(owner, path string, test, external bool) string {
	if within(path, modulePath) {
		local := strings.TrimPrefix(path, modulePath+"/")
		for _, old := range oldTrees {
			if within(local, "internal/"+old) {
				return "forbidden full old tree (ARCH-030; hard-cutover replacement seam)"
			}
		}
		if local == "internal/testenv" {
			if test {
				return ""
			}
			return "internal/testenv is permitted only in _test.go (ARCH-030)"
		}
		if external && local == "internal/"+owner {
			return "" // An external test's subject import is not a production edge.
		}
		for _, allowed := range dag[owner] {
			if local == "internal/"+allowed {
				return ""
			}
		}
		return "edge is absent from the exact ARCH-030 DAG"
	}
	// External imports are outside ARCH-030's replacement DAG. The compiler,
	// read-only module verification, and dependency review own their validity.
	return ""
}

// Only the module identity is needed, not dependency resolution or execution of
// go commands. Tokenization ignores comments, including fake module lines in
// block comments, and preserves line boundaries.
func readModule(root string) error {
	path := filepath.Join(root, "go.mod")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("go.mod: expected a readable regular file without symlinks")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("go.mod: cannot read module identity")
	}
	var lexer textscanner.Scanner
	lexer.Init(strings.NewReader(string(data)))
	lexer.Mode = textscanner.ScanIdents | textscanner.ScanStrings | textscanner.ScanRawStrings | textscanner.ScanComments | textscanner.SkipComments
	lexer.Whitespace &^= 1 << '\n'
	invalid := false
	lexer.Error = func(_ *textscanner.Scanner, _ string) { invalid = true }
	var line []string
	var modules []string
	consume := func() {
		if len(line) > 0 && line[0] == "module" {
			value := strings.Join(line[1:], "")
			if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "`") {
				decoded, err := strconv.Unquote(value)
				if err != nil {
					invalid = true
				}
				value = decoded
			}
			modules = append(modules, value)
		}
		line = nil
	}
	for {
		tok := lexer.Scan()
		if tok == '\n' || tok == textscanner.EOF {
			consume()
			if tok == textscanner.EOF {
				break
			}
		} else {
			line = append(line, lexer.TokenText())
		}
	}
	if invalid || len(modules) != 1 || modules[0] != modulePath {
		return errors.New("go.mod: expected exactly module " + modulePath)
	}
	return nil
}
