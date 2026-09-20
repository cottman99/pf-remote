//go:build windows

package autoupdate

import (
	"errors"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
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

// Idle is deliberately conservative across the local Windows user sessions.
// It does not infer Desktop completion from the short-lived launch request.
func Idle() bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	sshd := 0
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		switch name {
		case "mstsc.exe", "vncviewer.exe", "ssh.exe", "sshd-session.exe":
			return false
		case "sshd.exe":
			sshd++
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) || sshd > 1 {
		return false
	}
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count) != nil {
		return false
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	if count > 4096 {
		return false
	}
	for _, s := range unsafe.Slice(sessions, int(count)) {
		if s.State == 0 && s.SessionID != windows.WTSGetActiveConsoleSessionId() {
			return false
		}
	}
	// The current Gateway binary hosts only transactional enrollment/control
	// handlers, not content streams. Idle HTTP keep-alive connections must not
	// postpone upgrades forever. Content remains in independent SSH/RDP/FRP paths.
	return true
}
