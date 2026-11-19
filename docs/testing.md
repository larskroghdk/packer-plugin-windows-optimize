# 🧪 Testing

| Layer | Where | Runs on | Changes the machine |
| --- | --- | --- | --- |
| Go unit tests | `internal/*/…_test.go` | any OS | no |
| Fake-guest integration tests | `provisioner/…_test.go` | any OS | no |
| Guest script tests (Pester) | `test/pester/` | Windows PowerShell 5.1 | no (HKCU scratch keys only) |
| Live integration tests | `test/integration/` | Windows or Linux with Packer, against a VM | only with explicit authorization |

`make check` runs everything except the live tests.

## Go unit tests

- `internal/facts`: release classification for every supported release, unknown future builds, rejection of domain controllers, old builds and unknown installation types; fact parsing; every kind of WinRM regression, and that additions are not regressions.
- `internal/catalog`: the embedded catalog passes validation; no catalog change hits a protection rule; no change touches Defender, Windows Update, auditing, event logging, the firewall or WinRM; the recommended profile needs no restart and has Microsoft sources; profiles are cumulative; `All` returns a deep copy; the validator catches each kind of catalog mistake; shared targets are only allowed on disjoint systems.
- `internal/plan`: profile and category selection, include and exclude, high-risk only by exact ID, applicability per OS and edition, power scheme resolution, protection (static, WinRM dependencies, fail closed on unknown facts), evaluation of every outcome, verification that requires reading back, re-verification after restart, value comparison per kind, the journal (earliest original wins, round trip, rollback with partial failure, removal of values that were absent, refusal of entries for targets the catalog doesn't change), and idempotency over the `vdi` profile.
- `provisioner`: configuration defaults and every validation error, and the generated HCL2 spec.
- `guest`: every part the entry script loads is embedded, and every script is ASCII with CRLF line endings, which Windows PowerShell 5.1 needs.

## Fake-guest integration tests

`provisioner/guest_test.go` implements the six guest actions as an in-memory Windows Server 2022 behind a `packer.Communicator`. The real provisioner runs against it end to end, through the same upload, command and download path it uses with WinRM. The fake fails the test when the plugin applies a protected or WinRM-dependency target, or applies anything that isn't already in the rollback journal.

Scenarios: dry run changes nothing; apply then re-apply is idempotent and keeps the journal; rollback (dry run, full, and with an empty journal) restores every original value and removes the journal; a failed restore stays in the journal; a change the guest rejects or doesn't persist fails, and only warns with `fail_on_error = false`; a WinRM regression fails the build even with `fail_on_error = false`; a WinRM dependency is never touched, and unknown WinRM facts fail closed; restart only with `reboot_if_required`, and a value reverted by the restart fails; non-elevated, 32-bit, domain controller and SSH builds are refused; unknown builds are optimized with a warning; the report describes the run.

## Guest script tests

```sh
make test-ps
```

They run with Pester 5 or 6 under `powershell.exe` (5.1), the PowerShell the plugin uses on the guest. Registry and service-key reads run against a scratch key under HKCU used as the root, power reads use the real power API without writing, and `sc.exe`, `powercfg.exe` and the ScheduledTasks cmdlets are mocked. The entry-point tests run the read-only actions in a child process and check the JSON contract, the exit codes and that the output has no byte order mark.

## Live integration tests

`test/integration` builds the plugin, installs it into a private plugin directory, and runs Packer with the `null` builder over WinRM against an existing VM, so it tests the real binary, the real scripts and a real Packer run.

| Variable | Meaning |
| --- | --- |
| `WO_TEST_HOST` | Address of the VM. Without it, the tests are skipped. |
| `WO_TEST_USERNAME`, `WO_TEST_PASSWORD` | Administrator credentials. The username defaults to `Administrator`. |
| `WO_TEST_USE_SSL` | `true` for WinRM over HTTPS. |
| `WO_TEST_ALLOW_CHANGES` | Must be `yes` to run the test that changes the VM. |
| `WO_TEST_PROFILE` | Profile for the changing test. Defaults to `vdi`. |
| `WO_TEST_HYPERV_VM` | Optional Hyper-V VM name. A checkpoint is created before any change. |

```sh
WO_TEST_HOST=192.168.1.50 WO_TEST_PASSWORD=… make test-integration
```

`TestDryRun` is read-only. `TestApplyIdempotentRollback` applies the profile, applies it again and expects no changes, then rolls back and expects an empty journal. ⚠️ It changes the VM and restarts nothing, but use a disposable VM or a checkpoint. It never runs unless `WO_TEST_ALLOW_CHANGES=yes`.

## What has been verified

| Check | Result |
| --- | --- |
| `gofmt`, `go vet`, `staticcheck`, `doccheck`, `psdoccheck`, `catalogdoc -check` | clean |
| PSScriptAnalyzer (warnings and errors, Windows PowerShell 5.1 and pwsh) | clean |
| Go tests | pass |
| Pester 6.2 on Windows PowerShell 5.1 (Windows 11 25H2) | pass |
| Read-only guest actions (`Detect`, `Probe`, `Inspect`, `ReadJournal`) on Windows 11 25H2, non-elevated | correct JSON |
| `packer plugins install --path` and `packer validate` of the examples (Packer 1.16) | pass |
| Live apply and rollback against a VM | not run; requires a disposable VM and explicit authorization |
