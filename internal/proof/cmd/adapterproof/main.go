// adapterproof is a disposable Phase 1 proof executable. Hidden child modes
// are part of the same-binary FD-only supervisor and are not product commands.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/adapters"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/proofenv"
)

const btrfsSuperMagic = 0x9123683e

type output struct {
	Version   int               `json:"version"`
	Lifecycle *lifecycle.Result `json:"lifecycle,omitempty"`
	Adapters  *adapters.Result  `json:"adapters,omitempty"`
}

func main() { os.Exit(run()) }
func run() int {
	if handled, status := lifecycle.TrySupervisorMode(); handled {
		return status
	}
	if len(os.Args) > 1 && os.Args[1] == "__proof-child" {
		return lifecycle.ChildMode(os.Args[2:])
	}
	fs := flag.NewFlagSet("adapterproof", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	base := fs.String("base", "", "existing canonical disposable btrfs base outside repository and real HOME/XDG")
	artifacts := fs.String("artifacts", "", "existing artifact directory beneath base")
	git := fs.String("git", "", "canonical pinned Git executable")
	trace := fs.String("trace", "", "canonical pinned closed syscall tracer")
	bash := fs.String("bash", "", "canonical pinned Bash executable")
	signalFixture := fs.String("signal-fixture", "", "canonical pinned default-signal fixture executable")
	mise := fs.String("mise", "", "canonical pinned mise executable")
	chezmoi := fs.String("chezmoi", "", "canonical pinned chezmoi executable")
	ca := fs.String("cacert", "", "canonical pinned public CA bundle")
	lock := fs.String("node-lock", "", "supplied complete authored node@24.4.1 lock fixture")
	slice := fs.String("slice", "all", "all, lifecycle, or adapters")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return fail("unexpected operands")
	}
	if *slice != "all" && *slice != "lifecycle" && *slice != "adapters" {
		return fail("invalid slice")
	}
	protected, err := proofenv.CaptureProtected("")
	if err != nil {
		return fail(err.Error())
	}
	canonicalBase, err := safeBase(*base, protected.Paths)
	if err != nil {
		return fail(err.Error())
	}
	artifactDir, err := canonicalUnder(*artifacts, canonicalBase)
	if err != nil {
		return fail("invalid artifact directory")
	}
	_ = artifactDir
	paths := []*string{git, trace, bash, signalFixture, mise, chezmoi, ca}
	for _, p := range paths {
		c, e := canonicalFile(*p)
		if e != nil {
			return fail(e.Error())
		}
		*p = c
	}
	var lockBytes []byte
	if *slice == "all" || *slice == "adapters" {
		lockPath, e := canonicalFileUnder(*lock, canonicalBase)
		if e != nil {
			return fail("invalid authored lock fixture")
		}
		lockBytes, e = os.ReadFile(lockPath)
		if e != nil {
			return fail("read supplied authored lock fixture")
		}
	}
	root, err := os.MkdirTemp(canonicalBase, "adapterproof-")
	if err != nil {
		return fail("create disposable owned root")
	}
	if err = os.Chmod(root, 0700); err != nil {
		_ = os.RemoveAll(root)
		return fail("secure disposable root")
	}
	defer os.RemoveAll(root)
	self, err := os.Executable()
	if err != nil {
		return fail("resolve same-binary executable")
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fail("canonicalize same-binary executable")
	}
	ctx := context.Background()
	out := output{Version: 2}
	if *slice == "all" || *slice == "lifecycle" {
		r, e := lifecycle.Run(ctx, lifecycle.Config{Root: root, Executable: self, Git: *git, Bash: *bash, SignalFixture: *signalFixture, Mise: *mise, Chezmoi: *chezmoi, CACert: *ca, ArtifactDir: artifactDir, ProtectedRoots: protected.Paths})
		if e != nil {
			return fail(e.Error())
		}
		out.Lifecycle = &r
	}
	if *slice == "all" || *slice == "adapters" {
		r, e := adapters.Run(ctx, adapters.Config{Root: root, Executable: self, Git: *git, Trace: *trace, Bash: *bash, CoreutilsDir: filepath.Dir(*signalFixture), Mise: *mise, Chezmoi: *chezmoi, CACert: *ca, ArtifactDir: artifactDir, NodeLock: string(lockBytes), ProtectedRoots: protected.Paths})
		if e != nil {
			return fail(e.Error())
		}
		out.Adapters = &r
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(true)
	if err = enc.Encode(out); err != nil {
		return fail("encode redacted result")
	}
	return 0
}
func fail(s string) int { fmt.Fprintln(os.Stderr, "Error:", s); return 1 }
func canonicalFile(raw string) (string, error) {
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errors.New("all executable and CA paths must be explicit absolute paths")
	}
	c, e := filepath.EvalSymlinks(raw)
	if e != nil {
		return "", errors.New("canonical pinned path unavailable")
	}
	st, e := os.Stat(c)
	if e != nil || !st.Mode().IsRegular() {
		return "", errors.New("canonical pinned path is not a regular file")
	}
	return c, nil
}
func safeBase(raw string, protected []string) (string, error) {
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errors.New("explicit absolute disposable base required")
	}
	if len(protected) == 0 {
		return "", errors.New("canonical protected roots captured before environment poisoning are required")
	}
	base, e := filepath.EvalSymlinks(raw)
	if e != nil {
		return "", errors.New("canonical disposable base required")
	}
	st, e := os.Stat(base)
	if e != nil || !st.IsDir() {
		return "", errors.New("disposable base must be an existing directory")
	}
	var fs syscall.Statfs_t
	if syscall.Statfs(base, &fs) != nil || uint64(fs.Type) != btrfsSuperMagic {
		return "", errors.New("disposable base must be btrfs")
	}
	for _, p := range protected {
		p, e = filepath.EvalSymlinks(p)
		if e != nil {
			return "", errors.New("canonical protected path unavailable")
		}
		if within(base, p) || within(p, base) {
			return "", errors.New("disposable base overlaps a protected path class")
		}
	}
	return base, nil
}
func canonicalUnder(raw, base string) (string, error) {
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errors.New("explicit artifact directory required")
	}
	p, e := filepath.EvalSymlinks(raw)
	if e != nil {
		return "", e
	}
	if !within(p, base) {
		return "", errors.New("artifact directory outside disposable base")
	}
	st, e := os.Stat(p)
	if e != nil || !st.IsDir() {
		return "", errors.New("artifact directory unavailable")
	}
	return p, nil
}
func canonicalFileUnder(raw, base string) (string, error) {
	p, e := canonicalFile(raw)
	if e != nil {
		return "", e
	}
	if !within(p, base) {
		return "", errors.New("fixture outside disposable base")
	}
	return p, nil
}
func within(p, b string) bool {
	r, e := filepath.Rel(b, p)
	return e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)))
}
