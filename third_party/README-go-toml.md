# Temporary go-toml decoder cleanup

`github.com/pelletier/go-toml/v2@v2.4.3-zerb.1` is a temporary, owner-approved
PC-042 fix. Zeroing the decoder before pooling drops historical input, AST,
tracker and scratch backing on success and error. Model/error behavior remains
unchanged; additional per-call allocation is accepted. This is conservative
reservation accounting, not a runtime-memory cap.

Only the following inputs are checked in; the expanded module is ignored:

- `go-toml-v2.4.3.zip`: complete authentic upstream module, including all 113
  source/test/fixture files; SHA256
  `bc0de16d4942df8503be13bde0447e0b7ce1694f3bb1b840f485ce839f8974cf`.
- `go-toml-decoder-reset.patch`: the minimal production change and comment.
- `go-toml-decoder_pool_release_test.go.txt`: focused release/history and
  model/error stability regression, copied to its original Go test filename.
- `go-toml.LICENSE`: visible unchanged upstream MIT license.
- `go-toml.provenance.json`: original file hashes and exact source identities.

Upstream base is `v2.4.3`, commit `071a36c2a57244f2e70369bfc69889fda2a1f60f`,
module sum `h1:GTRvJQutkOSftxIFD5xw9aepkYNuPWmVJpffdDPYVpY=`. The unchanged
114-file prepared variant has module sum
`h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=` and unchanged go.mod sum
`h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=`. `Parser.OriginCommit`
denotes the upstream base; `ModuleSum` binds patched bytes. No remote fork
commit or published variant is claimed. Root go.sum entries describe the base.

## Fresh-checkout preparation

On Linux with Python 3 and the existing pinned Go 1.24 toolchain:

```sh
python3 scripts/check_parser_pin.py --prepare --go "$(command -v go)"
python3 scripts/check_parser_pin.py --go "$(command -v go)"
go -C third_party/go-toml test -race -count=1 ./...
```

Prepare before any Go build/test/lint or cloud report. No network, package
installation or external patch tool is used. The checker authenticates the ZIP
before parsing, enforces the exact original inventory and file hashes, applies
only the recorded replacements and adds the exact regression. It verifies the
complete fixed patched checksum before publishing a temporary sibling directory
with Linux `renameat2(RENAME_NOREPLACE)`. Unsupported publication fails closed.
A valid existing tree is verified and reused; altered source, extra files,
links or an unrelated directory fail without repair or overwrite.
Existing-tree verification admits only authenticated file and parent paths before
descending, and bounds each content read and the aggregate source bytes. Corrupt
directory depth or oversized source cannot bypass those limits.
Interrupted
preparation publishes no partial tree; hard termination may leave an unreferenced
private `.go-toml-*` staging directory. Packaged inputs are checked on every
invocation, including reuse; default verification is read-only. This is build
preparation, not a sandbox against concurrent hostile worktree writers.

Root `go test ./...` skips the nested module and `go mod verify` skips local
replacements. The source/selection verifier and explicit upstream race gate
remain mandatory. Cloud inventory never prepares source, and ignored expanded
files remain included in the report's before/after source identity. CI prepares
before both test and lint jobs. Other locked dependencies still need their
ordinary cache/install setup; this bundle supplies the parser offline.

## Returning to the normal upstream package

Do not upgrade automatically. Once an explicitly approved official release
clears all decoder-owned input-dependent backing on success and error:

1. Authenticate its official module/version/commit/checksums, review its actual
   cleanup and re-review the PC-042 reservation against the new implementation.
   Run release/history, model/error, full upstream, parser/fuzz/race and affected
   cloud checks. A release note alone is insufficient.
2. Replace the temporary requirement with that exact normal upstream version
   and remove the local `replace`. Update owning contracts and every parser
   identity producer to the official identity; preserve historical evidence.
3. Remove this ZIP, patch, regression workaround, license copy, provenance,
   generated/staging-directory ignore rules and dedicated preparation/verifier/tests.
   Remove preparation from CI/docs and replace temporary gate commands with the
   reviewed normal-package verification; retain equivalent regression coverage.
4. Validate a clean checkout, module verification and all affected gates with
   no generated local parser directory or replacement. Record the superseding
   review/evidence. Never depend on an old prepared tree or mutate old records.

## Validation limits

Upstream's separate Go 1.26/lint 2.8 CI, capslock and benchstat are unavailable
here. Extra upstream vet flags its unchanged intentionally malformed struct tag
in `marshaler_test.go:1572`. The authenticated archive retains upstream whitespace,
fixtures and generated conformance tests without expanding them into the PR.
Local checks do not establish frozen-host qualification, Phase 2 completion,
release support or persistent saved-environment configuration.
