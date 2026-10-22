package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const roadmap = "docs/roadmap"

var owners = map[string]string{
	"PC":   roadmap + "/product-contract.md",
	"ARCH": roadmap + "/architecture.md",
	"VAL":  roadmap + "/validation.md",
}

var idPattern = regexp.MustCompile(`(?:PC|ARCH|VAL)-[0-9]+`)

type location struct {
	file string
	line int
}

type diagnostic struct {
	location
	message string
	warning bool
}

func (d diagnostic) String() string {
	level := "error"
	if d.warning {
		level = "warning"
	}
	return fmt.Sprintf("%s:%d: %s: %s", d.file, d.line, level, d.message)
}

type checker struct {
	root        string
	documents   map[string]*document
	diagnostics []diagnostic
}

func (c *checker) report(at location, message string, historical bool) {
	c.diagnostics = append(c.diagnostics, diagnostic{at, message, historical && at.file == roadmap+"/proof-results.md"})
}

// localPath rejects paths outside the selected repository and symlink components.
// It deliberately does not provide a concurrent hostile-filesystem guarantee.
func (c *checker) localPath(path string) (string, error) {
	path = filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target escapes repository")
	}
	current := c.root
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("target unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink target is not checked")
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return "", fmt.Errorf("target is not an ordinary file or directory")
		}
	}
	return current, nil
}

func (c *checker) load(path string) (*document, error) {
	if doc, ok := c.documents[path]; ok {
		return doc, nil
	}
	absolute, err := c.localPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("cannot read Markdown file")
	}
	doc := parseMarkdown(string(data))
	c.documents[path] = doc
	return doc, nil
}

func check(root string) ([]diagnostic, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	// An explicitly chosen root may itself be a symlink; resolve it once.
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	c := checker{root: absolute, documents: make(map[string]*document)}
	directory, err := c.localPath(roadmap)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", roadmap, err)
	}
	var paths []string
	err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			relative, relErr := filepath.Rel(absolute, path)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", roadmap, err)
	}
	sort.Strings(paths)
	definitions := make(map[string][]location)
	for _, path := range paths {
		doc, loadErr := c.load(path)
		if loadErr != nil {
			c.report(location{path, 1}, loadErr.Error(), false)
			continue
		}
		for _, def := range doc.definitions {
			prefix, _, _ := strings.Cut(def.value, "-")
			if owners[prefix] == path {
				definitions[def.value] = append(definitions[def.value], location{path, def.line})
			}
		}
	}
	for _, owner := range owners {
		if _, ok := c.documents[owner]; !ok {
			c.report(location{owner, 1}, "owning document missing or unreadable", false)
		}
	}
	for id, locations := range definitions {
		if len(locations) > 1 {
			for _, at := range locations {
				c.report(at, fmt.Sprintf("%s has %d owning definitions", id, len(locations)), false)
			}
		}
	}
	for _, path := range paths {
		doc := c.documents[path]
		if doc == nil {
			continue
		}
		for _, issue := range doc.issues {
			c.report(location{path, issue.line}, issue.value, false)
		}
		for _, reference := range doc.ids {
			if len(definitions[reference.value]) == 0 {
				c.report(location{path, reference.line}, reference.value+" has no owning definition", true)
			}
			// Duplicate owners are diagnosed once per definition, not every use.
		}
		for _, link := range doc.links {
			c.checkLink(path, link)
		}
	}
	sort.Slice(c.diagnostics, func(i, j int) bool {
		a, b := c.diagnostics[i], c.diagnostics[j]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.line != b.line {
			return a.line < b.line
		}
		if a.message != b.message {
			return a.message < b.message
		}
		return !a.warning && b.warning
	})
	return c.diagnostics, nil
}

func (c *checker) checkLink(source string, link occurrence) {
	at := location{source, link.line}
	target := unescapeMarkdown(link.value)
	u, err := url.Parse(target)
	if err != nil {
		c.report(at, fmt.Sprintf("invalid link %q", link.value), true)
		return
	}
	if u.IsAbs() || u.Host != "" || strings.HasPrefix(target, "//") {
		return // No network access, including mailto/data and protocol-relative URLs.
	}
	path := source
	if u.Path != "" {
		if strings.HasPrefix(u.Path, "/") {
			path = strings.TrimPrefix(u.Path, "/")
		} else {
			path = filepath.ToSlash(filepath.Join(filepath.Dir(source), filepath.FromSlash(u.Path)))
		}
	}
	absolute, pathErr := c.localPath(path)
	if pathErr != nil {
		c.report(at, fmt.Sprintf("local link %q: %v", link.value, pathErr), true)
		return
	}
	if u.Fragment == "" {
		return
	}
	if !strings.EqualFold(filepath.Ext(absolute), ".md") {
		c.report(at, fmt.Sprintf("fragment in non-Markdown target %q is unsupported", link.value), true)
		return
	}
	doc, loadErr := c.load(path)
	if loadErr != nil {
		c.report(at, fmt.Sprintf("local link %q: %v", link.value, loadErr), true)
		return
	}
	if !doc.anchors[u.Fragment] {
		c.report(at, fmt.Sprintf("local link %q: anchor %q not found", link.value, u.Fragment), true)
	}
}
