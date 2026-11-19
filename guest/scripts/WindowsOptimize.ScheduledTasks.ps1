#Requires -Version 5.1

<#
.SYNOPSIS
    Reads and sets the enabled state of scheduled tasks.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1. Tasks are addressed by their full folder path
    and name, never by name alone, so a task with the same name in another folder is never
    touched.

.EXAMPLE
    . .\WindowsOptimize.ScheduledTasks.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Get-WOScheduledTaskState {
    <#
    .SYNOPSIS
        Reads whether a scheduled task is enabled.

    .DESCRIPTION
        Returns data 'disabled' when the task state is Disabled and 'enabled' otherwise. A task
        that does not exist is reported as absent.

    .PARAMETER TaskPath
        The task folder, starting and ending with a backslash.

    .PARAMETER TaskName
        The task name.

    .EXAMPLE
        Get-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask'

    .OUTPUTS
        System.Management.Automation.PSCustomObject with present, type and data.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidatePattern('^\\.*\\$')]
        [string]$TaskPath,
        [Parameter(Mandatory)]
        [ValidateNotNullOrEmpty()]
        [string]$TaskName
    )
    $task = Get-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($null -eq $task) {
        return [PSCustomObject]@{ present = $false; type = $null; data = $null }
    }
    $state = if ($task.State.ToString() -eq 'Disabled') { 'disabled' } else { 'enabled' }
    [PSCustomObject]@{ present = $true; type = $null; data = $state }
}

function Set-WOScheduledTaskState {
    <#
    .SYNOPSIS
        Enables or disables a scheduled task.

    .DESCRIPTION
        Calls Enable-ScheduledTask or Disable-ScheduledTask. A running instance is not
        stopped. Throws when the task does not exist or access is denied, which is the case
        for some tasks owned by TrustedInstaller.

    .PARAMETER TaskPath
        The task folder, starting and ending with a backslash.

    .PARAMETER TaskName
        The task name.

    .PARAMETER State
        enabled or disabled.

    .EXAMPLE
        Set-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask' -State 'disabled'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidatePattern('^\\.*\\$')]
        [string]$TaskPath,
        [Parameter(Mandatory)]
        [ValidateNotNullOrEmpty()]
        [string]$TaskName,
        [Parameter(Mandatory)]
        [ValidateSet('enabled', 'disabled')]
        [string]$State
    )
    if ($State -eq 'disabled') {
        $null = Disable-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction Stop
    }
    else {
        $null = Enable-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction Stop
    }
}
