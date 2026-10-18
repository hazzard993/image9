//go:build windows

package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func (v *viewer) wrapLineX() float64 {
	b := v.canvas.ClientBoundsPixels()
	if v.wrapWidth == 0 {
		return float64(b.Width - 16)
	}
	s := v.displayScale()
	left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	return left + float64(v.wrapWidth)*s
}
func (v *viewer) nearWrapLine(x int) bool {
	return v.highlightBounds && math.Abs(float64(x)-v.wrapLineX()) <= 7
}
func (v *viewer) setWrapAt(x int) {
	b := v.canvas.ClientBoundsPixels()
	s := v.displayScale()
	left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	limit := max(64, int(math.Round((float64(x)-left)/s/8))*8)
	if limit != v.wrapWidth {
		v.wrapWidth = limit
		v.arrangeWorkspace()
	}
}

func (v *viewer) trackWorkspacePointer(x, y int) {
	wasInside := v.pointerInside
	v.pointerInside = true
	v.pointerX, v.pointerY = x, y
	index, _, _ := v.hitImage(x, y)
	if index != v.hoverIndex || !wasInside {
		v.hoverIndex = index
		v.canvas.Invalidate()
	}
	if v.nearWrapLine(x) || v.resizingColumns {
		v.canvas.SetCursor(walk.CursorSizeWE())
	} else if v.colourPicking {
		v.canvas.SetCursor(walk.CursorCross())
	} else {
		v.canvas.SetCursor(walk.CursorArrow())
	}
	if !wasInside {
		tracking := win.TRACKMOUSEEVENT{DwFlags: win.TME_LEAVE, HwndTrack: v.canvas.Handle()}
		tracking.CbSize = uint32(unsafe.Sizeof(tracking))
		win.TrackMouseEvent(&tracking)
	}
}

func (v *viewer) prepareHoverCaption(scale, x, y float64, width, height int) {
	v.renderer.caption = ""
	v.renderer.captions = v.renderer.captions[:0]
	if !v.highlightBounds {
		return
	}
	hovered := -1
	if v.pointerInside && !v.resizingColumns && v.pointerX >= 0 && v.pointerY >= 0 && v.pointerX < width && v.pointerY < height {
		hovered, _, _ = v.hitImage(v.pointerX, v.pointerY)
	}
	for index, item := range v.workspace {
		if item.image == nil {
			continue
		}
		left := x + float64(item.x)*scale
		top := y + float64(item.y+item.height)*scale
		right := left + math.Min(560, math.Max(240, float64(item.width)*scale))
		if right <= 0 || left >= float64(width) || top+26 <= 0 || top >= float64(height) {
			continue
		}
		text := fmt.Sprintf("%s  •  %d × %d  •  %s", filepath.Base(item.path), item.width, item.height, formatFileSize(item.image.fileSize))
		v.renderer.captions = append(v.renderer.captions, imageCaption{text: text, bounds: d2dRect{left: float32(left), top: float32(top), right: float32(right), bottom: float32(top + 26)}, foreground: index == hovered})
	}
}

var pointerWindows sync.Map
var pointerCallback uintptr

func (v *viewer) installPointerTracking() {
	if pointerCallback == 0 {
		pointerCallback = syscall.NewCallback(workspacePointerProc)
	}
	pointerWindows.Store(v.canvas.Handle(), v)
	setSubclass.Call(uintptr(v.canvas.Handle()), pointerCallback, 3, 0)
}
func workspacePointerProc(hwnd win.HWND, message uint32, wParam uintptr, lParam unsafe.Pointer, id, data uintptr) uintptr {
	if value, ok := pointerWindows.Load(hwnd); ok {
		v := value.(*viewer)
		if message == win.WM_MOUSELEAVE {
			v.pointerInside = false
			v.hoverIndex = -1
			v.canvas.Invalidate()
		}
		if message == win.WM_NCDESTROY {
			pointerWindows.Delete(hwnd)
			removeSubclass.Call(uintptr(hwnd), pointerCallback, id)
		}
	}
	result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
	return result
}
