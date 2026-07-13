@echo off
if "%~1"=="" (
  echo Usage: parse-k6-x100-results.bat ^<k6-log-file^>
  exit /b 1
)
cd /d %~dp0..
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0parse-k6-x100-results.ps1" %*
exit /b %ERRORLEVEL%
