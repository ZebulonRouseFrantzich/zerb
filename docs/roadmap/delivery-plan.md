# Delivery Plan

## Delivery strategy

ZERB will make a hard pre-alpha cutover rather than incrementally extending the current architecture.

The implementation MAY temporarily coexist with old packages on a dedicated cutover branch while the replacement is built. It MUST NOT merge or release a mixed product. The cutover merge must expose only the new configuration model, CLI, state, and documentation; obsolete runtime paths, compatibility aliases, and old claims must be removed in the same change.

No product implementation begins until the Phase 1 proof results have been incorporated into the OpenSpec change, strict validation has been rerun, and renewed product/security approval has been recorded.

## Phase 0: OpenSpec architecture reset

### Purpose

Convert this roadmap into an approved, testable contract and explicitly stop work that depends on the obsolete architecture.

### Work

1. Create a verb-led OpenSpec change such as `refactor-profile-restoration-architecture`.
2. Add a proposal, mandatory design document, implementation tasks, and complete delta specs.
3. Define or replace capabilities for:
   - Environment source resolution and explicit first-run source binding.
   - Profile selection and composition.
   - Constrained native Git and selected private gitlinks.
   - Mise isolation and tool convergence.
   - Chezmoi isolation and home convergence.
   - Structural planning, digest-bound trust, and planned candidate artifacts.
   - Transition state, atomic stable commit, recovery, and receipts.
   - Secrets and sensitive output.
   - Command-scoped execution and child shells.
4. Define REMOVED/RENAMED requirements for:
   - Lua configuration.
   - Timestamped generated configs.
   - Config add/list/remove.
   - `.zerb-active` and active Lua symlinks.
   - Custom three-way drift and resolution.
   - Automatic source commits.
   - Claimed rollback.
5. Make the active `add-config-remove-command` change cease to be active with an explicit superseded/incomplete historical record; do not archive it as implemented and do not apply its deltas to canonical specs.
6. Define exact CLI contracts, source arguments, candidate identifiers, state-based command admission, exit codes, and automation behavior.
7. Define the MVP Linux/architecture matrix and pinned Git, mise, and chezmoi prerequisites.
8. Run strict validation and obtain initial approval for disposable proof work only. Canonical `openspec/specs/` remains current-state truth and is not replaced until the cutover merge applies the change's deltas through the approved cutover-specific archival process.

### Exit gate

- Every proposed breaking behavior has an ADDED, MODIFIED, REMOVED, or RENAMED delta requirement with scenarios.
- The proposed design resolves source topology, isolation, trust, state transitions, recovery, and legacy-state behavior far enough to bound the proofs.
- No product implementation task begins; only the Phase 1 disposable proofs are authorized.
- Initial product and security approval for those proofs is recorded.

## Phase 1: Upstream and safety proof spikes

### Purpose

Prove the external contracts before building product code around them. All experiments run under disposable HOME/XDG roots with pinned candidate versions.

### Git proof

- Resolve and pin an approved native Git executable once, then invoke it by absolute path.
- Fetch a public superproject without submodule recursion or worktree checkout.
- Resolve an embedded public profile and fetch only one selected private gitlink object.
- Prove that inaccessible unselected submodules are untouched.
- Prove approved SSH-agent and HTTPS credential-helper flows without credential-bearing URLs.
- Record exact public metadata exposed by `.gitmodules`, gitlinks, and history.
- Prove per-invocation policy neutralizes hooks, init templates, filters, custom remote helpers, pagers/editors, unapproved URL rewrites, and unsafe protocol forms while preserving only approved credential channels.
- Materialize through object plumbing and prove no checkout hook/filter executes.
- Display and authorize the post-policy credential-free endpoint before any credential invocation.
- Reject nested submodules, unsafe paths, unsupported object types, and dirty authored inputs.
- Demonstrate that every selected object is fetched before host mutation.

### Mise proof

