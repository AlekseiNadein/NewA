@echo off
setlocal
cd /d %~dp0..

call scripts\setup-nginx.bat
if errorlevel 1 exit /b 1

set NGINX_EXE=%CD%\tools\nginx\nginx.exe
set NGINX_PREFIX=%CD%\deploy

if not exist "%NGINX_PREFIX%\logs" mkdir "%NGINX_PREFIX%\logs"
if not exist "%NGINX_PREFIX%\temp" mkdir "%NGINX_PREFIX%\temp"

echo [nginx] stopping old processes on :8080...
"%NGINX_EXE%" -p "%NGINX_PREFIX%" -c nginx.conf -s stop >nul 2>&1
taskkill /IM nginx.exe /F >nul 2>&1
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do taskkill /PID %%a /F >nul 2>&1

echo [nginx] starting on :8080...
start "NAV Nginx" /MIN "%NGINX_EXE%" -p "%NGINX_PREFIX%" -c nginx.conf
ping 127.0.0.1 -n 3 >nul

netstat -ano | findstr :8080 | findstr LISTENING >nul
if errorlevel 1 (
  echo [nginx] failed to start — check deploy\logs\error.log
  exit /b 1
)

echo [nginx] started on :8080
exit /b 0
