# ⚡ packer-plugin-windows-optimize

A Packer provisioner that applies a small, validated set of Windows performance and image optimizations over WinRM. Every change is inspected first, recorded for rollback, applied, and verified by reading it back. Nothing that could cut off Packer's own WinRM session is changed, and nothing weakens Windows security.

## ✨ What it does

1. 🔍 Detects the OS, build, edition, installation type and architecture, the WinRM configuration with every service WinRM depends on, and the power schemes.
2. 🎯 Selects the optimizations of the chosen [profile](docs/profiles.md) that apply to that system. The rest are reported as unsupported.
3. 🛡️ Skips any optimization that touches a [protected](docs/connectivity.md) target: the WinRM service and its dependencies, networking, the firewall, authentication, Defender, Windows Update and event logging. When the guest state needed to decide is unknown, it skips (fails closed).
4. 🧭 Inspects every target. Optimizations already in place are **compliant** and left alone, so a second run changes nothing.
5. 📒 Records the original value of everything it is about to change in a [rollback journal](docs/architecture.md#rollback-journal) on the guest, **before** changing anything.
6. ✅ Applies the changes, reads every value back, and checks that WinRM still works and is configured as before.
7. 🔁 Optionally restarts, waits for WinRM, and verifies again.
8. 📊 Writes a JSON [report](docs/report.md) in every case, including failures.

`rollback = true` restores every journaled value and removes the journal.

## 💻 Supported operating systems

| OS | Build | Installation types |
| --- | --- | --- |
| Windows Server 2016 | 14393 | Desktop Experience, Server Core |
| Windows Server 2019 | 17763 | Desktop Experience, Server Core |
| Windows Server 2022 | 20348 | Desktop Experience, Server Core |
| Windows Server 2025 | 26100 | Desktop Experience, Server Core |
| Windows 10 | 14393 and later | 64-bit client editions |
| Windows 11 | 22000 and later | 64-bit client editions |

Each optimization declares the installation types, builds and editions it applies to. See the [compatibility matrix](docs/catalog.md#os-compatibility-matrix). Newer builds are accepted with a warning. Domain controllers and older releases are rejected. Details are in [docs/supported-os.md](docs/supported-os.md).

## 🚀 Usage

```hcl
packer {
  required_plugins {
    windows-optimize = {
      source  = "github.com/larskroghdk/windows-optimize"
      version = ">= 1.0.0"
    }
  }
}

build {
  sources = ["source.hyperv-vmcx.ws2025"]

  provisioner "windows-optimize" {
    profile            = "performance"
    exclude            = ["power.high_performance_scheme"]
    reboot_if_required = true
  }
}
```

| Profile | Contains |
| --- | --- |
| `recommended` (default) | Changes that are safe for every general-purpose VM template: no sleep, no hibernation, no Xbox services on servers, no first sign-in animation, no consumer experiences, no Server Manager at logon. |
| `performance` | `recommended` plus the High performance scheme, no disk idle, fewer background services and tasks, Required diagnostic data, no tips, widgets or feeds. |
| `vdi` | `performance` plus changes for non-persistent virtual desktops that cost features on a regular machine. Inspired by the Microsoft VDI guidance and Citrix Optimizer, each setting validated separately. |
| `custom` | Only what `include` lists. |

🔌 The communicator must be `winrm` with an elevated administrator session. All options are in [docs/configuration.md](docs/configuration.md), and complete templates are in [examples/](examples/).

## 📚 Documentation

| Document | Covers |
| --- | --- |
| [Installation](docs/installation.md) | Installing a release or a local build |
| [Configuration](docs/configuration.md) | Every option, IDs and patterns |
| [Profiles](docs/profiles.md) | What each profile selects and why |
| [Catalog](docs/catalog.md) | Every optimization: changes, impact, OS matrix, sources (generated) |
| [Supported OS](docs/supported-os.md) | Detection, classification and applicability |
| [Architecture](docs/architecture.md) | Run flow, guest actions, journal, design decisions |
| [WinRM protection](docs/connectivity.md) | How the Packer session is protected |
| [Security](docs/security.md) | What the plugin never changes, and the trust model |
| [Report](docs/report.md) | The JSON report |
| [Research](docs/research.md) | Sources evaluated, rejected optimizations, reuse of the original scripts |
| [Testing](docs/testing.md) | Unit, Pester, fake-guest and live tests |
| [Troubleshooting](docs/troubleshooting.md) | Common failures and what to do |

## 🔧 Development

Requires Go 1.26, `staticcheck`, Packer, `pwsh` with PSScriptAnalyzer, and Windows PowerShell 5.1 with Pester for the guest-script tests.

```sh
make check      # lint, typecheck, Go and Pester tests
make catalog    # regenerate docs/catalog.md from internal/catalog
make generate   # regenerate provisioner/config.hcl2spec.go
make build      # build the plugin binary
```

🤝 See [CONTRIBUTING.md](CONTRIBUTING.md). Agent guidance and hard constraints: [AGENTS.md](AGENTS.md) and [INVARIANTS.md](INVARIANTS.md).
