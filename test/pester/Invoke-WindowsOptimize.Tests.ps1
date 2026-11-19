#Requires -Version 5.1

<#
.SYNOPSIS
    Pester tests for the Invoke-WindowsOptimize.ps1 entry point.

.DESCRIPTION
    Runs the read-only actions of the entry script in a child process, as the plugin does,
    and checks the JSON contract, the exit codes and the UTF-8 encoding without a byte
    order mark. Apply and WriteJournal change the machine and are covered by the Go
    integration tests against a fake guest and by the optional live tests instead.

.EXAMPLE
    Invoke-Pester -Path .\test\pester\Invoke-WindowsOptimize.Tests.ps1

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


Describe 'Invoke-WindowsOptimize.ps1' {
    BeforeAll {
        $script:entry = Join-Path -Path $PSScriptRoot -ChildPath '..\..\guest\scripts\Invoke-WindowsOptimize.ps1'
        $script:shell = (Get-Process -Id $PID).Path
        function Invoke-Entry {
            <#
            .SYNOPSIS
                Runs Invoke-WindowsOptimize.ps1 in a child process, as the plugin does.

            .DESCRIPTION
                Writes the input as JSON to the test drive and runs the entry script with -File
                in the same PowerShell edition as the test. Returns the exit code, the stderr
                text and the raw bytes of the output file.

            .PARAMETER Action
                The guest action to run.

            .PARAMETER InputJson
                The request as JSON text. When omitted, no -InputPath is passed.

            .EXAMPLE
                Invoke-Entry -Action 'Detect'

            .OUTPUTS
                System.Management.Automation.PSCustomObject with ExitCode, StdErr and Bytes.

            .NOTES
                Author  : Lars Krogh
                Version : 1.0.0
            #>
            param (
                [string]$Action,
                [string]$InputJson
            )
            $outputPath = Join-Path -Path $TestDrive -ChildPath "$Action-output.json"
            $errorPath = Join-Path -Path $TestDrive -ChildPath "$Action-stderr.txt"
            Remove-Item -LiteralPath $outputPath -ErrorAction SilentlyContinue
            $arguments = @('-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', $entry, '-Action', $Action, '-OutputPath', $outputPath)
            if ($PSBoundParameters.ContainsKey('InputJson')) {
                $inputPath = Join-Path -Path $TestDrive -ChildPath "$Action-input.json"
                [IO.File]::WriteAllText($inputPath, $InputJson, (New-Object System.Text.UTF8Encoding ($false)))
                $arguments += @('-InputPath', $inputPath)
            }
            $process = Start-Process -FilePath $shell -ArgumentList $arguments -Wait -PassThru -NoNewWindow -RedirectStandardError $errorPath -RedirectStandardOutput (Join-Path -Path $TestDrive -ChildPath "$Action-stdout.txt")
            [PSCustomObject]@{
                ExitCode = $process.ExitCode
                StdErr   = [IO.File]::ReadAllText($errorPath)
                Bytes    = if (Test-Path -LiteralPath $outputPath) { [IO.File]::ReadAllBytes($outputPath) } else { $null }
            }
        }
        function ConvertFrom-EntryOutput {
            <#
            .SYNOPSIS
                Decodes the output bytes of an action.

            .DESCRIPTION
                Fails the test when the bytes start with a UTF-8 byte order mark, which the Go
                decoder does not expect, and returns the parsed JSON otherwise.

            .PARAMETER Bytes
                The raw output file contents.

            .EXAMPLE
                ConvertFrom-EntryOutput -Bytes $result.Bytes

            .OUTPUTS
                System.Management.Automation.PSCustomObject

            .NOTES
                Author  : Lars Krogh
                Version : 1.0.0
            #>
            param (
                [byte[]]$Bytes
            )
            $Bytes | Should -Not -BeNullOrEmpty
            ($Bytes.Length -ge 3 -and $Bytes[0] -eq 0xEF -and $Bytes[1] -eq 0xBB -and $Bytes[2] -eq 0xBF) | Should -BeFalse
            [Text.Encoding]::UTF8.GetString($Bytes) | ConvertFrom-Json
        }
    }

    It 'Detect returns the facts the plugin decodes' {
        $result = Invoke-Entry -Action 'Detect'
        $result.ExitCode | Should -Be 0
        $facts = ConvertFrom-EntryOutput -Bytes $result.Bytes
        $facts.os.major | Should -Be 10
        $facts.os.build | Should -BeGreaterThan 14000
        $facts.os.product_type | Should -BeIn 1, 2, 3
        $facts.os.is_64bit_process | Should -BeTrue
        $facts.session.user_name | Should -Not -BeNullOrEmpty
        $facts.power.active_scheme | Should -Match '^[0-9a-f-]{36}$'
        @($facts.power.schemes) | Should -Contain $facts.power.active_scheme
        $facts.PSObject.Properties.Name | Should -Contain 'winrm'
        , $facts.errors | Should -BeOfType [object[]]
    }
    It 'Probe returns a boot time' {
        $result = Invoke-Entry -Action 'Probe'
        $result.ExitCode | Should -Be 0
        $probe = ConvertFrom-EntryOutput -Bytes $result.Bytes
        [datetime]::Parse($probe.boot_time) | Should -BeLessThan (Get-Date).AddMinutes(1)
    }
    It 'Inspect returns one item per request in order, with per-item errors' {
        $request = @'
[{"id":"a","target":{"kind":"service","path":"","name":"PlugPlay","scheme":"","subgroup":"","setting":"","power_source":""}},
 {"id":"b","target":{"kind":"service","path":"","name":"NoSuchServicePester","scheme":"","subgroup":"","setting":"","power_source":""}},
 {"id":"c","target":{"kind":"unknown","path":"","name":"","scheme":"","subgroup":"","setting":"","power_source":""}},
 {"id":"d","target":{"kind":"power_scheme","path":"","name":"","scheme":"","subgroup":"","setting":"","power_source":""}}]
'@
        $result = Invoke-Entry -Action 'Inspect' -InputJson $request
        $result.ExitCode | Should -Be 0
        $items = @((ConvertFrom-EntryOutput -Bytes $result.Bytes).items)
        $items.id | Should -Be @('a', 'b', 'c', 'd')
        $items[0].present | Should -BeTrue
        $items[0].data | Should -BeIn 'automatic', 'manual'
        $items[1].present | Should -BeFalse
        $items[1].error | Should -BeNullOrEmpty
        $items[2].error | Should -Match 'Unsupported target kind'
        $items[3].data | Should -Match '^[0-9a-f-]{36}$'
    }
    It 'Inspect keeps a single item as a JSON array' {
        $request = '[{"id":"only","target":{"kind":"hibernation","path":"","name":"","scheme":"","subgroup":"","setting":"","power_source":""}}]'
        $result = Invoke-Entry -Action 'Inspect' -InputJson $request
        [Text.Encoding]::UTF8.GetString($result.Bytes) | Should -Match '^\{"items":\['
    }
    It 'ReadJournal returns the journal path under ProgramData' {
        $result = Invoke-Entry -Action 'ReadJournal'
        $result.ExitCode | Should -Be 0
        $journal = ConvertFrom-EntryOutput -Bytes $result.Bytes
        $journal.path | Should -Be (Join-Path -Path $env:ProgramData -ChildPath 'packer-plugin-windows-optimize\journal.json')
        $journal.PSObject.Properties.Name | Should -Contain 'content'
    }
    It 'exits 1 with the error on stderr when a required input is missing' {
        $result = Invoke-Entry -Action 'Inspect'
        $result.ExitCode | Should -Be 1
        $result.StdErr | Should -Match 'requires -InputPath'
        $result.Bytes | Should -BeNullOrEmpty
    }
}
