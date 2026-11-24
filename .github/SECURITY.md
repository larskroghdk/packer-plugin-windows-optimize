# Security policy

The security model is documented in [docs/security.md](../docs/security.md): what the plugin never changes, changes with a security dimension, the trust model and release integrity.

## Reporting a vulnerability

Don't open a public issue, pull request or discussion. Use [private vulnerability reporting](https://github.com/larskroghdk/packer-plugin-windows-optimize/security/advisories/new), the "Report a vulnerability" button on the repository's Security tab.

Include the plugin and Packer versions, the guest release, build and installation type, the `windows-optimize` provisioner block, steps to reproduce, and the impact. Remove passwords, tokens, host names and IP addresses from reports and logs first; the report records the WinRM user name.
