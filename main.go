//go:build windows

package main

import (
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type viewer struct {
	window              *walk.MainWindow
	canvas              *walk.CustomWidget
	footer              *walk.Composite
	status              *walk.Label
	renderer            *gpuRenderer
	rendererLabel       string
	loader              *imageLoader
	loadID              uint64
	pendingPath         string
	loading             bool
	highlightBounds     bool
	source              *decodedImage
	linear              bool
	interpolationButton *walk.PushButton
	palette             *walk.ImageView
	backgroundIndex     int
	colourPicking       bool
	colourPickerButton  *walk.PushButton
	colourPreview       *walk.ImageView
	previewColour       walk.Color
	copyFormat          colourFormat
	copyFormatActions   [3]*walk.Action
	sourceColourSpace   sourceColourSpace
	colourSpaceActions  [4]*walk.Action
	preserveView        bool
	dragButton          walk.MouseButton
	dark                bool
	themeBrush          *walk.SolidColorBrush
	files               []string
	path                string
	width, height       int
	scale, panX, panY   float64
	fit, dragging       bool
	lastX, lastY        int
}

func main() {
	v := &viewer{fit: true, scale: 1, dark: loadDarkMode(), copyFormat: loadCopyFormat()}
	setNativeAppTheme(v.dark)
	err := (MainWindow{
		AssignTo: &v.window, Title: "Quick Image Viewer", Size: Size{Width: 1000, Height: 740}, MinSize: Size{Width: 800, Height: 320},
		Layout: VBox{MarginsZero: true, SpacingZero: true},
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "&Open…", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyO}, OnTriggered: v.openDialog},
				Action{Text: "E&xit", Shortcut: Shortcut{Key: walk.KeyQ}, OnTriggered: func() { v.window.Close() }},
			}},
			Menu{Text: "&View", Items: []MenuItem{
				Action{Text: "Previous", Shortcut: Shortcut{Key: walk.KeyLeft}, OnTriggered: func() { v.navigate(-1) }},
				Action{Text: "Next", Shortcut: Shortcut{Key: walk.KeyRight}, OnTriggered: func() { v.navigate(1) }},
				Menu{Text: "Zoom", Items: []MenuItem{
					Action{Text: "Zoom in", Shortcut: Shortcut{Key: walk.KeyOEMPlus}, OnTriggered: func() { v.zoom(1.25) }},
					Action{Text: "Zoom out", Shortcut: Shortcut{Key: walk.KeyOEMMinus}, OnTriggered: func() { v.zoom(1 / 1.25) }},
					Action{Text: "Fit to window", Shortcut: Shortcut{Key: walk.KeyF}, OnTriggered: v.fitImage},
					Action{Text: "Reset zoom", Shortcut: Shortcut{Key: walk.KeyR}, OnTriggered: v.fitImage},
					Action{Text: "Actual size (100%)", Shortcut: Shortcut{Key: walk.KeyE}, OnTriggered: v.actualSize},
				}},
				Menu{Text: "Background", Items: []MenuItem{
					Action{Text: "Charcoal", Shortcut: Shortcut{Key: walk.Key1}, OnTriggered: func() { v.selectBackground(0) }},
					Action{Text: "Soft white", Shortcut: Shortcut{Key: walk.Key2}, OnTriggered: func() { v.selectBackground(1) }},
					Action{Text: "Soft black", Shortcut: Shortcut{Key: walk.Key3}, OnTriggered: func() { v.selectBackground(2) }},
				}},
				Menu{Text: "Source colour space", Items: []MenuItem{
					Action{AssignTo: &v.colourSpaceActions[0], Text: "sRGB (default)", Checkable: true, Checked: true, OnTriggered: func() { v.setSourceColourSpace(spaceSRGB) }},
					Action{AssignTo: &v.colourSpaceActions[1], Text: "Display P3", Checkable: true, OnTriggered: func() { v.setSourceColourSpace(spaceP3) }},
					Action{AssignTo: &v.colourSpaceActions[2], Text: "Adobe RGB (1998)", Checkable: true, OnTriggered: func() { v.setSourceColourSpace(spaceAdobeRGB) }},
					Action{AssignTo: &v.colourSpaceActions[3], Text: "Rec.2020 (SDR)", Checkable: true, OnTriggered: func() { v.setSourceColourSpace(spaceRec2020) }},
				}},
				Action{Text: "Pick colour from image", Shortcut: Shortcut{Key: walk.KeyC}, OnTriggered: v.toggleColourPicker},
				Menu{Text: "Copy colour format", Items: []MenuItem{
					Action{AssignTo: &v.copyFormatActions[0], Text: "Hex — #RRGGBB", Checkable: true, Checked: v.copyFormat == formatHex, OnTriggered: func() { v.setCopyFormat(formatHex) }},
					Action{AssignTo: &v.copyFormatActions[1], Text: "RGB — rgb(r, g, b)", Checkable: true, Checked: v.copyFormat == formatRGB, OnTriggered: func() { v.setCopyFormat(formatRGB) }},
					Action{AssignTo: &v.copyFormatActions[2], Text: "HSL — hsl(h, s%, l%)", Checkable: true, Checked: v.copyFormat == formatHSL, OnTriggered: func() { v.setCopyFormat(formatHSL) }},
				}},
				Action{Text: "Focus image", Shortcut: Shortcut{Key: walk.KeyEscape}, OnTriggered: v.focusImage},
				Action{Text: "Dark mode", Checkable: true, Checked: v.dark, OnTriggered: func() {
					v.dark = !v.dark
					v.applyTheme()
					saveDarkMode(v.dark)
				}},
			}},
		},
		Children: []Widget{
			CustomWidget{AssignTo: &v.canvas, Background: SolidColorBrush{Color: walk.RGB(30, 30, 33)},
				Style: win.WS_TABSTOP, PaintMode: PaintNoErase, InvalidatesOnResize: true, PaintPixels: v.paint,
				OnMouseDown: func(x, y int, b walk.MouseButton) {
					if b == walk.MiddleButton {
						v.canvas.SetFocus()
						v.startDrag(x, y, b)
						return
					}
					if b == walk.LeftButton {
						if v.colourPicking {
							v.pickColourAt(x, y)
							return
						}
						v.focusImage()
						v.startDrag(x, y, b)
					}
				},
				OnMouseUp: func(x, y int, b walk.MouseButton) {
					if b == v.dragButton {
						v.stopDrag()
					}
				},
				OnMouseMove: func(x, y int, b walk.MouseButton) {
					if v.dragging && b&v.dragButton == 0 {
						v.stopDrag()
					}
					if v.dragging {
						v.panX += float64(x - v.lastX)
						v.panY += float64(y - v.lastY)
						v.lastX = x
						v.lastY = y
						v.canvas.Invalidate()
					}
					if v.colourPicking {
						v.previewColourAt(x, y)
					}
				},
			},
			Composite{AssignTo: &v.footer, Layout: HBox{Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}, Spacing: 4}, Children: []Widget{
				Label{AssignTo: &v.status, MinSize: Size{Width: 240}, MaxSize: Size{Width: 240}, EllipsisMode: EllipsisEnd, Text: "Open an image • Ctrl+O"},
				HSpacer{},
				PushButton{Text: "Open…", OnClicked: v.openDialog},
				PushButton{Text: "‹ Previous", OnClicked: func() { v.navigate(-1) }},
				PushButton{Text: "Next ›", OnClicked: func() { v.navigate(1) }},
				PushButton{Text: "−", OnClicked: func() { v.zoom(1 / 1.25) }},
				PushButton{Text: "+", OnClicked: func() { v.zoom(1.25) }},
				PushButton{Text: "Fit", OnClicked: v.fitImage},
				PushButton{Text: "100%", OnClicked: v.actualSize},
				PushButton{AssignTo: &v.interpolationButton, Text: "Linear: Off", ToolTipText: "Toggle bilinear smoothing / nearest-neighbor pixels", OnClicked: v.toggleInterpolation},
				v.backgroundPalette(),
				PushButton{AssignTo: &v.colourPickerButton, MinSize: Size{Width: 100}, MaxSize: Size{Width: 100}, Text: "Pick colour", ToolTipText: "Pick a pixel from the image and copy its hex colour (C)", OnClicked: v.toggleColourPicker},
				v.colourPreviewWidget(),
			}},
		},
	}).Create()
	if err != nil {
		walk.MsgBox(nil, "Unable to start", err.Error(), walk.MsgBoxIconError)
		return
	}
	// Form shortcuts run before Windows dialog navigation, which otherwise uses
	// arrow keys to move focus among toolbar buttons.
	v.registerMenuShortcuts(v.window.Menu())
	v.registerZoomAliases()
	v.window.Starting().Attach(v.focusImage)
	v.applyTheme()
	installLoadPump(v)
	v.installBoundsKeys()
	v.canvas.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
		point := win.POINT{X: int32(x), Y: int32(y)}
		if !win.ScreenToClient(v.canvas.Handle(), &point) {
			return
		}
		if walk.MouseWheelEventDelta(b) > 0 {
			v.zoomAt(1.25, float64(point.X), float64(point.Y))
		} else {
			v.zoomAt(1/1.25, float64(point.X), float64(point.Y))
		}
	})
	v.window.DropFiles().Attach(func(paths []string) {
		if len(paths) > 0 {
			v.open(paths[0])
		}
	})
	v.canvas.DropFiles().Attach(func(paths []string) {
		if len(paths) > 0 {
			v.open(paths[0])
		}
	})
	if len(os.Args) > 1 {
		v.open(os.Args[1])
	}
	v.window.Run()
}

