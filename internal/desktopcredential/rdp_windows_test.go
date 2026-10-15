//go:build windows

package desktopcredential

import (
	"golang.org/x/sys/windows"
	"testing"
	"unsafe"
)

func TestNativeCredentialVaultOwnershipAndForget(t *testing.T) {
	owner := "pfremote://fabric-example/devices/device-vault-test/capabilities/desktop"
	defer clearRDPCredentials(owner)
	if err := PrepareRDP("pfremote-vault-test.example.invalid:59999", "example-user", "synthetic-password", owner); err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString("TERMSRV/pfremote-vault-test.example.invalid")
	read := credentialDLL.NewProc("CredReadW")
	var c *nativeCredential
	ok, _, _ := read.Call(uintptr(unsafe.Pointer(name)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok == 0 {
		t.Fatal("credential missing")
	}
	matched := windows.UTF16PtrToString(c.Username) == "example-user" && windows.UTF16PtrToString(c.Comment) == credentialMarker(owner) && c.BlobSize == uint32(len("synthetic-password")*2)
	credFree.Call(uintptr(unsafe.Pointer(c)))
	if !matched {
		t.Fatal("credential metadata mismatch")
	}
	if err := clearRDPCredentials(owner + "-other"); err != nil {
		t.Fatal(err)
	}
	ok, _, _ = read.Call(uintptr(unsafe.Pointer(name)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok == 0 {
		t.Fatal("other owner erased credential")
	}
	credFree.Call(uintptr(unsafe.Pointer(c)))
	if err := clearRDPCredentials(owner); err != nil {
		t.Fatal(err)
	}
	ok, _, _ = read.Call(uintptr(unsafe.Pointer(name)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok != 0 {
		credFree.Call(uintptr(unsafe.Pointer(c)))
		t.Fatal("credential survived forget")
	}
}
