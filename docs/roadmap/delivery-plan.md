# Delivery Plan

## Delivery strategy

ZERB is a pre-alpha hard cutover. There are no supported installations or data migrations, so the replacement must not ship compatibility aliases, an old parser/importer, a feature flag, a second public binary, state migration utilities, or mixed command routing. Replacement packages may coexist with untouched old packages on `project_refocus_and_security_fixes` and its isolated continuation branches while the replacement is built; no intermediate phase merges to `main`, and the public entrypoint remains the old pre-alpha program until the single Phase 6 cutover.

[Product contract](product-contract.md) owns public behavior. [Architecture](architecture.md) owns trust, process, protected-root, and durable-state invariants. [Validation](validation.md) owns evidence and support claims. [Hard cutover](hard-cutover.md) owns artifact disposition. [Post-MVP](post-mvp.md) owns non-committed intent. This document owns only sequencing and phase admission.

Phase 0 proof-only contracts are closed. Ten Phase 1 plan/security review rounds found blocking proof/API defects. The eleventh corrective source and affected artifacts are in progress; every candidate remains `inconclusive`, and Phase 1 freeze approval has not been requested. The owner approved four corrective contract decisions: initialization-error acquisition ownership, state-owned journal-less repair, a distinct empty-READY repair result, and atomic current completed-writer/operation identity. Incorporate those changes and rerun affected proofs plus independent plan/security review before requesting freeze approval. The owner also approved the split gate below: reviewed non-release implementation may proceed after cloud conformance while real frozen-host qualification remains outstanding. No Phase 1 status is support; promotion still requires VAL-001 plus Phase 6 installed-binary verticals.

## Split development and qualification gates

Cloud conformance is necessary for admission to reviewed, non-release Phase 2–4 implementation; it does not qualify a Phase 1 candidate or freeze an unproved contract. Admission requires both a passing current source-bound cloud report and resolved independent plan and focused security reviews of the affected plan/contracts. The owner's split-gate approval authorizes that sequencing without another discretionary approval for each report or work unit. Normal owning-contract edits and applicable repeat reviews still apply.

In a fresh Linux checkout, first run `python3 scripts/check_parser_pin.py --prepare --go /absolute/path/to/pinned/go` to reconstruct the temporary dependency offline. Then run `python3 scripts/cloud_conformance.py --output <outside-repository-report.json>`. Preparation is explicit; the cloud collector and inventory-only mode never prepare or repair source. Expanded ignored dependency bytes remain source-bound. Its fixed current scope is disposable `./internal/proof/...` race tests, all-package vet, the configured lint gate, read-only module verification, a release-shaped compile of the unchanged old entrypoint, the cloud script's boundary tests, the replacement import gate, the local parser source/effective-selection pin and upstream parser race suite, and race tests for the introduced `internal/paths`, `internal/safefs`, `internal/profile`, `internal/receipt`, `internal/toolchain` and the private `internal/state` replacement-record foundation and `tools/importcheck`. It binds results to source HEAD and tree state and records actual kernel/distribution/filesystem, process prerequisites, tools and CA inventory. Required checks and source-identity checks must pass; failed or not-exercised required checks block cloud admission. Inventory-only mode is diagnostic and cannot pass this gate. A report applies only to its recorded source; changed source needs the applicable checks and reviews again.

Inventory records observations and frozen-host mismatches; it never executes host-qualification probes. Every cloud report retains `host_qualified=false` and `release_supported=false`. A mismatching host/tool inventory alone does not fail cloud conformance, and a matching inventory alone cannot establish qualification. A successful old-entrypoint compile claims neither installation nor target-runtime behavior. Reports with exact source/tree/tool identities remain private operational diagnostics outside the repository; exported evidence follows VAL-004/005 redaction and claim rules.

This permits provisional implementation behind the exact ARCH-030 APIs in Phase 2, then Phase 3 and Phase 4 in their existing dependency order, with focused permanent replacement tests/race checks, their phase-specific checks and focused foundation security review before mutation integration. The cloud report's proof-package race suite does not validate future replacement packages or claim legacy full-runtime coverage. All PC/ARCH protections remain mandatory: exact tool/CA identities, authenticated supervisor, required cgroups, namespace/capped staging, filesystem and durable-state boundaries fail closed when unavailable. There is no development fallback runner, relaxed runtime mode or substitute mock qualification. Cloud tests may exercise safe local behavior and refusal paths; host-dependent success remains not exercised until the real host can run it. Integration rework is an accepted risk of implementation before qualification.

Before qualification, movement between Phase 2–4 work scopes requires complete implemented dependency interfaces and passing applicable local tests/reviews in dependency order; do not introduce placeholder services. Their exit-gate statements remain acceptance requirements. Unexercised real-host rows block qualified acceptance, rather than provisional implementation of the next reviewed scope. Do not report a phase as fully accepted or claim its host-dependent outcomes from local-only checks.

The separate Phase 1 qualification/freeze gate still requires the complete real Fedora/kernel/x86_64/btrfs matrix, exact tool and CA evidence, all required positive and dependent negative rows, independent plan/security review and owner freeze approval. Cloud conformance cannot waive any row or turn `inconclusive` into `implementation-qualified`/`exclusion-proven`. Real frozen-host qualification/freeze is required before local Phase 6 public-entrypoint cutover work. Release, real-workstation use and support additionally require complete Phase 6 installed-binary acceptance and final reviews; this plan grants no publication, push or merge authority. Phase 5 exclusion confirmation remains required before Phase 6.

