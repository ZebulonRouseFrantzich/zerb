# Hard Cutover Plan

## Decision

ZERB will replace the current pre-alpha product model in one breaking cutover. The shipped repository will not contain two configuration formats, two state models, compatibility aliases, or deprecated execution paths.

This is a source and product cutover, not permission to delete user data. Legacy installations are detected and preserved, but the new binary does not silently import or reinterpret them.

## Old and new product models

| Current model | Replacement |
|---|---|
| `zerb.lua` executable DSL | Versioned environment manifest plus native profile `mise.toml` and chezmoi source fragments |
| Platform table injected into Lua | Native mise platform/environment support constrained by the approved profile contract |
| Timestamped generated Lua configs | Exact Git source commits and canonical materialized-candidate digest |
| `.zerb-active` / `zerb.active.lua` | One atomic `stable/current` record that references and thereby completes an immutable receipt, plus a durable transition journal |
| `config add/list/remove` structural rewriting | User-authored profile repositories; source authoring commands deferred |
| Chezmoi source mutations from ZERB config CRUD | One derived effective source; source repos remain immutable during apply |
| Custom baseline/managed/active drift | Plan and doctor based on source pins, native manager observations, and last completed receipt |
| Automatic config commits | Explicit user Git workflow; ZERB restore is read-only with respect to source repos |
| go-git as pure-Go Git engine | Native Git invoked safely for standard credentials and submodules |
| Parent-shell `mise activate` | `zerb exec` and a clean `zerb shell` child |
| Transaction/rollback terminology | Journaled resumable convergence with truthful dirty state |
| Single “reproducible” claim | Independent pinning, integrity, availability, compatibility, and convergence facts |

## CLI cutover

### Initial MVP CLI

```text
zerb bootstrap <source> --profile <name>
zerb profile list <source>
zerb plan <source> --profile <name>
zerb apply --candidate <digest>
zerb doctor
zerb recover resume
zerb recover repair
zerb exec -- <command> [args...]
zerb shell
zerb version
zerb --version
```

Exact flags, prompts, output schemas, and exit codes belong in the OpenSpec change.

### Removed CLI

The hard cutover removes, without aliases:

```text
zerb init
zerb activate
zerb uninit
zerb drift
zerb config add
zerb config list
zerb config remove
```

Commands documented but not implemented in the current product, including `add`, `sync`, `push`, `pull`, `restore`, and interactive rollback workflows, are not carried forward merely because old roadmap text mentioned them.

Post-MVP commands such as profile switching or source authoring receive new proposals based on the new state model; they do not revive old semantics.

## Current code disposition

### Replace and remove

These areas encode the obsolete model and should be removed from the shipping cutover rather than wrapped:

- `internal/config/`: Lua types, parser, sandbox, generator, active markers, and status model.
- `internal/service/config_add.go`, `config_list.go`, and `config_remove.go`: config CRUD orchestration.
- `internal/drift/`: custom three-way detection and resolution.
- `internal/transaction/`: config-add/remove transaction format and recovery assumptions.
- `internal/git/`: go-git repository manager and automatic commit workflow.
- `internal/chezmoi/`: current path-translation and source-mutation wrapper.
- `cmd/zerb/config_*.go`, `drift.go`, `init.go`, `activate.go`, and `uninit.go`: old command contracts.
- Lua examples and test fixtures tied to the old public configuration format.

Associated tests are removed with the obsolete contracts. They are not mechanically ported if they defend behavior the new product rejects.

### Review before selective reuse

Reuse is allowed only after behavior and dependencies are re-audited against the new contract:

- `internal/binary/`: archive extraction, download, verification, cache, and pinned-tool acquisition primitives.
- `internal/platform/`: basic OS and architecture detection without Lua injection or unsupported platform claims.
- `internal/shell/`: shell detection only; current startup-file mutation and activation behavior are not retained.
- `internal/testutil/`: disposable environment helpers.
- Generic secure path, atomic-file, and lock primitives whose semantics pass the new state tests.
- Build, CI, formatting, linting, and development-environment infrastructure that does not encode old product behavior.

Before cutover merge, generate and review a disposition manifest for every tracked production source, test, fixture, example, canonical document, build/CI file, and direct dependency: `remove`, `replace`, or `reuse after audit`, with an owner and evidence. An unclassified artifact or dependency fails the cutover gate; broad directory bullets above are not a substitute for this inventory.

“Review before reuse” is not permission to retain an obsolete public type for convenience. Reused code adopts the new names and contracts directly.

### New architectural units

The replacement needs cohesive implementations for:

- Versioned profile manifests.
- Native Git source resolution and safe materialization.
- Pinned ZERB-managed mise and chezmoi acquisition.
- Controlled subprocess environments.
- Native mise profile composition and observation.
- Native chezmoi effective-source construction and target mapping.
- Structural plans, explicit source binding, planned candidate artifacts, and digest-bound trust.
- Transition locking, journals, atomic stable/current commit, and startup reconciliation.
- Immutable pending/completed receipts.
- Recovery admission, revalidation, resume, and forward repair.
- Command-scoped execution and clean child shells.
- Sensitive metadata and error redaction.

The OpenSpec design freezes package boundaries only after the proof spikes.

## Dependency cutover

Expected removals include dependencies pulled solely by:

- `gopher-lua` and the executable Lua model.
- `go-git` and its transitive graph.
- Obsolete signing or platform libraries not used by the approved binary-verification and platform matrix.

Expected runtime prerequisites include:

- Native Git at a specified minimum version.
- User-controlled SSH-agent or credential-helper infrastructure for private sources.

Mise and chezmoi are not system prerequisites. ZERB acquires verified pinned copies into its own protected toolchain and invokes them by absolute path.

