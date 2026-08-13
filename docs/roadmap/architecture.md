# Target Architecture

## Authority and status

This document is the sole normative owner of ZERB's trust, process, protected-filesystem, and durable-state invariants. [Product contract](product-contract.md) owns public CLI grammar, source and profile schema, digest serialization, approvals, output, and exit behavior. [Validation](validation.md) owns support claims and proof evidence. [Delivery plan](delivery-plan.md) owns sequencing.

Every invariant below is a provisional Phase 0 proof baseline. Phase 1 may narrow an invariant when real evidence disproves an assumption; Phase 1 then freezes the resulting implementation contract and may mark a finite proof candidate `implementation-qualified` before product code begins. That status is not evidence that a platform, adapter feature, authentication mechanism, shell, or parser is supported; support requires VAL-001 and the applicable Phase 6 installed-binary vertical.

## Numbered invariants

### ARCH-001 — Product boundary

ZERB owns explicit source/profile selection, canonical candidate construction, trust and authorization boundaries, orchestration, protected state, recovery, and receipts. Native Git owns transport and object semantics; mise owns supported tool resolution and installation; chezmoi owns supported source-to-target mapping, rendering, application, and verification.

ZERB MUST NOT become a second package manager, dotfile language, Git implementation, or general execution sandbox. It MUST NOT search `PATH` for managed Git, mise, or chezmoi executables, invoke them through a shell, modify the parent process environment, add shell-startup activation, recursively fetch unselected repositories, claim atomic host mutation, or claim rollback where it implements forward recovery only.

### ARCH-002 — Explicit source identity

Every operation that inspects a superproject is bound to the explicit canonical credential-free source defined by PC-020. ZERB never infers a source from the current directory, an ambient remote, or durable hidden defaults. The `source_id` is the PC-022 versioned digest of that canonical URI; fetched commit and object IDs are separate identities and availability is a separate observation.

Source identity includes the canonical source ID, superproject commit, selected embedded logical profile ID, component path, and tree digest. Statically validated unselected gitlink declarations are not candidate inputs and are never materialized; availability of the public identified objects remains separate from identity.

### ARCH-003 — Profile composition identity

The only MVP composition is `base + selected`, with each component present exactly once and both components embedded in the public superproject. A manifest may declare unselected gitlinks under PC-021, but selecting any gitlink fails before authentication, network, candidate creation, manager execution, or mutation. There is no implicit profile stack, transitive inheritance, recursive submodule traversal, machine-specific merge graph, override precedence, or arbitrary apply-time component path.

Duplicate tool identities, duplicate raw home-source paths, duplicate normalized paths, duplicate targets in PC-031's normalized structural inventory, unsupported tree entries, and nested gitlinks fail before candidate acceptance. ZERB preserves approved file bytes, normalized `/` paths, and ordinary-file executable modes; it does not reinterpret authored chezmoi filenames or implement target mapping itself.

### ARCH-004 — Candidate identity

A candidate is an immutable protected artifact binding the canonical credential-free public source, embedded logical profile/component identities, materialized public trees, exact authored tool versions and required lock values, exact adapter and policy identities, approved declaration/template surfaces, the deterministic structural target inventory, and destination revalidation rules. Every identity field exists before trust; live destination observations are not identity fields. A candidate never contains a gitlink/private locator, selected-gitlink object, transport identity, authentication value, provider value or reference, host-fact value, rendered content, mutation approval, or live destination observation.

Candidate and policy identity uses PC-050's versioned, map-free, deterministic JSON encoding and `sha256:<lowercase-hex>` digest. Collections are normalized and sorted before encoding. Unsupported objects or modes fail before hashing. A change to a bound source object, locator, profile/component identity, declaration, authored version/lock, adapter executable, policy, structural target inventory, or canonical input produces a different candidate and requires new trust; no post-trust result may mutate the digest. Live destination observations occur only after trust and mutation authorization under `A-X+W-X`, persist as journal/receipt preconditions and results, and are revalidated at every affected mutation boundary. Exact-candidate reapply is governed by PC-071.

### ARCH-005 — Distinct trust boundaries

Candidate trust and mutation authorization are the only MVP approval capabilities defined by PC-030 through PC-033. Pre-candidate-trust work is limited to verified adapter acquisition, quota-bounded credential-free public transport, safe public object parsing/materialization, schema and allowlist validation, canonical digest construction, and PC-031's one exact structural-adapter exception: verified chezmoi may run only its Phase 1-proven read-only, non-rendering, machine-readable target mapping over the protected embedded effective source. Selected gitlinks and private transport are rejected before this work and never become approval capabilities.

That mapping invocation uses the exact bound executable, argv, complete environment, descriptors, working/config/cache/state roots, and synthetic non-real destination root. It executes no template, helper, provider, host-fact lookup, script, external, or candidate content; accesses and writes no real destination; and emits no field beyond normalized structural targets. If the upstream/version cannot prove all of this, the candidate fails and the surface is removed; there is no custom fallback mapper. Otherwise, before digest-bound candidate trust ZERB MUST NOT invoke candidate-selected mise backends, hooks, tasks, plugins, sourced environments, or any other chezmoi mode; evaluate templates; read profile-selected host files; install declared tools; access a real destination; or mutate destinations. Rendering begins only after trust. Real-destination access and live observations begin only after mutation authorization under `A-X+W-X`. The Phase 1 disposable validator receives embedded fixtures directly and rejects provider/host-fact inputs before candidate creation. Selected gitlinks fail earlier under ARCH-007. Mutation authorization grants no new content trust or source eligibility.

### ARCH-006 — Native Git executable and policy

ZERB resolves one Phase 1 implementation-qualified native Git executable from the fixed ARCH-032 proof/release manifest, canonicalizes it, verifies and records its identity, and thereafter invokes that absolute path with argument arrays. It never writes system or global Git configuration and never interpolates a Git command into a shell.

