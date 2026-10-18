//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	staticButtonCallback uintptr
	drawButtonTextProc   = userDLL.NewProc("DrawTextW")
	drawButtonImageProc  = userDLL.NewProc("DrawStateW")
)

func installStaticButton(hwnd win.HWND, dark bool) {
	if staticButtonCallback == 0 {
		staticButtonCallback = syscall.NewCallback(staticButtonProc)
	}
	var theme uintptr
	if dark {
		theme = 1
	}
	setSubclass.Call(uintptr(hwnd), staticButtonCallback, 5, theme)
	win.InvalidateRect(hwnd, nil, false)
}

func staticButtonProc(hwnd win.HWND, message uint32, wParam uintptr, lParam unsafe.Pointer, id, dark uintptr) uintptr {
	switch message {
	case win.WM_ERASEBKGND:
		return 1
	case win.WM_PAINT:
		var paint win.PAINTSTRUCT
		dc := win.BeginPaint(hwnd, &paint)
		paintStaticButton(hwnd, dc, dark != 0)
		win.EndPaint(hwnd, &paint)
		return 0
	case win.WM_PRINTCLIENT:
		paintStaticButton(hwnd, win.HDC(wParam), dark != 0)
		return 0
	case win.WM_NCDESTROY:
		removeSubclass.Call(uintptr(hwnd), staticButtonCallback, id)
	}
	result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
	return result
}

func paintStaticButton(hwnd win.HWND, dc win.HDC, dark bool) {
	var bounds win.RECT
	win.GetClientRect(hwnd, &bounds)
	state := win.SendMessage(hwnd, win.BM_GETSTATE, 0, 0)
	fill, border, text := uint32(0x00e4e4e4), uint32(0x009b9b9b), uint32(0x00202020)
	if dark {
		fill, border, text = 0x00333333, 0x00151515, 0x00e6e6e6
	}
	if state&0x0200 != 0 { // BST_HOT.
		if dark {
			fill = 0x00444444
		} else {
			fill = 0x00f2f2f2
		}
	}
	if state&win.BST_PUSHED != 0 {
		if dark {
			fill = 0x00272727
		} else {
			fill = 0x00cccccc
		}
	}
	if state&win.BST_FOCUS != 0 {
		border = 0x00e69b50
	}
	if !win.IsWindowEnabled(hwnd) {
		text = 0x00808080
	}
	fillNativeRect(dc, bounds, border)
	inner := bounds
	inner.Left++
	inner.Top++
	inner.Right--
	inner.Bottom--
	fillNativeRect(dc, inner, fill)
	image := win.SendMessage(hwnd, win.BM_GETIMAGE, win.IMAGE_BITMAP, 0)
	if image != 0 {
		var bitmap win.BITMAP
		win.GetObject(win.HGDIOBJ(image), unsafe.Sizeof(bitmap), unsafe.Pointer(&bitmap))
		flags := uintptr(4) // DST_BITMAP.
		if !win.IsWindowEnabled(hwnd) {
			flags |= 0x20
		}
		drawButtonImageProc.Call(uintptr(dc), 0, 0, image, 0, uintptr((bounds.Right-bitmap.BmWidth)/2), uintptr((bounds.Bottom-bitmap.BmHeight)/2), uintptr(bitmap.BmWidth), uintptr(bitmap.BmHeight), flags)
	}
	length := int(win.SendMessage(hwnd, win.WM_GETTEXTLENGTH, 0, 0))
	if length == 0 {
		return
	}
	characters := make([]uint16, length+1)
	win.SendMessage(hwnd, win.WM_GETTEXT, uintptr(len(characters)), uintptr(unsafe.Pointer(&characters[0])))
	font := win.SendMessage(hwnd, win.WM_GETFONT, 0, 0)
	oldFont := win.SelectObject(dc, win.HGDIOBJ(font))
	defer win.SelectObject(dc, oldFont)
	win.SetTextColor(dc, win.COLORREF(text))
	win.SetBkMode(dc, win.TRANSPARENT)
	drawButtonTextProc.Call(uintptr(dc), uintptr(unsafe.Pointer(&characters[0])), uintptr(length), uintptr(unsafe.Pointer(&inner)), 0x25) // Centre, vertical centre, single line.
}
