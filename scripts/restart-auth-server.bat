@echo off
setlocal
cd /d %~dp0..

echo [auth] stopping old processes...
taskkill /IM nav-auth-server.exe /F >nul 2>&1
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8081 ^| findstr LISTENING') do taskkill /PID %%a /F >nul 2>&1

echo [auth] building...
go build -o data\nav-auth-server.exe .\backend\cmd\auth_server
if errorlevel 1 (
  echo [auth] build failed
  exit /b 1
)

if /I "%~1"=="foreground" (
  call scripts\run-auth-server-exec.bat
  exit /b %ERRORLEVEL%
)

echo [auth] starting in background...
start "NAV Auth Server" /MIN cmd /c "call %~dp0run-auth-server-exec.bat"
ping 127.0.0.1 -n 4 >nul
tasklist /FI "IMAGENAME eq nav-auth-server.exe" 2>nul | find /I "nav-auth-server.exe" >nul
if errorlevel 1 (
  echo [auth] failed to start
  exit /b 1
)
echo [auth] started on :8081
exit /b 0
