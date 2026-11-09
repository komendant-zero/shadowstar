# ShadowStar Icon Generator
# Generates high-res multi-platform icons: Windows ICO, Android Mipmaps, and Web PWA PNGs

Add-Type -AssemblyName System.Drawing

function Render-StarBadge {
    param(
        [int]$Size,
        [bool]$IsRound = $false
    )

    $bmp = New-Object System.Drawing.Bitmap($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.Clear([System.Drawing.Color]::Transparent)

    $pad = [float]($Size * 0.04)
    $rect = New-Object System.Drawing.RectangleF($pad, $pad, ($Size - 2 * $pad), ($Size - 2 * $pad))

    # Background Path
    $bgPath = New-Object System.Drawing.Drawing2D.GraphicsPath
    if ($IsRound) {
        $bgPath.AddEllipse($rect)
    } else {
        $radius = [float]($Size * 0.22)
        $d = $radius * 2
        $bgPath.AddArc($rect.X, $rect.Y, $d, $d, 180, 90)
        $bgPath.AddArc($rect.Right - $d, $rect.Y, $d, $d, 270, 90)
        $bgPath.AddArc($rect.Right - $d, $rect.Bottom - $d, $d, $d, 0, 90)
        $bgPath.AddArc($rect.X, $rect.Bottom - $d, $d, $d, 90, 90)
        $bgPath.CloseFigure()
    }

    # Fill Background Gradient (Deep Cyber Navy)
    $cTop = [System.Drawing.Color]::FromArgb(255, 16, 24, 40)
    $cBot = [System.Drawing.Color]::FromArgb(255, 9, 13, 22)
    $bgBrush = New-Object System.Drawing.Drawing2D.LinearGradientBrush($rect, $cTop, $cBot, 90.0)
    $g.FillPath($bgBrush, $bgPath)
    $bgBrush.Dispose()

    # Outer Glow / Border (Cyan)
    $borderPen = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(220, 0, 229, 255), [float]($Size * 0.035))
    $g.DrawPath($borderPen, $bgPath)
    $borderPen.Dispose()
    $bgPath.Dispose()

    # Star Center Coordinates
    $cx = [float]($Size / 2.0)
    $cy = [float]($Size / 2.0)

    # 1. Outer Star (Cyan)
    $rOut1 = [float]($Size * 0.38)
    $rIn1  = [float]($Size * 0.17)
    $pts1 = New-Object "System.Drawing.PointF[]" 10
    $angle = - [Math]::PI / 2.0
    $step = [Math]::PI / 5.0
    for ($i = 0; $i -lt 10; $i++) {
        $r = if ($i % 2 -eq 0) { $rOut1 } else { $rIn1 }
        $pts1[$i] = New-Object System.Drawing.PointF([float]($cx + $r * [Math]::Cos($angle)), [float]($cy + $r * [Math]::Sin($angle)))
        $angle += $step
    }

    $star1Path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $star1Path.AddPolygon($pts1)

    $cyanColor = [System.Drawing.Color]::FromArgb(255, 0, 229, 255)
    $star1Brush = New-Object System.Drawing.SolidBrush($cyanColor)
    $g.FillPath($star1Brush, $star1Path)
    $star1Brush.Dispose()

    # 2. Inner Star (Emerald Green)
    $rOut2 = [float]($Size * 0.22)
    $rIn2  = [float]($Size * 0.09)
    $pts2 = New-Object "System.Drawing.PointF[]" 10
    $angle = - [Math]::PI / 2.0
    for ($i = 0; $i -lt 10; $i++) {
        $r = if ($i % 2 -eq 0) { $rOut2 } else { $rIn2 }
        $pts2[$i] = New-Object System.Drawing.PointF([float]($cx + $r * [Math]::Cos($angle)), [float]($cy + $r * [Math]::Sin($angle)))
        $angle += $step
    }

    $star2Path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $star2Path.AddPolygon($pts2)

    $greenColor = [System.Drawing.Color]::FromArgb(255, 16, 185, 129)
    $star2Brush = New-Object System.Drawing.SolidBrush($greenColor)
    $g.FillPath($star2Brush, $star2Path)
    $star2Brush.Dispose()

    # 3. Center Highlight (Crisp White Core Dot)
    $coreR = [float]($Size * 0.04)
    $coreBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(240, 255, 255, 255))
    $g.FillEllipse($coreBrush, ($cx - $coreR), ($cy - $coreR), ($coreR * 2), ($coreR * 2))
    $coreBrush.Dispose()

    $star1Path.Dispose()
    $star2Path.Dispose()
    $g.Dispose()

    return $bmp
}

