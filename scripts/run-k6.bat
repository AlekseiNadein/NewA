@echo off
setlocal enabledelayedexpansion

cd /d %~dp0..
set "K6_DIR=%CD%\deploy\k6"

set "REL=scenarios\smoke.js"
if not "%~1"=="" set "REL=%~1"
set "REL=%REL:/=\%"

set "K6_BASE_URL=%K6_BASE_URL%"
if "%K6_BASE_URL%"=="" set "K6_BASE_URL=http://host.docker.internal:8080"

set "SCRIPT=/scripts/%REL:\=/%"
set "OUT_ARGS="

if /i "%K6_METRICS%"=="prometheus" (
  if "!K6_PROMETHEUS_RW_SERVER_URL!"=="" set "K6_PROMETHEUS_RW_SERVER_URL=http://host.docker.internal:9093/api/v1/write"
  set "OUT_ARGS=--out experimental-prometheus-rw"
)

echo [k6] script:   %SCRIPT%
echo [k6] base URL: %K6_BASE_URL%
if /i "%K6_METRICS%"=="prometheus" echo [k6] metrics:  !K6_PROMETHEUS_RW_SERVER_URL!

docker run --rm ^
  -e K6_BASE_URL=%K6_BASE_URL% ^
  -e K6_COMPANY_NAME=%K6_COMPANY_NAME% ^
  -e K6_USER_NAME=%K6_USER_NAME% ^
  -e K6_PASSWORD=%K6_PASSWORD% ^
  -e K6_VUS=%K6_VUS% ^
  -e K6_DURATION=%K6_DURATION% ^
  -e K6_SLEEP=%K6_SLEEP% ^
  -e K6_PROMETHEUS_RW_SERVER_URL=!K6_PROMETHEUS_RW_SERVER_URL! ^
  -v "%K6_DIR%:/scripts" ^
  grafana/k6:0.57.0 run !OUT_ARGS! "%SCRIPT%"

exit /b %ERRORLEVEL%
