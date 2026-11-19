#Requires -Version 5.1

<#
.SYNOPSIS
    Pester tests for WindowsOptimize.Registry.ps1.

.DESCRIPTION
    Covers DWORD sign conversion and the read, write and remove round trips against a
    temporary key under HKCU, which is deleted afterwards. Nothing under HKLM is touched.

.EXAMPLE
    Invoke-Pester -Path .\test\pester\Registry.Tests.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


BeforeAll {
    . (Join-Path -Path $PSScriptRoot -ChildPath '..\..\guest\scripts\WindowsOptimize.Registry.ps1')
}

Describe 'ConvertTo-WOUInt32 and ConvertTo-WOInt32' {
    It 'maps -1 and 4294967295 onto each other' {
        ConvertTo-WOUInt32 -Value -1 | Should -Be 4294967295
        ConvertTo-WOInt32 -Value 4294967295 | Should -Be -1
    }
    It 'keeps small values unchanged' {
        ConvertTo-WOUInt32 -Value 5 | Should -Be 5
        ConvertTo-WOInt32 -Value 5 | Should -Be 5
    }
}

Describe 'Registry value functions' {
    BeforeAll {
        $script:root = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser, [Microsoft.Win32.RegistryView]::Registry64)
        $script:path = 'Software\WindowsOptimizePester-' + [guid]::NewGuid().ToString('N')
    }
    AfterAll {
        $root.DeleteSubKeyTree($path, $false)
        $root.Dispose()
    }
    It 'reports a missing key as absent' {
        $state = Get-WORegistryValue -Root $root -Path "$path\Missing" -Name 'Value'
        $state.present | Should -BeFalse
        $state.type | Should -BeNullOrEmpty
        $state.data | Should -BeNullOrEmpty
    }
    It 'round-trips a REG_DWORD above 2^31 as unsigned decimal' {
        Set-WORegistryValue -Root $root -Path $path -Name 'Dword' -Type 'REG_DWORD' -Data '4294967295'
        $state = Get-WORegistryValue -Root $root -Path $path -Name 'Dword'
        $state.present | Should -BeTrue
        $state.type | Should -Be 'REG_DWORD'
        $state.data | Should -BeExactly '4294967295'
    }
    It 'round-trips a REG_SZ' {
        Set-WORegistryValue -Root $root -Path $path -Name 'String' -Type 'REG_SZ' -Data 'Packer'
        $state = Get-WORegistryValue -Root $root -Path $path -Name 'String'
        $state.type | Should -Be 'REG_SZ'
        $state.data | Should -BeExactly 'Packer'
    }
    It 'reports other value types by name without expanding them' {
        $key = $root.CreateSubKey($path)
        $key.SetValue('Expand', '%SystemRoot%', [Microsoft.Win32.RegistryValueKind]::ExpandString)
        $key.SetValue('Multi', [string[]]@('a', 'b'), [Microsoft.Win32.RegistryValueKind]::MultiString)
        $key.Dispose()
        $expand = Get-WORegistryValue -Root $root -Path $path -Name 'Expand'
        $expand.type | Should -Be 'REG_EXPAND_SZ'
        $expand.data | Should -BeExactly '%SystemRoot%'
        (Get-WORegistryValue -Root $root -Path $path -Name 'Multi').type | Should -Be 'REG_MULTISTRING'
    }
    It 'reports a missing value in an existing key as absent' {
        (Get-WORegistryValue -Root $root -Path $path -Name 'Nothing').present | Should -BeFalse
    }
    It 'removes a value and tolerates removing it again' {
        Set-WORegistryValue -Root $root -Path $path -Name 'Gone' -Type 'REG_DWORD' -Data '1'
        Remove-WORegistryValue -Root $root -Path $path -Name 'Gone'
        { Remove-WORegistryValue -Root $root -Path $path -Name 'Gone' } | Should -Not -Throw
        { Remove-WORegistryValue -Root $root -Path "$path\NoKey" -Name 'Gone' } | Should -Not -Throw
        (Get-WORegistryValue -Root $root -Path $path -Name 'Gone').present | Should -BeFalse
    }
    It 'rejects a REG_DWORD that is not a decimal uint32' {
        { Set-WORegistryValue -Root $root -Path $path -Name 'Bad' -Type 'REG_DWORD' -Data '-1' } | Should -Throw
        { Set-WORegistryValue -Root $root -Path $path -Name 'Bad' -Type 'REG_DWORD' -Data '4294967296' } | Should -Throw
        (Get-WORegistryValue -Root $root -Path $path -Name 'Bad').present | Should -BeFalse
    }
    It 'rejects registry types it cannot restore' {
        { Set-WORegistryValue -Root $root -Path $path -Name 'Binary' -Type 'REG_BINARY' -Data '00' } | Should -Throw
    }
}
