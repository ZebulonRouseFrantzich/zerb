# Target Architecture

## Authority and status

This document is the sole normative owner of ZERB's trust, process, protected-filesystem, and durable-state invariants. [Product contract](product-contract.md) owns public CLI grammar, source and profile schema, digest serialization, approvals, output, and exit behavior. [Validation](validation.md) owns support claims and proof evidence. [Delivery plan](delivery-plan.md) owns sequencing.

Every invariant below is the Phase 1 corrective implementation-contract candidate. Ten independent review rounds found proof/API defects; the eleventh corrective source and affected artifacts are in progress, every candidate remains `inconclusive`. Delivery's owner-approved split gate permits reviewed non-release implementation of these exact provisional invariants after source-bound cloud conformance and independent plan/security review. No invariant is qualified/frozen until the complete real frozen-host matrix, fresh plan review, focused security review, and owner freeze approval pass. Required runtime protections remain mandatory and fail closed when unavailable; no cloud or development fallback runner is permitted. Changing one still requires an owning `ARCH-*` edit, affected matrix/evidence rerun, and applicable repeat review.

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

Each invocation starts from a phase-complete environment and fixed configuration policy. Public fetch disables authentication, proxy use, redirects, automatic maintenance, and automatic garbage collection; the exact policy sets `maintenance.auto=false` and `gc.auto=0`, which prevents Git's detached maintenance `setsid` path. Source operations disable system, global, and repository configuration except audited fixed entries; neutralize hooks, init templates, attributes filters, checkout filters, custom update commands, URL rewrites, protocol overrides, pagers, editors, askpass, arbitrary helpers, and unsafe remote helpers; reject unapproved protocols; and avoid worktree checkout. An authentication challenge during public fetch fails rather than consulting ambient credentials.
Before invoking Git, the source boundary resolves the canonical hostname, rejects the entire answer set if any address is non-global or otherwise prohibited by PC-020, and passes only the validated addresses as repeated pinned `http.curloptResolve` entries. Git retains the canonical hostname for TLS/SNI and certificate verification. Redirects remain disabled, and the closed trace fails any connection not in the pinned set. Re-resolution cannot change an in-flight operation; an answer change requires a fresh inspection and cannot silently reuse source identity.

Every fetch runs in a fresh owner-validated staging subtree under hard aggregate on-disk/inode, per-file, per-object expanded-byte, total-expanded-byte, graph-depth, tree-depth, entry-count, process-output, wall-time, CPU, memory, and process-count ceilings. On the qualified Linux tuple, an unprivileged private mount namespace provides a size/inode-capped `nodev,noexec,nosuid` tmpfs staging root; cgroup/process limits and the watchdog bound the process. Pinned Git/libcurl exposes no exact aggregate wire-byte limit, and the MVP inserts no prohibited proxy or relay, so transfer bytes are observational telemetry only; high-rate, slow, stalled, partial, and interrupted transports remain bounded by wall-time, CPU, memory, output, and hard staging limits. Exceeding an enforced ceiling fails closed and removes only the verified owned staging mount/tree. A future wire-byte claim requires a separately proven direct boundary.
The closed Git trace permits only proof-owned paths, immutable `/nix/store` content, exact loader/control files, failed lexical probes beneath `/dev/null`, and the exact host-tuple `/sys/devices/system/cpu/online` compatibility observation used by native pack processing. That CPU observation is read-only, cannot enter source/candidate/digest/output identity, and is covered only by the exact `P1-PLATFORM` tuple. Any additional successful absolute, `/proc`, `/sys`, resolver, configuration, or credential-channel read fails qualification.

### ARCH-007 — Selected-gitlink and private-transport boundary

The MVP has no private transport authority. Selection of any gitlink and every private HTTPS/SSH source fails before authentication, network, candidate creation, manager execution, or mutation. No caller input, policy, manifest field, environment value, or candidate may enable a credential helper, askpass program, SSH agent/socket, identity/key file, known-hosts input, credential store, proxy, synthetic SSH configuration, relay, private endpoint, peer-authentication control, mechanism tuple, or transport approval.

Static validation of canonical HTTPS/SSH locators belonging to unselected gitlinks is syntax and manifest consistency checking only. It performs no DNS or endpoint resolution, Git invocation for the gitlink, authentication, helper/agent access, network access, candidate construction, object fetch, or materialization. Any implementation path that can produce a typed identity, credential channel, private endpoint spec, or authorization digest violates this boundary.

### ARCH-008 — Safe Git object materialization

Git fetches the canonical public superproject into a fresh protected, quota-bounded staging object store. ZERB validates the manifest, `.gitmodules`, unselected gitlink locators/object types/paths, and selected embedded path before candidate creation. It materializes only the embedded base and embedded selected component through bounded no-checkout object plumbing. Selecting any gitlink fails before Git or network is invoked for that gitlink. Materialization admits only approved directories and ordinary files with normalized executable or non-executable mode and rejects missing public objects, nested gitlinks, symlinks, devices, aliases, duplicate normalized paths, absolute or traversal paths, unsupported modes/types, hook/filter-dependent content, and every ARCH-006 resource-limit violation.

Every object and tree walk is bounded before allocating or emitting normal output. Full public-object authentication and selected embedded-tree hashing precede promotion from owned staging to protected source/tree storage. A failed, interrupted, oversized, slow, or resource-exhausting public fetch is never promoted; cleanup is confined to the verified staging root. Unselected gitlinks are never fetched. ZERB never accepts, copies, logs, or persists credential bytes.

### ARCH-009 — External-process primitive

Every external process specification contains an identity-checked absolute executable, argument array, working directory, complete constructed environment, approved standard streams, an explicit descriptor allowlist, and a process class. The runner never invokes a shell or mutates an ambient environment in place.

All ZERB-created descriptors are close-on-exec from creation. Every process is launched through a Phase 1-proven same-binary FD-only supervisor protocol. Public argv never selects supervisor mode. The child initially accepts only reserved FD 3 when it is an AF_UNIX socket whose SO_PEERCRED PID/UID is the exact parent. It opens both `/proc/<parent>/exe` and `/proc/self/exe`, validates each full device/inode/owner/group/type/mode/size/SHA256 content identity, and requires them to match; descriptor hashing uses positional reads so processes sharing the same open-file description cannot race its offset. On one locked OS thread, a child-generated random nonce authenticates one bounded strict framed stage; the child accepts only the expected inherited parent-death signal or zero, installs the stage-specific SIGKILL relation for supervisor/launcher or SIGUSR1 relation for the death guard when zero, then immediately rechecks parent PID, peer identity, executable identity, and the installed signal. The supervisor and launcher execute the already-opened authenticated same-binary descriptor rather than resolving a mutable path. The supervisor starts an unreleased same-binary launcher; that launcher sanitizes and validates its descriptor table, validates parent-death and group/session state, rechecks the opened self descriptor and exact parent identity, and reports each checkpoint before the supervisor releases the exact target exec. Non-close-on-exec descriptors are exactly `0,1,2,3` for internal control and `0,1,2` at managed exec; concurrent high and low-number, close-on-exec and non-close-on-exec parent descriptor churn cannot add a target descriptor. Missing or invalid reserved descriptors fall through without consuming public argv. Death before release exits without candidate code; death after release triggers the proven guard/group termination path. Git, acquisition, mise, and chezmoi trace-bearing manager classes additionally install the Phase 1-frozen x86_64 seccomp filter before release: only `io_uring_setup`, `io_uring_register`, and `io_uring_enter` return `EPERM`, closing the syscall-trace bypass; `exec` and `shell` payloads do not receive that filter.
Phase 1 descriptor evidence MUST combine two schedules. First, throughout every retained spawn, concurrent adversarial threads continuously cycle marker-bearing regular files, pipes, AF_UNIX sockets, and FIFOs through three currently free low-number slots and three currently free high-number ranges in both close-on-exec states. Second, isolated launches MUST occupy the exact reserved parent slots `3`, `4`, and `5` in both close-on-exec states and prove that the managed target still receives exactly `0`, `1`, and `2`; an untrusted AF_UNIX socket at dispatch-reserved slot `3` MUST reject, so the positive exact-reserved socket rows place sockets at `4` and `5` while a marker-bearing ordinary file occupies `3`. A completed spawn is evidence only when both schedules reach every declared resource/state/slot cell.

For `exec` and `shell`, the Phase 1-qualified managed image is a regular Linux x86_64 ELF file; scripts, shebangs, and every other executable format fail policy before child release. A bare name is resolved only against the already-constructed stable-profile `PATH`, whose entries are canonical absolute owner-validated directories with no empty, relative, duplicate, or writable-unowned component. A slash-containing input must be canonical absolute. Ambient `PATH` and cwd never resolve it. Resolution opens and fully identifies the source executable, copies those exact bytes into a fresh executable memfd, rehashes them, applies write/grow/shrink/seal seals, and executes only that immutable CLOEXEC snapshot. The source canonical path/device/inode/owner/mode/size/digest remain the public identity; the sealed image identity is process evidence. Symlink, path, inode, digest, owner, type, mode, same-size in-place content mutation, or final-release substitution cannot change executed bytes.

The production runner MUST NOT rely on `exec.CommandContext` or equivalent kill-the-leader behavior as its cancellation policy. It owns the supervisor/group/session setup, parent-death behavior, signal forwarding, grace and escalation, complete waiting/reaping, and orphan detection required by ARCH-010.

The first private production `execenv` increment observes already-open regular
file content with positional reads, fixed scratch, before/after full metadata
including group, and exact SHA256. Caller ownership and shared offsets remain
unchanged; failures return zero facts, and zero/unsupported facts cannot compare
as a match. This is executable observation only, not ELF/path/owner policy,
AF_UNIX peer authentication, dispatch, execution permission or a runner. The
complete authenticated supervisor and every mandatory protection above remain
required before managed execution; no incomplete exported Runner/Dispatch or
ambient fallback is supplied by this increment.

### ARCH-010 — Lifecycle policy by process class

Phase 1 freezes, for native Git, acquisition/extraction, mise and backend descendants, chezmoi/template descendants, `exec`, and `shell`, the exact Linux supervisor, process group/session/subreaper model; forwarded signal set; catchable-signal and context-cancellation grace period; escalation signal and timing; leader and descendant reaping; exit mapping; and orphan detection.

Each retained exact executable/argv process-class tuple MUST be exercised and observed not to daemonize, call `setsid`/successful group escape, or leave the cumulative lifecycle boundary; evidence does not generalize to another version, argv, backend, or arbitrary payload. Git uses the exact ARCH-006 no-auto-maintenance policy and mise install uses exact `--raw` mode to avoid their observed detached/group-changing paths. The authenticated FD-only supervisor and launcher release protocol are mandatory for every class. The exact Phase 1 cgroup-v2 limits are `memory.max=805306368`, `memory.swap.max=0`, `pids.max=96`, and `cpu.max="100000 100000"`; any OOM or OOM-kill event fails the invocation even when a child otherwise exits successfully. The 768 MiB value is the exact observed finite passing bound, not a proved minimal bound; lower-memory diagnostic failures are not retained qualification evidence. Phase 1 hard-kills before launcher spawn; after launcher start while the target remains unreleased; after actual descriptor, parent-death, group/session, and second-parent validations; immediately before exec release; and after first descendant observation. The Go fork lock, full-lifetime locked creator thread, authenticated child-side parent-death setup, and immediate parent recheck cover the internal fork/exec interval without assigning a false injectable label to it. After release, the authenticated death guard holds an opened identity-checked cgroup directory descriptor, kills the managed group, writes `1` to descriptor-relative `cgroup.kill`, and waits on descriptor-relative `pids.current` until zero. The outer subreaper reaps every adopted child and job-control group while preserving the guard status. The exact shell tuple is Bash 5.3.15 invoked as `--noprofile --norc -i` on a fresh PTY with `setsid`, `TIOCSCTTY`, and `TIOCSPGRP`; its foreground and background job-control groups remain inside the same cumulative cgroup and are reaped after catchable cancellation and hard wrapper death. Any uncovered race removes the process/platform tuple; the cgroup is cumulative around the mandatory supervisor and never replaces it. Catchable cancellation and wrapper hard death leave no managed descendant for the retained invocation; durable startup state diagnoses interrupted mutation.
Ordinary completion and catchable cancellation use the supervisor as the primary cgroup cleanup authority: after it opens and records the delegated cgroup directory identity, launcher placement, membership observation, `cgroup.kill`, `pids.current`, and emptiness verification are all descriptor-relative and never reopen a retained pathname. Phase 1 changes every remaining cgroup pathname field to a distinct sibling after opening the original authority, proves the managed PID appears only in the original descriptor and the sibling retains its identity and remains empty, and records that the candidate cgroupfs rejected directory rename; no infeasible rename claim is retained. The supervisor reaps the resulting children, verifies original-descriptor emptiness, then disarms and reaps the guard. The guard is the independent hard-parent-death authority. Every path MUST observe the guard's exact wait status; zero is required for success, and a missing, signaled, or nonzero guard status is an operational failure even when the cgroup later appears empty.

The owner-approved provisional termination policy forwards only SIGHUP, SIGINT,
SIGQUIT and SIGTERM through the managed boundary; context cancellation sends
SIGTERM. The first termination request starts one shared monotonic 750 ms grace
window for the leader and covered descendants. A normal leader exit with surviving
descendants starts that cleanup window; an existing window is never restarted by
leader exit, later cleanup stages or repeated requests. At expiry, escalate with
SIGKILL and the existing descriptor-relative cgroup kill boundary. Post-kill
emptiness verification, all-adopted-child reaping and the exact zero guard status
remain mandatory; 750 ms is not a total cleanup timeout or authority to return
with survivors. Hard-parent-death guard behavior remains immediate. The value
matches the proof candidate and is provisional, not optimality or qualification.
While the managed lifecycle remains open, the first observed forwarded POSIX
signal determines the signal-derived status, taking precedence over context-only
cancellation; repeated signals do not change that first signal or extend grace.
Without a POSIX termination request, context cancellation returns 1; otherwise
normal child status propagates under PC-015. Signal/context/exit race tests must
exercise that arbitration without manufacturing a real-time total-order claim.

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
The semantic provider/host-fact prohibition applies to candidate, mapping, template, configuration, output, digest, and durable-data channels. It does not pretend that an implementation-qualified native manager or installed tool is independent of the exact host tuple. Any separately permitted platform-compatibility observation MUST be finite, read-only, closed-traced, recorded as platform evidence, unavailable to candidate/template data, and excluded from candidate digests.

### ARCH-012 — Managed binary acquisition

ZERB acquires exact Phase 1 implementation-qualified mise and chezmoi artifacts into protected staging, verifies full SHA256 plus the Phase 1-frozen signature/provenance contract, verifies the extracted executable identity, and atomically promotes it into ZERB-owned toolchain storage. Managed operations use only that absolute executable identity and never `PATH` selection.

Acquisition is ZERB-controlled input processing, not candidate trust. A cache hit remains untrusted until reauthenticated and rehashed. An upstream version or artifact change requires renewed executable identity and affected proof evidence.

#### Chezmoi-first archive preparation

The owner approved the first bounded materializer target as the existing
`packages.x86_64-linux.chezmoi-zerb` source/build recipe in `flake.nix` and
`flake.lock`: fork commit `ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5`,
version `v2.70.5-zerb.1`, `CGO_ENABLED=0`, and the unchanged locked vendor,
compiler/build inputs and linker settings. This preserves the source/build
recipe; it does not establish an executable identity until actual output is
authenticated and compared with retained reviewed evidence. Mise remains a
separate, deferred artifact; same-version upstream binaries are not substitutes
for either retained build.

The planned project-produced envelope is
`zerb-chezmoi-v2.70.5-zerb.1-linux-x86_64.ustar`. Its name is not authority or
evidence that bytes exist. The selected format is uncompressed canonical USTAR,
with explicit parent directories and only the independently reviewed manifest's
exact path/type/mode/size/content set. Directories and the executable have
`0700`; ordinary notice/resource files have `0600`. There is no link, extension,
sparse/legacy type, root stripping, mode repair, concatenated archive or trailing
data. Freeze one raw-header/numeric/padding representation and the two zero-block
EOF in the reviewed artifact recipe. Do not grow a generic archive compatibility
surface. The expected runtime paths are `bin` and `bin/chezmoi`; the complete
runtime/notice inventory, including the pinned license, must be authenticated
before fixing the entry set. `CGO_ENABLED=0` alone does not prove closure or
relocatability.

A frozen, independently reviewed offline manifest is the provenance authority.
It binds the authenticated source/vendor inputs, exact derivation/toolchain and
build settings, resulting executable and complete runtime/notice inventory,
canonical envelope identity and reproducibility evidence. A producer-generated
hash, source pin, successful local build or caller-supplied provenance bytes
cannot authenticate themselves. This direction adds no online signing authority,
key, new package or exported API. Actual production manifest encoding and byte
values remain gated on the authenticated evidence, not invented defaults. If
retained executable identity is unavailable or differs, preserve the evidence
and resolve that candidate relation before coding depends on it.

Use a private owned byte snapshot selected by that authority. Bound size before
copying, then verify the complete owned snapshot before any archive parser sees
it. Trusted private extraction/publication may borrow read-only payload slices;
no mutable alias escapes to callers. Keep incoming buffers, the owned snapshot,
metadata, scratch and all simultaneously retained storage in the materializer's
fit argument, separately from the unchanged ARCH-020 stage reservation. Existing
synchronous byte-slice publication is preferred; no streaming or descriptor API
is admitted merely by this direction.

