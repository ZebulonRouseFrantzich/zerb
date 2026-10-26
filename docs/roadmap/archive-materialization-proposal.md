# Managed archive materialization: preparation and decisions

Status: **proposal, not an adopted contract or implementation evidence**.
Source inspection: refocus `6098d7aa76ea1eb5c9b6cff12f5a85f4ca802eb6`, whose
tree equals reviewed TreeStage head `760b90d`. ARCH-012/020/021/030 own the
requirements; VAL-028 owns archive observations and qualification. This note
does not freeze a format, grant acquisition/cleanup authority, or amend a gate.

## Bounded implementation objective

Authenticate one explicitly retained manager artifact, validate its complete
inventory, and materialize it into a fresh capability-owned TreeStage. No
network, manager execution, source acquisition, state integration, production
service scaffold, public command or automatic promotion belongs in this slice.
The later trust-owning operation alone controls promotion. No obsolete/proof
package may supply production code or authority.

## Requirements already settled

| Requirement | Existing authority and consequence |
|---|---|
| Input authenticity | ARCH-012/021: verify the frozen provenance contract and the complete size/SHA256 before any archive parser receives bytes. A caller-selected digest is not provenance. |
| Frozen selection | ARCH-030: Artifact and runtime manifests are frozen production data, never source/profile input. The exported Artifact fields alone are not a verified capability or a specified provenance verifier. |
| Safe contents | ARCH-021: reject links, devices/sockets, unsupported types/modes, aliases, duplicates, traversal and resource overflow. Never use the Node installation exception as generic manager extraction permission. |
| Protected output | ARCH-020: use retained descriptors and TreeStage's shared lifetime/reservations; never extract by an ambient pathname or infer cleanup rights from hashes, names or process death. |
| Completion | ARCH-012/021: independently verify the expected executable identity and complete required inventory. TreeStage publication does not authenticate those contents. |
| Failure | No overwrite, fallback extraction, automatic repair, false rollback or restart adoption. An uncertain stage is preserved and its views revoked according to ARCH-020. |

## Actual source findings

`internal/toolchain` currently implements FixedCA, not an artifact manifest,
provenance verifier, archive parser or acquiring Service. ARCH-030's
`Artifact.Provenance []byte` does not define a production encoding or trust
root. The adapter proof accepts exact executable paths; its unused
`Config.ArtifactDir` is not a manager archive manifest.

