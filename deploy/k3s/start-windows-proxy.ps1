# Starts Docker Desktop nginx proxy from Windows to k3s Traefik.
# Usage:
#   .\deploy\k3s\start-windows-proxy.ps1
#   .\deploy\k3s\start-windows-proxy.ps1 -IngressHost newa-staging.local
param(
  [string]$IngressHost = $(if ($env:NAV_INGRESS_HOST) { $env:NAV_INGRESS_HOST } else { "nav.local" }),
  [string]$ProxyName = $(if ($env:PROXY_NAME) { $env:PROXY_NAME } else { "nav-k3s-proxy" }),
  [int]$WindowsPort = $(if ($env:WINDOWS_PORT) { [int]$env:WINDOWS_PORT } else { 8088 }),
  [string]$WslDistro = "Ubuntu-24.04",
  [string]$WslUser = "alexey"
)

$ErrorActionPreference = "Stop"
$RootDir = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$Template = Join-Path $RootDir "deploy/k3s/windows-proxy.conf.template"

$ipScript = @'
kubectl get nodes -o jsonpath="{.items[0].status.addresses[?(@.type==\"InternalIP\")].address}"
'@
$ipScriptPath = Join-Path $env:TEMP "newa-k3s-ip.sh"
Set-Content -Path $ipScriptPath -Value $ipScript -Encoding ascii
$wslIpScript = (wsl.exe wslpath -a ($ipScriptPath -replace '\\', '/')).Trim()
$k3sIp = (wsl.exe -d $WslDistro -u $WslUser -- bash $wslIpScript).Trim()
if (-not $k3sIp) {
  throw "Could not determine the k3s node InternalIP."
}

docker rm --force $ProxyName 2>$null | Out-Null

$templateMount = ($Template -replace '\\', '/')
docker run --detach `
  --name $ProxyName `
  --restart unless-stopped `
  --publish "127.0.0.1:${WindowsPort}:80" `
  --env "NAV_K3S_IP=$k3sIp" `
  --env "NAV_INGRESS_HOST=$IngressHost" `
  --volume "${templateMount}:/etc/nginx/templates/default.conf.template:ro" `
  nginx:latest | Out-Null

Write-Host "NAV proxy is listening on http://127.0.0.1:${WindowsPort} (Host: $IngressHost, k3s node $k3sIp)."
