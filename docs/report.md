# 📊 Report

The plugin writes a JSON report to `report_path` at the end of every run, including failed runs, and prints a summary in the Packer log. `schema_version` changes when the format changes incompatibly.

```json
{
  "schema_version": 1,
  "plugin": "packer-plugin-windows-optimize 0.1.0",
  "started_at": "2026-11-19T09:12:03Z",
  "finished_at": "2026-11-19T09:12:41Z",
  "mode": "apply",
  "profile": "performance",
  "system": {
    "product_name": "Windows Server 2022 Datacenter",
    "release": "Windows Server 2022",
    "known_release": true,
    "installation": "server_desktop",
    "edition_id": "ServerDatacenter",
    "version": "10.0.20348.2700",
    "display_version": "21H2",
    "architecture": "AMD64",
    "winrm_user": "BUILD\\packer",
    "active_power_scheme": "381b4222-f694-41f0-9685-ff5bb260df2e",
    "detection_errors": []
  },
  "summary": { "selected": 18, "applied": 11, "compliant": 1, "pending": 0, "skipped": 0, "unsupported": 6, "failed": 0 },
  "journal": { "path": "C:\\ProgramData\\packer-plugin-windows-optimize\\journal.json", "entries": 22, "added": 22 },
  "connectivity": [
    { "name": "new WinRM command after applying optimizations", "at": "2026-11-19T09:12:39Z", "passed": true, "details": "a new command ran and the WinRM service, listeners and authentication are unchanged" }
  ],
  "reboot": { "required": false, "performed": false },
  "warnings": [],
  "optimizations": [
    {
      "id": "services.xbox_server",
      "name": "Disable Xbox Live services on Windows Server",
      "category": "services",
      "classification": "recommended",
      "selected_by": "profile performance",
      "status": "applied",
      "reboot_required": false,
      "references": ["https://learn.microsoft.com/windows-server/security/windows-services/security-guidelines-for-disabling-system-services-in-windows-server"],
      "changes": [
        { "id": "service:XblAuthManager", "kind": "service", "status": "applied", "expected": "disabled", "before": "manual", "after": "disabled" }
      ]
    }
  ]
}
```

## Fields

| Field | Meaning |
| --- | --- |
| `mode` | `apply`, `dry_run`, `rollback` or `rollback_dry_run`. |
| `system.known_release` | `false` for a build newer than the plugin's release tables. It is still optimized, with a warning. |
| `system.detection_errors` | Parts of `Detect` that failed. A missing WinRM or power configuration makes the dependent optimizations skip. |
| `journal.added` | Entries this run recorded. Entries from earlier runs are kept unchanged. |
| `connectivity` | Every WinRM check: after applying, after rollback, after restart. |
| `reboot.required` | An applied (or restored) change needs a restart to take full effect. |
| `rollback` | Present in rollback mode: counts and one entry per journal entry, with `optimization` naming its source. |

## Optimization status

| Status | Meaning |
| --- | --- |
| `applied` | Every pending change was applied and read back with the wanted value. |
| `compliant` | Every target already had the wanted value. Nothing was changed. |
| `pending` | Dry run only: something would change. |
| `skipped` | Excluded, protected, the guest state needed to decide is unknown, or a current value couldn't be restored later. `reason` says which. |
| `unsupported` | The OS rules don't match this system, or none of its targets exist (for example a service that isn't installed). |
| `failed` | A target couldn't be inspected, applied or verified, or didn't survive the restart. |

## Change status

`compliant`, `pending`, `applied`, `restored` (rollback), `not_applicable` (the target doesn't exist), `skipped`, `failed`. `before` and `after` are the values read from the guest, `expected` the value wanted.

## Using the report in a pipeline

```sh
jq -e '.summary.failed == 0 and .summary.pending == 0' windows-optimize-report-ws2022.json
jq -r '.optimizations[] | select(.status == "skipped") | "\(.id): \(.reason)"' windows-optimize-report-ws2022.json
```
