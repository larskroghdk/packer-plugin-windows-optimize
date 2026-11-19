#Requires -Version 5.1

<#
.SYNOPSIS
    Pester tests for WindowsOptimize.Services.ps1 and WindowsOptimize.ScheduledTasks.ps1.

.DESCRIPTION
    Reads service start types from a fake SYSTEM\CurrentControlSet\Services tree under a
    temporary HKCU key used as the root, and checks the sc.exe and ScheduledTasks calls with
    mocks. No service or task on the machine is changed.

.EXAMPLE
    Invoke-Pester -Path .\test\pester\Services.Tests.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    foreach ($part in 'NativeCommand', 'Registry', 'Services', 'ScheduledTasks') {
        . (Join-Path -Path $PSScriptRoot -ChildPath "..\..\guest\scripts\WindowsOptimize.$part.ps1")
    }
}

Describe 'Get-WOServiceStartType' {
    BeforeAll {
        $script:hkcu = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryView]::Registry64)
        $script:path = 'Software\WindowsOptimizePester-' + [guid]::NewGuid().ToString('N')
        $script:root = $hkcu.CreateSubKey($path)
        $services = @{
            Auto    = @{ Start = 2 }
            Delayed = @{ Start = 2; DelayedAutostart = 1 }
            Manual  = @{ Start = 3; DelayedAutostart = 1 }
            Off     = @{ Start = 4 }
            Boot    = @{ Start = 0 }
            Odd     = @{ Start = 9 }
        }
        foreach ($name in $services.Keys) {
            $key = $root.CreateSubKey("SYSTEM\CurrentControlSet\Services\$name")
            foreach ($value in $services[$name].GetEnumerator()) {
                $key.SetValue($value.Key, $value.Value, [Microsoft.Win32.RegistryValueKind]::DWord)
            }
            $key.Dispose()
        }
    }
    AfterAll {
        $root.Dispose()
        $hkcu.DeleteSubKeyTree($path, $false)
        $hkcu.Dispose()
    }
    It 'maps <name> to <expected>' -TestCases @(
        @{ name = 'Auto'; expected = 'automatic' }
        @{ name = 'Delayed'; expected = 'automatic_delayed' }
        @{ name = 'Manual'; expected = 'manual' }
        @{ name = 'Off'; expected = 'disabled' }
        @{ name = 'Boot'; expected = 'boot' }
        @{ name = 'Odd'; expected = 'unknown(9)' }
    ) {
        $state = Get-WOServiceStartType -Root $root -Name $name
        $state.present | Should -BeTrue
        $state.data | Should -BeExactly $expected
    }
    It 'reports a service that is not installed as absent' {
        (Get-WOServiceStartType -Root $root -Name 'NotInstalled').present | Should -BeFalse
    }
    It 'rejects names that could escape the Services key' {
        { Get-WOServiceStartType -Root $root -Name '..\Auto' } | Should -Throw
    }
}

Describe 'Set-WOServiceStartType' {
    It 'passes start= <scStart> to sc.exe for <startType>' -TestCases @(
        @{ startType = 'automatic'; scStart = 'auto' }
        @{ startType = 'automatic_delayed'; scStart = 'delayed-auto' }
        @{ startType = 'manual'; scStart = 'demand' }
        @{ startType = 'disabled'; scStart = 'disabled' }
    ) {
        Mock Invoke-WONativeCommand { [PSCustomObject]@{ ExitCode = 0; Output = '[SC] ChangeServiceConfig SUCCESS' } }
        Set-WOServiceStartType -Name 'Example' -StartType $startType
        Should -Invoke Invoke-WONativeCommand -Times 1 -Exactly -ParameterFilter {
            $FilePath -like '*\System32\sc.exe' -and ($ArgumentList -join ' ') -eq "config Example start= $scStart"
        }
    }
    It 'throws with the sc.exe output when it fails' {
        Mock Invoke-WONativeCommand { [PSCustomObject]@{ ExitCode = 1060; Output = 'The specified service does not exist' } }
        { Set-WOServiceStartType -Name 'Missing' -StartType 'disabled' } | Should -Throw '*1060*does not exist*'
    }
    It 'refuses start types it cannot restore' {
        { Set-WOServiceStartType -Name 'Example' -StartType 'boot' } | Should -Throw
    }
}

Describe 'Scheduled task functions' {
    It 'reports a disabled task as disabled' {
        Mock Get-ScheduledTask { [PSCustomObject]@{ State = 'Disabled' } }
        (Get-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask').data | Should -Be 'disabled'
    }
    It 'reports a ready or running task as enabled' {
        Mock Get-ScheduledTask { [PSCustomObject]@{ State = 'Running' } }
        $state = Get-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask'
        $state.present | Should -BeTrue
        $state.data | Should -Be 'enabled'
    }
    It 'reports a missing task as absent' {
        Mock Get-ScheduledTask { $null }
        (Get-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Nothing\' -TaskName 'Nothing').present | Should -BeFalse
    }
    It 'addresses the task by folder and name' {
        Mock Disable-ScheduledTask { }
        Mock Enable-ScheduledTask { }
        Set-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask' -State 'disabled'
        Set-WOScheduledTaskState -TaskPath '\Microsoft\Windows\Maps\' -TaskName 'MapsToastTask' -State 'enabled'
        Should -Invoke Disable-ScheduledTask -Times 1 -Exactly -ParameterFilter { $TaskPath -eq '\Microsoft\Windows\Maps\' -and $TaskName -eq 'MapsToastTask' }
        Should -Invoke Enable-ScheduledTask -Times 1 -Exactly -ParameterFilter { $TaskPath -eq '\Microsoft\Windows\Maps\' -and $TaskName -eq 'MapsToastTask' }
    }
    It 'rejects a folder without surrounding backslashes' {
        { Get-WOScheduledTaskState -TaskPath 'Microsoft\Windows\Maps' -TaskName 'MapsToastTask' } | Should -Throw
    }
}
