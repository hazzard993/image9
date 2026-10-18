//go:build windows

package main

import (
	"runtime"
	"testing"

	"github.com/lxn/walk"
)

func TestEmbeddedApplicationIcon(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// The resource compiler assigns the manifest ID 1 and icon group ID 2.
	for _, size := range []int{16, 32, 48, 256} {
		icon, err := walk.NewIconFromResourceIdWithSize(2, walk.Size{Width: size, Height: size})
		if err != nil {
			t.Fatalf("embedded %dpx icon: %v", size, err)
		}
		icon.Dispose()
	}
}
