//go:build windows

package autoupdate

import (
	"golang.org/x/sys/windows"
	"os/exec"
)

func StartSetup(path string) error {
	cmd := exec.Command(path, "apply-update", "--no-launch")
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// The daemon's ActionGate drains its owned commands and prevents new actions
// during handoff. Setup stops only version-owned Center/daemon/Gateway binaries;
// independent SSH/RDP/VNC executors and OS services are not stopped. Counting all
// remote viewers or incoming SSH sessions prevents always-connected hosts from
// ever updating, without protecting any process that Setup actually replaces.
func Idle() bool { return true }
