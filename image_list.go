//go:build windows

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func (v *viewer) refreshImageList() {
	if v.imageOrderList == nil {
		return
	}
	selected := v.imageOrderList.CurrentIndex()
	names := make([]string, len(v.workspace))
	for i, item := range v.workspace {
		names[i] = filepath.Base(item.path)
	}
	v.imageOrderList.SetModel(names)
	v.imageOrderList.SetCurrentIndex(min(selected, len(names)-1))
}

func (v *viewer) moveWorkspaceImage(index, delta int) int {
	target := index + delta
	if index < 0 || index >= len(v.workspace) || target < 0 || target >= len(v.workspace) {
		return index
	}
	v.workspace[index], v.workspace[target] = v.workspace[target], v.workspace[index]
	remap := func(current int) int {
		if current == index {
			return target
		}
		if current == target {
			return index
		}
		return current
	}
	v.activeIndex = remap(v.activeIndex)
	if v.loading {
		v.loadingIndex = remap(v.loadingIndex)
	}
	v.hoverIndex = -1
	v.arrangeWorkspace()
	v.notifyAction("Moved " + filepath.Base(v.workspace[target].path))
	return target
}

func (v *viewer) showImageList() {
	v.focusImage()
	originalOrder := v.snapshotImageOrder()
	var dialog *walk.Dialog
	var up, down, closeButton, cancelButton *walk.PushButton
	updateButtons := func() {
		if v.imageOrderList == nil || up == nil || down == nil {
			return
		}
		index := v.imageOrderList.CurrentIndex()
		up.SetEnabled(index > 0)
		down.SetEnabled(index >= 0 && index < len(v.workspace)-1)
		if index >= 0 && index < len(v.workspace) {
			v.imageOrderList.SetToolTipText(v.workspace[index].path)
		}
	}
	move := func(delta int) {
		index := v.moveWorkspaceImage(v.imageOrderList.CurrentIndex(), delta)
		v.imageOrderList.SetCurrentIndex(index)
		updateButtons()
		v.imageOrderList.SetFocus()
	}
	applySort := func(sortImages func()) {
		var selectedID uint64
		if index := v.imageOrderList.CurrentIndex(); index >= 0 && index < len(v.workspace) {
			selectedID = v.workspace[index].orderID
		}
		sortImages()
		for i, item := range v.workspace {
			if item.orderID == selectedID {
				v.imageOrderList.SetCurrentIndex(i)
				break
			}
		}
		updateButtons()
	}
	err := (Dialog{AssignTo: &dialog, Title: "Image order", Size: Size{Width: 600, Height: 440}, MinSize: Size{Width: 580, Height: 240}, Layout: VBox{}, CancelButton: &cancelButton, Children: []Widget{
		Label{Text: "Images appear on the canvas in this order."},
		ListBox{AssignTo: &v.imageOrderList, StretchFactor: 1, OnCurrentIndexChanged: updateButtons},
		Composite{Layout: HBox{}, Children: []Widget{
			PushButton{Text: "Sort alphabetically", OnClicked: func() { applySort(v.sortImagesAlphabetically) }},
			PushButton{Text: "Sort by file size", OnClicked: func() { applySort(v.sortImagesByFileSize) }},
			HSpacer{},
		}},
		Composite{Layout: HBox{}, Children: []Widget{
			PushButton{AssignTo: &up, Text: "Move Up", OnClicked: func() { move(-1) }},
			PushButton{AssignTo: &down, Text: "Move Down", OnClicked: func() { move(1) }},
			HSpacer{},
			PushButton{AssignTo: &cancelButton, Text: "Cancel", OnClicked: func() { dialog.Cancel() }},
			PushButton{AssignTo: &closeButton, Text: "OK", OnClicked: func() { dialog.Accept() }},
		}},
	}}).Create(v.window)
	if err != nil {
		v.imageOrderList = nil
		v.showError(err)
		return
	}
	defer dialog.Dispose()
	defer func() { v.imageOrderList = nil }()
	v.applyImageListTheme(dialog)
	v.refreshImageList()
	index := v.activeIndex
	if index < 0 && len(v.workspace) > 0 {
		index = 0
	}
	v.imageOrderList.SetCurrentIndex(index)
	updateButtons()
	if dialog.Run() != walk.DlgCmdOK {
		v.restoreImageOrder(originalOrder)
		v.notifyAction("Image order restored")
	}
}

