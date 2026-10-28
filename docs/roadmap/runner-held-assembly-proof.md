# Held-role assembly proof boundary

This non-normative plan implements the next ARCH-009/010 and VAL-018/019
bootstrap prerequisite. It is not a production protocol, full slice-2 completion,
an armed guard, a development Runner or host qualification. The parent/creator
tree remains F -> M/O/S and O -> G; M creates S with CLONE_PARENT. F remains
the original caller role. Managed entry and strace execution are not released.

## Authority and channels

Every bootstrap FD 3 retains ARCH-009's exact-parent SO_PEERCRED rule and full
same-image comparison. Every packet additionally carries kernel SCM_CREDENTIALS;
the receiver checks actual sender PID/UID from a separately held live identity,
never from the packet's claimed role. Every packet, including ACKs, goes through
recvmsg; ordinary File.Read cannot discard credentials. A child-generated nonce
binds its fixed role and the F/M/O/S/G identity tuple to the expected phase.

The test-private transport is bounded AF_UNIX SOCK_SEQPACKET, SO_PASSCRED enabled,
SO_PASSPIDFD disabled and MSG_CMSG_CLOEXEC. One packet is at most 2048 bytes and
at most four rights; the expected rights count is fixed by the current phase.
Exactly one credentials control is required. Truncated/extra controls, wrong
sender, malformed bounded JSON, wrong phase/nonce/role, and wrong rights count
refuse. Every installed right is owned immediately and closed on refusal; it is
never signalled until validated against the held role. Existing JSON decoded-value
semantics do not qualify duplicate-free production grammar. No generic registry,
new public token, exported API or second binary is introduced.

| Channel | Creator / endpoints | Accepted writers and purpose | Close responsibility |
|---|---|---|---|
| F-M bootstrap | F; F and M | F and actual M only; role setup and S birth/custody report | F closes its child endpoint immediately after M birth; M owns its endpoint until held cancellation/exit |
| F-O bootstrap | F; F and O | F and actual O only; O setup, G birth/custody, backup ACK and status | F closes its child endpoint immediately after O birth; no other role inherits either live endpoint |
| F-S bootstrap | F; child endpoint temporarily passed F -> M -> S | F and actual S only; parent is F despite creator M | F closes its child copy after M birth; M closes its temporary copy after S birth |
| O-G bootstrap | O; O and G | O and actual G only; exact-parent bootstrap and unarmed administrative stop | O closes its child copy after G birth; F never receives the O writer or G-disarm authority |
| F-G custody | F; child endpoint temporarily passed F -> O -> G | F and actual G only; registration, status capability binding and direct unarmed/refusal facts | F closes its child copy after O birth; O closes its temporary copy after G birth; G may consume it only after O-parent authentication |

G's secondary F channel never substitutes for its O-created bootstrap FD 3.
An inherited F pidfd, validated through authenticated parent setup, binds G's
secondary sender to F. An O relay is never described as a message sent by G.
No backup copy of a death/disarm writer may suppress EOF or create disarm power.
F's held G pidfd still permits fixture termination; that action latches failure,
never protocol disarm or guard-cleanup success. Borrowed startup descriptors are
untrusted until validated. Restore CLOEXEC on inherited slots above 3 before any
subsequent birth, especially M's temporary S and O's temporary F-G endpoints;
ancillary rights are atomically close-on-exec.

## Startup and retained capabilities

F captures an opened self image and its own pidfd before births. F/O/M/S/G
execute that same open self identity; full content/metadata and live pidfd identity
checks apply. Readiness may refuse terminal roles but is not status/reaping proof.
M's locked release thread keeps its direct SIGKILL relation to F. S intentionally
survives with zero parent-death signal, SID=M and PGID=S. O uses a fresh session;
G stays outside M's session/group and retains the O-parent guard-stage relation.
F is an explicit backup subreaper for this disposable, live-F test only.