Each invocation starts from a phase-complete environment and fixed configuration policy. Public fetch disables authentication, proxy use, and redirects. Source operations disable system, global, and repository configuration except audited fixed entries; neutralize hooks, init templates, attributes filters, checkout filters, custom update commands, URL rewrites, protocol overrides, pagers, editors, askpass, arbitrary helpers, and unsafe remote helpers; reject unapproved protocols; and avoid worktree checkout. An authentication challenge during public fetch fails rather than consulting ambient credentials.

Every fetch runs in a fresh owner-validated staging subtree under the Phase 1-frozen transfer-byte, on-disk-byte, object-count, per-object expanded-byte, total-expanded-byte, graph-depth, tree-depth, entry-count, process-output, wall-time, CPU, memory, and process-count ceilings. The boundary uses proven filesystem quota or equivalent confinement plus process resource limits and a watchdog around native Git; post-fetch plumbing output is separately bounded. Exceeding any ceiling fails closed and durably removes only the owned staging subtree. If the approved Git/platform tuple cannot enforce a ceiling before resource exhaustion, that tuple or source surface is removed before freeze.

### ARCH-007 — Selected-gitlink and private-transport boundary

The MVP has no private transport authority. Selection of any gitlink and every private HTTPS/SSH source fails before authentication, network, candidate creation, manager execution, or mutation. No caller input, policy, manifest field, environment value, or candidate may enable a credential helper, askpass program, SSH agent/socket, identity/key file, known-hosts input, credential store, proxy, synthetic SSH configuration, relay, private endpoint, peer-authentication control, mechanism tuple, or transport approval.

Static validation of canonical HTTPS/SSH locators belonging to unselected gitlinks is syntax and manifest consistency checking only. It performs no DNS or endpoint resolution, Git invocation for the gitlink, authentication, helper/agent access, network access, candidate construction, object fetch, or materialization. Any implementation path that can produce a typed identity, credential channel, private endpoint spec, or authorization digest violates this boundary.

### ARCH-008 — Safe Git object materialization

Git fetches the canonical public superproject into a fresh protected, quota-bounded staging object store. ZERB validates the manifest, `.gitmodules`, unselected gitlink locators/object types/paths, and selected embedded path before candidate creation. It materializes only the embedded base and embedded selected component through bounded no-checkout object plumbing. Selecting any gitlink fails before Git or network is invoked for that gitlink. Materialization admits only approved directories and ordinary files with normalized executable or non-executable mode and rejects missing public objects, nested gitlinks, symlinks, devices, aliases, duplicate normalized paths, absolute or traversal paths, unsupported modes/types, hook/filter-dependent content, and every ARCH-006 resource-limit violation.

Every object and tree walk is bounded before allocating or emitting normal output. Full public-object authentication and selected embedded-tree hashing precede promotion from owned staging to protected source/tree storage. A failed, interrupted, oversized, slow, or resource-exhausting public fetch is never promoted; cleanup is confined to the verified staging root. Unselected gitlinks are never fetched. ZERB never accepts, copies, logs, or persists credential bytes.

### ARCH-009 — External-process primitive

Every external process specification contains an identity-checked absolute executable, argument array, working directory, complete constructed environment, approved standard streams, an explicit descriptor allowlist, and a process class. The runner never invokes a shell or mutates an ambient environment in place.

All ZERB-created descriptors are close-on-exec from creation. Every process is launched through a Phase 1-proven same-binary FD-only supervisor protocol: the parent passes only declared streams and fixed control/ready descriptors; the supervisor performs a parent-identity check, installs its parent-death action, sanitizes the child descriptor table with a race-free close-range/allowlist primitive, establishes the frozen group/session/subreaper topology, rechecks parent identity, and acknowledges readiness before candidate code is released. Concurrent parent descriptor churn cannot add a child descriptor. Death before or during setup causes the unreleased supervisor to exit; death after readiness triggers the proven managed-boundary termination path.

For `exec`, a bare name is resolved only against the already-constructed stable-profile `PATH`, whose entries are canonical absolute owner-validated directories with no empty, relative, duplicate, or writable-unowned component. A slash-containing input must be canonical absolute. Ambient `PATH` and cwd never resolve it. Resolution opens and verifies the final executable and executes that same identity through the Phase 1-frozen fd-based or equivalently race-resistant boundary; symlink, inode, digest, owner, type, mode, or path substitution fails before execution.

The production runner MUST NOT rely on `exec.CommandContext` or equivalent kill-the-leader behavior as its cancellation policy. It owns the supervisor/group/session setup, parent-death behavior, signal forwarding, grace and escalation, complete waiting/reaping, and orphan detection required by ARCH-010.

### ARCH-010 — Lifecycle policy by process class

Phase 1 freezes, for native Git, acquisition/extraction, mise and backend descendants, chezmoi/template descendants, `exec`, and `shell`, the exact Linux supervisor, process group/session/subreaper model; forwarded signal set; catchable-signal and context-cancellation grace period; escalation signal and timing; leader and descendant reaping; exit mapping; and orphan detection.

Candidate managed versions MUST be proven not to daemonize, call `setsid`, or otherwise escape the controlled boundary. The FD-only supervisor and direct-child parent-death protocol are mandatory for every retained class, not optional evidence: Phase 1 hard-kills the wrapper before and after clone, descriptor sanitization, parent-death setup, group/session setup, readiness, exec, and first descendant creation. Any uncovered race removes the process/platform tuple unless a separately proven cgroup boundary closes that race cumulatively around the mandatory supervisor; a cgroup never replaces the supervisor. Catchable cancellation and wrapper hard death of a managed class leave no managed descendant; durable startup state diagnoses interrupted mutation.

For a normally exiting `exec` or `shell` child, PC-015 owns status propagation. Catchable wrapper signals are forwarded under the frozen policy, followed by escalation and reaping, and return the PC-015 signal-derived status. Context cancellation without an associated POSIX signal uses the same termination/reaping policy and returns the operational-failure status. An uncatchable wrapper death has no return. Deliberate detachment by arbitrary user payloads is outside orchestration only if PC-015 states that limit and validation proves ZERB supplies no manager credentials or manager-state paths to the detached process.

