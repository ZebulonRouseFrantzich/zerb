# Redacted Proof Results

**Status:** Evidence ledger; never normative authority  
**Current evidence state:** Phase 0 baseline only. No Phase 1 proof has run or passed.  
**Contract owners:** [Product contract](product-contract.md), [architecture](architecture.md), and [validation](validation.md) define requirements and gates. This file records observations and resulting contract edits; it cannot create support.

See the [roadmap index](README.md), [delivery plan](delivery-plan.md), [hard-cutover disposition](hard-cutover.md), and [post-MVP intent](post-mvp.md).

## Ledger rules

1. Records are append-only. Correct an earlier record by appending a new record that identifies the superseded record; do not rewrite the observation.
2. A record distinguishes **observed**, **failed**, **inconclusive**, and **not run**. Command success alone is not proof of an unstated property.
3. Every Phase 1 record names the requirement IDs it tests and the assumption it was capable of rejecting.
4. A fixture value is recorded verbatim only when its complete preimage and context are deliberately synthetic or already public, unrelated to a real person/organization/account/target, and explicitly reviewed as non-sensitive. A potentially enumerable locator, profile/employer/provider name, target path, candidate, tree, fixture revision, or comparable metadata uses an opaque random per-proof correlation reference.
5. Generated high-entropy marker redactions have the shape `RENDERED_MARKER{fp=sha256:<64-lowercase-hex>}` or `AMBIENT_MARKER{fp=sha256:<64-lowercase-hex>}`. The MVP generates no authentication/provider marker. Potentially low-entropy metadata uses typed random references such as `SENSITIVE_LOCATOR{ref=loc-...}`, `CANDIDATE_IDENTITY{ref=candidate-...}`, `TREE_IDENTITY{ref=tree-...}`, and `FIXTURE_IDENTITY{ref=fixture-...}`.
6. Raw rendered and ambient marker bytes follow the closed sink matrix below. Provider and authentication values/markers are not generated or admitted. No raw marker enters this ledger, an artifact locator, a retained transcript, or a retained capture.
7. A retained record expresses sensitive identity only through typed-reference equality/change relations. A literal digest is allowed only after complete-preimage review classifies it non-sensitive or already fully redacted. Selected-gitlink/private-transport records contain only rejection and zero-access observations; they never contain a transport tuple, approval digest, credential recipe, agent/process identity, relay identity, or private endpoint.
8. A conclusive Phase 1 record includes the exact upstream version and canonical executable identity, reviewed public source or correctly classified static-locator reference, fixture identity/revision relation, process tree, access trace, write set, resource observations, and applicable root/lifecycle schedule. An exclusion record additionally states the rejection boundary and zero authentication/network/candidate/manager/mutation observations.
9. Proof artifacts remain in approved redacted evidence storage outside every product root and repository. This ledger records stable typed locators and, only when the complete retained artifact is already redacted and reviewed, its literal artifact digest. An artifact digest authenticates that reviewed redacted artifact; it is never a fingerprint of removed sensitive metadata.
10. Each record states scan scope precisely. “ZERB repository” means the complete project worktree, including tracked, untracked, ignored, and repository-metadata files, plus the index and reachable committed content. “Fixture repositories/trees” means every proof-created fixture worktree, repository metadata/object store, fetched object store, materialized source/effective-source/generated-config tree, destination, and retained artifact tree. A generated rendered-marker source input must be disposable, outside every repository, and explicitly listed as an allowed sink. The record names exclusions and their justification; an unscanned or ambiguously scoped tree makes the marker/non-access result inconclusive.
The allowed-sink matrix is exact; every unlisted channel is forbidden:

| Marker class | Raw bytes allowed | Required absence |
|---|---|---|
| Authentication | Nowhere; no marker is generated and no helper/agent store, live protocol, relay, credential input, or private endpoint exists in the MVP | Every surface |
| Rendered content | Generated disposable source input outside every repository, live render channel, intended owner-validated `0600` disposable destination, and in-memory comparison only | Authentication storage, ZERB durable/cache/log/evidence surfaces, unintended destination, output, capture, transcript, review, evidence ledger, and ZERB repository |
| Provider | Nowhere; no marker is generated and any occurrence fails because the MVP has no provider channel | Every surface |
| Ambient | Live poisoned-parent injection and in-memory comparison only | Every child channel and every filesystem, network, output, capture, transcript, review, evidence, and repository surface |


## Phase 1 evidence record template

Every field below is explicitly a **Phase 1 evidence field**. A result is inconclusive until every field relevant to its requirement has a recorded value or a reason the validation owner accepted as not applicable.

### PHASE 1 EVIDENCE RECORD — record identifier