| Role | Bootstrap inherited slots before validation | Retained pidfd custody after registration |
|---|---|---|
| F | Original caller context | F self; birth M/O; validated creator-transferred S/G copies |
| M | 3 F control, 4 self, 5 temporary S endpoint, 6 F pidfd | F identity; creator S copy only until F and O acknowledge backup custody |
| O | 3 F control, 4 self, 5 temporary F-G endpoint, 6 F pidfd | F identity; its original G; validated M/S copies |
| S | 3 F control, 4 self, 5 F pidfd | F identity and validated M identity for the future observer boundary |
| G | 3 O control, 4 self, 5 F custody, 6 F pidfd, 7 O self pidfd | F/O identities and validated M group pidfd; explicitly no cgroup or arm authority |

O opens its own pidfd before G birth, after O's authenticated parent setup,
checks its own identity/liveness, passes that capability to G at slot 7 and closes
its temporary original after G inherits it. G validates it against its actual O
parent and retains it for every subsequent parent-channel sender check. G's
F capability is separately bound through the authenticated O-parent setup.

Actual parent, creator, session, group, UID, image and descriptor identities are
checked against this fixed table. Internal non-close-on-exec descriptors are
limited to standard streams and bootstrap control; declared extra capabilities
are validated and close-on-exec. Parent-side or stale creator copies close at
their named handoff. F/O retain S/G copies before creators may retire custody.

Initial authentication binds only known births: it cannot assert the as-yet
unknown S/G identities. This partial birth scope and its per-channel nonce are
distinct from final registration; zero, unknown or repeated role identities can
never satisfy the final complete-tuple validation. G's O-parent and secondary-F
channels use separate challenges and expected phases.

F authenticates M/O first. M births S and sends the atomic S capability; O births
G, authenticates its O-created bootstrap, and sends its atomic G capability.
F validates both before accepting direct S/G messages. The complete tuple is then
bound to every role's nonce. O acknowledges M/S backup copies; S/G acknowledge
their M capabilities. Only after every role's registration may M retire its S
creator copy. G reports unarmed; no role may interpret that as guard readiness.

## Completion and failure matrix

The positive result is a complete **held registration**, not operational readiness.
Attempted arming without a real cgroup capability must refuse and be observed
directly from G; managed release remains disabled. Normal disposal has F consume
M/S, O consume G before reporting stored facts, then F consume O. Raw unarmed G
exit zero is only that process's status, never guard-cleanup success.

Required integrated negative controls are: a foreign real sender with otherwise
matching role/nonce and rights; a wrong bootstrap-parent route; a wrong/replayed
nonce or phase; substituted custody capabilities; interrupted child birth/transfer;
O loss during held assembly; and inherited-descriptor sanitation. Existing scalar,
rights-framing and stored-status regressions remain in the combined gate.
Bad registrations cannot reset into success. Actual wait facts and held pidfd EXIT
are retained separately; missing evidence remains failure. Known statuses do not
erase failed registration. Unknown/rejected rights are close-only. Cleanup uses
only birth-held or already validated capabilities, closes exact owned channels,
and drains only this disposable invocation's children. No numeric signal fallback.

F loss, real external reaping, complete concurrent descriptor/death/exec races,
sealed-target substitution, native descendants, real guard/cgroup/namespace/resource
closure, strace activation, physical terminal, actual artifact inventory/fit and
frozen-host/installed acceptance remain separate gates. No host/security change,
mount, service, dependency or production activation follows from this plan.

## Implemented local checkpoint

The source is `internal/proof/lifecycle/held_assembly_linux_test.go` plus its
credential-bearing transport in `assembly_packet_linux_test.go`. The integrated
matrix has eleven rows: held registration; foreign actual writer; wrong G parent
route; wrong nonce; replayed phase; swapped backup capabilities; interrupted S/G
transfer; O loss; a queued O ACK after O becomes terminal but unconsumed; and
inherited-marker sanitation. Refusal senders remain held until revocation so an
accepted packet never claims an exited sender is a live role.

