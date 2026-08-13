# Delivery Plan

## Delivery strategy

ZERB is a pre-alpha hard cutover. There are no supported installations or data migrations, so the replacement must not ship compatibility aliases, an old parser/importer, a feature flag, a second public binary, state migration utilities, or mixed command routing. Replacement packages may coexist with untouched old packages only on `project_refocus_and_security_fixes` while the replacement is built; no intermediate phase merges to `main`, and the public entrypoint remains the old pre-alpha program until the single Phase 6 cutover.

[Product contract](product-contract.md) owns public behavior. [Architecture](architecture.md) owns trust, process, protected-root, and durable-state invariants. [Validation](validation.md) owns evidence and support claims. [Hard cutover](hard-cutover.md) owns artifact disposition. [Post-MVP](post-mvp.md) owns non-committed intent. This document owns only sequencing and phase admission.

Phase 0 contracts are provisional and authorize proof work only. Phase 1 evidence is the sole gate that freezes implementation contracts and implementation-qualified candidates; it creates no support claim. No product implementation begins before that gate closes, and support promotion waits for VAL-001 plus Phase 6 installed-binary verticals.

## Phase 0 — Authority migration and exhaustive disposition

### Purpose

Establish a credential-free roadmap as the only durable project authority, record the old branch truth without completing it, and bound disposable proof work.

### Work

1. Work only on `project_refocus_and_security_fixes`; retain history and never merge an intermediate phase to `main`.
2. Record the baseline output of branch status, `go test ./...`, `go test -race ./...`, `go vet ./...`, configured lint, a build of `cmd/zerb`, installed version output, and root help. Failures are facts and do not weaken later gates.
3. Complete the tracked-artifact table in [Hard cutover](hard-cutover.md): every production source, test, fixture, example, document, command prompt, build/CI file, direct dependency, generated artifact, and ignored runtime artifact is classified as `delete`, `replace`, or `audit then reimplement`, with owner and proof reference.
4. Record active change ID `add-config-remove-command` as `superseded/incomplete; delta never adopted`. Do not complete its CLI, service, transaction, resume/abort, signal, display, test, or documentation work. Leave reachable old code unchanged until Phase 6.
5. Establish stable authorities and IDs: `PC-*` in the product contract, `ARCH-*` in architecture, and `VAL-*` in validation. Add the evidence-only proof-results ledger and the target-compatible post-MVP ledger. No decision remains delegated to a removed file or process.
6. Define the provisional public CLI/schema/output/trust contracts in the product contract and the provisional trust/process/root/state contracts in architecture. Define all proof rows and conditional support rules in validation.
7. Crosswalk every surviving target requirement into its normative owner. Preserve target-compatible safety facts only: no-follow validation, restrictive creation modes, durable atomic persistence, causal error wrapping and active redaction, truthful forward recovery, and useful health observations.
8. Remove the former specification tree, obsolete drift/restore documentation, and obsolete workflow prompts only after the crosswalk and disposition are complete. Git history remains the historical record; do not relocate old records into the working tree.
9. Rewrite branch-level README and contributor guidance to state that the reachable CLI is still obsolete pre-alpha behavior, point to roadmap requirement IDs, and prohibit installation on a real workstation before Phase 6.
10. Sweep the whole working tree case-insensitively for the removed workflow name; the result must be zero. Separately sweep obsolete product names and claims. During Phase 0 they may remain only in reachable old implementation and generic disposition prose, never current documentation or workflow tooling.

### Exit gate

- The baseline and exhaustive disposition exist, including generated and ignored runtime artifacts.
- Every target requirement has one roadmap owner and stable ID; proof results contain evidence only and no normative decision.
- The active incomplete change is recorded as superseded without being finished.
- Current documentation contains no delegated source of truth or removed workflow/tool reference.
- The old public command table is unchanged and no second temporary product exists.
- Only Phase 1 disposable proof harnesses are authorized; product implementation remains barred.

## Phase 1 — Four proof slices and contract freeze

### Purpose and harness rules

