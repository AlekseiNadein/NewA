param(
    [int]$Vus = 10,
    [string]$LogLabel = '',
    [string]$MaxDuration = '45m',
    [int]$CalcTimeoutSec = 2700,
    [switch]$SkipPreClear,
    [switch]$SkipPostClear
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

if (-not $SkipPreClear) {
    & "$PSScriptRoot\clear-x100-calc.ps1" -Count $Vus -Mode cancel
    if (-not $?) {
        exit 1
    }
}

$stamp = if ($LogLabel) { $LogLabel } else { "x100-calc-wait-${Vus}vu-$(Get-Date -Format 'yyyy-MM-dd_HH-mm')" }
$logPath = Join-Path $repoRoot "deploy\k6\results-$stamp.log"

$wallStart = Get-Date
Write-Host "WALL_START=$($wallStart.ToString('o'))"

Remove-Item Env:K6_METRICS -ErrorAction SilentlyContinue
$env:K6_VUS = "$Vus"
$env:K6_MAX_DURATION = $MaxDuration
$env:K6_CALC_TIMEOUT_SEC = "$CalcTimeoutSec"
$env:K6_FORCE_RECALC = '1'

$k6Exit = 0
$prevEap = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try {
    & cmd /c "`"$repoRoot\scripts\run-k6.bat`" scenarios\x100-editor-calc-wait.js" 2>&1 | Tee-Object -FilePath $logPath
    $k6Exit = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $prevEap
}

$wallEnd = Get-Date
Write-Host "WALL_END=$($wallEnd.ToString('o'))"
Write-Host "WALL_MS=$([int]($wallEnd - $wallStart).TotalMilliseconds)"
Write-Host "LOG=$logPath"

if (-not $SkipPostClear) {
    Write-Host ''
    & "$PSScriptRoot\clear-x100-calc.ps1" -Count $Vus -Mode close
    if (-not $?) {
        if ($k6Exit -eq 0) {
            exit 1
        }
    }
}

exit $k6Exit
