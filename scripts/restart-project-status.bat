@echo off
setlocal
cd /d %~dp0..

set "PROJECT_STATUS_ROOT=C:\Codex\ProjectStatus"
if not exist "%PROJECT_STATUS_ROOT%\go.mod" (
  echo [project-status] repo not found: %PROJECT_STATUS_ROOT%
  exit /b 1
)

echo [project-status] stopping old processes...
taskkill /IM project-status.exe /F >nul 2>&1
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8100 ^| findstr LISTENING') do taskkill /PID %%a /F >nul 2>&1

echo [project-status] building...
pushd "%PROJECT_STATUS_ROOT%"
go build -o "%~dp0..\data\project-status.exe" .\cmd\project-status
set "BUILD_ERR=%ERRORLEVEL%"
popd
if not "%BUILD_ERR%"=="0" (
  echo [project-status] build failed
  exit /b 1
)

if /I "%~1"=="foreground" (
  call scripts\run-project-status-exec.bat
  exit /b %ERRORLEVEL%
)

echo [project-status] starting in background...
start "ProjectStatus" /MIN cmd /c "call %~dp0run-project-status-exec.bat"
ping 127.0.0.1 -n 4 >nul
tasklist /FI "IMAGENAME eq project-status.exe" 2>nul | find /I "project-status.exe" >nul
if errorlevel 1 (
  echo [project-status] failed to start
  exit /b 1
)
echo [project-status] started on :8100
exit /b 0