Authenticated actual build output, full runtime/notice inventory and a passing
concrete resource-fit proof precede dependent production coding. A missing build
environment, executable hash or inventory is a stopped gate, not permission to
fabricate an artifact, relax bounds or substitute a host/toolchain. Preparation
does not authorize installing Nix, changing its trust/configuration, invoking a
manager or altering the owner's host. See the non-normative
[build/evidence handoff](archive-materialization-proposal.md#build-and-evidence-handoff).
Network acquisition, manager execution, state/CLI integration, automatic
promotion and all host/release/support qualification remain outside this slice.

### ARCH-013 — Mise adapter isolation

Mise owns its native declaration/backend/install semantics. ZERB owns the implementation-qualified exact-version, exact-authored-lock, and declaration allowlists, verified binary, deterministic `base + selected` staging, isolation environment, invocation ordering, failure journaling, and observation of requested versus installed versions.

Every mise invocation and descendant stays within the ARCH-011 roots and uses the exact pinned-release surfaces: offline `install --raw`, `where`, `env --json`, and `exec`. `--raw` is mandatory for install because the ordinary parallel-output path performed a successful self `setpgid`; the raw path keeps standard streams direct and implies one install job. Mise must not discover user/system/global/project manager state, use settings or lock-generation commands that write configuration, execute tasks/hooks/plugins/custom backends/sourced environments, or fall back to ambient installs/shims. Duplicate tool identities fail rather than using mise override behavior. Aliases, ranges, channels, `latest`, missing exact locks, and any post-trust resolution or lock mutation fail before manager execution. Mise may verify an exact candidate-bound authored version/lock but cannot add identity to a trusted candidate.

The Phase 1 qualification proves the frozen configured-path and discovery controls for mise 2026.7.17, including data, config, cache, state, installs, downloads, plugins, shims, global/system config suppression, filename/ceiling controls, hooks, auto-environment, and auto-install. These upstream variables remain version-specific interfaces; changing the managed identity requires fresh evidence.
The exact retained mise/Node tuple permits successful host observations only in these eleven reported classes: `path-mise-platform-abi` for `/lib`, `/lib64`, and `/lib64/{libc.so.6,libdl.so.2,libgcc_s.so.1,libm.so.6,libpthread.so.0,libstdc++.so.6}`; `path-mise-platform-cpu` for `/proc/cpuinfo` and `/sys/devices/system/cpu/online`; `path-mise-platform-current-process` for `/proc/self/exe`, `/proc/self/maps`, and `/proc/sys/vm/overcommit_memory`; `path-mise-platform-memory` for `/proc/meminfo`; `path-mise-platform-os` for `/etc/os-release` and `/usr/lib/os-release`; `path-mise-platform-current-cgroup` for `/proc/self/cgroup`, only the invoking traced PID's `/proc/<pid>/cgroup`, `cpu.max` from the exact current cgroup leaf through `/sys/fs/cgroup`, and `cgroup.controllers`, `memory.high`, and `memory.max` only at that leaf; `path-mise-platform-timezone` for read-only paths at or beneath `/usr/share/zoneinfo`; `syscall-current-identity` for `getuid`, `geteuid`, `getgid`, `getegid`, `getgroups`, `getresuid`, and `getresgid`; `syscall-runtime-affinity` for `sched_getaffinity`; `syscall-runtime-limits` for `getrlimit` and `prlimit64`; and `syscall-uname` for `uname`. Evidence reports the complete key set including zero counts and the positive-count observed subset; the distinct PC-031 runtime-bootstrap counter must be zero for mise. Successful localtime, nscd/outside Unix sockets, `sysinfo`, PC-031-only mountinfo/THP bootstrap, or any omitted path/syscall fails. Byte-free failed discovery probes are classified separately. The manager seccomp boundary makes every `io_uring` setup/register/enter attempt fail before an untraced operation can be submitted. Permitted compatibility observations cannot enter declarations, locks, candidate/template data, output, canonical digests, or durable records beyond the declared platform tuple.

### ARCH-014 — Chezmoi adapter isolation

ZERB materializes one protected effective source from `base + selected`. For candidate construction it invokes only the `P1-CHEZMOI-STATIC` public fork commit, statically built with `CGO_ENABLED=0`, through PC-031's exact `managed --skip-template-data-environment` read-only, non-rendering, machine-readable target-mapping mode, with explicit protected source/config/cache/persistent-state/working roots and a synthetic non-real destination root. Normal and flagged structural output are identical for the frozen corpus. For post-authorization rendering/application it invokes the same verified identity with the explicit roots and real destination required by the frozen contract. It does not run repository initialization/update, commit, push, sequential independent-source application, or any settings path that can adopt ambient manager state.

Pre-trust validation admits only the frozen ordinary/private/executable entries and static template syntax from the embedded components. Provider references, semantic host facts, named-template expansion, scripts, externals, authored removal, source-root controls, arbitrary filesystem functions, and command/output functions fail before candidate creation. A selected gitlink fails before effective-source assembly. The sole pre-trust fork invocation derives only normalized structural targets: exact source/patch/static-build/executable identity, argv, environment, descriptors, and roots are candidate-bound; `uname`, `sysinfo`, UID/GID/group lookup, passwd/group databases, hostname, OS-release, kernel template data, templates, helpers, providers, scripts, externals, and candidate content do not execute; and the real destination is neither accessed nor writable. The only host-derived observations admitted are PC-031's separately counted pinned-Go bootstrap paths plus `sched_getaffinity` and `prlimit64` runtime-bootstrap syscalls; they are used only for runtime scheduling/resource setup, cannot enter mapping data, output, candidate identity, or durable state, and every other `/proc`/`sys` path or host-fact syscall fails. The manager seccomp boundary rejects all `io_uring` submission interfaces before target release. Output divergence, a patch beyond initialization/export suppression, or any restricted access removes the candidate; ZERB never substitutes a fallback mapper.

### ARCH-015 — Provider and host-fact exclusion

Secret-provider retrieval, provider-backed rendering, and host-fact-dependent templates are outside the MVP. In the Phase 1 disposable harness, the static validator receives an embedded component directly and rejects every provider reference, provider function, host-fact function, dynamic lookup, and unapproved template function before candidate creation. Selected gitlinks fail even earlier at the source-selection boundary, before authentication, network, materialization, provider processing, render, manager install, or mutation. Candidates and durable records contain neither provider references nor values.

Phase 3 repeats the source-specific boundary through `internal/app.Run`; Phase 6 repeats it through the installed binary. Future provider support requires a new product contract and real evidence. In particular, a versioned lookup is eligible only when its immutable generation selector is authored and statically knowable before trust; a provider that requires live pre-trust token discovery is ineligible rather than prefetched or late-bound.

### ARCH-016 — Durable root namespace

ZERB resolves data, cache, and state roots once from the real environment and user home according to the frozen Linux XDG contract. It exposes no ZERB-specific root override and creates or reads no ZERB XDG config root in the MVP.

The resolved `data/zerb`, `cache/zerb`, and `state/zerb` roots must be canonical, pairwise distinct, and pairwise non-aliasing/non-containing: none equals, contains, or descends from another by canonical path, descriptor-relative ancestor identity, device/inode identity, mount identity, or bind-mount relation. Duplicate or conflicting XDG inputs and any alias/nesting are rejected before creating a product path. `internal/paths` remains import-free and owns only immutable input capture plus lexical/canonical XDG resolution. `internal/safefs.OpenRoles` opens the descriptor-owned state/data/cache role set, and `safefs.Roles.Validate` revalidates descriptor/ancestor/device/inode/mount/bind relations before every protected-root or cache use. `state`, `source`, `toolchain`, `mise`, and `home` invoke that validation at each owned boundary; `app` retains the operation lease while those callers act. Cache-owned paths can never contain durable state or data.

`NewInput` validates only non-NUL home/environment bytes and nonempty `key=value` environment keys, retaining an independent raw ordered copy without I/O or root semantics. Opaque values and ordinary duplicate keys remain captured; `LookupEnv` returns the first matching entry. The explicit captured home, rather than a raw `HOME` entry, owns defaults. `Resolve` rejects an unconstructed input or non-absolute home, lexically cleans absolute paths without filesystem access, and implements [Linux XDG 0.8](https://specifications.freedesktop.org/basedir/latest/): unset, empty or relative data/cache/state selectors use `.local/share`, `.cache` and `.local/state` beneath that home. Any repeated `XDG_DATA_HOME`, `XDG_CACHE_HOME` or `XDG_STATE_HOME` selector fails, even with identical or ignored values. Only those three selectors affect roots; config/runtime/unknown variables remain opaque. Append `zerb`, then reject equal or segment-wise containing product roots; containing base directories alone are not overlapping product roles. Lexical resolution grants no descriptor, symlink, owner, mount or bind assurance; those checks remain mandatory in `safefs` before creation/use. Errors return zero roots without captured metadata.

The protected namespace is exactly:

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
  replace-witness.json  # optional exact interrupted replacement record
  candidates/<digest>.json
  journals/<operation-id>.json
  receipts/<digest>.json
  current.json
```

State/data directories and the per-instance cache directory are created as `0700`; synchronization inodes, owner publications, markers, candidates, journals, receipts, and current state are `0600` from creation, independent of umask. State and data share one cryptographically random 128-bit lowercase-hex instance ID. Cache uses that instance ID only as a namespace marker and remains untrusted. The three lock paths are stable synchronization inodes, not ownership records: they are synced once, never replaced or deleted, and are accepted only with the Phase 1-frozen owner/type/mode/link/device/inode checks. `writer-owner.json` is the separate atomically replaceable ownership publication.

Successful fresh initialization creates exactly the three stable locks, initialized `root.json`, released-generation-zero writer owner (or the exact returned bootstrap held-generation-one owner), and empty `candidates`, `journals`, and `receipts` directories in state; only `owner.json` in data; and only `owner.json` in the newly created cache instance. No current, witness or replacement temporary remains. Data `toolchain`, `sources`, `trees`, and `installs`, and cache-instance `downloads`, are reserved lazy branches: their owning service creates them later under its required lease. The namespace reservation applies even while these branches are absent. Existing unrelated cache entries are neither adopted nor inspected for ownership.

### ARCH-017 — Exclusive root initialization

ARCH-020 closes the minimum no-write bootstrap-eligibility predicate and exact state-only preparation needed to obtain I-X when state paths are absent. It does not authorize any preparation for nonempty/foreign/evidence-bearing roots; all claim and data/cache materialization retain the initialization authority below.

Clean first use is exactly the PC-060 predicate: after ancestor/owner/type/mode/no-follow checks, each product state/data directory is absent or an empty directory and neither root contains a recognizable synchronization name, reserved durable path, marker, claim, domain tag, or record. Mixed absent/empty pairs are permitted. Any non-empty root is classified by ARCH-018 instead.

Initialization uses a stable `state/zerb/init.lock` synchronization inode, created no-follow as `0600`, file-synced and parent-synced, never replaced or deleted, and held with the Phase 1-proven Linux open-file-description exclusive lock. All contenders open and validate the same inode; the descriptor is close-on-exec, is never supplied to a child, and kernel release on process death is part of the supported platform/filesystem tuple. The lock holder re-reads both roots, then completely writes and file-syncs an anonymous schema-1 `root.json` as `initializing` with a random instance ID, fresh random ownership token, PID, and process-start identity, publishes it without replacement, parent-syncs and reopens it. The first claim has no predecessor and never uses replacement or upsert.

ARCH-020 specifies the staged replacement record and exact maintenance overlays for claim takeover/completion; no other temporary evidence is admitted. Only the holder whose token and initialization-lock inode still match may create, file-sync, and parent-sync the stable `admission.lock` and `writer.lock` inodes; atomically publish and sync `writer-owner.json` as released generation zero; create and sync the matching data and cache markers; and finally atomically replace, file-sync, and parent-sync `root.json` as initialized. It reopens and verifies every publication before advancing. A loser writes no marker, synchronization inode, owner publication, or product data.

After proving the recorded process identity dead, a successor first acquires the same exclusive `init.lock`, revalidates its inode and all root topology, compares the persisted instance/token/status and record inode, atomically publishes and syncs a new random takeover token plus its own process identity, reopens and verifies that publication and the stable lock inode, and only then continues. The lock serializes simultaneous takeover; the old token is never authority. Failure before the synced token replacement leaves the old claim; failure afterward leaves the new claim. If lock semantics, identity proof, inode/token compare-transfer, or a durability boundary cannot be proven on a platform/filesystem tuple, recovery remains non-mutating and that tuple is not supported.

#### Bounded fresh initialization and admission

The owner-approved first production vertical is one private FreshInit producer for the typed profile-list, plan and bootstrap intents. It returns the planned ARCH-030 acquisition/lease values with their real revalidation and release behavior. It does not expose a partial `Store`, stub `AcquireFor`, caller-selected generic lease mode, public router, full classifier, takeover, restart maintenance, current, journal or receipt integration. Existing initialized, foreign or evidence-bearing input is refused for routing elsewhere; refusal is not a new classification or repair implementation.

Before I-X, minimum bootstrap eligibility uses only role/topology and bounded entry observations: state/data each absent or empty, no durable-record interpretation. `Names(0)` establishes emptiness; a generic open failure never establishes absence. Finite canonical-prefix opens plus an admitted parent's bounded names may establish the exact missing edge. Ancestor/structural-container inventories have a 65,536-name ceiling, no content reads, and refuse overflow; later prefix inventories use the exact expected count. Only missing state-path structural prefixes and the state root may be exclusively created before I-X. Each successful next edge is explicitly bound through `Roles.Add`, including its atomic shared-prefix advance. Role binding conveys no initialization authority.

Only a continuously retained pristine-bootstrap session may account for expected concurrent creation of the stable `init.lock` name: it captures the first safe inode and then requires exactly that init-lock-only prefix under I-X. A new invocation encountering a pre-existing init-lock-only root sees recognizable INIT evidence and this producer refuses. An observed exclusive-create collision terminates the attempt; no contender switches to a replacement inode. The initial lock must be empty: its captured `Lock.Identity().Size == 0` is bound by actual full lock revalidation, not assumed from `OpenLock` admission. Other unexpected appearances never become eligible by being opened.

The following sequence is closed. Each row expands into separate before/after creation, file-sync, directory-sync, parent-sync, link/rename and reopen boundaries where applicable. Before and after each action, revalidate every previously captured edge/identity and record byte sequence, the complete expected prefix, roles and any already captured live process evidence. Empty structural directories are permitted only at their listed position; unknown entries, gaps, wrong order or substitutions refuse. The ARCH-020 exact replacement overlay is the only temporary exception.

| Order | Action and required resulting prefix |
|---|---|
| 1 | Establish the no-write eligibility above, exclusively materialize only necessary state prefixes/root, bind each next role edge, synchronize and revalidate. |
| 2 | `OpenLock(init.lock,true)`, validate empty safe stable inode, synchronize file and parent and reopen/revalidate, take nonblocking I-X, and recheck state/data eligibility accounting only for that session's exact lock. |
| 3 | Capture distinct random instance/init token and the current process tuple; `PublishNoReplace` the canonical initializing root generation zero, synchronize and reopen. |
| 4 | Create admission lock, then writer lock, each with `PublishNoReplace(name,nil,0600)` followed by `OpenLock(name,false)`. Require exact returned published identity, zero size, live lock validation and parent synchronization. A failed/uncertain link or collision stops without adoption, retry or deletion. This makes no retained-origin-pin or invisible-cycle claim across publication return and reopen. |
| 5 | Publish, synchronize and reopen the canonical released-generation-zero writer owner. |
| 6 | Exclusively create empty `candidates`, then `journals`, then `receipts`, synchronizing each child and parent and reopening before advancing. |
| 7 | Under I-X, materialize missing data prefixes/root through exact role binding or retain the initially admitted empty root; publish, synchronize and reopen its matching owner. No lazy payload branch is created. |
| 8 | Materialize missing cache-root prefixes through exact role binding or explicitly open the safe existing root, then materialize/open its `instances` container; exclusively create the fresh instance directory and publish/synchronize/reopen its matching owner. An instance-name collision refuses even if its contents look suitable. No downloads or other instance content is used. |
| 9 | Retain I-X, open/revalidate the exact admission inode and take A-X; bootstrap additionally takes W-X. The writer owner remains released generation zero throughout INIT. No lease upgrade, drop or reacquisition occurs. |
| 10 | Stage initializing-to-initialized root replacement, binding the live pinned predecessor claim, init-lock, process, token and complete graph. Publish/reopen the immutable witness, link the complete stage, revalidate, publish once and verify the successor. Retire the exact pinned witness while original I-X and target A-X or A-X+W-X remain held; plan/profile never add W-X. |
| 11 | Verify complete READY with released generation zero. Bootstrap only then performs ordinary writer acquisition to held generation one under A-X+W-X, using its own new witness and exact live session; retire that witness and verify READY-held-one. This is post-initialization acquisition, not an INIT held-owner prefix. |
| 12 | Consume/release I-X and return the already-held target acquisition without changing its mode. |
| 13 | Successful bootstrap release revalidates exact READY-held-one authority, publishes/verifies released generation two through the witness protocol and retires its witness, then unlocks W before A. Plan/profile release validates and closes/unlocks A-X and owned resources. |

No implicit durability is supplied by directory creation. Before relying on a newly created or newly admitted empty product root/structural container, synchronize that directory and its required parent/name attachment through admitted descriptors, then revalidate. Apply this also to necessary newly observed owned-prefix attachments; do not assume an unseen concurrent creator synchronized them. A canonical parent proven to belong to the existing UID-0 structural prefix may be independently opened with `OpenRoot(parent,0)` solely for read/revalidation and `Sync`, cross-checking its identity and the retained child/name edge. That handle never admits a product root or performs child creation/publication/removal; all product mutation keeps the originally supplied owner. This is existing structural-prefix admission, not an arbitrary observed-owner selection. If the required parent cannot be acquired/synchronized under that unchanged policy, refuse, without permission repair, raw descriptor escape or a new owner exception. Existing empty roots retain the existing Dir mode policy; new directories are exact `0700`. Existing cache structural containers may be opened only under explicit unchanged directory admission; that grants no instance ownership.

Every failure returns the single still-owned acquisition containing every remaining I-X/A/W descriptor and other owned resource, even after target acquisition, initialized-root visibility, held-owner uncertainty or I-X consumption. The caller replaces its previous handle with that result before checking the error and releases exactly once. An errored acquisition is Release-only. For this bounded producer, Release on an errored acquisition performs no filesystem writes or witness cleanup, even if a later observation might permit them: it preserves evidence, closes/unlocks all still-owned resources and reports primary plus cleanup failure. Only the successful returned bootstrap lease may perform the fully revalidated released-generation-two protocol. Kernel release may leave INIT, uncertain or stale-held evidence; it is not rollback or successful initialization. Never return an unreported held lock, retry a failed replacement, infer absence from failure, or grant continuation from Close success. Cancellation checks surround logical actions but never omit cleanup; arbitrary filesystem I/O has no hard wall-time guarantee.

#### Fixed current-process identity observation

The private state reader admits only generated current-PID kernel observations, separate from the ARCH-020 proc-FD and topology exceptions. Each capture/revalidation locks its OS thread for that observation, validates the current kernel root, real `PROC_SUPER_MAGIC`, pinned kernel `self` resolving to that PID, and the numeric PID/`ns` directory edges with no symlink or internal mount crossing. A later observation may use another OS thread; PID/start/PID-namespace and retained edges must remain unchanged. The proc root, pinned self, numeric directories and stat pin/read descriptor must retain their expected parent/name edges and common proc filesystem/device/mount identity; only root-to-proc and the fixed final namespace-link traversal are intentional crossings. No caller path or foreign PID is accepted.

Open fixed `stat` read-only, no-follow, nonblocking and close-on-exec. Accept at most 4,096 bytes plus a one-byte overflow probe. Require the generated canonical decimal PID, a space, the complete parenthesized comm, its final `) ` separator, one kernel state byte and enough subsequent space-separated accounting fields to reach positive uint64 start-time field 22. Comm may contain whitespace, newlines and parentheses; do not split the whole record on whitespace. Require the final kernel newline and reject malformed framing, missing/invalid start, NUL or overflow; later accounting fields are unused and impose no exact total field count. Dynamic accounting bytes need not match across reads, but the pinned file object/edge and identity tuple must.

Pin only fixed `ns/pid` without following it; accept at most 64 link bytes plus an overflow probe, exactly `pid:[positive-decimal]`. Intentionally follow only that final fixed link to an `O_RDONLY|O_CLOEXEC` namespace descriptor, require NSFS and `NS_GET_NSTYPE == CLONE_NEWPID`, and match the target namespace inode to the decimal (not the symlink inode). Retain the namespace FD for the live session. Reobserve stat identity and namespace evidence around capture and at semantic boundaries, with deterministic cleanup and no child inheritance. Missing, restricted, changed or ambiguous facilities refuse. This adds no historical/death lookup, `kill(pid,0)`, ENOENT-as-death, age heuristic, takeover, generic proc API or package, privilege/security change or host qualification.

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

An exact initializing replacement overlay remains INIT. Any initialized graph with a replacement record is a named evidence-preserving BAD repair relation until exact retirement, never ordinary READY/STABLE/TRANSITION/CLEANUP admission. Unsupported or unprovable overlays remain BAD without repair.

Recognizable evidence includes every reserved ARCH-016 state/data path or filename and every ZERB schema/domain tag even when its object is empty, malformed, wrong-type, or from an unsupported version. Once any such evidence exists, an unrecognized extra entry anywhere under state/data is corruption rather than ignorable foreign content. If there is no recognizable evidence anywhere, all non-empty entries are foreign; this rule prevents content-dependent guessing.

`INIT` is closed, not an inference from `init.lock`. An exact ARCH-020 replacement-record prefix is the sole admitted temporary-evidence overlay, only under its independent maintenance admission; unknown residue never enters INIT. Otherwise it permits only the safe permanent lock plus the supported, well-formed publications from ARCH-017 in exact order without a gap, all bound to the same instance/token/process claim, with no initialized/current/candidate/journal/receipt/install record or object and no extra entry; the empty candidates/journals/receipts containers are admitted only in ARCH-017 order. VAL-025 enumerates every exact prefix at each before/after durability boundary. A malformed/unsupported publication, an initialized-root deletion, a substituted lock, or any evidence inconsistent with that prefix is `BAD`, even though `init.lock` persists.

A supported current relation is complete only when the owner pair, all stable synchronization inodes, released or exactly held writer-owner publication, and every present durable record cross-validate. `READY(∅)` has no current or transition. `STABLE(A)` has exactly one current reference whose candidate and completed receipt validate, no journal, and no unreferenced receipt. `TRANSITION(A,B)` has exactly one permitted prepared/mutating/dirty journal graph; `A` may be the explicit empty baseline only when no prior current/candidate/receipt exists. `CLEANUP(B)` is stable `B` plus its one exact writer-bound cleanup journal and only the journal-named prior candidate/receipt objects not yet removed, or the exact committed-and-cleaned held-owner relation with no journal and only writer release remaining; first bootstrap has no prior objects. Cleanup removes/syncs the exact prior candidate when distinct, then the exact prior completed receipt when distinct, and removes/syncs the journal last, so every remaining receipt is referenced by current or journal at every crash boundary. Independently published and fully authenticated immutable candidate/source/tree/toolchain objects may otherwise be unreferenced; a receipt, writer-owner publication, or current record may not. A dead-process `held` owner publication with a supported relation is recoverable only by ARCH-031 compare-transfer.

#### Resource-admitted graph validation

The owner-approved policy preserves independently valid unreferenced immutable
candidate/source/tree/toolchain objects; it adds no automatic historical cleanup,
authoritative index or permission to ignore an entry. Each operation needs a
reviewed complete representation and scan schedule before its resource envelope
is frozen. Account for retained record/model/name/path storage, pins and ancestor
descriptors, comparison workspace and cumulative enumeration/read/hash/revalidation
work. PC-042's 8,000,000-byte parsing/canonicalization invocation budget remains
unchanged; it is neither an already-proven whole-Store budget nor permission to
multiply/reset reservations while retaining earlier results.

If required validation cannot finish within the reviewed resources, return an
operational error with no unproved supported classification or mutation admission.
Do not label valid but incompletely inspected history corrupt, infer cleanup
rights, or issue move-aside/delete guidance. Preserve evidence and return any
already-owned acquisition for release under its unchanged intent lease; there is
no unlocked fallback, lease drop/reacquire or implicit repair. Cache absence or
poisoning still cannot change durable truth. A large valid history may therefore
be operationally unavailable; arbitrary history cannot be exhaustively verified
under a fixed total-work ceiling.

Before publication or mutation, reserve the worst permitted intermediate graph
and required verification/cleanup work. Exhaustion after a possible effect must
preserve the truthful transition/cleanup evidence and existing ownership, never
imply rollback. Full graph admission requires real source/tree/toolchain/install
inventory validation as well as active record correspondence. A record-only
predicate, directory count or inventory-only measurement does not establish it.

#### Bounded existing initialized observation

The bounded existing-empty-initialized observation is a private descriptor-owned
helper, not an exhaustive classifier or command admission API. From a typed
intent it takes only minimum safe role/state/admission-inode observations,
chooses the ARCH-031 A-S/A-X mode, acquires it nonblockingly, and retains that exact
OFD through all semantic reads, revalidation and close. It neither acquires I-X
or W-X nor returns an `Acquisition`, classification, writer ownership or completion
authority. The unchanged existing-lock primitive synchronizes its inode; no
namespace/content write, creation, owner publication or repair is performed.

Its narrow physical precondition is exactly the eight freshly initialized state
entries, empty candidates/journals/receipts, canonical initialized root and
general writer-owner values, matching data owner with no other data entries, and
the matching owner-only cache instance. It pins bounded full regular-file facts
and all retained directory/name identities, under unchanged directory admission.
Held owner facts give no liveness or ordinary-operation permission. No parity or
generation-headroom rule is added. Unrelated cache siblings are not inventoried
or adopted. Missing/poisoned cache, lazy branches and independently authenticated
unreferenced immutable content are outside this helper; refusal never labels
them BAD or changes their durable classification. Later complete routing still
requires ARCH-018/027/031 under its retained intent lease.

Copied observation handles share one serialized lifetime. Failure yields zero
facts and revokes/releases owned resources without semantic cleanup; construction
failure closes all owned descriptors and reports cleanup errors. Close never
releases a published writer owner or removes evidence. Reopening/checks detect
observed substitutions, not invisible restored same-UID cycles or an atomic
snapshot of arbitrary concurrent writers.

### ARCH-019 — Untrusted cache

Only the owner-validated `instances/<instance-id>` cache subtree may be used. ZERB validates owner/type/mode and no-follow topology on every cache access and authenticates and hashes every byte before parsing, extraction, or execution. It never adopts, relabels, trusts, or deletes unrelated cache content. A missing or poisoned cache is a cache miss, not durable truth.

For `P1-MISE-CORE-NODE`, authenticated bytes are published into a separate size/inode-capped private tmpfs inside an unprivileged user/private mount namespace. Authentication uses the still-open descriptor; file and directory are synced; the exact inode is renamed to mise's cache identity; the mount is remounted read-only; descriptor/path identity and SHA are reverified before and after a manager-UID modify/rename/remove attack, before mise open, and after mise. The manager runs as a distinct mapped UID and every mount/remount API is traced and forbidden. Only normal checked unmount/cleanup completes the evidence.

### ARCH-020 — Safe filesystem operations

All protected path traversal uses descriptor-relative or equivalently race-resistant no-follow operations and rejects symlinked, hardlinked where a single link is required, wrong-owner, wrong-type, and group/world-writable components. Creation establishes restrictive modes immediately. Staging and promotion use the same protected filesystem; durable replacement writes and syncs the file, atomically renames, and syncs the parent directory. Cleanup is confined to a verified owned staging subtree and never follows links.

The approved absolute ancestor policy permits a UID-0-owned structural prefix followed by the explicitly supplied-owner subtree. Once that subtree begins, traversal cannot switch back to another owner; every returned protected directory belongs to the supplied owner. Every component remains a non-symlink directory without group/world write permission. Sticky-directory semantics grant no exception to writable-parent rejection, and a container's alternate structural UID grants no exception to UID 0. Retain the kernel-root descriptor and every ancestor/name edge, observe actual descriptor-bound mount identity, and revalidate the chain and policy before and after child operations. Ordinary directory size/link-count growth is not an inode substitution; a zero-link directory or any observed device/inode/mount/owner/type/full-mode change fails. Mount observations alone do not establish the separate complete role-overlap proof.

The bounded `Dir` increment implements only `OpenRoot`, `OpenDir`, cached `Identity`, bounded `Names` and `ReadFile`, directory `Sync`, and `Close`. Root open requires an existing canonical absolute directory; child open accepts one nonempty non-dot component. Creation uses `mkdirat(0700)` and succeeds only when the first observed child is exactly owner-owned `0700`; an observed absence followed by `EEXIST` fails. It never changes process umask, chmods an existing/replacement inode, repairs unsafe content, or unlinks an unverified failed publication. A restrictive umask may therefore cause operational refusal, leaving the created entry untouched; full VAL-024 initialization remains outstanding. `mkdirat` returns no descriptor, so an indistinguishable same-UID replacement before first observation is outside this observed-boundary guarantee. Independent close-on-exec descriptor ownership lets children outlive parents; shared handle state prevents value-copy double close. `Identity` is a cached observation, not live authority, and returns zero after closure. Missing required statx/mount observations fail closed without fallback. Local descriptor-helper checks do not substitute for a positive absolute-root topology or frozen-host qualification.

`Names(limit)` returns bytewise-sorted raw immediate entry names, including ordinary non-UTF-8 names and all entry types, without opening or following children. A negative limit or an integer-overflowing one-entry lookahead fails before I/O; zero admits only an empty directory. A fresh independent close-on-exec directory cursor avoids changing the retained handle’s offset. The complete operation, including cursor cleanup, holds the shared handle mutex against `Close` and revalidates the full retained ancestor chain before and after reading. The fresh descriptor identity must match the retained object before reading. Fresh identity, size/link count and required modification/change timestamps must then match across cursor acquisition and the bounded read; an observed change, invalid/duplicate name, excess entry, read error, or cleanup failure returns a fixed redacted error and no partial names. This is observed-change detection, not an atomic snapshot or protection against every unobserved same-UID mutation. Listing grants no child-content, role-overlap or destination authority.

`ReadFile(name, limit)` accepts one ordinary child component and a nonnegative, addressable limit whose one-byte lookahead cannot overflow; zero admits only an empty file. Under the shared handle mutex, full retained-ancestor/final-owner admission precedes a no-I/O `O_PATH|O_NOFOLLOW|O_CLOEXEC` pin. The child must be a regular file on the parent mount, owned by the supplied owner, without group/world write permission, with exactly one link and a size within the caller bound. Required device/inode/mount/owner/full-mode/link/size and modification/change-time observations must match the pinned descriptor, reopened descriptor and parent/name edge at the defined boundaries. Symlinks and non-regular files are rejected before readable opening. A bounded positional measurement pass reads through EOF/growth lookahead without allocating a whole result from reported size or caller limit. Only its successfully observed byte count, equal to the pinned initial size after required checks, admits exact-capacity result allocation. Reread through the same pinned readable descriptor and require the same length and SHA256; preserve all proc/file/ancestor boundaries and every close/error rule. Each actual requested read and EOF probe is charged where a lifetime read budget applies, including partial positive reads. Hash-only verification remains one pass; ordinary and TreeStage collection require two passes, and PinFile retains its separate post-construction full-content revalidation. All passes, fixed scratch, returned buffer and any later decode must fit their complete owning invocation schedule; no codec success alone proves Store/read-plus-decode fit. File, proc and ancestor checks repeat before completion. Every read, validation or close failure returns the fixed redacted error, nil bytes and zero identity. This clears returned results, not every Go backing allocation; it is observed-change detection, not an atomic snapshot or a guarantee against an indistinguishable fully restored cycle. No new fixed memory cap or parser-budget exception is introduced.

`CreateDir(name)` is the separate exclusive-creation seam; `OpenDir(name,true)` retains its existing-if-present behavior. Validate one component and the full parent chain, attempt one `mkdirat(0700)`, and admit only the first observed owner-owned exact-`0700` directory on the parent mount. Retain private successful-mkdir provenance and independent close-on-exec ancestor/child ownership. EEXIST returns a fixed redacted error preserving `errors.Is(err, syscall.EEXIST)`; only a caller explicitly permitted to use an existing structural container may separately open it. An expected-new child never adopts a collision. Every failure returns no Dir and leaves any residue untouched; no chmod, umask change, retry, removal or implicit sync occurs. The caller performs child and parent sync. As with existing creation, indistinguishable replacement between mkdir and first observation is outside the observed-boundary guarantee.

#### Pinned regular-file observation and retirement

`PinFile(name,limit)` retains one opaque `File` capability and independently returned bytes. It uses the existing full-chain/no-I/O-pin/proc-FD readable-reopen mechanism, with the same addressable bound and overflow probe, but narrows admission to an owner-owned exact-`0600`, non-executable regular file with exactly one link on the parent mount. It records complete `FileFacts`, including identity, size, modification/change stamps and SHA256, before returning after final revalidation. The same inode pin, readable descriptor, private proc capability and independently owned ancestor chain remain live through semantic recognition and removal; closing the original Dir has no effect. Construction or cleanup failure returns nil File, nil bytes and a fixed redacted error. Reading grants no writes, publication or cleanup authority and may affect atime.

File copies share one mutex-protected descriptor lifetime. All descriptors are close-on-exec. `Facts` returns a copied cached observation, never live authority, and zero after closure/invalidation. `Revalidate` compares the complete facts, bounded hash, retained ancestors and current parent/name edge; any failure refuses and invalidates/closes the capability. Returned bytes cannot mutate the retained facts. `Close` is idempotent and never removes an entry; clear shared ownership before closing, never retry a descriptor close, and preserve primary plus cleanup errors.

`Remove` is one-shot and terminal on every attempt, including failed prevalidation. First revalidate the exact live facts/hash/chain/edge, then attempt one ordinary descriptor-relative unlink. A pre-unlink failure is fixed ordinary refusal; every failure at or after the unlink attempt is fixed removal uncertainty. After successful unlink require the retained original inode's link count to change from one to zero, permitting only unlink-induced ctime change while all other identity, mtime, size and bytes remain exact. Read through the same descriptor opened while links equaled one; never reopen an unlinked inode through proc or relax generic linked-file admission. Synchronize the parent, require final no-follow ENOENT at the name, and revalidate the retained chain and post-unlink file before closing everything. Success requires cleanup success. Never retry unlink, adopt/delete a replacement, roll back, or claim absence after failure.

The live pin prevents inode recycling across semantic validation and retirement; ordinary unlink is not inode CAS. A replacement between the final check and unlink may be displaced and postchecks cannot undo it; no invisible restored-cycle guarantee is claimed. State independently owns exact witness/residue cleanup authority and the required live leases/graph. File possession, bytes or facts never grant that authority. The state caller excludes stable locks, target/root/owner records and unrelated files from removal. `RemoveTree` is not a substitute for this complete file-facts contract, and state uses neither pathname `os.Remove` nor replacement staging to manufacture a witness pin.

The owner-approved readable-reopening policy adds one narrow kernel-resource trust exception. Each read owns a private, close-on-exec proc capability with deterministic cleanup and no global cache or caller-selected proc path. It opens fixed `proc` relative to the retained current kernel-root descriptor without following a user path, requires real `PROC_SUPER_MAGIC`, pins kernel `self` without following it, and reads that pinned link into a bounded buffer to require the generated current PID. Numeric PID and `fd` directory traversal uses `openat2` with `BENEATH|NO_SYMLINKS|NO_XDEV`. Descriptor identities, parent/name edges and common proc filesystem/device/mount identity, including pinned `self`, are revalidated; proc synthetic ownership is not managed-directory ownership and is never compared with `Dir.owner`. Only the generated decimal number of the still-owned, validated regular-file pin may be followed as the final proc-fd magic link for read-only nonblocking close-on-exec reopening. The reopened full file observation must match before the first read. Initial root-to-proc and final magic-link mount crossings are deliberate; constrained no-crossing traversal applies inside procfs, not to that final jump. The displayed link target pathname is never resolved or used as a fallback. This policy trusts the kernel, process memory/FD table and namespace administrators while treating filesystem writers as untrusted. Missing, restricted, mismatched or unsupported procfs/openat2 observations refuse the operation; no mounting, permission repair, access/security-setting change or weaker path fallback occurs. It does not qualify procfs availability, actual privileged mount races or frozen-host support merely because a cloud helper succeeds.

The read-only numeric-FD reopen also sets `O_NONBLOCK` so an already established conflicting kernel file lease cannot make this open wait for its holder. Refusal still returns no bytes and zero identity through the existing fixed read error; it is not an OFD-lock busy result. Normal kernel lease-break notification may still occur. This flag does not impose a deadline on regular-file reads or other filesystem I/O and does not bypass any file, proc or ancestor validation.

The separately approved lock-reopening policy admits only a still-owned no-I/O pin for a regular file of the supplied owner, exact `0600`, one link and the parent mount. A separate private operation follows only that pin's generated numeric proc-fd link with `O_RDWR|O_CLOEXEC|O_NONBLOCK`; it never reads or writes file data, truncates, appends or falls back to a pathname. The full device/inode/mount/owner/mode/link/size and modification/change-time observation must match the pin, writable descriptor and retained parent/name edge before and after protected operations. The lock policy does not widen the readable-reopening operation. `O_NONBLOCK` prevents a conflicting kernel file lease from blocking the reopen; ordinary kernel lease-break notification can still occur. A refused reopen is an operational error, not evidence of OFD-lock contention.

An existing admitted lock may contain bytes because locking never reads content or allocates from file size; its captured full observation must remain unchanged. State initialization separately owns exact prefix/record/content recognition. Optional creation uses no-follow, same-mount `O_CREAT|O_EXCL`, immediate requested `0600` and close-on-exec/nonblocking descriptors. An observed absence followed by a collision refuses without adopting the raced object. Retain the exclusive-create descriptor until its initially empty inode, full observation, pin and writable reopen match. Every admitted open, including an existing-inode contender or repair, file-syncs that same validated writable descriptor and repeats observation/proc checks before return. Synchronizing only the creator would leave an early opener dependent on a creator that may die before syncing. File-sync or validation failure closes the owned resources and returns fixed refusal. This persistence operation neither reads nor changes contents. A restrictive umask may cause refusal; never repair permissions, alter umask, truncate, unlink or replace a failed or existing synchronization inode. The caller still uses `Dir.Sync` for parent-directory durability; this constructor does not establish that boundary or complete initialization. Independent owned ancestor, pin, proc and writable descriptors allow a lock to outlive its parent `Dir`.

`Sync()` synchronizes only the retained directory descriptor with `fsync`, holding the shared handle mutex against `Close`. It requires the full retained ancestor chain and final supplied owner before the operation and revalidates the chain afterward, including after a syscall failure. The directory syscall boundary checks safe-directory status and retained device/inode/mount/owner/type/full-mode identity before and after synchronization; observed zero links fail, while ordinary size/link growth remains permitted. Invalid/closed handles, missing required observations, observed substitution, syscall failure or revalidation failure return the fixed redacted directory error without retry or fallback. It never recursively synchronizes children or other directories. Success does not establish child-file durability, atomic publication, crash/power-loss behavior or frozen-host filesystem qualification.

Before any protected product/manager path creation or use, safefs enforces the explicit protected-role reservation and topology contract below. ARCH-016's state/data/cache namespace parents stay mutually disjoint; only its named parent/child layout is permitted. Operational peers and cross-domain aliases remain forbidden. Pre-trust home comparison is textual only; real destination checks require exact trust and A-X+W-X. No role observation grants initialization, content, destination or cleanup authority.

#### Protected-role reservations and topology

The owner approved explicit layout containment, read-only missing-root reservations and a bounded validated kernel-topology capability. This provisional foundation implements the existing ARCH-030 `OpenRoles`, `Add`, `Validate` and `Close` seam; it grants no state, initialization, trust, destination or cleanup authority. It does not qualify a filesystem, namespace or host.

**Closed layout matrix.** State, data and cache are distinct namespace parents, pairwise non-containing and physically non-aliasing. Only the relationships below are admitted; equality with a namespace parent's own root is never permitted.

| Namespace parent | Permitted operational descendants |
|---|---|
| Data | Source at/below `sources`; Tree at/below `trees`; Toolchain at/below `toolchain`; Manager at/below `installs` |
| Cache | Staging strictly below `instances/<32-lowercase-hex-instance>/downloads` |
| Staging | Separate Source, Tree, Toolchain and Manager directories strictly inside that workspace |
| State | None; its control paths/future leaves remain reserved |
| Source, Tree, Toolchain, Manager, Destination | None |

Repeated operational roles are permitted, but every operational peer, including same-role roots, is disjoint in both directions. Staging workspaces are disjoint and do not nest. Cache containment of an operational root is permitted only transitively through its registered Staging envelope; it is not a general cache-descendant exception. Existing service owners choose the workspace's child names; this table adds no manager configuration layout. Data-side same-parent temporary source/tree/toolchain roots carry that operational role in a transient set, not an unlisted Data-to-Staging permission. Every other equality, containment or alias rejects. Approved logical containment admits only the identical namespace route to the same physical region; another bind route is an alias, not permission.

The cache instance spelling is syntax, not current-instance authentication; the owning state/service must independently validate its current marker/instance relation before use. Added directories retain their explicit supplied-owner policy; never infer a new owner from observed ownership. Every directory still satisfies the unchanged UID-0 structural-prefix then supplied-owner chain, mode, no-follow and mount checks. Mapped-manager inner namespaces gain no exception and require their separate runner/service integration proof. `RoleDestination` denotes only an actual existing destination directory, registered after trust and mutation authorization under A-X+W-X. Never register the whole HOME as a destination. Missing/ordinary-file target acceptance and pre-trust textual future-leaf comparison remain home-owned work, not claims of this increment.

**Read-only opening and reservations.** `OpenRoles` validates constructed immutable `paths.Roots`, canonical distinct targets and the entire proposed relation before creating anything. It opens each existing absolute prefix without following links, retaining independently owned close-on-exec ancestor descriptors through the first ENOENT. Only ENOENT is absence. Existing directories obey the unchanged directory admission policy; newly created directories must be exact `0700`. A complete root is supplied-owner-owned; incomplete prefixes may end in the permitted UID-0 structural prefix. Store each exact target, deepest pinned prefix and ordered missing components. Reserve the prospective target coordinate (prefix coordinate plus missing suffix), not the entire existing ancestor's subtree; shared safe HOME or `.local` ancestors do not make sibling roots overlap. State control future leaves and fixed data/cache namespace branches remain reserved through their namespace parent; a reservation authorizes no creation, read of contents or adoption.

`Validate` is observational: revalidate every retained chain, first-missing boundary, namespace and topology relation; never discover/adopt an appeared component, change an identity or shrink membership. A visible mismatch of retained evidence poisons the shared set until Close. An invalid proposed Add argument, disallowed candidate or duplicate refuses transactionally without damaging a still-validated prior set. A transient observation failure confers no continuation authority.

**Explicit `Add` meanings and lifetime.** Base roles are created by OpenRoles and are not ordinary repeatable registrations. While a base reservation is incomplete, `Add(baseRole, nextDir)` binds exactly ONE next missing component. The old pinned prefix and every edge must match; no skipped component or root rebind is permitted. The new final edge must carry private successful `mkdirat` provenance from `Dir.OpenDir` and be exact supplied-owner `0700`; opening an existing child with `create=true` does not supply this fact. Capture this private fact only after a successful exclusive mkdir, and retain the existing limitation that mkdir does not identify an invisible replacement before first observation. The actual state caller must perform the creation immediately within its admitted flow; provenance is not a state lease/token.

Construct a temporary candidate updating all reservations sharing that exact canonical prefix/name edge by the same new identity. Revalidate every old edge, the new descriptor/name edge, all unaffected absence boundaries and the complete candidate topology and successfully finish/close operation-owned topology resources before atomically committing the advances. Validate is not called unchanged at the entry to this transition: only this designated absent-to-present edge may differ. Physical alias routes that cannot be jointly proven refuse; never partially adopt one route and leave another appeared frontier pending. Once a base is complete, any further Add of that base, including an exact duplicate, refuses.

For an operational role, Add duplicates an admitted caller Dir under its shared lock, validates its full chain and complete proposed matrix, and successfully finishes/closes operation-owned topology resources before committing membership. Unknown role, nil/closed input, duplicate, alias or disallowed placement refuses. Caller closure does not close retained descriptors. Each Roles value copy shares one mutex-protected lifetime; operations serialize against Close. Close clears each owned descriptor before attempting close, reports all cleanup failures, never retries a released descriptor number and is idempotent across value copies. It never closes caller Dirs, removes entries, releases a lease or repairs permissions.

Membership never shrinks. A transient set is closed before retirement/promotion of its registered root, under a separately surviving parent/reservation set and operation lease; final target registration is fresh. Validate never forgets a missing member or rebinds a staging path after rename.

**Initialization authority boundary.** Roles records physical observations, not authorization. Minimum safe preparation requires an initialization-admitting intent AND a state-owned no-write bootstrap eligibility predicate: descriptor-safe state/data are each absent-or-empty, checked by bounded topology/names only, with no durable-record parsing or exported unlocked Classify. Revalidate eligibility immediately before/after each exact preparation action, accounting only for its own captured state-prefix/init.lock entries, and again under I-X. Pre-existing nonempty/recognizable evidence roots route read-only/no-write; an unknown appearance or collision stops. Only after that predicate may preparation create missing exact state-prefix/state-root directories and stable empty init.lock needed to obtain I-X; it creates no claim, data/cache marker or semantic durable state. It validates the whole reserved topology before/after each step; observed collisions/substitutions stop that attempt. A later fresh operation may reopen/classify through the ordinary acquisition path. Once I-X exists and is held, further data/cache materialization and claims remain under the exact initialization authority and ARCH-017/031 ordering. Doctor never prepares these paths. This makes the existing minimum-prelease-topology rule explicit, not an alternative writer or recovery lease. The bounded Roles implementation includes no state executor; initialization/admission implementation and every bootstrap prefix require separate focused integration review before use.

**Kernel metadata capability.** Each Roles operation locks its current OS thread from the first topology/path observation through resource cleanup. The private capability validates current kernel root, real procfs, pinned `self`, generated PID and generated TID, then opens only `/proc/<pid>/task/<tid>/mountinfo` and `/proc/<pid>/task/<tid>/ns/mnt`. Numeric traversal is beneath validated proc descriptors with no symlinks or mount crossings. Mountinfo is read-only, no-follow, nonblocking and close-on-exec, must be a regular procfs entry on the retained proc mount, and is bounded before parse. No displayed proc pathname is resolved.

The one additional fixed namespace magic-link exception pins and validates `ns/mnt`, checks its bounded exact `mnt:[positive-decimal]` spelling, then follows only that fixed final entry. The resulting close-on-exec descriptor must be NSFS, have inode identity equal to the positive decimal in `mnt:[number]` (not the proc symlink inode), and report `NS_GET_NSTYPE == CLONE_NEWNS`. Retain a duplicate for the Roles lifetime so namespace inode reuse cannot impersonate the captured namespace. Every operation matches its actual syscall thread's namespace, root and proc evidence before/after; PID/TID changes within an operation or any namespace change refuse. A later operation may run on another locked OS thread only when it matches the same retained namespace and root. There is no arbitrary proc input, global topology cache, mounting, setns, permission repair or fallback.

The kernel, process memory/FD table and namespace administrators remain trusted per the existing proc boundary. Metadata reads and namespace pinning do not grant authority to change a namespace. They do not extend the managed-child trace allowlist or supply proc descriptors to children.

**Finite topology grammar and limits.** Accept at most 8 MiB with only a one-byte EOF/overflow probe, 16,384 complete newline-terminated records, 64 KiB per line, 4,096 decoded bytes/256 components per root or mountpoint, and 64 optional fields per record. Admit at most 64 total role reservations/registrations, 256 relevant descendant mounts, 1,024 footprint cones and 4,096 simultaneously retained owned descriptors. The path bound also applies to role targets/reconstructed prefix paths; checked derived physical/inverse-mapping coordinates are at most 12,288 bytes. Check limits/overflow before allocation, growth or duplication; exhaustion refuses without partial result. These are explicit implementation availability limits, not qualification or a whole-process runtime-memory guarantee.

Require exact mandatory mountinfo fields/separator, positive overflow-checked mount/parent IDs, valid major/minor and no duplicate IDs/tags. Decode only kernel path escapes `\040`, `\011`, `\012`, `\134`, once; reject malformed/other escapes, NUL, truncation, noncanonical absolute root/mountpoint, empty/dot/dotdot components and unsupported coordinate semantics. Paths remain Linux byte strings, without Unicode normalization. Source/options are bounded metadata, never identity or permission. Kernel idmapped presentation does not alter physical coordinates; keep the unchanged explicitly supplied statx-owner chain checks without inferring underlying unmapped UID ownership or granting a mapped-manager exception. Alias checks never use owner equality to hide a physical overlap. Optional tags are exactly unique `shared:<positive-decimal>`, `master:<positive-decimal>`, `propagate_from:<positive-decimal>` or `unbindable`; unknown optional representations, refuse.

Require one unambiguous namespace `/` record matching the actual root descriptor MountID. Only the namespace root may have an unavailable parent or a self-parent edge; all other parent IDs exist and are acyclic with component-wise containing-or-equal namespace mountpoints. Unrelated mount stacks may exist; reject duplicate/stacked mountpoints in the role-relevant closure (mountpoints that are ancestors/equal of an existing/prospective target or descendants inside it, plus their parent records). Root `/` remains unique. Hidden-root/disconnected graph or chroot ambiguity refuses; never select a supposedly visible row by longest-prefix guessing. The filesystem root field need not be `/` (for example a Btrfs subvolume root). Unrelated mount types may exist, but relevant role/prefix/mount regions initially require reviewed Btrfs or tmpfs coordinate semantics; unknown/FUSE/overlay/network regions refuse rather than being assumed disjoint.

**Descriptor-correlated physical footprints.** For every pinned existing chain edge associate its descriptor MountID with exactly one trusted mountinfo row, match the actual mount-transition edge to that row's namespace mountpoint/parent relation, and verify filesystem type. Use its row's superblock major:minor plus `row.root + suffix(namespacePath, row.mountpoint)` as the physical coordinate. Do not identify a row by longest-string-prefix alone. Mountinfo reports superblock device while Btrfs statx reports a subvolume anonymous device: never require these two device fields equal. Retain descriptor device/inode as independent substitution/alias evidence; require actual coordinate/identity/type correspondence. Relevant mountpoints are pinned by no-follow descriptor traversal; unsupported non-directory mountpoints or unprovable associations refuse.

A role footprint is its root coordinate cone (including prospective missing suffix) PLUS the coordinate cone of every mountpoint strictly inside its namespace target. Include interior mounts even when not registered as another role; this preserves the required private staging and nested immutable archive-cache tmpfs while detecting interior cross-domain bind aliases. Every cone retains its namespace root. Compare every pair with the same superblock identity: physical cones intersect on component-wise containment. Forbidden role pairs reject any intersection. For an approved namespace parent/child pair, map the intersection physical root back through BOTH cones (`namespaceRoot + suffix(intersection, physicalRoot)`); both namespace results must match. Apply that same inverse-mapping equality test within each role to reject interior self-alias routes. Independent tmpfs regions have distinct physical domains; equal mount IDs are not inherently conflicts and different mount IDs are not disjointness proof.

Whole cones conservatively include portions hidden by overmounts. This may reject a harmless arrangement, but must never omit a prohibited alias. No subtraction, automatic alias correction or fallback mapping is implemented. A wrong device/inode relation, filesystem association, future coordinate or inverse mapping refuses.

For Open/Add/Validate, capture bounded mountinfo A, validate graph/coordinates/pins/absence/matrix, revalidate root/proc/thread/namespace and all descriptors, then capture B and require byte-identical topology for that operation. Require final observation and resource cleanup to succeed before returning success. Recompute at later boundaries; unrelated mounts need not be frozen for the whole set lifetime. Any observed retained-evidence mismatch poisons the set; permission/unsupported/ambiguous observations fail closed. These are checked-boundary facts, not a kernel transaction, inode CAS or proof against an indistinguishable restored cycle between checks. Actual Btrfs, mount/bind, namespace, restricted-proc and host qualification remain unexercised until real evidence exists.

Mechanism references: [kernel mountinfo emission](https://raw.githubusercontent.com/torvalds/linux/v7.1/fs/proc_namespace.c), [Btrfs getattr device reporting](https://raw.githubusercontent.com/torvalds/linux/v6.18/fs/btrfs/inode.c). Source-derived behavior and local parser/helper tests do not establish the frozen Fedora kernel's complete proof matrix.



Schema, path, archive, composition, hashing, and emitted-output limits are checked before their corresponding allocation or decode. Parser/model backing storage uses PC-042's checked conservative reservation for the complete API invocation, including parser passes, buffers, AST/tracker storage and retained models; it is not literal cumulative Go allocation enforcement or a hard runtime-memory cap. Native Git necessarily parses protocol and pack input internally, so ARCH-006's external quota/resource/watchdog boundary limits that untrusted parser before ZERB performs bounded object inspection. Unsupported aliases and object types fail rather than being normalized into a second semantic representation.

The bounded `PublishNoReplace(name, data, mode)` increment holds the shared directory lifetime while publishing one ordinary child on the retained parent mount. Its requested mode contains permission bits only and no group/world write bits; the exact created mode must match without permission repair. Durable record callers still select ARCH-016's `0600`. A close-on-exec `O_TMPFILE` inode remains unnamed while all bytes are written, verified through its retained read/write descriptor and file-synced. The same still-owned descriptor is linked atomically with `linkat(AT_EMPTY_PATH)` to the supplied child name, without replacing any existing entry. The kernel must admit the descriptor's unchanged open-time credentials; unavailable anonymous-file or descriptor-link support refuses without capabilities, proc linking, named staging or fallback. This uses the kernel's owned-descriptor publication primitive, not an extension of either proc reopening exception.

Full ancestor/owner admission surrounds each publication phase; local file and parent checks bind exact type/mode/owner/device/inode/mount, size, required timestamps and zero links before publication, then exactly one link afterward. Link-induced change-time/link-count changes are expected only at that boundary. After linking, parent-directory synchronization, no-I/O final-name pinning, exact identity/edge validation and positional full-byte verification must all pass, along with resource cleanup, before returning an identity. The caller's bytes and directory/lease ownership remain unchanged. Every error returns zero identity; an attempted-link or later error reports fixed redacted publication uncertainty and may leave the complete final entry visible with durability or verification unresolved. No error establishes absence or grants cleanup authority. Never unlink, replace, retry publication or claim rollback after a link attempt; a caller must retain its own admission and apply its owner-defined validation/recovery contract. This primitive grants no role-overlap, initialization, state-completion or storage/host qualification authority.

#### Staged replacement and operation-record contract

The owner approved the explicit stage and metadata-only operation-record direction. The provisional non-release foundation below replaces the unimplemented monolithic `Dir.Replace` seam. It does not activate state initialization, production recovery or a public command. VAL-027/030 retain their real-host and storage qualification requirements.

**Filesystem capability.** `StageReplacement` pins an existing predecessor, compares every supplied identity field before content I/O, captures its complete content hash and timestamps, and prepares/verifies/syncs one anonymous same-parent stage. Missing or zero predecessor is an error, never upsert. The capability duplicates the validated ancestor descriptors and owns its predecessor/stage pins; it holds no borrowed `Dir` mutex after construction. The caller may close its directory independently. Ordinary Go value copies share one serialized lifetime. Each link/publication is one-shot, including failed attempts. `Facts` is a cached, side-effect-free construction observation, not a live revalidation or authority; it returns copied facts, or zero after a validating action invalidates/closes the capability. `Close` closes owned descriptors only, never an entry, caller directory or lease; cleanup errors remain errors, ownership is cleared before each close and descriptor-number retries are forbidden.

`FileFacts` contains the complete `Identity`, modification/change seconds and nanoseconds, and lowercase exact-byte SHA256. `ReplacementFacts` contains the parent identity, distinct single-component target/temporary names, and predecessor/stage facts. The immutable staged facts describe the anonymous zero-link inode. Linking admits only zero-to-one link count and its ctime change; rename admits only the stage's rename-induced ctime change and predecessor's one-to-zero link count/ctime change. All other fields and bytes remain exact. During a live operation the newly observed complete stamp is retained at each allowed transition. A restart cannot know a link/rename-generated ctime in advance: it captures a fresh full stamp, verifies the exact inode, expected link count, original mtime, remaining identity and complete bytes, then requires that full stamp to remain unchanged throughout recognition/cleanup. This is not proof against an invisible restored same-UID cycle.

`Link` names the synced stage without clobbering, then syncs/verifies the parent and exact entry. State must durably publish/reopen its record before calling it. `Publish` requires that exact linked stage and predecessor, attempts one ordinary same-parent rename, syncs the parent and reopens/verifies the complete successor. State revalidates its semantic relation between calls. No method infers record durability, process death or lease authority. Failure before a rename attempt grants no retry/absence/cleanup right; after an attempt, publication/durability/verification is uncertain. Every error returns zero success identity. Publish is terminal: every call consumes the capability, validates/closes all owned descriptors before returning, and returns zero identity on any final-validation or cleanup failure. Subsequent Close is idempotent. A separate abandon/failed-link Close likewise never removes names. Ordinary rename and unlink are not inode CAS: substitution between the last check and either syscall can displace/remove an unexpected inode. Before/after checks refuse an observed mismatch, cannot undo displacement and do not prove invisible restored cycles absent.

**Record namespace and schema.** Exactly one optional `state/zerb/replace-witness.json` is reserved, mode `0600`, complete no-replace publication, file/parent sync and reopen before any stage link. The only temporary component is `.replace-<replacement-id>` in the admitted target's parent; the ID is fresh 32-character lowercase hex, distinct from instance/init/writer/operation tokens. Journals use their admitted journals directory; other targets use the state root. No other temporary name is recognized. The record is immutable; never replace a witness to clear its own occupied slot. No target, stable synchronization inode or immutable object is removed by this protocol.

The private state codec is exactly schema 1, domain `zerb.replace-witness`, at most 16384 bytes including canonical JSON encoding. It has no payload or extension field. Ordered top-level fields are `schema`, `domain`, `instance`, `id`, `publisher`, `authority`, `files`. `publisher` uses ARCH-024's `processIdentity`. `authority` has ordered fields `kind`, `generation`, `token`, `operation_id`, `operation_token`. Its closed kinds are `initialization`, `writer-acquire`, `writer-transfer`, `writer-release`, `journal-phase`, `journal-transfer`, `current-commit`. Initialization binds the exact existing claim generation/token. Completion requires its current claimant/publisher; takeover requires positive old-claimant death and the exact fresh token/generation/process transfer to the new publisher. Claim takeover advances the prior generation once (overflow refuses) with a fresh token; completion preserves its original claim generation/token as live authority facts while the initialized successor clears token/process/generation under ARCH-024. `journal-phase` and `current-commit` bind the exact held writer generation/token/process and operation ID/token. `journal-transfer` instead binds the dead predecessor held-owner generation/token and the unchanged operation ID/token; its journal successor advances once to a fresh writer token/new publisher before the matching owner transfer, exactly as ARCH-027 requires. Acquire binds the released predecessor generation with empty token; transfer binds the dead held predecessor generation/token; release binds the live held predecessor. In these three owner cases `publisher` is the newly acquiring/transferring or releasing process, independently checked against the exact role transition. Non-journal/current operation fields are empty. No other optional authority data is admitted.

`files` has ordered fields `parent`, `target`, `temporary`, `previous`, `staged`. An identity uses ordered `device`, `inode`, `owner`, `mode`, `links`, `size`, `mount_id`; a file fact uses ordered `identity`, `modified_seconds`, `modified_nanoseconds`, `changed_seconds`, `changed_nanoseconds`, `sha256`. Integer widths equal the ARCH-030 types; seconds are int64 and nanoseconds uint32 less than one billion. Parent is an owned `0700` directory with nonzero links; comparison preserves device/inode/owner/mode/mount but excludes directory link count/size, which legitimate child publication can change. Record files are exact `0600`, same-device/mount safe regular inodes, with distinct nonzero inode identities, predecessor link count one and stage count zero, exact restrictive permission modes, nonnegative bounded sizes and 64 lowercase hexadecimal hashes. Target is the admitted root/owner/current component or the exact operation journal component; locks, witness, temporary or immutable paths are forbidden. Root replacement uses initialization authority only; owner replacement uses its three owner kinds; journal/current use only their matching kind. Decode bounds bytes before parsing, requires one complete canonical value with EOF, exact field order/spelling and no unknown, duplicate, aliased, omitted, null, unsupported or noncanonical representation. Encode validates before output allocation. The private metadata codec/recognizer is not an exported state service or generic record decoder.

Scalar validation is closed: instance/ID and every nonempty token or operation ID are exactly 32 lowercase hexadecimal characters; ID differs from instance and every token/operation field. Publisher PID/start/PID-namespace are positive. The authority token is empty only for writer-acquire. Journal/current operation fields are required, distinct from one another and the authority token; other kinds require both empty. The journal target is exactly `<operation_id>.json`; other role targets are the exact components above. Held-predecessor/current kinds require positive generation, acquire permits zero, and initialization permits zero. Acquire, owner-transfer/release and journal-transfer reject uint64 generation overflow; retained journal-phase/current generation may equal uint64 max. Initialization completion may retain max; independent takeover admission must reject its overflow. File sizes are nonnegative and less than platform MaxInt, matching safefs' existing read bound; this metadata codec adds no payload-size authorization. No further scalar limits or authority fields are implied.

**Independent maintenance admission.** The operation record is evidence, not authority or proof of historical authorship. Record hashes, self-described process identity and process death never suffice. The state owner must first acquire and revalidate exact stable kernel exclusion and independently validate the complete protected graph with the record/its one named residue factored out, including root/data instance, unchanged lock inodes, and the permitted predecessor OR successor role transition. A released predecessor does not independently name a future publisher; the contract does not claim it does. Its exact generation/status plus the independently validated graph authorize only the closed acquire transition. The observed-boundary same-UID threat model applies equally to owner and witness records; this is not cryptographic authentication against arbitrary complete namespace forgery.

To avoid recursive writer acquisition through an occupied witness slot, this is a narrow exception to ARCH-023/027/031: under an exclusive intent admission and exclusive verified writer lock, **before** publishing a new held writer, the state owner may retire only an exact interrupted replacement record/residue. It may not rename/replay a target, publish an owner/journal/current, execute a manager or act as a normal writer. INIT maintenance uses I-X only for an exact initializing prefix; once the target is initialized it requires current-state A-X then W-X, with no I-X fallback or shared upgrade. Final-root publication itself retains I-X and the exact requested target A-X or A-X+W-X handoff lease, as ARCH-031 already requires. Restart maintenance of any initialized target requires W-X. Live final-root completion instead retains the original I-X and exact target A-X or A-X+W-X through record retirement before handoff; it never temporarily appends/drops W-X for a plan. Maintenance completes before acquiring a fresh owner or returning ordinary services. An A-S intent never cleans; normal commands encountering residue fail according to their row; only exact admitted repair performs this maintenance. No semantic probe before admission, lease drop/reacquisition or changed lock order is introduced.

Live completion/cleanup requires the same retained publishing session, exact current process and original lease/role relation (including released-owner or initialized-root successor intervals). Restart maintenance instead requires positive full interrupted-publisher death, fresh exclusion and the independently valid complete graph. Unavailable/reused/live process evidence refuses. A dead prior owner is required only for a transfer that already requires it; a released predecessor's former process need not die. The private recognizer accepts only separately constructed, independently validated expected authority/facts and fresh filesystem observations; it never derives authority from decoding the record and performs no I/O, cleanup or process probing. Production integration must construct those facts through the existing state-owned admission/graph path, not a caller boolean or callback.

**Closed crash prefixes.** Before the record is linked, anonymous-stage death leaves no operation name. An interrupted no-replace record publication can leave either no record or a complete record; malformed/partial records are BAD. Successful live retirement and exact restart repair follow the table below. All absence checks distinguish ENOENT from observation failure. Every present object is pinned, fully checked and revalidated immediately before/after action. Parent sync and absence-reopen are required even when a preceding unlink may already have happened. The record's own exact pin/bytes are retained until its final unlink; it is removed last.

| Record | Target | Temporary | Independently admitted action |
|---|---|---|---|
| Absent | Exact ordinary prior/successor graph | Absent | Ordinary graph classification; never infer/replay an operation from absence |
| Exact record | Exact predecessor | Absent | Anonymous stage lost or temporary already removed; sync/revalidate parent, retire record only |
| Exact record | Exact predecessor | Exact staged inode, one link | Unlink only that witnessed temp; sync/absence-reopen; revalidate predecessor and graph; retire record |
| Exact record | Exact successor and valid claim-takeover/INIT-completion/owner-transition/journal-phase/journal-ahead/current-cleanup graph | Absent | Sync/reopen/reverify successor; retire record; never rename or replay |
| Absent after record unlink | Exact admitted predecessor/successor graph | Absent | Parent sync/absence-reopen; ordinary classification only; no new historical operation claim |
| Any other combination | Missing, substituted, unsafe, unsupported or unrecognized | Missing/extra/mismatched/both temp and successor | BAD, evidence preserved; no namespace action |

Witness-only after temporary removal is identical to witness-only before link and intentionally needs no replay. Death after temp unlink but before sync, after sync before absence verification, after record unlink before sync and after final sync before verification all re-enter the corresponding exact row. A failed observation is not absence. A collision at the reserved temp name is never adopted, even if byte-identical; wrong inode/links/content/mode/owner/mount/ancestors or an unrecognized extra entry prevents maintenance. Cleanup never reconstructs a lost stage or successor, restores a predecessor, removes a target or accepts a stable lock as residue. Every role's before/after replacement must additionally satisfy ARCH-017/024/025/027/031; the table is filesystem shape recognition, not permission to accept an arbitrary state graph.

The bounded first implementation consists only of the descriptor primitive, private metadata codec and pure prefix recognition against independent expected evidence, plus disposable failure/kernel tests. It contains no maintenance executor or production state admission/initialization integration. Codec acceptance and a recognized shape cannot themselves trigger writes. The full lease/process/graph and power-loss matrix remains required before production integration or qualification.

#### TreeStage lifecycle and resource contract

The owner approved the bounded ARCH-030 `StageTree`, `Root`, tree `PublishNoReplace` and `Cleanup` contract and implementation, conditional on concrete representation fit. The [representation review](treestage-representation.md) and focused layout/overflow tests close that design gate; they do not establish runtime admission, complete test-matrix execution or host qualification. The general `RemoveTree` seam and production persistence remain outside this slice. The [TreeStage failure matrix](validation.md#treestage-lifecycle-failure-matrix) specifies required future observations until exercised evidence supersedes it.

**Authority and confinement.** A stage capability derives only from this invocation's successful exclusive creation and retained, validated parent/child descriptors. Existing names, matching hashes, cached `Identity`, process death or a reconstructed pathname never supply cleanup authority. The caller retains independent lease, content-authentication and role obligations. These methods neither acquire state admission nor authenticate archive/source bytes. A crash loses the in-memory capability: remnants are preserved, never rediscovered/adopted by spelling or deleted through generic `RemoveTree`.

Creation uses one safe child component on the admitted parent filesystem/mount, with exact supplied ownership and restrictive mode at first observation. Never adopt a collision, chmod an existing entry, change umask, follow a link or expand the proc exceptions. Existing mkdir provenance does not prove absence of an indistinguishable same-UID replacement before first observation. The stage independently retains parent/child descriptors; parent closure cannot revoke them. Ordinary stage value copies share one serialized lifetime.

**Private lifetime.** State names below are explanatory, not exports or a durable schema:

| State | Sole permitted action/outcome |
|---|---|
| Active | `Root` lends the admitted directory. One publication or cleanup attempt may consume the capability. Every stage-derived operation remains subject to its live subtree admission. |
| Published | Exact no-replace rename, content/parent synchronization, final reopen/inventory verification and operation-owned descriptor cleanup succeeded. Return the root identity; `Root` is nil. Later `Cleanup` is an idempotent descriptor-only no-op, never deletion of installed content. |
| Removed | Explicit active-stage cleanup verified/removed only its admitted subtree, synced the parent and verified absence. `Root` is nil. Repeated `Cleanup` does no filesystem work. |
| Preserve-only | Creation, identity, resource, cleanup or publication outcome is uncertain. `Root` is nil. `Cleanup` releases any still-owned descriptors, reports fixed failure and preserves remaining names; it grants no retry, rollback, absence or inferred success. |

A failed publication/cleanup attempt enters preserve-only, including a final close failure; success cannot survive cleanup failure. Before draining, a terminal action atomically stops new admissions, without holding a lent directory mutex while waiting. Revoke old views on both success and failure; setting `Root` to nil alone is insufficient. An attempted rename makes every subsequent error publication uncertainty, with zero returned identity even if the target appears correct. Invalid publication arguments refuse without consuming an active stage or performing named I/O; after terminal work starts, failures consume it. Publication on any consumed state returns zero identity/fixed error without named I/O. Repeated terminal calls cannot regain mutation authority or retry released descriptor numbers.

Any admitted descendant operation that observes a topology/content/identity mismatch, I/O/validation/cleanup failure or uncertain child publication poisons the shared stage: stop new admissions, drain/revoke covered operations and preserve names. Parameter-only and already-closed-view refusals before filesystem work do not poison a valid stage. Closing a view alone remains resource release, not a failure or mutation authority. Concurrent operations already admitted can have acted before poisoning completes; no rollback or instantaneous external-writer exclusion is claimed.

**No-clobber promotion.** Use descriptor-relative same-parent `renameat2(RENAME_NOREPLACE)` with distinct staging/final components. Preserve every existing target type; unsupported kernels/filesystems refuse without ordinary-rename, copy/delete, pathname or proc fallback. Revalidate retained ancestors and the exact stage edge/inode/owner/mode/mount around affected boundaries. Synchronize all admitted content bottom-up before rename, sync the parent afterward, then reopen and verify the exact root and bounded complete inventory. Root identity alone is not complete-content verification. Ordinary rename/unlink are not inode CAS: observed-boundary checks cannot undo displacement or rule out an invisible restored same-UID cycle.

**Cleanup.** Explicit cleanup addresses only the proven active stage, never its parent, final installed target, stable synchronization inode or unrelated sibling. Its live exclusively created root capability authorizes container cleanup of a complete safe bounded inventory, not historical authorship or authentication of each entry. No per-entry creation record is implied. Complete bounded no-follow prevalidation precedes its first removal. Nested mounts, links, unsupported types, unsafe owner/mode or observed substitutions refuse. New entries appearing after that snapshot are not adopted; observed mismatches stop removal. Pin/revalidate every acted-on parent/child edge; a partial removal followed by failure preserves the remainder without claiming rollback or an intact original tree. Never unlink an unvalidated entry to make cleanup pass. Prior inventory does not make a later unlink atomic.

**Approved implementation choices.**

1. **Descendant lifetime:** use a private shared subtree admission gate inherited by stage-derived directory handles. Terminal actions drain active operations and revoke later operations through previously lent handles before publication/removal. The first slice's exact coverage is below; any subsequently merged method requires explicit classification before implementation. Existing non-stage parent/child lifetime remains unchanged. Closing a lent root/child view releases that view only; the stage independently owns its root/parent pins, and another stage view does not gain deletion rights. Revocation refuses later I/O rather than closing caller-owned descriptor numbers. Caller-only quiescence is smaller but unenforced; closing only the root mutex does not prevent an in-flight descendant mutation. This coverage grants no permission to expand other safefs capabilities.

   | Stage-derived method | Required treatment |
   |---|---|
   | `OpenDir`; `CreateDir` if merged | Enroll the whole operation and inherit admission in every returned directory. |
   | `Names`, `ReadFile`, file `PublishNoReplace`, `Sync` | Enroll the complete operation, including validation and operation-owned cleanup. |
   | `Identity` | Existing cached observation only; grants no liveness or mutation authority. |
   | `Close` | Release that view only; terminal coordination must prevent lock-order deadlock and descriptor reuse. |
   | `OpenLock`, `StageReplacement`, nested `StageTree`; `PinFile` if merged | Refuse before effect; no escaping long-lived capability in this bounded slice. |

   `Roles.Add` is a separate observational lifetime, not a builder operation; its retained view must carry applicable stage admission and the caller must close the transient role set before promotion/retirement. A nonzero observer count causes pre-effect, nonconsuming terminal refusal without named I/O; terminal work never waits for role closure. Enroll every distinct stage gate before directory/role effects, with roster revalidation for a multi-stage set. No metadata duplicate may silently strip a gate. `Root` returns one constructor-created borrowed view with no per-call allocation, descriptor duplication or hidden failure channel. It returns nil after the stage is consumed or that view is closed. After draining, terminal disposal closes this stage-owned borrowed view through its shared state and all private pins. Independently returned child directories and retained role chains remain caller-owned, revoked to Close-only, and charged until their owner closes them. No new `Close` method exists for `TreeStage`; terminal `Cleanup` remains its explicit resource-disposal method.
2. **Creation ownership/mode:** require exact `0700`, rejecting other `mode` values. Constructor failure returns nil/error, internally releasing every known-owned descriptor while preserving all possible named residue. Clear ownership before each close; never retry a released descriptor number, repair restrictive umask or import another API's error-handle rule.
3. **Inventory bounds/residue:** use the fixed internal envelope below, under the existing signatures, with checked arithmetic and reservation before the next allocation/action. These approved limits are conservative refusal boundaries, not evidence that every retained artifact fits. Enforce cumulative build admission as content is added, not merely at final inventory. Bound every validation/removal pass and every retained result; streaming does not excuse unbounded total I/O. An explicit per-class limits input is the more flexible alternative but changes the frozen API and increases policy surface. Preserve crash/uncertain residue with no new witness, scanner, command or recovery schema. Restart cleanup would need a separate authority contract.

**Approved numeric envelope.** MiB/GiB below are powers of two. Counts include directories and zero-byte files; the implicit stage root is excluded from entry/path totals but its resources are charged. No value is inherited as authority from a proof fixture or PC-042's parser budget.

| Resource | Maximum | Rationale and exact accounting |
|---|---:|---|
| Descendant entries | 16,384 | A finite metadata/work envelope with headroom above two PC-042 inventories of at most 4,096 entries each; not a new component limit or proof all source layouts fit. Count every descendant once before creation or inventory admission. |
| Relative depth | 64 components | Matches the current logical-path depth envelope; stop before descending or allocating the next frame. |
| Component / relative path | 255 / 1,024 raw bytes | Preserve the existing logical path size envelope without inventing a UTF-8 requirement for generic filesystem names. Root-relative names and separators count; no truncation/normalization to fit. |
| Aggregate retained relative-path bytes | 16 MiB | Exactly 16,384 times 1,024; permits the maximum entry/path combination rather than imposing an undisclosed shorter-average-path rule. Charge each distinct stored path once, all copies separately to workspace. |
| One regular file | 256 MiB | A deliberately larger primitive envelope than current disposable Git proof payloads; exact mise/chezmoi expanded sizes are not authenticated by this proposal. Stat/creation checks precede reading or writing. |
| Complete regular-file payload | 512 MiB | Cumulative creation/inventory ceiling, including zero-length entries in the count bound. It bounds admitted tree contents, not disk quota or bytes an external writer can create. Recheck sizes and refuse observed growth/substitution. |
| Regular-file bytes read/verified | 4 GiB per stage lifetime | Count every actual content read, comparison/hash pass and lookahead, including builder reads, prepublication verification, postpublication verification and cleanup prevalidation. Reserve each requested read before I/O; do not reset the counter between methods/passes. Eight full 512 MiB passes are only illustrative headroom, not required work or unlimited retries. |
| Stage-owned descriptors | 256 live | Charge retained parent/root/ancestor pins, all lent views/descendants and operation/proc cursors together before opening/duplication. Source callers may need to close intermediate views; exhaustion refuses without increasing RLIMIT or changing host settings. No unbounded all-entry pin set. |
| Owned metadata/result/workspace | 128 MiB simultaneously reserved | Count the live stage/gate, all retained inventories/path copies, sorting/traversal work, concurrent method workspace and returned read buffers until transfer at return. Every allocation path needs an auditable conservative schedule; its complete reservation must fit before entry. This is a backing-storage reservation, not a hard Go heap/RSS/cumulative-allocation guarantee. |
| Streaming scratch | 128 KiB simultaneously reserved | Part of the 128 MiB budget, not an extra allowance; share/reserve across concurrent reads/hashes. Whole-file results remain separately charged to workspace and can therefore refuse below the 256 MiB per-file ceiling. Streaming verification must not allocate a whole payload. |

The payload and descriptor ceilings apply to building and to terminal prevalidation; cleanup cannot exceed them to delete a hostile oversized tree. A new descriptor/byte reservation failure before effects is a nonpoisoning admission refusal; an admitted operation that later exhausts a reservation, observes an oversized tree or fails I/O poisons the stage. Returned buffers become caller-owned after return; retaining arbitrary results across calls is outside the stage's simultaneous reservation claim, not an unlimited-memory guarantee. Use checked arithmetic, including on 32-bit integer widths. Lifetime byte/read-work counters never reset after unsuccessful operations; unused simultaneous reservations are released when their known-owned resources actually cease to be held.

The conservative reservation schedule is `1 MiB + 2,048N + 4P + 512F + S + R <= 128 MiB`: admitted inventory entries `N`, total distinct relative-path bytes `P`, all live stage-owned descriptors `F`, simultaneously reserved streaming scratch `S`, and new result/copy buffers not already included in the other terms `R`. The per-entry term covers bounded flat metadata/sort/traversal storage across all live inventories; the path term covers up to four retained/copied complete path inventories. At the simultaneous count/path/descriptor/scratch maxima with no result buffer this reserves 97.25 MiB, leaving 30.75 MiB for `R`. All constructor/descendant/concurrent-operation/terminal allocations must be classified before coding; a representation requiring more snapshots, path copies or per-entry space must be revised within the schedule or returned as a decision, never exempted from accounting. Persistent role-array and target-copy tickets also occupy `R`, jointly across all role sets; helper completion does not reset their ownership. A whole-file read may therefore refuse even while streaming verification of that file fits its separate payload bound. The layout proof establishes capacity arithmetic; actual implementation admission and allocation evidence remain separately required.

The aggregate path maximum alone is 16 MiB; the 128 MiB workspace leaves bounded room for multiple snapshots, metadata and sorting without equating input bytes to all allocations. The independently reviewed concrete representation fits simultaneous entry/path maxima; production types and helpers must be checked against that exact schedule. Any incompatible change must be surfaced rather than silently narrowing a bound. Before source/toolchain integration, authenticate actual frozen artifact/tree inventories and show that they fit; no artifact fetch or compatibility claim is made here. The separate native-mise Node inventory permits 200,000 entries and 1 GiB in its proof harness and admits symlinks: it is not this generic stage's resource class, and its installation is not added to this slice.

After the representation gate and independent security review, the bounded implementation is only the four existing tree methods, their minimal private admission/validation helpers and disposable tests. It excludes general-purpose deletion, archive parsing, CA materialization/acquisition, initialization/admission/repair integration, public routing, packages and owner-host operations. Existing role membership never shrinks: close the transient set before promotion/retirement under a surviving parent/reservation set and caller lease; register the final target afresh. Public positive topology, actual mount/bind/restricted-proc, Btrfs power-loss/storage durability and frozen-host qualification remain separate gates.

### ARCH-021 — Managed archive safety

A managed archive is fully authenticated and hashed before any parser receives its bytes. Extraction into a fresh protected root rejects absolute paths, traversal, empty or ambiguous names, normalized aliases, duplicate entries, symlinks, hardlinks, devices, sockets, unsupported types/modes, oversized entries, oversized totals, and entries outside frozen count/depth limits. ZERB verifies expected executable identity after extraction and before atomic promotion. Partial, corrupt, modified-after-hash, or interrupted artifacts never become visible as installed content, and cleanup cannot reach an outside sentinel.

The one native-mise Node archive is not extracted by ZERB and does not inherit the generic no-symlink extraction grammar above. After authentication and immutable publication but before mise, ZERB's bounded inventory parser strips exactly one archive root, rejects absolute/traversal/alias/duplicate/hardlink/special entries, and admits only directories, regular files, and canonical relative symlinks whose normalized target remains inside that root. Linux symlink mode is normalized to its non-mutable `0777`; regular and directory modes remain exact archive facts. Closed tracing fails every write escape. After mise, an independent no-follow walk requires the exact entry set, regular/directory type and mode, regular-file size/hash, symlink target, and `bin/node` digest/executable identity. An archived symlink may remain exact or materialize only as its exact authenticated regular target, the exact same-basename authenticated sibling in that target directory, or—only for `bin/npm` under mise 2026.7.17—the frozen public wrapper size/mode/content identity. That wrapper identity and normalization rule are adapter policy IDs bound into the candidate under PC-050. Every other type/content transformation fails. This exception is exact to the retained Node artifact and mise identity; no other archive or manager gains its grammar.

### ARCH-022 — Destination ownership and races

The real home destination is an intentional shared surface. A ZERB writer lock coordinates only cooperating ZERB processes and makes no claim about another manager or arbitrary writer. Before mutation and again at each affected boundary, ZERB validates every ancestor and final target with no-follow owner/type/mode/link/content checks. The path must be reached through ARCH-020's unchanged structural-prefix and expected-owner non-symlink directory chain.

Every normalized structural destination is compared without I/O against the reserved-role namespace during static composition; this catches textual equality/containment and not-yet-created protected/control leaves such as an absent `current.json` without touching the real home. Full live role-overlap, ancestor, alias, mount, and inode checks begin only after trust and mutation authorization under `A-X+W-X`, before any manager or filesystem mutation, and are revalidated at each affected boundary.

A pre-existing target not proven to belong to the exact stable receipt is a non-overridable conflict. A user-modified managed target is never silently overwritten. Any observed substitution, rename, hardlink, symlink, type, content, or ancestor race prevents stable completion and leaves truthful dirty state. Automatic old-target deletion, force, overwrite approval, and in-place profile switching are outside the MVP.

### ARCH-023 — Writer ownership

Writer ownership is not pathname creation, record existence, age, or token possession. It is the conjunction of an exclusive ARCH-031 admission lease, the held exclusive open-file-description lock on the verified stable `writer.lock` inode, a fully durable `writer-owner.json` publication naming the same fresh token/generation/process-start identity, and successful inode/token/generation revalidation. Initialization, takeover, writer, and operation tokens are distinct. Every journal and write-capable operation binds the active writer generation/token. A process that cannot prove the complete conjunction cannot advance a journal, receipt, current record, cleanup, or external mutation. The sole pre-owner exception is ARCH-020's exact replacement-record maintenance under independently validated exclusive kernel admission; it grants no writer, target-publication or external-mutation authority.

Staleness requires acquisition of the kernel exclusion after the prior holder released or died plus proof that the published process identity is dead; age and PID alone never suffice. Release atomically publishes a new durable released generation while the locks remain held and never unlinks or replaces a synchronization inode. ARCH-031 owns acquisition, compare-transfer, observed-boundary substitution defense, release, lease order, and crash boundaries.

### ARCH-024 — Versioned durable schemas

Every candidate, marker, initialization or writer-owner publication, journal, receipt, and current record is schema/domain versioned and encoded from explicit map-free structs with sorted collections and no authentication, provider, rendered, or ambient values. Stable synchronization inodes contain no mutable ownership record. Identity-bearing sensitive metadata required by PC-050 remains exact in protected candidates/receipts rather than being omitted or replaced by a public redaction. Exact authored versions, lock values, and the deterministic structural target inventory are present in the candidate before trust; no post-trust manager or live-destination observation may alter them or any digest field. Live destination topology/type/content/link/mode/owner observations occur only after trust and mutation authorization under `A-X+W-X`; journals and receipts persist them as preconditions/results and every affected mutation boundary revalidates them. Phase 1 qualifies/freezes exact JSON fields, enum values, byte/count/string limits, canonical serialization, replacement/sync protocol, and cross-record validation before release. Reviewed non-release implementation may use the exact provisional contracts only under delivery's cloud development gate; unexercised durability remains inconclusive.

The durable relationships are:

| Record | Required identity and role |
|---|---|
| State `init.lock` | Stable no-follow exclusive initialization/takeover synchronization inode; no replace/delete lifecycle |
| State `admission.lock` | Stable no-follow shared/exclusive command-admission synchronization inode; no replace/delete lifecycle |
| State `writer.lock` | Stable no-follow exclusive writer synchronization inode; no replace/delete lifecycle |
| State `writer-owner.json` | Atomic released/held publication with schema, strictly advancing generation, random token when held, PID/process-start identity, acquisition/release facts, and predecessor relation under ARCH-031 |
| State `replace-witness.json` | Optional immutable metadata-only interrupted replacement evidence under ARCH-020; never authority by itself |
| State `root.json` | Initialization status, schema, instance identity, current initialization ownership token, and claimant evidence |
| Data/cache `owner.json` | Schema and matching instance identity; cache marker confers namespace ownership only |
| Candidate | PC-050 digest plus exact PC-022/PC-043 source/locator/profile/component identities, policy/adapters, deterministic structural target inventory, and destination revalidation rules; no live destination observations |
| Journal | Operation identity/token, active writer generation/token, independently validated prior candidate and prior receipt digests, intended candidate `B`, state/phase, and post-authorization live destination observations persisted as preconditions/results for forward recovery and boundary revalidation |
| Pending receipt | Candidate `B`, exact protected identity metadata, requested and observed non-secret facts including structural targets and post-authorization live destination observations, completed phases, and verification observations; existence alone is not completion |
| `current.json` | One atomic `stable(B)` reference to candidate and receipt plus the completed writer/operation identity; the sole receipt-completion authority even when candidate and receipt are reused |

The private durable codecs use exactly these schema-1 field orders and domain strings; no generic record or superset codec is permitted:

```go
package state

type processIdentity struct {
    PID int `json:"pid"`
    Start uint64 `json:"start"`
    PIDNamespace uint64 `json:"pid_namespace"`
}
type rootRecord struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.root"
    Status string `json:"status"` // "initializing" or "initialized"
    Instance string `json:"instance"`
    Token string `json:"token"`
    Process processIdentity `json:"process"`
    Generation uint64 `json:"generation"`
}
type namespaceOwner struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.data-owner" or "zerb.cache-owner"
    Instance string `json:"instance"`
}
type writerOwner struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.writer-owner"
    Status string `json:"status"` // "held" or "released"
    Instance string `json:"instance"`
    Generation uint64 `json:"generation"`
    Token string `json:"token,omitempty"`
    Process processIdentity `json:"process,omitempty"`
    PreviousGeneration uint64 `json:"previous_generation"`
    PreviousToken string `json:"previous_token,omitempty"`
}
type journalRecord struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.journal"
    Instance string `json:"instance"`
    OperationID string `json:"operation_id"`
    OperationToken string `json:"operation_token"`
    WriterGeneration uint64 `json:"writer_generation"`
    WriterToken string `json:"writer_token"`
    WriterPreviousToken string `json:"writer_previous_token"`
    Previous receipt.Digest `json:"previous"` // empty only for exact READY(∅) first bootstrap
    PreviousReceipt receipt.Digest `json:"previous_receipt"` // empty only for exact READY(∅) first bootstrap
    Candidate receipt.Digest `json:"candidate"`
    Phase Phase `json:"phase"`
    Preconditions []receipt.DestinationObservation `json:"preconditions"`
    Results []receipt.DestinationObservation `json:"results"`
    CompletedPhases []Phase `json:"completed_phases"`
}
type pendingReceipt struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.receipt"
    Digest receipt.Digest `json:"digest"`
    Facts receipt.Completed `json:"facts"`
}
type currentRecord struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // "zerb.current"
    Status string `json:"status"` // exactly "stable"
    Instance string `json:"instance"`
    OperationID string `json:"operation_id"`
    OperationToken string `json:"operation_token"`
    WriterGeneration uint64 `json:"writer_generation"`
    WriterToken string `json:"writer_token"`
    Candidate receipt.Digest `json:"candidate"`
    Receipt receipt.Digest `json:"receipt"`
}
```

Every decode is byte-bounded, rejects unknown fields, requires one complete JSON value plus EOF, and enforces exact supported schema/domain/status. Ordinary small scalar codecs use `DisallowUnknownFields` and exact canonical re-encoding. The owner-approved large candidate, pending-receipt and journal codecs may instead use the private closed canonical parser/direct pending encoder described in [the representation plan](state-codec-representation.md), after independent concrete fit review. That exception preserves all existing byte/value rules and the 8,000,000-byte complete-invocation reservation, adds no field cap/dependency/exported API, and rejects every duplicate, alias, unknown/omitted/null/noncanonical representation before returning a value. It does not permit a standard decoder plus a second complete wire buffer, budget reset, or unaccounted historical storage. Collections use the receipt canonical order; candidate/receipt filenames equal their recomputed digest. A pending receipt has no completed status: only an exact `currentRecord` cross-reference completes it. A non-bootstrap journal independently binds both the prior candidate and the exact prior completed-receipt digest; neither is reconstructed from the other.

The ordinary initialization codecs (`rootRecord`, `namespaceOwner`, `writerOwner`) accept at most 4,096 bytes each; the ARCH-020 witness retains its 16,384-byte limit. Validate scalar bounds before encoding, encode the exact ordered structs without a trailing newline, and require byte equality with canonical re-encoding after strict decode. Reject duplicate, unknown, omitted required, null, aliased, unsupported, noncanonical and multiple-value representations. Every instance, initialization/writer token and replacement ID is 16 random bytes encoded as 32 lowercase hexadecimal characters; use `io.ReadFull` with the admitted random source and refuse incomplete/error results or collision with any identity/token minted in the same live session, including already retired replacement IDs, without unbounded retries. Retain the finite session set until release. No timestamp or clock participates in these fresh records.

Fresh root publication has schema 1, domain `zerb.root`, status `initializing`, the fresh instance and distinct initialization token, positive current PID/start/PID-namespace, and generation zero. Initialized root retains the instance but has status `initialized`, empty token, all-zero process tuple and generation zero, with every mandatory field still present. The original live claim/session remains pinned in memory through witness retirement and handoff; cleared persisted fields confer no initialization authority. Namespace owners have schema 1, exact `zerb.data-owner` or `zerb.cache-owner` domain and the same instance.

Fresh writer publication is `released`, generation zero, empty token, zero process, previous generation zero and empty previous token. Post-initialization bootstrap acquisition is `held`, generation one, a fresh distinct writer token and current process, previous generation zero and empty previous token. Its successful release is `released`, generation two, empty token and zero process, previous generation one and previous token equal to that held token. Preserve the declared non-pointer `processIdentity`: Go 1.24 `encoding/json` encodes its zero object despite `omitempty`, so released writer records include `"process":{"pid":0,"start":0,"pid_namespace":0}`; the empty token strings are omitted by their tags. Golden bytes freeze that representation. Other generation/transfer relations remain ARCH-031; FreshInit never advances an unrelated predecessor.

General writer-owner value validation uses the same concrete ordered fields and
4,096-byte strict canonical codec, without widening FreshInit's accepted subset.
The following retained-field shapes follow from ARCH-031's closed acquisition,
held-transfer and release increments, starting only at released generation zero:

| Status/generation | Required predecessor fields |
|---|---|
| `released(0)` | Previous generation zero and empty previous token |
| `held(1)` | Previous generation zero and empty previous token |
| `held(2)` | Previous generation one and a nonempty previous token |
| `held(g)`, `g >= 3` | Previous generation `g-1`; empty or nonempty previous token |
| `released(g)`, `g >= 2` | Previous generation `g-1` and a nonempty previous token |

Held generation zero is invalid. Released generation one would require a held
generation-zero predecessor, so it is invalid; for the same reason held
generation one cannot name a previous token, and held generation two cannot
name an empty one. These are immediate consequences of the existing transitions,
not reconstruction of deleted owner history. Both parities and permitted
maximum-uint64 record values remain scalar-valid; a later successor attempt
still refuses overflow under ARCH-031. The validator never increments a counter.
Held records require a lowercase 32-hex token distinct from instance and positive
PID/start/PID-namespace; released records clear token/process, retaining the
existing mandatory zero process object in their canonical bytes. Every nonempty
previous token is lowercase 32-hex and distinct from instance and, when held,
the current token. Empty/nonempty held predecessor tokens distinguish released
versus held predecessor shapes only; they prove no release, death or transfer.
FreshInit-facing validation/encoding/decoding still admit only its exact
released-zero, held-one and released-two forms and retain their existing errors.

`currentRecord.Instance` equals the initialized owner pair. Its instance, operation ID, operation token and writer token are each exactly 32 lowercase hexadecimal characters; operation ID, operation token and writer token are pairwise distinct. Its operation ID/token and positive uint64 writer generation/token bind the exact journal and active writer at stable commit; initialization, writer and operation tokens remain distinct. These fields are mandatory even when candidate and receipt bytes equal the prior current relation. Candidate and receipt digests do not encode completion identity. Missing, malformed, mismatched or unsupported current identity fails closed; there is no compatibility decoder or migration. After commit, cleanup and any recovery compare-transfer preserve the exact committed current bytes, including its completed writer/operation identity; a later recovery writer does not become the committing writer.

The bounded private current-record codec validates only this exact scalar/wire
shape, before any filesystem or graph integration. Its maximum canonical JSON
length is 464 bytes: 174 fixed syntax/key/domain/status/schema bytes, four
32-byte identities, two 71-byte digest tokens and at most 20 generation digits.
Reject empty/over-bound input before decoding; validate scalars before encoding;
require strict decoding, EOF and byte equality with canonical re-encoding.
Maximum uint64 and even positive writer generations remain valid scalar values;
do not infer a parity rule or additional instance/operation inequality here.
Malformed input returns no partial identity. A decoded record alone grants no
lease, owner/journal correspondence, publication or receipt-completion authority;
those complete graph relations remain required at their existing state-owned
boundaries. The 464-byte derived encoding bound is not a Go allocation/RSS cap.

Completion identity cross-validation uses these closed relations after ordinary instance/schema/graph validation. Here `C` is current and `W` is the writer owner; a journal's predecessor generation is its writer generation minus one.

| Compared record | Required completed-current relation |
|---|---|
| Held owner `W` | Equal generations require equal writer tokens. Otherwise `C` has an older generation, `W.PreviousToken` is nonempty, and their writer tokens differ. If `C` is the immediate predecessor, its token equals `W.PreviousToken`; if older, its token differs from `W.PreviousToken`. |
| Journal | Operation ID/token equal `C`. Equal writer generations require equal writer tokens. Otherwise `C` has an older writer generation, the journal's `writer_previous_token` is nonempty, and their writer tokens differ. An immediate predecessor's token equals `writer_previous_token`; an older token differs. |
| Released owner `W` | `C` has an older generation. If `C.WriterGeneration == W.PreviousGeneration`, its writer token equals `W.PreviousToken`; if older, its token differs from `W.PreviousToken`. |

A fresh held owner acquired from a released owner has an empty predecessor token and cannot satisfy the completed-held relation against an older current, regardless of their generation gap; that exact journal-less graph is pre-journal repair input. An admitted cleanup transfer has a nonempty predecessor token. Current binds the exact writer/operation at commit, and every admitted transfer validates the immediate predecessor plus retained current identity. Deleted historical owner generations are not reconstructed or claimed as independently retained evidence.

The private current-to-writer value predicate first validates both scalar records
and their instance equality, then applies only this table's held/released rows.
It compares generations and predecessor fields without addition or overflow.
Its Boolean correspondence does not validate the initialized owner pair, journal,
candidate/receipt graph, process liveness or retained lock/record inodes, grant
writer/completion authority, or classify a root. Those remain state-owned
integration obligations. Journal correspondence is not implemented by this
bounded writer/current increment.

### ARCH-025 — Prepared, mutating, dirty, and stable truth

Host mutation follows this durable sequence under the complete `A-X+W-X` ownership relation:

```text
current.json = stable(A)
  -> journal phase = "prepared"
  -> journal phase = "mutating-tool-install"
  -> journal phase = "mutating-home-apply"
  -> journal phase = "pending-receipt"
  -> pending receipt(B)
  -> current.json = stable(B)
  -> journal phase = "cleanup"
  -> remove distinct prior candidate, distinct prior receipt, then journal
