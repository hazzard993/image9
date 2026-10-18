//go:build windows

package main

import (
	"fmt"
	"math"
	"path/filepath"
)

const workspaceGap = 32

type workspaceImage struct {
	path                string
	image               *decodedImage
	x, y, width, height int
	failed              bool
	reload              bool
}

func arrangeWorkspace(items []workspaceImage) (int, int) {
	x, height := 0, 0
	for i := range items {
		item := &items[i]
		if item.image != nil {
			item.width, item.height = item.image.width, item.image.height
		}
		if item.width == 0 {
			item.width, item.height = 256, 256
		}
		item.x, item.y = x, 0
		x += item.width + workspaceGap
		height = max(height, item.height)
	}
	if len(items) > 0 {
		x -= workspaceGap
	}
	return x, height
}

func workspaceHit(items []workspaceImage, x, y float64) (int, int, int) {
	for i, item := range items {
		if item.image == nil {
			continue
		}
		px, py := int(math.Floor(x-float64(item.x))), int(math.Floor(y-float64(item.y)))
		if px >= 0 && py >= 0 && px < item.width && py < item.height {
			return i, px, py
		}
	}
	return -1, 0, 0
}

func (v *viewer) arrangeWorkspace() {
	width, height := arrangeWorkspace(v.workspace)
	// Keep the existing world origin fixed when appending or finishing a load
	// during manual pan/zoom. Fit mode intentionally reframes the entire strip.
	if !v.fit {
		v.panX += float64(width-v.width) * v.scale / 2
		v.panY += float64(height-v.height) * v.scale / 2
	}
	v.width, v.height = width, height
	v.canvas.Invalidate()
}

func (v *viewer) hitImage(x, y int) (int, int, int) {
	b := v.canvas.ClientBoundsPixels()
	s := v.displayScale()
	left := math.Round((float64(b.Width)-float64(v.width)*s)/2 + v.panX)
	top := math.Round((float64(b.Height)-float64(v.height)*s)/2 + v.panY)
	return workspaceHit(v.workspace, (float64(x)-left)/s, (float64(y)-top)/s)
}

func (v *viewer) selectImage(index int) {
	if index < 0 || index >= len(v.workspace) {
		return
	}
	v.activeIndex = index
	item := v.workspace[index]
	v.source = item.image
	v.path = item.path
	v.window.SetTitle(filepath.Base(item.path) + " — Quick Image Viewer")
	v.canvas.Invalidate()
}

func (v *viewer) centreActive(scale float64) {
	if v.activeIndex < 0 || v.activeIndex >= len(v.workspace) {
		return
	}
	item := v.workspace[v.activeIndex]
	v.panX = (float64(v.width)/2 - float64(item.x) - float64(item.width)/2) * scale
	v.panY = (float64(v.height)/2 - float64(item.y) - float64(item.height)/2) * scale
}

func (v *viewer) loadNextWorkspaceImage() {
	if v.loading {
		return
	}
	for i, item := range v.workspace {
		if (item.image == nil || item.reload) && !item.failed {
			v.loading = true
			v.loadingIndex = i
			v.loadID = v.loader.request(item.path, v.sourceColourSpace)
			v.canvas.Invalidate()
			return
		}
	}
	v.canvas.Invalidate()
}

func (v *viewer) reloadWorkspace() {
	v.loadID = v.loader.cancel()
	v.loading = false
	for i := range v.workspace {
		v.workspace[i].reload = true
		v.workspace[i].failed = false
	}
	v.loadNextWorkspaceImage()
}

func (v *viewer) clearWorkspace() {
	v.loadID = v.loader.cancel()
	v.loading = false
	v.workspace = nil
	v.source = nil
	v.path = ""
	v.activeIndex = 0
	v.width, v.height = 0, 0
	v.fitImage()
	v.focusImage()
	v.window.SetTitle("Quick Image Viewer")
}

func (v *viewer) removeWorkspaceImage(index int) {
	if index < 0 || index >= len(v.workspace) {
		return
	}
	if len(v.workspace) == 1 {
		v.clearWorkspace()
		return
	}
	v.stopDrag()
	if v.loading {
		if v.loadingIndex == index {
			v.loadID = v.loader.cancel()
			v.loading = false
		} else if v.loadingIndex > index {
			v.loadingIndex--
		}
	}
	if v.activeIndex > index {
		v.activeIndex--
	}
	copy(v.workspace[index:], v.workspace[index+1:])
	v.workspace[len(v.workspace)-1] = workspaceImage{}
	v.workspace = v.workspace[:len(v.workspace)-1]
	v.activeIndex = min(v.activeIndex, len(v.workspace)-1)
	v.arrangeWorkspace()
	v.selectImage(v.activeIndex)
	v.loadNextWorkspaceImage()
}

func (v *viewer) workspaceStatus(scale float64) string {
	zoom := fmt.Sprintf("%.0f%%", scale*100)
	if scale < 0.01 {
		zoom = fmt.Sprintf("%.2f%%", scale*100)
	}
	if v.loading {
		loaded := 0
		for _, item := range v.workspace {
			if item.image != nil && !item.reload || item.failed {
				loaded++
			}
		}
		return fmt.Sprintf("Loading %d/%d  •  %s", loaded+1, len(v.workspace), zoom)
	}
	if v.source != nil {
		return fmt.Sprintf("%d × %d  •  %s  •  %s", v.source.width, v.source.height, zoom, formatFileSize(v.source.fileSize))
	}
	return "Open or drop images • Ctrl+O"
}

func (v *viewer) addWorkspaceImages(paths []string) {
	files, err := normalizeImagePaths(paths)
	if err != nil {
		v.showError(err)
		return
	}
	if len(files) == 0 {
		return
	}
	wasEmpty := len(v.workspace) == 0
	for _, path := range files {
		v.workspace = append(v.workspace, workspaceImage{path: path})
	}
	v.arrangeWorkspace()
	if wasEmpty {
		v.selectImage(0)
		v.fitImage()
	}
	v.loadNextWorkspaceImage()
}
