package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/capabilitysync"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/migration"
)

type invitationSigner struct {
	id      string
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func (s invitationSigner) DeviceID() string             { return s.id }
func (s invitationSigner) PublicKey() ed25519.PublicKey { return s.public }
func (s invitationSigner) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(s.private, message), nil
}

func TestConnectionInvitationMatchesCurrentSetupAndStatusHidesEndpoint(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), "profile")
	t.Setenv("APPDATA", configRoot)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := identity.DeviceIDFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	invitation, err := capabilitysync.SignInvitation(
		"http://127.0.0.1:43210", "fabric-test", "owner", "invite-test-0001", now.Add(time.Hour),
		invitationSigner{id: deviceID, public: public, private: private},
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(invitation)
	if err != nil {
		t.Fatal(err)
	}
	invitationPath := filepath.Join(t.TempDir(), "join.pfremote-link")
	if err := os.WriteFile(invitationPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	config, err := activateConnectionInvitation(invitationPath, configPath, "fabric-test", deviceID, now)
	if err != nil || !config.Pull {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	var output bytes.Buffer
	if code := run([]string{"status-connection-service"}, &output, &output); code != 0 {
		t.Fatalf("status exit code = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), `"configured": true`) || !strings.Contains(output.String(), `"owner_sync": true`) || strings.Contains(output.String(), "127.0.0.1") {
		t.Fatalf("unsafe status output = %s", output.String())
	}

	wrong, err := capabilitysync.SignInvitation(
		"http://127.0.0.1:43211", "fabric-other", "owner", "invite-test-0002", now.Add(time.Hour),
		invitationSigner{id: deviceID, public: public, private: private},
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongPayload, _ := json.Marshal(wrong)
	wrongPath := filepath.Join(t.TempDir(), "wrong.pfremote-link")
	if err := os.WriteFile(wrongPath, wrongPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := activateConnectionInvitation(wrongPath, configPath, "fabric-test", deviceID, now); err == nil {
		t.Fatal("wrong Fabric invitation changed the profile")
	}
	loaded, err := capabilitysync.LoadConfig(configPath)
	if err != nil || loaded.FabricID != "fabric-test" || loaded.GatewayURL != "http://127.0.0.1:43210" {
		t.Fatalf("active profile changed: %#v err=%v", loaded, err)
	}
}

func TestRunProducesDryRunPlanWithoutWritingBesideInput(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "inventory.json")
	input := `{"schema_version":"pfremote.legacy-inventory/v1","devices":[{"legacy_ref":"legacy-device-a","name":"computer","display_name":"Computer","capabilities":[{"legacy_ref":"legacy-capability-a","name":"shell","display_name":"Shell","kind":"shell","path_count":1}]}]}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"plan", "--input", path}, &output, &output); code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), migration.PlanSchema) {
		t.Fatalf("plan output = %s", output.String())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "inventory.json" {
		t.Fatalf("planner wrote beside source: %#v", entries)
	}
}

func TestRunCanReturnToTheLocalListWithoutTouchingTheLegacySource(t *testing.T) {
	t.Setenv("APPDATA", filepath.Join(t.TempDir(), "profile"))
	var output bytes.Buffer
	if code := run([]string{"enable-legacy-center"}, &output, &output); code != 0 {
		t.Fatalf("enable exit code = %d output = %s", code, output.String())
	}
	output.Reset()
	if code := run([]string{"disable-legacy-center"}, &output, &output); code != 0 {
		t.Fatalf("disable exit code = %d output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), `"status": "disabled"`) {
		t.Fatalf("disable output = %s", output.String())
	}
	output.Reset()
	if code := run([]string{"status-legacy-center"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"enabled": false`) {
		t.Fatalf("status exit code = %d output = %s", code, output.String())
	}
}

func TestReadInventoryRejectsDirectoryAndOversizedFile(t *testing.T) {
	directory := t.TempDir()
	if _, err := readInventory(directory); err == nil {
		t.Fatal("directory was accepted")
	}
	path := filepath.Join(directory, "oversized.json")
	if err := os.WriteFile(path, make([]byte, migration.MaxInputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInventory(path); err == nil {
		t.Fatal("oversized file was accepted")
	}
}

func TestRunExportLegacyCenterWritesOnlyRedactedInventory(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "catalog.json")
	input := `{"deviceId":"owner-device","defaultRoute":"gateway","generatedAt":1,"services":[{"id":"service-a","deviceId":"device-a","deviceName":"Computer","name":"Current screen","kind":"rdp","username":"operator-name","aliyun":{"serverAddress":"192.0.2.10","proxySecret":"fixture-secret"},"tailscale":null,"webUrl":null,"external":null,"lan":null}]}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"export-legacy-center", "--input", path}, &output, &output); code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), migration.InventorySchema) {
		t.Fatalf("export output = %s", output.String())
	}
	for _, forbidden := range []string{"192.0.2.10", "fixture-secret", "operator-name", "defaultRoute", "aliyun"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("export contains %q: %s", forbidden, output.String())
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "catalog.json" {
		t.Fatalf("export wrote beside source: %#v", entries)
	}
}

func TestRunObserveProducesUserFacingComparison(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "legacy.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	legacy := `{"schema_version":"pfremote.migration-observation/v1","source":"legacy","observed_at":"2026-08-29T12:00:00Z","targets":[{"mapping_key":"migration-target-alpha","computer_name":"Lab computer","capability_name":"Current screen","kind":"desktop","result":"ready","timing_class":"normal"}]}`
	candidate := `{"schema_version":"pfremote.migration-observation/v1","source":"candidate","observed_at":"2026-08-29T12:05:00Z","rollback_trigger":"new-path-failure-revocation-or-major-slowdown","targets":[{"mapping_key":"migration-target-alpha","computer_name":"Lab computer","capability_name":"Current screen","kind":"desktop","result":"ready","timing_class":"normal","fallback_rechecked":true}]}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte(candidate), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"observe", "--legacy", legacyPath, "--candidate", candidatePath}, &output, &output); code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), migration.ReportSchema) || !strings.Contains(output.String(), "old path remains available") {
		t.Fatalf("observation output = %s", output.String())
	}
	output.Reset()
	if code := run([]string{"observe", "--legacy", legacyPath, "--candidate", candidatePath, "--format", "review", "--locale", "zh-CN"}, &output, &output); code != 0 {
		t.Fatalf("review exit code = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), "可以进入受控切换准备") || !strings.Contains(output.String(), "Lab computer · Current screen") {
		t.Fatalf("review output = %s", output.String())
	}
}

func TestRunEnablesAndReportsLegacyCenterWithoutPrivateData(t *testing.T) {
	config := t.TempDir()
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	var output bytes.Buffer
	if code := run([]string{"status-legacy-center"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"enabled": false`) {
		t.Fatalf("initial status code=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := run([]string{"enable-legacy-center"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"status": "enabled"`) {
		t.Fatalf("enable code=%d output=%s", code, output.String())
	}
	output.Reset()
	if code := run([]string{"status-legacy-center"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"enabled": true`) {
		t.Fatalf("enabled status code=%d output=%s", code, output.String())
	}
}
