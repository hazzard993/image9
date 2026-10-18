# image9

A small, native Windows image viewer with GPU rendering and a spatial workspace for multiple images.

## Download

[Download image9.exe for Windows x64](https://github.com/hazzard993/image9/releases/latest/download/image9.exe), then run it. No installation or build tools are required. Supports Windows 10 and 11.

[Release notes and checksums](https://github.com/hazzard993/image9/releases).

## Getting started

Use **Open** or drag images into the window. Pan with a left or middle mouse drag and zoom with the mouse wheel. Opening files through another launch adds them to the existing viewer.

| Shortcut | Action |
| --- | --- |
| Ctrl+O | Open images |
| Arrow keys | Select and centre an image |
| F / R / E | Fit all / frame selected / actual size |
| Hold Ctrl / I | Inspect Mode; Escape exits persistent mode |
| G / W | Set image gap / wrap width |
| L | Image list and ordering, outside persistent Inspect Mode |
| C | Copy the selected image's full path |
| Ctrl+N | Clear workspace |

Supports JPEG, PNG, GIF, BMP, TIFF, and WebP. Animated and multipage files display their first frame or page. Image files are never modified.

See the [full controls and settings guide](docs/CONTROLS.md) for Inspect Mode, colour picking, background presets, input formulas, and limitations.

## Development

See the [developer handoff](docs/HANDOFF.md) for the source layout, build and test commands, dependencies, Windows implementation details, and automated releases.