```

`A` is the prior stable candidate or the explicit empty baseline on first bootstrap; `B` is the exact immutable candidate being applied. `previous_receipt` is the independently validated receipt named by the prior current relation. On exact reapply, `A == B` and the candidate object is reused; the new receipt may differ from or equal the prior receipt according to its complete canonical facts.

Stable current publication atomically replaces candidate, receipt and completed writer/operation identity together. Even when `A == B` and both digests are reused, pre-commit current retains the prior operation identity and committed current names the new operation. After commit, a remaining journal must name the same operation ID/token; its writer may advance only through the exact ARCH-027 compare-transfer. Cleanup never republishes current merely to match its recovery writer.

Candidate `B` and its journal, including the active writer generation/token and independent prior candidate/receipt references, are written and synced before mutation. Existing immutable candidate or receipt objects are reused only after exact canonical-byte/digest/schema equality and are never overwritten. The exact phase byte `"prepared"` is written and synced before the first possible external mutation. The exact phase bytes `"mutating-tool-install"` and `"mutating-home-apply"` are each written and synced immediately before their corresponding possible external mutation. The exact phase byte `"pending-receipt"` is written and synced immediately before pending-receipt publication. After stable current publication, the holder publishes `"cleanup"`; the narrow crash interval containing stable `B` plus the exact pre-cleanup journal is also `CLEANUP(B)`. Under that journal authority it removes and parent-syncs the prior candidate only when its digest differs from `B`, then removes and parent-syncs the prior receipt only when its digest differs from receipt `B`, then removes and parent-syncs the journal last. First bootstrap has empty prior candidate/receipt digests and no prior objects. Repeated exact reapply may therefore reuse both objects without unlinking current authority. Shell and command environments are reconstructed from stable candidate/receipt/tool observations; no shell-metadata phase exists. ARCH-031 ownership is revalidated before each boundary. After journal publication, any abnormal exit before current commit is truthful dirty state naming the exact persisted phase; after current it is `CLEANUP(B)` until exact residue cleanup and verified released-owner publication complete.

Receipt `B` is fully written/synced pending before current commit; existence alone does not complete it. After possible mutation, startup exposes only `{current=A, dirty(A,B,phase,journal)}` or `{current=B, cleanup(B,exact-residue)}`/`{current=B, stable(B)}`. An unrelated pending receipt is orphan evidence. No cleanup step can make a receipt temporarily unreferenced.

#### Journal observations and execution checkpoints

The owner approved this exact `CompletedPhases` table. `tool` denotes
`mutating-tool-install`; `home` denotes `mutating-home-apply`.

| Durable journal phase | Permitted execution checkpoint |
|---|---|
| `prepared` | `[]` |
| `mutating-tool-install` | `[]` or `[tool]` |
| `mutating-home-apply` | `[tool]` or `[tool,home]` |
| `pending-receipt` | `[tool,home]` |
| `cleanup` | `[tool,home]` |

Journal checkpoints remain execution order, distinct from sorted receipt facts.
Preconditions and Results each cover every canonical candidate target throughout
all these stages. Results are the latest actual post-authorization observations,
initially permitted to equal preconditions, refreshed/revalidated at their owned
boundaries. They never assert that home apply ran or succeeded. A no-op phase is
completed only after verifying that no action applies. Missing completion is not
proof of no effect: an interruption can follow an effect and precede checkpoint
persistence. Recovery revalidates and converges that unfinished phase using the
qualified repeat/partial-state native behavior, or preserves dirty evidence and
stops; it never blindly replays a completed phase. No observation or checkpoint
substitutes for writer/lease/source/tool/destination authority.

### ARCH-026 — Receipts and repeatability

A protected receipt records only required non-secret facts: canonical source and component identities, candidate/tree digest, ZERB and exact Git/mise/chezmoi identities, schema/policy versions, observed platform tuple, authored tool requests and observed versions/integrity facts, managed target inventory, completed mutation phases, and post-apply observations. Public diagnostics and exported evidence redact or replace sensitive metadata and digest oracles under ARCH-028; protected identity remains exact.

Receipts report independent dimensions—inputs pinned, artifact integrity established, artifact currently available, host compatible, and convergence verified. They describe pinned inputs and observed convergence on a mutable host, never a reproducible build or atomic rollback.

The approved `DestinationObservation` wire conventions are exact: `Kind` is `absent`, `file` or `directory`. `Exists=false` requires `Kind="absent"`, empty `ContentHash` and zero `Mode`, `Owner`, `Device`, `Inode` and `Links`. An existing file or directory carries the actual post-authorization owner/device/inode/link observations. `Mode` is Unix permission and special bits (`stat.Mode & 07777`), not full type bits or Go's `FileMode` representation; `Kind` separately carries the type. A file has the SHA256 token for its exact observed bytes; a directory has an empty hash, with no invented directory-content digest. Symlinks and unsupported types remain rejected. Preconditions and results each contain exactly one actual observation per candidate target, sorted by target, including directory targets; an empty target inventory has `[]` in all three collections. Ancestor validation remains independently mandatory and is not replaced by these observations.

`ToolObservation.Integrity` denotes the checksum of the authenticated authored archive, equal to the candidate's exact artifact lock. Populate that observation only after actual archive authentication and the complete independent installed-tree verification required by ARCH-013/021: entry set, modes, file hashes, permitted semantic symlink transformations, npm wrapper and executable identity. The archive checksum alone proves none of those checks; no separate installed-tree digest or preimage is introduced. State validates the exact candidate/tool/current relation before stable publication. A pure receipt digest validates representation but cannot authenticate the archive, inspect an installation, prove candidate/current correspondence or grant completion authority. The independent dimension booleans remain facts, not implicit substitutes for one another; encoding a false dimension does not complete a receipt.

Stable publication requires both mutation phases and independently verified
`InputsPinned`, `ArtifactIntegrity`, `HostCompatible` and `ConvergenceVerified`.
`ArtifactAvailable` remains an independent observation: an unavailable upstream
artifact alone does not invalidate intact authenticated installed facts, while
an operation needing that missing artifact still refuses. This does not waive
archive authentication/installed-tree verification, runtime protections or host
qualification. Pure scalar codecs continue to represent all Boolean combinations;
these necessary completion checks are not the full candidate/receipt/current
relation or permission to publish. Current publication remains the sole completion
authority, and later availability changes never rewrite an immutable receipt.

### ARCH-027 — Command admission and forward recovery

ARCH-020 closes the sole replacement-record maintenance exception before fresh held-owner publication. It preserves exact intent/lock order and requires independent complete graph admission; its pure codec/shape recognizer cannot execute repair.

PC-011 parsing precedes root access; after parsing, semantic root/state/candidate/executable/destination validation occurs only under the compatible ARCH-031 lease and after synchronization-inode/root revalidation. PC-016 is the exhaustive state-by-command table. No implementation may infer admission from a pre-lease snapshot, validate then acquire, drop a lease before publication or launch binding, or upgrade a shared lease.

Every `plan` uses `A-X` from before root/state/source/candidate validation through cache staging, trust, permitted immutable publication, synchronization, reopen verification, and cleanup. In `STABLE(A)`, planning may finish only when the recomputed candidate is exact `A`; a differing result is an unsupported update and fails before publication. `TRANSITION(A,B)` and `CLEANUP(B)` deny planning before source access. Mutation and recovery are exclusive. A prepared pre-mutation operation may be reconstructed or abandoned only by the exact Phase 1-frozen transition; no state is relabeled by inference.

For journalled transition/cleanup convergence, `recover resume` and `recover repair` revalidate source objects, candidate/policy digest, trust, destinations and ancestor topology, disk/access preconditions, tool readiness, completed observations, writer ownership, and remaining phase inputs. Recovery persists each phase boundary exactly as a fresh apply would. It never edits journals manually, deletes unrelated data, claims rollback, converts an orphan receipt into truth, or marks an unverified host stable. When an external condition prevents convergence, dirty state remains and `doctor` reports the exact obstruction.

Journal-less dead-owner repair is a dedicated state-owned operation exposed only through ARCH-030's `RepairJournalLess` seam; callers never infer this relation from a missing journal or a failed transition read. For a pre-journal prefix it restores verified prior READY/STABLE authority; for a committed-and-cleaned prefix it completes release while preserving the exact committed current. Neither executes a candidate. Repair to empty READY has a distinct digest-free result under PC-012; repair to prior or committed STABLE returns only its validated current candidate. The atomic current completed-writer/operation identity distinguishes an interruption before journalling from a committed-and-cleaned exact reapply even when candidate and receipt digests are identical.

A dead-writer takeover is monotonic and stage-preserving. Under the same reopened `A-X+W-X` descriptors, recovery first validates the exact predecessor owner publication inode, canonical record, complete process identity, positively established process death, and complete protected graph; missing, unreadable, malformed, permission-denied, or otherwise unavailable liveness evidence fails closed and is never treated as death. An ordinary released READY/STABLE graph with no durable operation is not a recovery prefix. A dead owner-only or candidate-only pre-journal prefix may only publish/reopen the exact prior released owner and return to the prior READY/STABLE authority; it cannot synthesize or execute candidate `B`. A committed-and-cleaned no-journal prefix retains its exact committed current identity and may only complete writer release; it never reconstructs or replays the completed operation. If a journal exists, recovery publishes the same semantic stage at the next writer generation with a fresh writer token and `writer_previous_token` equal to the exact predecessor owner token; this journal-ahead compare-transfer interval is the sole admitted intermediate. It then publishes the matching held owner, retains those exact journal and owner publication identities, and revalidates the complete protected graph before and after every resumed persistence/authority/external boundary. It executes only actions strictly after the recognized durable stage through current publication, cleanup, and release. A temporary publication residue is recovery input only when retained by its exact path/device/inode/bytes and related interrupted process identity; recovery proves that process dead and revalidates the locks, owner, and full graph before unlink, then syncs the parent and absence-reopens. Pathname-only residue rediscovery or cleanup before authority validation is forbidden. A crash at either transfer publication or any resumed action remains exactly recognizable and repeatable; no completed external phase or current publication is replayed or regressed.

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

Linux is the implementation target, not an unconditional support claim. Finite positive validation candidates may become `implementation-qualified` and negative rows `exclusion-proven` only after the current corrective evidence and repeat reviews pass; neither authorizes private transport or creates support. A distribution/kernel/CPU/filesystem tuple, public native Git identity, shell, mise declaration/backend/package class, or chezmoi entry/template surface is supported only after VAL-001 accepts its complete Phase 6 installed-binary vertical and applicable focused rows. Selected gitlinks, private transport/authentication, provider, and semantic host-fact surfaces remain excluded. Non-orthogonal combinations are tested as combinations. A failed rerun removes or narrows the candidate; it never creates a fallback.

### ARCH-030 — Replacement package and internal API seam

The replacement package set is exactly `internal/paths`, `internal/safefs`, `internal/source`, `internal/profile`, `internal/toolchain`, `internal/execenv`, `internal/mise`, `internal/home`, `internal/plan`, `internal/state`, `internal/recovery`, `internal/receipt`, `internal/host`, `internal/app`, and test-only `internal/testenv`. None may import, alias, wrap, re-export, register, or accept a type from the obsolete `internal/config`, `internal/service`, `internal/git`, `internal/drift`, `internal/chezmoi`, `internal/transaction`, `internal/shell`, `internal/platform`, `internal/binary`, or `internal/testutil` package trees. Retained algorithms are rewritten behind replacement-owned types; there is no bridge package or service locator.

The allowed production import DAG is exact:

| Package | May import replacement packages |
|---|---|
| `paths` | none |
| `profile` | none |
| `receipt` | none |
| `safefs` | `paths` |
| `execenv` | `safefs` |
| `source` | `paths`, `safefs`, `execenv`, `toolchain`, `profile`, `receipt` |
| `toolchain` | `paths`, `safefs`, `execenv`, `receipt` |
| `mise` | `paths`, `safefs`, `execenv`, `toolchain`, `source`, `receipt` |
| `home` | `paths`, `safefs`, `execenv`, `toolchain`, `source`, `receipt` |
| `state` | `paths`, `safefs`, `receipt` |
| `recovery` | `paths`, `source`, `state`, `receipt`, `toolchain`, `mise`, `home` |
| `plan` | `paths`, `source`, `profile`, `toolchain`, `mise`, `home`, `state`, `receipt` |
| `app` | `paths`, `execenv`, `source`, `profile`, `toolchain`, `mise`, `home`, `plan`, `state`, `receipt`, `recovery` |
| `host` | `app` and any replacement package needed only to assemble production `app.Options` |
| `testenv` | `paths`, `safefs`; `_test.go` consumers only |
| `cmd/zerb` | `app`, `host`, `execenv` |

An omitted edge is forbidden. In particular, adapters never import `state`, `recovery`, or `app`; `state` never imports `plan` or an adapter; `receipt` owns durable canonical data types and imports no orchestration package; and no lower package imports `host`, `app`, `cmd/zerb`, or `testenv`.

The cross-package APIs below are frozen. Fields not shown do not exist; structs with unexported representation are constructed only by the named constructor and expose copies through accessors.

```go
package paths

