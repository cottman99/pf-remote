package desktopdisplay

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var oleacc = windows.NewLazySystemDLL("oleacc.dll")
var accessibleObjectFromWindow = oleacc.NewProc("AccessibleObjectFromWindow")
var accessibleChildren = oleacc.NewProc("AccessibleChildren")
var oleaut = windows.NewLazySystemDLL("oleaut32.dll")
var freeBSTR = oleaut.NewProc("SysFreeString")
var bstrLength = oleaut.NewProc("SysStringLen")
var variantClear = oleaut.NewProc("VariantClear")
var ole32 = windows.NewLazySystemDLL("ole32.dll")
var coInitialize = ole32.NewProc("CoInitializeEx")
var coUninitialize = ole32.NewProc("CoUninitialize")
var accessibleIID = windows.GUID{Data1: 0x618736e0, Data2: 0x3c3d, Data3: 0x11cf, Data4: [8]byte{0x81, 0x0c, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}

type accessibleObject struct{ vtable *[28]uintptr }

// Windows x64 VARIANT includes an 8-byte tag and a 16-byte union.
type accessibleVariant struct {
	kind     uint16
	reserved [3]uint16
	value    uintptr
	extra    uintptr
}

func (object *accessibleObject) release() {
	syscall.SyscallN(object.vtable[2], uintptr(unsafe.Pointer(object)))
}

func (object *accessibleObject) name(child *accessibleVariant) string {
	var value *uint16
	hr, _, _ := syscall.SyscallN(object.vtable[10], uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(child)), uintptr(unsafe.Pointer(&value)))
	if int32(hr) < 0 || value == nil {
		return ""
	}
	defer freeBSTR.Call(uintptr(unsafe.Pointer(value)))
	n, _, _ := bstrLength.Call(uintptr(unsafe.Pointer(value)))
	if n > 8192 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice(value, n))
}

func accessibleText(hwnd uintptr) []string {
	var object *accessibleObject
	hr, _, _ := accessibleObjectFromWindow.Call(hwnd, 0xFFFFFFFC, uintptr(unsafe.Pointer(&accessibleIID)), uintptr(unsafe.Pointer(&object)))
	if int32(hr) < 0 || object == nil {
		return nil
	}
	defer object.release()
	var out []string
	budget := 128
	var visit func(*accessibleObject, int)
	visit = func(object *accessibleObject, depth int) {
		if budget <= 0 || depth > 8 {
			return
		}
		budget--
		self := accessibleVariant{kind: 3}
		if name := object.name(&self); name != "" {
			out = append(out, name)
		}
		var count int32
		hr, _, _ := syscall.SyscallN(object.vtable[8], uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&count)))
		if int32(hr) < 0 || count <= 0 || count > 128 {
			return
		}
		children := make([]accessibleVariant, count)
		var actual int32
		hr, _, _ = accessibleChildren.Call(uintptr(unsafe.Pointer(object)), 0, uintptr(count), uintptr(unsafe.Pointer(&children[0])), uintptr(unsafe.Pointer(&actual)))
		if int32(hr) < 0 || actual < 0 || actual > count {
			return
		}
		for i := int32(0); i < actual; i++ {
			child := &children[i]
			switch child.kind {
			case 3:
				if name := object.name(child); name != "" {
					out = append(out, name)
				}
			case 9:
				dispatch := *(**accessibleObject)(unsafe.Pointer(&child.value))
				var nested *accessibleObject
				if dispatch != nil {
					hr, _, _ := syscall.SyscallN(dispatch.vtable[0], uintptr(unsafe.Pointer(dispatch)), uintptr(unsafe.Pointer(&accessibleIID)), uintptr(unsafe.Pointer(&nested)))
					if int32(hr) >= 0 && nested != nil {
						visit(nested, depth+1)
						nested.release()
					}
				}
			}
			variantClear.Call(uintptr(unsafe.Pointer(child)))
		}
	}
	visit(object, 0)
	return out
}