### ARCH-011 — Complete process environments

Unlisted variables and descriptors are absent. Phase 1 freezes the exact variable allowlist, replacement value, working directory, `HOME`/XDG roots, executable identity/search rule, network controls, prompt controls, and descriptors for every row below and proves the policy for descendants, not only leaders.

| Process class | Required roots and inputs | Explicitly absent or replaced |
|---|---|---|
| Native Git public fetch/materialization | Protected quota-bounded source/object staging; synthetic controls; minimal approved locale/terminal data | Ambient credentials, helpers, agents, proxy and askpass inputs; inherited `GIT_*`/`SSH_*`; arbitrary `PATH`; config, object, hooks, filters, pager, editor, protocol, and URL-rewrite controls |
| Binary acquisition/extraction | Fresh protected staging root and synthetic home/XDG roots; approved direct source, CA, and verification inputs | Manager/profile/provider variables, ambient secrets, proxy, arbitrary executable lookup, user configuration and state |
| Mise resolution/install and descendants | Protected candidate plus ZERB-owned home, config, data, cache, state, install, download, plugin, shim, and discovery roots; only backend channels frozen for the supported tuple | Inherited `MISE_*`, `RTX_*`, `ASDF_*`; user/system/global/local discovery; hooks, tasks, sourced environment, ambient shims/secrets, auto-install, plugins, idiomatic files, and unapproved network controls |
| Chezmoi mapping/apply and static-template descendants | Protected effective source/runtime config/cache/state/working tree, explicit real destination, and only approved authored non-secret data | Existing chezmoi discovery/state, ambient shims/secrets, host facts, provider channels, pager/editor, scripts/externals, arbitrary helper execution, and filesystem lookup |
| `exec` payload | Real user home, identity-checked payload, and explicit ZERB-produced stable tool environment; user streams and command arguments | Ambient lookup and manager controls are replaced with the frozen stable environment; ZERB manager credentials/state paths are not supplied |
| `shell` child | Real user home, approved identity-checked canonical shell selected by explicit ID, and explicit ZERB-produced stable tool environment | `$SHELL` selection, normal interactive startup files, ambient manager hooks/controls, and ZERB manager credentials/state paths |

Authentication, rendered-content, and ambient markers prove the policy. Provider markers are prohibited because the MVP has no provider channel. Approval of one channel never makes a different ambient channel inheritable.

### ARCH-012 — Managed binary acquisition

ZERB acquires exact Phase 1 implementation-qualified mise and chezmoi artifacts into protected staging, verifies full SHA256 plus the Phase 1-frozen signature/provenance contract, verifies the extracted executable identity, and atomically promotes it into ZERB-owned toolchain storage. Managed operations use only that absolute executable identity and never `PATH` selection.

Acquisition is ZERB-controlled input processing, not candidate trust. A cache hit remains untrusted until reauthenticated and rehashed. An upstream version or artifact change requires renewed executable identity and affected proof evidence.

### ARCH-013 — Mise adapter isolation

Mise owns its native declaration/backend/install semantics. ZERB owns the implementation-qualified exact-version, exact-authored-lock, and declaration allowlists, verified binary, deterministic `base + selected` staging, isolation environment, invocation ordering, failure journaling, and observation of requested versus installed versions.

Every mise invocation and descendant stays within the ARCH-011 roots and uses explicit inspect, install, and exec surfaces proven for the pinned release. It must not discover user/system/global/project manager state, use settings or lock-generation commands that write configuration, execute tasks/hooks/plugins/custom backends/sourced environments, or fall back to ambient installs/shims. Duplicate tool identities fail rather than using mise override behavior. Aliases, ranges, channels, `latest`, missing exact locks, and any post-trust resolution or lock mutation fail before manager execution. Mise may verify an exact candidate-bound authored version/lock but cannot add identity to a trusted candidate.

Phase 1 must prove every configured path and discovery control, including data, config, cache, state, installs, downloads, plugins, shims, global/system config suppression, filename/ceiling controls, hooks, auto-environment, and auto-install. These upstream variables are version-specific interfaces, not assumed guarantees.

### ARCH-014 — Chezmoi adapter isolation

ZERB materializes one protected effective source from `base + selected`. For candidate construction it invokes the verified chezmoi executable only through PC-031's Phase 1-proven read-only, non-rendering, machine-readable target-mapping mode, with explicit protected source/config/cache/persistent-state/working roots and a synthetic non-real destination root. For post-authorization rendering/application it invokes the same verified identity with the explicit roots and real destination required by the frozen contract. It does not run repository initialization/update, commit, push, sequential independent-source application, or any settings path that can adopt ambient manager state.

Pre-trust validation admits only the Phase 1-frozen ordinary/private/executable entries and static template/data syntax from the embedded components. Provider references, host facts, named-template expansion unless explicitly retained at freeze, scripts, externals, authored removal, source-root controls, arbitrary filesystem functions, and command/output functions fail before candidate creation. A selected gitlink fails before effective-source assembly. The sole pre-trust chezmoi execution then derives only normalized structural targets: exact argv/environment/descriptors/roots are frozen and proven, templates/helpers/providers/host facts/scripts/externals and candidate content do not execute, and the real destination is neither accessed nor writable.

### ARCH-015 — Provider and host-fact exclusion

Secret-provider retrieval, provider-backed rendering, and host-fact-dependent templates are outside the MVP. In the Phase 1 disposable harness, the static validator receives an embedded component directly and rejects every provider reference, provider function, host-fact function, dynamic lookup, and unapproved template function before candidate creation. Selected gitlinks fail even earlier at the source-selection boundary, before authentication, network, materialization, provider processing, render, manager install, or mutation. Candidates and durable records contain neither provider references nor values.

Phase 3 repeats the source-specific boundary through `internal/app.Run`; Phase 6 repeats it through the installed binary. Future provider support requires a new product contract and real evidence. In particular, a versioned lookup is eligible only when its immutable generation selector is authored and statically knowable before trust; a provider that requires live pre-trust token discovery is ineligible rather than prefetched or late-bound.

