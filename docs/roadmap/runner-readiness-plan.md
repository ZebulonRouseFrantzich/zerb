# Direct-parent Runner: implementation-readiness plan

This is a conditional plan for the approved three-contract design/proof phase,
not authorization to enable production execution. ARCH-009/010 and VAL-019 now
own those provisional proof directions. The existing
non-normative [candidate](runner-direct-parent-design.md) and its open gates apply.
These steps concern Runner readiness; they do not finish the product MVP.

## Capability admission: test behavior, not version strings

The exact frozen host remains VAL-001's unqualified Fedora/kernel/x86_64/btrfs
candidate. The cloud observations below use Linux 6.18.55 x86_64 and strace 6.13.
Backports, configuration, LSM, seccomp and namespace policy can change results.
The following probes must pass under the actual invocation's security context:

| Capability | Exact required probe / refusal |
|---|---|
| Identity-bound pidfd | Open before role release, verify CLOEXEC, exact process/namespace relation, duplicate/close ownership and stale-object behavior. Open failure grants no substitute PID authority. |
| Stored raw exit information | PIDFD_GET_INFO requesting PIDFD_INFO_EXIT; require returned EXIT bit after actual parent consumption for exit 0/11/137 and SIGKILL. Require bit absence during the tested unreaped interval. ENOTTY/EINVAL, unavailable fields or identity ambiguity refuse. |
| Group-addressed pidfd signalling | pidfd_send_signal with PIDFD_SIGNAL_PROCESS_GROUP, null siginfo, validated session/group leader. Signal a disposable member after leader reaping; verify exact member effect, unrelated control group unaffected, nonleader refusal and empty-old-group ESRCH. No numeric fallback. |
| Direct caller-death relation | Actual parent/creator-thread identity; child PR_SET/GET_PDEATHSIG SIGKILL and immediate recheck; original-caller death before held continuation, thread exit, ordinary exec preservation and rejection of unsupported credential transitions. |
| Separate full tracing | Exact identity-checked strace attaches before target release under existing policy, including required threads. Verify exact tracer and unchanged SID, complete declared trace coverage, and refusal on attachment loss/denial. No PR_SET_PTRACER or privilege repair. |
| Wait and adoption | Actual WNOWAIT versus consumption, nonparent ECHILD, live-F reaping, owner-loss fallback and the real external-reaper context. Readiness or cgroup emptiness is not consumption. |
| Containment and resources | Exact descriptor-relative cgroup boundaries, current memory/swap/process/CPU limits, namespaces, seccomp, guard authority and whole-role resource ledger. These require real qualified-host evidence; ordinary-file stand-ins cannot pass them. |

Upstream PIDFD_GET_INFO exists in the reviewed v6.13/v6.14 UAPI without EXIT;
the reviewed v6.15 UAPI adds PIDFD_INFO_EXIT and exit_code. Group scope is documented
since Linux 6.9. Consequently an ioctl's existence, or a nominal kernel >=6.15,
does not admit execution. Probe the exact required behavior and returned bits.
No kernel installation, upgrade or policy change is part of this plan.

## Proposed process and descriptor ownership

F remains the original shell-launched PID. Its retained creator thread directly
parents M. M is the authenticated, held same-image launcher, then the exact
managed image. F owns the sole release writer; M owns only its receiving end.
An unknown, incompletely registered or unauthenticated role cannot receive release.

O survives F and holds acknowledged, identity-bound status capabilities and
cleanup authority. F retains live backup copies before acknowledging transfer.
G independently holds the preheld M group pidfd and validated cgroup/control
authority; its actual raw zero
status is required in addition to cleanup facts. All F/O/G writer sets and close
responsibilities must be explicit; a stray writer must not suppress death EOF.

The approved bounded bootstrap direction has M create S after entering its
fresh SID=M and before entering the managed kill cgroup. S inherits SID=M, takes
its own process group, and becomes the exact plain strace observer. Thus S's own
session membership reserves M's numeric identity while S can execute `-p M`.
G can remain O's child, outside managed group/cgroup killing.

The reviewed birth comparison selected CLONE_PARENT for proof, with the ordinary
fork alternative retained below only to explain its native-wait risk:

- Ordinary fork leaves S as M's native child across exec. A valid payload that
  waits for all children can then wait for S while S waits for the payload. This
  user-visible wait-semantics risk cannot be dismissed by checking only Git.
- A CLONE_PARENT birth from M would retain the inherited session but make F the
  natural parent of S. This is the owner-approved proof candidate for preserving
  native child/wait semantics; actual Go/kernel and complete protocol proofs
  remain required before production activation.
  It adds explicit creator-M versus natural-parent-F registration/authentication
  and interrupted-birth obligations; Go API symbols alone do not qualify it.
  Go 1.24's nonzero SysProcAttr.Pdeathsig path checks against creator M, so S needs
  an explicitly reviewed zero/survival stage and separate parent-F authentication.
  Its clone3 path supplies SIGCHLD as exit_signal, which the reviewed Linux 6.18
  clone3 validation rejects with CLONE_PARENT. The exact legacy-clone/atomic-pidfd
  path and outer-cgroup inheritance therefore need actual proof; UseCgroupFD or
  another clone3-selecting option cannot be assumed to work. Failure refuses.

