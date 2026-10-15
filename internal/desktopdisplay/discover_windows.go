package desktopdisplay

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/cottman99/pf-remote/pkg/contracts"
	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var createDesktop = user32.NewProc("CreateDesktopW")
var closeDesktop = user32.NewProc("CloseDesktop")
var enumDesktopWindows = user32.NewProc("EnumDesktopWindows")
var windowProcessID = user32.NewProc("GetWindowThreadProcessId")
var discoveryMu sync.Mutex
var discoveryActive *discoveryState

// Discover runs only the system client's read-only monitor listing. Its dialog
// belongs to a disposable desktop that is never activated. No user input,
// network connection, existing window or global display setting is involved.
func Discover() (contracts.DesktopMonitorLayout, error) {
	discoveryMu.Lock()
	defer discoveryMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	empty := contracts.DesktopMonitorLayout{}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return empty, ErrSelection
	}
	hr, _, _ := coInitialize.Call(0, 2)
	if int32(hr) < 0 {
		return empty, ErrSelection
	}
	defer coUninitialize.Call()
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return empty, ErrSelection
	}
	nameBytes := make([]byte, 12)
	if _, err = rand.Read(nameBytes); err != nil {
		return empty, ErrSelection
	}
	name, _ := windows.UTF16PtrFromString("PFRemoteDisplays-" + hex.EncodeToString(nameBytes))
	desktop, _, _ := createDesktop.Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, 0x10000000, 0)
	if desktop == 0 {
		return empty, ErrSelection
	}
	defer closeDesktop.Call(desktop)
	exe, _ := windows.UTF16PtrFromString(filepath.Join(directory, "mstsc.exe"))
	args, _ := windows.UTF16PtrFromString(`"` + filepath.Join(directory, "mstsc.exe") + `" /l`)
	startup := windows.StartupInfo{Desktop: name}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var process windows.ProcessInformation
	if err := windows.CreateProcess(exe, args, nil, nil, false, 0, nil, nil, &startup, &process); err != nil {
		return empty, ErrSelection
	}
	windows.CloseHandle(process.Thread)
	defer windows.CloseHandle(process.Process)
	// Bound accessibility RPCs as well as the polling loop. Only the disposable
	// listing process is terminated; wait for a running timer before closing its handle.
	timedOut := make(chan struct{})
	timer := time.AfterFunc(4*time.Second, func() { _ = windows.TerminateProcess(process.Process, 0); close(timedOut) })
	defer func() {
		if !timer.Stop() {
			<-timedOut
		}
	}()
	defer func() {
		_ = windows.TerminateProcess(process.Process, 0)
		_, _ = windows.WaitForSingleObject(process.Process, 2000)
	}()
	// Fixed callbacks avoid exhausting Go's Windows callback slots. The mutex
	// serializes the short-lived state; no pointer is round-tripped via uintptr.
	state := discoveryState{pid: process.ProcessId}
	discoveryActive = &state
	defer func() { discoveryActive = nil }()
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		state.texts = nil
		enumDesktopWindows.Call(desktop, desktopCallback, 0)
		if layout, err := parseMonitorList(strings.Join(state.texts, "\n")); err == nil {
			return layout, nil
		}
		if result, _ := windows.WaitForSingleObject(process.Process, 75); result == windows.WAIT_OBJECT_0 {
			break
		}
	}
	return empty, ErrSelection
}

type discoveryState struct {
	pid   uint32
	texts []string
}

var desktopCallback = windows.NewCallback(func(hwnd, param uintptr) uintptr {
	state := discoveryActive
	if state == nil {
		return 0
	}
	var pid uint32
	windowProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == state.pid {
		state.texts = append(state.texts, accessibleText(hwnd)...)
	}
	return 1
})
