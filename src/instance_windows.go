//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const copyImagesMessage = 0x51495631
const maxInstanceMessage = 1024 * 1024

type viewerInstance struct {
	mutex    windows.Handle
	property *uint16
	onImages func([]string)
}
type instanceSearch struct {
	property *uint16
	window   win.HWND
}
type imageCopyData struct {
	kind uintptr
	size uint32
	data unsafe.Pointer
}

var (
	instanceWindows                        sync.Map
	instanceCallback, instanceEnumCallback uintptr
	enumWindowsProc                        = userDLL.NewProc("EnumWindows")
	getPropProc                            = userDLL.NewProc("GetPropW")
	setPropProc                            = userDLL.NewProc("SetPropW")
	removePropProc                         = userDLL.NewProc("RemovePropW")
	sendMessageTimeoutProc                 = userDLL.NewProc("SendMessageTimeoutW")
	allowForegroundProc                    = userDLL.NewProc("AllowSetForegroundWindow")
	createInstanceMutexProc                = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateMutexW")
)

func instanceIdentity() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(dir)))
	return fmt.Sprintf("image9-%x", hash[:16]), nil
}

// The named mutex is owned by the UI thread. A second launch sends a bounded
// WM_COPYDATA message to the marked viewer window; no file/network rendezvous is
// needed. Local\ scopes the mutex to this interactive Windows session.
func acquireViewerInstance(paths []string) (*viewerInstance, bool, error) {
	identity, err := instanceIdentity()
	if err != nil {
		return nil, false, err
	}
	paths, err = normalizeImagePaths(paths)
	if err != nil {
		return nil, false, err
	}
	name := syscall.StringToUTF16Ptr("Local\\" + identity)
	// Preserve ERROR_ALREADY_EXISTS even when CreateMutex returns a valid
	// handle, to distinguish the owner from subsequent launches.
	rawHandle, _, createErr := createInstanceMutexProc.Call(0, 1, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if rawHandle == 0 {
		return nil, false, createErr
	}
	handle := windows.Handle(rawHandle)
	instance := &viewerInstance{mutex: handle, property: syscall.StringToUTF16Ptr(identity)}
	if createErr != windows.ERROR_ALREADY_EXISTS {
		return instance, false, nil
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, waitErr := windows.WaitForSingleObject(handle, 0)
		if waitErr == nil && (state == windows.WAIT_OBJECT_0 || state == windows.WAIT_ABANDONED) {
			return instance, false, nil
		}
		if hwnd := findViewerWindow(instance.property); hwnd != 0 {
			err := forwardImages(hwnd, paths)
			windows.CloseHandle(handle)
			return nil, err == nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
	windows.CloseHandle(handle)
	return nil, false, fmt.Errorf("the existing viewer is still starting or is not responding; please try again")
}

func findViewerWindow(property *uint16) win.HWND {
	if instanceEnumCallback == 0 {
		instanceEnumCallback = syscall.NewCallback(func(hwnd win.HWND, context unsafe.Pointer) uintptr {
			search := (*instanceSearch)(context)
			value, _, _ := getPropProc.Call(uintptr(hwnd), uintptr(unsafe.Pointer(search.property)))
			if value != 0 {
				search.window = hwnd
				return 0
			}
			return 1
		})
	}
	search := instanceSearch{property: property}
	enumWindowsProc.Call(instanceEnumCallback, uintptr(unsafe.Pointer(&search)))
	runtime.KeepAlive(property)
	return search.window
}

func forwardImages(hwnd win.HWND, paths []string) error {
	data, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if len(data) > maxInstanceMessage {
		return fmt.Errorf("too many file paths to send to the existing viewer")
	}
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	allowForegroundProc.Call(uintptr(pid))
	message := imageCopyData{kind: copyImagesMessage, size: uint32(len(data)), data: unsafe.Pointer(&data[0])}
	var accepted uintptr
	result, _, _ := sendMessageTimeoutProc.Call(uintptr(hwnd), win.WM_COPYDATA, 0, uintptr(unsafe.Pointer(&message)), 3, 5000, uintptr(unsafe.Pointer(&accepted)))
	runtime.KeepAlive(data)
	if result == 0 || accepted != 1 {
		return fmt.Errorf("the existing viewer could not accept the images; please try again")
	}
	return nil
}

func (instance *viewerInstance) attach(hwnd win.HWND, onImages func([]string)) error {
	if instanceCallback == 0 {
		instanceCallback = syscall.NewCallback(viewerInstanceProc)
	}
	instance.onImages = onImages
	instanceWindows.Store(hwnd, instance)
	ok, _, _ := setSubclass.Call(uintptr(hwnd), instanceCallback, 4, 0)
	if ok == 0 {
		instanceWindows.Delete(hwnd)
		return fmt.Errorf("could not install the viewer's image receiver")
	}
	ok, _, _ = setPropProc.Call(uintptr(hwnd), uintptr(unsafe.Pointer(instance.property)), 1)
	if ok == 0 {
		return fmt.Errorf("could not register the existing viewer window")
	}
	return nil
}

func (instance *viewerInstance) close() {
	windows.ReleaseMutex(instance.mutex)
	windows.CloseHandle(instance.mutex)
}

func viewerInstanceProc(hwnd win.HWND, message uint32, wParam uintptr, lParam unsafe.Pointer, id, data uintptr) uintptr {
	if value, ok := instanceWindows.Load(hwnd); ok {
		instance := value.(*viewerInstance)
		switch message {
		case win.WM_COPYDATA:
			if lParam == nil {
				return 0
			}
			copyData := (*imageCopyData)(lParam)
			if copyData.kind != copyImagesMessage || copyData.size == 0 || copyData.size > maxInstanceMessage || copyData.data == nil {
				return 0
			}
			var paths []string
			if json.Unmarshal(unsafe.Slice((*byte)(copyData.data), int(copyData.size)), &paths) != nil {
				return 0
			}
			for _, path := range paths {
				if !filepath.IsAbs(path) {
					return 0
				}
			}
			instance.onImages(paths)
			if win.IsIconic(hwnd) {
				win.ShowWindow(hwnd, win.SW_RESTORE)
			}
			win.SetForegroundWindow(hwnd)
			return 1
		case win.WM_NCDESTROY:
			removePropProc.Call(uintptr(hwnd), uintptr(unsafe.Pointer(instance.property)))
			instanceWindows.Delete(hwnd)
			removeSubclass.Call(uintptr(hwnd), instanceCallback, id)
		}
	}
	result, _, _ := defaultSubclass.Call(uintptr(hwnd), uintptr(message), wParam, uintptr(lParam))
	return result
}
