package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cottman99/pf-remote/internal/capabilitysync"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/migration"
	"github.com/cottman99/pf-remote/internal/state"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(arguments []string, output, errorOutput io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprintln(errorOutput, "migration action is required")
		return 2
	}
	switch arguments[0] {
	case "export-legacy-center":
		return runExportLegacyCenter(arguments[1:], output, errorOutput)
	case "enable-legacy-center":
		return runEnableLegacyCenter(arguments[1:], output, errorOutput)
	case "disable-legacy-center":
		return runDisableLegacyCenter(arguments[1:], output, errorOutput)
	case "status-legacy-center":
		return runStatusLegacyCenter(arguments[1:], output, errorOutput)
	case "configure-connection-service":
		return runConfigureConnectionService(arguments[1:], output, errorOutput)
	case "status-connection-service":
		return runStatusConnectionService(arguments[1:], output, errorOutput)
	case "plan":
		return runPlan(arguments[1:], output, errorOutput)
	case "observe":
		return runObserve(arguments[1:], output, errorOutput)
	default:
		fmt.Fprintln(errorOutput, "migration action is unsupported")
		return 2
	}
}

func runConfigureConnectionService(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("configure-connection-service", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	invitationPath := set.String("invitation", "", "PF Remote connection invitation")
	if set.Parse(arguments) != nil || set.NArg() != 0 || *invitationPath == "" {
		fmt.Fprintln(errorOutput, "connection invitation options are invalid")
		return 2
	}
	configPath, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		fmt.Fprintln(errorOutput, "PF Remote connection settings could not be located")
		return 1
	}
	// Catalog listing includes bounded route-health checks. Connecting a new
	// computer must tolerate that normal work instead of reporting that a
	// healthy local daemon is stopped.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	catalog, catalogErr := localapi.NewClient().List(ctx)
	cancel()
	deviceID, identityErr := currentDeviceID()
	if catalogErr != nil || identityErr != nil {
		fmt.Fprintln(errorOutput, "PF Remote must be running before connecting the service")
		return 1
	}
	config, err := activateConnectionInvitation(*invitationPath, configPath, catalog.FabricID, deviceID, time.Now())
	if err != nil {
		fmt.Fprintln(errorOutput, "connection invitation does not match this PF Remote setup")
		return 1
	}
	return writeJSON(output, errorOutput, map[string]any{"status": "configured", "owner_sync": config.Pull})
}

func activateConnectionInvitation(invitationPath, configPath, currentFabricID, currentDeviceID string, now time.Time) (capabilitysync.Config, error) {
	config, err := capabilitysync.ReadInvitationFile(invitationPath, now)
	if err != nil {
		return capabilitysync.Config{}, err
	}
	if config.FabricID != currentFabricID || (config.Pull && config.OwnerDeviceID != currentDeviceID) {
		return capabilitysync.Config{}, errors.New("connection invitation identity mismatch")
	}
	if err := capabilitysync.SaveConfig(configPath, config); err != nil {
		return capabilitysync.Config{}, err
	}
	return config, nil
}

func currentDeviceID() (string, error) {
	path, err := identity.DefaultPath()
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("PF Remote Device identity is unavailable")
	}
	store, err := identity.NewStore(path)
	if err != nil {
		return "", err
	}
	loaded, err := store.LoadOrCreate()
	if err != nil {
		return "", err
	}
	return loaded.DeviceID(), nil
}

func runStatusConnectionService(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("status-connection-service", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if set.Parse(arguments) != nil || set.NArg() != 0 {
		fmt.Fprintln(errorOutput, "connection service status options are invalid")
		return 2
	}
	configPath, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		fmt.Fprintln(errorOutput, "PF Remote connection settings could not be located")
		return 1
	}
	config, err := capabilitysync.LoadConfig(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return writeJSON(output, errorOutput, map[string]bool{"configured": false})
	}
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, map[string]any{"configured": true, "owner_sync": config.Pull})
}

