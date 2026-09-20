// Command alignmentfixture exposes a deterministic isolated action target for
// the external-Codex alignment check. It never connects to a real computer.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type actionRecord struct {
	SchemaVersion string   `json:"schema_version"`
	Action        string   `json:"action"`
	Target        string   `json:"target"`
	Command       []string `json:"command,omitempty"`
}

type recordingShell struct{ path string }

func (r recordingShell) Run(_ context.Context, target string, command []string) (actions.ShellRunResult, error) {
	record := actionRecord{SchemaVersion: "pfremote.agent-control-record/v1", Action: "exec", Target: target, Command: append([]string(nil), command...)}
	if err := writeRecord(r.path, record); err != nil {
		return actions.ShellRunResult{}, err
	}
	return actions.ShellRunResult{SessionID: "session-external-agent-control", ExitCode: 0, Output: "isolated target action completed\n"}, nil
}

type recordingDesktop struct{ path string }

func (r recordingDesktop) Run(_ context.Context, target string) (actions.DesktopRunResult, error) {
	record := actionRecord{SchemaVersion: "pfremote.agent-control-record/v1", Action: "open", Target: target}
	if err := writeRecord(r.path, record); err != nil {
		return actions.DesktopRunResult{}, err
	}
	return actions.DesktopRunResult{
		SessionID:            "session-isolated-desktop-open",
		Protocol:             "rdp",
		RenderingEnvironment: "virtual",
	}, nil
}

func writeRecord(path string, record actionRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".agent-control-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func main() {
	recordPath := flag.String("record", "", "isolated action record path")
	desktopRecordPath := flag.String("desktop-record", "", "isolated desktop-open record path")
	flag.Parse()
	if *recordPath == "" {
		panic("record path is required")
	}
	listener, _, err := localapi.Listen()
	if err != nil {
		panic(err)
	}
	service := actions.New()
	for index := range service.Catalog.Devices {
		if service.Catalog.Devices[index].ID == "device-controller" {
			service.Catalog.Devices[index].State = "offline"
		}
	}
	for index := range service.Catalog.Capabilities {
		if service.Catalog.Capabilities[index].ID == "shell-local" {
			service.Catalog.Capabilities[index].State = "setup-required"
		}
	}
	service.Catalog.Capabilities = append(service.Catalog.Capabilities,
		contracts.Capability{ID: "desktop-three", DeviceID: "device-compute", Alias: "desktop-3", DisplayName: "Virtual Desktop :3", Kind: contracts.CapabilityDesktop, State: "available", DesktopProfile: &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}},
		contracts.Capability{ID: "desktop-four", DeviceID: "device-compute", Alias: "desktop-4", DisplayName: "Virtual Desktop :4", Kind: contracts.CapabilityDesktop, State: "available", DesktopProfile: &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}},
	)
	service.DeviceID = "device-controller"
	service.Shell = recordingShell{path: filepath.Clean(*recordPath)}
	if *desktopRecordPath != "" {
		service.Desktop = recordingDesktop{path: filepath.Clean(*desktopRecordPath)}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := localapi.NewServerWithService(service).Serve(ctx, listener); err != nil {
		panic(err)
	}
}
