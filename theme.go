//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	uxTheme         = windows.NewLazySystemDLL("uxtheme.dll")
	dwmSetAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

func themeSurface(dark bool) walk.Color {
	if dark {
		return walk.RGB(38, 38, 41)
	}
	return walk.RGB(240, 240, 240)
}

func themeConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "QuickImageViewer", "theme.txt")
}

func loadDarkMode() bool {
	if data, err := os.ReadFile(themeConfigPath()); err == nil {
		switch strings.TrimSpace(string(data)) {
		case "dark":
			return true
		case "light":
			return false
		}
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	return err == nil && value == 0
}

func saveDarkMode(dark bool) {
	path := themeConfigPath()
	if path == "" {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	value := "light\n"
	if dark {
		value = "dark\n"
	}
	// A read-only config directory should not prevent changing the current theme.
	_ = os.WriteFile(path, []byte(value), 0600)
}

// Native control dark themes use optional uxtheme ordinals, as in Microsoft's
// PowerToys. Gate their use to Windows 10 1903+; older systems keep native chrome.
// Named APIs and all optional exports are checked before calls.
func callThemeOrdinal(ordinal uintptr, args ...uintptr) {
	if windows.RtlGetVersion().BuildNumber < 18362 || uxTheme.Load() != nil {
		return
	}
	address, err := windows.GetProcAddressByOrdinal(windows.Handle(uxTheme.Handle()), ordinal)
	if err != nil {
		return
	}
	switch len(args) {
	case 0:
		syscall.Syscall(address, 0, 0, 0, 0)
	case 1:
		syscall.Syscall(address, 1, args[0], 0, 0)
	case 2:
		syscall.Syscall(address, 2, args[0], args[1], 0)
	}
}

func setNativeAppTheme(dark bool) {
	mode := uintptr(3) // ForceLight
	if dark {
		mode = 2
	} // ForceDark
	callThemeOrdinal(135, mode)
	callThemeOrdinal(136) // FlushMenuThemes
}

func (v *viewer) applyTheme() {
	brush, err := walk.NewSolidColorBrush(themeSurface(v.dark))
	if err != nil {
		return
	}
	old := v.themeBrush
	v.themeBrush = brush
	v.window.SetBackground(brush)
	v.footer.SetBackground(brush)
	v.status.SetBackground(brush)
	text := walk.RGB(32, 32, 32)
	if v.dark {
		text = walk.RGB(230, 230, 230)
	}
	v.status.SetTextColor(text)
	if old != nil {
		old.Dispose()
	}
	setNativeAppTheme(v.dark)
	enabled := uintptr(0)
	if v.dark {
		enabled = 1
	}
	callThemeOrdinal(133, uintptr(v.window.Handle()), enabled)
	if dwmSetAttribute.Find() == nil {
		value := int32(enabled)
		result, _, _ := dwmSetAttribute.Call(uintptr(v.window.Handle()), 20, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		if int32(result) < 0 {
			dwmSetAttribute.Call(uintptr(v.window.Handle()), 19, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		}
	}
	theme := "Explorer"
	if v.dark {
		theme = "DarkMode_Explorer"
	}
	name := syscall.StringToUTF16Ptr(theme)
	installThemePainting(v.footer.Handle(), enabled)
	installThemePainting(v.window.Handle(), enabled)
	for i := 0; i < v.footer.Children().Len(); i++ {
		child := v.footer.Children().At(i)
		if _, ok := child.(*walk.PushButton); ok {
			installStaticButton(child.Handle(), v.dark)
			callThemeOrdinal(133, uintptr(child.Handle()), enabled)
			win.SetWindowTheme(child.Handle(), name, nil)
			child.Invalidate()
		}
	}
	v.palette.Invalidate()
	v.footer.Invalidate()
	win.DrawMenuBar(v.window.Handle())
}
