//go:build windows

package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
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
	}
	// Restore the native process preference for any subsequent GUI tests.
	setNativeAppTheme(false)
}
