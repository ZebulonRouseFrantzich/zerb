# Cloud development and Phase 1 frozen-host handoff

This is an operator checklist for cloud conformance and the existing frozen-host proof commands, not a requirement owner or proof result. [Validation](validation.md) owns candidates and evidence; [delivery](delivery-plan.md) owns the gate. Every candidate remains inconclusive. Local tests on another host cannot qualify it.

## Cloud development admission

The owner approved reviewed non-release Phase 2–4 implementation after the separate cloud gate in [delivery](delivery-plan.md). Before starting that work, obtain a passing current source-bound cloud report and resolve independent plan and focused security review of the affected plan/contracts. The sequencing approval itself is not evidence that either prerequisite passed. Complete Phase 1 qualification/freeze still requires the full real-host run below, its reviews and owner freeze approval; real frozen-host qualification/freeze is required before local Phase 6 public-entrypoint cutover work. Release, real-workstation use and support additionally require complete Phase 6 installed-binary acceptance and final reviews; this handoff grants no publication, push or merge authority.

Use the activated Go 1.24 development tools and configured linter. In this linked cloud worktree use process-scoped `GOFLAGS='-mod=readonly -buildvcs=false'` with `/workspace/.devtools/activate.sh`; the VCS exception is local build plumbing, not a host qualification or trust-setting change. Use disposable build caches outside the repository and real HOME/XDG graph. Supply an output path outside those protected roots as a private operational report:

```sh
python3 scripts/check_parser_pin.py --prepare --go "$(command -v go)"
python3 scripts/cloud_conformance.py --output "$CLOUD_REPORT"
```

Preparation reconstructs the ignored temporary TOML dependency offline before source capture; the cloud collector and its inventory-only mode remain read-only with respect to source. The complete expanded input remains included in the report's before/after identity. A fresh checkout needs this step before any Go command; see the [packaging and upstream-reversion procedure](../../third_party/README-go-toml.md).

Separately run `go run ./tools/roadmapcheck` for roadmap definitions, references and local links. Its [documented scope](../../tools/roadmapcheck/README.md) is structural only; it grants no phase admission or qualification. Historical warnings in the append-only ledger remain visible without rewriting prior observations.

The current fixed check set is disposable `go test -race -count=1 ./internal/proof/...`, all-package vet, configured lint, read-only module verification, a release-shaped compile of the unchanged old entrypoint, the cloud script boundary tests, `go run ./tools/importcheck`, `go test -race -count=1 ./internal/paths ./internal/safefs ./internal/profile ./internal/receipt ./tools/importcheck`, the read-only parser pin verifier with the measured Go executable, and `go -C third_party/go-toml test -race -count=1 ./...`. The compile neither installs the binary nor exercises target-runtime behavior. The report binds exact source HEAD/tree state to these results and records actual kernel/distribution/filesystem, process-prerequisite, tool and CA inventory. It distinguishes `passed`, `failed` and `not-exercised`; every required local check and source-identity check must pass. An inventory-only report never admits development. A frozen-host inventory mismatch alone does not fail cloud conformance, and a matching inventory cannot qualify a host. Inventory may execute tool version queries; it never runs cgroup, namespace, manager behavior, state/durability or other host-qualification proof probes. Reports always state `host_qualified=false` and `release_supported=false`.

Inspect the report and reviews before relying on admission, and rerun applicable checks/reviews when their source changes. Future replacement work additionally needs focused permanent replacement tests/race checks and the phase-specific reviews. The current proof-only race scope does not validate absent replacement packages or legacy full-runtime behavior. Missing runtime prerequisites must cause the real runtime/proof boundary to fail closed; no development fallback runner or weaker cgroup/namespace/filesystem/tool/CA policy is permitted. Record unavailable real-host success rows as not exercised and leave their candidate dispositions inconclusive.

Keep full reports, exact source/tree/tool hashes, local paths and check outputs private outside the repository; export only VAL-004/005-reviewed redacted observations to the append-only proof ledger. Do not mint a host-proof artifact reference from a local report. No host-policy change or repeated incompatible proof attempt is needed to run this cloud inventory/check gate.

## Host and inputs

Before a run, verify these inputs without changing access, credentials or host policy:

