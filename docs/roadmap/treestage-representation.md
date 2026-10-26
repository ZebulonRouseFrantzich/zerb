# TreeStage representation fit candidate

This is the concrete, provisional representation review for ARCH-020/030's
TreeStage proposal and VAL-027's 22-row planned matrix. It implements no tree
primitive and proves no filesystem, allocation, concurrency, host or durability
property. The test-only layout checks sizes and arithmetic on Go 1.24; runtime
admission and every failure-matrix row remain future work. The owning lifecycle
and limits remain in `architecture.md`, not this note.

## Concrete storage

Use one shared stage state, referenced by ordinary stage value copies and every
derived `dirState` and `roleEntry`. Intended fields are `mu sync.Mutex`,
`drained *sync.Cond`, `phase uint8`, `owner uint32`, `parent []edge`, `root edge`,
`lent *Dir`, `active uint32`, `roleRefs uint32`, and checked `uint64` counters
`entries`, `paths`, `payload`, `reads`, `fds`, `workspace`, and `scratch`.
`phase` includes active, draining, published, removed and preserve-only. The gate
must stop admissions before waiting, and never wait with a directory mutex held.
An operation owns a reservation ticket until all its internal cleanup ends.
Closing a caller-owned child view releases only that view. Terminal work closes the constructor-owned borrowed root through its shared state after draining; revoked caller-owned child/role FD numbers remain live and charged until their explicit Close.

The constructor allocates one borrowed root view. `Root` only returns that view
or nil, with no allocation, duplication or failure channel. Each derived view
adds `gate *stageState` and `ticket *stageTicket` to `dirState`. The view ticket
is a separate 16-byte object (`bytes uint64`, `fds uint32`, `flags uint32`). A
retained role adds those same two pointers to `roleEntry` (80+16=96 bytes on
amd64), rather than embedding an unbounded/inline reservation; copying metadata may not strip
either. Stage operations use at most four admitted operation slots, with one
terminal slot after draining. Reject excess concurrent entrants before effects;
this is workspace admission, not a new tree size/depth policy.

The deliberately simple first layout reserves maximum entry/path capacities at
construction: four flat inventory slots and four tight byte arenas, each arena
capacity 16 MiB. No maps, recursive walk stack, input-dependent append capacity,
string interning, or per-entry descriptor set belongs to this new representation.
`N` and `P` in the reservation are **reserved capacity**, initially 16,384 and
16 MiB, even for an empty tree. Live entry/path counters separately enforce the
content ceilings. This conservative choice leaves a constant 30.75 MiB for
additional result/helper reservations. It is explicit availability behavior:
whole-file reads can refuse below the file-size limit even on an empty stage.

Each inventory entry contains `fileStamp` (identity, mtime, ctime), a `[32]byte`
SHA256, `pathOffset uint32`, `pathLength uint16`, `depth uint8`, `kind uint8`,
`parent uint32`, and `flags uint32`. Paths live in the arena, never in retained
substrings of caller data. Per admitted entry, reserve four such records, four
`uint32` sort indices, four traversal records (`listingStamp`, `next uint32`,
`parent uint32`), and four `uint64` admission/bookkeeping cells. On linux/amd64
these consume 912 bytes, below 2,048 bytes. The unused portion is reserved
headroom, not permission to add an unclassified allocation. Sorting permutes
indices in place with bounded stack, without copying names or entries.

Inventory slots have exclusive owners: cumulative builder ledger, complete
preterminal snapshot, after-sync comparison, and final reopen comparison. A
cleanup uses the same slots for complete prevalidation and checked postorder
progress. They coexist at their maxima; passing a phase does not excuse a fifth
snapshot. Concurrent builder operations serialize ledger edits under the gate;
they do not each clone the ledger. Operation results and temporary helper data
are separately reserved as `R`. No output may retain an arena after its slot is
reused: a returned `Names`/read result receives its own charged copy.

## Descriptor ownership and depth

Let `A` be the supplied absolute parent's full chain length, including `/`.
It is independent of the **relative** depth limit 64. Do not impose 64 on `A`.
The stage duplicates the entire parent chain (`A` FDs), owns its newly created
root independently (one FD), and constructs the borrowed view with another
complete chain (`A+1` FDs): constructor retained cost `2A+2`, plus temporary
validation opens. The private root edge borrows its parent edge only while the
stage's independent parent chain is live; it is not another hidden `A`-FD chain.
Closing the original parent or borrowed view leaves private pins intact.

