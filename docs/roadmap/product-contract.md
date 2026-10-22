# Phase 1 Product Contract

**Status:** Ten Phase 1 plan/security review rounds failed; eleventh corrective proof/API work in progress; reviewed non-release implementation subject to delivery's cloud gate; Phase 1 unqualified; not a support claim
**Normative owner:** This document alone owns public CLI, local source-schema, approval, output, and user-visible state behavior.  
**Change gate:** Any change to this contract requires an owning requirement edit, affected proof-matrix update, fresh evidence, and the applicable review. Phase 1 candidate qualification does not create a support claim.

See the [roadmap index](README.md), [architecture invariants](architecture.md), [delivery sequence](delivery-plan.md), [validation requirements](validation.md), [redacted proof ledger](proof-results.md), [hard-cutover disposition](hard-cutover.md), and [post-MVP intent](post-mvp.md).

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative. Every requirement has a stable `PC-*` ID. A scenario is an observable consequence of its requirement, not evidence that the behavior exists today.

## Scope and contract state

### PC-001 — Product boundary and implementation status

**Invariant.** ZERB restores one declared embedded `base + selected profile` developer environment on a mutable workstation. It is not an operating-system image, backup system, package manager, transactional rollback engine, hermetic build system, or machine snapshot. The target product is pre-alpha, has no supported users, and is not implemented on the current branch. No platform, CPU, public native Git identity, mise backend, chezmoi feature, shell, provider, or parser behavior becomes supported merely because it is frozen for implementation; selected-gitlink/private transport is excluded.

**Scenarios.**

- A later proof that contradicts a frozen assumption requires a normative edit, affected-matrix rerun, and review; it does not add a fallback parser, transport path, second execution path, or unsupported claim.
- Before final installed-binary acceptance, documentation describes target behavior as provisional or qualified/frozen according to actual Phase 1 evidence, and not yet publicly reachable. Reviewed non-release implementation under delivery's cloud gate neither qualifies a candidate nor establishes support.
- A Phase 1 result may qualify a finite candidate for implementation, but support still requires VAL-001 and the applicable Phase 6 installed-binary vertical.
- A user seeking system packages, root-level configuration, rollback, or bit-for-bit host reproduction is told that the request is outside the product boundary.

## Command line

### PC-010 — Exact CLI grammar

**Invariant.** The MVP accepts only the following public command forms. There are no compatibility aliases, implicit commands, alternate public binaries, source defaults, JSON variants, or global source/profile state.

```text
zerb profile list <source>
zerb plan <source> --profile <id> [--trust-candidate sha256:<64-lowercase-hex>]
zerb bootstrap <source> --profile <id> [--trust-candidate sha256:<64-lowercase-hex>] [--yes]
zerb apply --candidate sha256:<64-lowercase-hex> [--yes]
zerb doctor
zerb recover resume
zerb recover repair
zerb exec -- <command> [args...]
zerb shell --shell <id>
zerb version
zerb --version
```

`<source>` is defined by PC-020. `<id>` is defined by PC-040. Each digest token is exactly `sha256:` followed by 64 lowercase hexadecimal characters. `<command>` is either a bare executable name with no slash or a canonical absolute executable path. A bare name is resolved only against the already-constructed stable-profile `PATH`; a slash-containing relative path is invalid.

**Scenarios.**

- An omitted required positional argument or flag, an extra argument, an unknown command or flag, a malformed digest, a repeated singleton flag, or a missing command after `exec --` is invalid usage under PC-014.
- `exec` requires the literal `--` delimiter; wrapper flags are never inferred from payload arguments.
- `shell` requires an explicit Phase 1 implementation-qualified shell ID and never consults `$SHELL` as a selection mechanism.
- `version` and `--version` are equivalent public version forms.
- Stable-profile `PATH` entries are canonical absolute directories with no empty, relative, dot, duplicate, or writable-unowned component. Ambient `PATH` and the current directory never participate in executable resolution.

### PC-011 — Parsing and usage discipline

**Invariant.** Each subcommand uses the Go standard library `flag.FlagSet` with `ContinueOnError`. ZERB does not use Cobra or Viper. Parsing is non-exiting, writes usage diagnostics to the supplied stderr stream, and returns through the command runner. Values have no ambient or current-directory default. Only flags shown in PC-010 are accepted for an operation.

Command evaluation has this fixed precedence:

1. Parse the complete argv and perform only locally decidable token checks, including operand count, flag placement, digest shape, source byte grammar, profile-ID grammar, shell-ID grammar, and the `exec --` boundary. An invalid form returns its PC-014 usage or policy result before resolving or opening a durable root.
2. After successful parsing, either version form returns without root access. Every other form performs only the minimum no-follow path and synchronization-inode checks needed to obtain the PC-016/ARCH-031 lease; it does not semantically classify state or validate a durable candidate, executable, tool, destination, or receipt first.
3. With the lease held, ZERB reopens and revalidates the synchronization inode and root topology, applies PC-061 classification, then applies the PC-016 state row. Only an admitted command proceeds to candidate, approval, executable, tool, destination, or recovery validation.

Missing non-interactive approval is evaluated at its bound operation boundary after state admission. It returns `2` only if the command was otherwise admitted; it does not mask a foreign, corrupt, unsupported, initializing, or disallowed transition state. A flag-like payload argument after `exec --` belongs to the payload and is not parsed by ZERB.

### PC-012 — Stable text output

**Invariant.** Text is the only MVP output format. The following lines are stable public output; success MUST NOT depend on color, icons, or unstable progress text.

| Command/result | Required stdout contract |
|---|---|
| `profile list` success | Sorted logical profile IDs, one per line, with no header |
| `plan` success | Final line `Candidate: sha256:<digest>` |
| `apply` success | `Applied: sha256:<digest>` |
| `bootstrap` success | `Applied: sha256:<digest>` |
| `doctor` | First line begins exactly `Healthy` or `Unhealthy` |
| `recover resume` or candidate/cleanup/prior-STABLE `recover repair` success | `Recovered: sha256:<digest>` |
| Initialization-only `recover repair` success | `Repaired: initialization` |
| Journal-less `recover repair` restoring `READY(∅)` | `Repaired: state` |
| Either version form | `zerb <version>` |

