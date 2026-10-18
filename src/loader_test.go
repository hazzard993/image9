//go:build windows

package main

import (
	"errors"
	"image"
	"image/color"
	"path/filepath"
	"testing"
	"time"
)

func TestPremultipliedBGRA(t *testing.T) {
	im := image.NewNRGBA(image.Rect(10, 20, 12, 21))
	im.SetNRGBA(10, 20, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	im.SetNRGBA(11, 20, color.NRGBA{R: 255, A: 0})
	decoded, err := preparePixels(im, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := im.At(10, 20).RGBA()
	want := []byte{byte(b >> 8), byte(g >> 8), byte(r >> 8), byte(a >> 8), 0, 0, 0, 0}
	if string(decoded.pixels) != string(want) {
		t.Fatalf("got %v want %v", decoded.pixels, want)
	}
	if decoded.width != 2 || decoded.height != 1 {
		t.Fatal("incorrect image bounds")
	}
	if _, err := preparePixels(im, func() bool { return true }); !errors.Is(err, errLoadSuperseded) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestLoaderNewestRequestWins(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	notified := make(chan struct{}, 4)
	dir := t.TempDir()
	first, last := filepath.Join(dir, "first.png"), filepath.Join(dir, "last.png")
	l := newImageLoader(func() { notified <- struct{}{} }, func(path string, cancelled func() bool) (*decodedImage, error) {
		if path == first {
			close(started)
			<-release
		}
		if cancelled() {
			return nil, errLoadSuperseded
		}
		if path != last {
			t.Errorf("decoded obsolete queued file: %s", path)
		}
		return &decodedImage{width: 1, height: 1, pixels: []byte{0, 0, 0, 255}}, nil
	})
	defer l.close()
	l.request(first)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	l.request(filepath.Join(dir, "skipped.png"))
	id := l.request(last)
	close(release)
	select {
	case <-notified:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not complete")
	}
	result := <-l.results
	if result.request.id != id || result.request.path != last || result.err != nil {
		t.Fatalf("wrong completion: %+v", result)
	}
	l.close()
	select {
	case <-l.done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestDecodeMissingFile(t *testing.T) {
	if _, err := decodeImage(filepath.Join(t.TempDir(), "missing.png"), func() bool { return false }); err == nil {
		t.Fatal("expected missing file error")
	}
}
