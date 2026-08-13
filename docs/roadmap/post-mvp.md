# Post-MVP Intent

**Status:** Non-normative, non-committed direction  
**Authority:** This document may preserve target-compatible intent, but it does not add commands, schema fields, support claims, or implementation scope. [Product contract](product-contract.md), [architecture](architecture.md), [validation](validation.md), and the current [delivery plan](delivery-plan.md) remain authoritative.

The MVP is deliberately limited to fresh bootstrap and exact-candidate reapply under [PC-071](product-contract.md#pc-071--fresh-bootstrap-and-exact-candidate-reapply-only). Items below require separate evidence, threat modeling, contract IDs, recovery rules, and approval after MVP acceptance. Their names describe user outcomes, not reserved CLI grammar.

## Source authoring and synchronization

A later release may help users create, validate, publish, fetch, and reconcile environment superprojects without making Git mechanics the public data model.

Any design must:

- preserve canonical public source identity, credential-free public locator rules, redirect policy, and the MVP selected-gitlink/private-transport exclusion unless a separately reviewed future contract replaces that boundary;
- keep source authoring separate from host application and from mutation authorization;
- make local, remote, and selected commit identities explicit rather than silently choosing a branch tip;
- define conflict ownership and recovery before offering pull, push, merge, or publish behavior;
- never create automatic commits as a side effect of routine profile or destination mutation; and
- never infer the active source from the current directory or an ambient Git remote.

Offline authoring is a valid future use case, but it must produce ordinary schema-versioned source content that the same static inspection and trust path can consume later.

## Private selected transport

A future release may consider selecting a private HTTPS or SSH gitlink, but the MVP reserves no mechanism, CLI flag, schema extension, helper, agent, relay, endpoint channel, or approval tuple for it. Current canonical HTTPS/SSH gitlink locators are only statically validated zero-access declarations when unselected; selection remains a policy failure.

Work may begin only after a new reviewed product contract, architecture contract, threat model, closed mechanism matrix, approval and redaction model, lifecycle/recovery rules, and real evidence prerequisites are approved. That future design must prove rejection before access for every unselected or ineligible mechanism; exact endpoint and object identity; credential isolation; helper/agent/process/socket/relay identity and race resistance if any such mechanism is proposed; complete environment/descriptor/network confinement; cancellation and hard-death cleanup; durable-state exclusion of credential material; and installed-binary verticals before any support claim. No Phase 1 or Phase 5 MVP result can be reused as positive transport evidence.

## Explicitly owned profile updates, switching, and removal

A later release may support changing an installed candidate, switching profiles, shrinking tool/target sets, or deliberately abandoning an installation. This work begins only after ownership and failure-recovery proofs cover both additive and destructive transitions.

The future design must distinguish:

- ZERB-owned artifacts recorded by a stable receipt;
- pre-existing or externally modified targets;
- additive installs still referenced by another candidate;
- removals that are safe, removals that require explicit per-item approval, and removals that remain prohibited;
- forward recovery from interruption at every destructive boundary; and
- a truthful terminal state for deliberate abandonment that does not claim the host is clean or rolled back.

Switching must use explicit receipt-governed ownership and fail closed on ambiguity. It must not use last-writer-wins composition, implicit overwrite, broad directory deletion, or manager uninstall against a pre-existing installation. A changed profile remains unsupported until this ownership contract exists.

## Optional encrypted profiles

Whole-profile or selected-file encryption may be considered when users need confidentiality. It is independent of any future private selected transport design and cannot create or imply a transport mechanism in the MVP.

Before support, a design must prove:

- ciphertext and metadata schema, recipient identity, authenticated encryption, key rotation, and multi-recipient behavior;
- key bootstrap on a new machine without placing private keys, passphrases, or recovery material in ZERB arguments, state, receipts, logs, diagnostics, or repositories;
- protected in-memory and temporary-file handling, cancellation cleanup, destination modes, and marker scans;
- disaster recovery and revocation without an undecryptable silent failure mode; and
- how decrypted native mise/chezmoi inputs enter the existing digest, trust, rendering, and dirty-state lifecycle.

## Source-validation hooks

Optional local or CI validation may check source schema, canonical paths, declaration allowlists, duplicate tool/target identities, object limits, repository hygiene, and likely credential leakage before publication.

Hooks must be advisory defense in depth, not the runtime trust boundary: Git hooks can be absent or bypassed, so ZERB must repeat all required validation while consuming a source. A future design must define installation ownership, upgrades, user customization, deterministic output, false-positive handling, and secret-scanner data exposure. It must validate schema-versioned TOML and native source trees.

## Additional platforms

macOS, Windows, additional Linux distributions, and additional CPU architectures are candidates only after platform-specific proof covers:

- protected-root ownership, link and ancestor checks, restrictive creation, atomic replacement, locking, and directory durability;
- process groups or equivalent job boundaries, signal/cancellation behavior, parent death, descendant reaping, and shell discovery;
- executable identity and native Git authentication mechanisms;
- path, Unicode, case-sensitivity, mode, and reserved-name semantics;
- mise/chezmoi and provider behavior for every claimed tuple; and
- the complete installed-binary vertical acceptance scenario.

A platform is removed from the claimed matrix when an invariant cannot be met; detection-only support and best-effort security are not alternatives.

## Stronger execution surfaces

Future execution surfaces may include explicitly opted-in shell startup configuration, project-scoped environment export, security containment beyond the required lifecycle boundary, or bounded deliberate detachment.

The MVP process boundary remains governed by [ARCH-009](architecture.md#arch-009--external-process-primitive) and [ARCH-010](architecture.md#arch-010--lifecycle-policy-by-process-class). Cgroup or platform-equivalent job-object integration is deferred only when the candidate process/platform tuple that would require it is rejected or removed from the MVP support matrix. If retaining a tuple requires that stronger boundary to satisfy ARCH-010, its implementation, proof, and freeze are Phase 1 requirements and cannot be deferred.

Any future user-facing surface must preserve parent-shell non-mutation and the frozen environment matrix. It must specify descriptor inheritance, terminal control, process/session topology, signal forwarding, cancellation grace and escalation, descendant reaping, hard-death behavior, environment export lifetime, manager credential/state exposure, and durable diagnosis. Arbitrary user commands continue to have user authority; stronger lifecycle control must not be described as a security sandbox unless a separately reviewed containment design proves that claim.

Behavior excluded rather than deferred is recorded in the [hard-cutover crosswalk](hard-cutover.md#former-authority-requirement-crosswalk). See the [roadmap index](README.md) and [redacted proof ledger](proof-results.md) for authority and evidence boundaries.
