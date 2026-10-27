# Observed reads and graph-resource accounting

This note records representation evidence subordinate to PC-042/061 and
ARCH-018/020/024/030/031. The owner approved preserving valid immutable history,
operational resource refusal without classification/deletion, and observed-length
reads. It freezes no whole-Store numeric envelope and supplies no graph admission,
artifact authentication or host qualification.

## Implemented observed-read representation

The three existing collection routes use one private positional core:

1. Verify the expected size is addressable; measure the bounded file through its
   existing readable descriptor with one 32,768-byte scratch buffer. A complete
   measurement requires exactly the expected bytes plus a successful EOF/growth
   probe and all required file/proc/ancestor checks. Reported size or caller limit
   alone never causes a whole-result allocation.
2. Only after that evidence, allocate the observed length with exact capacity and
   reread through the same descriptor. Require identical length and SHA256; all
   reads retain their validation and error checks. Hashing is not an atomic
   snapshot or proof against a fully restored invisible same-UID cycle.
3. Return independent bytes; every failure returns nil bytes and zero digest or
   identity through the owning wrapper. Preserve descriptor offsets, ownership,
   restrictive admission, close-on-exec, cleanup failures and shared lifetimes.

The only input-dependent allocation in the content core is the one exact `W`-byte
result; it replaces `io.ReadAll`/append growth and the TreeStage's premature
reported-size allocation. One scratch is reused across both passes. SHA256 state,
pass/result digests, callbacks and counters have fixed size; File additionally
uses fixed-size hexadecimal encoding. No whole-wire pool, second payload buffer,
new descriptor or result slice growth is introduced. These fixed additions fit
inside TreeStage's existing 65,536-byte result/control allowance, with its
32,768-byte scratch separately reserved. Its existing requested-result/workspace
and descriptor admission stay unchanged. This is logical representation
accounting, not cumulative Go allocation or an RSS bound. Proc/ancestor/topology
work and later decoding remain the caller's complete accounting obligations.

| Existing route | Complete successful payload/EOF passes |
|---|---:|
| Ordinary `Dir.ReadFile` | 2 |
| TreeStage `Dir.ReadFile` | 2 |
| `PinFile` construction | 3: collection twice, existing automatic revalidation once |
| Each later `File.Revalidate` | 1 |
| Existing successful checked `File.Remove` | 3 |
| Tree hash-only verification | 1 per owned invocation |

The size remains an enforced boundary, not a metadata allocation instruction.
The measurement loop proves the observed count equals it before allocation.
For positive `W`, a pass can have `ceil(W/32768)` through `W` positive reads,
then one EOF probe. Short successful reads prevent using the lower bound as a
syscall/work upper bound. TreeStage charges every requested length before its
read, including both collection passes and each EOF probe; charge refusal stops
before that I/O. Typical full reads charge `W+1` per pass; the conservative
requested-byte upper bound is `32768*W+1`. Its existing 4 GiB lifetime ceiling,
poisoning and preserve-only outcomes remain unchanged. A later pass may exhaust
that budget; no partial result or cleanup permission follows.

Deterministic operation-local read/validation/charge callbacks exercise partial
reads and exact failure boundaries; they do not claim kernel fault injection.
Real disposable ordinary/pinned/tree reads exercise the existing descriptor and
proc implementation. Exported positive paths still need admissible absolute
ancestors; a skipped public test is not replaced by local helper success.

The borrowed codec entry points remain unchanged. A future production
file-to-codec caller must charge/transfer its uniquely owned canonical buffer
before private in-place decoding; retaining it and adding the borrowed decoder's
copy cannot be excused by per-codec success. No unused consuming variant or
partial Store is introduced here.

## Derived active-control bounds, not history limits

ARCH-018/024/025 admit at most two distinct candidate roles, two receipt objects,
and one journal in a live relation. Exact reapply can reuse identities; first
bootstrap omits prior objects. The existing pinned top-level model layouts use
`2*480 + 2*888 + 248 = 2,984` bytes before strings, recursive slice backing, wire,
small records and filesystem resources. This is not an outer allocation budget.
Independently authenticated unreferenced candidates/sources/trees/toolchains
remain legal and have no contract-derived count bound.

For a future complete graph, account:

`retained active models + retained history/name/path observations + filesystem capabilities + maximum overlapping codec/read/list/topology/comparison workspace`

Charge actual recursive retained capacities and decoded payloads. Each codec's
complete reservation is added to other retained graph storage; never reset the
operation's accounting while those results remain live. Validate historical
models serially where safe, releasing each full model before retaining active
models. Retain enough exact namespace/identity evidence for final revalidation;
serial models do not make total enumeration/hash/reread work bounded by themselves.

A retained File or Lock duplicates its parent chain and keeps seven more FDs:
pin, reader, four proc-chain entries and pinned `self`. With root-inclusive
ancestor depths `Ls`, `Ld`, `Lc`, the current narrow initialized observation's
steady resources derive to `10*Ls + 3*Ld + 5*Lc + 58` FDs. This includes its eight
Dirs, six Files, admission Lock and Roles chains/namespace pin; transient topology
validation and unrelated process descriptors are additional. It is an observation
of the existing implementation, not a proposed maximum.

More generally reserve Roles, each directory chain, each file/lock parent chain
plus seven, authenticated-data pins and simultaneous validation/cleanup resources.
`duplicateChain`/directory/proc-cleanup slice growth and `Names`/Readdirnames
capacity, strings, cursor buffers and over-limit probe must be accounted by their
actual representation. A name count is not a name-byte budget. Final namespace
comparison retains its original observations plus the newly enumerated directory.

The narrow initialized helper revalidates all retained files around each step.
After the read change its successful initial path without hooks performs 23
root, 21 init-lock, 19 writer-lock, 17 writer-owner, nine data-owner and seven
cache-owner passes; lock files are empty but still require probes/checks. Each
later facts read adds two full validations. Copying that pattern into a history
scanner would create quadratic reread work. A complete scanner needs a finite
capture/final-validation protocol, mandatory owned-boundary checks, comparison
byte accounting and cumulative I/O/topology/enumeration accounting. No such
complete scanner is claimed here.

Before publication, reserve the worst intermediate graph: existing history, new
objects, prior/successor candidates and receipts, journal/current/owner replacement
forms, exact witness/residue, caller model/output/equality-comparison storage and
required revalidation/cleanup. Digest equality never replaces required exact-byte
comparison. Exhaustion preserves evidence and the original intent lease, with
no unproved supported or BAD classification, mutation admission or cleanup right.

## Remaining fit gates

Concrete whole-graph numeric envelopes cannot yet be frozen from active reference
counts alone. Complete source/tree/toolchain/install layouts and authenticated
verification schedules are not implemented; exact-0600 record pins cannot silently
become executable/data pins. Unsupported-domain precedence requires recognition
evidence beyond a generic canonical-decode error. Composed Names/ancestor/proc/
topology capacities, concurrent peaks and finite scan work still require proof.

The real pinned Nix inventory supplies actual manager bytes, entry/path counts,
ELF/runtime/license forms, manifest/provenance and subtree/materialization fit.
It is not needed for the control-reference and read-pass derivations above, but
TreeStage/Node proof maxima cannot substitute for it. No artifact hashes, accepted
history counts, global workspace/FD/I/O thresholds or qualification are invented.
