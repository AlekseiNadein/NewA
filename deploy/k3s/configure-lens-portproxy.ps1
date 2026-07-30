param(
    [string]$Distro = "Ubuntu-24.04"
)

$ErrorActionPreference = "Stop"

$wslIp = (wsl.exe -d $Distro -- hostname -I).Trim().Split(" ")[0]
if (-not $wslIp) {
    throw "Could not determine the WSL IP address."
}

netsh interface portproxy delete v4tov4 `
    listenaddress=127.0.0.1 `
    listenport=6443 2>$null | Out-Null

netsh interface portproxy add v4tov4 `
    listenaddress=127.0.0.1 `
    listenport=6443 `
    connectaddress=$wslIp `
    connectport=6443 | Out-Null

if ($LASTEXITCODE -ne 0) {
    throw "Failed to configure the Windows port proxy."
}