A public child view at relative depth `d` owns `A+1+d` FDs; every intermediate
view and every `Roles.Add` duplicate costs its entire chain again. Ordinary value
copies share one state and do not duplicate. During `OpenDir`/`CreateDir`, the
old view, temporary candidate edge array, new child FD, complete duplicated
ancestor chain and validation FD coexist. Reserve all of them before opening or
duplicating. Never count only the leaf or only the final post-return chain.
Stage-owned FD accounting follows stage-derived role copies until role close,
including chains retired only after replacement candidates are committed.

Terminal traversal uses the private parent/root pins and a fixed 64-frame
relative cursor, one retained directory edge per relative component. It borrows
the private root and does **not** duplicate the absolute chain per frame. Close
an enumeration cursor before descent; close the child edge on ascent. Reopen
directories through checked pinned parents, compare the captured entry and
listing stamps around each affected action, and never follow a link or cross a
mount. Reserve a validation FD and the read/proc auxiliaries before use. With
views/role copies closed, this requires `A+1+64+8` FDs at the deepest streaming
file verification. Any constructor fitting `2A+2+8 <= 256` therefore fits that
64-level internal walk (`A <= 123`, terminal peak at most 196). Eight includes
file pin, proc's five retained FDs, readable reopen and one validation FD.

Retaining a public chain of deep descendants can exhaust 256 FDs and refuse
further calls. Even closing each parent after opening the next does not erase
the old/new overlap during construction; for sufficiently deep absolute parents
the public construction can refuse before relative depth 64. This is the stated
FD ceiling applied to actual duplicates, not a smaller relative-depth policy.
At `A=6`, opening depth-64 from depth-63 with the old parent and private stage
pins live uses `7+70+71+1 = 149` FDs after the borrowed root is closed. The bounded
internal walk can finish a legal depth-64 tree without this duplicated-chain
growth. Caller-retained exhausted views must be closed before terminal admission;
terminal cleanup may refuse/preserve rather than exceed its budget.

## Complete reservation schedule

All arithmetic uses checked `uint64` addition/multiplication, then checks against
the target's `maxInt` before `make`/index conversions. Byte counters include raw
bytes and separators; caller payload is excluded only while borrowed. An
internal payload copy is `R`. `reads` monotonically charges each requested read
before I/O, including comparisons, hashes, retries and one-byte EOF probes;
unused simultaneous capacity can be released, lifetime work cannot be reset.
Admission reserves entry, depth, name/path, aggregate payload, workspace, FD and
scratch needs atomically before allocation/open/duplication/mutation. Resource
refusal before effects is nonpoisoning; exhaustion/mismatch/I/O/close failure
after admission poisons and preserves. An active-operation count alone does not
reserve resources.

| Reservation | Concrete occupants |
|---|---|
| 1 MiB fixed | Shared stage/gate/condition, borrowed-view wrappers, tickets and four fixed operation control areas (64 KiB each), terminal cursor/control area (64 KiB), bounded component/path buffers, fixed crypto/stat/syscall state; remaining fixed slack is unavailable to variable helper storage. No content-sized allocation is hidden here. |
| `2,048N` | Four inventory records, four sort index arrays, four traversal/listing records and four bookkeeping cells per entry, including all simultaneously retained snapshot metadata. |
| `4P` | Four complete path arenas with exact capacities. No fifth path inventory or uncharged retained string is permitted. |
| `512F` | Tight copied edge array capacity, tight name bytes (at most 255 per owned edge), added gate/ticket state and directory wrappers, amortized per owned FD; no geometric chain capacity. View/role wrapper allocations must stay within the 512-byte charge, or reserve excess as `R`. Full absolute-chain names are cloned tightly, avoiding retention of a large caller path backing string. The worst one-edge directory view is 112-byte intended dirState +8-byte Dir +80-byte edge +16-byte ticket +255 name bytes =471 bytes; every larger chain amortizes the wrapper further. The layout test asserts these slots and the 96-byte intended role entry. |
| `S <= 128 KiB` | Four at-most-32-KiB streaming buffers shared/reserved by operations; terminal work reserves its buffers after draining. A helper's stack buffer counts even if it never escapes to the Go heap. |
| `R` | All result backing capacity and headers, output copies, temporary edge candidate arrays not represented by an owned FD, path assembly not in fixed buffers, role/topology work and all other helper transients. Several concurrent results sum here; returning one does not transfer another. |

