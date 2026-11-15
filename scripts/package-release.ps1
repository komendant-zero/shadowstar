# Package release v2.0.5 assets
$ErrorActionPreference = "Stop"
Set-Location "D:\ocli\ShadowStar"

$RelDir = "D:\ocli\ShadowStar\bin\release-v2.0.5"
if (-not (Test-Path $RelDir)) {
    New-Item -ItemType Directory -Path $RelDir -Force | Out-Null
}

$TempPkg = "$RelDir\temp"
if (Test-Path $TempPkg) { Remove-Item -Recurse -Force $TempPkg }
New-Item -ItemType Directory -Path $TempPkg -Force | Out-Null

# 1. Linux amd64 client
Write-Host "Packaging shadowstar-linux-amd64.tar.gz..."
New-Item -ItemType Directory -Path "$TempPkg\linux-amd64" -Force | Out-Null
Copy-Item "$RelDir\shadowstar-linux-amd64" "$TempPkg\linux-amd64\" -Force
Copy-Item "shadowstar-client.json" "$TempPkg\linux-amd64\" -Force
tar -czf "$RelDir\shadowstar-linux-amd64.tar.gz" -C "$TempPkg\linux-amd64" shadowstar-linux-amd64 shadowstar-client.json

# 2. Linux arm64 client
Write-Host "Packaging shadowstar-linux-arm64.tar.gz..."
New-Item -ItemType Directory -Path "$TempPkg\linux-arm64" -Force | Out-Null
Copy-Item "$RelDir\shadowstar-linux-arm64" "$TempPkg\linux-arm64\" -Force
Copy-Item "shadowstar-client.json" "$TempPkg\linux-arm64\" -Force
tar -czf "$RelDir\shadowstar-linux-arm64.tar.gz" -C "$TempPkg\linux-arm64" shadowstar-linux-arm64 shadowstar-client.json

# 3. Linux amd64 server
Write-Host "Packaging shadowstar-server-linux-amd64.tar.gz..."
New-Item -ItemType Directory -Path "$TempPkg\server-amd64" -Force | Out-Null
Copy-Item "$RelDir\shadowstar-server-linux-amd64" "$TempPkg\server-amd64\" -Force
tar -czf "$RelDir\shadowstar-server-linux-amd64.tar.gz" -C "$TempPkg\server-amd64" shadowstar-server-linux-amd64

# 4. Linux arm64 server
Write-Host "Packaging shadowstar-server-linux-arm64.tar.gz..."
New-Item -ItemType Directory -Path "$TempPkg\server-arm64" -Force | Out-Null
Copy-Item "$RelDir\shadowstar-server-linux-arm64" "$TempPkg\server-arm64\" -Force
tar -czf "$RelDir\shadowstar-server-linux-arm64.tar.gz" -C "$TempPkg\server-arm64" shadowstar-server-linux-arm64

# 5. Windows amd64 zip
Write-Host "Packaging shadowstar-windows-amd64.zip..."
New-Item -ItemType Directory -Path "$TempPkg\windows-amd64" -Force | Out-Null
Copy-Item "$RelDir\shadowstar.exe" "$TempPkg\windows-amd64\" -Force
Copy-Item "shadowstar-client.json" "$TempPkg\windows-amd64\" -Force
Copy-Item "bin\run-tun.bat" "$TempPkg\windows-amd64\" -Force
if (Test-Path "bin\run-v2-gui.bat") {
    Copy-Item "bin\run-v2-gui.bat" "$TempPkg\windows-amd64\run-gui.bat" -Force
}
if (Test-Path "$RelDir\shadowstar-windows-amd64.zip") {
    Remove-Item "$RelDir\shadowstar-windows-amd64.zip" -Force
}
Compress-Archive -Path "$TempPkg\windows-amd64\*" -DestinationPath "$RelDir\shadowstar-windows-amd64.zip"

# 6. APKs
Write-Host "Copying APKs to release directory..."
Copy-Item "bin\ShadowStar-v2.apk" "$RelDir\ShadowStar.apk" -Force
Copy-Item "bin\ShadowStar-v2.apk" "$RelDir\ShadowStar-v2.apk" -Force
if (Test-Path "bin\ShadowStar-v2-Personal.apk") {
    Copy-Item "bin\ShadowStar-v2-Personal.apk" "$RelDir\ShadowStar-v2-Personal.apk" -Force
}

# Cleanup temp
Remove-Item -Recurse -Force $TempPkg

Write-Host "Release packaging complete:" -ForegroundColor Green
Get-ChildItem "$RelDir\*.tar.gz", "$RelDir\*.zip", "$RelDir\*.apk" | Select-Object Name, Length