`exec` and `shell` pass the child streams through and add no success wrapper that could corrupt machine-consumed output.

**Scenarios.**

- `profile list` over IDs `work`, `personal` prints `personal` then `work`, each followed by a newline.
- A successful plan may print stable, redacted inspection information, but its last line is the candidate line.
- Machine-readable JSON, sensitive content diffs, and unstable progress events are not MVP output.
- Initialization repair and journal-less repair restoring empty `READY(∅)` never invent a candidate or receipt digest. Empty state repair prints exactly `Repaired: state`; repair restoring prior `STABLE(A)` and transition/cleanup recovery print the validated current candidate digest.

### PC-013 — Errors, abstraction, and redaction

**Invariant.** Every ZERB-generated error begins with the exact prefix `Error: ` on stderr. Sensitive locators, destination targets, authentication/profile names, rendered values, credentials, and ambient secret values are redacted before formatting or causal wrapping. Public errors describe the ZERB operation and actionable policy boundary; raw manager output is not exposed when it can disclose implementation details or sensitive data.

ZERB MUST NOT accept, copy, print, log, or persist credential bytes, provider values, or host-fact values. Candidate artifacts, state, journals, receipts, diagnostics, cache metadata, and generated non-secret runtime configuration contain no authentication, provider, rendered-secret, ambient-secret, or host-fact value. Provider references and host-fact functions are invalid MVP input. A selected profile may be trusted code with normal user authority after approval; it is not represented as sandboxed.

**Scenarios.**

- A selected gitlink or private source fails at the source-selection boundary without disclosing its locator and before authentication, network, candidate creation, manager execution, or mutation.
- An upstream public-fetch error containing a credential-bearing URL is scrubbed before it is wrapped with causal context.
- A plan reports structural target conflicts without printing secret-bearing file content.
- An invoked user payload may intentionally read or modify user files; that behavior is outside ZERB's storage guarantee, while ZERB still does not supply unrelated manager credentials or state paths.

### PC-014 — Exit status classes

**Invariant.** Before launching an `exec` or `shell` child, ZERB returns only:

- `0` for success;
- `1` for operational, policy, trust-mismatch, conflict, health, recovery, or launch failure; and
- `2` for invalid usage or missing required non-interactive approval.

After child launch, PC-015 owns the returned status.

**Scenarios.**

- A malformed candidate digest exits `2` without work beyond parsing.
- A well-formed `--trust-candidate` that does not equal the computed candidate exits `1` before post-trust execution.
- Missing candidate trust or mutation authorization in a non-terminal invocation exits `2` before the corresponding boundary; selected gitlinks and private sources are policy failures, not approval opportunities.
- A destination ownership conflict exits `1`; it is not converted into usage failure and has no force override.

### PC-015 — Child, signal, and cancellation status

**Invariant.** Once an `exec` or `shell` child has launched, every normal child exit status from `0` through `255` is returned unchanged. A child terminated by signal returns `128 + signal`. Under the Phase 1-frozen Linux lifecycle policy, a catchable POSIX signal received by the wrapper is forwarded to the managed child boundary, escalation occurs after the frozen grace period if necessary, all managed descendants covered by that policy are reaped, and the wrapper returns `128 + signal`. Thus SIGINT returns `130` and SIGTERM returns `143`. Context cancellation with no associated POSIX signal follows the same termination, escalation, and reaping policy and returns `1`.

An uncatchable wrapper death has no return status. Its consequences are diagnosed from truthful durable state on restart. A deliberately detached arbitrary user payload may be outside the orchestration guarantee only if Phase 1 explicitly freezes that limitation and validation proves that manager credentials and state paths are not supplied to it.

**Scenarios.**

- A payload that calls `_exit(255)` causes ZERB to return `255`, not `1`.
- A child killed by SIGINT produces `130`.
- A wrapper receiving SIGTERM forwards it under the frozen lifecycle, escalates only after the frozen grace period, reaps, and returns `143`.
- A hard-killed wrapper returns nothing; the next admitted command observes stable or recognized dirty state rather than inventing success.

### PC-016 — Command operation, state admission, and leases

**Invariant.** Commands have these public operation boundaries:

- `profile list` performs unauthenticated public static inspection and prints manifest IDs. Before releasing `A-X`, every fetched source byte and intermediate tree remains in the owner-validated untrusted per-instance cache and every operation-owned staging entry is removed or confined there; it writes no durable source/tree object, candidate, adapter, toolchain, manager state, receipt, journal, current record, or destination.
- Every `plan` conservatively acquires `A-X` before root, state, source, or candidate validation; there is no result-dependent shared-lease path. Before exact candidate trust, fetched source bytes, acquisition bytes, effective-source assembly, and adapter scratch remain only in the owner-validated untrusted per-instance cache. After trust, `plan` may publish the immutable candidate and independently valid content-addressed source, tree, and verified toolchain objects under the ARCH-016/ARCH-024 rules. Its complete write allowlist is protected root-initialization markers and synchronization files; the per-instance cache; those immutable post-trust records/objects; and no-replace publication metadata. It does not install declared tools, mutate home destinations, invoke a provider, or print content diffs. Named or partial staging content never remains under state or data.
- `bootstrap` orchestrates the same explicit source inspection, trust, persisted candidate, confirmation, apply, verification, receipt, and stable-commit path used by standalone operations. It maintains no hidden source or candidate state unavailable to those operations.
- `apply` consumes exactly the candidate named by `--candidate`; absent, ambiguous, stale, changed, or untrusted artifacts fail before mutation.
- `doctor` diagnoses root, schema, state, tool, and source readiness without adopting, deleting, repairing, or relabeling state.
- `recover resume` and `recover repair` perform only supported forward convergence. Neither claims rollback nor requires manual journal editing.
- Only `recover repair` admits the exact journal-less dead-writer prefix defined by ARCH-027. A pre-journal prefix restores prior `READY(∅)` or `STABLE(A)` authority; a committed-and-cleaned prefix finishes release while preserving its exact committed current. Neither executes an intended candidate. Ordinary released READY/STABLE state still has no recovery operation.
- `exec` resolves a bare payload only against the stable-profile executable set, or accepts a canonical absolute payload, then runs an identity-checked regular Linux x86_64 ELF image from an immutable sealed byte snapshot without mutating the parent process. Scripts, shebangs, and other executable formats are rejected before child release.
- `shell` accepts only the implementation-qualified ID `bash` and runs the exact Bash 5.3.15 ELF identity as `--noprofile --norc -i` on a clean PTY without normal startup files or parent mutation. Job-control process groups remain inside the frozen cumulative cgroup/lifecycle boundary.
- Version forms perform no source, state, network, manager, destination, or executable-resolution access.

