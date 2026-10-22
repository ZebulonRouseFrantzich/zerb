# Agent Guidelines for ZERB

## Normative authority

Read the relevant roadmap owner before planning, implementation, or review:

1. `docs/roadmap/product-contract.md` owns `PC-*` public CLI, schema, digest, approval, output, and user-visible behavior.
2. `docs/roadmap/architecture.md` owns `ARCH-*` trust, process, filesystem, root, cache, durable-state, and recovery invariants.
3. `docs/roadmap/validation.md` owns `VAL-*` support claims, proof requirements, and evidence links.
4. `docs/roadmap/delivery-plan.md` owns the current phase, ordering, orchestration, and gates.
5. `docs/roadmap/hard-cutover.md` owns artifact and dependency disposition.
6. `docs/roadmap/post-mvp.md` is non-committed intent, not an implementation contract.
7. `docs/roadmap/proof-results.md` is append-only redacted evidence, never normative authority.

Every implementation and review claim must cite applicable requirement IDs. Resolve conflicting prose in favor of the document that owns that contract, and fix the non-owning document rather than creating a second convention. Phase 0 contracts are provisional. Phase 1 may freeze only implementation-qualified proof candidates; it does not create support. The owner-approved split gate in delivery-plan.md permits reviewed non-release Phase 2–4 implementation after passing current source-bound cloud conformance and independent plan/security review. This leaves all unqualified contracts provisional, requires focused replacement tests/race checks and phase reviews, and never waives required runtime protections or admits a development fallback runner. Real frozen-host Phase 1 evidence, review and freeze approval remain mandatory before release/public cutover; support still requires VAL-001 and complete Phase 6 installed-binary acceptance.

## Toolchain and commands

- **Go version:** Go 1.24.
- Before Go checks/builds in a fresh checkout: `python3 scripts/check_parser_pin.py --prepare --go /absolute/path/to/pinned/go` (Linux, Python standard library; offline).
- Full unit/integration suite: `go test ./...`
- Focused test: `go test -run TestName ./path/to/package`
- Race suite: `go test -race ./...`
- Static analysis: `go vet ./...`
- Lint: `golangci-lint run`
- Replacement import gate: `go run ./tools/importcheck`
- Current foundation race gate: `go test -race -count=1 ./internal/profile ./internal/receipt ./tools/importcheck`
- Local parser pin: `python3 scripts/check_parser_pin.py --go /absolute/path/to/pinned/go`
- Patched parser upstream race gate: `go -C third_party/go-toml test -race -count=1 ./...`
- Release-shaped build: `go build -trimpath -o bin/zerb ./cmd/zerb`

Run the narrowest check that proves the change while iterating, then the phase-required gate once after integration. Do not weaken a gate to match a baseline failure. Tests and proof harnesses must use disposable HOME/XDG roots and must never address the developer's real home, credentials, manager state, destinations, agents, or configuration.

The temporary owner-approved TOML fix drops all decoder-owned storage before pooling. Its authentic upstream ZIP, minimal patch, focused regression and license are checked in; `third_party/go-toml` is an ignored, exactly reconstructed build input. Preparation verifies before atomic no-clobber publication and refuses an altered existing tree; it never downloads or repairs by overwriting. The normal pin check remains read-only. `go mod verify` skips local replacements, so retain the separate source check and upstream tests. `Parser.OriginCommit` denotes the upstream base; `ModuleSum` binds the unchanged exact patched identity. Never automatically rewrite pins. Follow `third_party/README-go-toml.md` to retire this workaround after a reviewed upstream release passes the retention and parser corpus.

## Go implementation rules

- Prefer standard-library and explicit concrete types. Add a dependency only when a frozen requirement needs it and its owner, version, checksum, and purpose are recorded.
- Keep imports grouped as standard library, third party, then local packages. Use ordinary Go naming and accept common abbreviations such as `ctx` and `err`.
- Wrap causal errors with `fmt.Errorf("context: %w", err)`. Never ignore errors. Cleanup errors must not hide the primary failure.
- Public command failures use the exact `Error: ` stderr prefix and the exit contract in `product-contract.md`; do not leak an internal tool, package, locator, target, provider/profile name, or sensitive value through formatting.
- Accept `context.Context` for cancellation at blocking or externally controlled boundaries. Do not replace the frozen process-lifecycle policy with `exec.CommandContext`.
- Use table-driven tests for behavioral boundaries and transitions. Test public outcomes, invariants, precedence, and real errors rather than source layout or mocks of plumbing.
- Avoid avoidable allocations, copies, map-based canonical encodings, reflection-heavy abstractions, and interfaces with only one production implementation. Keep package boundaries boring and explicit.
- Public subcommands use `flag.FlagSet` with `ContinueOnError`; do not add a CLI/configuration framework.

