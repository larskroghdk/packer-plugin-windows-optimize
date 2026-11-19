#Requires -Version 5.1

<#
.SYNOPSIS
    Reads and sets service start types.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1. Reads the start type from the registry, so a
    missing service is reported as absent, and changes it through the service control
    manager with sc.exe, the only inbox tool in Windows PowerShell 5.1 that can set and
    clear delayed automatic start. Services are never started or stopped.

.EXAMPLE
    . .\WindowsOptimize.Services.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Get-WOServiceStartType {
    <#
    .SYNOPSIS
        Reads a service's start type from the registry.

    .DESCRIPTION
        Reads the Start and DelayedAutostart values under SYSTEM\CurrentControlSet\Services
        and returns automatic, automatic_delayed, manual, disabled, boot or system. A service
        that is not installed is reported as absent.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Name
        The service name (not the display name).

    .EXAMPLE
        Get-WOServiceStartType -Root $root -Name 'XblGameSave'

    .OUTPUTS
        System.Management.Automation.PSCustomObject with present, type and data.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [ValidatePattern('^[A-Za-z0-9_.\-]+$')]
        [string]$Name
    )
    $path = "SYSTEM\CurrentControlSet\Services\$Name"
    $start = Get-WORegistryValue -Root $Root -Path $path -Name 'Start'
    if (-not $start.present) {
        return [PSCustomObject]@{ present = $false; type = $null; data = $null }
    }
    $delayed = Get-WORegistryValue -Root $Root -Path $path -Name 'DelayedAutostart'
    $startType = switch ($start.data) {
        '0' { 'boot' }
        '1' { 'system' }
        '2' { if ($delayed.present -and $delayed.data -eq '1') { 'automatic_delayed' } else { 'automatic' } }
        '3' { 'manual' }
        '4' { 'disabled' }
        default { "unknown($($start.data))" }
    }
    [PSCustomObject]@{ present = $true; type = $null; data = $startType }
}

function Set-WOServiceStartType {
    <#
    .SYNOPSIS
        Changes a service's start type.

    .DESCRIPTION
        Runs sc.exe config with start= auto, delayed-auto, demand or disabled. The service
        control manager applies the change at once and keeps the service running or stopped
        as it is. Throws when the service does not exist or sc.exe fails.

    .PARAMETER Name
        The service name (not the display name).

    .PARAMETER StartType
        automatic, automatic_delayed, manual or disabled.

    .EXAMPLE
        Set-WOServiceStartType -Name 'XblGameSave' -StartType 'disabled'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidatePattern('^[A-Za-z0-9_.\-]+$')]
        [string]$Name,
        [Parameter(Mandatory)]
        [ValidateSet('automatic', 'automatic_delayed', 'manual', 'disabled')]
        [string]$StartType
    )
    $scStart = @{ automatic = 'auto'; automatic_delayed = 'delayed-auto'; manual = 'demand'; disabled = 'disabled' }[$StartType]
    $sc = Join-Path -Path $env:SystemRoot -ChildPath 'System32\sc.exe'
    $result = Invoke-WONativeCommand -FilePath $sc -ArgumentList @('config', $Name, 'start=', $scStart)
    if ($result.ExitCode -ne 0) {
        throw "sc.exe config $Name start= $scStart failed with exit code $($result.ExitCode): $($result.Output)"
    }
}
