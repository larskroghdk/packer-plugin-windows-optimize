# 🛡️ WinRM protection

Packer talks to the guest over WinRM for the whole build. An optimization that stops a service WinRM needs, changes a firewall rule, or makes the machine sleep would end the build in the middle, often leaving the VM in an unknown state. The plugin prevents this in four layers.

## 1. Static rules

`plan.ProtectStatic` rejects any change to these targets, whatever the guest looks like:

| Area | Protected |
| --- | --- |
| Communicator | Services `WinRM`, `HTTP`, `RpcSs`, `RpcEptMapper`, `DcomLaunch`, `Winmgmt`, `Schedule`, `ProfSvc`, `SamSs`, `LSM`, `KeyIso`, `CryptSvc`; registry below `SOFTWARE\Policies\Microsoft\Windows\WinRM`, `SOFTWARE\Microsoft\Windows\CurrentVersion\WSMAN`, the `WinRM` and `HTTP` service keys |
| Networking and firewall | Services `BFE`, `MpsSvc`, `NSI`, `Tcpip`, `AFD`, `Dhcp`, `Dnscache`, `NlaSvc`, `netprofm`; registry below `SOFTWARE\Policies\Microsoft\WindowsFirewall`, `SharedAccess`, `MpsSvc`, `BFE`, `Tcpip`, `Tcpip6`, `Control\Network` |
| Authentication and remote UAC | `Control\Lsa`, `Control\SecurityProviders`, `EnableLUA`, `LocalAccountTokenFilterPolicy`, `FilterAdministratorToken` |
| Group Policy | Service `gpsvc` |
| Security and updates | See [security](security.md) |
| Packer | Scheduled tasks below `\packer` (Packer's elevated commands run as scheduled tasks) |
| Sleep | Any sleep or hibernate idle timeout other than 0 (never) |

A unit test checks that no catalog change matches a static rule. The rules are a safety net for future catalog entries, not the reason an existing optimization is skipped.

## 2. Dynamic rule: WinRM dependencies

`Detect` walks the `ServicesDependedOn` tree of the WinRM service on the guest. No optimization may change the start type of any service in that tree. If the tree couldn't be read, no service start type is changed at all (fail closed).

## 3. All or nothing

If one change of an optimization is protected, the whole optimization is skipped with the reason, so it is never left half applied.

## 4. Verification after the change

After applying, the plugin starts a **new** WinRM command (`Probe`) and compares the WinRM configuration with the one detected at the start:

- the service is running, and still starts automatically if it did before,
- every enabled listener still exists and is enabled,
- no authentication method that was enabled has been disabled,
- unencrypted traffic is still allowed if it was before,
- the session is still elevated.

Any regression fails the build, regardless of `fail_on_error`. After a restart, the plugin waits for the boot time to change, reinstalls the scripts and runs the same check.

## Restarts

The plugin never restarts on its own. With `reboot_if_required = true` it restarts only when an applied optimization needs it, using `shutdown.exe /r /f /t 5` so the command returns before the connection drops, then probes until the boot time changes or `restart_timeout` expires, uploading the guest scripts again after each failed attempt. Otherwise the report says a restart is required, so you can place a `windows-restart` provisioner where your workflow needs it.

## Rollback

Rollback restores the original values, including sleep timeouts that were not 0 before. Those are the values Windows had when the build started, so the session ran with them before. Run rollback as the last provisioner, or follow it with a `windows-restart`, when that matters.
