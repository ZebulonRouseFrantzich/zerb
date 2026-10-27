# Private durable-codec representation

This is the owner-approved design gate for the bounded private candidate,
pending-receipt and journal codecs. Production implementation and its independent
review must match this accounting before any fit claim. It neither supplies a
whole-graph budget nor qualifies filesystem, artifact or host behavior.

## Why the ordinary recipe is insufficient

Pinned Go 1.24.9 was exercised with synthetic existing canonicalizer fixtures.
An accepted candidate produced 6,002,199 canonical bytes under a 7,009,793-byte
encoding reservation. The standard decoder's buffer capacity alone reached
8,388,096 bytes, above the complete 8,000,000-byte reservation. Completed facts
produced 6,002,772 bytes; constructing their pending wrapper through a second
complete wire buffer needs at least 12,005,672 bytes. These are byte/capacity
observations, not cumulative allocation, RSS or artifact-inventory evidence.

The approved mechanism uses a private closed canonical parser and direct pending
encoder, preserving every wire/value rule and the existing reservation. Ordinary
small scalar codecs retain their strict standard decoder and re-encoding recipe.
No dependency, exported API, lower field cap or larger budget is introduced.

## Complete-invocation schedule

Use `W` for candidate/completed canonical bytes; `S_cap` for string payload in all
retained caller capacities, including spare entries; `A_cap` for recursive caller
slice backing; `A_len` for exact copied visible lengths; and `T` for both component
tree preimage lengths. Source scratch is the existing `Q = 2048 + 16*URI_bytes`.
Retain the existing conservative element charges: component 72, file 48, tool 56,
lock 32, executable/tool observation 64, string 16 and destination observation 96.
The actual destination-observation struct is 88 bytes on the pinned amd64 build;
the conservative charge stays 96.

The existing candidate schedule is
`2048 + Q + W + S_cap + A_cap + A_len + T`.
The existing completed schedule is
`4096 + Q + W + S_cap + A_cap + A_len + T`.

Direct pending encoding uses
`3968 + Q + (W + 128) + S_cap + A_cap + A_len + T`, exactly the old completed
schedule. Its 128-byte wrapper is explicitly charged; it is not an exemption.
Preflight precedes copying/sorting. Allocate only exact visible collection copies
and one final pending output, emit facts directly, hash that facts span only and
fill the wrapper digest. Never call ReceiptDigest to allocate an intermediate
completed output in the production pending encoder.

Canonical decoding uses exact-capacity output slices and one owned wire buffer:
candidate `2048 + Q + W + S_len + A_len`; pending
`3968 + Q + (W + 128) + S_len + A_len`. These are dominated by the existing
minimum-capacity encoder schedules. Reconstruct and enforce that old schedule
too: greater decoder efficiency must not widen accepted values.

Journal encoding reserves `2048 + J + S_cap + A_cap + A_len`; decoding reserves
`2048 + J + S_len + A_len`. `J` is the exact journal wire size. All arithmetic is
checked before allocation/growth. This does not introduce an aggregate graph or
retained-object-count policy.

## Fixed storage bound to implementation

| Storage | Candidate bytes | Pending bytes |
|---|---:|---:|
| Two top model slots | 960 | 1776 |
| Cursor | 40 | 40 |
| Counting/emitting writers | 96 | 96 |
| Reservation and scalar plan | 64 | 64 |
| Thirteen collection counters | 104 | 104 |
| Three spans | 48 | 48 |
| Reusable SHA-256 state | 120 | 120 |
| Three binary hashes | 96 | 96 |
| Hexadecimal scratch | 72 | 72 |
| Returned digest payload | 72 | 72 |
| Two tree slice headers | 48 | 48 |
| Decimal scratch | 24 | 24 |
| Pending wrapper string payload | 0 | 88 |
| **Concrete total** | **1744** | **2648** |
| **Reserved control** | **2048** | **3968** |

The concrete private types and hash state are checked by
`TestCodecImplementationLayout`; the scalar plan includes its actual alignment
and a separate reservation counter (64 bytes, eight above the initial sketch).
Both totals still fit the unchanged control allowances. Layout checks alone do
not validate allocation paths; those require independent implementation review.
These constraints never permit untracked allocations. Do not hold Candidate and Completed together
in an all-schema result union that exceeds the candidate allowance. Source
scratch remains separate. Runtime allocator metadata, stack/GC effects and fixed
schema caches retain PC-042's existing treatment; this is not an RSS guarantee.

## Parser and ownership

1. Reject over-bound wire before parsing.
2. Count decoded payload, exact collection lengths and facts/tree spans with an
   allocation-free closed-schema pass. Keys/order, escapes, UTF-8, numbers, arrays
   and EOF must already have their exact canonical representation.
3. Reserve the complete schedule and old minimum-capacity encoder admission
   before materializing values.
4. Copy borrowed bytes exactly once into a private buffer. Hash object/tree spans
   before mutation; unescape only inside that buffer, then copy each decoded span
   once into its final string. Never mutate caller input or retain aliases.
5. Allocate slices with `len == cap == counted length`, fill directly, and check
   semantic constraints and canonical ordering without sorting/copying again.

The private entry points are planned as `decodeCandidateRecord([]byte)` returning
Candidate plus its digest, `decodePendingReceipt([]byte)` returning the specified
private pending record, and `encodePendingReceipt(receipt.Completed)` returning
the computed receipt digest and complete pending bytes. The encoder constructs
its fixed envelope; it accepts no caller-supplied envelope digest. Decoding must
compare the supplied digest with SHA256 of the exact facts span. A separately
needed record-checking encoder must reject a supplied mismatched digest rather
than trusting it. These are private implementation functions, not ARCH-030 API
extensions.

State cannot call receipt's private validators/writers or call either exported
digest function as a production decode/pending-encode shortcut: that would add
another complete wire/copy pass. These state codecs perform direct semantic
checks and emission with strict differential parity; receipt functions are test
oracles only. The candidate-only 16,384-value-string count does not become a new
Completed limit. Executable absolute paths, ZERB version and platform strings keep
their existing aggregate-only bounds. Source scratch uses decoded URI bytes.

No whole-value Decoder, unquoting scratch, growing builder, map, token table,
input-dependent pool or allocated input-bearing error is admitted. Strict grammar
plus complete semantic checks replaces materialized re-encoding only for these
approved large codecs. The standard encoder/canonicalizers remain test oracles.

Future protected-file integration must replace growth-based reads with exact-size
reads under unchanged public signatures/identity checks and explicitly transfer
the newly owned buffer to a private consuming decoder. A borrowed decoder still
copies and preserves its input. This design alone does not establish the complete
Store/filesystem invocation reservation.

## Differential acceptance and evidence

Compare decoder acceptance against strict standard decoding followed by the
existing canonicalizer and exact input-byte equality. Compare values, digests and
bytes, never just a self-round-trip. Exercise all existing corpus vectors, empty
arrays, collection/count limits, identity collisions, tree hashes, escape classes,
invalid UTF-8, integer boundaries, unknown/duplicate/missing/aliased fields, null,
whitespace and trailing values. Include large escaped and unescaped values, exact
budget boundaries, hidden caller capacities, non-mutation, exact output capacities
and large-to-small histories. Fuzz valid model-to-wire and arbitrary wire paths.

Pending pure codecs preserve every dimension Boolean combination and phase subset
accepted by ReceiptDigest. Completion is separately checked by state. Journal
progress remains execution order and uses the approved latest-observation rules.
Source/installed authenticity, lease, live observations, current completion,
aggregate graph resources, actual artifact fit and host qualification remain
outside these pure codecs.
