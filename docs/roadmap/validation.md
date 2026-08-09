# Validation and Acceptance Plan

## Purpose

ZERB is accepted through end-to-end behavioral evidence, not coverage percentages or mocked wrappers. Unit tests remain useful for pure contracts, but release claims require real pinned Git, mise, and chezmoi processes operating inside disposable roots.

The validation strategy must prove three different properties:

1. The intended profile converges successfully.
2. Existing machine-manager installations and state are not touched.
3. Failure is represented truthfully and does not enable unsafe follow-on mutation.

## MVP support matrix

The formal OpenSpec proposal MUST freeze:

- Linux distributions supported in the MVP.
- CPU architectures supported in the MVP.
- Minimum native Git version.
- Exact ZERB-managed mise and chezmoi versions.
- Supported shells for `zerb shell`.
- Supported mise backends and package classes.
- Supported chezmoi source entry and template features.
- Supported Git transports and credential mechanisms.
- Supported secret providers.

A platform, backend, source control, or provider is unsupported until its full vertical scenario passes. Runtime detection without an apply test is not platform support.

## Test environment model

Every integration scenario runs in a filesystem/process sandbox that cannot address the developer's real home or credentials. Within that sandbox, use three distinct fixture layers:

1. **ZERB roots:** temporary XDG config, data, cache, and state roots; managed binary roots; Git object/materialization roots; and injected transition controls.
2. **Host-manager roots:** a disposable `HOME` populated with hostile pre-existing mise and chezmoi defaults, binaries, config, data, cache, state, hooks, filters, trust data, and shell startup files.
3. **Shared destination:** the same disposable home destination containing unowned files, ZERB-receipted files when applicable, collision targets, and an optional non-cooperating writer process.

Fixtures also provide dedicated public/private Git endpoints at exact commits, isolated credential-helper and SSH-agent processes, distinct deterministic marker providers, and real pinned Git/mise/chezmoi binaries.

The harness MUST combine:

- Before/after bytes, paths, modes, ownership, and directory-entry manifests.
- Process-tree capture of executable identity, argv, constructed environment, cwd, inherited descriptors, and descendants.
- Filesystem-access tracing or an equivalent deny-by-default namespace that proves which configured files and roots were opened, read, executed, created, renamed, or removed—not merely whether bytes changed.
- A write allowlist covering ZERB roots, explicitly planned shared-home targets, and explicitly permitted fixture-owned roots (approved helper/agent storage, approved provider-owned ephemeral storage). An outside sentinel and trace detect escape.
- Network endpoint/authentication-attempt capture for selected and unselected sources.
- Failure injection and hard process termination at every persistence and external-mutation boundary.

Mock processes are appropriate for unit-level error mapping. They are not evidence of upstream isolation, native authentication, target mapping, process inheritance, filesystem non-access, or end-to-end behavior.

## No-interference acceptance suite

### Existing mise fixture

Populate the host-manager layer with:

- A binary earlier in `PATH` that records execution and fails.
- User, environment, global, and system configs containing hooks/tasks and access sentinels.
- Nearby project config and legacy version files that would alter resolution if discovered.
- Existing data, cache, state, plugins, shims, installs, downloads, trust records, and an active parent-shell environment.
- A tool version intentionally different from the ZERB profile.

After direct `plan`, `apply`, `exec`, child-shell exit, recovery, and failure scenarios, prove:

- The existing mise binary was never executed.
- Access traces show that ZERB orchestration code did not open, read, execute, or write any pre-existing mise config, version file, data, cache, state, plugin, shim, install, download, or trust path.
- No sentinel hook or task ran.
- Parent-shell `PATH`, hooks, command resolution, and mise variables are unchanged.
- ZERB orchestration used the canonical absolute managed binary and only the explicit manager state roots and permitted fixture-owned roots.
- The selected command resolves the managed tool only inside `zerb exec` or `zerb shell`; exit restores the exact pre-ZERB command resolution.

Hashes alone cannot prove non-access. Access tracing/denial and process identity are release evidence.

### Poisoned-parent environment

Inject each cleared environment class from the process environment policy matrix and prove the managed descendant does not receive or act on it:

