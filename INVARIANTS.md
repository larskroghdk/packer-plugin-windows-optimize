# Invariants

Hard constraints. A change that breaks one of these is wrong even if tests pass. IDs are stable; reference them as `I<n>`.

## Catalog

- **I1** **Every optimization is documented and traceable.** It has a name, description, impact, at least one `https` reference, and a recommended optimization cites Microsoft and needs no restart.
  Enforced by: `catalog.Validate`; `TestCatalogIsValid`, `TestRecommendedProfileIsConservative`.

- **I2** **Classification and profile agree.** `recommended` is in the recommended profile, `optional` is in `performance`, `vdi` or no profile, `high_risk` is in no profile. An optimization in no profile is selected only through `include`, and a `high_risk` one only by its exact ID.
  Enforced by: `validateOptimization`; the exact-match rule in `plan.selection`; `TestValidateRejects`, `TestProfileSelection`, `TestIncludeExclude`.

- **I3** **Profiles are cumulative**: recommended ⊂ performance ⊂ vdi.
  Enforced by: `Profile.Includes`; `TestProfilesAreCumulative`.

- **I4** **A catalog change sets a value of one of the six restorable kinds**, never removes one, and two optimizations never set the same target on the same systems.
  Enforced by: `validateChange` and `checkSharedTargets`; `TestValidateRejects`, `TestValidateSharedTargets`.

- **I5** **No optimization weakens security by default.** Defender, Windows Update, auditing, event logging, the firewall, UAC and authentication are never changed by a profile.
  Enforced by: the security rules in `internal/plan/protect.go`; `TestNoSecurityControlIsDisabled`, `TestCatalogNeverTouchesProtectedTargets`.

- **I6** **`docs/catalog.md` is generated from the catalog** by `make catalog` and never edited by hand.
  Enforced by: `catalogdoc -check` in `make lint`.

## Packer session safety

- **I7** **No change is applied that the protection rules haven't cleared.** Every change of a selected optimization goes through `plan.Protect` in `plan.Build`, and a protected change skips the whole optimization.
  Enforced by: `plan.Build`; `TestProtectStatic`, `TestProtectSkipsWholeOptimization`; the fake guest in `provisioner/guest_test.go` fails any test that applies a protected target.

- **I8** **Protection fails closed.** When the WinRM dependency tree is unknown, no service start type is changed. When the power configuration is unknown, no power optimization is applied.
  Enforced by: `plan.Protect`, `Item.resolve`; `TestProvisionProtectsWinRMDependencies`, `TestPowerSchemeResolution`.

- **I9** **Every apply is followed by a new WinRM command and a comparison with the detected WinRM configuration**, and a regression fails the build regardless of `fail_on_error`.
  Enforced by: `run.checkConnectivity`, `facts.WinRMRegressions`; `TestWinRMRegressions`, `TestProvisionDetectsWinRMRegression`.

- **I10** **The plugin never restarts the guest unless `reboot_if_required` is set**, and never suspends it: sleep and hibernate idle timeouts are only ever set to 0.
  Enforced by: `run.optimize` and `run.rollback`; the sleep rule in `ProtectStatic`; `TestProvisionReboot`.

- **I11** **The plugin runs only over WinRM, in an elevated, 64-bit PowerShell session, on a supported OS.**
  Enforced by: the `ConnType` check in `Provisioner.Provision` and the checks in `run.prepareGuest`; `facts.Classify`; `TestProvisionRefusesUnsafeSessions`, `TestClassifyRejects`.

## Correctness

- **I12** **Nothing is reported as applied or restored unless the guest applied it and re-inspection reads the wanted value.** A value that doesn't survive a restart is failed.
  Enforced by: `verifyChange`, `Plan.Reverify`, `Rollback.Reverify`; `TestVerifyRequiresReinspection`, `TestReverify`, `TestRollback`.

- **I13** **Original values are journaled before anything changes, the earliest original wins, and rollback only restores targets the catalog changes.** A value that can't be restored is never changed.
  Enforced by: `run.recordJournal` before `applyAndInspect`; `Journal.Record`; `NewRollback`; `restoreValue`; `TestRecordKeepsEarliestOriginal`, `TestRollbackRejectsUnknownTargets`, the journal check in the fake guest.

- **I14** **The guest scripts make no decisions.** Selection, applicability, protection, comparison and verification are decided in Go. Guest actions only detect, read and write state.
  Enforced by: review only.

- **I15** **A second run with the same configuration changes nothing.**
  Enforced by: `TestApplyCycleAndIdempotency`, `TestProvisionApplyIsIdempotent`.

## Build and contracts

- **I16** **`provisioner/config.hcl2spec.go` matches `Config`.** Regenerate it with `make generate` whenever a `mapstructure` field changes.
  Enforced by: `make generate-check`; `TestConfigSpec`.

- **I17** **An incompatible change to the report JSON increments `report.SchemaVersion`, and to the journal increments `plan.JournalSchemaVersion`.**
  Enforced by: review only. `ParseJournal` rejects an unknown journal version.

- **I18** **`make check` passes before a change is committed.** The `//go:embed scripts/*.ps1` directive in `guest/guest.go` stays in place, and every `.ps1` file is ASCII with CRLF line endings.
  Enforced by: `make check`, locally and in `.github/workflows/build.yml`; `TestScriptsAreComplete`, `TestScriptsAreWindowsPowerShellSafe`.

- **I19** **Source code has no comments except documentation: Go doc comments and PowerShell comment-based help.** In Go, every function and method, unexported and test functions included, has a doc comment that starts with its name, describes non-obvious parameters under `Parameters:`, has a `Returns:` list when it returns an error or more than one value, and ends with `Author: Lars Krogh` and `Version: x.y.z` lines. Every exported type and every non-test package has a doc comment. No other Go comments are allowed. Generated files (`// Code generated ... DO NOT EDIT.`) are exempt. In PowerShell, the only comment allowed is a comment-based help block (`<# .SYNOPSIS ... #>`), and every `.ps1` file and every function has one. Script help goes directly after `#Requires`, followed by two blank lines so it isn't attached to the first function. Function help goes at the top of the function body. Each block contains `.SYNOPSIS`, `.DESCRIPTION`, `.PARAMETER` for each parameter, at least one `.EXAMPLE`, `.OUTPUTS` when the function returns something, and `.NOTES` with `Author` and a semantic `Version`. Keep the existing author and raise the version when behavior changes. Functional directives are allowed in both languages: `//go:embed`, `//go:build`, `#Requires`. Everything else belongs in Markdown (`AGENTS.md`, `docs/`).
  Enforced by: `make doccheck`, part of `make lint`. `cmd/doccheck` rejects missing or misnamed Go doc comments, missing `Author`, `Version` and `Returns:` lines, and any other Go comment except `//go:` directives. `psdoccheck.sh` rejects any PowerShell comment outside a help block and any block without `.SYNOPSIS`. Whether the documentation is complete and accurate is checked in review only.
