# Qualification-host preflight

**Status: planning only; host readiness, qualification and freeze are unproved.** This checklist was prepared from repository source. No qualification host was contacted, inventoried, configured or operated. All observations below remain pending. It grants no permission to run commands or alter an owner's host.

[Validation](validation.md) owns candidates and evidence; [architecture](architecture.md) owns safeguards; [delivery](delivery-plan.md) owns gates. This guide supplements the source-verified command interfaces in [the existing proof handoff](phase1-proof-handoff.md); it is not a new contract or successful proof result. Cloud conformance admits reviewed provisional development only. Real Phase 1 qualification/freeze precedes Phase 6 cutover; VAL-001 additionally requires installed-binary acceptance and final reviews.

## Candidate and input inventory

Record observed identities privately; matching version strings or lock files do not prove readiness.

| Item | Required candidate or check |
|---|---|
| Host | Fedora Linux **44**, kernel **`7.1.10-200.fc44.x86_64`**, **x86_64**, **btrfs** durable proof/test bases. This is the sole closed Phase 1 implementation candidate, currently unqualified. Overlayfs/tmpfs or another platform tuple cannot substitute. Required private capped tmpfs mounts remain separate. |
| Locked source | nixpkgs **`b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`**; compare existing `flake.lock`, reviewed provenance and realized binaries. A source/NAR pin is not an executable checksum. |
| Managed binaries | Git **2.55.0**, Bash **5.3.15**, mise **2026.7.17**, coreutils **9.11**. Authenticate each actual canonical executable and build/loader provenance, then record its complete identity/version. Do not substitute wrappers or ambient tools. |
| chezmoi | **`v2.70.5-zerb.1`**, public fork **`ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`**, upstream **v2.70.5**, reproducible **`CGO_ENABLED=0`** build. Verify patch scope, build inputs and actual executable separately; a submodule or build declaration is not a built artifact. |
| Go and supporting tools | Go **1.24**, with exact compiler patch/identity/build settings recorded. The owner has not frozen either historic 1.24.6 or local 1.24.9 as the universal qualification patch. Record actual Python 3, `cc`, configured golangci-lint, tracer and namespace/mapping-helper identities. Do not invent missing version pins. |
| Tracing | An authenticated absolute strace-compatible executable and complete closed event coverage. Git uses `-ff -qq -v -yy -s 4096 -e trace=all`; adapters use `-f -qq -v -yy -s 4096 -e trace=all`. Unknown/missing events fail; source inspection does not prove ptrace capability. |
| Parser | Exact approved **`github.com/pelletier/go-toml/v2@v2.4.3-zerb.1`** local variant. [Canonical source/pin/retirement instructions](../../third_party/README-go-toml.md) and [provenance](../../third_party/go-toml.provenance.json) own all hashes. Ordinary `go mod verify` does not authenticate local replacements. |
| Fixed CA | Exactly **472,033 bytes**, SHA256 **`3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a`**. Verify complete bytes before parsing and use the [reviewed provenance](../../internal/proof/adapters/testdata/reviewed-ca-bundle.PROVENANCE.md). No ambient/system trust or generated replacement. Child materialization/identity remains an actual proof requirement. |
| Node lock/artifact | Exact authored **node@24.4.1**, **`core:node`**, unchanged [proof lock](../../internal/proof/adapters/testdata/mise.lock). Verify its complete public URL/checksum/size and acquired archive against that fixture. No live lock generation, channels or post-trust resolution. |

- [ ] Record exact source commit plus tracked/generated tree before build and execution; source changes require affected checks/reviews again.
- [ ] Authenticate existing executable bytes/provenance before invoking candidate binaries, including version queries. Inspect `flake.nix` before any shell entry: its shell hook is not inventory or permission for setup work.
- [ ] If cached dependencies or the prepared parser tree are absent/altered, stop. Offline `--prepare` writes ignored repository input and belongs in an approved disposable checkout before source capture; it never repairs an altered tree. Downloads or locked Nix builds need their own approved acquisition/build scope.

## Action boundaries for a future host run

