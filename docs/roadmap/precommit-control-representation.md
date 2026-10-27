# Selected precommit control records

This note is subordinate to PC-042/050/061 and ARCH-018/024/025/026/027/030/031.
It closes private selected value correspondence only, before current commit.
It does not supply namespace recognition, classification, source/install
authentication, lease, admission, liveness, publication, repair, recovery or
completion authority. It adds no public API, format or package.

## Roles and relations

Reuse the concrete five-wire `committedControlInput` representation. For the
precommit entrypoint, `Current` is the complete prior scalar current record or
exactly its zero value; partially populated absence is invalid. Writer must be
scalar-valid and held. Slot zero is intended B; slot one is prior A.

| Selected role | Requirement |
|---|---|
| Candidate 0 | Mandatory canonical B, matching Journal.Candidate |
| Candidate 1 | Mandatory distinct A when prior Current exists and A differs from B; otherwise nil |
| Receipt 0 | Optional distinct pending B, only at pending-receipt; nil for absent or alias |
| Receipt 1 | Mandatory completed RA whenever prior Current exists; otherwise nil |
| Journal | Mandatory canonical prepared, tool-install, home-apply or pending-receipt record; cleanup is outside this boundary |

Only nil means absent. Present empty views are malformed. Alias roles reuse the
retained object, without duplicate wires even when canonical bytes are equal.
Currentless bootstrap has empty previous digests and absent prior slots, but may
have any scalar-valid held writer generation; there is no generation-one-only
claim or inference about its historical reachability.

Journal instance matches Writer and any prior Current. Its writer tuple is the
exact retained held owner, including previous token, or exactly one generation
ahead with previous token equal to the retained owner token and a fresh different
writer token. That ahead token differs from Writer.Token, Writer.Instance and
any nonempty Writer.PreviousToken; it cannot restore a known retained token or
name an impossible future held owner. The ahead interval cannot relax the
retained-owner comparison.
For prior Current generation c, require W.Generation > c before subtraction:
fresh acquisition (empty W.PreviousToken) has distance at least two; transferred
held ownership (nonempty W.PreviousToken) has distance at least three. Prior
committing token differs from W.Token, nonempty W.PreviousToken and the journal's
writer token. These are closed lifecycle consistency checks, not death proof.

Prior Current binds Journal.Previous and PreviousReceipt independently. New
operation ID and token each differ from their corresponding retained prior
Current field. This is not the completed-current predicate and does not infer
new authority from candidate/receipt digest reuse. No historical registry exists.

Recompute canonical candidate digests and authenticate pending-facts digests.
Check RA's own necessary completed phases/four dimensions and full static match
to A; its own Previous stays historical. Check B target coverage and exact
journal execution prefix at every accepted phase. Distinct pending RB requires
complete static B facts, Previous equal to Journal.Previous, exact preconditions
and results equal to the journal, both completed phases and the four true
dimensions. ArtifactAvailable remains independent. Scalar codecs continue to
accept their full Boolean and phase-subset value language.

If Receipt 0 is absent at pending phase, an already retained RA may serve as RB
only when all those complete B facts already match. Before pending phase it is
only the prior receipt, even if its facts happen to match. An unmatched RA is
not an implied pending receipt. Supplying the same receipt again in slot zero
refuses; actual object reuse does not create a second publication.

Return only B's protected digest, an actual selected pending receipt digest or
empty receipt, Journal operation/writer fields and phase, using the existing
104-byte links type. Failure returns zero links and fixed separate record versus
resource errors, preserves caller values/bytes and performs no external action.
False pending dimensions are a semantic refusal here, not a codec restriction
or a BAD classification. General A/B links never grant PC-071 update admission.

## Preimplementation resource schedule

Keep the exact existing representations, measured by pinned Go 1.24.9 on x86_64:

| Retained shape | Bytes |
|---|---:|
| Input | 392 |
| Two candidates, two pending receipts, journal and candidate digest slots | 3016 |
| Links | 104 |
| Counters | 40 |
| Preflight result | 16 |
| Validated small-record string payload maximum | 405 |

All five visible capacities are charged together, independently for overlapping
views, up to the existing 40,000,000-byte selected-wire allowance. Every complete
decoder schedule is preflighted without allocation before any materialization;
all retained decoder reservations stay charged together under the separate
40,000,000-byte allowance. Each original codec still admits at most 8,000,000.
Neither allowance is a Store or history cap.

Factor only capacity/grammar/reservation work into a pointer-taking shared helper.
Both wrappers keep their separate typed-scalar checks and error boundaries;
committed validation still requires its original completed-current predicate.
The shared helper receives an input pointer, not another by-value input copy.

Keep the original 8,493-byte outer schedule: two inputs, decoded values, two
links, two counters, two preflight results, 3,968 serial fixed workspace and
405 validated small-record payload bytes. Reassert actual type layouts before
coding. The original 1,760-byte serial preflight includes the reserve callee's
392-byte input copy. Conservatively additionally charge another full input,
four counters, sixteen pointers, four error interfaces, sixteen integers and
sixteen booleans for wrapper/shared/semantic bookkeeping: 2,648 bytes total,
within the existing 3,968 block. The 848-byte by-value Completed predicate and
the same extra bookkeeping plus two result copies also fit that block. Serial
preflight, decode and semantic comparison do not run concurrently. Do not add
input-dependent comparison buffers, sorting, maps, marshaling or retained copies.

Thus this exact representation can retain the 80,008,493-byte logical selected
invocation schedule, conditional on actual implementation/layout assertions and
independent review. It is conservative reservation accounting, not literal Go
cumulative allocation enforcement, a hard runtime-memory cap or a claim about
unobservable backing allocation outside a supplied slice's visible capacity.

## Gates retained

Complete namespace/history scanning, unsupported-before-corrupt recognition,
cache policy, source/tree/toolchain/install authentication, full Store work/FD/
intermediate-publication accounting and physical acquisition are not provided.
Actual pinned Nix artifact inventory and frozen-host/crash/installed acceptance
remain required. No artifact-fit, host support, release, Phase 2 or MVP completion
follows from this selected-control seam.
