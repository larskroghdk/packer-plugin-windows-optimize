# 🩺 Troubleshooting

Start with the report (`report_path`). Every skipped, unsupported or failed optimization has a `reason`, and every change has `before`, `after` and `expected`. Run with `PACKER_LOG=1` to see each guest action and its output.

## The build is refused before anything changes

| Message | Cause and fix |
| --- | --- |
| `windows-optimize requires the WinRM communicator` | The build uses SSH. The plugin only runs over WinRM. |
| `the WinRM session (…) is not elevated` | The account isn't an administrator, or remote UAC filters its token. Use the built-in Administrator, or set `LocalAccountTokenFilterPolicy = 1` in the base image for local administrator accounts. |
| `the guest PowerShell process is not 64-bit` | Packer started 32-bit PowerShell (`SysWOW64`). Check `remote_path` or a custom shell configuration in the builder. |
| `cannot optimize …: unsupported Windows …` | A domain controller, Nano Server, Windows 8.1 or older, or a build older than 14393. See [supported OS](supported-os.md). |
| `…; no changes were made` | The scripts couldn't be uploaded or `Detect` failed. The rest of the message names the step. |

## Configuration errors

| Message | Fix |
| --- | --- |
| `profile: custom selects nothing on its own` | Add the optimization IDs to `include`. |
| `include[n]: "…" matches no optimization ID` | Check the ID against the [catalog](catalog.md). `*` matches one ID segment, for example `services.*`. |
| `exclude[n]: pattern "…" would exclude every optimization` | Choose a smaller profile instead of excluding everything. |
| `rollback: … cannot be combined with it` | A rollback restores the whole journal. Remove `profile`, `categories`, `include` and `exclude`. |
| `restart_timeout: must be at least 1m` | Use a longer timeout, for example `"15m"`. |

## An optimization is skipped or unsupported

| Reason | Meaning |
| --- | --- |
| `excluded by configuration (…)` | An `exclude` pattern matched. |
| `the WinRM service dependencies could not be detected, …` | `Detect` couldn't read the WinRM service tree, so no service is changed (fail closed). Look at `system.detection_errors`. |
| `… WinRM … depends on it` or another protection reason | A change hits a protection rule. This is intended and can't be overridden. See [WinRM protection](connectivity.md). |
| `the power configuration could not be detected` | The power API failed. Look at `system.detection_errors`. |
| `not changed because the current value could not be restored: …` | The current value has a type the journal can't restore (for example a `REG_BINARY` value, or a boot or system start service), so the optimization is left alone. |
| `none of its targets exist on this system` | The service or task isn't installed, for example the Xbox services on Server Core. This is normal. |
| An OS rule (installation type, build or edition) | The optimization doesn't apply to this OS. `ux.consumer_features` and `ux.windows_tips`, for example, are ignored by Windows Pro, so they are only applied to Enterprise and Education. |

## An optimization failed

| Reason | What to check |
| --- | --- |
| `apply failed: …` | The guest error. For services, `sc.exe` output; for tasks, `Disable-ScheduledTask`; for hibernation, `powercfg.exe`. |
| `verification failed: the value after applying is …` | Something reset the value right after it was written: Group Policy, MDM, a configuration management agent or a security product. Configure the setting there, or exclude the optimization. |
| `no longer in the wanted state after restart: …` | The value was reset during the restart, usually by Group Policy or MDM. |
| `inspection failed: …` | The target couldn't be read, for example because of an ACL on the registry key. |

With `fail_on_error = false` these become warnings and the build continues. A WinRM regression always fails the build.

## WinRM regression

`WinRM configuration regressed after applying optimizations: …` means the plugin found that WinRM changed after the apply step: a listener, an authentication method, the service, or elevation. No catalog optimization touches WinRM, so look for something else on the guest that reacts to the changes (a security agent or a scheduled script). Roll back with `rollback = true` and apply the optimizations one at a time with `profile = "custom"` and `include` to find the trigger. Please open an issue with the report.

## Restart

| Message | Fix |
| --- | --- |
| `the machine did not come back over WinRM within …` | Raise `restart_timeout`, or check that the WinRM listener and firewall rule survive a restart in the base image. |
| `cannot read the boot time before restarting` | The probe failed before the restart. The rest of the message has the WinRM error. |

The report says `reboot.required = true` when a restart is still needed but `reboot_if_required` is off. Add a `windows-restart` provisioner, or let sysprep's shutdown handle it.

## Rollback

| Symptom | Cause |
| --- | --- |
| `the rollback journal is empty` warning | Nothing was applied on this machine, or a previous rollback already restored everything. |
| `n journal entries could not be restored and were kept in the journal` | The failed entries stay in the journal with their reason in the report. Fix the cause and run the rollback again. |
| Rollback restores a value you changed after the build | The journal holds the value from before the plugin first changed it, not later changes. Remove the journal (`%ProgramData%\packer-plugin-windows-optimize\journal.json`) once you no longer want to roll back. |
