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
