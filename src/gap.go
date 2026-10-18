//go:build windows

package main

import (
	"fmt"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	gapCaptureViewer   *viewer
	gapCaptureCallback uintptr
	setGapHookProc     = userDLL.NewProc("SetWindowsHookExW")
	unhookGapProc      = userDLL.NewProc("UnhookWindowsHookEx")
	nextGapHookProc    = userDLL.NewProc("CallNextHookEx")
)

func (v *viewer) captureGapKeyboard() error {
	if gapCaptureCallback == 0 {
		gapCaptureCallback = syscall.NewCallback(func(code int32, removed uintptr, data unsafe.Pointer) uintptr {
			if code >= 0 && removed == 1 && gapCaptureViewer != nil {
				v := gapCaptureViewer
				message := (*win.MSG)(data)
				switch message.Message {
				case win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
					v.hideKeyboardCursor()
					key := walk.Key(message.WParam)
					if message.LParam&(1<<30) == 0 || key != walk.KeyG && key != walk.KeyW {
						if v.placingWrap {
							v.handleWrapKey(key)
						} else if v.gapInput != nil {
							v.handleGapKey(key)
						}
					}
					message.Message = win.WM_NULL
				case win.WM_KEYUP, win.WM_SYSKEYUP, win.WM_CHAR, win.WM_SYSCHAR:
					message.Message = win.WM_NULL
				}
			}
			result, _, _ := nextGapHookProc.Call(0, uintptr(code), removed, uintptr(data))
			return result
		})
	}
	hook, _, err := setGapHookProc.Call(3, gapCaptureCallback, 0, uintptr(windows.GetCurrentThreadId())) // WH_GETMESSAGE, UI thread only.
	if hook == 0 {
		return fmt.Errorf("could not capture numeric input: %w", err)
	}
	v.gapKeyboardHook = hook
	gapCaptureViewer = v
	return nil
}

func (v *viewer) releaseGapKeyboard() {
	if v.gapKeyboardHook != 0 {
		unhookGapProc.Call(v.gapKeyboardHook)
		v.gapKeyboardHook = 0
	}
	if gapCaptureViewer == v {
		gapCaptureViewer = nil
	}
}

func (v *viewer) beginGapInput() {
	if v.gapInput != nil {
		return
	}
	v.focusImage()
	input := ""
	v.gapInput = &input
	if err := v.captureGapKeyboard(); err != nil {
		v.gapInput = nil
		v.showError(err)
		return
	}
	// Prevent number keys, Escape and other shortcuts from acting while typing.
	actions := v.window.ShortcutActions()
	for i := 0; i < actions.Len(); i++ {
		action := actions.At(i)
		if action.Enabled() {
			v.gapShortcuts = append(v.gapShortcuts, action)
			action.SetEnabled(false)
		}
	}
	v.showGapInput()
}

func (v *viewer) showGapInput() {
	gap := workspaceGap
	if v.customGap {
		gap = v.imageGap
	}
	v.notifyAction(fmt.Sprintf("Image gap: %s_ px (current %d px; Enter to apply, Esc to cancel)", *v.gapInput, gap))
}

func (v *viewer) endGapInput(apply bool) {
	if v.gapInput == nil {
		return
	}
	input := *v.gapInput
	v.gapInput = nil
	v.releaseGapKeyboard()
	for _, action := range v.gapShortcuts {
		action.SetEnabled(true)
	}
	v.gapShortcuts = nil
	if !apply {
		v.notifyAction("Gap setting cancelled")
		return
	}
	oldGap := workspaceGap
	if v.customGap {
		oldGap = v.imageGap
	}
	gap, err := v.evaluatePixelExpression(input, oldGap)
	if err != nil {
		v.notifyAction("Invalid gap - unchanged")
		return
	}
	v.wrapWidth = wrapWidthForGap(v.workspace, v.wrapWidth, oldGap, gap)
	v.imageGap, v.customGap = gap, true
	v.arrangeWorkspace()
	v.notifyAction(fmt.Sprintf("Image gap: %d px", gap))
}

func wrapWidthForGap(items []workspaceImage, width, oldGap, newGap int) int {
	if width == 0 {
		return 0
	}
	maxGaps, rowGaps := 0, 0
	for i := 1; i < len(items); i++ {
		if items[i].y == items[i-1].y {
			rowGaps++
			maxGaps = max(maxGaps, rowGaps)
		} else {
			rowGaps = 0
		}
	}
	return max(1, width+(newGap-oldGap)*maxGaps)
}

func (v *viewer) handleGapKey(key walk.Key) {
	if key == walk.KeyControl || key == walk.KeyShift || key == walk.KeyAlt {
		return
	}
	// Gap entry also starts from Ctrl view; keeping Ctrl held must not
	// discard number keys, Enter, or the second-G shortcut.
	if character, ok := pixelInputCharacter(key, walk.ModifiersDown()); ok {
		v.appendGapDigit(character)
		return
	}
	if walk.ModifiersDown() & ^walk.ModControl != 0 {
		v.endGapInput(false)
		return
	}
	switch {
	case key == walk.KeyG:
		gap := v.imageGap
		if !v.customGap {
			gap = workspaceGap
		}
		if gap == 0 {
			*v.gapInput = strconv.Itoa(workspaceGap)
		} else {
			*v.gapInput = "0"
		}
		v.endGapInput(true)
	case key == walk.KeyEscape:
		v.endGapInput(false)
	case key == walk.KeyReturn:
		v.endGapInput(true)
	case key == walk.KeyBack:
		if len(*v.gapInput) > 0 {
			*v.gapInput = (*v.gapInput)[:len(*v.gapInput)-1]
		}
		v.showGapInput()
	case key >= walk.Key0 && key <= walk.Key9:
		v.appendGapDigit(byte(key-walk.Key0) + '0')
	case key >= walk.KeyNumpad0 && key <= walk.KeyNumpad9:
		v.appendGapDigit(byte(key-walk.KeyNumpad0) + '0')
	default:
		v.endGapInput(false)
	}
}

func (v *viewer) appendGapDigit(digit byte) {
	input := *v.gapInput + string(digit)
	if len(input) > 128 {
		v.endGapInput(false)
		return
	}
	*v.gapInput = input
	v.showGapInput()
}