- **PHASE 1 EVIDENCE FIELD — date/time and operator:** UTC time; operator/reviewer identity.
- **PHASE 1 EVIDENCE FIELD — status:** `observed`, `failed`, `inconclusive`, or `not run`.
- **PHASE 1 EVIDENCE FIELD — requirement IDs:** Exact `PC-*`, `ARCH-*`, and `VAL-*` IDs tested.
- **PHASE 1 EVIDENCE FIELD — proof candidate:** Exact non-support candidate row from `validation.md`, its `not run`, `failed`, `inconclusive`, `implementation-qualified`, or `exclusion-proven` Phase 1 disposition, and an explicit statement that neither the row nor this record is support evidence.
- **PHASE 1 EVIDENCE FIELD — proof slice and scenario:** Native Git/source, managed adapters/lifecycle/shell, filesystem/state, or schema/parser; exact scenario and boundary.
- **PHASE 1 EVIDENCE FIELD — upstream identity:** Product name, exact upstream version, canonical executable path or source module, and canonical executable/module identity. Retain a literal digest only after complete-public-preimage review; otherwise use a typed identity reference. Provider and private transport mechanisms are not support fields.
- **PHASE 1 EVIDENCE FIELD — source/fixture identity:** Reviewed non-sensitive public source URL or typed static-locator reference plus fixture identity and typed revision/tree relations. State whether selection was embedded or a gitlink rejection case. A gitlink locator may be recorded only as reviewed public fixture data or an opaque reference and never as a candidate identity.
- **PHASE 1 EVIDENCE FIELD — invocation shape:** Absolute executable identity and argv array, stdin/TTY shape, working-directory class, descriptor allowlist, umask, cancellation/signal injection, concurrency shape, and exact spawn-boundary schedule. Record explicitly that no transport-authorization flag/input was accepted.
- **PHASE 1 EVIDENCE FIELD — spawn/descriptor race:** Close-on-exec creation evidence; concurrent descriptor churn; parent/supervisor descriptor tables; close-range/allowlist result; parent-identity/readiness steps; hard-death boundaries; and survivor/reaping result.
- **PHASE 1 EVIDENCE FIELD — environment shape:** Sorted allowed variable names, explicitly cleared variable classes, synthetic/real-home class, ambient marker fingerprints, and opaque references for sensitive metadata. Explicitly record authentication/helper/agent/relay/private-endpoint and provider/host-fact channels as absent.
- **PHASE 1 EVIDENCE FIELD — selected-gitlink/private-transport exclusion:** Locator classification and selected/unselected status; exact rejection/static-validation boundary; zero DNS/endpoint resolution, executable/helper/agent/relay/credential/authentication/network/candidate/manager/mutation observations; absence of typed identity producer, transport tuple/digest/prompt/output, and durable transport field. This field never describes a positive mechanism.
- **PHASE 1 EVIDENCE FIELD — observed process tree:** Parent/supervisor/child/session/group/subreaper topology, executable identities, ready handshake, parent-identity checks, exit/signal transitions, escalation, reaping, hard-death injection boundary, and surviving descendants.
- **PHASE 1 EVIDENCE FIELD — exec resolution/race:** Bare or canonical-absolute input class; canonical stable-profile `PATH` directory identities and rejection cases; ambient PATH/cwd same-name collisions; opened and executed executable path/inode/owner/type/mode identity; fd-based or equivalent race-resistant handoff; substitution schedule; and exact rejection/launch observation. A digest is literal only under rule 7; otherwise use typed identity and equality relations.
- **PHASE 1 EVIDENCE FIELD — access trace:** Filesystem/network/helper/agent/relay/private-endpoint reads, executions, authentication and endpoint attempts, and explicit non-access assertions backed by the trace. Selected-gitlink/private-transport and provider/host-fact rejection scenarios record every prohibited class as zero.
- **PHASE 1 EVIDENCE FIELD — write set:** Every created/modified/removed path represented as a reviewed non-sensitive fixture path or opaque typed target reference, with mode, owner, link/inode facts, timing boundary, root class, and outside-sentinel result.
- **PHASE 1 EVIDENCE FIELD — resource and boundary observations:** Frozen limits and observed transfer, disk, object, expanded-byte, graph/tree/entry, output, time, CPU, memory, process, and allocation values; enforcement mechanism; `limit-1`/`limit`/`limit+1` outcome; interruption/cleanup result; and the exact spawn, initialization, takeover, persistence, or execution-resolution boundary exercised.
- **PHASE 1 EVIDENCE FIELD — root fixture/classification:** Exact state/data/cache directory-entry and schema/domain shape, typed fixture/file-content identities unless rule 7 permits literal bytes or digests, inode/link/type/owner/mode/ancestor facts, applicable disjoint VAL-024/026 class and precedence, command admission, guidance, and complete write set.
- **PHASE 1 EVIDENCE FIELD — initialization/takeover:** Stable `init.lock` path/inode/mode/sync and open-file-description lock identity; contenders and sole holder; persisted instance/token/status/process identity; dead-claimant/PID-reuse proof; topology and compare result; old/new takeover token replacement and sync boundary; simultaneous-takeover outcome; injected death boundary; resulting authority/admission/write set; and proof that the lock inode was not replaced or deleted.
- **PHASE 1 EVIDENCE FIELD — marker allowed-sink scan:** Marker classes introduced, intended sink(s), required presence observations, all scanned surfaces, high-entropy one-way fingerprints, and required absence observations under the exact matrix above. Authentication and provider rows record that no marker/value was introduced and no channel existed.
- **PHASE 1 EVIDENCE FIELD — repository/tree scan:** Exact ZERB worktree/index/reachable-content scope and every fixture repository, object store, materialized/effective/generated tree, destination, and retained-artifact tree scanned; explicit allowed rendered-source/destination exceptions; exclusions and justification. Never retain raw matching bytes.
- **PHASE 1 EVIDENCE FIELD — result:** Exact observations, failed expectation, and whether the named requirements were proven, disproven, or remain inconclusive.
- **PHASE 1 EVIDENCE FIELD — rejected assumption:** The plausible unsafe or incompatible behavior this run could distinguish.
- **PHASE 1 EVIDENCE FIELD — resulting contract edit:** Requirement IDs and exact normative change, or `none` with a reason. Evidence never edits its own authority.
- **PHASE 1 EVIDENCE FIELD — artifact references:** Redacted retained transcript/trace/process-tree/write-set/resource-matrix locators and, only after complete-preimage review, artifact digests. Artifact references contain neither raw marker bytes nor dictionaryable candidate/tree/fixture/static-locator fingerprints, and never contain a transport tuple, credential recipe, authorization digest, identity producer, or private endpoint detail.
- **PHASE 1 EVIDENCE FIELD — review:** Owner approval and independent plan/security review disposition where the gate requires them, including explicit review of literal non-sensitive fixture classifications and repository/tree scan scope.

