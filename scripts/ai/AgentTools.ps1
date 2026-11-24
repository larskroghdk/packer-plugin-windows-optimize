#Requires -Version 7.2

<#
.SYNOPSIS
    Shared functions for the agent workflow scripts.

.DESCRIPTION
    Dot-sourced by the scripts in scripts/ai.
    Resolves agent CLIs and runs them with a timeout, identifies a changeset by its base and result Git trees, runs the deterministic checks and the Codex review on an exported snapshot of a changeset, and keeps review records, research cache and usage in the Git directory, where they stay local to the clone.

.EXAMPLE
    . (Join-Path $PSScriptRoot 'AgentTools.ps1')

.NOTES
    Author  : Lars Krogh
    Version : 1.0.0
#>


Set-StrictMode -Version Latest

$script:EmptyTree = '4b825dc642cb6eb9a060e54bf8d69288fbee4904'
$script:GitLocalVariables = @('GIT_ALTERNATE_OBJECT_DIRECTORIES', 'GIT_CONFIG', 'GIT_CONFIG_PARAMETERS', 'GIT_CONFIG_COUNT', 'GIT_OBJECT_DIRECTORY', 'GIT_DIR', 'GIT_WORK_TREE', 'GIT_IMPLICIT_WORK_TREE', 'GIT_GRAFT_FILE', 'GIT_INDEX_FILE', 'GIT_NO_REPLACE_OBJECTS', 'GIT_REPLACE_REF_BASE', 'GIT_PREFIX', 'GIT_SHALLOW_FILE', 'GIT_COMMON_DIR')

function Write-AIMessage {
    <#
    .SYNOPSIS
        Shows a status message to the person running the workflow.

    .DESCRIPTION
        Writes the message to the information stream and displays it, so it reaches the console like Write-AIMessage but never the success pipeline that callers capture.

    .PARAMETER Message
        The message text.

    .EXAMPLE
        Write-AIMessage -Message 'Requesting Codex review'

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Message
    )
    Write-Information -MessageData $Message -InformationAction Continue
}