- Acquire and verify one pinned mise binary.
- Invoke it only by absolute path.
- Prove isolation of config, data, cache, state, installs, downloads, plugins, shims, system config, and discovery roots.
- Plant hostile sentinels in user/global/system mise configuration and prove they are not read or executed.
- Prove deterministic native TOML composition for `base + selected` without duplicate tool keys.
- Prove exact resolution and lock reuse for the initial supported backend matrix.
- Prove actual installed-version verification and second-run idempotence.
- Prove `zerb exec` leaves the parent shell and existing mise activation untouched.
- Prove a clean child-shell strategy without sourcing existing mise hooks.

### Chezmoi proof

- Acquire and verify one pinned chezmoi binary.
- Invoke it only by absolute path with explicit source, config, cache, persistent-state, working-tree, and destination paths.
- Plant hostile sentinels in existing chezmoi config/source/state and prove they are not read or modified.
- Materialize one effective source from pinned base and selected fragments.
- Preserve supported file bytes, path bytes, and executable modes.
- Obtain reliable machine-readable destination mappings from chezmoi.
- Reject duplicate raw paths and duplicate resolved targets.
- Prove the allowed template/data subset.
- Prove structural inspection with secrets skipped.
- Apply and verify in a disposable destination.
- Demonstrate that an existing destination target is classified as a conflict rather than overwritten.

### Filesystem transition and protection primitives

- In a disposable spike harness, prove same-filesystem protected staging, file and directory sync, atomic rename behavior, and crash-visible outcomes needed by the stable/current commit.
- Prove lock ownership/staleness rules coordinate ZERB writers without claiming exclusion of unrelated host writers.
- Inject failure and process termination around each low-level persistence boundary and establish which bytes can be observed after restart.
- Record the minimum journal facts needed by Phase 2 to implement resume and repair without guessing.
- Prove umask-independent mode enforcement, symlinked/hardlinked/wrong-owner/world-writable root-component rejection, and no broader-permission temporary file after interruption.
- Prove archive hash/authentication completes before parser entry; demonstrate extraction rejection for absolute paths, traversal, aliases, duplicates, symlinks/hardlinks, devices, and oversize entries; prove poisoned/partial cache is detected and outside sentinel untouched.
- Do not treat the spike harness as the production state machine.

### Process environment proof

