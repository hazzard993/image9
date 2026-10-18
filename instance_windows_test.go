//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func TestInstanceForwarding(t *testing.T) {
	if mode := os.Getenv("VIEWER_INSTANCE_TEST"); mode != "" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		paths := []string{"relative image.png", "colour-é.png"}
		instance, forwarded, err := acquireViewerInstance(paths)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "secondary" {
			if !forwarded {
				t.Fatal("secondary became primary")
			}
			return
		}
		if forwarded {
			t.Fatal("primary forwarded")
		}
		defer instance.close()
		var window *walk.MainWindow
		if err := (MainWindow{AssignTo: &window, Visible: false, Layout: VBox{}}).Create(); err != nil {
			t.Fatal(err)
		}
		defer window.Dispose()
		if err := instance.attach(window.Handle(), func(received []string) {
			data, _ := json.Marshal(received)
			if err := os.WriteFile(filepath.Join(os.Getenv("APPDATA"), "received.json"), data, 0600); err != nil {
				panic(err)
			}
			win.PostMessage(window.Handle(), win.WM_CLOSE, 0, 0)
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("APPDATA"), "ready"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		window.Run()
		return
	}
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := func(mode string) *exec.Cmd {
		cmd := exec.Command(executable, "-test.run=^TestInstanceForwarding$")
		cmd.Env = append(os.Environ(), "VIEWER_INSTANCE_TEST="+mode)
		return cmd
	}
	primary := command("primary")
	if err := primary.Start(); err != nil {
		t.Fatal(err)
	}
	defer primary.Process.Kill()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("primary did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if output, err := command("secondary").CombinedOutput(); err != nil {
		t.Fatalf("secondary: %v: %s", err, output)
	}
	if err := primary.Wait(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "received.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want, err := normalizeImagePaths([]string{"relative image.png", "colour-é.png"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("received %v, want %v", got, want)
	}
}

func TestCaptionsHiddenWithoutCtrl(t *testing.T) {
	v := &viewer{renderer: &gpuRenderer{caption: "previous", captions: []imageCaption{{text: "previous"}}}, pointerInside: true}
	v.prepareHoverCaption(1, 0, 0, 800, 600)
	if v.renderer.caption != "" || len(v.renderer.captions) != 0 {
		t.Fatal("captions remained visible without Ctrl")
	}
}

func TestCtrlViewLabelDefaults(t *testing.T) {
	v := &viewer{renderer: &gpuRenderer{}, highlightBounds: true, workspace: []workspaceImage{{path: "sample.png", width: 64, height: 64, image: &decodedImage{width: 64, height: 64}}}}
	v.prepareHoverCaption(1, 400, 0, 800, 600)
	if len(v.renderer.captions) != 0 || len(v.renderer.shortcutLines) != 5 || v.renderer.shortcutLines[0] != "H / T - Show filename labels" || v.renderer.shortcutLines[1] != "C - Copy focused image path" || v.renderer.shortcutLines[3] != "L - Image list / reorder" || v.renderer.shortcutLines[4] != "O - Open containing folder" {
		t.Fatal("Ctrl view did not show H, C and G with labels hidden")
	}
	v.showFilenameLabels = true
	v.prepareHoverCaption(1, 400, 0, 800, 600)
	if len(v.renderer.captions) != 1 || len(v.renderer.shortcutLines) != 5 || v.renderer.shortcutLines[0] != "H / T - Hide filename labels" {
		t.Fatal("enabled labels did not retain the H reminder")
	}
	if v.renderer.shortcutX != 0 || v.renderer.shortcutY != 0 {
		t.Fatal("shortcut hints were not anchored to the canvas")
	}
}

func TestShortcutHintsWithEmptyWorkspace(t *testing.T) {
	v := &viewer{renderer: &gpuRenderer{}, highlightBounds: true}
	v.prepareHoverCaption(1, 400, 300, 800, 600)
	if len(v.renderer.shortcutLines) != 5 || v.renderer.shortcutX != 0 || v.renderer.shortcutY != 0 {
		t.Fatal("empty workspace did not keep hints at the fixed canvas anchor")
	}
	v.ctrlViewPinned = true
	v.prepareHoverCaption(2, 120, 240, 800, 600)
	if len(v.renderer.shortcutLines) != 7 || v.renderer.shortcutX != 0 || v.renderer.shortcutY != 0 {
		t.Fatal("Inspect Mode moved the shortcut anchor")
	}
}
