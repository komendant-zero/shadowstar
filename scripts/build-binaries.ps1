$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$BaseDir = Split-Path -Parent $ScriptDir
Set-Location $BaseDir

$RelDir = "$BaseDir\bin\release-v2.1.0"
if (-not (Test-Path $RelDir)) {
    New-Item -ItemType Directory -Path $RelDir -Force | Out-Null
}

Write-Host "Building Linux amd64..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar-linux-amd64" ./cmd/shadowstar
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar-server-linux-amd64" ./cmd/shadowstar-server

Write-Host "Building Linux arm64..."
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar-linux-arm64" ./cmd/shadowstar
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar-server-linux-arm64" ./cmd/shadowstar-server

Write-Host "Building Windows amd64..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar.exe" ./cmd/shadowstar
go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$RelDir\shadowstar-server.exe" ./cmd/shadowstar-server

Write-Host "Binaries successfully compiled to $RelDir"
