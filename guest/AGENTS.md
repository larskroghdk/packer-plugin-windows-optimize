# guest/

PowerShell embedded in the plugin (`//go:embed scripts/*.ps1`) and run on the Windows guest, one action per invocation. Parent: [AGENTS.md](../AGENTS.md). Constraints: [INVARIANTS.md](../INVARIANTS.md) (I14, I18 and I19 apply directly).

## Local constraints

- Target Windows PowerShell 5.1 (`#Requires -Version 5.1`). No PowerShell 7-only syntax: no `??`, ternaries, `-Parallel` or `Clean` blocks.
- Files are ASCII with CRLF line endings. `TestScriptsAreWindowsPowerShellSafe` fails otherwise.
- Each action writes one JSON output file through `Write-WOResult`, and reads at most one JSON input file through `Read-WOInput`. Exit 0 when the action ran. Report per-target failures in the item (`error`, `ok = false`). Exit 1 only when the whole action failed.
- Functions use the `WO` noun prefix. A new `WindowsOptimize.<Part>.ps1` must be added to the part list in `Invoke-WindowsOptimize.ps1`, or `TestScriptsAreComplete` fails.
- Guest output field names are the Go `json` tags in `internal/facts` and `internal/plan`. Rename them on both sides together, and update the fake guest in `provisioner/guest_test.go`.
- Read and write targets only. Never decide whether a change is safe or wanted (I14): the input has already been cleared by the plugin.
- Services are configured, never started or stopped.

## Footguns

- `Set-StrictMode -Version Latest` is on. Reading a missing property on a `ConvertFrom-Json` object throws.
- Windows PowerShell 5.1 `ConvertFrom-Json` emits a JSON array as one object. Assign it to a variable and output the variable to enumerate it (see `Read-WOInput`). A test that passes only under pwsh proves nothing.
- Write output as UTF-8 without a BOM (`UTF8Encoding($false)`). `Out-File` and `Set-Content -Encoding UTF8` in 5.1 add one.
- Use `Get-WORegistryRoot` (the 64-bit HKLM view), never `HKLM:` provider paths, so nothing goes through WOW64 redirection.
- `powercfg.exe` output is localized. Read power state through `WindowsOptimize.PowerApi` (powrprof P/Invoke), not by parsing text. `PowerReadACValueIndex` returns 2 when a setting doesn't exist in a scheme: that is "absent", not an error.
- Writing a power index doesn't take effect until the scheme is activated again. `Invoke-WOApply` does this once after all items.
- Native commands (`sc.exe`, `powercfg.exe`) go through `Invoke-WONativeCommand`, which checks `$LASTEXITCODE`. A bare call ignores failures.
- Scheduled task paths start and end with `\`. Match a task by path and name, never by name alone.
- In a comment-based help block, a line that starts with `.` followed by a word is read as a help keyword, and an unknown keyword makes PowerShell ignore the whole block silently. Check with `Get-Help <function>` after editing help.
- `sed -i` and some editors strip the CR from CRLF. Re-check line endings after a scripted edit.
