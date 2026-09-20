//go:build linux

package autoupdate

import (
	"os"
	"os/exec"
)

func StartSetup(path string) error {
	if err := os.Chmod(path, 0700); err != nil {
		return err
	}
	// A transient user service survives the daemon service's own restart.
	return exec.Command("systemd-run", "--user", "--quiet", "--collect", "--unit=pfremote-update", path, "apply-update").Run()
}
func Idle() bool {
	// The daemon's ActionGate drains its own commands before handoff. Incoming
	// SSH and desktop servers belong to independent services, not pfremote-node's
	// cgroup. Their persistent logins must not indefinitely prevent an update.
	return true
}
