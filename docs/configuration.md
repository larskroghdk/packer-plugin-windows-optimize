# ⚙️ Configuration reference

💡 All options are optional. Validation runs in `packer validate`, so a typo fails before a VM is started.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `profile` | string | `recommended` | `recommended`, `performance`, `vdi` or `custom`. Profiles are cumulative: `performance` contains `recommended`, and `vdi` contains `performance`. `custom` selects nothing by itself and requires `include`. See [profiles](profiles.md). |
| `categories` | list(string) | all | Limits the profile to these categories: `power`, `services`, `scheduled_tasks`, `telemetry`, `user_experience`, `storage`, `windows_update`, `diagnostics`. `include` is not limited by it. |
| `include` | list(string) | `[]` | Optimization ID patterns to add regardless of profile and category. A **high-risk** optimization is only added by its exact ID, never by a wildcard. |
| `exclude` | list(string) | `[]` | Optimization ID patterns to skip. Exclude wins over profile and include. A pattern made only of `*` is rejected. |
| `dry_run` | bool | `false` | Detect, inspect and plan, then write the report. Nothing on the guest changes and no journal is written. |
| `rollback` | bool | `false` | Restore every value recorded in the rollback journal, verify it, and remove restored entries. Cannot be combined with `profile`, `categories`, `include` or `exclude`. With `dry_run`, it shows what would be restored. |
| `fail_on_error` | bool | `true` | Fail the build when an optimization fails to apply or verify, or a journal entry can't be restored. When `false`, those failures are warnings. A WinRM regression, an unsupported OS, or a failure to write the journal always fails the build. |
| `reboot_if_required` | bool | `false` | Restart when an applied optimization needs it, wait for WinRM, reinstall the scripts, check WinRM, and verify every applied and compliant change again. Without it, the report says a restart is required. |
| `restart_timeout` | duration | `30m` | How long to wait for WinRM after the restart. Minimum `1m`. |
| `report_path` | string | `windows-optimize-report-<build name>.json` | Where the [report](report.md) is written on the machine running Packer. Missing directories are created. |

## 🆔 Optimization IDs and patterns

IDs look like `category.name`, for example `power.no_idle_sleep` or `ux.widgets`. The full list is in [catalog.md](catalog.md).

Patterns match the whole ID case-insensitively, and `*` matches any sequence: `ux.*`, `*.xbox_*`, `tasks.*`. An `include` or `exclude` pattern that matches no ID is a configuration error, so a typo can't silently change what is applied.

```hcl
provisioner "windows-optimize" {
  profile = "performance"
  exclude = [
    "power.high_performance_scheme",
    "ux.*",
  ]
  include = [
    "storage.short_names_off",
  ]
}
```

## 🎯 Target IDs

The report and the journal identify each change by its target:

| Kind | ID format | Example |
| --- | --- | --- |
| Registry value | `registry:HKLM\<path>\<name>` | `registry:HKLM\SOFTWARE\Policies\Microsoft\Dsh\AllowNewsAndInterests` |
| Service start type | `service:<name>` | `service:XblGameSave` |
| Scheduled task state | `scheduled_task:<folder><name>` | `scheduled_task:\Microsoft\Windows\Maps\MapsToastTask` |
| Power setting index | `power_setting:<scheme>/<subgroup>/<setting>/<ac\|dc>` | `power_setting:381b4222-…/238c9fa8-…/29f6c1db-…/ac` |
| Active power scheme | `power_scheme:active` | |
| Hibernation | `hibernation` | |

Power settings are written to the scheme that will be active after the run: High performance when `power.high_performance_scheme` is applied and that scheme is installed, the active scheme otherwise.

## 🧩 Typical combinations

```hcl
# Plan only, to review the report before a real build.
provisioner "windows-optimize" {
  profile = "performance"
  dry_run = true
}

# Apply, restart if needed, and keep going if an optimization fails.
provisioner "windows-optimize" {
  profile            = "performance"
  reboot_if_required = true
  fail_on_error      = false
}

# Undo everything an earlier run of this plugin changed on this machine.
provisioner "windows-optimize" {
  rollback = true
}
```
