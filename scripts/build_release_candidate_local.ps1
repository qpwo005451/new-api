[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9._-]+$')]
    [string]$ReleaseId,

    [string]$ReleaseTag = 'HEAD',

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
$bash = if ($env:GIT_BASH) { $env:GIT_BASH } else { 'C:\Program Files\Git\bin\bash.exe' }
$fileBin = if ($env:FILE_BIN) { $env:FILE_BIN } else { 'C:\Program Files\Git\usr\bin\file.exe' }
$go = Join-Path $toolchainRoot 'go\bin\go.exe'
$bun = Join-Path $toolchainRoot 'bun\bun.exe'
$buildScript = Join-Path $scriptRoot 'build_release_candidate.sh'
$releasesRoot = [System.IO.Path]::GetFullPath((Join-Path $repoRoot 'releases'))
$releaseRoot = [System.IO.Path]::GetFullPath((Join-Path $releasesRoot $ReleaseId))
$releaseCacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'release-cache'))
$legacyCacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'cache'))

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

Assert-ChildPath -Parent $repoRoot -Child $toolchainRoot
Assert-ChildPath -Parent $releasesRoot -Child $releaseRoot
Assert-ChildPath -Parent $toolchainRoot -Child $releaseCacheRoot
Assert-ChildPath -Parent $toolchainRoot -Child $legacyCacheRoot

foreach ($requiredPath in @($bash, $fileBin, $go, $bun, $buildScript)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Missing required local build tool: $requiredPath"
    }
}

$goVersionLines = & $go version
$goVersionExitCode = $LASTEXITCODE
$goVersionOutput = (($goVersionLines | Select-Object -First 1) | Out-String).Trim()
if ($goVersionExitCode -ne 0 -or -not $goVersionOutput) {
    throw "Failed to determine local Go version from $go"
}
$bunVersionLines = & $bun --version
$bunVersionExitCode = $LASTEXITCODE
$bunVersionOutput = (($bunVersionLines | Select-Object -First 1) | Out-String).Trim()
if ($bunVersionExitCode -ne 0 -or -not $bunVersionOutput) {
    throw "Failed to determine local Bun version from $bun"
}

$goVersionKey = ($goVersionOutput -replace '[^A-Za-z0-9._-]', '_')
$bunVersionKey = ($bunVersionOutput -replace '[^A-Za-z0-9._-]', '_')
if (-not $goVersionKey -or -not $bunVersionKey) {
    throw 'Local tool versions did not produce valid cache keys'
}

$goCacheRoot = Join-Path $releaseCacheRoot "go\$goVersionKey-linux-amd64-cgo0"
$bunCacheRoot = Join-Path $releaseCacheRoot "bun\$bunVersionKey"
$frontendCacheRoot = Join-Path $releaseCacheRoot 'frontend'
foreach ($cachePath in @($goCacheRoot, $bunCacheRoot, $frontendCacheRoot)) {
    Assert-ChildPath -Parent $toolchainRoot -Child $cachePath
}

if ($PurgeBuildCache) {
    foreach ($cachePath in @($releaseCacheRoot, $legacyCacheRoot)) {
        if (Test-Path -LiteralPath $cachePath) {
            Remove-DirectoryWithRetry -Parent $toolchainRoot -Path $cachePath -Description 'local release build cache'
        }
    }
}

if ($KeepCache) {
    Write-Warning '-KeepCache is no longer needed; versioned release build caches persist by default.'
}

New-Item -ItemType Directory -Force -Path $goCacheRoot, $bunCacheRoot, $frontendCacheRoot | Out-Null

$env:GO_BIN = $go.Replace('\', '/')
$env:BUN_BIN = $bun.Replace('\', '/')
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
$env:GOWORK = 'off'
$env:GOMODCACHE = (Join-Path $goCacheRoot 'mod').Replace('\', '/')
$env:GOCACHE = (Join-Path $goCacheRoot 'build').Replace('\', '/')
$env:GOPATH = (Join-Path $goCacheRoot 'path').Replace('\', '/')
$env:BUN_INSTALL_CACHE_DIR = $bunCacheRoot.Replace('\', '/')
$env:FRONTEND_CACHE_ROOT = $frontendCacheRoot.Replace('\', '/')

$escapedBuildScript = $buildScript.Replace("'", "'\''")
$unixBuildScript = (& $bash -lc "cygpath -u '$escapedBuildScript'").Trim()
if (-not $unixBuildScript) {
    throw 'Failed to resolve the release build script for Git Bash'
}

$buildSucceeded = $false
try {
    & $bash $unixBuildScript $ReleaseId $ReleaseTag
    if ($LASTEXITCODE -ne 0) {
        throw "Release build failed with exit code $LASTEXITCODE"
    }

    $candidateBin = Join-Path $releaseRoot 'bin\new-api'
    $manifestPath = Join-Path $releaseRoot 'manifest.env'
    $sourceRoot = Join-Path $releaseRoot 'src'
    if (-not (Test-Path -LiteralPath $candidateBin -PathType Leaf)) {
        throw "Release build did not produce the candidate binary: $candidateBin"
    }
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Release build did not produce the manifest: $manifestPath"
    }
    if (Test-Path -LiteralPath $sourceRoot) {
        throw "Release build left its temporary source worktree behind: $sourceRoot"
    }

    $fileOutput = (& $fileBin $candidateBin | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $fileOutput -notmatch 'ELF 64-bit LSB executable, x86-64') {
        throw "Candidate is not a Linux amd64 ELF binary: $fileOutput"
    }

    $buildSucceeded = $true
    Write-Output $fileOutput
    Write-Output "Local release candidate ready: $releaseRoot"
    Write-Output "After remote acceptance, run: scripts\cleanup_local_release.ps1 -ReleaseId $ReleaseId"
}
finally {
    if (-not $buildSucceeded -and (Test-Path -LiteralPath $releaseRoot)) {
        Remove-DirectoryWithRetry -Parent $releasesRoot -Path $releaseRoot -Description 'failed release candidate directory'
    }
}
