#Requires -Version 5.1

<#
.SYNOPSIS
    Collects the guest facts that the plugin uses to select optimizations and protect WinRM.

.DESCRIPTION
    Dot-sourced by Invoke-WindowsOptimize.ps1. Reads the operating system, the session, the
    WinRM configuration with the services WinRM depends on, and the power schemes.

.EXAMPLE
    . .\WindowsOptimize.Facts.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


function Get-WOOperatingSystem {
    <#
    .SYNOPSIS
        Collects the Windows version, edition and installation type.

    .DESCRIPTION
        Combines the CurrentVersion registry key with Win32_OperatingSystem. Values missing
        from the registry are returned as null or 0, so the plugin can reject an unknown
        release instead of guessing.

    .EXAMPLE
        (Get-WOOperatingSystem).build

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $currentVersion = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
    $optional = {
        param($name)
        $property = $currentVersion.PSObject.Properties[$name]
        if ($null -ne $property) { $property.Value } else { $null }
    }
    [PSCustomObject]@{
        product_name      = & $optional 'ProductName'
        edition_id        = & $optional 'EditionID'
        installation_type = & $optional 'InstallationType'
        display_version   = & $optional 'DisplayVersion'
        major             = [int](& $optional 'CurrentMajorVersionNumber')
        minor             = [int](& $optional 'CurrentMinorVersionNumber')
        build             = [int](& $optional 'CurrentBuildNumber')
        ubr               = [int](& $optional 'UBR')
        product_type      = [int]$os.ProductType
        architecture      = [string]$env:PROCESSOR_ARCHITECTURE
        is_64bit_process  = [Environment]::Is64BitProcess
    }
}

function Get-WOSessionIdentity {
    <#
    .SYNOPSIS
        Describes the account and elevation of the current WinRM session.

    .DESCRIPTION
        Returns the user name and whether the token holds the Administrators group. The
        plugin refuses to run without elevation and checks after every change that the
        session is still elevated.

    .EXAMPLE
        (Get-WOSessionIdentity).is_elevated

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    try {
        $principal = New-Object System.Security.Principal.WindowsPrincipal ($identity)
        [PSCustomObject]@{
            user_name   = $identity.Name
            is_elevated = $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)
        }
    }
    finally {
        $identity.Dispose()
    }
}

function ConvertTo-WOBoolean {
    <#
    .SYNOPSIS
        Converts a WSMan provider value to a Boolean.

    .DESCRIPTION
        The WSMan: drive reports settings as the strings 'true' and 'false'. Only 'true'
        (case-insensitive) converts to $true. Anything else, including an empty string,
        converts to $false.

    .PARAMETER Value
        The string value read from the WSMan: drive.

    .EXAMPLE
        ConvertTo-WOBoolean -Value (Get-Item WSMan:\localhost\Service\AllowUnencrypted).Value

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([bool])]
    param(
        [Parameter()]
        [AllowEmptyString()]
        [AllowNull()]
        [string]$Value
    )
    $Value -eq 'true'
}

function Get-WOServiceDependency {
    <#
    .SYNOPSIS
        Lists every service and driver a service depends on, directly or indirectly.

    .DESCRIPTION
        Walks ServicesDependedOn breadth-first and returns each service name once, sorted.
        The plugin never changes the start type of a service in this list.

    .PARAMETER Name
        The service whose dependencies are listed.

    .EXAMPLE
        Get-WOServiceDependency -Name WinRM

    .OUTPUTS
        System.String[]

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    [OutputType([string[]])]
    param(
        [Parameter(Mandatory)]
        [string]$Name
    )
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $queue = New-Object System.Collections.Generic.Queue[object]
    $queue.Enqueue((Get-Service -Name $Name -ErrorAction Stop))
    while ($queue.Count -gt 0) {
        $service = $queue.Dequeue()
        foreach ($dependency in $service.ServicesDependedOn) {
            if ($seen.Add($dependency.ServiceName)) {
                $queue.Enqueue($dependency)
            }
        }
    }
    $seen | Sort-Object
}

