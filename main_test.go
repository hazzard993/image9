//go:build windows

package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInterpolationModes(t *testing.T) {
	// A non-zero origin also checks that rendering aligns arbitrary image bounds.
	src := image.NewRGBA(image.Rect(10, 20, 12, 21))
	src.SetRGBA(10, 20, color.RGBA{A: 255})
	src.SetRGBA(11, 20, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	key := renderKey{width: 8, height: 4, scale: 4}
	nearest := renderImage(src, key)
	key.linear = true
	linear := renderImage(src, key)
	for x := 0; x < 8; x++ {
		p := nearest.RGBAAt(x, 2)
		if p.R != 0 && p.R != 255 {
			t.Fatalf("nearest neighbor blended pixel %d: %v", x, p)
		}
	}
	p := linear.RGBAAt(3, 2)
	if p.R == 0 || p.R == 255 || p.A != 255 {
		t.Fatalf("bilinear did not blend boundary: %v", p)
	}
	if nearest.RGBAAt(0, 2).R != 0 || nearest.RGBAAt(7, 2).R != 255 {
		t.Fatal("image bounds were not aligned")
	}
}

func TestFolderImages(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.JPG", "B.webp", "a.png", "notes.txt", "noextension"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.jpg"), 0700); err != nil {
		t.Fatal(err)
	}
	paths, err := folderImages(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.png"), filepath.Join(dir, "B.webp"), filepath.Join(dir, "z.JPG")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, want %v", paths, want)
	}
}

func TestFolderImagesMissing(t *testing.T) {
	if _, err := folderImages(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing directory error")
	}
}

func TestPanReusesScaledImage(t *testing.T) {
	view := renderKey{width: 1920, height: 1080, scale: 0.75, x: -100, y: -50}
	key, pos := planImageRender(image.Rect(0, 0, 2560, 1440), view)
	view.x += 37
	view.y -= 21
	next, moved := planImageRender(image.Rect(0, 0, 2560, 1440), view)
	if key != next {
		t.Fatal("panning invalidated the scaled bitmap")
	}
	if moved.Sub(pos) != image.Pt(37, -21) {
		t.Fatal("incorrect bitmap translation")
	}
	if key.width != 1920 || key.height != 1080 {
		t.Fatalf("wrong scaled dimensions: %+v", key)
	}
	view.linear = true
	changed, _ := planImageRender(image.Rect(0, 0, 2560, 1440), view)
	if changed == key {
		t.Fatal("interpolation change must invalidate bitmap")
	}
}

func TestOverscanPanMatchesViewport(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), A: 255})
		}
	}
	for _, linear := range []bool{false, true} {
		view := renderKey{width: 120, height: 90, scale: 128, x: -271, y: -349, linear: linear}
		key, pos := planImageRender(src.Bounds(), view)
		cached := renderImage(src, key)
		want := renderImage(src, view)
		for y := 0; y < view.height; y++ {
			for x := 0; x < view.width; x++ {
				if got := cached.RGBAAt(x-pos.X, y-pos.Y); got != want.RGBAAt(x, y) {
					t.Fatalf("linear=%v pixel %d,%d: got %v want %v", linear, x, y, got, want.RGBAAt(x, y))
				}
			}
		}
		view.x++
		next, _ := planImageRender(src.Bounds(), view)
		if next != key {
			t.Fatal("small drag should reuse overscan bitmap")
		}
		if key.width != view.width+512 || key.height != view.height+512 {
			t.Fatal("extreme zoom cache was not bounded")
		}
	}
}

func BenchmarkDrag1440p(b *testing.B) {
	src := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
	view := renderKey{width: 2560, height: 1440, scale: 1}
	b.Run("PreviousResampleEveryFrame", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			view.x = float64(i % 200)
			_ = renderImage(src, view)
		}
	})
	b.Run("CachedPan", func(b *testing.B) {
		key, _ := planImageRender(src.Bounds(), view)
		_ = renderImage(src, key)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			view.x = float64(i % 200)
			next, _ := planImageRender(src.Bounds(), view)
			if key != next {
				b.Fatal("unexpected resample")
			}
		}
	})
}
