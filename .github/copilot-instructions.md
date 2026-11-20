# GitHub Copilot instructions

This repository contains a Go Packer provisioner plugin that applies validated, reversible Windows optimizations over WinRM. Preserve the Packer session, security controls and rollback capability in every change.

## Repository guidance

Read [AGENTS.md](../AGENTS.md) and [INVARIANTS.md](../INVARIANTS.md) before changing code. Read the directory-specific instructions for the files you change:

- [provisioner/AGENTS.md](../provisioner/AGENTS.md): configuration, orchestration, guest calls and restart handling.
- [internal/catalog/AGENTS.md](../internal/catalog/AGENTS.md): optimization definitions, validation and generated catalog documentation.
- [internal/plan/AGENTS.md](../internal/plan/AGENTS.md): selection, protection, verification and rollback journals.
- [guest/AGENTS.md](../guest/AGENTS.md): embedded PowerShell scripts and Pester tests.

Use the relevant repository skill when adding or changing an [optimization](../.agents/skills/add-optimization/SKILL.md), a [protection rule](../.agents/skills/add-protection-rule/SKILL.md) or a [supported Windows release](../.agents/skills/support-windows-release/SKILL.md).

## Implementation rules

- Keep selection, applicability, safety decisions and verification in Go. `internal/plan` has no I/O. Guest scripts only detect, read and write state.
- Route guest calls through `remote.action`. Mirror guest action changes in `provisioner/guest_test.go`.
- Journal original values before applying changes. Preserve the earliest original and verify applied or restored values by re-inspecting the guest.
- Fail closed when required safety facts are unknown. Check WinRM connectivity after every apply and restore; `fail_on_error = false` must never hide a regression.
- Restart only when `reboot_if_required` is set. Profiles must never weaken security controls.
- Keep optimization IDs stable and validate settings against Microsoft documentation with the narrowest applicable OS rules.
- Keep Go syntax compatible with Go 1.24. Guest scripts must support Windows PowerShell 5.1 and remain ASCII with CRLF line endings.
- Follow I19 for documentation: Go functions and methods need named doc comments with the required parameter and return details, author and version; PowerShell scripts and functions need comment-based help. Do not add ordinary source comments. Preserve permitted functional directives and generated-file exemptions.
- Regenerate `provisioner/config.hcl2spec.go` with `make generate` after changing configuration fields, and `docs/catalog.md` with `make catalog` after changing the catalog. Do not edit generated files manually.
- Increment the report or journal schema version when changing its JSON contract incompatibly.

## Validation

Run `make check` before committing. It covers lint, type checking, Go tests and Pester tests. Run Pester under `powershell.exe` 5.1, even when PowerShell 7 is installed. If `make` is unavailable on Windows, run the corresponding [Makefile](../Makefile) recipes directly.

For changes to the provisioner or guest scripts, follow [docs/testing.md](../docs/testing.md) for live integration validation. Local checks do not prove a real WinRM build works. Never run `TestApplyIdempotentRollback` or a non-dry-run example against a machine without authorization to change it. Report which checks passed and any validation that could not be completed.

CI runs checks directly in [.github/workflows/build.yml](workflows/build.yml) and builds snapshot artifacts. PowerShell Gallery supplies test dependencies; this repository does not publish a PowerShell module there.
