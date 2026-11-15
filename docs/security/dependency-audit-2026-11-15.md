# Dependency security audit — 2026-11-15

## Scope and provenance

Bounded audit of ZERB main `311dae5364787de9e1bb7f995efa63d5b5312ef4`, Go module graph, application/test imports, and built Linux amd64 binary. Main and remote branches were reconciled before edits. Open PRs #35–#48 form the separate foundation/proof stack targeting `project_refocus_and_security_fixes`; this branch is based on main and does not modify or integrate that stack.

Scanner: `govulncheck v1.9.0`. Database: official [golang/vulndb snapshot](https://github.com/golang/vulndb/tree/e91d8491530fd8782760dd3916cd2708182b9f10/data/osv), commit `e91d8491530fd8782760dd3916cd2708182b9f10`. The hosted endpoint was inaccessible locally, so scans used the scanner's native local OSV-directory support, without filtering records. This snapshot contains zero-valued published/modified timestamps; the commit identifies the actual input, not a claimed database freshness timestamp. CI uses the live default database.

Baseline source/module scans found **96 distinct advisories** (108 advisory/module pairs, because some affect both stdlib and x/net). Final module, source, test-inclusive source and binary scans found **zero findings** against that snapshot. Static call-graph reachability is conservative and is not proof that every reported transport/server path is reachable through the CLI.

## Changes and compatibility

- Go minimum: 1.24.0 → **1.26.11**; local baseline compiler was 1.24.9. Latest patched x/net requires Go 1.26; Go 1.24 is no longer a sufficient patched build baseline. CI derives its compiler version from go.mod. No module-major/API migration or application-source change.
- go-git/v5: 5.16.3 → **5.19.2**; go-billy/v5: 5.6.2 → **5.9.0**. Includes Git object/index validation, filesystem traversal/symlink and transport fixes. Existing Git initialization/staging/commit tests pass.
- sigstore: pseudo-version 1.9.6 → **1.10.9**, fixing its legacy TUF cache traversal. Its signature/cryptoutils dependency changes remove boulder and go-jose from the application import graph after tidy; go-jose 4.1.4 remains selected in the complete module graph. Existing GPG/signature tests pass. The legacy TUF package itself was not imported by ZERB.
- circl: 1.6.1 → **1.6.3**; x/crypto: 0.42.0 → **0.59.0**; x/net: 0.43.0 → **0.62.0**. Other indirect changes follow minimum-version selection and tidy. cpuid/v2 is a required transitive dependency of the upgraded SHA-1 implementation, not a new application feature.
- CI linter: 2.6.1 → **2.15.0** for the newer compiler, with all existing security checks/configuration intact. Added pinned govulncheck 1.9.0 source scanning. Race detection, coverage, permissions and lint timeout are unchanged.

## Dependency paths and exposure

| Module | Path from ZERB | Exposure |
|---|---|---|
| go-git/v5, go-billy/v5 | cmd/zerb → internal/git → go-git → filesystem/storage | Production local repositories, staging and commits. CLI currently does not clone/fetch/push; scanner transport paths remain conservative. |
| circl, x/crypto | internal/binary → ProtonMail/openpgp → internal/ecc → circl; go-git → SSH → x/crypto | Production signature verification; SSH transport imported but not exercised by current local-only CLI operations. |
| sigstore | internal/binary → sigstore/pkg/signature | Production signature verification. Vulnerable legacy TUF cache package was module-only exposure. |
| go-jose/v4 (baseline) | internal/binary → sigstore/signature → cryptoutils → boulder/goodkey → boulder/core → go-jose | Baseline production import chain; removed from runtime imports by upgraded sigstore. |
| x/net | internal/git → go-git/utils/ioutil → jbenet/go-context/io → x/net/context | Module-only vulnerable HTML/DNS/HTTP2 packages in baseline; standard-library HTTP2 is separately reachable. |
| x/sys | platform → gopsutil/host; go-git and crypto filesystem support | Runtime module; reported Windows-only API was not imported on Linux amd64. |
| stdlib | internal/binary download/verification → net/http, TLS, x509, archive/tar; general filesystem code | Production runtime. Module-only template/mail/zip findings do not imply CLI use. Compiler/toolchain fixes also arrive through Go upgrade. |
| grpc (baseline) | sigstore → boulder/goodkey.test → boulder/web → boulder/errors → grpc/codes | Upstream dependency-test path, not ZERB runtime. Removed direct indirect requirement after tidy. |

## Full selected-graph follow-through

The normal govulncheck scans above cover modules supplying loaded packages, not every unused upstream requirement. A separate audit joined `go list -m -json all` with every non-withdrawn OSV SEMVER interval in the same official snapshot, using `golang.org/x/mod/semver.Compare`. It found 83 baseline advisory/module pairs (excluding stdlib, which is covered above), and **zero final matches**. This graph check is distinct from source reachability and from a scan of the developer tools' own binaries.

Five explicit indirect security floors keep unused selected requirements patched: compress 1.20.2, OpenTelemetry API and SDK 1.45.0, grpc 1.83.2, and x/mod 0.42.0. `go mod why -m` reports these are not needed by the main module. Their paths are upstream requirements: sigstore → containerregistry → compress; sigstore/genproto → grpc → OpenTelemetry; x/crypto → x/mod. The SDK requirement arises through upstream telemetry dependencies. These do not add imports or telemetry to ZERB. x/mod 0.42.0 is also required by the selected x/crypto; Go minimum-version selection must not be allowed to downgrade x/crypto/x/net to retain an older x/mod.

Tidy was run before adding these deliberate floors. **A subsequent `go mod tidy` can remove them**: restore the floor block and rerun the complete graph audit after dependency maintenance. CI's source scan does not enforce these unimported graph floors. No replaces, excludes or advisory suppressions are used. Intermediate selection of OpenTelemetry 1.41/1.44 also exposed GO-2026-5158 (baggage parsing) and GO-2026-6505 (exporter configuration logging); both API and SDK are therefore at 1.45.0.

Independent review confirmed Sigstore's changed `ValidatePubKey` API is not called on ZERB's path: x509 parsing requires ECDSA before `signature.LoadVerifier`. LoadVerifier/ECDSA implementation files are unchanged between 1.10.4 and 1.10.9. Existing verification and sandbox tests passed independently; the reviewer did not rerun the entire suite.

## Validation and remaining gates

- PASS: complete `go test -race -coverprofile=... ./...`, Go 1.26.11. Full tests required the normal 022 umask and temporary sandbox write access to the synthetic files used by the existing home-directory symlink test. Earlier read-only-home failures reproduced on baseline; no assertions/skips or host permissions were changed.
- PASS: `go vet ./...`, `go mod verify`, build and CLI help smoke test. Local worktree build used explicit GIT_DIR/GIT_WORK_TREE to avoid the environment's placeholder ancestor `.git`; VCS stamping stayed enabled.
- PASS: module, source, test-inclusive and built-binary vulnerability scans. Reproduction: `govulncheck -db file:///path/to/vulndb/data/osv ./...`, `govulncheck -C cmd/zerb -scan module -db ...`, `govulncheck -test -db ... ./...`, and `govulncheck -mode binary -db ... /path/to/zerb`.
- FAIL: lint 2.15.0 reports **51 findings**: 23 gosec (22 G703 path-taint findings plus one G101 synthetic credential fixture) and 28 staticcheck QF1012 formatting suggestions. The complete output is byte-identical when running the same linter/compiler on unchanged main. No exclusions, suppressions or source rewrites were added. Source-security triage remains separate work; this draft is not merge-ready.
- Coverage: 58.5% total; existing packages below the repository's >80% goal remain below it (CLI 26.2%, binary 71.6%, drift 77.3%, service 18.9%, shell 74.2%, transaction 0%). Coverage settings were not weakened.
- Remote exact-head CI and independent review status are recorded in the PR; local results alone do not establish remote CI success.

This bounded audit does not certify pinned Nix development tools, GitHub Actions' bundled dependencies, platform-specific execution, or downloaded helper binaries (mise 2024.12.7 / chezmoi 2.46.1) and their embedded dependency graphs. Those remain explicit follow-up audit scope. No deployment, release, main merge, host-security change, billing/access change or private data is included.

## Exact advisory inventory

`symbol` means a conservative call-graph match, `package` an imported affected package, and `module` only a selected affected module. “Fixed” is the scanner's baseline fix recommendation; the chosen version can be newer. All rows are resolved in the final scans. Advisory IDs link to primary Go vulnerability records.

| Advisory | Module | Baseline | Fixed | Final selected | Baseline reach |
|---|---|---|---|---|---|
| [GO-2026-4550](https://pkg.go.dev/vuln/GO-2026-4550) | github.com/cloudflare/circl | v1.6.1 | v1.6.3 | v1.6.3 | symbol |
| [GO-2026-5490](https://pkg.go.dev/vuln/GO-2026-5490) | github.com/go-git/go-billy/v5 | v5.6.2 | v5.9.0 | v5.9.0 | symbol |
| [GO-2026-5597](https://pkg.go.dev/vuln/GO-2026-5597) | github.com/go-git/go-billy/v5 | v5.6.2 | v5.9.0 | v5.9.0 | symbol |
| [GO-2026-4473](https://pkg.go.dev/vuln/GO-2026-4473) | github.com/go-git/go-git/v5 | v5.16.3 | v5.16.5 | v5.19.2 | symbol |
| [GO-2026-4909](https://pkg.go.dev/vuln/GO-2026-4909) | github.com/go-git/go-git/v5 | v5.16.3 | v5.17.1 | v5.19.2 | symbol |
| [GO-2026-4910](https://pkg.go.dev/vuln/GO-2026-4910) | github.com/go-git/go-git/v5 | v5.16.3 | v5.17.1 | v5.19.2 | symbol |
| [GO-2026-5074](https://pkg.go.dev/vuln/GO-2026-5074) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.0 | v5.19.2 | symbol |
| [GO-2026-5105](https://pkg.go.dev/vuln/GO-2026-5105) | github.com/go-git/go-git/v5 | v5.16.3 | v5.18.0 | v5.19.2 | symbol |
| [GO-2026-5336](https://pkg.go.dev/vuln/GO-2026-5336) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.1 | v5.19.2 | package |
| [GO-2026-5496](https://pkg.go.dev/vuln/GO-2026-5496) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.1 | v5.19.2 | symbol |
| [GO-2026-5693](https://pkg.go.dev/vuln/GO-2026-5693) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.1 | v5.19.2 | package |
| [GO-2026-6213](https://pkg.go.dev/vuln/GO-2026-6213) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.2 | v5.19.2 | symbol |
| [GO-2026-6214](https://pkg.go.dev/vuln/GO-2026-6214) | github.com/go-git/go-git/v5 | v5.16.3 | v5.19.2 | v5.19.2 | symbol |
| [GO-2026-4945](https://pkg.go.dev/vuln/GO-2026-4945) | github.com/go-jose/go-jose/v4 | v4.1.1 | v4.1.4 | v4.1.4 | symbol |
| [GO-2026-4358](https://pkg.go.dev/vuln/GO-2026-4358) | github.com/sigstore/sigstore | v1.9.6-0.20250729224751-181c5d3339b3 | v1.10.4 | v1.10.9 | module |
| [GO-2025-4116](https://pkg.go.dev/vuln/GO-2025-4116) | golang.org/x/crypto | v0.42.0 | v0.43.0 | v0.59.0 | package |
| [GO-2025-4134](https://pkg.go.dev/vuln/GO-2025-4134) | golang.org/x/crypto | v0.42.0 | v0.45.0 | v0.59.0 | package |
| [GO-2025-4135](https://pkg.go.dev/vuln/GO-2025-4135) | golang.org/x/crypto | v0.42.0 | v0.45.0 | v0.59.0 | package |
| [GO-2026-5005](https://pkg.go.dev/vuln/GO-2026-5005) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5006](https://pkg.go.dev/vuln/GO-2026-5006) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5013](https://pkg.go.dev/vuln/GO-2026-5013) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5014](https://pkg.go.dev/vuln/GO-2026-5014) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5015](https://pkg.go.dev/vuln/GO-2026-5015) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5016](https://pkg.go.dev/vuln/GO-2026-5016) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5017](https://pkg.go.dev/vuln/GO-2026-5017) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5018](https://pkg.go.dev/vuln/GO-2026-5018) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5019](https://pkg.go.dev/vuln/GO-2026-5019) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5020](https://pkg.go.dev/vuln/GO-2026-5020) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5021](https://pkg.go.dev/vuln/GO-2026-5021) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5023](https://pkg.go.dev/vuln/GO-2026-5023) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5033](https://pkg.go.dev/vuln/GO-2026-5033) | golang.org/x/crypto | v0.42.0 | v0.52.0 | v0.59.0 | package |
| [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) | golang.org/x/crypto | v0.42.0 | v0.58.1-0.20261009181718-2f1f834330b2 | v0.59.0 | module |
| [GO-2026-6303](https://pkg.go.dev/vuln/GO-2026-6303) | golang.org/x/crypto | v0.42.0 | v0.55.0 | v0.59.0 | package |
| [GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354) | golang.org/x/crypto | v0.42.0 | v0.56.0 | v0.59.0 | package |
| [GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355) | golang.org/x/crypto | v0.42.0 | v0.56.0 | v0.59.0 | package |
| [GO-2026-4440](https://pkg.go.dev/vuln/GO-2026-4440) | golang.org/x/net | v0.43.0 | v0.45.0 | v0.62.0 | module |
| [GO-2026-4441](https://pkg.go.dev/vuln/GO-2026-4441) | golang.org/x/net | v0.43.0 | v0.45.0 | v0.62.0 | module |
| [GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918) | golang.org/x/net | v0.43.0 | v0.53.0 | v0.62.0 | module |
| [GO-2026-5025](https://pkg.go.dev/vuln/GO-2026-5025) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5027](https://pkg.go.dev/vuln/GO-2026-5027) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5028](https://pkg.go.dev/vuln/GO-2026-5028) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5029](https://pkg.go.dev/vuln/GO-2026-5029) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5030](https://pkg.go.dev/vuln/GO-2026-5030) | golang.org/x/net | v0.43.0 | v0.55.0 | v0.62.0 | module |
| [GO-2026-5942](https://pkg.go.dev/vuln/GO-2026-5942) | golang.org/x/net | v0.43.0 | v0.56.0 | v0.62.0 | module |
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) | golang.org/x/net | v0.43.0 | v0.60.0 | v0.62.0 | module |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) | golang.org/x/net | v0.43.0 | v0.60.0 | v0.62.0 | module |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) | golang.org/x/net | v0.43.0 | v0.60.0 | v0.62.0 | module |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) | golang.org/x/net | v0.43.0 | v0.60.0 | v0.62.0 | module |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) | golang.org/x/net | v0.43.0 | v0.60.0 | v0.62.0 | module |
| [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) | golang.org/x/net | v0.43.0 | v0.62.0 | v0.62.0 | module |
| [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) | golang.org/x/net | v0.43.0 | v0.62.0 | v0.62.0 | module |
| [GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) | golang.org/x/sys | v0.37.0 | v0.44.0 | v0.50.0 | module |
| [GO-2025-4155](https://pkg.go.dev/vuln/GO-2025-4155) | stdlib | v1.24.9 | v1.24.11 | v1.26.11 | symbol |
| [GO-2025-4175](https://pkg.go.dev/vuln/GO-2025-4175) | stdlib | v1.24.9 | v1.24.11 | v1.26.11 | symbol |
| [GO-2026-4337](https://pkg.go.dev/vuln/GO-2026-4337) | stdlib | v1.24.9 | v1.24.13 | v1.26.11 | symbol |
| [GO-2026-4340](https://pkg.go.dev/vuln/GO-2026-4340) | stdlib | v1.24.9 | v1.24.12 | v1.26.11 | symbol |
| [GO-2026-4341](https://pkg.go.dev/vuln/GO-2026-4341) | stdlib | v1.24.9 | v1.24.12 | v1.26.11 | package |
| [GO-2026-4342](https://pkg.go.dev/vuln/GO-2026-4342) | stdlib | v1.24.9 | v1.24.12 | v1.26.11 | module |
| [GO-2026-4601](https://pkg.go.dev/vuln/GO-2026-4601) | stdlib | v1.24.9 | v1.25.8 | v1.26.11 | symbol |
| [GO-2026-4602](https://pkg.go.dev/vuln/GO-2026-4602) | stdlib | v1.24.9 | v1.25.8 | v1.26.11 | symbol |
| [GO-2026-4603](https://pkg.go.dev/vuln/GO-2026-4603) | stdlib | v1.24.9 | v1.25.8 | v1.26.11 | module |
| [GO-2026-4864](https://pkg.go.dev/vuln/GO-2026-4864) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | package |
| [GO-2026-4865](https://pkg.go.dev/vuln/GO-2026-4865) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | module |
| [GO-2026-4869](https://pkg.go.dev/vuln/GO-2026-4869) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | symbol |
| [GO-2026-4870](https://pkg.go.dev/vuln/GO-2026-4870) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | symbol |
| [GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | symbol |
| [GO-2026-4946](https://pkg.go.dev/vuln/GO-2026-4946) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | symbol |
| [GO-2026-4947](https://pkg.go.dev/vuln/GO-2026-4947) | stdlib | v1.24.9 | v1.25.9 | v1.26.11 | symbol |
| [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) | stdlib | v1.24.9 | v1.25.12 | v1.26.11 | package |
| [GO-2026-4971](https://pkg.go.dev/vuln/GO-2026-4971) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | symbol |
| [GO-2026-4976](https://pkg.go.dev/vuln/GO-2026-4976) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | module |
| [GO-2026-4977](https://pkg.go.dev/vuln/GO-2026-4977) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | module |
| [GO-2026-4980](https://pkg.go.dev/vuln/GO-2026-4980) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | module |
| [GO-2026-4981](https://pkg.go.dev/vuln/GO-2026-4981) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | package |
| [GO-2026-4982](https://pkg.go.dev/vuln/GO-2026-4982) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | module |
| [GO-2026-4986](https://pkg.go.dev/vuln/GO-2026-4986) | stdlib | v1.24.9 | v1.25.10 | v1.26.11 | module |
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | symbol |
| [GO-2026-5037](https://pkg.go.dev/vuln/GO-2026-5037) | stdlib | v1.24.9 | v1.25.11 | v1.26.11 | symbol |
| [GO-2026-5038](https://pkg.go.dev/vuln/GO-2026-5038) | stdlib | v1.24.9 | v1.25.11 | v1.26.11 | package |
| [GO-2026-5039](https://pkg.go.dev/vuln/GO-2026-5039) | stdlib | v1.24.9 | v1.25.11 | v1.26.11 | symbol |
| [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856) | stdlib | v1.24.9 | v1.25.12 | v1.26.11 | symbol |
| [GO-2026-5972](https://pkg.go.dev/vuln/GO-2026-5972) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | symbol |
| [GO-2026-6088](https://pkg.go.dev/vuln/GO-2026-6088) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | module |
| [GO-2026-6089](https://pkg.go.dev/vuln/GO-2026-6089) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | package |
| [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | symbol |
| [GO-2026-6091](https://pkg.go.dev/vuln/GO-2026-6091) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | module |
| [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) | stdlib | v1.24.9 | v1.25.13 | v1.26.11 | symbol |
| [GO-2026-6599](https://pkg.go.dev/vuln/GO-2026-6599) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | module |
| [GO-2026-6600](https://pkg.go.dev/vuln/GO-2026-6600) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | module |
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | package |
| [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6608](https://pkg.go.dev/vuln/GO-2026-6608) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | package |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6613](https://pkg.go.dev/vuln/GO-2026-6613) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) | stdlib | v1.24.9 | v1.26.9 | v1.26.11 | symbol |
| [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) | stdlib | v1.24.9 | v1.26.10 | v1.26.11 | symbol |
| [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859) | stdlib | v1.24.9 | v1.26.11 | v1.26.11 | symbol |

## Additional baseline graph-only findings

These selected modules were outside the baseline loaded-package scan. Their presence alone does not establish production exposure. Fixed versions below list the advisory intervals; the final version resolves all applicable intervals.

| Advisory | Module | Baseline | Fixed versions | Final selected |
|---|---|---|---|---|
| [GO-2025-3540](https://pkg.go.dev/vuln/GO-2025-3540) | github.com/redis/go-redis/v9 | v9.3.0 | 9.5.5, 9.6.3, 9.7.3 | removed |
| [GO-2025-3830](https://pkg.go.dev/vuln/GO-2025-3830) | github.com/docker/docker | v28.2.2+incompatible | 28.3.3+incompatible | removed |
| [GO-2026-4394](https://pkg.go.dev/vuln/GO-2026-4394) | go.opentelemetry.io/otel/sdk | v1.37.0 | 1.40.0 | v1.45.0 |
| [GO-2026-4610](https://pkg.go.dev/vuln/GO-2026-4610) | github.com/docker/cli | v28.2.2+incompatible | 29.2.0+incompatible | v29.5.3+incompatible |
| [GO-2026-4762](https://pkg.go.dev/vuln/GO-2026-4762) | google.golang.org/grpc | v1.75.0 | 1.79.3 | v1.83.2 |
| [GO-2026-4883](https://pkg.go.dev/vuln/GO-2026-4883) | github.com/docker/docker | v28.2.2+incompatible |  | removed |
| [GO-2026-4887](https://pkg.go.dev/vuln/GO-2026-4887) | github.com/docker/docker | v28.2.2+incompatible |  | removed |
| [GO-2026-4985](https://pkg.go.dev/vuln/GO-2026-4985) | go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp | v1.33.0 | 1.43.0 | removed |
| [GO-2026-5426](https://pkg.go.dev/vuln/GO-2026-5426) | go.opentelemetry.io/otel/sdk | v1.37.0 | 1.43.0 | v1.45.0 |
| [GO-2026-5506](https://pkg.go.dev/vuln/GO-2026-5506) | go.opentelemetry.io/otel | v1.37.0 | 1.41.0 | v1.45.0 |
| [GO-2026-5617](https://pkg.go.dev/vuln/GO-2026-5617) | github.com/docker/docker | v28.2.2+incompatible |  | removed |
| [GO-2026-5668](https://pkg.go.dev/vuln/GO-2026-5668) | github.com/docker/docker | v28.2.2+incompatible |  | removed |
| [GO-2026-5746](https://pkg.go.dev/vuln/GO-2026-5746) | github.com/docker/docker | v28.2.2+incompatible |  | removed |
| [GO-2026-5764](https://pkg.go.dev/vuln/GO-2026-5764) | github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream | v1.6.2 | 1.7.8 | removed |
| [GO-2026-5764](https://pkg.go.dev/vuln/GO-2026-5764) | github.com/aws/aws-sdk-go-v2/service/s3 | v1.55.1 | 1.97.3 | removed |
| [GO-2026-5841](https://pkg.go.dev/vuln/GO-2026-5841) | github.com/klauspost/compress | v1.18.0 | 1.18.7 | v1.20.2 |
| [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) | golang.org/x/text | v0.29.0 | 0.39.0 | v0.44.0 |
| [GO-2026-6061](https://pkg.go.dev/vuln/GO-2026-6061) | google.golang.org/grpc | v1.75.0 | 1.82.1 | v1.83.2 |
| [GO-2026-6179](https://pkg.go.dev/vuln/GO-2026-6179) | golang.org/x/mod | v0.25.0 | 0.40.0 | v0.42.0 |
| [GO-2026-6180](https://pkg.go.dev/vuln/GO-2026-6180) | golang.org/x/mod | v0.25.0 | 0.40.0 | v0.42.0 |
| [GO-2026-6348](https://pkg.go.dev/vuln/GO-2026-6348) | google.golang.org/grpc | v1.75.0 | 1.83.1 | v1.83.2 |
| [GO-2026-6441](https://pkg.go.dev/vuln/GO-2026-6441) | google.golang.org/grpc | v1.75.0 | 1.83.1 | v1.83.2 |
| [GO-2026-6443](https://pkg.go.dev/vuln/GO-2026-6443) | google.golang.org/grpc | v1.75.0 | 1.82.2, 1.83.2, 1.84.0-dev.0.20260825144003-d5a41119e0e3, 1.85.0-dev.0.20260825072537-93e31b48545e | v1.83.2 |
| [GO-2026-6505](https://pkg.go.dev/vuln/GO-2026-6505) | go.opentelemetry.io/otel/exporters/otlp/otlptrace | v1.27.0 | 1.45.0 | removed |
| [GO-2026-6505](https://pkg.go.dev/vuln/GO-2026-6505) | go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc | v1.27.0 | 1.45.0 | removed |
| [GO-2026-6505](https://pkg.go.dev/vuln/GO-2026-6505) | go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp | v1.33.0 | 1.45.0 | removed |
| [GO-2026-6505](https://pkg.go.dev/vuln/GO-2026-6505) | go.opentelemetry.io/otel/sdk | v1.37.0 | 1.45.0 | v1.45.0 |
| [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629) | golang.org/x/text | v0.29.0 | 0.41.0 | v0.44.0 |
| [GO-2026-6778](https://pkg.go.dev/vuln/GO-2026-6778) | github.com/docker/distribution | v2.8.3+incompatible |  | removed |
| [GO-2026-6865](https://pkg.go.dev/vuln/GO-2026-6865) | github.com/klauspost/compress | v1.18.0 | 1.20.2 | v1.20.2 |
