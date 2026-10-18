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
- **Escape:** return keyboard focus to the image from the toolbar.
- **Q:** close the viewer.
- **Mouse wheel / + / − buttons:** zoom, between 1% and 3200%.
- **Drag with the left mouse button:** pan.
- **Hold Ctrl:** show a blue outline around the image bounds.
- **F / R / Fit:** reset zoom to fit inside the window and centre the image, without enlarging small images.
- **1 / 100%:** display at one image pixel per screen pixel.
- **Linear: On / Off:** toggle bilinear interpolation (smooth scaling) or nearest-neighbor scaling (crisp pixels). Starts disabled and stays selected while browsing images.
- **Colour swatches:** the three small colour buttons in the bottom toolbar choose charcoal (`#1E1E21`), soft white (`#EEEEEE`), or soft black (`#121212`). Hover for a description. Also applies behind transparent images and stays selected while browsing.
- **View → Dark mode:** switch the interface between dark and light. Starts with the Windows app theme preference, then remembers your choice in `%APPDATA%\QuickImageViewer\theme.txt`. Image background swatches are independent of the interface theme. The File/View menu bar also follows the theme. System dialogs may retain Windows styling; native dark controls require Windows 10 1903 or newer.

Supports JPEG, PNG, GIF, BMP, TIFF, and WebP. Animated and multipage images display their first frame/page. EXIF orientation and color profile management are not implemented. No image files are modified.

Image rendering uses Direct2D hardware acceleration. Images upload at their original resolution once; panning, zooming, and interpolation reuse GPU textures without CPU resampling. Large images are split into textures with sampling gutters to support images larger than the GPU texture limit. Direct2D handles frame presentation and recreates device resources when needed. If hardware rendering is unavailable, it uses Direct2D's software renderer; hover over the image details to see which renderer is active.

Files decode and convert to premultiplied BGRA in a single background worker. The current image stays visible while loading. Rapid navigation replaces queued requests, and stale results cannot overwrite the latest image. The UI thread handles texture uploads, which can still take a moment for exceptionally large files. Original pixels are retained in RAM for device recovery; memory usage scales with image size.

## Development

```powershell
go test ./...
go vet ./...
```

Run the build script before using `go run .` so that the required common-controls manifest is embedded. Source: `main.go`; application manifest: `app.manifest`.
