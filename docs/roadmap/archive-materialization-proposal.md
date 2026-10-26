# Managed archive materialization: approved direction and evidence handoff

Status: **approved direction; actual artifact and resource-fit gates pending**.
Source inspection: refocus `6098d7aa76ea1eb5c9b6cff12f5a85f4ca802eb6`, whose
tree equals reviewed TreeStage head `760b90d`. The owner approved chezmoi first,
canonical uncompressed USTAR and independently reviewed offline-manifest
authority, with mise deferred and real build/inventory/fit required before
dependent coding. [ARCH-012](architecture.md#chezmoi-first-archive-preparation)
owns that direction; [VAL-028](validation.md#chezmoi-archive-preparation-gate)
owns its evidence gate. This note remains non-normative and does not supply
artifact bytes, cleanup authority, executed evidence or qualification.

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

## Approved authority and representation direction

Use a private, per-call owned byte snapshot in `toolchain`, selected by an
independently reviewed frozen artifact recipe. Bound length before copying;
then authenticate that exact owned snapshot completely before creating any
decompressor/archive reader. Never expose mutable backing storage outside
trusted private extraction/publication code; borrowed entry slices are read-only
by that internal contract, and existing publication does not modify them.
The archive/extraction code cannot create, choose or self-attest its own trust
root. Keep provenance verification failure distinct from a successful hash
check in tests, with fixed redacted operational errors.

This direction adds no export or constructor. Do not use
`NewAuthenticated(reader, callerDigest)` or a boolean verification argument as
an authority shortcut. Tests may have explicit synthetic recipes, but no test
fixture, test-only key or mock result enters the production manifest.

Reusing existing publication is preferable to adding a streaming API before
artifact facts require one. `Dir.PublishNoReplace` borrows caller bytes: its
input is not automatically TreeStage-owned workspace. Therefore it does not
inherently exceed the stage's 128 MiB reservation. The materializer must
separately account for its snapshot, decoded inventory, entry buffers and
parser/decompressor state, and report the combined simultaneous bound.

For the approved raw USTAR, a validated entry may be a temporary bounded slice
of the private snapshot without another payload copy. Compressed formats are
outside the first artifact grammar, so no decompressor is added here.
`PinFile` returns an append-grown whole buffer and offers no exported streaming
reader. A hash of an open mutable file followed by reopening is not a substitute
for the owned snapshot. A sealed descriptor-backed representation is an
alternative if measured input cannot fit; it needs its own authority, kernel,
descriptor, storage and lifecycle contract before implementation.

## Build and evidence handoff

The selected planned artifact is
`zerb-chezmoi-v2.70.5-zerb.1-linux-x86_64.ustar`; it has no established URL,
size or hash yet. The anticipated set is `bin` (`0700` directory),
`bin/chezmoi` (`0700` regular executable) and `LICENSE` (`0600` regular notice).
That set remains incomplete until actual runtime/notice inspection confirms it.
The fork's pinned `LICENSE` Git blob is
`24f8fc050ff6d61fbf2708b29e8330d6ae11cf28`, 1,076 ASCII bytes. A Git blob ID is
not a substitute for the manifest's eventual full file SHA256.

### Exact existing build recipe

Use this repository's unchanged `packages.x86_64-linux.chezmoi-zerb`, not an
upstream release or an ad-hoc local Go build. Existing frozen inputs are:

| Input | Existing value/source |
|---|---|
| nixpkgs | `b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`; `flake.lock` NAR hash `sha256-IItrdb7Puk05RqOBWZYFC5X6Wl1sJmCfh5MWVHw5iMM=` |
| Fork source | `ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`; source NAR hash `sha256-xM3lgbhXEWEXX5RkviTujfNfqLrjl3RRr1evBXhII9o=` |
| Vendor input | `sha256-jCKO/ih/HRLZ5hv4Ib8qB9kIJagJQq79wtSqBi+BgCY=` in `flake.nix` |
| Build settings | `buildGoModule`, `subPackages = [ "." ]`, `CGO_ENABLED=0`; linker flags `-s`, `-w`, `main.version=v2.70.5-zerb.1`, `main.commit=ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`, `main.date=1787510541`, `main.builtBy=zerb-phase1-proof` |
| Existing upstream check | `go test ./internal/cmd -run '^TestManagedCmd(SkipTemplateDataEnvironment)?$' -count=1` in the Nix check phase |
| Compiler | The compiler/derivation selected by this locked Nix recipe, with its actual identity recorded. The fork declares Go 1.26; the cloud project compiler Go 1.24.9 cannot replace that requirement. No new project Go pin is introduced. |

### Required supported environment

An existing Linux x86_64 Nix environment must already support the repository's
locked flake and have an approved builder/store/cache/trust configuration and
disposable build/evidence space. Record the actual Nix/compiler/build identities;
this note does not invent a Nix version pin or authorize enabling features,
installing Nix/Go, expanding access or changing trust settings. A non-qualifying
Linux builder may produce build/inventory evidence; it does not thereby qualify
manager execution. Fedora/kernel/Btrfs runtime qualification remains separate.

The existing locked build target, with explicit refusal to update/write locks,
is:

```sh
nix build --no-update-lock-file --no-write-lock-file --no-link --print-out-paths .#chezmoi-zerb
```

This is a handoff command, **not executed here**. Inspect/verify `flake.nix` and
`flake.lock` and require their exact unchanged locked inputs before and after.
The existing Nix must support these flags; their distinct update/write guards
are defined in the [Nix CLI source](https://github.com/NixOS/nix/blob/2.24.11/src/libcmd/installables.cc).
This reference verifies flag meaning, not a new required Nix version. Do not enter
the default dev shell: its legacy hook changes shell state and creates files.
Do not replace the command with downloaded executable version probes or manual
Go toolchain acquisition. A substituted realization, lock update or missing
retained identity stops the handoff rather than silently defining a new build.

### Required evidence return

1. Retain actual derivation/store-output/compiler identities, authenticated
   source/vendor facts, complete build settings and check results. Distinguish a
   cache realization from an independently reproduced build. Independent review
   must authenticate the provenance chain; a build-produced JSON/hash does not
   self-attest. Preserve and compare any retained reviewed executable digest.
2. Without invoking the output, identify the exact regular executable and all
   runtime/notice files, full byte sizes/hashes, ELF architecture/interpreter/
   dependency facts and relocation assumptions. Neither a successful build nor
   `CGO_ENABLED=0` alone closes this row. No outside path becomes an admitted
   runtime dependency merely because an ELF or source file names it.
3. Only after that complete set is reviewed, produce the canonical raw USTAR
   envelope with exact fixed headers, modes, explicit parents and EOF. Bind its
   full size/hash and complete entry inventory into the independently reviewed
   offline manifest. Do not publish a release or assign invented hashes/URLs.
4. Complete the concrete reservation/read/descriptor worksheet below against
   those bytes and resolve independent review before dependent production code.
   No stub Service, speculative streaming API or stage-reservation exemption is
   admitted. If fit fails, preserve evidence and present the specific necessary
   design change for approval.

For exactly the anticipated three entries, archive size would be
`roundUp512(executableBytes) + 4,096`. This uses three 512-byte headers, the
1,076-byte license padded to 1,536 bytes and two EOF blocks. It is a conditional
arithmetic relation, not an authenticated executable size or passing fit proof.
Additional required files change the calculation and must be included.

### Current cloud observation

Read-only checks found no `nix`, `nix-store`, `/nix/store`, `mise` or `chezmoi`.
The available project compiler is Go 1.24.9. No authenticated manager output,
archive or inventory was available in the inspected current environment, and
the earlier temporary conformance report was absent after environment restart.
No retained report or old binary identity is reconstructed from memory.

The exact stopped action is realizing the locked chezmoi build and collecting
authenticated output/inventory/fit evidence; its status is **not attempted:
required build environment unavailable**, not denied or executed successfully.
No install, configuration change, binary download, build, manager invocation,
owner-host operation or qualification run occurred. Until the supported builder
and missing evidence are available, the approved dependent coding gate stays
closed. Ordinary source/document preparation can continue independently.

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

The approved direction is recorded in the owning architecture and validation
documents. Do not mark this worksheet passed or create a
placeholder Service while any artifact authority or representation row is
unresolved. Existing twelve safefs and 46 state-integration qualification gaps,
full crash-matrix execution and frozen-host/storage acceptance remain open.
