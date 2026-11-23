# Generate web/apple-icon.png and web/og.png to match the dark UI.
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

function Draw-W {
  param($Graphics, $Pen, [float]$Left, [float]$Top, [float]$Width, [float]$Height)
  $x = { param($f) $Left + ($Width * $f) }
  $y = { param($f) $Top + ($Height * $f) }
  $points = @(
    (New-Object System.Drawing.PointF (& $x 0.12), (& $y 0.22)),
    (New-Object System.Drawing.PointF (& $x 0.32), (& $y 0.78)),
    (New-Object System.Drawing.PointF (& $x 0.50), (& $y 0.50)),
    (New-Object System.Drawing.PointF (& $x 0.68), (& $y 0.78)),
    (New-Object System.Drawing.PointF (& $x 0.88), (& $y 0.22))
  )
  $Graphics.DrawLines($Pen, $points)
}

# --- Apple touch icon: 180 x 180 -------------------------------------------
$icon = New-Object System.Drawing.Bitmap 180, 180
$g = [System.Drawing.Graphics]::FromImage($icon)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.Clear([System.Drawing.ColorTranslator]::FromHtml("#191919"))

$plate = New-RoundedPath 18 18 144 144 34
$g.FillPath((Brush "#ffe14d"), $plate)
Draw-W $g (Pen "#191919" 15) 18 18 144 144

$icon.Save((Join-Path $web "apple-icon.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$icon.Dispose()

# --- Open Graph card: 1200 x 630 -------------------------------------------
$og = New-Object System.Drawing.Bitmap 1200, 630
$g = [System.Drawing.Graphics]::FromImage($og)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
$g.Clear([System.Drawing.ColorTranslator]::FromHtml("#191919"))

$glow = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(16, 255, 225, 77))
$g.FillEllipse($glow, 260, -220, 680, 420)

$panel = New-RoundedPath 60 60 1080 530 28
$g.FillPath((Brush "#1f1e1c"), $panel)
$g.DrawPath((Pen "#302e29" 2), $panel)

$mark = New-RoundedPath 110 110 72 72 18
$g.FillPath((Brush "#ffe14d"), $mark)
Draw-W $g (Pen "#191919" 8) 110 110 72 72

$brandFont = New-Object System.Drawing.Font "Segoe UI", 26, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("Webiko Deployer", $brandFont, (Brush "#f2efe6"), 202, 128)

$headFont = New-Object System.Drawing.Font "Segoe UI", 76, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("Deploy situs", $headFont, (Brush "#f2efe6"), 108, 236)
$g.DrawString("ke Vercel", $headFont, (Brush "#f2efe6"), 108, 320)

$subFont = New-Object System.Drawing.Font "Segoe UI", 28, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("ZIP, GitHub, atau tempel kode.", $subFont, (Brush "#a39d90"), 112, 434)
$g.DrawString("Nama dicek otomatis sebelum deploy.", $subFont, (Brush "#a39d90"), 112, 474)

$g.DrawLine((Pen "#302e29" 2), 112, 524, 1088, 524)

$urlFont = New-Object System.Drawing.Font "Consolas", 23, ([System.Drawing.FontStyle]::Bold), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("webiko-deployer.vercel.app", $urlFont, (Brush "#ffe14d"), 112, 540)

$footFont = New-Object System.Drawing.Font "Segoe UI", 22, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("tanpa akun Vercel sendiri", $footFont, (Brush "#a39d90"), 790, 542)

$og.Save((Join-Path $web "og.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$og.Dispose()

Write-Output "assets written: web/apple-icon.png, web/og.png"
