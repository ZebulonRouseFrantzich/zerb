# Selected committed-control correspondence

Status: owner-authorized non-release foundation, subordinate to PC-042/050/061
and ARCH-018/024/025/026/027/030/031. Independent semantic and concrete fit review
preceded implementation. This is not a whole-store graph classifier or an
acquisition, filesystem, source/install, liveness, completion or cleanup capability.

## Complete selected boundary

The private input contains scalar Current and Writer values plus fixed canonical
wire slots: current candidate B, optional distinct prior candidate A, current
receipt RB, optional distinct prior receipt RA and an optional journal. Existing
strict scalar codecs own canonical Current/Writer wire verification; this seam
also checks their scalar validity, instance equality and completed-current writer
relation. A future filesystem caller must separately prove those bytes, initialized
owner pair, complete namespace, retained inodes/lease and actual source/install
facts. Nil alone represents an absent selected object; an empty nonnil wire is
malformed, and a failed read must never be converted to absence.

Every supplied large wire passes the existing canonical codec. Candidate hashes
are computed from canonical bytes; pending envelopes authenticate their facts
hashes. Actual digests bind Current and the independently named prior roles.
Aliases reuse the current object; duplicate prior-slot wires for an aliased role
are refused as an incorrect selected input, never classified as namespace
corruption. This boundary neither inventories nor ignores permitted immutable
history outside its fixed roles.

| Situation | Necessary committed-record correspondence |
|---|---|
| Current without journal | Resolve B/RB, exact candidate/static receipt links, necessary completed facts and `currentMatchesWriter`; prior selected slots absent |
| Current with journal | Held writer; same generation/token/predecessor, or exact journal-ahead successor with predecessor equal to old owner token; Current must still match retained Writer |
| Current/journal identity | Exact operation ID/token and ARCH-024 same/immediate/older writer-token table; digest equality alone proves no commit |
| Postcommit pending-receipt journal | All distinct prior objects present; no deletion before cleanup-phase publication |
| Cleanup journal | Exact candidate-first then receipt-second residue prefixes; reused current objects always retained |
| Prior candidate removed, prior receipt retained | Canonical prior receipt digest and `Facts.Candidate == journal.Previous`; no demand for the deliberately deleted candidate |
| Exact reapply | Prior candidate aliases B; prior receipt may be distinct or alias RB, retaining exact facts and independent historical `Previous` |

The current receipt binds exact source, components/files, parser, policy, targets,
Git/mise/chezmoi identities and one-to-one observed tool ID/requested/installed
version/archive-lock integrity to its candidate. Candidate Bash/HomeSources have
no receipt counterpart; receipt ZERB/platform have no candidate counterpart. They
remain bound by their own record identities and require their separately owned
runtime verification. Static correspondence proves none of those external facts.

Current and retained prior-completed receipts require both completed phases and
the four necessary true dimensions; availability remains independent. The prior
receipt's own `Previous` is historical and is not reconstructed or recursively
walked. When prior candidate A and receipt RA are both present, RA must match all A
static facts; when A aliases B, use B for that comparison. A reused candidate
remains after a distinct prior receipt has been removed.

A journal must be at pending-receipt or cleanup, cover B's exact targets, and
match RB's prior candidate and preconditions/results. Journal execution order
`[tool,home]` and canonical receipt fact order `[home,tool]` each retain their
own required checks. The validator does not compare the new operation's live
observations to prior RA. Earlier phases, currentless graphs and pending-only
false/incomplete receipts are outside this committed boundary; refusal is not a
corruption verdict. General A/B value correspondence does not authorize a PC-071
update or widen the bootstrap/exact-reapply public surface.

## Concrete logical reservation

The pinned Go 1.24.9 x86_64 prototype measured these actual type layouts before
production coding. Permanent tests bind the implementation types:

| Fixed storage | Bytes |
|---|---:|
| Input | 392 |
| Two candidates, two pending receipts, journal and two candidate-digest headers | 3,016 |
| Returned links | 104 |
| Accounting counters | 40 |
| Wire-preflight result | 16 |
| Codec plan / cursor / largest preflight model | 304 / 40 / 888 |
| Maximum validated Current + Writer string payload | 405 |

The outer allowance is the reviewed conservative sum
`2*392 + 3016 + 2*104 + 2*40 + 2*16 + 3968 + 405 = 8,493`.
The 3,968-byte serial preflight/comparison block is separately charged; it uses
the existing largest codec fixed-control allowance and fits actual fixed work.
Its explicit preflight maximum is 1,760 bytes: model 888, plan 304, cursor 40,
fit scalars 24, wire/kind arguments 32, additional reserve-callee input copy 392,
counters 40, two preflight-result copies 32 and codec budget 8. Scalar checking
and the existing 848-byte completed-facts predicate copy also fit this block;
static comparisons take pointers and allocate no input-dependent workspace.

For each of the five selected wires, length is at most the existing 8,000,000-byte
codec bound. Charge all visible slice capacities, including empty nonnil views;
allow their checked sum up to `5*8,000,000 = 40,000,000`. Overlapping views are
charged independently, not deduplicated by pointer. This is logical visible
capacity accounting; it cannot observe hidden prefix/suffix backing retained by
a capped subslice and is not a physical heap/RSS guarantee.

Before materialization, reuse one allocation-free cursor/plan/model slot to check
canonical grammar and the existing minimum-capacity canonicalizer gate. Preserve
schema refusals (candidate string count, URI bound and pending framing) separately
from reservation exhaustion. Then reserve each complete decoding schedule:

`D = control + Q + wire + decoded strings + exact array backing`

Control is 2,048 for candidate/journal and 3,968 for pending; journal has no source
scratch, others retain the existing Q. Each D is at most 8,000,000; retain their
checked sum, at most 40,000,000, throughout serial existing borrowed decoders,
all comparisons and result construction. The conservative selected invocation
maximum is therefore `40,000,000 + 40,000,000 + 8,493 = 80,008,493` bytes.
This is derived from five closed roles and their existing schedules. It changes
no individual codec budget or accepted canonical value, and freezes no Store,
history, filesystem, publication or host envelope. Earlier codec success is
never used to reset the retained-result accounting.

The initial aggregate preflight adds one grammar pass to the existing decoder's
preflight/materialization passes: at most three grammar passes per supplied wire.
Canonical-copy, unescape, facts/tree hashing and existing bounded semantic passes
remain included in each codec's schedule. Comparisons directly traverse the
already canonical arrays and strings; no sorting, maps, re-encoding or equality
buffers are introduced. Loops retain the existing per-model entry bounds and
finite fixed role counts. This proves bounded work by input lengths/counts, not
an instruction counter, disk-I/O limit, wall-clock deadline or whole-history scan.

Resource overflow/refusal has its own fixed private sentinel; malformed or
mismatched selected records have another. Both return zero links, perform no
external action and grant no BAD classification. All five capacities and all
wire preflights are checked before any model materialization, including a late
over-budget record after otherwise valid inputs. Caller bytes and typed values
remain unchanged.

## Integration gates retained

The result contains protected digests and the completed operation tuple, not
models with authority methods or a new public schema/API. Actual file/role/
namespace/source/tree/toolchain/install authentication, unsupported-before-corrupt
recognition, cache-miss treatment, whole-Store accumulation/scan accounting,
publication/lease/recovery and frozen-host/installed acceptance remain required.
No Nix artifact or host qualification follows from selected-record success.
