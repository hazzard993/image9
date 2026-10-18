//go:build windows

package main

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func TestWorkspaceOrderAndHitTesting(t *testing.T) {
	items := []workspaceImage{
		{path: "first", image: &decodedImage{width: 100, height: 80}},
		{path: "second", image: &decodedImage{width: 50, height: 200}},
	}
	w, h := arrangeWorkspace(items)
	if w != 182 || h != 200 || items[0].x != 0 || items[1].x != 132 {
		t.Fatalf("layout: %v %v %+v", w, h, items)
	}
	for _, tc := range []struct {
		x, y          float64
		index, px, py int
	}{{10, 20, 0, 10, 20}, {135, 99, 1, 3, 99}, {110, 10, -1, 0, 0}, {-0.1, 0, -1, 0, 0}, {10, 90, -1, 0, 0}} {
		i, x, y := workspaceHit(items, tc.x, tc.y)
		if i != tc.index || x != tc.px || y != tc.py {
			t.Fatalf("hit %v: %d,%d,%d", tc, i, x, y)
		}
	}
	items = append(items, workspaceImage{image: &decodedImage{width: 75, height: 40}})
	arrangeWorkspace(items)
	if items[0].x != 0 || items[1].x != 132 || items[2].x != 214 {
		t.Fatal("appending moved existing images")
	}
}

func TestWorkspaceWrapping(t *testing.T) {
	items := []workspaceImage{{image: &decodedImage{width: 100, height: 80}}, {image: &decodedImage{width: 50, height: 200}}, {image: &decodedImage{width: 75, height: 40}}}
	w, h := arrangeWorkspaceWithin(items, 200)
	if w != 200 || h != 272 || items[0].x != 0 || items[1].x != 132 || items[2].x != 0 || items[2].y != 232 {
		t.Fatalf("wrapped layout %d,%d: %+v", w, h, items)
	}
	if i, x, y := workspaceHit(items, 10, 240); i != 2 || x != 10 || y != 8 {
		t.Fatalf("wrapped image hit: %d,%d,%d", i, x, y)
	}
	if i, _, _ := workspaceHit(items, 10, 220); i != -1 {
		t.Fatal("row gap incorrectly hit an image")
	}
	// Oversized images occupy a row of their own without scaling or overlap.
	w, h = arrangeWorkspaceWithin(items, 64)
	if w != 100 || h != 384 || items[0].y != 0 || items[1].y != 112 || items[2].y != 344 {
		t.Fatalf("oversized layout %d,%d: %+v", w, h, items)
	}
	arrangeWorkspaceWithin(items, 0)
	if items[2].x != 214 || items[2].y != 0 {
		t.Fatal("reset did not restore the single row")
	}
}

func TestVerticalWorkspaceNavigation(t *testing.T) {
	items := []workspaceImage{
		{x: 0, y: 0, width: 64, image: &decodedImage{}},
		{x: 72, y: 0, width: 64, image: &decodedImage{}},
		{x: 0, y: 72, width: 40, image: &decodedImage{}},
		{x: 48, y: 72, width: 100, image: &decodedImage{}},
		{x: 0, y: 180, width: 64, image: &decodedImage{}},
	}
	for _, test := range []struct{ from, direction, want int }{
		{0, 1, 2}, {1, 1, 3}, {3, -1, 1}, {3, 1, 4}, {4, 1, 4}, {0, -1, 0}, {-1, 1, 0},
	} {
		if got := verticalWorkspaceNeighbor(items, test.from, test.direction); got != test.want {
			t.Fatalf("from %d direction %d: got %d, want %d", test.from, test.direction, got, test.want)
		}
	}
	items[3].image = nil
	if got := verticalWorkspaceNeighbor(items, 1, 1); got != 2 {
		t.Fatalf("navigation selected an unloaded image: %d", got)
	}
	if got := verticalWorkspaceNeighbor(nil, -1, 1); got != -1 {
		t.Fatal("empty workspace gained a selection")
	}
}

