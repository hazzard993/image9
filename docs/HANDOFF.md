# image9 developer handoff

[README](../README.md) | [Controls and settings](CONTROLS.md)

## Build and verify

Develop on Windows 10/11 x64 with Go 1.26.9 or newer. The application uses native Walk controls and Direct2D; no browser runtime or CGO toolchain is required. Run from the repository root in PowerShell:

```powershell
.\build.ps1
go test -timeout 3m ./...
go vet ./...
.\dist\image9.exe
# Or open files directly:
.\dist\image9.exe "C:\Pictures\photo.jpg"
```

Run the build script before tests or `go run ./src`. It uses the pinned resource compiler to generate `src/resource_windows_amd64.syso`, embedding the common-controls manifest and icon. Native tests depend on these resources, including icon group ID 2. Generated resources and `dist/` are ignored by Git.

## Repository layout

| Location | Purpose |
| --- | --- |
| `src/main.go` | Main window, toolbar, menus, shortcuts, camera and zoom |
| `src/workspace*.go` | Workspace layout, selection, removal and overlay state |
| `src/loader.go` | Background decoding and UI-thread completion pump |
| `src/renderer*_windows_amd64.go` | Direct2D/DirectWrite COM bindings, textures, labels and overlays |
| `src/gap.go`, `src/wrap_input.go`, `src/pixel_expression.go` | Captured input, formulas and image-width references |
| `src/image_list.go` | Reordering, sorting and Cancel restoration |
| `src/instance_windows.go` | Single-instance mutex and image-path forwarding |
| `src/theme*.go`, `src/buttons.go` | Native themes and immediate button painting |
| `src/keyboard_cursor.go`, `src/mode.go`, `src/feedback.go` | Cursor hiding, mode labels and action messages |
| `src/colour_*.go` | Colour-space conversion and clipboard colour formats |
| `assets/`, `tools/Generate-Icon.ps1` | Icon source, generated assets and regeneration script |
| `.github/workflows/release.yml` | Windows checks and tag-triggered releases |
| `go.mod`, `go.sum` | Root module requirements and checksums |

## Rendering and loading

Direct2D uses hardware acceleration with a software fallback. Hover over image details to see the active renderer. Visible images upload on demand and retain their GPU textures; panning, zooming and interpolation reuse those textures without CPU resampling. Large images use tiled textures with sampling gutters. Device loss recreates resources from retained pixels.

Decoding and premultiplied BGRA conversion run in a single background worker. Workspace scheduling queues loads rather than cancelling earlier drops. UI updates, COM rendering and texture uploads stay on the UI thread. Uploads can still take time for exceptionally large files. CPU pixels remain available for colour sampling and device recovery, so memory use scales with the loaded image sizes. Clearing releases images; workspaces are not saved between launches.

Draw image textures first, blue outlines next, then the selected white outline. Filename labels draw above images, with the hovered label last among labels. Shortcut text is left-aligned above workspace origin (0, 0), including in an empty workspace. Mode and action text share the bottom line, left- and right-aligned respectively, with no backgrounds; overlap is allowed.

## Input and state details

- Holding Ctrl temporarily enables Inspect Mode; I keeps it active until Escape. In persistent mode, H/J/K/L navigate, T toggles labels, and L opens the list only after leaving that mode.
- C copies a focused path. Ctrl+C clears the workspace unless persistent Inspect Mode is active. Ctrl+N and Clear always clear it. Enter copies a selected path in Inspect Mode but submits values in Input Mode.
- Gap and wrap entry use a UI-thread `WH_GETMESSAGE` hook before Walk shortcuts/dialog navigation. Release hooks when entry ends, loses focus or the window closes. Consume queued Enter characters after mode exit to avoid the Windows beep.
- Formulas support arithmetic, parentheses, current-value shorthand and one-based image-width references such as `$1+$2+16`. References follow current order. Invalid input leaves settings unchanged; results round to whole pixels.
- Gap changes adjust wrap width by the gap delta times the horizontal gap count in the fullest row. Typed wrap widths apply exactly; mouse placement snaps every 8 image pixels.
- Opening files selects/centres them after decoding. Drag/drop appends without taking selection or moving the existing camera, and reports successful loads per batch.
- Stable workspace IDs preserve focused and in-flight loading targets across reorder/sort/Cancel. Cancel restores the opening order while retaining newly added images and decoded data.
- Single-instance forwarding uses a session-local mutex and bounded `WM_COPYDATA` messages with absolute paths. Keep UI-thread receiving and startup readiness registration in sync.
- Cursor hiding uses balanced calls and restores on actual mouse movement, deactivation and shutdown. Ctrl alone must not hide the cursor, including during captured input.

## Preferences and limitations

The Windows app theme is used initially. Preferences live in `%APPDATA%\image9\theme.txt` and `colour-format.txt`. Reads fall back to the former `%APPDATA%\QuickImageViewer\` location; writes use image9. Background, layout and source colour interpretation are session state.

Source colour overrides interpret sRGB, Display P3, Adobe RGB (1998) or SDR Rec.2020 and convert to 8-bit SDR sRGB. Embedded ICC/monitor profiles, HDR/PQ/HLG, ACES and EXIF orientation are unsupported. Out-of-gamut values are clipped. The eyedropper samples converted original pixels independently of display interpolation; reconstructing RGB from premultiplied alpha may round values. Animated/multipage files show their first frame/page.

## Dependencies

Requirements are pinned in `go.mod`/`go.sum`. This update uses `golang.org/x/image v0.47.0` and `golang.org/x/sys v0.49.0`, which require Go 1.26; the module requires the maintained Go 1.26.9 patch. Walk, its Win32 bindings, govaluate and the resource compiler already use their latest available upstream revisions.

```powershell
go list -m -u all
go get -u ./...
go mod tidy
.\build.ps1
go test -timeout 3m ./...
go vet ./...
```

Check minimum Go requirements before upgrading. Actions reads the Go version from `go.mod`; the resource compiler is pinned separately in `build.ps1`. Native tests exercise hidden controls, Direct2D/WIC pixels, theme painting, input capture and cross-process forwarding. Run them on Windows rather than replacing them with compilation-only checks.

## Icon

The icon is three opaque, borderless cards fanned from the lower-left: `#0A2463`, `#FB3640` and `#605F5E`, back to front, on a transparent background. SVG, PNG and ICO outputs share a generator; ICO sizes range from 16px to 256px.

```powershell
.\tools\Generate-Icon.ps1
.\build.ps1
```

The icon is embedded and explicitly loaded for the window/taskbar. No loose icon file is needed beside the executable.

## Releases

`main` is the default branch. Pushes to main build/test without publishing. A pushed `vMAJOR.MINOR.PATCH` tag releases only when its commit belongs to main. The workflow builds on Windows, embeds resources, runs tests/vet, uploads `image9.exe` and its SHA-256 checksum, then publishes the draft.

Use an unused version; do not move a published tag. For example, when v0.1.2 is next:

```powershell
git switch main
git pull --ff-only
git tag -a v0.1.2 -m "image9 v0.1.2"
git push origin v0.1.2
```

A manual workflow run only builds/tests. Verify the tagged run and published executable download before reporting a release complete. Asset reruns replace downloads for that tag; avoid changing published tags.
