//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var messageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

const uninstallRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\PFRemote`
const runRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const gatewayStartupSchema = "pfremote.windows-gateway-startup/v1"
const interactiveActivationTaskName = "PFRemote-Interactive-Activation"

type gatewayStartupConfig struct {
	SchemaVersion string `json:"schema_version"`
	Listen        string `json:"listen"`
	TLSCert       string `json:"tls_cert"`
	TLSKey        string `json:"tls_key"`
}

type backgroundCommand struct {
	name string
	args []string
}

func notifyInstallResult(success bool) {
	title := "PF Remote"
	message := "PF Remote is ready. You can open it from the Start menu."
	flags := uintptr(0x00000040) // MB_ICONINFORMATION
	if !success {
		message = "PF Remote could not complete installation. Open PF Remote again to retry recovery of the previous version. Your saved configuration has not been replaced."
		flags = 0x00000010 // MB_ICONERROR
	}
	messagePointer, _ := windows.UTF16PtrFromString(message)
	titlePointer, _ := windows.UTF16PtrFromString(title)
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(messagePointer)), uintptr(unsafe.Pointer(titlePointer)), flags)
}

func updateShortcut(appPath string) error {
	shortcut, err := shortcutPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(shortcut), 0o700); err != nil {
		return errors.New("PF Remote Start menu entry could not be created")
	}
	const script = `$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($env:PFREMOTE_SHORTCUT_PATH)
