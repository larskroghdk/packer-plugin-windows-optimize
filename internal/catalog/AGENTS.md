# internal/catalog/

The optimization model ([model.go](model.go)), the catalog itself ([optimizations.go](optimizations.go)) and its validation ([validate.go](validate.go)). Parent: [AGENTS.md](../../AGENTS.md). Constraints: [INVARIANTS.md](../../INVARIANTS.md) (I1–I6 apply directly).

## Local constraints

- An entry needs a Microsoft-documented source and must pass the acceptance criteria in [docs/research.md](../../docs/research.md#method). Never copy a setting from Citrix Optimizer, VDOT or a script without checking it against Microsoft's documentation.
- Use the narrowest `Applies` rules where Windows honors the setting: installation types, build range and, for policies Pro ignores, `Editions: enterpriseClient`.
- Build changes with the helpers (`registryDWORD`, `serviceStart`, `taskState`, `powerIndex`) so targets are well formed.
- The ID prefix must match the category (`idPrefixCategory`). IDs are stable: they appear in user templates and in journals on deployed machines. Never rename one.
- After any catalog change, run `make catalog` to regenerate `docs/catalog.md`. `make lint` fails if it's stale (I6).
- `All` returns a deep copy. Callers may modify the result.

## Footguns

- Registry paths are below HKLM, without the `HKLM\` prefix and without leading or trailing backslashes.
- Power setting targets leave `Scheme` empty. The plan resolves it, and `Validate` rejects a set scheme.
- Two optimizations may set the same target only on disjoint systems (`checkSharedTargets`). Otherwise the journal can't tell them apart.
- A `recommended` entry must need no restart and cite `learn.microsoft.com` (`TestRecommendedProfileIsConservative`). Put anything that needs a restart in a higher profile or in no profile.
- Moving an entry to a lower profile changes what existing templates apply on their next build. Treat it as a behavior change in the release notes.
