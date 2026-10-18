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

Images share a spatial canvas, arranged at their original relative sizes in a top-aligned horizontal strip with gaps. Opening or dropping files appends them on the right in selection order. The fixed-width details area shows the selected image's dimensions and file size, plus the workspace zoom. Click an image to select it.

- **Open / Ctrl+O:** append one or more images. You can also drop files onto the window or pass multiple paths on the command line. Images appear as they finish background loading; further drops queue behind existing loads.
- **Left / Right:** select and centre the previous/next workspace image at the current zoom. Navigation wraps.
- **File → Clear workspace / Ctrl+N:** remove all images and cancel pending loads.
- **Escape:** return keyboard focus to the image from the toolbar.
- **Q:** close the viewer.
- **Mouse wheel:** zoom the workspace at the cursor position, up to 3200%. Very small zoom levels allow fitting large image strips. The **+ / − buttons** zoom at the centre of the view.
- **+ / = / numpad +:** zoom in at the view centre. **− / _ / numpad −:** zoom out at the view centre, independent of the cursor.
- **Drag with the left mouse button:** pan.
- **Middle-mouse drag:** pan while the eyedropper is active, without leaving picking mode. Also works in normal viewing mode.
- **Hold Ctrl:** show blue outlines around each image's bounds.
- **F / R / Fit:** fit the entire workspace inside the window, without enlarging small images. Fit mode automatically reframes when more images finish loading; manual pan/zoom keeps the existing view stable when images are appended.
- **E / 100%:** display at one image pixel per screen pixel and centre the selected image.
- **1 / 2 / 3:** choose charcoal, soft white, or soft black background, respectively.
- **View → Background:** contains the same three background presets.
- **View → Source colour space:** manually interpret file pixels as sRGB (default), Display P3, Adobe RGB (1998), or SDR Rec.2020. Conversion reloads the workspace images in the background while preserving the view. The override applies to all images, but resets to sRGB on restart. These are input interpretations, not creative looks or full colour management: embedded ICC and monitor profiles are not read, output remains 8-bit SDR sRGB, and out-of-gamut colours are clipped. The eyedropper samples the converted sRGB pixels. HDR/PQ/HLG and ACES are not supported.
- **C / Pick colour:** activate the eyedropper and move over the image to preview the pixel colour live, as a hex value and colour swatch. Click to copy it to the clipboard. Press Escape or C to cancel. Samples the original image pixel at the current pan/zoom, independent of interpolation. Transparent pixels include alpha as `#RRGGBBAA`; RGB is reconstructed from premultiplied pixels and may be rounded.
- **View → Copy colour format:** choose Hex, RGB, or HSL for the clipboard. Transparent colours use alpha hex, `rgba()`, or `hsla()`. The live preview stays in hex. Your format choice is remembered.
- **Linear: On / Off:** toggle bilinear interpolation (smooth scaling) or nearest-neighbor scaling (crisp pixels). Starts disabled and stays selected while browsing images.
- **Colour swatches:** the three small colour buttons in the bottom toolbar choose charcoal (`#1E1E21`), soft white (`#EEEEEE`), or soft black (`#121212`). Hover for a description. Also applies behind transparent images and stays selected while browsing.
- **View → Dark mode:** switch the interface between dark and light. Starts with the Windows app theme preference, then remembers your choice in `%APPDATA%\QuickImageViewer\theme.txt`. Image background swatches are independent of the interface theme. The File/View menu bar also follows the theme. System dialogs may retain Windows styling; native dark controls require Windows 10 1903 or newer.

Supports JPEG, PNG, GIF, BMP, TIFF, and WebP. Animated and multipage images display their first frame/page. EXIF orientation and color profile management are not implemented. No image files are modified.

Image rendering uses Direct2D hardware acceleration. Images upload at their original resolution once; panning, zooming, and interpolation reuse GPU textures without CPU resampling. Large images are split into textures with sampling gutters to support images larger than the GPU texture limit. Direct2D handles frame presentation and recreates device resources when needed. If hardware rendering is unavailable, it uses Direct2D's software renderer; hover over the image details to see which renderer is active.

Files decode and convert to premultiplied BGRA in a single background worker. Loaded images stay visible while further images load. Visible images upload to Direct2D on demand; their textures are retained for fast panning, and clearing the workspace releases them. The UI thread handles texture uploads, which can still take a moment for exceptionally large files. Pixels are retained in RAM for sampling and device recovery; memory usage scales with the combined size of opened images. The workspace is not saved between launches.

## Development

```powershell
go test ./...
go vet ./...
```

Run the build script before using `go run .` so that the required common-controls manifest is embedded. Source: `main.go`; application manifest: `app.manifest`.