The proposed natural-parent tree for that second choice is F->M, F->S and F->O->G;
the creation operation M=>S is distinct from the parent relationship. F remains
the original invocation. S's exact tuple must not leave SID=M, and observed
unexpected parenthood during bootstrap/authentication must refuse. This is not
a global no-attachment-after-F-death claim: S's own session reservation must keep
the lookup owned even after F dies following a valid check. Neither old
O->S semantics nor the new birth edge may be silently inferred. Unknown/partial
birth is failure.

Before release, F/O need acknowledged S pidfd ownership as well as M/G custody.
S's standard/control/trace descriptors must be explicit and bounded; S may not
inherit physical-terminal/master authority or undeclared protected-state handles.
At the managed exec boundary M still receives only the allowed target streams.
The current fixture's inherited pipes are not this authenticated FD protocol.

## Ordered proof gates

1. **Group and attachment authority.** Prove pinned group signalling through all
   leader/reap/empty transitions; close session membership and no-unowned numeric
   strace attachment at every M/F/G/S death boundary. A preheld pidfd alone does
   not reserve a numeric PID for strace's `-p` lookup.
2. **Whole-tree ownership scope.** Register each ZERB control role before release
   with exact identity, capabilities, descriptors and fallback custody. For native
   manager children, independently prove complete trace/native-wait/containment
   evidence. Stock strace output cannot retroactively mint a before-reap pidfd.
   The approved relation still needs complete protocol/evidence proof without
   dropping any child, status or actual-reaping fact; missing evidence is failure.
3. **Reaping, guard and resource closure.** Prove external-reaper disposition,
   deadlock-free waits, exact guard zero/nonzero/signaled/missing rows, O/F loss,
   bounded partial birth and complete resource/cleanup ownership. Keep G/S out of
   the subtree they must observe or clean. Do not invent an increased budget.
4. **Composed race matrix.** Authenticated FD-only dispatch, thread lifetime, all
   descriptor churn cells, final opened/sealed image identity, death before/at
   release, signal/context arbitration and the existing shared 750 ms grace.
   State the death/exec ordering precisely; race repetition is not a proof of all
   interleavings. Unknown status cannot become success after an empty cgroup.
5. **Actual classes and host acceptance.** Exercise every retained exact class,
   then PTY/job groups and actual frozen-host cgroup/namespace/btrfs tests. The
   installed acceptance gate follows full Runner/application integration and
   the later cutover; it is not a prerequisite to implementing those seams.
   Pinned artifact build/inventory/fit remains required;
   the missing authenticated Nix-built artifact cannot be replaced by a mock.

## External reaper: qualification and completion

Qualification must run disposable crash/reaping tests in the actual supported
launch context, including its PID namespace and ancestor reaper arrangement.
An inserted test-driver subreaper masks that context and cannot qualify it. Inspect
available namespace/ancestor identities, then observe actual orphan adoption and
stored EXIT only after consumption; do not assume arbitrary ancestors' subreaper
flags are readable from procfs. Include a deliberately delayed consumer as a
negative control, alongside normal completion and controlled failures.

Such tests establish observations for that exact context, not a universal future
reaping deadline. O cannot force an external ancestor to wait. Missing EXIT must
remain pending/unresolved, never zero; pre-release unsupported capability/context
must refuse, and a runtime loss cannot publish clean completion or a completed
receipt. Do not reuse the 750 ms signal grace as a reap timeout. Exact reporting
and durable evidence routing belong to later app/state integration.

A successful completion promise under arbitrary shell/container ancestry is not
approved. The owner approved explicit qualification of supported launch/reaper
contexts plus truthful unresolved failure outside the established relation. This
still needs actual qualification and protocol review; it does not prove an
ability to force reaping or authorize a host repair recipe.
No operation on the owner's host is authorized by this checklist.

## Smallest linked implementation PRs after those contracts close

| Slice | Bounded deliverable / done condition | Depends on |
|---|---|---|
| 1. Private kernel capabilities | Exact pidfd status/group operations, owned lifetimes and refusal regressions with no public Runner activation or fallback | Gates 1-2 design closure and reviewed capability fit |
| 2. Authenticated role assembly | F/O/M/S/G birth, session/FD ownership, acknowledged fallback custody and bounded partial-birth cleanup, exercised through private proof seams | Slice 1; agreed S bootstrap/evidence relation |
| 3. Guarded noninteractive lifecycle | Real cgroup/guard/status ownership, cancellation/death matrix and complete reaping/refusal behavior for synthetic sealed ELF fixtures | Slice 2; Gate 3; no incomplete public Runner |
| 4. Retained class integration | Exact source/toolchain/manager tuples, full tracing and installed identities; class failures remain refusal | Slice 3; authenticated build/inventory/fit and required host evidence |
| 5. Interactive/full Runner integration | Physical-terminal ownership, PTY/job groups, full method behavior and final runner review before non-public application integration | Slice 4 and required lifecycle/PTY proofs; installed acceptance follows integration/cutover |