## Phase 0 — Authority migration and exhaustive disposition

### Purpose

Establish a credential-free roadmap as the only durable project authority, record the old branch truth without completing it, and bound disposable proof work.

### Work

1. Work on `project_refocus_and_security_fixes` or its isolated continuation branches; retain history and never merge an intermediate phase to `main`.
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
- The Phase 0 exit authorizes disposable Phase 1 proof work. Later reviewed non-release implementation is admitted only by the separate cloud development gate above.

## Phase 1 — Four proof slices and contract freeze

### Purpose and harness rules

Qualify external and durability contracts on the real frozen host before contract freeze and release. Reviewed non-release implementation may provisionally depend on the exact contract candidates through the separate cloud development gate. The finite proof universe is the closed non-support matrix in `validation.md`; Phase 1 may mark a passing positive row only `implementation-qualified` and a passing negative row only `exclusion-proven`. Neither is support. Every experiment uses disposable `HOME`, XDG data/cache/state roots, destination, generated-source, manager state, evidence, and outside-sentinel paths that cannot address the developer's real home, credentials, manager state, or destinations. Authentication/provider markers, credential stores, helpers, agents, sockets, relays, and private endpoint fixtures are not positive inputs; exclusion proofs show they are never accepted or accessed.

Use the exact Fedora/kernel/x86_64/btrfs host candidate and Git 2.55.0, Bash 5.3.15, mise 2026.7.17, coreutils 9.11 signal fixture, and CA-bundle candidates declared in validation from nixpkgs revision `b7c2ada94fe99c15b0dbcf4d11fd7850b957a436`. Use forked chezmoi `v2.70.5-zerb.1` at public commit `ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`, based exactly on upstream v2.70.5 and reproducibly built with `CGO_ENABLED=0`. Use exact public Git fixtures, manifests containing unselected gitlinks, selected-gitlink/private-source rejection fixtures, exact-authored-lock mise fixtures, fork-equivalence and static chezmoi fixtures, and disposable root/state/marker fixtures.

Each proof result records the closed-matrix row, requirement IDs, exact upstream version and canonical executable identity, reviewed public source or typed static-locator reference, fixture/revision/tree/candidate identity and equality/change relations, invocation/environment shape, process tree, access/write/resource observations, result, rejected assumption, and contract edit. Selected-gitlink/private-transport rows record exclusion and zero access, never a tuple, credential recipe, authorization digest, or identity producer.

### Slice 1 — Native Git and source grammar

Prove all of the following as one public-source and exclusion contract:

- Resolve the `P1-PUBLIC-*` and `P1-GIT-MATERIALIZE` Git 2.55.0 candidate identity once and use argument arrays plus the ARCH-006 environment policy, including exact `maintenance.auto=false` and `gc.auto=0` controls that prevent detached automatic maintenance.
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
- Prove deterministic `base + selected` composition and only the closed candidate surfaces. Exact authored `node` version `24.4.1` and its complete authored lock enter the candidate before trust; aliases, ranges, channels, `latest`, missing/generated locks, and post-trust digest mutation reject before other manager execution. Exercise exact mise offline `install --raw`, `where`, `env --json`, and `exec`; ordinary install mode is excluded because it performed a successful self group change. The sole pre-trust adapter invocation is the exact forked chezmoi `managed --skip-template-data-environment` read-only, non-rendering, machine-readable target mapping over the protected effective source: prove reviewed upstream/patch/build/executable identity, normal-versus-flagged structural-output equivalence, exact argv/environment/descriptors/roots, a synthetic non-real destination, deterministic structural target inventory, zero semantic host-fact/template/helper/provider/script/external/candidate-content execution, only PC-031's pinned-Go bootstrap paths/syscalls, and zero real-destination access/write. Bind that inventory into the candidate before trust and revalidate the same mapping policy and executable identity before mutation. Any fork change beyond initialization/export suppression, any output divergence, or any restricted access removes the candidate; never add a fallback mapper.
- Invoke the VAL-038 disposable static validator directly, without `internal/app.Run`: directly supplied embedded provider/host-fact input rejects before candidate creation; selected-gitlink input rejects earlier under VAL-009 before authentication, network, materialization, or validator entry.
- Prove `exec` resolves a bare name only against the already-constructed stable profile `PATH`, rejects empty/relative entries and slash-containing relative paths, identity-checks explicit absolute paths, and accepts only regular Linux x86_64 ELF images. Copy/hash/seal the exact source bytes into a CLOEXEC executable memfd and execute only that snapshot; inject same-size source mutation after final validation and prove it cannot execute. Reject scripts/shebangs and other formats before release, leave the parent environment unchanged, and return every normal child status `0` through `255`. Prove `shell --shell bash` resolves only the exact Bash 5.3.15 identity, invokes `--noprofile --norc -i` without `$SHELL` or normal startup files, assigns a fresh PTY controlling terminal and foreground group, and contains both foreground and background job-control groups.
- For native Git, acquisition, mise, chezmoi, `exec`, and `shell`, freeze and exercise the complete environment and descriptor matrix under poisoned-parent values.
- Freeze the atomic spawn protocol: close-on-exec for every parent-created descriptor; child-side close-range/allowlist semantics safe against concurrent opens; authenticated descriptor-only stage selection; full device/inode/owner/group/type/mode/size/SHA256 identity of opened parent, child, and same-binary stage descriptors; descriptor hashing by positional reads; descriptor execution rather than mutable-path resolution; full-lifetime OS-thread locking around thread-local parent-death setup; acceptance only of the expected inherited signal or zero; child-side installation and immediate parent/peer/executable/signal rechecks; and parent synchronization through exact target release. Exercise concurrent high and low-number, close-on-exec and non-close-on-exec open/dup/close churn plus hard death before and after clone, descriptor sanitation, group/session setup, parent-death setup, exec, and first descendant fork for every process class.
  The proof must combine continuous per-spawn file/pipe/AF_UNIX/FIFO churn across selected free low and high slots with both close-on-exec states, plus isolated exact `3/4/5` collision rows. Because AF_UNIX slot `3` is the authenticated dispatch authority, its untrusted form is a rejection row; positive socket collision rows hold a marker file at `3` and sockets at `4/5`. Every managed target still admits exactly `0,1,2`.