### ARCH-016 — Durable root namespace

ZERB resolves data, cache, and state roots once from the real environment and user home according to the frozen Linux XDG contract. It exposes no ZERB-specific root override and creates or reads no ZERB XDG config root in the MVP.

The resolved `data/zerb`, `cache/zerb`, and `state/zerb` roots must be canonical, pairwise distinct, and pairwise non-aliasing/non-containing: none equals, contains, or descends from another by canonical path, descriptor-relative ancestor identity, device/inode identity, mount identity, or bind-mount relation. Duplicate or conflicting XDG inputs and any alias/nesting are rejected before creating a product path. `internal/paths` revalidates this relation descriptor-relatively before every protected-root or cache use; cache-owned paths can never contain durable state or data.

The protected layout is exactly:

```text
data/zerb/
  owner.json
  toolchain/
  sources/
  trees/
  installs/
cache/zerb/
  instances/<instance-id>/
    owner.json
    downloads/
state/zerb/
  root.json
  init.lock
  admission.lock
  writer.lock
  writer-owner.json
  candidates/<digest>.json
  journals/<operation-id>.json
  receipts/<digest>.json
  current.json
```

State/data directories and the per-instance cache directory are created as `0700`; synchronization inodes, owner publications, markers, candidates, journals, receipts, and current state are `0600` from creation, independent of umask. State and data share one cryptographically random 128-bit lowercase-hex instance ID. Cache uses that instance ID only as a namespace marker and remains untrusted. The three lock paths are stable synchronization inodes, not ownership records: they are synced once, never replaced or deleted, and are accepted only with the Phase 1-frozen owner/type/mode/link/device/inode checks. `writer-owner.json` is the separate atomically replaceable ownership publication.

### ARCH-017 — Exclusive root initialization

Clean first use is exactly the PC-060 predicate: after ancestor/owner/type/mode/no-follow checks, each product state/data directory is absent or an empty directory and neither root contains a recognizable synchronization name, reserved durable path, marker, claim, domain tag, or record. Mixed absent/empty pairs are permitted. Any non-empty root is classified by ARCH-018 instead.

Initialization uses a stable `state/zerb/init.lock` synchronization inode, created no-follow as `0600`, file-synced and parent-synced, never replaced or deleted, and held with the Phase 1-proven Linux open-file-description exclusive lock. All contenders open and validate the same inode; the descriptor is close-on-exec, is never supplied to a child, and kernel release on process death is part of the supported platform/filesystem tuple. The lock holder re-reads both roots, then atomically writes, file-syncs, renames, and parent-syncs schema-1 `root.json` as `initializing` with a random instance ID, fresh random ownership token, PID, and process-start identity.

Only the holder whose token and initialization-lock inode still match may create, file-sync, and parent-sync the stable `admission.lock` and `writer.lock` inodes; atomically publish and sync `writer-owner.json` as released generation zero; create and sync the matching data and cache markers; and finally atomically replace, file-sync, and parent-sync `root.json` as initialized. It reopens and verifies every publication before advancing. A loser writes no marker, synchronization inode, owner publication, or product data.

After proving the recorded process identity dead, a successor first acquires the same exclusive `init.lock`, revalidates its inode and all root topology, compares the persisted instance/token/status and record inode, atomically publishes and syncs a new random takeover token plus its own process identity, reopens and verifies that publication and the stable lock inode, and only then continues. The lock serializes simultaneous takeover; the old token is never authority. Failure before the synced token replacement leaves the old claim; failure afterward leaves the new claim. If lock semantics, identity proof, inode/token compare-transfer, or a durability boundary cannot be proven on a platform/filesystem tuple, recovery remains non-mutating and that tuple is not supported.

### ARCH-018 — Durable root acceptance and corruption

Root classification uses PC-061's exhaustive first-match order after only the minimum safe topology work needed for the applicable ARCH-031 lease acquisition:

| Predicate over the complete state/data namespace | Classification | Admission and guidance |
|---|---|---|
| Both roots satisfy PC-060 absent-or-empty and neither contains recognizable evidence | `CLEAN` | ARCH-017 initialization only when the PC-016 command row permits; topology-only `doctor` remains read-only |
| Clean is false, at least one root is non-empty, and neither root contains recognizable evidence | `FOREIGN` | Fail with generic move-aside/manual-clean-start guidance; write nothing |
| Any recognizable record/object has an unsupported schema/domain | `BAD` unsupported/evidence-preserving | `doctor` only unless the current contract names an exact non-migrating repair; never rewrite/promote; no delete/move-aside advice |
| The namespace equals one exact crash-valid ARCH-017 initialization prefix and contains nothing else | `INIT` | `doctor` and exact `recover repair` initialization path only; preserve evidence; no delete/move-aside advice |
| Recognizable evidence exists but the namespace is neither an exact `INIT` prefix nor a complete supported current relation | `BAD` corrupt/evidence-preserving | `doctor` and only an exact supported repair/resume relation; no delete/move-aside advice |
| State/data owner markers and all synchronization inodes/owner publications are safe, initialized, supported, instance-consistent, and every other state/data entry belongs to exactly one allowed relation below | `READY(∅)`, `STABLE(A)`, `TRANSITION(A,B)`, or `CLEANUP(B)` | Exactly the PC-016 state/lease row |
| The durable relation above is current but the owned per-instance cache/marker is absent or poisoned | Same durable classification plus cache miss | Recreate only the validated per-instance cache when the admitted command permits; durable truth is unchanged |

Recognizable evidence includes every reserved ARCH-016 state/data path or filename and every ZERB schema/domain tag even when its object is empty, malformed, wrong-type, or from an unsupported version. Once any such evidence exists, an unrecognized extra entry anywhere under state/data is corruption rather than ignorable foreign content. If there is no recognizable evidence anywhere, all non-empty entries are foreign; this rule prevents content-dependent guessing.

