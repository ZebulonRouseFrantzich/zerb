# Replacement import gate

Run from the repository root:

```sh
go run ./tools/importcheck -root .
go test ./tools/importcheck
go test -race ./tools/importcheck
go vet ./tools/importcheck
```

This read-only, standard-library developer tool enforces **ARCH-030 import
edges**, the hard-cutover replacement seam, and delivery's Phase 2 first work
item. It checks introduced replacement trees during the admitted non-release
foundation work. It is not cloud admission, Phase 1 qualification, completed
Phase 2 implementation, or Phase 6 acceptance.

The exact scanned directories are `internal/paths`, `internal/profile`,
`internal/receipt`, `internal/safefs`, `internal/execenv`, `internal/source`,
`internal/toolchain`, `internal/mise`, `internal/home`, `internal/state`,
`internal/recovery`, `internal/plan`, `internal/app`, `internal/host`, and
`internal/testenv`. Missing directories are allowed: later packages must not be
introduced as compile-only scaffolds. The output always reports the number of
target Go files; zero files passes this edge check and claims no implementation.

Every `.go` file in these trees is parsed, including `_test.go`, hidden or
underscore-prefixed names, platform-specific files, and files excluded by build
constraints. Build tags cannot bypass the gate. Unlisted subpackages are rejected,
including ones under `testdata`; their Go files are still checked. Parse/read
errors fail closed. Imports count regardless of ordinary, renamed, blank, or dot
aliases.

Both production and test imports obey the exact ARCH-030 table. All ten forbidden
old trees and their descendants fail: `config`, `service`, `git`, `drift`,
`chezmoi`, `transaction`, `shell`, `platform`, `binary`, and `testutil` beneath
`internal/`. Every unlisted import within this module fails too, including proof
harnesses and lower-package imports of `host`, `app`, or `cmd/zerb`. A `_test.go`
file may additionally import `internal/testenv`; production may never import it.
An external test named `<owner>_test` may import its own subject package. This is
a test edge and does not widen the production DAG. `testenv` itself may import
only `paths` and `safefs` among production replacement packages. Host's allowed
assembly imports are checked as edges; its assembly-only purpose requires review.

The main module identity is fixed to
`github.com/ZebulonRouseFrantzich/zerb` and checked against `go.mod`; changing the
module directive cannot disable local-edge enforcement. Matching uses exact
module/tree boundaries, so a similarly named external module is not mistaken for
this module. External and standard-library import resolution, dependency approval,
retention and version/checksum verification belong to compilation, read-only module
checks, and the hard-cutover dependency review. This tool does not approve them.
It does not resolve imports, compile fixtures, execute source, invoke subprocesses,
access the network, or modify files.

Phases 1–5 deliberately retain the old public `cmd/zerb` entrypoint. This gate
explicitly excludes that directory, all old package trees, proof harnesses, and
other tools from its source scan. It has no inferred phase or automatic cutover
mode. Phase 6 must separately enforce the command's `app`, `host`, `execenv`
edges, remove old artifacts/dependencies, and run the full installed-binary and
review gates. No passing foundation result stands in for those requirements.

Only `-root` is supported; it defaults to `.` and must identify a directory with
the expected `go.mod`. An explicitly selected root symlink is canonicalized once.
Repository-relative symlink entries in `go.mod`, `internal`, or any scanned tree
are rejected without following them, including source-file and directory links.
Diagnostics use sorted repository-relative paths. This is a routine developer
checker and does not promise confinement against hostile concurrent filesystem
replacement. Tests use only owned temporary repositories and outside sentinels;
they never read or write the developer's HOME/XDG or manager state.

Exit statuses are `0` for a clean scan/help, `1` for source/edge/tree violations,
and `2` for invalid arguments or an unusable root/module. `go run` may wrap a
nonzero program status. The gate checks syntactic import edges only: semantic
aliases, copied types, wrappers, re-exports, registries, fallback behavior, service
purpose, and API or architecture correctness still need implementation review.
