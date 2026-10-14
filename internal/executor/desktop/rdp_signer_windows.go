//go:build windows

package desktop

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed rdp_signer.ps1
var rdpSignerScript string

func signRDPProfile(ctx context.Context, path string) error {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	script := "$rdpFile = '" + strings.ReplaceAll(path, "'", "''") + "'\n" + rdpSignerScript
	shell := filepath.Join(os.Getenv("WINDIR"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(bounded, shell, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Run(); err != nil {
		return errors.New("local RDP profile signing failed")
	}
	return nil
}
