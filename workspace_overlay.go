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
	if v.placingWrap && v.pointerInside {
		s := v.displayScale()
		left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
		return left + float64(v.wrapLimitAt(v.pointerX))*s
	}
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
func (v *viewer) wrapLimitAt(x int) int {
	b := v.canvas.ClientBoundsPixels()
	s := v.displayScale()
	left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	return max(64, int(math.Round((float64(x)-left)/s/8))*8)
}

func (v *viewer) setWrapAt(x int) {
	limit := v.wrapLimitAt(x)
	if limit != v.wrapWidth {
		v.wrapWidth = limit
		v.arrangeWorkspace()
		v.notifyAction(fmt.Sprintf("Wrap width: %d px", limit))
	}
}

func (v *viewer) resetWrap() {
	v.endWrapPlacement()
	v.stopDrag()
	v.canvas.SetCursor(walk.CursorArrow())
	v.wrapWidth = 0
	v.arrangeWorkspace()
	v.notifyAction("Wrap limit reset - single row")
}

func (v *viewer) toggleWrapMode() {
	if v.placingWrap {
		v.resetWrap()
		return
	}
	if len(v.workspace) == 0 {
		return
	}
	v.focusImage()
	v.placingWrap = true
	v.wrapInput = ""
	if err := v.captureGapKeyboard(); err != nil {
		v.placingWrap = false
		v.showError(err)
		return
	}
	v.canvas.SetCursor(walk.CursorSizeWE())
	v.showWrapInput()
}

func (v *viewer) placeWrapAt(x int) {
	v.scale = v.displayScale()
	v.fit = false
	v.endWrapPlacement()
	v.setWrapAt(x)
	v.canvas.SetCursor(walk.CursorArrow())
	v.canvas.Invalidate()
}

func (v *viewer) trackWorkspacePointer(x, y int) {
	wasInside := v.pointerInside
	v.pointerInside = true
	v.pointerX, v.pointerY = x, y
	index, _, _ := v.hitImage(x, y)
	if index != v.hoverIndex || !wasInside || v.placingWrap {
		v.hoverIndex = index
		v.canvas.Invalidate()
	}
	if v.placingWrap || v.nearWrapLine(x) || v.resizingColumns {
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
	v.renderer.shortcutLines = nil
	if !v.highlightBounds {
		return
	}
	hint := "H / T - Show filename labels"
	if v.showFilenameLabels {
		hint = "H / T - Hide filename labels"
	}
	gap := workspaceGap
	if v.customGap {
		gap = v.imageGap
	}
	v.renderer.shortcutLines = []string{hint, "C - Copy focused image path", fmt.Sprintf("G - Gap: %d px (G again: 0 / default)", gap), "L - Image list / reorder", "O - Open containing folder", "F - Fit all images"}
	if v.ctrlViewPinned {
		v.renderer.shortcutLines[0] = "H / J / K / L - Left / down / up / right"
		v.renderer.shortcutLines[3] = "L - Image list / reorder (after Esc)"
		v.renderer.shortcutLines = append(v.renderer.shortcutLines, "Esc - Exit Inspect Mode")
		v.renderer.shortcutLines = append(v.renderer.shortcutLines, "T - Toggle filename labels")
	}
	// Bottom-align the shortcut block to workspace (0, 0), where the first
	// image begins. The same anchor exists when the workspace is empty.
	v.renderer.shortcutX = float32(x)
	v.renderer.shortcutY = float32(y) - float32(len(v.renderer.shortcutLines)*22)
	if !v.showFilenameLabels {
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
		right := float64(width)
		if left >= float64(width) || top+26 <= 0 || top >= float64(height) {
			continue
		}
		text := fmt.Sprintf("%s  •  %d × %d  •  %s", filepath.Base(item.path), item.width, item.height, formatFileSize(item.image.fileSize))
		v.renderer.captions = append(v.renderer.captions, imageCaption{text: text, bounds: d2dRect{left: float32(left), top: float32(top), right: float32(right), bottom: float32(top + 26)}, foreground: index == hovered})
	}
}

var pointerWindows sync.Map

var pointerCallback uintptr

func (v *viewer) toggleCtrlText() {
	v.showFilenameLabels = !v.showFilenameLabels
	if !v.showFilenameLabels {
		v.notifyAction("Filename labels hidden - H to show")
	} else {
		v.notifyAction("Filename labels shown")
	}
}

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
		if message == win.WM_GETDLGCODE {
			if v.gapInput != nil || v.placingWrap || walk.Key(wParam) == walk.KeyReturn {
				return win.DLGC_WANTALLKEYS | win.DLGC_WANTCHARS
			}
			// Keep queued characters out of IsDialogMessage even after Enter
			// exits gap mode, so they reach our silent WM_CHAR handler.
			result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
			return result | win.DLGC_WANTCHARS
		}
		// TranslateMessage queues WM_CHAR before WM_KEYDOWN is dispatched.
		// Enter may finish gap entry before its character arrives. The image
		// canvas has no text input, so consume characters even after mode exit.
		if message == win.WM_CHAR {
			return 0
		}
		if v.placingWrap {
			switch message {
			case win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
				if uintptr(lParam)&(1<<30) == 0 || walk.Key(wParam) != walk.KeyW {
					v.handleWrapKey(walk.Key(wParam))
				}
				return 0
			case win.WM_KILLFOCUS:
				v.endWrapPlacement()
				v.notifyAction("Wrap placement cancelled")
			}
		}
		if v.gapInput != nil {
			switch message {
			case win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
				if uintptr(lParam)&(1<<30) == 0 || walk.Key(wParam) != walk.KeyG {
					v.handleGapKey(walk.Key(wParam))
				}
				return 0
			case win.WM_CHAR, win.WM_SYSCHAR:
				return 0
			case win.WM_KILLFOCUS:
				v.endGapInput(false)
			}
		}
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
