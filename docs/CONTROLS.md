# Using image9

[Download and overview](../README.md) | [Developer handoff](HANDOFF.md)

## Controls

Images share a spatial canvas at their original relative sizes. They start in a horizontal strip; a draggable right-hand boundary can wrap them into rows. Opening or dropping files appends them in selection order. The fixed-width details area shows the selected image's dimensions and file size, plus the workspace zoom. Click an image to select it.

- **Open / Ctrl+O:** append one or more images. You can also drop files onto the window or pass multiple paths on the command line. Images appear as they finish background loading; further drops queue behind existing loads.
- Opening files through another launch sends them to the existing viewer and appends them to its workspace.
- Newly added images are selected and centred as they finish loading. Zoom is reduced if needed to show the whole image.
- Dragged and dropped images are appended without changing selection or moving the existing view.
- **U (also while holding Ctrl):** unset the focused image. In Inspect Mode, the focused image has a white outline; other images have blue outlines.
- **W (also while holding Ctrl):** enter wrap placement mode, preserving any existing limit until you left-click to set the width. The preview snaps to 8 image pixels. Press W again while placing to clear the limit and return to a single row; Escape cancels placement without changing the limit.
- In wrap mode, type a pixel width and press Enter to apply it exactly (0 clears wrapping). Backspace edits; invalid input or Escape preserves the previous width. Left-click placement continues to use 8px snapping.
- Gap and wrap inputs accept calculations with `+`, `-`, `*`, `/`, `%` and parentheses. A leading operator uses the current setting, e.g. `*2` doubles it or `-8` subtracts 8. Results round to the nearest pixel; invalid calculations leave settings unchanged.
- Use `$1`, `$2`, etc. in gap or wrap formulas for image widths in the current workspace order, e.g. `$1+$2+16`. Missing or not-yet-loaded image references leave the setting unchanged.
- **Left / Right:** select and centre the previous/next workspace image at the current zoom. Navigation wraps.
- **Up / Down:** select and centre the closest image in the row above or below, keeping the current zoom. Stops at the first or last row.
- All four arrow keys also navigate while Ctrl is held. The focused image's white outline is drawn above blue image outlines.
- **File → Clear workspace / Ctrl+N:** remove all images and cancel pending loads.
- **C:** copy the focused image's full path. With no focus, reports "No focus target, nothing copied" without changing the clipboard.
- **Enter in Inspect Mode:** copy the selected image's full path. Enter still submits gap or wrap calculations in Input Mode.
- **Ctrl+C in normal mode:** clear the workspace. In persistent Inspect Mode (I), Ctrl+C copies the focused image's path. C alone always copies the focused path.
- **Hold Ctrl for the wrap bar at the right (gray until a limit is set):** click and drag it left/right to set the workspace's wrapping width, snapping every 8 image pixels. Images flow left to right into new rows, with consistent gaps. Images wider than the limit occupy their own row without being resized. **View → Layout → Set / reset wrap limit** provides the same W-mode control. Clearing the workspace also resets the boundary.
- **T (or H outside persistent Inspect Mode):** toggle filename, pixel dimensions, and file-size labels below each image, left-aligned. Labels start hidden and are visible only in Inspect Mode. All labels appear above the images; hovering an image's bounds while Ctrl is held brings its label above the other labels. Labels are hidden when Ctrl is released.
- **Escape:** return keyboard focus to the image from the toolbar.
- Keyboard shortcuts hide the cursor until the mouse moves; Ctrl alone leaves it visible. The cursor is also restored when switching away from the viewer.
- **Q:** close the viewer.
- **Mouse wheel:** zoom the workspace at the cursor position, up to 3200%. Very small zoom levels allow fitting large image strips. The **+ / − buttons** zoom at the centre of the view.
- **View → Zoom → Zoom to cursor:** toggle mouse-wheel zoom between the cursor and the view centre. Starts enabled; keyboard and toolbar zoom stay centred.
- **+ / = / numpad +:** zoom in at the view centre. **− / _ / numpad −:** zoom out at the view centre, independent of the cursor.
- **Drag with the left mouse button:** pan.
- **Middle-mouse drag:** pan while the eyedropper is active, without leaving picking mode. Also works in normal viewing mode.
- **Hold Ctrl:** show blue outlines around each image's bounds.
- **I:** enter Inspect Mode without holding Ctrl, including path copying, image removal, labels, and wrap controls. Escape exits this mode.
- **H / J / K / L in I mode:** navigate left / down / up / right. Outside I mode, H toggles labels and L opens image order as usual. T toggles labels in either mode.
- **Hold Ctrl:** show left-aligned shortcut hints above workspace origin (0, 0), including when empty.
- **H (also while holding Ctrl):** toggle filename labels in Inspect Mode. Labels start hidden. The H reminder, outlines, and wrap bar remain visible; the choice lasts until the viewer closes.
- Action confirmations appear above the bottom-right controls for 2.5 seconds. A new action replaces the message and restarts its timer.
- **Ctrl + left-click an image:** copy its full file path to the clipboard.
- **Ctrl + right-click an image:** remove it from the workspace immediately, without a prompt. The file on disk is unchanged; remaining images close the gap.
- **Right-click:** toggle colour picking mode. Ctrl + right-click keeps its remove-image action.
- **O:** open the selected image's containing folder in Explorer.
- **L:** open the image list, styled to match dark mode. Move Up / Move Down reorder images immediately; Sort alphabetically orders filenames A to Z without case sensitivity; Sort by file size orders them smallest first. OK keeps the order. Cancel, Escape, or closing the window restores the opening order, preserving any images added while the dialog was open.
- **G:** enter image gap setting mode. Type a nonnegative pixel count and press Enter; empty input sets 0. Backspace edits the number; A second G immediately applies 0 if the current gap is nonzero, or the default 32 pixels if it is already 0, then exits gap mode. Escape, invalid input, or leaving the canvas cancels without changes. The gap applies between images and between wrapped rows.
- **F / Fit:** fit the entire workspace inside the window, without enlarging small images.
- **R:** frame the selected image. If it is already framed, fit all images instead. Panning or zooming away lets R frame the selected image again.
- **E / 100%:** display at one image pixel per screen pixel and centre the selected image.
- **1 / 2 / 3 / 4 (including numpad with Num Lock on):** choose charcoal, soft white, soft black, or green (`#2E7D32`) background, respectively.
- **0 (also numpad):** reset the background to the default charcoal.
- **View → Background:** contains the same four background presets.
- **View → Source colour space:** manually interpret file pixels as sRGB (default), Display P3, Adobe RGB (1998), or SDR Rec.2020. Conversion reloads the workspace images in the background while preserving the view. The override applies to all images, but resets to sRGB on restart. These are input interpretations, not creative looks or full colour management: embedded ICC and monitor profiles are not read, output remains 8-bit SDR sRGB, and out-of-gamut colours are clipped. The eyedropper samples the converted sRGB pixels. HDR/PQ/HLG and ACES are not supported.
- **Right-click / Pick colour:** activate the eyedropper and move over the image to preview the pixel colour live, as a hex value and colour swatch. Click to copy it to the clipboard. Press Escape or right-click to cancel. Samples the original image pixel at the current pan/zoom, independent of interpolation. Transparent pixels include alpha as `#RRGGBBAA`; RGB is reconstructed from premultiplied pixels and may be rounded.
- **View → Copy colour format:** choose Hex, RGB, or HSL for the clipboard. Transparent colours use alpha hex, `rgba()`, or `hsla()`. The live preview stays in hex. Your format choice is remembered.
- **Linear: On / Off:** toggle bilinear interpolation (smooth scaling) or nearest-neighbor scaling (crisp pixels). Starts disabled and stays selected while browsing images.
- **Colour swatches:** the four small colour buttons in the bottom toolbar choose charcoal (`#1E1E21`), soft white (`#EEEEEE`), soft black (`#121212`), or green (`#2E7D32`). Hover for a description. Also applies behind transparent images and stays selected while browsing.
- **View → Dark mode:** switch the interface between dark and light. Starts with the Windows app theme preference, then remembers your choice in `%APPDATA%\image9\theme.txt`. Image background swatches are independent of the interface theme. The File/View menu bar also follows the theme. System dialogs may retain Windows styling; native dark controls require Windows 10 1903 or newer.

## Supported files

Supports JPEG, PNG, GIF, BMP, TIFF, and WebP. Animated and multipage images display their first frame/page. EXIF orientation and color profile management are not implemented. No image files are modified.