func (v *viewer) focusImage() {
	v.stopDrag()
	if v.colourPicking {
		v.colourPicking = false
		v.canvas.SetCursor(walk.CursorArrow())
		v.colourPickerButton.SetText("Pick colour")
	}
	v.canvas.SetFocus()
}

func (v *viewer) registerMenuShortcuts(menu *walk.Menu) {
	for i := 0; i < menu.Actions().Len(); i++ {
		action := menu.Actions().At(i)
		if action.Shortcut().Key != 0 {
			v.window.ShortcutActions().Add(action)
		}
		if child := action.Menu(); child != nil {
			v.registerMenuShortcuts(child)
		}
	}
}

func (v *viewer) registerZoomAliases() {
	for _, key := range []walk.Key{walk.KeyOEMPlus, walk.KeyOEMMinus, walk.KeyAdd, walk.KeySubtract} {
		for _, modifiers := range []walk.Modifiers{0, walk.ModShift} {
			if modifiers == 0 && (key == walk.KeyOEMPlus || key == walk.KeyOEMMinus) {
				continue
			}
			factor := 1.25
			if key == walk.KeyOEMMinus || key == walk.KeySubtract {
				factor = 1 / 1.25
			}
			action := walk.NewAction()
			action.SetShortcut(walk.Shortcut{Modifiers: modifiers, Key: key})
			action.Triggered().Attach(func() { v.zoom(factor) })
			v.window.ShortcutActions().Add(action)
		}
	}
}

