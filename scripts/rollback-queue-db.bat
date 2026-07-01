@echo off
setlocal
cd /d %~dp0..

echo [queue] rollback to legacy DB worker mode...
echo Update APP_QUEUE_MODE=db in run.bat and scripts\run-calc-worker-exec.bat, then restart.

powershell -NoProfile -Command ^
  "(Get-Content run.bat) -replace 'APP_QUEUE_MODE=rabbit','APP_QUEUE_MODE=db' -replace 'APP_QUEUE_MODE=dual','APP_QUEUE_MODE=db' | Set-Content run.bat; " ^
  "(Get-Content scripts\run-calc-worker-exec.bat) -replace 'APP_QUEUE_MODE=rabbit','APP_QUEUE_MODE=db' -replace 'APP_QUEUE_MODE=dual','APP_QUEUE_MODE=db' | Set-Content scripts\run-calc-worker-exec.bat"

call run.bat
exit /b %ERRORLEVEL%
