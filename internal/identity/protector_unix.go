//go:build !windows

package identity

type userPermissionProtector struct{}

func newPlatformProtector() (protector, error) { return userPermissionProtector{}, nil }

func (userPermissionProtector) Name() string { return "os-user-file-permissions" }

func (userPermissionProtector) Protect(plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (userPermissionProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}
