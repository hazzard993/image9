# Quick Image Viewer

A small, native Windows image viewer written in Go using Walk. No browser runtime or CGO toolchain required.

## Build and run

Requires Windows 10/11 x64 and Go 1.25.5 or newer. From PowerShell:

```powershell
.\build.ps1
.\dist\QuickImageViewer.exe
# Or open a file directly:
.\dist\QuickImageViewer.exe "C:\Pictures\photo.jpg"
```

The build script embeds the Windows application manifest and creates a standalone GUI executable. The first build downloads Go dependencies and the pinned resource compiler. Copy the executable anywhere to use it.

## Controls

- **Open / Ctrl+O:** choose an image. You can also drop a file onto the window or pass its path on the command line.
- **Left / Right:** previous or next supported image in the same folder, sorted alphabetically (case insensitive). Navigation wraps.
- **Mouse wheel / + / − buttons:** zoom, between 1% and 3200%.
- **Drag with the left mouse button:** pan.
- **F / Fit:** fit inside the window, without enlarging small images.
- **1 / 100%:** display at one image pixel per screen pixel.
- **Linear: On / Off:** toggle bilinear interpolation (smooth scaling) or nearest-neighbor scaling (crisp pixels). Starts disabled and stays selected while browsing images.
- **Background — Current / White / Black:** choose the original charcoal (`#1E1E21`), soft white (`#EEEEEE`), or soft black (`#121212`). Also applies behind transparent images and stays selected while browsing.

Supports JPEG, PNG, GIF, BMP, TIFF, and WebP. Animated and multipage images display their first frame/page. EXIF orientation and color profile management are not implemented. Images decode on demand; very large files may briefly pause the interface. No image files are modified.

## Development

```powershell
go test ./...
go vet ./...
```

Run the build script before using `go run .` so that the required common-controls manifest is embedded. Source: `main.go`; application manifest: `app.manifest`.
