#Requires -Version 5.1

<#
.SYNOPSIS
    Runs external programs without Windows PowerShell 5.1 stderr pitfalls.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1 before the other parts. Used to call sc.exe
    for delayed-start service configuration and powercfg.exe for hibernation.

.EXAMPLE
    . .\WindowsOptimize.NativeCommand.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Invoke-WONativeCommand {
    <#
    .SYNOPSIS
        Runs an external program and captures its exit code and combined output.

    .DESCRIPTION
        Sets ErrorActionPreference to Continue for the call. Without this, Windows
        PowerShell 5.1 turns the first line the program writes to stderr into a terminating
        error, and the exit code is lost. stdout and stderr are merged into one string with
        blank lines removed. The caller decides which exit codes count as failures.

    .PARAMETER FilePath
        Full path of the program.

    .PARAMETER ArgumentList
        Arguments passed to the program, one per element.

    .EXAMPLE
        Invoke-WONativeCommand -FilePath "$env:SystemRoot\System32\powercfg.exe" -ArgumentList @('/hibernate', 'off')

    .OUTPUTS
        System.Management.Automation.PSCustomObject with ExitCode and Output.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$FilePath,
        [Parameter(Mandatory)]
        [string[]]$ArgumentList
    )
    $ErrorActionPreference = 'Continue'
    $lines = & $FilePath @ArgumentList 2>&1 | ForEach-Object {
        if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.Exception.Message } else { [string]$_ }
    }
    [PSCustomObject]@{
        ExitCode = $LASTEXITCODE
        Output   = (@($lines) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }) -join ' '
    }
}
