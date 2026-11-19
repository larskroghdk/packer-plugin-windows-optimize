---
name: support-windows-release
description: Add a new Windows or Windows Server release (a new feature update or build number) to the known releases, and check which optimizations apply to it. Use when a build warns that the OS is newer than the releases the plugin was tested with, or when Microsoft ships a new release.
---

# Support a new Windows release

Constraints: INVARIANTS.md I1, I11 and I15. Module context: `internal/catalog/AGENTS.md`.

Unknown builds are already optimized, with a warning and `known_release: false`. This skill turns a release into a known one after it has been checked.

## Steps

1. **Get the facts from a real guest.** Run the `local` example with `dry_run = true` against the new release, and read `system` in the report: `product_name`, `edition_id`, `installation`, `version`, `display_version`.
2. **Add the build** to `serverReleases` or `clientReleases` in `internal/facts/release.go`, with the Microsoft release name (for example `Windows 11 version 26H1`).
3. **Check each optimization's OS rules against Microsoft's documentation** for the new release:
   - Policies that were removed or are now ignored get a `MaxBuild` in their `Applies` rules.
   - Features that only exist from this release on get a `MinBuild`.
   - Services and scheduled tasks that were removed need no change: missing targets are `unsupported`.
4. **Compare the dry-run report** with the same profile on the previous release. Every difference must be explained by step 3.
5. **Update the tests:** add the build to `TestClassify` (`internal/facts/facts_test.go`), and to `TestApplicability` (`internal/plan/plan_test.go`) if a build range changed.
6. **Regenerate and document:** for a new major release, add a column to the matrix list in `cmd/catalogdoc/main.go`. Run `make catalog`, then update the tables in `docs/supported-os.md` and `README.md`.
7. **Run `make check`**, then the live integration tests against the new release.

## Done when

- `make check` passes.
- A dry run on the new release reports `known_release: true`, and the apply, idempotent re-apply and rollback cycle passes on a disposable VM.