- A canonical script with a valid shebang is still ineligible for `exec`; ZERB returns policy failure before releasing an interpreter or leaking a snapshot descriptor.
The state symbols in the admission table are disjoint results of PC-061 and ARCH-018:

- `CLEAN` is the exact uninitialized first-use predicate.
- `FOREIGN` has no recognizable ZERB evidence.
- `INIT` is recognizable incomplete initialization.
- `BAD` is evidence-preserving corrupt or unsupported state.
- `READY(∅)` is a complete supported initialized pair with no completed current candidate or transition.
- `STABLE(A)` is a complete supported stable relation for candidate `A`.
- `TRANSITION(A,B)` is one exact supported `prepared`, `mutating`, or `dirty` relation.
- `CLEANUP(B)` is stable `B` plus only its exact supported post-commit cleanup residue.

Lease symbols are `I-X` for the exclusive stable initialization lock, `A-S` for a shared admission lease, `A-X` for an exclusive admission lease with no writer ownership, and `A-X+W-X` for exclusive admission followed by exclusive writer ownership. `D-S` is a read-only diagnostic snapshot: it uses `A-S` when the admission inode is valid or the stable initialization inode for an `INIT` or lock-corruption snapshot when safely available; with neither inode safe it reports only that non-mutating topology failure and does not parse or validate semantic durable records. `AcquireFor` chooses one intent-owned admission mode before semantic classification: `A-S` for doctor/exec/shell and `A-X` for profile/plan/bootstrap/apply/recovery, adding `W-X` only after the under-A-X row admits a mutation or recovery. A denied row returns under the already-held intent admission lease; it never drops, downgrades, upgrades, or reacquires merely to report D-S. “Deny” returns `1` without the denied command's source, candidate, manager, destination, or child action. Current-state admission acquisition is non-blocking: an incompatible live lease returns operational busy `1` before semantic classification.

| PC-010 form | `CLEAN` | `FOREIGN` | `INIT` | `BAD` | `READY(∅)` | `STABLE(A)` | `TRANSITION(A,B)` | `CLEANUP(B)` |
|---|---|---|---|---|---|---|---|---|
| `version`, `--version` | No lease or root access | Same | Same | Same | Same | Same | Same | Same |
| `profile list <source>` | `I-X` initialization, then `A-X`; admit | Deny | Deny | Deny | `A-X`; admit | `A-X`; admit | Deny | `A-X`; admit |
| `plan <source> --profile <id> …` | `I-X` initialization, then `A-X`; admit | Deny | Deny | Deny | `A-X`; admit | `A-X`; admit, but PC-071 rejects a result other than exact `A` before publication | Deny before source access, cache staging, or candidate validation | Deny before source access, cache staging, candidate validation, or cleanup |
| `doctor` | `D-S`; diagnose clean without initializing | `D-S`; diagnose only | `D-S`; diagnose only | `D-S`; diagnose only | `A-S`; diagnose | `A-S`; diagnose | If no live exclusive holder, `A-S` and diagnose exact state; otherwise busy `1` | `A-S`; diagnose exact residue |
| `recover resume` | Deny: no operation | Deny | Deny: initialization requires repair | Deny: `BAD` is not a supported resumable relation | Deny: no operation | Deny: no operation | `A-X+W-X`; exact forward resume | `A-X+W-X`; finish exact journalled committed cleanup; deny journal-less residue requiring repair |
| `recover repair` | Deny: no operation | Deny | `I-X`; only exact initialization repair/takeover | If stable admission/writer inodes and the prior owner publication validate, `A-X+W-X` admits only a Phase 1-frozen repair of exact related residue/records; a missing, malformed, substituted, or unprovable synchronization inode/owner predecessor uses `I-X` only when `init.lock` is safe and returns `1` without replacing it; with no safe exclusion it returns `1` after topology-only diagnosis | Deny: no operation | Deny: no operation | `A-X+W-X`; exact forward repair | `A-X+W-X`; finish exact committed cleanup |
| `bootstrap` | `I-X` initialization, then `A-X+W-X`; admit | Deny | Deny | Deny | `A-X+W-X`; admit | `A-X+W-X`; admit | Deny | Deny until cleanup |
| `apply --candidate` | Deny: no candidate can exist | Deny | Deny | Deny | `A-X+W-X`; admit only for an exact published candidate | `A-X+W-X`; admit only for an exact published candidate | Deny | Deny until cleanup |
| `exec -- …`, `shell --shell …` | Deny: no stable profile | Deny | Deny | Deny | Deny: no stable profile | `A-S`; admit against `A` | Deny | `A-S`; admit against stable `B` |

Every plan takes `A-X` before state/source/candidate validation and keeps it through cache staging, candidate trust, every post-trust immutable source/tree/toolchain or candidate no-replace publication, file and parent synchronization, reopen verification, and cache cleanup. It never drops and reacquires or upgrades a lease. In `STABLE(A)`, the result must remain exact `A`; a changed result is an unsupported update under PC-071 and is not published. `TRANSITION` and `CLEANUP` deny planning entirely. Concurrent candidate writers therefore serialize; an existing immutable record is accepted only after exact byte/schema/digest equality and is never overwritten. A missing-trust exit leaves only untrusted per-instance cache bytes and no durable state/data staging residue.