type Input struct { /* unexported immutable copy */ }
func NewInput(home string, environ []string) (Input, error) // validates bytes and copies; no I/O
func (i Input) Home() string
func (i Input) Environ() []string
func (i Input) LookupEnv(name string) (string, bool)

type Roots struct { /* unexported canonical resolved roots */ }
func Resolve(Input) (Roots, error)
func (r Roots) Home() string
func (r Roots) Data() string  // $XDG_DATA_HOME/zerb
func (r Roots) Cache() string // $XDG_CACHE_HOME/zerb
func (r Roots) State() string // $XDG_STATE_HOME/zerb
```

```go
package safefs

type Kind uint8
const (
    KindDirectory Kind = iota + 1
    KindRegular
    KindExecutable
)
type Identity struct {
    Device uint64
    Inode uint64
    Owner uint32
    Mode uint32
    Links uint64
    Size int64
    MountID uint64
}
type Dir struct { /* unexported close-on-exec descriptor and identity */ }
func OpenRoot(path string, owner uint32) (*Dir, error)
func (d *Dir) OpenDir(name string, create bool) (*Dir, error)
func (d *Dir) CreateDir(name string) (*Dir, error)
func (d *Dir) Identity() Identity
func (d *Dir) Names(limit int) ([]string, error)
func (d *Dir) ReadFile(name string, limit int64) ([]byte, Identity, error)
func (d *Dir) PublishNoReplace(name string, data []byte, mode uint32) (Identity, error)
type FileFacts struct {
    Identity Identity
    ModifiedSeconds int64
    ModifiedNanoseconds uint32
    ChangedSeconds int64
    ChangedNanoseconds uint32
    SHA256 string
}
type File struct { /* shared owned pinned-file lifetime; no state cleanup authority */ }
func (d *Dir) PinFile(name string, limit int64) (*File, []byte, error)
func (f *File) Facts() FileFacts
func (f *File) Revalidate() error
func (f *File) Remove() error
func (f *File) Close() error
type ReplacementFacts struct {
    Parent Identity
    Target string
    Temporary string
    Previous FileFacts
    Staged FileFacts
}
type Replacement struct { /* shared owned descriptor lifetime; no cleanup authority */ }
func (d *Dir) StageReplacement(target, temporary string, data []byte, mode uint32, previous Identity) (*Replacement, error)
func (r *Replacement) Facts() ReplacementFacts
func (r *Replacement) Link() error
func (r *Replacement) Publish() (Identity, error)
func (r *Replacement) Close() error
func (d *Dir) RemoveTree(name string, expected Identity) error
func (d *Dir) Sync() error
func (d *Dir) Close() error
type Role uint8
const (
    RoleState Role = iota + 1
    RoleData
    RoleCache
    RoleSource
    RoleTree
    RoleToolchain
    RoleManager
    RoleStaging
    RoleDestination
)
type Roles struct { /* closed descriptor-owned role set */ }
func OpenRoles(paths.Roots, uint32) (*Roles, error)
func (r *Roles) Add(Role, *Dir) error
func (r *Roles) Validate() error
func (r *Roles) Close() error