func (v *viewer) startDrag(x, y int, button walk.MouseButton) {
	v.dragging = true
	v.dragButton = button
	v.lastX, v.lastY = x, y
	win.SetCapture(v.canvas.Handle())
}

func (v *viewer) stopDrag() {
	if v.dragging {
		win.ReleaseCapture()
	}
	v.dragging = false
	v.dragButton = 0
}

func (v *viewer) toggleColourPicker() {
	if v.colourPicking {
		v.focusImage()
		return
	}
	if v.source == nil {
		return
	}
	v.focusImage()
	v.colourPicking = true
	v.colourPickerButton.SetText("Click image")
	v.canvas.SetCursor(walk.CursorCross())
}

func (v *viewer) colourAt(x, y int) (string, bool) {
	if v.source == nil {
		return "", false
	}
	b := v.canvas.ClientBoundsPixels()
	s := v.displayScale()
	left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	top := math.Round((float64(b.Height)-float64(v.height)*s)/2 + v.panY)
	return imageColourAt(v.source, int(math.Floor((float64(x)-left)/s)), int(math.Floor((float64(y)-top)/s)))
}

func (v *viewer) previewColourAt(x, y int) {
	colour, ok := v.colourAt(x, y)
	if !ok || v.colourPickerButton.Text() == colour {
		return
	}
	v.colourPickerButton.SetText(colour)
	var red, green, blue uint8
	fmt.Sscanf(colour[:7], "#%02x%02x%02x", &red, &green, &blue)
	v.previewColour = walk.RGB(red, green, blue)
	v.colourPreview.Invalidate()
}

