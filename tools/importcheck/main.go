// importcheck checks ARCH-030 import edges in introduced replacement trees.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("importcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root (foundation replacement trees only)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "importcheck: unexpected operands")
		return 2
	}
	result, err := check(*root)
	if err != nil {
		fmt.Fprintf(stderr, "importcheck: %v\n", err)
		return 2
	}
	for _, diagnostic := range result.diagnostics {
		fmt.Fprintln(stdout, diagnostic)
	}
	fmt.Fprintf(stdout, "importcheck: %d error(s), %d target Go file(s); foundation import edges only (cmd/zerb excluded; not Phase 6 acceptance)\n", len(result.diagnostics), result.files)
	if len(result.diagnostics) != 0 {
		return 1
	}
	return 0
}
