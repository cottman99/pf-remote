//go:build windows

package desktopcredential

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"runtime"
	"unicode/utf16"
	"unsafe"
)

var credentialDLL = windows.NewLazySystemDLL("advapi32.dll")
var credWrite = credentialDLL.NewProc("CredWriteW")
var credEnumerate = credentialDLL.NewProc("CredEnumerateW")
var credDelete = credentialDLL.NewProc("CredDeleteW")
var credFree = credentialDLL.NewProc("CredFree")

type nativeCredential struct {
	Flags          uint32
	Type           uint32
	TargetName     *uint16
	Comment        *uint16
	LastWritten    windows.Filetime
	BlobSize       uint32
	Blob           *byte
	Persist        uint32
	AttributeCount uint32
	Attributes     uintptr
	TargetAlias    *uint16
	Username       *uint16
}

func credentialMarker(owner string) string { return "PFRemote/" + targetDigest(owner) }

// PrepareRDP places only the selected target's account in the OS credential
// vault. Passwords never appear in a process argument, environment or RDP file.
func PrepareRDP(endpoint, username, password, owner string) error {
	if owner == "" || password == "" {
		return nil
	}
	// MSTSC looks up credentials by server name, independently of the TCP port.
	// PF Remote selects and validates the exact target before populating this
	// native cache; each preset itself remains bound to the full target identity.
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" {
		return errors.New("invalid credential endpoint")
	}
	target, err := windows.UTF16PtrFromString("TERMSRV/" + host)
	if err != nil {
		return errors.New("invalid credential target")
	}
	user, err := windows.UTF16PtrFromString(username)
	if err != nil {
		return errors.New("invalid credential account")
	}
	comment, _ := windows.UTF16PtrFromString(credentialMarker(owner))
	secret := utf16.Encode([]rune(password))
	defer clear(secret)
	if len(secret) == 0 || len(secret) > 1200 {
		return errors.New("invalid credential length")
	}
	c := nativeCredential{Type: 1, TargetName: target, Comment: comment, Username: user, BlobSize: uint32(len(secret) * 2), Blob: (*byte)(unsafe.Pointer(&secret[0])), Persist: 2}
	ok, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(secret)
	if ok == 0 {
		return errors.New("save Windows remote desktop credential")
	}
	return nil
}
func clearRDPCredentials(owner string) error {
	filter, _ := windows.UTF16PtrFromString("TERMSRV/*")
	var count uint32
	var array **nativeCredential
	ok, _, err := credEnumerate.Call(uintptr(unsafe.Pointer(filter)), 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&array)))
	if ok == 0 {
		if err == windows.ERROR_NOT_FOUND {
			return nil
		}
		return errors.New("read Windows remote desktop credentials")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(array)))
	for _, c := range unsafe.Slice(array, int(count)) {
		if c.Type == 1 && windows.UTF16PtrToString(c.Comment) == credentialMarker(owner) {
			ok, _, _ := credDelete.Call(uintptr(unsafe.Pointer(c.TargetName)), 1, 0)
			if ok == 0 {
				return errors.New("remove Windows remote desktop credential")
			}
		}
	}
	return nil
}