`INIT` is closed, not an inference from `init.lock`: it permits only the safe permanent lock plus the supported, well-formed publications from ARCH-017 in exact order without a gap, all bound to the same instance/token/process claim, with no initialized/current/candidate/journal/receipt/install evidence or extra entry. VAL-025 enumerates every exact prefix at each before/after durability boundary. A malformed/unsupported publication, an initialized-root deletion, a substituted lock, or any evidence inconsistent with that prefix is `BAD`, even though `init.lock` persists.

A supported current relation is complete only when the owner pair, all stable synchronization inodes, released or exactly held writer-owner publication, and every present durable record cross-validate. `READY(∅)` has no current or transition. `STABLE(A)` has exactly one current reference whose candidate and completed receipt validate. `TRANSITION(A,B)` has exactly one permitted prepared/mutating/dirty journal graph. `CLEANUP(B)` is stable `B` plus only the exact related post-commit residue. Independently published and fully authenticated immutable candidates, source objects, materialized trees, and verified toolchain objects may be unreferenced; a journal, receipt, writer-owner publication, or current record may not. A dead-process `held` owner publication with no journal or possible-mutation evidence is an exact stale-owner relation on the otherwise complete state; ARCH-031 alone may repair it. Every other missing, extra, malformed, mismatched, unsafe, partial, or improperly unreferenced object is `BAD`.

### ARCH-019 — Untrusted cache

Only the owner-validated `instances/<instance-id>` cache subtree may be used. ZERB validates owner/type/mode and no-follow topology on every cache access and authenticates and hashes every byte before parsing, extraction, or execution. It never adopts, relabels, trusts, or deletes unrelated cache content. A missing or poisoned cache is a cache miss, not durable truth.

### ARCH-020 — Safe filesystem operations

All protected path traversal uses descriptor-relative or equivalently race-resistant no-follow operations and rejects symlinked, hardlinked where a single link is required, wrong-owner, wrong-type, and group/world-writable components. Creation establishes restrictive modes immediately. Staging and promotion use the same protected filesystem; durable replacement writes and syncs the file, atomically renames, and syncs the parent directory. Cleanup is confined to a verified owned staging subtree and never follows links.

Before any protected product/manager path creation or use, the filesystem layer enforces the full closed role-overlap matrix among ZERB state/data/cache/control paths, source/tree/toolchain/staging roots, and manager state/install roots. These protected roles must not equal, alias, replace, contain, descend from, or mutate metadata on one another; checks cover lexical/canonical paths, existing ancestor descriptors, device/inode and mount identities, bind aliases, and reserved future leaves. During candidate construction, home destinations receive only a no-I/O textual comparison of normalized structural target identities against that normalized reserved-role namespace; no real-home path is opened or observed. The candidate binds the target identities and full live revalidation policy. Only after candidate trust plus mutation authorization under `A-X+W-X`, and before any manager access that can mutate, filesystem creation, or external mutation, the filesystem layer opens the real destination and enforces every canonical/descriptor/owner/type/link/device/inode/mount/bind overlap check, repeating it at every affected boundary. Any ambiguity fails closed at its defined stage.

Schema, path, archive, composition, hashing, and emitted-output limits are checked before their corresponding allocation or decode. Native Git necessarily parses protocol and pack input internally, so ARCH-006's external quota/resource/watchdog boundary limits that untrusted parser before ZERB performs bounded object inspection. Unsupported aliases and object types fail rather than being normalized into a second semantic representation.

### ARCH-021 — Managed archive safety

A managed archive is fully authenticated and hashed before any parser receives its bytes. Extraction into a fresh protected root rejects absolute paths, traversal, empty or ambiguous names, normalized aliases, duplicate entries, symlinks, hardlinks, devices, sockets, unsupported types/modes, oversized entries, oversized totals, and entries outside frozen count/depth limits. ZERB verifies expected executable identity after extraction and before atomic promotion. Partial, corrupt, modified-after-hash, or interrupted artifacts never become visible as installed content, and cleanup cannot reach an outside sentinel.

### ARCH-022 — Destination ownership and races

The real home destination is an intentional shared surface. A ZERB writer lock coordinates only cooperating ZERB processes and makes no claim about another manager or arbitrary writer. Before mutation and again at each affected boundary, ZERB validates every ancestor and final target with no-follow owner/type/mode/link/content checks. The path must be reached through an unchanged chain of expected-owner non-symlink directories.

Every normalized structural destination is compared without I/O against the reserved-role namespace during static composition; this catches textual equality/containment and not-yet-created protected/control leaves such as an absent `current.json` without touching the real home. Full live role-overlap, ancestor, alias, mount, and inode checks begin only after trust and mutation authorization under `A-X+W-X`, before any manager or filesystem mutation, and are revalidated at each affected boundary.

A pre-existing target not proven to belong to the exact stable receipt is a non-overridable conflict. A user-modified managed target is never silently overwritten. Any observed substitution, rename, hardlink, symlink, type, content, or ancestor race prevents stable completion and leaves truthful dirty state. Automatic old-target deletion, force, overwrite approval, and in-place profile switching are outside the MVP.

### ARCH-023 — Writer ownership

Writer ownership is not pathname creation, record existence, age, or token possession. It is the conjunction of an exclusive ARCH-031 admission lease, the held exclusive open-file-description lock on the verified stable `writer.lock` inode, a fully durable `writer-owner.json` publication naming the same fresh token/generation/process-start identity, and successful inode/token/generation revalidation. Initialization, takeover, writer, and operation tokens are distinct. Every journal and write-capable operation binds the active writer generation/token. A process that cannot prove the complete conjunction cannot advance a journal, receipt, current record, cleanup, or external mutation.

Staleness requires acquisition of the kernel exclusion after the prior holder released or died plus proof that the published process identity is dead; age and PID alone never suffice. Release atomically publishes a new durable released generation while the locks remain held and never unlinks or replaces a synchronization inode. ARCH-031 owns acquisition, compare-transfer, ABA defense, release, lease order, and crash boundaries.

### ARCH-024 — Versioned durable schemas

