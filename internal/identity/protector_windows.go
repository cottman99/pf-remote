//go:build windows

package identity

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

type dpapiProtector struct{}

func newPlatformProtector() (protector, error) { return dpapiProtector{}, nil }

func (dpapiProtector) Name() string { return "windows-dpapi-current-user" }

func (dpapiProtector) Protect(plaintext []byte) ([]byte, error) {
	return transformDPAPI(plaintext, true)
}

func (dpapiProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	return transformDPAPI(ciphertext, false)
}

func transformDPAPI(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("DPAPI input is empty")
	}
	in := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]}
	var out windows.DataBlob
	var err error
	if protect {
		description, conversionErr := windows.UTF16PtrFromString("PF Remote device identity v1")
		if conversionErr != nil {
			return nil, conversionErr
		}
		err = windows.CryptProtectData(&in, description, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	if out.Data == nil || out.Size == 0 {
		return nil, errors.New("DPAPI returned an empty result")
	}
	decrypted := unsafe.Slice(out.Data, int(out.Size))
	result := append([]byte(nil), decrypted...)
	clear(decrypted)
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return result, nil
}