| Injected class | Example variables | Required observation |
|---|---|---|
| mise/asdf/rtx controls | `MISE_`, `RTX_`, `ASDF_` prefixes | Not present in descendant env; no side effects. |
| chezmoi controls | `CHEZMOI_` prefix | Not present in descendant env. |
| Git/SSH controls | `GIT_CONFIG_*`, `GIT_*`, `SSH_AUTH_SOCK`, unsafe `ProxyCommand` | Only explicitly approved channels present; unapproved values absent. |
| HOME/XDG | Injected hostile `HOME`, `XDG_*` paths | Managed descendants use synthetic ZERB-owned roots, not injected values. |
| PATH/shims | Hostile `PATH` with pre-existing mise/chezmoi shims | Managed binary resolved by absolute path; shim not executed. |
| Proxy/prompt/provider | `HTTP_PROXY`, `PS1`, provider env vars | Only approved channels present; no ambient secrets leaked. |
| Inherited FDs | Open hostile file descriptors | Managed descendants do not inherit or use unapproved descriptors. |
| Ambient marker | High-entropy sentinel in environment | Not present in any descendant output, cache, state, log, receipt, or temp file. |

For every poisoned-parent case, assert exact direct and descendant environment against the frozen process-environment matrix and zero sentinel access while approved auth/provider channels still work.

### Existing chezmoi fixture

Populate the host-manager and shared-destination layers with:

- A binary earlier in `PATH` that records execution and fails.
- Existing source, config, cache, persistent state, and working tree containing read/execute sentinels.
- An unowned destination unique to the existing manager.
- An unowned destination that collides with the ZERB candidate.

Prove:

- The existing chezmoi binary was never executed.
- Access traces show that existing source, config, cache, persistent state, and working tree were never opened, read, executed, or modified.
- The existing unique destination remains untouched.
- The colliding target is a non-overridable error and apply stops before mutation.
- ZERB passes every isolated path explicitly to its canonical managed binary and descendants.
- Concurrent ZERB mutation is blocked by the ZERB writer lock.
- A non-cooperating host writer is not claimed to be locked out: a detected change invalidates destination revalidation and fails safely; an undetectable concurrent race is documented as outside ZERB's consistency guarantee.

ZERB cannot prove safe simultaneous ownership with another manager on the same destination. The acceptance behavior is refusal and immediate revalidation, not a claim that the ZERB lock controls unrelated writers.

## Source and Git scenarios

| Scenario | Required observation |
|---|---|
| Public embedded profile | Resolves from the parent commit and component tree without any submodule authentication or checkout. |
| Public repo with inaccessible private profiles | Public selection succeeds; endpoint capture shows no authentication attempt for unselected gitlinks. |
| Selected private profile | Exactly one authorized gitlink object is fetched and materialized without worktree checkout semantics. |
| Effective endpoint authorization | Post-policy credential-free endpoint is displayed and accepted before any helper/agent credential request. |
| Missing private authorization | Fails before credential invocation, tool mutation, or destination mutation; candidate records no credentials. |
| Credential-bearing URL | Rejected before Git invocation. |
| Relative submodule URL | Effective credential-free locator is resolved after policy, displayed, and authorized before credentials. |
| Nested submodule | Rejected in the MVP. |
| Dirty working tree | Never used as profile input; pinned object content wins or resolution fails. |
| Unreachable pinned commit | Availability failure is distinct from identity; no host mutation begins. |
| Changed gitlink after plan | Revalidation fails; stale candidate is not applied. |
| Unsafe profile path/object | Traversal, absolute, aliasing, duplicate, unsupported-type, and extraction-escape inputs fail closed. |
| Execution-capable Git config | Hostile `core.hooksPath`, init templates, attributes filters, custom/ext remote helpers, pagers/editors, and protocol settings neither execute nor change resolution. |
| Endpoint rewrite | `insteadOf` or equivalent cannot silently change the displayed/authorized endpoint. |
| Public metadata audit | Documentation matches observed `.gitmodules`, gitlink, and history exposure. |
| Native Git identity/config | The canonical pinned executable is recorded; global/system Git config remains unchanged and disallowed entries are not honored. |

Git fixtures exercise every approved HTTPS credential-helper and SSH-agent mechanism. Separate authentication markers MAY remain only in the approved helper/agent's protected storage; none enters argv, URLs, repository config, ZERB state/output, or an unselected-source request.

## Profile and composition scenarios

| Scenario | Required observation |
|---|---|
| `base + selected` | Exactly two ordered components form the candidate. |
| Missing base | Validation fails. |
| Multiple selected profiles | Validation fails in the MVP. |
| Duplicate tool identity | Fails; no implicit override. |
| Duplicate raw home-source path | Fails. |
| Different source names mapping to one target | Fails using chezmoi-derived mapping. |
| Unsupported global chezmoi control | Fails with exact path and reason. |
| Unsupported Git object or filesystem symlink | Fails before candidate acceptance. |
| Stable input | Canonical materialized digest is stable across repeated construction. |
| Mode change | Candidate digest changes and plan reports it. |
| Unknown schema version or required field | Fails closed without mutation. |

