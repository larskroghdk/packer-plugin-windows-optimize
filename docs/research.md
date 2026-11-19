# 🔬 Research

How the catalog was built: which sources were evaluated, how each candidate was judged, and what was rejected and why.

## Method

A candidate became an optimization only if all of these hold:

1. **Documented.** Microsoft documents the setting (Policy CSP, ADMX reference, Windows Server service guidance, VDI guidance, `powercfg` reference). Community blogs and tool configurations were used to find candidates, never as the only source.
2. **Effective.** It changes something on a supported OS. Settings that are already the default, or that newer Windows ignores, were dropped.
3. **Reversible and verifiable.** It maps to one of the six target kinds the plugin can read, write and restore (registry `REG_DWORD`/`REG_SZ`, service start type, task state, power index, active scheme, hibernation).
4. **Safe for Packer.** It can't stop WinRM or anything WinRM depends on, change networking or the firewall, or suspend the machine.
5. **Not a security trade-off.** It doesn't weaken Defender, Windows Update, auditing, logging, authentication or exploit protection.
6. **Machine-wide.** It is a computer setting, not a per-user preference that can't be verified for future users.

Each accepted candidate was then classified as `recommended`, `optional` or `high_risk` from its impact, and given the narrowest OS rules that match where Windows honors it.

## Sources

