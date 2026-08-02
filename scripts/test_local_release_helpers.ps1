$ErrorActionPreference = 'Stop'

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $scriptRoot '..'))
$cleanupScript = Join-Path $scriptRoot 'cleanup_local_release.ps1'
$buildScript = Join-Path $scriptRoot 'build_release_candidate_local.ps1'
$releaseId = "test-local-cleanup-$PID"
$releaseRoot = Join-Path $repoRoot "releases\$releaseId"
$sourceRoot = Join-Path $releaseRoot 'src'
$toolchainRoot = Join-Path ([System.IO.Path]::GetTempPath()) "newapi-local-release-test-$PID"
$releaseCacheRoot = Join-Path $toolchainRoot 'release-cache'
$legacyCacheRoot = Join-Path $toolchainRoot 'cache'
$goCacheRoot = Join-Path $repoRoot '.gocache'
$frontendDistRoot = Join-Path $repoRoot 'web\dist'
$frontendModulesRoot = Join-Path $repoRoot 'web\node_modules'
$savedFrontendDistRoot = Join-Path $toolchainRoot 'saved-web-dist'
$savedFrontendModulesRoot = Join-Path $toolchainRoot 'saved-web-node-modules'
$previousToolchainRoot = $env:LOCAL_TOOLCHAIN_ROOT

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

function Remove-TreeIfExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    for ($attempt = 1; $attempt -le 3; $attempt++) {
        if (-not (Test-Path -LiteralPath $Path)) {
            return
        }
        try {
            Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop
            return
        } catch [System.IO.DirectoryNotFoundException] {
            return
        } catch {
            if (-not (Test-Path -LiteralPath $Path)) {
                return
            }
            if ($attempt -eq 3) {
                throw
            }
            Start-Sleep -Milliseconds 250
        }
    }
}

try {
    $env:LOCAL_TOOLCHAIN_ROOT = $toolchainRoot
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

    New-Item -ItemType Directory -Force -Path $toolchainRoot | Out-Null
    if (Test-Path -LiteralPath $frontendDistRoot) {
        Move-Item -LiteralPath $frontendDistRoot -Destination $savedFrontendDistRoot
    }
    if (Test-Path -LiteralPath $frontendModulesRoot) {
        Move-Item -LiteralPath $frontendModulesRoot -Destination $savedFrontendModulesRoot
    }

    New-Item -ItemType Directory -Force -Path `
        (Join-Path $releaseRoot 'bin'), `
        (Join-Path $releaseRoot 'runtime\logs'), `
        $releaseCacheRoot, `
        $legacyCacheRoot, `
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
    Set-Content -LiteralPath (Join-Path $releaseCacheRoot 'cache.marker') -Value 'cache'
    Set-Content -LiteralPath (Join-Path $legacyCacheRoot 'cache.marker') -Value 'legacy-cache'
    Set-Content -LiteralPath (Join-Path $goCacheRoot 'cache.marker') -Value 'cache'
    Set-Content -LiteralPath (Join-Path $frontendDistRoot 'dist.marker') -Value 'dist'
    Set-Content -LiteralPath (Join-Path $frontendModulesRoot 'modules.marker') -Value 'modules'

    & $cleanupScript -ReleaseId $releaseId -KeepCandidate
    Assert-True (Test-Path -LiteralPath (Join-Path $releaseRoot 'bin\new-api')) 'KeepCandidate removed the binary'
    Assert-True (Test-Path -LiteralPath (Join-Path $releaseRoot 'manifest.env')) 'KeepCandidate removed the manifest'
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $releaseRoot 'src'))) 'KeepCandidate left the source directory'
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $releaseRoot 'runtime'))) 'KeepCandidate left runtime files'
    Assert-True (Test-Path -LiteralPath $releaseCacheRoot) 'Cleanup removed the persistent release build cache'
    Assert-True (Test-Path -LiteralPath $legacyCacheRoot) 'Cleanup removed the legacy build cache without an explicit purge'
    Assert-True (-not (Test-Path -LiteralPath $goCacheRoot)) 'Cleanup left the Go build cache'
    Assert-True (-not (Test-Path -LiteralPath $frontendDistRoot)) 'Cleanup left frontend dist files'
    Assert-True (-not (Test-Path -LiteralPath $frontendModulesRoot)) 'Cleanup left frontend dependencies'
    $worktreeList = git -C $repoRoot worktree list --porcelain
    Assert-True (-not ($worktreeList -contains "worktree $($sourceRoot.Replace('\', '/'))")) 'Cleanup left registered worktree metadata'

    & $cleanupScript -ReleaseId $releaseId
    Assert-True (-not (Test-Path -LiteralPath $releaseRoot)) 'Cleanup left the local release directory'

    & $cleanupScript -ReleaseId $releaseId -PurgeBuildCache
    Assert-True (-not (Test-Path -LiteralPath $releaseCacheRoot)) 'PurgeBuildCache left the persistent release build cache'
    Assert-True (-not (Test-Path -LiteralPath $legacyCacheRoot)) 'PurgeBuildCache left the legacy build cache'

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
        git -C $repoRoot worktree prune | Out-Null
    }
    foreach ($path in @($goCacheRoot, $frontendDistRoot, $frontendModulesRoot)) {
        Remove-TreeIfExists -Path $path
    }
    Remove-TreeIfExists -Path $releaseRoot
    if (Test-Path -LiteralPath $savedFrontendDistRoot) {
        Move-Item -LiteralPath $savedFrontendDistRoot -Destination $frontendDistRoot
    }
    if (Test-Path -LiteralPath $savedFrontendModulesRoot) {
        Move-Item -LiteralPath $savedFrontendModulesRoot -Destination $frontendModulesRoot
    }
    Remove-TreeIfExists -Path $toolchainRoot
    if ($null -eq $previousToolchainRoot) {
        Remove-Item Env:\LOCAL_TOOLCHAIN_ROOT -ErrorAction SilentlyContinue
    } else {
        $env:LOCAL_TOOLCHAIN_ROOT = $previousToolchainRoot
    }
}

Write-Output 'local-release-helper-contracts:PASS'
