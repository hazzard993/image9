//go:build windows

package main

import (
	"fmt"
	"math"
	"path/filepath"
)

const workspaceGap = 32

type workspaceImage struct {
	orderID             uint64
	path                string
	image               *decodedImage
	x, y, width, height int
	failed              bool
	reload              bool
	skipFocus           bool
	dropBatch           *imageDropBatch
}

type imageDropBatch struct{ remaining, loaded int }

func (v *viewer) finishDroppedImage(index int, loaded bool) {
	batch := v.workspace[index].dropBatch
	if batch == nil {
		return
	}
	v.workspace[index].dropBatch = nil
	batch.remaining--
	if loaded {
		batch.loaded++
	}
	if batch.remaining == 0 {
		v.notifyAction(fmt.Sprintf("Loaded %d files into workspace", batch.loaded))
	}
}

func arrangeWorkspace(items []workspaceImage) (int, int) {
	return arrangeWorkspaceWithin(items, 0)
}

func arrangeWorkspaceWithin(items []workspaceImage, limit int) (int, int) {
	return arrangeWorkspaceWithGap(items, limit, workspaceGap)
}

func arrangeWorkspaceWithGap(items []workspaceImage, limit, gap int) (int, int) {
	x, y, rowHeight, width := 0, 0, 0, 0
	for i := range items {
		item := &items[i]
		if item.image != nil {
			item.width, item.height = item.image.width, item.image.height
		}
		if item.width == 0 {
			item.width, item.height = 256, 256
		}
		if limit > 0 && x > 0 && x+item.width > limit {
			y += rowHeight + gap
			x = 0
			rowHeight = 0
		}
		item.x, item.y = x, y
		x += item.width + gap
		width = max(width, x-gap)
		rowHeight = max(rowHeight, item.height)
	}
	if len(items) == 0 {
		return 0, 0
	}
	if limit > 0 {
		width = max(width, limit)
	}
	return width, y + rowHeight
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
	gap := workspaceGap
	if v.customGap {
		gap = v.imageGap
	}
	width, height := arrangeWorkspaceWithGap(v.workspace, v.wrapWidth, gap)
	// Keep the existing world origin fixed when appending or finishing a load
	// during manual pan/zoom. Fit mode intentionally reframes the entire strip.
	if !v.fit {
		v.panX += float64(width-v.width) * v.scale / 2
		v.panY += float64(height-v.height) * v.scale / 2
	}
	v.width, v.height = width, height
	v.refreshImageList()
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
	v.window.SetTitle("Quick Image Viewer - " + filepath.Base(item.path))
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

func verticalWorkspaceNeighbor(items []workspaceImage, index, direction int) int {
	if index < 0 || index >= len(items) {
		for i, item := range items {
			if item.image != nil {
				return i
			}
		}
		return -1
	}
	current := items[index]
	row := -1
	for _, item := range items {
		if item.image == nil || direction < 0 && item.y >= current.y || direction > 0 && item.y <= current.y {
			continue
		}
		if row == -1 || direction < 0 && item.y > row || direction > 0 && item.y < row {
			row = item.y
		}
	}
	if row == -1 {
		return index
	}
	centre := float64(current.x) + float64(current.width)/2
	best, distance := index, math.Inf(1)
	for i, item := range items {
		if item.image == nil || item.y != row {
			continue
		}
		d := math.Abs(float64(item.x) + float64(item.width)/2 - centre)
		if d < distance {
			best, distance = i, d
		}
	}
	return best
}

func (v *viewer) navigateVertical(direction int) {
	index := verticalWorkspaceNeighbor(v.workspace, v.activeIndex, direction)
	if index < 0 || index == v.activeIndex {
		return
	}
	scale := v.displayScale()
	v.selectImage(index)
	v.fit, v.scale = false, scale
	v.centreActive(scale)
	v.notifyAction("Focused on " + filepath.Base(v.path))
}

func (v *viewer) focusAddedImage(index int) {
	item := v.workspace[index]
	b := v.canvas.ClientBoundsPixels()
	scale := v.displayScale()
	if v.fit {
		scale = 1
	}
	// Preserve manual zoom where possible, but keep the new image fully visible.
	scale = math.Max(0.000001, math.Min(scale, math.Min(float64(max(1, b.Width-32))/float64(item.width), float64(max(1, b.Height-64))/float64(item.height))))
	v.selectImage(index)
	v.fit = false
	v.scale = scale
	v.centreActive(scale)
	v.focusImage()
	v.notifyAction("Focused on " + filepath.Base(item.path))
}

func (v *viewer) frameSelectedOrAll() {
	if v.activeIndex < 0 || v.activeIndex >= len(v.workspace) {
		return
	}
	item := v.workspace[v.activeIndex]
	if item.image == nil {
		return
	}
	b := v.canvas.ClientBoundsPixels()
	scale := math.Max(0.000001, math.Min(1, math.Min(float64(max(1, b.Width-32))/float64(item.width), float64(max(1, b.Height-64))/float64(item.height))))
	panX := (float64(v.width)/2 - float64(item.x) - float64(item.width)/2) * scale
	panY := (float64(v.height)/2 - float64(item.y) - float64(item.height)/2) * scale
	// Compare the actual camera so panning or zooming naturally resets the toggle.
	if math.Abs(v.displayScale()-scale) < scale*0.000001 && math.Abs(v.panX-panX) < 0.5 && math.Abs(v.panY-panY) < 0.5 {
		v.fitImage()
		return
	}
	v.fit = false
	v.scale = scale
	v.panX, v.panY = panX, panY
	v.notifyAction("Framed " + filepath.Base(item.path))
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
	v.wrapWidth = 0
	v.source = nil
	v.path = ""
	v.activeIndex = 0
	v.width, v.height = 0, 0
	v.fitImage()
	v.focusImage()
	v.window.SetTitle("Quick Image Viewer")
	v.notifyAction("Workspace cleared")
}

func (v *viewer) removeWorkspaceImage(index int) {
	if index < 0 || index >= len(v.workspace) {
		return
	}
	if len(v.workspace) == 1 {
		name := filepath.Base(v.workspace[index].path)
		v.clearWorkspace()
		v.notifyAction("Removed " + name)
		return
	}
	v.stopDrag()
	removedName := filepath.Base(v.workspace[index].path)
	v.finishDroppedImage(index, false)
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
	v.notifyAction("Removed " + removedName)
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
	v.addWorkspaceImagesWithFocus(paths, true)
}

func (v *viewer) dropImages(paths []string) {
	v.addWorkspaceImagesWithFocus(paths, false)
}

func (v *viewer) unsetFocusedImage() {
	for i := range v.workspace {
		v.workspace[i].skipFocus = true
	}
	v.activeIndex = -1
	v.source = nil
	v.path = ""
	v.window.SetTitle("Quick Image Viewer")
	v.notifyAction("Image focus cleared")
}

func (v *viewer) addWorkspaceImagesWithFocus(paths []string, focus bool) {
	files, err := normalizeImagePaths(paths)
	if err != nil {
		v.showError(err)
		return
	}
	if len(files) == 0 {
		return
	}
	wasEmpty := len(v.workspace) == 0
	var batch *imageDropBatch
	if !focus {
		batch = &imageDropBatch{remaining: len(files)}
	}
	if !focus && !wasEmpty {
		v.scale = v.displayScale()
		v.fit = false
	}
	for _, path := range files {
		v.nextOrderID++
		v.workspace = append(v.workspace, workspaceImage{orderID: v.nextOrderID, path: path, skipFocus: !focus, dropBatch: batch})
	}
	v.arrangeWorkspace()
	if wasEmpty && focus {
		v.selectImage(0)
		v.fitImage()
	}
	if wasEmpty && !focus {
		v.activeIndex = -1
	}
	v.loadNextWorkspaceImage()
}