function Invoke-AIProcess {
    <#
    .SYNOPSIS
        Runs a program with a timeout and captures its output.

    .DESCRIPTION
        Starts the program without a shell, passes each argument separately, writes the input text to standard input and closes it, and reads standard output and error as UTF-8. When the timeout passes, also while the input is still being written to a process that does not read it, the process tree is killed before standard input is closed.
        A process that is still running when the function fails is killed as well.
        The timeout also bounds reading the output: when a descendant keeps standard output or error open after the program has exited, the call returns as timed out instead of waiting for it.
        On Windows, a .cmd or .bat launcher such as an npm shim is started by CreateProcess through cmd.exe, with the same redirection and timeout; the tests run every fake agent that way.
        Agent processes get AI_AGENT_ACTIVE=1 in their environment, so a workflow script that they start refuses to run.

    .PARAMETER FilePath
        The program to run.

    .PARAMETER ArgumentList
        The arguments, each passed as one argument.

    .PARAMETER InputText
        Text for standard input.
        Standard input is closed without data when omitted.

    .PARAMETER WorkingDirectory
        The working directory.
        Defaults to the current location.

    .PARAMETER TimeoutSeconds
        How long the process may run.

    .PARAMETER Environment
        Extra environment variables for the process.

    .PARAMETER Agent
        Marks the process as an agent, which sets AI_AGENT_ACTIVE=1 and implies IsolateGit.

    .PARAMETER IsolateGit
        Removes the repository-local Git variables (the list of git rev-parse --local-env-vars, such as GIT_INDEX_FILE and GIT_DIR) that a hook inherits from Git.
        Without it, git commands in a check or an agent would use the temporary index of a path-limited commit instead of their own repository.

    .EXAMPLE
        Invoke-AIProcess -FilePath git -ArgumentList 'status' -TimeoutSeconds 30

    .OUTPUTS
        System.Management.Automation.PSCustomObject with ExitCode (null after a timeout), TimedOut, StandardOutput and StandardError.

    .NOTES
        Author  : Lars Krogh
        Version : 1.4.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$FilePath,
        [string[]]$ArgumentList = @(),
        [AllowEmptyString()]
        [string]$InputText,
        [string]$WorkingDirectory = (Get-Location).Path,
        [ValidateRange(1, 86400)]
        [int]$TimeoutSeconds = 600,
        [hashtable]$Environment = @{},
        [switch]$Agent,
        [switch]$IsolateGit
    )
    $utf8 = [Text.UTF8Encoding]::new($false)
    $startInfo = [Diagnostics.ProcessStartInfo]::new($FilePath)
    foreach ($argument in $ArgumentList) {
        $startInfo.ArgumentList.Add($argument)
    }
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.StandardOutputEncoding = $utf8
    $startInfo.StandardErrorEncoding = $utf8
    if ($Agent -or $IsolateGit) {
        foreach ($name in $script:GitLocalVariables) {
            $null = $startInfo.Environment.Remove($name)
        }
    }
    foreach ($name in $Environment.Keys) {
        $startInfo.Environment[$name] = [string]$Environment[$name]
    }
    if ($Agent) {
        $startInfo.Environment['AI_AGENT_ACTIVE'] = '1'
    }
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $process = [Diagnostics.Process]::Start($startInfo)
    $timedOut = $false
    try {
        $outputTask = $process.StandardOutput.ReadToEndAsync()
        $errorTask = $process.StandardError.ReadToEndAsync()
        $stdin = $process.StandardInput.BaseStream
        if ($PSBoundParameters.ContainsKey('InputText')) {
            $bytes = $utf8.GetBytes($InputText)
            foreach ($step in 'write', 'flush') {
                $task = if ($step -eq 'write') { $stdin.WriteAsync($bytes, 0, $bytes.Length) } else { $stdin.FlushAsync() }
                while ([Threading.Tasks.Task]::WaitAny([Threading.Tasks.Task[]]@($task), 200) -lt 0) {
                    if ($process.HasExited) {
                        break
                    }
                    if ([DateTime]::UtcNow -gt $deadline) {
                        $timedOut = $true
                        break
                    }
                }
                if ($timedOut -or -not $task.IsCompleted -or $task.IsFaulted) {
                    break
                }
            }
        }
        if ($timedOut) {
            $process.Kill($true)
            $process.WaitForExit()
        }
        try {
            $process.StandardInput.Dispose()
        }
        catch [IO.IOException], [InvalidOperationException] {
            Write-Verbose "Standard input of $FilePath closed before all input was written."
        }
        if (-not $timedOut) {
            $remaining = [Math]::Max(0, ($deadline - [DateTime]::UtcNow).TotalMilliseconds)
            $timedOut = -not $process.WaitForExit([int]$remaining)
            if ($timedOut) {
                $process.Kill($true)
                $process.WaitForExit()
            }
        }
        $drainMilliseconds = if ($timedOut) { 5000 } else { [Math]::Max(0, ($deadline - [DateTime]::UtcNow).TotalMilliseconds) }
        $drained = [Threading.Tasks.Task]::WaitAll([Threading.Tasks.Task[]]@($outputTask, $errorTask), [int]$drainMilliseconds)
        if (-not $drained) {
            $timedOut = $true
        }
        [PSCustomObject]@{
            ExitCode       = if ($timedOut) { $null } else { $process.ExitCode }
            TimedOut       = $timedOut
            StandardOutput = if ($outputTask.IsCompleted) { $outputTask.GetAwaiter().GetResult() } else { '' }
            StandardError  = if ($errorTask.IsCompleted) { $errorTask.GetAwaiter().GetResult() } else { '' }
        }
    }
    finally {
        if (-not $process.HasExited) {
            $process.Kill($true)
            $process.WaitForExit()
        }
        $process.Dispose()
    }
}

function Invoke-AIGit {
    <#
    .SYNOPSIS
        Runs git and returns its standard output.

    .DESCRIPTION
        Runs git in the given directory and throws with its error output when it fails.

    .PARAMETER ArgumentList
        The git arguments.

    .PARAMETER WorkingDirectory
        The repository directory.
        Defaults to the current location.

    .PARAMETER Environment
        Extra environment variables, for example GIT_INDEX_FILE.

    .PARAMETER AllowFailure
        Returns $null instead of throwing when git fails.

    .EXAMPLE
        Invoke-AIGit -ArgumentList 'write-tree'

    .OUTPUTS
        System.String. Standard output without the trailing line break.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string[]]$ArgumentList,
        [string]$WorkingDirectory = (Get-Location).Path,
        [hashtable]$Environment = @{},
        [switch]$AllowFailure
    )
    $result = Invoke-AIProcess -FilePath 'git' -ArgumentList (@('-c', 'core.quotepath=false') + $ArgumentList) -WorkingDirectory $WorkingDirectory -TimeoutSeconds 300 -Environment $Environment
    if ($result.ExitCode -ne 0) {
        if ($AllowFailure) {
            return $null
        }
        throw "git $($ArgumentList -join ' ') failed: $($result.StandardError.Trim())"
    }
    $result.StandardOutput.TrimEnd("`r", "`n")
}

function Get-AIRepositoryRoot {
    <#
    .SYNOPSIS
        Returns the root of the current Git working tree.

    .DESCRIPTION
        Uses git rev-parse --show-toplevel from the current location.

    .EXAMPLE
        Get-AIRepositoryRoot

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    [IO.Path]::GetFullPath((Invoke-AIGit -ArgumentList 'rev-parse', '--show-toplevel'))
}