func (v *viewer) colourPreviewWidget() ImageView {
	return ImageView{AssignTo: &v.colourPreview, Alignment: AlignHNearVCenter,
		MinSize: Size{Width: 20, Height: 20}, MaxSize: Size{Width: 20, Height: 20}, ToolTipText: "Eyedropper colour preview",
		Image: walk.NewPaintFuncImage(walk.Size{Width: 20, Height: 20}, func(c *walk.Canvas, _ walk.Rectangle) error {
			border, err := walk.NewSolidColorBrush(walk.RGB(128, 128, 128))
			if err != nil {
				return err
			}
			defer border.Dispose()
			fill, err := walk.NewSolidColorBrush(v.previewColour)
			if err != nil {
				return err
			}
			defer fill.Dispose()
			if err := c.FillRectangle(border, walk.Rectangle{Width: 20, Height: 20}); err != nil {
				return err
			}
			return c.FillRectangle(fill, walk.Rectangle{X: 1, Y: 1, Width: 18, Height: 18})
		}),
	}
}

func (v *viewer) pickColourAt(x, y int) {
	colour, ok := v.colourAt(x, y)
	if !ok {
		return
	}
	v.previewColourAt(x, y)
	copied := formatPickedColour(colour, v.copyFormat)
	if err := walk.Clipboard().SetText(copied); err != nil {
		walk.MsgBox(v.window, "Unable to copy colour", err.Error(), walk.MsgBoxIconError)
		return
	}
	v.focusImage()
	v.colourPickerButton.SetText(colour)
	v.colourPickerButton.SetToolTipText("Copied " + copied + " to clipboard. Press C to pick another pixel.")
}

func imageColourAt(im *decodedImage, x, y int) (string, bool) {
	if im == nil || x < 0 || y < 0 || x >= im.width || y >= im.height {
		return "", false
	}
	p := im.pixels[(y*im.width+x)*4:][:4]
	alpha := int(p[3])
	channel := func(value byte) int {
		if alpha == 0 {
			return 0
		}
		return min(255, (int(value)*255+alpha/2)/alpha)
	}
	colour := fmt.Sprintf("#%02X%02X%02X", channel(p[2]), channel(p[1]), channel(p[0]))
	if alpha != 255 {
		colour += fmt.Sprintf("%02X", alpha)
	}
	return colour, true
}

func (v *viewer) openDialog() {
	d := walk.FileDialog{Title: "Open image", Filter: "Images (*.jpg;*.jpeg;*.png;*.gif;*.bmp;*.tif;*.tiff;*.webp)|*.jpg;*.jpeg;*.png;*.gif;*.bmp;*.tif;*.tiff;*.webp|All files (*.*)|*.*", FilePath: v.path}
	if ok, err := d.ShowOpen(v.window); err != nil {
		v.showError(err)
	} else if ok {
		v.open(d.FilePath)
	}
}