function Get-WOWinRMConfiguration {
    <#
    .SYNOPSIS
        Reads the WinRM service, listener and authentication configuration.

    .DESCRIPTION
        Returns the service state, the services WinRM depends on, every listener, whether
        unencrypted traffic is allowed and which authentication methods are enabled. The
        plugin compares this before and after changes to confirm that WinRM did not regress.

    .EXAMPLE
        (Get-WOWinRMConfiguration).auth.basic

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $service = Get-Service -Name WinRM -ErrorAction Stop
    $listeners = foreach ($listener in (Get-ChildItem -Path 'WSMan:\localhost\Listener' -ErrorAction Stop)) {
        $values = @{}
        foreach ($item in (Get-ChildItem -Path "WSMan:\localhost\Listener\$($listener.Name)" -ErrorAction Stop)) {
            $values[$item.Name] = [string]$item.Value
        }
        [PSCustomObject]@{
            transport = $values['Transport']
            address   = $values['Address']
            port      = [int]$values['Port']
            enabled   = ConvertTo-WOBoolean -Value $values['Enabled']
        }
    }
    $auth = @{}
    foreach ($item in (Get-ChildItem -Path 'WSMan:\localhost\Service\Auth' -ErrorAction Stop)) {
        $auth[$item.Name] = [string]$item.Value
    }
    [PSCustomObject]@{
        service_status    = $service.Status.ToString()
        service_start     = $service.StartType.ToString()
        dependencies      = @(Get-WOServiceDependency -Name WinRM)
        listeners         = @($listeners)
        allow_unencrypted = ConvertTo-WOBoolean -Value ([string](Get-Item -Path 'WSMan:\localhost\Service\AllowUnencrypted').Value)
        auth              = [PSCustomObject]@{
            basic       = ConvertTo-WOBoolean -Value $auth['Basic']
            kerberos    = ConvertTo-WOBoolean -Value $auth['Kerberos']
            negotiate   = ConvertTo-WOBoolean -Value $auth['Negotiate']
            certificate = ConvertTo-WOBoolean -Value $auth['Certificate']
            credssp     = ConvertTo-WOBoolean -Value $auth['CredSSP']
        }
    }
}

function Get-WOPowerConfiguration {
    <#
    .SYNOPSIS
        Reads the active power scheme and the installed schemes.

    .DESCRIPTION
        The plugin uses the result to decide which scheme power settings are written to and
        whether the High performance scheme can be activated.

    .EXAMPLE
        (Get-WOPowerConfiguration).active_scheme

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    [PSCustomObject]@{
        active_scheme = Get-WOActivePowerScheme
        schemes       = @(Get-WOPowerSchemeList)
    }
}

function Get-WOSystemFact {
    <#
    .SYNOPSIS
        Collects all guest facts for the Detect action.

    .DESCRIPTION
        Gathers the operating system, the session, WinRM and power facts. When WinRM or power
        detection fails, that part is null and the error is added to the errors list, so the
        plugin treats it as unknown and fails closed. Operating system and session failures
        stop the action.

    .EXAMPLE
        Write-WOResult -Path $OutputPath -Result (Get-WOSystemFact)

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    $errors = New-Object System.Collections.Generic.List[string]
    $collect = {
        param($name, [scriptblock]$block)
        try {
            & $block
        }
        catch {
            $errors.Add("${name}: $($_.Exception.Message)")
            $null
        }
    }
    [PSCustomObject]@{
        os      = Get-WOOperatingSystem
        session = Get-WOSessionIdentity
        winrm   = & $collect 'winrm' { Get-WOWinRMConfiguration }
        power   = & $collect 'power' { Get-WOPowerConfiguration }
        errors  = @($errors)
    }
}