function Get-AIConfiguration {
    <#
    .SYNOPSIS
        Reads scripts/ai/settings.psd1 of a repository.

    .DESCRIPTION
        The settings hold the agent commands, models, timeouts, limits and the deterministic checks.
        They are data, so they cannot run code when they are read.

    .PARAMETER RepositoryRoot
        The repository root.

    .EXAMPLE
        Get-AIConfiguration -RepositoryRoot (Get-AIRepositoryRoot)

    .OUTPUTS
        System.Collections.Hashtable

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot
    )
    Import-PowerShellDataFile -LiteralPath (Join-Path $RepositoryRoot 'scripts/ai/settings.psd1')
}

function Get-AIStateDirectory {
    <#
    .SYNOPSIS
        Returns a directory for workflow state inside the Git directory.

    .DESCRIPTION
        Review records, the research cache, the usage log and the write lock live in <git common dir>/ai, which is never committed and is shared by all worktrees.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Name
        An optional subdirectory, which is created when missing.

    .EXAMPLE
        Get-AIStateDirectory -RepositoryRoot $root -Name 'reviews'

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [string]$Name = ''
    )
    $gitDirectory = Invoke-AIGit -ArgumentList 'rev-parse', '--path-format=absolute', '--git-common-dir' -WorkingDirectory $RepositoryRoot
    $path = Join-Path $gitDirectory 'ai'
    if ($Name) {
        $path = Join-Path $path $Name
    }
    $null = New-Item -ItemType Directory -Path $path -Force
    $path
}

function Assert-AINotNested {
    <#
    .SYNOPSIS
        Refuses to run when an agent process started the current script.

    .DESCRIPTION
        Agent CLIs run with AI_AGENT_ACTIVE=1. A workflow script started by one of them, directly or through a Git hook, would start another agent, so it stops instead.

    .EXAMPLE
        Assert-AINotNested

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param()
    if ($env:AI_AGENT_ACTIVE) {
        throw 'This script was started by an agent process (AI_AGENT_ACTIVE is set). Agents may not start other agents or commit; return the result to Claude instead.'
    }
}

function Resolve-AICommand {
    <#
    .SYNOPSIS
        Returns the path of an installed command.

    .DESCRIPTION
        Looks the command up on PATH as an application and throws when it is missing.

    .PARAMETER Name
        The command name.

    .EXAMPLE
        Resolve-AICommand -Name 'codex'

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Name
    )
    $command = Get-Command -Name $Name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $command) {
        throw "$Name is not installed or not on PATH. Run scripts/ai/Test-AgentEnvironment.ps1 for setup help."
    }
    $command.Source
}

function Get-AIChangeset {
    <#
    .SYNOPSIS
        Describes the staged changes or a commit as a pair of Git trees.

    .DESCRIPTION
        A changeset is the difference between a base tree and a result tree.
        For staged changes these are the tree of HEAD (or the empty tree) and the tree of the index.
        For a commit they are the trees of its first parent and of the commit, so a merge commit's changeset is everything the merge brings into the first parent, including conflict resolutions.
        The ID "<base>-<result>" changes whenever any byte of the changeset changes, so a review record keyed by it cannot be reused for different content.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Commit
        A commit to describe.
        The staged changes are described when omitted.

    .EXAMPLE
        Get-AIChangeset -RepositoryRoot $root -Commit HEAD

    .OUTPUTS
        System.Management.Automation.PSCustomObject with Id, BaseTree, Tree, Commit and Files.

    .NOTES
        Author  : Lars Krogh
        Version : 1.1.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [string]$Commit
    )
    $commitId = $null
    if ($Commit) {
        $commitId = Invoke-AIGit -ArgumentList 'rev-parse', '--verify', "$Commit^{commit}" -WorkingDirectory $RepositoryRoot
        $parents = @((Invoke-AIGit -ArgumentList 'rev-list', '--parents', '-n', '1', $commitId -WorkingDirectory $RepositoryRoot).Split(' ') | Select-Object -Skip 1)
        $baseTree = if ($parents.Count -gt 0) { Invoke-AIGit -ArgumentList 'rev-parse', "$($parents[0])^{tree}" -WorkingDirectory $RepositoryRoot } else { $script:EmptyTree }
        $tree = Invoke-AIGit -ArgumentList 'rev-parse', "$commitId^{tree}" -WorkingDirectory $RepositoryRoot
    }
    else {
        $tree = Invoke-AIGit -ArgumentList 'write-tree' -WorkingDirectory $RepositoryRoot
        $baseTree = Invoke-AIGit -ArgumentList 'rev-parse', '--verify', '--quiet', 'HEAD^{tree}' -WorkingDirectory $RepositoryRoot -AllowFailure
        if (-not $baseTree) {
            $baseTree = $script:EmptyTree
        }
    }
    $names = Invoke-AIGit -ArgumentList 'diff', '--name-only', '--no-renames', $baseTree, $tree -WorkingDirectory $RepositoryRoot
    [PSCustomObject]@{
        Id       = "$baseTree-$tree"
        BaseTree = $baseTree
        Tree     = $tree
        Commit   = $commitId
        Files    = @($names -split "`n" | Where-Object { $_ })
    }
}

