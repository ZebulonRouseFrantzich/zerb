# Darwin primitive feasibility spike (diagnostic only)

This disposable standard-library Python/C probe is **not** a product backend,
runner, fallback path, platform qualification, or installed-binary test. It does
not change the Linux implementation, public entrypoint, contracts, or gates.
It uses its own temporary directory, inert compiled fixtures and owned
lifecycle-test children. On the observed successful runs, those children were
reaped and temporary files removed. Compiler/linker subprocesses are not part
of the lifecycle proof, and abrupt wrapper-death cleanup is not qualified. No dependency installation is needed.

Run on Darwin arm64 with existing Apple clang and Python 3:

```sh
python3 tools/darwin-spike/probe.py <existing-disposable-parent-directory>
```

The parent must be a disposable test/build location, never a real HOME, manager
state, or destination. The script creates fresh private HOME/XDG/tmp roots inside
that parent and supplies a minimal explicit child environment. It does not run
Git, mise, chezmoi, user payloads or the legacy public binary.

## Scope and observed result

Base: PR48 `ae53bbb977a971d3f9dc9a7a87da02524d7291b6`.
Observed 2026-11-14 UTC: macOS 26.6.2 (25G83), arm64, Apple clang 21.0.0
(clang-2100.1.1.101), Python 3.9.6, external APFS.
The volume was mounted `noowners`: **owner/permission security is not qualified**.
A second run used a fresh private directory in system temporary storage on APFS
with ownership enabled; the same four diagnostics passed. It installed nothing
and removed its temporary artifacts. No mount, permission, system or credential
settings were changed.

- **ARCH-017/020; VAL-025 partial primitive diagnostic:** descriptor-relative
  `renameatx_np(RENAME_EXCL)` refused an existing target with EEXIST without
  modifying its bytes, then published at a fresh name. File `F_FULLFSYNC` and
  parent-directory `fsync` returned success. This is not power-loss evidence,
  complete root/topology validation, or a crash/recovery-state proof.
- **ARCH-017; VAL-025 partial primitive diagnostic:** `flock` blocked an
  independently opened contender, survived closing an unrelated descriptor,
  and admitted a contender after explicit unlock. This does not establish
  Linux OFD-lock equivalence, inherited-descriptor semantics, hard-death
  behavior, stable-inode security, or the complete initialization protocol.
- **ARCH-009 negative diagnostic:** opening/checking an inert Mach-O image and
  then spawning its pathname is insufficient. Replacing that path after open
  executed the replacement fixture (exit 32 instead of 31) while the checked
  descriptor retained the original inode. A pathname check alone cannot replace
  Linux's sealed-image execution boundary.
- **ARCH-010; VAL-020 negative diagnostic:** an owned child joined a managed
  process group, then used `setsid`; terminating the old group did not terminate
  it. The probe explicitly killed and reaped that direct child, and reaped the
  group leader. `killpg` alone cannot replace the cumulative lifecycle boundary.

All four bounded diagnostics completed as expected. `host_qualified=false` and
`release_supported=false`. This is not a closed-matrix qualification row and
must not promote any existing inconclusive candidate.

## Next decision and work

Keep one shared product/core and the Linux backend unchanged. Before production
Darwin code, settle a versioned Darwin contract for:

1. Race-resistant execution of the exact admitted Mach-O bytes, including loaded
   library policy and signing/launch constraints where applicable.
2. Cumulative child/resource containment and hard-parent-death cleanup. A
   cooperative process group is not equivalent to the current Linux contract.
3. Ownership-enforcing APFS roots, topology/mount identity, durable publication,
   stable lock semantics and recovery, tested on a qualified filesystem tuple.

Then test the selected boundaries adversarially before integrating adapters.
This spike adds no supervisor or platform abstraction because selecting one
before those answers would prematurely commit the architecture. Source-level
and native execution parity remain separate from full supported-platform parity.

Primary API references (documentation is not qualification):
- [Apple APFS APIs (archived)](https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/APFS_Guide/ToolsandAPIs/ToolsandAPIs.html)
- [Apple fsync manual and F_FULLFSYNC distinction](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html)
- [Apple launch/library constraints](https://developer.apple.com/documentation/security/applying-launch-environment-and-library-constraints)
- [Apple sandboxed helper embedding](https://developer.apple.com/documentation/xcode/embedding-a-helper-tool-in-a-sandboxed-app)

Independent source/documentation review found no blocking issue after narrowing
cleanup claims and removing unrelated project context. The reviewer did not rerun
the diagnostic. No production integration
or support claim follows from it.