## Clean-cutover boundary

- The cutover is breaking and pre-alpha. Do not add a compatibility alias, importer, state migration utility, feature flag, second binary, build-tagged product, deprecated type, fallback path, or mixed public router.
- ARCH-030's replacement packages must not import, alias, wrap, re-export, register, or accept a type from any forbidden full old tree: `internal/config`, `internal/service`, `internal/git`, `internal/drift`, `internal/chezmoi`, `internal/transaction`, `internal/shell`, `internal/platform`, `internal/binary`, or `internal/testutil`. Audit useful concepts, then rewrite them behind replacement-owned types. The import-DAG gate scans production and `_test.go`; old-tree imports fail in both, and `internal/testenv` is `_test.go`-only and forbidden from production.
- Phase 1 uses disposable proof harnesses and does not depend on `internal/app.Run` or the target installed binary. During Phases 3–5, keep the current public entrypoint unchanged and exercise the exact non-public ARCH-030 `internal/app.Run` seam. Phase 6 replaces the entrypoint and deletes all obsolete source, tests, fixtures, examples, dependencies, docs, and prompts atomically.
- Do not add an executable configuration DSL, source-rewriting command surface, custom reconciliation engine, automatic source commit, or parent-shell mutation. Native Git and managed tools are subprocesses behind the frozen runner and complete environment policy.
- Remove obsolete code at the cutover; do not retain it as a reference package. An old test is deleted when it defends rejected behavior, not mechanically ported.

## Trust and source boundaries

- Public source input is explicit, credential-free, canonical absolute HTTPS. Reject local, ambient, ambiguous, credential-bearing, redirected, or non-canonical sources before access.
- Use only the Phase 1 implementation-qualified native Git executable through `internal/execenv`. Disable ambient repositories, configuration, hooks, filters, helpers, prompts, and redirects unless a frozen phase explicitly supplies an approved value.
- Candidate trust and mutation approval are the only MVP approval capabilities. Non-interactive missing approval returns before its manager or mutation boundary. There is no transport approval flag, prompt, digest, or tuple.
- The sole positive source surface is canonical credential-free public HTTPS with embedded base and embedded selected profile. Unselected canonical HTTPS/SSH gitlinks may be statically validated but must receive zero executable, helper, agent, relay, authentication, DNS/endpoint, network, candidate, or materialization access. Selecting any gitlink/private source must fail before authentication, network, candidate creation, manager execution, or mutation. Never add or accept a credential helper, askpass program, SSH agent/socket, typed agent/process identity, identity/key file, known-hosts input, credential store selector, synthetic SSH config, proxy, relay, private endpoint, or transport authorization.
- Materialize source objects without a worktree and admit only frozen ordinary-file, executable-file, and directory forms from embedded components. Reject selected/nested gitlinks, traversal, aliases, links, devices, unsupported modes, duplicate normalized paths, and limits violations before hashing or parsing.
- An authored mise version and every required lock value must already be exact, statically valid, and candidate-bound before trust and before mise or any other candidate-selected manager execution. Reject aliases, channels, ranges, `latest`, missing locks, and live lock generation; never mutate a trusted candidate digest with a post-trust resolution.
- The only pre-trust structural-adapter execution is PC-031's verified chezmoi read-only, non-rendering, machine-readable target mapping over the protected effective source. Freeze and prove exact executable/argv/environment/descriptors/working/config/cache/state/synthetic-destination roots, deterministic structural output, no template/helper/provider/host-fact/script/external/candidate-content execution, and no real-destination access/write. Before trust, compare only normalized structural target text against normalized reserved-role paths/future leaves; bind target identities and the revalidation policy into the candidate. If upstream cannot prove every mapping property, fail/remove the surface; never build a fallback mapper. Only after trust and mutation authorization under `A-X+W-X`, open the real destination and perform full canonical/descriptor/owner/type/link/device/inode/mount/bind checks before manager/filesystem mutation and at every affected boundary.