function Get-AIWorkingTreeId {
    <#
    .SYNOPSIS
        Returns a tree ID for the current content of the working tree.

    .DESCRIPTION
        Adds every tracked and untracked, not ignored file to a temporary copy of the index and writes it as a tree.
        Two equal IDs mean that no file changed in between.
        The real index is not touched.

    .PARAMETER RepositoryRoot
        The repository root.

    .EXAMPLE
        Get-AIWorkingTreeId -RepositoryRoot $root

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot
    )
    $index = Join-Path ([IO.Path]::GetTempPath()) ("ai-index-" + [guid]::NewGuid().ToString('N'))
    try {
        $realIndex = Invoke-AIGit -ArgumentList 'rev-parse', '--path-format=absolute', '--git-path', 'index' -WorkingDirectory $RepositoryRoot
        if (Test-Path -LiteralPath $realIndex) {
            Copy-Item -LiteralPath $realIndex -Destination $index
        }
        $environment = @{ GIT_INDEX_FILE = $index }
        $null = Invoke-AIGit -ArgumentList 'add', '--all', '--', '.' -WorkingDirectory $RepositoryRoot -Environment $environment
        Invoke-AIGit -ArgumentList 'write-tree' -WorkingDirectory $RepositoryRoot -Environment $environment
    }
    finally {
        Remove-Item -LiteralPath $index -Force -ErrorAction SilentlyContinue
    }
}

function Get-AIChangedPath {
    <#
    .SYNOPSIS
        Lists the files that differ between two trees.

    .DESCRIPTION
        Returns the repository-relative paths, with forward slashes, of files that were added, changed or removed.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Before
        The earlier tree.

    .PARAMETER After
        The later tree.

    .EXAMPLE
        Get-AIChangedPath -RepositoryRoot $root -Before $before -After $after

    .OUTPUTS
        System.String

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [string]$Before,
        [Parameter(Mandatory)]
        [string]$After
    )
    if ($Before -eq $After) {
        return
    }
    $names = Invoke-AIGit -ArgumentList 'diff', '--name-only', '--no-renames', $Before, $After -WorkingDirectory $RepositoryRoot
    $names -split "`n" | Where-Object { $_ }
}

function Test-AIPathMatch {
    <#
    .SYNOPSIS
        Tests whether a repository path matches any of a set of wildcard patterns.

    .DESCRIPTION
        Uses -like matching, in which * also matches /, so 'docs/*' covers the whole docs directory and '*.md' covers Markdown files at any depth.

    .PARAMETER Path
        A repository-relative path with forward slashes.

    .PARAMETER Pattern
        The patterns.

    .EXAMPLE
        Test-AIPathMatch -Path 'docs/setup.md' -Pattern 'docs/*'

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Path,
        [Parameter(Mandatory)]
        [string[]]$Pattern
    )
    foreach ($item in $Pattern) {
        if ($Path -like $item) {
            return $true
        }
    }
    $false
}

function Export-AITree {
    <#
    .SYNOPSIS
        Writes the files of a Git tree to a new directory.

    .DESCRIPTION
        Uses git archive, so the directory holds exactly the content of the tree, without unstaged or untracked files.
        git archive applies the tree's .gitattributes conversions as a checkout does, so files marked eol=crlf, such as *.ps1, are written with CRLF line endings.
        Checks and the Codex review run there.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Tree
        The tree to export.

    .EXAMPLE
        $snapshot = Export-AITree -RepositoryRoot $root -Tree $changeset.Tree

    .OUTPUTS
        System.String. The new directory.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [string]$Tree
    )
    $directory = Join-Path ([IO.Path]::GetTempPath()) ("ai-snapshot-" + [guid]::NewGuid().ToString('N'))
    $archive = "$directory.zip"
    $null = New-Item -ItemType Directory -Path $directory
    try {
        if ($Tree -ne $script:EmptyTree) {
            $null = Invoke-AIGit -ArgumentList 'archive', '--format=zip', '-o', $archive, $Tree -WorkingDirectory $RepositoryRoot
            Expand-Archive -LiteralPath $archive -DestinationPath $directory
        }
    }
    catch {
        Remove-Item -LiteralPath $directory -Recurse -Force -ErrorAction SilentlyContinue
        throw
    }
    finally {
        Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
    }
    $directory
}

function Get-AIReviewRecord {
    <#
    .SYNOPSIS
        Reads the stored review record of a changeset.

    .DESCRIPTION
        Returns $null when the changeset was never reviewed or the record does not belong to it.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Changeset
        The changeset from Get-AIChangeset.

    .EXAMPLE
        Get-AIReviewRecord -RepositoryRoot $root -Changeset $changeset

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [PSCustomObject]$Changeset
    )
    $path = Join-Path (Get-AIStateDirectory -RepositoryRoot $RepositoryRoot -Name 'reviews') "$($Changeset.Id).json"
    if (-not (Test-Path -LiteralPath $path)) {
        return $null
    }
    $record = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    if ($record.changeset -ne $Changeset.Id) {
        return $null
    }
    $record
}

