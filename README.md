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

The fixed-width details area at the bottom left shows image dimensions, zoom, and the file's size on disk (B, KiB, MiB, etc.).

- **Open / Ctrl+O:** choose an image. You can also drop a file onto the window or pass its path on the command line.
- **Left / Right:** previous or next supported image in the same folder, sorted alphabetically (case insensitive). Navigation wraps.
- **Escape:** return keyboard focus to the image from the toolbar.
- **Q:** close the viewer.
- **Mouse wheel:** zoom at the cursor position, between 1% and 3200%. The **+ / − buttons** zoom at the centre of the view.
- **+ / = / numpad +:** zoom in at the view centre. **− / _ / numpad −:** zoom out at the view centre, independent of the cursor.
- **Drag with the left mouse button:** pan.
- **Middle-mouse drag:** pan while the eyedropper is active, without leaving picking mode. Also works in normal viewing mode.
- **Hold Ctrl:** show a blue outline around the image bounds.
- **F / R / Fit:** reset zoom to fit inside the window and centre the image, without enlarging small images.
- **E / 100%:** display at one image pixel per screen pixel.
- **1 / 2 / 3:** choose charcoal, soft white, or soft black background, respectively.
- **View → Background:** contains the same three background presets.
- **View → Source colour space:** manually interpret file pixels as sRGB (default), Display P3, Adobe RGB (1998), or SDR Rec.2020. Conversion runs in the background and preserves pan/zoom when changing the current image. The override remains active while browsing, but resets to sRGB on restart. These are input interpretations, not creative looks or full colour management: embedded ICC and monitor profiles are not read, output remains 8-bit SDR sRGB, and out-of-gamut colours are clipped. The eyedropper samples the converted sRGB pixels. HDR/PQ/HLG and ACES are not supported.
- **C / Pick colour:** activate the eyedropper and move over the image to preview the pixel colour live, as a hex value and colour swatch. Click to copy it to the clipboard. Press Escape or C to cancel. Samples the original image pixel at the current pan/zoom, independent of interpolation. Transparent pixels include alpha as `#RRGGBBAA`; RGB is reconstructed from premultiplied pixels and may be rounded.
- **View → Copy colour format:** choose Hex, RGB, or HSL for the clipboard. Transparent colours use alpha hex, `rgba()`, or `hsla()`. The live preview stays in hex. Your format choice is remembered.
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
