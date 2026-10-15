//go:build !windows

package desktopcredential

import "errors"

func PrepareRDP(endpoint, username, password, owner string) error {
	if password == "" {
		return nil
	}
	return errors.New("RDP account storage requires Windows")
}
func clearRDPCredentials(string) error { return nil }
