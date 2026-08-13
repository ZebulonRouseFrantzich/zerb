---
description: Review branch changes against roadmap contracts and the current delivery gate
agent: build
subtask: false
---

<UserRequest>
  $ARGUMENTS
</UserRequest>

Changed files:
!`git diff main --name-only -- .`

## Review authority

Read these documents before reviewing code:

1. `docs/roadmap/product-contract.md` for `PC-*` public CLI, schema, digest, approval, output, and user-visible behavior.
2. `docs/roadmap/architecture.md` for `ARCH-*` trust, process, filesystem, root, cache, state, receipt, and recovery invariants.
3. `docs/roadmap/validation.md` for `VAL-*` support claims and required evidence.
4. `docs/roadmap/delivery-plan.md` to identify the current phase and the gates that phase admits.
5. `docs/roadmap/hard-cutover.md` for deletion, replacement, audit/reimplementation, dependency, and generated-artifact disposition.
6. `docs/roadmap/proof-results.md` only when a claim depends on recorded proof. Evidence is not normative authority, and an empty or incomplete record is not success.

If documents conflict, use the document that owns the contract and report the contradictory non-owning text. Phase 0 contracts are provisional; only Phase 1 evidence may freeze implementation. Do not assume planned behavior exists.

## Review procedure

1. Inspect every changed file and its relevant callers, tests, data formats, process boundaries, and deletion obligations relative to `main`.
2. Determine the current delivery phase. Report implementation that crosses an unmet proof, approval, review, or sequencing gate.
3. Map each finding to the most specific `PC-*`, `ARCH-*`, or `VAL-*` ID. If no ID covers changed observable behavior or an invariant, report the missing contract rather than inventing one.
4. Reconcile added, retained, moved, and deleted artifacts with `hard-cutover.md`. Report compatibility aliases, old-package imports, mixed routing, stale exports, deprecated paths, hidden fallbacks, unclassified dependencies, and generated residue.
5. Trace security-sensitive values and boundaries end to end:
   - canonical public source input and redirect/authentication suppression;
   - separation of candidate trust, private transport approval, and mutation approval;
   - absolute executable identity, argv, complete environment, working directory, and approved descriptors;
   - process group/session, parent-death, signal forwarding, grace/escalation, reaping, exit mapping, and orphan detection;
   - no-follow root topology, restrictive creation modes, initialization ownership, matching state/data identity, and untrusted cache handling;
   - lock ownership, journal persistence before mutation, exact dirty phase, pending receipt, stable commit, startup reconciliation, and truthful forward recovery;
   - active redaction of credentials, locators, names, provider values, rendered content, host facts, ambient values, and test markers.
6. Check deterministic schema and digest behavior: pre-decode bounds, strict unknown-field rejection, ambiguity/duplicate rejection, normalized paths and modes, sorted collections, versioned map-free encoding, and exclusion of approvals, observations, timestamps, and sensitive values.
7. Check errors, cancellation, concurrency, durability, boundary conditions, resource limits, and failure ordering. A refusal must occur before the boundary prohibited by the requirement.
8. Evaluate tests and proof records against observable contracts. Require focused tests for new behavior and exact failure transitions; do not reward mock/provider claims, source-text assertions, incidental defaults, or evidence that omits the pinned executable, fixture identity, argv/environment shape, process/access/write observations, result, and redaction.
9. Check maintainability and performance after correctness: simple control flow, explicit types, narrow interfaces, no avoidable allocation/copy/computation, causal error wrapping, and no needless abstraction.
10. Report only actionable defects introduced by or materially exposed by the branch. Verify each finding against the actual diff and code path before including it.

## Report format

Group findings by:

- **Critical** — trust, credential, mutation, state-corruption, recovery-truth, or release-gate failure that can cause unsafe execution or data loss.
- **High** — violated public/invariant contract, bypassed approval or isolation boundary, incorrect durable transition, unsupported support claim, or incomplete hard cutover.
- **Medium** — real correctness, resilience, test, performance, or maintainability defect with a concrete failure mode.
- **Low** — bounded quality issue worth fixing that is not stylistic preference.

For each finding provide:

- requirement ID and current delivery phase;
- file and precise symbol/line;
- concrete trigger and observed or logically necessary impact;
- why existing validation does not catch or justify it;
- the smallest contract-compatible correction;
- the focused verification or evidence needed to close it.

List no section for a priority with no findings. Keep the report concise and technical. Do not modify files. End with any blocking contract ambiguity or evidence gap; do not convert it into an implementation assumption.
