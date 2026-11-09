# Automated Android APK Build Script for ShadowStar Mobile
param(
    [string]$SdkDir = "D:\REN\renpy-8.4.1-sdk\rapt\Sdk",
    [string]$BuildToolsVer = "34.0.0",
    [string]$AndroidPlatform = "android-34"
)

$ErrorActionPreference = "Stop"

$BuildTools = "$SdkDir\build-tools\$BuildToolsVer"
$AndroidJar = "$SdkDir\platforms\$AndroidPlatform\android.jar"
$Aapt2 = "$BuildTools\aapt2.exe"
$D8 = "$BuildTools\d8.bat"
$Zipalign = "$BuildTools\zipalign.exe"
$ApkSigner = "$BuildTools\apksigner.bat"

$JavaHome = "C:\Program Files\Eclipse Adoptium\jdk-21.0.12.101-hotspot"
$Javac = "$JavaHome\bin\javac.exe"
$Keytool = "$JavaHome\bin\keytool.exe"
$Jar = "$JavaHome\bin\jar.exe"

$SrcRoot = "d:\ocli\ShadowStar\mobile\android_src"
$WorkDir = "d:\ocli\ShadowStar\mobile\apk_build"

Write-Host "=== ShadowStar APK Builder ===" -ForegroundColor Cyan

# Clean and recreate workdir
if (Test-Path $WorkDir) { Remove-Item -Recurse -Force $WorkDir }
New-Item -ItemType Directory -Path "$WorkDir\compiled_res", "$WorkDir\gen", "$WorkDir\obj" -Force | Out-Null
New-Item -ItemType Directory -Path "d:\ocli\ShadowStar\bin" -Force | Out-Null

# 1. Compile Resources with aapt2
Write-Host "[1/7] Compiling Android resources..." -ForegroundColor Yellow
& $Aapt2 compile --dir "$SrcRoot\res" -o "$WorkDir\compiled_res\res.zip"

# Build Android Go daemon if missing
New-Item -ItemType Directory -Path "$SrcRoot\lib\arm64-v8a" -Force | Out-Null
if (-not (Test-Path "$SrcRoot\lib\arm64-v8a\libshadowstar.so")) {
    Write-Host "Compiling Go daemon for Android arm64..." -ForegroundColor Yellow
    $env:GOOS = "android"
    $env:GOARCH = "arm64"
    $env:CGO_ENABLED = "0"
    & go build -trimpath -ldflags="-s -w" -o "$SrcRoot\lib\arm64-v8a\libshadowstar.so" ./cmd/shadowstar-mobile
}