Fuzz and property tests should defend path canonicalization, source-tree digesting, manifest decoding, and duplicate detection. They do not replace real target-mapping tests against pinned chezmoi.

## Mise scenarios

| Scenario | Required observation |
|---|---|
| Exact supported tool | Resolves, locks where supported, installs below ZERB data, and verifies. |
| Floating alias | Rejected or resolved and frozen before apply according to the approved contract. |
| Unsupported backend | Fails during structural planning. |
| Base/profile duplicate | Fails before mise install. |
| Lock reuse | Second run consumes the same effective lock and observes the same supported resolution. |
| Artifact unavailable | Reports availability failure without claiming source drift. |
| Integrity unavailable | Reports a lower integrity dimension rather than “verified.” |
| Install failure | Transition becomes dirty at the exact tool phase. |
| Partial additive install | Stable/current remains A; pending B is not completed and the dirty journal records observed residue. |
| Exact-candidate reapply | Reuses the same effective config/lock, performs no surprise reinstall, and observes the declared versions. |
| User project config nearby | Not discovered by the managed invocation. |
| System/global mise config | Not read, executed, or modified. |
| Parent active mise | Parent environment and hooks remain unchanged. |

The suite must exercise every configured control against the exact pinned mise version. Upgrading mise requires rerunning the isolation and lock contracts before changing the pin.

For each supported backend, evidence distinguishes the exact requested tool version, transitive dependency closure, artifact integrity, and later availability. A native mise lock/checksum is recorded where the pinned version/backend actually provides it. Version-only backends such as npm/Cargo/pipx are never described as complete transitive or artifact locks unless the proof establishes that stronger fact.

## Chezmoi scenarios

| Scenario | Required observation |
|---|---|
| Ordinary file | Applied and verified from the effective source. |
| Private/executable mode | Exact supported destination mode is applied and verified. |
| Allowed template | Renders under the approved data contract. |
| Secret-bearing template structural plan | Provider value is not fetched or printed. |
| Existing unrelated target | Remains untouched. |
| Existing unowned candidate target | Is a non-overridable conflict; no apply begins. |
| User-modified stable managed target | Is detected and not silently overwritten. |
| Source or target collision | Fails before apply. |
| Forbidden script/external/remove/root control | Fails during validation. |
| Apply/provider/render failure | Transition becomes dirty at the exact home-apply phase. |
| Post-apply mismatch | Stable/current remains A and receipt B is not completed. |
| Exact-candidate reapply | `verify` succeeds and apply makes no surprise change. |
| Existing chezmoi state | Access trace proves it remains unread, unexecuted, and unchanged across every scenario. |
| Versioned provider profile | Provider generation token is bound in candidate; exact-candidate reapply is permitted and idempotent. |
| Dynamic-unversioned provider profile | Profile is enforced as bootstrap-only; exact-candidate reapply is refused with explicit message. |
| Host-fact-dependent template | Template reads runtime host facts (OS, env, hostname); profile is enforced as bootstrap-only; exact-candidate reapply is refused. |
| Approved provider lookup function | Function is in frozen MVP allowlist; versioned variant binds generation token; dynamic variant forces bootstrap-only. |
| Excluded template function (`output`/`run`/`command`/filesystem) | Rejected during plan validation before trust; fails closed. |
| Ancestor symlink to destination | no-follow ancestor validation rejects the target; apply fails before mutation. |
| Ancestor component substitution | Replaced directory component detected by ancestor chain validation; apply fails before mutation. |

The target destination is an intentional shared surface. Tests must never claim that isolated state makes overlapping destination ownership safe.

## State, locking, and recovery scenarios

Inject process errors, cancellation, and uncatchable termination at least:

1. Before candidate persistence.
2. After candidate-object persistence but before prepared-state persistence.
3. After prepared-state persistence.
4. Immediately before and after persisting each `mutating` phase.
5. During tool installation.
6. After tool installation but before verification.
7. During home render/apply, including provider retrieval.
8. After home apply but before destination verification.
9. During shell-metadata generation.
10. After all verification but before pending receipt persistence.
11. During pending receipt write/sync.
12. After pending receipt sync but before stable/current commit.
13. During temporary stable/current write, rename, file sync, and directory sync.
14. Immediately after stable/current commit but before journal cleanup.
15. During stale-journal cleanup.