F and O set and read back backup/primary subreaper attributes. G installs and
revalidates the existing O-parent SIGUSR1 stage after parent authentication;
it intentionally ignores that signal so it can survive, without claiming real
guard cleanup. Cleanup checks retained EXIT for every validated owned capability,
including G consumed by O, separately from F's actual wait records. Interrupted
untransferred roles remain failed/unresolved registration, not invented custody.

Eleven integrated rows and twenty-five packet cases passed ten repetitions with
race and coverage instrumentation (110 integrated cases and 250 packet checks).
A retained JSON run records zero skips or failures. Scoped lint reports zero
issues. Independent Sol security/design/source review has no remaining findings;
the reviewer inspected source/evidence without rerunning tests. Full clean-source
conformance and exact-head CI are separate subsequent observations. This records
the held bootstrap subset only, never an armed or complete production Runner.

The first full conformance attempt at source `75af604` failed its proof-race
stage (exit 1); the other nine stages passed and source binding stayed clean.
That collector discarded detailed diagnostics, so the failing test and cause
remain unknown. The exact proof-race command subsequently passed with retained
private diagnostics, and twenty additional covered-race repetitions of all eleven
integrated rows passed. Neither observation explains the initial failure or proves
it unrelated. Preserve that unresolved result alongside any fresh gate/CI result;
there is no claimed fix or general stability/qualification conclusion.

## Failure diagnostic retention

The collector now runs its three Go test stages with `-json`. Before execution,
its existing measured Git source scan copies Go/module bytes from the same
identity-checked open/read/hash operation. A frozen catalog admits only those
package/test identifiers and source locations; tests cannot enlarge it by creating
new source files. Capture is bounded by 4,096 files, 16 MiB total, 1 MiB per file,
1,024-byte source paths and 256-byte test identifiers. These are diagnostic
envelopes, not production codec or whole-Runner memory claims. The current fixture
measures 370 source/module files and approximately 4.04 MB.

For an output `report.json`, the command exclusively reserves an owner-only
`report.json.checks.jsonl` beside it, syncing the directory. Before any test stage
it persists a start record; before the next stage it flushes/fsyncs that stage's
exit/unknown status and safe failure observations. Source binding precedes checks
and collection completion records after-source stability. An interrupted start
without completion remains unresolved. An unavailable checkpoint fails collection;
existing output/journals are never overwritten or repaired. Raw output still dies
with its disposable temporary log and never enters the journal/report.

Failure evidence retains observed repo-relative file/line locations, catalogued
top-level tests, collector arrival ordinals, and closed diagnostic role events.
Dynamic subtest suffixes, messages, nonces, PIDs, descriptors, arbitrary paths and
output hashes are omitted. Locations identify source assertions without copying
their formatted values; they do not authenticate an assertion's execution.
Held assembly emits actual Wait4/retained EXIT facts, separately from expected
statuses, plus send/receive/birth/refusal events. `harness` and `fixture` are
diagnostic actors, not authenticated F/O/M/S/G roles. Child output streams to the
outer test capture, avoiding the former 8 KiB prefix truncation. Diagnostic-write
failure retains rejection/cleanup behavior, including untransferred capabilities.

The reducer accepts at most 64 KiB per line and keeps a 4,096-event tail, separate
bounded source locations and failure synopses (last eight source-location
ordinals and sixteen typed observations). Overflow, malformed/unknown records,
missing failure locations or unsupported output are explicitly incomplete. Early
failure locations survive later pass traffic. Producer-local sequence and capture
arrival do not establish a global causal order; uninstrumented subprocesses have
no newly claimed wait/status coverage. These diagnostics grant no authority.

Intentional no-role failures verify ordinary and late assertion retention, actual
driver/child statuses and secret-free structured output. They are regression
fixtures, not successful lifecycle/qualification rows. Nothing in this change
explains or erases the historical `75af604` failure.
