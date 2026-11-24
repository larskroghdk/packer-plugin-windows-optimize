# Security policy

The security model is documented in [docs/security.md](../docs/security.md): what the plugin never changes, changes with a security dimension, the trust model and release integrity.

## Supported versions

Only the latest release receives security fixes. Fixes ship in a new release, not as backports. Because the project is pre-1.0, a release with a fix can also contain breaking changes.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| Older releases | No |
| Unreleased `main` | Fixed in the next release |

The catalog is embedded in the plugin binary, so a fix to an optimization reaches users only in a new release.

## Reporting a vulnerability

Don't open a public issue, pull request or discussion. Use [private vulnerability reporting](https://github.com/larskroghdk/packer-plugin-windows-optimize/security/advisories/new), the "Report a vulnerability" button on the repository's Security tab.

Include:

- Plugin version, Packer version, builder and WinRM settings (HTTP or HTTPS, authentication type).
- Guest edition, version, build and installation type (Server Core or Desktop Experience).
- The `windows-optimize` provisioner block, the affected optimization IDs, and whether `dry_run` or `rollback` was set.
- Steps to reproduce, the impact, and what an attacker needs (network position, guest account, access to the build host).
- The relevant parts of the JSON report, the rollback journal or `PACKER_LOG=1` output.

Remove passwords, tokens, host names and IP addresses from reports and logs first; the report records the WinRM user name. Don't test against systems you don't own or aren't authorized to test. Reproduce on a disposable VM.

## Scope

In scope:

- A change to a control listed under [What the plugin never changes](../docs/security.md#what-the-plugin-never-changes), through a profile, `include`, a catalog entry or rollback.
- A change applied without clearing the protection rules, or a protection rule that doesn't fail closed when the WinRM dependency tree or power configuration is unknown.
- A WinRM regression after an apply that doesn't fail the build, or a restart without `reboot_if_required = true`.
- Configuration or guest data reaching PowerShell command text, registry paths or written values instead of selecting embedded catalog entries through the JSON files of the guest contract.
- Weak protection of the guest working directory or the rollback journal directory, for example a standard user being able to plant a journal, replace a script or redirect either directory through a link.
- A rollback that writes a value or target kind the catalog doesn't change, or that restores a planted journal.
- A report that shows an optimization as applied, compliant or restored when re-inspection doesn't read the wanted value.
- Credentials or signing material exposed in reports, journals, Packer output, uploaded files or release artifacts.
- Weaknesses in the CI or release workflows, such as unpinned Actions or excessive token permissions, that could compromise published binaries.

Out of scope:

- Vulnerabilities in Windows, WinRM or Windows PowerShell. Report these to the [Microsoft Security Response Center](https://msrc.microsoft.com/report).
- Vulnerabilities in Packer or the Packer plugin SDK. Report these through [HashiCorp's security policy](https://www.hashicorp.com/security).
- The documented effect of an optimization that the configuration selects, including those listed under [Changes with a security dimension](../docs/security.md#changes-with-a-security-dimension). Report a wrong or missing consideration as a bug.

## Process

The maintainer acknowledges the report, confirms or rejects it, and agrees a disclosure date with the reporter. This is a single-maintainer project, so response times are best effort.

Fixes are released in a new version and disclosed through a GitHub Security Advisory. A CVE is requested through GitHub when appropriate. Reporters are credited unless they prefer not to be.

## Verifying releases

Download `packer-plugin-windows-optimize_v<VERSION>_SHA256SUMS` and its detached `.sig` from the GitHub release. Verify the signature with GPG using the maintainer's release signing key, then check the plugin ZIP against the checksums.
