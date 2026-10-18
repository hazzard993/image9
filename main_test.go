//go:build windows

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
