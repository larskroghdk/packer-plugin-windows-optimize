# internal/plan/

Selects optimizations, checks applicability, resolves the power scheme, applies the protection rules, compares inspected state, verifies results, and owns the rollback journal. Parent: [AGENTS.md](../../AGENTS.md). Constraints: [INVARIANTS.md](../../INVARIANTS.md) (I2, I7, I8, I10, I12, I13 and I15 apply directly).

## Local constraints

- The package is pure: no I/O and no guest calls. Input comes through `facts.Facts`, `States` and `ApplyResults`, so every rule is unit-tested in [plan_test.go](plan_test.go) and [journal_test.go](journal_test.go).
- An optimization is all or nothing. Anything that blocks one change (protection, unknown state, a value that can't be restored) skips the whole item through `Item.skip`.
- A new rule in [protect.go](protect.go) must fail closed when the facts it needs are missing, and needs tests for the protected case, the allowed case and the unknown-state case.
- Status only becomes `applied` or `restored` in `verifyChange`, after the guest reported success and the re-inspected value matches (I12).
- `Journal.Record` never overwrites an existing entry. The earliest original wins.

## Footguns

- `States` and `ApplyResults` are keyed by lower-case target ID. Use their `Get` methods, or a lookup silently misses and the change is marked failed.
- Power setting targets have an empty `Scheme` in the catalog. `Item.resolve` fills it in, so the target ID changes from `power_setting:active/…` to `power_setting:<guid>/…`. Journal entries carry the resolved ID; `NewRollback` clears the scheme when it checks an entry against the catalog.
- Protected registry keys are lower-case paths without `HKLM\`, unlike target IDs (`registry:HKLM\…`).
- The sleep rule in `ProtectStatic` rejects any non-zero sleep or hibernate timeout. Rollback restores non-zero originals deliberately, so it doesn't run `Protect`; it only accepts targets the catalog changes.
- `Compare` reads REG_DWORD and power indexes as uint32 decimals. The guest must send unsigned values.
- A target that doesn't exist is `NotApplicable`. If none of an optimization's targets exist, the item is `unsupported`, not failed.
- Include patterns match high-risk optimizations only by exact ID (`selection`). A wildcard must never select one.