Prove external and durability contracts before product code depends on them. The finite proof universe is the closed non-support matrix in `validation.md`; Phase 1 may mark a passing positive row only `implementation-qualified` and a passing negative row only `exclusion-proven`. Neither is support. Every experiment uses disposable `HOME`, XDG data/cache/state roots, destination, generated-source, manager state, evidence, and outside-sentinel paths that cannot address the developer's real home, credentials, manager state, or destinations. Authentication/provider markers, credential stores, helpers, agents, sockets, relays, and private endpoint fixtures are not positive inputs; exclusion proofs show they are never accepted or accessed.

Use the exact Fedora/kernel/x86_64/btrfs host candidate and Git 2.55.0, Bash 5.3.9, mise 2026.7.17, chezmoi 2.70.5, and CA-bundle candidates declared in validation from nixpkgs revision `b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`; use exact public Git fixtures, manifests containing unselected gitlinks, selected-gitlink rejection fixtures, generated rendered/ambient markers, process-tree/argv/environment/descriptor/network capture, access tracing or a deny namespace, and a write allowlist. No private transport or credential mechanism is a positive candidate.

Each proof result records the closed-matrix row, requirement IDs, exact upstream version and canonical executable identity, reviewed public source or typed static-locator reference, fixture/revision/tree/candidate identity and equality/change relations, invocation/environment shape, process tree, access/write/resource observations, result, rejected assumption, and contract edit. Selected-gitlink/private-transport rows record exclusion and zero access, never a tuple, credential recipe, authorization digest, or identity producer.

### Slice 1 — Native Git and source grammar

Prove all of the following as one public-source and exclusion contract:

- Resolve the `P1-PUBLIC-*` and `P1-GIT-MATERIALIZE` Git 2.55.0 candidate identity once and use argument arrays and the ARCH-006 environment policy.
- Enforce PC-020 canonical public HTTPS grammar byte-for-byte, disable redirects/proxy/authentication, and reject credentials, local paths, `file:`, scp-like syntax, unsafe protocols, ambiguous encodings, hostile inherited Git/SSH configuration, and endpoint rewrites.
- Fetch into a fresh owner-validated staging root under every frozen byte/object/depth/output/time/CPU/memory/process ceiling enforced outside native Git; prove boundaries, hostile packs, interruption, and confined cleanup.
- Fetch a public superproject without recursive submodules, worktree checkout, hooks, filters, helpers, or candidate execution.
- Parse and statically validate the manifest, `.gitmodules`, unselected canonical HTTPS/SSH gitlink locators, object types, and paths without access to those gitlinks.
- Materialize only the embedded base and embedded selected object trees through Git plumbing; reject nested gitlinks and unsupported objects.
- Select every canonical and invalid gitlink shape and prove deterministic rejection before authentication, network, candidate creation, manager execution, or mutation.
- Exercise every helper/askpass/agent/socket/identity-file/known-hosts/credential-store/proxy/synthetic-config/relay/private-endpoint/typed-identity/transport-authorization input shape and prove it cannot create a mechanism or alter rejection.
- Prove unselected gitlinks receive zero executable, helper, agent, relay, authentication, DNS/endpoint, or network attempt.
- Prove every public embedded selected object is fetched before host mutation and distinguish public object identity from availability. Retained evidence uses typed source/tree/static-locator references rather than sensitive digest oracles.

### Slice 2 — Managed adapters, process lifecycle, exec, and shell

Prove verified acquisition, isolation, and lifecycle for the exact `P1-MISE-CORE-NODE`, `P1-CHEZMOI-STATIC`, `P1-EXEC`, and `P1-SHELL-BASH` candidates:

