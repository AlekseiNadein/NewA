@echo off
setlocal

cd /d %~dp0..
set "K6_VUS=%~1"
if "%K6_VUS%"=="" set "K6_VUS=5"

set "K6_METRICS=prometheus"
echo [k6] load-estimates VUs=%K6_VUS%
call scripts\run-k6.bat scenarios\load-estimates.js
exit /b %ERRORLEVEL%