## Process isolation

- Managed processes and shells use an identity-checked absolute executable, argv array, working directory, descriptor allowlist, complete constructed environment, and frozen process class. `exec` may additionally accept the bare-name form below. Never invoke a shell or mutate inherited environment in place.
- All ZERB-created descriptors are close-on-exec from creation. Every retained process class launches through the Phase 1-proven same-binary FD-only supervisor with atomic FD allowlisting and spawn synchronization; add the frozen cgroup boundary when that class needs it. The supervisor is mandatory and the cgroup is cumulative, never an alternative primitive or optional escalation. Sanitize the descriptor table to declared streams/control descriptors, synchronize parent identity, parent-death setup, lifecycle setup, and readiness, and only then release candidate code. Concurrent parent descriptor churn must be unable to add a child descriptor; an enumerate-close-then-spawn sequence without equivalent race proof is forbidden.
- Resolve an `exec` bare name only against the already-constructed stable-profile `PATH`. Its entries are canonical absolute owner-validated directories with no empty, relative, dot, duplicate, or writable-unowned component; ambient `PATH` and the current directory never participate. Open and verify the final executable, then execute that same identity through the frozen fd-based or equivalently race-resistant boundary. Reject any symlink, inode, digest, owner, type, mode, or path substitution before execution. Phase 1 must prove resolution-to-exec identity and substitution-race resistance. A slash-containing payload path must be canonical absolute.
- Credentials and authentication channels are absent from the MVP. Never accept credentials, create a helper/agent/relay store or protocol, place secret material in argv/environment/configuration/state, or add a private transport fixture recipe. Provider references, processes, channels, and values are likewise excluded.
- Implement the frozen per-class process group/session, parent-death, forwarding, grace/escalation, leader/descendant reaping, exit mapping, and orphan-detection policy inside that mandatory same-binary supervisor, plus the frozen cgroup where required. A catchable cancellation must leave no managed descendant.
- Managed Git, acquisition, mise, and chezmoi processes may not daemonize or escape the supervisor/cgroup boundary. User payload detachment is outside guarantees only if `product-contract.md` says so explicitly and `VAL-*` evidence proves no manager credential or state path is supplied.
- Do not mutate the parent shell. Execution and shell commands create clean child processes through the same runner and pass through the frozen statuses.

## Filesystem, root, and state invariants

- Production captures home and the raw environment exactly once into immutable copied `paths.Input`. `app.Options` carries that input and only side-effect-free service constructors, never resolved roots or active root services. `app.Run` parses and completes root-independent local checks first: invalid argv and both version forms return without `paths.Resolve` or constructor activation; each well-formed non-version command calls `paths.Resolve` exactly once, passes that single `paths.Roots` to constructors, and only then activates root services. The MVP neither creates nor reads a ZERB-specific XDG configuration root and has no root-override flag or environment variable.
- Validate every ancestor and object with no-follow owner, type, and mode checks. Create directories as `0700` and durable files as `0600` independent of umask. Use same-filesystem atomic replacement plus file and parent-directory sync where `ARCH-*` requires durability.
- State and data markers share one cryptographically random instance ID. First use begins with an exclusive, durable initializing claim; only its ownership-token holder may create the matching data and cache-instance markers. A losing initializer writes nothing.
- Accept durable roots only as a matching current pair. Foreign, missing, mismatched, malformed, initializing, dirty, or corrupt state fails closed with only the admissions defined by `architecture.md`.
- Cache is untrusted and disposable. Address only the owner-validated per-instance cache directory, validate topology on every access, and authenticate/hash every byte before parse or execution. Never adopt or delete unrelated cache content.
- Writer locks use random ownership tokens and proven process identity, not age alone. While `A-X+W-X` remains held, release verifies the exact stable writer-lock inode and current owner inode/generation/token, atomically publishes and file/parent-syncs `released(next-generation)`, reopens and verifies it, then unlocks writer and admission in order. `init.lock`, `admission.lock`, and `writer.lock` are stable synchronization inodes: never unlink or replace them. `writer-owner.json` is atomically replaced and never unlinked.
- Persist and sync the candidate and journal before mutation; persist live destination observations as journal/receipt preconditions only after trust and mutation authorization; revalidate them at affected mutation boundaries; persist the exact mutating phase before each possible external mutation; write a pending receipt; atomically commit the stable current reference; then clean exact stale journal data.
- Recovery is truthful forward convergence. Ordinary planning, apply, execution, and shell paths fail while state is dirty; only health and supported recovery paths are admitted. Never infer success from an orphan receipt, rewrite a journal manually, claim rollback, or give recognized dirty state generic delete/start-over guidance.

