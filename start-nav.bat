@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "DISTRO=Ubuntu-24.04"
set "PROJECT_ROOT=%~dp0"
set "DOCKER_DESKTOP=%ProgramFiles%\Docker\Docker\Docker Desktop.exe"

echo [1/5] Checking Docker Desktop...
docker info >nul 2>&1
if not errorlevel 1 goto docker_ready

if not exist "%DOCKER_DESKTOP%" (
    echo ERROR: Docker Desktop is not running and was not found at:
    echo        %DOCKER_DESKTOP%
    goto failed
)

echo Starting Docker Desktop...
start "" "%DOCKER_DESKTOP%"

for /L %%I in (1,1,60) do (
    docker info >nul 2>&1
    if not errorlevel 1 goto docker_ready
    timeout /t 2 /nobreak >nul
)

echo ERROR: Docker Desktop did not become ready within 120 seconds.
goto failed

:docker_ready
echo [2/5] Starting WSL keepalive and k3s...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PROJECT_ROOT%deploy\k3s\start-wsl-k3s.ps1"
if errorlevel 1 goto failed

echo [3/5] Waiting for the k3s node...
wsl.exe -d %DISTRO% -u root -- k3s kubectl wait node/nadein-envyi5 --for=condition=Ready --timeout=180s
if errorlevel 1 goto failed
wsl.exe -d %DISTRO% -u root -- k3s kubectl get node nadein-envyi5 --no-headers | findstr /C:" Ready " >nul
if errorlevel 1 (
    echo ERROR: The k3s node is not Ready.
    goto failed
)

echo [4/5] Waiting for NAV pods...
wsl.exe -d %DISTRO% -u root -- k3s kubectl -n nav wait --for=condition=Ready pod --all --timeout=240s
if errorlevel 1 goto failed

set "K3S_IP="
for /f "tokens=1" %%I in ('wsl.exe -d %DISTRO% -u root -- hostname -I') do set "K3S_IP=%%I"
if not defined K3S_IP (
    echo ERROR: Could not determine the WSL k3s node IP.
    goto failed
)

echo [5/5] Refreshing the Windows proxy...
docker container inspect nav-k3s-proxy >nul 2>&1
if not errorlevel 1 docker rm --force nav-k3s-proxy >nul

docker run --detach ^
    --name nav-k3s-proxy ^
    --restart unless-stopped ^
    --publish 127.0.0.1:8088:80 ^
    --env NAV_K3S_IP=!K3S_IP! ^
    --volume "%PROJECT_ROOT%deploy\k3s\windows-proxy.conf.template:/etc/nginx/templates/default.conf.template:ro" ^
    nginx:latest >nul
if errorlevel 1 goto failed

echo Waiting for the NAV health endpoint...
for /L %%I in (1,1,60) do (
    set "HEALTH_CODE="
    for /f "delims=" %%H in ('wsl.exe -d %DISTRO% -u root -- curl -sS -o /dev/null -w "%%{http_code}" --connect-timeout 3 --max-time 10 -H "Host: nav.local" http://127.0.0.1/api/healthz 2^>nul') do set "HEALTH_CODE=%%H"
    if "!HEALTH_CODE!"=="200" goto nav_ready
    timeout /t 2 /nobreak >nul
)

echo ERROR: NAV health check did not pass within 120 seconds.
goto failed

:nav_ready
echo.
echo NAV is ready: http://127.0.0.1:8088
exit /b 0

:failed
echo.
echo Startup failed. Review the error above.
pause
exit /b 1
