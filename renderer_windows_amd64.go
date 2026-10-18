package main

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

// Small Direct2D 1.0 COM binding. All calls and resources stay on the UI thread.
type d2dObject struct{ vtable *[61]uintptr }

func (o *d2dObject) call(slot int, args ...uintptr) uintptr {
	var all [8]uintptr
	all[0] = uintptr(unsafe.Pointer(o))
	copy(all[1:], args)
	r, _, _ := syscall.SyscallN(o.vtable[slot], all[:len(args)+1]...)
	runtime.KeepAlive(o)
	return r
}
func release(o **d2dObject) {
	if *o != nil {
		(*o).call(2)
		*o = nil
	}
}
func d2dError(operation string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s: Direct2D error 0x%08X", operation, uint32(hr))
	}
	return nil
}

type d2dPixelFormat struct{ format, alpha uint32 }
type d2dTargetProperties struct {
	kind            uint32
	format          d2dPixelFormat
	dpiX, dpiY      float32
	usage, minLevel uint32
}
type d2dWindowProperties struct {
	hwnd                   win.HWND
	width, height, options uint32
}
type d2dBitmapProperties struct {
	format     d2dPixelFormat
	dpiX, dpiY float32
}
type d2dRect struct{ left, top, right, bottom float32 }
type d2dColour struct{ r, g, b, a float32 }
type gpuTile struct {
	bitmap       *d2dObject
	core, pixels image.Rectangle
}

type gpuRenderer struct {
	factory, target *d2dObject
	tiles           []gpuTile
	textures        map[*decodedImage][]gpuTile
	image           *decodedImage
	width, height   int
	hardware        bool
	highlightBounds bool
	boundsBrush     *d2dObject
}

func (r *gpuRenderer) disposeTarget() {
	release(&r.boundsBrush)
	for im, tiles := range r.textures {
		for i := range tiles {
			release(&tiles[i].bitmap)
		}
		delete(r.textures, im)
	}
	r.tiles = nil
	r.image = nil
	release(&r.target)
}
func (r *gpuRenderer) close() { r.disposeTarget(); release(&r.factory) }

func (r *gpuRenderer) ensure(hwnd win.HWND, width, height int) error {
	if r.factory == nil {
		proc := windows.NewLazySystemDLL("d2d1.dll").NewProc("D2D1CreateFactory")
		if err := proc.Find(); err != nil {
			return err
		}
		iid := windows.GUID{Data1: 0x06152247, Data2: 0x6f50, Data3: 0x465a, Data4: [8]byte{0x92, 0x45, 0x11, 0x8b, 0xfd, 0x3b, 0x60, 0x07}}
		hr, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&iid)), 0, uintptr(unsafe.Pointer(&r.factory)))
		if err := d2dError("Create factory", hr); err != nil {
			return err
		}
	}
	if r.target == nil {
		properties := d2dTargetProperties{kind: 2, format: d2dPixelFormat{87, 3}, dpiX: 96, dpiY: 96}
		window := d2dWindowProperties{hwnd: hwnd, width: uint32(width), height: uint32(height)}
		hr := r.factory.call(14, uintptr(unsafe.Pointer(&properties)), uintptr(unsafe.Pointer(&window)), uintptr(unsafe.Pointer(&r.target)))
		r.hardware = int32(hr) >= 0
		if !r.hardware {
			properties.kind = 1 // Explicit software fallback when hardware is unavailable.
			hr = r.factory.call(14, uintptr(unsafe.Pointer(&properties)), uintptr(unsafe.Pointer(&window)), uintptr(unsafe.Pointer(&r.target)))
		}
		if err := d2dError("Create render target", hr); err != nil {
			return err
		}
		r.width, r.height = width, height
	} else if r.width != width || r.height != height {
		size := struct{ width, height uint32 }{uint32(width), uint32(height)}
		if err := d2dError("Resize", r.target.call(58, uintptr(unsafe.Pointer(&size)))); err != nil {
			r.disposeTarget()
			return err
		}
		r.width, r.height = width, height
	}
	return nil
}

func tileRegions(width, height, limit int) []gpuTile {
	var tiles []gpuTile
	bounds := image.Rect(0, 0, width, height)
	step := limit - 2 // A one-pixel sampling gutter prevents bilinear seams.
	for y := 0; y < height; y += step {
		for x := 0; x < width; x += step {
			core := image.Rect(x, y, min(x+step, width), min(y+step, height))
			tiles = append(tiles, gpuTile{core: core, pixels: core.Inset(-1).Intersect(bounds)})
		}
	}
	return tiles
}