$shortcut.TargetPath = $env:PFREMOTE_SHORTCUT_TARGET
$shortcut.WorkingDirectory = [IO.Path]::GetDirectoryName($env:PFREMOTE_SHORTCUT_TARGET)
$shortcut.Description = 'PF Remote'
$shortcut.Save()`
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded)
	command.Env = append(os.Environ(), "PFREMOTE_SHORTCUT_PATH="+shortcut, "PFREMOTE_SHORTCUT_TARGET="+appPath)
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Run(); err != nil {
		return errors.New("PF Remote Start menu entry could not be created")
	}
	return nil
}

func removeShortcut() error {
	shortcut, err := shortcutPath()
	if err != nil {
		return err
	}
	if err := os.Remove(shortcut); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("PF Remote Start menu entry could not be removed")
	}
	return nil
}

func shortcutPath() (string, error) {
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	if appData == "" {
		return "", errors.New("PF Remote Setup could not locate the Start menu")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "PF Remote.lnk"), nil
}

func updateInstalledApps(appPath, version string) error {
	registryPath, err := installedAppsRegistryPath()
	if err != nil {
		return err
	}
	helper, err := installerHelperPath()
	if err != nil {
		return err
	}
	current, err := os.Executable()
	if err != nil {
		return errors.New("PF Remote Setup could not locate its installer")
	}
	if !samePath(current, helper) {
		if err := copyExecutableAtomic(current, helper); err != nil {
			return err
		}
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return errors.New("PF Remote could not be added to Windows Installed apps")
	}
	defer key.Close()
	installRoot := filepath.Dir(filepath.Dir(filepath.Dir(appPath)))
	uninstallCommand := fmt.Sprintf("\"%s\" uninstall --no-launch", helper)
	values := map[string]string{
		"DisplayName":          "PF Remote",
		"DisplayVersion":       version,
		"Publisher":            "PF Remote",
		"DisplayIcon":          appPath + ",0",
		"InstallLocation":      installRoot,
		"UninstallString":      uninstallCommand,
		"QuietUninstallString": uninstallCommand,
	}
	for name, value := range values {
		if err := key.SetStringValue(name, value); err != nil {
			return errors.New("PF Remote could not be added to Windows Installed apps")
		}
	}
	if key.SetDWordValue("NoModify", 1) != nil || key.SetDWordValue("NoRepair", 1) != nil {
		return errors.New("PF Remote could not be added to Windows Installed apps")
	}
	return nil
}

func removeInstalledApps() error {
	registryPath, err := installedAppsRegistryPath()
	if err != nil {
		return err
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, registryPath); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return errors.New("PF Remote could not be removed from Windows Installed apps")
	}
	cleanupTestRegistryParents(registryPath)
	helper, err := installerHelperPath()
	if err != nil {
		return err
	}
	current, currentErr := os.Executable()
	if currentErr == nil && samePath(current, helper) {
		return scheduleSelfRemoval(helper)
	}
	if err := os.Remove(helper); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("PF Remote uninstall helper could not be removed")
	}
	_ = os.Remove(filepath.Dir(helper))
	return nil
}

func updateStartup() error {
	helper, err := installerHelperPath()
	if err != nil {
		return err
	}
	registryPath, err := startupRegistryPath()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return errors.New("PF Remote background startup could not be configured")
	}
	defer key.Close()
	return key.SetStringValue("PFRemote", fmt.Sprintf("\"%s\" startup --no-launch", helper))
}

func removeStartup() error {
	registryPath, pathErr := startupRegistryPath()
	if pathErr != nil {
		return pathErr
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return errors.New("PF Remote background startup could not be removed")
	}
	defer key.Close()
	if err := key.DeleteValue("PFRemote"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return errors.New("PF Remote background startup could not be removed")
	}
	key.Close()
	cleanupTestRegistryParents(registryPath)
	return nil
}

func startupRegistryPath() (string, error) {
	override := strings.TrimSpace(os.Getenv("PFREMOTE_SETUP_RUN_REGISTRY_PATH"))
	if override == "" {
		return runRegistryPath, nil
	}
	if !strings.EqualFold(os.Getenv("PFREMOTE_SETUP_TESTING"), "true") || !strings.HasPrefix(override, `Software\PFRemoteTests\`) {
		return "", errors.New("custom startup registry paths are available only to the isolated verifier")
	}
	return override, nil
}

func ensureBackground(appPath string) error {
	versionRoot := filepath.Dir(appPath)
	gateway, err := loadGatewayStartup()
	if err != nil {
		return err
	}
	running, err := runningBackground(versionRoot)
	if err != nil {
		return err
	}
	for _, item := range backgroundCommands(gateway) {
		if running[strings.ToLower(item.name)] {
			continue
		}
		path := filepath.Join(versionRoot, item.name)
		if _, err := os.Stat(path); err != nil {
			return errors.New("PF Remote background component is unavailable")
		}
		command := exec.Command(path, item.args...)
		command.Dir = versionRoot
		command.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		if err := command.Start(); err != nil {
			return errors.New("PF Remote background component could not be started")
		}
	}
	client := localapi.NewClient()
	return waitForDaemonReady(10*time.Second, func(ctx context.Context) error {
		active, err := runningBackground(versionRoot)
		if err != nil {
			return err
		}
		for _, item := range backgroundCommands(gateway) {
			if !active[strings.ToLower(item.name)] {
				return errors.New("PF Remote selected background version is not running")
			}
		}
		report, err := client.Doctor(ctx)
		return localHealth(report, err)
	})
}

func activateBackgroundAfterMaintenance(root, appPath string) error {
	var currentSession uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &currentSession); err != nil {
		return errors.New("PF Remote could not inspect the installer session")
	}
	activeSession := windows.WTSGetActiveConsoleSessionId()
	if !needsInteractiveActivation(currentSession, activeSession) {
		return restartBackground(root, appPath)
	}
	// The session-zero maintenance process owns any background set that was
	// started by SSH or an earlier installer run. Stop that set before handing
	// startup to the signed-in desktop; otherwise the old daemon can keep the
	// local API endpoint while only the new Center reaches the visible session.
	if err := stopOwnedBackground(root); err != nil {
		return err
	}
	if err := requestInteractiveActivation(appPath); err != nil {
		return err
	}
	return waitForInteractiveBackground(filepath.Dir(appPath), activeSession, 15*time.Second)
}

func needsInteractiveActivation(currentSession, activeSession uint32) bool {
	return currentSession == 0 && activeSession != 0 && activeSession != 0xffffffff
}

func requestInteractiveActivation(appPath string) error {
	helper, err := installerHelperPath()
	if err != nil {
		return err
	}
	const script = `$identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
