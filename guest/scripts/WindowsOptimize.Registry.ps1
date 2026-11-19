#Requires -Version 5.1

<#
.SYNOPSIS
    Reads, writes and removes values in the 64-bit HKLM registry view.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1. All registry access goes through an
    explicit RegistryKey root, so a 32-bit process or WOW64 redirection can never change
    which view is touched.

.EXAMPLE
    . .\WindowsOptimize.Registry.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Get-WORegistryRoot {
    <#
    .SYNOPSIS
        Opens the 64-bit view of HKEY_LOCAL_MACHINE.

    .DESCRIPTION
        Returns HKLM opened with RegistryView.Registry64, so reads and writes never land in
        the WOW6432Node redirected view. The caller must dispose the key.

    .EXAMPLE
        $root = Get-WORegistryRoot
        try { Get-WORegistryValue -Root $root -Path 'SOFTWARE\Policies' -Name 'Example' } finally { $root.Dispose() }

    .OUTPUTS
        Microsoft.Win32.RegistryKey

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([Microsoft.Win32.RegistryKey])]
    param()
    [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::LocalMachine, [Microsoft.Win32.RegistryView]::Registry64)
}

function ConvertTo-WOUInt32 {
    <#
    .SYNOPSIS
        Reinterprets a signed 32-bit registry DWORD as unsigned.

    .DESCRIPTION
        RegistryKey.GetValue returns REG_DWORD data as Int32, so values above 2147483647 come back negative.
        This keeps the bit pattern and returns the unsigned value that the plugin uses.

    .PARAMETER Value
        The Int32 value read from the registry.

    .EXAMPLE
        ConvertTo-WOUInt32 -Value -1

    .OUTPUTS
        System.UInt32

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([uint32])]
    param(
        [Parameter(Mandatory)]
        [int32]$Value
    )
    [BitConverter]::ToUInt32([BitConverter]::GetBytes($Value), 0)
}

function ConvertTo-WOInt32 {
    <#
    .SYNOPSIS
        Reinterprets an unsigned DWORD as the signed value .NET writes to the registry.

    .DESCRIPTION
        The reverse of ConvertTo-WOUInt32. RegistryKey.SetValue takes DWORD data as Int32,
        so catalog values above 2147483647 have to be passed as their signed equivalent.

    .PARAMETER Value
        The unsigned DWORD value from the catalog.

    .EXAMPLE
        ConvertTo-WOInt32 -Value 4294967295

    .OUTPUTS
        System.Int32

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([int32])]
    param(
        [Parameter(Mandatory)]
        [uint32]$Value
    )
    [BitConverter]::ToInt32([BitConverter]::GetBytes($Value), 0)
}

function Get-WORegistryValue {
    <#
    .SYNOPSIS
        Reads one registry value below a root key.

    .DESCRIPTION
        Returns present = $false when the key or the value does not exist. Otherwise returns
        the registry type name (REG_DWORD, REG_SZ, ...) and the data as a string. DWORDs are
        returned as unsigned decimal, and environment variables are not expanded.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Path
        The subkey path below the root, without a hive prefix.

    .PARAMETER Name
        The value name. An empty string reads the key's default value.

    .EXAMPLE
        Get-WORegistryValue -Root $root -Path 'SYSTEM\CurrentControlSet\Control\FileSystem' -Name 'NtfsDisable8dot3NameCreation'

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
        [ValidateNotNullOrEmpty()]
        [string]$Path,
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Name
    )
    $absent = [PSCustomObject]@{ present = $false; type = $null; data = $null }
    $key = $Root.OpenSubKey($Path, $false)
    if ($null -eq $key) {
        return $absent
    }
    try {
        if ($key.GetValueNames() -notcontains $Name) {
            return $absent
        }
        $kind = $key.GetValueKind($Name)
        $raw = $key.GetValue($Name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        switch ($kind) {
            ([Microsoft.Win32.RegistryValueKind]::DWord) {
                $type = 'REG_DWORD'
                $data = [string](ConvertTo-WOUInt32 -Value $raw)
            }
            ([Microsoft.Win32.RegistryValueKind]::String) {
                $type = 'REG_SZ'
                $data = [string]$raw
            }
            ([Microsoft.Win32.RegistryValueKind]::ExpandString) {
                $type = 'REG_EXPAND_SZ'
                $data = [string]$raw
            }
            default {
                $type = "REG_$($kind.ToString().ToUpperInvariant())"
                $data = ($raw | ForEach-Object { [string]$_ }) -join ','
            }
        }
        [PSCustomObject]@{ present = $true; type = $type; data = $data }
    }
    finally {
        $key.Dispose()
    }
}

function Set-WORegistryValue {
    <#
    .SYNOPSIS
        Writes a REG_DWORD or REG_SZ value, creating the key if needed.

    .DESCRIPTION
        REG_DWORD data must be an unsigned decimal string. Anything else, including a negative
        number, throws before the registry is touched.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Path
        The subkey path below the root, without a hive prefix.

    .PARAMETER Name
        The value name.

    .PARAMETER Type
        REG_DWORD or REG_SZ.

    .PARAMETER Data
        The value data as a string.

    .EXAMPLE
        Set-WORegistryValue -Root $root -Path 'SYSTEM\CurrentControlSet\Control\FileSystem' -Name 'NtfsDisable8dot3NameCreation' -Type REG_DWORD -Data '1'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [ValidateNotNullOrEmpty()]
        [string]$Path,
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Name,
        [Parameter(Mandatory)]
        [ValidateSet('REG_DWORD', 'REG_SZ')]
        [string]$Type,
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Data
    )
    $key = $Root.CreateSubKey($Path, $true)
    if ($null -eq $key) {
        throw "Unable to open or create registry key HKLM\$Path"
    }
    try {
        if ($Type -eq 'REG_DWORD') {
            $number = [uint32]::Parse($Data, [Globalization.CultureInfo]::InvariantCulture)
            $key.SetValue($Name, (ConvertTo-WOInt32 -Value $number), [Microsoft.Win32.RegistryValueKind]::DWord)
        }
        else {
            $key.SetValue($Name, $Data, [Microsoft.Win32.RegistryValueKind]::String)
        }
    }
    finally {
        $key.Dispose()
    }
}

function Remove-WORegistryValue {
    <#
    .SYNOPSIS
        Deletes a registry value if it exists.

    .DESCRIPTION
        Does nothing when the key or the value is already absent. The key itself is left in
        place.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .PARAMETER Path
        The subkey path below the root, without a hive prefix.

    .PARAMETER Name
        The value name to delete.

    .EXAMPLE
        Remove-WORegistryValue -Root $root -Path 'SOFTWARE\Policies\Microsoft\Windows\DataCollection' -Name 'AllowTelemetry'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root,
        [Parameter(Mandatory)]
        [ValidateNotNullOrEmpty()]
        [string]$Path,
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Name
    )
    $key = $Root.OpenSubKey($Path, $true)
    if ($null -eq $key) {
        return
    }
    try {
        $key.DeleteValue($Name, $false)
    }
    finally {
        $key.Dispose()
    }
}
