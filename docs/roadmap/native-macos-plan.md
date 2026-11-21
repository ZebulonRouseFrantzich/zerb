# Native macOS backend: deferred post-MVP plan

**Status:** Owner-requested planning, native implementation deferred until after
MVP. This document adds no runtime authority, supported platform, fallback
runner, dependency, public command, or release permission. The product contract,
architecture, validation, and delivery plan remain normative. This plan does not change priorities outside this repository.

## Outcome and immediate route

The intended eventual outcome is one ZERB product that restores native tools and
dotfiles on both Linux and macOS, with a shared Go core and small platform-owned
security boundaries. Maintain Linux behavior and protection while adding Darwin;
never silently weaken a Linux guarantee to make both platforms compile.

For the MVP and current PR testing, use a **dedicated Linux guest on the Mac**.
That runs Linux tools and manages guest files. It does not provision native macOS
tools or qualify application to the host home. A shared host directory does not
turn the Linux guest into a qualified Darwin backend. No real host home or
credentials may be mounted into the test guest.

A VM supports current engineering, not automatic Linux qualification: the frozen
candidate is Fedora 44/kernel 7.1.10-200.fc44.x86_64/x86_64/btrfs, with exact tool
pins in validation.md. An ordinary ARM64 Linux guest on Apple Silicon differs.
Use it only for correctly labeled development diagnostics, or reproduce the
frozen tuple through an approved x86_64 configuration. Changing the frozen tuple
requires its own evidence, reviews and approval. Do not use emulation success to
claim native ARM or current tuple support.

### Dedicated Linux-on-Mac checklist

- Inventory existing Lima/other guest runtimes before choosing one. Do not
  repurpose, restart, or contend with the other projects’ VMs or their work directories.
- Keep a dedicated guest, disposable HOME/XDG/manager/destination roots and
  dedicated writable disk. Keep VM images, packages, build caches and source
  worktrees on the designated external APFS drive. Do not install into the
  existing internal Nix store or change its configuration.
- Review host mounts, network, credentials, resource budget, storage and exact
  guest architecture/kernel/filesystem before VM creation/start. No credential
  forwarding; no host-home write mounts. Setup requiring new permission must
  receive it, rather than being hidden in a test script.
- Test PRs 35 through 48 in dependency order at their exact heads and bases;
  use independent worktrees, personal commit identity and gh. Refresh inventory
  if the stack changes. A top-of-stack pass is not proof that each earlier PR
  works independently or that an earlier failure was introduced by that PR.
- Prepare the pinned parser as AGENTS.md requires, run narrow checks then the
  applicable source-bound cloud/race/import gates. Preserve inconclusive results
  and runner logs; never rerun an uncertain process before reconciling it.
- There is no GUI in this replacement stack. The public command is still the
  obsolete pre-alpha entrypoint until Phase 6. Exercising it is not end-to-end
  evidence for the replacement implementation or its PRs.
- Hosted CI and local diagnostic evidence remain separate. Do not alter billing,
  safeguards, required checks, or support claims to get a green result.

## Reuse versus platform-owned behavior

Share profile parsing, canonical schema/digest rules, candidate trust and approval
logic, receipts/journals, recovery transitions, dependency planning, command
semantics, and Git/mise/chezmoi orchestration. Keep golden behavior tests shared.
Do not fork the repository or duplicate the whole product.

Platform boundaries must cover:

| Boundary | Existing Linux dependencies | Darwin work to qualify |
| --- | --- | --- |
| Root and object identity | openat2, statx mount identity, no-follow path walks, /proc mount geometry | Descriptor-based owner/type/mode/link checks, APFS volume/mount identity, rename races, case and Unicode behavior |
| Durable publication and locks | O_TMPFILE, rename flags, file/parent sync, OFD locks | Atomic exclusive/replace protocols, FULLFSYNC/directory sync, stable inode locking and death/recovery semantics |
| Process identity | PID/start/namespace identity, pidfds | Stable lifetime identity without pretending absent namespace fields are zero-valued Linux identities |
| Admitted executable launch | sealed memfd ELF execution | Exact admitted Mach-O bytes through a substitution-resistant launch, library policy and applicable signing constraints |
| Lifecycle and resource limits | FD-only supervisor, subreaper, parent-death guard, cgroups, namespaces | Equivalent reviewed per-class control, complete cleanup/reaping and finite CPU/memory/process/staging limits |
| Pinned native tools | exact Linux Git/mise/chezmoi/Bash builds and ABI observations | Exact ARM64 macOS tools, launch/environment/access/descriptor evidence and installed native integration |

