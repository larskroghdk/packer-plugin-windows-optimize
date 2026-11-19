---
name: add-optimization
description: Add a new optimization to the catalog, or change an existing one (its values, OS rules, profile or classification). Use when internal/catalog/optimizations.go or docs/catalog.md must change, or when evaluating a setting from Citrix Optimizer, VDOT, Microsoft VDI guidance or an existing script.
---

# Add an optimization

Constraints: INVARIANTS.md I1–I7, I13 and I15. Module context: `internal/catalog/AGENTS.md`, `internal/plan/AGENTS.md`.

## 1. Validate the candidate

Check it against every criterion in `docs/research.md` (Method). Reject it, and add it to the Rejected table in `docs/research.md` with the reason, if any of these hold:

- Microsoft doesn't document it. Community sources and tool templates only nominate candidates.
- It is already the default, or current Windows ignores it.
- It weakens Defender, Windows Update, auditing, logging, the firewall, UAC or authentication.
- It touches WinRM, networking or anything in the static protection lists in `internal/plan/protect.go`.
- It is per-user, removes a component or app, or isn't one of the six target kinds.

## 2. Classify

| Classification | Profile | When |
| --- | --- | --- |
| `recommended` | `recommended` | Safe for every template, no restart, cites `learn.microsoft.com` |
| `optional` | `performance` or `vdi`, or none | Has a functional trade-off. `vdi` when it only makes sense for non-persistent or pooled desktops. No profile when it needs a restart or is rarely wanted. |
| `high_risk` | none | Can lose data, block updates or break workloads. Selected only by exact ID. |

## 3. Write the entry

In `internal/catalog/optimizations.go`:

- `ID` is `<category prefix>.<name>` in lower case. Never reuse or rename an existing ID.
- `Description` says what changes and why. `Impact` says what the user loses.
- `Applies`: the narrowest installation types and build range, and `Editions: enterpriseClient` for policies that Windows Pro ignores.
- `Changes`: use `registryDWORD`, `serviceStart`, `taskState` or `powerIndex`. Use the policy value, not the preference value.
- `RebootRequired` only if Microsoft says the change needs a restart.
- `References`: the Microsoft page that documents the exact value.

## 4. Test and regenerate

1. `go test ./internal/catalog/ ./internal/plan/`. `TestCatalogIsValid`, `TestCatalogNeverTouchesProtectedTargets` and `TestNoSecurityControlIsDisabled` cover the new entry automatically.
2. If the profile or OS rules matter to a test expectation (counts in `TestProfileSelection`, `TestApplicability`, `TestApplyCycleAndIdempotency`), update it.
3. `make catalog` to regenerate `docs/catalog.md`.
4. Update `docs/profiles.md` if a profile's contents changed, and `docs/research.md` if the candidate came from the original scripts.
5. `make check`.

## 5. Verify on a guest

Run the live integration tests (`docs/testing.md`) against a disposable VM of each OS the entry applies to, with `WO_TEST_PROFILE` set to a profile that includes it, or the examples with `profile = "custom"` and `include = ["<id>"]`. First with `dry_run = true`, then without, then with `rollback = true`. Check `before` and `after` in the report.

## Done when

- `make check` passes, and `docs/catalog.md` is regenerated in the same commit.
- The report from a real guest shows the optimization `applied`, then `compliant` on a second run, and restored by rollback.
