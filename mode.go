//go:build windows

package main

func (v *viewer) modeText() string {
	if v.gapInput != nil || v.placingWrap {
		return "Input Mode"
	}
	if v.highlightBounds || v.ctrlViewPinned {
		return "Inspect Mode"
	}
	return ""
}
