# Agent instructions

A Packer provisioner plugin that applies validated, reversible performance optimizations to Windows images over WinRM without breaking the Packer session. Hard constraints are in [INVARIANTS.md](INVARIANTS.md); this file covers how to work here.

## Module map

| Path | Role | Context |
| --- | --- | --- |
| `main.go` | Plugin entry point; holds `version` | — |
| `provisioner/` | Packer config, run orchestration (`optimize`, `rollback`), guest access (`remote.go`), restart | [provisioner/AGENTS.md](provisioner/AGENTS.md) |
| `internal/catalog/` | Optimization model, the catalog in `optimizations.go`, catalog validation | [internal/catalog/AGENTS.md](internal/catalog/AGENTS.md) |
| `internal/plan/` | Selection, applicability, protection rules, compare and verify, rollback journal. No I/O. | [internal/plan/AGENTS.md](internal/plan/AGENTS.md) |
| `internal/facts/` | Guest facts, OS classification, WinRM regression checks | — |
| `internal/report/` | JSON report and console summary | — |
| `guest/` | Embedded PowerShell 5.1 run on the guest | [guest/AGENTS.md](guest/AGENTS.md) |
| `cmd/catalogdoc/` | Generates `docs/catalog.md` | [internal/catalog/AGENTS.md](internal/catalog/AGENTS.md) |
| `cmd/doccheck/` | Lint for the Go documentation rules of I19 | — |
| `test/pester/` | Guest-script tests | [guest/AGENTS.md](guest/AGENTS.md) |
| `test/integration/` | Live Packer tests (build tag `integration`) | [docs/testing.md](docs/testing.md) |

## Run flow

`Provision` in [provisioner/provisioner.go](provisioner/provisioner.go): upload scripts → `Detect` → classify OS → `plan.Build` (select, applicability, power scheme, protect) → `Inspect` → `Plan.Evaluate` → stop if `dry_run` → record originals in the journal → `Apply` and re-inspect → `Plan.Verify` → `Probe` and compare WinRM → optional restart and `Reverify` → write the report. Rollback replaces the middle with `ReadJournal` → inspect → restore → verify → `WriteJournal` with the remaining entries. Each guest action is one `powershell.exe -File Invoke-WindowsOptimize.ps1 -Action <name>` call that exchanges JSON files.

## Commands

| Command | Does |
| --- | --- |
| `make check` | Module tidiness, lint (including pinned staticcheck and govulncheck), typecheck and tests. Required before commit |
| `make generate` | Regenerate `provisioner/config.hcl2spec.go` |
| `make catalog` | Regenerate `docs/catalog.md` |
| `make test-integration` | Live tests; needs `WO_TEST_*` variables, see [docs/testing.md](docs/testing.md) |
| `make build` | Build the plugin binary |

## Footguns

- `make` is not on the default Git Bash PATH on Windows. Run the recipe lines from the [Makefile](Makefile) directly.
- `.ps1` files must stay ASCII with CRLF. Tools that rewrite files (`sed -i`, some editors) can strip the CR; `guest/guest_test.go` catches it.
- Pester tests must run under `powershell.exe` 5.1, the PowerShell on the guest, not only `pwsh`.
- `go.mod` declares a newer Go version than some machines have. Go 1.21 or later downloads that toolchain automatically.
- A green `make check` doesn't cover a real WinRM build. The fake guest in `provisioner/guest_test.go` mirrors the guest contract; keep it in sync when a guest action changes.
- Never run `TestApplyIdempotentRollback` or a non-dry-run example against a machine you haven't been authorized to change.

## Skills

Task procedures live in `.agents/skills/<name>/SKILL.md`. Read only the one that matches the task:

- [add-optimization](.agents/skills/add-optimization/SKILL.md): add or change a catalog optimization.
- [add-protection-rule](.agents/skills/add-protection-rule/SKILL.md): protect a target that could break the Packer session or weaken security.
- [support-windows-release](.agents/skills/support-windows-release/SKILL.md): make a new Windows release a known, checked release.
- [release-plugin](.agents/skills/release-plugin/SKILL.md): CI, packaging and authorized signed releases.

Activating a skill grants no authority to publish or to operate on a guest. [docs/agent-guidance.md](docs/agent-guidance.md) explains how each tool finds these files.
