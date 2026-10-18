//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	commonControls       = windows.NewLazySystemDLL("comctl32.dll")
	setSubclass          = commonControls.NewProc("SetWindowSubclass")
	removeSubclass       = commonControls.NewProc("RemoveWindowSubclass")
	defaultSubclass      = commonControls.NewProc("DefSubclassProc")
	userDLL              = windows.NewLazySystemDLL("user32.dll")
	fillRectProc         = userDLL.NewProc("FillRect")
	menuBarInfoProc      = userDLL.NewProc("GetMenuBarInfo")
	menuStringProc       = userDLL.NewProc("GetMenuStringW")
	getMenuProc          = userDLL.NewProc("GetMenu")
	getWindowDCProc      = userDLL.NewProc("GetWindowDC")
	themePaintCallback   uintptr
	setDCBrushColourProc = windows.NewLazySystemDLL("gdi32.dll").NewProc("SetDCBrushColor")
)

// UAH menu painting preserves native menu navigation and accelerators. Structure
// prefixes follow adzm/win32-custom-menubar-aero-theme's UAHMenuBar.h.
type uahMenu struct {
	menu  win.HMENU
	dc    win.HDC
	flags uint32
}
type uahMenuItem struct {
	draw     win.DRAWITEMSTRUCT
	menu     uahMenu
	position int32
}
type menuBarInfo struct {
	size   uint32
	rect   win.RECT
	menu   win.HMENU
	window win.HWND
	flags  uint32
}

func installThemePainting(hwnd win.HWND, dark uintptr) {
	if themePaintCallback == 0 {
		themePaintCallback = syscall.NewCallback(themePaintProc)
	}
	setSubclass.Call(uintptr(hwnd), themePaintCallback, 1, dark)
}

func fillNativeRect(dc win.HDC, rect win.RECT, colour uint32) {
	brush := win.CreateBrushIndirect(&win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: win.COLORREF(colour)})
	if brush == 0 {
		return
	}
	defer win.DeleteObject(win.HGDIOBJ(brush))
	fillRectProc.Call(uintptr(dc), uintptr(unsafe.Pointer(&rect)), uintptr(brush))
}

func themePaintProc(hwnd win.HWND, message uint32, wParam uintptr, lParam unsafe.Pointer, id, dark uintptr) uintptr {
	if message == win.WM_NCDESTROY {
		removeSubclass.Call(uintptr(hwnd), themePaintCallback, id)
	}
	if dark != 0 && lParam != nil {
		switch message {
		case win.WM_CTLCOLORLISTBOX:
			dc := win.HDC(wParam)
			colour := win.COLORREF(themeSurface(true))
			win.SetTextColor(dc, 0x00e6e6e6)
			win.SetBkColor(dc, colour)
			setDCBrushColourProc.Call(uintptr(dc), uintptr(colour))
			return uintptr(win.GetStockObject(win.DC_BRUSH))
		case win.WM_NOTIFY:
			draw := (*win.NMCUSTOMDRAW)(lParam)
			if draw.Hdr.Code == win.NM_CUSTOMDRAW {
				if draw.DwDrawStage == win.CDDS_PREPAINT {
					return win.CDRF_NOTIFYPOSTPAINT
				}
				if draw.DwDrawStage == win.CDDS_POSTPAINT && draw.UItemState&(win.CDIS_HOT|win.CDIS_FOCUS|win.CDIS_SELECTED) == 0 {
					r := draw.Rc
					// One-pixel charcoal outline; leave interactive highlights native.
					for _, edge := range []win.RECT{{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Top + 1}, {Left: r.Left, Top: r.Bottom - 1, Right: r.Right, Bottom: r.Bottom}, {Left: r.Left, Top: r.Top, Right: r.Left + 1, Bottom: r.Bottom}, {Left: r.Right - 1, Top: r.Top, Right: r.Right, Bottom: r.Bottom}} {
						fillNativeRect(draw.Hdc, edge, 0x00151515)
					}
					return 0
				}
			}
		case 0x0091: // WM_UAHDRAWMENU
			menu := (*uahMenu)(lParam)
			info := menuBarInfo{}
			info.size = uint32(unsafe.Sizeof(info))
			ok, _, _ := menuBarInfoProc.Call(uintptr(hwnd), 0xfffffffd, 0, uintptr(unsafe.Pointer(&info))) // OBJID_MENU
			var window win.RECT
			if ok != 0 && win.GetWindowRect(hwnd, &window) {
				r := info.rect
				r.Left -= window.Left
				r.Right -= window.Left
				r.Top -= window.Top
				r.Bottom -= window.Top
				fillNativeRect(menu.dc, r, 0x00292626)
				return 0
			}
		case 0x0092: // WM_UAHDRAWMENUITEM
			item := (*uahMenuItem)(lParam)
			bg := uint32(0x00292626)
			if item.draw.ItemState&(0x0040|win.ODS_SELECTED) != 0 {
				bg = 0x00484848
			}
			fillNativeRect(item.menu.dc, item.draw.RcItem, bg)
			var text [128]uint16
			count, _, _ := menuStringProc.Call(uintptr(item.menu.menu), uintptr(item.position), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), win.MF_BYPOSITION)
			oldColour := win.SetTextColor(item.menu.dc, 0x00e6e6e6)
			oldMode := win.SetBkMode(item.menu.dc, win.TRANSPARENT)
			flags := uint32(win.DT_CENTER | win.DT_VCENTER | win.DT_SINGLELINE)
			if item.draw.ItemState&0x0100 != 0 {
				flags |= win.DT_HIDEPREFIX
			}
			win.DrawTextEx(item.menu.dc, &text[0], int32(count), &item.draw.RcItem, flags, nil)
			win.SetTextColor(item.menu.dc, oldColour)
			win.SetBkMode(item.menu.dc, oldMode)
			return 0
		}
	}
	result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
	// Windows paints a light non-client separator below the native menu after
	// drawing its items. Repaint that seam after the default non-client handler.
	if dark != 0 && (message == win.WM_NCPAINT || message == win.WM_NCACTIVATE) {
		menu, _, _ := getMenuProc.Call(uintptr(hwnd))
		if menu == 0 {
			return result
		}
		var client, window win.RECT
		origin := win.POINT{}
		if win.GetClientRect(hwnd, &client) && win.GetWindowRect(hwnd, &window) && win.ClientToScreen(hwnd, &origin) {
			rawDC, _, _ := getWindowDCProc.Call(uintptr(hwnd))
			dc := win.HDC(rawDC)
			if dc != 0 {
				x, y := origin.X-window.Left, origin.Y-window.Top
				fillNativeRect(dc, win.RECT{Left: x, Top: y - 1, Right: x + client.Right, Bottom: y}, 0x00292626)
				win.ReleaseDC(hwnd, dc)
			}
		}
	}
	return result
}