func (v *viewer) open(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		v.showError(err)
		return
	}
	v.pendingPath = abs
	v.loading = true
	v.preserveView = false
	v.loadID = v.loader.request(abs, v.sourceColourSpace)
	v.status.SetText("Loading…")
	v.window.SetTitle(filepath.Base(abs) + " — Loading…")
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tif", ".tiff", ".webp":
		return true
	}
	return false
}

func folderImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && supported(e.Name()) {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Slice(paths, func(i, j int) bool { return strings.ToLower(paths[i]) < strings.ToLower(paths[j]) })
	return paths, nil
}

func (v *viewer) navigate(delta int) {
	if len(v.files) < 2 {
		return
	}
	for i, p := range v.files {
		if strings.EqualFold(p, v.pendingPath) {
			v.open(v.files[(i+delta+len(v.files))%len(v.files)])
			return
		}
	}
}

func (v *viewer) fitImage() { v.fit = true; v.panX = 0; v.panY = 0; v.canvas.Invalidate() }
func (v *viewer) actualSize() {
	v.fit = false
	v.scale = 1
	v.panX = 0
	v.panY = 0
	v.canvas.Invalidate()
}
func (v *viewer) zoom(factor float64) {
	b := v.canvas.ClientBoundsPixels()
	v.zoomAt(factor, float64(b.Width)/2, float64(b.Height)/2)
}

func (v *viewer) zoomAt(factor, x, y float64) {
	if v.source == nil {
		return
	}
	before := v.displayScale()
	v.scale = math.Max(0.01, math.Min(32, before*factor))
	v.fit = false
	b := v.canvas.ClientBoundsPixels()
	ratio := v.scale / before
	v.panX = zoomPan(v.panX, x, float64(b.Width)/2, ratio)
	v.panY = zoomPan(v.panY, y, float64(b.Height)/2, ratio)
	v.canvas.Invalidate()
}

// Keep the same image point at the anchor as its distance from the centre scales.
func zoomPan(pan, anchor, centre, ratio float64) float64 {
	return pan*ratio + (anchor-centre)*(1-ratio)
}
func (v *viewer) displayScale() float64 {
	if !v.fit || v.source == nil {
		return v.scale
	}
	b := v.canvas.ClientBoundsPixels()
	return math.Max(0.001, math.Min(1, math.Min(float64(b.Width)/float64(v.width), float64(b.Height)/float64(v.height))))
}
func (v *viewer) paint(_ *walk.Canvas, _ walk.Rectangle) error {
	b := v.canvas.ClientBoundsPixels()
	if b.Width <= 0 || b.Height <= 0 {
		return nil
	}
	if v.renderer == nil {
		v.renderer = &gpuRenderer{}
	}
	s := v.displayScale()
	x := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	y := math.Round((float64(b.Height)-float64(v.height)*s)/2 + v.panY)
	background := v.canvas.Background().(*walk.SolidColorBrush).Color()
	v.renderer.highlightBounds = v.highlightBounds
	err := v.renderer.draw(v.canvas.Handle(), b.Width, b.Height, v.source, s, x, y, v.linear, win.COLORREF(background))
	if err == errRecreateTarget {
		v.canvas.Invalidate()
		return nil
	}
	if err != nil {
		v.status.SetText("Rendering error")
		v.status.SetToolTipText(err.Error())
		return nil
	}
	status := "Open an image • Ctrl+O"
	if v.source != nil {
		status = fmt.Sprintf("%d × %d  •  %.0f%%  •  %s", v.width, v.height, s*100, formatFileSize(v.source.fileSize))
	}
	if v.loading {
		status = "Loading…"
	}
	if v.status.Text() != status {
		v.status.SetText(status)
	}
	label := "Direct2D GPU rendering"
	if !v.renderer.hardware {
		label = "Direct2D software fallback (GPU unavailable)"
	}
	if v.rendererLabel != label {
		v.status.SetToolTipText(label)
		v.rendererLabel = label
	}
	return nil
}

func formatFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	size := float64(bytes)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		size /= 1024
		if size < 1024 || unit == "EiB" {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
	}
	return ""
}

func (v *viewer) setBoundsHighlight(held bool) {
	if v.highlightBounds != held {
		v.highlightBounds = held
		v.canvas.Invalidate()
	}
}

func (v *viewer) installBoundsKeys() {
	update := func(walk.Key) { v.setBoundsHighlight(walk.ModifiersDown()&walk.ModControl != 0) }
	var attach func(walk.Window)
	attach = func(window walk.Window) {
		window.KeyDown().Attach(update)
		window.KeyUp().Attach(update)
		if container, ok := window.(walk.Container); ok {
			for i := 0; i < container.Children().Len(); i++ {
				attach(container.Children().At(i))
			}
		}
	}
	attach(v.window)
}

func (v *viewer) toggleInterpolation() {
	v.linear = !v.linear
	label := "Linear: Off"
	if v.linear {
		label = "Linear: On"
	}
	v.interpolationButton.SetText(label)
	v.canvas.Invalidate()
}

func (v *viewer) setBackground(color walk.Color) {
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		v.showError(err)
		return
	}
	old := v.canvas.Background()
	v.canvas.SetBackground(brush)
	if old != nil {
		old.Dispose()
	}
	v.canvas.Invalidate()
}

var backgroundColours = [...]walk.Color{walk.RGB(30, 30, 33), walk.RGB(238, 238, 238), walk.RGB(18, 18, 18)}

func (v *viewer) selectBackground(index int) {
	v.backgroundIndex = index
	v.setBackground(backgroundColours[index])
	v.palette.Invalidate()
}

func (v *viewer) backgroundPalette() ImageView {
	return ImageView{
		AssignTo: &v.palette, Alignment: AlignHNearVCenter, MinSize: Size{Width: 66, Height: 20}, MaxSize: Size{Width: 66, Height: 20},
		ToolTipText: "Background: charcoal, soft white, soft black. Arrow keys change colour.",
		OnMouseDown: func(x, y int, button walk.MouseButton) {
			if button == walk.LeftButton && x >= 0 && x < 66 && y >= 0 && y < 20 && x%22 < 20 {
				v.palette.SetFocus()
				v.selectBackground(x / 22)
			}
		},
		OnKeyDown: func(key walk.Key) {
			switch key {
			case walk.KeyLeft:
				v.selectBackground((v.backgroundIndex + 2) % 3)
			case walk.KeyRight:
				v.selectBackground((v.backgroundIndex + 1) % 3)
			}
		},
		Image: walk.NewPaintFuncImage(walk.Size{Width: 66, Height: 20}, func(c *walk.Canvas, _ walk.Rectangle) error {
			fill := func(colour walk.Color, rect walk.Rectangle) error {
				brush, err := walk.NewSolidColorBrush(colour)
				if err != nil {
					return err
				}
				defer brush.Dispose()
				return c.FillRectangle(brush, rect)
			}
			if err := fill(themeSurface(v.dark), v.palette.ClientBounds()); err != nil {
				return err
			}
			for i, colour := range backgroundColours {
				border := walk.RGB(150, 150, 150)
				if i == v.backgroundIndex {
					border = walk.RGB(0, 120, 215)
				}
				for _, layer := range []struct {
					colour walk.Color
					inset  int
				}{{border, 0}, {walk.RGB(255, 255, 255), 1}, {colour, 2}} {
					if err := fill(layer.colour, walk.Rectangle{X: i*22 + layer.inset, Y: layer.inset, Width: 20 - 2*layer.inset, Height: 20 - 2*layer.inset}); err != nil {
						return err
					}
				}
			}
			return nil
		}),
	}
}

func (v *viewer) showError(err error) {
	walk.MsgBox(v.window, "Unable to open image", err.Error(), walk.MsgBoxIconError)
}
