package desktopruntime

import (
	"context"
	"errors"
	"github.com/cottman99/pf-remote/internal/route"
	"os"
	"path/filepath"
	"testing"
)

type recordedVerifier struct {
	address string
	nodeID  string
	err     error
}

func (v *recordedVerifier) VerifyPeer(_ context.Context, address, nodeID string) error {
	v.address, v.nodeID = address, nodeID
	return v.err
}

func TestLoadProvidesExactTargetRouteAndTrust(t *testing.T) {
	directory := t.TempDir()
	trust := filepath.Join(directory, "vnc-ca.pem")
	if err := os.WriteFile(trust, []byte("synthetic public certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "desktop-runtime-v1.json")
	data := `{"schema_version":"pfremote.desktop-runtime/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","route_id":"route-lan-1","adapter":"lan","address":"192.0.2.10","port":5901,"vnc_trust_file":"` + filepath.ToSlash(trust) + `"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	target := "pfremote://fabric-test/devices/device-compute/capabilities/desktop-main"
	acquired, err := config.Acquire(context.Background(), route.Request{CanonicalTarget: target})
	if err != nil || acquired.Candidate().Address != "192.0.2.10" {
		t.Fatalf("candidate=%#v err=%v", acquired, err)
	}
	if got, err := config.VNCTrustFile(target); err != nil || got != trust {
		t.Fatalf("trust=%q err=%v", got, err)
	}
}

func TestTailscaleRouteRequiresCurrentImmutablePeer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-runtime-v1.json")
	data := `{"schema_version":"pfremote.desktop-runtime/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","route_id":"route-ts-1","adapter":"tailscale","address":"demo-laptop","port":5900,"tailscale_node_id":"node-synthetic"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	target := "pfremote://fabric-test/devices/device-compute/capabilities/desktop-main"
	verifier := &recordedVerifier{}
	config = config.WithTailscaleVerifier(verifier)
	if _, err := config.Acquire(context.Background(), route.Request{CanonicalTarget: target}); err != nil {
		t.Fatal(err)
	}
	if verifier.address != "demo-laptop" || verifier.nodeID != "node-synthetic" {
		t.Fatalf("verification=%q %q", verifier.address, verifier.nodeID)
	}
	verifier.err = errors.New("offline")
	if _, err := config.Acquire(context.Background(), route.Request{CanonicalTarget: target}); err == nil {
		t.Fatal("expected unavailable Tailscale identity to fail")
	}
}

func TestLoadRejectsUnknownFieldsAndUnsupportedAdapters(t *testing.T) {
	for name, data := range map[string]string{
		"unknown":            `{"schema_version":"pfremote.desktop-runtime/v1","targets":[],"secret":"bad"}`,
		"adapter":            `{"schema_version":"pfremote.desktop-runtime/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","route_id":"route-frp-1","adapter":"frp","address":"192.0.2.10","port":3389}]}`,
		"tailscale identity": `{"schema_version":"pfremote.desktop-runtime/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","route_id":"route-ts-1","adapter":"tailscale","address":"demo-laptop","port":5900}]}`,
		"profile field":      `{"schema_version":"pfremote.desktop-runtime/v1","targets":[{"canonical_target":"pfremote://fabric-test/devices/device-compute/capabilities/desktop-main","route_id":"route-ts-1","adapter":"tailscale","address":"demo-laptop","port":5900,"tailscale_node_id":"node-synthetic","protocol":"vnc"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
}
