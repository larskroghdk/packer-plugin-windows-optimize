# 💻 Supported operating systems

## Detection

`Detect` reads `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion` (product name, `EditionID`, `InstallationType`, `DisplayVersion`, major, minor, build, UBR) and `Win32_OperatingSystem.ProductType`, plus the processor architecture and whether PowerShell runs as a 64-bit process. Version numbers come from `CurrentMajorVersionNumber` and `CurrentBuildNumber`, not from `ProductName`, which still says "Windows 10" on Windows 11.

## Classification

| Product type | Installation type | Classified as | Accepted builds |
| --- | --- | --- | --- |
| 1 (workstation) | `Client` | `client` | 14393 and later |
| 3 (server) | `Server` | `server_desktop` | 14393 and later |
| 3 (server) | `Server Core` | `server_core` | 14393 and later |
| 2 (domain controller) | any | rejected | — |

Anything else is rejected before a change is made: Windows 8.1 and older, Windows 10 before 1607, Nano Server, unknown installation or product types, 32-bit PowerShell, and sessions without an elevated token.

| Build | Server release | Client release |
| --- | --- | --- |
| 14393 | Windows Server 2016 | Windows 10 1607 |
| 17763 | Windows Server 2019 | Windows 10 1809 |
| 19044, 19045 | — | Windows 10 21H2, 22H2 |
| 20348 | Windows Server 2022 | — |
| 22621, 22631 | — | Windows 11 22H2, 23H2 |
| 26100 | Windows Server 2025 | Windows 11 24H2 |
| 26200 | — | Windows 11 25H2 |

Other builds in the accepted range are named by family and build (`Windows 11 build 27000`) and reported with `known_release: false` and a warning. They are optimized with every optimization whose build range covers them, so a new feature update doesn't break existing templates.

## Applicability

Each optimization declares:

- **installation types**: `client`, `server_desktop`, `server_core`,
- an optional **build range**, for example `ux.news_and_interests` only on Windows 10 2004 through 22H2 (builds 19041–19045), and `ux.widgets` from Windows 11 (22000),
- optional **editions**, for policies Windows only honors on some editions. `ux.consumer_features` and `ux.windows_tips` apply only to Enterprise, Enterprise multi-session, Education, IoT Enterprise and their N variants, because Windows Pro ignores them.

An optimization that doesn't apply is reported as `unsupported` with the reason, and its targets are never inspected or changed. An optimization whose targets don't exist on the system (a service or task that isn't installed) is also `unsupported`, so the same profile works on every installation type.

The full matrix is in [catalog.md](catalog.md#os-compatibility-matrix), generated from the same rules the plugin uses.

## Architecture

The plugin is tested on x64. ARM64 Windows is accepted, as long as Windows PowerShell runs as a 64-bit process, but it has not been tested. Registry access always uses the 64-bit view, so WOW64 redirection can't change which values are read or written.
