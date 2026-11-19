#Requires -Version 5.1

<#
.SYNOPSIS
    Reads and sets power schemes, power setting indexes and hibernation.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1. Power schemes and setting indexes are read
    and written through the documented powrprof.dll API, because powercfg.exe output is
    localized. Hibernation is read from the registry and switched with powercfg.exe
    /hibernate, which also creates or deletes hiberfil.sys.

.EXAMPLE
    . .\WindowsOptimize.Power.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Initialize-WOPowerApi {
    <#
    .SYNOPSIS
        Loads the powrprof.dll declarations once per process.

    .DESCRIPTION
        Compiles a small P/Invoke class for PowerEnumerate, PowerGetActiveScheme,
        PowerSetActiveScheme and the AC and DC value index functions. Compiling takes a
        moment, so the other power functions call this only when they need the API.

    .EXAMPLE
        Initialize-WOPowerApi

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    if ($null -ne ('WindowsOptimize.PowerApi' -as [type])) {
        return
    }
    Add-Type -Namespace 'WindowsOptimize' -Name 'PowerApi' -MemberDefinition @'
[DllImport("powrprof.dll")]
public static extern uint PowerEnumerate(IntPtr RootPowerKey, IntPtr SchemeGuid, IntPtr SubGroupOfPowerSettingsGuid, uint AccessFlags, uint Index, byte[] Buffer, ref uint BufferSize);
[DllImport("powrprof.dll")]
public static extern uint PowerGetActiveScheme(IntPtr UserRootPowerKey, out IntPtr ActivePolicyGuid);
[DllImport("powrprof.dll")]
public static extern uint PowerSetActiveScheme(IntPtr UserRootPowerKey, ref Guid SchemeGuid);
[DllImport("powrprof.dll")]
public static extern uint PowerReadACValueIndex(IntPtr RootPowerKey, ref Guid SchemeGuid, ref Guid SubGroupOfPowerSettingsGuid, ref Guid PowerSettingGuid, out uint AcValueIndex);
[DllImport("powrprof.dll")]
public static extern uint PowerReadDCValueIndex(IntPtr RootPowerKey, ref Guid SchemeGuid, ref Guid SubGroupOfPowerSettingsGuid, ref Guid PowerSettingGuid, out uint DcValueIndex);
[DllImport("powrprof.dll")]
public static extern uint PowerWriteACValueIndex(IntPtr RootPowerKey, ref Guid SchemeGuid, ref Guid SubGroupOfPowerSettingsGuid, ref Guid PowerSettingGuid, uint AcValueIndex);
[DllImport("powrprof.dll")]
public static extern uint PowerWriteDCValueIndex(IntPtr RootPowerKey, ref Guid SchemeGuid, ref Guid SubGroupOfPowerSettingsGuid, ref Guid PowerSettingGuid, uint DcValueIndex);
[DllImport("kernel32.dll")]
public static extern IntPtr LocalFree(IntPtr hMem);
'@
}

function Get-WOPowerSchemeList {
    <#
    .SYNOPSIS
        Lists the GUIDs of the installed power schemes.

    .DESCRIPTION
        Enumerates schemes with PowerEnumerate and ACCESS_SCHEME. GUIDs are returned in lower
        case, the form the plugin compares.

    .EXAMPLE
        Get-WOPowerSchemeList

    .OUTPUTS
        System.String[]

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string[]])]
    param()
    Initialize-WOPowerApi
    $accessScheme = 16
    $errorNoMoreItems = 259
    $index = 0
    while ($true) {
        $buffer = New-Object byte[] 16
        $size = [uint32]16
        $code = [WindowsOptimize.PowerApi]::PowerEnumerate([IntPtr]::Zero, [IntPtr]::Zero, [IntPtr]::Zero, $accessScheme, $index, $buffer, [ref]$size)
        if ($code -eq $errorNoMoreItems) {
            break
        }
        if ($code -ne 0) {
            throw "PowerEnumerate failed with error $code"
        }
        (New-Object Guid (, $buffer)).ToString()
        $index++
    }
}