## Digests, schema, and sensitive data

- Follow `PC-050`'s versioned, map-free canonical digest structs. Normalize and sort every included collection exactly as specified; exclude timestamps, availability, approvals, and other run-local observations. Authentication, provider, rendered, ambient, and host-fact values are never digest inputs.
- Do not confuse prohibited values with identity-bearing sensitive metadata. Protected candidates, receipts, and their digest inputs retain only contract-required canonical credential-free public source, embedded logical profile/component identities, paths, adapter/policy identities, and other `ARCH-004`/`ARCH-024` fields. Gitlink locators remain public-manifest static-inspection input and never enter a candidate/receipt. Redact protected identity only on public output and exported evidence.
- Strictly decode bounded schema documents using the frozen parser/version and `DisallowUnknownFields`. Reject over-limit bytes before decode and reject duplicate, ambiguous, aliased, unknown, malformed, or unsupported representations.
- Apply this closed proof-marker sink matrix; every unlisted sink is forbidden:

  | Marker class | Raw bytes allowed | Required absence |
  |---|---|---|
  | Authentication | Nowhere; no marker is generated and no helper/agent store, relay, private endpoint, credential input, or live protocol exists in the MVP | Every surface |
  | Rendered content | Generated disposable source input/live render channel, intended owner-validated `0600` disposable destination, and in-memory comparison only | Authentication storage; ZERB durable, cache, log, and evidence surfaces; unintended destinations; output, capture, transcript, review, and repository |
  | Provider | Nowhere; any occurrence fails because the MVP has no provider channel | Every surface |
  | Ambient | Live poisoned-parent injection and in-memory comparison only | Every child channel and every filesystem, network, output, capture, transcript, review, evidence, and repository surface |

- After success, failure, catchable cancellation, and hard death, scan every controlled process channel, root, stream, capture, destination, repository/evidence surface, and lifecycle artifact for all marker classes. A transient occurrence in a forbidden sink fails even if cleanup later removes it.
- In retained proof evidence, plain SHA256 is permitted only for independently generated high-entropy non-authentication markers or when a reviewer has examined the entire digest preimage and classified it as non-sensitive or already fully redacted. Sensitive or dictionaryable candidates, trees, fixture revisions/digests, static locators, names, and paths use fresh typed random per-proof references plus equality/change relations. Selected-gitlink/private-transport evidence records exclusion and zero access, never a tuple, approval digest, credential recipe, or typed identity producer.
- Redaction is active and causal: sanitize structured sensitive metadata before formatting or wrapping. Never rely on callers to scrub an already constructed error string.

## Review and delivery gates

- Review against the requirement owner, current delivery phase, and the disposition table. A missing ID, unsupported claim, widened grammar, unclassified artifact, or implementation outside its admitted delivery gate is a blocking defect.
- Cloud development admission requires current source-bound conformance plus independent plan and focused security review; an inventory-only report cannot admit work. Phase 1 qualification/freeze separately requires owner approval, independent plan review, and focused security review after the complete real frozen-host proof matrix. Missing cgroups, namespaces, filesystem or exact tool/CA prerequisites must fail closed; they never authorize a fallback or qualification claim.
- Phase 2 requires focused security review of native Git policy, environment/descriptor handling, archive/root primitives, lock ownership, state transitions, canonical digests, and receipt completion before mutation integration.
- Provider references, functions, processes, storage, channels, and values are excluded from the MVP. A disposable embedded fixture must fail static validation before candidate creation. Selected-gitlink input must fail earlier before authentication, network, materialization, or validator entry. Future provider or private selected transport support requires new reviewed contracts and real evidence; never weaken an MVP exclusion or treat a mock as support evidence.
- Phase 6 requires the atomic public/source cutover, complete installed-binary and failure-matrix acceptance, final tracked/import/dependency/artifact sweeps, DeepReview, and security review with every finding resolved.
- Never record proof as successful unless the exact executable, fixture, argv/environment shape, process/access/write observations, and result were actually exercised and redacted according to `validation.md`.