Mutation and recovery keep `A-X+W-X` for the whole operation, including every revalidation, prompt, external mutation, durable transition, stable commit, and cleanup or truthful failure publication. `bootstrap` takes the exclusive lease before planning and never upgrades a shared lease. `exec` and `shell` keep `A-S` from stable-state validation through executable/environment identity binding, spawn, managed-child release/reaping, and final status. Thus no cooperating mutation can change current identity between read and launch or while the managed child boundary is live. `profile list` keeps `A-X` through cache staging and cleanup; `doctor` holds `A-S` through completion of its diagnostic snapshot. ARCH-031 defines publication, takeover, lock order, and revalidation.

**Scenarios.**

- Invalid argv always follows PC-011 before root classification; a malformed digest returns `2` even if a corrupt root exists, while a well-formed command reaches the root/state row before missing-approval handling.
- A non-terminal admitted public embedded plan without required candidate trust computes and displays the candidate digest, then exits `2` before post-trust work.
- Selecting any gitlink exits `1` before authentication, network, candidate creation, manager execution, or mutation; no transport digest is computed or accepted.
- A plan raced with recovery either completes under `A-X` before recovery can acquire, begins after recovery and is classified from the resulting state, or loses non-blocking acquisition and returns busy `1`; it never validates source or candidate state against an unlocked snapshot.
- An `exec` raced with `apply` either binds and releases the child under the old `A-S` before apply begins, observes the new stable identity after apply, or returns busy `1` if apply already owns `A-X`; no read-transition-launch interval exists.
- A plan cannot install a declared tool or write a home target, and `profile list` cannot write a candidate or toolchain artifact.

## Source identity and grammar

### PC-020 — Canonical public source

**Invariant.** Public `<source>` is an explicit, credential-free, absolute `https://` URI whose input bytes already equal its canonical serialization. It has:

- no userinfo, query, fragment, control byte, backslash, dot segment, or encoded path separator;
- no non-canonical percent encoding;
- a lowercase scheme and lowercase ASCII host;
- no trailing host dot and no explicit default port; and
- a non-empty path.

Redirects are disabled. Before native Git receives the URI, ZERB resolves the canonical host through the frozen resolver boundary, rejects the complete answer set if any address is not globally routable unicast or is loopback, private, link-local, multicast, unspecified, documentation-only, or otherwise nonpublic, and binds Git/libcurl to that exact validated address set with the pinned Git 2.55 `http.curloptResolve` interface while preserving the original TLS hostname. Every observed connect must match the bound set; DNS answer change or rebinding cannot alter the fetch endpoint. Public fetch has every credential channel disabled and fails an authentication challenge rather than invoking a helper. Local paths, `file:` URIs, scp-like syntax, ambient/current-directory remotes, and any value requiring canonical rewriting are rejected. More than one URI byte representation, an unbound connect, or a nonpublic answer fails before candidate creation or promotion.

**Scenarios.**

- `https://example.com/environment.git` is structurally eligible for Phase 1 validation.
- `HTTPS://example.com/environment.git`, `https://EXAMPLE.com/environment.git`, `https://example.com:443/environment.git`, and `https://example.com/environment.git#main` are rejected rather than rewritten.
- A redirect response fails without following its target.
- A public-looking hostname resolving to any private/link-local/loopback/mixed answer fails before Git connection; changing DNS after validation does not change the pinned connection target.
- `./environment`, `file:///tmp/environment`, and `git@example.com:environment.git` are rejected public sources.

### PC-021 — Gitlink declaration and selected-source exclusion

**Invariant.** The sole positive MVP selection is an embedded component fetched as part of the canonical public HTTPS superproject. A public manifest may declare unselected `gitlink` profiles only when each `.gitmodules` locator is statically canonical absolute HTTPS or SSH and exactly matches the declared path and superproject gitlink. Static acceptance is not transport eligibility. Selecting any gitlink fails closed before authentication, network, candidate creation, manager execution, or mutation. An unselected gitlink is never fetched, authenticated, contacted, resolved through a helper, or recursively traversed.

Private HTTPS and SSH sources are excluded. Relative URLs, scp-like syntax, credential-bearing locators, unsafe protocols, nested submodules, mismatched paths, non-gitlink tree entries, and ambiguous locators fail during bounded static validation. ZERB has no MVP helper, agent, relay, private endpoint, credential input, store selector, identity-file, known-hosts input, or transport-authorization path.

**Scenarios.**

- Listing profiles and selecting an embedded public profile invokes no helper, agent, relay, private endpoint, authentication, or gitlink network access.
- Valid canonical HTTPS and SSH locators on unselected gitlinks may pass static manifest validation, but receive zero authentication and network attempts.
- Selecting a statically valid gitlink returns policy failure before source transport, candidate creation, manager execution, or mutation.
- A nested gitlink or invalid locator fails during static validation without making the selected declaration accessible.

### PC-022 — Source and object identity separation

**Invariant.** `source_id` is `sha256:<64-lowercase-hex>` over `encoding/json` bytes of the exact map-free field order `{"schema":1,"domain":"zerb.source.v1","uri":"<canonical-public-uri>"}`, where `uri` is the byte-exact PC-020 canonical public URI. This one encoder is shared by acquisition, candidate, state, receipt, and approval proofs; no slice-specific prefix or alternate serialization exists. The source ID is not a Git commit or availability observation. Fetched public commit IDs, tree/object IDs, embedded component paths, and materialized tree digests remain separate identity fields. Unselected gitlink locator/commit declarations are static manifest facts only and never candidate/materialized-tree identity. ZERB never infers source identity from a current directory or ambient remote.

**Scenarios.**

- `https://github.com/ZebulonRouseFrantzich/zerb.git` has the fixed shared known-vector `source_id` `sha256:ca92a55154224f40a4e208e1412d8c2530c2afb345b2dfd1a55b9627a7b1b62b`.
- The same canonical source URI at two commits has one `source_id` and two distinct commit identities.
- A redirect target cannot silently replace the caller's source identity because redirects are disabled.
- An embedded profile identity binds the public source, superproject commit, component path, and component tree digest; selecting a gitlink fails before candidate identity exists.

## Approval boundaries

