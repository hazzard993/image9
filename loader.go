//go:build windows

package main

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

type decodedImage struct {
	fileSize      int64
	width, height int
	pixels        []byte
}
type loadRequest struct {
	id   uint64
	path string
}
type loadResult struct {
	request loadRequest
	image   *decodedImage
	files   []string
	err     error
}
type imageLoader struct {
	requests chan loadRequest
	results  chan loadResult
	stop     chan struct{}
	done     chan struct{}
	closed   sync.Once
	latest   atomic.Uint64
}

var errLoadSuperseded = errors.New("image load superseded")

func newImageLoader(notify func(), decode func(string, func() bool) (*decodedImage, error)) *imageLoader {
	l := &imageLoader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(l.done)
		for {
			select {
			case <-l.stop:
				return
			case request := <-l.requests:
				cancelled := func() bool {
					select {
					case <-l.stop:
						return true
					default:
						return l.latest.Load() != request.id
					}
				}
				if cancelled() {
					continue
				}
				files, err := folderImages(filepath.Dir(request.path))
				if err != nil {
					files = []string{request.path}
				}
				im, err := decode(request.path, cancelled)
				if cancelled() {
					continue
				}
				result := loadResult{request: request, image: im, files: files, err: err}
				select {
				case <-l.results:
				default:
				} // Retain only the newest completion.
				select {
				case <-l.stop:
					return
				case l.results <- result:
				}
				notify()
			}
		}
	}()
	return l
}
func (l *imageLoader) request(path string) uint64 {
	id := l.latest.Add(1)
	select {
	case <-l.requests:
	default:
	}
	l.requests <- loadRequest{id: id, path: path}
	return id
}
func (l *imageLoader) close() { l.closed.Do(func() { close(l.stop) }) }

type cancellableReader struct {
	io.Reader
	cancelled func() bool
}

func (r cancellableReader) Read(p []byte) (int, error) {
	if r.cancelled() {
		return 0, errLoadSuperseded
	}
	return r.Reader.Read(p)
}

func decodeImage(path string, cancelled func() bool) (*decodedImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	im, _, err := image.Decode(cancellableReader{Reader: f, cancelled: cancelled})
	if err != nil {
		return nil, fmt.Errorf("cannot decode %s: %w", filepath.Base(path), err)
	}
	decoded, err := preparePixels(im, cancelled)
	if err != nil {
		return nil, err
	}
	decoded.fileSize = info.Size()
	return decoded, nil
}

// Go RGBA is premultiplied; Direct2D expects premultiplied BGRA. Conversion is
// performed once in the worker, never during a pan/zoom repaint.
func preparePixels(im image.Image, cancelled func() bool) (*decodedImage, error) {
	bounds := im.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		if cancelled() {
			return nil, errLoadSuperseded
		}
		draw.Draw(rgba, image.Rect(0, y, bounds.Dx(), y+1), im, image.Pt(bounds.Min.X, bounds.Min.Y+y), draw.Src)
		row := rgba.Pix[y*rgba.Stride : (y+1)*rgba.Stride]
		for x := 0; x < len(row); x += 4 {
			row[x], row[x+2] = row[x+2], row[x]
		}
	}
	return &decodedImage{width: bounds.Dx(), height: bounds.Dy(), pixels: rgba.Pix}, nil
}

const imageReadyMessage = win.WM_APP + 77

var loadersByWindow sync.Map
var loadPumpCallback uintptr

func installLoadPump(v *viewer) {
	if loadPumpCallback == 0 {
		loadPumpCallback = syscall.NewCallback(loadPumpProc)
	}
	hwnd := v.window.Handle() // Capture an immutable native handle for the worker.
	loadersByWindow.Store(hwnd, v)
	setSubclass.Call(uintptr(hwnd), loadPumpCallback, 2, 0)
	v.loader = newImageLoader(func() { win.PostMessage(hwnd, imageReadyMessage, 0, 0) }, decodeImage)
}

func loadPumpProc(hwnd win.HWND, message uint32, wParam uintptr, lParam unsafe.Pointer, id, data uintptr) uintptr {
	if value, ok := loadersByWindow.Load(hwnd); ok {
		v := value.(*viewer)
		switch message {
		case win.WM_ACTIVATE:
			if wParam&0xffff == 0 {
				v.stopDrag()
			}
			v.setBoundsHighlight(wParam&0xffff != 0 && walk.ModifiersDown()&walk.ModControl != 0)
		case imageReadyMessage:
			v.finishLoad()
			return 0
		case win.WM_NCDESTROY:
			if v.loader != nil {
				v.loader.close()
			}
			if v.renderer != nil {
				v.renderer.close()
				v.renderer = nil
			}
			loadersByWindow.Delete(hwnd)
			removeSubclass.Call(uintptr(hwnd), loadPumpCallback, id)
		}
	}
	result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
	return result
}

func (v *viewer) finishLoad() {
	for {
		select {
		case result := <-v.loader.results:
			if result.request.id != v.loadID {
				continue
			}
			v.loading = false
			if result.err != nil {
				v.pendingPath = v.path
				v.window.SetTitle(filepath.Base(v.path) + " — Quick Image Viewer")
				v.canvas.Invalidate()
				v.showError(result.err)
				continue
			}
			v.source = result.image
			v.path, v.pendingPath = result.request.path, result.request.path
			v.files = result.files
			v.width, v.height = result.image.width, result.image.height
			v.fitImage()
			v.window.SetTitle(filepath.Base(v.path) + " — Quick Image Viewer")
		default:
			return
		}
	}
}
