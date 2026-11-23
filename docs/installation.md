# 📦 Installation

## From a release

Add the plugin to `required_plugins` and run `packer init`:

```hcl
packer {
  required_plugins {
    windows-optimize = {
      source  = "github.com/larskroghdk/windows-optimize"
      version = ">= 0.1.0"
    }
  }
}
```

```sh
packer init .
```

Or install it directly:

```sh
packer plugins install github.com/larskroghdk/windows-optimize
```

Releases contain binaries for Windows, Linux and macOS on amd64 and arm64, so Packer can run on any of them. Only the guest must be Windows. They also contain a `SHA256SUMS` file and its detached GPG signature. Packer verifies the checksum when it installs the plugin.

## From source

```sh
go build -o packer-plugin-windows-optimize.exe .
packer plugins install --path packer-plugin-windows-optimize.exe github.com/larskroghdk/windows-optimize
```

A local build reports the version in `main.go` without a prerelease suffix, so it satisfies `version = ">= 0.1.0"`. [examples/local](../examples/local/) leaves the `required_plugins` entry out, so it always uses the installed local build.

## Requirements on the build

- The communicator must be `winrm`. The plugin refuses to run over SSH.
- The WinRM user must have an elevated administrator token. With a local account other than the built-in Administrator, this usually means `LocalAccountTokenFilterPolicy = 1`, which the plugin never changes.
- The guest must run 64-bit Windows PowerShell 5.1, which every supported Windows version includes.
- The guest needs write access to `C:\Windows\Temp` (working directory) and `C:\ProgramData` (rollback journal).