- Fedora Linux 44, kernel `7.1.10-200.fc44.x86_64`, x86_64, and a disposable btrfs filesystem (VAL-001/002). Record actual observations; a container, overlayfs or tmpfs is a different tuple.
- Existing writable cgroup v2 delegation at `/sys/fs/cgroup/user.slice/user-<uid>.slice/user@<uid>.service`, owned by the proof user, with `cpu memory pids` present in both `cgroup.controllers` and `cgroup.subtree_control`. The harness creates only disposable child cgroups there. It requires `memory.max=805306368`, `memory.swap.max=0`, `pids.max=96`, `cpu.max="100000 100000"`, `cgroup.kill`, `memory.peak`, `pids.peak`, `pids.current`, `memory.events` and `cpu.stat`. A read-only mount or missing delegation blocks the proof; arranging delegation is a separate host administration action, not a fallback in this runbook (ARCH-009/010, VAL-018/019).
- Existing user/mount/PID/network namespace capability, subordinate UID/GID ranges in `/etc/subuid` and `/etc/subgid`, and compatible `/usr/bin/unshare`/mapping helpers. The actual adapters use `unshare --map-users ... --map-groups ... --mount --pid --fork --kill-child --mount-proc --setuid 0 --setgid 0`; owner-substitution rows also need the mapped subordinate identity. Do not add ranges or relax namespace policy as part of a proof invocation.
- `/proc` descriptor/process observations, pidfds, sealed memfd ELF execution, seccomp, PTY `/dev/ptmx` and controlling-terminal/job-control operations must work. Managed Git/acquisition/mise/chezmoi require denial of all three `io_uring` syscalls; arbitrary exec/shell payloads retain their separate policy. Record actual hard-death, guard-zero-status and reaping observations (ARCH-010/011, VAL-018/020/023).
- Absolute canonical executables: Git **2.55.0**, Bash **5.3.15**, mise **2026.7.17**, the GNU coreutils **9.11 multicall executable** (not a shell wrapper or `sleep` alias), and the closed syscall tracer. The first four candidates come from locked nixpkgs revision `b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`. Record the actual tracer identity and tracing coverage; a missing/unclassified event fails rather than broadening the allowlist.
- Static chezmoi fork **v2.70.5-zerb.1**, commit `ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`, upstream v2.70.5, `CGO_ENABLED=0`. `flake.nix` already defines `packages.x86_64-linux.chezmoi-zerb` with pinned vendor hash and focused upstream checks. Its existing locked build command is `nix build --no-write-lock-file --no-link --print-out-paths .#chezmoi-zerb`. Do not update the lock or substitute upstream chezmoi. The uninitialized Git submodule is not evidence that this binary was built or checked.
- The reviewed public CA bundle, exact SHA256 `3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a`. Copy the retained fixture unchanged into the approved disposable input area outside the repository/home/XDG graph; use that copy as `CA` below. Supply a canonical, no-symlink, single-link regular file, at most 1 MiB, root/proof-user owned and not group/other writable, beneath validated ancestors. Its 472,033 bytes use the pinned NSS/buildcatrust OpenSSL record format, including labels and certificate trust AUX records; the exclusive Go TLS pool contains its 121 ordinary server-auth certificates and excludes 51 AUX-only certificates (ARCH-030). The exact artifact has been reproduced locally from authenticated pinned public sources; [fixture provenance](../../internal/proof/adapters/testdata/reviewed-ca-bundle.PROVENANCE.md) records inputs and generation. Frozen-host acquisition evidence is still required (VAL-017/021/022).
- The existing complete authored Node **24.4.1** lock fixture, copied unchanged from `internal/proof/adapters/testdata/mise.lock` into the disposable base. Its sole `tools.node` entry must include exact `core:node`, Linux x64 URL, SHA256 and size. Never generate a lock during the run. Acquisition is ZERB-controlled and authenticated; mise install itself runs offline with `--raw` (VAL-016/021).
- The retained public HTTPS source is `https://github.com/ZebulonRouseFrantzich/zerb.git`. DNS must admit an entirely globally public answer set and native Git must retain the permanent address pin and canonical TLS hostname. No proxy, redirect, authentication, private endpoint, helper or agent is an allowed input (VAL-006/007/009).

Use Go 1.24 with the existing `go.mod`/`go.sum`, and record the exact compiler identity. Earlier proof observations used Go 1.24.6; this task's Go 1.24.9 checks are local source validation only. Resolve candidate packages from the existing `flake.lock`; the default development shell is not a frozen proof-tool inventory. It also has a legacy shell hook, so entering it is not the proof setup procedure.

## Disposable layout and invocation

Begin with the real account HOME and ambient XDG paths intact. Commands capture their protected identities before constructing disposable child environments; setting parent HOME to a temporary directory defeats that precondition and is rejected. Supply an explicit canonical repository root even if invoking from elsewhere. Disposable base, artifacts, generated source, manager roots, destinations and sentinels must be disjoint from the repository and every captured home/XDG/credential path (VAL-002). Use separate, existing, owner-only btrfs bases for each slice, with separate artifact subdirectories and no nested proof roots.

