$ErrorActionPreference = "Stop"
$base = "http://localhost:8080"
$iterations = 5
if ($args.Count -gt 0) { $iterations = [int]$args[0] }

$before = Invoke-RestMethod -Uri "$base/api/healthz"
$beforeProcessed = $before.queue.consumer.processed
$beforePublished = $before.queue.publisher.published

$login = Invoke-RestMethod -Uri "$base/api/auth/login" -Method Post -ContentType "application/json" -Body '{"companyName":"\u0421\u0438\u0441\u0442\u0435\u043c\u0430","name":"nadein.av@yandex.ru","password":"admin123"}' -SessionVariable session
$estimates = Invoke-RestMethod -Uri "$base/api/estimates" -WebSession $session
$estimate = $estimates | Select-Object -First 1
if (-not $estimate) { throw "no estimates found" }

$body = @{
    objectId = $estimate.objectId
    code = $estimate.code
    title = $estimate.title
    description = $estimate.description
    district = $estimate.district
    fgisSetId = $estimate.fgisSetId
    status = $estimate.status
    items = $estimate.items
} | ConvertTo-Json -Depth 20

$sw = [System.Diagnostics.Stopwatch]::StartNew()
for ($i = 1; $i -le $iterations; $i++) {
    Invoke-RestMethod -Uri "$base/api/estimates/$($estimate.id)" -Method Put -ContentType "application/json" -Body $body -WebSession $session | Out-Null
}
$sw.Stop()

Start-Sleep -Seconds 10
$after = Invoke-RestMethod -Uri "$base/api/healthz"
$processedDelta = $after.queue.consumer.processed - $beforeProcessed
$publishedDelta = $after.queue.publisher.published - $beforePublished

Write-Host "load iterations=$iterations elapsedMs=$($sw.ElapsedMilliseconds)"
Write-Host "publisher delta=$publishedDelta consumer processed delta=$processedDelta duplicates=$($after.queue.consumer.duplicates)"
Write-Host "pipelineReady=$($after.pipelineReady) releaseReady=$($after.releaseReady) status=$($after.status)"

if (-not $after.pipelineReady) { exit 1 }
exit 0
