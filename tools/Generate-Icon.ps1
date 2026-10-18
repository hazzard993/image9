$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$assetDirectory = Join-Path (Split-Path $PSScriptRoot -Parent) 'assets'
[IO.Directory]::CreateDirectory($assetDirectory) | Out-Null

# Three opaque, borderless cards fanned from a shared lower-left grip.
$frames = @(
    @{ X=34; Y=182; Width=80; Height=150; Angle=-5; Colour='#0A2463' },
    @{ X=34; Y=182; Width=80; Height=150; Angle=20; Colour='#FB3640' },
    @{ X=34; Y=182; Width=80; Height=150; Angle=45; Colour='#605F5E' }
)
$svgParts = @('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" fill="none" stroke-linejoin="round">')
foreach ($frame in $frames) {
    $svgParts += '<g transform="translate({0} {1}) rotate({2})"><rect x="0" y="-{3}" width="{4}" height="{3}" fill="{5}"/></g>' -f $frame.X,$frame.Y,$frame.Angle,$frame.Height,$frame.Width,$frame.Colour
}
$svgParts += '</svg>'
[IO.File]::WriteAllText((Join-Path $assetDirectory 'image9.svg'), ($svgParts -join "`n"))

$sizes = @(16,20,24,32,40,48,64,128,256)
$pngFrames = @()
foreach ($size in $sizes) {
    $bitmap = New-Object System.Drawing.Bitmap ($size*4),($size*4)
    $graphics = [Drawing.Graphics]::FromImage($bitmap)
    try {
        $graphics.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::AntiAlias
        $graphics.Clear([Drawing.Color]::Transparent)
        $graphics.ScaleTransform(($size*4/256.0),($size*4/256.0))
        foreach ($frame in $frames) {
            $state = $graphics.Save()
            $graphics.TranslateTransform($frame.X,$frame.Y)
            $graphics.RotateTransform($frame.Angle)
            $fill = New-Object Drawing.SolidBrush ([Drawing.ColorTranslator]::FromHtml($frame.Colour))
            try {
                $graphics.FillRectangle($fill, 0, -$frame.Height, $frame.Width, $frame.Height)
            } finally { $fill.Dispose(); $graphics.Restore($state) }
        }
        $output = New-Object Drawing.Bitmap $size,$size
        $resampler = [Drawing.Graphics]::FromImage($output)
        $stream = New-Object IO.MemoryStream
        try {
            $resampler.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $resampler.DrawImage($bitmap,0,0,$size,$size)
            $output.Save($stream,[Drawing.Imaging.ImageFormat]::Png)
            $pngFrames += ,$stream.ToArray()
            if ($size -eq 256) { [IO.File]::WriteAllBytes((Join-Path $assetDirectory 'image9.png'),$stream.ToArray()) }
        } finally { $stream.Dispose(); $resampler.Dispose(); $output.Dispose() }
    } finally { $graphics.Dispose(); $bitmap.Dispose() }
}
$iconStream = New-Object IO.MemoryStream
$writer = New-Object IO.BinaryWriter $iconStream
try {
    $writer.Write([uint16]0); $writer.Write([uint16]1); $writer.Write([uint16]$sizes.Count)
    $offset = 6 + 16*$sizes.Count
    for ($i=0; $i -lt $sizes.Count; $i++) {
        $dimension = if ($sizes[$i] -eq 256) { 0 } else { $sizes[$i] }
        $writer.Write([byte]$dimension); $writer.Write([byte]$dimension)
        $writer.Write([byte]0); $writer.Write([byte]0)
        $writer.Write([uint16]1); $writer.Write([uint16]32)
        $writer.Write([uint32]$pngFrames[$i].Length); $writer.Write([uint32]$offset)
        $offset += $pngFrames[$i].Length
    }
    foreach ($frame in $pngFrames) { $writer.Write([byte[]]$frame) }
    [IO.File]::WriteAllBytes((Join-Path $assetDirectory 'image9.ico'),$iconStream.ToArray())
} finally { $writer.Dispose(); $iconStream.Dispose() }
Write-Host 'Generated assets/image9.svg, image9.png and image9.ico'
