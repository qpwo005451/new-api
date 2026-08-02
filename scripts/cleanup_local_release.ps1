[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9._-]+$')]
    [string]$ReleaseId,

    [switch]$KeepCandidate,

    [switch]$KeepCache,

    [switch]$PurgeBuildCache
)

$ErrorActionPreference = 'Stop'

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $scriptRoot '..'))
$toolchainRoot = if ($env:LOCAL_TOOLCHAIN_ROOT) {
    [System.IO.Path]::GetFullPath($env:LOCAL_TOOLCHAIN_ROOT)
} else {
    [System.IO.Path]::GetFullPath((Join-Path $repoRoot '.local-tools'))
}
$releasesRoot = [System.IO.Path]::GetFullPath((Join-Path $repoRoot 'releases'))
$releaseRoot = [System.IO.Path]::GetFullPath((Join-Path $releasesRoot $ReleaseId))
$sourceRoot = Join-Path $releaseRoot 'src'
$runtimeRoot = Join-Path $releaseRoot 'runtime'
$releaseCacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'release-cache'))
$legacyCacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'cache'))
$workspaceBuildArtifacts = @(
    (Join-Path $repoRoot '.gocache'),
    (Join-Path $repoRoot '.gocache-temp'),
    (Join-Path $repoRoot '.gomodcache'),
    (Join-Path $repoRoot '.gopath'),
    (Join-Path $repoRoot 'web\node_modules'),
    (Join-Path $repoRoot 'web\default\node_modules'),
    (Join-Path $repoRoot 'web\classic\node_modules'),
    (Join-Path $repoRoot 'web\dist'),
    (Join-Path $repoRoot 'web\default\dist'),
    (Join-Path $repoRoot 'web\classic\dist')
)

function Assert-ChildPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Parent,

        [Parameter(Mandatory = $true)]
        [string]$Child
    )

    $parentPrefix = [System.IO.Path]::GetFullPath($Parent) + [System.IO.Path]::DirectorySeparatorChar
    if (-not [System.IO.Path]::GetFullPath($Child).StartsWith(
        $parentPrefix,
        [System.StringComparison]::OrdinalIgnoreCase
    )) {
        throw "Path escaped its expected parent: $Child"
    }
}

function Remove-DirectoryWithRetry {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Parent,

        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    Assert-ChildPath -Parent $Parent -Child $Path
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        if (-not (Test-Path -LiteralPath $Path)) {
            return
        }
        try {
            Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction SilentlyContinue
        } catch {
            # A real filesystem failure is confirmed below from the root path.
        }
        if (-not (Test-Path -LiteralPath $Path)) {
            return
        }
        if ($attempt -eq 3) {
            throw "Failed to remove $Description after $attempt attempts: $Path"
        }
        Start-Sleep -Seconds 1
    }
}

function Invoke-GitWithTimeout {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    $process = Start-Process -FilePath 'git.exe' -ArgumentList $Arguments -PassThru -WindowStyle Hidden
    if (-not $process.WaitForExit(60000)) {
        try {
            $process.Kill($true)
        } catch {
            $process.Kill()
        }
        $process.WaitForExit()
        throw "Timed out while $Description"
    }
    if ($process.ExitCode -ne 0) {
        throw "Failed while $Description"
    }
}

Assert-ChildPath -Parent $releasesRoot -Child $releaseRoot
Assert-ChildPath -Parent $toolchainRoot -Child $releaseCacheRoot
Assert-ChildPath -Parent $toolchainRoot -Child $legacyCacheRoot

if (Test-Path -LiteralPath $sourceRoot) {
    $registeredWorktrees = git -C $repoRoot worktree list --porcelain
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to inspect local Git worktrees'
    }
    if ($registeredWorktrees -contains "worktree $($sourceRoot.Replace('\', '/'))") {
        Invoke-GitWithTimeout `
            -Arguments @('-C', $repoRoot, 'worktree', 'remove', '--force', $sourceRoot) `
            -Description "removing release source worktree: $sourceRoot"
    } else {
        Remove-DirectoryWithRetry -Parent $releaseRoot -Path $sourceRoot -Description 'release source directory'
    }
}

Invoke-GitWithTimeout `
    -Arguments @('-C', $repoRoot, 'worktree', 'prune') `
    -Description 'pruning local Git worktree metadata'

if ($KeepCandidate) {
    if (Test-Path -LiteralPath $runtimeRoot) {
        Remove-DirectoryWithRetry -Parent $releaseRoot -Path $runtimeRoot -Description 'candidate runtime directory'
    }
} elseif (Test-Path -LiteralPath $releaseRoot) {
    Remove-DirectoryWithRetry -Parent $releasesRoot -Path $releaseRoot -Description 'candidate release directory'
}

if ($PurgeBuildCache) {
    foreach ($cachePath in @($releaseCacheRoot, $legacyCacheRoot)) {
        if (Test-Path -LiteralPath $cachePath) {
            Remove-DirectoryWithRetry -Parent $toolchainRoot -Path $cachePath -Description 'local release build cache'
        }
    }
}
if (-not $KeepCache) {
    foreach ($artifactPath in $workspaceBuildArtifacts) {
        if (Test-Path -LiteralPath $artifactPath) {
            Remove-DirectoryWithRetry -Parent $repoRoot -Path $artifactPath -Description 'workspace build artifact'
        }
    }
}

Write-Output "Local release cleanup complete: $ReleaseId"
