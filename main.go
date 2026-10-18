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
	v := &viewer{fit: true, scale: 1, dark: loadDarkMode()}
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
				Action{Text: "Fit to window", Shortcut: Shortcut{Key: walk.KeyF}, OnTriggered: v.fitImage},
				Action{Text: "Reset zoom", Shortcut: Shortcut{Key: walk.KeyR}, OnTriggered: v.fitImage},
				Action{Text: "Actual size (100%)", Shortcut: Shortcut{Key: walk.KeyE}, OnTriggered: v.actualSize},
				Action{Text: "Charcoal background", Shortcut: Shortcut{Key: walk.Key1}, OnTriggered: func() { v.selectBackground(0) }},
				Action{Text: "Soft white background", Shortcut: Shortcut{Key: walk.Key2}, OnTriggered: func() { v.selectBackground(1) }},
				Action{Text: "Soft black background", Shortcut: Shortcut{Key: walk.Key3}, OnTriggered: func() { v.selectBackground(2) }},
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
					if b == walk.LeftButton {
						v.focusImage()
						v.dragging = true
						v.lastX = x
						v.lastY = y
					}
				},
				OnMouseUp: func(x, y int, b walk.MouseButton) { v.dragging = false },
				OnMouseMove: func(x, y int, b walk.MouseButton) {
					if v.dragging {
						v.panX += float64(x - v.lastX)
						v.panY += float64(y - v.lastY)
						v.lastX = x
						v.lastY = y
						v.canvas.Invalidate()
					}
				},
			},
			Composite{AssignTo: &v.footer, Layout: HBox{Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}, Spacing: 4}, Children: []Widget{
				Label{AssignTo: &v.status, MinSize: Size{Width: 180}, MaxSize: Size{Width: 180}, EllipsisMode: EllipsisEnd, Text: "Open an image • Ctrl+O"},
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
			}},
		},
	}).Create()
	if err != nil {
		walk.MsgBox(nil, "Unable to start", err.Error(), walk.MsgBoxIconError)
		return
	}
	// Form shortcuts run before Windows dialog navigation, which otherwise uses
	// arrow keys to move focus among toolbar buttons.
	for i := 0; i < v.window.Menu().Actions().Len(); i++ {
		menu := v.window.Menu().Actions().At(i).Menu()
		if menu == nil {
			continue
		}
		for j := 0; j < menu.Actions().Len(); j++ {
			action := menu.Actions().At(j)
			if action.Shortcut().Key != 0 {
				v.window.ShortcutActions().Add(action)
			}
		}
	}
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
	v.dragging = false
	v.canvas.SetFocus()
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
	v.loadID = v.loader.request(abs)
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
		status = fmt.Sprintf("%d × %d  •  %.0f%%", v.width, v.height, s*100)
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