func exerciseSpatialLoading(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	v := &viewer{fit: true, scale: 1}
	defer v.showKeyboardCursor()
	if err := (MainWindow{AssignTo: &v.window, Visible: false, Size: Size{Width: 640, Height: 480}, Layout: VBox{}, Children: []Widget{
		CustomWidget{AssignTo: &v.canvas, PaintMode: PaintNoErase, PaintPixels: func(*walk.Canvas, walk.Rectangle) error { return nil }},
	}}).Create(); err != nil {
		t.Fatal(err)
	}
	defer v.window.Dispose()
	started, release := make(chan struct{}), make(chan struct{})
	notified := make(chan struct{}, 8)
	count := 0
	v.loader = newImageLoader(func() { notified <- struct{}{} }, func(path string, cancelled func() bool) (*decodedImage, error) {
		count++
		if count == 1 {
			close(started)
			<-release
		}
		return &decodedImage{width: 100 * count, height: 80, pixels: make([]byte, 100*count*80*4)}, nil
	})
	defer v.loader.close()
	first, second := filepath.Join(t.TempDir(), "first.png"), filepath.Join(t.TempDir(), "second.png")
	v.addWorkspaceImages([]string{first})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first load did not start")
	}
	// A new drop while decoding must queue behind the first, not cancel it.
	v.addWorkspaceImages([]string{second})
	close(release)
	for v.loading {
		select {
		case <-notified:
			v.finishLoad()
		case <-time.After(5 * time.Second):
			t.Fatal("workspace load timed out")
		}
	}
	if len(v.workspace) != 2 || v.workspace[0].image == nil || v.workspace[1].image == nil {
		t.Fatal("not all dropped images were retained")
	}
	if v.workspace[1].x != 132 || v.workspace[0].path != first || v.workspace[1].path != second {
		t.Fatal("opening order changed")
	}
	if v.activeIndex != 1 || v.path != second || v.source != v.workspace[1].image || v.fit {
		t.Fatal("newly added image was not selected")
	}
	v.loading, v.loadingIndex = true, 0
	if v.moveWorkspaceImage(1, -1) != 0 || v.workspace[0].path != second || v.activeIndex != 0 || v.loadingIndex != 1 || v.source != v.workspace[0].image {
		t.Fatal("reordering lost the focused image or in-flight load")
	}
	if v.moveWorkspaceImage(0, 1) != 1 || v.workspace[0].path != first || v.activeIndex != 1 || v.loadingIndex != 0 {
		t.Fatal("moving back did not restore image order and indices")
	}
	v.loading = false
	originalOrder := v.snapshotImageOrder()
	v.workspace[0].image.fileSize = 200
	v.workspace[1].image.fileSize = 100
	v.loading, v.loadingIndex = true, 0
	v.sortImagesByFileSize()
	if v.workspace[0].path != second || v.activeIndex != 0 || v.loadingIndex != 1 {
		t.Fatal("file-size sorting lost selection or in-flight loading")
	}
	v.sortImagesAlphabetically()
	if v.workspace[0].path != first || v.workspace[1].path != second || v.activeIndex != 1 || v.loadingIndex != 0 {
		t.Fatal("alphabetical sorting lost order, selection or in-flight loading")
	}
	v.sortImagesByFileSize()
	v.nextOrderID++
	v.workspace = append(v.workspace, workspaceImage{orderID: v.nextOrderID, path: "added-while-open.png"})
	v.restoreImageOrder(originalOrder)
	if v.workspace[0].path != first || v.workspace[1].path != second || v.workspace[2].path != "added-while-open.png" || v.activeIndex != 1 || v.loadingIndex != 0 || v.workspace[0].image.fileSize != 200 {
		t.Fatal("cancel did not restore order while preserving images and loading state")
	}
	v.workspace = v.workspace[:2]
	v.loading = false
	v.arrangeWorkspace()
	item := v.workspace[1]
	if math.Abs(v.panX-(float64(v.width)/2-float64(item.x)-float64(item.width)/2)*v.scale) > 0.001 ||
		math.Abs(v.panY-(float64(v.height)/2-float64(item.y)-float64(item.height)/2)*v.scale) > 0.001 {
		t.Fatal("newly added image was not centred")
	}
	v.scale = 0.25
	v.panX = 100
	v.frameSelectedOrAll()
	frameBounds := v.canvas.ClientBoundsPixels()
	frameScale := math.Max(0.000001, math.Min(1, math.Min(float64(max(1, frameBounds.Width-32))/float64(item.width), float64(max(1, frameBounds.Height-64))/float64(item.height))))
	if v.fit || v.scale != frameScale || math.Abs(v.panX-(float64(v.width)/2-float64(item.x)-float64(item.width)/2)*frameScale) > 0.001 {
		t.Fatal("R did not frame the selected image")
	}
	v.frameSelectedOrAll()
	if !v.fit || v.panX != 0 || v.panY != 0 {
		t.Fatal("R did not frame all images on the second press")
	}
	v.frameSelectedOrAll()
	if v.fit {
		t.Fatal("R did not return to the selected image")
	}
	v.panX += 20
	v.frameSelectedOrAll()
	if v.fit {
		t.Fatal("R framed all images after manual panning")
	}
	v.fit = false
	v.scale = 2
	v.panX = 10
	v.panY = 20
	beforeX, beforeY := float64(640-v.width*2)/2+v.panX, float64(480-v.height*2)/2+v.panY
	v.workspace = append(v.workspace, workspaceImage{image: &decodedImage{width: 50, height: 500}})
	v.arrangeWorkspace()
	afterX, afterY := float64(640-v.width*2)/2+v.panX, float64(480-v.height*2)/2+v.panY
	if beforeX != afterX || beforeY != afterY {
		t.Fatal("appending moved the existing view")
	}
	v.installPointerTracking()
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.Key1), 0)
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.Key6), 0)
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyReturn), 0)
	if v.gapInput != nil || !v.customGap || v.imageGap != 16 || v.workspace[1].x != 116 {
		t.Fatal("gap input did not apply 16 pixels")
	}
	previousWrap := v.wrapWidth
	v.beginGapInput()
	for _, key := range []walk.Key{walk.Key2, walk.Key4, walk.KeyReturn} {
		win.PostMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(key), 0)
		var message win.MSG
		if !win.PeekMessage(&message, v.canvas.Handle(), win.WM_KEYDOWN, win.WM_KEYDOWN, win.PM_REMOVE) || message.Message != win.WM_NULL {
			t.Fatal("gap keyboard capture did not consume queued input before shortcuts")
		}
	}
	if v.gapInput != nil || v.imageGap != 24 || v.gapKeyboardHook != 0 {
		t.Fatal("captured numeric entry did not apply or release the keyboard")
	}
	v.beginGapInput()
	*v.gapInput = "16"
	v.endGapInput(true)
	v.beginGapInput()
	*v.gapInput = "not a number"
	v.endGapInput(true)
	if v.gapInput != nil || v.imageGap != 16 || v.wrapWidth != previousWrap {
		t.Fatal("invalid gap submission changed the previous gap or wrap")
	}
	v.beginGapInput()
	*v.gapInput = "999999999999999999999999999"
	v.endGapInput(true)
	if v.gapInput != nil || v.imageGap != 16 || v.wrapWidth != previousWrap {
		t.Fatal("overflowing gap submission changed the previous gap or wrap")
	}
	// TranslateMessage has already queued this character when Enter finishes
	// gap mode. Dialog navigation must still allow it through to the canvas.
	if code := win.SendMessage(v.canvas.Handle(), win.WM_GETDLGCODE, uintptr(walk.KeyReturn), 0); code&win.DLGC_WANTCHARS == 0 || code&win.DLGC_WANTALLKEYS == 0 {
		t.Fatal("Enter reverted to dialog navigation after submitting the gap")
	}
	if result := win.SendMessage(v.canvas.Handle(), win.WM_CHAR, '\r', 0); result != 0 {
		t.Fatal("queued Enter character was not consumed")
	}
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.Key9), 0)
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyEscape), 0)
	if v.gapInput != nil || v.imageGap != 16 {
		t.Fatal("Escape changed the gap")
	}
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.Key2), 0)
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyQ), 0)
	if v.gapInput != nil || v.imageGap != 16 {
		t.Fatal("invalid input changed the gap")
	}
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.Key9), 0)
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyG), 0)
	if v.gapInput != nil || v.imageGap != 0 {
		t.Fatal("second G did not apply zero and exit gap mode")
	}
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyG), 0)
	if v.gapInput != nil || v.imageGap != workspaceGap {
		t.Fatal("second G did not apply the default and exit when gap was zero")
	}
	v.beginGapInput()
	win.SendMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyReturn), 0)
	if v.gapInput != nil || v.imageGap != 0 || v.workspace[1].x != 100 {
		t.Fatal("empty gap input did not apply zero")
	}
	v.selectImage(1)
	v.removeWorkspaceImage(0)
	if len(v.workspace) != 2 || v.activeIndex != 0 || v.workspace[0].path != second || v.path != second {
		t.Fatal("removing an earlier image lost the selected image")
	}
	v.removeWorkspaceImage(1)
	if len(v.workspace) != 1 || v.path != second {
		t.Fatal("removing the last image lost the preceding selection")
	}
	v.removeWorkspaceImage(0)
	if len(v.workspace) != 0 || v.source != nil || v.loading {
		t.Fatal("removing the final image did not clear the workspace")
	}
	v.dropImages([]string{first})
	for v.loading {
		select {
		case <-notified:
			v.finishLoad()
		case <-time.After(5 * time.Second):
			t.Fatal("dropped image load timed out")
		}
	}
	if v.activeIndex != -1 || v.source != nil || len(v.workspace) != 1 || v.workspace[0].image == nil {
		t.Fatal("dropped image unexpectedly gained focus")
	}
	if v.feedbackText != "Loaded 1 files into workspace" {
		t.Fatal("completed drop did not report the loaded file count")
	}
	v.selectImage(0)
	v.unsetFocusedImage()
	if v.activeIndex != -1 || v.source != nil || v.path != "" || len(v.workspace) != 1 {
		t.Fatal("unsetting focus changed the workspace or left a selection")
	}
	v.copyFocusedPath()
	if v.feedbackText != "No focus target, nothing copied" || len(v.workspace) != 1 {
		t.Fatal("copy without focus did not report the expected message")
	}
	v.toggleWrapMode()
	if !v.placingWrap || v.wrapWidth != 0 {
		t.Fatal("W did not enter wrap placement without changing layout")
	}
	v.placeWrapAt(200)
	if v.placingWrap || v.wrapWidth < 64 || v.wrapWidth%8 != 0 {
		t.Fatal("click did not place a snapped wrap limit")
	}
	v.toggleWrapMode()
	if !v.placingWrap || v.wrapWidth == 0 {
		t.Fatal("W did not enter placement while preserving the existing limit")
	}
	v.toggleWrapMode()
	if v.wrapWidth != 0 || v.placingWrap {
		t.Fatal("W did not reset the existing wrap limit")
	}
	v.toggleWrapMode()
	v.focusImage()
	if v.placingWrap || v.wrapWidth != 0 {
		t.Fatal("cancelling wrap placement changed the limit")
	}
	v.enterCtrlView()
	v.toggleWrapMode()
	for _, key := range []walk.Key{walk.Key1, walk.Key3, walk.Key3, walk.KeyReturn} {
		win.PostMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(key), 0)
		var message win.MSG
		if !win.PeekMessage(&message, v.canvas.Handle(), win.WM_KEYDOWN, win.WM_KEYDOWN, win.PM_REMOVE) || message.Message != win.WM_NULL {
			t.Fatal("wrap input was not captured before shortcuts")
		}
	}
	if v.placingWrap || v.wrapWidth != 133 || v.gapKeyboardHook != 0 {
		t.Fatal("typed wrap width was not applied exactly or capture was not released")
	}
	v.toggleWrapMode()
	v.handleWrapKey(walk.Key9)
	v.handleWrapKey(walk.KeyQ)
	if v.placingWrap || v.wrapWidth != 133 || v.gapKeyboardHook != 0 {
		t.Fatal("invalid input changed the previous wrap limit")
	}
	v.setBoundsHighlight(false)
	if !v.ctrlViewPinned || !v.highlightBounds {
		t.Fatal("I mode did not remain active after Ctrl release")
	}
	v.escapeMode()
	if v.ctrlViewPinned || v.highlightBounds != (walk.ModifiersDown()&walk.ModControl != 0) {
		t.Fatal("Escape did not release the persistent Ctrl view")
	}
	v.hideKeyboardCursor()
	v.hideKeyboardCursor()
	if !v.cursorHidden {
		t.Fatal("keyboard use did not hide the cursor")
	}
	v.revealCursorOnMovement(v.cursorHiddenAt)
	if !v.cursorHidden {
		t.Fatal("stationary mouse revealed the cursor")
	}
	moved := v.cursorHiddenAt
	moved.X++
	v.revealCursorOnMovement(moved)
	if v.cursorHidden {
		t.Fatal("mouse movement did not restore the cursor")
	}
	if err := v.installKeyboardCursor(); err != nil {
		t.Fatal(err)
	}
	defer v.closeKeyboardCursor()
	win.PostMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyControl), 0)
	var controlMessage win.MSG
	if !win.PeekMessage(&controlMessage, v.canvas.Handle(), win.WM_KEYDOWN, win.WM_KEYDOWN, win.PM_REMOVE) || v.cursorHidden {
		t.Fatal("Ctrl alone hid the cursor")
	}
	v.beginGapInput()
	win.PostMessage(v.canvas.Handle(), win.WM_KEYDOWN, uintptr(walk.KeyControl), 0)
	if !win.PeekMessage(&controlMessage, v.canvas.Handle(), win.WM_KEYDOWN, win.WM_KEYDOWN, win.PM_REMOVE) || v.cursorHidden {
		t.Fatal("Ctrl alone hid the cursor during captured input")
	}
	v.endGapInput(false)
}