$daemonAction = New-ScheduledTaskAction -Execute $env:PFREMOTE_SETUP_HELPER -Argument 'activate-interactive-daemon --no-launch'
$centerAction = New-ScheduledTaskAction -Execute $env:PFREMOTE_CENTER -Argument '--background'
$principal = New-ScheduledTaskPrincipal -UserId $identity -LogonType Interactive -RunLevel Limited
Stop-ScheduledTask -TaskName $env:PFREMOTE_SETUP_TASK -ErrorAction SilentlyContinue
Register-ScheduledTask -TaskName $env:PFREMOTE_SETUP_TASK -Action @($daemonAction, $centerAction) -Principal $principal -Force | Out-Null
Start-ScheduledTask -TaskName $env:PFREMOTE_SETUP_TASK`
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded)
	command.Env = append(os.Environ(),
		"PFREMOTE_SETUP_HELPER="+helper,
		"PFREMOTE_CENTER="+appPath,
		"PFREMOTE_SETUP_TASK="+interactiveActivationTaskName)
	command.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Run(); err != nil {
		return errors.New("PF Remote could not activate the current version in the signed-in desktop")
	}
	return nil
}

func waitForInteractiveBackground(versionRoot string, sessionID uint32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		running, err := runningBackgroundInSession(versionRoot, &sessionID)
		if err != nil {
			return err
		}
		if running["pfremoted.exe"] && running["pfremotecenter.exe"] {
			return waitForDaemonReady(5*time.Second, func(ctx context.Context) error {
				report, err := localapi.NewClient().Doctor(ctx)
				return localHealth(report, err)
			})
		}
		if !time.Now().Before(deadline) {
			return errors.New("PF Remote did not appear in the signed-in desktop")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitForDaemonReady(timeout time.Duration, probe func(context.Context) error) error {
	deadline := time.Now().Add(timeout)
	for {
		probeContext, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
		err := probe(probeContext)
		cancel()
		if err == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errors.New("PF Remote background did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func backgroundCommands(gateway *gatewayStartupConfig) []backgroundCommand {
	return append(agentBackgroundCommands(gateway), backgroundCommand{name: "PFRemoteCenter.exe", args: []string{"--background"}})
}

func agentBackgroundCommands(gateway *gatewayStartupConfig) []backgroundCommand {
	commands := make([]backgroundCommand, 0, 3)
	if gateway != nil {
		commands = append(commands, backgroundCommand{
			name: "pfremote-gateway.exe",
			args: []string{"--listen", gateway.Listen, "--tls-cert", gateway.TLSCert, "--tls-key", gateway.TLSKey},
		})
	}
	return append(commands, backgroundCommand{name: "pfremoted.exe"})
}

func runningBackground(versionRoot string) (map[string]bool, error) {
	return runningBackgroundInSession(versionRoot, nil)
}

func runningBackgroundInSession(versionRoot string, requiredSession *uint32) (map[string]bool, error) {
	root, err := filepath.Abs(versionRoot)
	if err != nil {
		return nil, errors.New("PF Remote installation path could not be resolved")
	}
	running := make(map[string]bool, 3)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, errors.New("PF Remote background processes could not be inspected")
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name != "pfremoted.exe" && name != "pfremote-gateway.exe" && name != "pfremotecenter.exe" {
			continue
		}
		if requiredSession != nil {
			var processSession uint32
			if windows.ProcessIdToSessionId(entry.ProcessID, &processSession) != nil || processSession != *requiredSession {
				continue
			}
		}
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		pathBuffer := make([]uint16, windows.MAX_PATH*4)
		pathLength := uint32(len(pathBuffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &pathBuffer[0], &pathLength)
		windows.CloseHandle(process)
		if queryErr == nil && strings.EqualFold(filepath.Dir(filepath.Clean(windows.UTF16ToString(pathBuffer[:pathLength]))), root) {
			running[name] = true
		}
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, errors.New("PF Remote background processes could not be inspected")
	}
	return running, nil
}

func configureGatewayStartup(listen, certificate, key string) error {
	config := gatewayStartupConfig{SchemaVersion: gatewayStartupSchema, Listen: strings.TrimSpace(listen), TLSCert: filepath.Clean(certificate), TLSKey: filepath.Clean(key)}
	if err := validateGatewayStartup(config); err != nil {
		return err
	}
	path, err := gatewayStartupPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("PF Remote Gateway startup directory could not be created")
	}
	input, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return errors.New("PF Remote Gateway startup could not be encoded")
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, append(input, '\n'), 0o600); err != nil {
		return errors.New("PF Remote Gateway startup could not be saved")
	}
	_ = os.Remove(path)
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return errors.New("PF Remote Gateway startup could not be activated")
	}
	return nil
}

func loadGatewayStartup() (*gatewayStartupConfig, error) {
	path, err := gatewayStartupPath()
	if err != nil {
		return nil, err
	}
	input, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("PF Remote Gateway startup could not be read")
	}
	var config gatewayStartupConfig
	if json.Unmarshal(input, &config) != nil {
		return nil, errors.New("PF Remote Gateway startup is invalid")
	}
	if err := validateGatewayStartup(config); err != nil {
		return nil, err
	}
	return &config, nil
}

func gatewayStartupPath() (string, error) {
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if local == "" {
		return "", errors.New("PF Remote Setup could not locate the user application directory")
	}
	return filepath.Join(local, "PFRemote", "Gateway", "startup-v1.json"), nil
}

func validateGatewayStartup(config gatewayStartupConfig) error {
	if config.SchemaVersion != gatewayStartupSchema {
		return errors.New("PF Remote Gateway startup is invalid")
	}
	host, port, err := net.SplitHostPort(config.Listen)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return errors.New("PF Remote Gateway listener is invalid")
	}
	for _, path := range []string{config.TLSCert, config.TLSKey} {
		if !filepath.IsAbs(path) {
			return errors.New("PF Remote Gateway TLS file is invalid")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("PF Remote Gateway TLS file is unavailable")
		}
	}
	return nil
}

func restartBackground(root, appPath string) error {
	if err := stopOwnedBackground(root); err != nil {
		return err
	}
	return ensureBackground(appPath)
}

func restartAgentBackground(root, appPath string) error {
	if err := stopOwnedBackground(root); err != nil {
		return err
	}
	versionRoot := filepath.Dir(appPath)
	gateway, err := loadGatewayStartup()
	if err != nil {
		return err
	}
	for _, item := range agentBackgroundCommands(gateway) {
		path := filepath.Join(versionRoot, item.name)
		if _, err := os.Stat(path); err != nil {
			return errors.New("PF Remote background component is unavailable")
		}
		command := exec.Command(path, item.args...)
		command.Dir = versionRoot
		command.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		if err := command.Start(); err != nil {
			return errors.New("PF Remote background component could not be started")
		}
	}
	return waitForDaemonReady(10*time.Second, func(ctx context.Context) error {
		report, err := localapi.NewClient().Doctor(ctx)
		return localHealth(report, err)
	})
}

func stopOwnedBackground(root string) error {
	versionsRoot, err := filepath.Abs(filepath.Join(root, "versions"))
	if err != nil {
		return errors.New("PF Remote installation path could not be resolved")
	}
	versionsPrefix := strings.ToLower(filepath.Clean(versionsRoot) + string(os.PathSeparator))
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return errors.New("PF Remote background processes could not be inspected")
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name != "pfremoted.exe" && name != "pfremote-gateway.exe" && name != "pfremotecenter.exe" {
			continue
		}
		process, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, entry.ProcessID)
		if openErr != nil {
			continue
		}
		pathBuffer := make([]uint16, windows.MAX_PATH*4)
		pathLength := uint32(len(pathBuffer))
		queryErr := windows.QueryFullProcessImageName(process, 0, &pathBuffer[0], &pathLength)
		path := strings.ToLower(filepath.Clean(windows.UTF16ToString(pathBuffer[:pathLength])))
		if queryErr == nil && strings.HasPrefix(path, versionsPrefix) {
			if terminateErr := windows.TerminateProcess(process, 0); terminateErr != nil {
				windows.CloseHandle(process)
				return errors.New("PF Remote background process could not be switched")
			}
			wait, waitErr := windows.WaitForSingleObject(process, 5000)
			if waitErr != nil || wait != windows.WAIT_OBJECT_0 {
				windows.CloseHandle(process)
				return errors.New("PF Remote background process has not stopped")
			}
		}
		windows.CloseHandle(process)
	}
	if err != nil && !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return errors.New("PF Remote background processes could not be inspected")
	}
	return nil
}

// Offline remote services are not local startup failures. Require the protected
// local API, runtime and per-user identity to be available.
func localHealth(report contracts.DoctorResponse, err error) error {
	if err != nil || report.SchemaVersion != contracts.DoctorSchema {
		return errors.New("PF Remote local health is unavailable")
	}
	runtimeOK, identityOK := false, false
	for _, check := range report.Checks {
		if check.Name == "runtime" {
			runtimeOK = check.Status == "pass"
		}
		if check.Name == "identity" {
			identityOK = check.Status == "pass"
		}
	}
	if !runtimeOK || !identityOK {
		return errors.New("PF Remote local identity or runtime is unavailable")
	}
	return nil
}

func cleanupTestRegistryParents(registryPath string) {
	if !strings.EqualFold(os.Getenv("PFREMOTE_SETUP_TESTING"), "true") || !strings.HasPrefix(registryPath, `Software\PFRemoteTests\`) {
		return
	}
	parent := registryPath
	for {
		separator := strings.LastIndex(parent, `\`)
		if separator < 0 {
			return
		}
		parent = parent[:separator]
		if !strings.HasPrefix(parent, `Software\PFRemoteTests`) {
			return
		}
		_ = registry.DeleteKey(registry.CURRENT_USER, parent)
		if parent == `Software\PFRemoteTests` {
			return
		}
	}
}

func installedAppsRegistryPath() (string, error) {
	override := strings.TrimSpace(os.Getenv("PFREMOTE_SETUP_REGISTRY_PATH"))
	if override == "" {
		return uninstallRegistryPath, nil
	}
	if !strings.EqualFold(os.Getenv("PFREMOTE_SETUP_TESTING"), "true") || !strings.HasPrefix(override, `Software\PFRemoteTests\`) {
		return "", errors.New("custom Installed apps registry paths are available only to the isolated verifier")
	}
	return override, nil
}

func installerHelperPath() (string, error) {
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if local == "" {
		return "", errors.New("PF Remote Setup could not locate the user application directory")
	}
	return filepath.Join(local, "PFRemote", "Installer", "PFRemoteSetup.exe"), nil
}

func copyExecutableAtomic(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return errors.New("PF Remote uninstall helper directory could not be created")
	}
	temporary := destination + ".new"
	input, err := os.Open(source)
	if err != nil {
		return errors.New("PF Remote installer could not be read")
	}
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		input.Close()
		return errors.New("PF Remote uninstall helper could not be written")
	}
	_, copyErr := io.Copy(output, input)
	closeOutputErr := output.Close()
	closeInputErr := input.Close()
	if copyErr != nil || closeOutputErr != nil || closeInputErr != nil {
		_ = os.Remove(temporary)
		return errors.New("PF Remote uninstall helper could not be written")
	}
	_ = os.Remove(destination)
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return errors.New("PF Remote uninstall helper could not be activated")
	}
	return nil
}

func scheduleSelfRemoval(helper string) error {
	const script = `Wait-Process -Id ([int]$env:PFREMOTE_SETUP_PID) -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $env:PFREMOTE_SETUP_HELPER -Force -ErrorAction SilentlyContinue
$parent = Split-Path -Parent $env:PFREMOTE_SETUP_HELPER
if ((Get-ChildItem -LiteralPath $parent -Force -ErrorAction SilentlyContinue | Measure-Object).Count -eq 0) {
    Remove-Item -LiteralPath $parent -Force -ErrorAction SilentlyContinue
}`
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encoded)
	command.Env = append(os.Environ(), fmt.Sprintf("PFREMOTE_SETUP_PID=%d", os.Getpid()), "PFREMOTE_SETUP_HELPER="+helper)
	if err := command.Start(); err != nil {
		return errors.New("PF Remote uninstall helper cleanup could not be scheduled")
	}
	return nil
}

func samePath(first, second string) bool {
	firstPath, firstErr := filepath.Abs(first)
	secondPath, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && strings.EqualFold(filepath.Clean(firstPath), filepath.Clean(secondPath))
}

func utf16LE(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	result := make([]byte, len(encoded)*2)
	for index, character := range encoded {
		result[index*2] = byte(character)
		result[index*2+1] = byte(character >> 8)
	}
	return result
}
