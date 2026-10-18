//go:build windows

package main

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestImageColourSampling(t *testing.T) {
	im := &decodedImage{width: 2, height: 1, pixels: []byte{0x56, 0x34, 0x12, 255, 0, 0, 128, 128}}
	for _, tc := range []struct {
		x, y int
		want string
		ok   bool
	}{
		{0, 0, "#123456", true}, {1, 0, "#FF000080", true}, {-1, 0, "", false}, {2, 0, "", false}, {0, 1, "", false},
	} {
		got, ok := imageColourAt(im, tc.x, tc.y)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("sample %d,%d: %q,%v", tc.x, tc.y, got, ok)
		}
	}
}

func TestMultipleImagePaths(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "First.png"), filepath.Join(dir, "Second.jpg")
	paths, err := normalizeImagePaths([]string{first, second, filepath.Join(dir, "FIRST.PNG")})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{first, second}) {
		t.Fatalf("order/deduplication: %v", paths)
	}
	if paths, err := normalizeImagePaths(nil); err != nil || len(paths) != 0 {
		t.Fatal("empty selection should be empty")
	}
}

func TestZoomKeepsCursorImagePoint(t *testing.T) {
	for _, test := range []struct{ pan, anchor, centre, scale, ratio float64 }{
		{0, 100, 500, 0.5, 1.25}, {135, 720, 500, 2, 0.8}, {-80, 300, 720, 1, 4}, {50, 500, 500, 1, 1},
	} {
		const imageSize = 2560
		before := (test.anchor - (test.centre - imageSize*test.scale/2 + test.pan)) / test.scale
		pan := zoomPan(test.pan, test.anchor, test.centre, test.ratio)
		scale := test.scale * test.ratio
		after := (test.anchor - (test.centre - imageSize*scale/2 + pan)) / scale
		if math.Abs(before-after) > 1e-9 {
			t.Fatalf("cursor image point moved: %v -> %v", before, after)
		}
	}
}