| Action class | Reviewable scope; separate from this planning task |
|---|---|
| Read-only inspection | Existing source/provenance, platform/filesystem metadata, account identity, cgroup delegation files and configured namespace/tracing restrictions. Observing configuration does not exercise facilities. |
| Inventory collector | Existing `--inventory-only` executes tool-version/Git queries, creates child HOME/XDG/cache scratch under explicit `/tmp/zerb-cloud-*`, and writes a new private report. It is **not literally write-free**, does not probe runtime facilities, and does not verify every future proof base or user delegation. Approve its exact scratch/report scope first. |
| Disposable offline checks | Existing source builds/tests, non-secret fixtures, CA/lock copies and before/after sentinels, confined to approved disposable roots and caches. Inspect test behavior; a test command may include child signals or resource limits. |
| Network/upstream execution | Exact public Git/Node endpoints, authenticated binaries, limits, output roots and manager invocations. Module downloads and locked Nix builds are acquisition/execution, not metadata inventory. |
| Namespace/cgroup/mount/UID probes | Private namespace creation, subordinate mappings, cgroup creation/limits/kill, temporary proc/tmpfs/bind mounts and read-only remounts, mapped-UID attacks, PTY/job control and deliberate child death. Approve the actual confined read/write/mount/kill scope before execution, even when unprivileged. |
| Administration/storage disruption | Provisioning a disposable machine/disk, arranging safe owned prefixes/delegation/subordinate IDs, or real storage-error/power-loss experiments. Require a concrete separate plan/approval. Never format a live disk, alter the owner's live workstation, weaken security policy or grant broad privileges to make a row pass. |

Missing permission or facility is a blocker. Do not opportunistically install tools, enter setup hooks, enable controllers, add UID ranges, mount filesystems, change credentials or alter host/saved-environment/CI settings during preflight.

## Disposable topology and protected capture

- [ ] Select an owner-approved disposable qualification host/account/storage area, disjoint from the live workstation. Before poisoning, capture real passwd HOME, actual parent HOME/XDG and repository identities using the existing protected-capture contract. Proof commands capture the real parent environment and construct isolated child environments themselves; do not spoof parent HOME to hide a protected path.
- [ ] Set distinct, canonical btrfs proof bases for state, Git, schema and adapters, with evidence beneath the appropriate base as required by each existing CLI. Reject overlap with repository, real HOME/XDG, unrelated caches and other protected paths. Capture outside sentinels and before/after object identities.
- [ ] Ordinary build/tests instead use explicit disposable HOME/XDG/TMP and build/module caches. Scope the parser verifier to that environment too: it invokes `go list -m`, not just file reads. Restore the real parent environment before collector/proof protected capture. Do not carry a cloud-only `-buildvcs=false` exception onto the qualification host; record source identity explicitly.
- [ ] Supply an **already provisioned** TMPDIR whose entire absolute non-symlink chain has no group/world write bits: UID-0 structural prefix, then the proof-user/supplied-owner subtree, with no owner switch back. A sticky `/tmp` or container structural UID 65534 grants no exception. Creating/chowning a suitable prefix is separately scoped disposable-host administration, not a test fix.
- [ ] Run and record each exported positive below. A skip, private helper success or synthetic identity mismatch does not close the corresponding public path:

| Exported behavior | Exact test |
|---|---|
| Root/directory lifetime | `TestPublicPositiveOnAdmissibleTopology` |
| Lock lifetime | `TestLockPublicPositiveOnAdmissibleTopology` |
| Bounded listing/close | `TestNamesPositiveAndConcurrentCloseOnAdmissibleTopology` |
| Pinned reading/close | `TestReadFilePositiveAndCloseOnAdmissibleTopology` |
| Directory sync/close | `TestSyncPositiveAndConcurrentCloseOnAdmissibleTopology` |
| No-replace publication/close | `TestPublicationPositiveAndConcurrentCloseOnAdmissibleTopology` |
| Staged replacement lifetime | `TestReplacementInvalidAndPublicBoundary` |
| Protected-role reservations/topology | `TestPublicRolesOnAdmissibleTopology` |
| Retained file/exclusive directory lifetime | `TestPublicFileAndCreateDirOnAdmissibleTopology` |

- [ ] Run the bounded private state integration family with `go test -race -count=1 -run '^TestFreshInitialize' ./internal/state` in the disposable environment. Record every intent, mode refusal, existing-evidence, boundary-failure, cancellation, contention and substitution subcase separately. The Cloud run skips these filesystem cases before product mutation; pure graph tests and successful current-process metadata capture do not substitute for them. State roots that cannot satisfy the existing replacement-witness parent requirement of exact `0700` refuse before writes; they are not chmodded or reclassified.

- [ ] Review the current mise harness write set: [mise_namespace.go](../../internal/proof/adapters/mise_namespace.go) uses `os.MkdirTemp("/tmp", "zerb-mise-private-")`, applies `0711`, then mounts its private workspace there. TMPDIR does not redirect this write. Approve that exact disposable mountpoint/write/cleanup scope, or stop for source/contract review. It does not relax safefs' writable-parent policy or mean every write is below ADAPTER_BASE.

## Facilities to prove after scoped approval

