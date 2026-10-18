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
	"runtime"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"golang.org/x/sys/windows"
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
	loading             bool
	highlightBounds     bool
	ctrlViewPinned      bool
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
	dragButton          walk.MouseButton
	dark                bool
	themeBrush          *walk.SolidColorBrush
	workspace           []workspaceImage
	activeIndex         int
	loadingIndex        int
	wrapWidth           int
	resizingColumns     bool
	placingWrap         bool
	wrapInput           string
	pointerInside       bool
	pointerX, pointerY  int
	hoverIndex          int
	path                string
	width, height       int
	scale, panX, panY   float64
	fit, dragging       bool
	zoomToCursor        bool
	zoomToCursorAction  *walk.Action
	zoomToCursorButton  *walk.PushButton
	cursorKeyboardHook  uintptr
	cursorHidden        bool
	cursorHiddenAt      win.POINT
	lastX, lastY        int
	feedbackText        string
	feedbackTime        time.Time
	gapInput            *string
	gapKeyboardHook     uintptr
	imageOrderList      *walk.ListBox
	nextOrderID         uint64
	showFilenameLabels  bool
	imageGap            int
	customGap           bool
	gapShortcuts        []*walk.Action
}

func main() {
	runtime.LockOSThread()
	instance, forwarded, err := acquireViewerInstance(os.Args[1:])
	if err != nil {
		walk.MsgBox(nil, "Unable to open viewer", err.Error(), walk.MsgBoxIconError)
		return
	}
	if forwarded {
		return
	}
	defer instance.close()
	v := &viewer{fit: true, scale: 1, zoomToCursor: true, dark: loadDarkMode(), copyFormat: loadCopyFormat()}
	setNativeAppTheme(v.dark)
	err = (MainWindow{
		AssignTo: &v.window, Title: "Quick Image Viewer", Size: Size{Width: 1000, Height: 740}, MinSize: Size{Width: 800, Height: 320},
		Layout: VBox{MarginsZero: true, SpacingZero: true},
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "Clear workspace", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyN}, OnTriggered: v.clearWorkspace},
				Action{Text: "Copy focused image path", Shortcut: Shortcut{Key: walk.KeyC}, OnTriggered: v.copyFocusedPath},
				Action{Text: "&Open…", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyO}, OnTriggered: v.openDialog},
				Action{Text: "Open containing folder", Shortcut: Shortcut{Key: walk.KeyO}, OnTriggered: v.openContainingFolder},
				Action{Text: "E&xit", Shortcut: Shortcut{Key: walk.KeyQ}, OnTriggered: func() { v.window.Close() }},
			}},
			Menu{Text: "&View", Items: []MenuItem{
				Action{Text: "Previous", Shortcut: Shortcut{Key: walk.KeyLeft}, OnTriggered: func() { v.navigate(-1) }},
				Action{Text: "Next", Shortcut: Shortcut{Key: walk.KeyRight}, OnTriggered: func() { v.navigate(1) }},
				Action{Text: "Image above", Shortcut: Shortcut{Key: walk.KeyUp}, OnTriggered: func() { v.navigateVertical(-1) }},
				Action{Text: "Image below", Shortcut: Shortcut{Key: walk.KeyDown}, OnTriggered: func() { v.navigateVertical(1) }},
				Action{Text: "Set image gap", Shortcut: Shortcut{Key: walk.KeyG}, OnTriggered: v.beginGapInput},
				Action{Text: "Image order / right in I mode", Shortcut: Shortcut{Key: walk.KeyL}, OnTriggered: v.handleL},
				Action{Text: "Unset focused image", Shortcut: Shortcut{Key: walk.KeyU}, OnTriggered: v.unsetFocusedImage},
				Action{Text: "Toggle filename labels / left in I mode", Shortcut: Shortcut{Key: walk.KeyH}, OnTriggered: v.handleH},
				Action{Text: "Toggle filename labels", Shortcut: Shortcut{Key: walk.KeyT}, OnTriggered: v.toggleCtrlText},
				Action{Text: "Down in I mode", Shortcut: Shortcut{Key: walk.KeyJ}, OnTriggered: v.handleJ},
				Action{Text: "Up in I mode", Shortcut: Shortcut{Key: walk.KeyK}, OnTriggered: v.handleK},
				Action{Text: "Enter Inspect Mode", Shortcut: Shortcut{Key: walk.KeyI}, OnTriggered: v.enterCtrlView},
				Action{Text: "Copy selected path in Inspect Mode", Shortcut: Shortcut{Key: walk.KeyReturn}, OnTriggered: v.copyPathInInspectMode},
				Menu{Text: "Zoom", Items: []MenuItem{
					Action{AssignTo: &v.zoomToCursorAction, Text: "Zoom to cursor", Checkable: true, Checked: v.zoomToCursor, OnTriggered: v.toggleZoomToCursor},
					Action{Text: "Zoom in", Shortcut: Shortcut{Key: walk.KeyOEMPlus}, OnTriggered: func() { v.zoom(1.25) }},
					Action{Text: "Zoom out", Shortcut: Shortcut{Key: walk.KeyOEMMinus}, OnTriggered: func() { v.zoom(1 / 1.25) }},
					Action{Text: "Fit to window", Shortcut: Shortcut{Key: walk.KeyF}, OnTriggered: v.fitImage},
					Action{Text: "Frame selected / all", Shortcut: Shortcut{Key: walk.KeyR}, OnTriggered: v.frameSelectedOrAll},
					Action{Text: "Actual size (100%)", Shortcut: Shortcut{Key: walk.KeyE}, OnTriggered: v.actualSize},
				}},
				Menu{Text: "Layout", Items: []MenuItem{Action{Text: "Set / reset wrap limit", Shortcut: Shortcut{Key: walk.KeyW}, OnTriggered: v.toggleWrapMode}}},
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
				Action{Text: "Pick colour from image", OnTriggered: v.toggleColourPicker},
				Menu{Text: "Copy colour format", Items: []MenuItem{
					Action{AssignTo: &v.copyFormatActions[0], Text: "Hex - #RRGGBB", Checkable: true, Checked: v.copyFormat == formatHex, OnTriggered: func() { v.setCopyFormat(formatHex) }},
					Action{AssignTo: &v.copyFormatActions[1], Text: "RGB - rgb(r, g, b)", Checkable: true, Checked: v.copyFormat == formatRGB, OnTriggered: func() { v.setCopyFormat(formatRGB) }},
					Action{AssignTo: &v.copyFormatActions[2], Text: "HSL - hsl(h, s%, l%)", Checkable: true, Checked: v.copyFormat == formatHSL, OnTriggered: func() { v.setCopyFormat(formatHSL) }},
				}},
				Action{Text: "Exit mode / focus image", Shortcut: Shortcut{Key: walk.KeyEscape}, OnTriggered: v.escapeMode},
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
					if b == walk.LeftButton && v.placingWrap {
						v.placeWrapAt(x)
						return
					}
					if b == walk.LeftButton && v.nearWrapLine(x) && len(v.workspace) > 0 {
						v.scale = v.displayScale()
						v.fit = false
						v.startDrag(x, y, b)
						v.resizingColumns = true
						v.setWrapAt(x)
						return
					}
					if b == walk.RightButton {
						if v.highlightBounds || walk.ModifiersDown()&walk.ModControl != 0 {
							if index, _, _ := v.hitImage(x, y); index >= 0 {
								v.removeWorkspaceImage(index)
							}
						} else {
							v.toggleColourPicker()
						}
						return
					}
					if b == walk.MiddleButton {
						v.canvas.SetFocus()
						v.startDrag(x, y, b)
						return
					}
					if b == walk.LeftButton {
						if v.highlightBounds || walk.ModifiersDown()&walk.ModControl != 0 {
							if index, _, _ := v.hitImage(x, y); index >= 0 {
								if err := walk.Clipboard().SetText(v.workspace[index].path); err != nil {
									v.showError(err)
								} else {
									v.notifyAction("Copied path " + v.workspace[index].path)
								}
							}
							return
						}
						if v.colourPicking {
							v.pickColourAt(x, y)
							return
						}
						v.focusImage()
						if index, _, _ := v.hitImage(x, y); index >= 0 {
							v.selectImage(index)
							v.notifyAction("Focused on " + filepath.Base(v.workspace[index].path))
						}
						v.startDrag(x, y, b)
					}
				},
				OnMouseUp: func(x, y int, b walk.MouseButton) {
					if b == v.dragButton {
						v.stopDrag()
						v.canvas.Invalidate()
					}
				},
				OnMouseMove: func(x, y int, b walk.MouseButton) {
					v.trackWorkspacePointer(x, y)
					if v.dragging && b&v.dragButton == 0 {
						v.stopDrag()
					}
					if v.resizingColumns {
						v.setWrapAt(x)
						return
					}
					if v.dragging {
						if v.fit && (x != v.lastX || y != v.lastY) {
							v.scale = v.displayScale()
							v.fit = false
						}
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
				PushButton{Text: "Open", OnClicked: v.openDialog},
				PushButton{Text: "Clear", ToolTipText: "Clear the workspace (Ctrl+N)", OnClicked: v.clearWorkspace},
				PushButton{Text: "‹ Previous", OnClicked: func() { v.navigate(-1) }},
				PushButton{Text: "Next ›", OnClicked: func() { v.navigate(1) }},
				PushButton{Text: "−", OnClicked: func() { v.zoom(1 / 1.25) }},
				PushButton{Text: "+", OnClicked: func() { v.zoom(1.25) }},
				PushButton{Text: "Fit", OnClicked: v.fitImage},
				PushButton{Text: "100%", OnClicked: v.actualSize},
				PushButton{AssignTo: &v.zoomToCursorButton, MinSize: Size{Width: 28}, MaxSize: Size{Width: 28}, Image: v.zoomTargetIcon(), ToolTipText: "Zoom to cursor: On. Click to zoom at the view centre instead.", OnClicked: v.toggleZoomToCursor},
				PushButton{AssignTo: &v.interpolationButton, Text: "Linear: Off", ToolTipText: "Toggle bilinear smoothing / nearest-neighbor pixels", OnClicked: v.toggleInterpolation},
				v.backgroundPalette(),
				PushButton{AssignTo: &v.colourPickerButton, MinSize: Size{Width: 100}, MaxSize: Size{Width: 100}, Text: "Pick colour", ToolTipText: "Pick a pixel from the image and copy its colour (right-click)", OnClicked: v.toggleColourPicker},
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
	v.registerBackgroundAliases()
	ctrlHelpAction := walk.NewAction()
	ctrlHelpAction.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: walk.KeyH})
	ctrlHelpAction.Triggered().Attach(v.handleH)
	v.window.ShortcutActions().Add(ctrlHelpAction)
	ctrlUnsetAction := walk.NewAction()
	ctrlUnsetAction.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: walk.KeyU})
	ctrlUnsetAction.Triggered().Attach(v.unsetFocusedImage)
	v.window.ShortcutActions().Add(ctrlUnsetAction)
	ctrlWrapAction := walk.NewAction()
	ctrlWrapAction.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: walk.KeyW})
	ctrlWrapAction.Triggered().Attach(v.toggleWrapMode)
	v.window.ShortcutActions().Add(ctrlWrapAction)
	for _, shortcut := range []struct {
		key     walk.Key
		handler func()
	}{
		{walk.KeyC, v.handleCtrlC}, {walk.KeyG, v.beginGapInput},
		{walk.KeyReturn, v.copyPathInInspectMode},
		{walk.KeyT, v.toggleCtrlText},
		{walk.KeyJ, v.handleJ}, {walk.KeyK, v.handleK}, {walk.KeyL, v.handleCtrlL},
		{walk.KeyLeft, func() { v.navigate(-1) }},
		{walk.KeyRight, func() { v.navigate(1) }},
		{walk.KeyUp, func() { v.navigateVertical(-1) }},
		{walk.KeyDown, func() { v.navigateVertical(1) }},
	} {
		action := walk.NewAction()
		action.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: shortcut.key})
		action.Triggered().Attach(shortcut.handler)
		v.window.ShortcutActions().Add(action)
	}
	v.window.Starting().Attach(v.focusImage)
	v.applyTheme()
	installLoadPump(v)
	if err := instance.attach(v.window.Handle(), v.openImages); err != nil {
		v.window.Dispose()
		walk.MsgBox(nil, "Unable to open viewer", err.Error(), walk.MsgBoxIconError)
		return
	}
	v.installPointerTracking()
	v.installBoundsKeys()
	if err := v.installKeyboardCursor(); err != nil {
		v.showError(err)
	}
	v.canvas.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
		point := win.POINT{X: int32(x), Y: int32(y)}
		if !win.ScreenToClient(v.canvas.Handle(), &point) {
			return
		}
		factor := 1.25
		if walk.MouseWheelEventDelta(b) < 0 {
			factor = 1 / 1.25
		}
		if v.zoomToCursor {
			v.zoomAt(factor, float64(point.X), float64(point.Y))
		} else {
			v.zoom(factor)
		}
	})
	v.window.DropFiles().Attach(v.dropImages)
	v.canvas.DropFiles().Attach(v.dropImages)
	if len(os.Args) > 1 {
		v.openImages(os.Args[1:])
	}
	v.window.Run()
}