func (v *viewer) sortImagesAlphabetically() {
	ids := v.snapshotImageOrder()
	names := make(map[uint64]string, len(ids))
	for _, item := range v.workspace {
		names[item.orderID] = strings.ToLower(filepath.Base(item.path))
	}
	sort.SliceStable(ids, func(i, j int) bool { return names[ids[i]] < names[ids[j]] })
	v.restoreImageOrder(ids)
	v.notifyAction("Sorted images alphabetically - A to Z")
}

func (v *viewer) sortImagesByFileSize() {
	ids := v.snapshotImageOrder()
	sizes := make(map[uint64]int64, len(ids))
	for _, item := range v.workspace {
		size := int64(1<<63 - 1) // Unavailable files sort last.
		if item.image != nil {
			size = item.image.fileSize
		} else if info, err := os.Stat(item.path); err == nil {
			size = info.Size()
		}
		sizes[item.orderID] = size
	}
	sort.SliceStable(ids, func(i, j int) bool { return sizes[ids[i]] < sizes[ids[j]] })
	v.restoreImageOrder(ids)
	v.notifyAction("Sorted images by file size - smallest first")
}

func (v *viewer) snapshotImageOrder() []uint64 {
	ids := make([]uint64, len(v.workspace))
	for i := range v.workspace {
		if v.workspace[i].orderID == 0 {
			v.nextOrderID++
			v.workspace[i].orderID = v.nextOrderID
		}
		ids[i] = v.workspace[i].orderID
	}
	return ids
}

func (v *viewer) restoreImageOrder(ids []uint64) {
	var activeID, loadingID uint64
	if v.activeIndex >= 0 && v.activeIndex < len(v.workspace) {
		activeID = v.workspace[v.activeIndex].orderID
	}
	if v.loading && v.loadingIndex >= 0 && v.loadingIndex < len(v.workspace) {
		loadingID = v.workspace[v.loadingIndex].orderID
	}
	byID := make(map[uint64]workspaceImage, len(v.workspace))
	for _, item := range v.workspace {
		byID[item.orderID] = item
	}
	items := make([]workspaceImage, 0, len(v.workspace))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			items = append(items, item)
			delete(byID, id)
		}
	}
	// Keep images added while the dialog was open, following the original set.
	for _, item := range v.workspace {
		if _, ok := byID[item.orderID]; ok {
			items = append(items, item)
		}
	}
	v.workspace = items
	v.activeIndex = -1
	for i, item := range items {
		if activeID != 0 && item.orderID == activeID {
			v.activeIndex = i
		}
		if loadingID != 0 && item.orderID == loadingID {
			v.loadingIndex = i
		}
	}
	v.hoverIndex = -1
	v.arrangeWorkspace()
}

func (v *viewer) applyImageListTheme(dialog *walk.Dialog) {
	var buttons func(walk.Container)
	buttons = func(container walk.Container) {
		for i := 0; i < container.Children().Len(); i++ {
			child := container.Children().At(i)
			if _, ok := child.(*walk.PushButton); ok {
				installStaticButton(child.Handle(), v.dark)
			}
			if nested, ok := child.(walk.Container); ok {
				buttons(nested)
			}
		}
	}
	buttons(dialog)
	if !v.dark {
		return
	}
	brush, err := walk.NewSolidColorBrush(themeSurface(true))
	if err != nil {
		return
	}
	dialog.Disposing().Attach(brush.Dispose)
	value := int32(1)
	callThemeOrdinal(133, uintptr(dialog.Handle()), 1)
	dwmSetAttribute.Call(uintptr(dialog.Handle()), 20, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
	var apply func(walk.Window)
	apply = func(window walk.Window) {
		window.SetBackground(brush)
		callThemeOrdinal(133, uintptr(window.Handle()), 1)
		win.SetWindowTheme(window.Handle(), syscall.StringToUTF16Ptr("DarkMode_Explorer"), nil)
		if label, ok := window.(*walk.Label); ok {
			label.SetTextColor(walk.RGB(230, 230, 230))
		}
		if container, ok := window.(walk.Container); ok {
			installThemePainting(window.Handle(), 1)
			for i := 0; i < container.Children().Len(); i++ {
				apply(container.Children().At(i))
			}
		}
	}
	apply(dialog)
}