- Verify source, public-artifact SHA256/signature/provenance, safe extraction, absolute executable identity, and poisoned-cache behavior.
- Freeze every managed path/config/cache/data/state/install/discovery control; plant hostile user/system/global/project manager state and prove no read, execution, write, or fallback.
- Prove deterministic `base + selected` composition and only the closed candidate surfaces. Exact authored `node` version `24.4.1` and its complete authored lock enter the candidate before trust; aliases, ranges, channels, `latest`, missing/generated locks, and post-trust digest mutation reject before other manager execution. The sole pre-trust adapter invocation is verified chezmoi's read-only, non-rendering, machine-readable target mapping over the protected effective source: prove exact executable/argv/environment/descriptors/roots, a synthetic non-real destination, deterministic structural target inventory, zero template/helper/provider/host-fact/script/external/candidate-content execution, and zero real-destination access/write. Bind that inventory into the candidate. If upstream cannot prove it, fail/remove the surface without a fallback mapper. Rendering starts after trust; real-destination access and live observations start after mutation authorization under `A-X+W-X`.
- Invoke the VAL-038 disposable static validator directly, without `internal/app.Run`: directly supplied embedded provider/host-fact input rejects before candidate creation; selected-gitlink input rejects earlier under VAL-009 before authentication, network, materialization, or validator entry.
- Prove `exec` resolves a bare name only against the already-constructed stable profile `PATH`, rejects empty/relative entries and slash-containing relative paths, identity-checks explicit absolute paths, leaves the parent environment unchanged, and returns every normal child status `0` through `255`; prove explicit `shell --shell <id>` resolution without `$SHELL` or normal interactive startup files.
- For native Git, acquisition, mise, chezmoi, `exec`, and `shell`, freeze and exercise the complete environment and descriptor matrix under poisoned-parent values.
- Freeze the atomic spawn protocol: close-on-exec for every parent-created descriptor; child-side close-range/allowlist semantics safe against concurrent opens; parent/child synchronization until descriptor, group/session, and parent-death setup completes; and parent-identity recheck before release. Exercise concurrent open/dup/close churn plus hard death before and after clone, descriptor sanitation, group/session setup, parent-death setup, exec, and first descendant fork for every process class.
- For every process class, prove the candidate Linux group/session setup, direct-child parent-death mechanism, catchable signal forwarding, context-cancellation grace/escalation, leader and descendant reaping, orphan detection, and wrapper/child status mapping. Exercise child signals and wrapper SIGINT/SIGTERM.
- Prove no managed descendant survives catchable cancellation or wrapper hard death. Every retained process class MUST use the Phase 1-proven same-binary FD-only supervisor; add the frozen cgroup boundary when that class needs it. These are cumulative requirements, never alternative primitives or optional escalation. Approved managed versions must not daemonize, call `setsid`, or escape the boundary. Do not accept leader-only cancellation or `exec.CommandContext` as lifecycle evidence. Deliberately detached arbitrary user payloads are outside orchestration only if the product contract states the limit and evidence proves no ZERB manager credentials or state paths reach them.

### Slice 3 — Filesystem, root initialization, durable state, and recovery

Prove all protected filesystem and transition primitives in a disposable harness:

