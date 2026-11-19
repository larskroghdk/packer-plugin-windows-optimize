---
name: add-protection-rule
description: Add or change a protection rule in internal/plan/protect.go for a target that could break the Packer WinRM session or weaken security (a service, registry key, scheduled task or power setting). Use when a build loses WinRM after optimizing, or before adding an optimization near WinRM, networking, authentication or security controls.
---

# Add a protection rule

Constraints: INVARIANTS.md I5, I7, I8, I9 and I10. Module context: `internal/plan/AGENTS.md`.

## 1. Choose the rule type

| Situation | Where |
| --- | --- |
| The target must never change, whatever the guest looks like | Static: `communicatorServices`, `securityServices`, `protectedRegistryPrefixes`, `protectedRegistryValues` or `protectedTaskPrefixes`, checked by `ProtectStatic` |
| Whether it is safe depends on this guest (for example the WinRM dependency tree) | Dynamic: a check in `Protect` that reads `facts.Facts` |

Prefer static. A dynamic rule must fail closed: when the facts it needs are `nil` or incomplete, it returns a reason.

## 2. Add facts if you need them

1. Collect them in `guest/scripts/WindowsOptimize.Facts.ps1` (`Get-WOSystemFact`). On failure, leave the section `$null` and add an entry to the detection errors.
2. Add the field to `internal/facts/facts.go`. The `json` tag must match the PowerShell property name.
3. Extend the Detect fixture in `internal/facts/facts_test.go` (`TestParse`) and the fake guest in `provisioner/guest_test.go`.
4. If losing the capability after a change must fail the run, add a comparison to `WinRMRegressions` (`internal/facts/connectivity.go`) and a case to `TestWinRMRegressions`.

## 3. Write the rule

- Keys are lower case: service names, and registry paths without `HKLM\`.
- The reason names the effect on the Packer session or on security, for example "WinRM listens through the HTTP.sys driver". It appears verbatim in the report.

## 4. Test

- `TestProtectStatic` or a new table test in `internal/plan/plan_test.go`: the protected case, the allowed case and, for a dynamic rule, the unknown-facts case.
- `TestProtectSkipsWholeOptimization` shows that one protected change skips the whole optimization.
- `TestCatalogNeverTouchesProtectedTargets` must still pass. If it fails, an existing optimization hits the new rule: remove or change that optimization, don't weaken the rule.
- Update the tables in `docs/connectivity.md` or `docs/security.md`, then `make check`.

## 5. Verify on a guest

For a rule found from a broken build, reproduce on a disposable VM: `dry_run = true` must show the optimization `skipped` with the new reason, and an apply must keep the WinRM check `passed`.
