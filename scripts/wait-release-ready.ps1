$ErrorActionPreference = "Stop"
for ($i = 1; $i -le 24; $i++) {
  $h = Invoke-RestMethod "http://localhost:8080/api/healthz"
  $dlq = $h.queue.depths."estimate.calc.dlq"
  Write-Host "t=$i outbox=$($h.queue.outboxPending) dlq=$dlq pipeline=$($h.pipelineReady) release=$($h.releaseReady)"
  if ($h.releaseReady) { exit 0 }
  Start-Sleep -Seconds 5
}
exit 1