- Prove umask-independent `0700`/`0600` creation; no-follow owner/type/mode/link/ancestor checks; same-filesystem staging; atomic publication; every file and parent-directory sync; and cleanup confined to validated owned staging.
- Exercise the complete ARCH-018 classifier, not marker-pair shorthand: all four clean absent/empty pairs; arbitrary foreign entries with no recognizable evidence; every reserved synchronization/path name alone; unsupported schema/domain records; every absent/malformed/unsafe/duplicated/aliased/mismatched/unreferenced relation; every unrecognized extra entry beside otherwise current evidence; independently valid unreferenced candidates; orphan receipts/unrelated journals/second current; each supported state graph; exact cleanup residue; and unrelated/missing/poisoned cache. Freeze one classification, PC-016 admission/lease, write set, and guidance result per fixture.
- Use the stable never-replaced `init.lock` inode for initialization/takeover. Prove one of simultaneous initializers writes the complete claim, stable admission/writer inodes, released owner generation, markers, and initialized publication; every loser writes nothing. At every create/write/file-sync/rename/parent-sync/reopen boundary, kill the holder and recover only after same-inode acquisition, full process-death proof, record-inode/instance/token/status compare-transfer, durable fresh token publication, and reopen verification. Race two reclaimers, PID reuse, token/inode/topology ABA, and successor death.
- Prove matching state/data ownership, stable synchronization inodes, writer-owner publication, cross-root/schema relation, recognized initializing/ready/stable/transition/cleanup state, foreign content, and cache behavior follow the exhaustive classifier without adoption or destructive guidance.
- Authenticate and fully hash managed archives before parsing; reject traversal, absolute paths, aliases, duplicates, links, devices, unsupported modes/types, oversize/depth/count inputs, modified-after-hash cache data, and outside-sentinel writes.
- Prove ARCH-031 shared/exclusive admission and writer exclusion on the candidate filesystem. Exercise fixed lock order, compatible readers, mutation/recovery exclusion, atomic fully-written owner acquire/release publications, strictly advancing generations, fresh tokens, full process identity, two stale reclaimers, malformed lock/owner records, record and lock-inode ABA/substitution, authority revalidation before every durable/external boundary, and every sync/crash point. Stable synchronization and owner paths are never unlinked.
- Run every PC-016 command/state cell and precedence. Prove every plan and profile-list operation uses `A-X`, doctor/exec/shell use `A-S`, and mutation/recovery use `A-X+W-X`. With valid stable locks/owner predecessor, exact residue repair uses `A-X+W-X`; malformed/substituted synchronization or unprovable owner evidence permits only safe-`I-X` no-write diagnosis, or topology-only diagnosis when `init.lock` is unsafe. Race source/candidate staging and exec/shell state validation, identity binding, spawn, child release, and status against every incompatible operation. An incompatible non-blocking loser returns busy `1` before semantic validation; a later caller reclassifies after release. No timeout, shared-lease upgrade, visible partial publication, or dropped lease is permitted.
- Prove destination ancestor/target no-follow revalidation, unowned and user-modified target refusal, content/type/link/rename races, and same-filesystem mutation staging without claiming cooperative locks exclude arbitrary writers.
- Prove candidate/journal durability, writer generation/token binding, `prepared`, every `mutating` phase, pending receipt, atomic stable/current commit, exact stale cleanup, orphan receipt handling, and all recognized restart states.
- Inject failure, cancellation, and hard termination at every lock-publication, lease, persistence, external-mutation, and release boundary, then run the directly admitted `doctor`, `recover resume`, or `recover repair` path without manual state edits.

The disposable spike is evidence for the frozen contract, not production state-machine code.

### Slice 4 — Schema and parser

Source-verify and pin `github.com/pelletier/go-toml/v2@v2.4.3` and its module checksum. Exercise the exact PC-040 and PC-041 structs only through an input-size bound followed by `toml.NewDecoder(...).DisallowUnknownFields().Decode(&target)`.

Prove rejection and deterministic diagnostics for schema mismatch, unknown fields and nested tables, duplicate keys/tables/IDs, dotted-key aliases, malformed input, invalid UTF-8 where prohibited, multiple semantic interpretations, invalid or aliased paths, gitlink mismatches, and every byte/count/string/path/depth bound. Exercise the provisional 1 MiB document and 256-profile limits, freeze every remaining field limit, and retain fuzz/property fixtures. If v2.4.3 admits ambiguity or bypasses a bound, reject the version or narrow the grammar before freeze.

### Phase 1 exit gate and freeze

Before Phase 2, every applicable closed-matrix row has a result and a Phase 1 disposition, every rejected assumption has changed the normative owner, and at least `P1-PUBLIC-EMBEDDED` plus every foundation candidate it needs is `implementation-qualified`. Every negative row required by a retained positive candidate is `exclusion-proven`; a failed or inconclusive required negative row blocks the dependent candidate until the normative owner narrows/removes the surface, updates the matrix, and all affected proofs pass. The disposable harness includes VAL-035 approval separation/precedence and VAL-036 all-plan `A-X` lifetime, cache-only pre-trust behavior, immutable-publication races, and changed-candidate rejection. Record owner approval, independent plan review, and focused security review. Freeze all of:

