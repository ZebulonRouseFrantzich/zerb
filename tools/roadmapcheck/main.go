// roadmapcheck checks document structure only; it is not a qualification gate.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("roadmapcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root (reads docs/roadmap recursively)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "roadmapcheck: unexpected operands")
		return 2
	}
	diagnostics, err := check(*root)
	if err != nil {
		fmt.Fprintf(stderr, "roadmapcheck: %v\n", err)
		return 2
	}
	errors, warnings := 0, 0
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(stdout, diagnostic)
		if diagnostic.warning {
			warnings++
		} else {
			errors++
		}
	}
	fmt.Fprintf(stdout, "roadmapcheck: %d error(s), %d warning(s); document structure only\n", errors, warnings)
	if errors > 0 {
		return 1
	}
	return 0
}