type TreeStage struct { /* unexported same-parent staging descriptor and identity */ }
func (d *Dir) StageTree(name string, mode uint32) (*TreeStage, error)
func (s *TreeStage) Root() *Dir
func (s *TreeStage) PublishNoReplace(name string) (Identity, error)
func (s *TreeStage) Cleanup() error

type LockMode uint8
const (
    LockShared LockMode = iota + 1
    LockExclusive
)
type Lock struct { /* unexported stable OFD lock descriptor and identity */ }
func (d *Dir) OpenLock(name string, create bool) (*Lock, error)
func (l *Lock) Try(LockMode) error
func (l *Lock) Identity() Identity
func (l *Lock) Unlock() error
func (l *Lock) Close() error
```

The owner-approved staged replacement seam replaces the unimplemented monolithic call. ARCH-020 owns its exact lifetime, operation-record ordering, narrowly scoped maintenance admission and uncertainty contract; facts alone confer no state or cleanup authority.

Each `Lock` is one shared lifetime across ordinary Go value copies, serialized by its private mutex. `Try` uses nonblocking whole-file `F_OFD_SETLK` with PID zero; it never substitutes process-associated or other locks. A repeated held mode only revalidates that same capability, without another acquisition or reference count. A different held mode is refused without conversion; an invalid mode does not drop an otherwise valid held capability. `Unlock` validates even when already unheld and is idempotent on an open valid handle. A nil/closed lock refuses `Try`/`Unlock`; `Identity` is cached, not current authority, and becomes zero after closure. `Close` is idempotent and always attempts release/cleanup even if validation fails.

Public construction, `Try` and `Unlock` revalidate the full absolute ancestor chain and local pin/proc/writable descriptor around their protected boundaries. An observed validation or unlock failure closes the owned OFD and marks the capability closed, so a failed operation cannot strand unreported exclusion. Only a validated `F_OFD_SETLK` result of `EAGAIN` reports contention through a fixed redacted error matching that errno; permission/security failures and unsupported primitives are operational refusal. Topology failure takes precedence over contention. Resource cleanup clears shared state before closing descriptors, preventing value-copy double close against reused descriptor numbers. Private local transition tests exercise the same kernel core but cannot establish public absolute-root admission, complete role isolation, state leases or frozen-host qualification.

```go
package execenv

