# ShadowStar Mobile Build Automation Script
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "   ShadowStar Mobile Builder (v1.3.0)   " -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$env:GOCACHE = Join-Path $root ".gocache"
if (!(Test-Path $env:GOCACHE)) {
    New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
}

$binDir = Join-Path $root "bin"
if (!(Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}

$assetsDir = Join-Path $root "mobile\android\app\src\main\assets"
if (!(Test-Path $assetsDir)) {
    New-Item -ItemType Directory -Path $assetsDir -Force | Out-Null
}

Write-Host "`n[1/3] Building native Android ARM64 Core daemon..." -ForegroundColor Yellow
$env:GOOS = "android"
$env:GOARCH = "arm64"
$arm64Target = Join-Path $binDir "shadowstar-mobile-android-arm64"
go build -ldflags="-s -w" -o $arm64Target ./cmd/shadowstar-mobile
if ($LASTEXITCODE -eq 0) {
    Write-Host "  -> Successfully generated: $arm64Target" -ForegroundColor Green
    Copy-Item $arm64Target (Join-Path $assetsDir "shadowstar-mobile-android-arm64") -Force
    Write-Host "  -> Bundled into Android assets: $assetsDir" -ForegroundColor Green
} else {
    Write-Host "  -> Android ARM64 build failed!" -ForegroundColor Red
}

Write-Host "`n[2/3] Building Linux ARM64 (mobile router / Raspberry Pi)..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$linuxTarget = Join-Path $binDir "shadowstar-mobile-linux-arm64"
go build -ldflags="-s -w" -o $linuxTarget ./cmd/shadowstar-mobile
if ($LASTEXITCODE -eq 0) {
    Write-Host "  -> Successfully generated: $linuxTarget" -ForegroundColor Green
}

Write-Host "`n[3/3] Building Windows local Mobile Test daemon..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$winTarget = Join-Path $binDir "shadowstar-mobile.exe"
go build -ldflags="-s -w" -o $winTarget ./cmd/shadowstar-mobile
if ($LASTEXITCODE -eq 0) {
    Write-Host "  -> Successfully generated: $winTarget" -ForegroundColor Green
}

Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host "  Mobile Build Complete!                " -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "To test Mobile Web Client locally, run:"
Write-Host "  .\bin\shadowstar-mobile.exe" -ForegroundColor White
Write-Host "Then open http://localhost:10810 in your smartphone or desktop browser.`n"
