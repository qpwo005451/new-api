[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9._-]+$')]
    [string]$ReleaseId,

    [string]$ReleaseTag = 'HEAD',

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
$bash = if ($env:GIT_BASH) { $env:GIT_BASH } else { 'C:\Program Files\Git\bin\bash.exe' }
$fileBin = if ($env:FILE_BIN) { $env:FILE_BIN } else { 'C:\Program Files\Git\usr\bin\file.exe' }
$go = Join-Path $toolchainRoot 'go\bin\go.exe'
$bun = Join-Path $toolchainRoot 'bun\bun.exe'
$buildScript = Join-Path $scriptRoot 'build_release_candidate.sh'
$releasesRoot = [System.IO.Path]::GetFullPath((Join-Path $repoRoot 'releases'))
$releaseRoot = [System.IO.Path]::GetFullPath((Join-Path $releasesRoot $ReleaseId))
$cacheRoot = [System.IO.Path]::GetFullPath((Join-Path $toolchainRoot 'cache'))

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

Assert-ChildPath -Parent $repoRoot -Child $toolchainRoot
Assert-ChildPath -Parent $releasesRoot -Child $releaseRoot
Assert-ChildPath -Parent $toolchainRoot -Child $cacheRoot

foreach ($requiredPath in @($bash, $fileBin, $go, $bun, $buildScript)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Missing required local build tool: $requiredPath"
    }
}

New-Item -ItemType Directory -Force -Path $cacheRoot | Out-Null

$env:GO_BIN = $go.Replace('\', '/')
$env:BUN_BIN = $bun.Replace('\', '/')
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
$env:GOMODCACHE = (Join-Path $cacheRoot 'go-mod').Replace('\', '/')
$env:GOCACHE = (Join-Path $cacheRoot 'go-build').Replace('\', '/')
$env:GOPATH = (Join-Path $cacheRoot 'go-path').Replace('\', '/')
$env:BUN_INSTALL_CACHE_DIR = (Join-Path $cacheRoot 'bun').Replace('\', '/')

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
        Remove-Item -LiteralPath $releaseRoot -Recurse -Force
    }
    if (-not $KeepCache -and (Test-Path -LiteralPath $cacheRoot)) {
        Remove-Item -LiteralPath $cacheRoot -Recurse -Force
    }
}
