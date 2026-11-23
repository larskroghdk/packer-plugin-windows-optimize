# 🤝 Contributing

🐛 Bug reports, catalog proposals and protection rules are welcome. Open an issue before starting larger changes so we can agree on the approach.

## 🧰 Setup

You need Go 1.21 or later, which downloads the version in `go.mod`, plus `staticcheck`, Packer, `pwsh` with PSScriptAnalyzer, and Windows PowerShell 5.1 with Pester 5 or 6. `make init` installs the Go tools.

⚠️ `make` isn't on the default Git Bash PATH on Windows. If it's missing, run the recipe lines from the [Makefile](Makefile) directly.

## ✅ Before you open a pull request

- 🧪 Run `make check` (lint, typecheck and tests). It must pass.
- 📜 Read [INVARIANTS.md](INVARIANTS.md). A change that breaks an invariant won't be merged, even if the tests pass.
- 🐞 Add a test for every bug you fix. Guest-script tests must pass under Windows PowerShell 5.1, not only pwsh.
- 🏗️ For changes to `provisioner/` or the guest scripts, also run the [live integration tests](docs/testing.md#live-integration-tests) against a disposable VM and attach the report.

## 📏 Code rules

- 📝 **No comments except documentation (I19).** Every Go function and method gets a doc comment in this format, checked by `cmd/doccheck`:

  ```go
  // Classify maps detected OS facts to a supported release.
  //
  // Returns:
  //   - Release: The classified release.
  //   - error: The OS is not supported.
  //
  // Author: Lars Krogh
  // Version: 1.0.0
  ```

  Every PowerShell script and function gets comment-based help with `.SYNOPSIS`, `.DESCRIPTION`, `.PARAMETER`, `.EXAMPLE`, `.OUTPUTS` and `.NOTES` (`Author`, `Version`). Keep the existing author and raise the version when behavior changes.
- 🧠 **Decisions belong in Go (I14).** Guest scripts only detect, read and write state.
- 🔤 PowerShell files are ASCII with CRLF line endings. `guest_test.go` checks this.
- ⚙️ Run `make generate` after changing a `mapstructure` field in `provisioner/config.go`.
- 🔢 Increment `report.SchemaVersion` for an incompatible change to the report JSON.

## 📚 Adding an optimization

The [add-optimization](.agents/skills/add-optimization/SKILL.md) skill has the full checklist.

1. Find a Microsoft source that documents the setting, and check that it has an effect on each OS you declare. See the acceptance criteria in [research](docs/research.md#method).
2. Add it to `internal/catalog/optimizations.go` with the narrowest `Applies` rules, the lowest fitting profile, and `RebootRequired` if Windows needs a restart.
3. Use one of the six target kinds. If the setting needs another kind, open an issue first: a kind has to be readable, writable and restorable from the journal.
4. Run `make catalog` to regenerate [docs/catalog.md](docs/catalog.md), and `make check`.

A new `recommended` optimization must not need a restart and must cite a Microsoft source. `TestRecommendedProfileIsConservative` enforces both.

## 🔐 Protection rules

A target that could break the Packer session or weaken security belongs in `internal/plan/protect.go` (see the [add-protection-rule](.agents/skills/add-protection-rule/SKILL.md) skill). A rule must fail closed when the guest state it needs is unknown. See [WinRM protection](docs/connectivity.md).
