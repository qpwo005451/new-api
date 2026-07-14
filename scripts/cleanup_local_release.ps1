[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9._-]+$')]
    [string]$ReleaseId,

    [switch]$KeepCandidate,

    [switch]$KeepCache
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
$cacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'cache'))
$workspaceBuildArtifacts = @(
    (Join-Path $repoRoot '.gocache'),
    (Join-Path $repoRoot '.gocache-temp'),
    (Join-Path $repoRoot '.gomodcache'),
    (Join-Path $repoRoot '.gopath'),
    (Join-Path $repoRoot 'web\node_modules'),
    (Join-Path $repoRoot 'web\default\node_modules'),
    (Join-Path $repoRoot 'web\classic\node_modules'),
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

Assert-ChildPath -Parent $releasesRoot -Child $releaseRoot
Assert-ChildPath -Parent $toolchainRoot -Child $cacheRoot

if (Test-Path -LiteralPath $sourceRoot) {
    $registeredWorktrees = git -C $repoRoot worktree list --porcelain
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to inspect local Git worktrees'
    }
    if ($registeredWorktrees -contains "worktree $($sourceRoot.Replace('\', '/'))") {
        git -C $repoRoot worktree remove --force $sourceRoot
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to remove release source worktree: $sourceRoot"
        }
    } else {
        Remove-Item -LiteralPath $sourceRoot -Recurse -Force
    }
}

git -C $repoRoot worktree prune
if ($LASTEXITCODE -ne 0) {
    throw 'Failed to prune local Git worktree metadata'
}

if ($KeepCandidate) {
    if (Test-Path -LiteralPath $runtimeRoot) {
        Remove-Item -LiteralPath $runtimeRoot -Recurse -Force
    }
} elseif (Test-Path -LiteralPath $releaseRoot) {
    Remove-Item -LiteralPath $releaseRoot -Recurse -Force
}

if (-not $KeepCache -and (Test-Path -LiteralPath $cacheRoot)) {
    Remove-Item -LiteralPath $cacheRoot -Recurse -Force
}
if (-not $KeepCache) {
    foreach ($artifactPath in $workspaceBuildArtifacts) {
        Assert-ChildPath -Parent $repoRoot -Child $artifactPath
        if (Test-Path -LiteralPath $artifactPath) {
            Remove-Item -LiteralPath $artifactPath -Recurse -Force
        }
    }
}

Write-Output "Local release cleanup complete: $ReleaseId"
