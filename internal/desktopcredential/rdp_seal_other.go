//go:build !windows

package desktopcredential

import "errors"

func SealRDPPassword(string) (string, error) {
	return "", errors.New("native RDP credential protection requires Windows")
}