function Test-AIApproval {
    <#
    .SYNOPSIS
        Tests whether a review record approves a changeset.

    .DESCRIPTION
        A record approves only the changeset it was made for, only with the verdict approve, and only when it has no finding with a blocking severity.
        The policy is applied here, so a reviewer that approves despite a blocking finding does not pass.
        Critical and high findings always block, whatever the settings say, so an edited settings file can add blocking severities but never remove these two.

    .PARAMETER Record
        The review record, or $null.

    .PARAMETER Changeset
        The changeset from Get-AIChangeset.

    .PARAMETER BlockingSeverities
        Further severities that block approval, in addition to critical and high.

    .EXAMPLE
        Test-AIApproval -Record $record -Changeset $changeset -BlockingSeverities 'critical', 'high'

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.1.0
    #>
    [CmdletBinding()]
    param(
        [AllowNull()]
        [PSCustomObject]$Record,
        [Parameter(Mandatory)]
        [PSCustomObject]$Changeset,
        [Parameter(Mandatory)]
        [AllowEmptyCollection()]
        [string[]]$BlockingSeverities
    )
    if (-not $Record -or $Record.changeset -ne $Changeset.Id -or $Record.verdict -ne 'approve') {
        return $false
    }
    $severities = @('critical', 'high') + $BlockingSeverities
    $blocking = @($Record.findings | Where-Object { $_.severity -in $severities })
    $blocking.Count -eq 0
}

function Test-AIApprovedPath {
    <#
    .SYNOPSIS
        Tests whether approved reviews lead from a changeset's base tree to its result tree.

    .DESCRIPTION
        Loads every stored review that approves its own changeset under Test-AIApproval and searches for a chain of them from the base tree to the result tree.
        A commit whose own changeset was reviewed passes with a chain of one.
        An amended commit passes when the original commit and the amendment were both approved, because each step was reviewed on the complete resulting tree.
        Records that cannot be read, are not objects, lack a field, or whose file name, changeset and trees disagree are ignored, so a damaged record never blocks other approvals.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Changeset
        The changeset from Get-AIChangeset.

    .PARAMETER BlockingSeverities
        Severities that block approval.

    .EXAMPLE
        Test-AIApprovedPath -RepositoryRoot $root -Changeset $changeset -BlockingSeverities 'critical', 'high'

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.1.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [PSCustomObject]$Changeset,
        [Parameter(Mandatory)]
        [string[]]$BlockingSeverities
    )
    $edges = @{}
    $directory = Get-AIStateDirectory -RepositoryRoot $RepositoryRoot -Name 'reviews'
    foreach ($file in @(Get-ChildItem -LiteralPath $directory -Filter '*.json' -File)) {
        try {
            $record = Get-Content -LiteralPath $file.FullName -Raw | ConvertFrom-Json
            if ($record -isnot [PSCustomObject]) {
                continue
            }
            $properties = @($record.PSObject.Properties.Name)
            $missing = @('base_tree', 'tree', 'changeset', 'verdict', 'findings' | Where-Object { $_ -notin $properties })
            if ($missing.Count -gt 0 -or $record.base_tree -isnot [string] -or $record.tree -isnot [string]) {
                continue
            }
            $id = "$($record.base_tree)-$($record.tree)"
            if ($record.changeset -ne $id -or $file.BaseName -ne $id) {
                continue
            }
            if (-not (Test-AIApproval -Record $record -Changeset ([PSCustomObject]@{ Id = $id }) -BlockingSeverities $BlockingSeverities)) {
                continue
            }
        }
        catch {
            Write-Verbose "Ignoring malformed review record $($file.Name): $($_.Exception.Message)"
            continue
        }
        if (-not $edges.ContainsKey($record.base_tree)) {
            $edges[$record.base_tree] = [Collections.Generic.List[string]]::new()
        }
        $edges[$record.base_tree].Add($record.tree)
    }
    $visited = [Collections.Generic.HashSet[string]]::new()
    $queue = [Collections.Generic.Queue[string]]::new()
    $queue.Enqueue($Changeset.BaseTree)
    $null = $visited.Add($Changeset.BaseTree)
    while ($queue.Count -gt 0) {
        $current = $queue.Dequeue()
        if (-not $edges.ContainsKey($current)) {
            continue
        }
        foreach ($next in $edges[$current]) {
            if ($next -eq $Changeset.Tree) {
                return $true
            }
            if ($visited.Add($next)) {
                $queue.Enqueue($next)
            }
        }
    }
    $false
}

