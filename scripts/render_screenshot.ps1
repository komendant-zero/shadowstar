Add-Type -AssemblyName System.Drawing

$jsonPath = Join-Path $PSScriptRoot "mock_frame.json"
if (-not (Test-Path $jsonPath)) {
    Write-Error "mock_frame.json not found"
    exit 1
}

$rawJson = [System.IO.File]::ReadAllText($jsonPath, [System.Text.Encoding]::UTF8)
$data = $rawJson | ConvertFrom-Json
$frame = $data.frame

$lines = $frame.Split("`n")

# Terminal font and measurements
$fontSize = 13.0
$fontFamily = "Cascadia Code"
$font = $null
try {
    $font = New-Object System.Drawing.Font($fontFamily, $fontSize, [System.Drawing.FontStyle]::Regular)
} catch {
    $fontFamily = "Consolas"
    $font = New-Object System.Drawing.Font($fontFamily, $fontSize, [System.Drawing.FontStyle]::Regular)
}
$fontBold = New-Object System.Drawing.Font($fontFamily, $fontSize, [System.Drawing.FontStyle]::Bold)

# Measure character width and line height
$dummyBmp = New-Object System.Drawing.Bitmap(100, 100)
$dummyG = [System.Drawing.Graphics]::FromImage($dummyBmp)
$charSize = $dummyG.MeasureString("M", $font, 100, [System.Drawing.StringFormat]::GenericTypographic)
$charW = [Math]::Ceiling($charSize.Width * 1.05)
$lineH = [Math]::Ceiling($charSize.Height * 1.35)
$dummyG.Dispose()
$dummyBmp.Dispose()

$termCols = 90
$termRows = $lines.Count

$padX = 24
$padY = 46
$padBottom = 24

$termWidth = ($termCols * $charW) + ($padX * 2)
$termHeight = ($termRows * $lineH) + $padY + $padBottom

$canvasW = $termWidth + 60
$canvasH = $termHeight + 60

