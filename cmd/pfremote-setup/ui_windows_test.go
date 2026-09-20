//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestLocalHealthRequiresIdentityButToleratesOfflineCatalog(t *testing.T) {
	report := contracts.DoctorResponse{SchemaVersion: contracts.DoctorSchema, Checks: []contracts.Check{
		{Name: "runtime", Status: "pass"}, {Name: "identity", Status: "pass"}, {Name: "catalog", Status: "fail"},
	}}
	if err := localHealth(report, nil); err != nil {
		t.Fatal("offline catalog rejected healthy local runtime")
	}
	report.Checks[1].Status = "fail"
	if err := localHealth(report, nil); err == nil {
		t.Fatal("missing identity accepted")
	}
	if err := localHealth(contracts.DoctorResponse{}, nil); err == nil {
		t.Fatal("empty response accepted")
	}
}

func TestWaitForDaemonReadyRequiresAUsableProbe(t *testing.T) {
	attempts := 0
	err := waitForDaemonReady(time.Second, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("starting")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("wait result err=%v attempts=%d", err, attempts)
	}
}

func TestWaitForDaemonReadyTimesOut(t *testing.T) {
	err := waitForDaemonReady(0, func(context.Context) error { return errors.New("unavailable") })
	if err == nil {
		t.Fatal("wait unexpectedly succeeded")
	}
}

func TestShortcutPathUsesPerUserStartMenu(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)

	got, err := shortcutPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "PF Remote.lnk")
	if got != want {
		t.Fatalf("shortcutPath() = %q, want %q", got, want)
	}
}

func TestInstallerHelperPathUsesLocalApplicationData(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)

	got, err := installerHelperPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(local, "PFRemote", "Installer", "PFRemoteSetup.exe")
	if got != want {
		t.Fatalf("installerHelperPath() = %q, want %q", got, want)
	}
}

func TestBackgroundStartupPlansGatewayThenDaemonThenTray(t *testing.T) {
	commands := backgroundCommands(&gatewayStartupConfig{
		Listen:  "192.0.2.10:8443",
		TLSCert: `D:\PFRemoteTest\gateway.crt`,
		TLSKey:  `D:\PFRemoteTest\gateway.key`,
	})
	if len(commands) != 3 || commands[0].name != "pfremote-gateway.exe" || commands[1].name != "pfremoted.exe" || commands[2].name != "PFRemoteCenter.exe" {
		t.Fatalf("background startup order = %#v", commands)
	}
	want := []string{"--listen", "192.0.2.10:8443", "--tls-cert", `D:\PFRemoteTest\gateway.crt`, "--tls-key", `D:\PFRemoteTest\gateway.key`}
	if fmt.Sprint(commands[0].args) != fmt.Sprint(want) {
		t.Fatalf("gateway startup arguments = %v, want %v", commands[0].args, want)
	}
	if fmt.Sprint(commands[2].args) != fmt.Sprint([]string{"--background"}) {
		t.Fatalf("tray startup arguments = %v", commands[2].args)
	}
}

func TestInteractiveDaemonStartupLeavesCenterToTaskScheduler(t *testing.T) {
	commands := agentBackgroundCommands(nil)
	if len(commands) != 1 || commands[0].name != "pfremoted.exe" {
		t.Fatalf("interactive daemon startup = %#v", commands)
	}
}

func TestBackgroundActivationUsesSignedInDesktopOnlyFromSessionZero(t *testing.T) {
	if !needsInteractiveActivation(0, 2) {
		t.Fatal("session-zero maintenance did not request signed-in desktop activation")
	}
	for _, test := range []struct {
		current uint32
		active  uint32
	}{
		{current: 2, active: 2},
		{current: 0, active: 0},
		{current: 0, active: 0xffffffff},
	} {
		if needsInteractiveActivation(test.current, test.active) {
			t.Fatalf("needsInteractiveActivation(%d, %d) = true", test.current, test.active)
		}
	}
}