- Prove that the assembled environment for each process class (Git, mise, chezmoi, exec/shell) matches the frozen variable-by-variable policy.
- Inject poisoned parent values for each cleared class (MISE_*/RTX_*/ASDF_*, CHEZMOI_*, GIT_*/*, SSH_*, HOME/XDG, PATH/shims, proxy/prompt/provider vars, hostile FDs, ambient markers) and prove descendants do not receive or act on them.
- Prove approved auth/provider channels still function with the constructed environment.

### Exit gate and contract freeze

1. Capture every proof result, pinned upstream version, fixture, and rejected assumption as a source-linked OpenSpec design decision and scenario.
2. Narrow the MVP wherever an upstream or filesystem guarantee failed; product code may not emulate unsupported behavior through text parsing or guessed semantics.
3. Re-run strict validation of the complete change.
4. Obtain renewed product approval and focused security approval of Git policy, process environments, storage, state/recovery, and no-interference proofs.
5. Freeze the source/profile/candidate/state/receipt interfaces before Phase 2 product implementation.
6. Provider execution contracts (mise backends, chezmoi templates, provider references) are approved provisionally in Phase 1 based on specification only; real-provider proof is required in Phase 5 and may narrow the approved surface. Phase 2–4 product code MUST gate provider execution on an approved allowlist and MUST NOT implement provider value retrieval until Phase 5.

## Phase 2: New foundation on the cutover branch

### Purpose

Build the replacement foundations with no destination mutation beyond fixtures.

### Target implementation units

- `source`: native Git invocation, source IDs, commits, gitlinks, safe tree extraction, and availability.
- `profile`: manifest parsing, schema versions, `base + selected` validation, and collision policy.
- `toolchain`: pinned mise/chezmoi acquisition, verification, and absolute executable resolution.
- `execenv`: allowlisted subprocess environment construction and inherited-control-variable scrubbing.
- `mise`: native configuration staging, inspection, lock handling, install planning, and observed-version verification.
- `home`: chezmoi effective-source construction, target mapping, conflict classification, apply, and verify.
- `plan`: destination-read-only aggregate plan and trust digest.
- `state`: writer lock, candidate journal, atomic stable/current commit, dirty-state startup reconciliation, and command-admission policy.
- `recovery`: resumable phase handlers, revalidation, forward repair, and fail-closed recovery CLI contracts.
- `receipt`: canonical digests, pending and completed immutable records, the atomic stable/current reference, and public redaction.

Package names are architectural boundaries, not frozen Go import paths. The OpenSpec design owns the final package map.

### Foundation requirements

- Filesystem roots are explicit dependencies. Tests inject them through internal constructors; the production CLI derives fixed XDG roots once, performs legacy detection independently of XDG overrides, rejects unsafe ownership/link/permission topology, and exposes no ZERB-specific arbitrary-root flag or environment variable.
- All subprocesses receive argument arrays and a constructed environment.
- No production operation resolves mise, chezmoi, or Git through an interpolated shell string.
- Mise and chezmoi are never selected from `PATH`.
- Secrets are represented by references, not value-bearing types.
- Every local schema is versioned.
- Existing legacy state is detected and refused without mutation.

### Exit gate

Focused tests demonstrate safe path handling, canonical digests, source provenance, environment isolation, locking, journaling, stable/current atomicity, crash reconciliation, recovery admission, and receipt completion semantics. No obsolete CLI is wired into the new foundations. A focused security review confirms the implemented trust-boundary primitives still match the frozen design before mutation integration begins.

## Phase 3: Structural plan and doctor

### Purpose

Deliver the entire decision path before host mutation.

### MVP commands introduced

- `zerb profile list <source>`
- `zerb plan <source> --profile <name>`
- `zerb doctor`
- `zerb version` and `zerb --version`

`profile list` receives the public superproject source explicitly, performs only constrained public-source/static inspection, and never authenticates to or fetches an unselected private profile.

### `plan` lifecycle and responsibilities

Phase 3 `plan` handles public embedded profiles with static template functions only. Private profiles, provider-backed profiles, and host-fact-dependent templates are rejected until Phase 5.

1. Receive the source and profile explicitly; never infer either from the current directory or ambient Git state.
2. Acquire and verify the pinned managed mise and chezmoi adapters in protected storage. This may mutate only ZERB's toolchain/cache roots.
3. Constrain and resolve Git, inspect public metadata, and select the profile. Reject private profiles and provider-backed profiles in Phase 3/4.
4. Fetch public source objects and safely materialize them through no-checkout object plumbing.
5. Perform schema, source-kind, path, collision, ancestor-path, and prohibited-execution checks without running candidate-controlled code.
6. Compute the candidate/policy digest and obtain trust bound to that digest before invoking mise, chezmoi, templates, or declared commands.
7. Build the protected effective mise and chezmoi candidates and run only approved inspection surfaces.
8. Resolve exact tool versions and available integrity facts without installing declared tools.
9. Classify every destination target with no-follow ancestor validation; any pre-existing target outside the stable receipt is a non-overridable MVP conflict.
10. Persist an immutable planned candidate artifact with its explicit digest, trust evidence, destination observations, expiry/revalidation conditions, mutation phases, and every non-hermetic or unavailable input. The artifact records no resolved secret values or provider outputs.

### `doctor` responsibilities

- Report installation and schema health.
- Report stable, prepared, mutating, or dirty state truthfully.
- Check ZERB-managed binary integrity and version support.
- Check source and artifact availability separately from identity.
- Check provider readiness without printing values.
- Compare observed state with the last completed receipt.
- Never adopt, rewrite, or delete user state automatically.

### Exit gate

On both clean and populated disposable homes, `plan` binds the explicit source/profile for a public embedded profile with static template functions only, names the planned candidate artifact digest, explains every proposed tool and target action without mutating destinations or declared-tool installations, and does not resolve secret values or retrieve provider outputs. Private profiles, provider-backed profiles, and host-fact-dependent templates are rejected with a clear message pointing to Phase 5. Candidate-controlled execution never precedes digest-bound trust. Repeated plans for unchanged pinned inputs and policy produce the same structural digest.

## Phase 4: Public bootstrap and apply

### Purpose

Complete one honest public vertical slice.

### Commands introduced

- `zerb bootstrap <source> --profile <name>`
- `zerb apply --candidate <digest>`
- `zerb recover resume`
- `zerb recover repair`
- `zerb exec -- <command>`
- `zerb shell`

Standalone `apply` always requires the exact planned candidate artifact digest; it never chooses "latest" or infers a candidate. `bootstrap` is an orchestration command, not a hidden bypass. It performs the same explicit source binding, adapter acquisition, constrained fetch, static validation, digest trust, semantic plan, candidate persistence, separate mutation confirmation, apply, verification, and stable commit as the standalone path.

### Mutation order

1. Acquire the writer lock.
2. Load the named planned candidate artifact and revalidate source objects, candidate/policy digest, trust, destination observations, disk space, and required access.
3. Refuse any pre-existing candidate target not owned by the exact stable receipt. No force flag or interactive approval exists in the MVP.
4. Persist and sync `prepared(A,B)`.
5. Persist and sync `mutating(A,B,tool-install)` immediately before the first call that may install a tool.
6. Install tools additively with the ZERB-managed mise.
7. Verify exact observed tool versions.
8. Persist and sync `mutating(A,B,home-apply)` immediately before the first call that may render or write a target.
9. Apply the single effective source with the ZERB-managed chezmoi.
10. Verify managed targets.
11. Persist and sync `mutating(A,B,shell-metadata)` before generating command/shell environment metadata.
12. Generate and verify that metadata.
13. Write and sync immutable pending receipt B.
14. Atomically write and durably sync one `stable/current(B)` record that both references B and declares stable. This commit alone completes receipt B.
15. Clean the obsolete journal; startup treats an interruption here as stable B with stale cleanup, never as crossed current/state truth.

Every abnormal exit after step 5 and before step 14 starts in `dirty(A,B,phase,journal)`. While mutating or dirty, apply/bootstrap/exec/shell and candidate replacement fail closed. `recover resume` and `recover repair` revalidate all relevant assumptions and perform only supported forward convergence; neither edits a journal manually nor claims rollback.

### MVP restrictions

- Public embedded profiles with static template functions only. Private profiles, provider-backed profiles, and host-fact-dependent templates are rejected until Phase 5.
- Fresh bootstrap and exact-candidate reapply only; any changed candidate is an unsupported update rejected before mutation.
- No automatic deletion of old-only targets.
- No in-place profile switching.
- No target-conflict override or force flag.
- No source authoring, Git commit, push, or pull commands.
- No global shell startup modification.
- No OS-package provisioning.
- No scripts or externals in home source fragments.
- No sensitive content diffs.

### Exit gate

A clean supported Linux fixture with static template functions reaches a working profile in one invocation. Reapplying the identical candidate is idempotent; a changed candidate fails before mutation. Profiles with host-fact-dependent templates are rejected in Phase 3/4. Existing mise and chezmoi installations and state are unchanged. `zerb exec` and `zerb shell` use only ZERB-managed tool installations in stable state and fail closed while dirty. Real failure injection proves resume or repair from every supported mutation phase.

## Phase 5: Private profiles and provider-backed secrets

### Purpose

Complete the required private-source and secret flow without weakening the public bootstrap.

### Sub-phase A: Proof and constraint freeze

Real-provider proof runs before any provider product code is written:

1. Extend `plan` and `apply` to handle selected-only private gitlink resolution, object fetch, and safe no-checkout materialization. Effective endpoint authorization before credential invocation and missing-authorization failure before host mutation. Credential-free locator validation and diagnostic redaction.
2. Run real-provider proof against the provisionally approved contract from Phase 1. For each approved provider/backend, exercise real retrieval/render in disposable fixtures and record the exact function surface, host-fact inputs, and output behavior.
3. Freeze the MVP template function allowlist: restrict to deterministic data-only functions and explicitly approved provider-specific lookup functions; `output`, `run`, `command`, and arbitrary filesystem-access functions remain excluded. Classify each approved provider-specific lookup function as versioned (result bound by provider generation token in the candidate; exact-candidate reapply permitted) or dynamic (result depends on runtime host state; profile is bootstrap-only).
4. Prove that ZERB orchestration does not supply pre-existing manager paths or credentials to managed descendants; document that trusted candidate code runs with full user authority and can access normal user files.
5. Prove exact-candidate reapplicability for profiles using only static or versioned inputs; prove bootstrap-only enforcement for profiles with dynamic-unversioned provider outputs or host-fact-dependent templates.

### Sub-phase B: Incorporate and revalidate

1. Incorporate proof results into the OpenSpec change: narrow the provider/template function surface, add or remove scenarios, update the design.
2. Run strict validation of the updated change.
3. Obtain focused security approval of the provider execution constraints, template function allowlist, and no-interference proof.

### Sub-phase C: Implement and integrate

1. Implement provider retrieval, rendering, and integration behind the frozen allowlist.
2. Explicitly document that some provider authentication, retrieval, rendering, or network failures are knowable only during apply; such failures leave the exact `home-apply` phase dirty.
3. Restrictive ownership and modes established at creation for rendered secret-bearing files and provider-adjacent temporary storage.
4. Security tests covering process arguments, constructed environments, inherited descriptors, descendants, temp files, caches, errors, logs, receipts, exported diagnostics, and approved provider-owned storage.

### Exit gate

A private bootstrap succeeds with approved native authentication; denied private authorization fails before host mutation; unselected private profiles are untouched; provider execution is confined to the frozen MVP allowlist and ZERB orchestration does not supply pre-existing manager paths or credentials to managed descendants; provider failures occur at the documented preflight or dirty apply boundary; profiles with static or versioned-provider inputs support exact-candidate reapply; profiles with dynamic-unversioned provider outputs or host-fact-dependent templates are enforced as bootstrap-only; separate auth/provider/rendered secret markers appear only in their explicitly allowed locations; and documented public metadata exactly matches observation.

## Phase 6: Hard-cutover MVP release

### Purpose

Remove the obsolete product and publish only the replacement.

### Work

- Replace the CLI entrypoint and help.
- Remove old commands, aliases, parser, generators, state model, drift engine, and automatic source mutation.
- Remove obsolete dependencies after retained code is re-audited.
- Replace README, examples, project context, current specs, and development documentation.
- Detect legacy installations and stop with exact preservation/manual-clean-start instructions.
- Validate the supported Linux/architecture matrix.
- Perform DeepReview and a dedicated security review.
- Re-run targeted checks after every accepted review fix.
- Build the release artifact and exercise the installed binary, not only package tests.

### Exit gate

The repository contains no reachable old behavior, compatibility shim, stale claim, or canonical documentation for the superseded architecture. The OpenSpec tasks and specs match observed behavior. The full release evidence in [Validation](validation.md) passes.

## Phase 7: Post-MVP work

Dependency order:

1. Precisely owned additive updates.
2. Safe shrinking updates.
3. In-place profile switching.
4. Profile authoring and source update/push workflows.
5. Metadata-hidden local source registry, if user demand justifies it.
6. Whole-file encrypted profiles and key recovery.
7. Additional simultaneous profile overlays if user evidence justifies them.
8. macOS support.
9. Windows support.
10. OS package provisioning.
11. Source scripts, externals, hooks, and task execution under a stronger trust model.

### Switching gate

Switching MUST NOT ship until a separate design proves:

- Old/new inventory computation under their exact contexts.
- Shared, new, old-only, moved-owner, and type-conflict classification.
- Immediate revalidation before mutation.
- User-modified target preservation.
- No symlink following during cleanup.
- No recursive deletion of directories containing unknown children.
- Journaled quarantine with recovery data.
- Recovery after interruption at every cleanup phase.

No `forget`, path-difference unlink, or forced `destroy` is an acceptable substitute.

## Implementation orchestration

### Execution mode

Use orchestrated implementation only after the post-proof OpenSpec contract freeze and renewed approval. The source/profile contract and transition state machine are cohesive and remain under one primary owner.

### Parallel units after contracts freeze

These units can proceed in parallel behind agreed interfaces:

1. Native Git source resolver and safe materializer.
2. Pinned mise isolation/resolution adapter.
3. Pinned chezmoi effective-source/target adapter.

The primary owner integrates them through candidate, plan, state, and receipt contracts.

### Ownership

- Primary: manifest, source topology, candidate lifecycle, state machine, receipts, CLI semantics, integration, and final tradeoffs.
- Focused workers: Git, mise, and chezmoi adapters with isolated acceptance tests after interface freeze.
- Validation owner: primary, using disposable end-to-end roots, real pinned binaries, direct CLI invocations, and crash injection.
- Designer: not required for the initial CLI.
- DeepReview: required before the cutover release.
- Security review: required after Phase 1 proofs, after trust-boundary foundation implementation, and before release for Git authentication, untrusted source paths, template/provider execution, secrets, subprocess environments, storage, and recovery.

### Ordering

```text
initial OpenSpec approval for proofs
  -> external/filesystem proof spikes
  -> proof incorporation + strict revalidation + renewed product/security approval
  -> frozen shared contracts
  -> parallel thin adapters + primary-owned state/candidate integration
  -> source-bound structural plan
  -> public apply and recovery
  -> private/provider flow
  -> hard cutover and release validation
```

## Definition of MVP done

The MVP is complete only when:

- All renewed, approved OpenSpec tasks and scenarios are satisfied. The cutover merge commit archives the change and applies its deltas to canonical `openspec/specs/` atomically (a cutover-specific exception to the normal post-deployment archival flow in `openspec/AGENTS.md`); no separate archival step follows.
- Clean public and selected-private bootstrap work on every supported platform tuple.
- Standalone commands work independently: explicit-source profile list/plan, exact-digest apply, doctor, recover resume/repair, exec, shell, version, and `--version`.
- Exact-candidate reapply is idempotent for profiles with static or versioned-provider inputs; profiles with dynamic unversioned provider outputs are enforced as bootstrap-only and refuse reapply.
- Every changed candidate is rejected before mutation.
- ZERB orchestration does not read, execute, or write pre-existing mise/chezmoi state; no-interference tests prove this through access tracing.
- Every pre-existing unowned target is a non-overridable conflict.
- Private endpoint authorization and every provider precondition knowable without value retrieval fail before mutation; documented runtime-only provider failures leave dirty home-apply state.
- Every abnormal exit after possible mutation is represented as dirty and can be resumed or repaired through the supported CLI to verified stable, except when external conditions (provider unavailable, source unreachable) prevent convergence, in which case the dirty state is truthfully diagnosed with explicit user guidance.
- The stable/current record and receipt reference cannot expose crossed truth under failure injection.
- Provider execution is confined to the approved MVP allowlist; ZERB orchestration does not supply pre-existing manager paths or credentials to managed descendants.
- Separate secret markers appear only in declared destinations or explicitly approved protected provider-owned ephemeral storage, never in ZERB-owned artifacts or output.
- No resolved secret values or provider outputs appear in planned candidate artifacts, receipts, logs, or exported diagnostics.
- Receipts accurately record observed facts and limitations.
- The shipped repository and documentation contain no reachable old architecture, compatibility path, or stale canonical claim.