At simultaneous maxima the scheduled reservation is
`1,048,576 + 2,048*16,384 + 4*16,777,216 + 512*256 + 131,072`
`= 101,974,016 bytes = 97.25 MiB`. Thus
`R <= 32,243,712 bytes = 30.75 MiB`; one extra result byte must refuse. The
test-only 912-byte per-entry layout plus four arenas uses 82,051,072 bytes;
the larger schedule intentionally reserves the difference. This is backing
storage accounting, **not** literal Go allocator, RSS, stack-growth, GC-retention
or host fit evidence. It also does not claim one 256-MiB `ReadFile` can fit.

## Existing helpers are not free

These are required implementation audit points, not assertions that current
helpers already enforce this schedule. New stage-aware branches must preserve
all existing validation and unchanged nonstage behavior.

| Helper | Actual auxiliary ownership and required admission |
|---|---|
| `duplicateChain` / `OpenDir` | Current append growth and temporary candidate array coexist with both full FD chains. Stage use needs exact-capacity duplication, candidate-array `R`, and FD reservation before each dup/open. `validateChain` temporarily opens `/` (one FD). `closeChain` adds no FD; clear ownership before close and never retry released numbers. |
| `procAccess` | Four chain edges (`/`, proc, numeric PID, fd), separately pinned self link: five FDs. Its validation adds a temporary root FD; `close` currently appends self into the chain and can allocate another backing array. Reserve this array as `R` or close separately with unchanged error handling. PID/FD-number formatting and struct/edge storage are charged too. |
| `pinnedFile` / `readFile` | One O_PATH pin, one readable reopen, proc five plus validation one: peak eight auxiliary FDs. Parent edge is borrowed, not another FD. `os.File` and its internal state are `R`. Current `io.ReadAll` has geometric result growth/EOF room: conservatively reserve at least `4*(requestedSize+1) + 64 KiB` as `R`, and separately charge any still-live old backing arrays. The safer planned stage branch allocates one exact size result, streams positional bounded reads plus one-byte probe, and transfers only after all validation/close succeeds. Large reads refuse before pin/open. |
| `publication` | O_TMPFILE FD plus optional final O_PATH `pinnedFile` (two), borrowed parent, `publication`/pin storage, and 32-KiB comparison buffer. `write` compares once and `verify` compares again: each byte and EOF probe spends lifetime-read budget. No caller payload copy is assumed; any added copy is `R`. Full descendant validation/cleanup remains enrolled through uncertain link and final close. |
| `Names` / `listNames` | Current `os.File.Readdirnames(limit+1)` adds a listing FD, `os.File`, internal directory buffer, growing `[]string`, separately allocated name strings and one-entry lookahead. It has no 255-byte stage-specific limit or total-byte preallocation admission. It cannot be called as an opaque bounded allocator. Planned stage enumeration parses no-follow directory records into a reserved 32-KiB scratch buffer, bounds each raw name/entry/path before copying, stores into one reserved snapshot slot, and returns a separately exact-capacity `[]string` plus copied name bytes (`16*count + nameBytes` on amd64) as `R`. No child is opened to list it. |
| `Sync` / file stat/hash | Directory sync adds no FD; stat structures and SHA256 state are fixed control storage. Streaming file hash adds the eight read auxiliaries and scratch above, never the file's size as workspace. |
| `PinFile`, `OpenLock`, `StageReplacement`, nested `StageTree` | Deny before effects on every stage-derived view. Their otherwise independent retained pin/proc/chain/file/lock/replacement lifetimes must not escape the gate; no resource estimate makes them admitted here. |
| `Roles.Add` and later retained-role validation | The new full chain is stage-charged, as are candidate/added/retired arrays, copied target names and gate references. Current topology opens six proc-chain edges, self, namespace link, namespace FD (nine retained), plus validation/current namespace/readMountinfo transients, mount-pin chains and its separate 4,096-FD allowance. It retains an 8-MiB mountinfo snapshot; `finish` reads another. Parsing allocates row arrays, per-line string/field arrays, decoded path buffers/strings, IDs map, color/trail arrays; geometry adds relevant/seen arrays/maps, coordinates/objects maps, paths, cones and derived coordinate strings (up to 12,288 bytes). Cleanup currently builds another combined edge array. Every such allocation must be charged to `R`; existing caller ownership is not an exemption for work caused by the stage. |

