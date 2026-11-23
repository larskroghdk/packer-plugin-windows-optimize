# Testing requirements

Run the checks defined by [.github/workflows/build.yml](../../.github/workflows/build.yml) and the project's existing Makefile or scripts before committing. Preserve formatting, generated-schema checks, Go tests/vet/static analysis, PSScriptAnalyzer and Windows PowerShell 5.1 Pester coverage where defined.

Run focused behavioral checks for changed contracts. Keep guest mutations confined to test fixtures or an explicitly authorized disposable VM. Do not enable live integration environment variables for ordinary checks. Mocked tests and cross-compilation are not evidence of live Windows validation.

Packaging changes require a snapshot with publication and signing skipped, followed by signed release asset verification when publication is authorized. Report actual checks and limitations.