For each point, restart a new process and assert:

- Before possible mutation, current remains A and an incomplete candidate can be safely rejected or reconstructed.
- After possible mutation, startup exposes only `{current=A, dirty(A,B,phase,journal)}` or `{current=B, stable(B)}`—never current A with stable B, current B with dirty A/B, or an unreferenced receipt reported complete.
- A stale journal after the stable/current commit is cleanup residue; the atomic current record remains authoritative.
- Lock, candidate, journal, receipt, and current files are durable, owner-only, and never broader during staging.
- Apply/bootstrap/exec/shell and candidate replacement cannot proceed while mutating or dirty; doctor and recovery remain available.
- Direct `recover resume` and `recover repair` revalidate source objects, candidate/policy digest, destination observations, provider/tool readiness, and completed phase outputs before supported forward convergence.
- Recovery never requires manual journal edits, claims rollback, deletes unrelated data, or marks an unverified host stable.

Concurrency tests include simultaneous ZERB apply attempts, stale-lock recovery, process-ID reuse assumptions, cancellation propagation to every descendant, and a non-cooperating writer. The writer lock is accepted only as ZERB-process coordination; destination revalidation detects observable external changes and no guarantee is made for an undetectable race.

## Protected-root and managed-archive scenarios

### Protected-root creation

| Scenario | Required observation |
|---|---|
| Permissive umask | Created roots and files receive declared restrictive modes regardless of caller umask. |
| Symlinked root component | Rejected before any write; root not created. |
| Hardlinked root component | Rejected before any write; root not created. |
| Wrong-owner root component | Rejected before any write; root not created. |
| Group/world-writable root component | Rejected before any write; root not created. |
| Interrupted creation | Partial staging cleaned up; no broader-permission temporary file remains. |

### Managed-binary archive extraction

| Scenario | Required observation |
|---|---|
| Archive not authenticated/hashed | Rejection before parser sees bytes. |
| Absolute path entry | Rejected during extraction. |
| Traversal (`../`) entry | Rejected during extraction. |
| Alias/duplicate entry | Rejected during extraction. |
| Symlink or hardlink entry | Rejected during extraction. |
| Device or unsupported type entry | Rejected during extraction. |
| Oversized entry | Rejected during extraction. |
| Partial/corrupt archive | Treated as invalid; staging cleaned up. |
| Poisoned cache (modified after hash) | Detected on re-read; treated as invalid. |
| Interrupted extraction | Staging cleaned up; promoted content not visible; outside sentinel untouched. |

For every extraction scenario, prove full hash/authentication completes before the extractor sees bytes and the outside sentinel file remains untouched.

## Secret and sensitive-metadata scenarios

Use distinct high-entropy markers for (a) Git authentication, (b) provider storage/output, and (c) rendered destination content. Define an owner/allowed-location table before each scenario. Instrument the complete subprocess tree and scan every controlled filesystem root and captured stream after success, failure, cancellation, and hard termination.

Evidence covers argv, constructed and inherited environment, cwd, open/inherited descriptors, descendant processes, network request metadata, terminal output, exported diagnostics, and every ZERB/provider temporary, cache, state, log, journal, receipt, candidate, and materialized-source location.

- Authentication marker: MAY exist only in the approved credential helper/SSH agent's protected storage or protocol exchange; ZERB never accepts, copies, logs, or persists it.
- Provider marker: MAY exist in the provider fixture's approved protected storage and documented provider-owned ephemeral storage; it MUST NOT enter ZERB-owned artifacts/output.
- Rendered marker: MAY exist only in the declared destination file with exact ownership/mode, plus approved provider-owned ephemeral storage when the pinned provider contract requires it.

No marker may appear in a Git URL/repository config, process argument, profile/lock file, candidate/materialized source, generated non-secret runtime config, ZERB cache/state/temp/journal/receipt/log/error/diagnostic, or unrelated destination. Cleanup assertions do not excuse a transient spill: access tracing and observation at injected failures must prove restrictive creation and allowed placement throughout the operation.

Target paths, credential-free locators, commit IDs, provider IDs, and employer/profile names are sensitive metadata even when not secret values. Local protected receipts may retain only the fields required by their schema; public output and exported diagnostics prove their redaction policy separately.

## Structural-plan scenarios

A plan is accepted only if it:

- Binds an explicit source/profile and reports an immutable planned candidate artifact with explicit digest. The artifact records no resolved secret values or provider outputs.
- Writes no destination file and installs no declared profile tool.
- Makes protected managed-adapter acquisition and source/candidate/cache writes explicit.
- Does not modify parent-shell state or retrieve secret values.
- Does not invoke credential channels before endpoint authorization.
- Performs no candidate-controlled Git filter/hook, mise, chezmoi, template, provider, script, or command execution before digest-bound trust.
- Lists every unsupported input, unowned target, collision, and runtime-only provider check.
- Reports exact source pins, available tool resolutions/integrity facts, target inventory, and mutation phases.
- Includes trust/policy identity and expiry/revalidation conditions.
- Does not retrieve provider values or render sensitive content.

## Vertical smoke scenarios

### Public bootstrap and standalone flow

1. Begin with the three-layer fixture: hostile manager state plus a clean shared destination.
2. Install the release artifact as the user would receive it.
3. Run `zerb profile list <source>` and prove only public static discovery.
4. Run `zerb plan <source> --profile <name>`, accept digest-bound trust, and capture the candidate digest.
5. In a separate process run `zerb apply --candidate <digest>` and confirm mutation.
6. Repeat from a clean root using the one-command bootstrap path and compare the resolved candidate/receipt facts.
7. Run a declared tool through `zerb exec`, then open and close `zerb shell`.
8. Verify declared home targets and inspect the completed receipt/current record.
9. Reapply the identical candidate and observe no surprise change.
10. Attempt a changed candidate and an unowned target; each fails before mutation.
11. Prove pre-existing manager binaries and state were not accessed and destination allowlists were respected.

### Selected-private bootstrap

Repeat both standalone and bootstrap flows with exactly one private gitlink, endpoint authorization before native authentication, and a provider-rendered destination marker. Denied source authorization must leave the host unchanged. Exercise provider declaration/readiness failures before mutation where supported and provider retrieval/render failures during apply; the latter must leave truthful dirty home-apply state and recover through the documented CLI.

Also exercise:
- A versioned-provider profile: apply succeeds; reapply of the identical candidate is idempotent.
- A dynamic-unversioned provider profile: apply succeeds; reapply of the identical candidate is refused as bootstrap-only.
- A host-fact-dependent template profile: apply succeeds; reapply is refused as bootstrap-only.
- An excluded template function (`output`/`run`/`command`): plan validation rejects it before trust.
- A destination with a symlinked ancestor component: ancestor validation rejects it before mutation.

### Dirty recovery

For a real pinned tool and home target, terminate apply at every externally observable mutation/persistence phase. Restart ZERB, run doctor, then invoke `zerb recover resume` or `zerb recover repair` directly. For every supported failure cause that can be restored (process killed, transient I/O, interrupted network), recovery must reach verified stable through the supported CLI. For failure causes that cannot be resolved without external action (provider permanently unavailable, source unreachable), recovery must truthfully diagnose the remaining dirty state with explicit user guidance. No test may pass by editing state files.

## Validation order

For implementation changes:

1. Reproduce the relevant failure or contract in the narrowest fixture.
2. Run focused package tests.
3. Run the real adapter scenario with pinned upstream binaries.
4. Run the affected vertical smoke scenario.
5. Run broader repository checks once after integration.
6. Perform DeepReview and security review for the hard-cutover release.
7. Resolve or explicitly reject every review finding with evidence.
8. Rerun targeted and vertical checks after fixes.

Coverage is diagnostic. It is not an acceptance criterion by itself.

## Release evidence

A release candidate must retain:

- Pinned ZERB, native Git, mise, and chezmoi versions plus canonical executable digests.
- Immutable asset manifest with source URLs, sizes, SHA256 digests, and signature/provenance material for every upstream fixture and release dependency.
- Backend-specific lock/integrity/repeatability results without upgrading version-only evidence to a stronger claim.
- Supported platform matrix results.
- Public and selected-private standalone/bootstrap transcripts with secret values removed.
- No-interference access traces, process trees, write-allowlist results, and before/after manifests.
- Candidate, pending receipt, stable/current, and completed receipt examples with sensitive metadata removed.
- Failure-injection and recovery matrix results.
- Separate authentication/provider/rendered marker results.
- OpenSpec initial, post-proof, and post-provider-proof (Phase 5 Sub-phase B) strict validation/approval records.
- Build, focused test, broader test, race, vet, and configured lint results.
- DeepReview and security-review dispositions.

No release claim may exceed the scenarios actually exercised.