func (v *viewer) focusImage() {
	if v.placingWrap {
		v.endWrapPlacement()
		v.notifyAction("Wrap placement cancelled")
	}
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

func (v *viewer) registerBackgroundAliases() {
	for index, key := range []walk.Key{walk.KeyNumpad1, walk.KeyNumpad2, walk.KeyNumpad3} {
		action := walk.NewAction()
		action.SetShortcut(walk.Shortcut{Key: key})
		action.Triggered().Attach(func() { v.selectBackground(index) })
		v.window.ShortcutActions().Add(action)
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
	v.resizingColumns = false
}

func (v *viewer) toggleColourPicker() {
	if v.colourPicking {
		v.focusImage()
		v.notifyAction("Colour picker off")
		return
	}
	if len(v.workspace) == 0 {
		return
	}
	v.focusImage()
	v.colourPicking = true
	v.colourPickerButton.SetText("Click image")
	v.canvas.SetCursor(walk.CursorCross())
	v.notifyAction("Colour picker - click a pixel to copy")
}

func (v *viewer) colourAt(x, y int) (string, bool) {
	index, px, py := v.hitImage(x, y)
	if index < 0 {
		return "", false
	}
	return imageColourAt(v.workspace[index].image, px, py)
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
	v.colourPickerButton.SetToolTipText("Copied " + copied + " to clipboard. Right-click to pick another pixel.")
	v.notifyAction("Copied colour: " + copied)
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
	d := walk.FileDialog{Title: "Open images", Filter: "Images (*.jpg;*.jpeg;*.png;*.gif;*.bmp;*.tif;*.tiff;*.webp)|*.jpg;*.jpeg;*.png;*.gif;*.bmp;*.tif;*.tiff;*.webp|All files (*.*)|*.*", FilePath: v.path}
	if ok, err := d.ShowOpenMultiple(v.window); err != nil {
		v.showError(err)
	} else if ok {
		v.openImages(d.FilePaths)
	}
}

func normalizeImagePaths(paths []string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(abs)
		if !seen[key] {
			result = append(result, abs)
			seen[key] = true
		}
	}
	return result, nil
}

func (v *viewer) openImages(paths []string) { v.addWorkspaceImages(paths) }

func (v *viewer) copyFocusedPath() {
	if v.activeIndex < 0 || v.activeIndex >= len(v.workspace) || v.workspace[v.activeIndex].path == "" {
		v.notifyAction("No focus target, nothing copied")
		return
	}
	path := v.workspace[v.activeIndex].path
	if err := walk.Clipboard().SetText(path); err != nil {
		v.showError(err)
		return
	}
	v.notifyAction("Copied path " + path)
}

func (v *viewer) openContainingFolder() {
	if v.activeIndex < 0 || v.activeIndex >= len(v.workspace) {
		return
	}
	folder := filepath.Dir(v.workspace[v.activeIndex].path)
	path, err := windows.UTF16PtrFromString(folder)
	if err == nil {
		err = windows.ShellExecute(windows.Handle(v.window.Handle()), nil, path, nil, nil, win.SW_SHOWNORMAL)
	}
	if err != nil {
		v.showError(err)
		return
	}
	v.notifyAction("Opened folder " + folder)
}

func (v *viewer) navigate(delta int) {
	if len(v.workspace) == 0 {
		return
	}
	s := v.displayScale()
	v.selectImage((v.activeIndex + delta + len(v.workspace)) % len(v.workspace))
	v.fit = false
	v.scale = s
	v.centreActive(s)
	v.notifyAction("Focused on " + filepath.Base(v.path))
	v.canvas.Invalidate()
}

func (v *viewer) fitImage() {
	v.fit = true
	v.panX = 0
	v.panY = 0
	v.notifyAction("Zoom reset - fit all images")
}
func (v *viewer) actualSize() {
	v.fit = false
	v.scale = 1
	v.panX = 0
	v.panY = 0
	v.centreActive(1)
	v.notifyAction("Actual size - 100%")
	v.canvas.Invalidate()
}
func (v *viewer) toggleZoomToCursor() {
	v.zoomToCursor = !v.zoomToCursor
	v.zoomToCursorAction.SetChecked(v.zoomToCursor)
	if v.zoomToCursorButton != nil {
		old := v.zoomToCursorButton.Image()
		v.zoomToCursorButton.SetImage(v.zoomTargetIcon())
		if old != nil {
			old.Dispose()
		}
	}
	if v.zoomToCursor {
		if v.zoomToCursorButton != nil {
			v.zoomToCursorButton.SetToolTipText("Zoom to cursor: On. Click to zoom at the view centre instead.")
		}
		v.notifyAction("Zoom to cursor on")
	} else {
		if v.zoomToCursorButton != nil {
			v.zoomToCursorButton.SetToolTipText("Zoom to cursor: Off. Click to zoom toward the cursor instead.")
		}
		v.notifyAction("Zoom to cursor off - zoom at view centre")
	}
}

func (v *viewer) zoomTargetIcon() walk.Image {
	colour := walk.RGB(128, 128, 128)
	if v.zoomToCursor {
		colour = walk.RGB(45, 145, 245)
	}
	return walk.NewPaintFuncImage(walk.Size{Width: 16, Height: 16}, func(c *walk.Canvas, _ walk.Rectangle) error {
		pen, err := walk.NewCosmeticPen(walk.PenSolid, colour)
		if err != nil {
			return err
		}
		defer pen.Dispose()
		if err := c.DrawEllipse(pen, walk.Rectangle{X: 3, Y: 3, Width: 10, Height: 10}); err != nil {
			return err
		}
		for _, line := range [][2]walk.Point{
			{{X: 8, Y: 0}, {X: 8, Y: 5}}, {{X: 8, Y: 11}, {X: 8, Y: 16}},
			{{X: 0, Y: 8}, {X: 5, Y: 8}}, {{X: 11, Y: 8}, {X: 16, Y: 8}},
		} {
			if err := c.DrawLine(pen, line[0], line[1]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (v *viewer) zoom(factor float64) {
	b := v.canvas.ClientBoundsPixels()
	v.zoomAt(factor, float64(b.Width)/2, float64(b.Height)/2)
}

func (v *viewer) zoomAt(factor, x, y float64) {
	if len(v.workspace) == 0 {
		return
	}
	before := v.displayScale()
	v.scale = math.Max(0.000001, math.Min(32, before*factor))
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
	if !v.fit || len(v.workspace) == 0 {
		return v.scale
	}
	b := v.canvas.ClientBoundsPixels()
	return math.Max(0.000001, math.Min(1, math.Min(float64(max(1, b.Width-32))/float64(v.width), float64(max(1, b.Height-64))/float64(v.height))))
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
	v.renderer.feedbackText = v.feedbackText
	v.renderer.modeText = v.modeText()
	v.renderer.feedbackAlpha = feedbackOpacity(time.Since(v.feedbackTime))
	if v.gapInput != nil || v.placingWrap {
		v.renderer.feedbackAlpha = 1
	}
	v.renderer.showBoundary = v.highlightBounds || v.placingWrap
	v.renderer.placingWrap = v.placingWrap
	v.renderer.boundaryUnset = v.wrapWidth == 0
	v.renderer.showBoundaryHint = v.placingWrap && v.wrapWidth > 0
	v.renderer.boundaryX = float32(v.wrapLineX())
	v.prepareHoverCaption(s, x, y, b.Width, b.Height)
	scene := make([]sceneImage, 0, len(v.workspace))
	for index, item := range v.workspace {
		scene = append(scene, sceneImage{image: item.image, x: item.x, y: item.y, selected: index == v.activeIndex})
	}
	err := v.renderer.drawScene(v.canvas.Handle(), b.Width, b.Height, scene, s, x, y, v.linear, win.COLORREF(background))
	if err == errRecreateTarget {
		v.canvas.Invalidate()
		return nil
	}
	if err != nil {
		v.status.SetText("Rendering error")
		v.status.SetToolTipText(err.Error())
		return nil
	}
	status := v.workspaceStatus(s)
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
	held = held || v.ctrlViewPinned
	if v.highlightBounds != held {
		v.highlightBounds = held
		if !held && v.resizingColumns {
			v.stopDrag()
		}
		if v.pointerInside {
			v.trackWorkspacePointer(v.pointerX, v.pointerY)
		}
		v.canvas.Invalidate()
	}
}

func (v *viewer) enterCtrlView() {
	v.focusImage()
	v.ctrlViewPinned = true
	v.setBoundsHighlight(true)
	v.notifyAction("Inspect Mode enabled - Esc to exit")
}

func (v *viewer) escapeMode() {
	wasPinned := v.ctrlViewPinned
	v.ctrlViewPinned = false
	v.setBoundsHighlight(walk.ModifiersDown()&walk.ModControl != 0)
	v.focusImage()
	if wasPinned {
		v.notifyAction("Inspect Mode exited")
	}
}

func (v *viewer) installBoundsKeys() {
	update := func(key walk.Key) {
		held := walk.ModifiersDown()&walk.ModControl != 0
		if key == walk.KeyControl && held {
			// Ctrl view belongs to the canvas. A previously clicked toolbar
			// button must not retain keyboard focus when entering it.
			v.canvas.SetFocus()
		}
		v.setBoundsHighlight(held)
	}
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
	v.notifyAction(label)
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
	v.notifyAction("Background: " + []string{"charcoal", "soft white", "soft black"}[index])
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
