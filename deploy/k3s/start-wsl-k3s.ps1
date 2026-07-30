param(
    [string]$Distro = "Ubuntu-24.04"
)

$keepaliveArgs = @(
    "-d", $Distro,
    "-u", "root",
    "--",
    "flock", "-n", "/run/nav-k3s-keepalive.lock",
    "sleep", "infinity"
)

Start-Process `
    -FilePath "wsl.exe" `
    -ArgumentList $keepaliveArgs `
    -WindowStyle Hidden

Start-Sleep -Seconds 3

$serviceState = & wsl.exe -d $Distro -u root -- systemctl is-active k3s
if ($LASTEXITCODE -ne 0 -or $serviceState.Trim() -ne "active") {
    throw "k3s did not become active in WSL distribution '$Distro'."
}

Write-Host "WSL distribution '$Distro' is kept alive; k3s is active."
