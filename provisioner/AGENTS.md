# provisioner/

Packer-facing layer: config decoding and validation, run orchestration (`optimize`, `rollback`), guest access and restart handling. Parent: [AGENTS.md](../AGENTS.md). Constraints: [INVARIANTS.md](../INVARIANTS.md) (I9, I10, I11, I13 and I16 apply directly).

## Local constraints

- Validate options in `Config.prepare` ([config.go](config.go)), and collect every error with `packer.MultiErrorAppend` instead of failing on the first.
- Every guest call goes through `remote.action` ([remote.go](remote.go)). Don't build PowerShell command lines anywhere else.
- Record the journal (`recordJournal`) before `applyAndInspect`. If the journal can't be written, nothing is applied.
- Run `checkConnectivity` after every apply and restore. Its failure is returned directly, never through `run.failure`, so `fail_on_error = false` can't hide it.
- Take the time from `run.now`, not `time.Now`. Report times and restart deadlines are computed from it.
- A change to a guest action must be mirrored in the fake guest in [guest_test.go](guest_test.go).

## Footguns

- Adding or renaming a `Config` field without `make generate` leaves `config.hcl2spec.go` stale. HCL2 templates then reject the option. `TestConfigSpec` catches it.
- `fail_on_error` is a `config.Trilean`. Unset means true (`failOnError` is `!False()`).
- After a restart the working directory under `C:\Windows\Temp` may be gone. `probeUntil` uploads the scripts again after a failed attempt, and `restartAndReverify` installs them before any further step. A new post-restart step must come after that.
- `restartCommand` uses `/t 5` so the command returns before the connection drops. Don't change it to `/t 0`.
- The report is written even when the run fails. A report-write error is returned only when the run itself succeeded.
- The fake guest runs real `RemoteCmd`s. It must call `cmd.SetExited` from a goroutine, or `RunWithUi` blocks.