## Concrete bounded topology candidate

For stage-derived Roles operations, use a private flat working representation
with the existing topology grammar and alias/mount/namespace predicates. Generic
nonstage behavior is unchanged. This is a representation proposal, not permission
to implement a broad topology rewrite before independent review.

The stage preflights all existing role-chain edges before duplicating or opening
anything. Conservatively reserve them against the stage's 256 FD/coordinate
slots even where the caller already owns those FDs. The topology's own retained
proc, namespace, mount-pin chains and transient opens also occupy those slots.
If that combined context cannot fit, refuse before effects. This explicit
resource refusal permits no omitted role or mount check. All mount-pin chains
remain retained until finish; each distinct physical coordinate therefore has
at least one retained edge behind it. At most 256 distinct coordinates are
admitted. Both directions of the coordinate/object bijection use sorted index
arrays over those records; repeated coordinates do not allocate another string.

| Topology `R` allocation | Exact reserved capacity |
|---|---:|
| Initial raw mountinfo snapshot | 8,388,608 bytes |
| Final raw mountinfo comparison snapshot | 8,388,608 bytes |
| Decoded root/point/filesystem-kind arena | 8,388,608 bytes |
| Physical-coordinate arena | `256 * 8,192 = 2,097,152` bytes |
| Flat rows (`id,parent,device uint64`; three offset/length spans) | `16,384 * 72 = 1,179,648` bytes |
| Sorted ID/index records (`uint64 id`, `uint32 index`, padding) | `16,384 * 16 = 262,144` bytes |
| All auxiliary arrays/control below | 1,048,576 bytes |
| **Total additional topology reservation** | **29,753,344 bytes = 28.375 MiB** |

The decoded arena contains tight copies of each retained root, point and kind
exactly once. Their aggregate decoded bytes cannot exceed the raw snapshot:
fields are disjoint raw spans and escape decoding only shortens them. Rows use `uint32` offset/length spans into this owned byte arena, with byte-span
comparison/decoding; they do not convert arena slices to allocating Go strings.
No unsafe byte/string alias is needed. No per-line `string(data)` copy
or split-field allocation may survive. Use a fixed 74-span field array while
parsing, and validate raw line/record/optional-field limits before decoding.
The complete arena is charged even if mostly empty. Initial and final snapshots
are exact-capacity arrays with a separately charged one-byte overflow probe;
there is no geometric `append`. EOF probes and 32-KiB read scratch are part of
`S` and all actual reads are charged to stage lifetime work before I/O.

Each physical coordinate is `row.root + suffix(current,row.point)`: each
operand is at most 4,096 bytes, so 8,192 bytes is a conservative storage bound.
The generic 12,288 derived-path validation bound remains unchanged for inverse
mapping/comparison intermediates. Namespace cone spans reference the exact
existing target or decoded mountpoint after equality validation; they are not
another per-cone string copy. A cone is four `uint32` values: coordinate index,
path index, next index and flags. Existing no-follow topology correlation checks
are retained, with a fixed 12,289-byte comparison buffer and checked lengths.
No cone or coordinate record may retain a temporary path-building string.

Before copying a computed physical coordinate into its arena, look it up by
byte span against existing coordinates and both object/coordinate indices.
Repeated prior/candidate correlation reuses the original span and never spends
another 8,192-byte slot. This check precedes the arena copy, not merely insertion.