1. The implementation-qualified Fedora Linux 44 / kernel `7.1.5-201.fc44.x86_64` / `x86_64` / btrfs proof candidate; explicitly record that it remains unsupported.
2. Git 2.55.0, Bash 5.3.9, mise 2026.7.17, chezmoi 2.70.5, `cacert`, parser, and fixture identities; their nixpkgs revision or upstream origin; canonical paths; public-artifact digests; checksums; and provenance checks.
3. Public source grammar, unselected canonical HTTPS/SSH gitlink static grammar and zero access, `P1-SELECTED-GITLINK-EXCLUSION`, absence of every credential/helper/agent/relay/private-endpoint/typed-identity/transport-authorization mechanism, and every public source ceiling.
4. Exact schema-1 fields and every byte/entry/string/path/depth/count/allocation limit; `go-toml/v2` version/checksum/strict behavior/error mapping and corpus.
5. Exact CLI/output/error/approval behavior; stable-PATH execution; child/wrapper/cancellation behavior; and the Bash-only shell candidate.
6. Exact source/profile/candidate/tree/policy/receipt digest inputs and canonical encoding, plus the rule that retained sensitive digests become fresh typed references and relations rather than digest oracles.
7. Only `P1-MISE-CORE-NODE` and `P1-CHEZMOI-STATIC`, including exact authored versions/locks before trust, every negative surface, and non-orthogonal combinations.
8. For every candidate process class: environment/descriptors; atomic spawn; group/session; forwarding; grace/escalation; parent death; reaping; orphan and hard-death behavior; and deliberate-user-detachment limit.
9. ARCH-016/017 protected-root layout and initialization; ARCH-018's exhaustive classifier and extra-entry rules; cache, filesystem/archive, and destination-race policy.
10. ARCH-023/024/025/027/031 owner schemas and relations; every initialization/writer publication and sync boundary; ABA-safe takeover; prepared/mutating/dirty/stable/cleanup truth; the complete PC-016 command/lease table; reconciliation; and recovery.
11. ARCH-030's exact replacement package set, forbidden-import seam, one-way import DAG, frozen internal APIs/service method sets, and permanent proof fixtures.
12. The closed marker/redaction policy and VAL-038 source-specific provider/host-fact exclusion in the disposable validator, without `internal/app.Run` or installed-binary evidence.

Product implementation cannot start while a required positive candidate is failed, inconclusive, unfrozen, awaiting review, or depends on a negative row that is not `exclusion-proven`. Phase 1 may remove or narrow a candidate and must update/rerun the dependent matrix, but it cannot call one supported; VAL-001 and Phase 6 own promotion.

## Phase 2 — Replacement foundations

### Purpose

Build protected source, execution, storage, and state foundations without importing old product contracts or mutating real destinations.

### Work

1. Implement the exact ARCH-030 replacement package set, one-way import DAG, forbidden old-package seam, and frozen internal APIs. The DAG gate scans production and `_test.go`, rejects imports of every full old package tree including `internal/testutil`, and permits `internal/testenv` only from `_test.go`. No bridge, alias, re-export, global registry, fallback service, production import of `testenv`, or old CLI route is permitted.
2. Implement immutable copied `paths.Input` and fixed XDG data/cache/state resolution through ARCH-030's `paths` API, with no product-specific override or ZERB config root. Service constructors are side-effect-free; they perform no root, filesystem, environment, process, network, or durable-state work until `app.Run` supplies its single resolved `paths.Roots`. Implement the exact ARCH-016/017 initialized layout before other persistence.
3. Implement no-follow protected filesystem and archive primitives, authenticated acquisition/extraction, native Git object storage/materialization, strict schemas/digests, managed adapters, and value-free durable record types behind their ARCH-030 owners.
4. Implement the ARCH-030 `execenv` seam with absolute executables, argument arrays, complete frozen environments, descriptor closure, no shell, and the mandatory same-binary FD-only supervisor for every retained class, plus the frozen cgroup boundary when required; neither is replaceable by `exec.CommandContext` or another primitive.
5. Implement ARCH-031 admission/writer synchronization and owner publication before state transitions. Persist candidate `B` and its writer-bound journal before mutation; sync `prepared`; sync each exact `mutating` phase; sync pending receipt; atomically publish/sync current as stable; clean exact residue; publish and verify the released writer-owner generation; then unlock writer and admission without unlinking either stable lock inode.
6. Retain the strict parser conformance/fuzz corpus and focused crash/ABA/TOCTOU failure injection from Phase 1 so package/API implementation cannot widen the frozen contracts.
7. Preserve redaction at every error and diagnostic boundary and keep candidate/state/receipt types explicit, versioned, map-free, sorted, and value-free.