func (r *gpuRenderer) upload(im *decodedImage) error {
	if im == nil {
		r.tiles = nil
		r.image = nil
		return nil
	}
	if tiles, ok := r.textures[im]; ok {
		r.tiles = tiles
		r.image = im
		return nil
	}
	limit := min(4096, int(r.target.call(55)))
	if limit < 3 {
		return fmt.Errorf("Direct2D bitmap size limit is invalid")
	}
	tiles := tileRegions(im.width, im.height, limit)
	props := d2dBitmapProperties{format: d2dPixelFormat{87, 1}, dpiX: 96, dpiY: 96}
	for i := range tiles {
		tile := &tiles[i]
		// SIZE_U is an eight-byte by-value structure on Windows x64.
		size := uintptr(uint64(uint32(tile.pixels.Dx())) | uint64(uint32(tile.pixels.Dy()))<<32)
		offset := (tile.pixels.Min.Y*im.width + tile.pixels.Min.X) * 4
		hr := r.target.call(4, size, uintptr(unsafe.Pointer(&im.pixels[offset])), uintptr(im.width*4), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&tile.bitmap)))
		runtime.KeepAlive(im)
		if err := d2dError("Upload bitmap", hr); err != nil {
			for j := range tiles {
				release(&tiles[j].bitmap)
			}
			return err
		}
	}
	if r.textures == nil {
		r.textures = make(map[*decodedImage][]gpuTile)
	}
	r.textures[im] = tiles
	r.tiles, r.image = tiles, im
	return nil
}

type sceneImage struct {
	image *decodedImage
	x, y  int
}

func (r *gpuRenderer) draw(hwnd win.HWND, width, height int, im *decodedImage, scale, x, y float64, linear bool, background win.COLORREF) error {
	var scene []sceneImage
	if im != nil {
		scene = []sceneImage{{image: im}}
	}
	return r.drawScene(hwnd, width, height, scene, scale, x, y, linear, background)
}

func (r *gpuRenderer) drawScene(hwnd win.HWND, width, height int, scene []sceneImage, scale, x, y float64, linear bool, background win.COLORREF) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if err := r.ensure(hwnd, width, height); err != nil {
		return err
	}
	keep := make(map[*decodedImage]bool, len(scene))
	for _, item := range scene {
		if item.image != nil {
			keep[item.image] = true
		}
	}
	for im, tiles := range r.textures {
		if !keep[im] {
			for i := range tiles {
				release(&tiles[i].bitmap)
			}
			delete(r.textures, im)
		}
	}
	if !keep[r.image] {
		r.image = nil
		r.tiles = nil
	}
	type preparedImage struct {
		im    *decodedImage
		tiles []gpuTile
		x, y  float64
	}
	prepared := make([]preparedImage, 0, len(scene))
	for _, item := range scene {
		im := item.image
		if im == nil {
			continue
		}
		left, top := x+float64(item.x)*scale, y+float64(item.y)*scale
		if left+float64(im.width)*scale <= 0 || top+float64(im.height)*scale <= 0 || left >= float64(width) || top >= float64(height) {
			continue
		}
		if err := r.upload(im); err != nil {
			return err
		}
		prepared = append(prepared, preparedImage{im: im, tiles: r.tiles, x: left, y: top})
	}
	if r.highlightBounds && len(prepared) > 0 && r.boundsBrush == nil {
		blue := d2dColour{r: 0.1, g: 0.5, b: 1, a: 1}
		if err := d2dError("Create bounds brush", r.target.call(8, uintptr(unsafe.Pointer(&blue)), 0, uintptr(unsafe.Pointer(&r.boundsBrush)))); err != nil {
			return err
		}
	}
	colour := d2dColour{float32(background&255) / 255, float32((background>>8)&255) / 255, float32((background>>16)&255) / 255, 1}
	r.target.call(48)
	r.target.call(47, uintptr(unsafe.Pointer(&colour)))
	mode := uintptr(0)
	if linear {
		mode = 1
	}
	for _, item := range prepared {
		for _, tile := range item.tiles {
			dst := d2dRect{float32(item.x + float64(tile.core.Min.X)*scale), float32(item.y + float64(tile.core.Min.Y)*scale), float32(item.x + float64(tile.core.Max.X)*scale), float32(item.y + float64(tile.core.Max.Y)*scale)}
			if dst.right <= 0 || dst.bottom <= 0 || dst.left >= float32(width) || dst.top >= float32(height) {
				continue
			}
			src := d2dRect{float32(tile.core.Min.X - tile.pixels.Min.X), float32(tile.core.Min.Y - tile.pixels.Min.Y), float32(tile.core.Max.X - tile.pixels.Min.X), float32(tile.core.Max.Y - tile.pixels.Min.Y)}
			r.target.call(26, uintptr(unsafe.Pointer(tile.bitmap)), uintptr(unsafe.Pointer(&dst)), uintptr(math.Float32bits(1)), mode, uintptr(unsafe.Pointer(&src)))
		}
		if r.highlightBounds {
			bounds := d2dRect{float32(item.x), float32(item.y), float32(item.x + float64(item.im.width)*scale), float32(item.y + float64(item.im.height)*scale)}
			r.target.call(16, uintptr(unsafe.Pointer(&bounds)), uintptr(unsafe.Pointer(r.boundsBrush)), uintptr(math.Float32bits(2)), 0)
		}
	}
	hr := r.target.call(49, 0, 0)
	if uint32(hr) == 0x8899000c {
		r.disposeTarget()
		return errRecreateTarget
	}
	return d2dError("Present", hr)
}

var errRecreateTarget = fmt.Errorf("Direct2D device needs recreation")