### PC-030 — Approval separation and non-terminal behavior

**Invariant.** Candidate trust and host mutation authorization are independent decisions. No one approval implies another. An interactive prompt binds the same digest as its explicit flag. When stdin is not a terminal, an omitted approval exits `2` before the corresponding trust or mutation boundary. The MVP has no transport authorization, transport prompt, transport digest, or private-source approval surface.

**Scenarios.**

- `--yes` cannot authorize candidate execution or make a gitlink/private source eligible.
- `--trust-candidate` cannot authorize a gitlink/private endpoint or destination mutation.
- Any transport-authorization flag is invalid usage under PC-010/PC-011.
- A command may prompt separately for candidate trust and mutation, and each prompt states and binds only its own object.

### PC-031 — Candidate trust

**Invariant.** `--trust-candidate` MUST exactly equal the complete candidate digest ZERB computed under PC-050. Every authored tool version and required lock value, selected source/tree identity, adapter identity, policy field, and deterministic structural target inventory is candidate-bound before this approval and before rendering or any candidate-selected manager execution. The sole pre-trust structural-adapter exception is one invocation of the exact `P1-CHEZMOI-STATIC` fork executable's `managed --skip-template-data-environment` read-only, non-rendering, machine-readable target-mapping mode over the protected effective source. That invocation may derive only the deterministic structural target inventory and MUST use the exact fork source/patch/build/executable identity, argv, complete environment, descriptor set, working/config/cache/state roots, and synthetic non-real destination root. It executes no semantic host-fact source (`uname`, `sysinfo`, UID/GID/group lookup, passwd/group database, hostname, OS-release, kernel template data), template, helper, provider, script, external, or candidate content and accesses or writes no real destination. The statically built pinned Go runtime may read only the separately traced bootstrap allowlist—`/proc/self/{cgroup,mountinfo,exe}`, transparent-hugepage size, and `cpu.max` at each directory in the current cgroup hierarchy—and may make only the separately counted `sched_getaffinity` and `prlimit64` runtime-bootstrap syscalls, solely for runtime scheduling, memory, executable, and resource setup; those observations are unavailable to mapping/template data, excluded from output/digests, and any additional `/proc`/`/sys` read or host-fact syscall fails. The x86_64 manager seccomp policy rejects `io_uring_setup`, `io_uring_register`, and `io_uring_enter` before release so the closed trace cannot be bypassed. Normal and flagged structural output MUST be byte-identical for the retained corpus. If the exact fork/build cannot prove every restriction, or the patch changes mapping/render/apply semantics, the candidate fails and the surface is removed; ZERB never implements a fallback mapper.

After exact trust, static rendering may begin. Real-destination access and every live topology, type, content, link, mode, and owner observation begin only after mutation authorization under the required `A-X+W-X` ownership relation. Post-trust work may verify or execute the immutable candidate; it MUST NOT resolve an alias, generate or change a lock, add an identity field, or otherwise mutate the trusted digest. Any candidate or bound-policy change requires a newly constructed candidate and new trust. A mismatch is a trust failure, not an implicit prompt fallback.

**Scenarios.**

- A first non-terminal plan may reveal `Candidate: sha256:<digest>` and exit `2`; a later invocation can supply that exact digest.
- A one-byte candidate or structural-target-inventory change makes the old trust value fail with exit `1` before rendering or candidate-selected manager execution.
- An alias, range, channel, `latest`, or missing exact lock fails before the candidate is offered for trust; there is no live post-trust resolution step.
- Trusting one profile does not trust another profile at the same source commit.

### PC-032 — Selected-gitlink and private-transport exclusion

**Invariant.** PC-032 defines an exclusion, not an authorization tuple. The MVP has no eligible selected-gitlink or private HTTPS/SSH transport mechanism and produces no transport identity, digest, approval prompt, or typed identity input. Selection of any `kind = "gitlink"` profile and every private source attempt fails before authentication, network, candidate creation, manager execution, or mutation.

ZERB MUST NOT accept or construct a credential helper, askpass program, SSH agent/socket, identity or key file, known-hosts input, credential store selector, synthetic SSH configuration, proxy, relay, private endpoint channel, mechanism tuple, or transport authorization. Ambient and explicit forms are equally ineligible. The static validation allowed by PC-021 for unselected canonical HTTPS/SSH gitlink locators performs no resolution, connection, authentication, helper invocation, candidate construction, or materialization of the gitlink.

**Scenarios.**

- Selecting a valid canonical HTTPS or SSH gitlink returns policy failure before Git is invoked for that gitlink.
- Supplying a helper, agent, relay, key/identity file, known-hosts file, proxy, endpoint input, tuple, or transport-authorization flag cannot make the selection eligible.
- An unselected canonical gitlink is statically checked and never fetched, including during profile listing and embedded-profile planning/bootstrap.
- Public unauthenticated superproject fetch consumes no private transport input and exposes no transport approval output.

### PC-033 — Mutation authorization

**Invariant.** `--yes` authorizes only the exact pending host mutation. Without `--yes`, an interactive terminal may obtain a mutation confirmation after structural conflicts and the exact candidate are known. Non-terminal mutation without `--yes` exits `2` before the first possible external mutation. There is no `--force`, ownership override, or implied authorization from a prior trust decision.

**Scenarios.**

- `plan` never uses `--yes` because it cannot perform declared-tool or destination mutation.
- `bootstrap --yes` with missing candidate trust still stops at the trust boundary.
- An unowned pre-existing destination remains exit `1` even when `--yes` is present.

## Source schemas and composition

### PC-040 — `zerb.toml` schema 1

**Invariant.** The superproject root contains exactly one `zerb.toml` decoded as schema version 1. This example is the complete frozen field shape:

```toml
schema = 1

[base]
path = "profiles/base"

[[profiles]]
id = "personal"
kind = "embedded"
path = "profiles/personal"
```

Rules:

- `schema` is required and exactly integer `1`.
- `[base]` is required, has only `path`, and is always embedded.
- `profiles` contains at most 256 selectable entries.
- Each profile has exactly `id`, `kind`, and `path`.
- `id` matches `[a-z0-9][a-z0-9_-]{0,63}` and is unique.
- `kind` is exactly `embedded` or `gitlink`.
- A path is valid UTF-8, slash-separated, clean, relative, and non-empty. It contains no empty component, `.`, `..`, absolute form, NUL, backslash, or duplicate normalized form.
- A `gitlink` path matches exactly one statically validated `.gitmodules` entry and the corresponding tree entry is a gitlink.
- Unknown fields, tables, schema versions, source kinds, and object types fail closed.

**Scenarios.**

- Schema `2`, duplicate ID `personal`, kind `private`, path `../personal`, and an undeclared field each fail before candidate creation.
- Two paths that normalize to the same UTF-8 slash path fail rather than alias.
- An embedded path naming a gitlink, or a gitlink path naming an ordinary tree, fails.
- `profile list` prints only declared selectable IDs, sorted as PC-012 requires; `base` is not a selectable ID.

### PC-041 — Component `profile.toml` schema 1

**Invariant.** Every base or selected component root contains one `profile.toml` with exactly required `schema = 1` and an optional string `label`. No other field or table is accepted. A component may contain optional `mise.toml`, optional `mise.lock`, and optional `home/`; at least one of `mise.toml` or `home/` MUST exist. `mise.lock` alone is not a component payload. A label is display metadata, not a logical ID, source identity, approval input substitute, or secret-value field.

**Scenarios.**

- `schema = 1` plus `label = "Personal"` is structurally valid subject to PC-042's frozen string bounds.
- Unknown `tools`, `provider`, or nested metadata fields fail; declarations belong to the native managed files, not a second ZERB DSL.
- A component containing only `profile.toml` and `mise.lock` fails.
- Secret values in profile metadata are unsupported and must not be copied into a candidate or receipt.

### PC-042 — Bounded strict parsing

**Invariant.** Every TOML document is bounded to at most 1 MiB before decode. `zerb.toml` declares at most 256 selectable profiles. Decoding uses strict Go structs with the owner-approved repository-local `github.com/pelletier/go-toml/v2` variant `v2.4.3-zerb.1`, full patched-module sum `h1:5hB+HyHqViZpFF0bpFFUYMD0v4y+9w/rtDABm6PmSIc=`, and unchanged go.mod sum `h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=`, through `toml.NewDecoder(...)` and `DisallowUnknownFields()`. Its sole production change zeros the decoder before returning it to the pool. The exact original base is upstream `v2.4.3`, commit `071a36c2a57244f2e70369bfc69889fda2a1f60f`, module sum `h1:GTRvJQutkOSftxIFD5xw9aepkYNuPWmVJpffdDPYVpY=`. `Parser.OriginCommit` denotes that upstream base, not an invented patched commit; `Parser.ModuleSum` authenticates the complete local variant. The source, original file hashes, exact patch and regression tests are bound by `third_party/go-toml.provenance.json` and its mandatory read-only verifier. The version-specific local replacement needs no remote publication; ordinary `go mod verify` does not authenticate it.

Duplicate keys or tables, dotted-key aliases, unknown nested fields or tables, malformed input, prohibited invalid UTF-8, trailing semantic content, and any representation with multiple semantic decodings fail. Bounds are checked before allocation or semantic decode where applicable. The complete frozen limit table is:

| Field/surface | Maximum | Unit |
|---|---:|---|
| TOML document | 1,048,576 | bytes |
| Selectable profiles | 256 | entries |
| Profile ID | 64 | UTF-8 bytes |
| Profile label | 256 | UTF-8 bytes |
| Canonical public source | 2,048 | ASCII bytes |
| Normalized logical path | 1,024 | UTF-8 bytes |
| Path depth | 64 | components |
| Path component | 255 | UTF-8 bytes |
| Component files | 4,096 | entries |
| Tool declarations | 1 | entry |
| Tool ID | 4 | ASCII bytes |
| Exact version | 6 | ASCII bytes |
| Locks per tool | 1 | entry |
| Exact lock value | 71 | ASCII bytes |
| Effective home sources | 4,096 | entries |
| Structural targets | 4,096 | entries |
| Static template | 1,048,576 | bytes |
| Template nesting | 32 | AST levels |
| Template actions | 4,096 | actions |
| Candidate strings | 16,384 | entries |
| Aggregate parser/model allocation | 8,000,000 | bytes |

The allocation row is a deterministic conservative reservation budget for one complete parsing or canonicalization API invocation, including all of its passes and its retained result. It covers input-buffer copies and growth workspace, AST/tracker backing storage, decoded/copied strings and collections, result-model storage, and canonical output buffers where applicable. The implementation reserves these input-dependent capacities before entering the corresponding parser, decoder or materializer; it uses checked arithmetic and rejects if the complete reservation exceeds the limit. It must not reset the budget between passes of the same invocation or substitute a post-decode payload estimate. Each later composition/canonicalization boundary reserves its complete retained input model and new workspace together, rather than claiming that earlier per-document success bounds a complete candidate.

Caller-owned input bytes are separately byte-bounded and are not charged again as a new allocation; every internal copy is charged. Pooled or reused input-dependent buffers, AST arenas and tracker backing remain charged by the invocation's schedule even when they allocate no new storage. Runtime allocator metadata, stack/GC behavior and input-independent fixed-schema cache metadata are outside this accounting. This approved reservation contract is neither literal cumulative Go allocation enforcement nor a hard runtime-memory cap. Reservation schedules are tied to the exact reviewed parser and closed schema; parser/schema changes require review and affected budget/boundary tests before reuse.

**Scenarios.**

- A 1 MiB + 1 byte TOML document fails before normal decode and creates no candidate.
- An unknown nested table, duplicate key, duplicate table, or dotted-key alias fails even if the decoded Go value would otherwise look valid.
- A parser result that accepts two semantic interpretations or bypasses a bound rejects the parser/version and requires a normative narrowing plus full affected proof/review rerun.
- A future parser upgrade cannot silently widen accepted syntax; it requires the same conformance and fuzz corpus.

### PC-043 — Exact profile composition and capability limits

