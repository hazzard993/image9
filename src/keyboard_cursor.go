//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	cursorHookViewer   *viewer
	cursorHookCallback uintptr
	showCursorProc     = userDLL.NewProc("ShowCursor")
)

func (v *viewer) hideKeyboardCursor() {
	if v.cursorHidden {
		return
	}
	if !win.GetCursorPos(&v.cursorHiddenAt) {
		return
	}
	showCursorProc.Call(0)
	v.cursorHidden = true
}

func (v *viewer) showKeyboardCursor() {
	if !v.cursorHidden {
		return
	}
	showCursorProc.Call(1)
	v.cursorHidden = false
}

func (v *viewer) revealCursorOnMovement(position win.POINT) {
	if v.cursorHidden && position != v.cursorHiddenAt {
		v.showKeyboardCursor()
	}
}

func (v *viewer) installKeyboardCursor() error {
	if cursorHookCallback == 0 {
		cursorHookCallback = syscall.NewCallback(func(code int32, removed uintptr, data unsafe.Pointer) uintptr {
			if code >= 0 && removed == 1 && cursorHookViewer != nil {
				v := cursorHookViewer
				message := (*win.MSG)(data)
				switch message.Message {
				case win.WM_MOUSEMOVE, win.WM_NCMOUSEMOVE:
					var position win.POINT
					if win.GetCursorPos(&position) {
						v.revealCursorOnMovement(position)
					}
				case win.WM_KEYDOWN:
					if message.HWnd == v.window.Handle() || win.IsChild(v.window.Handle(), message.HWnd) {
						key, modifiers := walk.Key(message.WParam), walk.ModifiersDown()
						actions := v.window.ShortcutActions()
						for i := 0; i < actions.Len(); i++ {
							action := actions.At(i)
							if action.Enabled() && action.Visible() && action.Shortcut() == (walk.Shortcut{Key: key, Modifiers: modifiers}) {
								v.hideKeyboardCursor()
								break
							}
						}
					}
				}
			}
			result, _, _ := nextGapHookProc.Call(0, uintptr(code), removed, uintptr(data))
			return result
		})
	}
	hook, _, err := setGapHookProc.Call(3, cursorHookCallback, 0, uintptr(windows.GetCurrentThreadId()))
	if hook == 0 {
		return fmt.Errorf("could not install shortcut cursor handling: %w", err)
	}
	v.cursorKeyboardHook = hook
	cursorHookViewer = v
	return nil
}

func (v *viewer) closeKeyboardCursor() {
	v.showKeyboardCursor()
	if v.cursorKeyboardHook != 0 {
		unhookGapProc.Call(v.cursorKeyboardHook)
		v.cursorKeyboardHook = 0
	}
	if cursorHookViewer == v {
		cursorHookViewer = nil
	}
}