func runStatusLegacyCenter(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("status-legacy-center", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if set.Parse(arguments) != nil || set.NArg() != 0 {
		fmt.Fprintln(errorOutput, "existing setup status options are invalid")
		return 2
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		fmt.Fprintln(errorOutput, "PF Remote local state could not be located")
		return 1
	}
	enabled, err := migration.LegacyCenterEnabled(migration.LegacyActivationPathForState(statePath))
	if errors.Is(err, os.ErrNotExist) {
		enabled, err = false, nil
	}
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, map[string]bool{"enabled": enabled})
}

func runEnableLegacyCenter(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("enable-legacy-center", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if set.Parse(arguments) != nil || set.NArg() != 0 {
		fmt.Fprintln(errorOutput, "existing setup activation options are invalid")
		return 2
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		fmt.Fprintln(errorOutput, "PF Remote local state could not be located")
		return 1
	}
	if err := migration.EnableLegacyCenter(migration.LegacyActivationPathForState(statePath)); err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, map[string]string{"status": "enabled"})
}

func runDisableLegacyCenter(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("disable-legacy-center", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if set.Parse(arguments) != nil || set.NArg() != 0 {
		fmt.Fprintln(errorOutput, "existing setup rollback options are invalid")
		return 2
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		fmt.Fprintln(errorOutput, "PF Remote local state could not be located")
		return 1
	}
	if err := migration.DisableLegacyCenter(migration.LegacyActivationPathForState(statePath)); err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, map[string]string{"status": "disabled"})
}

func runExportLegacyCenter(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("export-legacy-center", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	inputPath := set.String("input", "", "explicit legacy PF Remote Center catalog")
	if set.Parse(arguments) != nil || set.NArg() != 0 || *inputPath == "" {
		fmt.Fprintln(errorOutput, "legacy Center export options are invalid")
		return 2
	}
	input, err := readInventory(*inputPath)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	inventory, err := migration.ExportLegacyCenterCatalog(input)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, inventory)
}

func runPlan(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("plan", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	inputPath := set.String("input", "", "explicit redacted inventory file")
	if set.Parse(arguments) != nil || set.NArg() != 0 || *inputPath == "" {
		fmt.Fprintln(errorOutput, "migration plan options are invalid")
		return 2
	}
	input, err := readInventory(*inputPath)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	plan, err := migration.ParseAndPlan(input)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return writeJSON(output, errorOutput, plan)
}

func runObserve(arguments []string, output, errorOutput io.Writer) int {
	set := flag.NewFlagSet("observe", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	legacyPath := set.String("legacy", "", "explicit redacted legacy observation")
	candidatePath := set.String("candidate", "", "explicit redacted candidate observation")
	format := set.String("format", "json", "json or review")
	locale := set.String("locale", "zh-CN", "review locale")
	if set.Parse(arguments) != nil || set.NArg() != 0 || *legacyPath == "" || *candidatePath == "" || (*format != "json" && *format != "review") {
		fmt.Fprintln(errorOutput, "migration observation options are invalid")
		return 2
	}
	legacy, err := readInventory(*legacyPath)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	candidate, err := readInventory(*candidatePath)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	report, err := migration.CompareObservations(legacy, candidate)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	if *format == "review" {
		review, err := migration.RenderObservationReview(report, *locale)
		if err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
		if _, err := io.WriteString(output, review); err != nil {
			fmt.Fprintln(errorOutput, "migration review could not be written")
			return 1
		}
		return 0
	}
	return writeJSON(output, errorOutput, report)
}

func writeJSON(output, errorOutput io.Writer, value any) int {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(errorOutput, "migration plan could not be written")
		return 1
	}
	return 0
}

func readInventory(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("legacy inventory file could not be inspected")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > migration.MaxInputBytes {
		return nil, errors.New("legacy inventory must be one bounded regular file, not a link")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("legacy inventory file could not be opened")
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, migration.MaxInputBytes+1))
	if err != nil || len(input) == 0 || len(input) > migration.MaxInputBytes {
		return nil, errors.New("legacy inventory file could not be read safely")
	}
	return input, nil
}
