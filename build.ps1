$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -arch amd64 -o resource_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Manifest generation failed' }
    go build -trimpath -ldflags '-s -w -H=windowsgui' -o dist/QuickImageViewer.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    Write-Host 'Built dist/QuickImageViewer.exe'
} finally {
    Pop-Location
}
