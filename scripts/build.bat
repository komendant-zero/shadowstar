@echo off
setlocal
echo ====================================================
echo  Building ShadowStar Multi-Platform Binaries
echo ====================================================

set GOCACHE=%~dp0.gocache
set GOPATH=%~dp0.gopath
if not exist "%~dp0bin" mkdir "%~dp0bin"

echo [*] Building Windows Client (Interactive TUI + Core)...
go build -buildvcs=false -ldflags="-s -w" -o "%~dp0bin\shadowstar.exe" ./cmd/shadowstar
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build Windows client.
    pause
    exit /b 1
)

echo [*] Building Windows Server...
go build -ldflags="-s -w" -o "%~dp0bin\shadowstar-server.exe" ./cmd/shadowstar-server
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build Windows server.
    pause
    exit /b 1
)

echo [*] Cross-compiling Linux amd64 Client (TUI + Core) & Server...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -buildvcs=false -ldflags="-s -w" -o "%~dp0bin\shadowstar-linux-amd64" ./cmd/shadowstar
go build -buildvcs=false -ldflags="-s -w" -o "%~dp0bin\shadowstar-server-linux-amd64" ./cmd/shadowstar-server

echo [*] Cross-compiling Linux arm64 Client (TUI + Core) & Server...
set GOOS=linux
set GOARCH=arm64
go build -buildvcs=false -ldflags="-s -w" -o "%~dp0bin\shadowstar-linux-arm64" ./cmd/shadowstar
go build -buildvcs=false -ldflags="-s -w" -o "%~dp0bin\shadowstar-server-linux-arm64" ./cmd/shadowstar-server

set GOOS=windows
set GOARCH=amd64
echo ====================================================
echo  Build Complete! Binaries created in bin\
echo ====================================================
dir "%~dp0bin"
pause