function Build-IcoFile {
    param(
        [string]$OutputPath,
        [array]$Images
    )
    $fs = New-Object System.IO.FileStream($OutputPath, [System.IO.FileMode]::Create)
    $bw = New-Object System.IO.BinaryWriter($fs)

    # ICONDIR header
    $bw.Write([uint16]0)
    $bw.Write([uint16]1)
    $bw.Write([uint16]$Images.Count)

    $offset = 6 + ($Images.Count * 16)

    foreach ($img in $Images) {
        $w = if ($img.Size -eq 256) { [byte]0 } else { [byte]$img.Size }
        $h = if ($img.Size -eq 256) { [byte]0 } else { [byte]$img.Size }
        $bw.Write($w)
        $bw.Write($h)
        $bw.Write([byte]0)
        $bw.Write([byte]0)
        $bw.Write([uint16]1)
        $bw.Write([uint16]32)
        $bw.Write([uint32]$img.Bytes.Length)
        $bw.Write([uint32]$offset)
        $offset += $img.Bytes.Length
    }

    foreach ($img in $Images) {
        $bw.Write($img.Bytes)
    }

    $bw.Flush()
    $bw.Close()
    $fs.Close()
}

Write-Host "=== Generating ShadowStar Icons ===" -ForegroundColor Cyan

# 1. Generate Web / PWA PNGs
$WebDir = "mobile\android_src\assets\web"
$PkgWebDir = "pkg\mobile\web"
New-Item -ItemType Directory -Path $WebDir -Force | Out-Null
New-Item -ItemType Directory -Path $PkgWebDir -Force | Out-Null

$bmp512 = Render-StarBadge -Size 512 -IsRound $false
$bmp512.Save("$WebDir\icon-512.png", [System.Drawing.Imaging.ImageFormat]::Png)
$bmp512.Save("$PkgWebDir\icon-512.png", [System.Drawing.Imaging.ImageFormat]::Png)

$bmp192 = Render-StarBadge -Size 192 -IsRound $false
$bmp192.Save("$WebDir\icon-192.png", [System.Drawing.Imaging.ImageFormat]::Png)
$bmp192.Save("$PkgWebDir\icon-192.png", [System.Drawing.Imaging.ImageFormat]::Png)
Write-Host "[+] Generated Web / PWA PNG icons (192, 512)" -ForegroundColor Green

# 2. Generate Android Mipmap PNGs (Square & Round)
$Mipmaps = @(
    @{ Name = "mipmap-mdpi";   Size = 48 },
    @{ Name = "mipmap-hdpi";   Size = 72 },
    @{ Name = "mipmap-xhdpi";  Size = 96 },
    @{ Name = "mipmap-xxhdpi"; Size = 144 },
    @{ Name = "mipmap-xxxhdpi";Size = 192 }
)

foreach ($m in $Mipmaps) {
    $dir = "mobile\android_src\res\$($m.Name)"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null

    $bmpSquare = Render-StarBadge -Size $m.Size -IsRound $false
    $bmpSquare.Save("$dir\ic_launcher.png", [System.Drawing.Imaging.ImageFormat]::Png)
    $bmpSquare.Dispose()

    $bmpRound = Render-StarBadge -Size $m.Size -IsRound $true
    $bmpRound.Save("$dir\ic_launcher_round.png", [System.Drawing.Imaging.ImageFormat]::Png)
    $bmpRound.Dispose()
}
Write-Host "[+] Generated Android launcher mipmaps (mdpi to xxxhdpi)" -ForegroundColor Green

# 3. Create Windows Multi-Resolution .ICO file
# Sizes: 256, 128, 64, 48, 32, 16
$IcoSizes = @(256, 128, 64, 48, 32, 16)
$PngStreams = @()

foreach ($s in $IcoSizes) {
    $b = Render-StarBadge -Size $s -IsRound $false
    $ms = New-Object System.IO.MemoryStream
    $b.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $b.Dispose()
    $PngStreams += @{
        Size = $s
        Bytes = $ms.ToArray()
    }
    $ms.Dispose()
}

New-Item -ItemType Directory -Path "assets" -Force | Out-Null
Build-IcoFile -OutputPath "assets\icon.ico" -Images $PngStreams
Copy-Item "assets\icon.ico" "mobile\android_src\assets\web\favicon.ico" -Force
Copy-Item "assets\icon.ico" "pkg\mobile\web\favicon.ico" -Force
$bmp512.Save("assets\icon-512.png", [System.Drawing.Imaging.ImageFormat]::Png)
$bmp512.Dispose()
$bmp192.Dispose()

Write-Host "[+] Generated assets\icon.ico and favicons" -ForegroundColor Green

# 4. Compile .syso for Windows Desktop Executable via rsrc
$RsrcExe = "C:\Users\atack\go\bin\rsrc.exe"
if (Test-Path $RsrcExe) {
    & $RsrcExe -ico "assets\icon.ico" -o "cmd\shadowstar\rsrc_windows_amd64.syso"
    Write-Host "[+] Compiled Windows resource cmd\shadowstar\rsrc_windows_amd64.syso" -ForegroundColor Green
} else {
    Write-Host "[!] rsrc.exe not found" -ForegroundColor Yellow
}

Write-Host "ALL ICONS SUCCESSFULLY GENERATED!" -ForegroundColor Cyan

