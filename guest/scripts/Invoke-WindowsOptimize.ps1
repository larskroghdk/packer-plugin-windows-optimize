#Requires -Version 5.1

<#
.SYNOPSIS
    Runs one windows-optimize guest action and writes its result as JSON.

.DESCRIPTION
    Entry point that the Packer plugin starts over WinRM, once per action. It loads the
    WindowsOptimize.*.ps1 parts from its own directory, reads the JSON request from
    -InputPath when the action takes one, and writes the JSON result to -OutputPath.
    The script only detects, reads and writes state. Every decision is made by the plugin.
    Detect       collects operating system, session, WinRM and power facts.
    Probe        re-reads WinRM, the session and the boot time to check connectivity.
    Inspect      reads the current value of the requested targets.
    Apply        sets the requested targets to the requested values.
    ReadJournal  returns the rollback journal and its path.
    WriteJournal replaces the rollback journal with an uploaded file, or deletes it.
    Exits 0 when the action ran, even if individual targets failed. Exits 1 when the
    action itself failed, with the error on stderr.

.PARAMETER Action
    The action to run.

.PARAMETER InputPath
    JSON request file. Required by Inspect, Apply and WriteJournal.

.PARAMETER OutputPath
    Where the JSON result is written.

.EXAMPLE
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\Invoke-WindowsOptimize.ps1 -Action Detect -OutputPath C:\Windows\Temp\wo\01-detect-output.json

.EXAMPLE
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\Invoke-WindowsOptimize.ps1 -Action Inspect -InputPath C:\Windows\Temp\wo\03-inspect-input.json -OutputPath C:\Windows\Temp\wo\03-inspect-output.json

.OUTPUTS
    None. The result is written to -OutputPath. Progress lines go to stdout.

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>

[CmdletBinding()]
param (
    [Parameter(Mandatory)]
    [ValidateSet('Detect', 'Probe', 'Inspect', 'Apply', 'ReadJournal', 'WriteJournal')]
    [string]$Action,
    [Parameter()]
    [string]$InputPath,
    [Parameter(Mandatory)]
    [string]$OutputPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

trap {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    [Console]::Error.WriteLine("ERROR: $($_.ScriptStackTrace)")
    exit 1
}

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
foreach ($part in 'NativeCommand', 'Registry', 'Services', 'ScheduledTasks', 'Power', 'Facts') {
    . (Join-Path -Path $scriptRoot -ChildPath "WindowsOptimize.$part.ps1")
}

function Write-WOLog {
    <#
    .SYNOPSIS
        Writes a progress line to standard output.

    .DESCRIPTION
        Writes the message straight to the console output stream so that Packer shows it in
        the build log. It bypasses the PowerShell pipeline, so the line never ends up in a
        function's return value.

    .PARAMETER Message
        The line to write.

    .EXAMPLE
        Write-WOLog 'Set service:XblGameSave = disabled'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Message
    )
    [Console]::Out.WriteLine($Message)
}

function Read-WOInput {
    <#
    .SYNOPSIS
        Reads the JSON request that the plugin uploaded for the current action.

    .DESCRIPTION
        Parses the file as UTF-8 JSON. A JSON array is written to the pipeline one element
        at a time, so wrapping the call in @() gives the same result in Windows PowerShell 5.1
        and PowerShell 7.

    .PARAMETER Path
        The request file, the script's -InputPath. Empty when the plugin did not send one.

    .EXAMPLE
        $items = @(Read-WOInput -Path $InputPath)

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter()]
        [AllowEmptyString()]
        [string]$Path
    )
    if ([string]::IsNullOrEmpty($Path)) {
        throw 'This action requires -InputPath'
    }
    $parsed = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8) | ConvertFrom-Json
    $parsed
}

function Write-WOResult {
    <#
    .SYNOPSIS
        Writes the action result to the output file as JSON.

    .DESCRIPTION
        Serializes the result to compact JSON and writes it as UTF-8 without a byte order
        mark. The plugin downloads and decodes this file.

    .PARAMETER Path
        The result file, the script's -OutputPath.

    .PARAMETER Result
        The object to serialize. Property names are the JSON field names the Go side decodes.

    .EXAMPLE
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{ path = $path })

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param (
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [object]$Result
    )
    $json = ConvertTo-Json -InputObject $Result -Depth 10 -Compress
    [IO.File]::WriteAllText($Path, $json, (New-Object System.Text.UTF8Encoding ($false)))
}

