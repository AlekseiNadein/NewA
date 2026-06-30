@echo off
setlocal
cd /d %~dp0..

if exist tools\nginx\nginx.exe (
  echo [nginx] already installed at tools\nginx\nginx.exe
  exit /b 0
)

echo [nginx] downloading portable nginx for Windows...
if not exist tools mkdir tools
if not exist tools\nginx mkdir tools\nginx

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$zip = Join-Path $env:TEMP 'nginx-win.zip';" ^
  "$url = 'https://nginx.org/download/nginx-1.26.3.zip';" ^
  "Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing;" ^
  "$extract = Join-Path $env:TEMP 'nginx-win-extract';" ^
  "if (Test-Path $extract) { Remove-Item $extract -Recurse -Force };" ^
  "Expand-Archive -Path $zip -DestinationPath $extract -Force;" ^
  "$src = Get-ChildItem -Path $extract -Directory | Select-Object -First 1;" ^
  "Copy-Item -Path (Join-Path $src.FullName '*') -Destination 'tools\nginx' -Recurse -Force;" ^
  "Remove-Item $zip -Force; Remove-Item $extract -Recurse -Force"

if not exist tools\nginx\nginx.exe (
  echo [nginx] install failed — tools\nginx\nginx.exe not found
  exit /b 1
)

copy /Y tools\nginx\conf\mime.types deploy\mime.types >nul 2>&1

echo [nginx] installed to tools\nginx\nginx.exe
exit /b 0