- For every process class, prove the candidate Linux group/session setup, direct-child parent-death mechanism, catchable signal forwarding, context-cancellation grace/escalation, leader and every adopted descendant/job-control group reaping, orphan detection, and wrapper/child status mapping. Exercise child signals and wrapper SIGINT/SIGTERM.
- Prove no managed descendant survives catchable cancellation or wrapper hard death. Every retained process class MUST use the Phase 1-proven same-binary FD-only supervisor and exact cgroup boundary; these are cumulative requirements, never alternatives. The authenticated guard holds the exact cgroup directory descriptor, writes descriptor-relative `cgroup.kill`, waits for descriptor-relative `pids.current=0`, and is itself reaped with status verification. Freeze `memory.max=805306368`, zero swap, `pids.max=96`, and one-core CPU quota, require zero OOM/OOM-kill events, and install the manager-only seccomp denial for all `io_uring` interfaces before release. The 768 MiB edge is a finite exact passing bound, not a proved minimum; unretained lower-memory diagnostics cannot qualify or narrow it. Approved managed versions must not daemonize, call `setsid`, perform a successful group change, or escape the boundary. Do not accept leader-only cancellation or `exec.CommandContext` as lifecycle evidence. Deliberately detached arbitrary user payloads are outside orchestration only if the product contract states the limit and evidence proves no ZERB manager credentials or state paths reach them.
  Ordinary completion and catchable cancellation use descriptor-relative supervisor cgroup kill, reaping, and emptiness verification before guard disarm. Hard parent death uses the independent guard. Missing, signaled, or nonzero guard status is an operational failure on every path; inject a nonzero guard exit and prove its exact status is reaped and propagated.

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
- Prove candidate/journal durability, writer generation/token binding, `prepared`, every `mutating` phase, pending receipt, atomic stable/current commit including completed writer/operation identity, exact stale cleanup, orphan receipt handling, and all recognized restart states. Exercise byte-identical candidate-and-receipt reapply before journalling and after commit/cleanup, including transferred cleanup writers; prove state-owned journal-less repair and its digest-free empty-READY result. Inject initialization errors across handoff and prove the returned still-owned acquisition is released exactly once.
- Inject failure, cancellation, and hard termination at every lock-publication, lease, persistence, external-mutation, and release boundary, then run the directly admitted `doctor`, `recover resume`, or `recover repair` path without manual state edits.

The disposable spike is evidence for the frozen contract, not production state-machine code.

### Slice 4 — Schema and parser

Source-verify and pin the approved local `github.com/pelletier/go-toml/v2@v2.4.3-zerb.1` variant under PC-042, including its upstream base, patched-module checksum and effective version-specific replacement. Exercise the exact PC-040 and PC-041 structs only through an input-size bound followed by `toml.NewDecoder(...).DisallowUnknownFields().Decode(&target)`.

Prove rejection and deterministic diagnostics for schema mismatch, unknown fields and nested tables, duplicate keys/tables/IDs, dotted-key aliases, malformed input, invalid UTF-8 where prohibited, multiple semantic interpretations, invalid or aliased paths, gitlink mismatches, and every byte/count/string/path/depth bound. Exercise the provisional 1 MiB document and 256-profile limits, freeze every remaining field limit, and retain fuzz/property fixtures. If the pinned local variant admits ambiguity or bypasses a bound, reject the version or narrow the grammar before freeze.

### Phase 1 exit gate and freeze

Before Phase 1 qualification/freeze and any release or public cutover, every applicable closed-matrix row has a result and a Phase 1 disposition, every rejected assumption has changed the normative owner, and at least `P1-PUBLIC-EMBEDDED` plus every foundation candidate it needs is `implementation-qualified`. Every negative row required by a retained positive candidate is `exclusion-proven`; a failed or inconclusive required negative row blocks the dependent candidate until the normative owner narrows/removes the surface, updates the matrix, and all affected proofs pass. The disposable harness includes VAL-035 approval separation/precedence and VAL-036 all-plan `A-X` lifetime, cache-only pre-trust behavior, immutable-publication races, and changed-candidate rejection. Record owner approval, independent plan review, and focused security review. Freeze all of:

1. The implementation-qualified Fedora Linux 44 / kernel `7.1.10-200.fc44.x86_64` / `x86_64` / btrfs proof candidate; explicitly record that it remains unsupported.
2. Git 2.55.0, Bash 5.3.15, coreutils 9.11 signal fixture, mise 2026.7.17, forked chezmoi `v2.70.5-zerb.1` public source/patch/commit, `cacert`, parser, and fixture identities; their nixpkgs revision or upstream origin; canonical paths; public-artifact digests; checksums; build mode; and provenance checks.
3. Public source grammar, unselected canonical HTTPS/SSH gitlink static grammar and zero access, `P1-SELECTED-GITLINK-EXCLUSION`, absence of every credential/helper/agent/relay/private-endpoint/typed-identity/transport-authorization mechanism, and every public source ceiling.
4. Exact schema-1 fields, including mandatory current completed-writer/operation identity, and every byte/entry/string/path/depth/count/allocation limit; `go-toml/v2` version/checksum/strict behavior/error mapping and corpus.
5. Exact CLI/output/error/approval behavior; stable-PATH execution; child/wrapper/cancellation behavior; and the Bash-only shell candidate.
6. Exact source/profile/candidate/tree/policy/receipt digest inputs and canonical encoding, plus the rule that retained sensitive digests become fresh typed references and relations rather than digest oracles.
7. Only `P1-MISE-CORE-NODE` and `P1-CHEZMOI-STATIC`, including exact authored versions/locks before trust, every negative surface, and non-orthogonal combinations.
8. For every candidate process class: environment/descriptors; atomic spawn; group/session; forwarding; grace/escalation; parent death; reaping; orphan and hard-death behavior; and deliberate-user-detachment limit.
9. ARCH-016/017 protected-root layout and initialization; ARCH-018's exhaustive classifier and extra-entry rules; cache, filesystem/archive, and destination-race policy.
10. ARCH-023/024/025/027/031 owner schemas and relations; every initialization/writer publication and sync boundary; ABA-safe takeover; prepared/mutating/dirty/stable/cleanup truth; the complete PC-016 command/lease table; initialization-error acquisition ownership; the state-owned journal-less repair seam and distinct empty-READY result; reconciliation; and recovery.
11. ARCH-030's exact replacement package set, forbidden-import seam, one-way import DAG, frozen internal APIs/service method sets, and permanent proof fixtures.
12. The closed marker/redaction policy and VAL-038 source-specific provider/host-fact exclusion in the disposable validator, without `internal/app.Run` or installed-binary evidence.

A required positive candidate that is failed, inconclusive, unfrozen, awaiting review, or dependent on a negative row that is not `exclusion-proven` blocks Phase 1 qualification/freeze and release. Reviewed non-release Phase 2–4 work may proceed only through the separate cloud development gate; it does not change those dispositions. Phase 1 may remove or narrow a candidate and must update/rerun the dependent matrix, but it cannot call one supported; VAL-001 and Phase 6 own promotion.

## Phase 2 — Replacement foundations

### Purpose

Build protected source, execution, storage, and state foundations without importing old product contracts or mutating real destinations.

The provisional increments implement the bounded descriptor-owned `safefs.Dir` open/create/identity/bounded-list/pinned-file-read/directory-sync/close seam under the approved ancestor policy, immutable `paths.Input` capture and lexical Linux XDG resolution, `profile` declaration values/static selection, manifest/component TOML parsing and independent static template validation, the ARCH-030 import gate, the `receipt` value structs, strict PC-010/050 token parser and bounded pure candidate/completed-receipt canonicalizers, and the immutable `toolchain.FixedCA` bundle capability with exact identity and complete builder-format validation. The owner-approved local TOML fork zeros the decoder before pooling; deterministic release/history tests and the explicit source pin address the former historical-retention blocker without excluding pooled storage from PC-042. Template validation remains independent and never renders or executes input. These are not complete Phase 2 acceptance: source/state integration of candidate/receipt identities, authenticated source observations, acquiring services/CA materialization and the other foundations remain outstanding. Every affected local check/review must pass before provisional admission. PC-042's 8,000,000-byte reservation remains conservative backing-storage accounting, not literal cumulative Go allocation enforcement or a hard runtime-memory cap. Real frozen-host qualification and release/support gates remain separate.

The owner approved PC-050 empty-array normalization and content-only component-tree identity, PC-022 native root-tree identity, and subsequently the private bounded exact-wire encoder and ARCH-026 destination/integrity conventions. The pure `receipt.CandidateDigest` and `ReceiptDigest` seam now implements those value contracts with the validation document's checked whole-invocation reservation and standard-encoder differential oracles. It adds no source acquisition, runtime observation, installed-tree verification, state/current authority or public command routing. Source-owned URI/identity/object relations and state-owned candidate/receipt/runtime correspondence remain mandatory integration work. This local increment changes neither phase admission nor host/release gates and does not complete Phase 2.

The subsequent bounded `safefs.Lock` increment implements the existing descriptor-owned OFD seam, using the separately approved writable numeric-FD reopen while preserving the full root and proc trust checks. It provides no initialization/owner publication, state lease, role-overlap, recovery or durability authority. Direct local kernel evidence and refusal paths remain separate from skipped exported-path positives and the frozen-host matrix. Public routing, phase admission and release/support gates remain unchanged.

