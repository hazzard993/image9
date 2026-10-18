package main

import (
	"math"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type imageCaption struct {
	text       string
	bounds     d2dRect
	foreground bool
}

func (r *gpuRenderer) drawCaption(text string, bounds d2dRect) {
	if r.textFormat == nil {
		return
	}
	if width, err := r.measureCaption(text); err == nil {
		bounds.right = bounds.left + width + 16
	}
	r.target.call(17, uintptr(unsafe.Pointer(&bounds)), uintptr(unsafe.Pointer(r.captionBrush)))
	r.drawOverlayText(text, bounds)
}

func (r *gpuRenderer) measureCaption(text string) (float32, error) {
	if width, ok := r.captionWidths[text]; ok {
		return width, nil
	}
	characters := syscall.StringToUTF16(text)
	var layout *d2dObject
	if err := d2dError("Measure caption", r.textFactory.call(18, uintptr(unsafe.Pointer(&characters[0])), uintptr(len(characters)-1), uintptr(unsafe.Pointer(r.textFormat)), uintptr(math.Float32bits(1000000)), uintptr(math.Float32bits(26)), uintptr(unsafe.Pointer(&layout)))); err != nil {
		return 0, err
	}
	defer release(&layout)
	var metrics struct {
		left, top, width, widthWithWhitespace, height, layoutWidth, layoutHeight float32
		bidiDepth, lineCount                                                     uint32
	}
	if err := d2dError("Get caption metrics", layout.call(60, uintptr(unsafe.Pointer(&metrics)))); err != nil {
		return 0, err
	}
	width := float32(math.Ceil(float64(metrics.widthWithWhitespace)))
	if r.captionWidths == nil {
		r.captionWidths = make(map[string]float32)
	}
	r.captionWidths[text] = width
	return width, nil
}

func (r *gpuRenderer) drawOverlayText(text string, bounds d2dRect) {
	bounds.left += 8
	bounds.right -= 8
	r.drawTextWithoutPadding(text, bounds)
}

func (r *gpuRenderer) drawTextWithoutPadding(text string, bounds d2dRect) {
	characters := syscall.StringToUTF16(text)
	r.target.call(27, uintptr(unsafe.Pointer(&characters[0])), uintptr(len(characters)-1), uintptr(unsafe.Pointer(r.textFormat)), uintptr(unsafe.Pointer(&bounds)), uintptr(unsafe.Pointer(r.textBrush)), 2, 0)
}

func (r *gpuRenderer) drawImageCaptions(foreground bool) {
	for _, caption := range r.captions {
		if caption.foreground == foreground {
			r.drawCaption(caption.text, caption.bounds)
		}
	}
}

func (r *gpuRenderer) prepareCaption() error {
	if r.textFactory == nil {
		proc := windows.NewLazySystemDLL("dwrite.dll").NewProc("DWriteCreateFactory")
		if err := proc.Find(); err != nil {
			return err
		}
		iid := windows.GUID{Data1: 0xb859ee5a, Data2: 0xd838, Data3: 0x4b5b, Data4: [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
		hr, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&r.textFactory)))
		if err := d2dError("Create DirectWrite factory", hr); err != nil {
			return err
		}
	}
	if r.textFormat == nil {
		font, locale := syscall.StringToUTF16Ptr("Segoe UI"), syscall.StringToUTF16Ptr("en-us")
		hr := r.textFactory.call(15, uintptr(unsafe.Pointer(font)), 0, 400, 0, 5, uintptr(math.Float32bits(13)), uintptr(unsafe.Pointer(locale)), uintptr(unsafe.Pointer(&r.textFormat)))
		if err := d2dError("Create caption format", hr); err != nil {
			return err
		}
		r.textFormat.call(3, 0) // Leading / left aligned.
		r.textFormat.call(4, 2) // Vertically centred.
		r.textFormat.call(5, 1) // No wrapping.
	}
	for _, item := range []struct {
		target **d2dObject
		colour d2dColour
	}{
		{&r.captionBrush, d2dColour{r: 0.04, g: 0.04, b: 0.04, a: 0.88}},
		{&r.textBrush, d2dColour{r: 1, g: 1, b: 1, a: 1}},
	} {
		if *item.target == nil {
			if err := d2dError("Create caption brush", r.target.call(8, uintptr(unsafe.Pointer(&item.colour)), 0, uintptr(unsafe.Pointer(item.target)))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *gpuRenderer) drawWorkspaceOverlay(width, height int) {
	if r.showBoundary && r.boundaryX >= 0 && r.boundaryX < float32(width) {
		brush := r.boundsBrush
		if r.boundaryUnset {
			brush = r.boundaryGrayBrush
		}
		line := d2dRect{left: r.boundaryX, top: 0, right: r.boundaryX + 1, bottom: float32(height)}
		r.target.call(17, uintptr(unsafe.Pointer(&line)), uintptr(unsafe.Pointer(brush)))
		handle := d2dRect{left: r.boundaryX - 4, top: 8, right: r.boundaryX + 5, bottom: 26}
		r.target.call(17, uintptr(unsafe.Pointer(&handle)), uintptr(unsafe.Pointer(brush)))
	}
	r.drawImageCaptions(false)
	if r.caption != "" && r.textFormat != nil {
		r.drawCaption(r.caption, r.captionRect)
	}
	r.drawImageCaptions(true)
	if len(r.shortcutLines) > 0 && r.textFormat != nil {
		r.textFormat.call(3, 0) // Left aligned above the workspace origin.
		for index, line := range r.shortcutLines {
			top := r.shortcutY + float32(index*22)
			bounds := d2dRect{left: r.shortcutX, top: top, right: r.shortcutX + 420, bottom: top + 22}
			if bounds.right > 0 && bounds.left < float32(width) && bounds.bottom > 0 && bounds.top < float32(height) {
				r.drawTextWithoutPadding(line, bounds)
			}
		}
		r.textFormat.call(3, 0)
	}
	if r.showBoundary && r.showBoundaryHint && !r.boundaryUnset && r.boundaryX >= 0 && r.boundaryX < float32(width) && r.textFormat != nil {
		r.textFormat.call(3, 1)
		hint := "W - Adjust wrap limit"
		if r.placingWrap {
			hint = "W - Clear wrap limit"
		}
		r.drawOverlayText(hint, d2dRect{left: r.boundaryX - 240, top: 8, right: r.boundaryX - 8, bottom: 30})
		r.textFormat.call(3, 0)
	}
	if r.modeText != "" && r.textFormat != nil {
		r.textFormat.call(3, 0)
		r.drawOverlayText(r.modeText, d2dRect{left: 0, top: float32(max(0, height-28)), right: float32(width), bottom: float32(height)})
	}
	if r.feedbackText != "" && r.feedbackAlpha > 0 && r.textFormat != nil {
		// Anchor to the canvas bottom-right, immediately above the toolbar.
		bottom := height
		bounds := d2dRect{left: 0, top: float32(max(0, bottom-28)), right: float32(width), bottom: float32(bottom)}
		r.textBrush.call(4, uintptr(math.Float32bits(r.feedbackAlpha)))
		r.textFormat.call(3, 1) // Right aligned.
		r.drawOverlayText(r.feedbackText, bounds)
		r.textFormat.call(3, 0)
		r.textBrush.call(4, uintptr(math.Float32bits(1)))
	}
}
