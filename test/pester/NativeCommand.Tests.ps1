#Requires -Version 5.1

<#
.SYNOPSIS
    Pester tests for WindowsOptimize.NativeCommand.ps1.

.DESCRIPTION
    Checks that the exit code and the merged output survive a program that writes to
    stderr, which Windows PowerShell 5.1 otherwise turns into a terminating error.

.EXAMPLE
    Invoke-Pester -Path .\test\pester\NativeCommand.Tests.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    . (Join-Path -Path $PSScriptRoot -ChildPath '..\..\guest\scripts\WindowsOptimize.NativeCommand.ps1')
    $script:cmd = Join-Path -Path $env:SystemRoot -ChildPath 'System32\cmd.exe'
}

Describe 'Invoke-WONativeCommand' {
    It 'returns the exit code and merged output when the program writes to stderr' {
        $ErrorActionPreference = 'Stop'
        $result = Invoke-WONativeCommand -FilePath $cmd -ArgumentList @('/c', 'echo out & echo err 1>&2 & exit 3')
        $result.ExitCode | Should -Be 3
        $result.Output | Should -Match 'out'
        $result.Output | Should -Match 'err'
    }
    It 'returns exit code 0 on success' {
        (Invoke-WONativeCommand -FilePath $cmd -ArgumentList @('/c', 'exit 0')).ExitCode | Should -Be 0
    }
}
