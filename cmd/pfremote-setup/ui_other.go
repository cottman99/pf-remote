//go:build !windows

package main

import "errors"

func notifyInstallResult(bool)    {}
func updateShortcut(string) error { return errors.New("PF Remote Setup is available only on Windows") }
func removeShortcut() error       { return errors.New("PF Remote Setup is available only on Windows") }
func updateInstalledApps(string, string) error {
	return errors.New("PF Remote Setup is available only on Windows")
}

func updateStartup() error { return errors.New("PF Remote Setup is available only on Windows") }
func removeStartup() error { return errors.New("PF Remote Setup is available only on Windows") }
func ensureBackground(string) error {
	return errors.New("PF Remote Setup is available only on Windows")
}
func restartBackground(string, string) error {
	return errors.New("PF Remote Setup is available only on Windows")
}
func configureGatewayStartup(string, string, string) error {
	return errors.New("PF Remote Setup is available only on Windows")
}
func removeInstalledApps() error { return errors.New("PF Remote Setup is available only on Windows") }