Dependency removal happens only after retained binary-verification and platform code is re-audited. The cutover should reduce the dependency graph deliberately, not replace it with another broad framework.

## Local filesystem model

The OpenSpec design will finalize paths, but the model follows XDG separation:
```text
$XDG_CONFIG_HOME/zerb/      reserved; no ambient user-config discovery in the MVP
$XDG_DATA_HOME/zerb/        managed binaries, Git object stores/materialized trees, tool installs
$XDG_CACHE_HOME/zerb/       disposable downloads and computed caches
$XDG_STATE_HOME/zerb/       candidates, journals, receipts, locks, and stable/current record
```

Defaults follow the platform's XDG conventions. Sensitive roots are `0700`; state and receipt files are `0600`. Mise and chezmoi receive their own descendants beneath these roots.

The real home directory remains the intentional chezmoi destination. It is not treated as isolated storage.

## Legacy installation policy

### Detection

The new binary checks for unmistakable legacy artifacts before first initialization, including old active markers, timestamped config layouts, transaction files, and known state directories.

### Behavior

When legacy state is found, ZERB MUST:

- Make no changes to it.
- Make no destination or tool mutation.
- Explain that the installation belongs to the superseded pre-alpha architecture.
- Show exact paths detected.
- Provide exact preservation and manual clean-start instructions.
- Require the user to manually archive or remove every detected legacy root before the default new roots can be initialized.

The MVP has no alternate-root or override escape hatch. This keeps one state topology and prevents a flag from bypassing legacy detection.

ZERB MUST NOT:

- Parse legacy Lua to synthesize a new profile.
- Rename or delete legacy files automatically.
- Reuse old transactions or active markers.
- Claim that the previous machine state can be reconstructed from them.
- Leave a hidden compatibility reader in production code.

A separately proposed offline migration utility is possible only if user demand justifies it. It must not become part of the normal runtime or weaken the hard cutover.

## Documentation and specification cutover

The release change must replace or remove every canonical claim tied to the old model:

- README name expansion, quick start, roadmap, configuration, architecture, CLI, status, and feature claims.
- `openspec/project.md` purpose, stack, isolation, configuration, state, security, and testing sections.
- Canonical config-management, drift-detection, and initialization specifications.
- Active and future proposals that depend on timestamped Lua configs or automatic source commits.
- `docs/drift-detection.md`.
- The old restore enhancement based on uninit backups.
- Lua examples.
- Agent/build guidance that names old commands or versions.

Historical OpenSpec archives remain historical records but should receive an unambiguous superseded-context note if readers could mistake them for current direction. Completed history is not rewritten to pretend the old work never happened.

The new README should link to approved specs and this roadmap, but current specs—not roadmap prose—must describe shipped behavior.

## Branch and merge strategy

1. Keep architecture work on a dedicated cutover branch.
2. Land the initially approved OpenSpec proposal before disposable proof commits; that approval authorizes proofs only.
3. Incorporate proof results, rerun strict validation, and obtain renewed product/security approval before product implementation.
4. Build new internals behind tests and disposable integration roots.
5. Do not expose or document a mixed CLI on the main branch.
6. Replace the entrypoint and delete obsolete reachable code in the cutover integration change.
7. Archive the change and apply its deltas to canonical specs atomically within the cutover merge (cutover-specific exception to the normal post-deployment archival flow). Update docs, examples, dependencies, and CI in the same release series.
8. Run release validation and independent reviews on the final clean-cutover tree.
9. Merge only when no old compatibility path remains.

A temporary internal adapter used while constructing the branch must be deleted before merge. It is not a deprecation shim.

## Release reversal and host recovery

Because the project is pre-alpha, rollout is a breaking release with explicit notes rather than a compatibility period.

Returning to the previous binary is only a release-version reversal. The previous binary is already released and cannot be modified to detect new-architecture state. If the user has never applied the new architecture, returning to the previous binary with the original legacy state is safe. If the user has applied the new architecture, the old binary MUST NOT be run until all new-architecture roots have been moved aside and the host has been manually restored and validated; the old binary shares default XDG roots with the new architecture and could misinterpret or overwrite new-architecture state.

Before first apply, the new binary states that external backup and manual host recovery remain the user's responsibility. Preserving old state is data retention, not rollback.

## Cutover completion checklist

- [ ] Breaking OpenSpec change initially approved for disposable proofs.
- [ ] Conflicting active proposal no longer active and recorded as superseded/incomplete without applying its deltas.
- [ ] External Git/mise/chezmoi/shell/filesystem proofs passed.
- [ ] Proof results incorporated, strict validation rerun, and renewed product/security approval recorded.
- [ ] New CLI, including exact-candidate apply and supported recovery, is the only reachable CLI.
- [ ] Lua configuration and gopher-lua removed.
- [ ] Config CRUD and custom drift engine removed.
- [ ] Old active markers and transaction formats removed from runtime.
- [ ] go-git and automatic commit workflow removed.
- [ ] Existing mise and chezmoi no-interference suite passed.
- [ ] Legacy state detected and preserved without interpretation or root override.
- [ ] README, OpenSpec project context, canonical specs, examples, and developer guidance replaced.
- [ ] No compatibility aliases, deprecated types, stale exports, or hidden fallback paths remain.
- [ ] Public and selected-private vertical smoke scenarios passed through both bootstrap and standalone commands.
- [ ] Exact-candidate idempotence, changed-candidate refusal, recovery, and failure-injection matrices passed.
- [ ] Secret and sensitive-metadata review passed.
- [ ] DeepReview and security-review findings resolved with evidence.
- [ ] Release artifact exercised end to end.
