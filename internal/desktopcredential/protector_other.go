//go:build !windows

package desktopcredential

import "errors"

func newPlatformProtector() (protector, error) {
	return nil, errors.New("desktop credentials require Windows current-user protection")
}
