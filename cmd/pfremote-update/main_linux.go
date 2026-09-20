//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cottman99/pf-remote/internal/autoupdate"
	"github.com/cottman99/pf-remote/internal/linuxinstaller"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

var releaseVersion = "development"

func main() {
	if run() != nil {
		os.Exit(1)
	}
}
func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := filepath.Join(home, ".local", "lib", "pfremote", "bin")
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(root, ".update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	action := "install"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}
	if action != "install" && action != "apply-update" && action != "recover" {
		return errors.New("invalid action")
	}
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if action == "recover" && (errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)) {
			return nil
		}
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if action == "recover" {
		return linuxinstaller.Recover(root)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	stage := filepath.Dir(executable)
	if action == "install" {
		if err = autoupdate.Bootstrap(); err != nil {
			return err
		}
	}
	verified, err := autoupdate.VerifyPackage(stage, releaseVersion)
	if err != nil {
		return err
	}
	_, updates, err := autoupdate.Paths()
	if err != nil {
		return err
	}
	phase := "failed"
	defer func() {
		_ = autoupdate.WriteStatus(updates, autoupdate.Status{Phase: phase, Version: releaseVersion, Sequence: verified.Metadata().Sequence})
	}()
	if action == "apply-update" && !autoupdate.Idle() {
		phase = "busy"
		return errors.New("active session")
	}
	// Recovery executes before this existing managed service starts, without
	// altering its environment, key synchronization timer or protocol servers.
	drop := filepath.Join(home, ".config", "systemd", "user", "pfremote-node.service.d")
	if err = os.MkdirAll(drop, 0700); err != nil {
		return err
	}
	unit := "[Service]\nExecStartPre=%h/.local/lib/pfremote/bin/pfremote-update recover\n"
	// Install the recovery helper before the drop-in is activated. Upgrade backs
	// it up together with the existing managed binaries.
	helper := filepath.Join(root, "pfremote-update")
	if _, err = os.Stat(helper); os.IsNotExist(err) {
		data, e := os.ReadFile(executable)
		if e != nil {
			return e
		}
		if e = os.WriteFile(helper, data, 0700); e != nil {
			return e
		}
	}
	if err = os.WriteFile(filepath.Join(drop, "updates.conf"), []byte(unit), 0600); err != nil {
		return err
	}
	if err = exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return err
	}
	activate := func() error {
		if err := exec.Command("systemctl", "--user", "restart", "pfremote-node.service").Run(); err != nil {
			return err
		}
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			data, err := exec.Command(filepath.Join(root, "pfremote"), "doctor", "--json").Output()
			var d contracts.DoctorResponse
			if err == nil && json.Unmarshal(data, &d) == nil {
				for _, check := range d.Checks {
					if check.Name == "identity" && check.Status == "pass" {
						return nil
					}
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
		return errors.New("new daemon health check failed")
	}
	if err = linuxinstaller.Upgrade(root, stage, activate); err != nil {
		return err
	}
	phase = "complete"
	return nil
}
