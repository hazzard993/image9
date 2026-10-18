//go:build windows

package main

import "testing"

func TestWrapWidthTracksGapChanges(t *testing.T) {
	items := make([]workspaceImage, 5)
	for i := range items {
		items[i].image = &decodedImage{width: 64, height: 64}
	}
	arrangeWorkspaceWithGap(items, 208, 8)
	width := wrapWidthForGap(items, 208, 8, 0)
	if width != 192 {
		t.Fatalf("two 8px gaps should reduce wrap by 16px, got %d", width)
	}
	arrangeWorkspaceWithGap(items, width, 0)
	if items[2].y != 0 || items[3].y != 64 {
		t.Fatal("gap change did not retain three images in the first row")
	}
	if got := wrapWidthForGap(items, width, 0, 8); got != 208 {
		t.Fatalf("increasing gaps should restore wrap width, got %d", got)
	}
	if got := wrapWidthForGap(items, 0, 8, 0); got != 0 {
		t.Fatal("gap change enabled wrapping")
	}
	arrangeWorkspaceWithGap(items, 64, 8)
	if got := wrapWidthForGap(items, 64, 8, 0); got != 64 {
		t.Fatal("single-image rows changed wrap width")
	}
}
