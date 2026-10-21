//go:build linux

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/proofenv"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/state"
)

func main() {
	if handled, status := lifecycle.TrySupervisorMode(); handled {
		os.Exit(status)
	}
	root := flag.String("root", "", "canonical disposable btrfs base outside repository and real HOME/XDG")
	artifacts := flag.String("artifacts", "", "explicit evidence artifact directory")
	repo := flag.String("repository", "", "canonical repository root to exclude")
	ca := flag.String("cacert", "", "canonical CA certificate path, if a sibling slice needs it")
	git := flag.String("git", "", "canonical absolute pinned Git executable")
	worker := flag.String("worker", "", "internal proof worker mode")
	workerPath := flag.String("worker-path", "", "internal worker fixture path")
	flag.Parse()
	if *worker != "" {
		if err := state.Worker(*worker, *workerPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fatal(err)
		}
		*repo = cwd
	}
	if p, err := filepath.Abs(*repo); err == nil {
		*repo = p
	}
	if *git == "" || !filepath.IsAbs(*git) || filepath.Clean(*git) != *git {
		fatal(fmt.Errorf("--git must name the canonical absolute pinned Git executable"))
	}
	canonicalGit, err := filepath.EvalSymlinks(*git)
	if err != nil || canonicalGit != *git {
		fatal(fmt.Errorf("--git must name the canonical absolute pinned Git executable"))
	}
	protected, err := proofenv.CaptureProtected(*repo)
	if err != nil {
		fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		fatal(err)
	}
	result, err := state.Run(context.Background(), state.Config{Root: *root, ArtifactDir: *artifacts, CACert: *ca, RepositoryRoot: *repo, GitPath: canonicalGit, SelfExecutable: exe, ProtectedRoots: protected.Paths})
	if err != nil {
		fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