type Class uint8
const (
    ClassGit Class = iota + 1
    ClassAcquisition
    ClassMise
    ClassChezmoi
    ClassExec
    ClassShell
)
type Executable struct {
    ID string
    Version string
    Path string
    Device uint64
    Inode uint64
    Owner uint32
    Mode uint32
    Size int64
    SHA256 [32]byte
}
type Spec struct {
    Executable Executable
    Args []string
    Dir string
    Env []string
    Stdin io.Reader
    Stdout io.Writer
    Stderr io.Writer
    Class Class
}
type Exit struct {
    Code int
    Signal syscall.Signal
}
type Runner interface {
    Run(context.Context, Spec) (Exit, error)
}
func NewRunner() Runner
func DispatchSupervisor() (handled bool, code int)
```

`NewRunner` returns the one inert production runner value; construction performs no filesystem, environment, process, network, root-resolution, or durable-state work. `host.NewOptions` constructs one runner and supplies that exact instance to every service and as `app.Services.Runner`. `execenv.Spec` intentionally exposes no shell string, `ExtraFiles`, credential channel, transport mechanism, arbitrary descriptor, cgroup bypass, or alternate cancellation primitive. The production runner always performs the frozen executable-identity recheck, inherited-descriptor sanitation, same-binary supervisor handshake, class policy, and required cgroup setup before releasing the managed executable. `DispatchSupervisor` examines only the reserved close-on-exec control descriptor, validates its Unix-socket type, peer PID/UID, parent executable identity, one-time random protocol nonce, complete bounded specification, and parent-death relation, and consumes it only when every check passes; otherwise it returns `handled=false` without reading public argv or accepting any public dispatch token.

```go
package receipt