**Invariant.** MVP composition is exactly one embedded base component plus exactly one selected component, each composed once. There is no inheritance, profile stack, machine-specific merge graph, silent override, or sequential independent home apply. ZERB builds one effective native mise context and one effective native chezmoi source. Duplicate tool identities, duplicate raw home-source paths, and duplicate resolved destination targets fail rather than override.

Before candidate acceptance, ZERB compares each normalized structural target identity only as text against the normalized closed reserved-role namespace and every absent future control leaf derived from already-resolved product/manager roots. This static check performs no real-home traversal or live destination observation and binds both the target identity and the required post-authorization revalidation policy into the candidate. After exact candidate trust and mutation authorization under `A-X+W-X`, but before manager access that can mutate, filesystem creation, or any external mutation, ZERB opens the real destination and rejects any canonical/descriptor/owner/type/link/device/inode/mount/bind alias, replacement, containment, or descendant relation with a protected/control/manager role. It repeats those live checks at every affected boundary.

MVP accepts only Phase 1 implementation-qualified native mise declarations/backends/package classes and chezmoi entries/static templates. Every tool request is an exact authored version with every required exact lock value already present; both are statically validated and candidate-bound before mise or any other candidate-selected manager execution. Aliases, ranges, channels, `latest`, missing locks, and live lock generation or mutation are excluded. Tasks, hooks, plugins, custom backends, sourced environment, chezmoi scripts, externals, authored removals, root controls, arbitrary execution/template functions, target overrides, provider references/functions, and host-fact functions are excluded. PC-031's one verified pre-trust chezmoi structural-mapping invocation is the sole manager exception; it produces only normalized target structure and executes no candidate content. Candidate-selected static templates, installers, backends, and helpers are trusted execution after PC-031, not sandboxed input.

**Scenarios.**

- The same tool identity in base and selected fails even if versions match.
- The same raw home source or resolved target in both components fails rather than selecting one winner.
- A structural target text naming an absent future `current.json`, a protected-root ancestor, or a manager-state descendant fails before trust without touching the real home. A bind/device/inode alias not decidable from text fails after trust and mutation authorization but before any manager or filesystem mutation.
- Base is never omitted and the selected component is never applied twice.
- Changing or generating a lock after trust is forbidden even if it would resolve to the same installed version.
- Unsupported executable source controls fail before mutation rather than being ignored or delegated to a fallback.

## Digests and repeatability

### PC-050 — Canonical digest encoding

**Invariant.** Source-tree, policy, candidate, and receipt digests use `sha256:<64-lowercase-hex>`. Each digest input is an explicitly versioned, map-free Go struct serialized with `encoding/json`. Every slice is sorted lexicographically by normalized UTF-8 byte sequence before encoding; every path uses `/`. Timestamps, availability observations, interactive/flag approvals, mutable or live destination observations, and other run-local observations are excluded from the candidate digest. Included modes normalize to ordinary-file executable, ordinary-file non-executable, or directory. Unsupported object types fail before hashing. The MVP defines no transport-approval digest.

The candidate binds its selected source/object identities, profile ID, composed content and modes, declarations, exact authored tool versions and lock values, exact managed executable identities, parser/schema version, allowlists, execution policy, and the normalized structural target inventory produced solely by PC-031's proven pre-trust mapping invocation. That inventory contains deterministic target identities derived from the protected effective source, not live destination topology, type, content, link, mode, owner, existence, access, or availability. A manager observation may confirm already-bound fields but may not add to or mutate the digest. Secret-provider lookup and host-fact inputs are not MVP candidate surfaces. Phase 1 freezes the exact structs and domain/version tags; no generic map or unstable upstream serialization may become normative.

**Scenarios.**

- Two equivalent candidates whose input slices arrived in different orders produce one digest after canonical sorting.
- A source path, normalized mode, selected object, policy, managed executable identity, or structural target identity change changes the bound digest.
- Re-running availability checks or observing a live destination does not itself change the candidate digest.
- A post-trust manager result that would change a version, lock value, artifact identity, structural target inventory, or any digest field is rejected; ZERB does not rewrite and silently re-trust the candidate.
- A symlink, device, nested gitlink, or unsupported mode fails rather than receiving an implementation-dependent hash.

## State-visible migration and recovery policy

### PC-060 — Clean initialization and no old-user migration

**Invariant.** The MVP has no supported-user or old-format migration path, compatibility layer, importer, second state model, or legacy data adoption. It neither creates nor reads a ZERB-specific XDG config root. Durable state and data share one random instance ID; cache is untrusted and disposable. Architecture owns the no-follow ownership/type/mode checks, stable initialization, admission, and writer synchronization inodes, exclusive initialization/takeover claim, atomic writer-owner publication, file and directory synchronization, state/data/cache markers, and process-identity recovery mechanics.

A clean first use exists only when the product state and data directories are independently safe and each is either absent or an empty directory, with no recognizable ZERB synchronization name, marker, claim, domain tag, or durable record in either root. All four absent/empty combinations are eligible. The stable initialization lock serializes initial claim and stale-claim takeover. The current lock holder writes a fresh random ownership token and process identity into the synced initializing claim, creates and syncs the stable admission/writer synchronization inodes and released writer-owner record, and only then may create matching data and per-instance cache markers or mark initialization complete. A losing initializer writes no marker or product data. Unrelated cache content is neither adopted nor deleted.

**Scenarios.**

- Every clean absent/empty state-data combination may initialize through the single-winner protocol.
- Two simultaneous initializers have one lock holder; the loser re-reads a completed marker and continues or reports initialization-in-progress.
- After proving an initializer dead, exactly one lock-holding successor may durably compare-transfer the claim to a new token and continue; possession of the dead token alone grants no authority.
- A reserved synchronization filename, even with empty, malformed, or substituted content or inode, is recognizable evidence rather than a clean or foreign root.
- Existing files from the former product are not silently imported or reinterpreted.
- The MVP provides no cleanup command that recursively deletes a historical user directory.

### PC-061 — Exhaustive foreign, corrupt, initializing, and current-state behavior

**Invariant.** Root classification is disjoint in this first-match order:

