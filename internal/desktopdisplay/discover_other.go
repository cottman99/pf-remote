//go:build !windows

package desktopdisplay

import "github.com/cottman99/pf-remote/pkg/contracts"

func Discover() (contracts.DesktopMonitorLayout, error) {
	return contracts.DesktopMonitorLayout{}, ErrSelection
}