type Digest string
func ParseDigest(string) (Digest, error)

type Source struct {
    URI string `json:"uri"`
    SourceID string `json:"source_id"`
    Commit string `json:"commit"`
    RootTree string `json:"root_tree"`
}
type File struct {
    Path string `json:"path"`
    Mode string `json:"mode"`
    Hash string `json:"hash"`
}
type Component struct {
    ID string `json:"id"`
    Path string `json:"path"`
    TreeDigest string `json:"tree_digest"`
    Files []File `json:"files"`
}
type LockValue struct {
    Name string `json:"name"`
    Value string `json:"value"`
}
type Tool struct {
    ID string `json:"id"`
    Version string `json:"version"`
    Locks []LockValue `json:"locks"`
}
type Executable struct {
    ID string `json:"id"`
    Version string `json:"version"`
    Path string `json:"path"`
    SHA256 string `json:"sha256"`
}
type Parser struct {
    Schema int `json:"schema"`
    Module string `json:"module"`
    Version string `json:"version"`
    ModuleSum string `json:"module_sum"`
    GoModSum string `json:"go_mod_sum"`
    OriginCommit string `json:"origin_commit"` // upstream base; ModuleSum binds the local patched source
}
type Policy struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // exactly "zerb.policy"
    ObjectModes []string `json:"object_modes"`
    Declarations []string `json:"declarations"`
    TemplateActions []string `json:"template_actions"`
    ProcessClasses []string `json:"process_classes"`
    PolicyIDs []string `json:"policy_ids"`
    TargetMapperExecutableID string `json:"target_mapper_executable_id"`
    ProviderFree bool `json:"provider_free"`
    HostFactIndependent bool `json:"host_fact_independent"`
    DestinationRecheck bool `json:"destination_recheck"`
    ExactReapplyOnly bool `json:"exact_reapply_only"`
}
type Candidate struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // exactly "zerb.candidate"
    Source Source `json:"source"`
    ProfileID string `json:"profile_id"`
    Components []Component `json:"components"`
    Tools []Tool `json:"tools"`
    HomeSources []string `json:"home_sources"`
    Targets []string `json:"targets"`
    Executables []Executable `json:"executables"`
    Parser Parser `json:"parser"`
    Policy Policy `json:"policy"`
}
func CandidateDigest(Candidate) (Digest, []byte, error)

type DestinationObservation struct {
    Target string `json:"target"`
    Exists bool `json:"exists"`
    Kind string `json:"kind"`
    Mode uint32 `json:"mode"`
    Owner uint32 `json:"owner"`
    Device uint64 `json:"device"`
    Inode uint64 `json:"inode"`
    Links uint64 `json:"links"`
    ContentHash string `json:"content_hash"`
}
type ToolObservation struct {
    ID string `json:"id"`
    Requested string `json:"requested"`
    Installed string `json:"installed"`
    Integrity string `json:"integrity"`
}
type PlatformObservation struct {
    Distribution string `json:"distribution"`
    Kernel string `json:"kernel"`
    Architecture string `json:"architecture"`
    Filesystem string `json:"filesystem"`
}
type Dimensions struct {
    InputsPinned bool `json:"inputs_pinned"`
    ArtifactIntegrity bool `json:"artifact_integrity"`
    ArtifactAvailable bool `json:"artifact_available"`
    HostCompatible bool `json:"host_compatible"`
    ConvergenceVerified bool `json:"convergence_verified"`
}
type Completed struct {
    Schema int `json:"schema"`
    Domain string `json:"domain"` // exactly "zerb.completed"
    Candidate Digest `json:"candidate"`
    Previous Digest `json:"previous,omitempty"`
    Source Source `json:"source"`
    Components []Component `json:"components"`
    Zerb Executable `json:"zerb"`
    Git Executable `json:"git"`
    Mise Executable `json:"mise"`
    Chezmoi Executable `json:"chezmoi"`
    Parser Parser `json:"parser"`
    Policy Policy `json:"policy"`
    Platform PlatformObservation `json:"platform"`
    Tools []ToolObservation `json:"tools"`
    Targets []string `json:"targets"`
    Preconditions []DestinationObservation `json:"preconditions"`
    Results []DestinationObservation `json:"results"`
    Phases []string `json:"phases"`
    Dimensions Dimensions `json:"dimensions"`
}
func ReceiptDigest(Completed) (Digest, []byte, error)
```

The schema-1 candidate policy arrays are exact after lexical sorting: object modes `directory`, `executable`, `file`; declarations `home`, `mise.lock`, `mise.toml`, `profile.toml`; template actions `if-false`, `if-true`, `literal-text`; process classes `bash-shell`, `chezmoi-static`, `git-public`, `mise-core-node`; and policy IDs `zerb.chezmoi-static.v2`, `zerb.destination-recheck.v1`, `zerb.execenv.v4`, `zerb.git-public.v3`, `zerb.lifecycle.v6`, `zerb.mise-core-node.v3`. The final ID owns the exact ARCH-013/019/021 immutable archive, eleven platform-class, raw-install, semantic-symlink, frozen npm-wrapper, and manager-seccomp rules. Any missing, additional, reordered-after-canonicalization, or version-changed value changes or fails the candidate.

PC-022 owns the native `Source.Commit`/`RootTree` syntax and source-service relation; PC-050 owns the distinct materialized `Component.TreeDigest` preimage and empty-array encoding. Canonical identity uses these collection keys, compared by normalized UTF-8 bytes; no arrival order or reserved-base position is authority:

| Collection | Sort / identity key | Validation owner |
|---|---|---|
| Components | `ID` | Exactly one `base` and one selected ID, distinct component paths; PC-040/043, ARCH-003 |
| Component files | `Path` | Unique canonical paths, admitted modes and content-hash forms; PC-040/042/050 |
| Tools and tool observations | `ID` | Unique identities and existing closed declaration bounds; PC-042/043, ARCH-026 |
| Tool locks | `Name` | Exact authored lock inventory, no duplicate names; PC-042/043 |
| Executables | `ID` | Exact retained managed identities, no duplicate IDs; ARCH-006/010/013/014/021 |
| Home sources and targets | Complete string | Unique canonical logical paths and aggregate PC-042 bounds |
| Policy arrays | Complete string | Exact closed arrays above, without duplicates |
| Receipt preconditions and results | `Target` | Unique target identities; live facts belong to ARCH-021/025/026, not candidate identity |
| Receipt phase facts | Complete string | Distinct completed phase names, lexical receipt encoding under PC-050 |

Required identity collisions reject even if all other fields match. A pure canonicalizer validates owned value syntax and encodes facts; it cannot authenticate acquisition, observe a destination, establish cross-record current authority or complete a receipt. Such authority remains with the source/toolchain/state operation owners. Invalid UTF-8 rejects before JSON can replace malformed input. Physical executable paths are canonical absolute identities, distinct from PC-040's component-relative logical paths; do not silently apply a new field cap or reinterpret the authored Node version bound as an adapter-version bound.

Receipt phase facts are sorted for identity; the journal's `CompletedPhases` remains the exact execution-prefix sequence owned by ARCH-024/025. Sorting receipt facts must never reorder mutation actions or journal progress.

```go
package profile

type Kind string
const (
    Embedded Kind = "embedded"
    Gitlink Kind = "gitlink"
)
type Declaration struct {
    ID string
    Kind Kind
    Path string
}
type Manifest struct {
    Schema int
    Base string
    Profiles []Declaration
}
type Component struct {
    Schema int
    Label string
    HasMise bool
    HasLock bool
    HasHome bool
}
func ParseManifest([]byte) (Manifest, error)
func ParseComponent([]byte) (Component, error)
func Select(Manifest, string) (Declaration, error)
func ValidateTemplate(name string, source []byte) error
```

```go
package source

type Inspection struct { /* immutable source ID, commit, tree, parsed manifest and tree facts */ }
func (i *Inspection) Source() receipt.Source
func (i *Inspection) Manifest() profile.Manifest
func (i *Inspection) Select(profile.Declaration) (Selection, error)
func (i *Inspection) Close() error
type Selection struct { /* opaque identity bound to one Inspection */ }
type Component struct { /* descriptor-owned bounded staged component */ }
func (c *Component) Identity() receipt.Component
func (c *Component) ReadFile(string, int64) ([]byte, error)
func (c *Component) Names(int) ([]string, error)
type Effective struct { /* descriptor-owned base+selected composition */ }
func (e *Effective) Base() *Component
func (e *Effective) Selected() *Component
func (e *Effective) Close() error
type Staged struct { /* untrusted per-instance cache capability */ }
func (s *Staged) Source() receipt.Source
func (s *Staged) Effective() *Effective
func (s *Staged) Promote(context.Context) error
func (s *Staged) Cleanup() error
type Service struct { /* side-effect-free dependencies only */ }
func New(paths.Roots, execenv.Runner, *toolchain.FixedCA) (*Service, error)
func (s *Service) Inspect(context.Context, *toolchain.Handle, string) (*Inspection, error)
func (s *Service) Stage(context.Context, *Inspection, Selection) (*Staged, error)
func (s *Service) Open(context.Context, receipt.Candidate) (*Effective, error)
```

`Inspection`, `Selection`, `Component`, `Effective`, and `Staged` are non-forgeable capabilities. `Inspect` parses PC-040 once through `profile.ParseManifest` and stores that value; `Select` accepts only an exact declaration from that stored manifest, so source never reimplements or duplicates schema parsing. `Stage` verifies that selection and inspection identities match and never refetches. Staged bytes remain only in the validated per-instance cache before trust. `Promote` publishes the exact already-authenticated source/tree identities no-replace after trust; `Cleanup` closes every descriptor and removes only the validated stage. Every `Inspection` and `Effective` owner calls `Close`; no raw path returned by this API is authority.

```go
package toolchain

type FixedCA struct { /* unexported immutable canonical bundle bytes and reviewed SHA256 */ }
func NewFixedCA(canonicalPEM []byte, reviewedSHA256 [32]byte) (*FixedCA, error)
func (c *FixedCA) Bundle() []byte
func (c *FixedCA) SHA256() [32]byte

type Artifact struct {
    ID string
    Version string
    URL string
    SHA256 [32]byte
    Size int64
    Provenance []byte
}
type RuntimeManifest struct {
    ZerbVersion string
    Distribution string
    Kernel string
    Architecture string
    Filesystem string
}
type Handle struct { /* descriptor-owned promoted or immutable staged executable capability */ }
func (h *Handle) Executable() execenv.Executable
func (h *Handle) Promoted() bool
func (h *Handle) Promote(context.Context) error
func (h *Handle) Cleanup() error
type Service struct { /* side-effect-free dependencies only */ }
func New(paths.Roots, execenv.Runner, *FixedCA, []Artifact, RuntimeManifest) (*Service, error)
func (s *Service) Resolve(context.Context, string) (*Handle, error)
func (s *Service) Stage(context.Context, string) (*Handle, error)
func (s *Service) ObserveRuntime(context.Context) (receipt.Executable, receipt.PlatformObservation, error)
```

`FixedCA` is the one fixed-CA acquisition capability. `toolchain` owns it because the frozen DAG already places `toolchain` below both `source` and `mise`; this adds no package or import edge. `NewFixedCA` accepts only the frozen canonical bundle bytes and their independently reviewed SHA256 `3ba2000fe3d55cb6c41323fabbb105743e2c9422151eea213311538b07b23f5a`, copies and authenticates the bytes before completely parsing them, and returns an opaque immutable capability. `Bundle` returns a copy. Each service constructor rejects a nil capability before retaining dependencies. An empty or malformed bundle, mismatched digest, changed constructor input, changed accessor result, or substituted materialized CA file fails before DNS, network, or source/archive parsing. `Artifact.Provenance` remains artifact provenance only and never carries CA bytes or identity.

The approved 472,033-byte artifact is the pinned NSS 3.126/buildcatrust 0.5.1 OpenSSL bundle, including authenticated labels, ordinary certificate or explicit omission records, purpose descriptions, and paired `TRUSTED CERTIFICATE` records. Every PEM block and certificate DER is completely parsed; every trusted record additionally has exactly the builder's AUX sequence: mandatory trust-purpose sequence and optional nonempty implicit context-tag-0 rejected-purpose sequence. Reject unknown fields, extra DER, unsupported/duplicate/conflicting purposes, malformed boundaries or descriptions, and mismatched ordinary/trusted certificates. The exact artifact contains only server-auth and email-protection purposes. Its 121 ordinary certificates have matching affirmative server trust and form the entire Go TLS pool; its 51 additional AUX-only certificates do not enter that pool. No parser treats the presence of a trusted record alone as server trust. The full digest-bound artifact remains unchanged for child acquisition. This is the existing fixed candidate's format, not an alternate input, new CA authority or configurable trust policy.

Under ARCH-009, ARCH-011, ARCH-012, and this ARCH-030 seam, `source.Service`, `toolchain.Service`, and `mise.Service` receive the same non-nil `*FixedCA` explicitly at construction. In-process acquisition builds a new non-ambient `x509.CertPool` from only `FixedCA.Bundle()` and assigns it as `tls.Config.RootCAs`; a child acquisition receives only an owner-validated synthetic CA file containing those exact bytes, whose opened identity and SHA256 are rechecked immediately before release. No path calls `x509.SystemCertPool`, accepts a nil root pool, consults an ambient CA variable/store, or falls back after rejection. The capability and any materialized path are per-assembly/per-operation authority, never mutable package state or a global. VAL-017 poisons every ambient CA path, while VAL-021 and VAL-022 reprove the exact fixed-CA boundary for mise and chezmoi acquisition respectively.

`Resolve` returns a descriptor-owned handle for only an already-promoted, reauthenticated executable. `Stage` selects one frozen artifact by ID, acquires and authenticates it lazily inside the admitted operation, and returns an immutable cache-only handle with `Promoted()==false`; it never promotes. `Promote` is valid only for the exact staged handle, and `Cleanup` always closes descriptors while deleting bytes only for an unpromoted owned stage. `plan.Prepared` owns every exact `toolchain.Handle` used for Git/mise/chezmoi inspection and promotes it only after exact candidate trust, or cleans it before releasing the operation lease. Stable exec/shell operations use `Resolve` and close the promoted handle through `Cleanup`. `ObserveRuntime` performs no work at construction: under the caller's operation lease it opens `/proc/self/exe` with no-follow descriptor semantics, verifies the live regular-file device/inode/owner/group/type/mode/size identity, hashes that exact open descriptor, and returns the live ZERB executable identity plus the observed host tuple. The independently embeddable manifest supplies only the frozen ZERB version and supported distribution/kernel/architecture/filesystem values; it never embeds or compares a hash of the executable that contains the manifest. Fresh apply and recovery bind the returned live executable and platform facts identically into the exact candidate-bound receipt/current relation and installed-binary evidence; they never mutate the already-trusted candidate. Service construction performs no I/O. Artifact/runtime manifests and provenance are frozen production data, not caller or candidate input.

```go
package mise

type LockedTool struct {
    ID string
    Version string
    LockName string
    LockValue string
}
type Observation struct {
    Requested string
    Installed string
    InstallRoot string
}
type Environment struct { /* unexported complete stable tool environment */ }
func (e Environment) Values() []string
type Service struct { /* side-effect-free dependencies only */ }
func New(paths.Roots, execenv.Runner, *toolchain.FixedCA) (*Service, error)
func (s *Service) Parse(*source.Effective) (LockedTool, error)
func (s *Service) Inspect(context.Context, *toolchain.Handle, LockedTool) (Observation, error)
func (s *Service) Install(context.Context, *toolchain.Handle, LockedTool) (Observation, error)
func (s *Service) Environment(context.Context, *toolchain.Handle, LockedTool, receipt.ToolObservation) (Environment, error)
func (s *Service) Exec(context.Context, *toolchain.Handle, LockedTool, receipt.ToolObservation, []string, io.Reader, io.Writer, io.Writer) (execenv.Exit, error)
```

`Environment` is reconstructed from the exact stable candidate/receipt/tool observation under the caller's lease and contains no ZERB manager credential/state path. `Exec` releases the requested payload through `execenv.ClassExec`; `app.shell` combines the same environment with the exact promoted Bash executable and releases it through `execenv.ClassShell`. No separate shell metadata is generated or persisted.

```go
package home

type Mapping struct {
    Target string
    Source string
    Kind string
    Mode uint32
}
type Service struct { /* side-effect-free dependencies only */ }
func New(paths.Roots, execenv.Runner) *Service
func (s *Service) Map(context.Context, *toolchain.Handle, *source.Effective) ([]Mapping, error)
func (s *Service) Apply(context.Context, *toolchain.Handle, receipt.Candidate, *source.Effective, []receipt.DestinationObservation) ([]receipt.DestinationObservation, error)
func (s *Service) Verify(context.Context, *toolchain.Handle, receipt.Candidate) ([]receipt.DestinationObservation, error)
```

```go
package state