- [ ] Observe an existing delegated cgroup-v2 subtree for the proof UID, stable descriptor-relative control access and required memory/pids/cpu controllers. Then separately exercise the exact boundary: `memory.max=805306368`, zero swap, `pids.max=96`, one-core quota; zero OOM/OOM-kill, exact `cgroup.kill`/empty proof and all-child reaping. Root controller listings alone prove none of this. Never enable delegation/controllers from a test.
- [ ] Prove the required private user/mount/PID namespaces and exact UID/GID mappings with existing helpers. Isolated bind aliases, foreign-UID fixtures, restricted proc and namespace-local PID-reuse controls need explicit confined scope; never change host-global PID/security controls.
- [ ] Prove the mandatory authenticated same-binary descriptor-only supervisor, sealed exact ELF execution, parent-death synchronization, descriptor allowlisting/churn, signal forwarding, cancellation/hard-death cleanup and observed zero guard exit. No fallback runner or hidden public argv mode.
- [ ] Prove x86_64 seccomp denial of `io_uring_setup/register/enter` for Git/acquisition/mise/chezmoi; do not extend that manager policy to arbitrary exec/shell payloads.
- [ ] Prove capped private tmpfs size/inode limits, immutable authenticated archive-to-mise input and exact distinct mapped manager UID. Mount/remount/cleanup and full installed-tree verification are actual observations, not inventory facts.
- [ ] Prove the exact Bash `--noprofile --norc -i` PTY controlling-terminal, foreground/background job groups and containment; no normal startup files or parent-shell mutation.
- [ ] Prove full tracing, marker non-access/non-write, selected-gitlink/private/provider/host-fact exclusion and zero access for unselected gitlinks. Raw traces are private diagnostics, not automatically exportable evidence.

## Execution proposal and evidence gate

After approval of a concrete host run, use the existing [handoff command interfaces](phase1-proof-handoff.md#disposable-layout-and-invocation), checked against current `internal/proof/cmd/*/main.go`; do not invoke internal worker/supervisor modes manually. All adapter executable/CA flags remain required even for lifecycle-only runs; adapters also require the exact authored lock. Review each command's write/endpoint/process scope before running it.

A later inventory proposal may use the existing interface below only with an approved new report destination and scratch scope. The collector selects Go through **PATH**; its measured canonical executable must equal the selected authenticated Go, not merely an unused `$GO` variable. Leave real parent HOME/XDG intact for protected capture.

```sh
python3 -B scripts/cloud_conformance.py --inventory-only --output "$REPORT"
```

- [ ] Inventory-only must retain `host_qualified=false` and `release_supported=false`; it cannot admit development or qualify a host. Complete cloud conformance is a separate source-check run and still cannot qualify the frozen-host candidate.
- [ ] Record source/tool/CA/fixture identity, command/environment/descriptor shape, exact row/denominator, process/access/write/resource results and cleanup status under VAL-003/004/005. Use typed redacted artifact references only after artifacts actually exist. A command's zero exit status is insufficient.
- [ ] Exercise every required initialization, ownership transfer, mutation/recovery and cleanup crash prefix, including recovery itself, all PC-016 cells, exact reapply and operation identity. Do not treat the new metadata shape recognizer as graph authority or an implemented recovery service.
- [ ] Stop dependent rows on a wrong identity, unknown trace event, missing facility/limit, unapproved endpoint/write, survivor/guard failure, OOM, skip, marker spill, outside-sentinel change or ambiguous publication. Preserve evidence and report failed/inconclusive/not-exercised accurately; no repeated incompatible proof or weakened protection.
- [ ] Scan controlled sinks before checked cleanup of exact owned fixtures/processes/cgroups/mountpoints. Never use generic recursive deletion on real HOME, repository, unrelated cache, stable locks, unknown residue or dirty state. Cleanup failures remain failures.
- [ ] Real storage-error/power-loss durability and installed Phase 6 verticals require separately approved disposable experiment/acceptance plans. Process SIGKILL is not power-loss evidence.
- [ ] Complete real positive and dependent negative rows, independent plan/security review, all finding-driven reruns and owner freeze approval before qualification. Host readiness remains unproved until actual verified observations exist.

## Protected-root and initialization status

The owner approved the explicit parent/peer matrix, read-only missing-root reservations with explicit successful-creation binding, and bounded descriptor-correlated kernel topology. Those contracts and their provisional implementation are now present. ARCH-017/020/024/030 additionally own the approved retained-file and exclusive-directory capabilities, fixed current-process observation and bounded private CLEAN-to-READY producer. None of these source changes establishes executed public-path, mount/bind, initialization/handoff or storage qualification. Mountinfo alone remains insufficient; follow the complete owning contracts and record actual results. This checklist grants no host, namespace, mount, repair, takeover or cleanup authority.
