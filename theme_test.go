//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func TestThemePreferenceRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	for _, dark := range []bool{true, false, true} {
		saveDarkMode(dark)
		if got := loadDarkMode(); got != dark {
			t.Fatalf("saved dark=%v, loaded %v", dark, got)
		}
	}
	if _, err := os.Stat(themeConfigPath()); err != nil {
		t.Fatal(err)
	}
}

func TestRenamedAppKeepsPreferences(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	legacy := filepath.Join(dir, "QuickImageViewer")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "theme.txt"), []byte("dark\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "colour-format.txt"), []byte("rgb\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !loadDarkMode() || loadCopyFormat() != formatRGB {
		t.Fatal("legacy preferences were lost")
	}
	saveDarkMode(false)
	if loadDarkMode() {
		t.Fatal("new preferences did not override legacy settings")
	}
	if filepath.Dir(themeConfigPath()) != filepath.Join(dir, "image9") {
		t.Fatal("preferences still use the old app directory")
	}
}

func TestNativeThemeSwitch(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	v := &viewer{}
	err := (MainWindow{AssignTo: &v.window, Visible: false, Layout: VBox{}, MenuItems: []MenuItem{
		Menu{Text: "View", Items: []MenuItem{Menu{Text: "Background", Items: []MenuItem{
			Action{Text: "Charcoal", Shortcut: Shortcut{Key: walk.Key1}},
		}}}},
	}, Children: []Widget{
		Composite{AssignTo: &v.footer, Layout: HBox{}, Children: []Widget{
			Label{AssignTo: &v.status, Text: "Image details"},
			PushButton{Text: "Open"},
			ImageView{AssignTo: &v.palette},
		}},
	}}).Create()
	if err != nil {
		t.Fatal(err)
	}
	defer v.window.Dispose()
	v.registerMenuShortcuts(v.window.Menu())
	if v.window.ShortcutActions().Len() != 1 || v.window.ShortcutActions().At(0).Shortcut().Key != walk.Key1 {
		t.Fatal("nested background shortcut not registered")
	}
	t.Run("Direct2D render and recovery", exerciseDirect2DRenderAndRecovery)
	t.Run("Spatial workspace loading", exerciseSpatialLoading)
	for _, dark := range []bool{true, false, true} {
		v.dark = dark
		v.applyTheme()
		brush, ok := v.footer.Background().(*walk.SolidColorBrush)
		if !ok || brush.Color() != themeSurface(dark) {
			t.Fatal("footer theme did not change")
		}
		button := v.footer.Children().At(1)
		button.SetBoundsPixels(walk.Rectangle{Width: 100, Height: 24})
		screen := win.GetDC(button.Handle())
		dc := win.CreateCompatibleDC(screen)
		bitmap := win.CreateCompatibleBitmap(screen, 100, 24)
		previous := win.SelectObject(dc, win.HGDIOBJ(bitmap))
		win.SendMessage(button.Handle(), win.BM_SETSTATE, 0, 0)
		win.SendMessage(button.Handle(), win.WM_PRINTCLIENT, uintptr(dc), 0)
		normal := win.GetPixel(dc, 2, 2)
		win.SendMessage(button.Handle(), win.BM_SETSTATE, 1, 0)
		win.SendMessage(button.Handle(), win.WM_PRINTCLIENT, uintptr(dc), 0)
		pressed := win.GetPixel(dc, 2, 2)
		win.SendMessage(button.Handle(), win.BM_SETSTATE, 0, 0)
		win.SelectObject(dc, previous)
		win.DeleteObject(win.HGDIOBJ(bitmap))
		win.DeleteDC(dc)
		win.ReleaseDC(button.Handle(), screen)
		if normal == pressed || dark && pressed != 0x00272727 || !dark && pressed != 0x00cccccc {
			t.Fatalf("button did not immediately paint its pressed colour: normal %x pressed %x", normal, pressed)
		}
	}
	// Restore the native process preference for any subsequent GUI tests.
	t.Run("Image order dialog dark theme", func(t *testing.T) {
		var dialog *walk.Dialog
		var list *walk.ListBox
		if err := (Dialog{AssignTo: &dialog, Visible: false, Layout: VBox{}, Children: []Widget{ListBox{AssignTo: &list, Model: []string{"sample.png"}}}}).Create(v.window); err != nil {
			t.Fatal(err)
		}
		defer dialog.Dispose()
		v.dark = true
		v.applyImageListTheme(dialog)
		if brush, ok := dialog.Background().(*walk.SolidColorBrush); !ok || brush.Color() != themeSurface(true) {
			t.Fatal("dialog did not receive dark background")
		}
		dc := win.GetDC(list.Handle())
		defer win.ReleaseDC(list.Handle(), dc)
		if result := win.SendMessage(dialog.Handle(), win.WM_CTLCOLORLISTBOX, uintptr(dc), uintptr(list.Handle())); result == 0 {
			t.Fatal("dark list background brush was missing")
		}
		if win.GetTextColor(dc) != 0x00e6e6e6 || win.GetBkColor(dc) != win.COLORREF(themeSurface(true)) {
			t.Fatal("list text or background was not dark themed")
		}
	})
	setNativeAppTheme(false)
}
