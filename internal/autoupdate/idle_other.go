//go:build !windows

package autoupdate

import "errors"

func StartSetup(string) error { return errors.New("automatic installer unavailable") }

// Non-Windows installation is not implemented by the Windows release worker.
func Idle() bool { return false }
