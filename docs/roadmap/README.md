# ZERB Roadmap

**Status:** Phase 1 qualification outstanding; owner-approved split gate permits reviewed non-release implementation after cloud conformance; product not implemented
**Last updated:** 2026-10-22

ZERB is a declarative, profile-based developer-environment restoration tool for mutable workstations. It converges declared user-space tools and dotfiles through pinned, isolated native adapters. It is not an operating-system image, backup system, package manager, transactional rollback engine, hermetic build system, or machine snapshot.

ZERB is the project name, not an acronym or backronym.

## Authority and freeze state

Roadmap authority is deliberately split. When summaries differ, use the owner below:

1. [Product contract](product-contract.md) owns public CLI, output, errors, exit behavior, approvals, source grammar, local schemas, composition, and user-visible state behavior.
2. [Architecture](architecture.md) owns trust boundaries, process policy, protected roots, storage, durable-state transitions, recovery invariants, and package boundaries.
3. [Delivery plan](delivery-plan.md) owns phase sequencing, prerequisites, review gates, and implementation orchestration.
4. [Validation](validation.md) owns the support matrix, proof scenarios, evidence requirements, and release acceptance.
5. [Hard cutover](hard-cutover.md) owns old-to-new artifact disposition and the clean-cutover checklist.
6. [Post-MVP intent](post-mvp.md) owns non-committed, target-compatible deferred direction only.

[Proof results](proof-results.md) is an append-only redacted evidence ledger. It is never normative authority and cannot create a support claim. The [root README](../../README.md) describes the current branch and development warning.

Phase 0 contracts are provisional and proof-oriented. Phase 1 may narrow an assumption when real evidence disproves it. The owner-approved split gate in [delivery](delivery-plan.md) admits reviewed non-release Phase 2–4 implementation after current source-bound cloud conformance and independent plan/security review, with focused replacement tests and phase reviews. Only the complete real frozen-host Phase 1 matrix, owner incorporation, independent plan/security review and owner freeze approval qualify/freeze contracts. Required runtime protections remain mandatory, with no development fallback runner. Real qualification/freeze precedes local Phase 6 public-entrypoint cutover work; release, real-workstation use and support additionally require final installed-binary acceptance and reviews.

The former specification tree is historical input, not current authority. Git history preserves its exact contents. The active change ID `add-config-remove-command` is `superseded/incomplete; delta never adopted`; its missing work is not target scope.

## Non-negotiable direction

| Area | Provisional decision |
|---|---|
| Product boundary | Restore one declared developer environment on a mutable workstation. |
| Language | Keep Go; target development uses Go 1.24. |
| Delivery | Hard cutover with no aliases, importers, feature flags, state migration utility, second public binary, or mixed CLI release. |
| Composition | Exactly embedded `base + one selected profile`; reject tool, raw-source, and resolved-target collisions. |
| Source | One positive surface: explicit canonical credential-free public HTTPS superproject with embedded base and embedded selected profile. Unselected canonical HTTPS/SSH gitlinks are static zero-access declarations; selecting any gitlink/private source fails closed. |
| Declarations | Native mise TOML and one native chezmoi source under Phase 1-proven allowlists; no second tool or dotfile DSL. |
| Trust | Candidate trust and mutation authorization are distinct and digest-bound. There is no transport authorization, credential mechanism, or private-source approval in the MVP. |
| Isolation | Invoke pinned managed executables by absolute identity with reconstructed environments and ZERB-owned roots; never use a pre-existing manager's state. |
| Shell | Command-scoped execution or a clean child shell; never mutate or activate the parent shell. |
| State | Truthful prepared/mutating/dirty/stable journaling and forward recovery; no rollback claim. |
| Migration | There are no supported users or old-format migration. Foreign or corrupt durable roots fail closed. |
| Support | Linux and all Git/manager/shell/parser tuples remain provisional until required real evidence and final vertical acceptance; provider and host-fact behavior is excluded from the MVP. |
| Updates | MVP permits fresh bootstrap and eligible exact-candidate reapply only. Updates, switching, shrinking, and source authoring are post-MVP. |

## Target MVP outcome

Subject to Phase 1 qualification/freeze and final installed-binary acceptance, the intended installed product will:

1. Inspect an explicit public environment superproject without ambient source discovery or authentication.
2. Select exactly an embedded `base + one embedded profile`; reject any gitlink/private selection before authentication, network, candidate creation, manager execution, or mutation, while never fetching unselected gitlinks.
3. Validate and safely materialize supported embedded public source objects without checkout hooks or filters.
4. Compute a canonical candidate digest and obtain digest-bound trust before candidate-selected execution.
5. Produce a structural, redacted plan without installing declared tools, mutating destinations, or invoking provider/host-fact behavior.
6. Obtain mutation authorization separately.
7. Install exact supported tools into protected ZERB-owned storage and apply one collision-free effective home source.
8. Verify convergence, persist truthful receipts/state durably, and expose only supported forward recovery after interruption.
9. Run a command or explicit clean child shell without changing the parent process environment.

This is target scope, not a claim about the currently reachable binary. Exact behavior is identified by `PC-*`, `ARCH-*`, and `VAL-*` requirements in the owning documents.

## Phase summary

| Phase | Outcome | Permitted mutation |
|---|---|---|
| 0. Authority migration | Roadmap owns provisional contracts; exhaustive hard-cutover disposition recorded | Documentation only |
| 1. Proof and freeze | Real native Git, manager, lifecycle, filesystem/state, and parser proofs; contract narrowing and reviewed freeze | Disposable fixtures only |
| 2. Replacement foundations | Reviewed provisional foundations after cloud admission; qualification remains separate | Test roots only |
| 3. Non-public target runner | Exact dispatcher, profile listing, structural plan, doctor, and version exercised through injected services | Protected toolchain/cache/candidate roots only |
| 4. Public bootstrap/apply | Non-public runner implementation of bootstrap, exact reapply, recovery, exec and shell; provisional until qualification | Disposable approved fixture mutation only |
| 5. Selected-gitlink exclusion | Confirm selected gitlinks/private sources remain rejected, unselected gitlinks remain zero-access, and no transport/credential mechanism exists | No new transport mutation |
| 6. Atomic cutover | Old CLI/source packages removed; target binary and full supported matrix accepted | Approved user mutation |
| Post-MVP | Explicitly owned update/switch/removal, authoring/sync, encryption, hooks, more platforms, stronger execution | Separate future contracts |

Each phase is gated. The approved cloud development gate permits provisional Phase 2–4 work while real Phase 1 qualification is pending; it cannot compensate for missing proof, weaken runtime protections or authorize release. No target command is documented as publicly working before the atomic cutover.

## Current branch warning

The [Phase 1 frozen-host handoff](phase1-proof-handoff.md) lists the existing proof inputs and command interfaces. It is an operational checklist, not authority or a successful proof observation.

The provisional Phase 2 increments add `profile` declaration values/static selection, manifest/component TOML parsing and independent static template validation, the replacement import gate, strict `receipt.Digest` token parsing, and the bounded pure `receipt.CandidateDigest`/`ReceiptDigest` value seam with the approved exact-wire private encoder and receipt observation conventions. The owner-approved local TOML fork drops decoder-owned state before pooling and is verified separately from ordinary module checks. Its source pin, deterministic release/history tests and focused review address the former retention blocker; this does not qualify the parser or host. Template validation performs no rendering or execution. PC-042 remains a conservative parser/model reservation, not cumulative Go allocation enforcement or a runtime-memory cap. Source/state integration of candidate/receipt facts, authenticated source observations and other foundations remain outstanding; a pure digest grants no acquisition or completion authority. The bounded descriptor-owned `safefs.Lock` seam adds nonblocking kernel OFD operations and strict writable-descriptor admission; it grants no state lease, initialization durability or filesystem qualification. Phase 2 is not complete.

The branch remains pre-alpha. Its reachable binary is the obsolete pre-cutover product while reviewed non-release foundations proceed under the source-bound cloud gate. Reports and reviews apply only to their recorded source and scope; split-gate approval alone establishes none, and frozen-host Phase 1 qualification remains outstanding. Do not install or run this branch against a real workstation, home directory, credentials, manager state, or destination before Phase 6 installed-binary acceptance. See the [root branch-status README](../../README.md) and the baseline in [proof results](proof-results.md).
