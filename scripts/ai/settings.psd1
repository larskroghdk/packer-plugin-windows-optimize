@{
    Codex               = @{
        Command            = 'codex'
        Model              = ''
        TimeoutSeconds     = 900
        MaxAttempts        = 2
        MaxDiffBytes       = 400000
        BlockingSeverities = @('critical', 'high')
    }
    Gemini              = @{
        Command                   = 'gemini'
        Model                     = ''
        TimeoutSeconds            = 600
        MaxAttempts               = 2
        CacheDays                 = 7
        InputUsdPerMillionTokens  = 0
        OutputUsdPerMillionTokens = 0
    }
    Copilot             = @{
        Command             = 'copilot'
        Model               = ''
        TimeoutSeconds      = 900
        MaxAiCredits        = 0
        WriteRules          = @('*.md', 'docs/**', '.github/**')
        AllowedPathPatterns = @('*.md', 'docs/*', '.github/*')
        DenyTools           = @('shell(git commit)', 'shell(git push)', 'shell(git tag)', 'shell(git reset)', 'shell(git rebase)', 'shell(git checkout)', 'shell(gh:*)', 'write(.git/**)')
    }
    CheckTimeoutSeconds = 1200
    Checks              = @(
        @{ Name = 'gofmt'; Paths = @('*.go'); Command = @('gofmt', '-l', '.'); FailOnOutput = $true }
        @{ Name = 'go mod tidy'; Paths = @('*.go', 'go.mod', 'go.sum'); Command = @('go', 'mod', 'tidy', '-diff') }
        @{ Name = 'go vet'; Paths = @('*.go', 'go.mod', 'go.sum'); Command = @('go', 'vet', './...') }
        @{ Name = 'staticcheck'; Paths = @('*.go', 'go.mod', 'go.sum'); Command = @('sh', '-c', 'GOTOOLCHAIN=$(go env GOVERSION) go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...') }
        @{ Name = 'govulncheck'; Paths = @('*.go', 'go.mod', 'go.sum'); Command = @('go', 'run', 'golang.org/x/vuln/cmd/govulncheck@v1.10.0', './...') }
        @{ Name = 'go test'; Paths = @('*.go', 'go.mod', 'go.sum', 'guest/*', 'internal/*'); Command = @('go', 'test', './...') }
        @{ Name = 'go vet integration'; Paths = @('*.go', 'go.mod', 'go.sum'); Command = @('go', 'vet', '-tags', 'integration', './test/integration') }
        @{ Name = 'doccheck'; Paths = @('*.go'); Command = @('go', 'run', './cmd/doccheck', '.') }
        @{ Name = 'psdoccheck'; Paths = @('*.ps1'); Command = @('sh', './psdoccheck.sh') }
        @{ Name = 'catalog'; Paths = @('*.go', 'internal/*', 'docs/catalog.md'); Command = @('go', 'run', './cmd/catalogdoc', '-check') }
        @{ Name = 'guest PSScriptAnalyzer'; Paths = @('guest/scripts/*', 'test/pester/*', 'PSScriptAnalyzerSettings.psd1'); Command = @('pwsh', '-NoProfile', '-Command', '$ErrorActionPreference = "Stop"; $r = "guest/scripts", "test/pester" | ForEach-Object { Invoke-ScriptAnalyzer -Path $_ -Recurse -Settings ./PSScriptAnalyzerSettings.psd1 }; $r; if ($r) { exit 1 }') }
        @{ Name = 'guest Pester'; Paths = @('guest/scripts/*', 'test/pester/*'); Command = @('powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', '$ErrorActionPreference = "Stop"; $env:PSModulePath = (Join-Path ([Environment]::GetFolderPath("MyDocuments")) "WindowsPowerShell\Modules") + ";" + [Environment]::GetEnvironmentVariable("PSModulePath", "Machine"); Import-Module Pester -MinimumVersion 5.0; $c = New-PesterConfiguration; $c.Run.Path = "test/pester"; $c.Run.Exit = $true; $c.Output.Verbosity = "Normal"; Invoke-Pester -Configuration $c') }
        @{ Name = 'agent scripts PSScriptAnalyzer'; Paths = @('scripts/ai/*', 'test/ai/*'); Command = @('pwsh', '-NoProfile', '-Command', '$ErrorActionPreference = "Stop"; $r = "scripts/ai", "test/ai" | ForEach-Object { Invoke-ScriptAnalyzer -Path $_ -Recurse -Severity Warning, Error }; $r; if ($r) { exit 1 }') }
        @{ Name = 'agent workflow Pester'; Paths = @('scripts/ai/*', 'test/ai/*', '.githooks/*'); Command = @('pwsh', '-NoProfile', '-Command', '$ErrorActionPreference = "Stop"; Import-Module Pester -MinimumVersion 5.6.1; $c = New-PesterConfiguration; $c.Run.Path = "test/ai"; $c.Run.Exit = $true; $c.Output.Verbosity = "Normal"; Invoke-Pester -Configuration $c') }
        @{ Name = 'packer fmt'; Paths = @('*.hcl'); Command = @('sh', '-c', 'packer fmt -check -recursive examples && packer fmt -check -recursive test/integration') }
        @{ Name = 'actionlint'; Paths = @('.github/workflows/*'); Command = @('sh', '-c', 'go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.11 .github/workflows/*.yml') }
    )
}