| Source | Used for |
| --- | --- |
| [Guidance on disabling system services on Windows Server with Desktop Experience](https://learn.microsoft.com/windows-server/security/windows-services/security-guidelines-for-disabling-system-services-in-windows-server) | Server service changes. Only services marked "should be disabled" are `recommended`, and only "OK to disable" services are `optional`. |
| [Optimizing Windows configuration for VDI desktops](https://learn.microsoft.com/windows-server/remote/remote-desktop-services/remote-desktop-services-vdi-optimize-configuration) | VDI candidates and their rationale |
| [Virtual Desktop Optimization Tool](https://github.com/The-Virtual-Desktop-Team/Virtual-Desktop-Optimization-Tool) | VDI candidates. Its successor, the Windows Desktop Optimization Tool, uses the same approach. VDOT matches scheduled tasks by bare name, so a task with the same name in another folder is also disabled. This plugin always uses the full task path. |
| [Citrix Optimizer (CTX224676)](https://support.citrix.com/external/article/CTX224676/citrix-optimizer-tool.html), templates of version 3.5.0.13 | VDI candidates. Its Analyze, Execute and Rollback modes inspired the dry-run, apply and rollback design. Its rollback doesn't cover app removal, which is why app removal is out of scope here. |
| Policy CSP and ADMX references on Microsoft Learn | The exact registry value, data and supported editions of each policy (linked per optimization in the [catalog](catalog.md)) |
| [Powercfg command-line options](https://learn.microsoft.com/windows-hardware/design/device-experiences/powercfg-command-line-options), [Power and performance tuning](https://learn.microsoft.com/windows-server/administration/performance-tuning/hardware/power/power-performance-tuning) | Power scheme and setting GUIDs, which were also checked against `powercfg /aliases` on a live system |
| `Atea-TemplateFactory/DK-Factory/scripts/windows/100_OSLayer` | The scripts this plugin replaces (below) |

## The original scripts

Every script in `100_OSLayer` was evaluated. Many of them configure the image rather than optimize it, which is a different job with different owners, and belongs in a separate provisioner.

| Script | Outcome |
| --- | --- |
| `Disable-AteaFirstLogonAnimation` | ✅ `ux.first_logon_animation` |
| `Disable-AteaHibernation` | ✅ `power.hibernation_off` |
| `Disable-AteaOpeningServerManagerAtLogon` | ✅ `ux.server_manager_at_logon`, using the documented Group Policy value |
| `Disable-AteaPowerManagementTimeouts` | ✅ standby and hibernate timeouts → `power.no_idle_sleep`, disk timeout → `power.no_disk_idle`. ❌ Monitor timeout: no effect on performance in a VM. |
| `Disable-AteaServices` | ✅ `services.xbox_server`. The script only covered Server 2016. Microsoft's guidance applies to every Desktop Experience release, and the Xbox Live Game Save task is disabled too. |
| `Disable-AteaShutdownReasonPrompt`, `Disable-AteaShutdownTracker` | ✅ `diagnostics.shutdown_event_tracker_off`. The two scripts set the same policy. |
| `Disable-AteaWindowsErrorReporting` | ✅ `diagnostics.error_reporting_off`, in `vdi` only, because it removes diagnostics. |
| `Disable-AteaWindowsUpdateDriverSearch` | ✅ `windows_update.exclude_drivers`, in `vdi` only |
| `Set-AteaDiskSANPolicy` | ✅ `storage.new_disks_online`, `high_risk`: the registry value replaces the `diskpart` script, and it is never applied by a profile because it corrupts shared disks. |
| `Set-AteaTelemetryToBasic` | ✅ `telemetry.required_diagnostic_data` (Basic is now called Required) |
| `Set-AteaTelemetryToSecurity` | ❌ The Security level (0) is honored only on some editions, and it breaks Windows Update for Business reports. Required (1) is used instead. |
| `Disable-AteaNetNgenService` | ❌ The script itself is a no-op: disabling the .NET NGEN tasks delays native image generation and slows application start. |
| `Disable-AteaWindowsAdminCenterPopup` | ❌ `DoNotPopWACConsoleAtSMLaunch` is described only by community sources. |
| `Disable-AteaShowWelcomeTile` | ❌ Overwrites `ServerManager.exe.config`, a system file that servicing can replace. Not reversible from a journal. |
| `Disable-AteaIEEscAndWelcomeScreen` | ❌ IE Enhanced Security Configuration is a security control. |
| `Disable-AteaInternetExplorerFirstRun` | ❌ Internet Explorer is retired. No effect on current releases. |
| `Disable-AteaScreenSaver` | ❌ Per-user, and the screen saver lock is a security control. |
| `Disable-AteaAdministratorPasswordExpiration`, `Disable-AteaPasswordComplexity`, `Disable-AteaPasswordFromExpiring` | ❌ Weaken account security. |
| `Remove-AteaPowerShellISE`, `Remove-AteaLtmdm64Driver` | ❌ Component and driver removal is not reversible from a journal. |
| `Disable-AteaFindNetDevices`, `Disable-AteaNewNetworkLocationWizard`, `Suppress-AteaNetworkLocationPrompt`, `Disable-AteaIPv6TemporaryAddress`, `Set-AteaPreferIPv4OverIPv6`, `Enable-AteaEchoRequests`, `Enable-AteaFileAndPrinterSharing`, `Enable-AteaGCEMetadataServerAccess` | ⛔ Network and firewall configuration. Out of scope, and protected because it can break the WinRM session. Microsoft also advises against disabling or deprioritizing IPv6. |
| `Enable-AteaAutomaticUpdates`, `Enable-AteaRecommendedUpdates`, `Enable-AteaRepairViaWindowsUpdate` | ⛔ Windows Update configuration. Out of scope and protected. |
| `Enable-AteaRemoteDesktop`, `Enable-AteaRemoteDesktopKeepAlive`, `Set-AteaRemoteDesktopReconnectOptions`, `Enable-AteaEMS` | ⛔ Remote access configuration. Out of scope. |
| `Set-AteaCrashControlSettings` | ⛔ Crash dump configuration is an operational choice, not an optimization. |
| `Enable-AteaLongPaths` | ⛔ Functional configuration with no performance effect. |
| `Enable-AteaAutoTray`, `Enable-AteaWindowsBorders`, `Set-AteaExplorerViewOptions`, `Show-AteaExtensionsForKnownFileTypes` | ⛔ Per-user preferences. |
| `Add-AteaOSBranding`, `Add-AteaToolsConf`, `Copy-AteaSxsFolder`, `Set-AteaOSDiskLabel`, `Disable-AteaVMwareToolsGuestCustomization`, `Disable-AteaVMwareToolsUpdateNotifications` | ⛔ Branding, media staging and hypervisor tools. Out of scope. |

## Rejected

Besides the scripts above, these frequently recommended changes were evaluated and rejected:

| Candidate | Reason |
| --- | --- |
| `NtfsDisableLastAccessUpdate` | Last-access updates have been off by default since Windows Vista, and since Windows 10 1803 and Server 2019 the value is managed by Windows itself. Overriding it gains too little to justify taking over a value Windows manages. |
| Disable the `RegIdleBackup` task | The task has done nothing since Windows 10 1803, which stopped backing up the registry. |
| Disable the `DiagTrack` service | Microsoft supports controlling diagnostic data through the `AllowTelemetry` policy, not by disabling the service. |
| Disable Windows Search (`WSearch`) | Breaks search in Outlook, Start and File Explorer. Too broad for a profile. |
| Disable Defender, SmartScreen, Windows Update, UAC, the firewall | Security controls. Protected by code. |
| Remove inbox apps and capabilities | Not reversible, and the Store can bring them back. |
| Network adapter and TCP tuning | Hypervisor- and workload-specific, and can drop the WinRM connection. |
| Paging file and memory management changes | Workload-specific. Defaults are tuned by Windows. |
| Visual effects, wallpaper, per-user Explorer settings | Per-user. They can't be verified for users that don't exist yet. |

## Improvements over the original scripts

| Original | Plugin |
| --- | --- |
| Each script writes its value unconditionally | Every target is inspected first. Compliant targets are left alone and reported as compliant. |
| Success means "no exception" | Success means the value was read back and matches. |
| No undo | A journal records every original value before anything changes. `rollback = true` restores and verifies them. |
| OS checks by caption and build switch per script | One classification, and declared applicability per optimization, including editions where Windows ignores a policy. |
| Nothing prevents a script from breaking WinRM | Static and dynamic protection rules, all-or-nothing optimizations, and a WinRM comparison after the change. |
| Output is console text | A JSON report with before, after and reason for every change. |
| Restarts are left to chance | Restarts only on request, verified by boot time, followed by re-verification. |

The sibling project `packer-plugin-windows-harden` provided the architecture reused here: the Go-decides/guest-acts split, JSON file exchange per guest action, the WinRM probe and regression check, restart detection by boot time, and the documentation rules. One bug was found there while reusing it: in `guest/scripts/Invoke-SecureWindows.ps1`, the `Probe` action splits `Get-CimInstance` and `-ClassName Win32_OperatingSystem` over two lines, so the class name isn't passed. It was not changed in that project. The equivalent code here is correct and covered by a Pester test.

## Remaining opportunities

Candidates that need more validation before they can be added:

- **Microsoft Edge and OneDrive update tasks**: their names and folders change between versions, so they need detection by publisher, not by path.
- **Storage Sense policy** for persistent desktops: documented, but its effect overlaps with profile management products.
- **Windows Search on session hosts** with an explicit-only, high-risk classification.
- **Measured benchmarks** per optimization (boot time, idle CPU, disk I/O) on each supported OS, to replace qualitative impact descriptions with numbers.