Use explicit concrete backend boundaries where there are actually two production
implementations. Do not create a generic syscall wrapper or broad plugin system.
Version any durable platform-specific evidence deliberately; do not silently
reinterpret Linux state on Darwin. Cross-host state portability is not assumed.

## Ordered post-MVP milestones

1. **Contract and threat-model review.** Map each relevant PC/ARCH/VAL requirement
   to shared behavior or a platform-specific implementation. Define the macOS
   release/architecture/APFS/tool tuple and honest parity claims. Enumerate gaps
   without choosing a weaker fallback. Approve owning-contract changes before
   integration. Pay particular attention to ARCH-006/009/010/017/020/031 and
   VAL-020/023/025/041.
2. **Bounded feasibility.** Prove the hardest launch and lifecycle boundaries
   before a full port. Qualify immutable/protected image admission and execution,
   loaded libraries, substitution races, descriptor leakage, cancellation,
   parent hard death, group/session changes, descendant observation and cleanup.
   Independently review the design. If a requirement cannot be met, stop that
   platform tuple and present concrete options; do not relabel killpg as cgroups.
3. **APFS backend proof.** Use ownership-enforcing disposable APFS roots. Cover
   aliases, symlink/hardlink attacks, mount boundaries, replacement races,
   stable locks, unrelated descriptor closure, crash release, atomic publication,
   disk-full/disconnect and interrupted initialization/recovery. Distinguish
   syscall success from power-loss durability.
4. **Minimal shared-core extraction.** Retain Linux tests and behavior while
   separating only necessary backend calls. Keep public schemas/digests stable
   where their meaning is platform-independent; version platform records where
   required. Do not alter the public entrypoint early.
5. **Incremental Darwin integration.** Add inspection/planning, protected root
   and state operations, then native manager execution, in dependency order.
   Each increment needs regression coverage, race checks, focused security review
   and no real-user-state tests. Never leave a successful no-op implementation
   for an unsupported security boundary.
6. **Installed acceptance.** Run the full relevant installed CLI and failure
   matrix on the exact Darwin tuple, including native tool restoration, repeat
   application, dirty-state recovery, real cancellation and crash scenarios.
   Preserve Linux CI and negative tests. Only the owning validation/release gates
   can promote support; compilation or a VM pass cannot.

Each milestone should be a small reviewed feature-branch PR with its tested
commit and limitations. No automatic merge, main push, deployment or release.
Package additions/upgrades and unresolved major architecture decisions need
owner approval. No reliable full-port completion estimate exists before the
launch/lifecycle feasibility milestone; this is a multi-stage backend project,
not a build-tag patch. Native implementation is deliberately deferred now.

## Initial diagnostics retained, not qualification

The [bounded primitive probe](../../tools/darwin-spike/README.md) ran before the
owner deferred native implementation. Exclusive rename/sync and basic flock
semantics worked in disposable APFS locations. It also demonstrated two unsafe
shortcuts: a checked open image can differ from later pathname execution, and a
setsid child can escape process-group termination. Those are counterexamples,
not a production backend. On the successful observed runs, lifecycle-test children were reaped and temporary
files removed; compiler descendant or abrupt wrapper-death cleanup is not proven.
No credentials, real home, unrelated runtimes or hardware were touched.

The initial external volume has `noowners`, so it cannot establish owner security.
A second small run used a private temporary directory on ownership-enforcing
system APFS, without installing anything; the same bounded results held. Neither
run qualifies root validation, crash durability, Linux parity or support.

## Reference material

- [Apple APFS APIs, archived](https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/APFS_Guide/ToolsandAPIs/ToolsandAPIs.html)
- [Apple fsync and FULLFSYNC](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html)
- [Apple launch/library constraints](https://developer.apple.com/documentation/security/applying-launch-environment-and-library-constraints)
- [Apple sandboxed helper rules](https://developer.apple.com/documentation/xcode/embedding-a-helper-tool-in-a-sandboxed-app)

These identify candidate primitives, not proof of equivalence. App Sandbox is not
assumed to be a drop-in replacement for the existing lifecycle/resource contract.
