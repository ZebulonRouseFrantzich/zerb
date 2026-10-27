# Direct original-caller parenthood: bounded design and proof candidate

This is a non-normative proposal under the owner-approved design/proof phase.
It does not replace ARCH-009/010, PC-015, VAL-018/019/020, activate a Runner, or
qualify a platform. The repository's existing guarantees remain requirements.
One distributed ZERB image and the original shell-launched invocation PID are
preserved. The proposal is incomplete until every gate below closes.

## Process identities and authority

F is the original public invocation, never a renamed hidden child. F remains the
natural parent of an unreleased same-image launcher M, which later becomes the
managed leader without changing PID. A retained, locked F creator thread owns
that birth until the invocation closes. M installs/rechecks SIGKILL parent-death
behavior against that actual F relation before it can release any managed image.
Premature creator-thread exit is failure even if other F threads remain alive.
Ordinary exec must preserve the relation; unsupported credential transitions,
privileged executable formats and unproved changes refuse before release.

F separately creates O, the surviving invocation cleanup/status owner. O creates
S, the tracing process, and the independent cleanup guard G under an explicitly
registered authority arrangement. S is M's sole tracer; F does not trace M.
The candidate observation tuple is full-syscall strace attachment to the held
same-image launcher before its target exec. The current proof tuple that starts
the manager beneath strace is not silently treated as the same tuple.

Nonancestor attachment must work under existing kernel/LSM/seccomp/dumpability
policy. No PR_SET_PTRACER exception, capability grant, privilege change or host
policy repair is authorized. Attachment failure or inability to establish exact
tracer identity refuses release. `strace -D` is not an approved substitute: the
reviewed strace 6.13 launch path calls PR_SET_PTRACER_ANY and changes helper births.

## Release is a conjunction, not a message

The production candidate would release only after all of these are established:

- Full opened-image and exact role/parent/peer identity, authenticated stage and
  invocation binding, and required descriptor/environment/snapshot checks.
- Actual M/F creator-thread parent-death relation and the immediate parent
  recheck, with no substitute watcher or last liveness check used as a fence.
- An admitted group/session topology and placement in the exact descriptor-owned
  bounded cgroup before managed execution; no claim that the old formula
  `launcher session ID == immediate parent's PID` fits ordinary F unchanged.
- Completed role-birth registration, precommitted status capabilities and fallback
  custody, authenticated tracer attachment, and an armed independent death guard.
- Required observation armed and identity-verified, plus exact resource
  reservation for every role,
  including observer/guard processes that must survive managed-cgroup killing.

No target token is sent before that conjunction. Before the conjunction, loss of
the sole F-owned release writer must refuse without candidate execution. A queued
token is not an independent post-death authorization. The direct death mechanism
and final exec ordering still require the composed proof; ordered marker tests do
not establish all concurrent execution/death interleavings.
Complete required trace evidence is checked before successful completion, not
required to exist before the observed execution begins.

## Status observation is distinct from reaping

Each registered role needs an exact identity-bound pidfd held before its release.
Capabilities and registration must survive the specified single-custodian loss:
O-only handles are insufficient when O dies. F's live backup custody and O's
F-death custody must be explicit; copying PID numbers or rediscovering by pathname
does not create them. Role birth interruption remains an unresolved failure until
its exact child/resource disposition is established.

For a qualifying kernel, PIDFD_GET_INFO with PIDFD_INFO_EXIT can expose stored raw
exit information after actual task reaping. Require the returned EXIT mask bit;
missing information, readiness, terminal death, public status zero or cgroup
emptiness never imply an exit code or successful reap. Retain exit/signal/core
distinctions. In particular exit 137 and SIGKILL are not interchangeable.

The real wait owner must be allowed to reap before another role waits for stored
EXIT information. F must not wait for O's final result before reaping M. O may
consume its own S/G wait only after the kernel-custody prerequisites are valid,
then independently recognize stored EXIT. The existing acknowledgement-before-
consumption contract requires a reviewed superseding relation; this proposal does
not claim it was already replaced by a pidfd getter.

When F lives, F naturally reaps M. After F dies, M is reparented to the nearest
eligible living ancestor subreaper or namespace init, not necessarily promptly to
system init. O is M's sibling and cannot compel that external process to reap.
If reaping is delayed, O may have exact terminal-death evidence but no stored EXIT
information. That is unresolved operational failure, never clean completion.
No production timeout, external-reaper cooperation guarantee or stronger launch
prerequisite is frozen by this document. The disposable fixture's R explicitly
cooperates as the natural reaper; its existence is not production evidence.

Every required role needs separately established exact exit facts. S's exit zero
in attach-only mode does not mean M exited zero. A valid M pidfd does not register
fast manager-created children, and strace's ordinary trace log is not an
authenticated birth/custody acknowledgement. Whole-tree registration and complete
leader/descendant reaping remain open gates, not waived obligations.

