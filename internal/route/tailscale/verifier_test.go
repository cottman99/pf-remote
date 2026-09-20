package tailscale

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fixedStatus []byte

func (s fixedStatus) StatusJSON(context.Context) ([]byte, error) { return s, nil }

func TestVerifierRequiresExactOnlineNodeIdentity(t *testing.T) {
	status := fixedStatus(`{"Peer":{"peer-key":{"ID":"node-synthetic","HostName":"demo-laptop","DNSName":"demo-laptop.example.ts.net.","TailscaleIPs":["192.0.2.40"],"Online":true}}}`)
	verifier := Verifier{Command: status}
	if err := verifier.VerifyPeer(context.Background(), "demo-laptop", "node-synthetic"); err != nil {
		t.Fatal(err)
	}
	if err := verifier.VerifyPeer(context.Background(), "demo-laptop", "node-other"); err == nil {
		t.Fatal("expected wrong immutable node identity to fail")
	}
	if err := verifier.VerifyPeer(context.Background(), "192.0.2.40", "node-synthetic"); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryResolvesAndPinsOneOnlinePeerWithoutSerialization(t *testing.T) {
	status := fixedStatus(`{"Peer":{"peer-key":{"ID":"node-synthetic","HostName":"demo-laptop","DNSName":"demo-laptop.example.ts.net.","TailscaleIPs":["192.0.2.40"],"Online":true},"offline":{"ID":"node-offline","HostName":"old-laptop","Online":false}}}`)
	directory, err := LoadDirectory(context.Background(), status)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := directory.ResolvePeer(context.Background(), "demo-laptop.example.ts.net")
	if err != nil || nodeID != "node-synthetic" {
		t.Fatalf("node=%q err=%v", nodeID, err)
	}
	if err := directory.VerifyPeer(context.Background(), "192.0.2.40", nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.ResolvePeer(context.Background(), "old-laptop"); err == nil {
		t.Fatal("offline peer was resolved")
	}
	encoded, err := json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "node-synthetic") || strings.Contains(string(encoded), "demo-laptop") {
		t.Fatalf("directory serialization leaked peer details: %s", encoded)
	}
}
