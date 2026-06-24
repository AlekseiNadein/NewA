param(
    [string]$Label = (Get-Date -Format "yyyy-MM-dd_HH-mm")
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$dest = Join-Path $root "backups\$Label"

function Copy-ProjectPath {
    param(
        [string]$RelativePath
    )

    $source = Join-Path $root $RelativePath
    if (-not (Test-Path $source)) {
        return
    }

    $target = Join-Path $dest $RelativePath
    if (Test-Path $source -PathType Container) {
        New-Item -ItemType Directory -Path $target -Force | Out-Null
        Copy-Item -Path (Join-Path $source "*") -Destination $target -Recurse -Force
        return
    }

    $parent = Split-Path $target -Parent
    if ($parent -and -not (Test-Path $parent)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    Copy-Item -Path $source -Destination $target -Force
}

New-Item -ItemType Directory -Path $dest -Force | Out-Null

foreach ($path in @("backend", "web", "db", "go.mod", "run.bat")) {
    Copy-ProjectPath -RelativePath $path
}

Write-Output "Backup saved to backups\$Label"
