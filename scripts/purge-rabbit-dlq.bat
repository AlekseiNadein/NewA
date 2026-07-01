@echo off
setlocal
cd /d %~dp0..

set APP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
set APP_RABBITMQ_DLQ=estimate.calc.dlq

echo [rabbit] purging DLQ (poison messages are discarded, not replayed)...
go run .\backend\cmd\purge_dlq
if errorlevel 1 exit /b 1

echo.
echo [rabbit] healthz after purge:
powershell -NoProfile -Command "try { (Invoke-RestMethod 'http://localhost:8080/api/healthz') | ConvertTo-Json -Depth 6 } catch { Write-Host $_.Exception.Message; exit 1 }"