The following bounded `Dir.PublishNoReplace` increment implements only same-parent anonymous-file staging and atomic descriptor-owned no-replace publication, with file/parent synchronization, exact reopen verification and explicit post-attempt uncertainty. It leaves existing targets untouched and grants no initialized state, role-overlap, lease or recovery authority. No named partial state/data staging or new proc trust exception is admitted; host-dependent success remains separately qualified.

The owner-approved [staged replacement contract](architecture.md#staged-replacement-and-operation-record-contract) and [failure matrix](validation.md#protected-replacement-failure-matrix) admit only the bounded descriptor primitive, private metadata codec and pure prefix recognizer after independent authority/security review. Production initialization, maintenance execution and state integration remain separate; the exact source-bound cloud report and focused tests do not promote host qualification.

The protected-role increment closes the explicit namespace/peer matrix, read-only root reservations, explicit successful-creation prefix binding and validated thread-bound kernel topology before code. It is a bounded physical-directory foundation, with no state executor, cross-namespace manager or absent/file-destination completion claim.

The owner-approved next increment is ARCH-017's bounded private FreshInit producer and its exact ARCH-020/030 `CreateDir` and pinned `File` prerequisites, ARCH-024 ordinary record codecs and fixed current-process observation. Independent owning-contract review precedes production coding. Implement the complete ordered fresh prefix, live witness retirement, I-X handoff and Release-only failure ownership for the three admitted intents; no partial `Store`, stub `AcquireFor`, full classifier, takeover/current/journal/receipt work, new package/dependency, unsafe test factory or host-policy exception is included. Focused primitive, codec, production-recognizer, current-process and genuine disposable integration tests follow validation.md's bounded FreshInit matrix. Skipped absolute-path positives and lower-helper successes remain distinct; frozen-host acceptance, runtime protections, public routing and release/support gates are unchanged.

The owner approved the independent [TreeStage lifecycle/resource contract](architecture.md#treestage-lifecycle-and-resource-contract) and bounded implementation after concrete representation fit. The [representation review](treestage-representation.md) and focused layout tests close only that design gate; the [failure matrix](validation.md#treestage-lifecycle-failure-matrix), runtime resource enforcement and independent implementation security review remain required. The implementation is based on verified merged initialization, without adding state orchestration. Source/toolchain integration and all qualification/public-cutover gates remain unchanged.

The owner approved the [chezmoi-first canonical USTAR/offline-manifest direction](architecture.md#chezmoi-first-archive-preparation), with mise deferred. Preparation may close the exact recipe, evidence checklist and resource worksheet; authenticated actual build output, complete runtime/notice inventory and concrete fit must pass before dependent materializer production coding. No placeholder Service, caller-digest trust shortcut, new streaming API or substitute same-version executable is admitted. An unavailable Nix/compiler/build-output environment stops that gate without package or host-security changes. The [VAL-028 preparation matrix](validation.md#chezmoi-archive-preparation-gate) remains unexercised until actual observations exist; it grants no Phase 2, host or installed acceptance.

Independently of that stopped artifact gate, the owner approved the private
ARCH-024 current-record scalar codec. Its bounded exact encoding retains the
completed writer/operation identity and adds no filesystem publication,
owner/journal/receipt graph authority, lease, Store/router or recovery method.
Focused canonical/rejection/ownership/race/fuzz checks and independent security
review precede source-bound conformance. Existing initialization validators,
archive build/inventory/fit prerequisites and host/public-cutover gates remain
unchanged; a valid private current value is not evidence of a completed receipt.

The next owner-approved private increment completes general writer-owner value
validation/codecs and ARCH-024's current-to-held/released-writer correspondence.
It preserves FreshInit's narrower accepted set, concrete record tags, wire bytes
and errors. Closed predecessor-shape consequences add no history reconstruction,
parity or generation-headroom policy. This is not lock acquisition, liveness,
takeover, publication, recovery, completion authority or a partial AcquireFor.
Focused generation/token/JSON/FreshInit-preservation, race/fuzz and independent
security checks plus source-bound conformance precede publication. READY graph
reopening/admission and all archive/host/MVP acceptance dependencies remain
separate subsequent work.

The subsequent approved existing-empty-initialized observation retains the exact
intent-selected admission OFD while reopening and revalidating its narrow complete
physical graph. It is a private observation, not a partial Store, classifier,
command admission, liveness check or recovery executor. Broader valid durable
content and cache misses stay outside this helper without being classified as
corruption. Focused graph/lifetime/substitution/race/fuzz tests and independent
security review plus exact-source conformance precede its stacked draft PR.
Positive absolute-path checks may remain unexercised in Cloud; all existing host,
archive and installed acceptance gates remain unchanged.

The owner subsequently approved the exact journal checkpoint/observation and
four-dimension completion rules in ARCH-025/026 and the single-window provisional
process policy in ARCH-010. The private state value predicates and proof-fixture
parity implement the ARCH-025/026 value rules without a graph classifier, lease, journal codec,
mutation executor or completion/publication authority. Complete per-document
predecode/canonicalization reservation and round-trip fit remain separate; no
arbitrary wire cap or whole-graph accumulation envelope is introduced. Phase 2
acceptance, actual artifact inventory and frozen-host qualification remain open.

The separately reviewed process increment adds only private opened-executable
fingerprints and updates the actual lifecycle proof candidate to the approved
single graceful window. It includes cumulative forced termination before waiting,
complete post-kill cleanup/guard verification and explicit cleanup-failure status
precedence. The full production Runner/Dispatch, per-class environment assembly,
cgroup admission, full retained invocation matrix and host qualification remain
outstanding. Foundation conformance now explicitly races the new execenv package;
this coverage strengthening does not create process execution authority.

The owner approved the specialized private large-state codecs after the standard
decoder's capacity exceeded the complete reservation for existing accepted output.
First independently review the concrete representation/accounting plan and layout
checks, then implement the closed canonical decoders and direct pending encoder,
with differential/fuzz and exact-boundary evidence. Ordinary scalar codecs,
formats, accepted values, the complete budget, package/API seams and aggregate
resource/host gates remain unchanged. A passing layout argument alone is not
production enforcement, file integration or a completed durable graph. The
bounded implementation now provides private candidate/pending/journal decoders
and direct pending/journal encoders, with old-minimum-schedule admission before
materialization. Its actual layout and differential/boundary/race coverage are
reviewed independently. It does not activate a Store, graph classifier, writer,
filesystem publication or completion authority; full graph resource policy and
future file-read ownership accounting remain required before that integration.

### Work

1. Enforce the exact ARCH-030 one-way DAG and forbidden old-package seam for every package introduced in this phase. Phase 2 completes only `paths`, `safefs`, `profile`, `receipt`, `execenv`, `source`, `toolchain`, `state`, and test-only `testenv`; it does not create compile-only `mise`, `home`, `plan`, `recovery`, `app`, or `host` scaffolds. The DAG gate scans production and `_test.go`, rejects imports of every full old package tree including `internal/testutil`, and permits `internal/testenv` only from `_test.go`. No bridge, alias, re-export, registry, fallback, production `testenv` import, or old CLI route is permitted.
2. Implement immutable copied `paths.Input` and fixed XDG data/cache/state resolution through ARCH-030's import-free `paths` API, with no product-specific override or ZERB config root. Service constructors are side-effect-free; they perform no root, filesystem, environment, process, network, or durable-state work until `app.Run` supplies its single resolved `paths.Roots`. `safefs.OpenRoles` owns descriptor/mount/bind role validation and every lower service revalidates its role set at each protected boundary. Implement the exact ARCH-016/017 initialized layout and I-X-to-admission handoff before other persistence.
3. Implement no-follow protected filesystem and archive primitives, authenticated acquisition/extraction, native Git object storage/materialization, strict schemas/digests, exact staged toolchain capabilities, explicit descriptor close methods, and exact durable value/codec/diagnostic/initialization methods behind their ARCH-030 owners. Managed mise/home services begin only in Phase 3 when their complete structural behavior can be finished; no placeholder service is allowed.
4. Implement the ARCH-030 `execenv` seam with absolute executables, argument arrays, complete frozen environments, descriptor closure, no shell, and the mandatory authenticated same-binary FD-only supervisor for every retained class, plus the frozen cgroup boundary when required; neither is replaceable by `exec.CommandContext` or another primitive.
5. Implement ARCH-031 initialization/admission/writer synchronization and owner publication before state transitions. Persist candidate `B` and its writer-bound journal before mutation; sync `prepared`; sync exact `mutating-tool-install` and `mutating-home-apply`; sync pending receipt; atomically publish/sync current as stable; clean exact residue; publish and verify the released writer-owner generation; then unlock writer and admission without unlinking either stable lock inode. Shell/exec environment is reconstructed from stable receipt/tool facts and has no durable mutation phase.
6. Retain the strict parser conformance/fuzz corpus and focused crash/substitution/TOCTOU failure injection from Phase 1 so package/API implementation cannot widen the frozen contracts.
7. Preserve redaction at every error and diagnostic boundary and keep candidate/state/receipt types explicit, versioned, map-free, sorted, and value-free.

### Exit gate

Focused evidence covers the Phase-2 subset of the ARCH-030 import/API seam, path/archive safety, strict parser behavior, canonical digests, native Git policy, complete process environments/descriptors/lifecycle, exhaustive root classification, initialization/writer publication and observed-boundary substitution defense, every foundational lease race, candidate publication, durable phase schemas, and receipt completion authority. A dedicated security review of native Git, environment construction, descriptors, acquisition/extraction, roots, locks, admission, state, digests, and receipt completion is resolved and affected checks rerun before destination or tool mutation integration.

## Phase 3 — Non-public target runner and structural commands

### Purpose

Build the complete decision path behind one injectable runner while preserving the old public entrypoint until Phase 6.

### Work

1. Complete the remaining ARCH-030 `mise`, `home`, `plan`, `recovery`, `app`, and `host` packages behind the non-public seam. Their full methods are implemented and focused-tested now—no stubs, disabled branches, or compile-only interfaces—but only the structural commands named below are exercised through `app.Run` in this phase. `app.operations` is the sole command-flow owner; `host` only assembles inert `app.Options`.
2. Implement and exercise through `app.Run`: `profile list`, public/static `plan`, `doctor`, `version`, and `--version` under PC-010 through PC-016. `app.Run` parses and completes root-independent local checks first; invalid argv and either version form return without root resolution or constructor activation. A well-formed non-version command resolves paths exactly once, constructs complete services once, and invokes one lease-owning operation method.
3. `profile list` performs only public unauthenticated static inspection and sorted-ID output.
4. After the one root resolution and root-service activation, every `plan` acquires and revalidates `A-X` before root-state classification beyond minimum lease topology, source access, adapter/toolchain staging, or candidate validation, then holds it through cache staging, candidate trust, every permitted immutable source/tree/toolchain/candidate publication, synchronization, reopen verification, and cleanup. There is no same-candidate shared path, upgrade, or drop/reacquire. In `STABLE(A)`, a recomputed candidate other than exact `A` fails as an unsupported update before publication; transition and cleanup state deny planning before source access. Toolchain `Stage` returns immutable cache-only executable capabilities used explicitly by source/mise/home; exact trust permits `Prepared.Publish`, while every missing/mismatched trust path calls `Prepared.Cleanup`. Source inspection parses PC-040 once through `profile` and binds a selected declaration to the opaque inspection. Every opened inspection/effective source is closed.
5. Run the Phase 3 `internal/app.Run` clause of VAL-038: the injected embedded fixture rejects forbidden provider/host-fact syntax before candidate creation; selected-gitlink-shaped input rejects at source selection before authentication/network/materialization. Revalidate forged forbidden candidates before mutation.
6. `doctor` reports root/schema/state/tool/source readiness without adopting, deleting, relabeling, or treating availability as identity. Selected gitlinks and private sources remain rejected through every later phase. Keep the existing `cmd/zerb` routing unchanged; do not add a second binary, build tag, or mixed route.

### Exit gate

The exact runner handles public embedded static fixtures; invalid argv and both version forms perform no root resolution or root-service activation; each well-formed non-version command resolves the immutable captured input exactly once. Stable structural input produces the same candidate digest regardless of mutable/live destination state; trust mismatch and missing noninteractive approval fail before the corresponding boundary; candidates are owner-only and value-free; and existing manager state or real destinations are not accessed or mutated. The public binary still exposes only its pre-cutover commands.

## Phase 4 — Public bootstrap, exact apply, recovery, exec, and shell

### Purpose

Complete one public static vertical slice and every truthful mutation/recovery boundary, still through `app.Run` rather than the public entrypoint.

### Mutation order

1. Acquire the exclusive ARCH-031 admission lease, acquire the stable writer exclusion, atomically publish and sync the new writer-owner generation/token/process identity, reopen/revalidate it, and retain the complete `A-X+W-X` relation.
2. Under that unchanged ownership, load the exact named candidate and prior current candidate/receipt relation, revalidate source objects, policy digest, and trust, and bind both prior digests independently; only after mutation authorization, observe real-destination topology/type/content/link/mode/owner plus access/disk/tool preconditions.
3. Persist and sync the writer-bound `prepared(A,B)` journal containing the independent prior candidate/receipt references and live destination preconditions; none alters candidate `B` or its digest.
4. Revalidate ownership and affected live preconditions; persist and sync `mutating(A,B,tool-install)` immediately before possible tool mutation.
5. Install additively and verify exact observed tool versions, revalidating ownership and affected preconditions around the external boundary.
6. Revalidate ownership and affected live destination preconditions; persist and sync `mutating(A,B,home-apply)` immediately before possible render/home mutation.
7. Render/apply one effective source and verify target ownership, content, type, mode, link, and ancestor topology, revalidating ownership and live preconditions at every affected mutation boundary.
8. Persist and sync journal phase `pending-receipt`; write, sync, reopen, and verify pending receipt `B`, including the exact stable tool observations used to reconstruct future exec/shell environments.
9. Revalidate ownership; atomically replace and durably sync `current.json` as `stable(B)`; this alone completes receipt `B`.
10. Persist/sync journal phase `cleanup`; while that journal independently names the prior candidate and prior receipt, remove and parent-sync the prior candidate only when distinct from `B`, then remove and parent-sync the prior completed receipt only when distinct from receipt `B`, then remove and parent-sync the journal last. First bootstrap skips both absent prior objects; exact A→A reapply never removes the candidate now named by current, and repeated exact reapply may reuse an exact receipt. Every interruption from stable current through journal removal is exact `CLEANUP(B)` and no receipt is ever temporarily unreferenced.
11. Atomically publish and sync the released writer-owner generation, reopen and verify that released generation, then unlock writer and admission in order. Never unlink either stable lock inode.

### Work and restrictions

- Integrate and exercise standalone exact-digest `apply`; orchestration-only `bootstrap` uses the same `app.operations` inspect, trust, plan, candidate, authorization, mutation, verification, receipt, and stable-commit path and takes one `A-X+W-X` before planning rather than upgrading or reacquiring.
- Exercise every PC-016 state/admission row exactly, including same versus replacing plan, exact resume/repair, stable cleanup, parse/version precedence, and denial before missing-approval handling. Recovery revalidates and converges forward only.
- Exercise `exec` and explicit-ID `shell` on the complete `execenv` lifecycle, retaining `A-S` through stable identity/environment binding, authenticated same-binary supervisor release/reaping, the required cgroup boundary, and status. Neither command changes the parent or normal shell startup files.
- Limit this phase to public embedded profiles, `P1-CHEZMOI-STATIC`, exact authored versions and exact authored locks already candidate-bound before trust, `P1-MISE-CORE-NODE`, and one effective home source. Do not resolve aliases or generate/change locks after trust.
- Reject updates, profile switching, old-target deletion, force/override, tasks/hooks/plugins/custom backends/sourced env, scripts/externals/removal/root controls, target override, source authoring, sensitive diffs, provider values, and host-fact templates.

### Exit gate

A clean public fixture completes standalone plan/apply and one-command bootstrap to the same stable facts. Exact-candidate reapply is idempotent; every changed candidate and unowned/user-modified target fails before mutation. Same/replacing plan races, mutation/recovery exclusion, observed-boundary writer path/record substitution, and exec/shell read-transition-launch races match ARCH-031. All child statuses, signals, cancellation, escalation, descendants, hard death, and `P1-SHELL-BASH` behavior match the frozen policy. Failure injection at every lease, owner-publication, persistence, external-mutation, and release boundary restarts into only a complete supported pre-mutation relation, truthful dirty `A/B/phase`, `CLEANUP(B)`, stable `B`, or evidence-preserving `BAD`; direct recovery reaches stable truth without manual edits.

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

Phase 1 real frozen-host qualification/freeze is a prerequisite to this public cutover; cloud conformance and provisional Phase 2–4 completion cannot substitute for it.

1. In one integration change, make `cmd/zerb/main.go` first call `execenv.DispatchSupervisor`; an exactly authenticated reserved control descriptor returns the internal supervisor status without public argv parsing, while every absent/invalid descriptor falls through unchanged. The public path then performs only signal/context setup, one-time home/raw-environment/TTY capture, `host.NewOptions`, and `os.Exit(app.Run(...))`. `app.Run` retains parse/version-before-root precedence and exactly-once root/service activation. Remove every old command, alias, help line, hidden argv supervisor mode, and fallback.
2. Delete obsolete command files/tests and the full old `internal/config`, `service`, `drift`, `git`, `chezmoi`, `shell`, `binary`, `platform`, `transaction`, and `testutil` trees after any retained algorithm has been independently reimplemented under target contracts; reject old-tree imports from production and `_test.go`.
3. Replace old examples with one minimal schema-1 public superproject fixture. Remove `github.com/go-git/go-git/v5`, `github.com/yuin/gopher-lua`, `github.com/google/uuid`, and their unused transitives. Retain ProtonMail crypto, sigstore, or gopsutil only when target code imports it and focused evidence justifies it. Run `go mod tidy` only after old packages are gone.
4. Remove global `gosec` suppressions for `G204` and `G301` through `G306`; permit only narrow code-site exceptions with a contract reason and focused security evidence. Update build, CI, development tooling, contributor guidance, and README to the actually installed product.
5. Remove stale binaries and temporary output; retain only intended ignore rules.
6. Run final repository/reference sweeps and verify no old command, executable behavior, parser/config model, activation path, compatibility alias, stale export, hidden fallback, or obsolete documentation authority remains.
7. Build the installed artifact and run VAL-037's complete installed-binary argv/flag/state/precedence/side-effect/output/redaction matrix, the installed-binary clause of VAL-038, and VAL-041 through VAL-046 for every retained implementation candidate.
8. Perform DeepReview and final security review, resolve every accepted finding, and rerun every installed check affected by a review-driven change. Only after the reviews and affected reruns pass may VAL-001 promote an exact complete tuple to supported. Change roadmap status to implemented only after every claimed tuple is promoted.

### Exit gate

The working tree, help, installed binary, dependencies, examples, and documentation expose only the target product. All required checks and installed-binary scenarios pass before VAL-001 records each claimed support tuple; candidates that do not pass remain unsupported and are not claimed. Provider and host-fact surfaces remain rejected. Review dispositions and redacted evidence are complete.

## Implementation orchestration

The primary owner retains product-contract edits, source/profile/candidate/digest/trust interfaces, process environment and lifecycle policy, root/state/recovery/receipt semantics, `app.Run`, integration, and release decisions.

After either the reviewed cloud development gate (provisional non-release work) or Phase 1 qualification/freeze, use useful task-appropriate parallelism within actual platform/runtime limits behind the exact reviewed ARCH-030 interfaces. The owner removed custom concurrent-subagent limits. Coordinate file and contract ownership and select from these independent units:

1. native Git resolver and safe materializer;
2. mise isolation/resolution adapter; and
3. chezmoi effective-source/target adapter.

They may not redefine source identity, candidate fields, state transitions, redaction, or CLI behavior. Test/tracing infrastructure remains primary-owned unless one unit is otherwise complete. Required reviews are: independent plan and security review for cloud development admission and again at Phase 1 qualification/freeze; focused foundation security review before mutation; and DeepReview plus security review for Phase 6. Future provider work requires a separate approved contract and focused provider security review before retrieval.

## Definition of MVP done

MVP is complete only when the installed binary satisfies every applicable `PC-*`, `ARCH-*`, and `VAL-*` requirement and VAL-001 has promoted each exact claimed public tuple after its Phase 6 vertical; public embedded standalone/bootstrap flows pass; selected gitlinks/private sources and provider/host-fact profiles reject at their owned boundaries; unselected gitlinks receive zero access; exact authored versions/locks never mutate after trust; lifecycle/state/recovery/no-interference contracts pass; and no old behavior, compatibility surface, credential mechanism, transport authorization, or private transport implementation remains.

Deferred private selected transport, updates, switching, source authoring/synchronization, encryption, more platforms, and stronger execution surfaces remain non-committed intent in [Post-MVP](post-mvp.md).
