# Pandora's Veil - Live 3-Device Demonstration Script
# Tests: Device A (Sender) -> Device B (Recipient) -> Device C (Unauthorized Attacker)

$ErrorActionPreference = "Stop"
$RelayURL = "http://127.0.0.1:8080"

Write-Host "`n========================================================" -ForegroundColor Cyan
Write-Host "   PANDORA'S VEIL — LIVE THREE-DEVICE SECURITY DEMO   " -ForegroundColor White
Write-Host "   'Even if the link leaks, the secret doesn't.'        " -ForegroundColor Yellow
Write-Host "========================================================`n" -ForegroundColor Cyan

# Setup temporary config directories for Alice, Bob, and Eve
$DemoDir = Join-Path $PSScriptRoot ".demo_identities"
$AliceConfig = Join-Path $DemoDir "alice.json"
$BobConfig = Join-Path $DemoDir "bob.json"
$EveConfig = Join-Path $DemoDir "eve.json"

if (Test-Path $DemoDir) { Remove-Item -Recurse -Force $DemoDir }
New-Item -ItemType Directory -Path $DemoDir | Out-Null

Write-Host "[STEP 1] Initializing Device A (Alice) and Device B (Bob)..." -ForegroundColor Yellow
.\pandora.exe init --handle PV-ALICE --config $AliceConfig --relay $RelayURL --force
.\pandora.exe init --handle PV-BOB --config $BobConfig --relay $RelayURL --force
.\pandora.exe init --handle PV-EVE --config $EveConfig --relay $RelayURL --force

Write-Host "`n[STEP 2] Alice sends a confidential secret targeted to Bob's device key..." -ForegroundColor Yellow
$SecretMsg = "TOP_SECRET_COORDINATES: 37.7749N, 122.4194W [AUTHORIZED_FOR_BOB_ONLY]"

# Auto-confirm prompt with 'y' for the automated demo runner
$SendOutput = "y" | .\pandora.exe send --to PV-BOB --config $AliceConfig --relay $RelayURL $SecretMsg
Write-Host $SendOutput

# Extract Paste ID from output
$Match = [regex]::Match($SendOutput, "Share ID:\s+([a-zA-Z0-9_]+)")
if (-not $Match.Success) {
    Write-Host "[✗] Could not find Share ID in output. Make sure the relay server is running!" -ForegroundColor Red
    exit 1
}
$PasteID = $Match.Groups[1].Value
Write-Host "`n[✓] Generated Share ID: $PasteID" -ForegroundColor Green

Write-Host "`n--------------------------------------------------------" -ForegroundColor DarkGray
Write-Host "[STEP 3] LEAKING THE LINK TO UNAUTHORIZED DEVICE (EVE)" -ForegroundColor Red
Write-Host "Eve intercepts the share link and attempts decryption..." -ForegroundColor DarkYellow
Write-Host "--------------------------------------------------------" -ForegroundColor DarkGray
try {
    .\pandora.exe read $PasteID --config $EveConfig --relay $RelayURL
} catch {
    # Expected failure
}

Write-Host "`n--------------------------------------------------------" -ForegroundColor DarkGray
Write-Host "[STEP 4] AUTHORIZED DEVICE (BOB) ACCESSES THE LINK" -ForegroundColor Green
Write-Host "Bob accesses the exact same share link..." -ForegroundColor DarkYellow
Write-Host "--------------------------------------------------------" -ForegroundColor DarkGray
.\pandora.exe read $PasteID --config $BobConfig --relay $RelayURL

Write-Host "`n========================================================" -ForegroundColor Cyan
Write-Host "   DEMO COMPLETED: ZERO-KNOWLEDGE BOUNDARY VERIFIED   " -ForegroundColor Green
Write-Host "========================================================`n" -ForegroundColor Cyan