function Get-WOTargetState {
    <#
    .SYNOPSIS
        Reads the current value of one target.

    .DESCRIPTION
        Dispatches on the target kind to the matching part. The result always has present,
        type and data, the shape the plugin compares with the wanted value.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Target
        The target as sent by the plugin.

    .EXAMPLE
        Get-WOTargetState -Root $root -Target $item.target

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
        [object]$Target
    )
    switch ($Target.kind) {
        'registry' {
            return Get-WORegistryValue -Root $Root -Path $Target.path -Name $Target.name
        }
        'service' {
            return Get-WOServiceStartType -Root $Root -Name $Target.name
        }
        'scheduled_task' {
            return Get-WOScheduledTaskState -TaskPath $Target.path -TaskName $Target.name
        }
        'power_setting' {
            $params = @{
                Scheme      = $Target.scheme
                Subgroup    = $Target.subgroup
                Setting     = $Target.setting
                PowerSource = $Target.power_source
            }
            return Get-WOPowerSettingIndex @params
        }
        'power_scheme' {
            return [PSCustomObject]@{ present = $true; type = $null; data = Get-WOActivePowerScheme }
        }
        'hibernation' {
            return Get-WOHibernation -Root $Root
        }
    }
    throw "Unsupported target kind '$($Target.kind)'"
}

function Set-WOTargetState {
    <#
    .SYNOPSIS
        Sets one target to the wanted value.

    .DESCRIPTION
        Dispatches on the target kind to the matching part. A registry value whose wanted
        state is absent is deleted. Every other kind is set to the wanted data.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Target
        The target as sent by the plugin.

    .PARAMETER Want
        The wanted value, with present, type and data.

    .EXAMPLE
        Set-WOTargetState -Root $root -Target $item.target -Want $item.want

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [object]$Target,
        [Parameter(Mandatory)]
        [object]$Want
    )
    switch ($Target.kind) {
        'registry' {
            if ($Want.present) {
                Set-WORegistryValue -Root $Root -Path $Target.path -Name $Target.name -Type $Want.type -Data $Want.data
            }
            else {
                Remove-WORegistryValue -Root $Root -Path $Target.path -Name $Target.name
            }
            return
        }
        'service' {
            Set-WOServiceStartType -Name $Target.name -StartType $Want.data
            return
        }
        'scheduled_task' {
            Set-WOScheduledTaskState -TaskPath $Target.path -TaskName $Target.name -State $Want.data
            return
        }
        'power_setting' {
            $params = @{
                Scheme      = $Target.scheme
                Subgroup    = $Target.subgroup
                Setting     = $Target.setting
                PowerSource = $Target.power_source
                Index       = [uint32]::Parse($Want.data, [Globalization.CultureInfo]::InvariantCulture)
            }
            Set-WOPowerSettingIndex @params
            return
        }
        'power_scheme' {
            Set-WOActivePowerScheme -Scheme $Want.data
            return
        }
        'hibernation' {
            Set-WOHibernation -State $Want.data
            return
        }
    }
    throw "Unsupported target kind '$($Target.kind)'"
}

function Invoke-WOInspect {
    <#
    .SYNOPSIS
        Reads the current value of each requested target.

    .DESCRIPTION
        Returns one state object per item, in input order. A failure on one target is
        reported in its error field and never stops the others.

    .PARAMETER Items
        The requests, each with an id and a target.

    .EXAMPLE
        Invoke-WOInspect -Items @(Read-WOInput -Path $InputPath)

    .OUTPUTS
        System.Management.Automation.PSCustomObject with id, present, type, data and error.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object[]]$Items
    )
    $root = Get-WORegistryRoot
    try {
        foreach ($item in $Items) {
            $state = $null
            $failure = $null
            try {
                $state = Get-WOTargetState -Root $root -Target $item.target
            }
            catch {
                $failure = $_.Exception.Message
            }
            [PSCustomObject]@{
                id      = $item.id
                present = if ($null -ne $state) { [bool]$state.present } else { $false }
                type    = if ($null -ne $state) { $state.type } else { $null }
                data    = if ($null -ne $state) { $state.data } else { $null }
                error   = $failure
            }
        }
    }
    finally {
        $root.Dispose()
    }
}