### Exit gate

Focused evidence covers the ARCH-030 import/API seam, path/archive safety, strict parser behavior, canonical digests, native Git policy, complete process environments/descriptors/lifecycle, exhaustive root classification, initialization/writer publication and ABA takeover, every PC-016 compatible/incompatible lease race, candidate publication and read-transition-launch linearization, every state transition and persistence boundary, receipt completion, and recovery. A dedicated security review of native Git, environment construction, descriptors, acquisition/extraction, roots, locks, admission, state, digests, and receipt completion is resolved and affected checks rerun before destination or tool mutation integration.

## Phase 3 — Non-public target runner and structural commands

### Purpose

Build the complete decision path behind one injectable runner while preserving the old public entrypoint until Phase 6.

### Work

1. Wire the non-public dispatcher through the exact architecture-owned `internal/app.Run` and `app.Options` seam in ARCH-030: production captures home/raw environment once into immutable `paths.Input`; options contain only that input and side-effect-free constructors, not resolved roots or active root services. Delivery does not redefine the API.
2. Implement and exercise through `app.Run`: `profile list`, public/static `plan`, `doctor`, `version`, and `--version` under PC-010 through PC-016. `app.Run` parses and completes root-independent local checks first; invalid argv and either version form return without root resolution or constructor activation. A well-formed non-version command resolves paths exactly once and only then activates root services from that one value.
3. `profile list` performs only public unauthenticated static inspection and sorted-ID output.
4. After the one root resolution and root-service activation, every `plan` acquires and revalidates `A-X` before root-state classification beyond minimum lease topology, source access, adapter acquisition, staging, or candidate validation, then holds it through cache staging, candidate trust, every permitted immutable source/tree/toolchain/candidate publication, synchronization, reopen verification, and cleanup. There is no same-candidate shared path, upgrade, or drop/reacquire. In `STABLE(A)`, a recomputed candidate other than exact `A` fails as an unsupported update before publication; transition and cleanup state deny planning before source access. Under that admission, planning acquires verified adapters, performs constrained source inspection and the sole proven pre-trust chezmoi structural mapping, and computes a candidate containing exact authored versions/locks and deterministic structural target inventory. It obtains digest-bound trust before rendering or any other candidate-selected manager execution. Live destination observations begin only after mutation authorization under the mutation lease and enter journals/receipts, never the candidate digest.
5. Run the Phase 3 `internal/app.Run` clause of VAL-038: the injected embedded fixture rejects forbidden provider/host-fact syntax before candidate creation; selected-gitlink-shaped input rejects at source selection before authentication/network/materialization. Revalidate forged forbidden candidates before mutation.
6. `doctor` reports root/schema/state/tool/source readiness without adopting, deleting, relabeling, or treating availability as identity. Selected gitlinks and private sources remain rejected through every later phase. Keep the existing `cmd/zerb` routing unchanged; do not add a second binary, build tag, or mixed route.

### Exit gate

The exact runner handles public embedded static fixtures; invalid argv and both version forms perform no root resolution or root-service activation; each well-formed non-version command resolves the immutable captured input exactly once. Stable structural input produces the same candidate digest regardless of mutable/live destination state; trust mismatch and missing noninteractive approval fail before the corresponding boundary; candidates are owner-only and value-free; and existing manager state or real destinations are not accessed or mutated. The public binary still exposes only its pre-cutover commands.

## Phase 4 — Public bootstrap, exact apply, recovery, exec, and shell

### Purpose

Complete one public static vertical slice and every truthful mutation/recovery boundary, still through `app.Run` rather than the public entrypoint.

### Mutation order

