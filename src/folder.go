//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

func adjacentImage(path string, delta int) (string, error) {
	dir, name := filepath.Dir(path), filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tif", ".tiff", ".webp":
			names = append(names, entry.Name())
		}
	}
	if len(names) < 2 {
		return "", nil
	}
	for i, candidate := range names {
		if strings.EqualFold(candidate, name) {
			next := (i + delta + len(names)) % len(names)
			return filepath.Join(dir, names[next]), nil
		}
	}
	return "", nil
}

func (v *viewer) browseFolder(delta int) {
	if v.loading {
		return
	}
	path, err := adjacentImage(v.workspace[0].path, delta)
	if err != nil {
		v.notifyAction("Unable to browse image folder")
		return
	}
	if path == "" {
		return
	}
	id := v.workspace[0].orderID
	v.workspace[0] = workspaceImage{orderID: id, path: path}
	v.arrangeWorkspace()
	v.selectImage(0)
	v.loadNextWorkspaceImage()
}
