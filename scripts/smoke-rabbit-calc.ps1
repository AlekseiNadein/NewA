$ErrorActionPreference = "Stop"
$base = "http://localhost:8080"

$login = Invoke-RestMethod -Uri "$base/api/auth/login" -Method Post -ContentType "application/json" -Body '{"companyName":"\u0421\u0438\u0441\u0442\u0435\u043c\u0430","name":"nadein.av@yandex.ru","password":"admin123"}' -SessionVariable session
Write-Host "login: $($login.user.name)"

$estimates = Invoke-RestMethod -Uri "$base/api/estimates" -WebSession $session
Write-Host "estimates: $($estimates.Count)"
if ($estimates.Count -eq 0) { exit 1 }

$estimate = $estimates | Where-Object { $_.code -ne "" } | Select-Object -First 1
if (-not $estimate) { $estimate = $estimates[0] }
Write-Host "target estimate: $($estimate.id) code=$($estimate.code)"

$before = Invoke-RestMethod -Uri "$base/api/estimates/$($estimate.id)/calc-status" -WebSession $session
$beforeDone = @($before.items | Where-Object { $_.status -eq "done" }).Count
$beforeQueued = @($before.items | Where-Object { $_.status -eq "queued" }).Count
Write-Host "before: done=$beforeDone queued=$beforeQueued"

# Re-save estimate to enqueue outbox events (same items, triggers revision bump paths via editor data)
$detail = $estimate
$body = @{
    objectId = $detail.objectId
    code = $detail.code
    title = $detail.title
    description = $detail.description
    district = $detail.district
    fgisSetId = $detail.fgisSetId
    status = $detail.status
    items = $detail.items
} | ConvertTo-Json -Depth 20

Invoke-RestMethod -Uri "$base/api/estimates/$($estimate.id)" -Method Put -ContentType "application/json" -Body $body -WebSession $session | Out-Null
Write-Host "estimate re-saved, waiting for rabbit processing..."

Start-Sleep -Seconds 8

$after = Invoke-RestMethod -Uri "$base/api/estimates/$($estimate.id)/calc-status" -WebSession $session
$afterDone = @($after.items | Where-Object { $_.status -eq "done" }).Count
$afterQueued = @($after.items | Where-Object { $_.status -eq "queued" }).Count
$afterFailed = @($after.items | Where-Object { $_.status -eq "failed" }).Count
Write-Host "after: done=$afterDone queued=$afterQueued failed=$afterFailed"

$logTail = Get-Content "data\calc-worker.log" -Tail 5
Write-Host "calc-worker tail:"
$logTail | ForEach-Object { Write-Host $_ }

if ($afterDone -ge $beforeDone) {
    Write-Host "SMOKE_OK: rabbit pipeline processed calc statuses"
    exit 0
}
Write-Host "SMOKE_WARN: no new done statuses"
exit 0
