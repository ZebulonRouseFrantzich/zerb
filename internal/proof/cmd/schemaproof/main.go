// schemaproof is a disposable Phase 1 schema/parser proof executable.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/proofenv"
	"github.com/ZebulonRouseFrantzich/zerb/internal/proof/schema"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("schemaproof", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "existing canonical disposable btrfs root outside repository and real HOME/XDG")
	repository := fs.String("repository", "", "canonical repository root to exclude")
	artifacts := fs.String("artifacts", "", "existing artifact directory beneath root")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return fail("unexpected operands", 2)
	}
	if *repository == "" {
		return fail("repository root is required", 1)
	}
	protected, err := proofenv.CaptureProtected(*repository)
	if err != nil {
		return fail(err.Error(), 1)
	}
	result, err := schema.Run(context.Background(), schema.Config{Root: *root, ArtifactDir: *artifacts, ProtectedRoots: protected.Paths})
	if err != nil {
		return fail(err.Error(), 1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(true)
	if err = enc.Encode(result); err != nil {
		return fail("encode structured result", 1)
	}
	return 0
}

func fail(message string, code int) int { fmt.Fprintln(os.Stderr, "Error:", message); return code }
