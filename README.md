# ZERB

**Branch status:** pre-alpha architecture refocus and security hard cutover
**Active branch:** `project_refocus_and_security_fixes`

ZERB's target is a declarative, profile-based developer-environment restoration tool for mutable workstations. It will coordinate pinned native Git, mise, and chezmoi behavior while owning source selection, trust, collision policy, orchestration, durable state, and recovery.

## Do not install this branch

The currently reachable binary is the obsolete pre-cutover implementation. It does not implement the target public contract, source schemas, approval boundaries, protected-root model, or lifecycle guarantees. Target commands described in the roadmap are provisional contracts, not working usage instructions.

Do not install or run this branch against a real workstation, real home directory, credentials, existing manager state, or real destinations until Phase 6 installed-binary acceptance is complete. Development and proof work must use disposable HOME/XDG roots and dedicated non-sensitive fixtures.

There are no supported users or compatibility guarantees. The final release will be a hard cutover, not a migration layer over the reachable old product.

## Roadmap authority

Start with the [roadmap index](docs/roadmap/README.md). Its ownership order prevents planning summaries from becoming accidental behavior claims:

- [Provisional product contract](docs/roadmap/product-contract.md) — public CLI, schemas, approvals, output, exits, and user-visible behavior.
- [Architecture](docs/roadmap/architecture.md) — trust, process, storage, state, and recovery invariants.
- [Delivery plan](docs/roadmap/delivery-plan.md) — sequencing and review gates.
- [Validation](docs/roadmap/validation.md) — evidence and support claims.
- [Hard cutover](docs/roadmap/hard-cutover.md) — artifact disposition.
- [Post-MVP intent](docs/roadmap/post-mvp.md) — non-committed deferred direction.
- [Redacted proof results](docs/roadmap/proof-results.md) — evidence only, never normative authority.

Phase 0 is a provisional proof baseline. Only the approved Phase 1 exit can freeze contracts for implementation. The baseline ledger records the old binary's version and command table; this README intentionally does not present either old or target commands as usable product instructions.

## Development

Go 1.24 is the target development toolchain. From a disposable development checkout:

```sh
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
go build -o bin/zerb ./cmd/zerb
```

These are development gates, not installation steps. The recorded Phase 0 baseline passed tests, race tests, vet, and build; lint reported 54 issues in old code. See [proof results](docs/roadmap/proof-results.md) for the exact scope and limitations of that observation.

## Project scope

ZERB targets declared user-space development tools and dotfiles. It is not an operating-system image, system package manager, backup system, transactional rollback engine, hermetic build system, or machine snapshot. Support for a Linux distribution/kernel/CPU/filesystem tuple, transport, authentication mechanism, manager feature, shell, or parser exists only after the roadmap's required real evidence and final installed-binary vertical acceptance. Provider and host-fact behavior is excluded from the MVP.

## License

ZERB is licensed under [MIT-0](LICENSE).