function Add-AIUsage {
    <#
    .SYNOPSIS
        Appends one agent call to the usage log.

    .DESCRIPTION
        Writes a JSON line with the time, agent, operation and the reported usage to <git common dir>/ai/usage.jsonl.
        Prompts and responses are not logged.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Agent
        The agent name.

    .PARAMETER Operation
        What the call did.

    .PARAMETER Usage
        Token counts, costs, durations and outcome as reported by the CLI.

    .EXAMPLE
        Add-AIUsage -RepositoryRoot $root -Agent 'codex' -Operation 'review' -Usage @{ tokens = 1200 }

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [string]$Agent,
        [Parameter(Mandatory)]
        [string]$Operation,
        [hashtable]$Usage = @{}
    )
    $entry = [ordered]@{ time = [DateTime]::UtcNow.ToString('o'); agent = $Agent; operation = $Operation }
    foreach ($key in $Usage.Keys) {
        $entry[$key] = $Usage[$key]
    }
    $path = Join-Path (Get-AIStateDirectory -RepositoryRoot $RepositoryRoot) 'usage.jsonl'
    Add-Content -LiteralPath $path -Value ($entry | ConvertTo-Json -Compress -Depth 5) -Encoding utf8NoBOM
}

function Enter-AIWriteLock {
    <#
    .SYNOPSIS
        Takes the lock that marks an agent other than Claude as changing files.

    .DESCRIPTION
        Creates <git common dir>/ai/agent.lock exclusively.
        While it is held, a second writing agent and the pre-commit gate refuse to start.
        The lock is released by disposing the returned stream, and the operating system releases it when the process ends.

    .PARAMETER RepositoryRoot
        The repository root.

    .EXAMPLE
        $lock = Enter-AIWriteLock -RepositoryRoot $root; try { } finally { $lock.Dispose() }

    .OUTPUTS
        System.IO.FileStream

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot
    )
    $path = Join-Path (Get-AIStateDirectory -RepositoryRoot $RepositoryRoot) 'agent.lock'
    if (Test-AIWriteLock -RepositoryRoot $RepositoryRoot) {
        throw "Another agent is changing files in this repository ($path)."
    }
    [IO.FileStream]::new($path, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None, 4096, [IO.FileOptions]::DeleteOnClose)
}

function Test-AIWriteLock {
    <#
    .SYNOPSIS
        Tests whether another process holds the agent write lock.

    .DESCRIPTION
        A lock file that can be opened exclusively is left over from a process that ended without removing it, so it is deleted and reported as free.

    .PARAMETER RepositoryRoot
        The repository root.

    .EXAMPLE
        Test-AIWriteLock -RepositoryRoot $root

    .OUTPUTS
        System.Boolean

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot
    )
    $path = Join-Path (Get-AIStateDirectory -RepositoryRoot $RepositoryRoot) 'agent.lock'
    if (-not (Test-Path -LiteralPath $path)) {
        return $false
    }
    try {
        $stream = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
        $stream.Dispose()
        Remove-Item -LiteralPath $path -Force
        $false
    }
    catch [IO.IOException] {
        $true
    }
}

function Invoke-AICheck {
    <#
    .SYNOPSIS
        Runs the deterministic checks that apply to a changeset.

    .DESCRIPTION
        Runs each check from the settings whose path patterns match a changed file, in the given directory, and stops at the first failure.
        A check fails on a non-zero exit code, a timeout, a missing command, or, with FailOnOutput, any output.

    .PARAMETER Settings
        The settings from Get-AIConfiguration.

    .PARAMETER Changeset
        The changeset from Get-AIChangeset.

    .PARAMETER Directory
        The directory to run the checks in, normally an exported snapshot.

    .EXAMPLE
        Invoke-AICheck -Settings $settings -Changeset $changeset -Directory $snapshot

    .OUTPUTS
        System.Management.Automation.PSCustomObject with Name, Passed and Output, one per check that ran.

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [hashtable]$Settings,
        [Parameter(Mandatory)]
        [PSCustomObject]$Changeset,
        [Parameter(Mandatory)]
        [string]$Directory
    )
    foreach ($check in $Settings.Checks) {
        $matching = @($Changeset.Files | Where-Object { Test-AIPathMatch -Path $_ -Pattern $check.Paths })
        if ($matching.Count -eq 0) {
            continue
        }
        Write-AIMessage "  check: $($check.Name)"
        $failOnOutput = $check.ContainsKey('FailOnOutput') -and $check.FailOnOutput
        try {
            $program = Resolve-AICommand -Name $check.Command[0]
            $run = Invoke-AIProcess -FilePath $program -ArgumentList @($check.Command | Select-Object -Skip 1) -WorkingDirectory $Directory -TimeoutSeconds $Settings.CheckTimeoutSeconds -IsolateGit
            $output = ($run.StandardOutput + $run.StandardError).Trim()
            $passed = -not $run.TimedOut -and $run.ExitCode -eq 0 -and -not ($failOnOutput -and $run.StandardOutput.Trim())
            if ($run.TimedOut) {
                $output = "Timed out after $($Settings.CheckTimeoutSeconds) seconds.`n$output"
            }
        }
        catch {
            $passed = $false
            $output = $_.Exception.Message
        }
        [PSCustomObject]@{ Name = $check.Name; Passed = $passed; Output = $output }
        if (-not $passed) {
            return
        }
    }
}