Every candidate, marker, initialization or writer-owner publication, journal, receipt, and current record is schema/domain versioned and encoded from explicit map-free structs with sorted collections and no authentication, provider, rendered, or ambient values. Stable synchronization inodes contain no mutable ownership record. Identity-bearing sensitive metadata required by PC-050 remains exact in protected candidates/receipts rather than being omitted or replaced by a public redaction. Exact authored versions, lock values, and the deterministic structural target inventory are present in the candidate before trust; no post-trust manager or live-destination observation may alter them or any digest field. Live destination topology/type/content/link/mode/owner observations occur only after trust and mutation authorization under `A-X+W-X`; journals and receipts persist them as preconditions/results and every affected mutation boundary revalidates them. Phase 1 freezes exact JSON fields, enum values, byte/count/string limits, canonical serialization, replacement/sync protocol, and cross-record validation before implementation.

The durable relationships are:

| Record | Required identity and role |
|---|---|
| State `init.lock` | Stable no-follow exclusive initialization/takeover synchronization inode; no replace/delete lifecycle |
| State `admission.lock` | Stable no-follow shared/exclusive command-admission synchronization inode; no replace/delete lifecycle |
| State `writer.lock` | Stable no-follow exclusive writer synchronization inode; no replace/delete lifecycle |
| State `writer-owner.json` | Atomic released/held publication with schema, strictly advancing generation, random token when held, PID/process-start identity, acquisition/release facts, and predecessor relation under ARCH-031 |
| State `root.json` | Initialization status, schema, instance identity, current initialization ownership token, and claimant evidence |
| Data/cache `owner.json` | Schema and matching instance identity; cache marker confers namespace ownership only |
| Candidate | PC-050 digest plus exact PC-022/PC-043 source/locator/profile/component identities, policy/adapters, deterministic structural target inventory, and destination revalidation rules; no live destination observations |
| Journal | Operation identity/token, active writer generation/token, prior candidate `A`, intended candidate `B`, state/phase, and post-authorization live destination observations persisted as preconditions/results for forward recovery and boundary revalidation |
| Pending receipt | Candidate `B`, exact protected identity metadata, requested and observed non-secret facts including structural targets and post-authorization live destination observations, completed phases, and verification observations; existence alone is not completion |
| `current.json` | One atomic `stable(B)` reference to candidate and receipt; the sole receipt-completion authority |

### ARCH-025 — Prepared, mutating, dirty, and stable truth

Host mutation follows this durable sequence under the complete `A-X+W-X` ownership relation:

```text
stable/current(A)
  -> prepared(A,B)
  -> mutating(A,B,tool-install)
  -> mutating(A,B,home-apply)
  -> mutating(A,B,shell-metadata)
  -> pending receipt(B)
  -> stable/current(B)
```

`A` is the prior stable candidate or the explicit empty baseline on first bootstrap; `B` is the exact immutable candidate being applied.

Candidate `B` and its journal, including the active writer generation/token, are written and synced before mutation. `prepared(A,B)` is written and synced before the first possible external mutation. The exact `mutating(A,B,phase)` record is written and synced immediately before every call that may mutate. ARCH-031 ownership is revalidated before each publication and possible external mutation. Any abnormal exit after a possible mutation and before the stable/current commit is exposed on restart as `dirty(A,B,phase,journal)`. `A` remains the current completed candidate until one atomic, file-and-parent-synced `stable/current(B)` replacement.

Receipt `B` is fully written and synced as pending before the stable commit. Its existence does not complete it. After possible mutation, startup may expose only `{current=A, dirty(A,B,phase,journal)}` or `{current=B, stable(B)}`. An unreferenced pending receipt is orphan evidence. An interruption after the stable commit leaves stable `B` plus optional stale-journal cleanup, never crossed current/state truth.

### ARCH-026 — Receipts and repeatability

A protected receipt records only required non-secret facts: canonical source and component identities, candidate/tree digest, ZERB and exact Git/mise/chezmoi identities, schema/policy versions, observed platform tuple, authored tool requests and observed versions/integrity facts, managed target inventory, completed mutation phases, and post-apply observations. Public diagnostics and exported evidence redact or replace sensitive metadata and digest oracles under ARCH-028; protected identity remains exact.

Receipts report independent dimensions—inputs pinned, artifact integrity established, artifact currently available, host compatible, and convergence verified. They describe pinned inputs and observed convergence on a mutable host, never a reproducible build or atomic rollback.

### ARCH-027 — Command admission and forward recovery

PC-011 parsing precedes root access; after parsing, semantic root/state/candidate/executable/destination validation occurs only under the compatible ARCH-031 lease and after synchronization-inode/root revalidation. PC-016 is the exhaustive state-by-command table. No implementation may infer admission from a pre-lease snapshot, validate then acquire, drop a lease before publication or launch binding, or upgrade a shared lease.

Every `plan` uses `A-X` from before root/state/source/candidate validation through cache staging, trust, permitted immutable publication, synchronization, reopen verification, and cleanup. In `STABLE(A)`, planning may finish only when the recomputed candidate is exact `A`; a differing result is an unsupported update and fails before publication. `TRANSITION(A,B)` and `CLEANUP(B)` deny planning before source access. Mutation and recovery are exclusive. A prepared pre-mutation operation may be reconstructed or abandoned only by the exact Phase 1-frozen transition; no state is relabeled by inference.

`recover resume` and `recover repair` revalidate source objects, candidate/policy digest, trust, destinations and ancestor topology, disk/access preconditions, tool readiness, completed observations, writer ownership, and remaining phase inputs before supported forward convergence. Recovery persists each phase boundary exactly as a fresh apply would. It never edits journals manually, deletes unrelated data, claims rollback, converts an orphan receipt into truth, or marks an unverified host stable. When an external condition prevents convergence, dirty state remains and `doctor` reports the exact obstruction.
### ARCH-028 — Secrets, sensitive metadata, and redaction

ZERB durable records never contain authentication, provider, rendered, or ambient values. Provider references are invalid MVP input. Canonical credential-free public source and embedded logical profile/component identities are trust-critical metadata: protected candidates and receipts bind them exactly, while public output and exported evidence redact them. Gitlink locators may exist only in the public manifest/static-inspection input; they never enter a candidate or receipt.