These are conditional proposed slices, not new implementation authorization.
Use separate linked review PRs, preserve existing stacked work, and keep the public
router unchanged until its independent cutover gate. No stage is Phase 2/MVP or
release completion merely because a build or a narrow proof passes.

### Reconciliation of the current proof stack

This table records bounded implementation-readiness evidence, not phase completion
or a newly frozen production protocol. It does not reorder the gates above.

| Planned slice | Current evidence | Concrete unfinished boundary |
|---|---|---|
| 1. Private kernel capabilities | PR #45: Go observer birth, stored-status and tracing subsets. PR #46: retained group-pidfd authority through actual leader consumption, refusal and isolation controls. | Complete group/session/attachment races, capability admission/fit and their private production foundation remain unclosed. Neither PR completes slice 1. |
| 2. Authenticated role assembly | The registered-backup fixture adds actual pre-release M-pidfd transfer, bound role/value checks, F's retained birth capability, O-loss boundaries, and close-only rights refusal. This differs from the earlier post-exit G-status acknowledgement fixture. | Full F/O/M/S/G registration, mutual role authentication, complete descriptor ownership/churn, fallback custody under all required losses and bounded partial-birth assembly remain open. The test-private wire is not a production registry. |
| 3. Guarded noninteractive lifecycle | Separate timing, custody, direct-parent and owned-group prerequisites have local evidence. | Real independent guard/cgroup/namespace authority, actual reaper context, whole-tree resource/cleanup proof and composed cancellation/death/release matrix remain open. |
| 4. Retained class integration | No new manager or acquisition execution is introduced by this proof stack. | Prior lifecycle gates plus the actual authenticated pinned Nix build, runtime/license inventory and representation fit remain required. Synthetic inventories cannot replace them. |
| 5. Interactive/full Runner | Approved caller-only physical-terminal direction remains unchanged. | Full PTY/job-control, terminal authority/loss, complete Runner methods and required host/installed acceptance remain unproved. |

The registered-backup subset keeps F alive as M/O's natural wait owner; it cannot
qualify the unresolved external-reaper relation after actual F death. Socketpair
credentials authenticate F to its children. F's held birth, exclusive inherited
endpoint custody and full same-image observations provide limited provenance,
not mutual F-side kernel sender authentication. The existing proof JSON decoder
checks decoded values, unknown fields and trailing values; this subset does not
qualify duplicate-free production grammar. Readiness can reject a terminal role,
but neither readiness nor a missing stored EXIT establishes raw status or reaping.
Ordered fault checkpoints are not a concurrent death/release linearization proof.

## New local evidence and remaining decision

Thirty group-pidfd rows observed delivery after leader reaping, SIGKILL/reap of
the remaining member, ESRCH for the empty old group and no effect on a separate
control group. Source inspection supports kernel-object addressing; forced numeric
PID reuse was not tested. An injected unread-birth failure left no adopted child.

The repository Go regression `TestPidfdGroupAuthority` now repeats the narrower
owned-group transition with atomic birth pidfds, explicit session/group checks,
observable member delivery before and after the leader's actual consuming wait,
nonleader/conflicting-scope refusal, empty-old-group ESRCH, and an independently
responsive control whose exact zero status is required. A separate individual
SIGTERM row verifies that control would fail on an unintended signal. Interrupted
member registration proves sole-endpoint EOF disposition and actual child waits;
the fixture-local socket-shutdown regression covers pending-reader revocation.
CLONE_PARENT makes the cooperating test driver the member's natural parent; this
does not qualify orphan adoption or an actual external reaper. These regressions
do not close the remaining strace/session, numeric-reuse, complete protocol,
containment/resource or global death/exec race gates.

Thirty fast-child rows retained a full trace of child exit 11 and its parent's
actual reap, while a later pidfd_open returned ESRCH. This disproves post-hoc
capability registration as a general solution; it does not disprove trace-based
descendant observation or grant any late numeric PID authority.

The S-in-M-session CLONE_PARENT bootstrap, control-role versus native-descendant
custody/evidence relation and qualified external-reaper/refusal direction are
approved for bounded proof. The pidfd group operation remains a candidate safe
implementation of group signalling subject to exact runtime qualification and
independent review. Unclosed protocol/qualification gates remain in the owning
contracts. No new package, access grant or production activation follows.