type Classification uint8
const (
    Clean Classification = iota + 1
    Foreign
    Initializing
    BadUnsupported
    BadCorrupt
    Ready
    Stable
    Transition
    Cleanup
)
type LeaseMode uint8
const (
    DiagnosticSnapshot LeaseMode = iota + 1
    InitExclusive
    AdmissionShared
    AdmissionExclusive
    AdmissionWriterExclusive
)
type Intent uint8
const (
    IntentProfileList Intent = iota + 1
    IntentPlan
    IntentDoctor
    IntentRecoverResume
    IntentRecoverRepair
    IntentBootstrap
    IntentApply
    IntentExec
    IntentShell
)
type Phase string
const (
    PhasePrepared Phase = "prepared"
    PhaseToolInstall Phase = "mutating-tool-install"
    PhaseHomeApply Phase = "mutating-home-apply"
    PhasePendingReceipt Phase = "pending-receipt"
    PhaseCleanup Phase = "cleanup"
)
type Snapshot struct {
    Classification Classification
    Current receipt.Digest
    Candidate receipt.Digest
    Receipt receipt.Digest
    Phase Phase
}
type TransitionView struct {
    OperationID string
    OperationToken string
    WriterGeneration uint64
    WriterToken string
    Previous receipt.Digest
    PreviousReceipt receipt.Digest
    Candidate receipt.Digest
    Phase Phase
    Preconditions []receipt.DestinationObservation
    Results []receipt.DestinationObservation
    CompletedPhases []Phase
}
type Lease struct { /* unexported stable lock, descriptor-role and owner authority */ }
func (l *Lease) Mode() LeaseMode
func (l *Lease) Revalidate() error
func (l *Lease) Release() error
type Acquisition struct {
    Lease *Lease
    Snapshot Snapshot
    Mode LeaseMode
    Admitted bool
}
type Current struct {
    Candidate receipt.Digest
    Receipt receipt.Digest
    Facts receipt.Completed
}
type Store struct { /* side-effect-free roots/random/clock dependencies only */ }
func New(paths.Roots, io.Reader, func() time.Time) *Store
func (s *Store) AcquireFor(context.Context, Intent) (Acquisition, error)
func (s *Store) Initialize(context.Context, Acquisition) (Acquisition, error)
func (s *Store) RepairInitialization(context.Context, Acquisition) (Snapshot, error)
func (s *Store) RepairJournalLess(context.Context, *Lease, Snapshot) (Snapshot, bool, error)
func (s *Store) ReadCandidate(context.Context, *Lease, receipt.Digest) (receipt.Candidate, error)
func (s *Store) ReadCurrent(context.Context, *Lease) (Current, error)
func (s *Store) ReadTransition(context.Context, *Lease) (TransitionView, error)
func (s *Store) PublishCandidate(context.Context, *Lease, receipt.Candidate) (receipt.Digest, error)
func (s *Store) Begin(context.Context, *Lease, receipt.Candidate, []receipt.DestinationObservation) (TransitionView, error)
func (s *Store) RecordPhase(context.Context, *Lease, TransitionView, Phase, []receipt.DestinationObservation) (TransitionView, error)
func (s *Store) Commit(context.Context, *Lease, TransitionView, receipt.Completed) error
```

`AcquireFor` is the sole routing/classification entry. From only minimum safe topology and the typed intent, the state package chooses one intent-owned admission mode before semantic classification: A-S for doctor/exec/shell and A-X for profile/plan/bootstrap/apply/recovery. It acquires that relation non-blockingly, reopens/revalidates it, classifies under it, and applies the exact PC-016 row. Only an admitted mutation or current-state recovery then acquires writer.lock, compare-transfers writer ownership, and returns A-X+W-X while retaining the same A-X descriptor; an admitted initialization instead follows the I-X one-way handoff below. Denied current-state rows return `Admitted=false` under the already-held intent admission lease; they never drop, downgrade, upgrade, or reacquire to obtain D-S. D-S is reserved for read-only CLEAN/FOREIGN/INIT/BAD diagnostic topology when no safe current-state admission relation exists. `Initialize` accepts only an admitted CLEAN acquisition and remembers its intent-owned target mode. While I-X remains held, it publishes and reopens the initializing prefix, opens and validates the stable admission/writer inodes, and acquires the exact target A-X or A-X+W-X before publishing initialized root. It returns that already-held target acquisition only after the complete state/data/cache READY graph and any held-owner transfer reopen exactly.

`Initialize` returns the still-owned `Acquisition` on every error, including an error after target acquisition, final-root publication, or I-X consumption. Its one `Lease` retains every descriptor still held, including I-X and any target admission/writer descriptors during handoff; the input acquisition must not be released separately. `app` replaces its acquisition with the returned value before inspecting the error, then releases that returned lease exactly once on either path. Failure never returns an empty handle while exclusion remains held, silently transfers cleanup ownership, or permits an operation to continue from an unverified READY graph. Release preserves the primary error and reports cleanup errors without hiding it.

Every candidate/current/receipt/transition read takes and revalidates the compatible lease. `ReadCurrent` recomputes and cross-validates current, candidate filename/bytes, receipt filename/bytes, and the complete `receipt.Completed` facts before returning copies. There is no unlocked `Classify` API. `TransitionView` is immutable and bound to exact reopened journal/owner identities; each state method rejects a stale view rather than consulting hidden “last operation” state.

`RepairJournalLess` owns recognition and repair of ARCH-027's exact dead owner-only/candidate-only pre-journal or committed-and-cleaned prefix under the existing A-X+W-X lease. It validates the bound snapshot, retained identities, full graph and predecessor authority before any repair. On success it returns the verified READY/STABLE snapshot with `handled=true`; it never executes a candidate or replaces/releases the lease. It returns `handled=false` with no writes only after verifying a supported journalled transition/cleanup relation for normal recovery. Malformed, unrelated, stale, or otherwise unprovable input returns an error rather than a journal-absence hint. Callers never inspect journal absence or attempt `ReadTransition` to discover this case.

```go
package plan

type Request struct {
    Source string
    ProfileID string
}
type Prepared struct { /* exact inspection/stage/candidate capability */ }
func (p *Prepared) Candidate() receipt.Candidate
func (p *Prepared) Digest() receipt.Digest
func (p *Prepared) StagedSource() *source.Staged
func (p *Prepared) Publish(context.Context, *state.Lease) error
func (p *Prepared) Cleanup() error
func New(*source.Service, *toolchain.Service, *mise.Service, *home.Service, *state.Store) *Service
func (s *Service) Prepare(context.Context, *state.Lease, Request) (*Prepared, error)
```

`Prepare` never owns or releases the lease. It owns the exact source and toolchain staging capabilities used to construct the candidate. `app` owns the one operation lease, can display/prompt while it remains held, and either calls `Prepared.Publish` after exact trust—promoting those exact capabilities and the candidate no-replace—or `Prepared.Cleanup` before release. Bootstrap uses the same `AdmissionWriterExclusive` lease for prepare, both decisions, mutation, current publication, cleanup, and release.

```go
package recovery

type Kind uint8
const (
    InitializationRepaired Kind = iota + 1
    StateRepaired
    CandidateRecovered
)
type Result struct {
    Kind Kind
    Candidate receipt.Digest
}
type Service struct { /* exact source/state/toolchain/mise/home dependencies */ }
func New(*source.Service, *state.Store, *toolchain.Service, *mise.Service, *home.Service) *Service
func (s *Service) Resume(context.Context, *state.Lease, state.TransitionView) (Result, error)
func (s *Service) Repair(context.Context, *state.Lease, state.Snapshot) (Result, error)
```

For an `INIT` acquisition, `app.recoverRepair` calls `Store.RepairInitialization` under the one I-X lease and returns `InitializationRepaired` with an empty candidate, producing PC-012's exact initialization line. For other admitted repair, `recovery.Service.Repair` first calls `Store.RepairJournalLess` under the existing lease. A handled READY result becomes `StateRepaired` with an empty candidate and PC-012's exact `Repaired: state` line; a handled STABLE result becomes `CandidateRecovered` with its validated current candidate. Only `handled=false` proceeds to `ReadTransition` and journalled transition/cleanup convergence. `Resume` never performs journal-less repair. Before assembling any missing `receipt.Completed`, recovery calls the same `toolchain.Service.ObserveRuntime` used by fresh apply, descriptor-verifies and hashes the live ZERB executable, verifies the independently frozen supported platform tuple, and binds those exact live facts into the receipt; it then commits and returns `CandidateRecovered`. `Result` permits an empty candidate only for `InitializationRepaired` or `StateRepaired`; `CandidateRecovered` requires the exact validated current digest. Recovery never invents a candidate for an empty state, never imports `safefs`, and has no hidden runtime/global observation path.
`toolchain.Service.ObserveRuntime` opens the running image through `/proc/self/exe` with `O_RDONLY|O_CLOEXEC`, verifies its complete descriptor topology and hashes that exact descriptor, and separately opens the observed canonical pathname with no-follow semantics before and after hashing to require the same device and inode. Launch-path rename or replacement during the observation must either preserve binding to that running-image descriptor or fail closed; hashing a launch pathname alone is forbidden.

`app` is the sole production operation owner. It consumes granular service interfaces, acquires each PC-016 lease once, keeps it across interactive decisions and every named boundary, and releases only after cleanup or truthful residue:

```go
package app

type Sources interface {
    Inspect(context.Context, *toolchain.Handle, string) (*source.Inspection, error)
    Stage(context.Context, *source.Inspection, source.Selection) (*source.Staged, error)
    Open(context.Context, receipt.Candidate) (*source.Effective, error)
}
type Toolchains interface {
    Resolve(context.Context, string) (*toolchain.Handle, error)
    Stage(context.Context, string) (*toolchain.Handle, error)
    ObserveRuntime(context.Context) (receipt.Executable, receipt.PlatformObservation, error)
}
type Mise interface {
    Parse(*source.Effective) (mise.LockedTool, error)
    Inspect(context.Context, *toolchain.Handle, mise.LockedTool) (mise.Observation, error)
    Install(context.Context, *toolchain.Handle, mise.LockedTool) (mise.Observation, error)
    Environment(context.Context, *toolchain.Handle, mise.LockedTool, receipt.ToolObservation) (mise.Environment, error)
    Exec(context.Context, *toolchain.Handle, mise.LockedTool, receipt.ToolObservation, []string, io.Reader, io.Writer, io.Writer) (execenv.Exit, error)
}
type Home interface {
    Map(context.Context, *toolchain.Handle, *source.Effective) ([]home.Mapping, error)
    Apply(context.Context, *toolchain.Handle, receipt.Candidate, *source.Effective, []receipt.DestinationObservation) ([]receipt.DestinationObservation, error)
    Verify(context.Context, *toolchain.Handle, receipt.Candidate) ([]receipt.DestinationObservation, error)
}
type Planner interface {
    Prepare(context.Context, *state.Lease, plan.Request) (*plan.Prepared, error)
}
type Store interface {
    AcquireFor(context.Context, state.Intent) (state.Acquisition, error)
    Initialize(context.Context, state.Acquisition) (state.Acquisition, error)
    RepairInitialization(context.Context, state.Acquisition) (state.Snapshot, error)
    RepairJournalLess(context.Context, *state.Lease, state.Snapshot) (state.Snapshot, bool, error)
    ReadCandidate(context.Context, *state.Lease, receipt.Digest) (receipt.Candidate, error)
    ReadCurrent(context.Context, *state.Lease) (state.Current, error)
    ReadTransition(context.Context, *state.Lease) (state.TransitionView, error)
    Begin(context.Context, *state.Lease, receipt.Candidate, []receipt.DestinationObservation) (state.TransitionView, error)
    RecordPhase(context.Context, *state.Lease, state.TransitionView, state.Phase, []receipt.DestinationObservation) (state.TransitionView, error)
    Commit(context.Context, *state.Lease, state.TransitionView, receipt.Completed) error
}
type Recoverer interface {
    Resume(context.Context, *state.Lease, state.TransitionView) (recovery.Result, error)
    Repair(context.Context, *state.Lease, state.Snapshot) (recovery.Result, error)
}
type Services struct {
    Sources Sources
    Toolchains Toolchains
    Mise Mise
    Home Home
    Planner Planner
    Store Store
    Recoverer Recoverer
    Runner execenv.Runner
}
type Options struct {
    Input paths.Input
    Version string
    Terminal bool
    Clock func() time.Time
    Random io.Reader
    NewServices func(paths.Roots) (Services, error)
}
func Run(context.Context, []string, io.Reader, io.Writer, io.Writer, Options) int

type operations struct { /* one complete Services value and app streams */ }
func newOperations(Services, bool, io.Reader, io.Writer, io.Writer) *operations
func (o *operations) profileList(context.Context, string) error
func (o *operations) plan(context.Context, plan.Request, receipt.Digest) (receipt.Digest, error)
func (o *operations) bootstrap(context.Context, plan.Request, receipt.Digest, bool) (receipt.Completed, error)
func (o *operations) apply(context.Context, receipt.Digest, bool) (receipt.Completed, error)
func (o *operations) doctor(context.Context) (string, error)
func (o *operations) recoverResume(context.Context) (recovery.Result, error)
func (o *operations) recoverRepair(context.Context) (recovery.Result, error)
func (o *operations) exec(context.Context, []string) (execenv.Exit, error)
func (o *operations) shell(context.Context, string) (execenv.Exit, error)
```

The lower service interfaces are consumer-owned by `app`; production implementations are exactly `source.Service`, `toolchain.Service`, `mise.Service`, `home.Service`, `plan.Service`, `state.Store`, and `recovery.Service`. `app.operations` is the only implementer of the complete PC-016 command flows; `host` remains assembly-only.

`Options.NewServices` is the only injected constructor boundary. Calling it may only assemble values from supplied canonical `paths.Roots` and captured immutable dependencies; it performs no filesystem/environment/process/network/root-resolution/durable-state work. It returns complete non-nil services or an error and has no fallback. `Options` contains no resolved roots, generic maps, `any`, service locator, optional production implementation, or callback other than the frozen clock and side-effect-free constructor.

```go
package host

func NewOptions(home string, environ []string, version string, terminal bool) (app.Options, error)
```

`host.NewOptions` calls only `paths.NewInput`, constructs one inert `execenv.NewRunner`, and calls `toolchain.NewFixedCA` exactly once with the frozen canonical bundle supplied as an immutable private string plus the exact reviewed SHA256. It fails on any CA mismatch before returning options. Its inert `NewServices` closure captures that immutable capability and supplies the same capability to `source.New`, `toolchain.New`, and `mise.New`, alongside the same runner; no constructor reads a CA from roots, environment, system state, artifact provenance, or mutable package state. This assembly performs no path resolution or external work and does not add an option, support, provider, private-transport, or credential surface. Production main captures home/environment/TTY once, first offers the reserved descriptor to `execenv.DispatchSupervisor`, then constructs options and calls `app.Run`.

`app.Run` first parses argv and performs only root-independent local grammar checks. Invalid argv returns before root resolution. Either version form returns the frozen version without resolving roots or activating services. Only a well-formed non-version command calls `paths.Resolve(options.Input)` exactly once, passes that one `paths.Roots` to `NewServices` exactly once, constructs `operations`, and invokes exactly one matching command method. Each method owns one complete lease and does not return across a pending approval. Package-private parser nodes, syscall adapters, record codecs, tracing fixtures, and mutation implementations remain private unless a later owning requirement and reviewed call site justify export.

Each non-version operation maps directly to one `state.Intent` and calls `AcquireFor` exactly once. The returned acquisition already names the only safe lease mode, under-lock classification, and PC-016 admission result. If an admitted CLEAN acquisition is returned, profile list/plan/bootstrap call `Initialize` once, assign its returned acquisition before inspecting its error, and keep that one still-owned lease for continuation or cleanup. If an admitted INIT repair acquisition is returned, recover repair calls `RepairInitialization` and releases I-X after verified completion. Other admitted repair calls `Recoverer.Repair`, which uses the state-owned journal-less seam before any transition read. All other admitted/denied rows finish under the returned lease. No operation probes a second mode, interprets a generic acquisition error as topology, calls an unlocked classifier, reacquires, upgrades, or delegates lease ownership.

Fresh apply/bootstrap and recovery obtain ZERB/platform receipt facts only through `Toolchains.ObserveRuntime` under A-X+W-X. Stable exec/shell call `Store.ReadCurrent` under A-S, read the exact candidate, select the candidate's authored `LockedTool` and the matching committed `receipt.ToolObservation`, resolve the already-promoted mise/Bash handles, and pass those stable facts to `Mise.Environment`/`Mise.Exec` or `execenv.ClassShell`. They never substitute a live manager observation for the committed receipt and keep A-S through FD-authenticated child release/reaping.

### ARCH-031 — Lock publication and command-admission linearization

ARCH-020 protected-role reservations precede product creation. Explicit base-prefix binding is physical observation only; minimum state-only I-X preparation requires its state-owned absent-or-empty eligibility predicate and grants no unlocked semantic classifier or writer authority.

The staged ARCH-020 protocol replaces named partially written temporary creation below: anonymous stage, durable immutable replacement record, complete no-clobber temp link, semantic revalidation, then one rename. Its exact pre-owner maintenance exception clears only authenticated residue and never supplies ordinary held-writer authority.

Initialization creates, file-syncs, and parent-syncs stable no-follow `0600` `admission.lock` and `writer.lock` inodes before initialized `root.json` can become visible. Both are opened close-on-exec, have one verified link, are never passed to children, and are never replaced or unlinked. Phase 1 freezes and proves the Linux process-held open-file-description shared/exclusive primitive and kernel release behavior for each filesystem tuple. A command acquires by descriptor, then reopens the pathname and verifies device/inode/owner/type/mode/link identity and the complete root topology before semantic validation. A path/descriptor mismatch is `BAD`, not a retry against a replacement inode.

Current-state commands take leases non-blockingly in this fixed order with no upgrade: shared or exclusive `admission.lock`, then exclusive `writer.lock` only after the under-admission classification admits a mutation/recovery requiring `A-X+W-X`. An incompatible live holder returns the exact PC-016 busy result before semantic validation; there is no wait timeout. A denied row retains and returns under its intent admission descriptor without opening writer ownership, downgrading, dropping, or reacquiring. Current-state operations never acquire `init.lock`. During first-use initialization only, `Initialize` holds I-X, requires the exact init-lock-only closed prefix, and binds the path/device/inode/bytes of that lock, claim, roots, and every prior object. It revalidates the complete closed prefix immediately before and after every initialization/repair action; an extra entry, root replacement, lock/claim/prior-object inode replacement with identical or altered bytes, or any same-UID namespace substitution stops before the next action. It opens and validates the newly published admission/writer inodes and acquires the requested A-X or A-X+W-X before initialized `root.json` becomes visible; it publishes that final initialization root last, retires its exact witness under the original I-X and target lease, and verifies READY with released generation zero. Bootstrap then performs ordinary held-generation-one acquisition and witness retirement under the already-held A-X+W-X; only afterward does it consume/release I-X and return the target lease, in ARCH-017 order. This one-way handoff is not an upgrade, and incompatible contenders continuously attempting across final-root publication cannot acquire before the target lease is released. Every plan and `profile list` use `A-X`; `doctor`, `exec`, and `shell` use `A-S`; admitted `bootstrap`, `apply`, and current-state recovery append W-X. Each retains the exact PC-016 lifetime. If admission/writer inodes or the owner predecessor are missing, malformed, or substituted, current MVP recovery never replaces them: safe `init.lock` permits only the exact evidence-preserving repair path.

`writer-owner.json` is published only while both exclusive locks are held, except its first released-generation-zero no-replace publication under I-X in ARCH-017. The publisher follows ARCH-020: prepare and verify an anonymous same-directory `0600` stage, durably publish/reopen the immutable replacement record, link the complete stage without clobbering, revalidate the stable writer-lock inode plus prior owner inode/schema/generation/token and record/graph relation, then attempt one rename over the owner path, parent-sync, reopen and verify the new inode and complete bytes. Exact live record retirement completes before advancing or unlocking. Acquisition compare-transfers `released(g)` to `held(g+1,new-token,new-process-identity)`. A stale `held(g,old-token,old-identity)` transfers only after the kernel exclusion is acquired and the full old process-start identity is proved dead. The new record binds the exact predecessor generation/token digest. Generation overflow, malformed records, absent identity evidence, a live or reused PID identity, or any inode/token/generation substitution is `BAD` and non-mutating.

The held writer authority is valid only while the same writer-lock descriptor remains exclusively locked and the path still resolves to its verified inode, the admission lease remains exclusive, and reopen verification finds the publisher's exact owner-record inode/generation/token/process identity. This conjunction is checked after acquisition publication, before every durable transition, immediately before and after every possible external mutation boundary, before stable/current publication, and before release. Every journal/operation token binds that writer generation/token. Two reclaimers serialize on the stable kernel lock; after waiting, the loser reclassifies from a fresh snapshot and cannot reuse the predecessor comparison. Random non-reused tokens, strictly advancing generations, predecessor binding, and inode revalidation close record and pathname ABA.

The guarantee detects every lock/owner path, inode, generation, token, process-identity, predecessor, or byte substitution present at a required revalidation boundary. It does not claim to detect an arbitrary same-UID writer that performs and fully restores an indistinguishable `A→B→A` cycle entirely between two checks; no finite start/end identity comparison can prove that event. Such a transient writer is outside cooperative exclusion, cannot obtain ZERB authority from the restored bytes, and remains subject to the before/after checks at every possible external mutation boundary. Evidence MUST exercise visible A→B substitution, restoration attempts crossing a boundary, token/generation reuse, and concurrent ZERB contenders; it MUST NOT label A-versus-B inequality as proof of an invisible fully restored cycle.

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