Proof markers use a closed sink matrix:

| Marker class | Raw bytes allowed | Required absence |
|---|---|---|
| Authentication | Nowhere; the MVP has no authentication input, store, helper, agent, relay, private endpoint, or live credential channel | Every surface |
| Rendered content | Generated disposable source input/live render channel, intended owner-validated `0600` disposable destination, and in-memory comparison only | Authentication storage, ZERB durable/cache/log/evidence surfaces, unintended destination, output, capture, transcript, review, and repository |
| Provider | Nowhere; any occurrence fails because the MVP has no provider channel | Every surface |
| Ambient | Live poisoned-parent injection and in-memory comparison only | Every child channel and every filesystem, network, output, capture, transcript, review, evidence, and repository surface |

High-entropy generated rendered/ambient marker evidence may publish plain SHA256 fingerprints. A retained literal digest is otherwise permitted only after complete-preimage review classifies it non-sensitive or already fully redacted. Potentially enumerable candidates, trees, fixture revisions/digests, static locators, profiles, employers, providers, targets, and comparable metadata use fresh typed random per-proof references plus equality/change relations; raw sensitive digests and the reference mapping exist only in the approved live proof process and in-memory comparison and are never retained or reused. Selected-gitlink/private-transport evidence records only rejection and zero access, never a transport tuple, approval digest, credential recipe, identity producer, relay, or private endpoint. Errors redact structured sensitive metadata before formatting or causal wrapping.

### ARCH-029 — Conditional support

Linux is the implementation target, not an unconditional support claim. Phase 1 can mark only finite positive validation candidates `implementation-qualified` and negative rows `exclusion-proven`; neither authorizes private transport. A distribution/kernel/CPU/filesystem tuple, public native Git identity, shell, mise declaration/backend/package class, or chezmoi entry/template surface is supported only after VAL-001 accepts its complete Phase 6 installed-binary vertical and applicable focused rows. Selected gitlinks, private transport/authentication, provider, and host-fact surfaces are excluded, not provisional support. Non-orthogonal combinations are tested as combinations. A failed proof narrows or removes the candidate; it never creates a fallback.

### ARCH-030 — Replacement package and internal API seam

The replacement package set is exactly `internal/paths`, `internal/safefs`, `internal/source`, `internal/profile`, `internal/toolchain`, `internal/execenv`, `internal/mise`, `internal/home`, `internal/plan`, `internal/state`, `internal/recovery`, `internal/receipt`, `internal/host`, `internal/app`, and test-only `internal/testenv`. None may import, alias, wrap, re-export, register, or accept a type from the obsolete `internal/config`, `internal/service`, `internal/git`, `internal/drift`, `internal/chezmoi`, `internal/transaction`, `internal/shell`, `internal/platform`, `internal/binary`, or `internal/testutil` package trees. Retained algorithms are rewritten behind replacement-owned types; there is no bridge package or service locator.

The import seam is one-way: `cmd/zerb` may depend only on `internal/app` and production construction in `internal/host`; neither is imported by lower packages. `internal/app` owns parse-first dispatch/orchestration and depends on explicit replacement services. `internal/plan` owns candidate construction and consumes source/profile/adapter contracts without owning durable transition truth. `internal/state`, `internal/receipt`, and `internal/recovery` own durable records/transitions and use `internal/safefs`; adapters do not write those records directly. `internal/execenv` is the sole production process-launch boundary and uses the mandatory same-binary FD-only supervisor for every retained process class, plus the frozen cgroup boundary when that class needs it. `internal/testenv` may be imported only from `_test.go` and no production package. A Phase 1 import-DAG fixture scans production and `_test.go` imports, freezes every remaining allowed edge, and rejects every forbidden old-tree or production-to-testenv import, cycle, duplicate contract type, global registry, and reverse import before implementation.

These internal APIs are architecture-owned and freeze at the Phase 1 gate:

```go
package paths
type Input struct { /* immutable captured home and raw environment */ }
func NewInput(home string, environ []string) Input // copies inputs; no I/O
type Roots struct { Data, Cache, State string }
func Resolve(input Input) (Roots, error)

package execenv
type Spec struct {
    Path string
    Args []string
    Dir string
    Env []string
    Stdin io.Reader
    Stdout, Stderr io.Writer
    ExtraFiles []*os.File
    Class Class
}
type Runner interface { Run(context.Context, Spec) (Exit, error) }

package app
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, options Options) int
```

`paths.Input` owns private copies of the captured raw environment and home; callers cannot mutate or replace them after construction. `app.Options` is a concrete immutable dependency recipe containing `paths.Input`, frozen version, `execenv.Runner`, clock, cryptographic random reader, and side-effect-free constructors for concrete replacement source/toolchain/mise/home/state services. Constructors only assemble values from explicit inputs; they perform no filesystem, environment, process, network, root-resolution, or durable-state work. Phase 1 freezes exact field types and consumer-owned service method sets; options contain no resolved roots, generic maps, `any`, ambient-global lookup, optional fallback implementation, or callback that performs work or bypasses `execenv`.

`app.Run` first parses argv and performs only root-independent local grammar checks. Invalid argv returns before root resolution. Either version form returns the frozen version without resolving roots or activating any root-backed service. Only a well-formed non-version command calls `paths.Resolve(options.Input)` exactly once, then passes that single `paths.Roots` value to the side-effect-free constructors and activates root-backed services. Production captures environment/home exactly once into `paths.Input`; tests pass an explicit synthetic input and constructors.

### ARCH-031 — Lock publication and command-admission linearization

Initialization creates, file-syncs, and parent-syncs stable no-follow `0600` `admission.lock` and `writer.lock` inodes before initialized `root.json` can become visible. Both are opened close-on-exec, have one verified link, are never passed to children, and are never replaced or unlinked. Phase 1 freezes and proves the Linux process-held open-file-description shared/exclusive primitive and kernel release behavior for each filesystem tuple. A command acquires by descriptor, then reopens the pathname and verifies device/inode/owner/type/mode/link identity and the complete root topology before semantic validation. A path/descriptor mismatch is `BAD`, not a retry against a replacement inode.

