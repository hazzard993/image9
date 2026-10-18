//go:build windows

package main

import (
	"fmt"

	"github.com/lxn/walk"
)

func (v *viewer) endWrapPlacement() {
	v.placingWrap = false
	v.wrapInput = ""
	v.releaseGapKeyboard()
}

func (v *viewer) showWrapInput() {
	v.notifyAction(fmt.Sprintf("Wrap width: %s_ px - type and Enter, or left-click; W clears, Esc cancels", v.wrapInput))
}

func (v *viewer) cancelWrapInput() {
	v.endWrapPlacement()
	v.canvas.SetCursor(walk.CursorArrow())
	v.notifyAction("Wrap setting unchanged")
}

func (v *viewer) handleWrapKey(key walk.Key) {
	if key == walk.KeyControl || key == walk.KeyShift || key == walk.KeyAlt {
		return
	}
	if character, ok := pixelInputCharacter(key, walk.ModifiersDown()); ok {
		v.appendWrapDigit(character)
		return
	}
	if walk.ModifiersDown() & ^walk.ModControl != 0 {
		v.cancelWrapInput()
		return
	}
	switch {
	case key == walk.KeyEscape:
		v.cancelWrapInput()
	case key == walk.KeyW:
		v.resetWrap()
	case key == walk.KeyReturn:
		width, err := v.evaluatePixelExpression(v.wrapInput, v.wrapWidth)
		if err != nil {
			v.cancelWrapInput()
			return
		}
		v.scale = v.displayScale()
		v.fit = false
		v.endWrapPlacement()
		v.wrapWidth = int(width)
		v.arrangeWorkspace()
		v.canvas.SetCursor(walk.CursorArrow())
		v.notifyAction(fmt.Sprintf("Wrap width: %d px", width))
	case key == walk.KeyBack:
		if len(v.wrapInput) > 0 {
			v.wrapInput = v.wrapInput[:len(v.wrapInput)-1]
		}
		v.showWrapInput()
	case key >= walk.Key0 && key <= walk.Key9:
		v.appendWrapDigit(byte(key-walk.Key0) + '0')
	case key >= walk.KeyNumpad0 && key <= walk.KeyNumpad9:
		v.appendWrapDigit(byte(key-walk.KeyNumpad0) + '0')
	default:
		v.cancelWrapInput()
	}
}

func (v *viewer) appendWrapDigit(digit byte) {
	if len(v.wrapInput) >= 128 {
		v.cancelWrapInput()
		return
	}
	v.wrapInput += string(digit)
	v.showWrapInput()
}