function Get-WOActivePowerScheme {
    <#
    .SYNOPSIS
        Returns the GUID of the active power scheme.

    .DESCRIPTION
        Calls PowerGetActiveScheme and frees the buffer it allocates. The GUID is returned in
        lower case.

    .EXAMPLE
        Get-WOActivePowerScheme

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param()
    Initialize-WOPowerApi
    $pointer = [IntPtr]::Zero
    $code = [WindowsOptimize.PowerApi]::PowerGetActiveScheme([IntPtr]::Zero, [ref]$pointer)
    if ($code -ne 0) {
        throw "PowerGetActiveScheme failed with error $code"
    }
    try {
        ([Runtime.InteropServices.Marshal]::PtrToStructure($pointer, [type][Guid])).ToString()
    }
    finally {
        $null = [WindowsOptimize.PowerApi]::LocalFree($pointer)
    }
}

function Set-WOActivePowerScheme {
    <#
    .SYNOPSIS
        Activates a power scheme.

    .DESCRIPTION
        Calls PowerSetActiveScheme. Activating the scheme that is already active is also how
        changed value indexes of that scheme take effect.

    .PARAMETER Scheme
        The scheme GUID.

    .EXAMPLE
        Set-WOActivePowerScheme -Scheme '8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [guid]$Scheme
    )
    Initialize-WOPowerApi
    $code = [WindowsOptimize.PowerApi]::PowerSetActiveScheme([IntPtr]::Zero, [ref]$Scheme)
    if ($code -ne 0) {
        throw "PowerSetActiveScheme $Scheme failed with error $code"
    }
}

function Get-WOPowerSettingIndex {
    <#
    .SYNOPSIS
        Reads the AC or DC value index of a power setting in a scheme.

    .DESCRIPTION
        Returns the index as a decimal string. A scheme or setting that does not exist
        (ERROR_FILE_NOT_FOUND) is reported as absent. Any other error throws.

    .PARAMETER Scheme
        The scheme GUID.

    .PARAMETER Subgroup
        The subgroup GUID.

    .PARAMETER Setting
        The setting GUID.

    .PARAMETER PowerSource
        ac or dc.

    .EXAMPLE
        Get-WOPowerSettingIndex -Scheme $active -Subgroup '238c9fa8-0aad-41ed-83f4-97be242c8f20' -Setting '29f6c1db-86da-48c5-9fdb-f2b67b1f44da' -PowerSource ac

    .OUTPUTS
        System.Management.Automation.PSCustomObject with present, type and data.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [guid]$Scheme,
        [Parameter(Mandatory)]
        [guid]$Subgroup,
        [Parameter(Mandatory)]
        [guid]$Setting,
        [Parameter(Mandatory)]
        [ValidateSet('ac', 'dc')]
        [string]$PowerSource
    )
    Initialize-WOPowerApi
    $index = [uint32]0
    if ($PowerSource -eq 'ac') {
        $code = [WindowsOptimize.PowerApi]::PowerReadACValueIndex([IntPtr]::Zero, [ref]$Scheme, [ref]$Subgroup, [ref]$Setting, [ref]$index)
    }
    else {
        $code = [WindowsOptimize.PowerApi]::PowerReadDCValueIndex([IntPtr]::Zero, [ref]$Scheme, [ref]$Subgroup, [ref]$Setting, [ref]$index)
    }
    if ($code -eq 2) {
        return [PSCustomObject]@{ present = $false; type = $null; data = $null }
    }
    if ($code -ne 0) {
        throw "PowerRead$($PowerSource.ToUpperInvariant())ValueIndex failed with error $code"
    }
    [PSCustomObject]@{ present = $true; type = $null; data = [string]$index }
}