The following commands are the actual command interfaces. Assign each variable to an already prepared canonical absolute path on the approved host; these assignments are operator inputs, not paths created or verified by this document. `PROOF_BIN` is an existing disposable build directory outside the repository/home/XDG graph. Set `GOCACHE` and `GOMODCACHE` to separate disposable build-cache directories there before dependency/build checks, so they do not write the real account home. Keep process-scoped `GOFLAGS=-mod=readonly` for checks/builds. `GIT_BASE`, `ADAPTER_BASE`, `STATE_BASE` and `SCHEMA_BASE` are the separate btrfs bases. Each `*_ARTIFACTS` directory already exists below its corresponding base. `NODE_LOCK` is the copied authored fixture beneath `ADAPTER_BASE`.

```sh
go mod download
go mod verify
go test -race -count=1 ./internal/proof/...
go vet ./...
go build -trimpath -o "$PROOF_BIN/gitproof" ./internal/proof/cmd/gitproof
go build -trimpath -o "$PROOF_BIN/adapterproof" ./internal/proof/cmd/adapterproof
go build -trimpath -o "$PROOF_BIN/stateproof" ./internal/proof/cmd/stateproof
go build -trimpath -o "$PROOF_BIN/schemaproof" ./internal/proof/cmd/schemaproof

"$PROOF_BIN/gitproof" --repo-root "$REPOSITORY" \
  --root "$GIT_BASE" --artifact-dir "$GIT_ARTIFACTS" \
  --git "$GIT" --trace "$TRACE" --ca-cert "$CA"

"$PROOF_BIN/adapterproof" --repository "$REPOSITORY" \
  --base "$ADAPTER_BASE" --artifacts "$ADAPTER_ARTIFACTS" --slice all \
  --git "$GIT" --trace "$TRACE" --bash "$BASH_BINARY" \
  --signal-fixture "$COREUTILS" --mise "$MISE" --chezmoi "$CHEZMOI" \
  --cacert "$CA" --node-lock "$NODE_LOCK"

"$PROOF_BIN/stateproof" --repository "$REPOSITORY" \
  --root "$STATE_BASE" --artifacts "$STATE_ARTIFACTS" \
  --git "$GIT" --cacert "$CA"

"$PROOF_BIN/schemaproof" --repository "$REPOSITORY" \
  --root "$SCHEMA_BASE" --artifacts "$SCHEMA_ARTIFACTS"
```

These are individual commands: stop and inspect a failure before proceeding. Keep stdout/stderr in the explicitly approved disposable capture/evidence area, subject to the full marker scan and redaction review. No raw trace or fixture identity is copied into Git. Do not treat an exit status alone as a pass: inspect every structured scenario, denied boundary, unknown trace event, marker sink, guard status, cgroup/OOM observation, survivor and outside-write count (VAL-003/005). Lifecycle-only adapter runs still require every executable/CA flag; an adapters slice additionally requires the authored lock.

In this linked cloud worktree only, Go 1.24's VCS scan reaches a protected enclosing repository. Local checks use process-scoped `GOFLAGS='-mod=readonly -buildvcs=false'` with `/workspace/.devtools/activate.sh`; this is not a trust-setting change or a requirement to suppress VCS evidence on the frozen host. Record source HEAD and tree state explicitly there.

## Results needed before freeze

Retain exact command/source/tool identities and redacted typed artifact references for native Git, lifecycle, adapters, state/filesystem and schema slices. Repeat every affected crash/kill/publication/transfer/restart row, including initialization failures returning still-owned acquisitions; state-owned empty READY repair with no candidate; pre-journal rejection of resume; unchanged-digest reapply with distinct completed operation/writer identity; recovery after commit without republishing current; and a fresh pre-journal operation after multiple cleanup transfers (ARCH-024/025/027/030, VAL-025–034).

Append only observed results to `proof-results.md` using VAL-004's closed evidence format. Do not mint typed artifact references until the corresponding artifacts exist. All required positive rows must become implementation-qualified and dependent negatives exclusion-proven, followed by independent plan review, focused security review and owner freeze approval. An approved contract correction, local commit, cloud conformance pass, unit-test pass or exported patch cannot advance that qualification/freeze gate or establish support. Reviewed non-release implementation may proceed separately through the cloud development gate above.

The cloud environment recorded in the prior handoff could not perform this frozen-host run: Debian 13.6/kernel 6.18.55, overlayfs/tmpfs, read-only cgroup mount, different Git/Bash and absent retained mise/chezmoi/Nix do not match the candidate. The exact CA bytes are now available locally; that resolves the missing-input blocker, not the host qualification gate. Use the current source-bound inventory to record actual differences; do not treat this prior environment description as a fresh observation. No host-policy change or repeated incompatible proof attempt is part of this handoff. Saved Codex environment setup publication also remains separate; repository changes must be installed through supported saved-environment settings and verified in a fresh task before persistent readiness can be claimed.
