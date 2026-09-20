//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"os/exec"
)

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