function Invoke-WOApply {
    <#
    .SYNOPSIS
        Sets each requested target to its wanted value.

    .DESCRIPTION
        Applies the items one at a time, in input order, and returns one result per item. A
        failure on one item does not stop the rest. When a power setting of the active scheme
        was written, the active scheme is activated again so the new index takes effect; if
        that fails, those power settings are reported as failed.

    .PARAMETER Items
        The requests, each with an id, a target and the wanted value, already cleared by the
        plugin's protection rules.

    .EXAMPLE
        Invoke-WOApply -Items @(Read-WOInput -Path $InputPath)

    .OUTPUTS
        System.Management.Automation.PSCustomObject with id, ok and error.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [object[]]$Items
    )
    $root = Get-WORegistryRoot
    $results = New-Object System.Collections.Generic.List[object]
    $powerResults = New-Object System.Collections.Generic.List[object]
    try {
        foreach ($item in $Items) {
            $failure = $null
            try {
                Set-WOTargetState -Root $root -Target $item.target -Want $item.want
                Write-WOLog "Set $($item.id) = $(if ($item.want.present) { $item.want.data } else { '(absent)' })"
            }
            catch {
                $failure = $_.Exception.Message
                Write-WOLog "Failed to set $($item.id): $failure"
            }
            $result = [PSCustomObject]@{ id = $item.id; ok = ($null -eq $failure); error = $failure }
            $results.Add($result)
            if ($item.target.kind -eq 'power_setting' -and $result.ok) {
                $powerResults.Add([PSCustomObject]@{ scheme = $item.target.scheme; result = $result })
            }
        }
    }
    finally {
        $root.Dispose()
    }
    if ($powerResults.Count -gt 0) {
        try {
            $active = Get-WOActivePowerScheme
            $affected = @($powerResults | Where-Object { $_.scheme -eq $active })
            if ($affected.Count -gt 0) {
                Set-WOActivePowerScheme -Scheme $active
                Write-WOLog "Reactivated power scheme $active"
            }
        }
        catch {
            $failure = "the power setting was written but the active scheme could not be reactivated: $($_.Exception.Message)"
            foreach ($entry in $powerResults) {
                $entry.result.ok = $false
                $entry.result.error = $failure
            }
        }
    }
    $results
}

function Get-WOJournalPath {
    <#
    .SYNOPSIS
        Returns the path of the rollback journal.

    .DESCRIPTION
        The journal lives in ProgramData so it survives the removal of the working directory
        and restarts, and a later build or the rollback option can read it.

    .EXAMPLE
        Get-WOJournalPath

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param()
    Join-Path -Path $env:ProgramData -ChildPath 'packer-plugin-windows-optimize\journal.json'
}

function Initialize-WOJournalDirectory {
    <#
    .SYNOPSIS
        Creates the journal directory and restricts it to administrators.

    .DESCRIPTION
        ProgramData lets standard users create files, so the directory is limited to SYSTEM
        and the Administrators group, without inherited entries. A standard user can then
        not plant or edit the journal that a rollback would apply.

    .PARAMETER Path
        The directory.

    .EXAMPLE
        Initialize-WOJournalDirectory -Path (Split-Path -Parent (Get-WOJournalPath))

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        $null = New-Item -Path $Path -ItemType Directory
    }
    $acl = New-Object System.Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    $inherit = [System.Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
    foreach ($sid in 'S-1-5-18', 'S-1-5-32-544') {
        $identity = New-Object System.Security.Principal.SecurityIdentifier ($sid)
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule ($identity, 'FullControl', $inherit, 'None', 'Allow')
        $acl.AddAccessRule($rule)
    }
    $acl.SetOwner((New-Object System.Security.Principal.SecurityIdentifier ('S-1-5-32-544')))
    Set-Acl -LiteralPath $Path -AclObject $acl
}

switch ($Action) {
    'Detect' {
        Write-WOResult -Path $OutputPath -Result (Get-WOSystemFact)
    }
    'Probe' {
        $winrm = $null
        $failure = $null
        try {
            $winrm = Get-WOWinRMConfiguration
        }
        catch {
            $failure = $_.Exception.Message
        }
        $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{
                winrm     = $winrm
                session   = Get-WOSessionIdentity
                boot_time = $os.LastBootUpTime.ToUniversalTime().ToString('o', [Globalization.CultureInfo]::InvariantCulture)
                error     = $failure
            })
    }
    'Inspect' {
        $items = @(Read-WOInput -Path $InputPath)
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{ items = @(Invoke-WOInspect -Items $items) })
    }
    'Apply' {
        $items = @(Read-WOInput -Path $InputPath)
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{ items = @(Invoke-WOApply -Items $items) })
    }
    'ReadJournal' {
        $path = Get-WOJournalPath
        $content = ''
        if (Test-Path -LiteralPath $path -PathType Leaf) {
            $content = [IO.File]::ReadAllText($path, [Text.Encoding]::UTF8)
        }
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{ path = $path; content = $content })
    }
    'WriteJournal' {
        $request = Read-WOInput -Path $InputPath
        $path = Get-WOJournalPath
        if ([string]::IsNullOrEmpty($request.source)) {
            if (Test-Path -LiteralPath $path -PathType Leaf) {
                Remove-Item -LiteralPath $path -Force
                Write-WOLog "Removed the rollback journal $path"
            }
        }
        else {
            Initialize-WOJournalDirectory -Path (Split-Path -Parent $path)
            $temporary = "$path.tmp"
            Copy-Item -LiteralPath $request.source -Destination $temporary -Force
            Move-Item -LiteralPath $temporary -Destination $path -Force
            Write-WOLog "Wrote the rollback journal $path"
        }
        Write-WOResult -Path $OutputPath -Result ([PSCustomObject]@{ path = $path })
    }
}
exit 0