function Build-Variant {
    param(
        [string]$VariantName,
        [string]$ConfigPath,
        [string]$OutputFile
    )

    Write-Host "--- Building variant: $VariantName -> $OutputFile ---" -ForegroundColor Cyan

    $VarWork = "$WorkDir\$VariantName"
    New-Item -ItemType Directory -Path "$VarWork\assets\web", "$VarWork\dex" -Force | Out-Null

    # Read base assets
    $RawHtml = Get-Content -Raw "$SrcRoot\assets\web\index.html"
    $RawCss  = Get-Content -Raw "$SrcRoot\assets\web\style.css"
    $RawJs   = Get-Content -Raw "$SrcRoot\assets\web\app.js"
    $CfgJson = "{}"
    if (Test-Path $ConfigPath) {
        $CfgJson = Get-Content -Raw $ConfigPath
    }

    # Bundle all CSS, JS and Config directly into a single self-contained HTML
    $InjectedHead = "<style>`n$RawCss`n</style>`n<script>`nwindow.EMBEDDED_CONFIG = $CfgJson;`n</script>`n</head>"
    $BundledHtml = $RawHtml.Replace('<link rel="stylesheet" href="./style.css">', '')
    $BundledHtml = $BundledHtml.Replace('</head>', $InjectedHead)
    $BundledHtml = $BundledHtml.Replace('<script src="./app.js"></script>', "<script>`n$RawJs`n</script>")

    # Save flat assets (prevents any Windows backslash in zip entry paths)
    Set-Content -Path "$VarWork\assets\index.html" -Value $BundledHtml -Encoding utf8
    Set-Content -Path "$VarWork\assets\style.css" -Value $RawCss -Encoding utf8
    Set-Content -Path "$VarWork\assets\app.js" -Value $RawJs -Encoding utf8
    Set-Content -Path "$VarWork\assets\config.json" -Value $CfgJson -Encoding utf8
    Copy-Item "$SrcRoot\assets\web\manifest.json" "$VarWork\assets\" -Force
    Copy-Item "$SrcRoot\assets\web\sw.js" "$VarWork\assets\" -Force

    # Also keep web/ subfolder copy
    Copy-Item "$VarWork\assets\index.html" "$VarWork\assets\web\index.html" -Force
    Copy-Item "$VarWork\assets\style.css" "$VarWork\assets\web\style.css" -Force
    Copy-Item "$VarWork\assets\app.js" "$VarWork\assets\web\app.js" -Force
    Copy-Item "$VarWork\assets\config.json" "$VarWork\assets\web\config.json" -Force

    # 2. Link with aapt2
    Write-Host "[2/7] Linking APK with aapt2..." -ForegroundColor Yellow
    $UnsignedApk = "$VarWork\unsigned.apk"
    & $Aapt2 link -I $AndroidJar --manifest "$SrcRoot\AndroidManifest.xml" --java "$WorkDir\gen" -o $UnsignedApk -A "$VarWork\assets" "$WorkDir\compiled_res\res.zip" --auto-add-overlay

    # 3. Compile Java with javac
    Write-Host "[3/7] Compiling Java sources..." -ForegroundColor Yellow
    $JavaFiles = Get-ChildItem -Path "$WorkDir\gen", "$SrcRoot\src" -Filter "*.java" -Recurse | Select-Object -ExpandProperty FullName
    & $Javac -source 11 -target 11 -g -cp $AndroidJar -d "$WorkDir\obj" $JavaFiles

    # 4. Generate Dalvik Dex with d8
    Write-Host "[4/7] Generating classes.dex with d8..." -ForegroundColor Yellow
    $ClassFiles = Get-ChildItem -Path "$WorkDir\obj" -Filter "*.class" -Recurse | Select-Object -ExpandProperty FullName
    & $D8 --release --min-api 26 --output "$VarWork\dex" $ClassFiles

    # 5. Pack dex and native library into APK
    Write-Host "[5/7] Adding classes.dex and arm64 binary into APK..." -ForegroundColor Yellow
    Set-Location "$VarWork\dex"
    & $Jar -uf $UnsignedApk classes.dex
    Set-Location "d:\ocli\ShadowStar"

    if (Test-Path "$SrcRoot\lib") {
        Set-Location "$SrcRoot"
        & $Jar -uf $UnsignedApk lib/arm64-v8a/libshadowstar.so
        Set-Location "d:\ocli\ShadowStar"
    }

    # 6. Align APK with zipalign
    Write-Host "[6/7] 4-byte page-aligning APK..." -ForegroundColor Yellow
    $AlignedApk = "$VarWork\aligned.apk"
    & $Zipalign -f -p 4 $UnsignedApk $AlignedApk

    # 7. Sign APK
    Write-Host "[7/7] Signing APK with debug keystore..." -ForegroundColor Yellow
    $Keystore = "$WorkDir\debug.keystore"
    if (-not (Test-Path $Keystore)) {
        & $Keytool -genkey -v -keystore $Keystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=ShadowStar,O=ShadowStar,C=US"
    }

    & $ApkSigner sign --ks $Keystore --ks-pass pass:android --ks-key-alias androiddebugkey --key-pass pass:android --out $OutputFile $AlignedApk
    Write-Host "Success: $OutputFile" -ForegroundColor Green
}

# Build Public APK
Build-Variant -VariantName "public" -ConfigPath "shadowstar-client.json" -OutputFile "d:\ocli\ShadowStar\bin\ShadowStar.apk"
Copy-Item "d:\ocli\ShadowStar\bin\ShadowStar.apk" "d:\ocli\ShadowStar\ShadowStar.apk" -Force

# Build Personal APK
if (Test-Path "shadowstar-client.personal.json") {
    Build-Variant -VariantName "personal" -ConfigPath "shadowstar-client.personal.json" -OutputFile "d:\ocli\ShadowStar\bin\ShadowStar-Personal.apk"
    Copy-Item "d:\ocli\ShadowStar\bin\ShadowStar-Personal.apk" "d:\ocli\ShadowStar\ShadowStar-Personal.apk" -Force
}

Write-Host "ALL APKS SUCCESSFULLY BUILT" -ForegroundColor Green
Get-Item "d:\ocli\ShadowStar\bin\*.apk" | Select-Object Name, Length
