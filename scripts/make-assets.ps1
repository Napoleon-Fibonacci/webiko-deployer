# Generate favicon, touch icon, PWA icons and the Open Graph card so they
# match the yellow "W" badge in the app bar.
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

# The app bar badge is a yellow rounded square with a small, bold W. These
# ratios reproduce that mark at any size so every icon matches it.
function New-MarkBitmap {
  param([int]$Size, [switch]$Opaque)
  $bitmap = New-Object System.Drawing.Bitmap $Size, $Size
  $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
  $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias

  if ($Opaque) {
    # iOS applies its own rounded mask, so this one fills the whole square.
    $graphics.Clear([System.Drawing.ColorTranslator]::FromHtml("#ffe14d"))
  } else {
    $graphics.Clear([System.Drawing.Color]::Transparent)
    $plate = New-RoundedPath 0 0 $Size $Size ($Size * 0.25)
    $graphics.FillPath((Brush "#ffe14d"), $plate)
    $plate.Dispose()
  }

  $boxW = $Size * 0.579
  $boxH = $Size * 0.670
  Draw-W $graphics (Pen "#121314" ($Size * 0.085)) `
    (($Size - $boxW) / 2) (($Size - $boxH) / 2) $boxW $boxH

  $graphics.Dispose()
  return $bitmap
}

function Get-PngBytes {
  param($Bitmap)
  $stream = New-Object System.IO.MemoryStream
  $Bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
  $bytes = $stream.ToArray()
  $stream.Dispose()
  # The unary comma stops PowerShell from unrolling the byte array.
  return ,$bytes
}

function Write-Ico {
  param([string]$Path, $Entries)
  $stream = [System.IO.File]::Create($Path)
  $writer = New-Object System.IO.BinaryWriter($stream)
  try {
    $writer.Write([UInt16]0)
    $writer.Write([UInt16]1)
    $writer.Write([UInt16]$Entries.Count)
    $offset = 6 + (16 * $Entries.Count)
    foreach ($entry in $Entries) {
      $dimension = if ($entry.Size -ge 256) { 0 } else { $entry.Size }
      $writer.Write([byte]$dimension)
      $writer.Write([byte]$dimension)
      $writer.Write([byte]0)
      $writer.Write([byte]0)
      $writer.Write([UInt16]1)
      $writer.Write([UInt16]32)
      $writer.Write([UInt32]$entry.Bytes.Length)
      $writer.Write([UInt32]$offset)
      $offset += $entry.Bytes.Length
    }
    foreach ($entry in $Entries) {
      $writer.Write([byte[]]$entry.Bytes)
    }
  }
  finally {
    $writer.Dispose()
    $stream.Dispose()
  }
}

# --- Favicon and PWA icons --------------------------------------------------
$icon32 = New-MarkBitmap 32
$icon32.Save((Join-Path $web "icon-32.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$icon192 = New-MarkBitmap 192
$icon192.Save((Join-Path $web "icon-192.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$icon512 = New-MarkBitmap 512
$icon512.Save((Join-Path $web "icon-512.png"), [System.Drawing.Imaging.ImageFormat]::Png)

$icon16 = New-MarkBitmap 16
Write-Ico (Join-Path $web "favicon.ico") @(
  @{ Size = 16; Bytes = (Get-PngBytes $icon16) },
  @{ Size = 32; Bytes = (Get-PngBytes $icon32) }
)

# --- Apple touch icon: 180 x 180, opaque ------------------------------------
$apple = New-MarkBitmap 180 -Opaque
$apple.Save((Join-Path $web "apple-icon.png"), [System.Drawing.Imaging.ImageFormat]::Png)

$icon16.Dispose()
$icon32.Dispose()
$icon192.Dispose()
$icon512.Dispose()
$apple.Dispose()

# --- Open Graph card: 1200 x 630 -------------------------------------------
$og = New-Object System.Drawing.Bitmap 1200, 630
$g = [System.Drawing.Graphics]::FromImage($og)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
$g.Clear([System.Drawing.ColorTranslator]::FromHtml("#121314"))

$glow = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(16, 255, 225, 77))
$g.FillEllipse($glow, 260, -220, 680, 420)

$panel = New-RoundedPath 60 60 1080 530 28
$g.FillPath((Brush "#1f1e1c"), $panel)
$g.DrawPath((Pen "#302e29" 2), $panel)

$markSize = 72
$mark = New-RoundedPath 110 110 $markSize $markSize 18
$g.FillPath((Brush "#ffe14d"), $mark)
$markW = $markSize * 0.579
$markH = $markSize * 0.670
Draw-W $g (Pen "#121314" ($markSize * 0.085)) `
  (110 + (($markSize - $markW) / 2)) (110 + (($markSize - $markH) / 2)) $markW $markH

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
$g.DrawString("deployer.webiko.my.id", $urlFont, (Brush "#ffe14d"), 112, 540)

$footFont = New-Object System.Drawing.Font "Segoe UI", 22, ([System.Drawing.FontStyle]::Regular), ([System.Drawing.GraphicsUnit]::Pixel)
$g.DrawString("tanpa akun Vercel sendiri", $footFont, (Brush "#a39d90"), 790, 542)

$og.Save((Join-Path $web "og.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$og.Dispose()

Write-Output "assets written: favicon.ico, icon-32, icon-192, icon-512, apple-icon.png, og.png (icon.svg is hand-maintained)"
