Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  Starting Pandora's Veil Zero-Knowledge Relay... " -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Cyan

$GoCmd = "go"
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Program Files\Go\bin\go.exe") {
        $GoCmd = "C:\Program Files\Go\bin\go.exe"
    }
}

& $GoCmd run ./demo-backend