1. Acquire the exclusive ARCH-031 admission lease, acquire the stable writer exclusion, atomically publish and sync the new writer-owner generation/token/process identity, reopen/revalidate it, and retain the complete `A-X+W-X` relation.
2. Under that unchanged ownership, load the exact named candidate and revalidate source objects, policy digest, and trust; only after mutation authorization, observe real-destination topology/type/content/link/mode/owner plus access/disk/tool preconditions and persist them in the writer-bound journal.
3. Persist and sync the writer-bound `prepared(A,B)` journal containing those live destination preconditions; none alters candidate `B` or its digest.
4. Revalidate ownership and affected live preconditions; persist and sync `mutating(A,B,tool-install)` immediately before possible tool mutation.
5. Install additively and verify exact observed tool versions, revalidating ownership and affected preconditions around the external boundary.
6. Revalidate ownership and affected live destination preconditions; persist and sync `mutating(A,B,home-apply)` immediately before possible render/home mutation.
7. Render/apply one effective source and verify target ownership, content, type, mode, link, and ancestor topology, revalidating ownership and live preconditions at every affected mutation boundary.
8. Persist and sync `mutating(A,B,shell-metadata)` before generating stable command/shell metadata.
9. Generate and verify shell metadata under unchanged ownership.
10. Write, sync, reopen, and verify pending receipt `B`.
11. Revalidate ownership; atomically replace and durably sync `current.json` as `stable(B)`; this alone completes receipt `B`.
12. Remove and parent-sync only the exact stale journal data. Interruption here remains `CLEANUP(B)`.
13. Atomically publish and sync the released writer-owner generation, reopen and verify that released generation, then unlock writer and admission in order. Never unlink either stable lock inode.

### Work and restrictions

- Implement standalone exact-digest `apply`; orchestration-only `bootstrap` must call the same inspect, trust, plan, candidate, authorization, mutation, verification, receipt, and stable-commit path and must take `A-X+W-X` before planning rather than upgrade a shared lease.
- Implement every PC-016 state/admission row exactly, including same versus replacing plan, exact resume/repair, stable cleanup, parse/version precedence, and denial before missing-approval handling. Recovery revalidates and converges forward only.
- Implement `exec` and explicit-ID `shell` on the Phase 1 lifecycle, retaining `A-S` through stable identity/environment binding, same-binary FD-only supervisor release/reaping, the required cgroup boundary, and status. The supervisor is mandatory for every retained class and the cgroup is additionally mandatory where frozen; neither is an alternative. Neither command changes the parent or normal shell startup files.
- Limit this phase to public embedded profiles, `P1-CHEZMOI-STATIC`, exact authored versions and exact authored locks already candidate-bound before trust, `P1-MISE-CORE-NODE`, and one effective home source. Do not resolve aliases or generate/change locks after trust.
- Reject updates, profile switching, old-target deletion, force/override, tasks/hooks/plugins/custom backends/sourced env, scripts/externals/removal/root controls, target override, source authoring, sensitive diffs, provider values, and host-fact templates.

### Exit gate

A clean public fixture completes standalone plan/apply and one-command bootstrap to the same stable facts. Exact-candidate reapply is idempotent; every changed candidate and unowned/user-modified target fails before mutation. Same/replacing plan races, mutation/recovery exclusion, writer takeover ABA, and exec/shell read-transition-launch races match ARCH-031. All child statuses, signals, cancellation, escalation, descendants, hard death, and `P1-SHELL-BASH` behavior match the frozen policy. Failure injection at every lease, owner-publication, persistence, external-mutation, and release boundary restarts into only a complete supported pre-mutation relation, truthful dirty `A/B/phase`, `CLEANUP(B)`, stable `B`, or evidence-preserving `BAD`; direct recovery reaches verified stable only when the exact relation and external conditions permit.

## Phase 5 — Selected-gitlink exclusion confirmation

### Purpose

Confirm the closed source boundary without implementing transport.

### Work

1. Exercise planning and bootstrap-shaped runner paths with selected canonical and invalid HTTPS/SSH gitlinks. Each fails before authentication, network, candidate creation, manager execution, or mutation.
2. Confirm unselected canonical HTTPS/SSH gitlinks receive bounded static validation only and zero executable/helper/agent/relay/private-endpoint/authentication/network access.
3. Confirm there is no transport CLI flag, prompt, output, digest, tuple, typed identity producer, credential/helper/agent/relay recipe, private endpoint channel, or dormant transport service/API.
4. Repeat VAL-009/010/011/012, the selected-gitlink clause of VAL-038, and runner-level VAL-042. Phase 5 may add only rejection/non-access enforcement needed to satisfy the already frozen exclusion contract; it implements no positive transport.

