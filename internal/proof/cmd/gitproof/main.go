package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	proofgit "github.com/ZebulonRouseFrantzich/zerb/internal/proof/git"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/lifecycle"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/proofenv"
)

type output struct {
	Result proofgit.Result `json:"result"`
	Error  string          `json:"error,omitempty"`
}

func main() {
	if handled, status := lifecycle.TrySupervisorMode(); handled {
		os.Exit(status)
	}
	if len(os.Args) > 1 && os.Args[1] == "__proof-child" {
		os.Exit(lifecycle.ChildMode(os.Args[2:]))
	}
	var cfg proofgit.Config
	flag.StringVar(&cfg.Root, "root", "", "canonical disposable btrfs proof base outside repository and real HOME/XDG")
	flag.StringVar(&cfg.RepositoryRoot, "repo-root", "", "canonical repository root that the disposable proof base must not overlap")
	flag.StringVar(&cfg.GitPath, "git", "", "absolute pinned Git executable")
	flag.StringVar(&cfg.TracePath, "trace", "", "absolute strace-compatible instrumentation executable")
	flag.StringVar(&cfg.CACert, "ca-cert", "", "absolute fixed public CA bundle required for canonical HTTPS evidence")
	flag.StringVar(&cfg.ArtifactDir, "artifact-dir", "", "retained redacted artifact directory beneath root")
	flag.StringVar(&cfg.Source, "source", proofgit.ReviewedPublicSource, "reviewed canonical public HTTPS Git source")
	supervisor, err := os.Executable()
	if err != nil {
		emit(output{Error: "resolve same-binary supervisor"}, 1)
	}
	cfg.SupervisorPath, err = filepath.EvalSymlinks(supervisor)
	if err != nil {
		emit(output{Error: "canonicalize same-binary supervisor"}, 1)
	}
	flag.Parse()
	if flag.NArg() != 0 {
		emit(output{Error: "positional arguments and transport/mechanism inputs are not accepted"}, 2)
	}
	protected, err := proofenv.CaptureProtected(cfg.RepositoryRoot)
	if err != nil {
		emit(output{Error: err.Error()}, 1)
	}
	cfg.ProtectedRoots = protected.Paths
	result, err := proofgit.Run(context.Background(), cfg)
	out := output{Result: result}
	code := 0
	if err != nil {
		out.Error = err.Error()
		code = 1
	}
	emit(out, code)
}

func emit(v output, code int) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "failed to encode redacted proof result")
		os.Exit(1)
	}
	os.Exit(code)
}
