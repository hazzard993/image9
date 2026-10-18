package main

import (
	"image"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func exerciseDirect2DRenderAndRecovery(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var window *walk.MainWindow
	var canvas *walk.CustomWidget
	if err := (MainWindow{AssignTo: &window, Visible: false, Size: Size{Width: 320, Height: 240}, Layout: VBox{}, Children: []Widget{
		CustomWidget{AssignTo: &canvas, PaintMode: PaintNoErase, PaintPixels: func(*walk.Canvas, walk.Rectangle) error { return nil }},
	}}).Create(); err != nil {
		t.Fatal(err)
	}
	defer window.Dispose()
	r := &gpuRenderer{}
	defer r.close()
	im := &decodedImage{width: 2560, height: 1440, pixels: make([]byte, 2560*1440*4)}
	for i := 0; i < len(im.pixels); i += 4 {
		im.pixels[i+2] = 255
		im.pixels[i+3] = 255
	}
	frame := func(width, height int, scale, x float64, linear bool) {
		t.Helper()
		if err := r.draw(canvas.Handle(), width, height, im, scale, x, 10, linear, win.COLORREF(0x00202020)); err != nil {
			t.Fatal(err)
		}
	}
	frame(320, 240, 1, 0, false)
	t.Logf("Direct2D hardware target: %v", r.hardware)
	bitmap := r.tiles[0].bitmap
	for _, linear := range []bool{true, false} {
		for i := 0; i < 10; i++ {
			frame(320, 240, 1+float64(i), float64(i*3), linear)
		}
	}
	if r.tiles[0].bitmap != bitmap {
		t.Fatal("pan/zoom reuploaded the bitmap")
	}
	frame(640, 480, 2, 0, false)
	if r.tiles[0].bitmap != bitmap {
		t.Fatal("resize reuploaded the bitmap")
	}
	r.disposeTarget() // Simulate losing all device-dependent resources.
	frame(320, 240, 1, 0, false)
	if r.image != im || len(r.tiles) != 1 {
		t.Fatal("device recovery did not upload retained pixels")
	}
	checkDirect2DPixels(t, r.factory)
}

// A WIC-backed Direct2D target lets us read the actual output of the same COM
// drawing calls, rather than only checking that EndDraw returned success.
func checkDirect2DPixels(t *testing.T, factory *d2dObject) {
	t.Helper()
	hr := win.CoInitializeEx(nil, 2)
	if int32(hr) >= 0 {
		defer win.CoUninitialize()
	}
	clsid := windows.GUID{Data1: 0xcacaf262, Data2: 0x9370, Data3: 0x4615, Data4: [8]byte{0xa1, 0x3b, 0x9f, 0x55, 0x39, 0xda, 0x4c, 0x0a}}
	iid := windows.GUID{Data1: 0xec5ec8a9, Data2: 0xc395, Data3: 0x4314, Data4: [8]byte{0x9c, 0x77, 0x54, 0xd7, 0xa9, 0x35, 0xff, 0x70}}
	var wic *d2dObject
	proc := windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&wic)))
	if err := d2dError("Create WIC factory", result); err != nil {
		t.Fatal(err)
	}
	defer release(&wic)
	format := windows.GUID{Data1: 0x6fddc324, Data2: 0x4e03, Data3: 0x4bfe, Data4: [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x10}}
	var bitmap *d2dObject
	if err := d2dError("Create WIC bitmap", wic.call(17, 8, 4, uintptr(unsafe.Pointer(&format)), 2, uintptr(unsafe.Pointer(&bitmap)))); err != nil {
		t.Fatal(err)
	}
	defer release(&bitmap)
	props := d2dTargetProperties{kind: 1, format: d2dPixelFormat{87, 1}, dpiX: 96, dpiY: 96}
	r := &gpuRenderer{factory: factory, width: 8, height: 4}
	factory.call(1)
	defer r.close()
	if err := d2dError("Create WIC target", factory.call(13, uintptr(unsafe.Pointer(bitmap)), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&r.target)))); err != nil {
		t.Fatal(err)
	}
	read := func(im *decodedImage, scale float64, linear bool, bg win.COLORREF) []byte {
		t.Helper()
		if err := r.draw(0, 8, 4, im, scale, 0, 0, linear, bg); err != nil {
			t.Fatal(err)
		}
		pixels := make([]byte, 8*4*4)
		if err := d2dError("Read pixels", bitmap.call(7, 0, 32, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
			t.Fatal(err)
		}
		return pixels
	}
	im := &decodedImage{width: 2, height: 1, pixels: []byte{0, 0, 0, 255, 255, 255, 255, 255}}
	nearest := read(im, 4, false, 0)
	linear := read(im, 4, true, 0)
	for x := 0; x < 8; x++ {
		want := byte(0)
		if x >= 4 {
			want = 255
		}
		if nearest[(2*8+x)*4] != want {
			t.Fatalf("nearest interpolation at %d: %v", x, nearest)
		}
	}
	if p := linear[(2*8+3)*4]; p == 0 || p == 255 {
		t.Fatalf("linear interpolation failed: %v", linear)
	}
	r.highlightBounds = true
	outlined := read(im, 4, false, 0)
	edge := (2 * 8) * 4
	if outlined[edge] < 200 || outlined[edge+2] > 80 {
		t.Fatalf("blue bounds outline missing: %v", outlined[edge:edge+4])
	}
	r.highlightBounds = false
	plain := read(im, 4, false, 0)
	if plain[edge] != 0 {
		t.Fatal("bounds outline remained after release")
	}
	alpha := read(&decodedImage{width: 1, height: 1, pixels: []byte{0, 0, 128, 128}}, 1, false, 0xffffff)
	if alpha[0] != 127 || alpha[1] != 127 || alpha[2] != 255 {
		t.Fatalf("alpha/color output: %v", alpha[:4])
	}
	red, green := &decodedImage{width: 2, height: 4, pixels: make([]byte, 32)}, &decodedImage{width: 2, height: 4, pixels: make([]byte, 32)}
	for i := 0; i < 32; i += 4 {
		red.pixels[i+2] = 255
		red.pixels[i+3] = 255
		green.pixels[i+1] = 255
		green.pixels[i+3] = 255
	}
	scene := []sceneImage{{image: red}, {image: green, x: 4}}
	if err := r.drawScene(0, 8, 4, scene, 1, 0, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	redTexture, greenTexture := r.textures[red][0].bitmap, r.textures[green][0].bitmap
	spatial := make([]byte, 128)
	if err := d2dError("Read spatial output", bitmap.call(7, 0, 32, 128, uintptr(unsafe.Pointer(&spatial[0])))); err != nil {
		t.Fatal(err)
	}
	if spatial[32+2] != 255 || spatial[32+4*4+1] != 255 || spatial[32+2*4] != 0 || spatial[32+2*4+1] != 0 || spatial[32+2*4+2] != 0 {
		t.Fatalf("spatial images/gap were not rendered correctly: %v", spatial[32:64])
	}
	for _, pan := range []float64{-4, 0} {
		if err := r.drawScene(0, 8, 4, scene, 1, pan, 0, false, 0); err != nil {
			t.Fatal(err)
		}
		if r.textures[red][0].bitmap != redTexture || r.textures[green][0].bitmap != greenTexture {
			t.Fatal("spatial pan reuploaded images")
		}
	}
	if err := r.drawScene(0, 8, 4, nil, 1, 0, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	if len(r.textures) != 0 || r.image != nil {
		t.Fatal("clearing did not release image resources")
	}
	// Draw across a texture boundary at source x=4094 with linear filtering.
	large := &decodedImage{width: 9000, height: 1, pixels: make([]byte, 9000*4)}
	for i := 0; i < len(large.pixels); i += 4 {
		large.pixels[i] = 255
		large.pixels[i+3] = 255
	}
	if err := r.draw(0, 8, 4, large, 1, -4090, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	pixels := make([]byte, 8*4*4)
	if err := d2dError("Read tiled pixels", bitmap.call(7, 0, 32, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 8; x++ {
		if pixels[x*4] != 255 || pixels[x*4+3] != 255 {
			t.Fatalf("tile seam at %d: %v", x, pixels[:32])
		}
	}
	checkWorkspaceOverlayPixels(t, wic, factory, &format)
}

func checkWorkspaceOverlayPixels(t *testing.T, wic, factory *d2dObject, format *windows.GUID) {
	t.Helper()
	const width, height = 400, 80
	var bitmap *d2dObject
	if err := d2dError("Create overlay surface", wic.call(17, width, height, uintptr(unsafe.Pointer(format)), 2, uintptr(unsafe.Pointer(&bitmap)))); err != nil {
		t.Fatal(err)
	}
	defer release(&bitmap)
	factory.call(1)
	r := &gpuRenderer{factory: factory, width: width, height: height, showBoundary: true, boundaryX: 380, caption: "sample.png  •  100 × 80  •  4 KiB", captionRect: d2dRect{left: 0, top: 40, right: 350, bottom: 66}}
	defer r.close()
	props := d2dTargetProperties{kind: 1, format: d2dPixelFormat{87, 1}, dpiX: 96, dpiY: 96}
	if err := d2dError("Create overlay target", factory.call(13, uintptr(unsafe.Pointer(bitmap)), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&r.target)))); err != nil {
		t.Fatal(err)
	}
	if err := r.drawScene(0, width, height, nil, 1, 0, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	pixels := make([]byte, width*height*4)
	if err := d2dError("Read overlay", bitmap.call(7, 0, width*4, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
		t.Fatal(err)
	}
	if pixels[(30*width+380)*4] < 200 {
		t.Fatal("wrap boundary line was not drawn")
	}
	firstTextX := width
	for y := 40; y < 66; y++ {
		for x := 0; x < 350; x++ {
			offset := (y*width + x) * 4
			if pixels[offset] > 100 && pixels[offset+1] > 100 && pixels[offset+2] > 100 {
				firstTextX = min(firstTextX, x)
			}
		}
	}
	if firstTextX < 8 || firstTextX > 24 {
		t.Fatalf("caption was not left aligned/rendered: first text x=%d", firstTextX)
	}
	longCaption := strings.Repeat("long filename ", 20) + ".png"
	measured, err := r.measureCaption(longCaption)
	if err != nil || measured <= 560 {
		t.Fatalf("long caption remained width limited: %v, %v", measured, err)
	}
	if cached, err := r.measureCaption(longCaption); err != nil || cached != measured {
		t.Fatal("caption measurement cache changed its width")
	}
	r.caption = ""
	r.showBoundary = false
	r.captions = []imageCaption{{text: "always visible.png", bounds: d2dRect{left: 0, top: 40, right: 350, bottom: 66}}}
	red := &decodedImage{width: width, height: height, pixels: make([]byte, width*height*4)}
	for i := 0; i < len(red.pixels); i += 4 {
		red.pixels[i+2] = 255
		red.pixels[i+3] = 255
	}
	readCaptionPixel := func(im *decodedImage) []byte {
		t.Helper()
		if err := r.drawScene(0, width, height, []sceneImage{{image: im}}, 1, 0, 0, false, 0xffffff); err != nil {
			t.Fatal(err)
		}
		if err := d2dError("Read caption stacking", bitmap.call(7, 0, width*4, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
			t.Fatal(err)
		}
		offset := 50 * width * 4
		return append([]byte(nil), pixels[offset:offset+4]...)
	}
	if p := readCaptionPixel(red); p[2] > 80 || p[1] == 0 {
		t.Fatalf("normal caption was not above images: %v", p)
	}
	r.captions[0].foreground = true
	if p := readCaptionPixel(red); p[2] > 80 || p[1] == 0 {
		t.Fatalf("hover caption was not in front: %v", p)
	}
	r.captions[0].foreground = false
	short := &decodedImage{width: width, height: 40, pixels: append([]byte(nil), red.pixels[:width*40*4]...)}
	if p := readCaptionPixel(short); p[0] > 80 || p[1] > 80 || p[2] > 80 {
		t.Fatalf("non-hovered caption not visible below its image: %v", p)
	}
	r.captions = nil
	r.feedbackText = "Image path copied"
	r.feedbackAlpha = 1
	readCaptionPixel(red)
	offset := ((height-25)*width + 10) * 4
	if pixels[offset+2] != 255 || pixels[offset+1] != 0 {
		t.Fatal("action message unexpectedly drew a background")
	}
	textPixels := 0
	for y := height - 28; y < height; y++ {
		for x := 0; x < width; x++ {
			if pixels[(y*width+x)*4+1] > 0 {
				textPixels++
			}
		}
	}
	if textPixels == 0 {
		t.Fatal("action message text was not drawn")
	}
	r.feedbackAlpha = 0
	readCaptionPixel(red)
	if pixels[offset+2] != 255 {
		t.Fatal("expired action message remained visible")
	}
	r.feedbackText = ""
	r.highlightBounds = true
	square := &decodedImage{width: 32, height: 32, pixels: make([]byte, 32*32*4)}
	if err := r.drawScene(0, width, height, []sceneImage{{image: square, x: 20, selected: true}, {image: square, x: 52}}, 1, 0, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	if err := d2dError("Read selection stacking", bitmap.call(7, 0, width*4, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
		t.Fatal(err)
	}
	edge := (16*width + 52) * 4
	if pixels[edge] < 230 || pixels[edge+1] < 230 || pixels[edge+2] < 230 {
		t.Fatal("shared edge was not drawn with the white selection above blue bounds")
	}
	cover := &decodedImage{width: 100, height: 40, pixels: make([]byte, 100*40*4)}
	for i := 0; i < len(cover.pixels); i += 4 {
		cover.pixels[i+2], cover.pixels[i+3] = 255, 255
	}
	for _, selected := range []bool{false, true} {
		// The later opaque image covers the earlier image's right boundary.
		// Both blue and selected white outlines must survive above its pixels.
		if err := r.drawScene(0, width, height, []sceneImage{{image: square, x: 20, selected: selected}, {image: cover, x: 35}}, 1, 0, 0, false, 0); err != nil {
			t.Fatal(err)
		}
		if err := d2dError("Read outline above overlapping image", bitmap.call(7, 0, width*4, uintptr(len(pixels)), uintptr(unsafe.Pointer(&pixels[0])))); err != nil {
			t.Fatal(err)
		}
		if pixels[edge] < 230 || selected && (pixels[edge+1] < 230 || pixels[edge+2] < 230) || !selected && pixels[edge+2] > 80 {
			t.Fatalf("outline was covered by a later image (selected=%v): %v", selected, pixels[edge:edge+4])
		}
	}
}

func TestTileCoverageAndGutters(t *testing.T) {
	const width, height, limit = 9000, 5000, 4096
	tiles := tileRegions(width, height, limit)
	area := 0
	for i, tile := range tiles {
		area += tile.core.Dx() * tile.core.Dy()
		if tile.pixels.Dx() > limit || tile.pixels.Dy() > limit || !tile.core.In(tile.pixels) {
			t.Fatal("invalid tile dimensions")
		}
		if tile.pixels != tile.core.Inset(-1).Intersect(image.Rect(0, 0, width, height)) {
			t.Fatal("missing sampling gutter")
		}
		for j := 0; j < i; j++ {
			if !tile.core.Intersect(tiles[j].core).Empty() {
				t.Fatal("tile cores overlap")
			}
		}
	}
	if area != width*height {
		t.Fatal("tiles do not cover image")
	}
}
