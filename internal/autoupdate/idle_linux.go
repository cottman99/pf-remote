//go:build linux

package autoupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func StartSetup(path string) error {
	if err := os.Chmod(path, 0700); err != nil {
		return err
	}
	// A transient user service survives the daemon service's own restart.
	return exec.Command("systemd-run", "--user", "--quiet", "--collect", "--unit=pfremote-update", path, "apply-update").Run()
}
func Idle() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != os.Getuid() {
			continue
		}
		name, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(name)) {
		case "ssh", "sshd", "sshd-session":
			return false
		}
	}
	return true
}