## Guard and group authority

G must survive the death that triggers its cleanup and retain independently
validated descriptor-relative cgroup authority. Guard zero is required actual raw
status plus the required cleanup observations; an empty cgroup alone cannot
produce a zero guard result. A missing or failed guard status remains failure.
The role/control channels must not let candidate execution disarm or impersonate
the guard. Multi-custodian loss cannot manufacture a retained status record.

An externally reaped M no longer supplies the fixture's retained-zombie numeric
process-group pin. A pidfd is a stable task identity, not sufficient authority to
signal a recycled numeric group. The production design must prove a retained
owned-group relation or an explicitly reviewed different kill sequence. Never
signal F's inherited shell foreground group. No cgroup-only fallback, numeric
group guess or old mandatory group-kill contract change is approved here.

## Required capability and failure matrix

| Boundary / fault | Required disposition | Present evidence |
|---|---|---|
| pidfd open, ioctl or returned EXIT capability unavailable | Refuse; never substitute zero or weaker status | Local getter works; negative production admission unimplemented |
| Separate strace attachment denied or wrong tracer | No release; preserve unresolved role facts; no policy change | Exact local nonancestor attach observed |
| F dies before attachment / registration / release | No managed image; truthful interrupted birth accounting | Earlier sole-writer EOF subset; composed early matrix open |
| Creating F thread exits while process survives | Fail invocation; managed release cannot continue | Earlier direct-tracer thread subset only; direct-parent combined thread row open |
| F dies at held post-attachment checkpoint | Direct M death; S remains sole tracer; preserve raw status after actual reap | Combined local ordered checkpoint |
| External reaper delays consuming M | Missing EXIT remains unresolved; no success or reaping claim | Combined local delayed-reaper row |
| F waits for O before reaping M | Reject sequencing; avoid status/reaping deadlock | Design review only |
| O dies before/after role registration or status observation | F backup custody or explicit unresolved failure; preserve containment | Open |
| F and O unavailable during cleanup | No fabricated custody or guard success; defined fault disposition | Open |
| S dies or trace becomes incomplete | Abort candidate path, cleanup and latch observation failure | Open under composed topology |
| S observes a fast fork/clone/vfork/exec/exit | Exact child registration/status/cleanup without lost birth interval | Open; stock strace output alone insufficient |
| G missing, signaled, nonzero or status unavailable | Operational failure even if later cgroup empty | Open under composed topology |
| Group leader reaped / group ID reused | No unproved numeric group signal | Open; requires retained authority design |
| Final target identity, parent or descriptor changes | Refuse candidate release; preserve exact status/evidence | Existing separate checks; new topology integration open |
| Concurrent F death versus last target-resume / exec return | Exact declared linearization and negative controls; no global marker inference | Open |
| Unknown role, extra descriptor, namespace or cgroup mismatch | Refuse without recovering authority by guessed PID/path | Open |
| Resource exhaustion or partial birth | Bounded refusal retaining known ownership and unresolved facts | Open; no whole-ledger fit claim |
| Terminal/session foreground or caller authority lost | Preserve caller-only physical terminal policy and refuse unsupported mode | Open; no PTY proof in these fixtures |

## Evidence boundary and disposition

The local combined fixture is Python/ctypes plus the existing strace executable,
not an installed ZERB binary or Go runtime fork implementation. It uses inherited
fixture handles, a test driver/subreaper and a synthetic payload. It establishes
only actual parent-death, separate tracing, ordered status handoff and delayed
external-reaping observations. It has no authenticated production dispatch, real
death guard/cgroup, full tree or complete resource protocol. No literal zero-
instruction global race claim follows from marker absence.

The local Linux 6.18.55 x86_64 / strace 6.13 subset ran 30 repetitions of each
of three cases: live target entry and exit 17; original-F death at the held
checkpoint with M SIGKILL and no payload marker; and delayed external reaping.
In the delayed case, actual WNOWAIT terminal death preceded a missing EXIT bit;
only after the fixture reaper consumed M did O observe stored raw SIGKILL.
The live trace includes the payload exec, its marker and exit syscall. These
observations do not prove refusal on a different kernel/LSM policy or cgroup,
guard, descendant, resource, terminal, authentication or production dispatch
behavior. The raw zero S result is explicitly required, separately from M's
pidfd-derived result; attach-only strace status is not target status.

Required frozen-host capability checks, physical terminal, mount, namespace,
storage, actual manager classes and installed acceptance remain unexecuted. The
existing source-built artifact inventory and resource-fit gates remain unchanged.
Production Runner/Dispatch activation, Phase 2 completion and support stay false.

The next contract decision, if review cannot close it within the approved scope,
must consolidate external-reaper liveness, numeric-group authority and complete
role/descendant custody. Do not patch one gap by silently weakening another.