### Exit gate

Every selected gitlink and private source fails at the source-selection boundary; every unselected gitlink remains zero-access; public embedded selection remains the sole positive source surface; and no credential, helper, agent, relay, private endpoint, transport approval, or identity-producer implementation exists. This is exclusion confirmation, not transport qualification or support.

## Phase 6 — Atomic public/source cutover

### Purpose

Publish one product with no reachable old behavior or compatibility surface.

### Work

1. In one integration change, make `cmd/zerb/main.go` only production signal/context setup, one-time home/raw-environment capture into immutable `paths.Input`, side-effect-free option construction, and `os.Exit(app.Run(...))`; `app.Run` retains parse/version-before-root precedence and exactly-once root resolution. Remove every old command, alias, help line, and fallback.
2. Delete obsolete command files/tests and the full old `internal/config`, `service`, `drift`, `git`, `chezmoi`, `shell`, `binary`, `platform`, `transaction`, and `testutil` trees after any retained algorithm has been independently reimplemented under target contracts; reject old-tree imports from production and `_test.go`.
3. Replace old examples with one minimal schema-1 public superproject fixture. Remove `github.com/go-git/go-git/v5`, `github.com/yuin/gopher-lua`, their unused transitives, and `github.com/google/uuid` unless target code independently uses it. Retain ProtonMail crypto, sigstore, or gopsutil only when target code imports it and focused evidence justifies it. Run `go mod tidy` only after old packages are gone.
4. Remove global `gosec` suppressions for `G204` and `G301` through `G306`; permit only narrow code-site exceptions with a contract reason and focused security evidence. Update build, CI, development tooling, contributor guidance, and README to the actually installed product.
5. Remove stale binaries and temporary output; retain only intended ignore rules.
6. Run final repository/reference sweeps and verify no old command, executable behavior, parser/config model, activation path, compatibility alias, stale export, hidden fallback, or obsolete documentation authority remains.
7. Perform DeepReview and final security review, resolve every accepted finding, and rerun affected checks.
8. Build the installed artifact and run the installed-binary clauses of VAL-038 and VAL-041 through VAL-046 for every retained implementation candidate. Only after they pass may VAL-001 promote an exact complete tuple to supported. Change roadmap status to implemented only after every claimed tuple is promoted.

### Exit gate

The working tree, help, installed binary, dependencies, examples, and documentation expose only the target product. All required checks and installed-binary scenarios pass before VAL-001 records each claimed support tuple; candidates that do not pass remain unsupported and are not claimed. Provider and host-fact surfaces remain rejected. Review dispositions and redacted evidence are complete.

## Implementation orchestration

The primary owner retains product-contract edits, source/profile/candidate/digest/trust interfaces, process environment and lifecycle policy, root/state/recovery/receipt semantics, `app.Run`, integration, and release decisions.

Only after the Phase 1 freeze, at most three thin units may run concurrently behind frozen interfaces:

1. native Git resolver and safe materializer;
2. mise isolation/resolution adapter; and
3. chezmoi effective-source/target adapter.

They may not redefine source identity, candidate fields, state transitions, redaction, or CLI behavior. Test/tracing infrastructure remains primary-owned unless one unit is otherwise complete. Required reviews are: independent plan and security review at Phase 1 freeze; focused foundation security review before mutation; and DeepReview plus security review for Phase 6. Future provider work requires a separate approved contract and focused provider security review before retrieval.

## Definition of MVP done

MVP is complete only when the installed binary satisfies every applicable `PC-*`, `ARCH-*`, and `VAL-*` requirement and VAL-001 has promoted each exact claimed public tuple after its Phase 6 vertical; public embedded standalone/bootstrap flows pass; selected gitlinks/private sources and provider/host-fact profiles reject at their owned boundaries; unselected gitlinks receive zero access; exact authored versions/locks never mutate after trust; lifecycle/state/recovery/no-interference contracts pass; and no old behavior, compatibility surface, credential mechanism, transport authorization, or private transport implementation remains.

Deferred private selected transport, updates, switching, source authoring/synchronization, encryption, more platforms, and stronger execution surfaces remain non-committed intent in [Post-MVP](post-mvp.md).