function ConvertTo-AIReview {
    <#
    .SYNOPSIS
        Validates the JSON that Codex returned and converts it to a review.

    .DESCRIPTION
        The answer must be valid JSON that matches the review schema in every field, type and allowed value: the verdict approve or reject, a string summary, and an array of findings with a known severity, a string file, an integer line, a title, an impact and a remediation.
        Anything else is rejected, so a malformed answer never counts as an approval.

    .PARAMETER Json
        The final message from Codex.

    .PARAMETER SchemaPath
        The review schema that Codex was given, scripts/ai/review-schema.json.

    .EXAMPLE
        ConvertTo-AIReview -Json (Get-Content review.json -Raw) -SchemaPath scripts/ai/review-schema.json

    .OUTPUTS
        System.Management.Automation.PSCustomObject

    .NOTES
        Author  : Lars Krogh
        Version : 1.1.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [AllowEmptyString()]
        [string]$Json,
        [Parameter(Mandatory)]
        [string]$SchemaPath
    )
    try {
        $review = $Json | ConvertFrom-Json -ErrorAction Stop
    }
    catch {
        throw "Codex did not return valid JSON: $($_.Exception.Message)"
    }
    if (-not $review) {
        throw 'Codex returned an empty review.'
    }
    $schema = Get-Content -LiteralPath $SchemaPath -Raw
    $schemaErrors = $null
    $valid = Test-Json -Json $Json -Schema $schema -ErrorAction SilentlyContinue -ErrorVariable schemaErrors
    if (-not $valid) {
        throw "The Codex review does not match the review schema: $(@($schemaErrors | ForEach-Object { $_.Exception.Message }) -join '; ')"
    }
    $review
}

