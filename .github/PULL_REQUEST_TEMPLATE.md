## Summary

<!-- What changes and why. For behavior changes, describe the before and after. -->

## Related issues

<!-- `Closes #` / `Refs #`. Larger changes and new optimizations should start with an issue (see CONTRIBUTING.md). -->

## Breaking changes and compatibility

- [ ] No breaking change, or the change to configuration, profile contents, report output or behavior is described above
- [ ] Configuration change regenerates `provisioner/config.hcl2spec.go` with `make generate` and updates [docs/configuration.md](https://github.com/larskroghdk/packer-plugin-windows-optimize/blob/main/docs/configuration.md) (I16), or N/A
- [ ] Incompatible report JSON change increments `report.SchemaVersion`, and journal change increments `plan.JournalSchemaVersion` (I17), or N/A
- [ ] Journals written by the previous release can still be rolled back, or N/A
- [ ] Release classification changes follow the [support-windows-release](https://github.com/larskroghdk/packer-plugin-windows-optimize/blob/main/.agents/skills/support-windows-release/SKILL.md) skill, or N/A

Affected Windows releases and installation types (client, Server Desktop Experience, Server Core):

## Testing

- [ ] `make check` passes
- [ ] Regression tests fail on the old behavior and pass on the new one, or N/A
- [ ] The fake guest in `provisioner/guest_test.go` matches any changed guest action, or N/A
- [ ] Guest-script tests pass under Windows PowerShell 5.1
- [ ] Live run with `make test-integration` on a disposable VM or checkpoint: release, build, installation type, profile and attached report, or "Not run" with the reason

Unit, fake-guest and Pester tests don't validate a real WinRM session, restart recovery or the effect of a setting on Windows.

## Optimization behavior

- [ ] A second run with the same configuration changes nothing and requests no restart (I15)
- [ ] Every applied or restored value is confirmed by re-inspection, and a value that doesn't survive a restart is failed (I12), or N/A
- [ ] Original values are journaled before the apply, and rollback restores them (I13), or N/A
- [ ] `fail_on_error = false` still fails the build on a WinRM regression, an unsupported OS or a journal write failure, or N/A
- [ ] Restarts happen only with `reboot_if_required = true`, and the guest is never suspended (I10), or N/A
- [ ] Decisions stay in Go; guest scripts only detect, read and write state (I14), or N/A

## Catalog changes

- [ ] Each new or changed optimization cites a Microsoft source and meets the [acceptance criteria](https://github.com/larskroghdk/packer-plugin-windows-optimize/blob/main/docs/research.md#method), or N/A
- [ ] The setting has an effect on every release and installation type its `Applies` rules declare, with the evidence linked: documentation, a before and after measurement, or a reproducible test, or N/A
- [ ] Classification, profile and `RebootRequired` match the impact; `recommended` needs no restart (I1, I2), or N/A
- [ ] Possible regressions and side effects are described: lost features, slower first logon, higher resource use elsewhere, or N/A
- [ ] `docs/catalog.md` is regenerated with `make catalog`, not edited by hand (I6), or N/A

Performance claim and evidence:

## Session safety and security

- [ ] Changes that could affect WinRM, networking, the firewall or the Packer user have a protection rule that fails closed (I7, I8, [WinRM protection](https://github.com/larskroghdk/packer-plugin-windows-optimize/blob/main/docs/connectivity.md)), or N/A
- [ ] No profile weakens Defender, Windows Update, auditing, event logging, the firewall, UAC or authentication (I5, [security](https://github.com/larskroghdk/packer-plugin-windows-optimize/blob/main/docs/security.md))
- [ ] The change doesn't block Windows servicing, pending updates or Sysprep generalization of the image, or N/A
- [ ] Targets also managed by another `packer-plugin-windows-*` provisioner, such as a harden baseline, are named above with the build order that gives the intended result, or N/A
- [ ] Guest data passes through the JSON contract, not interpolated into PowerShell, or N/A
- [ ] No credentials or secrets in code, reports, logs or artifacts
- [ ] Actions remain pinned to commit SHAs with minimum permissions, or N/A

Security impact:

## Documentation

- [ ] README, `docs/`, examples, AGENTS.md and `.agents/skills` updated where behavior changed, or N/A
- [ ] Go doc comments and PowerShell help follow I19
- [ ] User-visible changes have a commit subject that the release notes pick up (`docs:` and `test:` commits are excluded), or N/A
