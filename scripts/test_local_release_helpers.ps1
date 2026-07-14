$ErrorActionPreference = 'Stop'

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $scriptRoot '..'))
$cleanupScript = Join-Path $scriptRoot 'cleanup_local_release.ps1'
$buildScript = Join-Path $scriptRoot 'build_release_candidate_local.ps1'
$releaseId = "test-local-cleanup-$PID"
$releaseRoot = Join-Path $repoRoot "releases\$releaseId"
$sourceRoot = Join-Path $releaseRoot 'src'
$cacheRoot = Join-Path $repoRoot '.local-tools\cache'
$goCacheRoot = Join-Path $repoRoot '.gocache'
$frontendDistRoot = Join-Path $repoRoot 'web\default\dist'
$frontendModulesRoot = Join-Path $repoRoot 'web\node_modules'

function Assert-True {
    param(
        [Parameter(Mandatory = $true)]
        [bool]$Condition,

        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

try {
    foreach ($path in @($cleanupScript, $buildScript)) {
        $tokens = $null
        $errors = $null
        [System.Management.Automation.Language.Parser]::ParseFile(
            $path,
            [ref]$tokens,
            [ref]$errors
        ) | Out-Null
        Assert-True ($errors.Count -eq 0) "PowerShell parse errors in $path"
    }

    New-Item -ItemType Directory -Force -Path `
        (Join-Path $releaseRoot 'bin'), `
        (Join-Path $releaseRoot 'runtime\logs'), `
        $cacheRoot, `
        $goCacheRoot, `
        $frontendDistRoot, `
        $frontendModulesRoot | Out-Null
    git -C $repoRoot worktree add --detach $sourceRoot HEAD | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to create the local cleanup worktree fixture'
    }
    Set-Content -LiteralPath (Join-Path $releaseRoot 'bin\new-api') -Value 'candidate'
    Set-Content -LiteralPath (Join-Path $releaseRoot 'manifest.env') -Value "RELEASE_ID=$releaseId"
    Set-Content -LiteralPath (Join-Path $releaseRoot 'runtime\candidate.log') -Value 'log'
    Set-Content -LiteralPath (Join-Path $cacheRoot 'cache.marker') -Value 'cache'
    Set-Content -LiteralPath (Join-Path $goCacheRoot 'cache.marker') -Value 'cache'
    Set-Content -LiteralPath (Join-Path $frontendDistRoot 'dist.marker') -Value 'dist'
    Set-Content -LiteralPath (Join-Path $frontendModulesRoot 'modules.marker') -Value 'modules'

    & $cleanupScript -ReleaseId $releaseId -KeepCandidate
    Assert-True (Test-Path -LiteralPath (Join-Path $releaseRoot 'bin\new-api')) 'KeepCandidate removed the binary'
    Assert-True (Test-Path -LiteralPath (Join-Path $releaseRoot 'manifest.env')) 'KeepCandidate removed the manifest'
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $releaseRoot 'src'))) 'KeepCandidate left the source directory'
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $releaseRoot 'runtime'))) 'KeepCandidate left runtime files'
    Assert-True (-not (Test-Path -LiteralPath $cacheRoot)) 'Cleanup left the local build cache'
    Assert-True (-not (Test-Path -LiteralPath $goCacheRoot)) 'Cleanup left the Go build cache'
    Assert-True (-not (Test-Path -LiteralPath $frontendDistRoot)) 'Cleanup left frontend dist files'
    Assert-True (-not (Test-Path -LiteralPath $frontendModulesRoot)) 'Cleanup left frontend dependencies'
    $worktreeList = git -C $repoRoot worktree list --porcelain
    Assert-True (-not ($worktreeList -contains "worktree $($sourceRoot.Replace('\', '/'))")) 'Cleanup left registered worktree metadata'

    & $cleanupScript -ReleaseId $releaseId
    Assert-True (-not (Test-Path -LiteralPath $releaseRoot)) 'Cleanup left the local release directory'

    $invalidRejected = $false
    try {
        & $cleanupScript -ReleaseId '../outside'
    } catch {
        $invalidRejected = $true
    }
    Assert-True $invalidRejected 'Cleanup accepted an invalid release id'
}
finally {
    $worktreeList = git -C $repoRoot worktree list --porcelain
    if ($worktreeList -contains "worktree $($sourceRoot.Replace('\', '/'))") {
        git -C $repoRoot worktree remove --force $sourceRoot | Out-Null
    }
    if (Test-Path -LiteralPath $releaseRoot) {
        Remove-Item -LiteralPath $releaseRoot -Recurse -Force
    }
    foreach ($path in @($cacheRoot, $goCacheRoot, $frontendDistRoot, $frontendModulesRoot)) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Recurse -Force
        }
    }
}

Write-Output 'local-release-helper-contracts:PASS'
