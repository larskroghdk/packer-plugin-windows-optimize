# 🔐 Security

## What the plugin never changes

Performance doesn't justify weaker security, so these are protected by code (`internal/plan/protect.go`) and can't be selected by any profile, `include` pattern or future catalog entry:

| Control | Protected |
| --- | --- |
| Microsoft Defender | Services `WinDefend`, `WdNisSvc`, `Sense`; registry below `SOFTWARE\Policies\Microsoft\Windows Defender` and `SOFTWARE\Microsoft\Windows Defender`; tasks below `\Microsoft\Windows\Windows Defender\` |
| Windows Security | Services `SecurityHealthService`, `wscsvc` |
| Windows Update | Services `wuauserv`, `UsoSvc`, `WaaSMedicSvc`, `BITS`, `TrustedInstaller`; registry below `SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU`; the `DisableWindowsUpdateAccess` and `DoNotConnectToWindowsUpdateInternetLocations` policies; tasks below `WindowsUpdate`, `UpdateOrchestrator`, `WaaSMedic` |
| Event logging and auditing | Service `EventLog`; registry below the `EventLog` service key and `SOFTWARE\Policies\Microsoft\Windows\EventLog` |
| Firewall, network, authentication, UAC | See [WinRM protection](connectivity.md) |
| Diagnostic data service | `DiagTrack` can't be disabled; the data level is set by the supported `AllowTelemetry` policy instead |

The catalog also contains nothing that weakens passwords, accounts, SMB, TLS, SmartScreen, credential protection or exploit mitigations. Those were evaluated and rejected (see [research](research.md#rejected)).

## Changes with a security dimension

| ID | Profile | Consideration |
| --- | --- | --- |
| `windows_update.exclude_drivers` | vdi | Updates still install. Drivers must then come from your image pipeline, including driver security fixes. |
| `windows_update.delivery_optimization_http_only` | vdi | Downloads come directly from Microsoft or your WSUS/Configuration Manager source, never from peers. |
| `diagnostics.error_reporting_off` | vdi | Microsoft no longer receives crash reports from the machine. Event Log crash events and LocalDumps are unaffected. |
| `telemetry.compatibility_appraiser` | explicit only | Safeguard holds for feature updates no longer protect the device. |

## Trust model

- The plugin runs with the rights of the Packer WinRM user, which must be an elevated administrator. It refuses to run otherwise.
- Guest scripts are embedded in the plugin binary and uploaded to a new directory under `C:\Windows\Temp` for each run, and removed afterwards. The directory is created together with an ACL for SYSTEM and Administrators only, and an existing directory is refused unless one of them or the build user owns it. The scripts run with `-ExecutionPolicy Bypass` for that process only; the machine's execution policy is not changed.
- Every value written comes from the embedded catalog or, for rollback, from the journal. The configuration can only select catalog entries by ID; it can't supply registry paths or values.
- The rollback journal directory is created with an ACL for SYSTEM and Administrators only. An existing directory that is a link, or is owned by anyone other than SYSTEM, Administrators or the build user, is refused before the journal is read or written, so a standard user can't plant a journal in advance. A rollback restores only target kinds the plugin knows, and only values it can write (`REG_DWORD`, `REG_SZ`, absence, and the fixed service, task, power and hibernation states).
- No secrets are read, written or logged. The report contains OS facts, the WinRM user name, and the values of the targets.

## Release integrity

Release archives are listed in a `SHA256SUMS` file signed with the maintainer's GPG key. Packer checks the checksum on install.

## Reporting a vulnerability

Report security issues privately, as described in the [security policy](../.github/SECURITY.md), not in a public issue.
