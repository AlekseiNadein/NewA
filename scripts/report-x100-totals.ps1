param(
    [string]$BaseUrl = $(if ($env:K6_BASE_URL) { $env:K6_BASE_URL } else { "http://localhost:8080" }),
    [string]$CompanyName = $(if ($env:K6_COMPANY_NAME) { $env:K6_COMPANY_NAME } else { "Система" }),
    [string]$UserName = $(if ($env:K6_USER_NAME) { $env:K6_USER_NAME } else { "nadein.av@yandex.ru" }),
    [string]$Password = $(if ($env:K6_PASSWORD) { $env:K6_PASSWORD } else { "admin123" }),
    [string]$Filter = "X100",
    [switch]$Json
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)

function Format-RubleTotal {
    param([double]$Value)
    if ($Value -ge 1e9) { return ("{0:N1} млрд" -f ($Value / 1e9)) }
    if ($Value -ge 1e6) { return ("{0:N1} млн" -f ($Value / 1e6)) }
    if ($Value -gt 0) { return ("{0:N2}" -f $Value) }
    return "0"
}

function Get-CalcSummary {
    param(
        [Microsoft.PowerShell.Commands.WebRequestSession]$Session,
        [string]$EstimateId
    )
    return Invoke-RestMethod -Uri "$BaseUrl/api/estimates/$EstimateId/calc-status?summary=1" -WebSession $Session
}

$loginBody = @{
    companyName = $CompanyName
    name        = $UserName
    password    = $Password
} | ConvertTo-Json

$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$null = Invoke-RestMethod -Uri "$BaseUrl/api/auth/login" -Method Post -ContentType "application/json" -Body $loginBody -WebSession $session

$list = Invoke-RestMethod -Uri "$BaseUrl/api/estimates?summary=1" -WebSession $session
$estimates = @(
    $list |
        Where-Object { $_.code -and $_.code.ToUpper().Contains($Filter.ToUpper()) } |
        Sort-Object code
)

if ($estimates.Count -eq 0) {
    Write-Error "No estimates matched filter '$Filter'."
}

$rows = foreach ($estimate in $estimates) {
    $summary = Get-CalcSummary -Session $session -EstimateId $estimate.id
    $linesTotal = [int]$summary.total
    $linesDone = [int]$summary.processed
    $errors = [int]$summary.errors
    $grandTotal = [double]$summary.grandTotal
    $pending = [Math]::Max(0, $linesTotal - $linesDone - $errors)
    [PSCustomObject]@{
        Code           = $estimate.code
        EstimateId     = $estimate.id
        LinesDone      = $linesDone
        LinesTotal     = $linesTotal
        Errors         = $errors
        Pending        = $pending
        GrandTotal     = $grandTotal
        GrandTotalText = Format-RubleTotal $grandTotal
        HeaderTotal    = [double]$estimate.total
        CalcComplete   = ($linesTotal -gt 0 -and $pending -eq 0)
    }
}

if ($Json) {
    $rows | ConvertTo-Json -Depth 4
    exit 0
}

Write-Host ""
Write-Host "X100 totals report (source: calc-status.grandTotal, same as UI)"
Write-Host "API: $BaseUrl"
Write-Host "Filter: *$Filter*"
Write-Host ""
Write-Host ("{0,-18} {1,12} {2,7} {3,16} {4,12}" -f "Code", "Lines", "Errors", "Grand total", "Header total")
Write-Host ("{0,-18} {1,12} {2,7} {3,16} {4,12}" -f "----", "-----", "------", "-----------", "------------")

foreach ($row in $rows) {
    $lines = if ($row.LinesTotal -gt 0) { "{0}/{1}" -f $row.LinesDone, $row.LinesTotal } else { "0/0" }
    Write-Host ("{0,-18} {1,12} {2,7} {3,16} {4,12}" -f `
            $row.Code, `
            $lines, `
            $row.Errors, `
            $row.GrandTotalText, `
            ("{0:N2}" -f $row.HeaderTotal))
}

Write-Host ""
Write-Host "Note: Header total (app_estimates.total) is legacy and may stay 0; use Grand total for reports."
Write-Host ""
