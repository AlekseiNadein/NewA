param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$LogPath,
    [switch]$Json
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)

if (-not (Test-Path -LiteralPath $LogPath)) {
    Write-Error "Log file not found: $LogPath"
}

function Format-RubleTotal {
    param([double]$Value)
    if ($Value -ge 1e9) { return ("{0:N1} млрд" -f ($Value / 1e9)) }
    if ($Value -ge 1e6) { return ("{0:N1} млн" -f ($Value / 1e6)) }
    if ($Value -gt 0) { return ("{0:N2}" -f $Value) }
    return "0"
}

function Parse-FlatResultLine {
    param([string]$Line)
    if ($Line -notmatch "X100_RESULT\b") { return $null }
    $fields = @{}
    foreach ($token in [regex]::Matches($Line, "(\w+)=([^\s""]+)")) {
        $fields[$token.Groups[1].Value] = $token.Groups[2].Value.Trim('"')
    }
    if (-not $fields.ContainsKey("code")) { return $null }
    $linesParts = ($fields["lines"] -split "/")
    return [PSCustomObject]@{
        VU         = [int]($fields["vu"])
        Code       = $fields["code"]
        EstimateId = $fields["id"]
        LinesDone  = if ($linesParts.Count -ge 1) { [int]$linesParts[0] } else { 0 }
        LinesTotal = if ($linesParts.Count -ge 2) { [int]$linesParts[1] } else { 0 }
        Errors     = [int]($fields["errors"])
        GrandTotal = [double]($fields["grandTotal"])
        CalcWaitMs = [int]($fields["calcWaitMs"])
        DurationMs = [int]($fields["durationMs"])
        Done       = ($fields["done"] -eq "true")
    }
}

function Parse-JsonResultBlob {
    param([string]$Blob)
    if ($Blob -notmatch '"type"\s*:\s*"x100_result"') { return $null }
    try {
        $obj = $Blob | ConvertFrom-Json
    } catch {
        return $null
    }
    if ($obj.type -ne "x100_result") { return $null }
    return [PSCustomObject]@{
        VU         = [int]$obj.vu
        Code       = [string]$obj.estimateCode
        EstimateId = [string]$obj.estimateId
        LinesDone  = [int]$obj.linesProcessed
        LinesTotal = [int]$obj.linesTotal
        Errors     = [int]$obj.linesErrors
        GrandTotal = [double]$obj.grandTotal
        CalcWaitMs = [int]$obj.calcWaitMs
        DurationMs = [int]$obj.durationMs
        Done       = [bool]$obj.calcDone
    }
}

$raw = Get-Content -LiteralPath $LogPath -Raw
$results = @{}

foreach ($match in [regex]::Matches($raw, "X100_RESULT[^\r\n]+")) {
    $parsed = Parse-FlatResultLine $match.Value
    if ($parsed) { $results[$parsed.VU] = $parsed }
}

# Legacy k6 JSON logs: remove line breaks inside msg payload, then unescape quotes.
$flat = ($raw -replace '\r?\n', '')
$flat = $flat -replace '\\"', '"'
foreach ($match in [regex]::Matches($flat, '\{"type":"x100_result"[^}]+\}')) {
    $parsed = Parse-JsonResultBlob $match.Value
    if ($parsed -and -not $results.ContainsKey($parsed.VU)) {
        $results[$parsed.VU] = $parsed
    }
}

$rows = @($results.Values | Sort-Object VU)
if ($rows.Count -eq 0) {
    Write-Error "No x100_result entries found in log: $LogPath"
}

foreach ($row in $rows) {
    $row | Add-Member -NotePropertyName GrandTotalText -NotePropertyValue (Format-RubleTotal $row.GrandTotal) -Force
    $row | Add-Member -NotePropertyName CalcWaitSec -NotePropertyValue ([Math]::Round($row.CalcWaitMs / 1000, 1)) -Force
    $row | Add-Member -NotePropertyName DurationSec -NotePropertyValue ([Math]::Round($row.DurationMs / 1000, 1)) -Force
}

if ($Json) {
    $rows | ConvertTo-Json -Depth 4
    exit 0
}

Write-Host ""
Write-Host "k6 X100 results: $LogPath"
Write-Host ""
Write-Host ("{0,3} {1,-18} {2,12} {3,7} {4,16} {5,10} {6,10}" -f "VU", "Code", "Lines", "Errors", "Grand total", "Calc wait", "Duration")
Write-Host ("{0,3} {1,-18} {2,12} {3,7} {4,16} {5,10} {6,10}" -f "--", "----", "-----", "------", "-----------", "---------", "--------")

foreach ($row in $rows) {
    $lines = if ($row.LinesTotal -gt 0) { "{0}/{1}" -f $row.LinesDone, $row.LinesTotal } else { "0/0" }
    Write-Host ("{0,3} {1,-18} {2,12} {3,7} {4,16} {5,9}s {6,9}s" -f `
            $row.VU, `
            $row.Code, `
            $lines, `
            $row.Errors, `
            (Format-RubleTotal $row.GrandTotal), `
            $row.CalcWaitSec, `
            $row.DurationSec)
}

Write-Host ""