func TestCopyExecutableAtomicReplacesCompleteFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.exe")
	destination := filepath.Join(root, "helper", "PFRemoteSetup.exe")
	if err := os.WriteFile(source, []byte("verified setup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyExecutableAtomic(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "verified setup" {
		t.Fatalf("copied helper = %q", got)
	}
	if _, err := os.Stat(destination + ".new"); !os.IsNotExist(err) {
		t.Fatalf("temporary helper remains: %v", err)
	}
}

func TestSamePathIgnoresWindowsCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PFRemoteSetup.exe")
	if !samePath(path, filepath.ToSlash(path)) {
		t.Fatal("samePath did not normalize path separators")
	}
}

func TestInstalledAppsRegistrationLifecycleIsIsolated(t *testing.T) {
	local := t.TempDir()
	registryPath := fmt.Sprintf(`Software\PFRemoteTests\%d\InstalledAppsLifecycle`, os.Getpid())
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("PFREMOTE_SETUP_TESTING", "true")
	t.Setenv("PFREMOTE_SETUP_REGISTRY_PATH", registryPath)
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, registryPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, fmt.Sprintf(`Software\PFRemoteTests\%d`, os.Getpid()))
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\PFRemoteTests`)
	})

	appPath := filepath.Join(local, "Programs", "PFRemote", "versions", "0.1.0-test", "PFRemoteCenter.exe")
	if err := updateInstalledApps(appPath, "0.1.0-test"); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	displayVersion, _, err := key.GetStringValue("DisplayVersion")
	key.Close()
	if err != nil || displayVersion != "0.1.0-test" {
		t.Fatalf("DisplayVersion = %q, err = %v", displayVersion, err)
	}
	helper, err := installerHelperPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Fatalf("registered uninstall helper is missing: %v", err)
	}
	if err := removeInstalledApps(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("isolated Installed apps key remains: %v", err)
	}
	if _, err := os.Stat(helper); !os.IsNotExist(err) {
		t.Fatalf("uninstall helper remains: %v", err)
	}
}

func TestStartupRegistrationUsesStableHelperAndIsIsolated(t *testing.T) {
	local := t.TempDir()
	registryPath := fmt.Sprintf(`Software\PFRemoteTests\%d\StartupLifecycle`, os.Getpid())
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("PFREMOTE_SETUP_TESTING", "true")
	t.Setenv("PFREMOTE_SETUP_RUN_REGISTRY_PATH", registryPath)
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, registryPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, fmt.Sprintf(`Software\PFRemoteTests\%d`, os.Getpid()))
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\PFRemoteTests`)
	})

	if err := updateStartup(); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := key.GetStringValue("PFRemote")
	key.Close()
	helper, _ := installerHelperPath()
	want := fmt.Sprintf("\"%s\" startup --no-launch", helper)
	if err != nil || command != want {
		t.Fatalf("startup command = %q, err = %v, want %q", command, err, want)
	}
	if err := removeStartup(); err != nil {
		t.Fatal(err)
	}
	if key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE); err == nil {
		defer key.Close()
		if _, _, valueErr := key.GetStringValue("PFRemote"); !errors.Is(valueErr, registry.ErrNotExist) {
			t.Fatalf("startup value remains: %v", valueErr)
		}
	}
}

func TestGatewayStartupConfigurationRoundTripsPrivateFiles(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	certificate := filepath.Join(local, "private", "gateway.crt")
	key := filepath.Join(local, "private", "gateway.key")
	if err := os.MkdirAll(filepath.Dir(certificate), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certificate, []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := configureGatewayStartup("127.0.0.1:47832", certificate, key); err != nil {
		t.Fatal(err)
	}
	config, err := loadGatewayStartup()
	if err != nil || config == nil {
		t.Fatalf("loadGatewayStartup() = %#v, %v", config, err)
	}
	if config.Listen != "127.0.0.1:47832" || config.TLSCert != certificate || config.TLSKey != key {
		t.Fatalf("gateway startup = %#v", config)
	}
}

func TestGatewayStartupRejectsMissingTLSFiles(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := configureGatewayStartup("127.0.0.1:47832", `C:\missing\gateway.crt`, `C:\missing\gateway.key`); err == nil {
		t.Fatal("missing Gateway TLS files were accepted")
	}
}