$bmp = New-Object System.Drawing.Bitmap($canvasW, $canvasH, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::ClearTypeGridFit

# Outer canvas background
$pt1 = New-Object System.Drawing.Point(0, 0)
$pt2 = New-Object System.Drawing.Point($canvasW, $canvasH)
$c1 = [System.Drawing.ColorTranslator]::FromHtml("#050811")
$c2 = [System.Drawing.ColorTranslator]::FromHtml("#0B1120")
$bgBrush = New-Object System.Drawing.Drawing2D.LinearGradientBrush($pt1, $pt2, $c1, $c2)
$g.FillRectangle($bgBrush, 0, 0, $canvasW, $canvasH)
$bgBrush.Dispose()

# Terminal Window Outer Rect
$winX = 30
$winY = 30
$winW = $termWidth
$winH = $termHeight
$winRadius = 14

$winPath = New-Object System.Drawing.Drawing2D.GraphicsPath
$winPath.AddArc($winX, $winY, $winRadius * 2, $winRadius * 2, 180, 90)
$winPath.AddArc($winX + $winW - $winRadius * 2, $winY, $winRadius * 2, $winRadius * 2, 270, 90)
$winPath.AddArc($winX + $winW - $winRadius * 2, $winY + $winH - $winRadius * 2, $winRadius * 2, $winRadius * 2, 0, 90)
$winPath.AddArc($winX, $winY + $winH - $winRadius * 2, $winRadius * 2, $winRadius * 2, 90, 90)
$winPath.CloseFigure()

# Terminal Body Background
$termBgBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#080D1A"))
$g.FillPath($termBgBrush, $winPath)
$termBgBrush.Dispose()

# Window Border
$borderPen = New-Object System.Drawing.Pen([System.Drawing.ColorTranslator]::FromHtml("#1E2D48"), 1.8)
$g.DrawPath($borderPen, $winPath)
$borderPen.Dispose()

# Titlebar
$tbPath = New-Object System.Drawing.Drawing2D.GraphicsPath
$tbPath.AddArc($winX, $winY, $winRadius * 2, $winRadius * 2, 180, 90)
$tbPath.AddArc($winX + $winW - $winRadius * 2, $winY, $winRadius * 2, $winRadius * 2, 270, 90)
$tbPath.AddLine($winX + $winW, $winY + 36, $winX, $winY + 36)
$tbPath.CloseFigure()

$tbBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#0F172A"))
$g.FillPath($tbBrush, $tbPath)
$tbBrush.Dispose()

# Window Control Buttons
$btnY = $winY + 13
$dotR = 11

$redBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#EF4444"))
$g.FillEllipse($redBrush, $winX + 16, $btnY, $dotR, $dotR)
$redBrush.Dispose()

$yelBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#F59E0B"))
$g.FillEllipse($yelBrush, $winX + 34, $btnY, $dotR, $dotR)
$yelBrush.Dispose()

$grnBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#10B981"))
$g.FillEllipse($grnBrush, $winX + 52, $btnY, $dotR, $dotR)
$grnBrush.Dispose()

# Window Title
$tbTitleFont = New-Object System.Drawing.Font("Segoe UI", 9.5, [System.Drawing.FontStyle]::Bold)
$tbTitleBrush = New-Object System.Drawing.SolidBrush([System.Drawing.ColorTranslator]::FromHtml("#94A3B8"))
$tbFormat = New-Object System.Drawing.StringFormat
$tbFormat.Alignment = [System.Drawing.StringAlignment]::Center
$tbFormat.LineAlignment = [System.Drawing.StringAlignment]::Center
$titleRect = New-Object System.Drawing.RectangleF($winX, $winY, $winW, 36)
$g.DrawString("ShadowStar TUI (v1.4.0-stealth) - Windows Terminal", $tbTitleFont, $tbTitleBrush, $titleRect, $tbFormat)
$tbTitleBrush.Dispose()
$tbTitleFont.Dispose()

# ANSI Color Palette
$palette = @{
    "0"  = [System.Drawing.ColorTranslator]::FromHtml("#94A3B8")
    "30" = [System.Drawing.ColorTranslator]::FromHtml("#1E293B")
    "31" = [System.Drawing.ColorTranslator]::FromHtml("#F87171")
    "32" = [System.Drawing.ColorTranslator]::FromHtml("#34D399")
    "33" = [System.Drawing.ColorTranslator]::FromHtml("#FBBF24")
    "34" = [System.Drawing.ColorTranslator]::FromHtml("#60A5FA")
    "35" = [System.Drawing.ColorTranslator]::FromHtml("#E879F9")
    "36" = [System.Drawing.ColorTranslator]::FromHtml("#22D3EE")
    "37" = [System.Drawing.ColorTranslator]::FromHtml("#F8FAFC")
    "90" = [System.Drawing.ColorTranslator]::FromHtml("#64748B")
    "91" = [System.Drawing.ColorTranslator]::FromHtml("#EF4444")
    "92" = [System.Drawing.ColorTranslator]::FromHtml("#10B981")
    "93" = [System.Drawing.ColorTranslator]::FromHtml("#F59E0B")
    "94" = [System.Drawing.ColorTranslator]::FromHtml("#3B82F6")
    "95" = [System.Drawing.ColorTranslator]::FromHtml("#D946EF")
    "96" = [System.Drawing.ColorTranslator]::FromHtml("#00E5FF")
    "97" = [System.Drawing.ColorTranslator]::FromHtml("#FFFFFF")
}

$strFormat = [System.Drawing.StringFormat]::GenericTypographic

$startY = $winY + $padY
for ($row = 0; $row -lt $lines.Count; $row++) {
    $line = $lines[$row]
    $curY = $startY + ($row * $lineH)
    $curX = $winX + $padX

    # Regex for ANSI escape codes: \x1b\[[0-9;]*m
    $pattern = [regex]'\x1b\[([0-9;]*)m'
    $matches = $pattern.Matches($line)

    $lastIdx = 0
    $curColor = $palette["0"]
    $isBold = $false

    $tokens = @()
    foreach ($m in $matches) {
        if ($m.Index -gt $lastIdx) {
            $tokens += @{ IsCode = $false; Text = $line.Substring($lastIdx, $m.Index - $lastIdx) }
        }
        $tokens += @{ IsCode = $true; Code = $m.Groups[1].Value }
        $lastIdx = $m.Index + $m.Length
    }
    if ($lastIdx -lt $line.Length) {
        $tokens += @{ IsCode = $false; Text = $line.Substring($lastIdx) }
    }

    foreach ($tok in $tokens) {
        if ($tok.IsCode) {
            $codes = $tok.Code.Split(";")
            foreach ($c in $codes) {
                if ($c -eq "0" -or $c -eq "") {
                    $curColor = $palette["0"]
                    $isBold = $false
                } elseif ($c -eq "1") {
                    $isBold = $true
                } elseif ($palette.ContainsKey($c)) {
                    $curColor = $palette[$c]
                }
            }
        } else {
            $txt = $tok.Text
            if ($txt.Length -gt 0) {
                $f = if ($isBold) { $fontBold } else { $font }
                $brush = New-Object System.Drawing.SolidBrush($curColor)
                for ($i = 0; $i -lt $txt.Length; $i++) {
                    $charStr = $txt[$i].ToString()
                    $g.DrawString($charStr, $f, $brush, [float]$curX, [float]$curY, $strFormat)
                    $curX += $charW
                }
                $brush.Dispose()
            }
        }
    }
}

# Save output PNG
$rootPath = Split-Path $PSScriptRoot -Parent
$outAssets = Join-Path $rootPath "assets\shadowstar-tui.png"
$bmp.Save($outAssets, [System.Drawing.Imaging.ImageFormat]::Png)
Write-Host "Screenshot saved to: $outAssets"

$docsDir = Join-Path $rootPath "docs\screenshots"
if (-not (Test-Path $docsDir)) { New-Item -ItemType Directory -Path $docsDir -Force | Out-Null }
$outDocs = Join-Path $docsDir "shadowstar-tui.png"
$bmp.Save($outDocs, [System.Drawing.Imaging.ImageFormat]::Png)
Write-Host "Screenshot saved to: $outDocs"

$artifactDir = "C:\Users\atack\.gemini\antigravity\brain\7b6ba60d-2cc7-485d-9c8b-38b0ceeb4b33"
if (Test-Path $artifactDir) {
    $outArtifact = Join-Path $artifactDir "shadowstar-tui.png"
    $bmp.Save($outArtifact, [System.Drawing.Imaging.ImageFormat]::Png)
    Write-Host "Screenshot saved to artifact: $outArtifact"
}

$font.Dispose()
$fontBold.Dispose()
$g.Dispose()
$bmp.Dispose()
