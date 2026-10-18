//go:build windows

package main

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
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

func exerciseSpatialLoading(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	v := &viewer{fit: true, scale: 1}
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
}
