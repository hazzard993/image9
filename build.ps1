$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -ico assets/image9.ico -arch amd64 -o resource_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw 'Manifest generation failed' }
    go build -trimpath -ldflags '-s -w -H=windowsgui' -o dist/image9.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    Write-Host 'Built dist/image9.exe'
} finally {
    Pop-Location
}
