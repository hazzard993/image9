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
	image           *decodedImage
	width, height   int
	hardware        bool
}

func (r *gpuRenderer) disposeTarget() {
	for i := range r.tiles {
		release(&r.tiles[i].bitmap)
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
	if r.image == im {
		return nil
	}
	for i := range r.tiles {
		release(&r.tiles[i].bitmap)
	}
	r.tiles = nil
	r.image = nil
	if im == nil {
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
	r.tiles, r.image = tiles, im
	return nil
}

func (r *gpuRenderer) draw(hwnd win.HWND, width, height int, im *decodedImage, scale, x, y float64, linear bool, background win.COLORREF) error {
	if width <= 0 || height <= 0 {
		return nil
	}
	if err := r.ensure(hwnd, width, height); err != nil {
		return err
	}
	if err := r.upload(im); err != nil {
		return err
	}
	colour := d2dColour{float32(background&255) / 255, float32((background>>8)&255) / 255, float32((background>>16)&255) / 255, 1}
	r.target.call(48) // BeginDraw
	r.target.call(47, uintptr(unsafe.Pointer(&colour)))
	mode := uintptr(0)
	if linear {
		mode = 1
	}
	for _, tile := range r.tiles {
		dst := d2dRect{float32(x + float64(tile.core.Min.X)*scale), float32(y + float64(tile.core.Min.Y)*scale), float32(x + float64(tile.core.Max.X)*scale), float32(y + float64(tile.core.Max.Y)*scale)}
		if dst.right <= 0 || dst.bottom <= 0 || dst.left >= float32(width) || dst.top >= float32(height) {
			continue
		}
		src := d2dRect{float32(tile.core.Min.X - tile.pixels.Min.X), float32(tile.core.Min.Y - tile.pixels.Min.Y), float32(tile.core.Max.X - tile.pixels.Min.X), float32(tile.core.Max.Y - tile.pixels.Min.Y)}
		r.target.call(26, uintptr(unsafe.Pointer(tile.bitmap)), uintptr(unsafe.Pointer(&dst)), uintptr(math.Float32bits(1)), mode, uintptr(unsafe.Pointer(&src)))
	}
	hr := r.target.call(49, 0, 0) // EndDraw presents the completed frame.
	if uint32(hr) == 0x8899000c {
		r.disposeTarget()
		return errRecreateTarget
	}
	return d2dError("Present", hr)
}

var errRecreateTarget = fmt.Errorf("Direct2D device needs recreation")
