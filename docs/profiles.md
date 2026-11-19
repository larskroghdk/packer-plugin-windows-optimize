# 🎯 Profiles

A profile is a named selection of optimizations. Profiles are cumulative: `performance` contains everything in `recommended`, and `vdi` contains everything in `performance`. On every system the plugin applies only the optimizations whose OS rules match, and reports the rest as unsupported. The OS columns are in the [compatibility matrix](catalog.md#os-compatibility-matrix).

No profile turns off Defender, Windows Update, auditing, event logging, the firewall or any authentication control. Those targets are protected by code and can't be selected at all (see [security](security.md)).

## recommended (default)

For every general-purpose VM template: servers, clients, golden images that are sysprepped and reused. Each change is documented by Microsoft, needs no restart, and costs no feature a VM uses.

| ID | Why |
| --- | --- |
| `power.no_idle_sleep` | A VM that sleeps is unreachable. The Balanced scheme on Windows client sleeps after 30 minutes on AC power. |
| `power.hibernation_off` | Removes `hiberfil.sys` (a large share of guest RAM) and Fast Startup, so images get smaller and shutdowns are real shutdowns. |
| `services.xbox_server` | Microsoft's Windows Server service guidance says the Xbox Live services should be disabled. |
| `ux.first_logon_animation` | Every new profile on a cloned VM signs in without the "Hi" animation. |
| `ux.consumer_features` | No suggested and promoted apps for new users. Honored on Enterprise and Education only, so other editions report it as unsupported. |
| `ux.server_manager_at_logon` | Server Manager doesn't start for every administrator who signs in. |

## performance

`recommended` plus changes that remove background work a VM template rarely needs. Each one has a documented cost under *Impact* in the [catalog](catalog.md). Use it for servers and persistent desktops where you control the workload.

| ID | Why |
| --- | --- |
| `power.high_performance_scheme` | Highest processor performance states. Skipped when the scheme isn't installed. The sleep and disk settings then go to this scheme. |
| `power.no_disk_idle` | Disks are never powered down. |
| `services.server_consumer` | Maps, geolocation, mobile hotspot and phone services on servers, which Microsoft's guidance marks as OK to disable. |
| `tasks.maps`, `tasks.winsat`, `tasks.power_efficiency_diagnostics` | Periodic tasks that do nothing useful in a VM. |
| `telemetry.required_diagnostic_data` | Limits diagnostic data to Required through the documented policy. The DiagTrack service keeps running, as Microsoft supports. |
| `telemetry.ceip_tasks` | Customer Experience Improvement Program collection tasks. |
| `ux.windows_tips`, `ux.widgets`, `ux.news_and_interests`, `ux.search_highlights` | Content that is downloaded and refreshed in the background on Windows client. |

## vdi

`performance` plus changes for non-persistent virtual desktops, where the machine is reset or rebuilt instead of maintained. They are inspired by the [Microsoft VDI guidance](https://learn.microsoft.com/windows-server/remote/remote-desktop-services/remote-desktop-services-vdi-optimize-configuration), the [Virtual Desktop Optimization Tool](https://github.com/The-Virtual-Desktop-Team/Virtual-Desktop-Optimization-Tool) and [Citrix Optimizer](https://support.citrix.com/external/article/CTX224676/citrix-optimizer-tool.html), and validated one by one: only settings that are documented, reversible and don't weaken security were kept.

| ID | Cost on a regular machine |
| --- | --- |
| `services.xbox_client` | Xbox app, Game Bar and Xbox Live sign-in stop working. |
| `services.sysmain` | No prefetching of frequently used applications. |
| `tasks.scheduled_defrag` | No weekly retrim, which returns freed blocks to thin-provisioned storage. Don't use it for persistent machines. |
| `tasks.memory_diagnostics` | No scheduled memory diagnostics. |
| `storage.system_restore_off` | No restore points. |
| `windows_update.exclude_drivers` | Drivers come only from your own image or tooling. |
| `windows_update.delivery_optimization_http_only` | No peer-to-peer update sharing between desktops. |
| `diagnostics.error_reporting_off` | Crash reports are no longer queued or sent to Microsoft. Local crash dumps configured with LocalDumps keep working. |
| `diagnostics.shutdown_event_tracker_off` | Servers no longer ask why they are shut down. |

## custom and explicit-only optimizations

`profile = "custom"` selects only what `include` lists. `include` also adds optimizations to any other profile. Four optimizations are in no profile and are only applied when included:

| ID | Class | Why it is not in a profile |
| --- | --- | --- |
| `storage.short_names_off` | optional | Old installers and applications that depend on 8.3 names can fail. Needs a restart. |
| `telemetry.compatibility_appraiser` | high_risk | Windows Update uses the appraiser's data for feature update safeguard holds, so a device can be offered an update with a known compatibility issue. |
| `ux.background_apps` | high_risk | Packaged apps that rely on background activity, such as notifications and sync, stop working correctly while minimized. |
| `storage.new_disks_online` | high_risk | Shared disks, for example in a failover cluster, come online on every node that sees them, and two nodes writing to one disk corrupt it. Needs a restart. |

A high-risk optimization is only selected by its exact ID. `include = ["storage.*"]` adds `storage.short_names_off` and `storage.system_restore_off`, but not `storage.new_disks_online`.

## Choosing

| Image | Profile |
| --- | --- |
| General-purpose server or client template | `recommended` |
| Application or infrastructure server template, persistent desktop | `performance`, excluding what you need |
| Non-persistent VDI or session host | `vdi` |
| A specific change only | `custom` with `include` |

Run with `dry_run = true` first. The report lists every selected optimization with its current and wanted values.
