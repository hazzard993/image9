//go:build windows

package main

import (
	"fmt"
	"image"
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
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type viewer struct {
	window              *walk.MainWindow
	canvas              *walk.CustomWidget
	status              *walk.Label
	bitmap              *walk.Bitmap
	source              image.Image
	linear              bool
	interpolationButton *walk.PushButton
	rendered            renderKey
	files               []string
	path                string
	width, height       int
	scale, panX, panY   float64
	fit, dragging       bool
	lastX, lastY        int
}

type renderKey struct {
	width, height int
	scale, x, y   float64
	linear        bool
}

func main() {
	v := &viewer{fit: true, scale: 1}
	err := (MainWindow{
		AssignTo: &v.window, Title: "Quick Image Viewer", Size: Size{Width: 1000, Height: 740}, MinSize: Size{Width: 800, Height: 320},
		Layout: VBox{MarginsZero: true, Spacing: 0},
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "&Open…", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyO}, OnTriggered: v.openDialog},
				Action{Text: "E&xit", OnTriggered: func() { v.window.Close() }},
			}},
			Menu{Text: "&View", Items: []MenuItem{
				Action{Text: "Previous", Shortcut: Shortcut{Key: walk.KeyLeft}, OnTriggered: func() { v.navigate(-1) }},
				Action{Text: "Next", Shortcut: Shortcut{Key: walk.KeyRight}, OnTriggered: func() { v.navigate(1) }},
				Action{Text: "Fit to window", Shortcut: Shortcut{Key: walk.KeyF}, OnTriggered: v.fitImage},
				Action{Text: "Actual size (100%)", Shortcut: Shortcut{Key: walk.Key1}, OnTriggered: v.actualSize},
			}},
		},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				PushButton{Text: "Open…", OnClicked: v.openDialog},
				PushButton{Text: "‹ Previous", OnClicked: func() { v.navigate(-1) }},
				PushButton{Text: "Next ›", OnClicked: func() { v.navigate(1) }},
				HSpacer{},
				PushButton{Text: "−", OnClicked: func() { v.zoom(1 / 1.25) }},
				PushButton{Text: "+", OnClicked: func() { v.zoom(1.25) }},
				PushButton{Text: "Fit", OnClicked: v.fitImage},
				PushButton{Text: "100%", OnClicked: v.actualSize},
				PushButton{AssignTo: &v.interpolationButton, Text: "Linear: Off", ToolTipText: "Toggle bilinear smoothing / nearest-neighbor pixels", OnClicked: v.toggleInterpolation},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				Label{Text: "Background:"},
				PushButton{Text: "Current", ToolTipText: "Original charcoal (#1E1E21)", OnClicked: func() { v.setBackground(walk.RGB(30, 30, 33)) }},
				PushButton{Text: "White", ToolTipText: "Soft white (#EEEEEE)", OnClicked: func() { v.setBackground(walk.RGB(238, 238, 238)) }},
				PushButton{Text: "Black", ToolTipText: "Soft black (#121212)", OnClicked: func() { v.setBackground(walk.RGB(18, 18, 18)) }},
				HSpacer{},
			}},
			CustomWidget{AssignTo: &v.canvas, Background: SolidColorBrush{Color: walk.RGB(30, 30, 33)},
				PaintMode: PaintBuffered, InvalidatesOnResize: true, PaintPixels: v.paint,
				OnMouseDown: func(x, y int, b walk.MouseButton) {
					if b == walk.LeftButton {
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
			Label{AssignTo: &v.status, Text: "Open an image or drop one here • Ctrl+O: Open • ← →: Browse • Wheel: Zoom • Drag: Pan", MinSize: Size{Height: 26}},
		},
	}).Create()
	if err != nil {
		walk.MsgBox(nil, "Unable to start", err.Error(), walk.MsgBoxIconError)
		return
	}
	v.canvas.MouseWheel().Attach(func(x, y int, b walk.MouseButton) {
		if walk.MouseWheelEventDelta(b) > 0 {
			v.zoom(1.25)
		} else {
			v.zoom(1 / 1.25)
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
	defer func() {
		if v.bitmap != nil {
			v.bitmap.Dispose()
		}
	}()
	if len(os.Args) > 1 {
		v.open(os.Args[1])
	}
	v.window.Run()
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
	f, err := os.Open(abs)
	if err != nil {
		v.showError(err)
		return
	}
	im, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		v.showError(fmt.Errorf("cannot decode %s: %w", filepath.Base(abs), err))
		return
	}
	old := v.bitmap
	v.bitmap = nil
	v.source = im
	if old != nil {
		old.Dispose()
	}
	v.path = abs
	v.width = im.Bounds().Dx()
	v.height = im.Bounds().Dy()
	v.files, err = folderImages(filepath.Dir(abs))
	if err != nil {
		v.files = []string{abs}
	}
	v.fitImage()
	v.window.SetTitle(filepath.Base(abs) + " — Quick Image Viewer")
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
		if strings.EqualFold(p, v.path) {
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
	if v.source == nil {
		return
	}
	before := v.displayScale()
	v.scale = math.Max(0.01, math.Min(32, before*factor))
	v.fit = false
	v.panX *= v.scale / before
	v.panY *= v.scale / before
	v.canvas.Invalidate()
}
func (v *viewer) displayScale() float64 {
	if !v.fit || v.source == nil {
		return v.scale
	}
	b := v.canvas.ClientBoundsPixels()
	return math.Max(0.001, math.Min(1, math.Min(float64(b.Width)/float64(v.width), float64(b.Height)/float64(v.height))))
}
func (v *viewer) paint(c *walk.Canvas, _ walk.Rectangle) error {
	// Paint a complete frame offscreen, including the background beneath transparent
	// pixels. PaintBuffered presents it in one blit without a visible erase step.
	b := v.canvas.ClientBoundsPixels()
	if err := c.FillRectanglePixels(v.canvas.Background(), b); err != nil {
		return err
	}
	if v.source == nil {
		return nil
	}
	s := v.displayScale()
	if b.Width <= 0 || b.Height <= 0 {
		return nil
	}
	key := renderKey{width: b.Width, height: b.Height, scale: s,
		x: (float64(b.Width)-float64(v.width)*s)/2 + v.panX,
		y: (float64(b.Height)-float64(v.height)*s)/2 + v.panY, linear: v.linear}
	if v.bitmap == nil || v.rendered != key {
		im := renderImage(v.source, key)
		bitmap, err := walk.NewBitmapFromImageForDPI(im, 96)
		if err != nil {
			return err
		}
		if v.bitmap != nil {
			v.bitmap.Dispose()
		}
		v.bitmap, v.rendered = bitmap, key
	}
	status := fmt.Sprintf("%s  •  %d × %d  •  %.0f%%  •  %d images in folder", filepath.Base(v.path), v.width, v.height, s*100, len(v.files))
	if v.status.Text() != status {
		v.status.SetText(status)
	}
	return c.DrawImagePixels(v.bitmap, walk.Point{})
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

// Render only the visible viewport, so a high zoom never allocates an enormous image.
// The bitmap is cached by the caller until the view or interpolation changes.
func renderImage(src image.Image, key renderKey) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, key.width, key.height))
	var interpolator draw.Transformer = draw.NearestNeighbor
	if key.linear {
		interpolator = draw.BiLinear
	}
	b := src.Bounds()
	interpolator.Transform(dst, f64.Aff3{key.scale, 0, key.x - float64(b.Min.X)*key.scale,
		0, key.scale, key.y - float64(b.Min.Y)*key.scale}, src, b, draw.Src, nil)
	return dst
}
func (v *viewer) showError(err error) {
	walk.MsgBox(v.window, "Unable to open image", err.Error(), walk.MsgBoxIconError)
}
