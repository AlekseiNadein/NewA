@echo off
setlocal
cd /d %~dp0..

echo [calc-worker] stopping old processes...
taskkill /IM nav-calc-worker.exe /F >nul 2>&1
taskkill /IM nav-calc-worker-new.exe /F >nul 2>&1

echo [calc-worker] building...
go build -o data\nav-calc-worker.exe .\backend\cmd\calc_worker
if errorlevel 1 (
  echo [calc-worker] build failed
  exit /b 1
)

if /I "%~1"=="foreground" (
  call scripts\run-calc-worker-exec.bat
  exit /b %ERRORLEVEL%
)

echo [calc-worker] starting in background...
start "NAV Calc Worker" /MIN cmd /c "call %~dp0run-calc-worker-exec.bat"
ping 127.0.0.1 -n 4 >nul
tasklist /FI "IMAGENAME eq nav-calc-worker.exe" 2>nul | find /I "nav-calc-worker.exe" >nul
if errorlevel 1 (
  echo [calc-worker] failed to start
  exit /b 1
)
echo [calc-worker] started
exit /b 0
