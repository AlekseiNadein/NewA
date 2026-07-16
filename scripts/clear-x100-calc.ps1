param(
    [int]$Count = 10,
    [ValidateSet('cancel', 'close')]
    [string]$Mode = 'cancel',
    [string]$BaseUrl = $(if ($env:K6_BASE_URL) { $env:K6_BASE_URL } else { 'http://localhost:8080' }),
    [string]$CompanyName = $env:K6_COMPANY_NAME,
    [string]$UserName = $(if ($env:K6_USER_NAME) { $env:K6_USER_NAME } else { 'nadein.av@yandex.ru' }),
    [string]$Password = $(if ($env:K6_PASSWORD) { $env:K6_PASSWORD } else { 'admin123' })
)

$ErrorActionPreference = 'Stop'

if (-not $CompanyName) {
    $CompanyName = -join ([char[]](0x0421, 0x0438, 0x0441, 0x0442, 0x0435, 0x043C, 0x0430))
}

function Get-X100Estimates {
    param(
        [Microsoft.PowerShell.Commands.WebRequestSession]$Session,
        [int]$Take
    )

    $list = Invoke-RestMethod -Uri "$BaseUrl/api/estimates?summary=1" -WebSession $Session
    $items = @($list | Where-Object { $_.code -match 'X100' } | Sort-Object code | Select-Object -First $Take)
    if ($items.Count -lt $Take) {
        throw "Need $Take X100 estimates, found $($items.Count)"
    }
    return $items
}

function Clear-EstimateCalcOnClose {
    param(
        [Microsoft.PowerShell.Commands.WebRequestSession]$Session,
        $Estimate
    )

    try {
        $lockRes = Invoke-WebRequest -Uri "$BaseUrl/api/estimates/$($Estimate.id)/lock" -Method Delete -WebSession $Session -UseBasicParsing
        if ($lockRes.StatusCode -eq 204) {
            Write-Host "$($Estimate.code) lock released"
        }
    } catch {
        $status = $null
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
        }
        if ($status -ne 404 -and $status -ne 409) {
            throw
        }
    }

    Start-Sleep -Milliseconds 500

    $null = Invoke-WebRequest -Uri "$BaseUrl/api/estimates/$($Estimate.id)/calc/cancel" -Method Post -WebSession $Session -UseBasicParsing
    Write-Host "$($Estimate.code) calc cleared"
}

function Clear-EstimateCalcCancel {
    param(
        [Microsoft.PowerShell.Commands.WebRequestSession]$Session,
        $Estimate
    )

    $null = Invoke-WebRequest -Uri "$BaseUrl/api/estimates/$($Estimate.id)/calc/cancel" -Method Post -WebSession $Session -UseBasicParsing
    Write-Host "$($Estimate.code) cancelled"
}

$loginBody = @{ companyName = $CompanyName; name = $UserName; password = $Password } | ConvertTo-Json
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$null = Invoke-RestMethod -Uri "$BaseUrl/api/auth/login" -Method Post -Body $loginBody -ContentType 'application/json' -WebSession $session

$x100 = Get-X100Estimates -Session $session -Take $Count

if ($Mode -eq 'close') {
    Write-Host "=== Close-like cleanup for $($x100.Count) X100 estimates ==="
    foreach ($e in $x100) {
        Clear-EstimateCalcOnClose -Session $session -Estimate $e
    }
} else {
    Write-Host "=== Cancelling $($x100.Count) X100 estimates ==="
    foreach ($e in $x100) {
        Clear-EstimateCalcCancel -Session $session -Estimate $e
    }
}

Start-Sleep -Seconds 2

Write-Host ''
Write-Host '=== Verify cleared ==='
$allClear = $true
foreach ($e in $x100) {
    $st = Invoke-RestMethod -Uri "$BaseUrl/api/estimates/$($e.id)/calc-status?summary=1" -WebSession $session
    Write-Host "$($e.code) processed=$($st.processed)/$($st.total) grandTotal=$($st.grandTotal) errors=$($st.errors)"
    if ($st.processed -ne 0 -or $st.grandTotal -ne 0) {
        $allClear = $false
    }
}

$health = Invoke-RestMethod -Uri "$BaseUrl/api/healthz"
Write-Host ''
Write-Host "All clear: $allClear | pipelineReady=$($health.pipelineReady) outbox=$($health.queue.outboxPending)"

if (-not $allClear) {
    exit 1
}