## BASELINE-2026-08-13 — pre-cutover branch health

- **Status:** Observed baseline only; not a Phase 1 proof and not a support claim.
- **Related requirements:** [PC-001](product-contract.md#pc-001--product-boundary-and-provisional-status) and [PC-070](product-contract.md#pc-070--evidence-gated-support-state) use this result only to describe current status.
- **Branch/worktree observation:** Branch `project_refocus_and_security_fixes` was clean at baseline.
- **Invocation shapes:**
  - `go test ./...`
  - `go test -race ./...`
  - `go vet ./...`
  - `golangci-lint run`
  - `go build -o bin/zerb ./cmd/zerb`
  - `./bin/zerb --version`
  - `./bin/zerb`
- **Environment shape:** The baseline used the developer environment. Its allowed-variable shape, ambient-marker fingerprint, descriptor set, umask, and process isolation were not captured, so this record cannot prove isolation or secret non-interference.
- **Upstream and fixture identity:** Not applicable to the recorded baseline health check. It did not run the pinned native Git, mise, chezmoi, filesystem, lifecycle, parser, or static provider/host-fact rejection fixtures and recorded no canonical upstream executable digest.
- **Process/access/write evidence:** No process-tree, access trace, write allowlist, resource matrix, outside sentinel, repository/tree scope, or marker allowed-sink scan was captured. The build produced `bin/zerb`; this record makes no stronger write-set claim.
- **Observed results:**

  | Check | Baseline result |
  |---|---|
  | Clean branch check | Passed; no baseline worktree changes |
  | `go test ./...` | Passed |
  | `go test -race ./...` | Passed |
  | `go vet ./...` | Passed |
  | `go build -o bin/zerb ./cmd/zerb` | Passed |
  | `golangci-lint run` | Failed with 54 old-code issues: 26 `gosec` and 28 `staticcheck` |
  | Built binary version | Reported `v0.0.1-alpha` |

- **Observed old command table:** The reachable binary advertised `--version`, `init`, `uninit`, `activate <shell>`, `drift [options]`, `config add [options]`, `config list [options]`, and `config remove [options]`. Its help separately labeled `add` and `sync` as coming soon; those labels are not target behavior or evidence of implementation.
- **Rejected assumptions:** The branch did not have a clean lint gate, and the reachable binary did not expose the provisional target command grammar in [PC-010](product-contract.md#pc-010--exact-provisional-cli-grammar).
- **Resulting contract edit:** None. The observation establishes the pre-cutover baseline and supports the warning that current behavior is obsolete; it does not freeze or prove a target contract.
- **Phase 1 status:** Native Git/source, managed adapters/lifecycle/shell, filesystem/state, and schema/parser proof slices are all **not run**.