The retained mise candidate comes from nixpkgs
`b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`. Its
[derivation](https://github.com/NixOS/nixpkgs/blob/b7c2ada94fe99c15b0dbcf4d11fd7850b957a436/pkgs/by-name/mi/mise/package.nix)
builds version 2026.7.17 from source, rewrites dependency paths and installs
runtime/package files. Its source hash is not an installable binary archive's
SHA256, and an upstream same-version release is not that exact executable.

The retained chezmoi candidate is the CGO_ENABLED=0 build at public fork commit
`ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`. A read-only check of the fork's
release list returned no releases. This does not prove no distributable exists
elsewhere; the source pin/build recipe alone still does not identify one.

No exact distributable bytes, authenticated inventory or complete frozen
archive-provenance procedure was established by this preparation. No binary
archive was fetched, parsed, built, executed, installed or qualified.

The USTAR implementation in `internal/proof/filesystem/filesystem.go` is an
auditable example, not reusable production authority. Its provenance constant
binds one state-proof fixture, and `ValidateTarIntegrity` explicitly does not
authenticate a caller's expected digest. Its modes, limits and grammar do not
silently become manager policy. The Node archive proof separately admits
symlinks, 200,000 entries and 1 GiB; it cannot widen ARCH-021 or TreeStage.

## Recommended authority and representation direction

Prefer a private, per-call owned byte snapshot in `toolchain`, selected by an
independently reviewed frozen artifact recipe. Bound length before copying;
then authenticate that exact owned snapshot completely before creating any
decompressor/archive reader. Never expose mutable backing storage outside
trusted private extraction/publication code; borrowed entry slices are read-only
by that internal contract, and existing publication does not modify them.
The archive/extraction code cannot create, choose or self-attest its own trust
root. Keep provenance verification failure distinct from a successful hash
check in tests, with fixed redacted operational errors.

This is a proposed direction, not an added export or constructor. Do not use
`NewAuthenticated(reader, callerDigest)` or a boolean verification argument as
an authority shortcut. Tests may have explicit synthetic recipes, but no test
fixture, test-only key or mock result enters the production manifest.

Reusing existing publication is preferable to adding a streaming API before
artifact facts require one. `Dir.PublishNoReplace` borrows caller bytes: its
input is not automatically TreeStage-owned workspace. Therefore it does not
inherently exceed the stage's 128 MiB reservation. The materializer must
separately account for its snapshot, decoded inventory, entry buffers and
parser/decompressor state, and report the combined simultaneous bound.

For raw tar, a validated entry may be a temporary bounded slice of the private
snapshot without another payload copy. For compressed input, decoded bytes,
decompressor retention and every repeated pass must be charged. Those two
representations have different fit arguments; format selection comes first.
`PinFile` returns an append-grown whole buffer and offers no exported streaming
reader. A hash of an open mutable file followed by reopening is not a substitute
for the owned snapshot. A sealed descriptor-backed representation is an
alternative if measured input cannot fit; it needs its own authority, kernel,
descriptor, storage and lifecycle contract before implementation.

## Consolidated decision before dependent coding

1. **Retained distributables and provenance.** Recommend preserving the exact
   source-built manager identities and obtaining reproducible distributables
   with a reviewed archive hash, executable hashes, complete runtime closure and
   explicit provenance-verification recipe. Do not substitute official upstream
   release binaries by version equality. Defining a project-produced envelope
   is a packaging decision for approval; creating/publishing one is not
   authorized by this note. The alternative is selecting different release
   artifacts and renewing every affected executable/provenance/runtime proof.
2. **Closed format and layout.** Recommend the smallest single format actually
   used by those approved distributables, exact entry inventory and executable
   locations, no links and no implicit root stripping or mode repair. Freeze
   compression/member/trailing-data rules, tar dialect and metadata rules,
   directory spelling/ordering, archive mode versus published mode mapping and
   runtime-file requirements together. A deterministic project-produced raw
   USTAR envelope would simplify bounded parsing and avoid decompression; it is
   an option, not an approved format. Supporting arbitrary upstream archives is
   a larger grammar and review burden, not an automatic compatibility goal.
3. **Measured resource schedule.** Recommend the private snapshot plus existing
   publication only after the following worksheet fits actual authenticated
   inventories. Specify any additional materializer workspace/read-work limits
   explicitly; TreeStage's 128 MiB is not a whole-process memory allowance.
   If a snapshot or file cannot fit the reviewed schedule, return the concrete
   descriptor/streaming alternative for approval rather than silently raising
   caps, narrowing the accepted artifact or bypassing the stage gate.

These decisions add no dependency, package version or public CLI surface by
default. No provenance trust root, exported filesystem API, runtime closure,
artifact identity or host permission changes automatically follow from the
general instruction to begin this work.

## Concrete fit worksheet: not yet passed

Each retained artifact needs independently authenticated values for every row.
Until then neither representation fit nor artifact compatibility is established.

| Quantity | Required bound/check |
|---|---|
| Complete archive bytes A | Exact frozen size/hash and provenance first; declared upper bound checked before copying/decoding. No invented archive size cap from an unrelated proof. |
| Entries N, depth, names and paths P | Include all explicit and required implicit directories; at most 16,384 descendants, depth 64, 255-byte components, 1,024-byte paths and 16 MiB aggregate paths. A format may need a separately approved narrower grammar. |
| Largest file F, full payload T | At most 256 MiB and 512 MiB respectively, including non-executable runtime files. Sparse/expanded logical sizes cannot evade the totals. |
| Stage workspace | Unchanged ARCH-020 reservation `1 MiB + 2048N + 4P + 512FD + scratch + R <= 128 MiB`. Count actual topology, retained roles and caller-owned views as its existing accounting requires. |
| Materializer workspace | Snapshot capacity A, complete metadata/index/path capacities, simultaneously decoded entry buffers, parser/decompressor state, scratch and retained copies. Account backing capacity, not only logical lengths. Sum with simultaneous stage workspace; do not describe it as a hard Go heap/RSS cap. |
| Content read work | Separately count archive authentication/parsing/decompression and output verification passes. Stage's 4 GiB lifetime reads remain unchanged, including lookahead and cleanup/terminal passes. |
| Descriptors | Include caller-held input/output/ancestor/proc/role chains and simultaneous construction/verification opens; obey the stage's 256-owned-FD bound without changing host limits. |
| Executable/runtime closure | Exact output bytes, ELF/interpreter/library and required resource relationships match the retained executable/host contract; a repackaged file does not prove relocatability. No execution occurs in this slice. |

## Focused future validation matrix

All rows are requirements for the future implementation, **not executed results**.

| Boundary | Required observation |
|---|---|
| Before parse | Wrong/missing provenance, wrong identity, size/hash mismatch, nil/zero capability and input bounds reject before archive/decompressor entry and before stage creation. |
| Snapshot ownership | Input mutation after snapshot construction cannot change parsed bytes; no returned mutable alias, historical retained buffer or closed capability supplies authority. |
| Grammar | Reject every unapproved dialect, metadata/header/type/mode, ambiguous name, duplicate, traversal, link/special file, sparse form and invalid directory/root representation. |
| Container completion | Reject truncated body/padding, corruption, trailing/concatenated input and decompression expansion according to the approved exact format. Do not rely on early tar EOF or gzip default multistream behavior. |
| Allocation order | Count raw/hidden headers and bound storage before standard-library metadata allocation. Go's tar reader consumes some extension headers internally; checking returned file headers alone does not establish that bound. |
| Inventory | Missing/extra executable/runtime entries, wrong bytes/modes/hash and same-version wrong builds refuse. Prevalidate the complete inventory before materialization; no trusted success from a partial prefix. |
| Fit | Below/exact/above all approved scalar and simultaneous limits; checked arithmetic, retained capacities, repeated parsing and failure/cancellation cleanup. No proof cap becomes production authority by copying it. |
| Materialization | Descriptor-relative ordinary entries only, existing targets untouched, exact bytes/modes, restrictive umask refusal, outside-sentinel preservation and no arbitrary pathname access. |
| Lifecycle | Relevant stage failures revoke all views. Keep the caller's lease/role ownership separate. Preserve uncertain leftovers, close known-owned descriptors and never rediscover cleanup authority after death. |
| Completion/review | Focused and race checks, source-bound conformance and independent security review; distinguish helper observations from skipped exported-topology tests. No installed/host/support promotion. |

Before production coding, adopt reviewed decisions in the owning architecture
and validation documents. Do not mark this worksheet passed or create a
placeholder Service while any artifact authority or representation row is
unresolved. Existing twelve safefs and 46 state-integration qualification gaps,
full crash-matrix execution and frozen-host/storage acceptance remain open.