function Set-WOPowerSettingIndex {
    <#
    .SYNOPSIS
        Writes the AC or DC value index of a power setting in a scheme.

    .DESCRIPTION
        The new index is stored in the scheme at once. When the scheme is active, it takes
        effect after the scheme is activated again with Set-WOActivePowerScheme.

    .PARAMETER Scheme
        The scheme GUID.

    .PARAMETER Subgroup
        The subgroup GUID.

    .PARAMETER Setting
        The setting GUID.

    .PARAMETER PowerSource
        ac or dc.

    .PARAMETER Index
        The value index, for timeouts the number of seconds, 0 meaning never.

    .EXAMPLE
        Set-WOPowerSettingIndex -Scheme $active -Subgroup '238c9fa8-0aad-41ed-83f4-97be242c8f20' -Setting '29f6c1db-86da-48c5-9fdb-f2b67b1f44da' -PowerSource ac -Index 0

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [guid]$Scheme,
        [Parameter(Mandatory)]
        [guid]$Subgroup,
        [Parameter(Mandatory)]
        [guid]$Setting,
        [Parameter(Mandatory)]
        [ValidateSet('ac', 'dc')]
        [string]$PowerSource,
        [Parameter(Mandatory)]
        [uint32]$Index
    )
    Initialize-WOPowerApi
    if ($PowerSource -eq 'ac') {
        $code = [WindowsOptimize.PowerApi]::PowerWriteACValueIndex([IntPtr]::Zero, [ref]$Scheme, [ref]$Subgroup, [ref]$Setting, $Index)
    }
    else {
        $code = [WindowsOptimize.PowerApi]::PowerWriteDCValueIndex([IntPtr]::Zero, [ref]$Scheme, [ref]$Subgroup, [ref]$Setting, $Index)
    }
    if ($code -ne 0) {
        throw "PowerWrite$($PowerSource.ToUpperInvariant())ValueIndex failed with error $code"
    }
}

function Get-WOHibernation {
    <#
    .SYNOPSIS
        Reads whether hibernation is enabled.

    .DESCRIPTION
        Reads HibernateEnabled under SYSTEM\CurrentControlSet\Control\Power, and
        HibernateEnabledDefault when Windows has not written HibernateEnabled yet. When
        neither value exists the state is reported as absent.

    .PARAMETER Root
        The open root key, normally from Get-WORegistryRoot.

    .EXAMPLE
        Get-WOHibernation -Root $root

    .OUTPUTS
        System.Management.Automation.PSCustomObject with present, type and data.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [Microsoft.Win32.RegistryKey]$Root
    )
    foreach ($name in 'HibernateEnabled', 'HibernateEnabledDefault') {
        $value = Get-WORegistryValue -Root $Root -Path 'SYSTEM\CurrentControlSet\Control\Power' -Name $name
        if ($value.present) {
            $state = if ($value.data -eq '0') { 'disabled' } else { 'enabled' }
            return [PSCustomObject]@{ present = $true; type = $null; data = $state }
        }
    }
    [PSCustomObject]@{ present = $false; type = $null; data = $null }
}

function Set-WOHibernation {
    <#
    .SYNOPSIS
        Turns hibernation on or off.

    .DESCRIPTION
        Runs powercfg.exe /hibernate on or off. Turning it off also deletes hiberfil.sys and
        turns off Fast Startup. Throws when powercfg.exe fails, for example when the
        hypervisor does not expose the sleep states that hibernation needs.

    .PARAMETER State
        enabled or disabled.

    .EXAMPLE
        Set-WOHibernation -State disabled

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [ValidateSet('enabled', 'disabled')]
        [string]$State
    )
    $argument = if ($State -eq 'enabled') { 'on' } else { 'off' }
    $powercfg = Join-Path -Path $env:SystemRoot -ChildPath 'System32\powercfg.exe'
    $result = Invoke-WONativeCommand -FilePath $powercfg -ArgumentList @('/hibernate', $argument)
    if ($result.ExitCode -ne 0) {
        throw "powercfg.exe /hibernate $argument failed with exit code $($result.ExitCode): $($result.Output)"
    }
}