function Invoke-AIReviewGate {
    <#
    .SYNOPSIS
        Runs the checks and the Codex review for a changeset and stores the result.

    .DESCRIPTION
        Exports the result tree of the changeset, runs the matching checks there and, when they pass, asks Codex in a read-only sandbox to review the diff against the repository rules.
        The checks, review-prompt.md and review-schema.json are read from that snapshot, so unstaged edits to them cannot change how the staged content is checked. Codex commands, limits and the blocking severities come from the Settings argument.
        The record is saved for every completed review, approved or not.
        A failed or timed-out diff, failed checks, Codex errors, timeouts and malformed answers throw, so no record and no approval is produced.

    .PARAMETER RepositoryRoot
        The repository root.

    .PARAMETER Settings
        The settings from Get-AIConfiguration.

    .PARAMETER Changeset
        The changeset from Get-AIChangeset.

    .EXAMPLE
        Invoke-AIReviewGate -RepositoryRoot $root -Settings $settings -Changeset $changeset

    .OUTPUTS
        System.Management.Automation.PSCustomObject. The review record.

    .NOTES
        Author  : Lars Krogh
        Version : 1.2.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$RepositoryRoot,
        [Parameter(Mandatory)]
        [hashtable]$Settings,
        [Parameter(Mandatory)]
        [PSCustomObject]$Changeset
    )
    $codex = Resolve-AICommand -Name $Settings.Codex.Command
    $diffRun = Invoke-AIProcess -FilePath 'git' -ArgumentList 'diff', '--no-color', '--no-ext-diff', '--no-textconv', '--find-renames', $Changeset.BaseTree, $Changeset.Tree -WorkingDirectory $RepositoryRoot -TimeoutSeconds 300
    if ($diffRun.TimedOut -or $diffRun.ExitCode -ne 0) {
        throw "git diff failed for changeset $($Changeset.Id), so it cannot be reviewed: $(if ($diffRun.TimedOut) { 'timed out' } else { "exit code $($diffRun.ExitCode). $($diffRun.StandardError.Trim())" })"
    }
    $diff = $diffRun.StandardOutput
    $size = [Text.Encoding]::UTF8.GetByteCount($diff)
    if ($size -gt $Settings.Codex.MaxDiffBytes) {
        throw "The changeset diff has $size bytes, more than the review limit of $($Settings.Codex.MaxDiffBytes). Split it into smaller commits."
    }
    $snapshot = Export-AITree -RepositoryRoot $RepositoryRoot -Tree $Changeset.Tree
    $work = Join-Path ([IO.Path]::GetTempPath()) ("ai-review-" + [guid]::NewGuid().ToString('N'))
    $null = New-Item -ItemType Directory -Path $work
    try {
        Write-AIMessage "Checking changeset $($Changeset.Id)"
        try {
            $stagedSettings = Get-AIConfiguration -RepositoryRoot $snapshot
        }
        catch {
            throw "The changeset has no readable scripts/ai/settings.psd1, so its checks are unknown: $($_.Exception.Message)"
        }
        $checks = @(Invoke-AICheck -Settings $stagedSettings -Changeset $Changeset -Directory $snapshot)
        $failed = @($checks | Where-Object { -not $_.Passed })
        if ($failed.Count -gt 0) {
            throw "Check '$($failed[0].Name)' failed:`n$($failed[0].Output)"
        }
        $schema = Join-Path $snapshot 'scripts/ai/review-schema.json'
        $output = Join-Path $work 'review.json'
        $arguments = @('exec', '--sandbox', 'read-only', '--ephemeral', '--skip-git-repo-check', '-C', $snapshot, '--output-schema', $schema, '-o', $output)
        if ($Settings.Codex.Model) {
            $arguments += @('-m', $Settings.Codex.Model)
        }
        $arguments += '-'
        $prompt = (Get-Content -LiteralPath (Join-Path $snapshot 'scripts/ai/review-prompt.md') -Raw) + "`n<changed_files>`n$($Changeset.Files -join "`n")`n</changed_files>`n`n<diff>`n$diff`n</diff>`n"
        $review = $null
        $failure = $null
        for ($attempt = 1; $attempt -le $Settings.Codex.MaxAttempts -and -not $review; $attempt++) {
            Write-AIMessage "Requesting Codex review (attempt $attempt of $($Settings.Codex.MaxAttempts))"
            Remove-Item -LiteralPath $output -Force -ErrorAction SilentlyContinue
            $started = [DateTime]::UtcNow
            $run = Invoke-AIProcess -FilePath $codex -ArgumentList $arguments -InputText $prompt -WorkingDirectory $snapshot -TimeoutSeconds $Settings.Codex.TimeoutSeconds -Agent
            $tokens = if ($run.StandardError -match '(?m)^tokens used\s*\r?\n\s*([\d,]+)') { [long]($Matches[1] -replace ',', '') } else { $null }
            Add-AIUsage -RepositoryRoot $RepositoryRoot -Agent 'codex' -Operation 'review' -Usage @{ changeset = $Changeset.Id; attempt = $attempt; tokens = $tokens; seconds = [int]([DateTime]::UtcNow - $started).TotalSeconds; exit_code = $run.ExitCode; timed_out = $run.TimedOut }
            if ($run.TimedOut) {
                $failure = "Codex timed out after $($Settings.Codex.TimeoutSeconds) seconds."
                break
            }
            if ($run.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $output)) {
                $failure = "Codex exited with code $($run.ExitCode): $((($run.StandardError -split "`n") | Select-Object -Last 5) -join "`n")"
                continue
            }
            try {
                $review = ConvertTo-AIReview -Json (Get-Content -LiteralPath $output -Raw) -SchemaPath $schema
            }
            catch {
                $failure = $_.Exception.Message
            }
        }
        if (-not $review) {
            throw "The Codex review failed, so the changeset is not approved. $failure"
        }
        $record = [PSCustomObject][ordered]@{
            changeset   = $Changeset.Id
            base_tree   = $Changeset.BaseTree
            tree        = $Changeset.Tree
            commit      = $Changeset.Commit
            verdict     = $review.verdict
            approved    = $false
            summary     = $review.summary
            findings    = @($review.findings)
            checks      = @($checks | ForEach-Object Name)
            reviewer    = 'codex'
            model       = $Settings.Codex.Model
            reviewed_at = [DateTime]::UtcNow.ToString('o')
        }
        $record.approved = Test-AIApproval -Record $record -Changeset $Changeset -BlockingSeverities $Settings.Codex.BlockingSeverities
        $path = Join-Path (Get-AIStateDirectory -RepositoryRoot $RepositoryRoot -Name 'reviews') "$($Changeset.Id).json"
        Set-Content -LiteralPath $path -Value ($record | ConvertTo-Json -Depth 6) -Encoding utf8NoBOM
        $record
    }
    finally {
        Remove-Item -LiteralPath $snapshot, $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Write-AIReview {
    <#
    .SYNOPSIS
        Prints a review record for people and for Claude.

    .DESCRIPTION
        Shows the verdict, the summary and each finding with its severity, location, impact and remediation.

    .PARAMETER Record
        The review record.

    .EXAMPLE
        Write-AIReview -Record $record

    .NOTES
        Author  : Lars Krogh
        Version : 1.0.0
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Record
    )
    $status = if ($Record.approved) { 'APPROVED' } else { 'REJECTED' }
    Write-AIMessage "Codex review: $status ($($Record.changeset))"
    Write-AIMessage "  $($Record.summary)"
    foreach ($finding in @($Record.findings)) {
        Write-AIMessage ("  [{0}] {1}:{2} {3}" -f $finding.severity.ToUpperInvariant(), $finding.file, $finding.line, $finding.title)
        Write-AIMessage "      Impact: $($finding.impact)"
        Write-AIMessage "      Fix:    $($finding.remediation)"
    }
}
