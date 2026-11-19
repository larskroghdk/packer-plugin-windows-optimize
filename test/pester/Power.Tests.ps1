#Requires -Version 5.1

<#
.SYNOPSIS
    Pester tests for WindowsOptimize.Power.ps1.

.DESCRIPTION
    Reads the real power configuration through the powrprof.dll API without changing it,
    reads hibernation from a fake Control\Power key under a temporary HKCU root, and checks
    the powercfg.exe call with a mock.

.EXAMPLE
    Invoke-Pester -Path .\test\pester\Power.Tests.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    foreach ($part in 'NativeCommand', 'Registry', 'Power') {
        . (Join-Path -Path $PSScriptRoot -ChildPath "..\..\guest\scripts\WindowsOptimize.$part.ps1")
    }
    $script:subgroupSleep = '238c9fa8-0aad-41ed-83f4-97be242c8f20'
    $script:settingStandbyIdle = '29f6c1db-86da-48c5-9fdb-f2b67b1f44da'
}

Describe 'Power scheme functions' {
    It 'returns the active scheme as a lower-case GUID that is installed' {
        $active = Get-WOActivePowerScheme
        $active | Should -Match '^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$'
        @(Get-WOPowerSchemeList) | Should -Contain $active
    }
    It 'reads the standby timeout of the active scheme on <source> power' -TestCases @(
        @{ source = 'ac' }
        @{ source = 'dc' }
    ) {
        $state = Get-WOPowerSettingIndex -Scheme (Get-WOActivePowerScheme) -Subgroup $subgroupSleep -Setting $settingStandbyIdle -PowerSource $source
        $state.present | Should -BeTrue
        $state.data | Should -Match '^\d+$'
    }
    It 'reports a scheme that does not exist as absent' {
        $state = Get-WOPowerSettingIndex -Scheme ([guid]::NewGuid()) -Subgroup $subgroupSleep -Setting $settingStandbyIdle -PowerSource 'ac'
        $state.present | Should -BeFalse
    }
}

Describe 'Hibernation functions' {
    BeforeAll {
        $script:hkcu = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryView]::Registry64)
        $script:path = 'Software\WindowsOptimizePester-' + [guid]::NewGuid().ToString('N')
        $script:root = $hkcu.CreateSubKey($path)
        $script:power = 'SYSTEM\CurrentControlSet\Control\Power'
    }
    AfterAll {
        $root.Dispose()
        $hkcu.DeleteSubKeyTree($path, $false)
        $hkcu.Dispose()
    }
    It 'reports hibernation as absent when Windows has written neither value' {
        (Get-WOHibernation -Root $root).present | Should -BeFalse
    }
    It 'falls back to HibernateEnabledDefault' {
        Set-WORegistryValue -Root $root -Path $power -Name 'HibernateEnabledDefault' -Type 'REG_DWORD' -Data '1'
        (Get-WOHibernation -Root $root).data | Should -Be 'enabled'
    }
    It 'prefers HibernateEnabled' {
        Set-WORegistryValue -Root $root -Path $power -Name 'HibernateEnabled' -Type 'REG_DWORD' -Data '0'
        $state = Get-WOHibernation -Root $root
        $state.present | Should -BeTrue
        $state.data | Should -Be 'disabled'
    }
    It 'runs powercfg /hibernate <argument> for <state>' -TestCases @(
        @{ state = 'enabled'; argument = 'on' }
        @{ state = 'disabled'; argument = 'off' }
    ) {
        Mock Invoke-WONativeCommand { [PSCustomObject]@{ ExitCode = 0; Output = '' } }
        Set-WOHibernation -State $state
        Should -Invoke Invoke-WONativeCommand -Times 1 -Exactly -ParameterFilter {
            $FilePath -like '*\System32\powercfg.exe' -and ($ArgumentList -join ' ') -eq "/hibernate $argument"
        }
    }
    It 'throws when powercfg fails' {
        Mock Invoke-WONativeCommand { [PSCustomObject]@{ ExitCode = 1; Output = 'The request is not supported.' } }
        { Set-WOHibernation -State 'enabled' } | Should -Throw '*not supported*'
    }
}
