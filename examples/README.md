# 📦 Examples

| Example | Builder | Use it to |
| --- | --- | --- |
| [local](local/) | `null` | Run the plugin against an existing Windows machine over WinRM: preview, apply or roll back. |
| [hyperv](hyperv/) | `hyperv-vmcx` | Clone a prepared base VM, preview, optimize, sysprep and export a template. |

Both need the plugin installed (see [installation](../docs/installation.md)) and `packer init`.

## local

```sh
cd examples/local
cp local.pkrvars.hcl.example local.pkrvars.hcl
packer init .
packer build -var-file=local.pkrvars.hcl .
```

The example file starts with `dry_run = true`, so the first run only writes the plan to `reports/local.json`. Set `dry_run = false` to apply, and `rollback = true` to restore the original values from the journal. ⚠️ Without a dry run, this changes the target machine.

## hyperv

The base VM must exist in Hyper-V with WinRM enabled for the Administrator account, and be in its final state before optimization (updates, drivers, applications).

```sh
cd examples/hyperv
packer init .
packer build -var-file=win11.pkrvars.hcl -var winrm_password=… .
```

| Variables file | Base VM | Template | Profile |
| --- | --- | --- | --- |
| `win11.pkrvars.hcl` | `win11-base` | `win11-optimized` | `performance` |
| `ws2022.pkrvars.hcl` | `ws2022-base` | `ws2022-optimized` | `performance` |
| `ws2025.pkrvars.hcl` | `ws2025-base` | `ws2025-optimized` | `recommended` |

The build runs the plugin twice: a dry run that writes `reports/<template>-plan.json`, then the apply run that writes `reports/<template>.json` and restarts if an applied optimization needs it. Packer then runs sysprep through `shutdown_command` and exports the VM to `output_directory`. Machine-specific values go in `site.pkrvars.hcl`, which git ignores.