The 1-MiB auxiliary reservation has the following fixed occupants (amd64):
16,384 graph colors (16,384 bytes), a 16,384-element `uint32` graph trail
(65,536), a 16,384-element relevant-row bitmap stored as bytes (16,384), a
16,384-element sorted mountpoint index for duplicate/seen checks (65,536), 256
coordinate records with device/inode/kind and physical offset/length (8,192),
two 256-element `uint32` sorted coordinate/object indices (2,048), 1,024
16-byte cones (16,384), 128 16-byte footprint range records (2,048), three
64-element `topologyPath` arrays using 96-byte slots (18,432: one current and
two retained borrowed path arrays across both `Roles.Add` passes), 256 mount-pin
path headers using 96-byte slots (24,576),
three 64-element candidate/added/retired role-control arrays (each conservatively
96 bytes per element, 18,432 total), 64 affected indices (256), 74 field spans
(592), three 12,289-byte path comparison/assembly buffers (36,867), tight copied
role targets of at most 4,096 bytes for 64 entries (262,144), and 131,072 bytes
for fixed topology/namespace/proc/File wrappers, numeric formatting, sorting
stack, close-control state and alignment, plus 8,192 bytes for the at-most-64
participating stage-gate tickets/roster. The test computes the complete sum; it
is below 1 MiB;
the remainder is reserved slack. Chain edge/name storage itself is charged
through `512F`, never this slack. Existing target copies belonging to ordinary
caller roles are conservatively covered too; nothing is declared externally
owned to avoid accounting. Cleanup iterates known-owned arrays without creating
a combined all-FD slice. Stage-aware operations may use linear scans/sorted
indices instead of maps; the proof of the existing predicates still needs tests.

At maximum stage workspace and this complete topology reservation,
`101,974,016 + 29,753,344 = 131,727,360 bytes = 125.625 MiB`, below 128 MiB.
The remaining `2,490,368 bytes = 2.375 MiB` covers separately charged
results/transients, or their admission refuses. Retained `roleEntry.target`
strings are tight independent copies, charged by the role ticket as `R` until
Roles.Close, never aliases into the temporary topology slab. Conservatively
allow another `64*4,096 = 262,144` bytes for these retained targets, including
old/candidate coexistence within the at-most-64 registration capacity (any
additional replacement copy is separately reserved before allocation). Thus
full stage + topology + 64 retained targets totals `131,989,504 bytes = 125.875
MiB`, leaving `2,228,224 bytes = 2.125 MiB`. The slab target slots cover
transient working copies only and are distinct from these retained strings. The topology pass reserves all
28.375 MiB before its first allocation/open and does not call the current
unreserved map/append implementation internally. Its reserved operation slot
and FD/scratch capacity also have to fit; bytes alone do not grant admission.
A large preexisting role context can refuse under 256 descriptors even though
ordinary Roles admits 4,096; no host limit or alias check is weakened.

This closes a conservative **layout** accounting candidate, conditional on the
new private bounded topology path being implemented exactly and reviewed.
Unmodified helpers are not fit evidence. A larger stage allocation, more than
four path inventories, an exemption for stage-origin topology work or weaker
validation would require a new decision; none is assumed here. The independent
reviewer and lead must accept this design before tree primitives begin. Byte-span
adapters must later be equivalence-tested against the owning topology grammar,
including optional tags, duplicate handling and all existing alias checks;
matching storage sizes alone does not prove those predicates.

A Roles set can retain several stage gates. Before taking directory locks or
performing effects, enroll every distinct participating gate and reserve the
whole operation against each; ordinary metadata copies retain the gate/ticket.
Capture the at-most-64-gate roster under the Roles lock, release that lock for
gate admissions, then reacquire it and verify its generation/roster unchanged.
On change refuse and release tickets before effects. No gate lock is held while
waiting for a Roles/directory lock; terminal work never waits for Roles close.
The two topology passes share the global 1,024-cone ceiling and 256-coordinate
ledger rather than resetting counters per pass. Both borrowed path arrays remain
charged through final validation. Reobservation reuses immutable coordinate spans.

All method coverage is inherited by `OpenDir` and merged `CreateDir`, `Names`,
`ReadFile`, file publication and `Sync`; cached `Identity` stays observation-only.
Retained stage roles participate in admission for future validation. A nonzero
retained-role observer count causes pre-effect terminal refusal, leaving the
stage active, without named I/O; callers close that transient role set before
promotion/retirement. It is not an indefinite drain waiting for role Close.
Caller-owned child chains are revoked and remain charged until their Close;
only the constructor-owned borrowed root is terminally closed through shared
state after draining. Complete safe inventory precedes first removal, every
acted-on edge is pinned/revalidated at that boundary, changed entries are not
adopted, and partial removal/rename/close uncertainty preserves the remainder
without retry or rollback. No array shape supplies historical authorship or
inode CAS.
