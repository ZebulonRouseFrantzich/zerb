# ZERB Roadmap

**Status:** Proposed direction; not yet an approved OpenSpec change  
**Last updated:** 2026-08-08

## Product statement

**ZERB is a declarative, profile-based developer-environment restoration tool for mutable workstations.**

ZERB converges declared development tools and dotfiles. It is not an operating-system image, backup system, package manager, transactional rollback engine, or hermetic build system.

ZERB is the project name, not an acronym or backronym.

## Authority and approval

These documents record the agreed product direction and implementation roadmap. They are planning artifacts, not current behavioral specifications. Until the implemented cutover is applied and archived, `openspec/specs/` remains the description of current behavior. Architecture owns invariants, delivery owns phase ordering, validation owns evidence, and hard cutover owns old-to-new disposition; repeated summaries defer to the approved OpenSpec change.

This roadmap introduces a breaking architecture and security model. Phase 0 creates and initially approves a formal OpenSpec proposal, design, tasks, and delta specs only to authorize disposable Phase 1 proofs; it does not authorize product implementation or edit canonical specs. After the proofs, every result MUST be incorporated, strictly revalidated, and receive renewed product and security approval before Phase 2. The active `add-config-remove-command` change conflicts with this direction and MUST cease to be active with an explicit superseded/incomplete record; its deltas MUST NOT be applied as shipped behavior.

## Non-negotiable decisions

| Area | Decision |
|---|---|
| Name | Use `ZERB` without an expansion. |
| Product boundary | Restore a declared developer environment on a mutable workstation. |
| Language | Keep Go. Do not rewrite the project in Rust. |
| Delivery | Make a hard cutover. Do not preserve the old CLI, Lua format, state model, or compatibility aliases. |
| Profiles | MVP composition is exactly `base + one selected profile`. |
| Sources | A public superproject contains the manifest and public profiles; one selected private profile MAY be referenced by a Git submodule gitlink. Metadata-hidden local source discovery is post-MVP. |
| Tool declarations | Use native mise TOML and exact versions for an approved backend matrix. |
| Dotfile declarations | Use native chezmoi source entries and trusted templates. |
| Composition | Build one protected effective mise context and one protected effective chezmoi source. Reject collisions; do not silently override. |
| Existing mise | ZERB orchestration never executes, configures, activates, uninstalls, or writes state belonging to a pre-existing mise installation. Trusted candidate code runs with user authority. |
| Existing chezmoi | ZERB orchestration never executes, configures, initializes, updates, or writes state belonging to a pre-existing chezmoi installation. In the MVP, every pre-existing candidate target not owned by the stable ZERB receipt is a non-overridable conflict. |
| Managed binaries | Invoke ZERB's pinned mise and chezmoi executables by absolute path with ZERB-owned config, data, cache, and state. |
| Shell behavior | Do not call `mise activate` in the user's current shell. MVP uses command-scoped execution and a clean child shell. |
| Git | Use a constrained native Git policy for approved credential-helper, SSH-agent, transport, and object-fetch behavior without modifying global Git configuration or running worktree checkout semantics. |
| Authentication | ZERB does not accept, copy, log, or persist authentication secrets. Approved native Git/SSH channels perform authentication under their own storage policy. |
| Secrets | Store provider references, not resolved values. Private Git access is authorization, not encryption. |
| State | Host mutation is non-atomic. Journal `prepared`, `mutating`, and `dirty` truthfully; atomically commit one `stable/current` record. |
| Receipts | Record immutable observed facts and separate repeatability dimensions; do not claim machine snapshots. |
| Updates and switching | MVP permits fresh bootstrap and exact-candidate reapply only. Candidate changes, updates, and in-place profile switching are post-MVP until ownership and recovery are proven. |
| MVP platform | Linux only, with an explicitly tested architecture and distribution matrix. |

## MVP outcome

Starting with an installed ZERB binary, an approved native Git executable, and user-controlled authentication/provider access, one invocation can:

1. Acquire and verify ZERB's pinned mise and chezmoi binaries into protected storage.
2. Fetch a public environment superproject without recursive submodule checkout.
3. Select `base + one profile`.
4. Display and obtain authorization for the effective private transport endpoint before invoking credentials, then fetch only the selected private Git object if needed.
5. Statically validate and safely materialize selected objects without checkout filters/hooks.
6. Compute and obtain digest-bound trust for the candidate and execution policy.
7. Use the isolated managed adapters to produce the semantic plan without displaying secret-bearing content.
8. Confirm mutation separately.
9. Install exact supported tools into ZERB-owned storage.
10. Apply one effective chezmoi source using isolated state.
11. Verify convergence and atomically commit the stable receipt/current record.
12. Run commands or enter a clean child shell using the restored profile without changing the parent shell's mise activation.

The invocation MAY prompt for private transport authorization, authentication, candidate trust, mutation confirmation, or provider unlock. Target ownership conflicts are not overridable in the MVP. One command does not mean unattended operation.

The initial MVP supports fresh bootstrap and exact-candidate idempotent reapply for profiles with static or versioned-provider inputs. Profiles with dynamic-unversioned provider outputs or host-fact-dependent templates are bootstrap-only and refuse reapply. A changed source commit, candidate digest, profile ID, declaration, tool, target, managed-target set, adapter version, policy, or provider generation token is an unsupported update and fails before mutation.

## Document index

- [Architecture](architecture.md): product boundaries, profiles, isolation, composition, trust, state, and receipts.
- [Delivery plan](delivery-plan.md): phases, gates, dependencies, orchestration, and definition of done.
- [Validation](validation.md): integration matrix, no-interference proofs, failure injection, and release evidence.
- [Hard cutover](hard-cutover.md): old-to-new inventory, CLI replacement, dependency changes, and legacy-state policy.

## Phase summary

| Phase | Outcome | Mutation permitted? |
|---|---|---:|
| 0. OpenSpec reset | Initially approved breaking-change proposal authorizing proof work only; conflicting plans closed | No product mutation |
| 1. Proof spikes and contract freeze | Current pinned Git, mise, chezmoi, shell, and filesystem assumptions proven; proposal strictly revalidated and reapproved | Disposable fixtures only |
| 2. New foundation | New source/profile, isolated execution, state, recovery, and receipt foundations on a cutover branch | Test roots only |
| 3. Source-bound structural plan | Adapter acquisition, explicit source binding, digest trust, semantic `plan`, and `doctor` | Protected toolchain/candidate/cache state only |
| 4. Public bootstrap | Fresh public profile bootstrap, exact-candidate reapply, tool install, dotfile apply, verification, recovery, and receipt | Yes |
| 5. Private profiles and providers | Selected-only private source authentication and bounded provider flow | Yes |
| 6. Hard-cutover release | Old architecture removed, documentation replaced, Linux acceptance matrix passed | Yes |
| 7. Post-MVP convergence | Precisely owned additive and shrinking updates, profile switching, additional platforms, and authoring workflows | Yes |

Each phase is gated. A later phase MUST NOT compensate for an unproven earlier contract.

## What this roadmap supersedes

The intended future architecture supersedes these planning assumptions:

- `zerb.lua` and executable Lua configuration.
- Generated timestamped Lua snapshots.
- `.zerb-active` and `zerb.active.lua` as machine truth.
- `zerb config add/list/remove` as the profile-authoring model.
- Custom three-way drift resolution as a second tool-state engine.
- Automatic Git commits for routine configuration mutations.
- Pure-Go Git as a product constraint.
- Parent-shell `mise activate` integration.
- Claimed automatic rollback across multi-tool mutation.
- “Reproducible builds,” “complete isolation,” and “machine snapshot” language.

Existing user data MUST be preserved but MUST NOT be silently reinterpreted. See [Hard cutover](hard-cutover.md).

## Go versus Rust

Go remains the correct implementation language. The hard work is process isolation, native Git orchestration, deterministic composition, filesystem safety, state journaling, and recovery semantics. Rust would increase rewrite cost without preventing incorrect operation ordering, false state claims, target deletion, metadata leakage, or unsafe template execution.
