package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/capabilitysync"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/state"
)

type failingCommitter struct{}

type routeStatusProvider struct {
	acquisition route.Acquisition
	err         error
}

func (p routeStatusProvider) Acquire(context.Context, route.Request) (route.Acquisition, error) {
	return p.acquisition, p.err
}

type routeStatusAcquisition struct{ closed bool }

func (*routeStatusAcquisition) Candidate() route.Candidate { return route.Candidate{} }
func (a *routeStatusAcquisition) Close() error {
	a.closed = true
	return nil
}

func (failingCommitter) CommitSnapshot(context.Context, state.Snapshot) error {
	return errors.New("synthetic persistence failure")
}

func TestGatewayRouteStatusReflectsTheLiveProvider(t *testing.T) {
	acquisition := &routeStatusAcquisition{}
	if got := probeRouteStatus(routeStatusProvider{acquisition: acquisition}, "pfremote://fabric-test/devices/device-a/capabilities/desktop-a", time.Second); got != "available" || !acquisition.closed {
		t.Fatalf("live status=%q closed=%v", got, acquisition.closed)
	}
	if got := probeRouteStatus(routeStatusProvider{err: errors.New("synthetic route failure")}, "pfremote://fabric-test/devices/device-a/capabilities/desktop-a", time.Second); got != "unavailable" {
		t.Fatalf("failed status=%q", got)
	}
	if got := probeRouteStatus(nil, "pfremote://fabric-test/devices/device-a/capabilities/desktop-a", time.Second); got != "unavailable" {
		t.Fatalf("nil status=%q", got)
	}
}

func TestConnectionSyncRetriesPromptlyAfterAHealthFailure(t *testing.T) {
	lastPull := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if connectionSyncDue(false, true, false, lastPull, lastPull.Add(29*time.Second)) {
		t.Fatal("healthy connection synchronized before its normal interval")
	}
	if !connectionSyncDue(false, true, true, lastPull, lastPull.Add(5*time.Second)) {
		t.Fatal("failed connection did not retry on the next health cycle")
	}
	if !connectionSyncDue(false, true, false, lastPull, lastPull.Add(30*time.Second)) {
		t.Fatal("healthy connection did not perform its periodic synchronization")
	}
}

func TestConnectionProfileAppearsWhileDaemonIsAlreadyRunning(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), "profile")
	t.Setenv("APPDATA", configRoot)
	t.Setenv("PFREMOTE_GATEWAY_URL", "")
	t.Setenv("PFREMOTE_GATEWAY_OWNER_SYNC", "")
	settings, err := loadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.GatewayURL != "" {
		t.Fatalf("unexpected initial settings = %#v", settings)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ownerDeviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	path, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	config := capabilitysync.Config{
		SchemaVersion:  capabilitysync.ConfigSchema,
		GatewayURL:     "http://127.0.0.1:43210",
		FabricID:       "fabric-live-profile",
		OwnerDeviceID:  ownerDeviceID,
		OwnerPublicKey: base64.RawURLEncoding.EncodeToString(public),
		Pull:           true,
	}
	if err := capabilitysync.SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	settings, err = loadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.GatewayURL != config.GatewayURL || settings.FabricID != config.FabricID || settings.OwnerDeviceID != ownerDeviceID || !settings.Pull {
		t.Fatalf("live settings = %#v", settings)
	}
	syncer := settings.synchronizer()
	if syncer.ExpectedFabricID != config.FabricID || syncer.ExpectedOwnerDeviceID != ownerDeviceID || !syncer.Pull {
		t.Fatalf("synchronizer = %#v", syncer)
	}
}

func TestInvalidProtectedConnectionProfileDoesNotFallBackToEnvironment(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), "profile")
	t.Setenv("APPDATA", configRoot)
	t.Setenv("PFREMOTE_GATEWAY_URL", "http://127.0.0.1:43210")
	path, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := loadConnectionSettings()
	if err == nil || settings.GatewayURL != "" {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
}

func TestImportedSnapshotCommitFailureIsReturned(t *testing.T) {
	err := commitImportedSnapshot(context.Background(), failingCommitter{}, state.Snapshot{}, 1)
	if err == nil {
		t.Fatal("persistence failure was hidden")
	}
	if err := commitImportedSnapshot(context.Background(), failingCommitter{}, state.Snapshot{}, 0); err != nil {
		t.Fatalf("zero-import synchronization required persistence: %v", err)
	}
}
