@echo off
setlocal
cd /d %~dp0..

set APP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
set APP_RABBITMQ_EXCHANGE=estimate.calc

echo [rabbit] queue depths via API healthz...
powershell -NoProfile -Command "try { (Invoke-RestMethod 'http://localhost:8080/api/healthz').queue | ConvertTo-Json -Depth 6 } catch { Write-Host $_.Exception.Message; exit 1 }"
if errorlevel 1 exit /b 1

echo.
echo [rabbit] docker queue list (if container is running)...
for /f "tokens=*" %%i in ('docker ps --format "{{.Names}}" 2^>nul ^| findstr /i rabbit') do (
  docker exec %%i rabbitmqctl list_queues name messages consumers
  goto :done
)
echo no rabbit container name matched; skipped docker list
:done
