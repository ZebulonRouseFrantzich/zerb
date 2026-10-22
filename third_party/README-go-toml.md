# Local go-toml decoder cleanup

This is the owner-approved PC-042 local variant of
`github.com/pelletier/go-toml/v2`, identified as `v2.4.3-zerb.1`. No remote
version or fork commit is claimed. Root `go.mod` selects the exact version with
a version-specific local replacement.

The complete original module is upstream `v2.4.3`, commit
`071a36c2a57244f2e70369bfc69889fda2a1f60f`, authenticated by module checksum
`h1:GTRvJQutkOSftxIFD5xw9aepkYNuPWmVJpffdDPYVpY=`. Its 113 files, including
the MIT license, remain present. The only production difference is whole-decoder
zeroing before returning to the pool, plus its explanatory comment. This drops
input references, AST/tracker storage and scratch capacities on both success
and error; returned models and errors retain their own backing bytes. More
per-call allocation is an accepted tradeoff. The added regression file verifies
complete release, decreasing-size input histories, captures and result/error
stability without garbage collection or heap sampling.

The complete 114-file variant has module checksum
`h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=`. Its `go.mod` is
byte-identical to upstream, so its go.mod checksum is unchanged. In the existing
`Parser` record, `OriginCommit` explicitly means the upstream base; `ModuleSum`
binds the actual patched source. The external provenance JSON binds original
file hashes, the exact patch and the complete patched module. No provenance file
is inserted into the hashed module itself.

## Reproduction and checks

1. Obtain the original module ZIP and verify its standard Go module checksum
   above. The reviewed archive's SHA256 is
   `bc0de16d4942df8503be13bde0447e0b7ce1694f3bb1b840f485ce839f8974cf`.
2. Extract its complete module file set, stripping only the
   `github.com/pelletier/go-toml/v2@v2.4.3/` prefix. Apply
   `go-toml-decoder-reset.patch` and add the committed
   `go-toml/decoder_pool_release_test.go`. No other original file changes.
3. Run the read-only verifier with the pinned Go 1.24 executable:

   ```sh
   python3 scripts/check_parser_pin.py --go /absolute/path/to/pinned/go
   go -C third_party/go-toml test -race -count=1 ./...
   ```

The verifier reconstructs Go's `dirhash.Hash1`: sort complete prefixed file
names, hash each file, concatenate `hexSHA256 + "  " + name + "\n"`, then SHA256
and base64 the summary. It first authenticates the recorded upstream file hashes
against the official checksum, then verifies the fork inventory, exact patch,
patched `unmarshaler.go`, unchanged base files, module hash and effective Go
selection. Missing, extra, altered, non-regular or symlinked inputs fail. It
never rewrites pins and is a routine development check, not a sandbox against
concurrent hostile writers.

Ordinary `go mod verify` does not authenticate local replacements; root
`go test ./...` skips this nested module. Cloud conformance and CI therefore
require both the separate source/selection check and explicit upstream race
suite. Changing source or tests changes the module identity and needs review.
The original module checksum entries in root `go.sum` remain upstream-base
provenance, not an assertion about patched bytes.

## Validation limits

Run checks with disposable HOME/XDG/TMP roots and put coverage/benchmark output
outside this source tree. The complete upstream source and contributor guidance
are retained. Its separate Go 1.26/golangci-lint 2.8 CI, capslock and benchstat
checks require tools not present in this task; no unrelated toolchain upgrade or
tool installation is implied. Local Go 1.24 tests, race checks, coverage and
ten-sample diagnostic benchmarks are distinct from those upstream CI results.
An additional upstream `go vet ./...` run fails on the intentionally malformed
struct tag in `marshaler_test.go:1572`, which is byte-identical to the original
module. Root ZERB vet does not scan this nested module; neither that fixture nor
the vet configuration was changed to hide the failure.
The complete import also preserves upstream whitespace that `git diff --check`
flags when these files first enter this repository; literal unified-patch
context has the same issue. Reformatting either would change the approved
identity. Check authored ZERB changes separately and keep the source pin exact.
The one-line production change adds no imports or capability-bearing call.

Local conformance does not establish frozen-host qualification, Phase 2
completion, release support or persistent saved-environment configuration.
