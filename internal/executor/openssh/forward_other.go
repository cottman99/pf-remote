//go:build !windows

package openssh

import "os/exec"

func configureForwardProcess(*exec.Cmd) {}