Current-state commands take leases non-blockingly in this fixed order with no upgrade: shared or exclusive `admission.lock`, then exclusive `writer.lock` only for `A-X+W-X`. An incompatible live holder returns the exact PC-016 busy result before semantic validation; there is no wait timeout. Initialization before a valid current pair uses exclusive `init.lock` and never holds it while acquiring admission/writer locks. Every plan and `profile list` use `A-X`; `doctor`, `exec`, and `shell` use `A-S`; `bootstrap`, `apply`, and current-state recovery use `A-X+W-X`. Each retains the exact PC-016 lifetime. If admission/writer inodes or the owner predecessor are missing, malformed, or substituted, current MVP recovery never replaces them: safe `init.lock` permits only an `I-X` topology diagnosis and explicit failure unless the state is an exact ARCH-017 initialization prefix; an unsafe `init.lock` permits only topology diagnosis with no mutation.

`writer-owner.json` is published only while both exclusive locks are held. The publisher creates a unique same-directory no-follow `0600` temporary file exclusively, writes one complete canonical record, syncs and verifies the file, revalidates the stable writer-lock inode plus the prior owner record's inode/schema/generation/token, atomically renames the temporary file over the owner path, syncs the parent directory, reopens no-follow, and verifies the new inode and complete bytes. Acquisition compare-transfers `released(g)` to `held(g+1,new-token,new-process-identity)`. A stale `held(g,old-token,old-identity)` transfers only after the kernel exclusion is acquired and the full old process-start identity is proved dead. The new record binds the exact predecessor generation/token digest. Generation overflow, malformed records, absent identity evidence, a live or reused PID identity, or any inode/token/generation substitution is `BAD` and non-mutating.

The held writer authority is valid only while the same writer-lock descriptor remains exclusively locked and the path still resolves to its verified inode, the admission lease remains exclusive, and reopen verification finds the publisher's exact owner-record inode/generation/token/process identity. This conjunction is checked after acquisition publication, before every durable transition, immediately before and after every possible external mutation boundary, before stable/current publication, and before release. Every journal/operation token binds that writer generation/token. Two reclaimers serialize on the stable kernel lock; after waiting, the loser reclassifies from a fresh snapshot and cannot reuse the predecessor comparison. Random non-reused tokens, strictly advancing generations, predecessor binding, and inode revalidation close record and pathname ABA.

Clean release, while authority is still held, atomically publishes and file/parent-syncs `released(g+1)` bound to the held predecessor, reopens and verifies it, then unlocks writer and finally admission; no lock or owner path is unlinked. A crash before held publication leaves the prior durable owner; after held publication it leaves a stale held owner recoverable only by compare-transfer; before released publication it does the same; after released publication but before unlock it leaves a released record while kernel exclusion still blocks contenders. Temporary files are recognizable evidence and are removed only by the exact owner/recovery relation after inode/token validation and parent sync.

Every plan keeps `A-X` through cache staging, candidate trust, every permitted immutable source/tree/toolchain or candidate publication, synchronization, reopen verification, and cleanup. No plan upgrades, drops, or reacquires a lease. An existing immutable destination is accepted only after exact byte/digest/schema equality; it is never overwritten. In `STABLE(A)`, the recomputed candidate must remain exact `A`; a differing result fails as an unsupported update before publication. `profile list` keeps `A-X` through its cache-only source staging and cleanup and publishes no durable source object. `exec`/`shell` revalidate current/candidate/executable/environment identity under `A-S` immediately before spawn and keep it through managed-child release/reaping and status collection. These lifetimes linearize every read-to-publication and read-to-launch boundary.

### ARCH-032 — Closed source-mechanism resolution

Mechanism resolution is closed and contains exactly one positive source recipe: the canonical credential-free public HTTPS superproject fetch defined by PC-020 and ARCH-006, with direct routing, redirects/proxy/authentication disabled, the implementation-qualified native Git identity, and the fixed public CA policy. Its selected component must be embedded.

Every selected gitlink and private HTTPS/SSH source maps directly to policy rejection before authentication, network, candidate creation, manager execution, or mutation. There is no typed identity producer, scheme-to-private-mechanism table, credential/helper/agent/relay recipe, private endpoint spec, authorization tuple, or fallback/probing path. Inputs named or shaped as `SSH_AUTH_SOCK`, identity/key files, known-hosts material, helper configuration, askpass, credential stores, proxies, synthetic SSH configuration, agent/process identity, sockets, peer credentials, relays, or transport authorization are forbidden rather than resolved.

For an unselected manifest gitlink, resolution stops after bounded, non-network static validation of its canonical absolute HTTPS or SSH locator and exact path/gitlink correspondence. No executable, DNS lookup, endpoint connection, authentication channel, candidate field, source object fetch, or manager is created for that declaration. Zero access is an invariant, not a best-effort optimization.

Phase 1 proves the single public recipe and the closed selected-gitlink/private-transport rejection. Phase 5 implements no transport; it only confirms through the runner that selected gitlinks remain rejected and that unselected gitlinks cause zero authentication/network/helper/agent/relay activity. A future private selected transport requires new reviewed PC/ARCH contracts, a new closed evidence matrix, threat model, approval model, and accepted proof before any mechanism code or input surface may be added.

## Primary upstream references

- [Git submodule configuration](https://git-scm.com/docs/gitmodules)
- [mise configuration](https://mise.jdx.dev/configuration.html)
- [mise settings](https://mise.jdx.dev/configuration/settings.html)
- [mise exec](https://mise.jdx.dev/cli/exec.html)
- [mise lockfiles](https://mise.jdx.dev/dev-tools/mise-lock.html)
- [chezmoi global flags](https://www.chezmoi.io/reference/command-line-flags/global/)
- [chezmoi managed](https://www.chezmoi.io/reference/commands/managed/)
- [chezmoi verify](https://www.chezmoi.io/reference/commands/verify/)
