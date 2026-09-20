package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cottman99/pf-remote/internal/autoupdate"
	"github.com/cottman99/pf-remote/internal/windowsinstaller"
)

var (
	releaseVersion      = "development"
	releaseArchitecture = runtime.GOARCH
	payloadSHA256       = ""
	manifestSHA256      = ""
)

func main() {
	arguments := os.Args[1:]
	code := run(arguments)
	if len(arguments) == 0 {
		notifyInstallResult(code == 0)
	}
	os.Exit(code)
}

func run(arguments []string) int {
	command := "install"
	if len(arguments) > 0 && !strings.HasPrefix(arguments[0], "-") {
		command = arguments[0]
		arguments = arguments[1:]
	}
	set := flag.NewFlagSet("pfremote-setup", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	rootOverride := set.String("root", "", "isolated test installation root")
	noLaunch := set.Bool("no-launch", false, "do not launch Center after install")
	gatewayListen := set.String("gateway-listen", "", "private Gateway listener")
	gatewayTLSCert := set.String("gateway-tls-cert", "", "private Gateway certificate")
	gatewayTLSKey := set.String("gateway-tls-key", "", "private Gateway key")
	if set.Parse(arguments) != nil || set.NArg() != 0 {
		return 2
	}
	root, err := installationRoot(*rootOverride)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	var result windowsinstaller.Result
	manageShortcut := *rootOverride == ""
	testingSetup := strings.EqualFold(os.Getenv("PFREMOTE_SETUP_TESTING"), "true")
	manageInstalledApps := manageShortcut && (!testingSetup || strings.TrimSpace(os.Getenv("PFREMOTE_SETUP_REGISTRY_PATH")) != "")
	manageBackground := manageInstalledApps && (!testingSetup || strings.EqualFold(os.Getenv("PFREMOTE_SETUP_START_BACKGROUND"), "true"))
	activate := func(next windowsinstaller.Result) error {
		if manageInstalledApps {
			if err := updateInstalledApps(next.AppPath, next.Current); err != nil {
				return err
			}
		}
		if manageShortcut {
			if err := updateShortcut(next.AppPath); err != nil {
				return err
			}
		}
		if manageInstalledApps {
			if err := updateStartup(); err != nil {
				return err
			}
		}
		if manageBackground {
			return activateBackgroundAfterMaintenance(root, next.AppPath)
		}
		return nil
	}
	switch command {
	case "install", "apply-update":
		executable, executableErr := os.Executable()
		if executableErr != nil {
			err = errors.New("PF Remote Setup could not locate its release files")
			break
		}
		releaseDirectory := filepath.Dir(executable)
		if command == "apply-update" {
			verified, verifyErr := autoupdate.VerifyPackage(releaseDirectory, releaseVersion)
			if verifyErr != nil {
				err = errors.New("automatic update package verification failed")
				break
			}
			_, updateRoot, pathErr := autoupdate.Paths()
			if pathErr != nil {
				err = pathErr
				break
			}
			phase := "failed"
			defer func() {
				_ = autoupdate.WriteStatus(updateRoot, autoupdate.Status{Phase: phase, Version: releaseVersion, Sequence: verified.Metadata().Sequence})
			}()
			if !(*rootOverride != "" && testingSetup) && !autoupdate.Idle() {
				phase = "busy"
				err = errors.New("remote sessions are in use; update deferred")
				break
			}
			defer func() {
				if err == nil {
					phase = "complete"
				}
			}()
		} else if manageBackground {
			if err = autoupdate.Bootstrap(); err != nil {
				break
			}
			// Seed the replay high-water mark from the very first trusted package,
			// before polling can observe an older, still-valid signed release.
			if autoupdate.PublisherKey != "" {
				if _, err = autoupdate.VerifyPackage(releaseDirectory, releaseVersion); err != nil {
					break
				}
			}
		}
		archive := filepath.Join(releaseDirectory, fmt.Sprintf("PFRemote-Windows-%s-%s.zip", releaseArchitecture, releaseVersion))
		manifest := filepath.Join(releaseDirectory, "release-manifest.json")
		_, stateErr := os.Stat(filepath.Join(root, "current.json"))
		if os.IsNotExist(stateErr) {
			result, err = windowsinstaller.Install(root, archive, manifest, payloadSHA256, manifestSHA256)
			if err == nil {
				err = activate(result)
			}
		} else if stateErr != nil {
			err = errors.New("PF Remote installation state could not be inspected")
		} else {
			_, err = windowsinstaller.RecoverUpgrade(root, activate)
			if err == nil {
				result, err = windowsinstaller.Upgrade(root, archive, manifest, payloadSHA256, manifestSHA256, activate)
			}
		}
		if err == nil && !*noLaunch {
			err = exec.Command(result.AppPath).Start()
		}
	case "rollback":
		result, err = windowsinstaller.Rollback(root)
		if err == nil && manageInstalledApps {
			err = updateInstalledApps(result.AppPath, result.Current)
		}
		if err == nil && manageShortcut {
			err = updateShortcut(result.AppPath)
		}
		if err == nil && manageInstalledApps {
			err = updateStartup()
		}
		if err == nil && manageBackground {
			err = activateBackgroundAfterMaintenance(root, result.AppPath)
		}
		if err == nil && !*noLaunch {
			err = exec.Command(result.AppPath).Start()
		}
	case "status":
		result, err = windowsinstaller.Current(root)
	case "startup":
		result, err = windowsinstaller.RecoverUpgrade(root, activate)
		if err == nil {
			err = ensureBackground(result.AppPath)
		}
	case "activate-interactive":
		result, err = windowsinstaller.Current(root)
		if err == nil {
			err = restartBackground(root, result.AppPath)
		}
	case "activate-interactive-daemon":
		result, err = windowsinstaller.Current(root)
		if err == nil {
			err = restartAgentBackground(root, result.AppPath)
		}
	case "configure-gateway-startup":
		err = configureGatewayStartup(*gatewayListen, *gatewayTLSCert, *gatewayTLSKey)
	case "uninstall":
		if manageInstalledApps {
			err = removeStartup()
		}
		if err == nil {
			err = windowsinstaller.Uninstall(root)
		}
		if err == nil && manageShortcut {
			err = removeShortcut()
		}
		if err == nil && manageInstalledApps {
			err = removeInstalledApps()
		}
	default:
		err = errors.New("unknown PF Remote Setup action")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	response := map[string]any{"schema_version": "pfremote.windows-setup-result/v1", "status": "completed", "action": command}
	if command != "uninstall" && command != "configure-gateway-startup" {
		response["current"] = result.Current
		response["previous"] = result.Previous
		response["app_path"] = result.AppPath
	}
	_ = json.NewEncoder(os.Stdout).Encode(response)
	return 0
}

func installationRoot(override string) (string, error) {
	if override != "" {
		if !strings.EqualFold(os.Getenv("PFREMOTE_SETUP_TESTING"), "true") {
			return "", errors.New("custom installation roots are available only to the isolated verifier")
		}
		return filepath.Abs(override)
	}
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if local == "" {
		return "", errors.New("PF Remote Setup could not locate the user application directory")
	}
	return filepath.Join(local, "Programs", "PFRemote"), nil
}
