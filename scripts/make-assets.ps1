# Generate web/apple-icon.png and web/og.png to match the dark monochrome UI.
# Run from the repository root: powershell -ExecutionPolicy Bypass -File .\scripts\make-assets.ps1

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$root = Split-Path -Parent $PSScriptRoot
$web = Join-Path $root "web"

function New-RoundedPath {
  param([float]$X, [float]$Y, [float]$W, [float]$H, [float]$R)
  $path = New-Object System.Drawing.Drawing2D.GraphicsPath
  $d = $R * 2
  $path.AddArc($X, $Y, $d, $d, 180, 90)
  $path.AddArc($X + $W - $d, $Y, $d, $d, 270, 90)
  $path.AddArc($X + $W - $d, $Y + $H - $d, $d, $d, 0, 90)
  $path.AddArc($X, $Y + $H - $d, $d, $d, 90, 90)
  $path.CloseFigure()
  return $path
}

function Brush([string]$Hex) {
  return New-Object System.Drawing.SolidBrush ([System.Drawing.ColorTranslator]::FromHtml($Hex))
}

function Pen([string]$Hex, [float]$Width) {
  $pen = New-Object System.Drawing.Pen ([System.Drawing.ColorTranslator]::FromHtml($Hex)), $Width
  $pen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
  $pen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
  $pen.LineJoin = [System.Drawing.Drawing2D.LineJoin]::Round
  return $pen
}

# --- Apple touch icon: 180 x 180 -------------------------------------------
$icon = New-Object System.Drawing.Bitmap 180, 180
$g = [System.Drawing.Graphics]::FromImage($icon)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.Clear([System.Drawing.ColorTranslator]::FromHtml("#191919"))

$plate = New-RoundedPath 4 4 172 172 34
$g.FillPath((Brush "#191919"), $plate)
$g.DrawPath((Pen "#f2efe6" 6), $plate)

$yellow = Pen "#ffe14d" 9
$g.DrawEllipse($yellow, 50, 50, 80, 80)
$g.DrawLine($yellow, 90, 118, 90, 64)
$g.DrawLine($yellow, 70, 84, 90, 64)
$g.DrawLine($yellow, 110, 84, 90, 64)

$icon.Save((Join-Path $web "apple-icon.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$icon.Dispose()

# --- Open Graph card: 1200 x 630 -------------------------------------------
$og = New-Object System.Drawing.Bitmap 1200, 630
$g = [System.Drawing.Graphics]::FromImage($og)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
$g.Clear([System.Drawing.ColorTranslator]::FromHtml("#191919"))

$grid = Pen "#262626" 1
for ($x = 0; $x -le 1200; $x += 40) { $g.DrawLine($grid, $x, 0, $x, 630) }
for ($y = 0; $y -le 630; $y += 40) { $g.DrawLine($grid, 0, $y, 1200, $y) }

$mark = New-RoundedPath 80 58 72 72 14
$g.FillPath((Brush "#ffe14d"), $mark)
$g.DrawPath((Pen "#191919" 4), $mark)
$wFont = New-Object System.Drawing.Font "Segoe UI", 42, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("W", $wFont, (Brush "#191919"), 99, 63)

$brandFont = New-Object System.Drawing.Font "Segoe UI", 25, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("WEBIKO DEPLOYER", $brandFont, (Brush "#f2efe6"), 174, 81)

$headFont = New-Object System.Drawing.Font "Segoe UI", 82, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("DEPLOY SITUS", $headFont, (Brush "#f2efe6"), 80, 196)
$g.DrawString("KE VERCEL", $headFont, (Brush "#f2efe6"), 80, 286)

$subFont = New-Object System.Drawing.Font "Segoe UI", 30, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("ZIP, GitHub, atau tempel kode.", $subFont, (Brush "#b8b3a8"), 82, 418)
$g.DrawString("Nama dicek otomatis.", $subFont, (Brush "#b8b3a8"), 82, 460)

$g.DrawLine((Pen "#403d37" 3), 80, 532, 1120, 532)

$dot = Brush "#a8f0d0"
$g.FillEllipse($dot, 82, 558, 14, 14)
$footFont = New-Object System.Drawing.Font "Segoe UI", 23, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("Tanpa akun Vercel sendiri", $footFont, (Brush "#b8b3a8"), 108, 550)

$pill = New-RoundedPath 760 540 360 52 10
$g.FillPath((Brush "#191919"), $pill)
$g.DrawPath((Pen "#f2efe6" 3), $pill)
$urlFont = New-Object System.Drawing.Font "Consolas", 22, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("webiko-deployer.vercel.app", $urlFont, (Brush "#ffe14d"), 782, 553)

$og.Save((Join-Path $web "og.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$og.Dispose()

Write-Output "assets written: web/apple-icon.png, web/og.png"