1. `CLEAN` is only the exact PC-060 absent-or-empty first-use predicate.
2. `FOREIGN` applies only when `CLEAN` is false and neither state nor data contains a recognizable ZERB synchronization filename, reserved durable path, marker, claim, schema/domain tag, or record. All entries are unrecognized foreign content; ZERB gives generic move-aside/manual-clean-start guidance and writes nothing.
3. `BAD unsupported` applies when any recognizable record or object uses an unsupported schema or domain. This precedes initialization classification even when `init.lock` or an initializing-looking record also exists; current code never promotes, rewrites, or migrates it.
4. `INIT` applies only to one exact crash-valid prefix of ARCH-017: the safe stable `init.lock`; zero or one supported, well-formed `root.json(status=initializing)` bound to that lock holder; then, strictly in publication order without gaps, the same-instance safe `admission.lock`, safe `writer.lock`, released-generation-zero `writer-owner.json`, and matching data `owner.json`. Every present record/inode must match the same supported schema/domain, instance, token, predecessor, and process identity; no initialized/current/candidate/journal/receipt/install evidence and no unrelated, extra, malformed, temporary, or unrecognized state/data entry may exist. ARCH-017 and VAL-025 enumerate the exact before/after-crash prefixes and the sole compare-transfer repair for each.
5. A supported current relation exists only when the complete initialized owner pair, all stable synchronization inodes, writer-owner publication, and every state/data entry validate under ARCH-018/ARCH-024/ARCH-025. Independently valid immutable content-addressed candidate, source, materialized-tree, and verified-toolchain objects may be unreferenced; they remain data, not completion authority, and are reauthenticated on every use. Every install, journal, receipt, owner publication, and current record must participate in the exact permitted relation. The result is exactly `READY(∅)`, `STABLE(A)`, `TRANSITION(A,B)`, or `CLEANUP(B)`.
6. `BAD corrupt` is the fallback for every other namespace containing recognizable ZERB evidence: any required path/record is absent, malformed, partial, mismatched, unsafe, duplicated, aliased, improperly unreferenced, cross-related incorrectly, or accompanied by an unrecognized extra state/data entry. A broken initialized relation is never reclassified as `INIT` merely because the permanent `init.lock` remains.
7. Cache is not durable truth. A missing or poisoned owned per-instance cache is a cache miss; entries outside the validated cache subtree remain unrelated and are neither adopted nor deleted. Operation staging is confined to this cache and cannot create a state/data relation.

`INIT`, `BAD`, `READY(∅)`, `STABLE(A)`, `TRANSITION(A,B)`, and `CLEANUP(B)` receive exactly the command/lease row in PC-016. Initialization repair uses only stable initialization exclusion and token compare-transfer over a listed `INIT` prefix. Writer-owner repair or stale takeover uses ARCH-031. `doctor` never repairs implicitly.

**Scenarios.**

- A non-empty root with no recognizable ZERB evidence exits `1`, writes nothing, and gives generic move-aside guidance without interpreting its content.
- An absent state root paired with a data marker, a missing data marker paired with a current record, a cross-root instance-ID mismatch, a permanent `init.lock` beside broken initialized evidence, and a supported marker pair plus an unknown extra state/data entry are `BAD corrupt`, never `INIT`.
- A valid-looking candidate, source, tree, or toolchain object with an unsupported domain; an orphan pending receipt; a second current record; an unrelated journal; a malformed writer-owner publication; or a substituted synchronization inode prevents current classification. A fully valid unreferenced immutable candidate/source/tree/toolchain object alone does not.
- A journal identifying an interrupted home apply is not relabeled stable and is not told to be deleted.
- An orphan pending receipt never becomes current merely because its file exists.

## MVP support and change limits

### PC-070 — Evidence-gated support state

**Invariant.** The finite Phase 1 matrix in [validation](validation.md) contains non-support proof candidates. A passing Phase 1 harness may mark one positive row `implementation-qualified` or one negative row `exclusion-proven`; neither state supports a private transport surface. An exact distribution/kernel/CPU/filesystem and retained public feature tuple becomes supported only when VAL-001 accepts its complete Phase 6 installed-binary vertical and retained evidence. Detection, source review, mocks, and Phase 1 adapter proofs cannot promote support.

Selected gitlinks, private HTTPS/SSH, credential/helper/agent/relay mechanisms, provider retrieval, provider-backed rendering, and host-fact-dependent templates are excluded from every MVP candidate. Phase 1 proves these exclusions; Phase 3 and Phase 5 runner coverage and Phase 6 installed-binary coverage preserve them. Future private selected transport or provider work belongs only in [post-MVP intent](post-mvp.md) and requires a new reviewed product/architecture contract and evidence matrix before implementation.

**Scenarios.**

- A passing version-detection test or Phase 1 proof does not establish a Linux support tuple.
- A provider reference never invokes its executable or API, and selecting a gitlink never reaches materialization or transport.
- A failed backend or shell proof candidate loses implementation qualification; it is never labeled best-effort or supported.
- Current target commands and schemas are documented as frozen implementation contracts, not as behavior of the reachable old binary.

### PC-071 — Fresh bootstrap and exact-candidate reapply only

**Invariant.** MVP permits fresh bootstrap and exact-candidate reapply only for static, provider-free, host-fact-independent inputs. Reapply is allowed only when source objects, profile ID, declarations, tool set, target set, managed executable and policy identities, and canonical digest remain exact.

Changed sources, candidate digests, profiles, declarations, tools, targets, managed-target sets, adapter versions, or policies are unsupported updates and fail before mutation. Provider-backed or host-fact-dependent candidates are invalid rather than bootstrap-only exceptions. Profile updating, switching, shrinking/removal, source authoring/synchronization, provider retrieval, and abandonment of dirty state are deferred to [post-MVP intent](post-mvp.md).

**Scenarios.**

- Reapplying an unchanged static candidate may converge idempotently after full revalidation.
- Changing only the selected profile is still an unsupported update.
- A provider or host-fact function fails static validation before candidate creation.
- No command silently uninstalls an old tool or deletes an old target to approximate switching.
